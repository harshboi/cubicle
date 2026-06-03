# Cubicle PR History

Verified from GitHub on 2026-06-03. Re-check with `gh pr list --state all` before relying on freshness.

```text
#1 merged  Bootstrap Cubicle monorepo structure
 |
 +-- repo layout, README DAGs, SwiftPM root, ignores

#2 open    Refactor knowledge database setup into data layer
 |
 +-- KnowledgeDatabase / KnowledgeStore / KnowledgeDAO / ConnectorCheckpointDAO

#3 open    Add signal connector substrate
 |
 +-- SignalConnector, SignalModels, pipeline, writer, Webex/iMessage adapters

#4 merged  Document codebase boundaries
 |
 +-- docstrings + commenting guide

#5 merged  Add Codex working memory
 |
 +-- repo-local working preferences

#6 closed  Add AppModel memory diagram
 |
 +-- superseded by README/memory diagram work

#7 merged  Add AppModel README diagram
 |
 +-- visual AppModel map

#8 merged  Document preferred diagram explanation style
 |
 +-- canonical diagram-first explanation style

#9 merged  Add visual local services diagram
 |
 +-- starred Local Services DAG

#10 open   Wire signal connector processing service
 |
 +-- production .webexSync uses SignalConnectorProcessingService

#11 merged Fix app runtime root fallback
 |
 +-- Finder-launched app finds Desktop data root when env is absent

#12 merged Fix iMessage unavailable focus state
 |
 +-- iMessage authorization failure is visible in focus rows
```

Stack caveat:

```text
review split
 |
 +-- #10 connector wiring
 +-- #11 runtime fallback
 +-- #12 iMessage unavailable
 |
 v
current #10 diff may include #11/#12 if they were merged into its head branch
```

Always inspect:

```bash
gh pr diff <number> --name-only
gh pr view <number> --json baseRefName,headRefName,state,mergedAt
```
