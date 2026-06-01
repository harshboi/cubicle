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

## AppModel Summary Diagram

```text
AppModel.swift
  -> owns app state and coordinates local services so SwiftUI screens can show current work context.

  |
  +-- Startup
  |     -> loads settings, runtime status, DB, focus caches, questions, and background refresh.
  |     |
  |     +-- init()
  |     +-- startProgram()
  |     +-- loadAll()
  |
  +-- UI State
  |     -> tracks selected screen, selected focus item, loading state, errors, and draft settings.
  |     |
  |     +-- selectedSection
  |     +-- selectedFocusKind
  |     +-- selectedItemIDByKind
  |     +-- isLoading / errorMessage
  |
  +-- UI Data
  |     -> feeds dashboard tiles, focus lists, question views, beliefs, and Ask Codex history.
  |     |
  |     +-- spaceCache
  |     +-- personCache
  |     +-- questionCandidates
  |     +-- manualBeliefs
  |     +-- automaticBeliefs
  |     +-- askCodexQueryHistory
  |
  +-- Refresh
  |     -> runs manual, startup, page-priority, and background refresh pipelines.
  |     |
  |     +-- refreshNow()
  |     +-- refreshSelectedPageNow()
  |     +-- runRefreshCycle()
  |     +-- reloadAfterRefresh()
  |
  +-- Local Services
  |     -> bridges UI actions to files, SQLite, connectors, Codex, OAuth, and transcription.
        |
        +-- NativeRuntimeStore
        +-- ConfigStore
        +-- KnowledgeStore
        +-- NativeRefreshCoordinator
        +-- QuestionCandidateService
        +-- CodexPromptOrchestrationService
        +-- CodexRunner
        +-- OAuthService
        +-- TranscriptionViewModel
```

One-line summary:

```text
AppModel is the macOS app controller: it loads runtime state, holds UI-facing data, coordinates refresh/Codex/DB services, and publishes everything the SwiftUI screens render.
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
