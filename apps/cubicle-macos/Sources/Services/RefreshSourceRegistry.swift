import Foundation

/// One registered producer for a refresh scope.
protocol RefreshSource: AnyObject {
    var id: String { get }
    var scope: RefreshScope { get }

    func refresh(
        mode: RefreshExecutionMode,
        progress: ((RefreshExecutionProgress) async -> Void)?
    ) async throws -> RefreshSourceResult?
}

/// User-visible contribution from one registered refresh source.
struct RefreshSourceResult: Hashable {
    var sourceID: String
    var summary: String
    var completedAt: String?
}

/// Ordered registry for scope-specific refresh sources.
final class RefreshSourceRegistry {
    private var sourcesByScope: [RefreshScope: [RefreshSource]] = [:]

    func register(_ source: RefreshSource) {
        sourcesByScope[source.scope, default: []].append(source)
    }

    func sources(for scope: RefreshScope) -> [RefreshSource] {
        sourcesByScope[scope] ?? []
    }

    func refresh(
        scope: RefreshScope,
        mode: RefreshExecutionMode,
        progress: ((RefreshExecutionProgress) async -> Void)?
    ) async throws -> [RefreshSourceResult] {
        var results: [RefreshSourceResult] = []
        for source in sources(for: scope) {
            if let result = try await source.refresh(mode: mode, progress: progress) {
                results.append(result)
            }
        }
        return results
    }
}

/// Webex polling source backed by the existing adaptive sync engine.
final class WebexSyncRefreshSource: RefreshSource {
    let id = "webex"
    let scope = RefreshScope.webexSync

    private let webexIngestionService: NativeWebexIngesting

    init(webexIngestionService: NativeWebexIngesting) {
        self.webexIngestionService = webexIngestionService
    }

    func refresh(
        mode: RefreshExecutionMode,
        progress: ((RefreshExecutionProgress) async -> Void)?
    ) async throws -> RefreshSourceResult? {
        let webexSyncMode: WebexSyncMode = mode == .full ? .full : .incremental
        let trigger: WebexSyncTriggerReason = mode == .full ? .manual : .scheduled
        let outcome = try await webexIngestionService.syncTrackedTargets(
            messageLimitPerRoom: 150,
            mode: webexSyncMode,
            trigger: trigger
        ) { update in
            await progress?(
                RefreshExecutionProgress(
                    scope: self.scope,
                    message: update.summary
                )
            )
        }
        return RefreshSourceResult(
            sourceID: id,
            summary: outcome.summary,
            completedAt: outcome.completedAt
        )
    }
}

/// iMessage source that routes configured handles through signal connectors.
final class IMessageSignalRefreshSource: RefreshSource {
    let id = "imessage"
    let scope = RefreshScope.webexSync

    private let configStore: ConfigStore
    private let processingService: SignalConnectorProcessing

    init(
        configStore: ConfigStore,
        processingService: SignalConnectorProcessing
    ) {
        self.configStore = configStore
        self.processingService = processingService
    }

    func refresh(
        mode: RefreshExecutionMode,
        progress: ((RefreshExecutionProgress) async -> Void)?
    ) async throws -> RefreshSourceResult? {
        let targets = try iMessageSignalSyncTargets()
        guard !targets.isEmpty else {
            return nil
        }

        let preparingSummary = "iMessage signal sync: preparing \(targets.count) target(s)."
        await progress?(
            RefreshExecutionProgress(
                scope: scope,
                message: preparingSummary
            )
        )

        let signalMode: SignalSyncMode = mode == .full ? .full : .incremental
        let outcome = try await processingService.sync(
            configTargets: targets,
            mode: signalMode,
            limit: 150,
            since: nil
        )
        await progress?(
            RefreshExecutionProgress(
                scope: scope,
                message: outcome.summary
            )
        )
        return RefreshSourceResult(
            sourceID: id,
            summary: outcome.summary,
            completedAt: nil
        )
    }

    private func iMessageSignalSyncTargets() throws -> [ConfigTarget] {
        stableDedupedTargets(
            try configStore.importantSpaces()
                + configStore.importantPeople()
                + configStore.beliefTargets()
        ).filter { !$0.iMessageHandles.isEmpty }
    }

    private func stableDedupedTargets(_ targets: [ConfigTarget]) -> [ConfigTarget] {
        var seenIDs: Set<String> = []
        var deduped: [ConfigTarget] = []
        deduped.reserveCapacity(targets.count)
        for target in targets where seenIDs.insert(target.id).inserted {
            deduped.append(target)
        }
        return deduped
    }
}
