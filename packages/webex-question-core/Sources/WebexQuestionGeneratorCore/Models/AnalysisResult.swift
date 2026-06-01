import Foundation

public struct DatasetSummary: Codable, Sendable, Hashable {
    public var messageCount: Int
    public var threadCount: Int
    public var participantCount: Int
    public var spaceCount: Int
    public var startDate: Date?
    public var endDate: Date?
    public var durationDays: Double
    public var dataCompletenessScore: Double

    public init(messageCount: Int, threadCount: Int, participantCount: Int, spaceCount: Int, startDate: Date?, endDate: Date?, durationDays: Double, dataCompletenessScore: Double) {
        self.messageCount = messageCount
        self.threadCount = threadCount
        self.participantCount = participantCount
        self.spaceCount = spaceCount
        self.startDate = startDate
        self.endDate = endDate
        self.durationDays = durationDays
        self.dataCompletenessScore = dataCompletenessScore
    }
}

public struct TopicMetric: Identifiable, Codable, Sendable, Hashable {
    public var id: String { topic }
    public var topic: String
    public var count: Int
    public var averageResponseTimeSeconds: Double?
    public var unansweredQuestionCount: Int
    public var negativeSentimentShare: Double

    public init(topic: String, count: Int, averageResponseTimeSeconds: Double? = nil, unansweredQuestionCount: Int = 0, negativeSentimentShare: Double = 0) {
        self.topic = topic
        self.count = count
        self.averageResponseTimeSeconds = averageResponseTimeSeconds
        self.unansweredQuestionCount = unansweredQuestionCount
        self.negativeSentimentShare = negativeSentimentShare
    }
}

public struct UserMetric: Identifiable, Codable, Sendable, Hashable {
    public var id: String { user }
    public var user: String
    public var messageCount: Int
    public var questionCount: Int
    public var mentionCount: Int
    public var averageResponseTimeSeconds: Double?

    public init(user: String, messageCount: Int, questionCount: Int = 0, mentionCount: Int = 0, averageResponseTimeSeconds: Double? = nil) {
        self.user = user
        self.messageCount = messageCount
        self.questionCount = questionCount
        self.mentionCount = mentionCount
        self.averageResponseTimeSeconds = averageResponseTimeSeconds
    }
}

public struct SpaceMetric: Identifiable, Codable, Sendable, Hashable {
    public var id: String { space }
    public var space: String
    public var messageCount: Int
    public var threadCount: Int
    public var participantCount: Int

    public init(space: String, messageCount: Int, threadCount: Int, participantCount: Int) {
        self.space = space
        self.messageCount = messageCount
        self.threadCount = threadCount
        self.participantCount = participantCount
    }
}

public struct OutlierThread: Identifiable, Codable, Sendable, Hashable {
    public var id: String { threadID }
    public var threadID: String
    public var metric: String
    public var value: Double
    public var threshold: Double
    public var description: String

    public init(threadID: String, metric: String, value: Double, threshold: Double, description: String) {
        self.threadID = threadID
        self.metric = metric
        self.value = value
        self.threshold = threshold
        self.description = description
    }
}

public struct ActivityBucket: Identifiable, Codable, Sendable, Hashable {
    public var id: Int { bucket }
    public var bucket: Int
    public var count: Int

    public init(bucket: Int, count: Int) {
        self.bucket = bucket
        self.count = count
    }
}

public struct ContrastFinding: Identifiable, Codable, Sendable, Hashable {
    public let id: UUID
    public var name: String
    public var leftLabel: String
    public var rightLabel: String
    public var metric: String
    public var leftValue: Double
    public var rightValue: Double
    public var magnitude: Double
    public var sampleSize: Int

    public init(id: UUID = UUID(), name: String, leftLabel: String, rightLabel: String, metric: String, leftValue: Double, rightValue: Double, magnitude: Double, sampleSize: Int) {
        self.id = id
        self.name = name
        self.leftLabel = leftLabel
        self.rightLabel = rightLabel
        self.metric = metric
        self.leftValue = leftValue
        self.rightValue = rightValue
        self.magnitude = magnitude
        self.sampleSize = sampleSize
    }
}

public struct CorrelationFinding: Identifiable, Codable, Sendable, Hashable {
    public let id: UUID
    public var xMetric: String
    public var yMetric: String
    public var coefficient: Double
    public var sampleSize: Int

    public init(id: UUID = UUID(), xMetric: String, yMetric: String, coefficient: Double, sampleSize: Int) {
        self.id = id
        self.xMetric = xMetric
        self.yMetric = yMetric
        self.coefficient = coefficient
        self.sampleSize = sampleSize
    }
}

public struct NetworkParticipantMetric: Identifiable, Codable, Sendable, Hashable {
    public var id: String { user }
    public var user: String
    public var mentionsReceived: Int
    public var repliesReceived: Int
    public var repliesSent: Int
    public var interactionCount: Int

    public init(user: String, mentionsReceived: Int, repliesReceived: Int, repliesSent: Int, interactionCount: Int) {
        self.user = user
        self.mentionsReceived = mentionsReceived
        self.repliesReceived = repliesReceived
        self.repliesSent = repliesSent
        self.interactionCount = interactionCount
    }
}

public struct NetworkSummary: Codable, Sendable, Hashable {
    public var mostMentionedUsers: [NetworkParticipantMetric]
    public var mostRepliedToUsers: [NetworkParticipantMetric]
    public var highInteractionParticipants: [NetworkParticipantMetric]
    public var possibleBottleneckUsers: [NetworkParticipantMetric]
    public var isolatedParticipants: [String]
    public var centralizationScore: Double

    public init(mostMentionedUsers: [NetworkParticipantMetric] = [], mostRepliedToUsers: [NetworkParticipantMetric] = [], highInteractionParticipants: [NetworkParticipantMetric] = [], possibleBottleneckUsers: [NetworkParticipantMetric] = [], isolatedParticipants: [String] = [], centralizationScore: Double = 0) {
        self.mostMentionedUsers = mostMentionedUsers
        self.mostRepliedToUsers = mostRepliedToUsers
        self.highInteractionParticipants = highInteractionParticipants
        self.possibleBottleneckUsers = possibleBottleneckUsers
        self.isolatedParticipants = isolatedParticipants
        self.centralizationScore = centralizationScore
    }
}

/// Complete local analysis of an imported Webex dataset.
public struct AnalysisResult: Codable, Sendable, Hashable {
    public var datasetSummary: DatasetSummary
    public var messageCount: Int
    public var threadCount: Int
    public var participantCount: Int
    public var spaceCount: Int
    public var threads: [ConversationThread]
    public var topTopicsByVolume: [TopicMetric]
    public var topicsWithLongestResponseTimes: [TopicMetric]
    public var topicsWithMostUnansweredQuestions: [TopicMetric]
    public var usersWithHighestMessageVolume: [UserMetric]
    public var usersWithFastestAverageResponseTime: [UserMetric]
    public var usersWithSlowestAverageResponseTime: [UserMetric]
    public var usersAskingMostQuestions: [UserMetric]
    public var spacesWithHighestActivity: [SpaceMetric]
    public var outlierThreadsByLength: [OutlierThread]
    public var outlierThreadsByDuration: [OutlierThread]
    public var outlierThreadsByNegativeSentiment: [OutlierThread]
    public var outlierThreadsByResponseDelay: [OutlierThread]
    public var activityByHour: [ActivityBucket]
    public var activityByDay: [ActivityBucket]
    public var highDelayVsLowDelayContrasts: [ContrastFinding]
    public var longThreadVsShortThreadContrasts: [ContrastFinding]
    public var positiveVsNegativeSentimentContrasts: [ContrastFinding]
    public var highQuestionVsLowQuestionContrasts: [ContrastFinding]
    public var correlationFindings: [CorrelationFinding]
    public var networkSummary: NetworkSummary

    public init(
        datasetSummary: DatasetSummary,
        messageCount: Int,
        threadCount: Int,
        participantCount: Int,
        spaceCount: Int,
        threads: [ConversationThread],
        topTopicsByVolume: [TopicMetric],
        topicsWithLongestResponseTimes: [TopicMetric],
        topicsWithMostUnansweredQuestions: [TopicMetric],
        usersWithHighestMessageVolume: [UserMetric],
        usersWithFastestAverageResponseTime: [UserMetric],
        usersWithSlowestAverageResponseTime: [UserMetric],
        usersAskingMostQuestions: [UserMetric],
        spacesWithHighestActivity: [SpaceMetric],
        outlierThreadsByLength: [OutlierThread],
        outlierThreadsByDuration: [OutlierThread],
        outlierThreadsByNegativeSentiment: [OutlierThread],
        outlierThreadsByResponseDelay: [OutlierThread],
        activityByHour: [ActivityBucket],
        activityByDay: [ActivityBucket],
        highDelayVsLowDelayContrasts: [ContrastFinding],
        longThreadVsShortThreadContrasts: [ContrastFinding],
        positiveVsNegativeSentimentContrasts: [ContrastFinding],
        highQuestionVsLowQuestionContrasts: [ContrastFinding],
        correlationFindings: [CorrelationFinding],
        networkSummary: NetworkSummary
    ) {
        self.datasetSummary = datasetSummary
        self.messageCount = messageCount
        self.threadCount = threadCount
        self.participantCount = participantCount
        self.spaceCount = spaceCount
        self.threads = threads
        self.topTopicsByVolume = topTopicsByVolume
        self.topicsWithLongestResponseTimes = topicsWithLongestResponseTimes
        self.topicsWithMostUnansweredQuestions = topicsWithMostUnansweredQuestions
        self.usersWithHighestMessageVolume = usersWithHighestMessageVolume
        self.usersWithFastestAverageResponseTime = usersWithFastestAverageResponseTime
        self.usersWithSlowestAverageResponseTime = usersWithSlowestAverageResponseTime
        self.usersAskingMostQuestions = usersAskingMostQuestions
        self.spacesWithHighestActivity = spacesWithHighestActivity
        self.outlierThreadsByLength = outlierThreadsByLength
        self.outlierThreadsByDuration = outlierThreadsByDuration
        self.outlierThreadsByNegativeSentiment = outlierThreadsByNegativeSentiment
        self.outlierThreadsByResponseDelay = outlierThreadsByResponseDelay
        self.activityByHour = activityByHour
        self.activityByDay = activityByDay
        self.highDelayVsLowDelayContrasts = highDelayVsLowDelayContrasts
        self.longThreadVsShortThreadContrasts = longThreadVsShortThreadContrasts
        self.positiveVsNegativeSentimentContrasts = positiveVsNegativeSentimentContrasts
        self.highQuestionVsLowQuestionContrasts = highQuestionVsLowQuestionContrasts
        self.correlationFindings = correlationFindings
        self.networkSummary = networkSummary
    }
}
