import SwiftUI

/// Primary navigation sidebar for app sections.
struct SidebarView: View {
    @EnvironmentObject private var model: AppModel

    var body: some View {
        List(selection: selection) {
            Section("Workspace") {
                sidebarRows([.home])
            }
            Section("Focus") {
                sidebarRows([.spaceFocus, .personFocus, .spaceFocusTargets, .personFocusTargets, .execFocusTargets])
            }
            Section("Intelligence") {
                sidebarRows([.questions, .transcription, .beliefs, .askCodex, .jobs])
            }
            Section("System") {
                sidebarRows([.settings])
            }
        }
        .listStyle(.sidebar)
        .navigationTitle("Webex Intel")
    }

    @ViewBuilder
    private func sidebarRows(_ sections: [AppSection]) -> some View {
        ForEach(sections) { section in
            Label(section.title, systemImage: section.symbolName)
                .tag(section)
        }
    }

    private var selection: Binding<AppSection?> {
        Binding<AppSection?>(
            get: { model.selectedSection },
            set: { newValue in
                if let newValue {
                    model.select(section: newValue)
                }
            }
        )
    }
}
