import Foundation
import SQLite3

/// Filesystem status for the SQLite database and its WAL sidecars.
struct KnowledgeDatabaseStatus: Equatable {
    var databaseExists: Bool
    var walExists: Bool
    var shmExists: Bool
    var path: String
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

    private func execute(sql: String, db: OpaquePointer) throws {
        var errorMessage: UnsafeMutablePointer<Int8>?
        if sqlite3_exec(db, sql, nil, nil, &errorMessage) != SQLITE_OK {
            let message = errorMessage.map { String(cString: $0) } ?? String(cString: sqlite3_errmsg(db))
            sqlite3_free(errorMessage)
            throw KnowledgeStoreError.sqliteExecFailed(sql: sql, message: message)
        }
        sqlite3_free(errorMessage)
    }
}
