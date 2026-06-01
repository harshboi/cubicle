import Foundation

/// Plain text fallback importer for simple timestamp/sender/message lines.
public struct TextChatImporter: ChatImporting {
    private let dateParser = DateParser()

    public init() {}

    public func importMessages(from url: URL) async throws -> [Message] {
        let content = try String(contentsOf: url, encoding: .utf8)
        return try importMessages(fromText: content)
    }

    public func importMessages(fromText content: String) throws -> [Message] {
        var messages: [Message] = []
        for (index, line) in content.split(whereSeparator: { $0.isNewline }).map(String.init).enumerated() {
            let trimmed = line.trimmingCharacters(in: .whitespacesAndNewlines)
            guard !trimmed.isEmpty else { continue }
            let parsed = parseLine(trimmed)
            messages.append(Message(
                messageID: "text-\(index + 1)",
                spaceName: "Plain Text Import",
                senderID: parsed.sender,
                senderName: parsed.sender,
                timestamp: parsed.date,
                text: parsed.text,
                mentions: TextUtilities.extractMentions(from: parsed.text),
                rawSource: line
            ))
        }
        if messages.isEmpty { throw WebexQGError.noMessagesFound }
        return messages.sorted { $0.timestamp < $1.timestamp }
    }

    private func parseLine(_ line: String) -> (date: Date, sender: String?, text: String) {
        let separators = [" | ", " - ", "\t"]
        for separator in separators {
            let parts = line.components(separatedBy: separator)
            if parts.count >= 3, let date = dateParser.parse(parts[0]) {
                return (date, parts[1], parts.dropFirst(2).joined(separator: separator))
            }
        }
        if let colon = line.firstIndex(of: ":") {
            let sender = String(line[..<colon]).trimmingCharacters(in: .whitespacesAndNewlines)
            let text = String(line[line.index(after: colon)...]).trimmingCharacters(in: .whitespacesAndNewlines)
            return (Date(timeIntervalSince1970: TimeInterval(messagesStableHash(line) % 1_000_000)), sender.isEmpty ? nil : sender, text)
        }
        return (Date(timeIntervalSince1970: TimeInterval(messagesStableHash(line) % 1_000_000)), nil, line)
    }

    private func messagesStableHash(_ value: String) -> UInt64 {
        value.utf8.reduce(UInt64(5381)) { (($0 << 5) &+ $0) &+ UInt64($1) }
    }
}
