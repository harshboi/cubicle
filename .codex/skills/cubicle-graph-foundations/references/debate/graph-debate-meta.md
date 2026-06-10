# Meta Debate Notes

## Round 5 - Meta Call-Out

Opposing claims being attacked:

- Palantir claim: Cubicle is an operational belief system and needs ingestion, projection, actions, and health.
- Glean claim: Cubicle is a workplace retrieval/RAG problem and needs search chunks, activity, and permissions sidecars.
- Obsidian claim: Cubicle must feel like local graph, backlinks, blocks, and canvas for engineer cognition.

Meta response: each camp is trying to make its support layer the center of the product. That is dangerous. Cubicle's center must be typed work objects and typed association lists, served through bounded reads. Everything else is supporting infrastructure.

### What They Are Dangerously Wrong About

Palantir is dangerously wrong if it makes Cubicle build a data platform before Cubicle can answer one fast engineering question. Ingestion runs and health are necessary, but they are not the graph. Actions are later. If actions arrive before trustworthy object and association reads, Cubicle becomes a workflow engine with no reliable operating picture.

Glean is dangerously wrong if it treats Cubicle as RAG over workplace blobs. Search can find text, but it cannot by itself define ownership, blocker state, implementation links, or canonical issue identity. A paragraph that says "launch is risky" is evidence, not the operational object. Retrieval must return candidates that hydrate into typed Ent objects and associations.

Obsidian is dangerously wrong if it makes links and canvases the model. Backlinks are useful navigation, but loose backlinks cannot distinguish "mentions a launch", "blocks a launch", "owns a launch", and "approved a launch". If every connection is just a cognitive link, Cubicle will look insightful while failing operational questions.

### Failure Scenario

Question: "Why is launch blocked?"

Bad Palantir-literal failure:

```text
user question
 |
 +-- ingestion dashboards say Slack is 72% complete
 +-- projection health says Jira synced
 +-- actions framework can create follow-up tasks
 |
 -> still no fast answer because no bounded blocker association list exists
```

Bad Glean-literal failure:

```text
user question
 |
 +-- vector search returns Slack chunks and doc paragraphs
 +-- answer says "launch may be blocked by infra risk"
 |
 -> weak because it did not join canonical Ticket, owner, PR, stale review, or evidence edges
```

Bad Obsidian-literal failure:

```text
user question
 |
 +-- local graph shows launch note, design doc, Slack thread, PR
 |
 -> pretty but ambiguous: it cannot prove which object is the blocker or who owns resolution
```

Meta-required shape:

```text
Workstream("Launch")
 |
 +-- open_blockers window
 |     |
 |     +-- Ticket
 |     +-- owner -> Person
 |     +-- blocking_prs -> PullRequest
 |     +-- evidence -> Message / DocumentFragment
 |
 +-- search sidecar only fills missing unlinked evidence
 +-- ingestion sidecar only explains source freshness
 +-- local graph only visualizes the bounded neighborhood
```

### Exact Next Cubicle PR I Demand

PR #68: Graph Source Metadata.

Purpose: make every source-backed graph row and metadata-bearing association explain its freshness and visibility without introducing a platform-sized ingestion system.

Schemas touched:

- Add a shared Ent source metadata mixin with `source_key`, `external_id`, `source_updated_at`, `ingested_at`, `last_seen_at`, `deleted_at`, and `visibility_hash`.
- Apply it to source-backed objects: `Ticket`, `PullRequest`, `Document`, `DocumentFragment`, `Message`, and `Evidence`.
- Apply the relevant freshness/visibility fields to association result rows: `DocumentLensResult`, `PullRequestLensResult`, `TicketLensResult`, and `MessageLensResult`.

Why this PR first:

- It preserves the Meta serving model.
- It handles the user's staleness concern directly.
- It creates the fields that Palantir ingestion can later populate.
- It gives Glean retrieval a permission/freshness contract.
- It gives Obsidian-style local graph views safe labels like stale, deleted, source, and visible.

What to defer:

- `SourceConnection`, `IngestionRun`, and `SourceRecord` until the next PR.
- Chunk search until graph rows have visibility/freshness.
- Local graph API until the canonical rows can be trusted.

### What Not To Build

Do not build generic `nodes` and `edges` tables. That throws away Ent's typed object and association discipline before Cubicle has earned a reason to do so.

Do not build Cypher-style arbitrary traversal in V0. Cubicle needs bounded GraphQL reads over association lists, not an online graph query engine that can time out on day one data.

Do not build vector-first RAG or a full actions/workflow engine yet. Search and actions are support layers. The first product truth must be the typed Ent graph: objects, associations, counts, windows, freshness, and evidence.

Final Meta demand:

```text
Build PR #68 as graph source metadata.
Then build minimal ingestion.
Then build retrieval chunks.
Then build local graph.
Do not invert that order.
```

## Round 6 - Rebuttal On PR #68

Claims being attacked from the live log:

- Palantir PR #68: ontology lifecycle metadata.
- Glean PR #68: `SearchDocument` and `SearchChunk`.
- Obsidian PR #68: `DocumentSection` and `DocumentBlock`.

Final Meta pick: PR #68 must be **Graph Source Metadata**, not Palantir's broader "ontology lifecycle metadata", not Glean search chunks, and not Obsidian document blocks.

### Attack 1 - Palantir's Lifecycle Metadata Is Close But Still Sloppy

Palantir is directionally right that graph rows need trust markers. The dangerous mistake is calling this "ontology lifecycle metadata" and applying it like platform governance across `Person`, `Workstream`, `Ticket`, `PullRequest`, `Document`, `Message`, and `Evidence` as one blob.

That blurs three different concepts:

```text
source freshness
 |
 +-- when did Cubicle observe the source row?

object lifecycle
 |
 +-- is the work object active, merged, archived, deleted, or superseded?

ontology/schema lifecycle
 |
 +-- is a type/field experimental, active, deprecated, promoted?
```

Mixing those in PR #68 will create vague fields that every later PR interprets differently. `is_deleted` on a Jira ticket is not the same thing as a deprecated ontology field. `last_seen_at` on a Slack message is not the same thing as a workstream's lifecycle state. `permission_policy_key` is not source freshness.

Failure mode:

```text
Ticket(CUB-123)
 |
 +-- is_deleted = true
 +-- last_seen_at = yesterday
 +-- permission_policy_key = jira-private
 |
 -> reviewer asks: deleted in Jira, hidden by ACL, merged into another issue, or stale because the crawl failed?
```

That is not trust. That is ambiguity with enterprise vocabulary.

Meta correction: PR #68 should add a narrowly named **SourceMetadataMixin** for source-backed graph rows and metadata-bearing association rows:

```text
SourceMetadataMixin
 |
 +-- source_key
 +-- source_instance
 +-- external_id
 +-- source_updated_at
 +-- ingested_at
 +-- last_seen_at
 +-- deleted_at
 +-- visibility_hash
```

Do not add schema lifecycle, projection lifecycle, action lifecycle, or full health semantics in this PR. Those belong in later dedicated PRs.

### Attack 2 - Glean Search Chunks Are Wrong As PR #68

Glean's claim: build `SearchDocument` and `SearchChunk` first so Cubicle can discover unlinked evidence.

This is the wrong first move. It creates a second content universe before the canonical graph rows have a freshness and visibility contract. Search chunks with `freshness_state` and `visibility_hash` sound responsible, but if the canonical `Document`, `Message`, and association rows do not carry the same contract, search will return results that the graph cannot safely hydrate or compare.

Failure mode:

```text
search("why is launch blocked")
 |
 +-- SearchChunk says freshness_state = fresh
 +-- SearchChunk says visibility_hash = abc
 |
 +-- Ent Document has no source freshness contract
 +-- Ent Message has no source freshness contract
 +-- LensResult edge has no shared visibility contract
 |
 -> answer stitches together rows with incompatible trust semantics
```

That makes Cubicle look smart before it is reliable. A retrieval sidecar must be downstream of graph trust metadata, not ahead of it.

Meta position: search chunks are PR #70 or later. First make the graph rows safe. Then chunks can point at them.

### Attack 3 - Obsidian Document Blocks Are Wrong As PR #68

Obsidian's claim: build `DocumentSection` and `DocumentBlock` first because exact paragraphs are the user-visible unit of meaning.

This is a product-surface argument pretending to be a backend foundation. Blocks are useful, but block modeling before shared source metadata will fragment Cubicle's source contract immediately. Worse, it overfits PR #68 to Google Docs while Cubicle's backend must handle Jira, GitHub, Slack, Docs, messages, and evidence uniformly.

Failure mode:

```text
Document
 |
 +-- DocumentSection
      |
      +-- DocumentBlock
           |
           +-- source_updated_at
           +-- ingested_at
           +-- last_seen_at
 |
 +-- Ticket has different metadata
 +-- PullRequest has different metadata
 +-- Message has different metadata
 +-- LensResult has different metadata
```

Now Cubicle has precise paragraphs but inconsistent graph trust. That is a bad trade. The first backend PR after PR #67 should strengthen every source-backed graph row, not just docs.

Meta position: document blocks are valuable after the graph source metadata contract exists. They should reuse the same mixin, not invent their own metadata fields.

### Final PR #68 Demand

Build **PR #68: Graph Source Metadata Contract**.

Scope:

```text
Ent source metadata mixin
 |
 +-- source_key
 +-- source_instance
 +-- external_id
 +-- source_updated_at
 +-- ingested_at
 +-- last_seen_at
 +-- deleted_at
 +-- visibility_hash
```

Apply to:

```text
source-backed objects
 |
 +-- Ticket
 +-- PullRequest
 +-- Document
 +-- DocumentFragment
 +-- Message
 +-- Evidence

metadata-bearing associations
 |
 +-- DocumentLensResult
 +-- PullRequestLensResult
 +-- TicketLensResult
 +-- MessageLensResult
```

Do **not** apply blindly to `Person` or `Workstream` in PR #68 unless a row is explicitly source-backed. A `Person` may later have multiple external identities; a `Workstream` may be Cubicle-native. Do not force source-row semantics onto aggregate or first-party objects.

Acceptance:

```text
go generate ./ent
go test ./...
schema test verifies the mixin fields on every selected schema
README explains:
  SourceMetadataMixin = source observation metadata
  not ingestion replay
  not ontology schema lifecycle
  not search chunk metadata
  not document block modeling
```

### What Not To Build In PR #68

Do not build `SearchDocument` or `SearchChunk`. Search without shared graph trust metadata becomes a parallel truth store.

Do not build `DocumentSection` or `DocumentBlock`. Blocks without shared graph trust metadata become a docs-only island.

Do not build `SourceConnection`, `IngestionRun`, `SourceRecord`, `ProjectionRun`, or action/health objects yet. Those should consume and explain source metadata after the canonical graph rows carry it.

Final order:

```text
PR #68 Graph Source Metadata Contract
 |
 +-- PR #69 Minimal ingestion run/source record
 |
 +-- PR #70 Search chunks using the same visibility/freshness contract
 |
 +-- PR #71 Document blocks using the same source metadata mixin
 |
 +-- PR #72 Local graph API over trusted typed rows
```
