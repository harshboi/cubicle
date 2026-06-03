import Foundation

private let webexSignalSyncStateCheckpointKey = "sync-state"

/// Deferred Webex state store used by the signal connector path.
actor WebexSignalSyncStateStore: WebexSyncStateStoring {
    typealias LegacyStateLookup = (String) throws -> WebexConversationSyncStateRecord?

    private let accountID: String
    private let checkpoints: ConnectorCheckpointSet
    private let legacyStateLookup: LegacyStateLookup?
    private let now: () -> Date
    private var targetIDByConversationID: [String: String] = [:]
    private var pendingStatesByConversationID: [String: WebexConversationSyncStateRecord] = [:]

    init(
        accountID: String,
        checkpoints: ConnectorCheckpointSet,
        legacyStateLookup: LegacyStateLookup? = nil,
        now: @escaping () -> Date = Date.init
    ) {
        self.accountID = accountID
        self.checkpoints = checkpoints
        self.legacyStateLookup = legacyStateLookup
        self.now = now
    }

    func loadOrCreate(for conversation: WebexTrackedConversation) async throws -> WebexConversationSyncStateRecord {
        let targetID = Self.checkpointTargetID(for: conversation)
        targetIDByConversationID[conversation.conversationID] = targetID

        if let pending = pendingStatesByConversationID[conversation.conversationID] {
            return pending
        }
        if let checkpoint = checkpoint(for: conversation, targetID: targetID) {
            return Self.state(from: checkpoint, fallbackConversation: conversation)
        }
        if let legacy = try legacyStateLookup?(conversation.conversationID) {
            return legacy
        }
        return Self.blankState(for: conversation, updatedAt: Self.iso8601(now()))
    }

    func save(_ state: WebexConversationSyncStateRecord) async throws {
        pendingStatesByConversationID[state.conversationID] = state
    }

    func pendingCheckpoints() -> [ConnectorCheckpoint] {
        pendingStatesByConversationID.values.flatMap { state in
            Self.checkpointTargetIDs(
                from: state,
                primaryTargetID: targetIDByConversationID[state.conversationID] ?? state.conversationID
            ).map { targetID in
                Self.checkpoint(from: state, accountID: accountID, targetID: targetID)
            }
        }
            .sorted { lhs, rhs in
                if lhs.targetID != rhs.targetID {
                    return lhs.targetID < rhs.targetID
                }
                return lhs.key < rhs.key
            }
    }

    private func checkpoint(
        for conversation: WebexTrackedConversation,
        targetID: String
    ) -> ConnectorCheckpoint? {
        if let exact = checkpoints.checkpoint(
            connectorID: .webex,
            targetID: targetID,
            key: webexSignalSyncStateCheckpointKey
        ) ?? checkpoints.checkpoint(
            connectorID: .webex,
            targetID: conversation.conversationID,
            key: webexSignalSyncStateCheckpointKey
        ) {
            return exact
        }

        let roomID = Self.normalized(conversation.roomID)
        let email = Self.normalized(conversation.personEmail?.lowercased() ?? "")
        return checkpoints.all.last { checkpoint in
            guard checkpoint.connectorID == .webex,
                  checkpoint.key == webexSignalSyncStateCheckpointKey else {
                return false
            }
            let payload = checkpoint.payload
            if Self.nonEmpty(payload["conversationID"]) == conversation.conversationID {
                return true
            }
            if !roomID.isEmpty, Self.nonEmpty(payload["roomID"]) == roomID {
                return true
            }
            if !email.isEmpty, Self.nonEmpty(payload["personEmail"])?.lowercased() == email {
                return true
            }
            return false
        }
    }

    static func checkpointTargetID(for conversation: WebexTrackedConversation) -> String {
        let roomID = normalized(conversation.roomID)
        if !roomID.isEmpty {
            return "roomID:\(roomID)"
        }
        let email = normalized(conversation.personEmail?.lowercased() ?? "")
        if !email.isEmpty {
            return "email:\(email)"
        }
        return normalized(conversation.conversationID)
    }

    static func checkpointTargetIDs(
        from state: WebexConversationSyncStateRecord,
        primaryTargetID: String
    ) -> [String] {
        stableDeduped(
            [
                primaryTargetID,
                targetID(kind: "roomID", value: state.roomID),
                targetID(kind: "email", value: state.personEmail?.lowercased() ?? ""),
                state.conversationID
            ].compactMap { $0 }
        )
    }

    static func checkpoint(
        from state: WebexConversationSyncStateRecord,
        accountID: String,
        targetID: String
    ) -> ConnectorCheckpoint {
        ConnectorCheckpoint(
            connectorID: .webex,
            accountID: accountID,
            targetID: targetID,
            key: webexSignalSyncStateCheckpointKey,
            updatedAt: parseDate(state.updatedAt) ?? Date(),
            payload: [
                "conversationID": state.conversationID,
                "conversationType": state.conversationType.rawValue,
                "roomID": state.roomID,
                "personID": state.personID ?? "",
                "personEmail": state.personEmail ?? "",
                "title": state.title ?? "",
                "lastSeenMessageID": state.lastSeenMessageID ?? "",
                "lastSeenCreated": state.lastSeenCreated ?? "",
                "lastSuccessfulSyncAt": state.lastSuccessfulSyncAt ?? "",
                "nextAllowedSyncAt": state.nextAllowedSyncAt ?? "",
                "pollingMode": state.pollingMode.rawValue,
                "consecutiveFailureCount": String(state.consecutiveFailureCount),
                "lastError": state.lastError ?? "",
                "lastErrorAt": state.lastErrorAt ?? "",
                "updatedAt": state.updatedAt
            ],
            metadata: ["kind": "webex.sync-state"]
        )
    }

    private static func state(
        from checkpoint: ConnectorCheckpoint,
        fallbackConversation: WebexTrackedConversation
    ) -> WebexConversationSyncStateRecord {
        let payload = checkpoint.payload
        return WebexConversationSyncStateRecord(
            conversationID: fallbackConversation.conversationID,
            conversationType: WebexConversationType(rawValue: payload["conversationType"] ?? "") ?? fallbackConversation.conversationType,
            roomID: nonEmpty(payload["roomID"]) ?? fallbackConversation.roomID,
            personID: nonEmpty(payload["personID"]) ?? fallbackConversation.personID,
            personEmail: nonEmpty(payload["personEmail"]) ?? fallbackConversation.personEmail,
            title: nonEmpty(payload["title"]) ?? fallbackConversation.displayName,
            lastSeenMessageID: nonEmpty(payload["lastSeenMessageID"]),
            lastSeenCreated: nonEmpty(payload["lastSeenCreated"]),
            lastSuccessfulSyncAt: nonEmpty(payload["lastSuccessfulSyncAt"]),
            nextAllowedSyncAt: nonEmpty(payload["nextAllowedSyncAt"]),
            pollingMode: WebexPollingMode(rawValue: payload["pollingMode"] ?? "") ?? fallbackConversation.pollingMode,
            consecutiveFailureCount: Int(payload["consecutiveFailureCount"] ?? "") ?? 0,
            lastError: nonEmpty(payload["lastError"]),
            lastErrorAt: nonEmpty(payload["lastErrorAt"]),
            updatedAt: nonEmpty(payload["updatedAt"]) ?? iso8601(checkpoint.updatedAt)
        )
    }

    private static func blankState(
        for conversation: WebexTrackedConversation,
        updatedAt: String
    ) -> WebexConversationSyncStateRecord {
        WebexConversationSyncStateRecord(
            conversationID: conversation.conversationID,
            conversationType: conversation.conversationType,
            roomID: normalized(conversation.roomID),
            personID: nonEmpty(conversation.personID),
            personEmail: nonEmpty(conversation.personEmail?.lowercased()),
            title: nonEmpty(conversation.displayName),
            lastSeenMessageID: nil,
            lastSeenCreated: nil,
            lastSuccessfulSyncAt: nil,
            nextAllowedSyncAt: nil,
            pollingMode: conversation.pollingMode,
            consecutiveFailureCount: 0,
            lastError: nil,
            lastErrorAt: nil,
            updatedAt: updatedAt
        )
    }

    private static func normalized(_ value: String) -> String {
        value.trimmingCharacters(in: .whitespacesAndNewlines)
    }

    private static func targetID(kind: String, value: String) -> String? {
        let normalized = normalized(value)
        guard !normalized.isEmpty else {
            return nil
        }
        return "\(kind):\(normalized)"
    }

    private static func stableDeduped(_ values: [String]) -> [String] {
        var seen: Set<String> = []
        var result: [String] = []
        for value in values {
            let normalized = normalized(value)
            if !normalized.isEmpty, seen.insert(normalized).inserted {
                result.append(normalized)
            }
        }
        return result
    }

    private static func nonEmpty(_ value: String?) -> String? {
        guard let normalized = value?.trimmingCharacters(in: .whitespacesAndNewlines),
              !normalized.isEmpty else {
            return nil
        }
        return normalized
    }

    private static func iso8601(_ date: Date) -> String {
        iso8601WithFractionalSeconds.string(from: date)
    }

    private static func parseDate(_ value: String) -> Date? {
        if let date = iso8601WithFractionalSeconds.date(from: value) {
            return date
        }
        return iso8601.date(from: value)
    }

    private static let iso8601WithFractionalSeconds: ISO8601DateFormatter = {
        let formatter = ISO8601DateFormatter()
        formatter.formatOptions = [.withInternetDateTime, .withFractionalSeconds]
        return formatter
    }()

    private static let iso8601: ISO8601DateFormatter = {
        let formatter = ISO8601DateFormatter()
        formatter.formatOptions = [.withInternetDateTime]
        return formatter
    }()
}
