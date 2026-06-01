import Foundation
import SQLite3
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

    func testIMessageSignalConnectorHandlesNilSinceWithNanosecondChatDatabase() async throws {
        let runtimeRoot = FileManager.default.temporaryDirectory
            .appendingPathComponent("CubicleSignalIMessageNanoseconds-\(UUID().uuidString)", isDirectory: true)
        defer { try? FileManager.default.removeItem(at: runtimeRoot) }
        try FileManager.default.createDirectory(at: runtimeRoot, withIntermediateDirectories: true)
        let databaseURL = runtimeRoot.appendingPathComponent("chat.db")
        let messageDate = try XCTUnwrap(testISO8601.date(from: "2026-06-02T19:40:00.000Z"))
        let rawDate = appleMessageRawDate(messageDate, scale: 1_000_000_000)
        try executeSignalSQLite(
            """
            CREATE TABLE handle (ROWID INTEGER PRIMARY KEY, id TEXT);
            CREATE TABLE chat (ROWID INTEGER PRIMARY KEY, guid TEXT, chat_identifier TEXT, display_name TEXT);
            CREATE TABLE chat_handle_join (chat_id INTEGER, handle_id INTEGER);
            CREATE TABLE chat_message_join (chat_id INTEGER, message_id INTEGER);
            CREATE TABLE message (ROWID INTEGER PRIMARY KEY, guid TEXT, text TEXT, date INTEGER, is_from_me INTEGER, handle_id INTEGER);
            INSERT INTO handle (ROWID, id) VALUES (1, '+14085550100');
            INSERT INTO chat (ROWID, guid, chat_identifier, display_name) VALUES (1, 'chat-guid', '+14085550100', '');
            INSERT INTO chat_handle_join (chat_id, handle_id) VALUES (1, 1);
            INSERT INTO message (ROWID, guid, text, date, is_from_me, handle_id) VALUES (1, 'nanosecond-guid', 'Crash repro message.', \(rawDate), 0, 1);
            INSERT INTO chat_message_join (chat_id, message_id) VALUES (1, 1);
            """,
            databaseURL: databaseURL
        )
        let connector = IMessageSignalConnector(
            ingestionService: NativeIMessageIngestionService(chatDatabaseURL: databaseURL)
        )
        let target = SignalTarget(
            id: "person:alex",
            label: "Alex Chen",
            entityKind: .person,
            selectors: [ConnectorSelector(connectorID: .iMessage, kind: .handle, value: "+14085550100")]
        )

        let batch = try await connector.sync(
            request: SignalSyncRequest(mode: .incremental, targets: [target], startedAt: messageDate, since: nil, limit: 10),
            checkpoint: nil
        )

        XCTAssertEqual(batch.availability, .available)
        XCTAssertEqual(batch.events.count, 1)
        guard case .message(let payload) = batch.events[0].payload else {
            return XCTFail("Expected a message payload.")
        }
        XCTAssertEqual(payload.body, "Crash repro message.")
        XCTAssertEqual(payload.threadTitle, "iMessage - Alex Chen")
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

    func testSignalConnectorFactoryBuildsWebexAndIMessageConnectors() throws {
        let webexClient = StubFactoryWebexClient()
        let iMessageService = StubIMessageSignalIngestionService(messages: [])
        let factory = SignalConnectorFactory(
            webexClient: webexClient,
            iMessageIngestionService: iMessageService,
            webexAccountID: "workspace",
            iMessageAccountID: "local"
        )

        let connectors = try factory.makeSignalConnectors(ids: [.webex, .iMessage])

        XCTAssertEqual(connectors.map(\.descriptor.id), [.webex, .iMessage])
        XCTAssertEqual(connectors.map(\.descriptor.displayName), ["Webex", "iMessage"])
    }

    func testSignalConnectorProcessingServiceBuildsConnectorsAndWritesRoutedBatches() async throws {
        let since = Date(timeIntervalSince1970: 1_715_000_000)
        let webexClient = StubFactoryWebexClient()
        webexClient.recentMessagesByRoomID["room-1"] = [
            makeWebexSignalMessage(
                id: "webex-message-1",
                roomID: "room-1",
                personID: "person-1",
                personEmail: "alex@example.com",
                text: "The rollout is blocked on approval.",
                createdAt: since.addingTimeInterval(60)
            )
        ]
        let iMessageService = StubIMessageSignalIngestionService(messages: [
            IMessageTimelineMessage(
                id: "imessage-row-1",
                threadID: "chat-1",
                threadTitle: "Alex Chen",
                handle: "+14085550100",
                sender: "Alex Chen",
                body: "Can you review the Jira plan?",
                createdAt: testISO8601.string(from: since.addingTimeInterval(120)),
                sortDate: since.addingTimeInterval(120),
                isFromMe: false
            )
        ])
        let factory = SignalConnectorFactory(
            webexClient: webexClient,
            iMessageIngestionService: iMessageService,
            webexAccountID: "workspace",
            iMessageAccountID: "local"
        )
        let writer = RecordingSignalKnowledgeWriter()
        let service = SignalConnectorProcessingService(
            factory: factory,
            writer: writer,
            connectorIDs: [.webex, .iMessage],
            now: { since }
        )
        let spaceTarget = ConfigTarget(
            kind: .space,
            label: "Launch Room",
            roomID: "room-1",
            roomType: "group",
            email: "",
            iMessageHandles: []
        )
        let personTarget = ConfigTarget(
            kind: .person,
            label: "Alex Chen",
            roomID: "",
            roomType: "direct",
            email: "alex@example.com",
            iMessageHandles: ["(408) 555-0100"]
        )

        let result = try await service.sync(
            configTargets: [spaceTarget, personTarget],
            mode: .incremental,
            limit: 10,
            since: since
        )

        XCTAssertEqual(result.targetCount, 2)
        XCTAssertEqual(result.pipelineResult.batches.map(\.connectorID), [.webex, .iMessage])
        XCTAssertEqual(writer.writtenConnectorIDs, [.webex, .iMessage])
        XCTAssertEqual(webexClient.recentFetches.map(\.0), ["room-1"])
        XCTAssertEqual(iMessageService.receivedHandles, ["+14085550100"])
        XCTAssertEqual(result.summary, "Signal sync: targets=2, batches=2, failures=0.")
    }

    func testWebexProductServiceListsRoomsAndMembershipsWithoutAppModelCasting() async throws {
        let webexClient = StubFactoryWebexClient()
        webexClient.rooms = [
            makeWebexRoom(id: "room-1", title: "Launch Room", type: "group", lastActivity: "2026-06-01T00:00:00.000Z")
        ]
        webexClient.membershipsByRoomID["room-1"] = [
            makeWebexMembership(id: "membership-1", roomID: "room-1", personEmail: "alex@example.com")
        ]
        let service = WebexProductService(client: webexClient)

        let rooms = try await service.listRooms()
        let memberships = try await service.listMemberships(roomID: "room-1")

        XCTAssertEqual(rooms.map(\.id), ["room-1"])
        XCTAssertEqual(rooms.map(\.title), ["Launch Room"])
        XCTAssertEqual(memberships.map(\.personEmail), ["alex@example.com"])
    }

    func testIMessageProductServiceNormalizesHandlesAndPreviewsMessages() throws {
        let since = Date(timeIntervalSince1970: 1_715_000_000)
        let message = IMessageTimelineMessage(
            id: "imessage-row-1",
            threadID: "chat-1",
            threadTitle: "Alex Chen",
            handle: "+14085550100",
            sender: "Alex Chen",
            body: "Can you review the Jira plan?",
            createdAt: testISO8601.string(from: since.addingTimeInterval(120)),
            sortDate: since.addingTimeInterval(120),
            isFromMe: false
        )
        let iMessageService = StubIMessageSignalIngestionService(messages: [message])
        let service = IMessageProductService(ingestionService: iMessageService)

        let normalized = service.normalizedHandle("(408) 555-0100")
        let preview = try service.previewMessages(
            matching: ["(408) 555-0100"],
            displayName: "Alex Chen",
            since: since,
            limit: 10
        )

        XCTAssertEqual(normalized, "+14085550100")
        XCTAssertEqual(iMessageService.receivedHandles, ["+14085550100"])
        XCTAssertEqual(preview.map(\.id), ["imessage-row-1"])
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

    func testSignalKnowledgeWriterPersistsMixedMessageBatchIntoKnowledgeStore() throws {
        let runtimeRoot = FileManager.default.temporaryDirectory
            .appendingPathComponent("CubicleSignalWriterMixedBatch-\(UUID().uuidString)", isDirectory: true)
        defer { try? FileManager.default.removeItem(at: runtimeRoot) }
        let knowledgeStore = KnowledgeStore(configuration: testSignalRuntimeConfiguration(runtimeRoot: runtimeRoot))
        let writer = SignalKnowledgeWriter(knowledgeStore: knowledgeStore)
        let occurredAt = Date(timeIntervalSince1970: 1_715_000_240)

        let summary = try writer.write(makeMixedSignalMessageBatch(occurredAt: occurredAt))

        XCTAssertEqual(summary.messageEventsProcessed, 3)
        XCTAssertEqual(summary.evidenceRecordsWritten, 2)

        let room = try XCTUnwrap(knowledgeStore.loadRoom(roomID: "room-1"))
        XCTAssertEqual(room.title, "Launch Room")

        let messages = try knowledgeStore.loadMessages(roomID: "room-1", limit: 10)
        let messagesByID = Dictionary(uniqueKeysWithValues: messages.map { ($0.id, $0) })
        XCTAssertEqual(Set(messagesByID.keys), [
            "webex:workspace:message:webex-message-1",
            "webex:workspace:message:webex-message-2",
            "webex:workspace:message:webex-message-empty"
        ])
        XCTAssertEqual(messagesByID["webex:workspace:message:webex-message-2"]?.body, "Can you review Jira?")
        XCTAssertEqual(messagesByID["webex:workspace:message:webex-message-empty"]?.body, "")

        let evidence = try knowledgeStore.loadBeliefEvidence(scope: .space, entityKey: "room-1", limit: 10)
        XCTAssertEqual(Set(evidence.map(\.sourceID)), [
            "webex:workspace:message:webex-message-1",
            "webex:workspace:message:webex-message-2"
        ])
        XCTAssertFalse(evidence.contains { $0.sourceID == "webex:workspace:message:webex-message-empty" })

        let personMessages = try knowledgeStore.loadMessages(
            personID: "webex:workspace:person:person-1",
            limit: 10
        )
        XCTAssertEqual(Set(personMessages.map(\.id)), Set(messagesByID.keys))
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
        XCTAssertEqual(batchWrite.rooms.map(\.id), ["room-1"])
        XCTAssertEqual(batchWrite.people.map(\.id), ["webex:workspace:person:person-1"])
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

private final class StubFactoryWebexClient: WebexClienting, WebexProductClienting {
    var rooms: [WebexRoom] = []
    var membershipsByRoomID: [String: [WebexMembership]] = [:]
    var recentMessagesByRoomID: [String: [WebexMessage]] = [:]
    private(set) var recentFetches: [(String, Int)] = []

    func rooms() async throws -> [WebexRoom] {
        rooms
    }

    func memberships(roomID: String) async throws -> [WebexMembership] {
        membershipsByRoomID[roomID] ?? []
    }

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
        []
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

private func makeWebexRoom(id: String, title: String, type: String, lastActivity: String) -> WebexRoom {
    decodeWebexPayload([
        "id": id,
        "title": title,
        "type": type,
        "lastActivity": lastActivity
    ])
}

private func makeWebexMembership(id: String, roomID: String, personEmail: String) -> WebexMembership {
    decodeWebexPayload([
        "id": id,
        "roomId": roomID,
        "personId": "person-\(id)",
        "personEmail": personEmail,
        "personDisplayName": personEmail
    ])
}

private func decodeWebexPayload<T: Decodable>(_ payload: [String: Any]) -> T {
    let data = try! JSONSerialization.data(withJSONObject: payload, options: [])
    return try! JSONDecoder().decode(T.self, from: data)
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

private func makeMixedSignalMessageBatch(occurredAt: Date) -> SignalSyncBatch {
    var batch = makeSignalMessageBatch(occurredAt: occurredAt)
    let roomSourceID = SourceObjectID(connectorID: .webex, accountID: "workspace", kind: "room", externalID: "room-1")
    let personSourceID = SourceObjectID(connectorID: .webex, accountID: "workspace", kind: "person", externalID: "person-1")
    batch.events.append(
        makeSignalMessageEvent(
            externalID: "webex-message-2",
            roomSourceID: roomSourceID,
            personSourceID: personSourceID,
            occurredAt: occurredAt.addingTimeInterval(60),
            body: "  Can you review\u{00a0}Jira?  "
        )
    )
    batch.events.append(
        makeSignalMessageEvent(
            externalID: "webex-message-empty",
            roomSourceID: roomSourceID,
            personSourceID: personSourceID,
            occurredAt: occurredAt.addingTimeInterval(120),
            body: " \u{00a0} "
        )
    )
    return batch
}

private func makeSignalMessageEvent(
    externalID: String,
    roomSourceID: SourceObjectID,
    personSourceID: SourceObjectID,
    occurredAt: Date,
    body: String
) -> SignalEvent {
    let eventSourceID = SourceEventID(
        connectorID: .webex,
        accountID: "workspace",
        kind: "message",
        externalID: externalID
    )
    let payload = MessageEventPayload(
        threadID: roomSourceID.globalID,
        threadSourceID: roomSourceID,
        threadTitle: "Launch Room",
        senderID: personSourceID.globalID,
        senderDisplayName: "alex@example.com",
        senderEmail: "alex@example.com",
        body: body,
        isFromCurrentUser: false
    )
    return SignalEvent(
        id: eventSourceID.globalID,
        sourceID: eventSourceID,
        kind: .message,
        actor: SignalActor(id: personSourceID.globalID, displayName: "alex@example.com", email: "alex@example.com"),
        occurredAt: occurredAt,
        objectIDs: [roomSourceID.globalID, personSourceID.globalID],
        visibility: .authenticatedUser(connectorID: .webex, accountID: "workspace"),
        payload: .message(payload)
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

private func executeSignalSQLite(
    _ sql: String,
    databaseURL: URL,
    file: StaticString = #filePath,
    line: UInt = #line
) throws {
    var db: OpaquePointer?
    XCTAssertEqual(sqlite3_open(databaseURL.path, &db), SQLITE_OK, file: file, line: line)
    defer { sqlite3_close(db) }
    var errorMessage: UnsafeMutablePointer<Int8>?
    let result = sqlite3_exec(db, sql, nil, nil, &errorMessage)
    if let errorMessage {
        let message = String(cString: errorMessage)
        sqlite3_free(errorMessage)
        XCTFail("SQLite exec failed: \(message)", file: file, line: line)
    }
    XCTAssertEqual(result, SQLITE_OK, file: file, line: line)
}

private func appleMessageRawDate(_ date: Date, scale: TimeInterval) -> Int64 {
    let appleEpochOffset: TimeInterval = 978_307_200
    return Int64((date.timeIntervalSince1970 - appleEpochOffset) * scale)
}

private let testISO8601: ISO8601DateFormatter = {
    let formatter = ISO8601DateFormatter()
    formatter.formatOptions = [.withInternetDateTime, .withFractionalSeconds]
    return formatter
}()
