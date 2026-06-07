import Foundation
import CryptoKit

/// Prompt contract family for cluster summary/title work.
enum CodexClusterPromptKind: String, CaseIterable, Codable, Hashable {
    case personSummary = "person-cluster-summary"
    case personTitle = "person-cluster-title"
    case spaceTitle = "space-cluster-title"
}

/// Version and expected JSON field for a cluster prompt.
struct CodexClusterPromptContract: Hashable {
    var kind: CodexClusterPromptKind
    var promptVersion: String
    var outputField: String
}

/// Central prompt-version registry for Codex-generated artifacts.
enum CodexPromptVersionRegistry {
    static let personFocusClusterSummary = "person-focus-cluster-summary-v10"
    static let personFocusClusterTitle = "person-focus-cluster-title-v1"
    static let spaceFocusSummary = "space-focus-summary-v5"
    static let spaceFocusClusterTitle = "space-focus-cluster-title-v1"
    static let spaceFocusExecQuestions = "space-focus-exec-questions-v1"
    static let questionSynthesis = "question-synthesis-v1"
    static let beliefReconciliation = "belief-reconciliation-v2"
    static let beliefTaskType = "TASK_TYPE_BELIEF_RECONCILIATION"
    static let clusterPromptContracts: [CodexClusterPromptKind: CodexClusterPromptContract] = [
        .personSummary: CodexClusterPromptContract(
            kind: .personSummary,
            promptVersion: personFocusClusterSummary,
            outputField: "summary"
        ),
        .personTitle: CodexClusterPromptContract(
            kind: .personTitle,
            promptVersion: personFocusClusterTitle,
            outputField: "title"
        ),
        .spaceTitle: CodexClusterPromptContract(
            kind: .spaceTitle,
            promptVersion: spaceFocusClusterTitle,
            outputField: "title"
        )
    ]
}

/// Cache freshness policy for Codex prompt outputs.
struct CodexPromptCachePolicy: Hashable {
    var maxAgeSeconds: TimeInterval

    static let summary = CodexPromptCachePolicy(maxAgeSeconds: 15 * 60)
    static let execQuestions = CodexPromptCachePolicy(maxAgeSeconds: 15 * 60)

    func applying(maxAgeSeconds: Double?) -> CodexPromptCachePolicy {
        guard let maxAgeSeconds else { return self }
        return CodexPromptCachePolicy(maxAgeSeconds: min(max(TimeInterval(maxAgeSeconds), 1), 86_400))
    }
}

/// Conversation cluster passed into space/exec prompts.
struct SpaceConversationCluster: Codable, Hashable {
    var title: String
    var summary: String
    var openLoops: [String]
}

/// Input context for a space summary prompt.
struct SpaceSummaryContext: Hashable {
    var roomID: String
    var roomTitle: String
    var clusters: [SpaceConversationCluster]
    var openLoops: [String]
    var previousSummary: String?
    var previousGeneratedAt: String?
}

/// Meaningful topic extracted from a space summary.
struct SpaceSummaryTopic: Codable, Hashable {
    var title: String
    var summary: String
    var soWhat: String
}

/// Persisted Codex space summary output.
struct SpaceSummaryResult: Codable, Hashable {
    var roomID: String
    var summary: String
    var openLoops: [String]
    var topics: [SpaceSummaryTopic]
    var source: String
    var generatedAt: String
    var promptVersion: String
    var inputHash: String

    enum CodingKeys: String, CodingKey {
        case roomID
        case summary
        case openLoops
        case topics
        case source
        case generatedAt
        case promptVersion
        case inputHash
    }

    /// Creates a normalized space summary result.
    init(
        roomID: String,
        summary: String,
        openLoops: [String],
        topics: [SpaceSummaryTopic],
        source: String,
        generatedAt: String,
        promptVersion: String,
        inputHash: String
    ) {
        self.roomID = roomID
        self.summary = summary
        self.openLoops = openLoops
        self.topics = topics
        self.source = source
        self.generatedAt = generatedAt
        self.promptVersion = promptVersion
        self.inputHash = inputHash
    }

    /// Decodes older cache entries that may omit open loops.
    init(from decoder: Decoder) throws {
        let container = try decoder.container(keyedBy: CodingKeys.self)
        roomID = try container.decode(String.self, forKey: .roomID)
        summary = try container.decode(String.self, forKey: .summary)
        openLoops = try container.decodeIfPresent([String].self, forKey: .openLoops) ?? []
        topics = try container.decode([SpaceSummaryTopic].self, forKey: .topics)
        source = try container.decode(String.self, forKey: .source)
        generatedAt = try container.decode(String.self, forKey: .generatedAt)
        promptVersion = try container.decode(String.self, forKey: .promptVersion)
        inputHash = try container.decode(String.self, forKey: .inputHash)
    }
}

/// Focus kind used by cluster title prompts.
enum ClusterFocusKind: String, Codable, Hashable {
    case person
    case space
}

/// Input context for a person cluster summary prompt.
struct PersonClusterSummaryContext: Hashable {
    var personID: String
    var personLabel: String
    var conversationHighlights: [String]
    var existingSummary: String?
}

/// Persisted Codex person cluster summary output.
struct PersonClusterSummaryResult: Codable, Hashable {
    var personID: String
    var personLabel: String
    var summary: String
    var source: String
    var generatedAt: String
    var promptVersion: String
    var inputHash: String
}

/// Input context for a cluster title prompt.
struct ClusterTitleContext: Hashable {
    var focusKind: ClusterFocusKind
    var clusterID: String
    var clusterSummary: String
    var supportingSignals: [String]
    var existingTitle: String?
}

/// Persisted Codex cluster title output.
struct ClusterTitleResult: Codable, Hashable {
    var focusKind: ClusterFocusKind
    var clusterID: String
    var title: String
    var source: String
    var generatedAt: String
    var promptVersion: String
    var inputHash: String
}

/// Input context for executive-question generation.
struct ExecQuestionsContext: Hashable {
    var roomID: String
    var roomTitle: String
    var execName: String
    var execEmail: String
    var summary: String
    var openLoops: [String]
    var clusters: [SpaceConversationCluster]
    var execBeliefs: [BeliefSnapshot]
}

/// Room participant used to match executives.
struct SpaceParticipant: Hashable {
    var name: String
    var email: String
}

/// Executive identity loaded from config.
struct ImportantExecutive: Hashable {
    var name: String
    var email: String
}

/// Persisted Codex executive-question output.
struct ExecQuestionsResult: Codable, Hashable {
    var roomID: String
    var execName: String
    var header: String
    var questions: [String]
    var source: String
    var generatedAt: String
    var promptVersion: String
    var inputHash: String
}

/// Draft question produced by Codex before store normalization.
struct CodexQuestionSynthesisDraft: Codable, Hashable {
    var scopeType: QuestionScopeType
    var scopeKey: String
    var scopeLabel: String
    var questionText: String
    var whyNow: String
    var tags: [String]
    var priorityScore: Double?
    var evidenceSourceIDs: [String]

    enum CodingKeys: String, CodingKey {
        case scopeType = "scope_type"
        case scopeKey = "scope_key"
        case scopeLabel = "scope_label"
        case questionText = "question"
        case whyNow = "why_now"
        case tags
        case priorityScore = "priority_score"
        case evidenceSourceIDs = "evidence_source_ids"
    }
}

/// Persisted Codex question-synthesis output.
struct CodexQuestionSynthesisResult: Codable, Hashable {
    var questions: [CodexQuestionSynthesisDraft]
    var source: String
    var generatedAt: String
    var promptVersion: String
    var inputHash: String
}

/// Belief reconciliation scope used in prompt contracts.
enum BeliefScope: String, Codable, Hashable {
    case global
    case person
    case space

    static let globalEntityKey = "__global__"
}

/// Current belief snapshot included in reconciliation prompts.
struct BeliefSnapshot: Codable, Hashable {
    var statement: String
    var confidence: Double
    var evidenceLinks: [String]
    var beliefKind: String = "second_order"
    var lifecycle: String = "candidate"
    var supportCount: Int = 1
    var contradictionCount: Int = 0
    var lastEvidenceAt: String = ""
}

/// Evidence row included in reconciliation prompts.
struct BeliefEvidence: Codable, Hashable {
    var id: String
    var text: String
    var source: String
    var occurredAt: String
}

/// Input context for one belief reconciliation run.
struct BeliefReconciliationContext: Hashable {
    var scope: BeliefScope
    var entityKey: String
    var currentBeliefs: [BeliefSnapshot]
    var manualBeliefs: [BeliefSnapshot]
    var evidence: [BeliefEvidence]
    var forceRefresh: Bool
    var incrementalWindowDays: Int
    var chunkIndex: Int?
    var chunkCount: Int?
}

/// Persisted reconciliation checkpoint for one scope/entity.
struct BeliefReconciliationState: Codable, Hashable {
    var lastRunAt: String?
    var lastEvidenceHash: String?
}

/// Reason reconciliation did or did not run.
enum BeliefReconciliationTriggerReason: String, Codable, Hashable {
    case firstRun
    case forced
    case evidenceChanged
    case stale
    case upToDate
    case invalidScopeEntityKey
}

/// Run decision plus evidence hash and staleness marker.
struct BeliefReconciliationDecision: Codable, Hashable {
    var shouldRun: Bool
    var reason: BeliefReconciliationTriggerReason
    var evidenceHash: String
    var staleAt: String?
}

/// Belief mutation proposed by Codex.
struct BeliefChange: Codable, Hashable {
    var statement: String
    var confidence: Double
    var evidenceLinks: [String]
    var beliefKind: String = "second_order"
    var lifecycle: String = "candidate"
    var supportCount: Int = 1
    var contradictionCount: Int = 0
    var lastEvidenceAt: String = ""
}

/// Parsed Codex reconciliation result.
struct BeliefReconciliationResult: Codable, Hashable {
    var taskType: String
    var scope: BeliefScope
    var entityKey: String
    var promptVersion: String
    var decision: BeliefReconciliationDecision
    var beliefsToAdd: [BeliefChange]
    var beliefsToUpdate: [BeliefChange]
    var beliefsToWeaken: [BeliefChange]
    var confidence: Double
    var evidenceLinks: [String]
    var generatedAt: String
    var inputHash: String
    var source: String
}

/// Reconciliation decision plus optional Codex result and next checkpoint.
struct BeliefReconciliationRunOutcome: Hashable {
    var decision: BeliefReconciliationDecision
    var result: BeliefReconciliationResult?
    var nextState: BeliefReconciliationState
}

/// Stable key for reconciliation state.
struct BeliefReconciliationScopeKey: Codable, Hashable {
    var scope: BeliefScope
    var entityKey: String
}

/// Reconciliation target with current state.
struct BeliefReconciliationTarget: Hashable {
    var context: BeliefReconciliationContext
    var state: BeliefReconciliationState
}

/// Outcome for one reconciliation target.
struct BeliefReconciliationTargetOutcome: Hashable {
    var key: BeliefReconciliationScopeKey
    var outcome: BeliefReconciliationRunOutcome
}

/// Batched reconciliation results plus trigger counts.
struct BeliefReconciliationBatchOutcome: Hashable {
    var outcomes: [BeliefReconciliationTargetOutcome]
    var triggeredCount: Int
    var skippedCount: Int
}

/// Error thrown when a requested Codex feature is disabled.
struct CodexFeatureDisabledError: LocalizedError {
    var feature: CodexFeatureToggle

    var errorDescription: String? {
        "\(feature.displayName) is disabled in Settings."
    }
}

/// Builds prompts, runs Codex, parses outputs, and manages prompt caches.
final class CodexPromptOrchestrationService: QuestionCandidateSynthesizing {
    let configuration: RuntimeConfiguration
    let runner: CodexRunner
    private let configStore: ConfigStore
    private let decoder: JSONDecoder
    private let encoder: JSONEncoder

    /// Injects runner/config and sets stable JSON output formatting.
    init(
        configuration: RuntimeConfiguration = .current,
        runner: CodexRunner? = nil,
        configStore: ConfigStore? = nil
    ) {
        self.configuration = configuration
        self.runner = runner ?? CodexRunner(configuration: configuration)
        self.configStore = configStore ?? ConfigStore(configuration: configuration)
        self.decoder = JSONDecoder()
        self.encoder = JSONEncoder()
        self.encoder.outputFormatting = [.prettyPrinted, .sortedKeys]
    }

    private func codexFeatureEnabled(_ feature: CodexFeatureToggle) -> Bool {
        configStore.loadSystemSettings()
            .codexFeatureEnabled(feature)
    }

    private func requireCodexFeature(_ feature: CodexFeatureToggle) throws {
        guard codexFeatureEnabled(feature) else {
            throw CodexFeatureDisabledError(feature: feature)
        }
    }

    private func configuredCodexDocument() -> MacAppJSONCodexConfiguration? {
        configuration.jsonConfiguration?.codex
    }

    private func configuredCodexRunPolicy(overrides: MacAppJSONRunPolicy? = nil) -> CodexRunPolicy {
        CodexRunPolicy.default
            .applying(configuredCodexDocument()?.runPolicy)
            .applying(overrides)
    }

    private func configuredSummaryCachePolicy() -> CodexPromptCachePolicy {
        CodexPromptCachePolicy.summary.applying(
            maxAgeSeconds: configuredCodexDocument()?.cachePolicy?.summaryMaxAgeSeconds
        )
    }

    private func configuredExecQuestionsCachePolicy() -> CodexPromptCachePolicy {
        CodexPromptCachePolicy.execQuestions.applying(
            maxAgeSeconds: configuredCodexDocument()?.cachePolicy?.execQuestionsMaxAgeSeconds
        )
    }

    private func codexQuestionSynthesisPolicy() -> MacAppJSONCodexQuestionSynthesisPolicy? {
        configuredCodexDocument()?.questionSynthesis
    }

    private func configuredBeliefStaleHours(_ staleHours: Int) -> Int {
        guard staleHours == 24 else {
            return max(1, staleHours)
        }
        return configuredInt(
            configuredCodexDocument()?.beliefs?.staleHours,
            defaultValue: 24,
            minimum: 1,
            maximum: 8_760
        )
    }

    private func configuredBeliefEvidenceChunkSize() -> Int {
        configuredInt(
            configuredCodexDocument()?.beliefs?.evidenceChunkSize,
            defaultValue: Self.defaultBeliefEvidenceChunkSize,
            minimum: 1,
            maximum: 500
        )
    }

    private func configuredMaxBeliefIncrementalWindowDays() -> Int {
        configuredInt(
            configuredCodexDocument()?.beliefs?.maxIncrementalWindowDays,
            defaultValue: Self.maxBeliefIncrementalWindowDays,
            minimum: 1,
            maximum: 3_650
        )
    }

    private func configuredInt(
        _ value: Int?,
        defaultValue: Int,
        minimum: Int,
        maximum: Int
    ) -> Int {
        guard let value else { return defaultValue }
        return min(max(value, minimum), maximum)
    }

    /// Synthesizes publishable question candidates from deterministic candidates.
    func synthesizeQuestionCandidates(
        from candidates: [QuestionCandidate],
        now: Date = Date()
    ) async throws -> [QuestionCandidate] {
        guard codexFeatureEnabled(.questionSynthesis) else {
            return []
        }
        let synthesisPolicy = codexQuestionSynthesisPolicy()
        let seedCandidateLimit = configuredInt(
            synthesisPolicy?.seedCandidateLimit,
            defaultValue: 40,
            minimum: 1,
            maximum: 200
        )
        let queryHistoryLimit = configuredInt(
            synthesisPolicy?.queryHistoryLimit,
            defaultValue: 40,
            minimum: 0,
            maximum: 500
        )
        let promptHistoryLimit = configuredInt(
            synthesisPolicy?.promptHistoryLimit,
            defaultValue: 24,
            minimum: 0,
            maximum: 100
        )
        let candidateEvidenceLimit = configuredInt(
            synthesisPolicy?.candidateEvidenceLimit,
            defaultValue: 4,
            minimum: 1,
            maximum: 20
        )
        let outputLimit = configuredInt(
            synthesisPolicy?.outputLimit,
            defaultValue: 7,
            minimum: 1,
            maximum: 24
        )

        let seedCandidates = Array(
            candidates
                .filter { $0.status == .candidate || $0.status == .surfaced }
                .prefix(seedCandidateLimit)
        )
        guard !seedCandidates.isEmpty else {
            return []
        }

        let promptVersion = CodexPromptVersionRegistry.questionSynthesis
        let queryHistory = configStore.loadAskCodexQueryHistory(limit: queryHistoryLimit)
        let inputHash = hashForQuestionSynthesisCandidates(
            seedCandidates,
            queryHistory: queryHistory,
            evidenceLimit: candidateEvidenceLimit
        )
        let cacheURL = cacheFileURL(kind: "question-synthesis", key: inputHash)

        let result: CodexQuestionSynthesisResult
        if let cached = try loadCache(url: cacheURL, as: CodexQuestionSynthesisResult.self),
           cached.promptVersion == promptVersion,
           cached.inputHash == inputHash {
            result = cached
        } else {
            let prompt = questionSynthesisPrompt(
                candidates: seedCandidates,
                queryHistory: queryHistory,
                promptVersion: promptVersion,
                historyLimit: promptHistoryLimit,
                candidateEvidenceLimit: candidateEvidenceLimit
            )
            let job = makeJob(
                kind: "question-synthesis",
                key: inputHash,
                title: "Question Synthesis",
                promptVersion: promptVersion,
                createdAt: now
            )
            let output = try await runner.run(
                request: CodexRunRequest(
                    prompt: prompt,
                    job: job,
                    workingDirectory: configuration.runtimeRoot,
                    policy: configuredCodexRunPolicy(overrides: synthesisPolicy?.runPolicy)
                )
            ).output
            let parsed = parseQuestionSynthesisOutput(output, limit: outputLimit)
            result = CodexQuestionSynthesisResult(
                questions: parsed,
                source: "Codex",
                generatedAt: Self.iso8601String(from: now),
                promptVersion: promptVersion,
                inputHash: inputHash
            )
            try persistCache(value: result, url: cacheURL)
        }

        return makeSynthesizedQuestionCandidates(
            from: result,
            seedCandidates: seedCandidates,
            evidenceLimit: candidateEvidenceLimit,
            now: now
        )
    }

    /// Generates or reuses a person cluster summary.
    func generatePersonClusterSummary(
        context: PersonClusterSummaryContext,
        workingDirectory: URL,
        forceRefresh: Bool = false,
        cachePolicy: CodexPromptCachePolicy = .summary,
        now: Date = Date()
    ) async throws -> PersonClusterSummaryResult {
        try requireCodexFeature(.personFocusSummaries)
        let effectiveCachePolicy = cachePolicy == .summary ? configuredSummaryCachePolicy() : cachePolicy
        let promptContract = clusterPromptContract(for: .personSummary)
        let promptVersion = promptContract.promptVersion
        let inputHash = hashForPersonClusterSummaryContext(context)
        let cacheURL = cacheFileURL(kind: "person-cluster-summary", key: context.personID)

        if !forceRefresh,
           let cached = try loadCache(url: cacheURL, as: PersonClusterSummaryResult.self),
           cached.promptVersion == promptVersion,
           cached.inputHash == inputHash,
           isFresh(timestamp: cached.generatedAt, maxAgeSeconds: effectiveCachePolicy.maxAgeSeconds, now: now) {
            var fromCache = cached
            fromCache.source = "Codex cache"
            return fromCache
        }

        let prompt = personClusterSummaryPrompt(context: context, promptVersion: promptVersion)
        let job = makeJob(
            kind: "person-cluster-summary",
            key: context.personID,
            title: "Person Cluster Summary: \(context.personLabel)",
            promptVersion: promptVersion,
            createdAt: now
        )
        let output = try await runner.run(
            request: CodexRunRequest(
                prompt: prompt,
                job: job,
                workingDirectory: workingDirectory,
                policy: configuredCodexRunPolicy()
            )
        ).output

        let parsedSummary = parsePersonClusterSummaryOutput(output, expectedField: promptContract.outputField)
        let result = PersonClusterSummaryResult(
            personID: context.personID,
            personLabel: context.personLabel,
            summary: parsedSummary.isEmpty ? "No summary generated." : parsedSummary,
            source: "Codex",
            generatedAt: Self.iso8601String(from: now),
            promptVersion: promptVersion,
            inputHash: inputHash
        )
        try persistCache(value: result, url: cacheURL)
        return result
    }

    /// Generates or reuses a compact cluster title.
    func generateClusterTitle(
        context: ClusterTitleContext,
        workingDirectory: URL,
        forceRefresh: Bool = false,
        cachePolicy: CodexPromptCachePolicy = .summary,
        now: Date = Date()
    ) async throws -> ClusterTitleResult {
        try requireCodexFeature(.clusterTitles)
        let effectiveCachePolicy = cachePolicy == .summary ? configuredSummaryCachePolicy() : cachePolicy
        let promptContract = clusterPromptContract(for: clusterPromptKindForTitle(context.focusKind))
        let promptVersion = promptContract.promptVersion
        let inputHash = hashForClusterTitleContext(context)
        let cacheKey = "\(context.focusKind.rawValue)-\(context.clusterID)"
        let cacheURL = cacheFileURL(kind: "cluster-title", key: cacheKey)

        if !forceRefresh,
           let cached = try loadCache(url: cacheURL, as: ClusterTitleResult.self),
           cached.promptVersion == promptVersion,
           cached.inputHash == inputHash,
           isFresh(timestamp: cached.generatedAt, maxAgeSeconds: effectiveCachePolicy.maxAgeSeconds, now: now) {
            var fromCache = cached
            fromCache.source = "Codex cache"
            return fromCache
        }

        let prompt = clusterTitlePrompt(context: context, promptVersion: promptVersion)
        let job = makeJob(
            kind: "cluster-title",
            key: cacheKey,
            title: "Cluster Title (\(context.focusKind.rawValue)): \(context.clusterID)",
            promptVersion: promptVersion,
            createdAt: now
        )
        let output = try await runner.run(
            request: CodexRunRequest(
                prompt: prompt,
                job: job,
                workingDirectory: workingDirectory,
                policy: configuredCodexRunPolicy()
            )
        ).output

        let parsedTitle = parseClusterTitleOutput(output, expectedField: promptContract.outputField)
        let title = parsedTitle.isEmpty ? "Untitled Cluster" : parsedTitle
        let result = ClusterTitleResult(
            focusKind: context.focusKind,
            clusterID: context.clusterID,
            title: title,
            source: "Codex",
            generatedAt: Self.iso8601String(from: now),
            promptVersion: promptVersion,
            inputHash: inputHash
        )
        try persistCache(value: result, url: cacheURL)
        return result
    }

    /// Generates or reuses a room-level space summary.
    func generateSpaceSummary(
        context: SpaceSummaryContext,
        workingDirectory: URL,
        forceRefresh: Bool = false,
        cachePolicy: CodexPromptCachePolicy = .summary,
        now: Date = Date()
    ) async throws -> SpaceSummaryResult {
        try requireCodexFeature(.spaceFocusSummaries)
        let effectiveCachePolicy = cachePolicy == .summary ? configuredSummaryCachePolicy() : cachePolicy
        let promptVersion = CodexPromptVersionRegistry.spaceFocusSummary
        let inputHash = hashForSpaceSummaryContext(context)
        let cacheURL = cacheFileURL(kind: "space-summary", key: context.roomID)

        if !forceRefresh,
           let cached = try loadCache(url: cacheURL, as: SpaceSummaryResult.self),
           cached.promptVersion == promptVersion,
           cached.inputHash == inputHash,
           isFresh(timestamp: cached.generatedAt, maxAgeSeconds: effectiveCachePolicy.maxAgeSeconds, now: now) {
            var fromCache = cached
            fromCache.source = "Codex cache"
            return fromCache
        }

        let prompt = spaceSummaryPrompt(context: context, promptVersion: promptVersion)
        let job = makeJob(
            kind: "space-summary",
            key: context.roomID,
            title: "Space Summary: \(context.roomTitle)",
            promptVersion: promptVersion,
            createdAt: now
        )
        let output = try await runner.run(
            request: CodexRunRequest(
                prompt: prompt,
                job: job,
                workingDirectory: workingDirectory,
                policy: configuredCodexRunPolicy()
            )
        ).output

        let parsed = parseSpaceSummaryOutput(output)
        let result = SpaceSummaryResult(
            roomID: context.roomID,
            summary: parsed.summary,
            openLoops: parsed.openLoops,
            topics: Array(parsed.topics.prefix(5)),
            source: "Codex",
            generatedAt: Self.iso8601String(from: now),
            promptVersion: promptVersion,
            inputHash: inputHash
        )
        try persistCache(value: result, url: cacheURL)
        return result
    }

    /// Generates exec questions for matching room participants.
    func generateExecQuestions(
        context: ExecQuestionsContext,
        workingDirectory: URL,
        forceRefresh: Bool = false,
        cachePolicy: CodexPromptCachePolicy = .execQuestions,
        now: Date = Date()
    ) async throws -> ExecQuestionsResult {
        try requireCodexFeature(.execQuestions)
        let effectiveCachePolicy = cachePolicy == .execQuestions ? configuredExecQuestionsCachePolicy() : cachePolicy
        let promptVersion = CodexPromptVersionRegistry.spaceFocusExecQuestions
        let inputHash = hashForExecQuestionsContext(context)
        let cacheKey = "\(context.roomID)-\(context.execEmail)-\(context.execName)"
        let cacheURL = cacheFileURL(kind: "exec-questions", key: cacheKey)

        if !forceRefresh,
           let cached = try loadCache(url: cacheURL, as: ExecQuestionsResult.self),
           cached.promptVersion == promptVersion,
           cached.inputHash == inputHash,
           isFresh(timestamp: cached.generatedAt, maxAgeSeconds: effectiveCachePolicy.maxAgeSeconds, now: now) {
            var fromCache = cached
            fromCache.source = "Codex cache"
            return fromCache
        }

        let prompt = execQuestionsPrompt(context: context, promptVersion: promptVersion)
        let job = makeJob(
            kind: "exec-questions",
            key: cacheKey,
            title: "Exec Questions: \(context.execName)",
            promptVersion: promptVersion,
            createdAt: now
        )
        let output = try await runner.run(
            request: CodexRunRequest(
                prompt: prompt,
                job: job,
                workingDirectory: workingDirectory,
                policy: configuredCodexRunPolicy()
            )
        ).output

        let parsedQuestions = parseExecQuestionsOutput(output)
        let result = ExecQuestionsResult(
            roomID: context.roomID,
            execName: context.execName,
            header: "What are the Questions running in the Exec's (\(context.execName)) Mind:",
            questions: parsedQuestions,
            source: "Codex",
            generatedAt: Self.iso8601String(from: now),
            promptVersion: promptVersion,
            inputHash: inputHash
        )
        try persistCache(value: result, url: cacheURL)
        return result
    }

    /// Generates or reuses exec questions for one executive/room pair.
    func generateExecQuestionsForRoom(
        roomID: String,
        roomTitle: String,
        summary: String,
        openLoops: [String],
        clusters: [SpaceConversationCluster],
        roomParticipants: [SpaceParticipant],
        importantExecutives: [ImportantExecutive],
        execBeliefsByEmail: [String: [BeliefSnapshot]],
        workingDirectory: URL,
        forceRefresh: Bool = false,
        cachePolicy: CodexPromptCachePolicy = .execQuestions,
        now: Date = Date()
    ) async throws -> [ExecQuestionsResult] {
        guard codexFeatureEnabled(.execQuestions) else {
            return []
        }
        let participantNameByEmail: [String: String] = Dictionary(
            uniqueKeysWithValues: roomParticipants.compactMap { participant in
                let normalized = normalizedEmail(participant.email)
                guard !normalized.isEmpty else {
                    return nil
                }
                return (normalized, participant.name) as (String, String)
            }
        )
        let participantEmails = Set(participantNameByEmail.keys)

        let matchedExecutives = importantExecutives.compactMap { exec -> ImportantExecutive? in
            let normalized = normalizedEmail(exec.email)
            guard !normalized.isEmpty else {
                return nil
            }
            return participantEmails.contains(normalized) ? exec : nil
        }.sorted {
            let lhsName = $0.name.trimmingCharacters(in: .whitespacesAndNewlines).lowercased()
            let rhsName = $1.name.trimmingCharacters(in: .whitespacesAndNewlines).lowercased()
            if lhsName == rhsName {
                return normalizedEmail($0.email) < normalizedEmail($1.email)
            }
            return lhsName < rhsName
        }

        guard !matchedExecutives.isEmpty else {
            return []
        }

        var results: [ExecQuestionsResult] = []
        results.reserveCapacity(matchedExecutives.count)
        for exec in matchedExecutives {
            let normalizedExecEmail = normalizedEmail(exec.email)
            let participantName = participantNameByEmail[normalizedExecEmail]?.trimmingCharacters(in: CharacterSet.whitespacesAndNewlines) ?? ""
            let configuredName = exec.name.trimmingCharacters(in: CharacterSet.whitespacesAndNewlines)
            let execName = !configuredName.isEmpty ? configuredName : (!participantName.isEmpty ? participantName : exec.email)
            let context = ExecQuestionsContext(
                roomID: roomID,
                roomTitle: roomTitle,
                execName: execName,
                execEmail: exec.email,
                summary: summary,
                openLoops: openLoops,
                clusters: clusters,
                execBeliefs: execBeliefsByEmail[normalizedExecEmail] ?? []
            )
            let result = try await generateExecQuestions(
                context: context,
                workingDirectory: workingDirectory,
                forceRefresh: forceRefresh,
                cachePolicy: cachePolicy,
                now: now
            )
            results.append(result)
        }

        return results
    }

    /// Decides whether reconciliation should run for a scope/entity.
    func evaluateBeliefReconciliation(
        context: BeliefReconciliationContext,
        state: BeliefReconciliationState,
        staleHours: Int = 24,
        now: Date = Date()
    ) -> BeliefReconciliationDecision {
        let effectiveStaleHours = configuredBeliefStaleHours(staleHours)
        guard let normalizedContext = normalizedBeliefContext(context) else {
            return BeliefReconciliationDecision(
                shouldRun: false,
                reason: .invalidScopeEntityKey,
                evidenceHash: hashForBeliefContext(context),
                staleAt: nil
            )
        }

        let evidenceHash = hashForBeliefContext(normalizedContext)
        let staleInterval = TimeInterval(max(1, effectiveStaleHours) * 3600)

        if normalizedContext.forceRefresh {
            return BeliefReconciliationDecision(
                shouldRun: true,
                reason: .forced,
                evidenceHash: evidenceHash,
                staleAt: nil
            )
        }
        guard let lastRunAt = parseISO8601(state.lastRunAt) else {
            return BeliefReconciliationDecision(
                shouldRun: true,
                reason: .firstRun,
                evidenceHash: evidenceHash,
                staleAt: nil
            )
        }
        if state.lastEvidenceHash != evidenceHash {
            return BeliefReconciliationDecision(
                shouldRun: true,
                reason: .evidenceChanged,
                evidenceHash: evidenceHash,
                staleAt: Self.iso8601String(from: lastRunAt.addingTimeInterval(staleInterval))
            )
        }
        let staleAt = lastRunAt.addingTimeInterval(staleInterval)
        if staleAt <= now {
            return BeliefReconciliationDecision(
                shouldRun: true,
                reason: .stale,
                evidenceHash: evidenceHash,
                staleAt: Self.iso8601String(from: staleAt)
            )
        }
        return BeliefReconciliationDecision(
            shouldRun: false,
            reason: .upToDate,
            evidenceHash: evidenceHash,
            staleAt: Self.iso8601String(from: staleAt)
        )
    }

    /// Runs reconciliation for one context/state pair.
    func runBeliefReconciliation(
        context: BeliefReconciliationContext,
        state: BeliefReconciliationState,
        workingDirectory: URL,
        staleHours: Int = 24,
        now: Date = Date()
    ) async throws -> BeliefReconciliationRunOutcome {
        try requireCodexFeature(.beliefs)
        let effectiveContext = normalizedBeliefContext(context) ?? context
        let decision = evaluateBeliefReconciliation(
            context: effectiveContext,
            state: state,
            staleHours: configuredBeliefStaleHours(staleHours),
            now: now
        )
        let nextState = BeliefReconciliationState(
            lastRunAt: decision.shouldRun ? Self.iso8601String(from: now) : state.lastRunAt,
            lastEvidenceHash: decision.reason == .invalidScopeEntityKey ? state.lastEvidenceHash : decision.evidenceHash
        )

        guard decision.shouldRun else {
            return BeliefReconciliationRunOutcome(decision: decision, result: nil, nextState: nextState)
        }

        let promptVersion = CodexPromptVersionRegistry.beliefReconciliation
        let prompt = beliefReconciliationPrompt(context: effectiveContext, promptVersion: promptVersion)
        let key = "\(effectiveContext.scope.rawValue)-\(effectiveContext.entityKey)"
        let job = makeJob(
            kind: "belief-reconciliation",
            key: key,
            title: "Belief Reconciliation \(effectiveContext.scope.rawValue):\(effectiveContext.entityKey)",
            promptVersion: promptVersion,
            createdAt: now
        )
        let output = try await runner.run(
            request: CodexRunRequest(
                prompt: prompt,
                job: job,
                workingDirectory: workingDirectory,
                policy: configuredCodexRunPolicy()
            )
        ).output
        let parsed = parseBeliefReconciliationOutput(output)

        let result = BeliefReconciliationResult(
            taskType: CodexPromptVersionRegistry.beliefTaskType,
            scope: effectiveContext.scope,
            entityKey: effectiveContext.entityKey,
            promptVersion: promptVersion,
            decision: decision,
            beliefsToAdd: parsed.beliefsToAdd,
            beliefsToUpdate: parsed.beliefsToUpdate,
            beliefsToWeaken: parsed.beliefsToWeaken,
            confidence: parsed.confidence,
            evidenceLinks: parsed.evidenceLinks,
            generatedAt: Self.iso8601String(from: now),
            inputHash: hashForBeliefContext(effectiveContext),
            source: "Codex"
        )
        return BeliefReconciliationRunOutcome(decision: decision, result: result, nextState: nextState)
    }

    /// Runs reconciliation across multiple targets and reports trigger counts.
    func runBeliefReconciliationBatch(
        targets: [BeliefReconciliationTarget],
        workingDirectory: URL,
        staleHours: Int = 24,
        now: Date = Date()
    ) async throws -> BeliefReconciliationBatchOutcome {
        guard codexFeatureEnabled(.beliefs) else {
            return BeliefReconciliationBatchOutcome(
                outcomes: [],
                triggeredCount: 0,
                skippedCount: targets.count
            )
        }
        var outcomes: [BeliefReconciliationTargetOutcome] = []
        outcomes.reserveCapacity(targets.count)

        for target in targets {
            let key = beliefScopeKey(for: target.context) ?? BeliefReconciliationScopeKey(
                scope: target.context.scope,
                entityKey: target.context.entityKey.trimmingCharacters(in: .whitespacesAndNewlines)
            )
            let outcome = try await runDeepBeliefReconciliation(
                target: target,
                workingDirectory: workingDirectory,
                staleHours: configuredBeliefStaleHours(staleHours),
                now: now
            )
            outcomes.append(
                BeliefReconciliationTargetOutcome(
                    key: key,
                    outcome: outcome
                )
            )
        }

        let triggeredCount = outcomes.reduce(into: 0) { partialResult, entry in
            if entry.outcome.decision.shouldRun {
                partialResult += 1
            }
        }
        return BeliefReconciliationBatchOutcome(
            outcomes: outcomes,
            triggeredCount: triggeredCount,
            skippedCount: outcomes.count - triggeredCount
        )
    }

    /// Expands broad reconciliation into smaller evidence windows/chunks.
    private func runDeepBeliefReconciliation(
        target: BeliefReconciliationTarget,
        workingDirectory: URL,
        staleHours: Int,
        now: Date
    ) async throws -> BeliefReconciliationRunOutcome {
        guard var baseContext = normalizedBeliefContext(target.context) else {
            return try await runBeliefReconciliation(
                context: target.context,
                state: target.state,
                workingDirectory: workingDirectory,
                staleHours: configuredBeliefStaleHours(staleHours),
                now: now
            )
        }

        baseContext.incrementalWindowDays = max(1, baseContext.incrementalWindowDays)
        baseContext.chunkIndex = nil
        baseContext.chunkCount = nil

        let baseDecision = evaluateBeliefReconciliation(
            context: baseContext,
            state: target.state,
            staleHours: configuredBeliefStaleHours(staleHours),
            now: now
        )
        let baseHash = baseDecision.evidenceHash

        guard baseDecision.shouldRun else {
            let nextState = BeliefReconciliationState(
                lastRunAt: target.state.lastRunAt,
                lastEvidenceHash: baseHash
            )
            return BeliefReconciliationRunOutcome(
                decision: baseDecision,
                result: nil,
                nextState: nextState
            )
        }

        let deepContexts = deepBeliefContexts(
            for: baseContext,
            now: now,
            maxChunkSize: configuredBeliefEvidenceChunkSize(),
            maxWindowDays: configuredMaxBeliefIncrementalWindowDays()
        )
        var chunkResults: [BeliefReconciliationResult] = []
        chunkResults.reserveCapacity(deepContexts.count)

        var workingState = target.state
        for var chunkContext in deepContexts {
            // The run gate is evaluated once against the full scope/evidence hash.
            // Every generated deep chunk should execute in that run.
            chunkContext.forceRefresh = true
            let chunkOutcome = try await runBeliefReconciliation(
                context: chunkContext,
                state: workingState,
                workingDirectory: workingDirectory,
                staleHours: configuredBeliefStaleHours(staleHours),
                now: now
            )
            if let result = chunkOutcome.result {
                chunkResults.append(result)
            }
            workingState = chunkOutcome.nextState
        }

        let mergedResult = mergeBeliefReconciliationResults(
            chunkResults,
            decision: baseDecision,
            baseContext: baseContext,
            now: now
        )
        let nextState = BeliefReconciliationState(
            lastRunAt: Self.iso8601String(from: now),
            lastEvidenceHash: baseHash
        )
        return BeliefReconciliationRunOutcome(
            decision: baseDecision,
            result: mergedResult,
            nextState: nextState
        )
    }

    /// Creates deterministic Codex job paths for one prompt kind/key.
    private func makeJob(
        kind: String,
        key: String,
        title: String,
        promptVersion: String,
        createdAt: Date
    ) -> CodexPromptJob {
        let base = jobDirectoryURL(kind: kind, key: key)
        let id = "\(kind)-\(safeKey(key))-\(Int(createdAt.timeIntervalSince1970))"
        return CodexPromptJob(
            id: id,
            title: title,
            promptVersion: promptVersion,
            promptURL: base.appendingPathComponent("prompt.md"),
            outputURL: base.appendingPathComponent("output.txt"),
            logURL: base.appendingPathComponent("run.log"),
            metadataURL: base.appendingPathComponent("run-metadata.json"),
            status: .pending,
            createdAt: createdAt
        )
    }

    /// Root directory for Codex cache artifacts.
    private func cacheDirectoryURL() -> URL {
        configuration.runtimeRoot
            .appendingPathComponent("knowledge", isDirectory: true)
            .appendingPathComponent("codex", isDirectory: true)
            .appendingPathComponent("cache", isDirectory: true)
    }

    /// Root directory for Codex job artifacts of one kind/key.
    private func jobDirectoryURL(kind: String, key: String) -> URL {
        configuration.runtimeRoot
            .appendingPathComponent("knowledge", isDirectory: true)
            .appendingPathComponent("codex", isDirectory: true)
            .appendingPathComponent("jobs", isDirectory: true)
            .appendingPathComponent(kind, isDirectory: true)
            .appendingPathComponent(safeKey(key), isDirectory: true)
    }

    /// Cache file path for one prompt kind/key.
    private func cacheFileURL(kind: String, key: String) -> URL {
        cacheDirectoryURL().appendingPathComponent("\(kind)-\(safeKey(key)).json")
    }

    private func safeKey(_ value: String) -> String {
        let allowed = Set("abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ0123456789-_")
        let cleaned = value.map { allowed.contains($0) ? $0 : "-" }
        return String(cleaned).trimmingCharacters(in: CharacterSet(charactersIn: "-"))
    }

    private func compactWhitespace(_ value: String) -> String {
        value
            .trimmingCharacters(in: .whitespacesAndNewlines)
            .replacingOccurrences(of: #"\s+"#, with: " ", options: .regularExpression)
    }

    private func truncate(_ value: String, maxLength: Int) -> String {
        let compacted = compactWhitespace(value)
        guard compacted.count > maxLength else {
            return compacted
        }
        return "\(String(compacted.prefix(max(0, maxLength - 3))))..."
    }

    private func normalizedEmail(_ value: String) -> String {
        value.trimmingCharacters(in: .whitespacesAndNewlines).lowercased()
    }

    private func beliefScopeKey(for context: BeliefReconciliationContext) -> BeliefReconciliationScopeKey? {
        guard let normalizedEntityKey = normalizedBeliefEntityKey(scope: context.scope, entityKey: context.entityKey) else {
            return nil
        }
        return BeliefReconciliationScopeKey(scope: context.scope, entityKey: normalizedEntityKey)
    }

    private func normalizedBeliefContext(_ context: BeliefReconciliationContext) -> BeliefReconciliationContext? {
        guard let scopeKey = beliefScopeKey(for: context) else {
            return nil
        }
        var normalized = context
        normalized.entityKey = scopeKey.entityKey
        return normalized
    }

    private func normalizedBeliefEntityKey(scope: BeliefScope, entityKey: String) -> String? {
        switch scope {
        case .global:
            return BeliefScope.globalEntityKey
        case .person, .space:
            let normalized = entityKey.trimmingCharacters(in: .whitespacesAndNewlines)
            return normalized.isEmpty ? nil : normalized
        }
    }

    private func deepBeliefContexts(
        for context: BeliefReconciliationContext,
        now: Date,
        maxChunkSize: Int,
        maxWindowDays: Int
    ) -> [BeliefReconciliationContext] {
        let incrementalWindows = deepBeliefIncrementalWindows(
            baseDays: context.incrementalWindowDays,
            maxWindowDays: maxWindowDays
        )
        var contexts: [BeliefReconciliationContext] = []
        var seenHashes = Set<String>()

        for windowDays in incrementalWindows {
            let evidenceWindow = beliefEvidence(context.evidence, withinDays: windowDays, now: now)
            let chunks = chunkBeliefEvidence(evidenceWindow, maxChunkSize: maxChunkSize)
            let chunkCount = chunks.count

            for (index, chunk) in chunks.enumerated() {
                var chunkContext = context
                chunkContext.incrementalWindowDays = windowDays
                chunkContext.chunkIndex = chunkCount > 1 ? index + 1 : nil
                chunkContext.chunkCount = chunkCount > 1 ? chunkCount : nil
                chunkContext.evidence = chunk

                let chunkHash = hashForBeliefContext(chunkContext)
                if seenHashes.insert(chunkHash).inserted {
                    contexts.append(chunkContext)
                }
            }
        }

        if contexts.isEmpty {
            return [context]
        }
        return contexts
    }

    private func deepBeliefIncrementalWindows(baseDays: Int, maxWindowDays: Int) -> [Int] {
        let cap = max(1, maxWindowDays)
        let base = min(max(1, baseDays), cap)
        let candidates = [
            base,
            min(cap, max(base + 1, base * 2)),
            min(cap, max(base + 2, base * 4))
        ]
        var seen = Set<Int>()
        return candidates.filter { seen.insert($0).inserted }.sorted()
    }

    private func beliefEvidence(_ evidence: [BeliefEvidence], withinDays days: Int, now: Date) -> [BeliefEvidence] {
        let cutoff = now.addingTimeInterval(-TimeInterval(max(1, days) * 86_400))
        return evidence.filter { item in
            guard let timestamp = parseISO8601(item.occurredAt) else {
                return true
            }
            return timestamp >= cutoff
        }
    }

    private func chunkBeliefEvidence(_ evidence: [BeliefEvidence], maxChunkSize: Int) -> [[BeliefEvidence]] {
        let chunkSize = max(1, maxChunkSize)
        guard !evidence.isEmpty else {
            return [[]]
        }

        var chunks: [[BeliefEvidence]] = []
        chunks.reserveCapacity((evidence.count + (chunkSize - 1)) / chunkSize)

        var start = 0
        while start < evidence.count {
            let end = min(start + chunkSize, evidence.count)
            chunks.append(Array(evidence[start..<end]))
            start = end
        }
        return chunks
    }

    private func mergeBeliefReconciliationResults(
        _ results: [BeliefReconciliationResult],
        decision: BeliefReconciliationDecision,
        baseContext: BeliefReconciliationContext,
        now: Date
    ) -> BeliefReconciliationResult {
        let mergedAdd = mergeBeliefChanges(results.flatMap(\.beliefsToAdd))
        let mergedUpdate = mergeBeliefChanges(results.flatMap(\.beliefsToUpdate))
        let mergedWeaken = mergeBeliefChanges(results.flatMap(\.beliefsToWeaken))
        let mergedEvidenceLinks = uniqueOrderedStrings(results.flatMap(\.evidenceLinks))
        let averageConfidence: Double
        if results.isEmpty {
            averageConfidence = 0
        } else {
            averageConfidence = results.reduce(0) { partial, item in
                partial + item.confidence
            } / Double(results.count)
        }
        let normalizedConfidence = min(max(averageConfidence, 0), 1)
        let promptVersion = results.last?.promptVersion ?? CodexPromptVersionRegistry.beliefReconciliation
        let source = results.count > 1 ? "Codex (deep)" : (results.first?.source ?? "Codex")

        return BeliefReconciliationResult(
            taskType: CodexPromptVersionRegistry.beliefTaskType,
            scope: baseContext.scope,
            entityKey: baseContext.entityKey,
            promptVersion: promptVersion,
            decision: decision,
            beliefsToAdd: mergedAdd,
            beliefsToUpdate: mergedUpdate,
            beliefsToWeaken: mergedWeaken,
            confidence: normalizedConfidence,
            evidenceLinks: mergedEvidenceLinks,
            generatedAt: Self.iso8601String(from: now),
            inputHash: hashForBeliefContext(baseContext),
            source: source
        )
    }

    private func mergeBeliefChanges(_ changes: [BeliefChange]) -> [BeliefChange] {
        struct MergedBeliefChange {
            var statement: String
            var confidence: Double
            var evidenceLinks: [String]
            var beliefKind: String
            var lifecycle: String
            var supportCount: Int
            var contradictionCount: Int
            var lastEvidenceAt: String
        }

        var mergedByStatement: [String: MergedBeliefChange] = [:]
        for change in changes {
            let statement = change.statement.trimmingCharacters(in: .whitespacesAndNewlines)
            guard !statement.isEmpty else {
                continue
            }
            let key = statement.lowercased()
            let confidence = min(max(change.confidence, 0), 1)
            let links = uniqueOrderedStrings(change.evidenceLinks)
            let supportCount = max(0, change.supportCount)
            let contradictionCount = max(0, change.contradictionCount)
            let lastEvidenceAt = change.lastEvidenceAt.trimmingCharacters(in: .whitespacesAndNewlines)

            if var existing = mergedByStatement[key] {
                if confidence >= existing.confidence {
                    existing.statement = statement
                    existing.confidence = confidence
                    existing.beliefKind = normalizedBeliefMetadata(change.beliefKind, fallback: existing.beliefKind)
                    existing.lifecycle = normalizedBeliefMetadata(change.lifecycle, fallback: existing.lifecycle)
                }
                existing.evidenceLinks = uniqueOrderedStrings(existing.evidenceLinks + links)
                existing.supportCount = max(existing.supportCount, supportCount)
                existing.contradictionCount = max(existing.contradictionCount, contradictionCount)
                if !lastEvidenceAt.isEmpty {
                    existing.lastEvidenceAt = max(existing.lastEvidenceAt, lastEvidenceAt)
                }
                mergedByStatement[key] = existing
            } else {
                mergedByStatement[key] = MergedBeliefChange(
                    statement: statement,
                    confidence: confidence,
                    evidenceLinks: links,
                    beliefKind: normalizedBeliefMetadata(change.beliefKind, fallback: "second_order"),
                    lifecycle: normalizedBeliefMetadata(change.lifecycle, fallback: "candidate"),
                    supportCount: supportCount,
                    contradictionCount: contradictionCount,
                    lastEvidenceAt: lastEvidenceAt
                )
            }
        }

        return mergedByStatement.values
            .map { value in
                BeliefChange(
                    statement: value.statement,
                    confidence: value.confidence,
                    evidenceLinks: value.evidenceLinks,
                    beliefKind: value.beliefKind,
                    lifecycle: value.lifecycle,
                    supportCount: value.supportCount,
                    contradictionCount: value.contradictionCount,
                    lastEvidenceAt: value.lastEvidenceAt
                )
            }
            .sorted { lhs, rhs in
                if lhs.confidence == rhs.confidence {
                    return lhs.statement.localizedCaseInsensitiveCompare(rhs.statement) == .orderedAscending
                }
                return lhs.confidence > rhs.confidence
            }
    }

    private func uniqueOrderedStrings(_ values: [String]) -> [String] {
        var seen = Set<String>()
        var ordered: [String] = []
        ordered.reserveCapacity(values.count)

        for value in values {
            let trimmed = value.trimmingCharacters(in: .whitespacesAndNewlines)
            guard !trimmed.isEmpty else {
                continue
            }
            if seen.insert(trimmed).inserted {
                ordered.append(trimmed)
            }
        }
        return ordered
    }

    private func normalizedBeliefMetadata(_ value: String, fallback: String) -> String {
        let normalized = value
            .trimmingCharacters(in: .whitespacesAndNewlines)
            .lowercased()
            .replacingOccurrences(of: " ", with: "_")
            .replacingOccurrences(of: "-", with: "_")
        return normalized.isEmpty ? fallback : normalized
    }

    private func persistCache<T: Encodable>(value: T, url: URL) throws {
        try FileManager.default.createDirectory(at: url.deletingLastPathComponent(), withIntermediateDirectories: true)
        let data = try encoder.encode(value)
        try data.write(to: url, options: .atomic)
    }

    private func loadCache<T: Decodable>(url: URL, as type: T.Type) throws -> T? {
        guard FileManager.default.fileExists(atPath: url.path) else {
            return nil
        }
        let data = try Data(contentsOf: url)
        return try decoder.decode(T.self, from: data)
    }

    private func isFresh(timestamp: String, maxAgeSeconds: TimeInterval, now: Date) -> Bool {
        guard let time = parseISO8601(timestamp) else {
            return false
        }
        return now.timeIntervalSince(time) <= maxAgeSeconds
    }

    private func parseISO8601(_ value: String?) -> Date? {
        guard let value, !value.isEmpty else { return nil }
        return Self.iso8601Formatter.date(from: value)
    }

    private func hashForSpaceSummaryContext(_ context: SpaceSummaryContext) -> String {
        let source = [
            context.roomID,
            context.roomTitle,
            context.previousSummary ?? "",
            context.previousGeneratedAt ?? "",
            context.openLoops.joined(separator: "\n"),
            context.clusters.map { "\($0.title)|\($0.summary)|\($0.openLoops.joined(separator: ","))" }.joined(separator: "\n")
        ].joined(separator: "\n---\n")
        return Self.sha256Hex(source)
    }

    private func hashForPersonClusterSummaryContext(_ context: PersonClusterSummaryContext) -> String {
        let source = [
            context.personID,
            context.personLabel,
            context.existingSummary ?? "",
            context.conversationHighlights.joined(separator: "\n")
        ].joined(separator: "\n---\n")
        return Self.sha256Hex(source)
    }

    private func hashForClusterTitleContext(_ context: ClusterTitleContext) -> String {
        let source = [
            context.focusKind.rawValue,
            context.clusterID,
            context.clusterSummary,
            context.existingTitle ?? "",
            context.supportingSignals.joined(separator: "\n")
        ].joined(separator: "\n---\n")
        return Self.sha256Hex(source)
    }

    private func hashForExecQuestionsContext(_ context: ExecQuestionsContext) -> String {
        let source = [
            context.roomID,
            context.roomTitle,
            context.execName,
            context.execEmail,
            context.summary,
            context.openLoops.joined(separator: "\n"),
            context.clusters.map { "\($0.title)|\($0.summary)|\($0.openLoops.joined(separator: ","))" }.joined(separator: "\n"),
            context.execBeliefs.map { "\($0.statement)|\($0.confidence)|\($0.evidenceLinks.joined(separator: ","))" }.joined(separator: "\n")
        ].joined(separator: "\n---\n")
        return Self.sha256Hex(source)
    }

    private func hashForBeliefContext(_ context: BeliefReconciliationContext) -> String {
        let source = [
            context.scope.rawValue,
            context.entityKey,
            String(context.incrementalWindowDays),
            String(context.chunkIndex ?? 0),
            String(context.chunkCount ?? 0),
            context.currentBeliefs.map { "\($0.statement)|\($0.confidence)|\($0.evidenceLinks.joined(separator: ","))" }.joined(separator: "\n"),
            context.manualBeliefs.map { "\($0.statement)|\($0.confidence)|\($0.evidenceLinks.joined(separator: ","))" }.joined(separator: "\n"),
            context.evidence.map { "\($0.id)|\($0.source)|\($0.occurredAt)|\($0.text)" }.joined(separator: "\n")
        ].joined(separator: "\n---\n")
        return Self.sha256Hex(source)
    }

    private func hashForQuestionSynthesisCandidates(
        _ candidates: [QuestionCandidate],
        queryHistory: [AskCodexQueryHistoryEntry],
        evidenceLimit: Int
    ) -> String {
        let candidateSource = candidates.map { candidate in
            [
                candidate.scopeType.rawValue,
                candidate.scopeKey,
                candidate.scopeLabel,
                candidate.questionText,
                candidate.whyNow,
                candidate.evidence
                    .prefix(max(1, evidenceLimit))
                    .map { "\($0.sourceType)|\($0.sourceID)|\($0.label)|\($0.preview)" }
                    .joined(separator: "\n")
            ].joined(separator: "\n")
        }.joined(separator: "\n---\n")
        let historySource = queryHistory.map { entry in
            [
                entry.targetScope.rawValue,
                entry.targetKey,
                entry.targetTitle,
                entry.question
            ].joined(separator: "\n")
        }.joined(separator: "\n---\n")
        let source = [candidateSource, historySource].joined(separator: "\n=== operator ask history ===\n")
        return Self.sha256Hex(source)
    }

    private func questionSynthesisPrompt(
        candidates: [QuestionCandidate],
        queryHistory: [AskCodexQueryHistoryEntry],
        promptVersion: String,
        historyLimit: Int,
        candidateEvidenceLimit: Int
    ) -> String {
        let candidateBlocks = candidates.enumerated().map { index, candidate in
            let evidence = candidate.evidence
                .prefix(max(1, candidateEvidenceLimit))
                .enumerated()
                .map { evidenceIndex, ref in
                """
                Evidence \(evidenceIndex + 1):
                - source_id: \(ref.sourceID)
                - label: \(truncate(ref.label, maxLength: 160))
                - preview: \(truncate(ref.preview, maxLength: 420))
                """
            }.joined(separator: "\n")

            return """
            Candidate \(index + 1):
            - scope_type: \(candidate.scopeType.rawValue)
            - scope_key: \(candidate.scopeKey)
            - scope_label: \(candidate.scopeLabel)
            - seed_question: \(truncate(candidate.questionText, maxLength: 240))
            - seed_why_now: \(truncate(candidate.whyNow, maxLength: 320))
            - seed_tags: \(candidate.tags.joined(separator: ", "))
            \(evidence.isEmpty ? "Evidence: none" : evidence)
            """
        }.joined(separator: "\n\n")
        let historyBlocks = queryHistory.prefix(max(0, historyLimit)).enumerated().map { index, entry in
            """
            Prior Question \(index + 1):
            - target_scope: \(entry.targetScope.rawValue)
            - target_key: \(entry.targetKey)
            - target_title: \(truncate(entry.targetTitle, maxLength: 180))
            - asked_at: \(entry.submittedAt)
            - question: \(truncate(entry.question, maxLength: 260))
            """
        }.joined(separator: "\n\n")

        return """
        Prompt Version: \(promptVersion)
        Task: Act as Cubicle's Questions editor. Rewrite deterministic question seeds into high-value questions a busy operator would actually ask now.
        Output format: JSON object only.

        JSON schema:
        {
          "questions": [
            {
              "scope_type": "person|space",
              "scope_key": "must match one supplied candidate scope_key",
              "scope_label": "string",
              "question": "string",
              "why_now": "string",
              "tags": ["string"],
              "priority_score": 0,
              "evidence_source_ids": ["source_id from supplied evidence"]
            }
          ]
        }

        Rules:
        - Use Codex's configured model; do not mention or assume any model version in the output.
        - Return at most 24 questions and at most 2 questions per scope_key.
        - Base every question only on supplied evidence.
        - Every question must be useful for action: owner, follow-up, decision, deadline, customer risk, escalation, blocker, dependency, or unresolved question.
        - Do not publish analytics phrasing: no cohorts, sample sizes, raw metric names, duration_seconds, response time seconds, high-question/low-question wording, thread IDs, or prediction/correlation wording.
        - Make questions concrete and natural. Mention the scope label when it helps.
        - Use operator Ask Codex history only as a preference signal for question shape, recurring concerns, and target interest.
        - Do not treat operator Ask Codex history as evidence; every published question must still be grounded in supplied candidate evidence.
        - why_now should explain the business/action reason in one sentence, not list metrics.
        - evidence_source_ids must reference supplied evidence source_id values.
        - If no seed is useful, return {"questions":[]}.

        Operator Ask Codex History:
        \(historyBlocks.isEmpty ? "none" : historyBlocks)

        Candidate Seeds:
        \(candidateBlocks)
        """
    }

    private func makeSynthesizedQuestionCandidates(
        from result: CodexQuestionSynthesisResult,
        seedCandidates: [QuestionCandidate],
        evidenceLimit: Int,
        now: Date
    ) -> [QuestionCandidate] {
        let candidatesByScope = Dictionary(grouping: seedCandidates) { candidate in
            "\(candidate.scopeType.rawValue)|\(candidate.scopeKey)"
        }
        let evidenceByID = seedCandidates
            .flatMap(\.evidence)
            .reduce(into: [String: QuestionEvidenceRef]()) { partialResult, evidence in
                if partialResult[evidence.sourceID] == nil {
                    partialResult[evidence.sourceID] = evidence
                }
            }

        var output: [QuestionCandidate] = []
        var emittedPerScope: [String: Int] = [:]
        var seenQuestionKeys = Set<String>()
        for draft in result.questions {
            let scopeKey = "\(draft.scopeType.rawValue)|\(draft.scopeKey)"
            guard let scopeSeeds = candidatesByScope[scopeKey],
                  (emittedPerScope[scopeKey] ?? 0) < 2 else {
                continue
            }

            let questionText = compactWhitespace(draft.questionText)
            let whyNow = compactWhitespace(draft.whyNow)
            guard !questionText.isEmpty,
                  !whyNow.isEmpty,
                  QuestionCandidateService.isPublishableQuestionTextForCubicle(
                      questionText: questionText,
                      whyNow: whyNow,
                      tags: draft.tags
                  ) else {
                continue
            }

            let evidence = selectedEvidence(
                sourceIDs: draft.evidenceSourceIDs,
                evidenceByID: evidenceByID,
                fallbackCandidates: scopeSeeds,
                limit: evidenceLimit
            )
            guard !evidence.isEmpty else {
                continue
            }

            let normalizedQuestionKey = safeKey(questionText.lowercased())
            guard seenQuestionKeys.insert("\(scopeKey)|\(normalizedQuestionKey)").inserted else {
                continue
            }

            let seed = scopeSeeds.first!
            let priorityScore = min(132, max(95, draft.priorityScore ?? 124))
            let sourceKey = "\(draft.scopeKey):\(result.inputHash.prefix(16))"
            let candidateID = "question-\(Self.sha256Hex(["codex-question-synthesis", draft.scopeType.rawValue, draft.scopeKey, questionText].joined(separator: "|")))"
            output.append(
                QuestionCandidate(
                    id: candidateID,
                    scopeType: draft.scopeType,
                    scopeKey: draft.scopeKey,
                    scopeLabel: seed.scopeLabel,
                    questionText: questionText,
                    questionType: "codex_synthesized_question",
                    whyNow: whyNow,
                    evidence: evidence,
                    sourceKind: "codex_question_synthesis",
                    sourceKey: sourceKey,
                    tags: synthesizedQuestionTags(draft.tags),
                    priorityScore: priorityScore,
                    status: .candidate,
                    answerSnapshotId: nil,
                    createdAt: now,
                    updatedAt: now,
                    expiresAt: Calendar.current.date(byAdding: .day, value: 14, to: now)
                )
            )
            emittedPerScope[scopeKey, default: 0] += 1
        }
        return output
    }

    private func selectedEvidence(
        sourceIDs: [String],
        evidenceByID: [String: QuestionEvidenceRef],
        fallbackCandidates: [QuestionCandidate],
        limit: Int
    ) -> [QuestionEvidenceRef] {
        let evidenceLimit = max(1, limit)
        let selected = sourceIDs.compactMap { evidenceByID[$0] }
        if !selected.isEmpty {
            return Array(selected.prefix(evidenceLimit))
        }
        return Array(fallbackCandidates.flatMap(\.evidence).prefix(evidenceLimit))
    }

    private func synthesizedQuestionTags(_ tags: [String]) -> [String] {
        var output = ["codex", "synthesis", "ai"]
        output.append(contentsOf: tags.map { safeKey($0.lowercased()) }.filter { !$0.isEmpty }.prefix(5))
        var seen = Set<String>()
        return output.filter { seen.insert($0).inserted }
    }

    private func parseQuestionSynthesisOutput(_ output: String, limit: Int = 7) -> [CodexQuestionSynthesisDraft] {
        guard let json = extractJSONObject(from: output),
              let data = json.data(using: .utf8),
              let payload = try? decoder.decode(CodexQuestionSynthesisPayload.self, from: data) else {
            return []
        }
        return Array(payload.questions.prefix(max(0, limit)))
    }

    private func spaceSummaryPrompt(context: SpaceSummaryContext, promptVersion: String) -> String {
        let clusterLines = context.clusters.enumerated().map { index, cluster in
            let loops = cluster.openLoops.isEmpty ? "none" : cluster.openLoops.joined(separator: "; ")
            return "\(index + 1). \(cluster.title)\nSummary: \(cluster.summary)\nOpen loops: \(loops)"
        }.joined(separator: "\n\n")
        let roomSummary = context.previousSummary?.isEmpty == false ? context.previousSummary! : "none"
        let roomSummaryGeneratedAt = context.previousGeneratedAt?.isEmpty == false ? context.previousGeneratedAt! : "unknown"

        return """
        Prompt Version: \(promptVersion)
        Task: Generate space summary and meaningful topics for a Webex space.
        Output format: JSON object only.

        JSON schema:
        {
          "space_summary": "string",
          "open_loops": ["string"],
          "meaningful_topics": [
            {
              "title": "string",
              "summary": "string",
              "so_what": "string"
            }
          ]
        }

        Rules:
        - Keep meaningful_topics to at most 5 entries.
        - Include "so_what" for each topic; if weak, infer business impact.
        - Base output only on provided context.

        Room ID: \(context.roomID)
        Room Title: \(context.roomTitle)
        Previous Summary: \(roomSummary)
        Previous Summary Generated At: \(roomSummaryGeneratedAt)
        Global Open Loops: \(context.openLoops.joined(separator: " | "))

        Clusters:
        \(clusterLines)
        """
    }

    private func personClusterSummaryPrompt(context: PersonClusterSummaryContext, promptVersion: String) -> String {
        let existingSummary = context.existingSummary?.isEmpty == false ? context.existingSummary! : "none"
        let highlights = context.conversationHighlights.isEmpty
            ? "none"
            : context.conversationHighlights.enumerated().map { "\($0 + 1). \($1)" }.joined(separator: "\n")

        return """
        Prompt Version: \(promptVersion)
        Task: Generate a concise person-focus cluster summary from recent conversations.
        Output format: JSON object only.

        JSON schema:
        {
          "summary": "string"
        }

        Rules:
        - 2 to 4 sentences, focusing on current priorities, blockers, and next likely moves.
        - Use only provided highlights.
        - Keep language concrete and operator-facing.

        Person ID: \(context.personID)
        Person Label: \(context.personLabel)
        Existing Summary: \(existingSummary)
        Conversation Highlights:
        \(highlights)
        """
    }

    private func clusterTitlePrompt(context: ClusterTitleContext, promptVersion: String) -> String {
        let existingTitle = context.existingTitle?.isEmpty == false ? context.existingTitle! : "none"
        let signals = context.supportingSignals.isEmpty
            ? "none"
            : context.supportingSignals.enumerated().map { "\($0 + 1). \($1)" }.joined(separator: "\n")

        return """
        Prompt Version: \(promptVersion)
        Task: Generate a short cluster title for \(context.focusKind.rawValue)-focus output.
        Output format: JSON object only.

        JSON schema:
        {
          "title": "string"
        }

        Rules:
        - 3 to 7 words.
        - Reflect the summary theme and supporting signals.
        - Avoid punctuation-heavy or vague titles.

        Cluster ID: \(context.clusterID)
        Existing Title: \(existingTitle)
        Cluster Summary: \(context.clusterSummary)
        Supporting Signals:
        \(signals)
        """
    }

    private func clusterPromptContract(for kind: CodexClusterPromptKind) -> CodexClusterPromptContract {
        if let contract = CodexPromptVersionRegistry.clusterPromptContracts[kind] {
            return contract
        }
        // Should never happen because the registry is static and exhaustive.
        return CodexClusterPromptContract(
            kind: kind,
            promptVersion: CodexPromptVersionRegistry.personFocusClusterSummary,
            outputField: "summary"
        )
    }

    private func clusterPromptKindForTitle(_ focusKind: ClusterFocusKind) -> CodexClusterPromptKind {
        switch focusKind {
        case .person:
            return .personTitle
        case .space:
            return .spaceTitle
        }
    }

    private func execQuestionsPrompt(context: ExecQuestionsContext, promptVersion: String) -> String {
        let clusterLines = context.clusters.enumerated().map { index, cluster in
            "\(index + 1). \(cluster.title): \(cluster.summary)"
        }.joined(separator: "\n")
        let beliefLines = context.execBeliefs.enumerated().map { index, belief in
            "\(index + 1). \(belief.statement) (confidence \(String(format: "%.2f", belief.confidence)))"
        }.joined(separator: "\n")

        return """
        Prompt Version: \(promptVersion)
        Task: Generate strategic questions likely running in the exec's mind for this room.
        Output format: JSON object only.

        JSON schema:
        {
          "questions": ["string"]
        }

        Rules:
        - Return 3 to 7 concise questions.
        - Questions must reflect room context, open loops, and belief context.

        Room ID: \(context.roomID)
        Room Title: \(context.roomTitle)
        Executive: \(context.execName) <\(context.execEmail)>
        Space Summary: \(context.summary)
        Open Loops: \(context.openLoops.joined(separator: " | "))

        Cluster Context:
        \(clusterLines)

        Executive Belief Context:
        \(beliefLines.isEmpty ? "none" : beliefLines)
        """
    }

    private func beliefReconciliationPrompt(context: BeliefReconciliationContext, promptVersion: String) -> String {
        let currentBeliefs = context.currentBeliefs.enumerated().map { index, belief in
            "\(index + 1). \(belief.statement) [confidence=\(String(format: "%.2f", belief.confidence)); lifecycle=\(belief.lifecycle); support=\(belief.supportCount); contradictions=\(belief.contradictionCount); last_evidence=\(belief.lastEvidenceAt)]"
        }.joined(separator: "\n")
        let manualBeliefs = context.manualBeliefs.enumerated().map { index, belief in
            "\(index + 1). \(belief.statement) [confidence=\(String(format: "%.2f", belief.confidence)); manual_prior=true]"
        }.joined(separator: "\n")
        let evidenceLines = context.evidence.enumerated().map { index, evidence in
            "\(index + 1). (\(evidence.id)) [\(evidence.source)] \(evidence.occurredAt) - \(evidence.text)"
        }.joined(separator: "\n")

        return """
        Prompt Version: \(promptVersion)
        Task Type: \(CodexPromptVersionRegistry.beliefTaskType)
        Task: Reconcile durable second-order beliefs for this scope using current beliefs and incremental evidence.
        Output format: JSON object only.

        JSON schema:
        {
          "beliefs_to_add": [{
            "statement":"string",
            "confidence":0.0,
            "evidence_links":["string"],
            "belief_kind":"second_order",
            "lifecycle":"candidate|active|stable|retired",
            "support_count":1,
            "contradiction_count":0,
            "last_evidence_at":"string"
          }],
          "beliefs_to_update": [{
            "statement":"string",
            "confidence":0.0,
            "evidence_links":["string"],
            "belief_kind":"second_order",
            "lifecycle":"candidate|active|stable|retired",
            "support_count":1,
            "contradiction_count":0,
            "last_evidence_at":"string"
          }],
          "beliefs_to_weaken": [{
            "statement":"string",
            "confidence":0.0,
            "evidence_links":["string"],
            "belief_kind":"second_order",
            "lifecycle":"candidate|active|stable|retired",
            "support_count":1,
            "contradiction_count":0,
            "last_evidence_at":"string"
          }],
          "confidence": 0.0,
          "evidence_links": ["string"]
        }

        Rules:
        - Use only evidence provided.
        - Keep confidence between 0.0 and 1.0.
        - Respect manual beliefs as stronger priors unless contradictory evidence is explicit.
        - A belief is a durable second-order pattern about preference, judgment, operating principle, risk posture, communication style, decision heuristic, or value tradeoff.
        - Do not add tactical facts, current project status, incident updates, action items, deadlines, product feature requests, customer-specific gaps, process steps, or one-off technical opinions as beliefs.
        - Convert repeated tactical observations into a belief only when the pattern generalizes beyond the specific topic and at least two independent evidence items support it.
        - Use lifecycle "candidate" for early supported patterns, "active" for repeated patterns, "stable" for long-lived patterns already present in current beliefs, and "retired" only for contradicted beliefs.
        - For unsupported tactical evidence, return empty arrays rather than forcing a belief.
        - Set support_count to the number of evidence items backing the belief and contradiction_count to explicit conflicting evidence count.
        - Set last_evidence_at to the newest evidence timestamp used for that belief.
        - Preserve incremental/deep-belief semantics with chunk metadata.

        Scope: \(context.scope.rawValue)
        Entity Key: \(context.entityKey)
        Incremental Window Days: \(context.incrementalWindowDays)
        Chunk Index: \(context.chunkIndex.map(String.init) ?? "none")
        Chunk Count: \(context.chunkCount.map(String.init) ?? "none")

        Current Auto Beliefs:
        \(currentBeliefs.isEmpty ? "none" : currentBeliefs)

        Current Manual Beliefs:
        \(manualBeliefs.isEmpty ? "none" : manualBeliefs)

        Evidence:
        \(evidenceLines.isEmpty ? "none" : evidenceLines)
        """
    }

    private func parseSpaceSummaryOutput(_ output: String) -> (summary: String, openLoops: [String], topics: [SpaceSummaryTopic]) {
        if let payload = decodePayload(SpaceSummaryPayload.self, from: output) {
            let summary = payload.spaceSummary.trimmingCharacters(in: .whitespacesAndNewlines)
            let openLoops = payload.openLoops.map {
                $0.trimmingCharacters(in: .whitespacesAndNewlines)
            }.filter { !$0.isEmpty }
            let topics = payload.meaningfulTopics.map { topic in
                let soWhat = topic.soWhat.trimmingCharacters(in: .whitespacesAndNewlines)
                return SpaceSummaryTopic(
                    title: topic.title.trimmingCharacters(in: .whitespacesAndNewlines),
                    summary: topic.summary.trimmingCharacters(in: .whitespacesAndNewlines),
                    soWhat: soWhat.isEmpty ? "Monitor impact and required decisions." : soWhat
                )
            }
            return (
                summary.isEmpty ? "No summary generated." : summary,
                openLoops,
                topics.filter { !$0.title.isEmpty }
            )
        }

        let lines = output
            .split(whereSeparator: \.isNewline)
            .map { String($0).trimmingCharacters(in: .whitespacesAndNewlines) }
            .filter { !$0.isEmpty }
        let summary = lines.first ?? "No summary generated."
        return (summary, [], [])
    }

    private func parsePersonClusterSummaryOutput(_ output: String, expectedField: String) -> String {
        if let payload = decodePayload(PersonClusterSummaryPayload.self, from: output) {
            return payload.summary.trimmingCharacters(in: .whitespacesAndNewlines)
        }
        return parsePlainTextFieldFromJSON(output: output, key: expectedField)
    }

    private func parseClusterTitleOutput(_ output: String, expectedField: String) -> String {
        if let payload = decodePayload(ClusterTitlePayload.self, from: output) {
            return payload.title.trimmingCharacters(in: .whitespacesAndNewlines)
        }
        return parsePlainTextFieldFromJSON(output: output, key: expectedField)
    }

    private func parseExecQuestionsOutput(_ output: String) -> [String] {
        if let payload = decodePayload(ExecQuestionsPayload.self, from: output), !payload.questions.isEmpty {
            return payload.questions.map {
                $0.trimmingCharacters(in: .whitespacesAndNewlines)
            }.filter { !$0.isEmpty }
        }

        let parsed = output
            .split(whereSeparator: \.isNewline)
            .map { String($0).trimmingCharacters(in: .whitespacesAndNewlines) }
            .map { $0.replacingOccurrences(of: #"^\d+[\).\s-]+"#, with: "", options: .regularExpression) }
            .map { $0.replacingOccurrences(of: #"^[-*]\s+"#, with: "", options: .regularExpression) }
            .filter { !$0.isEmpty }
        return Array(parsed.prefix(7))
    }

    private func parseBeliefReconciliationOutput(_ output: String) -> BeliefReconciliationParsed {
        guard let payload = decodePayload(BeliefReconciliationPayload.self, from: output) else {
            return BeliefReconciliationParsed(
                beliefsToAdd: [],
                beliefsToUpdate: [],
                beliefsToWeaken: [],
                confidence: 0,
                evidenceLinks: []
            )
        }
        return BeliefReconciliationParsed(
            beliefsToAdd: payload.beliefsToAdd.map(\.asBeliefChange).filter { !$0.statement.isEmpty },
            beliefsToUpdate: payload.beliefsToUpdate.map(\.asBeliefChange).filter { !$0.statement.isEmpty },
            beliefsToWeaken: payload.beliefsToWeaken.map(\.asBeliefChange).filter { !$0.statement.isEmpty },
            confidence: payload.confidence,
            evidenceLinks: payload.evidenceLinks
        )
    }

    private func parsePlainTextFieldFromJSON(output: String, key: String) -> String {
        if let dictionary = decodePayload([String: String].self, from: output), let value = dictionary[key] {
            return value.trimmingCharacters(in: .whitespacesAndNewlines)
        }
        let lines = output
            .split(whereSeparator: \.isNewline)
            .map { String($0).trimmingCharacters(in: .whitespacesAndNewlines) }
            .filter { !$0.isEmpty }
        return lines.first ?? ""
    }

    private func decodePayload<T: Decodable>(_ type: T.Type, from output: String) -> T? {
        let trimmed = output.trimmingCharacters(in: .whitespacesAndNewlines)
        if let data = trimmed.data(using: .utf8), let payload = try? decoder.decode(T.self, from: data) {
            return payload
        }
        guard let extracted = extractJSONObject(from: output), let data = extracted.data(using: .utf8) else {
            return nil
        }
        return try? decoder.decode(T.self, from: data)
    }

    private func extractJSONObject(from value: String) -> String? {
        guard let start = value.firstIndex(of: "{"), let end = value.lastIndex(of: "}") else {
            return nil
        }
        guard start <= end else {
            return nil
        }
        return String(value[start...end])
    }

    private static func sha256Hex(_ value: String) -> String {
        let digest = SHA256.hash(data: Data(value.utf8))
        return digest.map { String(format: "%02x", $0) }.joined()
    }

    private static func iso8601String(from date: Date) -> String {
        iso8601Formatter.string(from: date)
    }

    private static let iso8601Formatter: ISO8601DateFormatter = {
        let formatter = ISO8601DateFormatter()
        formatter.formatOptions = [.withInternetDateTime, .withFractionalSeconds]
        return formatter
    }()

    private static let defaultBeliefEvidenceChunkSize = 25
    private static let maxBeliefIncrementalWindowDays = 90
}

/// Lenient decode payload for Codex space summaries.
private struct SpaceSummaryPayload: Decodable {
    var spaceSummary: String
    var openLoops: [String]
    var meaningfulTopics: [SpaceSummaryTopicPayload]

    enum CodingKeys: String, CodingKey {
        case spaceSummary = "space_summary"
        case openLoops = "open_loops"
        case meaningfulTopics = "meaningful_topics"
    }

    init(from decoder: Decoder) throws {
        let container = try decoder.container(keyedBy: CodingKeys.self)
        spaceSummary = try container.decodeIfPresent(String.self, forKey: .spaceSummary) ?? ""
        openLoops = try container.decodeIfPresent([String].self, forKey: .openLoops) ?? []
        meaningfulTopics = try container.decodeIfPresent([SpaceSummaryTopicPayload].self, forKey: .meaningfulTopics) ?? []
    }
}

/// Decode payload for Codex-refined question candidates.
private struct CodexQuestionSynthesisPayload: Decodable {
    var questions: [CodexQuestionSynthesisDraft]
}

/// Topic payload supporting both snake_case and camelCase keys.
private struct SpaceSummaryTopicPayload: Decodable {
    var title: String
    var summary: String
    var soWhat: String

    enum CodingKeys: String, CodingKey {
        case title
        case summary
        case soWhat = "so_what"
        case soWhatCamel = "soWhat"
    }

    init(from decoder: Decoder) throws {
        let container = try decoder.container(keyedBy: CodingKeys.self)
        title = try container.decodeIfPresent(String.self, forKey: .title) ?? ""
        summary = try container.decodeIfPresent(String.self, forKey: .summary) ?? ""
        if let snake = try container.decodeIfPresent(String.self, forKey: .soWhat) {
            soWhat = snake
        } else if let camel = try container.decodeIfPresent(String.self, forKey: .soWhatCamel) {
            soWhat = camel
        } else {
            soWhat = ""
        }
    }
}

/// Encode/decode payload for one person-cluster summary.
private struct PersonClusterSummaryPayload: Codable {
    var summary: String
}

/// Encode/decode payload for one generated cluster title.
private struct ClusterTitlePayload: Codable {
    var title: String
}

/// Encode/decode payload for generated executive questions.
private struct ExecQuestionsPayload: Codable {
    var questions: [String]
}

/// Lenient decode payload for belief reconciliation results.
private struct BeliefReconciliationPayload: Codable {
    var beliefsToAdd: [BeliefChangePayload]
    var beliefsToUpdate: [BeliefChangePayload]
    var beliefsToWeaken: [BeliefChangePayload]
    var confidence: Double
    var evidenceLinks: [String]

    enum CodingKeys: String, CodingKey {
        case beliefsToAdd = "beliefs_to_add"
        case beliefsToUpdate = "beliefs_to_update"
        case beliefsToWeaken = "beliefs_to_weaken"
        case confidence
        case evidenceLinks = "evidence_links"
    }

    init(from decoder: Decoder) throws {
        let container = try decoder.container(keyedBy: CodingKeys.self)
        beliefsToAdd = try container.decodeIfPresent([BeliefChangePayload].self, forKey: .beliefsToAdd) ?? []
        beliefsToUpdate = try container.decodeIfPresent([BeliefChangePayload].self, forKey: .beliefsToUpdate) ?? []
        beliefsToWeaken = try container.decodeIfPresent([BeliefChangePayload].self, forKey: .beliefsToWeaken) ?? []
        confidence = try container.decodeIfPresent(Double.self, forKey: .confidence) ?? 0
        evidenceLinks = try container.decodeIfPresent([String].self, forKey: .evidenceLinks) ?? []
    }
}

/// Raw belief change before normalization into `BeliefChange`.
private struct BeliefChangePayload: Codable {
    var statement: String
    var confidence: Double
    var evidenceLinks: [String]
    var beliefKind: String
    var lifecycle: String
    var supportCount: Int
    var contradictionCount: Int
    var lastEvidenceAt: String

    enum CodingKeys: String, CodingKey {
        case statement
        case confidence
        case evidenceLinks = "evidence_links"
        case beliefKind = "belief_kind"
        case lifecycle
        case supportCount = "support_count"
        case contradictionCount = "contradiction_count"
        case lastEvidenceAt = "last_evidence_at"
    }

    init(from decoder: Decoder) throws {
        let container = try decoder.container(keyedBy: CodingKeys.self)
        statement = try container.decodeIfPresent(String.self, forKey: .statement) ?? ""
        confidence = try container.decodeIfPresent(Double.self, forKey: .confidence) ?? 0
        evidenceLinks = try container.decodeIfPresent([String].self, forKey: .evidenceLinks) ?? []
        beliefKind = try container.decodeIfPresent(String.self, forKey: .beliefKind) ?? "second_order"
        lifecycle = try container.decodeIfPresent(String.self, forKey: .lifecycle) ?? "candidate"
        supportCount = try container.decodeIfPresent(Int.self, forKey: .supportCount) ?? max(1, evidenceLinks.count)
        contradictionCount = try container.decodeIfPresent(Int.self, forKey: .contradictionCount) ?? 0
        lastEvidenceAt = try container.decodeIfPresent(String.self, forKey: .lastEvidenceAt) ?? ""
    }

    var asBeliefChange: BeliefChange {
        let normalizedStatement = statement.trimmingCharacters(in: .whitespacesAndNewlines)
        let normalizedEvidenceLinks = evidenceLinks
            .map { $0.trimmingCharacters(in: .whitespacesAndNewlines) }
            .filter { !$0.isEmpty }
        return BeliefChange(
            statement: normalizedStatement,
            confidence: min(max(confidence, 0), 1),
            evidenceLinks: normalizedEvidenceLinks,
            beliefKind: normalizedMetadata(beliefKind, fallback: "second_order"),
            lifecycle: normalizedMetadata(lifecycle, fallback: "candidate"),
            supportCount: max(0, supportCount),
            contradictionCount: max(0, contradictionCount),
            lastEvidenceAt: lastEvidenceAt.trimmingCharacters(in: .whitespacesAndNewlines)
        )
    }

    private func normalizedMetadata(_ value: String, fallback: String) -> String {
        let normalized = value
            .trimmingCharacters(in: .whitespacesAndNewlines)
            .lowercased()
            .replacingOccurrences(of: " ", with: "_")
            .replacingOccurrences(of: "-", with: "_")
        return normalized.isEmpty ? fallback : normalized
    }
}

/// Normalized belief reconciliation payload used after JSON repair/decoding.
private struct BeliefReconciliationParsed: Hashable {
    var beliefsToAdd: [BeliefChange]
    var beliefsToUpdate: [BeliefChange]
    var beliefsToWeaken: [BeliefChange]
    var confidence: Double
    var evidenceLinks: [String]
}
