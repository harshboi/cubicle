import Foundation

/// Dependency-light local topic labeler based on token frequencies.
public struct TopicAnalyzer: Sendable {
    public init() {}

    private let stopWords: Set<String> = [
        "the", "and", "for", "that", "this", "with", "you", "are", "was", "were", "have", "has", "had", "not", "but", "from", "they", "their", "our", "your", "can", "will", "would", "should", "about", "into", "there", "here", "what", "when", "where", "why", "how", "all", "any", "more", "some", "just", "team", "please", "thanks", "thank", "webex", "message"
    ]

    public func label(messages: [Message], configuration: TopicConfiguration) -> [Message] {
        guard configuration.enabled else { return messages }
        let corpusCounts = termCounts(messages.flatMap { tokens($0.text) })
        let globalTerms = Array(corpusCounts.sorted { left, right in
            if left.value == right.value { return left.key < right.key }
            return left.value > right.value
        }.prefix(max(configuration.numberOfTopics * 4, 1))).map(\.key)
        var result: [Message] = []
        for message in messages {
            var copy = message
            let messageCounts = termCounts(tokens(message.text))
            let ranked = globalTerms
                .map { ($0, messageCounts[$0, default: 0], corpusCounts[$0, default: 0]) }
                .filter { $0.1 > 0 }
                .sorted { left, right in
                    let leftScore = Double(left.1) * log(Double(messages.count + 1) / Double(left.2 + 1))
                    let rightScore = Double(right.1) * log(Double(messages.count + 1) / Double(right.2 + 1))
                    if leftScore == rightScore { return left.0 < right.0 }
                    return leftScore > rightScore
                }
            copy.topicLabel = ranked.prefix(3).map(\.0).joined(separator: " / ")
            if copy.topicLabel?.isEmpty != false { copy.topicLabel = "general" }
            result.append(copy)
        }
        return result
    }

    private func tokens(_ text: String) -> [String] {
        TextUtilities.tokenize(text).filter { token in
            token.count > 2 && !stopWords.contains(token) && !token.allSatisfy(\.isNumber)
        }
    }

    private func termCounts(_ tokens: [String]) -> [String: Int] {
        tokens.reduce(into: [:]) { counts, token in counts[token, default: 0] += 1 }
    }
}
