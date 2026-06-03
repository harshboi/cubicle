import Foundation
import OSLog

/// Client errors normalized for sync status and backoff decisions.
enum WebexSyncClientError: LocalizedError, Equatable {
    case unauthorized
    case forbidden
    case notFound
    case rateLimited(retryAfterSeconds: Int)
    case transientServerError
    case networkError
    case decodeError
    case unknown(String)

    var errorDescription: String? {
        switch self {
        case .unauthorized:
            return "Webex auth required."
        case .forbidden:
            return "Webex access forbidden."
        case .notFound:
            return "Webex conversation not found."
        case .rateLimited(let retryAfterSeconds):
            return "Webex rate-limited. Retry after \(retryAfterSeconds)s."
        case .transientServerError:
            return "Webex transient server error."
        case .networkError:
            return "Webex network error."
        case .decodeError:
            return "Webex response decoding failed."
        case .unknown(let detail):
            return detail
        }
    }
}

/// Coarse status surfaced in sync results and focus badges.
enum WebexSyncStatusIndicator: String, Hashable {
    case synced
    case syncing
    case delayedRateLimit
    case offline
    case authRequired
}

/// External reason for a sync run; force/backoff behavior depends on this.
enum WebexSyncTriggerReason: String, Hashable {
    case startup
    case scheduled
    case manual
    case wakeFromSleep
    case networkReconnect
    case userOpenedConversation
}

/// Conversation descriptor passed into the adaptive polling engine.
struct WebexTrackedConversation: Hashable {
    var conversationID: String
    var conversationType: WebexConversationType
    var roomID: String
    var personID: String?
    var personEmail: String?
    var displayName: String
    var pollingMode: WebexPollingMode
}

/// Per-conversation sync result before ingestion maps it back to room summaries.
struct WebexConversationSyncResult: Hashable {
    var conversationID: String
    var roomID: String
    var displayName: String
    var fetchedMessages: Int
    var processedMessages: Int
    var processedEvidence: Int
    var latestMessageID: String?
    var latestMessageCreated: String?
    var skippedReason: String?
    var unchanged: Bool
    var status: WebexSyncStatusIndicator
}

/// Testable Webex client facade used by the sync engine.
protocol WebexClienting {
    func fetchLatestMessage(roomID: String) async throws -> WebexMessage?
    func fetchRecentMessages(roomID: String, max: Int) async throws -> [WebexMessage]
    func fetchMessage(messageID: String) async throws -> WebexMessage
    func fetchDirectMessages(personEmail: String?, personID: String?, max: Int) async throws -> [WebexMessage]
}

extension WebexAPIClient: WebexClienting {
    func fetchLatestMessage(roomID: String) async throws -> WebexMessage? {
        try await latestMessage(roomID: roomID)
    }

    func fetchRecentMessages(roomID: String, max: Int) async throws -> [WebexMessage] {
        try await messages(roomID: roomID, max: max)
    }

    func fetchMessage(messageID: String) async throws -> WebexMessage {
        try await message(id: messageID)
    }

    func fetchDirectMessages(personEmail: String?, personID: String?, max: Int) async throws -> [WebexMessage] {
        try await directMessages(personEmail: personEmail, personID: personID, max: max)
    }
}

/// Actor-backed semaphore limiting concurrent Webex API calls.
private actor AsyncPermitPool {
    private let maxPermits: Int
    private var availablePermits: Int
    private var waiters: [CheckedContinuation<Void, Never>] = []

    init(maxPermits: Int) {
        let bounded = max(1, maxPermits)
        self.maxPermits = bounded
        self.availablePermits = bounded
    }

    func acquire() async {
        if availablePermits > 0 {
            availablePermits -= 1
            return
        }
        await withCheckedContinuation { continuation in
            waiters.append(continuation)
        }
    }

    func release() {
        if waiters.isEmpty {
            availablePermits = min(maxPermits, availablePermits + 1)
            return
        }
        let waiter = waiters.removeFirst()
        waiter.resume()
    }
}

/// State boundary used by the Webex sync engine.
protocol WebexSyncStateStoring: AnyObject {
    func loadOrCreate(for conversation: WebexTrackedConversation) async throws -> WebexConversationSyncStateRecord
    func save(_ state: WebexConversationSyncStateRecord) async throws
}

/// Serializes sync watermark reads/writes through `KnowledgeStore`.
actor SyncStateStore: WebexSyncStateStoring {
    private let knowledgeStore: KnowledgeStore
    private let now: () -> Date

    /// Creates a state store with injectable time for deterministic tests.
    init(
        knowledgeStore: KnowledgeStore,
        now: @escaping () -> Date = Date.init
    ) {
        self.knowledgeStore = knowledgeStore
        self.now = now
    }

    /// Loads persisted state or creates a blank watermark for a new conversation.
    func loadOrCreate(for conversation: WebexTrackedConversation) async throws -> WebexConversationSyncStateRecord {
        if let existing = try knowledgeStore.loadWebexSyncState(conversationID: conversation.conversationID) {
            return existing
        }
        let roomID = normalizedRoomID(conversation.roomID)
        return WebexConversationSyncStateRecord(
            conversationID: conversation.conversationID,
            conversationType: conversation.conversationType,
            roomID: roomID,
            personID: normalizedOptional(conversation.personID),
            personEmail: normalizedOptional(conversation.personEmail?.lowercased()),
            title: normalizedOptional(conversation.displayName),
            lastSeenMessageID: nil,
            lastSeenCreated: nil,
            lastSuccessfulSyncAt: nil,
            nextAllowedSyncAt: nil,
            pollingMode: conversation.pollingMode,
            consecutiveFailureCount: 0,
            lastError: nil,
            lastErrorAt: nil,
            updatedAt: Self.iso8601(now())
        )
    }

    /// Persists the latest watermark/backoff state.
    func save(_ state: WebexConversationSyncStateRecord) async throws {
        try knowledgeStore.upsertWebexSyncState(state)
    }

    private func normalizedRoomID(_ value: String) -> String {
        value.trimmingCharacters(in: .whitespacesAndNewlines)
    }

    private func normalizedOptional(_ value: String?) -> String? {
        guard let value = value?.trimmingCharacters(in: .whitespacesAndNewlines),
              !value.isEmpty else {
            return nil
        }
        return value
    }

    static func iso8601(_ date: Date) -> String {
        let formatter = ISO8601DateFormatter()
        formatter.formatOptions = [.withInternetDateTime, .withFractionalSeconds]
        return formatter.string(from: date)
    }
}

/// Result of attempting to index one Webex message.
enum MessageProcessOutcome: Hashable {
    case processed(evidenceIndexed: Int)
    case duplicate
    case ignoredSelf
}

/// Boundary between sync polling and knowledge-store writes.
protocol MessageProcessing {
    /// Indexes a message and returns duplicate/self-message handling status.
    func process(
        message: WebexMessage,
        conversation: WebexTrackedConversation,
        updatedAt: String
    ) throws -> MessageProcessOutcome

    func messageExists(messageID: String) throws -> Bool
}

/// Converts Webex messages into room/person/message/evidence records.
final class MessageProcessor: MessageProcessing {
    private let knowledgeStore: KnowledgeStore
    private let ignoreSelfMessages: Bool
    private let selfPersonID: String?
    private let selfEmail: String?

    /// Configures self-message filtering for the current Webex user.
    init(
        knowledgeStore: KnowledgeStore,
        ignoreSelfMessages: Bool = true,
        selfPersonID: String? = nil,
        selfEmail: String? = nil
    ) {
        self.knowledgeStore = knowledgeStore
        self.ignoreSelfMessages = ignoreSelfMessages
        self.selfPersonID = selfPersonID?.trimmingCharacters(in: .whitespacesAndNewlines)
        self.selfEmail = selfEmail?.trimmingCharacters(in: .whitespacesAndNewlines).lowercased()
    }

    /// Idempotently indexes a Webex message and its belief evidence.
    func process(
        message: WebexMessage,
        conversation: WebexTrackedConversation,
        updatedAt: String
    ) throws -> MessageProcessOutcome {
        if ignoreSelfMessages {
            let authorPersonID = message.personID.trimmingCharacters(in: .whitespacesAndNewlines)
            let authorEmail = message.personEmail.trimmingCharacters(in: .whitespacesAndNewlines).lowercased()
            if (!authorPersonID.isEmpty && authorPersonID == selfPersonID) ||
                (!authorEmail.isEmpty && authorEmail == selfEmail) {
                return .ignoredSelf
            }
        }

        if try knowledgeStore.messageExists(messageID: message.id) {
            return .duplicate
        }

        let roomID = resolvedRoomID(messageRoomID: message.roomID, fallback: conversation.roomID)
        let personID = resolvedPersonID(messagePersonID: message.personID, fallbackEmail: message.personEmail)
        if !personID.isEmpty {
            try knowledgeStore.upsertPerson(
                PersonRecord(
                    id: personID,
                    displayName: senderDisplayName(email: message.personEmail, fallback: personID),
                    email: message.personEmail.trimmingCharacters(in: .whitespacesAndNewlines).lowercased(),
                    updatedAt: updatedAt
                )
            )
        }

        let createdAt = normalizedTimestamp(message.created, fallback: updatedAt)
        try knowledgeStore.upsertRoom(
            RoomRecord(
                id: roomID,
                title: conversation.displayName,
                updatedAt: createdAt
            )
        )
        try knowledgeStore.upsertMessage(
            MessageRecord(
                id: message.id,
                roomID: roomID,
                personID: personID.isEmpty ? nil : personID,
                body: normalizedMessageText(message.text),
                createdAt: createdAt,
                updatedAt: updatedAt
            )
        )

        let cleanedText = normalizedMessageText(message.text)
        guard !cleanedText.isEmpty else {
            return .processed(evidenceIndexed: 0)
        }
        try knowledgeStore.upsertBeliefEvidence(
            BeliefEvidenceRecord(
                id: "webex-message-\(message.id)",
                source: "webex_message",
                sourceID: message.id,
                roomID: roomID,
                personID: personID.isEmpty ? nil : personID,
                occurredAt: createdAt,
                text: cleanedText
            )
        )
        return .processed(evidenceIndexed: 1)
    }

    func messageExists(messageID: String) throws -> Bool {
        try knowledgeStore.messageExists(messageID: messageID)
    }

    private func resolvedRoomID(messageRoomID: String, fallback: String) -> String {
        let normalized = messageRoomID.trimmingCharacters(in: .whitespacesAndNewlines)
        if !normalized.isEmpty {
            return normalized
        }
        return fallback.trimmingCharacters(in: .whitespacesAndNewlines)
    }

    private func resolvedPersonID(messagePersonID: String, fallbackEmail: String) -> String {
        let normalized = messagePersonID.trimmingCharacters(in: .whitespacesAndNewlines)
        if !normalized.isEmpty {
            return normalized
        }
        return fallbackEmail.trimmingCharacters(in: .whitespacesAndNewlines).lowercased()
    }

    private func senderDisplayName(email: String, fallback: String) -> String {
        let normalized = email.trimmingCharacters(in: .whitespacesAndNewlines)
        return normalized.isEmpty ? fallback : normalized
    }

    private func normalizedMessageText(_ value: String) -> String {
        value
            .replacingOccurrences(of: "\u{00a0}", with: " ")
            .trimmingCharacters(in: .whitespacesAndNewlines)
    }

    private func normalizedTimestamp(_ value: String, fallback: String) -> String {
        let trimmed = value.trimmingCharacters(in: .whitespacesAndNewlines)
        return trimmed.isEmpty ? fallback : trimmed
    }
}

/// Adaptive polling engine for Webex conversations.
///
/// It owns concurrency limits, in-flight de-duping, watermarks, and backoff.
actor WebexSyncEngine {
    /// Polling cadence and concurrency knobs.
    struct Configuration: Hashable {
        var maxConcurrentAPIRequests: Int = 3
        var activeIntervalSeconds: TimeInterval = 20
        var recentIntervalSeconds: TimeInterval = 60
        var backgroundIntervalSeconds: TimeInterval = 180
        var jitterRatio: Double = 0.20
    }

    private let webexClient: WebexClienting
    private let stateStore: any WebexSyncStateStoring
    private let messageProcessor: MessageProcessing
    private let requestPermits: AsyncPermitPool
    private let configuration: Configuration
    private let now: () -> Date
    private let randomUnitInterval: () -> Double
    private let logger = Logger(subsystem: "Cubicle", category: "WebexSync")
    private var inFlightByRoomID: [String: Task<WebexConversationSyncResult, Never>] = [:]

    /// Creates an engine with injectable time/randomness for deterministic tests.
    init(
        webexClient: WebexClienting,
        stateStore: any WebexSyncStateStoring,
        messageProcessor: MessageProcessing,
        configuration: Configuration = Configuration(),
        now: @escaping () -> Date = Date.init,
        randomUnitInterval: @escaping () -> Double = { Double.random(in: 0...1) }
    ) {
        self.webexClient = webexClient
        self.stateStore = stateStore
        self.messageProcessor = messageProcessor
        self.configuration = configuration
        self.now = now
        self.randomUnitInterval = randomUnitInterval
        self.requestPermits = AsyncPermitPool(maxPermits: max(1, configuration.maxConcurrentAPIRequests))
    }

    /// Syncs conversations concurrently while preserving input order in results.
    func syncConversations(
        _ conversations: [WebexTrackedConversation],
        trigger: WebexSyncTriggerReason
    ) async -> [WebexConversationSyncResult] {
        await withTaskGroup(of: (Int, WebexConversationSyncResult).self) { group in
            for (index, conversation) in conversations.enumerated() {
                group.addTask { [weak self] in
                    guard let self else {
                        return (
                            index,
                            WebexConversationSyncResult(
                                conversationID: conversation.conversationID,
                                roomID: conversation.roomID,
                                displayName: conversation.displayName,
                                fetchedMessages: 0,
                                processedMessages: 0,
                                processedEvidence: 0,
                                latestMessageID: nil,
                                latestMessageCreated: nil,
                                skippedReason: "Sync engine unavailable.",
                                unchanged: true,
                                status: .offline
                            )
                        )
                    }
                    return (index, await self.syncConversation(conversation, trigger: trigger))
                }
            }

            var indexedResults: [Int: WebexConversationSyncResult] = [:]
            indexedResults.reserveCapacity(conversations.count)
            while let (index, result) = await group.next() {
                indexedResults[index] = result
            }
            return conversations.indices.compactMap { indexedResults[$0] }
        }
    }

    /// Syncs one conversation with in-flight coalescing by room/conversation key.
    func syncConversation(
        _ conversation: WebexTrackedConversation,
        trigger: WebexSyncTriggerReason
    ) async -> WebexConversationSyncResult {
        let force = trigger == .manual
            || trigger == .userOpenedConversation
            || trigger == .startup
            || trigger == .wakeFromSleep
            || trigger == .networkReconnect
        let roomID = normalizedOptional(conversation.roomID) ?? "unknown-room"
        let roomKey = normalizedOptional(conversation.roomID)
            ?? "conversation:\(conversation.conversationID)"

        if let existing = inFlightByRoomID[roomKey] {
            return await existing.value
        }

        let task = Task { [weak self] () -> WebexConversationSyncResult in
            guard let self else {
                return WebexConversationSyncResult(
                    conversationID: conversation.conversationID,
                    roomID: roomID,
                    displayName: conversation.displayName,
                    fetchedMessages: 0,
                    processedMessages: 0,
                    processedEvidence: 0,
                    latestMessageID: nil,
                    latestMessageCreated: nil,
                    skippedReason: "Sync engine unavailable.",
                    unchanged: true,
                    status: .offline
                )
            }
            return await self.performSync(conversation, trigger: trigger, force: force)
        }
        inFlightByRoomID[roomKey] = task
        let result = await task.value
        inFlightByRoomID.removeValue(forKey: roomKey)
        return result
    }

    /// Performs watermark checks, catch-up fetch, message processing, and state updates.
    private func performSync(
        _ conversation: WebexTrackedConversation,
        trigger: WebexSyncTriggerReason,
        force: Bool
    ) async -> WebexConversationSyncResult {
        let syncStartedAt = now()
        var state: WebexConversationSyncStateRecord
        do {
            state = try await stateStore.loadOrCreate(for: conversation)
        } catch {
            return await failureResult(
                conversation: conversation,
                state: nil,
                mappedError: .unknown("Sync state load failed: \(error.localizedDescription)"),
                fetchedMessages: 0,
                processedMessages: 0,
                processedEvidence: 0,
                trigger: trigger
            )
        }

        let effectiveRoomID = normalizedOptional(state.roomID)
            ?? normalizedOptional(conversation.roomID)

        if conversation.pollingMode == .paused || conversation.pollingMode == .disabled {
            if !force {
                return WebexConversationSyncResult(
                    conversationID: conversation.conversationID,
                    roomID: effectiveRoomID ?? "",
                    displayName: conversation.displayName,
                    fetchedMessages: 0,
                    processedMessages: 0,
                    processedEvidence: 0,
                    latestMessageID: state.lastSeenMessageID,
                    latestMessageCreated: state.lastSeenCreated,
                    skippedReason: "Polling is \(conversation.pollingMode.rawValue).",
                    unchanged: true,
                    status: .synced
                )
            }
        }

        if !force,
           let nextAllowed = parsedDate(state.nextAllowedSyncAt),
           nextAllowed > now() {
            logger.debug("sync_skip_next_allowed conversation=\(conversation.conversationID, privacy: .public) room=\((effectiveRoomID ?? ""), privacy: .public) next_allowed=\(SyncStateStore.iso8601(nextAllowed), privacy: .public)")
            return WebexConversationSyncResult(
                conversationID: conversation.conversationID,
                roomID: effectiveRoomID ?? "",
                displayName: conversation.displayName,
                fetchedMessages: 0,
                processedMessages: 0,
                processedEvidence: 0,
                latestMessageID: state.lastSeenMessageID,
                latestMessageCreated: state.lastSeenCreated,
                skippedReason: "Next allowed sync at \(SyncStateStore.iso8601(nextAllowed)).",
                unchanged: true,
                status: .delayedRateLimit
            )
        }

        let resolvedRoomID: String
        do {
            resolvedRoomID = try await resolveRoomID(conversation: conversation, state: &state)
        } catch {
            let mapped = Self.mapClientError(error)
            return await failureResult(
                conversation: conversation,
                state: state,
                mappedError: mapped,
                fetchedMessages: 0,
                processedMessages: 0,
                processedEvidence: 0,
                trigger: trigger
            )
        }

        guard !resolvedRoomID.isEmpty else {
            return WebexConversationSyncResult(
                conversationID: conversation.conversationID,
                roomID: "",
                displayName: conversation.displayName,
                fetchedMessages: 0,
                processedMessages: 0,
                processedEvidence: 0,
                latestMessageID: state.lastSeenMessageID,
                latestMessageCreated: state.lastSeenCreated,
                skippedReason: "Missing room ID and DM discovery did not resolve one.",
                unchanged: true,
                status: .offline
            )
        }

        logger.debug("sync_start conversation=\(conversation.conversationID, privacy: .public) room=\(resolvedRoomID, privacy: .public) trigger=\(trigger.rawValue, privacy: .public)")

        do {
            let latest = try await withRequestPermit {
                try await webexClient.fetchLatestMessage(roomID: resolvedRoomID)
            }

            if latest == nil {
                state.lastSuccessfulSyncAt = SyncStateStore.iso8601(now())
                state.nextAllowedSyncAt = nextAllowedForSuccess(mode: conversation.pollingMode, from: now())
                state.consecutiveFailureCount = 0
                state.lastError = nil
                state.lastErrorAt = nil
                state.updatedAt = SyncStateStore.iso8601(now())
                try await stateStore.save(state)
                return successResult(
                    conversation: conversation,
                    state: state,
                    roomID: resolvedRoomID,
                    fetchedMessages: 0,
                    processedMessages: 0,
                    processedEvidence: 0,
                    unchanged: true
                )
            }

            guard let latest else {
                return successResult(
                    conversation: conversation,
                    state: state,
                    roomID: resolvedRoomID,
                    fetchedMessages: 0,
                    processedMessages: 0,
                    processedEvidence: 0,
                    unchanged: true
                )
            }

            if latest.id == state.lastSeenMessageID {
                state.lastSuccessfulSyncAt = SyncStateStore.iso8601(now())
                state.nextAllowedSyncAt = nextAllowedForSuccess(mode: conversation.pollingMode, from: now())
                state.consecutiveFailureCount = 0
                state.lastError = nil
                state.lastErrorAt = nil
                state.updatedAt = SyncStateStore.iso8601(now())
                try await stateStore.save(state)
                return successResult(
                    conversation: conversation,
                    state: state,
                    roomID: resolvedRoomID,
                    fetchedMessages: 0,
                    processedMessages: 0,
                    processedEvidence: 0,
                    unchanged: true
                )
            }

            if let latestCreatedDate = parsedDate(latest.created),
               let lastSeenCreatedDate = parsedDate(state.lastSeenCreated),
               latestCreatedDate <= lastSeenCreatedDate,
               try messageAlreadyProcessed(latest.id) {
                state.lastSuccessfulSyncAt = SyncStateStore.iso8601(now())
                state.nextAllowedSyncAt = nextAllowedForSuccess(mode: conversation.pollingMode, from: now())
                state.consecutiveFailureCount = 0
                state.lastError = nil
                state.lastErrorAt = nil
                state.updatedAt = SyncStateStore.iso8601(now())
                try await stateStore.save(state)
                return successResult(
                    conversation: conversation,
                    state: state,
                    roomID: resolvedRoomID,
                    fetchedMessages: 0,
                    processedMessages: 0,
                    processedEvidence: 0,
                    unchanged: true
                )
            }

            let catchup = try await withRequestPermit {
                try await webexClient.fetchRecentMessages(roomID: resolvedRoomID, max: 100)
            }

            let unseen = unseenMessages(
                from: catchup,
                lastSeenMessageID: state.lastSeenMessageID,
                lastSeenCreated: state.lastSeenCreated
            )
            logger.debug("sync_catchup conversation=\(conversation.conversationID, privacy: .public) room=\(resolvedRoomID, privacy: .public) latest_id=\(latest.id, privacy: .public) last_seen_id=\((state.lastSeenMessageID ?? ""), privacy: .public) fetched=\(catchup.count, privacy: .public) unseen=\(unseen.count, privacy: .public)")

            var processedMessages = 0
            var processedEvidence = 0
            var newestHandled: WebexMessage?
            let processingTimestamp = SyncStateStore.iso8601(now())
            for message in unseen {
                let outcome = try messageProcessor.process(
                    message: message,
                    conversation: WebexTrackedConversation(
                        conversationID: conversation.conversationID,
                        conversationType: conversation.conversationType,
                        roomID: resolvedRoomID,
                        personID: conversation.personID,
                        personEmail: conversation.personEmail,
                        displayName: conversation.displayName,
                        pollingMode: conversation.pollingMode
                    ),
                    updatedAt: processingTimestamp
                )
                switch outcome {
                case .processed(let evidenceIndexed):
                    processedMessages += 1
                    processedEvidence += evidenceIndexed
                    newestHandled = message
                case .duplicate, .ignoredSelf:
                    newestHandled = message
                }
            }

            if let newestHandled {
                state.lastSeenMessageID = newestHandled.id
                state.lastSeenCreated = normalizedTimestamp(newestHandled.created, fallback: processingTimestamp)
            } else {
                state.lastSeenMessageID = latest.id
                state.lastSeenCreated = normalizedTimestamp(latest.created, fallback: processingTimestamp)
            }

            state.lastSuccessfulSyncAt = processingTimestamp
            state.nextAllowedSyncAt = nextAllowedForSuccess(mode: conversation.pollingMode, from: now())
            state.pollingMode = conversation.pollingMode
            state.consecutiveFailureCount = 0
            state.lastError = nil
            state.lastErrorAt = nil
            state.updatedAt = processingTimestamp
            try await stateStore.save(state)

            logger.debug("sync_end conversation=\(conversation.conversationID, privacy: .public) room=\(resolvedRoomID, privacy: .public) fetched=\(catchup.count, privacy: .public) processed=\(processedMessages, privacy: .public)")
            logger.debug("sync_latency conversation=\(conversation.conversationID, privacy: .public) room=\(resolvedRoomID, privacy: .public) seconds=\(self.now().timeIntervalSince(syncStartedAt), privacy: .public)")

            return successResult(
                conversation: conversation,
                state: state,
                roomID: resolvedRoomID,
                fetchedMessages: catchup.count,
                processedMessages: processedMessages,
                processedEvidence: processedEvidence,
                unchanged: processedMessages == 0
            )
        } catch {
            let mapped = Self.mapClientError(error)
            logger.error("sync_error conversation=\(conversation.conversationID, privacy: .public) room=\(resolvedRoomID, privacy: .public) error=\(mapped.localizedDescription, privacy: .public)")
            return await failureResult(
                conversation: conversation,
                state: state,
                mappedError: mapped,
                fetchedMessages: 0,
                processedMessages: 0,
                processedEvidence: 0,
                trigger: trigger
            )
        }
    }

    /// Resolves direct-message room IDs lazily and persists the discovered value.
    private func resolveRoomID(
        conversation: WebexTrackedConversation,
        state: inout WebexConversationSyncStateRecord
    ) async throws -> String {
        if let existing = normalizedOptional(state.roomID), !existing.isEmpty {
            return existing
        }
        if let configured = normalizedOptional(conversation.roomID), !configured.isEmpty {
            state.roomID = configured
            state.updatedAt = SyncStateStore.iso8601(now())
            try await stateStore.save(state)
            return configured
        }
        guard conversation.conversationType == .direct else {
            return ""
        }
        let directMessages = try await withRequestPermit {
            try await webexClient.fetchDirectMessages(
                personEmail: normalizedOptional(conversation.personEmail),
                personID: normalizedOptional(conversation.personID),
                max: 100
            )
        }
        guard let resolved = directMessages.first(where: { !normalizedRoomID($0.roomID).isEmpty })?.roomID else {
            return ""
        }
        let roomID = normalizedRoomID(resolved)
        state.roomID = roomID
        state.updatedAt = SyncStateStore.iso8601(now())
        try await stateStore.save(state)
        return roomID
    }

    /// Returns messages newer than the stored ID/timestamp watermark.
    private func unseenMessages(
        from messages: [WebexMessage],
        lastSeenMessageID: String?,
        lastSeenCreated: String?
    ) -> [WebexMessage] {
        let ascending = messages.sorted { lhs, rhs in
            let lhsDate = parsedDate(lhs.created) ?? Date.distantPast
            let rhsDate = parsedDate(rhs.created) ?? Date.distantPast
            if lhsDate == rhsDate {
                return lhs.id < rhs.id
            }
            return lhsDate < rhsDate
        }

        let normalizedLastSeenID = normalizedOptional(lastSeenMessageID) ?? ""
        if !normalizedLastSeenID.isEmpty,
           let lastSeenIndex = ascending.lastIndex(where: { $0.id == normalizedLastSeenID }) {
            let unseenSlice = ascending[(lastSeenIndex + 1)...]
            return Array(unseenSlice)
        }

        if let lastSeenCreatedDate = parsedDate(lastSeenCreated) {
            return ascending.filter { message in
                guard let messageDate = parsedDate(message.created) else {
                    return false
                }
                return messageDate > lastSeenCreatedDate
            }
        }
        return ascending
    }

    private func successResult(
        conversation: WebexTrackedConversation,
        state: WebexConversationSyncStateRecord,
        roomID: String,
        fetchedMessages: Int,
        processedMessages: Int,
        processedEvidence: Int,
        unchanged: Bool
    ) -> WebexConversationSyncResult {
        WebexConversationSyncResult(
            conversationID: conversation.conversationID,
            roomID: roomID,
            displayName: conversation.displayName,
            fetchedMessages: fetchedMessages,
            processedMessages: processedMessages,
            processedEvidence: processedEvidence,
            latestMessageID: state.lastSeenMessageID,
            latestMessageCreated: state.lastSeenCreated,
            skippedReason: nil,
            unchanged: unchanged,
            status: .synced
        )
    }

    /// Persists failure backoff when state is available and returns a skipped result.
    private func failureResult(
        conversation: WebexTrackedConversation,
        state: WebexConversationSyncStateRecord?,
        mappedError: WebexSyncClientError,
        fetchedMessages: Int,
        processedMessages: Int,
        processedEvidence: Int,
        trigger: WebexSyncTriggerReason
    ) async -> WebexConversationSyncResult {
        var latestMessageID = state?.lastSeenMessageID
        var latestMessageCreated = state?.lastSeenCreated
        if var state {
            state.consecutiveFailureCount += 1
            state.lastError = mappedError.localizedDescription
            state.lastErrorAt = SyncStateStore.iso8601(now())
            state.nextAllowedSyncAt = nextAllowedForFailure(
                failureCount: state.consecutiveFailureCount,
                mappedError: mappedError,
                mode: conversation.pollingMode,
                trigger: trigger
            )
            state.updatedAt = SyncStateStore.iso8601(now())
            try? await stateStore.save(state)
            logger.error("sync_backoff conversation=\(conversation.conversationID, privacy: .public) room=\(conversation.roomID, privacy: .public) error=\(mappedError.localizedDescription, privacy: .public) failures=\(state.consecutiveFailureCount, privacy: .public) next_allowed=\((state.nextAllowedSyncAt ?? ""), privacy: .public)")
            latestMessageID = state.lastSeenMessageID
            latestMessageCreated = state.lastSeenCreated
        }

        return WebexConversationSyncResult(
            conversationID: conversation.conversationID,
            roomID: conversation.roomID,
            displayName: conversation.displayName,
            fetchedMessages: fetchedMessages,
            processedMessages: processedMessages,
            processedEvidence: processedEvidence,
            latestMessageID: latestMessageID,
            latestMessageCreated: latestMessageCreated,
            skippedReason: mappedError.localizedDescription,
            unchanged: true,
            status: status(for: mappedError)
        )
    }

    private func status(for error: WebexSyncClientError) -> WebexSyncStatusIndicator {
        switch error {
        case .unauthorized, .forbidden:
            return .authRequired
        case .rateLimited:
            return .delayedRateLimit
        case .transientServerError, .networkError, .decodeError, .notFound, .unknown:
            return .offline
        }
    }

    private func nextAllowedForSuccess(mode: WebexPollingMode, from date: Date) -> String {
        let base: TimeInterval
        switch mode {
        case .active:
            base = configuration.activeIntervalSeconds
        case .recent:
            base = configuration.recentIntervalSeconds
        case .background:
            base = configuration.backgroundIntervalSeconds
        case .paused, .disabled:
            base = configuration.backgroundIntervalSeconds
        }
        return SyncStateStore.iso8601(date.addingTimeInterval(jittered(base)))
    }

    /// Computes retry/backoff time for rate limits and transient failures.
    private func nextAllowedForFailure(
        failureCount: Int,
        mappedError: WebexSyncClientError,
        mode: WebexPollingMode,
        trigger: WebexSyncTriggerReason
    ) -> String {
        let nowDate = now()
        if case .rateLimited(let retryAfterSeconds) = mappedError {
            return SyncStateStore.iso8601(nowDate.addingTimeInterval(TimeInterval(max(1, retryAfterSeconds))))
        }
        let schedule: [TimeInterval] = [30, 60, 120, 300, 600]
        let index = min(max(failureCount - 1, 0), schedule.count - 1)
        var delay = jittered(schedule[index])
        if (mode == .paused || mode == .disabled), trigger != .manual {
            delay = max(delay, configuration.backgroundIntervalSeconds)
        }
        return SyncStateStore.iso8601(nowDate.addingTimeInterval(delay))
    }

    private func jittered(_ baseSeconds: TimeInterval) -> TimeInterval {
        let clampedBase = max(1, baseSeconds)
        let ratio = min(max(0, configuration.jitterRatio), 0.9)
        guard ratio > 0 else { return clampedBase }
        let jitterRange = clampedBase * ratio
        let minValue = clampedBase - jitterRange
        let maxValue = clampedBase + jitterRange
        let unit = min(max(randomUnitInterval(), 0), 1)
        return minValue + (maxValue - minValue) * unit
    }

    /// Runs one Webex request under the global sync-engine concurrency limit.
    private func withRequestPermit<T>(_ operation: () async throws -> T) async throws -> T {
        await requestPermits.acquire()
        do {
            let result = try await operation()
            await requestPermits.release()
            return result
        } catch {
            await requestPermits.release()
            throw error
        }
    }

    private func messageAlreadyProcessed(_ messageID: String) throws -> Bool {
        try messageProcessor.messageExists(messageID: messageID)
    }

    private func normalizedRoomID(_ value: String) -> String {
        value.trimmingCharacters(in: .whitespacesAndNewlines)
    }

    private func normalizedOptional(_ value: String?) -> String? {
        guard let value = value?.trimmingCharacters(in: .whitespacesAndNewlines),
              !value.isEmpty else {
            return nil
        }
        return value
    }

    private func normalizedTimestamp(_ value: String, fallback: String) -> String {
        let trimmed = value.trimmingCharacters(in: .whitespacesAndNewlines)
        return trimmed.isEmpty ? fallback : trimmed
    }

    private func parsedDate(_ value: String?) -> Date? {
        guard let value = value?.trimmingCharacters(in: .whitespacesAndNewlines),
              !value.isEmpty else {
            return nil
        }
        if let parsed = Self.iso8601WithFractionalSeconds.date(from: value) {
            return parsed
        }
        return Self.iso8601.date(from: value)
    }

    /// Maps transport/API errors to sync statuses and retry behavior.
    static func mapClientError(_ error: Error) -> WebexSyncClientError {
        if let mapped = error as? WebexSyncClientError {
            return mapped
        }
        if let apiError = error as? WebexAPIError {
            switch apiError {
            case .missingAccessToken, .invalidAccessToken:
                return .unauthorized
            case .httpStatus(let code, _, let retryAfterSeconds):
                switch code {
                case 401:
                    return .unauthorized
                case 403:
                    return .forbidden
                case 404:
                    return .notFound
                case 429:
                    return .rateLimited(retryAfterSeconds: Int((retryAfterSeconds ?? 60).rounded(.up)))
                case 500...599:
                    return .transientServerError
                default:
                    return .unknown(apiError.localizedDescription)
                }
            case .unexpectedResponse:
                return .decodeError
            case .network:
                return .networkError
            case .invalidBaseURL, .invalidHTTPResponse:
                return .unknown(apiError.localizedDescription)
            case .exhaustedRetries:
                return .transientServerError
            }
        }
        if error is DecodingError {
            return .decodeError
        }
        if let urlError = error as? URLError {
            switch urlError.code {
            case .notConnectedToInternet,
                 .networkConnectionLost,
                 .timedOut,
                 .cannotFindHost,
                 .cannotConnectToHost:
                return .networkError
            default:
                return .unknown(urlError.localizedDescription)
            }
        }
        return .unknown(error.localizedDescription)
    }

    private static let iso8601WithFractionalSeconds: ISO8601DateFormatter = {
        let formatter = ISO8601DateFormatter()
        formatter.formatOptions = [.withInternetDateTime, .withFractionalSeconds]
        return formatter
    }()

    private static let iso8601: ISO8601DateFormatter = {
        let formatter = ISO8601DateFormatter()
        formatter.formatOptions = [.withInternetDateTime]
        return formatter
    }()
}
