import Foundation
import XCTest
@testable import GetWebexSpaceMacApp

final class ConnectorCheckpointDAOTests: XCTestCase {
    func testConnectorCheckpointDAORoundTripsIndependentKeysForOneTarget() throws {
        let harness = TestRuntimeHarness(label: "checkpoint-roundtrip")
        defer { harness.cleanup() }
        let dao = ConnectorCheckpointDAO(configuration: harness.configuration)

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
        let harness = TestRuntimeHarness(label: "checkpoint-namespace")
        defer { harness.cleanup() }
        let dao = ConnectorCheckpointDAO(configuration: harness.configuration)

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
        let harness = TestRuntimeHarness(label: "checkpoint-upsert")
        defer { harness.cleanup() }
        let dao = ConnectorCheckpointDAO(configuration: harness.configuration)

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
}
