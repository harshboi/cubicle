import Foundation

/// Full output of an import-analysis-question run.
public struct WebexQGRunResult: Codable, Sendable, Hashable {
    public let messages: [Message]
    public let analysis: AnalysisResult
    public let questions: [GeneratedQuestion]

    public init(messages: [Message], analysis: AnalysisResult, questions: [GeneratedQuestion]) {
        self.messages = messages
        self.analysis = analysis
        self.questions = questions
    }
}
