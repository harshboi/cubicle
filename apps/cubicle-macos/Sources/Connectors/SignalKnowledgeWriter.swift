import Foundation

struct SignalWriteSummary: Hashable {
    var messageEventsProcessed: Int
    var evidenceRecordsWritten: Int
}

protocol SignalKnowledgeWriting {
    func write(_ batch: SignalSyncBatch) throws -> SignalWriteSummary
}

final class SignalKnowledgeWriter: SignalKnowledgeWriting {
    private let knowledgeStore: KnowledgeStore
    private let now: () -> Date

    init(
        knowledgeStore: KnowledgeStore,
        now: @escaping () -> Date = Date.init
    ) {
        self.knowledgeStore = knowledgeStore
        self.now = now
    }

    func write(_ batch: SignalSyncBatch) throws -> SignalWriteSummary {
        try knowledgeStore.bootstrap()
        let updatedAt = Self.iso8601(now())
        try writeObjects(batch.objects, fallbackUpdatedAt: updatedAt)

        var messageEventsProcessed = 0
        var evidenceRecordsWritten = 0
        for event in batch.events {
            guard case .message(let payload) = event.payload else { continue }
            let wroteEvidence = try writeMessageEvent(
                event,
                payload: payload,
                batch: batch,
                fallbackUpdatedAt: updatedAt
            )
            messageEventsProcessed += 1
            evidenceRecordsWritten += wroteEvidence ? 1 : 0
        }

        return SignalWriteSummary(
            messageEventsProcessed: messageEventsProcessed,
            evidenceRecordsWritten: evidenceRecordsWritten
        )
    }

    private func writeObjects(
        _ objects: [SignalObject],
        fallbackUpdatedAt: String
    ) throws {
        for object in objects {
            let updatedAt = object.updatedAt.map(Self.iso8601) ?? fallbackUpdatedAt
            switch object.kind {
            case .person:
                try knowledgeStore.upsertPerson(
                    PersonRecord(
                        id: object.id.rawValue,
                        displayName: normalizedText(object.title, fallback: object.id.rawValue),
                        email: object.emailProperty,
                        updatedAt: updatedAt
                    )
                )
            case .space, .thread, .channel:
                try knowledgeStore.upsertRoom(
                    RoomRecord(
                        id: object.sourceID.externalID,
                        title: normalizedText(object.title, fallback: object.sourceID.externalID),
                        updatedAt: updatedAt
                    )
                )
            case .document, .file, .issue, .pullRequest, .project:
                continue
            }
        }
    }

    @discardableResult
    private func writeMessageEvent(
        _ event: SignalEvent,
        payload: MessageEventPayload,
        batch: SignalSyncBatch,
        fallbackUpdatedAt: String
    ) throws -> Bool {
        let createdAt = Self.iso8601(event.occurredAt)
        let roomID = payload.threadSourceID.externalID
        let roomTitle = normalizedText(payload.threadTitle, fallback: roomID)
        try knowledgeStore.upsertRoom(
            RoomRecord(
                id: roomID,
                title: roomTitle,
                updatedAt: createdAt
            )
        )

        let personID = payload.senderID?.rawValue
        if let personID, !personID.isEmpty {
            try knowledgeStore.upsertPerson(
                PersonRecord(
                    id: personID,
                    displayName: normalizedText(payload.senderDisplayName, fallback: personID),
                    email: payload.senderEmail ?? "",
                    updatedAt: fallbackUpdatedAt
                )
            )
        }

        let body = normalizedMessageText(payload.body)
        try knowledgeStore.upsertMessage(
            MessageRecord(
                id: event.id.rawValue,
                roomID: roomID,
                personID: personID,
                body: body,
                createdAt: createdAt,
                updatedAt: fallbackUpdatedAt
            )
        )

        guard !body.isEmpty else {
            return false
        }

        let evidenceSource = "\(batch.connectorID.rawValue)_message"
        try knowledgeStore.upsertBeliefEvidence(
            BeliefEvidenceRecord(
                id: "\(evidenceSource)-\(event.id.rawValue)",
                source: evidenceSource,
                sourceID: event.id.rawValue,
                roomID: roomID,
                personID: personID,
                occurredAt: createdAt,
                text: body
            )
        )
        return true
    }

    private func normalizedText(_ value: String, fallback: String) -> String {
        let normalized = value.trimmingCharacters(in: .whitespacesAndNewlines)
        return normalized.isEmpty ? fallback : normalized
    }

    private func normalizedMessageText(_ value: String) -> String {
        value
            .replacingOccurrences(of: "\u{00a0}", with: " ")
            .trimmingCharacters(in: .whitespacesAndNewlines)
    }

    private static func iso8601(_ date: Date) -> String {
        iso8601WithFractionalSeconds.string(from: date)
    }

    private static let iso8601WithFractionalSeconds: ISO8601DateFormatter = {
        let formatter = ISO8601DateFormatter()
        formatter.formatOptions = [.withInternetDateTime, .withFractionalSeconds]
        return formatter
    }()
}

private extension SignalObject {
    var emailProperty: String {
        switch properties["webex.email"] ?? properties["imessage.email"] {
        case .string(let value):
            return value.trimmingCharacters(in: .whitespacesAndNewlines).lowercased()
        case .number, .bool, .none:
            return ""
        }
    }
}
