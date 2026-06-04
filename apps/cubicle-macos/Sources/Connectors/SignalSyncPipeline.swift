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

    /// Keeps connector execution injectable so tests and future schedulers share one path.
    init(connectors: [SignalConnector], writer: SignalKnowledgeWriting) {
        self.connectors = connectors
        self.writer = writer
    }

    /// Runs connectors independently; failures stay attached to their source.
    func sync(
        request: SignalSyncRequest,
        checkpoints: [ConnectorID: ConnectorCheckpoint] = [:]
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
                let batch = try await connector.sync(
                    request: connectorRequest,
                    checkpoint: checkpoints[connectorID]
                )
                let summary = try writer.write(batch)
                batches.append(batch)
                writeSummaries[connectorID] = summary
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
}
