import Foundation

/// Deterministic local question generator grounded in metrics, anomalies, contrasts, correlations, topics, and objectives.
public struct QuestionGenerator: Sendable {
    private let ranker = QuestionRanker()

    public init() {}

    public func generateQuestions(from analysis: AnalysisResult, configuration: WebexQGConfiguration, topN: Int?) throws -> [GeneratedQuestion] {
        var questions: [GeneratedQuestion] = []
        let enabled = configuration.questions.enabledCategories
        func add(_ question: GeneratedQuestion) {
            if enabled.contains(question.category) { questions.append(question) }
        }

        if let topTopic = analysis.topTopicsByVolume.first {
            add(makeQuestion(
                text: "What are the most discussed topics, and why is \(topTopic.topic) dominating the dataset?",
                category: .descriptive,
                rationale: "\(topTopic.topic) is the highest-volume topic with \(topTopic.count) messages.",
                metrics: ["topic": topTopic.topic, "message_count": "\(topTopic.count)"],
                suggested: "Segment \(topTopic.topic) by space, sender, response time, and unanswered questions.",
                interestingness: supportScore(topTopic.count, total: analysis.messageCount),
                actionability: 0.75,
                confidence: confidence(sample: topTopic.count, total: analysis.messageCount),
                dimensions: ["topic"],
                related: ["message_count"]
            ))
        }

        if let activeSpace = analysis.spacesWithHighestActivity.first {
            add(makeQuestion(
                text: "Which work patterns explain why \(activeSpace.space) has the highest message volume?",
                category: .diagnostic,
                rationale: "\(activeSpace.space) has \(activeSpace.messageCount) messages across \(activeSpace.threadCount) threads.",
                metrics: ["space": activeSpace.space, "message_count": "\(activeSpace.messageCount)", "thread_count": "\(activeSpace.threadCount)"],
                suggested: "Compare this space against lower-volume spaces by topic mix, response delay, and unresolved question rate.",
                interestingness: supportScore(activeSpace.messageCount, total: analysis.messageCount),
                actionability: 0.82,
                confidence: confidence(sample: activeSpace.messageCount, total: analysis.messageCount),
                dimensions: ["space"],
                related: ["message_count", "thread_count"]
            ))
        }

        for topic in analysis.topicsWithLongestResponseTimes.prefix(5) {
            add(makeQuestion(
                text: "Why do \(topic.topic) conversations have higher response times than average?",
                category: .diagnostic,
                rationale: "\(topic.topic) has average response time \(formatSeconds(topic.averageResponseTimeSeconds)) with \(topic.count) supporting messages.",
                metrics: ["topic": topic.topic, "average_response_time": formatSeconds(topic.averageResponseTimeSeconds), "support": "\(topic.count)"],
                suggested: "Inspect participants, time of day, unanswered questions, and handoff points for this topic.",
                interestingness: 0.72 + min(0.2, Double(topic.unansweredQuestionCount) / 10),
                actionability: 0.88,
                confidence: confidence(sample: topic.count, total: analysis.messageCount),
                dimensions: ["topic", "time"],
                related: ["average_response_time_seconds", "unanswered_question_count"]
            ))
        }

        for topic in analysis.topicsWithMostUnansweredQuestions.prefix(5) where topic.unansweredQuestionCount > 0 {
            add(makeQuestion(
                text: "Where do questions about \(topic.topic) go unanswered, and what decision owner is missing?",
                category: .efficiency,
                rationale: "\(topic.topic) has \(topic.unansweredQuestionCount) unanswered questions.",
                metrics: ["topic": topic.topic, "unanswered_questions": "\(topic.unansweredQuestionCount)"],
                suggested: "Trace unanswered questions to spaces, senders, and first-response delays.",
                interestingness: min(1, 0.65 + Double(topic.unansweredQuestionCount) / 10),
                actionability: 0.95,
                confidence: confidence(sample: topic.count, total: analysis.messageCount),
                dimensions: ["topic", "owner"],
                related: ["unanswered_question_count"]
            ))
        }

        for outlier in (analysis.outlierThreadsByLength + analysis.outlierThreadsByDuration + analysis.outlierThreadsByResponseDelay).prefix(10) {
            add(makeQuestion(
                text: "What made thread \(outlier.threadID) an outlier for \(outlier.metric)?",
                category: .diagnostic,
                rationale: outlier.description + " Value \(format(outlier.value)) exceeds threshold \(format(outlier.threshold)).",
                metrics: ["thread_id": outlier.threadID, outlier.metric: format(outlier.value), "threshold": format(outlier.threshold)],
                suggested: "Inspect the first messages, participants, topic transitions, and response gaps in this outlier thread.",
                interestingness: 0.9,
                actionability: 0.78,
                confidence: 0.78,
                dimensions: ["thread"],
                related: [outlier.metric]
            ))
        }

        for outlier in analysis.outlierThreadsByNegativeSentiment.prefix(5) {
            add(makeQuestion(
                text: "What explains negative sentiment spikes in thread \(outlier.threadID)?",
                category: .diagnostic,
                rationale: "The thread is an outlier for negative sentiment with value \(format(outlier.value)).",
                metrics: ["thread_id": outlier.threadID, "sentiment_min": format(outlier.value)],
                suggested: "Review topic, participants, escalation words, and nearby response gaps.",
                interestingness: 0.86,
                actionability: 0.74,
                confidence: 0.72,
                dimensions: ["sentiment", "thread"],
                related: ["sentiment_min"]
            ))
        }

        for contrast in allContrasts(analysis).prefix(10) {
            add(makeQuestion(
                text: "How do \(contrast.name.lowercased()) differ on \(humanMetricName(contrast.metric))?",
                category: .comparative,
                rationale: "\(contrast.leftLabel.capitalized) and \(contrast.rightLabel) cohorts differ on \(humanMetricName(contrast.metric)) by \(format(contrast.magnitude)) across \(contrast.sampleSize) threads.",
                metrics: ["contrast": contrast.name, "metric": contrast.metric, "magnitude": format(contrast.magnitude), "sample_size": "\(contrast.sampleSize)"],
                suggested: "Compare topic mix, participants, and timing across the two cohorts.",
                interestingness: min(1, 0.55 + normalizedMagnitude(contrast.magnitude)),
                actionability: 0.82,
                confidence: confidence(sample: contrast.sampleSize, total: max(1, analysis.threadCount)),
                dimensions: ["cohort"],
                related: [contrast.metric]
            ))
        }

        if let asker = analysis.usersAskingMostQuestions.first, asker.questionCount > 0 {
            add(makeQuestion(
                text: "Who asks the most questions, and are those questions getting resolved?",
                category: .behavioral,
                rationale: "\(asker.user) asks the most questions with \(asker.questionCount) detected questions.",
                metrics: ["user": asker.user, "question_count": "\(asker.questionCount)"],
                suggested: "Compare question askers by unanswered rate, response time, and topic.",
                interestingness: 0.7,
                actionability: 0.8,
                confidence: confidence(sample: asker.questionCount, total: analysis.messageCount),
                dimensions: ["user"],
                related: ["question_count"]
            ))
        }

        if let mentioned = analysis.networkSummary.mostMentionedUsers.first, mentioned.mentionsReceived > 0 {
            add(makeQuestion(
                text: "Which users receive the most mentions, and are conversations overly dependent on them?",
                category: .network,
                rationale: "\(mentioned.user) received \(mentioned.mentionsReceived) mentions; centralization score is \(format(analysis.networkSummary.centralizationScore)).",
                metrics: ["top_mentioned_user": mentioned.user, "mentions": "\(mentioned.mentionsReceived)", "centralization": format(analysis.networkSummary.centralizationScore)],
                suggested: "Inspect whether mentioned users are responders, blockers, bridges, or decision owners.",
                interestingness: min(1, 0.6 + analysis.networkSummary.centralizationScore),
                actionability: 0.83,
                confidence: 0.76,
                dimensions: ["network", "user"],
                related: ["mentions_received", "centralization_score"]
            ))
        }

        if let bottleneck = analysis.networkSummary.possibleBottleneckUsers.first {
            add(makeQuestion(
                text: "Are any spaces overly dependent on \(bottleneck.user) or another single responder?",
                category: .network,
                rationale: "\(bottleneck.user) is prominent in replies/mentions with \(bottleneck.interactionCount) interactions.",
                metrics: ["user": bottleneck.user, "interactions": "\(bottleneck.interactionCount)", "replies_received": "\(bottleneck.repliesReceived)"],
                suggested: "Compare thread resolution time before and after this user participates.",
                interestingness: 0.82,
                actionability: 0.86,
                confidence: 0.72,
                dimensions: ["network", "user"],
                related: ["replies_received", "mentions_received"]
            ))
        }

        for correlation in analysis.correlationFindings.prefix(5) {
            add(makeQuestion(
                text: "Can \(correlation.xMetric) predict \(correlation.yMetric)?",
                category: .predictive,
                rationale: "Local correlation is \(format(correlation.coefficient)) across \(correlation.sampleSize) threads.",
                metrics: ["x_metric": correlation.xMetric, "y_metric": correlation.yMetric, "correlation": format(correlation.coefficient), "sample_size": "\(correlation.sampleSize)"],
                suggested: "Validate with holdout windows and inspect false positives before using as an escalation signal.",
                interestingness: min(1, abs(correlation.coefficient)),
                actionability: 0.7,
                confidence: confidence(sample: correlation.sampleSize, total: max(1, analysis.threadCount)),
                dimensions: ["prediction"],
                related: [correlation.xMetric, correlation.yMetric]
            ))
        }

        for objective in configuration.objectives.prefix(4) {
            add(makeQuestion(
                text: "Which measurable communication patterns most affect the objective to \(objective)?",
                category: .descriptive,
                rationale: "This objective is configured by the caller and should be tied to observed metrics rather than generic summaries.",
                metrics: ["objective": objective, "messages": "\(analysis.messageCount)", "threads": "\(analysis.threadCount)"],
                suggested: "Map the objective to response time, unresolved questions, topic friction, and network centralization metrics.",
                interestingness: 0.6,
                actionability: 0.9,
                confidence: analysis.datasetSummary.dataCompletenessScore,
                dimensions: ["objective"],
                related: ["response_time", "unanswered_questions", "network"]
            ))
        }

        return try ranker.rank(deduplicate(questions), configuration: configuration.scoring, topN: topN ?? configuration.questions.topN)
    }

    private func makeQuestion(text: String, category: QuestionCategory, rationale: String, metrics: [String: String], suggested: String, interestingness: Double, actionability: Double, confidence: Double, dimensions: [String], related: [String]) -> GeneratedQuestion {
        GeneratedQuestion(text: text, category: category, rationale: rationale, supportingMetrics: metrics, suggestedAnalysis: suggested, interestingnessScore: interestingness, actionabilityScore: actionability, confidenceScore: confidence, relatedDimensions: dimensions, relatedMetrics: related)
    }

    private func allContrasts(_ analysis: AnalysisResult) -> [ContrastFinding] {
        analysis.highDelayVsLowDelayContrasts + analysis.longThreadVsShortThreadContrasts + analysis.positiveVsNegativeSentimentContrasts + analysis.highQuestionVsLowQuestionContrasts
    }

    private func deduplicate(_ questions: [GeneratedQuestion]) -> [GeneratedQuestion] {
        var seen: Set<String> = []
        var result: [GeneratedQuestion] = []
        for question in questions where !seen.contains(question.text) {
            seen.insert(question.text)
            result.append(question)
        }
        return result
    }

    private func confidence(sample: Int, total: Int) -> Double {
        guard total > 0 else { return 0 }
        return min(1, max(0.25, sqrt(Double(sample) / Double(total))))
    }

    private func supportScore(_ sample: Int, total: Int) -> Double {
        min(1, 0.45 + confidence(sample: sample, total: total) * 0.55)
    }

    private func normalizedMagnitude(_ value: Double) -> Double {
        min(0.4, log10(max(1, value)) / 10)
    }

    private func formatSeconds(_ value: Double?) -> String {
        guard let value else { return "unknown" }
        if value < 60 { return "\(Int(value))s" }
        if value < 3600 { return "\(Int(value / 60))m" }
        return "\(String(format: "%.1f", value / 3600))h"
    }

    private func format(_ value: Double) -> String {
        if value.isNaN || value.isInfinite { return "0" }
        return String(format: "%.2f", value)
    }

    private func humanMetricName(_ metric: String) -> String {
        metric.replacingOccurrences(of: "_", with: " ")
    }
}
