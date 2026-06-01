import SwiftUI

struct SettingsView: View {
    @EnvironmentObject private var model: AppModel
    @State private var pendingConfirmation: SystemSettingsAction?
    @State private var pendingOAuthRevocation: OAuthProviderKind?
    @State private var transcriptionAuthTokenDraft = ""

    var body: some View {
        ScrollView {
            VStack(alignment: .leading, spacing: 16) {
                SectionHeader(
                    title: "System Settings",
                    subtitle: "Native controls for Webex sync, focus rebuilds, background cadence, and runtime settings."
                )

                if let message = model.settingsLastMessage {
                    SettingsBanner(text: message, symbolName: "checkmark.circle", color: .green)
                }
                if let error = model.settingsLastError {
                    SettingsBanner(text: error, symbolName: "exclamationmark.triangle", color: .red)
                }

                SettingsGroup(title: "Actions") {
                    SettingsActionRow(
                        title: "Sync Webex",
                        subtitle: "Refresh map.txt and native Webex snapshots",
                        symbolName: "arrow.triangle.2.circlepath",
                        actionSymbolName: "arrow.clockwise",
                        isRunning: model.isSystemSettingsActionRunning(.syncWebex)
                    ) {
                        Task { await model.runSystemSettingsAction(.syncWebex) }
                    }
                    SettingsActionRow(
                        title: "Rebuild Person Focus Clusters",
                        subtitle: "Full recluster plus Codex summaries and titles",
                        symbolName: "person.2.crop.square.stack",
                        actionSymbolName: "arrow.triangle.2.circlepath",
                        isRunning: model.isSystemSettingsActionRunning(.rebuildPersonFocusAll),
                        role: .destructive
                    ) {
                        pendingConfirmation = .rebuildPersonFocusAll
                    }
                    SettingsActionRow(
                        title: "Rebuild Space Focus Clusters",
                        subtitle: "Full space recluster plus Codex summaries and titles",
                        symbolName: "bubble.left.and.bubble.right",
                        actionSymbolName: "arrow.triangle.2.circlepath",
                        isRunning: model.isSystemSettingsActionRunning(.rebuildSpaceFocusAll),
                        role: .destructive
                    ) {
                        pendingConfirmation = .rebuildSpaceFocusAll
                    }
                }

                SettingsGroup(title: "Connected Accounts") {
                    OAuthSettingsRow(
                        provider: .webex,
                        status: model.oauthStatus(for: .webex),
                        isRunning: model.isOAuthActionRunning(.webex)
                    ) {
                        Task { await model.connectOAuth(provider: .webex) }
                    } revokeAction: {
                        pendingOAuthRevocation = .webex
                    }
                    OAuthSettingsRow(
                        provider: .outlook,
                        status: model.oauthStatus(for: .outlook),
                        isRunning: model.isOAuthActionRunning(.outlook)
                    ) {
                        Task { await model.connectOAuth(provider: .outlook) }
                    } revokeAction: {
                        pendingOAuthRevocation = .outlook
                    }
                }

                SettingsGroup(title: "Live Transcription") {
                    SettingsToggleRow(
                        title: "Enable Live Transcription",
                        subtitle: "When off, Cubicle does not capture audio or open transcription sessions",
                        symbolName: "mic",
                        isOn: Binding(
                            get: { model.systemSettings.transcriptionEnabled },
                            set: { model.updateSystemSetting(.transcriptionEnabled, boolValue: $0) }
                        )
                    )
                    SettingsToggleRow(
                        title: "Enable Speaker Diarization",
                        subtitle: "Included as a session option for AWS transcription",
                        symbolName: "person.2.wave.2",
                        isOn: Binding(
                            get: { model.systemSettings.transcriptionDiarizationEnabled },
                            set: { model.updateSystemSetting(.transcriptionDiarizationEnabled, boolValue: $0) }
                        )
                    )
                    .disabled(!model.systemSettings.transcriptionEnabled)
                    .opacity(model.systemSettings.transcriptionEnabled ? 1 : 0.55)
                    SettingsPickerRow(
                        title: "Audio Language / Mode",
                        subtitle: "Sent in the transcription session start payload",
                        symbolName: "captions.bubble",
                        selection: Binding(
                            get: { model.systemSettings.transcriptionLanguageMode },
                            set: { model.updateSystemSetting(.transcriptionLanguageMode, stringValue: $0.rawValue) }
                        ),
                        options: TranscriptionLanguageMode.allCases
                    ) { selection in
                        selection.displayName
                    }
                    .disabled(!model.systemSettings.transcriptionEnabled)
                    .opacity(model.systemSettings.transcriptionEnabled ? 1 : 0.55)
                    SettingsTextFieldRow(
                        title: "AWS endpoint",
                        subtitle: "WebSocket endpoint for staging or production",
                        symbolName: "network",
                        text: Binding(
                            get: { model.systemSettings.transcriptionAWSEndpoint },
                            set: { model.updateSystemSetting(.transcriptionAWSEndpoint, stringValue: $0) }
                        ),
                        placeholder: "wss://transcription.example.com/session"
                    )
                    TranscriptionAuthTokenRow(
                        token: $transcriptionAuthTokenDraft,
                        configured: model.transcriptionAuthTokenConfigured,
                        saveAction: {
                            model.saveTranscriptionAuthToken(transcriptionAuthTokenDraft)
                            transcriptionAuthTokenDraft = ""
                        },
                        clearAction: {
                            model.deleteTranscriptionAuthToken()
                            transcriptionAuthTokenDraft = ""
                        }
                    )
                    TranscriptionSettingsStatusRow(viewModel: model.transcriptionViewModel)
                }

                SettingsGroup(title: "Codex") {
                    SettingsToggleRow(
                        title: "Enable Codex",
                        subtitle: "Master switch for all Codex-backed workflows",
                        symbolName: "power",
                        isOn: Binding(
                            get: { model.systemSettings.codexEnabled },
                            set: { model.updateSystemSetting(.codexEnabled, boolValue: $0) }
                        )
                    )
                    SettingsPickerRow(
                        title: "GPT model",
                        subtitle: "Used for Ask Codex, summaries, questions, and beliefs",
                        symbolName: "sparkles",
                        selection: Binding(
                            get: { model.systemSettings.codexModel },
                            set: { model.updateSystemSetting(.codexModel, stringValue: $0.rawValue) }
                        ),
                        options: CodexModelSelection.allCases
                    ) { selection in
                        selection.displayName
                    }
                    SettingsPickerRow(
                        title: "Reasoning level",
                        subtitle: "Higher levels trade speed for deeper analysis",
                        symbolName: "brain.head.profile",
                        selection: Binding(
                            get: { model.systemSettings.codexReasoningLevel },
                            set: { model.updateSystemSetting(.codexReasoningLevel, stringValue: $0.rawValue) }
                        ),
                        options: CodexReasoningLevel.allCases
                    ) { selection in
                        selection.displayName
                    }
                }

                SettingsGroup(title: "Codex Feature Controls") {
                    ForEach(CodexFeatureToggle.allCases) { feature in
                        SettingsToggleRow(
                            title: feature.displayName,
                            subtitle: feature.settingsSubtitle,
                            symbolName: feature.symbolName,
                            isOn: Binding(
                                get: { model.systemSettings.boolValue(for: feature.settingKey) },
                                set: { model.updateSystemSetting(feature.settingKey, boolValue: $0) }
                            )
                        )
                        .disabled(!model.systemSettings.codexEnabled)
                        .opacity(model.systemSettings.codexEnabled ? 1 : 0.55)
                    }
                }

                SettingsGroup(title: "Process Flags") {
                    SettingsToggleRow(
                        title: "Debug output",
                        subtitle: "Saved to runtime settings",
                        symbolName: "ladybug",
                        isOn: Binding(
                            get: { model.systemSettings.debug },
                            set: { model.updateSystemSetting(.debug, boolValue: $0) }
                        )
                    )
                    SettingsToggleRow(
                        title: "Background summaries",
                        subtitle: "Saved to runtime settings",
                        symbolName: "text.bubble",
                        isOn: Binding(
                            get: { model.systemSettings.backgroundStatus },
                            set: { model.updateSystemSetting(.backgroundStatus, boolValue: $0) }
                        )
                    )
                    SettingsToggleRow(
                        title: "Webex sync",
                        subtitle: "Allow startup, background, and manual Webex refreshes",
                        symbolName: "arrow.triangle.2.circlepath",
                        isOn: Binding(
                            get: { model.systemSettings.webexSyncEnabled },
                            set: { model.updateSystemSetting(.webexSyncEnabled, boolValue: $0) }
                        )
                    )
                    SettingsToggleRow(
                        title: "Auto-query-all",
                        subtitle: "Interval \(model.systemSettings.autoQueryAllMinutes) min",
                        symbolName: "timer",
                        isOn: Binding(
                            get: { model.systemSettings.autoQueryAllEnabled },
                            set: { model.updateSystemSetting(.autoQueryAllEnabled, boolValue: $0) }
                        )
                    )
                    SettingsToggleRow(
                        title: "Page reload pauses background work",
                        subtitle: "Turn off to leave active background refreshes running",
                        symbolName: "pause.circle",
                        isOn: Binding(
                            get: { model.systemSettings.priorityRefreshPausesBackground },
                            set: { model.updateSystemSetting(.priorityRefreshPausesBackground, boolValue: $0) }
                        )
                    )
                }

                SettingsGroup(title: "Cadence") {
                    SettingsStepperRow(
                        title: "Webex sync interval",
                        subtitle: "minutes",
                        symbolName: "arrow.triangle.2.circlepath",
                        value: Binding(
                            get: { model.systemSettings.webexSyncMinutes },
                            set: { model.updateSystemSetting(.webexSyncMinutes, intValue: $0) }
                        ),
                        range: 1...1440,
                        unit: "min"
                    )
                    SettingsStepperRow(
                        title: "Auto-query-all interval",
                        subtitle: "minutes",
                        symbolName: "timer",
                        value: Binding(
                            get: { model.systemSettings.autoQueryAllMinutes },
                            set: { model.updateSystemSetting(.autoQueryAllMinutes, intValue: $0) }
                        ),
                        range: 1...1440,
                        unit: "min"
                    )
                    SettingsStepperRow(
                        title: "Tracked actions refresh",
                        subtitle: "minutes",
                        symbolName: "checklist",
                        value: Binding(
                            get: { model.systemSettings.trackedActionsRefreshMinutes },
                            set: { model.updateSystemSetting(.trackedActionsRefreshMinutes, intValue: $0) }
                        ),
                        range: 1...1440,
                        unit: "min"
                    )
                    SettingsStepperRow(
                        title: "Person Focus refresh",
                        subtitle: "minutes",
                        symbolName: "person.crop.circle.badge.clock",
                        value: Binding(
                            get: { model.systemSettings.personFocusRefreshMinutes },
                            set: { model.updateSystemSetting(.personFocusRefreshMinutes, intValue: $0) }
                        ),
                        range: 1...1440,
                        unit: "min"
                    )
                    SettingsStepperRow(
                        title: "Space Focus refresh",
                        subtitle: "minutes",
                        symbolName: "bubble.left.and.bubble.right.fill",
                        value: Binding(
                            get: { model.systemSettings.spaceFocusRefreshMinutes },
                            set: { model.updateSystemSetting(.spaceFocusRefreshMinutes, intValue: $0) }
                        ),
                        range: 1...1440,
                        unit: "min"
                    )
                }

                SettingsGroup(title: "Focus Windows") {
                    SettingsStepperRow(
                        title: "Person Focus days",
                        subtitle: "lookback window",
                        symbolName: "calendar.badge.clock",
                        value: Binding(
                            get: { model.focusAnalysisDraft.personFocusDays },
                            set: { model.updateFocusAnalysisDraft(.personFocusDays, intValue: $0) }
                        ),
                        range: SystemSettings.focusDaysBounds,
                        unit: "days"
                    )
                    SettingsStepperRow(
                        title: "Person analysis cadence",
                        subtitle: "Codex bucket",
                        symbolName: "clock.badge.checkmark",
                        value: Binding(
                            get: { model.focusAnalysisDraft.personFocusAnalysisCadenceHours },
                            set: { model.updateFocusAnalysisDraft(.personFocusAnalysisCadenceHours, intValue: $0) }
                        ),
                        range: SystemSettings.focusAnalysisCadenceHoursBounds,
                        unit: "hrs"
                    )
                    SettingsReadOnlyRow(
                        title: "Person analysis status",
                        value: model.focusAnalysisStatusText(kind: .person),
                        symbolName: model.focusAnalysisCacheStatusByKind[.person]?.canUseExactCache == true ? "checkmark.seal" : "exclamationmark.arrow.triangle.2.circlepath"
                    )
                    SettingsActionRow(
                        title: "Apply / Refresh Person Focus",
                        subtitle: model.focusAnalysisDraftHasChanges(kind: .person) ? "Commit draft values and refresh analysis" : "Refresh with saved values",
                        symbolName: "person.crop.circle.badge.clock",
                        actionSymbolName: "arrow.clockwise",
                        isRunning: model.isFocusAnalysisRefreshRunning(kind: .person)
                    ) {
                        Task { await model.applyFocusAnalysisDraftAndRefresh(kind: .person) }
                    }
                    SettingsStepperRow(
                        title: "Space Focus days",
                        subtitle: "lookback window",
                        symbolName: "calendar",
                        value: Binding(
                            get: { model.focusAnalysisDraft.spaceFocusDays },
                            set: { model.updateFocusAnalysisDraft(.spaceFocusDays, intValue: $0) }
                        ),
                        range: SystemSettings.focusDaysBounds,
                        unit: "days"
                    )
                    SettingsStepperRow(
                        title: "Space analysis cadence",
                        subtitle: "Codex bucket",
                        symbolName: "clock.badge.checkmark",
                        value: Binding(
                            get: { model.focusAnalysisDraft.spaceFocusAnalysisCadenceHours },
                            set: { model.updateFocusAnalysisDraft(.spaceFocusAnalysisCadenceHours, intValue: $0) }
                        ),
                        range: SystemSettings.focusAnalysisCadenceHoursBounds,
                        unit: "hrs"
                    )
                    SettingsReadOnlyRow(
                        title: "Space analysis status",
                        value: model.focusAnalysisStatusText(kind: .space),
                        symbolName: model.focusAnalysisCacheStatusByKind[.space]?.canUseExactCache == true ? "checkmark.seal" : "exclamationmark.arrow.triangle.2.circlepath"
                    )
                    SettingsActionRow(
                        title: "Apply / Refresh Space Focus",
                        subtitle: model.focusAnalysisDraftHasChanges(kind: .space) ? "Commit draft values and refresh analysis" : "Refresh with saved values",
                        symbolName: "bubble.left.and.bubble.right.fill",
                        actionSymbolName: "arrow.clockwise",
                        isRunning: model.isFocusAnalysisRefreshRunning(kind: .space)
                    ) {
                        Task { await model.applyFocusAnalysisDraftAndRefresh(kind: .space) }
                    }
                    if model.focusAnalysisDraftHasChanges() {
                        SettingsActionRow(
                            title: "Reset Draft",
                            subtitle: "Restore saved focus analysis settings",
                            symbolName: "arrow.uturn.backward",
                            actionSymbolName: "arrow.uturn.backward",
                            isRunning: false
                        ) {
                            model.resetFocusAnalysisDraft()
                        }
                    }
                }

                SettingsGroup(title: "Runtime") {
                    SettingsReadOnlyRow(title: "Storage root", value: model.runtimeStatus.runtimeRoot.path, symbolName: "externaldrive")
                    SettingsReadOnlyRow(title: "Watch sources", value: model.configStore.watchSourcesDescription, symbolName: "eye")
                    SettingsReadOnlyRow(title: "Settings file", value: model.configStore.systemSettingsURL.path, symbolName: "doc.text")
                }
            }
            .padding(24)
            .frame(maxWidth: .infinity, alignment: .leading)
        }
        .navigationTitle("Settings")
        .onAppear {
            model.reloadSystemSettings()
            model.reloadOAuthStatuses()
        }
        .confirmationDialog(
            confirmationTitle,
            isPresented: Binding(
                get: { pendingConfirmation != nil },
                set: { if !$0 { pendingConfirmation = nil } }
            ),
            titleVisibility: .visible
        ) {
            if let action = pendingConfirmation {
                Button(confirmationButtonTitle(action), role: .destructive) {
                    let selectedAction = action
                    pendingConfirmation = nil
                    Task { await model.runSystemSettingsAction(selectedAction) }
                }
            }
            Button("Cancel", role: .cancel) {
                pendingConfirmation = nil
            }
        } message: {
            Text("This reruns clustering and Codex enrichment for all configured targets.")
        }
        .confirmationDialog(
            oauthRevocationTitle,
            isPresented: Binding(
                get: { pendingOAuthRevocation != nil },
                set: { if !$0 { pendingOAuthRevocation = nil } }
            ),
            titleVisibility: .visible
        ) {
            if let provider = pendingOAuthRevocation {
                Button("Revoke \(provider.displayName)", role: .destructive) {
                    let selectedProvider = provider
                    pendingOAuthRevocation = nil
                    model.revokeOAuth(provider: selectedProvider)
                }
            }
            Button("Cancel", role: .cancel) {
                pendingOAuthRevocation = nil
            }
        } message: {
            Text("This removes the local token file. You can reconnect with OAuth afterward.")
        }
    }

    private var confirmationTitle: String {
        guard let pendingConfirmation else {
            return "Run rebuild?"
        }
        switch pendingConfirmation {
        case .rebuildPersonFocusAll:
            return "Rebuild all Person Focus clusters?"
        case .rebuildSpaceFocusAll:
            return "Rebuild all Space Focus clusters?"
        case .syncWebex:
            return "Sync Webex?"
        }
    }

    private func confirmationButtonTitle(_ action: SystemSettingsAction) -> String {
        switch action {
        case .rebuildPersonFocusAll:
            return "Rebuild Person Focus"
        case .rebuildSpaceFocusAll:
            return "Rebuild Space Focus"
        case .syncWebex:
            return "Sync Webex"
        }
    }

    private var oauthRevocationTitle: String {
        guard let provider = pendingOAuthRevocation else {
            return "Revoke OAuth token?"
        }
        return "Revoke \(provider.displayName) OAuth token?"
    }
}

struct RuntimeInspectorView: View {
    @EnvironmentObject private var model: AppModel

    var body: some View {
        ScrollView {
            VStack(alignment: .leading, spacing: 16) {
                SectionHeader(
                    title: "Runtime Inspector",
                    subtitle: "Current native runtime state and settings paths."
                )
                SettingsMetricGrid(
                    metrics: [
                        SettingsMetric(
                            title: "Knowledge",
                            value: runtimeMetricValue(model.runtimeStatus.knowledgeDirectoryExists),
                            symbolName: "cylinder.split.1x2",
                            color: runtimeMetricColor(model.runtimeStatus.knowledgeDirectoryExists)
                        ),
                        SettingsMetric(
                            title: "Space Cache",
                            value: runtimeMetricValue(model.runtimeStatus.spaceSnapshotExists),
                            symbolName: "bubble.left.and.bubble.right",
                            color: runtimeMetricColor(model.runtimeStatus.spaceSnapshotExists)
                        ),
                        SettingsMetric(
                            title: "Person Cache",
                            value: runtimeMetricValue(model.runtimeStatus.personSnapshotExists),
                            symbolName: "person.2",
                            color: runtimeMetricColor(model.runtimeStatus.personSnapshotExists)
                        )
                    ]
                )
                SettingsGroup(title: "Paths") {
                    SettingsReadOnlyRow(title: "Root", value: model.runtimeStatus.runtimeRoot.path, symbolName: "folder")
                    SettingsReadOnlyRow(title: "Map file", value: model.configStore.mapFileURL.path, symbolName: "map")
                    SettingsReadOnlyRow(title: "Codex executable", value: model.runtimeStatus.codexExecutable, symbolName: "sparkles")
                }
                SettingsGroup(title: "Saved Values") {
                    SettingsReadOnlyRow(title: "Person Focus", value: "\(model.systemSettings.personFocusDays) days, analysis every \(model.systemSettings.personFocusAnalysisCadenceHours)h, local every \(model.systemSettings.personFocusRefreshMinutes) min", symbolName: "person.crop.circle.badge.clock")
                    SettingsReadOnlyRow(title: "Space Focus", value: "\(model.systemSettings.spaceFocusDays) days, analysis every \(model.systemSettings.spaceFocusAnalysisCadenceHours)h, local every \(model.systemSettings.spaceFocusRefreshMinutes) min", symbolName: "bubble.left.and.bubble.right.fill")
                    SettingsReadOnlyRow(title: "Webex sync", value: model.systemSettings.webexSyncEnabled ? "enabled, every \(model.systemSettings.webexSyncMinutes) min" : "disabled, every \(model.systemSettings.webexSyncMinutes) min", symbolName: "arrow.triangle.2.circlepath")
                    SettingsReadOnlyRow(title: "Transcription", value: transcriptionRuntimeValue, symbolName: "waveform")
                    SettingsReadOnlyRow(title: "Codex", value: "\(model.systemSettings.codexModel.displayName), \(model.systemSettings.codexReasoningLevel.displayName), \(model.systemSettings.codexEnabled ? "enabled" : "disabled")", symbolName: "sparkles")
                    SettingsReadOnlyRow(title: "Page reload", value: model.systemSettings.priorityRefreshPausesBackground ? "pauses lower-priority work" : "does not pause background work", symbolName: "pause.circle")
                    SettingsReadOnlyRow(title: "Updated", value: settingsUpdatedText, symbolName: "clock")
                }
            }
            .padding(24)
            .frame(maxWidth: .infinity, alignment: .topLeading)
        }
    }

    private var settingsUpdatedText: String {
        guard let updatedAt = model.systemSettings.updatedAt else {
            return "not saved yet"
        }
        return SettingsDateFormatter.string(from: updatedAt)
    }

    private var transcriptionRuntimeValue: String {
        guard model.systemSettings.transcriptionEnabled else {
            return "disabled, mic \(model.systemSettings.transcriptionMicrophoneGain)x"
        }
        return "\(model.systemSettings.transcriptionLanguageMode.displayName), diarization \(model.systemSettings.transcriptionDiarizationEnabled ? "on" : "off"), mic \(model.systemSettings.transcriptionMicrophoneGain)x"
    }

    private func runtimeMetricValue(_ available: Bool) -> String {
        model.isLoading ? "checking" : (available ? "available" : "missing")
    }

    private func runtimeMetricColor(_ available: Bool) -> Color {
        model.isLoading ? .secondary : (available ? .green : .red)
    }
}

private struct SettingsGroup<Content: View>: View {
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

private struct SettingsActionRow: View {
    let title: String
    let subtitle: String
    let symbolName: String
    let actionSymbolName: String
    let isRunning: Bool
    var role: ButtonRole?
    var action: () -> Void

    var body: some View {
        SettingsBaseRow(symbolName: symbolName, title: title, subtitle: subtitle) {
            Button(role: role, action: action) {
                if isRunning {
                    ProgressView()
                        .controlSize(.small)
                } else {
                    Image(systemName: actionSymbolName)
                }
            }
            .buttonStyle(.bordered)
            .disabled(isRunning)
            .help(title)
        }
    }
}

private struct OAuthSettingsRow: View {
    let provider: OAuthProviderKind
    let status: OAuthProviderStatus
    let isRunning: Bool
    var connectAction: () -> Void
    var revokeAction: () -> Void

    var body: some View {
        SettingsBaseRow(
            symbolName: provider.settingsSymbolName,
            title: "\(provider.displayName) OAuth",
            subtitle: status.settingsSubtitle
        ) {
            HStack(spacing: 8) {
                Text(status.settingsStatusText)
                    .font(.caption.weight(.semibold))
                    .foregroundStyle(status.settingsStatusColor)
                    .lineLimit(1)
                    .frame(minWidth: 96, alignment: .trailing)
                Button(action: connectAction) {
                    if isRunning {
                        ProgressView()
                            .controlSize(.small)
                    } else {
                        Label(status.tokenFileExists ? "Reconnect" : "Connect", systemImage: "person.crop.circle.badge.plus")
                    }
                }
                .buttonStyle(.bordered)
                .disabled(isRunning)
                .help("Connect \(provider.displayName) with OAuth")

                Button(role: .destructive, action: revokeAction) {
                    Label("Revoke", systemImage: "xmark.circle")
                }
                .buttonStyle(.bordered)
                .disabled(isRunning || !status.tokenFileExists)
                .help("Remove the local \(provider.displayName) OAuth token")
            }
        }
    }
}

private struct SettingsToggleRow: View {
    let title: String
    let subtitle: String
    let symbolName: String
    @Binding var isOn: Bool

    var body: some View {
        SettingsBaseRow(symbolName: symbolName, title: title, subtitle: subtitle) {
            Toggle("", isOn: $isOn)
                .labelsHidden()
        }
    }
}

private struct SettingsPickerRow<Option: Hashable>: View {
    let title: String
    let subtitle: String
    let symbolName: String
    @Binding var selection: Option
    let options: [Option]
    let label: (Option) -> String

    var body: some View {
        SettingsBaseRow(symbolName: symbolName, title: title, subtitle: subtitle) {
            Picker("", selection: $selection) {
                ForEach(options, id: \.self) { option in
                    Text(label(option)).tag(option)
                }
            }
            .labelsHidden()
            .pickerStyle(.menu)
            .frame(width: 220, alignment: .trailing)
        }
    }
}

private struct SettingsTextFieldRow: View {
    let title: String
    let subtitle: String
    let symbolName: String
    @Binding var text: String
    let placeholder: String

    var body: some View {
        SettingsBaseRow(symbolName: symbolName, title: title, subtitle: subtitle) {
            TextField(placeholder, text: $text)
                .textFieldStyle(.roundedBorder)
                .font(.system(.body, design: .monospaced))
                .frame(width: 320)
        }
    }
}

private struct TranscriptionAuthTokenRow: View {
    @Binding var token: String
    let configured: Bool
    let saveAction: () -> Void
    let clearAction: () -> Void
    @State private var tokenVisible = false

    private var trimmedToken: String {
        token.trimmingCharacters(in: .whitespacesAndNewlines)
    }

    var body: some View {
        VStack(alignment: .leading, spacing: 10) {
            HStack(alignment: .top, spacing: 12) {
                Image(systemName: configured ? "key.fill" : "key")
                    .foregroundStyle(.tint)
                    .frame(width: 24)
                VStack(alignment: .leading, spacing: 3) {
                    Text("Service token")
                        .font(.body.weight(.semibold))
                    Text(configured ? "Stored in Keychain and sent as a Bearer header" : "Stored in Keychain, not settings files")
                        .font(.caption)
                        .foregroundStyle(.secondary)
                    Text(configured ? "Keychain token configured" : "No token saved")
                        .font(.caption.weight(.semibold))
                        .foregroundStyle(configured ? .green : .secondary)
                }
                Spacer()
            }

            HStack(spacing: 8) {
                Color.clear
                    .frame(width: 36)
                Group {
                    if tokenVisible {
                        TextField(configured ? "Paste replacement token" : "Paste bearer token", text: $token)
                    } else {
                        SecureField(configured ? "Paste replacement token" : "Paste bearer token", text: $token)
                    }
                }
                .textFieldStyle(.roundedBorder)
                .font(.system(.body, design: .monospaced))
                .onSubmit {
                    guard !trimmedToken.isEmpty else { return }
                    saveAction()
                }

                Button {
                    tokenVisible.toggle()
                } label: {
                    Image(systemName: tokenVisible ? "eye.slash" : "eye")
                }
                .buttonStyle(.bordered)
                .help(tokenVisible ? "Hide token" : "Show pasted token")
            }

            HStack(spacing: 8) {
                Color.clear
                    .frame(width: 36)
                Spacer()
                Button(role: .destructive, action: clearAction) {
                    Label("Remove token", systemImage: "trash")
                }
                .buttonStyle(.bordered)
                .disabled(!configured)
                .help("Remove the saved transcription token from Keychain")

                Button(action: saveAction) {
                    Label("Save token to Keychain", systemImage: "key.fill")
                }
                .buttonStyle(.borderedProminent)
                .disabled(trimmedToken.isEmpty)
                .help("Save the pasted transcription token to Keychain")
            }
        }
        .frame(maxWidth: .infinity, alignment: .leading)
        .padding(.vertical, 8)
        .overlay(alignment: .bottom) {
            Divider()
                .padding(.leading, 36)
        }
    }
}

private struct SettingsStepperRow: View {
    let title: String
    let subtitle: String
    let symbolName: String
    @Binding var value: Int
    let range: ClosedRange<Int>
    let unit: String

    private var clampedValue: Binding<Int> {
        Binding(
            get: { value },
            set: { value = min(max($0, range.lowerBound), range.upperBound) }
        )
    }

    var body: some View {
        SettingsBaseRow(symbolName: symbolName, title: title, subtitle: subtitle) {
            HStack(spacing: 8) {
                TextField("Value", value: clampedValue, format: .number)
                    .font(.system(.body, design: .monospaced))
                    .multilineTextAlignment(.trailing)
                    .textFieldStyle(.roundedBorder)
                    .frame(width: 76)
                Text(unit)
                    .font(.system(.body, design: .monospaced))
                    .foregroundStyle(.secondary)
                    .frame(width: 42, alignment: .leading)
                Stepper("", value: clampedValue, in: range)
                    .labelsHidden()
            }
        }
    }
}

private struct SettingsReadOnlyRow: View {
    let title: String
    let value: String
    let symbolName: String

    var body: some View {
        SettingsBaseRow(symbolName: symbolName, title: title, subtitle: "") {
            Text(value)
                .font(.system(.body, design: .monospaced))
                .foregroundStyle(.secondary)
                .lineLimit(2)
                .truncationMode(.middle)
                .textSelection(.enabled)
                .frame(maxWidth: .infinity, alignment: .trailing)
        }
    }
}

private struct TranscriptionSettingsStatusRow: View {
    @ObservedObject var viewModel: TranscriptionViewModel

    var body: some View {
        SettingsReadOnlyRow(
            title: "Connection status",
            value: viewModel.sessionStateText,
            symbolName: viewModel.status.symbolName
        )
    }
}

private struct SettingsBaseRow<Trailing: View>: View {
    let symbolName: String
    let title: String
    let subtitle: String
    @ViewBuilder var trailing: Trailing

    var body: some View {
        HStack(alignment: .center, spacing: 12) {
            Image(systemName: symbolName)
                .foregroundStyle(.tint)
                .frame(width: 24)
            VStack(alignment: .leading, spacing: 3) {
                Text(title)
                    .font(.body.weight(.semibold))
                if !subtitle.isEmpty {
                    Text(subtitle)
                        .font(.caption)
                        .foregroundStyle(.secondary)
                }
            }
            Spacer(minLength: 18)
            trailing
        }
        .frame(minHeight: 46)
        .padding(.vertical, 8)
        .overlay(alignment: .bottom) {
            Divider()
                .padding(.leading, 36)
        }
    }
}

private struct SettingsBanner: View {
    let text: String
    let symbolName: String
    let color: Color

    var body: some View {
        HStack(spacing: 10) {
            Image(systemName: symbolName)
                .foregroundStyle(color)
            Text(text)
                .font(.callout)
                .textSelection(.enabled)
            Spacer()
        }
        .padding(12)
        .background(color.opacity(0.10), in: RoundedRectangle(cornerRadius: 10, style: .continuous))
    }
}

private struct SettingsMetric: Identifiable {
    var title: String
    var value: String
    var symbolName: String
    var color: Color

    var id: String { title }
}

private struct SettingsMetricGrid: View {
    let metrics: [SettingsMetric]

    var body: some View {
        Grid(horizontalSpacing: 12, verticalSpacing: 12) {
            GridRow {
                ForEach(metrics) { metric in
                    VStack(alignment: .leading, spacing: 8) {
                        Image(systemName: metric.symbolName)
                            .foregroundStyle(metric.color)
                        Text(metric.value)
                            .font(.headline)
                        Text(metric.title.uppercased())
                            .font(.caption.weight(.semibold))
                            .foregroundStyle(.secondary)
                    }
                    .frame(maxWidth: .infinity, alignment: .leading)
                    .padding(14)
                    .background(.regularMaterial, in: RoundedRectangle(cornerRadius: 10, style: .continuous))
                }
            }
        }
    }
}

private extension OAuthProviderKind {
    var settingsSymbolName: String {
        switch self {
        case .webex:
            return "bubble.left.and.bubble.right"
        case .outlook:
            return "envelope"
        }
    }
}

private extension OAuthProviderStatus {
    var settingsStatusText: String {
        switch healthState {
        case OAuthTokenHealthState.healthy.rawValue:
            return "Connected"
        case OAuthTokenHealthState.expiringSoon.rawValue,
             OAuthTokenHealthState.refreshExpiringSoon.rawValue,
             OAuthTokenHealthState.unknownExpiry.rawValue:
            return "Connected"
        case OAuthTokenHealthState.expired.rawValue,
             OAuthTokenHealthState.refreshExpired.rawValue,
             OAuthTokenHealthState.missingRefreshToken.rawValue:
            return "Reconnect"
        case OAuthTokenHealthState.invalidTokenFile.rawValue,
             OAuthTokenHealthState.missingAccessToken.rawValue:
            return "Invalid"
        case OAuthTokenHealthState.missingTokenFile.rawValue:
            return "Not connected"
        default:
            return "Unknown"
        }
    }

    var settingsStatusColor: Color {
        switch healthState {
        case OAuthTokenHealthState.healthy.rawValue,
             OAuthTokenHealthState.unknownExpiry.rawValue:
            return .green
        case OAuthTokenHealthState.expiringSoon.rawValue,
             OAuthTokenHealthState.refreshExpiringSoon.rawValue:
            return .orange
        case OAuthTokenHealthState.missingTokenFile.rawValue:
            return .secondary
        default:
            return .red
        }
    }

    var settingsSubtitle: String {
        if let parseError, !parseError.isEmpty {
            return parseError
        }
        if let path = resolvedTokenFilePath, !path.isEmpty {
            let expiry = accessTokenExpiresAt.map {
                "expires \(SettingsDateFormatter.shortString(from: $0))"
            } ?? "expiry unknown"
            return "\(expiry) - \(path)"
        }
        return "Use OAuth to create a local token, or revoke an existing local token."
    }
}

private enum SettingsDateFormatter {
    static func string(from date: Date) -> String {
        formatter.string(from: date)
    }

    static func shortString(from date: Date) -> String {
        shortFormatter.string(from: date)
    }

    private static let formatter: DateFormatter = {
        let formatter = DateFormatter()
        formatter.dateStyle = .medium
        formatter.timeStyle = .medium
        return formatter
    }()

    private static let shortFormatter: DateFormatter = {
        let formatter = DateFormatter()
        formatter.dateStyle = .short
        formatter.timeStyle = .short
        return formatter
    }()
}

private extension SystemSettingsAction {
    var displayTitle: String {
        switch self {
        case .syncWebex:
            return "Sync Webex"
        case .rebuildPersonFocusAll:
            return "Rebuild Person Focus"
        case .rebuildSpaceFocusAll:
            return "Rebuild Space Focus"
        }
    }
}

struct PlaceholderView: View {
    let title: String
    let symbolName: String
    let message: String

    var body: some View {
        VStack(spacing: 14) {
            Image(systemName: symbolName)
                .font(.system(size: 42))
                .foregroundStyle(.tint)
            Text(title)
                .font(.title2.bold())
            Text(message)
                .multilineTextAlignment(.center)
                .foregroundStyle(.secondary)
                .frame(maxWidth: 460)
        }
        .frame(maxWidth: .infinity, maxHeight: .infinity)
        .padding(32)
    }
}
