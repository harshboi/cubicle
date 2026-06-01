import SwiftUI

struct DetailView: View {
    @EnvironmentObject private var model: AppModel
    let kind: FocusKind
    let item: FocusItem?

    var body: some View {
        ScrollView {
            if let item {
                VStack(alignment: .leading, spacing: 18) {
                    header(item)
                    summaryCards(item)
                    generatedSections(item)
                    fallbackLines(item)
                }
                .padding(24)
                .frame(maxWidth: .infinity, alignment: .leading)
            } else {
                PlaceholderView(
                    title: "No \(kind.title) item selected",
                    symbolName: "tray",
                    message: "Load runtime snapshots or select an item from the list."
                )
                .padding(24)
            }
        }
        .navigationTitle(item?.title ?? kind.title)
    }

    private func header(_ item: FocusItem) -> some View {
        VStack(alignment: .leading, spacing: 10) {
            HStack(alignment: .top) {
                VStack(alignment: .leading, spacing: 6) {
                    Text(item.title)
                        .font(.largeTitle.bold())
                    if !item.subtitle.isEmpty {
                        Text(item.subtitle)
                            .font(.body)
                            .foregroundStyle(.secondary)
                    }
                }
                Spacer()
                if !item.timestamp.isEmpty {
                    Badge(text: DisplayFormatters.latestMessageLabel(item.timestamp), color: .secondary)
                }
            }
            HStack(spacing: 8) {
                if !item.badge.isEmpty {
                    Badge(text: item.badge, color: .blue)
                }
                if !item.statusBadge.isEmpty {
                    Badge(text: item.statusBadge, color: .orange)
                }
                if !item.meta.isEmpty {
                    Badge(text: item.meta, color: .secondary)
                }
                if let url = webexSpaceURL(for: item) {
                    Link(destination: url) {
                        Label("Open Webex Space", systemImage: "arrow.up.right.square")
                    }
                    .font(.caption.weight(.semibold))
                    .buttonStyle(.borderless)
                }
            }
        }
    }

    private func webexSpaceURL(for item: FocusItem) -> URL? {
        guard kind == .space, item.statusBadge == "live-webex", !item.id.isEmpty else {
            return nil
        }
        let encoded = item.id.addingPercentEncoding(withAllowedCharacters: .urlQueryAllowed) ?? item.id
        return URL(string: "webexteams://im?space=\(encoded)")
    }

    @ViewBuilder
    private func summaryCards(_ item: FocusItem) -> some View {
        let summary = item.firstDetailLine(prefix: "Space summary:")
        let posture = item.firstDetailLine(prefix: "Current posture / next move:")
        let freshness = item.firstDetailLine(prefix: "Guidance freshness:")

        if !summary.isEmpty || !posture.isEmpty || !freshness.isEmpty {
            VStack(alignment: .leading, spacing: 12) {
                if !summary.isEmpty {
                    InsightCard(title: "Space Summary", text: summary, symbolName: "doc.text.magnifyingglass")
                }
                if !posture.isEmpty {
                    InsightCard(title: "Current Posture", text: posture, symbolName: "scope")
                }
                if !freshness.isEmpty {
                    InsightCard(title: "Freshness", text: freshness, symbolName: "checkmark.seal")
                }
            }
        }
    }

    @ViewBuilder
    private func generatedSections(_ item: FocusItem) -> some View {
        let visibleSections = generatedDisplaySections(for: item)
        let scopedQuestions = model.questionCandidates(for: kind, itemID: item.id)
        if !visibleSections.isEmpty || !scopedQuestions.isEmpty {
            VStack(alignment: .leading, spacing: 12) {
                SectionHeader(title: "Generated Sections", subtitle: "Collapsed by default to keep the review flow scannable.")
                if !item.detailIntroLines.isEmpty {
                    DetailLinesCard(
                        title: "Context Before Sections",
                        subtitle: "Runtime context lines captured ahead of generated section payloads.",
                        lines: item.detailIntroLines
                    )
                }
                if !item.detailTailLines.isEmpty {
                    DetailLinesCard(
                        title: "Focus Intelligence",
                        subtitle: "Cached profile, posture, risks, priorities, and topic stance notes from the focus runtime.",
                        lines: item.detailTailLines,
                        style: .intelligence
                    )
                }
                if !scopedQuestions.isEmpty {
                    ScopedQuestionsCard(questions: scopedQuestions)
                }
                if !visibleSections.isEmpty {
                    generatedSectionControls(item, sections: visibleSections)
                }
                ForEach(visibleSections) { section in
                    GeneratedSectionCard(
                        section: section,
                        isExpanded: Binding(
                            get: {
                                model.isDetailSectionExpanded(
                                    for: kind,
                                    itemID: item.id,
                                    sectionID: section.id
                                )
                            },
                            set: {
                                model.setDetailSectionExpanded(
                                    $0,
                                    for: kind,
                                    itemID: item.id,
                                    sectionID: section.id
                                )
                            }
                        )
                    )
                }
            }
        }
    }

    private func generatedDisplaySections(for item: FocusItem) -> [FocusDetailSection] {
        item.detailSections.filter { section in
            !section.header.localizedCaseInsensitiveContains("Meaningful topics from Codex")
        }
    }

    private func generatedSectionControls(_ item: FocusItem, sections: [FocusDetailSection]) -> some View {
        let expanded = sections.filter { section in
            model.isDetailSectionExpanded(
                for: kind,
                itemID: item.id,
                sectionID: section.id
            )
        }.count
        return HStack(spacing: 10) {
            Text("\(expanded)/\(sections.count) expanded")
                .font(.caption)
                .foregroundStyle(.secondary)
            Spacer()
            Button("Expand All") {
                model.setAllDetailSectionsExpanded(
                    true,
                    for: kind,
                    itemID: item.id,
                    sectionIDs: sections.map(\.id)
                )
            }
            .disabled(expanded == sections.count)
            Button("Collapse All") {
                model.setAllDetailSectionsExpanded(
                    false,
                    for: kind,
                    itemID: item.id,
                    sectionIDs: sections.map(\.id)
                )
            }
            .disabled(expanded == 0)
        }
        .buttonStyle(.borderless)
    }

    @ViewBuilder
    private func fallbackLines(_ item: FocusItem) -> some View {
        if item.detailSections.isEmpty {
            VStack(alignment: .leading, spacing: 8) {
                SectionHeader(title: "Details", subtitle: "Raw cache detail lines from the current runtime.")
                if !item.detailLines.isEmpty {
                    ForEach(Array(item.detailLines.enumerated()), id: \.offset) { _, line in
                        if line.trimmingCharacters(in: .whitespacesAndNewlines).isEmpty {
                            Divider()
                        } else {
                            DetailLineRow(line: line, style: .metadata)
                        }
                    }
                } else {
                    if !item.detailIntroLines.isEmpty {
                        DetailLinesCard(
                            title: "Context Before Sections",
                            subtitle: "Runtime context lines available without generated sections.",
                            lines: item.detailIntroLines
                        )
                    }
                    if !item.detailTailLines.isEmpty {
                        DetailLinesCard(
                            title: "Context After Sections",
                            subtitle: "Additional runtime context lines available without generated sections.",
                            lines: item.detailTailLines
                        )
                    }
                }
            }
            .padding(16)
            .background(.regularMaterial, in: RoundedRectangle(cornerRadius: 14, style: .continuous))
        }
    }
}

private struct InsightCard: View {
    let title: String
    let text: String
    let symbolName: String

    var body: some View {
        HStack(alignment: .top, spacing: 12) {
            Image(systemName: symbolName)
                .foregroundStyle(.tint)
                .frame(width: 24)
            VStack(alignment: .leading, spacing: 5) {
                Text(title)
                    .font(.headline)
                Text(DisplayFormatters.linkifiedText(text))
                    .font(.body)
                    .lineSpacing(2)
                    .foregroundStyle(.secondary)
                    .textSelection(.enabled)
            }
            Spacer()
        }
        .padding(16)
        .background(.regularMaterial, in: RoundedRectangle(cornerRadius: 14, style: .continuous))
    }
}

private enum DetailLinesCardStyle {
    case metadata
    case intelligence
}

private struct DetailLinesCard: View {
    let title: String
    let subtitle: String
    let lines: [String]
    var style: DetailLinesCardStyle = .metadata

    private var visibleLines: [String] {
        lines.filter { line in
            !line.trimmingCharacters(in: .whitespacesAndNewlines)
                .localizedCaseInsensitiveContains("Room ID:")
        }
    }

    var body: some View {
        VStack(alignment: .leading, spacing: 8) {
            Text(title)
                .font(.headline)
            Text(subtitle)
                .font(.caption)
                .foregroundStyle(.secondary)
            ForEach(Array(visibleLines.enumerated()), id: \.offset) { _, line in
                if line.trimmingCharacters(in: .whitespacesAndNewlines).isEmpty {
                    Divider()
                } else {
                    DetailLineRow(line: line, style: style)
                }
            }
        }
        .padding(14)
        .background(.regularMaterial, in: RoundedRectangle(cornerRadius: 12, style: .continuous))
    }
}

private struct ScopedQuestionsCard: View {
    @EnvironmentObject private var model: AppModel
    let questions: [QuestionCandidate]

    var body: some View {
        VStack(alignment: .leading, spacing: 12) {
            HStack {
                VStack(alignment: .leading, spacing: 4) {
                    Text("Questions Worth Asking Now")
                        .font(.headline)
                    Text("Generated from recent person and space evidence.")
                        .font(.caption)
                        .foregroundStyle(.secondary)
                }
                Spacer()
                Button("Open Questions") {
                    model.setSelectedQuestionID(questions.first?.id)
                    model.select(section: .questions)
                }
                .buttonStyle(.borderless)
            }

            ForEach(questions) { question in
                VStack(alignment: .leading, spacing: 6) {
                    HStack(alignment: .firstTextBaseline) {
                        Text(question.questionText)
                            .font(.body.weight(.semibold))
                            .lineLimit(2)
                        Spacer()
                        Badge(text: String(format: "%.0f", question.priorityScore), color: .orange)
                    }
                    Text(question.whyNow)
                        .font(.caption)
                        .foregroundStyle(.secondary)
                        .lineLimit(2)
                    HStack(spacing: 6) {
                        Badge(text: question.questionType.replacingOccurrences(of: "_", with: " "), color: .secondary)
                        Badge(text: "\(question.evidence.count) evidence", color: .secondary)
                    }
                }
                .contentShape(Rectangle())
                .onTapGesture {
                    model.setSelectedQuestionID(question.id)
                    model.select(section: .questions)
                }
                Divider()
            }
        }
        .padding(14)
        .background(.regularMaterial, in: RoundedRectangle(cornerRadius: 12, style: .continuous))
    }
}

private struct DetailLineRow: View {
    let line: String
    let style: DetailLinesCardStyle

    var body: some View {
        let readable = DisplayFormatters.readableDetailLine(line)
        let trimmed = readable.trimmingCharacters(in: .whitespacesAndNewlines)

        switch style {
        case .metadata:
            Text(DisplayFormatters.linkifiedText(
                trimmed,
                boldPrefixThrough: labelDelimiter(in: trimmed)
            ))
                .font(.body)
                .lineSpacing(2)
                .textSelection(.enabled)
                .frame(maxWidth: .infinity, alignment: .leading)
        case .intelligence:
            if isStandaloneHeading(trimmed) {
                Text(DisplayFormatters.linkifiedText(trimmed))
                    .font(.headline.weight(.semibold))
                    .foregroundStyle(.primary)
                    .padding(.top, 6)
                    .padding(.bottom, 1)
                    .textSelection(.enabled)
                    .frame(maxWidth: .infinity, alignment: .leading)
            } else if let bullet = bulletBody(trimmed) {
                HStack(alignment: .firstTextBaseline, spacing: 8) {
                    Text("•")
                        .font(.body.weight(.semibold))
                        .foregroundStyle(.secondary)
                    Text(DisplayFormatters.linkifiedText(
                        bullet,
                        boldPrefixThrough: labelDelimiter(in: bullet)
                    ))
                    .font(.body)
                    .lineSpacing(2)
                    .textSelection(.enabled)
                    .frame(maxWidth: .infinity, alignment: .leading)
                }
                .padding(.leading, 4)
            } else {
                Text(DisplayFormatters.linkifiedText(
                    trimmed,
                    boldPrefixThrough: labelDelimiter(in: trimmed)
                ))
                .font(.body)
                .lineSpacing(2)
                .textSelection(.enabled)
                .frame(maxWidth: .infinity, alignment: .leading)
            }
        }
    }

    private func isStandaloneHeading(_ text: String) -> Bool {
        guard text.hasSuffix(":"),
              !text.hasPrefix("-"),
              text.count <= 80 else {
            return false
        }
        let heading = text.dropLast()
        return !heading.contains("|") && !heading.contains(".")
    }

    private func bulletBody(_ text: String) -> String? {
        guard text.hasPrefix("- ") else {
            return nil
        }
        return String(text.dropFirst(2)).trimmingCharacters(in: .whitespacesAndNewlines)
    }

    private func labelDelimiter(in text: String) -> String? {
        let lowercased = text.lowercased()
        guard !lowercased.hasPrefix("http://"),
              !lowercased.hasPrefix("https://") else {
            return nil
        }
        guard let colon = text.firstIndex(of: ":") else {
            return nil
        }
        let labelLength = text.distance(from: text.startIndex, to: colon)
        guard labelLength > 0, labelLength <= 42 else {
            return nil
        }
        let label = text[..<colon]
        guard label.rangeOfCharacter(from: .letters) != nil,
              !label.contains("/") else {
            return nil
        }
        return ":"
    }
}

private struct GeneratedSectionCard: View {
    let section: FocusDetailSection
    @Binding var isExpanded: Bool

    private let recentConversationPreviewLimit = 18
    private var isRecentConversationSection: Bool {
        section.header.hasPrefix("Recent conversations (last ")
    }

    private var isConversationSection: Bool {
        section.header.hasPrefix("Conversation ")
    }

    private var shouldCompactConversationLines: Bool {
        isRecentConversationSection || isConversationSection
    }

    private var conversationSourceSummary: String? {
        guard shouldCompactConversationLines else { return nil }
        let webexCount = displayLines.reduce(into: 0) { count, line in
            let normalized = line.trimmingCharacters(in: .whitespacesAndNewlines).lowercased()
            if normalized.hasPrefix("webex ") || normalized.hasPrefix("space message ") {
                count += 1
            }
        }
        let iMessageCount = displayLines.reduce(into: 0) { count, line in
            let normalized = line.trimmingCharacters(in: .whitespacesAndNewlines).lowercased()
            if normalized.hasPrefix("imessage ") {
                count += 1
            }
        }
        guard webexCount > 0 || iMessageCount > 0 else {
            return nil
        }
        if webexCount > 0 && iMessageCount > 0 {
            return "webex \(webexCount) • iMessage \(iMessageCount)"
        }
        if iMessageCount > 0 {
            return "iMessage \(iMessageCount)"
        }
        return "webex \(webexCount)"
    }

    private var displayLines: [String] {
        if shouldCompactConversationLines {
            return section.lines.filter { !isNoisyRecentConversationLine($0) }
        }
        return section.lines
    }

    private var visibleLines: [String] {
        if shouldCompactConversationLines {
            return Array(displayLines.prefix(recentConversationPreviewLimit))
        }
        return displayLines
    }

    var body: some View {
        VStack(alignment: .leading, spacing: 0) {
            Button {
                withAnimation(.easeInOut(duration: 0.16)) {
                    isExpanded.toggle()
                }
            } label: {
                HStack(spacing: 8) {
                    Image(systemName: isExpanded ? "chevron.down" : "chevron.right")
                        .font(.caption.weight(.semibold))
                        .foregroundStyle(.secondary)
                        .frame(width: 12)
                    Text(section.header)
                        .font(.headline)
                        .foregroundStyle(.primary)
                    Spacer()
                    Text(sectionCountLabel)
                        .font(.caption)
                        .foregroundStyle(.secondary)
                }
                .frame(maxWidth: .infinity, alignment: .leading)
                .contentShape(Rectangle())
            }
            .buttonStyle(.plain)
            .accessibilityLabel(section.header)
            .accessibilityValue(isExpanded ? "Expanded" : "Collapsed")
            .accessibilityHint("Click to \(isExpanded ? "collapse" : "expand") this section.")

            if isExpanded {
                VStack(alignment: .leading, spacing: 8) {
                    if !section.summarySource.isEmpty || !section.summaryGeneratedAt.isEmpty {
                        HStack {
                            if !section.summarySource.isEmpty {
                                Badge(text: section.summarySource, color: .blue)
                            }
                            if !section.summaryGeneratedAt.isEmpty {
                                Badge(text: section.summaryGeneratedAt, color: .secondary)
                            }
                        }
                        .padding(.bottom, 4)
                    }

                    ForEach(Array(visibleLines.enumerated()), id: \.offset) { _, line in
                        if shouldCompactConversationLines {
                            RecentConversationRow(
                                line: RecentConversationLineFormatter.format(
                                    line,
                                    sectionRoomTitle: section.roomTitle
                                )
                            )
                        } else {
                            Text(attributedLine(line))
                                .font(.system(.body, design: .default))
                                .textSelection(.enabled)
                                .frame(maxWidth: .infinity, alignment: .leading)
                        }
                    }

                    EmptyView()
                }
                .padding(.top, 12)
            }
        }
        .padding(16)
        .background(.regularMaterial, in: RoundedRectangle(cornerRadius: 14, style: .continuous))
    }

    private var sectionCountLabel: String {
        let sourceSummary = conversationSourceSummary
        if shouldCompactConversationLines, displayLines.count > visibleLines.count {
            if let sourceSummary {
                return "\(visibleLines.count)/\(displayLines.count) shown • \(sourceSummary)"
            }
            return "\(visibleLines.count)/\(displayLines.count) shown"
        }
        if let sourceSummary {
            return "\(displayLines.count) lines • \(sourceSummary)"
        }
        return "\(displayLines.count) lines"
    }

    private func attributedLine(_ line: String) -> AttributedString {
        DisplayFormatters.linkifiedText(DisplayFormatters.readableDetailLine(line), boldSoWhat: true)
    }

    private func isNoisyRecentConversationLine(_ line: String) -> Bool {
        let normalized = line
            .trimmingCharacters(in: .whitespacesAndNewlines)
            .lowercased()
        return normalized.hasPrefix("conversation extraction source:")
            || normalized.hasPrefix("room topic analysis source:")
    }
}

private struct RecentConversationRow: View {
    let line: FormattedRecentConversationLine
    @State private var showingFullMessage = false

    var body: some View {
        HStack(alignment: .firstTextBaseline, spacing: 8) {
            Text(DisplayFormatters.linkifiedText(
                line.previewLine,
                boldPrefixThrough: line.boldPrefixThrough
            ))
                .font(.system(.body, design: .default))
                .textSelection(.enabled)
                .frame(maxWidth: .infinity, alignment: .leading)

            if line.isTruncated {
                Button("Full") {
                    showingFullMessage = true
                }
                .font(.caption.weight(.semibold))
                .buttonStyle(.borderless)
                .accessibilityLabel("Show full message")
            }
        }
            .frame(maxWidth: .infinity, alignment: .leading)
            .contentShape(Rectangle())
            .onTapGesture {
                if line.isTruncated {
                    showingFullMessage = true
                }
            }
            .popover(isPresented: $showingFullMessage, arrowEdge: .trailing) {
                popoverContent
            }
    }

    private var popoverContent: some View {
        VStack(alignment: .leading, spacing: 12) {
            Text(line.header)
                .font(.headline)
            Divider()
            ScrollView {
                Text(DisplayFormatters.linkifiedText(line.fullMessage))
                    .font(.body)
                    .textSelection(.enabled)
                    .frame(maxWidth: .infinity, alignment: .leading)
            }
            .frame(maxHeight: 320)
            Text("Links and local data-source paths are clickable.")
                .font(.caption)
                .foregroundStyle(.secondary)
        }
        .padding(16)
        .frame(width: 560)
    }
}

private struct FormattedRecentConversationLine {
    var header: String
    var previewLine: String
    var fullMessage: String
    var isTruncated: Bool
    var boldPrefixThrough: String?
}

private enum RecentConversationLineFormatter {
    static let headerDelimiter = " - "

    private static let messagePreviewLimit = 260

    static func format(_ rawLine: String, sectionRoomTitle: String) -> FormattedRecentConversationLine {
        let line = rawLine.trimmingCharacters(in: .whitespacesAndNewlines)
        if let metadataLine = formatMetadataLine(line) {
            return metadataLine
        }

        guard let parsed = parsedPayloadAfterSourceLabel(line) else {
            return FormattedRecentConversationLine(
                header: "",
                previewLine: DisplayFormatters.readableDetailLine(line),
                fullMessage: DisplayFormatters.readableDetailLine(line),
                isTruncated: false,
                boldPrefixThrough: nil
            )
        }
        let source = parsed.source
        let payload = parsed.payload

        let fields = payload
            .split(separator: "|", maxSplits: 3, omittingEmptySubsequences: false)
            .map { String($0).trimmingCharacters(in: .whitespacesAndNewlines) }
        guard fields.count >= 2 else {
            return FormattedRecentConversationLine(
                header: "",
                previewLine: payload,
                fullMessage: payload,
                isTruncated: false,
                boldPrefixThrough: nil
            )
        }

        let timestamp = readableTimestamp(fields[0])
        let sender = fields[1]
        let room = fields.count >= 4 ? fields[2] : ""
        let message: String
        if fields.count >= 4 {
            message = fields[3]
        } else if fields.count == 3 {
            message = fields[2]
        } else {
            message = ""
        }

        var parts: [String] = [source]
        parts.append(timestamp)
        if !sender.isEmpty {
            parts.append(sender)
        }
        if shouldShowRoom(room, sender: sender, sectionRoomTitle: sectionRoomTitle) {
            parts.append("in \(room)")
        }

        let prefix = parts.filter { !$0.isEmpty }.joined(separator: " | ")
        let normalizedMessage = normalizeMessage(message)
        let preview = previewMessage(normalizedMessage)
        let previewLine = preview.isEmpty ? prefix : "\(prefix)\(headerDelimiter)\(preview)"
        return FormattedRecentConversationLine(
            header: prefix,
            previewLine: previewLine,
            fullMessage: normalizedMessage.isEmpty ? previewLine : normalizedMessage,
            isTruncated: normalizedMessage.count > messagePreviewLimit,
            boldPrefixThrough: headerDelimiter
        )
    }

    private static func formatMetadataLine(_ line: String) -> FormattedRecentConversationLine? {
        let mappings = [
            ("Date:", "Conversation Started:"),
            ("Date range:", "Date Range:"),
            ("Space:", "Space:"),
            ("Channels:", "Space:"),
            ("Initiated by:", "Initiated By:"),
            ("Anchored on:", "Initiated By:"),
            ("Summary conversation/topic:", "Summary:"),
            ("Status:", "Status:"),
            ("Closed in last 60 days:", "Closed in Last 60 Days:")
        ]

        for (sourcePrefix, displayPrefix) in mappings where line.hasPrefix(sourcePrefix) {
            let value = line.dropFirst(sourcePrefix.count)
                .trimmingCharacters(in: .whitespacesAndNewlines)
            let displayValue: String
            if sourcePrefix == "Date:" {
                displayValue = DisplayFormatters.localDateTime(value)
            } else if sourcePrefix == "Date range:" {
                displayValue = readableDateRange(value)
            } else {
                displayValue = value
            }
            let preview = displayValue.isEmpty ? displayPrefix : "\(displayPrefix) \(displayValue)"
            return FormattedRecentConversationLine(
                header: displayPrefix,
                previewLine: preview,
                fullMessage: preview,
                isTruncated: false,
                boldPrefixThrough: ":"
            )
        }

        return nil
    }

    private static func parsedPayloadAfterSourceLabel(_ line: String) -> (source: String, payload: String)? {
        let patterns: [(String, String)] = [
            (#"^Webex(?:\s+[^:]+)?:\s*(.+)$"#, "Webex"),
            (#"^Space message(?:\s+[^:]+)?:\s*(.+)$"#, "Webex"),
            (#"^iMessage(?:\s+[^:]+)?:\s*(.+)$"#, "iMessage")
        ]
        for (pattern, source) in patterns {
            guard let regex = try? NSRegularExpression(pattern: pattern),
                  let match = regex.firstMatch(
                    in: line,
                    range: NSRange(line.startIndex..<line.endIndex, in: line)
                  ),
                  match.numberOfRanges >= 2,
                  let payloadRange = Range(match.range(at: 1), in: line) else {
                continue
            }
            return (
                source: source,
                payload: String(line[payloadRange]).trimmingCharacters(in: .whitespacesAndNewlines)
            )
        }
        return nil
    }

    private static func readableDateRange(_ rawValue: String) -> String {
        let parts = rawValue
            .components(separatedBy: "->")
            .map { $0.trimmingCharacters(in: .whitespacesAndNewlines) }
            .filter { !$0.isEmpty }
        guard parts.count == 2 else {
            return rawValue
        }
        return "\(readableTimestamp(parts[0])) -> \(readableTimestamp(parts[1]))"
    }

    private static func normalizeMessage(_ value: String) -> String {
        value
            .trimmingCharacters(in: .whitespacesAndNewlines)
            .replacingOccurrences(of: #"\s+"#, with: " ", options: .regularExpression)
    }

    private static func previewMessage(_ normalized: String) -> String {
        guard normalized.count > messagePreviewLimit else {
            return normalized
        }
        let index = normalized.index(normalized.startIndex, offsetBy: messagePreviewLimit)
        let prefix = normalized[..<index].trimmingCharacters(in: .whitespacesAndNewlines)
        return "\(prefix)..."
    }

    private static func shouldShowRoom(_ room: String, sender: String, sectionRoomTitle: String) -> Bool {
        let trimmed = room.trimmingCharacters(in: .whitespacesAndNewlines)
        guard !trimmed.isEmpty, !looksLikeWebexID(trimmed) else {
            return false
        }
        let normalizedRoom = normalizedLabel(trimmed)
        return normalizedRoom != normalizedLabel(sender)
            && normalizedRoom != normalizedLabel(sectionRoomTitle)
    }

    private static func readableTimestamp(_ rawValue: String) -> String {
        guard let date = DisplayFormatters.parseDate(rawValue) else {
            return rawValue
        }
        return displayDateFormatter.string(from: date)
    }

    private static func normalizedLabel(_ value: String) -> String {
        value
            .trimmingCharacters(in: .whitespacesAndNewlines)
            .lowercased()
            .replacingOccurrences(of: #"\s+"#, with: " ", options: .regularExpression)
    }

    private static func looksLikeWebexID(_ value: String) -> Bool {
        value.replacingOccurrences(of: #"\s+"#, with: "", options: .regularExpression)
            .hasPrefix("Y2lzY29zcGFyazov")
    }

    private static let displayDateFormatter: DateFormatter = {
        let formatter = DateFormatter()
        formatter.locale = Locale(identifier: "en_US_POSIX")
        formatter.timeZone = .current
        formatter.dateFormat = "MMM d, h:mm a"
        return formatter
    }()
}
