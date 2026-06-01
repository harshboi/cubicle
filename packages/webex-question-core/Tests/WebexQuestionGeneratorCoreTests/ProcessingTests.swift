import XCTest
@testable import WebexQuestionGeneratorCore

final class ProcessingTests: XCTestCase {
    func testTimestampParsingMultipleFormats() throws {
        let parser = DateParser()
        XCTAssertNotNil(parser.parse("2026-05-01T10:00:00Z"))
        XCTAssertNotNil(parser.parse("2026-05-01 10:00:00"))
        XCTAssertNotNil(parser.parse("5/1/2026 10:00 AM"))
    }

    func testRedactionAnonymizationMentionsAndQuestionDetection() throws {
        var message = Message(
            senderID: "alice@example.com",
            senderName: "Alice Smith",
            timestamp: Date(timeIntervalSince1970: 0),
            text: "Can @Bob review https://example.com and email alice@example.com?"
        )
        message = MessageNormalizer().normalize(message)
        XCTAssertTrue(message.isQuestion == true)
        XCTAssertEqual(message.mentionCount, 2)

        var processor = PrivacyProcessor()
        let processed = processor.process(messages: [message], configuration: PrivacyConfiguration(anonymizeUsers: true, redactURLs: true, redactEmails: true))[0]
        XCTAssertFalse(processed.text.contains("https://example.com"))
        XCTAssertFalse(processed.text.contains("alice@example.com"))
        XCTAssertEqual(processed.senderName, "User 2")
    }

    func testThreadReconstructionUsesExplicitThreadAndFallbackWindow() throws {
        let base = Date(timeIntervalSince1970: 0)
        let messages = [
            Message(messageID: "1", spaceName: "A", senderName: "Alice", timestamp: base, text: "First?", mentions: [], replyToMessageID: nil),
            Message(messageID: "2", spaceName: "A", senderName: "Bob", timestamp: base.addingTimeInterval(60), text: "Reply", mentions: [], replyToMessageID: "1"),
            Message(messageID: "3", spaceName: "A", senderName: "Carol", timestamp: base.addingTimeInterval(7200), text: "Later", mentions: [], replyToMessageID: nil)
        ].map(MessageNormalizer().normalize)
        let threads = ThreadReconstructor().reconstruct(messages: messages, configuration: ThreadingConfiguration(fallbackWindowMinutes: 30))
        XCTAssertEqual(threads.count, 2)
        XCTAssertEqual(threads.first?.messageCount, 2)
        XCTAssertEqual(threads.first?.questionCount, 1)
    }
}
