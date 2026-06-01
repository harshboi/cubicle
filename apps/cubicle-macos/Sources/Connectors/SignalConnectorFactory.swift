import Foundation

/// Construction root for signal connectors and source-specific product services.
final class SignalConnectorFactory {
    private let webexClient: WebexClienting
    private let webexProductClient: WebexProductClienting
    private let iMessageIngestionService: NativeIMessageIngesting
    private let webexAccountID: String
    private let iMessageAccountID: String

    /// Builds production connectors from runtime configuration.
    convenience init(configuration: RuntimeConfiguration = .current) {
        let webexAPIClient = WebexAPIClient(configuration: configuration)
        self.init(
            webexClient: webexAPIClient,
            webexProductClient: webexAPIClient,
            iMessageIngestionService: NativeIMessageIngestionService(),
            webexAccountID: "default",
            iMessageAccountID: "local"
        )
    }

    /// Shares one Webex client across sync and product-service surfaces.
    convenience init(
        webexClient: WebexClienting & WebexProductClienting,
        iMessageIngestionService: NativeIMessageIngesting,
        webexAccountID: String = "default",
        iMessageAccountID: String = "local"
    ) {
        self.init(
            webexClient: webexClient,
            webexProductClient: webexClient,
            iMessageIngestionService: iMessageIngestionService,
            webexAccountID: webexAccountID,
            iMessageAccountID: iMessageAccountID
        )
    }

    /// Allows tests to split Webex sync and product clients when needed.
    init(
        webexClient: WebexClienting,
        webexProductClient: WebexProductClienting,
        iMessageIngestionService: NativeIMessageIngesting,
        webexAccountID: String = "default",
        iMessageAccountID: String = "local"
    ) {
        self.webexClient = webexClient
        self.webexProductClient = webexProductClient
        self.iMessageIngestionService = iMessageIngestionService
        self.webexAccountID = webexAccountID
        self.iMessageAccountID = iMessageAccountID
    }

    /// Creates one signal connector by stable source ID.
    func makeSignalConnector(id: ConnectorID) throws -> SignalConnector {
        switch id {
        case .webex:
            return WebexSignalConnector(webexClient: webexClient, accountID: webexAccountID)
        case .iMessage:
            return IMessageSignalConnector(
                ingestionService: iMessageIngestionService,
                accountID: iMessageAccountID
            )
        default:
            throw SignalConnectorFactoryError.unsupportedConnector(id)
        }
    }

    /// Creates connectors in caller-specified order.
    func makeSignalConnectors(ids: [ConnectorID]) throws -> [SignalConnector] {
        try ids.map(makeSignalConnector)
    }

    /// Creates the Webex-only surface for rooms, memberships, and settings flows.
    func makeWebexProductService() -> WebexProductService {
        WebexProductService(client: webexProductClient)
    }

    /// Creates the iMessage-only surface for local handle/chat workflows.
    func makeIMessageProductService() -> IMessageProductService {
        IMessageProductService(ingestionService: iMessageIngestionService)
    }
}

enum SignalConnectorFactoryError: LocalizedError {
    case unsupportedConnector(ConnectorID)

    var errorDescription: String? {
        switch self {
        case .unsupportedConnector(let connectorID):
            return "Unsupported signal connector: \(connectorID.rawValue)"
        }
    }
}
