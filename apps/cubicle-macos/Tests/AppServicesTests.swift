import XCTest
@testable import GetWebexSpaceMacApp

final class AppServicesTests: XCTestCase {
    @MainActor
    func testAppModelAndRefreshCoordinatorShareServiceGraph() {
        let harness = TestRuntimeHarness(label: "app-services-share")
        defer { harness.cleanup() }
        let services = AppServices(runtimeStore: harness.runtimeStore)

        let model = AppModel(
            services: services,
            transcriptionClient: MockTranscriptionClient(),
            audioCaptureService: NoopAudioCaptureService()
        )

        XCTAssertTrue(model.runtimeStore === services.runtimeStore)
        XCTAssertTrue(model.configStore === services.configStore)
        XCTAssertTrue(model.codexRunner === services.codexRunner)
        XCTAssertTrue(model.knowledgeStore === services.knowledgeStore)
        XCTAssertTrue(model.codexOrchestrationService === services.codexOrchestrationService)
        XCTAssertTrue(model.questionService === services.questionService)
        XCTAssertTrue(model.oauthService === services.oauthService)
        XCTAssertTrue(model.refreshCoordinator.configStore === services.configStore)
        XCTAssertTrue(model.refreshCoordinator.knowledgeStore === services.knowledgeStore)
        XCTAssertTrue(model.refreshCoordinator.webexIngestionService === services.webexIngestionService)
    }

    func testRefreshCoordinatorConvenienceInitializerUsesSharedServices() {
        let harness = TestRuntimeHarness(label: "refresh-services-share")
        defer { harness.cleanup() }
        let coordinator = NativeRefreshCoordinator(configuration: harness.configuration)

        XCTAssertEqual(coordinator.configuration.runtimeRoot, harness.runtimeRoot)
        XCTAssertTrue(coordinator.webexIngestionService.configStore === coordinator.configStore)
        XCTAssertTrue(coordinator.webexIngestionService.knowledgeStore === coordinator.knowledgeStore)
        XCTAssertTrue(coordinator.codexOrchestrationService.runner === coordinator.codexRunner)
    }
}
