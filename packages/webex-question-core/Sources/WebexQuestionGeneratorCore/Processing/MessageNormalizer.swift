import Foundation

/// Normalizes message text and computes cheap text-derived attributes.
public struct MessageNormalizer: Sendable {
    public init() {}

    public func normalize(_ message: Message) -> Message {
        var result = message
        result.text = TextUtilities.collapseWhitespace(message.text)
        result.mentions = TextUtilities.extractMentions(from: result.text)
        result.messageLengthCharacters = result.text.count
        result.messageLengthWords = TextUtilities.words(in: result.text).count
        result.isQuestion = TextUtilities.isQuestion(result.text)
        result.mentionCount = result.mentions.count
        result.hasMention = !result.mentions.isEmpty
        let components = Calendar(identifier: .gregorian).dateComponents([.hour, .weekday], from: result.timestamp)
        result.hourOfDay = components.hour
        result.dayOfWeek = components.weekday
        return result
    }
}
