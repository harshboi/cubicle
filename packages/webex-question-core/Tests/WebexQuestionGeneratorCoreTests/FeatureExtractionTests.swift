import XCTest
@testable import WebexQuestionGeneratorCore

final class FeatureExtractionTests: XCTestCase {
    func testSentimentScoring() throws {
        let analyzer = SentimentAnalyzer()
        XCTAssertGreaterThan(analyzer.score("great progress and excellent work"), 0)
        XCTAssertLessThan(analyzer.score("blocked by a bad outage and slow response"), 0)
        XCTAssertGreaterThan(analyzer.score("not bad"), 0)
    }

    func testTopicLabelingAssignsLocalTopic() throws {
        let messages = [
            Message(timestamp: Date(), text: "gateway policy routing model", mentions: []),
            Message(timestamp: Date(), text: "gateway policy enforcement", mentions: []),
            Message(timestamp: Date(), text: "customer migration plan", mentions: [])
        ]
        let labeled = TopicAnalyzer().label(messages: messages, configuration: TopicConfiguration(enabled: true, numberOfTopics: 4, minimumTopicSize: 1))
        XCTAssertTrue(labeled.contains { ($0.topicLabel ?? "").contains("gateway") })
    }
}
