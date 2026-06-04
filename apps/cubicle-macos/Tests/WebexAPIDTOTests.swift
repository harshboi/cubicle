import XCTest
@testable import GetWebexSpaceMacApp

final class WebexAPIDTOTests: XCTestCase {
    func testWebexMembershipDecodesCamelCaseAPIKeys() throws {
        let data = Data(
            """
            {
              "id": "membership-1",
              "roomId": "room-1",
              "personId": "person-1",
              "personEmail": "person@example.com",
              "personDisplayName": "Person Example"
            }
            """.utf8
        )

        let membership = try JSONDecoder().decode(WebexMembership.self, from: data)

        XCTAssertEqual(membership.id, "membership-1")
        XCTAssertEqual(membership.roomID, "room-1")
        XCTAssertEqual(membership.personID, "person-1")
        XCTAssertEqual(membership.personEmail, "person@example.com")
        XCTAssertEqual(membership.personDisplayName, "Person Example")
    }
}
