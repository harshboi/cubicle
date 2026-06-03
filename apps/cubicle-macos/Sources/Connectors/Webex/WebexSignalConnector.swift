import Foundation

/// Signal adapter for Webex room and direct-message conversations.
final class WebexSignalConnector: SignalConnector {
    let descriptor = ConnectorDescriptor(
        id: .webex,
        displayName: "Webex",
        capabilities: [.messages, .mapRefresh]
    )

    private let webexClient: WebexClienting
    private let accountID: String
    private let engineConfiguration: WebexSyncEngine.Configuration
    private let selfPersonID: String?
    private let selfEmail: String?
    private let existingMessageLookup: (String) throws -> Bool
    private let legacyStateLookup: WebexSignalSyncStateStore.LegacyStateLookup?
    private let now: () -> Date
    private let randomUnitInterval: () -> Double

    init(
        webexClient: WebexClienting,
        accountID: String = "default",
        engineConfiguration: WebexSyncEngine.Configuration = WebexSyncEngine.Configuration(),
        selfPersonID: String? = nil,
        selfEmail: String? = nil,
        existingMessageLookup: @escaping (String) throws -> Bool = { _ in false },
        legacyStateLookup: WebexSignalSyncStateStore.LegacyStateLookup? = nil,
        now: @escaping () -> Date = Date.init,
        randomUnitInterval: @escaping () -> Double = { Double.random(in: 0...1) }
    ) {
        self.webexClient = webexClient
        self.accountID = accountID
        self.engineConfiguration = engineConfiguration
        self.selfPersonID = selfPersonID
        self.selfEmail = selfEmail
        self.existingMessageLookup = existingMessageLookup
        self.legacyStateLookup = legacyStateLookup
        self.now = now
        self.randomUnitInterval = randomUnitInterval
    }

    func sync(
        request: SignalSyncRequest,
        checkpoints: ConnectorCheckpointSet
    ) async throws -> SignalSyncBatch {
        let conversations = webexConversations(from: request)
        guard !conversations.isEmpty else {
            return .empty(connectorID: .webex, accountID: accountID)
        }

        let stateStore = WebexSignalSyncStateStore(
            accountID: accountID,
            checkpoints: checkpoints,
            legacyStateLookup: legacyStateLookup,
            now: now
        )
        let processor = WebexSignalBatchMessageProcessor(
            accountID: accountID,
            ignoreSelfMessages: true,
            selfPersonID: selfPersonID,
            selfEmail: selfEmail,
            messageExists: existingMessageLookup
        )
        let engine = WebexSyncEngine(
            webexClient: webexClient,
            stateStore: stateStore,
            messageProcessor: processor,
            configuration: engineConfiguration,
            now: now,
            randomUnitInterval: randomUnitInterval
        )
        let results = await engine.syncConversations(
            conversations,
            trigger: webexTrigger(from: request.trigger)
        )
        let checkpoints = await stateStore.pendingCheckpoints()
        let warnings = connectorWarnings(from: results)
        return processor.makeBatch(
            checkpoints: checkpoints,
            warnings: warnings,
            availability: availability(results: results, warnings: warnings)
        )
    }

    private func webexConversations(from request: SignalSyncRequest) -> [WebexTrackedConversation] {
        var conversations: [WebexTrackedConversation] = []
        for target in request.targets {
            let roomIDs = target.selectors(for: .webex, kind: .roomID).map(\.value).filter { !$0.isEmpty }
            let emails = target.selectors(for: .webex, kind: .email).map(\.value).filter { !$0.isEmpty }
            for roomID in roomIDs {
                let conversationType: WebexConversationType = target.entityKind == .person ? .direct : .space
                conversations.append(
                    WebexTrackedConversation(
                        conversationID: "room:\(roomID)",
                        conversationType: conversationType,
                        roomID: roomID,
                        personID: nil,
                        personEmail: emails.first,
                        displayName: target.label,
                        pollingMode: pollingMode(for: request.mode, conversationType: conversationType)
                    )
                )
            }
            guard roomIDs.isEmpty else {
                continue
            }
            for email in emails {
                conversations.append(
                    WebexTrackedConversation(
                        conversationID: "person:\(email)",
                        conversationType: .direct,
                        roomID: "",
                        personID: nil,
                        personEmail: email,
                        displayName: target.label.isEmpty ? email : target.label,
                        pollingMode: pollingMode(for: request.mode, conversationType: .direct)
                    )
                )
            }
        }
        return conversations.sorted { lhs, rhs in
            if lhs.displayName.localizedCaseInsensitiveCompare(rhs.displayName) != .orderedSame {
                return lhs.displayName.localizedCaseInsensitiveCompare(rhs.displayName) == .orderedAscending
            }
            return lhs.conversationID < rhs.conversationID
        }
    }

    private func pollingMode(
        for mode: SignalSyncMode,
        conversationType: WebexConversationType
    ) -> WebexPollingMode {
        switch mode {
        case .full:
            return .active
        case .incremental:
            return conversationType == .direct ? .recent : .background
        }
    }

    private func webexTrigger(from trigger: SignalSyncTriggerReason) -> WebexSyncTriggerReason {
        switch trigger {
        case .startup:
            return .startup
        case .scheduled:
            return .scheduled
        case .manual:
            return .manual
        case .wakeFromSleep:
            return .wakeFromSleep
        case .networkReconnect:
            return .networkReconnect
        case .userOpenedTarget:
            return .userOpenedConversation
        }
    }

    private func connectorWarnings(from results: [WebexConversationSyncResult]) -> [ConnectorWarning] {
        results.compactMap { result in
            guard let reason = result.skippedReason, result.status != .synced else {
                return nil
            }
            return ConnectorWarning(
                connectorID: .webex,
                targetID: result.conversationID,
                message: reason
            )
        }
    }

    private func availability(
        results: [WebexConversationSyncResult],
        warnings: [ConnectorWarning]
    ) -> ConnectorAvailability {
        if results.contains(where: { $0.status == .authRequired }) {
            return .authRequired
        }
        if results.contains(where: { $0.status == .delayedRateLimit }) {
            return .rateLimited
        }
        guard !warnings.isEmpty else {
            return .available
        }
        return results.contains(where: { $0.processedMessages > 0 }) ? .partial : .unavailable
    }
}
