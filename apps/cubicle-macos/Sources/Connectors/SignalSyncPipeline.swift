import Foundation

/// Per-connector failure captured without aborting the whole sync run.
struct SignalSyncFailure: Hashable {
    var connectorID: ConnectorID
    var message: String
}

/// Connector batches, DAO write summaries, and recoverable connector failures.
struct SignalSyncPipelineResult: Hashable {
    var batches: [SignalSyncBatch]
    var writeSummaries: [ConnectorID: SignalWriteSummary]
    var failures: [SignalSyncFailure]
}

/// Routes targets to connectors and persists each connector's signal batch.
final class SignalSyncPipeline {
    private let connectors: [SignalConnector]
    private let writer: SignalKnowledgeWriting
    private let checkpointStore: SignalCheckpointStoring?

    /// Keeps connector execution injectable so tests and future schedulers share one path.
    init(
        connectors: [SignalConnector],
        writer: SignalKnowledgeWriting,
        checkpointStore: SignalCheckpointStoring? = nil
    ) {
        self.connectors = connectors
        self.writer = writer
        self.checkpointStore = checkpointStore
    }

    /// Runs connectors independently; failures stay attached to their source.
    func sync(
        request: SignalSyncRequest,
        checkpoints: ConnectorCheckpointSet = .empty
    ) async -> SignalSyncPipelineResult {
        let router = TargetRouter(connectorIDs: connectors.map(\.descriptor.id))
        let targetsByConnector = router.targetsByConnector(request.targets)
        var batches: [SignalSyncBatch] = []
        var writeSummaries: [ConnectorID: SignalWriteSummary] = [:]
        var failures: [SignalSyncFailure] = []

        for connector in connectors {
            let connectorID = connector.descriptor.id
            guard let targets = targetsByConnector[connectorID], !targets.isEmpty else {
                continue
            }

            var connectorRequest = request
            connectorRequest.targets = targets
            do {
                let loadedCheckpoints = try checkpointSet(
                    connectorID: connectorID,
                    targets: targets,
                    seeded: checkpoints.filtered(connectorID: connectorID)
                )
                let batch = try await connector.sync(
                    request: connectorRequest,
                    checkpoints: loadedCheckpoints
                )
                let summary = try writer.write(batch)
                batches.append(batch)
                writeSummaries[connectorID] = summary
                try checkpointStore?.save(batch.checkpoints)
            } catch {
                // One dead source should not block fresh evidence from another;
                // callers decide how to surface partial refresh state.
                failures.append(
                    SignalSyncFailure(
                        connectorID: connectorID,
                        message: error.localizedDescription
                    )
                )
            }
        }

        return SignalSyncPipelineResult(
            batches: batches,
            writeSummaries: writeSummaries,
            failures: failures
        )
    }

    private func checkpointSet(
        connectorID: ConnectorID,
        targets: [SignalTarget],
        seeded: ConnectorCheckpointSet
    ) throws -> ConnectorCheckpointSet {
        guard let checkpointStore else {
            return seeded
        }
        let targetIDs = targets.flatMap { $0.checkpointTargetIDs(for: connectorID) }
        return try checkpointStore
            .loadCheckpoints(connectorID: connectorID, targetIDs: targetIDs)
            .merging(seeded)
    }
}
