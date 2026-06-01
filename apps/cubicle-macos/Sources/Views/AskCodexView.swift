import SwiftUI

struct AskCodexComposerView: View {
    @EnvironmentObject private var model: AppModel
    @State private var isContextPreviewExpanded = false

    var body: some View {
        ScrollView {
            VStack(alignment: .leading, spacing: 18) {
                SectionHeader(
                    title: "Ask Codex",
                    subtitle: "Run focused questions through the native Codex runner with explicit target scope and context preview."
                )

                VStack(alignment: .leading, spacing: 10) {
                    Text("Target Scope")
                        .font(.headline)
                    Picker("Target Scope", selection: targetScopeBinding) {
                        ForEach(AskCodexTargetScope.allCases) { scope in
                            Text(scope.title).tag(scope)
                        }
                    }
                    .pickerStyle(.menu)
                }
                .padding(14)
                .background(.regularMaterial, in: RoundedRectangle(cornerRadius: 12, style: .continuous))

                if !model.askCodexQueryHistory.isEmpty {
                    VStack(alignment: .leading, spacing: 10) {
                        HStack(alignment: .center, spacing: 10) {
                            Text("Query History")
                                .font(.headline)
                            Spacer()
                            Menu {
                                ForEach(Array(model.askCodexQueryHistory.prefix(30))) { entry in
                                    Button {
                                        model.applyAskCodexQueryHistory(entry)
                                    } label: {
                                        Text(historyMenuTitle(entry))
                                    }
                                }
                            } label: {
                                Label("Choose Previous Question", systemImage: "clock.arrow.circlepath")
                            }
                            .menuStyle(.button)
                        }
                    }
                    .padding(14)
                    .background(.regularMaterial, in: RoundedRectangle(cornerRadius: 12, style: .continuous))
                }

                if let warning = model.askCodexTargetWarning() {
                    AskCodexStatusCard(
                        title: "Scope Not Ready",
                        message: warning,
                        color: .orange,
                        symbolName: "exclamationmark.triangle"
                    )
                }

                VStack(alignment: .leading, spacing: 12) {
                    HStack(alignment: .center, spacing: 10) {
                        Text("Question")
                            .font(.headline)

                        Spacer()

                        Button {
                            Task { await model.runAskCodex() }
                        } label: {
                            Label(model.askCodexIsRunning ? "Running..." : "Run Ask Codex", systemImage: "sparkles")
                        }
                        .buttonStyle(.borderedProminent)
                        .disabled(!model.canRunAskCodex)

                        Button("Clear Question") {
                            model.clearAskCodexQuestion()
                        }
                        .buttonStyle(.bordered)
                        .disabled(model.askCodexQuestion.isEmpty)
                    }

                    TextEditor(text: questionBinding)
                        .font(.system(.body, design: .default))
                        .frame(minHeight: 132)
                        .scrollContentBackground(.hidden)
                        .padding(10)
                        .background(
                            RoundedRectangle(cornerRadius: 10, style: .continuous)
                                .fill(Color(nsColor: .textBackgroundColor).opacity(0.72))
                        )
                        .overlay(
                            RoundedRectangle(cornerRadius: 10, style: .continuous)
                                .stroke(Color.secondary.opacity(0.14), lineWidth: 1)
                        )
                }
                .padding(14)
                .background(.regularMaterial, in: RoundedRectangle(cornerRadius: 12, style: .continuous))

                DisclosureGroup(isExpanded: $isContextPreviewExpanded) {
                    VStack(alignment: .leading, spacing: 6) {
                        if contextLines.isEmpty {
                            Text("No context is currently available for this scope.")
                                .foregroundStyle(.secondary)
                        } else {
                            ForEach(Array(contextLines.enumerated()), id: \.offset) { _, line in
                                Text(line)
                                    .font(.system(.caption, design: .monospaced))
                                    .foregroundStyle(.secondary)
                                    .frame(maxWidth: .infinity, alignment: .leading)
                            }
                        }
                    }
                    .padding(.top, 8)
                } label: {
                    VStack(alignment: .leading, spacing: 4) {
                        Text("Context Preview")
                            .font(.headline)
                        Text(contextSummary)
                            .font(.caption)
                            .foregroundStyle(.secondary)
                    }
                }
                .padding(14)
                .background(.regularMaterial, in: RoundedRectangle(cornerRadius: 12, style: .continuous))

                if let error = model.askCodexLastError, !error.isEmpty {
                    AskCodexStatusCard(
                        title: "Run Failed",
                        message: error,
                        color: .red,
                        symbolName: "xmark.octagon"
                    )
                }
            }
            .padding(24)
            .frame(maxWidth: .infinity, alignment: .leading)
        }
        .navigationTitle("Ask Codex")
    }

    private var targetScopeBinding: Binding<AskCodexTargetScope> {
        Binding(
            get: { model.askCodexTargetScope },
            set: { model.askCodexTargetScope = $0 }
        )
    }

    private var questionBinding: Binding<String> {
        Binding(
            get: { model.askCodexQuestion },
            set: { model.askCodexQuestion = $0 }
        )
    }

    private var contextLines: [String] {
        model.askCodexContextPreviewLines()
    }

    private var contextSummary: String {
        guard !contextLines.isEmpty else {
            return "No context available."
        }
        let characterCount = contextLines.reduce(0) { $0 + $1.count }
        let approximateTokens = max(1, characterCount / 4)
        return "\(contextLines.count) lines, about \(approximateTokens) tokens. Expand to inspect the context sent to Codex."
    }

    private func historyMenuTitle(_ entry: AskCodexQueryHistoryEntry) -> String {
        let question = entry.question.trimmingCharacters(in: .whitespacesAndNewlines)
        let clippedQuestion = question.count > 72 ? "\(question.prefix(72))..." : question
        return "\(entry.targetTitle): \(clippedQuestion)"
    }
}

struct AskCodexResultView: View {
    @EnvironmentObject private var model: AppModel

    var body: some View {
        ScrollView {
            VStack(alignment: .leading, spacing: 16) {
                SectionHeader(
                    title: "Codex Response",
                    subtitle: "Latest native runner output."
                )

                AskCodexTargetResponseCard(
                    title: "Ask Codex: \(model.askCodexCurrentTargetTitle())",
                    scopeTitle: model.askCodexTargetScope.title,
                    warning: model.askCodexTargetWarning()
                )

                if model.askCodexIsRunning {
                    HStack(spacing: 8) {
                        ProgressView()
                            .controlSize(.small)
                        Text("Codex run in progress...")
                            .foregroundStyle(.secondary)
                    }
                    .padding(14)
                    .frame(maxWidth: .infinity, alignment: .leading)
                    .background(.regularMaterial, in: RoundedRectangle(cornerRadius: 12, style: .continuous))
                }

                if let run = model.askCodexResult {
                    VStack(alignment: .leading, spacing: 8) {
                        Text("Last Response")
                            .font(.headline)
                        HStack(spacing: 8) {
                            Badge(text: run.targetScope.title, color: .blue)
                            Badge(text: run.targetTitle, color: .secondary)
                            Badge(text: run.status, color: run.status.lowercased() == "completed" ? .green : .orange)
                            Badge(text: "\(run.attempts) attempt(s)", color: .secondary)
                        }
                        Text(Self.dateFormatter.string(from: run.submittedAt))
                            .font(.caption)
                            .foregroundStyle(.secondary)
                        Text("Question: \(run.question)")
                            .font(.subheadline)
                    }
                    .padding(14)
                    .background(.regularMaterial, in: RoundedRectangle(cornerRadius: 12, style: .continuous))

                    VStack(alignment: .leading, spacing: 8) {
                        Text("Response")
                            .font(.headline)
                        Text(run.output.isEmpty ? "No output returned." : run.output)
                            .font(.system(.body, design: .default))
                            .textSelection(.enabled)
                            .frame(maxWidth: .infinity, alignment: .leading)
                    }
                    .padding(14)
                    .background(.regularMaterial, in: RoundedRectangle(cornerRadius: 12, style: .continuous))
                } else {
                    PlaceholderView(
                        title: "No Ask Codex Run Yet",
                        symbolName: "sparkles",
                        message: "Submit a question from the Ask Codex panel to view response output."
                    )
                    .frame(maxWidth: .infinity, maxHeight: .infinity, alignment: .top)
                }
            }
            .padding(24)
            .frame(maxWidth: .infinity, alignment: .leading)
        }
    }

    private static let dateFormatter: DateFormatter = {
        let formatter = DateFormatter()
        formatter.dateFormat = "yyyy-MM-dd HH:mm:ss z"
        return formatter
    }()
}

private struct AskCodexTargetResponseCard: View {
    let title: String
    let scopeTitle: String
    let warning: String?

    var body: some View {
        VStack(alignment: .leading, spacing: 8) {
            Text(title)
                .font(.headline)
            HStack(spacing: 8) {
                Badge(text: scopeTitle, color: .blue)
                Badge(text: warning == nil ? "ready" : "unavailable", color: warning == nil ? .green : .orange)
            }
            if let warning {
                Text(warning)
                    .font(.caption)
                    .foregroundStyle(.secondary)
            }
        }
        .padding(14)
        .background(.regularMaterial, in: RoundedRectangle(cornerRadius: 12, style: .continuous))
    }
}

private struct AskCodexStatusCard: View {
    let title: String
    let message: String
    let color: Color
    let symbolName: String

    var body: some View {
        HStack(alignment: .top, spacing: 10) {
            Image(systemName: symbolName)
                .foregroundStyle(color)
                .padding(.top, 2)
            VStack(alignment: .leading, spacing: 4) {
                Text(title)
                    .font(.headline)
                    .foregroundStyle(color)
                Text(message)
                    .font(.subheadline)
                    .foregroundStyle(.secondary)
            }
            Spacer()
        }
        .padding(12)
        .background(.regularMaterial, in: RoundedRectangle(cornerRadius: 12, style: .continuous))
    }
}
