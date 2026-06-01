import Foundation
import CryptoKit

/// Codex job artifact locations and lifecycle status.
struct CodexPromptJob: Identifiable, Hashable {
    /// Persisted status for a Codex prompt job.
    enum Status: String {
        case pending
        case running
        case completed
        case failed
    }

    var id: String
    var title: String
    var promptVersion: String
    var promptURL: URL
    var outputURL: URL
    var logURL: URL
    var metadataURL: URL? = nil
    var status: Status
    var createdAt: Date
}

/// Retry and timeout policy for Codex process execution.
struct CodexRunPolicy: Hashable {
    var timeoutSeconds: TimeInterval?
    var maxAttempts: Int
    var retryDelaySeconds: TimeInterval

    static let `default` = CodexRunPolicy(
        timeoutSeconds: 120,
        maxAttempts: 2,
        retryDelaySeconds: 1.5
    )
}

/// Complete request for one Codex process run.
struct CodexRunRequest: Hashable {
    var prompt: String
    var job: CodexPromptJob
    var workingDirectory: URL
    var policy: CodexRunPolicy
}

/// Metadata for one Codex attempt.
struct CodexAttemptMetadata: Codable, Hashable {
    var attempt: Int
    var startedAt: String
    var finishedAt: String
    var durationMilliseconds: Int
    var terminationStatus: Int32?
    var logPath: String
    var error: String?
}

/// Metadata persisted alongside prompt/output/log artifacts.
struct CodexRunArtifactMetadata: Codable, Hashable {
    var jobID: String
    var title: String
    var promptVersion: String
    var promptHash: String
    var promptPath: String
    var outputPath: String
    var logPath: String
    var metadataPath: String
    var attempts: Int
    var startedAt: String
    var finishedAt: String
    var durationMilliseconds: Int
    var status: String
    var attemptDetails: [CodexAttemptMetadata]
    var error: String?
}

/// Codex output plus persisted artifact metadata.
struct CodexRunResult: Hashable {
    var output: String
    var metadata: CodexRunArtifactMetadata
}

/// One-shot termination continuation guard for Process callbacks.
private final class CodexProcessTerminationWaiter: @unchecked Sendable {
    private let lock = NSLock()
    private var didResume = false
    private var continuation: CheckedContinuation<Int32, Error>?

    /// Captures the continuation waiting for process termination.
    init(continuation: CheckedContinuation<Int32, Error>) {
        self.continuation = continuation
    }

    /// Resumes termination wait exactly once.
    func resume(_ result: Result<Int32, Error>) {
        lock.lock()
        guard !didResume, let continuation else {
            lock.unlock()
            return
        }
        didResume = true
        self.continuation = nil
        lock.unlock()

        continuation.resume(with: result)
    }
}

/// Process, timeout, and artifact failures from Codex execution.
enum CodexRunnerError: LocalizedError {
    case codexFailed(Int32)
    case outputMissing(URL)
    case timeout(seconds: TimeInterval)
    case cancelled
    case unsafeCodexExecutable(String)
    case retriesExhausted(attempts: Int, lastError: String)

    var errorDescription: String? {
        switch self {
        case .codexFailed(let status):
            return "Codex exited with status \(status)."
        case .outputMissing(let url):
            return "Codex did not produce output at \(url.path)."
        case .timeout(let seconds):
            return "Codex timed out after \(Int(seconds)) seconds."
        case .cancelled:
            return "Codex run was cancelled."
        case .unsafeCodexExecutable(let path):
            return "Unsafe Codex executable path rejected because it can expose prompts through process arguments: \(path)"
        case .retriesExhausted(let attempts, let lastError):
            return "Codex failed after \(attempts) attempts: \(lastError)"
        }
    }
}

/// Runs Codex as a local process and writes private prompt/output artifacts.
final class CodexRunner {
    let configuration: RuntimeConfiguration

    /// Binds the runner to a runtime configuration.
    init(configuration: RuntimeConfiguration = .current) {
        self.configuration = configuration
    }

    /// Convenience wrapper returning only output text.
    func run(prompt: String, job: CodexPromptJob, workingDirectory: URL) async throws -> String {
        let request = CodexRunRequest(
            prompt: prompt,
            job: job,
            workingDirectory: workingDirectory,
            policy: .default
        )
        return try await run(request: request).output
    }

    /// Runs Codex with retry policy and returns output plus artifact metadata.
    func run(request: CodexRunRequest) async throws -> CodexRunResult {
        let normalizedMaxAttempts = max(1, request.policy.maxAttempts)
        let startedAt = Date()
        let promptHash = Self.sha256Hex(request.prompt)

        try ensureArtifactDirectories(for: request.job)
        try writePrivateUTF8(request.prompt, to: request.job.promptURL)

        var attemptDetails: [CodexAttemptMetadata] = []
        var lastErrorDescription = "unknown error"
        var attemptCount = 0

        while attemptCount < normalizedMaxAttempts {
            try Task.checkCancellation()
            attemptCount += 1

            do {
                let attemptOutput = try await executeAttempt(
                    request: request,
                    attempt: attemptCount
                )

                let output = try String(contentsOf: request.job.outputURL, encoding: .utf8)
                let finishedAt = Date()
                let metadata = CodexRunArtifactMetadata(
                    jobID: request.job.id,
                    title: request.job.title,
                    promptVersion: request.job.promptVersion,
                    promptHash: promptHash,
                    promptPath: request.job.promptURL.path,
                    outputPath: request.job.outputURL.path,
                    logPath: request.job.logURL.path,
                    metadataPath: metadataURL(for: request.job).path,
                    attempts: attemptCount,
                    startedAt: Self.iso8601String(from: startedAt),
                    finishedAt: Self.iso8601String(from: finishedAt),
                    durationMilliseconds: milliseconds(from: startedAt, to: finishedAt),
                    status: CodexPromptJob.Status.completed.rawValue,
                    attemptDetails: attemptDetails + [attemptOutput],
                    error: nil
                )
                try persistMetadata(metadata, at: metadataURL(for: request.job))
                return CodexRunResult(output: output, metadata: metadata)
            } catch is CancellationError {
                let attemptEndedAt = Date()
                let cancelledAttempt = CodexAttemptMetadata(
                    attempt: attemptCount,
                    startedAt: Self.iso8601String(from: attemptEndedAt),
                    finishedAt: Self.iso8601String(from: attemptEndedAt),
                    durationMilliseconds: 0,
                    terminationStatus: nil,
                    logPath: request.job.logURL.path,
                    error: CodexRunnerError.cancelled.localizedDescription
                )
                attemptDetails.append(cancelledAttempt)
                let metadata = failureMetadata(
                    request: request,
                    startedAt: startedAt,
                    attemptCount: attemptCount,
                    promptHash: promptHash,
                    attemptDetails: attemptDetails,
                    error: CodexRunnerError.cancelled.localizedDescription
                )
                try persistMetadata(metadata, at: metadataURL(for: request.job))
                throw CodexRunnerError.cancelled
            } catch {
                lastErrorDescription = error.localizedDescription
                let attemptEndedAt = Date()
                let failedAttempt = CodexAttemptMetadata(
                    attempt: attemptCount,
                    startedAt: Self.iso8601String(from: attemptEndedAt),
                    finishedAt: Self.iso8601String(from: attemptEndedAt),
                    durationMilliseconds: 0,
                    terminationStatus: nil,
                    logPath: request.job.logURL.path,
                    error: error.localizedDescription
                )
                attemptDetails.append(failedAttempt)

                let canRetry = attemptCount < normalizedMaxAttempts && isRetryable(error: error)
                if canRetry {
                    let retryDelay = max(0, request.policy.retryDelaySeconds)
                    if retryDelay > 0 {
                        try await Task.sleep(nanoseconds: UInt64(retryDelay * 1_000_000_000))
                    }
                    continue
                }

                let metadata = failureMetadata(
                    request: request,
                    startedAt: startedAt,
                    attemptCount: attemptCount,
                    promptHash: promptHash,
                    attemptDetails: attemptDetails,
                    error: lastErrorDescription
                )
                try persistMetadata(metadata, at: metadataURL(for: request.job))
                if attemptCount > 1 {
                    throw CodexRunnerError.retriesExhausted(
                        attempts: attemptCount,
                        lastError: lastErrorDescription
                    )
                }
                throw error
            }
        }

        let metadata = failureMetadata(
            request: request,
            startedAt: startedAt,
            attemptCount: attemptCount,
            promptHash: promptHash,
            attemptDetails: attemptDetails,
            error: lastErrorDescription
        )
        try persistMetadata(metadata, at: metadataURL(for: request.job))
        throw CodexRunnerError.retriesExhausted(
            attempts: attemptCount,
            lastError: lastErrorDescription
        )
    }

    /// Executes one process attempt and validates output artifact creation.
    private func executeAttempt(request: CodexRunRequest, attempt: Int) async throws -> CodexAttemptMetadata {
        let attemptStartedAt = Date()
        let attemptLogURL = attemptLogURL(for: request.job, attempt: attempt)
        FileManager.default.createFile(atPath: attemptLogURL.path, contents: nil)
        let logHandle = try FileHandle(forWritingTo: attemptLogURL)
        let inputHandle = try FileHandle(forReadingFrom: request.job.promptURL)
        let systemSettings = ConfigStore(configuration: configuration).loadSystemSettings()
        let codexExecutable = try validatedCodexExecutable()

        let process = Process()
        process.executableURL = URL(fileURLWithPath: "/usr/bin/env")
        process.arguments = codexExecArguments(
            codexExecutable: codexExecutable,
            settings: systemSettings,
            outputURL: request.job.outputURL
        )
        process.currentDirectoryURL = request.workingDirectory
        process.environment = Self.codexProcessEnvironment(ProcessInfo.processInfo.environment)
        process.standardInput = inputHandle
        process.standardOutput = logHandle
        process.standardError = logHandle

        do {
            try process.run()
            try? inputHandle.close()
        } catch {
            try? inputHandle.close()
            try? logHandle.close()
            throw error
        }

        let terminationStatus: Int32
        do {
            terminationStatus = try await waitForTermination(
                process: process,
                timeoutSeconds: request.policy.timeoutSeconds
            )
        } catch {
            if process.isRunning {
                process.terminate()
            }
            try? logHandle.close()
            if error is CancellationError {
                throw CodexRunnerError.cancelled
            }
            throw error
        }

        try? logHandle.close()
        try promoteAttemptLog(attemptLogURL, to: request.job.logURL)

        let attemptFinishedAt = Date()
        let durationMs = milliseconds(from: attemptStartedAt, to: attemptFinishedAt)

        guard terminationStatus == 0 else {
            throw CodexRunnerError.codexFailed(terminationStatus)
        }
        guard FileManager.default.fileExists(atPath: request.job.outputURL.path) else {
            throw CodexRunnerError.outputMissing(request.job.outputURL)
        }
        try? setPrivateFilePermissions(request.job.outputURL)
        try? setPrivateFilePermissions(request.job.logURL)

        return CodexAttemptMetadata(
            attempt: attempt,
            startedAt: Self.iso8601String(from: attemptStartedAt),
            finishedAt: Self.iso8601String(from: attemptFinishedAt),
            durationMilliseconds: durationMs,
            terminationStatus: terminationStatus,
            logPath: attemptLogURL.path,
            error: nil
        )
    }

    /// Waits for process exit with optional timeout termination.
    private func waitForTermination(
        process: Process,
        timeoutSeconds: TimeInterval?
    ) async throws -> Int32 {
        try await withCheckedThrowingContinuation { continuation in
            let waiter = CodexProcessTerminationWaiter(continuation: continuation)
            process.terminationHandler = { terminatedProcess in
                waiter.resume(.success(terminatedProcess.terminationStatus))
            }

            if !process.isRunning {
                waiter.resume(.success(process.terminationStatus))
                return
            }

            guard let timeoutSeconds, timeoutSeconds > 0 else {
                return
            }

            DispatchQueue.global(qos: .utility).asyncAfter(deadline: .now() + timeoutSeconds) {
                if process.isRunning {
                    process.terminate()
                }
                waiter.resume(.failure(CodexRunnerError.timeout(seconds: timeoutSeconds)))
            }
        }
    }

    /// Creates artifact directories and restricts directory permissions.
    private func ensureArtifactDirectories(for job: CodexPromptJob) throws {
        try FileManager.default.createDirectory(at: job.promptURL.deletingLastPathComponent(), withIntermediateDirectories: true)
        try FileManager.default.createDirectory(at: job.outputURL.deletingLastPathComponent(), withIntermediateDirectories: true)
        try FileManager.default.createDirectory(at: job.logURL.deletingLastPathComponent(), withIntermediateDirectories: true)
        try FileManager.default.createDirectory(at: metadataURL(for: job).deletingLastPathComponent(), withIntermediateDirectories: true)
        try setPrivateDirectoryPermissions(job.promptURL.deletingLastPathComponent())
        try setPrivateDirectoryPermissions(job.outputURL.deletingLastPathComponent())
        try setPrivateDirectoryPermissions(job.logURL.deletingLastPathComponent())
        try setPrivateDirectoryPermissions(metadataURL(for: job).deletingLastPathComponent())
    }

    /// Copies the successful attempt log to the canonical job log path.
    private func promoteAttemptLog(_ sourceURL: URL, to destinationURL: URL) throws {
        if sourceURL.path == destinationURL.path {
            return
        }
        if FileManager.default.fileExists(atPath: destinationURL.path) {
            try FileManager.default.removeItem(at: destinationURL)
        }
        try FileManager.default.copyItem(at: sourceURL, to: destinationURL)
    }

    /// Builds failure metadata after retries are exhausted.
    private func failureMetadata(
        request: CodexRunRequest,
        startedAt: Date,
        attemptCount: Int,
        promptHash: String,
        attemptDetails: [CodexAttemptMetadata],
        error: String
    ) -> CodexRunArtifactMetadata {
        let finishedAt = Date()
        return CodexRunArtifactMetadata(
            jobID: request.job.id,
            title: request.job.title,
            promptVersion: request.job.promptVersion,
            promptHash: promptHash,
            promptPath: request.job.promptURL.path,
            outputPath: request.job.outputURL.path,
            logPath: request.job.logURL.path,
            metadataPath: metadataURL(for: request.job).path,
            attempts: attemptCount,
            startedAt: Self.iso8601String(from: startedAt),
            finishedAt: Self.iso8601String(from: finishedAt),
            durationMilliseconds: milliseconds(from: startedAt, to: finishedAt),
            status: CodexPromptJob.Status.failed.rawValue,
            attemptDetails: attemptDetails,
            error: error
        )
    }

    /// Persists metadata with private file permissions.
    private func persistMetadata(_ metadata: CodexRunArtifactMetadata, at url: URL) throws {
        let encoder = JSONEncoder()
        encoder.outputFormatting = [.prettyPrinted, .sortedKeys]
        let data = try encoder.encode(metadata)
        try data.write(to: url, options: .atomic)
        try setPrivateFilePermissions(url)
    }

    /// Writes prompt/output-adjacent text with private file permissions.
    private func writePrivateUTF8(_ value: String, to url: URL) throws {
        guard let data = value.data(using: .utf8) else {
            try value.write(to: url, atomically: true, encoding: .utf8)
            try setPrivateFilePermissions(url)
            return
        }
        try data.write(to: url, options: .atomic)
        try setPrivateFilePermissions(url)
    }

    private func setPrivateFilePermissions(_ url: URL) throws {
        try FileManager.default.setAttributes(
            [.posixPermissions: NSNumber(value: Int16(0o600))],
            ofItemAtPath: url.path
        )
    }

    private func setPrivateDirectoryPermissions(_ url: URL) throws {
        try FileManager.default.setAttributes(
            [.posixPermissions: NSNumber(value: Int16(0o700))],
            ofItemAtPath: url.path
        )
    }

    /// Resolves explicit metadata URL or default output-neighbor path.
    private func metadataURL(for job: CodexPromptJob) -> URL {
        if let metadataURL = job.metadataURL {
            return metadataURL
        }
        return job.outputURL.deletingLastPathComponent().appendingPathComponent("\(job.id)-metadata.json")
    }

    /// Per-attempt log path, preserving the canonical first-attempt path.
    private func attemptLogURL(for job: CodexPromptJob, attempt: Int) -> URL {
        if attempt <= 1 {
            return job.logURL
        }
        return job.logURL.deletingLastPathComponent().appendingPathComponent("\(job.id)-attempt-\(attempt).log")
    }

    /// Arguments for stdin-fed Codex execution.
    private func codexExecArguments(codexExecutable: String, settings: SystemSettings, outputURL: URL) -> [String] {
        [
            codexExecutable,
            "exec",
            "--ignore-user-config",
            "--skip-git-repo-check",
            "--ephemeral",
            "--model",
            settings.codexModel.rawValue,
            "--config",
            #"model_reasoning_effort="\#(settings.codexReasoningLevel.rawValue)""#,
            "--output-last-message",
            outputURL.path,
            "-"
        ]
    }

    /// Rejects unsafe wrappers and returns executable fallback.
    private func validatedCodexExecutable() throws -> String {
        let executable = configuration.codexExecutable.trimmingCharacters(in: .whitespacesAndNewlines)
        if Self.isUnsafeCodexExecutablePath(executable) {
            throw CodexRunnerError.unsafeCodexExecutable(executable)
        }
        return executable.isEmpty ? "codex" : executable
    }

    /// Blocks computer-use wrappers that can expose prompts through process args.
    private static func isUnsafeCodexExecutablePath(_ path: String) -> Bool {
        let normalized = path.lowercased()
        let unsafeMarkers = [
            ".codex/computer-use",
            "codex computer use.app",
            "skycomputeruseclient"
        ]
        return unsafeMarkers.contains { normalized.contains($0) }
    }

    private static func codexProcessEnvironment(_ baseEnvironment: [String: String]) -> [String: String] {
        var environment = baseEnvironment
        let defaultPathEntries = [
            "/opt/homebrew/bin",
            "/opt/homebrew/sbin",
            "/usr/local/bin",
            "/usr/local/sbin",
            "/usr/bin",
            "/bin",
            "/usr/sbin",
            "/sbin"
        ]
        let currentPath = environment["PATH"]?.trimmingCharacters(in: .whitespacesAndNewlines) ?? ""
        var pathEntries = currentPath.isEmpty ? [] : currentPath.split(separator: ":").map(String.init)
        for entry in defaultPathEntries where !pathEntries.contains(entry) {
            pathEntries.append(entry)
        }
        environment["PATH"] = pathEntries.joined(separator: ":")
        environment["SHELL"] = environment["SHELL"] ?? "/bin/zsh"
        environment["LANG"] = environment["LANG"] ?? "en_US.UTF-8"
        environment["LC_ALL"] = environment["LC_ALL"] ?? "en_US.UTF-8"
        return environment
    }

    private func isRetryable(error: Error) -> Bool {
        guard let codexError = error as? CodexRunnerError else {
            return false
        }
        switch codexError {
        case .codexFailed, .outputMissing, .timeout:
            return true
        case .cancelled, .unsafeCodexExecutable, .retriesExhausted:
            return false
        }
    }

    private func milliseconds(from start: Date, to end: Date) -> Int {
        Int(end.timeIntervalSince(start) * 1000)
    }

    private static func sha256Hex(_ value: String) -> String {
        let digest = SHA256.hash(data: Data(value.utf8))
        return digest.map { String(format: "%02x", $0) }.joined()
    }

    private static func iso8601String(from date: Date) -> String {
        let formatter = ISO8601DateFormatter()
        formatter.formatOptions = [.withInternetDateTime, .withFractionalSeconds]
        return formatter.string(from: date)
    }
}
