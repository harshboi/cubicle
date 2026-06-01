import Foundation

protocol TranscriptionAudioStreamingClient: AnyObject {
    func sendAudioChunk(_ data: Data) async throws
}

protocol TranscriptionWebSocketTransport: AnyObject {
    func connect(to url: URL, headers: [String: String]) async throws
    func send(_ message: TranscriptionWebSocketMessage) async throws
    func receive() async throws -> TranscriptionWebSocketMessage
    func close() async
}

final class URLSessionTranscriptionWebSocketTransport: TranscriptionWebSocketTransport {
    private let session: URLSession
    private var task: URLSessionWebSocketTask?

    init(session: URLSession = URLSession(configuration: .ephemeral)) {
        self.session = session
    }

    func connect(to url: URL, headers: [String: String]) async throws {
        var request = URLRequest(url: url)
        for (key, value) in headers {
            request.setValue(value, forHTTPHeaderField: key)
        }
        let task = session.webSocketTask(with: request)
        self.task = task
        task.resume()
    }

    func send(_ message: TranscriptionWebSocketMessage) async throws {
        guard let task else {
            throw TranscriptionProtocolError.notConnected
        }
        switch message {
        case .text(let text):
            try await task.send(.string(text))
        case .data(let data):
            try await task.send(.data(data))
        }
    }

    func receive() async throws -> TranscriptionWebSocketMessage {
        guard let task else {
            throw TranscriptionProtocolError.notConnected
        }
        let message = try await task.receive()
        switch message {
        case .string(let text):
            return .text(text)
        case .data(let data):
            return .data(data)
        @unknown default:
            throw TranscriptionProtocolError.unsupportedMessageType
        }
    }

    func close() async {
        task?.cancel(with: .normalClosure, reason: nil)
        task = nil
    }
}

final class TranscriptionWebSocketClient: TranscriptionClient, TranscriptionAudioStreamingClient {
    private let codec: TranscriptionProtocolCodec
    private let transportFactory: () -> TranscriptionWebSocketTransport
    private var transport: TranscriptionWebSocketTransport?
    private var receiveTask: Task<Void, Never>?
    private var activeSessionID: String?

    init(
        codec: TranscriptionProtocolCodec = TranscriptionProtocolCodec(),
        transportFactory: @escaping () -> TranscriptionWebSocketTransport = {
            URLSessionTranscriptionWebSocketTransport()
        }
    ) {
        self.codec = codec
        self.transportFactory = transportFactory
    }

    func startSession(config: TranscriptionSessionConfig) async throws -> AsyncStream<TranscriptionServerEvent> {
        guard config.transcriptionEnabled else {
            return AsyncStream { continuation in
                continuation.finish()
            }
        }

        let url = try codec.endpointURL(from: config.endpointURL)
        let transport = transportFactory()
        self.transport = transport
        self.activeSessionID = config.sessionID
        try await transport.connect(to: url, headers: codec.authorizationHeaders(for: config))
        try await transport.send(.text(try codec.encodeStartSession(config)))
        let initialEvent = try codec.decodeServerEvent(from: try await transport.receive())
        switch initialEvent {
        case .sessionStarted(let sessionID):
            self.activeSessionID = sessionID
        case .error(let message):
            await transport.close()
            self.transport = nil
            self.activeSessionID = nil
            throw TranscriptionProtocolError.sessionStartRejected(message)
        default:
            await transport.close()
            self.transport = nil
            self.activeSessionID = nil
            throw TranscriptionProtocolError.unexpectedSessionStartEvent(Self.eventName(initialEvent))
        }

        return AsyncStream { [weak self, transport, codec, initialEvent] continuation in
            self?.receiveTask?.cancel()
            continuation.yield(initialEvent)
            self?.receiveTask = Task {
                while !Task.isCancelled {
                    do {
                        let message = try await transport.receive()
                        let event = try codec.decodeServerEvent(from: message)
                        continuation.yield(event)
                        if case .sessionStopped = event {
                            break
                        }
                    } catch {
                        if Task.isCancelled {
                            break
                        }
                        continuation.yield(.error(error.localizedDescription))
                        break
                    }
                }
                continuation.finish()
            }
        }
    }

    func sendAudioChunk(_ data: Data) async throws {
        guard let transport else {
            throw TranscriptionProtocolError.notConnected
        }
        try await transport.send(.data(codec.encodeAudioChunk(data)))
    }

    func stopSession() async {
        let taskToDrain = receiveTask
        if let transport, let activeSessionID {
            try? await transport.send(.text(try codec.encodeStopSession(sessionID: activeSessionID)))
            await waitForReceiveTaskToFinish(taskToDrain, timeoutNanoseconds: 60_000_000_000)
            await transport.close()
        } else if let transport {
            await transport.close()
        }
        receiveTask?.cancel()
        receiveTask = nil
        transport = nil
        activeSessionID = nil
    }

    private func waitForReceiveTaskToFinish(_ task: Task<Void, Never>?, timeoutNanoseconds: UInt64) async {
        guard let task else {
            return
        }
        await withTaskGroup(of: Void.self) { group in
            group.addTask {
                await task.value
            }
            group.addTask {
                try? await Task.sleep(nanoseconds: timeoutNanoseconds)
            }
            await group.next()
            group.cancelAll()
        }
    }

    private static func eventName(_ event: TranscriptionServerEvent) -> String {
        switch event {
        case .sessionStarted:
            return "session_started"
        case .partialTranscript:
            return "partial_transcript"
        case .finalTranscript:
            return "final_transcript"
        case .speakerUpdate:
            return "speaker_update"
        case .diarizationStatus:
            return "diarization_status"
        case .correctionUpdate:
            return "correction_update"
        case .error:
            return "error"
        case .sessionStopped:
            return "session_stopped"
        }
    }
}
