import Foundation

struct ConnectorID: RawRepresentable, Hashable, Codable, ExpressibleByStringLiteral {
    var rawValue: String

    init(rawValue: String) {
        self.rawValue = rawValue
    }

    init(stringLiteral value: String) {
        self.rawValue = value
    }

    static let webex = ConnectorID(rawValue: "webex")
    static let iMessage = ConnectorID(rawValue: "imessage")
}

enum ConnectorCapability: String, Hashable, Codable {
    case messages
    case content
    case relations
    case mapRefresh
}

struct ConnectorDescriptor: Hashable {
    var id: ConnectorID
    var displayName: String
    var capabilities: Set<ConnectorCapability>
}

enum SignalEntityKind: String, Hashable, Codable {
    case person
    case space
    case project
    case repository
    case document
    case unknown
}

struct ConnectorSelectorKind: RawRepresentable, Hashable, Codable, ExpressibleByStringLiteral {
    var rawValue: String

    init(rawValue: String) {
        self.rawValue = rawValue
    }

    init(stringLiteral value: String) {
        self.rawValue = value
    }

    static let roomID = ConnectorSelectorKind(rawValue: "roomID")
    static let email = ConnectorSelectorKind(rawValue: "email")
    static let handle = ConnectorSelectorKind(rawValue: "handle")
    static let channelID = ConnectorSelectorKind(rawValue: "channelID")
    static let userID = ConnectorSelectorKind(rawValue: "userID")
    static let documentID = ConnectorSelectorKind(rawValue: "documentID")
    static let driveFileID = ConnectorSelectorKind(rawValue: "driveFileID")
    static let issueKey = ConnectorSelectorKind(rawValue: "issueKey")
    static let projectKey = ConnectorSelectorKind(rawValue: "projectKey")
    static let repository = ConnectorSelectorKind(rawValue: "repository")
}

struct ConnectorSelector: Hashable {
    var connectorID: ConnectorID
    var kind: ConnectorSelectorKind
    var value: String

    init(connectorID: ConnectorID, kind: ConnectorSelectorKind, value: String) {
        self.connectorID = connectorID
        self.kind = kind
        self.value = value.trimmingCharacters(in: .whitespacesAndNewlines)
    }
}

struct SignalTarget: Identifiable, Hashable {
    var id: String
    var label: String
    var entityKind: SignalEntityKind
    var selectors: [ConnectorSelector]

    init(id: String, label: String, entityKind: SignalEntityKind, selectors: [ConnectorSelector]) {
        self.id = id
        self.label = label
        self.entityKind = entityKind
        self.selectors = selectors
    }

    init(configTarget: ConfigTarget) {
        var selectors: [ConnectorSelector] = []
        let roomID = configTarget.roomID.trimmingCharacters(in: .whitespacesAndNewlines)
        if !roomID.isEmpty {
            selectors.append(ConnectorSelector(connectorID: .webex, kind: .roomID, value: roomID))
        }

        // Roomless person targets still need Webex email lookup; room-backed
        // targets keep it only as identity metadata.
        let email = configTarget.email.trimmingCharacters(in: .whitespacesAndNewlines).lowercased()
        if !email.isEmpty {
            selectors.append(ConnectorSelector(connectorID: .webex, kind: .email, value: email))
        }

        for handle in configTarget.iMessageHandles {
            if let normalized = IMessageHandleNormalizer.normalizedStorageValue(handle) {
                selectors.append(ConnectorSelector(connectorID: .iMessage, kind: .handle, value: normalized))
            }
        }

        self.init(
            id: configTarget.id,
            label: configTarget.label,
            entityKind: SignalEntityKind(configTargetKind: configTarget.kind),
            selectors: stableDedupedSelectors(selectors)
        )
    }

    func selectors(for connectorID: ConnectorID, kind: ConnectorSelectorKind? = nil) -> [ConnectorSelector] {
        selectors.filter { selector in
            selector.connectorID == connectorID && (kind == nil || selector.kind == kind)
        }
    }
}

extension SignalEntityKind {
    init(configTargetKind: ConfigTarget.Kind) {
        switch configTargetKind {
        case .person:
            self = .person
        case .space:
            self = .space
        case .unknown:
            self = .unknown
        }
    }
}

struct TargetRouter {
    var connectorIDs: [ConnectorID]

    func targetsByConnector(_ targets: [SignalTarget]) -> [ConnectorID: [SignalTarget]] {
        var routed = Dictionary(uniqueKeysWithValues: connectorIDs.map { ($0, [SignalTarget]()) })
        for target in targets {
            // A shared person target can route to Webex and iMessage; an absent
            // selector is the opt-out signal for that connector.
            for connectorID in connectorIDs where !target.selectors(for: connectorID).isEmpty {
                routed[connectorID, default: []].append(target)
            }
        }
        return routed
    }
}

enum SignalSyncMode: String, Hashable, Codable {
    case full
    case incremental
}

struct SignalSyncRequest: Hashable {
    var runID: UUID
    var mode: SignalSyncMode
    var targets: [SignalTarget]
    var startedAt: Date
    var since: Date?
    var limit: Int

    init(
        runID: UUID = UUID(),
        mode: SignalSyncMode,
        targets: [SignalTarget],
        startedAt: Date = Date(),
        since: Date? = nil,
        limit: Int
    ) {
        self.runID = runID
        self.mode = mode
        self.targets = targets
        self.startedAt = startedAt
        self.since = since
        self.limit = limit
    }
}

struct ConnectorCheckpoint: Hashable {
    var connectorID: ConnectorID
    var accountID: String
    var updatedAt: Date
    var payload: [String: String]
}

struct ConnectorWarning: Hashable {
    var connectorID: ConnectorID
    var targetID: String?
    var message: String
}

enum ConnectorAvailability: String, Hashable, Codable {
    case available
    case unavailable
    case permissionDenied
    case authRequired
    case rateLimited
    case partial
}

struct SignalSyncBatch: Hashable {
    var connectorID: ConnectorID
    var accountID: String
    var objects: [SignalObject]
    var events: [SignalEvent]
    var relations: [SignalRelation]
    var content: [SignalContentChunk]
    var checkpoint: ConnectorCheckpoint?
    var warnings: [ConnectorWarning]
    var availability: ConnectorAvailability

    static func empty(
        connectorID: ConnectorID,
        accountID: String,
        availability: ConnectorAvailability = .available,
        warnings: [ConnectorWarning] = []
    ) -> SignalSyncBatch {
        SignalSyncBatch(
            connectorID: connectorID,
            accountID: accountID,
            objects: [],
            events: [],
            relations: [],
            content: [],
            checkpoint: nil,
            warnings: warnings,
            availability: availability
        )
    }
}

struct GlobalSignalID: RawRepresentable, Hashable, Codable, ExpressibleByStringLiteral {
    var rawValue: String

    init(rawValue: String) {
        self.rawValue = rawValue
    }

    init(stringLiteral value: String) {
        self.rawValue = value
    }
}

struct SourceSignalID: Hashable, Codable {
    var connectorID: ConnectorID
    var accountID: String
    var kind: String
    var externalID: String

    var globalID: GlobalSignalID {
        GlobalSignalID(rawValue: [
            connectorID.rawValue,
            accountID,
            kind,
            externalID
        ].map(Self.stableComponent).joined(separator: ":"))
    }

    private static func stableComponent(_ value: String) -> String {
        // Global IDs use ':' as their delimiter; source IDs can contain it
        // already (for example synthetic direct-message thread IDs).
        value
            .trimmingCharacters(in: .whitespacesAndNewlines)
            .replacingOccurrences(of: ":", with: "%3A")
    }
}

typealias SourceObjectID = SourceSignalID
typealias SourceEventID = SourceSignalID

enum SignalObjectKind: String, Hashable, Codable {
    case person
    case space
    case thread
    case channel
    case document
    case file
    case issue
    case pullRequest
    case project
}

enum SignalEventKind: String, Hashable, Codable {
    case message
}

struct SignalObject: Hashable {
    var id: GlobalSignalID
    var sourceID: SourceObjectID
    var kind: SignalObjectKind
    var title: String
    var url: URL?
    var createdAt: Date?
    var updatedAt: Date?
    var visibility: SignalVisibility
    var properties: SignalProperties
}

struct SignalActor: Hashable {
    var id: GlobalSignalID?
    var displayName: String
    var email: String?
}

struct SignalEvent: Hashable {
    var id: GlobalSignalID
    var sourceID: SourceEventID
    var kind: SignalEventKind
    var actor: SignalActor?
    var occurredAt: Date
    var objectIDs: [GlobalSignalID]
    var visibility: SignalVisibility
    var payload: SignalEventPayload
}

enum SignalEventPayload: Hashable {
    case message(MessageEventPayload)
}

struct MessageEventPayload: Hashable {
    var threadID: GlobalSignalID
    var threadSourceID: SourceObjectID
    var threadTitle: String
    var senderID: GlobalSignalID?
    var senderDisplayName: String
    var senderEmail: String?
    var body: String
    var isFromCurrentUser: Bool
}

struct SignalRelation: Hashable {
    var id: GlobalSignalID
    var kind: String
    var fromID: GlobalSignalID
    var toID: GlobalSignalID
    var observedAt: Date
    var sourceEventID: GlobalSignalID?
    var confidence: SignalConfidence
    var visibility: SignalVisibility
}

enum SignalConfidence: String, Hashable, Codable {
    case direct
    case inferredHigh
    case inferredMedium
    case inferredLow
}

struct SignalContentChunk: Hashable {
    var id: GlobalSignalID
    var objectID: GlobalSignalID
    var sourceID: SourceObjectID
    var text: String
    var chunkIndex: Int
    var contentHash: String
    var observedAt: Date
    var visibility: SignalVisibility
}

struct SignalVisibility: Hashable {
    var source: ConnectorID
    var accountID: String
    var scope: String
    var principals: [String]

    // Local Messages rows inherit macOS-user visibility, not any Webex/OAuth
    // account scope that may be active in the same app session.
    static let localUserOnly = SignalVisibility(
        source: .iMessage,
        accountID: "local",
        scope: "local-user-only",
        principals: []
    )

    static func authenticatedUser(connectorID: ConnectorID, accountID: String) -> SignalVisibility {
        SignalVisibility(
            source: connectorID,
            accountID: accountID,
            scope: "\(connectorID.rawValue)-authenticated-user",
            principals: []
        )
    }
}

enum SignalPropertyValue: Hashable {
    case string(String)
    case number(Double)
    case bool(Bool)
}

typealias SignalProperties = [String: SignalPropertyValue]

private func stableDedupedSelectors(_ selectors: [ConnectorSelector]) -> [ConnectorSelector] {
    var seen: Set<ConnectorSelector> = []
    var result: [ConnectorSelector] = []
    for selector in selectors where !selector.value.isEmpty {
        if seen.insert(selector).inserted {
            result.append(selector)
        }
    }
    return result
}
