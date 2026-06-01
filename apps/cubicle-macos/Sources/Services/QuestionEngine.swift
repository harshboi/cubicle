import CryptoKit
import Foundation
import WebexQuestionGeneratorCore

protocol QuestionCandidateSynthesizing: AnyObject {
    func synthesizeQuestionCandidates(
        from candidates: [QuestionCandidate],
        now: Date
    ) async throws -> [QuestionCandidate]
}

struct QuestionRefreshOutcome: Hashable {
    var generatedCount: Int
    var persistedCount: Int
    var personCount: Int
    var spaceCount: Int
    var codexSynthesizedCount: Int
    var completedAt: String

    var summary: String {
        "Question Engine generated \(generatedCount) Codex-synthesized candidate(s): \(personCount) person, \(spaceCount) space."
    }
}

final class QuestionCandidateService {
    private let knowledgeStore: KnowledgeStore
    private let questionSynthesizer: QuestionCandidateSynthesizing?
    private let timestampFormatter: ISO8601DateFormatter = {
        let formatter = ISO8601DateFormatter()
        formatter.formatOptions = [.withInternetDateTime, .withFractionalSeconds]
        return formatter
    }()

    init(knowledgeStore: KnowledgeStore, questionSynthesizer: QuestionCandidateSynthesizing? = nil) {
        self.knowledgeStore = knowledgeStore
        self.questionSynthesizer = questionSynthesizer
    }

    func refreshQuestionCandidates(spaceCache: FocusCache, personCache: FocusCache) async throws -> QuestionRefreshOutcome {
        try knowledgeStore.bootstrap()
        let now = Date()
        var candidates: [QuestionCandidate] = []
        candidates.reserveCapacity((spaceCache.items.count + personCache.items.count) * 3)

        candidates.append(contentsOf: try await makeCandidates(cache: spaceCache, kind: .space, now: now))
        candidates.append(contentsOf: try await makeCandidates(cache: personCache, kind: .person, now: now))

        let seedCandidates = dedupe(candidates)
        let codexSynthesizedCandidates = try await synthesizeQuestionCandidates(
            from: seedCandidates,
            now: now
        )
        let publishableCandidates = dedupe(codexSynthesizedCandidates)
        if !publishableCandidates.isEmpty {
            try knowledgeStore.deleteActiveQuestionCandidates()
            try knowledgeStore.upsertQuestionCandidates(publishableCandidates)
        }

        return QuestionRefreshOutcome(
            generatedCount: publishableCandidates.count,
            persistedCount: publishableCandidates.count,
            personCount: publishableCandidates.filter { $0.scopeType == .person }.count,
            spaceCount: publishableCandidates.filter { $0.scopeType == .space }.count,
            codexSynthesizedCount: publishableCandidates.count,
            completedAt: timestampFormatter.string(from: now)
        )
    }

    func listQuestionCandidates(limit: Int = 100, status: QuestionStatus? = nil) throws -> [QuestionCandidate] {
        try knowledgeStore.bootstrap()
        return try knowledgeStore.listQuestionCandidates(limit: limit, status: status)
    }

    func questionCandidate(id: String) throws -> QuestionCandidate? {
        try knowledgeStore.bootstrap()
        return try knowledgeStore.questionCandidate(id: id)
    }

    @discardableResult
    func updateQuestionStatus(id: String, status: QuestionStatus, expiresAt: Date? = nil) throws -> Bool {
        try knowledgeStore.bootstrap()
        return try knowledgeStore.updateQuestionStatus(id: id, status: status, expiresAt: expiresAt)
    }

    @discardableResult
    func dismissQuestion(id: String) throws -> Bool {
        try knowledgeStore.bootstrap()
        return try knowledgeStore.dismissQuestion(id: id)
    }

    @discardableResult
    func snoozeQuestion(id: String, until: Date) throws -> Bool {
        try knowledgeStore.bootstrap()
        return try knowledgeStore.snoozeQuestion(id: id, until: until)
    }

    private func synthesizeQuestionCandidates(
        from candidates: [QuestionCandidate],
        now: Date
    ) async throws -> [QuestionCandidate] {
        guard let questionSynthesizer,
              !candidates.isEmpty else {
            return []
        }

        do {
            let synthesized = try await questionSynthesizer.synthesizeQuestionCandidates(
                from: candidates,
                now: now
            )
            return synthesized.filter(Self.isPublishableCandidateForCubicle)
        } catch {
            return []
        }
    }

    private func makeCandidates(cache: FocusCache, kind: FocusKind, now: Date) async throws -> [QuestionCandidate] {
        var output: [QuestionCandidate] = []
        for (index, item) in cache.items.enumerated() {
            output.append(contentsOf: try await makeCandidates(item: item, kind: kind, itemIndex: index, now: now))
        }
        return output
    }

    private func makeCandidates(item: FocusItem, kind: FocusKind, itemIndex: Int, now: Date) async throws -> [QuestionCandidate] {
        let coreCandidates = try await makeCoreCandidates(item: item, kind: kind, itemIndex: itemIndex, now: now)
        if !coreCandidates.isEmpty {
            return coreCandidates
        }

        let scopeType: QuestionScopeType = kind == .person ? .person : .space
        let events = item.normalizedEvents(kind: kind).sorted { left, right in
            (parseDate(left.timestampLabel) ?? .distantPast) > (parseDate(right.timestampLabel) ?? .distantPast)
        }
        let evidence = evidenceRefs(from: events, fallbackItem: item)
        let latestEventDate = parseDate(item.timestamp)
            ?? events.compactMap { parseDate($0.timestampLabel) }.max()
        let analysisGeneratedDate = firstDate(in: item.detailLines, prefixes: ["Analysis Generated:", "Summary generated:"])
        let detailText = searchText(item)
        let signals = signalFlags(detailText: detailText, events: events)
        let recency = recencyScore(latestEventDate, now: now)
        let importanceBoost = max(0, 12 - Double(itemIndex))
        let evidenceBoost = Double(min(evidence.count, 4)) * 4

        // Prefer Codex-authored executive questions when they are present in
        // generated section payloads; fall back to deterministic drafts otherwise.
        var drafts = codexQuestionDrafts(item: item, kind: kind)
        if drafts.isEmpty {
            drafts = [
                latestChangeDraft(kind: kind)
            ]

            if signals.openLoop || signals.replyNeeded {
                drafts.append(openLoopDraft(kind: kind, replyNeeded: signals.replyNeeded))
            }
            if signals.decision {
                drafts.append(decisionDraft(kind: kind))
            }
            if kind == .space && signals.execQuestion {
                drafts.append(
                    QuestionDraft(
                        questionType: "space_exec_likely_question",
                        questionText: "What executive question is most likely to land next in \(item.title)?",
                        tags: ["exec", "question", "preparedness"],
                        baseScore: 18,
                        drivers: ["important-exec question context"]
                    )
                )
            }
            if kind == .space && isSummaryStale(latestEventDate: latestEventDate, analysisGeneratedDate: analysisGeneratedDate) {
                drafts.append(
                    QuestionDraft(
                        questionType: "space_summary_stale",
                        questionText: "Does \(item.title) need a refreshed summary before the next action?",
                        tags: ["summary", "stale", "refresh"],
                        baseScore: 14,
                        drivers: ["latest message is newer than analysis"]
                    )
                )
            }
            if kind == .person && signals.relationship {
                drafts.append(
                    QuestionDraft(
                        questionType: "person_relationship_signal",
                        questionText: "What relationship signal around \(item.title) changes how we should engage?",
                        tags: ["relationship", "engagement"],
                        baseScore: 12,
                        drivers: ["relationship language present"]
                    )
                )
            }
            if kind == .person && isPersonSilent(latestEventDate: latestEventDate, now: now) {
                drafts.append(
                    QuestionDraft(
                        questionType: "person_silence",
                        questionText: "Should \(item.title) be re-engaged because their latest signal is getting stale?",
                        tags: ["silence", "follow-up"],
                        baseScore: 8,
                        drivers: ["person signal is older than seven days"]
                    )
                )
            }
        }

        return drafts.prefix(4).map { draft in
            let weakEvidencePenalty = evidence.isEmpty ? 12.0 : 0
            let score = max(
                1,
                draft.baseScore + recency + importanceBoost + evidenceBoost
                    + (signals.openLoop ? 10 : 0)
                    + (signals.decision ? 8 : 0)
                    + (signals.replyNeeded ? 8 : 0)
                    - weakEvidencePenalty
            )
            return makeCandidate(
                draft: draft,
                item: item,
                scopeType: scopeType,
                evidence: evidence,
                latestEventDate: latestEventDate,
                priorityScore: score,
                now: now
            )
        }
    }

    private func latestChangeDraft(kind: FocusKind) -> QuestionDraft {
        switch kind {
        case .space:
            return QuestionDraft(
                questionType: "space_latest_change",
                questionText: "What changed in this space that needs action now?",
                tags: ["latest-change", "action"],
                baseScore: 14,
                drivers: ["latest tracked space activity"]
            )
        case .person:
            return QuestionDraft(
                questionType: "person_latest_change",
                questionText: "What changed around this person that changes how we should engage?",
                tags: ["latest-change", "engagement"],
                baseScore: 14,
                drivers: ["latest tracked person activity"]
            )
        }
    }

    private func openLoopDraft(kind: FocusKind, replyNeeded: Bool) -> QuestionDraft {
        switch kind {
        case .space:
            return QuestionDraft(
                questionType: replyNeeded ? "space_reply_needed" : "space_open_loop",
                questionText: "Which open loop in this space needs an owner, reply, or deadline?",
                tags: ["open-loop", "owner", "reply"],
                baseScore: 18,
                drivers: [replyNeeded ? "reply-needed signal" : "open-loop signal"]
            )
        case .person:
            return QuestionDraft(
                questionType: replyNeeded ? "person_reply_needed" : "person_open_loop",
                questionText: "What does this person need from us next, and who owns the follow-up?",
                tags: ["open-loop", "follow-up", "owner"],
                baseScore: 18,
                drivers: [replyNeeded ? "reply-needed signal" : "open-loop signal"]
            )
        }
    }

    private func decisionDraft(kind: FocusKind) -> QuestionDraft {
        switch kind {
        case .space:
            return QuestionDraft(
                questionType: "space_decision_followthrough",
                questionText: "Which decision in this space needs closure evidence before the next update?",
                tags: ["decision", "closure", "evidence"],
                baseScore: 17,
                drivers: ["decision or closure language"]
            )
        case .person:
            return QuestionDraft(
                questionType: "person_priority_watch",
                questionText: "Which priority around this person needs a sharper owner or next step?",
                tags: ["priority", "owner", "decision"],
                baseScore: 15,
                drivers: ["decision or priority language"]
            )
        }
    }

    private func makeCandidate(
        draft: QuestionDraft,
        item: FocusItem,
        scopeType: QuestionScopeType,
        evidence: [QuestionEvidenceRef],
        latestEventDate: Date?,
        priorityScore: Double,
        now: Date
    ) -> QuestionCandidate {
        let scopedQuestionText = draft.questionText
            .replacingOccurrences(of: "this space", with: item.title)
            .replacingOccurrences(of: "this person", with: item.title)
        let normalizedQuestion = normalizedKey(scopedQuestionText)
        let questionKey = [
            scopeType.rawValue,
            item.id,
            draft.questionType,
            normalizedQuestion
        ].joined(separator: "|")
        let questionID = "question-\(Self.stableHash(questionKey))"
        let drivers = draft.drivers.joined(separator: ", ")
        let whyNowParts = [
            latestEventDate.map { "latest evidence is \(displayDate($0))" },
            drivers.isEmpty ? nil : drivers,
            evidence.isEmpty ? "evidence is weak, so priority is reduced" : "\(evidence.count) evidence reference(s)"
        ].compactMap { $0 }
        return QuestionCandidate(
            id: questionID,
            scopeType: scopeType,
            scopeKey: item.id,
            scopeLabel: item.title,
            questionText: scopedQuestionText,
            questionType: draft.questionType,
            whyNow: whyNowParts.joined(separator: "; "),
            evidence: evidence,
            sourceKind: draft.questionType.hasPrefix("codex_")
                ? "codex_ai"
                : (evidence.first?.sourceType ?? "focus_cache"),
            sourceKey: draft.questionType.hasPrefix("codex_")
                ? "\(item.id):codex"
                : (evidence.first?.sourceID ?? item.id),
            tags: draft.tags,
            priorityScore: priorityScore,
            status: .candidate,
            answerSnapshotId: nil,
            createdAt: now,
            updatedAt: now,
            expiresAt: Calendar.current.date(byAdding: .day, value: 14, to: now)
        )
    }

    private func makeCoreCandidates(
        item: FocusItem,
        kind: FocusKind,
        itemIndex: Int,
        now: Date
    ) async throws -> [QuestionCandidate] {
        let extractedMessages = extractCoreMessages(item: item, kind: kind)
        guard extractedMessages.count >= 2 else {
            return []
        }

        var configuration = WebexQGConfiguration.default
        configuration.privacy.anonymizeUsers = false
        configuration.privacy.redactURLs = true
        configuration.privacy.redactEmails = true
        configuration.topics = TopicConfiguration(enabled: true, numberOfTopics: 8, minimumTopicSize: 1)
        configuration.questions = QuestionConfiguration(
            topN: 12,
            enabledCategories: [.behavioral, .diagnostic, .efficiency, .network]
        )
        configuration.objectives = coreObjectives(item: item, kind: kind)

        let generator = WebexQuestionGenerator(configuration: configuration)
        let messages = extractedMessages.map(\.message)
        let analysis = try await generator.analyze(messages: messages)
        let generatedQuestions = try await generator.generateQuestions(from: analysis, topN: 12)
        let publishableQuestions = generatedQuestions
            .filter(Self.isPublishableCoreQuestionForCubicle)
            .prefix(4)
        guard !publishableQuestions.isEmpty else {
            return []
        }

        let scopeType: QuestionScopeType = kind == .person ? .person : .space
        let latestDate = messages.map(\.timestamp).max()
        let recency = recencyScore(latestDate, now: now)
        let importanceBoost = max(0, 12 - Double(itemIndex))
        let sourceSignature = Self.stableHash(
            extractedMessages
                .map { [$0.sourceID, $0.threadID, $0.message.text].joined(separator: "|") }
                .joined(separator: "\n")
        )

        return publishableQuestions.map { question in
            makeCoreCandidate(
                question: question,
                item: item,
                scopeType: scopeType,
                extractedMessages: extractedMessages,
                latestEventDate: latestDate,
                sourceSignature: sourceSignature,
                priorityScore: corePriorityScore(question: question, recency: recency, importanceBoost: importanceBoost),
                now: now
            )
        }
    }

    private func makeCoreCandidate(
        question: GeneratedQuestion,
        item: FocusItem,
        scopeType: QuestionScopeType,
        extractedMessages: [CoreFocusMessage],
        latestEventDate: Date?,
        sourceSignature: String,
        priorityScore: Double,
        now: Date
    ) -> QuestionCandidate {
        let normalizedQuestion = normalizedKey(question.text)
        let questionID = "question-\(Self.stableHash([scopeType.rawValue, item.id, "core", normalizedQuestion].joined(separator: "|")))"
        let evidence = evidenceForCoreQuestion(question, extractedMessages: extractedMessages)
        let whyNowParts = [
            latestEventDate.map { "latest evidence is \(displayDate($0))" },
            "local analytics: \(question.category.rawValue)",
            coreDisplayRationale(question),
            evidence.isEmpty ? "evidence is weak, so priority is reduced" : "\(evidence.count) evidence reference(s)"
        ].compactMap { $0 }

        return QuestionCandidate(
            id: questionID,
            scopeType: scopeType,
            scopeKey: item.id,
            scopeLabel: item.title,
            questionText: scopedCoreQuestionText(question.text, item: item, scopeType: scopeType),
            questionType: "core_\(question.category.rawValue)",
            whyNow: whyNowParts.joined(separator: "; "),
            evidence: evidence,
            sourceKind: "webex_qg_core",
            sourceKey: "\(item.id):\(sourceSignature.prefix(16))",
            tags: coreTags(question),
            priorityScore: priorityScore,
            status: .candidate,
            answerSnapshotId: nil,
            createdAt: now,
            updatedAt: now,
            expiresAt: Calendar.current.date(byAdding: .day, value: 14, to: now)
        )
    }

    private func extractCoreMessages(item: FocusItem, kind: FocusKind) -> [CoreFocusMessage] {
        let roomTitle = item.firstDetailLine(prefix: "Space Name:").isEmpty
            ? item.title
            : item.firstDetailLine(prefix: "Space Name:")
        let sourceLines = messageSourceLines(item)
        var currentThreadID = "thread:\(Self.stableHash([kind.rawValue, item.id, "default"].joined(separator: "|")).prefix(16))"
        var extracted: [CoreFocusMessage] = []
        var seenSourceIDs = Set<String>()

        for (ordinal, rawLine) in sourceLines.enumerated() {
            let line = normalizeWhitespace(rawLine)
            if let threadSeed = threadBoundarySeed(line) {
                currentThreadID = "thread:\(Self.stableHash([kind.rawValue, item.id, threadSeed, String(ordinal)].joined(separator: "|")).prefix(16))"
                continue
            }
            guard let parsed = parseCoreMessageLine(line, fallbackRoomTitle: roomTitle) else {
                continue
            }
            guard let timestamp = parseDate(parsed.timestamp) else {
                continue
            }
            let sourceID = "event:\(Self.stableHash([kind.rawValue, item.id, parsed.sourceType, parsed.timestamp, parsed.sender, parsed.room, parsed.text, String(ordinal)].joined(separator: "|")))"
            guard seenSourceIDs.insert(sourceID).inserted else {
                continue
            }
            let message = WebexQuestionGeneratorCore.Message(
                messageID: sourceID,
                threadID: currentThreadID,
                spaceID: nil,
                spaceName: parsed.room,
                senderID: nil,
                senderName: parsed.sender,
                timestamp: timestamp,
                text: parsed.text,
                mentions: [],
                replyToMessageID: nil,
                rawSource: parsed.sourceType
            )
            let evidence = QuestionEvidenceRef(
                sourceType: parsed.sourceType,
                sourceID: sourceID,
                createdAt: timestamp,
                label: "\(displayDate(timestamp)) | \(parsed.sender)",
                preview: parsed.text
            )
            extracted.append(
                CoreFocusMessage(
                    sourceID: sourceID,
                    threadID: currentThreadID,
                    sourceType: parsed.sourceType,
                    message: message,
                    evidence: evidence
                )
            )
        }

        return extracted.sorted { $0.message.timestamp < $1.message.timestamp }
    }

    private func messageSourceLines(_ item: FocusItem) -> [String] {
        if !item.detailLines.isEmpty {
            return item.detailLines
        }
        return item.detailSections.flatMap { [$0.header] + $0.lines } + item.detailTailLines
    }

    private func threadBoundarySeed(_ line: String) -> String? {
        let prefixes = [
            "Date:",
            "Date range:",
            "Conversation ",
            "Recent conversations"
        ]
        guard prefixes.contains(where: { line.hasPrefix($0) }) else {
            return nil
        }
        return line
    }

    private func parseCoreMessageLine(_ line: String, fallbackRoomTitle: String) -> ParsedCoreMessageLine? {
        guard let colon = line.firstIndex(of: ":") else {
            return nil
        }
        let prefix = String(line[..<colon]).trimmingCharacters(in: .whitespacesAndNewlines)
        guard isMessagePrefix(prefix) else {
            return nil
        }
        let payload = String(line[line.index(after: colon)...]).trimmingCharacters(in: .whitespacesAndNewlines)
        let fields = payload
            .split(separator: "|", maxSplits: 3, omittingEmptySubsequences: false)
            .map { String($0).trimmingCharacters(in: .whitespacesAndNewlines) }
        guard fields.count >= 3 else {
            return nil
        }

        let timestamp = fields[0]
        let sender = fields[1].isEmpty ? "Unknown" : fields[1]
        let room: String
        let text: String
        if fields.count >= 4 {
            room = fields[2].isEmpty ? fallbackRoomTitle : fields[2]
            text = fields[3]
        } else {
            room = fallbackRoomTitle
            text = fields[2]
        }
        let normalizedText = normalizeWhitespace(text)
        guard normalizedText.count >= 2 else {
            return nil
        }
        let lower = "\(prefix) \(room)".lowercased()
        let sourceType = lower.contains("imessage") || lower.contains("context") || lower.contains("anchor")
            ? "imessage"
            : "webex_message"
        return ParsedCoreMessageLine(
            sourceType: sourceType,
            timestamp: timestamp,
            sender: sender,
            room: room,
            text: normalizedText
        )
    }

    private func isMessagePrefix(_ prefix: String) -> Bool {
        let lower = prefix.lowercased()
        return lower.hasPrefix("webex")
            || lower.hasPrefix("space message")
            || lower.hasPrefix("context")
            || lower.hasPrefix("anchor")
            || lower.hasPrefix("imessage")
            || lower.hasPrefix("message")
    }

    private func evidenceForCoreQuestion(
        _ question: GeneratedQuestion,
        extractedMessages: [CoreFocusMessage]
    ) -> [QuestionEvidenceRef] {
        let threadID = question.supportingMetrics["thread_id"]
        let matchingThread = threadID.map { id in
            extractedMessages.filter { $0.threadID == id }
        } ?? []
        let selected = matchingThread.isEmpty ? Array(extractedMessages.suffix(4)) : Array(matchingThread.prefix(4))
        return selected.map(\.evidence)
    }

    static func isPublishableCoreQuestionForCubicle(_ question: GeneratedQuestion) -> Bool {
        switch question.category {
        case .comparative, .descriptive, .predictive:
            return false
        case .behavioral, .diagnostic, .efficiency, .network:
            break
        }

        if question.supportingMetrics["thread_id"] != nil {
            return false
        }

        return isPublishableQuestionTextForCubicle(
            questionText: question.text,
            whyNow: [
                question.rationale,
                question.suggestedAnalysis
            ].joined(separator: " "),
            tags: question.relatedDimensions + question.relatedMetrics
        )
    }

    static func isPublishableCandidateForCubicle(_ candidate: QuestionCandidate) -> Bool {
        isPublishableQuestionTextForCubicle(
            questionText: candidate.questionText,
            whyNow: candidate.whyNow,
            tags: candidate.tags
        )
    }

    static func isPublishableQuestionTextForCubicle(
        questionText: String,
        whyNow: String,
        tags: [String]
    ) -> Bool {
        let visibleText = normalizedPublicationText(
            ([questionText, whyNow] + tags).joined(separator: " ")
        )
        let blockedTerms = [
            "average_response_time",
            "cohort",
            "correlation",
            "dataset",
            "duration seconds",
            "duration_seconds",
            "high-question",
            "low-question",
            "message volume",
            "participant count",
            "predict ",
            "predict?",
            "response time seconds",
            "response times",
            "sample_size",
            "thread:",
            "thread_id",
            "x_metric",
            "y_metric"
        ]
        if blockedTerms.contains(where: { visibleText.contains($0) }) {
            return false
        }
        if questionText.range(
            of: #"\bthread\s+(thread:|[0-9a-f]{6,}|[0-9]+)"#,
            options: [.regularExpression, .caseInsensitive]
        ) != nil {
            return false
        }

        let productTerms = [
            "approval",
            "blocker",
            "bottleneck",
            "customer",
            "deadline",
            "decision",
            "dependent",
            "escalation",
            "follow up",
            "follow-up",
            "next step",
            "owner",
            "resolved",
            "risk",
            "unanswered"
        ]
        return productTerms.contains { visibleText.contains($0) }
    }

    private func scopedCoreQuestionText(_ text: String, item: FocusItem, scopeType: QuestionScopeType) -> String {
        let label: String
        switch scopeType {
        case .person:
            label = "\(item.title) conversations"
        case .space:
            label = item.title
        }
        var trimmed = normalizeWhitespace(text)
        trimmed = trimmed.replacingOccurrences(of: "about general", with: "in \(label)")
        trimmed = trimmed.replacingOccurrences(of: "general conversations", with: label)
        trimmed = trimmed.replacingOccurrences(of: "why is general dominating the dataset", with: "why this pattern is dominating \(label)")
        trimmed = trimmed.replacingOccurrences(of: "why is General dominating the dataset", with: "why this pattern is dominating \(label)")
        if trimmed.localizedCaseInsensitiveContains(item.title) {
            return trimmed
        }
        switch scopeType {
        case .person:
            return "\(trimmed) [\(item.title)]"
        case .space:
            return "\(trimmed) [\(item.title)]"
        }
    }

    private func coreTags(_ question: GeneratedQuestion) -> [String] {
        var tags = ["core", "local-analytics", question.category.rawValue]
        tags.append(contentsOf: question.relatedDimensions.prefix(3).map { "dimension:\($0)" })
        var seen = Set<String>()
        return tags.filter { seen.insert($0).inserted }
    }

    private func corePriorityScore(question: GeneratedQuestion, recency: Double, importanceBoost: Double) -> Double {
        let base = 82 + question.finalScore * 22
        let recencyBoost = min(8, recency / 4)
        let priority = base + recencyBoost + min(4, importanceBoost / 3)
        return min(120, max(1, priority))
    }

    private func coreDisplayRationale(_ question: GeneratedQuestion) -> String {
        switch question.category {
        case .behavioral:
            return "Recent evidence shows a repeated question pattern that may need resolution."
        case .diagnostic:
            return "Recent evidence shows a communication pattern that may need review."
        case .efficiency:
            return "Recent evidence contains unanswered questions or missing decision ownership."
        case .network:
            return "Recent evidence suggests replies or mentions may be concentrated around key people."
        case .comparative, .descriptive, .predictive:
            return "Recent evidence surfaced a local communication pattern worth checking."
        }
    }

    private func coreObjectives(item: FocusItem, kind: FocusKind) -> [String] {
        switch kind {
        case .person:
            return [
                "identify unresolved follow-ups with \(item.title)",
                "find communication bottlenecks involving \(item.title)",
                "understand topic-level friction around \(item.title)",
                "surface owner, decision, and timing questions"
            ]
        case .space:
            return [
                "identify unresolved decisions in \(item.title)",
                "find communication bottlenecks in the space",
                "understand topic-level friction and escalation risk",
                "surface owner, deadline, and customer-impact questions"
            ]
        }
    }

    private func codexQuestionDrafts(item: FocusItem, kind: FocusKind) -> [QuestionDraft] {
        let drafts: [QuestionDraft]
        switch kind {
        case .space:
            let questionSections = item.detailSections.filter { section in
                section.header.hasPrefix("What are the Questions running in the Exec's (")
                    && section.header.hasSuffix("Mind:")
            }

            if !questionSections.isEmpty {
                drafts = questionSections.flatMap { section in
                    codexDrafts(from: section.lines, execName: execName(from: section.header))
                }
            } else {
                let numberedDrafts = codexDrafts(from: item.detailLines, execName: nil)
                if numberedDrafts.isEmpty {
                    drafts = codexSpaceSummaryDrafts(item: item)
                } else {
                    drafts = numberedDrafts
                }
            }
        case .person:
            drafts = codexPersonDrafts(item: item)
        }

        var deduped: [QuestionDraft] = []
        var seen = Set<String>()
        for draft in drafts {
            let key = normalizedKey(draft.questionText)
            guard !key.isEmpty, seen.insert(key).inserted else {
                continue
            }
            deduped.append(draft)
        }
        return deduped
    }

    private func codexSpaceSummaryDrafts(item: FocusItem) -> [QuestionDraft] {
        let summarySource = item.firstDetailLine(prefix: "Space summary source:").lowercased()
        let summary = item.firstDetailLine(prefix: "Space summary:")
        guard !summary.isEmpty,
              summarySource.contains("codex") else {
            return []
        }

        return [
            QuestionDraft(
                questionType: "codex_space_question",
                questionText: "What is the highest-impact next action in \(item.title) given the latest Codex summary?",
                tags: ["codex", "ai", "space", "summary"],
                baseScore: 24,
                drivers: ["Codex-generated space summary"]
            )
        ]
    }

    private func codexPersonDrafts(item: FocusItem) -> [QuestionDraft] {
        var drafts: [QuestionDraft] = []
        let lines = item.detailLines
        guard !lines.isEmpty else {
            return []
        }

        if let summaryLine = lines.first(where: { $0.trimmingCharacters(in: .whitespacesAndNewlines).hasPrefix("Person summary:") }) {
            let summary = summaryLine
                .replacingOccurrences(of: "Person summary:", with: "")
                .trimmingCharacters(in: .whitespacesAndNewlines)
            if summary.count >= 16 {
                drafts.append(
                    QuestionDraft(
                        questionType: "codex_person_question",
                        questionText: "What is the highest-leverage follow-up with \(item.title) based on the latest Codex summary?",
                        tags: ["codex", "ai", "person", "summary"],
                        baseScore: 22,
                        drivers: ["Codex-generated person summary"]
                    )
                )
            }
        }

        guard let clusterHeaderIndex = lines.firstIndex(where: { line in
            line.trimmingCharacters(in: .whitespacesAndNewlines).hasPrefix("Codex-named conversation clusters:")
        }) else {
            return drafts
        }

        let clusterLines = lines[(clusterHeaderIndex + 1)...].prefix(20)
        for raw in clusterLines {
            let trimmed = raw.trimmingCharacters(in: .whitespacesAndNewlines)
            guard trimmed.hasPrefix("- ") else {
                if !trimmed.isEmpty { break }
                continue
            }

            let body = String(trimmed.dropFirst(2))
            let titlePart = body.split(separator: ":", maxSplits: 1).first.map(String.init)?
                .trimmingCharacters(in: .whitespacesAndNewlines) ?? ""
            guard !titlePart.isEmpty else {
                continue
            }
            let questionText = "What follow-up with \(item.title) should we drive next on \"\(titlePart)\"?"
            drafts.append(
                QuestionDraft(
                    questionType: "codex_person_question",
                    questionText: questionText,
                    tags: ["codex", "ai", "person", "cluster"],
                    baseScore: 23,
                    drivers: ["Codex-named person cluster: \(titlePart)"]
                )
            )
        }

        return drafts
    }

    private func codexDrafts(from lines: [String], execName: String?) -> [QuestionDraft] {
        var drafts: [QuestionDraft] = []
        let numberPattern = #"^\d+\.\s*(.+)$"#
        let regex = try? NSRegularExpression(pattern: numberPattern, options: [])

        for rawLine in lines {
            let line = normalizeWhitespace(rawLine)
            guard let regex,
                  let match = regex.firstMatch(
                    in: line,
                    range: NSRange(line.startIndex..<line.endIndex, in: line)
                  ),
                  match.numberOfRanges >= 2,
                  let range = Range(match.range(at: 1), in: line) else {
                continue
            }
            let questionText = line[range].trimmingCharacters(in: .whitespacesAndNewlines)
            guard questionText.count >= 8 else {
                continue
            }

            var tags = ["codex", "ai", "exec"]
            var drivers = ["Codex-generated executive question"]
            if let execName, !execName.isEmpty {
                tags.append("exec:\(normalizedKey(execName))")
                drivers.append("executive context: \(execName)")
            }

            drafts.append(
                QuestionDraft(
                    questionType: "codex_exec_question",
                    questionText: questionText,
                    tags: tags,
                    baseScore: 24,
                    drivers: drivers
                )
            )
        }
        return drafts
    }

    private func execName(from header: String) -> String {
        guard let start = header.firstIndex(of: "("),
              let end = header[start...].firstIndex(of: ")"),
              start < end else {
            return ""
        }
        let name = header[header.index(after: start)..<end]
        return String(name).trimmingCharacters(in: .whitespacesAndNewlines)
    }

    private func evidenceRefs(from events: [FocusNormalizedEvent], fallbackItem item: FocusItem) -> [QuestionEvidenceRef] {
        let eventRefs = events.prefix(4).map { event in
            QuestionEvidenceRef(
                sourceType: "focus_event",
                sourceID: event.id,
                createdAt: parseDate(event.timestampLabel),
                label: "\(displayTimestamp(event.timestampLabel)) | \(event.senderLabel)",
                preview: normalizeWhitespace(event.messageText)
            )
        }
        if !eventRefs.isEmpty {
            return Array(eventRefs)
        }
        return item.detailLines.prefix(3).enumerated().compactMap { offset, line in
            let preview = normalizeWhitespace(line)
            guard !preview.isEmpty else {
                return nil
            }
            return QuestionEvidenceRef(
                sourceType: "focus_cache",
                sourceID: "\(item.id):detail:\(offset)",
                createdAt: parseDate(item.timestamp),
                label: item.title,
                preview: preview
            )
        }
    }

    private func signalFlags(detailText: String, events: [FocusNormalizedEvent]) -> QuestionSignals {
        let lower = detailText.lowercased()
        let eventText = events.map(\.normalizedText).joined(separator: " ").lowercased()
        let combined = lower + " " + eventText
        return QuestionSignals(
            openLoop: containsAny(combined, [
                "open loop", "unresolved", "blocker", "blocking", "follow up", "follow-up", "next step",
                "need ", "needs ", "owner", "deadline", "close", "closure", "question"
            ]),
            decision: containsAny(combined, [
                "decision", "decide", "approval", "scope", "prd", "launch", "ship", "commit",
                "owner", "deadline", "closure", "priority"
            ]),
            replyNeeded: containsAny(combined, [
                "please", "reply", "respond", "asked", "ask ", "can you", "need your", "feedback",
                "review", "clarify", "help"
            ]),
            execQuestion: containsAny(combined, [
                "exec", "jeetu", "peter", "leadership", "questions running", "executive", "svf", "svp"
            ]),
            relationship: containsAny(combined, [
                "relationship", "trust", "influence", "communication style", "decision style",
                "influence pattern", "relationship to"
            ])
        )
    }

    private func recencyScore(_ date: Date?, now: Date) -> Double {
        guard let date else {
            return 0
        }
        let age = now.timeIntervalSince(date)
        if age <= 24 * 60 * 60 { return 36 }
        if age <= 3 * 24 * 60 * 60 { return 28 }
        if age <= 7 * 24 * 60 * 60 { return 18 }
        if age <= 30 * 24 * 60 * 60 { return 8 }
        return 2
    }

    private func isSummaryStale(latestEventDate: Date?, analysisGeneratedDate: Date?) -> Bool {
        guard let latestEventDate, let analysisGeneratedDate else {
            return false
        }
        return latestEventDate.timeIntervalSince(analysisGeneratedDate) > 30 * 60
    }

    private func isPersonSilent(latestEventDate: Date?, now: Date) -> Bool {
        guard let latestEventDate else {
            return false
        }
        return now.timeIntervalSince(latestEventDate) > 7 * 24 * 60 * 60
    }

    private func firstDate(in lines: [String], prefixes: [String]) -> Date? {
        for line in lines {
            let trimmed = line.trimmingCharacters(in: .whitespacesAndNewlines)
            for prefix in prefixes where trimmed.localizedCaseInsensitiveContains(prefix) {
                let value = trimmed.replacingOccurrences(of: prefix, with: "")
                    .trimmingCharacters(in: .whitespacesAndNewlines)
                if let date = parseDate(value) {
                    return date
                }
            }
        }
        return nil
    }

    private func searchText(_ item: FocusItem) -> String {
        var parts = [item.title, item.subtitle, item.meta, item.timestamp, item.statusBadge]
        parts.append(contentsOf: item.detailIntroLines)
        parts.append(contentsOf: item.detailLines)
        for section in item.detailSections {
            parts.append(section.header)
            parts.append(contentsOf: section.lines)
        }
        parts.append(contentsOf: item.detailTailLines)
        return parts.joined(separator: " ")
    }

    private func dedupe(_ candidates: [QuestionCandidate]) -> [QuestionCandidate] {
        var byID: [String: QuestionCandidate] = [:]
        for candidate in candidates {
            if let current = byID[candidate.id],
               current.priorityScore >= candidate.priorityScore {
                continue
            }
            byID[candidate.id] = candidate
        }
        return byID.values.sorted { left, right in
            if left.priorityScore == right.priorityScore {
                return left.updatedAt > right.updatedAt
            }
            return left.priorityScore > right.priorityScore
        }
    }

    private func parseDate(_ value: String) -> Date? {
        let trimmed = value.trimmingCharacters(in: .whitespacesAndNewlines)
        guard !trimmed.isEmpty else {
            return nil
        }
        if let date = timestampFormatter.date(from: trimmed) {
            return date
        }
        let internetFormatter = ISO8601DateFormatter()
        internetFormatter.formatOptions = [.withInternetDateTime]
        if let date = internetFormatter.date(from: trimmed) {
            return date
        }
        for formatter in localDateFormatters {
            if let date = formatter.date(from: trimmed) {
                return date
            }
        }
        return nil
    }

    private func displayTimestamp(_ value: String) -> String {
        guard let date = parseDate(value) else {
            return value
        }
        return displayDate(date)
    }

    private func displayDate(_ date: Date) -> String {
        Self.localDisplayFormatter.string(from: date)
    }

    private func containsAny(_ text: String, _ needles: [String]) -> Bool {
        needles.contains { text.contains($0) }
    }

    private func normalizedKey(_ value: String) -> String {
        value
            .lowercased()
            .replacingOccurrences(of: #"[^a-z0-9]+"#, with: "-", options: .regularExpression)
            .trimmingCharacters(in: CharacterSet(charactersIn: "-"))
    }

    private func normalizeWhitespace(_ value: String) -> String {
        value
            .trimmingCharacters(in: .whitespacesAndNewlines)
            .replacingOccurrences(of: #"\s+"#, with: " ", options: .regularExpression)
    }

    private static func normalizedPublicationText(_ value: String) -> String {
        value
            .lowercased()
            .trimmingCharacters(in: .whitespacesAndNewlines)
            .replacingOccurrences(of: #"\s+"#, with: " ", options: .regularExpression)
    }

    private static func stableHash(_ value: String) -> String {
        let digest = SHA256.hash(data: Data(value.utf8))
        return digest.map { String(format: "%02x", $0) }.joined()
    }

    private var localDateFormatters: [DateFormatter] {
        [Self.localSecondFormatter, Self.localMinuteFormatter]
    }

    private static let localDisplayFormatter: DateFormatter = {
        let formatter = DateFormatter()
        formatter.locale = Locale(identifier: "en_US_POSIX")
        formatter.timeZone = .current
        formatter.dateFormat = "MM/dd/yyyy HH:mm:ss z"
        return formatter
    }()

    private static let localSecondFormatter: DateFormatter = {
        let formatter = DateFormatter()
        formatter.locale = Locale(identifier: "en_US_POSIX")
        formatter.dateFormat = "yyyy-MM-dd HH:mm:ss z"
        return formatter
    }()

    private static let localMinuteFormatter: DateFormatter = {
        let formatter = DateFormatter()
        formatter.locale = Locale(identifier: "en_US_POSIX")
        formatter.dateFormat = "yyyy-MM-dd HH:mm z"
        return formatter
    }()
}

private struct QuestionDraft {
    var questionType: String
    var questionText: String
    var tags: [String]
    var baseScore: Double
    var drivers: [String]
}

private struct QuestionSignals {
    var openLoop: Bool
    var decision: Bool
    var replyNeeded: Bool
    var execQuestion: Bool
    var relationship: Bool
}

private struct CoreFocusMessage {
    var sourceID: String
    var threadID: String
    var sourceType: String
    var message: WebexQuestionGeneratorCore.Message
    var evidence: QuestionEvidenceRef
}

private struct ParsedCoreMessageLine {
    var sourceType: String
    var timestamp: String
    var sender: String
    var room: String
    var text: String
}
