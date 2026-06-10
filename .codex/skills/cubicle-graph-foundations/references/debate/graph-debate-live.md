# Cubicle Graph Debate Live Log

## Rules

- Attack architecture claims, not people.
- Be blunt and adversarial. No politeness padding.
- Every response must cite the opposing claim it is attacking.
- Every response must end with concrete Cubicle backend decisions.
- Prefer examples: Slack partial crawl, Jira rename/delete, unlinked Google Doc paragraph, "why is launch blocked?"
- Do not propose generic node/edge tables, Cypher, full workflow engines, or vector-first RAG unless defending why they are worth the cost.

## Current Cubicle Baseline

```text
ontology-service after PR #67
 |
 +-- Ent typed graph
 +-- SQLite local persistence
 +-- GraphQL endpoint
 +-- Person / WorkArea / WorkLens / WorkLensWindow / LensResult
 +-- typed objects: Person, Workstream, Ticket, PullRequest, Document, Message, Evidence
```

## Moderator Current Synthesis

```text
Meta       -> serving/storage discipline: typed objects, typed associations, bounded reads
Palantir   -> trust/provenance: source records, projection, health, actions later
Glean      -> retrieval: chunks, search, activity, permissions, answer grounding
Obsidian   -> product surface: local graph, backlinks, exact document blocks
```

## Question For Debaters

What should Cubicle build next after PR #67, and what architecture mistakes would make the product fail?

## Round 5 Conflict

```text
Meta       -> PR #68 = graph source metadata
Palantir   -> PR #68 = ontology lifecycle metadata
Glean      -> PR #68 = SearchDocument + SearchChunk
Obsidian   -> PR #68 = DocumentSection + DocumentBlock
```

Moderator note:

```text
metadata-first
 |
 +-- graph rows become safer immediately
 +-- ingestion/search/block layers get freshness contract
 |
 `-- but no new user-visible context by itself

retrieval/block-first
 |
 +-- finds/exposes exact paragraphs earlier
 +-- gives product loop faster
 |
 `-- but risks building context over rows without a shared freshness/visibility contract
```

## Round 6 Prompt

Directly attack the other proposed PR #68 choices. Pick one final PR #68.

