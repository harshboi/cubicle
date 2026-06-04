import Foundation
import SQLite3

/// Filesystem status for the SQLite database and its WAL sidecars.
struct KnowledgeDatabaseStatus: Equatable {
    var databaseExists: Bool
    var walExists: Bool
    var shmExists: Bool
    var path: String
}

private let sqliteTransientDestructor = unsafeBitCast(-1, to: sqlite3_destructor_type.self)

/// SQLite binding cases used by DAO insert/update helpers.
enum SQLiteBindValue {
    case text(String)
    case optionalText(String?)
    case int64(Int64)
    case double(Double)
}

func execute(sql: String, db: OpaquePointer) throws {
    var errorMessage: UnsafeMutablePointer<Int8>?
    if sqlite3_exec(db, sql, nil, nil, &errorMessage) != SQLITE_OK {
        let message = errorMessage.map { String(cString: $0) } ?? String(cString: sqlite3_errmsg(db))
        sqlite3_free(errorMessage)
        throw KnowledgeStoreError.sqliteExecFailed(sql: sql, message: message)
    }
    sqlite3_free(errorMessage)
}

func withPreparedStatement<T>(
    db: OpaquePointer,
    sql: String,
    _ body: (OpaquePointer) throws -> T
) throws -> T {
    let statement = try prepareStatement(db: db, sql: sql)
    defer { sqlite3_finalize(statement) }
    return try body(statement)
}

func stepToDone(_ statement: OpaquePointer, sql: String, db: OpaquePointer) throws {
    let rc = sqlite3_step(statement)
    guard rc == SQLITE_DONE else {
        throw sqliteErrorForStep(sql: sql, db: db, code: rc)
    }
}

func stepSelect(_ statement: OpaquePointer, sql: String, db: OpaquePointer) throws -> Int32 {
    let rc = sqlite3_step(statement)
    guard rc == SQLITE_ROW || rc == SQLITE_DONE else {
        throw sqliteErrorForStep(sql: sql, db: db, code: rc)
    }
    return rc
}

func bindText(_ value: String, at index: Int32, in statement: OpaquePointer, sql: String) throws {
    let rc = sqlite3_bind_text(statement, index, value, -1, sqliteTransientDestructor)
    guard rc == SQLITE_OK else {
        throw sqliteBindError(statement: statement, sql: sql, index: index, code: rc)
    }
}

func bindOptionalText(_ value: String?, at index: Int32, in statement: OpaquePointer, sql: String) throws {
    let rc: Int32
    if let value, value.isEmpty == false {
        rc = sqlite3_bind_text(statement, index, value, -1, sqliteTransientDestructor)
    } else {
        rc = sqlite3_bind_null(statement, index)
    }
    guard rc == SQLITE_OK else {
        throw sqliteBindError(statement: statement, sql: sql, index: index, code: rc)
    }
}

func bindInteger(_ value: Int32, at index: Int32, in statement: OpaquePointer, sql: String) throws {
    let rc = sqlite3_bind_int(statement, index, value)
    guard rc == SQLITE_OK else {
        throw sqliteBindError(statement: statement, sql: sql, index: index, code: rc)
    }
}

func bindInt64(_ value: Int64, at index: Int32, in statement: OpaquePointer, sql: String) throws {
    let rc = sqlite3_bind_int64(statement, index, value)
    guard rc == SQLITE_OK else {
        throw sqliteBindError(statement: statement, sql: sql, index: index, code: rc)
    }
}

func bindDouble(_ value: Double, at index: Int32, in statement: OpaquePointer, sql: String) throws {
    let rc = sqlite3_bind_double(statement, index, value)
    guard rc == SQLITE_OK else {
        throw sqliteBindError(statement: statement, sql: sql, index: index, code: rc)
    }
}

func bindOptionalDouble(_ value: Double?, at index: Int32, in statement: OpaquePointer, sql: String) throws {
    let rc: Int32
    if let value {
        rc = sqlite3_bind_double(statement, index, value)
    } else {
        rc = sqlite3_bind_null(statement, index)
    }
    guard rc == SQLITE_OK else {
        throw sqliteBindError(statement: statement, sql: sql, index: index, code: rc)
    }
}

func bindValues(_ values: [SQLiteBindValue], in statement: OpaquePointer, sql: String) throws {
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

func columnText(_ statement: OpaquePointer, index: Int32) -> String {
    guard let cValue = sqlite3_column_text(statement, index) else {
        return ""
    }
    return String(cString: cValue)
}

func columnOptionalText(_ statement: OpaquePointer, index: Int32) -> String? {
    guard sqlite3_column_type(statement, index) != SQLITE_NULL else {
        return nil
    }
    return columnText(statement, index: index)
}

func sqliteErrorForStep(sql: String, db: OpaquePointer, code: Int32) -> KnowledgeStoreError {
    KnowledgeStoreError.sqliteStepFailed(
        sql: sql,
        code: code,
        message: String(cString: sqlite3_errmsg(db))
    )
}

private func prepareStatement(db: OpaquePointer, sql: String) throws -> OpaquePointer {
    var statement: OpaquePointer?
    let rc = sqlite3_prepare_v2(db, sql, -1, &statement, nil)
    guard rc == SQLITE_OK, let statement else {
        throw KnowledgeStoreError.sqlitePrepareFailed(sql: sql, message: String(cString: sqlite3_errmsg(db)))
    }
    return statement
}

private func sqliteBindError(
    statement: OpaquePointer,
    sql: String,
    index: Int32,
    code: Int32
) -> KnowledgeStoreError {
    KnowledgeStoreError.sqliteBindFailed(
        sql: sql,
        index: index,
        code: code,
        message: String(cString: sqlite3_errmsg(sqlite3_db_handle(statement)))
    )
}

/// Owns knowledge database pathing, directory creation, and SQLite handles.
struct KnowledgeDatabase {
    let configuration: RuntimeConfiguration
    private let fileManager: FileManager

    /// Pins database files to a runtime root and allows filesystem injection in tests.
    init(configuration: RuntimeConfiguration = .current, fileManager: FileManager = .default) {
        self.configuration = configuration
        self.fileManager = fileManager
    }

    /// Directory containing the knowledge graph SQLite files.
    var knowledgeDirectory: URL {
        configuration.runtimeRoot.appendingPathComponent("knowledge", isDirectory: true)
    }

    /// Canonical SQLite database file used by all knowledge DAOs.
    var databaseURL: URL {
        knowledgeDirectory.appendingPathComponent("knowledge.db")
    }

    /// Reports local database file presence without opening SQLite.
    func status() -> KnowledgeDatabaseStatus {
        KnowledgeDatabaseStatus(
            databaseExists: fileManager.fileExists(atPath: databaseURL.path),
            walExists: fileManager.fileExists(atPath: databaseURL.path + "-wal"),
            shmExists: fileManager.fileExists(atPath: databaseURL.path + "-shm"),
            path: databaseURL.path
        )
    }

    /// Executes work inside a short-lived connection with migrations and pragmas applied.
    func withOpenConnection<T>(_ body: (OpaquePointer) throws -> T) throws -> T {
        try ensureKnowledgeDirectory()
        let db = try openDatabase()
        defer { sqlite3_close(db) }
        try applyConnectionPragmas(db)
        return try body(db)
    }

    private func ensureKnowledgeDirectory() throws {
        try fileManager.createDirectory(at: knowledgeDirectory, withIntermediateDirectories: true)
    }

    private func openDatabase() throws -> OpaquePointer {
        var db: OpaquePointer?
        // App refreshes and tests open short-lived connections from different
        // tasks; FULLMUTEX keeps SQLite serialization inside the handle.
        let flags = SQLITE_OPEN_READWRITE | SQLITE_OPEN_CREATE | SQLITE_OPEN_FULLMUTEX
        if sqlite3_open_v2(databaseURL.path, &db, flags, nil) != SQLITE_OK {
            let message = db.flatMap { String(cString: sqlite3_errmsg($0)) } ?? "unknown SQLite error"
            if let db {
                sqlite3_close(db)
            }
            throw KnowledgeStoreError.sqliteOpenFailed(path: databaseURL.path, message: message)
        }
        guard let db else {
            throw KnowledgeStoreError.sqliteOpenFailed(path: databaseURL.path, message: "nil SQLite pointer")
        }
        sqlite3_busy_timeout(db, 5000)
        return db
    }

    private func applyConnectionPragmas(_ db: OpaquePointer) throws {
        // Both pragmas are connection-scoped; every short-lived DAO connection
        // must opt back into the same FK/WAL guarantees.
        try execute(sql: "PRAGMA foreign_keys = ON;", db: db)
        try execute(sql: "PRAGMA journal_mode = WAL;", db: db)
    }
}
