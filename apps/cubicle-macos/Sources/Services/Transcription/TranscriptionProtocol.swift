import Foundation

enum TranscriptionProtocolError: LocalizedError {
    case invalidEndpoint(String)
    case emptyEndpoint
    case endpointContainsQuery
    case notConnected
    case invalidMessageEncoding
    case invalidJSON
    case missingField(String)
    case unsupportedEventType(String)
    case unsupportedMessageType
    case sessionStartRejected(String)
    case unexpectedSessionStartEvent(String)

    var errorDescription: String? {
        switch self {
        case .invalidEndpoint(let endpoint):
            return "Invalid transcription WebSocket endpoint: \(endpoint)"
        case .emptyEndpoint:
            return "Set an AWS transcription endpoint before starting a session."
        case .endpointContainsQuery:
            return "Transcription endpoint must not include query parameters."
        case .notConnected:
            return "Transcription WebSocket is not connected."
        case .invalidMessageEncoding:
            return "Transcription message was not valid UTF-8."
        case .invalidJSON:
            return "Transcription message was not valid JSON."
        case .missingField(let field):
            return "Transcription message is missing \(field)."
        case .unsupportedEventType(let type):
            return "Unsupported transcription event type: \(type)."
        case .unsupportedMessageType:
            return "Unsupported transcription WebSocket message type."
        case .sessionStartRejected(let message):
            return message
        case .unexpectedSessionStartEvent(let event):
            return "Transcription service did not confirm the session before audio capture started: \(event)."
        }
    }
}

enum TranscriptionWebSocketMessage: Equatable, Sendable {
    case text(String)
    case data(Data)
}

struct TranscriptionProtocolCodec: Sendable {
    func endpointURL(from rawEndpoint: String) throws -> URL {
        let trimmed = rawEndpoint.trimmingCharacters(in: .whitespacesAndNewlines)
        guard !trimmed.isEmpty else {
            throw TranscriptionProtocolError.emptyEndpoint
        }
        guard let components = URLComponents(string: trimmed),
              let scheme = components.scheme?.lowercased(),
              scheme == "ws" || scheme == "wss",
              let url = components.url else {
            throw TranscriptionProtocolError.invalidEndpoint(rawEndpoint)
        }
        if let query = components.percentEncodedQuery, !query.isEmpty {
            throw TranscriptionProtocolError.endpointContainsQuery
        }
        return url
    }

    func encodeStartSession(_ config: TranscriptionSessionConfig) throws -> String {
        var payload: [String: Any] = [
            "type": "start_session",
            "protocol_version": config.protocolVersion,
            "session_id": config.sessionID,
            "transcription_enabled": config.transcriptionEnabled,
            "diarization_enabled": config.diarizationEnabled,
            "language_mode": config.languageMode.rawValue,
            "sample_rate": config.sampleRate,
            "channel_count": config.channelCount,
            "audio_encoding": config.audioEncoding,
            "client_timestamp": Self.string(from: config.clientTimestamp)
        ]
        if let appVersion = clean(config.appVersion) {
            payload["app_version"] = appVersion
        }
        if let deviceID = clean(config.privacySafeDeviceID) {
            payload["privacy_safe_device_id"] = deviceID
        }
        return try encodeJSONObject(payload)
    }

    func authorizationHeaders(for config: TranscriptionSessionConfig) -> [String: String] {
        guard let authToken = clean(config.authToken) else {
            return [:]
        }
        return ["Authorization": "Bearer \(authToken)"]
    }

    func encodeStopSession(sessionID: String) throws -> String {
        try encodeJSONObject([
            "type": "stop_session",
            "protocol_version": TranscriptionSessionConfig.currentProtocolVersion,
            "session_id": sessionID,
            "client_timestamp": Self.string(from: Date())
        ])
    }

    func encodeAudioChunk(_ data: Data) -> Data {
        data
    }

    func decodeServerEvent(from message: TranscriptionWebSocketMessage) throws -> TranscriptionServerEvent {
        switch message {
        case .text(let text):
            guard let data = text.data(using: .utf8) else {
                throw TranscriptionProtocolError.invalidMessageEncoding
            }
            return try decodeServerEvent(from: data)
        case .data(let data):
            return try decodeServerEvent(from: data)
        }
    }

    func decodeServerEvent(from data: Data) throws -> TranscriptionServerEvent {
        guard let raw = try JSONSerialization.jsonObject(with: data) as? [String: Any] else {
            throw TranscriptionProtocolError.invalidJSON
        }
        guard let type = stringValue(raw["type"] ?? raw["event"]) else {
            throw TranscriptionProtocolError.missingField("type")
        }

        switch type {
        case "session_started":
            return .sessionStarted(sessionID: try requiredString("session_id", in: raw))
        case "partial_transcript":
            return .partialTranscript(try decodeSegment(from: raw, isFinalFallback: false))
        case "final_transcript":
            return .finalTranscript(try decodeSegment(from: raw, isFinalFallback: true))
        case "speaker_update":
            return .speakerUpdate(
                segmentID: try requiredString("segment_id", in: raw),
                speakerID: stringValue(raw["speaker_id"])
            )
        case "diarization_status":
            return .diarizationStatus(diarizationStatusText(from: raw))
        case "correction_update":
            return .correctionUpdate(
                segmentID: try requiredString("segment_id", in: raw),
                text: try requiredString("text", in: raw),
                confidence: doubleValue(raw["confidence"])
            )
        case "error":
            return .error(
                stringValue(raw["message"])
                    ?? stringValue(raw["error"])
                    ?? "Transcription service error."
            )
        case "session_stopped":
            return .sessionStopped
        default:
            throw TranscriptionProtocolError.unsupportedEventType(type)
        }
    }

    private func diarizationStatusText(from raw: [String: Any]) -> String {
        let status = stringValue(raw["status"])
            ?? stringValue(raw["message"])
            ?? "updated"
        switch status {
        case "completed":
            var parts = ["completed"]
            if let speakerCount = intValue(raw["speaker_count"]) {
                parts.append("\(speakerCount) speaker\(speakerCount == 1 ? "" : "s")")
            }
            if let turnCount = intValue(raw["speaker_turn_count"]) {
                parts.append("\(turnCount) turn\(turnCount == 1 ? "" : "s")")
            }
            if let segmentCount = intValue(raw["segment_count"]) {
                parts.append("\(segmentCount) segment\(segmentCount == 1 ? "" : "s")")
            }
            return parts.joined(separator: " - ")
        case "timed_out":
            if let timeoutSeconds = doubleValue(raw["timeout_seconds"]) {
                return "timed out after \(secondsText(timeoutSeconds))"
            }
            return "timed out"
        case "failed":
            if let errorClass = stringValue(raw["error_class"]) {
                return "failed - \(errorClass)"
            }
            return "failed"
        case "unavailable":
            var parts = ["speaker labels unavailable"]
            let provider = stringValue(raw["provider"])
            if let provider, !provider.isEmpty {
                parts.append("\(provider) provider")
            }
            if let reason = stringValue(raw["reason"]), !reason.isEmpty {
                let reasonText = reason.replacingOccurrences(of: "_", with: " ")
                if reasonText != "\(provider ?? "") provider" {
                    parts.append(reasonText)
                }
            }
            return parts.joined(separator: " - ")
        default:
            return status
        }
    }

    private func secondsText(_ seconds: Double) -> String {
        if seconds.rounded() == seconds {
            return "\(Int(seconds))s"
        }
        return String(format: "%.1fs", seconds)
    }

    private func decodeSegment(from raw: [String: Any], isFinalFallback: Bool) throws -> TranscriptSegment {
        let languageMode = TranscriptionLanguageMode.normalized(stringValue(raw["language_mode"]))
        return TranscriptSegment(
            id: try requiredString("segment_id", in: raw),
            startTimeMilliseconds: try requiredInt("start_time_ms", in: raw),
            endTimeMilliseconds: intValue(raw["end_time_ms"]),
            text: try requiredString("text", in: raw),
            isFinal: boolValue(raw["is_final"]) ?? !((boolValue(raw["is_partial"]) ?? !isFinalFallback)),
            speakerID: stringValue(raw["speaker_id"]),
            languageMode: languageMode,
            modelName: stringValue(raw["model_name"]),
            modelVersion: stringValue(raw["model_version"]),
            confidence: doubleValue(raw["confidence"]),
            createdAt: dateValue(raw["created_at"]) ?? Date()
        )
    }

    private func encodeJSONObject(_ object: [String: Any]) throws -> String {
        let data = try JSONSerialization.data(withJSONObject: object, options: [.sortedKeys])
        guard let string = String(data: data, encoding: .utf8) else {
            throw TranscriptionProtocolError.invalidMessageEncoding
        }
        return string
    }

    private func requiredString(_ key: String, in raw: [String: Any]) throws -> String {
        guard let value = stringValue(raw[key]), !value.isEmpty else {
            throw TranscriptionProtocolError.missingField(key)
        }
        return value
    }

    private func requiredInt(_ key: String, in raw: [String: Any]) throws -> Int {
        guard let value = intValue(raw[key]) else {
            throw TranscriptionProtocolError.missingField(key)
        }
        return value
    }

    private func stringValue(_ value: Any?) -> String? {
        switch value {
        case let value as String:
            return value
        case let value as CustomStringConvertible:
            return value.description
        default:
            return nil
        }
    }

    private func intValue(_ value: Any?) -> Int? {
        switch value {
        case let value as Int:
            return value
        case let value as NSNumber:
            return value.intValue
        case let value as String:
            return Int(value)
        default:
            return nil
        }
    }

    private func doubleValue(_ value: Any?) -> Double? {
        switch value {
        case let value as Double:
            return value
        case let value as NSNumber:
            return value.doubleValue
        case let value as String:
            return Double(value)
        default:
            return nil
        }
    }

    private func boolValue(_ value: Any?) -> Bool? {
        switch value {
        case let value as Bool:
            return value
        case let value as NSNumber:
            return value.boolValue
        case let value as String:
            return Bool(value)
        default:
            return nil
        }
    }

    private func dateValue(_ value: Any?) -> Date? {
        guard let rawValue = stringValue(value) else {
            return nil
        }
        return ISO8601DateFormatter.transcriptionProtocolFormatter.date(from: rawValue)
            ?? ISO8601DateFormatter().date(from: rawValue)
    }

    private func clean(_ value: String?) -> String? {
        guard let value = value?.trimmingCharacters(in: .whitespacesAndNewlines),
              !value.isEmpty else {
            return nil
        }
        return value
    }

    private static func string(from date: Date) -> String {
        ISO8601DateFormatter.transcriptionProtocolFormatter.string(from: date)
    }
}

private extension ISO8601DateFormatter {
    static let transcriptionProtocolFormatter: ISO8601DateFormatter = {
        let formatter = ISO8601DateFormatter()
        formatter.formatOptions = [.withInternetDateTime, .withFractionalSeconds]
        return formatter
    }()
}
