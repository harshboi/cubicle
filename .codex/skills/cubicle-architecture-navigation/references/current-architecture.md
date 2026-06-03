# Current Cubicle Architecture

## App As Hub

```text
GetWebexSpaceMacApp.swift
 |
 v
* AppModel.swift -> state + workflow hub
 |
 +-- Views
 |     -> Root, Sidebar, Dashboard, Focus, Questions, Beliefs, Ask Codex, Settings
 |
 +-- Local services
 |     -> runtime cache, DB, refresh, OAuth, Codex, transcription shell
 |
 +-- Connectors
       -> source adapters and signal pipeline
```

## Data Flow

```text
Webex / iMessage / transcript
 |
 v
Connectors / refresh coordinator
 |
 v
KnowledgeStore.sqlite
 |
 v
NativeRuntimeStore JSON snapshots
 |
 v
AppModel
 |
 v
SwiftUI screens
```

## Connector Flow

```text
ConfigTarget
 |
 v
SignalTarget selectors
 |
 v
TargetRouter
 |
 +-- WebexSignalConnector
 +-- IMessageSignalConnector
 |
 v
SignalSyncBatch
 |
 v
SignalKnowledgeWriter
 |
 v
KnowledgeStore.writeConnectorMessageBatch
```

## Knowledge Graph State

Current state is a local intelligence store with derived projections, not yet a full ontology.

```text
SQLite facts
 |
 +-- rooms / people / messages / files
 |
 +-- belief_evidence -> beliefs
 |
 +-- focus cache JSON -> topics / question candidates
 |
 +-- webex_sync_state / connector_checkpoints
```

Important gap:

```text
current
 |
 +-- messages and evidence exist
 +-- beliefs/questions/focus exist
 |
 x-- no complete typed graph traversal layer yet
 x-- no full permission graph yet
 x-- no action/writeback layer yet
```
