import Foundation
@testable import GetWebexSpaceMacApp

final class TestRuntimeHarness {
    let runtimeRoot: URL
    let configuration: RuntimeConfiguration
    let configStore: ConfigStore
    let runtimeStore: NativeRuntimeStore
    let knowledgeStore: KnowledgeStore

    init(label: String, codexExecutable: String = "codex") {
        self.runtimeRoot = temporaryRuntimeRoot(label: label)
        self.configuration = testRuntimeConfiguration(
            runtimeRoot: runtimeRoot,
            codexExecutable: codexExecutable
        )
        self.configStore = ConfigStore(configuration: configuration)
        self.runtimeStore = NativeRuntimeStore(configuration: configuration)
        self.knowledgeStore = KnowledgeStore(configuration: configuration)
    }

    func cleanup() {
        try? FileManager.default.removeItem(at: runtimeRoot)
    }

    deinit {
        cleanup()
    }
}

func testRuntimeConfiguration(
    runtimeRoot: URL,
    codexExecutable: String = "codex",
    webexBaseURL: URL = URL(string: "https://webexapis.com/v1")!
) -> RuntimeConfiguration {
    RuntimeConfiguration(
        runtimeRoot: runtimeRoot,
        codexExecutable: codexExecutable,
        webexBaseURL: webexBaseURL,
        webexPageSize: 100,
        webexRetryCount: 0,
        webexTimeoutSeconds: 1,
        webexOAuthTokenPathOverride: nil,
        webexOAuthRefreshSkewSeconds: 300,
        webexOAuthRefreshTokenSkewSeconds: 86_400
    )
}

func testConfiguration(runtimeRoot: URL) -> RuntimeConfiguration {
    testRuntimeConfiguration(runtimeRoot: runtimeRoot)
}

func temporaryRuntimeRoot(label: String) -> URL {
    FileManager.default.temporaryDirectory.appendingPathComponent(
        "Cubicle-\(label)-\(UUID().uuidString)",
        isDirectory: true
    )
}
