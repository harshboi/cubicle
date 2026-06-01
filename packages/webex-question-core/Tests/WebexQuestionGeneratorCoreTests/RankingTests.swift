import XCTest
@testable import WebexQuestionGeneratorCore

final class RankingTests: XCTestCase {
    func testRankingFormulaUsesConfiguredWeights() throws {
        let questions = [
            GeneratedQuestion(text: "A", category: .diagnostic, rationale: "r", suggestedAnalysis: "s", interestingnessScore: 1.0, actionabilityScore: 0.0, confidenceScore: 0.0),
            GeneratedQuestion(text: "B", category: .diagnostic, rationale: "r", suggestedAnalysis: "s", interestingnessScore: 0.0, actionabilityScore: 1.0, confidenceScore: 0.0)
        ]
        let ranked = try QuestionRanker().rank(questions, configuration: ScoringConfiguration(interestingnessWeight: 0.2, actionabilityWeight: 0.7, confidenceWeight: 0.1), topN: nil)
        XCTAssertEqual(ranked.first?.text, "B")
        let first = try XCTUnwrap(ranked.first)
        XCTAssertEqual(first.finalScore, 0.7, accuracy: 0.0001)
    }

    func testFullRunPipelineOnSampleCSV() async throws {
        let result = try await sampleRunResult(topN: 5)
        XCTAssertGreaterThan(result.messages.count, 0)
        XCTAssertGreaterThan(result.analysis.threadCount, 0)
        XCTAssertEqual(result.questions.count, 5)
    }
}
