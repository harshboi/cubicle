import Foundation

/// A reconstructed conversation thread with thread-level metrics.
public struct ConversationThread: Identifiable, Codable, Sendable, Hashable {
    public let id: UUID
    public var threadID: String
    public var messages: [Message]
    public var participants: [String]
    public var startTime: Date
    public var endTime: Date
    public var durationSeconds: TimeInterval
    public var messageCount: Int
    public var averageResponseTimeSeconds: TimeInterval?
    public var maxResponseGapSeconds: TimeInterval?
    public var questionCount: Int
    public var unansweredQuestionCount: Int
    public var sentimentMean: Double?
    public var sentimentMin: Double?
    public var topicDistribution: [String: Int]
    public var firstResponseTimeSeconds: TimeInterval?

    public init(
        id: UUID = UUID(),
        threadID: String,
        messages: [Message],
        participants: [String],
        startTime: Date,
        endTime: Date,
        durationSeconds: TimeInterval,
        messageCount: Int,
        averageResponseTimeSeconds: TimeInterval? = nil,
        maxResponseGapSeconds: TimeInterval? = nil,
        questionCount: Int,
        unansweredQuestionCount: Int,
        sentimentMean: Double? = nil,
        sentimentMin: Double? = nil,
        topicDistribution: [String: Int] = [:],
        firstResponseTimeSeconds: TimeInterval? = nil
    ) {
        self.id = id
        self.threadID = threadID
        self.messages = messages
        self.participants = participants
        self.startTime = startTime
        self.endTime = endTime
        self.durationSeconds = durationSeconds
        self.messageCount = messageCount
        self.averageResponseTimeSeconds = averageResponseTimeSeconds
        self.maxResponseGapSeconds = maxResponseGapSeconds
        self.questionCount = questionCount
        self.unansweredQuestionCount = unansweredQuestionCount
        self.sentimentMean = sentimentMean
        self.sentimentMin = sentimentMin
        self.topicDistribution = topicDistribution
        self.firstResponseTimeSeconds = firstResponseTimeSeconds
    }
}
