import XCTest
@testable import WebexQuestionGeneratorCore

final class AnalysisTests: XCTestCase {
    func testAnalysisComputesOutliersAndNetworkSummary() async throws {
        let base = Date(timeIntervalSince1970: 0)
        var messages: [Message] = []
        for i in 0..<12 {
            messages.append(Message(threadID: "long", spaceName: "Engineering", senderName: i.isMultiple(of: 2) ? "Alice" : "Bob", timestamp: base.addingTimeInterval(Double(i * 60)), text: i == 0 ? "Can we fix the urgent blocker?" : "More detail on blocker delay", mentions: i == 0 ? ["Bob"] : []))
        }
        messages.append(Message(threadID: "short", spaceName: "General", senderName: "Carol", timestamp: base.addingTimeInterval(10_000), text: "Looks good", mentions: []))
        let generator = WebexQuestionGenerator(configuration: .defaultWithoutPrivacyForTests)
        let analysis = try await generator.analyze(messages: messages)
        XCTAssertEqual(analysis.messageCount, 13)
        XCTAssertFalse(analysis.outlierThreadsByLength.isEmpty)
        XCTAssertFalse(analysis.networkSummary.highInteractionParticipants.isEmpty)
        XCTAssertEqual(analysis.spacesWithHighestActivity.first?.space, "Engineering")
    }
}
