import Foundation

/// Detects outlier threads using IQR thresholds with sensible fallbacks.
public struct AnomalyDetector: Sendable {
    public init() {}

    public func outliersByLength(_ threads: [ConversationThread]) -> [OutlierThread] {
        outliers(threads: threads, metric: "message_count", values: threads.map { Double($0.messageCount) }) { Double($0.messageCount) }
    }

    public func outliersByDuration(_ threads: [ConversationThread]) -> [OutlierThread] {
        outliers(threads: threads, metric: "duration_seconds", values: threads.map(\.durationSeconds)) { $0.durationSeconds }
    }

    public func outliersByNegativeSentiment(_ threads: [ConversationThread]) -> [OutlierThread] {
        let values = threads.compactMap { $0.sentimentMin.map { -$0 } }
        let threshold = upperTailThreshold(values) ?? 0.5
        return threads.compactMap { thread in
            guard let min = thread.sentimentMin else { return nil }
            let value = -min
            guard value >= threshold, min < 0 else { return nil }
            return OutlierThread(threadID: thread.threadID, metric: "negative_sentiment", value: min, threshold: -threshold, description: "Thread sentiment minimum is materially lower than the dataset baseline.")
        }.sorted { $0.value < $1.value }
    }

    public func outliersByResponseDelay(_ threads: [ConversationThread]) -> [OutlierThread] {
        let values = threads.compactMap(\.maxResponseGapSeconds)
        let threshold = upperTailThreshold(values) ?? 0
        return threads.compactMap { thread in
            guard let value = thread.maxResponseGapSeconds, value >= threshold, value > 0 else { return nil }
            return OutlierThread(threadID: thread.threadID, metric: "max_response_gap_seconds", value: value, threshold: threshold, description: "Thread has an unusually long response gap.")
        }.sorted { $0.value > $1.value }
    }

    private func outliers(threads: [ConversationThread], metric: String, values: [Double], value: (ConversationThread) -> Double) -> [OutlierThread] {
        let threshold = upperTailThreshold(values) ?? Double.greatestFiniteMagnitude
        return threads.compactMap { thread in
            let metricValue = value(thread)
            guard metricValue >= threshold, metricValue > 0 else { return nil }
            return OutlierThread(threadID: thread.threadID, metric: metric, value: metricValue, threshold: threshold, description: "Thread is an upper-tail outlier for \(metric).")
        }.sorted { $0.value > $1.value }
    }

    private func upperTailThreshold(_ values: [Double]) -> Double? {
        let cleanValues = values.filter(\.isFinite).sorted()
        guard !cleanValues.isEmpty else { return nil }
        guard cleanValues.count >= 4 else {
            return compactSmallSampleThreshold(cleanValues)
        }
        guard let threshold = Statistics.upperOutlierThreshold(cleanValues), threshold.isFinite else {
            return compactSmallSampleThreshold(cleanValues)
        }
        if let maximum = cleanValues.last, threshold > maximum {
            return compactSmallSampleThreshold(cleanValues)
        }
        return threshold
    }

    private func compactSmallSampleThreshold(_ values: [Double]) -> Double? {
        guard let percentile90 = Statistics.percentile(values, 0.9), let median = Statistics.median(values) else {
            return nil
        }
        let ratioThreshold = median > 0 ? median * 1.5 : percentile90
        return max(percentile90, ratioThreshold)
    }
}
