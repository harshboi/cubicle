import XCTest
@testable import WebexQuestionGeneratorCore

final class ImportTests: XCTestCase {
    func testCSVImportWithFlexibleColumnNames() async throws {
        let csv = """
        from,created,body,space,parent,message_id
        Alice,2026-05-01T10:00:00Z,Can Bob review this?,Engineering,t1,m1
        Bob,2026-05-01T10:05:00Z,Yes this looks good,Engineering,t1,m2
        """
        let url = try writeTempFile(name: "messages.csv", contents: csv)
        let messages = try await CSVChatImporter().importMessages(from: url)
        XCTAssertEqual(messages.count, 2)
        XCTAssertEqual(messages[0].senderName, "Alice")
        XCTAssertEqual(messages[0].threadID, "t1")
        XCTAssertEqual(messages[0].spaceName, "Engineering")
        XCTAssertEqual(messages[0].messageID, "m1")
    }

    func testJSONImportWithFlexibleKeys() async throws {
        let json = """
        [{"from":"Alice","created_at":"2026-05-01T10:00:00Z","text":"Hello @Bob","room":"General","message_id":"m1"}]
        """
        let url = try writeTempFile(name: "messages.json", contents: json)
        let messages = try await JSONChatImporter().importMessages(from: url)
        XCTAssertEqual(messages.count, 1)
        XCTAssertEqual(messages[0].senderName, "Alice")
        XCTAssertEqual(messages[0].mentions, ["Bob"])
    }

    func testColumnMapperSurfacesAmbiguousMappings() throws {
        let suggestion = ColumnMapper().suggestMapping(headers: ["sender", "from", "created", "body"])
        XCTAssertEqual(suggestion.detected[.timestamp], "created")
        XCTAssertEqual(suggestion.detected[.message], "body")
        XCTAssertEqual(Set(suggestion.ambiguous[.sender] ?? []), Set(["sender", "from"]))
    }
}
