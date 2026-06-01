import Foundation

struct RuntimeConfiguration: Equatable {
    var runtimeRoot: URL
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
        let environment = ProcessInfo.processInfo.environment
        let rootPath = environment["GETWEBEXSPACE_RUNTIME_ROOT"]?.trimmingCharacters(in: .whitespacesAndNewlines)
        let resolvedRoot = rootPath?.isEmpty == false ? rootPath! : "/Volumes/Webex/getwebexspace-data"
        let baseURLString = environment["WEBEX_API_BASE_URL"]?.trimmingCharacters(in: .whitespacesAndNewlines)
        let baseURL = URL(string: baseURLString ?? "") ?? URL(string: "https://webexapis.com/v1")!
        let pageSize = parseInt(
            environment["WEBEX_API_PAGE_SIZE"],
            defaultValue: 100,
            minimum: 1,
            maximum: 1000
        )
        let retries = parseInt(
            environment["WEBEX_API_RETRIES"],
            defaultValue: 5,
            minimum: 0,
            maximum: 10
        )
        let timeoutSeconds = TimeInterval(
            parseInt(
                environment["WEBEX_API_TIMEOUT_SECONDS"],
                defaultValue: 20,
                minimum: 1,
                maximum: 120
            )
        )
        let oauthRefreshSkew = TimeInterval(
            parseInt(
                environment["WEBEX_OAUTH_REFRESH_SKEW_SECONDS"],
                defaultValue: 300,
                minimum: 0,
                maximum: 86_400
            )
        )
        let oauthRefreshTokenSkew = TimeInterval(
            parseInt(
                environment["WEBEX_OAUTH_REFRESH_TOKEN_SKEW_SECONDS"],
                defaultValue: 86_400,
                minimum: 0,
                maximum: 2_592_000
            )
        )
        let adaptiveActiveInterval = TimeInterval(
            parseInt(
                environment["WEBEX_ADAPTIVE_ACTIVE_INTERVAL_SECONDS"],
                defaultValue: 20,
                minimum: 5,
                maximum: 120
            )
        )
        let adaptiveRecentInterval = TimeInterval(
            parseInt(
                environment["WEBEX_ADAPTIVE_RECENT_INTERVAL_SECONDS"],
                defaultValue: 60,
                minimum: 15,
                maximum: 300
            )
        )
        let adaptiveBackgroundInterval = TimeInterval(
            parseInt(
                environment["WEBEX_ADAPTIVE_BACKGROUND_INTERVAL_SECONDS"],
                defaultValue: 180,
                minimum: 60,
                maximum: 900
            )
        )
        let adaptiveJitterPercent = parseInt(
            environment["WEBEX_ADAPTIVE_JITTER_PERCENT"],
            defaultValue: 20,
            minimum: 0,
            maximum: 80
        )
        let syncConcurrencyLimit = parseInt(
            environment["WEBEX_SYNC_CONCURRENCY_LIMIT"],
            defaultValue: 3,
            minimum: 1,
            maximum: 10
        )
        let publicWebhookURL = trimToNil(environment["WEBEX_PUBLIC_WEBHOOK_URL"]).flatMap(URL.init(string:))
        let oauthTokenPath = trimToNil(environment["WEBEX_OAUTH_TOKEN_FILE"])
        return RuntimeConfiguration(
            runtimeRoot: URL(fileURLWithPath: resolvedRoot, isDirectory: true),
            codexExecutable: resolvedCodexExecutable(environment["CODEX_BIN"]),
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
