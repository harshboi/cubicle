---
name: cubicle-graph-foundations
description: Use when working on Cubicle's ontology-service graph/backend foundations, Source Evidence Spine, Ent schema stack, source identity/provenance, evidence anchors, Glean/Palantir/Meta/Obsidian-inspired graph tradeoffs, or PR planning after ontology PR #67.
---

# Cubicle Graph Foundations

Use this skill when a task touches Cubicle's Go `ontology-service`, Ent ontology schema, graph evidence model, source provenance, or the PR stack after `feat/ontology-ent-runtime-cardinality`.

## Core Ruling

The accepted foundation is the **Source Evidence Spine**. Keep the typed Ent ontology graph and the cardinality-safe lens topology, then add a source-neutral provenance/citation spine.

```text
typed Ent ontology graph
 |
 +-- Person / Workstream / Ticket / PullRequest / Document / Message / Evidence
 |
 +-- Person -> WorkArea -> WorkLens -> WorkLensWindow -> *LensResult -> target
 |
 +-- Source Evidence Spine
       |
       +-- SourceRun
       |     -> crawl coverage and partial/failure authority
       |
       +-- ExternalIdentity
       |     -> source identity, aliases, renames, merges, moved URLs
       |
       +-- SourceObservation
       |     -> observed source item state, deletion, visibility, content hash
       |
       +-- EvidenceAnchor
             -> exact citeable source span/comment/message/paragraph
```

## Non-Negotiables

- Keep typed Ent schemas. Do not add durable generic `nodes` / `edges` tables.
- Source identity belongs in `ExternalIdentity`, not directly on canonical ontology objects.
- Source crawl coverage belongs in `SourceRun`, not `WorkLensWindow`.
- Observed item state belongs in `SourceObservation`, not object/link metadata alone.
- Exact proof belongs in `EvidenceAnchor`; `Evidence` points to it.
- `SearchChunk`, `DocumentBlock`, FTS, embeddings, and RAG are later projections, not the first foundation.
- Any query returning evidence preview/text must check `SourceObservation.is_deleted`, permissions, visibility, and `SourceRun.status`.
- Missing rows from a partial/failed/rate-limited `SourceRun` cannot support absence claims.

## Read Order

Start with the transition research note when planning or implementing changes:

- `../../tmp/source-evidence-spine-transition-research.md`

Read the raw debate artifacts only when you need the reasoning trail or want to challenge the ruling:

- `references/debate/graph-foundations-live-chat.md`
- `references/debate/graph-foundations-meta.md`
- `references/debate/graph-foundations-palantir.md`
- `references/debate/graph-foundations-glean.md`
- `references/debate/graph-foundations-obsidian.md`

## PR Boundary

For the first implementation PR after #67:

```text
Allowed
 |
 +-- SourceRun
 +-- ExternalIdentity
 +-- SourceObservation
 +-- EvidenceAnchor
 +-- Evidence.evidence_anchor_id
 +-- generated Ent code
 +-- composite indexes
 +-- enum constants
 +-- idempotent upsert helpers
 +-- schema/repository tests
 +-- README/docs explaining the spine

Cut
 |
 +-- GraphQL contextSearch
 +-- GraphQL whyBlocked changes
 +-- SearchDocument / SearchChunk
 +-- DocumentSection / DocumentBlock
 +-- SQLite FTS5
 +-- embeddings/vector DB/RAG
 +-- raw source payload blobs
 +-- scheduler/retry/health platform
 +-- generic traversal over target_kind/target_id
```

## Review Checklist

Ask these before approving a graph-foundation change:

```text
Identity
 |
 +-- Is this Cubicle canonical identity or source identity?
 `-- If source identity, why is it not ExternalIdentity?

Freshness
 |
 +-- Is this item freshness or source crawl coverage?
 `-- If source crawl coverage, why is it not SourceRun?

Evidence
 |
 +-- Can this answer cite exact proof?
 `-- If exact proof, why is it not EvidenceAnchor?

Search
 |
 +-- Is this retrieval/indexing?
 `-- If yes, why is it in the foundation PR?

Cardinality
 |
 +-- Can this create a hot parent or unbounded traversal?
 `-- If person activity, why is it not behind WorkLensWindow?

Permissions
 |
 +-- Could this return source text without checking permission/freshness?
 `-- If yes, block it.
```
