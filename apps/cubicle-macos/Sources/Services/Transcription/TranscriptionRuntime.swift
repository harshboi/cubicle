import AVFoundation
import Combine
import Foundation

protocol TranscriptionClient: AnyObject {
    func startSession(config: TranscriptionSessionConfig) async throws -> AsyncStream<TranscriptionServerEvent>
    func stopSession() async
}

struct AudioCaptureTelemetry: Equatable, Sendable {
    var preGainPeakLevel: Double
    var preGainRMSLevel: Double
    var postGainPeakLevel: Double
    var postGainRMSLevel: Double
    var appliedGain: Double
    var clippedSampleCount: Int
    var sampleCount: Int

    static let zero = AudioCaptureTelemetry(
        preGainPeakLevel: 0,
        preGainRMSLevel: 0,
        postGainPeakLevel: 0,
        postGainRMSLevel: 0,
        appliedGain: 1,
        clippedSampleCount: 0,
        sampleCount: 0
    )
}

protocol AudioCaptureService: AnyObject {
    func startCapture(
        config: TranscriptionSessionConfig,
        microphoneGainMultiplier: Double,
        onAudioChunk: @escaping @Sendable (Data, AudioCaptureTelemetry) async -> Void
    ) async throws
    func updateMicrophoneGainMultiplier(_ multiplier: Double)
    func stopCapture() async
}

final class NoopAudioCaptureService: AudioCaptureService {
    private(set) var startCallCount = 0
    private(set) var stopCallCount = 0
    private(set) var lastConfig: TranscriptionSessionConfig?
    private(set) var lastMicrophoneGainMultiplier: Double?

    func startCapture(
        config: TranscriptionSessionConfig,
        microphoneGainMultiplier: Double,
        onAudioChunk: @escaping @Sendable (Data, AudioCaptureTelemetry) async -> Void
    ) async throws {
        startCallCount += 1
        lastConfig = config
        lastMicrophoneGainMultiplier = microphoneGainMultiplier
    }

    func updateMicrophoneGainMultiplier(_ multiplier: Double) {
        lastMicrophoneGainMultiplier = multiplier
    }

    func stopCapture() async {
        stopCallCount += 1
    }
}

enum AudioCaptureServiceError: LocalizedError {
    case unsupportedAudioContract
    case microphonePermissionDenied
    case inputFormatUnavailable
    case converterUnavailable

    var errorDescription: String? {
        switch self {
        case .unsupportedAudioContract:
            return "Microphone capture requires 16 kHz mono pcm_s16le audio."
        case .microphonePermissionDenied:
            return "Microphone permission is required for live transcription."
        case .inputFormatUnavailable:
            return "No microphone input format is available."
        case .converterUnavailable:
            return "Could not create a microphone audio converter."
        }
    }
}

final class MicrophoneAudioCaptureService: AudioCaptureService {
    private let engine = AVAudioEngine()
    private let inputBus: AVAudioNodeBus = 0
    private let conversionQueue = DispatchQueue(label: "cubicle.transcription.microphone-conversion")
    private let maximumAllowedMicrophoneGain: Double
    private let targetPeakSample: Double
    private var microphoneGainMultiplier: Double
    private var converter: AVAudioConverter?
    private var outputFormat: AVAudioFormat?

    init(maximumMicrophoneGain: Double = 18.0, targetPeakSample: Double = 28_000) {
        self.maximumAllowedMicrophoneGain = 32.0
        self.microphoneGainMultiplier = min(max(maximumMicrophoneGain, 1.0), maximumAllowedMicrophoneGain)
        self.targetPeakSample = min(max(targetPeakSample, 4_000), Double(Int16.max))
    }

    func startCapture(
        config: TranscriptionSessionConfig,
        microphoneGainMultiplier: Double,
        onAudioChunk: @escaping @Sendable (Data, AudioCaptureTelemetry) async -> Void
    ) async throws {
        guard config.sampleRate == 16_000,
              config.channelCount == 1,
              config.audioEncoding == "pcm_s16le" else {
            throw AudioCaptureServiceError.unsupportedAudioContract
        }

        guard await requestMicrophoneAccess() else {
            throw AudioCaptureServiceError.microphonePermissionDenied
        }

        await stopCapture()

        let inputNode = engine.inputNode
        let inputFormat = inputNode.outputFormat(forBus: inputBus)
        guard inputFormat.sampleRate > 0, inputFormat.channelCount > 0 else {
            throw AudioCaptureServiceError.inputFormatUnavailable
        }
        guard let outputFormat = AVAudioFormat(
            commonFormat: .pcmFormatInt16,
            sampleRate: Double(config.sampleRate),
            channels: AVAudioChannelCount(config.channelCount),
            interleaved: true
        ) else {
            throw AudioCaptureServiceError.inputFormatUnavailable
        }
        guard let converter = AVAudioConverter(from: inputFormat, to: outputFormat) else {
            throw AudioCaptureServiceError.converterUnavailable
        }

        conversionQueue.sync {
            self.microphoneGainMultiplier = clampedMicrophoneGain(microphoneGainMultiplier)
            self.converter = converter
            self.outputFormat = outputFormat
        }

        inputNode.removeTap(onBus: inputBus)
        inputNode.installTap(onBus: inputBus, bufferSize: 4_096, format: inputFormat) { [weak self] buffer, _ in
            guard let chunk = self?.convertToPCM16(buffer), !chunk.data.isEmpty else {
                return
            }
            Task.detached(priority: .utility) {
                await onAudioChunk(chunk.data, chunk.telemetry)
            }
        }

        engine.prepare()
        try engine.start()
    }

    func updateMicrophoneGainMultiplier(_ multiplier: Double) {
        conversionQueue.sync {
            microphoneGainMultiplier = clampedMicrophoneGain(multiplier)
        }
    }

    func stopCapture() async {
        if engine.isRunning {
            engine.stop()
        }
        engine.inputNode.removeTap(onBus: inputBus)
        conversionQueue.sync {
            converter = nil
            outputFormat = nil
        }
    }

    private func clampedMicrophoneGain(_ multiplier: Double) -> Double {
        guard multiplier.isFinite else {
            return 1.0
        }
        return min(max(multiplier, 1.0), maximumAllowedMicrophoneGain)
    }

    private func requestMicrophoneAccess() async -> Bool {
        switch AVCaptureDevice.authorizationStatus(for: .audio) {
        case .authorized:
            return true
        case .notDetermined:
            return await withCheckedContinuation { continuation in
                AVCaptureDevice.requestAccess(for: .audio) { granted in
                    continuation.resume(returning: granted)
                }
            }
        case .denied, .restricted:
            return false
        @unknown default:
            return false
        }
    }

    private func convertToPCM16(_ inputBuffer: AVAudioPCMBuffer) -> CapturedAudioChunk? {
        conversionQueue.sync {
            guard let converter, let outputFormat else {
                return nil
            }
            let frameRatio = outputFormat.sampleRate / inputBuffer.format.sampleRate
            let frameCapacity = AVAudioFrameCount(max(1, ceil(Double(inputBuffer.frameLength) * frameRatio) + 256))
            guard let outputBuffer = AVAudioPCMBuffer(pcmFormat: outputFormat, frameCapacity: frameCapacity) else {
                return nil
            }

            var didProvideInput = false
            var conversionError: NSError?
            let status = converter.convert(to: outputBuffer, error: &conversionError) { _, outStatus in
                if didProvideInput {
                    outStatus.pointee = .noDataNow
                    return nil
                }
                didProvideInput = true
                outStatus.pointee = .haveData
                return inputBuffer
            }
            guard conversionError == nil, status != .error, outputBuffer.frameLength > 0 else {
                return nil
            }

            let audioBuffer = outputBuffer.audioBufferList.pointee.mBuffers
            guard let bytes = audioBuffer.mData, audioBuffer.mDataByteSize > 0 else {
                return nil
            }
            let data = Data(bytes: bytes, count: Int(audioBuffer.mDataByteSize))
            return applyAdaptiveGain(to: data)
        }
    }

    private func applyAdaptiveGain(to data: Data) -> CapturedAudioChunk {
        let gainCeiling = microphoneGainMultiplier
        let preGainLevels = Self.audioLevels(in: data)
        guard gainCeiling > 1.0, data.count >= 2 else {
            return CapturedAudioChunk(
                data: data,
                telemetry: AudioCaptureTelemetry(
                    preGainPeakLevel: preGainLevels.peakLevel,
                    preGainRMSLevel: preGainLevels.rmsLevel,
                    postGainPeakLevel: preGainLevels.peakLevel,
                    postGainRMSLevel: preGainLevels.rmsLevel,
                    appliedGain: 1,
                    clippedSampleCount: 0,
                    sampleCount: preGainLevels.sampleCount
                )
            )
        }
        var boosted = data
        var appliedGain = 1.0
        var clippedSampleCount = 0
        boosted.withUnsafeMutableBytes { rawBuffer in
            guard let bytes = rawBuffer.baseAddress?.assumingMemoryBound(to: UInt8.self) else {
                return
            }
            let sampleCount = rawBuffer.count / 2
            guard preGainLevels.peakSample > 0 else {
                return
            }
            let gain = min(gainCeiling, max(1.0, targetPeakSample / Double(preGainLevels.peakSample)))
            appliedGain = gain
            for index in 0..<sampleCount {
                let byteIndex = index * 2
                let rawSample = UInt16(bytes[byteIndex]) | (UInt16(bytes[byteIndex + 1]) << 8)
                let sample = Int16(bitPattern: rawSample)
                let amplified = Int((Double(sample) * gain).rounded())
                let clipped = min(max(amplified, Int(Int16.min)), Int(Int16.max))
                if clipped != amplified {
                    clippedSampleCount += 1
                }
                let output = UInt16(bitPattern: Int16(clipped))
                bytes[byteIndex] = UInt8(output & 0xff)
                bytes[byteIndex + 1] = UInt8((output >> 8) & 0xff)
            }
        }
        let postGainLevels = Self.audioLevels(in: boosted)
        return CapturedAudioChunk(
            data: boosted,
            telemetry: AudioCaptureTelemetry(
                preGainPeakLevel: preGainLevels.peakLevel,
                preGainRMSLevel: preGainLevels.rmsLevel,
                postGainPeakLevel: postGainLevels.peakLevel,
                postGainRMSLevel: postGainLevels.rmsLevel,
                appliedGain: appliedGain,
                clippedSampleCount: clippedSampleCount,
                sampleCount: preGainLevels.sampleCount
            )
        )
    }

    private static func audioLevels(in data: Data) -> AudioLevels {
        guard data.count >= 2 else {
            return AudioLevels(peakSample: 0, peakLevel: 0, rmsLevel: 0, sampleCount: 0)
        }
        return data.withUnsafeBytes { rawBuffer in
            guard let bytes = rawBuffer.baseAddress?.assumingMemoryBound(to: UInt8.self) else {
                return AudioLevels(peakSample: 0, peakLevel: 0, rmsLevel: 0, sampleCount: 0)
            }
            let sampleCount = rawBuffer.count / 2
            var peakSample = 0
            var sumSquares = 0.0
            for index in 0..<sampleCount {
                let byteIndex = index * 2
                let rawSample = UInt16(bytes[byteIndex]) | (UInt16(bytes[byteIndex + 1]) << 8)
                let sample = Int(Int16(bitPattern: rawSample))
                peakSample = max(peakSample, abs(sample))
                let normalized = Double(sample) / Double(Int16.max)
                sumSquares += normalized * normalized
            }
            let rmsLevel = sampleCount > 0 ? sqrt(sumSquares / Double(sampleCount)) : 0
            return AudioLevels(
                peakSample: peakSample,
                peakLevel: min(1.0, Double(peakSample) / Double(Int16.max)),
                rmsLevel: min(1.0, rmsLevel),
                sampleCount: sampleCount
            )
        }
    }
}

private struct CapturedAudioChunk {
    var data: Data
    var telemetry: AudioCaptureTelemetry
}

private struct AudioLevels {
    var peakSample: Int
    var peakLevel: Double
    var rmsLevel: Double
    var sampleCount: Int
}

final class MockTranscriptionClient: TranscriptionClient {
    private(set) var startCallCount = 0
    private(set) var stopCallCount = 0
    private(set) var lastConfig: TranscriptionSessionConfig?

    var scriptedEvents: [TranscriptionServerEvent]

    init(scriptedEvents: [TranscriptionServerEvent] = MockTranscriptionClient.defaultEvents()) {
        self.scriptedEvents = scriptedEvents
    }

    func startSession(config: TranscriptionSessionConfig) async throws -> AsyncStream<TranscriptionServerEvent> {
        startCallCount += 1
        lastConfig = config
        let events = scriptedEvents
        return AsyncStream { continuation in
            continuation.yield(.sessionStarted(sessionID: config.sessionID))
            for event in events {
                continuation.yield(event)
            }
            continuation.finish()
        }
    }

    func stopSession() async {
        stopCallCount += 1
    }

    static func defaultEvents(now: Date = Date()) -> [TranscriptionServerEvent] {
        [
            .partialTranscript(
                TranscriptSegment(
                    id: "mock-segment-1",
                    startTimeMilliseconds: 0,
                    endTimeMilliseconds: nil,
                    text: "We should",
                    isFinal: false,
                    speakerID: "1",
                    languageMode: .englishToEnglish,
                    modelName: "mock-asr",
                    modelVersion: "slice-1",
                    confidence: nil,
                    createdAt: now
                )
            ),
            .partialTranscript(
                TranscriptSegment(
                    id: "mock-segment-1",
                    startTimeMilliseconds: 0,
                    endTimeMilliseconds: nil,
                    text: "We should close the launch blocker today.",
                    isFinal: false,
                    speakerID: "1",
                    languageMode: .englishToEnglish,
                    modelName: "mock-asr",
                    modelVersion: "slice-1",
                    confidence: nil,
                    createdAt: now
                )
            ),
            .finalTranscript(
                TranscriptSegment(
                    id: "mock-segment-1",
                    startTimeMilliseconds: 0,
                    endTimeMilliseconds: 2_400,
                    text: "We should close the launch blocker today.",
                    isFinal: true,
                    speakerID: "1",
                    languageMode: .englishToEnglish,
                    modelName: "mock-asr",
                    modelVersion: "slice-1",
                    confidence: 0.94,
                    createdAt: now
                )
            ),
            .partialTranscript(
                TranscriptSegment(
                    id: "mock-segment-2",
                    startTimeMilliseconds: 2_600,
                    endTimeMilliseconds: nil,
                    text: "I can take the follow-up.",
                    isFinal: false,
                    speakerID: "2",
                    languageMode: .englishToEnglish,
                    modelName: "mock-asr",
                    modelVersion: "slice-1",
                    confidence: nil,
                    createdAt: now
                )
            )
        ]
    }
}

@MainActor
final class TranscriptionViewModel: ObservableObject {
    @Published private(set) var status: TranscriptionConnectionStatus = .disabled
    @Published private(set) var aggregator = TranscriptAggregator()
    @Published private(set) var currentConfig: TranscriptionSessionConfig?
    @Published private(set) var lastDiarizationStatus: String?
    @Published private(set) var lastError: String?
    @Published private(set) var isStoppingSession = false
    @Published private(set) var audioChunksSent = 0
    @Published private(set) var audioBytesSent = 0
    @Published private(set) var lastAudioChunkSentAt: Date?
    @Published private(set) var lastAudioPeakLevel = 0.0
    @Published private(set) var lastAudioTelemetry = AudioCaptureTelemetry.zero
    @Published private(set) var audioCaptureWarning: String?

    private let client: TranscriptionClient
    private let audioCaptureService: AudioCaptureService
    private let authTokenLoader: (() async -> String?)?
    private let reconnectDelayNanoseconds: UInt64
    private let maxReconnectAttempts: Int
    private var settings = SystemSettings()
    private var eventTask: Task<Void, Never>?
    private var reconnectTask: Task<Void, Never>?
    private var audioCaptureWatchdogTask: Task<Void, Never>?
    private var reconnectAttempts = 0
    private var sawSessionStoppedEvent = false
    private var hasStartedSession = false

    init(
        client: TranscriptionClient = MockTranscriptionClient(),
        audioCaptureService: AudioCaptureService = NoopAudioCaptureService(),
        authTokenLoader: (() async -> String?)? = nil,
        reconnectDelayNanoseconds: UInt64 = 1_000_000_000,
        maxReconnectAttempts: Int = 3
    ) {
        self.client = client
        self.audioCaptureService = audioCaptureService
        self.authTokenLoader = authTokenLoader
        self.reconnectDelayNanoseconds = reconnectDelayNanoseconds
        self.maxReconnectAttempts = max(0, maxReconnectAttempts)
        apply(settings: settings)
    }

    var visibleSegments: [TranscriptSegment] {
        aggregator.visibleSegments
    }

    var transcriptSubmissionText: String {
        visibleSegments
            .map(Self.timelineLine)
            .filter { !$0.isEmpty }
            .joined(separator: "\n")
    }

    var hasTranscriptForSubmission: Bool {
        !transcriptSubmissionText.trimmingCharacters(in: .whitespacesAndNewlines).isEmpty
    }

    var sessionStateText: String {
        status.displayName
    }

    var sessionDetailText: String {
        if isStoppingSession {
            return "Capture stopped. Waiting briefly for final transcript and speaker labels."
        }
        if let lastError, !lastError.isEmpty {
            return lastError
        }
        if let audioCaptureWarning, !audioCaptureWarning.isEmpty {
            return audioCaptureWarning
        }
        return status.detailText
    }

    var audioStatusText: String? {
        guard hasStartedSession || audioChunksSent > 0 else {
            return nil
        }
        guard audioChunksSent > 0 else {
            return "Audio: waiting for microphone frames"
        }
        let kilobytes = max(1, audioBytesSent / 1_024)
        let inputRMSPercent = Int((lastAudioTelemetry.preGainRMSLevel * 100).rounded())
        let inputPeakPercent = Int((lastAudioTelemetry.preGainPeakLevel * 100).rounded())
        let outputRMSPercent = Int((lastAudioTelemetry.postGainRMSLevel * 100).rounded())
        let outputPeakPercent = Int((lastAudioTelemetry.postGainPeakLevel * 100).rounded())
        let gainText = Self.gainFormatter.string(from: NSNumber(value: lastAudioTelemetry.appliedGain)) ?? "\(lastAudioTelemetry.appliedGain)"
        return "Audio: \(audioChunksSent) frames, \(kilobytes) KB; in rms \(inputRMSPercent)% peak \(inputPeakPercent)%; out rms \(outputRMSPercent)% peak \(outputPeakPercent)%; gain \(gainText)x; clip \(lastAudioTelemetry.clippedSampleCount)"
    }

    func apply(settings: SystemSettings) {
        self.settings = settings
        let microphoneGainMultiplier = Double(settings.transcriptionMicrophoneGain)
        audioCaptureService.updateMicrophoneGainMultiplier(microphoneGainMultiplier)
        if settings.transcriptionEnabled {
            if status == .disabled {
                status = .stopped
            }
        } else {
            lastError = nil
            status = .disabled
            if hasStartedSession || eventTask != nil || currentConfig != nil {
                let stopTask = Task { [weak self] in
                    await self?.stopSession()
                }
                _ = stopTask
            }
        }
    }

    func startSessionForCurrentSettings() async {
        reconnectTask?.cancel()
        reconnectTask = nil
        reconnectAttempts = 0
        await startSessionForCurrentSettings(preservingTranscript: false)
    }

    private func startSessionForCurrentSettings(preservingTranscript: Bool) async {
        guard settings.transcriptionEnabled else {
            status = .disabled
            lastError = nil
            return
        }

        var sessionSettings = settings
        if sessionSettings.transcriptionAuthToken == nil,
           let loadedToken = await authTokenLoader?()?.trimmingCharacters(in: .whitespacesAndNewlines),
           !loadedToken.isEmpty {
            sessionSettings.transcriptionAuthToken = loadedToken
            settings = sessionSettings
        }
        let config = TranscriptionSessionConfig(settings: sessionSettings)
        guard !config.endpointURL.isEmpty else {
            status = .stopped
            lastError = "Set an AWS transcription endpoint before starting a session."
            return
        }

        eventTask?.cancel()
        if !preservingTranscript {
            aggregator.reset()
        }
        sawSessionStoppedEvent = false
        lastError = nil
        lastDiarizationStatus = nil
        resetAudioTelemetry()
        status = .connecting

        do {
            let stream = try await client.startSession(config: config)
            currentConfig = config
            hasStartedSession = true
            eventTask = Task { @MainActor [weak self] in
                for await event in stream {
                    self?.handle(event)
                }
                await self?.markStreamFinished()
            }
            let audioStreamingClient = client as? TranscriptionAudioStreamingClient
            try await audioCaptureService.startCapture(
                config: config,
                microphoneGainMultiplier: Double(sessionSettings.transcriptionMicrophoneGain)
            ) { [weak self, weak audioStreamingClient] audioChunk, telemetry in
                guard let audioStreamingClient else { return }
                do {
                    try await audioStreamingClient.sendAudioChunk(audioChunk)
                    await self?.recordAudioChunkSent(audioChunk, telemetry: telemetry)
                } catch {
                    await self?.markAudioStreamingFailed(error)
                }
            }
            status = .live
            scheduleAudioCaptureWatchdog()
        } catch {
            audioCaptureWatchdogTask?.cancel()
            audioCaptureWatchdogTask = nil
            await audioCaptureService.stopCapture()
            await client.stopSession()
            eventTask?.cancel()
            eventTask = nil
            hasStartedSession = false
            currentConfig = nil
            lastError = error.localizedDescription
            status = .failed(error.localizedDescription)
        }
    }

    func stopSession() async {
        guard !isStoppingSession else {
            return
        }
        reconnectTask?.cancel()
        reconnectTask = nil
        audioCaptureWatchdogTask?.cancel()
        audioCaptureWatchdogTask = nil
        reconnectAttempts = 0
        let shouldStopRuntime = hasStartedSession || eventTask != nil || currentConfig != nil
        isStoppingSession = true
        if shouldStopRuntime {
            await audioCaptureService.stopCapture()
            hasStartedSession = false
            currentConfig = nil
            status = settings.transcriptionEnabled ? .stopped : .disabled
            await client.stopSession()
            await waitForEventTaskToFinish(timeoutNanoseconds: 3_000_000_000)
        }
        eventTask?.cancel()
        eventTask = nil
        isStoppingSession = false
        hasStartedSession = false
        currentConfig = nil
        status = settings.transcriptionEnabled ? .stopped : .disabled
    }

    func simulateReconnectForTesting() {
        status = .reconnecting
    }

    private func handle(_ event: TranscriptionServerEvent) {
        switch event {
        case .sessionStarted(let sessionID):
            if var config = currentConfig {
                config.sessionID = sessionID
                currentConfig = config
            }
            status = .live
        case .partialTranscript,
             .finalTranscript,
             .speakerUpdate,
             .correctionUpdate:
            reconnectAttempts = 0
            audioCaptureWarning = nil
            aggregator.apply(event)
        case .diarizationStatus(let statusText):
            lastDiarizationStatus = statusText
        case .error(let message):
            lastError = message
            status = .failed(message)
        case .sessionStopped:
            sawSessionStoppedEvent = true
            status = settings.transcriptionEnabled ? .stopped : .disabled
        }
    }

    private func markStreamFinished() async {
        if isStoppingSession {
            return
        }
        let shouldTearDown = hasStartedSession || currentConfig != nil
        guard shouldTearDown else {
            return
        }
        let shouldReconnect = settings.transcriptionEnabled
            && !sawSessionStoppedEvent
            && reconnectAttempts < maxReconnectAttempts
        await tearDownRuntime()
        if shouldReconnect {
            reconnectAttempts += 1
            status = .reconnecting
            scheduleReconnect()
        } else if settings.transcriptionEnabled {
            if case .failed = status {
                return
            }
            status = .stopped
        } else {
            status = .disabled
        }
    }

    private func markAudioStreamingFailed(_ error: Error) async {
        guard hasStartedSession || currentConfig != nil else {
            return
        }
        lastError = error.localizedDescription
        audioCaptureWarning = nil
        status = .failed(error.localizedDescription)
        audioCaptureWatchdogTask?.cancel()
        audioCaptureWatchdogTask = nil
        await tearDownRuntime()
        guard settings.transcriptionEnabled, reconnectAttempts < maxReconnectAttempts else {
            return
        }
        reconnectAttempts += 1
        status = .reconnecting
        scheduleReconnect()
    }

    private func tearDownRuntime() async {
        audioCaptureWatchdogTask?.cancel()
        audioCaptureWatchdogTask = nil
        eventTask = nil
        hasStartedSession = false
        currentConfig = nil
        await client.stopSession()
        await audioCaptureService.stopCapture()
    }

    private func resetAudioTelemetry() {
        audioCaptureWatchdogTask?.cancel()
        audioCaptureWatchdogTask = nil
        audioChunksSent = 0
        audioBytesSent = 0
        lastAudioChunkSentAt = nil
        lastAudioPeakLevel = 0
        lastAudioTelemetry = .zero
        audioCaptureWarning = nil
    }

    @MainActor
    private func recordAudioChunkSent(_ audioChunk: Data, telemetry: AudioCaptureTelemetry) {
        audioChunksSent += 1
        audioBytesSent += audioChunk.count
        lastAudioChunkSentAt = Date()
        lastAudioPeakLevel = telemetry.postGainPeakLevel
        lastAudioTelemetry = telemetry
        audioCaptureWarning = nil
    }

    private func scheduleAudioCaptureWatchdog() {
        audioCaptureWatchdogTask?.cancel()
        audioCaptureWatchdogTask = Task { @MainActor [weak self] in
            do {
                try await Task.sleep(nanoseconds: 6_000_000_000)
            } catch {
                return
            }
            guard !Task.isCancelled,
                  let self,
                  self.status == .live,
                  self.audioChunksSent == 0 else {
                return
            }
            self.audioCaptureWarning = "No microphone audio frames have reached the transcription service yet. Check the macOS input source and input level."
        }
    }

    nonisolated private static var gainFormatter: NumberFormatter {
        let formatter = NumberFormatter()
        formatter.minimumFractionDigits = 1
        formatter.maximumFractionDigits = 1
        return formatter
    }

    private func scheduleReconnect() {
        reconnectTask?.cancel()
        reconnectTask = Task { @MainActor [weak self] in
            guard let self else { return }
            if reconnectDelayNanoseconds > 0 {
                do {
                    try await Task.sleep(nanoseconds: reconnectDelayNanoseconds)
                } catch {
                    return
                }
            }
            guard !Task.isCancelled, settings.transcriptionEnabled else {
                return
            }
            await startSessionForCurrentSettings(preservingTranscript: true)
        }
    }

    private func waitForEventTaskToFinish(timeoutNanoseconds: UInt64) async {
        guard let eventTask else {
            return
        }
        await withTaskGroup(of: Void.self) { group in
            group.addTask {
                await eventTask.value
            }
            group.addTask {
                try? await Task.sleep(nanoseconds: timeoutNanoseconds)
            }
            await group.next()
            group.cancelAll()
        }
    }

    private static func timelineLine(_ segment: TranscriptSegment) -> String {
        let text = segment.text.trimmingCharacters(in: .whitespacesAndNewlines)
        guard !text.isEmpty else {
            return ""
        }

        var parts: [String] = ["[\(timelineRange(segment))]"]
        if let speakerLabel = segment.speakerLabel {
            parts.append(speakerLabel)
        }
        if !segment.isFinal {
            parts.append("(partial)")
        }
        return "\(parts.joined(separator: " ")): \(text)"
    }

    private static func timelineRange(_ segment: TranscriptSegment) -> String {
        let start = String(format: "%.1fs", Double(segment.startTimeMilliseconds) / 1000.0)
        guard let end = segment.endTimeMilliseconds else {
            return start
        }
        return "\(start)-\(String(format: "%.1fs", Double(end) / 1000.0))"
    }
}
