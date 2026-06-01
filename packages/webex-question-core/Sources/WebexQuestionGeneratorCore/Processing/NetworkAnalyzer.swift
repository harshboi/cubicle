import Foundation

/// Builds a local participant interaction graph from mentions and thread responder pairs.
public struct NetworkAnalyzer: Sendable {
    public init() {}

    public func analyze(messages: [Message], threads: [ConversationThread]) -> NetworkSummary {
        let participants = Set(messages.compactMap { $0.senderID ?? $0.senderName })
        var mentionsReceived: [String: Int] = [:]
        var repliesReceived: [String: Int] = [:]
        var repliesSent: [String: Int] = [:]
        var interactions: [String: Int] = [:]

        for message in messages {
            let sender = message.senderID ?? message.senderName ?? "unknown"
            interactions[sender, default: 0] += 1
            for mention in message.mentions {
                mentionsReceived[mention, default: 0] += 1
                interactions[mention, default: 0] += 1
            }
        }

        for thread in threads {
            for pair in zip(thread.messages, thread.messages.dropFirst()) {
                let sender = pair.0.senderID ?? pair.0.senderName
                let responder = pair.1.senderID ?? pair.1.senderName
                guard let sender, let responder, sender != responder else { continue }
                repliesReceived[responder, default: 0] += 1
                repliesSent[sender, default: 0] += 1
                interactions[sender, default: 0] += 1
                interactions[responder, default: 0] += 1
            }
        }

        let allUsers = participants.union(mentionsReceived.keys).union(repliesReceived.keys).union(interactions.keys)
        let metrics = allUsers.map { user in
            NetworkParticipantMetric(
                user: user,
                mentionsReceived: mentionsReceived[user, default: 0],
                repliesReceived: repliesReceived[user, default: 0],
                repliesSent: repliesSent[user, default: 0],
                interactionCount: interactions[user, default: 0]
            )
        }
        let sortedByMentions = metrics.sorted { $0.mentionsReceived == $1.mentionsReceived ? $0.user < $1.user : $0.mentionsReceived > $1.mentionsReceived }
        let sortedByReplies = metrics.sorted { $0.repliesReceived == $1.repliesReceived ? $0.user < $1.user : $0.repliesReceived > $1.repliesReceived }
        let sortedByInteractions = metrics.sorted { $0.interactionCount == $1.interactionCount ? $0.user < $1.user : $0.interactionCount > $1.interactionCount }
        let isolated = metrics.filter { $0.interactionCount <= 1 && $0.mentionsReceived == 0 && $0.repliesReceived == 0 }.map(\.user).sorted()
        let totalInteractions = max(1, metrics.map(\.interactionCount).reduce(0, +))
        let topInteraction = metrics.map(\.interactionCount).max() ?? 0
        return NetworkSummary(
            mostMentionedUsers: Array(sortedByMentions.prefix(10)),
            mostRepliedToUsers: Array(sortedByReplies.prefix(10)),
            highInteractionParticipants: Array(sortedByInteractions.prefix(10)),
            possibleBottleneckUsers: Array(sortedByReplies.filter { $0.repliesReceived + $0.mentionsReceived >= 3 }.prefix(10)),
            isolatedParticipants: isolated,
            centralizationScore: Double(topInteraction) / Double(totalInteractions)
        )
    }
}
