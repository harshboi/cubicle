# Connector Factory + Product Services Design

## Goal

```text
Cubicle connector layer
  -> make ingestion generic
  -> keep product-specific source behavior isolated
  -> keep AppModel out of Webex/iMessage/Slack/Jira branching
  -> prepare for Slack, Google Docs, Drive, Jira, GitHub, Linear
```

```text
Primary outcome
  -> AppModel asks for product actions
  -> services choose connector behavior
  -> connectors emit normalized knowledge
  -> DAOs persist product-level context
```

## What / Why / How

```text
What
  -> connector factory + small Swift protocols + product-specific services
```

```text
Why
  -> current ingestion is Webex-shaped
  -> future Slack/Jira/Drive should not force AppModel rewrites
  -> DB should store product knowledge, not source orchestration
```

```text
How
  -> wrap Webex and iMessage first
  -> route generic refresh through ConnectorProcessingService
  -> keep Webex rooms/iMessage chats in product services
  -> write normalized batches through SignalKnowledgeWriter
```

## Non-Goals

```text
This design does not center
  -> transcription
  -> remote backend service extraction
  -> full schema rewrite
  -> replacing WebexSyncEngine immediately
```

## Current Shape

```text
Current macOS app ingestion path

AppModel.swift *
  -> UI-facing app controller and refresh trigger
        |
        v
NativeRefreshCoordinator.swift *
  -> selects refresh scope and calls ingestion services
        |
        v
NativeWebexIngestionService.swift *
  -> Webex map/snapshot/sync orchestration, plus iMessage timeline assembly
        |
        +-- WebexSyncEngine.swift *
        |     -> Webex polling, cursors, backoff, message indexing
        |
        +-- WebexAPIClient.swift
        |     -> Webex HTTP client
        |
        +-- NativeIMessageIngestionService.swift
        |     -> local iMessage DB reads for person timelines
        |
        v
KnowledgeStore.swift *
  -> SQLite schema, writes, reads, Webex sync state, questions, beliefs
```

```text
Current problem
  -> NativeWebexIngestionService is product-shaped around Webex
  -> iMessage is nested under Webex/person timeline workflows
  -> KnowledgeStore contains generic knowledge plus Webex sync state
  -> adding Slack/Jira/Drive will add more source-specific branches
```

## Target Shape

```text
Target connector architecture

AppModel.swift *
  -> calls app-level use cases, no connector casting
        |
        v
ConnectorProcessingService.swift *
  -> common refresh/backfill/error/write orchestration
        |
        v
ConnectorFactory.swift *
  -> constructs connector implementations and product services
        |
        +-- WebexConnector.swift *
        |     -> Webex rooms/messages/memberships/API sync
        |
        +-- IMessageConnector.swift *
        |     -> local iMessage chats/messages/handle matching
        |
        +-- SlackConnector.swift
        |     -> future channels/threads/messages
        |
        +-- JiraConnector.swift
        |     -> future projects/issues/status changes
        |
        +-- DriveConnector.swift
              -> future docs/files/comments/activity
        |
        v
SignalKnowledgeWriter.swift *
  -> normalized batch writes into KnowledgeStore/DAO
        |
        v
KnowledgeStore / DAO *
  -> product knowledge, not connector orchestration
```

## Side-By-Side

```text
Current                                     Target
=======                                     ======

AppModel.swift *                            AppModel.swift *
  -> Webex/iMessage-flavored calls            -> app-level actions only
        |                                            |
        v                                            v
NativeRefreshCoordinator.swift *            ConnectorProcessingService.swift *
  -> refresh scope routing                    -> generic sync orchestration
        |                                            |
        v                                            v
NativeWebexIngestionService.swift *         ConnectorFactory.swift *
  -> Webex-first ingestion owner              -> source construction owner
        |                                            |
        +-- WebexSyncEngine.swift                    +-- WebexConnector.swift *
        +-- WebexAPIClient.swift                     +-- IMessageConnector.swift *
        +-- NativeIMessageIngestionService           +-- Slack/Jira/Drive later
        |                                            |
        v                                            v
KnowledgeStore.swift *                       SignalKnowledgeWriter.swift *
  -> generic + Webex state mixed              -> normalized write boundary
                                                     |
                                                     v
                                             KnowledgeStore / DAO *
                                               -> product knowledge only
```

## Core Interfaces

```text
SignalConnector *
  -> minimum contract every source must support
        |
        +-- id
        +-- displayName
        +-- capabilities
        +-- sync(request) -> ConnectorSyncResult
```

```swift
protocol SignalConnector {
    var id: ConnectorID { get }
    var displayName: String { get }
    var capabilities: Set<ConnectorCapability> { get }

    func sync(request: ConnectorSyncRequest) async throws -> ConnectorSyncResult
}
```

```text
ConnectorSyncRequest
  -> tells a connector what work to do
        |
        +-- mode                 -> full, incremental, targeted
        +-- targetIDs            -> source-specific IDs wrapped as strings
        +-- since                -> optional lower bound
        +-- limit                -> bounded fetch size
        +-- reason               -> startup, manual, scheduled, page-priority
```

```text
ConnectorSyncResult
  -> normalized payload back to Cubicle
        |
        +-- batch                -> people/conversations/messages/files/evidence
        +-- checkpointUpdates    -> source cursors/watermarks
        +-- diagnostics          -> user-visible sync summaries
        +-- partialFailures      -> isolated target failures
```

## Capability Interfaces

Use small protocols instead of one huge abstract class.

```text
Connector capabilities
  |
  +-- ConversationListingConnector
  |     -> rooms/chats/channels for settings and target selection
  |
  +-- OAuthBackedConnector
  |     -> connect/revoke/status for sources with OAuth
  |
  +-- FileSourceConnector
  |     -> docs/files/folders/activity for Drive/Docs
  |
  +-- IssueTrackerConnector
        -> projects/issues/status/assignees for Jira/Linear/GitHub
```

```text
WebexConnector *
  -> SignalConnector
  -> ConversationListingConnector
  -> OAuthBackedConnector

IMessageConnector *
  -> SignalConnector
  -> ConversationListingConnector

SlackConnector
  -> SignalConnector
  -> ConversationListingConnector
  -> OAuthBackedConnector

JiraConnector
  -> SignalConnector
  -> IssueTrackerConnector
  -> OAuthBackedConnector

DriveConnector
  -> SignalConnector
  -> FileSourceConnector
  -> OAuthBackedConnector
```

## Factory Design

```text
ConnectorFactory.swift *
  -> construction only; no sync orchestration
        |
        +-- makeConnector(.webex)    -> any SignalConnector
        +-- makeConnector(.imessage) -> any SignalConnector
        +-- makeWebexProductService()
        +-- makeIMessageProductService()
```

```text
Factory inputs
  |
  +-- RuntimeConfiguration.swift
  |     -> paths, API base URLs, runtime root
  |
  +-- ConfigStore.swift
  |     -> settings, targets, OAuth config
  |
  +-- OAuthKeychainStore.swift
  |     -> source secrets
  |
  +-- KnowledgeStore / DAO
        -> writer target and checkpoint access
```

## Downcasting Rule

```text
Rule
  -> downcast only where the code is already source-specific
  -> prefer concrete product services when construction can avoid casting
```

```text
Allowed
  |
  +-- WebexProductService.swift *
  |     -> Webex-only rooms, memberships, target setup, sync health
  |
  +-- IMessageProductService.swift *
  |     -> iMessage-only chats, DB access checks, handle matching
  |
  +-- ConnectorCapabilityResolver.swift
        -> generic code checking optional capabilities
```

```text
Avoid
  |
  +-- AppModel.swift *
  |     -> should not know private connector implementation types
  |
  +-- SwiftUI Views *
  |     -> should render state, not branch on source internals
  |
  +-- KnowledgeStore / DAO *
        -> should persist normalized knowledge, not call source APIs
```

```text
Best construction path

ConnectorFactory.swift *
  -> webexConnector = WebexConnector(...)
  -> WebexProductService(connector: webexConnector)
        |
        v
WebexProductService.swift *
  -> concrete WebexConnector, no downcast needed
```

```text
Acceptable fallback

WebexProductService.swift *
  -> receives any SignalConnector
  -> guards connector as? WebexConnector
  -> throws configuration error if wrong type
```

## Generic Sync Call Flow

```text
Startup/manual/background refresh
  |
  v
AppModel.swift *
  -> refreshNow() / runRefreshCycle()
  |
  v
ConnectorProcessingService.swift *
  -> refresh(source: .webex, mode: .incremental)
  |
  v
ConnectorFactory.swift *
  -> makeConnector(.webex)
  |
  v
WebexConnector.swift *
  -> internally uses WebexSyncEngine/WebexAPIClient at first
  |
  v
ConnectorSyncResult
  -> normalized batch + checkpoint updates + diagnostics
  |
  v
SignalKnowledgeWriter.swift *
  -> atomic batch write
  |
  v
KnowledgeStore / DAO *
  -> people, conversations, messages, files, evidence
  |
  v
AppModel.swift *
  -> reload focus/questions/beliefs/UI caches
```

## Webex Product Flow

```text
Settings target picker
  |
  v
SettingsView.swift
  -> user asks for Webex rooms
  |
  v
AppModel.swift *
  -> loadWebexRoomsForSettings()
  |
  v
WebexProductService.swift *
  -> listRooms()
  -> listMemberships()
  -> sync health
  |
  v
WebexConnector.swift *
  -> WebexAPIClient.swift
  |
  v
Webex API
```

```text
Boundary
  -> AppModel can expose "webex rooms" UI state
  -> AppModel should not cast "any SignalConnector as WebexConnector"
```

## iMessage Product Flow

```text
Person target setup
  |
  v
SettingsView.swift
  -> user adds iMessage handle
  |
  v
AppModel.swift *
  -> validateIMessageHandle()
  |
  v
IMessageProductService.swift *
  -> inspect chat DB access
  -> normalize handles
  -> preview matching chats/messages
  |
  v
IMessageConnector.swift *
  -> NativeIMessageIngestionService.swift initially
  |
  v
local iMessage database
```

## Storage Boundary

```text
Connector-specific state
  -> cursor, watermark, pagination token, rate limit, backoff
  -> belongs in connector checkpoint DAO
```

```text
Product knowledge
  -> people, conversations, messages, files, evidence, questions, beliefs
  -> belongs in KnowledgeStore / DAO
```

```text
Target storage after DAO refactor

Data layer
  |
  +-- * KnowledgeDatabase.swift        -> SQLite open/path/migration runner
  |
  +-- * KnowledgeStore.swift           -> product-level facade
  |
  +-- CoreKnowledgeDAO.swift           -> people/conversations/messages/files/evidence
  |
  +-- ConnectorCheckpointDAO.swift *   -> generic connector cursors/checkpoints
  |
  +-- WebexSyncStateDAO.swift          -> optional compatibility adapter
  |
  +-- QuestionDAO.swift                -> question candidates
  |
  +-- BeliefDAO.swift                  -> beliefs and evidence links
```

```text
Compatibility path
  -> keep current webex_sync_state table readable
  -> add generic connector_checkpoints table
  -> migrate Webex reads/writes behind WebexSyncStateDAO adapter
  -> later collapse adapter once callers use ConnectorCheckpointDAO
```

## Suggested Files

```text
apps/cubicle-macos/Sources
  |
  +-- Connectors
  |     |
  |     +-- * ConnectorID.swift                 -> source identity enum
  |     +-- * SignalConnector.swift             -> generic sync contract
  |     +-- * ConnectorSyncModels.swift         -> request/result/batch/checkpoint models
  |     +-- ConnectorCapabilities.swift         -> optional small protocols
  |     +-- ConnectorFactory.swift              -> connector/product service construction
  |     |
  |     +-- Webex
  |     |     |
  |     |     +-- * WebexConnector.swift        -> Webex SignalConnector implementation
  |     |     +-- * WebexProductService.swift   -> rooms/memberships/Webex settings support
  |     |
  |     +-- IMessage
  |           |
  |           +-- * IMessageConnector.swift     -> iMessage SignalConnector implementation
  |           +-- IMessageProductService.swift  -> iMessage handle/chat support
  |
  +-- Services
        |
        +-- * ConnectorProcessingService.swift  -> common refresh/backfill pipeline
        +-- * SignalKnowledgeWriter.swift       -> normalized writes into KnowledgeStore/DAO
```

## Migration Plan

```text
Phase 1: types only
  -> add connector protocols and normalized models
  -> no behavior change
  -> compile proves boundaries fit Swift
```

```text
Phase 2: Webex wrapper
  -> WebexConnector wraps WebexSyncEngine/WebexAPIClient
  -> WebexProductService owns rooms/memberships/settings methods
  -> AppModel stops constructing Webex-only dependencies directly where possible
```

```text
Phase 3: iMessage wrapper
  -> IMessageConnector wraps NativeIMessageIngestionService
  -> IMessageProductService owns handle/chat lookup workflows
  -> iMessage no longer hangs off Webex ingestion naming
```

```text
Phase 4: common processing
  -> ConnectorProcessingService runs generic refresh flow
  -> SignalKnowledgeWriter owns normalized DB batch write
  -> NativeRefreshCoordinator delegates connector refreshes
```

```text
Phase 5: storage cleanup
  -> move connector state behind ConnectorCheckpointDAO
  -> keep Webex sync compatibility while callers migrate
  -> leave product knowledge queries stable
```

```text
Phase 6: next connector
  -> implement Slack or Jira as proof of extensibility
  -> do not declare abstraction done until second non-Webex connector fits
```

## Testing Plan

```text
Unit tests
  |
  +-- FakeSignalConnector
  |     -> emits people/conversations/messages/evidence
  |
  +-- ConnectorProcessingServiceTests *
  |     -> success writes batch once
  |     -> partial failure preserves successful target diagnostics
  |     -> connector error does not corrupt existing knowledge
  |
  +-- SignalKnowledgeWriterTests *
  |     -> atomic batch write
  |     -> duplicate source message is idempotent
  |     -> source metadata survives normalization
  |
  +-- WebexProductServiceTests
  |     -> room listing uses concrete Webex connector path
  |
  +-- IMessageProductServiceTests
        -> handle normalization and DB unavailable path
```

```text
Integration smoke
  -> swift test
  -> swift run Cubicle --help or compile app target
  -> manual Webex sync with existing runtime root
  -> manual iMessage handle lookup if local DB permission exists
```

## Adversarial Review

```text
Attack: "This is fake abstraction; Webex still dominates."
  |
  +-- Evidence
  |     -> WebexSyncEngine remains Webex-specific
  |     -> Webex sync state exists today in KnowledgeStore
  |
  +-- Mitigation
        -> wrapper first, replacement later
        -> require second connector proof before broadening interface
```

```text
Attack: "SignalConnector becomes a god protocol."
  |
  +-- Failure mode
  |     -> every new source adds methods to base protocol
  |
  +-- Mitigation
        -> base protocol only syncs
        -> rooms/files/issues/OAuth live in capability protocols
```

```text
Attack: "Downcasting leaks anyway."
  |
  +-- Failure mode
  |     -> AppModel casts to WebexConnector for room-specific methods
  |
  +-- Mitigation
        -> AppModel calls WebexProductService
        -> only product services/capability resolver may cast
        -> prefer concrete product service construction
```

```text
Attack: "Normalized model loses source detail."
  |
  +-- Failure mode
  |     -> Slack threads, Jira status, Drive comments flatten badly
  |
  +-- Mitigation
        -> normalized core fields for product queries
        -> sourceMetadata JSON for source-native detail
        -> typed product services for source-specific UI
```

```text
Attack: "Generic checkpoint DAO cannot represent every source."
  |
  +-- Failure mode
  |     -> Jira pagination, Slack cursors, Drive delta tokens differ
  |
  +-- Mitigation
        -> key/value checkpoint rows scoped by connector + target + key
        -> structured metadata JSON for source-specific payloads
        -> source adapters own interpretation
```

```text
Attack: "Factory hides dependencies and makes tests harder."
  |
  +-- Failure mode
  |     -> tests need real OAuth/config/DB to build connectors
  |
  +-- Mitigation
        -> factory is composition root only
        -> services accept protocols/concrete connectors in init
        -> tests inject fake connectors directly
```

```text
Attack: "Common processing cannot handle non-message sources."
  |
  +-- Failure mode
  |     -> Jira/Drive do not map cleanly to chat messages
  |
  +-- Mitigation
        -> ConnectorSyncBatch supports multiple artifact kinds
        -> message-like events become evidence
        -> file/issue records get dedicated normalized types
```

```text
Attack: "This is too much refactor before product value."
  |
  +-- Failure mode
  |     -> many files move, little visible behavior changes
  |
  +-- Mitigation
        -> sequence in wrapper phases
        -> keep NativeRefreshCoordinator working throughout
        -> use Slack/Jira proof as the value gate
```

## Design Decisions

```text
Use Swift protocols over abstract class
  -> Swift has protocols as the natural abstraction
  -> avoids inheritance tree pressure
  -> supports small capability surfaces
```

```text
Use factory for construction only
  -> centralizes config/OAuth/store wiring
  -> keeps orchestration in ConnectorProcessingService
```

```text
Use product services for source-specific operations
  -> Webex rooms stay Webex-aware
  -> iMessage chat DB inspection stays iMessage-aware
  -> AppModel stays product-controller, not connector switchboard
```

```text
Use normalized writer between connectors and DB
  -> connectors do not own persistence details
  -> KnowledgeStore/DAO does not call external APIs
```

## Success Criteria

```text
Done enough
  |
  +-- AppModel has no `as? WebexConnector` or `as? IMessageConnector`
  |
  +-- Webex and iMessage both implement SignalConnector
  |
  +-- ConnectorProcessingService can run a fake connector in tests
  |
  +-- SignalKnowledgeWriter writes one normalized batch atomically
  |
  +-- Webex room-specific UI path goes through WebexProductService
  |
  +-- adding Slack does not require editing AppModel refresh branching
```

## Final Recommendation

```text
Build this as a connector substrate, not a source rewrite.
  |
  +-- first wrap Webex/iMessage
  +-- then extract common processing
  +-- then move sync state into connector checkpoints
  +-- then prove with Slack or Jira
```
