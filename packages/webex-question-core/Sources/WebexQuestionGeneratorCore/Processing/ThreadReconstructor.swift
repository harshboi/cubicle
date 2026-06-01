import Foundation

/// Reconstructs conversation threads from explicit thread IDs, reply links, or time-window grouping.
public struct ThreadReconstructor: Sendable {
    public init() {}

    public func reconstruct(messages: [Message], configuration: ThreadingConfiguration) -> [ConversationThread] {
        let sorted = messages.sorted { $0.timestamp < $1.timestamp }
        var groups: [String: [Message]] = [:]
        var messageIDToThread: [String: String] = [:]
        var fallbackThreadBySpace: [String: (threadID: String, lastTimestamp: Date)] = [:]
        let window = TimeInterval(max(configuration.fallbackWindowMinutes, 1) * 60)

        for message in sorted {
            let spaceKey = message.spaceID ?? message.spaceName ?? "unknown-space"
            let threadKey: String
            if let explicit = nonEmpty(message.threadID) {
                threadKey = explicit
            } else if let replyTo = nonEmpty(message.replyToMessageID), let existing = messageIDToThread[replyTo] {
                threadKey = existing
            } else if let previous = fallbackThreadBySpace[spaceKey], message.timestamp.timeIntervalSince(previous.lastTimestamp) <= window {
                threadKey = previous.threadID
            } else {
                threadKey = "fallback:\(spaceKey):\(message.timestamp.timeIntervalSince1970)"
            }
            groups[threadKey, default: []].append(message)
            if let messageID = nonEmpty(message.messageID) { messageIDToThread[messageID] = threadKey }
            fallbackThreadBySpace[spaceKey] = (threadKey, message.timestamp)
        }

        return groups.map { key, messages in makeThread(threadID: key, messages: messages) }
            .sorted { $0.startTime < $1.startTime }
    }

    private func makeThread(threadID: String, messages rawMessages: [Message]) -> ConversationThread {
        let messages = rawMessages.sorted { $0.timestamp < $1.timestamp }
        let participants = Array(Set(messages.compactMap { $0.senderID ?? $0.senderName })).sorted()
        let start = messages.first?.timestamp ?? Date(timeIntervalSince1970: 0)
        let end = messages.last?.timestamp ?? start
        var gaps: [TimeInterval] = []
        var firstResponse: TimeInterval?
        for pair in zip(messages, messages.dropFirst()) {
            if (pair.0.senderID ?? pair.0.senderName) != (pair.1.senderID ?? pair.1.senderName) {
                let gap = pair.1.timestamp.timeIntervalSince(pair.0.timestamp)
                gaps.append(gap)
                if firstResponse == nil { firstResponse = gap }
            }
        }
        let questionCount = messages.filter { $0.isQuestion == true }.count
        let unanswered = unansweredQuestionCount(messages: messages)
        let sentiments = messages.compactMap(\.sentimentScore)
        let topics = messages.reduce(into: [String: Int]()) { counts, message in
            counts[message.topicLabel ?? "general", default: 0] += 1
        }
        return ConversationThread(
            threadID: threadID,
            messages: messages,
            participants: participants,
            startTime: start,
            endTime: end,
            durationSeconds: end.timeIntervalSince(start),
            messageCount: messages.count,
            averageResponseTimeSeconds: Statistics.mean(gaps),
            maxResponseGapSeconds: gaps.max(),
            questionCount: questionCount,
            unansweredQuestionCount: unanswered,
            sentimentMean: Statistics.mean(sentiments),
            sentimentMin: sentiments.min(),
            topicDistribution: topics,
            firstResponseTimeSeconds: firstResponse
        )
    }

    private func unansweredQuestionCount(messages: [Message]) -> Int {
        var count = 0
        for (index, message) in messages.enumerated() where message.isQuestion == true {
            let asker = message.senderID ?? message.senderName
            let hasLaterDifferentSender = messages.dropFirst(index + 1).contains { ($0.senderID ?? $0.senderName) != asker }
            if !hasLaterDifferentSender { count += 1 }
        }
        return count
    }

    private func nonEmpty(_ value: String?) -> String? {
        guard let trimmed = value?.trimmingCharacters(in: .whitespacesAndNewlines), !trimmed.isEmpty else { return nil }
        return trimmed
    }
}
