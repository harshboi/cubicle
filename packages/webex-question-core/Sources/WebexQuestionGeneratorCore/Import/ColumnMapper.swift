import Foundation

public enum MessageColumn: String, Codable, Sendable, CaseIterable, Hashable {
    case sender
    case timestamp
    case message
    case room
    case thread
    case messageID
    case replyTo
    case spaceID
    case senderID
}

public struct ColumnMappingSuggestion: Codable, Sendable, Hashable {
    public var detected: [MessageColumn: String]
    public var ambiguous: [MessageColumn: [String]]
    public var missingRequired: [MessageColumn]

    public init(detected: [MessageColumn: String], ambiguous: [MessageColumn: [String]], missingRequired: [MessageColumn]) {
        self.detected = detected
        self.ambiguous = ambiguous
        self.missingRequired = missingRequired
    }
}

/// Maps common Webex/chat export column variants onto Message fields.
public struct ColumnMapper: Sendable {
    public init() {}

    public static let variants: [MessageColumn: Set<String>] = [
        .sender: ["sender", "from", "author", "person", "sender_name", "displayname", "display_name", "name"],
        .timestamp: ["timestamp", "time", "created", "created_at", "date", "datetime", "sent_at"],
        .message: ["message", "text", "body", "content", "html", "markdown"],
        .room: ["room", "space", "channel", "room_name", "space_name", "channel_name"],
        .thread: ["thread", "parent", "conversation_id", "thread_id", "parent_id"],
        .messageID: ["id", "message_id", "messageid", "event_id"],
        .replyTo: ["reply_to", "reply_to_message_id", "parent_message_id", "in_reply_to"],
        .spaceID: ["space_id", "room_id", "channel_id"],
        .senderID: ["sender_id", "person_id", "author_id", "email", "sender_email"]
    ]

    public func suggestMapping(headers: [String]) -> ColumnMappingSuggestion {
        let normalizedHeaders = headers.map { ($0, Self.normalize($0)) }
        var detected: [MessageColumn: String] = [:]
        var ambiguous: [MessageColumn: [String]] = [:]
        for column in MessageColumn.allCases {
            let matches = normalizedHeaders.filter { _, normalized in
                Self.variants[column, default: []].contains(normalized)
            }.map(\.0)
            if matches.count == 1 {
                detected[column] = matches[0]
            } else if matches.count > 1 {
                ambiguous[column] = matches
                detected[column] = matches[0]
            }
        }
        let required: [MessageColumn] = [.timestamp, .message]
        let missing = required.filter { detected[$0] == nil }
        return ColumnMappingSuggestion(detected: detected, ambiguous: ambiguous, missingRequired: missing)
    }

    public func indexMapping(headers: [String], override: [MessageColumn: String] = [:]) throws -> [MessageColumn: Int] {
        var names = suggestMapping(headers: headers).detected
        for (column, header) in override {
            names[column] = header
        }
        let missing = [MessageColumn.timestamp, .message].filter { names[$0] == nil }
        if !missing.isEmpty { throw WebexQGError.missingRequiredColumns(missing.map(\.rawValue)) }
        let headerByNormalized = Dictionary(uniqueKeysWithValues: headers.enumerated().map { (Self.normalize($0.element), $0.offset) })
        var indexes: [MessageColumn: Int] = [:]
        for (column, name) in names {
            if let index = headerByNormalized[Self.normalize(name)] {
                indexes[column] = index
            }
        }
        return indexes
    }

    static func normalize(_ value: String) -> String {
        value.trimmingCharacters(in: .whitespacesAndNewlines)
            .lowercased()
            .replacingOccurrences(of: " ", with: "_")
            .replacingOccurrences(of: "-", with: "_")
    }
}
