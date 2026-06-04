import Foundation

/// Runtime paths, executable choices, and network tuning loaded from env.
struct RuntimeConfiguration: Equatable {
    var runtimeRoot: URL
    var jsonConfiguration: MacAppJSONConfigurationDocument? = nil
    var codexExecutable: String
    var webexBaseURL: URL
    var webexPageSize: Int
    var webexRetryCount: Int
    var webexTimeoutSeconds: TimeInterval
    var webexOAuthTokenPathOverride: String?
    var webexOAuthRefreshSkewSeconds: TimeInterval
    var webexOAuthRefreshTokenSkewSeconds: TimeInterval
    var webexAdaptiveActiveIntervalSeconds: TimeInterval = 20
    var webexAdaptiveRecentIntervalSeconds: TimeInterval = 60
    var webexAdaptiveBackgroundIntervalSeconds: TimeInterval = 180
    var webexAdaptiveJitterRatio: Double = 0.20
    var webexSyncConcurrencyLimit: Int = 3
    // Cubicle runs locally on a user Mac, so adaptive polling is the default.
    // Webhooks are optional dev mode only because Webex requires a public HTTPS target URL.
    var webexPublicWebhookURL: URL? = nil

    static var current: RuntimeConfiguration {
        resolved(environment: ProcessInfo.processInfo.environment)
    }

    static func resolved(
        environment: [String: String],
        fileManager: FileManager = .default
    ) -> RuntimeConfiguration {
        let rootPath = environment["GETWEBEXSPACE_RUNTIME_ROOT"]?.trimmingCharacters(in: .whitespacesAndNewlines)
        let resolvedRoot = rootPath?.isEmpty == false ? rootPath! : "/Volumes/Webex/getwebexspace-data"
        let runtimeRootURL = URL(fileURLWithPath: resolvedRoot, isDirectory: true)
        let jsonDocument = MacAppJSONConfigurationLoader(
            runtimeRoot: runtimeRootURL,
            environment: environment,
            fileManager: fileManager
        ).configurationDocument()
        let environmentJSON = jsonDocument?.environment
        let webexJSON = environmentJSON?.webex
        let webexNetworkJSON = webexJSON?.networkPolicy
        let webexSyncJSON = webexJSON?.syncPolicy

        let baseURLString = trimToNil(environment["WEBEX_API_BASE_URL"])
            ?? trimToNil(webexJSON?.apiBaseURL)
        let baseURL = URL(string: baseURLString ?? "") ?? URL(string: "https://webexapis.com/v1")!
        let pageSize = parseInt(
            environment["WEBEX_API_PAGE_SIZE"],
            defaultValue: clampedInt(webexNetworkJSON?.pageSize, defaultValue: 100, minimum: 1, maximum: 1000),
            minimum: 1,
            maximum: 1000
        )
        let retries = parseInt(
            environment["WEBEX_API_RETRIES"],
            defaultValue: clampedInt(webexNetworkJSON?.retryCount, defaultValue: 5, minimum: 0, maximum: 10),
            minimum: 0,
            maximum: 10
        )
        let timeoutSeconds = parseTimeInterval(
            environment["WEBEX_API_TIMEOUT_SECONDS"],
            defaultValue: clampedTimeInterval(webexNetworkJSON?.timeoutSeconds, defaultValue: 20, minimum: 1, maximum: 120),
            minimum: 1,
            maximum: 120
        )
        let oauthRefreshSkew = parseTimeInterval(
            environment["WEBEX_OAUTH_REFRESH_SKEW_SECONDS"],
            defaultValue: clampedTimeInterval(webexJSON?.oauthRefreshSkewSeconds, defaultValue: 300, minimum: 0, maximum: 86_400),
            minimum: 0,
            maximum: 86_400
        )
        let oauthRefreshTokenSkew = parseTimeInterval(
            environment["WEBEX_OAUTH_REFRESH_TOKEN_SKEW_SECONDS"],
            defaultValue: clampedTimeInterval(
                webexJSON?.oauthRefreshTokenSkewSeconds,
                defaultValue: 86_400,
                minimum: 0,
                maximum: 2_592_000
            ),
            minimum: 0,
            maximum: 2_592_000
        )
        let adaptiveActiveInterval = parseTimeInterval(
            environment["WEBEX_ADAPTIVE_ACTIVE_INTERVAL_SECONDS"],
            defaultValue: clampedTimeInterval(
                webexSyncJSON?.adaptiveActiveIntervalSeconds,
                defaultValue: 20,
                minimum: 5,
                maximum: 120
            ),
            minimum: 5,
            maximum: 120
        )
        let adaptiveRecentInterval = parseTimeInterval(
            environment["WEBEX_ADAPTIVE_RECENT_INTERVAL_SECONDS"],
            defaultValue: clampedTimeInterval(
                webexSyncJSON?.adaptiveRecentIntervalSeconds,
                defaultValue: 60,
                minimum: 15,
                maximum: 300
            ),
            minimum: 15,
            maximum: 300
        )
        let adaptiveBackgroundInterval = parseTimeInterval(
            environment["WEBEX_ADAPTIVE_BACKGROUND_INTERVAL_SECONDS"],
            defaultValue: clampedTimeInterval(
                webexSyncJSON?.adaptiveBackgroundIntervalSeconds,
                defaultValue: 180,
                minimum: 60,
                maximum: 900
            ),
            minimum: 60,
            maximum: 900
        )
        let adaptiveJitterPercent = parseInt(
            environment["WEBEX_ADAPTIVE_JITTER_PERCENT"],
            defaultValue: clampedInt(webexSyncJSON?.adaptiveJitterPercent, defaultValue: 20, minimum: 0, maximum: 80),
            minimum: 0,
            maximum: 80
        )
        let syncConcurrencyLimit = parseInt(
            environment["WEBEX_SYNC_CONCURRENCY_LIMIT"],
            defaultValue: clampedInt(webexSyncJSON?.concurrencyLimit, defaultValue: 3, minimum: 1, maximum: 10),
            minimum: 1,
            maximum: 10
        )
        let publicWebhookURL = (trimToNil(environment["WEBEX_PUBLIC_WEBHOOK_URL"])
            ?? trimToNil(webexJSON?.publicWebhookURL))
            .flatMap(URL.init(string:))
        let oauthTokenPath = trimToNil(environment["WEBEX_OAUTH_TOKEN_FILE"])
            ?? trimToNil(webexJSON?.oauthTokenFile)
        return RuntimeConfiguration(
            runtimeRoot: runtimeRootURL,
            jsonConfiguration: jsonDocument,
            codexExecutable: resolvedCodexExecutable(
                trimToNil(environment["CODEX_BIN"]) ?? trimToNil(environmentJSON?.codexExecutable)
            ),
            webexBaseURL: baseURL,
            webexPageSize: pageSize,
            webexRetryCount: retries,
            webexTimeoutSeconds: timeoutSeconds,
            webexOAuthTokenPathOverride: oauthTokenPath,
            webexOAuthRefreshSkewSeconds: oauthRefreshSkew,
            webexOAuthRefreshTokenSkewSeconds: oauthRefreshTokenSkew,
            webexAdaptiveActiveIntervalSeconds: adaptiveActiveInterval,
            webexAdaptiveRecentIntervalSeconds: adaptiveRecentInterval,
            webexAdaptiveBackgroundIntervalSeconds: adaptiveBackgroundInterval,
            webexAdaptiveJitterRatio: Double(adaptiveJitterPercent) / 100.0,
            webexSyncConcurrencyLimit: syncConcurrencyLimit,
            webexPublicWebhookURL: publicWebhookURL
        )
    }

    private static func parseTimeInterval(
        _ rawValue: String?,
        defaultValue: TimeInterval,
        minimum: TimeInterval,
        maximum: TimeInterval
    ) -> TimeInterval {
        guard let rawValue = rawValue?.trimmingCharacters(in: .whitespacesAndNewlines),
              !rawValue.isEmpty,
              let parsed = Double(rawValue) else {
            return defaultValue
        }
        return min(max(parsed, minimum), maximum)
    }

    private static func clampedInt(
        _ value: Int?,
        defaultValue: Int,
        minimum: Int,
        maximum: Int
    ) -> Int {
        guard let value else { return defaultValue }
        return min(max(value, minimum), maximum)
    }

    private static func clampedTimeInterval(
        _ value: Double?,
        defaultValue: TimeInterval,
        minimum: TimeInterval,
        maximum: TimeInterval
    ) -> TimeInterval {
        guard let value else { return defaultValue }
        return min(max(TimeInterval(value), minimum), maximum)
    }

    private static func parseInt(
        _ rawValue: String?,
        defaultValue: Int,
        minimum: Int,
        maximum: Int
    ) -> Int {
        guard let rawValue = rawValue?.trimmingCharacters(in: .whitespacesAndNewlines),
              !rawValue.isEmpty,
              let parsed = Int(rawValue) else {
            return defaultValue
        }
        return min(max(parsed, minimum), maximum)
    }

    private static func trimToNil(_ value: String?) -> String? {
        guard let trimmed = value?.trimmingCharacters(in: .whitespacesAndNewlines),
              !trimmed.isEmpty else {
            return nil
        }
        return trimmed
    }

    private static func resolvedCodexExecutable(_ override: String?) -> String {
        if let override = trimToNil(override) {
            return override
        }

        let fileManager = FileManager.default
        let candidatePaths = [
            "/opt/homebrew/bin/codex",
            "/usr/local/bin/codex",
            "\(NSHomeDirectory())/.local/bin/codex",
            "\(NSHomeDirectory())/.npm-global/bin/codex"
        ]
        if let path = candidatePaths.first(where: { fileManager.isExecutableFile(atPath: $0) }) {
            return path
        }
        return "codex"
    }
}
