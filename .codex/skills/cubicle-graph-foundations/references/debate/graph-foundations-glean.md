Read before replying:
- graph-foundations-live-chat.md
- graph-foundations-meta.md
- graph-foundations-palantir.md
- graph-foundations-glean.md
- graph-foundations-obsidian.md

## Round A - Glean Reply

### Meta Claim 1

> "Cubicle's foundation should be **typed work objects + typed associations + bounded association-list reads**, with a narrow source metadata contract on source-backed rows."

No. That is a serving foundation, not a knowledge foundation.

Meta is importing the correct lesson from TAO into the wrong layer. TAO-style typed objects and bounded association lists are useful once the system already knows what objects and associations exist. Cubicle's foundation problem is earlier: workplace evidence is scattered across docs, Slack threads, Jira comments, PR reviews, and source-specific metadata. A typed association list cannot retrieve evidence it has never indexed.

Glean's public product implementation proves the opposite ordering. Glean connectors are built around each source's APIs and data model, fetch source content, mirror source permissions, and keep data current in the tenant. They parse title, body, comments, media, and permissions, not just canonical object rows. Source: https://docs.glean.com/connectors/about

Glean's knowledge graph docs say the content crawler indexes titles, body copy, comments, media, metadata, permissions, and facets across connectors. Source: https://docs.glean.com/security/knowledge-graph

That is the foundation Cubicle needs: source-aware searchable evidence with visibility/freshness attached. Typed Ent associations should hydrate and explain retrieved candidates; they should not be the only way evidence becomes visible.

### Meta Claim 2

> "But Glean is wrong as a first foundation if it starts with `SearchDocument` and `SearchChunk`. Search chunks are not canonical work state."

This is a strawman. Glean is not claiming chunks are canonical work state. Glean is claiming chunks are the first discoverability substrate.

The canonical state can still be Ent:

```text
Ticket
PullRequest
Document
Message
Workstream
LensResult
```

But canonical state without searchable evidence is a thin ontology. It can answer only questions the graph already modeled. It cannot answer the common workplace question where the evidence exists before the edge:

```text
"Why is launch blocked?"
 |
 +-- real answer lives in a Google Doc paragraph
 +-- no Jira backlink exists
 +-- Slack has a partial crawl
 +-- PR review comment hints at the same issue
```

Meta's source metadata contract can mark the `Ticket` fresh. It still cannot find the unlinked paragraph.

Glean's Indexing API is explicit that organizations push documents, full-text content, metadata, and permissions to make custom content searchable and discoverable alongside other organizational content. It lists document indexing, permission management, custom metadata, and discovery as core capabilities. Source: https://developers.glean.com/api-info/indexing/getting-started/overview

The foundation should therefore be:

```text
SearchDocument
 |
 +-- source
 +-- source_instance
 +-- external_id
 +-- optional Ent object ref
 +-- visibility_hash
 +-- freshness_state
 +-- last_indexed_at

SearchChunk
 |
 +-- search_document_id
 +-- source_span
 +-- text_hash
 +-- lexical_text
```

This does not replace Ent. It gives Ent something useful to explain.

### Meta Claim 3

> "Retrieval before typed graph source metadata creates a parallel truth store: `SearchChunk.visibility_hash` and `SearchChunk.freshness_state` will drift from the canonical `Document`, `Message`, and `LensResult` rows."

Wrong failure model. Drift is not caused by retrieval-first. Drift is caused by pretending there is one canonical row when the source system has multiple retrievable units with different crawl, visibility, and freshness behavior.

Glean treats permissions as part of indexing and retrieval, not as a later graph-row decoration. Its permission docs require users and groups to be indexed before document permissions reference them; document-level allowed users determine who can search for the document. Source: https://developers.glean.com/api-info/indexing/documents/permissions

Glean's connector docs say connectors fetch the permissions map and content together so results follow source access rules. Source: https://docs.glean.com/connectors/about

For Cubicle, that means `visibility_hash` belongs first on the searchable source item. The typed Ent object can share or derive it later. If we put visibility only on `Document` and `Message`, we immediately lose precision for:

```text
Google Doc paragraph
Slack thread reply
Jira comment
PR review comment
attached PDF snippet
```

The source item is what gets retrieved. The source item needs the trust contract first.

### Meta Claim 4

> "Search should discover candidate evidence after the graph rows have a shared freshness/visibility contract."

This is backwards in real enterprise search systems.

Glean's "How connectors power the Glean experience" page states that connectors synchronize content and metadata into the search index, and that the Enterprise Graph is built from that foundation; the graph reflects relationships and organizational context layered on structure and signals derived from indexed data. Source: https://docs.glean.com/connectors/connectors-power-glean

That is the direct rebuttal: the index plus identity/permission structures are not downstream of the graph. They are the foundation from which graph interpretation is built.

Meta's order:

```text
typed graph rows
 |
 +-- source metadata
 |
 +-- later retrieval
```

Glean's documented order:

```text
connectors
 |
 +-- content + metadata + permissions
 |
 +-- index / retrieval substrate
 |
 +-- enterprise graph interpretation
 |
 +-- search / chat / agents
```

Cubicle should copy the second order at local POC scale.

### Meta Claim 5

> "Unicorn proves search is powerful, but separate. Meta built a graph-aware search/indexing system for social graph search instead of turning the storage graph into a search blob store."

Agreed on separation. Rejected on priority.

Unicorn proves users need a search/index layer because storage graph traversal is not enough for discovery. The fact that Meta separated Unicorn from TAO does not imply Cubicle should defer retrieval. It implies Cubicle should not pollute Ent with search blobs. That is exactly why Glean wants `SearchDocument` and `SearchChunk` as a sidecar, not a generic graph table and not blob fields on `Ticket`.

Unicorn paper: https://www.vldb.org/pvldb/vol6/p1150-curtiss.pdf

For Cubicle:

```text
Ent graph
 |
 +-- canonical typed work state

Search sidecar
 |
 +-- source evidence retrieval
 +-- visibility/freshness at the retrieved unit
 +-- optional pointer back to Ent
```

This is not "vector search everything." It is the minimal index foundation that lets the typed graph become useful.

### Foundation Decision Glean Demands

Cubicle PR #68 should be Option D: **search/retrieval sidecar first**, with a tiny trust contract on searchable source items.

```text
PR #68 = SearchDocument + SearchChunk
 |
 +-- lexical retrieval only
 +-- source / source_instance / external_id
 +-- visibility_hash
 +-- freshness_state
 +-- last_indexed_at
 +-- optional object_kind / object_id back to Ent
```

API:

```graphql
searchChunks(query: String!, first: Int!, sourceKinds: [String!], freshnessPolicy: FreshnessPolicy): SearchChunkConnection
```

This maps directly to Glean's documented product shape:

- connectors index content and permissions: https://docs.glean.com/connectors/about
- content crawler indexes body/comments/metadata/permissions/facets: https://docs.glean.com/security/knowledge-graph
- custom content is pushed into the search index with metadata and permissions: https://developers.glean.com/api-info/indexing/getting-started/overview
- retrieved content powers search, chat, and grounded answers with source citations: https://docs.glean.com/connectors/connectors-power-glean
- activity signals later improve ranking: https://developers.glean.com/api/client-api/activity/overview

### What Breaks If Cubicle Copies Glean Too Literally

Do not copy Glean's full enterprise tenant model, 100+ connector platform, hosted admin surface, vector ranking stack, assistant, agents, or enterprise permission engine for PR #68. Cubicle is local and day-0.

Copy only the foundation ordering:

```text
source item retrieval before graph-only explanation
permission/freshness attached to retrievable evidence
typed Ent graph used to explain and canonicalize top candidates
```

### What Meta's Foundation Would Break

If Cubicle builds `SourceMetadataMixin` first, the next demo will still fail the real product question:

```text
"Why is launch blocked?"
```

The graph can return a fresh ticket, a fresh document row, and a fresh PR. It still cannot locate the paragraph, comment, or Slack reply that contains the reason unless that text was indexed. Metadata-first makes the graph more respectable while leaving it blind.

That is not a foundation. That is labeling the dark.

## Round B - Glean Counterattack

Read before replying:
- graph-foundations-live-chat.md
- graph-foundations-meta.md
- graph-foundations-palantir.md
- graph-foundations-glean.md
- graph-foundations-obsidian.md

### Meta

> "Search should discover candidate evidence after the graph rows have a shared freshness/visibility contract."

This is the exact inversion that makes graph-first products feel authoritative while missing the answer.

Real-world evidence from Glean contradicts Meta's order. Glean says connectors are the foundation of a deployment: they ingest, map, and normalize content, activity signals, and permission sets into a unified data layer; that data feeds the Knowledge Graph and powers Search, Chat/RAG, and Assistant through indexed, live-retrieval, and hybrid access patterns. Source: https://docs.glean.com/connectors/connectors-power-glean

Glean is even more explicit in the same page: connectors synchronize content and metadata into the tenant search index; that index powers retrieval, faceting, and ranking; the Enterprise Graph is built from that foundation. Source: https://docs.glean.com/connectors/connectors-power-glean

Meta's proposed order:

```text
typed Ent object
 |
 +-- source metadata
 |
 +-- later retrieval
```

Glean's documented order:

```text
source connector
 |
 +-- content + metadata + permissions + activity
 |
 +-- search index / retrieval substrate
 |
 +-- graph interpretation
```

Meta's TAO evidence supports bounded serving, not source discovery. TAO itself is described as a service designed around typed objects and associations for efficient social graph serving at huge read/write volume. Source: https://engineering.fb.com/2013/06/25/core-infra/tao-the-power-of-the-graph/

Accepted: keep Ent typed objects, typed associations, and bounded reads.

Rejected: making graph-row metadata the foundation. That still leaves Cubicle unable to retrieve the paragraph, Jira comment, Slack reply, or PR review text that has not already become an Ent edge.

Cubicle foundation implication:

```text
PR #68
 |
 +-- SearchDocument.visibility_hash
 +-- SearchDocument.freshness_state
 +-- SearchChunk.lexical_text
 +-- optional object_kind/object_id back to Ent
```

Ent should explain retrieved candidates. It should not be the only way candidates become visible.

### Palantir

> "Correct foundation compromise: PR #68 = Ontology Lifecycle Metadata"

Accepted: Palantir is right that the system needs lifecycle semantics. Cubicle cannot serve stale or deleted source-backed content as if it were current.

Rejected: lifecycle metadata on ontology rows is the wrong first foundation. It labels what the graph already knows; it does not create discoverable evidence.

Palantir's own docs undercut a pure ontology-row-first plan. Foundry says a dataset is the essential representation of data from when it lands in Foundry through when it is mapped into the Ontology, and datasets provide permission management, schema management, version control, and updates over time. Source: https://www.palantir.com/docs/foundry/data-integration/datasets

That means Palantir's real foundation is not "put lifecycle fields on ontology rows." It is landed data with lifecycle, permissions, versioning, and later ontology mapping.

Health checks reinforce this: Foundry describes health checks as validating synced/transformed/scheduled data so pipeline data remains reliable over time, including freshness checks. Source: https://www.palantir.com/docs/foundry/data-integration/health-checks

So if Cubicle copies Palantir's principle correctly, the first foundation should attach freshness/visibility to the landed retrievable source item, not only to final ontology objects.

Glean-compatible Cubicle shape:

```text
SearchDocument
 |
 +-- landed searchable source item
 +-- source/source_instance/external_id
 +-- visibility_hash
 +-- freshness_state
 +-- last_indexed_at
 |
 +-- optional object ref
      -> Ticket / Document / Message / PullRequest
```

This gives Palantir its trust boundary without pretending that final Ent rows are the first place source truth exists.

### Obsidian

> "Blocks first create the canonical address that both metadata and search can attach to."

Accepted: Obsidian is right that exact addressability matters. Whole documents are too coarse. Obsidian supports links to notes, headings, and blocks; its docs define a block as a paragraph, quote, or list item that can be linked with a block identifier. Source: https://obsidian.md/help/links

Rejected: `DocumentSection` / `DocumentBlock` is too narrow as the first backend foundation.

Obsidian's graph view visualizes notes as nodes and internal links as edges. Source: https://obsidian.md/help/plugins/graph

That is excellent for local knowledge navigation, but it is not enough for workplace evidence ingestion. Cubicle's retrievable units are not all document blocks:

```text
Slack thread reply
Jira issue comment
GitHub PR review comment
Google Doc paragraph
PDF excerpt
email fragment
ticket description
```

Glean connectors explicitly parse source-specific content such as title, body, comments, media, permissions, attachments, threaded results, and thread mentions. Source: https://docs.glean.com/connectors/about

That is why `SearchChunk` is the correct foundation unit. A future `DocumentBlock` can be one producer of `SearchChunk`. It should not be the root abstraction for all workplace evidence.

Correct relationship:

```text
DocumentBlock later
 |
 +-- precise document-native block
 |
 +-- emits / maps to
      |
      +-- SearchChunk
           -> common retrieval unit across docs, Slack, Jira, GitHub
```

If Obsidian wins first, Cubicle builds a beautiful docs island. If Glean wins first, Cubicle gets a source-neutral evidence unit that Obsidian can render later.

### Foundation Decision

The foundation is not "generic search over blobs." It is permission/freshness-aware source evidence retrieval.

```text
PR #68 = SearchDocument + SearchChunk
 |
 +-- lexical only
 +-- source/source_instance/external_id
 +-- visibility_hash
 +-- freshness_state
 +-- last_indexed_at
 +-- text_hash/source_span
 +-- optional Ent object pointer
```

Do not build:

- source metadata only on ontology rows
- full ingestion/projection platform
- document block ontology as the first backend foundation
- vector DB
- LLM answer generation
- generic node/edge tables

Meta gets serving discipline. Palantir gets trust semantics. Obsidian gets exact addressability later. Glean gets the first foundation because evidence must be retrievable before it can be explained, governed, or visualized.

## Round C - Glean Foundation Invariants

Read before replying:
- graph-foundations-live-chat.md
- graph-foundations-meta.md
- graph-foundations-palantir.md
- graph-foundations-glean.md
- graph-foundations-obsidian.md

### 1. Canonical identity

The stable identity of a Cubicle work object is a Cubicle-owned Ent object id, not a source key.

```text
canonical work object
 |
 +-- Ticket(id = cubicle-owned)
 +-- PullRequest(id = cubicle-owned)
 +-- Document(id = cubicle-owned)
 +-- Message(id = cubicle-owned)
 |
 +-- ExternalIdentity later
      -> jira:CUB-123
      -> jira:PLAT-77
      -> github:old-org/repo#88
      -> github:new-org/repo#88
      -> slack:workspace/channel/ts
      -> gdocs:doc-id
```

Meta is right that the serving graph needs stable typed objects. Palantir is right that source identity is not the same as ontology identity. Glean agrees with both. A Jira key rename, repo transfer, Slack mirror, or Google Doc copy must add/change an external identity mapping; it must not mutate the canonical Cubicle object identity.

Real-world evidence:

- Glean describes connector-backed identity resolution as a synchronization function that aligns identifiers or display names across apps when connector and directory data support it: https://docs.glean.com/connectors/connectors-power-glean
- Glean permissions docs require users and groups to be indexed before documents can safely reference them in permissions, which proves cross-source identity must be explicit before it is referenced: https://developers.glean.com/api-info/indexing/documents/permissions
- Meta TAO models typed objects and associations as the stable serving units, validating Cubicle-owned typed object ids as the serving identity: https://engineering.fb.com/2013/06/25/core-infra/tao-the-power-of-the-graph/

Invariant:

```text
source ids are aliases/observations
Cubicle ids are canonical objects
retrieval rows may point to Cubicle ids, but never define them
```

### 2. Canonical evidence unit

The smallest durable thing Cubicle can cite is a source-backed evidence span.

For PR #68, represent that as `SearchChunk`. Longer-term, it can be renamed or generalized to `EvidenceSpan`, but the invariant is the same:

```text
SearchDocument
 |
 +-- SearchChunk
      |
      +-- source_span
      +-- chunk_ordinal
      +-- text_hash
      +-- lexical_text
      +-- optional object_kind/object_id
```

An ontology object is too coarse. A whole document is too coarse. A source record is too raw. An association row explains a relationship, but does not necessarily cite the exact text. A document block is a good source-specific evidence unit, but it overfits document-shaped data. A source-backed chunk/span is the cross-source evidence unit that can cover:

```text
Google Doc paragraph
Slack thread reply
Jira comment
GitHub PR review comment
PDF paragraph
ticket description
```

Real-world evidence:

- Glean connectors parse item content including title, body, comments, media, permissions, attachments, threaded results, and thread mentions; the evidence unit must therefore be source-neutral, not document-only: https://docs.glean.com/connectors/about
- Glean Chat retrieves candidate snippets from the connector-backed index and surfaces citations/deep links so users can verify grounded answers: https://docs.glean.com/connectors/connectors-power-glean
- Obsidian proves exact addressability matters by supporting links to headings and blocks, where a block is a paragraph/list/quote-level unit: https://obsidian.md/help/links

Invariant:

```text
Cubicle answer citations point at source-backed evidence spans
Ent objects explain those spans
chunks/spans do not replace canonical work objects
```

### 3. Freshness authority

Freshness is not owned by one row forever. It has an authority chain.

```text
SourceRun / SourceCheckpoint
 |
 +-- authoritative for source coverage
 |    -> complete / partial / failed / stale
 |
 +-- SearchDocument
 |    -> authoritative for indexed item snapshot in PR #68
 |    -> visibility_hash
 |    -> freshness_state
 |    -> last_indexed_at
 |
 +-- SearchChunk
 |    -> inherits parent SearchDocument freshness
 |    -> can prove content change with text_hash
 |
 +-- Ent object / association
      -> derived freshness for product reads
      -> must not be the only freshness authority
```

In PR #68, because `SourceRun` does not exist yet, `SearchDocument` is allowed to carry the minimal item-level freshness/visibility contract. But this must be named as indexed-item freshness, not global ontology truth.

Real-world evidence:

- Glean connectors fetch source content and permissions, keep data updated, and support indexed/live/hybrid access depending on freshness and source behavior: https://docs.glean.com/connectors/about
- Glean's connector experience docs explicitly call out crawl health, incremental updates, webhooks, and permissions as conditions for reliable Search/Chat behavior: https://docs.glean.com/connectors/connectors-power-glean
- Palantir Foundry health checks validate data quality and freshness over synced/transformed/scheduled pipelines: https://www.palantir.com/docs/foundry/data-integration/health-checks
- Palantir datasets model changes over time through transactions, which is the right evidence that freshness belongs to observed source/data state before final ontology projection: https://www.palantir.com/docs/foundry/data-integration/datasets

Invariant:

```text
SearchChunk cannot independently declare freshness
Ent object cannot be the only freshness authority
Source coverage belongs to source-run/checkpoint
Indexed item freshness belongs to SearchDocument until SourceRun exists
```

### 4. Retrieval/addressability ordering

Cubicle should define searchable, source-addressed chunks first, with the minimal source/freshness metadata on the same retrieval unit.

This is not "search before addressability." The chunk is the first cross-source addressable evidence unit:

```text
SearchDocument
 |
 +-- source / source_instance / external_id
 +-- visibility_hash
 +-- freshness_state
 +-- last_indexed_at
 |
 +-- SearchChunk
      |
      +-- source_span
      +-- chunk_ordinal
      +-- text_hash
      +-- lexical_text
```

Reject the three bad orderings:

```text
metadata first
 |
 +-- labels known graph rows
 `-- does not find unlinked evidence

document blocks first
 |
 +-- precise for Google Docs
 `-- weak for Slack/Jira/GitHub evidence

graph associations first
 |
 +-- fast bounded serving
 `-- only traverses relationships already modeled
```

Real-world evidence:

- Glean says connectors synchronize content and metadata into the tenant search index, and that the Enterprise Graph is built from that foundation: https://docs.glean.com/connectors/connectors-power-glean
- Glean's knowledge graph docs define the enterprise graph around content, people, and activity, and the crawler indexes content plus metadata, permissions, and facets: https://docs.glean.com/security/knowledge-graph
- Meta's Unicorn proves a search/index layer is necessary for discovery over graph-shaped data rather than relying only on object association serving: https://www.vldb.org/pvldb/vol6/p1150-curtiss.pdf
- Obsidian proves exact local anchors matter, but its official graph model is note/link-centered; Cubicle needs a source-neutral evidence unit before document-only blocks: https://obsidian.md/help/plugins/graph and https://obsidian.md/help/links

Invariant:

```text
retrieval and addressability must start together
SearchChunk is addressable by source span + text hash
later DocumentBlock can specialize document-shaped chunks
later SourceRun can become the stronger freshness authority
```

Non-negotiable foundation invariant:

Every Cubicle answer must resolve through a source-addressed evidence span with visibility and freshness, then optionally hydrate into a canonical Ent object and bounded association path. No answer should cite a whole object, whole document, raw source row, or free text blob without a stable source address and trust metadata.

Concession to strongest opposing camp:

Palantir has the strongest objection: long-term freshness and identity cannot live only on search rows. After PR #68, Cubicle needs `ExternalIdentity`, `SourceRun`, and `SourceCheckpoint` so source coverage, rename/merge/delete behavior, and projection provenance are not trapped in the retrieval sidecar.

Concrete PR #68 after this invariant:

Build `SearchDocument` + `SearchChunk` as a permission/freshness-aware lexical retrieval sidecar:

```text
SearchDocument
 |
 +-- source
 +-- source_instance
 +-- external_id
 +-- object_kind
 +-- object_id
 +-- title
 +-- url
 +-- visibility_hash
 +-- freshness_state
 +-- last_indexed_at

SearchChunk
 |
 +-- search_document_id
 +-- chunk_ordinal
 +-- source_span
 +-- text_hash
 +-- lexical_text
 +-- object_kind
 +-- object_id
```

Add `searchChunks(query, first, sourceKinds, freshnessPolicy)` and keep it lexical-only. No vector DB, no LLM answers, no generic node/edge table, no full ingestion platform, and no document-block ontology in PR #68.

## Round D - Glean Design Review

Read before replying:
- graph-foundations-live-chat.md
- graph-foundations-meta.md
- graph-foundations-palantir.md
- graph-foundations-glean.md
- graph-foundations-obsidian.md

### 1. Accept/reject the moderator candidate

| Candidate piece | Glean decision | Concrete row/table failure | Product query that fails |
|---|---|---|---|
| `ExternalIdentity` | Accept. This fixes Palantir's strongest objection: source identity is not canonical object identity. | Without it, putting `source_key/source_instance/external_id` directly on `Ticket` makes `jira:CUB-123 -> jira:PLAT-77` a row mutation instead of an alias history. | `resolveExternalIdentity(source: "jira", externalID: "CUB-123")` fails after rename/merge. |
| `SourceObservation` | Accept. This is the right landed-source state row. | Without it, `Document`, `Message`, and `Ticket` become the only place to record source_updated/deleted/visibility; that collapses raw source state and Cubicle belief. | `why is launch blocked?` cannot show "source said X, Cubicle believes Y." |
| `EvidenceAnchor` | Accept only if it is queryable. A locator-only anchor is dead weight. | If `EvidenceAnchor` has only `source_span` and `content_hash`, the row cannot satisfy lexical retrieval; it becomes a citation table search cannot use. | `contextSearch("OAuth config migration")` misses the unlinked Google Doc paragraph unless another `SearchChunk` table exists. |
| Missing `SourceRun` | Reject as written. PR #68 must include a minimal source-run row. | `SourceObservation` rows prove only items we saw. They cannot represent the Slack channel/window we failed to crawl. Missing rows are not data. | `sourceCoverage(source: "slack", scope: "channel:C123")` cannot distinguish partial crawl from no matching messages. |

Evidence:

- Glean connector docs say connectors fetch content and permissions, keep data updated, and support indexed, live, and hybrid access modes; that is source observation plus retrieval, not graph-row metadata only: https://docs.glean.com/connectors/about
- Glean's connector experience docs say connectors populate the search index and associated identity/permission structures, then graph interpretation layers structure/signals on top. It also calls out retrieval snippets, permissions, and crawl health as preconditions for Search/Chat: https://docs.glean.com/connectors/connectors-power-glean
- Palantir Foundry datasets model data from landing through mapping into the ontology, with permissions, schema, versioning, updates, and transactions. That validates a source observation layer before final ontology belief: https://www.palantir.com/docs/foundry/data-integration/datasets
- Palantir health checks validate job/build/freshness for pipelines. That validates a source-run row for partial Slack crawl, not just item rows: https://www.palantir.com/docs/foundry/data-integration/health-checks
- Obsidian links to blocks prove exact evidence anchors matter, but Obsidian does not solve cross-source retrieval/permissions by itself: https://obsidian.md/help/links
- Meta TAO validates typed object/association serving, but Unicorn validates that discovery needs a separate search/index layer: https://engineering.fb.com/2013/06/25/core-infra/tao-the-power-of-the-graph/ and https://www.vldb.org/pvldb/vol6/p1150-curtiss.pdf

Glean position: accept the moderator architecture shape, reject the exact PR if it omits `SourceRun` or makes `EvidenceAnchor` non-queryable.

### 2. Minimal PR #68 schema

List only these Ent schemas and fields.

#### `SourceRun`

| Field | Mark | Why it exists |
|---|---|---|
| `id` | identity | Cubicle-owned run id. |
| `source_key` | identity | Source family, such as `slack`, `jira`, `github`, `google_docs`. |
| `source_instance` | identity | Workspace/repo/site/tenant namespace. |
| `scope_key` | identity | Crawl scope, such as Slack channel id, Jira project, GitHub repo, Drive folder. |
| `started_at` | freshness | Run start. |
| `completed_at` | freshness | Run completion when known. |
| `coverage_state` | freshness | `complete`, `partial`, `failed`, `unknown`. |
| `cursor` | freshness | Source pagination/checkpoint cursor. |
| `error_code` | freshness | Failure/partial reason without logging huge error payloads. |

#### `ExternalIdentity`

| Field | Mark | Why it exists |
|---|---|---|
| `id` | identity | Cubicle-owned identity mapping row. |
| `object_kind` | identity | Target Ent kind, such as `ticket`, `pull_request`, `document`, `message`, `person`. |
| `object_id` | identity | Target Ent object id. |
| `source_key` | identity | Source family. |
| `source_instance` | identity | Source namespace. |
| `external_id` | identity | Source-owned id/key/url tuple. |
| `identity_state` | identity | `active`, `alias`, `retired`, `merged`, `deleted`. |
| `first_seen_at` | freshness | When Cubicle first saw this identity. |
| `last_seen_at` | freshness | When Cubicle last saw this identity. |

#### `SourceObservation`

| Field | Mark | Why it exists |
|---|---|---|
| `id` | identity | Cubicle-owned observation id. |
| `source_run_id` | freshness | Run that observed this item. |
| `external_identity_id` | identity | Source identity being observed. |
| `observed_at` | freshness | When Cubicle observed the item. |
| `source_updated_at` | freshness | Source-reported update time. |
| `ingested_at` | freshness | Local ingest time. |
| `last_seen_at` | freshness | Last time this source identity was seen. |
| `deleted_at` | freshness | Source deletion/tombstone time if observed. |
| `visibility_hash` | permission | Stable hash of source visibility/ACL state. |
| `permission_policy_key` | permission | Named policy/key used to interpret visibility. |
| `source_url` | citation | Deep link/source locator for inspection. |
| `content_hash` | citation | Hash of source-visible content for change detection. |

#### `EvidenceAnchor`

| Field | Mark | Why it exists |
|---|---|---|
| `id` | identity | Cubicle-owned citation anchor id. |
| `source_observation_id` | citation | Observation this anchor is inside. |
| `object_kind` | identity | Optional Ent object kind this anchor supports. |
| `object_id` | identity | Optional Ent object id this anchor supports. |
| `anchor_kind` | citation | `doc_span`, `slack_message`, `jira_comment`, `pr_review_comment`, `ticket_description`, etc. |
| `source_span` | citation | Source-local address: paragraph id, byte/line range, message ts, comment id. |
| `ordinal` | citation | Stable ordering inside the source item. |
| `text_hash` | citation | Hash of cited text/span. |
| `lexical_text` | citation | Minimal queryable text for lexical retrieval; no embeddings. |
| `visibility_hash` | permission | Copied from/consistent with observation for cheap filtering. |
| `freshness_state` | freshness | Derived from observation/run for cheap filtering: `fresh`, `stale`, `partial`, `deleted`, `unknown`. |

No serving optimization fields in PR #68 except indexes implied by the query paths:

```text
SourceRun(source_key, source_instance, scope_key, started_at)
ExternalIdentity(source_key, source_instance, external_id)
SourceObservation(external_identity_id, observed_at)
EvidenceAnchor(object_kind, object_id)
EvidenceAnchor(freshness_state, visibility_hash)
EvidenceAnchor(lexical_text) via SQLite FTS or equivalent lexical index
```

### 3. Four failure-mode proof

| Failure | Rows involved | Query path | What breaks without this schema |
|---|---|---|---|
| partial Slack crawl | `SourceRun(source_key="slack", scope_key="channel:C123", coverage_state="partial")` + any `SourceObservation` rows for messages actually seen | `sourceCoverage(slack, channel:C123)` checks `SourceRun`; `contextSearch` can mark Slack anchors from that run as partial. | Without `SourceRun`, absence of Slack messages is indistinguishable from no relevant messages. `Message.last_seen_at` cannot speak for an uncrawled page. |
| renamed/merged/deleted Jira issue | `ExternalIdentity(jira, CUB-123, identity_state="retired")`, `ExternalIdentity(jira, PLAT-77, identity_state="active")`, `SourceObservation.deleted_at` when tombstoned | `resolveExternalIdentity(jira, CUB-123)` maps old key to canonical `Ticket`; `ticketContext` shows alias/merge/deleted state. | If `external_id` lives directly on `Ticket`, rename overwrites history or creates duplicate tickets. If only `SearchChunk` has the key, graph ownership is fuzzy. |
| unlinked Google Doc paragraph | `ExternalIdentity(google_docs, doc-id)`, `SourceObservation(content_hash, visibility_hash)`, `EvidenceAnchor(anchor_kind="doc_span", lexical_text="OAuth config migration...", source_span="paragraph:abc")` | `contextSearch("OAuth config migration")` hits `EvidenceAnchor.lexical_text`, then hydrates `Document` through `object_kind/object_id` or the observation identity. | Metadata-only rows cannot find the paragraph. DocumentBlock-only overfits docs and ignores Slack/Jira/PR comments. Locator-only `EvidenceAnchor` cannot search. |
| fast "why is launch blocked?" | top `EvidenceAnchor` rows + `ExternalIdentity` + typed Ent object expansion through `Ticket`, `PullRequest`, `Document`, `Message`, `LensResult` | `contextSearch("why is launch blocked?", first: 20)` returns anchors, filters permission/freshness, hydrates top object ids, then bounded Ent expansion answers with citations. | Pure graph traversal misses unlinked evidence. Pure search chunk lacks source-run coverage and canonical identity. Whole-document citation is not inspectable enough. |

This is Glean's evidence model at local scale: source connectors index content/metadata/permissions for search, while the graph interpretation layers relationships and organizational context on top. Glean explicitly describes connectors as feeding the search index and Enterprise Graph, with Search/Chat retrieving snippets under source permissions: https://docs.glean.com/connectors/connectors-power-glean

### 4. What must NOT be in PR #68

Brutal cuts:

- No `SearchDocument` table. `SourceObservation` is the source item; do not duplicate it.
- No separate `SearchChunk` table. `EvidenceAnchor` is the source-neutral citeable/retrievable unit; do not create a parallel truth row.
- No `DocumentSection` / `DocumentBlock`. Docs are one source; PR #68 must also fit Slack/Jira/GitHub.
- No embeddings, vector DB, reranker, LLM answer generation, or RAG orchestration.
- No generic `nodes` / `edges` tables.
- No Cypher or arbitrary graph query language.
- No Palantir-style actions, writeback, health-check engine, scheduler, or projection platform.
- No source fields pasted onto every ontology object as canonical identity.
- No `Person` or `Workstream` direct source identity in PR #68; use `ExternalIdentity` only when there is an actual source identity.
- No raw payload archive. `content_hash`, `source_url`, `source_span`, and `lexical_text` are enough for the POC foundation.
- No activity ranking. Glean uses activity for relevance, but ranking waits until the citation spine exists. Source: https://docs.glean.com/connectors/about

Final PR #68 recommendation:

Accept the moderator candidate with two Glean amendments: include minimal `SourceRun`, and make `EvidenceAnchor` lexical/queryable. Add four small Ent schemas: `SourceRun`, `ExternalIdentity`, `SourceObservation`, and `EvidenceAnchor`. This gives Cubicle source coverage, identity survival, source observation state, and exact evidence retrieval without making search chunks, document blocks, or ontology rows the canonical source of truth.

PR #69 immediately after:

Wire the first read path: `contextSearch(query, first, sourceKinds, freshnessPolicy)` over `EvidenceAnchor.lexical_text`, returning anchors with `SourceObservation`, `ExternalIdentity`, optional Ent object refs, and source coverage warnings. Then seed one fixture for each failure mode: partial Slack crawl, Jira rename/delete, unlinked Google Doc paragraph, and launch-blocker answer.

What I would block in review:

I would block any PR #68 that ships only graph metadata fields and cannot run `contextSearch("OAuth config migration")`; any PR that ships locator-only `EvidenceAnchor` with no queryable text/hash; any PR that puts `source_key/source_instance/external_id` directly on `Ticket` as identity; any PR that adds `SearchChunk` or `DocumentBlock` as a second canonical evidence store; and any PR that claims partial Slack crawl can be inferred from missing message rows instead of a `SourceRun.coverage_state`.

## Round E - Glean Implementation Review

Read before replying:
- graph-foundations-live-chat.md
- graph-foundations-meta.md
- graph-foundations-palantir.md
- graph-foundations-glean.md
- graph-foundations-obsidian.md

I accept the **Source Evidence Spine** ruling, but I would still block a sloppy PR #68. The danger is not philosophy anymore. The danger is shipping Ent schemas that look clean in a diagram and then fail on codegen churn, SQLite bloat, identity duplication, and unsafe queries.

Evidence used for this review:

- Ent edges are typed relationships generated into query APIs; the docs model relations through `edge.To` / `edge.From` and generated edge query methods: https://entgo.io/docs/schema-edges/
- Ent indexes support composite unique indexes, including edge/field composition, which is mandatory for source identity constraints: https://entgo.io/docs/schema-indexes/
- Ent codegen is schema-driven; adding schema/edge fields means rerunning generation and changing generated clients/resolvers: https://entgo.io/docs/code-gen/
- Ent hooks are the right place for mutation validation when the database cannot enforce a polymorphic target: https://entgo.io/docs/hooks/
- SQLite FTS5 can search text efficiently, but it stores/indexes text and has deletion/secure-delete tradeoffs; external/contentless modes reduce storage but create consistency obligations: https://www.sqlite.org/fts5.html
- Glean's connector model treats content, metadata, permissions, activity, indexing, and graph interpretation as one pipeline; permissions are not optional filters after retrieval: https://docs.glean.com/connectors/connectors-power-glean and https://developers.glean.com/api-info/indexing/documents/permissions

### 1. Ent implementation risk

| Question | Glean review answer | Concrete Ent pain |
|---|---|---|
| Is `target_kind` + `target_id` acceptable? | Yes, for PR #68 only, and only as a **provenance pointer** from `ExternalIdentity` / `EvidenceAnchor` to a canonical Ent object. | Ent cannot enforce a real foreign key from one `(target_kind, target_id)` pair to many possible tables. This is not a typed Ent edge, so generated code will not give `QueryTarget()` or compile-time traversal. |
| Must PR #68 use typed optional edges instead? | No. That is too strict for the first foundation PR. | If `ExternalIdentity` has optional edges to `Ticket`, `PullRequest`, `Document`, `Message`, `Person`, and every future type, every new ontology type forces a migration, codegen, GraphQL resolver update, and nullable-edge validation. The foundation table becomes a schema-change magnet. |
| What is the merge-safe compromise? | Keep `(target_kind, target_id)` now, but make it explicit that this is **not** a hot traversal edge. Add enum validation, indexes, and a mutation hook that verifies the target exists for known kinds. | Code reads must switch on `target_kind` in service code. That is acceptable for PR #68 because identity resolution is not the main traversal path. Typed Ent associations remain the graph traversal path. |

Strict typed edges would generate nicer APIs but create the wrong blast radius:

```text
ExternalIdentity
 |
 +-- ticket_id nullable
 +-- pull_request_id nullable
 +-- document_id nullable
 +-- message_id nullable
 +-- person_id nullable
 +-- future_type_id nullable

New type added later
 |
 +-- migration adds another nullable FK
 +-- ent generate rewrites client/edge code
 +-- gqlgen/resolver types change
 +-- tests must prove exactly one optional edge is set
```

That is not reviewable as PR #68. Use strict typed edges later only if a target relationship becomes a hot path. The PR #68 invariant should be:

```text
target_kind + target_id
 |
 +-- allowed on provenance rows
 +-- indexed for lookup
 +-- validated by Ent hooks
 +-- forbidden as generic graph traversal API
```

### 2. Identity uniqueness

The current ruling needs sharper indexes or it will duplicate canonical objects silently.

| Failure | Required uniqueness/index | Why |
|---|---|---|
| Jira issue renamed from `CUB-123` to `PLAT-77` | `UNIQUE(source_key, source_instance, external_id)` on `ExternalIdentity` and non-unique `INDEX(target_kind, target_id, identity_status)` | Old source keys must resolve to exactly one identity row. If the same old key can be inserted twice, backlinks and aliases corrupt the canonical `Ticket`. |
| Jira issue merged/deleted | `ExternalIdentity.replaced_by_identity_id` self-edge/index plus `identity_status` enum | Merge/redirect history belongs in identity rows, not by overwriting `Ticket.external_id`. |
| GitHub repo moved from `old-org/repo` to `new-org/repo` | Prefer immutable provider IDs when available. If only path-based IDs exist, store both as separate `ExternalIdentity` rows pointing to the same canonical object, with replacement link. | Repo path is not durable identity. A unique key on the full source namespace prevents accidental reuse from creating duplicate PR objects. |
| Slack mirrored/exported message | Unique per source namespace: `UNIQUE(source_key, source_instance, external_id)`, not global by timestamp. | Slack timestamps are only meaningful inside workspace/channel context. Mirrored data from another workspace may be a separate source identity even when it targets the same canonical `Message`. |
| Google Doc copied | Do **not** collapse by `content_hash`. Use unique `ExternalIdentity(source_key, source_instance, external_id)` and let the copy become a new identity unless a resolver explicitly links it. | Same content does not mean same work object. Copies fork lifecycle, permissions, comments, and future edits. |

Required Ent indexes for PR #68:

```text
SourceRun
 |
 +-- INDEX(source_key, source_instance, scope_kind, scope_key, started_at)
 +-- UNIQUE(source_key, source_instance, run_key) if run_key exists

ExternalIdentity
 |
 +-- UNIQUE(source_key, source_instance, external_id)
 +-- INDEX(target_kind, target_id, identity_status)
 +-- INDEX(replaced_by_identity_id)

SourceObservation
 |
 +-- UNIQUE(source_run_id, external_identity_id)
 +-- INDEX(external_identity_id, observed_at)
 +-- INDEX(source_run_id, observed_kind)
 +-- INDEX(permission_policy_key, visibility_hash)

EvidenceAnchor
 |
 +-- UNIQUE(source_observation_id, anchor_kind, anchor_locator, text_hash)
 +-- INDEX(object_kind, object_id, anchor_kind)
 +-- INDEX(source_observation_id, ordinal)
```

I am intentionally not asking for a partial unique active-identity index in PR #68. SQLite supports partial indexes, but Ent/Atlas migration complexity is unnecessary for the first PR. One identity row per `(source_key, source_instance, external_id)` is simpler and safer for now. Mutate `identity_status` in place and point to a replacement identity when needed.

### 3. Anchor text risk

`EvidenceAnchor.anchor_text` is dangerous if it is unbounded. It can leak source content into local SQLite backups/logs, duplicate large docs/messages, and quietly become the `SearchChunk` table we explicitly cut. SQLite FTS5 makes lexical lookup feasible, but FTS storage and delete behavior matter; old index entries can remain unless secure-delete behavior is handled, and external/contentless FTS modes push consistency back onto us.

Cut `anchor_text` as a vague field name. Replace it with bounded fields:

| Field | Limit | Purpose |
|---|---:|---|
| `text_hash` | hash only | Detect citation drift without storing the full source span. |
| `text_preview` | <= 512 chars | Human inspection snippet for Day 0 UI and tests. |
| `lexical_text` | nullable, <= 2048 chars | Exact source-derived text used only for Day 0 lexical lookup. No LLM summary, no embedding payload, no whole document. |
| `source_locator` / `source_url` | bounded string | Lets the UI fetch/open the source of truth. |

If PR #68 adds FTS, it must be an implementation detail over `EvidenceAnchor.lexical_text`, not a new semantic table. The safe model is:

```text
EvidenceAnchor
 |
 +-- text_hash       // citation integrity
 +-- text_preview    // bounded display
 +-- lexical_text    // bounded Day 0 search text
 |
 +-- optional FTS5 virtual table
      |
      +-- rowid = evidence_anchor.id
      +-- no independent identity
      +-- rebuildable from EvidenceAnchor
```

Do not store full Google Docs, Slack threads, Jira issues, PR descriptions, or generated summaries in `EvidenceAnchor`. That is SearchChunk/RAG scope hiding under a different name.

### 4. Permission/freshness correctness

This query is illegal:

```sql
SELECT ea.*
FROM evidence_anchors ea
WHERE ea.lexical_text MATCH 'launch blocked'
ORDER BY ea.ordinal
LIMIT 20;
```

It leaks or misleads because it ignores visibility, policy namespace, deletion, and source coverage. The minimum legal shape is:

```sql
SELECT ea.*, so.*, sr.*
FROM evidence_anchor_fts fts
JOIN evidence_anchors ea ON ea.id = fts.rowid
JOIN source_observations so ON so.id = ea.source_observation_id
JOIN source_runs sr ON sr.id = so.source_run_id
WHERE evidence_anchor_fts MATCH :query
  AND so.is_deleted = FALSE
  AND so.permission_policy_key IN (:viewer_policy_keys)
  AND so.visibility_hash IN (:viewer_visibility_hashes)
  AND sr.status IN ('complete', 'partial')
  AND (:include_partial = TRUE OR sr.status = 'complete')
ORDER BY rank
LIMIT :limit;
```

Rules:

- If `SourceRun.status = partial`, the result may be returned only with a coverage warning. Missing rows from that scope are unknown, not negative evidence.
- If `SourceObservation.is_deleted = true`, the anchor is excluded unless the query explicitly asks for tombstones/history.
- If `permission_policy_key` is unrecognized, exclude by default. Localhost/no-auth can use an explicit `local_dev_open` policy key; it must not mean "skip the filter."
- If `visibility_hash` is stale relative to the viewer permission snapshot, exclude or force live validation. Do not return "probably visible" content.

This is not optional product polish. Glean's connector docs explicitly tie retrieval to mirrored permissions and source visibility, and Glean's permissions API docs show that users/groups must be indexed before permissioned documents can be searched. Cubicle can simplify auth locally, but it cannot build a retrieval API whose shape ignores permission/freshness joins.

### 5. PR boundary

PR #68 should be four Ent schemas, indexes, generated code, and validation tests. Cut everything else.

| Cut from PR #68 | Why |
|---|---|
| `contextSearch` GraphQL endpoint | PR #69. First prove the schema and indexes migrate cleanly. |
| `SearchDocument` / `SearchChunk` | Explicitly rejected. Retrieval projection comes after the spine. |
| `DocumentSection` / `DocumentBlock` | Projection over `EvidenceAnchor`, not foundation. |
| embeddings/vector DB/reranker/LLM summaries | Not a foundation. Also explodes review scope. |
| ingestion scheduler/retry platform/health dashboard | `SourceRun.status` is enough for PR #68. |
| raw payload archive | Too big and source-specific. Use hashes/locators now. |
| typed optional target edges to every ontology type | Codegen/migration churn before target set stabilizes. |
| permission engine | Store policy keys/hashes now. Enforce through query filters later. |

PR #68 tests must be schema-level and migration-level:

```text
1. duplicate ExternalIdentity(source_key, source_instance, external_id) fails
2. duplicate EvidenceAnchor(source_observation_id, anchor_kind, anchor_locator, text_hash) fails
3. SourceObservation cannot point at a missing SourceRun
4. target_kind/target_id hook rejects unknown target kind or missing known target
5. deleted SourceObservation anchors are excluded by the repository helper
6. partial SourceRun is distinguishable from complete SourceRun in helper output
```

Approve PR #68 only if:

It adds exactly the Source Evidence Spine schemas (`SourceRun`, `ExternalIdentity`, `SourceObservation`, `EvidenceAnchor`) with explicit enum fields, the indexes above, Ent codegen, bounded text fields (`text_preview` / optional `lexical_text`, not vague unbounded `anchor_text`), and validation hooks/tests proving identity uniqueness and target reference safety. `target_kind + target_id` is acceptable only as a provenance pointer with hook validation and explicit documentation that typed Ent associations remain the graph traversal path.

Block PR #68 if:

It adds `SearchChunk`, `DocumentBlock`, generic nodes/edges, embeddings, GraphQL search APIs, raw payload blobs, source identity fields directly on canonical ontology objects, unbounded anchor text, tombstone inference from partial runs, or any lexical query helper that can return anchors without joining through `SourceObservation` and `SourceRun` and checking `permission_policy_key`, `visibility_hash`, `SourceRun.status`, and `SourceObservation.is_deleted`.

One migration/index I insist on:

`UNIQUE(source_key, source_instance, external_id)` on `ExternalIdentity`. Without that, Jira rename, GitHub repo move, Slack mirrored messages, and Google Doc copy all become duplicate/corrupt canonical-object bugs instead of explicit identity-resolution events.
