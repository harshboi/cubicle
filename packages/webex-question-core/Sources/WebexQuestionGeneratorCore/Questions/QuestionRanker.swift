import Foundation

/// Applies configured scoring weights and deterministic ordering.
public struct QuestionRanker: Sendable {
    public init() {}

    public func rank(_ questions: [GeneratedQuestion], configuration: ScoringConfiguration, topN: Int?) throws -> [GeneratedQuestion] {
        if let topN, topN <= 0 { throw WebexQGError.invalidTopN(topN) }
        let configuredTotal = configuration.interestingnessWeight + configuration.actionabilityWeight + configuration.confidenceWeight
        let weights = configuredTotal > 0
            ? configuration
            : ScoringConfiguration(interestingnessWeight: 0.45, actionabilityWeight: 0.35, confidenceWeight: 0.20)
        let denominator = max(
            0.0001,
            weights.interestingnessWeight + weights.actionabilityWeight + weights.confidenceWeight
        )
        let scored = questions.map { question -> GeneratedQuestion in
            var copy = question
            copy.interestingnessScore = clamp(copy.interestingnessScore)
            copy.actionabilityScore = clamp(copy.actionabilityScore)
            copy.confidenceScore = clamp(copy.confidenceScore)
            copy.finalScore = (
                weights.interestingnessWeight * copy.interestingnessScore +
                weights.actionabilityWeight * copy.actionabilityScore +
                weights.confidenceWeight * copy.confidenceScore
            ) / denominator
            return copy
        }
        let sorted = scored.sorted { left, right in
            if left.finalScore == right.finalScore { return left.text < right.text }
            return left.finalScore > right.finalScore
        }
        return Array(sorted.prefix(topN ?? sorted.count))
    }

    private func clamp(_ value: Double) -> Double {
        max(0, min(1, value))
    }
}
