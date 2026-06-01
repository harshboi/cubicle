import Foundation
import SQLite3

struct KnowledgeDatabaseStatus: Equatable {
    var databaseExists: Bool
    var walExists: Bool
    var shmExists: Bool
    var path: String
}

struct KnowledgeDatabase {
    let configuration: RuntimeConfiguration
    private let fileManager: FileManager

    init(configuration: RuntimeConfiguration = .current, fileManager: FileManager = .default) {
        self.configuration = configuration
        self.fileManager = fileManager
    }

    var knowledgeDirectory: URL {
        configuration.runtimeRoot.appendingPathComponent("knowledge", isDirectory: true)
    }

    var databaseURL: URL {
        knowledgeDirectory.appendingPathComponent("knowledge.db")
    }

    func status() -> KnowledgeDatabaseStatus {
        KnowledgeDatabaseStatus(
            databaseExists: fileManager.fileExists(atPath: databaseURL.path),
            walExists: fileManager.fileExists(atPath: databaseURL.path + "-wal"),
            shmExists: fileManager.fileExists(atPath: databaseURL.path + "-shm"),
            path: databaseURL.path
        )
    }

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
