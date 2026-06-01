# Codex Working Memory

This file captures user preferences and repo-working context for future Codex sessions.

## Product Goal

```text
Cubicle
  -> eliminate TPM / program-manager busywork
  -> give software engineers the execution context needed to move forward
  -> surface blockers, open questions, ownership gaps, stale work, and next actions
  -> build from engineering communications and knowledge graph signals
```

Do not treat transcription as the product center unless explicitly requested.

```text
Primary focus
  -> connectors
  -> knowledge graph / DB
  -> questions
  -> focus views
  -> Codex orchestration

Lower priority
  -> transcription
```

## Explanation Style

Use diagrams first.

```text
Answer
  -> ASCII DAGs / line diagrams
  -> file maps
  -> call-flow trees
  -> important nodes starred with *
  -> one-line summaries inline on the same row
  -> minimal prose
  -> concrete filenames
```

Prefer this shape:

```text
Thing
  |
  +-- file
  |     |
  |     +-- responsibility
  |
  +-- next file
        |
        +-- responsibility
```

For architecture questions:

```text
Entry point
  -> state/controller
    -> service
      -> storage/API
        -> UI output
```

Explain by paths, not paragraphs.

## Canonical Diagram Style

This is the preferred explanation style because it is easier for the user to consume.
Use it before prose when explaining code structure.
A picture is worth a thousand words: make the structure visual first, then add terse labels.

```text
Component As Hub

                         EntryPoint.swift
                                  |
                                  v
                            CoreFile.swift
                                  |
        +-------------------------+--------------------------+
        |                         |                          |
        v                         v                          v
   UI / callers              Local Services              Data Models
        |                         |                          |
        v                         v                          v
 ScreenA.swift            Store.swift                  ModelA.swift
 ScreenB.swift            Config.swift                 ModelB.swift
 ScreenC.swift            Coordinator.swift            ModelC.swift
```

For file/component maps, star important nodes and put summaries on the side.

```text
Component File Map
  |
  +-- Runtime / Config
  |     |
  |     +-- * RuntimeConfiguration.swift -> env/runtime root/API config
  |     +-- * ConfigStore.swift          -> settings, targets, token config, history
  |     +-- * NativeRuntimeStore.swift   -> local cache files, snapshots, manifests
  |
  +-- Knowledge
  |     |
  |     +-- * KnowledgeStore.swift       -> SQLite backbone for product context
  |     +-- QuestionEngine.swift         -> turns evidence into candidate questions
  |
  +-- Optional / Lower Priority
        |
        +-- TranscriptionRuntime.swift   -> live transcript session state

* = important for the product core
```

Use named diagrams for different angles on the same system.

```text
Boot Diagram

EntryPoint.swift
  |
  +-- optional CLI/runtime command
  |     |
  |     +-- command runs, then exits
  |
  +-- CoreModel()
  |
  +-- RootView()
        |
        +-- model.startProgram()
              |
              +-- loadAll()
              |     |
              |     +-- settings
              |     +-- runtime status
              |     +-- caches
              |     +-- DB bootstrap
              |     +-- questions
              |     +-- target lists
              |
              +-- startup refresh
              |
              +-- background refresh loop
```

```text
Boundary Diagram

Frontend / SwiftUI
  |
  +-- RootView.swift
  +-- SidebarView.swift
  +-- DashboardView.swift
  +-- FocusListView.swift
  +-- QuestionsView.swift
  +-- BeliefsView.swift
  +-- AskCodexView.swift
  +-- SettingsView.swift
  |
  v
AppModel.swift
  |
  v
Local backend inside app
  |
  +-- KnowledgeStore.swift
  +-- ConfigStore.swift
  +-- NativeRuntimeStore.swift
  +-- NativeRefreshCoordinator.swift
  +-- Connectors/*
  +-- CodexPromptOrchestration.swift
```

```text
Responsibilities Diagram

Component.swift
  |
  +-- UI state
  |     |
  |     +-- selected section
  |     +-- selected item
  |     +-- loading/error state
  |
  +-- Startup
  |     |
  |     +-- startProgram()
  |     +-- loadAll()
  |
  +-- Refresh
  |     |
  |     +-- refreshNow()
  |     +-- refreshSelectedPageNow()
  |     +-- runRefreshCycle()
  |
  +-- Knowledge views
        |
        +-- load focus cache
        +-- load questions
        +-- load beliefs
```

```text
Data Flow Diagram

External source
  |
  v
Connectors / coordinator
  |
  v
KnowledgeStore.sqlite
  |
  v
AppModel
  |
  +-- Focus cache
  +-- Questions
  +-- Beliefs
  +-- Ask Codex context
  |
  v
SwiftUI screens
```

```text
Reading Path

EntryPoint.swift
  |
  v
RootView.swift
  |
  v
CoreFile.swift
  |
  +-- init()
  |
  +-- startProgram()
  |
  +-- loadAll()
  |
  +-- refreshNow()
  |
  +-- runRefreshCycle()
  |
  v
StorageFile.swift
```

When answering architecture questions:

```text
Diagram first
  -> one-line summary
  -> next files to read
  -> only then short prose
```

## Context Priority

Maintaining context is more important than isolated answers.

```text
When explaining
  -> place the file in the larger flow
  -> show what comes before it
  -> show what it calls next
  -> show what user-facing behavior it supports
```

## Codebase Mental Model

```text
Frontend / SwiftUI
  -> GetWebexSpaceMacApp.swift
  -> RootView.swift
  -> Views/*
  -> AppModel.swift as product shell/controller

Local backend inside app
  -> Data/DAO/KnowledgeStore.swift
  -> Data/Database/KnowledgeDatabase.swift
  -> Services/ConfigStore.swift
  -> Services/NativeRuntimeStore.swift
  -> Services/NativeRefreshCoordinator.swift
  -> Connectors/*
  -> Services/QuestionEngine.swift
  -> Services/CodexPromptOrchestration.swift
  -> Services/CodexRunner.swift
```

## AppModel Reading Map

```text
GetWebexSpaceMacApp.swift
  -> AppModel.swift
    -> FocusModels.swift
    -> KnowledgeModels.swift
    -> RuntimeConfiguration.swift
    -> ConfigStore.swift
    -> NativeRuntimeStore.swift
    -> KnowledgeStore.swift
    -> NativeRefreshCoordinator.swift
    -> QuestionEngine.swift
    -> CodexPromptOrchestration.swift
    -> CodexRunner.swift
    -> OAuthService.swift
    -> TranscriptionRuntime.swift
    -> Views/*
```

## Commenting Rubric

Trust the reader.

Good comments explain:

```text
non-obvious invariant
  -> why ordering matters
  -> why a fallback exists
  -> why a name avoids collision
  -> why data is written before a possible throw
  -> why a range/ID/schema rule exists
```

Avoid:

```text
comments that restate the function signature
comments that explain obvious enum/class semantics
multi-paragraph docblocks
bullet lists of obvious cases
line numbers from other files unless load-bearing
```

When adding docstrings broadly, keep them short and useful:

```text
Class/method docstring
  -> purpose or boundary
  -> invariant or side effect if important
  -> no prose version of the signature
```

For any comment, ask:

```text
Would removing this confuse a strong engineer?
  -> yes: keep and tighten
  -> no: delete
```

## Diff Summary Rubric

Keep summaries brief, but keep structural visuals.

```text
Diff summary
  -> terse bullets
  -> exact files
  -> testing results
  -> ASCII call-flow DAGs when useful
  -> tables/counter trees when they convey structure faster than prose
```

Avoid long narrative around obvious code changes.

## Planning And Review

For non-trivial changes:

```text
plan
  -> adversarial review
  -> execute
  -> focused tests
  -> full tests
  -> update PR body with What / Why / How / Testing
```

Use Superpowers skills when requested or clearly applicable.

For implementation:

```text
TDD
  -> write failing test
  -> verify red
  -> implement smallest change
  -> verify green
  -> run broader tests
```

## PR Summary Shape

```text
## What
  -> concrete changes

## Why
  -> product / architecture reason

## How
  -> implementation approach

## Testing
  -> exact commands and results
```

Use call-flow DAGs in PR descriptions when changing architecture.
