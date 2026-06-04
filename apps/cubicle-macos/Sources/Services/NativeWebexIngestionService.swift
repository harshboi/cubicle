import Foundation

/// Per-room sync result used for progress, summaries, and live snapshot badges.
struct WebexRoomSyncResult: Hashable {
    var roomID: String
    var title: String
    var targetKinds: [String]
    var fetchedMessages: Int
    var indexedMessages: Int
    var indexedEvidence: Int
    var memberCount: Int
    var latestMessageID: String?
    var latestMessageCreated: String?
    var skippedReason: String?
    var unchanged: Bool = false
}

/// Aggregate result for a Webex target sync run.
struct WebexSyncOutcome: Hashable {
    var startedAt: String
    var completedAt: String
    var requestedTargets: Int
    var syncedRooms: Int
    var fetchedMessages: Int
    var indexedMessages: Int
    var indexedEvidence: Int
    var skippedTargets: Int
    var unchangedRooms: Int
    var roomResults: [WebexRoomSyncResult]
    var summaryOverride: String? = nil

    var summary: String {
        summaryOverride ?? "Webex sync indexed \(indexedMessages) message(s), \(indexedEvidence) evidence row(s), \(syncedRooms) changed room(s), \(unchangedRooms) unchanged room(s), skipped \(skippedTargets) target(s)."
    }
}

/// Webex sync breadth selected by refresh orchestration.
enum WebexSyncMode: Hashable {
    case full
    case incremental
}

/// Result for regenerating the Webex target lookup map.
struct WebexMapRefreshOutcome: Hashable {
    var mapFileURL: URL
    var rooms: Int
    var spaces: Int
    var senders: Int
    var summaryOverride: String? = nil

    var summary: String {
        summaryOverride ?? "\(mapFileURL.lastPathComponent) refreshed: rooms=\(rooms), spaces=\(spaces), senders=\(senders)."
    }
}

/// Result for rebuilding native focus snapshots from stored connector evidence.
struct WebexFocusSnapshotRefreshOutcome: Hashable {
    var completedAt: String
    var spaceTargets: Int
    var personTargets: Int
    var supplementalRoomTargets: Int

    var summary: String {
        "Focus snapshots refreshed: spaces=\(spaceTargets), people=\(personTargets), supplemental=\(supplementalRoomTargets)."
    }
}

/// Webex ingestion boundary used by refresh orchestration tests.
protocol NativeWebexIngesting: AnyObject {
    func refreshMapFile() async throws -> WebexMapRefreshOutcome
    func refreshFocusSnapshotsFromKnowledgeStore() throws -> WebexFocusSnapshotRefreshOutcome
    func syncTrackedTargets(
        messageLimitPerRoom: Int,
        mode: WebexSyncMode,
        trigger: WebexSyncTriggerReason,
        progress: ((WebexSyncProgress) async -> Void)?
    ) async throws -> WebexSyncOutcome
}

/// Progress event emitted while tracked conversations are syncing.
struct WebexSyncProgress: Hashable {
    var completedRooms: Int
    var totalRooms: Int
    var fetchedMessages: Int
    var indexedMessages: Int
    var currentRoomTitle: String?
    var unchangedRooms: Int = 0
    var skippedRooms: Int = 0

    var summary: String {
        let roomText = totalRooms == 0 ? "0 pending rooms" : "\(completedRooms)/\(totalRooms) rooms"
        let messageText = "\(indexedMessages) indexed"
        let unchangedText = unchangedRooms > 0 ? ", \(unchangedRooms) unchanged" : ""
        let skippedText = skippedRooms > 0 ? ", \(skippedRooms) skipped" : ""
        if let currentRoomTitle, !currentRoomTitle.isEmpty {
            return "Webex sync: \(roomText), \(messageText)\(unchangedText)\(skippedText). Now: \(currentRoomTitle)"
        }
        return "Webex sync: \(roomText), \(messageText)\(unchangedText)\(skippedText)."
    }
}

/// Resolved room target with all config roles that point at that room.
private struct TrackedRoomTarget {
    var roomID: String
    var label: String
    var kinds: Set<ConfigTarget.Kind>
}

/// Tab-separated map row written for target picking and config bootstrap.
private struct WebexMapEntry: Hashable {
    var kind: String
    var label: String
    var roomID: String
    var roomType: String
    var personEmail: String = ""
    var personDisplayName: String = ""
}

/// Optional room-list snapshot used to skip unchanged incremental syncs.
private struct TrackedRoomActivityProbe {
    var roomsByID: [String: WebexRoom]
    var usedLiveRoomList: Bool
}

/// Legacy room-based sync plan retained for direct room refresh helpers.
private struct RoomSyncPlan {
    var roomsToSync: [TrackedRoomTarget]
    var unchangedResults: [WebexRoomSyncResult]
}

/// Conversation sync plan after backoff and polling-mode gates.
private struct ConversationSyncPlan {
    var conversationsToSync: [WebexTrackedConversation]
    var precomputedResults: [WebexConversationSyncResult]
}

/// Unified person timeline event across Webex and iMessage.
private struct PersonTimelineEvent {
    var id: String
    var source: String
    var createdAt: String
    var sortDate: Date?
    var sender: String
    var sourceTitle: String
    var body: String
}

/// Ingestion failures that are local to Webex sync orchestration.
private enum NativeWebexIngestionError: LocalizedError {
    case webexSyncDisabled

    var errorDescription: String? {
        switch self {
        case .webexSyncDisabled:
            return "Webex sync disabled in Settings."
        }
    }
}

/// Coordinates Webex ingestion, iMessage timeline enrichment, and live focus snapshots.
final class NativeWebexIngestionService: NativeWebexIngesting {
    let configuration: RuntimeConfiguration
    let configStore: ConfigStore
    let webexClient: NativeWebexClienting
    let knowledgeStore: KnowledgeStore
    let iMessageService: NativeIMessageIngesting

    private let encoder: JSONEncoder

    /// Wires ingestion dependencies; tests can inject stores/clients independently.
    init(
        configuration: RuntimeConfiguration = .current,
        configStore: ConfigStore? = nil,
        webexClient: NativeWebexClienting? = nil,
        knowledgeStore: KnowledgeStore? = nil,
        iMessageService: NativeIMessageIngesting? = nil
    ) {
        self.configuration = configuration
        self.configStore = configStore ?? ConfigStore(configuration: configuration)
        self.webexClient = webexClient ?? Self.makeWebexClient(configuration: configuration)
        self.knowledgeStore = knowledgeStore ?? KnowledgeStore(configuration: configuration)
        self.iMessageService = iMessageService ?? NativeIMessageIngestionService(configuration: configuration)
        self.encoder = JSONEncoder()
        self.encoder.outputFormatting = [.prettyPrinted, .sortedKeys]
    }

    static func makeWebexClient(
        configuration: RuntimeConfiguration,
        configStore: ConfigStore? = nil
    ) -> NativeWebexClienting {
        let configDirectory = configuration.jsonConfigurationDirectory
            ?? configuration.runtimeRoot.appendingPathComponent("config", isDirectory: true)
        guard let fixtureURL = configuration.jsonConfiguration?.connectorFixtureURL(
            "webex",
            runtimeRoot: configuration.runtimeRoot,
            configDirectory: configDirectory
        ) else {
            return WebexAPIClient(configuration: configuration, configStore: configStore)
        }
        do {
            return try FixtureWebexAPIClient(fixtureURL: fixtureURL)
        } catch {
            preconditionFailure(error.localizedDescription)
        }
    }

    /// Writes the local Webex map file used to configure important people/spaces.
    func refreshMapFile() async throws -> WebexMapRefreshOutcome {
        let rooms = try await webexClient.rooms()
        guard webexSyncEnabled() else {
            return WebexMapRefreshOutcome(
                mapFileURL: configStore.mapFileURL,
                rooms: rooms.count,
                spaces: 0,
                senders: 0,
                summaryOverride: "Webex map refresh stopped: disabled in Settings."
            )
        }
        let me = rooms.isEmpty ? nil : try? await webexClient.currentUser()
        var entries = makeSpaceMapEntries(rooms)
        entries.append(contentsOf: await makeGroupSpaceMemberAliasEntries(rooms, meID: me?.id ?? ""))
        entries.append(contentsOf: await makeSenderMapEntries(rooms, meID: me?.id ?? ""))

        var deduped: [WebexMapEntry: WebexMapEntry] = [:]
        for entry in entries {
            let key = WebexMapEntry(
                kind: entry.kind,
                label: entry.label.lowercased(),
                roomID: entry.roomID,
                roomType: entry.roomType,
                personEmail: entry.personEmail.lowercased(),
                personDisplayName: entry.personDisplayName.lowercased()
            )
            deduped[key] = entry
        }

        let sortedEntries = deduped.values.sorted { lhs, rhs in
            let left = [lhs.kind, lhs.label.lowercased(), lhs.roomID]
            let right = [rhs.kind, rhs.label.lowercased(), rhs.roomID]
            return left.lexicographicallyPrecedes(right)
        }

        let mapURL = configStore.mapFileURL
        try FileManager.default.createDirectory(
            at: mapURL.deletingLastPathComponent(),
            withIntermediateDirectories: true
        )
        var lines = [
            "# Webex map generated \(Self.mapTimestamp(from: Date()))",
            "# kind\tlabel\troom_id\troom_type\tperson_email\tperson_display_name"
        ]
        lines.append(contentsOf: sortedEntries.map(mapEntryLine))
        try lines.joined(separator: "\n").appending("\n").write(to: mapURL, atomically: true, encoding: .utf8)

        return WebexMapRefreshOutcome(
            mapFileURL: mapURL,
            rooms: rooms.count,
            spaces: sortedEntries.filter { $0.kind == "space" }.count,
            senders: sortedEntries.filter { $0.kind == "sender" }.count
        )
    }

    /// Rebuilds native live focus snapshots from already-stored connector rows.
    func refreshFocusSnapshotsFromKnowledgeStore() throws -> WebexFocusSnapshotRefreshOutcome {
        try knowledgeStore.bootstrap()
        let completedAt = Self.iso8601String(from: Date())
        let spaceTargets = try configStore.importantSpaces()
        let personTargets = try configStore.importantPeople()
        let beliefTargets = try configStore.beliefTargets()
        try writeFocusSnapshots(
            spaceTargets: spaceTargets,
            personTargets: personTargets,
            supplementalRoomTargets: beliefTargets,
            roomTitlesByID: [:],
            updatedAt: completedAt
        )
        return WebexFocusSnapshotRefreshOutcome(
            completedAt: completedAt,
            spaceTargets: spaceTargets.count,
            personTargets: personTargets.count,
            supplementalRoomTargets: beliefTargets.count
        )
    }

    /// Syncs configured targets and writes native live focus snapshots after completion.
    func syncTrackedTargets(
        messageLimitPerRoom: Int = 150,
        mode: WebexSyncMode = .full,
        trigger: WebexSyncTriggerReason = .scheduled,
        progress: ((WebexSyncProgress) async -> Void)? = nil
    ) async throws -> WebexSyncOutcome {
        _ = messageLimitPerRoom
        let startedAt = Self.iso8601String(from: Date())
        guard webexSyncEnabled() else {
            await progress?(disabledProgress(totalRooms: 0, roomResults: []))
            return disabledWebexSyncOutcome(startedAt: startedAt)
        }

        try knowledgeStore.bootstrap()

        let spaceTargets = try configStore.importantSpaces()
        let personTargets = try configStore.importantPeople()
        let beliefTargets = try configStore.beliefTargets()
        let initialRoomTargets = trackedRoomTargets(
            spaceTargets: spaceTargets,
            personTargets: personTargets,
            beliefTargets: beliefTargets
        )
        guard webexSyncEnabled() else {
            await progress?(disabledProgress(totalRooms: initialRoomTargets.count, roomResults: []))
            return disabledWebexSyncOutcome(
                startedAt: startedAt,
                spaceTargets: spaceTargets,
                personTargets: personTargets,
                beliefTargets: beliefTargets
            )
        }

        await progress?(
            WebexSyncProgress(
                completedRooms: 0,
                totalRooms: initialRoomTargets.count,
                fetchedMessages: 0,
                indexedMessages: 0,
                currentRoomTitle: "preparing live snapshots"
            )
        )
        guard webexSyncEnabled() else {
            await progress?(disabledProgress(totalRooms: initialRoomTargets.count, roomResults: []))
            return disabledWebexSyncOutcome(
                startedAt: startedAt,
                spaceTargets: spaceTargets,
                personTargets: personTargets,
                beliefTargets: beliefTargets
            )
        }
        // Do not overwrite persisted focus snapshots before sync completes. If the app is
        // restarted mid-run, early placeholder snapshots can look like random data loss.

        await progress?(
            WebexSyncProgress(
                completedRooms: 0,
                totalRooms: initialRoomTargets.count,
                fetchedMessages: 0,
                indexedMessages: 0,
                currentRoomTitle: "resolving room names"
            )
        )
        guard webexSyncEnabled() else {
            await progress?(disabledProgress(totalRooms: initialRoomTargets.count, roomResults: []))
            return disabledWebexSyncOutcome(
                startedAt: startedAt,
                spaceTargets: spaceTargets,
                personTargets: personTargets,
                beliefTargets: beliefTargets
            )
        }

        let roomActivityProbe = await trackedRoomActivityProbe(
            for: initialRoomTargets,
            mode: mode,
            progress: progress
        )
        let resolvedRoomTitles = await resolveRoomTitles(
            for: initialRoomTargets,
            knownRoomsByID: roomActivityProbe.roomsByID,
            allowRoomDetailLookup: mode == .full,
            progress: progress
        )
        let roomTargets = trackedRoomTargets(
            spaceTargets: spaceTargets,
            personTargets: personTargets,
            beliefTargets: beliefTargets,
            roomTitlesByID: resolvedRoomTitles
        )
        let conversations = trackedConversations(
            roomTargets: roomTargets,
            personTargets: personTargets,
            mode: mode
        )
        let targetKindsByRoomID = Dictionary(
            uniqueKeysWithValues: roomTargets.map { (normalizedRoomID($0.roomID), $0.kinds) }
        )

        let triggerForSync = resolvedTrigger(mode: mode, requestedTrigger: trigger)
        let conversationPlan: ConversationSyncPlan
        do {
            conversationPlan = try planConversationsForSync(
                conversations,
                trigger: triggerForSync
            )
        } catch {
            conversationPlan = ConversationSyncPlan(
                conversationsToSync: conversations,
                precomputedResults: []
            )
        }

        var roomResults: [WebexRoomSyncResult] = conversationPlan.precomputedResults.map {
            roomSyncResult(from: $0, targetKindsByRoomID: targetKindsByRoomID)
        }
        roomResults.reserveCapacity(conversations.count)
        let precomputedCount = roomResults.count
        let precomputedChangedRooms = roomResults.filter { !$0.unchanged }

        await progress?(
            WebexSyncProgress(
                completedRooms: precomputedCount,
                totalRooms: conversations.count,
                fetchedMessages: precomputedChangedRooms.reduce(0) { $0 + $1.fetchedMessages },
                indexedMessages: precomputedChangedRooms.reduce(0) { $0 + $1.indexedMessages },
                currentRoomTitle: conversationPlan.conversationsToSync.first?.displayName,
                unchangedRooms: roomResults.filter(\.unchanged).count,
                skippedRooms: roomResults.filter { $0.skippedReason != nil && !$0.unchanged }.count
            )
        )

        guard webexSyncEnabled() else {
            await progress?(disabledProgress(totalRooms: conversations.count, roomResults: roomResults))
            return disabledWebexSyncOutcome(
                startedAt: startedAt,
                spaceTargets: spaceTargets,
                personTargets: personTargets,
                beliefTargets: beliefTargets,
                roomResults: roomResults
            )
        }

        if !conversationPlan.conversationsToSync.isEmpty {
            let syncEngine = await makeSyncEngine()
            let syncResults = await syncEngine.syncConversations(
                conversationPlan.conversationsToSync,
                trigger: triggerForSync
            )

            for (index, result) in syncResults.enumerated() {
                let roomResult = roomSyncResult(from: result, targetKindsByRoomID: targetKindsByRoomID)
                roomResults.append(roomResult)

                let changedRoomResults = roomResults.filter { !$0.unchanged }
                await progress?(
                    WebexSyncProgress(
                        completedRooms: precomputedCount + index + 1,
                        totalRooms: conversations.count,
                        fetchedMessages: changedRoomResults.reduce(0) { $0 + $1.fetchedMessages },
                        indexedMessages: changedRoomResults.reduce(0) { $0 + $1.indexedMessages },
                        currentRoomTitle: conversationPlan.conversationsToSync.dropFirst(index + 1).first?.displayName,
                        unchangedRooms: roomResults.filter(\.unchanged).count,
                        skippedRooms: roomResults.filter { $0.skippedReason != nil && !$0.unchanged }.count
                    )
                )
            }
        }

        let completedAt = Self.iso8601String(from: Date())
        let roomTitlesByID = resolvedRoomTitles.merging(Dictionary(
            uniqueKeysWithValues: roomResults
                .filter { !$0.roomID.isEmpty }
                .map { ($0.roomID, $0.title) }
        )) { _, syncedTitle in syncedTitle }
        guard webexSyncEnabled() else {
            await progress?(disabledProgress(totalRooms: conversations.count, roomResults: roomResults))
            return disabledWebexSyncOutcome(
                startedAt: startedAt,
                spaceTargets: spaceTargets,
                personTargets: personTargets,
                beliefTargets: beliefTargets,
                roomResults: roomResults
            )
        }
        try writeFocusSnapshots(
            spaceTargets: spaceTargets,
            personTargets: personTargets,
            supplementalRoomTargets: beliefTargets,
            roomTitlesByID: roomTitlesByID,
            updatedAt: completedAt
        )

        let requestedTargets = spaceTargets.count + personTargets.count + beliefTargets.count
        let skippedConfigTargets = (spaceTargets + personTargets + beliefTargets).filter {
            normalizedRoomID($0.roomID).isEmpty && normalizedEmail($0.email).isEmpty
        }.count
        let skippedRooms = roomResults.filter { $0.skippedReason != nil && !$0.unchanged }.count
        let unchangedRooms = roomResults.filter(\.unchanged).count

        return WebexSyncOutcome(
            startedAt: startedAt,
            completedAt: completedAt,
            requestedTargets: requestedTargets,
            syncedRooms: roomResults.filter { $0.skippedReason == nil && !$0.unchanged }.count,
            fetchedMessages: roomResults.filter { !$0.unchanged }.reduce(0) { $0 + $1.fetchedMessages },
            indexedMessages: roomResults.filter { !$0.unchanged }.reduce(0) { $0 + $1.indexedMessages },
            indexedEvidence: roomResults.filter { !$0.unchanged }.reduce(0) { $0 + $1.indexedEvidence },
            skippedTargets: skippedConfigTargets + skippedRooms,
            unchangedRooms: unchangedRooms,
            roomResults: roomResults
        )
    }

    /// Applies persisted backoff/polling gates before API calls are made.
    private func planConversationsForSync(
        _ conversations: [WebexTrackedConversation],
        trigger: WebexSyncTriggerReason
    ) throws -> ConversationSyncPlan {
        guard !shouldBypassNextAllowedForTrigger(trigger) else {
            return ConversationSyncPlan(
                conversationsToSync: conversations,
                precomputedResults: []
            )
        }

        let allStates = try knowledgeStore.loadWebexSyncStates(limit: max(10_000, conversations.count + 100))
        let statesByConversationID = Dictionary(uniqueKeysWithValues: allStates.map { ($0.conversationID, $0) })
        let nowDate = Date()

        var conversationsToSync: [WebexTrackedConversation] = []
        conversationsToSync.reserveCapacity(conversations.count)
        var precomputedResults: [WebexConversationSyncResult] = []
        precomputedResults.reserveCapacity(conversations.count)

        for conversation in conversations {
            if conversation.pollingMode == .paused || conversation.pollingMode == .disabled {
                precomputedResults.append(
                    skippedConversationSyncResult(
                        conversation: conversation,
                        state: statesByConversationID[conversation.conversationID],
                        reason: "Polling is \(conversation.pollingMode.rawValue).",
                        status: .synced
                    )
                )
                continue
            }

            guard let state = statesByConversationID[conversation.conversationID] else {
                conversationsToSync.append(conversation)
                continue
            }

            if let nextAllowed = normalizedOptionalText(state.nextAllowedSyncAt),
               let nextAllowedDate = parsedDate(nextAllowed),
               nextAllowedDate > nowDate {
                precomputedResults.append(
                    skippedConversationSyncResult(
                        conversation: conversation,
                        state: state,
                        reason: "Next allowed sync at \(nextAllowed).",
                        status: .delayedRateLimit
                    )
                )
                continue
            }

            conversationsToSync.append(conversation)
        }

        return ConversationSyncPlan(
            conversationsToSync: conversationsToSync,
            precomputedResults: precomputedResults
        )
    }

    private func shouldBypassNextAllowedForTrigger(_ trigger: WebexSyncTriggerReason) -> Bool {
        switch trigger {
        case .manual, .wakeFromSleep, .networkReconnect, .userOpenedConversation:
            return true
        case .startup, .scheduled:
            return false
        }
    }

    private func skippedConversationSyncResult(
        conversation: WebexTrackedConversation,
        state: WebexConversationSyncStateRecord?,
        reason: String,
        status: WebexSyncStatusIndicator
    ) -> WebexConversationSyncResult {
        let persistedRoomID = normalizedOptionalText(state?.roomID)
        let roomID = persistedRoomID ?? normalizedRoomID(conversation.roomID)
        return WebexConversationSyncResult(
            conversationID: conversation.conversationID,
            roomID: roomID,
            displayName: conversation.displayName,
            fetchedMessages: 0,
            processedMessages: 0,
            processedEvidence: 0,
            latestMessageID: state?.lastSeenMessageID,
            latestMessageCreated: state?.lastSeenCreated,
            skippedReason: reason,
            unchanged: true,
            status: status
        )
    }

    private func roomSyncResult(
        from result: WebexConversationSyncResult,
        targetKindsByRoomID: [String: Set<ConfigTarget.Kind>]
    ) -> WebexRoomSyncResult {
        let roomID = normalizedRoomID(result.roomID)
        var targetKinds = targetKindsByRoomID[roomID] ?? []
        if targetKinds.isEmpty, result.conversationID.hasPrefix("person:") {
            targetKinds = [.person]
        }
        let title = result.displayName.trimmingCharacters(in: .whitespacesAndNewlines).isEmpty
            ? (roomID.isEmpty ? result.conversationID : fallbackRoomTitle(roomID))
            : result.displayName
        return WebexRoomSyncResult(
            roomID: roomID,
            title: title,
            targetKinds: targetKinds.map(\.rawValue).sorted(),
            fetchedMessages: result.fetchedMessages,
            indexedMessages: result.processedMessages,
            indexedEvidence: result.processedEvidence,
            memberCount: 0,
            latestMessageID: result.latestMessageID,
            latestMessageCreated: result.latestMessageCreated,
            skippedReason: result.skippedReason,
            unchanged: result.unchanged
        )
    }

    private func webexSyncEnabled() -> Bool {
        configStore.loadSystemSettings().webexSyncEnabled
            && connectorEnabled("webex")
    }

    private func iMessageConnectorEnabled() -> Bool {
        connectorEnabled("imessage")
    }

    private func connectorEnabled(_ connectorID: String) -> Bool {
        configuration.jsonConfiguration?
            .connectors?
            .connectorEnabled(connectorID) ?? true
    }

    private func disabledProgress(totalRooms: Int, roomResults: [WebexRoomSyncResult]) -> WebexSyncProgress {
        let changedRoomResults = roomResults.filter { !$0.unchanged }
        return WebexSyncProgress(
            completedRooms: roomResults.count,
            totalRooms: totalRooms,
            fetchedMessages: changedRoomResults.reduce(0) { $0 + $1.fetchedMessages },
            indexedMessages: changedRoomResults.reduce(0) { $0 + $1.indexedMessages },
            currentRoomTitle: "disabled in Settings",
            unchangedRooms: roomResults.filter(\.unchanged).count,
            skippedRooms: roomResults.filter { $0.skippedReason != nil && !$0.unchanged }.count
        )
    }

    private func disabledWebexSyncOutcome(
        startedAt: String,
        spaceTargets: [ConfigTarget] = [],
        personTargets: [ConfigTarget] = [],
        beliefTargets: [ConfigTarget] = [],
        roomResults: [WebexRoomSyncResult] = []
    ) -> WebexSyncOutcome {
        let completedAt = Self.iso8601String(from: Date())
        let changedRoomResults = roomResults.filter { !$0.unchanged }
        let requestedTargets = spaceTargets.count + personTargets.count + beliefTargets.count
        let skippedConfigTargets = (spaceTargets + personTargets + beliefTargets).filter {
            normalizedRoomID($0.roomID).isEmpty && normalizedEmail($0.email).isEmpty
        }.count
        let skippedRooms = roomResults.filter { $0.skippedReason != nil && !$0.unchanged }.count
        return WebexSyncOutcome(
            startedAt: startedAt,
            completedAt: completedAt,
            requestedTargets: requestedTargets,
            syncedRooms: changedRoomResults.filter { $0.skippedReason == nil }.count,
            fetchedMessages: changedRoomResults.reduce(0) { $0 + $1.fetchedMessages },
            indexedMessages: changedRoomResults.reduce(0) { $0 + $1.indexedMessages },
            indexedEvidence: changedRoomResults.reduce(0) { $0 + $1.indexedEvidence },
            skippedTargets: skippedConfigTargets + skippedRooms,
            unchangedRooms: roomResults.filter(\.unchanged).count,
            roomResults: roomResults,
            summaryOverride: "Webex sync stopped: disabled in Settings."
        )
    }

    /// Builds the adaptive polling engine with self-message filtering.
    private func makeSyncEngine() async -> WebexSyncEngine {
        // Cubicle runs as a local macOS app, so we keep adaptive REST polling as the
        // production sync path. Webex webhooks require a stable public HTTPS target URL.
        // An optional publicWebhookUrl can be configured for future/dev use, but polling
        // must remain fully functional without any webhook endpoint.
        let currentUser = try? await webexClient.currentUser()
        let selfPersonID = currentUser?.id
            .trimmingCharacters(in: .whitespacesAndNewlines)
        let selfEmail = currentUser?.emails.first?
            .trimmingCharacters(in: .whitespacesAndNewlines)
            .lowercased()
        let processor = MessageProcessor(
            knowledgeStore: knowledgeStore,
            ignoreSelfMessages: true,
            selfPersonID: selfPersonID,
            selfEmail: selfEmail
        )
        let syncStateStore = SyncStateStore(knowledgeStore: knowledgeStore)
        let engineConfig = WebexSyncEngine.Configuration(
            maxConcurrentAPIRequests: max(1, configuration.webexSyncConcurrencyLimit),
            activeIntervalSeconds: max(5, configuration.webexAdaptiveActiveIntervalSeconds),
            recentIntervalSeconds: max(15, configuration.webexAdaptiveRecentIntervalSeconds),
            backgroundIntervalSeconds: max(60, configuration.webexAdaptiveBackgroundIntervalSeconds),
            jitterRatio: min(max(0, configuration.webexAdaptiveJitterRatio), 0.8)
        )
        return WebexSyncEngine(
            webexClient: webexClient,
            stateStore: syncStateStore,
            messageProcessor: processor,
            configuration: engineConfig
        )
    }

    /// Converts current config targets into unique sync-engine conversations.
    private func trackedConversations(
        roomTargets: [TrackedRoomTarget],
        personTargets: [ConfigTarget],
        mode: WebexSyncMode
    ) -> [WebexTrackedConversation] {
        var conversationsByID: [String: WebexTrackedConversation] = [:]

        for target in roomTargets {
            let roomID = normalizedRoomID(target.roomID)
            guard !roomID.isEmpty else { continue }
            let kindValues = target.kinds
            let conversationType: WebexConversationType = kindValues.allSatisfy { $0 == .person } ? .direct : .space
            let matchedPersonTarget = personTargets.first { normalizedRoomID($0.roomID) == roomID }
            let conversation = WebexTrackedConversation(
                conversationID: "room:\(roomID)",
                conversationType: conversationType,
                roomID: roomID,
                personID: nil,
                personEmail: normalizedEmail(matchedPersonTarget?.email ?? ""),
                displayName: target.label,
                pollingMode: pollingMode(for: mode, conversationType: conversationType)
            )
            conversationsByID[conversation.conversationID] = conversation
        }

        for personTarget in personTargets {
            let roomID = normalizedRoomID(personTarget.roomID)
            if !roomID.isEmpty {
                continue
            }
            let email = normalizedEmail(personTarget.email)
            guard !email.isEmpty else {
                continue
            }
            let displayName = personTarget.label.trimmingCharacters(in: .whitespacesAndNewlines).isEmpty
                ? email
                : personTarget.label
            let conversationID = "person:\(email)"
            let conversation = WebexTrackedConversation(
                conversationID: conversationID,
                conversationType: .direct,
                roomID: "",
                personID: nil,
                personEmail: email,
                displayName: displayName,
                pollingMode: pollingMode(for: mode, conversationType: .direct)
            )
            conversationsByID[conversationID] = conversation
        }

        return conversationsByID.values.sorted { lhs, rhs in
            lhs.displayName.localizedCaseInsensitiveCompare(rhs.displayName) == .orderedAscending
        }
    }

    private func pollingMode(
        for mode: WebexSyncMode,
        conversationType: WebexConversationType
    ) -> WebexPollingMode {
        switch mode {
        case .full:
            return .active
        case .incremental:
            return conversationType == .direct ? .recent : .background
        }
    }

    private func resolvedTrigger(
        mode: WebexSyncMode,
        requestedTrigger: WebexSyncTriggerReason
    ) -> WebexSyncTriggerReason {
        switch requestedTrigger {
        case .manual, .wakeFromSleep, .networkReconnect, .userOpenedConversation:
            return requestedTrigger
        case .startup, .scheduled:
            return mode == .full ? .startup : .scheduled
        }
    }

    /// Uses Webex room activity as a cheap unchanged-room probe for incremental runs.
    private func trackedRoomActivityProbe(
        for targets: [TrackedRoomTarget],
        mode: WebexSyncMode,
        progress: ((WebexSyncProgress) async -> Void)?
    ) async -> TrackedRoomActivityProbe {
        guard mode == .incremental else {
            return TrackedRoomActivityProbe(roomsByID: [:], usedLiveRoomList: false)
        }

        await progress?(
            WebexSyncProgress(
                completedRooms: 0,
                totalRooms: 0,
                fetchedMessages: 0,
                indexedMessages: 0,
                currentRoomTitle: "checking Webex room activity"
            )
        )

        do {
            guard webexSyncEnabled() else {
                return TrackedRoomActivityProbe(roomsByID: [:], usedLiveRoomList: false)
            }
            let rooms = try await webexClient.rooms()
            let roomsByID = Dictionary(uniqueKeysWithValues: rooms.compactMap { room -> (String, WebexRoom)? in
                let roomID = normalizedRoomID(room.id)
                guard !roomID.isEmpty else { return nil }
                return (roomID, room)
            })
            return TrackedRoomActivityProbe(roomsByID: roomsByID, usedLiveRoomList: true)
        } catch {
            await progress?(
                WebexSyncProgress(
                    completedRooms: 0,
                    totalRooms: targets.count,
                    fetchedMessages: 0,
                    indexedMessages: 0,
                    currentRoomTitle: "activity check unavailable; scanning tracked rooms"
                )
            )
            return TrackedRoomActivityProbe(roomsByID: [:], usedLiveRoomList: false)
        }
    }

    /// Plans legacy room syncs by comparing remote activity with local latest messages.
    private func roomSyncPlan(
        for targets: [TrackedRoomTarget],
        roomsByID: [String: WebexRoom],
        mode: WebexSyncMode
    ) throws -> RoomSyncPlan {
        guard mode == .incremental, !roomsByID.isEmpty else {
            return RoomSyncPlan(roomsToSync: targets, unchangedResults: [])
        }

        var roomsToSync: [TrackedRoomTarget] = []
        var unchangedResults: [WebexRoomSyncResult] = []
        for target in targets {
            let roomID = normalizedRoomID(target.roomID)
            guard !roomID.isEmpty else {
                roomsToSync.append(target)
                continue
            }
            guard let room = roomsByID[roomID],
                  let remoteActivity = parsedDate(room.lastActivity),
                  let latestLocalMessage = try knowledgeStore.latestMessage(roomID: roomID),
                  let latestLocalActivity = parsedDate(latestLocalMessage.createdAt),
                  remoteActivity <= latestLocalActivity else {
                roomsToSync.append(target)
                continue
            }

            let title = usableTitle(room.title, roomID: roomID) ?? target.label
            unchangedResults.append(
                WebexRoomSyncResult(
                    roomID: roomID,
                    title: title,
                    targetKinds: target.kinds.map(\.rawValue).sorted(),
                    fetchedMessages: 0,
                    indexedMessages: 0,
                    indexedEvidence: 0,
                    memberCount: 0,
                    latestMessageID: latestLocalMessage.id,
                    latestMessageCreated: latestLocalMessage.createdAt,
                    skippedReason: nil,
                    unchanged: true
                )
            )
        }

        return RoomSyncPlan(roomsToSync: roomsToSync, unchangedResults: unchangedResults)
    }

    /// Legacy direct room sync path retained for compatibility with older tests/callers.
    private func syncRoomTarget(_ target: TrackedRoomTarget, messageLimit: Int) async throws -> WebexRoomSyncResult {
        let roomID = normalizedRoomID(target.roomID)
        guard !roomID.isEmpty else {
            return WebexRoomSyncResult(
                roomID: target.roomID,
                title: target.label,
                targetKinds: target.kinds.map(\.rawValue).sorted(),
                fetchedMessages: 0,
                indexedMessages: 0,
                indexedEvidence: 0,
                memberCount: 0,
                latestMessageID: nil,
                latestMessageCreated: nil,
                skippedReason: "Missing Webex room ID"
            )
        }

        let latest = try knowledgeStore.latestMessage(roomID: roomID)
        let messages: [WebexMessage]
        if let latest {
            messages = try await webexClient.messagesAfter(
                roomID: roomID,
                lastMessageID: latest.id,
                lastMessageCreated: latest.createdAt,
                max: messageLimit
            )
        } else {
            messages = try await webexClient.messages(roomID: roomID, max: messageLimit)
        }
        guard webexSyncEnabled() else {
            throw NativeWebexIngestionError.webexSyncDisabled
        }

        let now = Self.iso8601String(from: Date())
        let memberships = try await webexClient.memberships(roomID: roomID)
        guard webexSyncEnabled() else {
            throw NativeWebexIngestionError.webexSyncDisabled
        }
        for membership in memberships {
            try knowledgeStore.upsertPerson(
                PersonRecord(
                    id: normalizedPersonID(membership.personID, fallbackEmail: membership.personEmail),
                    displayName: membership.personDisplayName,
                    email: membership.personEmail,
                    updatedAt: now
                )
            )
        }

        let messageRecords = messages.map { message in
            MessageRecord(
                id: message.id,
                roomID: roomID,
                personID: normalizedPersonID(message.personID, fallbackEmail: message.personEmail),
                body: normalizedMessageText(message.text),
                createdAt: normalizedTimestamp(message.created, fallback: now),
                updatedAt: now
            )
        }
        try knowledgeStore.upsertMessages(messageRecords)

        let evidenceRecords = messageRecords.compactMap { message -> BeliefEvidenceRecord? in
            let text = message.body.trimmingCharacters(in: .whitespacesAndNewlines)
            guard !text.isEmpty else { return nil }
            return BeliefEvidenceRecord(
                id: "webex-message-\(message.id)",
                source: "webex_message",
                sourceID: message.id,
                roomID: roomID,
                personID: message.personID,
                occurredAt: message.createdAt,
                text: text
            )
        }
        try knowledgeStore.upsertBeliefEvidence(evidenceRecords)

        let previousLatest = latest.map { [$0] } ?? []
        let latestSynced = (messageRecords + previousLatest)
            .max { lhs, rhs in lhs.createdAt < rhs.createdAt }
        let roomUpdatedAt = latestSynced?.createdAt ?? now
        try knowledgeStore.upsertRoom(
            RoomRecord(
                id: roomID,
                title: target.label,
                updatedAt: roomUpdatedAt
            )
        )

        return WebexRoomSyncResult(
            roomID: roomID,
            title: target.label,
            targetKinds: target.kinds.map(\.rawValue).sorted(),
            fetchedMessages: messages.count,
            indexedMessages: messageRecords.count,
            indexedEvidence: evidenceRecords.count,
            memberCount: memberships.count,
            latestMessageID: latestSynced?.id,
            latestMessageCreated: latestSynced?.createdAt,
            skippedReason: nil
        )
    }

    private func makeSpaceMapEntries(_ rooms: [WebexRoom]) -> [WebexMapEntry] {
        rooms.compactMap { room in
            guard room.type != "direct" else { return nil }
            let roomID = cleanMapValue(room.id)
            let title = cleanMapValue(room.title)
            guard !roomID.isEmpty, !title.isEmpty else { return nil }
            return WebexMapEntry(
                kind: "space",
                label: title,
                roomID: roomID,
                roomType: cleanMapValue(room.type.isEmpty ? "unknown" : room.type)
            )
        }
    }

    private func makeSenderMapEntries(_ rooms: [WebexRoom], meID: String) async -> [WebexMapEntry] {
        let directRooms = rooms.filter { $0.type == "direct" && !$0.id.trimmingCharacters(in: .whitespacesAndNewlines).isEmpty }
        var entries: [WebexMapEntry] = []
        for room in directRooms {
            guard webexSyncEnabled() else { break }
            let memberships = (try? await webexClient.memberships(roomID: room.id)) ?? []
            let otherMembers = memberships.filter { member in
                let memberID = member.personID.trimmingCharacters(in: .whitespacesAndNewlines)
                return meID.isEmpty || memberID != meID
            }
            for member in otherMembers {
                let displayName = cleanMapValue(member.personDisplayName)
                let personEmail = cleanMapValue(member.personEmail)
                var labels = Array(Set([displayName, personEmail])).filter { !$0.isEmpty }
                if labels.isEmpty {
                    labels = [room.id]
                }
                for label in labels {
                    entries.append(
                        WebexMapEntry(
                            kind: "sender",
                            label: label,
                            roomID: cleanMapValue(room.id),
                            roomType: cleanMapValue(room.type.isEmpty ? "direct" : room.type),
                            personEmail: personEmail,
                            personDisplayName: displayName
                        )
                    )
                }
            }
        }
        return entries
    }

    private func makeGroupSpaceMemberAliasEntries(_ rooms: [WebexRoom], meID: String) async -> [WebexMapEntry] {
        let candidateRooms = rooms.filter { room in
            room.type != "direct"
                && !room.id.trimmingCharacters(in: .whitespacesAndNewlines).isEmpty
                && looksLikePersonListSpaceTitle(room.title)
        }

        var entries: [WebexMapEntry] = []
        for room in candidateRooms {
            guard webexSyncEnabled() else { break }
            let memberships = (try? await webexClient.memberships(roomID: room.id)) ?? []
            var memberNames: [String] = []
            var seenNames = Set<String>()
            for member in memberships {
                let memberID = member.personID.trimmingCharacters(in: .whitespacesAndNewlines)
                if !meID.isEmpty, memberID == meID {
                    continue
                }
                let displayName = cleanMapValue(member.personDisplayName.isEmpty ? member.personEmail : member.personDisplayName)
                guard !displayName.isEmpty else { continue }
                let key = displayName.lowercased()
                guard seenNames.insert(key).inserted else { continue }
                memberNames.append(displayName)
            }
            guard memberNames.count >= 2, memberNames.count <= 12 else { continue }
            let alias = memberNames.sorted { $0.localizedCaseInsensitiveCompare($1) == .orderedAscending }
                .joined(separator: ", ")
            let currentTitle = cleanMapValue(room.title)
            guard !alias.isEmpty, alias.lowercased() != currentTitle.lowercased() else { continue }
            entries.append(
                WebexMapEntry(
                    kind: "space",
                    label: alias,
                    roomID: cleanMapValue(room.id),
                    roomType: cleanMapValue(room.type.isEmpty ? "group" : room.type)
                )
            )
        }
        return entries
    }

    private func mapEntryLine(_ entry: WebexMapEntry) -> String {
        [
            entry.kind,
            entry.label,
            entry.roomID,
            entry.roomType,
            entry.personEmail,
            entry.personDisplayName
        ]
        .map(cleanMapValue)
        .joined(separator: "\t")
    }

    /// Writes native live snapshots consumed by the runtime store, not canonical Pine output.
    private func writeFocusSnapshots(
        spaceTargets: [ConfigTarget],
        personTargets: [ConfigTarget],
        supplementalRoomTargets: [ConfigTarget],
        roomTitlesByID: [String: String],
        updatedAt: String
    ) throws {
        let knowledgeDirectory = configuration.runtimeRoot.appendingPathComponent("knowledge", isDirectory: true)
        try FileManager.default.createDirectory(at: knowledgeDirectory, withIntermediateDirectories: true)
        let nativeDirectory = knowledgeDirectory.appendingPathComponent("native", isDirectory: true)
        try FileManager.default.createDirectory(at: nativeDirectory, withIntermediateDirectories: true)

        let allRoomTargets = deduplicatedTargetsByRoom(spaceTargets + personTargets + supplementalRoomTargets)
        let allTrackedRoomIDs = allRoomTargets.map { normalizedRoomID($0.roomID) }.filter { !$0.isEmpty }
        let people = try knowledgeStore.loadPeople()
        let peopleByID = Dictionary(uniqueKeysWithValues: people.map { ($0.id, $0) })
        let syncStatesByRoomID = try latestSyncStatesByRoomID()
        let settings = configStore.loadSystemSettings()

        let spaceCache = try makeSpaceFocusCache(
            targets: spaceTargets,
            roomTitlesByID: roomTitlesByID,
            peopleByID: peopleByID,
            syncStatesByRoomID: syncStatesByRoomID,
            updatedAt: updatedAt,
            focusDays: settings.spaceFocusDays
        )
        let personCache = try makePersonFocusCache(
            targets: personTargets,
            trackedRoomIDs: allTrackedRoomIDs,
            roomTitlesByID: roomTitlesByID,
            peopleByID: peopleByID,
            syncStatesByRoomID: syncStatesByRoomID,
            updatedAt: updatedAt,
            focusDays: settings.personFocusDays
        )

        // Canonical focus snapshots are owned by the Pine/Python runtime and carry
        // rich Codex sections plus opaque metadata. Keep native live Webex snapshots
        // separate so API polling cannot clobber the richer focus cache.
        try encoder.encode(spaceCache)
            .write(to: nativeDirectory.appendingPathComponent("live_\(FocusKind.space.snapshotFilename(days: settings.spaceFocusDays))"), options: [.atomic])
        try encoder.encode(personCache)
            .write(to: nativeDirectory.appendingPathComponent("live_\(FocusKind.person.snapshotFilename(days: settings.personFocusDays))"), options: [.atomic])
    }

    /// Builds the live Webex-backed cache for configured spaces.
    private func makeSpaceFocusCache(
        targets: [ConfigTarget],
        roomTitlesByID: [String: String],
        peopleByID: [String: PersonRecord],
        syncStatesByRoomID: [String: WebexConversationSyncStateRecord],
        updatedAt: String,
        focusDays: Int
    ) throws -> FocusCache {
        var totalRecentMessages = 0
        let sinceTimestamp = sinceTimestamp(focusDays: focusDays)
        let items = try targets.map { target -> FocusItem in
            let roomID = normalizedRoomID(target.roomID)
            let title = displayTitle(target: target, roomTitlesByID: roomTitlesByID)
            guard !roomID.isEmpty else {
                return missingRoomItem(target: target, kind: .space, updatedAt: updatedAt)
            }

            let recentMessages = try knowledgeStore.loadMessages(roomID: roomID, sinceTimestamp: sinceTimestamp, limit: 500)
            let latestKnownMessage: MessageRecord?
            if let recentFirst = recentMessages.first {
                latestKnownMessage = recentFirst
            } else {
                latestKnownMessage = try knowledgeStore.loadMessages(roomID: roomID, limit: 1).first
            }
            totalRecentMessages += recentMessages.count
            return focusItem(
                id: roomID,
                title: title,
                subtitle: latestMessagePreview(
                    recentMessage: recentMessages.first,
                    latestKnownMessage: latestKnownMessage,
                    focusDays: focusDays
                ),
                meta: "\(autoReplyMeta(target)) | messages=\(recentMessages.count)",
                timestamp: recentMessages.first?.createdAt ?? latestKnownMessage?.createdAt ?? "",
                badge: "space",
                statusBadge: appendSyncStatusBadge(
                    base: "live-webex",
                    state: syncStatesByRoomID[roomID]
                ),
                roomID: roomID,
                roomTitle: title,
                messages: recentMessages,
                latestKnownMessage: latestKnownMessage,
                peopleByID: peopleByID,
                focusDays: focusDays,
                updatedAt: updatedAt
            )
        }

        return FocusCache(
            focusDays: focusDays,
            items: items,
            updatedAt: updatedAt,
            countLabel: String(items.count),
            recentMessages: totalRecentMessages,
            summaryGenerationInProgress: false,
            subjectsProcessed: items.count,
            subjectsTotal: items.count
        )
    }

    /// Builds the live person cache from Webex messages plus optional iMessage events.
    func makePersonFocusCache(
        targets: [ConfigTarget],
        trackedRoomIDs: [String],
        roomTitlesByID: [String: String],
        peopleByID: [String: PersonRecord],
        syncStatesByRoomID: [String: WebexConversationSyncStateRecord],
        updatedAt: String,
        focusDays: Int
    ) throws -> FocusCache {
        var messagesByRoom: [String: [MessageRecord]] = [:]
        let sinceTimestamp = sinceTimestamp(focusDays: focusDays)
        let sinceDate = sinceDate(focusDays: focusDays)
        for roomID in trackedRoomIDs {
            messagesByRoom[roomID] = try knowledgeStore.loadMessages(roomID: roomID, sinceTimestamp: sinceTimestamp, limit: 500)
        }

        func mergedMessages(_ groups: [[MessageRecord]]) -> [MessageRecord] {
            var byID: [String: MessageRecord] = [:]
            for message in groups.flatMap({ $0 }) {
                byID[message.id] = message
            }
            return byID.values.sorted { left, right in
                left.createdAt > right.createdAt
            }
        }

        var totalRecentMessages = 0
        let items = try targets.map { target -> FocusItem in
            let targetEmail = normalizedEmail(target.email)
            let directRoomID = normalizedRoomID(target.roomID)
            let targetPersonID = targetEmail.isEmpty ? "" : normalizedPersonID("", fallbackEmail: targetEmail)
            let title = target.label.trimmingCharacters(in: .whitespacesAndNewlines).isEmpty
                ? (targetEmail.isEmpty ? target.id : targetEmail)
                : target.label

            let matchingMessages: [MessageRecord]
            let locallySubmittedPersonMessages = targetPersonID.isEmpty
                ? []
                : try knowledgeStore.loadMessages(personID: targetPersonID, sinceTimestamp: sinceTimestamp, limit: 500)
            if !directRoomID.isEmpty, let directMessages = messagesByRoom[directRoomID] {
                matchingMessages = mergedMessages([directMessages, locallySubmittedPersonMessages])
            } else if !targetEmail.isEmpty {
                let webexMatchedMessages = messagesByRoom.values
                    .flatMap { $0 }
                    .filter { message in
                        guard let personID = message.personID,
                              let person = peopleByID[personID] else {
                            return false
                        }
                        return normalizedEmail(person.email) == targetEmail
                    }
                matchingMessages = mergedMessages([webexMatchedMessages, locallySubmittedPersonMessages])
            } else {
                matchingMessages = []
            }

            let iMessageOutcome = loadIMessageTimeline(
                target: target,
                title: title,
                since: sinceDate
            )
            let timelineEvents = personTimelineEvents(
                webexMessages: matchingMessages,
                iMessages: iMessageOutcome.messages,
                peopleByID: peopleByID,
                roomTitlesByID: roomTitlesByID,
                fallbackRoomTitle: title
            )
            totalRecentMessages += timelineEvents.count
            var detailLines = personDetailLines(
                target: target,
                roomID: directRoomID.isEmpty ? "derived-from-tracked-spaces" : directRoomID,
                title: title,
                updatedAt: updatedAt,
                focusDays: focusDays,
                timelineEvents: timelineEvents,
                iMessageLoadError: iMessageOutcome.error
            )
            if directRoomID.isEmpty {
                detailLines.insert("Person source: matched by sender email across tracked rooms.", at: 2)
            }

            return FocusItem(
                id: targetEmail.isEmpty ? target.id : targetEmail,
                title: title,
                subtitle: latestTimelinePreview(timelineEvents.first, iMessageLoadError: iMessageOutcome.error),
                meta: "\(autoReplyMeta(target)) | messages=\(timelineEvents.count)",
                timestamp: timelineEvents.first?.createdAt ?? "",
                badge: "person",
                statusBadge: personStatusBadge(
                    directRoomID: directRoomID,
                    webexMessages: matchingMessages,
                    iMessages: iMessageOutcome.messages,
                    target: target,
                    syncState: syncStatesByRoomID[directRoomID],
                    iMessageLoadError: iMessageOutcome.error
                ),
                detailLines: detailLines,
                detailIntroLines: [],
                detailSections: [],
                detailTailLines: []
            )
        }

        return FocusCache(
            focusDays: focusDays,
            items: items,
            updatedAt: updatedAt,
            countLabel: String(items.count),
            recentMessages: totalRecentMessages,
            summaryGenerationInProgress: false,
            subjectsProcessed: items.count,
            subjectsTotal: items.count
        )
    }

    /// Loads iMessage evidence without failing the broader person-focus snapshot.
    private func loadIMessageTimeline(
        target: ConfigTarget,
        title: String,
        since: Date
    ) -> (messages: [IMessageTimelineMessage], error: String?) {
        guard !target.iMessageHandles.isEmpty else {
            return ([], nil)
        }
        guard iMessageConnectorEnabled() else {
            return ([], nil)
        }
        do {
            let messages = try iMessageService.loadMessages(
                matching: target.iMessageHandles,
                displayName: title,
                since: since,
                limit: 500
            )
            return (messages, nil)
        } catch {
            return ([], error.localizedDescription)
        }
    }

    /// Merges Webex and iMessage evidence into one recency-sorted timeline.
    private func personTimelineEvents(
        webexMessages: [MessageRecord],
        iMessages: [IMessageTimelineMessage],
        peopleByID: [String: PersonRecord],
        roomTitlesByID: [String: String],
        fallbackRoomTitle: String
    ) -> [PersonTimelineEvent] {
        let webexEvents = webexMessages.map { message in
            PersonTimelineEvent(
                id: message.id,
                source: "Webex",
                createdAt: message.createdAt,
                sortDate: parsedDate(message.createdAt),
                sender: senderLabel(message: message, peopleByID: peopleByID),
                sourceTitle: roomTitlesByID[message.roomID] ?? fallbackRoomTitle,
                body: message.body
            )
        }
        let iMessageEvents = iMessages.map { message in
            PersonTimelineEvent(
                id: message.id,
                source: "iMessage",
                createdAt: message.createdAt,
                sortDate: message.sortDate,
                sender: message.sender,
                sourceTitle: message.threadTitle,
                body: message.body
            )
        }
        return (webexEvents + iMessageEvents).sorted { left, right in
            switch (left.sortDate, right.sortDate) {
            case let (leftDate?, rightDate?):
                return leftDate > rightDate
            case (_?, nil):
                return true
            case (nil, _?):
                return false
            case (nil, nil):
                return left.createdAt > right.createdAt
            }
        }
    }

    private func personDetailLines(
        target: ConfigTarget,
        roomID: String,
        title: String,
        updatedAt: String,
        focusDays: Int,
        timelineEvents: [PersonTimelineEvent],
        iMessageLoadError: String?
    ) -> [String] {
        var lines: [String] = [
            "Space Name: \(title)",
            "Person Name: \(title)",
            "Live Webex Sync: \(updatedAt)",
            "Room ID: \(roomID)",
            "Recent messages indexed: \(timelineEvents.count)"
        ]
        if !target.iMessageHandles.isEmpty {
            lines.append("iMessage handles: \(target.iMessageHandles.joined(separator: ", "))")
            let iMessageCount = timelineEvents.filter { $0.source == "iMessage" }.count
            lines.append("iMessage messages indexed: \(iMessageCount)")
        }
        if let iMessageLoadError {
            lines.append("iMessage unavailable: \(iMessageLoadError)")
        }

        if !timelineEvents.isEmpty {
            lines.append("")
            lines.append("Recent conversations (last \(focusDays) days):")
            for event in timelineEvents.prefix(120) {
                lines.append("\(event.source) \(event.id): \(event.createdAt) | \(event.sender) | \(event.sourceTitle) | \(oneLine(event.body))")
            }
        }
        return lines
    }

    private func latestTimelinePreview(_ event: PersonTimelineEvent?, iMessageLoadError: String? = nil) -> String {
        if iMessageLoadError != nil {
            guard let event else {
                return "iMessage unavailable; no Webex messages found."
            }
            return "iMessage unavailable; latest \(event.source): \(oneLine(event.body))"
        }
        guard let event else {
            return "No synced Webex or iMessage messages yet."
        }
        return oneLine(event.body)
    }

    private func personStatusBadge(
        directRoomID: String,
        webexMessages: [MessageRecord],
        iMessages: [IMessageTimelineMessage],
        target: ConfigTarget,
        syncState: WebexConversationSyncStateRecord?,
        iMessageLoadError: String? = nil
    ) -> String {
        let hasWebex = !webexMessages.isEmpty
        let hasIMessage = !iMessages.isEmpty
        if hasWebex && hasIMessage {
            return appendSyncStatusBadge(base: "webex+imessage", state: syncState)
        }
        if hasIMessage {
            return appendSyncStatusBadge(base: "imessage", state: syncState)
        }
        if iMessageLoadError != nil, !target.iMessageHandles.isEmpty {
            let base = directRoomID.isEmpty ? "email-match+imessage-unavailable" : "live-webex+imessage-unavailable"
            return appendSyncStatusBadge(base: base, state: syncState)
        }
        if !target.iMessageHandles.isEmpty {
            let base = directRoomID.isEmpty ? "email-match+imessage-configured" : "live-webex+imessage-configured"
            return appendSyncStatusBadge(base: base, state: syncState)
        }
        let base = directRoomID.isEmpty ? "email-match" : "live-webex"
        return appendSyncStatusBadge(base: base, state: syncState)
    }

    private func latestSyncStatesByRoomID() throws -> [String: WebexConversationSyncStateRecord] {
        let states = try knowledgeStore.loadWebexSyncStates()
        var byRoomID: [String: WebexConversationSyncStateRecord] = [:]
        for state in states {
            let roomID = normalizedRoomID(state.roomID)
            guard !roomID.isEmpty, byRoomID[roomID] == nil else {
                continue
            }
            byRoomID[roomID] = state
        }
        return byRoomID
    }

    private func appendSyncStatusBadge(
        base: String,
        state: WebexConversationSyncStateRecord?
    ) -> String {
        guard let status = syncStatusToken(state: state), !status.isEmpty else {
            return base
        }
        return "\(base)+\(status)"
    }

    private func syncStatusToken(state: WebexConversationSyncStateRecord?) -> String? {
        guard let state else {
            return nil
        }
        if let lastError = state.lastError?.lowercased(), !lastError.isEmpty {
            if lastError.contains("rate-limited") {
                return "delayed-rate-limit"
            }
            if lastError.contains("auth required") || lastError.contains("forbidden") {
                return "auth-required"
            }
            return "offline"
        }
        return "synced"
    }

    private func focusItem(
        id: String,
        title: String,
        subtitle: String,
        meta: String,
        timestamp: String,
        badge: String,
        statusBadge: String,
        roomID: String,
        roomTitle: String,
        messages: [MessageRecord],
        latestKnownMessage: MessageRecord? = nil,
        peopleByID: [String: PersonRecord],
        focusDays: Int,
        updatedAt: String
    ) -> FocusItem {
        FocusItem(
            id: id,
            title: title,
            subtitle: subtitle,
            meta: meta,
            timestamp: timestamp,
            badge: badge,
            statusBadge: statusBadge,
            detailLines: baseDetailLines(
                roomID: roomID,
                roomTitle: roomTitle,
                updatedAt: updatedAt,
                messages: messages,
                latestKnownMessage: latestKnownMessage,
                peopleByID: peopleByID,
                focusDays: focusDays,
                roomTitlesByID: [roomID: roomTitle]
            ),
            detailIntroLines: [],
            detailSections: [],
            detailTailLines: []
        )
    }

    private func baseDetailLines(
        roomID: String,
        roomTitle: String,
        updatedAt: String,
        messages: [MessageRecord],
        latestKnownMessage: MessageRecord?,
        peopleByID: [String: PersonRecord],
        focusDays: Int,
        roomTitlesByID: [String: String]
    ) -> [String] {
        var lines: [String] = [
            "Space Name: \(roomTitle)",
            "Live Webex Sync: \(updatedAt)",
            "Room ID: \(roomID)",
            "Recent messages indexed: \(messages.count)"
        ]
        if !messages.isEmpty {
            lines.append("")
            lines.append("Recent conversations (last \(focusDays) days):")
            for message in messages.prefix(80) {
                let sender = senderLabel(message: message, peopleByID: peopleByID)
                let sourceRoomTitle = roomTitlesByID[message.roomID] ?? roomTitle
                lines.append("Webex \(message.id): \(message.createdAt) | \(sender) | \(sourceRoomTitle) | \(oneLine(message.body))")
            }
        } else if let latestKnownMessage {
            let sender = senderLabel(message: latestKnownMessage, peopleByID: peopleByID)
            let sourceRoomTitle = roomTitlesByID[latestKnownMessage.roomID] ?? roomTitle
            lines.append("No messages were found in the last \(focusDays) day(s).")
            lines.append("Latest known message: \(latestKnownMessage.createdAt) | \(sender) | \(sourceRoomTitle) | \(oneLine(latestKnownMessage.body))")
        }
        return lines
    }

    private func missingRoomItem(target: ConfigTarget, kind: FocusKind, updatedAt: String) -> FocusItem {
        let title = target.label.trimmingCharacters(in: .whitespacesAndNewlines).isEmpty ? target.id : target.label
        return FocusItem(
            id: target.id,
            title: title,
            subtitle: "No Webex room ID configured for this target.",
            meta: "\(autoReplyMeta(target)) | messages=0",
            timestamp: "",
            badge: kind.rawValue,
            statusBadge: "room-id-required",
            detailLines: [
                "Target: \(title)",
                "Live Webex Sync: \(updatedAt)",
                "Missing Webex room ID: configure a room_id for this target before the native app can read channel messages."
            ],
            detailIntroLines: [],
            detailSections: [],
            detailTailLines: []
        )
    }

    private func trackedRoomTargets(
        spaceTargets: [ConfigTarget],
        personTargets: [ConfigTarget],
        beliefTargets: [ConfigTarget],
        roomTitlesByID: [String: String] = [:]
    ) -> [TrackedRoomTarget] {
        var targetsByRoomID: [String: TrackedRoomTarget] = [:]
        for target in spaceTargets + personTargets + beliefTargets {
            let roomID = normalizedRoomID(target.roomID)
            guard !roomID.isEmpty else { continue }
            let candidateTitle = displayTitle(target: target, roomTitlesByID: roomTitlesByID)
            var existing = targetsByRoomID[roomID] ?? TrackedRoomTarget(
                roomID: roomID,
                label: candidateTitle,
                kinds: []
            )
            if shouldPreferTitle(candidateTitle, over: existing.label, roomID: roomID) {
                existing.label = candidateTitle
            }
            existing.kinds.insert(target.kind)
            targetsByRoomID[roomID] = existing
        }
        return targetsByRoomID.values.sorted { $0.label.localizedCaseInsensitiveCompare($1.label) == .orderedAscending }
    }

    private func resolveRoomTitles(
        for targets: [TrackedRoomTarget],
        knownRoomsByID: [String: WebexRoom] = [:],
        allowRoomDetailLookup: Bool = true,
        progress: ((WebexSyncProgress) async -> Void)? = nil
    ) async -> [String: String] {
        let roomIDs = Set(targets.map { normalizedRoomID($0.roomID) }.filter { !$0.isEmpty })
        guard !roomIDs.isEmpty else { return [:] }

        var titlesByID: [String: String] = [:]
        for (index, target) in targets.enumerated() {
            guard webexSyncEnabled() else {
                break
            }
            let roomID = normalizedRoomID(target.roomID)
            guard !roomID.isEmpty, titlesByID[roomID] == nil else { continue }
            await progress?(
                WebexSyncProgress(
                    completedRooms: index,
                    totalRooms: targets.count,
                    fetchedMessages: 0,
                    indexedMessages: 0,
                    currentRoomTitle: "resolving \(target.label)"
                )
            )

            if let room = knownRoomsByID[roomID],
               let title = usableTitle(room.title, roomID: roomID) {
                titlesByID[roomID] = title
                continue
            }

            if allowRoomDetailLookup {
                if let room = try? await webexClient.room(id: roomID),
                   let title = usableTitle(room.title, roomID: roomID) {
                    titlesByID[roomID] = title
                    continue
                }
            }

            if let room = try? knowledgeStore.loadRoom(roomID: roomID),
               let title = usableTitle(room.title, roomID: roomID) {
                titlesByID[roomID] = title
            }
        }

        return titlesByID
    }

    private func deduplicatedTargetsByRoom(_ targets: [ConfigTarget]) -> [ConfigTarget] {
        var seen = Set<String>()
        var values: [ConfigTarget] = []
        for target in targets {
            let roomID = normalizedRoomID(target.roomID)
            guard !roomID.isEmpty, seen.insert(roomID).inserted else {
                continue
            }
            values.append(target)
        }
        return values
    }

    private func displayTitle(target: ConfigTarget, roomTitlesByID: [String: String]) -> String {
        let roomID = normalizedRoomID(target.roomID)
        let label = target.label.trimmingCharacters(in: .whitespacesAndNewlines)
        let resolvedRoomTitle = usableTitle(roomTitlesByID[roomID] ?? "", roomID: roomID)

        if let resolvedRoomTitle {
            if let configuredTitle = usableTitle(label, roomID: roomID),
               shouldPreferConfiguredTitle(configuredTitle, overResolvedTitle: resolvedRoomTitle) {
                return configuredTitle
            }
            return resolvedRoomTitle
        }

        if let title = usableTitle(label, roomID: roomID) {
            return title
        }
        let email = normalizedEmail(target.email)
        if !email.isEmpty {
            return email
        }
        return roomID.isEmpty ? target.id : fallbackRoomTitle(roomID)
    }

    private func autoReplyMeta(_ target: ConfigTarget) -> String {
        "auto-reply=\(target.autoReply ? "yes" : "no")"
    }

    private func usableTitle(_ value: String, roomID: String) -> String? {
        let title = value.trimmingCharacters(in: .whitespacesAndNewlines)
        guard !title.isEmpty else { return nil }
        guard title != roomID else { return nil }
        guard !looksLikeOpaqueWebexRoomID(title) else { return nil }
        return title
    }

    private func shouldPreferTitle(_ candidate: String, over existing: String, roomID: String) -> Bool {
        if usableTitle(existing, roomID: roomID) == nil {
            return usableTitle(candidate, roomID: roomID) != nil
        }

        if isGeneratedFallbackTitle(existing, roomID: roomID),
           let candidateTitle = usableTitle(candidate, roomID: roomID),
           !isGeneratedFallbackTitle(candidateTitle, roomID: roomID) {
            return true
        }

        return false
    }

    private func shouldPreferConfiguredTitle(_ configured: String, overResolvedTitle resolved: String) -> Bool {
        let configuredParticipants = participantNameCount(configured)
        let resolvedParticipants = participantNameCount(resolved)
        if configuredParticipants > resolvedParticipants {
            return true
        }

        if configuredParticipants == resolvedParticipants,
           configuredParticipants >= 2,
           configured.count > resolved.count {
            return true
        }

        return false
    }

    private func participantNameCount(_ value: String) -> Int {
        let parts = value
            .split(separator: ",")
            .map { $0.trimmingCharacters(in: .whitespacesAndNewlines) }
            .filter { !$0.isEmpty }
        guard parts.count >= 2 else {
            return 0
        }

        let likelyNameParts = parts.filter { part in
            let words = part.split(whereSeparator: \.isWhitespace)
            return !words.isEmpty && words.count <= 4
        }
        return likelyNameParts.count
    }

    private func looksLikeOpaqueWebexRoomID(_ value: String) -> Bool {
        let compactValue = normalizedRoomID(value)
        guard !compactValue.isEmpty else { return false }
        return compactValue.hasPrefix("Y2lzY29zcGFyazov") || decodedWebexRoomUUID(compactValue) != nil
    }

    private func fallbackRoomTitle(_ roomID: String) -> String {
        if let uuid = decodedWebexRoomUUID(roomID) {
            return "Webex Space \(uuid.prefix(8))"
        }

        let suffix = String(roomID.suffix(8))
        return suffix.isEmpty ? "Webex Space" : "Webex Space \(suffix)"
    }

    private func isGeneratedFallbackTitle(_ title: String, roomID: String) -> Bool {
        title == fallbackRoomTitle(roomID)
    }

    private func decodedWebexRoomUUID(_ roomID: String) -> String? {
        var encoded = normalizedRoomID(roomID)
            .replacingOccurrences(of: "-", with: "+")
            .replacingOccurrences(of: "_", with: "/")
        let padding = (4 - (encoded.count % 4)) % 4
        if padding > 0 {
            encoded.append(String(repeating: "=", count: padding))
        }

        guard let data = Data(base64Encoded: encoded),
              let decoded = String(data: data, encoding: .utf8),
              decoded.lowercased().contains("/room/"),
              let uuid = decoded.split(separator: "/").last else {
            return nil
        }

        return String(uuid)
    }

    private func latestMessagePreview(
        recentMessage: MessageRecord?,
        latestKnownMessage: MessageRecord? = nil,
        focusDays: Int? = nil
    ) -> String {
        if let recentMessage {
            let preview = oneLine(recentMessage.body)
            if preview.isEmpty {
                return "Latest Webex message has no text body."
            }
            return String(preview.prefix(140))
        }

        if let latestKnownMessage {
            let preview = oneLine(latestKnownMessage.body)
            let body = preview.isEmpty ? "Latest Webex message has no text body." : String(preview.prefix(120))
            if let focusDays {
                return "No messages in last \(focusDays) day(s). Latest: \(body)"
            }
            return "Latest: \(body)"
        }

        return "No synced Webex messages yet."
    }

    private func senderLabel(message: MessageRecord, peopleByID: [String: PersonRecord]) -> String {
        guard let personID = message.personID, !personID.isEmpty else {
            return "unknown sender"
        }
        guard let person = peopleByID[personID] else {
            return personID
        }
        if !person.displayName.trimmingCharacters(in: .whitespacesAndNewlines).isEmpty {
            return person.displayName
        }
        if !person.email.trimmingCharacters(in: .whitespacesAndNewlines).isEmpty {
            return person.email
        }
        return personID
    }

    private func normalizedPersonID(_ personID: String, fallbackEmail: String) -> String {
        let normalized = personID.trimmingCharacters(in: .whitespacesAndNewlines)
        if !normalized.isEmpty {
            return normalized
        }
        return normalizedEmail(fallbackEmail)
    }

    private func normalizedRoomID(_ value: String) -> String {
        value
            .trimmingCharacters(in: .whitespacesAndNewlines)
            .replacingOccurrences(of: #"\s+"#, with: "", options: .regularExpression)
    }

    private func normalizedEmail(_ value: String) -> String {
        value.trimmingCharacters(in: .whitespacesAndNewlines).lowercased()
    }

    private func normalizedMessageText(_ value: String) -> String {
        value
            .replacingOccurrences(of: "\u{00a0}", with: " ")
            .trimmingCharacters(in: .whitespacesAndNewlines)
    }

    private func normalizedTimestamp(_ value: String, fallback: String) -> String {
        let trimmed = value.trimmingCharacters(in: .whitespacesAndNewlines)
        return trimmed.isEmpty ? fallback : trimmed
    }

    private func normalizedOptionalText(_ value: String?) -> String? {
        guard let trimmed = value?.trimmingCharacters(in: .whitespacesAndNewlines),
              !trimmed.isEmpty else {
            return nil
        }
        return trimmed
    }

    private func parsedDate(_ value: String) -> Date? {
        let trimmed = value.trimmingCharacters(in: .whitespacesAndNewlines)
        guard !trimmed.isEmpty else { return nil }
        let fractional = ISO8601DateFormatter()
        fractional.formatOptions = [.withInternetDateTime, .withFractionalSeconds]
        if let date = fractional.date(from: trimmed) {
            return date
        }
        let plain = ISO8601DateFormatter()
        plain.formatOptions = [.withInternetDateTime]
        return plain.date(from: trimmed)
    }

    private func sinceTimestamp(focusDays: Int) -> String {
        let start = Calendar(identifier: .gregorian).date(
            byAdding: .day,
            value: -max(1, focusDays),
            to: Date()
        ) ?? Date()
        return Self.iso8601String(from: start)
    }

    private func sinceDate(focusDays: Int) -> Date {
        Calendar(identifier: .gregorian).date(
            byAdding: .day,
            value: -max(1, focusDays),
            to: Date()
        ) ?? Date()
    }

    private func oneLine(_ value: String) -> String {
        value
            .replacingOccurrences(of: #"\s+"#, with: " ", options: .regularExpression)
            .trimmingCharacters(in: .whitespacesAndNewlines)
    }

    private func cleanMapValue(_ value: String) -> String {
        value
            .replacingOccurrences(of: #"[\r\n\t]+"#, with: " ", options: .regularExpression)
            .trimmingCharacters(in: .whitespacesAndNewlines)
    }

    private func looksLikePersonListSpaceTitle(_ title: String) -> Bool {
        let text = cleanMapValue(title)
        guard !text.isEmpty, text.count <= 120 else { return false }
        guard text.range(of: #"[,+&]"#, options: .regularExpression) != nil else { return false }
        if text.range(
            of: #"\b(team|project|program|review|update|updates|announcement|announcements|customer|working|deal|security|leadership|market|competitive|confidential|squad|exec|forum|discussion|discussions|offer|space|sync|status|incident)\b"#,
            options: [.regularExpression, .caseInsensitive]
        ) != nil {
            return false
        }

        let normalized = text
            .replacingOccurrences(of: "+", with: ",")
            .replacingOccurrences(of: "&", with: ",")
        let parts = normalized
            .split(separator: ",")
            .map { $0.trimmingCharacters(in: .whitespacesAndNewlines) }
            .filter { !$0.isEmpty }
        guard parts.count >= 2, parts.count <= 12 else { return false }
        for part in parts {
            let words = part.split(whereSeparator: \.isWhitespace)
            guard !words.isEmpty, words.count <= 4 else { return false }
            if words.contains(where: { word in
                String(word).range(of: #"[^A-Za-z .'\-]"#, options: .regularExpression) != nil
            }) {
                return false
            }
        }
        return true
    }

    private static func mapTimestamp(from date: Date) -> String {
        let formatter = DateFormatter()
        formatter.dateFormat = "yyyy-MM-dd HH:mm:ss z"
        return formatter.string(from: date)
    }

    private static func iso8601String(from date: Date) -> String {
        let formatter = ISO8601DateFormatter()
        formatter.formatOptions = [.withInternetDateTime, .withFractionalSeconds]
        return formatter.string(from: date)
    }
}
