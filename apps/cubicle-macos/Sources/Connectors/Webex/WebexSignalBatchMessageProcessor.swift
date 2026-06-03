import Foundation

/// Webex engine processor that accumulates normalized signal rows.
final class WebexSignalBatchMessageProcessor: MessageProcessing {
    typealias MessageExists = (String) throws -> Bool

    private let accountID: String
    private let ignoreSelfMessages: Bool
    private let selfPersonID: String?
    private let selfEmail: String?
    private let existingMessageLookup: MessageExists
    private let lock = NSLock()
    private var objects: [SignalObject] = []
    private var events: [SignalEvent] = []
    private var seenObjectIDs: Set<GlobalSignalID> = []
    private var seenEventIDs: Set<GlobalSignalID> = []

    init(
        accountID: String,
        ignoreSelfMessages: Bool = true,
        selfPersonID: String? = nil,
        selfEmail: String? = nil,
        messageExists: @escaping MessageExists = { _ in false }
    ) {
        self.accountID = accountID
        self.ignoreSelfMessages = ignoreSelfMessages
        self.selfPersonID = selfPersonID?.trimmingCharacters(in: .whitespacesAndNewlines)
        self.selfEmail = selfEmail?.trimmingCharacters(in: .whitespacesAndNewlines).lowercased()
        self.existingMessageLookup = messageExists
    }

    func process(
        message: WebexMessage,
        conversation: WebexTrackedConversation,
        updatedAt: String
    ) throws -> MessageProcessOutcome {
        if ignoreSelfMessage(message) {
            return .ignoredSelf
        }
        if try messageExists(messageID: message.id) {
            return .duplicate
        }

        let converted = makeMessageSignal(message, conversation: conversation)
        lock.lock()
        defer { lock.unlock() }
        for object in converted.objects where seenObjectIDs.insert(object.id).inserted {
            objects.append(object)
        }
        if seenEventIDs.insert(converted.event.id).inserted {
            events.append(converted.event)
            let evidenceIndexed = converted.bodyIsEmpty ? 0 : 1
            return .processed(evidenceIndexed: evidenceIndexed)
        }
        return .duplicate
    }

    func messageExists(messageID: String) throws -> Bool {
        let sourceID = SourceEventID(
            connectorID: .webex,
            accountID: accountID,
            kind: "message",
            externalID: messageID
        )
        lock.lock()
        let alreadySeen = seenEventIDs.contains(sourceID.globalID)
        lock.unlock()
        if alreadySeen {
            return true
        }
        if try existingMessageLookup(messageID) {
            return true
        }
        return try existingMessageLookup(sourceID.globalID.rawValue)
    }

    func makeBatch(
        checkpoints: [ConnectorCheckpoint],
        warnings: [ConnectorWarning],
        availability: ConnectorAvailability
    ) -> SignalSyncBatch {
        lock.lock()
        let batchObjects = objects
        let batchEvents = events
        lock.unlock()
        return SignalSyncBatch(
            connectorID: .webex,
            accountID: accountID,
            objects: batchObjects,
            events: batchEvents,
            relations: [],
            content: [],
            checkpoint: nil,
            checkpoints: checkpoints,
            warnings: warnings,
            availability: availability
        )
    }

    private func ignoreSelfMessage(_ message: WebexMessage) -> Bool {
        guard ignoreSelfMessages else {
            return false
        }
        let authorPersonID = message.personID.trimmingCharacters(in: .whitespacesAndNewlines)
        let authorEmail = message.personEmail.trimmingCharacters(in: .whitespacesAndNewlines).lowercased()
        return (!authorPersonID.isEmpty && authorPersonID == selfPersonID)
            || (!authorEmail.isEmpty && authorEmail == selfEmail)
    }

    private func makeMessageSignal(
        _ message: WebexMessage,
        conversation: WebexTrackedConversation
    ) -> (objects: [SignalObject], event: SignalEvent, bodyIsEmpty: Bool) {
        let roomID = normalized(message.roomID).isEmpty ? normalized(conversation.roomID) : normalized(message.roomID)
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
        let threadTitle = normalized(conversation.displayName).isEmpty ? roomID : normalized(conversation.displayName)
        let roomObject = SignalObject(
            id: roomSourceID.globalID,
            sourceID: roomSourceID,
            kind: conversation.conversationType == .space ? .space : .thread,
            title: threadTitle,
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
            threadTitle: threadTitle,
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
            actor: SignalActor(
                id: personSourceID.globalID,
                displayName: senderDisplayName,
                email: senderEmail.isEmpty ? nil : senderEmail
            ),
            occurredAt: occurredAt,
            objectIDs: [roomSourceID.globalID, personSourceID.globalID],
            visibility: visibility,
            payload: .message(payload)
        )
        return ([roomObject, personObject], event, normalizedMessageText(message.text).isEmpty)
    }

    private func normalized(_ value: String) -> String {
        value.trimmingCharacters(in: .whitespacesAndNewlines)
    }

    private func normalizedMessageText(_ value: String) -> String {
        value
            .replacingOccurrences(of: "\u{00a0}", with: " ")
            .trimmingCharacters(in: .whitespacesAndNewlines)
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
