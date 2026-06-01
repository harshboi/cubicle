import Foundation

struct ConfigTargetSourceMetadata: Hashable {
    var fileName: String
    var lineNumber: Int?
    var entryKey: String
    var parseMode: String

    var deterministicID: String {
        let lineSegment = lineNumber.map(String.init) ?? "na"
        return "\(fileName)#\(lineSegment):\(entryKey):\(parseMode)"
    }
}

struct ConfigTarget: Identifiable, Hashable {
    enum Kind: String {
        case person
        case space
        case unknown
    }

    var id: String {
        let normalizedRoomID = Self.normalizeRoomID(roomID)
        if !normalizedRoomID.isEmpty {
            return "\(kind.rawValue):room:\(normalizedRoomID)"
        }

        let normalizedEmail = Self.normalizeEmail(email)
        if !normalizedEmail.isEmpty {
            return "\(kind.rawValue):email:\(normalizedEmail)"
        }

        let normalizedLabel = Self.normalizeLabel(label)
        return "\(kind.rawValue):label:\(normalizedLabel)"
    }

    var kind: Kind
    var label: String
    var roomID: String
    var roomType: String = ""
    var email: String
    var autoReply: Bool = false
    var iMessageHandles: [String] = []
    var sourceMetadata: ConfigTargetSourceMetadata? = nil

    private static func normalizeLabel(_ value: String) -> String {
        value
            .trimmingCharacters(in: .whitespacesAndNewlines)
            .lowercased()
            .replacingOccurrences(of: #"\s+"#, with: " ", options: .regularExpression)
    }

    private static func normalizeRoomID(_ value: String) -> String {
        value
            .trimmingCharacters(in: .whitespacesAndNewlines)
            .replacingOccurrences(of: #"\s+"#, with: "", options: .regularExpression)
    }

    private static func normalizeEmail(_ value: String) -> String {
        value.trimmingCharacters(in: .whitespacesAndNewlines).lowercased()
    }
}

enum SystemSettingKey: String, CaseIterable, Hashable {
    case debug = "debug"
    case backgroundStatus = "background_status"
    case webexSyncEnabled = "webex_sync_enabled"
    case autoQueryAllEnabled = "auto_query_all_enabled"
    case priorityRefreshPausesBackground = "priority_refresh_pauses_background"
    case codexEnabled = "codex_enabled"
    case codexAskEnabled = "codex_ask_enabled"
    case codexQuestionSynthesisEnabled = "codex_question_synthesis_enabled"
    case codexPersonSummariesEnabled = "codex_person_summaries_enabled"
    case codexSpaceSummariesEnabled = "codex_space_summaries_enabled"
    case codexClusterTitlesEnabled = "codex_cluster_titles_enabled"
    case codexExecQuestionsEnabled = "codex_exec_questions_enabled"
    case codexBeliefsEnabled = "codex_beliefs_enabled"
    case codexModel = "codex_model"
    case codexReasoningLevel = "codex_reasoning_level"
    case webexSyncMinutes = "webex_sync_minutes"
    case autoQueryAllMinutes = "auto_query_all_minutes"
    case trackedActionsRefreshMinutes = "tracked_actions_refresh_minutes"
    case personFocusRefreshMinutes = "person_focus_refresh_minutes"
    case personFocusDays = "person_focus_days"
    case personFocusAnalysisCadenceHours = "person_focus_analysis_cadence_hours"
    case spaceFocusRefreshMinutes = "space_focus_refresh_minutes"
    case spaceFocusDays = "space_focus_days"
    case spaceFocusAnalysisCadenceHours = "space_focus_analysis_cadence_hours"
    case transcriptionEnabled = "transcription_enabled"
    case transcriptionDiarizationEnabled = "transcription_diarization_enabled"
    case transcriptionLanguageMode = "transcription_language_mode"
    case transcriptionMicrophoneGain = "transcription_microphone_gain"
    case transcriptionAWSEndpoint = "transcription_aws_endpoint"
    case pollSeconds = "poll_seconds"
}

enum CodexFeatureToggle: String, CaseIterable, Identifiable, Hashable {
    case askCodex
    case questionSynthesis
    case personFocusSummaries
    case spaceFocusSummaries
    case clusterTitles
    case execQuestions
    case beliefs

    var id: String { rawValue }

    var settingKey: SystemSettingKey {
        switch self {
        case .askCodex:
            return .codexAskEnabled
        case .questionSynthesis:
            return .codexQuestionSynthesisEnabled
        case .personFocusSummaries:
            return .codexPersonSummariesEnabled
        case .spaceFocusSummaries:
            return .codexSpaceSummariesEnabled
        case .clusterTitles:
            return .codexClusterTitlesEnabled
        case .execQuestions:
            return .codexExecQuestionsEnabled
        case .beliefs:
            return .codexBeliefsEnabled
        }
    }

    var displayName: String {
        switch self {
        case .askCodex:
            return "Ask Codex"
        case .questionSynthesis:
            return "Question synthesis"
        case .personFocusSummaries:
            return "Person Focus summaries"
        case .spaceFocusSummaries:
            return "Space Focus summaries"
        case .clusterTitles:
            return "Cluster titles"
        case .execQuestions:
            return "Exec questions"
        case .beliefs:
            return "Belief analysis"
        }
    }

    var settingsSubtitle: String {
        switch self {
        case .askCodex:
            return "Manual Ask Codex runs"
        case .questionSynthesis:
            return "Questions generated from focus evidence"
        case .personFocusSummaries:
            return "Codex-written Person Focus summaries"
        case .spaceFocusSummaries:
            return "Codex-written Space Focus summaries"
        case .clusterTitles:
            return "Codex titles for local conversation clusters"
        case .execQuestions:
            return "Exec-question sections inside Space Focus"
        case .beliefs:
            return "Second-order belief reconciliation"
        }
    }

    var symbolName: String {
        switch self {
        case .askCodex:
            return "sparkles"
        case .questionSynthesis:
            return "questionmark.bubble"
        case .personFocusSummaries:
            return "person.crop.circle.badge.clock"
        case .spaceFocusSummaries:
            return "bubble.left.and.bubble.right.fill"
        case .clusterTitles:
            return "textformat"
        case .execQuestions:
            return "person.crop.circle.badge.questionmark"
        case .beliefs:
            return "brain.head.profile"
        }
    }
}

enum CodexModelSelection: String, CaseIterable, Identifiable {
    case gpt55 = "gpt-5.5"
    case gpt54 = "gpt-5.4"
    case gpt54Mini = "gpt-5.4-mini"
    case gpt53Codex = "gpt-5.3-codex"
    case gpt53CodexSpark = "gpt-5.3-codex-spark-preview"
    case gpt52 = "gpt-5.2"

    var id: String { rawValue }

    var displayName: String {
        switch self {
        case .gpt55:
            return "GPT-5.5"
        case .gpt54:
            return "GPT-5.4"
        case .gpt54Mini:
            return "GPT-5.4 Mini"
        case .gpt53Codex:
            return "GPT-5.3 Codex"
        case .gpt53CodexSpark:
            return "GPT-5.3 Codex Spark"
        case .gpt52:
            return "GPT-5.2"
        }
    }

    static func normalized(_ rawValue: String?) -> CodexModelSelection {
        guard let rawValue = rawValue?.trimmingCharacters(in: .whitespacesAndNewlines),
              let value = CodexModelSelection(rawValue: rawValue) else {
            return .gpt55
        }
        return value
    }
}

enum CodexReasoningLevel: String, CaseIterable, Identifiable {
    case low
    case medium
    case high
    case xhigh

    var id: String { rawValue }

    var displayName: String {
        switch self {
        case .low:
            return "Low"
        case .medium:
            return "Medium"
        case .high:
            return "High"
        case .xhigh:
            return "Extra High"
        }
    }

    static func normalized(_ rawValue: String?) -> CodexReasoningLevel {
        guard let rawValue = rawValue?.trimmingCharacters(in: .whitespacesAndNewlines),
              let value = CodexReasoningLevel(rawValue: rawValue) else {
            return .xhigh
        }
        return value
    }
}

struct SystemSettings: Equatable {
    static let persistedVersion = 8
    static let focusDaysBounds = 7...90
    static let focusAnalysisCadenceHoursBounds = 1...168
    static let transcriptionMicrophoneGainBounds = 1...32
    static let defaultTranscriptionMicrophoneGain = 18

    var debug: Bool = false
    var backgroundStatus: Bool = true
    var webexSyncEnabled: Bool = true
    var autoQueryAllEnabled: Bool = false
    var priorityRefreshPausesBackground: Bool = true
    var codexEnabled: Bool = true
    var codexAskEnabled: Bool = true
    var codexQuestionSynthesisEnabled: Bool = true
    var codexPersonSummariesEnabled: Bool = true
    var codexSpaceSummariesEnabled: Bool = true
    var codexClusterTitlesEnabled: Bool = true
    var codexExecQuestionsEnabled: Bool = true
    var codexBeliefsEnabled: Bool = true
    var codexModel: CodexModelSelection = .gpt55
    var codexReasoningLevel: CodexReasoningLevel = .xhigh
    var webexSyncMinutes: Int = 5
    var autoQueryAllMinutes: Int = 60
    var trackedActionsRefreshMinutes: Int = 15
    var personFocusRefreshMinutes: Int = 15
    var personFocusDays: Int = 30
    var personFocusAnalysisCadenceHours: Int = 24
    var spaceFocusRefreshMinutes: Int = 15
    var spaceFocusDays: Int = 60
    var spaceFocusAnalysisCadenceHours: Int = 24
    var transcriptionEnabled: Bool = false
    var transcriptionDiarizationEnabled: Bool = false
    var transcriptionLanguageMode: TranscriptionLanguageMode = .englishToEnglish
    var transcriptionMicrophoneGain: Int = Self.defaultTranscriptionMicrophoneGain
    var transcriptionAWSEndpoint: String = ""
    var transcriptionAuthToken: String? = nil
    var pollSeconds: Int = 300
    var updatedAt: Date?

    func boolValue(for key: SystemSettingKey) -> Bool {
        switch key {
        case .debug:
            return debug
        case .backgroundStatus:
            return backgroundStatus
        case .webexSyncEnabled:
            return webexSyncEnabled
        case .autoQueryAllEnabled:
            return autoQueryAllEnabled
        case .priorityRefreshPausesBackground:
            return priorityRefreshPausesBackground
        case .codexEnabled:
            return codexEnabled
        case .codexAskEnabled:
            return codexAskEnabled
        case .codexQuestionSynthesisEnabled:
            return codexQuestionSynthesisEnabled
        case .codexPersonSummariesEnabled:
            return codexPersonSummariesEnabled
        case .codexSpaceSummariesEnabled:
            return codexSpaceSummariesEnabled
        case .codexClusterTitlesEnabled:
            return codexClusterTitlesEnabled
        case .codexExecQuestionsEnabled:
            return codexExecQuestionsEnabled
        case .codexBeliefsEnabled:
            return codexBeliefsEnabled
        case .transcriptionEnabled:
            return transcriptionEnabled
        case .transcriptionDiarizationEnabled:
            return transcriptionDiarizationEnabled
        case .webexSyncMinutes,
             .autoQueryAllMinutes,
             .codexModel,
             .codexReasoningLevel,
             .transcriptionLanguageMode,
             .transcriptionMicrophoneGain,
             .transcriptionAWSEndpoint,
             .trackedActionsRefreshMinutes,
             .personFocusRefreshMinutes,
             .personFocusDays,
             .personFocusAnalysisCadenceHours,
             .spaceFocusRefreshMinutes,
             .spaceFocusDays,
             .spaceFocusAnalysisCadenceHours,
             .pollSeconds:
            return false
        }
    }

    func intValue(for key: SystemSettingKey) -> Int {
        switch key {
        case .webexSyncMinutes:
            return webexSyncMinutes
        case .autoQueryAllMinutes:
            return autoQueryAllMinutes
        case .trackedActionsRefreshMinutes:
            return trackedActionsRefreshMinutes
        case .personFocusRefreshMinutes:
            return personFocusRefreshMinutes
        case .personFocusDays:
            return personFocusDays
        case .personFocusAnalysisCadenceHours:
            return personFocusAnalysisCadenceHours
        case .spaceFocusRefreshMinutes:
            return spaceFocusRefreshMinutes
        case .spaceFocusDays:
            return spaceFocusDays
        case .spaceFocusAnalysisCadenceHours:
            return spaceFocusAnalysisCadenceHours
        case .transcriptionMicrophoneGain:
            return transcriptionMicrophoneGain
        case .pollSeconds:
            return pollSeconds
        case .codexModel,
             .codexReasoningLevel,
             .transcriptionLanguageMode,
             .transcriptionAWSEndpoint:
            return 0
        case .debug,
             .backgroundStatus,
             .webexSyncEnabled,
             .autoQueryAllEnabled,
             .priorityRefreshPausesBackground,
             .codexEnabled,
             .codexAskEnabled,
             .codexQuestionSynthesisEnabled,
             .codexPersonSummariesEnabled,
             .codexSpaceSummariesEnabled,
             .codexClusterTitlesEnabled,
             .codexExecQuestionsEnabled,
             .codexBeliefsEnabled,
             .transcriptionEnabled,
             .transcriptionDiarizationEnabled:
            return 0
        }
    }

    func stringValue(for key: SystemSettingKey) -> String {
        switch key {
        case .codexModel:
            return codexModel.rawValue
        case .codexReasoningLevel:
            return codexReasoningLevel.rawValue
        case .transcriptionLanguageMode:
            return transcriptionLanguageMode.rawValue
        case .transcriptionAWSEndpoint:
            return transcriptionAWSEndpoint
        case .debug,
             .backgroundStatus,
             .webexSyncEnabled,
             .autoQueryAllEnabled,
             .priorityRefreshPausesBackground,
             .codexEnabled,
             .codexAskEnabled,
             .codexQuestionSynthesisEnabled,
             .codexPersonSummariesEnabled,
             .codexSpaceSummariesEnabled,
             .codexClusterTitlesEnabled,
             .codexExecQuestionsEnabled,
             .codexBeliefsEnabled,
             .transcriptionEnabled,
             .transcriptionDiarizationEnabled,
             .webexSyncMinutes,
             .autoQueryAllMinutes,
             .trackedActionsRefreshMinutes,
             .personFocusRefreshMinutes,
             .personFocusDays,
             .personFocusAnalysisCadenceHours,
             .spaceFocusRefreshMinutes,
             .spaceFocusDays,
             .spaceFocusAnalysisCadenceHours,
             .transcriptionMicrophoneGain,
             .pollSeconds:
            return ""
        }
    }

    mutating func setBool(_ value: Bool, for key: SystemSettingKey) {
        switch key {
        case .debug:
            debug = value
        case .backgroundStatus:
            backgroundStatus = value
        case .webexSyncEnabled:
            webexSyncEnabled = value
        case .autoQueryAllEnabled:
            autoQueryAllEnabled = value
        case .priorityRefreshPausesBackground:
            priorityRefreshPausesBackground = value
        case .codexEnabled:
            codexEnabled = value
        case .codexAskEnabled:
            codexAskEnabled = value
        case .codexQuestionSynthesisEnabled:
            codexQuestionSynthesisEnabled = value
        case .codexPersonSummariesEnabled:
            codexPersonSummariesEnabled = value
        case .codexSpaceSummariesEnabled:
            codexSpaceSummariesEnabled = value
        case .codexClusterTitlesEnabled:
            codexClusterTitlesEnabled = value
        case .codexExecQuestionsEnabled:
            codexExecQuestionsEnabled = value
        case .codexBeliefsEnabled:
            codexBeliefsEnabled = value
        case .transcriptionEnabled:
            transcriptionEnabled = value
        case .transcriptionDiarizationEnabled:
            transcriptionDiarizationEnabled = value
        case .webexSyncMinutes,
             .autoQueryAllMinutes,
             .codexModel,
             .codexReasoningLevel,
             .transcriptionLanguageMode,
             .transcriptionMicrophoneGain,
             .transcriptionAWSEndpoint,
             .trackedActionsRefreshMinutes,
             .personFocusRefreshMinutes,
             .personFocusDays,
             .personFocusAnalysisCadenceHours,
             .spaceFocusRefreshMinutes,
             .spaceFocusDays,
             .spaceFocusAnalysisCadenceHours,
             .pollSeconds:
            break
        }
    }

    mutating func setString(_ value: String, for key: SystemSettingKey) {
        switch key {
        case .codexModel:
            codexModel = CodexModelSelection.normalized(value)
        case .codexReasoningLevel:
            codexReasoningLevel = CodexReasoningLevel.normalized(value)
        case .transcriptionLanguageMode:
            transcriptionLanguageMode = TranscriptionLanguageMode.normalized(value)
        case .transcriptionAWSEndpoint:
            transcriptionAWSEndpoint = value.trimmingCharacters(in: .whitespacesAndNewlines)
        case .debug,
             .backgroundStatus,
             .webexSyncEnabled,
             .autoQueryAllEnabled,
             .priorityRefreshPausesBackground,
             .codexEnabled,
             .codexAskEnabled,
             .codexQuestionSynthesisEnabled,
             .codexPersonSummariesEnabled,
             .codexSpaceSummariesEnabled,
             .codexClusterTitlesEnabled,
             .codexExecQuestionsEnabled,
             .codexBeliefsEnabled,
             .transcriptionEnabled,
             .transcriptionDiarizationEnabled,
             .webexSyncMinutes,
             .autoQueryAllMinutes,
             .transcriptionMicrophoneGain,
             .trackedActionsRefreshMinutes,
             .personFocusRefreshMinutes,
             .personFocusDays,
             .personFocusAnalysisCadenceHours,
             .spaceFocusRefreshMinutes,
             .spaceFocusDays,
             .spaceFocusAnalysisCadenceHours,
             .pollSeconds:
            break
        }
    }

    mutating func setInt(_ value: Int, for key: SystemSettingKey) {
        switch key {
        case .webexSyncMinutes:
            webexSyncMinutes = Self.clamped(value, to: 1...1440)
            pollSeconds = webexSyncMinutes * 60
        case .autoQueryAllMinutes:
            autoQueryAllMinutes = Self.clamped(value, to: 1...1440)
        case .trackedActionsRefreshMinutes:
            trackedActionsRefreshMinutes = Self.clamped(value, to: 1...1440)
        case .personFocusRefreshMinutes:
            personFocusRefreshMinutes = Self.clamped(value, to: 1...1440)
        case .personFocusDays:
            personFocusDays = Self.clamped(value, to: Self.focusDaysBounds)
        case .personFocusAnalysisCadenceHours:
            personFocusAnalysisCadenceHours = Self.clamped(value, to: Self.focusAnalysisCadenceHoursBounds)
        case .spaceFocusRefreshMinutes:
            spaceFocusRefreshMinutes = Self.clamped(value, to: 1...1440)
        case .spaceFocusDays:
            spaceFocusDays = Self.clamped(value, to: Self.focusDaysBounds)
        case .spaceFocusAnalysisCadenceHours:
            spaceFocusAnalysisCadenceHours = Self.clamped(value, to: Self.focusAnalysisCadenceHoursBounds)
        case .transcriptionMicrophoneGain:
            transcriptionMicrophoneGain = Self.clamped(value, to: Self.transcriptionMicrophoneGainBounds)
        case .pollSeconds:
            pollSeconds = max(1, value)
            webexSyncMinutes = Self.clamped(Int(ceil(Double(pollSeconds) / 60.0)), to: 1...1440)
        case .codexModel,
             .codexReasoningLevel,
             .transcriptionLanguageMode,
             .transcriptionAWSEndpoint:
            break
        case .debug,
             .backgroundStatus,
             .webexSyncEnabled,
             .autoQueryAllEnabled,
             .priorityRefreshPausesBackground,
             .codexEnabled,
             .codexAskEnabled,
             .codexQuestionSynthesisEnabled,
             .codexPersonSummariesEnabled,
             .codexSpaceSummariesEnabled,
             .codexClusterTitlesEnabled,
             .codexExecQuestionsEnabled,
             .codexBeliefsEnabled,
             .transcriptionEnabled,
             .transcriptionDiarizationEnabled:
            break
        }
    }

    func codexFeatureEnabled(_ feature: CodexFeatureToggle) -> Bool {
        codexEnabled && boolValue(for: feature.settingKey)
    }

    static func clamped(_ value: Int, to bounds: ClosedRange<Int>) -> Int {
        min(max(value, bounds.lowerBound), bounds.upperBound)
    }
}

enum ConfigStoreError: LocalizedError {
    case oauthSettingsPayloadUnreadable(URL, Error)
    case oauthSettingsPayloadInvalidJSON(URL, Error)
    case oauthSettingsPayloadInvalidShape(URL)
    case oauthTokenFileMissing([URL])
    case oauthTokenPayloadUnreadable(URL, Error)
    case oauthTokenPayloadInvalidJSON(URL, Error)
    case oauthTokenPayloadInvalidShape(URL)
    case oauthAccessTokenMissing(URL)
    case focusTargetMissingRoomID(String)
    case focusTargetMissingEmail(String)
    case personIMessageHandleInvalid(String)
    case focusTargetUnsupportedKind(String)
    case focusTargetUnsupportedSpaceKind(String)

    var errorDescription: String? {
        switch self {
        case .oauthSettingsPayloadUnreadable(let url, let error):
            return "Could not read OAuth settings file \(url.path): \(error.localizedDescription)"
        case .oauthSettingsPayloadInvalidJSON(let url, let error):
            return "OAuth settings file \(url.path) is not valid JSON: \(error.localizedDescription)"
        case .oauthSettingsPayloadInvalidShape(let url):
            return "OAuth settings file \(url.path) must contain a JSON object."
        case .oauthTokenFileMissing(let candidates):
            let listed = candidates.map(\.path).joined(separator: ", ")
            return "No Webex OAuth token file found. Checked: \(listed)"
        case .oauthTokenPayloadUnreadable(let url, let error):
            return "Could not read OAuth token file \(url.path): \(error.localizedDescription)"
        case .oauthTokenPayloadInvalidJSON(let url, let error):
            return "OAuth token file \(url.path) is not valid JSON: \(error.localizedDescription)"
        case .oauthTokenPayloadInvalidShape(let url):
            return "OAuth token file \(url.path) must contain a JSON object."
        case .oauthAccessTokenMissing(let url):
            return "OAuth token file \(url.path) does not contain an access token."
        case .focusTargetMissingRoomID(let label):
            return "Focus target \(label) does not have a stable Webex room id."
        case .focusTargetMissingEmail(let label):
            return "Focus target \(label) does not have a stable person email."
        case .personIMessageHandleInvalid(let value):
            return "iMessage handle \(value) must be a phone number or iMessage email."
        case .focusTargetUnsupportedKind(let label):
            return "Focus target \(label) is not a person target."
        case .focusTargetUnsupportedSpaceKind(let label):
            return "Focus target \(label) is not a space target."
        }
    }
}

enum OAuthProviderKind: String, CaseIterable, Identifiable {
    case webex
    case outlook

    var id: String { rawValue }

    var displayName: String {
        switch self {
        case .webex:
            return "Webex"
        case .outlook:
            return "Outlook"
        }
    }
}

extension OAuthProviderKind {
    var oauthSettingsSectionNames: [String] {
        switch self {
        case .webex:
            return ["webex"]
        case .outlook:
            return ["outlook", "ms_graph", "microsoft"]
        }
    }

    var oauthClientIDSettingKeys: [String] {
        switch self {
        case .webex:
            return ["WEBEX_OAUTH_CLIENT_ID"]
        case .outlook:
            return ["OUTLOOK_OAUTH_CLIENT_ID", "MS_GRAPH_OAUTH_CLIENT_ID", "MICROSOFT_OAUTH_CLIENT_ID"]
        }
    }

    var oauthClientSecretSettingKeys: [String] {
        switch self {
        case .webex:
            return ["WEBEX_OAUTH_CLIENT_SECRET"]
        case .outlook:
            return ["OUTLOOK_OAUTH_CLIENT_SECRET", "MS_GRAPH_OAUTH_CLIENT_SECRET", "MICROSOFT_OAUTH_CLIENT_SECRET"]
        }
    }

    var oauthRedirectURISettingKeys: [String] {
        switch self {
        case .webex:
            return ["WEBEX_OAUTH_REDIRECT_URI"]
        case .outlook:
            return ["OUTLOOK_OAUTH_REDIRECT_URI", "MS_GRAPH_OAUTH_REDIRECT_URI"]
        }
    }

    var oauthScopeSettingKeys: [String] {
        switch self {
        case .webex:
            return ["WEBEX_OAUTH_SCOPE"]
        case .outlook:
            return ["OUTLOOK_OAUTH_SCOPE", "MS_GRAPH_OAUTH_SCOPE"]
        }
    }

    var oauthTenantSettingKeys: [String] {
        switch self {
        case .webex:
            return []
        case .outlook:
            return ["OUTLOOK_OAUTH_TENANT", "MS_GRAPH_OAUTH_TENANT"]
        }
    }
}

struct OAuthAppSettings: Equatable {
    var sourceFile: URL?
    var clientID: String?
    var clientSecret: String?
    var redirectURI: String?
    var scope: String?
    var tenant: String?
}

struct OAuthTokenRecord: Equatable {
    var sourceFile: URL
    var accessToken: String
    var refreshToken: String
    var scope: String
    var obtainedAt: Date?
    var accessTokenExpiresAt: Date?
    var refreshTokenExpiresAt: Date?
    var expiresIn: Int?
    var refreshTokenExpiresIn: Int?

    var hasAccessToken: Bool { !accessToken.isEmpty }
    var hasRefreshToken: Bool { !refreshToken.isEmpty }

    func isAccessTokenExpired(at now: Date = Date()) -> Bool {
        guard let accessTokenExpiresAt else { return false }
        return accessTokenExpiresAt <= now
    }

    func isAccessTokenExpiringSoon(skewSeconds: TimeInterval, at now: Date = Date()) -> Bool {
        guard let accessTokenExpiresAt else { return false }
        return accessTokenExpiresAt <= now.addingTimeInterval(max(0, skewSeconds))
    }

    func isRefreshTokenExpired(at now: Date = Date()) -> Bool {
        guard let refreshTokenExpiresAt else { return false }
        return refreshTokenExpiresAt <= now
    }

    func isRefreshTokenExpiringSoon(skewSeconds: TimeInterval, at now: Date = Date()) -> Bool {
        guard let refreshTokenExpiresAt else { return false }
        return refreshTokenExpiresAt <= now.addingTimeInterval(max(0, skewSeconds))
    }
}

enum OAuthTokenHealthState: String, Equatable {
    case missingTokenFile
    case invalidTokenFile
    case missingAccessToken
    case missingRefreshToken
    case expired
    case expiringSoon
    case refreshExpired
    case refreshExpiringSoon
    case healthy
    case unknownExpiry
}

struct OAuthTokenHealth: Equatable {
    var state: OAuthTokenHealthState
    var record: OAuthTokenRecord?
    var parseError: String?
}

final class ConfigStore {
    private static let transcriptionAuthTokenKeychainAccount = "transcription.service_token"

    let configuration: RuntimeConfiguration
    private let defaultOAuthTokenFilename = ".webex_oauth_tokens.json"
    private let defaultOutlookOAuthTokenFilename = ".outlook_oauth_tokens.json"
    private let oauthSettingsFilename = "oauth-settings.json"
    private let keychainStore = OAuthKeychainStore()

    init(configuration: RuntimeConfiguration = .current) {
        self.configuration = configuration
    }

    var configDirectory: URL {
        configuration.runtimeRoot.appendingPathComponent("config", isDirectory: true)
    }

    var systemSettingsURL: URL {
        configDirectory.appendingPathComponent("pine-ui-settings.json")
    }

    var askCodexQueryHistoryURL: URL {
        configDirectory.appendingPathComponent("ask-codex-query-history.json")
    }

    var refreshCheckpointURL: URL {
        configDirectory.appendingPathComponent("refresh-checkpoint.json")
    }

    var oauthSettingsURL: URL {
        configDirectory.appendingPathComponent(oauthSettingsFilename)
    }

    var mapFileURL: URL {
        configDirectory.appendingPathComponent("map.txt")
    }

    var importantTargetsURL: URL {
        configDirectory.appendingPathComponent("important-senders.txt")
    }

    var importantExecutivesURL: URL {
        configDirectory.appendingPathComponent("importantexec.txt")
    }

    var personFocusPreferencesURL: URL {
        configDirectory.appendingPathComponent("person-focus-people.json")
    }

    var spaceFocusPreferencesURL: URL {
        configDirectory.appendingPathComponent("space-focus-spaces.json")
    }

    var watchSourcesDescription: String {
        [
            "Important: \(importantTargetsURL.path)",
            "Beliefs: \(configDirectory.appendingPathComponent("belieftargets.txt").path)"
        ].joined(separator: " | ")
    }

    func importantExecutives() throws -> [ConfigTarget] {
        try loadTargets(filename: "importantexec.txt")
    }

    func beliefTargets() throws -> [ConfigTarget] {
        try loadTargets(filename: "belieftargets.txt")
    }

    func importantTargets() throws -> [ConfigTarget] {
        try loadTargets(filename: "important-senders.txt")
    }

    func importantSpaces() throws -> [ConfigTarget] {
        let preferences = try loadJSONFocusPreferences(filename: "space-focus-spaces.json", mapKey: "spaces")
        return try importantTargets()
            .filter { $0.kind == .space }
            .map { applyFocusPreference(to: $0, preferences: preferences, key: normalizeRoomID($0.roomID)) }
    }

    func importantPeople() throws -> [ConfigTarget] {
        let preferences = try loadJSONFocusPreferences(filename: "person-focus-people.json", mapKey: "people")
        return try importantTargets()
            .filter { $0.kind == .person }
            .map { applyFocusPreference(to: $0, preferences: preferences, key: normalizeEmail($0.email)) }
    }

    func personFocusManagementTargets() throws -> [ConfigTarget] {
        let preferences = try loadJSONFocusPreferences(filename: "person-focus-people.json", mapKey: "people")
        return try loadManagedTargets(filename: "important-senders.txt")
            .filter { $0.kind == .person }
            .map { applyFocusPreference(to: $0, preferences: preferences, key: normalizeEmail($0.email)) }
            .sorted(by: targetDisplaySort)
    }

    func spaceFocusManagementTargets() throws -> [ConfigTarget] {
        let preferences = try loadJSONFocusPreferences(filename: "space-focus-spaces.json", mapKey: "spaces")
        return try loadManagedTargets(filename: "important-senders.txt")
            .filter { $0.kind == .space }
            .map { applyFocusPreference(to: $0, preferences: preferences, key: normalizeRoomID($0.roomID)) }
            .sorted(by: targetDisplaySort)
    }

    func execFocusManagementTargets() throws -> [ConfigTarget] {
        try loadManagedTargets(filename: "importantexec.txt")
            .filter { $0.kind == .person }
            .sorted(by: targetDisplaySort)
    }

    func personFocusAddablePeople() throws -> [ConfigTarget] {
        let existingKeys = Set(try personFocusManagementTargets().map(fileIdentityKey))
        return try mapPersonCandidates(excluding: existingKeys)
    }

    func spaceFocusAddableSpaces() throws -> [ConfigTarget] {
        let existingKeys = Set(try spaceFocusManagementTargets().map(fileIdentityKey))
        return try mapSpaceCandidates(excluding: existingKeys)
    }

    func execFocusAddablePeople() throws -> [ConfigTarget] {
        let existingKeys = Set(try execFocusManagementTargets().map(fileIdentityKey))
        return try mapPersonCandidates(excluding: existingKeys)
    }

    @discardableResult
    func addPersonFocusTarget(_ target: ConfigTarget) throws -> Bool {
        guard target.kind == .person else {
            throw ConfigStoreError.focusTargetUnsupportedKind(target.label)
        }
        return try appendTargetByRoomIfMissing(target, to: importantTargetsURL)
    }

    @discardableResult
    func addSpaceFocusTarget(_ target: ConfigTarget) throws -> Bool {
        guard target.kind == .space else {
            throw ConfigStoreError.focusTargetUnsupportedSpaceKind(target.label)
        }
        return try appendTargetByRoomIfMissing(target, to: importantTargetsURL)
    }

    @discardableResult
    func addExecFocusTarget(_ target: ConfigTarget) throws -> Bool {
        guard target.kind == .person else {
            throw ConfigStoreError.focusTargetUnsupportedKind(target.label)
        }
        return try appendTargetByRoomIfMissing(target, to: importantExecutivesURL)
    }

    @discardableResult
    func removePersonFocusTarget(_ target: ConfigTarget) throws -> Int {
        try removeEntriesByRoom(target, from: importantTargetsURL)
    }

    @discardableResult
    func removeSpaceFocusTarget(_ target: ConfigTarget) throws -> Int {
        try removeEntriesByRoom(target, from: importantTargetsURL)
    }

    @discardableResult
    func removeExecFocusTarget(_ target: ConfigTarget) throws -> Int {
        try removeEntriesByRoom(target, from: importantExecutivesURL)
    }

    func setPersonFocusAutoReply(_ enabled: Bool, for target: ConfigTarget) throws {
        let key = normalizeEmail(target.email)
        guard !key.isEmpty else {
            throw ConfigStoreError.focusTargetMissingEmail(target.label)
        }
        var preferences = try loadJSONFocusPreferences(filename: "person-focus-people.json", mapKey: "people")
        var preference = preferences[key] ?? FocusPreference(label: "", autoReply: false)
        preference.autoReply = enabled
        preferences[key] = preference
        try savePersonFocusPeoplePreferences(preferences)
    }

    @discardableResult
    func addPersonIMessageHandle(_ rawHandle: String, to target: ConfigTarget) throws -> Bool {
        let key = normalizeEmail(target.email)
        guard !key.isEmpty else {
            throw ConfigStoreError.focusTargetMissingEmail(target.label)
        }
        guard let handle = IMessageHandleNormalizer.normalizedStorageValue(rawHandle) else {
            throw ConfigStoreError.personIMessageHandleInvalid(rawHandle)
        }
        var preferences = try loadJSONFocusPreferences(filename: "person-focus-people.json", mapKey: "people")
        var preference = preferences[key] ?? FocusPreference(label: "", autoReply: target.autoReply)
        if preference.iMessageHandles.contains(where: { $0.caseInsensitiveCompare(handle) == .orderedSame }) {
            return false
        }
        preference.iMessageHandles.append(handle)
        preference.iMessageHandles = dedupedIMessageHandles(preference.iMessageHandles)
        preferences[key] = preference
        try savePersonFocusPeoplePreferences(preferences)
        return true
    }

    @discardableResult
    func removePersonIMessageHandle(_ rawHandle: String, from target: ConfigTarget) throws -> Bool {
        let key = normalizeEmail(target.email)
        guard !key.isEmpty else {
            throw ConfigStoreError.focusTargetMissingEmail(target.label)
        }
        guard let handle = IMessageHandleNormalizer.normalizedStorageValue(rawHandle) else {
            throw ConfigStoreError.personIMessageHandleInvalid(rawHandle)
        }
        var preferences = try loadJSONFocusPreferences(filename: "person-focus-people.json", mapKey: "people")
        guard var preference = preferences[key] else {
            return false
        }
        let originalCount = preference.iMessageHandles.count
        preference.iMessageHandles.removeAll { $0.caseInsensitiveCompare(handle) == .orderedSame }
        preferences[key] = preference
        try savePersonFocusPeoplePreferences(preferences)
        return preference.iMessageHandles.count != originalCount
    }

    func oauthTokenFileCandidates() -> [URL] {
        oauthTokenFileCandidates(provider: .webex)
    }

    func oauthTokenFileCandidates(provider: OAuthProviderKind) -> [URL] {
        var candidates: [URL] = []

        let environment = ProcessInfo.processInfo.environment
        let override: String?
        let defaultFilename: String
        switch provider {
        case .webex:
            override = configuration.webexOAuthTokenPathOverride
            defaultFilename = defaultOAuthTokenFilename
        case .outlook:
            override = Self.trimToNil(environment["OUTLOOK_OAUTH_TOKEN_FILE"])
                ?? Self.trimToNil(environment["MS_GRAPH_OAUTH_TOKEN_FILE"])
            defaultFilename = defaultOutlookOAuthTokenFilename
        }

        if let override {
            if override.hasPrefix("/") {
                candidates.append(URL(fileURLWithPath: override))
            } else {
                candidates.append(configuration.runtimeRoot.appendingPathComponent(override))
            }
        }

        candidates.append(configDirectory.appendingPathComponent(defaultFilename))
        candidates.append(configuration.runtimeRoot.appendingPathComponent(defaultFilename))

        var deduped: [URL] = []
        var seen = Set<String>()
        for candidate in candidates {
            let key = candidate.standardizedFileURL.path
            if seen.insert(key).inserted {
                deduped.append(candidate)
            }
        }
        return deduped
    }

    func oauthAppSettings(provider: OAuthProviderKind) throws -> OAuthAppSettings {
        let url = oauthSettingsURL
        guard FileManager.default.fileExists(atPath: url.path) else {
            return OAuthAppSettings()
        }

        let data: Data
        do {
            data = try Data(contentsOf: url)
        } catch {
            throw ConfigStoreError.oauthSettingsPayloadUnreadable(url, error)
        }

        let rawPayload: Any
        do {
            rawPayload = try JSONSerialization.jsonObject(with: data)
        } catch {
            throw ConfigStoreError.oauthSettingsPayloadInvalidJSON(url, error)
        }

        guard let payload = rawPayload as? [String: Any] else {
            throw ConfigStoreError.oauthSettingsPayloadInvalidShape(url)
        }

        let section = Self.oauthSettingsSection(in: payload, provider: provider)
        return OAuthAppSettings(
            sourceFile: url,
            clientID: Self.oauthSettingsString(
                section: section,
                root: payload,
                sectionKeys: ["client_id", "clientID", "clientId"] + provider.oauthClientIDSettingKeys,
                rootKeys: provider.oauthClientIDSettingKeys
            ),
            clientSecret: Self.oauthSettingsString(
                section: section,
                root: payload,
                sectionKeys: ["client_secret", "clientSecret"] + provider.oauthClientSecretSettingKeys,
                rootKeys: provider.oauthClientSecretSettingKeys
            ),
            redirectURI: Self.oauthSettingsString(
                section: section,
                root: payload,
                sectionKeys: ["redirect_uri", "redirectURI", "redirectUri"] + provider.oauthRedirectURISettingKeys,
                rootKeys: provider.oauthRedirectURISettingKeys
            ),
            scope: Self.oauthSettingsString(
                section: section,
                root: payload,
                sectionKeys: ["scope"] + provider.oauthScopeSettingKeys,
                rootKeys: provider.oauthScopeSettingKeys
            ),
            tenant: Self.oauthSettingsString(
                section: section,
                root: payload,
                sectionKeys: ["tenant"] + provider.oauthTenantSettingKeys,
                rootKeys: provider.oauthTenantSettingKeys
            )
        )
    }

    func loadOAuthTokenRecord() throws -> OAuthTokenRecord? {
        try loadOAuthTokenRecord(provider: .webex)
    }

    func loadOAuthTokenRecord(provider: OAuthProviderKind) throws -> OAuthTokenRecord? {
        let fileManager = FileManager.default
        for candidate in oauthTokenFileCandidates(provider: provider) where fileManager.fileExists(atPath: candidate.path) {
            let payloadData: Data
            do {
                payloadData = try Data(contentsOf: candidate)
            } catch {
                throw ConfigStoreError.oauthTokenPayloadUnreadable(candidate, error)
            }

            let rawPayload: Any
            do {
                rawPayload = try JSONSerialization.jsonObject(with: payloadData)
            } catch {
                throw ConfigStoreError.oauthTokenPayloadInvalidJSON(candidate, error)
            }

            guard let payload = rawPayload as? [String: Any] else {
                throw ConfigStoreError.oauthTokenPayloadInvalidShape(candidate)
            }

            let record = OAuthTokenRecord(
                sourceFile: candidate,
                accessToken: stringValue(payload["access_token"]),
                refreshToken: stringValue(payload["refresh_token"]),
                scope: stringValue(payload["scope"]),
                obtainedAt: parseTimestamp(payload["obtained_at"]),
                accessTokenExpiresAt: parseTimestamp(payload["access_token_expires_at"]),
                refreshTokenExpiresAt: parseTimestamp(payload["refresh_token_expires_at"]),
                expiresIn: intValue(payload["expires_in"]),
                refreshTokenExpiresIn: intValue(payload["refresh_token_expires_in"])
            ).withComputedExpiryFallbacks()
            return record
        }
        return nil
    }

    func webexAccessToken() throws -> String {
        if let keychainToken = keychainStore.loadAccessToken(provider: .webex, allowUserInteraction: false),
           !keychainToken.isEmpty {
            return keychainToken
        }
        guard let record = try loadOAuthTokenRecord(provider: .webex) else {
            throw ConfigStoreError.oauthTokenFileMissing(oauthTokenFileCandidates())
        }
        guard record.hasAccessToken else {
            throw ConfigStoreError.oauthAccessTokenMissing(record.sourceFile)
        }
        return record.accessToken
    }

    func oauthTokenHealth(now: Date = Date()) -> OAuthTokenHealth {
        oauthTokenHealth(provider: .webex, now: now)
    }

    func oauthTokenHealth(provider: OAuthProviderKind, now: Date = Date()) -> OAuthTokenHealth {
        do {
            guard let record = try loadOAuthTokenRecord(provider: provider) else {
                return OAuthTokenHealth(state: .missingTokenFile, record: nil, parseError: nil)
            }
            if !record.hasAccessToken {
                return OAuthTokenHealth(state: .missingAccessToken, record: record, parseError: nil)
            }
            if record.isAccessTokenExpired(at: now) {
                if !record.hasRefreshToken {
                    return OAuthTokenHealth(state: .missingRefreshToken, record: record, parseError: nil)
                }
                if record.isRefreshTokenExpired(at: now) {
                    return OAuthTokenHealth(state: .refreshExpired, record: record, parseError: nil)
                }
                return OAuthTokenHealth(state: .expired, record: record, parseError: nil)
            }
            if record.hasRefreshToken,
               record.isRefreshTokenExpiringSoon(
                   skewSeconds: configuration.webexOAuthRefreshTokenSkewSeconds,
                   at: now
               ) {
                return OAuthTokenHealth(state: .refreshExpiringSoon, record: record, parseError: nil)
            }
            if record.isAccessTokenExpiringSoon(
                skewSeconds: configuration.webexOAuthRefreshSkewSeconds,
                at: now
            ) {
                return OAuthTokenHealth(state: .expiringSoon, record: record, parseError: nil)
            }
            if record.accessTokenExpiresAt == nil {
                return OAuthTokenHealth(state: .unknownExpiry, record: record, parseError: nil)
            }
            return OAuthTokenHealth(state: .healthy, record: record, parseError: nil)
        } catch {
            return OAuthTokenHealth(
                state: .invalidTokenFile,
                record: nil,
                parseError: error.localizedDescription
            )
        }
    }

    func tokenFileStatus() -> TokenFileStatus {
        let local = configuration.runtimeRoot.appendingPathComponent(defaultOAuthTokenFilename)
        let config = configDirectory.appendingPathComponent(defaultOAuthTokenFilename)
        let health = oauthTokenHealth()
        return TokenFileStatus(
            rootTokenFileExists: FileManager.default.fileExists(atPath: local.path),
            configTokenFileExists: FileManager.default.fileExists(atPath: config.path),
            resolvedTokenFilePath: health.record?.sourceFile.path,
            healthState: health.state.rawValue,
            accessTokenExpiresAt: health.record?.accessTokenExpiresAt,
            accessTokenExpired: health.record?.isAccessTokenExpired(),
            accessTokenExpiringSoon: health.record?.isAccessTokenExpiringSoon(
                skewSeconds: configuration.webexOAuthRefreshSkewSeconds
            ),
            refreshTokenExpiresAt: health.record?.refreshTokenExpiresAt,
            refreshTokenExpired: health.record?.isRefreshTokenExpired(),
            refreshTokenExpiringSoon: health.record?.isRefreshTokenExpiringSoon(
                skewSeconds: configuration.webexOAuthRefreshTokenSkewSeconds
            ),
            refreshTokenPresent: health.record?.hasRefreshToken,
            parseError: health.parseError
        )
    }

    func oauthProviderStatus(provider: OAuthProviderKind) -> OAuthProviderStatus {
        let candidates = oauthTokenFileCandidates(provider: provider)
        let health = oauthTokenHealth(provider: provider)
        let existingFiles = candidates.filter { FileManager.default.fileExists(atPath: $0.path) }
        return OAuthProviderStatus(
            provider: provider,
            tokenFileExists: !existingFiles.isEmpty,
            resolvedTokenFilePath: health.record?.sourceFile.path ?? existingFiles.first?.path,
            healthState: health.state.rawValue,
            accessTokenExpiresAt: health.record?.accessTokenExpiresAt,
            refreshTokenExpiresAt: health.record?.refreshTokenExpiresAt,
            refreshTokenPresent: health.record?.hasRefreshToken,
            scope: health.record?.scope,
            parseError: health.parseError
        )
    }

    func saveOAuthTokenPayload(_ payload: [String: Any], provider: OAuthProviderKind) throws -> URL {
        let url = configDirectory.appendingPathComponent(
            provider == .webex ? defaultOAuthTokenFilename : defaultOutlookOAuthTokenFilename
        )
        try FileManager.default.createDirectory(at: url.deletingLastPathComponent(), withIntermediateDirectories: true)
        let data = try JSONSerialization.data(withJSONObject: payload, options: [.prettyPrinted, .sortedKeys])
        try data.write(to: url, options: [.atomic])
        let accessToken = stringValue(payload["access_token"])
        if !accessToken.trimmingCharacters(in: .whitespacesAndNewlines).isEmpty {
            try? keychainStore.saveAccessToken(accessToken, provider: provider)
        }
        return url
    }

    @discardableResult
    func deleteOAuthTokenFiles(provider: OAuthProviderKind) throws -> [URL] {
        let candidates = oauthTokenFileCandidates(provider: provider)
        var removed: [URL] = []
        for candidate in candidates where FileManager.default.fileExists(atPath: candidate.path) {
            try FileManager.default.removeItem(at: candidate)
            removed.append(candidate)
        }
        try? keychainStore.deleteAccessToken(provider: provider)
        return removed
    }

    func loadSystemSettings() -> SystemSettings {
        var settings = SystemSettings()
        settings.updatedAt = nil

        guard FileManager.default.fileExists(atPath: systemSettingsURL.path),
              let data = try? Data(contentsOf: systemSettingsURL),
              let rawPayload = try? JSONSerialization.jsonObject(with: data) as? [String: Any] else {
            return settings
        }

        let settingsPayload = rawPayload["settings"] as? [String: Any] ?? rawPayload
        settings.debug = optionalBoolValue(settingsPayload[SystemSettingKey.debug.rawValue]) ?? settings.debug
        settings.backgroundStatus = optionalBoolValue(settingsPayload[SystemSettingKey.backgroundStatus.rawValue]) ?? settings.backgroundStatus
        settings.webexSyncEnabled = optionalBoolValue(settingsPayload[SystemSettingKey.webexSyncEnabled.rawValue]) ?? settings.webexSyncEnabled
        settings.autoQueryAllEnabled = optionalBoolValue(settingsPayload[SystemSettingKey.autoQueryAllEnabled.rawValue]) ?? settings.autoQueryAllEnabled
        settings.priorityRefreshPausesBackground = optionalBoolValue(settingsPayload[SystemSettingKey.priorityRefreshPausesBackground.rawValue]) ?? settings.priorityRefreshPausesBackground
        settings.codexEnabled = optionalBoolValue(settingsPayload[SystemSettingKey.codexEnabled.rawValue]) ?? settings.codexEnabled
        settings.codexAskEnabled = optionalBoolValue(settingsPayload[SystemSettingKey.codexAskEnabled.rawValue]) ?? settings.codexAskEnabled
        settings.codexQuestionSynthesisEnabled = optionalBoolValue(settingsPayload[SystemSettingKey.codexQuestionSynthesisEnabled.rawValue]) ?? settings.codexQuestionSynthesisEnabled
        settings.codexPersonSummariesEnabled = optionalBoolValue(settingsPayload[SystemSettingKey.codexPersonSummariesEnabled.rawValue]) ?? settings.codexPersonSummariesEnabled
        settings.codexSpaceSummariesEnabled = optionalBoolValue(settingsPayload[SystemSettingKey.codexSpaceSummariesEnabled.rawValue]) ?? settings.codexSpaceSummariesEnabled
        settings.codexClusterTitlesEnabled = optionalBoolValue(settingsPayload[SystemSettingKey.codexClusterTitlesEnabled.rawValue]) ?? settings.codexClusterTitlesEnabled
        settings.codexExecQuestionsEnabled = optionalBoolValue(settingsPayload[SystemSettingKey.codexExecQuestionsEnabled.rawValue]) ?? settings.codexExecQuestionsEnabled
        settings.codexBeliefsEnabled = optionalBoolValue(settingsPayload[SystemSettingKey.codexBeliefsEnabled.rawValue]) ?? settings.codexBeliefsEnabled
        settings.codexModel = CodexModelSelection.normalized(stringValue(settingsPayload[SystemSettingKey.codexModel.rawValue]))
        settings.codexReasoningLevel = CodexReasoningLevel.normalized(stringValue(settingsPayload[SystemSettingKey.codexReasoningLevel.rawValue]))
        settings.webexSyncMinutes = positiveIntValue(settingsPayload[SystemSettingKey.webexSyncMinutes.rawValue]) ?? settings.webexSyncMinutes
        settings.autoQueryAllMinutes = positiveIntValue(settingsPayload[SystemSettingKey.autoQueryAllMinutes.rawValue]) ?? settings.autoQueryAllMinutes
        settings.trackedActionsRefreshMinutes = positiveIntValue(settingsPayload[SystemSettingKey.trackedActionsRefreshMinutes.rawValue]) ?? settings.trackedActionsRefreshMinutes
        settings.personFocusRefreshMinutes = positiveIntValue(settingsPayload[SystemSettingKey.personFocusRefreshMinutes.rawValue]) ?? settings.personFocusRefreshMinutes
        settings.personFocusDays = clampedIntValue(
            settingsPayload[SystemSettingKey.personFocusDays.rawValue],
            defaultValue: settings.personFocusDays,
            bounds: SystemSettings.focusDaysBounds
        )
        settings.personFocusAnalysisCadenceHours = clampedIntValue(
            settingsPayload[SystemSettingKey.personFocusAnalysisCadenceHours.rawValue],
            defaultValue: settings.personFocusAnalysisCadenceHours,
            bounds: SystemSettings.focusAnalysisCadenceHoursBounds
        )
        settings.spaceFocusRefreshMinutes = positiveIntValue(settingsPayload[SystemSettingKey.spaceFocusRefreshMinutes.rawValue]) ?? settings.spaceFocusRefreshMinutes
        settings.spaceFocusDays = clampedIntValue(
            settingsPayload[SystemSettingKey.spaceFocusDays.rawValue],
            defaultValue: settings.spaceFocusDays,
            bounds: SystemSettings.focusDaysBounds
        )
        settings.spaceFocusAnalysisCadenceHours = clampedIntValue(
            settingsPayload[SystemSettingKey.spaceFocusAnalysisCadenceHours.rawValue],
            defaultValue: settings.spaceFocusAnalysisCadenceHours,
            bounds: SystemSettings.focusAnalysisCadenceHoursBounds
        )
        settings.transcriptionEnabled = optionalBoolValue(settingsPayload[SystemSettingKey.transcriptionEnabled.rawValue]) ?? settings.transcriptionEnabled
        settings.transcriptionDiarizationEnabled = optionalBoolValue(settingsPayload[SystemSettingKey.transcriptionDiarizationEnabled.rawValue]) ?? settings.transcriptionDiarizationEnabled
        settings.transcriptionLanguageMode = TranscriptionLanguageMode.normalized(
            stringValue(settingsPayload[SystemSettingKey.transcriptionLanguageMode.rawValue])
        )
        settings.transcriptionMicrophoneGain = clampedIntValue(
            settingsPayload[SystemSettingKey.transcriptionMicrophoneGain.rawValue],
            defaultValue: settings.transcriptionMicrophoneGain,
            bounds: SystemSettings.transcriptionMicrophoneGainBounds
        )
        settings.transcriptionAWSEndpoint = stringValue(settingsPayload[SystemSettingKey.transcriptionAWSEndpoint.rawValue])
        if let pollSeconds = positiveIntValue(settingsPayload[SystemSettingKey.pollSeconds.rawValue]) {
            if settingsPayload[SystemSettingKey.webexSyncMinutes.rawValue] == nil {
                settings.pollSeconds = pollSeconds
                settings.webexSyncMinutes = max(1, Int(ceil(Double(pollSeconds) / 60.0)))
            } else {
                settings.pollSeconds = settings.webexSyncMinutes * 60
            }
        } else {
            settings.pollSeconds = settings.webexSyncMinutes * 60
        }
        settings.updatedAt = parseTimestamp(rawPayload["updated_at"])
        return settings
    }

    func saveSystemSettings(_ settings: SystemSettings) throws {
        try FileManager.default.createDirectory(at: configDirectory, withIntermediateDirectories: true)
        let payload = PersistedSystemSettingsPayload(
            version: SystemSettings.persistedVersion,
            updatedAt: Self.iso8601String(from: Date()),
            settings: PersistedSystemSettingsValues(settings: settings)
        )
        let encoder = JSONEncoder()
        encoder.outputFormatting = [.prettyPrinted, .sortedKeys]
        let data = try encoder.encode(payload)
        try data.write(to: systemSettingsURL, options: [.atomic])
    }

    func loadTranscriptionAuthToken() -> String? {
        keychainStore.loadSecret(
            account: Self.transcriptionAuthTokenKeychainAccount,
            allowUserInteraction: true
        )
    }

    func saveTranscriptionAuthToken(_ token: String) throws {
        try keychainStore.saveSecret(
            token,
            account: Self.transcriptionAuthTokenKeychainAccount,
            description: "transcription service token"
        )
    }

    func deleteTranscriptionAuthToken() throws {
        try keychainStore.deleteSecret(
            account: Self.transcriptionAuthTokenKeychainAccount,
            description: "transcription service token"
        )
    }

    func transcriptionAuthTokenConfigured() -> Bool {
        keychainStore.secretExists(account: Self.transcriptionAuthTokenKeychainAccount)
    }

    func loadAskCodexQueryHistory(limit: Int = 100) -> [AskCodexQueryHistoryEntry] {
        guard FileManager.default.fileExists(atPath: askCodexQueryHistoryURL.path),
              let data = try? Data(contentsOf: askCodexQueryHistoryURL) else {
            return []
        }
        let decoder = JSONDecoder()
        let entries: [AskCodexQueryHistoryEntry]
        if let payload = try? decoder.decode(PersistedAskCodexQueryHistoryPayload.self, from: data) {
            entries = payload.queries
        } else if let legacyEntries = try? decoder.decode([AskCodexQueryHistoryEntry].self, from: data) {
            entries = legacyEntries
        } else {
            return []
        }
        return Array(entries.prefix(max(0, limit)))
    }

    func saveAskCodexQueryHistory(_ entries: [AskCodexQueryHistoryEntry], limit: Int = 100) throws {
        try FileManager.default.createDirectory(at: configDirectory, withIntermediateDirectories: true)
        let payload = PersistedAskCodexQueryHistoryPayload(
            version: 1,
            updatedAt: Self.iso8601String(from: Date()),
            queries: Array(entries.prefix(max(0, limit)))
        )
        let encoder = JSONEncoder()
        encoder.outputFormatting = [.prettyPrinted, .sortedKeys]
        let data = try encoder.encode(payload)
        try data.write(to: askCodexQueryHistoryURL, options: [.atomic])
    }

    func loadRefreshCheckpointData() -> Data? {
        guard FileManager.default.fileExists(atPath: refreshCheckpointURL.path) else {
            return nil
        }
        return try? Data(contentsOf: refreshCheckpointURL)
    }

    func saveRefreshCheckpointData(_ data: Data) throws {
        try FileManager.default.createDirectory(at: configDirectory, withIntermediateDirectories: true)
        try data.write(to: refreshCheckpointURL, options: [.atomic])
    }

    func clearRefreshCheckpoint() throws {
        guard FileManager.default.fileExists(atPath: refreshCheckpointURL.path) else {
            return
        }
        try FileManager.default.removeItem(at: refreshCheckpointURL)
    }

    private func loadTargets(filename: String) throws -> [ConfigTarget] {
        let url = configDirectory.appendingPathComponent(filename)
        guard FileManager.default.fileExists(atPath: url.path) else { return [] }
        let text = try String(contentsOf: url, encoding: .utf8)
        let lines = text.components(separatedBy: .newlines)
        let parsedTargets = lines.enumerated().compactMap { index, line in
            parseTargetLine(
                line,
                sourceFile: filename,
                lineNumber: index + 1
            )
        }
        return deduplicateTargets(parsedTargets)
    }

    private func loadManagedTargets(filename: String) throws -> [ConfigTarget] {
        let url = configDirectory.appendingPathComponent(filename)
        guard FileManager.default.fileExists(atPath: url.path) else { return [] }
        let text = try String(contentsOf: url, encoding: .utf8)
        let lines = text.components(separatedBy: .newlines)
        let parsedTargets = lines.enumerated().compactMap { index, line in
            parseManagedTargetLine(
                line,
                sourceFile: filename,
                lineNumber: index + 1
            )
        }
        return representativeTargets(parsedTargets)
    }

    private func parseTargetLine(
        _ rawLine: String,
        sourceFile: String,
        lineNumber: Int
    ) -> ConfigTarget? {
        let line = rawLine.trimmingCharacters(in: .whitespacesAndNewlines)
        guard !line.isEmpty, !line.hasPrefix("#") else { return nil }

        guard let row = parseTargetRow(line) else { return nil }

        let label = normalizeLabel(row.label)
        let roomID = normalizeRoomID(row.roomID)
        let email = normalizeEmail(row.email)

        switch row.kind {
        case .person:
            guard isValidEmail(email), isLikelyWebexRoomID(roomID), !label.isEmpty else { return nil }
        case .space:
            guard isLikelyWebexRoomID(roomID), !label.isEmpty else { return nil }
        case .unknown:
            return nil
        }

        return ConfigTarget(
            kind: row.kind,
            label: label,
            roomID: roomID,
            roomType: normalizeRoomType(row.roomType),
            email: email,
            sourceMetadata: ConfigTargetSourceMetadata(
                fileName: sourceFile,
                lineNumber: lineNumber,
                entryKey: row.kind.rawValue,
                parseMode: row.parseMode
            )
        )
    }

    private func parseManagedTargetLine(
        _ rawLine: String,
        sourceFile: String,
        lineNumber: Int
    ) -> ConfigTarget? {
        let line = rawLine.trimmingCharacters(in: .whitespacesAndNewlines)
        guard !line.isEmpty, !line.hasPrefix("#") else { return nil }
        guard let row = parseTargetRow(line) else { return nil }

        let label = normalizeLabel(row.label)
        let roomID = normalizeRoomID(row.roomID)
        let email = normalizeEmail(row.email)

        guard !label.isEmpty, !roomID.isEmpty else { return nil }
        if row.kind == .person, !email.isEmpty, !isValidEmail(email) {
            return nil
        }

        return ConfigTarget(
            kind: row.kind,
            label: label,
            roomID: roomID,
            roomType: normalizeRoomType(row.roomType),
            email: email,
            sourceMetadata: ConfigTargetSourceMetadata(
                fileName: sourceFile,
                lineNumber: lineNumber,
                entryKey: row.kind.rawValue,
                parseMode: row.parseMode
            )
        )
    }

    private struct FocusPreference {
        var label: String
        var autoReply: Bool
        var iMessageHandles: [String] = []
    }

    private func loadJSONFocusPreferences(filename: String, mapKey: String) throws -> [String: FocusPreference] {
        let url = configDirectory.appendingPathComponent(filename)
        guard FileManager.default.fileExists(atPath: url.path) else { return [:] }
        let data = try Data(contentsOf: url)
        let raw = try JSONSerialization.jsonObject(with: data)
        guard let root = raw as? [String: Any] else { return [:] }
        let mapped = (root[mapKey] as? [String: Any]) ?? root

        var preferences: [String: FocusPreference] = [:]
        for (rawKey, rawValue) in mapped {
            let key = mapKey == "spaces" ? normalizeRoomID(rawKey) : normalizeEmail(rawKey)
            guard !key.isEmpty else { continue }

            if let value = rawValue as? [String: Any] {
                preferences[key] = FocusPreference(
                    label: normalizeLabel(stringValue(value["label"] ?? value["title"] ?? value["name"])),
                    autoReply: boolValue(value["auto_reply"] ?? value["autoReply"]),
                    iMessageHandles: iMessageHandles(from: value)
                )
            } else {
                preferences[key] = FocusPreference(
                    label: "",
                    autoReply: boolValue(rawValue)
                )
            }
        }
        return preferences
    }

    private func savePersonFocusPeoplePreferences(_ preferences: [String: FocusPreference]) throws {
        try FileManager.default.createDirectory(at: configDirectory, withIntermediateDirectories: true)
        let peoplePayload = Dictionary(
            uniqueKeysWithValues: preferences
                .map { (normalizeEmail($0.key), $0.value) }
                .filter { !$0.0.isEmpty }
                .sorted { $0.0 < $1.0 }
                .map { key, preference in
                    (
                        key,
                        personFocusPreferencePayload(
                            autoReply: preference.autoReply,
                            iMessageHandles: preference.iMessageHandles
                        )
                    )
                }
        )
        let payload: [String: Any] = [
            "version": 1,
            "updated_at": Self.iso8601String(from: Date()),
            "people": peoplePayload
        ]
        let data = try JSONSerialization.data(withJSONObject: payload, options: [.prettyPrinted, .sortedKeys])
        try data.write(to: personFocusPreferencesURL, options: [.atomic])
    }

    private func applyFocusPreference(
        to target: ConfigTarget,
        preferences: [String: FocusPreference],
        key: String
    ) -> ConfigTarget {
        guard let preference = preferences[key] else {
            return target
        }

        var updated = target
        if !preference.label.isEmpty {
            updated.label = preference.label
        }
        updated.autoReply = preference.autoReply
        updated.iMessageHandles = preference.iMessageHandles
        return updated
    }

    private func loadJSONFocusTargets(filename: String, kind: ConfigTarget.Kind) throws -> [ConfigTarget] {
        let url = configDirectory.appendingPathComponent(filename)
        guard FileManager.default.fileExists(atPath: url.path) else { return [] }
        let data = try Data(contentsOf: url)
        let raw = try JSONSerialization.jsonObject(with: data)

        if let root = raw as? [String: Any] {
            let mapKey = kind == .space ? "spaces" : "people"
            if let mapped = root[mapKey] as? [String: Any] {
                let parsedTargets = mapped.keys.sorted().enumerated().compactMap { index, key -> ConfigTarget? in
                    let value = mapped[key] as? [String: Any] ?? [:]
                    if kind == .space {
                        let roomID = normalizeRoomID(key)
                        guard isLikelyWebexRoomID(roomID) else { return nil }
                        let label = normalizeLabel(stringValue(value["label"] ?? value["title"] ?? value["name"] ?? key))
                        guard !label.isEmpty else { return nil }
                        let email = normalizeEmail(stringValue(value["person_email"] ?? value["email"]))
                        return ConfigTarget(
                            kind: .space,
                            label: label,
                            roomID: roomID,
                            email: email,
                            autoReply: boolValue(value["auto_reply"] ?? value["autoReply"]),
                            sourceMetadata: ConfigTargetSourceMetadata(
                                fileName: filename,
                                lineNumber: nil,
                                entryKey: "\(mapKey).\(key)",
                                parseMode: "json-map[\(index)]"
                            )
                        )
                    }

                    let email = normalizeEmail(key)
                    guard isValidEmail(email) else { return nil }
                    let label = normalizeLabel(stringValue(value["label"] ?? value["title"] ?? value["name"] ?? email))
                    guard !label.isEmpty else { return nil }
                    let roomID = normalizeRoomID(stringValue(value["room_id"] ?? value["roomID"] ?? value["id"]))
                    return ConfigTarget(
                        kind: .person,
                        label: label,
                        roomID: roomID,
                        email: email,
                        autoReply: boolValue(value["auto_reply"] ?? value["autoReply"]),
                        sourceMetadata: ConfigTargetSourceMetadata(
                            fileName: filename,
                            lineNumber: nil,
                            entryKey: "\(mapKey).\(key)",
                            parseMode: "json-map[\(index)]"
                        )
                    )
                }
                return deduplicateTargets(parsedTargets)
            }
        }

        guard let values = raw as? [[String: Any]] else { return [] }
        let parsedTargets = values.enumerated().compactMap { index, value -> ConfigTarget? in
            let label = normalizeLabel(stringValue(value["label"] ?? value["title"] ?? value["name"]))
            let roomID = normalizeRoomID(stringValue(value["room_id"] ?? value["roomID"] ?? value["id"]))
            let email = normalizeEmail(stringValue(value["person_email"] ?? value["email"]))

            switch kind {
            case .person:
                guard isValidEmail(email), !label.isEmpty else { return nil }
            case .space:
                guard isLikelyWebexRoomID(roomID), !label.isEmpty else { return nil }
            case .unknown:
                return nil
            }

            return ConfigTarget(
                kind: kind,
                label: label,
                roomID: roomID,
                email: email,
                autoReply: boolValue(value["auto_reply"] ?? value["autoReply"]),
                iMessageHandles: iMessageHandles(from: value),
                sourceMetadata: ConfigTargetSourceMetadata(
                    fileName: filename,
                    lineNumber: nil,
                    entryKey: "array[\(index)]",
                    parseMode: "json-array"
                )
            )
        }
        return deduplicateTargets(parsedTargets)
    }

    private func stringValue(_ value: Any?) -> String {
        guard let value else { return "" }
        return String(describing: value).trimmingCharacters(in: .whitespacesAndNewlines)
    }

    private func iMessageHandles(from payload: [String: Any]) -> [String] {
        let rawValue = payload["imessage_handles"]
            ?? payload["iMessageHandles"]
            ?? payload["phone_numbers"]
            ?? payload["phones"]
            ?? payload["handles"]

        let rawHandles: [String]
        switch rawValue {
        case let values as [String]:
            rawHandles = values
        case let values as [Any]:
            rawHandles = values.map { String(describing: $0) }
        case let value as String:
            rawHandles = value
                .split(whereSeparator: { $0 == "," || $0 == ";" || $0 == "\n" })
                .map(String.init)
        default:
            rawHandles = []
        }
        return dedupedIMessageHandles(rawHandles)
    }

    private func dedupedIMessageHandles(_ handles: [String]) -> [String] {
        var result: [String] = []
        var seen = Set<String>()
        for rawHandle in handles {
            guard let handle = IMessageHandleNormalizer.normalizedStorageValue(rawHandle) else {
                continue
            }
            let key = handle.lowercased()
            if seen.insert(key).inserted {
                result.append(handle)
            }
        }
        return result
    }

    private func personFocusPreferencePayload(
        autoReply: Bool,
        iMessageHandles: [String]
    ) -> [String: Any] {
        var payload: [String: Any] = [
            "auto_reply": autoReply
        ]
        let handles = dedupedIMessageHandles(iMessageHandles)
        if !handles.isEmpty {
            payload["imessage_handles"] = handles
        }
        return payload
    }

    private static func oauthSettingsSection(
        in payload: [String: Any],
        provider: OAuthProviderKind
    ) -> [String: Any]? {
        for name in provider.oauthSettingsSectionNames {
            if let value = payload[name] as? [String: Any] {
                return value
            }
        }
        return nil
    }

    private static func oauthSettingsString(
        section: [String: Any]?,
        root: [String: Any],
        sectionKeys: [String],
        rootKeys: [String]
    ) -> String? {
        if let sectionValue = firstString(in: section, keys: sectionKeys) {
            return sectionValue
        }
        return firstString(in: root, keys: rootKeys)
    }

    private static func firstString(in payload: [String: Any]?, keys: [String]) -> String? {
        guard let payload else { return nil }
        for key in expandedOAuthKeys(keys) {
            guard let rawValue = payload[key] else { continue }
            if let value = trimToNil(String(describing: rawValue)) {
                return value
            }
        }
        return nil
    }

    private static func expandedOAuthKeys(_ keys: [String]) -> [String] {
        var expanded: [String] = []
        var seen = Set<String>()
        for key in keys {
            for candidate in [key, key.lowercased()] where seen.insert(candidate).inserted {
                expanded.append(candidate)
            }
        }
        return expanded
    }

    private static func trimToNil(_ value: String?) -> String? {
        guard let trimmed = value?.trimmingCharacters(in: .whitespacesAndNewlines),
              !trimmed.isEmpty else {
            return nil
        }
        return trimmed
    }

    private func intValue(_ value: Any?) -> Int? {
        switch value {
        case let intValue as Int:
            return intValue
        case let number as NSNumber:
            return number.intValue
        case let text as String:
            return Int(text.trimmingCharacters(in: .whitespacesAndNewlines))
        default:
            return nil
        }
    }

    private func boolValue(_ value: Any?) -> Bool {
        switch value {
        case let boolValue as Bool:
            return boolValue
        case let number as NSNumber:
            return number.boolValue
        case let text as String:
            switch text.trimmingCharacters(in: .whitespacesAndNewlines).lowercased() {
            case "1", "true", "yes", "on", "enabled":
                return true
            default:
                return false
            }
        default:
            return false
        }
    }

    private func optionalBoolValue(_ value: Any?) -> Bool? {
        switch value {
        case let boolValue as Bool:
            return boolValue
        case let number as NSNumber:
            return number.boolValue
        case let text as String:
            switch text.trimmingCharacters(in: .whitespacesAndNewlines).lowercased() {
            case "1", "true", "yes", "on", "enabled":
                return true
            case "0", "false", "no", "off", "disabled":
                return false
            default:
                return nil
            }
        default:
            return nil
        }
    }

    private func positiveIntValue(_ value: Any?) -> Int? {
        guard let parsed = intValue(value) else {
            return nil
        }
        return max(1, parsed)
    }

    private func clampedIntValue(
        _ value: Any?,
        defaultValue: Int,
        bounds: ClosedRange<Int>
    ) -> Int {
        guard let parsed = intValue(value) else {
            return SystemSettings.clamped(defaultValue, to: bounds)
        }
        return SystemSettings.clamped(parsed, to: bounds)
    }

    private func parseTimestamp(_ value: Any?) -> Date? {
        guard let raw = value.map({ String(describing: $0).trimmingCharacters(in: .whitespacesAndNewlines) }),
              !raw.isEmpty else {
            return nil
        }
        if let parsed = Self.iso8601WithFractionalSeconds.date(from: raw) {
            return parsed
        }
        return Self.iso8601.date(from: raw)
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

    private static func iso8601String(from date: Date) -> String {
        iso8601WithFractionalSeconds.string(from: date)
    }

    private struct ParsedTargetRow {
        var kind: ConfigTarget.Kind
        var label: String
        var roomID: String
        var roomType: String
        var email: String
        var parseMode: String
    }

    private func parseTargetRow(_ line: String) -> ParsedTargetRow? {
        let columns: [String]
        let parseMode: String

        if line.contains("\t") {
            columns = splitAndTrim(line, separator: "\t")
            parseMode = "tab"
        } else if line.contains("|") {
            columns = splitAndTrim(line, separator: "|")
            parseMode = "pipe"
        } else if let row = parseWhitespaceFallbackRow(line) {
            return row
        } else {
            return nil
        }

        guard let kind = parseKind(columns.first) else { return nil }

        // Canonical target row schema:
        // kind | label | room_id | room_type | person_email | person_display_name
        let rawLabel = columns[safe: 1] ?? columns[safe: 0] ?? ""
        let personDisplayName = columns[safe: 5]?.trimmingCharacters(in: .whitespacesAndNewlines) ?? ""
        let label = kind == .person && !personDisplayName.isEmpty ? personDisplayName : rawLabel
        let roomID = columns[safe: 2] ?? ""
        let roomType = columns[safe: 3] ?? ""
        let email = columns[safe: 4] ?? columns.first(where: { $0.contains("@") }) ?? ""

        return ParsedTargetRow(
            kind: kind,
            label: label,
            roomID: roomID,
            roomType: roomType,
            email: email,
            parseMode: parseMode
        )
    }

    private func parseWhitespaceFallbackRow(_ line: String) -> ParsedTargetRow? {
        let tokens = line.split(whereSeparator: \.isWhitespace).map(String.init)
        guard let first = tokens.first,
              let kind = parseKind(first) else {
            return nil
        }

        guard let roomTokenIndex = tokens.firstIndex(where: isLikelyWebexRoomID) else {
            return nil
        }
        guard roomTokenIndex > 1 else { return nil }

        let labelTokens = tokens[1..<roomTokenIndex]
        let label = labelTokens.joined(separator: " ")
        let roomID = tokens[roomTokenIndex]
        let trailing = Array(tokens.suffix(from: roomTokenIndex + 1))
        let email = trailing.first(where: { $0.contains("@") }) ?? ""

        return ParsedTargetRow(
            kind: kind,
            label: label,
            roomID: roomID,
            roomType: "",
            email: email,
            parseMode: "whitespace"
        )
    }

    private func parseKind(_ rawKind: String?) -> ConfigTarget.Kind? {
        guard let kindToken = rawKind?.trimmingCharacters(in: .whitespacesAndNewlines).lowercased(),
              !kindToken.isEmpty else {
            return nil
        }
        switch kindToken {
        case "sender", "person":
            return .person
        case "space", "room":
            return .space
        default:
            return nil
        }
    }

    private func splitAndTrim(_ line: String, separator: Character) -> [String] {
        line.split(separator: separator, omittingEmptySubsequences: false)
            .map { String($0).trimmingCharacters(in: .whitespacesAndNewlines) }
    }

    private func normalizeLabel(_ value: String) -> String {
        value
            .trimmingCharacters(in: .whitespacesAndNewlines)
            .replacingOccurrences(of: #"\s+"#, with: " ", options: .regularExpression)
    }

    private func normalizeRoomID(_ value: String) -> String {
        value
            .trimmingCharacters(in: .whitespacesAndNewlines)
            .replacingOccurrences(of: #"\s+"#, with: "", options: .regularExpression)
    }

    private func normalizeRoomType(_ value: String) -> String {
        value.trimmingCharacters(in: .whitespacesAndNewlines).lowercased()
    }

    private func normalizeEmail(_ value: String) -> String {
        value.trimmingCharacters(in: .whitespacesAndNewlines).lowercased()
    }

    private func isValidEmail(_ email: String) -> Bool {
        let value = normalizeEmail(email)
        guard !value.isEmpty else { return false }
        guard !value.contains(where: \.isWhitespace) else { return false }
        let parts = value.split(separator: "@")
        guard parts.count == 2 else { return false }
        return !parts[0].isEmpty && !parts[1].isEmpty
    }

    private func isLikelyWebexRoomID(_ value: String) -> Bool {
        let normalized = normalizeRoomID(value)
        guard !normalized.isEmpty else { return false }
        guard !normalized.contains(where: \.isWhitespace) else { return false }
        return normalized.hasPrefix("Y2lzY29zcGFyazovL3VzL1JPT00v")
    }

    private func deduplicateTargets(_ targets: [ConfigTarget]) -> [ConfigTarget] {
        var seen = Set<String>()
        var deduped: [ConfigTarget] = []
        deduped.reserveCapacity(targets.count)

        for target in targets {
            if seen.insert(target.id).inserted {
                deduped.append(target)
            }
        }
        return deduped
    }

    private func representativeTargets(_ targets: [ConfigTarget]) -> [ConfigTarget] {
        var byKey: [String: ConfigTarget] = [:]
        for target in targets {
            let key = fileIdentityKey(target)
            guard !key.isEmpty else { continue }
            if let current = byKey[key] {
                if targetPriority(target) > targetPriority(current) {
                    byKey[key] = target
                }
            } else {
                byKey[key] = target
            }
        }
        return byKey.values.sorted(by: targetDisplaySort)
    }

    private func mapPersonCandidates(excluding existingKeys: Set<String>) throws -> [ConfigTarget] {
        let candidates = try loadManagedTargets(filename: "map.txt")
            .filter { $0.kind == .person }
            .filter { isValidEmail($0.email) }
            .filter { !existingKeys.contains(fileIdentityKey($0)) }
        return candidates.sorted(by: targetDisplaySort)
    }

    private func mapSpaceCandidates(excluding existingKeys: Set<String>) throws -> [ConfigTarget] {
        let candidates = try loadManagedTargets(filename: "map.txt")
            .filter { $0.kind == .space }
            .filter { isLikelyWebexRoomID($0.roomID) }
            .filter { !existingKeys.contains(fileIdentityKey($0)) }
        return candidates.sorted(by: targetDisplaySort)
    }

    private func appendTargetByRoomIfMissing(_ target: ConfigTarget, to url: URL) throws -> Bool {
        guard target.kind == .person || target.kind == .space else {
            throw ConfigStoreError.focusTargetUnsupportedKind(target.label)
        }
        guard !normalizeRoomID(target.roomID).isEmpty else {
            throw ConfigStoreError.focusTargetMissingRoomID(target.label)
        }

        try FileManager.default.createDirectory(at: url.deletingLastPathComponent(), withIntermediateDirectories: true)
        if !FileManager.default.fileExists(atPath: url.path) {
            try "# kind\tlabel\troom_id\troom_type\tperson_email\tperson_display_name\n".write(to: url, atomically: true, encoding: .utf8)
        }

        let originalText = try String(contentsOf: url, encoding: .utf8)
        let existingTargets = originalText.components(separatedBy: .newlines).compactMap {
            parseManagedTargetLine($0, sourceFile: url.lastPathComponent, lineNumber: 0)
        }
        if existingTargets.contains(where: { targetMatchesFileIdentity($0, target) }) {
            return false
        }

        var rewritten = originalText
        if !rewritten.isEmpty, !rewritten.hasSuffix("\n") {
            rewritten += "\n"
        }
        rewritten += mapStyleLine(for: target) + "\n"
        try rewritten.write(to: url, atomically: true, encoding: .utf8)
        return true
    }

    private func removeEntriesByRoom(_ target: ConfigTarget, from url: URL) throws -> Int {
        guard FileManager.default.fileExists(atPath: url.path) else {
            return 0
        }
        let originalLines = try String(contentsOf: url, encoding: .utf8).components(separatedBy: .newlines)
        var keptLines: [String] = []
        var removedCount = 0
        for line in originalLines {
            if let parsed = parseManagedTargetLine(line, sourceFile: url.lastPathComponent, lineNumber: 0),
               targetMatchesFileIdentity(parsed, target) {
                removedCount += 1
                continue
            }
            keptLines.append(line)
        }
        guard removedCount > 0 else {
            return 0
        }
        var rewritten = keptLines.joined(separator: "\n").trimmingCharacters(in: .newlines)
        if !rewritten.isEmpty {
            rewritten += "\n"
        }
        try rewritten.write(to: url, atomically: true, encoding: .utf8)
        return removedCount
    }

    private func targetMatchesFileIdentity(_ lhs: ConfigTarget, _ rhs: ConfigTarget) -> Bool {
        guard lhs.kind == rhs.kind else { return false }
        let leftRoomID = normalizeRoomID(lhs.roomID)
        let rightRoomID = normalizeRoomID(rhs.roomID)
        if !leftRoomID.isEmpty, !rightRoomID.isEmpty {
            return leftRoomID == rightRoomID
        }

        let leftEmail = normalizeEmail(lhs.email)
        let rightEmail = normalizeEmail(rhs.email)
        if lhs.kind == .person, !leftEmail.isEmpty, !rightEmail.isEmpty {
            return leftEmail == rightEmail
        }

        return normalizeLabel(lhs.label).casefoldKey == normalizeLabel(rhs.label).casefoldKey
    }

    private func fileIdentityKey(_ target: ConfigTarget) -> String {
        let roomID = normalizeRoomID(target.roomID)
        if !roomID.isEmpty {
            return "\(target.kind.rawValue):room:\(roomID)"
        }
        let email = normalizeEmail(target.email)
        if target.kind == .person, !email.isEmpty {
            return "\(target.kind.rawValue):email:\(email)"
        }
        return "\(target.kind.rawValue):label:\(normalizeLabel(target.label).casefoldKey)"
    }

    private func targetDisplaySort(_ lhs: ConfigTarget, _ rhs: ConfigTarget) -> Bool {
        let leftLabel = lhs.label.casefoldKey
        let rightLabel = rhs.label.casefoldKey
        if leftLabel != rightLabel {
            return leftLabel < rightLabel
        }
        return lhs.roomID < rhs.roomID
    }

    private func targetPriority(_ target: ConfigTarget) -> (Int, Int, Int) {
        (
            target.label.casefoldKey == normalizeEmail(target.email) ? 0 : 1,
            normalizeEmail(target.email).isEmpty ? 0 : 1,
            target.label.count
        )
    }

    private func mapStyleLine(for target: ConfigTarget) -> String {
        let roomType = normalizeRoomType(target.roomType).isEmpty ? "direct" : normalizeRoomType(target.roomType)
        let email = normalizeEmail(target.email)
        let displayName = target.label
        return [
            target.kind == .person ? "sender" : "space",
            cleanMapValue(target.label),
            cleanMapValue(normalizeRoomID(target.roomID)),
            cleanMapValue(roomType),
            cleanMapValue(email),
            cleanMapValue(displayName)
        ].joined(separator: "\t")
    }

    private func cleanMapValue(_ value: String) -> String {
        normalizeLabel(
            value
                .replacingOccurrences(of: "\t", with: " ")
                .replacingOccurrences(of: "\n", with: " ")
                .replacingOccurrences(of: "\r", with: " ")
        )
    }
}

private extension String {
    var casefoldKey: String {
        folding(options: [.caseInsensitive, .diacriticInsensitive], locale: .current)
    }
}

private extension Array {
    subscript(safe index: Index) -> Element? {
        indices.contains(index) ? self[index] : nil
    }
}

private struct PersistedSystemSettingsPayload: Codable {
    var version: Int
    var updatedAt: String
    var settings: PersistedSystemSettingsValues

    enum CodingKeys: String, CodingKey {
        case version
        case updatedAt = "updated_at"
        case settings
    }
}

private struct PersistedAskCodexQueryHistoryPayload: Codable {
    var version: Int
    var updatedAt: String
    var queries: [AskCodexQueryHistoryEntry]

    enum CodingKeys: String, CodingKey {
        case version
        case updatedAt = "updated_at"
        case queries
    }
}

private struct PersistedSystemSettingsValues: Codable {
    var debug: Bool
    var backgroundStatus: Bool
    var webexSyncEnabled: Bool
    var autoQueryAllEnabled: Bool
    var priorityRefreshPausesBackground: Bool
    var codexEnabled: Bool
    var codexAskEnabled: Bool
    var codexQuestionSynthesisEnabled: Bool
    var codexPersonSummariesEnabled: Bool
    var codexSpaceSummariesEnabled: Bool
    var codexClusterTitlesEnabled: Bool
    var codexExecQuestionsEnabled: Bool
    var codexBeliefsEnabled: Bool
    var codexModel: String
    var codexReasoningLevel: String
    var webexSyncMinutes: Int
    var autoQueryAllMinutes: Int
    var trackedActionsRefreshMinutes: Int
    var personFocusRefreshMinutes: Int
    var personFocusDays: Int
    var personFocusAnalysisCadenceHours: Int
    var spaceFocusRefreshMinutes: Int
    var spaceFocusDays: Int
    var spaceFocusAnalysisCadenceHours: Int
    var transcriptionEnabled: Bool
    var transcriptionDiarizationEnabled: Bool
    var transcriptionLanguageMode: String
    var transcriptionMicrophoneGain: Int
    var transcriptionAWSEndpoint: String
    var pollSeconds: Int

    enum CodingKeys: String, CodingKey {
        case debug
        case backgroundStatus = "background_status"
        case webexSyncEnabled = "webex_sync_enabled"
        case autoQueryAllEnabled = "auto_query_all_enabled"
        case priorityRefreshPausesBackground = "priority_refresh_pauses_background"
        case codexEnabled = "codex_enabled"
        case codexAskEnabled = "codex_ask_enabled"
        case codexQuestionSynthesisEnabled = "codex_question_synthesis_enabled"
        case codexPersonSummariesEnabled = "codex_person_summaries_enabled"
        case codexSpaceSummariesEnabled = "codex_space_summaries_enabled"
        case codexClusterTitlesEnabled = "codex_cluster_titles_enabled"
        case codexExecQuestionsEnabled = "codex_exec_questions_enabled"
        case codexBeliefsEnabled = "codex_beliefs_enabled"
        case codexModel = "codex_model"
        case codexReasoningLevel = "codex_reasoning_level"
        case webexSyncMinutes = "webex_sync_minutes"
        case autoQueryAllMinutes = "auto_query_all_minutes"
        case trackedActionsRefreshMinutes = "tracked_actions_refresh_minutes"
        case personFocusRefreshMinutes = "person_focus_refresh_minutes"
        case personFocusDays = "person_focus_days"
        case personFocusAnalysisCadenceHours = "person_focus_analysis_cadence_hours"
        case spaceFocusRefreshMinutes = "space_focus_refresh_minutes"
        case spaceFocusDays = "space_focus_days"
        case spaceFocusAnalysisCadenceHours = "space_focus_analysis_cadence_hours"
        case transcriptionEnabled = "transcription_enabled"
        case transcriptionDiarizationEnabled = "transcription_diarization_enabled"
        case transcriptionLanguageMode = "transcription_language_mode"
        case transcriptionMicrophoneGain = "transcription_microphone_gain"
        case transcriptionAWSEndpoint = "transcription_aws_endpoint"
        case pollSeconds = "poll_seconds"
    }

    init(settings: SystemSettings) {
        self.debug = settings.debug
        self.backgroundStatus = settings.backgroundStatus
        self.webexSyncEnabled = settings.webexSyncEnabled
        self.autoQueryAllEnabled = settings.autoQueryAllEnabled
        self.priorityRefreshPausesBackground = settings.priorityRefreshPausesBackground
        self.codexEnabled = settings.codexEnabled
        self.codexAskEnabled = settings.codexAskEnabled
        self.codexQuestionSynthesisEnabled = settings.codexQuestionSynthesisEnabled
        self.codexPersonSummariesEnabled = settings.codexPersonSummariesEnabled
        self.codexSpaceSummariesEnabled = settings.codexSpaceSummariesEnabled
        self.codexClusterTitlesEnabled = settings.codexClusterTitlesEnabled
        self.codexExecQuestionsEnabled = settings.codexExecQuestionsEnabled
        self.codexBeliefsEnabled = settings.codexBeliefsEnabled
        self.codexModel = settings.codexModel.rawValue
        self.codexReasoningLevel = settings.codexReasoningLevel.rawValue
        self.webexSyncMinutes = max(1, settings.webexSyncMinutes)
        self.autoQueryAllMinutes = max(1, settings.autoQueryAllMinutes)
        self.trackedActionsRefreshMinutes = max(1, settings.trackedActionsRefreshMinutes)
        self.personFocusRefreshMinutes = max(1, settings.personFocusRefreshMinutes)
        self.personFocusDays = SystemSettings.clamped(settings.personFocusDays, to: SystemSettings.focusDaysBounds)
        self.personFocusAnalysisCadenceHours = SystemSettings.clamped(
            settings.personFocusAnalysisCadenceHours,
            to: SystemSettings.focusAnalysisCadenceHoursBounds
        )
        self.spaceFocusRefreshMinutes = max(1, settings.spaceFocusRefreshMinutes)
        self.spaceFocusDays = SystemSettings.clamped(settings.spaceFocusDays, to: SystemSettings.focusDaysBounds)
        self.spaceFocusAnalysisCadenceHours = SystemSettings.clamped(
            settings.spaceFocusAnalysisCadenceHours,
            to: SystemSettings.focusAnalysisCadenceHoursBounds
        )
        self.transcriptionEnabled = settings.transcriptionEnabled
        self.transcriptionDiarizationEnabled = settings.transcriptionDiarizationEnabled
        self.transcriptionLanguageMode = settings.transcriptionLanguageMode.rawValue
        self.transcriptionMicrophoneGain = SystemSettings.clamped(
            settings.transcriptionMicrophoneGain,
            to: SystemSettings.transcriptionMicrophoneGainBounds
        )
        self.transcriptionAWSEndpoint = settings.transcriptionAWSEndpoint.trimmingCharacters(in: .whitespacesAndNewlines)
        self.pollSeconds = max(1, settings.webexSyncMinutes * 60)
    }
}

struct TokenFileStatus: Equatable {
    var rootTokenFileExists: Bool
    var configTokenFileExists: Bool
    var resolvedTokenFilePath: String? = nil
    var healthState: String? = nil
    var accessTokenExpiresAt: Date? = nil
    var accessTokenExpired: Bool? = nil
    var accessTokenExpiringSoon: Bool? = nil
    var refreshTokenExpiresAt: Date? = nil
    var refreshTokenExpired: Bool? = nil
    var refreshTokenExpiringSoon: Bool? = nil
    var refreshTokenPresent: Bool? = nil
    var parseError: String? = nil
}

struct OAuthProviderStatus: Equatable, Identifiable {
    var provider: OAuthProviderKind
    var tokenFileExists: Bool
    var resolvedTokenFilePath: String?
    var healthState: String
    var accessTokenExpiresAt: Date?
    var refreshTokenExpiresAt: Date?
    var refreshTokenPresent: Bool?
    var scope: String?
    var parseError: String?

    var id: String { provider.rawValue }
}

private extension OAuthTokenRecord {
    func withComputedExpiryFallbacks() -> OAuthTokenRecord {
        var updated = self
        if updated.accessTokenExpiresAt == nil,
           let obtainedAt = updated.obtainedAt,
           let expiresIn = updated.expiresIn {
            updated.accessTokenExpiresAt = obtainedAt.addingTimeInterval(TimeInterval(expiresIn))
        }
        if updated.refreshTokenExpiresAt == nil,
           let obtainedAt = updated.obtainedAt,
           let expiresIn = updated.refreshTokenExpiresIn {
            updated.refreshTokenExpiresAt = obtainedAt.addingTimeInterval(TimeInterval(expiresIn))
        }
        return updated
    }
}
