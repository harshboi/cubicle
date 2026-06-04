import Foundation

/// File names and document types for optional macOS JSON configuration.
enum MacAppJSONConfigurationFiles {
    static let entrypoint = "cubicle.json"
    static let base = "base.json"
}

/// Environment names controlling the optional JSON configuration layer.
enum MacAppJSONConfigurationEnvironment {
    static let enabled = "CUBICLE_JSON_CONFIG_ENABLED"
    static let file = "CUBICLE_CONFIG_FILE"
    static let directory = "CUBICLE_JSON_CONFIG_DIR"

    static func isEnabled(in environment: [String: String]) -> Bool {
        guard let value = trimmed(environment[enabled])?.lowercased() else {
            return false
        }
        return ["1", "true", "yes", "on"].contains(value)
    }

    static func trimmed(_ value: String?) -> String? {
        guard let trimmed = value?.trimmingCharacters(in: .whitespacesAndNewlines),
              !trimmed.isEmpty else {
            return nil
        }
        return trimmed
    }
}

/// HOCON-style JSON composer used before decoding the typed Cubicle config.
struct MacAppJSONConfigurationComposer {
    var fileManager: FileManager

    init(fileManager: FileManager = .default) {
        self.fileManager = fileManager
    }

    /// Loads one entrypoint, applies `extends`, resolves `use` references, and decodes the typed document.
    func loadDocument(entrypointURL: URL) throws -> MacAppJSONConfigurationDocument {
        let object = try resolvedJSONObject(entrypointURL: entrypointURL)
        try MacAppJSONConfigurationValidator.validateNoSecrets(in: object)
        let data = try JSONSerialization.data(withJSONObject: object, options: [.sortedKeys])
        do {
            return try JSONDecoder().decode(MacAppJSONConfigurationDocument.self, from: data)
        } catch {
            throw MacAppJSONConfigurationError.decodeFailed(entrypointURL, error)
        }
    }

    /// Loads one entrypoint and returns the resolved raw object for diagnostics and tests.
    func resolvedJSONObject(entrypointURL: URL) throws -> [String: Any] {
        let loaded = try loadJSONObject(entrypointURL, stack: [])
        return try resolveUseReferences(in: loaded, root: loaded, path: [], stack: [])
    }

    private func loadJSONObject(_ url: URL, stack: [URL]) throws -> [String: Any] {
        let standardizedURL = url.standardizedFileURL
        if stack.contains(standardizedURL) {
            throw MacAppJSONConfigurationError.extendCycle((stack + [standardizedURL]).map(\.path))
        }
        guard fileManager.fileExists(atPath: standardizedURL.path) else {
            throw MacAppJSONConfigurationError.missingFile(standardizedURL)
        }

        let data: Data
        do {
            data = try Data(contentsOf: standardizedURL)
        } catch {
            throw MacAppJSONConfigurationError.readFailed(standardizedURL, error)
        }

        let raw: Any
        do {
            raw = try JSONSerialization.jsonObject(with: data)
        } catch {
            throw MacAppJSONConfigurationError.invalidJSON(standardizedURL, error)
        }
        guard var object = raw as? [String: Any] else {
            throw MacAppJSONConfigurationError.invalidRootObject(standardizedURL)
        }

        let extendsValues = try extendsList(in: object, sourceURL: standardizedURL)
        object.removeValue(forKey: "extends")
        let includeValues = try includeMap(in: object, sourceURL: standardizedURL)
        object.removeValue(forKey: "include")

        var merged: [String: Any] = [:]
        let parentStack = stack + [standardizedURL]
        for extendsValue in extendsValues {
            let parentURL = expandedFileURL(
                extendsValue,
                baseDirectory: standardizedURL.deletingLastPathComponent()
            )
            let parent = try loadJSONObject(parentURL, stack: parentStack)
            merged = Self.deepMerging(merged, parent)
        }
        for (section, includePath) in includeValues.sorted(by: { $0.key < $1.key }) {
            let includeURL = try includedFileURL(
                includePath,
                baseDirectory: standardizedURL.deletingLastPathComponent(),
                sourceURL: standardizedURL
            )
            let includedObject = try loadJSONObject(includeURL, stack: parentStack)
            merged = Self.deepMerging(merged, [section: includedObject])
        }
        return Self.deepMerging(merged, object)
    }

    private func extendsList(in object: [String: Any], sourceURL: URL) throws -> [String] {
        guard let value = object["extends"] else { return [] }
        if let string = value as? String {
            return [string]
        }
        if let array = value as? [String] {
            return array
        }
        throw MacAppJSONConfigurationError.invalidExtends(sourceURL)
    }

    private func includeMap(in object: [String: Any], sourceURL: URL) throws -> [String: String] {
        guard let value = object["include"] else { return [:] }
        guard let rawMap = value as? [String: Any] else {
            throw MacAppJSONConfigurationError.invalidInclude(sourceURL)
        }

        var includes: [String: String] = [:]
        for (section, path) in rawMap {
            guard let path = path as? String else {
                throw MacAppJSONConfigurationError.invalidInclude(sourceURL)
            }
            includes[section] = path
        }
        return includes
    }

    private func resolveUseReferences(
        in value: Any,
        root: [String: Any],
        path: [String],
        stack: [String]
    ) throws -> [String: Any] {
        guard var object = value as? [String: Any] else {
            throw MacAppJSONConfigurationError.invalidUseObject(path.joined(separator: "."))
        }

        if let usePath = object["use"] as? String {
            if stack.contains(usePath) {
                throw MacAppJSONConfigurationError.useCycle(stack + [usePath])
            }
            guard let referenced = Self.value(at: usePath, in: root) as? [String: Any] else {
                throw MacAppJSONConfigurationError.invalidUseReference(usePath)
            }
            object.removeValue(forKey: "use")
            let resolvedReference = try resolveUseReferences(
                in: referenced,
                root: root,
                path: usePath.components(separatedBy: "."),
                stack: stack + [usePath]
            )
            object = Self.deepMerging(resolvedReference, object)
        }

        for (key, child) in object {
            if let childObject = child as? [String: Any] {
                object[key] = try resolveUseReferences(
                    in: childObject,
                    root: root,
                    path: path + [key],
                    stack: stack
                )
            } else if let childArray = child as? [Any] {
                let resolvedArray: [Any] = try childArray.enumerated().map { index, element in
                    if let elementObject = element as? [String: Any] {
                        let resolvedObject = try resolveUseReferences(
                            in: elementObject,
                            root: root,
                            path: path + [key, String(index)],
                            stack: stack
                        )
                        return resolvedObject as Any
                    }
                    return element
                }
                object[key] = resolvedArray
            }
        }
        return object
    }

    private func expandedFileURL(_ value: String, baseDirectory: URL) -> URL {
        let expanded = NSString(string: value).expandingTildeInPath
        if expanded.hasPrefix("/") {
            return URL(fileURLWithPath: expanded)
        }
        return baseDirectory.appendingPathComponent(expanded)
    }

    private func includedFileURL(_ value: String, baseDirectory: URL, sourceURL: URL) throws -> URL {
        let trimmed = value.trimmingCharacters(in: .whitespacesAndNewlines)
        guard !trimmed.isEmpty,
              !trimmed.hasPrefix("~"),
              !trimmed.hasPrefix("/"),
              !trimmed.split(separator: "/", omittingEmptySubsequences: false).contains("..") else {
            throw MacAppJSONConfigurationError.invalidIncludePath(sourceURL, value)
        }
        return baseDirectory.appendingPathComponent(trimmed)
    }

    static func deepMerging(_ base: [String: Any], _ overlay: [String: Any]) -> [String: Any] {
        var merged = base
        for (key, overlayValue) in overlay {
            if let baseObject = merged[key] as? [String: Any],
               let overlayObject = overlayValue as? [String: Any] {
                merged[key] = deepMerging(baseObject, overlayObject)
            } else {
                merged[key] = overlayValue
            }
        }
        return merged
    }

    static func value(at path: String, in root: [String: Any]) -> Any? {
        let parts = path
            .split(separator: ".")
            .map(String.init)
            .filter { !$0.isEmpty }
        guard !parts.isEmpty else { return nil }
        var current: Any = root
        for part in parts {
            guard let object = current as? [String: Any],
                  let next = object[part] else {
                return nil
            }
            current = next
        }
        return current
    }
}

enum MacAppJSONConfigurationError: LocalizedError, Equatable {
    case missingFile(URL)
    case readFailed(URL, Error)
    case invalidJSON(URL, Error)
    case invalidRootObject(URL)
    case invalidExtends(URL)
    case invalidInclude(URL)
    case invalidIncludePath(URL, String)
    case extendCycle([String])
    case invalidUseObject(String)
    case invalidUseReference(String)
    case useCycle([String])
    case secretKeyNotAllowed(String)
    case decodeFailed(URL, Error)

    static func == (lhs: MacAppJSONConfigurationError, rhs: MacAppJSONConfigurationError) -> Bool {
        switch (lhs, rhs) {
        case (.missingFile(let left), .missingFile(let right)):
            return left == right
        case (.readFailed(let left, _), .readFailed(let right, _)):
            return left == right
        case (.invalidJSON(let left, _), .invalidJSON(let right, _)):
            return left == right
        case (.invalidRootObject(let left), .invalidRootObject(let right)):
            return left == right
        case (.invalidExtends(let left), .invalidExtends(let right)):
            return left == right
        case (.invalidInclude(let left), .invalidInclude(let right)):
            return left == right
        case (.invalidIncludePath(let leftURL, let leftPath), .invalidIncludePath(let rightURL, let rightPath)):
            return leftURL == rightURL && leftPath == rightPath
        case (.extendCycle(let left), .extendCycle(let right)):
            return left == right
        case (.invalidUseObject(let left), .invalidUseObject(let right)):
            return left == right
        case (.invalidUseReference(let left), .invalidUseReference(let right)):
            return left == right
        case (.useCycle(let left), .useCycle(let right)):
            return left == right
        case (.secretKeyNotAllowed(let left), .secretKeyNotAllowed(let right)):
            return left == right
        case (.decodeFailed(let left, _), .decodeFailed(let right, _)):
            return left == right
        default:
            return false
        }
    }

    var errorDescription: String? {
        switch self {
        case .missingFile(let url):
            return "Missing Cubicle JSON config file: \(url.path)"
        case .readFailed(let url, let error):
            return "Could not read Cubicle JSON config file \(url.path): \(error.localizedDescription)"
        case .invalidJSON(let url, let error):
            return "Invalid Cubicle JSON config in \(url.path): \(error.localizedDescription)"
        case .invalidRootObject(let url):
            return "Cubicle JSON config root must be an object: \(url.path)"
        case .invalidExtends(let url):
            return "`extends` must be a string or string array in \(url.path)."
        case .invalidInclude(let url):
            return "`include` must be an object of top-level section names to relative file paths in \(url.path)."
        case .invalidIncludePath(let url, let path):
            return "Cubicle JSON config include path must be relative and stay under \(url.deletingLastPathComponent().path): \(path)"
        case .extendCycle(let paths):
            return "Cubicle JSON config extends cycle: \(paths.joined(separator: " -> "))"
        case .invalidUseObject(let path):
            return "`use` can only appear in an object at \(path)."
        case .invalidUseReference(let path):
            return "Cubicle JSON config `use` reference does not point to an object: \(path)"
        case .useCycle(let paths):
            return "Cubicle JSON config `use` cycle: \(paths.joined(separator: " -> "))"
        case .secretKeyNotAllowed(let keyPath):
            return "Secret-bearing key is not allowed in Cubicle JSON config: \(keyPath)"
        case .decodeFailed(let url, let error):
            return "Could not decode resolved Cubicle JSON config from \(url.path): \(error.localizedDescription)"
        }
    }
}

enum MacAppJSONConfigurationValidator {
    private static let forbiddenSecretKeys: Set<String> = [
        "access_token",
        "accesstoken",
        "auth_token",
        "authtoken",
        "client_secret",
        "clientsecret",
        "refresh_token",
        "refreshtoken"
    ]

    static func validateNoSecrets(in object: [String: Any]) throws {
        try validateNoSecrets(in: object, path: [])
    }

    private static func validateNoSecrets(in value: Any, path: [String]) throws {
        if let object = value as? [String: Any] {
            for (key, child) in object {
                let normalizedKey = key
                    .trimmingCharacters(in: .whitespacesAndNewlines)
                    .lowercased()
                    .replacingOccurrences(of: "-", with: "_")
                if forbiddenSecretKeys.contains(normalizedKey) {
                    throw MacAppJSONConfigurationError.secretKeyNotAllowed((path + [key]).joined(separator: "."))
                }
                try validateNoSecrets(in: child, path: path + [key])
            }
        } else if let array = value as? [Any] {
            for (index, child) in array.enumerated() {
                try validateNoSecrets(in: child, path: path + [String(index)])
            }
        }
    }
}

/// Resolved top-level Cubicle configuration after composition.
struct MacAppJSONConfigurationDocument: Codable, Equatable {
    var version: Int?
    var environment: MacAppJSONEnvironmentConfiguration?
    var connectors: MacAppJSONConnectorsConfiguration?
    var codex: MacAppJSONCodexConfiguration?
    var questionGeneration: MacAppJSONQuestionGenerationConfiguration?
    var testMode: MacAppJSONTestModeConfiguration?

    enum CodingKeys: String, CodingKey {
        case version
        case environment
        case connectors
        case codex
        case questionGeneration = "question_generation"
        case testMode = "test_mode"
    }
}

/// Reusable policy blocks scoped to one parent section.
struct MacAppJSONCommonPolicies: Codable, Equatable {
    var runPolicy: MacAppJSONRunPolicy?
    var networkPolicy: MacAppJSONNetworkPolicy?
    var cachePolicy: MacAppJSONCachePolicy?
    var syncPolicy: MacAppJSONSyncPolicy?

    enum CodingKeys: String, CodingKey {
        case runPolicy = "run_policy"
        case networkPolicy = "network_policy"
        case cachePolicy = "cache_policy"
        case syncPolicy = "sync_policy"
    }
}

/// Parent defaults applied to child sections before child overrides.
struct MacAppJSONPolicyDefaults: Codable, Equatable {
    var runPolicy: MacAppJSONRunPolicy?
    var networkPolicy: MacAppJSONNetworkPolicy?
    var cachePolicy: MacAppJSONCachePolicy?
    var syncPolicy: MacAppJSONSyncPolicy?

    enum CodingKeys: String, CodingKey {
        case runPolicy = "run_policy"
        case networkPolicy = "network_policy"
        case cachePolicy = "cache_policy"
        case syncPolicy = "sync_policy"
    }
}

/// Retry and timeout behavior shared by Codex jobs and other runnable work.
struct MacAppJSONRunPolicy: Codable, Equatable {
    var timeoutSeconds: Double?
    var maxAttempts: Int?
    var retryDelaySeconds: Double?

    enum CodingKeys: String, CodingKey {
        case timeoutSeconds = "timeout_seconds"
        case maxAttempts = "max_attempts"
        case retryDelaySeconds = "retry_delay_seconds"
    }
}

/// Network request behavior shared by remote connectors.
struct MacAppJSONNetworkPolicy: Codable, Equatable {
    var timeoutSeconds: Double?
    var retryCount: Int?
    var pageSize: Int?

    enum CodingKeys: String, CodingKey {
        case timeoutSeconds = "timeout_seconds"
        case retryCount = "retry_count"
        case pageSize = "page_size"
    }
}

/// Cache freshness behavior shared by generated artifacts.
struct MacAppJSONCachePolicy: Codable, Equatable {
    var maxAgeSeconds: Double?
    var summaryMaxAgeSeconds: Double?
    var execQuestionsMaxAgeSeconds: Double?

    enum CodingKeys: String, CodingKey {
        case maxAgeSeconds = "max_age_seconds"
        case summaryMaxAgeSeconds = "summary_max_age_seconds"
        case execQuestionsMaxAgeSeconds = "exec_questions_max_age_seconds"
    }
}

/// Scheduler/concurrency behavior shared by connector sync work.
struct MacAppJSONSyncPolicy: Codable, Equatable {
    var concurrencyLimit: Int?
    var adaptiveActiveIntervalSeconds: Double?
    var adaptiveRecentIntervalSeconds: Double?
    var adaptiveBackgroundIntervalSeconds: Double?
    var adaptiveJitterPercent: Int?

    enum CodingKeys: String, CodingKey {
        case concurrencyLimit = "concurrency_limit"
        case adaptiveActiveIntervalSeconds = "adaptive_active_interval_seconds"
        case adaptiveRecentIntervalSeconds = "adaptive_recent_interval_seconds"
        case adaptiveBackgroundIntervalSeconds = "adaptive_background_interval_seconds"
        case adaptiveJitterPercent = "adaptive_jitter_percent"
    }
}

/// Env-like non-secret runtime tuning.
struct MacAppJSONEnvironmentConfiguration: Codable, Equatable {
    var common: MacAppJSONCommonPolicies?
    var defaults: MacAppJSONPolicyDefaults?
    var codexExecutable: String?
    var runtimeRoot: String?
    var webex: MacAppJSONEnvironmentWebexSettings?
    var imessage: MacAppJSONEnvironmentIMessageSettings?

    enum CodingKeys: String, CodingKey {
        case common
        case defaults
        case codexExecutable = "codex_executable"
        case runtimeRoot = "runtime_root"
        case webex
        case imessage
    }
}

/// Env-like Webex tuning that is not secret material.
struct MacAppJSONEnvironmentWebexSettings: Codable, Equatable {
    var apiBaseURL: String?
    var publicWebhookURL: String?
    var oauthTokenFile: String?
    var oauthRefreshSkewSeconds: Double?
    var oauthRefreshTokenSkewSeconds: Double?
    var networkPolicy: MacAppJSONNetworkPolicy?
    var syncPolicy: MacAppJSONSyncPolicy?

    enum CodingKeys: String, CodingKey {
        case apiBaseURL = "api_base_url"
        case publicWebhookURL = "public_webhook_url"
        case oauthTokenFile = "oauth_token_file"
        case oauthRefreshSkewSeconds = "oauth_refresh_skew_seconds"
        case oauthRefreshTokenSkewSeconds = "oauth_refresh_token_skew_seconds"
        case networkPolicy = "network_policy"
        case syncPolicy = "sync_policy"
    }
}

/// Env-like iMessage source tuning.
struct MacAppJSONEnvironmentIMessageSettings: Codable, Equatable {
    var chatDatabasePath: String?
    var busyTimeoutMilliseconds: Int?

    enum CodingKeys: String, CodingKey {
        case chatDatabasePath = "chat_database_path"
        case busyTimeoutMilliseconds = "busy_timeout_milliseconds"
    }
}

/// Connector selection and connector-specific non-secret settings.
struct MacAppJSONConnectorsConfiguration: Codable, Equatable {
    var common: MacAppJSONCommonPolicies?
    var defaults: MacAppJSONPolicyDefaults?
    var enabled: [String]?
    var webex: MacAppJSONWebexConnectorConfiguration?
    var imessage: MacAppJSONIMessageConnectorConfiguration?

    enum CodingKeys: String, CodingKey {
        case common
        case defaults
        case enabled
        case webex
        case imessage
    }
}

struct MacAppJSONWebexConnectorConfiguration: Codable, Equatable {
    var enabled: Bool?
    var networkPolicy: MacAppJSONNetworkPolicy?
    var syncPolicy: MacAppJSONSyncPolicy?
    var fixturePath: String?

    enum CodingKeys: String, CodingKey {
        case enabled
        case networkPolicy = "network_policy"
        case syncPolicy = "sync_policy"
        case fixturePath = "fixture_path"
    }
}

struct MacAppJSONIMessageConnectorConfiguration: Codable, Equatable {
    var enabled: Bool?
    var chatDatabasePath: String?
    var busyTimeoutMilliseconds: Int?
    var fixturePath: String?

    enum CodingKeys: String, CodingKey {
        case enabled
        case chatDatabasePath = "chat_database_path"
        case busyTimeoutMilliseconds = "busy_timeout_milliseconds"
        case fixturePath = "fixture_path"
    }
}

/// Codex execution, cache, belief, and synthesis tuning.
struct MacAppJSONCodexConfiguration: Codable, Equatable {
    var common: MacAppJSONCommonPolicies?
    var defaults: MacAppJSONPolicyDefaults?
    var runPolicy: MacAppJSONRunPolicy?
    var cachePolicy: MacAppJSONCachePolicy?
    var beliefs: MacAppJSONCodexBeliefPolicy?
    var questionSynthesis: MacAppJSONCodexQuestionSynthesisPolicy?

    enum CodingKeys: String, CodingKey {
        case common
        case defaults
        case runPolicy = "run_policy"
        case cachePolicy = "cache_policy"
        case beliefs
        case questionSynthesis = "question_synthesis"
    }
}

/// Belief reconciliation scheduling and chunking tuning.
struct MacAppJSONCodexBeliefPolicy: Codable, Equatable {
    var staleHours: Int?
    var evidenceChunkSize: Int?
    var maxIncrementalWindowDays: Int?

    enum CodingKeys: String, CodingKey {
        case staleHours = "stale_hours"
        case evidenceChunkSize = "evidence_chunk_size"
        case maxIncrementalWindowDays = "max_incremental_window_days"
    }
}

/// Question-synthesis prompt input sizing and optional run-policy override.
struct MacAppJSONCodexQuestionSynthesisPolicy: Codable, Equatable {
    var runPolicy: MacAppJSONRunPolicy?
    var seedCandidateLimit: Int?
    var queryHistoryLimit: Int?
    var promptHistoryLimit: Int?
    var candidateEvidenceLimit: Int?
    var outputLimit: Int?

    enum CodingKeys: String, CodingKey {
        case runPolicy = "run_policy"
        case seedCandidateLimit = "seed_candidate_limit"
        case queryHistoryLimit = "query_history_limit"
        case promptHistoryLimit = "prompt_history_limit"
        case candidateEvidenceLimit = "candidate_evidence_limit"
        case outputLimit = "output_limit"
    }
}

/// Local question generation and filtering tuning.
struct MacAppJSONQuestionGenerationConfiguration: Codable, Equatable {
    var common: MacAppJSONCommonPolicies?
    var defaults: MacAppJSONPolicyDefaults?
    var core: MacAppJSONQuestionGenerationCoreSettings?
    var cubicle: MacAppJSONQuestionGenerationCubicleSettings?
}

/// WebexQuestionGeneratorCore configuration overrides.
struct MacAppJSONQuestionGenerationCoreSettings: Codable, Equatable {
    var privacy: MacAppJSONQuestionGenerationPrivacySettings?
    var topics: MacAppJSONQuestionGenerationTopicSettings?
    var questions: MacAppJSONQuestionGenerationQuestionSettings?
}

/// Local analytics privacy settings.
struct MacAppJSONQuestionGenerationPrivacySettings: Codable, Equatable {
    var anonymizeUsers: Bool?
    var redactURLs: Bool?
    var redactEmails: Bool?

    enum CodingKeys: String, CodingKey {
        case anonymizeUsers = "anonymize_users"
        case redactURLs = "redact_urls"
        case redactEmails = "redact_emails"
    }
}

/// Local topic extraction settings.
struct MacAppJSONQuestionGenerationTopicSettings: Codable, Equatable {
    var enabled: Bool?
    var numberOfTopics: Int?
    var minimumTopicSize: Int?

    enum CodingKeys: String, CodingKey {
        case enabled
        case numberOfTopics = "number_of_topics"
        case minimumTopicSize = "minimum_topic_size"
    }
}

/// Local question extraction settings.
struct MacAppJSONQuestionGenerationQuestionSettings: Codable, Equatable {
    var topN: Int?
    var enabledCategories: [String]?

    enum CodingKeys: String, CodingKey {
        case topN = "top_n"
        case enabledCategories = "enabled_categories"
    }
}

/// Cubicle-specific fallback and publication thresholds.
struct MacAppJSONQuestionGenerationCubicleSettings: Codable, Equatable {
    var fallbackDraftLimit: Int?
    var generatedQuestionLimit: Int?
    var publishableQuestionLimit: Int?
    var evidenceLimit: Int?
    var coreEvidenceLimit: Int?

    enum CodingKeys: String, CodingKey {
        case fallbackDraftLimit = "fallback_draft_limit"
        case generatedQuestionLimit = "generated_question_limit"
        case publishableQuestionLimit = "publishable_question_limit"
        case evidenceLimit = "evidence_limit"
        case coreEvidenceLimit = "core_evidence_limit"
    }
}

/// Test-app mode uses stable input files that cleanup must never delete.
struct MacAppJSONTestModeConfiguration: Codable, Equatable {
    var enabled: Bool?
    var profile: String?
    var fixtureRoot: String?
    var targetData: String?
    var settings: String?
    var protectPaths: [String]?
    var connectorFixtures: [String: String]?

    enum CodingKeys: String, CodingKey {
        case enabled
        case profile
        case fixtureRoot = "fixture_root"
        case targetData = "target_data"
        case settings
        case protectPaths = "protect_paths"
        case connectorFixtures = "connector_fixtures"
    }
}
