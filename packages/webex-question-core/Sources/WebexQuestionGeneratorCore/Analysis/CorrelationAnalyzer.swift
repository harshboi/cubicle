import Foundation

/// Computes dependency-light correlations between thread metrics.
public struct CorrelationAnalyzer: Sendable {
    public init() {}

    public func analyze(threads: [ConversationThread]) -> [CorrelationFinding] {
        let candidates: [(String, String, [(Double, Double)])] = [
            ("message_count", "duration_seconds", threads.map { (Double($0.messageCount), $0.durationSeconds) }),
            ("question_count", "duration_seconds", threads.map { (Double($0.questionCount), $0.durationSeconds) }),
            ("max_response_gap_seconds", "unanswered_question_count", threads.compactMap { thread in
                guard let gap = thread.maxResponseGapSeconds else { return nil }
                return (gap, Double(thread.unansweredQuestionCount))
            })
        ]
        return candidates.compactMap { x, y, pairs in
            guard let coefficient = Statistics.pearson(pairs), abs(coefficient) >= 0.2 else { return nil }
            return CorrelationFinding(xMetric: x, yMetric: y, coefficient: coefficient, sampleSize: pairs.count)
        }.sorted { abs($0.coefficient) > abs($1.coefficient) }
    }
}
