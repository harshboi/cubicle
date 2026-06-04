import Foundation

/// Stable connector namespace used in IDs, routing, and visibility.
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

/// Declared connector behavior that callers can schedule or surface.
enum ConnectorCapability: String, Hashable, Codable {
    case messages
    case content
    case relations
    case mapRefresh
}

/// Connector identity plus capabilities, independent of a live account.
struct ConnectorDescriptor: Hashable {
    var id: ConnectorID
    var displayName: String
    var capabilities: Set<ConnectorCapability>
}

/// Product-level entity shape a target or object represents.
enum SignalEntityKind: String, Hashable, Codable {
    case person
    case space
    case project
    case repository
    case document
    case unknown
}

/// Connector-specific lookup key type, intentionally open-ended.
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

/// One lookup key for one connector.
struct ConnectorSelector: Hashable {
    var connectorID: ConnectorID
    var kind: ConnectorSelectorKind
    var value: String

    /// Normalizes user/config values before routing uses them.
    init(connectorID: ConnectorID, kind: ConnectorSelectorKind, value: String) {
        self.connectorID = connectorID
        self.kind = kind
        self.value = value.trimmingCharacters(in: .whitespacesAndNewlines)
    }
}

/// User-configured focus target after connector-specific selectors are attached.
struct SignalTarget: Identifiable, Hashable {
    var id: String
    var label: String
    var entityKind: SignalEntityKind
    var selectors: [ConnectorSelector]

    /// Preserves caller-owned identity while accepting precomputed selectors.
    init(id: String, label: String, entityKind: SignalEntityKind, selectors: [ConnectorSelector]) {
        self.id = id
        self.label = label
        self.entityKind = entityKind
        self.selectors = selectors
    }

    /// Translates persisted focus-target config into connector selectors.
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

    /// Returns selectors for a connector, optionally narrowed to one selector kind.
    func selectors(for connectorID: ConnectorID, kind: ConnectorSelectorKind? = nil) -> [ConnectorSelector] {
        selectors.filter { selector in
            selector.connectorID == connectorID && (kind == nil || selector.kind == kind)
        }
    }

    /// Candidate scopes the checkpoint store can load before connector-specific sync.
    func checkpointTargetIDs(for connectorID: ConnectorID) -> [String] {
        stableDedupedStrings(
            [id] + selectors(for: connectorID).map { "\($0.kind.rawValue):\($0.value)" }
        )
    }
}

extension SignalEntityKind {
    /// Maps legacy config kinds into the signal model vocabulary.
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

/// Expands targets into the connectors that have enough selectors to act.
struct TargetRouter {
    var connectorIDs: [ConnectorID]

    /// Groups shared targets by connector without duplicating selector logic in callers.
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

/// Sync scope requested by schedulers or manual refresh.
enum SignalSyncMode: String, Hashable, Codable {
    case full
    case incremental
}

/// Generic sync trigger; connectors may map it to source-specific force rules.
enum SignalSyncTriggerReason: String, Hashable, Codable {
    case startup
    case scheduled
    case manual
    case wakeFromSleep
    case networkReconnect
    case userOpenedTarget
}

/// Connector-agnostic input for one signal sync run.
struct SignalSyncRequest: Hashable {
    var runID: UUID
    var mode: SignalSyncMode
    var trigger: SignalSyncTriggerReason
    var targets: [SignalTarget]
    var startedAt: Date
    var since: Date?
    var limit: Int

    /// Creates a sync request with a generated run ID unless callers provide one.
    init(
        runID: UUID = UUID(),
        mode: SignalSyncMode,
        trigger: SignalSyncTriggerReason = .scheduled,
        targets: [SignalTarget],
        startedAt: Date = Date(),
        since: Date? = nil,
        limit: Int
    ) {
        self.runID = runID
        self.mode = mode
        self.trigger = trigger
        self.targets = targets
        self.startedAt = startedAt
        self.since = since
        self.limit = limit
    }
}

/// Storage scope for one connector cursor/backoff payload.
struct ConnectorCheckpointScope: Hashable {
    var connectorID: ConnectorID
    var targetID: String
    var key: String

    init(connectorID: ConnectorID, targetID: String, key: String) {
        self.connectorID = connectorID
        self.targetID = normalizedCheckpointComponent(targetID)
        self.key = normalizedCheckpointComponent(key)
    }
}

/// Connector-owned cursor/backoff payload for incremental syncs.
struct ConnectorCheckpoint: Hashable {
    var connectorID: ConnectorID
    var accountID: String
    var targetID: String
    var key: String
    var updatedAt: Date
    var payload: [String: String]
    var metadata: [String: String]

    var scope: ConnectorCheckpointScope {
        ConnectorCheckpointScope(connectorID: connectorID, targetID: targetID, key: key)
    }

    init(
        connectorID: ConnectorID,
        accountID: String,
        targetID: String = "default",
        key: String = "default",
        updatedAt: Date,
        payload: [String: String],
        metadata: [String: String] = [:]
    ) {
        self.connectorID = connectorID
        self.accountID = normalizedCheckpointComponent(accountID)
        self.targetID = normalizedCheckpointComponent(targetID)
        self.key = normalizedCheckpointComponent(key)
        self.updatedAt = updatedAt
        self.payload = payload
        self.metadata = metadata
    }
}

/// Loaded checkpoints keyed by connector/target/key for one routed sync call.
struct ConnectorCheckpointSet: Hashable {
    private let checkpoints: [ConnectorCheckpoint]

    init(_ checkpoints: [ConnectorCheckpoint] = []) {
        self.checkpoints = stableDedupedCheckpoints(checkpoints)
    }

    static let empty = ConnectorCheckpointSet()

    var all: [ConnectorCheckpoint] {
        checkpoints
    }

    func checkpoint(connectorID: ConnectorID? = nil, targetID: String, key: String) -> ConnectorCheckpoint? {
        let scope = ConnectorCheckpointScope(connectorID: connectorID ?? .init(rawValue: ""), targetID: targetID, key: key)
        return checkpoints.last { checkpoint in
            (connectorID == nil || checkpoint.connectorID == connectorID)
                && checkpoint.targetID == scope.targetID
                && checkpoint.key == scope.key
        }
    }

    func filtered(connectorID: ConnectorID) -> ConnectorCheckpointSet {
        ConnectorCheckpointSet(checkpoints.filter { $0.connectorID == connectorID })
    }

    func merging(_ other: ConnectorCheckpointSet) -> ConnectorCheckpointSet {
        ConnectorCheckpointSet(checkpoints + other.checkpoints)
    }
}

/// Recoverable connector issue attached to a target when possible.
struct ConnectorWarning: Hashable {
    var connectorID: ConnectorID
    var targetID: String?
    var message: String
}

/// Availability state for one connector batch.
enum ConnectorAvailability: String, Hashable, Codable {
    case available
    case unavailable
    case permissionDenied
    case authRequired
    case rateLimited
    case partial
}

/// Normalized output from one connector account for one request.
struct SignalSyncBatch: Hashable {
    var connectorID: ConnectorID
    var accountID: String
    var objects: [SignalObject]
    var events: [SignalEvent]
    var relations: [SignalRelation]
    var content: [SignalContentChunk]
    var checkpoints: [ConnectorCheckpoint]
    var warnings: [ConnectorWarning]
    var availability: ConnectorAvailability

    var checkpoint: ConnectorCheckpoint? {
        checkpoints.first
    }

    init(
        connectorID: ConnectorID,
        accountID: String,
        objects: [SignalObject],
        events: [SignalEvent],
        relations: [SignalRelation],
        content: [SignalContentChunk],
        checkpoint: ConnectorCheckpoint? = nil,
        checkpoints: [ConnectorCheckpoint]? = nil,
        warnings: [ConnectorWarning],
        availability: ConnectorAvailability
    ) {
        self.connectorID = connectorID
        self.accountID = accountID
        self.objects = objects
        self.events = events
        self.relations = relations
        self.content = content
        self.checkpoints = checkpoints ?? checkpoint.map { [$0] } ?? []
        self.warnings = warnings
        self.availability = availability
    }

    /// Returns an empty batch without forcing each adapter to spell out all arrays.
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

/// Globally unique signal ID after connector/account namespacing.
struct GlobalSignalID: RawRepresentable, Hashable, Codable, ExpressibleByStringLiteral {
    var rawValue: String

    init(rawValue: String) {
        self.rawValue = rawValue
    }

    init(stringLiteral value: String) {
        self.rawValue = value
    }
}

/// Connector-native ID plus enough namespace to become globally stable.
struct SourceSignalID: Hashable, Codable {
    var connectorID: ConnectorID
    var accountID: String
    var kind: String
    var externalID: String

    /// Namespaced ID used for cross-connector persistence and de-duplication.
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

/// Normalized object classes emitted by connectors.
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

/// Normalized event classes emitted by connectors.
enum SignalEventKind: String, Hashable, Codable {
    case message
}

/// Long-lived entity observed from a connector.
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

/// Actor metadata carried by events without requiring a persisted person row.
struct SignalActor: Hashable {
    var id: GlobalSignalID?
    var displayName: String
    var email: String?
}

/// Time-ordered connector event with normalized payload.
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

/// Payload union for event-specific fields.
enum SignalEventPayload: Hashable {
    case message(MessageEventPayload)
}

/// Message-specific event fields shared by chat-like connectors.
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

/// Relationship inferred or observed from connector data.
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

/// Confidence bucket for inferred relationships.
enum SignalConfidence: String, Hashable, Codable {
    case direct
    case inferredHigh
    case inferredMedium
    case inferredLow
}

/// Text payload slice associated with a signal object.
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

/// Access scope attached to objects, events, relations, and content chunks.
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

    /// Visibility for connector accounts authenticated in the current app session.
    static func authenticatedUser(connectorID: ConnectorID, accountID: String) -> SignalVisibility {
        SignalVisibility(
            source: connectorID,
            accountID: accountID,
            scope: "\(connectorID.rawValue)-authenticated-user",
            principals: []
        )
    }
}

/// Typed scalar bag for connector-specific properties.
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

private func normalizedCheckpointComponent(_ value: String) -> String {
    let trimmed = value.trimmingCharacters(in: .whitespacesAndNewlines)
    return trimmed.isEmpty ? "default" : trimmed
}

private func stableDedupedStrings(_ values: [String]) -> [String] {
    var seen: Set<String> = []
    var result: [String] = []
    for value in values {
        let normalized = normalizedCheckpointComponent(value)
        if seen.insert(normalized).inserted {
            result.append(normalized)
        }
    }
    return result
}

private func stableDedupedCheckpoints(_ checkpoints: [ConnectorCheckpoint]) -> [ConnectorCheckpoint] {
    var indexesByScope: [ConnectorCheckpointScope: Int] = [:]
    var result: [ConnectorCheckpoint] = []
    for checkpoint in checkpoints {
        if let index = indexesByScope[checkpoint.scope] {
            result[index] = checkpoint
        } else {
            indexesByScope[checkpoint.scope] = result.count
            result.append(checkpoint)
        }
    }
    return result.sorted(by: checkpointSort)
}

private func checkpointSort(_ lhs: ConnectorCheckpoint, _ rhs: ConnectorCheckpoint) -> Bool {
    if lhs.connectorID.rawValue != rhs.connectorID.rawValue {
        return lhs.connectorID.rawValue < rhs.connectorID.rawValue
    }
    if lhs.targetID != rhs.targetID {
        return lhs.targetID < rhs.targetID
    }
    return lhs.key < rhs.key
}
