import Foundation
import MetaCodable

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
@Codable
struct MacAppJSONConfigurationDocument: Equatable {
    var version: Int?
    var environment: MacAppJSONEnvironmentConfiguration?
    var connectors: MacAppJSONConnectorsConfiguration?
    var codex: MacAppJSONCodexConfiguration?
    @CodedAt("question_generation")
    var questionGeneration: MacAppJSONQuestionGenerationConfiguration?
    @CodedAt("test_mode")
    var testMode: MacAppJSONTestModeConfiguration?
}

/// Reusable policy blocks scoped to one parent section.
@Codable
struct MacAppJSONCommonPolicies: Equatable {
    @CodedAt("run_policy")
    var runPolicy: MacAppJSONRunPolicy?
    @CodedAt("network_policy")
    var networkPolicy: MacAppJSONNetworkPolicy?
    @CodedAt("cache_policy")
    var cachePolicy: MacAppJSONCachePolicy?
    @CodedAt("sync_policy")
    var syncPolicy: MacAppJSONSyncPolicy?
}

/// Parent defaults applied to child sections before child overrides.
@Codable
struct MacAppJSONPolicyDefaults: Equatable {
    @CodedAt("run_policy")
    var runPolicy: MacAppJSONRunPolicy?
    @CodedAt("network_policy")
    var networkPolicy: MacAppJSONNetworkPolicy?
    @CodedAt("cache_policy")
    var cachePolicy: MacAppJSONCachePolicy?
    @CodedAt("sync_policy")
    var syncPolicy: MacAppJSONSyncPolicy?
}

/// Retry and timeout behavior shared by Codex jobs and other runnable work.
@Codable
struct MacAppJSONRunPolicy: Equatable {
    @CodedAt("timeout_seconds")
    var timeoutSeconds: Double?
    @CodedAt("max_attempts")
    var maxAttempts: Int?
    @CodedAt("retry_delay_seconds")
    var retryDelaySeconds: Double?
}

/// Network request behavior shared by remote connectors.
@Codable
struct MacAppJSONNetworkPolicy: Equatable {
    @CodedAt("timeout_seconds")
    var timeoutSeconds: Double?
    @CodedAt("retry_count")
    var retryCount: Int?
    @CodedAt("page_size")
    var pageSize: Int?
}

/// Cache freshness behavior shared by generated artifacts.
@Codable
struct MacAppJSONCachePolicy: Equatable {
    @CodedAt("max_age_seconds")
    var maxAgeSeconds: Double?
    @CodedAt("summary_max_age_seconds")
    var summaryMaxAgeSeconds: Double?
    @CodedAt("exec_questions_max_age_seconds")
    var execQuestionsMaxAgeSeconds: Double?
}

/// Scheduler/concurrency behavior shared by connector sync work.
@Codable
struct MacAppJSONSyncPolicy: Equatable {
    @CodedAt("concurrency_limit")
    var concurrencyLimit: Int?
    @CodedAt("adaptive_active_interval_seconds")
    var adaptiveActiveIntervalSeconds: Double?
    @CodedAt("adaptive_recent_interval_seconds")
    var adaptiveRecentIntervalSeconds: Double?
    @CodedAt("adaptive_background_interval_seconds")
    var adaptiveBackgroundIntervalSeconds: Double?
    @CodedAt("adaptive_jitter_percent")
    var adaptiveJitterPercent: Int?
}

/// Env-like non-secret runtime tuning.
@Codable
struct MacAppJSONEnvironmentConfiguration: Equatable {
    var common: MacAppJSONCommonPolicies?
    var defaults: MacAppJSONPolicyDefaults?
    @CodedAt("codex_executable")
    var codexExecutable: String?
    @CodedAt("runtime_root")
    var runtimeRoot: String?
    var webex: MacAppJSONEnvironmentWebexSettings?
    var imessage: MacAppJSONEnvironmentIMessageSettings?
}

/// Env-like Webex tuning that is not secret material.
@Codable
struct MacAppJSONEnvironmentWebexSettings: Equatable {
    @CodedAt("api_base_url")
    var apiBaseURL: String?
    @CodedAt("public_webhook_url")
    var publicWebhookURL: String?
    @CodedAt("oauth_token_file")
    var oauthTokenFile: String?
    @CodedAt("oauth_refresh_skew_seconds")
    var oauthRefreshSkewSeconds: Double?
    @CodedAt("oauth_refresh_token_skew_seconds")
    var oauthRefreshTokenSkewSeconds: Double?
    @CodedAt("network_policy")
    var networkPolicy: MacAppJSONNetworkPolicy?
    @CodedAt("sync_policy")
    var syncPolicy: MacAppJSONSyncPolicy?
}

/// Env-like iMessage source tuning.
@Codable
struct MacAppJSONEnvironmentIMessageSettings: Equatable {
    @CodedAt("chat_database_path")
    var chatDatabasePath: String?
    @CodedAt("busy_timeout_milliseconds")
    var busyTimeoutMilliseconds: Int?
}

/// Connector selection and connector-specific non-secret settings.
struct MacAppJSONConnectorsConfiguration: Codable, Equatable {
    var common: MacAppJSONCommonPolicies?
    var defaults: MacAppJSONPolicyDefaults?
    var enabled: [String]?
    var webex: MacAppJSONWebexConnectorConfiguration?
    var imessage: MacAppJSONIMessageConnectorConfiguration?
}

@Codable
struct MacAppJSONWebexConnectorConfiguration: Equatable {
    var enabled: Bool?
    @CodedAt("network_policy")
    var networkPolicy: MacAppJSONNetworkPolicy?
    @CodedAt("sync_policy")
    var syncPolicy: MacAppJSONSyncPolicy?
    @CodedAt("fixture_path")
    var fixturePath: String?
}

@Codable
struct MacAppJSONIMessageConnectorConfiguration: Equatable {
    var enabled: Bool?
    @CodedAt("chat_database_path")
    var chatDatabasePath: String?
    @CodedAt("busy_timeout_milliseconds")
    var busyTimeoutMilliseconds: Int?
    @CodedAt("fixture_path")
    var fixturePath: String?
}

/// Codex execution, cache, belief, and synthesis tuning.
@Codable
struct MacAppJSONCodexConfiguration: Equatable {
    var common: MacAppJSONCommonPolicies?
    var defaults: MacAppJSONPolicyDefaults?
    @CodedAt("run_policy")
    var runPolicy: MacAppJSONRunPolicy?
    @CodedAt("cache_policy")
    var cachePolicy: MacAppJSONCachePolicy?
    var beliefs: MacAppJSONCodexBeliefPolicy?
    @CodedAt("question_synthesis")
    var questionSynthesis: MacAppJSONCodexQuestionSynthesisPolicy?
}

/// Belief reconciliation scheduling and chunking tuning.
@Codable
struct MacAppJSONCodexBeliefPolicy: Equatable {
    @CodedAt("stale_hours")
    var staleHours: Int?
    @CodedAt("evidence_chunk_size")
    var evidenceChunkSize: Int?
    @CodedAt("max_incremental_window_days")
    var maxIncrementalWindowDays: Int?
}

/// Question-synthesis prompt input sizing and optional run-policy override.
@Codable
struct MacAppJSONCodexQuestionSynthesisPolicy: Equatable {
    @CodedAt("run_policy")
    var runPolicy: MacAppJSONRunPolicy?
    @CodedAt("seed_candidate_limit")
    var seedCandidateLimit: Int?
    @CodedAt("query_history_limit")
    var queryHistoryLimit: Int?
    @CodedAt("prompt_history_limit")
    var promptHistoryLimit: Int?
    @CodedAt("candidate_evidence_limit")
    var candidateEvidenceLimit: Int?
    @CodedAt("output_limit")
    var outputLimit: Int?
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
@Codable
struct MacAppJSONQuestionGenerationPrivacySettings: Equatable {
    @CodedAt("anonymize_users")
    var anonymizeUsers: Bool?
    @CodedAt("redact_urls")
    var redactURLs: Bool?
    @CodedAt("redact_emails")
    var redactEmails: Bool?
}

/// Local topic extraction settings.
@Codable
struct MacAppJSONQuestionGenerationTopicSettings: Equatable {
    var enabled: Bool?
    @CodedAt("number_of_topics")
    var numberOfTopics: Int?
    @CodedAt("minimum_topic_size")
    var minimumTopicSize: Int?
}

/// Local question extraction settings.
@Codable
struct MacAppJSONQuestionGenerationQuestionSettings: Equatable {
    @CodedAt("top_n")
    var topN: Int?
    @CodedAt("enabled_categories")
    var enabledCategories: [String]?
}

/// Cubicle-specific fallback and publication thresholds.
@Codable
struct MacAppJSONQuestionGenerationCubicleSettings: Equatable {
    @CodedAt("fallback_draft_limit")
    var fallbackDraftLimit: Int?
    @CodedAt("generated_question_limit")
    var generatedQuestionLimit: Int?
    @CodedAt("publishable_question_limit")
    var publishableQuestionLimit: Int?
    @CodedAt("evidence_limit")
    var evidenceLimit: Int?
    @CodedAt("core_evidence_limit")
    var coreEvidenceLimit: Int?
}

/// Test-app mode uses stable input files that cleanup must never delete.
@Codable
struct MacAppJSONTestModeConfiguration: Equatable {
    var enabled: Bool?
    var profile: String?
    @CodedAt("fixture_root")
    var fixtureRoot: String?
    @CodedAt("target_data")
    var targetData: String?
    var settings: String?
    @CodedAt("protect_paths")
    var protectPaths: [String]?
    @CodedAt("connector_fixtures")
    var connectorFixtures: [String: String]?
}
