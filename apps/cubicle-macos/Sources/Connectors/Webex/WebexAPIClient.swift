import Foundation
import MetaCodable

/// Room payload normalized from the Webex REST API.
struct WebexRoom: Identifiable, Hashable, Decodable {
    var id: String
    var title: String
    var type: String
    var lastActivity: String

    enum CodingKeys: String, CodingKey {
        case id
        case title
        case type
        case lastActivity
    }

    init(from decoder: Decoder) throws {
        let container = try decoder.container(keyedBy: CodingKeys.self)
        id = try container.decode(String.self, forKey: .id)
        title = try container.decodeIfPresent(String.self, forKey: .title) ?? ""
        type = try container.decodeIfPresent(String.self, forKey: .type) ?? ""
        lastActivity = try container.decodeIfPresent(String.self, forKey: .lastActivity) ?? ""
    }
}

/// Person payload used for self identity, members, and direct-message targets.
struct WebexPerson: Identifiable, Hashable, Decodable {
    var id: String
    var displayName: String
    var emails: [String]
}

/// Room membership payload used to resolve participants and aliases.
@Codable
struct WebexMembership: Identifiable, Hashable {
    var id: String
    @CodedAt("roomId")
    var roomID: String
    @CodedAt("personId")
    var personID: String
    var personEmail: String
    var personDisplayName: String
}

/// Message payload normalized across Webex `text` and `markdown` fields.
struct WebexMessage: Identifiable, Hashable, Decodable {
    var id: String
    var roomID: String
    var personID: String
    var personEmail: String
    var text: String
    var created: String

    enum CodingKeys: String, CodingKey {
        case id
        case roomID = "roomId"
        case personID = "personId"
        case personEmail
        case text
        case markdown
        case created
    }

    init(from decoder: Decoder) throws {
        let container = try decoder.container(keyedBy: CodingKeys.self)
        id = try container.decode(String.self, forKey: .id)
        roomID = try container.decode(String.self, forKey: .roomID)
        personID = try container.decodeIfPresent(String.self, forKey: .personID) ?? ""
        personEmail = try container.decodeIfPresent(String.self, forKey: .personEmail) ?? ""
        text = try container.decodeIfPresent(String.self, forKey: .text)
            ?? container.decodeIfPresent(String.self, forKey: .markdown)
            ?? ""
        created = try container.decodeIfPresent(String.self, forKey: .created) ?? ""
    }
}

/// Webex client failures mapped into sync-engine status categories.
enum WebexAPIError: LocalizedError {
    case missingAccessToken
    case invalidAccessToken(String)
    case invalidBaseURL(String)
    case invalidHTTPResponse(URL)
    case unexpectedResponse(String)
    case httpStatus(code: Int, detail: String, retryAfterSeconds: TimeInterval? = nil)
    case network(String)
    case exhaustedRetries

    var errorDescription: String? {
        switch self {
        case .missingAccessToken:
            return "No Webex access token is available to the native Swift client."
        case .invalidAccessToken(let detail):
            return detail
        case .invalidBaseURL(let value):
            return "Invalid Webex API base URL: \(value)"
        case .invalidHTTPResponse(let url):
            return "Webex API returned a non-HTTP response for \(url.absoluteString)"
        case .unexpectedResponse(let detail):
            return detail
        case .httpStatus(let code, let detail, let retryAfterSeconds):
            if let retryAfterSeconds, retryAfterSeconds > 0 {
                return "Webex API request failed with HTTP \(code): \(detail) (retry after \(Int(retryAfterSeconds.rounded(.up)))s)"
            }
            return "Webex API request failed with HTTP \(code): \(detail)"
        case .network(let detail):
            return detail
        case .exhaustedRetries:
            return "Webex API request failed after exhausting configured retries."
        }
    }
}

/// Thin async Webex REST client with token reload, pagination, and retry handling.
final class WebexAPIClient {
    let configuration: RuntimeConfiguration
    private let urlSession: URLSession
    private let configStore: ConfigStore
    private let decoder = JSONDecoder()

    /// Allows tests to inject a session/config store while production uses app settings.
    init(
        configuration: RuntimeConfiguration = .current,
        configStore: ConfigStore? = nil,
        urlSession: URLSession = .shared
    ) {
        self.configuration = configuration
        self.configStore = configStore ?? ConfigStore(configuration: configuration)
        self.urlSession = urlSession
    }

    /// Fetches the authenticated Webex user.
    func currentUser() async throws -> WebexPerson {
        let url = try buildURL(pathOrURL: "/people/me")
        let (data, _) = try await requestData(url: url, method: "GET")
        do {
            return try decoder.decode(WebexPerson.self, from: data)
        } catch {
            throw WebexAPIError.unexpectedResponse(
                "Unexpected response for GET /people/me: \(error.localizedDescription)"
            )
        }
    }

    /// Lists all visible rooms using Webex pagination.
    func rooms() async throws -> [WebexRoom] {
        try await fetchPaginated(
            path: "/rooms",
            queryItems: [URLQueryItem(name: "max", value: String(configuration.webexPageSize))]
        )
    }

    /// Fetches one room by ID.
    func room(id roomID: String) async throws -> WebexRoom {
        let normalizedRoomID = roomID.trimmingCharacters(in: .whitespacesAndNewlines)
        guard !normalizedRoomID.isEmpty else {
            throw WebexAPIError.unexpectedResponse("roomID is required for room lookup.")
        }

        let url = try buildURL(pathOrURL: "/rooms/\(encodedPathSegment(normalizedRoomID))")
        let (data, _) = try await requestData(url: url, method: "GET")
        do {
            return try decoder.decode(WebexRoom.self, from: data)
        } catch {
            throw WebexAPIError.unexpectedResponse(
                "Unexpected response for GET /rooms/{roomId}: \(error.localizedDescription)"
            )
        }
    }

    /// Lists memberships for a room.
    func memberships(roomID: String) async throws -> [WebexMembership] {
        let normalizedRoomID = roomID.trimmingCharacters(in: .whitespacesAndNewlines)
        guard !normalizedRoomID.isEmpty else {
            throw WebexAPIError.unexpectedResponse("roomID is required for memberships lookup.")
        }
        return try await fetchPaginated(
            path: "/memberships",
            queryItems: [
                URLQueryItem(name: "roomId", value: normalizedRoomID),
                URLQueryItem(name: "max", value: String(configuration.webexPageSize)),
            ]
        )
    }

    /// Loads recent room messages newest-first, bounded by `max`.
    func messages(roomID: String, before: Date? = nil, max: Int = 100) async throws -> [WebexMessage] {
        let normalizedRoomID = roomID.trimmingCharacters(in: .whitespacesAndNewlines)
        guard !normalizedRoomID.isEmpty else {
            throw WebexAPIError.unexpectedResponse("roomID is required for message ingestion.")
        }

        let cappedMax = min(Swift.max(1, max), 5_000)
        var collected: [WebexMessage] = []
        var nextPageURL: URL? = try buildURL(
            pathOrURL: "/messages",
            queryItems: messageQueryItems(roomID: normalizedRoomID, before: before, max: cappedMax)
        )
        var visitedPageURLs = Set<String>()

        while let pageURL = nextPageURL, collected.count < cappedMax {
            let pageKey = pageURL.absoluteString
            guard visitedPageURLs.insert(pageKey).inserted else {
                throw WebexAPIError.unexpectedResponse("Detected pagination loop for \(pageKey)")
            }

            let (data, response) = try await requestData(url: pageURL, method: "GET")
            let page: PaginatedResponse<WebexMessage>
            do {
                page = try decoder.decode(PaginatedResponse<WebexMessage>.self, from: data)
            } catch {
                throw WebexAPIError.unexpectedResponse(
                    "Expected paginated JSON object with 'items' for \(pageURL.absoluteString)"
                )
            }

            if page.items.isEmpty {
                break
            }

            let remaining = cappedMax - collected.count
            if page.items.count > remaining {
                collected.append(contentsOf: page.items.prefix(remaining))
                break
            }

            collected.append(contentsOf: page.items)
            nextPageURL = self.nextURL(from: response)
        }

        return collected
    }

    /// Loads messages newer than the stored watermark and returns them oldest-first.
    func messagesAfter(
        roomID: String,
        lastMessageID: String = "",
        lastMessageCreated: String = "",
        max: Int = 100
    ) async throws -> [WebexMessage] {
        let normalizedRoomID = roomID.trimmingCharacters(in: .whitespacesAndNewlines)
        guard !normalizedRoomID.isEmpty else {
            throw WebexAPIError.unexpectedResponse("roomID is required for incremental message ingestion.")
        }

        let normalizedLastMessageID = lastMessageID.trimmingCharacters(in: .whitespacesAndNewlines)
        let cappedMax = min(Swift.max(1, max), 5_000)
        let cutoff = parseTimestamp(lastMessageCreated)
        var collected: [WebexMessage] = []
        var nextPageURL: URL? = try buildURL(
            pathOrURL: "/messages",
            queryItems: [
                URLQueryItem(name: "roomId", value: normalizedRoomID),
                URLQueryItem(name: "max", value: String(configuration.webexPageSize)),
            ]
        )
        var visitedPageURLs = Set<String>()

        while let pageURL = nextPageURL, collected.count < cappedMax {
            let pageKey = pageURL.absoluteString
            guard visitedPageURLs.insert(pageKey).inserted else {
                throw WebexAPIError.unexpectedResponse("Detected pagination loop for \(pageKey)")
            }

            let (data, response) = try await requestData(url: pageURL, method: "GET")
            let page: PaginatedResponse<WebexMessage>
            do {
                page = try decoder.decode(PaginatedResponse<WebexMessage>.self, from: data)
            } catch {
                throw WebexAPIError.unexpectedResponse(
                    "Expected paginated JSON object with 'items' for \(pageURL.absoluteString)"
                )
            }

            if page.items.isEmpty {
                break
            }

            var shouldStopPaging = false
            for item in page.items {
                if !normalizedLastMessageID.isEmpty && item.id == normalizedLastMessageID {
                    shouldStopPaging = true
                    break
                }

                if let cutoff, let createdAt = parseTimestamp(item.created), createdAt < cutoff {
                    shouldStopPaging = true
                    break
                }

                collected.append(item)
                if collected.count >= cappedMax {
                    shouldStopPaging = true
                    break
                }
            }

            if shouldStopPaging {
                break
            }
            nextPageURL = self.nextURL(from: response)
        }

        collected.reverse()
        return collected
    }

    /// Fetches the newest message for activity probing.
    func latestMessage(roomID: String) async throws -> WebexMessage? {
        let messages = try await messages(roomID: roomID, max: 1)
        return messages.first
    }

    /// Fetches one message by ID.
    func message(id messageID: String) async throws -> WebexMessage {
        let normalizedMessageID = messageID.trimmingCharacters(in: .whitespacesAndNewlines)
        guard !normalizedMessageID.isEmpty else {
            throw WebexAPIError.unexpectedResponse("messageID is required for message lookup.")
        }
        let url = try buildURL(pathOrURL: "/messages/\(encodedPathSegment(normalizedMessageID))")
        let (data, _) = try await requestData(url: url, method: "GET")
        do {
            return try decoder.decode(WebexMessage.self, from: data)
        } catch {
            throw WebexAPIError.unexpectedResponse(
                "Unexpected response for GET /messages/{messageId}: \(error.localizedDescription)"
            )
        }
    }

    /// Fetches direct messages by email or person ID.
    func directMessages(
        personEmail: String? = nil,
        personID: String? = nil,
        max: Int = 100
    ) async throws -> [WebexMessage] {
        let normalizedEmail = personEmail?.trimmingCharacters(in: .whitespacesAndNewlines).lowercased() ?? ""
        let normalizedPersonID = personID?.trimmingCharacters(in: .whitespacesAndNewlines) ?? ""
        guard !normalizedEmail.isEmpty || !normalizedPersonID.isEmpty else {
            throw WebexAPIError.unexpectedResponse("personEmail or personID is required for direct messages lookup.")
        }

        let cappedMax = min(Swift.max(1, max), 500)
        var queryItems: [URLQueryItem] = [URLQueryItem(name: "max", value: String(cappedMax))]
        if !normalizedEmail.isEmpty {
            queryItems.append(URLQueryItem(name: "personEmail", value: normalizedEmail))
        } else {
            queryItems.append(URLQueryItem(name: "personId", value: normalizedPersonID))
        }

        let url = try buildURL(pathOrURL: "/messages/direct", queryItems: queryItems)
        let (data, _) = try await requestData(url: url, method: "GET")
        do {
            let page = try decoder.decode(PaginatedResponse<WebexMessage>.self, from: data)
            return page.items
        } catch {
            throw WebexAPIError.unexpectedResponse(
                "Expected paginated JSON object with 'items' for direct messages: \(error.localizedDescription)"
            )
        }
    }

    /// Follows Webex `Link` pagination while detecting loops.
    private func fetchPaginated<T: Decodable>(
        path: String,
        queryItems: [URLQueryItem]
    ) async throws -> [T] {
        var results: [T] = []
        var nextPageURL: URL? = try buildURL(pathOrURL: path, queryItems: queryItems)
        var visitedPageURLs = Set<String>()

        while let pageURL = nextPageURL {
            let pageKey = pageURL.absoluteString
            guard visitedPageURLs.insert(pageKey).inserted else {
                throw WebexAPIError.unexpectedResponse("Detected pagination loop for \(pageKey)")
            }

            let (data, response) = try await requestData(url: pageURL, method: "GET")
            let page: PaginatedResponse<T>
            do {
                page = try decoder.decode(PaginatedResponse<T>.self, from: data)
            } catch {
                throw WebexAPIError.unexpectedResponse(
                    "Expected paginated JSON object with 'items' for \(pageURL.absoluteString)"
                )
            }

            results.append(contentsOf: page.items)
            nextPageURL = nextURL(from: response)
        }

        return results
    }

    /// Performs an authenticated request with token reload and retry/backoff.
    private func requestData(
        url: URL,
        method: String,
        body: Data? = nil
    ) async throws -> (Data, HTTPURLResponse) {
        let retries = max(0, configuration.webexRetryCount)
        var shouldReloadToken = false

        for attempt in 0...retries {
            let token: String
            do {
                token = try loadAccessToken(forceReload: shouldReloadToken)
            } catch {
                throw WebexAPIError.invalidAccessToken(error.localizedDescription)
            }

            var request = URLRequest(url: url)
            request.httpMethod = method
            request.timeoutInterval = configuration.webexTimeoutSeconds
            request.setValue("Bearer \(token)", forHTTPHeaderField: "Authorization")
            request.setValue("application/json", forHTTPHeaderField: "Accept")

            if let body {
                request.httpBody = body
                request.setValue("application/json", forHTTPHeaderField: "Content-Type")
            }

            do {
                let (data, response) = try await urlSession.data(for: request)
                guard let httpResponse = response as? HTTPURLResponse else {
                    throw WebexAPIError.invalidHTTPResponse(url)
                }

                if (200..<300).contains(httpResponse.statusCode) {
                    return (data, httpResponse)
                }

                if httpResponse.statusCode == 401 && !shouldReloadToken {
                    shouldReloadToken = true
                    continue
                }

                if isRetriableStatus(httpResponse.statusCode), attempt < retries {
                    let delay = retryDelaySeconds(attempt: attempt, response: httpResponse)
                    try await sleep(seconds: delay)
                    continue
                }

                throw WebexAPIError.httpStatus(
                    code: httpResponse.statusCode,
                    detail: decodeErrorBody(data) ?? HTTPURLResponse.localizedString(forStatusCode: httpResponse.statusCode),
                    retryAfterSeconds: retryAfterDelaySeconds(httpResponse) ?? rateLimitResetDelaySeconds(httpResponse)
                )
            } catch let apiError as WebexAPIError {
                throw apiError
            } catch let urlError as URLError {
                if attempt < retries {
                    let delay = retryDelaySeconds(attempt: attempt, response: nil)
                    try await sleep(seconds: delay)
                    continue
                }
                throw WebexAPIError.network("Network error calling Webex API: \(urlError.localizedDescription)")
            } catch {
                if attempt < retries {
                    let delay = retryDelaySeconds(attempt: attempt, response: nil)
                    try await sleep(seconds: delay)
                    continue
                }
                throw WebexAPIError.network("Failed calling Webex API: \(error.localizedDescription)")
            }
        }

        throw WebexAPIError.exhaustedRetries
    }

    private func loadAccessToken(forceReload: Bool) throws -> String {
        let health = configStore.oauthTokenHealth()
        switch health.state {
        case .invalidTokenFile:
            if let parseError = health.parseError, !parseError.isEmpty {
                throw WebexAPIError.invalidAccessToken(parseError)
            }
            throw WebexAPIError.invalidAccessToken("OAuth token file is invalid.")
        case .missingTokenFile, .missingAccessToken:
            throw WebexAPIError.missingAccessToken
        case .missingRefreshToken:
            throw WebexAPIError.invalidAccessToken(
                "Webex access token is expired and no refresh token is available."
            )
        case .refreshExpired:
            throw WebexAPIError.invalidAccessToken(
                "Webex access token is expired and the refresh token is also expired."
            )
        case .expired, .expiringSoon, .refreshExpiringSoon, .healthy, .unknownExpiry:
            break
        }

        if !forceReload,
           let tokenFromHealth = health.record?.accessToken.trimmingCharacters(in: .whitespacesAndNewlines),
           !tokenFromHealth.isEmpty {
            return tokenFromHealth
        }

        let token = try configStore.webexAccessToken().trimmingCharacters(in: .whitespacesAndNewlines)
        guard !token.isEmpty else {
            throw WebexAPIError.missingAccessToken
        }
        return token
    }

    private func buildURL(pathOrURL: String, queryItems: [URLQueryItem] = []) throws -> URL {
        if let absolute = URL(string: pathOrURL), absolute.scheme != nil {
            guard var components = URLComponents(url: absolute, resolvingAgainstBaseURL: false) else {
                throw WebexAPIError.invalidBaseURL(pathOrURL)
            }
            components.queryItems = mergeQueryItems(existing: components.queryItems, updates: queryItems)
            guard let url = components.url else {
                throw WebexAPIError.invalidBaseURL(pathOrURL)
            }
            return url
        }

        let base = configuration.webexBaseURL.absoluteString.trimmingCharacters(in: CharacterSet(charactersIn: "/"))
        let normalizedPath = pathOrURL.hasPrefix("/") ? String(pathOrURL.dropFirst()) : pathOrURL
        guard var components = URLComponents(string: "\(base)/\(normalizedPath)") else {
            throw WebexAPIError.invalidBaseURL(configuration.webexBaseURL.absoluteString)
        }
        components.queryItems = mergeQueryItems(existing: components.queryItems, updates: queryItems)
        guard let url = components.url else {
            throw WebexAPIError.invalidBaseURL(configuration.webexBaseURL.absoluteString)
        }
        return url
    }

    private func encodedPathSegment(_ value: String) -> String {
        var allowedCharacters = CharacterSet.urlPathAllowed
        allowedCharacters.remove(charactersIn: "/")
        return value.addingPercentEncoding(withAllowedCharacters: allowedCharacters) ?? value
    }

    private func messageQueryItems(roomID: String, before: Date?, max: Int) -> [URLQueryItem] {
        var queryItems: [URLQueryItem] = [
            URLQueryItem(name: "roomId", value: roomID),
            URLQueryItem(name: "max", value: String(min(configuration.webexPageSize, max))),
        ]
        if let before {
            queryItems.append(
                URLQueryItem(
                    name: "before",
                    value: WebexAPIClient.iso8601WithFractionalSeconds.string(from: before)
                )
            )
        }
        return queryItems
    }

    private func nextURL(from response: HTTPURLResponse) -> URL? {
        guard let linkHeader = response.value(forHTTPHeaderField: "Link") else {
            return nil
        }
        let links = parseLinkHeader(linkHeader)
        guard let next = links["next"] else {
            return nil
        }
        if let absolute = URL(string: next), absolute.scheme != nil {
            return absolute
        }
        return URL(string: next, relativeTo: response.url)?.absoluteURL
    }

    private func parseLinkHeader(_ linkHeader: String) -> [String: String] {
        var result: [String: String] = [:]
        let entries = linkHeader.split(separator: ",", omittingEmptySubsequences: true)

        for entry in entries {
            let segment = entry.trimmingCharacters(in: .whitespacesAndNewlines)
            guard let urlStart = segment.firstIndex(of: "<"),
                  let urlEnd = segment[urlStart...].firstIndex(of: ">"),
                  urlStart < urlEnd else {
                continue
            }

            let url = String(segment[segment.index(after: urlStart)..<urlEnd])
            let parameterStart = segment.index(after: urlEnd)
            let parameters = segment[parameterStart...].split(separator: ";", omittingEmptySubsequences: true)

            for rawParameter in parameters {
                let parameter = rawParameter.trimmingCharacters(in: .whitespacesAndNewlines)
                guard parameter.lowercased().hasPrefix("rel=") else {
                    continue
                }

                var relationValue = String(parameter.dropFirst(4)).trimmingCharacters(in: .whitespacesAndNewlines)
                if relationValue.hasPrefix("\""), relationValue.hasSuffix("\""), relationValue.count >= 2 {
                    relationValue.removeFirst()
                    relationValue.removeLast()
                }

                for relation in relationValue.split(separator: " ", omittingEmptySubsequences: true) {
                    result[String(relation).lowercased()] = url
                }
            }
        }

        return result
    }

    private func mergeQueryItems(
        existing: [URLQueryItem]?,
        updates: [URLQueryItem]
    ) -> [URLQueryItem]? {
        guard !updates.isEmpty else { return existing }
        var merged = existing ?? []
        for update in updates {
            if let index = merged.firstIndex(where: { $0.name == update.name }) {
                merged[index] = update
            } else {
                merged.append(update)
            }
        }
        return merged
    }

    private func retryDelaySeconds(
        attempt: Int,
        response: HTTPURLResponse?
    ) -> TimeInterval {
        if let retryAfterDelay = retryAfterDelaySeconds(response) {
            return retryAfterDelay
        }
        if let resetDelay = rateLimitResetDelaySeconds(response) {
            return resetDelay
        }
        return TimeInterval(min(1 << min(attempt, 4), 30))
    }

    private func retryAfterDelaySeconds(_ response: HTTPURLResponse?) -> TimeInterval? {
        guard let retryAfter = response?.value(forHTTPHeaderField: "Retry-After") else {
            return nil
        }
        let normalized = retryAfter.trimmingCharacters(in: .whitespacesAndNewlines)
        if let seconds = TimeInterval(normalized), seconds > 0 {
            return min(seconds, 300)
        }
        if let retryDate = WebexAPIClient.httpDateFormatter.date(from: normalized) {
            let seconds = retryDate.timeIntervalSinceNow
            if seconds > 0 {
                return min(seconds, 300)
            }
        }
        return nil
    }

    private func rateLimitResetDelaySeconds(_ response: HTTPURLResponse?) -> TimeInterval? {
        guard let rawReset = response?.value(forHTTPHeaderField: "X-RateLimit-Reset") else {
            return nil
        }

        let normalized = rawReset.trimmingCharacters(in: .whitespacesAndNewlines)
        guard let numeric = Double(normalized), numeric > 0 else {
            return nil
        }

        // Accept both epoch seconds and epoch milliseconds.
        let epochSeconds = numeric > 4_102_444_800 ? numeric / 1_000 : numeric
        let delay = epochSeconds - Date().timeIntervalSince1970
        guard delay > 0 else {
            return nil
        }
        return min(delay, 300)
    }

    private func sleep(seconds: TimeInterval) async throws {
        let nanoseconds = UInt64(max(0, seconds) * 1_000_000_000)
        try await Task.sleep(nanoseconds: nanoseconds)
    }

    private func isRetriableStatus(_ statusCode: Int) -> Bool {
        statusCode == 408
            || statusCode == 429
            || statusCode == 500
            || statusCode == 502
            || statusCode == 503
            || statusCode == 504
    }

    private func decodeErrorBody(_ data: Data) -> String? {
        guard !data.isEmpty else { return nil }
        if let payload = try? JSONSerialization.jsonObject(with: data) as? [String: Any] {
            if let message = payload["message"] as? String, !message.isEmpty {
                return message
            }
            if let errors = payload["errors"] {
                return "details=\(errors)"
            }
        }
        return String(data: data, encoding: .utf8)?.trimmingCharacters(in: .whitespacesAndNewlines)
    }

    private func parseTimestamp(_ value: String) -> Date? {
        let normalized = value.trimmingCharacters(in: .whitespacesAndNewlines)
        guard !normalized.isEmpty else { return nil }
        if let withFractionalSeconds = WebexAPIClient.iso8601WithFractionalSeconds.date(from: normalized) {
            return withFractionalSeconds
        }
        return WebexAPIClient.iso8601.date(from: normalized)
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

    private static let httpDateFormatter: DateFormatter = {
        let formatter = DateFormatter()
        formatter.locale = Locale(identifier: "en_US_POSIX")
        formatter.timeZone = TimeZone(secondsFromGMT: 0)
        formatter.dateFormat = "EEE, dd MMM yyyy HH:mm:ss zzz"
        return formatter
    }()
}

/// Standard Webex paginated response wrapper.
private struct PaginatedResponse<T: Decodable>: Decodable {
    let items: [T]
}
