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

    func testAppServicesBuildsRefreshCoordinatorFromSharedGraph() {
        let harness = TestRuntimeHarness(label: "app-services-coordinator")
        defer { harness.cleanup() }
        let services = AppServices(runtimeStore: harness.runtimeStore)

        let coordinator = services.makeRefreshCoordinator()

        XCTAssertEqual(coordinator.configuration.runtimeRoot, harness.runtimeRoot)
        XCTAssertTrue(coordinator.runtimeStore === services.runtimeStore)
        XCTAssertTrue(coordinator.configStore === services.configStore)
        XCTAssertTrue(coordinator.webexClient === services.webexClient)
        XCTAssertTrue(coordinator.knowledgeStore === services.knowledgeStore)
        XCTAssertTrue(coordinator.codexRunner === services.codexRunner)
        XCTAssertTrue(coordinator.webexIngestionService === services.webexIngestionService)
        XCTAssertTrue(coordinator.codexOrchestrationService === services.codexOrchestrationService)
        XCTAssertTrue(coordinator.questionService === services.questionService)
    }

    func testRefreshCoordinatorConvenienceInitializerUsesSharedServices() throws {
        let harness = TestRuntimeHarness(label: "refresh-services-share")
        defer { harness.cleanup() }
        let coordinator = NativeRefreshCoordinator(configuration: harness.configuration)
        let webexIngestionService = try XCTUnwrap(
            coordinator.webexIngestionService as? NativeWebexIngestionService
        )

        XCTAssertEqual(coordinator.configuration.runtimeRoot, harness.runtimeRoot)
        XCTAssertTrue(webexIngestionService.configStore === coordinator.configStore)
        XCTAssertTrue(webexIngestionService.knowledgeStore === coordinator.knowledgeStore)
        XCTAssertTrue(coordinator.codexOrchestrationService.runner === coordinator.codexRunner)
    }
}
