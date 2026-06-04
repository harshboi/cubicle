import Foundation

/// Provider boundary for one connector family.
protocol SignalConnectorProvider {
    var connectorID: ConnectorID { get }

    func makeSignalConnector() throws -> SignalConnector
}

/// Provider capability for Webex product-specific UI/services.
protocol WebexProductServiceProviding {
    func makeWebexProductService() -> WebexProductService
}

/// Provider capability for iMessage product-specific UI/services.
protocol IMessageProductServiceProviding {
    func makeIMessageProductService() -> IMessageProductService
}

/// Connector provider lookup keyed by stable connector ID.
final class SignalConnectorRegistry {
    private var providersByID: [ConnectorID: SignalConnectorProvider]

    init(providers: [SignalConnectorProvider]) throws {
        var indexed: [ConnectorID: SignalConnectorProvider] = [:]
        for provider in providers {
            guard indexed[provider.connectorID] == nil else {
                throw SignalConnectorFactoryError.duplicateConnectorProvider(provider.connectorID)
            }
            indexed[provider.connectorID] = provider
        }
        self.providersByID = indexed
    }

    func provider(for connectorID: ConnectorID) throws -> SignalConnectorProvider {
        guard let provider = providersByID[connectorID] else {
            throw SignalConnectorFactoryError.unsupportedConnector(connectorID)
        }
        return provider
    }
}

/// Webex connector provider for sync and product-specific surfaces.
final class WebexConnectorProvider: SignalConnectorProvider, WebexProductServiceProviding {
    let connectorID = ConnectorID.webex

    private let webexClient: WebexClienting
    private let productClient: WebexProductClienting
    private let accountID: String
    private let engineConfiguration: WebexSyncEngine.Configuration
    private let selfIdentityLookup: WebexSignalConnector.SelfIdentityLookup?
    private let existingMessageLookup: (String) throws -> Bool
    private let legacyStateLookup: WebexSignalSyncStateStore.LegacyStateLookup?

    init(
        webexClient: WebexClienting,
        productClient: WebexProductClienting,
        accountID: String = "default",
        engineConfiguration: WebexSyncEngine.Configuration = WebexSyncEngine.Configuration(),
        selfIdentityLookup: WebexSignalConnector.SelfIdentityLookup? = nil,
        existingMessageLookup: @escaping (String) throws -> Bool = { _ in false },
        legacyStateLookup: WebexSignalSyncStateStore.LegacyStateLookup? = nil
    ) {
        self.webexClient = webexClient
        self.productClient = productClient
        self.accountID = accountID
        self.engineConfiguration = engineConfiguration
        self.selfIdentityLookup = selfIdentityLookup
        self.existingMessageLookup = existingMessageLookup
        self.legacyStateLookup = legacyStateLookup
    }

    func makeSignalConnector() throws -> SignalConnector {
        WebexSignalConnector(
            webexClient: webexClient,
            accountID: accountID,
            engineConfiguration: engineConfiguration,
            selfIdentityLookup: selfIdentityLookup,
            existingMessageLookup: existingMessageLookup,
            legacyStateLookup: legacyStateLookup
        )
    }

    func makeWebexProductService() -> WebexProductService {
        WebexProductService(client: productClient)
    }
}

/// iMessage connector provider for sync and product-specific surfaces.
final class IMessageConnectorProvider: SignalConnectorProvider, IMessageProductServiceProviding {
    let connectorID = ConnectorID.iMessage

    private let ingestionService: NativeIMessageIngesting
    private let accountID: String

    init(
        ingestionService: NativeIMessageIngesting,
        accountID: String = "local"
    ) {
        self.ingestionService = ingestionService
        self.accountID = accountID
    }

    func makeSignalConnector() throws -> SignalConnector {
        IMessageSignalConnector(
            ingestionService: ingestionService,
            accountID: accountID
        )
    }

    func makeIMessageProductService() -> IMessageProductService {
        IMessageProductService(ingestionService: ingestionService)
    }
}
