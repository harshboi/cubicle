import Foundation
import WebexQuestionGeneratorCore

/// CLI smoke test for running the Webex question generator on a local export.
@main
struct WebexQGSmoke {
    static func main() async throws {
        let arguments = Array(CommandLine.arguments.dropFirst())
        guard let inputPath = arguments.first, !inputPath.hasPrefix("-") else {
            print("Usage: swift run WebexQGSmoke <input.csv|json|txt> [--top N] [--no-anonymize]")
            return
        }

        let topN = parseTopN(arguments) ?? 10
        var configuration = WebexQGConfiguration.default
        configuration.topics = TopicConfiguration(enabled: true, numberOfTopics: 12, minimumTopicSize: 1)
        if arguments.contains("--no-anonymize") {
            configuration.privacy.anonymizeUsers = false
        }

        let inputURL = URL(fileURLWithPath: inputPath)
        let result = try await WebexQuestionGenerator(configuration: configuration).run(inputURL: inputURL, topN: topN)

        print("Messages: \(result.messages.count)")
        print("Threads: \(result.analysis.threadCount)")
        print("Participants: \(result.analysis.participantCount)")
        print("Questions: \(result.questions.count)")

        for (index, question) in result.questions.enumerated() {
            print("\n\(index + 1). [\(question.category.rawValue)] \(question.text)")
            print("   score: \(String(format: "%.3f", question.finalScore))")
            print("   rationale: \(question.rationale)")
            print("   suggested: \(question.suggestedAnalysis)")
            if !question.supportingMetrics.isEmpty {
                let metrics = question.supportingMetrics
                    .sorted { $0.key < $1.key }
                    .map { "\($0.key)=\($0.value)" }
                    .joined(separator: ", ")
                print("   metrics: \(metrics)")
            }
        }
    }

    private static func parseTopN(_ arguments: [String]) -> Int? {
        guard let index = arguments.firstIndex(of: "--top"), arguments.indices.contains(index + 1) else {
            return nil
        }
        return Int(arguments[index + 1])
    }
}
