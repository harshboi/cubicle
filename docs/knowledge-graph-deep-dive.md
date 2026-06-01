# Knowledge Graph Deep Dive

This is project memory for the current Cubicle knowledge graph. I inspected code paths, tests, schema, local runtime shape, and public Glean/Palantir ontology docs. I did not read or quote private message content from the local database.

Diagram-first companion: `docs/knowledge-graph-diagrams.md`.

## Executive Picture

```text
Webex API / transcripts / iMessage
        |
        v
SQLite facts: rooms, people, messages, files
        |
        +--> belief_evidence --> Codex belief reconciliation --> beliefs
        |
        +--> focus cache JSON --> local clusters --> Codex summaries/topics
        |                                      |
        |                                      v
        |                              topics table + detail sections
        |
        +--> focus cache JSON --> local question seeds --> Codex synthesis --> question_candidates
```

Current state: this is a local intelligence store with derived projections. It is not yet a true typed knowledge graph or ontology. There are few first-class edges, no graph traversal layer, no permission graph, and no action layer.

## Runtime Snapshot

Local DB inspected: `/Users/prabhat/Desktop/getwebexspace-data/knowledge/knowledge.db`

```text
schema_migrations: 1,2,3,4,5
integrity_check: ok
foreign_key_check: no rows

rooms        135        people       553
messages  22,311        files      1,545
topics     7,356        beliefs    4,916
evidence   4,541        questions     23
sync_state    34        focus_clusters 0

knowledge dir: 8.2G
knowledge.db: 1.6G
codex cache: 65M
```

Important counts:

```text
belief_evidence by source:
  llm            3,795   legacy/migrated source
  webex_message    746   current Webex mirror source

messages not mirrored as source=webex_message evidence: 21,543
meeting transcript messages: 4
meeting transcript evidence: 0

question_candidates:
  candidate / codex_question_synthesis: 23

belief reconciliation state:
  space: 3 targets
  person/global: 0 targets in state table
```

## Storage Model

```text
rooms(room_id)
  |
  +-- messages(message_id, room_id, person_id, body, created_at)
  |      |
  |      +-- files(file_id, message_id, room_id)
  |
  +-- belief_evidence(evidence_id, source, source_id, room_id, person_id, text)

people(person_id, email)
  |
  +-- messages.person_id
  +-- belief_evidence.person_id

beliefs(scope, entity_key, statement, confidence, lifecycle)
belief_reconciliation_state(scope, entity_key, last_evidence_hash)

topics(focus_kind, scope, entity_key, topic_key)
focus_clusters(focus_kind, scope, entity_key, topic_key)  # table exists, no runtime writer found

question_candidates(scope_type, scope_key, evidence_json, status)
webex_sync_state(conversation_id, room_id, polling_mode, cursor)
```

Source files:

```text
Models/KnowledgeModels.swift          record types
Services/KnowledgeStore.swift         SQLite schema, migrations, compatibility, queries
Services/WebexSyncEngine.swift        incremental Webex message writes
Services/NativeWebexIngestionService.swift  batch Webex sync and focus source snapshots
Models/AppModel.swift                 transcript submission and belief UI operations
Models/FocusModels.swift              local event parsing and heuristic clustering
Services/NativeRuntimeStore.swift     focus cache persistence/reuse
Services/NativeRefreshCoordinator.swift     orchestration
Services/CodexPromptOrchestration.swift     Codex summaries, questions, beliefs
Services/QuestionEngine.swift         question candidate pipeline
```

## Ingestion DAG

```text
Tracked target
  |
  v
Webex room/direct messages
  |
  +--> upsert people
  +--> upsert room
  +--> upsert messages
  +--> if text not empty:
          upsert belief_evidence(source=webex_message, source_id=message_id)
```

Transcript submission is different:

```text
live transcript
  |
  +--> upsert person
  +--> upsert room
  +--> upsert message(id=meetinging-transcript:...)
  |
  x   no belief_evidence write today
```

This means transcript text can appear in focus timelines, but belief reconciliation will not see it unless it is separately backfilled as evidence.

## Focus DAG

```text
messages
  |
  v
FocusItem.detailLines
  |
  v
FocusMessageLineParser
  |
  v
FocusNormalizedEvent
  |
  v
FocusClusterSeed.makeSeeds
  |
  +-- person: roomKey + topicKey
  +-- space: semantic tokens + synonym map + Jaccard-like merge
  |
  v
native focus cache JSON
  |
  v
Codex summary/title/exec-question enrichment
```

Focus is currently a file projection, not a normalized graph table. The real product surface reads JSON snapshots under `knowledge/native` and preserves Codex sections by cache signatures.

## Belief DAG

```text
configured belief target
  |
  v
load current beliefs + manual beliefs
  |
  v
load evidence(limit=500)
  |
  +-- space: room_id = entity_key
  +-- person: person_id = entity_key, unless direct room exists
  +-- global: all evidence
  |
  v
hash(scope + beliefs + evidence)
  |
  +-- first run / evidence changed / stale after 24h / forced
  |
  v
deep reconciliation
  |
  +-- evidence windows: base days, then up to 90 days
  +-- chunks: 25 evidence items
  |
  v
Codex JSON: add/update/weaken
  |
  v
upsert beliefs + reconciliation state
```

Good design choices:

```text
manual beliefs dominate automatic updates
belief lifecycle exists: candidate, active, stable, retired
support_count / contradiction_count exist
evidence hash prevents unnecessary runs
chunking prevents huge prompts
```

Weak spots:

```text
evidence links are strings, not normalized joins
beliefs are not connected to people/rooms/topics by edge tables
person target semantics can switch between person_id evidence and direct-room evidence
legacy llm evidence dominates current webex_message evidence
```

## Question DAG

```text
space focus cache + person focus cache
  |
  v
extract message-like detail lines
  |
  v
WebexQuestionGeneratorCore local analysis
  |
  v
seed candidates
  |
  v
Codex question synthesis
  |
  v
question_candidates
```

Important behavior: if no Codex synthesizer is available, deterministic seeds are not published. Existing questions are preserved when Codex returns empty.

## Current Alignment To Glean / Palantir

The shared ChatGPT URL only exposed the title, `Glean vs Palantir Knowledge Graph`, behind a login shell. So this comparison uses official public docs:

- Glean benchmark: content + people + activity graph, many connectors, permission-aware indexing.
- Palantir benchmark: ontology as operational layer with objects, properties, links, actions/functions, dynamic security.

```text
Cubicle today
  |
  +-- aligns with Glean: messages + people + activity-ish recency
  +-- aligns with Palantir: early beliefs/questions as decision context
  |
  x-- missing vs Glean: connectors, ACL graph, enterprise search permissions
  x-- missing vs Palantir: typed objects/links, action functions, dynamic security, closed-loop operations
```

Best positioning: start as a personal/local executive intelligence graph. Do not pitch it as an enterprise ontology yet.

## Highest-Leverage Improvements

```text
P0: Evidence completeness
  messages -> backfill belief_evidence
  transcripts -> write belief_evidence on submit

P1: Normalize provenance
  beliefs.evidence_links_json -> belief_evidence_links(belief_id, evidence_id)
  question_candidates.evidence_json -> question_evidence_links(question_id, evidence_id)

P2: Add real graph edges
  knowledge_edges(source_type, source_id, relation, target_type, target_id, evidence_id, occurred_at)

P3: Decide cluster persistence
  either write focus_clusters from FocusClusterSeed
  or delete/retire the table

P4: Make topics useful
  topics currently persist but no runtime reader was found
  use them in detail/Ask/Questions or stop writing them

P5: Add retrieval
  FTS5 first, embeddings later
  do not send every graph query through flattened JSON detail lines

P6: Add action loop
  question -> answer snapshot -> task/decision -> outcome
  this is the Palantir-style jump from memory to operations
```

Suggested next schema:

```text
entities(entity_id, type, canonical_key, label)
events(event_id, source, source_id, occurred_at, text_hash)
documents/doc_fragments for longer text
edges(edge_id, source_entity, relation, target_entity, evidence_id)
claims(claim_id, scope_entity, statement, confidence, lifecycle)
claim_evidence(claim_id, evidence_id, stance)
topics(topic_id, label, embedding_ref)
topic_members(topic_id, event_id)
actions(action_id, source_question_id, owner_entity, status, outcome)
```

## Setup Notes

Use the local runtime root on this machine:

```bash
cd /Users/prabhat/Desktop/getwebexspace-data/GetWebexSpaceMac
export GETWEBEXSPACE_RUNTIME_ROOT=/Users/prabhat/Desktop/getwebexspace-data
swift test
swift run Cubicle
```

Build the app bundle:

```bash
bash Scripts/build-app.sh
```

Default runtime root in code is `/Volumes/Webex/getwebexspace-data`; this terminal could not read `/Volumes/Webex`, so set `GETWEBEXSPACE_RUNTIME_ROOT` for local runs from this checkout.

Codex features require `codex` on PATH or `CODEX_BIN`. Webex sync requires OAuth configuration in settings/env. Transcription is off by default.

## Git Upload Readiness

This checkout was not a Git repo during inspection. I added a root `.gitignore` for the obvious local/runtime risks.

Do commit:

```text
Package.swift
Sources/
Tests/
WebexQuestionGeneratorCore/
Scripts/
docs/
README.md
ARCHITECTURE.md
PRODUCT_DESIGN.md
```

Do not commit:

```text
.build/
.cache/
.playwright-cli/
.pytest_cache/
.venv/
output/
backups/
knowledge/
*.tfstate*
*.tfvars*
.env*
logs, pids, local runtime DBs
```

Before first push:

```bash
git init
git status --ignored
swift test
rg -n "AKIA|OPENAI_API_KEY|WEBEX|MISTRAL_API_KEY|PRIVATE KEY|terraform.tfstate|tfvars" .
```

The runtime data is the product's memory, not source. Keep it out of Git.

## Bottom Line

```text
Today:
  durable local fact store + AI projections

Near-term product:
  personal executive memory graph

To become a real knowledge graph:
  normalize evidence, add typed edges, add retrieval, connect questions to actions

To become enterprise-grade:
  connectors, permissions, audit, action governance, and deployment boundaries
```
