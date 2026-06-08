# Signal Connectors Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Add the first SignalConnector substrate and adapters for Webex/iMessage without changing user-visible refresh behavior.

**Architecture:** Add source-neutral signal models, a connector protocol, target routing, a sync pipeline, and a knowledge writer. Move existing Webex/iMessage source code under `apps/cubicle-macos/Sources/Connectors`, then add adapters that can emit normalized message signals while the existing refresh facade remains stable.

**Tech Stack:** Swift 5.9, SwiftPM, XCTest, SQLite-backed `KnowledgeStore`.

---

### Task 1: Add Signal Models And Protocol

**Files:**
- Create: `apps/cubicle-macos/Sources/Connectors/SignalModels.swift`
- Create: `apps/cubicle-macos/Sources/Connectors/SignalConnector.swift`
- Test: `apps/cubicle-macos/Tests/SignalConnectorTests.swift`

- [x] Step 1: Write tests for target selectors and connector routing primitives.
- [x] Step 2: Run `swift test --filter SignalConnectorTests` and verify it fails because the types do not exist.
- [x] Step 3: Add `ConnectorID`, `SignalTarget`, selectors, signal envelopes, connector descriptors, sync requests, sync batches, and `SignalConnector`.
- [x] Step 4: Run `swift test --filter SignalConnectorTests` and verify the new tests pass.

### Task 2: Add iMessage And Webex Signal Adapters

**Files:**
- Move: `apps/cubicle-macos/Sources/Services/NativeIMessageIngestionService.swift` to `apps/cubicle-macos/Sources/Connectors/IMessage/NativeIMessageIngestionService.swift`
- Move: `apps/cubicle-macos/Sources/Services/WebexAPIClient.swift` to `apps/cubicle-macos/Sources/Connectors/Webex/WebexAPIClient.swift`
- Move: `apps/cubicle-macos/Sources/Services/WebexSyncEngine.swift` to `apps/cubicle-macos/Sources/Connectors/Webex/WebexSyncEngine.swift`
- Create: `apps/cubicle-macos/Sources/Connectors/IMessage/IMessageSignalConnector.swift`
- Create: `apps/cubicle-macos/Sources/Connectors/Webex/WebexSignalConnector.swift`
- Test: `apps/cubicle-macos/Tests/SignalConnectorTests.swift`

- [x] Step 1: Add failing tests proving iMessage and Webex adapters emit `MessageEventPayload` batches.
- [x] Step 2: Run `swift test --filter SignalConnectorTests` and verify adapter tests fail because adapters do not exist.
- [x] Step 3: Move existing source-specific files into `Connectors/`.
- [x] Step 4: Implement `IMessageSignalConnector` and `WebexSignalConnector`.
- [x] Step 5: Run `swift test --filter SignalConnectorTests` and verify adapter tests pass.

### Task 3: Add Common Processing Layer

**Files:**
- Create: `apps/cubicle-macos/Sources/Connectors/SignalSyncPipeline.swift`
- Create: `apps/cubicle-macos/Sources/Connectors/SignalKnowledgeWriter.swift`
- Test: `apps/cubicle-macos/Tests/SignalConnectorTests.swift`

- [x] Step 1: Add failing tests proving the pipeline invokes connector-specific targets and the writer persists message events idempotently.
- [x] Step 2: Run `swift test --filter SignalConnectorTests` and verify the new tests fail because pipeline/writer code does not exist.
- [x] Step 3: Implement `SignalSyncPipeline`, `SignalKnowledgeWriting`, and `SignalKnowledgeWriter`.
- [x] Step 4: Run `swift test --filter SignalConnectorTests` and verify the tests pass.

### Task 4: Verify Existing Behavior And Commit

**Files:**
- Modify only if required by compiler fallout.

- [x] Step 1: Run `swift build`.
- [x] Step 2: Run `swift test`.
- [x] Step 3: Run `git diff --check`.
- [x] Step 4: Commit implementation.
- [x] Step 5: Push branch and open a stacked PR with base `chore/knowledge-dao-refactor`.
