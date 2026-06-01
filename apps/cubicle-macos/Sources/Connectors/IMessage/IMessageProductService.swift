import Foundation

/// iMessage-only operations for local handle validation and preview flows.
final class IMessageProductService {
    private let ingestionService: NativeIMessageIngesting

    /// Injects the local DB reader so tests never need TCC access.
    init(ingestionService: NativeIMessageIngesting = NativeIMessageIngestionService()) {
        self.ingestionService = ingestionService
    }

    /// Returns the canonical handle value used in config and DB matching.
    func normalizedHandle(_ value: String) -> String? {
        IMessageHandleNormalizer.normalizedStorageValue(value)
    }

    /// Loads a bounded message preview for source-specific target setup.
    func previewMessages(
        matching handles: [String],
        displayName: String,
        since: Date,
        limit: Int
    ) throws -> [IMessageTimelineMessage] {
        let normalizedHandles = handles.compactMap(normalizedHandle)
        return try ingestionService.loadMessages(
            matching: normalizedHandles,
            displayName: displayName,
            since: since,
            limit: limit
        )
    }
}
