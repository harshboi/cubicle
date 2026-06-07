import CryptoKit
import Foundation

/// Refresh jobs exposed to the scheduler, CLI, and Jobs view.
enum RefreshScope: String, CaseIterable, Identifiable {
    case webexSync
    case beliefMaintenance
    case personFocus
    case spaceFocus
    case questions
    case codexJobs

    var id: String { rawValue }

    var title: String {
        switch self {
        case .webexSync:
            return "Webex Sync"
        case .beliefMaintenance:
            return "Belief Maintenance"
        case .personFocus:
            return "Person Focus"
        case .spaceFocus:
            return "Space Focus"
        case .questions:
            return "Questions"
        case .codexJobs:
            return "Codex Jobs"
        }
    }
}

/// Cadence metadata for one refresh scope.
struct RefreshPlan: Hashable {
    var scope: RefreshScope
    var cadenceSeconds: Int
    var description: String
}

/// Final user-visible result for one refresh run.
struct RefreshExecutionResult: Hashable {
    var scope: RefreshScope
    var summary: String
    var completedAt: String
    var reusedCache: Bool?
}

/// Lightweight progress event emitted while a scope is running.
struct RefreshExecutionProgress: Hashable {
    var scope: RefreshScope
    var message: String
}

/// Refresh strategy selected by manual actions, scheduler ticks, and CLI runs.
enum RefreshExecutionMode: String, Hashable {
    case full
    case incremental
    case localOnly
    case cacheAwareLocalOnly
    case cacheAwareFull
    case codexOnly
}

/// Internal result for focus refreshes that may invoke Codex enrichment.
private struct FocusCodexRefreshOutcome {
    var summary: String
    var reusedCache: Bool
}

/// Orchestrates ingestion, local cache rebuilds, Codex enrichment, questions, and beliefs.
///
/// The app and CLI call this class instead of coordinating stores/services directly.
final class NativeRefreshCoordinator {
    let configuration: RuntimeConfiguration
    let configStore: ConfigStore
    let webexClient: NativeWebexClienting
    let knowledgeStore: KnowledgeStore
    let codexRunner: CodexRunner
    let runtimeStore: NativeRuntimeStore
    let webexIngestionService: NativeWebexIngestionService
    let codexOrchestrationService: CodexPromptOrchestrationService
    let questionService: QuestionCandidateService

    /// Wires runtime services for a single runtime root.
    convenience init(configuration: RuntimeConfiguration = .current) {
        self.init(services: AppServices(runtimeStore: NativeRuntimeStore(configuration: configuration)))
    }

    /// Reuses an already-built app service graph.
    init(services: AppServices) {
        self.configuration = services.configuration
        self.configStore = services.configStore
        self.webexClient = services.webexClient
        self.knowledgeStore = services.knowledgeStore
        self.codexRunner = services.codexRunner
        self.runtimeStore = services.runtimeStore
        self.webexIngestionService = services.webexIngestionService
        self.codexOrchestrationService = services.codexOrchestrationService
        self.questionService = services.questionService
    }

    /// Builds scheduler plans from current user settings.
    func defaultPlans() -> [RefreshPlan] {
        let settings = configStore.loadSystemSettings()
        let focusQuestionCadence = min(
            settings.personFocusRefreshMinutes,
            settings.spaceFocusRefreshMinutes
        ) * 60
        return [
            RefreshPlan(scope: .webexSync, cadenceSeconds: settings.webexSyncMinutes * 60, description: "Poll Webex for tracked target updates."),
            RefreshPlan(scope: .personFocus, cadenceSeconds: settings.personFocusRefreshMinutes * 60, description: "Refresh Person Focus clusters with cached Codex title/summary enrichment."),
            RefreshPlan(scope: .spaceFocus, cadenceSeconds: settings.spaceFocusRefreshMinutes * 60, description: "Refresh Space Focus clusters with cached Codex titles, summaries, and exec questions."),
            RefreshPlan(scope: .questions, cadenceSeconds: focusQuestionCadence, description: "Refresh person and space question candidates from focus evidence."),
            RefreshPlan(scope: .codexJobs, cadenceSeconds: 30, description: "Advance native Codex prompt jobs and retry eligible failures."),
            RefreshPlan(scope: .beliefMaintenance, cadenceSeconds: 300, description: "Check deep belief targets and 24h staleness gate.")
        ]
    }

    /// Refreshes the Webex lookup map unless sync is disabled globally.
    func refreshWebexMapFile() async throws -> WebexMapRefreshOutcome {
        guard configStore.loadSystemSettings().webexSyncEnabled else {
            return WebexMapRefreshOutcome(
                mapFileURL: configStore.mapFileURL,
                rooms: 0,
                spaces: 0,
                senders: 0,
                summaryOverride: "Webex map refresh skipped: Webex sync disabled in Settings."
            )
        }
        return try await webexIngestionService.refreshMapFile()
    }

    /// Executes one refresh scope with the requested cache/network strategy.
    func refresh(
        _ scope: RefreshScope,
        mode: RefreshExecutionMode = .full,
        progress: ((RefreshExecutionProgress) async -> Void)? = nil
    ) async throws -> RefreshExecutionResult {
        let completedAt: () -> String = { Self.iso8601String(from: Date()) }
        switch scope {
        case .webexSync:
            guard configStore.loadSystemSettings().webexSyncEnabled else {
                let summary = "Webex sync skipped: disabled in Settings."
                await progress?(
                    RefreshExecutionProgress(
                        scope: scope,
                        message: summary
                    )
                )
                return RefreshExecutionResult(scope: scope, summary: summary, completedAt: completedAt(), reusedCache: nil)
            }
            let syncMode: WebexSyncMode = mode == .full ? .full : .incremental
            let trigger: WebexSyncTriggerReason = mode == .full ? .manual : .scheduled
            let outcome = try await webexIngestionService.syncTrackedTargets(
                mode: syncMode,
                trigger: trigger
            ) { update in
                await progress?(
                    RefreshExecutionProgress(
                        scope: scope,
                        message: update.summary
                    )
                )
            }
            return RefreshExecutionResult(scope: scope, summary: outcome.summary, completedAt: outcome.completedAt, reusedCache: nil)
        case .beliefMaintenance:
            let outcome = try await runBeliefMaintenance()
            return RefreshExecutionResult(scope: scope, summary: outcome, completedAt: completedAt(), reusedCache: nil)
        case .personFocus:
            if mode == .localOnly {
                let outcome = try runtimeStore.refreshPersonFocusCache(forceRebuild: true)
                return RefreshExecutionResult(
                    scope: scope,
                    summary: "Person Focus local rebuild: \(outcome.clusterCount) cluster(s) from \(outcome.normalizedEventCount) event(s).",
                    completedAt: completedAt(),
                    reusedCache: outcome.reusedCache
                )
            }
            if mode == .cacheAwareLocalOnly {
                let outcome = try runtimeStore.refreshPersonFocusCache(forceRebuild: false)
                let action = outcome.reusedCache ? "cache reused" : "local rebuild"
                return RefreshExecutionResult(
                    scope: scope,
                    summary: "Person Focus \(action): \(outcome.clusterCount) cluster(s) from \(outcome.normalizedEventCount) event(s).",
                    completedAt: completedAt(),
                    reusedCache: outcome.reusedCache
                )
            }
            let outcome = try await runPersonFocusWithCodex(
                forceLocalRebuild: mode == .full,
                skipIfCacheReused: mode == .cacheAwareFull
            )
            return RefreshExecutionResult(
                scope: scope,
                summary: outcome.summary,
                completedAt: completedAt(),
                reusedCache: outcome.reusedCache
            )
        case .spaceFocus:
            if mode == .localOnly {
                let outcome = try runtimeStore.refreshSpaceFocusCache(forceRebuild: true)
                return RefreshExecutionResult(
                    scope: scope,
                    summary: "Space Focus local rebuild: \(outcome.clusterCount) conversation(s) from \(outcome.normalizedEventCount) event(s).",
                    completedAt: completedAt(),
                    reusedCache: outcome.reusedCache
                )
            }
            if mode == .cacheAwareLocalOnly {
                let outcome = try runtimeStore.refreshSpaceFocusCache(forceRebuild: false)
                let action = outcome.reusedCache ? "cache reused" : "local rebuild"
                return RefreshExecutionResult(
                    scope: scope,
                    summary: "Space Focus \(action): \(outcome.clusterCount) conversation(s) from \(outcome.normalizedEventCount) event(s).",
                    completedAt: completedAt(),
                    reusedCache: outcome.reusedCache
                )
            }
            let outcome = try await runSpaceFocusWithCodex(
                forceLocalRebuild: mode == .full,
                skipIfCacheReused: mode == .cacheAwareFull
            )
            return RefreshExecutionResult(
                scope: scope,
                summary: outcome.summary,
                completedAt: completedAt(),
                reusedCache: outcome.reusedCache
            )
        case .questions:
            let spaceCache = try runtimeStore.loadFocusCache(kind: .space)
            let personCache = try runtimeStore.loadFocusCache(kind: .person)
            let outcome = try await questionServiceForCurrentSettings().refreshQuestionCandidates(
                spaceCache: spaceCache,
                personCache: personCache
            )
            return RefreshExecutionResult(scope: scope, summary: outcome.summary, completedAt: outcome.completedAt, reusedCache: nil)
        case .codexJobs:
            let outcome = runtimeStore.refreshCodexJobs()
            return RefreshExecutionResult(
                scope: scope,
                summary: "Codex jobs: runs=\(outcome.runDirectoryCount), queued=\(outcome.queuedPromptCount).",
                completedAt: completedAt(),
                reusedCache: nil
            )
        }
    }

    /// Per-item cluster title enrichment result; failures stay isolated to their seed.
    private struct ClusterTitleEnrichmentOutcome {
        var seeds: [FocusClusterSeed]
        var generatedCount: Int
        var errors: [String]
    }

    /// Rebuilds person focus and optionally enriches clusters with titles/summaries.
    private func runPersonFocusWithCodex(
        forceLocalRebuild: Bool,
        skipIfCacheReused: Bool,
        maxCodexItems: Int? = nil,
        maxTitlesPerItem: Int = 3
    ) async throws -> FocusCodexRefreshOutcome {
        let localOutcome = try runtimeStore.refreshPersonFocusCache(forceRebuild: forceLocalRebuild)
        let currentCache = try runtimeStore.loadFocusCache(kind: .person)
        let settings = configStore.loadSystemSettings()
        let titlesEnabled = settings.codexFeatureEnabled(.clusterTitles)
        let summariesEnabled = settings.codexFeatureEnabled(.personFocusSummaries)
        if skipIfCacheReused,
           localOutcome.reusedCache,
           !focusCacheNeedsCodexEnrichment(currentCache, kind: .person) {
            return FocusCodexRefreshOutcome(
                summary: "Person Focus cache reused: Codex enrichment skipped because source evidence is unchanged.",
                reusedCache: true
            )
        }
        let sourceCache = currentCache
        var refreshedItems: [FocusItem] = []
        refreshedItems.reserveCapacity(sourceCache.items.count)

        let codexItemLimit = maxCodexItems ?? Int.max
        var codexProcessed = 0
        var summaryGenerated = 0
        var titleGenerated = 0
        var codexErrors: [String] = []
        var disabledNotes: [String] = []
        if !titlesEnabled {
            disabledNotes.append("cluster titles disabled")
        }
        if !summariesEnabled {
            disabledNotes.append("person summaries disabled")
        }

        for item in sourceCache.items {
            let events = item.normalizedEvents(kind: .person)
            let seeds = FocusClusterSeed.makeSeeds(kind: .person, events: events)
            let liveSettings = configStore.loadSystemSettings()
            let liveTitlesEnabled = liveSettings.codexFeatureEnabled(.clusterTitles)
            let liveSummariesEnabled = liveSettings.codexFeatureEnabled(.personFocusSummaries)
            guard (liveTitlesEnabled || liveSummariesEnabled), codexProcessed < codexItemLimit, !seeds.isEmpty else {
                refreshedItems.append(
                    item.assembledDetailPayload(
                        kind: .person,
                        focusDays: sourceCache.focusDays,
                        clusterSeeds: seeds
                    )
                )
                continue
            }

            do {
                let titleOutcome = await enrichClusterTitles(
                    kind: .person,
                    item: item,
                    seeds: seeds,
                    maxTitles: liveTitlesEnabled ? maxTitlesPerItem : 0
                )
                codexProcessed += 1
                titleGenerated += titleOutcome.generatedCount
                codexErrors.append(contentsOf: titleOutcome.errors.map { "\(item.title): \($0)" })

                guard liveSummariesEnabled else {
                    refreshedItems.append(
                        item.assembledDetailPayload(
                            kind: .person,
                            focusDays: sourceCache.focusDays,
                            clusterSeeds: titleOutcome.seeds
                        )
                    )
                    continue
                }

                let summary = try await codexOrchestrationService.generatePersonClusterSummary(
                    context: personSummaryContext(
                        item: item,
                        seeds: titleOutcome.seeds
                    ),
                    workingDirectory: configuration.runtimeRoot
                )
                refreshedItems.append(
                    itemWithPersonCodexSections(
                        item: item,
                        summary: summary,
                        clusterSeeds: titleOutcome.seeds,
                        focusDays: sourceCache.focusDays
                    )
                )
                summaryGenerated += 1
            } catch {
                codexErrors.append("\(item.title): \(error.localizedDescription)")
                refreshedItems.append(
                    item.assembledDetailPayload(
                        kind: .person,
                        focusDays: sourceCache.focusDays,
                        clusterSeeds: seeds
                    )
                )
            }
        }

        let refreshedCache = FocusCache(
            focusDays: sourceCache.focusDays,
            items: refreshedItems,
            updatedAt: Self.iso8601String(from: Date()),
            countLabel: sourceCache.countLabel,
            recentMessages: sourceCache.recentMessages,
            summaryGenerationInProgress: false,
            subjectsProcessed: sourceCache.subjectsTotal,
            subjectsTotal: sourceCache.subjectsTotal
        )
        try runtimeStore.saveFocusCache(refreshedCache, kind: .person)
        let outcome = try runtimeStore.refreshFocusCache(
            kind: .person,
            sourceURL: runtimeStore.snapshotURL(kind: .person),
            forceRebuild: true
        )
        var summary = "Person Focus rebuilt \(outcome.clusterCount) cluster(s) from \(outcome.normalizedEventCount) event(s); Codex processed \(codexProcessed) person(s), summarized \(summaryGenerated), titled \(titleGenerated) cluster(s)."
        if !disabledNotes.isEmpty {
            summary += " Disabled: \(disabledNotes.joined(separator: ", "))."
        }
        if !codexErrors.isEmpty {
            summary += " Codex errors: \(codexErrors.prefix(2).joined(separator: " | "))"
        }
        return FocusCodexRefreshOutcome(summary: summary, reusedCache: false)
    }

    /// Rebuilds space focus and optionally enriches clusters, summaries, and exec questions.
    private func runSpaceFocusWithCodex(
        forceLocalRebuild: Bool,
        skipIfCacheReused: Bool,
        maxCodexItems: Int? = nil,
        maxTitlesPerItem: Int = 3
    ) async throws -> FocusCodexRefreshOutcome {
        let localOutcome = try runtimeStore.refreshSpaceFocusCache(forceRebuild: forceLocalRebuild)
        let currentCache = try runtimeStore.loadFocusCache(kind: .space)
        let settings = configStore.loadSystemSettings()
        let titlesEnabled = settings.codexFeatureEnabled(.clusterTitles)
        let summariesEnabled = settings.codexFeatureEnabled(.spaceFocusSummaries)
        let execQuestionsEnabled = settings.codexFeatureEnabled(.execQuestions)
        if skipIfCacheReused,
           localOutcome.reusedCache,
           !focusCacheNeedsCodexEnrichment(currentCache, kind: .space) {
            return FocusCodexRefreshOutcome(
                summary: "Space Focus cache reused: Codex enrichment skipped because source evidence is unchanged.",
                reusedCache: true
            )
        }
        let sourceCache = currentCache
        var refreshedItems: [FocusItem] = []
        refreshedItems.reserveCapacity(sourceCache.items.count)

        let codexItemLimit = maxCodexItems ?? Int.max
        var codexProcessed = 0
        var summaryGenerated = 0
        var titleGenerated = 0
        var execQuestionSections = 0
        var codexErrors: [String] = []
        var disabledNotes: [String] = []
        if !titlesEnabled {
            disabledNotes.append("cluster titles disabled")
        }
        if !summariesEnabled {
            disabledNotes.append("space summaries disabled")
        }
        if !execQuestionsEnabled {
            disabledNotes.append("exec questions disabled")
        }
        if execQuestionsEnabled && !summariesEnabled {
            disabledNotes.append("exec questions skipped because space summaries are disabled")
        }
        let importantExecutives = try configStore.importantExecutives().map { target in
            ImportantExecutive(
                name: target.label,
                email: target.email
            )
        }

        for item in sourceCache.items {
            let events = item.normalizedEvents(kind: .space)
            let seeds = FocusClusterSeed.makeSeeds(kind: .space, events: events)
            let liveSettings = configStore.loadSystemSettings()
            let liveTitlesEnabled = liveSettings.codexFeatureEnabled(.clusterTitles)
            let liveSummariesEnabled = liveSettings.codexFeatureEnabled(.spaceFocusSummaries)
            let liveExecQuestionsEnabled = liveSettings.codexFeatureEnabled(.execQuestions)
            guard (liveTitlesEnabled || liveSummariesEnabled || liveExecQuestionsEnabled), codexProcessed < codexItemLimit, !seeds.isEmpty else {
                refreshedItems.append(
                    item.assembledDetailPayload(
                        kind: .space,
                        focusDays: sourceCache.focusDays,
                        clusterSeeds: seeds
                    )
                )
                continue
            }

            do {
                let titleOutcome = await enrichClusterTitles(
                    kind: .space,
                    item: item,
                    seeds: seeds,
                    maxTitles: liveTitlesEnabled ? maxTitlesPerItem : 0
                )
                codexProcessed += 1
                titleGenerated += titleOutcome.generatedCount
                codexErrors.append(contentsOf: titleOutcome.errors.map { "\(item.title): \($0)" })

                let clusters = titleOutcome.seeds.prefix(5).map { seed in
                    SpaceConversationCluster(
                        title: seed.title,
                        summary: seed.sampleMessages.joined(separator: " "),
                        openLoops: [seed.soWhat]
                    )
                }
                guard liveSummariesEnabled else {
                    refreshedItems.append(
                        item.assembledDetailPayload(
                            kind: .space,
                            focusDays: sourceCache.focusDays,
                            clusterSeeds: titleOutcome.seeds
                        )
                    )
                    continue
                }

                let summary = try await codexOrchestrationService.generateSpaceSummary(
                    context: SpaceSummaryContext(
                        roomID: item.id,
                        roomTitle: item.title,
                        clusters: clusters,
                        openLoops: titleOutcome.seeds.prefix(5).map(\.soWhat),
                        previousSummary: item.firstDetailLine(prefix: "Space summary:"),
                        previousGeneratedAt: item.firstDetailLine(prefix: "Summary generated:")
                    ),
                    workingDirectory: configuration.runtimeRoot
                )
                try persistTopics(summary: summary, item: item)
                summaryGenerated += 1

                let execQuestions: [ExecQuestionsResult]
                if liveExecQuestionsEnabled {
                    do {
                        let webexRoomID = webexRoomID(for: item.id)
                        let participants = try await roomParticipants(roomID: webexRoomID)
                        execQuestions = try await codexOrchestrationService.generateExecQuestionsForRoom(
                            roomID: webexRoomID,
                            roomTitle: item.title,
                            summary: summary.summary,
                            openLoops: summary.openLoops,
                            clusters: clusters,
                            roomParticipants: participants,
                            importantExecutives: importantExecutives,
                            execBeliefsByEmail: [:],
                            workingDirectory: configuration.runtimeRoot
                        )
                        execQuestionSections += execQuestions.count
                    } catch {
                        codexErrors.append("\(item.title) exec questions: \(error.localizedDescription)")
                        execQuestions = []
                    }
                } else {
                    execQuestions = []
                }

                let updatedItem = itemWithCodexSections(
                    item: item,
                    summary: summary,
                    execQuestions: execQuestions,
                    clusterSeeds: titleOutcome.seeds,
                    focusDays: sourceCache.focusDays
                )
                refreshedItems.append(updatedItem)
            } catch {
                codexErrors.append("\(item.title): \(error.localizedDescription)")
                refreshedItems.append(
                    item.assembledDetailPayload(
                        kind: .space,
                        focusDays: sourceCache.focusDays,
                        clusterSeeds: seeds
                    )
                )
            }
        }

        let refreshedCache = FocusCache(
            focusDays: sourceCache.focusDays,
            items: refreshedItems,
            updatedAt: Self.iso8601String(from: Date()),
            countLabel: sourceCache.countLabel,
            recentMessages: sourceCache.recentMessages,
            summaryGenerationInProgress: false,
            subjectsProcessed: sourceCache.subjectsTotal,
            subjectsTotal: sourceCache.subjectsTotal
        )
        try runtimeStore.saveFocusCache(refreshedCache, kind: .space)
        let outcome = try runtimeStore.refreshFocusCache(
            kind: .space,
            sourceURL: runtimeStore.snapshotURL(kind: .space),
            forceRebuild: true
        )
        var summary = "Space Focus rebuilt \(outcome.clusterCount) cluster(s) from \(outcome.normalizedEventCount) event(s); Codex processed \(codexProcessed) space(s), summarized \(summaryGenerated), titled \(titleGenerated) cluster(s), generated \(execQuestionSections) exec-question section(s)."
        if !disabledNotes.isEmpty {
            summary += " Disabled: \(disabledNotes.joined(separator: ", "))."
        }
        if !codexErrors.isEmpty {
            summary += " Codex errors: \(codexErrors.prefix(2).joined(separator: " | "))"
        }
        return FocusCodexRefreshOutcome(summary: summary, reusedCache: false)
    }

    /// Adds Codex titles to the first few local cluster seeds without blocking fallback output.
    private func enrichClusterTitles(
        kind: FocusKind,
        item: FocusItem,
        seeds: [FocusClusterSeed],
        maxTitles: Int
    ) async -> ClusterTitleEnrichmentOutcome {
        guard maxTitles > 0 else {
            return ClusterTitleEnrichmentOutcome(seeds: seeds, generatedCount: 0, errors: [])
        }

        var enriched: [FocusClusterSeed] = []
        enriched.reserveCapacity(seeds.count)
        var generatedCount = 0
        var errors: [String] = []

        for (index, seed) in seeds.enumerated() {
            guard index < maxTitles else {
                enriched.append(seed)
                continue
            }

            do {
                let result = try await codexOrchestrationService.generateClusterTitle(
                    context: clusterTitleContext(kind: kind, item: item, seed: seed),
                    workingDirectory: configuration.runtimeRoot
                )
                let title = sanitizedCodexTitle(result.title, fallback: seed.title)
                enriched.append(seedWithTitle(seed, title: title))
                generatedCount += 1
            } catch {
                errors.append("\(seed.title): \(error.localizedDescription)")
                enriched.append(seed)
            }
        }

        return ClusterTitleEnrichmentOutcome(
            seeds: enriched,
            generatedCount: generatedCount,
            errors: errors
        )
    }

    private func focusCacheNeedsCodexEnrichment(_ cache: FocusCache, kind: FocusKind) -> Bool {
        cache.items.contains { item in
            !item.normalizedEvents(kind: kind).isEmpty && !hasCodexSummary(item, kind: kind)
        }
    }

    private func hasCodexSummary(_ item: FocusItem, kind: FocusKind) -> Bool {
        switch kind {
        case .person:
            return !item.firstDetailLine(prefix: "Person summary:").isEmpty
                && item.firstDetailLine(prefix: "Summary source:").localizedCaseInsensitiveContains("codex")
        case .space:
            return !item.firstDetailLine(prefix: "Space summary:").isEmpty
                && item.firstDetailLine(prefix: "Space summary source:").localizedCaseInsensitiveContains("codex")
        }
    }

    private func clusterTitleContext(kind: FocusKind, item: FocusItem, seed: FocusClusterSeed) -> ClusterTitleContext {
        let focusKind: ClusterFocusKind = kind == .person ? .person : .space
        let signalLines = [
            "Scope: \(item.title)",
            "Latest timestamp: \(seed.lastTimestampLabel)",
            "Participants: \(seed.participants.prefix(8).joined(separator: ", "))",
            "Local so what: \(seed.soWhat)"
        ] + seed.sampleMessages.prefix(4)

        let summaryLines = ([seed.soWhat] + seed.sampleMessages.prefix(6))
            .map { $0.trimmingCharacters(in: .whitespacesAndNewlines) }
            .filter { !$0.isEmpty }

        return ClusterTitleContext(
            focusKind: focusKind,
            clusterID: "\(item.id)-\(seed.id)",
            clusterSummary: summaryLines.joined(separator: "\n"),
            supportingSignals: signalLines,
            existingTitle: seed.title
        )
    }

    private func seedWithTitle(_ seed: FocusClusterSeed, title: String) -> FocusClusterSeed {
        let trimmedTitle = title.trimmingCharacters(in: .whitespacesAndNewlines)
        let effectiveTitle = trimmedTitle.isEmpty ? seed.title : trimmedTitle
        let oldLower = seed.title.lowercased()
        let newLower = effectiveTitle.lowercased()
        let updatedSoWhat = seed.soWhat.replacingOccurrences(of: oldLower, with: newLower)

        return FocusClusterSeed(
            id: seed.id,
            key: seed.key,
            title: effectiveTitle,
            eventCount: seed.eventCount,
            lastTimestampLabel: seed.lastTimestampLabel,
            participants: seed.participants,
            sampleMessages: seed.sampleMessages,
            soWhat: updatedSoWhat
        )
    }

    private func sanitizedCodexTitle(_ raw: String, fallback: String) -> String {
        var title = raw
            .trimmingCharacters(in: .whitespacesAndNewlines)
            .replacingOccurrences(of: #"\s+"#, with: " ", options: .regularExpression)
            .trimmingCharacters(in: CharacterSet(charactersIn: "\"'`*_ "))

        title = title.replacingOccurrences(of: #"[.!?]+$"#, with: "", options: .regularExpression)
        let words = title.split(whereSeparator: \.isWhitespace).map(String.init)
        if words.count > 8 {
            title = words.prefix(8).joined(separator: " ")
        }
        if title.count < 3 {
            return fallback
        }
        return title
    }

    /// Runs gated belief reconciliation for configured targets with indexed evidence.
    private func runBeliefMaintenance() async throws -> String {
        guard configStore.loadSystemSettings().codexFeatureEnabled(.beliefs) else {
            return "Belief maintenance skipped: Codex belief analysis is disabled in Settings."
        }
        try knowledgeStore.bootstrap()
        let targets = try configStore.beliefTargets()
        let orchestrationTargets = try targets.compactMap { target -> BeliefReconciliationTarget? in
            guard let scope = beliefScope(for: target) else {
                return nil
            }
            let entityKey = beliefEntityKey(for: target, scope: scope)
            guard !entityKey.isEmpty else {
                return nil
            }

            let current = try knowledgeStore.loadBeliefs(scope: scope.rawValue, entityKey: entityKey)
                .map(beliefSnapshot)
            let manual = try knowledgeStore.loadManualBeliefs(scope: scope.rawValue, entityKey: entityKey)
                .map(beliefSnapshot)
            let evidence = try beliefEvidence(for: target, scope: scope, entityKey: entityKey)
            guard !evidence.isEmpty else {
                return nil
            }
            let stateRecord = try knowledgeStore.loadBeliefReconciliationState(
                scope: scope.rawValue,
                entityKey: entityKey
            )
            let state = BeliefReconciliationState(
                lastRunAt: stateRecord?.lastRunAt,
                lastEvidenceHash: stateRecord?.lastEvidenceHash
            )
            return BeliefReconciliationTarget(
                context: BeliefReconciliationContext(
                    scope: scope,
                    entityKey: entityKey,
                    currentBeliefs: current,
                    manualBeliefs: manual,
                    evidence: evidence,
                    forceRefresh: false,
                    incrementalWindowDays: 60,
                    chunkIndex: nil,
                    chunkCount: nil
                ),
                state: state
            )
        }

        guard !orchestrationTargets.isEmpty else {
            return "Belief maintenance skipped: no configured targets had indexed evidence."
        }

        let batch = try await codexOrchestrationService.runBeliefReconciliationBatch(
            targets: orchestrationTargets,
            workingDirectory: configuration.runtimeRoot
        )
        var persistedBeliefs = 0
        for entry in batch.outcomes {
            try knowledgeStore.upsertBeliefReconciliationState(
                BeliefReconciliationStateRecord(
                    scope: entry.key.scope.rawValue,
                    entityKey: entry.key.entityKey,
                    lastRunAt: entry.outcome.nextState.lastRunAt,
                    lastEvidenceHash: entry.outcome.nextState.lastEvidenceHash,
                    updatedAt: Self.iso8601String(from: Date())
                )
            )
            if let result = entry.outcome.result {
                persistedBeliefs += try persistBeliefResult(result)
            }
        }
        return "Belief maintenance evaluated \(batch.outcomes.count) target(s), triggered \(batch.triggeredCount), skipped \(batch.skippedCount), persisted \(persistedBeliefs) belief(s)."
    }

    /// Enables Codex question synthesis only when the feature flag is active.
    private func questionServiceForCurrentSettings() -> QuestionCandidateService {
        let synthesizer: QuestionCandidateSynthesizing? = configStore
            .loadSystemSettings()
            .codexFeatureEnabled(.questionSynthesis)
            ? codexOrchestrationService
            : nil
        return QuestionCandidateService(
            knowledgeStore: knowledgeStore,
            questionSynthesizer: synthesizer
        )
    }

    private func roomParticipants(roomID: String) async throws -> [SpaceParticipant] {
        let memberships = try await webexClient.memberships(roomID: roomID)
        return memberships.map {
            SpaceParticipant(name: $0.personDisplayName, email: $0.personEmail)
        }
    }

    private func webexRoomID(for focusItemID: String) -> String {
        let trimmed = focusItemID.trimmingCharacters(in: .whitespacesAndNewlines)
        let prefix = "spacefocus:"
        if trimmed.hasPrefix(prefix) {
            return String(trimmed.dropFirst(prefix.count))
        }
        return trimmed
    }

    private func personSummaryContext(item: FocusItem, seeds: [FocusClusterSeed]) -> PersonClusterSummaryContext {
        let highlights = seeds.prefix(10).map { seed in
            let examples = seed.sampleMessages
                .prefix(3)
                .map { $0.trimmingCharacters(in: .whitespacesAndNewlines) }
                .filter { !$0.isEmpty }
                .joined(separator: " | ")
            if examples.isEmpty {
                return "\(seed.title): \(seed.soWhat)"
            }
            return "\(seed.title): \(seed.soWhat) Evidence: \(examples)"
        }

        let existingSummary = item.detailTailLines.first { line in
            line.localizedCaseInsensitiveContains("summary")
        }

        return PersonClusterSummaryContext(
            personID: item.id,
            personLabel: item.title,
            conversationHighlights: highlights,
            existingSummary: existingSummary
        )
    }

    private func itemWithPersonCodexSections(
        item: FocusItem,
        summary: PersonClusterSummaryResult,
        clusterSeeds: [FocusClusterSeed],
        focusDays: Int
    ) -> FocusItem {
        let assembled = item.assembledDetailPayload(
            kind: .person,
            focusDays: focusDays,
            clusterSeeds: clusterSeeds
        )
        let existingTail = stripPersonCodexGeneratedContent(from: assembled.detailTailLines)
        var tailLines = [
            "Person summary: \(summary.summary)",
            "Summary source: \(summary.source)",
            "Summary generated: \(summary.generatedAt)",
            "",
            "Codex-named conversation clusters:"
        ]
        for seed in clusterSeeds.prefix(8) {
            tailLines.append("- \(seed.title): \(seed.soWhat)")
        }
        if !existingTail.isEmpty {
            tailLines.append("")
            tailLines.append(contentsOf: existingTail)
        }

        let flattened = flattenedFocusDetailLines(
            intro: assembled.detailIntroLines,
            sections: assembled.detailSections,
            tail: tailLines
        )

        return FocusItem(
            id: assembled.id,
            title: assembled.title,
            subtitle: assembled.subtitle,
            meta: assembled.meta,
            timestamp: assembled.timestamp,
            badge: assembled.badge,
            statusBadge: assembled.statusBadge,
            detailLines: flattened,
            detailIntroLines: assembled.detailIntroLines,
            detailSections: assembled.detailSections,
            detailTailLines: tailLines
        )
    }

    private func itemWithCodexSections(
        item: FocusItem,
        summary: SpaceSummaryResult,
        execQuestions: [ExecQuestionsResult],
        clusterSeeds: [FocusClusterSeed],
        focusDays: Int
    ) -> FocusItem {
        var lines = stripCodexGeneratedContent(from: item.detailLines)
        lines.removeAll { $0.trimmingCharacters(in: .whitespacesAndNewlines).isEmpty }
        let posture = summary.openLoops.first ?? summary.topics.first?.soWhat ?? "Review generated topics and confirm next owner."
        var codexLines = [
            "Space summary: \(summary.summary)",
            "Current posture / next move: \(posture)",
            "Guidance freshness: Updated by \(summary.source) at \(summary.generatedAt) from live Webex sync.",
            "Space summary source: \(summary.source)",
            "Summary generated: \(summary.generatedAt)",
            ""
        ]
        codexLines.append(contentsOf: lines)
        for execResult in execQuestions {
            codexLines.append("")
            codexLines.append(execResult.header)
            codexLines.append(contentsOf: execResult.questions.prefix(7).enumerated().map { index, question in
                "\(index + 1). \(question)"
            })
        }

        let updated = FocusItem(
            id: item.id,
            title: item.title,
            subtitle: item.subtitle,
            meta: item.meta,
            timestamp: item.timestamp,
            badge: item.badge,
            statusBadge: item.statusBadge,
            detailLines: codexLines,
            detailIntroLines: [],
            detailSections: [],
            detailTailLines: []
        )
        return updated.assembledDetailPayload(kind: .space, focusDays: focusDays, clusterSeeds: clusterSeeds)
    }

    private func stripPersonCodexGeneratedContent(from lines: [String]) -> [String] {
        var result: [String] = []
        var skippingClusterList = false
        for line in lines {
            let trimmed = line.trimmingCharacters(in: .whitespacesAndNewlines)
            if trimmed.hasPrefix("Person summary:")
                || trimmed.hasPrefix("Summary source:")
                || trimmed.hasPrefix("Summary generated:") {
                continue
            }
            if trimmed.hasPrefix("Codex-named conversation clusters:") {
                skippingClusterList = true
                continue
            }
            if skippingClusterList {
                if trimmed.hasPrefix("- ") || trimmed.isEmpty {
                    continue
                }
                skippingClusterList = false
            }
            result.append(line)
        }
        return result
    }

    private func flattenedFocusDetailLines(
        intro: [String],
        sections: [FocusDetailSection],
        tail: [String]
    ) -> [String] {
        var lines: [String] = []
        lines.append(contentsOf: intro)
        if !intro.isEmpty, !sections.isEmpty, lines.last?.isEmpty == false {
            lines.append("")
        }
        for (index, section) in sections.enumerated() {
            lines.append(section.header)
            lines.append(contentsOf: section.lines)
            if index < sections.count - 1 {
                lines.append("")
            }
        }
        if !tail.isEmpty {
            if lines.last?.isEmpty == false {
                lines.append("")
            }
            lines.append(contentsOf: tail)
        }
        return lines
    }

    private func stripCodexGeneratedContent(from lines: [String]) -> [String] {
        var result: [String] = []
        var skippingGeneratedSection = false
        for line in lines {
            let trimmed = line.trimmingCharacters(in: .whitespacesAndNewlines)
            if trimmed.hasPrefix("Space summary:")
                || trimmed.hasPrefix("Current posture / next move:")
                || trimmed.hasPrefix("Guidance freshness:")
                || trimmed.hasPrefix("Space summary source:")
                || trimmed.hasPrefix("Summary generated:") {
                continue
            }
            if trimmed.hasPrefix("Meaningful topics from Codex")
                || (trimmed.hasPrefix("What are the Questions running in the Exec's (") && trimmed.hasSuffix("Mind:")) {
                skippingGeneratedSection = true
                continue
            }
            if skippingGeneratedSection {
                if trimmed.hasPrefix("Recent conversations (last ") {
                    skippingGeneratedSection = false
                    result.append(line)
                }
                continue
            }
            result.append(line)
        }
        return result
    }

    private func persistTopics(summary: SpaceSummaryResult, item: FocusItem) throws {
        for (index, topic) in summary.topics.prefix(5).enumerated() {
            try knowledgeStore.upsertTopic(
                TopicRecord(
                    id: "space-topic-\(safeKey(summary.roomID))-\(index + 1)-\(safeKey(topic.title))",
                    focusKind: FocusKind.space.rawValue,
                    scope: KnowledgeBeliefScope.space.rawValue,
                    entityKey: summary.roomID,
                    topicKey: safeKey(topic.title),
                    title: topic.title,
                    summary: topic.summary,
                    soWhat: topic.soWhat,
                    sourceLabel: summary.source,
                    score: Double(5 - index),
                    generatedAt: summary.generatedAt,
                    updatedAt: summary.generatedAt
                )
            )
        }
    }

    private func persistBeliefResult(_ result: BeliefReconciliationResult) throws -> Int {
        let taggedChanges: [(change: BeliefChange, fallbackLifecycle: String)] =
            result.beliefsToAdd.map { ($0, "candidate") }
            + result.beliefsToUpdate.map { ($0, "active") }
            + result.beliefsToWeaken.map { ($0, "retired") }
        guard !taggedChanges.isEmpty else {
            return 0
        }
        var records: [BeliefRecord] = []
        records.reserveCapacity(taggedChanges.count)
        for taggedChange in taggedChanges {
            let change = taggedChange.change
            let statement = change.statement.trimmingCharacters(in: .whitespacesAndNewlines)
            guard !statement.isEmpty else { continue }
            let evidenceLinks = uniqueStrings(result.evidenceLinks + change.evidenceLinks)
            let lastEvidenceAt = change.lastEvidenceAt.trimmingCharacters(in: .whitespacesAndNewlines)
            records.append(
                BeliefRecord(
                    id: stableBeliefID(
                        scope: result.scope,
                        entityKey: result.entityKey,
                        statement: statement
                    ),
                    scope: result.scope.rawValue,
                    entityKey: result.entityKey,
                    statement: statement,
                    confidence: max(0, min(1, change.confidence)),
                    updatedAt: result.generatedAt,
                    isManual: false,
                    evidenceLinks: evidenceLinks,
                    createdAt: result.generatedAt,
                    beliefKind: normalizedBeliefMetadata(change.beliefKind, fallback: "second_order"),
                    lifecycle: normalizedBeliefMetadata(change.lifecycle, fallback: taggedChange.fallbackLifecycle),
                    supportCount: max(0, change.supportCount),
                    contradictionCount: max(0, change.contradictionCount),
                    lastEvidenceAt: lastEvidenceAt.isEmpty ? result.generatedAt : lastEvidenceAt
                )
            )
        }
        try knowledgeStore.upsertBeliefs(records)
        return records.count
    }

    private func normalizedBeliefMetadata(_ value: String, fallback: String) -> String {
        let normalized = value
            .trimmingCharacters(in: .whitespacesAndNewlines)
            .lowercased()
            .replacingOccurrences(of: " ", with: "_")
            .replacingOccurrences(of: "-", with: "_")
        return normalized.isEmpty ? fallback : normalized
    }

    private func stableBeliefID(scope: BeliefScope, entityKey: String, statement: String) -> String {
        let keyMaterial = "\(scope.rawValue)|\(entityKey)|\(statement)"
        let digest = SHA256.hash(data: Data(keyMaterial.utf8))
            .map { String(format: "%02x", $0) }
            .joined()
        return "auto-\(scope.rawValue)-\(safeKey(entityKey))-\(String(digest.prefix(16)))"
    }

    private func beliefScope(for target: ConfigTarget) -> BeliefScope? {
        switch target.kind {
        case .space:
            return .space
        case .person:
            return .person
        case .unknown:
            return nil
        }
    }

    private func beliefEntityKey(for target: ConfigTarget, scope: BeliefScope) -> String {
        switch scope {
        case .space:
            return target.roomID.trimmingCharacters(in: .whitespacesAndNewlines)
        case .person:
            let email = target.email.trimmingCharacters(in: .whitespacesAndNewlines).lowercased()
            return email.isEmpty ? target.roomID.trimmingCharacters(in: .whitespacesAndNewlines) : email
        case .global:
            return BeliefScope.globalEntityKey
        }
    }

    private func beliefEvidence(
        for target: ConfigTarget,
        scope: BeliefScope,
        entityKey: String
    ) throws -> [BeliefEvidence] {
        let records: [BeliefEvidenceRecord]
        switch scope {
        case .space:
            records = try knowledgeStore.loadBeliefEvidence(
                scope: KnowledgeBeliefScope.space.rawValue,
                entityKey: entityKey,
                limit: 500
            )
        case .person:
            let directRoomID = target.roomID.trimmingCharacters(in: .whitespacesAndNewlines)
            if directRoomID.isEmpty {
                records = try knowledgeStore.loadBeliefEvidence(
                    scope: KnowledgeBeliefScope.person.rawValue,
                    entityKey: entityKey,
                    limit: 500
                )
            } else {
                records = try knowledgeStore.loadBeliefEvidence(
                    scope: KnowledgeBeliefScope.space.rawValue,
                    entityKey: directRoomID,
                    limit: 500
                )
            }
        case .global:
            records = try knowledgeStore.loadBeliefEvidence(
                scope: KnowledgeBeliefScope.global.rawValue,
                entityKey: KnowledgeBeliefScope.globalEntityKey,
                limit: 500
            )
        }
        return records.map {
            BeliefEvidence(
                id: $0.id,
                text: $0.text,
                source: "\($0.source):\($0.sourceID)",
                occurredAt: $0.occurredAt
            )
        }
    }

    private func beliefSnapshot(_ record: BeliefRecord) -> BeliefSnapshot {
        BeliefSnapshot(
            statement: record.statement,
            confidence: record.confidence,
            evidenceLinks: record.evidenceLinks,
            beliefKind: record.beliefKind,
            lifecycle: record.lifecycle,
            supportCount: record.supportCount,
            contradictionCount: record.contradictionCount,
            lastEvidenceAt: record.lastEvidenceAt
        )
    }

    private func safeKey(_ value: String) -> String {
        let allowed = Set("abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ0123456789-_")
        let cleaned = value.map { allowed.contains($0) ? $0 : "-" }
        let collapsed = String(cleaned)
            .replacingOccurrences(of: #"-+"#, with: "-", options: .regularExpression)
            .trimmingCharacters(in: CharacterSet(charactersIn: "-"))
        if collapsed.isEmpty {
            return "value"
        }
        return String(collapsed.prefix(96))
    }

    private func uniqueStrings(_ values: [String]) -> [String] {
        var seen = Set<String>()
        var result: [String] = []
        for value in values {
            let trimmed = value.trimmingCharacters(in: .whitespacesAndNewlines)
            guard !trimmed.isEmpty, seen.insert(trimmed).inserted else {
                continue
            }
            result.append(trimmed)
        }
        return result
    }

    private static func iso8601String(from date: Date) -> String {
        let formatter = ISO8601DateFormatter()
        formatter.formatOptions = [.withInternetDateTime, .withFractionalSeconds]
        return formatter.string(from: date)
    }
}
