import Foundation

/// Imports JSON arrays, JSON objects with message arrays, or JSONL rows.
public struct JSONChatImporter: ChatImporting {
    private let dateParser = DateParser()

    public init() {}

    public func importMessages(from url: URL) async throws -> [Message] {
        let data = try Data(contentsOf: url)
        if url.pathExtension.lowercased() == "jsonl" {
            let text = String(data: data, encoding: .utf8) ?? ""
            return try importJSONLines(text)
        }
        return try importMessages(fromJSONData: data)
    }

    public func importMessages(fromJSONData data: Data) throws -> [Message] {
        if let messages = try? JSONDecoder.webexQG.decode([Message].self, from: data) {
            return messages.sorted { $0.timestamp < $1.timestamp }
        }
        let object = try JSONSerialization.jsonObject(with: data)
        let rows: [[String: Any]]
        if let array = object as? [[String: Any]] {
            rows = array
        } else if let dict = object as? [String: Any] {
            rows = (dict["messages"] ?? dict["items"] ?? dict["data"]).flatMap { $0 as? [[String: Any]] } ?? []
        } else {
            rows = []
        }
        let messages = rows.compactMap(message(from:))
        if messages.isEmpty { throw WebexQGError.noMessagesFound }
        return messages.sorted { $0.timestamp < $1.timestamp }
    }

    private func importJSONLines(_ text: String) throws -> [Message] {
        var messages: [Message] = []
        for line in text.split(whereSeparator: { $0.isNewline }) {
            guard let data = String(line).data(using: .utf8),
                  let object = try JSONSerialization.jsonObject(with: data) as? [String: Any],
                  let message = message(from: object) else { continue }
            messages.append(message)
        }
        if messages.isEmpty { throw WebexQGError.noMessagesFound }
        return messages.sorted { $0.timestamp < $1.timestamp }
    }

    private func message(from row: [String: Any]) -> Message? {
        func string(_ keys: [String]) -> String? {
            for key in keys {
                if let value = row[key] as? String, !value.trimmingCharacters(in: .whitespacesAndNewlines).isEmpty { return value }
                if let number = row[key] as? NSNumber { return number.stringValue }
            }
            return nil
        }
        let timestampRaw = string(["timestamp", "time", "created", "created_at", "date", "sent_at"])
        guard let timestamp = dateParser.parse(timestampRaw) else { return nil }
        guard let text = string(["message", "text", "body", "content", "markdown"]), !text.isEmpty else { return nil }
        let senderName = string(["sender", "from", "author", "person", "sender_name", "displayName", "display_name", "name"])
        let senderID = string(["sender_id", "person_id", "author_id", "email", "sender_email"])
        return Message(
            messageID: string(["id", "message_id", "messageID", "event_id"]),
            threadID: string(["thread", "thread_id", "parent", "conversation_id"]),
            spaceID: string(["space_id", "room_id", "channel_id"]),
            spaceName: string(["space", "room", "channel", "space_name", "room_name", "channel_name"]),
            senderID: senderID ?? senderName,
            senderName: senderName,
            timestamp: timestamp,
            text: text,
            mentions: TextUtilities.extractMentions(from: text),
            replyToMessageID: string(["reply_to", "reply_to_message_id", "parent_message_id", "in_reply_to"]),
            rawSource: nil
        )
    }
}

extension JSONDecoder {
    static var webexQG: JSONDecoder {
        let decoder = JSONDecoder()
        decoder.dateDecodingStrategy = .iso8601
        return decoder
    }
}
