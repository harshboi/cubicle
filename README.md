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

## Architecture DAG

```text
External sources
  -> apps/cubicle-macos/Sources/Connectors/
    -> SignalSyncPipeline
      -> SignalKnowledgeWriter
        -> apps/cubicle-macos/Sources/Data/DAO/KnowledgeStore.swift
          -> SQLite knowledge tables
            -> AppModel / NativeRefreshCoordinator
              -> SwiftUI views
              -> CodexPromptOrchestration -> CodexRunner

apps/cubicle-macos
  -> packages/webex-question-core
    -> question/topic/sentiment/network analysis

apps/cubicle-macos
  -> services/transcription
    -> live transcript events

apps/voicenotes-web
  -> services/transcription
```

## Runtime Call Flow DAG

```text
Config targets
  -> SignalTarget selectors
    -> TargetRouter
      -> WebexSignalConnector / IMessageSignalConnector
        -> SignalSyncBatch
          -> SignalKnowledgeWriter.mapRecords
            -> KnowledgeStore.writeConnectorMessageBatch
              -> rooms / people / messages / belief_evidence
                -> focus, question, belief, and Ask Codex views
```

## Filename Call Flow DAG

```text
ConfigStore.swift
  -> SignalModels.swift
    -> SignalConnector.swift
      -> SignalSyncPipeline.swift
        -> Webex/WebexSignalConnector.swift
        -> IMessage/IMessageSignalConnector.swift
          -> SignalKnowledgeWriter.swift
            -> Data/DAO/KnowledgeStore.swift
              -> Models/AppModel.swift
              -> Services/NativeRefreshCoordinator.swift
                -> Views/DashboardView.swift
                -> Views/QuestionsView.swift
                -> Views/AskCodexView.swift
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
