import SwiftUI

/// Landing dashboard that summarizes focus freshness and top items.
struct DashboardView: View {
    @EnvironmentObject private var model: AppModel

    private let columns = [
        GridItem(.adaptive(minimum: 160), spacing: 12)
    ]

    var body: some View {
        ScrollView {
            VStack(alignment: .leading, spacing: 20) {
                header
                LazyVGrid(columns: columns, spacing: 12) {
                    MetricCard(title: "Spaces", value: model.spaceCache.countLabel, detail: "\(model.spaceCache.recentMessages) recent messages", symbolName: "bubble.left.and.bubble.right")
                    MetricCard(title: "People", value: model.personCache.countLabel, detail: "\(model.personCache.recentMessages) recent messages", symbolName: "person.2")
                    MetricCard(
                        title: "Space Refresh",
                        value: model.isRefreshScopeRunning(.spaceFocus) ? "Running" : "Idle",
                        detail: model.refreshStatusDetail(for: .spaceFocus, fallback: progressText(model.spaceCache)),
                        symbolName: "arrow.triangle.2.circlepath"
                    )
                    MetricCard(
                        title: "Person Refresh",
                        value: model.isRefreshScopeRunning(.personFocus) ? "Running" : "Idle",
                        detail: model.refreshStatusDetail(for: .personFocus, fallback: progressText(model.personCache)),
                        symbolName: "clock"
                    )
                }

                SectionHeader(title: "Needs Attention", subtitle: "Most recent tracked spaces and people from the current cache snapshots.")

                VStack(spacing: 12) {
                    DashboardStrip(title: "Important Spaces", kind: .space, items: Array(model.spaceCache.items.prefix(5)))
                    DashboardStrip(title: "Important People", kind: .person, items: Array(model.personCache.items.prefix(5)))
                }
            }
            .padding(24)
        }
        .navigationTitle("Home")
    }

    private var header: some View {
        VStack(alignment: .leading, spacing: 8) {
            Text("Executive Intelligence")
                .font(.largeTitle.bold())
            Text("Native Swift port of the Webex/Codex runtime. It currently reads migration snapshots while ingestion, knowledge, clustering, and Codex execution move into Swift.")
                .font(.body)
                .foregroundStyle(.secondary)
        }
    }

    private func progressText(_ cache: FocusCache) -> String {
        guard cache.subjectsTotal > 0 else { return "No active refresh" }
        return "\(cache.subjectsProcessed)/\(cache.subjectsTotal) subjects"
    }
}

/// Horizontal preview strip for one focus kind.
private struct DashboardStrip: View {
    @EnvironmentObject private var model: AppModel
    let title: String
    let kind: FocusKind
    let items: [FocusItem]

    var body: some View {
        VStack(alignment: .leading, spacing: 10) {
            HStack {
                Text(title)
                    .font(.headline)
                Spacer()
                Button("Open") {
                    model.select(section: kind == .space ? .spaceFocus : .personFocus)
                }
                .buttonStyle(.borderless)
            }

            ForEach(items) { item in
                Button {
                    model.select(section: kind == .space ? .spaceFocus : .personFocus)
                    model.setSelectedItemID(item.id, for: kind)
                } label: {
                    HStack(alignment: .top, spacing: 12) {
                        Image(systemName: kind == .space ? "bubble.left.and.bubble.right" : "person.crop.circle")
                            .foregroundStyle(.tint)
                        VStack(alignment: .leading, spacing: 3) {
                            Text(item.title)
                                .font(.subheadline.weight(.semibold))
                                .foregroundStyle(.primary)
                            Text(item.subtitle.isEmpty ? item.meta : item.subtitle)
                                .lineLimit(2)
                                .font(.caption)
                                .foregroundStyle(.secondary)
                        }
                        Spacer()
                        Text(DisplayFormatters.latestMessageLabel(item.timestamp))
                            .font(.caption2)
                            .foregroundStyle(.secondary)
                    }
                    .contentShape(Rectangle())
                }
                .buttonStyle(.plain)
                Divider()
            }
        }
        .padding(16)
        .background(.regularMaterial, in: RoundedRectangle(cornerRadius: 14, style: .continuous))
    }
}

/// Reusable metric tile for overview and settings surfaces.
struct MetricCard: View {
    let title: String
    let value: String
    let detail: String
    let symbolName: String

    var body: some View {
        VStack(alignment: .leading, spacing: 12) {
            HStack {
                Image(systemName: symbolName)
                    .foregroundStyle(.tint)
                Spacer()
            }
            Text(value)
                .font(.title.bold())
            VStack(alignment: .leading, spacing: 3) {
                Text(title)
                    .font(.caption.weight(.semibold))
                    .textCase(.uppercase)
                    .foregroundStyle(.secondary)
                Text(detail)
                    .font(.caption)
                    .foregroundStyle(.secondary)
            }
        }
        .padding(16)
        .frame(maxWidth: .infinity, alignment: .leading)
        .background(.regularMaterial, in: RoundedRectangle(cornerRadius: 14, style: .continuous))
    }
}

/// Standard section title/subtitle row.
struct SectionHeader: View {
    let title: String
    let subtitle: String

    var body: some View {
        VStack(alignment: .leading, spacing: 4) {
            Text(title)
                .font(.title2.bold())
            Text(subtitle)
                .font(.callout)
                .foregroundStyle(.secondary)
        }
    }
}
