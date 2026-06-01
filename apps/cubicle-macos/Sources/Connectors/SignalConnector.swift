import Foundation

protocol SignalConnector {
    var descriptor: ConnectorDescriptor { get }

    func sync(
        request: SignalSyncRequest,
        checkpoint: ConnectorCheckpoint?
    ) async throws -> SignalSyncBatch
}
