import AppKit
import CryptoKit
import Foundation
import Network

/// Ask Codex context scope selected by the user.
enum AskCodexTargetScope: String, CaseIterable, Identifiable, Codable {
    case allTracked = "all_tracked"
    case selectedSpace = "selected_space"
    case selectedPerson = "selected_person"

    var id: String { rawValue }

    /// Display label used by the Ask Codex scope picker.
    var title: String {
        switch self {
        case .allTracked:
            return "All Tracked Targets"
        case .selectedSpace:
            return "Selected Space"
        case .selectedPerson:
            return "Selected Person"
        }
    }
}

/// Destination type for submitting a transcript into timeline evidence.
enum TranscriptionTimelineTargetKind: String, CaseIterable, Identifiable, Hashable {
    case space
    case person

    var id: String { rawValue }

    /// Picker label for transcript submission targets.
    var title: String {
        switch self {
        case .space:
            return "Space"
        case .person:
            return "Person"
        }
    }

    /// Target-management list that owns this timeline kind.
    var focusTargetKind: FocusTargetManagementKind {
        switch self {
        case .space:
            return .spaceFocus
        case .person:
            return .personFocus
        }
    }
}

/// Timeline submission validation error surfaced directly to the settings UI.
private enum TranscriptionTimelineSubmissionError: LocalizedError {
    case invalidTarget(String)

    var errorDescription: String? {
        switch self {
        case .invalidTarget(let message):
            return message
        }
    }
}

/// Sort mode for focus lists.
enum FocusSortOption: String, CaseIterable, Identifiable {
    case latestMessage = "latest_message"
    case name = "name"

    var id: String { rawValue }

    /// Display label used in focus-list sort controls.
    var title: String {
        switch self {
        case .latestMessage:
            return "Date/Time (Newest)"
        case .name:
            return "Name (A-Z)"
        }
    }
}

/// Long-running settings actions surfaced in the Settings view.
enum SystemSettingsAction: String, Identifiable {
    case syncWebex = "sync_webex"
    case rebuildPersonFocusAll = "rebuild_person_focus_all"
    case rebuildSpaceFocusAll = "rebuild_space_focus_all"

    var id: String { rawValue }
}

/// Editable copy of focus-analysis settings before persistence.
struct FocusAnalysisSettingsDraft: Equatable {
    var personFocusDays: Int
    var personFocusAnalysisCadenceHours: Int
    var spaceFocusDays: Int
    var spaceFocusAnalysisCadenceHours: Int

    /// Seeds draft fields from clamped persisted settings.
    init(settings: SystemSettings = SystemSettings()) {
        personFocusDays = SystemSettings.clamped(settings.personFocusDays, to: SystemSettings.focusDaysBounds)
        personFocusAnalysisCadenceHours = SystemSettings.clamped(
            settings.personFocusAnalysisCadenceHours,
            to: SystemSettings.focusAnalysisCadenceHoursBounds
        )
        spaceFocusDays = SystemSettings.clamped(settings.spaceFocusDays, to: SystemSettings.focusDaysBounds)
        spaceFocusAnalysisCadenceHours = SystemSettings.clamped(
            settings.spaceFocusAnalysisCadenceHours,
            to: SystemSettings.focusAnalysisCadenceHoursBounds
        )
    }

    /// Updates one integer setting while preserving bounds.
    mutating func set(_ key: SystemSettingKey, value: Int) {
        switch key {
        case .personFocusDays:
            personFocusDays = SystemSettings.clamped(value, to: SystemSettings.focusDaysBounds)
        case .personFocusAnalysisCadenceHours:
            personFocusAnalysisCadenceHours = SystemSettings.clamped(
                value,
                to: SystemSettings.focusAnalysisCadenceHoursBounds
            )
        case .spaceFocusDays:
            spaceFocusDays = SystemSettings.clamped(value, to: SystemSettings.focusDaysBounds)
        case .spaceFocusAnalysisCadenceHours:
            spaceFocusAnalysisCadenceHours = SystemSettings.clamped(
                value,
                to: SystemSettings.focusAnalysisCadenceHoursBounds
            )
        default:
            break
        }
    }

    /// Returns the draft value for one focus-analysis setting key.
    func value(for key: SystemSettingKey) -> Int {
        switch key {
        case .personFocusDays:
            return personFocusDays
        case .personFocusAnalysisCadenceHours:
            return personFocusAnalysisCadenceHours
        case .spaceFocusDays:
            return spaceFocusDays
        case .spaceFocusAnalysisCadenceHours:
            return spaceFocusAnalysisCadenceHours
        default:
            return 0
        }
    }

    /// Applies the draft fields to a persisted settings value.
    func applying(to settings: SystemSettings) -> SystemSettings {
        var updated = settings
        updated.setInt(personFocusDays, for: .personFocusDays)
        updated.setInt(personFocusAnalysisCadenceHours, for: .personFocusAnalysisCadenceHours)
        updated.setInt(spaceFocusDays, for: .spaceFocusDays)
        updated.setInt(spaceFocusAnalysisCadenceHours, for: .spaceFocusAnalysisCadenceHours)
        return updated
    }
}

/// Completed Ask Codex run shown in job/result views.
struct AskCodexRunRecord: Hashable {
    var jobID: String
    var title: String
    var targetScope: AskCodexTargetScope
    var targetTitle: String
    var question: String
    var submittedAt: Date
    var output: String
    var outputPath: String
    var logPath: String
    var metadataPath: String
    var attempts: Int
    var status: String
}

/// Persisted Ask Codex prompt history entry.
struct AskCodexQueryHistoryEntry: Codable, Identifiable, Hashable {
    var id: String
    var question: String
    var targetScope: AskCodexTargetScope
    var targetTitle: String
    var targetKey: String
    var targetItemID: String?
    var submittedAt: String
}

/// Selectable target for scoped belief views.
struct BeliefTargetOption: Identifiable, Hashable {
    var entityKey: String
    var title: String
    var subtitle: String

    var id: String { entityKey }
}

/// UI status for one refresh scope.
struct RefreshRunStatus: Identifiable, Hashable {
    var scope: RefreshScope
    var isRunning: Bool
    var lastStartedAt: String?
    var lastCompletedAt: String?
    var lastSummary: String?
    var lastError: String?

    var id: String { scope.rawValue }
}

/// Per-step state in the visible refresh progress pipeline.
enum RefreshProgressStepState: String, Hashable {
    case pending
    case running
    case succeeded
    case skipped
    case failed
}

/// One visible step in a refresh pipeline run.
struct RefreshProgressStep: Identifiable, Hashable {
    var id: String
    var scope: RefreshScope
    var title: String
    var usesCodex: Bool
    var state: RefreshProgressStepState
    var summary: String?
    var error: String?
}

/// Whole-pipeline progress state for foreground refreshes.
struct RefreshProgressState: Hashable {
    var isActive: Bool
    var title: String
    var startedAt: Date?
    var completedAt: Date?
    var steps: [RefreshProgressStep]
    var currentStepID: String?
    var currentScope: RefreshScope?
    var lastSummary: String?

    static let idle = RefreshProgressState(
        isActive: false,
        title: "",
        startedAt: nil,
        completedAt: nil,
        steps: [],
        currentStepID: nil,
        currentScope: nil,
        lastSummary: nil
    )

    /// Number of steps in the visible pipeline.
    var totalStepCount: Int { steps.count }

    /// Terminal steps across success, skip, and failure states.
    var completedStepCount: Int {
        steps.filter { $0.state == .succeeded || $0.state == .skipped || $0.state == .failed }.count
    }

    /// Step currently receiving progress/error updates.
    var currentStep: RefreshProgressStep? {
        if let currentStepID {
            return steps.first { $0.id == currentStepID }
        }
        guard let currentScope else { return nil }
        return steps.first { $0.scope == currentScope }
    }
}

/// Concrete context sent to Ask Codex.
private struct AskCodexTargetContext {
    var key: String
    var title: String
    var itemID: String?
    var lines: [String]
}

/// Prompt-size limits for selected-target and all-tracked contexts.
private struct AskCodexContextLimits {
    var maxItemsPerKind: Int
    var maxIntroLines: Int
    var maxDetailLines: Int
    var maxSections: Int
    var maxSectionLines: Int
    var maxTailLines: Int
    var maxQuestions: Int

    static let allTracked = AskCodexContextLimits(
        maxItemsPerKind: 30,
        maxIntroLines: 10,
        maxDetailLines: 12,
        maxSections: 4,
        maxSectionLines: 8,
        maxTailLines: 6,
        maxQuestions: 20
    )

    static let selected = AskCodexContextLimits(
        maxItemsPerKind: 1,
        maxIntroLines: 40,
        maxDetailLines: 80,
        maxSections: 8,
        maxSectionLines: 30,
        maxTailLines: 30,
        maxQuestions: 10
    )
}

/// Cache key for belief list reads.
private struct BeliefCacheKey: Hashable {
    var scope: KnowledgeBeliefScope
    var entityKey: String
}

/// Cached belief rows for one scope/entity selection.
private struct CachedBeliefSet {
    var manualBeliefs: [BeliefRecord]
    var automaticBeliefs: [BeliefRecord]
    var loadedAt: Date
}

/// Entity to include in belief-set summary counts.
private struct BeliefSummaryTarget: Hashable {
    var scope: KnowledgeBeliefScope
    var entityKey: String
    var title: String
}

/// Cached belief summary list plus the target fingerprint it covered.
private struct CachedBeliefSetSummaries {
    var summaries: [BeliefSetSummary]
    var targetFingerprint: String
    var loadedAt: Date
}

/// Internal refresh step with execution mode and cadence behavior.
private struct RefreshPipelinePlan: Hashable {
    var id: String
    var scope: RefreshScope
    var title: String
    var mode: RefreshExecutionMode
    var markCadence: Bool
    var skipWhenPreviousScopeReused: RefreshScope? = nil

    /// Whether this step can launch Codex work.
    var usesCodex: Bool {
        if mode == .codexOnly {
            return true
        }
        switch scope {
        case .questions, .codexJobs, .beliefMaintenance:
            return true
        case .personFocus, .spaceFocus:
            return mode == .full || mode == .cacheAwareFull
        case .webexSync:
            return false
        }
    }
}

/// Page-triggered refresh request that can interrupt background work.
private struct PagePriorityRefreshRequest: Hashable {
    var section: AppSection
    var title: String
    var plans: [RefreshPipelinePlan]
    var reloadAfterEachScope: Bool
}

/// Disk checkpoint for restart/resume of a refresh pipeline.
private struct PersistedRefreshCheckpoint: Codable {
    var version: Int
    var runID: String
    var title: String
    var startedAt: String
    var updatedAt: String
    var stateFingerprint: String
    var reloadAfterEachScope: Bool
    var isPriorityRefresh: Bool
    var steps: [PersistedRefreshCheckpointStep]

    enum CodingKeys: String, CodingKey {
        case version
        case runID = "run_id"
        case title
        case startedAt = "started_at"
        case updatedAt = "updated_at"
        case stateFingerprint = "state_fingerprint"
        case reloadAfterEachScope = "reload_after_each_scope"
        case isPriorityRefresh = "is_priority_refresh"
        case steps
    }
}

/// Disk checkpoint row for one refresh pipeline step.
private struct PersistedRefreshCheckpointStep: Codable {
    var id: String
    var scopeRawValue: String
    var title: String
    var modeRawValue: String
    var markCadence: Bool
    var skipWhenPreviousScopeReusedRawValue: String?
    var stateRawValue: String
    var summary: String?
    var error: String?

    enum CodingKeys: String, CodingKey {
        case id
        case scopeRawValue = "scope"
        case title
        case modeRawValue = "mode"
        case markCadence = "mark_cadence"
        case skipWhenPreviousScopeReusedRawValue = "skip_when_previous_scope_reused"
        case stateRawValue = "state"
        case summary
        case error
    }
}

/// Main product state store and orchestration layer for the Mac app.
@MainActor
final class AppModel: ObservableObject {
    @Published var selectedSection: AppSection = .home
    @Published var selectedFocusKind: FocusKind = .space
    @Published private var selectedItemIDByKind: [FocusKind: String] = [:]
    @Published private var searchTextByKind: [FocusKind: String] = [:]
    @Published private var sortOptionByKind: [FocusKind: FocusSortOption] = [:]
    @Published private var expandedDetailSectionIDsByItemKey: [String: Set<String>] = [:]
    @Published private(set) var spaceCache: FocusCache = .empty(kind: .space)
    @Published private(set) var personCache: FocusCache = .empty(kind: .person)
    @Published private(set) var runtimeStatus: RuntimeStatus
    @Published private(set) var isLoading: Bool = false
    @Published var errorMessage: String?
    @Published var askCodexTargetScope: AskCodexTargetScope = .allTracked
    @Published var askCodexQuestion: String = ""
    @Published private(set) var askCodexIsRunning: Bool = false
    @Published private(set) var codexActivityMessage: String?
    @Published private(set) var askCodexResult: AskCodexRunRecord?
    @Published private(set) var askCodexLastError: String?
    @Published private(set) var askCodexQueryHistory: [AskCodexQueryHistoryEntry] = []
    @Published var beliefScopeFilter: KnowledgeBeliefScope = .global
    @Published private(set) var beliefEntityKeyByScope: [KnowledgeBeliefScope: String] = [:]
    @Published private(set) var manualBeliefs: [BeliefRecord] = []
    @Published private(set) var automaticBeliefs: [BeliefRecord] = []
    @Published private(set) var selectedBeliefID: String?
    @Published private(set) var beliefsIsLoading: Bool = false
    @Published private(set) var beliefsLastError: String?
    @Published private(set) var beliefSetSummaries: [BeliefSetSummary] = []
    @Published private(set) var beliefSetSummariesIsLoading: Bool = false
    @Published private(set) var beliefSetSummariesLastError: String?
    @Published private(set) var refreshPlans: [RefreshPlan] = []
    @Published private(set) var refreshStatuses: [RefreshScope: RefreshRunStatus] = [:]
    @Published private(set) var refreshProgress: RefreshProgressState = .idle
    @Published private(set) var backgroundRefreshActive: Bool = false
    @Published private(set) var questionCandidates: [QuestionCandidate] = []
    @Published private(set) var questionsIsLoading: Bool = false
    @Published private(set) var questionsLastError: String?
    @Published var selectedQuestionID: String?
    @Published private(set) var runtimeAccessDenied: Bool = false
    @Published private(set) var systemSettings: SystemSettings = SystemSettings()
    @Published private(set) var focusAnalysisDraft: FocusAnalysisSettingsDraft = FocusAnalysisSettingsDraft()
    @Published private(set) var focusAnalysisCacheStatusByKind: [FocusKind: FocusAnalysisCacheStatus] = [:]
    @Published private(set) var settingsActionID: String?
    @Published private(set) var settingsLastMessage: String?
    @Published private(set) var settingsLastError: String?
    @Published private(set) var oauthProviderStatuses: [OAuthProviderKind: OAuthProviderStatus] = [:]
    @Published private(set) var oauthActionID: String?
    @Published private(set) var spaceFocusManagementTargets: [ConfigTarget] = []
    @Published private(set) var spaceFocusCandidateTargets: [ConfigTarget] = []
    @Published private(set) var personFocusManagementTargets: [ConfigTarget] = []
    @Published private(set) var personFocusCandidateTargets: [ConfigTarget] = []
    @Published private(set) var execFocusManagementTargets: [ConfigTarget] = []
    @Published private(set) var execFocusCandidateTargets: [ConfigTarget] = []
    @Published var transcriptionTimelineTargetKind: TranscriptionTimelineTargetKind = .space {
        didSet {
            ensureTranscriptionTimelineTargetSelection(for: transcriptionTimelineTargetKind)
        }
    }
    @Published private var selectedTranscriptionTimelineTargetIDByKind: [TranscriptionTimelineTargetKind: String] = [:]
    @Published private(set) var transcriptionTimelineSubmissionRunning: Bool = false
    @Published private(set) var transcriptionTimelineSubmissionMessage: String?
    @Published private(set) var transcriptionTimelineSubmissionError: String?
    @Published private(set) var targetManagementIsLoading: Bool = false
    @Published private(set) var targetManagementMutationID: String?
    @Published private(set) var targetManagementLastMessage: String?
    @Published private(set) var targetManagementLastError: String?
    @Published var personIMessageHandleDraft: String = ""
    @Published private var selectedFocusTargetIDByKind: [FocusTargetManagementKind: String] = [:]

    let runtimeStore: NativeRuntimeStore
    let configStore: ConfigStore
    let codexRunner: CodexRunner
    let knowledgeStore: KnowledgeStore
    let codexOrchestrationService: CodexPromptOrchestrationService
    let questionService: QuestionCandidateService
    let refreshCoordinator: NativeRefreshCoordinator
    let oauthService: OAuthService
    let transcriptionViewModel: TranscriptionViewModel

    private var refreshLoopTask: Task<Void, Never>?
    private var lastRefreshDateByScope: [RefreshScope: Date] = [:]
    private var refreshCycleRunning = false
    private var activeRefreshCycleID: UUID?
    private var interruptedRefreshCycleIDs: Set<UUID> = []
    private var startupRefreshStarted = false
    private var activeRefreshCheckpointRunID: UUID?
    private var activeRefreshCheckpointTitle: String = ""
    private var activeRefreshCheckpointFingerprint: String = ""
    private var activeRefreshCheckpointReloadAfterEachScope = false
    private var activeRefreshCheckpointIsPriority = false
    private var activeRefreshPipelinePlansByID: [String: RefreshPipelinePlan] = [:]
    private var wakeObserver: NSObjectProtocol?
    private var networkPathMonitor: NWPathMonitor?
    private let networkMonitorQueue = DispatchQueue(label: "Cubicle.NetworkMonitor")
    private var lastNetworkPathSatisfied: Bool?
    private var beliefSetCache: [BeliefCacheKey: CachedBeliefSet] = [:]
    private var activeBeliefLoadKey: BeliefCacheKey?
    private var cachedBeliefSetSummaries: CachedBeliefSetSummaries?
    private var activeBeliefSummaryFingerprint: String?
    private let activeQuestionStatuses: Set<QuestionStatus> = [.candidate, .surfaced]
    private static let beliefCacheFreshnessInterval: TimeInterval = 120
    private static let refreshCheckpointStaleInterval: TimeInterval = 30 * 60

    /// Wires runtime-backed services and restores persisted UI-adjacent state.
    init(
        services: AppServices = AppServices(),
        transcriptionClient: TranscriptionClient = TranscriptionWebSocketClient(),
        audioCaptureService: AudioCaptureService = MicrophoneAudioCaptureService()
    ) {
        self.runtimeStore = services.runtimeStore
        self.configStore = services.configStore
        self.codexRunner = services.codexRunner
        self.knowledgeStore = services.knowledgeStore
        self.codexOrchestrationService = services.codexOrchestrationService
        self.questionService = services.questionService
        self.refreshCoordinator = services.makeRefreshCoordinator()
        self.oauthService = services.oauthService
        self.transcriptionViewModel = TranscriptionViewModel(
            client: transcriptionClient,
            audioCaptureService: audioCaptureService,
            authTokenLoader: { [configStore] in
                await Task.detached(priority: .utility) {
                    configStore.loadTranscriptionAuthToken()
                }.value
            }
        )
        self.runtimeStatus = runtimeStore.runtimeStatus()
        self.systemSettings = SystemSettings()
        self.transcriptionViewModel.apply(settings: self.systemSettings)
        self.refreshPlans = []
        self.refreshStatuses = Dictionary(
            uniqueKeysWithValues: RefreshScope.allCases.map { scope in
                (scope, RefreshRunStatus(scope: scope, isRunning: false))
            }
        )
        reloadOAuthStatuses()
        ensureBeliefTargetSelection(for: .global)
    }

    convenience init(
        runtimeStore: NativeRuntimeStore,
        transcriptionClient: TranscriptionClient = TranscriptionWebSocketClient(),
        audioCaptureService: AudioCaptureService = MicrophoneAudioCaptureService()
    ) {
        self.init(
            services: AppServices(runtimeStore: runtimeStore),
            transcriptionClient: transcriptionClient,
            audioCaptureService: audioCaptureService
        )
    }

    deinit {
        refreshLoopTask?.cancel()
        if let wakeObserver {
            NSWorkspace.shared.notificationCenter.removeObserver(wakeObserver)
        }
        networkPathMonitor?.cancel()
    }

    /// Persists in-flight refresh state and shuts down transcription work.
    func handleAppWillTerminate() {
        persistActiveRefreshCheckpoint(cycleID: activeRefreshCycleID)
        Task { [transcriptionViewModel] in
            await transcriptionViewModel.stopSession()
        }
    }

    /// Performs first-load work and optional startup refresh once per app launch.
    func startProgram() async {
        guard !startupRefreshStarted else {
            return
        }
        startupRefreshStarted = true
        let loaded = await loadAll()
        guard loaded else {
            return
        }
        if systemSettings.backgroundStatus {
            let resumed = await resumeRefreshCheckpointIfNeeded()
            if !resumed {
                _ = await runRefreshCycle(
                    title: "Startup refresh",
                    forceAll: true,
                    pipelinePlans: startupRefreshPipelinePlans(),
                    reloadAfterEachScope: false
                )
            }
        }
        installSystemEventObserversIfNeeded()
        startBackgroundRefreshIfNeeded()
    }

    @discardableResult
    /// Loads runtime files, settings, caches, questions, and target lists.
    func loadAll() async -> Bool {
        isLoading = true
        errorMessage = nil
        let runtimeStore = self.runtimeStore
        let configStore = self.configStore
        let refreshCoordinator = self.refreshCoordinator
        let initialState = await Task.detached(priority: .utility) {
            (
                runtimeStore.runtimeStatus(),
                configStore.loadSystemSettings(),
                refreshCoordinator.defaultPlans(),
                configStore.loadAskCodexQueryHistory()
            )
        }.value
        runtimeStatus = initialState.0
        systemSettings = initialState.1
        transcriptionViewModel.apply(settings: systemSettings)
        syncFocusAnalysisDraftFromSystemSettings()
        refreshPlans = initialState.2
        askCodexQueryHistory = initialState.3
        reloadOAuthStatuses()
        do {
            try knowledgeStore.bootstrap()
            spaceCache = try runtimeStore.loadFocusCache(kind: .space)
            personCache = try runtimeStore.loadFocusCache(kind: .person)
            spaceCache = canonicalizeSpaceTitles(spaceCache)
            pruneDetailSectionExpansionState(for: .space)
            pruneDetailSectionExpansionState(for: .person)
            ensureSelection(for: .space)
            ensureSelection(for: .person)
            ensureBeliefTargetSelection(for: .person)
            ensureBeliefTargetSelection(for: .space)
            reloadFocusTargetManagementState()
            try await refreshQuestionsIfEmpty()
            try loadQuestionsFromStore()
            runtimeAccessDenied = false
            if selectedSection == .beliefs {
                await refreshBeliefSetSummaries()
            }
            isLoading = false
            return true
        } catch {
            handleRuntimeAccessError(error)
            isLoading = false
            return false
        }
    }

    /// Starts the periodic refresh loop when settings and runtime access allow it.
    func startBackgroundRefreshIfNeeded() {
        guard refreshLoopTask == nil, !runtimeAccessDenied, systemSettings.backgroundStatus else {
            return
        }
        backgroundRefreshActive = true
        refreshLoopTask = Task { [weak self] in
            while !Task.isCancelled {
                try? await Task.sleep(nanoseconds: 60_000_000_000)
                _ = await self?.runRefreshCycle(
                    title: "Background refresh",
                    forceAll: false,
                    pipelinePlans: nil,
                    reloadAfterEachScope: false
                )
            }
        }
    }

    /// Runs a foreground full refresh across enabled scopes.
    func refreshNow() async {
        _ = await runRefreshCycle(
            title: "Manual refresh",
            forceAll: true,
            pipelinePlans: nil,
            reloadAfterEachScope: false,
            taskPriority: .userInitiated
        )
    }

    /// Runs the refresh plan for the currently selected page.
    func refreshSelectedPageNow() async {
        let request = priorityRefreshRequest(for: selectedSection)
        if refreshCycleRunning {
            if systemSettings.priorityRefreshPausesBackground {
                interruptActiveRefreshForPriority(request)
            } else {
                markPriorityRefreshSkippedBecauseBusy(request)
                return
            }
        }
        await runPriorityRefresh(request)
    }

    /// Reloads settings and derived UI state from disk/keychain.
    func reloadSystemSettings() {
        systemSettings = loadSystemSettingsWithSecureFields()
        transcriptionViewModel.apply(settings: systemSettings)
        syncFocusAnalysisDraftFromSystemSettings()
        refreshPlans = refreshCoordinator.defaultPlans()
        settingsLastError = nil
        reloadOAuthStatuses()
    }

    /// Updates the unsaved focus-analysis draft and previews matching caches.
    func updateFocusAnalysisDraft(_ key: SystemSettingKey, intValue: Int) {
        var draft = focusAnalysisDraft
        draft.set(key, value: intValue)
        guard draft != focusAnalysisDraft else {
            return
        }
        focusAnalysisDraft = draft
        updateFocusAnalysisCacheStatus()
        previewExactFocusAnalysisCacheIfAvailable(for: focusKind(forDraftKey: key))
        settingsLastError = nil
        settingsLastMessage = "Focus analysis settings changed. Apply / Refresh to commit."
    }

    /// Restores the focus-analysis draft from persisted settings.
    func resetFocusAnalysisDraft() {
        syncFocusAnalysisDraftFromSystemSettings()
        settingsLastMessage = "Focus analysis draft reset to saved values."
        settingsLastError = nil
    }

    /// Checks whether the focus-analysis draft differs from saved settings.
    func focusAnalysisDraftHasChanges(kind: FocusKind? = nil) -> Bool {
        switch kind {
        case .person:
            return focusAnalysisDraft.personFocusDays != systemSettings.personFocusDays
                || focusAnalysisDraft.personFocusAnalysisCadenceHours != systemSettings.personFocusAnalysisCadenceHours
        case .space:
            return focusAnalysisDraft.spaceFocusDays != systemSettings.spaceFocusDays
                || focusAnalysisDraft.spaceFocusAnalysisCadenceHours != systemSettings.spaceFocusAnalysisCadenceHours
        case nil:
            return focusAnalysisDraftHasChanges(kind: .person)
                || focusAnalysisDraftHasChanges(kind: .space)
        }
    }

    /// Human summary for whether an exact focus-analysis cache is reusable.
    func focusAnalysisStatusText(kind: FocusKind) -> String {
        let status = focusAnalysisCacheStatusByKind[kind] ?? computedFocusAnalysisCacheStatus(kind: kind)
        return status.summary
    }

    /// Whether the settings refresh action for one focus kind is active.
    func isFocusAnalysisRefreshRunning(kind: FocusKind) -> Bool {
        settingsActionID == focusAnalysisRefreshActionID(kind: kind)
    }

    /// Persists focus-analysis settings and rebuilds or reuses matching cache output.
    func applyFocusAnalysisDraftAndRefresh(kind: FocusKind) async {
        guard settingsActionID == nil else {
            settingsLastMessage = "A settings action is already running."
            return
        }
        guard !refreshCycleRunning else {
            settingsLastMessage = "Refresh is already running."
            return
        }

        settingsActionID = focusAnalysisRefreshActionID(kind: kind)
        settingsLastError = nil
        defer {
            settingsActionID = nil
        }

        let exactCache = try? runtimeStore.loadExactFocusAnalysisCache(
            kind: kind,
            focusDays: focusAnalysisDraftDays(kind: kind),
            analysisCadenceHours: focusAnalysisDraftCadenceHours(kind: kind)
        )
        let updated = focusAnalysisDraft.applying(to: systemSettings)
        guard persistSystemSettings(updated, message: "Focus analysis settings applied.") else {
            return
        }
        syncFocusAnalysisDraftFromSystemSettings()

        if let exactCache {
            applyFocusAnalysisCache(exactCache, kind: kind)
            settingsLastMessage = "\(kind.title) exact analysis cache reused."
            return
        }

        await runSettingsFocusRebuildAction(
            title: "Apply / Refresh \(kind.title)",
            stepID: focusAnalysisRefreshActionID(kind: kind),
            scope: kind == .person ? .personFocus : .spaceFocus,
            mode: .full
        )
        updateFocusAnalysisCacheStatus()
    }

    private func syncFocusAnalysisDraftFromSystemSettings() {
        focusAnalysisDraft = FocusAnalysisSettingsDraft(settings: systemSettings)
        updateFocusAnalysisCacheStatus()
    }

    private func updateFocusAnalysisCacheStatus() {
        focusAnalysisCacheStatusByKind = [
            .person: computedFocusAnalysisCacheStatus(kind: .person),
            .space: computedFocusAnalysisCacheStatus(kind: .space)
        ]
    }

    private func computedFocusAnalysisCacheStatus(kind: FocusKind) -> FocusAnalysisCacheStatus {
        runtimeStore.focusAnalysisCacheStatus(
            kind: kind,
            focusDays: focusAnalysisDraftDays(kind: kind),
            analysisCadenceHours: focusAnalysisDraftCadenceHours(kind: kind)
        )
    }

    private func previewExactFocusAnalysisCacheIfAvailable(for kind: FocusKind?) {
        guard let kind else {
            return
        }
        guard let exactCache = try? runtimeStore.loadExactFocusAnalysisCache(
            kind: kind,
            focusDays: focusAnalysisDraftDays(kind: kind),
            analysisCadenceHours: focusAnalysisDraftCadenceHours(kind: kind)
        ) else {
            return
        }
        applyFocusAnalysisCache(exactCache, kind: kind)
        settingsLastMessage = "\(kind.title) exact analysis cache preview loaded. Apply / Refresh to commit the setting."
    }

    private func applyFocusAnalysisCache(_ cache: FocusCache, kind: FocusKind) {
        switch kind {
        case .space:
            spaceCache = canonicalizeSpaceTitles(cache)
            pruneDetailSectionExpansionState(for: .space)
            ensureSelection(for: .space)
            ensureBeliefTargetSelection(for: .space)
        case .person:
            personCache = cache
            pruneDetailSectionExpansionState(for: .person)
            ensureSelection(for: .person)
            ensureBeliefTargetSelection(for: .person)
        }
        runtimeStatus = runtimeStore.runtimeStatus()
        reloadFocusTargetManagementState()
        if askCodexTargetScope == (kind == .space ? .selectedSpace : .selectedPerson) || askCodexTargetScope == .allTracked {
            askCodexResult = nil
            askCodexLastError = nil
        }
    }

    private func focusKind(forDraftKey key: SystemSettingKey) -> FocusKind? {
        switch key {
        case .personFocusDays, .personFocusAnalysisCadenceHours:
            return .person
        case .spaceFocusDays, .spaceFocusAnalysisCadenceHours:
            return .space
        default:
            return nil
        }
    }

    private func focusAnalysisDraftDays(kind: FocusKind) -> Int {
        switch kind {
        case .person:
            return focusAnalysisDraft.personFocusDays
        case .space:
            return focusAnalysisDraft.spaceFocusDays
        }
    }

    private func focusAnalysisDraftCadenceHours(kind: FocusKind) -> Int {
        switch kind {
        case .person:
            return focusAnalysisDraft.personFocusAnalysisCadenceHours
        case .space:
            return focusAnalysisDraft.spaceFocusAnalysisCadenceHours
        }
    }

    private func focusAnalysisRefreshActionID(kind: FocusKind) -> String {
        "focus_analysis_refresh_\(kind.rawValue)"
    }

    /// Reloads OAuth status indicators for every configured provider.
    func reloadOAuthStatuses() {
        oauthProviderStatuses = Dictionary(
            uniqueKeysWithValues: OAuthProviderKind.allCases.map { provider in
                (provider, configStore.oauthProviderStatus(provider: provider))
            }
        )
    }

    /// Returns cached provider status, falling back to a fresh config read.
    func oauthStatus(for provider: OAuthProviderKind) -> OAuthProviderStatus {
        oauthProviderStatuses[provider] ?? configStore.oauthProviderStatus(provider: provider)
    }

    /// Whether an OAuth connect/revoke operation is in flight for a provider.
    func isOAuthActionRunning(_ provider: OAuthProviderKind) -> Bool {
        oauthActionID == provider.rawValue
    }

    /// Starts browser-based OAuth and persists the resulting token.
    func connectOAuth(provider: OAuthProviderKind) async {
        guard oauthActionID == nil else {
            settingsLastMessage = "An OAuth action is already running."
            return
        }
        oauthActionID = provider.rawValue
        settingsLastError = nil
        settingsLastMessage = "Opening \(provider.displayName) OAuth in your browser."
        defer {
            oauthActionID = nil
            reloadOAuthStatuses()
        }

        do {
            let outcome = try await oauthService.authorize(provider: provider)
            settingsLastMessage = "\(provider.displayName) OAuth token saved to \(outcome.tokenFile.path)."
            settingsLastError = nil
        } catch {
            settingsLastError = error.localizedDescription
        }
    }

    /// Removes locally persisted OAuth material for a provider.
    func revokeOAuth(provider: OAuthProviderKind) {
        guard oauthActionID == nil else {
            settingsLastMessage = "An OAuth action is already running."
            return
        }
        oauthActionID = provider.rawValue
        settingsLastError = nil
        defer {
            oauthActionID = nil
            reloadOAuthStatuses()
        }

        do {
            let removedFiles = try oauthService.revoke(provider: provider)
            if removedFiles.isEmpty {
                settingsLastMessage = "\(provider.displayName) OAuth token was not present."
            } else {
                settingsLastMessage = "\(provider.displayName) OAuth token revoked locally."
            }
        } catch {
            settingsLastError = error.localizedDescription
        }
    }

    /// Persists one boolean setting and applies dependent runtime changes.
    func updateSystemSetting(_ key: SystemSettingKey, boolValue: Bool) {
        let previousValue = systemSettings.boolValue(for: key)
        var updated = systemSettings
        updated.setBool(boolValue, for: key)
        if persistSystemSettings(updated, message: "\(settingTitle(key)) saved.") {
            handleBooleanSystemSettingChange(
                key,
                previousValue: previousValue,
                currentValue: systemSettings.boolValue(for: key)
            )
        }
    }

    /// Persists one integer setting and applies dependent runtime changes.
    func updateSystemSetting(_ key: SystemSettingKey, intValue: Int) {
        let previousValue = systemSettings.intValue(for: key)
        var updated = systemSettings
        updated.setInt(intValue, for: key)
        if persistSystemSettings(updated, message: "\(settingTitle(key)) set to \(updated.intValue(for: key)).") {
            handleIntegerSystemSettingChange(
                key,
                previousValue: previousValue,
                currentValue: systemSettings.intValue(for: key)
            )
        }
    }

    /// Persists one string-backed setting.
    func updateSystemSetting(_ key: SystemSettingKey, stringValue: String) {
        var updated = systemSettings
        updated.setString(stringValue, for: key)
        let valueDescription: String
        switch key {
        case .codexModel:
            valueDescription = updated.codexModel.displayName
        case .codexReasoningLevel:
            valueDescription = updated.codexReasoningLevel.displayName
        case .transcriptionLanguageMode:
            valueDescription = updated.transcriptionLanguageMode.displayName
        case .transcriptionAWSEndpoint:
            valueDescription = updated.transcriptionAWSEndpoint.isEmpty ? "empty" : updated.transcriptionAWSEndpoint
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
             .transcriptionMicrophoneGain,
             .webexSyncMinutes,
             .autoQueryAllMinutes,
             .trackedActionsRefreshMinutes,
             .personFocusRefreshMinutes,
             .personFocusDays,
             .personFocusAnalysisCadenceHours,
             .spaceFocusRefreshMinutes,
             .spaceFocusDays,
             .spaceFocusAnalysisCadenceHours,
             .pollSeconds:
            valueDescription = updated.stringValue(for: key)
        }
        persistSystemSettings(updated, message: "\(settingTitle(key)) set to \(valueDescription).")
    }

    /// Whether a transcription auth token is present in settings or Keychain.
    var transcriptionAuthTokenConfigured: Bool {
        if systemSettings.transcriptionAuthToken?.trimmingCharacters(in: .whitespacesAndNewlines).isEmpty == false {
            return true
        }
        return configStore.transcriptionAuthTokenConfigured()
    }

    /// Normalizes and saves the transcription auth token to Keychain.
    func saveTranscriptionAuthToken(_ token: String) {
        let normalized = normalizedTranscriptionAuthToken(token)
        guard !normalized.isEmpty else {
            settingsLastError = "Enter a transcription service token before saving."
            return
        }
        do {
            try configStore.saveTranscriptionAuthToken(normalized)
            systemSettings.transcriptionAuthToken = normalized
            transcriptionViewModel.apply(settings: systemSettings)
            settingsLastMessage = "Transcription service token saved to Keychain."
            settingsLastError = nil
        } catch {
            settingsLastError = error.localizedDescription
        }
    }

    private func normalizedTranscriptionAuthToken(_ token: String) -> String {
        var normalized = token
            .trimmingCharacters(in: .whitespacesAndNewlines)
            .trimmingCharacters(in: CharacterSet(charactersIn: "\"'"))
        if let bearerRange = normalized.range(of: "Bearer ", options: [.caseInsensitive]) {
            let prefix = normalized[..<bearerRange.lowerBound].trimmingCharacters(in: .whitespacesAndNewlines)
            if prefix.isEmpty || prefix.localizedCaseInsensitiveCompare("Authorization:") == .orderedSame {
                normalized = String(normalized[bearerRange.upperBound...])
            }
        }
        return normalized
            .trimmingCharacters(in: .whitespacesAndNewlines)
            .trimmingCharacters(in: CharacterSet(charactersIn: "\"'"))
    }

    /// Removes the transcription auth token from Keychain and active settings.
    func deleteTranscriptionAuthToken() {
        do {
            try configStore.deleteTranscriptionAuthToken()
            systemSettings.transcriptionAuthToken = nil
            transcriptionViewModel.apply(settings: systemSettings)
            settingsLastMessage = "Transcription service token removed from Keychain."
            settingsLastError = nil
        } catch {
            settingsLastError = error.localizedDescription
        }
    }

    /// Runs one settings-triggered refresh action.
    func runSystemSettingsAction(_ action: SystemSettingsAction) async {
        guard settingsActionID == nil else {
            settingsLastMessage = "A settings action is already running."
            return
        }
        guard !refreshCycleRunning else {
            settingsLastMessage = "Refresh is already running."
            return
        }

        settingsActionID = action.rawValue
        settingsLastError = nil
        defer {
            settingsActionID = nil
        }

        switch action {
        case .syncWebex:
            await runSettingsWebexSyncAction()
        case .rebuildPersonFocusAll:
            await runSettingsFocusRebuildAction(
                title: "Rebuild Person Focus",
                stepID: action.rawValue,
                scope: .personFocus
            )
        case .rebuildSpaceFocusAll:
            await runSettingsFocusRebuildAction(
                title: "Rebuild Space Focus",
                stepID: action.rawValue,
                scope: .spaceFocus
            )
        }
    }

    /// Whether a settings-triggered action is currently active.
    func isSystemSettingsActionRunning(_ action: SystemSettingsAction) -> Bool {
        settingsActionID == action.rawValue
    }

    @discardableResult
    /// Saves settings and refreshes derived service/UI state.
    private func persistSystemSettings(_ settings: SystemSettings, message: String) -> Bool {
        do {
            try configStore.saveSystemSettings(settings)
            systemSettings = loadSystemSettingsWithSecureFields()
            transcriptionViewModel.apply(settings: systemSettings)
            refreshPlans = refreshCoordinator.defaultPlans()
            runtimeStatus = runtimeStore.runtimeStatus()
            settingsLastMessage = message
            settingsLastError = nil
            return true
        } catch {
            settingsLastError = error.localizedDescription
            return false
        }
    }

    /// Applies side effects for boolean setting changes.
    private func handleBooleanSystemSettingChange(
        _ key: SystemSettingKey,
        previousValue: Bool,
        currentValue: Bool
    ) {
        guard previousValue != currentValue else {
            return
        }

        switch key {
        case .webexSyncEnabled:
            handleWebexSyncSettingChange(previousValue: previousValue, currentValue: currentValue)
        case .transcriptionEnabled:
            settingsLastMessage = currentValue
                ? "Live transcription enabled. It will not capture or stream until a session is started."
                : "Live transcription disabled. Capture and streaming stopped."
        case .transcriptionDiarizationEnabled:
            settingsLastMessage = currentValue
                ? "Speaker diarization will be included in new transcription sessions."
                : "Speaker diarization disabled for new transcription sessions."
        case .backgroundStatus:
            handleBackgroundSummariesSettingChange(currentValue: currentValue)
        case .codexEnabled,
             .codexAskEnabled,
             .codexQuestionSynthesisEnabled,
             .codexPersonSummariesEnabled,
             .codexSpaceSummariesEnabled,
             .codexClusterTitlesEnabled,
             .codexExecQuestionsEnabled,
             .codexBeliefsEnabled,
             .autoQueryAllEnabled:
            if !currentValue {
                interruptActiveRefreshForSettingsChange("\(settingTitle(key)) disabled in Settings.")
            }
        case .debug,
             .priorityRefreshPausesBackground,
             .webexSyncMinutes,
             .codexModel,
             .codexReasoningLevel,
             .transcriptionLanguageMode,
             .transcriptionMicrophoneGain,
             .transcriptionAWSEndpoint,
             .autoQueryAllMinutes,
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

    /// Loads settings while preserving secure fields held only in memory/Keychain.
    private func loadSystemSettingsWithSecureFields() -> SystemSettings {
        var settings = configStore.loadSystemSettings()
        settings.transcriptionAuthToken = systemSettings.transcriptionAuthToken
        return settings
    }

    /// Applies side effects for integer setting changes.
    private func handleIntegerSystemSettingChange(
        _ key: SystemSettingKey,
        previousValue: Int,
        currentValue: Int
    ) {
        guard previousValue != currentValue else {
            return
        }

        switch key {
        case .personFocusDays:
            reloadFocusCacheForWindowSettingChange(kind: .person, days: currentValue)
        case .spaceFocusDays:
            reloadFocusCacheForWindowSettingChange(kind: .space, days: currentValue)
        case .webexSyncMinutes,
             .autoQueryAllMinutes,
             .trackedActionsRefreshMinutes,
             .personFocusRefreshMinutes,
             .personFocusAnalysisCadenceHours,
             .spaceFocusRefreshMinutes,
             .spaceFocusAnalysisCadenceHours,
             .transcriptionEnabled,
             .transcriptionDiarizationEnabled,
             .transcriptionLanguageMode,
             .transcriptionMicrophoneGain,
             .transcriptionAWSEndpoint,
             .pollSeconds,
             .debug,
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
             .codexModel,
             .codexReasoningLevel:
            break
        }
    }

    /// Reloads visible focus cache when its lookback window changes.
    private func reloadFocusCacheForWindowSettingChange(kind: FocusKind, days: Int) {
        do {
            switch kind {
            case .space:
                spaceCache = canonicalizeSpaceTitles(try runtimeStore.loadFocusCache(kind: .space))
                pruneDetailSectionExpansionState(for: .space)
                ensureSelection(for: .space)
                ensureBeliefTargetSelection(for: .space)
            case .person:
                personCache = try runtimeStore.loadFocusCache(kind: .person)
                pruneDetailSectionExpansionState(for: .person)
                ensureSelection(for: .person)
                ensureBeliefTargetSelection(for: .person)
            }
            runtimeStatus = runtimeStore.runtimeStatus()
            reloadFocusTargetManagementState()
            if askCodexTargetScope == (kind == .space ? .selectedSpace : .selectedPerson) || askCodexTargetScope == .allTracked {
                askCodexResult = nil
                askCodexLastError = nil
            }
            settingsLastMessage = "\(kind.title) lookback changed to \(days) days. Visible cache and Ask Codex context reloaded."
        } catch {
            settingsLastError = error.localizedDescription
        }
    }

    /// Starts or stops background refresh after the master switch changes.
    private func handleBackgroundSummariesSettingChange(currentValue: Bool) {
        if currentValue {
            settingsLastMessage = "Background summaries enabled. Starting background refresh."
            startBackgroundRefreshIfNeeded()
            Task { [weak self] in
                await self?.runRefreshCycle(
                    title: "Background refresh",
                    forceAll: false,
                    pipelinePlans: nil,
                    reloadAfterEachScope: false
                )
            }
        } else {
            settingsLastMessage = "Background summaries disabled. Startup and background refresh work stopped."
            stopBackgroundRefresh()
            interruptActiveRefreshForSettingsChange("Background summaries disabled in Settings.")
        }
    }

    /// Stops the periodic refresh loop.
    private func stopBackgroundRefresh() {
        refreshLoopTask?.cancel()
        refreshLoopTask = nil
        backgroundRefreshActive = false
    }

    /// Installs wake/network observers used to trigger Webex catch-up.
    private func installSystemEventObserversIfNeeded() {
        if wakeObserver == nil {
            wakeObserver = NSWorkspace.shared.notificationCenter.addObserver(
                forName: NSWorkspace.didWakeNotification,
                object: nil,
                queue: .main
            ) { [weak self] _ in
                guard let self else { return }
                Task { @MainActor [weak self] in
                    guard let self else { return }
                    await self.runWebexCatchUpForSystemEvent(title: "Wake catch-up")
                }
            }
        }

        if networkPathMonitor == nil {
            let monitor = NWPathMonitor()
            monitor.pathUpdateHandler = { [weak self] path in
                Task { @MainActor [weak self] in
                    guard let self else { return }
                    self.handleNetworkPathUpdate(isSatisfied: path.status == .satisfied)
                }
            }
            monitor.start(queue: networkMonitorQueue)
            networkPathMonitor = monitor
        }
    }

    /// Detects offline-to-online transitions.
    private func handleNetworkPathUpdate(isSatisfied: Bool) {
        let previous = lastNetworkPathSatisfied
        lastNetworkPathSatisfied = isSatisfied
        guard let previous else {
            return
        }
        guard previous == false, isSatisfied else {
            return
        }
        Task { @MainActor [weak self] in
            guard let self else { return }
            await self.runWebexCatchUpForSystemEvent(title: "Network reconnect catch-up")
        }
    }

    /// Runs a small Webex-only catch-up after wake or reconnect.
    private func runWebexCatchUpForSystemEvent(title: String) async {
        guard systemSettings.backgroundStatus, systemSettings.webexSyncEnabled else {
            return
        }
        let request = PagePriorityRefreshRequest(
            section: selectedSection,
            title: title,
            plans: [
                RefreshPipelinePlan(
                    id: "system-event-webex-sync",
                    scope: .webexSync,
                    title: "Webex sync",
                    mode: .incremental,
                    markCadence: true
                )
            ],
            reloadAfterEachScope: true
        )
        if refreshCycleRunning {
            if systemSettings.priorityRefreshPausesBackground {
                interruptActiveRefreshForPriority(request)
            } else {
                return
            }
        }
        _ = await runPriorityRefresh(request)
    }

    /// Starts or marks Webex sync after its enable switch changes.
    private func handleWebexSyncSettingChange(previousValue: Bool, currentValue: Bool) {
        guard previousValue != currentValue else {
            return
        }

        if currentValue {
            settingsLastMessage = "Webex sync enabled. Starting Webex refresh."
            Task { [weak self] in
                await self?.runWebexSyncAfterEnable()
            }
        } else {
            settingsLastMessage = "Webex sync disabled. Active Webex refresh work will stop."
            markWebexSyncDisabledInUI()
        }
    }

    /// Runs Webex sync immediately after enabling the feature when possible.
    private func runWebexSyncAfterEnable() async {
        guard systemSettings.webexSyncEnabled else {
            return
        }
        let request = PagePriorityRefreshRequest(
            section: .settings,
            title: "Refresh Webex Sync",
            plans: [
                RefreshPipelinePlan(
                    id: "settings-webex-sync-enabled",
                    scope: .webexSync,
                    title: "Webex sync",
                    mode: .incremental,
                    markCadence: true
                )
            ],
            reloadAfterEachScope: true
        )

        if refreshCycleRunning {
            if systemSettings.priorityRefreshPausesBackground {
                interruptActiveRefreshForPriority(request)
            } else {
                settingsLastMessage = "Webex sync enabled. Refresh is running; sync will run on the next eligible cycle."
                return
            }
        }

        let completed = await runPriorityRefresh(request)
        if completed {
            settingsLastMessage = refreshStatuses[.webexSync]?.lastSummary ?? "Webex sync refresh completed."
        }
    }

    /// Clears visible Webex sync work when the feature is disabled.
    private func markWebexSyncDisabledInUI() {
        let summary = "Webex sync disabled in Settings."
        updateRefreshStatus(.webexSync) { status in
            status.isRunning = false
            status.lastCompletedAt = Self.iso8601Timestamp(Date())
            status.lastSummary = summary
            status.lastError = nil
        }

        var next = refreshProgress
        guard next.isActive else {
            return
        }

        var didUpdate = false
        next.steps = next.steps.map { step in
            guard step.scope == .webexSync,
                  step.state == .pending || step.state == .running else {
                return step
            }
            var updated = step
            updated.state = .skipped
            updated.summary = summary
            updated.error = nil
            didUpdate = true
            return updated
        }

        if next.currentScope == .webexSync {
            let nextStep = next.steps.first { $0.state == .pending || $0.state == .running }
            next.currentStepID = nextStep?.id
            next.currentScope = nextStep?.scope
        }

        if didUpdate {
            next.lastSummary = summary
            refreshProgress = next
            persistActiveRefreshCheckpoint(cycleID: activeRefreshCycleID)
        }
    }

    /// Runs a settings-triggered focus rebuild as a one-step refresh cycle.
    private func runSettingsFocusRebuildAction(
        title: String,
        stepID: String,
        scope: RefreshScope,
        mode: RefreshExecutionMode = .full
    ) async {
        _ = await runRefreshCycle(
            title: title,
            forceAll: false,
            pipelinePlans: [
                RefreshPipelinePlan(
                    id: stepID,
                    scope: scope,
                    title: scope.title,
                    mode: mode,
                    markCadence: true
                )
            ],
            reloadAfterEachScope: false,
            taskPriority: .userInitiated
        )
        settingsLastMessage = refreshStatuses[scope]?.lastSummary ?? "\(scope.title) rebuild completed."
    }

    /// Runs a settings-triggered Webex map refresh plus sync.
    private func runSettingsWebexSyncAction() async {
        guard systemSettings.webexSyncEnabled else {
            let summary = "Webex sync skipped: disabled in Settings."
            updateRefreshStatus(.webexSync) { status in
                status.isRunning = false
                status.lastCompletedAt = Self.iso8601Timestamp(Date())
                status.lastSummary = summary
                status.lastError = nil
            }
            settingsLastMessage = summary
            return
        }

        let cycleID = UUID()
        refreshCycleRunning = true
        activeRefreshCycleID = cycleID
        defer {
            if activeRefreshCycleID == cycleID {
                refreshCycleRunning = false
                activeRefreshCycleID = nil
            }
            interruptedRefreshCycleIDs.remove(cycleID)
        }

        let stepID = SystemSettingsAction.syncWebex.rawValue
        beginRefreshProgress(
            title: "Sync Webex",
            plans: [
                RefreshPipelinePlan(
                    id: stepID,
                    scope: .webexSync,
                    title: "Webex map + snapshots",
                    mode: .full,
                    markCadence: true
                )
            ]
        )
        defer {
            finishRefreshProgress(cycleID: cycleID)
        }

        markRefreshStepRunning(stepID, cycleID: cycleID)
        let startedAt = Self.iso8601Timestamp(Date())
        updateRefreshStatus(.webexSync) { status in
            status.isRunning = true
            status.lastStartedAt = startedAt
            status.lastError = nil
        }

        do {
            let coordinator = refreshCoordinator
            let model = self
            let outcome = try await Task.detached(priority: .utility) {
                let mapOutcome = try await coordinator.refreshWebexMapFile()
                await MainActor.run {
                    model.markRefreshStepProgress(id: stepID, message: mapOutcome.summary, cycleID: cycleID)
                }
                let syncOutcome = try await coordinator.refresh(.webexSync) { progress in
                    await MainActor.run {
                        model.markRefreshStepProgress(id: stepID, message: progress.message, cycleID: cycleID)
                    }
                }
                return (mapOutcome, syncOutcome)
            }.value

            let wasInterrupted = interruptedRefreshCycleIDs.contains(cycleID)
            if !wasInterrupted {
                lastRefreshDateByScope[.webexSync] = Date()
            }
            let summary = "\(outcome.0.summary) \(outcome.1.summary)"
            updateRefreshStatus(.webexSync) { status in
                status.isRunning = false
                status.lastCompletedAt = outcome.1.completedAt
                status.lastSummary = summary
                status.lastError = nil
            }
            if !wasInterrupted {
                markRefreshStepFinished(
                    id: stepID,
                    result: RefreshScopeRunResult(
                        succeeded: true,
                        summary: summary,
                        error: nil,
                        reusedCache: nil
                    ),
                    cycleID: cycleID
                )
                settingsLastMessage = summary
                await reloadRuntimeStateAfterRefresh()
            }
        } catch {
            let wasInterrupted = interruptedRefreshCycleIDs.contains(cycleID)
            updateRefreshStatus(.webexSync) { status in
                status.isRunning = false
                status.lastCompletedAt = Self.iso8601Timestamp(Date())
                status.lastError = error.localizedDescription
            }
            if !wasInterrupted {
                markRefreshStepFinished(
                    id: stepID,
                    result: RefreshScopeRunResult(
                        succeeded: false,
                        summary: nil,
                        error: error.localizedDescription,
                        reusedCache: nil
                    ),
                    cycleID: cycleID
                )
                settingsLastError = error.localizedDescription
                handleRuntimeAccessError(error)
            }
        }
    }

    private func settingTitle(_ key: SystemSettingKey) -> String {
        switch key {
        case .debug:
            return "Debug output"
        case .backgroundStatus:
            return "Background summaries"
        case .webexSyncEnabled:
            return "Webex sync"
        case .autoQueryAllEnabled:
            return "Auto-query-all"
        case .priorityRefreshPausesBackground:
            return "Page reload pause"
        case .codexEnabled:
            return "Codex"
        case .codexAskEnabled:
            return "Ask Codex"
        case .codexQuestionSynthesisEnabled:
            return "Question synthesis"
        case .codexPersonSummariesEnabled:
            return "Person Focus summaries"
        case .codexSpaceSummariesEnabled:
            return "Space Focus summaries"
        case .codexClusterTitlesEnabled:
            return "Cluster titles"
        case .codexExecQuestionsEnabled:
            return "Exec questions"
        case .codexBeliefsEnabled:
            return "Belief analysis"
        case .codexModel:
            return "Codex model"
        case .codexReasoningLevel:
            return "Codex reasoning"
        case .webexSyncMinutes:
            return "Webex sync interval"
        case .autoQueryAllMinutes:
            return "Auto-query-all interval"
        case .trackedActionsRefreshMinutes:
            return "Tracked actions refresh"
        case .personFocusRefreshMinutes:
            return "Person Focus refresh"
        case .personFocusDays:
            return "Person Focus days"
        case .personFocusAnalysisCadenceHours:
            return "Person analysis cadence"
        case .spaceFocusRefreshMinutes:
            return "Space Focus refresh"
        case .spaceFocusDays:
            return "Space Focus days"
        case .spaceFocusAnalysisCadenceHours:
            return "Space analysis cadence"
        case .transcriptionEnabled:
            return "Live transcription"
        case .transcriptionDiarizationEnabled:
            return "Speaker diarization"
        case .transcriptionLanguageMode:
            return "Transcription mode"
        case .transcriptionMicrophoneGain:
            return "Microphone gain"
        case .transcriptionAWSEndpoint:
            return "AWS transcription endpoint"
        case .pollSeconds:
            return "Poll seconds"
        }
    }

    /// Startup pipeline ordered to get local evidence visible before Codex enrichment.
    private func startupRefreshPipelinePlans() -> [RefreshPipelinePlan] {
        [
            RefreshPipelinePlan(id: "startup-webex-sync", scope: .webexSync, title: "Webex sync", mode: .incremental, markCadence: true),
            RefreshPipelinePlan(id: "startup-person-local", scope: .personFocus, title: "Person clusters", mode: .cacheAwareLocalOnly, markCadence: false),
            RefreshPipelinePlan(id: "startup-space-local", scope: .spaceFocus, title: "Space conversations", mode: .cacheAwareLocalOnly, markCadence: false),
            RefreshPipelinePlan(id: "startup-questions-local", scope: .questions, title: "Questions from fresh evidence", mode: .full, markCadence: false),
            RefreshPipelinePlan(id: "startup-person-codex", scope: .personFocus, title: "Person summaries", mode: .codexOnly, markCadence: true),
            RefreshPipelinePlan(id: "startup-space-codex", scope: .spaceFocus, title: "Space summaries and exec questions", mode: .codexOnly, markCadence: true),
            RefreshPipelinePlan(id: "startup-questions-final", scope: .questions, title: "Questions from enriched summaries", mode: .full, markCadence: true),
            RefreshPipelinePlan(id: "startup-codex-jobs", scope: .codexJobs, title: "Codex job status", mode: .full, markCadence: true)
        ]
    }

    /// Drops disabled feature steps from a requested refresh pipeline.
    private func enabledRefreshPipelinePlans(_ plans: [RefreshPipelinePlan]) -> [RefreshPipelinePlan] {
        plans.filter(isRefreshPipelinePlanEnabled)
    }

    /// Applies feature-flag gating to one refresh pipeline step.
    private func isRefreshPipelinePlanEnabled(_ plan: RefreshPipelinePlan) -> Bool {
        if plan.scope == .webexSync {
            return systemSettings.webexSyncEnabled
        }

        switch (plan.scope, plan.mode) {
        case (.personFocus, .codexOnly):
            return systemSettings.codexFeatureEnabled(.personFocusSummaries)
                || systemSettings.codexFeatureEnabled(.clusterTitles)
        case (.spaceFocus, .codexOnly):
            return systemSettings.codexFeatureEnabled(.spaceFocusSummaries)
                || systemSettings.codexFeatureEnabled(.clusterTitles)
                || systemSettings.codexFeatureEnabled(.execQuestions)
        case (.codexJobs, _):
            return systemSettings.codexEnabled
        case (.beliefMaintenance, _):
            return systemSettings.codexFeatureEnabled(.beliefs)
        default:
            return true
        }
    }

    /// Test hook for the startup plan after settings gates are applied.
    func visibleStartupRefreshPlanTitlesForTesting() -> [String] {
        guard systemSettings.backgroundStatus else {
            return []
        }
        return enabledRefreshPipelinePlans(startupRefreshPipelinePlans()).map(\.title)
    }

    /// Test hook for page-priority plans after settings gates are applied.
    func visiblePriorityRefreshPlanTitlesForTesting(section: AppSection) -> [String] {
        let request = priorityRefreshRequest(for: section)
        return enabledRefreshPipelinePlans(request.plans).map(\.title)
    }

    /// Builds the smallest useful refresh plan for one visible page.
    private func priorityRefreshRequest(for section: AppSection) -> PagePriorityRefreshRequest {
        switch section {
        case .home:
            return PagePriorityRefreshRequest(
                section: section,
                title: "Refresh Home",
                plans: [
                    RefreshPipelinePlan(id: "priority-home-webex", scope: .webexSync, title: "Webex sync", mode: .incremental, markCadence: true),
                    RefreshPipelinePlan(id: "priority-home-person", scope: .personFocus, title: "Person Focus", mode: .cacheAwareFull, markCadence: true),
                    RefreshPipelinePlan(id: "priority-home-space", scope: .spaceFocus, title: "Space Focus", mode: .cacheAwareFull, markCadence: true),
                    RefreshPipelinePlan(id: "priority-home-questions", scope: .questions, title: "Questions", mode: .full, markCadence: true),
                    RefreshPipelinePlan(id: "priority-home-codex-jobs", scope: .codexJobs, title: "Codex jobs", mode: .full, markCadence: true)
                ],
                reloadAfterEachScope: true
            )
        case .spaceFocus:
            return PagePriorityRefreshRequest(
                section: section,
                title: "Refresh Space Focus",
                plans: [
                    RefreshPipelinePlan(id: "priority-space-webex", scope: .webexSync, title: "Webex sync", mode: .incremental, markCadence: true),
                    RefreshPipelinePlan(id: "priority-space-focus", scope: .spaceFocus, title: "Space Focus summaries", mode: .full, markCadence: true),
                    RefreshPipelinePlan(id: "priority-space-questions", scope: .questions, title: "Space Focus questions", mode: .full, markCadence: true)
                ],
                reloadAfterEachScope: true
            )
        case .personFocus:
            return PagePriorityRefreshRequest(
                section: section,
                title: "Refresh Person Focus",
                plans: [
                    RefreshPipelinePlan(id: "priority-person-webex", scope: .webexSync, title: "Webex sync", mode: .incremental, markCadence: true),
                    RefreshPipelinePlan(id: "priority-person-focus", scope: .personFocus, title: "Person Focus summaries", mode: .full, markCadence: true),
                    RefreshPipelinePlan(id: "priority-person-questions", scope: .questions, title: "Person Focus questions", mode: .full, markCadence: true)
                ],
                reloadAfterEachScope: true
            )
        case .spaceFocusTargets:
            return PagePriorityRefreshRequest(
                section: section,
                title: "Refresh Space Focus Targets",
                plans: [
                    RefreshPipelinePlan(id: "priority-space-targets-webex", scope: .webexSync, title: "Webex map + snapshots", mode: .incremental, markCadence: true)
                ],
                reloadAfterEachScope: true
            )
        case .personFocusTargets:
            return PagePriorityRefreshRequest(
                section: section,
                title: "Refresh Person Focus Targets",
                plans: [
                    RefreshPipelinePlan(id: "priority-person-targets-webex", scope: .webexSync, title: "Webex map + snapshots", mode: .incremental, markCadence: true)
                ],
                reloadAfterEachScope: true
            )
        case .execFocusTargets:
            return PagePriorityRefreshRequest(
                section: section,
                title: "Refresh Exec Focus Targets",
                plans: [
                    RefreshPipelinePlan(id: "priority-exec-targets-webex", scope: .webexSync, title: "Webex map + snapshots", mode: .incremental, markCadence: true)
                ],
                reloadAfterEachScope: true
            )
        case .questions:
            return PagePriorityRefreshRequest(
                section: section,
                title: "Refresh Questions",
                plans: [
                    RefreshPipelinePlan(id: "priority-questions", scope: .questions, title: "Questions", mode: .full, markCadence: true)
                ],
                reloadAfterEachScope: true
            )
        case .transcription:
            return PagePriorityRefreshRequest(
                section: section,
                title: "Refresh Transcription",
                plans: [],
                reloadAfterEachScope: false
            )
        case .beliefs:
            return PagePriorityRefreshRequest(
                section: section,
                title: "Refresh Beliefs",
                plans: [
                    RefreshPipelinePlan(id: "priority-beliefs", scope: .beliefMaintenance, title: "Belief maintenance", mode: .full, markCadence: true)
                ],
                reloadAfterEachScope: true
            )
        case .askCodex:
            return PagePriorityRefreshRequest(
                section: section,
                title: "Refresh Ask Codex",
                plans: [
                    RefreshPipelinePlan(id: "priority-ask-codex-jobs", scope: .codexJobs, title: "Codex jobs", mode: .full, markCadence: true)
                ],
                reloadAfterEachScope: true
            )
        case .jobs:
            return PagePriorityRefreshRequest(
                section: section,
                title: "Refresh Jobs",
                plans: [
                    RefreshPipelinePlan(id: "priority-jobs", scope: .codexJobs, title: "Codex jobs", mode: .full, markCadence: true)
                ],
                reloadAfterEachScope: true
            )
        case .settings:
            return PagePriorityRefreshRequest(
                section: section,
                title: "Refresh Settings",
                plans: [],
                reloadAfterEachScope: false
            )
        }
    }

    @discardableResult
    /// Runs a page-priority refresh and follows with page-specific reload work.
    private func runPriorityRefresh(_ request: PagePriorityRefreshRequest) async -> Bool {
        var activeRequest = request
        activeRequest.plans = enabledRefreshPipelinePlans(request.plans)
        if activeRequest.plans.isEmpty {
            beginRefreshProgress(title: activeRequest.title, plans: [])
            defer {
                finishRefreshProgress()
            }
            await runPostPriorityRefresh(for: activeRequest.section)
            return true
        }

        let completed = await runRefreshCycle(
            title: activeRequest.title,
            forceAll: false,
            pipelinePlans: activeRequest.plans,
            reloadAfterEachScope: activeRequest.reloadAfterEachScope,
            taskPriority: .userInitiated,
            isPriorityRefresh: true
        )
        if completed {
            await runPostPriorityRefresh(for: activeRequest.section)
        }
        return completed
    }

    /// Performs non-pipeline reload work after a priority refresh finishes.
    private func runPostPriorityRefresh(for section: AppSection) async {
        switch section {
        case .settings:
            reloadSystemSettings()
            runtimeStatus = runtimeStore.runtimeStatus()
            settingsLastMessage = "Settings reloaded."
        case .spaceFocusTargets, .personFocusTargets, .execFocusTargets:
            reloadFocusTargetManagement()
        case .beliefs:
            await refreshBeliefs(force: true)
            await refreshBeliefSetSummaries(force: true)
        case .questions:
            await loadQuestions()
        default:
            break
        }
    }

    /// Runs a guarded refresh pipeline with optional checkpoint persistence.
    private func runRefreshCycle(
        title: String,
        forceAll: Bool,
        pipelinePlans requestedPipelinePlans: [RefreshPipelinePlan]?,
        reloadAfterEachScope: Bool,
        taskPriority: TaskPriority = .utility,
        isPriorityRefresh: Bool = false
    ) async -> Bool {
        guard systemSettings.backgroundStatus || isPriorityRefresh || taskPriority == .userInitiated else {
            return true
        }
        guard !refreshCycleRunning else {
            return false
        }
        let cycleID = UUID()
        refreshCycleRunning = true
        activeRefreshCycleID = cycleID
        defer {
            if activeRefreshCycleID == cycleID {
                refreshCycleRunning = false
                activeRefreshCycleID = nil
            }
            interruptedRefreshCycleIDs.remove(cycleID)
        }

        let candidatePlans = requestedPipelinePlans ?? refreshPlans.compactMap { plan in
            guard forceAll || isRefreshDue(plan) else { return nil }
            return RefreshPipelinePlan(
                id: plan.scope.rawValue,
                scope: plan.scope,
                title: plan.scope.title,
                mode: refreshExecutionMode(scope: plan.scope, force: forceAll),
                markCadence: true
            )
        }
        let plansToRun = enabledRefreshPipelinePlans(candidatePlans)
        guard !plansToRun.isEmpty else {
            return true
        }

        let refreshCheckpointFingerprint = currentRefreshCheckpointFingerprint()
        activateRefreshCheckpointContext(
            cycleID: cycleID,
            title: title,
            plans: plansToRun,
            reloadAfterEachScope: reloadAfterEachScope,
            isPriorityRefresh: isPriorityRefresh,
            stateFingerprint: refreshCheckpointFingerprint
        )
        beginRefreshProgress(title: title, plans: plansToRun)
        defer {
            let wasInterrupted = interruptedRefreshCycleIDs.contains(cycleID)
            let hasFailedSteps = refreshProgress.steps.contains(where: { $0.state == .failed })
            if wasInterrupted || hasFailedSteps {
                persistActiveRefreshCheckpoint(cycleID: cycleID)
            } else {
                clearPersistedRefreshCheckpoint()
            }
            finishRefreshProgress(cycleID: cycleID)
            resetRefreshCheckpointContext(cycleID: cycleID)
        }
        persistActiveRefreshCheckpoint(cycleID: cycleID)

        var latestCacheReuseByScope: [RefreshScope: Bool] = [:]
        var wasInterrupted = false
        for (index, plan) in plansToRun.enumerated() {
            guard isRefreshPipelinePlanEnabled(plan) else {
                markRefreshStepSkipped(
                    id: plan.id,
                    summary: "\(plan.title) skipped because its Settings flag is disabled.",
                    cycleID: cycleID
                )
                continue
            }
            if let scope = plan.skipWhenPreviousScopeReused,
               latestCacheReuseByScope[scope] == true {
                markRefreshStepSkipped(
                    id: plan.id,
                    summary: "\(plan.title) skipped because \(scope.title) source evidence is unchanged.",
                    cycleID: cycleID
                )
                if plan.markCadence {
                    lastRefreshDateByScope[plan.scope] = Date()
                }
                continue
            }
            markRefreshStepRunning(plan.id, cycleID: cycleID)
            let result = await runRefreshScope(
                plan.scope,
                mode: plan.mode,
                markCadence: plan.markCadence,
                progressStepID: plan.id,
                reloadAfterCompletion: reloadAfterEachScope,
                taskPriority: taskPriority,
                cycleID: cycleID
            )
            markRefreshStepFinished(id: plan.id, result: result, cycleID: cycleID)
            if let reusedCache = result.reusedCache {
                latestCacheReuseByScope[plan.scope] = reusedCache
            }
            if interruptedRefreshCycleIDs.contains(cycleID) {
                wasInterrupted = true
                markRemainingRefreshStepsSkipped(
                    after: index,
                    in: plansToRun,
                    summary: "Paused for a newer page refresh.",
                    cycleID: cycleID
                )
                break
            }
        }
        guard !wasInterrupted else {
            return false
        }
        await reloadRuntimeStateAfterRefresh()
        if selectedSection == .beliefs {
            await refreshBeliefs()
            await refreshBeliefSetSummaries()
        }
        if selectedSection == .questions {
            await loadQuestions()
        }
        return true
    }

    /// Resumes a persisted refresh checkpoint when settings/targets still match.
    private func resumeRefreshCheckpointIfNeeded() async -> Bool {
        guard let data = configStore.loadRefreshCheckpointData() else {
            return false
        }

        let decoder = JSONDecoder()
        guard let checkpoint = try? decoder.decode(PersistedRefreshCheckpoint.self, from: data) else {
            clearPersistedRefreshCheckpoint()
            return false
        }

        guard checkpoint.version == 1 else {
            clearPersistedRefreshCheckpoint()
            return false
        }

        guard let updatedAt = parsedTimestamp(checkpoint.updatedAt),
              Date().timeIntervalSince(updatedAt) <= Self.refreshCheckpointStaleInterval else {
            clearPersistedRefreshCheckpoint()
            return false
        }

        let currentFingerprint = currentRefreshCheckpointFingerprint()
        guard checkpoint.stateFingerprint == currentFingerprint else {
            clearPersistedRefreshCheckpoint()
            return false
        }

        var remainingPlans: [RefreshPipelinePlan] = []
        var seenStepIDs = Set<String>()
        for step in checkpoint.steps {
            let state = RefreshProgressStepState(rawValue: step.stateRawValue) ?? .pending
            if state == .succeeded || state == .skipped {
                continue
            }
            guard seenStepIDs.insert(step.id).inserted,
                  let scope = RefreshScope(rawValue: step.scopeRawValue),
                  let mode = RefreshExecutionMode(rawValue: step.modeRawValue) else {
                continue
            }
            let skipScope = step.skipWhenPreviousScopeReusedRawValue.flatMap(RefreshScope.init(rawValue:))
            remainingPlans.append(
                RefreshPipelinePlan(
                    id: step.id,
                    scope: scope,
                    title: step.title,
                    mode: mode,
                    markCadence: step.markCadence,
                    skipWhenPreviousScopeReused: skipScope
                )
            )
        }

        remainingPlans = enabledRefreshPipelinePlans(remainingPlans)
        guard !remainingPlans.isEmpty else {
            clearPersistedRefreshCheckpoint()
            return false
        }

        _ = await runRefreshCycle(
            title: "Resume \(checkpoint.title)",
            forceAll: false,
            pipelinePlans: remainingPlans,
            reloadAfterEachScope: checkpoint.reloadAfterEachScope,
            taskPriority: .utility,
            isPriorityRefresh: true
        )
        return true
    }

    /// Captures metadata needed to persist the active refresh pipeline.
    private func activateRefreshCheckpointContext(
        cycleID: UUID,
        title: String,
        plans: [RefreshPipelinePlan],
        reloadAfterEachScope: Bool,
        isPriorityRefresh: Bool,
        stateFingerprint: String
    ) {
        activeRefreshCheckpointRunID = cycleID
        activeRefreshCheckpointTitle = title
        activeRefreshCheckpointFingerprint = stateFingerprint
        activeRefreshCheckpointReloadAfterEachScope = reloadAfterEachScope
        activeRefreshCheckpointIsPriority = isPriorityRefresh
        activeRefreshPipelinePlansByID = Dictionary(uniqueKeysWithValues: plans.map { ($0.id, $0) })
    }

    /// Clears active checkpoint metadata for the finishing refresh cycle.
    private func resetRefreshCheckpointContext(cycleID: UUID? = nil) {
        if let cycleID, activeRefreshCheckpointRunID != cycleID {
            return
        }
        activeRefreshCheckpointRunID = nil
        activeRefreshCheckpointTitle = ""
        activeRefreshCheckpointFingerprint = ""
        activeRefreshCheckpointReloadAfterEachScope = false
        activeRefreshCheckpointIsPriority = false
        activeRefreshPipelinePlansByID = [:]
    }

    /// Writes the current refresh progress to disk for crash/restart recovery.
    private func persistActiveRefreshCheckpoint(cycleID: UUID?) {
        guard let runID = activeRefreshCheckpointRunID,
              refreshProgress.totalStepCount > 0 else {
            return
        }
        if let cycleID, runID != cycleID {
            return
        }

        let startedAt = refreshProgress.startedAt ?? Date()
        let updatedAt = Date()
        let steps = refreshProgress.steps.compactMap { step -> PersistedRefreshCheckpointStep? in
            let plan = activeRefreshPipelinePlansByID[step.id] ?? RefreshPipelinePlan(
                id: step.id,
                scope: step.scope,
                title: step.title,
                mode: .full,
                markCadence: false
            )
            return PersistedRefreshCheckpointStep(
                id: step.id,
                scopeRawValue: plan.scope.rawValue,
                title: plan.title,
                modeRawValue: plan.mode.rawValue,
                markCadence: plan.markCadence,
                skipWhenPreviousScopeReusedRawValue: plan.skipWhenPreviousScopeReused?.rawValue,
                stateRawValue: step.state.rawValue,
                summary: step.summary,
                error: step.error
            )
        }

        let checkpoint = PersistedRefreshCheckpoint(
            version: 1,
            runID: runID.uuidString,
            title: activeRefreshCheckpointTitle.isEmpty ? refreshProgress.title : activeRefreshCheckpointTitle,
            startedAt: Self.iso8601Timestamp(startedAt),
            updatedAt: Self.iso8601Timestamp(updatedAt),
            stateFingerprint: activeRefreshCheckpointFingerprint,
            reloadAfterEachScope: activeRefreshCheckpointReloadAfterEachScope,
            isPriorityRefresh: activeRefreshCheckpointIsPriority,
            steps: steps
        )

        do {
            let encoder = JSONEncoder()
            encoder.outputFormatting = [.prettyPrinted, .sortedKeys]
            let data = try encoder.encode(checkpoint)
            try configStore.saveRefreshCheckpointData(data)
        } catch {
            settingsLastError = error.localizedDescription
        }
    }

    /// Deletes any persisted refresh resume checkpoint.
    private func clearPersistedRefreshCheckpoint() {
        do {
            try configStore.clearRefreshCheckpoint()
        } catch {
            settingsLastError = error.localizedDescription
        }
    }

    /// Hashes settings and target files that make a checkpoint safe to resume.
    private func currentRefreshCheckpointFingerprint() -> String {
        var lines: [String] = [
            "webex_sync_enabled=\(systemSettings.webexSyncEnabled)",
            "webex_sync_minutes=\(systemSettings.webexSyncMinutes)",
            "person_focus_days=\(systemSettings.personFocusDays)",
            "space_focus_days=\(systemSettings.spaceFocusDays)",
            "person_focus_analysis_cadence_hours=\(systemSettings.personFocusAnalysisCadenceHours)",
            "space_focus_analysis_cadence_hours=\(systemSettings.spaceFocusAnalysisCadenceHours)",
            "person_focus_refresh_minutes=\(systemSettings.personFocusRefreshMinutes)",
            "space_focus_refresh_minutes=\(systemSettings.spaceFocusRefreshMinutes)",
            "codex_enabled=\(systemSettings.codexEnabled)"
        ]

        do {
            let targets = try configStore.importantTargets()
            let normalizedTargets = targets.map { target in
                let roomID = normalizedRoomID(target.roomID)
                let email = target.email.trimmingCharacters(in: .whitespacesAndNewlines).lowercased()
                return "\(target.kind.rawValue)|\(roomID)|\(email)"
            }.sorted()
            lines.append(contentsOf: normalizedTargets.map { "target=\($0)" })
        } catch {
            lines.append("targets_error=\(error.localizedDescription)")
        }

        do {
            let beliefTargets = try configStore.beliefTargets()
            let normalizedBeliefs = beliefTargets.map { target in
                let roomID = normalizedRoomID(target.roomID)
                let email = target.email.trimmingCharacters(in: .whitespacesAndNewlines).lowercased()
                return "\(target.kind.rawValue)|\(roomID)|\(email)"
            }.sorted()
            lines.append(contentsOf: normalizedBeliefs.map { "belief=\($0)" })
        } catch {
            lines.append("belief_targets_error=\(error.localizedDescription)")
        }

        let data = Data(lines.joined(separator: "\n").utf8)
        let digest = SHA256.hash(data: data)
        return digest.map { String(format: "%02x", $0) }.joined()
    }

    /// Checks cadence for a scheduled refresh scope.
    private func isRefreshDue(_ plan: RefreshPlan) -> Bool {
        guard let lastRun = lastRefreshDateByScope[plan.scope] else {
            return true
        }
        return Date().timeIntervalSince(lastRun) >= TimeInterval(plan.cadenceSeconds)
    }

    /// Chooses incremental/cache-aware/full mode for one scope.
    private func refreshExecutionMode(scope: RefreshScope, force: Bool) -> RefreshExecutionMode {
        guard !force else {
            return .full
        }
        switch scope {
        case .personFocus, .spaceFocus:
            return .cacheAwareFull
        case .webexSync:
            return .incremental
        case .beliefMaintenance, .questions, .codexJobs:
            return .full
        }
    }

    /// Internal result shape shared by progress and status updates.
    private struct RefreshScopeRunResult {
        var succeeded: Bool
        var summary: String?
        var error: String?
        var reusedCache: Bool?
    }

    /// Executes one refresh scope and updates status/progress surfaces.
    private func runRefreshScope(
        _ scope: RefreshScope,
        mode: RefreshExecutionMode,
        markCadence: Bool,
        progressStepID: String,
        reloadAfterCompletion: Bool,
        taskPriority: TaskPriority,
        cycleID: UUID
    ) async -> RefreshScopeRunResult {
        let startedAt = Self.iso8601Timestamp(Date())
        updateRefreshStatus(scope) { status in
            status.isRunning = true
            status.lastStartedAt = startedAt
            status.lastError = nil
        }

        do {
            let coordinator = refreshCoordinator
            let model = self
            let result = try await Task.detached(priority: taskPriority) {
                try await coordinator.refresh(scope, mode: mode) { progress in
                    await MainActor.run {
                        model.markRefreshStepProgress(id: progressStepID, message: progress.message, cycleID: cycleID)
                    }
                }
            }.value
            guard isCurrentRefreshCycle(cycleID),
                  !interruptedRefreshCycleIDs.contains(cycleID) else {
                return RefreshScopeRunResult(
                    succeeded: false,
                    summary: "Refresh interrupted by a newer Settings change.",
                    error: nil,
                    reusedCache: result.reusedCache
                )
            }
            if markCadence {
                lastRefreshDateByScope[scope] = Date()
            }
            updateRefreshStatus(scope) { status in
                status.isRunning = false
                status.lastCompletedAt = result.completedAt
                status.lastSummary = result.summary
                status.lastError = nil
            }
            if scope == .beliefMaintenance {
                invalidateBeliefCaches()
            }
            if reloadAfterCompletion,
               scope == .webexSync || scope == .personFocus || scope == .spaceFocus || scope == .questions {
                await reloadRuntimeStateAfterRefresh()
            }
            return RefreshScopeRunResult(succeeded: true, summary: result.summary, error: nil, reusedCache: result.reusedCache)
        } catch {
            guard isCurrentRefreshCycle(cycleID),
                  !interruptedRefreshCycleIDs.contains(cycleID) else {
                return RefreshScopeRunResult(
                    succeeded: false,
                    summary: "Refresh interrupted by a newer Settings change.",
                    error: nil,
                    reusedCache: nil
                )
            }
            updateRefreshStatus(scope) { status in
                status.isRunning = false
                status.lastCompletedAt = Self.iso8601Timestamp(Date())
                status.lastError = error.localizedDescription
            }
            handleRuntimeAccessError(error)
            return RefreshScopeRunResult(succeeded: false, summary: nil, error: error.localizedDescription, reusedCache: nil)
        }
    }

    /// Mutates one refresh status entry in published state.
    private func updateRefreshStatus(_ scope: RefreshScope, mutate: (inout RefreshRunStatus) -> Void) {
        var next = refreshStatuses
        var status = next[scope] ?? RefreshRunStatus(scope: scope, isRunning: false)
        mutate(&status)
        next[scope] = status
        refreshStatuses = next
    }

    /// Starts a new visible progress pipeline.
    private func beginRefreshProgress(title: String, plans: [RefreshPipelinePlan]) {
        refreshProgress = RefreshProgressState(
            isActive: true,
            title: title,
            startedAt: Date(),
            completedAt: nil,
            steps: plans.map {
                RefreshProgressStep(
                    id: $0.id,
                    scope: $0.scope,
                    title: $0.title,
                    usesCodex: $0.usesCodex,
                    state: .pending,
                    summary: nil,
                    error: nil
                )
            },
            currentStepID: plans.first?.id,
            currentScope: plans.first?.scope,
            lastSummary: nil
        )
    }

    /// Guards async callbacks from stale refresh cycles.
    private func isCurrentRefreshCycle(_ cycleID: UUID?) -> Bool {
        guard let cycleID else {
            return true
        }
        return activeRefreshCycleID == cycleID
    }

    /// Marks a pipeline step as running.
    private func markRefreshStepRunning(_ id: String, cycleID: UUID? = nil) {
        guard isCurrentRefreshCycle(cycleID) else {
            return
        }
        var next = refreshProgress
        guard next.steps.contains(where: { $0.id == id }) else {
            return
        }
        next.currentStepID = id
        next.currentScope = next.steps.first { $0.id == id }?.scope
        next.steps = next.steps.map { step in
            guard step.id == id else { return step }
            var updated = step
            updated.state = .running
            updated.error = nil
            return updated
        }
        refreshProgress = next
        persistActiveRefreshCheckpoint(cycleID: cycleID)
    }

    /// Updates the latest message for a running pipeline step.
    private func markRefreshStepProgress(id: String, message: String, cycleID: UUID? = nil) {
        guard isCurrentRefreshCycle(cycleID) else {
            return
        }
        var next = refreshProgress
        var didUpdate = false
        next.steps = next.steps.map { step in
            guard step.id == id else { return step }
            var updated = step
            updated.summary = message
            didUpdate = true
            return updated
        }
        guard didUpdate else {
            return
        }
        next.lastSummary = message
        refreshProgress = next
        persistActiveRefreshCheckpoint(cycleID: cycleID)
    }

    /// Interrupts lower-priority refresh work for a page-triggered refresh.
    private func interruptActiveRefreshForPriority(_ request: PagePriorityRefreshRequest) {
        if let cycleID = activeRefreshCycleID {
            interruptedRefreshCycleIDs.insert(cycleID)
        }
        activeRefreshCycleID = nil
        refreshCycleRunning = false

        var next = refreshProgress
        let message = "Prioritizing \(request.title); pausing lower-priority refresh work."
        var didUpdate = false
        next.steps = next.steps.map { step in
            var updated = step
            switch step.state {
            case .pending:
                updated.state = .skipped
                updated.summary = message
                updated.error = nil
                didUpdate = true
            case .running:
                updated.summary = message
                didUpdate = true
            case .succeeded, .failed, .skipped:
                break
            }
            return updated
        }
        if didUpdate {
            next.lastSummary = message
            refreshProgress = next
            persistActiveRefreshCheckpoint(cycleID: activeRefreshCycleID)
        }
    }

    /// Stops active refresh UI/status after a setting invalidates the run.
    private func interruptActiveRefreshForSettingsChange(_ message: String) {
        if let cycleID = activeRefreshCycleID {
            interruptedRefreshCycleIDs.insert(cycleID)
        }
        activeRefreshCycleID = nil
        refreshCycleRunning = false

        let completedAt = Self.iso8601Timestamp(Date())
        for scope in RefreshScope.allCases where refreshStatuses[scope]?.isRunning == true {
            updateRefreshStatus(scope) { status in
                status.isRunning = false
                status.lastCompletedAt = completedAt
                status.lastSummary = message
                status.lastError = nil
            }
        }

        var next = refreshProgress
        guard next.isActive else {
            return
        }
        next.steps = next.steps.map { step in
            guard step.state == .pending || step.state == .running else {
                return step
            }
            var updated = step
            updated.state = .skipped
            updated.summary = message
            updated.error = nil
            return updated
        }
        next.isActive = false
        next.currentStepID = nil
        next.currentScope = nil
        next.completedAt = Date()
        next.lastSummary = message
        refreshProgress = next
        persistActiveRefreshCheckpoint(cycleID: activeRefreshCycleID)
    }

    /// Records that a page refresh was intentionally skipped while busy.
    private func markPriorityRefreshSkippedBecauseBusy(_ request: PagePriorityRefreshRequest) {
        let message = "\(request.title) skipped because another refresh is running and page reload pausing is disabled."
        var next = refreshProgress
        next.lastSummary = message
        refreshProgress = next
        persistActiveRefreshCheckpoint(cycleID: activeRefreshCycleID)
    }

    /// Marks one pipeline step as skipped with its visible summary.
    private func markRefreshStepSkipped(id: String, summary: String, cycleID: UUID? = nil) {
        guard isCurrentRefreshCycle(cycleID) else {
            return
        }
        var next = refreshProgress
        guard next.steps.contains(where: { $0.id == id }) else {
            return
        }
        next.currentStepID = id
        next.currentScope = next.steps.first { $0.id == id }?.scope
        next.steps = next.steps.map { step in
            guard step.id == id else { return step }
            var updated = step
            updated.state = .skipped
            updated.summary = summary
            updated.error = nil
            return updated
        }
        next.lastSummary = summary
        refreshProgress = next
        persistActiveRefreshCheckpoint(cycleID: cycleID)
    }

    /// Skips pending downstream steps after a refresh interruption.
    private func markRemainingRefreshStepsSkipped(
        after completedPlanIndex: Int,
        in plans: [RefreshPipelinePlan],
        summary: String,
        cycleID: UUID? = nil
    ) {
        guard isCurrentRefreshCycle(cycleID) else {
            return
        }
        let remainingStepIDs = Set(plans.dropFirst(completedPlanIndex + 1).map(\.id))
        guard !remainingStepIDs.isEmpty else {
            return
        }
        var next = refreshProgress
        var didUpdate = false
        next.steps = next.steps.map { step in
            guard remainingStepIDs.contains(step.id), step.state == .pending else {
                return step
            }
            var updated = step
            updated.state = .skipped
            updated.summary = summary
            updated.error = nil
            didUpdate = true
            return updated
        }
        guard didUpdate else {
            return
        }
        next.lastSummary = summary
        refreshProgress = next
        persistActiveRefreshCheckpoint(cycleID: cycleID)
    }

    /// Applies the terminal state for one pipeline step.
    private func markRefreshStepFinished(id: String, result: RefreshScopeRunResult, cycleID: UUID? = nil) {
        guard isCurrentRefreshCycle(cycleID) else {
            return
        }
        var next = refreshProgress
        var didUpdate = false
        next.steps = next.steps.map { step in
            guard step.id == id else { return step }
            var updated = step
            updated.state = result.succeeded ? .succeeded : .failed
            updated.summary = result.summary
            updated.error = result.error
            didUpdate = true
            return updated
        }
        guard didUpdate else {
            return
        }
        next.lastSummary = result.summary ?? result.error
        refreshProgress = next
        persistActiveRefreshCheckpoint(cycleID: cycleID)
    }

    /// Closes the visible progress pipeline.
    private func finishRefreshProgress(cycleID: UUID? = nil) {
        if let cycleID, activeRefreshCycleID != cycleID {
            return
        }
        var next = refreshProgress
        next.isActive = false
        next.currentStepID = nil
        next.currentScope = nil
        next.completedAt = Date()
        refreshProgress = next
    }

    /// Reloads runtime-derived caches after refresh work changes files/DB rows.
    private func reloadRuntimeStateAfterRefresh() async {
        runtimeStatus = runtimeStore.runtimeStatus()
        do {
            spaceCache = try runtimeStore.loadFocusCache(kind: .space)
            personCache = try runtimeStore.loadFocusCache(kind: .person)
            spaceCache = canonicalizeSpaceTitles(spaceCache)
            pruneDetailSectionExpansionState(for: .space)
            pruneDetailSectionExpansionState(for: .person)
            ensureSelection(for: .space)
            ensureSelection(for: .person)
            ensureBeliefTargetSelection(for: .person)
            ensureBeliefTargetSelection(for: .space)
            reloadFocusTargetManagementState()
            try loadQuestionsFromStore()
            runtimeAccessDenied = false
        } catch {
            handleRuntimeAccessError(error)
        }
    }

    /// Whether a refresh scope is currently marked running.
    func isRefreshScopeRunning(_ scope: RefreshScope) -> Bool {
        refreshStatuses[scope]?.isRunning == true
    }

    /// Visible status text for refresh cards.
    func refreshStatusDetail(for scope: RefreshScope, fallback: String) -> String {
        if let status = refreshStatuses[scope] {
            if let error = status.lastError, !error.isEmpty {
                return "Last error: \(error)"
            }
            if let summary = status.lastSummary, !summary.isEmpty {
                return summary
            }
        }
        return fallback
    }

    /// Compact inline status for a focus list.
    func focusRefreshStatusText(for kind: FocusKind) -> String? {
        let scope: RefreshScope = kind == .person ? .personFocus : .spaceFocus
        if refreshProgress.isActive {
            if refreshProgress.currentScope == scope {
                return "updating \(kind.title) \(refreshProgress.completedStepCount)/\(refreshProgress.totalStepCount)"
            }
            if refreshProgress.currentScope == .webexSync {
                return "syncing Webex \(refreshProgress.completedStepCount)/\(refreshProgress.totalStepCount)"
            }
            if refreshProgress.currentScope == .questions {
                return "updating Questions \(refreshProgress.completedStepCount)/\(refreshProgress.totalStepCount)"
            }
        }
        if isRefreshScopeRunning(scope) {
            return "updating \(kind.title)"
        }
        return nil
    }

    /// Reloads the active focus cache from disk.
    func reloadSelectedFocus() async {
        isLoading = true
        errorMessage = nil
        do {
            switch selectedFocusKind {
            case .space:
                spaceCache = try runtimeStore.loadFocusCache(kind: .space)
                pruneDetailSectionExpansionState(for: .space)
                ensureBeliefTargetSelection(for: .space)
            case .person:
                personCache = try runtimeStore.loadFocusCache(kind: .person)
                pruneDetailSectionExpansionState(for: .person)
                ensureBeliefTargetSelection(for: .person)
            }
            ensureSelection(for: selectedFocusKind)
        } catch {
            handleRuntimeAccessError(error)
        }
        isLoading = false
    }

    /// Switches the active app section and loads section-specific state.
    func select(section: AppSection) {
        selectedSection = section
        switch section {
        case .spaceFocus:
            activateFocus(.space)
        case .personFocus:
            activateFocus(.person)
        case .spaceFocusTargets, .personFocusTargets:
            reloadFocusTargetManagement()
        case .execFocusTargets:
            reloadFocusTargetManagement()
        case .beliefs:
            ensureBeliefTargetSelection(for: beliefScopeFilter)
            Task {
                await refreshBeliefs()
                await refreshBeliefSetSummaries()
            }
        case .questions:
            Task { await loadQuestions() }
        default:
            break
        }
    }

    /// Reloads all focus target management lists.
    func reloadFocusTargetManagement() {
        targetManagementIsLoading = true
        reloadFocusTargetManagementState()
        targetManagementIsLoading = false
    }

    /// Configured targets for one management surface.
    func focusTargets(for kind: FocusTargetManagementKind) -> [ConfigTarget] {
        switch kind {
        case .spaceFocus:
            return spaceFocusManagementTargets
        case .personFocus:
            return personFocusManagementTargets
        case .execFocus:
            return execFocusManagementTargets
        }
    }

    /// Addable target candidates for one management surface.
    func focusCandidates(for kind: FocusTargetManagementKind) -> [ConfigTarget] {
        switch kind {
        case .spaceFocus:
            return spaceFocusCandidateTargets
        case .personFocus:
            return personFocusCandidateTargets
        case .execFocus:
            return execFocusCandidateTargets
        }
    }

    /// Source file backing one target-management surface.
    func focusTargetSourcePath(for kind: FocusTargetManagementKind) -> String {
        switch kind {
        case .spaceFocus:
            return configStore.importantTargetsURL.path
        case .personFocus:
            return configStore.importantTargetsURL.path
        case .execFocus:
            return configStore.importantExecutivesURL.path
        }
    }

    /// Selected target ID for one management surface.
    func selectedFocusTargetID(for kind: FocusTargetManagementKind) -> String? {
        selectedFocusTargetIDByKind[kind]
    }

    /// Updates selected target ID for one management surface.
    func setSelectedFocusTargetID(_ id: String?, for kind: FocusTargetManagementKind) {
        var next = selectedFocusTargetIDByKind
        if let id {
            next[kind] = id
        } else {
            next.removeValue(forKey: kind)
        }
        selectedFocusTargetIDByKind = next
    }

    /// Selected target row, falling back to the first configured target.
    func selectedFocusTarget(for kind: FocusTargetManagementKind) -> ConfigTarget? {
        let targets = focusTargets(for: kind)
        if let selectedID = selectedFocusTargetID(for: kind),
           let target = targets.first(where: { $0.id == selectedID }) {
            return target
        }
        return targets.first
    }

    /// Timeline targets that can accept a submitted transcript.
    func transcriptionTimelineTargets(for kind: TranscriptionTimelineTargetKind) -> [ConfigTarget] {
        switch kind {
        case .space:
            return spaceFocusManagementTargets.filter {
                !normalizedRoomID($0.roomID).isEmpty
            }
        case .person:
            return personFocusManagementTargets.filter {
                !normalizedRoomID($0.roomID).isEmpty || !normalizedEmail($0.email).isEmpty
            }
        }
    }

    /// Selected transcript-submission target ID for a target kind.
    func selectedTranscriptionTimelineTargetID(for kind: TranscriptionTimelineTargetKind) -> String? {
        selectedTranscriptionTimelineTargetIDByKind[kind]
    }

    /// Updates transcript-submission target selection.
    func setSelectedTranscriptionTimelineTargetID(_ id: String?, for kind: TranscriptionTimelineTargetKind) {
        var next = selectedTranscriptionTimelineTargetIDByKind
        if let id, !id.isEmpty {
            next[kind] = id
        } else {
            next.removeValue(forKey: kind)
        }
        selectedTranscriptionTimelineTargetIDByKind = next
    }

    /// Selected transcript-submission target, falling back to the first valid target.
    func selectedTranscriptionTimelineTarget(for kind: TranscriptionTimelineTargetKind) -> ConfigTarget? {
        let targets = transcriptionTimelineTargets(for: kind)
        if let selectedID = selectedTranscriptionTimelineTargetID(for: kind),
           let target = targets.first(where: { $0.id == selectedID }) {
            return target
        }
        return targets.first
    }

    /// Persists the current transcript as timeline evidence and queues refresh.
    func submitCurrentTranscriptToTimeline() {
        guard !transcriptionTimelineSubmissionRunning else {
            return
        }
        let kind = transcriptionTimelineTargetKind
        guard let target = selectedTranscriptionTimelineTarget(for: kind) else {
            transcriptionTimelineSubmissionError = "Choose a tracked \(kind.title.lowercased()) first."
            transcriptionTimelineSubmissionMessage = nil
            return
        }
        let transcriptText = transcriptionViewModel.transcriptSubmissionText
            .trimmingCharacters(in: .whitespacesAndNewlines)
        guard !transcriptText.isEmpty else {
            transcriptionTimelineSubmissionError = "There is no transcript text to submit yet."
            transcriptionTimelineSubmissionMessage = nil
            return
        }

        transcriptionTimelineSubmissionRunning = true
        transcriptionTimelineSubmissionError = nil
        transcriptionTimelineSubmissionMessage = nil
        defer {
            transcriptionTimelineSubmissionRunning = false
        }

        do {
            let submittedAt = Date()
            try persistTranscriptionTimelineSubmission(
                kind: kind,
                target: target,
                transcriptText: transcriptText,
                submittedAt: submittedAt
            )
            markFocusTargetRefreshDue(for: kind.focusTargetKind)
            queueFocusTargetRefresh(for: kind.focusTargetKind)
            transcriptionTimelineSubmissionMessage = "Submitted meetinging-transcript to \(timelineTargetTitle(target)). Refresh queued."
        } catch {
            transcriptionTimelineSubmissionError = error.localizedDescription
        }
    }

    /// Adds one candidate to a focus target file and queues affected refreshes.
    func addFocusTarget(_ candidate: ConfigTarget, to kind: FocusTargetManagementKind) {
        let mutationID = focusTargetMutationID(action: "add", target: candidate, kind: kind)
        guard targetManagementMutationID == nil else { return }
        targetManagementMutationID = mutationID
        targetManagementLastError = nil
        defer { targetManagementMutationID = nil }

        do {
            let added: Bool
            switch kind {
            case .spaceFocus:
                added = try configStore.addSpaceFocusTarget(candidate)
            case .personFocus:
                added = try configStore.addPersonFocusTarget(candidate)
            case .execFocus:
                added = try configStore.addExecFocusTarget(candidate)
            }
            reloadFocusTargetManagementState()
            setSelectedFocusTargetID(candidate.id, for: kind)
            if added {
                targetManagementLastMessage = "Added \(candidate.label) to \(kind.shortTitle). Refresh queued for affected focus results."
                markFocusTargetRefreshDue(for: kind)
                queueFocusTargetRefresh(for: kind)
            } else {
                targetManagementLastMessage = "\(candidate.label) is already in \(kind.shortTitle)."
            }
        } catch {
            targetManagementLastError = error.localizedDescription
        }
    }

    /// Removes one configured focus target and queues affected refreshes.
    func removeFocusTarget(_ target: ConfigTarget, from kind: FocusTargetManagementKind) {
        let mutationID = focusTargetMutationID(action: "remove", target: target, kind: kind)
        guard targetManagementMutationID == nil else { return }
        targetManagementMutationID = mutationID
        targetManagementLastError = nil
        defer { targetManagementMutationID = nil }

        do {
            let removed: Int
            switch kind {
            case .spaceFocus:
                removed = try configStore.removeSpaceFocusTarget(target)
            case .personFocus:
                removed = try configStore.removePersonFocusTarget(target)
            case .execFocus:
                removed = try configStore.removeExecFocusTarget(target)
            }
            reloadFocusTargetManagementState()
            if removed > 0 {
                targetManagementLastMessage = "Removed \(target.label) from \(kind.shortTitle). Refresh queued for affected focus results."
                markFocusTargetRefreshDue(for: kind)
                queueFocusTargetRefresh(for: kind)
            } else {
                targetManagementLastMessage = "\(target.label) was not present in \(kind.shortTitle)."
            }
        } catch {
            targetManagementLastError = error.localizedDescription
        }
    }

    /// Updates the Auto-Reply metadata on a person focus target.
    func setPersonFocusAutoReply(_ enabled: Bool, for target: ConfigTarget) {
        let mutationID = focusTargetMutationID(action: "auto-reply", target: target, kind: .personFocus)
        guard targetManagementMutationID == nil else { return }
        targetManagementMutationID = mutationID
        targetManagementLastError = nil
        defer { targetManagementMutationID = nil }

        do {
            try configStore.setPersonFocusAutoReply(enabled, for: target)
            reloadFocusTargetManagementState()
            targetManagementLastMessage = "Auto-Reply set to \(enabled ? "Yes" : "No") for \(target.label)."
        } catch {
            targetManagementLastError = error.localizedDescription
        }
    }

    /// Adds an iMessage handle to a person focus target.
    func addPersonIMessageHandle(to target: ConfigTarget) {
        let mutationID = focusTargetMutationID(action: "imessage-add", target: target, kind: .personFocus)
        guard targetManagementMutationID == nil else { return }
        let rawHandle = personIMessageHandleDraft.trimmingCharacters(in: .whitespacesAndNewlines)
        guard !rawHandle.isEmpty else {
            targetManagementLastError = "Enter a phone number or iMessage email."
            return
        }
        targetManagementMutationID = mutationID
        targetManagementLastError = nil
        defer { targetManagementMutationID = nil }

        do {
            let added = try configStore.addPersonIMessageHandle(rawHandle, to: target)
            reloadFocusTargetManagementState()
            if added {
                personIMessageHandleDraft = ""
                targetManagementLastMessage = "Added iMessage handle for \(target.label). Refresh queued for Person Focus."
                markFocusTargetRefreshDue(for: .personFocus)
                queueFocusTargetRefresh(for: .personFocus)
            } else {
                targetManagementLastMessage = "That iMessage handle is already configured for \(target.label)."
            }
        } catch {
            targetManagementLastError = error.localizedDescription
        }
    }

    /// Removes an iMessage handle from a person focus target.
    func removePersonIMessageHandle(_ handle: String, from target: ConfigTarget) {
        let mutationID = focusTargetMutationID(action: "imessage-remove-\(handle)", target: target, kind: .personFocus)
        guard targetManagementMutationID == nil else { return }
        targetManagementMutationID = mutationID
        targetManagementLastError = nil
        defer { targetManagementMutationID = nil }

        do {
            let removed = try configStore.removePersonIMessageHandle(handle, from: target)
            reloadFocusTargetManagementState()
            if removed {
                targetManagementLastMessage = "Removed iMessage handle for \(target.label). Refresh queued for Person Focus."
                markFocusTargetRefreshDue(for: .personFocus)
                queueFocusTargetRefresh(for: .personFocus)
            } else {
                targetManagementLastMessage = "That iMessage handle was not configured for \(target.label)."
            }
        } catch {
            targetManagementLastError = error.localizedDescription
        }
    }

    /// Whether a specific target mutation button should show progress.
    func isTargetMutationRunning(action: String, target: ConfigTarget, kind: FocusTargetManagementKind) -> Bool {
        targetManagementMutationID == focusTargetMutationID(action: action, target: target, kind: kind)
    }

    /// Reads target-management state from config files and candidate sources.
    private func reloadFocusTargetManagementState() {
        do {
            spaceFocusManagementTargets = try configStore.spaceFocusManagementTargets()
            spaceFocusCandidateTargets = try configStore.spaceFocusAddableSpaces()
            personFocusManagementTargets = try configStore.personFocusManagementTargets()
            personFocusCandidateTargets = try configStore.personFocusAddablePeople()
            execFocusManagementTargets = try configStore.execFocusManagementTargets()
            execFocusCandidateTargets = try configStore.execFocusAddablePeople()
            ensureFocusTargetSelection(for: .spaceFocus)
            ensureFocusTargetSelection(for: .personFocus)
            ensureFocusTargetSelection(for: .execFocus)
            ensureTranscriptionTimelineTargetSelection(for: .space)
            ensureTranscriptionTimelineTargetSelection(for: .person)
            targetManagementLastError = nil
        } catch {
            targetManagementLastError = error.localizedDescription
        }
    }

    /// Keeps selected focus target valid after list reloads.
    private func ensureFocusTargetSelection(for kind: FocusTargetManagementKind) {
        let targets = focusTargets(for: kind)
        if let selectedID = selectedFocusTargetID(for: kind),
           targets.contains(where: { $0.id == selectedID }) {
            return
        }
        setSelectedFocusTargetID(targets.first?.id, for: kind)
    }

    /// Stable ID for per-row target management actions.
    private func focusTargetMutationID(action: String, target: ConfigTarget, kind: FocusTargetManagementKind) -> String {
        "\(kind.rawValue):\(action):\(target.id)"
    }

    /// Converts a submitted transcript into message/evidence rows.
    private func persistTranscriptionTimelineSubmission(
        kind: TranscriptionTimelineTargetKind,
        target: ConfigTarget,
        transcriptText: String,
        submittedAt: Date
    ) throws {
        let submittedTimestamp = Self.iso8601Timestamp(submittedAt)
        let targetTitle = timelineTargetTitle(target)
        let targetRoomID = normalizedRoomID(target.roomID)
        let targetEmail = normalizedEmail(target.email)
        let targetComponent = stableTimelineIDComponent("\(kind.rawValue):\(target.id)")

        let roomID: String
        let personID: String?
        switch kind {
        case .space:
            guard !targetRoomID.isEmpty else {
                throw TranscriptionTimelineSubmissionError.invalidTarget("Choose a tracked space with a Webex room ID.")
            }
            roomID = targetRoomID
            personID = "cubicle-meeting-transcript"
            try knowledgeStore.upsertPerson(
                PersonRecord(
                    id: personID ?? "",
                    displayName: "Cubicle Meeting Transcript",
                    email: "",
                    updatedAt: submittedTimestamp
                )
            )
        case .person:
            if !targetRoomID.isEmpty {
                roomID = targetRoomID
            } else if !targetEmail.isEmpty {
                roomID = "cubicle-person-\(targetComponent)-meeting-transcripts"
            } else {
                throw TranscriptionTimelineSubmissionError.invalidTarget("Choose a tracked person with an email or Webex room ID.")
            }
            personID = targetEmail.isEmpty ? "cubicle-meeting-transcript" : targetEmail
            try knowledgeStore.upsertPerson(
                PersonRecord(
                    id: personID ?? "",
                    displayName: targetTitle,
                    email: targetEmail,
                    updatedAt: submittedTimestamp
                )
            )
        }

        try knowledgeStore.upsertRoom(
            RoomRecord(
                id: roomID,
                title: targetTitle,
                updatedAt: submittedTimestamp
            )
        )

        let messageID = "meetinging-transcript:\(kind.rawValue):\(targetComponent):\(UUID().uuidString)"
        let body = """
        Annotation: meetinging-transcript
        Target type: \(kind.title)
        Target: \(targetTitle)
        Submitted at: \(submittedTimestamp)
        Session state: \(transcriptionViewModel.sessionStateText)

        Transcript:
        \(transcriptText)
        """
        try knowledgeStore.upsertMessage(
            MessageRecord(
                id: messageID,
                roomID: roomID,
                personID: personID,
                body: body,
                createdAt: submittedTimestamp,
                updatedAt: submittedTimestamp
            )
        )
    }

    private func ensureTranscriptionTimelineTargetSelection(for kind: TranscriptionTimelineTargetKind) {
        let targets = transcriptionTimelineTargets(for: kind)
        if let selectedID = selectedTranscriptionTimelineTargetID(for: kind),
           targets.contains(where: { $0.id == selectedID }) {
            return
        }
        setSelectedTranscriptionTimelineTargetID(targets.first?.id, for: kind)
    }

    private func timelineTargetTitle(_ target: ConfigTarget) -> String {
        let label = target.label.trimmingCharacters(in: .whitespacesAndNewlines)
        if !label.isEmpty {
            return label
        }
        let email = normalizedEmail(target.email)
        if !email.isEmpty {
            return email
        }
        let roomID = normalizedRoomID(target.roomID)
        if !roomID.isEmpty {
            return roomID
        }
        return target.id
    }

    private func stableTimelineIDComponent(_ value: String) -> String {
        let digest = SHA256.hash(data: Data(value.utf8))
        return digest.prefix(8).map { String(format: "%02x", $0) }.joined()
    }

    /// Marks affected refresh cadences stale after target changes.
    private func markFocusTargetRefreshDue(for kind: FocusTargetManagementKind) {
        switch kind {
        case .spaceFocus:
            lastRefreshDateByScope.removeValue(forKey: .webexSync)
            lastRefreshDateByScope.removeValue(forKey: .spaceFocus)
            lastRefreshDateByScope.removeValue(forKey: .questions)
        case .personFocus:
            lastRefreshDateByScope.removeValue(forKey: .webexSync)
            lastRefreshDateByScope.removeValue(forKey: .personFocus)
            lastRefreshDateByScope.removeValue(forKey: .spaceFocus)
            lastRefreshDateByScope.removeValue(forKey: .questions)
        case .execFocus:
            lastRefreshDateByScope.removeValue(forKey: .webexSync)
            lastRefreshDateByScope.removeValue(forKey: .spaceFocus)
            lastRefreshDateByScope.removeValue(forKey: .questions)
        }
    }

    /// Queues a focused refresh after target changes.
    private func queueFocusTargetRefresh(for kind: FocusTargetManagementKind) {
        let plans: [RefreshPipelinePlan]
        switch kind {
        case .spaceFocus:
            plans = [
                RefreshPipelinePlan(id: "target-space-webex", scope: .webexSync, title: "Webex sync", mode: .incremental, markCadence: true),
                RefreshPipelinePlan(id: "target-space-focus", scope: .spaceFocus, title: "Space Focus", mode: .full, markCadence: true),
                RefreshPipelinePlan(id: "target-questions", scope: .questions, title: "Questions", mode: .full, markCadence: true)
            ]
        case .personFocus:
            plans = [
                RefreshPipelinePlan(id: "target-person-webex", scope: .webexSync, title: "Webex sync", mode: .incremental, markCadence: true),
                RefreshPipelinePlan(id: "target-person-focus", scope: .personFocus, title: "Person Focus", mode: .cacheAwareFull, markCadence: true),
                RefreshPipelinePlan(id: "target-space-focus", scope: .spaceFocus, title: "Space Focus", mode: .cacheAwareFull, markCadence: true),
                RefreshPipelinePlan(id: "target-questions", scope: .questions, title: "Questions", mode: .full, markCadence: true)
            ]
        case .execFocus:
            plans = [
                RefreshPipelinePlan(id: "target-exec-webex", scope: .webexSync, title: "Webex sync", mode: .incremental, markCadence: true),
                RefreshPipelinePlan(id: "target-space-focus", scope: .spaceFocus, title: "Space Focus", mode: .full, markCadence: true),
                RefreshPipelinePlan(id: "target-questions", scope: .questions, title: "Questions", mode: .full, markCadence: true)
            ]
        }

        Task {
            _ = await runRefreshCycle(
                title: "\(kind.shortTitle) targets changed",
                forceAll: false,
                pipelinePlans: plans,
                reloadAfterEachScope: false
            )
        }
    }

    /// Switches the active focus kind and ensures selection remains valid.
    func activateFocus(_ kind: FocusKind) {
        selectedFocusKind = kind
        ensureSelection(for: kind)
    }

    /// Current search text for one focus kind.
    func searchText(for kind: FocusKind) -> String {
        searchTextByKind[kind, default: ""]
    }

    /// Updates search text and repairs selection if filtering removes it.
    func setSearchText(_ text: String, for kind: FocusKind) {
        var next = searchTextByKind
        next[kind] = text
        searchTextByKind = next
        ensureSelection(for: kind)
    }

    /// Current sort option for one focus kind.
    func sortOption(for kind: FocusKind) -> FocusSortOption {
        sortOptionByKind[kind, default: .latestMessage]
    }

    /// Updates sort option and repairs selection.
    func setSortOption(_ option: FocusSortOption, for kind: FocusKind) {
        var next = sortOptionByKind
        next[kind] = option
        sortOptionByKind = next
        ensureSelection(for: kind)
    }

    /// Selected focus item ID for one focus kind.
    func selectedItemID(for kind: FocusKind) -> String? {
        selectedItemIDByKind[kind]
    }

    /// Updates selected focus item ID for one focus kind.
    func setSelectedItemID(_ id: String?, for kind: FocusKind) {
        var next = selectedItemIDByKind
        if let id {
            next[kind] = id
        } else {
            next.removeValue(forKey: kind)
        }
        selectedItemIDByKind = next
    }

    /// Whether the focus list has an active search query.
    func hasSearchQuery(for kind: FocusKind) -> Bool {
        !searchText(for: kind).trimmingCharacters(in: .whitespacesAndNewlines).isEmpty
    }

    /// Number of expanded detail sections for one item.
    func expandedDetailSectionCount(for kind: FocusKind, itemID: String) -> Int {
        let key = detailItemKey(kind: kind, itemID: itemID)
        return expandedDetailSectionIDsByItemKey[key]?.count ?? 0
    }

    /// Whether one detail section is expanded.
    func isDetailSectionExpanded(for kind: FocusKind, itemID: String, sectionID: String) -> Bool {
        let key = detailItemKey(kind: kind, itemID: itemID)
        return expandedDetailSectionIDsByItemKey[key]?.contains(sectionID) ?? false
    }

    /// Updates expansion state for one detail section.
    func setDetailSectionExpanded(
        _ isExpanded: Bool,
        for kind: FocusKind,
        itemID: String,
        sectionID: String
    ) {
        let key = detailItemKey(kind: kind, itemID: itemID)
        var next = expandedDetailSectionIDsByItemKey
        var sectionIDs = next[key, default: []]
        if isExpanded {
            sectionIDs.insert(sectionID)
        } else {
            sectionIDs.remove(sectionID)
        }
        if sectionIDs.isEmpty {
            next.removeValue(forKey: key)
        } else {
            next[key] = sectionIDs
        }
        expandedDetailSectionIDsByItemKey = next
    }

    /// Expands or collapses every detail section for one item.
    func setAllDetailSectionsExpanded(
        _ isExpanded: Bool,
        for kind: FocusKind,
        itemID: String,
        sectionIDs: [String]
    ) {
        let key = detailItemKey(kind: kind, itemID: itemID)
        var next = expandedDetailSectionIDsByItemKey
        if isExpanded {
            next[key] = Set(sectionIDs)
        } else {
            next.removeValue(forKey: key)
        }
        expandedDetailSectionIDsByItemKey = next
    }

    /// Current cache for one focus kind.
    func cache(for kind: FocusKind) -> FocusCache {
        switch kind {
        case .space: return spaceCache
        case .person: return personCache
        }
    }

    /// Search-filtered and sorted focus items.
    func filteredItems(for kind: FocusKind) -> [FocusItem] {
        let items = cache(for: kind).items
        let query = searchText(for: kind).trimmingCharacters(in: .whitespacesAndNewlines)
        let filteredItems: [FocusItem]
        if query.isEmpty {
            filteredItems = items
        } else {
            filteredItems = items.filter { $0.searchableText.localizedCaseInsensitiveContains(query) }
        }
        return sortItems(filteredItems, for: kind)
    }

    /// Selected focus item, falling back to the first filtered item.
    func selectedItem(for kind: FocusKind) -> FocusItem? {
        let items = filteredItems(for: kind)
        if let selectedItemID = selectedItemID(for: kind),
           let match = items.first(where: { $0.id == selectedItemID }) {
            return match
        }
        return items.first
    }

    /// Selectable belief targets for a scope.
    func beliefTargetOptions(for scope: KnowledgeBeliefScope) -> [BeliefTargetOption] {
        switch scope {
        case .global:
            return [
                BeliefTargetOption(
                    entityKey: KnowledgeBeliefScope.globalEntityKey,
                    title: "Global",
                    subtitle: "System-wide belief scope"
                )
            ]
        case .person:
            return personCache.items.map { item in
                BeliefTargetOption(
                    entityKey: beliefEntityKey(for: item, scope: .person),
                    title: item.title,
                    subtitle: item.subtitle.isEmpty ? item.meta : item.subtitle
                )
            }
        case .space:
            return spaceCache.items.map { item in
                BeliefTargetOption(
                    entityKey: beliefEntityKey(for: item, scope: .space),
                    title: item.title,
                    subtitle: item.subtitle.isEmpty ? item.meta : item.subtitle
                )
            }
        }
    }

    /// Current belief entity key for a scope.
    func selectedBeliefEntityKey(for scope: KnowledgeBeliefScope) -> String? {
        resolvedBeliefEntityKey(for: scope)
    }

    /// Switches belief scope and applies any cached rows for it.
    func setBeliefScopeFilter(_ scope: KnowledgeBeliefScope) {
        beliefScopeFilter = scope
        ensureBeliefTargetSelection(for: scope)
        applyCachedBeliefsForCurrentSelection()
    }

    /// Updates selected belief entity key within a scope.
    func setBeliefEntityKey(_ entityKey: String, for scope: KnowledgeBeliefScope) {
        var next = beliefEntityKeyByScope
        next[scope] = normalizedBeliefEntityKey(entityKey, scope: scope)
        beliefEntityKeyByScope = next
        applyCachedBeliefsForCurrentSelection()
    }

    /// Manual plus automatic beliefs in display order.
    func combinedBeliefs() -> [BeliefRecord] {
        manualBeliefs + automaticBeliefs
    }

    /// Selected belief, falling back to the first combined belief.
    func selectedBelief() -> BeliefRecord? {
        guard let selectedBeliefID else {
            return combinedBeliefs().first
        }
        return combinedBeliefs().first(where: { $0.id == selectedBeliefID }) ?? combinedBeliefs().first
    }

    /// Updates selected belief ID.
    func setSelectedBeliefID(_ id: String?) {
        selectedBeliefID = id
    }

    /// Refresh status row for belief maintenance.
    func beliefMaintenanceStatus() -> RefreshRunStatus {
        refreshStatuses[.beliefMaintenance] ?? RefreshRunStatus(scope: .beliefMaintenance, isRunning: false)
    }

    /// Opens a belief summary row in the scoped belief list.
    func selectBeliefSetSummary(_ summary: BeliefSetSummary) {
        setBeliefScopeFilter(summary.scope)
        setBeliefEntityKey(summary.entityKey, for: summary.scope)
    }

    /// Runs belief reconciliation immediately and reloads belief views.
    func runBeliefMaintenanceNow() async {
        let startedAt = Self.iso8601Timestamp(Date())
        updateRefreshStatus(.beliefMaintenance) { status in
            status.isRunning = true
            status.lastStartedAt = startedAt
            status.lastError = nil
        }
        do {
            let result = try await refreshCoordinator.refresh(.beliefMaintenance)
            lastRefreshDateByScope[.beliefMaintenance] = Date()
            updateRefreshStatus(.beliefMaintenance) { status in
                status.isRunning = false
                status.lastCompletedAt = result.completedAt
                status.lastSummary = result.summary
                status.lastError = nil
            }
            beliefsLastError = nil
            invalidateBeliefCaches()
            await refreshBeliefs(force: true)
            await refreshBeliefSetSummaries(force: true)
        } catch {
            updateRefreshStatus(.beliefMaintenance) { status in
                status.isRunning = false
                status.lastCompletedAt = Self.iso8601Timestamp(Date())
                status.lastError = error.localizedDescription
            }
            beliefsLastError = error.localizedDescription
            handleRuntimeAccessError(error)
        }
    }

    /// Refreshes cached belief-set summary counts.
    func refreshBeliefSetSummaries(force: Bool = false, showLoadingIndicator: Bool = true) async {
        let targets = beliefSummaryTargets()
        let targetFingerprint = beliefSummaryTargetFingerprint(targets)
        if let cached = cachedBeliefSetSummaries,
           cached.targetFingerprint == targetFingerprint,
           !force {
            beliefSetSummaries = cached.summaries
            beliefSetSummariesLastError = nil
            if isBeliefCacheFresh(cached.loadedAt) {
                return
            }
            guard activeBeliefSummaryFingerprint != targetFingerprint else {
                return
            }
            Task { await refreshBeliefSetSummaries(force: true, showLoadingIndicator: false) }
            return
        }

        guard activeBeliefSummaryFingerprint != targetFingerprint else {
            return
        }

        activeBeliefSummaryFingerprint = targetFingerprint
        let shouldShowLoading = showLoadingIndicator || cachedBeliefSetSummaries == nil
        if shouldShowLoading {
            beliefSetSummariesIsLoading = true
        }
        beliefSetSummariesLastError = nil
        defer {
            if activeBeliefSummaryFingerprint == targetFingerprint {
                activeBeliefSummaryFingerprint = nil
                if shouldShowLoading {
                    beliefSetSummariesIsLoading = false
                }
            }
        }

        do {
            let configuration = runtimeStore.configuration
            let loaded = try await Self.loadBeliefSetSummaries(
                configuration: configuration,
                targets: targets
            )
            let sorted = sortedBeliefSetSummaries(loaded)
            let cache = CachedBeliefSetSummaries(
                summaries: sorted,
                targetFingerprint: targetFingerprint,
                loadedAt: Date()
            )
            cachedBeliefSetSummaries = cache
            if beliefSummaryTargetFingerprint(beliefSummaryTargets()) == targetFingerprint {
                beliefSetSummaries = sorted
            }
            beliefSetSummariesLastError = nil
        } catch {
            if cachedBeliefSetSummaries?.targetFingerprint != targetFingerprint {
                beliefSetSummaries = []
            }
            beliefSetSummariesLastError = error.localizedDescription
        }
    }

    /// Loads question candidates from the knowledge store.
    func loadQuestions() async {
        questionsIsLoading = true
        questionsLastError = nil
        defer {
            questionsIsLoading = false
        }
        do {
            try loadQuestionsFromStore()
        } catch {
            questionCandidates = []
            selectedQuestionID = nil
            questionsLastError = error.localizedDescription
        }
    }

    /// Rebuilds question candidates and reloads the visible list.
    func refreshQuestions() async {
        questionsIsLoading = true
        questionsLastError = nil
        defer {
            questionsIsLoading = false
        }
        do {
            _ = try await questionServiceForCurrentSettings().refreshQuestionCandidates(
                spaceCache: spaceCache,
                personCache: personCache
            )
            try loadQuestionsFromStore()
        } catch {
            questionsLastError = error.localizedDescription
        }
    }

    /// Selected question, falling back to the first candidate.
    func selectedQuestion() -> QuestionCandidate? {
        guard let selectedQuestionID else {
            return questionCandidates.first
        }
        return questionCandidates.first(where: { $0.id == selectedQuestionID }) ?? questionCandidates.first
    }

    /// Updates selected question ID.
    func setSelectedQuestionID(_ id: String?) {
        selectedQuestionID = id
    }

    /// Active questions scoped to a focus item.
    func questionCandidates(for kind: FocusKind, itemID: String, limit: Int = 5) -> [QuestionCandidate] {
        let scopeType: QuestionScopeType = kind == .person ? .person : .space
        let canonicalItemID = canonicalFocusScopeKey(itemID, kind: kind)
        return Array(
            questionCandidates
                .filter {
                    $0.scopeType == scopeType
                        && canonicalFocusScopeKey($0.scopeKey, kind: kind) == canonicalItemID
                        && activeQuestionStatuses.contains($0.status)
                }
                .sorted { left, right in
                    if left.priorityScore == right.priorityScore {
                        return left.updatedAt > right.updatedAt
                    }
                    return left.priorityScore > right.priorityScore
                }
                .prefix(limit)
        )
    }

    /// Updates a question lifecycle state.
    func updateQuestionStatus(id: String, status: QuestionStatus) async {
        do {
            let expiresAt = status == .snoozed
                ? Calendar.current.date(byAdding: .day, value: 1, to: Date())
                : nil
            _ = try questionService.updateQuestionStatus(id: id, status: status, expiresAt: expiresAt)
            questionsLastError = nil
            try loadQuestionsFromStore()
        } catch {
            questionsLastError = error.localizedDescription
        }
    }

    /// Reloads question rows and repairs selection.
    private func loadQuestionsFromStore() throws {
        questionCandidates = try questionService.listQuestionCandidates(limit: 250)
        ensureQuestionSelection()
    }

    /// Seeds questions when no Codex-generated questions are present.
    private func refreshQuestionsIfEmpty() async throws {
        let service = questionServiceForCurrentSettings()
        let existing = try service.listQuestionCandidates(limit: 250)
        let hasCodexSynthesizedQuestions = existing.contains { $0.sourceKind == "codex_question_synthesis" }
        guard existing.isEmpty || !hasCodexSynthesizedQuestions else {
            return
        }
        _ = try await service.refreshQuestionCandidates(
            spaceCache: spaceCache,
            personCache: personCache
        )
    }

    /// Creates a question service respecting current Codex feature flags.
    private func questionServiceForCurrentSettings() -> QuestionCandidateService {
        let synthesizer: QuestionCandidateSynthesizing? = systemSettings.codexFeatureEnabled(.questionSynthesis)
            ? codexOrchestrationService
            : nil
        return QuestionCandidateService(
            knowledgeStore: knowledgeStore,
            questionSynthesizer: synthesizer
        )
    }

    /// Keeps selected question valid after list changes.
    private func ensureQuestionSelection() {
        guard let selectedQuestionID,
              questionCandidates.contains(where: { $0.id == selectedQuestionID }) else {
            selectedQuestionID = questionCandidates.first?.id
            return
        }
    }

    /// Refreshes manual/automatic beliefs for the selected scope/entity.
    func refreshBeliefs(force: Bool = false, showLoadingIndicator: Bool = true) async {
        guard let key = currentBeliefCacheKey() else {
            manualBeliefs = []
            automaticBeliefs = []
            selectedBeliefID = nil
            return
        }

        if let cached = beliefSetCache[key], !force {
            applyBeliefCacheEntry(cached)
            beliefsLastError = nil
            if isBeliefCacheFresh(cached.loadedAt) {
                return
            }
            guard activeBeliefLoadKey != key else {
                return
            }
            Task { await refreshBeliefs(force: true, showLoadingIndicator: false) }
            return
        }

        guard activeBeliefLoadKey != key else {
            return
        }

        activeBeliefLoadKey = key
        let shouldShowLoading = showLoadingIndicator || beliefSetCache[key] == nil
        if shouldShowLoading {
            beliefsIsLoading = true
        }
        beliefsLastError = nil
        defer {
            if activeBeliefLoadKey == key {
                activeBeliefLoadKey = nil
                if shouldShowLoading {
                    beliefsIsLoading = false
                }
            }
        }

        do {
            let entry = try await Self.loadBeliefSet(
                configuration: runtimeStore.configuration,
                key: key
            )
            beliefSetCache[key] = entry
            if currentBeliefCacheKey() == key {
                applyBeliefCacheEntry(entry)
            }
        } catch {
            if beliefSetCache[key] == nil {
                manualBeliefs = []
                automaticBeliefs = []
                selectedBeliefID = nil
            }
            beliefsLastError = error.localizedDescription
        }
    }

    /// Adds a manual belief to the selected scope/entity.
    func addManualBelief(
        statement: String,
        confidence: Double,
        evidenceLinks: [String]
    ) async {
        let normalizedStatement = statement.trimmingCharacters(in: .whitespacesAndNewlines)
        guard !normalizedStatement.isEmpty else {
            beliefsLastError = "Manual belief statement is required."
            return
        }
        guard let entityKey = resolvedBeliefEntityKey(for: beliefScopeFilter) else {
            beliefsLastError = "No belief target is selected for this scope."
            return
        }

        do {
            try knowledgeStore.bootstrap()
            let clampedConfidence = max(0, min(1, confidence))
            let now = Self.iso8601Timestamp(Date())
            let record = BeliefRecord(
                id: "manual-\(UUID().uuidString.lowercased())",
                scope: beliefScopeFilter.rawValue,
                entityKey: entityKey,
                statement: normalizedStatement,
                confidence: clampedConfidence,
                updatedAt: now,
                isManual: true,
                evidenceLinks: evidenceLinks,
                createdAt: now,
                beliefKind: "manual",
                lifecycle: "manual",
                supportCount: 1,
                contradictionCount: 0,
                lastEvidenceAt: now
            )
            try knowledgeStore.upsertManualBelief(record)
            beliefsLastError = nil
            invalidateBeliefCaches(for: BeliefCacheKey(scope: beliefScopeFilter, entityKey: entityKey))
            await refreshBeliefs(force: true)
        } catch {
            beliefsLastError = error.localizedDescription
        }
    }

    /// Updates an existing manual belief in the selected store.
    func updateManualBelief(
        id: String,
        statement: String,
        confidence: Double,
        evidenceLinks: [String]
    ) async {
        guard let current = manualBeliefs.first(where: { $0.id == id }) else {
            beliefsLastError = "The selected manual belief no longer exists."
            return
        }
        let normalizedStatement = statement.trimmingCharacters(in: .whitespacesAndNewlines)
        guard !normalizedStatement.isEmpty else {
            beliefsLastError = "Manual belief statement is required."
            return
        }

        do {
            try knowledgeStore.bootstrap()
            let clampedConfidence = max(0, min(1, confidence))
            _ = try knowledgeStore.updateManualBelief(
                id: id,
                scope: current.scope,
                entityKey: current.entityKey,
                statement: normalizedStatement,
                confidence: clampedConfidence,
                evidenceLinks: evidenceLinks,
                updatedAt: Self.iso8601Timestamp(Date())
            )
            beliefsLastError = nil
            invalidateBeliefCaches(for: BeliefCacheKey(scope: KnowledgeBeliefScope(rawValue: current.scope) ?? beliefScopeFilter, entityKey: current.entityKey))
            await refreshBeliefs(force: true)
            selectedBeliefID = id
        } catch {
            beliefsLastError = error.localizedDescription
        }
    }

    /// Deletes a manual belief and reloads the current belief set.
    func deleteManualBelief(id: String) async {
        do {
            try knowledgeStore.bootstrap()
            _ = try knowledgeStore.deleteManualBelief(id: id)
            beliefsLastError = nil
            if selectedBeliefID == id {
                selectedBeliefID = nil
            }
            let scope = KnowledgeBeliefScope(rawValue: manualBeliefs.first(where: { $0.id == id })?.scope ?? beliefScopeFilter.rawValue) ?? beliefScopeFilter
            let entityKey = manualBeliefs.first(where: { $0.id == id })?.entityKey ?? resolvedBeliefEntityKey(for: beliefScopeFilter) ?? ""
            invalidateBeliefCaches(for: BeliefCacheKey(scope: scope, entityKey: entityKey))
            await refreshBeliefs(force: true)
        } catch {
            beliefsLastError = error.localizedDescription
        }
    }

    /// Whether Ask Codex has enough input and context to run.
    var canRunAskCodex: Bool {
        guard !askCodexIsRunning else { return false }
        guard !askCodexQuestion.trimmingCharacters(in: .whitespacesAndNewlines).isEmpty else { return false }
        return targetContext(for: askCodexTargetScope) != nil
    }

    /// Warning text for unavailable scoped Ask Codex targets.
    func askCodexTargetWarning() -> String? {
        switch askCodexTargetScope {
        case .allTracked:
            return nil
        case .selectedSpace:
            return selectedItem(for: .space) == nil ? "Select a Space Focus item before running this scope." : nil
        case .selectedPerson:
            return selectedItem(for: .person) == nil ? "Select a Person Focus item before running this scope." : nil
        }
    }

    /// Preview context lines for the current Ask Codex scope.
    func askCodexContextPreviewLines() -> [String] {
        guard let context = targetContext(for: askCodexTargetScope) else {
            return []
        }
        return context.lines
    }

    /// Current Ask Codex target title.
    func askCodexCurrentTargetTitle() -> String {
        targetContext(for: askCodexTargetScope)?.title ?? askCodexTargetScope.title
    }

    /// Restores an Ask Codex history entry into the composer.
    func applyAskCodexQueryHistory(_ entry: AskCodexQueryHistoryEntry) {
        switch entry.targetScope {
        case .allTracked:
            break
        case .selectedSpace:
            setSearchText("", for: .space)
            if let targetItemID = entry.targetItemID {
                setSelectedItemID(targetItemID, for: .space)
            }
        case .selectedPerson:
            setSearchText("", for: .person)
            if let targetItemID = entry.targetItemID {
                setSelectedItemID(targetItemID, for: .person)
            }
        }
        askCodexTargetScope = entry.targetScope
        askCodexQuestion = entry.question
        askCodexLastError = nil
        askCodexResult = nil
    }

    /// Clears the Ask Codex prompt box.
    func clearAskCodexQuestion() {
        askCodexQuestion = ""
    }

    /// Runs Codex over the selected local context and stores the result.
    func runAskCodex() async {
        guard !askCodexIsRunning else {
            return
        }

        guard systemSettings.codexFeatureEnabled(.askCodex) else {
            askCodexResult = nil
            askCodexLastError = "Ask Codex is disabled in Settings."
            return
        }

        let question = askCodexQuestion.trimmingCharacters(in: .whitespacesAndNewlines)
        guard !question.isEmpty else {
            askCodexLastError = "Enter a question before running Ask Codex."
            return
        }

        guard let context = targetContext(for: askCodexTargetScope) else {
            askCodexLastError = askCodexTargetWarning() ?? "The selected target scope is unavailable."
            return
        }

        askCodexIsRunning = true
        let codexMessage = "Using Codex for activity Ask Codex: \(context.title)..."
        codexActivityMessage = codexMessage
        askCodexResult = nil
        askCodexLastError = nil
        recordAskCodexQuery(question: question, context: context)
        defer {
            askCodexIsRunning = false
            if codexActivityMessage == codexMessage {
                codexActivityMessage = nil
            }
        }

        let createdAt = Date()
        let promptVersion = "ask-codex-v1"
        let prompt = askCodexPrompt(
            question: question,
            targetContext: context,
            promptVersion: promptVersion
        )
        let job = askCodexJob(
            targetContext: context,
            promptVersion: promptVersion,
            createdAt: createdAt
        )

        let request = CodexRunRequest(
            prompt: prompt,
            job: job,
            workingDirectory: runtimeStore.configuration.runtimeRoot,
            policy: .default
        )

        do {
            let result = try await codexRunner.run(request: request)
            askCodexResult = AskCodexRunRecord(
                jobID: result.metadata.jobID,
                title: result.metadata.title,
                targetScope: askCodexTargetScope,
                targetTitle: context.title,
                question: question,
                submittedAt: createdAt,
                output: result.output,
                outputPath: result.metadata.outputPath,
                logPath: result.metadata.logPath,
                metadataPath: result.metadata.metadataPath,
                attempts: result.metadata.attempts,
                status: result.metadata.status,
                )
        } catch {
            askCodexLastError = error.localizedDescription
        }
    }

    /// Saves Ask Codex prompt history with duplicate collapse.
    private func recordAskCodexQuery(question: String, context: AskCodexTargetContext) {
        let entry = AskCodexQueryHistoryEntry(
            id: UUID().uuidString,
            question: question,
            targetScope: askCodexTargetScope,
            targetTitle: context.title,
            targetKey: context.key,
            targetItemID: context.itemID,
            submittedAt: Self.iso8601Timestamp(Date())
        )
        var entries = askCodexQueryHistory.filter { existing in
            !(existing.question.localizedCaseInsensitiveCompare(entry.question) == .orderedSame
              && existing.targetScope == entry.targetScope
              && existing.targetKey == entry.targetKey)
        }
        entries.insert(entry, at: 0)
        entries = Array(entries.prefix(100))
        askCodexQueryHistory = entries
        do {
            try configStore.saveAskCodexQueryHistory(entries)
        } catch {
            askCodexLastError = error.localizedDescription
        }
    }

    /// Keeps selected focus item valid after filter/cache changes.
    private func ensureSelection(for kind: FocusKind) {
        let items = filteredItems(for: kind)
        if let selectedItemID = selectedItemID(for: kind),
           items.contains(where: { $0.id == selectedItemID }) {
            return
        }
        setSelectedItemID(items.first?.id, for: kind)
    }

    /// Storage key for expanded detail sections by item.
    private func detailItemKey(kind: FocusKind, itemID: String) -> String {
        "\(kind.rawValue):\(itemID)"
    }

    /// Normalizes old prefixed focus IDs before comparisons.
    private func canonicalFocusScopeKey(_ value: String, kind: FocusKind) -> String {
        let trimmed = value.trimmingCharacters(in: .whitespacesAndNewlines)
        switch kind {
        case .space:
            if let range = trimmed.range(of: "spacefocus:", options: [.anchored, .caseInsensitive]) {
                return String(trimmed[range.upperBound...])
            }
        case .person:
            if let range = trimmed.range(of: "personfocus:", options: [.anchored, .caseInsensitive]) {
                return String(trimmed[range.upperBound...])
            }
        }
        return trimmed
    }

    /// Drops expansion state for items no longer present in the cache.
    private func pruneDetailSectionExpansionState(for kind: FocusKind) {
        let validKeys = Set(cache(for: kind).items.map { detailItemKey(kind: kind, itemID: $0.id) })
        let prefix = "\(kind.rawValue):"
        let staleKeys = expandedDetailSectionIDsByItemKey.keys.filter { key in
            key.hasPrefix(prefix) && !validKeys.contains(key)
        }
        guard !staleKeys.isEmpty else {
            return
        }
        var next = expandedDetailSectionIDsByItemKey
        staleKeys.forEach { next.removeValue(forKey: $0) }
        expandedDetailSectionIDsByItemKey = next
    }

    /// Keeps belief target selection valid after cache changes.
    private func ensureBeliefTargetSelection(for scope: KnowledgeBeliefScope) {
        let options = beliefTargetOptions(for: scope)
        guard !options.isEmpty else {
            var next = beliefEntityKeyByScope
            next.removeValue(forKey: scope)
            beliefEntityKeyByScope = next
            return
        }
        let validKeys = Set(options.map(\.entityKey))
        let current = beliefEntityKeyByScope[scope]
        guard let current, validKeys.contains(current) else {
            var next = beliefEntityKeyByScope
            next[scope] = options[0].entityKey
            beliefEntityKeyByScope = next
            return
        }
    }

    /// Selected belief entity key after validation.
    private func resolvedBeliefEntityKey(for scope: KnowledgeBeliefScope) -> String? {
        ensureBeliefTargetSelection(for: scope)
        return beliefEntityKeyByScope[scope]
    }

    /// Keeps selected belief valid after row changes.
    private func ensureBeliefSelection() {
        let availableIDs = Set(combinedBeliefs().map(\.id))
        guard let selectedBeliefID, availableIDs.contains(selectedBeliefID) else {
            self.selectedBeliefID = combinedBeliefs().first?.id
            return
        }
    }

    /// Current belief cache key, if a target is selected.
    private func currentBeliefCacheKey() -> BeliefCacheKey? {
        ensureBeliefTargetSelection(for: beliefScopeFilter)
        guard let entityKey = beliefEntityKeyByScope[beliefScopeFilter] else {
            return nil
        }
        return BeliefCacheKey(
            scope: beliefScopeFilter,
            entityKey: normalizedBeliefEntityKey(entityKey, scope: beliefScopeFilter)
        )
    }

    /// Applies cached belief rows for the current selection when present.
    private func applyCachedBeliefsForCurrentSelection() {
        guard let key = currentBeliefCacheKey() else {
            manualBeliefs = []
            automaticBeliefs = []
            selectedBeliefID = nil
            return
        }
        guard let cached = beliefSetCache[key] else {
            manualBeliefs = []
            automaticBeliefs = []
            selectedBeliefID = nil
            return
        }
        applyBeliefCacheEntry(cached)
    }

    /// Publishes cached manual/automatic rows.
    private func applyBeliefCacheEntry(_ cached: CachedBeliefSet) {
        manualBeliefs = cached.manualBeliefs
        automaticBeliefs = cached.automaticBeliefs
        ensureBeliefSelection()
    }

    /// Invalidates one belief cache entry or all belief caches.
    private func invalidateBeliefCaches(for key: BeliefCacheKey? = nil) {
        if let key {
            beliefSetCache.removeValue(forKey: key)
        } else {
            beliefSetCache.removeAll()
        }
        cachedBeliefSetSummaries = nil
    }

    /// Whether cached belief rows are still fresh enough for instant reuse.
    private func isBeliefCacheFresh(_ loadedAt: Date) -> Bool {
        Date().timeIntervalSince(loadedAt) < Self.beliefCacheFreshnessInterval
    }

    /// Entity key used to load beliefs for a focus item.
    private func beliefEntityKey(for item: FocusItem, scope: KnowledgeBeliefScope) -> String {
        normalizedBeliefEntityKey(item.id, scope: scope)
    }

    /// Canonical entity key per belief scope.
    private func normalizedBeliefEntityKey(_ entityKey: String, scope: KnowledgeBeliefScope) -> String {
        switch scope {
        case .global:
            return KnowledgeBeliefScope.globalEntityKey
        case .person:
            return canonicalFocusScopeKey(entityKey, kind: .person)
        case .space:
            return canonicalFocusScopeKey(entityKey, kind: .space)
        }
    }

    /// Targets included in the belief summary index.
    private func beliefSummaryTargets() -> [BeliefSummaryTarget] {
        var targets = [
            BeliefSummaryTarget(
                scope: .global,
                entityKey: KnowledgeBeliefScope.globalEntityKey,
                title: "Global Beliefs"
            )
        ]
        targets.append(contentsOf: beliefTargetOptions(for: .person).map { option in
            BeliefSummaryTarget(scope: .person, entityKey: option.entityKey, title: option.title)
        })
        targets.append(contentsOf: beliefTargetOptions(for: .space).map { option in
            BeliefSummaryTarget(scope: .space, entityKey: option.entityKey, title: option.title)
        })
        return targets
    }

    /// Fingerprint used to reuse belief summary cache entries.
    private func beliefSummaryTargetFingerprint(_ targets: [BeliefSummaryTarget]) -> String {
        targets
            .map { "\($0.scope.rawValue):\($0.entityKey):\($0.title)" }
            .joined(separator: "|")
    }

    /// Sorts summary cards by scope priority and recency.
    private func sortedBeliefSetSummaries(_ summaries: [BeliefSetSummary]) -> [BeliefSetSummary] {
        summaries.sorted { left, right in
            let leftRank = Self.beliefScopeRank(left.scope)
            let rightRank = Self.beliefScopeRank(right.scope)
            if leftRank != rightRank {
                return leftRank > rightRank
            }
            let leftDate = parsedTimestamp(left.updatedAt) ?? Date.distantPast
            let rightDate = parsedTimestamp(right.updatedAt) ?? Date.distantPast
            if leftDate != rightDate {
                return leftDate > rightDate
            }
            return left.title.localizedCaseInsensitiveCompare(right.title) == .orderedAscending
        }
    }

    /// Loads belief rows off the main actor.
    nonisolated private static func loadBeliefSet(
        configuration: RuntimeConfiguration,
        key: BeliefCacheKey
    ) async throws -> CachedBeliefSet {
        try await Task.detached(priority: .userInitiated) {
            let store = KnowledgeStore(configuration: configuration)
            try store.bootstrap()
            return CachedBeliefSet(
                manualBeliefs: try store.loadManualBeliefs(scope: key.scope.rawValue, entityKey: key.entityKey),
                automaticBeliefs: try store.loadAutomaticBeliefs(scope: key.scope.rawValue, entityKey: key.entityKey),
                loadedAt: Date()
            )
        }.value
    }

    /// Loads belief summary counts off the main actor.
    nonisolated private static func loadBeliefSetSummaries(
        configuration: RuntimeConfiguration,
        targets: [BeliefSummaryTarget]
    ) async throws -> [BeliefSetSummary] {
        try await Task.detached(priority: .userInitiated) {
            let store = KnowledgeStore(configuration: configuration)
            try store.bootstrap()
            return try targets.compactMap { target in
                let manual = try store.loadManualBeliefs(scope: target.scope.rawValue, entityKey: target.entityKey)
                let automatic = try store.loadAutomaticBeliefs(scope: target.scope.rawValue, entityKey: target.entityKey)
                let manualCount = manual.count
                let autoCount = automatic.count
                guard manualCount + autoCount > 0 else {
                    return nil
                }
                let updatedAt = (manual + automatic).map(\.updatedAt).max() ?? ""
                return BeliefSetSummary(
                    scope: target.scope,
                    entityKey: target.entityKey,
                    title: target.title,
                    autoCount: autoCount,
                    manualCount: manualCount,
                    updatedAt: updatedAt
                )
            }
        }.value
    }

    /// Builds one belief summary from the currently loaded knowledge store.
    private func makeBeliefSetSummary(
        scope: KnowledgeBeliefScope,
        entityKey: String,
        title: String
    ) throws -> BeliefSetSummary? {
        let manual = try knowledgeStore.loadManualBeliefs(scope: scope.rawValue, entityKey: entityKey)
        let automatic = try knowledgeStore.loadAutomaticBeliefs(scope: scope.rawValue, entityKey: entityKey)
        let manualCount = manual.count
        let autoCount = automatic.count
        guard manualCount + autoCount > 0 else {
            return nil
        }
        let updatedAt = (manual + automatic)
            .map(\.updatedAt)
            .max { left, right in
                let leftDate = parsedTimestamp(left) ?? Date.distantPast
                let rightDate = parsedTimestamp(right) ?? Date.distantPast
                return leftDate < rightDate
            } ?? ""

        return BeliefSetSummary(
            scope: scope,
            entityKey: entityKey,
            title: title,
            autoCount: autoCount,
            manualCount: manualCount,
            updatedAt: updatedAt
        )
    }

    /// Resolves the Ask Codex context for the selected scope.
    private func targetContext(for scope: AskCodexTargetScope) -> AskCodexTargetContext? {
        switch scope {
        case .allTracked:
            var lines: [String] = []
            lines.append("Scope: all tracked targets")
            lines.append("Tracked spaces: \(spaceCache.items.count)")
            lines.append("Tracked people: \(personCache.items.count)")
            if !spaceCache.updatedAt.isEmpty {
                lines.append("Space cache updated at: \(spaceCache.updatedAt)")
            }
            if !personCache.updatedAt.isEmpty {
                lines.append("Person cache updated at: \(personCache.updatedAt)")
            }
            if !spaceCache.items.isEmpty {
                lines.append("")
                lines.append("Space evidence:")
                for item in spaceCache.items.prefix(AskCodexContextLimits.allTracked.maxItemsPerKind) {
                    lines.append(contentsOf: askCodexItemContextLines(
                        item: item,
                        kind: .space,
                        limits: .allTracked,
                        includeScopeHeader: false
                    ))
                }
            }
            if !personCache.items.isEmpty {
                lines.append("")
                lines.append("Person evidence:")
                for item in personCache.items.prefix(AskCodexContextLimits.allTracked.maxItemsPerKind) {
                    lines.append(contentsOf: askCodexItemContextLines(
                        item: item,
                        kind: .person,
                        limits: .allTracked,
                        includeScopeHeader: false
                    ))
                }
            }
            let questions = askCodexQuestionContextLines(limit: AskCodexContextLimits.allTracked.maxQuestions)
            if !questions.isEmpty {
                lines.append("")
                lines.append(contentsOf: questions)
            }
            let beliefs = askCodexBeliefSummaryContextLines(limit: 20)
            if !beliefs.isEmpty {
                lines.append("")
                lines.append(contentsOf: beliefs)
            }
            return AskCodexTargetContext(
                key: "all-tracked",
                title: "All Tracked Targets",
                itemID: nil,
                lines: lines
            )
        case .selectedSpace:
            guard let item = selectedItem(for: .space) else {
                return nil
            }
            return AskCodexTargetContext(
                key: "space-\(safeKey(item.id))",
                title: "Space: \(item.title)",
                itemID: item.id,
                lines: askCodexSelectedItemContextLines(item: item, kind: .space)
            )
        case .selectedPerson:
            guard let item = selectedItem(for: .person) else {
                return nil
            }
            return AskCodexTargetContext(
                key: "person-\(safeKey(item.id))",
                title: "Person: \(item.title)",
                itemID: item.id,
                lines: askCodexSelectedItemContextLines(item: item, kind: .person)
            )
        }
    }

    /// Context lines for one selected focus item plus scoped questions.
    private func askCodexSelectedItemContextLines(item: FocusItem, kind: FocusKind) -> [String] {
        var lines = askCodexItemContextLines(
            item: item,
            kind: kind,
            limits: .selected,
            includeScopeHeader: true
        )
        let questions = askCodexQuestionContextLines(
            for: kind,
            itemID: item.id,
            limit: AskCodexContextLimits.selected.maxQuestions
        )
        if !questions.isEmpty {
            lines.append("")
            lines.append(contentsOf: questions)
        }
        return lines
    }

    /// Converts one focus item into bounded prompt context.
    private func askCodexItemContextLines(
        item: FocusItem,
        kind: FocusKind,
        limits: AskCodexContextLimits,
        includeScopeHeader: Bool
    ) -> [String] {
        var lines: [String] = []
        if includeScopeHeader {
            lines.append("Scope: \(kind == .space ? "selected space" : "selected person")")
            lines.append("Title: \(item.title)")
        } else {
            lines.append("- \(kind == .space ? "Space" : "Person"): \(item.title)")
        }
        if !item.subtitle.isEmpty {
            lines.append("Subtitle: \(item.subtitle)")
        }
        if !item.meta.isEmpty {
            lines.append("Meta: \(item.meta)")
        }
        if !item.timestamp.isEmpty {
            lines.append("Timestamp: \(item.timestamp)")
        }
        if !item.statusBadge.isEmpty {
            lines.append("Status: \(item.statusBadge)")
        }
        let introLines = limitedCleanLines(item.detailIntroLines, limit: limits.maxIntroLines)
        if !introLines.isEmpty {
            lines.append("Summary and posture:")
            lines.append(contentsOf: introLines.map { "- \($0)" })
        }
        let detailLines = item.detailLines
            .map { $0.trimmingCharacters(in: .whitespacesAndNewlines) }
            .filter { !$0.isEmpty }
        if !detailLines.isEmpty {
            let limited = Array(detailLines.prefix(limits.maxDetailLines))
            lines.append("Raw/detail evidence:")
            lines.append(contentsOf: limited.map { "- \($0)" })
            if detailLines.count > limits.maxDetailLines {
                lines.append("- ... truncated \(detailLines.count - limits.maxDetailLines) additional raw/detail lines")
            }
        }
        let sections = item.detailSections.filter { !$0.lines.isEmpty }
        if !sections.isEmpty {
            lines.append("Generated/recent sections:")
            for section in sections.prefix(limits.maxSections) {
                lines.append("- Section: \(section.header)")
                let sectionLines = limitedCleanLines(section.lines, limit: limits.maxSectionLines)
                lines.append(contentsOf: sectionLines.map { "  - \($0)" })
                if section.lines.count > limits.maxSectionLines {
                    lines.append("  - ... truncated \(section.lines.count - limits.maxSectionLines) additional section lines")
                }
            }
            if sections.count > limits.maxSections {
                lines.append("- ... truncated \(sections.count - limits.maxSections) additional sections")
            }
        }
        let tailLines = limitedCleanLines(item.detailTailLines, limit: limits.maxTailLines)
        if !tailLines.isEmpty {
            lines.append("Additional context:")
            lines.append(contentsOf: tailLines.map { "- \($0)" })
            if item.detailTailLines.count > limits.maxTailLines {
                lines.append("- ... truncated \(item.detailTailLines.count - limits.maxTailLines) additional context lines")
            }
        }
        return lines
    }

    /// Question context for all tracked Ask Codex scope.
    private func askCodexQuestionContextLines(limit: Int) -> [String] {
        let questions = sortedAskCodexQuestionCandidates(questionCandidates)
            .prefix(limit)
        guard !questions.isEmpty else {
            return []
        }
        var lines = ["Open questions from Questions engine:"]
        for question in questions {
            lines.append(contentsOf: askCodexQuestionLines(question))
        }
        return lines
    }

    /// Question context for one selected focus item.
    private func askCodexQuestionContextLines(for kind: FocusKind, itemID: String, limit: Int) -> [String] {
        let questions = questionCandidates(for: kind, itemID: itemID, limit: limit)
        guard !questions.isEmpty else {
            return []
        }
        var lines = ["Open questions for this target:"]
        for question in questions {
            lines.append(contentsOf: askCodexQuestionLines(question))
        }
        return lines
    }

    /// Prompt lines for one question candidate.
    private func askCodexQuestionLines(_ question: QuestionCandidate) -> [String] {
        var lines = [
            "- \(question.scopeType.title): \(question.scopeLabel) — \(question.questionText)",
            "  Why now: \(question.whyNow)",
            "  Priority: \(Int(question.priorityScore.rounded())) | Status: \(question.status.title) | Source: \(question.sourceKind)"
        ]
        let evidence = question.evidence.prefix(3)
        if !evidence.isEmpty {
            lines.append("  Evidence:")
            lines.append(contentsOf: evidence.map { ref in
                let preview = ref.preview.trimmingCharacters(in: .whitespacesAndNewlines)
                return "    - \(ref.label): \(preview)"
            })
        }
        return lines
    }

    /// Active Ask Codex questions sorted by priority.
    private func sortedAskCodexQuestionCandidates(_ candidates: [QuestionCandidate]) -> [QuestionCandidate] {
        candidates
            .filter { activeQuestionStatuses.contains($0.status) }
            .sorted { left, right in
                if left.priorityScore == right.priorityScore {
                    return left.updatedAt > right.updatedAt
                }
                return left.priorityScore > right.priorityScore
            }
    }

    /// Belief summary context for all-tracked Ask Codex scope.
    private func askCodexBeliefSummaryContextLines(limit: Int) -> [String] {
        let summaries = sortedBeliefSetSummaries(beliefSetSummaries).prefix(limit)
        guard !summaries.isEmpty else {
            return []
        }
        var lines = ["Belief set summaries currently loaded:"]
        for summary in summaries {
            lines.append("- \(summary.scope.rawValue): \(summary.title) — auto=\(summary.autoCount), manual=\(summary.manualCount), updated=\(summary.updatedAt)")
        }
        return lines
    }

    /// Trims and bounds a list of prompt lines.
    private func limitedCleanLines(_ values: [String], limit: Int) -> [String] {
        values
            .map { $0.trimmingCharacters(in: .whitespacesAndNewlines) }
            .filter { !$0.isEmpty }
            .prefix(max(0, limit))
            .map { String($0) }
    }

    /// Final Ask Codex prompt assembled from question and local context.
    private func askCodexPrompt(
        question: String,
        targetContext: AskCodexTargetContext,
        promptVersion: String
    ) -> String {
        let contextLines = targetContext.lines.joined(separator: "\n")
        return """
        Prompt Version: \(promptVersion)
        Task: Answer the operator question using the provided Webex intelligence context.
        Output format: Markdown.

        Rules:
        - Use only the supplied context. Treat the context as the current local Webex intelligence snapshot.
        - If the context is insufficient, say exactly what is missing.
        - Prefer the newest timestamps, current posture/status lines, open questions, and recent conversation evidence.
        - Include concise, actionable bullets.

        Target:
        \(targetContext.title)

        Question:
        \(question)

        Context:
        \(contextLines)
        """
    }

    /// Job metadata and output locations for one Ask Codex run.
    private func askCodexJob(
        targetContext: AskCodexTargetContext,
        promptVersion: String,
        createdAt: Date
    ) -> CodexPromptJob {
        let jobID = "ask-codex-\(targetContext.key)-\(Int(createdAt.timeIntervalSince1970))"
        let baseURL = runtimeStore.configuration.runtimeRoot
            .appendingPathComponent("knowledge", isDirectory: true)
            .appendingPathComponent("codex", isDirectory: true)
            .appendingPathComponent("jobs", isDirectory: true)
            .appendingPathComponent("ask-codex", isDirectory: true)
            .appendingPathComponent(safeKey(targetContext.key), isDirectory: true)
            .appendingPathComponent(jobID, isDirectory: true)

        return CodexPromptJob(
            id: jobID,
            title: "Ask Codex: \(targetContext.title)",
            promptVersion: promptVersion,
            promptURL: baseURL.appendingPathComponent("prompt.md"),
            outputURL: baseURL.appendingPathComponent("output.txt"),
            logURL: baseURL.appendingPathComponent("run.log"),
            metadataURL: baseURL.appendingPathComponent("run-metadata.json"),
            status: .pending,
            createdAt: createdAt
        )
    }

    private func safeKey(_ value: String) -> String {
        let allowed = Set("abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ0123456789-_")
        let cleaned = value.map { allowed.contains($0) ? $0 : "-" }
        let normalized = String(cleaned).trimmingCharacters(in: CharacterSet(charactersIn: "-"))
        return normalized.isEmpty ? "target" : normalized
    }

    /// Rewrites space titles to configured labels when cache titles drift.
    private func canonicalizeSpaceTitles(_ cache: FocusCache) -> FocusCache {
        guard let targets = try? configStore.importantSpaces(),
              !targets.isEmpty else {
            return cache
        }

        let configuredPairs: [(String, String)] = targets.compactMap { target in
                let roomID = normalizedRoomID(target.roomID)
                let label = target.label.trimmingCharacters(in: .whitespacesAndNewlines)
                guard !roomID.isEmpty, !label.isEmpty else {
                    return nil
                }
                return (roomID, label)
        }
        let configuredLabelByRoomID = Dictionary(uniqueKeysWithValues: configuredPairs)
        guard !configuredLabelByRoomID.isEmpty else {
            return cache
        }

        let items = cache.items.map { item in
            let roomID = normalizedRoomID(roomIDForSpaceItem(item))
            guard let configuredTitle = configuredLabelByRoomID[roomID],
                  !configuredTitle.isEmpty,
                  configuredTitle != item.title else {
                return item
            }

            let detailLines = item.detailLines.map { line -> String in
                if line.hasPrefix("Space Name:") {
                    return "Space Name: \(configuredTitle)"
                }
                return line
            }

            let detailSections = item.detailSections.map { section in
                var updated = section
                if !updated.roomTitle.trimmingCharacters(in: .whitespacesAndNewlines).isEmpty {
                    updated.roomTitle = configuredTitle
                }
                return updated
            }

            return FocusItem(
                id: item.id,
                title: configuredTitle,
                subtitle: item.subtitle,
                meta: item.meta,
                timestamp: item.timestamp,
                badge: item.badge,
                statusBadge: item.statusBadge,
                detailLines: detailLines,
                detailIntroLines: item.detailIntroLines,
                detailSections: detailSections,
                detailTailLines: item.detailTailLines
            )
        }

        return FocusCache(
            focusDays: cache.focusDays,
            items: items,
            updatedAt: cache.updatedAt,
            countLabel: cache.countLabel,
            recentMessages: cache.recentMessages,
            summaryGenerationInProgress: cache.summaryGenerationInProgress,
            subjectsProcessed: cache.subjectsProcessed,
            subjectsTotal: cache.subjectsTotal
        )
    }

    /// Resolves the room ID from cache IDs or legacy detail lines.
    private func roomIDForSpaceItem(_ item: FocusItem) -> String {
        let trimmedID = item.id.trimmingCharacters(in: .whitespacesAndNewlines)
        if let range = trimmedID.range(of: "spacefocus:", options: [.anchored, .caseInsensitive]) {
            return String(trimmedID[range.upperBound...])
        }

        let detailRoomID = item.firstDetailLine(prefix: "Room ID:")
        if !detailRoomID.isEmpty {
            return detailRoomID
        }
        return trimmedID
    }

    private func normalizedRoomID(_ value: String) -> String {
        value
            .trimmingCharacters(in: .whitespacesAndNewlines)
            .replacingOccurrences(of: #"\s+"#, with: "", options: .regularExpression)
    }

    private func normalizedEmail(_ value: String) -> String {
        value.trimmingCharacters(in: .whitespacesAndNewlines).lowercased()
    }

    private static func iso8601Timestamp(_ date: Date) -> String {
        let formatter = ISO8601DateFormatter()
        formatter.formatOptions = [.withInternetDateTime, .withFractionalSeconds]
        return formatter.string(from: date)
    }

    private static func beliefScopeRank(_ scope: KnowledgeBeliefScope) -> Int {
        switch scope {
        case .global:
            return 2
        case .person:
            return 1
        case .space:
            return 0
        }
    }

    /// Sorts visible focus items according to the current per-kind preference.
    private func sortItems(_ items: [FocusItem], for kind: FocusKind) -> [FocusItem] {
        switch sortOption(for: kind) {
        case .latestMessage:
            return items.sorted { left, right in
                let leftDate = parsedTimestamp(left.timestamp) ?? .distantPast
                let rightDate = parsedTimestamp(right.timestamp) ?? .distantPast
                if leftDate != rightDate {
                    return leftDate > rightDate
                }
                let titleComparison = left.title.localizedCaseInsensitiveCompare(right.title)
                if titleComparison != .orderedSame {
                    return titleComparison == .orderedAscending
                }
                return left.id < right.id
            }
        case .name:
            return items.sorted { left, right in
                let titleComparison = left.title.localizedCaseInsensitiveCompare(right.title)
                if titleComparison != .orderedSame {
                    return titleComparison == .orderedAscending
                }
                let leftDate = parsedTimestamp(left.timestamp) ?? .distantPast
                let rightDate = parsedTimestamp(right.timestamp) ?? .distantPast
                if leftDate != rightDate {
                    return leftDate > rightDate
                }
                return left.id < right.id
            }
        }
    }

    /// Parses current and legacy timestamp formats emitted by runtime scripts.
    private func parsedTimestamp(_ value: String) -> Date? {
        let trimmed = value.trimmingCharacters(in: .whitespacesAndNewlines)
        guard !trimmed.isEmpty else {
            return nil
        }
        if let date = Self.iso8601WithFractionalSeconds.date(from: trimmed) {
            return date
        }
        if let date = Self.iso8601WithoutFractionalSeconds.date(from: trimmed) {
            return date
        }
        if let date = Self.legacyTimestampFormatter.date(from: trimmed) {
            return date
        }
        return Self.legacyMinuteTimestampFormatter.date(from: trimmed)
    }

    private static let iso8601WithFractionalSeconds: ISO8601DateFormatter = {
        let formatter = ISO8601DateFormatter()
        formatter.formatOptions = [.withInternetDateTime, .withFractionalSeconds]
        return formatter
    }()

    private static let iso8601WithoutFractionalSeconds: ISO8601DateFormatter = {
        let formatter = ISO8601DateFormatter()
        formatter.formatOptions = [.withInternetDateTime]
        return formatter
    }()

    private static let legacyTimestampFormatter: DateFormatter = {
        let formatter = DateFormatter()
        formatter.locale = Locale(identifier: "en_US_POSIX")
        formatter.dateFormat = "yyyy-MM-dd HH:mm:ss z"
        return formatter
    }()

    private static let legacyMinuteTimestampFormatter: DateFormatter = {
        let formatter = DateFormatter()
        formatter.locale = Locale(identifier: "en_US_POSIX")
        formatter.dateFormat = "yyyy-MM-dd HH:mm z"
        return formatter
    }()

    /// Marks runtime access failures and stops background work when permissions are denied.
    private func handleRuntimeAccessError(_ error: Error) {
        let message = error.localizedDescription
        errorMessage = message
        guard isRuntimeAccessDeniedError(message) else {
            return
        }
        runtimeAccessDenied = true
        stopBackgroundRefreshLoop()
    }

    /// Stops background refresh after runtime access becomes unavailable.
    private func stopBackgroundRefreshLoop() {
        refreshLoopTask?.cancel()
        refreshLoopTask = nil
        backgroundRefreshActive = false
    }

    /// Detects macOS/TCC-style filesystem permission failures.
    private func isRuntimeAccessDeniedError(_ message: String) -> Bool {
        let normalized = message.lowercased()
        return normalized.contains("authorization denied")
            || normalized.contains("operation not permitted")
            || normalized.contains("permission denied")
            || normalized.contains("not permitted")
    }
}
