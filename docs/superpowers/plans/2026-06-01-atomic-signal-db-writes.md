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
    - Add rollback test proving an evidence primary-key failure rolls back earlier message/room/person writes.
- PR #3:
  - Modify `apps/cubicle-macos/Sources/Connectors/SignalKnowledgeWriter.swift`
    - Collect mapped records from a `SignalSyncBatch`, call `writeConnectorMessageBatch`, and keep summary counts unchanged.
  - Modify `apps/cubicle-macos/Tests/SignalConnectorTests.swift`
    - Add writer-boundary test proving mapped rows are handed to storage as one connector batch.
  - Modify `README.md`
    - Replace broad DAGs with architecture DAG, runtime call-flow DAG, and filename call-flow DAG.

## Task 1: PR #2 DB Transaction Primitive

**Files:**
- Modify: `apps/cubicle-macos/Sources/Data/DAO/KnowledgeStore.swift`
- Test: `apps/cubicle-macos/Tests/QuestionEngineCoreIntegrationTests.swift`

- [x] **Step 1: Write failing rollback test**

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
            evidence: [
                BeliefEvidenceRecord(id: "duplicate-evidence-id", source: "webex_message", sourceID: "message-rollback", roomID: "room-rollback", personID: "person-rollback", occurredAt: now, text: "First evidence row should be rolled back."),
                BeliefEvidenceRecord(id: "duplicate-evidence-id", source: "webex_message", sourceID: "message-rollback-conflict", roomID: "room-rollback", personID: "person-rollback", occurredAt: now, text: "Primary key conflict should roll back prior writes.")
            ]
        )
    )

    XCTAssertNil(try store.loadRoom(roomID: "room-rollback"))
    XCTAssertFalse(try store.messageExists(messageID: "message-rollback"))
    XCTAssertTrue(try store.loadBeliefEvidence(scope: .space, entityKey: "room-rollback").isEmpty)
}
```

- [x] **Step 2: Run test to verify it fails**

Run:

```bash
swift test --filter QuestionEngineCoreIntegrationTests/testKnowledgeStoreConnectorBatchRollsBackOnEvidenceFailure
```

Expected: compile failure because `writeConnectorMessageBatch` does not exist.

- [x] **Step 3: Implement transaction helper**

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

- [x] **Step 4: Run focused test**

Run:

```bash
swift test --filter QuestionEngineCoreIntegrationTests/testKnowledgeStoreConnectorBatchRollsBackOnEvidenceFailure
```

Expected: pass.

- [x] **Step 5: Run PR #2 Swift tests**

Run:

```bash
swift test
```

Expected: all Swift tests pass.

- [x] **Step 6: Commit PR #2 change**

```bash
git add apps/cubicle-macos/Sources/Data/DAO/KnowledgeStore.swift apps/cubicle-macos/Tests/QuestionEngineCoreIntegrationTests.swift
git commit -m "Add atomic connector batch write"
```

## Task 2: PR #3 Atomic Signal Writer

**Files:**
- Modify: `apps/cubicle-macos/Sources/Connectors/SignalKnowledgeWriter.swift`
- Test: `apps/cubicle-macos/Tests/SignalConnectorTests.swift`

- [x] **Step 1: Rebase PR #3 onto updated PR #2**

```bash
git checkout feat/signal-connectors
git rebase chore/knowledge-dao-refactor
```

Expected: clean rebase or only doc comment conflicts.

- [x] **Step 2: Write failing writer boundary test**

Add this test to `SignalConnectorTests`:

```swift
func testSignalKnowledgeWriterWritesMappedRowsAsOneConnectorBatch() throws {
    let knowledgeStore = RecordingSignalKnowledgeStore()
    let writer = SignalKnowledgeWriter(knowledgeStore: knowledgeStore)
    let summary = try writer.write(makeSignalMessageBatch(occurredAt: Date(timeIntervalSince1970: 1_715_000_240)))

    XCTAssertEqual(summary.messageEventsProcessed, 1)
    XCTAssertEqual(summary.evidenceRecordsWritten, 1)
    XCTAssertEqual(knowledgeStore.bootstrapCallCount, 1)
    XCTAssertEqual(knowledgeStore.batchWrites.count, 1)
    XCTAssertEqual(knowledgeStore.batchWrites.first?.messages.count, 1)
    XCTAssertEqual(knowledgeStore.batchWrites.first?.evidence.count, 1)
}
```

- [x] **Step 3: Run test to verify it fails**

Run:

```bash
swift test --filter SignalConnectorTests/testSignalKnowledgeWriterWritesMappedRowsAsOneConnectorBatch
```

Expected: compile failure because `SignalKnowledgeWritableStore` does not exist and the writer requires concrete `KnowledgeStore`.

- [x] **Step 4: Refactor writer to collect rows before writing**

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

- [x] **Step 5: Run focused writer tests**

Run:

```bash
swift test --filter SignalConnectorTests
```

Expected: all connector tests pass.

- [x] **Step 6: Run full Swift tests**

Run:

```bash
swift test
```

Expected: all Swift tests pass.

- [x] **Step 7: Commit PR #3 writer change**

```bash
git add apps/cubicle-macos/Sources/Connectors/SignalKnowledgeWriter.swift apps/cubicle-macos/Tests/SignalConnectorTests.swift
git commit -m "Make signal batch writes atomic"
```

## Task 3: README DAGs

**Files:**
- Modify: `README.md`

- [x] **Step 1: Replace architecture sections with three DAGs**

Add:

```text
Architecture DAG
Runtime Call Flow DAG
Filename Call Flow DAG
```

Use current filenames and keep prose brief.

- [x] **Step 2: Run markdown diff check**

Run:

```bash
git diff -- README.md
```

Expected: README diagrams are accurate, concise, and ASCII-only.

- [x] **Step 3: Commit README update**

```bash
git add README.md docs/superpowers/plans/2026-06-01-atomic-signal-db-writes.md
git commit -m "Document architecture call flow"
```

## Task 4: Final Verification and Push

**Files:**
- No code changes.

- [x] **Step 1: Run final checks**

```bash
git diff --check
swift test
```

Expected: no whitespace errors; all Swift tests pass.

- [x] **Step 2: Push PR #2 and PR #3**

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
