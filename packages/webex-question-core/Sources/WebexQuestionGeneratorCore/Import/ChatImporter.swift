import Foundation

/// Importer boundary for file formats that produce normalized messages.
protocol ChatImporting: Sendable {
    func importMessages(from url: URL) async throws -> [Message]
}

/// Format-dispatching importer for CSV, JSON, and plain text exports.
public struct ChatImporter: Sendable {
    private let dateParser = DateParser()

    public init() {}

    public func importMessages(from url: URL) async throws -> [Message] {
        let ext = url.pathExtension.lowercased()
        switch ext {
        case "csv", "tsv":
            return try await CSVChatImporter().importMessages(from: url)
        case "json", "jsonl":
            return try await JSONChatImporter().importMessages(from: url)
        case "txt", "text", "log", "md":
            return try await TextChatImporter().importMessages(from: url)
        default:
            if let json = try? await JSONChatImporter().importMessages(from: url), !json.isEmpty { return json }
            if let csv = try? await CSVChatImporter().importMessages(from: url), !csv.isEmpty { return csv }
            if let text = try? await TextChatImporter().importMessages(from: url), !text.isEmpty { return text }
            throw WebexQGError.unsupportedInputFormat(ext.isEmpty ? url.lastPathComponent : ext)
        }
    }
}
