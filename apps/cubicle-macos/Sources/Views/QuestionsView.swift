import SwiftUI

struct QuestionsListView: View {
    @EnvironmentObject private var model: AppModel
    @State private var scopeFilter: QuestionScopeFilter = .active
    @State private var statusFilter: QuestionStatusFilter = .active

    private var filteredQuestions: [QuestionCandidate] {
        model.questionCandidates.filter { question in
            scopeFilter.includes(question) && statusFilter.includes(question)
        }
        .sorted { left, right in
            if left.priorityScore == right.priorityScore {
                return left.updatedAt > right.updatedAt
            }
            return left.priorityScore > right.priorityScore
        }
    }

    var body: some View {
        VStack(alignment: .leading, spacing: 12) {
            HStack {
                VStack(alignment: .leading, spacing: 4) {
                    Text("Questions")
                        .font(.title2.bold())
                    Text("Based on what changed, these are the questions worth asking now.")
                        .font(.caption)
                        .foregroundStyle(.secondary)
                }
                Spacer()
                Button {
                    Task { await model.refreshQuestions() }
                } label: {
                    Label("Refresh Questions", systemImage: "arrow.clockwise")
                }
                .disabled(model.questionsIsLoading)
            }

            VStack(alignment: .leading, spacing: 8) {
                Picker("Scope", selection: $scopeFilter) {
                    ForEach(QuestionScopeFilter.allCases) { filter in
                        Text(filter.title).tag(filter)
                    }
                }
                .pickerStyle(.segmented)

                Picker("Status", selection: $statusFilter) {
                    ForEach(QuestionStatusFilter.allCases) { filter in
                        Text(filter.title).tag(filter)
                    }
                }
                .pickerStyle(.segmented)
            }

            if model.questionsIsLoading {
                HStack(spacing: 8) {
                    ProgressView()
                        .controlSize(.small)
                    Text("Refreshing question candidates...")
                        .foregroundStyle(.secondary)
                }
                .font(.caption)
            }

            if let error = model.questionsLastError {
                Text(error)
                    .font(.caption)
                    .foregroundStyle(.red)
                    .textSelection(.enabled)
            }

            List(selection: selectedQuestionBinding) {
                ForEach(filteredQuestions) { question in
                    QuestionRow(question: question)
                        .tag(question.id)
                        .listRowInsets(EdgeInsets(top: 6, leading: 14, bottom: 6, trailing: 10))
                }
            }
            .listStyle(.plain)
        }
        .frame(maxWidth: .infinity, maxHeight: .infinity, alignment: .topLeading)
        .padding(20)
        .navigationTitle("Questions")
        .task {
            await model.loadQuestions()
        }
    }

    private var selectedQuestionBinding: Binding<String?> {
        Binding<String?>(
            get: { model.selectedQuestion()?.id },
            set: { model.setSelectedQuestionID($0) }
        )
    }
}

struct QuestionDetailView: View {
    @EnvironmentObject private var model: AppModel

    var body: some View {
        ScrollView {
            if let question = model.selectedQuestion() {
                VStack(alignment: .leading, spacing: 18) {
                    header(question)
                    whyNow(question)
                    evidence(question)
                    metadata(question)
                    actions(question)
                }
                .padding(24)
                .frame(maxWidth: .infinity, alignment: .leading)
            } else {
                PlaceholderView(
                    title: "No Question Selected",
                    symbolName: "questionmark.bubble",
                    message: "Refresh the Question Engine or select a question from the list."
                )
                .padding(24)
            }
        }
        .navigationTitle(model.selectedQuestion()?.scopeLabel ?? "Question Detail")
    }

    private func header(_ question: QuestionCandidate) -> some View {
        VStack(alignment: .leading, spacing: 10) {
            HStack(alignment: .top) {
                VStack(alignment: .leading, spacing: 8) {
                    Text(question.questionText)
                        .font(.largeTitle.bold())
                    HStack(spacing: 8) {
                        Badge(text: question.scopeType.title, color: .blue)
                        Badge(text: readableQuestionType(question.questionType), color: .secondary)
                        QuestionStatusBadge(status: question.status)
                        Badge(text: String(format: "Priority %.0f", question.priorityScore), color: .orange)
                    }
                }
                Spacer()
            }
            Text(question.scopeLabel)
                .font(.title3.weight(.semibold))
                .foregroundStyle(.secondary)
        }
    }

    private func whyNow(_ question: QuestionCandidate) -> some View {
        VStack(alignment: .leading, spacing: 8) {
            Text("Why Now")
                .font(.headline)
            Text(question.whyNow)
                .font(.body)
                .lineSpacing(2)
                .textSelection(.enabled)
        }
        .padding(16)
        .background(.regularMaterial, in: RoundedRectangle(cornerRadius: 14, style: .continuous))
    }

    private func evidence(_ question: QuestionCandidate) -> some View {
        VStack(alignment: .leading, spacing: 10) {
            Text("Evidence")
                .font(.headline)
            if question.evidence.isEmpty {
                Text("No evidence references were attached. This question is lower confidence.")
                    .foregroundStyle(.secondary)
            } else {
                ForEach(question.evidence) { ref in
                    VStack(alignment: .leading, spacing: 4) {
                        Text(ref.label)
                            .font(.subheadline.weight(.semibold))
                        Text(DisplayFormatters.linkifiedText(ref.preview))
                            .font(.body)
                            .lineSpacing(2)
                            .textSelection(.enabled)
                        HStack {
                            Badge(text: ref.sourceType, color: .secondary)
                            Text(ref.sourceID)
                                .font(.caption.monospaced())
                                .foregroundStyle(.secondary)
                                .lineLimit(1)
                                .textSelection(.enabled)
                        }
                    }
                    .padding(.vertical, 6)
                    Divider()
                }
            }
        }
        .padding(16)
        .background(.regularMaterial, in: RoundedRectangle(cornerRadius: 14, style: .continuous))
    }

    private func metadata(_ question: QuestionCandidate) -> some View {
        Grid(alignment: .leading, horizontalSpacing: 16, verticalSpacing: 8) {
            GridRow {
                Text("Scope")
                    .font(.caption.weight(.semibold))
                Text("\(question.scopeType.rawValue): \(question.scopeKey)")
                    .textSelection(.enabled)
            }
            GridRow {
                Text("Source")
                    .font(.caption.weight(.semibold))
                Text("\(question.sourceKind): \(question.sourceKey)")
                    .textSelection(.enabled)
            }
            GridRow {
                Text("Tags")
                    .font(.caption.weight(.semibold))
                Text(question.tags.joined(separator: ", "))
                    .textSelection(.enabled)
            }
            GridRow {
                Text("Created")
                    .font(.caption.weight(.semibold))
                Text(readableDate(question.createdAt))
            }
            GridRow {
                Text("Updated")
                    .font(.caption.weight(.semibold))
                Text(readableDate(question.updatedAt))
            }
            if let expiresAt = question.expiresAt {
                GridRow {
                    Text("Expires")
                        .font(.caption.weight(.semibold))
                    Text(readableDate(expiresAt))
                }
            }
        }
        .font(.body)
        .padding(16)
        .background(.regularMaterial, in: RoundedRectangle(cornerRadius: 14, style: .continuous))
    }

    private func actions(_ question: QuestionCandidate) -> some View {
        HStack(spacing: 10) {
            Button("Refresh") {
                Task { await model.refreshQuestions() }
            }
            Button("Mark Surfaced") {
                Task { await model.updateQuestionStatus(id: question.id, status: .surfaced) }
            }
            Button("Snooze 24h") {
                Task { await model.updateQuestionStatus(id: question.id, status: .snoozed) }
            }
            Button("Dismiss") {
                Task { await model.updateQuestionStatus(id: question.id, status: .dismissed) }
            }
        }
        .buttonStyle(.bordered)
    }

    private func readableDate(_ date: Date) -> String {
        Self.dateFormatter.string(from: date)
    }

    private func readableQuestionType(_ value: String) -> String {
        value
            .split(separator: "_")
            .map { word in
                String(word.prefix(1)).uppercased() + String(word.dropFirst())
            }
            .joined(separator: " ")
    }

    private static let dateFormatter: DateFormatter = {
        let formatter = DateFormatter()
        formatter.locale = Locale(identifier: "en_US_POSIX")
        formatter.timeZone = .current
        formatter.dateFormat = "MM/dd/yyyy HH:mm:ss z"
        return formatter
    }()
}

private struct QuestionRow: View {
    let question: QuestionCandidate

    var body: some View {
        VStack(alignment: .leading, spacing: 8) {
            HStack(alignment: .top) {
                Text(question.questionText)
                    .font(.headline)
                    .lineLimit(2)
                Spacer()
                Text(String(format: "%.0f", question.priorityScore))
                    .font(.caption.monospacedDigit().weight(.semibold))
                    .foregroundStyle(.secondary)
            }
            Text(question.whyNow)
                .font(.caption)
                .foregroundStyle(.secondary)
                .lineLimit(2)
            HStack(spacing: 6) {
                Badge(text: compactScopeLabel, color: .blue)
                Badge(text: question.scopeType.title, color: .secondary)
                QuestionStatusBadge(status: question.status)
                Badge(text: "\(question.evidence.count) evidence", color: .secondary)
            }
            .frame(maxWidth: .infinity, alignment: .leading)
        }
        .frame(maxWidth: .infinity, alignment: .leading)
        .padding(.vertical, 8)
    }

    private var compactScopeLabel: String {
        let trimmed = question.scopeLabel.trimmingCharacters(in: .whitespacesAndNewlines)
        guard trimmed.count > 28 else {
            return trimmed
        }
        return String(trimmed.prefix(28)) + "…"
    }
}

private struct QuestionStatusBadge: View {
    let status: QuestionStatus

    var body: some View {
        Badge(text: status.title.lowercased(), color: color)
    }

    private var color: Color {
        switch status {
        case .candidate: return .orange
        case .surfaced: return .blue
        case .answered: return .green
        case .snoozed: return .secondary
        case .dismissed: return .red
        }
    }
}

private enum QuestionScopeFilter: String, CaseIterable, Identifiable {
    case active
    case person
    case space

    var id: String { rawValue }

    var title: String {
        switch self {
        case .active: return "All"
        case .person: return "People"
        case .space: return "Spaces"
        }
    }

    func includes(_ question: QuestionCandidate) -> Bool {
        switch self {
        case .active: return true
        case .person: return question.scopeType == .person
        case .space: return question.scopeType == .space
        }
    }
}

private enum QuestionStatusFilter: String, CaseIterable, Identifiable {
    case active
    case candidate
    case surfaced
    case snoozed
    case answered
    case dismissed

    var id: String { rawValue }

    var title: String {
        switch self {
        case .active: return "Active"
        case .candidate: return "New"
        case .surfaced: return "Surfaced"
        case .snoozed: return "Snoozed"
        case .answered: return "Answered"
        case .dismissed: return "Dismissed"
        }
    }

    func includes(_ question: QuestionCandidate) -> Bool {
        switch self {
        case .active:
            return question.status == .candidate || question.status == .surfaced
        case .candidate:
            return question.status == .candidate
        case .surfaced:
            return question.status == .surfaced
        case .snoozed:
            return question.status == .snoozed
        case .answered:
            return question.status == .answered
        case .dismissed:
            return question.status == .dismissed
        }
    }
}
