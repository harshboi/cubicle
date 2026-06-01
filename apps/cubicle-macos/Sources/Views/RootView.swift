import SwiftUI

/// Top-level app shell that coordinates sidebar, split views, and toolbar actions.
struct RootView: View {
    @EnvironmentObject private var model: AppModel

    var body: some View {
        NavigationSplitView {
            SidebarView()
                .navigationSplitViewColumnWidth(min: 210, ideal: 240, max: 280)
        } content: {
            contentView
                .navigationSplitViewColumnWidth(
                    min: contentColumnWidth.min,
                    ideal: contentColumnWidth.ideal,
                    max: contentColumnWidth.max
                )
        } detail: {
            detailView
        }
        .safeAreaInset(edge: .bottom, spacing: 0) {
            RefreshProgressStatusBar(
                progress: model.refreshProgress,
                codexActivityMessage: model.codexActivityMessage
            )
                .background(.bar)
        }
        .toolbar {
            ToolbarItemGroup {
                MicGainToolbarControl(
                    gain: Binding(
                        get: { model.systemSettings.transcriptionMicrophoneGain },
                        set: { model.updateSystemSetting(.transcriptionMicrophoneGain, intValue: $0) }
                    )
                )
                if model.isLoading {
                    ProgressView()
                        .controlSize(.small)
                }
                Button {
                    Task { await model.refreshSelectedPageNow() }
                } label: {
                    Label("Reload", systemImage: "arrow.clockwise")
                }
                .help("Refresh the selected page with priority over background work")
            }
        }
        .alert("Runtime Error", isPresented: Binding(
            get: { model.errorMessage != nil },
            set: { if !$0 { model.errorMessage = nil } }
        )) {
            Button("OK", role: .cancel) { model.errorMessage = nil }
        } message: {
            Text(model.errorMessage ?? "")
        }
    }

    @ViewBuilder
    private var contentView: some View {
        switch model.selectedSection {
        case .home:
            DashboardView()
        case .spaceFocus:
            FocusListView(kind: .space)
        case .personFocus:
            FocusListView(kind: .person)
        case .spaceFocusTargets:
            FocusTargetManagementView(kind: .spaceFocus)
        case .personFocusTargets:
            FocusTargetManagementView(kind: .personFocus)
        case .execFocusTargets:
            FocusTargetManagementView(kind: .execFocus)
        case .questions:
            QuestionsListView()
        case .transcription:
            TranscriptionOverviewView()
        case .beliefs:
            BeliefsView()
        case .askCodex:
            AskCodexComposerView()
        case .jobs:
            JobsView()
        case .settings:
            SettingsView()
        }
    }

    private var contentColumnWidth: (min: CGFloat, ideal: CGFloat, max: CGFloat) {
        switch model.selectedSection {
        case .questions:
            return (420, 560, 760)
        case .settings:
            return (640, 740, 900)
        default:
            return (340, 420, 520)
        }
    }

    @ViewBuilder
    private var detailView: some View {
        switch model.selectedSection {
        case .spaceFocus:
            DetailView(kind: .space, item: model.selectedItem(for: .space))
        case .personFocus:
            DetailView(kind: .person, item: model.selectedItem(for: .person))
        case .spaceFocusTargets:
            FocusTargetDetailView(kind: .spaceFocus)
        case .personFocusTargets:
            FocusTargetDetailView(kind: .personFocus)
        case .execFocusTargets:
            FocusTargetDetailView(kind: .execFocus)
        case .questions:
            QuestionDetailView()
        case .transcription:
            TranscriptionRightPaneView()
        case .home:
            RuntimeInspectorView()
        case .settings:
            RuntimeInspectorView()
        case .askCodex:
            AskCodexResultView()
        case .beliefs:
            AutomaticBeliefsDetailView()
        case .jobs:
            JobsInspectorView()
        }
    }
}

/// Toolbar control for live microphone gain while transcription is active.
private struct MicGainToolbarControl: View {
    @Binding var gain: Int

    private var sliderValue: Binding<Double> {
        Binding(
            get: { Double(gain) },
            set: { newValue in
                gain = SystemSettings.clamped(
                    Int(newValue.rounded()),
                    to: SystemSettings.transcriptionMicrophoneGainBounds
                )
            }
        )
    }

    private var sliderRange: ClosedRange<Double> {
        let lowerBound = Double(SystemSettings.transcriptionMicrophoneGainBounds.lowerBound)
        let upperBound = Double(SystemSettings.transcriptionMicrophoneGainBounds.upperBound)
        return lowerBound...upperBound
    }

    var body: some View {
        HStack(spacing: 7) {
            Image(systemName: "mic.fill")
                .font(.caption.weight(.semibold))
                .foregroundStyle(.secondary)
            Slider(
                value: sliderValue,
                in: sliderRange,
                step: 1
            )
            .frame(width: 116)
            Text("\(gain)x")
                .font(.caption.monospacedDigit().weight(.semibold))
                .foregroundStyle(.secondary)
                .frame(width: 34, alignment: .trailing)
        }
        .help("Microphone gain limit: \(gain)x")
    }
}

/// Bottom status bar for refresh progress and Codex activity.
private struct RefreshProgressStatusBar: View {
    let progress: RefreshProgressState
    let codexActivityMessage: String?

    var body: some View {
        HStack(spacing: 8) {
            if progress.isActive {
                ProgressView(value: Double(progress.completedStepCount), total: Double(max(progress.totalStepCount, 1)))
                    .frame(width: 112)
                    .controlSize(.small)
            }

            Image(systemName: progress.isActive ? "arrow.triangle.2.circlepath" : "checkmark.circle")
                .font(.caption.weight(.semibold))
                .foregroundStyle(progress.isActive ? .blue : .secondary)

            Text(titleText)
                .font(.caption.weight(.semibold))
                .lineLimit(1)

            Text(detailText)
                .font(.caption)
                .foregroundStyle(.secondary)
                .lineLimit(1)

            Spacer()

            if progress.totalStepCount > 0 {
                Text("\(progress.completedStepCount)/\(progress.totalStepCount)")
                    .font(.caption.monospacedDigit().weight(.semibold))
                    .foregroundStyle(.secondary)
            }
        }
        .frame(minHeight: 28)
        .padding(.horizontal, 12)
        .overlay(alignment: .top) {
            Divider()
        }
    }

    private var titleText: String {
        if progress.isActive {
            return progress.title
        }
        return "Ready"
    }

    private var detailText: String {
        if let codexActivityMessage,
           !codexActivityMessage.trimmingCharacters(in: .whitespacesAndNewlines).isEmpty {
            return codexActivityMessage
        }
        if let current = progress.currentStep {
            if current.state == .running, current.usesCodex {
                return "Using Codex for activity \(current.title)..."
            }
            if let summary = current.summary, !summary.isEmpty {
                return summary
            }
            return "Now: \(current.title)"
        }
        if let lastSummary = progress.lastSummary, !lastSummary.isEmpty {
            return lastSummary
        }
        return "Runtime loaded"
    }
}
