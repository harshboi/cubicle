import Foundation

/// Main facade for importing Webex chat data, analyzing it locally, and generating ranked analytical questions.
public struct WebexQuestionGenerator: Sendable {
    public let configuration: WebexQGConfiguration

    /// Creates a local-first generator. No external calls are made by default.
    public init(configuration: WebexQGConfiguration = .default) {
        self.configuration = configuration
    }

    /// Imports messages from CSV, JSON, JSONL, or plain text.
    public func importMessages(from url: URL) async throws -> [Message] {
        try await ChatImporter().importMessages(from: url)
    }

    /// Enriches messages, reconstructs threads, computes metrics, detects outliers, and builds network summaries.
    public func analyze(messages: [Message]) async throws -> AnalysisResult {
        guard !messages.isEmpty else { throw WebexQGError.noMessagesFound }
        let enriched = FeatureExtractor().enrich(messages: messages, configuration: configuration)
        let threads = ThreadReconstructor().reconstruct(messages: enriched, configuration: configuration.threading)
        return MetricsAnalyzer().analyze(messages: enriched, threads: threads)
    }

    /// Generates deterministic ranked analytical questions from an AnalysisResult.
    public func generateQuestions(from analysis: AnalysisResult, topN: Int? = nil) async throws -> [GeneratedQuestion] {
        try QuestionGenerator().generateQuestions(from: analysis, configuration: configuration, topN: topN)
    }

    /// Runs the full import, analysis, and deterministic question generation pipeline.
    public func run(inputURL: URL, topN: Int? = nil) async throws -> WebexQGRunResult {
        let messages = try await importMessages(from: inputURL)
        let analysis = try await analyze(messages: messages)
        let questions = try await generateQuestions(from: analysis, topN: topN)
        return WebexQGRunResult(messages: messages, analysis: analysis, questions: questions)
    }
}
