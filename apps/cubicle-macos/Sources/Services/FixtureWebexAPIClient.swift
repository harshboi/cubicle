import Foundation

/// File-backed Webex client used by JSON-config test mode.
final class FixtureWebexAPIClient: NativeWebexClienting {
    private let currentUserValue: WebexPerson?
    private let roomsValue: [WebexRoom]
    private let membershipsByRoomID: [String: [WebexMembership]]
    private let messagesByRoomID: [String: [WebexMessage]]
    private let directMessagesByLookup: [String: [WebexMessage]]
    private let messagesByID: [String: WebexMessage]

    init(fixtureURL: URL) throws {
        let data = try Data(contentsOf: fixtureURL)
        let payload: FixtureWebexPayload
        do {
            payload = try JSONDecoder().decode(FixtureWebexPayload.self, from: data)
        } catch {
            throw WebexAPIError.unexpectedResponse(
                "Could not decode Webex fixture \(fixtureURL.path): \(error.localizedDescription)"
            )
        }
        currentUserValue = payload.currentUser
        roomsValue = payload.rooms
        membershipsByRoomID = payload.membershipsByRoomID
        messagesByRoomID = payload.messagesByRoomID.mapValues(Self.newestFirst)
        directMessagesByLookup = payload.directMessagesByLookup.mapValues(Self.newestFirst)

        var indexedMessages = payload.messagesByID
        for message in payload.messagesByRoomID.values.flatMap({ $0 }) {
            indexedMessages[message.id] = message
        }
        for message in payload.directMessagesByLookup.values.flatMap({ $0 }) {
            indexedMessages[message.id] = message
        }
        messagesByID = indexedMessages
    }

    func currentUser() async throws -> WebexPerson {
        guard let currentUserValue else {
            throw WebexAPIError.unexpectedResponse("Webex fixture is missing current_user.")
        }
        return currentUserValue
    }

    func rooms() async throws -> [WebexRoom] {
        roomsValue
    }

    func room(id roomID: String) async throws -> WebexRoom {
        let normalizedRoomID = roomID.trimmingCharacters(in: .whitespacesAndNewlines)
        guard !normalizedRoomID.isEmpty else {
            throw WebexAPIError.unexpectedResponse("roomID is required for room lookup.")
        }
        guard let room = roomsValue.first(where: { $0.id == normalizedRoomID }) else {
            throw WebexAPIError.httpStatus(code: 404, detail: "Fixture room not found: \(normalizedRoomID)")
        }
        return room
    }

    func memberships(roomID: String) async throws -> [WebexMembership] {
        membershipsByRoomID[roomID.trimmingCharacters(in: .whitespacesAndNewlines)] ?? []
    }

    func messages(roomID: String, max: Int) async throws -> [WebexMessage] {
        let normalizedRoomID = roomID.trimmingCharacters(in: .whitespacesAndNewlines)
        let cappedMax = min(Swift.max(1, max), 5_000)
        return Array((messagesByRoomID[normalizedRoomID] ?? []).prefix(cappedMax))
    }

    func messagesAfter(
        roomID: String,
        lastMessageID: String,
        lastMessageCreated: String,
        max: Int
    ) async throws -> [WebexMessage] {
        let normalizedLastMessageID = lastMessageID.trimmingCharacters(in: .whitespacesAndNewlines)
        let cutoff = Self.parsedDate(lastMessageCreated)
        var collected: [WebexMessage] = []
        for message in try await messages(roomID: roomID, max: max) {
            if !normalizedLastMessageID.isEmpty, message.id == normalizedLastMessageID {
                break
            }
            if let cutoff,
               let createdAt = Self.parsedDate(message.created),
               createdAt < cutoff {
                break
            }
            collected.append(message)
            if collected.count >= max {
                break
            }
        }
        return Array(collected.reversed())
    }

    func fetchLatestMessage(roomID: String) async throws -> WebexMessage? {
        try await messages(roomID: roomID, max: 1).first
    }

    func fetchRecentMessages(roomID: String, max: Int) async throws -> [WebexMessage] {
        try await messages(roomID: roomID, max: max)
    }

    func fetchMessage(messageID: String) async throws -> WebexMessage {
        let normalizedMessageID = messageID.trimmingCharacters(in: .whitespacesAndNewlines)
        guard let message = messagesByID[normalizedMessageID] else {
            throw WebexAPIError.httpStatus(code: 404, detail: "Fixture message not found: \(normalizedMessageID)")
        }
        return message
    }

    func fetchDirectMessages(personEmail: String?, personID: String?, max: Int) async throws -> [WebexMessage] {
        let cappedMax = min(Swift.max(1, max), 500)
        let lookupKeys = Self.directLookupKeys(personEmail: personEmail, personID: personID)
        for key in lookupKeys {
            if let messages = directMessagesByLookup[key] {
                return Array(messages.prefix(cappedMax))
            }
        }

        let email = personEmail?.trimmingCharacters(in: .whitespacesAndNewlines).lowercased() ?? ""
        let personID = personID?.trimmingCharacters(in: .whitespacesAndNewlines) ?? ""
        let matches = messagesByRoomID.values.flatMap { $0 }
            .filter { message in
                (!email.isEmpty && message.personEmail.trimmingCharacters(in: .whitespacesAndNewlines).lowercased() == email)
                    || (!personID.isEmpty && message.personID.trimmingCharacters(in: .whitespacesAndNewlines) == personID)
            }
        return Array(Self.newestFirst(matches).prefix(cappedMax))
    }

    private static func directLookupKeys(personEmail: String?, personID: String?) -> [String] {
        let email = personEmail?.trimmingCharacters(in: .whitespacesAndNewlines).lowercased() ?? ""
        if !email.isEmpty {
            return ["email:\(email)", email]
        }
        let personID = personID?.trimmingCharacters(in: .whitespacesAndNewlines) ?? ""
        guard !personID.isEmpty else { return [] }
        return ["person:\(personID)", personID]
    }

    private static func newestFirst(_ messages: [WebexMessage]) -> [WebexMessage] {
        messages.sorted { lhs, rhs in
            let lhsDate = parsedDate(lhs.created) ?? .distantPast
            let rhsDate = parsedDate(rhs.created) ?? .distantPast
            if lhsDate == rhsDate {
                return lhs.id > rhs.id
            }
            return lhsDate > rhsDate
        }
    }

    private static func parsedDate(_ value: String) -> Date? {
        let trimmed = value.trimmingCharacters(in: .whitespacesAndNewlines)
        guard !trimmed.isEmpty else { return nil }
        if let date = iso8601WithFractionalSeconds.date(from: trimmed) {
            return date
        }
        return iso8601.date(from: trimmed)
    }

    private static let iso8601WithFractionalSeconds: ISO8601DateFormatter = {
        let formatter = ISO8601DateFormatter()
        formatter.formatOptions = [.withInternetDateTime, .withFractionalSeconds]
        return formatter
    }()

    private static let iso8601: ISO8601DateFormatter = {
        let formatter = ISO8601DateFormatter()
        formatter.formatOptions = [.withInternetDateTime]
        return formatter
    }()
}

private struct FixtureWebexPayload: Decodable {
    var currentUser: WebexPerson?
    var rooms: [WebexRoom]
    var membershipsByRoomID: [String: [WebexMembership]]
    var messagesByRoomID: [String: [WebexMessage]]
    var directMessagesByLookup: [String: [WebexMessage]]
    var messagesByID: [String: WebexMessage]

    enum CodingKeys: String, CodingKey {
        case currentUser
        case currentUserSnake = "current_user"
        case rooms
        case memberships
        case messages
        case directMessages
        case directMessagesSnake = "direct_messages"
        case messagesByID
        case messagesByIDSnake = "messages_by_id"
    }

    init(from decoder: Decoder) throws {
        let container = try decoder.container(keyedBy: CodingKeys.self)
        currentUser = try container.decodeIfPresent(WebexPerson.self, forKey: .currentUser)
            ?? container.decodeIfPresent(WebexPerson.self, forKey: .currentUserSnake)
        rooms = try container.decodeIfPresent([WebexRoom].self, forKey: .rooms) ?? []
        membershipsByRoomID = try Self.decodeGroupedOrFlat(
            from: container,
            key: .memberships,
            groupingKey: \.roomID
        )
        messagesByRoomID = try Self.decodeGroupedOrFlat(
            from: container,
            key: .messages,
            groupingKey: \.roomID
        )
        let directMessages = try container.decodeIfPresent([String: [WebexMessage]].self, forKey: .directMessages)
            ?? container.decodeIfPresent([String: [WebexMessage]].self, forKey: .directMessagesSnake)
            ?? [:]
        directMessagesByLookup = directMessages.reduce(into: [:]) { result, entry in
            let key = Self.normalizedLookupKey(entry.key)
            result[key, default: []].append(contentsOf: entry.value)
        }
        messagesByID = try container.decodeIfPresent([String: WebexMessage].self, forKey: .messagesByID)
            ?? container.decodeIfPresent([String: WebexMessage].self, forKey: .messagesByIDSnake)
            ?? [:]
    }

    private static func decodeGroupedOrFlat<T: Decodable>(
        from container: KeyedDecodingContainer<CodingKeys>,
        key: CodingKeys,
        groupingKey: (T) -> String
    ) throws -> [String: [T]] {
        if let grouped = try container.decodeIfPresent([String: [T]].self, forKey: key) {
            return grouped
        }
        let flat = try container.decodeIfPresent([T].self, forKey: key) ?? []
        return Dictionary(grouping: flat) { groupingKey($0) }
    }

    private static func normalizedLookupKey(_ key: String) -> String {
        let trimmed = key.trimmingCharacters(in: .whitespacesAndNewlines)
        guard let separator = trimmed.firstIndex(of: ":") else {
            return trimmed.lowercased()
        }
        let prefix = trimmed[..<separator].lowercased()
        let valueStart = trimmed.index(after: separator)
        let value = trimmed[valueStart...].trimmingCharacters(in: .whitespacesAndNewlines)
        return "\(prefix):\(prefix == "email" ? value.lowercased() : value)"
    }
}
