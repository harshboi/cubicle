import Foundation

/// Counts the legacy knowledge rows touched by one signal batch write.
struct SignalWriteSummary: Hashable {
    var messageEventsProcessed: Int
    var evidenceRecordsWritten: Int
}

/// Persistence boundary for normalized connector output.
protocol SignalKnowledgeWriting {
    /// Writes a connector batch into the current knowledge store schema.
    func write(_ batch: SignalSyncBatch) throws -> SignalWriteSummary
}

/// Storage calls needed by the signal writer.
protocol SignalKnowledgeWritableStore {
    func bootstrap() throws
    func writeConnectorMessageBatch(
        rooms: [RoomRecord],
        people: [PersonRecord],
        messages: [MessageRecord],
        evidence: [BeliefEvidenceRecord]
    ) throws
}

extension KnowledgeStore: SignalKnowledgeWritableStore {}

private struct SignalKnowledgeMappedRecords {
    var rooms: [RoomRecord] = []
    var people: [PersonRecord] = []
    var messages: [MessageRecord] = []
    var evidence: [BeliefEvidenceRecord] = []
    var messageEventsProcessed = 0
}

/// Bridges normalized signal batches into the existing knowledge tables.
final class SignalKnowledgeWriter: SignalKnowledgeWriting {
    private let knowledgeStore: SignalKnowledgeWritableStore
    private let now: () -> Date

    /// Injects storage and time so idempotency tests can use deterministic rows.
    init(
        knowledgeStore: SignalKnowledgeWritableStore,
        now: @escaping () -> Date = Date.init
    ) {
        self.knowledgeStore = knowledgeStore
        self.now = now
    }

    /// Persists supported objects/events and returns counts for sync reporting.
    func write(_ batch: SignalSyncBatch) throws -> SignalWriteSummary {
        try knowledgeStore.bootstrap()
        let updatedAt = Self.iso8601(now())
        let mappedRecords = deduplicateDimensionRows(
            in: mapRecords(for: batch, fallbackUpdatedAt: updatedAt)
        )
        try knowledgeStore.writeConnectorMessageBatch(
            rooms: mappedRecords.rooms,
            people: mappedRecords.people,
            messages: mappedRecords.messages,
            evidence: mappedRecords.evidence
        )

        return SignalWriteSummary(
            messageEventsProcessed: mappedRecords.messageEventsProcessed,
            evidenceRecordsWritten: mappedRecords.evidence.count
        )
    }

    /// Maps a signal batch into legacy knowledge rows without touching storage.
    private func mapRecords(
        for batch: SignalSyncBatch,
        fallbackUpdatedAt: String
    ) -> SignalKnowledgeMappedRecords {
        var records = SignalKnowledgeMappedRecords()
        appendObjects(batch.objects, fallbackUpdatedAt: fallbackUpdatedAt, to: &records)
        for event in batch.events {
            guard case .message(let payload) = event.payload else { continue }
            appendMessageEvent(
                event,
                payload: payload,
                batch: batch,
                fallbackUpdatedAt: fallbackUpdatedAt,
                to: &records
            )
            records.messageEventsProcessed += 1
        }
        return records
    }

    private func deduplicateDimensionRows(
        in records: SignalKnowledgeMappedRecords
    ) -> SignalKnowledgeMappedRecords {
        var deduplicated = records
        deduplicated.rooms = lastWriteWins(records.rooms, key: \.id)
        deduplicated.people = lastWriteWins(records.people, key: \.id)
        return deduplicated
    }

    private func lastWriteWins<Record>(
        _ records: [Record],
        key: KeyPath<Record, String>
    ) -> [Record] {
        var indexesByKey: [String: Int] = [:]
        var result: [Record] = []
        for record in records {
            let recordKey = record[keyPath: key]
            if let index = indexesByKey[recordKey] {
                result[index] = record
            } else {
                indexesByKey[recordKey] = result.count
                result.append(record)
            }
        }
        return result
    }

    /// Maps supported signal objects into the legacy room/person tables.
    private func appendObjects(
        _ objects: [SignalObject],
        fallbackUpdatedAt: String,
        to records: inout SignalKnowledgeMappedRecords
    ) {
        for object in objects {
            let updatedAt = object.updatedAt.map(Self.iso8601) ?? fallbackUpdatedAt
            switch object.kind {
            case .person:
                records.people.append(
                    PersonRecord(
                        id: object.id.rawValue,
                        displayName: normalizedText(object.title, fallback: object.id.rawValue),
                        email: object.emailProperty,
                        updatedAt: updatedAt
                    )
                )
            case .space, .thread, .channel:
                records.rooms.append(
                    RoomRecord(
                        id: object.sourceID.externalID,
                        title: normalizedText(object.title, fallback: object.sourceID.externalID),
                        updatedAt: updatedAt
                    )
                )
            case .document, .file, .issue, .pullRequest, .project:
                // No generic object tables yet; keep these in the batch until
                // raw signal persistence lands.
                continue
            }
        }
    }

    /// Maps one message event into room/person/message/evidence rows.
    private func appendMessageEvent(
        _ event: SignalEvent,
        payload: MessageEventPayload,
        batch: SignalSyncBatch,
        fallbackUpdatedAt: String,
        to records: inout SignalKnowledgeMappedRecords
    ) {
        let createdAt = Self.iso8601(event.occurredAt)
        // Existing focus views key rooms by native thread/room ID; message and
        // evidence IDs stay globally scoped to avoid cross-connector collisions.
        let roomID = payload.threadSourceID.externalID
        let roomTitle = normalizedText(payload.threadTitle, fallback: roomID)
        records.rooms.append(
            RoomRecord(
                id: roomID,
                title: roomTitle,
                updatedAt: createdAt
            )
        )

        let personID = payload.senderID?.rawValue
        if let personID, !personID.isEmpty {
            records.people.append(
                PersonRecord(
                    id: personID,
                    displayName: normalizedText(payload.senderDisplayName, fallback: personID),
                    email: payload.senderEmail ?? "",
                    updatedAt: fallbackUpdatedAt
                )
            )
        }

        let body = normalizedMessageText(payload.body)
        records.messages.append(
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
            return
        }

        let evidenceSource = "\(batch.connectorID.rawValue)_message"
        // Existing belief filters group by source kind; event.id carries the
        // globally-scoped uniqueness.
        records.evidence.append(
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
