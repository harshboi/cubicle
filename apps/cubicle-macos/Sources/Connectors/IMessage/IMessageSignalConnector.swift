import Foundation

/// Signal adapter for the local macOS Messages database.
final class IMessageSignalConnector: SignalConnector {
    let descriptor = ConnectorDescriptor(
        id: .iMessage,
        displayName: "iMessage",
        capabilities: [.messages]
    )

    private let ingestionService: NativeIMessageIngesting
    private let accountID: String

    /// Injects the database reader so tests avoid touching chat.db/TCC.
    init(
        ingestionService: NativeIMessageIngesting = NativeIMessageIngestionService(),
        accountID: String = "local"
    ) {
        self.ingestionService = ingestionService
        self.accountID = accountID
    }

    /// Converts matching iMessage rows into thread objects and message events.
    func sync(
        request: SignalSyncRequest,
        checkpoints: ConnectorCheckpointSet
    ) async throws -> SignalSyncBatch {
        // chat.db has no remote cursor contract; request.since is the replay
        // boundary until we add a local checkpoint payload.
        _ = checkpoints
        let since = request.since ?? Date.distantPast
        var objects: [SignalObject] = []
        var events: [SignalEvent] = []
        var warnings: [ConnectorWarning] = []
        var seenEventIDs: Set<GlobalSignalID> = []
        var seenObjectIDs: Set<GlobalSignalID> = []

        for target in request.targets {
            let handles = target
                .selectors(for: .iMessage, kind: .handle)
                .map(\.value)
                .filter { !$0.isEmpty }
            guard !handles.isEmpty else { continue }

            do {
                let messages = try ingestionService.loadMessages(
                    matching: handles,
                    displayName: target.label,
                    since: since,
                    limit: request.limit
                )
                for message in messages {
                    let converted = makeMessageSignal(message, fallbackTitle: target.label)
                    if seenObjectIDs.insert(converted.threadObject.id).inserted {
                        objects.append(converted.threadObject)
                    }
                    if seenEventIDs.insert(converted.event.id).inserted {
                        events.append(converted.event)
                    }
                }
            } catch {
                // TCC/db-denied reads should degrade this source only; Webex
                // and other connectors can still produce a useful batch.
                warnings.append(
                    ConnectorWarning(
                        connectorID: .iMessage,
                        targetID: target.id,
                        message: error.localizedDescription
                    )
                )
            }
        }

        return SignalSyncBatch(
            connectorID: .iMessage,
            accountID: accountID,
            objects: objects,
            events: events,
            relations: [],
            content: [],
            checkpoint: nil,
            warnings: warnings,
            availability: availability(hasSignals: !events.isEmpty || !objects.isEmpty, warnings: warnings)
        )
    }

    /// Builds the normalized thread/event pair for one Messages row.
    private func makeMessageSignal(
        _ message: IMessageTimelineMessage,
        fallbackTitle: String
    ) -> (threadObject: SignalObject, event: SignalEvent) {
        let threadExternalID = message.threadID.trimmingCharacters(in: .whitespacesAndNewlines).isEmpty
            ? "thread:\(message.handle)"
            : message.threadID
        let threadSourceID = SourceObjectID(
            connectorID: .iMessage,
            accountID: accountID,
            kind: "thread",
            externalID: threadExternalID
        )
        let eventSourceID = SourceEventID(
            connectorID: .iMessage,
            accountID: accountID,
            kind: "message",
            externalID: message.id
        )
        let actorID = SourceObjectID(
            connectorID: .iMessage,
            accountID: accountID,
            kind: "person",
            externalID: message.isFromMe ? "me" : message.handle
        ).globalID
        let title = message.threadTitle.trimmingCharacters(in: .whitespacesAndNewlines).isEmpty
            ? fallbackTitle
            : message.threadTitle
        let visibility = SignalVisibility.localUserOnly
        let actor = SignalActor(
            id: actorID,
            displayName: message.sender,
            email: message.handle.contains("@") ? message.handle.lowercased() : nil
        )
        let threadObject = SignalObject(
            id: threadSourceID.globalID,
            sourceID: threadSourceID,
            kind: .thread,
            title: title,
            url: nil,
            createdAt: nil,
            updatedAt: message.sortDate,
            visibility: visibility,
            properties: [
                "imessage.handle": .string(message.handle)
            ]
        )
        let payload = MessageEventPayload(
            threadID: threadSourceID.globalID,
            threadSourceID: threadSourceID,
            threadTitle: title,
            senderID: actorID,
            senderDisplayName: message.sender,
            senderEmail: message.handle.contains("@") ? message.handle.lowercased() : nil,
            body: message.body,
            isFromCurrentUser: message.isFromMe
        )
        let event = SignalEvent(
            id: eventSourceID.globalID,
            sourceID: eventSourceID,
            kind: .message,
            actor: actor,
            occurredAt: message.sortDate,
            objectIDs: [threadSourceID.globalID],
            visibility: visibility,
            payload: .message(payload)
        )
        return (threadObject, event)
    }

    /// Distinguishes partial local-db failures from a clean empty sync.
    private func availability(hasSignals: Bool, warnings: [ConnectorWarning]) -> ConnectorAvailability {
        guard !warnings.isEmpty else { return .available }
        return hasSignals ? .partial : .unavailable
    }
}
