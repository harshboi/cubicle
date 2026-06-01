import Foundation
import XCTest
@testable import GetWebexSpaceMacApp

final class WebexSyncEngineTests: XCTestCase {
    func testLatestMessageSameAsLastSeenSkipsCatchupCall() async throws {
        let harness = try makeHarness()
        defer { harness.cleanup() }

        let conversation = makeConversation(roomID: "room-1")
        let latest = makeMessage(id: "m2", roomID: "room-1", createdAt: date(minutesFromBase: 2))
        try harness.knowledgeStore.upsertWebexSyncState(
            makeState(
                conversationID: conversation.conversationID,
                conversationType: .space,
                roomID: "room-1",
                lastSeenMessageID: "m2",
                lastSeenCreated: iso(date(minutesFromBase: 2)),
                pollingMode: .recent
            )
        )

        harness.client.setLatest(roomID: "room-1", message: latest)

        let result = await harness.engine.syncConversation(conversation, trigger: .scheduled)

        XCTAssertTrue(result.unchanged)
        XCTAssertNil(result.skippedReason)
        XCTAssertEqual(harness.client.recentCallCount(roomID: "room-1"), 0)
    }

    func testLatestMessageDiffTriggersCatchupAndProcessesUnseenOnly() async throws {
        let harness = try makeHarness()
        defer { harness.cleanup() }

        let conversation = makeConversation(roomID: "room-1")
        let m1 = makeMessage(id: "m1", roomID: "room-1", createdAt: date(minutesFromBase: 1))
        let m2 = makeMessage(id: "m2", roomID: "room-1", createdAt: date(minutesFromBase: 2))
        let m3 = makeMessage(id: "m3", roomID: "room-1", createdAt: date(minutesFromBase: 3))
        try harness.knowledgeStore.upsertWebexSyncState(
            makeState(
                conversationID: conversation.conversationID,
                conversationType: .space,
                roomID: "room-1",
                lastSeenMessageID: "m1",
                lastSeenCreated: iso(date(minutesFromBase: 1)),
                pollingMode: .recent
            )
        )

        harness.client.setLatest(roomID: "room-1", message: m3)
        harness.client.setRecent(roomID: "room-1", messages: [m3, m2, m1])

        let result = await harness.engine.syncConversation(conversation, trigger: .scheduled)

        XCTAssertEqual(harness.client.recentCallCount(roomID: "room-1"), 1)
        XCTAssertEqual(harness.client.lastRecentMax(roomID: "room-1"), 100)
        XCTAssertEqual(result.processedMessages, 2)
        XCTAssertEqual(result.latestMessageID, "m3")
        XCTAssertTrue(try harness.knowledgeStore.messageExists(messageID: "m2"))
        XCTAssertTrue(try harness.knowledgeStore.messageExists(messageID: "m3"))
        XCTAssertFalse(try harness.knowledgeStore.messageExists(messageID: "m1"))
    }

    func testCatchupStopsAtLastSeenMessageID() async throws {
        let harness = try makeHarness()
        defer { harness.cleanup() }

        let conversation = makeConversation(roomID: "room-2")
        let m1 = makeMessage(id: "m1", roomID: "room-2", createdAt: date(minutesFromBase: 1))
        let m2 = makeMessage(id: "m2", roomID: "room-2", createdAt: date(minutesFromBase: 2))
        let m3 = makeMessage(id: "m3", roomID: "room-2", createdAt: date(minutesFromBase: 3))
        let m4 = makeMessage(id: "m4", roomID: "room-2", createdAt: date(minutesFromBase: 4))
        try harness.knowledgeStore.upsertWebexSyncState(
            makeState(
                conversationID: conversation.conversationID,
                conversationType: .space,
                roomID: "room-2",
                lastSeenMessageID: "m2",
                lastSeenCreated: iso(date(minutesFromBase: 2)),
                pollingMode: .recent
            )
        )

        harness.client.setLatest(roomID: "room-2", message: m4)
        harness.client.setRecent(roomID: "room-2", messages: [m4, m3, m2, m1])

        let result = await harness.engine.syncConversation(conversation, trigger: .scheduled)

        XCTAssertEqual(result.processedMessages, 2)
        XCTAssertTrue(try harness.knowledgeStore.messageExists(messageID: "m3"))
        XCTAssertTrue(try harness.knowledgeStore.messageExists(messageID: "m4"))
        XCTAssertFalse(try harness.knowledgeStore.messageExists(messageID: "m1"))
    }

    func testDuplicateMessagesAreProcessedOnce() async throws {
        let harness = try makeHarness()
        defer { harness.cleanup() }

        let conversation = makeConversation(roomID: "room-3")
        let duplicate = makeMessage(id: "m9", roomID: "room-3", createdAt: date(minutesFromBase: 9))
        harness.client.setLatest(roomID: "room-3", message: duplicate)
        harness.client.setRecent(roomID: "room-3", messages: [duplicate, duplicate])

        let result = await harness.engine.syncConversation(conversation, trigger: .scheduled)

        XCTAssertEqual(result.processedMessages, 1)
        XCTAssertTrue(try harness.knowledgeStore.messageExists(messageID: "m9"))
    }

    func testProcessingFailureDoesNotAdvanceCursor() async throws {
        let runtimeRoot = temporaryRuntimeRoot(label: "cursor-failure")
        let configuration = testConfiguration(runtimeRoot: runtimeRoot)
        let knowledgeStore = KnowledgeStore(configuration: configuration)
        try knowledgeStore.bootstrap()

        let clock = TestClock(start: date(minutesFromBase: 0))
        let stateStore = SyncStateStore(knowledgeStore: knowledgeStore, now: clock.now)
        let client = FakeWebexClient()
        let processor = FailingMessageProcessor(failingMessageIDs: ["m2"])
        let engine = WebexSyncEngine(
            webexClient: client,
            stateStore: stateStore,
            messageProcessor: processor,
            configuration: .init(maxConcurrentAPIRequests: 3, activeIntervalSeconds: 20, recentIntervalSeconds: 60, backgroundIntervalSeconds: 180, jitterRatio: 0),
            now: clock.now,
            randomUnitInterval: { 0.5 }
        )
        defer { try? FileManager.default.removeItem(at: runtimeRoot) }

        let conversation = makeConversation(roomID: "room-fail")
        let m1 = makeMessage(id: "m1", roomID: "room-fail", createdAt: date(minutesFromBase: 1))
        let m2 = makeMessage(id: "m2", roomID: "room-fail", createdAt: date(minutesFromBase: 2))
        let m3 = makeMessage(id: "m3", roomID: "room-fail", createdAt: date(minutesFromBase: 3))
        try knowledgeStore.upsertWebexSyncState(
            makeState(
                conversationID: conversation.conversationID,
                conversationType: .space,
                roomID: "room-fail",
                lastSeenMessageID: "m1",
                lastSeenCreated: iso(date(minutesFromBase: 1)),
                pollingMode: .recent
            )
        )

        client.setLatest(roomID: "room-fail", message: m3)
        client.setRecent(roomID: "room-fail", messages: [m3, m2, m1])

        let result = await engine.syncConversation(conversation, trigger: .scheduled)
        let state = try XCTUnwrap(knowledgeStore.loadWebexSyncState(conversationID: conversation.conversationID))

        XCTAssertEqual(result.status, .offline)
        XCTAssertEqual(state.lastSeenMessageID, "m1")
    }

    func testRateLimitRetryAfterIsPersistedAndRespected() async throws {
        let harness = try makeHarness()
        defer { harness.cleanup() }

        let conversation = makeConversation(roomID: "room-429")
        harness.client.setLatestError(
            roomID: "room-429",
            error: .httpStatus(code: 429, detail: "rate limited", retryAfterSeconds: 120)
        )

        _ = await harness.engine.syncConversation(conversation, trigger: .scheduled)
        let firstState = try XCTUnwrap(
            harness.knowledgeStore.loadWebexSyncState(conversationID: conversation.conversationID)
        )
        let nextAllowed = try XCTUnwrap(parseISO(firstState.nextAllowedSyncAt))
        XCTAssertEqual(harness.client.latestCallCount(roomID: "room-429"), 1)
        XCTAssertGreaterThan(nextAllowed.timeIntervalSince(harness.clock.current), 110)

        let second = await harness.engine.syncConversation(conversation, trigger: .scheduled)
        XCTAssertEqual(harness.client.latestCallCount(roomID: "room-429"), 1)
        XCTAssertEqual(second.status, .delayedRateLimit)
    }

    func testServerErrorAppliesExponentialBackoff() async throws {
        let harness = try makeHarness(jitterRatio: 0)
        defer { harness.cleanup() }

        let conversation = makeConversation(roomID: "room-503")
        harness.client.setLatestError(
            roomID: "room-503",
            error: .httpStatus(code: 503, detail: "unavailable", retryAfterSeconds: nil)
        )

        _ = await harness.engine.syncConversation(conversation, trigger: .scheduled)
        let state = try XCTUnwrap(
            harness.knowledgeStore.loadWebexSyncState(conversationID: conversation.conversationID)
        )
        let nextAllowed = try XCTUnwrap(parseISO(state.nextAllowedSyncAt))
        let delay = nextAllowed.timeIntervalSince(harness.clock.current)

        XCTAssertEqual(state.consecutiveFailureCount, 1)
        XCTAssertEqual(Int(delay.rounded()), 30)
    }

    func testRestartLoadsPersistedCursor() async throws {
        let runtimeRoot = temporaryRuntimeRoot(label: "restart")
        defer { try? FileManager.default.removeItem(at: runtimeRoot) }
        let configuration = testConfiguration(runtimeRoot: runtimeRoot)
        let knowledgeStore = KnowledgeStore(configuration: configuration)
        try knowledgeStore.bootstrap()

        let clock = TestClock(start: date(minutesFromBase: 0))
        let conversation = makeConversation(roomID: "room-restart")
        let m1 = makeMessage(id: "m1", roomID: "room-restart", createdAt: date(minutesFromBase: 1))
        let m2 = makeMessage(id: "m2", roomID: "room-restart", createdAt: date(minutesFromBase: 2))

        let client1 = FakeWebexClient()
        client1.setLatest(roomID: "room-restart", message: m2)
        client1.setRecent(roomID: "room-restart", messages: [m2, m1])
        let engine1 = WebexSyncEngine(
            webexClient: client1,
            stateStore: SyncStateStore(knowledgeStore: knowledgeStore, now: clock.now),
            messageProcessor: MessageProcessor(knowledgeStore: knowledgeStore),
            configuration: .init(maxConcurrentAPIRequests: 3, activeIntervalSeconds: 20, recentIntervalSeconds: 60, backgroundIntervalSeconds: 180, jitterRatio: 0),
            now: clock.now,
            randomUnitInterval: { 0.5 }
        )
        _ = await engine1.syncConversation(conversation, trigger: .scheduled)

        let client2 = FakeWebexClient()
        client2.setLatest(roomID: "room-restart", message: m2)
        let engine2 = WebexSyncEngine(
            webexClient: client2,
            stateStore: SyncStateStore(knowledgeStore: knowledgeStore, now: clock.now),
            messageProcessor: MessageProcessor(knowledgeStore: knowledgeStore),
            configuration: .init(maxConcurrentAPIRequests: 3, activeIntervalSeconds: 20, recentIntervalSeconds: 60, backgroundIntervalSeconds: 180, jitterRatio: 0),
            now: clock.now,
            randomUnitInterval: { 0.5 }
        )

        _ = await engine2.syncConversation(conversation, trigger: .scheduled)

        XCTAssertEqual(client2.recentCallCount(roomID: "room-restart"), 0)
        let state = try XCTUnwrap(knowledgeStore.loadWebexSyncState(conversationID: conversation.conversationID))
        XCTAssertEqual(state.lastSeenMessageID, "m2")
    }

    func testDirectMessageDiscoveryPersistsRoomAndAvoidsRepeatedLookup() async throws {
        let harness = try makeHarness()
        defer { harness.cleanup() }

        let conversation = WebexTrackedConversation(
            conversationID: "person:apolak@cisco.com",
            conversationType: .direct,
            roomID: "",
            personID: nil,
            personEmail: "apolak@cisco.com",
            displayName: "Apolak",
            pollingMode: .recent
        )
        let discovery = makeMessage(
            id: "dm-discovery",
            roomID: "dm-room-1",
            personID: "person-a",
            personEmail: "apolak@cisco.com",
            text: "hello",
            createdAt: date(minutesFromBase: 1)
        )
        let latest = makeMessage(id: "dm-latest", roomID: "dm-room-1", createdAt: date(minutesFromBase: 2))
        harness.client.setDirect(personEmail: "apolak@cisco.com", messages: [discovery])
        harness.client.setLatest(roomID: "dm-room-1", message: latest)
        harness.client.setRecent(roomID: "dm-room-1", messages: [latest, discovery])

        _ = await harness.engine.syncConversation(conversation, trigger: .scheduled)
        _ = await harness.engine.syncConversation(conversation, trigger: .scheduled)

        XCTAssertEqual(harness.client.directCallCount, 1)
        let state = try XCTUnwrap(
            harness.knowledgeStore.loadWebexSyncState(conversationID: conversation.conversationID)
        )
        XCTAssertEqual(state.roomID, "dm-room-1")
    }

    func testGlobalConcurrencyLimitIsRespected() async throws {
        let harness = try makeHarness(maxConcurrentRequests: 3)
        defer { harness.cleanup() }

        harness.client.latestDelayNanoseconds = 120_000_000
        for index in 0..<5 {
            harness.client.setLatest(roomID: "room-concurrency-\(index)", message: nil)
        }
        let conversations = (0..<5).map { index in
            makeConversation(roomID: "room-concurrency-\(index)")
        }

        _ = await harness.engine.syncConversations(conversations, trigger: .scheduled)

        XCTAssertLessThanOrEqual(harness.client.maxLatestConcurrencyObserved, 3)
    }

    func testSingleFlightPreventsDuplicateRoomCalls() async throws {
        let harness = try makeHarness()
        defer { harness.cleanup() }

        harness.client.latestDelayNanoseconds = 150_000_000
        let conversation = makeConversation(roomID: "room-single-flight")
        harness.client.setLatest(roomID: "room-single-flight", message: nil)

        async let first = harness.engine.syncConversation(conversation, trigger: .scheduled)
        async let second = harness.engine.syncConversation(conversation, trigger: .scheduled)
        _ = await first
        _ = await second

        XCTAssertEqual(harness.client.latestCallCount(roomID: "room-single-flight"), 1)
    }

    func testWakeAndNetworkReconnectForceCatchupPassEvenWhenNextAllowedIsFuture() async throws {
        let harness = try makeHarness()
        defer { harness.cleanup() }

        let conversation = makeConversation(roomID: "room-wake-network")
        try harness.knowledgeStore.upsertWebexSyncState(
            makeState(
                conversationID: conversation.conversationID,
                conversationType: .space,
                roomID: "room-wake-network",
                lastSeenMessageID: "m1",
                lastSeenCreated: iso(date(minutesFromBase: 1)),
                nextAllowedSyncAt: iso(harness.clock.current.addingTimeInterval(300)),
                pollingMode: .recent
            )
        )
        harness.client.setLatest(roomID: "room-wake-network", message: nil)

        _ = await harness.engine.syncConversation(conversation, trigger: .wakeFromSleep)
        XCTAssertEqual(harness.client.latestCallCount(roomID: "room-wake-network"), 1)

        try harness.knowledgeStore.upsertWebexSyncState(
            makeState(
                conversationID: conversation.conversationID,
                conversationType: .space,
                roomID: "room-wake-network",
                lastSeenMessageID: "m1",
                lastSeenCreated: iso(date(minutesFromBase: 1)),
                nextAllowedSyncAt: iso(harness.clock.current.addingTimeInterval(300)),
                pollingMode: .recent
            )
        )

        _ = await harness.engine.syncConversation(conversation, trigger: .networkReconnect)
        XCTAssertEqual(harness.client.latestCallCount(roomID: "room-wake-network"), 2)
    }
}

private struct WebexSyncHarness {
    var runtimeRoot: URL
    var knowledgeStore: KnowledgeStore
    var clock: TestClock
    var client: FakeWebexClient
    var engine: WebexSyncEngine

    func cleanup() {
        try? FileManager.default.removeItem(at: runtimeRoot)
    }
}

private func makeHarness(
    jitterRatio: Double = 0,
    maxConcurrentRequests: Int = 3
) throws -> WebexSyncHarness {
    let runtimeRoot = temporaryRuntimeRoot(label: "webex-sync")
    let configuration = testConfiguration(runtimeRoot: runtimeRoot)
    let knowledgeStore = KnowledgeStore(configuration: configuration)
    try knowledgeStore.bootstrap()
    let clock = TestClock(start: date(minutesFromBase: 0))
    let stateStore = SyncStateStore(knowledgeStore: knowledgeStore, now: clock.now)
    let client = FakeWebexClient()
    let processor = MessageProcessor(knowledgeStore: knowledgeStore)
    let engine = WebexSyncEngine(
        webexClient: client,
        stateStore: stateStore,
        messageProcessor: processor,
        configuration: .init(
            maxConcurrentAPIRequests: maxConcurrentRequests,
            activeIntervalSeconds: 20,
            recentIntervalSeconds: 60,
            backgroundIntervalSeconds: 180,
            jitterRatio: jitterRatio
        ),
        now: clock.now,
        randomUnitInterval: { 0.5 }
    )
    return WebexSyncHarness(
        runtimeRoot: runtimeRoot,
        knowledgeStore: knowledgeStore,
        clock: clock,
        client: client,
        engine: engine
    )
}

private func makeConversation(
    roomID: String,
    pollingMode: WebexPollingMode = .recent
) -> WebexTrackedConversation {
    WebexTrackedConversation(
        conversationID: "room:\(roomID)",
        conversationType: .space,
        roomID: roomID,
        personID: nil,
        personEmail: nil,
        displayName: roomID,
        pollingMode: pollingMode
    )
}

private func makeState(
    conversationID: String,
    conversationType: WebexConversationType,
    roomID: String,
    lastSeenMessageID: String? = nil,
    lastSeenCreated: String? = nil,
    lastSuccessfulSyncAt: String? = nil,
    nextAllowedSyncAt: String? = nil,
    pollingMode: WebexPollingMode = .recent,
    consecutiveFailureCount: Int = 0,
    lastError: String? = nil,
    lastErrorAt: String? = nil
) -> WebexConversationSyncStateRecord {
    WebexConversationSyncStateRecord(
        conversationID: conversationID,
        conversationType: conversationType,
        roomID: roomID,
        personID: nil,
        personEmail: nil,
        title: nil,
        lastSeenMessageID: lastSeenMessageID,
        lastSeenCreated: lastSeenCreated,
        lastSuccessfulSyncAt: lastSuccessfulSyncAt,
        nextAllowedSyncAt: nextAllowedSyncAt,
        pollingMode: pollingMode,
        consecutiveFailureCount: consecutiveFailureCount,
        lastError: lastError,
        lastErrorAt: lastErrorAt,
        updatedAt: iso(date(minutesFromBase: 0))
    )
}

private final class TestClock {
    var current: Date

    init(start: Date) {
        self.current = start
    }

    func now() -> Date {
        current
    }
}

private final class FailingMessageProcessor: MessageProcessing {
    private let lock = NSLock()
    private let failingMessageIDs: Set<String>
    private var processedMessageIDs = Set<String>()

    init(failingMessageIDs: Set<String>) {
        self.failingMessageIDs = failingMessageIDs
    }

    func process(
        message: WebexMessage,
        conversation: WebexTrackedConversation,
        updatedAt: String
    ) throws -> MessageProcessOutcome {
        lock.lock()
        defer { lock.unlock() }
        if failingMessageIDs.contains(message.id) {
            throw NSError(domain: "TestProcessor", code: 1, userInfo: [NSLocalizedDescriptionKey: "forced failure"])
        }
        if processedMessageIDs.contains(message.id) {
            return .duplicate
        }
        processedMessageIDs.insert(message.id)
        return .processed(evidenceIndexed: 1)
    }

    func messageExists(messageID: String) throws -> Bool {
        lock.lock()
        defer { lock.unlock() }
        return processedMessageIDs.contains(messageID)
    }
}

private final class FakeWebexClient: WebexClienting {
    private let lock = NSLock()

    private var latestByRoom: [String: WebexMessage?] = [:]
    private var latestErrorsByRoom: [String: WebexAPIError] = [:]
    private var recentByRoom: [String: [WebexMessage]] = [:]
    private var recentErrorsByRoom: [String: WebexAPIError] = [:]
    private var messageByID: [String: WebexMessage] = [:]
    private var directByLookup: [String: [WebexMessage]] = [:]
    private var directErrorsByLookup: [String: WebexAPIError] = [:]

    private var latestCallsByRoom: [String: Int] = [:]
    private var recentCallsByRoom: [String: Int] = [:]
    private var recentMaxByRoom: [String: Int] = [:]
    private(set) var directCallCount: Int = 0

    var latestDelayNanoseconds: UInt64 = 0
    private var latestInFlight: Int = 0
    private(set) var maxLatestConcurrencyObserved: Int = 0

    private func withLockedState<T>(_ body: () -> T) -> T {
        lock.lock()
        defer { lock.unlock() }
        return body()
    }

    func setLatest(roomID: String, message: WebexMessage?) {
        withLockedState {
            latestByRoom[roomID] = message
            latestErrorsByRoom.removeValue(forKey: roomID)
        }
    }

    func setLatestError(roomID: String, error: WebexAPIError) {
        withLockedState {
            latestErrorsByRoom[roomID] = error
        }
    }

    func setRecent(roomID: String, messages: [WebexMessage]) {
        withLockedState {
            recentByRoom[roomID] = messages
            recentErrorsByRoom.removeValue(forKey: roomID)
        }
    }

    func setRecentError(roomID: String, error: WebexAPIError) {
        withLockedState {
            recentErrorsByRoom[roomID] = error
        }
    }

    func setDirect(personEmail: String, messages: [WebexMessage]) {
        withLockedState {
            directByLookup["email:\(personEmail.lowercased())"] = messages
            directErrorsByLookup.removeValue(forKey: "email:\(personEmail.lowercased())")
        }
    }

    func latestCallCount(roomID: String) -> Int {
        withLockedState { latestCallsByRoom[roomID] ?? 0 }
    }

    func recentCallCount(roomID: String) -> Int {
        withLockedState { recentCallsByRoom[roomID] ?? 0 }
    }

    func lastRecentMax(roomID: String) -> Int {
        withLockedState { recentMaxByRoom[roomID] ?? 0 }
    }

    func fetchLatestMessage(roomID: String) async throws -> WebexMessage? {
        let snapshot = withLockedState { () -> (WebexAPIError?, WebexMessage??, UInt64) in
            latestCallsByRoom[roomID, default: 0] += 1
            latestInFlight += 1
            maxLatestConcurrencyObserved = max(maxLatestConcurrencyObserved, latestInFlight)
            return (latestErrorsByRoom[roomID], latestByRoom[roomID], latestDelayNanoseconds)
        }
        defer {
            withLockedState {
                latestInFlight -= 1
            }
        }

        if snapshot.2 > 0 {
            try await Task.sleep(nanoseconds: snapshot.2)
        }
        if let error = snapshot.0 {
            throw error
        }
        return snapshot.1 ?? nil
    }

    func fetchRecentMessages(roomID: String, max: Int) async throws -> [WebexMessage] {
        let snapshot = withLockedState { () -> (WebexAPIError?, [WebexMessage]) in
            recentCallsByRoom[roomID, default: 0] += 1
            recentMaxByRoom[roomID] = max
            return (recentErrorsByRoom[roomID], recentByRoom[roomID] ?? [])
        }
        if let error = snapshot.0 {
            throw error
        }
        return snapshot.1
    }

    func fetchMessage(messageID: String) async throws -> WebexMessage {
        let message = withLockedState { messageByID[messageID] }
        if let message {
            return message
        }
        throw WebexAPIError.httpStatus(code: 404, detail: "not found", retryAfterSeconds: nil)
    }

    func fetchDirectMessages(personEmail: String?, personID: String?, max: Int) async throws -> [WebexMessage] {
        let key = directLookupKey(personEmail: personEmail, personID: personID)
        let snapshot = withLockedState { () -> (WebexAPIError?, [WebexMessage]) in
            directCallCount += 1
            return (directErrorsByLookup[key], directByLookup[key] ?? [])
        }
        if let error = snapshot.0 {
            throw error
        }
        return snapshot.1
    }

    private func directLookupKey(personEmail: String?, personID: String?) -> String {
        let email = personEmail?.trimmingCharacters(in: .whitespacesAndNewlines).lowercased() ?? ""
        if !email.isEmpty {
            return "email:\(email)"
        }
        let personID = personID?.trimmingCharacters(in: .whitespacesAndNewlines) ?? ""
        return "person:\(personID)"
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

private func makeMessage(
    id: String,
    roomID: String,
    personID: String = "person-id",
    personEmail: String = "person@example.com",
    text: String = "message",
    createdAt: Date
) -> WebexMessage {
    let payload: [String: Any] = [
        "id": id,
        "roomId": roomID,
        "personId": personID,
        "personEmail": personEmail,
        "text": text,
        "created": iso(createdAt)
    ]
    let data = try! JSONSerialization.data(withJSONObject: payload, options: [])
    return try! JSONDecoder().decode(WebexMessage.self, from: data)
}

private func date(minutesFromBase minutes: Int) -> Date {
    Date(timeIntervalSince1970: 1_715_000_000).addingTimeInterval(TimeInterval(minutes * 60))
}

private func parseISO(_ value: String?) -> Date? {
    guard let value else { return nil }
    if let withFractional = testISO8601WithFractional.date(from: value) {
        return withFractional
    }
    return testISO8601.date(from: value)
}

private func iso(_ date: Date) -> String {
    testISO8601WithFractional.string(from: date)
}

private let testISO8601WithFractional: ISO8601DateFormatter = {
    let formatter = ISO8601DateFormatter()
    formatter.formatOptions = [.withInternetDateTime, .withFractionalSeconds]
    return formatter
}()

private let testISO8601: ISO8601DateFormatter = {
    let formatter = ISO8601DateFormatter()
    formatter.formatOptions = [.withInternetDateTime]
    return formatter
}()
