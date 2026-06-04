import Foundation
import MetaCodable

/// Language/translation mode sent to the transcription backend.
enum TranscriptionLanguageMode: String, CaseIterable, Identifiable, Codable, Hashable, Sendable {
    case englishToEnglish = "english_to_english"
    case japaneseToEnglish = "japanese_to_english"
    case multilingualToEnglish = "multilingual_to_english"

    var id: String { rawValue }

    var displayName: String {
        switch self {
        case .englishToEnglish:
            return "English -> English"
        case .japaneseToEnglish:
            return "Japanese -> English"
        case .multilingualToEnglish:
            return "Multilingual -> English"
        }
    }

    static func normalized(_ rawValue: String?) -> TranscriptionLanguageMode {
        guard let rawValue = rawValue?.trimmingCharacters(in: .whitespacesAndNewlines),
              let value = TranscriptionLanguageMode(rawValue: rawValue) else {
            return .englishToEnglish
        }
        return value
    }
}

/// UI-facing state for the live transcription session.
enum TranscriptionConnectionStatus: Equatable, Sendable {
    case disabled
    case stopped
    case connecting
    case live
    case reconnecting
    case failed(String)

    var displayName: String {
        switch self {
        case .disabled:
            return "Disabled"
        case .stopped:
            return "Stopped"
        case .connecting:
            return "Connecting"
        case .live:
            return "Live"
        case .reconnecting:
            return "Reconnecting"
        case .failed:
            return "Failed"
        }
    }

    var detailText: String {
        switch self {
        case .disabled:
            return "Capture and network streaming are off."
        case .stopped:
            return "Ready, with no active transcription session."
        case .connecting:
            return "Opening the transcription session."
        case .live:
            return "Receiving live transcript events."
        case .reconnecting:
            return "Reconnecting while preserving finalized transcript."
        case .failed(let message):
            return message
        }
    }

    var symbolName: String {
        switch self {
        case .disabled:
            return "mic.slash"
        case .stopped:
            return "pause.circle"
        case .connecting:
            return "arrow.triangle.2.circlepath"
        case .live:
            return "waveform"
        case .reconnecting:
            return "wifi.exclamationmark"
        case .failed:
            return "exclamationmark.triangle"
        }
    }
}

/// Session-start contract shared by settings, WebSocket codec, and capture runtime.
@Codable
struct TranscriptionSessionConfig: Equatable, Sendable {
    @CodedAt("protocol_version")
    var protocolVersion: String
    @CodedAt("app_version")
    var appVersion: String?
    @CodedAt("session_id")
    var sessionID: String
    @CodedAt("endpoint_url")
    var endpointURL: String
    @CodedAt("transcription_enabled")
    var transcriptionEnabled: Bool
    @CodedAt("diarization_enabled")
    var diarizationEnabled: Bool
    @CodedAt("language_mode")
    var languageMode: TranscriptionLanguageMode
    @CodedAt("sample_rate")
    var sampleRate: Int
    @CodedAt("channel_count")
    var channelCount: Int
    @CodedAt("audio_encoding")
    var audioEncoding: String
    @CodedAt("client_timestamp")
    var clientTimestamp: Date
    @CodedAt("auth_token")
    var authToken: String?
    @CodedAt("privacy_safe_device_id")
    var privacySafeDeviceID: String?

    /// Builds a backend config from current system settings.
    init(
        settings: SystemSettings,
        appVersion: String? = Bundle.main.infoDictionary?["CFBundleShortVersionString"] as? String,
        sessionID: String = UUID().uuidString,
        clientTimestamp: Date = Date(),
        sampleRate: Int = Self.defaultSampleRate,
        channelCount: Int = Self.defaultChannelCount,
        audioEncoding: String = Self.defaultAudioEncoding,
        authToken: String? = nil,
        privacySafeDeviceID: String? = nil
    ) {
        self.protocolVersion = Self.currentProtocolVersion
        self.appVersion = appVersion
        self.sessionID = sessionID
        self.endpointURL = settings.transcriptionAWSEndpoint
            .trimmingCharacters(in: .whitespacesAndNewlines)
        self.transcriptionEnabled = settings.transcriptionEnabled
        self.diarizationEnabled = settings.transcriptionDiarizationEnabled
        self.languageMode = settings.transcriptionLanguageMode
        self.sampleRate = sampleRate
        self.channelCount = channelCount
        self.audioEncoding = audioEncoding
        self.clientTimestamp = clientTimestamp
        self.authToken = authToken ?? settings.transcriptionAuthToken
        self.privacySafeDeviceID = privacySafeDeviceID
    }
}

extension TranscriptionSessionConfig {
    static let currentProtocolVersion = "transcription.v1"
    static let defaultSampleRate = 16_000
    static let defaultChannelCount = 1
    static let defaultAudioEncoding = "pcm_s16le"
}

/// One transcript span, partial or final, optionally attributed to a speaker.
struct TranscriptSegment: Identifiable, Equatable, Sendable {
    var id: String
    var startTimeMilliseconds: Int
    var endTimeMilliseconds: Int?
    var text: String
    var isFinal: Bool
    var speakerID: String?
    var languageMode: TranscriptionLanguageMode
    var modelName: String?
    var modelVersion: String?
    var confidence: Double?
    var createdAt: Date

    var speakerLabel: String? {
        guard let speakerID = speakerID?.trimmingCharacters(in: .whitespacesAndNewlines),
              !speakerID.isEmpty else {
            return nil
        }
        if speakerID.lowercased().hasPrefix("speaker ") {
            return speakerID
        }
        return "Speaker \(speakerID)"
    }

    /// Returns a final copy without mutating the original segment.
    func finalized() -> TranscriptSegment {
        var updated = self
        updated.isFinal = true
        return updated
    }

    func withSpeakerID(_ speakerID: String?) -> TranscriptSegment {
        var updated = self
        updated.speakerID = speakerID
        return updated
    }

    func corrected(text: String, confidence: Double?) -> TranscriptSegment {
        var updated = self
        updated.text = text
        updated.confidence = confidence ?? updated.confidence
        updated.isFinal = true
        return updated
    }
}

/// Events emitted by the transcription backend over WebSocket.
enum TranscriptionServerEvent: Equatable, Sendable {
    case sessionStarted(sessionID: String)
    case partialTranscript(TranscriptSegment)
    case finalTranscript(TranscriptSegment)
    case speakerUpdate(segmentID: String, speakerID: String?)
    case diarizationStatus(String)
    case correctionUpdate(segmentID: String, text: String, confidence: Double?)
    case error(String)
    case sessionStopped
}

/// Maintains final/partial transcript state from an event stream.
struct TranscriptAggregator: Equatable, Sendable {
    private var finalizedSegments: [TranscriptSegment] = []
    private var partialSegmentsByID: [String: TranscriptSegment] = [:]

    var finalSegments: [TranscriptSegment] {
        finalizedSegments.sorted(by: segmentSort)
    }

    var partialSegments: [TranscriptSegment] {
        partialSegmentsByID.values.sorted(by: segmentSort)
    }

    var visibleSegments: [TranscriptSegment] {
        let finalIDs = Set(finalizedSegments.map(\.id))
        let visiblePartials = partialSegmentsByID.values
            .filter { !finalIDs.contains($0.id) }
        return (finalizedSegments + visiblePartials).sorted(by: segmentSort)
    }

    var livePartialText: String? {
        partialSegments.last?.text
    }

    /// Clears all accumulated transcript state.
    mutating func reset() {
        finalizedSegments.removeAll()
        partialSegmentsByID.removeAll()
    }

    /// Applies one backend event to the visible transcript model.
    mutating func apply(_ event: TranscriptionServerEvent) {
        switch event {
        case .partialTranscript(let segment):
            applyPartial(segment)
        case .finalTranscript(let segment):
            applyFinal(segment)
        case .speakerUpdate(let segmentID, let speakerID):
            updateSpeaker(segmentID: segmentID, speakerID: speakerID)
        case .correctionUpdate(let segmentID, let text, let confidence):
            applyCorrection(segmentID: segmentID, text: text, confidence: confidence)
        case .sessionStarted,
             .diarizationStatus,
             .error,
             .sessionStopped:
            break
        }
    }

    private mutating func applyPartial(_ segment: TranscriptSegment) {
        guard finalizedSegments.contains(where: { $0.id == segment.id }) == false else {
            return
        }
        var partial = segment
        partial.isFinal = false
        partialSegmentsByID[segment.id] = partial
    }

    private mutating func applyFinal(_ segment: TranscriptSegment) {
        let final = segment.finalized()
        partialSegmentsByID.removeValue(forKey: final.id)
        if let index = finalizedSegments.firstIndex(where: { $0.id == final.id }) {
            finalizedSegments[index] = final
        } else {
            finalizedSegments.append(final)
        }
    }

    private mutating func updateSpeaker(segmentID: String, speakerID: String?) {
        if let index = finalizedSegments.firstIndex(where: { $0.id == segmentID }) {
            finalizedSegments[index] = finalizedSegments[index].withSpeakerID(speakerID)
        }
        if let partial = partialSegmentsByID[segmentID] {
            partialSegmentsByID[segmentID] = partial.withSpeakerID(speakerID)
        }
    }

    private mutating func applyCorrection(segmentID: String, text: String, confidence: Double?) {
        if let index = finalizedSegments.firstIndex(where: { $0.id == segmentID }) {
            finalizedSegments[index] = finalizedSegments[index].corrected(text: text, confidence: confidence)
            return
        }
        if let partial = partialSegmentsByID[segmentID] {
            partialSegmentsByID[segmentID] = partial.corrected(text: text, confidence: confidence)
            applyFinal(partialSegmentsByID[segmentID] ?? partial)
        }
    }

    private func segmentSort(_ lhs: TranscriptSegment, _ rhs: TranscriptSegment) -> Bool {
        if lhs.startTimeMilliseconds == rhs.startTimeMilliseconds {
            return lhs.id < rhs.id
        }
        return lhs.startTimeMilliseconds < rhs.startTimeMilliseconds
    }
}
