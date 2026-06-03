import Foundation

/// Source adapter that emits normalized signal batches for selected targets.
protocol SignalConnector {
    /// Static identity and capability metadata used by routing/UI layers.
    var descriptor: ConnectorDescriptor { get }

    /// Fetches connector-native data and converts it into signal objects/events.
    func sync(
        request: SignalSyncRequest,
        checkpoints: ConnectorCheckpointSet
    ) async throws -> SignalSyncBatch
}
