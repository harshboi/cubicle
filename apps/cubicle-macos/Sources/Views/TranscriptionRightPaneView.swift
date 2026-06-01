import SwiftUI

struct TranscriptionOverviewView: View {
    @EnvironmentObject private var model: AppModel

    var body: some View {
        TranscriptionOverviewContent(
            viewModel: model.transcriptionViewModel,
            settings: model.systemSettings
        )
    }
}

private struct TranscriptionOverviewContent: View {
    @ObservedObject var viewModel: TranscriptionViewModel
    let settings: SystemSettings

    var body: some View {
        ScrollView {
            VStack(alignment: .leading, spacing: 16) {
                SectionHeader(
                    title: "Transcription",
                    subtitle: "Live transcript runtime and source controls."
                )

                TranscriptionStatusPanel(viewModel: viewModel, settings: settings)

                SettingsGroupLike(title: "Session") {
                    TranscriptionInfoRow(
                        symbolName: "captions.bubble",
                        title: "Mode",
                        value: settings.transcriptionLanguageMode.displayName
                    )
                    TranscriptionInfoRow(
                        symbolName: "person.2.wave.2",
                        title: "Diarization",
                        value: settings.transcriptionDiarizationEnabled ? "enabled" : "disabled"
                    )
                    TranscriptionInfoRow(
                        symbolName: "mic.fill",
                        title: "Mic gain",
                        value: "\(settings.transcriptionMicrophoneGain)x"
                    )
                    TranscriptionInfoRow(
                        symbolName: "network",
                        title: "Endpoint",
                        value: settings.transcriptionAWSEndpoint.isEmpty ? "not set" : settings.transcriptionAWSEndpoint
                    )
                }

                HStack(spacing: 10) {
                    Button {
                        Task { await viewModel.startSessionForCurrentSettings() }
                    } label: {
                        Label("Start Session", systemImage: "play.fill")
                    }
                    .buttonStyle(.borderedProminent)
                    .disabled(!canStart)
                    .help("Start the configured transcription session")

                    Button {
                        Task { await viewModel.stopSession() }
                    } label: {
                        Label("Stop", systemImage: "stop.fill")
                    }
                    .buttonStyle(.bordered)
                    .disabled(viewModel.status == .disabled || viewModel.status == .stopped)
                    .help("Stop capture and close the transcription session")
                }
            }
            .padding(24)
            .frame(maxWidth: .infinity, alignment: .topLeading)
        }
        .navigationTitle("Transcription")
    }

    private var canStart: Bool {
        settings.transcriptionEnabled
            && !settings.transcriptionAWSEndpoint.trimmingCharacters(in: .whitespacesAndNewlines).isEmpty
            && viewModel.status != .live
            && viewModel.status != .connecting
            && !viewModel.isStoppingSession
    }
}

struct TranscriptionRightPaneView: View {
    @EnvironmentObject private var model: AppModel

    var body: some View {
        TranscriptionTranscriptContent(
            model: model,
            viewModel: model.transcriptionViewModel,
            settings: model.systemSettings
        )
    }
}

private struct TranscriptionTranscriptContent: View {
    @ObservedObject var model: AppModel
    @ObservedObject var viewModel: TranscriptionViewModel
    let settings: SystemSettings

    var body: some View {
        ScrollView {
            VStack(alignment: .leading, spacing: 16) {
                SectionHeader(
                    title: "Live Transcript",
                    subtitle: "\(settings.transcriptionLanguageMode.displayName) - \(viewModel.sessionStateText)"
                )

                TranscriptionStatusPanel(viewModel: viewModel, settings: settings)

                TranscriptionTimelineSubmissionPanel(model: model, viewModel: viewModel)

                if !settings.transcriptionEnabled {
                    TranscriptionEmptyState(
                        symbolName: "mic.slash",
                        title: "Live transcription is disabled",
                        message: "Enable Live Transcription in Settings to allow capture and streaming."
                    )
                } else if viewModel.visibleSegments.isEmpty {
                    TranscriptionEmptyState(
                        symbolName: "text.bubble",
                        title: "No transcript yet",
                        message: "No audio source is active. A transcription session can be started from the Transcription pane after an endpoint is set."
                    )
                } else {
                    VStack(alignment: .leading, spacing: 10) {
                        ForEach(viewModel.visibleSegments) { segment in
                            TranscriptSegmentRow(
                                segment: segment,
                                diarizationEnabled: settings.transcriptionDiarizationEnabled
                            )
                        }
                    }
                }
            }
            .padding(24)
            .frame(maxWidth: .infinity, alignment: .topLeading)
        }
    }
}

private struct TranscriptionTimelineSubmissionPanel: View {
    @ObservedObject var model: AppModel
    @ObservedObject var viewModel: TranscriptionViewModel

    private var kind: TranscriptionTimelineTargetKind {
        model.transcriptionTimelineTargetKind
    }

    private var targets: [ConfigTarget] {
        model.transcriptionTimelineTargets(for: kind)
    }

    var body: some View {
        VStack(alignment: .leading, spacing: 10) {
            HStack(spacing: 10) {
                Picker("Timeline type", selection: Binding(
                    get: { model.transcriptionTimelineTargetKind },
                    set: { model.transcriptionTimelineTargetKind = $0 }
                )) {
                    ForEach(TranscriptionTimelineTargetKind.allCases) { kind in
                        Text(kind.title).tag(kind)
                    }
                }
                .pickerStyle(.segmented)
                .frame(width: 180)
                .labelsHidden()

                Picker("Timeline target", selection: Binding(
                    get: { model.selectedTranscriptionTimelineTargetID(for: kind) ?? "" },
                    set: { model.setSelectedTranscriptionTimelineTargetID($0, for: kind) }
                )) {
                    if targets.isEmpty {
                        Text("No tracked \(kind.title.lowercased())s").tag("")
                    } else {
                        ForEach(targets) { target in
                            Text(targetTitle(target)).tag(target.id)
                        }
                    }
                }
                .pickerStyle(.menu)
                .frame(maxWidth: .infinity, alignment: .leading)
                .disabled(targets.isEmpty)

                Button {
                    model.submitCurrentTranscriptToTimeline()
                } label: {
                    Label("Submit", systemImage: "tray.and.arrow.down.fill")
                }
                .buttonStyle(.borderedProminent)
                .disabled(!canSubmit)
                .help("Submit the current transcript snapshot to the selected local timeline")
            }

            if let message = model.transcriptionTimelineSubmissionMessage {
                Label(message, systemImage: "checkmark.circle.fill")
                    .font(.caption)
                    .foregroundStyle(.green)
            } else if let error = model.transcriptionTimelineSubmissionError {
                Label(error, systemImage: "exclamationmark.triangle.fill")
                    .font(.caption)
                    .foregroundStyle(.red)
            }
        }
        .padding(14)
        .background(.regularMaterial, in: RoundedRectangle(cornerRadius: 10, style: .continuous))
    }

    private var canSubmit: Bool {
        !targets.isEmpty
            && viewModel.hasTranscriptForSubmission
            && !model.transcriptionTimelineSubmissionRunning
    }

    private func targetTitle(_ target: ConfigTarget) -> String {
        let label = target.label.trimmingCharacters(in: .whitespacesAndNewlines)
        if !label.isEmpty {
            return label
        }
        let email = target.email.trimmingCharacters(in: .whitespacesAndNewlines)
        if !email.isEmpty {
            return email
        }
        let roomID = target.roomID.trimmingCharacters(in: .whitespacesAndNewlines)
        if !roomID.isEmpty {
            return roomID
        }
        return target.id
    }
}

private struct TranscriptionStatusPanel: View {
    @ObservedObject var viewModel: TranscriptionViewModel
    let settings: SystemSettings

    var body: some View {
        HStack(alignment: .top, spacing: 12) {
            Image(systemName: viewModel.status.symbolName)
                .font(.title3.weight(.semibold))
                .foregroundStyle(statusColor)
                .frame(width: 28)
            VStack(alignment: .leading, spacing: 4) {
                Text(viewModel.sessionStateText)
                    .font(.headline)
                Text(viewModel.sessionDetailText)
                    .font(.callout)
                    .foregroundStyle(.secondary)
                    .textSelection(.enabled)
                if let currentConfig = viewModel.currentConfig {
                    Text("Session \(currentConfig.sessionID)")
                        .font(.caption.monospaced())
                        .foregroundStyle(.secondary)
                        .lineLimit(1)
                        .truncationMode(.middle)
                        .textSelection(.enabled)
                }
                if let audioStatusText = viewModel.audioStatusText {
                    Text(audioStatusText)
                        .font(.caption.monospacedDigit())
                        .foregroundStyle(viewModel.audioChunksSent == 0 ? .orange : .secondary)
                        .textSelection(.enabled)
                }
            }
            Spacer()
            VStack(alignment: .trailing, spacing: 4) {
                Text(settings.transcriptionEnabled ? "enabled" : "disabled")
                    .font(.caption.weight(.semibold))
                    .foregroundStyle(settings.transcriptionEnabled ? .green : .secondary)
                Text(settings.transcriptionDiarizationEnabled ? "diarization on" : "diarization off")
                    .font(.caption)
                    .foregroundStyle(.secondary)
                Text("mic \(settings.transcriptionMicrophoneGain)x")
                    .font(.caption.monospacedDigit())
                    .foregroundStyle(.secondary)
                if settings.transcriptionDiarizationEnabled,
                   let diarizationStatus = viewModel.lastDiarizationStatus,
                   !diarizationStatus.isEmpty {
                    Text(diarizationStatus)
                        .font(.caption2)
                        .foregroundStyle(diarizationStatus.contains("failed") || diarizationStatus.contains("timed out") ? .orange : .secondary)
                        .multilineTextAlignment(.trailing)
                }
            }
        }
        .padding(14)
        .background(.regularMaterial, in: RoundedRectangle(cornerRadius: 10, style: .continuous))
    }

    private var statusColor: Color {
        switch viewModel.status {
        case .disabled, .stopped:
            return .secondary
        case .connecting, .reconnecting:
            return .orange
        case .live:
            return .green
        case .failed:
            return .red
        }
    }
}

private struct TranscriptSegmentRow: View {
    let segment: TranscriptSegment
    let diarizationEnabled: Bool

    var body: some View {
        VStack(alignment: .leading, spacing: 8) {
            HStack(spacing: 8) {
                if diarizationEnabled, let speakerLabel = segment.speakerLabel {
                    Text(speakerLabel)
                        .font(.caption.weight(.semibold))
                        .foregroundStyle(.blue)
                }
                Spacer()
                Text(timeRange)
                    .font(.caption.monospacedDigit())
                    .foregroundStyle(.secondary)
            }
            Text(segment.text)
                .font(.body)
                .foregroundStyle(segment.isFinal ? .primary : .secondary)
                .textSelection(.enabled)
        }
        .padding(14)
        .background(.regularMaterial, in: RoundedRectangle(cornerRadius: 10, style: .continuous))
    }

    private var timeRange: String {
        let start = String(format: "%.1fs", Double(segment.startTimeMilliseconds) / 1000.0)
        guard let end = segment.endTimeMilliseconds else {
            return start
        }
        return "\(start)-\(String(format: "%.1fs", Double(end) / 1000.0))"
    }
}

private struct TranscriptionInfoRow: View {
    let symbolName: String
    let title: String
    let value: String

    var body: some View {
        HStack(spacing: 10) {
            Image(systemName: symbolName)
                .foregroundStyle(.tint)
                .frame(width: 24)
            Text(title)
                .font(.body.weight(.semibold))
            Spacer()
            Text(value)
                .font(.system(.body, design: .monospaced))
                .foregroundStyle(.secondary)
                .lineLimit(1)
                .truncationMode(.middle)
                .textSelection(.enabled)
        }
        .frame(minHeight: 42)
        .overlay(alignment: .bottom) {
            Divider()
                .padding(.leading, 34)
        }
    }
}

private struct SettingsGroupLike<Content: View>: View {
    let title: String
    @ViewBuilder var content: Content

    var body: some View {
        VStack(alignment: .leading, spacing: 12) {
            Text(title)
                .font(.headline)
            VStack(spacing: 0) {
                content
            }
        }
        .padding(16)
        .background(.regularMaterial, in: RoundedRectangle(cornerRadius: 10, style: .continuous))
    }
}

private struct TranscriptionEmptyState: View {
    let symbolName: String
    let title: String
    let message: String

    var body: some View {
        VStack(spacing: 12) {
            Image(systemName: symbolName)
                .font(.system(size: 34, weight: .semibold))
                .foregroundStyle(.secondary)
            Text(title)
                .font(.headline)
            Text(message)
                .font(.callout)
                .foregroundStyle(.secondary)
                .multilineTextAlignment(.center)
                .frame(maxWidth: 420)
        }
        .frame(maxWidth: .infinity, minHeight: 220)
        .padding(20)
        .background(.regularMaterial, in: RoundedRectangle(cornerRadius: 10, style: .continuous))
    }
}
