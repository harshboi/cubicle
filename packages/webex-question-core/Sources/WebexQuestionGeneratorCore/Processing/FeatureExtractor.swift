import Foundation

/// Computes message-level local features.
public struct FeatureExtractor: Sendable {
    private let normalizer = MessageNormalizer()
    private let sentiment = SentimentAnalyzer()
    private let topics = TopicAnalyzer()

    public init() {}

    public func enrich(messages: [Message], configuration: WebexQGConfiguration) -> [Message] {
        var privacyProcessor = PrivacyProcessor()
        let privateMessages = privacyProcessor.process(messages: messages, configuration: configuration.privacy)
        let normalized = privateMessages.map(normalizer.normalize)
        var enriched = normalized.map { message -> Message in
            var copy = message
            copy.sentimentScore = sentiment.score(message.text)
            return copy
        }
        enriched = topics.label(messages: enriched, configuration: configuration.topics)
        return enriched.sorted { $0.timestamp < $1.timestamp }
    }
}
