import XCTest
@testable import GetWebexSpaceMacApp

final class MetaCodableDTOTests: XCTestCase {
    func testTranscriptionSessionConfigDecodesAndRoundTripsSnakeCaseKeys() throws {
        let data = Data(
            """
            {
              "protocol_version": "transcription.v1",
              "app_version": "1.2.3",
              "session_id": "session-123",
              "endpoint_url": "wss://transcription.example.com/session",
              "transcription_enabled": true,
              "diarization_enabled": true,
              "language_mode": "multilingual_to_english",
              "sample_rate": 16000,
              "channel_count": 1,
              "audio_encoding": "pcm_s16le",
              "client_timestamp": 0,
              "auth_token": "session-token",
              "privacy_safe_device_id": "device-abc"
            }
            """.utf8
        )

        let decoded = try JSONDecoder().decode(TranscriptionSessionConfig.self, from: data)

        XCTAssertEqual(decoded.protocolVersion, "transcription.v1")
        XCTAssertEqual(decoded.appVersion, "1.2.3")
        XCTAssertEqual(decoded.sessionID, "session-123")
        XCTAssertEqual(decoded.endpointURL, "wss://transcription.example.com/session")
        XCTAssertTrue(decoded.transcriptionEnabled)
        XCTAssertTrue(decoded.diarizationEnabled)
        XCTAssertEqual(decoded.languageMode, .multilingualToEnglish)
        XCTAssertEqual(decoded.sampleRate, 16_000)
        XCTAssertEqual(decoded.channelCount, 1)
        XCTAssertEqual(decoded.audioEncoding, "pcm_s16le")
        XCTAssertEqual(decoded.clientTimestamp, Date(timeIntervalSinceReferenceDate: 0))
        XCTAssertEqual(decoded.authToken, "session-token")
        XCTAssertEqual(decoded.privacySafeDeviceID, "device-abc")

        let roundTripData = try JSONEncoder().encode(decoded)
        let roundTrip = try jsonObject(from: roundTripData)

        XCTAssertEqual(roundTrip["protocol_version"] as? String, "transcription.v1")
        XCTAssertEqual(roundTrip["app_version"] as? String, "1.2.3")
        XCTAssertEqual(roundTrip["session_id"] as? String, "session-123")
        XCTAssertEqual(roundTrip["endpoint_url"] as? String, "wss://transcription.example.com/session")
        XCTAssertEqual(roundTrip["transcription_enabled"] as? Bool, true)
        XCTAssertEqual(roundTrip["diarization_enabled"] as? Bool, true)
        XCTAssertEqual(roundTrip["language_mode"] as? String, "multilingual_to_english")
        XCTAssertEqual(roundTrip["sample_rate"] as? Int, 16_000)
        XCTAssertEqual(roundTrip["channel_count"] as? Int, 1)
        XCTAssertEqual(roundTrip["audio_encoding"] as? String, "pcm_s16le")
        XCTAssertEqual(roundTrip["auth_token"] as? String, "session-token")
        XCTAssertEqual(roundTrip["privacy_safe_device_id"] as? String, "device-abc")
        XCTAssertNil(roundTrip["protocolVersion"])
        XCTAssertNil(roundTrip["sessionID"])
        XCTAssertNil(roundTrip["endpointURL"])
    }

    func testSystemSettingsPersistenceEncodesMetaCodableSnakeCaseKeys() throws {
        let runtimeRoot = temporaryRuntimeRoot(label: "metacodable-settings")
        defer { try? FileManager.default.removeItem(at: runtimeRoot) }
        let configStore = ConfigStore(configuration: testRuntimeConfiguration(runtimeRoot: runtimeRoot))
        var settings = SystemSettings()
        settings.debug = true
        settings.webexSyncEnabled = false
        settings.codexModel = .gpt54Mini
        settings.codexReasoningLevel = .high
        settings.personFocusDays = 21
        settings.spaceFocusAnalysisCadenceHours = 12
        settings.transcriptionEnabled = true
        settings.transcriptionLanguageMode = .japaneseToEnglish
        settings.transcriptionAWSEndpoint = "wss://transcription.example.com/session"

        try configStore.saveSystemSettings(settings)

        let payload = try Data(contentsOf: configStore.systemSettingsURL)
        let root = try jsonObject(from: payload)
        let persistedSettings = try XCTUnwrap(root["settings"] as? [String: Any])
        let loaded = configStore.loadSystemSettings()

        XCTAssertEqual(root["version"] as? Int, SystemSettings.persistedVersion)
        XCTAssertNotNil(root["updated_at"])
        XCTAssertNil(root["updatedAt"])
        for key in SystemSettingKey.allCases {
            XCTAssertNotNil(persistedSettings[key.rawValue], "Missing persisted key \(key.rawValue)")
        }
        XCTAssertEqual(persistedSettings["webex_sync_enabled"] as? Bool, false)
        XCTAssertEqual(persistedSettings["codex_model"] as? String, "gpt-5.4-mini")
        XCTAssertEqual(persistedSettings["codex_reasoning_level"] as? String, "high")
        XCTAssertEqual(persistedSettings["person_focus_days"] as? Int, 21)
        XCTAssertEqual(persistedSettings["space_focus_analysis_cadence_hours"] as? Int, 12)
        XCTAssertEqual(persistedSettings["transcription_enabled"] as? Bool, true)
        XCTAssertEqual(persistedSettings["transcription_language_mode"] as? String, "japanese_to_english")
        XCTAssertEqual(persistedSettings["transcription_aws_endpoint"] as? String, "wss://transcription.example.com/session")
        XCTAssertNil(persistedSettings["webexSyncEnabled"])
        XCTAssertNil(persistedSettings["codexReasoningLevel"])
        XCTAssertNil(persistedSettings["transcriptionAWSEndpoint"])
        XCTAssertEqual(loaded.webexSyncEnabled, false)
        XCTAssertEqual(loaded.codexModel, .gpt54Mini)
        XCTAssertEqual(loaded.codexReasoningLevel, .high)
        XCTAssertEqual(loaded.personFocusDays, 21)
        XCTAssertEqual(loaded.spaceFocusAnalysisCadenceHours, 12)
        XCTAssertTrue(loaded.transcriptionEnabled)
        XCTAssertEqual(loaded.transcriptionLanguageMode, .japaneseToEnglish)
        XCTAssertEqual(loaded.transcriptionAWSEndpoint, "wss://transcription.example.com/session")
    }

    func testAskCodexQueryHistoryDecodesAndRoundTripsVersionedMetaCodablePayload() throws {
        let runtimeRoot = temporaryRuntimeRoot(label: "metacodable-ask-history")
        defer { try? FileManager.default.removeItem(at: runtimeRoot) }
        let configStore = ConfigStore(configuration: testRuntimeConfiguration(runtimeRoot: runtimeRoot))
        let entries = [
            AskCodexQueryHistoryEntry(
                id: "history-1",
                question: "What changed?",
                targetScope: .selectedSpace,
                targetTitle: "Space: Product",
                targetKey: "space-product",
                targetItemID: "spacefocus:room-product",
                submittedAt: "2026-06-03T12:00:00Z"
            )
        ]

        try configStore.saveAskCodexQueryHistory(entries)

        let payload = try Data(contentsOf: configStore.askCodexQueryHistoryURL)
        let root = try jsonObject(from: payload)
        let queries = try XCTUnwrap(root["queries"] as? [[String: Any]])

        XCTAssertEqual(root["version"] as? Int, 1)
        XCTAssertNotNil(root["updated_at"])
        XCTAssertNil(root["updatedAt"])
        XCTAssertEqual(queries.first?["targetScope"] as? String, "selected_space")
        XCTAssertEqual(configStore.loadAskCodexQueryHistory(), entries)
    }

    private func jsonObject(from data: Data) throws -> [String: Any] {
        try XCTUnwrap(JSONSerialization.jsonObject(with: data) as? [String: Any])
    }
}
