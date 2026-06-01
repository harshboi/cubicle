import SwiftUI

/// Scheduler/job overview for manual refresh controls and current run states.
struct JobsView: View {
    @EnvironmentObject private var model: AppModel

    var body: some View {
        ScrollView {
            VStack(alignment: .leading, spacing: 14) {
                SectionHeader(
                    title: "Runtime Jobs",
                    subtitle: model.backgroundRefreshActive
                        ? "Background refresh is active. Manual reload runs the same pipeline immediately."
                        : "Background refresh has not started yet."
                )
                Button {
                    Task { await model.refreshNow() }
                } label: {
                    Label("Run Full Refresh Now", systemImage: "arrow.triangle.2.circlepath")
                }
                .buttonStyle(.borderedProminent)

                ForEach(model.refreshPlans, id: \.scope) { plan in
                    RefreshStatusCard(
                        plan: plan,
                        status: model.refreshStatuses[plan.scope] ?? RefreshRunStatus(scope: plan.scope, isRunning: false)
                    )
                }
            }
            .padding(24)
            .frame(maxWidth: .infinity, alignment: .leading)
        }
        .navigationTitle("Jobs")
    }
}

/// Status card for one refresh plan.
private struct RefreshStatusCard: View {
    let plan: RefreshPlan
    let status: RefreshRunStatus

    var body: some View {
        VStack(alignment: .leading, spacing: 10) {
            HStack {
                Text(plan.scope.displayTitle)
                    .font(.headline)
                Spacer()
                Badge(text: status.isRunning ? "running" : "idle", color: status.isRunning ? .blue : .secondary)
                Badge(text: "\(plan.cadenceSeconds)s", color: .secondary)
            }
            Text(plan.description)
                .font(.caption)
                .foregroundStyle(.secondary)
            if let started = status.lastStartedAt {
                Text("Last started: \(DisplayFormatters.localDateTime(started))")
                    .font(.caption.monospaced())
                    .foregroundStyle(.secondary)
            }
            if let completed = status.lastCompletedAt {
                Text("Last completed: \(DisplayFormatters.localDateTime(completed))")
                    .font(.caption.monospaced())
                    .foregroundStyle(.secondary)
            }
            if let summary = status.lastSummary, !summary.isEmpty {
                Text(summary)
                    .textSelection(.enabled)
            }
            if let error = status.lastError, !error.isEmpty {
                Text(error)
                    .foregroundStyle(.red)
                    .textSelection(.enabled)
            }
        }
        .padding(14)
        .background(.regularMaterial, in: RoundedRectangle(cornerRadius: 14, style: .continuous))
    }
}

/// Detail inspector for the selected refresh plan.
struct JobsInspectorView: View {
    @EnvironmentObject private var model: AppModel

    var body: some View {
        ScrollView {
            VStack(alignment: .leading, spacing: 16) {
                SectionHeader(
                    title: "Jobs Inspector",
                    subtitle: "Status for the background refresh pipeline. These jobs keep Webex data, focus caches, Codex artifacts, and beliefs current."
                )

                HStack(spacing: 12) {
                    JobMetricCard(
                        title: "Pipeline",
                        value: model.backgroundRefreshActive ? "Active" : "Stopped",
                        symbolName: "arrow.triangle.2.circlepath",
                        color: model.backgroundRefreshActive ? .blue : .secondary
                    )
                    JobMetricCard(
                        title: "Running",
                        value: "\(runningCount)",
                        symbolName: "clock.arrow.circlepath",
                        color: runningCount > 0 ? .blue : .secondary
                    )
                    JobMetricCard(
                        title: "Failed",
                        value: "\(failedCount)",
                        symbolName: "exclamationmark.triangle",
                        color: failedCount > 0 ? .red : .secondary
                    )
                }

                VStack(alignment: .leading, spacing: 10) {
                    ForEach(model.refreshPlans, id: \.scope) { plan in
                        JobInspectorRow(
                            plan: plan,
                            status: model.refreshStatuses[plan.scope] ?? RefreshRunStatus(scope: plan.scope, isRunning: false)
                        )
                    }
                }
            }
            .padding(24)
            .frame(maxWidth: .infinity, alignment: .topLeading)
        }
    }

    private var runningCount: Int {
        model.refreshStatuses.values.filter(\.isRunning).count
    }

    private var failedCount: Int {
        model.refreshStatuses.values.filter { status in
            guard let error = status.lastError else {
                return false
            }
            return !error.trimmingCharacters(in: .whitespacesAndNewlines).isEmpty
        }.count
    }
}

/// Metric tile used inside the job inspector.
private struct JobMetricCard: View {
    let title: String
    let value: String
    let symbolName: String
    let color: Color

    var body: some View {
        VStack(alignment: .leading, spacing: 8) {
            Image(systemName: symbolName)
                .foregroundStyle(color)
            Text(value)
                .font(.title2.bold())
            Text(title.uppercased())
                .font(.caption.weight(.semibold))
                .foregroundStyle(.secondary)
        }
        .frame(maxWidth: .infinity, alignment: .leading)
        .padding(14)
        .background(.regularMaterial, in: RoundedRectangle(cornerRadius: 14, style: .continuous))
    }
}

/// Row summary for one refresh plan in the inspector list.
private struct JobInspectorRow: View {
    let plan: RefreshPlan
    let status: RefreshRunStatus

    var body: some View {
        VStack(alignment: .leading, spacing: 10) {
            HStack(alignment: .firstTextBaseline, spacing: 10) {
                Label(plan.scope.displayTitle, systemImage: plan.scope.symbolName)
                    .font(.headline)
                Spacer()
                Badge(text: status.isRunning ? "running" : "idle", color: status.isRunning ? .blue : .secondary)
                Badge(text: "every \(plan.cadenceSeconds)s", color: .secondary)
            }

            Text(plan.description)
                .font(.body)
                .foregroundStyle(.secondary)

            Grid(alignment: .leading, horizontalSpacing: 18, verticalSpacing: 8) {
                GridRow {
                    Text("Last started")
                        .font(.caption.weight(.semibold))
                        .foregroundStyle(.secondary)
                    Text(readableDate(status.lastStartedAt))
                        .textSelection(.enabled)
                }
                GridRow {
                    Text("Last completed")
                        .font(.caption.weight(.semibold))
                        .foregroundStyle(.secondary)
                    Text(readableDate(status.lastCompletedAt))
                        .textSelection(.enabled)
                }
                if let summary = status.lastSummary,
                   !summary.trimmingCharacters(in: .whitespacesAndNewlines).isEmpty {
                    GridRow {
                        Text("Last result")
                            .font(.caption.weight(.semibold))
                            .foregroundStyle(.secondary)
                        Text(summary)
                            .textSelection(.enabled)
                    }
                }
                if let error = status.lastError,
                   !error.trimmingCharacters(in: .whitespacesAndNewlines).isEmpty {
                    GridRow {
                        Text("Last error")
                            .font(.caption.weight(.semibold))
                            .foregroundStyle(.red)
                        Text(error)
                            .foregroundStyle(.red)
                            .textSelection(.enabled)
                    }
                }
            }
            .font(.body)
        }
        .padding(16)
        .background(.regularMaterial, in: RoundedRectangle(cornerRadius: 14, style: .continuous))
    }

    private func readableDate(_ rawValue: String?) -> String {
        guard let rawValue,
              !rawValue.trimmingCharacters(in: .whitespacesAndNewlines).isEmpty else {
            return "Not run yet"
        }
        return DisplayFormatters.localDateTime(rawValue)
    }
}

/// Presentation metadata for refresh scopes.
private extension RefreshScope {
    var displayTitle: String {
        switch self {
        case .webexSync:
            return "Webex Sync"
        case .beliefMaintenance:
            return "Belief Maintenance"
        case .personFocus:
            return "Person Focus"
        case .spaceFocus:
            return "Space Focus + Codex"
        case .questions:
            return "Questions"
        case .codexJobs:
            return "Codex Jobs"
        }
    }

    var symbolName: String {
        switch self {
        case .webexSync:
            return "cloud"
        case .beliefMaintenance:
            return "brain.head.profile"
        case .personFocus:
            return "person.2"
        case .spaceFocus:
            return "bubble.left.and.bubble.right"
        case .questions:
            return "questionmark.bubble"
        case .codexJobs:
            return "sparkles"
        }
    }
}
