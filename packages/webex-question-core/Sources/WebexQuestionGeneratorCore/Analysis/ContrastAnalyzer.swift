import Foundation

/// Computes simple high-vs-low cohort contrasts over reconstructed threads.
public struct ContrastAnalyzer: Sendable {
    public init() {}

    public func highDelayVsLowDelay(_ threads: [ConversationThread]) -> [ContrastFinding] {
        contrast(
            name: "High-delay vs low-delay threads",
            threads: threads,
            splitValue: { $0.averageResponseTimeSeconds ?? 0 },
            metric: "unanswered_question_count",
            metricValue: { Double($0.unansweredQuestionCount) }
        )
    }

    public func longThreadVsShortThread(_ threads: [ConversationThread]) -> [ContrastFinding] {
        contrast(
            name: "Long-thread vs short-thread conversations",
            threads: threads,
            splitValue: { Double($0.messageCount) },
            metric: "participant_count",
            metricValue: { Double($0.participants.count) }
        )
    }

    public func positiveVsNegativeSentiment(_ threads: [ConversationThread]) -> [ContrastFinding] {
        let positives = threads.filter { ($0.sentimentMean ?? 0) > 0.1 }
        let negatives = threads.filter { ($0.sentimentMean ?? 0) < -0.1 }
        return compare(
            name: "Positive vs negative sentiment threads",
            leftLabel: "positive",
            rightLabel: "negative",
            left: positives,
            right: negatives,
            metric: "average_response_time_seconds",
            metricValue: { $0.averageResponseTimeSeconds ?? 0 }
        )
    }

    public func highQuestionVsLowQuestion(_ threads: [ConversationThread]) -> [ContrastFinding] {
        contrast(
            name: "High-question vs low-question threads",
            threads: threads,
            splitValue: { Double($0.questionCount) },
            metric: "duration_seconds",
            metricValue: { $0.durationSeconds }
        )
    }

    private func contrast(name: String, threads: [ConversationThread], splitValue: (ConversationThread) -> Double, metric: String, metricValue: (ConversationThread) -> Double) -> [ContrastFinding] {
        guard let median = Statistics.median(threads.map(splitValue)) else { return [] }
        let high = threads.filter { splitValue($0) >= median }
        let low = threads.filter { splitValue($0) < median }
        return compare(name: name, leftLabel: "high", rightLabel: "low", left: high, right: low, metric: metric, metricValue: metricValue)
    }

    private func compare(name: String, leftLabel: String, rightLabel: String, left: [ConversationThread], right: [ConversationThread], metric: String, metricValue: (ConversationThread) -> Double) -> [ContrastFinding] {
        guard !left.isEmpty, !right.isEmpty,
              let leftMean = Statistics.mean(left.map(metricValue)),
              let rightMean = Statistics.mean(right.map(metricValue)) else { return [] }
        let magnitude = abs(leftMean - rightMean)
        guard magnitude > 0 else { return [] }
        return [ContrastFinding(name: name, leftLabel: leftLabel, rightLabel: rightLabel, metric: metric, leftValue: leftMean, rightValue: rightMean, magnitude: magnitude, sampleSize: left.count + right.count)]
    }
}
