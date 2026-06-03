# Cubicle Navigation Paths

## To Understand App Startup

```text
GetWebexSpaceMacApp.swift
 |
 +-- RuntimeCommandLine
 |     -> CLI refresh commands
 |
 +-- AppModel()
 |
 +-- RootView()
       -> model.startProgram()
```

Then read:

```text
AppModel.swift
 |
 +-- init()
 +-- startProgram()
 +-- loadAll()
 +-- refreshNow()
 +-- runRefreshCycle()
```

## To Understand DB

```text
Data/DAO/KnowledgeDatabase.swift
 |
 +-- path/open/bootstrap policy
 |
 v
Data/DAO/KnowledgeStore.swift
 |
 +-- schema migrations
 +-- message/evidence/belief/question writes
 +-- connector batch writes
 |
 v
Data/DAO/ConnectorCheckpointDAO.swift
 |
 +-- connector cursor/backoff/checkpoint rows
```

## To Understand Focus

```text
KnowledgeStore messages
 |
 v
NativeWebexIngestionService / focus snapshot rebuild
 |
 v
NativeRuntimeStore
 |
 v
FocusModels.swift
 |
 v
FocusListView.swift / DetailView.swift
```

## To Understand Questions

```text
QuestionEngine.swift
 |
 v
CodexPromptOrchestration.swift
 |
 v
CodexRunner.swift
 |
 v
question_candidates in KnowledgeStore
 |
 v
QuestionsView.swift
```

## To Understand Beliefs

```text
BeliefsView.swift
 |
 v
AppModel belief methods
 |
 v
NativeRefreshCoordinator belief refresh
 |
 v
CodexPromptOrchestration reconciliation
 |
 v
KnowledgeStore beliefs + belief_evidence
```

## To Add A Future Connector

```text
SignalModels.swift
 |
 +-- add selector/object/event shapes only if needed
 |
 +-- new Connectors/<Source>/<Source>SignalConnector.swift
 |
 +-- SignalConnectorFactory.swift
 |
 +-- SignalConnectorTests.swift
 |
 +-- SignalKnowledgeWriter only if mapping needs new normalized record types
```
