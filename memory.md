<!--
Association:
memory.md -> future Codex session -> product direction + working style ->
Cubicle ontology and PR review decisions.
-->

# Codex Working Memory

This file captures user preferences and repo-working context for future Codex sessions.

## Workspace History Trail

The user wants an analyzable memory trail for Cubicle and adjacent workspace work.

Use:

```text
/Users/prabhat/workspace/history
 |
 +-- entries/ -> one new file per substantial work chunk
 +-- daily/   -> compacted daily rollups preserving reasoning/work diffs
 +-- index.md -> latest map
```

At the end of each substantial work chunk, and before any visible/manual
compaction point, write a new entry with:

```text
Context
Reasoning Summary
Work State
Reasoning vs Work Diff
```

Then update the relevant `daily/YYYY-MM-DD.md` rollup. Store concise reasoning
summaries, assumptions, decisions, and evidence, not raw hidden chain-of-thought.

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

For comparisons, use side-by-side structure by default.

```text
Current / Option A              Proposed / Option B
 |                               |
 +-- node/edge -> limitation      +-- node/edge -> improvement
 +-- query path -> bottleneck     +-- query path -> bounded read
```

Only use sequential diagrams when temporal order is the point.

For architecture improvements, always include a diagram that explains why the
new architecture is better than the current architecture.

```text
Current Architecture              Better Architecture
 |                                |
 +-- current node/edge            +-- improved node/edge
 |   -> limitation                |   -> why it fixes the limitation
 +-- current query path           +-- improved query path
     -> bottleneck                    -> better bounded traversal/search
```

For graph explanations, show the system as a connected graph, not just a
sequential pipeline. Prefer a main entity centered or top-down, with typed nodes
and edge labels visible.

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

## Saved Cubicle Working Preferences

These are explicit user preferences from the ontology-service and graph design
work.

```text
PR workflow
 |
 +-- open small stacked PRs
 |     -> make review units narrow enough to inspect one by one
 |
 +-- update PR bodies heavily
 |     -> include What / Why / How / Testing
 |     -> include diagrams and explicit boundaries
 |
 +-- review chronologically
       -> explain each PR as if the user is the reviewer
```

```text
Implementation style
 |
 +-- use established framework/library patterns
 |     -> Go service uses Gin + gqlgen/Huma decisions intentionally
 |
 +-- keep names generic and truthful
 |     -> fixture/demo data must be named as fixture/demo data
 |     -> source-specific fake data must not sound authoritative
 |
 +-- feature-flag fake data
       -> do not silently load fabricated workplace data in product startup
```

```text
Go teaching mode
 |
 +-- add docstrings to exported types, functions, methods, and packages
 |     -> explain purpose, boundary, invariant, or side effect
 |
 +-- add comments next to important vars, consts, booleans, and typed aliases
 |     -> explain what the value means and how it is used
 |
 +-- prefer a little more context than normal
       -> the user is learning from the implementation
```

When this conflicts with the general commenting rubric, honor the explicit
Cubicle Go teaching-mode request while keeping comments concise and useful.

## Cubicle Graph Foundation Memory

Use these defaults when working on Cubicle ontology-service or graph design.

```text
Graph foundation
 |
 +-- typed Ent ontology schemas
 |     -> prefer typed object/link schemas over durable generic Object/Association tables
 |
 +-- high-cardinality reads
 |     -> Person -> WorkArea -> WorkLens -> WorkLensWindow -> *LensResult -> target
 |
 +-- source truth
       -> SourceRun -> SourceObservation -> EvidenceAnchor -> Evidence
```

Responsibility split:

```text
Serving graph                     Source Evidence Spine
 |                                |
 +-- WorkLensWindow               +-- SourceRun
 |   -> bounded read partition    |   -> crawl coverage, checkpoints, failures
 |                                |
 +-- *LensResult                  +-- ExternalIdentity
 |   -> ranked relation result    |   -> aliases, moves, merges, deleted source IDs
 |                                |
 +-- Evidence                     +-- SourceObservation
 |   -> graph-claim support       |   -> observed state, permissions, source hash
 |                                |
 +-- target object                +-- EvidenceAnchor
     -> canonical Cubicle row         -> exact paragraph/message/comment span
```

Do not make vectors, summaries, document chunks, or raw source payloads the
canonical graph truth. They can be retrieval projections later, backed by exact
source observations and evidence anchors.

For source-backed answer reads:

```text
Evidence
 |
 +-- EvidenceAnchor
 |
 +-- SourceObservation
 |     where is_deleted = false
 |     where permission policy and visibility match the viewer
 |
 +-- SourceRun
       where complete, or partial with an explicit warning
```

When available, load the `cubicle-graph-foundations` skill before changing this
foundation. It contains the Meta / Palantir / Glean / Obsidian debate and the
Source Evidence Spine transition research.

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
