import Foundation

struct SignalSyncFailure: Hashable {
    var connectorID: ConnectorID
    var message: String
}

struct SignalSyncPipelineResult: Hashable {
    var batches: [SignalSyncBatch]
    var writeSummaries: [ConnectorID: SignalWriteSummary]
    var failures: [SignalSyncFailure]
}

final class SignalSyncPipeline {
    private let connectors: [SignalConnector]
    private let writer: SignalKnowledgeWriting

    init(connectors: [SignalConnector], writer: SignalKnowledgeWriting) {
        self.connectors = connectors
        self.writer = writer
    }

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
