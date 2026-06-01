import Foundation

/// A normalized Webex chat message enriched with local analytical features.
public struct Message: Identifiable, Codable, Sendable, Hashable {
    public let id: UUID
    public var messageID: String?
    public var threadID: String?
    public var spaceID: String?
    public var spaceName: String?
    public var senderID: String?
    public var senderName: String?
    public var timestamp: Date
    public var text: String
    public var mentions: [String]
    public var replyToMessageID: String?
    public var rawSource: String?

    public var messageLengthCharacters: Int?
    public var messageLengthWords: Int?
    public var isQuestion: Bool?
    public var hasMention: Bool?
    public var mentionCount: Int?
    public var hourOfDay: Int?
    public var dayOfWeek: Int?
    public var sentimentScore: Double?
    public var topicLabel: String?

    public init(
        id: UUID = UUID(),
        messageID: String? = nil,
        threadID: String? = nil,
        spaceID: String? = nil,
        spaceName: String? = nil,
        senderID: String? = nil,
        senderName: String? = nil,
        timestamp: Date,
        text: String,
        mentions: [String] = [],
        replyToMessageID: String? = nil,
        rawSource: String? = nil,
        messageLengthCharacters: Int? = nil,
        messageLengthWords: Int? = nil,
        isQuestion: Bool? = nil,
        hasMention: Bool? = nil,
        mentionCount: Int? = nil,
        hourOfDay: Int? = nil,
        dayOfWeek: Int? = nil,
        sentimentScore: Double? = nil,
        topicLabel: String? = nil
    ) {
        self.id = id
        self.messageID = messageID
        self.threadID = threadID
        self.spaceID = spaceID
        self.spaceName = spaceName
        self.senderID = senderID
        self.senderName = senderName
        self.timestamp = timestamp
        self.text = text
        self.mentions = mentions
        self.replyToMessageID = replyToMessageID
        self.rawSource = rawSource
        self.messageLengthCharacters = messageLengthCharacters
        self.messageLengthWords = messageLengthWords
        self.isQuestion = isQuestion
        self.hasMention = hasMention
        self.mentionCount = mentionCount
        self.hourOfDay = hourOfDay
        self.dayOfWeek = dayOfWeek
        self.sentimentScore = sentimentScore
        self.topicLabel = topicLabel
    }
}
