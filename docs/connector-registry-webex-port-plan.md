# Connector Registry And Webex Port Plan

## Registry Slice

```text
SignalConnectorFactory
 |
 +-- * SignalConnectorRegistry
 |     -> stable ConnectorID lookup
 |
 +-- WebexConnectorProvider
 |     -> WebexSignalConnector + WebexProductService
 |
 +-- IMessageConnectorProvider
       -> IMessageSignalConnector + IMessageProductService
```

## Webex Production Port

```text
Target state
 |
 +-- * SignalSyncPipeline
 |     -> loads per-target checkpoints before connector sync
 |     -> writes signal batch
 |     -> saves checkpoints only after write succeeds
 |
 +-- * WebexSignalConnector
 |     -> owns Webex cursor/backoff decisions
 |     -> emits normalized signal rows
 |
 +-- * ConnectorCheckpointDAO
       -> stores per-room/per-person cursor + retry state
```

## Implemented Checkpoint Slice

```text
SignalSyncPipeline
 |
 +-- * SignalCheckpointStore
 |     -> loads connector checkpoints by routed target/selector scope
 |
 +-- SignalConnector.sync(...)
 |     -> receives ConnectorCheckpointSet
 |
 +-- SignalKnowledgeWriter.write(batch)
 |     -> writes normalized rows first
 |
 +-- * SignalCheckpointStore.save(batch.checkpoints)
       -> advances cursor/backoff only after write succeeds
```

## Implemented Webex Connector Engine Slice

```text
WebexSignalConnector
 |
 +-- * WebexSyncEngine
 |     -> latest-message probe, catch-up, backoff, single-flight
 |
 +-- * WebexSignalSyncStateStore
 |     -> loads ConnectorCheckpointSet / legacy webex_sync_state fallback
 |     -> records pending state as connector checkpoints
 |
 +-- * WebexSignalBatchMessageProcessor
 |     -> accumulates SignalObject + SignalEvent rows
 |
 v
SignalSyncPipeline
 |
 +-- writes batch
 +-- saves returned checkpoints after write succeeds
```

## Adversarial Review

```text
Risk: one connector-level checkpoint
 |
 +-- Webex needs per conversation state
 |     -> room cursor
 |     -> direct-message discovery
 |     -> next_allowed_at
 |
 v
Covered by ConnectorCheckpointSet target/key contract.
```

```text
Risk: cursor advances before DB write
 |
 +-- connector returns batch + checkpoint
 +-- writer fails
 +-- checkpoint saved anyway
 |
v
Covered by checkpoint pipeline tests.
```

```text
Risk: duplicating WebexSyncEngine
 |
 +-- two implementations diverge
 +-- rate-limit/backoff tests cover old path only
 |
 v
Covered by WebexSignalConnector delegating to WebexSyncEngine.
```

## Release Gate

```text
safe to merge registry
 |
 +-- factory switch removed
 +-- providers are tested
 +-- production Webex behavior unchanged

not safe to switch Webex yet
 |
 +-- NativeRefreshCoordinator still uses NativeWebexIngestionService
 +-- focus snapshots/status badges still read legacy Webex state
 +-- production cutover needs snapshot/status bridge
```
