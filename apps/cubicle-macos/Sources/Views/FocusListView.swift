import SwiftUI

struct FocusListView: View {
    @EnvironmentObject private var model: AppModel
    let kind: FocusKind

    var body: some View {
        VStack(spacing: 0) {
            HStack(spacing: 10) {
                Label("Sort", systemImage: "arrow.up.arrow.down")
                    .font(.caption.weight(.semibold))
                    .foregroundStyle(.secondary)
                Picker("Sort", selection: sortOption) {
                    Text("Date/Time").tag(FocusSortOption.latestMessage)
                    Text("Name").tag(FocusSortOption.name)
                }
                .pickerStyle(.segmented)
                .frame(maxWidth: 280)
                Spacer()
            }
            .padding(.horizontal, 10)
            .padding(.top, 8)
            .padding(.bottom, 6)
            .background(.bar)

            List(selection: selectedItem) {
                ForEach(filteredItems) { item in
                    FocusRow(item: item, kind: kind)
                        .tag(item.id)
                }
            }
        }
        .searchable(text: searchText, placement: .toolbar, prompt: "Search \(kind.title)")
        .navigationTitle(kind.title)
        .overlay {
            if filteredItems.isEmpty {
                emptyState
            }
        }
        .safeAreaInset(edge: .bottom) {
            cacheFooter
        }
        .onAppear {
            model.activateFocus(kind)
        }
    }

    private var filteredItems: [FocusItem] {
        model.filteredItems(for: kind)
    }

    private var searchText: Binding<String> {
        Binding<String>(
            get: { model.searchText(for: kind) },
            set: { model.setSearchText($0, for: kind) }
        )
    }

    private var selectedItem: Binding<String?> {
        Binding<String?>(
            get: { model.selectedItemID(for: kind) },
            set: { model.setSelectedItemID($0, for: kind) }
        )
    }

    private var sortOption: Binding<FocusSortOption> {
        Binding<FocusSortOption>(
            get: { model.sortOption(for: kind) },
            set: { model.setSortOption($0, for: kind) }
        )
    }

    @ViewBuilder
    private var emptyState: some View {
        if model.isLoading {
            ProgressView("Loading \(kind.title)")
                .controlSize(.small)
                .padding(20)
                .background(.regularMaterial, in: RoundedRectangle(cornerRadius: 12, style: .continuous))
        } else if model.hasSearchQuery(for: kind) {
            EmptyStateCard(
                title: "No Matches",
                message: "Try a broader query for \(kind.title).",
                symbolName: "magnifyingglass"
            )
        } else {
            EmptyStateCard(
                title: "\(kind.title) Empty",
                message: "No runtime snapshot rows are available yet.",
                symbolName: kind == .space ? "tray.full" : "person.crop.circle.badge.exclamationmark"
            )
        }
    }

    private var cacheFooter: some View {
        let cache = model.cache(for: kind)
        return HStack {
            Label(
                cache.updatedAt.isEmpty ? "No snapshot loaded" : DisplayFormatters.snapshotUpdatedLabel(cache.updatedAt),
                systemImage: "clock"
            )
                .font(.caption)
                .foregroundStyle(.secondary)
            Spacer()
            if let refreshText = model.focusRefreshStatusText(for: kind) {
                Label(refreshText, systemImage: "arrow.triangle.2.circlepath")
                    .font(.caption)
                    .foregroundStyle(.blue)
            }
        }
        .padding(10)
        .background(.bar)
    }
}

private struct EmptyStateCard: View {
    let title: String
    let message: String
    let symbolName: String

    var body: some View {
        VStack(spacing: 10) {
            Image(systemName: symbolName)
                .font(.title2)
                .foregroundStyle(.secondary)
            Text(title)
                .font(.headline)
            Text(message)
                .font(.callout)
                .foregroundStyle(.secondary)
                .multilineTextAlignment(.center)
        }
        .padding(18)
        .frame(maxWidth: 320)
        .background(.regularMaterial, in: RoundedRectangle(cornerRadius: 14, style: .continuous))
    }
}

private struct FocusRow: View {
    let item: FocusItem
    let kind: FocusKind

    var body: some View {
        VStack(alignment: .leading, spacing: 8) {
            HStack(alignment: .top) {
                Image(systemName: kind == .space ? "bubble.left.and.bubble.right.fill" : "person.crop.circle.fill")
                    .foregroundStyle(kind == .space ? .blue : .green)
                    .frame(width: 22)
                VStack(alignment: .leading, spacing: 4) {
                    Text(item.title)
                        .font(.headline)
                        .lineLimit(1)
                    if !item.subtitle.isEmpty {
                        Text(item.subtitle)
                            .font(.subheadline)
                            .foregroundStyle(.secondary)
                            .lineLimit(2)
                    }
                    if !item.meta.isEmpty {
                        Text(item.meta)
                            .font(.caption)
                            .foregroundStyle(.secondary)
                            .lineLimit(1)
                    }
                }
                Spacer()
            }
            HStack(spacing: 6) {
                if !item.badge.isEmpty {
                    Badge(text: item.badge, color: .blue)
                }
                if !item.statusBadge.isEmpty {
                    Badge(text: item.statusBadge, color: statusColor(item.statusBadge))
                }
                Spacer()
                Text(DisplayFormatters.latestMessageLabel(item.timestamp))
                    .font(.caption2)
                    .foregroundStyle(.secondary)
            }
        }
        .padding(.vertical, 6)
    }

    private func statusColor(_ value: String) -> Color {
        let normalized = value.lowercased()
        if normalized.contains("closed") { return .green }
        if normalized.contains("failed") || normalized.contains("stale") { return .red }
        if normalized.contains("unclear") || normalized.contains("pending") { return .orange }
        return .secondary
    }
}

struct Badge: View {
    let text: String
    let color: Color

    var body: some View {
        Text(text)
            .font(.caption2.weight(.semibold))
            .padding(.horizontal, 7)
            .padding(.vertical, 3)
            .foregroundStyle(color)
            .background(color.opacity(0.12), in: Capsule())
    }
}
