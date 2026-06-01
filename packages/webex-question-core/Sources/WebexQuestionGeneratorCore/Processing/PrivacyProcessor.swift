import Foundation

/// Applies local-only privacy transforms before analysis.
public struct PrivacyProcessor: Sendable {
    private var userMap: [String: String] = [:]

    public init() {}

    public mutating func process(messages: [Message], configuration: PrivacyConfiguration) -> [Message] {
        messages.map { process(message: $0, configuration: configuration) }
    }

    private mutating func process(message: Message, configuration: PrivacyConfiguration) -> Message {
        var result = message
        if configuration.redactURLs { result.text = TextUtilities.redactURLs(result.text) }
        if configuration.redactEmails { result.text = TextUtilities.redactEmails(result.text) }
        result.mentions = TextUtilities.extractMentions(from: result.text)
        if configuration.anonymizeUsers {
            if let senderID = result.senderID { result.senderID = anonymized(senderID) }
            if let senderName = result.senderName { result.senderName = anonymized(senderName) }
            result.mentions = result.mentions.map { anonymized($0) }
        }
        return result
    }

    private mutating func anonymized(_ value: String) -> String {
        let normalized = value.trimmingCharacters(in: .whitespacesAndNewlines).lowercased()
        guard !normalized.isEmpty else { return value }
        if let existing = userMap[normalized] { return existing }
        let next = "User \(userMap.count + 1)"
        userMap[normalized] = next
        return next
    }
}
