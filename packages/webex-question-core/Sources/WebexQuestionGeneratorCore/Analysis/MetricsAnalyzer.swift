import Foundation

/// Builds the full AnalysisResult from enriched messages and reconstructed threads.
public struct MetricsAnalyzer: Sendable {
    private let anomalyDetector = AnomalyDetector()
    private let contrastAnalyzer = ContrastAnalyzer()
    private let correlationAnalyzer = CorrelationAnalyzer()
    private let networkAnalyzer = NetworkAnalyzer()

    public init() {}

    public func analyze(messages: [Message], threads: [ConversationThread]) -> AnalysisResult {
        let participants = Set(messages.compactMap { $0.senderID ?? $0.senderName })
        let spaces = Set(messages.compactMap { $0.spaceID ?? $0.spaceName })
        let start = messages.map(\.timestamp).min()
        let end = messages.map(\.timestamp).max()
        let durationDays = start.flatMap { start in end.map { max(0, $0.timeIntervalSince(start) / 86_400) } } ?? 0
        let completeness = dataCompleteness(messages)
        let summary = DatasetSummary(
            messageCount: messages.count,
            threadCount: threads.count,
            participantCount: participants.count,
            spaceCount: spaces.count,
            startDate: start,
            endDate: end,
            durationDays: durationDays,
            dataCompletenessScore: completeness
        )
        let topicMetrics = topicMetrics(threads: threads)
        let userMetrics = userMetrics(messages: messages, threads: threads)
        let spaceMetrics = spaceMetrics(messages: messages, threads: threads)

        return AnalysisResult(
            datasetSummary: summary,
            messageCount: messages.count,
            threadCount: threads.count,
            participantCount: participants.count,
            spaceCount: spaces.count,
            threads: threads,
            topTopicsByVolume: Array(topicMetrics.sorted { $0.count == $1.count ? $0.topic < $1.topic : $0.count > $1.count }.prefix(20)),
            topicsWithLongestResponseTimes: Array(topicMetrics.filter { $0.averageResponseTimeSeconds != nil }.sorted { ($0.averageResponseTimeSeconds ?? 0) > ($1.averageResponseTimeSeconds ?? 0) }.prefix(20)),
            topicsWithMostUnansweredQuestions: Array(topicMetrics.sorted { $0.unansweredQuestionCount == $1.unansweredQuestionCount ? $0.topic < $1.topic : $0.unansweredQuestionCount > $1.unansweredQuestionCount }.prefix(20)),
            usersWithHighestMessageVolume: Array(userMetrics.sorted { $0.messageCount == $1.messageCount ? $0.user < $1.user : $0.messageCount > $1.messageCount }.prefix(20)),
            usersWithFastestAverageResponseTime: Array(userMetrics.filter { $0.averageResponseTimeSeconds != nil }.sorted { ($0.averageResponseTimeSeconds ?? .greatestFiniteMagnitude) < ($1.averageResponseTimeSeconds ?? .greatestFiniteMagnitude) }.prefix(20)),
            usersWithSlowestAverageResponseTime: Array(userMetrics.filter { $0.averageResponseTimeSeconds != nil }.sorted { ($0.averageResponseTimeSeconds ?? 0) > ($1.averageResponseTimeSeconds ?? 0) }.prefix(20)),
            usersAskingMostQuestions: Array(userMetrics.sorted { $0.questionCount == $1.questionCount ? $0.user < $1.user : $0.questionCount > $1.questionCount }.prefix(20)),
            spacesWithHighestActivity: Array(spaceMetrics.sorted { $0.messageCount == $1.messageCount ? $0.space < $1.space : $0.messageCount > $1.messageCount }.prefix(20)),
            outlierThreadsByLength: anomalyDetector.outliersByLength(threads),
            outlierThreadsByDuration: anomalyDetector.outliersByDuration(threads),
            outlierThreadsByNegativeSentiment: anomalyDetector.outliersByNegativeSentiment(threads),
            outlierThreadsByResponseDelay: anomalyDetector.outliersByResponseDelay(threads),
            activityByHour: buckets(messages.compactMap(\.hourOfDay), range: 0..<24),
            activityByDay: buckets(messages.compactMap(\.dayOfWeek), range: 1..<8),
            highDelayVsLowDelayContrasts: contrastAnalyzer.highDelayVsLowDelay(threads),
            longThreadVsShortThreadContrasts: contrastAnalyzer.longThreadVsShortThread(threads),
            positiveVsNegativeSentimentContrasts: contrastAnalyzer.positiveVsNegativeSentiment(threads),
            highQuestionVsLowQuestionContrasts: contrastAnalyzer.highQuestionVsLowQuestion(threads),
            correlationFindings: correlationAnalyzer.analyze(threads: threads),
            networkSummary: networkAnalyzer.analyze(messages: messages, threads: threads)
        )
    }

    private func dataCompleteness(_ messages: [Message]) -> Double {
        guard !messages.isEmpty else { return 0 }
        let present = messages.map { message -> Double in
            var score = 0.0
            if message.senderID != nil || message.senderName != nil { score += 1 }
            if message.spaceID != nil || message.spaceName != nil { score += 1 }
            if message.messageID != nil { score += 1 }
            return score / 3
        }
        return Statistics.mean(present) ?? 0
    }

    private func topicMetrics(threads: [ConversationThread]) -> [TopicMetric] {
        var volume: [String: Int] = [:]
        var responseTimes: [String: [Double]] = [:]
        var unanswered: [String: Int] = [:]
        var negative: [String: Int] = [:]
        for thread in threads {
            for (topic, count) in thread.topicDistribution {
                volume[topic, default: 0] += count
                if let response = thread.averageResponseTimeSeconds { responseTimes[topic, default: []].append(response) }
                unanswered[topic, default: 0] += thread.unansweredQuestionCount
                if (thread.sentimentMean ?? 0) < -0.1 { negative[topic, default: 0] += 1 }
            }
        }
        return volume.map { topic, count in
            TopicMetric(
                topic: topic,
                count: count,
                averageResponseTimeSeconds: Statistics.mean(responseTimes[topic, default: []]),
                unansweredQuestionCount: unanswered[topic, default: 0],
                negativeSentimentShare: Double(negative[topic, default: 0]) / Double(max(1, count))
            )
        }
    }

    private func userMetrics(messages: [Message], threads: [ConversationThread]) -> [UserMetric] {
        var messageCounts: [String: Int] = [:]
        var questionCounts: [String: Int] = [:]
        var mentionCounts: [String: Int] = [:]
        var responseTimes: [String: [Double]] = [:]
        for message in messages {
            let user = message.senderID ?? message.senderName ?? "unknown"
            messageCounts[user, default: 0] += 1
            if message.isQuestion == true { questionCounts[user, default: 0] += 1 }
            for mention in message.mentions { mentionCounts[mention, default: 0] += 1 }
        }
        for thread in threads {
            for pair in zip(thread.messages, thread.messages.dropFirst()) {
                let previousUser = pair.0.senderID ?? pair.0.senderName
                let responder = pair.1.senderID ?? pair.1.senderName
                guard let responder, responder != previousUser else { continue }
                responseTimes[responder, default: []].append(pair.1.timestamp.timeIntervalSince(pair.0.timestamp))
            }
        }
        let users = Set(messageCounts.keys).union(questionCounts.keys).union(mentionCounts.keys).union(responseTimes.keys)
        return users.map { user in
            UserMetric(
                user: user,
                messageCount: messageCounts[user, default: 0],
                questionCount: questionCounts[user, default: 0],
                mentionCount: mentionCounts[user, default: 0],
                averageResponseTimeSeconds: Statistics.mean(responseTimes[user, default: []])
            )
        }
    }

    private func spaceMetrics(messages: [Message], threads: [ConversationThread]) -> [SpaceMetric] {
        var messageCounts: [String: Int] = [:]
        var participants: [String: Set<String>] = [:]
        for message in messages {
            let space = message.spaceID ?? message.spaceName ?? "unknown-space"
            messageCounts[space, default: 0] += 1
            if let user = message.senderID ?? message.senderName { participants[space, default: []].insert(user) }
        }
        var threadCounts: [String: Int] = [:]
        for thread in threads {
            let space = thread.messages.first?.spaceID ?? thread.messages.first?.spaceName ?? "unknown-space"
            threadCounts[space, default: 0] += 1
        }
        return messageCounts.map { space, count in
            SpaceMetric(space: space, messageCount: count, threadCount: threadCounts[space, default: 0], participantCount: participants[space, default: []].count)
        }
    }

    private func buckets(_ values: [Int], range: Range<Int>) -> [ActivityBucket] {
        let counts = values.reduce(into: [Int: Int]()) { $0[$1, default: 0] += 1 }
        return range.map { ActivityBucket(bucket: $0, count: counts[$0, default: 0]) }
    }
}
