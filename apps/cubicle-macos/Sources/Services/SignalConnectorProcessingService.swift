import Foundation

/// Result envelope for one generic connector-processing run.
struct SignalConnectorProcessingResult: Hashable {
    var targetCount: Int
    var pipelineResult: SignalSyncPipelineResult
    var summary: String
}

/// App-facing connector processor used by refresh orchestration.
protocol SignalConnectorProcessing {
    func sync(
        configTargets: [ConfigTarget],
        mode: SignalSyncMode,
        limit: Int,
        since: Date?
    ) async throws -> SignalConnectorProcessingResult
}

/// App-level service that builds connectors, routes targets, and writes signal batches.
final class SignalConnectorProcessingService: SignalConnectorProcessing {
    private let factory: SignalConnectorFactory
    private let writer: SignalKnowledgeWriting
    private let checkpointStore: SignalCheckpointStoring?
    private let connectorIDs: [ConnectorID]
    private let now: () -> Date

    /// Keeps construction injectable while the coordinator owns production wiring.
    init(
        factory: SignalConnectorFactory,
        writer: SignalKnowledgeWriting,
        checkpointStore: SignalCheckpointStoring? = nil,
        connectorIDs: [ConnectorID] = [.webex, .iMessage],
        now: @escaping () -> Date = Date.init
    ) {
        self.factory = factory
        self.writer = writer
        self.checkpointStore = checkpointStore
        self.connectorIDs = connectorIDs
        self.now = now
    }

    /// Runs configured focus targets through the generic signal pipeline.
    func sync(
        configTargets: [ConfigTarget],
        mode: SignalSyncMode,
        limit: Int,
        since: Date? = nil
    ) async throws -> SignalConnectorProcessingResult {
        let targets = configTargets.map(SignalTarget.init)
        let connectors = try factory.makeSignalConnectors(ids: connectorIDs)
        let pipeline = SignalSyncPipeline(
            connectors: connectors,
            writer: writer,
            checkpointStore: checkpointStore
        )
        let result = await pipeline.sync(
            request: SignalSyncRequest(
                mode: mode,
                targets: targets,
                startedAt: now(),
                since: since,
                limit: limit
            )
        )
        return SignalConnectorProcessingResult(
            targetCount: targets.count,
            pipelineResult: result,
            summary: "Signal sync: targets=\(targets.count), batches=\(result.batches.count), failures=\(result.failures.count)."
        )
    }
}
