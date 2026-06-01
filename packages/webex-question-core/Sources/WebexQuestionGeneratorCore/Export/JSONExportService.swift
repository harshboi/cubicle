import Foundation

/// JSON export helpers for analysis and generated questions.
public struct JSONExportService: Sendable {
    private let encoder: JSONEncoder

    public init(prettyPrinted: Bool = true) {
        let encoder = JSONEncoder()
        encoder.dateEncodingStrategy = .iso8601
        if prettyPrinted { encoder.outputFormatting = [.prettyPrinted, .sortedKeys] }
        self.encoder = encoder
    }

    public func exportAnalysis(_ analysis: AnalysisResult) throws -> Data {
        try encoder.encode(analysis)
    }

    public func exportQuestions(_ questions: [GeneratedQuestion]) throws -> Data {
        try encoder.encode(questions)
    }

    public func writeAnalysis(_ analysis: AnalysisResult, to url: URL) throws {
        try exportAnalysis(analysis).write(to: url, options: [.atomic])
    }

    public func writeQuestions(_ questions: [GeneratedQuestion], to url: URL) throws {
        try exportQuestions(questions).write(to: url, options: [.atomic])
    }
}
