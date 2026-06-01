import Darwin
import Foundation
import SwiftUI

/// SwiftUI app entry point plus hidden CLI refresh entry points.
@main
struct GetWebexSpaceMacApp: App {
    @StateObject private var model = AppModel()

    /// Handles runtime-only CLI commands before SwiftUI launches.
    init() {
        RuntimeCommandLine.runAndExitIfRequested()
    }

    /// Main app window and command menu wiring.
    var body: some Scene {
        WindowGroup {
            RootView()
                .environmentObject(model)
                .task {
                    await model.startProgram()
                }
                .onReceive(NotificationCenter.default.publisher(for: NSApplication.willTerminateNotification)) { _ in
                    model.handleAppWillTerminate()
                }
        }
        .commands {
            CommandGroup(after: .appInfo) {
                Button("Run Runtime Refresh") {
                    Task { await model.refreshNow() }
                }
                .keyboardShortcut("r", modifiers: [.command])
            }
        }
    }
}

/// Bridges packaged app code to scriptable refresh commands.
private enum RuntimeCommandLine {
    /// Runs the requested refresh command and exits without opening the UI.
    static func runAndExitIfRequested() {
        let arguments = Set(CommandLine.arguments.dropFirst())
        guard arguments.contains("--refresh-space-focus-cache")
            || arguments.contains("--refresh-person-focus-cache")
            || arguments.contains("--refresh-space-focus-with-codex")
            || arguments.contains("--refresh-person-focus-with-codex")
            || arguments.contains("--sync-webex-now") else {
            return
        }

        do {
            let store = NativeRuntimeStore()
            if arguments.contains("--refresh-space-focus-cache") {
                let outcome = try store.refreshSpaceFocusCache(forceRebuild: true)
                print(summary(label: "space", outcome: outcome))
            }
            if arguments.contains("--refresh-person-focus-cache") {
                let outcome = try store.refreshPersonFocusCache(forceRebuild: true)
                print(summary(label: "person", outcome: outcome))
            }
            if arguments.contains("--refresh-space-focus-with-codex") {
                let result = try runAsyncRefresh(scope: .spaceFocus, mode: .full)
                print(result.summary)
            }
            if arguments.contains("--refresh-person-focus-with-codex") {
                let result = try runAsyncRefresh(scope: .personFocus, mode: .full)
                print(result.summary)
            }
            if arguments.contains("--sync-webex-now") {
                let result = try runAsyncRefresh(scope: .webexSync, mode: .full)
                print(result.summary)
            }
            exit(0)
        } catch {
            FileHandle.standardError.write(Data("Cubicle runtime command failed: \(error)\n".utf8))
            exit(1)
        }
    }

    /// Compact stdout line used by shell-triggered focus-cache refreshes.
    private static func summary(label: String, outcome: FocusRefreshOutcome) -> String {
        "\(label) focus cache: focusDays=\(outcome.focusDays), reused=\(outcome.reusedCache), events=\(outcome.normalizedEventCount), clusters=\(outcome.clusterCount), output=\(outcome.outputSnapshotURL.path)"
    }

    /// Runs async refresh code from the synchronous CLI startup path.
    private static func runAsyncRefresh(
        scope: RefreshScope,
        mode: RefreshExecutionMode
    ) throws -> RefreshExecutionResult {
        let semaphore = DispatchSemaphore(value: 0)
        var capturedResult: Result<RefreshExecutionResult, Error>?
        Task {
            do {
                let result = try await NativeRefreshCoordinator().refresh(scope, mode: mode)
                capturedResult = .success(result)
            } catch {
                capturedResult = .failure(error)
            }
            semaphore.signal()
        }
        semaphore.wait()
        return try capturedResult!.get()
    }
}
