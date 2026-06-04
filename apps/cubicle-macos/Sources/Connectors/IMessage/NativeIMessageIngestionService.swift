import Foundation
import SQLite3

/// Normalized iMessage row used as person-focus evidence.
struct IMessageTimelineMessage: Hashable {
    var id: String
    var threadID: String
    var threadTitle: String
    var handle: String
    var sender: String
    var body: String
    var createdAt: String
    var sortDate: Date
    var isFromMe: Bool
}

/// Narrow iMessage ingestion boundary used by Webex/person timeline assembly.
protocol NativeIMessageIngesting {
    /// Loads messages for matching handles from the local Messages database.
    func loadMessages(
        matching handles: [String],
        displayName: String,
        since: Date,
        limit: Int
    ) throws -> [IMessageTimelineMessage]
}

/// Normalizes email/phone handles to match Apple's Messages storage variants.
enum IMessageHandleNormalizer {
    /// Canonical value used for config persistence and direct matching.
    static func normalizedStorageValue(_ value: String) -> String? {
        let trimmed = value.trimmingCharacters(in: .whitespacesAndNewlines)
        guard !trimmed.isEmpty else { return nil }

        if trimmed.contains("@") {
            return trimmed.lowercased()
        }

        let digits = trimmed.filter(\.isNumber)
        if digits.count == 10 {
            return "+1\(digits)"
        }
        if digits.count == 11, digits.first == "1" {
            return "+\(digits)"
        }
        if trimmed.hasPrefix("+"), !digits.isEmpty {
            return "+\(digits)"
        }
        return trimmed
    }

    /// Matching keys that cover phone/email formats seen in `chat.db`.
    static func matchKeys(_ value: String) -> Set<String> {
        let trimmed = value.trimmingCharacters(in: .whitespacesAndNewlines)
        guard !trimmed.isEmpty else { return [] }

        var keys: Set<String> = [trimmed.lowercased()]
        if let normalized = normalizedStorageValue(trimmed) {
            keys.insert(normalized.lowercased())
        }

        let digits = trimmed.filter(\.isNumber)
        if !digits.isEmpty {
            keys.insert(digits)
            keys.insert("+\(digits)")
            if digits.count == 10 {
                keys.insert("+1\(digits)")
            } else if digits.count > 10 {
                keys.insert(String(digits.suffix(10)))
            }
        }
        return keys
    }
}

/// SQLite access failures for the local Messages database.
enum NativeIMessageIngestionError: LocalizedError {
    case databaseUnavailable(URL)
    case openFailed(URL, String)
    case prepareFailed(String, String)
    case stepFailed(String, String)

    var errorDescription: String? {
        switch self {
        case .databaseUnavailable(let url):
            return "iMessage database not found at \(url.path)."
        case .openFailed(let url, let message):
            return "Could not open iMessage database \(url.path): \(message)"
        case .prepareFailed(let sql, let message):
            return "Could not prepare iMessage query \(sql): \(message)"
        case .stepFailed(let sql, let message):
            return "Could not read iMessage query \(sql): \(message)"
        }
    }
}

/// Reads the user's local `~/Library/Messages/chat.db` without mutating it.
final class NativeIMessageIngestionService: NativeIMessageIngesting {
    private let chatDatabaseURL: URL
    private let busyTimeoutMilliseconds: Int32
    private let fileManager: FileManager
    private let timestampFormatter: ISO8601DateFormatter = {
        let formatter = ISO8601DateFormatter()
        formatter.formatOptions = [.withInternetDateTime, .withFractionalSeconds]
        return formatter
    }()

    /// Allows tests to point at a fixture database instead of the user's chat DB.
    init(
        configuration: RuntimeConfiguration = .current,
        chatDatabaseURL: URL? = nil,
        busyTimeoutMilliseconds: Int? = nil,
        fileManager: FileManager = .default
    ) {
        let document = configuration.jsonConfiguration
        let jsonSettings = document?.connectors?.imessage
        let environmentSettings = document?.environment?.imessage
        let configDirectory = configuration.jsonConfigurationDirectory
            ?? configuration.runtimeRoot.appendingPathComponent("config", isDirectory: true)
        self.chatDatabaseURL = chatDatabaseURL
            ?? document?.connectorFixtureURL(
                "imessage",
                runtimeRoot: configuration.runtimeRoot,
                configDirectory: configDirectory
            )
            ?? jsonSettings?.chatDatabasePath.flatMap { Self.expandedFileURL($0, relativeTo: configuration.runtimeRoot) }
            ?? environmentSettings?.chatDatabasePath.flatMap { Self.expandedFileURL($0, relativeTo: configuration.runtimeRoot) }
            ?? URL(fileURLWithPath: NSHomeDirectory(), isDirectory: true)
                .appendingPathComponent("Library/Messages/chat.db")
        self.busyTimeoutMilliseconds = Int32(
            max(
                1,
                min(
                    busyTimeoutMilliseconds
                        ?? jsonSettings?.busyTimeoutMilliseconds
                        ?? environmentSettings?.busyTimeoutMilliseconds
                        ?? 2_000,
                    60_000
                )
            )
        )
        self.fileManager = fileManager
    }

    /// Loads recent timeline messages for configured handles.
    func loadMessages(
        matching handles: [String],
        displayName: String,
        since: Date,
        limit: Int
    ) throws -> [IMessageTimelineMessage] {
        let matchKeys = Set(handles.flatMap(IMessageHandleNormalizer.matchKeys))
        guard !matchKeys.isEmpty else { return [] }
        guard fileManager.fileExists(atPath: chatDatabaseURL.path) else {
            throw NativeIMessageIngestionError.databaseUnavailable(chatDatabaseURL)
        }

        return try withDatabase { db in
            let matchedHandles = try matchingHandles(matchKeys: matchKeys, db: db)
            guard !matchedHandles.isEmpty else { return [] }

            let scale = try messageDateScale(db: db)
            let threshold = scale.rawValue(for: since)
            let chatIDs = try matchingChatIDs(handleIDs: matchedHandles.map(\.rowID), db: db)
            if chatIDs.isEmpty {
                return try loadHandleOnlyMessages(
                    handleIDs: matchedHandles.map(\.rowID),
                    displayName: displayName,
                    matchKeys: matchKeys,
                    threshold: threshold,
                    scale: scale,
                    limit: limit,
                    db: db
                )
            }

            return try loadChatMessages(
                chatIDs: chatIDs,
                displayName: displayName,
                matchKeys: matchKeys,
                threshold: threshold,
                scale: scale,
                limit: limit,
                db: db
            )
        }
    }

    /// Internal handle row matched from Apple Messages tables.
    private struct MatchedHandle {
        var rowID: Int64
        var handle: String
    }

    /// Scans handles because normalized phone/email variants are not indexed uniformly.
    private func matchingHandles(matchKeys: Set<String>, db: OpaquePointer) throws -> [MatchedHandle] {
        let sql = "SELECT ROWID, COALESCE(id, '') FROM handle;"
        return try withPreparedStatement(db: db, sql: sql) { statement in
            var rows: [MatchedHandle] = []
            while try stepSelect(statement, sql: sql, db: db) == SQLITE_ROW {
                let handle = columnText(statement, index: 1)
                guard !handle.isEmpty,
                      !IMessageHandleNormalizer.matchKeys(handle).isDisjoint(with: matchKeys) else {
                    continue
                }
                rows.append(MatchedHandle(rowID: sqlite3_column_int64(statement, 0), handle: handle))
            }
            return rows
        }
    }

    private func matchingChatIDs(handleIDs: [Int64], db: OpaquePointer) throws -> [Int64] {
        guard !handleIDs.isEmpty else { return [] }
        var chatIDs = Set<Int64>()

        if try tableExists("chat_handle_join", db: db) {
            let placeholders = Array(repeating: "?", count: handleIDs.count).joined(separator: ", ")
            let sql = "SELECT DISTINCT chat_id FROM chat_handle_join WHERE handle_id IN (\(placeholders));"
            try withPreparedStatement(db: db, sql: sql) { statement in
                try bindIntegers(handleIDs, startingAt: 1, in: statement, sql: sql)
                while try stepSelect(statement, sql: sql, db: db) == SQLITE_ROW {
                    chatIDs.insert(sqlite3_column_int64(statement, 0))
                }
            }
        }

        if try tableExists("chat_message_join", db: db) {
            let placeholders = Array(repeating: "?", count: handleIDs.count).joined(separator: ", ")
            let sql = """
            SELECT DISTINCT cmj.chat_id
            FROM chat_message_join cmj
            JOIN message m ON m.ROWID = cmj.message_id
            WHERE m.handle_id IN (\(placeholders));
            """
            try withPreparedStatement(db: db, sql: sql) { statement in
                try bindIntegers(handleIDs, startingAt: 1, in: statement, sql: sql)
                while try stepSelect(statement, sql: sql, db: db) == SQLITE_ROW {
                    chatIDs.insert(sqlite3_column_int64(statement, 0))
                }
            }
        }

        return chatIDs.sorted()
    }

    private func loadChatMessages(
        chatIDs: [Int64],
        displayName: String,
        matchKeys: Set<String>,
        threshold: Int64,
        scale: IMessageDateScale,
        limit: Int,
        db: OpaquePointer
    ) throws -> [IMessageTimelineMessage] {
        guard !chatIDs.isEmpty else { return [] }
        let placeholders = Array(repeating: "?", count: chatIDs.count).joined(separator: ", ")
        let sql = """
        SELECT m.ROWID,
               COALESCE(m.guid, ''),
               COALESCE(m.text, ''),
               m.date,
               m.is_from_me,
               COALESCE(h.id, ''),
               COALESCE(c.display_name, ''),
               COALESCE(c.chat_identifier, ''),
               c.ROWID
        FROM message m
        JOIN chat_message_join cmj ON cmj.message_id = m.ROWID
        JOIN chat c ON c.ROWID = cmj.chat_id
        LEFT JOIN handle h ON h.ROWID = m.handle_id
        WHERE cmj.chat_id IN (\(placeholders))
          AND m.date >= ?
          AND TRIM(COALESCE(m.text, '')) != ''
        ORDER BY m.date DESC
        LIMIT ?;
        """

        return try withPreparedStatement(db: db, sql: sql) { statement in
            try bindIntegers(chatIDs, startingAt: 1, in: statement, sql: sql)
            try bindInteger(threshold, at: Int32(chatIDs.count + 1), in: statement, sql: sql)
            try bindInteger(Int64(max(1, limit)), at: Int32(chatIDs.count + 2), in: statement, sql: sql)
            return try readTimelineRows(
                statement: statement,
                sql: sql,
                db: db,
                displayName: displayName,
                matchKeys: matchKeys,
                scale: scale,
                hasChatColumns: true
            )
        }
    }

    private func loadHandleOnlyMessages(
        handleIDs: [Int64],
        displayName: String,
        matchKeys: Set<String>,
        threshold: Int64,
        scale: IMessageDateScale,
        limit: Int,
        db: OpaquePointer
    ) throws -> [IMessageTimelineMessage] {
        guard !handleIDs.isEmpty else { return [] }
        let placeholders = Array(repeating: "?", count: handleIDs.count).joined(separator: ", ")
        let sql = """
        SELECT m.ROWID,
               COALESCE(m.guid, ''),
               COALESCE(m.text, ''),
               m.date,
               m.is_from_me,
               COALESCE(h.id, ''),
               '',
               '',
               0
        FROM message m
        LEFT JOIN handle h ON h.ROWID = m.handle_id
        WHERE m.handle_id IN (\(placeholders))
          AND m.date >= ?
          AND TRIM(COALESCE(m.text, '')) != ''
        ORDER BY m.date DESC
        LIMIT ?;
        """

        return try withPreparedStatement(db: db, sql: sql) { statement in
            try bindIntegers(handleIDs, startingAt: 1, in: statement, sql: sql)
            try bindInteger(threshold, at: Int32(handleIDs.count + 1), in: statement, sql: sql)
            try bindInteger(Int64(max(1, limit)), at: Int32(handleIDs.count + 2), in: statement, sql: sql)
            return try readTimelineRows(
                statement: statement,
                sql: sql,
                db: db,
                displayName: displayName,
                matchKeys: matchKeys,
                scale: scale,
                hasChatColumns: false
            )
        }
    }

    private func readTimelineRows(
        statement: OpaquePointer,
        sql: String,
        db: OpaquePointer,
        displayName: String,
        matchKeys: Set<String>,
        scale: IMessageDateScale,
        hasChatColumns: Bool
    ) throws -> [IMessageTimelineMessage] {
        var rows: [IMessageTimelineMessage] = []
        while try stepSelect(statement, sql: sql, db: db) == SQLITE_ROW {
            let rowID = sqlite3_column_int64(statement, 0)
            let guid = columnText(statement, index: 1)
            let body = columnText(statement, index: 2)
            let rawDate = sqlite3_column_int64(statement, 3)
            let isFromMe = sqlite3_column_int(statement, 4) != 0
            let handle = columnText(statement, index: 5)
            let chatDisplayName = hasChatColumns ? columnText(statement, index: 6) : ""
            let chatIdentifier = hasChatColumns ? columnText(statement, index: 7) : ""
            let chatRowID = hasChatColumns ? sqlite3_column_int64(statement, 8) : 0
            let sortDate = scale.date(from: rawDate)
            let threadID = chatRowID == 0 ? "handle:\(handle)" : "chat:\(chatRowID)"
            let threadTitle = resolvedThreadTitle(
                displayName: displayName,
                chatDisplayName: chatDisplayName,
                chatIdentifier: chatIdentifier,
                matchKeys: matchKeys
            )
            rows.append(
                IMessageTimelineMessage(
                    id: guid.isEmpty ? "imessage-\(rowID)" : guid,
                    threadID: threadID,
                    threadTitle: threadTitle,
                    handle: handle,
                    sender: senderLabel(
                        isFromMe: isFromMe,
                        handle: handle,
                        displayName: displayName,
                        matchKeys: matchKeys
                    ),
                    body: body,
                    createdAt: timestampFormatter.string(from: sortDate),
                    sortDate: sortDate,
                    isFromMe: isFromMe
                )
            )
        }
        return rows
    }

    private func senderLabel(
        isFromMe: Bool,
        handle: String,
        displayName: String,
        matchKeys: Set<String>
    ) -> String {
        if isFromMe {
            return "Me"
        }
        if !handle.isEmpty, !IMessageHandleNormalizer.matchKeys(handle).isDisjoint(with: matchKeys) {
            return displayName.trimmingCharacters(in: .whitespacesAndNewlines).isEmpty ? handle : displayName
        }
        return handle.isEmpty ? "Unknown" : handle
    }

    private func resolvedThreadTitle(
        displayName: String,
        chatDisplayName: String,
        chatIdentifier: String,
        matchKeys: Set<String>
    ) -> String {
        let trimmedDisplayName = displayName.trimmingCharacters(in: .whitespacesAndNewlines)
        let fallback = trimmedDisplayName.isEmpty ? "iMessage" : "iMessage - \(trimmedDisplayName)"
        let display = chatDisplayName.trimmingCharacters(in: .whitespacesAndNewlines)
        if !display.isEmpty {
            return display
        }
        let identifier = chatIdentifier.trimmingCharacters(in: .whitespacesAndNewlines)
        if !identifier.isEmpty, IMessageHandleNormalizer.matchKeys(identifier).isDisjoint(with: matchKeys) {
            return "iMessage - \(identifier)"
        }
        return fallback
    }

    private func withDatabase<T>(_ body: (OpaquePointer) throws -> T) throws -> T {
        var db: OpaquePointer?
        let flags = SQLITE_OPEN_READONLY | SQLITE_OPEN_FULLMUTEX
        guard sqlite3_open_v2(chatDatabaseURL.path, &db, flags, nil) == SQLITE_OK, let openedDB = db else {
            let message = db.map { sqlite3_errmsg($0).map(String.init(cString:)) ?? "unknown SQLite error" }
                ?? "unknown SQLite error"
            sqlite3_close(db)
            throw NativeIMessageIngestionError.openFailed(chatDatabaseURL, message)
        }
        defer { sqlite3_close(openedDB) }
        sqlite3_busy_timeout(openedDB, busyTimeoutMilliseconds)
        return try body(openedDB)
    }

    private static func expandedFileURL(_ path: String, relativeTo baseDirectory: URL? = nil) -> URL? {
        let trimmed = path.trimmingCharacters(in: .whitespacesAndNewlines)
        guard !trimmed.isEmpty else { return nil }
        let expanded = NSString(string: trimmed).expandingTildeInPath
        if expanded.hasPrefix("/") {
            return URL(fileURLWithPath: expanded)
        }
        guard let baseDirectory else {
            return URL(fileURLWithPath: expanded)
        }
        return baseDirectory.appendingPathComponent(expanded)
    }

    private func tableExists(_ tableName: String, db: OpaquePointer) throws -> Bool {
        let sql = "SELECT name FROM sqlite_master WHERE type = 'table' AND name = ? LIMIT 1;"
        return try withPreparedStatement(db: db, sql: sql) { statement in
            try bindText(tableName, at: 1, in: statement, sql: sql)
            return try stepSelect(statement, sql: sql, db: db) == SQLITE_ROW
        }
    }

    private func messageDateScale(db: OpaquePointer) throws -> IMessageDateScale {
        let sql = "SELECT COALESCE(MAX(date), 0) FROM message;"
        return try withPreparedStatement(db: db, sql: sql) { statement in
            guard try stepSelect(statement, sql: sql, db: db) == SQLITE_ROW else {
                return .seconds
            }
            return IMessageDateScale(rawMaximum: sqlite3_column_int64(statement, 0))
        }
    }

    private func withPreparedStatement<T>(
        db: OpaquePointer,
        sql: String,
        _ body: (OpaquePointer) throws -> T
    ) throws -> T {
        var statement: OpaquePointer?
        guard sqlite3_prepare_v2(db, sql, -1, &statement, nil) == SQLITE_OK, let prepared = statement else {
            throw NativeIMessageIngestionError.prepareFailed(sql, sqliteErrorMessage(db))
        }
        defer { sqlite3_finalize(prepared) }
        return try body(prepared)
    }

    private func stepSelect(_ statement: OpaquePointer, sql: String, db: OpaquePointer) throws -> Int32 {
        let result = sqlite3_step(statement)
        if result == SQLITE_ROW || result == SQLITE_DONE {
            return result
        }
        throw NativeIMessageIngestionError.stepFailed(sql, sqliteErrorMessage(db))
    }

    private func bindIntegers(
        _ values: [Int64],
        startingAt startIndex: Int32,
        in statement: OpaquePointer,
        sql: String
    ) throws {
        for (offset, value) in values.enumerated() {
            try bindInteger(value, at: startIndex + Int32(offset), in: statement, sql: sql)
        }
    }

    private func bindInteger(_ value: Int64, at index: Int32, in statement: OpaquePointer, sql: String) throws {
        guard sqlite3_bind_int64(statement, index, value) == SQLITE_OK else {
            throw NativeIMessageIngestionError.prepareFailed(sql, "Could not bind integer at index \(index).")
        }
    }

    private func bindText(_ value: String, at index: Int32, in statement: OpaquePointer, sql: String) throws {
        guard sqlite3_bind_text(statement, index, value, -1, imessageSQLiteTransientDestructor) == SQLITE_OK else {
            throw NativeIMessageIngestionError.prepareFailed(sql, "Could not bind text at index \(index).")
        }
    }

    private func columnText(_ statement: OpaquePointer, index: Int32) -> String {
        guard let text = sqlite3_column_text(statement, index) else { return "" }
        return String(cString: text)
    }

    private func sqliteErrorMessage(_ db: OpaquePointer) -> String {
        sqlite3_errmsg(db).map(String.init(cString:)) ?? "unknown SQLite error"
    }
}

/// Converts Apple Messages timestamps across nanosecond/second-era schemas.
private enum IMessageDateScale {
    case seconds
    case milliseconds
    case microseconds
    case nanoseconds

    private static let appleEpochOffset: TimeInterval = 978_307_200

    init(rawMaximum: Int64) {
        if rawMaximum > 100_000_000_000_000_000 {
            self = .nanoseconds
        } else if rawMaximum > 100_000_000_000_000 {
            self = .microseconds
        } else if rawMaximum > 100_000_000_000 {
            self = .milliseconds
        } else {
            self = .seconds
        }
    }

    func date(from rawValue: Int64) -> Date {
        let seconds: TimeInterval
        switch self {
        case .seconds:
            seconds = TimeInterval(rawValue)
        case .milliseconds:
            seconds = TimeInterval(rawValue) / 1_000
        case .microseconds:
            seconds = TimeInterval(rawValue) / 1_000_000
        case .nanoseconds:
            seconds = TimeInterval(rawValue) / 1_000_000_000
        }
        return Date(timeIntervalSince1970: seconds + Self.appleEpochOffset)
    }

    func rawValue(for date: Date) -> Int64 {
        let appleSeconds = date.timeIntervalSince1970 - Self.appleEpochOffset
        switch self {
        case .seconds:
            return Self.clampedInt64(appleSeconds)
        case .milliseconds:
            return Self.clampedInt64(appleSeconds * 1_000)
        case .microseconds:
            return Self.clampedInt64(appleSeconds * 1_000_000)
        case .nanoseconds:
            return Self.clampedInt64(appleSeconds * 1_000_000_000)
        }
    }

    private static func clampedInt64(_ value: TimeInterval) -> Int64 {
        guard value.isFinite else {
            return value.sign == .minus ? Int64.min : Int64.max
        }
        if value <= Double(Int64.min) {
            return Int64.min
        }
        if value >= Double(Int64.max) {
            return Int64.max
        }
        return Int64(value)
    }
}

private let imessageSQLiteTransientDestructor = unsafeBitCast(-1, to: sqlite3_destructor_type.self)
