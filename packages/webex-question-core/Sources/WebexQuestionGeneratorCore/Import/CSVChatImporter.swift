import Foundation

/// Imports CSV or TSV chat exports using flexible column names.
public struct CSVChatImporter: ChatImporting {
    private let mapper = ColumnMapper()
    private let dateParser = DateParser()

    public init() {}

    public func importMessages(from url: URL) async throws -> [Message] {
        let content = try String(contentsOf: url, encoding: .utf8)
        return try importMessages(fromCSV: content, delimiter: url.pathExtension.lowercased() == "tsv" ? "\t" : nil)
    }

    public func importMessages(fromCSV content: String, delimiter explicitDelimiter: Character? = nil) throws -> [Message] {
        let delimiter = explicitDelimiter ?? detectDelimiter(content)
        let rows = try parseRows(content, delimiter: delimiter)
        guard let headers = rows.first, !headers.isEmpty else { throw WebexQGError.noMessagesFound }
        let mapping = try mapper.indexMapping(headers: headers)
        var messages: [Message] = []
        for (offset, row) in rows.dropFirst().enumerated() {
            if row.allSatisfy({ $0.trimmingCharacters(in: .whitespacesAndNewlines).isEmpty }) { continue }
            guard row.count <= headers.count + 8 else { throw WebexQGError.malformedCSVRow(line: offset + 2) }
            guard let timestamp = dateParser.parse(value(for: .timestamp, row: row, mapping: mapping)) else { continue }
            let text = value(for: .message, row: row, mapping: mapping) ?? ""
            if text.trimmingCharacters(in: .whitespacesAndNewlines).isEmpty { continue }
            let senderName = value(for: .sender, row: row, mapping: mapping)
            let senderID = value(for: .senderID, row: row, mapping: mapping) ?? senderName
            let message = Message(
                messageID: value(for: .messageID, row: row, mapping: mapping),
                threadID: value(for: .thread, row: row, mapping: mapping),
                spaceID: value(for: .spaceID, row: row, mapping: mapping),
                spaceName: value(for: .room, row: row, mapping: mapping),
                senderID: senderID,
                senderName: senderName,
                timestamp: timestamp,
                text: text,
                mentions: TextUtilities.extractMentions(from: text),
                replyToMessageID: value(for: .replyTo, row: row, mapping: mapping),
                rawSource: row.joined(separator: String(delimiter))
            )
            messages.append(message)
        }
        if messages.isEmpty { throw WebexQGError.noMessagesFound }
        return messages.sorted { $0.timestamp < $1.timestamp }
    }

    private func value(for column: MessageColumn, row: [String], mapping: [MessageColumn: Int]) -> String? {
        guard let index = mapping[column], row.indices.contains(index) else { return nil }
        let trimmed = row[index].trimmingCharacters(in: .whitespacesAndNewlines)
        return trimmed.isEmpty ? nil : trimmed
    }

    private func detectDelimiter(_ content: String) -> Character {
        let firstLine = content.split(whereSeparator: { $0.isNewline }).first.map(String.init) ?? ""
        return firstLine.filter { $0 == "\t" }.count > firstLine.filter { $0 == "," }.count ? "\t" : ","
    }

    private func parseRows(_ content: String, delimiter: Character) throws -> [[String]] {
        var rows: [[String]] = []
        var row: [String] = []
        var field = ""
        var inQuotes = false
        var iterator = content.makeIterator()
        while let character = iterator.next() {
            if character == "\"" {
                if inQuotes, let next = iterator.next() {
                    if next == "\"" {
                        field.append("\"")
                    } else {
                        inQuotes = false
                        if next == delimiter {
                            row.append(field); field = ""
                        } else if next == "\n" {
                            row.append(field); rows.append(row); row = []; field = ""
                        } else if next != "\r" {
                            field.append(next)
                        }
                    }
                } else {
                    inQuotes.toggle()
                }
            } else if character == delimiter && !inQuotes {
                row.append(field); field = ""
            } else if character == "\n" && !inQuotes {
                row.append(field); rows.append(row); row = []; field = ""
            } else if character != "\r" || inQuotes {
                field.append(character)
            }
        }
        if !field.isEmpty || !row.isEmpty {
            row.append(field); rows.append(row)
        }
        return rows
    }
}
