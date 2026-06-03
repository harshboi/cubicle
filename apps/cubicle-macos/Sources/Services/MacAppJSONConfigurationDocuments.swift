import Foundation

/// File names and document types for optional macOS JSON configuration.
enum MacAppJSONConfigurationFiles {
    static let runtime = "runtime.json"
    static let targets = "targets.json"
    static let codex = "codex.json"
    static let questionGeneration = "question-generation.json"
}

/// Runtime, Webex, Codex executable, and local source tuning.
struct MacAppJSONRuntimeDocument: Codable, Equatable {
    var version: Int?
    var codex: MacAppJSONRuntimeCodexSettings?
    var webex: MacAppJSONWebexRuntimeSettings?
    var imessage: MacAppJSONIMessageRuntimeSettings?
}

/// Runtime Codex executable choices.
struct MacAppJSONRuntimeCodexSettings: Codable, Equatable {
    var executable: String?
}

/// Runtime Webex API/OAuth/sync tuning.
struct MacAppJSONWebexRuntimeSettings: Codable, Equatable {
    var apiBaseURL: String?
    var pageSize: Int?
    var retryCount: Int?
    var timeoutSeconds: Double?
    var oauthTokenFile: String?
    var oauthRefreshSkewSeconds: Double?
    var oauthRefreshTokenSkewSeconds: Double?
    var publicWebhookURL: String?
    var sync: MacAppJSONWebexSyncSettings?

    enum CodingKeys: String, CodingKey {
        case apiBaseURL = "api_base_url"
        case pageSize = "page_size"
        case retryCount = "retry_count"
        case timeoutSeconds = "timeout_seconds"
        case oauthTokenFile = "oauth_token_file"
        case oauthRefreshSkewSeconds = "oauth_refresh_skew_seconds"
        case oauthRefreshTokenSkewSeconds = "oauth_refresh_token_skew_seconds"
        case publicWebhookURL = "public_webhook_url"
        case sync
    }
}

/// Webex sync scheduler/concurrency tuning.
struct MacAppJSONWebexSyncSettings: Codable, Equatable {
    var concurrencyLimit: Int?
    var adaptiveActiveIntervalSeconds: Double?
    var adaptiveRecentIntervalSeconds: Double?
    var adaptiveBackgroundIntervalSeconds: Double?
    var adaptiveJitterPercent: Int?

    enum CodingKeys: String, CodingKey {
        case concurrencyLimit = "concurrency_limit"
        case adaptiveActiveIntervalSeconds = "adaptive_active_interval_seconds"
        case adaptiveRecentIntervalSeconds = "adaptive_recent_interval_seconds"
        case adaptiveBackgroundIntervalSeconds = "adaptive_background_interval_seconds"
        case adaptiveJitterPercent = "adaptive_jitter_percent"
    }
}

/// Local iMessage source tuning.
struct MacAppJSONIMessageRuntimeSettings: Codable, Equatable {
    var chatDatabasePath: String?
    var busyTimeoutMilliseconds: Int?

    enum CodingKeys: String, CodingKey {
        case chatDatabasePath = "chat_database_path"
        case busyTimeoutMilliseconds = "busy_timeout_milliseconds"
    }
}

/// Operator-controlled focus target groups.
struct MacAppJSONTargetsDocument: Codable, Equatable {
    var version: Int?
    var groups: MacAppJSONTargetGroups?
}

/// Configured focus, executive, and belief target groups.
struct MacAppJSONTargetGroups: Codable, Equatable {
    var important: [MacAppJSONTargetDocument]?
    var executives: [MacAppJSONTargetDocument]?
    var beliefs: [MacAppJSONTargetDocument]?
}

/// JSON representation of one person or space target.
struct MacAppJSONTargetDocument: Codable, Equatable {
    var kind: String?
    var label: String?
    var roomID: String?
    var roomType: String?
    var email: String?
    var autoReply: Bool?
    var iMessageHandles: [String]?

    enum CodingKeys: String, CodingKey {
        case kind
        case label
        case roomID = "room_id"
        case roomType = "room_type"
        case email
        case autoReply = "auto_reply"
        case iMessageHandles = "imessage_handles"
    }
}

/// Codex execution, cache, belief, and synthesis tuning.
struct MacAppJSONCodexDocument: Codable, Equatable {
    var version: Int?
    var runPolicy: MacAppJSONCodexRunPolicy?
    var cachePolicy: MacAppJSONCodexCachePolicy?
    var beliefs: MacAppJSONCodexBeliefPolicy?
    var questionSynthesis: MacAppJSONQuestionSynthesisPolicy?

    enum CodingKeys: String, CodingKey {
        case version
        case runPolicy = "run_policy"
        case cachePolicy = "cache_policy"
        case beliefs
        case questionSynthesis = "question_synthesis"
    }
}

/// Retry and timeout tuning for local Codex processes.
struct MacAppJSONCodexRunPolicy: Codable, Equatable {
    var timeoutSeconds: Double?
    var maxAttempts: Int?
    var retryDelaySeconds: Double?

    enum CodingKeys: String, CodingKey {
        case timeoutSeconds = "timeout_seconds"
        case maxAttempts = "max_attempts"
        case retryDelaySeconds = "retry_delay_seconds"
    }
}

/// Cache freshness tuning for Codex prompt outputs.
struct MacAppJSONCodexCachePolicy: Codable, Equatable {
    var summaryMaxAgeSeconds: Double?
    var execQuestionsMaxAgeSeconds: Double?

    enum CodingKeys: String, CodingKey {
        case summaryMaxAgeSeconds = "summary_max_age_seconds"
        case execQuestionsMaxAgeSeconds = "exec_questions_max_age_seconds"
    }
}

/// Belief reconciliation scheduling and chunking tuning.
struct MacAppJSONCodexBeliefPolicy: Codable, Equatable {
    var staleHours: Int?
    var evidenceChunkSize: Int?
    var maxIncrementalWindowDays: Int?

    enum CodingKeys: String, CodingKey {
        case staleHours = "stale_hours"
        case evidenceChunkSize = "evidence_chunk_size"
        case maxIncrementalWindowDays = "max_incremental_window_days"
    }
}

/// Question-synthesis prompt input sizing.
struct MacAppJSONQuestionSynthesisPolicy: Codable, Equatable {
    var seedCandidateLimit: Int?
    var queryHistoryLimit: Int?
    var promptHistoryLimit: Int?
    var candidateEvidenceLimit: Int?
    var outputLimit: Int?

    enum CodingKeys: String, CodingKey {
        case seedCandidateLimit = "seed_candidate_limit"
        case queryHistoryLimit = "query_history_limit"
        case promptHistoryLimit = "prompt_history_limit"
        case candidateEvidenceLimit = "candidate_evidence_limit"
        case outputLimit = "output_limit"
    }
}

/// Local question generation and filtering tuning.
struct MacAppJSONQuestionGenerationDocument: Codable, Equatable {
    var version: Int?
    var core: MacAppJSONQuestionGenerationCoreSettings?
    var cubicle: MacAppJSONQuestionGenerationCubicleSettings?
}

/// WebexQuestionGeneratorCore configuration overrides.
struct MacAppJSONQuestionGenerationCoreSettings: Codable, Equatable {
    var privacy: MacAppJSONQuestionGenerationPrivacySettings?
    var topics: MacAppJSONQuestionGenerationTopicSettings?
    var questions: MacAppJSONQuestionGenerationQuestionSettings?
}

/// Local analytics privacy settings.
struct MacAppJSONQuestionGenerationPrivacySettings: Codable, Equatable {
    var anonymizeUsers: Bool?
    var redactURLs: Bool?
    var redactEmails: Bool?

    enum CodingKeys: String, CodingKey {
        case anonymizeUsers = "anonymize_users"
        case redactURLs = "redact_urls"
        case redactEmails = "redact_emails"
    }
}

/// Local topic extraction settings.
struct MacAppJSONQuestionGenerationTopicSettings: Codable, Equatable {
    var enabled: Bool?
    var numberOfTopics: Int?
    var minimumTopicSize: Int?

    enum CodingKeys: String, CodingKey {
        case enabled
        case numberOfTopics = "number_of_topics"
        case minimumTopicSize = "minimum_topic_size"
    }
}

/// Local question extraction settings.
struct MacAppJSONQuestionGenerationQuestionSettings: Codable, Equatable {
    var topN: Int?
    var enabledCategories: [String]?

    enum CodingKeys: String, CodingKey {
        case topN = "top_n"
        case enabledCategories = "enabled_categories"
    }
}

/// Cubicle-specific fallback and publication thresholds.
struct MacAppJSONQuestionGenerationCubicleSettings: Codable, Equatable {
    var fallbackDraftLimit: Int?
    var generatedQuestionLimit: Int?
    var publishableQuestionLimit: Int?
    var evidenceLimit: Int?
    var coreEvidenceLimit: Int?

    enum CodingKeys: String, CodingKey {
        case fallbackDraftLimit = "fallback_draft_limit"
        case generatedQuestionLimit = "generated_question_limit"
        case publishableQuestionLimit = "publishable_question_limit"
        case evidenceLimit = "evidence_limit"
        case coreEvidenceLimit = "core_evidence_limit"
    }
}
