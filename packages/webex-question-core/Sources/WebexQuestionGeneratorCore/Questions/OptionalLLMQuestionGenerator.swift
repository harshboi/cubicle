import Foundation

/// Optional extension point for apps that want to add an LLM provider externally.
/// This package does not include any network implementation.
public protocol LLMQuestionGenerating: Sendable {
    func generateQuestions(from analysis: AnalysisResult) async throws -> [GeneratedQuestion]
}
