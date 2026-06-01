import Foundation

/// Errors returned by WebexQuestionGeneratorCore.
public enum WebexQGError: LocalizedError, Sendable, Equatable {
    case unsupportedInputFormat(String)
    case unreadableFile(String)
    case malformedCSVRow(line: Int)
    case missingRequiredColumns([String])
    case invalidJSON(String)
    case noMessagesFound
    case invalidTopN(Int)

    public var errorDescription: String? {
        switch self {
        case .unsupportedInputFormat(let value):
            return "Unsupported input format: \(value)"
        case .unreadableFile(let path):
            return "Unable to read file at \(path)"
        case .malformedCSVRow(let line):
            return "Malformed CSV row at line \(line)"
        case .missingRequiredColumns(let columns):
            return "Missing required columns: \(columns.joined(separator: ", "))"
        case .invalidJSON(let reason):
            return "Invalid JSON input: \(reason)"
        case .noMessagesFound:
            return "No messages were found in the input."
        case .invalidTopN(let value):
            return "topN must be positive when provided; received \(value)."
        }
    }
}
