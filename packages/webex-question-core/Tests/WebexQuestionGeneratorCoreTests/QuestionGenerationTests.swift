import XCTest
@testable import WebexQuestionGeneratorCore

final class QuestionGenerationTests: XCTestCase {
    func testGeneratedQuestionsCoverCategories() async throws {
        let result = try await sampleRunResult(topN: 20)
        let categories = Set(result.questions.map(\.category))
        XCTAssertTrue(categories.contains(.diagnostic))
        XCTAssertTrue(categories.contains(.efficiency) || categories.contains(.network))
        XCTAssertTrue(result.questions.allSatisfy { !$0.rationale.isEmpty && $0.finalScore > 0 })
        XCTAssertFalse(result.questions.contains { $0.text.contains("high high") || $0.text.contains("positive positive") })
    }
}
