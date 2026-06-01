import Foundation

/// Generates a Markdown report from local analysis and ranked questions.
public struct MarkdownReportGenerator: Sendable {
    public init() {}

    public func generate(analysis: AnalysisResult, questions: [GeneratedQuestion]) -> String {
        var lines: [String] = []
        lines.append("# Webex Question Generator Report")
        lines.append("")
        lines.append("## Executive Summary")
        lines.append("- Analyzed \(analysis.messageCount) messages across \(analysis.threadCount) threads, \(analysis.participantCount) participants, and \(analysis.spaceCount) spaces.")
        if let topTopic = analysis.topTopicsByVolume.first {
            lines.append("- Top topic by volume: \(topTopic.topic) (\(topTopic.count) messages).")
        }
        if let delayTopic = analysis.topicsWithLongestResponseTimes.first {
            lines.append("- Slowest response topic: \(delayTopic.topic) (average response \(formatSeconds(delayTopic.averageResponseTimeSeconds))).")
        }
        lines.append("")
        lines.append("## Dataset Overview")
        lines.append("- Message count: \(analysis.messageCount)")
        lines.append("- Thread count: \(analysis.threadCount)")
        lines.append("- Participant count: \(analysis.participantCount)")
        lines.append("- Space count: \(analysis.spaceCount)")
        lines.append("- Data completeness score: \(format(analysis.datasetSummary.dataCompletenessScore))")
        lines.append("")
        lines.append("## Top Patterns")
        for topic in analysis.topTopicsByVolume.prefix(5) {
            lines.append("- \(topic.topic): \(topic.count) messages, \(topic.unansweredQuestionCount) unanswered questions, avg response \(formatSeconds(topic.averageResponseTimeSeconds)).")
        }
        for outlier in analysis.outlierThreadsByResponseDelay.prefix(3) {
            lines.append("- Delay outlier \(outlier.threadID): \(format(outlier.value)) seconds.")
        }
        lines.append("")
        lines.append("## Top Generated Questions")
        for (index, question) in questions.enumerated() {
            lines.append("### \(index + 1). \(question.text)")
            lines.append("- Category: \(question.category.rawValue)")
            lines.append("- Final score: \(format(question.finalScore))")
            lines.append("- Rationale: \(question.rationale)")
            lines.append("- Suggested next analysis: \(question.suggestedAnalysis)")
            if !question.supportingMetrics.isEmpty {
                lines.append("- Supporting metrics: " + question.supportingMetrics.sorted(by: { $0.key < $1.key }).map { "\($0.key)=\($0.value)" }.joined(separator: "; "))
            }
            lines.append("")
        }
        lines.append("## Privacy Note")
        lines.append("This report is generated locally. The core package does not send chat content to external services. If anonymization/redaction is enabled, exported names, emails, and URLs reflect those local transforms.")
        lines.append("")
        return lines.joined(separator: "\n")
    }

    private func formatSeconds(_ value: Double?) -> String {
        guard let value else { return "unknown" }
        if value < 60 { return "\(Int(value))s" }
        if value < 3600 { return "\(Int(value / 60))m" }
        return "\(String(format: "%.1f", value / 3600))h"
    }

    private func format(_ value: Double) -> String {
        String(format: "%.2f", value)
    }
}
