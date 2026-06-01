import Foundation

/// Webex API surface used by product-specific settings and target flows.
protocol WebexProductClienting {
    func rooms() async throws -> [WebexRoom]
    func memberships(roomID: String) async throws -> [WebexMembership]
}

extension WebexAPIClient: WebexProductClienting {}

/// Webex-only operations that should not leak into AppModel connector casting.
final class WebexProductService {
    private let client: WebexProductClienting

    /// Keeps Webex-specific API access behind a typed service boundary.
    init(client: WebexProductClienting) {
        self.client = client
    }

    /// Lists rooms visible to the current Webex account.
    func listRooms() async throws -> [WebexRoom] {
        try await client.rooms()
    }

    /// Lists room memberships for target setup and diagnostics.
    func listMemberships(roomID: String) async throws -> [WebexMembership] {
        try await client.memberships(roomID: roomID)
    }
}
