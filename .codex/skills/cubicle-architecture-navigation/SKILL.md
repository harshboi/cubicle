---
name: cubicle-architecture-navigation
description: Navigate and explain the Cubicle codebase architecture. Use when Codex needs to understand where code lives, where to start reading, how AppModel, SwiftUI views, local services, DAO/KnowledgeStore, connectors, runtime caches, Codex orchestration, beliefs, questions, or knowledge graph flows fit together, or when the user asks how to navigate Cubicle.
---

# Cubicle Architecture Navigation

Use this skill to orient before explaining or editing Cubicle architecture.

## Start With Current Files

Run targeted discovery first:

```bash
pwd
git status --short --branch
rg --files apps/cubicle-macos/Sources | sort
rg -n "class AppModel|struct RootView|final class NativeRefreshCoordinator|final class KnowledgeStore|protocol SignalConnector" apps/cubicle-macos/Sources
```

Then answer in DAG-first style.

## Top-Level Map

```text
cubicle/
 |
 +-- * apps/cubicle-macos
 |     -> SwiftUI app + local backend
 |
 +-- packages/webex-question-core
 |     -> analytics/question-generation library
 |
 +-- services/transcription
 |     -> separate transcription backend; avoid unless asked
 |
 +-- apps/voicenotes-web
 |     -> separate web app; avoid unless asked
 |
 +-- infra/aws/transcription
       -> deployment infra; avoid unless asked
```

## macOS App Layers

```text
SwiftUI Views
 |
 v
* AppModel.swift -> state, user actions, orchestration hub
 |
 +-- Local Services -> DB, runtime, refresh, OAuth, Codex
 |
 +-- Connectors -> Webex/iMessage now, Slack/Jira/Drive later
 |
 +-- Models -> focus, knowledge, questions, beliefs
```

## Reading Paths

```text
app boot
 |
 +-- GetWebexSpaceMacApp.swift
 +-- RootView.swift
 +-- * AppModel.swift
 +-- NativeRuntimeStore.swift
 +-- NativeRefreshCoordinator.swift
```

```text
knowledge / DB
 |
 +-- * Data/DAO/KnowledgeDatabase.swift
 +-- * Data/DAO/KnowledgeStore.swift
 +-- Data/DAO/KnowledgeDAO.swift
 +-- Data/DAO/ConnectorCheckpointDAO.swift
 +-- Models/KnowledgeModels.swift
```

```text
connector flow
 |
 +-- * Connectors/SignalModels.swift
 +-- * Connectors/SignalConnector.swift
 +-- Connectors/SignalConnectorFactory.swift
 +-- Connectors/SignalSyncPipeline.swift
 +-- * Connectors/SignalKnowledgeWriter.swift
 +-- Connectors/Webex/*
 +-- Connectors/IMessage/*
 +-- Services/SignalConnectorProcessingService.swift
```

```text
insight surfaces
 |
 +-- Questions -> QuestionEngine.swift + CodexPromptOrchestration.swift
 +-- Beliefs   -> KnowledgeStore.swift + BeliefsView.swift + AppModel.swift
 +-- Focus     -> NativeRuntimeStore.swift + FocusModels.swift + FocusListView.swift
 +-- Ask Codex -> AskCodexView.swift + CodexRunner.swift
```

## Architecture Rules

- Treat the Swift app as frontend plus local backend.
- Do not call it "frontend only"; it owns SQLite, runtime caches, connectors, and Codex orchestration.
- Prefer source-neutral connector abstractions over Webex-shaped logic.
- Keep source-specific product methods in product services, not AppModel downcasts.
- Do not work on transcription unless the user explicitly asks.

## References

Read selectively:

- `references/current-architecture.md` for the component map.
- `references/navigation-paths.md` for task-specific reading order.
- Repo docs: `README.md`, `docs/knowledge-graph-diagrams.md`, `docs/knowledge-graph-deep-dive.md`, `docs/superpowers/specs/2026-06-01-signal-connectors-design.md`.
