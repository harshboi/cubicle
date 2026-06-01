import Foundation

/// Lightweight local sentiment scorer using small lexicons and negation handling.
public struct SentimentAnalyzer: Sendable {
    public init() {}

    private let positive: Set<String> = [
        "good", "great", "excellent", "positive", "helpful", "clear", "resolved", "done", "thanks", "thank", "progress", "improved", "fast", "win", "success", "aligned", "useful", "strong", "works"
    ]
    private let negative: Set<String> = [
        "bad", "poor", "blocked", "blocker", "delay", "delayed", "late", "risk", "issue", "problem", "broken", "confusing", "unclear", "failed", "failure", "concern", "urgent", "escalate", "stuck", "slow", "outage", "incident"
    ]
    private let negations: Set<String> = ["not", "no", "never", "hardly", "without", "isn't", "aren't", "wasn't", "don't", "doesn't", "can't", "won't"]

    public func score(_ text: String) -> Double {
        let tokens = TextUtilities.tokenize(text)
        guard !tokens.isEmpty else { return 0 }
        var score = 0.0
        var negationWindow = 0
        for token in tokens {
            if negations.contains(token) {
                negationWindow = 3
                continue
            }
            var delta = 0.0
            if positive.contains(token) { delta = 1 }
            if negative.contains(token) { delta = -1 }
            if negationWindow > 0 { delta *= -1 }
            score += delta
            if negationWindow > 0 { negationWindow -= 1 }
        }
        return max(-1, min(1, score / max(3, Double(tokens.count) / 4)))
    }
}
