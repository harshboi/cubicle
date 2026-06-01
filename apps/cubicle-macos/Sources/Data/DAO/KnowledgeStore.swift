import Foundation
import SQLite3

private let sqliteTransientDestructor = unsafeBitCast(-1, to: sqlite3_destructor_type.self)

enum KnowledgeStoreError: LocalizedError {
    case sqliteOpenFailed(path: String, message: String)
    case sqliteExecFailed(sql: String, message: String)
    case sqlitePrepareFailed(sql: String, message: String)
    case sqliteBindFailed(sql: String, index: Int32, code: Int32, message: String)
    case sqliteStepFailed(sql: String, code: Int32, message: String)
    case unsupportedSchemaVersion(current: Int32, latest: Int32)
    case invalidJSON(String)
    case invalidBeliefScope(String)
    case invalidBeliefStatement(String)

    var errorDescription: String? {
        switch self {
        case .sqliteOpenFailed(let path, let message):
            return "Failed to open SQLite database at \(path): \(message)"
        case .sqliteExecFailed(let sql, let message):
            return "SQLite exec failed for [\(sql)]: \(message)"
        case .sqlitePrepareFailed(let sql, let message):
            return "SQLite prepare failed for [\(sql)]: \(message)"
        case .sqliteBindFailed(let sql, let index, let code, let message):
            return "SQLite bind failed at index \(index) for [\(sql)] (code \(code)): \(message)"
        case .sqliteStepFailed(let sql, let code, let message):
            return "SQLite step failed for [\(sql)] (code \(code)): \(message)"
        case .unsupportedSchemaVersion(let current, let latest):
            return "SQLite schema version \(current) is newer than supported version \(latest)"
        case .invalidJSON(let value):
            return "Unable to decode JSON payload: \(value)"
        case .invalidBeliefScope(let scope):
            return "Unsupported belief scope value: \(scope)"
        case .invalidBeliefStatement(let statement):
            return "Belief statement must not be empty: \(statement)"
        }
    }
}

private enum BeliefManualFilter {
    case all
    case manualOnly
    case automaticOnly
}

private enum SQLiteBindValue {
    case text(String)
    case optionalText(String?)
    case int64(Int64)
    case double(Double)
}

final class KnowledgeStore: KnowledgeDAO {
    let configuration: RuntimeConfiguration
    let database: KnowledgeDatabase
    private let jsonEncoder = JSONEncoder()
    private let jsonDecoder = JSONDecoder()
    private let timestampFormatter: ISO8601DateFormatter = {
        let formatter = ISO8601DateFormatter()
        formatter.formatOptions = [.withInternetDateTime, .withFractionalSeconds]
        return formatter
    }()

    init(configuration: RuntimeConfiguration = .current) {
        self.configuration = configuration
        self.database = KnowledgeDatabase(configuration: configuration)
        self.jsonEncoder.dateEncodingStrategy = .iso8601
        self.jsonDecoder.dateDecodingStrategy = .iso8601
    }

    var knowledgeDirectory: URL {
        database.knowledgeDirectory
    }

    var databaseURL: URL {
        database.databaseURL
    }

    func status() -> KnowledgeDatabaseStatus {
        database.status()
    }

    func bootstrap() throws {
        _ = try withDatabase { _ in true }
    }

    func upsertFocusCluster(_ cluster: FocusClusterRecord) throws {
        try withDatabase { db in
            let sql = """
            INSERT INTO focus_clusters (
                cluster_id,
                focus_kind,
                scope,
                entity_key,
                topic_key,
                title,
                summary,
                prompt_version,
                source_hash,
                generated_at,
                updated_at
            ) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
            ON CONFLICT(focus_kind, scope, entity_key, topic_key) DO UPDATE SET
                cluster_id = excluded.cluster_id,
                title = excluded.title,
                summary = excluded.summary,
                prompt_version = excluded.prompt_version,
                source_hash = excluded.source_hash,
                generated_at = excluded.generated_at,
                updated_at = excluded.updated_at;
            """
            let clusterID = normalizedID(cluster.id)
            let generatedAt = normalizedTimestamp(cluster.generatedAt)
            let updatedAt = normalizedTimestamp(cluster.updatedAt.isEmpty ? generatedAt : cluster.updatedAt)
            try withPreparedStatement(db: db, sql: sql) { statement in
                try bindText(clusterID, at: 1, in: statement, sql: sql)
                try bindText(normalizedNonEmpty(cluster.focusKind), at: 2, in: statement, sql: sql)
                try bindText(normalizedNonEmpty(cluster.scope), at: 3, in: statement, sql: sql)
                try bindText(normalizedNonEmpty(cluster.entityKey), at: 4, in: statement, sql: sql)
                try bindText(normalizedNonEmpty(cluster.topicKey), at: 5, in: statement, sql: sql)
                try bindText(cluster.title.trimmingCharacters(in: .whitespacesAndNewlines), at: 6, in: statement, sql: sql)
                try bindText(cluster.summary.trimmingCharacters(in: .whitespacesAndNewlines), at: 7, in: statement, sql: sql)
                try bindText(cluster.promptVersion.trimmingCharacters(in: .whitespacesAndNewlines), at: 8, in: statement, sql: sql)
                try bindText(cluster.sourceHash.trimmingCharacters(in: .whitespacesAndNewlines), at: 9, in: statement, sql: sql)
                try bindText(generatedAt, at: 10, in: statement, sql: sql)
                try bindText(updatedAt, at: 11, in: statement, sql: sql)
                try stepToDone(statement, sql: sql, db: db)
            }
        }
    }

    func loadFocusClusters(focusKind: String, scope: String, entityKey: String) throws -> [FocusClusterRecord] {
        try withDatabase { db in
            let sql = """
            SELECT
                cluster_id,
                focus_kind,
                scope,
                entity_key,
                topic_key,
                title,
                summary,
                prompt_version,
                source_hash,
                generated_at,
                updated_at
            FROM focus_clusters
            WHERE focus_kind = ? AND scope = ? AND entity_key = ?
            ORDER BY updated_at DESC;
            """
            return try withPreparedStatement(db: db, sql: sql) { statement in
                try bindText(normalizedNonEmpty(focusKind), at: 1, in: statement, sql: sql)
                try bindText(normalizedNonEmpty(scope), at: 2, in: statement, sql: sql)
                try bindText(normalizedNonEmpty(entityKey), at: 3, in: statement, sql: sql)
                var rows: [FocusClusterRecord] = []
                while try stepSelect(statement, sql: sql, db: db) == SQLITE_ROW {
                    rows.append(
                        FocusClusterRecord(
                            id: columnText(statement, index: 0),
                            focusKind: columnText(statement, index: 1),
                            scope: columnText(statement, index: 2),
                            entityKey: columnText(statement, index: 3),
                            topicKey: columnText(statement, index: 4),
                            title: columnText(statement, index: 5),
                            summary: columnText(statement, index: 6),
                            promptVersion: columnText(statement, index: 7),
                            sourceHash: columnText(statement, index: 8),
                            generatedAt: columnText(statement, index: 9),
                            updatedAt: columnText(statement, index: 10)
                        )
                    )
                }
                return rows
            }
        }
    }

    func upsertTopic(_ topic: TopicRecord) throws {
        try withDatabase { db in
            let topicID = normalizedID(topic.id)
            let title = topic.title.trimmingCharacters(in: .whitespacesAndNewlines)
            let summary = topic.summary.trimmingCharacters(in: .whitespacesAndNewlines)
            let soWhat = topic.soWhat.trimmingCharacters(in: .whitespacesAndNewlines)
            let sourceLabel = topic.sourceLabel.trimmingCharacters(in: .whitespacesAndNewlines)
            let generatedAt = normalizedTimestamp(topic.generatedAt)
            let updatedAt = normalizedTimestamp(topic.updatedAt.isEmpty ? generatedAt : topic.updatedAt)
            let columns = try tableColumnNames("topics", db: db)
            var insertColumns = [
                "topic_id",
                "focus_kind",
                "scope",
                "entity_key",
                "topic_key",
                "title",
                "summary",
                "so_what",
                "source_label",
                "score",
                "generated_at",
                "updated_at"
            ]
            var updateAssignments = [
                "topic_id = excluded.topic_id",
                "title = excluded.title",
                "summary = excluded.summary",
                "so_what = excluded.so_what",
                "source_label = excluded.source_label",
                "score = excluded.score",
                "generated_at = excluded.generated_at",
                "updated_at = excluded.updated_at"
            ]
            if columns.contains("label") {
                insertColumns.append("label")
                updateAssignments.append("label = excluded.label")
            }
            if columns.contains("first_seen_at") {
                insertColumns.append("first_seen_at")
                updateAssignments.append(
                    "first_seen_at = CASE WHEN TRIM(COALESCE(first_seen_at, '')) = '' THEN excluded.first_seen_at ELSE first_seen_at END"
                )
            }
            if columns.contains("last_seen_at") {
                insertColumns.append("last_seen_at")
                updateAssignments.append("last_seen_at = excluded.last_seen_at")
            }
            let placeholders = Array(repeating: "?", count: insertColumns.count).joined(separator: ", ")
            let sql = """
            INSERT INTO topics (
                \(insertColumns.joined(separator: ",\n                "))
            ) VALUES (\(placeholders))
            ON CONFLICT(focus_kind, scope, entity_key, topic_key) DO UPDATE SET
                \(updateAssignments.joined(separator: ",\n                "));
            """
            try withPreparedStatement(db: db, sql: sql) { statement in
                try bindText(topicID, at: 1, in: statement, sql: sql)
                try bindText(normalizedNonEmpty(topic.focusKind), at: 2, in: statement, sql: sql)
                try bindText(normalizedNonEmpty(topic.scope), at: 3, in: statement, sql: sql)
                try bindText(normalizedNonEmpty(topic.entityKey), at: 4, in: statement, sql: sql)
                try bindText(normalizedNonEmpty(topic.topicKey), at: 5, in: statement, sql: sql)
                try bindText(title, at: 6, in: statement, sql: sql)
                try bindText(summary, at: 7, in: statement, sql: sql)
                try bindText(soWhat, at: 8, in: statement, sql: sql)
                try bindText(sourceLabel, at: 9, in: statement, sql: sql)
                try bindOptionalDouble(topic.score, at: 10, in: statement, sql: sql)
                try bindText(generatedAt, at: 11, in: statement, sql: sql)
                try bindText(updatedAt, at: 12, in: statement, sql: sql)
                var bindIndex: Int32 = 13
                if columns.contains("label") {
                    try bindText(title.isEmpty ? topicID : title, at: bindIndex, in: statement, sql: sql)
                    bindIndex += 1
                }
                if columns.contains("first_seen_at") {
                    try bindText(generatedAt, at: bindIndex, in: statement, sql: sql)
                    bindIndex += 1
                }
                if columns.contains("last_seen_at") {
                    try bindText(updatedAt, at: bindIndex, in: statement, sql: sql)
                }
                try stepToDone(statement, sql: sql, db: db)
            }
        }
    }

    func loadTopics(focusKind: String, scope: String, entityKey: String, limit: Int = 5) throws -> [TopicRecord] {
        try withDatabase { db in
            let sql = """
            SELECT
                topic_id,
                focus_kind,
                scope,
                entity_key,
                topic_key,
                title,
                summary,
                so_what,
                source_label,
                score,
                generated_at,
                updated_at
            FROM topics
            WHERE focus_kind = ? AND scope = ? AND entity_key = ?
            ORDER BY COALESCE(score, -1.0) DESC, updated_at DESC
            LIMIT ?;
            """
            return try withPreparedStatement(db: db, sql: sql) { statement in
                try bindText(normalizedNonEmpty(focusKind), at: 1, in: statement, sql: sql)
                try bindText(normalizedNonEmpty(scope), at: 2, in: statement, sql: sql)
                try bindText(normalizedNonEmpty(entityKey), at: 3, in: statement, sql: sql)
                try bindInteger(Int32(max(limit, 1)), at: 4, in: statement, sql: sql)
                var rows: [TopicRecord] = []
                while try stepSelect(statement, sql: sql, db: db) == SQLITE_ROW {
                    rows.append(
                        TopicRecord(
                            id: columnText(statement, index: 0),
                            focusKind: columnText(statement, index: 1),
                            scope: columnText(statement, index: 2),
                            entityKey: columnText(statement, index: 3),
                            topicKey: columnText(statement, index: 4),
                            title: columnText(statement, index: 5),
                            summary: columnText(statement, index: 6),
                            soWhat: columnText(statement, index: 7),
                            sourceLabel: columnText(statement, index: 8),
                            score: sqlite3_column_type(statement, 9) == SQLITE_NULL ? nil : sqlite3_column_double(statement, 9),
                            generatedAt: columnText(statement, index: 10),
                            updatedAt: columnText(statement, index: 11)
                        )
                    )
                }
                return rows
            }
        }
    }

    func loadBeliefs(scope: KnowledgeBeliefScope, entityKey: String = "") throws -> [BeliefRecord] {
        try loadBeliefs(
            scope: scope.rawValue,
            entityKey: scopedBeliefEntityKey(scope: scope, entityKey: entityKey),
            manualFilter: .all
        )
    }

    func loadBeliefs(scope: String, entityKey: String) throws -> [BeliefRecord] {
        try loadBeliefs(scope: scope, entityKey: entityKey, manualFilter: .all)
    }

    func loadManualBeliefs(scope: KnowledgeBeliefScope, entityKey: String = "") throws -> [BeliefRecord] {
        try loadBeliefs(
            scope: scope.rawValue,
            entityKey: scopedBeliefEntityKey(scope: scope, entityKey: entityKey),
            manualFilter: .manualOnly
        )
    }

    func loadManualBeliefs(scope: String, entityKey: String) throws -> [BeliefRecord] {
        try loadBeliefs(scope: scope, entityKey: entityKey, manualFilter: .manualOnly)
    }

    func loadAutomaticBeliefs(scope: KnowledgeBeliefScope, entityKey: String = "") throws -> [BeliefRecord] {
        try loadBeliefs(
            scope: scope.rawValue,
            entityKey: scopedBeliefEntityKey(scope: scope, entityKey: entityKey),
            manualFilter: .automaticOnly
        )
    }

    func loadAutomaticBeliefs(scope: String, entityKey: String) throws -> [BeliefRecord] {
        try loadBeliefs(scope: scope, entityKey: entityKey, manualFilter: .automaticOnly)
    }

    private func loadBeliefs(scope: String, entityKey: String, manualFilter: BeliefManualFilter) throws -> [BeliefRecord] {
        let beliefScope = try normalizedBeliefScope(scope)
        let scopedEntityKey = scopedBeliefEntityKey(scope: beliefScope, entityKey: entityKey)

        var whereClauses = ["scope = ?", "entity_key = ?"]
        switch manualFilter {
        case .all:
            break
        case .manualOnly:
            whereClauses.append("is_manual = 1")
        case .automaticOnly:
            whereClauses.append("is_manual = 0")
        }

        return try withDatabase { db in
            let sql = """
            SELECT
                belief_id,
                scope,
                entity_key,
                statement,
                confidence,
                updated_at,
                is_manual,
                evidence_links_json,
                created_at,
                belief_kind,
                lifecycle,
                support_count,
                contradiction_count,
                last_evidence_at
            FROM beliefs
            WHERE \(whereClauses.joined(separator: " AND "))
            ORDER BY updated_at DESC;
            """
            return try withPreparedStatement(db: db, sql: sql) { statement in
                try bindText(beliefScope.rawValue, at: 1, in: statement, sql: sql)
                try bindText(scopedEntityKey, at: 2, in: statement, sql: sql)
                var beliefs: [BeliefRecord] = []
                while try stepSelect(statement, sql: sql, db: db) == SQLITE_ROW {
                    beliefs.append(try decodeBeliefRow(statement))
                }
                return beliefs
            }
        }
    }

    func upsertManualBelief(_ belief: BeliefRecord) throws {
        var manualBelief = belief
        manualBelief.isManual = true
        try upsertBelief(manualBelief)
    }

    func upsertBelief(_ belief: BeliefRecord) throws {
        try withDatabase { db in
            try upsertBeliefRecord(belief, db: db)
        }
    }

    func upsertBeliefs(_ beliefs: [BeliefRecord]) throws {
        guard beliefs.isEmpty == false else {
            return
        }
        try withDatabase { db in
            try execute(sql: "BEGIN IMMEDIATE TRANSACTION;", db: db)
            do {
                for belief in beliefs {
                    try upsertBeliefRecord(belief, db: db)
                }
                try execute(sql: "COMMIT;", db: db)
            } catch {
                _ = sqlite3_exec(db, "ROLLBACK;", nil, nil, nil)
                throw error
            }
        }
    }

    @discardableResult
    func updateManualBelief(
        id: String,
        scope: KnowledgeBeliefScope,
        entityKey: String = "",
        statement: String,
        confidence: Double,
        evidenceLinks: [String],
        updatedAt: String = ""
    ) throws -> Bool {
        try updateManualBelief(
            id: id,
            scope: scope.rawValue,
            entityKey: scopedBeliefEntityKey(scope: scope, entityKey: entityKey),
            statement: statement,
            confidence: confidence,
            evidenceLinks: evidenceLinks,
            updatedAt: updatedAt
        )
    }

    @discardableResult
    func updateManualBelief(
        id: String,
        scope: String,
        entityKey: String,
        statement: String,
        confidence: Double,
        evidenceLinks: [String],
        updatedAt: String = ""
    ) throws -> Bool {
        let beliefScope = try normalizedBeliefScope(scope)
        let scopedEntityKey = scopedBeliefEntityKey(scope: beliefScope, entityKey: entityKey)
        let normalizedStatement = try normalizedBeliefStatement(statement)
        let normalizedBeliefID = id.trimmingCharacters(in: .whitespacesAndNewlines)
        guard normalizedBeliefID.isEmpty == false else {
            return false
        }
        let normalizedUpdatedAt = normalizedTimestamp(updatedAt)
        let encodedEvidence = try encodeEvidenceLinks(evidenceLinks)

        return try withDatabase { db in
            let sql = """
            UPDATE beliefs
            SET scope = ?,
                entity_key = ?,
                statement = ?,
                confidence = ?,
                is_manual = 1,
                evidence_links_json = ?,
                belief_kind = 'manual',
                lifecycle = 'manual',
                support_count = MAX(COALESCE(support_count, 0), 1),
                contradiction_count = 0,
                last_evidence_at = ?,
                updated_at = ?
            WHERE belief_id = ? AND is_manual = 1;
            """
            try withPreparedStatement(db: db, sql: sql) { statementHandle in
                try bindText(beliefScope.rawValue, at: 1, in: statementHandle, sql: sql)
                try bindText(scopedEntityKey, at: 2, in: statementHandle, sql: sql)
                try bindText(normalizedStatement, at: 3, in: statementHandle, sql: sql)
                try bindDouble(confidence, at: 4, in: statementHandle, sql: sql)
                try bindText(encodedEvidence, at: 5, in: statementHandle, sql: sql)
                try bindText(normalizedUpdatedAt, at: 6, in: statementHandle, sql: sql)
                try bindText(normalizedUpdatedAt, at: 7, in: statementHandle, sql: sql)
                try bindText(normalizedBeliefID, at: 8, in: statementHandle, sql: sql)
                try stepToDone(statementHandle, sql: sql, db: db)
            }
            return sqlite3_changes(db) > 0
        }
    }

    @discardableResult
    func deleteManualBelief(id: String) throws -> Bool {
        let normalizedBeliefID = id.trimmingCharacters(in: .whitespacesAndNewlines)
        guard normalizedBeliefID.isEmpty == false else {
            return false
        }
        return try withDatabase { db in
            let sql = "DELETE FROM beliefs WHERE belief_id = ? AND is_manual = 1;"
            try withPreparedStatement(db: db, sql: sql) { statement in
                try bindText(normalizedBeliefID, at: 1, in: statement, sql: sql)
                try stepToDone(statement, sql: sql, db: db)
            }
            return sqlite3_changes(db) > 0
        }
    }

    @discardableResult
    func deleteManualBelief(scope: KnowledgeBeliefScope, entityKey: String = "", statement: String) throws -> Bool {
        try deleteManualBelief(
            scope: scope.rawValue,
            entityKey: scopedBeliefEntityKey(scope: scope, entityKey: entityKey),
            statement: statement
        )
    }

    @discardableResult
    func deleteManualBelief(scope: String, entityKey: String, statement: String) throws -> Bool {
        let beliefScope = try normalizedBeliefScope(scope)
        let scopedEntityKey = scopedBeliefEntityKey(scope: beliefScope, entityKey: entityKey)
        let normalizedStatement = try normalizedBeliefStatement(statement)
        return try withDatabase { db in
            let sql = "DELETE FROM beliefs WHERE scope = ? AND entity_key = ? AND statement = ? AND is_manual = 1;"
            try withPreparedStatement(db: db, sql: sql) { statementHandle in
                try bindText(beliefScope.rawValue, at: 1, in: statementHandle, sql: sql)
                try bindText(scopedEntityKey, at: 2, in: statementHandle, sql: sql)
                try bindText(normalizedStatement, at: 3, in: statementHandle, sql: sql)
                try stepToDone(statementHandle, sql: sql, db: db)
            }
            return sqlite3_changes(db) > 0
        }
    }

    func loadBelief(id: String) throws -> BeliefRecord? {
        let normalizedBeliefID = id.trimmingCharacters(in: .whitespacesAndNewlines)
        guard normalizedBeliefID.isEmpty == false else {
            return nil
        }
        return try withDatabase { db in
            let sql = """
            SELECT
                belief_id,
                scope,
                entity_key,
                statement,
                confidence,
                updated_at,
                is_manual,
                evidence_links_json,
                created_at,
                belief_kind,
                lifecycle,
                support_count,
                contradiction_count,
                last_evidence_at
            FROM beliefs
            WHERE belief_id = ?
            LIMIT 1;
            """
            return try withPreparedStatement(db: db, sql: sql) { statement in
                try bindText(normalizedBeliefID, at: 1, in: statement, sql: sql)
                guard try stepSelect(statement, sql: sql, db: db) == SQLITE_ROW else {
                    return nil
                }
                return try decodeBeliefRow(statement)
            }
        }
    }

    func loadManualBelief(id: String) throws -> BeliefRecord? {
        guard let belief = try loadBelief(id: id) else {
            return nil
        }
        return belief.isManual ? belief : nil
    }

    func loadBeliefReconciliationState(
        scope: KnowledgeBeliefScope,
        entityKey: String = ""
    ) throws -> BeliefReconciliationStateRecord? {
        try loadBeliefReconciliationState(
            scope: scope.rawValue,
            entityKey: scopedBeliefEntityKey(scope: scope, entityKey: entityKey)
        )
    }

    func loadBeliefReconciliationState(
        scope: String,
        entityKey: String
    ) throws -> BeliefReconciliationStateRecord? {
        let beliefScope = try normalizedBeliefScope(scope)
        let scopedEntityKey = scopedBeliefEntityKey(scope: beliefScope, entityKey: entityKey)
        return try withDatabase { db in
            let sql = """
            SELECT
                scope,
                entity_key,
                last_run_at,
                last_evidence_hash,
                updated_at
            FROM belief_reconciliation_state
            WHERE scope = ? AND entity_key = ?
            LIMIT 1;
            """
            return try withPreparedStatement(db: db, sql: sql) { statement in
                try bindText(beliefScope.rawValue, at: 1, in: statement, sql: sql)
                try bindText(scopedEntityKey, at: 2, in: statement, sql: sql)
                guard try stepSelect(statement, sql: sql, db: db) == SQLITE_ROW else {
                    return nil
                }
                return decodeBeliefReconciliationStateRow(statement)
            }
        }
    }

    func upsertBeliefReconciliationState(_ state: BeliefReconciliationStateRecord) throws {
        let beliefScope = try normalizedBeliefScope(state.scope)
        let scopedEntityKey = scopedBeliefEntityKey(scope: beliefScope, entityKey: state.entityKey)
        let normalizedLastRunAt = state.lastRunAt?.trimmingCharacters(in: .whitespacesAndNewlines)
        let normalizedEvidenceHash = state.lastEvidenceHash?.trimmingCharacters(in: .whitespacesAndNewlines)
        let updatedAt = normalizedTimestamp(state.updatedAt)

        try withDatabase { db in
            let sql = """
            INSERT INTO belief_reconciliation_state (
                scope,
                entity_key,
                last_run_at,
                last_evidence_hash,
                updated_at
            ) VALUES (?, ?, ?, ?, ?)
            ON CONFLICT(scope, entity_key) DO UPDATE SET
                last_run_at = excluded.last_run_at,
                last_evidence_hash = excluded.last_evidence_hash,
                updated_at = excluded.updated_at;
            """
            try withPreparedStatement(db: db, sql: sql) { statement in
                try bindText(beliefScope.rawValue, at: 1, in: statement, sql: sql)
                try bindText(scopedEntityKey, at: 2, in: statement, sql: sql)
                try bindOptionalText(normalizedLastRunAt, at: 3, in: statement, sql: sql)
                try bindOptionalText(normalizedEvidenceHash, at: 4, in: statement, sql: sql)
                try bindText(updatedAt, at: 5, in: statement, sql: sql)
                try stepToDone(statement, sql: sql, db: db)
            }
        }
    }

    @discardableResult
    func deleteBeliefReconciliationState(
        scope: KnowledgeBeliefScope,
        entityKey: String = ""
    ) throws -> Bool {
        try deleteBeliefReconciliationState(
            scope: scope.rawValue,
            entityKey: scopedBeliefEntityKey(scope: scope, entityKey: entityKey)
        )
    }

    @discardableResult
    func deleteBeliefReconciliationState(
        scope: String,
        entityKey: String
    ) throws -> Bool {
        let beliefScope = try normalizedBeliefScope(scope)
        let scopedEntityKey = scopedBeliefEntityKey(scope: beliefScope, entityKey: entityKey)
        return try withDatabase { db in
            let sql = "DELETE FROM belief_reconciliation_state WHERE scope = ? AND entity_key = ?;"
            try withPreparedStatement(db: db, sql: sql) { statement in
                try bindText(beliefScope.rawValue, at: 1, in: statement, sql: sql)
                try bindText(scopedEntityKey, at: 2, in: statement, sql: sql)
                try stepToDone(statement, sql: sql, db: db)
            }
            return sqlite3_changes(db) > 0
        }
    }

    func upsertQuestionCandidates(_ candidates: [QuestionCandidate]) throws {
        guard !candidates.isEmpty else {
            return
        }
        try withDatabase { db in
            try execute(sql: "BEGIN IMMEDIATE TRANSACTION;", db: db)
            do {
                for candidate in candidates {
                    try upsertQuestionCandidateRecord(candidate, db: db)
                }
                try execute(sql: "COMMIT;", db: db)
            } catch {
                _ = sqlite3_exec(db, "ROLLBACK;", nil, nil, nil)
                throw error
            }
        }
    }

    func deleteActiveQuestionCandidates(sourceKind: String) throws {
        let normalizedSourceKind = sourceKind.trimmingCharacters(in: .whitespacesAndNewlines)
        guard !normalizedSourceKind.isEmpty else {
            return
        }
        try withDatabase { db in
            let sql = """
            DELETE FROM question_candidates
            WHERE source_kind = ?
                AND status NOT IN ('answered', 'dismissed');
            """
            try withPreparedStatement(db: db, sql: sql) { statement in
                try bindText(normalizedSourceKind, at: 1, in: statement, sql: sql)
                try stepToDone(statement, sql: sql, db: db)
            }
        }
    }

    func deleteActiveQuestionCandidates() throws {
        try withDatabase { db in
            let sql = """
            DELETE FROM question_candidates
            WHERE status NOT IN ('answered', 'dismissed');
            """
            try execute(sql: sql, db: db)
        }
    }

    func listQuestionCandidates(limit: Int = 100, status: QuestionStatus? = nil) throws -> [QuestionCandidate] {
        try withDatabase { db in
            let hasStatusFilter = status != nil
            let sql: String
            if hasStatusFilter {
                sql = """
                SELECT
                    question_id,
                    scope_type,
                    scope_key,
                    scope_label,
                    question_text,
                    question_type,
                    why_now,
                    evidence_json,
                    source_kind,
                    source_key,
                    tags_json,
                    priority_score,
                    status,
                    answer_snapshot_id,
                    created_at,
                    updated_at,
                    expires_at
                FROM question_candidates
                WHERE status = ?
                ORDER BY priority_score DESC, updated_at DESC
                LIMIT ?;
                """
            } else {
                sql = """
                SELECT
                    question_id,
                    scope_type,
                    scope_key,
                    scope_label,
                    question_text,
                    question_type,
                    why_now,
                    evidence_json,
                    source_kind,
                    source_key,
                    tags_json,
                    priority_score,
                    status,
                    answer_snapshot_id,
                    created_at,
                    updated_at,
                    expires_at
                FROM question_candidates
                ORDER BY priority_score DESC, updated_at DESC
                LIMIT ?;
                """
            }

            return try withPreparedStatement(db: db, sql: sql) { statement in
                var bindIndex: Int32 = 1
                if let status {
                    try bindText(status.rawValue, at: bindIndex, in: statement, sql: sql)
                    bindIndex += 1
                }
                try bindInteger(Int32(max(limit, 1)), at: bindIndex, in: statement, sql: sql)
                var rows: [QuestionCandidate] = []
                while try stepSelect(statement, sql: sql, db: db) == SQLITE_ROW {
                    rows.append(try decodeQuestionCandidateRow(statement))
                }
                return rows
            }
        }
    }

    func listQuestionCandidates(
        scopeType: QuestionScopeType,
        scopeKey: String,
        limit: Int = 20
    ) throws -> [QuestionCandidate] {
        try withDatabase { db in
            let sql = """
            SELECT
                question_id,
                scope_type,
                scope_key,
                scope_label,
                question_text,
                question_type,
                why_now,
                evidence_json,
                source_kind,
                source_key,
                tags_json,
                priority_score,
                status,
                answer_snapshot_id,
                created_at,
                updated_at,
                expires_at
            FROM question_candidates
            WHERE scope_type = ? AND scope_key = ?
            ORDER BY priority_score DESC, updated_at DESC
            LIMIT ?;
            """
            return try withPreparedStatement(db: db, sql: sql) { statement in
                try bindText(scopeType.rawValue, at: 1, in: statement, sql: sql)
                try bindText(normalizedNonEmpty(scopeKey), at: 2, in: statement, sql: sql)
                try bindInteger(Int32(max(limit, 1)), at: 3, in: statement, sql: sql)
                var rows: [QuestionCandidate] = []
                while try stepSelect(statement, sql: sql, db: db) == SQLITE_ROW {
                    rows.append(try decodeQuestionCandidateRow(statement))
                }
                return rows
            }
        }
    }

    func questionCandidate(id: String) throws -> QuestionCandidate? {
        let normalizedQuestionID = id.trimmingCharacters(in: .whitespacesAndNewlines)
        guard !normalizedQuestionID.isEmpty else {
            return nil
        }
        return try withDatabase { db in
            let sql = """
            SELECT
                question_id,
                scope_type,
                scope_key,
                scope_label,
                question_text,
                question_type,
                why_now,
                evidence_json,
                source_kind,
                source_key,
                tags_json,
                priority_score,
                status,
                answer_snapshot_id,
                created_at,
                updated_at,
                expires_at
            FROM question_candidates
            WHERE question_id = ?
            LIMIT 1;
            """
            return try withPreparedStatement(db: db, sql: sql) { statement in
                try bindText(normalizedQuestionID, at: 1, in: statement, sql: sql)
                guard try stepSelect(statement, sql: sql, db: db) == SQLITE_ROW else {
                    return nil
                }
                return try decodeQuestionCandidateRow(statement)
            }
        }
    }

    @discardableResult
    func updateQuestionStatus(id: String, status: QuestionStatus, expiresAt: Date? = nil) throws -> Bool {
        let normalizedQuestionID = id.trimmingCharacters(in: .whitespacesAndNewlines)
        guard !normalizedQuestionID.isEmpty else {
            return false
        }
        return try withDatabase { db in
            let sql = """
            UPDATE question_candidates
            SET status = ?,
                updated_at = ?,
                expires_at = ?
            WHERE question_id = ?;
            """
            try withPreparedStatement(db: db, sql: sql) { statement in
                try bindText(status.rawValue, at: 1, in: statement, sql: sql)
                try bindText(nowTimestamp(), at: 2, in: statement, sql: sql)
                try bindOptionalText(expiresAt.map(timestampString), at: 3, in: statement, sql: sql)
                try bindText(normalizedQuestionID, at: 4, in: statement, sql: sql)
                try stepToDone(statement, sql: sql, db: db)
            }
            return sqlite3_changes(db) > 0
        }
    }

    @discardableResult
    func dismissQuestion(id: String) throws -> Bool {
        try updateQuestionStatus(id: id, status: .dismissed)
    }

    @discardableResult
    func snoozeQuestion(id: String, until: Date) throws -> Bool {
        try updateQuestionStatus(id: id, status: .snoozed, expiresAt: until)
    }

    private func upsertBeliefRecord(_ belief: BeliefRecord, db: OpaquePointer) throws {
        let beliefScope = try normalizedBeliefScope(belief.scope)
        let scopedEntityKey = scopedBeliefEntityKey(scope: beliefScope, entityKey: belief.entityKey)
        let normalizedStatement = try normalizedBeliefStatement(belief.statement)
        let beliefID = normalizedID(belief.id)
        let createdAt = normalizedTimestamp(belief.createdAt)
        let updatedAt = normalizedTimestamp(belief.updatedAt.isEmpty ? createdAt : belief.updatedAt)
        let evidence = try encodeEvidenceLinks(belief.evidenceLinks)
        let normalizedKind = normalizedBeliefMetadata(
            belief.beliefKind,
            fallback: belief.isManual ? "manual" : "second_order"
        )
        let normalizedLifecycle = normalizedBeliefMetadata(
            belief.lifecycle,
            fallback: belief.isManual ? "manual" : "candidate"
        )
        let supportCount = max(0, belief.supportCount)
        let contradictionCount = max(0, belief.contradictionCount)
        let lastEvidenceAt = belief.lastEvidenceAt.trimmingCharacters(in: .whitespacesAndNewlines).isEmpty
            ? updatedAt
            : belief.lastEvidenceAt.trimmingCharacters(in: .whitespacesAndNewlines)
        let columns = try tableColumnNames("beliefs", db: db)
        var insertColumns = [
            "belief_id",
            "scope",
            "entity_key",
            "statement",
            "confidence",
            "is_manual",
            "evidence_links_json",
            "created_at",
            "updated_at",
            "belief_kind",
            "lifecycle",
            "support_count",
            "contradiction_count",
            "last_evidence_at"
        ]
        var values: [SQLiteBindValue] = [
            .text(beliefID),
            .text(beliefScope.rawValue),
            .text(scopedEntityKey),
            .text(normalizedStatement),
            .double(belief.confidence),
            .int64(belief.isManual ? 1 : 0),
            .text(evidence),
            .text(createdAt),
            .text(updatedAt),
            .text(normalizedKind),
            .text(normalizedLifecycle),
            .int64(Int64(supportCount)),
            .int64(Int64(contradictionCount)),
            .text(lastEvidenceAt)
        ]
        var updateAssignments = [
            "scope = excluded.scope",
            "entity_key = excluded.entity_key",
            "statement = excluded.statement",
            "confidence = excluded.confidence",
            "is_manual = MAX(beliefs.is_manual, excluded.is_manual)",
            "evidence_links_json = excluded.evidence_links_json",
            "updated_at = excluded.updated_at",
            "belief_kind = CASE WHEN excluded.is_manual = 1 THEN excluded.belief_kind WHEN beliefs.is_manual = 1 THEN beliefs.belief_kind ELSE excluded.belief_kind END",
            "lifecycle = CASE WHEN excluded.is_manual = 1 THEN excluded.lifecycle WHEN beliefs.is_manual = 1 THEN beliefs.lifecycle ELSE excluded.lifecycle END",
            "support_count = MAX(COALESCE(beliefs.support_count, 0), excluded.support_count)",
            "contradiction_count = MAX(COALESCE(beliefs.contradiction_count, 0), excluded.contradiction_count)",
            "last_evidence_at = CASE WHEN TRIM(COALESCE(excluded.last_evidence_at, '')) = '' THEN beliefs.last_evidence_at ELSE excluded.last_evidence_at END"
        ]

        func appendLegacyColumn(_ name: String, value: SQLiteBindValue, update: String? = nil) {
            guard columns.contains(name) else { return }
            insertColumns.append(name)
            values.append(value)
            if let update {
                updateAssignments.append(update)
            }
        }

        appendLegacyColumn("domain", value: .text(""))
        appendLegacyColumn("belief_scope", value: .text(beliefScope.rawValue), update: "belief_scope = excluded.belief_scope")
        appendLegacyColumn("entity_type", value: .text(beliefScope.rawValue), update: "entity_type = excluded.entity_type")
        appendLegacyColumn("status", value: .text(normalizedLifecycle), update: "status = excluded.status")
        appendLegacyColumn("portability_bucket", value: .text(""))
        appendLegacyColumn("temporal_bucket", value: .text(""))

        let placeholders = Array(repeating: "?", count: insertColumns.count).joined(separator: ", ")
        let sql = """
        INSERT INTO beliefs (\(insertColumns.joined(separator: ", ")))
        VALUES (\(placeholders))
        ON CONFLICT DO UPDATE SET
            \(updateAssignments.joined(separator: ",\n            "));
        """
        try withPreparedStatement(db: db, sql: sql) { statement in
            try bindValues(values, in: statement, sql: sql)
            try stepToDone(statement, sql: sql, db: db)
        }
    }

    func upsertRoom(_ room: RoomRecord) throws {
        try withDatabase { db in
            try upsertRoomRecord(room, db: db)
        }
    }

    func upsertPerson(_ person: PersonRecord) throws {
        try withDatabase { db in
            try upsertPersonRecord(person, db: db)
        }
    }

    func upsertMessage(_ message: MessageRecord) throws {
        try withDatabase { db in
            try upsertMessageRecord(message, db: db)
        }
    }

    func upsertMessages(_ messages: [MessageRecord]) throws {
        guard messages.isEmpty == false else {
            return
        }
        try withDatabase { db in
            try execute(sql: "BEGIN IMMEDIATE TRANSACTION;", db: db)
            do {
                for message in messages {
                    try upsertMessageRecord(message, db: db)
                }
                try execute(sql: "COMMIT;", db: db)
            } catch {
                _ = sqlite3_exec(db, "ROLLBACK;", nil, nil, nil)
                throw error
            }
        }
    }

    func messageExists(messageID: String) throws -> Bool {
        try withDatabase { db in
            let sql = """
            SELECT 1
            FROM messages
            WHERE message_id = ?
            LIMIT 1;
            """
            return try withPreparedStatement(db: db, sql: sql) { statement in
                try bindText(normalizedNonEmpty(messageID), at: 1, in: statement, sql: sql)
                return try stepSelect(statement, sql: sql, db: db) == SQLITE_ROW
            }
        }
    }

    func latestMessage(roomID: String) throws -> MessageRecord? {
        try loadMessages(roomID: roomID, limit: 1).first
    }

    func loadWebexSyncState(conversationID: String) throws -> WebexConversationSyncStateRecord? {
        try withDatabase { db in
            let sql = """
            SELECT
                conversation_id,
                conversation_type,
                room_id,
                person_id,
                person_email,
                display_name,
                last_seen_message_id,
                last_seen_created,
                last_successful_sync_at,
                next_allowed_sync_at,
                polling_mode,
                consecutive_failure_count,
                last_error,
                last_error_at,
                updated_at
            FROM webex_sync_state
            WHERE conversation_id = ?
            LIMIT 1;
            """
            return try withPreparedStatement(db: db, sql: sql) { statement in
                try bindText(normalizedNonEmpty(conversationID), at: 1, in: statement, sql: sql)
                guard try stepSelect(statement, sql: sql, db: db) == SQLITE_ROW else {
                    return nil
                }
                return decodeWebexSyncStateRow(statement)
            }
        }
    }

    func loadWebexSyncStates(limit: Int = 10_000) throws -> [WebexConversationSyncStateRecord] {
        try withDatabase { db in
            let sql = """
            SELECT
                conversation_id,
                conversation_type,
                room_id,
                person_id,
                person_email,
                display_name,
                last_seen_message_id,
                last_seen_created,
                last_successful_sync_at,
                next_allowed_sync_at,
                polling_mode,
                consecutive_failure_count,
                last_error,
                last_error_at,
                updated_at
            FROM webex_sync_state
            ORDER BY updated_at DESC
            LIMIT ?;
            """
            return try withPreparedStatement(db: db, sql: sql) { statement in
                try bindInteger(Int32(max(limit, 1)), at: 1, in: statement, sql: sql)
                var rows: [WebexConversationSyncStateRecord] = []
                while try stepSelect(statement, sql: sql, db: db) == SQLITE_ROW {
                    rows.append(decodeWebexSyncStateRow(statement))
                }
                return rows
            }
        }
    }

    func upsertWebexSyncState(_ state: WebexConversationSyncStateRecord) throws {
        try withDatabase { db in
            let sql = """
            INSERT INTO webex_sync_state (
                conversation_id,
                conversation_type,
                room_id,
                person_id,
                person_email,
                display_name,
                last_seen_message_id,
                last_seen_created,
                last_successful_sync_at,
                next_allowed_sync_at,
                polling_mode,
                consecutive_failure_count,
                last_error,
                last_error_at,
                updated_at
            ) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
            ON CONFLICT(conversation_id) DO UPDATE SET
                conversation_type = excluded.conversation_type,
                room_id = excluded.room_id,
                person_id = excluded.person_id,
                person_email = excluded.person_email,
                display_name = excluded.display_name,
                last_seen_message_id = excluded.last_seen_message_id,
                last_seen_created = excluded.last_seen_created,
                last_successful_sync_at = excluded.last_successful_sync_at,
                next_allowed_sync_at = excluded.next_allowed_sync_at,
                polling_mode = excluded.polling_mode,
                consecutive_failure_count = excluded.consecutive_failure_count,
                last_error = excluded.last_error,
                last_error_at = excluded.last_error_at,
                updated_at = excluded.updated_at;
            """
            try withPreparedStatement(db: db, sql: sql) { statement in
                try bindText(normalizedNonEmpty(state.conversationID), at: 1, in: statement, sql: sql)
                try bindText(state.conversationType.rawValue, at: 2, in: statement, sql: sql)
                try bindText(normalizedNonEmpty(state.roomID), at: 3, in: statement, sql: sql)
                try bindOptionalText(normalizedOptionalText(state.personID), at: 4, in: statement, sql: sql)
                try bindOptionalText(normalizedOptionalText(state.personEmail), at: 5, in: statement, sql: sql)
                try bindOptionalText(normalizedOptionalText(state.title), at: 6, in: statement, sql: sql)
                try bindOptionalText(normalizedOptionalText(state.lastSeenMessageID), at: 7, in: statement, sql: sql)
                try bindOptionalText(normalizedOptionalText(state.lastSeenCreated), at: 8, in: statement, sql: sql)
                try bindOptionalText(normalizedOptionalText(state.lastSuccessfulSyncAt), at: 9, in: statement, sql: sql)
                try bindOptionalText(normalizedOptionalText(state.nextAllowedSyncAt), at: 10, in: statement, sql: sql)
                try bindText(state.pollingMode.rawValue, at: 11, in: statement, sql: sql)
                try bindInteger(Int32(max(state.consecutiveFailureCount, 0)), at: 12, in: statement, sql: sql)
                try bindOptionalText(normalizedOptionalText(state.lastError), at: 13, in: statement, sql: sql)
                try bindOptionalText(normalizedOptionalText(state.lastErrorAt), at: 14, in: statement, sql: sql)
                try bindText(normalizedTimestamp(state.updatedAt), at: 15, in: statement, sql: sql)
                try stepToDone(statement, sql: sql, db: db)
            }
        }
    }

    func loadRoom(roomID: String) throws -> RoomRecord? {
        try withDatabase { db in
            let sql = """
            SELECT room_id, title, updated_at
            FROM rooms
            WHERE room_id = ?
            LIMIT 1;
            """
            return try withPreparedStatement(db: db, sql: sql) { statement in
                try bindText(normalizedNonEmpty(roomID), at: 1, in: statement, sql: sql)
                guard try stepSelect(statement, sql: sql, db: db) == SQLITE_ROW else {
                    return nil
                }
                return RoomRecord(
                    id: columnText(statement, index: 0),
                    title: columnText(statement, index: 1),
                    updatedAt: columnText(statement, index: 2)
                )
            }
        }
    }

    func loadMessages(roomID: String, sinceTimestamp: String? = nil, limit: Int = 500) throws -> [MessageRecord] {
        try withDatabase { db in
            let hasSinceFilter = (sinceTimestamp?.trimmingCharacters(in: .whitespacesAndNewlines).isEmpty == false)
            let sql: String
            if hasSinceFilter {
                sql = """
                SELECT message_id, room_id, person_id, body, created_at, updated_at
                FROM messages
                WHERE room_id = ? AND created_at >= ?
                ORDER BY created_at DESC
                LIMIT ?;
                """
            } else {
                sql = """
                SELECT message_id, room_id, person_id, body, created_at, updated_at
                FROM messages
                WHERE room_id = ?
                ORDER BY created_at DESC
                LIMIT ?;
                """
            }

            return try withPreparedStatement(db: db, sql: sql) { statement in
                var bindIndex: Int32 = 1
                try bindText(normalizedNonEmpty(roomID), at: bindIndex, in: statement, sql: sql)
                bindIndex += 1
                if hasSinceFilter {
                    try bindText(normalizedTimestamp(sinceTimestamp ?? ""), at: bindIndex, in: statement, sql: sql)
                    bindIndex += 1
                }
                try bindInteger(Int32(max(limit, 1)), at: bindIndex, in: statement, sql: sql)

                var rows: [MessageRecord] = []
                while try stepSelect(statement, sql: sql, db: db) == SQLITE_ROW {
                    rows.append(
                        MessageRecord(
                            id: columnText(statement, index: 0),
                            roomID: columnText(statement, index: 1),
                            personID: columnOptionalText(statement, index: 2),
                            body: columnText(statement, index: 3),
                            createdAt: columnText(statement, index: 4),
                            updatedAt: columnText(statement, index: 5)
                        )
                    )
                }
                return rows
            }
        }
    }

    func loadMessages(personID: String, sinceTimestamp: String? = nil, limit: Int = 500) throws -> [MessageRecord] {
        try withDatabase { db in
            let hasSinceFilter = (sinceTimestamp?.trimmingCharacters(in: .whitespacesAndNewlines).isEmpty == false)
            let sql: String
            if hasSinceFilter {
                sql = """
                SELECT message_id, room_id, person_id, body, created_at, updated_at
                FROM messages
                WHERE person_id = ? AND created_at >= ?
                ORDER BY created_at DESC
                LIMIT ?;
                """
            } else {
                sql = """
                SELECT message_id, room_id, person_id, body, created_at, updated_at
                FROM messages
                WHERE person_id = ?
                ORDER BY created_at DESC
                LIMIT ?;
                """
            }

            return try withPreparedStatement(db: db, sql: sql) { statement in
                var bindIndex: Int32 = 1
                try bindText(normalizedNonEmpty(personID), at: bindIndex, in: statement, sql: sql)
                bindIndex += 1
                if hasSinceFilter {
                    try bindText(normalizedTimestamp(sinceTimestamp ?? ""), at: bindIndex, in: statement, sql: sql)
                    bindIndex += 1
                }
                try bindInteger(Int32(max(limit, 1)), at: bindIndex, in: statement, sql: sql)

                var rows: [MessageRecord] = []
                while try stepSelect(statement, sql: sql, db: db) == SQLITE_ROW {
                    rows.append(
                        MessageRecord(
                            id: columnText(statement, index: 0),
                            roomID: columnText(statement, index: 1),
                            personID: columnOptionalText(statement, index: 2),
                            body: columnText(statement, index: 3),
                            createdAt: columnText(statement, index: 4),
                            updatedAt: columnText(statement, index: 5)
                        )
                    )
                }
                return rows
            }
        }
    }

    func loadPeople(limit: Int = 10_000) throws -> [PersonRecord] {
        try withDatabase { db in
            let sql = """
            SELECT person_id, display_name, email, updated_at
            FROM people
            ORDER BY updated_at DESC
            LIMIT ?;
            """
            return try withPreparedStatement(db: db, sql: sql) { statement in
                try bindInteger(Int32(max(limit, 1)), at: 1, in: statement, sql: sql)
                var rows: [PersonRecord] = []
                while try stepSelect(statement, sql: sql, db: db) == SQLITE_ROW {
                    rows.append(
                        PersonRecord(
                            id: columnText(statement, index: 0),
                            displayName: columnText(statement, index: 1),
                            email: columnText(statement, index: 2),
                            updatedAt: columnText(statement, index: 3)
                        )
                    )
                }
                return rows
            }
        }
    }

    func upsertFile(_ file: FileRecord) throws {
        try withDatabase { db in
            try upsertFileRecord(file, db: db)
        }
    }

    func upsertFiles(_ files: [FileRecord]) throws {
        guard files.isEmpty == false else {
            return
        }
        try withDatabase { db in
            try execute(sql: "BEGIN IMMEDIATE TRANSACTION;", db: db)
            do {
                for file in files {
                    try upsertFileRecord(file, db: db)
                }
                try execute(sql: "COMMIT;", db: db)
            } catch {
                _ = sqlite3_exec(db, "ROLLBACK;", nil, nil, nil)
                throw error
            }
        }
    }

    func loadFiles(
        roomID: String,
        messageID: String? = nil,
        sinceTimestamp: String? = nil,
        limit: Int = 500
    ) throws -> [FileRecord] {
        try withDatabase { db in
            let hasMessageFilter = (messageID?.trimmingCharacters(in: .whitespacesAndNewlines).isEmpty == false)
            let hasSinceFilter = (sinceTimestamp?.trimmingCharacters(in: .whitespacesAndNewlines).isEmpty == false)
            var whereClauses = ["room_id = ?"]
            if hasMessageFilter {
                whereClauses.append("message_id = ?")
            }
            if hasSinceFilter {
                whereClauses.append("updated_at >= ?")
            }

            let sql = """
            SELECT file_id, message_id, room_id, filename, mime_type, file_size, updated_at
            FROM files
            WHERE \(whereClauses.joined(separator: " AND "))
            ORDER BY updated_at DESC
            LIMIT ?;
            """

            return try withPreparedStatement(db: db, sql: sql) { statement in
                var bindIndex: Int32 = 1
                try bindText(normalizedNonEmpty(roomID), at: bindIndex, in: statement, sql: sql)
                bindIndex += 1
                if hasMessageFilter {
                    try bindText(normalizedID(messageID ?? ""), at: bindIndex, in: statement, sql: sql)
                    bindIndex += 1
                }
                if hasSinceFilter {
                    try bindText(normalizedTimestamp(sinceTimestamp ?? ""), at: bindIndex, in: statement, sql: sql)
                    bindIndex += 1
                }
                try bindInteger(Int32(max(limit, 1)), at: bindIndex, in: statement, sql: sql)

                var rows: [FileRecord] = []
                while try stepSelect(statement, sql: sql, db: db) == SQLITE_ROW {
                    rows.append(
                        FileRecord(
                            id: columnText(statement, index: 0),
                            messageID: columnOptionalText(statement, index: 1),
                            roomID: columnText(statement, index: 2),
                            filename: columnText(statement, index: 3),
                            mimeType: columnText(statement, index: 4),
                            fileSize: Int(sqlite3_column_int64(statement, 5)),
                            updatedAt: columnText(statement, index: 6)
                        )
                    )
                }
                return rows
            }
        }
    }

    func upsertBeliefEvidence(_ evidence: BeliefEvidenceRecord) throws {
        try withDatabase { db in
            try upsertBeliefEvidenceRecord(evidence, db: db)
        }
    }

    func upsertBeliefEvidence(_ evidenceRecords: [BeliefEvidenceRecord]) throws {
        guard evidenceRecords.isEmpty == false else {
            return
        }
        try withDatabase { db in
            try execute(sql: "BEGIN IMMEDIATE TRANSACTION;", db: db)
            do {
                for evidence in evidenceRecords {
                    try upsertBeliefEvidenceRecord(evidence, db: db)
                }
                try execute(sql: "COMMIT;", db: db)
            } catch {
                _ = sqlite3_exec(db, "ROLLBACK;", nil, nil, nil)
                throw error
            }
        }
    }

    func loadBeliefEvidence(
        scope: KnowledgeBeliefScope,
        entityKey: String = "",
        sinceTimestamp: String? = nil,
        limit: Int = 500
    ) throws -> [BeliefEvidenceRecord] {
        try loadBeliefEvidence(
            scope: scope.rawValue,
            entityKey: scopedBeliefEntityKey(scope: scope, entityKey: entityKey),
            sinceTimestamp: sinceTimestamp,
            limit: limit
        )
    }

    func loadBeliefEvidence(
        scope: String,
        entityKey: String,
        sinceTimestamp: String? = nil,
        limit: Int = 500
    ) throws -> [BeliefEvidenceRecord] {
        let beliefScope = try normalizedBeliefScope(scope)
        let scopedEntityKey = scopedBeliefEntityKey(scope: beliefScope, entityKey: entityKey)
        return try withDatabase { db in
            let hasSinceFilter = (sinceTimestamp?.trimmingCharacters(in: .whitespacesAndNewlines).isEmpty == false)

            var whereClauses: [String] = []
            switch beliefScope {
            case .space:
                whereClauses.append("room_id = ?")
            case .person:
                whereClauses.append("person_id = ?")
            case .global:
                break
            }

            if hasSinceFilter {
                whereClauses.append("occurred_at >= ?")
            }

            let whereClause = whereClauses.isEmpty ? "" : "WHERE " + whereClauses.joined(separator: " AND ")
            let sql = """
            SELECT evidence_id, source, source_id, room_id, person_id, occurred_at, evidence_text
            FROM belief_evidence
            \(whereClause)
            ORDER BY occurred_at DESC, updated_at DESC
            LIMIT ?;
            """

            return try withPreparedStatement(db: db, sql: sql) { statement in
                var bindIndex: Int32 = 1
                switch beliefScope {
                case .space, .person:
                    try bindText(scopedEntityKey, at: bindIndex, in: statement, sql: sql)
                    bindIndex += 1
                case .global:
                    break
                }

                if hasSinceFilter {
                    try bindText(normalizedTimestamp(sinceTimestamp ?? ""), at: bindIndex, in: statement, sql: sql)
                    bindIndex += 1
                }
                try bindInteger(Int32(max(limit, 1)), at: bindIndex, in: statement, sql: sql)

                var rows: [BeliefEvidenceRecord] = []
                while try stepSelect(statement, sql: sql, db: db) == SQLITE_ROW {
                    rows.append(
                        BeliefEvidenceRecord(
                            id: columnText(statement, index: 0),
                            source: columnText(statement, index: 1),
                            sourceID: columnText(statement, index: 2),
                            roomID: columnText(statement, index: 3),
                            personID: columnOptionalText(statement, index: 4),
                            occurredAt: columnText(statement, index: 5),
                            text: columnText(statement, index: 6)
                        )
                    )
                }
                return rows
            }
        }
    }

    @discardableResult
    func deleteBeliefEvidence(source: String, sourceID: String) throws -> Bool {
        let normalizedSource = normalizedNonEmpty(source)
        let normalizedSourceID = normalizedNonEmpty(sourceID)
        return try withDatabase { db in
            let sql = "DELETE FROM belief_evidence WHERE source = ? AND source_id = ?;"
            try withPreparedStatement(db: db, sql: sql) { statement in
                try bindText(normalizedSource, at: 1, in: statement, sql: sql)
                try bindText(normalizedSourceID, at: 2, in: statement, sql: sql)
                try stepToDone(statement, sql: sql, db: db)
            }
            return sqlite3_changes(db) > 0
        }
    }

    @discardableResult
    func pruneBeliefEvidence(
        scope: KnowledgeBeliefScope,
        entityKey: String = "",
        beforeTimestamp: String? = nil
    ) throws -> Int {
        try pruneBeliefEvidence(
            scope: scope.rawValue,
            entityKey: scopedBeliefEntityKey(scope: scope, entityKey: entityKey),
            beforeTimestamp: beforeTimestamp
        )
    }

    @discardableResult
    func pruneBeliefEvidence(
        scope: String,
        entityKey: String,
        beforeTimestamp: String? = nil
    ) throws -> Int {
        let beliefScope = try normalizedBeliefScope(scope)
        let scopedEntityKey = scopedBeliefEntityKey(scope: beliefScope, entityKey: entityKey)
        let hasBeforeFilter = (beforeTimestamp?.trimmingCharacters(in: .whitespacesAndNewlines).isEmpty == false)

        // Prevent accidental global table wipe when no time bound is provided.
        if beliefScope == .global && hasBeforeFilter == false {
            return 0
        }

        var whereClauses: [String] = []
        switch beliefScope {
        case .space:
            whereClauses.append("room_id = ?")
        case .person:
            whereClauses.append("person_id = ?")
        case .global:
            break
        }
        if hasBeforeFilter {
            whereClauses.append("occurred_at < ?")
        }

        guard whereClauses.isEmpty == false else {
            return 0
        }

        let sql = "DELETE FROM belief_evidence WHERE \(whereClauses.joined(separator: " AND "));"
        return try withDatabase { db in
            try withPreparedStatement(db: db, sql: sql) { statement in
                var bindIndex: Int32 = 1
                switch beliefScope {
                case .space, .person:
                    try bindText(scopedEntityKey, at: bindIndex, in: statement, sql: sql)
                    bindIndex += 1
                case .global:
                    break
                }
                if hasBeforeFilter {
                    try bindText(normalizedTimestamp(beforeTimestamp ?? ""), at: bindIndex, in: statement, sql: sql)
                }
                try stepToDone(statement, sql: sql, db: db)
            }
            return Int(sqlite3_changes(db))
        }
    }

    private func upsertRoomRecord(_ room: RoomRecord, db: OpaquePointer) throws {
        let columns = try tableColumnNames("rooms", db: db)
        let roomID = normalizedNonEmpty(room.id)
        let updatedAt = normalizedTimestamp(room.updatedAt)
        var insertColumns = ["room_id", "title", "updated_at"]
        var values: [SQLiteBindValue] = [
            .text(roomID),
            .text(room.title.trimmingCharacters(in: .whitespacesAndNewlines)),
            .text(updatedAt)
        ]
        var updateAssignments = [
            """
            title = CASE
                WHEN TRIM(excluded.title) = '' THEN rooms.title
                ELSE excluded.title
            END
            """,
            "updated_at = excluded.updated_at"
        ]

        func appendLegacyColumn(_ name: String, value: SQLiteBindValue, update: String? = nil) {
            guard columns.contains(name) else { return }
            insertColumns.append(name)
            values.append(value)
            if let update {
                updateAssignments.append(update)
            }
        }

        appendLegacyColumn("room_type", value: .text("unknown"))
        appendLegacyColumn("raw_path", value: .text(""))
        appendLegacyColumn("first_seen_at", value: .text(updatedAt))
        appendLegacyColumn("last_seen_at", value: .text(updatedAt), update: "last_seen_at = excluded.last_seen_at")
        appendLegacyColumn("last_message_at", value: .text(updatedAt), update: "last_message_at = excluded.last_message_at")

        let placeholders = Array(repeating: "?", count: insertColumns.count).joined(separator: ", ")
        let sql = """
        INSERT INTO rooms (\(insertColumns.joined(separator: ", ")))
        VALUES (\(placeholders))
        ON CONFLICT(room_id) DO UPDATE SET
            \(updateAssignments.joined(separator: ",\n            "));
        """
        try withPreparedStatement(db: db, sql: sql) { statement in
            try bindValues(values, in: statement, sql: sql)
            try stepToDone(statement, sql: sql, db: db)
        }
    }

    private func upsertPersonRecord(_ person: PersonRecord, db: OpaquePointer) throws {
        let columns = try tableColumnNames("people", db: db)
        let personID = normalizedNonEmpty(person.id)
        let updatedAt = normalizedTimestamp(person.updatedAt)
        let displayName = person.displayName.trimmingCharacters(in: .whitespacesAndNewlines)
        let email = person.email.trimmingCharacters(in: .whitespacesAndNewlines)
        var insertColumns = ["person_id", "display_name", "email", "updated_at"]
        var values: [SQLiteBindValue] = [
            .text(personID),
            .text(displayName),
            .text(email),
            .text(updatedAt)
        ]
        var updateAssignments = [
            """
            display_name = CASE
                WHEN TRIM(excluded.display_name) = '' THEN people.display_name
                ELSE excluded.display_name
            END
            """,
            """
            email = CASE
                WHEN TRIM(excluded.email) = '' THEN people.email
                ELSE excluded.email
            END
            """,
            "updated_at = excluded.updated_at"
        ]

        func appendLegacyColumn(_ name: String, value: SQLiteBindValue, update: String? = nil) {
            guard columns.contains(name) else { return }
            insertColumns.append(name)
            values.append(value)
            if let update {
                updateAssignments.append(update)
            }
        }

        appendLegacyColumn("person_key", value: .text(personID))
        appendLegacyColumn("role_hint", value: .text(""))
        appendLegacyColumn("org_hint", value: .text(""))
        appendLegacyColumn("first_seen_at", value: .text(updatedAt))
        appendLegacyColumn("last_seen_at", value: .text(updatedAt), update: "last_seen_at = excluded.last_seen_at")

        let placeholders = Array(repeating: "?", count: insertColumns.count).joined(separator: ", ")
        let sql = """
        INSERT INTO people (\(insertColumns.joined(separator: ", ")))
        VALUES (\(placeholders))
        ON CONFLICT(person_id) DO UPDATE SET
            \(updateAssignments.joined(separator: ",\n            "));
        """
        try withPreparedStatement(db: db, sql: sql) { statement in
            try bindValues(values, in: statement, sql: sql)
            try stepToDone(statement, sql: sql, db: db)
        }
    }

    private func upsertMessageRecord(_ message: MessageRecord, db: OpaquePointer) throws {
        let roomID = normalizedNonEmpty(message.roomID)
        let now = nowTimestamp()
        try upsertRoomRecord(RoomRecord(id: roomID, title: "", updatedAt: now), db: db)

        if let personID = message.personID?.trimmingCharacters(in: .whitespacesAndNewlines), personID.isEmpty == false {
            try upsertPersonRecord(PersonRecord(id: personID, displayName: "", email: "", updatedAt: now), db: db)
        }

        let columns = try tableColumnNames("messages", db: db)
        let messageID = normalizedID(message.id)
        let createdAt = normalizedTimestamp(message.createdAt)
        let updatedAt = normalizedTimestamp(message.updatedAt.isEmpty ? createdAt : message.updatedAt)
        let personID = message.personID?.trimmingCharacters(in: .whitespacesAndNewlines)
        let body = message.body
        var insertColumns = ["message_id", "room_id", "person_id", "body", "created_at", "updated_at"]
        var values: [SQLiteBindValue] = [
            .text(messageID),
            .text(roomID),
            .optionalText(personID),
            .text(body),
            .text(createdAt),
            .text(updatedAt)
        ]
        var updateAssignments = [
            "room_id = excluded.room_id",
            "person_id = excluded.person_id",
            "body = excluded.body",
            "created_at = excluded.created_at",
            "updated_at = excluded.updated_at"
        ]

        func appendLegacyColumn(_ name: String, value: SQLiteBindValue, update: String? = nil) {
            guard columns.contains(name) else { return }
            insertColumns.append(name)
            values.append(value)
            if let update {
                updateAssignments.append(update)
            }
        }

        appendLegacyColumn("sender_key", value: .optionalText(personID), update: "sender_key = excluded.sender_key")
        appendLegacyColumn("parent_id", value: .optionalText(nil))
        appendLegacyColumn("text_raw", value: .text(body), update: "text_raw = excluded.text_raw")
        appendLegacyColumn("text_norm", value: .text(body), update: "text_norm = excluded.text_norm")
        appendLegacyColumn("transcript_path", value: .text(""))
        appendLegacyColumn("block_hash", value: .text(messageID), update: "block_hash = excluded.block_hash")
        appendLegacyColumn("indexed_at", value: .text(updatedAt), update: "indexed_at = excluded.indexed_at")

        let placeholders = Array(repeating: "?", count: insertColumns.count).joined(separator: ", ")
        let sql = """
        INSERT INTO messages (\(insertColumns.joined(separator: ", ")))
        VALUES (\(placeholders))
        ON CONFLICT(message_id) DO UPDATE SET
            \(updateAssignments.joined(separator: ",\n            "));
        """
        try withPreparedStatement(db: db, sql: sql) { statement in
            try bindValues(values, in: statement, sql: sql)
            try stepToDone(statement, sql: sql, db: db)
        }
    }

    private func upsertFileRecord(_ file: FileRecord, db: OpaquePointer) throws {
        let roomID = normalizedNonEmpty(file.roomID)
        let updatedAt = normalizedTimestamp(file.updatedAt)
        try upsertRoomRecord(RoomRecord(id: roomID, title: "", updatedAt: updatedAt), db: db)

        if let messageID = file.messageID?.trimmingCharacters(in: .whitespacesAndNewlines), messageID.isEmpty == false {
            let messageStub = MessageRecord(
                id: messageID,
                roomID: roomID,
                personID: nil,
                body: "",
                createdAt: updatedAt,
                updatedAt: updatedAt
            )
            try upsertMessageRecord(messageStub, db: db)
        }

        let columns = try tableColumnNames("files", db: db)
        let fileID = normalizedID(file.id)
        var insertColumns = ["file_id", "message_id", "room_id", "filename", "mime_type", "file_size", "updated_at"]
        var values: [SQLiteBindValue] = [
            .text(fileID),
            .optionalText(file.messageID?.trimmingCharacters(in: .whitespacesAndNewlines)),
            .text(roomID),
            .text(file.filename.trimmingCharacters(in: .whitespacesAndNewlines)),
            .text(file.mimeType.trimmingCharacters(in: .whitespacesAndNewlines)),
            .int64(Int64(max(file.fileSize, 0))),
            .text(updatedAt)
        ]
        var updateAssignments = [
            "message_id = excluded.message_id",
            "room_id = excluded.room_id",
            "filename = excluded.filename",
            "mime_type = excluded.mime_type",
            "file_size = excluded.file_size",
            "updated_at = excluded.updated_at"
        ]

        func appendLegacyColumn(_ name: String, value: SQLiteBindValue, update: String? = nil) {
            guard columns.contains(name) else { return }
            insertColumns.append(name)
            values.append(value)
            if let update {
                updateAssignments.append(update)
            }
        }

        appendLegacyColumn("file_key", value: .text(fileID))
        appendLegacyColumn("sender_key", value: .optionalText(nil))
        appendLegacyColumn("created_at", value: .text(updatedAt))
        appendLegacyColumn("local_path", value: .text(file.filename.trimmingCharacters(in: .whitespacesAndNewlines)))
        appendLegacyColumn("sha256", value: .text(""))
        appendLegacyColumn("extracted_text_path", value: .text(""))
        appendLegacyColumn("summary", value: .text(""), update: "summary = excluded.summary")
        appendLegacyColumn("indexed_at", value: .text(updatedAt), update: "indexed_at = excluded.indexed_at")

        let placeholders = Array(repeating: "?", count: insertColumns.count).joined(separator: ", ")
        let sql = """
        INSERT INTO files (\(insertColumns.joined(separator: ", ")))
        VALUES (\(placeholders))
        ON CONFLICT(file_id) DO UPDATE SET
            \(updateAssignments.joined(separator: ",\n            "));
        """
        try withPreparedStatement(db: db, sql: sql) { statement in
            try bindValues(values, in: statement, sql: sql)
            try stepToDone(statement, sql: sql, db: db)
        }
    }

    private func upsertBeliefEvidenceRecord(_ evidence: BeliefEvidenceRecord, db: OpaquePointer) throws {
        let roomID = normalizedNonEmpty(evidence.roomID)
        let occurredAt = normalizedTimestamp(evidence.occurredAt)
        let updatedAt = nowTimestamp()
        try upsertRoomRecord(RoomRecord(id: roomID, title: "", updatedAt: updatedAt), db: db)

        let normalizedPersonID = evidence.personID?.trimmingCharacters(in: .whitespacesAndNewlines)
        if let normalizedPersonID, normalizedPersonID.isEmpty == false {
            try upsertPersonRecord(
                PersonRecord(id: normalizedPersonID, displayName: "", email: "", updatedAt: updatedAt),
                db: db
            )
        }

        let normalizedSourceID: String = {
            let trimmedSourceID = evidence.sourceID.trimmingCharacters(in: .whitespacesAndNewlines)
            if trimmedSourceID.isEmpty == false {
                return trimmedSourceID
            }
            let fallbackID = evidence.id.trimmingCharacters(in: .whitespacesAndNewlines)
            return fallbackID.isEmpty ? UUID().uuidString : fallbackID
        }()

        let columns = try tableColumnNames("belief_evidence", db: db)
        let evidenceID = normalizedID(evidence.id)
        let source = normalizedNonEmpty(evidence.source)
        var insertColumns = [
            "evidence_id",
            "source",
            "source_id",
            "room_id",
            "person_id",
            "occurred_at",
            "evidence_text",
            "updated_at"
        ]
        var values: [SQLiteBindValue] = [
            .text(evidenceID),
            .text(source),
            .text(normalizedSourceID),
            .text(roomID),
            .optionalText(normalizedPersonID),
            .text(occurredAt),
            .text(evidence.text),
            .text(updatedAt)
        ]
        var updateAssignments = [
            "evidence_id = excluded.evidence_id",
            "room_id = excluded.room_id",
            "person_id = excluded.person_id",
            "occurred_at = excluded.occurred_at",
            "evidence_text = excluded.evidence_text",
            "updated_at = excluded.updated_at"
        ]

        func appendLegacyColumn(_ name: String, value: SQLiteBindValue, update: String? = nil) {
            guard columns.contains(name) else { return }
            insertColumns.append(name)
            values.append(value)
            if let update {
                updateAssignments.append(update)
            }
        }

        appendLegacyColumn("belief_id", value: .text("evidence-\(evidenceID)"))
        appendLegacyColumn("source_type", value: .text(source), update: "source_type = excluded.source_type")
        appendLegacyColumn("direction", value: .text("support"), update: "direction = excluded.direction")
        appendLegacyColumn("weight", value: .double(1.0), update: "weight = excluded.weight")
        appendLegacyColumn("note", value: .text(evidence.text), update: "note = excluded.note")

        let placeholders = Array(repeating: "?", count: insertColumns.count).joined(separator: ", ")
        let sql = """
        INSERT INTO belief_evidence (\(insertColumns.joined(separator: ", ")))
        VALUES (\(placeholders))
        ON CONFLICT(source, source_id) DO UPDATE SET
            \(updateAssignments.joined(separator: ",\n            "));
        """
        try withPreparedStatement(db: db, sql: sql) { statement in
            try bindValues(values, in: statement, sql: sql)
            try stepToDone(statement, sql: sql, db: db)
        }
    }

    private func upsertQuestionCandidateRecord(_ candidate: QuestionCandidate, db: OpaquePointer) throws {
        let questionID = normalizedID(candidate.id)
        let scopeKey = normalizedNonEmpty(candidate.scopeKey)
        let scopeLabel = candidate.scopeLabel.trimmingCharacters(in: .whitespacesAndNewlines)
        let questionText = candidate.questionText.trimmingCharacters(in: .whitespacesAndNewlines)
        let questionType = normalizedNonEmpty(candidate.questionType)
        let whyNow = candidate.whyNow.trimmingCharacters(in: .whitespacesAndNewlines)
        let sourceKind = candidate.sourceKind.trimmingCharacters(in: .whitespacesAndNewlines)
        let sourceKey = candidate.sourceKey.trimmingCharacters(in: .whitespacesAndNewlines)
        let evidenceJSON = try encodeQuestionEvidence(candidate.evidence)
        let tagsJSON = try encodeQuestionTags(candidate.tags)
        let answerSnapshotID = candidate.answerSnapshotId?.trimmingCharacters(in: .whitespacesAndNewlines) ?? ""
        let createdAt = timestampString(candidate.createdAt)
        let updatedAt = timestampString(candidate.updatedAt)

        let sql = """
        INSERT INTO question_candidates (
            question_id,
            scope_type,
            scope_key,
            scope_label,
            question_text,
            question_type,
            why_now,
            evidence_json,
            source_kind,
            source_key,
            tags_json,
            priority_score,
            status,
            answer_snapshot_id,
            created_at,
            updated_at,
            expires_at
        ) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
        ON CONFLICT(question_id) DO UPDATE SET
            scope_label = excluded.scope_label,
            question_text = excluded.question_text,
            question_type = excluded.question_type,
            why_now = excluded.why_now,
            evidence_json = excluded.evidence_json,
            source_kind = excluded.source_kind,
            source_key = excluded.source_key,
            tags_json = excluded.tags_json,
            priority_score = excluded.priority_score,
            status = CASE
                WHEN question_candidates.status IN ('answered', 'dismissed') THEN question_candidates.status
                ELSE excluded.status
            END,
            answer_snapshot_id = CASE
                WHEN TRIM(question_candidates.answer_snapshot_id) != '' THEN question_candidates.answer_snapshot_id
                ELSE excluded.answer_snapshot_id
            END,
            updated_at = excluded.updated_at,
            expires_at = CASE
                WHEN question_candidates.status = 'snoozed'
                    AND question_candidates.expires_at IS NOT NULL
                    AND question_candidates.expires_at > excluded.updated_at
                THEN question_candidates.expires_at
                ELSE excluded.expires_at
            END;
        """
        try withPreparedStatement(db: db, sql: sql) { statement in
            try bindText(questionID, at: 1, in: statement, sql: sql)
            try bindText(candidate.scopeType.rawValue, at: 2, in: statement, sql: sql)
            try bindText(scopeKey, at: 3, in: statement, sql: sql)
            try bindText(scopeLabel, at: 4, in: statement, sql: sql)
            try bindText(questionText, at: 5, in: statement, sql: sql)
            try bindText(questionType, at: 6, in: statement, sql: sql)
            try bindText(whyNow, at: 7, in: statement, sql: sql)
            try bindText(evidenceJSON, at: 8, in: statement, sql: sql)
            try bindText(sourceKind, at: 9, in: statement, sql: sql)
            try bindText(sourceKey, at: 10, in: statement, sql: sql)
            try bindText(tagsJSON, at: 11, in: statement, sql: sql)
            try bindDouble(candidate.priorityScore, at: 12, in: statement, sql: sql)
            try bindText(candidate.status.rawValue, at: 13, in: statement, sql: sql)
            try bindText(answerSnapshotID, at: 14, in: statement, sql: sql)
            try bindText(createdAt, at: 15, in: statement, sql: sql)
            try bindText(updatedAt, at: 16, in: statement, sql: sql)
            try bindOptionalText(candidate.expiresAt.map(timestampString), at: 17, in: statement, sql: sql)
            try stepToDone(statement, sql: sql, db: db)
        }
    }

    private func withDatabase<T>(_ body: (OpaquePointer) throws -> T) throws -> T {
        try database.withOpenConnection { db in
            try applyPendingMigrations(db: db)
            try ensureCoreSchemaCompatibility(db: db)
            try ensureTopicSchemaCompatibility(db: db)
            try ensureBeliefSchemaCompatibility(db: db)
            try ensureBeliefEvidenceSchemaCompatibility(db: db)
            return try body(db)
        }
    }

    private func applyPendingMigrations(db: OpaquePointer) throws {
        try execute(
            sql: """
            CREATE TABLE IF NOT EXISTS schema_migrations (
                version INTEGER PRIMARY KEY,
                applied_at TEXT NOT NULL
            );
            """,
            db: db
        )

        let currentVersion = try currentSchemaVersion(db: db)
        let latestVersion = migrations.last?.version ?? 0
        if currentVersion > latestVersion {
            throw KnowledgeStoreError.unsupportedSchemaVersion(current: currentVersion, latest: latestVersion)
        }

        for migration in migrations where migration.version > currentVersion {
            try execute(sql: "BEGIN IMMEDIATE TRANSACTION;", db: db)
            do {
                for sql in migration.statements {
                    try execute(sql: sql, db: db)
                }
                try withPreparedStatement(
                    db: db,
                    sql: "INSERT INTO schema_migrations(version, applied_at) VALUES (?, ?);"
                ) { statement in
                    try bindInteger(migration.version, at: 1, in: statement, sql: "INSERT INTO schema_migrations")
                    try bindText(nowTimestamp(), at: 2, in: statement, sql: "INSERT INTO schema_migrations")
                    try stepToDone(statement, sql: "INSERT INTO schema_migrations", db: db)
                }
                try execute(sql: "COMMIT;", db: db)
            } catch {
                _ = sqlite3_exec(db, "ROLLBACK;", nil, nil, nil)
                throw error
            }
        }
    }

    private func ensureCoreSchemaCompatibility(db: OpaquePointer) throws {
        try ensureRoomSchemaCompatibility(db: db)
        try ensurePeopleSchemaCompatibility(db: db)
        try ensureMessageSchemaCompatibility(db: db)
        try ensureFileSchemaCompatibility(db: db)
    }

    private func ensureRoomSchemaCompatibility(db: OpaquePointer) throws {
        var columns = try tableColumnNames("rooms", db: db)
        guard columns.isEmpty == false else {
            return
        }

        try execute(sql: "BEGIN IMMEDIATE TRANSACTION;", db: db)
        do {
            let requiredColumns: [(name: String, definition: String)] = [
                ("title", "title TEXT NOT NULL DEFAULT ''"),
                ("updated_at", "updated_at TEXT NOT NULL DEFAULT ''")
            ]
            for column in requiredColumns where columns.contains(column.name) == false {
                try execute(sql: "ALTER TABLE rooms ADD COLUMN \(column.definition);", db: db)
                columns.insert(column.name)
            }

            let updatedAtFallback: String
            if columns.contains("last_message_at"), columns.contains("last_seen_at") {
                updatedAtFallback = "COALESCE(NULLIF(TRIM(last_message_at), ''), NULLIF(TRIM(last_seen_at), ''), CURRENT_TIMESTAMP)"
            } else if columns.contains("last_seen_at") {
                updatedAtFallback = "COALESCE(NULLIF(TRIM(last_seen_at), ''), CURRENT_TIMESTAMP)"
            } else {
                updatedAtFallback = "CURRENT_TIMESTAMP"
            }

            try execute(
                sql: """
                UPDATE rooms
                SET title = CASE WHEN TRIM(COALESCE(title, '')) = '' THEN room_id ELSE title END,
                    updated_at = CASE WHEN TRIM(COALESCE(updated_at, '')) = '' THEN \(updatedAtFallback) ELSE updated_at END;
                """,
                db: db
            )
            try execute(sql: "CREATE INDEX IF NOT EXISTS idx_rooms_updated_at ON rooms(updated_at DESC);", db: db)
            try execute(sql: "COMMIT;", db: db)
        } catch {
            _ = sqlite3_exec(db, "ROLLBACK;", nil, nil, nil)
            throw error
        }
    }

    private func ensurePeopleSchemaCompatibility(db: OpaquePointer) throws {
        var columns = try tableColumnNames("people", db: db)
        guard columns.isEmpty == false else {
            return
        }

        try execute(sql: "BEGIN IMMEDIATE TRANSACTION;", db: db)
        do {
            let requiredColumns: [(name: String, definition: String)] = [
                ("person_id", "person_id TEXT NOT NULL DEFAULT ''"),
                ("display_name", "display_name TEXT NOT NULL DEFAULT ''"),
                ("email", "email TEXT NOT NULL DEFAULT ''"),
                ("updated_at", "updated_at TEXT NOT NULL DEFAULT ''")
            ]
            for column in requiredColumns where columns.contains(column.name) == false {
                try execute(sql: "ALTER TABLE people ADD COLUMN \(column.definition);", db: db)
                columns.insert(column.name)
            }

            let personIDFallback = columns.contains("person_key")
                ? "COALESCE(NULLIF(TRIM(person_key), ''), NULLIF(TRIM(email), ''), LOWER(HEX(RANDOMBLOB(16))))"
                : "COALESCE(NULLIF(TRIM(email), ''), LOWER(HEX(RANDOMBLOB(16))))"
            let updatedAtFallback: String
            if columns.contains("last_seen_at"), columns.contains("first_seen_at") {
                updatedAtFallback = "COALESCE(NULLIF(TRIM(last_seen_at), ''), NULLIF(TRIM(first_seen_at), ''), CURRENT_TIMESTAMP)"
            } else if columns.contains("last_seen_at") {
                updatedAtFallback = "COALESCE(NULLIF(TRIM(last_seen_at), ''), CURRENT_TIMESTAMP)"
            } else {
                updatedAtFallback = "CURRENT_TIMESTAMP"
            }

            try execute(
                sql: """
                UPDATE people
                SET person_id = CASE WHEN TRIM(COALESCE(person_id, '')) = '' THEN \(personIDFallback) ELSE person_id END,
                    display_name = CASE WHEN TRIM(COALESCE(display_name, '')) = '' THEN COALESCE(NULLIF(TRIM(email), ''), person_id) ELSE display_name END,
                    email = COALESCE(email, ''),
                    updated_at = CASE WHEN TRIM(COALESCE(updated_at, '')) = '' THEN \(updatedAtFallback) ELSE updated_at END;
                """,
                db: db
            )
            try execute(
                sql: """
                UPDATE people
                SET person_id = person_id || '#' || rowid
                WHERE rowid IN (
                    SELECT rowid
                    FROM (
                        SELECT rowid, COUNT(*) OVER (PARTITION BY person_id) AS duplicate_count
                        FROM people
                    )
                    WHERE duplicate_count > 1
                );
                """,
                db: db
            )
            try execute(sql: "CREATE UNIQUE INDEX IF NOT EXISTS idx_people_person_id_unique ON people(person_id);", db: db)
            try execute(sql: "CREATE INDEX IF NOT EXISTS idx_people_email ON people(email);", db: db)
            try execute(sql: "COMMIT;", db: db)
        } catch {
            _ = sqlite3_exec(db, "ROLLBACK;", nil, nil, nil)
            throw error
        }
    }

    private func ensureMessageSchemaCompatibility(db: OpaquePointer) throws {
        var columns = try tableColumnNames("messages", db: db)
        guard columns.isEmpty == false else {
            return
        }

        try execute(sql: "BEGIN IMMEDIATE TRANSACTION;", db: db)
        do {
            let requiredColumns: [(name: String, definition: String)] = [
                ("person_id", "person_id TEXT"),
                ("body", "body TEXT NOT NULL DEFAULT ''"),
                ("updated_at", "updated_at TEXT NOT NULL DEFAULT ''")
            ]
            for column in requiredColumns where columns.contains(column.name) == false {
                try execute(sql: "ALTER TABLE messages ADD COLUMN \(column.definition);", db: db)
                columns.insert(column.name)
            }

            let personFallback = columns.contains("sender_key") ? "NULLIF(TRIM(sender_key), '')" : "NULL"
            let bodyFallback: String
            if columns.contains("text_norm"), columns.contains("text_raw") {
                bodyFallback = "COALESCE(NULLIF(TRIM(text_norm), ''), NULLIF(TRIM(text_raw), ''), '')"
            } else if columns.contains("text_raw") {
                bodyFallback = "COALESCE(NULLIF(TRIM(text_raw), ''), '')"
            } else {
                bodyFallback = "''"
            }
            let updatedAtFallback: String
            if columns.contains("indexed_at"), columns.contains("created_at") {
                updatedAtFallback = "COALESCE(NULLIF(TRIM(indexed_at), ''), NULLIF(TRIM(created_at), ''), CURRENT_TIMESTAMP)"
            } else if columns.contains("indexed_at") {
                updatedAtFallback = "COALESCE(NULLIF(TRIM(indexed_at), ''), CURRENT_TIMESTAMP)"
            } else if columns.contains("created_at") {
                updatedAtFallback = "COALESCE(NULLIF(TRIM(created_at), ''), CURRENT_TIMESTAMP)"
            } else {
                updatedAtFallback = "CURRENT_TIMESTAMP"
            }

            try execute(
                sql: """
                UPDATE messages
                SET person_id = CASE WHEN TRIM(COALESCE(person_id, '')) = '' THEN \(personFallback) ELSE person_id END,
                    body = CASE WHEN TRIM(COALESCE(body, '')) = '' THEN \(bodyFallback) ELSE body END,
                    updated_at = CASE WHEN TRIM(COALESCE(updated_at, '')) = '' THEN \(updatedAtFallback) ELSE updated_at END;
                """,
                db: db
            )
            try execute(sql: "CREATE INDEX IF NOT EXISTS idx_messages_room_created ON messages(room_id, created_at DESC);", db: db)
            try execute(sql: "CREATE INDEX IF NOT EXISTS idx_messages_person_created ON messages(person_id, created_at DESC);", db: db)
            try execute(sql: "COMMIT;", db: db)
        } catch {
            _ = sqlite3_exec(db, "ROLLBACK;", nil, nil, nil)
            throw error
        }
    }

    private func ensureFileSchemaCompatibility(db: OpaquePointer) throws {
        var columns = try tableColumnNames("files", db: db)
        guard columns.isEmpty == false else {
            return
        }

        try execute(sql: "BEGIN IMMEDIATE TRANSACTION;", db: db)
        do {
            let requiredColumns: [(name: String, definition: String)] = [
                ("file_id", "file_id TEXT NOT NULL DEFAULT ''"),
                ("filename", "filename TEXT NOT NULL DEFAULT ''"),
                ("file_size", "file_size INTEGER NOT NULL DEFAULT 0"),
                ("updated_at", "updated_at TEXT NOT NULL DEFAULT ''")
            ]
            for column in requiredColumns where columns.contains(column.name) == false {
                try execute(sql: "ALTER TABLE files ADD COLUMN \(column.definition);", db: db)
                columns.insert(column.name)
            }

            let fileIDFallback = columns.contains("file_key")
                ? "COALESCE(NULLIF(TRIM(file_key), ''), LOWER(HEX(RANDOMBLOB(16))))"
                : "LOWER(HEX(RANDOMBLOB(16)))"
            let filenameFallback: String
            if columns.contains("local_path"), columns.contains("file_key") {
                filenameFallback = "COALESCE(NULLIF(TRIM(local_path), ''), NULLIF(TRIM(file_key), ''), '')"
            } else if columns.contains("local_path") {
                filenameFallback = "COALESCE(NULLIF(TRIM(local_path), ''), '')"
            } else {
                filenameFallback = "''"
            }
            let updatedAtFallback: String
            if columns.contains("indexed_at"), columns.contains("created_at") {
                updatedAtFallback = "COALESCE(NULLIF(TRIM(indexed_at), ''), NULLIF(TRIM(created_at), ''), CURRENT_TIMESTAMP)"
            } else if columns.contains("indexed_at") {
                updatedAtFallback = "COALESCE(NULLIF(TRIM(indexed_at), ''), CURRENT_TIMESTAMP)"
            } else if columns.contains("created_at") {
                updatedAtFallback = "COALESCE(NULLIF(TRIM(created_at), ''), CURRENT_TIMESTAMP)"
            } else {
                updatedAtFallback = "CURRENT_TIMESTAMP"
            }

            try execute(
                sql: """
                UPDATE files
                SET file_id = CASE WHEN TRIM(COALESCE(file_id, '')) = '' THEN \(fileIDFallback) ELSE file_id END,
                    filename = CASE WHEN TRIM(COALESCE(filename, '')) = '' THEN \(filenameFallback) ELSE filename END,
                    updated_at = CASE WHEN TRIM(COALESCE(updated_at, '')) = '' THEN \(updatedAtFallback) ELSE updated_at END;
                """,
                db: db
            )
            try execute(
                sql: """
                UPDATE files
                SET file_id = file_id || '#' || rowid
                WHERE rowid IN (
                    SELECT rowid
                    FROM (
                        SELECT rowid, COUNT(*) OVER (PARTITION BY file_id) AS duplicate_count
                        FROM files
                    )
                    WHERE duplicate_count > 1
                );
                """,
                db: db
            )
            try execute(sql: "CREATE UNIQUE INDEX IF NOT EXISTS idx_files_file_id_unique ON files(file_id);", db: db)
            try execute(sql: "CREATE INDEX IF NOT EXISTS idx_files_room_updated ON files(room_id, updated_at DESC);", db: db)
            try execute(sql: "COMMIT;", db: db)
        } catch {
            _ = sqlite3_exec(db, "ROLLBACK;", nil, nil, nil)
            throw error
        }
    }

    private func ensureTopicSchemaCompatibility(db: OpaquePointer) throws {
        var columns = try tableColumnNames("topics", db: db)
        guard columns.isEmpty == false else {
            return
        }

        try execute(sql: "BEGIN IMMEDIATE TRANSACTION;", db: db)
        do {
            let requiredColumns: [(name: String, definition: String)] = [
                ("focus_kind", "focus_kind TEXT NOT NULL DEFAULT 'legacy'"),
                ("scope", "scope TEXT NOT NULL DEFAULT 'legacy'"),
                ("entity_key", "entity_key TEXT NOT NULL DEFAULT '__legacy__'"),
                ("topic_key", "topic_key TEXT NOT NULL DEFAULT '__legacy__'"),
                ("title", "title TEXT NOT NULL DEFAULT ''"),
                ("summary", "summary TEXT NOT NULL DEFAULT ''"),
                ("so_what", "so_what TEXT NOT NULL DEFAULT ''"),
                ("source_label", "source_label TEXT NOT NULL DEFAULT ''"),
                ("score", "score REAL"),
                ("generated_at", "generated_at TEXT NOT NULL DEFAULT ''"),
                ("updated_at", "updated_at TEXT NOT NULL DEFAULT ''")
            ]

            for column in requiredColumns where columns.contains(column.name) == false {
                try execute(sql: "ALTER TABLE topics ADD COLUMN \(column.definition);", db: db)
                columns.insert(column.name)
            }

            let titleFallback: String
            if columns.contains("label") {
                titleFallback = "COALESCE(NULLIF(TRIM(label), ''), NULLIF(TRIM(topic_id), ''), 'Legacy topic')"
            } else {
                titleFallback = "COALESCE(NULLIF(TRIM(topic_id), ''), 'Legacy topic')"
            }

            let summaryFallback = columns.contains("description")
                ? "COALESCE(description, '')"
                : "''"
            let generatedAtFallback = columns.contains("first_seen_at")
                ? "COALESCE(NULLIF(TRIM(first_seen_at), ''), CURRENT_TIMESTAMP)"
                : "CURRENT_TIMESTAMP"
            let updatedAtFallback: String
            if columns.contains("last_seen_at"), columns.contains("first_seen_at") {
                updatedAtFallback = "COALESCE(NULLIF(TRIM(last_seen_at), ''), NULLIF(TRIM(first_seen_at), ''), CURRENT_TIMESTAMP)"
            } else if columns.contains("last_seen_at") {
                updatedAtFallback = "COALESCE(NULLIF(TRIM(last_seen_at), ''), CURRENT_TIMESTAMP)"
            } else {
                updatedAtFallback = "CURRENT_TIMESTAMP"
            }

            var assignments = [
                "focus_kind = CASE WHEN TRIM(COALESCE(focus_kind, '')) = '' THEN 'legacy' ELSE focus_kind END",
                "scope = CASE WHEN TRIM(COALESCE(scope, '')) = '' THEN 'legacy' ELSE scope END",
                "entity_key = CASE WHEN TRIM(COALESCE(entity_key, '')) = '' THEN '__legacy__' ELSE entity_key END",
                """
                topic_key = CASE
                    WHEN TRIM(COALESCE(topic_key, '')) = '' OR topic_key = '__legacy__'
                    THEN COALESCE(NULLIF(TRIM(topic_id), ''), LOWER(HEX(RANDOMBLOB(16))))
                    ELSE topic_key
                END
                """,
                "title = CASE WHEN TRIM(COALESCE(title, '')) = '' THEN \(titleFallback) ELSE title END",
                "summary = CASE WHEN TRIM(COALESCE(summary, '')) = '' THEN \(summaryFallback) ELSE summary END",
                "generated_at = CASE WHEN TRIM(COALESCE(generated_at, '')) = '' THEN \(generatedAtFallback) ELSE generated_at END",
                "updated_at = CASE WHEN TRIM(COALESCE(updated_at, '')) = '' THEN \(updatedAtFallback) ELSE updated_at END"
            ]
            if columns.contains("confidence") {
                assignments.append("score = COALESCE(score, confidence)")
            }

            try execute(
                sql: """
                UPDATE topics
                SET \(assignments.joined(separator: ",\n                    "));
                """,
                db: db
            )
            try execute(
                sql: "CREATE UNIQUE INDEX IF NOT EXISTS idx_topics_scope_unique ON topics(focus_kind, scope, entity_key, topic_key);",
                db: db
            )
            try execute(
                sql: "CREATE INDEX IF NOT EXISTS idx_topics_scope ON topics(focus_kind, scope, entity_key, updated_at DESC);",
                db: db
            )
            try execute(sql: "COMMIT;", db: db)
        } catch {
            _ = sqlite3_exec(db, "ROLLBACK;", nil, nil, nil)
            throw error
        }
    }

    private func ensureBeliefSchemaCompatibility(db: OpaquePointer) throws {
        var columns = try tableColumnNames("beliefs", db: db)
        guard columns.isEmpty == false else {
            return
        }

        try execute(sql: "BEGIN IMMEDIATE TRANSACTION;", db: db)
        do {
            let requiredColumns: [(name: String, definition: String)] = [
                ("scope", "scope TEXT NOT NULL DEFAULT 'global'"),
                ("is_manual", "is_manual INTEGER NOT NULL DEFAULT 0"),
                ("evidence_links_json", "evidence_links_json TEXT NOT NULL DEFAULT '[]'"),
                ("belief_kind", "belief_kind TEXT NOT NULL DEFAULT 'second_order'"),
                ("lifecycle", "lifecycle TEXT NOT NULL DEFAULT 'candidate'"),
                ("support_count", "support_count INTEGER NOT NULL DEFAULT 1"),
                ("contradiction_count", "contradiction_count INTEGER NOT NULL DEFAULT 0"),
                ("last_evidence_at", "last_evidence_at TEXT NOT NULL DEFAULT ''")
            ]

            for column in requiredColumns where columns.contains(column.name) == false {
                try execute(sql: "ALTER TABLE beliefs ADD COLUMN \(column.definition);", db: db)
                columns.insert(column.name)
            }

            let legacyScopeExpression = columns.contains("belief_scope")
                ? "CASE LOWER(TRIM(COALESCE(belief_scope, ''))) WHEN 'person' THEN 'person' WHEN 'space' THEN 'space' ELSE 'global' END"
                : "CASE LOWER(TRIM(COALESCE(scope, ''))) WHEN 'person' THEN 'person' WHEN 'space' THEN 'space' ELSE 'global' END"

            try execute(
                sql: """
                UPDATE beliefs
                SET scope = CASE
                        WHEN TRIM(COALESCE(scope, '')) = '' THEN \(legacyScopeExpression)
                        WHEN LOWER(TRIM(scope)) = 'global' AND \(legacyScopeExpression) IN ('person', 'space') THEN \(legacyScopeExpression)
                        WHEN LOWER(TRIM(scope)) IN ('global', 'person', 'space') THEN LOWER(TRIM(scope))
                        ELSE 'global'
                    END,
                    entity_key = CASE
                        WHEN \(legacyScopeExpression) = 'global' THEN '\(KnowledgeBeliefScope.globalEntityKey)'
                        WHEN TRIM(COALESCE(entity_key, '')) = '' THEN '__unknown__'
                        ELSE entity_key
                    END,
                    evidence_links_json = CASE
                        WHEN TRIM(COALESCE(evidence_links_json, '')) = '' THEN '[]'
                        ELSE evidence_links_json
                    END,
                    belief_kind = CASE
                        WHEN TRIM(COALESCE(belief_kind, '')) = '' THEN CASE WHEN is_manual = 1 THEN 'manual' ELSE 'second_order' END
                        ELSE LOWER(REPLACE(REPLACE(TRIM(belief_kind), ' ', '_'), '-', '_'))
                    END,
                    lifecycle = CASE
                        WHEN is_manual = 1 THEN 'manual'
                        WHEN TRIM(COALESCE(lifecycle, '')) = '' THEN 'candidate'
                        ELSE LOWER(REPLACE(REPLACE(TRIM(lifecycle), ' ', '_'), '-', '_'))
                    END,
                    support_count = CASE WHEN COALESCE(support_count, 0) < 0 THEN 0 ELSE COALESCE(support_count, 1) END,
                    contradiction_count = CASE WHEN COALESCE(contradiction_count, 0) < 0 THEN 0 ELSE COALESCE(contradiction_count, 0) END,
                    last_evidence_at = CASE
                        WHEN TRIM(COALESCE(last_evidence_at, '')) = '' THEN COALESCE(NULLIF(TRIM(updated_at), ''), COALESCE(NULLIF(TRIM(created_at), ''), ''))
                        ELSE last_evidence_at
                    END;
                """,
                db: db
            )

            try migrateLegacyManualBeliefsIfNeeded(db: db, beliefColumns: columns)
            try execute(
                sql: "CREATE UNIQUE INDEX IF NOT EXISTS idx_beliefs_scope_statement_unique ON beliefs(scope, entity_key, statement);",
                db: db
            )
            try execute(
                sql: "CREATE INDEX IF NOT EXISTS idx_beliefs_scope ON beliefs(scope, entity_key, updated_at DESC);",
                db: db
            )
            try execute(sql: "COMMIT;", db: db)
        } catch {
            _ = sqlite3_exec(db, "ROLLBACK;", nil, nil, nil)
            throw error
        }
    }

    private func migrateLegacyManualBeliefsIfNeeded(db: OpaquePointer, beliefColumns: Set<String>) throws {
        let manualColumns = try tableColumnNames("manual_beliefs", db: db)
        let requiredManualColumns: Set<String> = [
            "belief_id",
            "statement",
            "belief_scope",
            "entity_type",
            "entity_key",
            "confidence",
            "status",
            "created_at",
            "updated_at"
        ]
        guard manualColumns.isSuperset(of: requiredManualColumns),
              beliefColumns.isSuperset(of: ["belief_scope", "entity_type", "status"]) else {
            return
        }

        let domainColumn = beliefColumns.contains("domain") && manualColumns.contains("domain")
        let portabilityColumn = beliefColumns.contains("portability_bucket")
        let temporalColumn = beliefColumns.contains("temporal_bucket")

        var targetColumns = ["belief_id", "statement"]
        var sourceExpressions = ["belief_id", "statement"]
        if domainColumn {
            targetColumns.append("domain")
            sourceExpressions.append("domain")
        }
        targetColumns += ["belief_scope", "entity_type", "entity_key", "confidence", "status", "created_at", "updated_at"]
        sourceExpressions += [
            "belief_scope",
            "entity_type",
            "CASE WHEN LOWER(TRIM(COALESCE(belief_scope, ''))) = 'global' THEN '\(KnowledgeBeliefScope.globalEntityKey)' ELSE entity_key END",
            "confidence",
            "status",
            "created_at",
            "updated_at"
        ]
        if portabilityColumn {
            targetColumns.append("portability_bucket")
            sourceExpressions.append("''")
        }
        if temporalColumn {
            targetColumns.append("temporal_bucket")
            sourceExpressions.append("''")
        }
        targetColumns += ["scope", "is_manual", "evidence_links_json"]
        sourceExpressions += [
            "CASE LOWER(TRIM(COALESCE(belief_scope, ''))) WHEN 'person' THEN 'person' WHEN 'space' THEN 'space' ELSE 'global' END",
            "1",
            "'[]'"
        ]
        targetColumns += ["belief_kind", "lifecycle", "support_count", "contradiction_count", "last_evidence_at"]
        sourceExpressions += ["'manual'", "'manual'", "1", "0", "updated_at"]

        try execute(
            sql: """
            INSERT OR IGNORE INTO beliefs (\(targetColumns.joined(separator: ", ")))
            SELECT \(sourceExpressions.joined(separator: ", "))
            FROM manual_beliefs;
            """,
            db: db
        )
    }

    private func ensureBeliefEvidenceSchemaCompatibility(db: OpaquePointer) throws {
        var columns = try tableColumnNames("belief_evidence", db: db)
        guard columns.isEmpty == false else {
            return
        }

        try execute(sql: "BEGIN IMMEDIATE TRANSACTION;", db: db)
        do {
            let requiredColumns: [(name: String, definition: String)] = [
                ("evidence_id", "evidence_id TEXT NOT NULL DEFAULT ''"),
                ("source", "source TEXT NOT NULL DEFAULT 'legacy'"),
                ("room_id", "room_id TEXT NOT NULL DEFAULT '__legacy__'"),
                ("person_id", "person_id TEXT"),
                ("occurred_at", "occurred_at TEXT NOT NULL DEFAULT ''"),
                ("evidence_text", "evidence_text TEXT NOT NULL DEFAULT ''"),
                ("updated_at", "updated_at TEXT NOT NULL DEFAULT ''")
            ]

            for column in requiredColumns where columns.contains(column.name) == false {
                try execute(sql: "ALTER TABLE belief_evidence ADD COLUMN \(column.definition);", db: db)
                columns.insert(column.name)
            }

            let sourceFallback = columns.contains("source_type")
                ? "COALESCE(NULLIF(TRIM(source_type), ''), 'legacy')"
                : "'legacy'"
            let evidenceTextFallback = columns.contains("note")
                ? "COALESCE(note, '')"
                : "''"

            try execute(
                sql: """
                UPDATE belief_evidence
                SET source = CASE WHEN TRIM(COALESCE(source, '')) = '' OR source = 'legacy' THEN \(sourceFallback) ELSE source END,
                    source_id = CASE
                        WHEN TRIM(COALESCE(source_id, '')) = '' THEN LOWER(HEX(RANDOMBLOB(16)))
                        ELSE source_id
                    END,
                    evidence_id = CASE
                        WHEN TRIM(COALESCE(evidence_id, '')) = '' THEN COALESCE(NULLIF(TRIM(source_id), ''), LOWER(HEX(RANDOMBLOB(16))))
                        ELSE evidence_id
                    END,
                    occurred_at = CASE WHEN TRIM(COALESCE(occurred_at, '')) = '' THEN CURRENT_TIMESTAMP ELSE occurred_at END,
                    evidence_text = CASE WHEN TRIM(COALESCE(evidence_text, '')) = '' THEN \(evidenceTextFallback) ELSE evidence_text END,
                    updated_at = CASE WHEN TRIM(COALESCE(updated_at, '')) = '' THEN CURRENT_TIMESTAMP ELSE updated_at END;
                """,
                db: db
            )
            try execute(
                sql: """
                UPDATE belief_evidence
                SET source_id = source_id || '#' || rowid
                WHERE rowid IN (
                    SELECT rowid
                    FROM (
                        SELECT rowid, COUNT(*) OVER (PARTITION BY source, source_id) AS duplicate_count
                        FROM belief_evidence
                    )
                    WHERE duplicate_count > 1
                );
                """,
                db: db
            )
            try execute(
                sql: "CREATE UNIQUE INDEX IF NOT EXISTS idx_belief_evidence_source_unique ON belief_evidence(source, source_id);",
                db: db
            )
            try execute(
                sql: "CREATE INDEX IF NOT EXISTS idx_belief_evidence_room_time ON belief_evidence(room_id, occurred_at DESC);",
                db: db
            )
            try execute(
                sql: "CREATE INDEX IF NOT EXISTS idx_belief_evidence_person_time ON belief_evidence(person_id, occurred_at DESC);",
                db: db
            )
            try execute(sql: "COMMIT;", db: db)
        } catch {
            _ = sqlite3_exec(db, "ROLLBACK;", nil, nil, nil)
            throw error
        }
    }

    private func tableColumnNames(_ tableName: String, db: OpaquePointer) throws -> Set<String> {
        let sql = "PRAGMA table_info(\(tableName));"
        return try withPreparedStatement(db: db, sql: sql) { statement in
            var columns = Set<String>()
            while true {
                let stepResult = sqlite3_step(statement)
                switch stepResult {
                case SQLITE_ROW:
                    columns.insert(columnText(statement, index: 1))
                case SQLITE_DONE:
                    return columns
                default:
                    throw sqliteErrorForStep(sql: sql, db: db, code: stepResult)
                }
            }
        }
    }

    private func currentSchemaVersion(db: OpaquePointer) throws -> Int32 {
        let sql = "SELECT COALESCE(MAX(version), 0) FROM schema_migrations;"
        return try withPreparedStatement(db: db, sql: sql) { statement in
            let stepResult = sqlite3_step(statement)
            if stepResult == SQLITE_ROW {
                return sqlite3_column_int(statement, 0)
            }
            throw sqliteErrorForStep(sql: sql, db: db, code: stepResult)
        }
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
        let statement = try prepareStatement(db: db, sql: sql)
        defer { sqlite3_finalize(statement) }
        return try body(statement)
    }

    private func prepareStatement(db: OpaquePointer, sql: String) throws -> OpaquePointer {
        var statement: OpaquePointer?
        let rc = sqlite3_prepare_v2(db, sql, -1, &statement, nil)
        guard rc == SQLITE_OK, let statement else {
            throw KnowledgeStoreError.sqlitePrepareFailed(sql: sql, message: String(cString: sqlite3_errmsg(db)))
        }
        return statement
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

    private func bindText(_ value: String, at index: Int32, in statement: OpaquePointer, sql: String) throws {
        let rc = sqlite3_bind_text(statement, index, value, -1, sqliteTransientDestructor)
        guard rc == SQLITE_OK else {
            throw KnowledgeStoreError.sqliteBindFailed(
                sql: sql,
                index: index,
                code: rc,
                message: String(cString: sqlite3_errmsg(sqlite3_db_handle(statement)))
            )
        }
    }

    private func bindOptionalText(_ value: String?, at index: Int32, in statement: OpaquePointer, sql: String) throws {
        let rc: Int32
        if let value, value.isEmpty == false {
            rc = sqlite3_bind_text(statement, index, value, -1, sqliteTransientDestructor)
        } else {
            rc = sqlite3_bind_null(statement, index)
        }
        guard rc == SQLITE_OK else {
            throw KnowledgeStoreError.sqliteBindFailed(
                sql: sql,
                index: index,
                code: rc,
                message: String(cString: sqlite3_errmsg(sqlite3_db_handle(statement)))
            )
        }
    }

    private func bindInteger(_ value: Int32, at index: Int32, in statement: OpaquePointer, sql: String) throws {
        let rc = sqlite3_bind_int(statement, index, value)
        guard rc == SQLITE_OK else {
            throw KnowledgeStoreError.sqliteBindFailed(
                sql: sql,
                index: index,
                code: rc,
                message: String(cString: sqlite3_errmsg(sqlite3_db_handle(statement)))
            )
        }
    }

    private func bindInt64(_ value: Int64, at index: Int32, in statement: OpaquePointer, sql: String) throws {
        let rc = sqlite3_bind_int64(statement, index, value)
        guard rc == SQLITE_OK else {
            throw KnowledgeStoreError.sqliteBindFailed(
                sql: sql,
                index: index,
                code: rc,
                message: String(cString: sqlite3_errmsg(sqlite3_db_handle(statement)))
            )
        }
    }

    private func bindDouble(_ value: Double, at index: Int32, in statement: OpaquePointer, sql: String) throws {
        let rc = sqlite3_bind_double(statement, index, value)
        guard rc == SQLITE_OK else {
            throw KnowledgeStoreError.sqliteBindFailed(
                sql: sql,
                index: index,
                code: rc,
                message: String(cString: sqlite3_errmsg(sqlite3_db_handle(statement)))
            )
        }
    }

    private func bindOptionalDouble(_ value: Double?, at index: Int32, in statement: OpaquePointer, sql: String) throws {
        let rc: Int32
        if let value {
            rc = sqlite3_bind_double(statement, index, value)
        } else {
            rc = sqlite3_bind_null(statement, index)
        }
        guard rc == SQLITE_OK else {
            throw KnowledgeStoreError.sqliteBindFailed(
                sql: sql,
                index: index,
                code: rc,
                message: String(cString: sqlite3_errmsg(sqlite3_db_handle(statement)))
            )
        }
    }

    private func bindValues(_ values: [SQLiteBindValue], in statement: OpaquePointer, sql: String) throws {
        for (offset, value) in values.enumerated() {
            let index = Int32(offset + 1)
            switch value {
            case .text(let text):
                try bindText(text, at: index, in: statement, sql: sql)
            case .optionalText(let text):
                try bindOptionalText(text, at: index, in: statement, sql: sql)
            case .int64(let intValue):
                try bindInt64(intValue, at: index, in: statement, sql: sql)
            case .double(let doubleValue):
                try bindDouble(doubleValue, at: index, in: statement, sql: sql)
            }
        }
    }

    private func sqliteErrorForStep(sql: String, db: OpaquePointer, code: Int32) -> KnowledgeStoreError {
        KnowledgeStoreError.sqliteStepFailed(
            sql: sql,
            code: code,
            message: String(cString: sqlite3_errmsg(db))
        )
    }

    private func columnText(_ statement: OpaquePointer, index: Int32) -> String {
        guard let cValue = sqlite3_column_text(statement, index) else {
            return ""
        }
        return String(cString: cValue)
    }

    private func columnOptionalText(_ statement: OpaquePointer, index: Int32) -> String? {
        guard sqlite3_column_type(statement, index) != SQLITE_NULL else {
            return nil
        }
        return columnText(statement, index: index)
    }

    private func normalizedID(_ value: String) -> String {
        let trimmed = value.trimmingCharacters(in: .whitespacesAndNewlines)
        return trimmed.isEmpty ? UUID().uuidString : trimmed
    }

    private func normalizedNonEmpty(_ value: String) -> String {
        let trimmed = value.trimmingCharacters(in: .whitespacesAndNewlines)
        return trimmed.isEmpty ? "__unknown__" : trimmed
    }

    private func normalizedOptionalText(_ value: String?) -> String? {
        guard let trimmed = value?.trimmingCharacters(in: .whitespacesAndNewlines),
              !trimmed.isEmpty else {
            return nil
        }
        return trimmed
    }

    private func normalizedTimestamp(_ value: String) -> String {
        let trimmed = value.trimmingCharacters(in: .whitespacesAndNewlines)
        return trimmed.isEmpty ? nowTimestamp() : trimmed
    }

    private func nowTimestamp() -> String {
        timestampFormatter.string(from: Date())
    }

    private func timestampString(_ date: Date) -> String {
        timestampFormatter.string(from: date)
    }

    private func dateFromTimestamp(_ value: String?) -> Date? {
        guard let value, !value.trimmingCharacters(in: .whitespacesAndNewlines).isEmpty else {
            return nil
        }
        if let date = timestampFormatter.date(from: value) {
            return date
        }
        let fallback = ISO8601DateFormatter()
        fallback.formatOptions = [.withInternetDateTime]
        return fallback.date(from: value)
    }

    private func encodeEvidenceLinks(_ links: [String]) throws -> String {
        let data = try jsonEncoder.encode(links)
        return String(data: data, encoding: .utf8) ?? "[]"
    }

    private func decodeEvidenceLinks(_ value: String) throws -> [String] {
        guard let data = value.data(using: .utf8) else {
            throw KnowledgeStoreError.invalidJSON(value)
        }
        return try jsonDecoder.decode([String].self, from: data)
    }

    private func encodeQuestionEvidence(_ evidence: [QuestionEvidenceRef]) throws -> String {
        let data = try jsonEncoder.encode(evidence)
        return String(data: data, encoding: .utf8) ?? "[]"
    }

    private func decodeQuestionEvidence(_ value: String) throws -> [QuestionEvidenceRef] {
        guard let data = value.data(using: .utf8) else {
            throw KnowledgeStoreError.invalidJSON(value)
        }
        return try jsonDecoder.decode([QuestionEvidenceRef].self, from: data)
    }

    private func encodeQuestionTags(_ tags: [String]) throws -> String {
        let data = try jsonEncoder.encode(tags)
        return String(data: data, encoding: .utf8) ?? "[]"
    }

    private func decodeQuestionTags(_ value: String) throws -> [String] {
        guard let data = value.data(using: .utf8) else {
            throw KnowledgeStoreError.invalidJSON(value)
        }
        return try jsonDecoder.decode([String].self, from: data)
    }

    private func decodeBeliefRow(_ statement: OpaquePointer) throws -> BeliefRecord {
        let evidenceJSONString = columnText(statement, index: 7)
        return BeliefRecord(
            id: columnText(statement, index: 0),
            scope: columnText(statement, index: 1),
            entityKey: columnText(statement, index: 2),
            statement: columnText(statement, index: 3),
            confidence: sqlite3_column_double(statement, 4),
            updatedAt: columnText(statement, index: 5),
            isManual: sqlite3_column_int(statement, 6) == 1,
            evidenceLinks: try decodeEvidenceLinks(evidenceJSONString),
            createdAt: columnText(statement, index: 8),
            beliefKind: columnText(statement, index: 9),
            lifecycle: columnText(statement, index: 10),
            supportCount: Int(sqlite3_column_int64(statement, 11)),
            contradictionCount: Int(sqlite3_column_int64(statement, 12)),
            lastEvidenceAt: columnText(statement, index: 13)
        )
    }

    private func decodeBeliefReconciliationStateRow(_ statement: OpaquePointer) -> BeliefReconciliationStateRecord {
        BeliefReconciliationStateRecord(
            scope: columnText(statement, index: 0),
            entityKey: columnText(statement, index: 1),
            lastRunAt: columnOptionalText(statement, index: 2),
            lastEvidenceHash: columnOptionalText(statement, index: 3),
            updatedAt: columnText(statement, index: 4)
        )
    }

    private func decodeQuestionCandidateRow(_ statement: OpaquePointer) throws -> QuestionCandidate {
        let scopeTypeValue = columnText(statement, index: 1)
        let statusValue = columnText(statement, index: 12)
        return QuestionCandidate(
            id: columnText(statement, index: 0),
            scopeType: QuestionScopeType(rawValue: scopeTypeValue) ?? .space,
            scopeKey: columnText(statement, index: 2),
            scopeLabel: columnText(statement, index: 3),
            questionText: columnText(statement, index: 4),
            questionType: columnText(statement, index: 5),
            whyNow: columnText(statement, index: 6),
            evidence: try decodeQuestionEvidence(columnText(statement, index: 7)),
            sourceKind: columnText(statement, index: 8),
            sourceKey: columnText(statement, index: 9),
            tags: try decodeQuestionTags(columnText(statement, index: 10)),
            priorityScore: sqlite3_column_double(statement, 11),
            status: QuestionStatus(rawValue: statusValue) ?? .candidate,
            answerSnapshotId: {
                let value = columnText(statement, index: 13)
                    .trimmingCharacters(in: .whitespacesAndNewlines)
                return value.isEmpty ? nil : value
            }(),
            createdAt: dateFromTimestamp(columnText(statement, index: 14)) ?? Date.distantPast,
            updatedAt: dateFromTimestamp(columnText(statement, index: 15)) ?? Date.distantPast,
            expiresAt: dateFromTimestamp(columnOptionalText(statement, index: 16))
        )
    }

    private func decodeWebexSyncStateRow(_ statement: OpaquePointer) -> WebexConversationSyncStateRecord {
        WebexConversationSyncStateRecord(
            conversationID: columnText(statement, index: 0),
            conversationType: WebexConversationType(rawValue: columnText(statement, index: 1)) ?? .space,
            roomID: columnText(statement, index: 2),
            personID: columnOptionalText(statement, index: 3),
            personEmail: columnOptionalText(statement, index: 4),
            title: columnOptionalText(statement, index: 5),
            lastSeenMessageID: columnOptionalText(statement, index: 6),
            lastSeenCreated: columnOptionalText(statement, index: 7),
            lastSuccessfulSyncAt: columnOptionalText(statement, index: 8),
            nextAllowedSyncAt: columnOptionalText(statement, index: 9),
            pollingMode: WebexPollingMode(rawValue: columnText(statement, index: 10)) ?? .background,
            consecutiveFailureCount: Int(sqlite3_column_int64(statement, 11)),
            lastError: columnOptionalText(statement, index: 12),
            lastErrorAt: columnOptionalText(statement, index: 13),
            updatedAt: columnText(statement, index: 14)
        )
    }

    private func normalizedBeliefScope(_ scope: String) throws -> KnowledgeBeliefScope {
        let normalizedValue = scope.trimmingCharacters(in: .whitespacesAndNewlines).lowercased()
        guard let normalizedScope = KnowledgeBeliefScope(rawValue: normalizedValue) else {
            throw KnowledgeStoreError.invalidBeliefScope(scope)
        }
        return normalizedScope
    }

    private func scopedBeliefEntityKey(scope: KnowledgeBeliefScope, entityKey: String) -> String {
        switch scope {
        case .global:
            return KnowledgeBeliefScope.globalEntityKey
        case .person, .space:
            return normalizedNonEmpty(entityKey)
        }
    }

    private func normalizedBeliefStatement(_ statement: String) throws -> String {
        let normalizedStatement = statement.trimmingCharacters(in: .whitespacesAndNewlines)
        guard normalizedStatement.isEmpty == false else {
            throw KnowledgeStoreError.invalidBeliefStatement(statement)
        }
        return normalizedStatement
    }

    private func normalizedBeliefMetadata(_ value: String, fallback: String) -> String {
        let normalized = value
            .trimmingCharacters(in: .whitespacesAndNewlines)
            .lowercased()
            .replacingOccurrences(of: " ", with: "_")
            .replacingOccurrences(of: "-", with: "_")
        return normalized.isEmpty ? fallback : normalized
    }

    private var migrations: [Migration] {
        [
            Migration(
                version: 1,
                statements: [
                    """
                    CREATE TABLE IF NOT EXISTS rooms (
                        room_id TEXT PRIMARY KEY,
                        title TEXT NOT NULL DEFAULT '',
                        updated_at TEXT NOT NULL
                    );
                    """,
                    """
                    CREATE TABLE IF NOT EXISTS people (
                        person_id TEXT PRIMARY KEY,
                        display_name TEXT NOT NULL DEFAULT '',
                        email TEXT NOT NULL DEFAULT '',
                        updated_at TEXT NOT NULL
                    );
                    """,
                    """
                    CREATE TABLE IF NOT EXISTS messages (
                        message_id TEXT PRIMARY KEY,
                        room_id TEXT NOT NULL,
                        person_id TEXT,
                        body TEXT NOT NULL DEFAULT '',
                        created_at TEXT NOT NULL,
                        updated_at TEXT NOT NULL,
                        FOREIGN KEY (room_id) REFERENCES rooms(room_id) ON DELETE CASCADE,
                        FOREIGN KEY (person_id) REFERENCES people(person_id) ON DELETE SET NULL
                    );
                    """,
                    """
                    CREATE TABLE IF NOT EXISTS files (
                        file_id TEXT PRIMARY KEY,
                        message_id TEXT,
                        room_id TEXT NOT NULL,
                        filename TEXT NOT NULL DEFAULT '',
                        mime_type TEXT NOT NULL DEFAULT '',
                        file_size INTEGER NOT NULL DEFAULT 0,
                        updated_at TEXT NOT NULL,
                        FOREIGN KEY (message_id) REFERENCES messages(message_id) ON DELETE SET NULL,
                        FOREIGN KEY (room_id) REFERENCES rooms(room_id) ON DELETE CASCADE
                    );
                    """,
                    """
                    CREATE TABLE IF NOT EXISTS focus_clusters (
                        cluster_id TEXT NOT NULL PRIMARY KEY,
                        focus_kind TEXT NOT NULL,
                        scope TEXT NOT NULL,
                        entity_key TEXT NOT NULL,
                        topic_key TEXT NOT NULL,
                        title TEXT NOT NULL DEFAULT '',
                        summary TEXT NOT NULL DEFAULT '',
                        prompt_version TEXT NOT NULL DEFAULT '',
                        source_hash TEXT NOT NULL DEFAULT '',
                        generated_at TEXT NOT NULL,
                        updated_at TEXT NOT NULL,
                        UNIQUE(focus_kind, scope, entity_key, topic_key)
                    );
                    """,
                    """
                    CREATE TABLE IF NOT EXISTS topics (
                        topic_id TEXT NOT NULL PRIMARY KEY,
                        focus_kind TEXT NOT NULL,
                        scope TEXT NOT NULL,
                        entity_key TEXT NOT NULL,
                        topic_key TEXT NOT NULL,
                        title TEXT NOT NULL DEFAULT '',
                        summary TEXT NOT NULL DEFAULT '',
                        so_what TEXT NOT NULL DEFAULT '',
                        source_label TEXT NOT NULL DEFAULT '',
                        score REAL,
                        generated_at TEXT NOT NULL,
                        updated_at TEXT NOT NULL,
                        UNIQUE(focus_kind, scope, entity_key, topic_key)
                    );
                    """,
                    """
                    CREATE TABLE IF NOT EXISTS beliefs (
                        belief_id TEXT NOT NULL PRIMARY KEY,
                        scope TEXT NOT NULL,
                        entity_key TEXT NOT NULL,
                        statement TEXT NOT NULL,
                        confidence REAL NOT NULL,
                        is_manual INTEGER NOT NULL DEFAULT 0,
                        evidence_links_json TEXT NOT NULL DEFAULT '[]',
                        created_at TEXT NOT NULL,
                        updated_at TEXT NOT NULL,
                        belief_kind TEXT NOT NULL DEFAULT 'second_order',
                        lifecycle TEXT NOT NULL DEFAULT 'candidate',
                        support_count INTEGER NOT NULL DEFAULT 1,
                        contradiction_count INTEGER NOT NULL DEFAULT 0,
                        last_evidence_at TEXT NOT NULL DEFAULT '',
                        UNIQUE(scope, entity_key, statement)
                    );
                    """,
                    "CREATE INDEX IF NOT EXISTS idx_messages_room_created ON messages(room_id, created_at DESC);",
                    "CREATE INDEX IF NOT EXISTS idx_focus_clusters_scope ON focus_clusters(focus_kind, scope, entity_key, updated_at DESC);"
                ]
            ),
            Migration(
                version: 2,
                statements: [
                    """
                    CREATE TABLE IF NOT EXISTS belief_evidence (
                        evidence_id TEXT NOT NULL PRIMARY KEY,
                        source TEXT NOT NULL DEFAULT '',
                        source_id TEXT NOT NULL DEFAULT '',
                        room_id TEXT NOT NULL,
                        person_id TEXT,
                        occurred_at TEXT NOT NULL,
                        evidence_text TEXT NOT NULL DEFAULT '',
                        updated_at TEXT NOT NULL,
                        UNIQUE(source, source_id),
                        FOREIGN KEY (room_id) REFERENCES rooms(room_id) ON DELETE CASCADE,
                        FOREIGN KEY (person_id) REFERENCES people(person_id) ON DELETE SET NULL
                    );
                    """
                ]
            ),
            Migration(
                version: 3,
                statements: [
                    """
                    CREATE TABLE IF NOT EXISTS belief_reconciliation_state (
                        scope TEXT NOT NULL,
                        entity_key TEXT NOT NULL,
                        last_run_at TEXT,
                        last_evidence_hash TEXT,
                        updated_at TEXT NOT NULL,
                        PRIMARY KEY (scope, entity_key)
                    );
                    """,
                    "CREATE INDEX IF NOT EXISTS idx_belief_reconciliation_state_updated_at ON belief_reconciliation_state(updated_at DESC);"
                ]
            ),
            Migration(
                version: 4,
                statements: [
                    """
                    CREATE TABLE IF NOT EXISTS question_candidates (
                        question_id TEXT PRIMARY KEY,
                        scope_type TEXT NOT NULL,
                        scope_key TEXT NOT NULL,
                        scope_label TEXT NOT NULL DEFAULT '',
                        question_text TEXT NOT NULL,
                        question_type TEXT NOT NULL,
                        why_now TEXT NOT NULL DEFAULT '',
                        evidence_json TEXT NOT NULL DEFAULT '[]',
                        source_kind TEXT NOT NULL DEFAULT '',
                        source_key TEXT NOT NULL DEFAULT '',
                        tags_json TEXT NOT NULL DEFAULT '[]',
                        priority_score REAL NOT NULL DEFAULT 0,
                        status TEXT NOT NULL DEFAULT 'candidate',
                        answer_snapshot_id TEXT NOT NULL DEFAULT '',
                        created_at TEXT NOT NULL,
                        updated_at TEXT NOT NULL,
                        expires_at TEXT
                    );
                    """,
                    "CREATE INDEX IF NOT EXISTS idx_question_candidates_status_priority ON question_candidates(status, priority_score DESC, updated_at DESC);",
                    "CREATE INDEX IF NOT EXISTS idx_question_candidates_scope ON question_candidates(scope_type, scope_key, updated_at DESC);",
                    "CREATE INDEX IF NOT EXISTS idx_question_candidates_type ON question_candidates(question_type, updated_at DESC);",
                    "CREATE INDEX IF NOT EXISTS idx_question_candidates_source ON question_candidates(source_kind, source_key);"
                ]
            ),
            Migration(
                version: 5,
                statements: [
                    """
                    CREATE TABLE IF NOT EXISTS webex_sync_state (
                        conversation_id TEXT PRIMARY KEY,
                        conversation_type TEXT NOT NULL,
                        room_id TEXT NOT NULL,
                        person_id TEXT,
                        person_email TEXT,
                        display_name TEXT,
                        last_seen_message_id TEXT,
                        last_seen_created TEXT,
                        last_successful_sync_at TEXT,
                        next_allowed_sync_at TEXT,
                        polling_mode TEXT NOT NULL DEFAULT 'background',
                        consecutive_failure_count INTEGER NOT NULL DEFAULT 0,
                        last_error TEXT,
                        last_error_at TEXT,
                        updated_at TEXT NOT NULL
                    );
                    """,
                    "CREATE INDEX IF NOT EXISTS idx_webex_sync_state_room_id ON webex_sync_state(room_id);",
                    "CREATE INDEX IF NOT EXISTS idx_webex_sync_state_next_allowed ON webex_sync_state(next_allowed_sync_at);",
                    "CREATE INDEX IF NOT EXISTS idx_webex_sync_state_updated_at ON webex_sync_state(updated_at DESC);"
                ]
            )
        ]
    }
}

private struct Migration {
    var version: Int32
    var statements: [String]
}
