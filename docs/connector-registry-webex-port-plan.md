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
Do not move production Webex until checkpoint contract is per target/key.
```

```text
Risk: cursor advances before DB write
 |
 +-- connector returns batch + checkpoint
 +-- writer fails
 +-- checkpoint saved anyway
 |
 v
Pipeline must save checkpoint after SignalKnowledgeWriter succeeds.
```

```text
Risk: duplicating WebexSyncEngine
 |
 +-- two implementations diverge
 +-- rate-limit/backoff tests cover old path only
 |
 v
Port by extracting/reusing engine behavior, not by a shallow fetchRecentMessages adapter.
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
 +-- checkpoint pipeline missing
 +-- per-target cursor/backoff not in connector contract
```
