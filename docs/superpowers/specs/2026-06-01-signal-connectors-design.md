# Signal Connectors Design Spec

Date: 2026-06-01

## Thesis

Cubicle should ingest engineering work signals through a source-neutral connector layer.
The layer must support Webex and iMessage now, and Slack, Google Docs, Drive, Jira,
GitHub, Calendar, CI, and similar systems later.

The core abstraction is not a chat connector. It is a signal connector.

```text
source-specific APIs
  -> SignalConnector
  -> SignalSyncBatch
  -> common processing pipeline
  -> knowledge graph
  -> engineer insights and safe writeback
```

## Product Goal

The product goal is to reduce TPM and program-manager coordination work by giving
engineers evidence-backed operational insight:

```text
what changed
  -> what matters
  -> what is blocked
  -> who owns it
  -> what decision is pending
  -> what action should happen next
```

Connectors are the source acquisition layer for that product. They must preserve
source fidelity, permissions, provenance, and incremental sync state.

## Current Problem

The current code has Webex, iMessage, processing, and persistence responsibilities
mixed together.

```text
AppModel
  -> NativeRefreshCoordinator
      -> NativeWebexIngestionService
          -> WebexAPIClient
          -> WebexSyncEngine
          -> NativeIMessageIngesting
          -> KnowledgeStore
```

`NativeWebexIngestionService` is currently a facade, Webex connector, iMessage
orchestrator, focus timeline builder, and knowledge writer. This makes it hard to
add Slack, Docs, Drive, or Jira without copying source-specific assumptions into
product logic.

The refactor should introduce a connector substrate without changing user-visible
behavior in the first pass.

## Non-Goals

This spec does not include transcription work.

This spec does not require a new database schema in the first implementation pass.
Schema changes may be useful later, but the initial migration should map normalized
signals into the existing knowledge records.

This spec does not implement Slack, Google, Jira, or GitHub connectors. It designs
the interface so those connectors can be added without another architecture rewrite.

This spec does not move Swift source to a repo-root `connectors/` directory. The
compiled Swift connector code should live under:

```text
apps/cubicle-macos/Sources/Connectors
```

The current SwiftPM target compiles `apps/cubicle-macos/Sources`. A repo-root
`connectors/` folder would need package restructuring before Swift could compile it.

## Recommended Architecture

```text
NativeRefreshCoordinator
  -> SignalSyncOrchestrator
      -> TargetRouter
          -> WebexConnector
          -> IMessageConnector
          -> SlackConnector later
          -> GoogleDocsConnector later
          -> GoogleDriveConnector later
          -> JiraConnector later
      -> ConnectorCheckpointStore
      -> SignalNormalizer
      -> SignalKnowledgeWriter
          -> KnowledgeStore
  -> FocusSnapshotBuilder
  -> QuestionEngine
  -> CodexPromptOrchestration
```

### Responsibilities

```text
SignalConnector
  -> talks to one external/local source
  -> returns source-faithful signals
  -> owns API pagination and source-specific cursors
  -> does not decide product meaning
  -> does not write to KnowledgeStore

SignalSyncOrchestrator
  -> schedules connector syncs
  -> passes targets and checkpoints
  -> handles connector availability and partial failure

TargetRouter
  -> maps product targets to source selectors
  -> decides which connectors should receive each target

SignalNormalizer
  -> converts source-native signals into canonical objects, events, relations,
     and content chunks

SignalKnowledgeWriter
  -> performs dedupe and idempotent writes
  -> maps normalized signals into existing KnowledgeStore records
  -> owns cross-source source IDs

FocusSnapshotBuilder
  -> builds person/space timelines from stored knowledge
  -> no longer calls Webex or iMessage directly
```

## Core Connector Contract

Swift should use protocols rather than abstract classes.

```swift
protocol SignalConnector {
    var descriptor: ConnectorDescriptor { get }

    func sync(
        request: SignalSyncRequest,
        checkpoint: ConnectorCheckpoint?
    ) async throws -> SignalSyncBatch
}
```

This is deliberately sync-oriented, not `fetchEvents(since:limit:)`, because real
connectors need checkpoints, backfill, retries, permission state, and partial failure.

```swift
struct ConnectorDescriptor: Hashable {
    var id: ConnectorID
    var displayName: String
    var capabilities: Set<ConnectorCapability>
}

struct SignalSyncRequest: Hashable {
    var runID: UUID
    var mode: SignalSyncMode
    var targets: [SignalTarget]
    var startedAt: Date
    var limit: Int
}

struct SignalSyncBatch: Hashable {
    var connectorID: ConnectorID
    var accountID: String
    var objects: [SignalObject]
    var events: [SignalEvent]
    var relations: [SignalRelation]
    var content: [SignalContentChunk]
    var checkpoint: ConnectorCheckpoint?
    var warnings: [ConnectorWarning]
    var availability: ConnectorAvailability
}
```

## Target Model

Targets must not be shaped around Webex room IDs and iMessage handles. A target is
a product entity with source-specific selectors.

```text
SignalTarget
  -> canonical entity
      -> person
      -> space
      -> project
      -> repository
      -> document
  -> selectors
      -> webex.roomID
      -> webex.email
      -> imessage.handle
      -> slack.channelID
      -> slack.userID
      -> google.docID
      -> google.driveFileID
      -> jira.projectKey
      -> jira.issueKey
      -> github.repo
```

Example:

```swift
struct SignalTarget: Hashable {
    var id: String
    var label: String
    var entityKind: SignalEntityKind
    var selectors: [ConnectorSelector]
}

struct ConnectorSelector: Hashable {
    var connectorID: ConnectorID
    var kind: String
    var value: String
}
```

This lets one person target resolve to Webex email, iMessage handles, Slack user ID,
Jira assignee, and Google document ownership without adding fields to the base type.

## Signal Model

The shared batch should carry four categories:

```text
objects
  -> durable things: person, channel, document, file, issue, PR, project

events
  -> changes over time: message sent, issue assigned, doc commented, file shared

relations
  -> graph edges: person owns doc, issue blocks issue, message mentions person

content
  -> searchable text chunks with provenance and permissions
```

### Object Envelope

```swift
struct SignalObject: Hashable {
    var id: GlobalSignalID
    var sourceID: SourceObjectID
    var kind: SignalObjectKind
    var title: String
    var url: URL?
    var createdAt: Date?
    var updatedAt: Date?
    var visibility: SignalVisibility
    var properties: SignalProperties
}
```

### Event Envelope

```swift
struct SignalEvent: Hashable {
    var id: GlobalSignalID
    var sourceID: SourceEventID
    var kind: SignalEventKind
    var actor: SignalActor?
    var occurredAt: Date
    var objectIDs: [GlobalSignalID]
    var visibility: SignalVisibility
    var payload: SignalEventPayload
}
```

Payloads should be typed. Avoid a generic junk drawer of string metadata.

```swift
enum SignalEventPayload: Hashable {
    case message(MessageEventPayload)
    case issueStatusChanged(IssueStatusChangedPayload)
    case issueAssigned(IssueAssignedPayload)
    case documentCommented(DocumentCommentPayload)
    case documentEdited(DocumentEditedPayload)
    case fileShared(FileSharedPayload)
    case pullRequestReviewed(PullRequestReviewPayload)
}
```

### Relation Envelope

```swift
struct SignalRelation: Hashable {
    var id: GlobalSignalID
    var kind: SignalRelationKind
    var fromID: GlobalSignalID
    var toID: GlobalSignalID
    var observedAt: Date
    var sourceEventID: GlobalSignalID?
    var confidence: SignalConfidence
    var visibility: SignalVisibility
}
```

### Content Chunk Envelope

```swift
struct SignalContentChunk: Hashable {
    var id: GlobalSignalID
    var objectID: GlobalSignalID
    var sourceID: SourceObjectID
    var text: String
    var chunkIndex: Int
    var contentHash: String
    var observedAt: Date
    var visibility: SignalVisibility
}
```

## Global Identity

Source IDs must be globally scoped. Current Webex message IDs can map directly into
`MessageRecord.id`, but that will not hold across all sources.

```text
GlobalSignalID
  -> connectorID + accountID + sourceKind + externalID
```

Examples:

```text
webex:prabhat:webex-message:abc123
imessage:local:message:chat42-99
slack:T123:message:C456-1717000000.123
google-drive:workspace:file:1abc
jira:company:issue:ENG-42
github:org:pull-request:repo-184
```

This gives idempotent writes and allows cross-source references without collisions.

## Permissions And Visibility

Permissions are part of the graph. Every object, event, relation, and content chunk
must carry visibility metadata.

```text
SignalVisibility
  -> source system
  -> source account/workspace
  -> access groups or principals when available
  -> visibility confidence
```

Initial local macOS implementation may store simple visibility:

```text
local-user-only
webex-authenticated-user
imessage-local-user
```

Future enterprise connectors need richer access maps so the system does not surface
private documents, restricted Jira issues, or private Slack channels to the wrong user.

## Connector Fit Matrix

```text
Source        Objects                  Events                         Content
Webex         rooms, people            messages, memberships          message text
iMessage      chats, handles           messages                       message text
Slack         channels, users          messages, threads, mentions    message text
Google Docs   docs, comments           edits, comments, suggestions   doc chunks
Google Drive  files, folders, owners   shares, moves, updates         extracted text
Jira          issues, projects, epics   status, assign, comment        issue text
GitHub        repos, PRs, issues       review, commit, CI             PR/issue text
Calendar      meetings, attendees      scheduled, joined, changed     agenda/notes
```

The model must support both event-heavy systems and content-heavy systems.

## Source Examples

### Slack

```text
SlackConnector
  -> objects: channel, user, thread
  -> events: message, reply, reaction, mention
  -> relations: message mentions person, thread links document
  -> content: message text
```

### Google Docs

```text
GoogleDocsConnector
  -> objects: document, comment, suggestion
  -> events: document edited, comment added, suggestion resolved
  -> relations: document owned by person, comment mentions issue
  -> content: document chunks, comment text
```

### Google Drive

```text
GoogleDriveConnector
  -> objects: file, folder, shared drive
  -> events: file shared, moved, updated, permission changed
  -> relations: file in folder, file owned by person, file references issue
  -> content: extracted text for supported file types
```

### Jira

```text
JiraConnector
  -> objects: issue, project, epic, sprint
  -> events: status changed, assigned, commented, linked
  -> relations: issue blocks issue, issue belongs to epic, issue owned by person
  -> content: issue description, comments
```

## Processing Pipeline

```text
SignalSyncBatch
  -> validate global IDs
  -> validate visibility
  -> dedupe by global ID and content hash
  -> resolve people and aliases
  -> extract cross-source links
  -> write objects/events/relations/content
  -> update checkpoints only after successful writes
```

Checkpoint update must be atomic relative to processed writes. If the app crashes
after fetching but before writing, the next sync should replay safely.

## Error Handling

Connector failures must be represented without collapsing the entire refresh.

```text
ConnectorAvailability
  -> available
  -> unavailable
  -> permissionDenied
  -> authRequired
  -> rateLimited
  -> partial
```

Examples:

```text
Webex rate-limited
  -> keep previous checkpoint
  -> emit warning
  -> set next allowed sync

iMessage database unavailable
  -> mark connector unavailable
  -> keep Webex sync working

Google Docs permission denied on one document
  -> emit warning for that object
  -> continue processing accessible docs

Jira API page failure after partial results
  -> write processed page results if checkpoint semantics are safe
  -> otherwise discard partial page and retry later
```

## Migration Strategy

The migration should be incremental.

```text
Phase 1: substrate
  -> add Connectors/SignalConnector.swift
  -> add Connectors/SignalModels.swift
  -> add mock connector tests
  -> no behavior change

Phase 2: iMessage adapter
  -> move iMessage code to Connectors/IMessage
  -> emit MessageEventPayload
  -> keep NativeIMessageIngesting compatibility shim if needed

Phase 3: Webex adapter
  -> move Webex connector-facing code to Connectors/Webex
  -> keep WebexSyncEngine behavior
  -> adapt Webex messages into SignalSyncBatch

Phase 4: knowledge writer
  -> introduce SignalKnowledgeWriter
  -> centralize idempotent writes into KnowledgeStore
  -> stop new connector code from writing directly to KnowledgeStore

Phase 5: focus timeline extraction
  -> move person timeline merging into FocusSnapshotBuilder
  -> remove direct iMessage calls from NativeWebexIngestionService

Phase 6: facade shrink
  -> keep NativeWebexIngestionService only as a compatibility facade
  -> route refresh through SignalSyncOrchestrator
```

This keeps the product stable while changing ownership boundaries.

## Testing Strategy

```text
Unit tests
  -> mock SignalConnector emits objects/events/relations/content
  -> SignalKnowledgeWriter writes idempotently
  -> TargetRouter routes selectors to the right connector
  -> checkpoint only advances after successful processing

Integration tests
  -> iMessage test database still loads messages
  -> Webex sync tests keep current behavior
  -> person focus cache includes Webex + iMessage messages
  -> partial connector failure does not fail unrelated connector sync

Regression tests
  -> duplicate source IDs do not duplicate knowledge rows
  -> global IDs prevent cross-source collision
  -> permission-denied connector result is visible as connector status
```

Verification commands for implementation work:

```bash
swift build
swift test
```

## Adversarial Review

### Finding 1: The interface can become too abstract to implement.

Risk:

```text
SignalConnector
  -> objects/events/relations/content
```

This is powerful, but each connector author may interpret it differently.

Mitigation:

```text
1. Start with only MessageEventPayload for Webex and iMessage.
2. Add Jira/Docs payloads only when implementing those connectors.
3. Keep payload enums typed and reviewed.
4. Add connector contract tests.
```

### Finding 2: `properties` can become the new junk drawer.

Risk: typed payloads help, but `SignalObject.properties` can silently recreate
`metadata: [String: String]`.

Mitigation: restrict `SignalProperties` to scalar values, require stable keys with
source prefixes, and promote repeated product-critical fields into typed payloads
or first-class object fields.

### Finding 3: Permissions are easy to underbuild.

Risk: the first app is local and single-user, so visibility may be treated as a
thin default. That would block enterprise use later.

Mitigation: require a non-empty `SignalVisibility` value in every signal type from
day one. The initial implementation can use simple local visibility values, but
the field must exist and be validated.

### Finding 4: Webex may keep bypassing the common pipeline.

Risk: if `WebexSyncEngine` keeps writing directly to `KnowledgeStore`, the new
connector layer is only cosmetic.

Mitigation: allow a short transitional adapter, but make `SignalKnowledgeWriter`
the only writer for new connector implementations. Add a follow-up task to move
Webex message persistence behind the writer.

### Finding 5: Checkpoint semantics are source-specific.

Risk: a single checkpoint shape may not fit Webex polling, Slack cursors, Google
Drive page tokens, Jira updated timestamps, and local iMessage SQLite offsets.

Mitigation: model `ConnectorCheckpoint` as an opaque connector-owned payload with
common wrapper fields:

```text
connectorID
accountID
updatedAt
payload
```

The orchestrator stores checkpoints but does not interpret connector internals.

### Finding 6: Drive and Docs are content-heavy, not event-heavy.

Risk: a message/event-first model will work for Slack/Webex but fail for Docs and
Drive.

Mitigation: make `SignalContentChunk` first-class in `SignalSyncBatch`. Docs and
Drive can emit content with object metadata even when no meaningful event exists.

### Finding 7: Relations require identity resolution that may be wrong.

Risk: extracting "doc references Jira ticket" or "message mentions person" can
create false links.

Mitigation: relations need provenance and confidence when inferred. Direct source
relations can be high confidence. Extracted links should include evidence source
and confidence in `SignalRelation`.

### Finding 8: This can overreach the current product.

Risk: building the full signal substrate before Webex/iMessage are clean can stall
the product.

Mitigation: implement the smallest vertical slice:

```text
Webex + iMessage
  -> MessageEventPayload
  -> SignalKnowledgeWriter
  -> existing focus/person views unchanged
```

Leave Slack, Docs, Drive, and Jira as design constraints, not implementation scope.

## Open Decisions

The next implementation plan should choose whether `SignalKnowledgeWriter` maps
only to existing `PersonRecord`, `RoomRecord`, `MessageRecord`, and
`BeliefEvidenceRecord`, or also introduces append-only raw signal tables.

Recommendation: first map to existing records and defer raw signal tables until a
second persistence-focused design. This avoids mixing connector refactor and schema
expansion in one PR.

## Approval Gate

This spec is ready for implementation planning when the team agrees on three
decisions:

```text
1. Use SignalConnector as the base protocol name.
2. Put compiled Swift connectors under apps/cubicle-macos/Sources/Connectors.
3. Keep the first implementation scoped to Webex + iMessage behavior preservation.
```
