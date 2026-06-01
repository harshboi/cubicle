# Atomic Signal DB Writes Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Make connector batch persistence atomic and document the current architecture/call-flow DAGs in the repo README.

**Architecture:** PR #2 adds a narrow `KnowledgeStore` transaction primitive and test coverage for rollback. PR #3 uses that primitive in `SignalKnowledgeWriter` so a connector batch either writes all mapped rows or no mapped rows, then updates README diagrams for architecture and filename-level call flow.

**Tech Stack:** SwiftPM, XCTest, SQLite3, SwiftUI macOS app, GitHub stacked PRs.

---

## File Structure

- PR #2:
  - Modify `apps/cubicle-macos/Sources/Data/DAO/KnowledgeStore.swift`
    - Add a narrow internal `writeConnectorMessageBatch` method that performs room/person/message/evidence upserts in one SQLite transaction.
  - Modify `apps/cubicle-macos/Tests/QuestionEngineCoreIntegrationTests.swift`
    - Add rollback test proving failed evidence validation rolls back earlier message/room/person writes.
- PR #3:
  - Modify `apps/cubicle-macos/Sources/Connectors/SignalKnowledgeWriter.swift`
    - Collect mapped records from a `SignalSyncBatch`, call `writeConnectorMessageBatch`, and keep summary counts unchanged.
  - Modify `apps/cubicle-macos/Tests/SignalConnectorTests.swift`
    - Add atomic writer test proving invalid second event rolls back the first event.
  - Modify `README.md`
    - Replace broad DAGs with architecture DAG, runtime call-flow DAG, and filename call-flow DAG.

## Task 1: PR #2 DB Transaction Primitive

**Files:**
- Modify: `apps/cubicle-macos/Sources/Data/DAO/KnowledgeStore.swift`
- Test: `apps/cubicle-macos/Tests/QuestionEngineCoreIntegrationTests.swift`

- [ ] **Step 1: Write failing rollback test**

Add this test to `QuestionEngineCoreIntegrationTests`:

```swift
func testKnowledgeStoreConnectorBatchRollsBackOnEvidenceFailure() throws {
    let runtimeRoot = FileManager.default.temporaryDirectory
        .appendingPathComponent("CubicleConnectorBatchRollback-\(UUID().uuidString)", isDirectory: true)
    defer { try? FileManager.default.removeItem(at: runtimeRoot) }
    let store = KnowledgeStore(configuration: testRuntimeConfiguration(runtimeRoot: runtimeRoot))
    let now = "2026-06-01T00:00:00.000Z"

    XCTAssertThrowsError(
        try store.writeConnectorMessageBatch(
            rooms: [RoomRecord(id: "room-rollback", title: "Rollback Room", updatedAt: now)],
            people: [PersonRecord(id: "person-rollback", displayName: "Rollback Person", email: "rollback@example.com", updatedAt: now)],
            messages: [MessageRecord(id: "message-rollback", roomID: "room-rollback", personID: "person-rollback", body: "Should roll back.", createdAt: now, updatedAt: now)],
            evidence: [BeliefEvidenceRecord(id: "bad-evidence", source: "test", sourceID: "message-rollback", roomID: "room-rollback", personID: "person-rollback", occurredAt: now, text: "")]
        )
    )

    XCTAssertNil(try store.loadRoom(roomID: "room-rollback"))
    XCTAssertFalse(try store.messageExists(messageID: "message-rollback"))
    XCTAssertTrue(try store.loadBeliefEvidence(scope: .space, entityKey: "room-rollback").isEmpty)
}
```

- [ ] **Step 2: Run test to verify it fails**

Run:

```bash
swift test --filter QuestionEngineCoreIntegrationTests/testKnowledgeStoreConnectorBatchRollsBackOnEvidenceFailure
```

Expected: compile failure because `writeConnectorMessageBatch` does not exist.

- [ ] **Step 3: Implement transaction helper**

Add this method near existing room/person/message/evidence upsert methods in `KnowledgeStore`:

```swift
/// Atomically persists connector-mapped rows for one signal batch.
func writeConnectorMessageBatch(
    rooms: [RoomRecord],
    people: [PersonRecord],
    messages: [MessageRecord],
    evidence: [BeliefEvidenceRecord]
) throws {
    guard !rooms.isEmpty || !people.isEmpty || !messages.isEmpty || !evidence.isEmpty else {
        return
    }
    try withDatabase { db in
        try execute(sql: "BEGIN IMMEDIATE TRANSACTION;", db: db)
        do {
            for room in rooms {
                try upsertRoomRecord(room, db: db)
            }
            for person in people {
                try upsertPersonRecord(person, db: db)
            }
            for message in messages {
                try upsertMessageRecord(message, db: db)
            }
            for evidenceRecord in evidence {
                try upsertBeliefEvidenceRecord(evidenceRecord, db: db)
            }
            try execute(sql: "COMMIT;", db: db)
        } catch {
            _ = sqlite3_exec(db, "ROLLBACK;", nil, nil, nil)
            throw error
        }
    }
}
```

- [ ] **Step 4: Run focused test**

Run:

```bash
swift test --filter QuestionEngineCoreIntegrationTests/testKnowledgeStoreConnectorBatchRollsBackOnEvidenceFailure
```

Expected: pass.

- [ ] **Step 5: Run PR #2 Swift tests**

Run:

```bash
swift test
```

Expected: all Swift tests pass.

- [ ] **Step 6: Commit PR #2 change**

```bash
git add apps/cubicle-macos/Sources/Data/DAO/KnowledgeStore.swift apps/cubicle-macos/Tests/QuestionEngineCoreIntegrationTests.swift
git commit -m "Add atomic connector batch write"
```

## Task 2: PR #3 Atomic Signal Writer

**Files:**
- Modify: `apps/cubicle-macos/Sources/Connectors/SignalKnowledgeWriter.swift`
- Test: `apps/cubicle-macos/Tests/SignalConnectorTests.swift`

- [ ] **Step 1: Rebase PR #3 onto updated PR #2**

```bash
git checkout feat/signal-connectors
git rebase chore/knowledge-dao-refactor
```

Expected: clean rebase or only doc comment conflicts.

- [ ] **Step 2: Write failing writer rollback test**

Add this test to `SignalConnectorTests`:

```swift
func testSignalKnowledgeWriterRollsBackWholeBatchOnInvalidEvidence() throws {
    let runtimeRoot = FileManager.default.temporaryDirectory
        .appendingPathComponent("CubicleSignalWriterRollback-\(UUID().uuidString)", isDirectory: true)
    defer { try? FileManager.default.removeItem(at: runtimeRoot) }
    let knowledgeStore = KnowledgeStore(configuration: testSignalRuntimeConfiguration(runtimeRoot: runtimeRoot))
    let writer = SignalKnowledgeWriter(knowledgeStore: knowledgeStore)
    let occurredAt = Date(timeIntervalSince1970: 1_715_000_240)
    var batch = makeSignalMessageBatch(occurredAt: occurredAt)
    let invalidEvent = SignalEvent(
        id: "webex:workspace:message:bad-empty",
        sourceID: SourceEventID(connectorID: .webex, accountID: "workspace", kind: "message", externalID: "bad-empty"),
        kind: .message,
        actor: nil,
        occurredAt: occurredAt.addingTimeInterval(1),
        objectIDs: [],
        visibility: .authenticatedUser(connectorID: .webex, accountID: "workspace"),
        payload: .message(
            MessageEventPayload(
                threadID: "webex:workspace:room:room-1",
                threadSourceID: SourceObjectID(connectorID: .webex, accountID: "workspace", kind: "room", externalID: "room-1"),
                threadTitle: "Launch Room",
                senderID: nil,
                senderDisplayName: "",
                senderEmail: nil,
                body: "",
                isFromCurrentUser: false
            )
        )
    )
    batch.events.append(invalidEvent)

    XCTAssertThrowsError(try writer.write(batch))
    XCTAssertFalse(try knowledgeStore.messageExists(messageID: "webex:workspace:message:webex-message-1"))
    XCTAssertTrue(try knowledgeStore.loadBeliefEvidence(scope: .space, entityKey: "room-1").isEmpty)
}
```

- [ ] **Step 3: Run test to verify it fails**

Run:

```bash
swift test --filter SignalConnectorTests/testSignalKnowledgeWriterRollsBackWholeBatchOnInvalidEvidence
```

Expected: failure because the first message persists before the invalid event throws.

- [ ] **Step 4: Refactor writer to collect rows before writing**

Change `SignalKnowledgeWriter.write(_:)` so it:

```swift
let mapped = try mappedRecords(for: batch, fallbackUpdatedAt: updatedAt)
try knowledgeStore.writeConnectorMessageBatch(
    rooms: mapped.rooms,
    people: mapped.people,
    messages: mapped.messages,
    evidence: mapped.evidence
)
return SignalWriteSummary(
    messageEventsProcessed: mapped.messageEventsProcessed,
    evidenceRecordsWritten: mapped.evidence.count
)
```

Add a private mapped-record struct and mapping helpers. Do not write to `KnowledgeStore` while mapping.

- [ ] **Step 5: Run focused writer tests**

Run:

```bash
swift test --filter SignalConnectorTests
```

Expected: all connector tests pass.

- [ ] **Step 6: Run full Swift tests**

Run:

```bash
swift test
```

Expected: all Swift tests pass.

- [ ] **Step 7: Commit PR #3 writer change**

```bash
git add apps/cubicle-macos/Sources/Connectors/SignalKnowledgeWriter.swift apps/cubicle-macos/Tests/SignalConnectorTests.swift
git commit -m "Make signal batch writes atomic"
```

## Task 3: README DAGs

**Files:**
- Modify: `README.md`

- [ ] **Step 1: Replace architecture sections with three DAGs**

Add:

```text
Architecture DAG
Runtime Call Flow DAG
Filename Call Flow DAG
```

Use current filenames and keep prose brief.

- [ ] **Step 2: Run markdown diff check**

Run:

```bash
git diff -- README.md
```

Expected: README diagrams are accurate, concise, and ASCII-only.

- [ ] **Step 3: Commit README update**

```bash
git add README.md docs/superpowers/plans/2026-06-01-atomic-signal-db-writes.md
git commit -m "Document architecture call flow"
```

## Task 4: Final Verification and Push

**Files:**
- No code changes.

- [ ] **Step 1: Run final checks**

```bash
git diff --check
swift test
```

Expected: no whitespace errors; all Swift tests pass.

- [ ] **Step 2: Push PR #2 and PR #3**

```bash
git checkout chore/knowledge-dao-refactor
git push origin chore/knowledge-dao-refactor
git checkout feat/signal-connectors
git push origin feat/signal-connectors
```

Expected: both branches push cleanly or use `--force-with-lease` only if a rebase rewrote history.

---

## Self-Review

- Spec coverage: transaction primitive, signal writer atomicity, README architecture/call-flow DAGs, verification, and stacked PR placement are covered.
- Placeholder scan: no TBD/TODO/fill-in steps.
- Type consistency: uses existing `RoomRecord`, `PersonRecord`, `MessageRecord`, `BeliefEvidenceRecord`, `SignalSyncBatch`, `SignalEvent`, `MessageEventPayload`, and `KnowledgeStore` names.
