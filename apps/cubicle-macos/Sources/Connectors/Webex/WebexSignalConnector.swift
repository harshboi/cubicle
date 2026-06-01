import Foundation

final class WebexSignalConnector: SignalConnector {
    let descriptor = ConnectorDescriptor(
        id: .webex,
        displayName: "Webex",
        capabilities: [.messages, .mapRefresh]
    )

    private let webexClient: WebexClienting
    private let accountID: String

    init(webexClient: WebexClienting, accountID: String = "default") {
        self.webexClient = webexClient
        self.accountID = accountID
    }

    func sync(
        request: SignalSyncRequest,
        checkpoint: ConnectorCheckpoint?
    ) async throws -> SignalSyncBatch {
        _ = checkpoint
        var objects: [SignalObject] = []
        var events: [SignalEvent] = []
        var warnings: [ConnectorWarning] = []
        var seenObjectIDs: Set<GlobalSignalID> = []
        var seenEventIDs: Set<GlobalSignalID> = []

        for target in request.targets {
            let roomIDs = target.selectors(for: .webex, kind: .roomID).map(\.value).filter { !$0.isEmpty }
            let emails = target.selectors(for: .webex, kind: .email).map(\.value).filter { !$0.isEmpty }

            for roomID in roomIDs {
                do {
                    let messages = try await webexClient.fetchRecentMessages(roomID: roomID, max: request.limit)
                    appendSignals(
                        from: messages,
                        target: target,
                        fallbackRoomID: roomID,
                        objects: &objects,
                        events: &events,
                        seenObjectIDs: &seenObjectIDs,
                        seenEventIDs: &seenEventIDs
                    )
                } catch {
                    warnings.append(ConnectorWarning(connectorID: .webex, targetID: target.id, message: error.localizedDescription))
                }
            }

            for email in emails where roomIDs.isEmpty {
                // Prefer configured room IDs when present; direct lookup is only
                // for email-only targets and can duplicate the same conversation.
                do {
                    let messages = try await webexClient.fetchDirectMessages(personEmail: email, personID: nil, max: request.limit)
                    appendSignals(
                        from: messages,
                        target: target,
                        fallbackRoomID: "direct:\(email)",
                        objects: &objects,
                        events: &events,
                        seenObjectIDs: &seenObjectIDs,
                        seenEventIDs: &seenEventIDs
                    )
                } catch {
                    warnings.append(ConnectorWarning(connectorID: .webex, targetID: target.id, message: error.localizedDescription))
                }
            }
        }

        return SignalSyncBatch(
            connectorID: .webex,
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

    private func appendSignals(
        from messages: [WebexMessage],
        target: SignalTarget,
        fallbackRoomID: String,
        objects: inout [SignalObject],
        events: inout [SignalEvent],
        seenObjectIDs: inout Set<GlobalSignalID>,
        seenEventIDs: inout Set<GlobalSignalID>
    ) {
        for message in messages {
            let converted = makeMessageSignal(message, target: target, fallbackRoomID: fallbackRoomID)
            for object in converted.objects where seenObjectIDs.insert(object.id).inserted {
                objects.append(object)
            }
            if seenEventIDs.insert(converted.event.id).inserted {
                events.append(converted.event)
            }
        }
    }

    private func makeMessageSignal(
        _ message: WebexMessage,
        target: SignalTarget,
        fallbackRoomID: String
    ) -> (objects: [SignalObject], event: SignalEvent) {
        let roomID = normalized(message.roomID).isEmpty ? fallbackRoomID : normalized(message.roomID)
        let roomSourceID = SourceObjectID(
            connectorID: .webex,
            accountID: accountID,
            kind: "room",
            externalID: roomID
        )
        let personExternalID = normalized(message.personID).isEmpty
            ? normalized(message.personEmail).lowercased()
            : normalized(message.personID)
        let personSourceID = SourceObjectID(
            connectorID: .webex,
            accountID: accountID,
            kind: "person",
            externalID: personExternalID
        )
        let eventSourceID = SourceEventID(
            connectorID: .webex,
            accountID: accountID,
            kind: "message",
            externalID: message.id
        )
        let occurredAt = Self.parseDate(message.created) ?? Date()
        let visibility = SignalVisibility.authenticatedUser(connectorID: .webex, accountID: accountID)
        let senderEmail = normalized(message.personEmail).lowercased()
        let senderDisplayName = senderEmail.isEmpty ? personExternalID : senderEmail
        let roomObject = SignalObject(
            id: roomSourceID.globalID,
            sourceID: roomSourceID,
            kind: target.entityKind == .space ? .space : .thread,
            title: target.label,
            url: nil,
            createdAt: nil,
            updatedAt: occurredAt,
            visibility: visibility,
            properties: ["webex.roomID": .string(roomID)]
        )
        let personObject = SignalObject(
            id: personSourceID.globalID,
            sourceID: personSourceID,
            kind: .person,
            title: senderDisplayName,
            url: nil,
            createdAt: nil,
            updatedAt: occurredAt,
            visibility: visibility,
            properties: senderEmail.isEmpty ? [:] : ["webex.email": .string(senderEmail)]
        )
        let payload = MessageEventPayload(
            threadID: roomSourceID.globalID,
            threadSourceID: roomSourceID,
            threadTitle: target.label,
            senderID: personSourceID.globalID,
            senderDisplayName: senderDisplayName,
            senderEmail: senderEmail.isEmpty ? nil : senderEmail,
            body: message.text,
            isFromCurrentUser: false
        )
        let event = SignalEvent(
            id: eventSourceID.globalID,
            sourceID: eventSourceID,
            kind: .message,
            actor: SignalActor(id: personSourceID.globalID, displayName: senderDisplayName, email: senderEmail.isEmpty ? nil : senderEmail),
            occurredAt: occurredAt,
            objectIDs: [roomSourceID.globalID, personSourceID.globalID],
            visibility: visibility,
            payload: .message(payload)
        )
        return ([roomObject, personObject], event)
    }

    private func normalized(_ value: String) -> String {
        value.trimmingCharacters(in: .whitespacesAndNewlines)
    }

    private func availability(hasSignals: Bool, warnings: [ConnectorWarning]) -> ConnectorAvailability {
        guard !warnings.isEmpty else { return .available }
        return hasSignals ? .partial : .unavailable
    }

    private static func parseDate(_ value: String) -> Date? {
        if let date = iso8601WithFractionalSeconds.date(from: value) {
            return date
        }
        return iso8601.date(from: value)
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
