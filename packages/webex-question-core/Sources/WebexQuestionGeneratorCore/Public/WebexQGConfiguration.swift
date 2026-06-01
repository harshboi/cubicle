import Foundation

/// Privacy controls applied before analysis and question generation.
public struct PrivacyConfiguration: Codable, Sendable, Equatable {
    public var anonymizeUsers: Bool
    public var redactURLs: Bool
    public var redactEmails: Bool

    public init(anonymizeUsers: Bool, redactURLs: Bool, redactEmails: Bool) {
        self.anonymizeUsers = anonymizeUsers
        self.redactURLs = redactURLs
        self.redactEmails = redactEmails
    }
}

/// Controls fallback thread reconstruction when explicit thread IDs are unavailable.
public struct ThreadingConfiguration: Codable, Sendable, Equatable {
    public var fallbackWindowMinutes: Int

    public init(fallbackWindowMinutes: Int) {
        self.fallbackWindowMinutes = fallbackWindowMinutes
    }
}

/// Controls local topic labeling.
public struct TopicConfiguration: Codable, Sendable, Equatable {
    public var enabled: Bool
    public var numberOfTopics: Int
    public var minimumTopicSize: Int

    public init(enabled: Bool, numberOfTopics: Int, minimumTopicSize: Int) {
        self.enabled = enabled
        self.numberOfTopics = numberOfTopics
        self.minimumTopicSize = minimumTopicSize
    }
}

/// Controls deterministic question generation.
public struct QuestionConfiguration: Codable, Sendable, Equatable {
    public var topN: Int
    public var enabledCategories: Set<QuestionCategory>

    public init(topN: Int, enabledCategories: Set<QuestionCategory>) {
        self.topN = topN
        self.enabledCategories = enabledCategories
    }
}

/// Weights used by QuestionRanker to compute finalScore.
public struct ScoringConfiguration: Codable, Sendable, Equatable {
    public var interestingnessWeight: Double
    public var actionabilityWeight: Double
    public var confidenceWeight: Double

    public init(interestingnessWeight: Double, actionabilityWeight: Double, confidenceWeight: Double) {
        self.interestingnessWeight = interestingnessWeight
        self.actionabilityWeight = actionabilityWeight
        self.confidenceWeight = confidenceWeight
    }
}

/// Top-level configuration for the local Webex question-generation pipeline.
public struct WebexQGConfiguration: Codable, Sendable, Equatable {
    public var privacy: PrivacyConfiguration
    public var threading: ThreadingConfiguration
    public var topics: TopicConfiguration
    public var questions: QuestionConfiguration
    public var scoring: ScoringConfiguration
    public var objectives: [String]

    public init(
        privacy: PrivacyConfiguration,
        threading: ThreadingConfiguration,
        topics: TopicConfiguration,
        questions: QuestionConfiguration,
        scoring: ScoringConfiguration,
        objectives: [String]
    ) {
        self.privacy = privacy
        self.threading = threading
        self.topics = topics
        self.questions = questions
        self.scoring = scoring
        self.objectives = objectives
    }

    public static let `default` = WebexQGConfiguration(
        privacy: PrivacyConfiguration(anonymizeUsers: true, redactURLs: true, redactEmails: true),
        threading: ThreadingConfiguration(fallbackWindowMinutes: 30),
        topics: TopicConfiguration(enabled: true, numberOfTopics: 12, minimumTopicSize: 10),
        questions: QuestionConfiguration(topN: 50, enabledCategories: Set(QuestionCategory.allCases)),
        scoring: ScoringConfiguration(
            interestingnessWeight: 0.45,
            actionabilityWeight: 0.35,
            confidenceWeight: 0.20
        ),
        objectives: [
            "improve team productivity",
            "identify communication bottlenecks",
            "find unresolved questions",
            "understand topic-level friction"
        ]
    )
}
