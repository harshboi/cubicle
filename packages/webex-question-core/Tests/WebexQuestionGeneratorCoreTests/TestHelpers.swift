import Foundation
@testable import WebexQuestionGeneratorCore

func writeTempFile(name: String, contents: String) throws -> URL {
    let directory = FileManager.default.temporaryDirectory.appendingPathComponent("WebexQGTests-\(UUID().uuidString)", isDirectory: true)
    try FileManager.default.createDirectory(at: directory, withIntermediateDirectories: true)
    let url = directory.appendingPathComponent(name)
    try contents.write(to: url, atomically: true, encoding: .utf8)
    return url
}

func sampleRunResult(topN: Int) async throws -> WebexQGRunResult {
    let csv = """
    sender,timestamp,message,space,thread
    Alice,2026-05-01T10:00:00Z,Can we unblock the gateway policy decision?,Engineering,t1
    Bob,2026-05-01T10:45:00Z,The policy decision is blocked by unclear ownership,Engineering,t1
    Alice,2026-05-01T11:00:00Z,Who owns the next step?,Engineering,t1
    Carol,2026-05-01T12:00:00Z,Customer migration is going well and looks good,Customer,t2
    Dave,2026-05-01T12:05:00Z,Great progress on customer migration,Customer,t2
    Eve,2026-05-02T09:00:00Z,Why is the incident response delayed?,Operations,t3
    Frank,2026-05-02T18:00:00Z,The outage created urgent risk and the response is slow,Operations,t3
    """
    let url = try writeTempFile(name: "sample.csv", contents: csv)
    let generator = WebexQuestionGenerator(configuration: .defaultWithoutPrivacyForTests)
    return try await generator.run(inputURL: url, topN: topN)
}

extension WebexQGConfiguration {
    static var defaultWithoutPrivacyForTests: WebexQGConfiguration {
        var config = WebexQGConfiguration.default
        config.privacy = PrivacyConfiguration(anonymizeUsers: false, redactURLs: true, redactEmails: true)
        config.topics = TopicConfiguration(enabled: true, numberOfTopics: 6, minimumTopicSize: 1)
        return config
    }
}
