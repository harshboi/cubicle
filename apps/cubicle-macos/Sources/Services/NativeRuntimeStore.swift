import Foundation

/// Runtime-store failures that should surface as actionable setup/cache problems.
enum NativeRuntimeStoreError: LocalizedError {
    case missingSnapshot(URL)
    case invalidNativeCacheManifest(URL)

    var errorDescription: String? {
        switch self {
        case .missingSnapshot(let url):
            return "Missing migration snapshot: \(url.path)"
        case .invalidNativeCacheManifest(let url):
            return "Native cache manifest is invalid at \(url.path)."
        }
    }
}

/// Local focus-cache rebuild result used by refresh orchestration and job UI.
struct FocusRefreshOutcome: Equatable {
    var kind: FocusKind
    var focusDays: Int
    var sourceSnapshotURL: URL
    var outputSnapshotURL: URL
    var reusedCache: Bool
    var sourceSignature: String
    var normalizedEventCount: Int
    var clusterCount: Int
}

/// Exact-analysis cache state for a focus scope at the current evidence bucket.
enum FocusAnalysisCacheAvailability: String, Equatable {
    case exactCache = "exact_cache"
    case needsRefresh = "needs_refresh"
    case missingRawEvidence = "missing_raw_evidence"
    case invalidManifest = "invalid_manifest"
}

/// Cache identity report for deciding whether a Codex-backed analysis can be reused.
struct FocusAnalysisCacheStatus: Equatable {
    var kind: FocusKind
    var focusDays: Int
    var analysisCadenceHours: Int
    var analysisBucket: Int
    var availability: FocusAnalysisCacheAvailability
    var summary: String
    var cacheID: String?
    var rawEvidenceHash: String?
    var messageIDsHash: String?
    var outputSnapshotPath: String?

    /// True only when source evidence, model settings, prompt version, and bucket all match.
    var canUseExactCache: Bool {
        availability == .exactCache
    }
}

/// Filesystem counts for the native Codex run/queue directories.
struct CodexJobRefreshOutcome: Equatable {
    var runsDirectoryExists: Bool
    var queueDirectoryExists: Bool
    var runDirectoryCount: Int
    var queuedPromptCount: Int
}

/// Sidecar metadata that makes local focus caches reusable without decoding large snapshots first.
private struct NativeFocusCacheManifest: Codable {
    var kind: String
    var focusDays: Int
    var sourceSnapshotPath: String
    var outputSnapshotPath: String
    var sourceSignature: String
    var normalizedEventCount: Int
    var clusterCount: Int
    var cacheReuseVersion: Int
    var sourceFileSizeBytes: Int64
    var sourceModifiedAtEpoch: TimeInterval
    var targetID: String?
    var windowDays: Int?
    var windowStart: String?
    var windowEnd: String?
    var analysisCadenceHours: Int?
    var analysisBucket: Int?
    var rawEvidenceHash: String?
    var messageIDsHash: String?
    var promptVersion: String?
    var model: String?
    var reasoning: String?
    var generationType: String?
    var seedCacheID: String?
    var seedWindowStart: String?
    var seedWindowEnd: String?
    var seedRawEvidenceHash: String?
    var cacheID: String?

    enum CodingKeys: String, CodingKey {
        case kind
        case focusDays
        case sourceSnapshotPath
        case outputSnapshotPath
        case sourceSignature
        case normalizedEventCount
        case clusterCount
        case cacheReuseVersion
        case sourceFileSizeBytes
        case sourceModifiedAtEpoch
        case targetID = "target_id"
        case windowDays = "window_days"
        case windowStart = "window_start"
        case windowEnd = "window_end"
        case analysisCadenceHours = "analysis_cadence_hours"
        case analysisBucket = "analysis_bucket"
        case rawEvidenceHash = "raw_evidence_hash"
        case messageIDsHash = "message_ids_hash"
        case promptVersion = "prompt_version"
        case model
        case reasoning
        case generationType = "generation_type"
        case seedCacheID = "seed_cache_id"
        case seedWindowStart = "seed_window_start"
        case seedWindowEnd = "seed_window_end"
        case seedRawEvidenceHash = "seed_raw_evidence_hash"
        case cacheID = "cache_id"
    }

    init(
        kind: String,
        focusDays: Int,
        sourceSnapshotPath: String,
        outputSnapshotPath: String,
        sourceSignature: String,
        normalizedEventCount: Int,
        clusterCount: Int,
        cacheReuseVersion: Int,
        sourceFileSizeBytes: Int64,
        sourceModifiedAtEpoch: TimeInterval,
        targetID: String? = nil,
        windowDays: Int? = nil,
        windowStart: String? = nil,
        windowEnd: String? = nil,
        analysisCadenceHours: Int? = nil,
        analysisBucket: Int? = nil,
        rawEvidenceHash: String? = nil,
        messageIDsHash: String? = nil,
        promptVersion: String? = nil,
        model: String? = nil,
        reasoning: String? = nil,
        generationType: String? = nil,
        seedCacheID: String? = nil,
        seedWindowStart: String? = nil,
        seedWindowEnd: String? = nil,
        seedRawEvidenceHash: String? = nil,
        cacheID: String? = nil
    ) {
        self.kind = kind
        self.focusDays = focusDays
        self.sourceSnapshotPath = sourceSnapshotPath
        self.outputSnapshotPath = outputSnapshotPath
        self.sourceSignature = sourceSignature
        self.normalizedEventCount = normalizedEventCount
        self.clusterCount = clusterCount
        self.cacheReuseVersion = cacheReuseVersion
        self.sourceFileSizeBytes = sourceFileSizeBytes
        self.sourceModifiedAtEpoch = sourceModifiedAtEpoch
        self.targetID = targetID
        self.windowDays = windowDays
        self.windowStart = windowStart
        self.windowEnd = windowEnd
        self.analysisCadenceHours = analysisCadenceHours
        self.analysisBucket = analysisBucket
        self.rawEvidenceHash = rawEvidenceHash
        self.messageIDsHash = messageIDsHash
        self.promptVersion = promptVersion
        self.model = model
        self.reasoning = reasoning
        self.generationType = generationType
        self.seedCacheID = seedCacheID
        self.seedWindowStart = seedWindowStart
        self.seedWindowEnd = seedWindowEnd
        self.seedRawEvidenceHash = seedRawEvidenceHash
        self.cacheID = cacheID
    }

    init(from decoder: Decoder) throws {
        let container = try decoder.container(keyedBy: CodingKeys.self)
        kind = try container.decode(String.self, forKey: .kind)
        focusDays = try container.decode(Int.self, forKey: .focusDays)
        sourceSnapshotPath = try container.decode(String.self, forKey: .sourceSnapshotPath)
        outputSnapshotPath = try container.decode(String.self, forKey: .outputSnapshotPath)
        sourceSignature = try container.decode(String.self, forKey: .sourceSignature)
        normalizedEventCount = try container.decode(Int.self, forKey: .normalizedEventCount)
        clusterCount = try container.decode(Int.self, forKey: .clusterCount)
        cacheReuseVersion = try container.decodeIfPresent(Int.self, forKey: .cacheReuseVersion) ?? 0
        sourceFileSizeBytes = try container.decodeIfPresent(Int64.self, forKey: .sourceFileSizeBytes) ?? 0
        sourceModifiedAtEpoch = try container.decodeIfPresent(TimeInterval.self, forKey: .sourceModifiedAtEpoch) ?? 0
        targetID = try container.decodeIfPresent(String.self, forKey: .targetID)
        windowDays = try container.decodeIfPresent(Int.self, forKey: .windowDays)
        windowStart = try container.decodeIfPresent(String.self, forKey: .windowStart)
        windowEnd = try container.decodeIfPresent(String.self, forKey: .windowEnd)
        analysisCadenceHours = try container.decodeIfPresent(Int.self, forKey: .analysisCadenceHours)
        analysisBucket = try container.decodeIfPresent(Int.self, forKey: .analysisBucket)
        rawEvidenceHash = try container.decodeIfPresent(String.self, forKey: .rawEvidenceHash)
        messageIDsHash = try container.decodeIfPresent(String.self, forKey: .messageIDsHash)
        promptVersion = try container.decodeIfPresent(String.self, forKey: .promptVersion)
        model = try container.decodeIfPresent(String.self, forKey: .model)
        reasoning = try container.decodeIfPresent(String.self, forKey: .reasoning)
        generationType = try container.decodeIfPresent(String.self, forKey: .generationType)
        seedCacheID = try container.decodeIfPresent(String.self, forKey: .seedCacheID)
        seedWindowStart = try container.decodeIfPresent(String.self, forKey: .seedWindowStart)
        seedWindowEnd = try container.decodeIfPresent(String.self, forKey: .seedWindowEnd)
        seedRawEvidenceHash = try container.decodeIfPresent(String.self, forKey: .seedRawEvidenceHash)
        cacheID = try container.decodeIfPresent(String.self, forKey: .cacheID)
    }
}

/// Cheap filesystem fingerprint used before falling back to content signatures.
private struct FocusSourceSnapshotFingerprint: Equatable {
    var fileSizeBytes: Int64
    var modifiedAtEpoch: TimeInterval
}

/// Owns native runtime files under `runtimeRoot/knowledge`.
///
/// This is the boundary between raw source snapshots, display-ready focus caches,
/// and cache manifests consumed by refresh orchestration.
final class NativeRuntimeStore {
    let configuration: RuntimeConfiguration
    let configStore: ConfigStore
    private let decoder: JSONDecoder
    private let encoder: JSONEncoder
    private let fileManager = FileManager.default
    // Sparse fallback detection is only useful for tiny live snapshots that can
    // represent partial shell output. Skip expensive full-cache decoding for
    // larger snapshots during startup.
    private static let sparseLiveSnapshotInspectionMaxBytes: Int64 = 256 * 1024
    private static let sparseFallbackCandidateDecodeMaxBytes: Int64 = 2 * 1024 * 1024
    private static let maxReusableNativeCacheBytes: Int64 = 32 * 1024 * 1024
    private static let maxEnrichmentCandidateDecodeBytes: Int64 = 8 * 1024 * 1024
    private static let personFocusCacheReuseVersion = 13
    private static let spaceFocusCacheReuseVersion = 12

    /// Creates a store bound to one runtime root and config source.
    init(configuration: RuntimeConfiguration = .current) {
        self.configuration = configuration
        self.configStore = ConfigStore(configuration: configuration)
        self.decoder = JSONDecoder()
        self.encoder = JSONEncoder()
        self.encoder.outputFormatting = [.prettyPrinted, .sortedKeys]
    }

    /// Live user-facing snapshot path for the configured focus window.
    func snapshotURL(kind: FocusKind) -> URL {
        snapshotURL(kind: kind, focusDays: configuredFocusDays(kind: kind))
    }

    /// User-facing snapshot path for a specific focus window.
    func snapshotURL(kind: FocusKind, focusDays: Int) -> URL {
        configuration.runtimeRoot
            .appendingPathComponent("knowledge", isDirectory: true)
            .appendingPathComponent(kind.snapshotFilename(days: focusDays))
    }

    /// Native display-ready cache path for a specific focus window.
    func nativeSnapshotURL(kind: FocusKind, focusDays: Int) -> URL {
        configuration.runtimeRoot
            .appendingPathComponent("knowledge", isDirectory: true)
            .appendingPathComponent("native", isDirectory: true)
            .appendingPathComponent("\(kind.rawValue)_focus_cache_\(focusDays)d.native.json")
    }

    /// Legacy/latest manifest path kept for compatibility with older cache writers.
    func nativeManifestURL(kind: FocusKind) -> URL {
        configuration.runtimeRoot
            .appendingPathComponent("knowledge", isDirectory: true)
            .appendingPathComponent("native", isDirectory: true)
            .appendingPathComponent("\(kind.rawValue)_focus_cache_manifest.json")
    }

    /// Window-scoped manifest path used for exact cache reuse.
    func nativeManifestURL(kind: FocusKind, focusDays: Int) -> URL {
        configuration.runtimeRoot
            .appendingPathComponent("knowledge", isDirectory: true)
            .appendingPathComponent("native", isDirectory: true)
            .appendingPathComponent("\(kind.rawValue)_focus_cache_\(focusDays)d.manifest.json")
    }

    /// Checks whether Codex analysis output matches current evidence and settings.
    func focusAnalysisCacheStatus(
        kind: FocusKind,
        focusDays: Int,
        analysisCadenceHours: Int,
        now: Date = Date()
    ) -> FocusAnalysisCacheStatus {
        let boundedFocusDays = SystemSettings.clamped(focusDays, to: SystemSettings.focusDaysBounds)
        let boundedCadenceHours = SystemSettings.clamped(
            analysisCadenceHours,
            to: SystemSettings.focusAnalysisCadenceHoursBounds
        )
        let bucket = analysisBucket(for: now, cadenceHours: boundedCadenceHours)
        let baseStatus = FocusAnalysisCacheStatus(
            kind: kind,
            focusDays: boundedFocusDays,
            analysisCadenceHours: boundedCadenceHours,
            analysisBucket: bucket,
            availability: .needsRefresh,
            summary: "",
            cacheID: nil,
            rawEvidenceHash: nil,
            messageIDsHash: nil,
            outputSnapshotPath: nil
        )

        do {
            let sourceURL = try exactSourceURL(kind: kind, focusDays: boundedFocusDays)
            let sourceCache = try loadFocusCache(kind: kind, sourceURL: sourceURL)
            let sourceSignature = sourceCache.sourceSignature(kind: kind)
            let messageIDsHash = sourceCache.messageIDsSignature(kind: kind)
            guard let manifest = try? loadManifest(kind: kind, focusDays: boundedFocusDays) else {
                return status(
                    baseStatus,
                    availability: .needsRefresh,
                    summary: "\(kind.title) needs refresh: no exact \(boundedFocusDays)-day analysis manifest.",
                    rawEvidenceHash: sourceSignature,
                    messageIDsHash: messageIDsHash
                )
            }
            guard exactAnalysisManifestMatches(
                manifest,
                kind: kind,
                focusDays: boundedFocusDays,
                analysisCadenceHours: boundedCadenceHours,
                analysisBucket: bucket,
                rawEvidenceHash: sourceSignature,
                messageIDsHash: messageIDsHash
            ) else {
                return status(
                    baseStatus,
                    availability: .needsRefresh,
                    summary: "\(kind.title) needs refresh: exact \(boundedFocusDays)-day cache is missing, stale, or incompatible.",
                    cacheID: manifest.cacheID,
                    rawEvidenceHash: sourceSignature,
                    messageIDsHash: messageIDsHash,
                    outputSnapshotPath: manifest.outputSnapshotPath
                )
            }
            return status(
                baseStatus,
                availability: .exactCache,
                summary: "\(kind.title) exact cache ready for \(boundedFocusDays) days and \(boundedCadenceHours)h bucket \(bucket).",
                cacheID: manifest.cacheID,
                rawEvidenceHash: sourceSignature,
                messageIDsHash: messageIDsHash,
                outputSnapshotPath: manifest.outputSnapshotPath
            )
        } catch NativeRuntimeStoreError.missingSnapshot {
            return status(
                baseStatus,
                availability: .missingRawEvidence,
                summary: "\(kind.title) needs refresh: no raw/source snapshot for \(boundedFocusDays) days."
            )
        } catch {
            return status(
                baseStatus,
                availability: .invalidManifest,
                summary: "\(kind.title) needs refresh: \(error.localizedDescription)"
            )
        }
    }

    /// Loads an exact analysis cache only when the manifest proves it is current.
    func loadExactFocusAnalysisCache(
        kind: FocusKind,
        focusDays: Int,
        analysisCadenceHours: Int,
        now: Date = Date()
    ) throws -> FocusCache? {
        let status = focusAnalysisCacheStatus(
            kind: kind,
            focusDays: focusDays,
            analysisCadenceHours: analysisCadenceHours,
            now: now
        )
        guard status.canUseExactCache,
              let outputSnapshotPath = status.outputSnapshotPath else {
            return nil
        }
        return try loadFocusCache(kind: kind, sourceURL: URL(fileURLWithPath: outputSnapshotPath))
    }

    /// Loads the best display-ready focus cache for the configured focus window.
    func loadFocusCache(kind: FocusKind) throws -> FocusCache {
        let sourceURL = try resolvedSnapshotURL(kind: kind)
        let sourceFocusDays = focusDaysFromSnapshotURL(sourceURL) ?? configuredFocusDays(kind: kind)
        let reuseVersion = cacheReuseVersion(kind: kind)
        let currentSourceFingerprint = try? sourceFingerprint(url: sourceURL)
        if let sourceFingerprint = currentSourceFingerprint,
           let manifest = try? loadManifest(kind: kind, focusDays: sourceFocusDays),
           canReuseByFingerprint(
            manifest: manifest,
            kind: kind,
            sourceURL: sourceURL,
            sourceFingerprint: sourceFingerprint,
            cacheReuseVersion: reuseVersion
           ) {
            let nativeURL = URL(fileURLWithPath: manifest.outputSnapshotPath)
            if canReadReusableNativeCache(at: nativeURL) {
                return try loadFocusCache(kind: kind, sourceURL: nativeURL)
            }
        }
        let sourceCache = try loadFocusCache(kind: kind, sourceURL: sourceURL)
        let signature = sourceCache.sourceSignature(kind: kind)
        let messageIDsHash = sourceCache.messageIDsSignature(kind: kind)
        let outputURL = nativeSnapshotURL(kind: kind, focusDays: sourceCache.focusDays)
        if let sourceFingerprint = currentSourceFingerprint,
           let manifest = try? loadManifest(kind: kind, focusDays: sourceCache.focusDays),
           canReuseBySignature(
            manifest: manifest,
            kind: kind,
            focusDays: sourceCache.focusDays,
            outputURL: outputURL,
            sourceSignature: signature,
            cacheReuseVersion: reuseVersion
           ) {
            try? rewriteManifestForReuse(
                manifest: manifest,
                kind: kind,
                sourceURL: sourceURL,
                sourceFingerprint: sourceFingerprint,
                focusDays: sourceCache.focusDays,
                outputURL: outputURL,
                sourceSignature: signature,
                messageIDsHash: messageIDsHash
            )
            if canReadReusableNativeCache(at: outputURL) {
                return try loadFocusCache(kind: kind, sourceURL: outputURL)
            }
        }
        return displayReadyCache(
            sourceCache,
            kind: kind,
            enrichmentCache: bestEnrichmentCache(
                kind: kind,
                excluding: sourceURL,
                matchingFocusDays: sourceCache.focusDays
            )
        )
    }

    private func status(
        _ base: FocusAnalysisCacheStatus,
        availability: FocusAnalysisCacheAvailability,
        summary: String,
        cacheID: String? = nil,
        rawEvidenceHash: String? = nil,
        messageIDsHash: String? = nil,
        outputSnapshotPath: String? = nil
    ) -> FocusAnalysisCacheStatus {
        FocusAnalysisCacheStatus(
            kind: base.kind,
            focusDays: base.focusDays,
            analysisCadenceHours: base.analysisCadenceHours,
            analysisBucket: base.analysisBucket,
            availability: availability,
            summary: summary,
            cacheID: cacheID,
            rawEvidenceHash: rawEvidenceHash,
            messageIDsHash: messageIDsHash,
            outputSnapshotPath: outputSnapshotPath
        )
    }

    /// Writes the user-facing snapshot before native cache regeneration.
    func saveFocusCache(_ cache: FocusCache, kind: FocusKind) throws {
        try ensureKnowledgeDirectory()
        let data = try encoder.encode(cache)
        try data.write(to: snapshotURL(kind: kind), options: [.atomic])
    }

    /// Rebuilds or reuses the native cache from the best available source snapshot.
    func refreshFocusCache(kind: FocusKind, forceRebuild: Bool = false) throws -> FocusRefreshOutcome {
        try refreshFocusCache(
            kind: kind,
            sourceURL: refreshSourceURL(kind: kind),
            forceRebuild: forceRebuild
        )
    }

    /// Rebuilds or reuses the native cache from an explicit source snapshot.
    func refreshFocusCache(
        kind: FocusKind,
        sourceURL: URL,
        forceRebuild: Bool = false
    ) throws -> FocusRefreshOutcome {
        let reuseVersion = cacheReuseVersion(kind: kind)
        let sourceFingerprint = try sourceFingerprint(url: sourceURL)
        let sourceFocusDays = focusDaysFromSnapshotURL(sourceURL) ?? configuredFocusDays(kind: kind)

        if !forceRebuild,
           let manifest = try? loadManifest(kind: kind, focusDays: sourceFocusDays),
           canReuseByFingerprint(
            manifest: manifest,
            kind: kind,
            sourceURL: sourceURL,
            sourceFingerprint: sourceFingerprint,
            cacheReuseVersion: reuseVersion
           ) {
            return FocusRefreshOutcome(
                kind: kind,
                focusDays: manifest.focusDays,
                sourceSnapshotURL: sourceURL,
                outputSnapshotURL: URL(fileURLWithPath: manifest.outputSnapshotPath),
                reusedCache: true,
                sourceSignature: manifest.sourceSignature,
                normalizedEventCount: manifest.normalizedEventCount,
                clusterCount: manifest.clusterCount
            )
        }

        let sourceCache = try loadFocusCache(kind: kind, sourceURL: sourceURL)
        let signature = sourceCache.sourceSignature(kind: kind)
        let messageIDsHash = sourceCache.messageIDsSignature(kind: kind)
        let previousEnrichmentByID = enrichmentIndex(
            items: bestEnrichmentCache(
                kind: kind,
                excluding: sourceURL,
                matchingFocusDays: sourceCache.focusDays
            )?.items ?? [],
            kind: kind
        )
        let outputURL = nativeSnapshotURL(kind: kind, focusDays: sourceCache.focusDays)

        if !forceRebuild,
           let manifest = try? loadManifest(kind: kind, focusDays: sourceCache.focusDays),
           canReuseBySignature(
            manifest: manifest,
            kind: kind,
            focusDays: sourceCache.focusDays,
            outputURL: outputURL,
            sourceSignature: signature,
            cacheReuseVersion: reuseVersion
           ) {
            try rewriteManifestForReuse(
                manifest: manifest,
                kind: kind,
                sourceURL: sourceURL,
                sourceFingerprint: sourceFingerprint,
                focusDays: sourceCache.focusDays,
                outputURL: outputURL,
                sourceSignature: signature,
                messageIDsHash: messageIDsHash
            )
            return FocusRefreshOutcome(
                kind: kind,
                focusDays: sourceCache.focusDays,
                sourceSnapshotURL: sourceURL,
                outputSnapshotURL: outputURL,
                reusedCache: true,
                sourceSignature: signature,
                normalizedEventCount: manifest.normalizedEventCount,
                clusterCount: manifest.clusterCount
            )
        }

        var refreshedItems: [FocusItem] = []
        refreshedItems.reserveCapacity(sourceCache.items.count)

        var normalizedEventCount = 0
        var clusterCount = 0

        for item in sourceCache.items {
            let events = item.normalizedEvents(kind: kind)
            normalizedEventCount += events.count
            let seeds = FocusClusterSeed.makeSeeds(kind: kind, events: events)
            clusterCount += seeds.count
            let freshItem = displayReadyItem(item, kind: kind, focusDays: sourceCache.focusDays, seeds: seeds)
            refreshedItems.append(
                mergeFreshItem(
                    freshItem,
                    with: previousEnrichmentByID[canonicalFocusItemID(item.id, kind: kind)],
                    kind: kind
                )
            )
        }

        let refreshedCache = FocusCache(
            focusDays: sourceCache.focusDays,
            items: refreshedItems,
            updatedAt: sourceCache.updatedAt,
            countLabel: sourceCache.countLabel,
            recentMessages: sourceCache.recentMessages,
            summaryGenerationInProgress: sourceCache.summaryGenerationInProgress,
            subjectsProcessed: sourceCache.subjectsProcessed,
            subjectsTotal: sourceCache.subjectsTotal
        )

        try ensureNativeCacheDirectory()
        let cacheData = try encoder.encode(refreshedCache)
        try cacheData.write(to: outputURL, options: [.atomic])

        let manifest = makeManifest(
            kind: kind,
            focusDays: sourceCache.focusDays,
            sourceURL: sourceURL,
            outputURL: outputURL,
            sourceSignature: signature,
            messageIDsHash: messageIDsHash,
            normalizedEventCount: normalizedEventCount,
            clusterCount: clusterCount,
            sourceFingerprint: sourceFingerprint
        )
        try writeManifest(manifest, kind: kind)

        return FocusRefreshOutcome(
            kind: kind,
            focusDays: sourceCache.focusDays,
            sourceSnapshotURL: sourceURL,
            outputSnapshotURL: outputURL,
            reusedCache: false,
            sourceSignature: signature,
            normalizedEventCount: normalizedEventCount,
            clusterCount: clusterCount
        )
    }

    /// Counts run and queue directories without mutating job state.
    func refreshCodexJobs() -> CodexJobRefreshOutcome {
        let knowledgeURL = configuration.runtimeRoot.appendingPathComponent("knowledge", isDirectory: true)
        let runsURL = knowledgeURL.appendingPathComponent("runs", isDirectory: true)
        let queueURL = knowledgeURL.appendingPathComponent("queue", isDirectory: true)

        let fileManager = FileManager.default
        let runsExists = fileManager.fileExists(atPath: runsURL.path)
        let queueExists = fileManager.fileExists(atPath: queueURL.path)

        let runDirectoryCount: Int
        if runsExists {
            let entries = (try? fileManager.contentsOfDirectory(at: runsURL, includingPropertiesForKeys: [.isDirectoryKey])) ?? []
            runDirectoryCount = entries.filter { url in
                ((try? url.resourceValues(forKeys: [.isDirectoryKey]).isDirectory) ?? false) == true
            }.count
        } else {
            runDirectoryCount = 0
        }

        let queuedPromptCount: Int
        if queueExists {
            let entries = (try? fileManager.contentsOfDirectory(at: queueURL, includingPropertiesForKeys: [.isRegularFileKey])) ?? []
            queuedPromptCount = entries.filter { url in
                ((try? url.resourceValues(forKeys: [.isRegularFileKey]).isRegularFile) ?? false) == true
            }.count
        } else {
            queuedPromptCount = 0
        }

        return CodexJobRefreshOutcome(
            runsDirectoryExists: runsExists,
            queueDirectoryExists: queueExists,
            runDirectoryCount: runDirectoryCount,
            queuedPromptCount: queuedPromptCount
        )
    }

    /// Person-focus convenience wrapper for refresh orchestration.
    func refreshPersonFocusCache(forceRebuild: Bool = false) throws -> FocusRefreshOutcome {
        try refreshFocusCache(kind: .person, forceRebuild: forceRebuild)
    }

    /// Space-focus convenience wrapper for refresh orchestration.
    func refreshSpaceFocusCache(forceRebuild: Bool = false) throws -> FocusRefreshOutcome {
        try refreshFocusCache(kind: .space, forceRebuild: forceRebuild)
    }

    /// Reports runtime file presence for Settings diagnostics.
    func runtimeStatus() -> RuntimeStatus {
        let knowledgeURL = configuration.runtimeRoot.appendingPathComponent("knowledge", isDirectory: true)
        let nativeURL = knowledgeURL.appendingPathComponent("native", isDirectory: true)
        let knowledgeExists = directoryExists(at: knowledgeURL)
        let spaceSnapshotExists = hasSnapshotForRuntimeStatus(
            kind: .space,
            knowledgeURL: knowledgeURL,
            nativeURL: nativeURL
        )
        let personSnapshotExists = hasSnapshotForRuntimeStatus(
            kind: .person,
            knowledgeURL: knowledgeURL,
            nativeURL: nativeURL
        )
        return RuntimeStatus(
            runtimeRoot: configuration.runtimeRoot,
            knowledgeDirectoryExists: knowledgeExists,
            spaceSnapshotExists: spaceSnapshotExists,
            personSnapshotExists: personSnapshotExists,
            codexExecutable: configuration.codexExecutable
        )
    }

    private func directoryExists(at url: URL) -> Bool {
        var isDirectory = ObjCBool(false)
        guard fileManager.fileExists(atPath: url.path, isDirectory: &isDirectory) else {
            return false
        }
        return isDirectory.boolValue
    }

    private func hasSnapshotForRuntimeStatus(
        kind: FocusKind,
        knowledgeURL: URL,
        nativeURL: URL
    ) -> Bool {
        let directCandidates = [
            snapshotURL(kind: kind),
            liveSnapshotURL(kind: kind),
            knowledgeURL.appendingPathComponent(kind.snapshotFilename()),
            nativeURL.appendingPathComponent("live_\(kind.snapshotFilename())")
        ]
        if directCandidates.contains(where: { fileManager.fileExists(atPath: $0.path) }) {
            return true
        }

        return hasMatchingSnapshotFileForRuntimeStatus(in: knowledgeURL, kind: kind)
            || hasMatchingSnapshotFileForRuntimeStatus(in: nativeURL, kind: kind)
    }

    private func hasMatchingSnapshotFileForRuntimeStatus(in directory: URL, kind: FocusKind) -> Bool {
        guard let entries = try? fileManager.contentsOfDirectory(
            at: directory,
            includingPropertiesForKeys: nil,
            options: [.skipsHiddenFiles]
        ) else {
            return false
        }
        let prefix = "\(kind.rawValue)_focus_cache_"
        let livePrefix = "live_\(prefix)"
        return entries.contains { url in
            let filename = url.lastPathComponent.lowercased()
            return (filename.hasPrefix(prefix) || filename.hasPrefix(livePrefix)) && filename.hasSuffix(".json")
        }
    }

    private func ensureNativeCacheDirectory() throws {
        let directory = configuration.runtimeRoot
            .appendingPathComponent("knowledge", isDirectory: true)
            .appendingPathComponent("native", isDirectory: true)
        try FileManager.default.createDirectory(at: directory, withIntermediateDirectories: true)
    }

    private func ensureKnowledgeDirectory() throws {
        let directory = configuration.runtimeRoot.appendingPathComponent("knowledge", isDirectory: true)
        try FileManager.default.createDirectory(at: directory, withIntermediateDirectories: true)
    }

    private func loadFocusCache(kind: FocusKind, sourceURL: URL) throws -> FocusCache {
        guard fileManager.fileExists(atPath: sourceURL.path) else {
            throw NativeRuntimeStoreError.missingSnapshot(sourceURL)
        }
        let data = try Data(contentsOf: sourceURL)
        var cache = try decoder.decode(FocusCache.self, from: data)
        if let filenameFocusDays = focusDaysFromSnapshotURL(sourceURL) {
            cache.focusDays = filenameFocusDays
        }
        return cache
    }

    private func focusDaysFromSnapshotURL(_ url: URL) -> Int? {
        guard let range = url.lastPathComponent.range(of: "_focus_cache_") else {
            return nil
        }
        let suffix = url.lastPathComponent[range.upperBound...]
        let digits = suffix.prefix { $0.isNumber }
        guard !digits.isEmpty, let days = Int(digits), days > 0 else {
            return nil
        }
        return days
    }

    /// Converts raw snapshots into UI-ready detail payloads while preserving useful Codex sections.
    private func displayReadyCache(
        _ cache: FocusCache,
        kind: FocusKind,
        enrichmentCache: FocusCache? = nil
    ) -> FocusCache {
        let previousEnrichmentByID = enrichmentIndex(
            items: enrichmentCache?.items ?? [],
            kind: kind
        )
        let items = cache.items.map { item -> FocusItem in
            let events = item.normalizedEvents(kind: kind)
            let seeds = FocusClusterSeed.makeSeeds(kind: kind, events: events)
            let freshItem = displayReadyItem(item, kind: kind, focusDays: cache.focusDays, seeds: seeds)
            return mergeFreshItem(
                freshItem,
                with: previousEnrichmentByID[canonicalFocusItemID(item.id, kind: kind)],
                kind: kind
            )
        }

        return FocusCache(
            focusDays: cache.focusDays,
            items: items,
            updatedAt: cache.updatedAt,
            countLabel: cache.countLabel,
            recentMessages: cache.recentMessages,
            summaryGenerationInProgress: cache.summaryGenerationInProgress,
            subjectsProcessed: cache.subjectsProcessed,
            subjectsTotal: cache.subjectsTotal
        )
    }

    /// Finds the richest compatible prior cache for carrying forward generated detail sections.
    private func bestEnrichmentCache(
        kind: FocusKind,
        excluding sourceURL: URL? = nil,
        matchingFocusDays: Int? = nil
    ) -> FocusCache? {
        struct Candidate {
            var cache: FocusCache
            var score: Int
            var modifiedAt: Date
        }

        let candidateFocusDays = matchingFocusDays ?? configuredFocusDays(kind: kind)
        var candidateURLs: [URL] = [
            nativeSnapshotURL(kind: kind, focusDays: candidateFocusDays)
        ]
        if candidateFocusDays == configuredFocusDays(kind: kind) {
            candidateURLs.append(snapshotURL(kind: kind))
        }
        if let manifest = try? loadManifest(kind: kind, focusDays: candidateFocusDays) {
            candidateURLs.append(URL(fileURLWithPath: manifest.outputSnapshotPath))
        } else if matchingFocusDays == nil,
                  let manifest = try? loadManifest(kind: kind) {
            candidateURLs.append(URL(fileURLWithPath: manifest.outputSnapshotPath))
        }

        var seenPaths = Set<String>()
        let excludedPath = sourceURL?.standardizedFileURL.path
        let candidates = candidateURLs.compactMap { url -> Candidate? in
            let path = url.standardizedFileURL.path
            guard seenPaths.insert(path).inserted else { return nil }
            guard path != excludedPath else { return nil }
            guard fileManager.fileExists(atPath: path) else { return nil }
            guard !isSelfSourcedNativeSnapshot(url, kind: kind) else { return nil }
            if let sizeBytes = fileSizeBytes(for: url),
               sizeBytes > Self.maxEnrichmentCandidateDecodeBytes {
                return nil
            }
            guard let cache = try? loadFocusCache(kind: kind, sourceURL: url) else { return nil }
            if let matchingFocusDays,
               cache.focusDays != matchingFocusDays {
                return nil
            }
            let score = preservationScore(cache: cache, kind: kind)
            guard score > 0 else { return nil }
            let modifiedAt = (try? url.resourceValues(forKeys: [.contentModificationDateKey]).contentModificationDate) ?? .distantPast
            return Candidate(cache: cache, score: score, modifiedAt: modifiedAt)
        }

        return candidates.max { lhs, rhs in
            if lhs.score != rhs.score {
                return lhs.score < rhs.score
            }
            return lhs.modifiedAt < rhs.modifiedAt
        }?.cache
    }

    private func enrichmentScore(cache: FocusCache, kind: FocusKind) -> Int {
        cache.items.reduce(0) { partial, item in
            partial + enrichmentScore(item: item, kind: kind)
        }
    }

    private func preservationScore(cache: FocusCache, kind: FocusKind) -> Int {
        cache.items.reduce(0) { partial, item in
            partial + preservationScore(item: item, kind: kind)
        }
    }

    private func preservationScore(item: FocusItem, kind: FocusKind) -> Int {
        let metaCount = messageCount(from: item.meta) ?? 0
        let eventCount = item.normalizedEvents(kind: kind).count
        return enrichmentScore(item: item, kind: kind) * 10 + min(max(metaCount, eventCount), 1_000)
    }

    private func enrichmentScore(item: FocusItem, kind: FocusKind) -> Int {
        var score = 0
        if kind == .space {
            if !item.firstDetailLine(prefix: "Space summary:").isEmpty { score += 5 }
            if !item.firstDetailLine(prefix: "Current posture / next move:").isEmpty { score += 5 }
            if !item.firstDetailLine(prefix: "Guidance freshness:").isEmpty { score += 3 }
        } else {
            if !item.firstDetailLine(prefix: "Person summary:").isEmpty { score += 5 }
            if !item.detailTailLines.isEmpty { score += 5 }
        }
        score += item.detailSections.filter(shouldPreserveGeneratedSection).count
        return score
    }

    /// Merges new raw evidence with prior generated summaries when evidence has not advanced.
    private func mergeFreshItem(_ freshItem: FocusItem, with previousItem: FocusItem?, kind: FocusKind) -> FocusItem {
        if let previousItem,
           shouldPreferPreviousNonEmptyItem(freshItem: freshItem, previousItem: previousItem, kind: kind) {
            return previousItem
        }
        guard let previousItem,
              enrichmentScore(item: previousItem, kind: kind) > 0 else {
            return freshItem
        }
        guard shouldPreservePreviousEnrichment(
            freshItem: freshItem,
            previousItem: previousItem,
            kind: kind
        ) else {
            return freshItem
        }

        let preserveFullPreviousPayload = shouldPreferPreviousRichPayload(
            freshItem: freshItem,
            previousItem: previousItem
        )
        let generatedPreviousSections = previousItem.detailSections.filter(shouldPreserveGeneratedSection)
        let preservedSections: [FocusDetailSection]
        if preserveFullPreviousPayload,
           generatedPreviousSections.count == previousItem.detailSections.count {
            preservedSections = previousItem.detailSections
        } else {
            preservedSections = generatedPreviousSections
        }
        let freshSections = freshItem.detailSections.filter { section in
            !preservedSections.contains { $0.id == section.id || $0.header == section.header }
        }
        let sections = preservedSections + freshSections

        let introLines = mergedIntroLines(
            freshItem: freshItem,
            previousItem: previousItem,
            kind: kind,
            includePreviousIntro: preserveFullPreviousPayload
        )
        let tailLines = preferredPayloadLines(
            primary: previousItem.detailTailLines,
            fallback: freshItem.detailTailLines
        )
        let detailLines = flattenFocusDetailLines(
            intro: introLines,
            sections: sections,
            tail: tailLines
        )

        return FocusItem(
            id: freshItem.id,
            title: freshItem.title,
            subtitle: freshItem.subtitle,
            meta: freshItem.meta,
            timestamp: freshItem.timestamp,
            badge: freshItem.badge,
            statusBadge: freshItem.statusBadge,
            detailLines: detailLines,
            detailIntroLines: introLines,
            detailSections: sections,
            detailTailLines: tailLines
        )
    }

    private func shouldPreservePreviousEnrichment(
        freshItem: FocusItem,
        previousItem: FocusItem,
        kind: FocusKind
    ) -> Bool {
        guard hasGeneratedCodexSummary(previousItem, kind: kind) else {
            return true
        }

        let generatedAt = generatedSummaryDate(previousItem)
        let latestEvidenceAt = latestEvidenceDate(freshItem, kind: kind)
        if let generatedAt, let latestEvidenceAt, latestEvidenceAt > generatedAt {
            return false
        }

        if let freshCount = messageCount(from: freshItem.meta),
           let previousCount = messageCount(from: previousItem.meta),
           freshCount > previousCount {
            return false
        }

        return true
    }

    private func hasGeneratedCodexSummary(_ item: FocusItem, kind: FocusKind) -> Bool {
        switch kind {
        case .space:
            return !item.firstDetailLine(prefix: "Space summary:").isEmpty
                || !item.firstDetailLine(prefix: "Current posture / next move:").isEmpty
                || !item.firstDetailLine(prefix: "Space summary source:").isEmpty
        case .person:
            return !item.firstDetailLine(prefix: "Person summary:").isEmpty
                || !item.firstDetailLine(prefix: "Summary source:").isEmpty
        }
    }

    private func generatedSummaryDate(_ item: FocusItem) -> Date? {
        let candidates = [
            item.firstDetailLine(prefix: "Summary generated:"),
            item.firstDetailLine(prefix: "Analysis Generated:")
        ]
        return candidates.lazy.compactMap(parseFocusDate).first
    }

    private func latestEvidenceDate(_ item: FocusItem, kind: FocusKind) -> Date? {
        var dates: [Date] = []
        if let date = parseFocusDate(item.timestamp) {
            dates.append(date)
        }
        let detailCandidates = [
            item.firstDetailLine(prefix: "Latest room message:"),
            item.firstDetailLine(prefix: "Latest Message:")
        ]
        dates.append(contentsOf: detailCandidates.compactMap(parseFocusDate))
        dates.append(contentsOf: item.normalizedEvents(kind: kind).compactMap { parseFocusDate($0.timestampLabel) })
        return dates.max()
    }

    private func messageCount(from meta: String) -> Int? {
        guard let range = meta.range(of: "messages=", options: .caseInsensitive) else {
            return nil
        }
        let suffix = meta[range.upperBound...]
        let digits = suffix.prefix { $0.isNumber }
        guard !digits.isEmpty else {
            return nil
        }
        return Int(digits)
    }

    private func shouldPreferPreviousNonEmptyItem(
        freshItem: FocusItem,
        previousItem: FocusItem,
        kind: FocusKind
    ) -> Bool {
        if let freshCount = messageCount(from: freshItem.meta),
           let previousCount = messageCount(from: previousItem.meta),
           freshCount == 0,
           previousCount > 0 {
            return true
        }

        let freshEvents = freshItem.normalizedEvents(kind: kind)
        let previousEvents = previousItem.normalizedEvents(kind: kind)
        if freshEvents.isEmpty,
           !previousEvents.isEmpty,
           isNoSyncedMessagesSubtitle(freshItem.subtitle) {
            return true
        }

        return false
    }

    private func isNoSyncedMessagesSubtitle(_ subtitle: String) -> Bool {
        let normalized = subtitle
            .trimmingCharacters(in: .whitespacesAndNewlines)
            .lowercased()
        return normalized.contains("no synced webex messages yet")
            || normalized.contains("no synced webex or imessage messages yet")
    }

    private func parseFocusDate(_ rawValue: String?) -> Date? {
        guard let rawValue else { return nil }
        let value = rawValue.trimmingCharacters(in: .whitespacesAndNewlines)
        guard !value.isEmpty else { return nil }
        if let date = Self.iso8601WithFractionalSeconds.date(from: value) {
            return date
        }
        if let date = Self.iso8601.date(from: value) {
            return date
        }
        return Self.focusDateFormatters.lazy.compactMap { $0.date(from: value) }.first
    }

    private static let iso8601WithFractionalSeconds: ISO8601DateFormatter = {
        let formatter = ISO8601DateFormatter()
        formatter.formatOptions = [.withInternetDateTime, .withFractionalSeconds]
        return formatter
    }()

    private static let iso8601: ISO8601DateFormatter = {
        let formatter = ISO8601DateFormatter()
        formatter.formatOptions = [.withInternetDateTime]
        return formatter
    }()

    private static let focusDateFormatters: [DateFormatter] = {
        let formats = [
            "yyyy-MM-dd HH:mm:ss zzz",
            "yyyy-MM-dd HH:mm zzz",
            "MM/dd/yyyy HH:mm:ss zzz",
            "MM/dd/yyyy HH:mm zzz"
        ]
        return formats.map { format in
            let formatter = DateFormatter()
            formatter.locale = Locale(identifier: "en_US_POSIX")
            formatter.dateFormat = format
            return formatter
        }
    }()

    private func mergedIntroLines(freshItem: FocusItem, previousItem: FocusItem, kind: FocusKind) -> [String] {
        mergedIntroLines(
            freshItem: freshItem,
            previousItem: previousItem,
            kind: kind,
            includePreviousIntro: false
        )
    }

    private func mergedIntroLines(
        freshItem: FocusItem,
        previousItem: FocusItem,
        kind: FocusKind,
        includePreviousIntro: Bool
    ) -> [String] {
        var lines: [String] = []
        if kind == .space {
            lines.append(contentsOf: preservedSpaceAnalysisLines(from: previousItem))
            if !lines.isEmpty, !freshItem.detailIntroLines.isEmpty {
                lines.append("")
            }
        }
        lines.append(contentsOf: freshItem.detailIntroLines)
        if includePreviousIntro {
            let preservedPreviousIntro = preservedHistoricalIntroLines(
                previousItem: previousItem,
                freshItem: freshItem,
                kind: kind
            )
            guard !preservedPreviousIntro.isEmpty else {
                return deduplicatedConsecutiveLines(lines)
            }
            if !lines.isEmpty, lines.last?.isEmpty == false {
                lines.append("")
            }
            lines.append(contentsOf: preservedPreviousIntro)
            return deduplicatedLinesPreservingOrder(lines)
        }
        return deduplicatedConsecutiveLines(lines)
    }

    private func preservedHistoricalIntroLines(
        previousItem: FocusItem,
        freshItem: FocusItem,
        kind: FocusKind
    ) -> [String] {
        let canonicalFreshLines = Set(freshItem.detailIntroLines.map(canonicalIntroLine))
        return previousItem.detailIntroLines.filter { line in
            let trimmed = line.trimmingCharacters(in: .whitespacesAndNewlines)
            guard !trimmed.isEmpty else {
                return false
            }
            if canonicalFreshLines.contains(canonicalIntroLine(trimmed)) {
                return true
            }
            return isGeneratedAnalysisIntroLine(trimmed, kind: kind)
        }
    }

    private func canonicalIntroLine(_ line: String) -> String {
        line.trimmingCharacters(in: .whitespacesAndNewlines).lowercased()
    }

    private func isGeneratedAnalysisIntroLine(_ line: String, kind: FocusKind) -> Bool {
        let prefixes: [String]
        switch kind {
        case .space:
            prefixes = [
                "Summary generated:",
                "Guidance freshness:",
                "Space summary:",
                "Space summary source:",
                "Current posture / next move:"
            ]
        case .person:
            prefixes = [
                "Person summary:",
                "Summary source:",
                "Analysis Generated:"
            ]
        }
        let normalized = line.lowercased()
        return prefixes.contains { normalized.hasPrefix($0.lowercased()) }
    }

    private func preservedSpaceAnalysisLines(from item: FocusItem) -> [String] {
        let prefixes = [
            "Space summary:",
            "Current posture / next move:",
            "Guidance freshness:",
            "Space summary source:",
            "Summary generated:"
        ]
        return item.detailLines.filter { line in
            let trimmed = line.trimmingCharacters(in: .whitespacesAndNewlines)
            return prefixes.contains { trimmed.hasPrefix($0) }
        }
    }

    private func shouldPreserveGeneratedSection(_ section: FocusDetailSection) -> Bool {
        let header = section.header.trimmingCharacters(in: .whitespacesAndNewlines)
        guard !header.hasPrefix("Recent conversations (last ") else { return false }
        guard !header.localizedCaseInsensitiveContains("local heuristic") else { return false }
        guard !header.localizedCaseInsensitiveContains("Meaningful topics from Codex") else { return false }
        return header.hasPrefix("What are the Questions running in the Exec's (")
            || header.hasPrefix("Conversation ")
            || header.hasPrefix("Message ")
    }

    private func flattenFocusDetailLines(
        intro: [String],
        sections: [FocusDetailSection],
        tail: [String]
    ) -> [String] {
        var lines: [String] = []
        lines.append(contentsOf: intro)
        if !intro.isEmpty, !sections.isEmpty, lines.last?.isEmpty == false {
            lines.append("")
        }
        for (index, section) in sections.enumerated() {
            lines.append(section.header)
            lines.append(contentsOf: section.lines)
            if index < sections.count - 1 {
                lines.append("")
            }
        }
        if !tail.isEmpty {
            if lines.last?.isEmpty == false {
                lines.append("")
            }
            lines.append(contentsOf: tail)
        }
        return lines
    }

    private func deduplicatedConsecutiveLines(_ lines: [String]) -> [String] {
        var result: [String] = []
        for line in lines {
            if result.last == line {
                continue
            }
            result.append(line)
        }
        return result
    }

    private func deduplicatedLinesPreservingOrder(_ lines: [String]) -> [String] {
        var seen = Set<String>()
        var result: [String] = []
        for line in lines {
            if seen.insert(line).inserted {
                result.append(line)
            }
        }
        return result
    }

    private func preferredPayloadLines(primary: [String], fallback: [String]) -> [String] {
        let primaryCount = substantiveLineCount(primary)
        let fallbackCount = substantiveLineCount(fallback)
        if primaryCount >= fallbackCount {
            return primary
        }
        return fallback
    }

    private func shouldPreferPreviousRichPayload(freshItem: FocusItem, previousItem: FocusItem) -> Bool {
        let freshDepth = detailPayloadDepthScore(freshItem)
        let previousDepth = detailPayloadDepthScore(previousItem)
        guard previousDepth > freshDepth else {
            return false
        }
        if hasPineDetailPayload(previousItem), !hasPineDetailPayload(freshItem) {
            return true
        }
        return previousDepth >= max(80, freshDepth * 2)
    }

    private func detailPayloadDepthScore(_ item: FocusItem) -> Int {
        let intro = substantiveLineCount(item.detailIntroLines)
        let tail = substantiveLineCount(item.detailTailLines)
        let sections = item.detailSections.reduce(0) { partial, section in
            let headerCount = section.header.trimmingCharacters(in: .whitespacesAndNewlines).isEmpty ? 0 : 1
            return partial + headerCount + substantiveLineCount(section.lines)
        }
        return intro + tail + sections
    }

    private func substantiveLineCount(_ lines: [String]) -> Int {
        lines.reduce(0) { partial, line in
            partial + (line.trimmingCharacters(in: .whitespacesAndNewlines).isEmpty ? 0 : 1)
        }
    }

    private func enrichmentIndex(items: [FocusItem], kind: FocusKind) -> [String: FocusItem] {
        var index: [String: FocusItem] = [:]
        for item in items {
            let key = canonicalFocusItemID(item.id, kind: kind)
            guard !key.isEmpty else { continue }
            if let existing = index[key] {
                if preservationScore(item: item, kind: kind) > preservationScore(item: existing, kind: kind) {
                    index[key] = item
                }
            } else {
                index[key] = item
            }
        }
        return index
    }

    private func canonicalFocusItemID(_ itemID: String, kind: FocusKind) -> String {
        let trimmed = itemID.trimmingCharacters(in: .whitespacesAndNewlines)
        let canonical: String
        switch kind {
        case .space:
            canonical = droppingPrefix(trimmed, prefix: "spacefocus:")
        case .person:
            canonical = droppingPrefix(trimmed, prefix: "personfocus:")
        }
        return canonical.isEmpty ? trimmed : canonical
    }

    private func droppingPrefix(_ value: String, prefix: String) -> String {
        guard let range = value.range(of: prefix, options: [.anchored, .caseInsensitive]) else {
            return value
        }
        return String(value[range.upperBound...])
    }

    private func displayReadyItem(
        _ item: FocusItem,
        kind: FocusKind,
        focusDays: Int,
        seeds: [FocusClusterSeed]
    ) -> FocusItem {
        if hasPineDetailPayload(item) {
            return item
        }
        return item.assembledDetailPayload(
            kind: kind,
            focusDays: focusDays,
            clusterSeeds: seeds
        )
    }

    private func hasPineDetailPayload(_ item: FocusItem) -> Bool {
        if !item.detailTailLines.isEmpty {
            return true
        }
        if item.detailSections.count > 2 {
            return true
        }
        return item.detailSections.contains { section in
            section.header.hasPrefix("Conversation ")
                || section.header.hasPrefix("Message ")
                || section.header.hasPrefix("What are the Questions running in the Exec's (")
        }
    }

    /// Resolves source snapshots with live native output preferred over older JSON exports.
    private func resolvedSnapshotURL(kind: FocusKind) throws -> URL {
        let preferred = snapshotURL(kind: kind)
        let nativeLiveSnapshot = liveSnapshotURL(kind: kind)
        if fileManager.fileExists(atPath: nativeLiveSnapshot.path) {
            if let fallback = fallbackForSparseLiveSnapshot(nativeLiveSnapshot, kind: kind) {
                return fallback
            }
            return nativeLiveSnapshot
        }
        if fileManager.fileExists(atPath: preferred.path) {
            return preferred
        }

        let knowledgeDirectory = configuration.runtimeRoot.appendingPathComponent("knowledge", isDirectory: true)
        guard fileManager.fileExists(atPath: knowledgeDirectory.path) else {
            throw NativeRuntimeStoreError.missingSnapshot(preferred)
        }

        let prefix = "\(kind.rawValue)_focus_cache_"
        let candidates = (try? fileManager.contentsOfDirectory(
            at: knowledgeDirectory,
            includingPropertiesForKeys: [.contentModificationDateKey],
            options: [.skipsHiddenFiles]
        )) ?? []
        let matched = candidates.filter { url in
            let filename = url.lastPathComponent
            return filename.hasPrefix(prefix) && filename.hasSuffix("d.json")
        }

        if let latest = matched.max(by: { lhs, rhs in
            let lhsDate = (try? lhs.resourceValues(forKeys: [.contentModificationDateKey]).contentModificationDate) ?? .distantPast
            let rhsDate = (try? rhs.resourceValues(forKeys: [.contentModificationDateKey]).contentModificationDate) ?? .distantPast
            return lhsDate < rhsDate
        }) {
            return latest
        }

        throw NativeRuntimeStoreError.missingSnapshot(preferred)
    }

    private func refreshSourceURL(kind: FocusKind) throws -> URL {
        let nativeLiveSnapshot = liveSnapshotURL(kind: kind)
        if fileManager.fileExists(atPath: nativeLiveSnapshot.path) {
            return nativeLiveSnapshot
        }

        let preferred = snapshotURL(kind: kind)
        if fileManager.fileExists(atPath: preferred.path) {
            return preferred
        }

        return try resolvedSnapshotURL(kind: kind)
    }

    private func exactSourceURL(kind: FocusKind, focusDays: Int) throws -> URL {
        let nativeLiveSnapshot = liveSnapshotURL(kind: kind, focusDays: focusDays)
        if fileManager.fileExists(atPath: nativeLiveSnapshot.path) {
            return nativeLiveSnapshot
        }
        let sourceSnapshot = snapshotURL(kind: kind, focusDays: focusDays)
        if fileManager.fileExists(atPath: sourceSnapshot.path) {
            return sourceSnapshot
        }
        throw NativeRuntimeStoreError.missingSnapshot(sourceSnapshot)
    }

    /// Falls back when a live snapshot is only a sparse shell around zero evidence.
    private func fallbackForSparseLiveSnapshot(_ liveSnapshot: URL, kind: FocusKind) -> URL? {
        if let liveSizeBytes = fileSizeBytes(for: liveSnapshot),
           liveSizeBytes > Self.sparseLiveSnapshotInspectionMaxBytes {
            return nil
        }

        guard snapshotEvidenceCount(liveSnapshot, kind: kind) == 0 else {
            return nil
        }

        let candidates = [
            snapshotURL(kind: kind),
            nativeSnapshotURL(kind: kind, focusDays: configuredFocusDays(kind: kind))
        ].filter { url in
            url.standardizedFileURL.path != liveSnapshot.standardizedFileURL.path
                && fileManager.fileExists(atPath: url.path)
                && !isSelfSourcedNativeSnapshot(url, kind: kind)
        }

        let evidenceCandidates = candidates.filter { candidate in
            if let sizeBytes = fileSizeBytes(for: candidate),
               sizeBytes > Self.sparseFallbackCandidateDecodeMaxBytes {
                return false
            }
            return snapshotEvidenceCount(candidate, kind: kind) > 0
        }

        return evidenceCandidates.max { lhs, rhs in
            let lhsScore = snapshotEvidenceCount(lhs, kind: kind)
            let rhsScore = snapshotEvidenceCount(rhs, kind: kind)
            if lhsScore != rhsScore {
                return lhsScore < rhsScore
            }
            let lhsDate = (try? lhs.resourceValues(forKeys: [.contentModificationDateKey]).contentModificationDate) ?? .distantPast
            let rhsDate = (try? rhs.resourceValues(forKeys: [.contentModificationDateKey]).contentModificationDate) ?? .distantPast
            return lhsDate < rhsDate
        }
    }

    private func fileSizeBytes(for url: URL) -> Int64? {
        let values = try? url.resourceValues(forKeys: [.fileSizeKey])
        guard let fileSize = values?.fileSize else {
            return nil
        }
        return Int64(fileSize)
    }

    private func canReadReusableNativeCache(at url: URL) -> Bool {
        guard let sizeBytes = fileSizeBytes(for: url) else {
            return true
        }
        return sizeBytes <= Self.maxReusableNativeCacheBytes
    }

    private func isSelfSourcedNativeSnapshot(_ url: URL, kind: FocusKind) -> Bool {
        guard isGeneratedNativeSnapshotURL(url, kind: kind),
              let focusDays = focusDaysFromSnapshotURL(url),
              let manifest = try? loadManifest(kind: kind, focusDays: focusDays) else {
            return false
        }

        let candidatePath = url.standardizedFileURL.path
        let sourcePath = URL(fileURLWithPath: manifest.sourceSnapshotPath).standardizedFileURL.path
        let outputPath = URL(fileURLWithPath: manifest.outputSnapshotPath).standardizedFileURL.path
        return candidatePath == sourcePath && candidatePath == outputPath
    }

    private func isGeneratedNativeSnapshotURL(_ url: URL, kind: FocusKind) -> Bool {
        guard let focusDays = focusDaysFromSnapshotURL(url) else {
            return false
        }
        return url.lastPathComponent == "\(kind.rawValue)_focus_cache_\(focusDays)d.native.json"
    }

    private func snapshotEvidenceCount(_ url: URL, kind: FocusKind) -> Int {
        guard let cache = try? loadFocusCache(kind: kind, sourceURL: url) else {
            return 0
        }
        let itemEvidence = cache.items.reduce(0) { partial, item in
            let metaCount = messageCount(from: item.meta) ?? 0
            let eventCount = item.normalizedEvents(kind: kind).count
            return partial + max(metaCount, eventCount)
        }
        return max(cache.recentMessages, itemEvidence)
    }

    private func liveSnapshotURL(kind: FocusKind) -> URL {
        liveSnapshotURL(kind: kind, focusDays: configuredFocusDays(kind: kind))
    }

    private func liveSnapshotURL(kind: FocusKind, focusDays: Int) -> URL {
        configuration.runtimeRoot
            .appendingPathComponent("knowledge", isDirectory: true)
            .appendingPathComponent("native", isDirectory: true)
            .appendingPathComponent("live_\(kind.snapshotFilename(days: focusDays))")
    }

    private func configuredFocusDays(kind: FocusKind) -> Int {
        let settings = configStore.loadSystemSettings()
        switch kind {
        case .person:
            return SystemSettings.clamped(settings.personFocusDays, to: SystemSettings.focusDaysBounds)
        case .space:
            return SystemSettings.clamped(settings.spaceFocusDays, to: SystemSettings.focusDaysBounds)
        }
    }

    private func newestSnapshotURL(_ lhs: URL, _ rhs: URL) -> URL {
        let lhsDate = (try? lhs.resourceValues(forKeys: [.contentModificationDateKey]).contentModificationDate) ?? .distantPast
        let rhsDate = (try? rhs.resourceValues(forKeys: [.contentModificationDateKey]).contentModificationDate) ?? .distantPast
        return rhsDate > lhsDate ? rhs : lhs
    }

    private func cacheReuseVersion(kind: FocusKind) -> Int {
        switch kind {
        case .person:
            return Self.personFocusCacheReuseVersion
        case .space:
            return Self.spaceFocusCacheReuseVersion
        }
    }

    private func sourceFingerprint(url: URL) throws -> FocusSourceSnapshotFingerprint {
        let values = try url.resourceValues(forKeys: [.fileSizeKey, .contentModificationDateKey])
        let fileSize = Int64(values.fileSize ?? 0)
        let modifiedAt = values.contentModificationDate?.timeIntervalSince1970 ?? 0
        return FocusSourceSnapshotFingerprint(
            fileSizeBytes: fileSize,
            modifiedAtEpoch: modifiedAt
        )
    }

    private func configuredAnalysisCadenceHours(kind: FocusKind) -> Int {
        let settings = configStore.loadSystemSettings()
        let rawValue: Int
        switch kind {
        case .person:
            rawValue = settings.personFocusAnalysisCadenceHours
        case .space:
            rawValue = settings.spaceFocusAnalysisCadenceHours
        }
        return SystemSettings.clamped(rawValue, to: SystemSettings.focusAnalysisCadenceHoursBounds)
    }

    private func analysisBucket(for date: Date, cadenceHours: Int) -> Int {
        let boundedCadence = SystemSettings.clamped(
            cadenceHours,
            to: SystemSettings.focusAnalysisCadenceHoursBounds
        )
        let bucketSeconds = TimeInterval(boundedCadence * 60 * 60)
        return Int(floor(date.timeIntervalSince1970 / bucketSeconds))
    }

    private func promptVersion(kind: FocusKind) -> String {
        switch kind {
        case .person:
            return "person-focus-cluster-summary-v10+person-focus-cluster-title-v1"
        case .space:
            return "space-focus-summary-v5+space-focus-cluster-title-v1+space-focus-exec-questions-v1"
        }
    }

    /// Creates the cache identity record used by exact reuse and analysis freshness checks.
    private func makeManifest(
        kind: FocusKind,
        focusDays: Int,
        sourceURL: URL,
        outputURL: URL,
        sourceSignature: String,
        messageIDsHash: String,
        normalizedEventCount: Int,
        clusterCount: Int,
        sourceFingerprint: FocusSourceSnapshotFingerprint,
        generationType: String = "full",
        now: Date = Date()
    ) -> NativeFocusCacheManifest {
        let cadenceHours = configuredAnalysisCadenceHours(kind: kind)
        let bucket = analysisBucket(for: now, cadenceHours: cadenceHours)
        let settings = configStore.loadSystemSettings()
        let windowEnd = Self.manifestDateFormatter.string(from: now)
        let windowStartDate = Calendar(identifier: .gregorian).date(
            byAdding: .day,
            value: -focusDays,
            to: now
        ) ?? now
        let windowStart = Self.manifestDateFormatter.string(from: windowStartDate)
        let cacheID = analysisCacheID(
            kind: kind,
            focusDays: focusDays,
            analysisCadenceHours: cadenceHours,
            analysisBucket: bucket,
            rawEvidenceHash: sourceSignature,
            messageIDsHash: messageIDsHash,
            promptVersion: promptVersion(kind: kind),
            model: settings.codexModel.rawValue,
            reasoning: settings.codexReasoningLevel.rawValue,
            generationType: generationType
        )
        return NativeFocusCacheManifest(
            kind: kind.rawValue,
            focusDays: focusDays,
            sourceSnapshotPath: sourceURL.path,
            outputSnapshotPath: outputURL.path,
            sourceSignature: sourceSignature,
            normalizedEventCount: normalizedEventCount,
            clusterCount: clusterCount,
            cacheReuseVersion: cacheReuseVersion(kind: kind),
            sourceFileSizeBytes: sourceFingerprint.fileSizeBytes,
            sourceModifiedAtEpoch: sourceFingerprint.modifiedAtEpoch,
            targetID: "__all_targets__",
            windowDays: focusDays,
            windowStart: windowStart,
            windowEnd: windowEnd,
            analysisCadenceHours: cadenceHours,
            analysisBucket: bucket,
            rawEvidenceHash: sourceSignature,
            messageIDsHash: messageIDsHash,
            promptVersion: promptVersion(kind: kind),
            model: settings.codexModel.rawValue,
            reasoning: settings.codexReasoningLevel.rawValue,
            generationType: generationType,
            cacheID: cacheID
        )
    }

    /// Stable cache ID for the tuple of evidence, prompt, model, reasoning, and time bucket.
    private func analysisCacheID(
        kind: FocusKind,
        focusDays: Int,
        analysisCadenceHours: Int,
        analysisBucket: Int,
        rawEvidenceHash: String,
        messageIDsHash: String,
        promptVersion: String,
        model: String,
        reasoning: String,
        generationType: String
    ) -> String {
        let payload = [
            kind.rawValue,
            String(focusDays),
            String(analysisCadenceHours),
            String(analysisBucket),
            rawEvidenceHash,
            messageIDsHash,
            promptVersion,
            model,
            reasoning,
            generationType
        ].joined(separator: "\n")
        return "\(kind.rawValue)-\(focusDays)d-\(analysisBucket)-\(FocusStableHash.hex(payload))"
    }

    /// Guards exact reuse against stale evidence, prompt drift, model changes, and missing output.
    private func exactAnalysisManifestMatches(
        _ manifest: NativeFocusCacheManifest,
        kind: FocusKind,
        focusDays: Int,
        analysisCadenceHours: Int,
        analysisBucket: Int,
        rawEvidenceHash: String,
        messageIDsHash: String
    ) -> Bool {
        let settings = configStore.loadSystemSettings()
        guard manifest.kind == kind.rawValue else { return false }
        guard manifest.focusDays == focusDays else { return false }
        guard manifest.windowDays == focusDays else { return false }
        guard manifest.analysisCadenceHours == analysisCadenceHours else { return false }
        guard manifest.analysisBucket == analysisBucket else { return false }
        guard manifest.rawEvidenceHash == rawEvidenceHash else { return false }
        guard manifest.messageIDsHash == messageIDsHash else { return false }
        guard manifest.promptVersion == promptVersion(kind: kind) else { return false }
        guard manifest.model == settings.codexModel.rawValue else { return false }
        guard manifest.reasoning == settings.codexReasoningLevel.rawValue else { return false }
        guard manifest.generationType == "full" || manifest.generationType == "delta_reconcile" else { return false }
        guard let cacheID = manifest.cacheID, !cacheID.isEmpty else { return false }
        return fileManager.fileExists(atPath: manifest.outputSnapshotPath)
    }

    private static let manifestDateFormatter: ISO8601DateFormatter = {
        let formatter = ISO8601DateFormatter()
        formatter.formatOptions = [.withInternetDateTime, .withFractionalSeconds]
        return formatter
    }()

    private func canReuseByFingerprint(
        manifest: NativeFocusCacheManifest,
        kind: FocusKind,
        sourceURL: URL,
        sourceFingerprint: FocusSourceSnapshotFingerprint,
        cacheReuseVersion: Int
    ) -> Bool {
        guard manifest.kind == kind.rawValue else { return false }
        guard manifest.cacheReuseVersion == cacheReuseVersion else { return false }
        guard manifest.sourceSnapshotPath == sourceURL.path else { return false }
        guard manifest.sourceFileSizeBytes == sourceFingerprint.fileSizeBytes else { return false }
        guard manifest.sourceModifiedAtEpoch == sourceFingerprint.modifiedAtEpoch else { return false }
        return fileManager.fileExists(atPath: manifest.outputSnapshotPath)
    }

    private func canReuseBySignature(
        manifest: NativeFocusCacheManifest,
        kind: FocusKind,
        focusDays: Int,
        outputURL: URL,
        sourceSignature: String,
        cacheReuseVersion: Int
    ) -> Bool {
        guard manifest.kind == kind.rawValue else { return false }
        guard manifest.cacheReuseVersion == cacheReuseVersion else { return false }
        guard manifest.focusDays == focusDays else { return false }
        guard manifest.outputSnapshotPath == outputURL.path else { return false }
        guard manifest.sourceSignature == sourceSignature else { return false }
        return fileManager.fileExists(atPath: outputURL.path)
    }

    private func rewriteManifestForReuse(
        manifest: NativeFocusCacheManifest,
        kind: FocusKind,
        sourceURL: URL,
        sourceFingerprint: FocusSourceSnapshotFingerprint,
        focusDays: Int,
        outputURL: URL,
        sourceSignature: String,
        messageIDsHash: String
    ) throws {
        let refreshedManifest = makeManifest(
            kind: kind,
            focusDays: focusDays,
            sourceURL: sourceURL,
            outputURL: outputURL,
            sourceSignature: sourceSignature,
            messageIDsHash: manifest.messageIDsHash ?? messageIDsHash,
            normalizedEventCount: manifest.normalizedEventCount,
            clusterCount: manifest.clusterCount,
            sourceFingerprint: sourceFingerprint,
            generationType: manifest.generationType ?? "full"
        )
        try writeManifest(refreshedManifest, kind: kind)
    }

    private func writeManifest(_ manifest: NativeFocusCacheManifest, kind: FocusKind) throws {
        try ensureNativeCacheDirectory()
        let manifestData = try encoder.encode(manifest)
        try manifestData.write(to: nativeManifestURL(kind: kind, focusDays: manifest.focusDays), options: [.atomic])
        try manifestData.write(to: nativeManifestURL(kind: kind), options: [.atomic])
    }

    private func loadManifest(kind: FocusKind, focusDays: Int) throws -> NativeFocusCacheManifest {
        let scopedManifestURL = nativeManifestURL(kind: kind, focusDays: focusDays)
        if fileManager.fileExists(atPath: scopedManifestURL.path) {
            return try loadManifest(at: scopedManifestURL)
        }

        let legacyManifest = try loadManifest(kind: kind)
        guard legacyManifest.focusDays == focusDays else {
            throw NativeRuntimeStoreError.invalidNativeCacheManifest(scopedManifestURL)
        }
        return legacyManifest
    }

    private func loadManifest(kind: FocusKind) throws -> NativeFocusCacheManifest {
        try loadManifest(at: nativeManifestURL(kind: kind))
    }

    private func loadManifest(at manifestURL: URL) throws -> NativeFocusCacheManifest {
        guard fileManager.fileExists(atPath: manifestURL.path) else {
            throw NativeRuntimeStoreError.invalidNativeCacheManifest(manifestURL)
        }
        let data = try Data(contentsOf: manifestURL)
        do {
            return try decoder.decode(NativeFocusCacheManifest.self, from: data)
        } catch {
            throw NativeRuntimeStoreError.invalidNativeCacheManifest(manifestURL)
        }
    }
}
