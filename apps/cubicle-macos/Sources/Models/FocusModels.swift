import Foundation

enum AppSection: String, CaseIterable, Identifiable {
    case home
    case spaceFocus
    case personFocus
    case spaceFocusTargets
    case personFocusTargets
    case execFocusTargets
    case questions
    case transcription
    case beliefs
    case askCodex
    case jobs
    case settings

    var id: String { rawValue }

    var title: String {
        switch self {
        case .home: return "Home"
        case .spaceFocus: return "Space Focus"
        case .personFocus: return "Person Focus"
        case .spaceFocusTargets: return "Add/Remove Space Focus"
        case .personFocusTargets: return "Add/Remove Person Focus"
        case .execFocusTargets: return "Add/Remove Exec Focus"
        case .questions: return "Questions"
        case .transcription: return "Transcription"
        case .beliefs: return "Beliefs"
        case .askCodex: return "Ask Codex"
        case .jobs: return "Jobs"
        case .settings: return "Settings"
        }
    }

    var symbolName: String {
        switch self {
        case .home: return "square.grid.2x2"
        case .spaceFocus: return "bubble.left.and.bubble.right"
        case .personFocus: return "person.2"
        case .spaceFocusTargets: return "plus.bubble"
        case .personFocusTargets: return "person.crop.circle.badge.plus"
        case .execFocusTargets: return "person.badge.key"
        case .questions: return "questionmark.bubble"
        case .transcription: return "waveform"
        case .beliefs: return "brain"
        case .askCodex: return "sparkles"
        case .jobs: return "clock.arrow.circlepath"
        case .settings: return "gearshape"
        }
    }
}

enum FocusTargetManagementKind: String, CaseIterable, Identifiable, Hashable {
    case spaceFocus
    case personFocus
    case execFocus

    var id: String { rawValue }

    var title: String {
        switch self {
        case .spaceFocus:
            return "Add/Remove Space Focus"
        case .personFocus:
            return "Add/Remove Person Focus"
        case .execFocus:
            return "Add/Remove Exec Focus"
        }
    }

    var shortTitle: String {
        switch self {
        case .spaceFocus:
            return "Space Focus"
        case .personFocus:
            return "Person Focus"
        case .execFocus:
            return "Exec Focus"
        }
    }

    var badgeText: String {
        switch self {
        case .spaceFocus:
            return "space"
        case .personFocus:
            return "person"
        case .execFocus:
            return "exec"
        }
    }

    var symbolName: String {
        switch self {
        case .spaceFocus:
            return "plus.bubble"
        case .personFocus:
            return "person.crop.circle.badge.plus"
        case .execFocus:
            return "person.badge.key"
        }
    }

    var sourceFilename: String {
        switch self {
        case .spaceFocus:
            return "important-senders.txt"
        case .personFocus:
            return "important-senders.txt"
        case .execFocus:
            return "importantexec.txt"
        }
    }
}

enum FocusKind: String, CaseIterable, Identifiable {
    case space
    case person

    var id: String { rawValue }

    var title: String {
        switch self {
        case .space: return "Space Focus"
        case .person: return "Person Focus"
        }
    }

    var defaultDays: Int {
        switch self {
        case .space: return 60
        case .person: return 60
        }
    }

    func snapshotFilename(days: Int? = nil) -> String {
        let value = days ?? defaultDays
        switch self {
        case .space:
            return "space_focus_cache_\(value)d.json"
        case .person:
            return "person_focus_cache_\(value)d.json"
        }
    }
}

struct FocusCache: Codable, Equatable {
    var focusDays: Int
    var items: [FocusItem]
    var updatedAt: String
    var countLabel: String
    var recentMessages: Int
    var summaryGenerationInProgress: Bool
    var subjectsProcessed: Int
    var subjectsTotal: Int

    static func empty(kind: FocusKind) -> FocusCache {
        FocusCache(
            focusDays: kind.defaultDays,
            items: [],
            updatedAt: "",
            countLabel: "0",
            recentMessages: 0,
            summaryGenerationInProgress: false,
            subjectsProcessed: 0,
            subjectsTotal: 0
        )
    }

    enum CodingKeys: String, CodingKey {
        case focusDays = "focus_days"
        case items
        case updatedAt = "updated_at"
        case spaces
        case people
        case recentMessages = "recent_messages"
        case summaryGenerationInProgress = "summary_generation_in_progress"
        case subjectsProcessed = "subjects_processed"
        case subjectsTotal = "subjects_total"
    }

    init(
        focusDays: Int,
        items: [FocusItem],
        updatedAt: String,
        countLabel: String,
        recentMessages: Int,
        summaryGenerationInProgress: Bool,
        subjectsProcessed: Int,
        subjectsTotal: Int
    ) {
        self.focusDays = focusDays
        self.items = items
        self.updatedAt = updatedAt
        self.countLabel = countLabel
        self.recentMessages = recentMessages
        self.summaryGenerationInProgress = summaryGenerationInProgress
        self.subjectsProcessed = subjectsProcessed
        self.subjectsTotal = subjectsTotal
    }

    init(from decoder: Decoder) throws {
        let container = try decoder.container(keyedBy: CodingKeys.self)
        focusDays = try container.decodeIfPresent(Int.self, forKey: .focusDays) ?? 60
        items = try container.decodeIfPresent([FocusItem].self, forKey: .items) ?? []
        updatedAt = try container.decodeIfPresent(String.self, forKey: .updatedAt) ?? ""
        let spaces = try container.decodeIfPresent(Int.self, forKey: .spaces)
        let people = try container.decodeIfPresent(Int.self, forKey: .people)
        countLabel = String(spaces ?? people ?? items.count)
        recentMessages = try container.decodeIfPresent(Int.self, forKey: .recentMessages) ?? 0
        summaryGenerationInProgress = try container.decodeIfPresent(Bool.self, forKey: .summaryGenerationInProgress) ?? false
        subjectsProcessed = try container.decodeIfPresent(Int.self, forKey: .subjectsProcessed) ?? 0
        subjectsTotal = try container.decodeIfPresent(Int.self, forKey: .subjectsTotal) ?? 0
    }

    func encode(to encoder: Encoder) throws {
        var container = encoder.container(keyedBy: CodingKeys.self)
        try container.encode(focusDays, forKey: .focusDays)
        try container.encode(items, forKey: .items)
        try container.encode(updatedAt, forKey: .updatedAt)
        if let count = Int(countLabel) {
            try container.encode(count, forKey: .spaces)
        } else {
            try container.encode(items.count, forKey: .spaces)
        }
        try container.encode(recentMessages, forKey: .recentMessages)
        try container.encode(summaryGenerationInProgress, forKey: .summaryGenerationInProgress)
        try container.encode(subjectsProcessed, forKey: .subjectsProcessed)
        try container.encode(subjectsTotal, forKey: .subjectsTotal)
    }
}

struct FocusItem: Identifiable, Codable, Hashable {
    var id: String
    var title: String
    var subtitle: String
    var meta: String
    var timestamp: String
    var badge: String
    var statusBadge: String
    var detailLines: [String]
    var detailIntroLines: [String]
    var detailSections: [FocusDetailSection]
    var detailTailLines: [String]

    enum CodingKeys: String, CodingKey {
        case id
        case title
        case subtitle
        case meta
        case timestamp
        case badge
        case statusBadge = "status_badge"
        case detailLines = "detail_lines"
        case detailIntroLines = "detail_intro_lines"
        case detailSections = "detail_sections"
        case detailTailLines = "detail_tail_lines"
    }

    init(
        id: String,
        title: String,
        subtitle: String,
        meta: String,
        timestamp: String,
        badge: String,
        statusBadge: String,
        detailLines: [String],
        detailIntroLines: [String],
        detailSections: [FocusDetailSection],
        detailTailLines: [String]
    ) {
        self.id = id
        self.title = title
        self.subtitle = subtitle
        self.meta = meta
        self.timestamp = timestamp
        self.badge = badge
        self.statusBadge = statusBadge
        self.detailLines = detailLines
        self.detailIntroLines = detailIntroLines
        self.detailSections = detailSections
        self.detailTailLines = detailTailLines
    }

    init(from decoder: Decoder) throws {
        let container = try decoder.container(keyedBy: CodingKeys.self)
        id = try container.decodeIfPresent(String.self, forKey: .id) ?? UUID().uuidString
        title = try container.decodeIfPresent(String.self, forKey: .title) ?? "Untitled"
        subtitle = try container.decodeIfPresent(String.self, forKey: .subtitle) ?? ""
        meta = try container.decodeIfPresent(String.self, forKey: .meta) ?? ""
        timestamp = try container.decodeIfPresent(String.self, forKey: .timestamp) ?? ""
        badge = try container.decodeIfPresent(String.self, forKey: .badge) ?? ""
        statusBadge = try container.decodeIfPresent(String.self, forKey: .statusBadge) ?? ""
        detailLines = try container.decodeIfPresent([String].self, forKey: .detailLines) ?? []
        detailIntroLines = try container.decodeIfPresent([String].self, forKey: .detailIntroLines) ?? []
        detailSections = try container.decodeIfPresent([FocusDetailSection].self, forKey: .detailSections) ?? []
        detailTailLines = try container.decodeIfPresent([String].self, forKey: .detailTailLines) ?? []
    }

    var searchableText: String {
        ([title, subtitle, meta, timestamp, badge, statusBadge] + detailLines.prefix(12)).joined(separator: " ")
    }

    func firstDetailLine(prefix: String) -> String {
        detailLines.first { $0.localizedCaseInsensitiveContains(prefix) }?
            .replacingOccurrences(of: prefix, with: "")
            .trimmingCharacters(in: .whitespacesAndNewlines) ?? ""
    }

    func normalizedEvents(kind: FocusKind) -> [FocusNormalizedEvent] {
        let roomTitle = firstDetailLine(prefix: "Space Name:").isEmpty ? title : firstDetailLine(prefix: "Space Name:")
        let sourceLines = detailLines.isEmpty ? fallbackDetailLines : detailLines
        return sourceLines.enumerated().compactMap { index, rawLine in
            FocusMessageLineParser.parse(
                rawLine,
                itemID: id,
                itemTitle: title,
                roomTitle: roomTitle,
                ordinal: index,
                kind: kind
            )
        }
    }

    func assembledDetailPayload(kind: FocusKind, focusDays: Int, clusterSeeds: [FocusClusterSeed]) -> FocusItem {
        let payload = FocusDetailPayloadAssembler.assemble(
            item: self,
            kind: kind,
            focusDays: focusDays,
            clusterSeeds: clusterSeeds
        )
        return FocusItem(
            id: id,
            title: title,
            subtitle: subtitle,
            meta: meta,
            timestamp: timestamp,
            badge: badge,
            statusBadge: statusBadge,
            detailLines: payload.flattenedLines,
            detailIntroLines: payload.introLines,
            detailSections: payload.sections,
            detailTailLines: payload.tailLines
        )
    }

    private var fallbackDetailLines: [String] {
        var lines: [String] = []
        if !detailIntroLines.isEmpty {
            lines.append(contentsOf: detailIntroLines)
            if detailIntroLines.last?.isEmpty == false {
                lines.append("")
            }
        }
        for (index, section) in detailSections.enumerated() {
            lines.append(section.header)
            lines.append(contentsOf: section.lines)
            if index < detailSections.count - 1 {
                lines.append("")
            }
        }
        if !detailTailLines.isEmpty {
            if lines.last?.isEmpty == false {
                lines.append("")
            }
            lines.append(contentsOf: detailTailLines)
        }
        return lines
    }
}

struct FocusDetailSection: Identifiable, Codable, Hashable {
    var id: String
    var header: String
    var lines: [String]
    var roomTitle: String
    var summarySource: String
    var summaryGeneratedAt: String

    enum CodingKeys: String, CodingKey {
        case id
        case header
        case lines
        case roomTitle = "room_title"
        case summarySource = "summary_source"
        case summaryGeneratedAt = "summary_generated_at"
    }

    init(
        id: String,
        header: String,
        lines: [String],
        roomTitle: String,
        summarySource: String,
        summaryGeneratedAt: String
    ) {
        self.id = id
        self.header = header
        self.lines = lines
        self.roomTitle = roomTitle
        self.summarySource = summarySource
        self.summaryGeneratedAt = summaryGeneratedAt
    }

    init(from decoder: Decoder) throws {
        let container = try decoder.container(keyedBy: CodingKeys.self)
        header = try container.decodeIfPresent(String.self, forKey: .header) ?? "Details"
        lines = try container.decodeIfPresent([String].self, forKey: .lines) ?? []
        roomTitle = try container.decodeIfPresent(String.self, forKey: .roomTitle) ?? ""
        summarySource = try container.decodeIfPresent(String.self, forKey: .summarySource) ?? ""
        summaryGeneratedAt = try container.decodeIfPresent(String.self, forKey: .summaryGeneratedAt) ?? ""
        id = try container.decodeIfPresent(String.self, forKey: .id) ?? "\(header)-\(roomTitle)"
    }
}

struct RuntimeStatus: Equatable {
    var runtimeRoot: URL
    var knowledgeDirectoryExists: Bool
    var spaceSnapshotExists: Bool
    var personSnapshotExists: Bool
    var codexExecutable: String
}

struct FocusNormalizedEvent: Identifiable, Hashable {
    var id: String
    var subjectID: String
    var roomKey: String
    var senderKey: String
    var senderLabel: String
    var timestampLabel: String
    var messageText: String
    var normalizedText: String
    var topicKey: String
    var linkageKey: String
}

struct FocusClusterSeed: Identifiable, Hashable {
    var id: String
    var key: String
    var title: String
    var eventCount: Int
    var lastTimestampLabel: String
    var participants: [String]
    var sampleMessages: [String]
    var soWhat: String

    static func makeSeeds(kind: FocusKind, events: [FocusNormalizedEvent], limit: Int = 12) -> [FocusClusterSeed] {
        FocusClusterSeedBuilder.makeSeeds(kind: kind, events: events, limit: limit)
    }
}

extension FocusCache {
    func normalizedEvents(kind: FocusKind) -> [FocusNormalizedEvent] {
        items.flatMap { $0.normalizedEvents(kind: kind) }
    }

    func sourceSignature(kind: FocusKind) -> String {
        let itemFingerprint = items
            .map { item in
                let normalized = item.normalizedEvents(kind: kind)
                let eventFingerprint = normalized
                    .map { event in
                        [
                            event.subjectID,
                            event.roomKey,
                            event.senderKey,
                            event.timestampLabel,
                            event.topicKey,
                            event.linkageKey,
                            event.normalizedText
                        ].joined(separator: "|")
                    }
                    .joined(separator: "\n")
                return [
                    item.id,
                    item.title,
                    item.subtitle,
                    item.meta,
                    item.badge,
                    item.statusBadge,
                    String(normalized.count),
                    eventFingerprint
                ].joined(separator: "\n")
            }
            .joined(separator: "\n---\n")
        let base = [
            kind.rawValue,
            String(focusDays),
            String(items.count),
            String(recentMessages),
            String(subjectsProcessed),
            String(subjectsTotal),
            itemFingerprint
        ].joined(separator: "\n")
        return FocusStableHash.hex(base)
    }

    func messageIDsSignature(kind: FocusKind) -> String {
        let eventFingerprint = items
            .flatMap { $0.normalizedEvents(kind: kind) }
            .map { event in
                [
                    event.id,
                    event.subjectID,
                    event.roomKey,
                    event.senderKey,
                    event.timestampLabel,
                    event.linkageKey
                ].joined(separator: "|")
            }
            .sorted()
            .joined(separator: "\n")
        return FocusStableHash.hex(eventFingerprint)
    }
}

private enum FocusMessageLineParser {
    private static let messagePrefixes = ["Webex ", "Space message "]
    private static let nonTopicWords: Set<String> = [
        "the", "and", "for", "that", "with", "this", "from", "into", "have", "been", "were", "will",
        "would", "could", "about", "there", "their", "they", "your", "you", "our", "out", "are", "not",
        "but", "who", "what", "when", "where", "while", "how", "why", "yes", "has", "had", "use", "using",
        "via", "can", "just", "also", "than", "then", "its", "it's", "all", "any", "one", "two", "three",
        "today", "tomorrow", "yesterday"
    ]

    static func parse(
        _ rawLine: String,
        itemID: String,
        itemTitle: String,
        roomTitle: String,
        ordinal: Int,
        kind: FocusKind
    ) -> FocusNormalizedEvent? {
        let line = rawLine.trimmingCharacters(in: .whitespacesAndNewlines)
        guard messagePrefixes.contains(where: { line.hasPrefix($0) }) else {
            return nil
        }
        guard let colon = line.firstIndex(of: ":") else {
            return nil
        }

        let payload = String(line[line.index(after: colon)...]).trimmingCharacters(in: .whitespacesAndNewlines)
        let fields = payload
            .split(separator: "|", maxSplits: 3, omittingEmptySubsequences: false)
            .map { String($0).trimmingCharacters(in: .whitespacesAndNewlines) }

        guard fields.count >= 2 else {
            return nil
        }

        let timestamp = fields[0]
        let sender = fields[1].isEmpty ? itemTitle : fields[1]
        let room: String
        let message: String
        if fields.count >= 4 {
            room = fields[2].isEmpty ? roomTitle : fields[2]
            message = fields[3]
        } else if fields.count == 3 {
            room = roomTitle
            message = fields[2]
        } else {
            room = roomTitle
            message = ""
        }

        let normalizedText = normalizeText(message)
        let topicKey = makeTopicKey(from: normalizedText)
        let linkageKey = makeLinkageKey(
            subjectID: itemID,
            sender: sender,
            room: room,
            normalizedText: normalizedText
        )
        let eventID = FocusStableHash.hex(
            [kind.rawValue, itemID, room, sender, timestamp, linkageKey, String(ordinal)].joined(separator: "|")
        )

        return FocusNormalizedEvent(
            id: "event:\(eventID)",
            subjectID: itemID,
            roomKey: stableKey(room),
            senderKey: stableKey(sender),
            senderLabel: sender,
            timestampLabel: timestamp,
            messageText: message,
            normalizedText: normalizedText,
            topicKey: topicKey,
            linkageKey: linkageKey
        )
    }

    static func topicTitle(for topicKey: String, fallback: String) -> String {
        guard topicKey != "general" else {
            return fallback.isEmpty ? "General update" : fallback
        }
        return topicKey
            .split(separator: "-")
            .map { token in
                let word = String(token)
                return word.prefix(1).uppercased() + word.dropFirst()
            }
            .joined(separator: " ")
    }

    private static func makeTopicKey(from normalizedText: String) -> String {
        let tokens = tokenize(normalizedText)
            .filter { token in
                token.count > 2 && !nonTopicWords.contains(token)
            }
        guard !tokens.isEmpty else {
            return "general"
        }
        return tokens.prefix(6).joined(separator: "-")
    }

    private static func makeLinkageKey(subjectID: String, sender: String, room: String, normalizedText: String) -> String {
        let urlToken = normalizedText
            .split(separator: " ")
            .map(String.init)
            .first(where: { $0.contains("://") || $0.hasPrefix("webexteams://") })
        if let urlToken {
            return FocusStableHash.hex([subjectID, sender, room, urlToken].joined(separator: "|"))
        }
        let compact = tokenize(normalizedText).prefix(14).joined(separator: "-")
        return FocusStableHash.hex([subjectID, sender, room, compact].joined(separator: "|"))
    }

    private static func stableKey(_ value: String) -> String {
        normalizeText(value)
            .replacingOccurrences(of: " ", with: "_")
    }

    private static func normalizeText(_ value: String) -> String {
        value
            .lowercased()
            .split(whereSeparator: \.isWhitespace)
            .map(String.init)
            .joined(separator: " ")
            .trimmingCharacters(in: .whitespacesAndNewlines)
    }

    private static func tokenize(_ value: String) -> [String] {
        value
            .components(separatedBy: CharacterSet.alphanumerics.inverted)
            .map { $0.trimmingCharacters(in: .whitespacesAndNewlines) }
            .filter { !$0.isEmpty }
    }
}

private enum FocusClusterSeedBuilder {
    private struct Accumulator {
        var events: [FocusNormalizedEvent] = []
        var participants: Set<String> = []
        var rooms: Set<String> = []
        var latestTimestamp: String = ""

        mutating func append(event: FocusNormalizedEvent) {
            events.append(event)
            participants.insert(event.senderLabel)
            rooms.insert(event.roomKey)
            if latestTimestamp < event.timestampLabel {
                latestTimestamp = event.timestampLabel
            }
        }

        mutating func merge(with other: Accumulator) {
            events.append(contentsOf: other.events)
            participants.formUnion(other.participants)
            rooms.formUnion(other.rooms)
            if latestTimestamp < other.latestTimestamp {
                latestTimestamp = other.latestTimestamp
            }
        }
    }

    private static let semanticNoiseWords: Set<String> = [
        "http", "https", "www", "com", "net", "org", "webex", "message", "space",
        "thread", "team", "update", "discussion", "status", "today", "tomorrow", "yesterday",
        "from", "with", "that", "this", "have", "been", "will", "would", "could", "about"
    ]

    private static let semanticSynonyms: [String: String] = [
        "auth": "identity",
        "authentication": "identity",
        "oauth": "identity",
        "credentials": "credential",
        "tokens": "token",
        "policies": "policy",
        "models": "model",
        "gateways": "gateway",
        "agents": "agent",
        "workflows": "workflow"
    ]

    static func makeSeeds(kind: FocusKind, events: [FocusNormalizedEvent], limit: Int) -> [FocusClusterSeed] {
        var grouped: [String: Accumulator] = [:]
        grouped.reserveCapacity(events.count)

        for event in events {
            let groupKey: String
            switch kind {
            case .person:
                groupKey = "\(event.roomKey)|\(event.topicKey)"
            case .space:
                groupKey = semanticSpaceTopicKey(event: event)
            }

            var accumulator = grouped[groupKey] ?? Accumulator()
            accumulator.append(event: event)
            grouped[groupKey] = accumulator
        }

        if kind == .space {
            grouped = consolidateDuplicateSpaceTopics(grouped)
        }

        let seeds: [FocusClusterSeed] = grouped.map { key, accumulator in
            let sample = accumulator.events.prefix(2).map(\.messageText)
            let topic = key == "general" ? (accumulator.events.first?.topicKey ?? "general") : key
            let fallbackTitle = sample.first?.split(separator: " ").prefix(8).joined(separator: " ") ?? "General update"
            let title = FocusMessageLineParser.topicTitle(for: topic, fallback: fallbackTitle)
            let participants = accumulator.participants.sorted()
            let soWhat = makeSoWhat(
                kind: kind,
                participants: participants.count,
                roomCount: accumulator.rooms.count,
                eventCount: accumulator.events.count,
                title: title,
                latestTimestamp: accumulator.latestTimestamp
            )

            return FocusClusterSeed(
                id: "cluster:\(FocusStableHash.hex(key))",
                key: key,
                title: title,
                eventCount: accumulator.events.count,
                lastTimestampLabel: accumulator.latestTimestamp,
                participants: participants,
                sampleMessages: sample,
                soWhat: soWhat
            )
        }

        return seeds
            .sorted { lhs, rhs in
                if lhs.eventCount != rhs.eventCount {
                    return lhs.eventCount > rhs.eventCount
                }
                if lhs.lastTimestampLabel != rhs.lastTimestampLabel {
                    return lhs.lastTimestampLabel > rhs.lastTimestampLabel
                }
                return lhs.key < rhs.key
            }
            .prefix(limit)
            .map { $0 }
    }

    private static func semanticSpaceTopicKey(event: FocusNormalizedEvent) -> String {
        var tokens: [String]
        if event.topicKey != "general" {
            tokens = event.topicKey.split(separator: "-").map(String.init)
        } else {
            tokens = event.normalizedText
                .components(separatedBy: CharacterSet.alphanumerics.inverted)
                .map { $0.trimmingCharacters(in: .whitespacesAndNewlines) }
                .filter { !$0.isEmpty }
        }

        let semanticTokens = Set(
            tokens.compactMap { rawToken in
                normalizedSemanticToken(rawToken)
            }
        )

        guard !semanticTokens.isEmpty else {
            return "general"
        }
        return semanticTokens.sorted().prefix(6).joined(separator: "-")
    }

    private static func normalizedSemanticToken(_ rawToken: String) -> String? {
        var token = rawToken.lowercased().trimmingCharacters(in: .whitespacesAndNewlines)
        guard token.count > 2 else { return nil }
        if token.hasSuffix("ing"), token.count > 5 {
            token = String(token.dropLast(3))
        } else if token.hasSuffix("ed"), token.count > 4 {
            token = String(token.dropLast(2))
        } else if token.hasSuffix("es"), token.count > 4 {
            token = String(token.dropLast(2))
        } else if token.hasSuffix("s"), token.count > 3 {
            token = String(token.dropLast())
        }
        if let synonym = semanticSynonyms[token] {
            token = synonym
        }
        guard token.count > 2 else { return nil }
        guard !semanticNoiseWords.contains(token) else { return nil }
        return token
    }

    private static func consolidateDuplicateSpaceTopics(_ grouped: [String: Accumulator]) -> [String: Accumulator] {
        var merged: [(key: String, accumulator: Accumulator)] = []
        merged.reserveCapacity(grouped.count)

        for key in grouped.keys.sorted() {
            guard let source = grouped[key] else { continue }
            var targetIndex: Int?
            var bestSimilarity: Double = 0

            for (index, entry) in merged.enumerated() {
                let similarity = semanticTopicSimilarity(lhs: key, rhs: entry.key)
                if similarity >= 0.67, (targetIndex == nil || similarity > bestSimilarity) {
                    targetIndex = index
                    bestSimilarity = similarity
                }
            }

            if let targetIndex {
                var targetEntry = merged[targetIndex]
                targetEntry.accumulator.merge(with: source)
                targetEntry.key = preferredMergedTopicKey(lhs: targetEntry.key, rhs: key)
                merged[targetIndex] = targetEntry
            } else {
                merged.append((key: key, accumulator: source))
            }
        }

        return Dictionary(uniqueKeysWithValues: merged.map { ($0.key, $0.accumulator) })
    }

    private static func semanticTopicSimilarity(lhs: String, rhs: String) -> Double {
        if lhs == rhs {
            return 1.0
        }
        let lhsSet = Set(lhs.split(separator: "-").map(String.init))
        let rhsSet = Set(rhs.split(separator: "-").map(String.init))
        guard !lhsSet.isEmpty, !rhsSet.isEmpty else {
            return 0
        }

        let intersectionCount = lhsSet.intersection(rhsSet).count
        let unionCount = lhsSet.union(rhsSet).count
        guard unionCount > 0 else {
            return 0
        }

        if lhsSet.isSubset(of: rhsSet) || rhsSet.isSubset(of: lhsSet) {
            return max(Double(intersectionCount) / Double(lhsSet.count), Double(intersectionCount) / Double(rhsSet.count))
        }
        return Double(intersectionCount) / Double(unionCount)
    }

    private static func preferredMergedTopicKey(lhs: String, rhs: String) -> String {
        let lhsCount = lhs.split(separator: "-").count
        let rhsCount = rhs.split(separator: "-").count
        if lhsCount != rhsCount {
            return lhsCount > rhsCount ? lhs : rhs
        }
        return lhs < rhs ? lhs : rhs
    }

    private static func makeSoWhat(
        kind: FocusKind,
        participants: Int,
        roomCount: Int,
        eventCount: Int,
        title: String,
        latestTimestamp: String
    ) -> String {
        let participantPhrase = participants == 1 ? "1 participant" : "\(participants) participants"
        let roomPhrase: String
        switch kind {
        case .person:
            roomPhrase = roomCount == 1 ? "1 room" : "\(roomCount) rooms"
        case .space:
            roomPhrase = "this space"
        }
        return "\(participantPhrase), \(eventCount) messages across \(roomPhrase), latest update at \(latestTimestamp) on \(title.lowercased())."
    }
}

private enum FocusDetailPayloadAssembler {
    struct Payload {
        var introLines: [String]
        var sections: [FocusDetailSection]
        var tailLines: [String]
        var flattenedLines: [String]
    }

    static func assemble(item: FocusItem, kind: FocusKind, focusDays: Int, clusterSeeds: [FocusClusterSeed]) -> Payload {
        let sourceLines = item.detailLines.isEmpty ? fallbackSourceLines(item) : item.detailLines
        let partitioned = partition(sourceLines)
        let roomTitle = roomTitle(from: sourceLines, fallback: item.title)
        let rawSummarySource = valueAfterPrefix(in: sourceLines, prefixes: ["Space summary source:", "Summary source:", "Title source:"])
        let summarySource = rawSummarySource.localizedCaseInsensitiveContains("codex") ? rawSummarySource : ""
        let summaryGeneratedAt = valueAfterPrefix(in: sourceLines, prefixes: ["Summary generated:", "Title generated at:"])

        let displaySections = partitioned.sections.filter { section in
            !isLocalHeuristicSectionHeader(section.header)
        }
        var sectionModels = displaySections.enumerated().map { index, section in
            FocusDetailSection(
                id: sectionID(itemID: item.id, index: index, header: section.header, lines: section.lines),
                header: section.header,
                lines: section.lines,
                roomTitle: roomTitle,
                summarySource: summarySource,
                summaryGeneratedAt: summaryGeneratedAt
            )
        }

        let hasRecentConversation = sectionModels.contains {
            $0.header.hasPrefix("Recent conversations (last ")
        }
        if !hasRecentConversation {
            let messageLines = sourceLines.filter { line in
                line.hasPrefix("Webex ") || line.hasPrefix("Space message ")
            }
            if !messageLines.isEmpty {
                sectionModels.append(
                    FocusDetailSection(
                        id: sectionID(
                            itemID: item.id,
                            index: sectionModels.count,
                            header: "Recent conversations (last \(focusDays) days):",
                            lines: messageLines
                        ),
                        header: "Recent conversations (last \(focusDays) days):",
                        lines: messageLines,
                        roomTitle: roomTitle,
                        summarySource: summarySource,
                        summaryGeneratedAt: summaryGeneratedAt
                    )
                )
            }
        }

        let flattened = flatten(intro: partitioned.intro, sections: sectionModels, tail: partitioned.tail)
        return Payload(
            introLines: partitioned.intro,
            sections: sectionModels,
            tailLines: partitioned.tail,
            flattenedLines: flattened
        )
    }

    private struct ParsedSection {
        var header: String
        var lines: [String]
    }

    private struct PartitionedLines {
        var intro: [String]
        var sections: [ParsedSection]
        var tail: [String]
    }

    private static func partition(_ lines: [String]) -> PartitionedLines {
        var intro: [String] = []
        var sections: [ParsedSection] = []
        var tail: [String] = []

        var currentHeader: String?
        var currentLines: [String] = []
        var seenFirstSection = false

        func flushCurrentSection() {
            guard let currentHeader else {
                return
            }
            sections.append(ParsedSection(header: currentHeader, lines: trimTrailingEmptyLines(currentLines)))
            currentLines = []
        }

        for line in lines {
            let trimmed = line.trimmingCharacters(in: .whitespacesAndNewlines)
            if isSectionHeader(trimmed) {
                flushCurrentSection()
                currentHeader = trimmed
                seenFirstSection = true
                continue
            }
            if let currentHeader, !currentHeader.isEmpty {
                currentLines.append(line)
            } else if seenFirstSection {
                tail.append(line)
            } else {
                intro.append(line)
            }
        }
        flushCurrentSection()

        if sections.isEmpty {
            intro = lines
        }

        return PartitionedLines(
            intro: trimTrailingEmptyLines(intro),
            sections: sections,
            tail: trimLeadingAndTrailingEmptyLines(tail)
        )
    }

    private static func roomTitle(from lines: [String], fallback: String) -> String {
        let value = valueAfterPrefix(in: lines, prefixes: ["Space Name:", "Room Name:"])
        return value.isEmpty ? fallback : value
    }

    private static func valueAfterPrefix(in lines: [String], prefixes: [String]) -> String {
        for line in lines {
            for prefix in prefixes {
                if line.localizedCaseInsensitiveContains(prefix) {
                    return line
                        .replacingOccurrences(of: prefix, with: "")
                        .trimmingCharacters(in: .whitespacesAndNewlines)
                }
            }
        }
        return ""
    }

    private static func sectionID(itemID: String, index: Int, header: String, lines: [String]) -> String {
        let hashInput = ([header] + lines.prefix(8))
            .joined(separator: "\n")
            .lowercased()
        return "section:\(itemID):\(index):\(FocusStableHash.hex(hashInput))"
    }

    private static func flatten(intro: [String], sections: [FocusDetailSection], tail: [String]) -> [String] {
        var flattened: [String] = []
        flattened.reserveCapacity(intro.count + tail.count + sections.count * 6)
        flattened.append(contentsOf: intro)

        if !intro.isEmpty, !sections.isEmpty, flattened.last?.isEmpty == false {
            flattened.append("")
        }

        for (index, section) in sections.enumerated() {
            flattened.append(section.header)
            flattened.append(contentsOf: section.lines)
            if index < sections.count - 1 {
                flattened.append("")
            }
        }

        if !tail.isEmpty {
            if flattened.last?.isEmpty == false {
                flattened.append("")
            }
            flattened.append(contentsOf: tail)
        }
        return flattened
    }

    private static func fallbackSourceLines(_ item: FocusItem) -> [String] {
        var lines: [String] = []
        lines.append(contentsOf: item.detailIntroLines)
        if !item.detailIntroLines.isEmpty, item.detailIntroLines.last?.isEmpty == false, !item.detailSections.isEmpty {
            lines.append("")
        }
        for (index, section) in item.detailSections.enumerated() {
            lines.append(section.header)
            lines.append(contentsOf: section.lines)
            if index < item.detailSections.count - 1 {
                lines.append("")
            }
        }
        if !item.detailTailLines.isEmpty {
            if lines.last?.isEmpty == false {
                lines.append("")
            }
            lines.append(contentsOf: item.detailTailLines)
        }
        return lines
    }

    private static func isSectionHeader(_ line: String) -> Bool {
        guard !line.isEmpty else { return false }
        if line.hasPrefix("Recent conversations (last ") {
            return true
        }
        if line.hasPrefix("Meaningful topics from Codex") {
            return true
        }
        if line.hasPrefix("Conversation ") {
            return true
        }
        if line.hasPrefix("What are the Questions running in the Exec's (") && line.hasSuffix("Mind:") {
            return true
        }
        if line.hasPrefix("+Conversation ") {
            return true
        }
        if line.hasPrefix("## ") {
            return true
        }
        if line.hasSuffix(":"),
           line.count <= 120,
           !line.hasPrefix("-"),
           !line.hasPrefix("http://"),
           !line.hasPrefix("https://"),
           line.rangeOfCharacter(from: .letters) != nil {
            return true
        }
        return false
    }

    private static func isLocalHeuristicSectionHeader(_ line: String) -> Bool {
        let normalized = line.trimmingCharacters(in: .whitespacesAndNewlines).lowercased()
        return normalized.hasPrefix("conversation extraction source:")
            && normalized.contains("local heuristic")
    }

    private static func trimTrailingEmptyLines(_ lines: [String]) -> [String] {
        var values = lines
        while values.last?.trimmingCharacters(in: .whitespacesAndNewlines).isEmpty == true {
            values.removeLast()
        }
        return values
    }

    private static func trimLeadingAndTrailingEmptyLines(_ lines: [String]) -> [String] {
        var values = lines
        while values.first?.trimmingCharacters(in: .whitespacesAndNewlines).isEmpty == true {
            values.removeFirst()
        }
        while values.last?.trimmingCharacters(in: .whitespacesAndNewlines).isEmpty == true {
            values.removeLast()
        }
        return values
    }
}

enum FocusStableHash {
    static func hex(_ input: String) -> String {
        var hash: UInt64 = 1469598103934665603
        for byte in input.utf8 {
            hash ^= UInt64(byte)
            hash = hash &* 1099511628211
        }
        return String(format: "%016llx", hash)
    }
}
