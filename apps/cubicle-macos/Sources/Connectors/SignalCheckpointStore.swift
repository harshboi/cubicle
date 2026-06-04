import Foundation

private let signalCheckpointAccountIDMetadataKey = "_accountID"

/// Typed checkpoint boundary used by connector pipelines.
protocol SignalCheckpointStoring {
    func loadCheckpoints(connectorID: ConnectorID, targetIDs: [String]) throws -> ConnectorCheckpointSet
    func save(_ checkpoint: ConnectorCheckpoint) throws
}

extension SignalCheckpointStoring {
    func save(_ checkpoints: [ConnectorCheckpoint]) throws {
        for checkpoint in checkpoints {
            try save(checkpoint)
        }
    }
}

enum SignalCheckpointStoreError: LocalizedError {
    case invalidUpdatedAt(String)

    var errorDescription: String? {
        switch self {
        case .invalidUpdatedAt(let value):
            return "Invalid connector checkpoint timestamp: \(value)"
        }
    }
}

/// DAO-backed checkpoint store for connector cursors and retry state.
final class SignalCheckpointStore: SignalCheckpointStoring {
    private let dao: ConnectorCheckpointDAO

    init(configuration: RuntimeConfiguration = .current) {
        self.dao = ConnectorCheckpointDAO(configuration: configuration)
    }

    init(dao: ConnectorCheckpointDAO) {
        self.dao = dao
    }

    func loadCheckpoints(connectorID: ConnectorID, targetIDs: [String]) throws -> ConnectorCheckpointSet {
        var checkpoints: [ConnectorCheckpoint] = []
        for targetID in stableTargetIDs(targetIDs) {
            let records = try dao.loadAll(connectorID: connectorID.rawValue, targetID: targetID)
            checkpoints.append(contentsOf: try records.map(decodeCheckpoint))
        }
        return ConnectorCheckpointSet(checkpoints)
    }

    func save(_ checkpoint: ConnectorCheckpoint) throws {
        var metadata = checkpoint.metadata
        metadata[signalCheckpointAccountIDMetadataKey] = checkpoint.accountID
        try dao.upsert(
            ConnectorCheckpointRecord(
                connectorID: checkpoint.connectorID.rawValue,
                targetID: checkpoint.targetID,
                key: checkpoint.key,
                valueJSON: try encodeDictionary(checkpoint.payload),
                metadataJSON: try encodeDictionary(metadata),
                updatedAt: Self.iso8601(checkpoint.updatedAt)
            )
        )
    }

    private func decodeCheckpoint(_ record: ConnectorCheckpointRecord) throws -> ConnectorCheckpoint {
        guard let updatedAt = Self.date(from: record.updatedAt) else {
            throw SignalCheckpointStoreError.invalidUpdatedAt(record.updatedAt)
        }
        var metadata = try decodeDictionary(record.metadataJSON)
        let accountID = metadata.removeValue(forKey: signalCheckpointAccountIDMetadataKey) ?? "default"
        return ConnectorCheckpoint(
            connectorID: ConnectorID(rawValue: record.connectorID),
            accountID: accountID,
            targetID: record.targetID,
            key: record.key,
            updatedAt: updatedAt,
            payload: try decodeDictionary(record.valueJSON),
            metadata: metadata
        )
    }

    private func stableTargetIDs(_ targetIDs: [String]) -> [String] {
        var seen: Set<String> = []
        var result: [String] = []
        for targetID in targetIDs {
            let normalized = targetID.trimmingCharacters(in: .whitespacesAndNewlines)
            guard !normalized.isEmpty, seen.insert(normalized).inserted else {
                continue
            }
            result.append(normalized)
        }
        return result
    }

    private func encodeDictionary(_ value: [String: String]) throws -> String {
        guard !value.isEmpty else { return "{}" }
        let data = try JSONSerialization.data(withJSONObject: value, options: [.sortedKeys])
        return String(data: data, encoding: .utf8) ?? "{}"
    }

    private func decodeDictionary(_ value: String) throws -> [String: String] {
        let data = Data(value.utf8)
        let object = try JSONSerialization.jsonObject(with: data, options: [])
        guard let dictionary = object as? [String: Any] else {
            return [:]
        }
        var result: [String: String] = [:]
        for (key, value) in dictionary {
            switch value {
            case let string as String:
                result[key] = string
            case let bool as Bool:
                result[key] = bool ? "true" : "false"
            case let number as NSNumber:
                result[key] = number.stringValue
            case is NSNull:
                continue
            default:
                result[key] = String(describing: value)
            }
        }
        return result
    }

    private static func iso8601(_ date: Date) -> String {
        iso8601WithFractionalSeconds.string(from: date)
    }

    private static func date(from value: String) -> Date? {
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
