import SwiftUI

/// Belief workspace for manual belief editing and automatic-belief review.
struct BeliefsView: View {
    @EnvironmentObject private var model: AppModel
    @State private var draftStatement: String = ""
    @State private var draftConfidence: Double = 0.7
    @State private var draftEvidenceText: String = ""
    @State private var editingBeliefID: String?
    @State private var isManualBeliefEditorPresented = false
    @State private var pendingDeleteBelief: BeliefRecord?
    @State private var actionMode: BeliefActionMode = .view
    @State private var browseScope: BeliefBrowseScope = .all
    @State private var browseFilter: String = ""

    var body: some View {
        ScrollView {
            VStack(alignment: .leading, spacing: 16) {
                SectionHeader(
                    title: "Beliefs",
                    subtitle: "Belief actions for view, deep maintenance, edit, and browse."
                )

                actionModeCard
                actionBody

                if let error = model.beliefsLastError, !error.isEmpty {
                    BeliefStatusCard(
                        title: "Belief Operation Failed",
                        message: error,
                        color: .red,
                        symbolName: "xmark.octagon"
                    )
                }
            }
            .padding(24)
            .frame(maxWidth: .infinity, alignment: .leading)
        }
        .navigationTitle("Beliefs")
        .onAppear {
            Task {
                await model.refreshBeliefs()
                await model.refreshBeliefSetSummaries()
            }
        }
        .sheet(isPresented: $isManualBeliefEditorPresented) {
            manualBeliefEditorSheet
        }
        .confirmationDialog(
            "Delete manual belief?",
            isPresented: Binding(
                get: { pendingDeleteBelief != nil },
                set: { isPresented in
                    if !isPresented {
                        pendingDeleteBelief = nil
                    }
                }
            ),
            presenting: pendingDeleteBelief
        ) { belief in
            Button("Delete Belief", role: .destructive) {
                Task {
                    await model.deleteManualBelief(id: belief.id)
                    if editingBeliefID == belief.id {
                        clearDraft()
                    }
                    pendingDeleteBelief = nil
                }
            }
            Button("Cancel", role: .cancel) {
                pendingDeleteBelief = nil
            }
        } message: { belief in
            Text(belief.statement)
        }
    }

    @ViewBuilder
    private var actionBody: some View {
        switch actionMode {
        case .view:
            filtersCard
            selectedBeliefSetCard
            manualBeliefsCard
            beliefInspectorCard
        case .browse:
            browseBeliefSetsCard
        case .edit:
            filtersCard
            manualBeliefsCard
            beliefInspectorCard
        case .deep:
            deepBeliefMaintenanceCard
        }
    }

    private var actionModeCard: some View {
        VStack(alignment: .leading, spacing: 12) {
            Text("Belief Actions")
                .font(.headline)

            HStack(spacing: 8) {
                ForEach(BeliefActionMode.allCases) { mode in
                    Button {
                        actionMode = mode
                        if mode == .browse {
                            Task { await model.refreshBeliefSetSummaries() }
                        } else if mode == .view || mode == .edit {
                            Task { await model.refreshBeliefs() }
                        }
                    } label: {
                        Text(mode.title)
                            .font(.subheadline.weight(.semibold))
                            .padding(.horizontal, 12)
                            .padding(.vertical, 7)
                            .frame(maxWidth: .infinity)
                            .foregroundStyle(actionMode == mode ? .white : .primary)
                            .background(
                                RoundedRectangle(cornerRadius: 8, style: .continuous)
                                    .fill(actionMode == mode ? Color.blue : Color.secondary.opacity(0.12))
                            )
                    }
                    .buttonStyle(.plain)
                }
            }

            Text(actionMode.subtitle)
                .font(.caption)
                .foregroundStyle(.secondary)
        }
        .padding(14)
        .background(.regularMaterial, in: RoundedRectangle(cornerRadius: 12, style: .continuous))
    }

    private var filtersCard: some View {
        VStack(alignment: .leading, spacing: 12) {
            Text("Scope + Target")
                .font(.headline)

            Picker("Scope", selection: scopeBinding) {
                ForEach(KnowledgeBeliefScope.allCases, id: \.self) { scope in
                    Text(scope.rawValue.capitalized).tag(scope)
                }
            }
            .pickerStyle(.segmented)

            if model.beliefScopeFilter != .global {
                let options = model.beliefTargetOptions(for: model.beliefScopeFilter)
                if options.isEmpty {
                    Text("No targets are available for this scope yet.")
                        .font(.caption)
                        .foregroundStyle(.secondary)
                } else {
                    Picker("Target", selection: targetBinding) {
                        ForEach(options) { option in
                            Text(option.title).tag(option.entityKey)
                        }
                    }
                    .pickerStyle(.menu)
                }
            } else {
                Text("Target: Global")
                    .font(.caption)
                    .foregroundStyle(.secondary)
            }

            HStack {
                Button("Refresh Beliefs") {
                    Task { await model.refreshSelectedPageNow() }
                }
                .buttonStyle(.bordered)

                if model.beliefsIsLoading {
                    ProgressView()
                        .controlSize(.small)
                }
            }
        }
        .padding(14)
        .background(.regularMaterial, in: RoundedRectangle(cornerRadius: 12, style: .continuous))
    }

    private var selectedBeliefSetCard: some View {
        VStack(alignment: .leading, spacing: 8) {
            Text("Current Belief Set")
                .font(.headline)
            Text(currentBeliefSetLabel)
                .font(.subheadline.weight(.semibold))
            Text("Autolearnt and manual sections are shown separately below for quick inspection.")
                .font(.caption)
                .foregroundStyle(.secondary)
        }
        .padding(14)
        .background(.regularMaterial, in: RoundedRectangle(cornerRadius: 12, style: .continuous))
    }

    private var browseBeliefSetsCard: some View {
        VStack(alignment: .leading, spacing: 12) {
            Text("Browse Stored Belief Sets")
                .font(.headline)

            Picker("Scope", selection: $browseScope) {
                ForEach(BeliefBrowseScope.allCases) { scope in
                    Text(scope.title).tag(scope)
                }
            }
            .pickerStyle(.segmented)

            TextField("Filter by person, space, or entity key", text: $browseFilter)
                .textFieldStyle(.roundedBorder)

            HStack(spacing: 10) {
                Button("Refresh Stored Sets") {
                    Task { await model.refreshBeliefSetSummaries(force: true) }
                }
                .buttonStyle(.bordered)

                if model.beliefSetSummariesIsLoading {
                    ProgressView()
                        .controlSize(.small)
                }
            }

            if let error = model.beliefSetSummariesLastError, !error.isEmpty {
                Text(error)
                    .font(.caption)
                    .foregroundStyle(.red)
                    .textSelection(.enabled)
            }

            if filteredBeliefSetSummaries.isEmpty {
                Text("No stored belief sets matched this filter.")
                    .foregroundStyle(.secondary)
            } else {
                ForEach(filteredBeliefSetSummaries) { summary in
                    Button {
                        model.selectBeliefSetSummary(summary)
                        actionMode = .view
                        Task { await model.refreshBeliefs() }
                    } label: {
                        BeliefSetSummaryRow(summary: summary)
                    }
                    .buttonStyle(.plain)
                }
            }
        }
        .padding(14)
        .background(.regularMaterial, in: RoundedRectangle(cornerRadius: 12, style: .continuous))
    }

    private var manualEditorCard: some View {
        VStack(alignment: .leading, spacing: 10) {
            Text(editingBeliefID == nil ? "Add Manual Belief" : "Edit Manual Belief")
                .font(.headline)

            TextField("Belief statement", text: $draftStatement, axis: .vertical)
                .textFieldStyle(.roundedBorder)
                .lineLimit(3...6)

            VStack(alignment: .leading, spacing: 6) {
                Text("Confidence: \(draftConfidence, specifier: "%.2f")")
                    .font(.caption)
                    .foregroundStyle(.secondary)
                Slider(value: $draftConfidence, in: 0...1, step: 0.05)
            }

            VStack(alignment: .leading, spacing: 6) {
                Text("Evidence Links (one per line)")
                    .font(.caption)
                    .foregroundStyle(.secondary)
                TextEditor(text: $draftEvidenceText)
                    .font(.system(.caption, design: .monospaced))
                    .frame(minHeight: 80)
                    .padding(6)
                    .background(
                        RoundedRectangle(cornerRadius: 8, style: .continuous)
                            .fill(Color.secondary.opacity(0.08))
                    )
            }

            HStack(spacing: 10) {
                Button(editingBeliefID == nil ? "Add Manual Belief" : "Save Changes") {
                    Task {
                        await saveManualBeliefDraft()
                    }
                }
                .buttonStyle(.borderedProminent)
                .disabled(draftStatement.trimmingCharacters(in: .whitespacesAndNewlines).isEmpty)

                Button("Clear") {
                    clearDraft()
                }
                .buttonStyle(.bordered)
            }
        }
        .padding(14)
        .background(.regularMaterial, in: RoundedRectangle(cornerRadius: 12, style: .continuous))
    }

    private var manualBeliefEditorSheet: some View {
        VStack(alignment: .leading, spacing: 16) {
            HStack {
                VStack(alignment: .leading, spacing: 4) {
                    Text(editingBeliefID == nil ? "Add Manual Belief" : "Edit Manual Belief")
                        .font(.title3.weight(.semibold))
                    Text(currentBeliefSetLabel)
                        .font(.caption)
                        .foregroundStyle(.secondary)
                }
                Spacer()
            }

            VStack(alignment: .leading, spacing: 8) {
                Text("Belief")
                    .font(.headline)
                TextEditor(text: $draftStatement)
                    .font(.body)
                    .frame(minHeight: 140)
                    .padding(8)
                    .background(
                        RoundedRectangle(cornerRadius: 10, style: .continuous)
                            .fill(Color.secondary.opacity(0.08))
                    )
            }

            VStack(alignment: .leading, spacing: 8) {
                HStack {
                    Text("Confidence")
                        .font(.headline)
                    Spacer()
                    Text(String(format: "%.2f", draftConfidence))
                        .font(.caption.monospacedDigit())
                        .foregroundStyle(.secondary)
                }
                Slider(value: $draftConfidence, in: 0...1, step: 0.05)
            }

            VStack(alignment: .leading, spacing: 8) {
                Text("Evidence Links")
                    .font(.headline)
                TextEditor(text: $draftEvidenceText)
                    .font(.system(.caption, design: .monospaced))
                    .frame(minHeight: 100)
                    .padding(8)
                    .background(
                        RoundedRectangle(cornerRadius: 10, style: .continuous)
                            .fill(Color.secondary.opacity(0.08))
                    )
            }

            HStack {
                Button("Cancel") {
                    isManualBeliefEditorPresented = false
                }
                .buttonStyle(.bordered)

                Spacer()

                Button(editingBeliefID == nil ? "Add Belief" : "Save Belief") {
                    Task { await saveManualBeliefDraft() }
                }
                .buttonStyle(.borderedProminent)
                .disabled(draftStatement.trimmingCharacters(in: .whitespacesAndNewlines).isEmpty)
            }
        }
        .padding(22)
        .frame(width: 560)
    }

    private var deepBeliefMaintenanceCard: some View {
        let status = model.beliefMaintenanceStatus()
        return VStack(alignment: .leading, spacing: 12) {
            Text("Deep Belief Maintenance")
                .font(.headline)

            Text("Runs the same deep reconciliation pipeline used by background belief maintenance.")
                .font(.subheadline)
                .foregroundStyle(.secondary)

            HStack(spacing: 10) {
                Button(status.isRunning ? "Running…" : "Run Deep Belief Maintenance") {
                    Task { await model.runBeliefMaintenanceNow() }
                }
                .buttonStyle(.borderedProminent)
                .disabled(status.isRunning)

                if status.isRunning {
                    ProgressView()
                        .controlSize(.small)
                }
            }

            if let started = status.lastStartedAt, !started.isEmpty {
                Text("Last Started: \(DisplayFormatters.localDateTime(started))")
                    .font(.caption)
                    .foregroundStyle(.secondary)
            }
            if let completed = status.lastCompletedAt, !completed.isEmpty {
                Text("Last Completed: \(DisplayFormatters.localDateTime(completed))")
                    .font(.caption)
                    .foregroundStyle(.secondary)
            }
            if let summary = status.lastSummary, !summary.isEmpty {
                Text(summary)
                    .font(.caption)
                    .textSelection(.enabled)
            }
            if let error = status.lastError, !error.isEmpty {
                Text(error)
                    .font(.caption)
                    .foregroundStyle(.red)
                    .textSelection(.enabled)
            }
        }
        .padding(14)
        .background(.regularMaterial, in: RoundedRectangle(cornerRadius: 12, style: .continuous))
    }

    private var manualBeliefsCard: some View {
        VStack(alignment: .leading, spacing: 10) {
            HStack {
                Text("Manual Beliefs (\(model.manualBeliefs.count))")
                    .font(.headline)
                Spacer()
                Button {
                    clearDraft()
                    actionMode = .edit
                    isManualBeliefEditorPresented = true
                } label: {
                    Label("Add", systemImage: "plus")
                }
                .buttonStyle(.bordered)
            }
            if model.manualBeliefs.isEmpty {
                Text("No manual beliefs for the selected scope/target.")
                    .foregroundStyle(.secondary)
            } else {
                ForEach(model.manualBeliefs) { belief in
                    VStack(alignment: .leading, spacing: 8) {
                        Button {
                            model.setSelectedBeliefID(belief.id)
                        } label: {
                            BeliefRow(
                                belief: belief,
                                isSelected: model.selectedBeliefID == belief.id
                            )
                        }
                        .buttonStyle(.plain)

                        HStack(spacing: 10) {
                            Button("Edit") {
                                model.setSelectedBeliefID(belief.id)
                                presentEditor(for: belief)
                            }
                            .buttonStyle(.bordered)

                            Button("Delete", role: .destructive) {
                                pendingDeleteBelief = belief
                            }
                            .buttonStyle(.bordered)
                        }
                    }
                    .padding(.vertical, 4)
                }
            }
        }
        .padding(14)
        .background(.regularMaterial, in: RoundedRectangle(cornerRadius: 12, style: .continuous))
    }

    private var beliefInspectorCard: some View {
        BeliefInspectorCard()
    }

    private var scopeBinding: Binding<KnowledgeBeliefScope> {
        Binding(
            get: { model.beliefScopeFilter },
            set: { newScope in
                model.setBeliefScopeFilter(newScope)
                Task { await model.refreshBeliefs() }
            }
        )
    }

    private var targetBinding: Binding<String> {
        Binding(
            get: {
                model.selectedBeliefEntityKey(for: model.beliefScopeFilter) ?? ""
            },
            set: { newTarget in
                model.setBeliefEntityKey(newTarget, for: model.beliefScopeFilter)
                Task { await model.refreshBeliefs() }
            }
        )
    }

    private func parseEvidenceLinks(from text: String) -> [String] {
        text.split(whereSeparator: \.isNewline)
            .map { String($0).trimmingCharacters(in: .whitespacesAndNewlines) }
            .filter { !$0.isEmpty }
    }

    private func populateDraft(from belief: BeliefRecord) {
        editingBeliefID = belief.id
        draftStatement = belief.statement
        draftConfidence = belief.confidence
        draftEvidenceText = belief.evidenceLinks.joined(separator: "\n")
    }

    private func presentEditor(for belief: BeliefRecord) {
        populateDraft(from: belief)
        actionMode = .edit
        isManualBeliefEditorPresented = true
    }

    private func saveManualBeliefDraft() async {
        let evidenceLinks = parseEvidenceLinks(from: draftEvidenceText)
        if let editingBeliefID {
            await model.updateManualBelief(
                id: editingBeliefID,
                statement: draftStatement,
                confidence: draftConfidence,
                evidenceLinks: evidenceLinks
            )
        } else {
            await model.addManualBelief(
                statement: draftStatement,
                confidence: draftConfidence,
                evidenceLinks: evidenceLinks
            )
        }
        clearDraft()
        isManualBeliefEditorPresented = false
    }

    private func clearDraft() {
        editingBeliefID = nil
        draftStatement = ""
        draftConfidence = 0.7
        draftEvidenceText = ""
    }

    private var filteredBeliefSetSummaries: [BeliefSetSummary] {
        let scopeFiltered = model.beliefSetSummaries.filter { summary in
            switch browseScope {
            case .all:
                return true
            case .person:
                return summary.scope == .person
            case .space:
                return summary.scope == .space
            }
        }
        let query = browseFilter.trimmingCharacters(in: .whitespacesAndNewlines)
        guard !query.isEmpty else {
            return scopeFiltered
        }
        return scopeFiltered.filter { summary in
            "\(summary.title) \(summary.entityKey)"
                .localizedCaseInsensitiveContains(query)
        }
    }

    private var currentBeliefSetLabel: String {
        switch model.beliefScopeFilter {
        case .global:
            return "Global"
        case .person, .space:
            guard let entityKey = model.selectedBeliefEntityKey(for: model.beliefScopeFilter),
                  !entityKey.isEmpty else {
                return "No target selected"
            }
            if let option = model.beliefTargetOptions(for: model.beliefScopeFilter).first(where: { $0.entityKey == entityKey }) {
                return option.title
            }
            return entityKey
        }
    }
}

/// Editor mode for creating or updating manual beliefs.
private enum BeliefActionMode: String, CaseIterable, Identifiable {
    case view
    case browse
    case edit
    case deep

    var id: String { rawValue }

    var title: String {
        switch self {
        case .view:
            return "View"
        case .browse:
            return "Browse"
        case .edit:
            return "Edit"
        case .deep:
            return "Deep"
        }
    }

    var subtitle: String {
        switch self {
        case .view:
            return "Read the selected cached belief set with separate autolearnt and manual sections."
        case .browse:
            return "List every stored global/person/space belief set and open one directly."
        case .edit:
            return "Add, update, or delete manual beliefs for the current scope and target."
        case .deep:
            return "Trigger deep reconciliation for configured targets and persist refreshed beliefs."
        }
    }
}

/// Scope filter for browsing persisted beliefs.
private enum BeliefBrowseScope: String, CaseIterable, Identifiable {
    case all
    case person
    case space

    var id: String { rawValue }

    var title: String {
        switch self {
        case .all:
            return "All"
        case .person:
            return "People"
        case .space:
            return "Spaces"
        }
    }
}

/// Summary row for one belief scope bucket.
private struct BeliefSetSummaryRow: View {
    let summary: BeliefSetSummary

    var body: some View {
        HStack(alignment: .firstTextBaseline, spacing: 10) {
            VStack(alignment: .leading, spacing: 4) {
                HStack(spacing: 6) {
                    Badge(text: scopeLabel(summary.scope), color: summary.scope == .person ? .blue : (summary.scope == .space ? .orange : .green))
                    Text(summary.title)
                        .font(.subheadline.weight(.semibold))
                }
                Text("\(summary.autoCount) autolearnt | \(summary.manualCount) manual")
                    .font(.caption)
                    .foregroundStyle(.secondary)
                Text(summary.entityKey)
                    .font(.caption2.monospaced())
                    .foregroundStyle(.secondary)
                    .lineLimit(1)
            }
            Spacer()
            if !summary.updatedAt.trimmingCharacters(in: .whitespacesAndNewlines).isEmpty {
                Text(DisplayFormatters.localDateTime(summary.updatedAt))
                    .font(.caption2)
                    .foregroundStyle(.secondary)
                    .multilineTextAlignment(.trailing)
            }
        }
        .padding(10)
        .background(
            RoundedRectangle(cornerRadius: 10, style: .continuous)
                .fill(Color.secondary.opacity(0.08))
        )
    }

    private func scopeLabel(_ scope: KnowledgeBeliefScope) -> String {
        switch scope {
        case .global:
            return "global"
        case .person:
            return "person"
        case .space:
            return "space"
        }
    }
}

/// Detail view for automatic beliefs associated with a selected focus item.
struct AutomaticBeliefsDetailView: View {
    @EnvironmentObject private var model: AppModel

    var body: some View {
        ScrollView {
            VStack(alignment: .leading, spacing: 14) {
                SectionHeader(
                    title: "Automatic Beliefs",
                    subtitle: "Autolearnt beliefs for the selected Global, Person, or Space target."
                )
                AutomaticBeliefsCard()
            }
            .padding(24)
            .frame(maxWidth: .infinity, alignment: .leading)
        }
    }
}

/// Sidebar inspector for the selected belief.
struct BeliefInspectorView: View {
    var body: some View {
        ScrollView {
            VStack(alignment: .leading, spacing: 14) {
                SectionHeader(
                    title: "Belief Inspector",
                    subtitle: "Selected belief metadata, provenance, and evidence links."
                )
                BeliefInspectorCard()
            }
            .padding(24)
            .frame(maxWidth: .infinity, alignment: .leading)
        }
    }
}

/// Card listing automatic beliefs for one focus context.
private struct AutomaticBeliefsCard: View {
    @EnvironmentObject private var model: AppModel

    var body: some View {
        VStack(alignment: .leading, spacing: 10) {
            Text("Automatic Beliefs (\(model.automaticBeliefs.count))")
                .font(.headline)
            if model.automaticBeliefs.isEmpty {
                Text("No automatic beliefs for the selected scope/target.")
                    .foregroundStyle(.secondary)
            } else {
                ForEach(model.automaticBeliefs) { belief in
                    Button {
                        model.setSelectedBeliefID(belief.id)
                    } label: {
                        BeliefRow(
                            belief: belief,
                            isSelected: model.selectedBeliefID == belief.id
                        )
                    }
                    .buttonStyle(.plain)
                }
            }
        }
        .padding(14)
        .background(.regularMaterial, in: RoundedRectangle(cornerRadius: 12, style: .continuous))
    }
}

/// Detailed card for one belief and its evidence links.
private struct BeliefInspectorCard: View {
    @EnvironmentObject private var model: AppModel

    var body: some View {
        VStack(alignment: .leading, spacing: 10) {
            Text("Belief Inspector")
                .font(.headline)
            Text("Selected belief metadata, provenance, and evidence links.")
                .font(.caption)
                .foregroundStyle(.secondary)

            if let belief = model.selectedBelief() {
                HStack(spacing: 8) {
                    Badge(text: belief.isManual ? "manual" : "automatic", color: belief.isManual ? .blue : .green)
                    Badge(text: belief.scope, color: .secondary)
                    Badge(text: belief.lifecycle, color: belief.lifecycle == "stable" ? .purple : .secondary)
                    Badge(text: belief.beliefKind, color: .secondary)
                    Badge(text: String(format: "confidence %.2f", belief.confidence), color: .secondary)
                }

                Text(belief.statement)
                    .font(.headline)
                    .textSelection(.enabled)
                    .frame(maxWidth: .infinity, alignment: .leading)

                BeliefMetaRow(label: "Entity Key", value: belief.entityKey)
                BeliefMetaRow(label: "Belief ID", value: belief.id)
                BeliefMetaRow(label: "Kind", value: belief.beliefKind)
                BeliefMetaRow(label: "Lifecycle", value: belief.lifecycle)
                BeliefMetaRow(label: "Support", value: "\(belief.supportCount)")
                BeliefMetaRow(label: "Contradictions", value: "\(belief.contradictionCount)")
                BeliefMetaRow(label: "Last Evidence", value: belief.lastEvidenceAt)
                BeliefMetaRow(label: "Updated At", value: belief.updatedAt)
                BeliefMetaRow(label: "Created At", value: belief.createdAt)

                VStack(alignment: .leading, spacing: 6) {
                    Text("Evidence Links")
                        .font(.headline)
                    if belief.evidenceLinks.isEmpty {
                        Text("No evidence links attached.")
                            .foregroundStyle(.secondary)
                    } else {
                        ForEach(Array(belief.evidenceLinks.enumerated()), id: \.offset) { index, link in
                            Text("\(index + 1). \(link)")
                                .font(.system(.caption, design: .monospaced))
                                .textSelection(.enabled)
                                .frame(maxWidth: .infinity, alignment: .leading)
                            }
                        }
                }
            } else {
                Text("Select a manual or automatic belief to inspect its metadata and evidence links.")
                    .foregroundStyle(.secondary)
            }
        }
        .padding(14)
        .background(.regularMaterial, in: RoundedRectangle(cornerRadius: 12, style: .continuous))
    }
}

/// Label/value metadata row for belief cards.
private struct BeliefMetaRow: View {
    let label: String
    let value: String

    var body: some View {
        HStack(alignment: .top, spacing: 10) {
            Text(label)
                .font(.headline)
                .frame(width: 100, alignment: .leading)
            Text(value.isEmpty ? "-" : value)
                .font(.system(.caption, design: .monospaced))
                .foregroundStyle(.secondary)
                .textSelection(.enabled)
                .frame(maxWidth: .infinity, alignment: .leading)
        }
    }
}

/// Row summary for belief lists.
private struct BeliefRow: View {
    let belief: BeliefRecord
    let isSelected: Bool

    var body: some View {
        VStack(alignment: .leading, spacing: 6) {
            HStack(spacing: 8) {
                Text(belief.isManual ? "Manual" : "Auto")
                    .font(.caption2.weight(.semibold))
                    .padding(.horizontal, 7)
                    .padding(.vertical, 3)
                    .foregroundStyle(belief.isManual ? .blue : .green)
                    .background((belief.isManual ? Color.blue : Color.green).opacity(0.12), in: Capsule())
                Text(String(format: "Confidence %.2f", belief.confidence))
                    .font(.caption)
                    .foregroundStyle(.secondary)
                Text(belief.lifecycle)
                    .font(.caption)
                    .foregroundStyle(.secondary)
                if !belief.isManual {
                    Text("support \(belief.supportCount)")
                        .font(.caption)
                        .foregroundStyle(.secondary)
                }
                Spacer()
                Text(belief.updatedAt)
                    .font(.caption2)
                    .foregroundStyle(.secondary)
            }

            Text(belief.statement)
                .font(.subheadline)
                .foregroundStyle(.primary)
                .frame(maxWidth: .infinity, alignment: .leading)
        }
        .padding(10)
        .background(
            RoundedRectangle(cornerRadius: 10, style: .continuous)
                .fill(isSelected ? Color.blue.opacity(0.10) : Color.clear)
        )
    }
}

/// Empty/error/status card for belief surfaces.
private struct BeliefStatusCard: View {
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
                    .textSelection(.enabled)
            }
            Spacer()
        }
        .padding(12)
        .background(.regularMaterial, in: RoundedRectangle(cornerRadius: 12, style: .continuous))
    }
}
