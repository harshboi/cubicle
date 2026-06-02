import XCTest
import SQLite3
import WebexQuestionGeneratorCore
@testable import GetWebexSpaceMacApp

final class QuestionEngineCoreIntegrationTests: XCTestCase {
    func testOAuthAppSettingsLoadsOutlookRuntimeConfig() throws {
        let runtimeRoot = FileManager.default.temporaryDirectory
            .appendingPathComponent("CubicleOAuthSettingsTests-\(UUID().uuidString)", isDirectory: true)
        defer { try? FileManager.default.removeItem(at: runtimeRoot) }
        let configuration = RuntimeConfiguration(
            runtimeRoot: runtimeRoot,
            codexExecutable: "codex",
            webexBaseURL: URL(string: "https://webexapis.com/v1")!,
            webexPageSize: 100,
            webexRetryCount: 0,
            webexTimeoutSeconds: 1,
            webexOAuthTokenPathOverride: nil,
            webexOAuthRefreshSkewSeconds: 300,
            webexOAuthRefreshTokenSkewSeconds: 86_400
        )
        let configStore = ConfigStore(configuration: configuration)
        try FileManager.default.createDirectory(
            at: configStore.configDirectory,
            withIntermediateDirectories: true
        )
        let payload = """
        {
          "outlook": {
            "client_id": "outlook-client-id",
            "redirect_uri": "http://127.0.0.1:8788/callback",
            "scope": "offline_access User.Read Mail.Read",
            "tenant": "organizations"
          },
          "MS_GRAPH_OAUTH_CLIENT_SECRET": "fake-secret"
        }
        """
        try payload.data(using: .utf8)?.write(to: configStore.oauthSettingsURL)

        let settings = try configStore.oauthAppSettings(provider: .outlook)

        XCTAssertEqual(settings.sourceFile, configStore.oauthSettingsURL)
        XCTAssertEqual(settings.clientID, "outlook-client-id")
        XCTAssertEqual(settings.clientSecret, "fake-secret")
        XCTAssertEqual(settings.redirectURI, "http://127.0.0.1:8788/callback")
        XCTAssertEqual(settings.scope, "offline_access User.Read Mail.Read")
        XCTAssertEqual(settings.tenant, "organizations")
    }

    func testRuntimeConfigurationResolvesCodexExecutableOverride() throws {
        let previousValue = ProcessInfo.processInfo.environment["CODEX_BIN"]
        setenv("CODEX_BIN", "/tmp/cubicle-codex", 1)
        defer {
            if let previousValue {
                setenv("CODEX_BIN", previousValue, 1)
            } else {
                unsetenv("CODEX_BIN")
            }
        }

        XCTAssertEqual(RuntimeConfiguration.current.codexExecutable, "/tmp/cubicle-codex")
    }

    func testRuntimeConfigurationUsesDesktopRuntimeRootWhenEnvIsMissing() throws {
        let homeRoot = FileManager.default.temporaryDirectory
            .appendingPathComponent("CubicleRuntimeHomeTests-\(UUID().uuidString)", isDirectory: true)
        defer { try? FileManager.default.removeItem(at: homeRoot) }
        let desktopRuntimeRoot = homeRoot
            .appendingPathComponent("Desktop", isDirectory: true)
            .appendingPathComponent("getwebexspace-data", isDirectory: true)
        try FileManager.default.createDirectory(
            at: desktopRuntimeRoot,
            withIntermediateDirectories: true
        )

        let configuration = RuntimeConfiguration.resolved(
            environment: ["HOME": homeRoot.path],
            fileExists: { $0 == desktopRuntimeRoot.path }
        )

        XCTAssertEqual(configuration.runtimeRoot.path, desktopRuntimeRoot.path)
    }

    func testSystemSettingsPersistsCodexModelAndReasoning() throws {
        let runtimeRoot = FileManager.default.temporaryDirectory
            .appendingPathComponent("CubicleSystemSettingsTests-\(UUID().uuidString)", isDirectory: true)
        defer { try? FileManager.default.removeItem(at: runtimeRoot) }
        let configuration = RuntimeConfiguration(
            runtimeRoot: runtimeRoot,
            codexExecutable: "codex",
            webexBaseURL: URL(string: "https://webexapis.com/v1")!,
            webexPageSize: 100,
            webexRetryCount: 0,
            webexTimeoutSeconds: 1,
            webexOAuthTokenPathOverride: nil,
            webexOAuthRefreshSkewSeconds: 300,
            webexOAuthRefreshTokenSkewSeconds: 86_400
        )
        let configStore = ConfigStore(configuration: configuration)
        var settings = SystemSettings()
        settings.codexModel = .gpt54Mini
        settings.codexReasoningLevel = .high
        settings.priorityRefreshPausesBackground = false
        settings.webexSyncEnabled = false
        settings.codexEnabled = false
        settings.codexAskEnabled = false
        settings.codexQuestionSynthesisEnabled = false
        settings.codexPersonSummariesEnabled = false
        settings.codexSpaceSummariesEnabled = false
        settings.codexClusterTitlesEnabled = false
        settings.codexExecQuestionsEnabled = false
        settings.codexBeliefsEnabled = false
        settings.webexSyncMinutes = 7

        try configStore.saveSystemSettings(settings)
        let loaded = configStore.loadSystemSettings()
        let payload = try String(contentsOf: configStore.systemSettingsURL, encoding: .utf8)

        XCTAssertEqual(loaded.codexModel, .gpt54Mini)
        XCTAssertEqual(loaded.codexReasoningLevel, .high)
        XCTAssertFalse(loaded.priorityRefreshPausesBackground)
        XCTAssertFalse(loaded.webexSyncEnabled)
        XCTAssertFalse(loaded.codexEnabled)
        XCTAssertFalse(loaded.codexAskEnabled)
        XCTAssertFalse(loaded.codexQuestionSynthesisEnabled)
        XCTAssertFalse(loaded.codexPersonSummariesEnabled)
        XCTAssertFalse(loaded.codexSpaceSummariesEnabled)
        XCTAssertFalse(loaded.codexClusterTitlesEnabled)
        XCTAssertFalse(loaded.codexExecQuestionsEnabled)
        XCTAssertFalse(loaded.codexBeliefsEnabled)
        XCTAssertEqual(loaded.webexSyncMinutes, 7)
        XCTAssertEqual(loaded.pollSeconds, 420)
        XCTAssertTrue(payload.contains(#""codex_model" : "gpt-5.4-mini""#))
        XCTAssertTrue(payload.contains(#""codex_reasoning_level" : "high""#))
        XCTAssertTrue(payload.contains(#""priority_refresh_pauses_background" : false"#))
        XCTAssertTrue(payload.contains(#""webex_sync_enabled" : false"#))
        XCTAssertTrue(payload.contains(#""webex_sync_minutes" : 7"#))
        XCTAssertTrue(payload.contains(#""poll_seconds" : 420"#))
        XCTAssertTrue(payload.contains(#""codex_enabled" : false"#))
        XCTAssertTrue(payload.contains(#""codex_question_synthesis_enabled" : false"#))
    }

    func testSystemSettingsPersistsTranscriptionControls() throws {
        let runtimeRoot = FileManager.default.temporaryDirectory
            .appendingPathComponent("CubicleTranscriptionSettingsTests-\(UUID().uuidString)", isDirectory: true)
        defer { try? FileManager.default.removeItem(at: runtimeRoot) }
        let configuration = testRuntimeConfiguration(runtimeRoot: runtimeRoot)
        let configStore = ConfigStore(configuration: configuration)
        var settings = SystemSettings()
        XCTAssertFalse(settings.transcriptionEnabled)

        settings.transcriptionEnabled = true
        settings.transcriptionDiarizationEnabled = true
        settings.transcriptionLanguageMode = .japaneseToEnglish
        settings.transcriptionMicrophoneGain = 27
        settings.transcriptionAWSEndpoint = "wss://transcription.staging.example.com/session"

        try configStore.saveSystemSettings(settings)
        let loaded = configStore.loadSystemSettings()
        let payload = try String(contentsOf: configStore.systemSettingsURL, encoding: .utf8)

        XCTAssertTrue(loaded.transcriptionEnabled)
        XCTAssertTrue(loaded.transcriptionDiarizationEnabled)
        XCTAssertEqual(loaded.transcriptionLanguageMode, .japaneseToEnglish)
        XCTAssertEqual(loaded.transcriptionMicrophoneGain, 27)
        XCTAssertEqual(loaded.transcriptionAWSEndpoint, "wss://transcription.staging.example.com/session")
        XCTAssertTrue(payload.contains(#""transcription_enabled" : true"#))
        XCTAssertTrue(payload.contains(#""transcription_diarization_enabled" : true"#))
        XCTAssertTrue(payload.contains(#""transcription_language_mode" : "japanese_to_english""#))
        XCTAssertTrue(payload.contains(#""transcription_microphone_gain" : 27"#))
        XCTAssertTrue(payload.contains(#""transcription_aws_endpoint" : "wss:\/\/transcription.staging.example.com\/session""#))
    }

    @MainActor
    func testDisabledTranscriptionDoesNotStartAudioOrConnection() async throws {
        let client = MockTranscriptionClient(scriptedEvents: [])
        let audio = NoopAudioCaptureService()
        let viewModel = TranscriptionViewModel(client: client, audioCaptureService: audio)
        var settings = SystemSettings()
        settings.transcriptionEnabled = false
        settings.transcriptionAWSEndpoint = "wss://transcription.example.com/session"
        viewModel.apply(settings: settings)

        await viewModel.startSessionForCurrentSettings()

        XCTAssertEqual(client.startCallCount, 0)
        XCTAssertEqual(audio.startCallCount, 0)
        XCTAssertEqual(viewModel.status, .disabled)
    }

    @MainActor
    func testEnabledTranscriptionBuildsSessionConfigWithModeAndDiarization() async throws {
        let client = MockTranscriptionClient(scriptedEvents: [])
        let audio = NoopAudioCaptureService()
        let viewModel = TranscriptionViewModel(client: client, audioCaptureService: audio)
        var settings = SystemSettings()
        settings.transcriptionEnabled = true
        settings.transcriptionDiarizationEnabled = true
        settings.transcriptionLanguageMode = .japaneseToEnglish
        settings.transcriptionMicrophoneGain = 24
        settings.transcriptionAWSEndpoint = "wss://transcription.example.com/session"
        viewModel.apply(settings: settings)

        await viewModel.startSessionForCurrentSettings()

        let config = try XCTUnwrap(client.lastConfig)
        XCTAssertEqual(client.startCallCount, 1)
        XCTAssertEqual(audio.startCallCount, 1)
        XCTAssertTrue(config.transcriptionEnabled)
        XCTAssertTrue(config.diarizationEnabled)
        XCTAssertEqual(config.languageMode, .japaneseToEnglish)
        XCTAssertEqual(config.endpointURL, "wss://transcription.example.com/session")
        XCTAssertEqual(config.sampleRate, 16_000)
        XCTAssertEqual(config.channelCount, 1)
        XCTAssertEqual(config.audioEncoding, "pcm_s16le")
        XCTAssertEqual(audio.lastMicrophoneGainMultiplier, 24)
    }

    @MainActor
    func testEnabledTranscriptionStreamsCapturedAudioChunksToClient() async throws {
        let client = StreamingTranscriptionClient()
        let audioChunk = Data([0x01, 0x02, 0x03, 0x04])
        let audio = SingleChunkAudioCaptureService(chunk: audioChunk)
        let viewModel = TranscriptionViewModel(client: client, audioCaptureService: audio)
        var settings = SystemSettings()
        settings.transcriptionEnabled = true
        settings.transcriptionMicrophoneGain = 12
        settings.transcriptionAWSEndpoint = "wss://transcription.example.com/session"
        viewModel.apply(settings: settings)

        await viewModel.startSessionForCurrentSettings()

        XCTAssertEqual(audio.startCallCount, 1)
        XCTAssertEqual(audio.lastMicrophoneGainMultiplier, 12)
        XCTAssertEqual(client.audioChunks, [audioChunk])
        XCTAssertEqual(viewModel.audioChunksSent, 1)
        XCTAssertEqual(viewModel.audioBytesSent, audioChunk.count)
        XCTAssertTrue(viewModel.audioStatusText?.contains("1 frames") ?? false)
        XCTAssertTrue(viewModel.audioStatusText?.contains("in rms") ?? false)
        XCTAssertEqual(viewModel.lastAudioTelemetry.appliedGain, 1.0)
    }

    func testTranscriptAggregatorUpdatesPartialInPlaceAndLocksFinalWithoutDuplicates() {
        var aggregator = TranscriptAggregator()
        aggregator.apply(.partialTranscript(makeTranscriptSegment(id: "s1", text: "We should", isFinal: false)))
        aggregator.apply(.partialTranscript(makeTranscriptSegment(id: "s1", text: "We should ship today.", isFinal: false)))

        XCTAssertEqual(aggregator.visibleSegments.count, 1)
        XCTAssertEqual(aggregator.visibleSegments.first?.text, "We should ship today.")
        XCTAssertFalse(aggregator.visibleSegments.first?.isFinal ?? true)

        aggregator.apply(.finalTranscript(makeTranscriptSegment(id: "s1", text: "We should ship today.", isFinal: true)))
        aggregator.apply(.finalTranscript(makeTranscriptSegment(id: "s1", text: "We should ship today.", isFinal: true)))
        aggregator.apply(.partialTranscript(makeTranscriptSegment(id: "s1", text: "stale partial", isFinal: false)))

        XCTAssertEqual(aggregator.visibleSegments.count, 1)
        XCTAssertEqual(aggregator.visibleSegments.first?.text, "We should ship today.")
        XCTAssertTrue(aggregator.visibleSegments.first?.isFinal ?? false)
        XCTAssertEqual(aggregator.partialSegments.count, 0)
    }

    func testTranscriptAggregatorAppliesSpeakerAndCorrectionUpdatesBySegmentID() {
        var aggregator = TranscriptAggregator()
        aggregator.apply(.finalTranscript(makeTranscriptSegment(id: "s2", text: "initial text", isFinal: true)))
        aggregator.apply(.speakerUpdate(segmentID: "s2", speakerID: "2"))
        aggregator.apply(.correctionUpdate(segmentID: "s2", text: "corrected text", confidence: 0.98))

        let segment = aggregator.visibleSegments.first
        XCTAssertEqual(segment?.text, "corrected text")
        XCTAssertEqual(segment?.speakerLabel, "Speaker 2")
        XCTAssertEqual(segment?.confidence, 0.98)
        XCTAssertTrue(segment?.isFinal ?? false)
    }

    @MainActor
    func testTranscriptionViewModelStopDrainsFinalTranscriptAndSpeakerUpdate() async throws {
        let client = StopFinalizingTranscriptionClient()
        let audio = NoopAudioCaptureService()
        let viewModel = TranscriptionViewModel(client: client, audioCaptureService: audio)
        var settings = SystemSettings()
        settings.transcriptionEnabled = true
        settings.transcriptionDiarizationEnabled = true
        settings.transcriptionAWSEndpoint = "wss://transcription.example.com/session"
        viewModel.apply(settings: settings)

        await viewModel.startSessionForCurrentSettings()
        try await Task.sleep(nanoseconds: 20_000_000)

        XCTAssertEqual(viewModel.visibleSegments.first?.text, "draft text")
        XCTAssertFalse(viewModel.visibleSegments.first?.isFinal ?? true)

        await viewModel.stopSession()

        let segment = viewModel.visibleSegments.first
        XCTAssertEqual(segment?.text, "final text")
        XCTAssertEqual(segment?.speakerLabel, "Speaker 2")
        XCTAssertTrue(segment?.isFinal ?? false)
        XCTAssertEqual(audio.stopCallCount, 1)
    }

    @MainActor
    func testTranscriptionViewModelMarksStoppedBeforeDelayedFinalDrain() async throws {
        let client = DelayedStopFinalizingTranscriptionClient(stopDelayNanoseconds: 200_000_000)
        let audio = NoopAudioCaptureService()
        let viewModel = TranscriptionViewModel(client: client, audioCaptureService: audio)
        var settings = SystemSettings()
        settings.transcriptionEnabled = true
        settings.transcriptionDiarizationEnabled = true
        settings.transcriptionAWSEndpoint = "wss://transcription.example.com/session"
        viewModel.apply(settings: settings)

        await viewModel.startSessionForCurrentSettings()
        try await Task.sleep(nanoseconds: 20_000_000)
        XCTAssertEqual(viewModel.status, .live)

        let stopTask = Task { @MainActor in
            await viewModel.stopSession()
        }
        try await Task.sleep(nanoseconds: 50_000_000)

        XCTAssertEqual(audio.stopCallCount, 1)
        XCTAssertEqual(viewModel.status, .stopped)
        XCTAssertTrue(viewModel.isStoppingSession)
        XCTAssertEqual(viewModel.visibleSegments.first?.text, "draft text")
        XCTAssertFalse(viewModel.visibleSegments.first?.isFinal ?? true)

        await stopTask.value

        let segment = viewModel.visibleSegments.first
        XCTAssertEqual(segment?.text, "delayed final text")
        XCTAssertEqual(segment?.speakerLabel, "Speaker 3")
        XCTAssertTrue(segment?.isFinal ?? false)
        XCTAssertFalse(viewModel.isStoppingSession)
        XCTAssertEqual(viewModel.status, .stopped)
    }

    @MainActor
    func testTranscriptionViewModelFormatsTranscriptForTimelineSubmission() async throws {
        let client = MockTranscriptionClient(scriptedEvents: [
            .finalTranscript(makeTranscriptSegment(
                id: "s1",
                text: "First decision is final.",
                isFinal: true,
                startTimeMilliseconds: 0,
                endTimeMilliseconds: 1_200,
                speakerID: "1"
            )),
            .partialTranscript(makeTranscriptSegment(
                id: "s2",
                text: "Follow-up is still being discussed.",
                isFinal: false,
                startTimeMilliseconds: 1_300,
                endTimeMilliseconds: nil,
                speakerID: "2"
            ))
        ])
        let viewModel = TranscriptionViewModel(client: client, audioCaptureService: NoopAudioCaptureService())
        var settings = SystemSettings()
        settings.transcriptionEnabled = true
        settings.transcriptionAWSEndpoint = "wss://transcription.example.com/session"
        viewModel.apply(settings: settings)

        await viewModel.startSessionForCurrentSettings()
        try await Task.sleep(nanoseconds: 20_000_000)

        let submissionText = viewModel.transcriptSubmissionText
        XCTAssertTrue(viewModel.hasTranscriptForSubmission)
        XCTAssertTrue(submissionText.contains("[0.0s-1.2s] Speaker 1: First decision is final."))
        XCTAssertTrue(submissionText.contains("[1.3s] Speaker 2 (partial): Follow-up is still being discussed."))
    }

    @MainActor
    func testReconnectStatePreservesFinalTranscriptSegments() async throws {
        let client = MockTranscriptionClient(scriptedEvents: [
            .finalTranscript(makeTranscriptSegment(id: "s3", text: "stable final", isFinal: true))
        ])
        let audio = NoopAudioCaptureService()
        let viewModel = TranscriptionViewModel(client: client, audioCaptureService: audio)
        var settings = SystemSettings()
        settings.transcriptionEnabled = true
        settings.transcriptionAWSEndpoint = "wss://transcription.example.com/session"
        viewModel.apply(settings: settings)

        await viewModel.startSessionForCurrentSettings()
        try await Task.sleep(nanoseconds: 20_000_000)
        viewModel.simulateReconnectForTesting()

        XCTAssertEqual(viewModel.status, .reconnecting)
        XCTAssertEqual(viewModel.visibleSegments.count, 1)
        XCTAssertEqual(viewModel.visibleSegments.first?.text, "stable final")
        XCTAssertTrue(viewModel.visibleSegments.first?.isFinal ?? false)
    }

    @MainActor
    func testDisablingWhileLiveStopsAudioAndConnection() async throws {
        let client = HoldingTranscriptionClient()
        let audio = NoopAudioCaptureService()
        let viewModel = TranscriptionViewModel(client: client, audioCaptureService: audio)
        var settings = SystemSettings()
        settings.transcriptionEnabled = true
        settings.transcriptionAWSEndpoint = "wss://transcription.example.com/session"
        viewModel.apply(settings: settings)

        await viewModel.startSessionForCurrentSettings()
        XCTAssertEqual(viewModel.status, .live)

        settings.transcriptionEnabled = false
        viewModel.apply(settings: settings)
        try await Task.sleep(nanoseconds: 20_000_000)

        XCTAssertEqual(client.stopCallCount, 1)
        XCTAssertEqual(audio.stopCallCount, 1)
        XCTAssertEqual(viewModel.status, .disabled)
    }

    @MainActor
    func testUnexpectedTranscriptionStreamFinishRestartsAudioAndConnection() async throws {
        let client = RestartingTranscriptionClient()
        let audio = NoopAudioCaptureService()
        let viewModel = TranscriptionViewModel(
            client: client,
            audioCaptureService: audio,
            reconnectDelayNanoseconds: 0,
            maxReconnectAttempts: 1
        )
        var settings = SystemSettings()
        settings.transcriptionEnabled = true
        settings.transcriptionAWSEndpoint = "wss://transcription.example.com/session"
        viewModel.apply(settings: settings)

        await viewModel.startSessionForCurrentSettings()
        try await Task.sleep(nanoseconds: 50_000_000)

        XCTAssertEqual(client.startCallCount, 2)
        XCTAssertEqual(client.stopCallCount, 1)
        XCTAssertEqual(audio.startCallCount, 2)
        XCTAssertEqual(audio.stopCallCount, 1)
        XCTAssertEqual(viewModel.status, .live)

        await viewModel.stopSession()
    }

    func testTranscriptionProtocolStartSessionEncodesVersionedMetadata() throws {
        var settings = SystemSettings()
        settings.transcriptionEnabled = true
        settings.transcriptionDiarizationEnabled = true
        settings.transcriptionLanguageMode = .japaneseToEnglish
        settings.transcriptionAWSEndpoint = "wss://transcription.example.com/session"
        let config = TranscriptionSessionConfig(
            settings: settings,
            appVersion: "1.2.3",
            sessionID: "session-123",
            clientTimestamp: Date(timeIntervalSince1970: 0),
            authToken: "short-lived-token",
            privacySafeDeviceID: "device-abc"
        )
        let payload = try TranscriptionProtocolCodec().encodeStartSession(config)
        let json = try decodeTranscriptionJSONObject(payload)

        XCTAssertEqual(json["type"] as? String, "start_session")
        XCTAssertEqual(json["protocol_version"] as? String, "transcription.v1")
        XCTAssertEqual(json["app_version"] as? String, "1.2.3")
        XCTAssertEqual(json["session_id"] as? String, "session-123")
        XCTAssertEqual(json["transcription_enabled"] as? Bool, true)
        XCTAssertEqual(json["diarization_enabled"] as? Bool, true)
        XCTAssertEqual(json["language_mode"] as? String, "japanese_to_english")
        XCTAssertEqual(json["sample_rate"] as? Int, 16_000)
        XCTAssertEqual(json["channel_count"] as? Int, 1)
        XCTAssertEqual(json["audio_encoding"] as? String, "pcm_s16le")
        XCTAssertEqual(json["privacy_safe_device_id"] as? String, "device-abc")
        XCTAssertNil(json["auth_token"])
        XCTAssertNil(json["endpoint_url"])
        XCTAssertEqual(
            TranscriptionProtocolCodec().authorizationHeaders(for: config)["Authorization"],
            "Bearer short-lived-token"
        )
    }

    func testTranscriptionProtocolRejectsQueryStringEndpoints() throws {
        let codec = TranscriptionProtocolCodec()
        XCTAssertNoThrow(try codec.endpointURL(from: "wss://transcription.example.com/session"))
        XCTAssertThrowsError(try codec.endpointURL(from: "https://transcription.example.com/session")) { error in
            guard case TranscriptionProtocolError.invalidEndpoint = error else {
                return XCTFail("Expected invalid endpoint, got \(error)")
            }
        }
        XCTAssertThrowsError(try codec.endpointURL(from: "wss://transcription.example.com/session?token=plain")) { error in
            guard case TranscriptionProtocolError.endpointContainsQuery = error else {
                return XCTFail("Expected query rejection, got \(error)")
            }
        }
    }

    func testTranscriptionProtocolDecodesServerEventContract() throws {
        let codec = TranscriptionProtocolCodec()
        let partialEvent = try codec.decodeServerEvent(from: .text("""
        {
          "type": "partial_transcript",
          "segment_id": "seg-1",
          "start_time_ms": 120,
          "end_time_ms": 760,
          "text": "partial text",
          "is_partial": true,
          "speaker_id": "1",
          "language_mode": "english_to_english",
          "model_name": "faster-whisper",
          "model_version": "large-v3-turbo",
          "confidence": 0.72,
          "created_at": "2026-05-17T16:00:00.000Z"
        }
        """))
        let finalEvent = try codec.decodeServerEvent(from: .text("""
        {
          "type": "final_transcript",
          "segment_id": "seg-1",
          "start_time_ms": 120,
          "end_time_ms": 960,
          "text": "final text",
          "is_final": true,
          "speaker_id": "2",
          "language_mode": "japanese_to_english",
          "model_name": "faster-whisper",
          "model_version": "large-v3-turbo",
          "confidence": 0.94,
          "created_at": "2026-05-17T16:00:02.000Z"
        }
        """))
        let speakerEvent = try codec.decodeServerEvent(from: .text("""
        {"type":"speaker_update","segment_id":"seg-1","speaker_id":"3"}
        """))
        let diarizationCompletedEvent = try codec.decodeServerEvent(from: .text("""
        {"type":"diarization_status","status":"completed","speaker_count":2,"speaker_turn_count":7,"segment_count":3}
        """))
        let diarizationTimedOutEvent = try codec.decodeServerEvent(from: .text("""
        {"type":"diarization_status","status":"timed_out","timeout_seconds":45}
        """))
        let diarizationUnavailableEvent = try codec.decodeServerEvent(from: .text("""
        {"type":"diarization_status","status":"unavailable","provider":"mock","reason":"mock_provider"}
        """))
        let correctionEvent = try codec.decodeServerEvent(from: .text("""
        {"type":"correction_update","segment_id":"seg-1","text":"corrected final","confidence":0.98}
        """))

        guard case .partialTranscript(let partial) = partialEvent else {
            return XCTFail("Expected partial transcript event")
        }
        XCTAssertEqual(partial.id, "seg-1")
        XCTAssertEqual(partial.text, "partial text")
        XCTAssertFalse(partial.isFinal)
        XCTAssertEqual(partial.speakerLabel, "Speaker 1")
        XCTAssertEqual(partial.languageMode, .englishToEnglish)

        guard case .finalTranscript(let final) = finalEvent else {
            return XCTFail("Expected final transcript event")
        }
        XCTAssertEqual(final.text, "final text")
        XCTAssertTrue(final.isFinal)
        XCTAssertEqual(final.speakerLabel, "Speaker 2")
        XCTAssertEqual(final.languageMode, .japaneseToEnglish)
        XCTAssertEqual(final.confidence, 0.94)

        XCTAssertEqual(speakerEvent, .speakerUpdate(segmentID: "seg-1", speakerID: "3"))
        XCTAssertEqual(diarizationCompletedEvent, .diarizationStatus("completed - 2 speakers - 7 turns - 3 segments"))
        XCTAssertEqual(diarizationTimedOutEvent, .diarizationStatus("timed out after 45s"))
        XCTAssertEqual(diarizationUnavailableEvent, .diarizationStatus("speaker labels unavailable - mock provider"))
        XCTAssertEqual(correctionEvent, .correctionUpdate(segmentID: "seg-1", text: "corrected final", confidence: 0.98))
    }

    func testTranscriptionWebSocketClientUsesLocalMockServerForSessionAndAudioFrames() async throws {
        var settings = SystemSettings()
        settings.transcriptionEnabled = true
        settings.transcriptionDiarizationEnabled = true
        settings.transcriptionLanguageMode = .japaneseToEnglish
        settings.transcriptionAWSEndpoint = "wss://transcription.example.com/session"
        let config = TranscriptionSessionConfig(
            settings: settings,
            appVersion: "test-app",
            sessionID: "client-session",
            clientTimestamp: Date(timeIntervalSince1970: 0),
            authToken: "secure-session-token"
        )
        let server = LocalMockTranscriptionWebSocketServer(scriptedMessages: [
            .text(#"{"type":"session_started","session_id":"server-session"}"#),
            .text(#"{"type":"partial_transcript","segment_id":"seg-1","start_time_ms":0,"text":"hello","is_partial":true,"language_mode":"japanese_to_english","model_name":"mock-asr","model_version":"slice-2","created_at":"2026-05-17T16:00:00.000Z"}"#),
            .text(#"{"type":"final_transcript","segment_id":"seg-1","start_time_ms":0,"end_time_ms":900,"text":"hello world","is_final":true,"speaker_id":"1","language_mode":"japanese_to_english","model_name":"mock-asr","model_version":"slice-2","confidence":0.91,"created_at":"2026-05-17T16:00:01.000Z"}"#)
        ])
        let client = TranscriptionWebSocketClient(
            transportFactory: {
                LocalMockTranscriptionWebSocketTransport(server: server)
            }
        )

        let stream = try await client.startSession(config: config)
        try await client.sendAudioChunk(Data([0x01, 0x02, 0x03]))

        var events: [TranscriptionServerEvent] = []
        for await event in stream {
            events.append(event)
            if events.count == 3 {
                break
            }
        }
        await client.stopSession()

        XCTAssertEqual(events.count, 3)
        XCTAssertEqual(events.first, .sessionStarted(sessionID: "server-session"))
        guard case .finalTranscript(let finalSegment) = events.last else {
            return XCTFail("Expected final transcript event")
        }
        XCTAssertEqual(finalSegment.text, "hello world")
        XCTAssertEqual(finalSegment.speakerLabel, "Speaker 1")
        XCTAssertEqual(finalSegment.languageMode, .japaneseToEnglish)

        let snapshot = await server.snapshot()
        XCTAssertEqual(snapshot.connectedURLs.first?.absoluteString, "wss://transcription.example.com/session")
        XCTAssertEqual(snapshot.connectionHeaders.first?["Authorization"], "Bearer secure-session-token")
        XCTAssertTrue(snapshot.isClosed)
        XCTAssertEqual(snapshot.receivedMessages.count, 3)
        guard case .text(let startPayload) = snapshot.receivedMessages[0] else {
            return XCTFail("Expected start session text payload")
        }
        let startJSON = try decodeTranscriptionJSONObject(startPayload)
        XCTAssertEqual(startJSON["type"] as? String, "start_session")
        XCTAssertEqual(startJSON["diarization_enabled"] as? Bool, true)
        XCTAssertEqual(startJSON["language_mode"] as? String, "japanese_to_english")
        guard case .data(let audioPayload) = snapshot.receivedMessages[1] else {
            return XCTFail("Expected binary audio payload")
        }
        XCTAssertEqual(audioPayload, Data([0x01, 0x02, 0x03]))
        guard case .text(let stopPayload) = snapshot.receivedMessages[2] else {
            return XCTFail("Expected stop session text payload")
        }
        XCTAssertEqual(try decodeTranscriptionJSONObject(stopPayload)["type"] as? String, "stop_session")
    }

    func testTranscriptionWebSocketClientRejectsUnsafeEndpointBeforeConnecting() async throws {
        var settings = SystemSettings()
        settings.transcriptionEnabled = true
        settings.transcriptionAWSEndpoint = "wss://transcription.example.com/session?token=plain"
        let config = TranscriptionSessionConfig(settings: settings)
        let server = LocalMockTranscriptionWebSocketServer(scriptedMessages: [])
        let client = TranscriptionWebSocketClient(
            transportFactory: {
                LocalMockTranscriptionWebSocketTransport(server: server)
            }
        )

        do {
            _ = try await client.startSession(config: config)
            XCTFail("Expected query-string endpoint rejection")
        } catch TranscriptionProtocolError.endpointContainsQuery {
            let snapshot = await server.snapshot()
            XCTAssertTrue(snapshot.connectedURLs.isEmpty)
        } catch {
            XCTFail("Expected endpointContainsQuery, got \(error)")
        }
    }

    func testTranscriptionWebSocketClientRequiresSessionStartedBeforeAudioCanStream() async throws {
        var settings = SystemSettings()
        settings.transcriptionEnabled = true
        settings.transcriptionAWSEndpoint = "wss://transcription.example.com/session"
        let config = TranscriptionSessionConfig(settings: settings)
        let server = LocalMockTranscriptionWebSocketServer(scriptedMessages: [
            .text(#"{"type":"partial_transcript","segment_id":"early","start_time_ms":0,"text":"too early","is_partial":true,"language_mode":"english_to_english","model_name":"mock-asr","model_version":"slice-2","created_at":"2026-05-17T16:00:00.000Z"}"#)
        ])
        let client = TranscriptionWebSocketClient(
            transportFactory: {
                LocalMockTranscriptionWebSocketTransport(server: server)
            }
        )

        do {
            _ = try await client.startSession(config: config)
            XCTFail("Expected the client to require session_started before returning")
        } catch TranscriptionProtocolError.unexpectedSessionStartEvent(let eventName) {
            XCTAssertEqual(eventName, "partial_transcript")
        } catch {
            XCTFail("Expected unexpectedSessionStartEvent, got \(error)")
        }

        let snapshot = await server.snapshot()
        XCTAssertEqual(snapshot.receivedMessages.count, 1)
        guard case .text(let startPayload) = snapshot.receivedMessages[0] else {
            return XCTFail("Expected start session text payload")
        }
        XCTAssertEqual(try decodeTranscriptionJSONObject(startPayload)["type"] as? String, "start_session")
        XCTAssertTrue(snapshot.isClosed)
    }

    func testFocusAnalysisSettingsNormalizeWindowAndCadenceBounds() throws {
        let runtimeRoot = FileManager.default.temporaryDirectory
            .appendingPathComponent("CubicleFocusAnalysisSettingsBoundsTests-\(UUID().uuidString)", isDirectory: true)
        defer { try? FileManager.default.removeItem(at: runtimeRoot) }
        let configuration = testRuntimeConfiguration(runtimeRoot: runtimeRoot)
        let configStore = ConfigStore(configuration: configuration)
        var settings = SystemSettings()
        settings.personFocusDays = 1
        settings.spaceFocusDays = 3650
        settings.personFocusAnalysisCadenceHours = 0
        settings.spaceFocusAnalysisCadenceHours = 240

        try configStore.saveSystemSettings(settings)
        let loaded = configStore.loadSystemSettings()
        let payload = try String(contentsOf: configStore.systemSettingsURL, encoding: .utf8)

        XCTAssertEqual(loaded.personFocusDays, 7)
        XCTAssertEqual(loaded.spaceFocusDays, 90)
        XCTAssertEqual(loaded.personFocusAnalysisCadenceHours, 1)
        XCTAssertEqual(loaded.spaceFocusAnalysisCadenceHours, 168)
        XCTAssertTrue(payload.contains(#""person_focus_days" : 7"#))
        XCTAssertTrue(payload.contains(#""space_focus_days" : 90"#))
        XCTAssertTrue(payload.contains(#""person_focus_analysis_cadence_hours" : 1"#))
        XCTAssertTrue(payload.contains(#""space_focus_analysis_cadence_hours" : 168"#))
    }

    @MainActor
    func testFocusAnalysisDraftChangesDoNotPersistOrTriggerCodex() throws {
        let runtimeRoot = FileManager.default.temporaryDirectory
            .appendingPathComponent("CubicleFocusAnalysisDraftTests-\(UUID().uuidString)", isDirectory: true)
        defer { try? FileManager.default.removeItem(at: runtimeRoot) }
        let configuration = testRuntimeConfiguration(runtimeRoot: runtimeRoot)
        let configStore = ConfigStore(configuration: configuration)
        var settings = SystemSettings()
        settings.personFocusDays = 30
        settings.personFocusAnalysisCadenceHours = 24
        try configStore.saveSystemSettings(settings)

        let fakeCodexURL = runtimeRoot.appendingPathComponent("fake-codex")
        let codexMarkerURL = runtimeRoot.appendingPathComponent("codex-launched")
        try "#!/bin/sh\ntouch '\(codexMarkerURL.path)'\n".write(to: fakeCodexURL, atomically: true, encoding: .utf8)
        try FileManager.default.setAttributes(
            [.posixPermissions: NSNumber(value: Int16(0o755))],
            ofItemAtPath: fakeCodexURL.path
        )
        let model = AppModel(runtimeStore: NativeRuntimeStore(configuration: testRuntimeConfiguration(
            runtimeRoot: runtimeRoot,
            codexExecutable: fakeCodexURL.path
        )))
        model.reloadSystemSettings()

        model.updateFocusAnalysisDraft(.personFocusDays, intValue: 45)
        model.updateFocusAnalysisDraft(.personFocusAnalysisCadenceHours, intValue: 12)
        let persisted = configStore.loadSystemSettings()

        XCTAssertEqual(model.focusAnalysisDraft.personFocusDays, 45)
        XCTAssertEqual(model.focusAnalysisDraft.personFocusAnalysisCadenceHours, 12)
        XCTAssertEqual(model.systemSettings.personFocusDays, 30)
        XCTAssertEqual(persisted.personFocusDays, 30)
        XCTAssertEqual(persisted.personFocusAnalysisCadenceHours, 24)
        XCTAssertFalse(FileManager.default.fileExists(atPath: codexMarkerURL.path))
    }

    func testExactFocusAnalysisCacheStatusReusesMatchingManifest() throws {
        let runtimeRoot = FileManager.default.temporaryDirectory
            .appendingPathComponent("CubicleExactFocusAnalysisCacheTests-\(UUID().uuidString)", isDirectory: true)
        defer { try? FileManager.default.removeItem(at: runtimeRoot) }
        let configuration = testRuntimeConfiguration(runtimeRoot: runtimeRoot)
        let configStore = ConfigStore(configuration: configuration)
        var settings = SystemSettings()
        settings.spaceFocusDays = 14
        settings.spaceFocusAnalysisCadenceHours = 6
        try configStore.saveSystemSettings(settings)
        let knowledgeDirectory = runtimeRoot.appendingPathComponent("knowledge", isDirectory: true)
        try writeFocusCache(
            makeFocusCache(kind: .space, days: 14, itemID: "spacefocus:room-14", title: "Room 14"),
            to: knowledgeDirectory.appendingPathComponent("space_focus_cache_14d.json")
        )
        let store = NativeRuntimeStore(configuration: configuration)

        let outcome = try store.refreshSpaceFocusCache(forceRebuild: true)
        let status = store.focusAnalysisCacheStatus(kind: .space, focusDays: 14, analysisCadenceHours: 6)
        let loaded = try XCTUnwrap(store.loadExactFocusAnalysisCache(kind: .space, focusDays: 14, analysisCadenceHours: 6))
        let manifest = try String(contentsOf: store.nativeManifestURL(kind: .space, focusDays: 14), encoding: .utf8)

        XCTAssertFalse(outcome.reusedCache)
        XCTAssertTrue(status.canUseExactCache)
        XCTAssertEqual(status.availability, .exactCache)
        XCTAssertEqual(loaded.focusDays, 14)
        XCTAssertTrue(manifest.contains(#""analysis_bucket""#))
        XCTAssertTrue(manifest.contains(#""raw_evidence_hash""#))
        XCTAssertTrue(manifest.contains(#""message_ids_hash""#))
        XCTAssertTrue(manifest.contains(#""generation_type" : "full""#))
    }

    func testPromptModelMismatchPreventsExactFocusAnalysisReuse() throws {
        let runtimeRoot = FileManager.default.temporaryDirectory
            .appendingPathComponent("CubicleFocusAnalysisMismatchTests-\(UUID().uuidString)", isDirectory: true)
        defer { try? FileManager.default.removeItem(at: runtimeRoot) }
        let configuration = testRuntimeConfiguration(runtimeRoot: runtimeRoot)
        let configStore = ConfigStore(configuration: configuration)
        var settings = SystemSettings()
        settings.personFocusDays = 21
        settings.personFocusAnalysisCadenceHours = 12
        settings.codexModel = .gpt55
        try configStore.saveSystemSettings(settings)
        let knowledgeDirectory = runtimeRoot.appendingPathComponent("knowledge", isDirectory: true)
        try writeFocusCache(
            makeFocusCache(kind: .person, days: 21, itemID: "personfocus:alice@example.com", title: "Alice"),
            to: knowledgeDirectory.appendingPathComponent("person_focus_cache_21d.json")
        )
        let store = NativeRuntimeStore(configuration: configuration)
        _ = try store.refreshPersonFocusCache(forceRebuild: true)

        settings.codexModel = .gpt54Mini
        try configStore.saveSystemSettings(settings)
        let status = store.focusAnalysisCacheStatus(kind: .person, focusDays: 21, analysisCadenceHours: 12)

        XCTAssertFalse(status.canUseExactCache)
        XCTAssertEqual(status.availability, .needsRefresh)
        XCTAssertTrue(status.summary.contains("incompatible"))
    }

    func testNarrowerWindowDoesNotReuseWiderFocusAnalysisCache() throws {
        let runtimeRoot = FileManager.default.temporaryDirectory
            .appendingPathComponent("CubicleFocusAnalysisNarrowerWindowTests-\(UUID().uuidString)", isDirectory: true)
        defer { try? FileManager.default.removeItem(at: runtimeRoot) }
        let configuration = testRuntimeConfiguration(runtimeRoot: runtimeRoot)
        let configStore = ConfigStore(configuration: configuration)
        var settings = SystemSettings()
        settings.spaceFocusDays = 30
        settings.spaceFocusAnalysisCadenceHours = 24
        try configStore.saveSystemSettings(settings)
        let knowledgeDirectory = runtimeRoot.appendingPathComponent("knowledge", isDirectory: true)
        try writeFocusCache(
            makeFocusCache(kind: .space, days: 30, itemID: "spacefocus:wide-room", title: "Wide Room"),
            to: knowledgeDirectory.appendingPathComponent("space_focus_cache_30d.json")
        )
        try writeFocusCache(
            makeFocusCache(kind: .space, days: 7, itemID: "spacefocus:narrow-room", title: "Narrow Room"),
            to: knowledgeDirectory.appendingPathComponent("space_focus_cache_7d.json")
        )
        let store = NativeRuntimeStore(configuration: configuration)
        _ = try store.refreshSpaceFocusCache(forceRebuild: true)

        let status = store.focusAnalysisCacheStatus(kind: .space, focusDays: 7, analysisCadenceHours: 24)

        XCTAssertFalse(status.canUseExactCache)
        XCTAssertEqual(status.availability, .needsRefresh)
        XCTAssertNil(status.outputSnapshotPath)
    }

    func testWebexSyncMinutesControlsRefreshPlanCadenceAndMigratesPollSeconds() throws {
        let runtimeRoot = FileManager.default.temporaryDirectory
            .appendingPathComponent("CubicleWebexSyncMinutesTests-\(UUID().uuidString)", isDirectory: true)
        defer { try? FileManager.default.removeItem(at: runtimeRoot) }
        let configuration = RuntimeConfiguration(
            runtimeRoot: runtimeRoot,
            codexExecutable: "codex",
            webexBaseURL: URL(string: "https://webexapis.com/v1")!,
            webexPageSize: 100,
            webexRetryCount: 0,
            webexTimeoutSeconds: 1,
            webexOAuthTokenPathOverride: nil,
            webexOAuthRefreshSkewSeconds: 300,
            webexOAuthRefreshTokenSkewSeconds: 86_400
        )
        let configStore = ConfigStore(configuration: configuration)
        var settings = SystemSettings()
        settings.webexSyncMinutes = 11
        try configStore.saveSystemSettings(settings)

        let webexPlan = NativeRefreshCoordinator(configuration: configuration)
            .defaultPlans()
            .first { $0.scope == .webexSync }

        XCTAssertEqual(webexPlan?.cadenceSeconds, 660)

        let legacyPayload = """
        {
          "version" : 4,
          "updated_at" : "2026-05-15T17:30:00Z",
          "settings" : {
            "poll_seconds" : 420
          }
        }
        """
        try FileManager.default.createDirectory(at: configStore.configDirectory, withIntermediateDirectories: true)
        try legacyPayload.write(to: configStore.systemSettingsURL, atomically: true, encoding: .utf8)

        let migrated = configStore.loadSystemSettings()
        XCTAssertEqual(migrated.webexSyncMinutes, 7)
        XCTAssertEqual(migrated.pollSeconds, 420)
    }

    func testRuntimeStatusDetectsFallbackSnapshotsOutsideConfiguredDays() throws {
        let runtimeRoot = FileManager.default.temporaryDirectory
            .appendingPathComponent("CubicleRuntimeStatusFallbackTests-\(UUID().uuidString)", isDirectory: true)
        defer { try? FileManager.default.removeItem(at: runtimeRoot) }

        let configuration = RuntimeConfiguration(
            runtimeRoot: runtimeRoot,
            codexExecutable: "codex",
            webexBaseURL: URL(string: "https://webexapis.com/v1")!,
            webexPageSize: 100,
            webexRetryCount: 0,
            webexTimeoutSeconds: 1,
            webexOAuthTokenPathOverride: nil,
            webexOAuthRefreshSkewSeconds: 300,
            webexOAuthRefreshTokenSkewSeconds: 86_400
        )
        let store = NativeRuntimeStore(configuration: configuration)
        let fallbackPersonCacheURL = runtimeRoot
            .appendingPathComponent("knowledge", isDirectory: true)
            .appendingPathComponent("person_focus_cache_2d.json")
        try writeFocusCache(FocusCache.empty(kind: .person), to: fallbackPersonCacheURL)

        let status = store.runtimeStatus()
        XCTAssertTrue(status.knowledgeDirectoryExists)
        XCTAssertTrue(status.personSnapshotExists)
        XCTAssertFalse(status.spaceSnapshotExists)
    }

    func testRuntimeStatusDetectsConfiguredLiveSnapshots() throws {
        let runtimeRoot = FileManager.default.temporaryDirectory
            .appendingPathComponent("CubicleRuntimeStatusLiveSnapshotTests-\(UUID().uuidString)", isDirectory: true)
        defer { try? FileManager.default.removeItem(at: runtimeRoot) }

        let configuration = RuntimeConfiguration(
            runtimeRoot: runtimeRoot,
            codexExecutable: "codex",
            webexBaseURL: URL(string: "https://webexapis.com/v1")!,
            webexPageSize: 100,
            webexRetryCount: 0,
            webexTimeoutSeconds: 1,
            webexOAuthTokenPathOverride: nil,
            webexOAuthRefreshSkewSeconds: 300,
            webexOAuthRefreshTokenSkewSeconds: 86_400
        )
        let configStore = ConfigStore(configuration: configuration)
        var settings = configStore.loadSystemSettings()
        settings.personFocusDays = 30
        settings.spaceFocusDays = 60
        try configStore.saveSystemSettings(settings)

        let nativeDirectory = runtimeRoot
            .appendingPathComponent("knowledge", isDirectory: true)
            .appendingPathComponent("native", isDirectory: true)
        try writeFocusCache(
            FocusCache.empty(kind: .person),
            to: nativeDirectory.appendingPathComponent("live_person_focus_cache_30d.json")
        )
        try writeFocusCache(
            FocusCache.empty(kind: .space),
            to: nativeDirectory.appendingPathComponent("live_space_focus_cache_60d.json")
        )

        let status = NativeRuntimeStore(configuration: configuration).runtimeStatus()
        XCTAssertTrue(status.knowledgeDirectoryExists)
        XCTAssertTrue(status.personSnapshotExists)
        XCTAssertTrue(status.spaceSnapshotExists)
    }

    func testLoadFocusCachePrefersLiveSnapshotWhenFallbackCandidatesAreOversized() throws {
        let runtimeRoot = FileManager.default.temporaryDirectory
            .appendingPathComponent("CubicleLargeFallbackSkipTests-\(UUID().uuidString)", isDirectory: true)
        defer { try? FileManager.default.removeItem(at: runtimeRoot) }

        let configuration = RuntimeConfiguration(
            runtimeRoot: runtimeRoot,
            codexExecutable: "codex",
            webexBaseURL: URL(string: "https://webexapis.com/v1")!,
            webexPageSize: 100,
            webexRetryCount: 0,
            webexTimeoutSeconds: 1,
            webexOAuthTokenPathOverride: nil,
            webexOAuthRefreshSkewSeconds: 300,
            webexOAuthRefreshTokenSkewSeconds: 86_400
        )
        let configStore = ConfigStore(configuration: configuration)
        var settings = configStore.loadSystemSettings()
        settings.spaceFocusDays = 7
        try configStore.saveSystemSettings(settings)

        let knowledgeDirectory = runtimeRoot.appendingPathComponent("knowledge", isDirectory: true)
        let nativeDirectory = knowledgeDirectory.appendingPathComponent("native", isDirectory: true)
        try FileManager.default.createDirectory(at: nativeDirectory, withIntermediateDirectories: true)

        let liveCache = FocusCache.empty(kind: .space)
        try writeFocusCache(liveCache, to: nativeDirectory.appendingPathComponent("live_space_focus_cache_7d.json"))

        let hugeText = String(repeating: "x", count: 3_000_000)
        let oversizedFallbackCache = FocusCache(
            focusDays: 7,
            items: [
                FocusItem(
                    id: "spacefocus:oversized",
                    title: "Oversized fallback",
                    subtitle: hugeText,
                    meta: "auto-reply=no | messages=1",
                    timestamp: "2026-05-16T23:00:00.000Z",
                    badge: "space",
                    statusBadge: "live-webex",
                    detailLines: [hugeText],
                    detailIntroLines: [],
                    detailSections: [],
                    detailTailLines: []
                )
            ],
            updatedAt: "2026-05-16T23:00:00.000Z",
            countLabel: "1",
            recentMessages: 1,
            summaryGenerationInProgress: false,
            subjectsProcessed: 1,
            subjectsTotal: 1
        )
        try writeFocusCache(oversizedFallbackCache, to: knowledgeDirectory.appendingPathComponent("space_focus_cache_7d.json"))

        let loaded = try NativeRuntimeStore(configuration: configuration).loadFocusCache(kind: .space)
        XCTAssertEqual(loaded.items.count, 0)
        XCTAssertEqual(loaded.focusDays, 7)
    }

    func testCodexMasterSwitchDisablesEveryFeature() {
        var settings = SystemSettings()
        settings.codexEnabled = false
        settings.codexAskEnabled = true
        settings.codexQuestionSynthesisEnabled = true
        settings.codexPersonSummariesEnabled = true
        settings.codexSpaceSummariesEnabled = true
        settings.codexClusterTitlesEnabled = true
        settings.codexExecQuestionsEnabled = true
        settings.codexBeliefsEnabled = true

        XCTAssertFalse(settings.codexFeatureEnabled(.askCodex))
        XCTAssertFalse(settings.codexFeatureEnabled(.questionSynthesis))
        XCTAssertFalse(settings.codexFeatureEnabled(.personFocusSummaries))
        XCTAssertFalse(settings.codexFeatureEnabled(.spaceFocusSummaries))
        XCTAssertFalse(settings.codexFeatureEnabled(.clusterTitles))
        XCTAssertFalse(settings.codexFeatureEnabled(.execQuestions))
        XCTAssertFalse(settings.codexFeatureEnabled(.beliefs))
    }

    @MainActor
    func testBackgroundSummariesFlagStopsAutomaticRefreshImmediately() throws {
        let runtimeRoot = FileManager.default.temporaryDirectory
            .appendingPathComponent("CubicleBackgroundFlagTests-\(UUID().uuidString)", isDirectory: true)
        defer { try? FileManager.default.removeItem(at: runtimeRoot) }
        let configuration = RuntimeConfiguration(
            runtimeRoot: runtimeRoot,
            codexExecutable: "codex",
            webexBaseURL: URL(string: "https://webexapis.com/v1")!,
            webexPageSize: 100,
            webexRetryCount: 0,
            webexTimeoutSeconds: 1,
            webexOAuthTokenPathOverride: nil,
            webexOAuthRefreshSkewSeconds: 300,
            webexOAuthRefreshTokenSkewSeconds: 86_400
        )
        let model = AppModel(runtimeStore: NativeRuntimeStore(configuration: configuration))
        model.reloadSystemSettings()
        model.startBackgroundRefreshIfNeeded()

        XCTAssertTrue(model.backgroundRefreshActive)

        model.updateSystemSetting(.backgroundStatus, boolValue: false)

        XCTAssertFalse(model.backgroundRefreshActive)
        XCTAssertTrue(model.visibleStartupRefreshPlanTitlesForTesting().isEmpty)
    }

    @MainActor
    func testVisibleRefreshPlansRespectWebexAndCodexFeatureFlags() throws {
        let runtimeRoot = FileManager.default.temporaryDirectory
            .appendingPathComponent("CubicleVisibleRefreshPlanTests-\(UUID().uuidString)", isDirectory: true)
        defer { try? FileManager.default.removeItem(at: runtimeRoot) }
        let configuration = RuntimeConfiguration(
            runtimeRoot: runtimeRoot,
            codexExecutable: "codex",
            webexBaseURL: URL(string: "https://webexapis.com/v1")!,
            webexPageSize: 100,
            webexRetryCount: 0,
            webexTimeoutSeconds: 1,
            webexOAuthTokenPathOverride: nil,
            webexOAuthRefreshSkewSeconds: 300,
            webexOAuthRefreshTokenSkewSeconds: 86_400
        )
        let configStore = ConfigStore(configuration: configuration)
        var settings = SystemSettings()
        settings.webexSyncEnabled = false
        settings.codexEnabled = false
        try configStore.saveSystemSettings(settings)

        let model = AppModel(runtimeStore: NativeRuntimeStore(configuration: configuration))
        model.reloadSystemSettings()

        XCTAssertFalse(model.visibleStartupRefreshPlanTitlesForTesting().contains("Webex sync"))
        XCTAssertFalse(model.visibleStartupRefreshPlanTitlesForTesting().contains("Person summaries"))
        XCTAssertFalse(model.visibleStartupRefreshPlanTitlesForTesting().contains("Space summaries and exec questions"))
        XCTAssertTrue(model.visibleStartupRefreshPlanTitlesForTesting().contains("Questions from fresh evidence"))
        XCTAssertTrue(model.visiblePriorityRefreshPlanTitlesForTesting(section: .spaceFocusTargets).isEmpty)
        XCTAssertTrue(model.visiblePriorityRefreshPlanTitlesForTesting(section: .personFocusTargets).isEmpty)

        settings.webexSyncEnabled = true
        settings.codexEnabled = true
        try configStore.saveSystemSettings(settings)
        model.reloadSystemSettings()

        XCTAssertTrue(model.visibleStartupRefreshPlanTitlesForTesting().contains("Webex sync"))
        XCTAssertTrue(model.visibleStartupRefreshPlanTitlesForTesting().contains("Person summaries"))
        XCTAssertTrue(model.visibleStartupRefreshPlanTitlesForTesting().contains("Space summaries and exec questions"))
        XCTAssertEqual(model.visiblePriorityRefreshPlanTitlesForTesting(section: .spaceFocusTargets), ["Webex map + snapshots"])
    }

    func testDisabledWebexSyncSkipsBeforeOAuthOrNetwork() async throws {
        let runtimeRoot = FileManager.default.temporaryDirectory
            .appendingPathComponent("CubicleDisabledWebexSyncTests-\(UUID().uuidString)", isDirectory: true)
        defer { try? FileManager.default.removeItem(at: runtimeRoot) }
        let configuration = RuntimeConfiguration(
            runtimeRoot: runtimeRoot,
            codexExecutable: "codex",
            webexBaseURL: URL(string: "https://127.0.0.1:9/v1")!,
            webexPageSize: 100,
            webexRetryCount: 0,
            webexTimeoutSeconds: 1,
            webexOAuthTokenPathOverride: nil,
            webexOAuthRefreshSkewSeconds: 300,
            webexOAuthRefreshTokenSkewSeconds: 86_400
        )
        var settings = SystemSettings()
        settings.webexSyncEnabled = false
        try ConfigStore(configuration: configuration).saveSystemSettings(settings)
        let coordinator = NativeRefreshCoordinator(configuration: configuration)

        let mapResult = try await coordinator.refreshWebexMapFile()
        let result = try await coordinator.refresh(.webexSync)

        XCTAssertEqual(mapResult.summary, "Webex map refresh skipped: Webex sync disabled in Settings.")
        XCTAssertEqual(result.summary, "Webex sync skipped: disabled in Settings.")
    }

    func testDisabledCodexFeatureSwitchesDoNotLaunchCodex() async throws {
        let runtimeRoot = FileManager.default.temporaryDirectory
            .appendingPathComponent("CubicleDisabledCodexFeaturesTests-\(UUID().uuidString)", isDirectory: true)
        defer { try? FileManager.default.removeItem(at: runtimeRoot) }
        try FileManager.default.createDirectory(at: runtimeRoot, withIntermediateDirectories: true)
        let fakeCodexURL = runtimeRoot.appendingPathComponent("fake-codex")
        let calledURL = runtimeRoot.appendingPathComponent("codex-called")
        let script = """
        #!/bin/sh
        touch "$CODEX_SHOULD_NOT_RUN_FILE"
        exit 73
        """
        try script.write(to: fakeCodexURL, atomically: true, encoding: .utf8)
        try FileManager.default.setAttributes(
            [.posixPermissions: NSNumber(value: Int16(0o755))],
            ofItemAtPath: fakeCodexURL.path
        )
        setenv("CODEX_SHOULD_NOT_RUN_FILE", calledURL.path, 1)
        defer { unsetenv("CODEX_SHOULD_NOT_RUN_FILE") }

        let configuration = RuntimeConfiguration(
            runtimeRoot: runtimeRoot,
            codexExecutable: fakeCodexURL.path,
            webexBaseURL: URL(string: "https://webexapis.com/v1")!,
            webexPageSize: 100,
            webexRetryCount: 0,
            webexTimeoutSeconds: 1,
            webexOAuthTokenPathOverride: nil,
            webexOAuthRefreshSkewSeconds: 300,
            webexOAuthRefreshTokenSkewSeconds: 86_400
        )
        var settings = SystemSettings()
        settings.codexPersonSummariesEnabled = false
        settings.codexSpaceSummariesEnabled = false
        settings.codexClusterTitlesEnabled = false
        settings.codexExecQuestionsEnabled = false
        settings.codexBeliefsEnabled = false
        try ConfigStore(configuration: configuration).saveSystemSettings(settings)

        let service = CodexPromptOrchestrationService(configuration: configuration)

        do {
            _ = try await service.generatePersonClusterSummary(
                context: PersonClusterSummaryContext(
                    personID: "person-disabled",
                    personLabel: "Disabled Person",
                    conversationHighlights: ["A recent highlight."],
                    existingSummary: nil
                ),
                workingDirectory: runtimeRoot
            )
            XCTFail("Person summaries should be blocked before launching Codex.")
        } catch {}

        do {
            _ = try await service.generateClusterTitle(
                context: ClusterTitleContext(
                    focusKind: .space,
                    clusterID: "cluster-disabled",
                    clusterSummary: "A cluster summary.",
                    supportingSignals: ["signal"],
                    existingTitle: nil
                ),
                workingDirectory: runtimeRoot
            )
            XCTFail("Cluster titles should be blocked before launching Codex.")
        } catch {}

        do {
            _ = try await service.generateSpaceSummary(
                context: SpaceSummaryContext(
                    roomID: "room-disabled",
                    roomTitle: "Disabled Room",
                    clusters: [
                        SpaceConversationCluster(title: "Topic", summary: "Summary", openLoops: ["open"])
                    ],
                    openLoops: ["open"],
                    previousSummary: nil,
                    previousGeneratedAt: nil
                ),
                workingDirectory: runtimeRoot
            )
            XCTFail("Space summaries should be blocked before launching Codex.")
        } catch {}

        let execQuestions = try await service.generateExecQuestionsForRoom(
            roomID: "room-disabled",
            roomTitle: "Disabled Room",
            summary: "Summary",
            openLoops: ["open"],
            clusters: [
                SpaceConversationCluster(title: "Topic", summary: "Summary", openLoops: ["open"])
            ],
            roomParticipants: [
                SpaceParticipant(name: "Executive", email: "exec@example.com")
            ],
            importantExecutives: [
                ImportantExecutive(name: "Executive", email: "exec@example.com")
            ],
            execBeliefsByEmail: [:],
            workingDirectory: runtimeRoot
        )
        XCTAssertTrue(execQuestions.isEmpty)

        let batch = try await service.runBeliefReconciliationBatch(
            targets: [
                BeliefReconciliationTarget(
                    context: BeliefReconciliationContext(
                        scope: .space,
                        entityKey: "space-disabled",
                        currentBeliefs: [],
                        manualBeliefs: [],
                        evidence: [
                            BeliefEvidence(
                                id: "evidence-disabled",
                                text: "Evidence that should not invoke Codex.",
                                source: "test",
                                occurredAt: "2026-05-15T13:00:00Z"
                            )
                        ],
                        forceRefresh: true,
                        incrementalWindowDays: 60,
                        chunkIndex: nil,
                        chunkCount: nil
                    ),
                    state: BeliefReconciliationState(lastRunAt: nil, lastEvidenceHash: nil)
                )
            ],
            workingDirectory: runtimeRoot
        )
        XCTAssertEqual(batch.triggeredCount, 0)
        XCTAssertEqual(batch.skippedCount, 1)
        XCTAssertFalse(FileManager.default.fileExists(atPath: calledURL.path))
    }

    func testDisabledQuestionSynthesisDoesNotLaunchCodex() async throws {
        let runtimeRoot = FileManager.default.temporaryDirectory
            .appendingPathComponent("CubicleDisabledQuestionSynthesisTests-\(UUID().uuidString)", isDirectory: true)
        defer { try? FileManager.default.removeItem(at: runtimeRoot) }
        try FileManager.default.createDirectory(at: runtimeRoot, withIntermediateDirectories: true)
        let fakeCodexURL = runtimeRoot.appendingPathComponent("fake-codex")
        let calledURL = runtimeRoot.appendingPathComponent("codex-called")
        let script = """
        #!/bin/sh
        touch "$CODEX_SHOULD_NOT_RUN_FILE"
        exit 73
        """
        try script.write(to: fakeCodexURL, atomically: true, encoding: .utf8)
        try FileManager.default.setAttributes(
            [.posixPermissions: NSNumber(value: Int16(0o755))],
            ofItemAtPath: fakeCodexURL.path
        )
        setenv("CODEX_SHOULD_NOT_RUN_FILE", calledURL.path, 1)
        defer { unsetenv("CODEX_SHOULD_NOT_RUN_FILE") }

        let configuration = RuntimeConfiguration(
            runtimeRoot: runtimeRoot,
            codexExecutable: fakeCodexURL.path,
            webexBaseURL: URL(string: "https://webexapis.com/v1")!,
            webexPageSize: 100,
            webexRetryCount: 0,
            webexTimeoutSeconds: 1,
            webexOAuthTokenPathOverride: nil,
            webexOAuthRefreshSkewSeconds: 300,
            webexOAuthRefreshTokenSkewSeconds: 86_400
        )
        var settings = SystemSettings()
        settings.codexQuestionSynthesisEnabled = false
        try ConfigStore(configuration: configuration).saveSystemSettings(settings)

        let candidate = QuestionCandidate(
            id: "seed-disabled",
            scopeType: .space,
            scopeKey: "space-disabled",
            scopeLabel: "Disabled Space",
            questionText: "Who owns the disabled synthesis follow-up?",
            questionType: "space_open_loop",
            whyNow: "This candidate should stay local when synthesis is disabled.",
            evidence: [
                QuestionEvidenceRef(
                    sourceType: "space",
                    sourceID: "evidence-disabled",
                    createdAt: Date(timeIntervalSince1970: 1_778_859_000),
                    label: "Recent message",
                    preview: "A follow-up needs an owner."
                )
            ],
            sourceKind: "webex_qg_core",
            sourceKey: "seed",
            tags: ["open-loop"],
            priorityScore: 80,
            status: .candidate,
            answerSnapshotId: nil,
            createdAt: Date(timeIntervalSince1970: 1_778_859_000),
            updatedAt: Date(timeIntervalSince1970: 1_778_859_000),
            expiresAt: nil
        )
        let service = CodexPromptOrchestrationService(configuration: configuration)

        let synthesized = try await service.synthesizeQuestionCandidates(
            from: [candidate],
            now: Date(timeIntervalSince1970: 1_778_859_000)
        )

        XCTAssertTrue(synthesized.isEmpty)
        XCTAssertFalse(FileManager.default.fileExists(atPath: calledURL.path))
    }

    func testAskCodexQueryHistoryPersistsTargetContextAndFeedsQuestionSynthesis() async throws {
        let runtimeRoot = FileManager.default.temporaryDirectory
            .appendingPathComponent("CubicleAskCodexHistoryTests-\(UUID().uuidString)", isDirectory: true)
        defer { try? FileManager.default.removeItem(at: runtimeRoot) }
        try FileManager.default.createDirectory(at: runtimeRoot, withIntermediateDirectories: true)
        let fakeCodexURL = runtimeRoot.appendingPathComponent("fake-codex")
        let stdinCaptureURL = runtimeRoot.appendingPathComponent("question-synthesis-prompt.txt")
        let script = """
        #!/bin/sh
        cat > "$CODEX_STDIN_CAPTURE"
        output_path=""
        while [ "$#" -gt 0 ]; do
          if [ "$1" = "--output-last-message" ]; then
            shift
            output_path="$1"
            break
          fi
          shift
        done
        printf '{"questions":[]}\\n' > "$output_path"
        """
        try script.write(to: fakeCodexURL, atomically: true, encoding: .utf8)
        try FileManager.default.setAttributes(
            [.posixPermissions: NSNumber(value: Int16(0o755))],
            ofItemAtPath: fakeCodexURL.path
        )
        setenv("CODEX_STDIN_CAPTURE", stdinCaptureURL.path, 1)
        defer { unsetenv("CODEX_STDIN_CAPTURE") }

        let configuration = RuntimeConfiguration(
            runtimeRoot: runtimeRoot,
            codexExecutable: fakeCodexURL.path,
            webexBaseURL: URL(string: "https://webexapis.com/v1")!,
            webexPageSize: 100,
            webexRetryCount: 0,
            webexTimeoutSeconds: 1,
            webexOAuthTokenPathOverride: nil,
            webexOAuthRefreshSkewSeconds: 300,
            webexOAuthRefreshTokenSkewSeconds: 86_400
        )
        let configStore = ConfigStore(configuration: configuration)
        let historyEntry = AskCodexQueryHistoryEntry(
            id: "history-1",
            question: "what are the key points",
            targetScope: .selectedSpace,
            targetTitle: "Space: Prabhat - Staff",
            targetKey: "space-prabhat-staff",
            targetItemID: "spacefocus:room-prabhat-staff",
            submittedAt: "2026-05-15T15:31:00Z"
        )
        try configStore.saveAskCodexQueryHistory([historyEntry])
        XCTAssertEqual(configStore.loadAskCodexQueryHistory(), [historyEntry])

        let candidate = QuestionCandidate(
            id: "seed-1",
            scopeType: .space,
            scopeKey: "room-prabhat-staff",
            scopeLabel: "Prabhat - Staff",
            questionText: "Which owner needs to follow up in Prabhat - Staff?",
            questionType: "space_open_loop",
            whyNow: "A latest message indicates a follow-up may be needed.",
            evidence: [
                QuestionEvidenceRef(
                    sourceType: "space",
                    sourceID: "evidence-1",
                    createdAt: Date(timeIntervalSince1970: 1_778_859_000),
                    label: "Recent message",
                    preview: "The space has an unresolved follow-up."
                )
            ],
            sourceKind: "webex_qg_core",
            sourceKey: "seed",
            tags: ["open-loop"],
            priorityScore: 80,
            status: .candidate,
            answerSnapshotId: nil,
            createdAt: Date(timeIntervalSince1970: 1_778_859_000),
            updatedAt: Date(timeIntervalSince1970: 1_778_859_000),
            expiresAt: nil
        )
        let service = CodexPromptOrchestrationService(configuration: configuration)

        _ = try await service.synthesizeQuestionCandidates(
            from: [candidate],
            now: Date(timeIntervalSince1970: 1_778_859_000)
        )

        let prompt = try String(contentsOf: stdinCaptureURL, encoding: .utf8)
        XCTAssertTrue(prompt.contains("Operator Ask Codex History:"))
        XCTAssertTrue(prompt.contains("Space: Prabhat - Staff"))
        XCTAssertTrue(prompt.contains("what are the key points"))
        XCTAssertTrue(prompt.contains("Do not treat operator Ask Codex history as evidence"))
    }

    func testCodexRunnerSendsPromptOverStdinNotProcessArguments() async throws {
        let runtimeRoot = FileManager.default.temporaryDirectory
            .appendingPathComponent("CubicleCodexRunnerStdinTests-\(UUID().uuidString)", isDirectory: true)
        defer { try? FileManager.default.removeItem(at: runtimeRoot) }
        try FileManager.default.createDirectory(at: runtimeRoot, withIntermediateDirectories: true)
        let fakeCodexURL = runtimeRoot.appendingPathComponent("fake-codex")
        let argvCaptureURL = runtimeRoot.appendingPathComponent("argv.txt")
        let stdinCaptureURL = runtimeRoot.appendingPathComponent("stdin.txt")
        let script = """
        #!/bin/sh
        printf '%s\\n' "$@" > "$CODEX_ARGV_CAPTURE"
        cat > "$CODEX_STDIN_CAPTURE"
        output_path=""
        while [ "$#" -gt 0 ]; do
          if [ "$1" = "--output-last-message" ]; then
            shift
            output_path="$1"
            break
          fi
          shift
        done
        printf 'fake codex output\\n' > "$output_path"
        """
        try script.write(to: fakeCodexURL, atomically: true, encoding: .utf8)
        try FileManager.default.setAttributes(
            [.posixPermissions: NSNumber(value: Int16(0o755))],
            ofItemAtPath: fakeCodexURL.path
        )
        setenv("CODEX_ARGV_CAPTURE", argvCaptureURL.path, 1)
        setenv("CODEX_STDIN_CAPTURE", stdinCaptureURL.path, 1)
        defer {
            unsetenv("CODEX_ARGV_CAPTURE")
            unsetenv("CODEX_STDIN_CAPTURE")
        }
        let configuration = RuntimeConfiguration(
            runtimeRoot: runtimeRoot,
            codexExecutable: fakeCodexURL.path,
            webexBaseURL: URL(string: "https://webexapis.com/v1")!,
            webexPageSize: 100,
            webexRetryCount: 0,
            webexTimeoutSeconds: 1,
            webexOAuthTokenPathOverride: nil,
            webexOAuthRefreshSkewSeconds: 300,
            webexOAuthRefreshTokenSkewSeconds: 86_400
        )
        let configStore = ConfigStore(configuration: configuration)
        var settings = SystemSettings()
        settings.codexModel = .gpt54Mini
        settings.codexReasoningLevel = .high
        try configStore.saveSystemSettings(settings)
        let runner = CodexRunner(configuration: configuration)
        let jobDirectory = runtimeRoot.appendingPathComponent("job", isDirectory: true)
        let secretPrompt = "SECRET-ARG-LEAK-CHECK: use private Webex context only."
        let job = CodexPromptJob(
            id: "stdin-test",
            title: "Stdin Test",
            promptVersion: "test",
            promptURL: jobDirectory.appendingPathComponent("prompt.md"),
            outputURL: jobDirectory.appendingPathComponent("output.txt"),
            logURL: jobDirectory.appendingPathComponent("run.log"),
            metadataURL: jobDirectory.appendingPathComponent("metadata.json"),
            status: .pending,
            createdAt: Date()
        )

        let result = try await runner.run(
            request: CodexRunRequest(
                prompt: secretPrompt,
                job: job,
                workingDirectory: runtimeRoot,
                policy: CodexRunPolicy(timeoutSeconds: 5, maxAttempts: 1, retryDelaySeconds: 0)
            )
        )

        let argv = try String(contentsOf: argvCaptureURL, encoding: .utf8)
        let stdin = try String(contentsOf: stdinCaptureURL, encoding: .utf8)
        XCTAssertEqual(result.output, "fake codex output\n")
        XCTAssertFalse(argv.contains(secretPrompt))
        XCTAssertTrue(argv.split(separator: "\n").contains("--ignore-user-config"))
        XCTAssertTrue(argv.split(separator: "\n").contains("-"))
        XCTAssertTrue(argv.split(separator: "\n").contains("--model"))
        XCTAssertTrue(argv.split(separator: "\n").contains("gpt-5.4-mini"))
        XCTAssertTrue(argv.split(separator: "\n").contains(#"model_reasoning_effort="high""#))
        XCTAssertEqual(stdin, secretPrompt)
    }

    func testCodexRunnerRejectsComputerUseExecutablePath() async throws {
        let runtimeRoot = FileManager.default.temporaryDirectory
            .appendingPathComponent("CubicleCodexRunnerUnsafePathTests-\(UUID().uuidString)", isDirectory: true)
        defer { try? FileManager.default.removeItem(at: runtimeRoot) }
        try FileManager.default.createDirectory(at: runtimeRoot, withIntermediateDirectories: true)
        let unsafeExecutable = runtimeRoot
            .appendingPathComponent("Codex Computer Use.app/Contents/SharedSupport/SkyComputerUseClient.app/Contents/MacOS/SkyComputerUseClient")
            .path
        let configuration = RuntimeConfiguration(
            runtimeRoot: runtimeRoot,
            codexExecutable: unsafeExecutable,
            webexBaseURL: URL(string: "https://webexapis.com/v1")!,
            webexPageSize: 100,
            webexRetryCount: 0,
            webexTimeoutSeconds: 1,
            webexOAuthTokenPathOverride: nil,
            webexOAuthRefreshSkewSeconds: 300,
            webexOAuthRefreshTokenSkewSeconds: 86_400
        )
        let runner = CodexRunner(configuration: configuration)
        let jobDirectory = runtimeRoot.appendingPathComponent("job", isDirectory: true)
        let job = CodexPromptJob(
            id: "unsafe-path-test",
            title: "Unsafe Path Test",
            promptVersion: "test",
            promptURL: jobDirectory.appendingPathComponent("prompt.md"),
            outputURL: jobDirectory.appendingPathComponent("output.txt"),
            logURL: jobDirectory.appendingPathComponent("run.log"),
            metadataURL: jobDirectory.appendingPathComponent("metadata.json"),
            status: .pending,
            createdAt: Date()
        )

        do {
            _ = try await runner.run(
                request: CodexRunRequest(
                    prompt: "SECRET-UNSAFE-PATH-CHECK",
                    job: job,
                    workingDirectory: runtimeRoot,
                    policy: CodexRunPolicy(timeoutSeconds: 5, maxAttempts: 1, retryDelaySeconds: 0)
                )
            )
            XCTFail("Expected unsafe Computer Use Codex executable path to be rejected.")
        } catch let error as CodexRunnerError {
            guard case .unsafeCodexExecutable(let rejectedPath) = error else {
                return XCTFail("Expected unsafeCodexExecutable, got \(error)")
            }
            XCTAssertEqual(rejectedPath, unsafeExecutable)
        }
    }

    func testBeliefUpsertHandlesPrimaryKeyCollisionWithoutRuntimeError() throws {
        let runtimeRoot = FileManager.default.temporaryDirectory
            .appendingPathComponent("CubicleBeliefUpsertCollisionTests-\(UUID().uuidString)", isDirectory: true)
        defer { try? FileManager.default.removeItem(at: runtimeRoot) }
        let configuration = RuntimeConfiguration(
            runtimeRoot: runtimeRoot,
            codexExecutable: "codex",
            webexBaseURL: URL(string: "https://webexapis.com/v1")!,
            webexPageSize: 100,
            webexRetryCount: 0,
            webexTimeoutSeconds: 1,
            webexOAuthTokenPathOverride: nil,
            webexOAuthRefreshSkewSeconds: 300,
            webexOAuthRefreshTokenSkewSeconds: 86_400
        )
        let store = KnowledgeStore(configuration: configuration)
        let first = BeliefRecord(
            id: "auto-space-room-1-colliding",
            scope: KnowledgeBeliefScope.space.rawValue,
            entityKey: "room-1",
            statement: "Durable belief one.",
            confidence: 0.6,
            updatedAt: "2026-05-14T20:00:00Z",
            isManual: false,
            evidenceLinks: ["evidence-1"],
            createdAt: "2026-05-14T20:00:00Z",
            beliefKind: "second_order",
            lifecycle: "candidate",
            supportCount: 1,
            contradictionCount: 0,
            lastEvidenceAt: "2026-05-14T20:00:00Z"
        )
        let second = BeliefRecord(
            id: first.id,
            scope: KnowledgeBeliefScope.space.rawValue,
            entityKey: "room-1",
            statement: "Durable belief two.",
            confidence: 0.9,
            updatedAt: "2026-05-14T21:00:00Z",
            isManual: false,
            evidenceLinks: ["evidence-2"],
            createdAt: "2026-05-14T21:00:00Z",
            beliefKind: "second_order",
            lifecycle: "active",
            supportCount: 3,
            contradictionCount: 0,
            lastEvidenceAt: "2026-05-14T21:00:00Z"
        )
        let sameStatementNewID = BeliefRecord(
            id: "auto-space-room-1-new-id",
            scope: KnowledgeBeliefScope.space.rawValue,
            entityKey: "room-1",
            statement: second.statement,
            confidence: 1.0,
            updatedAt: "2026-05-14T22:00:00Z",
            isManual: false,
            evidenceLinks: ["evidence-3"],
            createdAt: "2026-05-14T22:00:00Z",
            beliefKind: "second_order",
            lifecycle: "stable",
            supportCount: 4,
            contradictionCount: 0,
            lastEvidenceAt: "2026-05-14T22:00:00Z"
        )

        try store.upsertBelief(first)
        try store.upsertBelief(second)
        try store.upsertBelief(sameStatementNewID)

        let beliefs = try store.loadBeliefs(scope: .space, entityKey: "room-1")
        XCTAssertEqual(beliefs.count, 1)
        XCTAssertEqual(beliefs.first?.id, first.id)
        XCTAssertEqual(beliefs.first?.statement, second.statement)
        XCTAssertEqual(beliefs.first?.confidence, 1.0)
        XCTAssertEqual(beliefs.first?.lifecycle, "stable")
        XCTAssertEqual(beliefs.first?.supportCount, 4)
    }

    func testKnowledgeStoreConnectorBatchRollsBackOnEvidenceFailure() throws {
        let runtimeRoot = FileManager.default.temporaryDirectory
            .appendingPathComponent("CubicleConnectorBatchRollback-\(UUID().uuidString)", isDirectory: true)
        defer { try? FileManager.default.removeItem(at: runtimeRoot) }
        let store = KnowledgeStore(configuration: testRuntimeConfiguration(runtimeRoot: runtimeRoot))
        let now = "2026-06-01T00:00:00.000Z"

        XCTAssertThrowsError(
            try store.writeConnectorMessageBatch(
                rooms: [RoomRecord(id: "room-rollback", title: "Rollback Room", updatedAt: now)],
                people: [
                    PersonRecord(
                        id: "person-rollback",
                        displayName: "Rollback Person",
                        email: "rollback@example.com",
                        updatedAt: now
                    )
                ],
                messages: [
                    MessageRecord(
                        id: "message-rollback",
                        roomID: "room-rollback",
                        personID: "person-rollback",
                        body: "Should roll back.",
                        createdAt: now,
                        updatedAt: now
                    )
                ],
                evidence: [
                    BeliefEvidenceRecord(
                        id: "duplicate-evidence-id",
                        source: "webex_message",
                        sourceID: "message-rollback",
                        roomID: "room-rollback",
                        personID: "person-rollback",
                        occurredAt: now,
                        text: "First evidence row should be rolled back."
                    ),
                    BeliefEvidenceRecord(
                        id: "duplicate-evidence-id",
                        source: "webex_message",
                        sourceID: "message-rollback-conflict",
                        roomID: "room-rollback",
                        personID: "person-rollback",
                        occurredAt: now,
                        text: "Primary key conflict should roll back prior writes."
                    )
                ]
            )
        )

        XCTAssertNil(try store.loadRoom(roomID: "room-rollback"))
        XCTAssertFalse(try store.messageExists(messageID: "message-rollback"))
        XCTAssertTrue(try store.loadBeliefEvidence(scope: .space, entityKey: "room-rollback").isEmpty)
    }

    func testTopicUpsertHandlesMigratedLegacyRequiredColumns() throws {
        let runtimeRoot = FileManager.default.temporaryDirectory
            .appendingPathComponent("CubicleTopicLegacyUpsertTests-\(UUID().uuidString)", isDirectory: true)
        defer { try? FileManager.default.removeItem(at: runtimeRoot) }
        let knowledgeDirectory = runtimeRoot.appendingPathComponent("knowledge", isDirectory: true)
        try FileManager.default.createDirectory(at: knowledgeDirectory, withIntermediateDirectories: true)
        let databaseURL = knowledgeDirectory.appendingPathComponent("knowledge.db")
        try executeSQLite(
            """
            CREATE TABLE topics (
              topic_id TEXT PRIMARY KEY,
              label TEXT NOT NULL,
              description TEXT,
              status TEXT,
              confidence REAL,
              first_seen_at TEXT NOT NULL,
              last_seen_at TEXT NOT NULL
            );
            """,
            databaseURL: databaseURL
        )
        let configuration = RuntimeConfiguration(
            runtimeRoot: runtimeRoot,
            codexExecutable: "codex",
            webexBaseURL: URL(string: "https://webexapis.com/v1")!,
            webexPageSize: 100,
            webexRetryCount: 0,
            webexTimeoutSeconds: 1,
            webexOAuthTokenPathOverride: nil,
            webexOAuthRefreshSkewSeconds: 300,
            webexOAuthRefreshTokenSkewSeconds: 86_400
        )
        let store = KnowledgeStore(configuration: configuration)

        try store.upsertTopic(
            TopicRecord(
                id: "topic-1",
                focusKind: "space",
                scope: "space",
                entityKey: "room-1",
                topicKey: "agentic-security",
                title: "Agentic Security Strategy",
                summary: "Dynamic controls and Cisco Live positioning are active.",
                soWhat: "Leadership needs a credible demo and message.",
                sourceLabel: "Codex",
                score: 0.95,
                generatedAt: "2026-05-14T20:00:00Z",
                updatedAt: "2026-05-14T21:00:00Z"
            )
        )

        let topics = try store.loadTopics(focusKind: "space", scope: "space", entityKey: "room-1")
        XCTAssertEqual(topics.count, 1)
        XCTAssertEqual(topics.first?.title, "Agentic Security Strategy")
        XCTAssertEqual(
            try querySQLiteText("SELECT label FROM topics WHERE topic_id = 'topic-1';", databaseURL: databaseURL),
            "Agentic Security Strategy"
        )
        XCTAssertEqual(
            try querySQLiteText("SELECT first_seen_at FROM topics WHERE topic_id = 'topic-1';", databaseURL: databaseURL),
            "2026-05-14T20:00:00Z"
        )
        XCTAssertEqual(
            try querySQLiteText("SELECT last_seen_at FROM topics WHERE topic_id = 'topic-1';", databaseURL: databaseURL),
            "2026-05-14T21:00:00Z"
        )
    }

    @MainActor
    func testAskCodexAllTrackedContextIncludesFocusEvidenceAndQuestions() async throws {
        let runtimeRoot = FileManager.default.temporaryDirectory
            .appendingPathComponent("CubicleAskCodexContextTests-\(UUID().uuidString)", isDirectory: true)
        defer { try? FileManager.default.removeItem(at: runtimeRoot) }
        let configuration = RuntimeConfiguration(
            runtimeRoot: runtimeRoot,
            codexExecutable: "codex",
            webexBaseURL: URL(string: "https://webexapis.com/v1")!,
            webexPageSize: 100,
            webexRetryCount: 0,
            webexTimeoutSeconds: 1,
            webexOAuthTokenPathOverride: nil,
            webexOAuthRefreshSkewSeconds: 300,
            webexOAuthRefreshTokenSkewSeconds: 86_400
        )
        let runtimeStore = NativeRuntimeStore(configuration: configuration)
        let store = KnowledgeStore(configuration: configuration)
        try store.bootstrap()
        try store.upsertQuestionCandidates([
            QuestionCandidate(
                id: "question-1",
                scopeType: .space,
                scopeKey: "agentic-security-room",
                scopeLabel: "Agentic Security",
                questionText: "Who owns the adaptive policy demo before the Friday review?",
                questionType: "codex_synthesized_question",
                whyNow: "The latest Agentic Security activity mentions a demo-script owner gap.",
                evidence: [
                    QuestionEvidenceRef(
                        sourceType: "space",
                        sourceID: "agentic-security-room",
                        createdAt: Date(timeIntervalSince1970: 1_768_900_000),
                        label: "Recent message",
                        preview: "Arjun asked who owns the adaptive policy demo-script updates."
                    )
                ],
                sourceKind: "codex_question_synthesis",
                sourceKey: "ask-context-test",
                tags: ["codex synthesized question"],
                priorityScore: 95,
                status: .candidate,
                answerSnapshotId: nil,
                createdAt: Date(timeIntervalSince1970: 1_768_900_000),
                updatedAt: Date(timeIntervalSince1970: 1_768_900_100),
                expiresAt: nil
            )
        ])

        let spaceCache = FocusCache(
            focusDays: 60,
            items: [
                FocusItem(
                    id: "spacefocus:agentic-security-room",
                    title: "Agentic Security",
                    subtitle: "Adaptive policy demo owner gap",
                    meta: "auto-reply=no | messages=33",
                    timestamp: "2026-05-15 06:40 PDT",
                    badge: "space",
                    statusBadge: "live-webex",
                    detailLines: [],
                    detailIntroLines: [
                        "Space summary: Agentic Security is tracking the adaptive policy demo, MCP Cloud Control app, and Cisco Live readiness.",
                        "Current posture / next move: Name the demo-script owner and validate Secure Client enforcement dependencies."
                    ],
                    detailSections: [
                        FocusDetailSection(
                            id: "recent",
                            header: "Recent conversations (last 3 days):",
                            lines: [
                                "May 15, 6:35 AM | Arjun - Who owns the adaptive policy demo-script and UX-flow changes?",
                                "May 15, 6:38 AM | DJ - Backend dependencies need Identity Intelligence and Secure Client API validation."
                            ],
                            roomTitle: "Agentic Security",
                            summarySource: "Codex",
                            summaryGeneratedAt: "2026-05-15T13:40:00Z"
                        ),
                        FocusDetailSection(
                            id: "exec-jeetu",
                            header: "What are the Questions running in the Exec's (Jeetu Patel) Mind:",
                            lines: ["Is the Agentic Security demo credible enough for Cisco Live?"],
                            roomTitle: "Agentic Security",
                            summarySource: "Codex",
                            summaryGeneratedAt: "2026-05-15T13:40:00Z"
                        ),
                        FocusDetailSection(
                            id: "topics",
                            header: "Meaningful topics from Codex (top 5):",
                            lines: ["Adaptive policy demo ownership: Without a named owner, the review path is blocked."],
                            roomTitle: "Agentic Security",
                            summarySource: "Codex",
                            summaryGeneratedAt: "2026-05-15T13:40:00Z"
                        )
                    ],
                    detailTailLines: []
                )
            ],
            updatedAt: "2026-05-15T13:40:00Z",
            countLabel: "1",
            recentMessages: 33,
            summaryGenerationInProgress: false,
            subjectsProcessed: 1,
            subjectsTotal: 1
        )
        let personCache = FocusCache(
            focusDays: 30,
            items: [
                FocusItem(
                    id: "personfocus:peter-bailey",
                    title: "Peter Bailey",
                    subtitle: "AI Gateway direction",
                    meta: "messages=12",
                    timestamp: "2026-05-15 06:00 PDT",
                    badge: "person",
                    statusBadge: "live-webex",
                    detailLines: [],
                    detailIntroLines: ["Person summary: Peter is pushing for crisp AI Gateway ownership and customer-facing readiness."],
                    detailSections: [
                        FocusDetailSection(
                            id: "person-recent",
                            header: "Recent conversations (last 30 days):",
                            lines: ["May 15, 6:00 AM | Peter - The gateway plan needs one owner and one date."],
                            roomTitle: "Peter Bailey",
                            summarySource: "Codex",
                            summaryGeneratedAt: "2026-05-15T13:40:00Z"
                        ),
                        FocusDetailSection(
                            id: "person-topic",
                            header: "Conversation extraction source: Codex",
                            lines: ["AI Gateway ownership remains the recurring leadership theme."],
                            roomTitle: "Peter Bailey",
                            summarySource: "Codex",
                            summaryGeneratedAt: "2026-05-15T13:40:00Z"
                        ),
                        FocusDetailSection(
                            id: "person-status",
                            header: "Current status:",
                            lines: ["Needs owner/date clarity."],
                            roomTitle: "Peter Bailey",
                            summarySource: "Codex",
                            summaryGeneratedAt: "2026-05-15T13:40:00Z"
                        )
                    ],
                    detailTailLines: []
                )
            ],
            updatedAt: "2026-05-15T13:40:00Z",
            countLabel: "1",
            recentMessages: 12,
            summaryGenerationInProgress: false,
            subjectsProcessed: 1,
            subjectsTotal: 1
        )

        try writeFocusCache(spaceCache, to: runtimeStore.snapshotURL(kind: .space))
        try writeFocusCache(personCache, to: runtimeStore.snapshotURL(kind: .person))

        let model = AppModel(runtimeStore: runtimeStore)
        let loaded = await model.loadAll()
        XCTAssertTrue(loaded)

        let context = model.askCodexContextPreviewLines().joined(separator: "\n")
        XCTAssertTrue(context.contains("Space evidence:"))
        XCTAssertTrue(context.contains("Space summary: Agentic Security is tracking the adaptive policy demo"))
        XCTAssertTrue(context.contains("May 15, 6:35 AM | Arjun"))
        XCTAssertTrue(context.contains("Person evidence:"))
        XCTAssertTrue(context.contains("Person summary: Peter is pushing for crisp AI Gateway ownership"))
        XCTAssertTrue(context.contains("Open questions from Questions engine:"))
        XCTAssertTrue(context.contains("Who owns the adaptive policy demo before the Friday review?"))
    }

    func testSpaceFocusCacheDropsStaleCodexSummaryWhenNewerEvidenceExists() throws {
        let runtimeRoot = FileManager.default.temporaryDirectory
            .appendingPathComponent("CubicleFocusFreshnessTests-\(UUID().uuidString)", isDirectory: true)
        defer { try? FileManager.default.removeItem(at: runtimeRoot) }
        let knowledgeDirectory = runtimeRoot.appendingPathComponent("knowledge", isDirectory: true)
        let nativeDirectory = knowledgeDirectory.appendingPathComponent("native", isDirectory: true)
        try FileManager.default.createDirectory(at: nativeDirectory, withIntermediateDirectories: true)
        let configuration = RuntimeConfiguration(
            runtimeRoot: runtimeRoot,
            codexExecutable: "codex",
            webexBaseURL: URL(string: "https://webexapis.com/v1")!,
            webexPageSize: 100,
            webexRetryCount: 0,
            webexTimeoutSeconds: 1,
            webexOAuthTokenPathOverride: nil,
            webexOAuthRefreshSkewSeconds: 300,
            webexOAuthRefreshTokenSkewSeconds: 86_400
        )

        let roomID = "room-prabhat-staff"
        let freshCache = FocusCache(
            focusDays: 60,
            items: [
                FocusItem(
                    id: roomID,
                    title: "Prabhat - Staff",
                    subtitle: "Fresh Webex evidence",
                    meta: "auto-reply=no | messages=204",
                    timestamp: "2026-05-14T23:15:39+00:00",
                    badge: "space",
                    statusBadge: "live-webex",
                    detailLines: [
                        "Space Name: Prabhat - Staff",
                        "Recent conversations (last 60 days):",
                        "Webex message-new: 2026-05-14T23:15:39+00:00 | Purna Chandra Chevukula | Prabhat - Staff | New guidance landed after the cached summary."
                    ],
                    detailIntroLines: [],
                    detailSections: [],
                    detailTailLines: []
                )
            ],
            updatedAt: "2026-05-14T23:34:32Z",
            countLabel: "1",
            recentMessages: 204,
            summaryGenerationInProgress: false,
            subjectsProcessed: 1,
            subjectsTotal: 1
        )
        let staleNativeCache = FocusCache(
            focusDays: 60,
            items: [
                FocusItem(
                    id: "spacefocus:\(roomID)",
                    title: "Prabhat - Staff",
                    subtitle: "Stale Codex summary",
                    meta: "auto-reply=no | messages=203",
                    timestamp: "2026-05-14 16:15 PDT",
                    badge: "space",
                    statusBadge: "live-webex",
                    detailLines: [
                        "Summary generated: 2026-05-11 23:57 PDT",
                        "Guidance freshness: cached summary already includes Prabhat Singh's latest reply; treat the guidance as next-step posture.",
                        "Space summary: Old generated summary.",
                        "Space summary source: Codex cache",
                        "Current posture / next move: Prabhat Singh already replied on 2026-05-06 11:30 PDT.",
                        "Latest room message: 2026-05-14 16:15 PDT"
                    ],
                    detailIntroLines: [
                        "Summary generated: 2026-05-11 23:57 PDT",
                        "Space summary: Old generated summary."
                    ],
                    detailSections: [],
                    detailTailLines: []
                )
            ],
            updatedAt: "2026-05-11T23:57:00Z",
            countLabel: "1",
            recentMessages: 203,
            summaryGenerationInProgress: false,
            subjectsProcessed: 1,
            subjectsTotal: 1
        )
        let encoder = JSONEncoder()
        try encoder.encode(freshCache).write(
            to: knowledgeDirectory.appendingPathComponent("space_focus_cache_60d.json"),
            options: [.atomic]
        )
        try encoder.encode(staleNativeCache).write(
            to: nativeDirectory.appendingPathComponent("space_focus_cache_60d.native.json"),
            options: [.atomic]
        )

        let loaded = try NativeRuntimeStore(configuration: configuration).loadFocusCache(kind: .space)
        let item = try XCTUnwrap(loaded.items.first)
        let detail = item.detailLines.joined(separator: "\n")

        XCTAssertFalse(detail.contains("Summary generated: 2026-05-11 23:57 PDT"))
        XCTAssertFalse(detail.contains("already replied on 2026-05-06"))
        XCTAssertFalse(detail.contains("Codex cache"))
        XCTAssertTrue(detail.contains("2026-05-14"))
        XCTAssertEqual(item.meta, "auto-reply=no | messages=204")
    }

    func testRepeatedGeneratedSectionsReceiveDistinctIDs() throws {
        let item = FocusItem(
            id: "spacefocus:test-room",
            title: "Test Room",
            subtitle: "",
            meta: "",
            timestamp: "2026-05-14 16:15 PDT",
            badge: "space",
            statusBadge: "live-webex",
            detailLines: [
                "Conversation from Codex:",
                "Status: Open",
                "Summary: First Codex section.",
                "",
                "Conversation from Codex:",
                "Status: Closed",
                "Summary: Second Codex section."
            ],
            detailIntroLines: [],
            detailSections: [],
            detailTailLines: []
        )

        let assembled = item.assembledDetailPayload(kind: .space, focusDays: 60, clusterSeeds: [])

        XCTAssertEqual(assembled.detailSections.count, 2)
        XCTAssertEqual(Set(assembled.detailSections.map(\.id)).count, 2)
    }

    func testConfiguredLiveSnapshotWinsOverOlderMismatchedDaySnapshot() throws {
        let runtimeRoot = FileManager.default.temporaryDirectory
            .appendingPathComponent("CubicleFocusSnapshotPriorityTests-\(UUID().uuidString)", isDirectory: true)
        defer { try? FileManager.default.removeItem(at: runtimeRoot) }
        let knowledgeDirectory = runtimeRoot.appendingPathComponent("knowledge", isDirectory: true)
        let nativeDirectory = knowledgeDirectory.appendingPathComponent("native", isDirectory: true)
        try FileManager.default.createDirectory(at: nativeDirectory, withIntermediateDirectories: true)
        let configuration = RuntimeConfiguration(
            runtimeRoot: runtimeRoot,
            codexExecutable: "codex",
            webexBaseURL: URL(string: "https://webexapis.com/v1")!,
            webexPageSize: 100,
            webexRetryCount: 0,
            webexTimeoutSeconds: 1,
            webexOAuthTokenPathOverride: nil,
            webexOAuthRefreshSkewSeconds: 300,
            webexOAuthRefreshTokenSkewSeconds: 86_400
        )
        var settings = SystemSettings()
        settings.spaceFocusDays = 10
        try ConfigStore(configuration: configuration).saveSystemSettings(settings)

        let oldSixtyDayCache = FocusCache(
            focusDays: 60,
            items: [
                FocusItem(
                    id: "spacefocus:live-room",
                    title: "Live 10 Day",
                    subtitle: "",
                    meta: "messages=203",
                    timestamp: "2026-05-10 10:00 PDT",
                    badge: "space",
                    statusBadge: "stale",
                    detailLines: ["Recent conversations (last 60 days):", "Space message 1: old"],
                    detailIntroLines: [],
                    detailSections: [
                        FocusDetailSection(
                            id: "old-recent",
                            header: "Recent conversations (last 60 days):",
                            lines: ["Space message 1: old"],
                            roomTitle: "Live 10 Day",
                            summarySource: "Local heuristic clustering",
                            summaryGeneratedAt: "2026-05-10T17:00:00Z"
                        ),
                        FocusDetailSection(
                            id: "old-local",
                            header: "Conversation extraction source: Local heuristic clustering",
                            lines: ["Summary: stale local extraction"],
                            roomTitle: "Live 10 Day",
                            summarySource: "Local heuristic clustering",
                            summaryGeneratedAt: "2026-05-10T17:00:00Z"
                        )
                    ],
                    detailTailLines: []
                )
            ],
            updatedAt: "2026-05-10T17:00:00Z",
            countLabel: "1",
            recentMessages: 203,
            summaryGenerationInProgress: false,
            subjectsProcessed: 1,
            subjectsTotal: 1
        )
        let encoder = JSONEncoder()
        try encoder.encode(oldSixtyDayCache).write(
            to: knowledgeDirectory.appendingPathComponent("space_focus_cache_60d.json"),
            options: [.atomic]
        )
        let oldNativeURL = nativeDirectory.appendingPathComponent("space_focus_cache_60d.native.json")
        try encoder.encode(oldSixtyDayCache).write(to: oldNativeURL, options: [.atomic])
        let oldManifestPayload = """
        {
          "kind": "space",
          "focusDays": 60,
          "sourceSnapshotPath": "\(knowledgeDirectory.appendingPathComponent("space_focus_cache_60d.json").path)",
          "outputSnapshotPath": "\(oldNativeURL.path)",
          "sourceSignature": "stale-60-day",
          "normalizedEventCount": 1,
          "clusterCount": 1,
          "cacheReuseVersion": 8,
          "sourceFileSizeBytes": 1,
          "sourceModifiedAtEpoch": 1
        }
        """
        try Data(oldManifestPayload.utf8).write(
            to: nativeDirectory.appendingPathComponent("space_focus_cache_manifest.json"),
            options: [.atomic]
        )
        let liveTenDayPayload = """
        {
          "items": [
            {
              "id": "live-room",
              "title": "Live 10 Day",
              "subtitle": "",
              "meta": "messages=9",
              "timestamp": "2026-05-14T23:15:39+00:00",
              "badge": "space",
              "status_badge": "live-webex",
              "detail_lines": ["Recent conversations (last 10 days):", "Webex 1: live"],
              "detail_intro_lines": [],
              "detail_sections": [],
              "detail_tail_lines": []
            }
          ],
          "updated_at": "2026-05-15T04:06:45Z",
          "spaces": 1,
          "recent_messages": 9,
          "summary_generation_in_progress": false,
          "subjects_processed": 1,
          "subjects_total": 1
        }
        """
        try Data(liveTenDayPayload.utf8).write(
            to: nativeDirectory.appendingPathComponent("live_space_focus_cache_10d.json"),
            options: [.atomic]
        )

        let runtimeStore = NativeRuntimeStore(configuration: configuration)
        let loaded = try runtimeStore.loadFocusCache(kind: .space)

        XCTAssertEqual(loaded.focusDays, 10)
        XCTAssertEqual(loaded.items.first?.title, "Live 10 Day")
        XCTAssertEqual(loaded.items.first?.statusBadge, "live-webex")

        let outcome = try runtimeStore.refreshSpaceFocusCache(forceRebuild: true)
        XCTAssertEqual(outcome.focusDays, 10)
        XCTAssertEqual(outcome.outputSnapshotURL.lastPathComponent, "space_focus_cache_10d.native.json")

        let refreshedData = try Data(contentsOf: outcome.outputSnapshotURL)
        let refreshed = try JSONDecoder().decode(FocusCache.self, from: refreshedData)
        let refreshedItem = try XCTUnwrap(refreshed.items.first)
        let refreshedHeaders = refreshedItem.detailSections.map(\.header)
        XCTAssertEqual(refreshed.focusDays, 10)
        XCTAssertTrue(refreshedHeaders.contains("Recent conversations (last 10 days):"))
        XCTAssertFalse(refreshedHeaders.contains { $0.localizedCaseInsensitiveContains("local heuristic") })
        XCTAssertFalse(refreshedHeaders.contains { $0.contains("last 60 days") })
    }

    func testSparseLiveSnapshotDoesNotReplaceRicherNativeCache() throws {
        let runtimeRoot = FileManager.default.temporaryDirectory
            .appendingPathComponent("CubicleFocusSparseLiveTests-\(UUID().uuidString)", isDirectory: true)
        defer { try? FileManager.default.removeItem(at: runtimeRoot) }
        let knowledgeDirectory = runtimeRoot.appendingPathComponent("knowledge", isDirectory: true)
        let nativeDirectory = knowledgeDirectory.appendingPathComponent("native", isDirectory: true)
        try FileManager.default.createDirectory(at: nativeDirectory, withIntermediateDirectories: true)
        let configuration = RuntimeConfiguration(
            runtimeRoot: runtimeRoot,
            codexExecutable: "codex",
            webexBaseURL: URL(string: "https://webexapis.com/v1")!,
            webexPageSize: 100,
            webexRetryCount: 0,
            webexTimeoutSeconds: 1,
            webexOAuthTokenPathOverride: nil,
            webexOAuthRefreshSkewSeconds: 300,
            webexOAuthRefreshTokenSkewSeconds: 86_400
        )
        var settings = SystemSettings()
        settings.spaceFocusDays = 10
        try ConfigStore(configuration: configuration).saveSystemSettings(settings)

        let richNativeCache = FocusCache(
            focusDays: 10,
            items: [
                FocusItem(
                    id: "spacefocus:room-rich",
                    title: "Rich Room",
                    subtitle: "Codex summary is available.",
                    meta: "auto-reply=no | messages=42",
                    timestamp: "2026-05-14 16:15 PDT",
                    badge: "space",
                    statusBadge: "live-webex",
                    detailLines: [
                        "Space summary: Existing Codex summary.",
                        "Space summary source: Codex cache",
                        "Recent conversations (last 10 days):",
                        "Webex 1: 2026-05-14 16:15 PDT | A | Rich Room | Existing evidence."
                    ],
                    detailIntroLines: [
                        "Space summary: Existing Codex summary.",
                        "Space summary source: Codex cache"
                    ],
                    detailSections: [
                        FocusDetailSection(
                            id: "recent",
                            header: "Recent conversations (last 10 days):",
                            lines: ["Webex 1: 2026-05-14 16:15 PDT | A | Rich Room | Existing evidence."],
                            roomTitle: "Rich Room",
                            summarySource: "Codex cache",
                            summaryGeneratedAt: "2026-05-14T23:15:00Z"
                        )
                    ],
                    detailTailLines: []
                )
            ],
            updatedAt: "2026-05-15T04:15:00Z",
            countLabel: "1",
            recentMessages: 42,
            summaryGenerationInProgress: false,
            subjectsProcessed: 1,
            subjectsTotal: 1
        )
        let sparseLivePayload = """
        {
          "items": [
            {
              "id": "room-rich",
              "title": "Rich Room",
              "subtitle": "No synced Webex messages yet.",
              "meta": "auto-reply=no | messages=0",
              "timestamp": "2026-05-15T04:28:20Z",
              "badge": "space",
              "status_badge": "live-webex",
              "detail_lines": ["Space Name: Rich Room", "Webex Checked: 2026-05-15T04:28:20Z", "Recent messages indexed: 0"],
              "detail_intro_lines": [],
              "detail_sections": [],
              "detail_tail_lines": []
            }
          ],
          "updated_at": "2026-05-15T04:28:20Z",
          "spaces": 1,
          "recent_messages": 0,
          "summary_generation_in_progress": false,
          "subjects_processed": 1,
          "subjects_total": 1
        }
        """
        let encoder = JSONEncoder()
        try encoder.encode(richNativeCache).write(
            to: nativeDirectory.appendingPathComponent("space_focus_cache_10d.native.json"),
            options: [.atomic]
        )
        try Data(sparseLivePayload.utf8).write(
            to: nativeDirectory.appendingPathComponent("live_space_focus_cache_10d.json"),
            options: [.atomic]
        )

        let loaded = try NativeRuntimeStore(configuration: configuration).loadFocusCache(kind: .space)
        let item = try XCTUnwrap(loaded.items.first)
        let detail = item.detailLines.joined(separator: "\n")

        XCTAssertEqual(item.meta, "auto-reply=no | messages=42")
        XCTAssertFalse(item.subtitle.localizedCaseInsensitiveContains("No synced Webex messages yet"))
        XCTAssertTrue(detail.contains("Space summary: Existing Codex summary."))
        XCTAssertTrue(detail.contains("Existing evidence."))
    }

    func testSparseLiveSnapshotPreservesPreviousMessageOnlyRows() throws {
        let runtimeRoot = FileManager.default.temporaryDirectory
            .appendingPathComponent("CubicleSparseLiveMessageOnlyTests-\(UUID().uuidString)", isDirectory: true)
        defer { try? FileManager.default.removeItem(at: runtimeRoot) }
        let knowledgeDirectory = runtimeRoot.appendingPathComponent("knowledge", isDirectory: true)
        let nativeDirectory = knowledgeDirectory.appendingPathComponent("native", isDirectory: true)
        try FileManager.default.createDirectory(at: nativeDirectory, withIntermediateDirectories: true)
        let configuration = RuntimeConfiguration(
            runtimeRoot: runtimeRoot,
            codexExecutable: "codex",
            webexBaseURL: URL(string: "https://webexapis.com/v1")!,
            webexPageSize: 100,
            webexRetryCount: 0,
            webexTimeoutSeconds: 1,
            webexOAuthTokenPathOverride: nil,
            webexOAuthRefreshSkewSeconds: 300,
            webexOAuthRefreshTokenSkewSeconds: 86_400
        )
        var settings = SystemSettings()
        settings.spaceFocusDays = 10
        try ConfigStore(configuration: configuration).saveSystemSettings(settings)
        let previousMessageOnlyCache = FocusCache(
            focusDays: 10,
            items: [
                FocusItem(
                    id: "spacefocus:room-message-only",
                    title: "Message Only Room",
                    subtitle: "Previous evidence is available.",
                    meta: "auto-reply=no | messages=7",
                    timestamp: "2026-05-14 16:15 PDT",
                    badge: "space",
                    statusBadge: "live-webex",
                    detailLines: [
                        "Space Name: Message Only Room",
                        "Recent conversations (last 10 days):",
                        "Webex 1: 2026-05-14 16:15 PDT | A | Message Only Room | Existing message evidence."
                    ],
                    detailIntroLines: [],
                    detailSections: [
                        FocusDetailSection(
                            id: "recent",
                            header: "Recent conversations (last 10 days):",
                            lines: ["Webex 1: 2026-05-14 16:15 PDT | A | Message Only Room | Existing message evidence."],
                            roomTitle: "Message Only Room",
                            summarySource: "Webex cache",
                            summaryGeneratedAt: ""
                        )
                    ],
                    detailTailLines: []
                )
            ],
            updatedAt: "2026-05-15T04:15:00Z",
            countLabel: "1",
            recentMessages: 7,
            summaryGenerationInProgress: false,
            subjectsProcessed: 1,
            subjectsTotal: 1
        )
        let sparseLivePayload = """
        {
          "items": [
            {
              "id": "room-message-only",
              "title": "Message Only Room",
              "subtitle": "No synced Webex messages yet.",
              "meta": "auto-reply=no | messages=0",
              "timestamp": "2026-05-15T04:28:20Z",
              "badge": "space",
              "status_badge": "live-webex",
              "detail_lines": ["Space Name: Message Only Room", "Webex Checked: 2026-05-15T04:28:20Z", "Recent messages indexed: 0"],
              "detail_intro_lines": [],
              "detail_sections": [],
              "detail_tail_lines": []
            }
          ],
          "updated_at": "2026-05-15T04:28:20Z",
          "spaces": 1,
          "recent_messages": 0,
          "summary_generation_in_progress": false,
          "subjects_processed": 1,
          "subjects_total": 1
        }
        """
        let encoder = JSONEncoder()
        try encoder.encode(previousMessageOnlyCache).write(
            to: nativeDirectory.appendingPathComponent("space_focus_cache_10d.native.json"),
            options: [.atomic]
        )
        try Data(sparseLivePayload.utf8).write(
            to: nativeDirectory.appendingPathComponent("live_space_focus_cache_10d.json"),
            options: [.atomic]
        )

        let loaded = try NativeRuntimeStore(configuration: configuration).loadFocusCache(kind: .space)
        let item = try XCTUnwrap(loaded.items.first)

        XCTAssertEqual(item.meta, "auto-reply=no | messages=7")
        XCTAssertEqual(item.timestamp, "2026-05-14 16:15 PDT")
        XCTAssertFalse(item.subtitle.localizedCaseInsensitiveContains("No synced Webex messages yet"))
        XCTAssertTrue(item.detailLines.joined(separator: "\n").contains("Existing message evidence."))
    }

    func testSparseLiveSnapshotDoesNotFallBackToSelfSourcedNativeCache() throws {
        let runtimeRoot = FileManager.default.temporaryDirectory
            .appendingPathComponent("CubicleSelfSourcedNativeCacheTests-\(UUID().uuidString)", isDirectory: true)
        defer { try? FileManager.default.removeItem(at: runtimeRoot) }
        let knowledgeDirectory = runtimeRoot.appendingPathComponent("knowledge", isDirectory: true)
        let nativeDirectory = knowledgeDirectory.appendingPathComponent("native", isDirectory: true)
        try FileManager.default.createDirectory(at: nativeDirectory, withIntermediateDirectories: true)
        let configuration = RuntimeConfiguration(
            runtimeRoot: runtimeRoot,
            codexExecutable: "codex",
            webexBaseURL: URL(string: "https://webexapis.com/v1")!,
            webexPageSize: 100,
            webexRetryCount: 0,
            webexTimeoutSeconds: 1,
            webexOAuthTokenPathOverride: nil,
            webexOAuthRefreshSkewSeconds: 300,
            webexOAuthRefreshTokenSkewSeconds: 86_400
        )
        var settings = SystemSettings()
        settings.spaceFocusDays = 7
        try ConfigStore(configuration: configuration).saveSystemSettings(settings)

        let liveCache = FocusCache(
            focusDays: 7,
            items: [
                FocusItem(
                    id: "room-self-source",
                    title: "Self Source Room",
                    subtitle: "No synced Webex messages yet.",
                    meta: "auto-reply=no | messages=0",
                    timestamp: "",
                    badge: "space",
                    statusBadge: "live-webex",
                    detailLines: ["Space Name: Self Source Room", "Recent messages indexed: 0"],
                    detailIntroLines: [],
                    detailSections: [],
                    detailTailLines: []
                )
            ],
            updatedAt: "2026-05-16T15:00:00Z",
            countLabel: "1",
            recentMessages: 0,
            summaryGenerationInProgress: false,
            subjectsProcessed: 1,
            subjectsTotal: 1
        )
        let selfSourcedNativeCache = FocusCache(
            focusDays: 7,
            items: [
                FocusItem(
                    id: "spacefocus:room-self-source",
                    title: "Self Source Room",
                    subtitle: "Generated native cache should not be reused as source.",
                    meta: "auto-reply=no | messages=88",
                    timestamp: "2026-05-10 09:00 PDT",
                    badge: "space",
                    statusBadge: "live-webex",
                    detailLines: [
                        "Space summary: Generated native payload that must not be used as input.",
                        "Recent conversations (last 7 days):",
                        "Webex 1: 2026-05-10 09:00 PDT | A | Self Source Room | Old generated evidence."
                    ],
                    detailIntroLines: ["Space summary: Generated native payload that must not be used as input."],
                    detailSections: [
                        FocusDetailSection(
                            id: "old-recent",
                            header: "Recent conversations (last 7 days):",
                            lines: ["Webex 1: 2026-05-10 09:00 PDT | A | Self Source Room | Old generated evidence."],
                            roomTitle: "Self Source Room",
                            summarySource: "Generated native cache",
                            summaryGeneratedAt: "2026-05-10T16:00:00Z"
                        )
                    ],
                    detailTailLines: []
                )
            ],
            updatedAt: "2026-05-10T16:00:00Z",
            countLabel: "1",
            recentMessages: 88,
            summaryGenerationInProgress: false,
            subjectsProcessed: 1,
            subjectsTotal: 1
        )
        let liveURL = nativeDirectory.appendingPathComponent("live_space_focus_cache_7d.json")
        let nativeURL = nativeDirectory.appendingPathComponent("space_focus_cache_7d.native.json")
        try writeFocusCache(liveCache, to: liveURL)
        try writeFocusCache(selfSourcedNativeCache, to: nativeURL)
        let selfSourcedManifest = """
        {
          "kind": "space",
          "focusDays": 7,
          "sourceSnapshotPath": "\(nativeURL.path)",
          "outputSnapshotPath": "\(nativeURL.path)",
          "sourceSignature": "self-sourced-generated-output",
          "normalizedEventCount": 88,
          "clusterCount": 1,
          "cacheReuseVersion": 12,
          "sourceFileSizeBytes": 1,
          "sourceModifiedAtEpoch": 1
        }
        """
        try Data(selfSourcedManifest.utf8).write(
            to: nativeDirectory.appendingPathComponent("space_focus_cache_manifest.json"),
            options: [.atomic]
        )

        let store = NativeRuntimeStore(configuration: configuration)
        let loaded = try store.loadFocusCache(kind: .space)
        let loadedItem = try XCTUnwrap(loaded.items.first)
        XCTAssertEqual(loadedItem.meta, "auto-reply=no | messages=0")
        XCTAssertFalse(loadedItem.detailLines.joined(separator: "\n").contains("Old generated evidence"))

        let outcome = try store.refreshSpaceFocusCache(forceRebuild: true)
        XCTAssertEqual(outcome.sourceSnapshotURL.lastPathComponent, "live_space_focus_cache_7d.json")

        let manifestPayload = try Data(contentsOf: nativeDirectory.appendingPathComponent("space_focus_cache_7d.manifest.json"))
        let manifest = try XCTUnwrap(JSONSerialization.jsonObject(with: manifestPayload) as? [String: Any])
        XCTAssertEqual(manifest["sourceSnapshotPath"] as? String, liveURL.path)
        XCTAssertEqual(manifest["outputSnapshotPath"] as? String, nativeURL.path)
    }

    func testSparsePersonLiveSnapshotWithIMessageEmptySubtitleKeepsPreviousEvidence() throws {
        let runtimeRoot = FileManager.default.temporaryDirectory
            .appendingPathComponent("CubiclePersonSparseIMessageTests-\(UUID().uuidString)", isDirectory: true)
        defer { try? FileManager.default.removeItem(at: runtimeRoot) }
        let knowledgeDirectory = runtimeRoot.appendingPathComponent("knowledge", isDirectory: true)
        let nativeDirectory = knowledgeDirectory.appendingPathComponent("native", isDirectory: true)
        try FileManager.default.createDirectory(at: nativeDirectory, withIntermediateDirectories: true)
        let configuration = RuntimeConfiguration(
            runtimeRoot: runtimeRoot,
            codexExecutable: "codex",
            webexBaseURL: URL(string: "https://webexapis.com/v1")!,
            webexPageSize: 100,
            webexRetryCount: 0,
            webexTimeoutSeconds: 1,
            webexOAuthTokenPathOverride: nil,
            webexOAuthRefreshSkewSeconds: 300,
            webexOAuthRefreshTokenSkewSeconds: 86_400
        )
        var settings = SystemSettings()
        settings.personFocusDays = 10
        try ConfigStore(configuration: configuration).saveSystemSettings(settings)

        let previousCache = FocusCache(
            focusDays: 10,
            items: [
                FocusItem(
                    id: "anil@cisco.com",
                    title: "Anil Nair",
                    subtitle: "Previous Webex and iMessage evidence.",
                    meta: "auto-reply=no",
                    timestamp: "2026-05-15 12:00 PDT",
                    badge: "person",
                    statusBadge: "live-webex",
                    detailLines: [
                        "Person Name: Anil Nair",
                        "Recent conversations (last 10 days):",
                        "iMessage 1: 2026-05-15 12:00 PDT | Me | iMessage - Anil Nair | Existing outbound reply.",
                        "Webex 1: 2026-05-15 11:00 PDT | Anil Nair | Anil Nair | Existing Webex evidence."
                    ],
                    detailIntroLines: [],
                    detailSections: [
                        FocusDetailSection(
                            id: "previous-recent",
                            header: "Recent conversations (last 10 days):",
                            lines: [
                                "iMessage 1: 2026-05-15 12:00 PDT | Me | iMessage - Anil Nair | Existing outbound reply.",
                                "Webex 1: 2026-05-15 11:00 PDT | Anil Nair | Anil Nair | Existing Webex evidence."
                            ],
                            roomTitle: "Anil Nair",
                            summarySource: "Webex and iMessage cache",
                            summaryGeneratedAt: ""
                        )
                    ],
                    detailTailLines: []
                )
            ],
            updatedAt: "2026-05-15T19:00:00Z",
            countLabel: "1",
            recentMessages: 2,
            summaryGenerationInProgress: false,
            subjectsProcessed: 1,
            subjectsTotal: 1
        )
        let sparseLiveCache = FocusCache(
            focusDays: 10,
            items: [
                FocusItem(
                    id: "anil@cisco.com",
                    title: "Anil Nair",
                    subtitle: "No synced Webex or iMessage messages yet.",
                    meta: "auto-reply=no",
                    timestamp: "",
                    badge: "person",
                    statusBadge: "live-webex",
                    detailLines: ["Person Name: Anil Nair", "Recent messages indexed: 0"],
                    detailIntroLines: [],
                    detailSections: [],
                    detailTailLines: []
                )
            ],
            updatedAt: "2026-05-16T15:00:00Z",
            countLabel: "1",
            recentMessages: 0,
            summaryGenerationInProgress: false,
            subjectsProcessed: 1,
            subjectsTotal: 1
        )
        try writeFocusCache(previousCache, to: nativeDirectory.appendingPathComponent("person_focus_cache_10d.native.json"))
        try writeFocusCache(sparseLiveCache, to: nativeDirectory.appendingPathComponent("live_person_focus_cache_10d.json"))

        let loaded = try NativeRuntimeStore(configuration: configuration).loadFocusCache(kind: .person)
        let item = try XCTUnwrap(loaded.items.first)
        let detail = item.detailLines.joined(separator: "\n")

        XCTAssertFalse(item.subtitle.localizedCaseInsensitiveContains("No synced Webex or iMessage messages yet"))
        XCTAssertTrue(detail.contains("Existing outbound reply."))
        XCTAssertTrue(detail.contains("Existing Webex evidence."))
    }

    func testPersonFocusMergeDropsStaleIMessageDeniedLineFromPreservedIntro() throws {
        let runtimeRoot = FileManager.default.temporaryDirectory
            .appendingPathComponent("CubiclePersonIntroMergeTests-\(UUID().uuidString)", isDirectory: true)
        defer { try? FileManager.default.removeItem(at: runtimeRoot) }
        let knowledgeDirectory = runtimeRoot.appendingPathComponent("knowledge", isDirectory: true)
        let nativeDirectory = knowledgeDirectory.appendingPathComponent("native", isDirectory: true)
        try FileManager.default.createDirectory(at: nativeDirectory, withIntermediateDirectories: true)
        let configuration = RuntimeConfiguration(
            runtimeRoot: runtimeRoot,
            codexExecutable: "codex",
            webexBaseURL: URL(string: "https://webexapis.com/v1")!,
            webexPageSize: 100,
            webexRetryCount: 0,
            webexTimeoutSeconds: 1,
            webexOAuthTokenPathOverride: nil,
            webexOAuthRefreshSkewSeconds: 300,
            webexOAuthRefreshTokenSkewSeconds: 86_400
        )
        var settings = SystemSettings()
        settings.personFocusDays = 21
        try ConfigStore(configuration: configuration).saveSystemSettings(settings)

        let retainedTail = [
            "Person summary: Purna is driving follow-ups on notification completion and country date confirmation.",
            "Summary source: Codex",
            "Analysis Generated: 2026-05-16 09:22:28 PDT"
        ] + (1...90).map { "Codex context line \($0)." }

        let previousCache = FocusCache(
            focusDays: 21,
            items: [
                FocusItem(
                    id: "purna@cisco.com",
                    title: "Purna Chandra Chevukula",
                    subtitle: "Older enriched payload",
                    meta: "auto-reply=no | messages=119",
                    timestamp: "2026-05-14 16:30:04 PDT",
                    badge: "person",
                    statusBadge: "live-webex+imessage",
                    detailLines: [
                        "Space Name: Purna Chandra Chevukula",
                        "Person Name: Purna Chandra Chevukula",
                        "Webex Checked: 2026-05-16 08:57:51 PDT",
                        "Recent messages indexed: 119",
                        "iMessage handles: +12142504013",
                        "iMessage unavailable: Could not open iMessage database /Users/prabhat7/Library/Messages/chat.db: authorization denied",
                        "",
                        "Recent conversations (last 21 days):",
                        "Webex 1: 2026-05-14 16:30:04 PDT | Purna Chandra Chevukula | Purna Chandra Chevukula | Existing cached timeline event.",
                        "",
                        "Person summary: Purna is driving follow-ups on notification completion and country date confirmation.",
                        "Summary source: Codex",
                        "Analysis Generated: 2026-05-16 09:22:28 PDT"
                    ],
                    detailIntroLines: [
                        "Space Name: Purna Chandra Chevukula",
                        "Person Name: Purna Chandra Chevukula",
                        "Webex Checked: 2026-05-16 08:57:51 PDT",
                        "Recent messages indexed: 119",
                        "iMessage handles: +12142504013",
                        "iMessage unavailable: Could not open iMessage database /Users/prabhat7/Library/Messages/chat.db: authorization denied"
                    ],
                    detailSections: [
                        FocusDetailSection(
                            id: "previous-recent",
                            header: "Recent conversations (last 21 days):",
                            lines: ["Webex 1: 2026-05-14 16:30:04 PDT | Purna Chandra Chevukula | Purna Chandra Chevukula | Existing cached timeline event."],
                            roomTitle: "Purna Chandra Chevukula",
                            summarySource: "Webex and iMessage cache",
                            summaryGeneratedAt: ""
                        )
                    ],
                    detailTailLines: retainedTail
                )
            ],
            updatedAt: "2026-05-16T16:22:28Z",
            countLabel: "1",
            recentMessages: 119,
            summaryGenerationInProgress: false,
            subjectsProcessed: 1,
            subjectsTotal: 1
        )
        let freshLiveCache = FocusCache(
            focusDays: 21,
            items: [
                FocusItem(
                    id: "purna@cisco.com",
                    title: "Purna Chandra Chevukula",
                    subtitle: "Notifications update latest timeline event.",
                    meta: "auto-reply=no | messages=119",
                    timestamp: "2026-05-14 16:30:04 PDT",
                    badge: "person",
                    statusBadge: "webex+imessage",
                    detailLines: [
                        "Space Name: Purna Chandra Chevukula",
                        "Person Name: Purna Chandra Chevukula",
                        "Webex Checked: 2026-05-16 09:40:00 PDT",
                        "Recent messages indexed: 119",
                        "iMessage handles: +12142504013",
                        "",
                        "Recent conversations (last 21 days):",
                        "iMessage imessage-1: 2026-05-14 16:30:04 PDT | Purna Chandra Chevukula | iMessage - Purna Chandra Chevukula | Fresh iMessage timeline event."
                    ],
                    detailIntroLines: [
                        "Space Name: Purna Chandra Chevukula",
                        "Person Name: Purna Chandra Chevukula",
                        "Webex Checked: 2026-05-16 09:40:00 PDT",
                        "Recent messages indexed: 119",
                        "iMessage handles: +12142504013"
                    ],
                    detailSections: [],
                    detailTailLines: []
                )
            ],
            updatedAt: "2026-05-16T16:40:00Z",
            countLabel: "1",
            recentMessages: 119,
            summaryGenerationInProgress: false,
            subjectsProcessed: 1,
            subjectsTotal: 1
        )

        try writeFocusCache(previousCache, to: nativeDirectory.appendingPathComponent("person_focus_cache_21d.native.json"))
        try writeFocusCache(freshLiveCache, to: nativeDirectory.appendingPathComponent("live_person_focus_cache_21d.json"))

        let loaded = try NativeRuntimeStore(configuration: configuration).loadFocusCache(kind: .person)
        let item = try XCTUnwrap(loaded.items.first)
        let detail = item.detailLines.joined(separator: "\n")

        XCTAssertFalse(detail.contains("iMessage unavailable:"))
        XCTAssertFalse(detail.contains("authorization denied"))
        XCTAssertTrue(detail.contains("Fresh iMessage timeline event."))
        XCTAssertTrue(detail.contains("Person summary: Purna is driving follow-ups"))
    }

    func testFocusWindowDoesNotReuseEnrichmentFromDifferentLookback() throws {
        let runtimeRoot = FileManager.default.temporaryDirectory
            .appendingPathComponent("CubicleFocusWindowIsolationTests-\(UUID().uuidString)", isDirectory: true)
        defer { try? FileManager.default.removeItem(at: runtimeRoot) }
        let knowledgeDirectory = runtimeRoot.appendingPathComponent("knowledge", isDirectory: true)
        let nativeDirectory = knowledgeDirectory.appendingPathComponent("native", isDirectory: true)
        try FileManager.default.createDirectory(at: nativeDirectory, withIntermediateDirectories: true)
        let configuration = RuntimeConfiguration(
            runtimeRoot: runtimeRoot,
            codexExecutable: "codex",
            webexBaseURL: URL(string: "https://webexapis.com/v1")!,
            webexPageSize: 100,
            webexRetryCount: 0,
            webexTimeoutSeconds: 1,
            webexOAuthTokenPathOverride: nil,
            webexOAuthRefreshSkewSeconds: 300,
            webexOAuthRefreshTokenSkewSeconds: 86_400
        )
        var settings = SystemSettings()
        settings.personFocusDays = 7
        try ConfigStore(configuration: configuration).saveSystemSettings(settings)

        let oldTenDayCache = FocusCache(
            focusDays: 10,
            items: [
                FocusItem(
                    id: "anil@cisco.com",
                    title: "Anil Nair",
                    subtitle: "Older 10-day evidence.",
                    meta: "auto-reply=no | messages=76",
                    timestamp: "2026-05-14 10:00 PDT",
                    badge: "person",
                    statusBadge: "live-webex",
                    detailLines: [
                        "Person summary: Older 10-day summary.",
                        "Recent topic clusters (last 10 days):",
                        "Context 1: old evidence that must not leak into a 7-day window."
                    ],
                    detailIntroLines: ["Person summary: Older 10-day summary."],
                    detailSections: [
                        FocusDetailSection(
                            id: "old-recent",
                            header: "Recent topic clusters (last 10 days):",
                            lines: ["Context 1: old evidence that must not leak into a 7-day window."],
                            roomTitle: "Anil Nair",
                            summarySource: "Codex cache",
                            summaryGeneratedAt: "2026-05-14T17:00:00Z"
                        )
                    ],
                    detailTailLines: []
                )
            ],
            updatedAt: "2026-05-14T17:00:00Z",
            countLabel: "1",
            recentMessages: 76,
            summaryGenerationInProgress: false,
            subjectsProcessed: 1,
            subjectsTotal: 1
        )
        let liveSevenDayCache = FocusCache(
            focusDays: 7,
            items: [
                FocusItem(
                    id: "anil@cisco.com",
                    title: "Anil Nair",
                    subtitle: "No synced Webex messages yet.",
                    meta: "auto-reply=no | messages=0",
                    timestamp: "",
                    badge: "person",
                    statusBadge: "live-webex",
                    detailLines: [
                        "Space Name: Anil Nair",
                        "Live Webex Sync: 2026-05-15T23:17:11Z",
                        "Recent messages indexed: 0"
                    ],
                    detailIntroLines: [],
                    detailSections: [],
                    detailTailLines: []
                )
            ],
            updatedAt: "2026-05-15T23:17:11Z",
            countLabel: "1",
            recentMessages: 0,
            summaryGenerationInProgress: false,
            subjectsProcessed: 1,
            subjectsTotal: 1
        )
        let encoder = JSONEncoder()
        let oldNativeURL = nativeDirectory.appendingPathComponent("person_focus_cache_10d.native.json")
        try encoder.encode(oldTenDayCache).write(to: oldNativeURL, options: [.atomic])
        try encoder.encode(liveSevenDayCache).write(
            to: nativeDirectory.appendingPathComponent("live_person_focus_cache_7d.json"),
            options: [.atomic]
        )
        let oldManifestPayload = """
        {
          "kind": "person",
          "focusDays": 10,
          "sourceSnapshotPath": "\(knowledgeDirectory.appendingPathComponent("person_focus_cache_10d.json").path)",
          "outputSnapshotPath": "\(oldNativeURL.path)",
          "sourceSignature": "old-ten-day",
          "normalizedEventCount": 76,
          "clusterCount": 1,
          "cacheReuseVersion": 11,
          "sourceFileSizeBytes": 1,
          "sourceModifiedAtEpoch": 1
        }
        """
        try Data(oldManifestPayload.utf8).write(
            to: nativeDirectory.appendingPathComponent("person_focus_cache_manifest.json"),
            options: [.atomic]
        )

        let loaded = try NativeRuntimeStore(configuration: configuration).loadFocusCache(kind: .person)
        let item = try XCTUnwrap(loaded.items.first)
        let detail = item.detailLines.joined(separator: "\n")

        XCTAssertEqual(loaded.focusDays, 7)
        XCTAssertEqual(item.meta, "auto-reply=no | messages=0")
        XCTAssertTrue(detail.contains("Recent messages indexed: 0"))
        XCTAssertFalse(detail.contains("Older 10-day summary"))
        XCTAssertFalse(detail.contains("old evidence that must not leak"))
    }

    @MainActor
    func testChangingPersonFocusDaysReloadsAskCodexContextImmediately() async throws {
        let runtimeRoot = FileManager.default.temporaryDirectory
            .appendingPathComponent("CubicleAskCodexWindowReloadTests-\(UUID().uuidString)", isDirectory: true)
        defer { try? FileManager.default.removeItem(at: runtimeRoot) }
        let knowledgeDirectory = runtimeRoot.appendingPathComponent("knowledge", isDirectory: true)
        try FileManager.default.createDirectory(at: knowledgeDirectory, withIntermediateDirectories: true)
        let configuration = RuntimeConfiguration(
            runtimeRoot: runtimeRoot,
            codexExecutable: "codex",
            webexBaseURL: URL(string: "https://webexapis.com/v1")!,
            webexPageSize: 100,
            webexRetryCount: 0,
            webexTimeoutSeconds: 1,
            webexOAuthTokenPathOverride: nil,
            webexOAuthRefreshSkewSeconds: 300,
            webexOAuthRefreshTokenSkewSeconds: 86_400
        )
        var settings = SystemSettings()
        settings.personFocusDays = 90
        settings.spaceFocusDays = 60
        settings.codexQuestionSynthesisEnabled = false
        try ConfigStore(configuration: configuration).saveSystemSettings(settings)

        let oldPersonCache = FocusCache(
            focusDays: 90,
            items: [
                FocusItem(
                    id: "anil@cisco.com",
                    title: "Anil Nair",
                    subtitle: "Older 90-day evidence.",
                    meta: "messages=76",
                    timestamp: "2026-05-14 10:00 PDT",
                    badge: "person",
                    statusBadge: "live-webex",
                    detailLines: ["Old 90-day evidence should disappear."],
                    detailIntroLines: [],
                    detailSections: [],
                    detailTailLines: []
                )
            ],
            updatedAt: "2026-05-14T17:00:00Z",
            countLabel: "1",
            recentMessages: 76,
            summaryGenerationInProgress: false,
            subjectsProcessed: 1,
            subjectsTotal: 1
        )
        let sevenDayPersonCache = FocusCache(
            focusDays: 7,
            items: [
                FocusItem(
                    id: "anil@cisco.com",
                    title: "Anil Nair",
                    subtitle: "Current 7-day evidence.",
                    meta: "messages=1",
                    timestamp: "2026-05-15 09:00 PDT",
                    badge: "person",
                    statusBadge: "live-webex",
                    detailLines: ["Fresh 7-day evidence should appear."],
                    detailIntroLines: [],
                    detailSections: [],
                    detailTailLines: []
                )
            ],
            updatedAt: "2026-05-15T16:00:00Z",
            countLabel: "1",
            recentMessages: 1,
            summaryGenerationInProgress: false,
            subjectsProcessed: 1,
            subjectsTotal: 1
        )
        let spaceCache = FocusCache(
            focusDays: 60,
            items: [
                FocusItem(
                    id: "spacefocus:test-room",
                    title: "Test Room",
                    subtitle: "Space context.",
                    meta: "messages=1",
                    timestamp: "2026-05-15 09:00 PDT",
                    badge: "space",
                    statusBadge: "live-webex",
                    detailLines: ["Space evidence."],
                    detailIntroLines: [],
                    detailSections: [],
                    detailTailLines: []
                )
            ],
            updatedAt: "2026-05-15T16:00:00Z",
            countLabel: "1",
            recentMessages: 1,
            summaryGenerationInProgress: false,
            subjectsProcessed: 1,
            subjectsTotal: 1
        )
        let encoder = JSONEncoder()
        try encoder.encode(oldPersonCache).write(
            to: knowledgeDirectory.appendingPathComponent("person_focus_cache_90d.json"),
            options: [.atomic]
        )
        try encoder.encode(sevenDayPersonCache).write(
            to: knowledgeDirectory.appendingPathComponent("person_focus_cache_7d.json"),
            options: [.atomic]
        )
        try encoder.encode(spaceCache).write(
            to: knowledgeDirectory.appendingPathComponent("space_focus_cache_60d.json"),
            options: [.atomic]
        )

        let model = AppModel(runtimeStore: NativeRuntimeStore(configuration: configuration))
        let loaded = await model.loadAll()
        XCTAssertTrue(loaded)
        model.askCodexTargetScope = .selectedPerson
        let initialContext = model.askCodexContextPreviewLines().joined(separator: "\n")
        XCTAssertTrue(initialContext.contains("Old 90-day evidence should disappear."))

        model.updateSystemSetting(.personFocusDays, intValue: 7)
        let reloadedContext = model.askCodexContextPreviewLines().joined(separator: "\n")

        XCTAssertEqual(model.systemSettings.personFocusDays, 7)
        XCTAssertEqual(model.personCache.focusDays, 7)
        XCTAssertTrue(reloadedContext.contains("Fresh 7-day evidence should appear."))
        XCTAssertFalse(reloadedContext.contains("Old 90-day evidence should disappear."))
    }

    func testCodexEnrichedSnapshotPublishesAsNativeCacheSource() throws {
        let runtimeRoot = FileManager.default.temporaryDirectory
            .appendingPathComponent("CubicleCodexEnrichedPublishTests-\(UUID().uuidString)", isDirectory: true)
        defer { try? FileManager.default.removeItem(at: runtimeRoot) }
        let knowledgeDirectory = runtimeRoot.appendingPathComponent("knowledge", isDirectory: true)
        let nativeDirectory = knowledgeDirectory.appendingPathComponent("native", isDirectory: true)
        try FileManager.default.createDirectory(at: nativeDirectory, withIntermediateDirectories: true)
        let configuration = RuntimeConfiguration(
            runtimeRoot: runtimeRoot,
            codexExecutable: "codex",
            webexBaseURL: URL(string: "https://webexapis.com/v1")!,
            webexPageSize: 100,
            webexRetryCount: 0,
            webexTimeoutSeconds: 1,
            webexOAuthTokenPathOverride: nil,
            webexOAuthRefreshSkewSeconds: 300,
            webexOAuthRefreshTokenSkewSeconds: 86_400
        )
        var settings = SystemSettings()
        settings.spaceFocusDays = 10
        try ConfigStore(configuration: configuration).saveSystemSettings(settings)

        let codexItem = FocusItem(
            id: "room-summary",
            title: "Summary Room",
            subtitle: "Latest evidence.",
            meta: "auto-reply=no | messages=2",
            timestamp: "2026-05-14T20:00:00Z",
            badge: "space",
            statusBadge: "live-webex",
            detailLines: [
                "Space summary: Codex says this room is about summary preservation.",
                "Current posture / next move: Keep the Codex summary after native publication.",
                "Guidance freshness: Updated by Codex at 2026-05-14T20:01:00Z from live Webex sync.",
                "Space summary source: Codex",
                "Summary generated: 2026-05-14T20:01:00Z",
                "",
                "Recent conversations (last 10 days):",
                "Webex 1: 2026-05-14T20:00:00Z | A | Summary Room | Existing message evidence.",
                "Webex 2: 2026-05-14T19:00:00Z | B | Summary Room | Follow-up evidence."
            ],
            detailIntroLines: [],
            detailSections: [],
            detailTailLines: []
        ).assembledDetailPayload(kind: .space, focusDays: 10, clusterSeeds: [])
        let codexCache = FocusCache(
            focusDays: 10,
            items: [codexItem],
            updatedAt: "2026-05-14T20:01:00Z",
            countLabel: "1",
            recentMessages: 2,
            summaryGenerationInProgress: false,
            subjectsProcessed: 1,
            subjectsTotal: 1
        )
        let livePayload = """
        {
          "items": [
            {
              "id": "room-summary",
              "title": "Summary Room",
              "subtitle": "Latest evidence.",
              "meta": "auto-reply=no | messages=2",
              "timestamp": "2026-05-14T20:00:00Z",
              "badge": "space",
              "status_badge": "live-webex",
              "detail_lines": ["Space Name: Summary Room", "Recent messages indexed: 2", "", "Recent conversations (last 10 days):", "Webex 1: live-only"],
              "detail_intro_lines": [],
              "detail_sections": [],
              "detail_tail_lines": []
            }
          ],
          "updated_at": "2026-05-14T20:02:00Z",
          "spaces": 1,
          "recent_messages": 2,
          "summary_generation_in_progress": false,
          "subjects_processed": 1,
          "subjects_total": 1
        }
        """
        let encoder = JSONEncoder()
        let codexURL = knowledgeDirectory.appendingPathComponent("space_focus_cache_10d.json")
        try encoder.encode(codexCache).write(to: codexURL, options: [.atomic])
        try Data(livePayload.utf8).write(
            to: nativeDirectory.appendingPathComponent("live_space_focus_cache_10d.json"),
            options: [.atomic]
        )

        let outcome = try NativeRuntimeStore(configuration: configuration).refreshFocusCache(
            kind: .space,
            sourceURL: codexURL,
            forceRebuild: true
        )
        let published = try JSONDecoder().decode(FocusCache.self, from: Data(contentsOf: outcome.outputSnapshotURL))
        let item = try XCTUnwrap(published.items.first)
        let detail = item.detailLines.joined(separator: "\n")

        XCTAssertEqual(outcome.sourceSnapshotURL.lastPathComponent, "space_focus_cache_10d.json")
        XCTAssertTrue(detail.contains("Space summary: Codex says this room is about summary preservation."))
        XCTAssertTrue(detail.contains("Current posture / next move: Keep the Codex summary after native publication."))
        XCTAssertFalse(detail.contains("Webex 1: live-only"))
    }

    func testLocalHeuristicConversationSectionsAreHiddenFromGeneratedDetails() throws {
        let item = FocusItem(
            id: "spacefocus:test-room",
            title: "Test Room",
            subtitle: "",
            meta: "",
            timestamp: "2026-05-14 16:15 PDT",
            badge: "space",
            statusBadge: "live-webex",
            detailLines: [
                "Space summary: Codex summary.",
                "Space summary source: Codex",
                "",
                "Recent conversations (last 60 days):",
                "Webex 1: 2026-05-14 16:15 PDT | A | Test Room | Recent update.",
                "",
                "Conversation extraction source: Local heuristic clustering",
                "Status: Unclear (inferred from archive)",
                "Summary source: Heuristic",
                "Summary: Local fallback should not be displayed as generated output."
            ],
            detailIntroLines: [],
            detailSections: [],
            detailTailLines: []
        )

        let assembled = item.assembledDetailPayload(kind: .space, focusDays: 60, clusterSeeds: [])
        let detail = assembled.detailLines.joined(separator: "\n")

        XCTAssertFalse(assembled.detailSections.contains { $0.header.localizedCaseInsensitiveContains("Local heuristic") })
        XCTAssertFalse(detail.localizedCaseInsensitiveContains("Local heuristic"))
        XCTAssertFalse(detail.localizedCaseInsensitiveContains("Summary source: Heuristic"))
        XCTAssertTrue(detail.contains("Space summary: Codex summary."))
        XCTAssertTrue(assembled.detailSections.contains { $0.header.hasPrefix("Recent conversations") })
    }

    func testQuestionRefreshDoesNotPublishDeterministicQuestionsWithoutSynthesizer() async throws {
        let runtimeRoot = FileManager.default.temporaryDirectory
            .appendingPathComponent("CubicleQuestionEngineTests-\(UUID().uuidString)", isDirectory: true)
        try FileManager.default.createDirectory(at: runtimeRoot, withIntermediateDirectories: true)
        let configuration = RuntimeConfiguration(
            runtimeRoot: runtimeRoot,
            codexExecutable: "codex",
            webexBaseURL: URL(string: "https://webexapis.com/v1")!,
            webexPageSize: 100,
            webexRetryCount: 0,
            webexTimeoutSeconds: 1,
            webexOAuthTokenPathOverride: nil,
            webexOAuthRefreshSkewSeconds: 300,
            webexOAuthRefreshTokenSkewSeconds: 86_400
        )
        let store = KnowledgeStore(configuration: configuration)
        let service = QuestionCandidateService(knowledgeStore: store)

        let spaceCache = FocusCache(
            focusDays: 60,
            items: [Self.spaceItem],
            updatedAt: "2026-05-14T07:00:00Z",
            countLabel: "1",
            recentMessages: 3,
            summaryGenerationInProgress: false,
            subjectsProcessed: 1,
            subjectsTotal: 1
        )
        let personCache = FocusCache(
            focusDays: 60,
            items: [Self.personItem],
            updatedAt: "2026-05-14T07:00:00Z",
            countLabel: "1",
            recentMessages: 4,
            summaryGenerationInProgress: false,
            subjectsProcessed: 1,
            subjectsTotal: 1
        )

        let outcome = try await service.refreshQuestionCandidates(spaceCache: spaceCache, personCache: personCache)
        let candidates = try service.listQuestionCandidates(limit: 20)

        XCTAssertEqual(outcome.generatedCount, 0)
        XCTAssertTrue(candidates.isEmpty)
    }

    func testQuestionRefreshPreservesExistingRowsWhenCodexSynthesizerReturnsEmpty() async throws {
        let runtimeRoot = FileManager.default.temporaryDirectory
            .appendingPathComponent("CubicleQuestionEngineEmptyCodexTests-\(UUID().uuidString)", isDirectory: true)
        defer { try? FileManager.default.removeItem(at: runtimeRoot) }
        try FileManager.default.createDirectory(at: runtimeRoot, withIntermediateDirectories: true)
        let configuration = RuntimeConfiguration(
            runtimeRoot: runtimeRoot,
            codexExecutable: "codex",
            webexBaseURL: URL(string: "https://webexapis.com/v1")!,
            webexPageSize: 100,
            webexRetryCount: 0,
            webexTimeoutSeconds: 1,
            webexOAuthTokenPathOverride: nil,
            webexOAuthRefreshSkewSeconds: 300,
            webexOAuthRefreshTokenSkewSeconds: 86_400
        )
        let store = KnowledgeStore(configuration: configuration)
        let service = QuestionCandidateService(
            knowledgeStore: store,
            questionSynthesizer: EmptyQuestionSynthesizer()
        )
        let existing = QuestionCandidate(
            id: "question-existing-codex",
            scopeType: .space,
            scopeKey: Self.spaceItem.id,
            scopeLabel: Self.spaceItem.title,
            questionText: "Which existing follow-up should remain visible?",
            questionType: "codex_synthesized_question",
            whyNow: "A previous successful Codex synthesis produced this question.",
            evidence: [
                QuestionEvidenceRef(
                    sourceType: "event",
                    sourceID: "event-existing",
                    createdAt: nil,
                    label: "Existing evidence",
                    preview: "Existing evidence"
                )
            ],
            sourceKind: "codex_question_synthesis",
            sourceKey: "existing",
            tags: ["codex", "synthesis"],
            priorityScore: 120,
            status: .candidate,
            answerSnapshotId: nil,
            createdAt: Date(),
            updatedAt: Date(),
            expiresAt: Calendar.current.date(byAdding: .day, value: 14, to: Date())
        )
        try store.upsertQuestionCandidates([existing])
        let spaceCache = FocusCache(
            focusDays: 60,
            items: [Self.spaceItem],
            updatedAt: "2026-05-14T07:00:00Z",
            countLabel: "1",
            recentMessages: 3,
            summaryGenerationInProgress: false,
            subjectsProcessed: 1,
            subjectsTotal: 1
        )
        let personCache = FocusCache(
            focusDays: 60,
            items: [Self.personItem],
            updatedAt: "2026-05-14T07:00:00Z",
            countLabel: "1",
            recentMessages: 4,
            summaryGenerationInProgress: false,
            subjectsProcessed: 1,
            subjectsTotal: 1
        )

        let outcome = try await service.refreshQuestionCandidates(spaceCache: spaceCache, personCache: personCache)
        let candidates = try service.listQuestionCandidates(limit: 20)

        XCTAssertEqual(outcome.generatedCount, 0)
        XCTAssertEqual(candidates.map(\.id), ["question-existing-codex"])
    }

    func testCubiclePublicationPolicyRejectsRawMetricQuestions() {
        let rawMetricQuestion = GeneratedQuestion(
            text: "How do high-question vs low-question threads differ on duration seconds?",
            category: .comparative,
            rationale: "High and low cohorts differ on duration seconds by 109478.58 across 63 threads.",
            supportingMetrics: ["metric": "duration_seconds", "sample_size": "63"],
            suggestedAnalysis: "Compare topic mix, participants, and timing across the two cohorts.",
            interestingnessScore: 1,
            actionabilityScore: 0.8,
            confidenceScore: 0.9
        )

        let usefulQuestion = GeneratedQuestion(
            text: "Where do questions about customer approval go unanswered, and what decision owner is missing?",
            category: .efficiency,
            rationale: "Customer approval has unanswered questions.",
            supportingMetrics: ["topic": "customer approval"],
            suggestedAnalysis: "Trace unanswered questions to senders and owner gaps.",
            interestingnessScore: 0.9,
            actionabilityScore: 0.95,
            confidenceScore: 0.8
        )

        XCTAssertFalse(QuestionCandidateService.isPublishableCoreQuestionForCubicle(rawMetricQuestion))
        XCTAssertTrue(QuestionCandidateService.isPublishableCoreQuestionForCubicle(usefulQuestion))
    }

    func testQuestionRefreshAddsCodexSynthesisCandidatesWhenProviderIsAvailable() async throws {
        let runtimeRoot = FileManager.default.temporaryDirectory
            .appendingPathComponent("CubicleQuestionEngineCodexTests-\(UUID().uuidString)", isDirectory: true)
        try FileManager.default.createDirectory(at: runtimeRoot, withIntermediateDirectories: true)
        let configuration = RuntimeConfiguration(
            runtimeRoot: runtimeRoot,
            codexExecutable: "codex",
            webexBaseURL: URL(string: "https://webexapis.com/v1")!,
            webexPageSize: 100,
            webexRetryCount: 0,
            webexTimeoutSeconds: 1,
            webexOAuthTokenPathOverride: nil,
            webexOAuthRefreshSkewSeconds: 300,
            webexOAuthRefreshTokenSkewSeconds: 86_400
        )
        let store = KnowledgeStore(configuration: configuration)
        let synthesizer = StubQuestionSynthesizer()
        let service = QuestionCandidateService(
            knowledgeStore: store,
            questionSynthesizer: synthesizer
        )

        let spaceCache = FocusCache(
            focusDays: 60,
            items: [Self.spaceItem],
            updatedAt: "2026-05-14T07:00:00Z",
            countLabel: "1",
            recentMessages: 3,
            summaryGenerationInProgress: false,
            subjectsProcessed: 1,
            subjectsTotal: 1
        )
        let personCache = FocusCache(
            focusDays: 60,
            items: [Self.personItem],
            updatedAt: "2026-05-14T07:00:00Z",
            countLabel: "1",
            recentMessages: 4,
            summaryGenerationInProgress: false,
            subjectsProcessed: 1,
            subjectsTotal: 1
        )

        let outcome = try await service.refreshQuestionCandidates(spaceCache: spaceCache, personCache: personCache)
        let candidates = try service.listQuestionCandidates(limit: 20)
        let synthesized = candidates.filter { $0.sourceKind == "codex_question_synthesis" }

        XCTAssertGreaterThan(synthesizer.receivedCandidateCount, 0)
        XCTAssertEqual(outcome.codexSynthesizedCount, 1)
        XCTAssertEqual(synthesized.count, 1)
        XCTAssertEqual(synthesized.first?.questionType, "codex_synthesized_question")
        XCTAssertTrue(synthesized.first?.questionText.localizedCaseInsensitiveContains("owner") == true)
        XCTAssertTrue(candidates.allSatisfy { $0.sourceKind == "codex_question_synthesis" })
        XCTAssertFalse(candidates.contains { $0.sourceKind == "webex_qg_core" })
        XCTAssertFalse(candidates.contains { $0.sourceKind == "focus_event" || $0.sourceKind == "focus_cache" })
        XCTAssertFalse(candidates.contains { $0.questionText.localizedCaseInsensitiveContains("duration seconds") })
    }

    func testPersonFocusPreferencesPersistIMessageHandles() throws {
        let runtimeRoot = FileManager.default.temporaryDirectory
            .appendingPathComponent("CubiclePersonIMessagePreferenceTests-\(UUID().uuidString)", isDirectory: true)
        defer { try? FileManager.default.removeItem(at: runtimeRoot) }
        let configuration = RuntimeConfiguration(
            runtimeRoot: runtimeRoot,
            codexExecutable: "codex",
            webexBaseURL: URL(string: "https://webexapis.com/v1")!,
            webexPageSize: 100,
            webexRetryCount: 0,
            webexTimeoutSeconds: 1,
            webexOAuthTokenPathOverride: nil,
            webexOAuthRefreshSkewSeconds: 300,
            webexOAuthRefreshTokenSkewSeconds: 86_400
        )
        let configStore = ConfigStore(configuration: configuration)
        try FileManager.default.createDirectory(at: configStore.configDirectory, withIntermediateDirectories: true)
        try """
        sender\tAnil Nair\tY2lzY29zcGFyazovL3VzL1JPT00vQU5JTA\tdirect\tanil@cisco.com\tAnil Nair
        """.write(to: configStore.importantTargetsURL, atomically: true, encoding: .utf8)

        let target = try XCTUnwrap(configStore.personFocusManagementTargets().first)
        XCTAssertTrue(try configStore.addPersonIMessageHandle("(408) 555-0123", to: target))
        XCTAssertFalse(try configStore.addPersonIMessageHandle("+1 408 555 0123", to: target))

        var updatedTarget = try XCTUnwrap(configStore.personFocusManagementTargets().first)
        XCTAssertEqual(updatedTarget.iMessageHandles, ["+14085550123"])

        XCTAssertTrue(try configStore.removePersonIMessageHandle("4085550123", from: updatedTarget))
        updatedTarget = try XCTUnwrap(configStore.personFocusManagementTargets().first)
        XCTAssertEqual(updatedTarget.iMessageHandles, [])
    }

    func testNativeIMessageIngestionIncludesOutboundRepliesInMatchedThread() throws {
        let runtimeRoot = FileManager.default.temporaryDirectory
            .appendingPathComponent("CubicleIMessageThreadTests-\(UUID().uuidString)", isDirectory: true)
        defer { try? FileManager.default.removeItem(at: runtimeRoot) }
        try FileManager.default.createDirectory(at: runtimeRoot, withIntermediateDirectories: true)
        let databaseURL = runtimeRoot.appendingPathComponent("chat.db")
        let formatter = ISO8601DateFormatter()
        formatter.formatOptions = [.withInternetDateTime, .withFractionalSeconds]
        let inboundDate = try XCTUnwrap(formatter.date(from: "2026-05-15T19:38:50.016Z"))
        let outboundDate = try XCTUnwrap(formatter.date(from: "2026-05-15T19:40:00.000Z"))
        let appleEpochOffset: TimeInterval = 978_307_200
        let inboundRawDate = Int64(inboundDate.timeIntervalSince1970 - appleEpochOffset)
        let outboundRawDate = Int64(outboundDate.timeIntervalSince1970 - appleEpochOffset)

        try executeSQLite(
            """
            CREATE TABLE handle (ROWID INTEGER PRIMARY KEY, id TEXT);
            CREATE TABLE chat (ROWID INTEGER PRIMARY KEY, guid TEXT, chat_identifier TEXT, display_name TEXT);
            CREATE TABLE chat_handle_join (chat_id INTEGER, handle_id INTEGER);
            CREATE TABLE chat_message_join (chat_id INTEGER, message_id INTEGER);
            CREATE TABLE message (ROWID INTEGER PRIMARY KEY, guid TEXT, text TEXT, date INTEGER, is_from_me INTEGER, handle_id INTEGER);
            INSERT INTO handle (ROWID, id) VALUES (1, '+14085550123');
            INSERT INTO chat (ROWID, guid, chat_identifier, display_name) VALUES (1, 'chat-guid', '+14085550123', '');
            INSERT INTO chat_handle_join (chat_id, handle_id) VALUES (1, 1);
            INSERT INTO message (ROWID, guid, text, date, is_from_me, handle_id) VALUES (1, 'inbound-guid', 'Inbound from Anil.', \(inboundRawDate), 0, 1);
            INSERT INTO message (ROWID, guid, text, date, is_from_me, handle_id) VALUES (2, 'outbound-guid', 'My reply to Anil.', \(outboundRawDate), 1, NULL);
            INSERT INTO chat_message_join (chat_id, message_id) VALUES (1, 1);
            INSERT INTO chat_message_join (chat_id, message_id) VALUES (1, 2);
            """,
            databaseURL: databaseURL
        )

        let service = NativeIMessageIngestionService(chatDatabaseURL: databaseURL)
        let messages = try service.loadMessages(
            matching: ["408-555-0123"],
            displayName: "Anil Nair",
            since: inboundDate.addingTimeInterval(-60),
            limit: 10
        )

        XCTAssertEqual(messages.map(\.body), ["My reply to Anil.", "Inbound from Anil."])
        XCTAssertEqual(messages.first?.sender, "Me")
        XCTAssertEqual(messages.last?.sender, "Anil Nair")
        XCTAssertEqual(messages.first?.threadTitle, "iMessage - Anil Nair")
    }

    func testPersonFocusCacheCombinesWebexAndIMessageTimeline() throws {
        let runtimeRoot = FileManager.default.temporaryDirectory
            .appendingPathComponent("CubiclePersonTimelineMergeTests-\(UUID().uuidString)", isDirectory: true)
        defer { try? FileManager.default.removeItem(at: runtimeRoot) }
        let configuration = RuntimeConfiguration(
            runtimeRoot: runtimeRoot,
            codexExecutable: "codex",
            webexBaseURL: URL(string: "https://webexapis.com/v1")!,
            webexPageSize: 100,
            webexRetryCount: 0,
            webexTimeoutSeconds: 1,
            webexOAuthTokenPathOverride: nil,
            webexOAuthRefreshSkewSeconds: 300,
            webexOAuthRefreshTokenSkewSeconds: 86_400
        )
        let knowledgeStore = KnowledgeStore(configuration: configuration)
        try knowledgeStore.bootstrap()
        let roomID = "Y2lzY29zcGFyazovL3VzL1JPT00vQU5JTA"
        let formatter = ISO8601DateFormatter()
        formatter.formatOptions = [.withInternetDateTime, .withFractionalSeconds]
        let webexDate = Date().addingTimeInterval(-3_600)
        let iMessageDate = Date().addingTimeInterval(-3_500)
        let updatedDate = Date().addingTimeInterval(-3_400)
        let roomUpdatedAt = formatter.string(from: webexDate.addingTimeInterval(-600))
        let webexCreatedAt = formatter.string(from: webexDate)
        let iMessageCreatedAt = formatter.string(from: iMessageDate)
        let cacheUpdatedAt = formatter.string(from: updatedDate)
        try knowledgeStore.upsertRoom(RoomRecord(id: roomID, title: "Anil Direct", updatedAt: roomUpdatedAt))
        let person = PersonRecord(id: "person-anil", displayName: "Anil Nair", email: "anil@cisco.com", updatedAt: roomUpdatedAt)
        try knowledgeStore.upsertPerson(person)
        try knowledgeStore.upsertMessage(
            MessageRecord(
                id: "webex-1",
                roomID: roomID,
                personID: "person-anil",
                body: "Webex message from Anil.",
                createdAt: webexCreatedAt,
                updatedAt: webexCreatedAt
            )
        )

        let iMessageService = StubIMessageIngestionService(messages: [
            IMessageTimelineMessage(
                id: "imessage-1",
                threadID: "chat:1",
                threadTitle: "iMessage - Anil Nair",
                handle: "+14085550123",
                sender: "Me",
                body: "Outbound iMessage reply.",
                createdAt: iMessageCreatedAt,
                sortDate: iMessageDate,
                isFromMe: true
            )
        ])
        let service = NativeWebexIngestionService(
            configuration: configuration,
            knowledgeStore: knowledgeStore,
            iMessageService: iMessageService
        )
        let target = ConfigTarget(
            kind: .person,
            label: "Anil Nair",
            roomID: roomID,
            roomType: "direct",
            email: "anil@cisco.com",
            iMessageHandles: ["+14085550123"]
        )

        let cache = try service.makePersonFocusCache(
            targets: [target],
            trackedRoomIDs: [roomID],
            roomTitlesByID: [roomID: "Anil Direct"],
            peopleByID: ["person-anil": person],
            syncStatesByRoomID: [:],
            updatedAt: cacheUpdatedAt,
            focusDays: 2
        )

        let item = try XCTUnwrap(cache.items.first)
        let detail = item.detailLines.joined(separator: "\n")
        XCTAssertEqual(item.meta, "auto-reply=no | messages=2")
        XCTAssertEqual(item.subtitle, "Outbound iMessage reply.")
        XCTAssertEqual(item.statusBadge, "webex+imessage")
        XCTAssertTrue(detail.contains("Webex webex-1: \(webexCreatedAt) | Anil Nair | Anil Direct | Webex message from Anil."))
        XCTAssertTrue(detail.contains("iMessage imessage-1: \(iMessageCreatedAt) | Me | iMessage - Anil Nair | Outbound iMessage reply."))
    }

    func testPersonFocusCacheIncludesEmailOnlySubmittedTranscriptMessages() throws {
        let runtimeRoot = FileManager.default.temporaryDirectory
            .appendingPathComponent("CubiclePersonTranscriptTimelineTests-\(UUID().uuidString)", isDirectory: true)
        defer { try? FileManager.default.removeItem(at: runtimeRoot) }
        let configuration = testRuntimeConfiguration(runtimeRoot: runtimeRoot)
        let knowledgeStore = KnowledgeStore(configuration: configuration)
        try knowledgeStore.bootstrap()
        let formatter = ISO8601DateFormatter()
        formatter.formatOptions = [.withInternetDateTime, .withFractionalSeconds]
        let createdAt = formatter.string(from: Date().addingTimeInterval(-600))
        let person = PersonRecord(
            id: "prabhat@example.com",
            displayName: "Prabhat Singh",
            email: "prabhat@example.com",
            updatedAt: createdAt
        )
        let roomID = "cubicle-person-prabhat-meeting-transcripts"
        try knowledgeStore.upsertPerson(person)
        try knowledgeStore.upsertRoom(RoomRecord(id: roomID, title: "Prabhat Singh", updatedAt: createdAt))
        try knowledgeStore.upsertMessage(
            MessageRecord(
                id: "meetinging-transcript:person:test",
                roomID: roomID,
                personID: person.id,
                body: "Annotation: meetinging-transcript\nTranscript:\n[0.0s] Speaker 1: Roadmap checkpoint.",
                createdAt: createdAt,
                updatedAt: createdAt
            )
        )

        let submittedMessages = try knowledgeStore.loadMessages(personID: person.id)
        XCTAssertEqual(submittedMessages.map(\.id), ["meetinging-transcript:person:test"])

        let service = NativeWebexIngestionService(
            configuration: configuration,
            knowledgeStore: knowledgeStore,
            iMessageService: StubIMessageIngestionService(messages: [])
        )
        let target = ConfigTarget(
            kind: .person,
            label: "Prabhat Singh",
            roomID: "",
            roomType: "",
            email: person.email
        )

        let cache = try service.makePersonFocusCache(
            targets: [target],
            trackedRoomIDs: [],
            roomTitlesByID: [roomID: "Prabhat Singh"],
            peopleByID: [person.id: person],
            syncStatesByRoomID: [:],
            updatedAt: createdAt,
            focusDays: 2
        )

        let item = try XCTUnwrap(cache.items.first)
        let detail = item.detailLines.joined(separator: "\n")
        XCTAssertEqual(item.meta, "auto-reply=no | messages=1")
        XCTAssertEqual(item.statusBadge, "email-match")
        XCTAssertTrue(detail.contains("Webex meetinging-transcript:person:test: \(createdAt) | Prabhat Singh | Prabhat Singh | Annotation: meetinging-transcript"))
    }

    private static let spaceItem = FocusItem(
        id: "spacefocus:test-space",
        title: "Incident Response Room",
        subtitle: "Latest Webex outage update",
        meta: "auto-reply=no | messages=3",
        timestamp: "2026-05-13 22:13 PDT",
        badge: "space",
        statusBadge: "live-webex",
        detailLines: [
            "Space Name: Incident Response Room",
            "Recent conversations (last 60 days):",
            "Date range: 2026-05-13 22:13 PDT -> 2026-05-13 20:00 PDT",
            "Space: Incident Response Room",
            "Space message 1: 2026-05-13 20:00 PDT | Alice | Incident Response Room | Can we name the incident owner and customer communication checkpoint?",
            "Space message 2: 2026-05-13 20:30 PDT | Bob | Incident Response Room | The owner is unclear and customer communication is delayed by missing approval.",
            "Space message 3: 2026-05-13 22:13 PDT | Alice | Incident Response Room | We still need the exact deadline and escalation backup."
        ],
        detailIntroLines: [],
        detailSections: [],
        detailTailLines: []
    )

    private static let personItem = FocusItem(
        id: "personfocus:test-person",
        title: "Raj Chopra",
        subtitle: "iMessage thread about AI Gateway",
        meta: "messages=4",
        timestamp: "2026-05-13 18:43 PDT",
        badge: "person",
        statusBadge: "live-webex",
        detailLines: [
            "Recent topic clusters (last 60 days):",
            "Date range: 2026-05-13 18:43 PDT -> 2026-05-13 18:00 PDT",
            "Channels: iMessage — Raj Chopra",
            "Context 1: 2026-05-13 18:00 PDT | Raj Chopra | iMessage — Raj Chopra | Can we clarify the AI gateway wedge before the staff update?",
            "Anchor 2: 2026-05-13 18:10 PDT | Prabhat | iMessage — Raj Chopra | The product boundary is still unclear between proxy, gateway, and identity.",
            "Anchor 3: 2026-05-13 18:20 PDT | Raj Chopra | iMessage — Raj Chopra | Who owns the decision and what is the next milestone?",
            "Context 4: 2026-05-13 18:43 PDT | Prabhat | iMessage — Raj Chopra | We need one owner and one date before the review."
        ],
        detailIntroLines: [],
        detailSections: [],
        detailTailLines: []
    )
}

private func executeSQLite(
    _ sql: String,
    databaseURL: URL,
    file: StaticString = #filePath,
    line: UInt = #line
) throws {
    var db: OpaquePointer?
    XCTAssertEqual(sqlite3_open(databaseURL.path, &db), SQLITE_OK, file: file, line: line)
    defer { sqlite3_close(db) }
    var errorMessage: UnsafeMutablePointer<CChar>?
    let result = sqlite3_exec(db, sql, nil, nil, &errorMessage)
    if result != SQLITE_OK {
        let message = errorMessage.map { String(cString: $0) } ?? "unknown SQLite error"
        sqlite3_free(errorMessage)
        XCTFail(message, file: file, line: line)
    }
}

private func querySQLiteText(
    _ sql: String,
    databaseURL: URL,
    file: StaticString = #filePath,
    line: UInt = #line
) throws -> String? {
    var db: OpaquePointer?
    XCTAssertEqual(sqlite3_open(databaseURL.path, &db), SQLITE_OK, file: file, line: line)
    defer { sqlite3_close(db) }
    var statement: OpaquePointer?
    XCTAssertEqual(sqlite3_prepare_v2(db, sql, -1, &statement, nil), SQLITE_OK, file: file, line: line)
    defer { sqlite3_finalize(statement) }
    guard sqlite3_step(statement) == SQLITE_ROW else {
        return nil
    }
    guard let text = sqlite3_column_text(statement, 0) else {
        return nil
    }
    return String(cString: text)
}

private func testRuntimeConfiguration(
    runtimeRoot: URL,
    codexExecutable: String = "codex"
) -> RuntimeConfiguration {
    RuntimeConfiguration(
        runtimeRoot: runtimeRoot,
        codexExecutable: codexExecutable,
        webexBaseURL: URL(string: "https://webexapis.com/v1")!,
        webexPageSize: 100,
        webexRetryCount: 0,
        webexTimeoutSeconds: 1,
        webexOAuthTokenPathOverride: nil,
        webexOAuthRefreshSkewSeconds: 300,
        webexOAuthRefreshTokenSkewSeconds: 86_400
    )
}

private func makeFocusCache(
    kind: FocusKind,
    days: Int,
    itemID: String,
    title: String
) -> FocusCache {
    let roomTitle = kind == .space ? title : "Direct with \(title)"
    let linePrefix = kind == .space ? "Webex message-new" : "Webex direct-message"
    return FocusCache(
        focusDays: days,
        items: [
            FocusItem(
                id: itemID,
                title: title,
                subtitle: "Fresh raw evidence",
                meta: "messages=1",
                timestamp: "2026-05-16T20:00:00Z",
                badge: kind.rawValue,
                statusBadge: "live-webex",
                detailLines: [
                    kind == .space ? "Space Name: \(title)" : "Person Name: \(title)",
                    "Recent conversations (last \(days) days):",
                    "\(linePrefix): 2026-05-16T20:00:00Z | Sender Example | \(roomTitle) | Raw message for \(title)"
                ],
                detailIntroLines: [],
                detailSections: [],
                detailTailLines: []
            )
        ],
        updatedAt: "2026-05-16T20:00:00Z",
        countLabel: "1",
        recentMessages: 1,
        summaryGenerationInProgress: false,
        subjectsProcessed: 1,
        subjectsTotal: 1
    )
}

private func writeFocusCache(_ cache: FocusCache, to url: URL) throws {
    try FileManager.default.createDirectory(
        at: url.deletingLastPathComponent(),
        withIntermediateDirectories: true
    )
    let encoder = JSONEncoder()
    encoder.outputFormatting = [.prettyPrinted, .sortedKeys]
    try encoder.encode(cache).write(to: url, options: [.atomic])
}

private func decodeTranscriptionJSONObject(_ payload: String) throws -> [String: Any] {
    let data = try XCTUnwrap(payload.data(using: .utf8))
    return try XCTUnwrap(JSONSerialization.jsonObject(with: data) as? [String: Any])
}

private func makeTranscriptSegment(
    id: String,
    text: String,
    isFinal: Bool,
    startTimeMilliseconds: Int = 0,
    endTimeMilliseconds: Int? = 1_000,
    speakerID: String? = nil,
    languageMode: TranscriptionLanguageMode = .englishToEnglish
) -> TranscriptSegment {
    TranscriptSegment(
        id: id,
        startTimeMilliseconds: startTimeMilliseconds,
        endTimeMilliseconds: endTimeMilliseconds,
        text: text,
        isFinal: isFinal,
        speakerID: speakerID,
        languageMode: languageMode,
        modelName: "test-model",
        modelVersion: "test-version",
        confidence: nil,
        createdAt: Date(timeIntervalSince1970: 0)
    )
}

private actor LocalMockTranscriptionWebSocketServer {
    struct Snapshot {
        var connectedURLs: [URL]
        var connectionHeaders: [[String: String]]
        var receivedMessages: [TranscriptionWebSocketMessage]
        var isClosed: Bool
    }

    private var scriptedMessages: [TranscriptionWebSocketMessage]
    private var connectedURLs: [URL] = []
    private var connectionHeaders: [[String: String]] = []
    private var receivedMessages: [TranscriptionWebSocketMessage] = []
    private var isClosed = false

    init(scriptedMessages: [TranscriptionWebSocketMessage]) {
        self.scriptedMessages = scriptedMessages
    }

    func connect(to url: URL, headers: [String: String]) {
        connectedURLs.append(url)
        connectionHeaders.append(headers)
        isClosed = false
    }

    func send(_ message: TranscriptionWebSocketMessage) {
        receivedMessages.append(message)
    }

    func receive() throws -> TranscriptionWebSocketMessage {
        guard !scriptedMessages.isEmpty else {
            throw TranscriptionProtocolError.notConnected
        }
        return scriptedMessages.removeFirst()
    }

    func close() {
        isClosed = true
    }

    func snapshot() -> Snapshot {
        Snapshot(
            connectedURLs: connectedURLs,
            connectionHeaders: connectionHeaders,
            receivedMessages: receivedMessages,
            isClosed: isClosed
        )
    }
}

private final class LocalMockTranscriptionWebSocketTransport: TranscriptionWebSocketTransport {
    private let server: LocalMockTranscriptionWebSocketServer

    init(server: LocalMockTranscriptionWebSocketServer) {
        self.server = server
    }

    func connect(to url: URL, headers: [String: String]) async throws {
        await server.connect(to: url, headers: headers)
    }

    func send(_ message: TranscriptionWebSocketMessage) async throws {
        await server.send(message)
    }

    func receive() async throws -> TranscriptionWebSocketMessage {
        try await server.receive()
    }

    func close() async {
        await server.close()
    }
}

private final class HoldingTranscriptionClient: TranscriptionClient {
    private(set) var startCallCount = 0
    private(set) var stopCallCount = 0
    private(set) var lastConfig: TranscriptionSessionConfig?
    private var continuation: AsyncStream<TranscriptionServerEvent>.Continuation?

    func startSession(config: TranscriptionSessionConfig) async throws -> AsyncStream<TranscriptionServerEvent> {
        startCallCount += 1
        lastConfig = config
        return AsyncStream { continuation in
            self.continuation = continuation
            continuation.yield(.sessionStarted(sessionID: config.sessionID))
        }
    }

    func stopSession() async {
        stopCallCount += 1
        continuation?.yield(.sessionStopped)
        continuation?.finish()
        continuation = nil
    }
}

private final class StopFinalizingTranscriptionClient: TranscriptionClient, TranscriptionAudioStreamingClient {
    private var continuation: AsyncStream<TranscriptionServerEvent>.Continuation?

    func startSession(config: TranscriptionSessionConfig) async throws -> AsyncStream<TranscriptionServerEvent> {
        AsyncStream { continuation in
            self.continuation = continuation
            continuation.yield(.sessionStarted(sessionID: config.sessionID))
            continuation.yield(
                .partialTranscript(
                    makeTranscriptSegment(
                        id: "server-segment-1",
                        text: "draft text",
                        isFinal: false,
                        startTimeMilliseconds: 0,
                        endTimeMilliseconds: nil
                    )
                )
            )
        }
    }

    func sendAudioChunk(_ data: Data) async throws {
        _ = data
    }

    func stopSession() async {
        continuation?.yield(
            .finalTranscript(
                makeTranscriptSegment(
                    id: "server-segment-1",
                    text: "final text",
                    isFinal: true,
                    startTimeMilliseconds: 0,
                    endTimeMilliseconds: 1_200
                )
            )
        )
        continuation?.yield(.speakerUpdate(segmentID: "server-segment-1", speakerID: "2"))
        continuation?.yield(.sessionStopped)
        continuation?.finish()
        continuation = nil
    }
}

private final class DelayedStopFinalizingTranscriptionClient: TranscriptionClient, TranscriptionAudioStreamingClient {
    private let stopDelayNanoseconds: UInt64
    private var continuation: AsyncStream<TranscriptionServerEvent>.Continuation?

    init(stopDelayNanoseconds: UInt64) {
        self.stopDelayNanoseconds = stopDelayNanoseconds
    }

    func startSession(config: TranscriptionSessionConfig) async throws -> AsyncStream<TranscriptionServerEvent> {
        AsyncStream { continuation in
            self.continuation = continuation
            continuation.yield(.sessionStarted(sessionID: config.sessionID))
            continuation.yield(
                .partialTranscript(
                    makeTranscriptSegment(
                        id: "server-segment-1",
                        text: "draft text",
                        isFinal: false,
                        startTimeMilliseconds: 0,
                        endTimeMilliseconds: nil
                    )
                )
            )
        }
    }

    func sendAudioChunk(_ data: Data) async throws {
        _ = data
    }

    func stopSession() async {
        try? await Task.sleep(nanoseconds: stopDelayNanoseconds)
        continuation?.yield(
            .finalTranscript(
                makeTranscriptSegment(
                    id: "server-segment-1",
                    text: "delayed final text",
                    isFinal: true,
                    startTimeMilliseconds: 0,
                    endTimeMilliseconds: 1_200
                )
            )
        )
        continuation?.yield(.speakerUpdate(segmentID: "server-segment-1", speakerID: "3"))
        continuation?.yield(.sessionStopped)
        continuation?.finish()
        continuation = nil
    }
}

private final class RestartingTranscriptionClient: TranscriptionClient {
    private(set) var startCallCount = 0
    private(set) var stopCallCount = 0
    private var continuation: AsyncStream<TranscriptionServerEvent>.Continuation?

    func startSession(config: TranscriptionSessionConfig) async throws -> AsyncStream<TranscriptionServerEvent> {
        startCallCount += 1
        if startCallCount == 1 {
            return AsyncStream { continuation in
                continuation.yield(.sessionStarted(sessionID: config.sessionID))
                continuation.finish()
            }
        }
        return AsyncStream { continuation in
            self.continuation = continuation
            continuation.yield(.sessionStarted(sessionID: config.sessionID))
        }
    }

    func stopSession() async {
        stopCallCount += 1
        continuation?.yield(.sessionStopped)
        continuation?.finish()
        continuation = nil
    }
}

private final class StreamingTranscriptionClient: TranscriptionClient, TranscriptionAudioStreamingClient {
    private(set) var audioChunks: [Data] = []

    func startSession(config: TranscriptionSessionConfig) async throws -> AsyncStream<TranscriptionServerEvent> {
        AsyncStream { continuation in
            continuation.yield(.sessionStarted(sessionID: config.sessionID))
            continuation.finish()
        }
    }

    func sendAudioChunk(_ data: Data) async throws {
        audioChunks.append(data)
    }

    func stopSession() async {}
}

private final class SingleChunkAudioCaptureService: AudioCaptureService {
    private let chunk: Data
    private(set) var startCallCount = 0
    private(set) var stopCallCount = 0
    private(set) var lastMicrophoneGainMultiplier: Double?

    init(chunk: Data) {
        self.chunk = chunk
    }

    func startCapture(
        config: TranscriptionSessionConfig,
        microphoneGainMultiplier: Double,
        onAudioChunk: @escaping @Sendable (Data, AudioCaptureTelemetry) async -> Void
    ) async throws {
        startCallCount += 1
        lastMicrophoneGainMultiplier = microphoneGainMultiplier
        await onAudioChunk(chunk, .zero)
    }

    func updateMicrophoneGainMultiplier(_ multiplier: Double) {
        lastMicrophoneGainMultiplier = multiplier
    }

    func stopCapture() async {
        stopCallCount += 1
    }
}

private final class StubIMessageIngestionService: NativeIMessageIngesting {
    private let messages: [IMessageTimelineMessage]

    init(messages: [IMessageTimelineMessage]) {
        self.messages = messages
    }

    func loadMessages(
        matching handles: [String],
        displayName: String,
        since: Date,
        limit: Int
    ) throws -> [IMessageTimelineMessage] {
        Array(messages.filter { $0.sortDate >= since }.prefix(limit))
    }
}

private final class StubQuestionSynthesizer: QuestionCandidateSynthesizing {
    private(set) var receivedCandidateCount = 0

    func synthesizeQuestionCandidates(
        from candidates: [QuestionCandidate],
        now: Date
    ) async throws -> [QuestionCandidate] {
        receivedCandidateCount = candidates.count
        guard let seed = candidates.first else {
            return []
        }

        let good = QuestionCandidate(
            id: "question-codex-synth-good",
            scopeType: seed.scopeType,
            scopeKey: seed.scopeKey,
            scopeLabel: seed.scopeLabel,
            questionText: "Which customer approval follow-up needs an owner before the next update?",
            questionType: "codex_synthesized_question",
            whyNow: "Recent evidence mentions missing approval, customer communication, and owner ambiguity.",
            evidence: seed.evidence,
            sourceKind: "codex_question_synthesis",
            sourceKey: "\(seed.scopeKey):stub",
            tags: ["codex", "synthesis", "owner"],
            priorityScore: 125,
            status: .candidate,
            answerSnapshotId: nil,
            createdAt: now,
            updatedAt: now,
            expiresAt: Calendar.current.date(byAdding: .day, value: 14, to: now)
        )
        let bad = QuestionCandidate(
            id: "question-codex-synth-bad",
            scopeType: seed.scopeType,
            scopeKey: seed.scopeKey,
            scopeLabel: seed.scopeLabel,
            questionText: "How do high-question vs low-question threads differ on duration seconds?",
            questionType: "codex_synthesized_question",
            whyNow: "The cohort has duration_seconds and sample_size metrics.",
            evidence: seed.evidence,
            sourceKind: "codex_question_synthesis",
            sourceKey: "\(seed.scopeKey):stub-bad",
            tags: ["codex", "cohort"],
            priorityScore: 130,
            status: .candidate,
            answerSnapshotId: nil,
            createdAt: now,
            updatedAt: now,
            expiresAt: Calendar.current.date(byAdding: .day, value: 14, to: now)
        )
        return [bad, good]
    }
}

private final class EmptyQuestionSynthesizer: QuestionCandidateSynthesizing {
    func synthesizeQuestionCandidates(
        from candidates: [QuestionCandidate],
        now: Date
    ) async throws -> [QuestionCandidate] {
        []
    }
}
