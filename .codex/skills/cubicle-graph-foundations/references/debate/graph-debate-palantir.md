# Palantir Advocate - Round 5 Call-Out

Opposing claim being attacked: Meta says typed objects/associations and bounded association lists are the correct backend. Glean says retrieval, activity, and permission-aware search should be the central product layer. Obsidian says the user-facing graph should feel like local graph, backlinks, blocks, and canvas.

They are each right about a layer and dangerously wrong about the center of gravity.

## What They Are Dangerously Wrong About

Meta is dangerously wrong if it treats storage discipline as product truth. Typed objects and bounded association lists are necessary, but they do not explain whether a Slack crawl was partial, whether Jira renamed a ticket, whether a stale edge came from source truth or projection logic, or whether the user is allowed to trust the result. A fast association list can still serve wrong operational state.

Glean is dangerously wrong if it treats retrieval as the product brain. Search can find relevant material, but ranking is not the same as operational truth. A high-scoring Google Doc paragraph is evidence, not an ontology edge. A permission-aware answer is still unsafe if Cubicle cannot show which source record, projection run, and object state produced it.

Obsidian is dangerously wrong if it treats navigability as correctness. Backlinks and block graphs are excellent for exploration, but they are not source governance. A beautiful local graph that cannot distinguish "Jira says this is deleted" from "the crawler failed halfway" will make users trust a lie faster.

## Failure Scenario

User asks: "Why is launch blocked?"

Bad Meta-only answer:

```text
Workstream
 |
 +-- blocked_by -> Ticket
 +-- discussed_in -> SlackThread
```

It returns quickly, but the blocking ticket was renamed in Jira and the Slack crawl was partial. The graph looks authoritative while hiding the data failure.

Bad Glean-only answer:

```text
search("why is launch blocked")
 |
 +-- Jira snippet
 +-- Slack snippet
 +-- Google Doc paragraph
```

It finds plausible context, but cannot prove which blocker is current, which mention is stale, and which source was fully crawled.

Bad Obsidian-only answer:

```text
launch note
 |
 +-- backlink to blocker note
 +-- backlink to Slack summary
 +-- backlink to PR note
```

It is navigable, but it cannot tell whether those links are sourced, fresh, permission-valid, or operationally current.

Palantir-style answer:

```text
whyBlocked(workstream)
 |
 +-- current ontology blocker
 |     -> Ticket canonical ID, not mutable Jira key
 |
 +-- evidence
 |     -> SourceRecord, ProjectionRun, source_updated_at
 |
 +-- source health
 |     -> Slack SyncRun partial, Jira SyncRun complete
 |
 +-- action
       -> refresh Slack, confirm blocker owner, request review
```

The answer is allowed to be fast because Meta-style bounded reads exist underneath. It is allowed to be useful because Glean-style retrieval can supply candidate evidence. It is allowed to be understandable because Obsidian-style local graph can render the context. But the final authority must be Palantir-style operational ontology: source-backed objects, typed links, provenance, health, and eventually actions.

## Exact Next Cubicle PR Demanded

PR #68 must be: Add ontology lifecycle metadata.

Small scope only:

```text
Ent mixin:
 |
 +-- source_updated_at
 +-- ingested_at
 +-- last_seen_at
 +-- is_deleted
 +-- deleted_at
 +-- permission_policy_key
 +-- visibility_hash
```

Apply it to current canonical ontology object schemas:

```text
Person
Workstream
Ticket
PullRequest
Document
Message
Evidence
```

Why this PR first: it gives Cubicle immediate graph-level trust markers without building a full ingestion platform. It also sets the contract for the next Palantir PRs: `SyncRun`, `SourceRecord`, `ProjectionRun`, and `DataCheck`.

Acceptance criteria:

```text
go generate ./ent
go test ./...
go vet ./...
schema test verifies lifecycle fields exist on canonical objects
README documents that metadata is freshness/trust metadata, not source replay
```

## What Not To Build

Do not build a generic `objects` / `edges` table. It throws away the Ent typed ontology discipline that makes review and query planning sane.

Do not build Cypher, arbitrary multi-hop traversal, or a custom graph query language. Cubicle needs bounded operational queries first.

Do not build vector-first RAG as the canonical graph. Semantic retrieval should propose evidence candidates; it must not directly create trusted ontology edges.

Do not build a full workflow/action engine yet. Add action/audit primitives later, after source health and projection provenance exist.

## Concrete Backend Decisions

1. PR #68: ontology lifecycle metadata on typed Ent objects.
2. PR #69: `SourceConnection`, `SyncRun`, and `SyncCheckpoint`.
3. PR #70: `SourceRecord` journal for replay/debug.
4. PR #71: `ExternalIdentity` and `ObjectAlias` for Jira rename/merge/delete survival.
5. PR #72: `ProjectionRun` and `DataCheck`, plus GraphQL `sourceHealth`.

## Round 6 Rebuttal - Final PR #68 Decision

Opposing claims being attacked:

- Meta claim: PR #68 should be "Graph Source Metadata" on source-backed graph rows and association result rows.
- Glean claim: PR #68 should be `SearchDocument` and `SearchChunk`.
- Obsidian claim: PR #68 should be `DocumentSection` and `DocumentBlock`.

Final Palantir position: PR #68 must be **Ontology Lifecycle Metadata**, not retrieval chunks, not document blocks, and not a half-baked source identity scheme pasted onto every row.

### Attack On Meta's Source Metadata First Move

Meta is closest, but still wrong in a dangerous way.

Meta says the first PR should add `source_key`, `external_id`, `source_updated_at`, `ingested_at`, `last_seen_at`, `deleted_at`, and `visibility_hash` directly to source-backed objects and LensResult rows. The useful part is lifecycle/trust metadata. The dangerous part is treating source identity as a row property instead of a separate identity model.

That will break the first time Jira renames `CUB-123` to `PLATFORM-44`, a GitHub PR is transferred across repos, or a Slack message is mirrored into another channel. The object is Cubicle's canonical belief. The source key is not its identity. If PR #68 bakes source identity into every object as the primary mental model, the next PR will have to unwind it into `ExternalIdentity` anyway.

Correct first move:

```text
Ontology object
 |
 +-- lifecycle/trust fields
 |     -> source_updated_at
 |     -> ingested_at
 |     -> last_seen_at
 |     -> is_deleted
 |     -> deleted_at
 |     -> permission_policy_key
 |     -> visibility_hash
 |
 `-- no claim that one source key equals object identity
```

Defer source identity to the next identity PR:

```text
ExternalIdentity
 |
 +-- source_key
 +-- source_instance
 +-- external_id
 +-- object_kind
 +-- object_id
 +-- first_seen_at
 +-- last_seen_at
```

Meta's source metadata proposal is acceptable only if narrowed to lifecycle/trust metadata. It is not acceptable if it smuggles canonical identity into source-backed rows.

### Attack On Glean's Search Chunk First Move

Glean is wrong as a first move because it wants to index before Cubicle has a trust contract.

Glean says the real answer may live in an unlinked Google Doc paragraph. True. But `SearchChunk` before lifecycle metadata is how Cubicle creates a snippet machine with no operational guarantee. A chunk result must carry freshness, source visibility, deletion state, and object trust. If those fields are not already normalized on the ontology layer, every retrieval result will invent its own version of truth.

Failure:

```text
searchChunks("why is launch blocked")
 |
 +-- returns stale Google Doc paragraph
 +-- returns Slack snippet from partial crawl
 +-- returns Jira text for renamed ticket key
 |
 `-- user gets plausible answer with no trustworthy state boundary
```

Search chunks are necessary, but they must sit on top of the lifecycle contract:

```text
Ontology lifecycle metadata
 |
 +-- tells search what is stale/deleted/visible
 |
 +-- then SearchDocument/SearchChunk
       -> retrieves candidate evidence
       -> does not become canonical truth
```

Glean is right about discovery. Glean is wrong to demand discovery before freshness and visibility are canonical.

### Attack On Obsidian's Document Block First Move

Obsidian is wrong as a first move because it optimizes inspectability before trust.

Obsidian says engineers need exact paragraphs and local graph context. True. But `DocumentSection` and `DocumentBlock` before lifecycle metadata creates precise-looking stale context. Exact blocks are not enough. A stale exact paragraph is still wrong. A fresh exact paragraph from a source the user cannot see is still unsafe. A block from a partially crawled document set is still incomplete.

Failure:

```text
Document
 |
 +-- Section("Migration readiness")
      |
      +-- Block("rollout blocked until checkpoint migration is verified")
```

That looks product-useful. It is also dangerous if the source document was deleted, moved behind a new ACL, or superseded by a newer doc that the crawler did not ingest. Block identity does not solve source truth.

Blocks are a good PR after lifecycle metadata:

```text
Document lifecycle metadata
 |
 +-- then DocumentSection
 |
 +-- then DocumentBlock
       -> exact paragraph
       -> inherits freshness/visibility/deletion semantics
```

Obsidian is right about the UX unit. Obsidian is wrong to make the UX unit the first backend primitive.

### Final PR #68

Build PR #68 as **Ontology Lifecycle Metadata**.

Exact scope:

```text
Ent mixin: OntologyLifecycleMixin
 |
 +-- source_updated_at
 +-- ingested_at
 +-- last_seen_at
 +-- is_deleted
 +-- deleted_at
 +-- permission_policy_key
 +-- visibility_hash
```

Apply to:

```text
Person
Workstream
Ticket
PullRequest
Document
DocumentFragment
Message
Evidence
WorkLensResult rows where the target relationship is source-backed
```

Do not include in PR #68:

```text
SourceConnection
SyncRun
SourceRecord
ExternalIdentity
SearchDocument
SearchChunk
DocumentSection
DocumentBlock
```

Those are next, but not first.

### Why This Is The Only Defensible First Move

```text
Lifecycle metadata first
 |
 +-- Meta gets safer bounded reads
 +-- Palantir gets trust/provenance contract
 +-- Glean gets freshness/visibility fields for retrieval
 +-- Obsidian gets block/local graph safety semantics
```

The other orders invert dependency:

```text
Search chunks first
 |
 `-- discovers evidence before Cubicle can say if it is fresh or visible

Document blocks first
 |
 `-- makes stale context precise and therefore more misleading

Source identity first
 |
 `-- confuses source keys with canonical Cubicle objects
```

### Concrete Backend Decisions

1. PR #68: `OntologyLifecycleMixin` and tests proving lifecycle fields exist on canonical objects and source-backed lens result rows.
2. PR #69: `SourceConnection`, `SyncRun`, and `SyncCheckpoint` to explain crawl completeness.
3. PR #70: `ExternalIdentity` and `ObjectAlias`, before raw source records, so source keys cannot corrupt canonical object identity.
4. PR #71: `SourceRecord` and `ProjectionRun` to enable replay and audit.
5. PR #72: choose between `SearchDocument/SearchChunk` and `DocumentSection/DocumentBlock`, but only after the lifecycle contract exists.

What not to build:

- Do not build vector-first RAG.
- Do not build Cypher or arbitrary traversal.
- Do not build generic nodes/edges.
- Do not build source identity as object identity.
- Do not build document blocks or search chunks before lifecycle metadata.
