import AppKit
import CryptoKit
import Darwin
import Foundation
import Security

enum OAuthServiceError: LocalizedError {
    case missingConfiguration(String)
    case invalidRedirectURI(String)
    case browserOpenFailed(URL)
    case callbackTimeout(provider: String)
    case callbackFailed(String)
    case tokenExchangeFailed(provider: String, detail: String)

    var errorDescription: String? {
        switch self {
        case .missingConfiguration(let detail):
            return detail
        case .invalidRedirectURI(let value):
            return "Invalid OAuth redirect URI: \(value)"
        case .browserOpenFailed(let url):
            return "Could not open OAuth authorization URL: \(url.absoluteString)"
        case .callbackTimeout(let provider):
            return "\(provider) OAuth timed out waiting for the browser callback."
        case .callbackFailed(let detail):
            return detail
        case .tokenExchangeFailed(let provider, let detail):
            return "\(provider) OAuth token exchange failed: \(detail)"
        }
    }
}

struct OAuthAuthorizationOutcome: Equatable {
    var provider: OAuthProviderKind
    var tokenFile: URL
}

final class OAuthService {
    private let configuration: RuntimeConfiguration
    private let configStore: ConfigStore
    private let urlSession: URLSession

    init(
        configuration: RuntimeConfiguration = .current,
        configStore: ConfigStore? = nil,
        urlSession: URLSession = .shared
    ) {
        self.configuration = configuration
        self.configStore = configStore ?? ConfigStore(configuration: configuration)
        self.urlSession = urlSession
    }

    func authorize(provider: OAuthProviderKind) async throws -> OAuthAuthorizationOutcome {
        let providerConfig = try OAuthProviderConfiguration(provider: provider, configStore: configStore)
        let verifier = Self.randomOAuthToken(byteCount: 72)
        let state = Self.randomOAuthToken(byteCount: 48)
        let callbackServer = try OAuthCallbackServer(
            redirectURI: providerConfig.redirectURI,
            expectedState: state,
            providerName: provider.displayName
        )
        let authorizeURL = try authorizationURL(
            providerConfig: providerConfig,
            state: state,
            codeChallenge: Self.pkceCodeChallenge(verifier)
        )

        let code = try await withThrowingTaskGroup(of: String.self) { group in
            group.addTask {
                try await callbackServer.waitForCode()
            }
            group.addTask {
                try await Task.sleep(nanoseconds: UInt64(providerConfig.timeoutSeconds) * 1_000_000_000)
                throw OAuthServiceError.callbackTimeout(provider: provider.displayName)
            }

            try await Task.sleep(nanoseconds: 150_000_000)
            guard NSWorkspace.shared.open(authorizeURL) else {
                callbackServer.cancel()
                throw OAuthServiceError.browserOpenFailed(authorizeURL)
            }

            guard let result = try await group.next() else {
                callbackServer.cancel()
                throw OAuthServiceError.callbackFailed("\(provider.displayName) OAuth callback did not return a code.")
            }
            group.cancelAll()
            callbackServer.cancel()
            return result
        }

        let payload = try await exchangeCodeForToken(
            providerConfig: providerConfig,
            code: code,
            verifier: verifier
        )
        let tokenFile = try configStore.saveOAuthTokenPayload(payload, provider: provider)
        return OAuthAuthorizationOutcome(provider: provider, tokenFile: tokenFile)
    }

    @discardableResult
    func revoke(provider: OAuthProviderKind) throws -> [URL] {
        try configStore.deleteOAuthTokenFiles(provider: provider)
    }

    private func authorizationURL(
        providerConfig: OAuthProviderConfiguration,
        state: String,
        codeChallenge: String
    ) throws -> URL {
        var components = URLComponents(url: providerConfig.authorizeURL, resolvingAgainstBaseURL: false)
        components?.queryItems = [
            URLQueryItem(name: "response_type", value: "code"),
            URLQueryItem(name: "client_id", value: providerConfig.clientID),
            URLQueryItem(name: "redirect_uri", value: providerConfig.redirectURI.absoluteString),
            URLQueryItem(name: "scope", value: providerConfig.scope),
            URLQueryItem(name: "state", value: state),
            URLQueryItem(name: "code_challenge", value: codeChallenge),
            URLQueryItem(name: "code_challenge_method", value: "S256")
        ]
        guard let url = components?.url else {
            throw OAuthServiceError.invalidRedirectURI(providerConfig.redirectURI.absoluteString)
        }
        return url
    }

    private func exchangeCodeForToken(
        providerConfig: OAuthProviderConfiguration,
        code: String,
        verifier: String
    ) async throws -> [String: Any] {
        var formFields: [String: String] = [
            "grant_type": "authorization_code",
            "client_id": providerConfig.clientID,
            "code": code,
            "redirect_uri": providerConfig.redirectURI.absoluteString,
            "code_verifier": verifier
        ]
        if let clientSecret = providerConfig.clientSecret, !clientSecret.isEmpty {
            formFields["client_secret"] = clientSecret
        }

        var request = URLRequest(url: providerConfig.tokenURL)
        request.httpMethod = "POST"
        request.setValue("application/json", forHTTPHeaderField: "Accept")
        request.setValue("application/x-www-form-urlencoded", forHTTPHeaderField: "Content-Type")
        request.timeoutInterval = TimeInterval(providerConfig.timeoutSeconds)
        request.httpBody = Self.formURLEncoded(formFields).data(using: .utf8)

        let data: Data
        let response: URLResponse
        do {
            (data, response) = try await urlSession.data(for: request)
        } catch {
            throw OAuthServiceError.tokenExchangeFailed(
                provider: providerConfig.provider.displayName,
                detail: error.localizedDescription
            )
        }

        guard let httpResponse = response as? HTTPURLResponse else {
            throw OAuthServiceError.tokenExchangeFailed(
                provider: providerConfig.provider.displayName,
                detail: "Token endpoint returned a non-HTTP response."
            )
        }
        guard 200..<300 ~= httpResponse.statusCode else {
            let detail = String(data: data, encoding: .utf8) ?? "HTTP \(httpResponse.statusCode)"
            throw OAuthServiceError.tokenExchangeFailed(
                provider: providerConfig.provider.displayName,
                detail: detail
            )
        }

        guard var payload = try JSONSerialization.jsonObject(with: data) as? [String: Any] else {
            throw OAuthServiceError.tokenExchangeFailed(
                provider: providerConfig.provider.displayName,
                detail: "Token endpoint did not return a JSON object."
            )
        }
        payload["obtained_at"] = Self.iso8601String(from: Date())
        if payload["scope"] == nil {
            payload["scope"] = providerConfig.scope
        }
        if let expiresIn = Self.intValue(payload["expires_in"]) {
            payload["access_token_expires_at"] = Self.iso8601String(
                from: Date().addingTimeInterval(TimeInterval(expiresIn))
            )
        }
        if let refreshExpiresIn = Self.intValue(payload["refresh_token_expires_in"]) {
            payload["refresh_token_expires_at"] = Self.iso8601String(
                from: Date().addingTimeInterval(TimeInterval(refreshExpiresIn))
            )
        }
        return payload
    }

    private static func randomOAuthToken(byteCount: Int) -> String {
        var bytes = [UInt8](repeating: 0, count: byteCount)
        _ = SecRandomCopyBytes(kSecRandomDefault, bytes.count, &bytes)
        return Data(bytes).base64EncodedString()
            .replacingOccurrences(of: "+", with: "-")
            .replacingOccurrences(of: "/", with: "_")
            .replacingOccurrences(of: "=", with: "")
    }

    private static func pkceCodeChallenge(_ verifier: String) -> String {
        let digest = SHA256.hash(data: Data(verifier.utf8))
        return Data(digest).base64EncodedString()
            .replacingOccurrences(of: "+", with: "-")
            .replacingOccurrences(of: "/", with: "_")
            .replacingOccurrences(of: "=", with: "")
    }

    private static func formURLEncoded(_ fields: [String: String]) -> String {
        fields
            .sorted { $0.key < $1.key }
            .map { key, value in
                "\(percentEncode(key))=\(percentEncode(value))"
            }
            .joined(separator: "&")
    }

    private static func percentEncode(_ value: String) -> String {
        var allowed = CharacterSet.urlQueryAllowed
        allowed.remove(charactersIn: ":#[]@!$&'()*+,;=")
        return value.addingPercentEncoding(withAllowedCharacters: allowed) ?? value
    }

    private static func intValue(_ value: Any?) -> Int? {
        switch value {
        case let intValue as Int:
            return intValue
        case let number as NSNumber:
            return number.intValue
        case let string as String:
            return Int(string.trimmingCharacters(in: .whitespacesAndNewlines))
        default:
            return nil
        }
    }

    private static func iso8601String(from date: Date) -> String {
        let formatter = ISO8601DateFormatter()
        formatter.formatOptions = [.withInternetDateTime, .withFractionalSeconds]
        return formatter.string(from: date)
    }
}

private struct OAuthProviderConfiguration {
    var provider: OAuthProviderKind
    var clientID: String
    var clientSecret: String?
    var redirectURI: URL
    var scope: String
    var authorizeURL: URL
    var tokenURL: URL
    var timeoutSeconds: Int

    init(provider: OAuthProviderKind, configStore: ConfigStore) throws {
        let environment = ProcessInfo.processInfo.environment
        let appSettings = try configStore.oauthAppSettings(provider: provider)
        self.provider = provider
        switch provider {
        case .webex:
            clientID = try Self.requiredSetting(
                envNames: provider.oauthClientIDSettingKeys,
                fileValue: appSettings.clientID,
                provider: provider,
                settingName: "client id",
                configURL: configStore.oauthSettingsURL
            )
            clientSecret = try Self.requiredSetting(
                envNames: provider.oauthClientSecretSettingKeys,
                fileValue: appSettings.clientSecret,
                provider: provider,
                settingName: "client secret",
                configURL: configStore.oauthSettingsURL
            )
            redirectURI = try Self.redirectURI(
                Self.setting(envNames: provider.oauthRedirectURISettingKeys, fileValue: appSettings.redirectURI)
                    ?? "http://127.0.0.1:8787/callback"
            )
            scope = Self.setting(envNames: provider.oauthScopeSettingKeys, fileValue: appSettings.scope)
                ?? "spark:rooms_read spark:messages_read spark:messages_write spark:memberships_read spark:people_read"
            authorizeURL = URL(string: "https://webexapis.com/v1/authorize")!
            tokenURL = URL(string: "https://webexapis.com/v1/access_token")!
        case .outlook:
            clientID = try Self.requiredSetting(
                envNames: provider.oauthClientIDSettingKeys,
                fileValue: appSettings.clientID,
                provider: provider,
                settingName: "client id",
                configURL: configStore.oauthSettingsURL
            )
            clientSecret = Self.setting(
                envNames: provider.oauthClientSecretSettingKeys,
                fileValue: appSettings.clientSecret
            )
            redirectURI = try Self.redirectURI(
                Self.setting(envNames: provider.oauthRedirectURISettingKeys, fileValue: appSettings.redirectURI)
                    ?? "http://127.0.0.1:8788/callback"
            )
            scope = Self.setting(envNames: provider.oauthScopeSettingKeys, fileValue: appSettings.scope)
                ?? "offline_access User.Read Mail.Read Calendars.Read Files.Read.All Sites.Read.All"
            let tenant = Self.setting(envNames: provider.oauthTenantSettingKeys, fileValue: appSettings.tenant)
                ?? "common"
            authorizeURL = URL(string: "https://login.microsoftonline.com/\(tenant)/oauth2/v2.0/authorize")!
            tokenURL = URL(string: "https://login.microsoftonline.com/\(tenant)/oauth2/v2.0/token")!
        }
        timeoutSeconds = Int(environment["OAUTH_CALLBACK_TIMEOUT_SECONDS"] ?? "") ?? 300
    }

    private static func env(_ name: String) -> String? {
        let value = ProcessInfo.processInfo.environment[name]?.trimmingCharacters(in: .whitespacesAndNewlines)
        return value?.isEmpty == false ? value : nil
    }

    private static func setting(envNames: [String], fileValue: String?) -> String? {
        for name in envNames {
            if let value = env(name) {
                return value
            }
        }
        return fileValue
    }

    private static func requiredSetting(
        envNames: [String],
        fileValue: String?,
        provider: OAuthProviderKind,
        settingName: String,
        configURL: URL
    ) throws -> String {
        if let value = setting(envNames: envNames, fileValue: fileValue) {
            return value
        }
        let joined = envNames.joined(separator: " or ")
        throw OAuthServiceError.missingConfiguration(
            "\(provider.displayName) OAuth is missing a \(settingName). Set \(joined) or add \(provider.rawValue).\(Self.configKey(for: settingName)) to \(configURL.path) before connecting."
        )
    }

    private static func configKey(for settingName: String) -> String {
        settingName.replacingOccurrences(of: " ", with: "_")
    }

    private static func redirectURI(_ value: String) throws -> URL {
        guard let url = URL(string: value),
              url.scheme == "http",
              let host = url.host,
              ["127.0.0.1", "localhost"].contains(host),
              url.port != nil else {
            throw OAuthServiceError.invalidRedirectURI(value)
        }
        return url
    }
}

private final class OAuthCallbackServer: @unchecked Sendable {
    private let socketFD: Int32
    private let queue = DispatchQueue(label: "local.cubicle.oauth.callback")
    private let lock = NSLock()
    private let callbackPath: String
    private let callbackHost: String
    private let expectedState: String
    private let providerName: String
    private var continuation: CheckedContinuation<String, Error>?
    private var completed = false

    init(redirectURI: URL, expectedState: String, providerName: String) throws {
        guard let portValue = redirectURI.port,
              portValue > 0,
              portValue <= Int(UInt16.max),
              let host = redirectURI.host?.trimmingCharacters(in: .whitespacesAndNewlines),
              !host.isEmpty else {
            throw OAuthServiceError.invalidRedirectURI(redirectURI.absoluteString)
        }
        callbackPath = redirectURI.path.isEmpty ? "/" : redirectURI.path
        callbackHost = host
        self.expectedState = expectedState
        self.providerName = providerName
        socketFD = try Self.makeLoopbackSocket(host: host, portValue: portValue)
    }

    func waitForCode() async throws -> String {
        try await withTaskCancellationHandler {
            try await withCheckedThrowingContinuation { continuation in
                queue.async {
                    self.continuation = continuation
                    self.acceptCallback()
                }
            }
        } onCancel: {
            cancel()
        }
    }

    func cancel() {
        complete(.failure(CancellationError()))
    }

    private func acceptCallback() {
        let clientFD = Darwin.accept(socketFD, nil, nil)
        guard clientFD >= 0 else {
            guard !isCompleted else { return }
            complete(.failure(OAuthServiceError.callbackFailed("Local OAuth callback listener stopped before receiving the browser callback.")))
            return
        }
        defer {
            Darwin.shutdown(clientFD, SHUT_RDWR)
            Darwin.close(clientFD)
        }
        var noSigPipe: Int32 = 1
        _ = setsockopt(clientFD, SOL_SOCKET, SO_NOSIGPIPE, &noSigPipe, socklen_t(MemoryLayout<Int32>.size))

        guard let request = readRequest(from: clientFD),
              let result = parseCode(from: request) else {
            respond(
                clientFD: clientFD,
                statusCode: 400,
                title: "\(providerName) authorization failed",
                message: "OAuth callback did not include an authorization code."
            )
            complete(.failure(OAuthServiceError.callbackFailed("OAuth callback did not include an authorization code.")))
            return
        }

        switch result {
        case .success(let code):
            respond(
                clientFD: clientFD,
                statusCode: 200,
                title: "\(providerName) authorization complete",
                message: "You can close this tab and return to Cubicle."
            )
            complete(.success(code))
        case .failure(let error):
            respond(
                clientFD: clientFD,
                statusCode: 400,
                title: "\(providerName) authorization failed",
                message: error.localizedDescription
            )
            complete(.failure(error))
        }
    }

    private var isCompleted: Bool {
        lock.lock()
        defer { lock.unlock() }
        return completed
    }

    private func parseCode(from request: String) -> Result<String, Error>? {
        guard let requestLine = request.components(separatedBy: "\r\n").first,
              requestLine.hasPrefix("GET ") else {
            return nil
        }
        let parts = requestLine.split(separator: " ")
        guard parts.count >= 2 else { return nil }
        let pathAndQuery = String(parts[1])
        guard let components = URLComponents(string: "http://\(callbackHost)\(pathAndQuery)") else {
            return nil
        }
        guard components.path == callbackPath else {
            return .failure(OAuthServiceError.callbackFailed("Unexpected OAuth callback path: \(components.path)"))
        }
        let values = Dictionary(uniqueKeysWithValues: (components.queryItems ?? []).map { ($0.name, $0.value ?? "") })
        if let error = values["error"] {
            let description = values["error_description"] ?? ""
            return .failure(OAuthServiceError.callbackFailed("\(error): \(description)".trimmingCharacters(in: CharacterSet(charactersIn: ": "))))
        }
        guard values["state"] == expectedState else {
            return .failure(OAuthServiceError.callbackFailed("OAuth state mismatch. Start the login flow again."))
        }
        guard let code = values["code"], !code.isEmpty else {
            return nil
        }
        return .success(code)
    }

    private func respond(clientFD: Int32, statusCode: Int, title: String, message: String) {
        let statusText = statusCode == 200 ? "OK" : "Bad Request"
        let escapedTitle = Self.escapeHTML(title)
        let escapedMessage = Self.escapeHTML(message)
        let body = "<html><body><h1>\(escapedTitle)</h1><p>\(escapedMessage)</p></body></html>"
        let response = [
            "HTTP/1.1 \(statusCode) \(statusText)",
            "Content-Type: text/html; charset=utf-8",
            "Content-Length: \(Data(body.utf8).count)",
            "Connection: close",
            "",
            body
        ].joined(separator: "\r\n")
        sendAll(Data(response.utf8), to: clientFD)
    }

    private func complete(_ result: Result<String, Error>) {
        lock.lock()
        guard !completed else {
            lock.unlock()
            return
        }
        completed = true
        let continuation = self.continuation
        self.continuation = nil
        lock.unlock()
        Darwin.shutdown(socketFD, SHUT_RDWR)
        Darwin.close(socketFD)
        continuation?.resume(with: result)
    }

    private func readRequest(from clientFD: Int32) -> String? {
        var buffer = [UInt8](repeating: 0, count: 16_384)
        let count = buffer.withUnsafeMutableBufferPointer { pointer in
            Darwin.recv(clientFD, pointer.baseAddress, pointer.count, 0)
        }
        guard count > 0 else {
            return nil
        }
        return String(data: Data(buffer.prefix(count)), encoding: .utf8)
    }

    private func sendAll(_ data: Data, to clientFD: Int32) {
        data.withUnsafeBytes { rawBuffer in
            guard let baseAddress = rawBuffer.bindMemory(to: UInt8.self).baseAddress else {
                return
            }
            var sentTotal = 0
            while sentTotal < data.count {
                let sent = Darwin.send(clientFD, baseAddress.advanced(by: sentTotal), data.count - sentTotal, 0)
                guard sent > 0 else {
                    return
                }
                sentTotal += sent
            }
        }
    }

    private static func makeLoopbackSocket(host: String, portValue: Int) throws -> Int32 {
        let fd = Darwin.socket(AF_INET, SOCK_STREAM, 0)
        guard fd >= 0 else {
            throw OAuthServiceError.callbackFailed("Could not create local OAuth callback socket: \(errnoMessage())")
        }

        var reuseAddress: Int32 = 1
        guard setsockopt(fd, SOL_SOCKET, SO_REUSEADDR, &reuseAddress, socklen_t(MemoryLayout<Int32>.size)) == 0 else {
            Darwin.close(fd)
            throw OAuthServiceError.callbackFailed("Could not configure local OAuth callback socket: \(errnoMessage())")
        }

        var noSigPipe: Int32 = 1
        _ = setsockopt(fd, SOL_SOCKET, SO_NOSIGPIPE, &noSigPipe, socklen_t(MemoryLayout<Int32>.size))

        var address = sockaddr_in()
        address.sin_len = UInt8(MemoryLayout<sockaddr_in>.size)
        address.sin_family = sa_family_t(AF_INET)
        address.sin_port = UInt16(portValue).bigEndian
        let bindHost = host.localizedCaseInsensitiveCompare("localhost") == .orderedSame ? "127.0.0.1" : host
        guard inet_pton(AF_INET, bindHost, &address.sin_addr) == 1 else {
            Darwin.close(fd)
            throw OAuthServiceError.invalidRedirectURI("http://\(host):\(portValue)")
        }

        let bindResult = withUnsafePointer(to: &address) { pointer in
            pointer.withMemoryRebound(to: sockaddr.self, capacity: 1) { socketAddress in
                Darwin.bind(fd, socketAddress, socklen_t(MemoryLayout<sockaddr_in>.size))
            }
        }
        guard bindResult == 0 else {
            Darwin.close(fd)
            throw OAuthServiceError.callbackFailed("Could not bind local OAuth callback to \(host):\(portValue): \(errnoMessage())")
        }
        guard Darwin.listen(fd, 4) == 0 else {
            Darwin.close(fd)
            throw OAuthServiceError.callbackFailed("Could not listen for local OAuth callback on \(host):\(portValue): \(errnoMessage())")
        }
        return fd
    }

    private static func errnoMessage() -> String {
        String(cString: strerror(errno))
    }

    private static func escapeHTML(_ value: String) -> String {
        value
            .replacingOccurrences(of: "&", with: "&amp;")
            .replacingOccurrences(of: "<", with: "&lt;")
            .replacingOccurrences(of: ">", with: "&gt;")
            .replacingOccurrences(of: "\"", with: "&quot;")
    }
}
