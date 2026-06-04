import Foundation

/// Typed composition root for the macOS app's local backend services.
struct AppServices {
    let runtimeStore: NativeRuntimeStore
    let configStore: ConfigStore
    let codexRunner: CodexRunner
    let knowledgeStore: KnowledgeStore
    let webexClient: WebexAPIClient
    let webexIngestionService: NativeWebexIngestionService
    let codexOrchestrationService: CodexPromptOrchestrationService
    let questionService: QuestionCandidateService
    let oauthService: OAuthService

    var configuration: RuntimeConfiguration {
        runtimeStore.configuration
    }

    /// Builds the shared services for one runtime root.
    init(
        runtimeStore: NativeRuntimeStore = NativeRuntimeStore(),
        configStore: ConfigStore? = nil,
        codexRunner: CodexRunner? = nil,
        knowledgeStore: KnowledgeStore? = nil,
        webexClient: WebexAPIClient? = nil,
        iMessageService: NativeIMessageIngesting? = nil
    ) {
        let configuration = runtimeStore.configuration
        let resolvedConfigStore = configStore ?? ConfigStore(configuration: configuration)
        let resolvedCodexRunner = codexRunner ?? CodexRunner(configuration: configuration)
        let resolvedKnowledgeStore = knowledgeStore ?? KnowledgeStore(configuration: configuration)
        let resolvedWebexClient = webexClient ?? WebexAPIClient(
            configuration: configuration,
            configStore: resolvedConfigStore
        )
        let resolvedWebexIngestionService = NativeWebexIngestionService(
            configuration: configuration,
            configStore: resolvedConfigStore,
            webexClient: resolvedWebexClient,
            knowledgeStore: resolvedKnowledgeStore,
            iMessageService: iMessageService
        )
        let resolvedCodexOrchestrationService = CodexPromptOrchestrationService(
            configuration: configuration,
            runner: resolvedCodexRunner,
            configStore: resolvedConfigStore
        )
        let resolvedQuestionService = QuestionCandidateService(
            knowledgeStore: resolvedKnowledgeStore,
            questionSynthesizer: resolvedCodexOrchestrationService
        )

        self.runtimeStore = runtimeStore
        self.configStore = resolvedConfigStore
        self.codexRunner = resolvedCodexRunner
        self.knowledgeStore = resolvedKnowledgeStore
        self.webexClient = resolvedWebexClient
        self.webexIngestionService = resolvedWebexIngestionService
        self.codexOrchestrationService = resolvedCodexOrchestrationService
        self.questionService = resolvedQuestionService
        self.oauthService = OAuthService(
            configuration: configuration,
            configStore: resolvedConfigStore
        )
    }
}
