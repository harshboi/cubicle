import Foundation
import XCTest
@testable import GetWebexSpaceMacApp

final class ConnectorCheckpointDAOTests: XCTestCase {
    func testConnectorCheckpointDAORoundTripsIndependentKeysForOneTarget() throws {
        let runtimeRoot = temporaryRuntimeRoot(label: "checkpoint-roundtrip")
        defer { try? FileManager.default.removeItem(at: runtimeRoot) }
        let dao = ConnectorCheckpointDAO(configuration: testConfiguration(runtimeRoot: runtimeRoot))

        try dao.upsert(
            ConnectorCheckpointRecord(
                connectorID: "webex",
                targetID: "room-1",
                key: "last-message",
                valueJSON: #"{"messageID":"m1"}"#,
                metadataJSON: #"{"mode":"recent"}"#,
                updatedAt: "2026-06-01T00:00:00.000Z"
            )
        )
        try dao.upsert(
            ConnectorCheckpointRecord(
                connectorID: "webex",
                targetID: "room-1",
                key: "backoff",
                valueJSON: #"{"nextAllowed":"2026-06-01T00:05:00.000Z"}"#,
                metadataJSON: "{}",
                updatedAt: "2026-06-01T00:01:00.000Z"
            )
        )

        let lastMessage = try XCTUnwrap(
            dao.load(connectorID: "webex", targetID: "room-1", key: "last-message")
        )
        let allForTarget = try dao.loadAll(connectorID: "webex", targetID: "room-1")

        XCTAssertEqual(lastMessage.valueJSON, #"{"messageID":"m1"}"#)
        XCTAssertEqual(lastMessage.metadataJSON, #"{"mode":"recent"}"#)
        XCTAssertEqual(allForTarget.map(\.key).sorted(), ["backoff", "last-message"])
    }

    func testConnectorCheckpointDAOIsolatesConnectorNamespaces() throws {
        let runtimeRoot = temporaryRuntimeRoot(label: "checkpoint-namespace")
        defer { try? FileManager.default.removeItem(at: runtimeRoot) }
        let dao = ConnectorCheckpointDAO(configuration: testConfiguration(runtimeRoot: runtimeRoot))

        try dao.upsert(
            ConnectorCheckpointRecord(
                connectorID: "webex",
                targetID: "conversation-1",
                key: "cursor",
                valueJSON: #"{"cursor":"webex-cursor"}"#,
                metadataJSON: "{}",
                updatedAt: "2026-06-01T00:00:00.000Z"
            )
        )
        try dao.upsert(
            ConnectorCheckpointRecord(
                connectorID: "slack",
                targetID: "conversation-1",
                key: "cursor",
                valueJSON: #"{"cursor":"slack-cursor"}"#,
                metadataJSON: "{}",
                updatedAt: "2026-06-01T00:00:01.000Z"
            )
        )

        XCTAssertEqual(
            try dao.load(connectorID: "webex", targetID: "conversation-1", key: "cursor")?.valueJSON,
            #"{"cursor":"webex-cursor"}"#
        )
        XCTAssertEqual(
            try dao.load(connectorID: "slack", targetID: "conversation-1", key: "cursor")?.valueJSON,
            #"{"cursor":"slack-cursor"}"#
        )
    }

    func testConnectorCheckpointDAOUpsertReplacesSingleCheckpoint() throws {
        let runtimeRoot = temporaryRuntimeRoot(label: "checkpoint-upsert")
        defer { try? FileManager.default.removeItem(at: runtimeRoot) }
        let dao = ConnectorCheckpointDAO(configuration: testConfiguration(runtimeRoot: runtimeRoot))

        try dao.upsert(
            ConnectorCheckpointRecord(
                connectorID: "jira",
                targetID: "project-1",
                key: "delta",
                valueJSON: #"{"page":1}"#,
                metadataJSON: "{}",
                updatedAt: "2026-06-01T00:00:00.000Z"
            )
        )
        try dao.upsert(
            ConnectorCheckpointRecord(
                connectorID: "jira",
                targetID: "project-1",
                key: "delta",
                valueJSON: #"{"page":2}"#,
                metadataJSON: #"{"reason":"scheduled"}"#,
                updatedAt: "2026-06-01T00:02:00.000Z"
            )
        )

        let checkpoints = try dao.loadAll(connectorID: "jira", targetID: "project-1")

        XCTAssertEqual(checkpoints.count, 1)
        XCTAssertEqual(checkpoints.first?.valueJSON, #"{"page":2}"#)
        XCTAssertEqual(checkpoints.first?.metadataJSON, #"{"reason":"scheduled"}"#)
        XCTAssertEqual(checkpoints.first?.updatedAt, "2026-06-01T00:02:00.000Z")
    }

    func testSignalCheckpointStoreRoundTripsTypedCheckpoint() throws {
        let runtimeRoot = temporaryRuntimeRoot(label: "checkpoint-store-roundtrip")
        defer { try? FileManager.default.removeItem(at: runtimeRoot) }
        let dao = ConnectorCheckpointDAO(configuration: testConfiguration(runtimeRoot: runtimeRoot))
        let store = SignalCheckpointStore(dao: dao)
        let checkpoint = ConnectorCheckpoint(
            connectorID: .webex,
            accountID: "workspace",
            targetID: "roomID:room-1",
            key: "cursor",
            updatedAt: Date(timeIntervalSince1970: 1_715_000_000),
            payload: ["messageID": "webex-message-1"],
            metadata: ["mode": "incremental"]
        )

        try store.save(checkpoint)
        let loaded = try store
            .loadCheckpoints(connectorID: .webex, targetIDs: ["roomID:room-1"])
            .checkpoint(connectorID: .webex, targetID: "roomID:room-1", key: "cursor")

        XCTAssertEqual(loaded?.connectorID, .webex)
        XCTAssertEqual(loaded?.accountID, "workspace")
        XCTAssertEqual(loaded?.targetID, "roomID:room-1")
        XCTAssertEqual(loaded?.key, "cursor")
        XCTAssertEqual(loaded?.payload, ["messageID": "webex-message-1"])
        XCTAssertEqual(loaded?.metadata, ["mode": "incremental"])
        XCTAssertEqual(loaded?.updatedAt, checkpoint.updatedAt)
    }
}

private func testConfiguration(runtimeRoot: URL) -> RuntimeConfiguration {
    RuntimeConfiguration(
        runtimeRoot: runtimeRoot,
        codexExecutable: "codex",
        webexBaseURL: URL(string: "https://webexapis.com/v1")!,
        webexPageSize: 100,
        webexRetryCount: 0,
        webexTimeoutSeconds: 1,
        webexOAuthTokenPathOverride: nil,
        webexOAuthRefreshSkewSeconds: 300,
        webexOAuthRefreshTokenSkewSeconds: 86_400
    )
}

private func temporaryRuntimeRoot(label: String) -> URL {
    FileManager.default.temporaryDirectory.appendingPathComponent(
        "Cubicle-\(label)-\(UUID().uuidString)",
        isDirectory: true
    )
}
