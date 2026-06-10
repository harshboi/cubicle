# Meta Mailbox

## Round A - Meta

Meta foundation position: Cubicle's foundation should be **typed work objects + typed associations + bounded association-list reads**, with a narrow source metadata contract on source-backed rows. That is Option E with a small slice of Option A. It is not ingestion-first, search-first, or block-first.

Evidence:

- TAO exists because Facebook needed a serving layer for a constantly changing social graph. The TAO paper says the system restricts the model to **objects and associations** and optimizes heavily for reads over that graph: https://www.usenix.org/system/files/conference/atc13/atc13-bronson.pdf
- Meta's TAO engineering writeup says objects and associations are stored persistently in MySQL and cached together by shard. That is the exact lesson for Cubicle: SQL storage is fine if the API and schema discipline are graph-shaped: https://engineering.fb.com/2013/06/25/core-infra/tao-the-power-of-the-graph/
- Facebook's external Graph API exposes the same product model as **nodes, edges, and fields**, not "run a warehouse pipeline first" or "vector search everything first": https://developers.facebook.com/docs/graph-api/reference/
- Unicorn proves search is powerful, but separate. Meta built a graph-aware search/indexing system for social graph search instead of turning the storage graph into a search blob store: https://www.vldb.org/pvldb/vol6/p1150-curtiss.pdf and https://research.facebook.com/publications/unicorn-a-system-for-searching-the-social-graph/
- Ent's public docs model database schema as a graph with typed schemas, edges, code generation, and privacy policies evaluated through generated query paths. That maps directly to Cubicle's Go foundation: https://entgo.io/docs/getting-started/, https://entgo.io/docs/schema-edges/, https://entgo.io/docs/privacy/

Cubicle mapping:

```text
Meta/TAO concept        Cubicle foundation
 |                      |
 +-- object             +-- Person, Workstream, Ticket, PullRequest, Document, Message, Evidence
 +-- association        +-- Ent edge / Through table / LensResult
 +-- association list   +-- WorkLens + WorkLensWindow + paged GraphQL resolver
 +-- count/range read   +-- result_count, rank_score, last_activity_at, cursor
 +-- search sidecar     +-- later SearchChunk, hydrated back into Ent object IDs
```

The foundation API should answer bounded questions first:

```graphql
workstreamBlockers(workstreamID, first, after)
personActivity(personID, workArea, lens, first, after)
ticketContext(ticketID) {
  pullRequests(first: 10)
  messages(first: 10)
  documents(first: 10)
  evidence(first: 10)
}
```

Attack on Palantir foundation choice: Palantir is right that real operational systems need object types, link types, data integration, health, and actions. Their docs define object types as real-world entities/events and link types as relationships between object types, which validates Cubicle's typed ontology: https://palantir.com/docs/foundry/object-link-types/object-types-overview/ and https://palantir.com/docs/foundry/object-link-types/link-types-overview/

But Palantir is wrong as a first foundation if it starts with ingestion/source-run provenance or health. Foundry data integration and health checks are platform machinery for synced datasets and pipelines: https://palantir.com/docs/foundry/data-integration/overview/ and https://palantir.com/docs/foundry/data-integration/health-checks/. Cubicle is a local POC. If PR #68 starts with `SourceConnection`, `IngestionRun`, `ProjectionRun`, and `HealthCheck`, we get pipeline observability before a canonical serving graph can answer "what is blocking launch?" That is infrastructure cosplay. The first trust step should be narrower: source metadata on the typed graph rows that will later be populated by ingestion.

Attack on Glean foundation choice: Glean is right that enterprise knowledge products need connectors, full content analysis, metadata extraction, permissions, faceted search, and hybrid retrieval. Their docs say their crawlers index titles, body copy, comments, metadata, permissions, and facets across 100+ connectors: https://docs.glean.com/security/knowledge-graph and https://docs.glean.com/connectors/connectors-power-glean

But Glean is wrong as a first foundation if it starts with `SearchDocument` and `SearchChunk`. Search chunks are not canonical work state. A chunk can say "OAuth migration blocks launch", but it cannot decide whether the blocker is current, which Jira ticket owns it, which PR resolves it, or whether the Slack evidence is partial. Retrieval before typed graph source metadata creates a parallel truth store: `SearchChunk.visibility_hash` and `SearchChunk.freshness_state` will drift from the canonical `Document`, `Message`, and `LensResult` rows. Search should discover candidate evidence after the graph rows have a shared freshness/visibility contract.

Attack on Obsidian foundation choice: Obsidian is right that graph UX matters. Its official graph view docs say notes are nodes and internal links are edges, and backlinks/local graph make navigation useful: https://obsidian.md/help/plugins/graph

But Obsidian is wrong as a first foundation if it starts with `DocumentSection` and `DocumentBlock`. That makes Google Docs the center of a product that must also model Jira, GitHub, Slack, people, workstreams, blockers, and evidence. Blocks are a content granularity, not the backend foundation. A block-first PR would give precise paragraphs while tickets, PRs, messages, and association rows still lack a common source contract. That is a docs island, not an engineering execution graph.

Meta's demanded foundation decision:

```text
PR #68 = Graph Source Metadata Contract
 |
 +-- SourceMetadataMixin
 |    +-- source_key
 |    +-- source_instance
 |    +-- external_id
 |    +-- source_updated_at
 |    +-- ingested_at
 |    +-- last_seen_at
 |    +-- deleted_at
 |    +-- visibility_hash
 |
 +-- apply to source-backed typed objects
 |    +-- Ticket
 |    +-- PullRequest
 |    +-- Document
 |    +-- DocumentFragment
 |    +-- Message
 |    +-- Evidence
 |
 +-- apply to metadata-bearing associations
      +-- DocumentLensResult
      +-- PullRequestLensResult
      +-- TicketLensResult
      +-- MessageLensResult
```

Do not apply this blindly to `Person` or `Workstream` yet. Those can be aggregate/Cubicle-native objects and may later need `ExternalIdentity` rather than one source row identity.

What breaks if Cubicle copies Meta too literally: TAO-scale sharding/cache infrastructure is not needed for local SQLite. Unicorn-scale search is not needed for PR #68. Facebook privacy complexity is not needed while localhost auth is open. Cubicle should copy the modeling discipline, not the planetary infrastructure.

Concrete foundation order:

```text
1. Meta foundation: typed objects/associations + source metadata contract
2. Palantir sidecar: minimal SourceConnection/IngestionRun/SourceRecord
3. Glean sidecar: SearchDocument/SearchChunk using the same visibility/freshness fields
4. Obsidian surface: DocumentSection/DocumentBlock/local graph using the same canonical IDs
```

What not to build now:

- No generic `nodes` / `edges` table.
- No Cypher-style arbitrary traversal.
- No vector-first RAG.
- No full Foundry-style ingestion, actions, or health platform.
- No docs-only block ontology before graph-wide source metadata.

## Round B - Meta Counterattack

Read before replying:
- graph-foundations-live-chat.md
- graph-foundations-meta.md
- graph-foundations-palantir.md
- graph-foundations-glean.md
- graph-foundations-obsidian.md

I read all five files. The other debaters keep calling Meta "just serving." That is lazy. Serving shape is foundation shape. If the first durable contract is wrong, every later ingestion run, search chunk, and block link will either duplicate identity or drift from graph truth.

### Counterattack On Palantir

Palantir claim quoted:

> "Correct foundation compromise: PR #68 = Ontology Lifecycle Metadata"

Palantir is close, but still mixing layers. I accept the need for freshness, deletion, and visibility on graph rows. I reject calling that "ontology lifecycle metadata" and I reject making it the foundation vocabulary. "Ontology lifecycle" means schema/type lifecycle: active, deprecated, experimental, promoted. "Source observation metadata" means when a source row was observed and whether it is stale, deleted, or visibility-shifted. Those are different foundations.

Real-world evidence cuts against Palantir's broad naming. Foundry separates data integration resources/datasets/transactions from ontology objects and link types. Dataset docs model transaction semantics such as snapshots, appends, updates, and deletes; object/link docs model real-world entity and relationship schemas. Those are not the same surface: https://www.palantir.com/docs/foundry/data-integration/datasets, https://palantir.com/docs/foundry/object-link-types/object-types-overview/, https://palantir.com/docs/foundry/object-link-types/link-types-overview/

Palantir claim quoted:

> "Do not put `source_key` / `source_instance` / `external_id` everywhere in PR #68 as if one source row equals one ontology object."

Accepted in part. Do not put source identity blindly on aggregate objects like `Person` and `Workstream`. But rejecting source identity on source-backed leaf objects is overcorrection. `Ticket`, `PullRequest`, `Document`, `DocumentFragment`, `Message`, and `Evidence` need a direct source observation contract now or every later sidecar has to invent one.

The correct foundation is not:

```text
OntologyLifecycleMetadata
 |
 +-- is_deleted
 +-- permission_policy_key
 +-- visibility_hash
```

That makes reviewers guess whether a field describes source state, object state, permission state, or schema state.

The correct foundation is:

```text
SourceObservationMixin
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

Then later:

```text
ExternalIdentity
 |
 +-- for multi-source identity resolution

IngestionRun / SourceRecord
 |
 +-- for replay and partial crawl debugging

ObjectLifecycle
 |
 +-- for merged / archived / superseded work state

OntologySchemaLifecycle
 |
 +-- for experimental / active / deprecated type definitions
```

Palantir's weakest assumption is that operational trust starts with a provenance platform. It does not. It starts with the canonical rows refusing to be anonymous. TAO's object/association model puts typed rows and typed relationships at the center; Meta's engineering writeup says product features like likes, pages, and events were implemented over objects and associations, and TAO keeps association range and count reads simple rather than building a general query platform first: https://engineering.fb.com/2013/06/25/core-infra/tao-the-power-of-the-graph/ and https://www.usenix.org/system/files/conference/atc13/atc13-bronson.pdf

### Counterattack On Glean

Glean claim quoted:

> "No. That is a serving foundation, not a knowledge foundation."

Wrong. A knowledge system that cannot serve canonical relationships predictably is a search appliance, not a graph foundation. The fact that workplace evidence is scattered does not make retrieval the first durable model. It means retrieval must exist as a sidecar that hydrates into typed objects and associations.

Glean claim quoted:

> "Cubicle PR #68 should be Option D: **search/retrieval sidecar first**, with a tiny trust contract on searchable source items."

Rejected as a first move. Glean is right that connectors index content, metadata, and permissions; its connector docs describe fetching content and permission data into an isolated tenant, and its knowledge graph docs describe full content analysis, metadata extraction, permissions, and facets: https://docs.glean.com/connectors/about and https://docs.glean.com/security/knowledge-graph

But that evidence proves retrieval needs a permission/freshness contract. It does not prove Cubicle should create `SearchDocument` and `SearchChunk` before the canonical graph rows share the same contract.

Bad foundation:

```text
SearchChunk
 |
 +-- visibility_hash = abc
 +-- freshness_state = fresh
 +-- object_id = Document:123

Document
 |
 +-- no same source observation fields

LensResult
 |
 +-- no same visibility/freshness contract
```

That is how a sidecar becomes a parallel truth store. Glean claims chunks are "not canonical work state" and then demands they receive the first trust contract. That is incoherent. If chunks are not canonical, they do not get to define the first system-wide freshness vocabulary.

Meta accepts this much: search is not optional. Unicorn proves Meta itself needed a graph-aware search/index layer for discovery over the social graph: https://www.vldb.org/pvldb/vol6/p1150-curtiss.pdf and https://research.facebook.com/publications/unicorn-a-system-for-searching-the-social-graph/

But Unicorn is separate from TAO. That is the lesson. Search comes as a sidecar after the object/association contract is clear enough to hydrate, explain, and bound results. Cubicle should do the same:

```text
PR #68: SourceObservationMixin on canonical Ent rows
PR #69: minimal ingestion/source record
PR #70: SearchDocument/SearchChunk pointing back to canonical IDs
```

### Counterattack On Obsidian

Obsidian claim quoted:

> "Blocks make exact evidence addressable."

Accepted. Exact evidence addressability matters. Obsidian's docs support links to files, headings, and blocks, and its graph view renders notes as nodes and internal links as edges. That is real product evidence that note-level containers are too coarse for knowledge work: https://obsidian.md/help/links and https://obsidian.md/help/plugins/graph

Obsidian claim quoted:

> "Blocks first create the canonical address that both metadata and search can attach to."

Rejected as Cubicle's foundation. Blocks create one kind of address: document-local content address. Cubicle's foundation must cover Jira tickets, GitHub PRs, Slack messages, Google docs, evidence, and metadata-bearing associations. A block-first PR optimizes for the most inspectable source while leaving the graph-wide source contract fragmented.

Failure:

```text
DocumentBlock
 |
 +-- source_updated_at
 +-- ingested_at
 +-- last_seen_at

Ticket
 |
 +-- no same source observation fields

PullRequest
 |
 +-- no same source observation fields

Message
 |
 +-- no same source observation fields

LensResult
 |
 +-- no same source observation fields
```

That is not a foundation. That is a docs island with better citations than the rest of the graph.

Meta accepts the pattern, not the priority:

```text
Document -> DocumentFragment already exists
 |
 +-- PR #68 gives fragments the shared source observation contract
 |
 +-- later PR can refine fragments into DocumentSection / DocumentBlock
 |
 +-- search chunks can use block IDs when they exist
```

Obsidian's weakest assumption is that "smallest addressable unit" must be settled before "shared trust contract." That is backwards for Cubicle. A precise stale paragraph is still stale. A precise private paragraph is still unsafe. A precise unlinked paragraph is still not a blocker until it is tied to typed work objects.

### Meta Foundation Decision

PR #68 must be **Graph Source Observation Contract**.

Apply to source-backed typed rows:

```text
Ticket
PullRequest
Document
DocumentFragment
Message
Evidence
```

Apply to metadata-bearing associations:

```text
DocumentLensResult
PullRequestLensResult
TicketLensResult
MessageLensResult
```

Do not apply to aggregate/native rows yet:

```text
Person       -> later ExternalIdentity
Workstream   -> can be Cubicle-native or inferred aggregate
WorkArea     -> organizing node, not source row
WorkLens     -> serving/index node, not source row
WorkLensWindow -> serving partition, not source row
```

The real foundation is:

```text
typed Ent object / association
 |
 +-- bounded GraphQL read
 |
 +-- shared source observation metadata when source-backed
 |
 +-- later ingestion explains how metadata was produced
 |
 +-- later search retrieves candidate text using same visibility/freshness terms
 |
 +-- later blocks provide exact anchors using same source metadata
```

What breaks if Cubicle follows each opponent first:

```text
Palantir first
 |
 +-- too much provenance vocabulary before canonical rows have precise source observation semantics

Glean first
 |
 +-- retrieval sidecar becomes the first trust store and drifts from graph truth

Obsidian first
 |
 +-- document blocks become a privileged island while tickets/PRs/messages stay under-specified
```

Final foundation order:

```text
1. PR #68 SourceObservationMixin on source-backed Ent rows and LensResult rows
2. Minimal ingestion/source record for partial crawls and replay
3. Search chunks as retrieval sidecar using the same source observation vocabulary
4. Document sections/blocks as exact anchors using the same source observation vocabulary
5. Local graph API over trusted typed rows
```

Not now:

- No generic node/edge table.
- No Cypher-style graph query language.
- No vector-first RAG.
- No Foundry-style workflow/action/health platform.
- No document-block-first model that treats Google Docs as the center of Cubicle.

## Round C - Meta Foundation Invariants

Read before replying:
- graph-foundations-live-chat.md
- graph-foundations-meta.md
- graph-foundations-palantir.md
- graph-foundations-glean.md
- graph-foundations-obsidian.md

### 1. Canonical identity

Canonical identity is a **Cubicle-owned immutable typed object ID**, not a Jira key, GitHub URL, Slack timestamp, Google Doc URL, search chunk ID, or document block ID.

Meta correction to my earlier phrasing: `source_key`, `source_instance`, and `external_id` are source observation keys. They are not canonical object identity. Palantir is right to attack any design that makes one source row equal one ontology object forever. Jira keys rename, GitHub repos move, Slack messages can be mirrored/exported, and Google Docs can be copied. The canonical `Ticket`, `PullRequest`, `Message`, or `Document` must survive source identity drift.

Real-world evidence:

- Meta TAO uses unique object IDs and typed objects/associations; associations are identified by source object, association type, and destination object, not by mutable display labels. TAO also exposes point/range/count association reads over typed IDs: https://www.usenix.org/system/files/conference/atc13/atc13-bronson.pdf and https://engineering.fb.com/2013/06/25/core-infra/tao-the-power-of-the-graph/
- Facebook Graph API exposes graph resources as nodes/edges/fields, with node IDs as the API identity surface rather than source display names: https://developers.facebook.com/docs/graph-api/reference/
- Palantir separates source data integration/datasets from ontology object and link types, which supports the distinction between raw source identity and operational object identity: https://www.palantir.com/docs/foundry/data-integration/datasets and https://palantir.com/docs/foundry/object-link-types/object-types-overview/

Cubicle invariant:

```text
CubicleObjectID
 |
 +-- typed Ent object identity
 |     -> Ticket, PullRequest, Document, Message, Person, Workstream
 |
 +-- ExternalIdentity later
 |     -> source_key, source_instance, external_id, first_seen_at, last_seen_at
 |
 +-- SourceObservation metadata now
       -> latest known source observation on source-backed rows
```

PR #68 can add source observation metadata to source-backed rows, but it must not claim those fields are canonical identity. `Person` and `Workstream` should not receive direct source identity in PR #68.

### 2. Canonical evidence unit

Canonical evidence is an **Evidence row that points to an addressable source anchor**, not the object alone and not the search chunk alone.

Meta accepts Obsidian's strongest point: whole documents are too coarse. Obsidian supports links to files, headings, and blocks because note-level nodes are not precise enough for knowledge work: https://obsidian.md/help/links and https://obsidian.md/help/plugins/graph

Meta rejects Obsidian's conclusion that `DocumentBlock` must be the first backend foundation. Cubicle evidence spans Slack messages, Jira comments, PR reviews, Google Doc paragraphs, and association rows. The common durable citation wrapper should be `Evidence`; the addressable anchor can be `Message`, `DocumentFragment`, future `DocumentBlock`, future `SearchChunk`, or a source record.

Real-world evidence:

- TAO models content and actions as typed objects and relationships as typed associations; if something has its own content/lifecycle, model it as an object, then connect it with associations: https://engineering.fb.com/2013/06/25/core-infra/tao-the-power-of-the-graph/
- Glean's connector docs support the fact that workplace evidence lives in titles, bodies, comments, media, metadata, permissions, and source-specific structures, not only document blocks: https://docs.glean.com/connectors/about and https://docs.glean.com/security/knowledge-graph
- Obsidian proves exact anchors matter, but its anchor model is local content/navigation, not operational source governance: https://obsidian.md/help/links

Cubicle invariant:

```text
Evidence
 |
 +-- evidence_kind
 +-- target_object_kind / target_object_id
 +-- anchor_kind / anchor_id
 |     -> Message
 |     -> DocumentFragment today
 |     -> DocumentBlock later
 |     -> SearchChunk later
 |     -> SourceRecord later
 |
 +-- quote_hash / source_span later
 +-- confidence / visibility / observed_at
```

This lets `whyBlocked(workstream)` cite a blocker edge plus the exact anchor that supports it. It also avoids making search chunks or document blocks the canonical proof layer before the graph has object/association semantics.

### 3. Freshness authority

Freshness authority is layered. No single row gets to speak for every failure mode.

```text
SourceRun / IngestionRun
 |
 +-- authority for crawl completeness:
 |     complete, partial, failed, cursor, scope

SourceRecord / SourceObservation
 |
 +-- authority for raw source observation:
 |     last_seen_at, source_updated_at, deleted_at, visibility_hash

Ontology object / association row
 |
 +-- serving cache of projected freshness:
 |     can say "this row was last observed at X"
 |     cannot alone say "the source crawl was complete"

SearchChunk / DocumentBlock
 |
 +-- inherits freshness from source observation/run
 |     cannot invent independent freshness truth
```

Palantir is right that partial Slack crawls and pipeline health cannot be solved by object metadata alone. Foundry datasets model transactions over time, and Foundry health checks exist because synced/transformed/scheduled data must be validated for reliability and freshness: https://www.palantir.com/docs/foundry/data-integration/datasets and https://www.palantir.com/docs/foundry/data-integration/health-checks

But Palantir is wrong if it makes `IngestionRun` PR #68. The current Cubicle graph already has source-backed rows with no shared observation contract. The first foundation must stop those rows from being anonymous; the next PR can add source runs to explain partiality.

Cubicle invariant:

```text
PR #68 fields may say:
 |
 +-- source_updated_at
 +-- ingested_at
 +-- last_seen_at
 +-- deleted_at
 +-- visibility_hash

PR #68 fields may not say:
 |
 +-- crawl_complete
 +-- crawl_partial
 +-- source_health_green
 +-- projection_authoritative
```

Partial/complete belongs to the later ingestion/source-run layer. Object/association rows carry derived observation metadata for fast Meta-style serving.

### 4. Retrieval/addressability ordering

Ordering: **shared source/freshness metadata first**, then ingestion run/source records, then retrieval chunks and document blocks.

Glean is right that retrieval is mandatory. Its connector docs say connectors fetch content and permission data into an isolated tenant, and its knowledge graph docs describe full content analysis, metadata extraction, permissions, and facets: https://docs.glean.com/connectors/about and https://docs.glean.com/security/knowledge-graph

Glean is wrong that `SearchDocument` / `SearchChunk` should be PR #68. Search chunks are not canonical work state. If chunks receive `freshness_state` and `visibility_hash` before canonical `Document`, `Message`, `Evidence`, and `LensResult` rows share those semantics, search becomes the first trust store and drifts from the graph.

Obsidian is right that addressability matters. Its block/heading links are the strongest evidence that exact anchors are product-critical: https://obsidian.md/help/links

Obsidian is wrong that `DocumentSection` / `DocumentBlock` should be PR #68. Blocks are one source's addressability model. Cubicle also needs Slack messages, PR comments, Jira comments, association evidence, and source records. A block-first move creates a document island before the graph-wide trust contract exists.

Meta evidence for ordering:

- TAO keeps the serving API simple: object operations plus association point/range/count reads. It explicitly avoids complex online graph traversal/pattern matching. That validates a typed, bounded graph foundation before search or arbitrary traversal: https://engineering.fb.com/2013/06/25/core-infra/tao-the-power-of-the-graph/
- Unicorn shows search is separate and powerful, but not the storage graph itself. Cubicle should copy that separation: https://www.vldb.org/pvldb/vol6/p1150-curtiss.pdf and https://research.facebook.com/publications/unicorn-a-system-for-searching-the-social-graph/
- Ent models schemas as a graph in Go with typed schemas/edges and privacy hooks, which is the correct local implementation substrate for Cubicle's canonical graph: https://entgo.io/docs/getting-started/, https://entgo.io/docs/schema-edges/, https://entgo.io/docs/privacy/

Cubicle ordering:

```text
1. Shared SourceObservationMixin
 |   -> makes existing graph rows carry source observation and visibility terms
 |
2. Minimal IngestionRun / SourceRecord / ExternalIdentity
 |   -> explains partial crawls, replay, and source identity drift
 |
3. SearchDocument / SearchChunk
 |   -> retrieves unlinked evidence using the same trust vocabulary
 |
4. DocumentSection / DocumentBlock
 |   -> gives exact doc anchors using the same trust vocabulary
 |
5. LocalGraph / whyBlocked APIs
     -> fast bounded answers with inspectable evidence
```

Non-negotiable foundation invariant:
Canonical Cubicle truth is a typed Ent object or typed Ent association with a Cubicle-owned immutable ID, and every source-backed row or evidence-bearing association must use one shared source observation vocabulary before search chunks, document blocks, or ingestion projections define their own trust semantics.

Concession to strongest opposing camp:
Palantir has the strongest foundation objection: object metadata alone cannot prove partial crawl completeness or source lineage. Cubicle must add `IngestionRun`, `SourceRecord`, and `ExternalIdentity` immediately after PR #68. Glean and Obsidian are also right that unlinked text and exact blocks are essential, but they should inherit the shared source observation contract rather than invent it.

Concrete PR #68 after this invariant:
Add `SourceObservationMixin` to source-backed Ent rows and evidence-bearing association rows only: `Ticket`, `PullRequest`, `Document`, `DocumentFragment`, `Message`, `Evidence`, `DocumentLensResult`, `PullRequestLensResult`, `TicketLensResult`, and `MessageLensResult`. Fields: `source_key`, `source_instance`, `external_id`, `source_updated_at`, `ingested_at`, `last_seen_at`, `deleted_at`, `visibility_hash`. Document that these fields are observation/provenance keys, not canonical object identity, not crawl completeness, not schema lifecycle, and not retrieval/block identity.

## Round D - Meta Adversarial Design Review

Read before replying:
- graph-foundations-live-chat.md
- graph-foundations-meta.md
- graph-foundations-palantir.md
- graph-foundations-glean.md
- graph-foundations-obsidian.md

### 1. Accept/reject the moderator candidate

| Decision | Concrete row/table failure | Concrete product query that fails |
|---|---|---|
| **Reject as written; accept with one hard correction.** | The candidate has `ExternalIdentity`, `SourceObservation`, and `EvidenceAnchor`, but no `SourceRun`. `SourceObservation` cannot say whether absence means deleted, permission-hidden, or never crawled because Slack failed halfway. | `whyBlocked(workstreamID)` returns no Slack evidence for a blocker. Without `SourceRun(status=partial, scope=channel/time)`, the API cannot distinguish "no messages exist" from "the crawl did not cover that channel window." |

Moderator candidate is otherwise the first design that stops the debate from confusing three different jobs:

```text
ExternalIdentity
 |
 +-- source identity relation
 +-- not canonical object identity

SourceObservation
 |
 +-- what one source item looked like when observed
 +-- not crawl completeness

EvidenceAnchor
 |
 +-- exact citation address/span
 +-- not search index and not document-only block model
```

Meta accepts the spine because it keeps the canonical graph typed. TAO's public model is still the right serving discipline: typed objects, typed associations, association lists, point/range/count reads, and no arbitrary online pattern matching. Sources: https://engineering.fb.com/2013/06/25/core-infra/tao-the-power-of-the-graph/ and https://www.usenix.org/system/files/conference/atc13/atc13-bronson.pdf

But the candidate must add `SourceRun` in PR #68 or it fails the partial Slack crawl requirement. Palantir is correct here: source health/coverage is not derivable from item rows. Foundry datasets and health checks exist because pipeline state and data state are separate concerns. Sources: https://www.palantir.com/docs/foundry/data-integration/datasets and https://www.palantir.com/docs/foundry/data-integration/health-checks

Glean is correct that content and permissions must be retrievable, but wrong to make `SearchChunk` the first trust row. Glean's connector docs prove the need for content plus permission indexing, not that search rows should become source authority. Sources: https://docs.glean.com/connectors/about and https://docs.glean.com/security/knowledge-graph

Obsidian is correct that exact anchors matter, but wrong to make `DocumentBlock` the first source-neutral foundation. Obsidian block links prove addressability, not document supremacy. Source: https://obsidian.md/help/links

### 2. Minimal PR #68 schema

Only add the foundation spine. Do **not** mutate existing ontology objects with source fields in this PR. Keep Ent typed objects as the serving graph and add source/provenance rows beside them. Ent is explicitly built around typed schema-as-code and graph edges, which fits this split: https://entgo.io/docs/getting-started/ and https://entgo.io/docs/schema-edges/

| Ent schema | Field | Mark | Purpose |
|---|---|---|---|
| `SourceRun` | `source_key` | identity | Source system, e.g. `slack`, `jira`, `github`, `google_docs`. |
| `SourceRun` | `source_instance` | identity | Workspace/repo/site/account namespace. |
| `SourceRun` | `run_key` | identity | Connector-generated stable run id. |
| `SourceRun` | `scope_kind` | identity | Crawl scope type, e.g. `workspace`, `channel`, `project`, `repo`, `document`. |
| `SourceRun` | `scope_external_id` | identity | Source-local scope id. |
| `SourceRun` | `status` | freshness | `running`, `complete`, `partial`, `failed`, `rate_limited`. |
| `SourceRun` | `started_at` | freshness | Run start time. |
| `SourceRun` | `completed_at` | freshness | Run completion time, nullable. |
| `SourceRun` | `checkpoint` | freshness | Opaque connector cursor/checkpoint. |
| `SourceRun` | `error_message` | freshness | Failure/partial reason, bounded text. |
| `ExternalIdentity` | `source_key` | identity | Source system for this external id. |
| `ExternalIdentity` | `source_instance` | identity | Source namespace for this external id. |
| `ExternalIdentity` | `external_id` | identity | Source-owned id/key/url component. |
| `ExternalIdentity` | `target_kind` | identity | Typed Cubicle target kind, constrained enum. |
| `ExternalIdentity` | `target_id` | identity | Cubicle-owned Ent object id for that kind. |
| `ExternalIdentity` | `identity_state` | identity | `active`, `alias`, `retired`, `redirected`, `merged`. |
| `ExternalIdentity` | `first_seen_at` | freshness | First time Cubicle observed this identity. |
| `ExternalIdentity` | `last_seen_at` | freshness | Last time Cubicle observed this identity. |
| `ExternalIdentity` | `retired_at` | freshness | When this source identity stopped being primary, nullable. |
| `ExternalIdentity` | `redirect_external_id` | identity | Source-level redirect/merge target, nullable. |
| `SourceObservation` | `external_identity_id` | identity | The source identity being observed. |
| `SourceObservation` | `source_run_id` | freshness | The run that produced this observation. |
| `SourceObservation` | `observed_at` | freshness | When Cubicle observed this source item. |
| `SourceObservation` | `source_updated_at` | freshness | Source-reported update time, nullable. |
| `SourceObservation` | `observation_state` | freshness | `observed`, `deleted`, `unreachable`, `permission_denied`, `redirected`. |
| `SourceObservation` | `visibility_hash` | permission | Stable hash of source ACL/visibility state. |
| `SourceObservation` | `content_hash` | citation | Hash of observed content or normalized source item. |
| `SourceObservation` | `source_url` | citation | Deep link to source item when available. |
| `SourceObservation` | `title` | citation | Human-readable source title/key preview. |
| `EvidenceAnchor` | `source_observation_id` | citation | Observation containing the cited evidence. |
| `EvidenceAnchor` | `anchor_kind` | citation | `whole_item`, `span`, `comment`, `message`, `block`, `review_comment`. |
| `EvidenceAnchor` | `anchor_locator` | citation | Source-local locator: paragraph id, comment id, Slack ts, byte/char range label. |
| `EvidenceAnchor` | `source_span_start` | citation | Optional normalized start offset. |
| `EvidenceAnchor` | `source_span_end` | citation | Optional normalized end offset. |
| `EvidenceAnchor` | `text_hash` | citation | Hash of cited text/span. |
| `EvidenceAnchor` | `preview` | citation | Short bounded preview for local UI; not a search corpus. |

This is intentionally not a generic graph. `target_kind` / `target_id` in `ExternalIdentity` is a provenance pointer, not a graph traversal API. The canonical graph remains typed Ent objects and typed Ent associations. If reviewers want stricter Ent typing later, replace `target_kind` / `target_id` with typed optional edges, but do not block PR #68 on schema ceremony.

### 3. Four failure-mode proof

| Failure | How PR #68 answers it | Query path |
|---|---|---|
| Partial Slack crawl | `SourceRun(source_key=slack, scope_kind=channel, status=partial, checkpoint=...)` is the authority for coverage. `SourceObservation` rows for seen messages are valid; missing messages inside that scope are **unknown**, not deleted. | `whyBlocked(workstreamID)` reads typed blocker association list, joins message evidence anchors, and returns `sourceRun.status=partial` as a warning. |
| Renamed/merged/deleted Jira issue | `ExternalIdentity(jira, old_key, target=Ticket, identity_state=retired/redirected)` preserves old keys. `ExternalIdentity(jira, new_key, same Ticket)` preserves continuity. `SourceObservation(observation_state=deleted/redirected, redirect_external_id=...)` records source state. | `ticket(idOrAlias:"CUB-123")` resolves through `ExternalIdentity` to the same `Ticket`, then exposes old identity state and latest Jira observation. |
| Unlinked but relevant Google Doc paragraph | `SourceObservation` records the Google Doc item. `EvidenceAnchor(anchor_kind=span/block, anchor_locator=heading/paragraph/span, text_hash=...)` gives a durable citation without making `SearchChunk` or `DocumentBlock` canonical. | A later retrieval job can index anchors; before that, any promoted evidence can still cite the exact doc paragraph through `EvidenceAnchor -> SourceObservation -> source_url`. |
| Fast "why is launch blocked?" with inspectable evidence | Fast path remains Meta-style: `Workstream -> WorkLensWindow -> TicketLensResult -> Ticket` and bounded related associations. Evidence inspection uses `EvidenceAnchor`; freshness/coverage warnings use `SourceRun` and `SourceObservation`. | `whyBlocked(workstreamID, first:20)` does bounded Ent reads first, then hydrates anchors and source run statuses for the returned blockers. No Cypher, no vector search, no document-wide scan. |

This is the split the public systems imply:

```text
Meta TAO
 |
 +-- typed serving graph, bounded reads

Palantir Foundry
 |
 +-- source/data state and operational ontology are distinct

Glean
 |
 +-- retrieval needs content + permissions, but index rows are projections

Obsidian
 |
 +-- exact anchors matter, but blocks are source-specific addresses
```

### 4. What must NOT be in PR #68

| Cut from PR #68 | Why it must wait |
|---|---|
| `SearchDocument` / `SearchChunk` | Retrieval projection. It should index `EvidenceAnchor` or source observations after the source spine exists. Building it first makes the index the first trust store. |
| `DocumentSection` / `DocumentBlock` | Document-specific projection. It should become one producer/consumer of `EvidenceAnchor`, not the source-neutral foundation. |
| Generic `nodes` / `edges` | Direct violation of the Ent/TAO typed object-association model. |
| Cypher-like query language | TAO explicitly avoids arbitrary online traversal/pattern matching; Cubicle needs bounded GraphQL reads first. Source: https://engineering.fb.com/2013/06/25/core-infra/tao-the-power-of-the-graph/ |
| Vector DB / embeddings / RAG answers | Glean-style retrieval is necessary later; vector-first answer generation before identity/freshness/evidence is a correctness bug. |
| Full Foundry-style health/action/workflow engine | Foundry-scale health/actions are useful later. PR #68 only needs `SourceRun` status, not a platform. |
| Source fields pasted onto every ontology object | Palantir is right: source identity and ontology identity differ. Keep source identity in `ExternalIdentity`. |
| Fields on `Person`, `Workstream`, `WorkArea`, `WorkLens`, `WorkLensWindow` | These can be aggregate/native/serving nodes. They are not source items in PR #68. |
| Raw JSON payload storage | Too large and too source-specific. Add `SourceRecord` with payload/ref later if replay needs it. |

Final PR #68 recommendation:
Add the source-neutral foundation spine: `SourceRun`, `ExternalIdentity`, `SourceObservation`, and `EvidenceAnchor`. Keep canonical graph identity in existing typed Ent objects and associations. Use PR #68 only to separate source coverage, source identity, source observation, and exact evidence citation.

PR #69 immediately after:
Wire the existing `Evidence` schema and the first GraphQL read path to the spine: add `Evidence -> EvidenceAnchor`, add `sourceHealth`/`evidenceAnchors` fields for blocker/ticket context queries, and seed one Slack/Jira/Docs fixture proving partial crawl, Jira rename/delete, and doc paragraph citation.

What I would block in review:
I would block any PR #68 that creates `SearchChunk` or `DocumentBlock` before `EvidenceAnchor`, any PR that puts `source_key/source_instance/external_id` directly on all ontology objects as identity, any PR that lacks `SourceRun.status`, any PR that stores raw source JSON as the foundation, and any PR that turns this spine into a generic graph query model.

## Round E - Meta Implementation Review

Read before replying:
- graph-foundations-live-chat.md
- graph-foundations-meta.md
- graph-foundations-palantir.md
- graph-foundations-glean.md
- graph-foundations-obsidian.md

### 1. Ent implementation risk

`target_kind` + `target_id` is acceptable for PR #68 **only as a provenance pointer**, not as a graph edge substitute.

Strict typed optional edges are the wrong first migration:

| Choice | Migration/codegen pain | Query pain | Review verdict |
|---|---|---|---|
| `ExternalIdentity.target_kind` + `target_id` | One table, stable columns, one generated Ent schema. No FK, so integrity must be enforced by constrained enums, helper resolvers, tests, and hooks. | Query resolves identity through `target_kind` switch, then calls the typed Ent client for `Ticket`, `PullRequest`, `Document`, etc. | Accept for PR #68. This is source identity metadata, not the serving graph. |
| Typed optional edges on `ExternalIdentity` | Requires nullable FK columns/edges for every possible target: `ticket_id`, `pull_request_id`, `document_id`, `message_id`, `person_id`, later more. Every new ontology type changes the schema, migration, generated code, tests, and GraphQL hydration. | Query still needs a switch because exactly one optional edge should be set. Codegen gives many optional edge APIs but no single polymorphic identity resolver. | Block for PR #68. Over-typed for the source spine. |
| Generic `node_id` / `edge_id` | Easy migration, bad model. It bypasses Ent typed object/association discipline. | Turns the source spine into a generic graph store. | Block. |

Evidence: Ent's docs model edges as relations between concrete entity types, and generated graph traversal/codegen follows those typed relations: https://entgo.io/docs/schema-edges/. Ent's schema/index docs support composite and unique indexes on fields and edges, so the right PR #68 pressure is strong uniqueness/indexing on the source spine, not fake polymorphic edge sprawl: https://entgo.io/docs/schema-indexes/. Ent hooks can enforce mutation-time invariants where DB FKs cannot express polymorphic targets: https://entgo.io/docs/hooks/.

Hard implementation rule:

```text
ExternalIdentity.target_kind + target_id
 |
 +-- allowed for identity resolution
 +-- must use constrained enum target_kind
 +-- must not be used for graph traversal
 +-- must have one resolver package:
      identity.Resolve(ctx, client, source_key, source_instance, external_kind, external_id)
```

`EvidenceAnchor -> SourceObservation` and `SourceObservation -> SourceRun` must be normal typed Ent edges. No excuse. Those are not polymorphic.

### 2. Identity uniqueness

The current ruling is missing one field: `external_kind`. Without it, `external_id` collisions across source item classes are inevitable. Slack channel IDs, message timestamps, thread IDs, Jira issue IDs, Jira comments, GitHub PR numbers, and GitHub review comments are not all the same identity namespace.

Minimal correction:

```text
ExternalIdentity
 |
 +-- source_key
 +-- source_instance
 +-- external_kind
 +-- external_id
 +-- target_kind
 +-- target_id
 +-- identity_status
```

Required unique indexes:

| Table | Required index | Why |
|---|---|---|
| `SourceRun` | unique `(source_key, source_instance, run_key)` | Makes connector writes idempotent. Without `run_key`, retrying the same run creates duplicate coverage truth. If the ruling refuses `run_key`, use unique `(source_key, source_instance, scope_kind, scope_key, started_at)`, but that is weaker. |
| `SourceRun` | non-unique `(source_key, source_instance, scope_kind, scope_key, started_at)` | Fast latest-run lookup for `sourceCoverage` and `whyBlocked` warnings. |
| `ExternalIdentity` | unique `(source_key, source_instance, external_kind, external_id)` | Prevents Jira rename, GitHub repo move, Slack mirror, or Google Doc copy from creating two active canonical mappings for the same source identity. |
| `ExternalIdentity` | non-unique `(target_kind, target_id, identity_status)` | Lists all aliases/renames for a canonical object. |
| `SourceObservation` | unique `(source_run_id, external_identity_id)` | One source item observation per run. Prevents duplicate rows on retry. |
| `SourceObservation` | non-unique `(external_identity_id, observed_at)` | Observation history for "what did source say over time?" |
| `EvidenceAnchor` | unique `(source_observation_id, anchor_kind, anchor_locator)` | Prevents duplicate paragraph/comment/message anchors inside one observed item. |
| `EvidenceAnchor` | non-unique `(source_observation_id, ordinal)` | Bounded ordered anchor reads inside a source item. |

Concrete corruption if these are absent:

```text
Jira rename
 |
 +-- CUB-123 retired
 +-- PLAT-77 active
 `-- without unique ExternalIdentity, both can accidentally map to different Ticket rows

Slack mirrored message
 |
 +-- slack workspace A channel C ts T
 +-- exported/mirrored source instance B channel C ts T
 `-- source_instance must be part of identity, or mirrors collide

Google Doc copy
 |
 +-- old doc id and copied doc id are different external identities
 `-- content_hash equality must not merge them automatically

GitHub repo move
 |
 +-- old org/repo#88 and new org/repo#88 may map to same PR after resolution
 `-- ExternalIdentity rows preserve alias history without mutating PullRequest identity
```

Ent supports composite/unique indexes directly in schema, and dialect-specific index controls through Atlas annotations when needed: https://entgo.io/docs/schema-indexes/. Use that. Do not rely on application-side "find then insert" checks.

### 3. Anchor text risk

`EvidenceAnchor.anchor_text` as proposed is too easy to abuse. It can leak source text, bloat SQLite, and quietly recreate `SearchChunk`.

The smaller PR #68 field set should be:

| Field | Keep? | Reason |
|---|---|---|
| `text_hash` | Yes | Stable drift/dedupe proof without storing full content. |
| `anchor_preview` | Yes, bounded to 240-512 chars | UI inspection and Day 0 lexical demo only. Must be source text, not LLM summary. |
| `anchor_terms` | Optional, bounded | Sanitized lowercase terms for local `LIKE`/simple token search. This is not a document body. |
| `anchor_text` full span | No | This becomes `SearchChunk` under a different name. |
| FTS virtual table | No in PR #68 | SQLite FTS5 can duplicate stored content unless configured as contentless/external-content; that is an implementation detail for PR #69, not the foundation. SQLite FTS5 docs describe external/contentless options precisely because naive FTS stores a private content copy: https://sqlite.org/fts5.html |

Day 0 lexical lookup can use:

```text
EvidenceAnchor
 |
 +-- anchor_preview        // bounded visible snippet
 +-- anchor_terms          // bounded normalized search terms, optional
 +-- text_hash             // exact cited text hash
```

Illegal shape:

```text
EvidenceAnchor
 |
 +-- anchor_text = entire Google Doc paragraph/page/thread
 +-- FTS index over anchor_text
```

That is `SearchChunk` without the honesty of naming it. Block it.

### 4. Permission/freshness correctness

This query is illegal:

```sql
SELECT ea.id, ea.anchor_preview, so.source_url
FROM evidence_anchors ea
JOIN source_observations so ON so.id = ea.source_observation_id
JOIN external_identities ei ON ei.id = so.external_identity_id
WHERE ea.anchor_preview LIKE '%launch blocked%';
```

It is illegal because it ignores:

```text
visibility_hash
permission_policy_key
SourceRun.status
SourceObservation.is_deleted
```

Minimum legal query shape:

```sql
SELECT ea.id, ea.anchor_preview, so.source_url, sr.status
FROM evidence_anchors ea
JOIN source_observations so ON so.id = ea.source_observation_id
JOIN source_runs sr ON sr.id = so.source_run_id
JOIN external_identities ei ON ei.id = so.external_identity_id
WHERE ea.anchor_preview LIKE ?
  AND so.is_deleted = false
  AND so.visibility_hash IN (:viewer_visibility_hashes)
  AND so.permission_policy_key IN (:viewer_policy_keys)
  AND sr.status IN ('complete', 'partial')
ORDER BY so.observed_at DESC
LIMIT :first;
```

If `sr.status = 'partial'`, the result may be returned as evidence, but the answer must carry a source coverage warning. It is illegal to make absence-based claims from partial runs:

```text
Illegal:
 |
 +-- "No Slack discussion exists for this blocker"
      when latest Slack SourceRun is partial/failed/rate_limited

Legal:
 |
 +-- "No Slack discussion found in observed data; Slack crawl is partial"
```

If `so.is_deleted = true`, the anchor can be used only as historical evidence, never as current proof. If the query is `whyBlocked(current)`, deleted observations must be excluded unless the API explicitly asks for history.

This is where Glean and Palantir are both right: permissions and freshness must be evaluated before serving answers. Glean connector docs emphasize source permissions and indexed retrieval under those constraints: https://docs.glean.com/connectors/about. Palantir health/data docs separate data state and health/freshness from ontology usage: https://www.palantir.com/docs/foundry/data-integration/health-checks.

### 5. PR boundary

Cut the PR harder.

Keep in PR #68:

```text
SourceRun
ExternalIdentity
SourceObservation
EvidenceAnchor
indexes
enum constants
schema tests
minimal write helpers for idempotent upsert
README note explaining the spine is not search/block/graph traversal
```

Cut from PR #68:

```text
GraphQL contextSearch
GraphQL whyBlocked
SearchDocument
SearchChunk
DocumentSection
DocumentBlock
SQLite FTS5 setup
embeddings
RAG answer generation
raw payload blobs
ProjectionRun
HealthCheck
scheduler/retry engine
permission engine
generic graph resolver over target_kind/target_id
```

PR #68 should be boring generated Ent schema plus tests. If it has product APIs, it is too large. If it has FTS, it is too large. If it writes raw documents, it is too large.

Approve PR #68 only if:
It adds exactly the Source Evidence Spine tables (`SourceRun`, `ExternalIdentity`, `SourceObservation`, `EvidenceAnchor`), uses `target_kind + target_id` only as a constrained provenance pointer, adds `external_kind`, includes required unique indexes, keeps anchor text bounded as `anchor_preview`/`anchor_terms` rather than full text, and proves with tests that partial source runs cannot tombstone missing data.

Block PR #68 if:
It adds `SearchChunk`, `DocumentBlock`, FTS, embeddings, GraphQL search APIs, raw payload blobs, direct `source_key/external_id` identity fields on ontology objects, nullable typed target edges for every object kind, or any query path that returns anchor text without filtering `visibility_hash`, `permission_policy_key`, `SourceRun.status`, and `SourceObservation.is_deleted`.

One migration/index I insist on:
`ExternalIdentity` must have a unique composite index on `(source_key, source_instance, external_kind, external_id)`. Without this, Jira renames, GitHub repo moves, Slack mirrors, and Google Doc copies will eventually create duplicate or corrupt canonical object mappings.
