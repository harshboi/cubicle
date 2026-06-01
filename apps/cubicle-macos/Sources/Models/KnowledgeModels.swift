import Foundation

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

struct BeliefSetSummary: Identifiable, Hashable {
    var scope: KnowledgeBeliefScope
    var entityKey: String
    var title: String
    var autoCount: Int
    var manualCount: Int
    var updatedAt: String

    var id: String {
        "\(scope.rawValue):\(entityKey)"
    }
}

enum KnowledgeBeliefScope: String, CaseIterable, Codable, Hashable {
    case global
    case person
    case space

    static let globalEntityKey = "__global__"
}

struct RoomRecord: Identifiable, Hashable {
    var id: String
    var title: String
    var updatedAt: String
}

struct PersonRecord: Identifiable, Hashable {
    var id: String
    var displayName: String
    var email: String
    var updatedAt: String
}

struct MessageRecord: Identifiable, Hashable {
    var id: String
    var roomID: String
    var personID: String?
    var body: String
    var createdAt: String
    var updatedAt: String
}

enum WebexConversationType: String, CaseIterable, Codable, Hashable {
    case space
    case direct
}

enum WebexPollingMode: String, CaseIterable, Codable, Hashable {
    case active
    case recent
    case background
    case paused
    case disabled
}

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

struct FileRecord: Identifiable, Hashable {
    var id: String
    var messageID: String?
    var roomID: String
    var filename: String
    var mimeType: String
    var fileSize: Int
    var updatedAt: String
}

struct BeliefEvidenceRecord: Identifiable, Hashable {
    var id: String
    var source: String
    var sourceID: String
    var roomID: String
    var personID: String?
    var occurredAt: String
    var text: String
}

struct BeliefReconciliationStateRecord: Hashable {
    var scope: String
    var entityKey: String
    var lastRunAt: String?
    var lastEvidenceHash: String?
    var updatedAt: String
}

enum QuestionScopeType: String, CaseIterable, Codable, Hashable, Identifiable {
    case person
    case space

    var id: String { rawValue }

    var title: String {
        switch self {
        case .person: return "Person"
        case .space: return "Space"
        }
    }
}

enum QuestionStatus: String, CaseIterable, Codable, Hashable, Identifiable {
    case candidate
    case surfaced
    case answered
    case snoozed
    case dismissed

    var id: String { rawValue }

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

struct QuestionEvidenceRef: Identifiable, Codable, Hashable {
    var sourceType: String
    var sourceID: String
    var createdAt: Date?
    var label: String
    var preview: String

    var id: String {
        [
            sourceType,
            sourceID,
            createdAt.map { String(Int($0.timeIntervalSince1970)) } ?? "undated"
        ].joined(separator: ":")
    }
}

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
