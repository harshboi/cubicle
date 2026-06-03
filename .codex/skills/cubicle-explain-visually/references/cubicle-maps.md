# Cubicle Visual Maps

## AppModel As Hub

```text
GetWebexSpaceMacApp.swift
 |
 v
* AppModel.swift -> app state + orchestration hub
 |
 +-- SwiftUI Views -> user-facing screens
 |
 +-- Local Services -> DB, refresh, OAuth, Codex, connectors
 |
 +-- Data Models -> focus, knowledge, questions, beliefs
```

## Boot

```text
GetWebexSpaceMacApp.swift
 |
 +-- RuntimeCommandLine
 |     -> optional CLI refresh, then exit
 |
 +-- AppModel()
 |
 +-- RootView()
       |
       +-- model.startProgram()
             |
             +-- loadAll()
             +-- startup refresh
             +-- background refresh loop
```

## Frontend Boundary

```text
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
* AppModel.swift -> state and user actions
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

## Read Path

```text
GetWebexSpaceMacApp.swift
 |
 v
RootView.swift
 |
 v
* AppModel.swift
 |
 +-- init()
 +-- startProgram()
 +-- loadAll()
 +-- refreshNow()
 +-- runRefreshCycle()
 |
 v
KnowledgeStore.swift
```
