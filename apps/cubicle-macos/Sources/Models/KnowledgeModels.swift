import Foundation

/// Stored cluster summary for a focus target/topic pair.
struct FocusClusterRecord: Identifiable, Hashable {
    var id: String
    var focusKind: String
    var scope: String
    var entityKey: String
    var topicKey: String
    var title: String
    var summary: String
    var promptVersion: String
    var sourceHash: String
    var generatedAt: String
    var updatedAt: String
}

/// Stored topic insight generated from focus evidence.
struct TopicRecord: Identifiable, Hashable {
    var id: String
    var focusKind: String
    var scope: String
    var entityKey: String
    var topicKey: String
    var title: String
    var summary: String
    var soWhat: String
    var sourceLabel: String
    var score: Double?
    var generatedAt: String
    var updatedAt: String
}

/// Belief row, including both manual assertions and generated candidates.
struct BeliefRecord: Identifiable, Hashable {
    var id: String
    var scope: String
    var entityKey: String
    var statement: String
    var confidence: Double
    var updatedAt: String
    var isManual: Bool
    var evidenceLinks: [String]
    var createdAt: String
    var beliefKind: String = "second_order"
    var lifecycle: String = "candidate"
    var supportCount: Int = 1
    var contradictionCount: Int = 0
    var lastEvidenceAt: String = ""
}

/// Rollup counts for one belief scope/entity in the UI.
struct BeliefSetSummary: Identifiable, Hashable {
    var scope: KnowledgeBeliefScope
    var entityKey: String
    var title: String
    var autoCount: Int
    var manualCount: Int
    var updatedAt: String

    /// Stable UI key across manual/automatic belief refreshes.
    var id: String {
        "\(scope.rawValue):\(entityKey)"
    }
}

/// Scope partition used by beliefs and reconciliation state.
enum KnowledgeBeliefScope: String, CaseIterable, Codable, Hashable {
    case global
    case person
    case space

    static let globalEntityKey = "__global__"
}

/// Conversation/space row projected into the knowledge store.
struct RoomRecord: Identifiable, Hashable {
    var id: String
    var title: String
    var updatedAt: String
}

/// Person row projected into the knowledge store.
struct PersonRecord: Identifiable, Hashable {
    var id: String
    var displayName: String
    var email: String
    var updatedAt: String
}

/// Message row projected into the knowledge store.
struct MessageRecord: Identifiable, Hashable {
    var id: String
    var roomID: String
    var personID: String?
    var body: String
    var createdAt: String
    var updatedAt: String
}

/// Webex conversation category used by sync state and polling policy.
enum WebexConversationType: String, CaseIterable, Codable, Hashable {
    case space
    case direct
}

/// Polling cadence bucket for tracked Webex conversations.
enum WebexPollingMode: String, CaseIterable, Codable, Hashable {
    case active
    case recent
    case background
    case paused
    case disabled
}

/// Persisted cursor/backoff state for one Webex conversation.
struct WebexConversationSyncStateRecord: Hashable {
    var conversationID: String
    var conversationType: WebexConversationType
    var roomID: String
    var personID: String?
    var personEmail: String?
    var title: String?
    var lastSeenMessageID: String?
    var lastSeenCreated: String?
    var lastSuccessfulSyncAt: String?
    var nextAllowedSyncAt: String?
    var pollingMode: WebexPollingMode
    var consecutiveFailureCount: Int
    var lastError: String?
    var lastErrorAt: String?
    var updatedAt: String
}

/// File attachment row tied to a room/message.
struct FileRecord: Identifiable, Hashable {
    var id: String
    var messageID: String?
    var roomID: String
    var filename: String
    var mimeType: String
    var fileSize: Int
    var updatedAt: String
}

/// Text evidence row consumed by belief and question synthesis.
struct BeliefEvidenceRecord: Identifiable, Hashable {
    var id: String
    var source: String
    var sourceID: String
    var roomID: String
    var personID: String?
    var occurredAt: String
    var text: String
}

/// Last reconciliation checkpoint for a belief scope/entity.
struct BeliefReconciliationStateRecord: Hashable {
    var scope: String
    var entityKey: String
    var lastRunAt: String?
    var lastEvidenceHash: String?
    var updatedAt: String
}

/// Scope a generated question is about.
enum QuestionScopeType: String, CaseIterable, Codable, Hashable, Identifiable {
    case person
    case space

    var id: String { rawValue }

    /// Display label used in question lists and filters.
    var title: String {
        switch self {
        case .person: return "Person"
        case .space: return "Space"
        }
    }
}

/// Lifecycle state for a generated question candidate.
enum QuestionStatus: String, CaseIterable, Codable, Hashable, Identifiable {
    case candidate
    case surfaced
    case answered
    case snoozed
    case dismissed

    var id: String { rawValue }

    /// Display label used in status controls.
    var title: String {
        switch self {
        case .candidate: return "Candidate"
        case .surfaced: return "Surfaced"
        case .answered: return "Answered"
        case .snoozed: return "Snoozed"
        case .dismissed: return "Dismissed"
        }
    }
}

/// Evidence pointer shown under a generated question.
struct QuestionEvidenceRef: Identifiable, Codable, Hashable {
    var sourceType: String
    var sourceID: String
    var createdAt: Date?
    var label: String
    var preview: String

    /// Stable key for evidence arrays that may omit createdAt.
    var id: String {
        [
            sourceType,
            sourceID,
            createdAt.map { String(Int($0.timeIntervalSince1970)) } ?? "undated"
        ].joined(separator: ":")
    }
}

/// Generated question plus ranking, provenance, and lifecycle fields.
struct QuestionCandidate: Identifiable, Codable, Hashable {
    var id: String
    var scopeType: QuestionScopeType
    var scopeKey: String
    var scopeLabel: String
    var questionText: String
    var questionType: String
    var whyNow: String
    var evidence: [QuestionEvidenceRef]
    var sourceKind: String
    var sourceKey: String
    var tags: [String]
    var priorityScore: Double
    var status: QuestionStatus
    var answerSnapshotId: String?
    var createdAt: Date
    var updatedAt: Date
    var expiresAt: Date?
}
