import Foundation

/// Construction root for signal connectors and source-specific product services.
final class SignalConnectorFactory {
    private let registry: SignalConnectorRegistry

    /// Builds production connectors from runtime configuration.
    convenience init(configuration: RuntimeConfiguration = .current) {
        let webexAPIClient = WebexAPIClient(configuration: configuration)
        let knowledgeStore = KnowledgeStore(configuration: configuration)
        self.init(
            webexClient: webexAPIClient,
            webexProductClient: webexAPIClient,
            iMessageIngestionService: NativeIMessageIngestionService(),
            webexAccountID: "default",
            iMessageAccountID: "local",
            webexEngineConfiguration: Self.webexEngineConfiguration(from: configuration),
            webexExistingMessageLookup: { messageID in
                try knowledgeStore.messageExists(messageID: messageID)
            },
            webexLegacyStateLookup: { conversationID in
                try knowledgeStore.loadWebexSyncState(conversationID: conversationID)
            }
        )
    }

    /// Shares one Webex client across sync and product-service surfaces.
    convenience init(
        webexClient: WebexClienting & WebexProductClienting,
        iMessageIngestionService: NativeIMessageIngesting,
        webexAccountID: String = "default",
        iMessageAccountID: String = "local",
        webexEngineConfiguration: WebexSyncEngine.Configuration = WebexSyncEngine.Configuration(),
        webexExistingMessageLookup: @escaping (String) throws -> Bool = { _ in false },
        webexLegacyStateLookup: WebexSignalSyncStateStore.LegacyStateLookup? = nil
    ) {
        self.init(
            webexClient: webexClient,
            webexProductClient: webexClient,
            iMessageIngestionService: iMessageIngestionService,
            webexAccountID: webexAccountID,
            iMessageAccountID: iMessageAccountID,
            webexEngineConfiguration: webexEngineConfiguration,
            webexExistingMessageLookup: webexExistingMessageLookup,
            webexLegacyStateLookup: webexLegacyStateLookup
        )
    }

    /// Allows tests to split Webex sync and product clients when needed.
    init(
        webexClient: WebexClienting,
        webexProductClient: WebexProductClienting,
        iMessageIngestionService: NativeIMessageIngesting,
        webexAccountID: String = "default",
        iMessageAccountID: String = "local",
        webexEngineConfiguration: WebexSyncEngine.Configuration = WebexSyncEngine.Configuration(),
        webexExistingMessageLookup: @escaping (String) throws -> Bool = { _ in false },
        webexLegacyStateLookup: WebexSignalSyncStateStore.LegacyStateLookup? = nil
    ) {
        self.registry = try! SignalConnectorRegistry(
            providers: [
                WebexConnectorProvider(
                    webexClient: webexClient,
                    productClient: webexProductClient,
                    accountID: webexAccountID,
                    engineConfiguration: webexEngineConfiguration,
                    existingMessageLookup: webexExistingMessageLookup,
                    legacyStateLookup: webexLegacyStateLookup
                ),
                IMessageConnectorProvider(
                    ingestionService: iMessageIngestionService,
                    accountID: iMessageAccountID
                )
            ]
        )
    }

    /// Uses caller-supplied providers for tests and future connector packs.
    init(registry: SignalConnectorRegistry) {
        self.registry = registry
    }

    /// Creates one signal connector by stable source ID.
    func makeSignalConnector(id: ConnectorID) throws -> SignalConnector {
        try registry.provider(for: id).makeSignalConnector()
    }

    /// Creates connectors in caller-specified order.
    func makeSignalConnectors(ids: [ConnectorID]) throws -> [SignalConnector] {
        try ids.map(makeSignalConnector)
    }

    /// Creates the Webex-only surface for rooms, memberships, and settings flows.
    func makeWebexProductService() throws -> WebexProductService {
        guard let provider = try registry.provider(for: .webex) as? WebexProductServiceProviding else {
            throw SignalConnectorFactoryError.unsupportedProductService(.webex)
        }
        return provider.makeWebexProductService()
    }

    /// Creates the iMessage-only surface for local handle/chat workflows.
    func makeIMessageProductService() throws -> IMessageProductService {
        guard let provider = try registry.provider(for: .iMessage) as? IMessageProductServiceProviding else {
            throw SignalConnectorFactoryError.unsupportedProductService(.iMessage)
        }
        return provider.makeIMessageProductService()
    }

    static func webexEngineConfiguration(
        from configuration: RuntimeConfiguration
    ) -> WebexSyncEngine.Configuration {
        WebexSyncEngine.Configuration(
            maxConcurrentAPIRequests: max(1, configuration.webexSyncConcurrencyLimit),
            activeIntervalSeconds: max(5, configuration.webexAdaptiveActiveIntervalSeconds),
            recentIntervalSeconds: max(15, configuration.webexAdaptiveRecentIntervalSeconds),
            backgroundIntervalSeconds: max(60, configuration.webexAdaptiveBackgroundIntervalSeconds),
            jitterRatio: min(max(0, configuration.webexAdaptiveJitterRatio), 0.8)
        )
    }
}

enum SignalConnectorFactoryError: LocalizedError {
    case unsupportedConnector(ConnectorID)
    case duplicateConnectorProvider(ConnectorID)
    case unsupportedProductService(ConnectorID)

    var errorDescription: String? {
        switch self {
        case .unsupportedConnector(let connectorID):
            return "Unsupported signal connector: \(connectorID.rawValue)"
        case .duplicateConnectorProvider(let connectorID):
            return "Duplicate signal connector provider: \(connectorID.rawValue)"
        case .unsupportedProductService(let connectorID):
            return "Unsupported connector product service: \(connectorID.rawValue)"
        }
    }
}
