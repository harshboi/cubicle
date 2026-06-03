import Foundation
import SQLite3

private let checkpointSQLiteTransientDestructor = unsafeBitCast(-1, to: sqlite3_destructor_type.self)

/// Persisted cursor/backoff payload for one connector target.
struct ConnectorCheckpointRecord: Hashable {
    var connectorID: String
    var targetID: String
    var key: String
    var valueJSON: String
    var metadataJSON: String
    var updatedAt: String
}

/// SQLite DAO for connector-owned sync cursors and watermarks.
final class ConnectorCheckpointDAO {
    let database: KnowledgeDatabase

    /// Creates a checkpoint DAO against the active runtime root.
    init(configuration: RuntimeConfiguration = .current) {
        self.database = KnowledgeDatabase(configuration: configuration)
    }

    /// Creates a checkpoint DAO sharing an existing database configuration.
    init(database: KnowledgeDatabase) {
        self.database = database
    }

    /// Inserts or replaces one connector checkpoint.
    func upsert(_ checkpoint: ConnectorCheckpointRecord) throws {
        try withDatabase { db in
            let sql = """
            INSERT INTO connector_checkpoints (
                connector_id,
                target_id,
                checkpoint_key,
                value_json,
                metadata_json,
                updated_at
            ) VALUES (?, ?, ?, ?, ?, ?)
            ON CONFLICT(connector_id, target_id, checkpoint_key) DO UPDATE SET
                value_json = excluded.value_json,
                metadata_json = excluded.metadata_json,
                updated_at = excluded.updated_at;
            """
            try withPreparedStatement(db: db, sql: sql) { statement in
                try bindText(normalizedNonEmpty(checkpoint.connectorID), at: 1, in: statement, sql: sql)
                try bindText(normalizedNonEmpty(checkpoint.targetID), at: 2, in: statement, sql: sql)
                try bindText(normalizedNonEmpty(checkpoint.key), at: 3, in: statement, sql: sql)
                try bindText(normalizedJSON(checkpoint.valueJSON), at: 4, in: statement, sql: sql)
                try bindText(normalizedJSON(checkpoint.metadataJSON), at: 5, in: statement, sql: sql)
                try bindText(normalizedNonEmpty(checkpoint.updatedAt), at: 6, in: statement, sql: sql)
                try stepToDone(statement, sql: sql, db: db)
            }
        }
    }

    /// Loads one connector checkpoint by source, target, and key.
    func load(connectorID: String, targetID: String, key: String) throws -> ConnectorCheckpointRecord? {
        try withDatabase { db in
            let sql = """
            SELECT
                connector_id,
                target_id,
                checkpoint_key,
                value_json,
                metadata_json,
                updated_at
            FROM connector_checkpoints
            WHERE connector_id = ?
              AND target_id = ?
              AND checkpoint_key = ?
            LIMIT 1;
            """
            return try withPreparedStatement(db: db, sql: sql) { statement in
                try bindText(normalizedNonEmpty(connectorID), at: 1, in: statement, sql: sql)
                try bindText(normalizedNonEmpty(targetID), at: 2, in: statement, sql: sql)
                try bindText(normalizedNonEmpty(key), at: 3, in: statement, sql: sql)
                guard try stepSelect(statement, sql: sql, db: db) == SQLITE_ROW else {
                    return nil
                }
                return decodeCheckpoint(statement)
            }
        }
    }

    /// Lists checkpoints for one connector, optionally scoped to one target.
    func loadAll(connectorID: String, targetID: String? = nil) throws -> [ConnectorCheckpointRecord] {
        try withDatabase { db in
            var values: [String] = [normalizedNonEmpty(connectorID)]
            var whereClause = "WHERE connector_id = ?"
            if let targetID {
                whereClause += " AND target_id = ?"
                values.append(normalizedNonEmpty(targetID))
            }
            let sql = """
            SELECT
                connector_id,
                target_id,
                checkpoint_key,
                value_json,
                metadata_json,
                updated_at
            FROM connector_checkpoints
            \(whereClause)
            ORDER BY target_id ASC, checkpoint_key ASC;
            """
            return try withPreparedStatement(db: db, sql: sql) { statement in
                for (offset, value) in values.enumerated() {
                    try bindText(value, at: Int32(offset + 1), in: statement, sql: sql)
                }
                var records: [ConnectorCheckpointRecord] = []
                while try stepSelect(statement, sql: sql, db: db) == SQLITE_ROW {
                    records.append(decodeCheckpoint(statement))
                }
                return records
            }
        }
    }

    private func withDatabase<T>(_ body: (OpaquePointer) throws -> T) throws -> T {
        try database.withOpenConnection { db in
            try ensureSchema(db: db)
            return try body(db)
        }
    }

    private func ensureSchema(db: OpaquePointer) throws {
        try execute(
            sql: """
            CREATE TABLE IF NOT EXISTS connector_checkpoints (
                connector_id TEXT NOT NULL,
                target_id TEXT NOT NULL,
                checkpoint_key TEXT NOT NULL,
                value_json TEXT NOT NULL DEFAULT '{}',
                metadata_json TEXT NOT NULL DEFAULT '{}',
                updated_at TEXT NOT NULL,
                PRIMARY KEY (connector_id, target_id, checkpoint_key)
            );
            """,
            db: db
        )
        try execute(
            sql: """
            CREATE INDEX IF NOT EXISTS idx_connector_checkpoints_target
            ON connector_checkpoints(connector_id, target_id, updated_at DESC);
            """,
            db: db
        )
        try execute(
            sql: """
            CREATE INDEX IF NOT EXISTS idx_connector_checkpoints_updated
            ON connector_checkpoints(updated_at DESC);
            """,
            db: db
        )
    }

    private func execute(sql: String, db: OpaquePointer) throws {
        var errorMessage: UnsafeMutablePointer<Int8>?
        if sqlite3_exec(db, sql, nil, nil, &errorMessage) != SQLITE_OK {
            let message = errorMessage.map { String(cString: $0) } ?? String(cString: sqlite3_errmsg(db))
            sqlite3_free(errorMessage)
            throw KnowledgeStoreError.sqliteExecFailed(sql: sql, message: message)
        }
        sqlite3_free(errorMessage)
    }

    private func withPreparedStatement<T>(
        db: OpaquePointer,
        sql: String,
        _ body: (OpaquePointer) throws -> T
    ) throws -> T {
        var statement: OpaquePointer?
        let rc = sqlite3_prepare_v2(db, sql, -1, &statement, nil)
        guard rc == SQLITE_OK, let statement else {
            throw KnowledgeStoreError.sqlitePrepareFailed(sql: sql, message: String(cString: sqlite3_errmsg(db)))
        }
        defer { sqlite3_finalize(statement) }
        return try body(statement)
    }

    private func bindText(_ value: String, at index: Int32, in statement: OpaquePointer, sql: String) throws {
        let rc = sqlite3_bind_text(statement, index, value, -1, checkpointSQLiteTransientDestructor)
        guard rc == SQLITE_OK else {
            throw KnowledgeStoreError.sqliteBindFailed(
                sql: sql,
                index: index,
                code: rc,
                message: String(cString: sqlite3_errmsg(sqlite3_db_handle(statement)))
            )
        }
    }

    private func stepToDone(_ statement: OpaquePointer, sql: String, db: OpaquePointer) throws {
        let rc = sqlite3_step(statement)
        guard rc == SQLITE_DONE else {
            throw sqliteErrorForStep(sql: sql, db: db, code: rc)
        }
    }

    private func stepSelect(_ statement: OpaquePointer, sql: String, db: OpaquePointer) throws -> Int32 {
        let rc = sqlite3_step(statement)
        guard rc == SQLITE_ROW || rc == SQLITE_DONE else {
            throw sqliteErrorForStep(sql: sql, db: db, code: rc)
        }
        return rc
    }

    private func sqliteErrorForStep(sql: String, db: OpaquePointer, code: Int32) -> KnowledgeStoreError {
        KnowledgeStoreError.sqliteStepFailed(
            sql: sql,
            code: code,
            message: String(cString: sqlite3_errmsg(db))
        )
    }

    private func decodeCheckpoint(_ statement: OpaquePointer) -> ConnectorCheckpointRecord {
        ConnectorCheckpointRecord(
            connectorID: columnText(statement, index: 0),
            targetID: columnText(statement, index: 1),
            key: columnText(statement, index: 2),
            valueJSON: columnText(statement, index: 3),
            metadataJSON: columnText(statement, index: 4),
            updatedAt: columnText(statement, index: 5)
        )
    }

    private func columnText(_ statement: OpaquePointer, index: Int32) -> String {
        guard let cValue = sqlite3_column_text(statement, index) else {
            return ""
        }
        return String(cString: cValue)
    }

    private func normalizedNonEmpty(_ value: String) -> String {
        let trimmed = value.trimmingCharacters(in: .whitespacesAndNewlines)
        return trimmed.isEmpty ? "unknown" : trimmed
    }

    private func normalizedJSON(_ value: String) throws -> String {
        let trimmed = value.trimmingCharacters(in: .whitespacesAndNewlines)
        let json = trimmed.isEmpty ? "{}" : trimmed
        guard let data = json.data(using: .utf8) else {
            throw KnowledgeStoreError.invalidJSON(json)
        }
        do {
            _ = try JSONSerialization.jsonObject(with: data, options: [.fragmentsAllowed])
            return json
        } catch {
            throw KnowledgeStoreError.invalidJSON(json)
        }
    }
}
