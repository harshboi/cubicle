import Foundation
import XCTest
@testable import GetWebexSpaceMacApp

final class SignalConnectorTests: XCTestCase {
    func testConfigTargetMapsToSignalTargetSelectors() {
        let configTarget = ConfigTarget(
            kind: .person,
            label: "Alex Chen",
            roomID: " room-123 ",
            roomType: "direct",
            email: "ALEX@EXAMPLE.COM",
            iMessageHandles: ["(408) 555-0100", "alex@example.com"]
        )

        let target = SignalTarget(configTarget: configTarget)

        XCTAssertEqual(target.id, configTarget.id)
        XCTAssertEqual(target.label, "Alex Chen")
        XCTAssertEqual(target.entityKind, .person)
        XCTAssertTrue(target.selectors.contains(ConnectorSelector(connectorID: .webex, kind: .roomID, value: "room-123")))
        XCTAssertTrue(target.selectors.contains(ConnectorSelector(connectorID: .webex, kind: .email, value: "alex@example.com")))
        XCTAssertTrue(target.selectors.contains(ConnectorSelector(connectorID: .iMessage, kind: .handle, value: "+14085550100")))
        XCTAssertTrue(target.selectors.contains(ConnectorSelector(connectorID: .iMessage, kind: .handle, value: "alex@example.com")))
    }

    func testTargetRouterGroupsTargetsByConnectorSelectors() {
        let webexTarget = SignalTarget(
            id: "space:1",
            label: "Space 1",
            entityKind: .space,
            selectors: [ConnectorSelector(connectorID: .webex, kind: .roomID, value: "room-1")]
        )
        let iMessageTarget = SignalTarget(
            id: "person:1",
            label: "Person 1",
            entityKind: .person,
            selectors: [ConnectorSelector(connectorID: .iMessage, kind: .handle, value: "+14085550100")]
        )
        let sharedTarget = SignalTarget(
            id: "person:2",
            label: "Person 2",
            entityKind: .person,
            selectors: [
                ConnectorSelector(connectorID: .webex, kind: .email, value: "person2@example.com"),
                ConnectorSelector(connectorID: .iMessage, kind: .handle, value: "+14085550101")
            ]
        )

        let routed = TargetRouter(connectorIDs: [.webex, .iMessage])
            .targetsByConnector([webexTarget, iMessageTarget, sharedTarget])

        XCTAssertEqual(routed[.webex]?.map(\.id), ["space:1", "person:2"])
        XCTAssertEqual(routed[.iMessage]?.map(\.id), ["person:1", "person:2"])
    }

    func testIMessageSignalConnectorEmitsMessageEvents() async throws {
        let since = Date(timeIntervalSince1970: 1_715_000_000)
        let messageDate = since.addingTimeInterval(120)
        let iMessageService = StubIMessageSignalIngestionService(messages: [
            IMessageTimelineMessage(
                id: "imessage-row-1",
                threadID: "chat-1",
                threadTitle: "iMessage - Alex Chen",
                handle: "+14085550100",
                sender: "Alex Chen",
                body: "Can you review the Jira plan?",
                createdAt: testISO8601.string(from: messageDate),
                sortDate: messageDate,
                isFromMe: false
            )
        ])
        let connector = IMessageSignalConnector(ingestionService: iMessageService)
        let target = SignalTarget(
            id: "person:alex",
            label: "Alex Chen",
            entityKind: .person,
            selectors: [ConnectorSelector(connectorID: .iMessage, kind: .handle, value: "+14085550100")]
        )

        let batch = try await connector.sync(
            request: SignalSyncRequest(mode: .incremental, targets: [target], startedAt: since, since: since, limit: 10),
            checkpoint: nil
        )

        XCTAssertEqual(iMessageService.receivedHandles, ["+14085550100"])
        XCTAssertEqual(batch.connectorID, .iMessage)
        XCTAssertEqual(batch.accountID, "local")
        XCTAssertEqual(batch.availability, .available)
        XCTAssertEqual(batch.events.count, 1)
        guard case .message(let payload) = batch.events[0].payload else {
            return XCTFail("Expected a message payload.")
        }
        XCTAssertEqual(payload.threadSourceID.externalID, "chat-1")
        XCTAssertEqual(payload.threadTitle, "iMessage - Alex Chen")
        XCTAssertEqual(payload.senderDisplayName, "Alex Chen")
        XCTAssertEqual(payload.body, "Can you review the Jira plan?")
        XCTAssertFalse(payload.isFromCurrentUser)
    }

    func testWebexSignalConnectorEmitsRoomMessageEvents() async throws {
        let createdAt = Date(timeIntervalSince1970: 1_715_000_120)
        let client = StubSignalWebexClient()
        client.recentMessagesByRoomID["room-1"] = [
            makeWebexSignalMessage(
                id: "webex-message-1",
                roomID: "room-1",
                personID: "person-1",
                personEmail: "alex@example.com",
                text: "The rollout is blocked on approval.",
                createdAt: createdAt
            )
        ]
        let connector = WebexSignalConnector(webexClient: client, accountID: "workspace")
        let target = SignalTarget(
            id: "space:room-1",
            label: "Launch Room",
            entityKind: .space,
            selectors: [ConnectorSelector(connectorID: .webex, kind: .roomID, value: "room-1")]
        )

        let batch = try await connector.sync(
            request: SignalSyncRequest(mode: .incremental, targets: [target], startedAt: createdAt, since: nil, limit: 25),
            checkpoint: nil
        )

        XCTAssertEqual(client.recentFetches.count, 1)
        XCTAssertEqual(client.recentFetches.first?.0, "room-1")
        XCTAssertEqual(client.recentFetches.first?.1, 25)
        XCTAssertEqual(batch.connectorID, .webex)
        XCTAssertEqual(batch.accountID, "workspace")
        XCTAssertEqual(batch.availability, .available)
        XCTAssertEqual(batch.events.count, 1)
        guard case .message(let payload) = batch.events[0].payload else {
            return XCTFail("Expected a message payload.")
        }
        XCTAssertEqual(payload.threadSourceID.externalID, "room-1")
        XCTAssertEqual(payload.threadTitle, "Launch Room")
        XCTAssertEqual(payload.senderID?.rawValue, "webex:workspace:person:person-1")
        XCTAssertEqual(payload.senderEmail, "alex@example.com")
        XCTAssertEqual(payload.body, "The rollout is blocked on approval.")
    }

    func testSignalSyncPipelineRoutesTargetsAndWritesBatches() async throws {
        let webexBatch = SignalSyncBatch.empty(connectorID: .webex, accountID: "workspace")
        let iMessageBatch = SignalSyncBatch.empty(connectorID: .iMessage, accountID: "local")
        let webexConnector = StubSignalConnector(connectorID: .webex, batch: webexBatch)
        let iMessageConnector = StubSignalConnector(connectorID: .iMessage, batch: iMessageBatch)
        let writer = RecordingSignalKnowledgeWriter()
        let webexTarget = SignalTarget(
            id: "space:1",
            label: "Space 1",
            entityKind: .space,
            selectors: [ConnectorSelector(connectorID: .webex, kind: .roomID, value: "room-1")]
        )
        let sharedTarget = SignalTarget(
            id: "person:1",
            label: "Person 1",
            entityKind: .person,
            selectors: [
                ConnectorSelector(connectorID: .webex, kind: .email, value: "person@example.com"),
                ConnectorSelector(connectorID: .iMessage, kind: .handle, value: "+14085550100")
            ]
        )

        let pipeline = SignalSyncPipeline(
            connectors: [webexConnector, iMessageConnector],
            writer: writer
        )
        let result = await pipeline.sync(
            request: SignalSyncRequest(mode: .incremental, targets: [webexTarget, sharedTarget], limit: 50)
        )

        XCTAssertEqual(webexConnector.receivedTargetIDs, ["space:1", "person:1"])
        XCTAssertEqual(iMessageConnector.receivedTargetIDs, ["person:1"])
        XCTAssertEqual(writer.writtenConnectorIDs, [.webex, .iMessage])
        XCTAssertEqual(result.batches.map(\.connectorID), [.webex, .iMessage])
        XCTAssertTrue(result.failures.isEmpty)
    }

    func testSignalKnowledgeWriterPersistsMessageEventsIdempotently() throws {
        let runtimeRoot = FileManager.default.temporaryDirectory
            .appendingPathComponent("CubicleSignalWriterTests-\(UUID().uuidString)", isDirectory: true)
        defer { try? FileManager.default.removeItem(at: runtimeRoot) }
        let knowledgeStore = KnowledgeStore(configuration: testSignalRuntimeConfiguration(runtimeRoot: runtimeRoot))
        let writer = SignalKnowledgeWriter(knowledgeStore: knowledgeStore)
        let occurredAt = Date(timeIntervalSince1970: 1_715_000_240)
        let batch = makeSignalMessageBatch(occurredAt: occurredAt)

        let firstSummary = try writer.write(batch)
        let secondSummary = try writer.write(batch)

        XCTAssertEqual(firstSummary.messageEventsProcessed, 1)
        XCTAssertEqual(secondSummary.messageEventsProcessed, 1)
        let messages = try knowledgeStore.loadMessages(roomID: "room-1")
        XCTAssertEqual(messages.count, 1)
        XCTAssertEqual(messages.first?.id, "webex:workspace:message:webex-message-1")
        XCTAssertEqual(messages.first?.body, "The rollout is blocked on approval.")
        XCTAssertTrue(try knowledgeStore.messageExists(messageID: "webex:workspace:message:webex-message-1"))
        let evidence = try knowledgeStore.loadBeliefEvidence(scope: .space, entityKey: "room-1")
        XCTAssertEqual(evidence.count, 1)
        XCTAssertEqual(evidence.first?.source, "webex_message")
    }

    func testSignalKnowledgeWriterWritesMappedRowsAsOneConnectorBatch() throws {
        let knowledgeStore = RecordingSignalKnowledgeStore()
        let writer = SignalKnowledgeWriter(
            knowledgeStore: knowledgeStore,
            now: { Date(timeIntervalSince1970: 1_715_000_360) }
        )
        let occurredAt = Date(timeIntervalSince1970: 1_715_000_240)

        let summary = try writer.write(makeSignalMessageBatch(occurredAt: occurredAt))

        XCTAssertEqual(summary.messageEventsProcessed, 1)
        XCTAssertEqual(summary.evidenceRecordsWritten, 1)
        XCTAssertEqual(knowledgeStore.bootstrapCallCount, 1)
        XCTAssertEqual(knowledgeStore.batchWrites.count, 1)
        let batchWrite = try XCTUnwrap(knowledgeStore.batchWrites.first)
        XCTAssertEqual(batchWrite.rooms.map(\.id), ["room-1", "room-1"])
        XCTAssertEqual(batchWrite.people.map(\.id), [
            "webex:workspace:person:person-1",
            "webex:workspace:person:person-1"
        ])
        XCTAssertEqual(batchWrite.messages.map(\.id), ["webex:workspace:message:webex-message-1"])
        XCTAssertEqual(batchWrite.evidence.map(\.source), ["webex_message"])
        XCTAssertEqual(batchWrite.evidence.map(\.sourceID), ["webex:workspace:message:webex-message-1"])
    }
}

private final class StubIMessageSignalIngestionService: NativeIMessageIngesting {
    private let messages: [IMessageTimelineMessage]
    private(set) var receivedHandles: [String] = []

    init(messages: [IMessageTimelineMessage]) {
        self.messages = messages
    }

    func loadMessages(
        matching handles: [String],
        displayName: String,
        since: Date,
        limit: Int
    ) throws -> [IMessageTimelineMessage] {
        receivedHandles = handles
        return Array(messages.filter { $0.sortDate >= since }.prefix(limit))
    }
}

private final class StubSignalWebexClient: WebexClienting {
    var recentMessagesByRoomID: [String: [WebexMessage]] = [:]
    var directMessagesByEmail: [String: [WebexMessage]] = [:]
    private(set) var recentFetches: [(String, Int)] = []

    func fetchLatestMessage(roomID: String) async throws -> WebexMessage? {
        recentMessagesByRoomID[roomID]?.first
    }

    func fetchRecentMessages(roomID: String, max: Int) async throws -> [WebexMessage] {
        recentFetches.append((roomID, max))
        return recentMessagesByRoomID[roomID] ?? []
    }

    func fetchMessage(messageID: String) async throws -> WebexMessage {
        throw WebexAPIError.unexpectedResponse(messageID)
    }

    func fetchDirectMessages(personEmail: String?, personID: String?, max: Int) async throws -> [WebexMessage] {
        directMessagesByEmail[personEmail ?? ""] ?? []
    }
}

private final class StubSignalConnector: SignalConnector {
    let descriptor: ConnectorDescriptor
    let batch: SignalSyncBatch
    private(set) var receivedTargetIDs: [String] = []

    init(connectorID: ConnectorID, batch: SignalSyncBatch) {
        self.descriptor = ConnectorDescriptor(id: connectorID, displayName: connectorID.rawValue, capabilities: [.messages])
        self.batch = batch
    }

    func sync(
        request: SignalSyncRequest,
        checkpoint: ConnectorCheckpoint?
    ) async throws -> SignalSyncBatch {
        receivedTargetIDs = request.targets.map(\.id)
        return batch
    }
}

private final class RecordingSignalKnowledgeWriter: SignalKnowledgeWriting {
    private(set) var writtenConnectorIDs: [ConnectorID] = []

    func write(_ batch: SignalSyncBatch) throws -> SignalWriteSummary {
        writtenConnectorIDs.append(batch.connectorID)
        return SignalWriteSummary(messageEventsProcessed: batch.events.count, evidenceRecordsWritten: 0)
    }
}

private struct RecordedConnectorBatchWrite {
    var rooms: [RoomRecord]
    var people: [PersonRecord]
    var messages: [MessageRecord]
    var evidence: [BeliefEvidenceRecord]
}

private final class RecordingSignalKnowledgeStore: SignalKnowledgeWritableStore {
    private(set) var bootstrapCallCount = 0
    private(set) var batchWrites: [RecordedConnectorBatchWrite] = []

    func bootstrap() throws {
        bootstrapCallCount += 1
    }

    func writeConnectorMessageBatch(
        rooms: [RoomRecord],
        people: [PersonRecord],
        messages: [MessageRecord],
        evidence: [BeliefEvidenceRecord]
    ) throws {
        batchWrites.append(
            RecordedConnectorBatchWrite(
                rooms: rooms,
                people: people,
                messages: messages,
                evidence: evidence
            )
        )
    }
}

private func makeWebexSignalMessage(
    id: String,
    roomID: String,
    personID: String,
    personEmail: String,
    text: String,
    createdAt: Date
) -> WebexMessage {
    let payload: [String: Any] = [
        "id": id,
        "roomId": roomID,
        "personId": personID,
        "personEmail": personEmail,
        "text": text,
        "created": testISO8601.string(from: createdAt)
    ]
    let data = try! JSONSerialization.data(withJSONObject: payload, options: [])
    return try! JSONDecoder().decode(WebexMessage.self, from: data)
}

private func makeSignalMessageBatch(occurredAt: Date) -> SignalSyncBatch {
    let roomSourceID = SourceObjectID(connectorID: .webex, accountID: "workspace", kind: "room", externalID: "room-1")
    let personSourceID = SourceObjectID(connectorID: .webex, accountID: "workspace", kind: "person", externalID: "person-1")
    let eventSourceID = SourceEventID(connectorID: .webex, accountID: "workspace", kind: "message", externalID: "webex-message-1")
    let visibility = SignalVisibility.authenticatedUser(connectorID: .webex, accountID: "workspace")
    let payload = MessageEventPayload(
        threadID: roomSourceID.globalID,
        threadSourceID: roomSourceID,
        threadTitle: "Launch Room",
        senderID: personSourceID.globalID,
        senderDisplayName: "alex@example.com",
        senderEmail: "alex@example.com",
        body: "The rollout is blocked on approval.",
        isFromCurrentUser: false
    )
    return SignalSyncBatch(
        connectorID: .webex,
        accountID: "workspace",
        objects: [
            SignalObject(
                id: roomSourceID.globalID,
                sourceID: roomSourceID,
                kind: .space,
                title: "Launch Room",
                url: nil,
                createdAt: nil,
                updatedAt: occurredAt,
                visibility: visibility,
                properties: ["webex.roomID": .string("room-1")]
            ),
            SignalObject(
                id: personSourceID.globalID,
                sourceID: personSourceID,
                kind: .person,
                title: "alex@example.com",
                url: nil,
                createdAt: nil,
                updatedAt: occurredAt,
                visibility: visibility,
                properties: ["webex.email": .string("alex@example.com")]
            )
        ],
        events: [
            SignalEvent(
                id: eventSourceID.globalID,
                sourceID: eventSourceID,
                kind: .message,
                actor: SignalActor(id: personSourceID.globalID, displayName: "alex@example.com", email: "alex@example.com"),
                occurredAt: occurredAt,
                objectIDs: [roomSourceID.globalID, personSourceID.globalID],
                visibility: visibility,
                payload: .message(payload)
            )
        ],
        relations: [],
        content: [],
        checkpoint: nil,
        warnings: [],
        availability: .available
    )
}

private func testSignalRuntimeConfiguration(runtimeRoot: URL) -> RuntimeConfiguration {
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

private let testISO8601: ISO8601DateFormatter = {
    let formatter = ISO8601DateFormatter()
    formatter.formatOptions = [.withInternetDateTime, .withFractionalSeconds]
    return formatter
}()
