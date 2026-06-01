import SwiftUI

/// Target-management screen for important people, spaces, and belief targets.
struct FocusTargetManagementView: View {
    @EnvironmentObject private var model: AppModel
    let kind: FocusTargetManagementKind

    @State private var targetSearchText = ""
    @State private var candidateSearchText = ""
    @State private var pendingRemoval: ConfigTarget?

    var body: some View {
        ScrollView {
            VStack(alignment: .leading, spacing: 16) {
                FocusTargetHeader(kind: kind)

                if let message = model.targetManagementLastMessage {
                    FocusTargetBanner(text: message, symbolName: "checkmark.circle", color: .green)
                }
                if let error = model.targetManagementLastError {
                    FocusTargetBanner(text: error, symbolName: "exclamationmark.triangle", color: .red)
                }

                FocusTargetMetricStrip(
                    currentCount: model.focusTargets(for: kind).count,
                    candidateCount: model.focusCandidates(for: kind).count,
                    sourceFilename: kind.sourceFilename
                )

                FocusTargetPanel(title: "Current \(kind.shortTitle)") {
                    VStack(alignment: .leading, spacing: 10) {
                        TextField("Search current \(kind.entityPlural)", text: $targetSearchText)
                            .textFieldStyle(.roundedBorder)
                        if filteredTargets.isEmpty {
                            FocusTargetEmptyState(
                                symbolName: kind.symbolName,
                                title: "No configured \(kind.entityPlural)",
                                message: "Use Add \(kind.addPanelObject) below to add indexed Webex \(kind.entityPlural) to \(kind.shortTitle)."
                            )
                        } else {
                            LazyVStack(spacing: 8) {
                                ForEach(filteredTargets) { target in
                                    FocusTargetRow(
                                        target: target,
                                        kind: kind,
                                        isSelected: model.selectedFocusTargetID(for: kind) == target.id,
                                        isRemoving: model.isTargetMutationRunning(action: "remove", target: target, kind: kind),
                                        isAutoReplyUpdating: model.isTargetMutationRunning(action: "auto-reply", target: target, kind: .personFocus),
                                        onSelect: { model.setSelectedFocusTargetID(target.id, for: kind) },
                                        onRemove: { pendingRemoval = target },
                                        onAutoReplyChange: { isEnabled in
                                            model.setPersonFocusAutoReply(isEnabled, for: target)
                                        }
                                    )
                                }
                            }
                        }
                    }
                }

                FocusTargetPanel(title: "Add \(kind.addPanelObject)") {
                    VStack(alignment: .leading, spacing: 10) {
                        TextField("Search indexed Webex \(kind.entityPlural)", text: $candidateSearchText)
                            .textFieldStyle(.roundedBorder)
                        if visibleCandidates.isEmpty {
                            FocusTargetEmptyState(
                                symbolName: "magnifyingglass",
                                title: "No addable \(kind.entityPlural)",
                                message: candidateSearchText.trimmingCharacters(in: .whitespacesAndNewlines).isEmpty
                                    ? "Every indexed \(kind.entitySingular) is already configured for \(kind.shortTitle)."
                                    : "No indexed \(kind.entityPlural) match this search."
                            )
                        } else {
                            LazyVStack(spacing: 8) {
                                ForEach(visibleCandidates) { candidate in
                                    FocusCandidateRow(
                                        target: candidate,
                                        kind: kind,
                                        isAdding: model.isTargetMutationRunning(action: "add", target: candidate, kind: kind),
                                        onAdd: { model.addFocusTarget(candidate, to: kind) }
                                    )
                                }
                            }
                            if filteredCandidates.count > visibleCandidates.count {
                                Text("Showing \(visibleCandidates.count) of \(filteredCandidates.count). Search to narrow the list.")
                                    .font(.caption)
                                    .foregroundStyle(.secondary)
                            }
                        }
                    }
                }

                FocusTargetPanel(title: "Source") {
                    FocusTargetSourceRow(
                        title: "File",
                        value: model.focusTargetSourcePath(for: kind),
                        symbolName: "doc.text"
                    )
                    if kind == .personFocus {
                        FocusTargetSourceRow(
                            title: "Auto-Reply",
                            value: model.configStore.personFocusPreferencesURL.path,
                            symbolName: "arrowshape.turn.up.left.2"
                        )
                    }
                    FocusTargetSourceRow(
                        title: "Indexed \(kind.entityPlural)",
                        value: model.configStore.mapFileURL.path,
                        symbolName: "map"
                    )
                }
            }
            .padding(24)
            .frame(maxWidth: .infinity, alignment: .leading)
        }
        .navigationTitle(kind.shortTitle)
        .onAppear {
            model.reloadFocusTargetManagement()
        }
        .confirmationDialog(
            removalTitle,
            isPresented: Binding(
                get: { pendingRemoval != nil },
                set: { if !$0 { pendingRemoval = nil } }
            ),
            titleVisibility: .visible
        ) {
            if let pendingRemoval {
                Button("Remove", role: .destructive) {
                    let target = pendingRemoval
                    self.pendingRemoval = nil
                    model.removeFocusTarget(target, from: kind)
                }
            }
            Button("Cancel", role: .cancel) {
                pendingRemoval = nil
            }
        } message: {
            Text("This updates \(kind.sourceFilename) and queues affected focus refreshes.")
        }
    }

    private var removalTitle: String {
        guard let pendingRemoval else {
            return "Remove target?"
        }
        return "Remove \(pendingRemoval.label) from \(kind.shortTitle)?"
    }

    private var filteredTargets: [ConfigTarget] {
        filtered(model.focusTargets(for: kind), query: targetSearchText)
    }

    private var filteredCandidates: [ConfigTarget] {
        filtered(model.focusCandidates(for: kind), query: candidateSearchText)
    }

    private var visibleCandidates: [ConfigTarget] {
        Array(filteredCandidates.prefix(80))
    }

    private func filtered(_ targets: [ConfigTarget], query: String) -> [ConfigTarget] {
        let searchText = query.trimmingCharacters(in: .whitespacesAndNewlines)
        guard !searchText.isEmpty else { return targets }
        return targets.filter { $0.focusTargetSearchText.localizedCaseInsensitiveContains(searchText) }
    }
}

/// Detail pane for a selected configured target.
struct FocusTargetDetailView: View {
    @EnvironmentObject private var model: AppModel
    let kind: FocusTargetManagementKind

    var body: some View {
        ScrollView {
            VStack(alignment: .leading, spacing: 16) {
                if let target = model.selectedFocusTarget(for: kind) {
                    FocusTargetDetailHeader(target: target, kind: kind)

                    FocusTargetPanel(title: "Details") {
                        FocusTargetSourceRow(title: "State", value: detailStateText(for: target), symbolName: kind.symbolName)
                        if kind == .personFocus {
                            FocusTargetSourceRow(title: "Auto-Reply", value: target.autoReply ? "Yes" : "No", symbolName: "arrowshape.turn.up.left.2")
                            FocusTargetSourceRow(title: "iMessage", value: target.iMessageHandles.isEmpty ? "Not configured" : "\(target.iMessageHandles.count) handle(s)", symbolName: "message")
                        }
                        FocusTargetSourceRow(title: "Room ID", value: target.roomID, symbolName: "number")
                        if !target.roomType.isEmpty {
                            FocusTargetSourceRow(title: "Room type", value: target.roomType, symbolName: "rectangle.3.group")
                        }
                        if !target.email.isEmpty {
                            FocusTargetSourceRow(title: "Email", value: target.email, symbolName: "envelope")
                        }
                        if let lineNumber = target.sourceMetadata?.lineNumber {
                            FocusTargetSourceRow(title: "Source row", value: "\(kind.sourceFilename):\(lineNumber)", symbolName: "list.number")
                        }
                    }

                    if kind == .personFocus {
                        FocusTargetPanel(title: "iMessage Handles") {
                            VStack(alignment: .leading, spacing: 10) {
                                if target.iMessageHandles.isEmpty {
                                    Text("No iMessage handles configured.")
                                        .font(.callout)
                                        .foregroundStyle(.secondary)
                                } else {
                                    VStack(spacing: 8) {
                                        ForEach(target.iMessageHandles, id: \.self) { handle in
                                            FocusTargetHandleRow(
                                                handle: handle,
                                                isRemoving: model.targetManagementMutationID == "personFocus:imessage-remove-\(handle):\(target.id)",
                                                onRemove: { model.removePersonIMessageHandle(handle, from: target) }
                                            )
                                        }
                                    }
                                }

                                HStack(spacing: 10) {
                                    TextField("Phone number or iMessage email", text: $model.personIMessageHandleDraft)
                                        .textFieldStyle(.roundedBorder)
                                        .onSubmit {
                                            model.addPersonIMessageHandle(to: target)
                                        }
                                    Button {
                                        model.addPersonIMessageHandle(to: target)
                                    } label: {
                                        Label("Add", systemImage: "plus.circle")
                                    }
                                    .buttonStyle(.bordered)
                                    .disabled(model.targetManagementMutationID != nil)
                                }
                            }
                        }
                    }

                    FocusTargetPanel(title: "Actions") {
                        if kind == .personFocus {
                            Toggle("Auto-Reply", isOn: Binding(
                                get: { target.autoReply },
                                set: { model.setPersonFocusAutoReply($0, for: target) }
                            ))
                            .toggleStyle(.switch)
                            .disabled(model.targetManagementMutationID != nil)
                            .padding(.vertical, 8)
                        }
                        Button(role: .destructive) {
                            model.removeFocusTarget(target, from: kind)
                        } label: {
                            Label("Remove from \(kind.shortTitle)", systemImage: "minus.circle")
                        }
                        .buttonStyle(.bordered)
                        .disabled(model.targetManagementMutationID != nil)
                    }

                    FocusTargetPanel(title: "Source") {
                        FocusTargetSourceRow(title: "File", value: model.focusTargetSourcePath(for: kind), symbolName: "doc.text")
                        if kind == .personFocus {
                            FocusTargetSourceRow(title: "iMessage handles", value: model.configStore.personFocusPreferencesURL.path, symbolName: "message")
                        }
                    }
                } else {
                    PlaceholderView(
                        title: kind.shortTitle,
                        symbolName: kind.symbolName,
                        message: "Select a configured \(kind.entitySingular) or add one from the indexed Webex \(kind.entityPlural) list."
                    )
                }
            }
            .padding(24)
            .frame(maxWidth: .infinity, alignment: .topLeading)
        }
    }

    private func detailStateText(for target: ConfigTarget) -> String {
        switch kind {
        case .spaceFocus:
            return "Included in Space Focus."
        case .personFocus:
            return "Included in Person Focus."
        case .execFocus:
            return "Included in Exec Focus. Space Focus treats this person as an important topic initiator."
        }
    }
}

/// Header for one target-management mode.
private struct FocusTargetHeader: View {
    let kind: FocusTargetManagementKind

    var body: some View {
        VStack(alignment: .leading, spacing: 6) {
            HStack(spacing: 10) {
                Image(systemName: kind.symbolName)
                    .foregroundStyle(.tint)
                Text(kind.title)
                    .font(.title2.bold())
            }
            Text(subtitle)
                .font(.callout)
                .foregroundStyle(.secondary)
        }
    }

    private var subtitle: String {
        switch kind {
        case .spaceFocus:
            return "Manage the space watchlist backed by important-senders.txt."
        case .personFocus:
            return "Manage the person-only watchlist backed by important-senders.txt."
        case .execFocus:
            return "Manage the exec watchlist used by Space Focus topic and exec-question generation."
        }
    }
}

/// Metric strip for configured/candidate target counts.
private struct FocusTargetMetricStrip: View {
    let currentCount: Int
    let candidateCount: Int
    let sourceFilename: String

    var body: some View {
        HStack(spacing: 10) {
            FocusTargetMetric(value: "\(currentCount)", label: "Configured", symbolName: "checkmark.circle")
            FocusTargetMetric(value: "\(candidateCount)", label: "Addable", symbolName: "plus.circle")
            FocusTargetMetric(value: sourceFilename, label: "Source", symbolName: "doc.text")
        }
    }
}

/// Small count tile for target-management metrics.
private struct FocusTargetMetric: View {
    let value: String
    let label: String
    let symbolName: String

    var body: some View {
        VStack(alignment: .leading, spacing: 6) {
            Image(systemName: symbolName)
                .foregroundStyle(.tint)
            Text(value)
                .font(.headline)
                .lineLimit(1)
                .truncationMode(.middle)
            Text(label.uppercased())
                .font(.caption.weight(.semibold))
                .foregroundStyle(.secondary)
        }
        .frame(maxWidth: .infinity, alignment: .leading)
        .padding(12)
        .background(.regularMaterial, in: RoundedRectangle(cornerRadius: 8, style: .continuous))
    }
}

/// Framed panel used by target lists and candidate lists.
private struct FocusTargetPanel<Content: View>: View {
    let title: String
    @ViewBuilder var content: Content

    var body: some View {
        VStack(alignment: .leading, spacing: 12) {
            Text(title)
                .font(.headline)
            VStack(alignment: .leading, spacing: 0) {
                content
            }
            .frame(maxWidth: .infinity, alignment: .leading)
        }
        .padding(16)
        .background(.regularMaterial, in: RoundedRectangle(cornerRadius: 8, style: .continuous))
    }
}

/// Row for an already-configured target.
private struct FocusTargetRow: View {
    let target: ConfigTarget
    let kind: FocusTargetManagementKind
    let isSelected: Bool
    let isRemoving: Bool
    let isAutoReplyUpdating: Bool
    let onSelect: () -> Void
    let onRemove: () -> Void
    let onAutoReplyChange: (Bool) -> Void

    var body: some View {
        HStack(spacing: 10) {
            Button(action: onSelect) {
                HStack(alignment: .top, spacing: 10) {
                    FocusTargetBadge(text: kind.badgeText)
                    VStack(alignment: .leading, spacing: 4) {
                        Text(target.label)
                            .font(.body.weight(.semibold))
                            .foregroundStyle(.primary)
                            .lineLimit(1)
                        Text(target.focusTargetSubtitle)
                            .font(.caption)
                            .foregroundStyle(.secondary)
                            .lineLimit(2)
                    }
                    Spacer(minLength: 8)
                }
                .contentShape(Rectangle())
            }
            .buttonStyle(.plain)

            if kind == .personFocus {
                Toggle("", isOn: Binding(
                    get: { target.autoReply },
                    set: onAutoReplyChange
                ))
                .labelsHidden()
                .toggleStyle(.switch)
                .disabled(isAutoReplyUpdating)
                .help("Toggle Auto-Reply for this Person Focus target")
            }

            Button(role: .destructive, action: onRemove) {
                if isRemoving {
                    ProgressView()
                        .controlSize(.small)
                } else {
                    Image(systemName: "minus.circle")
                }
            }
            .buttonStyle(.borderless)
            .disabled(isRemoving)
            .help("Remove from \(kind.shortTitle)")
        }
        .padding(10)
        .background(isSelected ? Color.accentColor.opacity(0.12) : Color.clear, in: RoundedRectangle(cornerRadius: 8, style: .continuous))
        .overlay(
            RoundedRectangle(cornerRadius: 8, style: .continuous)
                .stroke(isSelected ? Color.accentColor.opacity(0.35) : Color.secondary.opacity(0.12), lineWidth: 1)
        )
    }
}

/// Row for a candidate target discovered from source maps.
private struct FocusCandidateRow: View {
    let target: ConfigTarget
    let kind: FocusTargetManagementKind
    let isAdding: Bool
    let onAdd: () -> Void

    var body: some View {
        HStack(spacing: 10) {
            FocusTargetBadge(text: kind.badgeText)
            VStack(alignment: .leading, spacing: 4) {
                Text(target.label)
                    .font(.body.weight(.semibold))
                    .lineLimit(1)
                Text(target.focusTargetSubtitle)
                    .font(.caption)
                    .foregroundStyle(.secondary)
                    .lineLimit(2)
            }
            Spacer(minLength: 8)
            Button(action: onAdd) {
                if isAdding {
                    ProgressView()
                        .controlSize(.small)
                } else {
                    Image(systemName: "plus.circle.fill")
                }
            }
            .buttonStyle(.borderless)
            .disabled(isAdding)
            .help("Add this \(kind.entitySingular)")
        }
        .padding(10)
        .overlay(
            RoundedRectangle(cornerRadius: 8, style: .continuous)
                .stroke(Color.secondary.opacity(0.12), lineWidth: 1)
        )
    }
}

/// Type badge for target rows.
private struct FocusTargetBadge: View {
    let text: String

    var body: some View {
        Text(text.uppercased())
            .font(.caption2.weight(.bold))
            .foregroundStyle(.secondary)
            .padding(.horizontal, 7)
            .padding(.vertical, 4)
            .background(Color.secondary.opacity(0.10), in: Capsule())
    }
}

/// Header card for the selected target detail pane.
private struct FocusTargetDetailHeader: View {
    let target: ConfigTarget
    let kind: FocusTargetManagementKind

    var body: some View {
        VStack(alignment: .leading, spacing: 8) {
            HStack(spacing: 10) {
                Image(systemName: kind.symbolName)
                    .foregroundStyle(.tint)
                FocusTargetBadge(text: kind.badgeText)
            }
            Text(target.label)
                .font(.title2.bold())
                .textSelection(.enabled)
            Text(target.focusTargetSubtitle)
                .font(.callout)
                .foregroundStyle(.secondary)
                .textSelection(.enabled)
        }
    }
}

/// Row for one configured iMessage handle.
private struct FocusTargetHandleRow: View {
    let handle: String
    let isRemoving: Bool
    let onRemove: () -> Void

    var body: some View {
        HStack(spacing: 10) {
            Image(systemName: "message")
                .foregroundStyle(.tint)
                .frame(width: 22)
            Text(handle)
                .font(.system(.body, design: .monospaced))
                .textSelection(.enabled)
            Spacer(minLength: 12)
            Button(role: .destructive, action: onRemove) {
                if isRemoving {
                    ProgressView()
                        .controlSize(.small)
                } else {
                    Image(systemName: "minus.circle")
                }
            }
            .buttonStyle(.borderless)
            .disabled(isRemoving)
        }
        .frame(minHeight: 34)
        .padding(.vertical, 4)
    }
}

/// Source metadata row for the selected target.
private struct FocusTargetSourceRow: View {
    let title: String
    let value: String
    let symbolName: String

    var body: some View {
        HStack(alignment: .firstTextBaseline, spacing: 12) {
            Image(systemName: symbolName)
                .foregroundStyle(.tint)
                .frame(width: 22)
            Text(title)
                .font(.body.weight(.semibold))
            Spacer(minLength: 14)
            Text(value)
                .font(.system(.body, design: .monospaced))
                .foregroundStyle(.secondary)
                .lineLimit(3)
                .truncationMode(.middle)
                .multilineTextAlignment(.trailing)
                .textSelection(.enabled)
        }
        .frame(minHeight: 42)
        .padding(.vertical, 6)
        .overlay(alignment: .bottom) {
            Divider()
                .padding(.leading, 34)
        }
    }
}

/// Status/error banner used in target management.
private struct FocusTargetBanner: View {
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
        .background(color.opacity(0.10), in: RoundedRectangle(cornerRadius: 8, style: .continuous))
    }
}

/// Empty state for target-management panels.
private struct FocusTargetEmptyState: View {
    let symbolName: String
    let title: String
    let message: String

    var body: some View {
        VStack(spacing: 8) {
            Image(systemName: symbolName)
                .font(.title2)
                .foregroundStyle(.secondary)
            Text(title)
                .font(.headline)
            Text(message)
                .font(.caption)
                .foregroundStyle(.secondary)
                .multilineTextAlignment(.center)
        }
        .frame(maxWidth: .infinity)
        .padding(18)
    }
}

/// Presentation helpers for configured targets.
private extension ConfigTarget {
    var focusTargetSubtitle: String {
        var parts: [String] = []
        if !email.isEmpty {
            parts.append(email)
        }
        if !roomID.isEmpty {
            parts.append("room \(roomIDSuffix)")
        }
        if !iMessageHandles.isEmpty {
            parts.append("iMessage \(iMessageHandles.count)")
        }
        return parts.joined(separator: " | ")
    }

    var focusTargetSearchText: String {
        [label, email, roomID, roomIDSuffix, iMessageHandles.joined(separator: " ")]
            .joined(separator: " ")
    }

    private var roomIDSuffix: String {
        guard roomID.count > 12 else { return roomID }
        return String(roomID.suffix(12))
    }
}

/// Copy helpers for each target-management mode.
private extension FocusTargetManagementKind {
    var entitySingular: String {
        switch self {
        case .spaceFocus:
            return "space"
        case .personFocus, .execFocus:
            return "person"
        }
    }

    var entityPlural: String {
        switch self {
        case .spaceFocus:
            return "spaces"
        case .personFocus, .execFocus:
            return "people"
        }
    }

    var addPanelObject: String {
        switch self {
        case .spaceFocus:
            return "Spaces"
        case .personFocus, .execFocus:
            return "People"
        }
    }
}
