# Cubicle Monorepo

Cubicle is a workspace for turning engineering communication into usable execution context: synced conversations, generated questions, focus targets, knowledge views, transcription, and operational tooling.

## Layout

```text
cubicle/
  Package.swift                 root SwiftPM workspace manifest
  apps/
    cubicle-macos/              SwiftUI macOS app
    voicenotes-web/             FastAPI VoiceNotes web app
  packages/
    webex-question-core/        Swift analytics/question-generation library
  services/
    transcription/              FastAPI/WebSocket transcription service
  infra/
    aws/transcription/          Terraform and deployment scripts
  docs/                         product, architecture, research, runbooks
  scripts/                      build, release, local runtime scripts
  connectors/                   future external-source connectors
  tools/                        future internal CLIs
  tests/                        future cross-system integration tests
```

## Code Structure DAG

```text
                         +-----------------------------+
                         |        Cubicle repo         |
                         +--------------+--------------+
                                        |
       +--------------------------------+--------------------------------+
       |                                |                                |
+------+-------+                +-------+--------+               +-------+------+
| apps/        |                | packages/      |               | services/    |
+------+-------+                +-------+--------+               +-------+------+
       |                                |                                |
       |                                |                                |
+------v---------------+        +-------v----------------+       +-------v----------------+
| cubicle-macos        |------->| webex-question-core    |       | transcription          |
| SwiftUI desktop app  |        | import, analyze, rank  |       | audio -> transcript    |
+------+---------------+        +------------------------+       +-------+----------------+
       |                                                                 |
       | runtime WebSocket/API                                           |
       +---------------------------------------------------------------->|
       |
       | local runtime state
       v
+------+----------------+
| knowledge/runtime    |
| outside git checkout |
+----------------------+

+----------------------+       +-----------------------+
| voicenotes-web       |------>| services/transcription|
| browser review app   |       | shared transcript API |
+----------------------+       +-----------------------+

+----------------------+       +-----------------------+
| infra/aws            |------>| deployable services   |
| Terraform + scripts  |       | ECS/Lambda/runtime    |
+----------------------+       +-----------------------+
```

## macOS App Internal DAG

```text
SwiftUI Views
  -> AppModel / FocusModels / KnowledgeModels
    -> Services
      -> WebexSyncEngine -> WebexAPIClient -> Webex API
      -> NativeIMessageIngestionService -> local iMessage source
      -> KnowledgeStore / NativeRuntimeStore -> runtime knowledge files
      -> QuestionEngine -> WebexQuestionGeneratorCore
      -> TranscriptionRuntime -> TranscriptionWebSocketClient -> transcription service
      -> CodexPromptOrchestration -> CodexRunner -> local Codex CLI
```

## AppModel Summary DAG

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

```text
AppModel is the macOS app controller: it loads runtime state, holds UI-facing data, coordinates refresh/Codex/DB services, and publishes everything the SwiftUI screens render.
```

## Local Services DAG

```text
apps/cubicle-macos/Sources/Services
  |
  +-- Runtime / Config
  |     |
  |     +-- * RuntimeConfiguration.swift         -> env/runtime root/Webex/Codex config
  |     +-- * ConfigStore.swift                  -> settings, targets, OAuth config, Ask Codex history
  |     +-- * NativeRuntimeStore.swift           -> focus cache files, snapshots, manifests, Codex job files
  |
  +-- Knowledge / Questions
  |     |
  |     +-- * KnowledgeStore.swift               -> SQLite backbone for messages, evidence, beliefs, questions
  |     +-- * QuestionEngine.swift               -> turns focus/evidence context into question candidates
  |
  +-- Refresh / Ingestion
  |     |
  |     +-- * NativeRefreshCoordinator.swift     -> main refresh pipeline coordinator
  |     +-- * NativeWebexIngestionService.swift  -> production Webex ingestion orchestration
  |     +-- WebexAPIClient.swift                 -> low-level Webex HTTP client
  |     +-- * WebexSyncEngine.swift              -> Webex cursors, polling, backoff, message processing
  |     +-- NativeIMessageIngestionService.swift -> local iMessage timeline ingestion
  |
  +-- Codex
  |     |
  |     +-- * CodexPromptOrchestration.swift     -> builds Codex prompts/context for summaries/questions/beliefs
  |     +-- CodexRunner.swift                    -> runs local Codex CLI and records artifacts
  |
  +-- OAuth
  |     |
  |     +-- OAuthService.swift                   -> browser OAuth flow and token persistence
  |     +-- OAuthKeychainStore.swift             -> keychain helper for OAuth secrets
  |
  +-- Transcription
        |
        +-- Transcription/TranscriptionModels.swift          -> transcript/session data shapes
        +-- Transcription/TranscriptionProtocol.swift        -> client-server protocol encoding/decoding
        +-- Transcription/TranscriptionRuntime.swift         -> session state, audio capture, transcript aggregation
        +-- Transcription/TranscriptionWebSocketClient.swift -> WebSocket client for transcription service
```

```text
* = important for understanding the product core
```

## Knowledge Flow DAG

```text
Webex + iMessage + transcripts
  -> ingestion/sync services
  -> normalized messages and threads
  -> feature extraction + topic/sentiment/network analysis
  -> generated questions + ranked focus targets
  -> SwiftUI dashboards, beliefs, jobs, and ask-Codex prompts
```

## Swift macOS App

Run from source:

```bash
cd /Users/prabhat/workspace/cubicle
export GETWEBEXSPACE_RUNTIME_ROOT=/Users/prabhat/Desktop/getwebexspace-data
swift run Cubicle
```

Run tests:

```bash
cd /Users/prabhat/workspace/cubicle
swift test
```

Build a local `.app` bundle:

```bash
cd /Users/prabhat/workspace/cubicle
bash scripts/build-app.sh
```

The app bundle is written to:

```text
.build/app/Cubicle.app
```

## Transcription Service

Run unit tests without installing FastAPI:

```bash
cd /Users/prabhat/workspace/cubicle
PYTHONPATH=services/transcription python3 -m unittest discover services/transcription/tests -v
```

Run the service locally:

```bash
cd /Users/prabhat/workspace/cubicle/services/transcription
python3 -m venv .venv
. .venv/bin/activate
pip install -r requirements.txt
TRANSCRIPTION_SERVICE_TOKEN=dev-token python -m transcription_service.main
```

## VoiceNotes Web App

```bash
cd /Users/prabhat/workspace/cubicle/apps/voicenotes-web
python3 -m venv .venv
. .venv/bin/activate
python -m pip install -e ".[dev]"
python -m uvicorn voicenotes_app.main:app --host 127.0.0.1 --port 8787
```

Run VoiceNotes tests:

```bash
cd /Users/prabhat/workspace/cubicle/apps/voicenotes-web
python -m pytest
```

## Git Safety

Runtime data and generated artifacts do not belong in this repo:

```text
knowledge/
.build/
.venv/
.pytest_cache/
__pycache__/
*.tfstate*
*.tfvars*
.env*
logs
```

## Code Comments

Use [docs/commenting-guide.md](docs/commenting-guide.md) for the repo commenting rubric.
