# Cubicle Graph Foundations Live Chat

## Protocol

- This is a direct shared debate workspace. Each debater must read this file and all peer files before replying.
- Each debater writes to their own mailbox file so everyone can read everyone else at any time without file conflicts.
- Reply directly to named debaters and quote the claim you are attacking from their file.
- Do not wait for moderator summaries or turn order. Read the latest visible files and respond.
- The moderator only extracts final decisions.
- Stay hostile to weak architecture, but do not attack people.
- Every major claim must cite real-world evidence: paper, official docs, engineering blog, customer implementation, or public product documentation.
- Focus only on foundations for Cubicle's graph/backend. Do not move into implementation unless the foundation is settled.

## Debaters

```text
Meta      -> social graph / TAO / Unicorn / Graph API / Ent-style typed graph
Palantir  -> Foundry ontology / data integration / operational workflows
Glean     -> enterprise knowledge graph / connectors / permissions / search/RAG
Obsidian  -> local-first notes / links / backlinks / blocks / graph UX
```

## Mailbox Files

```text
.codex/agent-instructions/graph-foundations-meta.md
.codex/agent-instructions/graph-foundations-palantir.md
.codex/agent-instructions/graph-foundations-glean.md
.codex/agent-instructions/graph-foundations-obsidian.md
```

Every response must start with:

```text
Read before replying:
- graph-foundations-live-chat.md
- graph-foundations-meta.md
- graph-foundations-palantir.md
- graph-foundations-glean.md
- graph-foundations-obsidian.md
```

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

## Foundation Question

Before deciding PR #68, what is the correct foundation for Cubicle?

```text
Option A: source/lifecycle metadata first
Option B: ingestion/source-run provenance first
Option C: document blocks/chunks first
Option D: search/retrieval sidecar first
Option E: another foundation backed by evidence
```

## Evidence Requirement

Each debater must provide:

1. Real-world example of the system/product implementing the pattern.
2. Source citation URL.
3. How the pattern maps to Cubicle schemas/query APIs.
4. What breaks if Cubicle copies it too literally.

## Round A Prompt

Make the foundation case from real-world evidence. Attack the other foundations directly.

## Round B Prompt

Read all mailbox files directly. Attack the specific claims written by other debaters. Do not rely on moderator summaries. Use real-world implementation evidence, not abstract preference.

## Moderator Challenge - Round C

The debate is getting stuck on "my PR #68 first." Stop arguing only priority. Answer the foundation invariants.

Every debater must answer these four questions directly:

```text
1. Canonical identity
   What is the stable identity of a Cubicle work object when Jira keys rename,
   GitHub repos move, Slack messages are mirrored, or Google Docs are copied?

2. Canonical evidence unit
   What is the smallest durable thing Cubicle can cite as evidence:
   object, document, block/chunk, source record, association row, or something else?

3. Freshness authority
   Which row/layer is allowed to say "this thing is fresh/stale/partial/deleted":
   source run, source record, ontology object, association row, search chunk, document block?

4. Retrieval/addressability ordering
   Should Cubicle define addressable blocks first, searchable chunks first,
   or shared source/freshness metadata first? Defend with real system evidence.
```

Moderator constraint:

```text
The final foundation must support all four real failures:
 |
 +-- partial Slack crawl
 +-- renamed/merged/deleted Jira issue
 +-- unlinked but relevant Google Doc paragraph
 +-- fast "why is launch blocked?" answer with inspectable evidence
```

Round C output must end with:

```text
Non-negotiable foundation invariant:
...

Concession to strongest opposing camp:
...

Concrete PR #68 after this invariant:
...
```

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

## Moderator Ruling - After Round C

The four camps now agree on three hard invariants:

```text
Canonical object identity
 |
 +-- Cubicle-owned typed Ent ID
 `-- never Jira key, GitHub URL, Slack timestamp, Google Doc URL, chunk ID, or block ID

Freshness authority
 |
 +-- source run/checkpoint says crawl scope is complete/partial/failed
 +-- source observation says what one source item looked like at one time
 +-- ontology object/association exposes derived serving freshness
 `-- search chunk/document block inherits freshness; it does not invent it

Evidence precision
 |
 +-- whole objects and whole docs are too coarse
 +-- user-visible claims need exact evidence anchors
 `-- search and block systems should index/resolve anchors, not become canonical identity
```

The unresolved split is only first implementation order:

```text
Meta       -> shared source observation fields on typed graph rows first
Palantir   -> lifecycle/trust metadata first, but source identity separate
Glean      -> SearchDocument/SearchChunk first for unlinked evidence
Obsidian   -> DocumentSection/DocumentBlock first for exact citations
```

Moderator position for the next challenge:

```text
PR #68 should not make either SearchChunk or DocumentBlock the canonical foundation.
 |
 +-- SearchChunk is a retrieval/index projection.
 +-- DocumentBlock is a document-shaped evidence projection.
 `-- Both should attach to a smaller source-neutral evidence/provenance spine.

PR #68 also should not put source_key/source_instance/external_id directly on every ontology object
as if source identity and object identity were the same thing.
 |
 +-- Source identity belongs in ExternalIdentity.
 +-- Observed source state belongs in SourceObservation.
 +-- Crawl coverage belongs in SourceRun/SourceCheckpoint.
 `-- Exact citations belong in EvidenceAnchor.
```

Tentative minimal foundation candidate:

```text
Cubicle typed object
 |
 +-- ExternalIdentity
 |     -> source system identity for one object/source item
 |
 +-- SourceObservation
 |     -> observed source state, freshness, deletion, visibility, content hash
 |
 +-- EvidenceAnchor
 |     -> exact address/span inside an observation
 |
 +-- SearchChunk later
 |     -> retrieval projection over EvidenceAnchor
 |
 +-- DocumentBlock later
       -> document-specific projection over EvidenceAnchor
```

Why this beats the current split:

```text
Current proposed split             Moderator candidate
 |                                  |
 +-- metadata-only first            +-- identity + observation + anchor spine
 |   -> cannot cite exact text       |   -> can cite exact text later without making search/block canonical
 |
 +-- search-first                   +-- retrieval remains projection
 |   -> index becomes truth          |   -> index hydrates back to object + anchor
 |
 +-- block-first                    +-- anchors stay source-neutral
 |   -> Google Docs island           |   -> Slack/Jira/GitHub/Docs all fit same citation contract
 |
 +-- source fields on objects        +-- ExternalIdentity separates source identity
     -> identity confusion               -> source renames/copies/merges do not replace Cubicle object
```

## Moderator Challenge - Round D

Every debater must read all mailbox files directly again and respond to the moderator candidate above.

Do not write a philosophy essay. Produce a concrete adversarial design review.

Required output:

```text
Read before replying:
- graph-foundations-live-chat.md
- graph-foundations-meta.md
- graph-foundations-palantir.md
- graph-foundations-glean.md
- graph-foundations-obsidian.md
```

Then answer side-by-side:

```text
1. Accept/reject the moderator candidate.
   If rejecting, name the exact row/table that fails and the concrete product query that fails.

2. Minimal PR #68 schema.
   List only the Ent schemas and fields you would add in one small PR.
   Mark each field as:
   - identity
   - freshness
   - permission
   - citation
   - serving optimization

3. Four failure-mode proof.
   Show exactly how the schema answers:
   - partial Slack crawl
   - renamed/merged/deleted Jira issue
   - unlinked but relevant Google Doc paragraph
   - fast "why is launch blocked?" with inspectable evidence

4. What must NOT be in PR #68.
   Be brutal. Cut anything that can wait.
```

Round D must end with:

```text
Final PR #68 recommendation:
...

PR #69 immediately after:
...

What I would block in review:
...
```

## Moderator Ruling - After Round D

Round D produced a real convergence.

All four camps now accept this foundation shape, with one mandatory amendment:

```text
SourceRun
 |
 +-- crawl coverage and partial/failure authority

ExternalIdentity
 |
 +-- source identity and alias/rename/merge history

SourceObservation
 |
 +-- observed source item state, visibility, content hash, deletion/freshness

EvidenceAnchor
 |
 +-- exact citeable span/address inside an observed item
```

The amendment is not optional:

```text
SourceRun must be in PR #68.
 |
 `-- Without it, partial Slack crawl cannot be represented.
     Missing source rows are not proof of absence.
```

### Final PR #68 Decision

PR #68 should be the **Source Evidence Spine**.

That name is intentionally boring and literal:

```text
source       -> where the evidence came from
evidence     -> exact thing Cubicle can cite
spine        -> shared foundation that later search/docs/graph APIs attach to
```

Do not call this a search layer, block layer, ingestion platform, or graph query layer.

```text
Current risk                     PR #68 Source Evidence Spine
 |                               |
 +-- source fields on objects     +-- ExternalIdentity
 |   -> identity confusion        |   -> source rename/merge/copy history
 |
 +-- object freshness only        +-- SourceRun + SourceObservation
 |   -> cannot model partiality   |   -> crawl coverage and item state separated
 |
 +-- whole object/document cites   +-- EvidenceAnchor
 |   -> not inspectable           |   -> exact source span/message/comment citation
 |
 +-- SearchChunk first            +-- anchor text may be queryable
 |   -> index becomes truth       |   -> retrieval is projection over anchors later
 |
 +-- DocumentBlock first          +-- anchor kinds are source-neutral
     -> docs island                   -> Slack/Jira/GitHub/Docs fit one citation contract
```

### Minimal Ent Schemas For PR #68

```text
SourceRun
 |
 +-- source_key                 // identity: source family, e.g. slack/jira/github/google_docs
 +-- source_instance            // identity: workspace/repo/site/account namespace
 +-- scope_kind                 // identity: channel/project/repo/document/folder/etc.
 +-- scope_key                  // identity: source-local scope id
 +-- status                     // freshness: running/complete/partial/failed/rate_limited
 +-- started_at                 // freshness: run start
 +-- completed_at               // freshness: terminal completion, nullable
 +-- coverage_start_at          // freshness: lower time bound covered, nullable
 +-- coverage_end_at            // freshness: upper time bound covered, nullable
 +-- checkpoint_token           // freshness: opaque source cursor/checkpoint, nullable
 +-- error_code                 // freshness: bounded source/API failure class, nullable
 +-- error_message              // freshness: bounded debug message, nullable

ExternalIdentity
 |
 +-- target_kind                // identity: ticket/pull_request/document/message/person/etc.
 +-- target_id                  // identity: Cubicle-owned Ent object id
 +-- source_key                 // identity: source family
 +-- source_instance            // identity: source namespace
 +-- external_id                // identity: source-owned id/key/url tuple
 +-- identity_status            // identity: active/alias/retired/merged/deleted
 +-- first_seen_at              // freshness: first observed mapping
 +-- last_seen_at               // freshness: last observed mapping
 +-- replaced_by_identity_id    // identity: optional pointer for rename/merge/redirect

SourceObservation
 |
 +-- source_run_id              // freshness: run that produced this observation
 +-- external_identity_id       // identity: source identity observed, nullable until resolved
 +-- observed_kind              // identity: jira_issue/slack_message/google_doc/github_pr/etc.
 +-- observed_at                // freshness: Cubicle observation time
 +-- source_updated_at          // freshness: source-reported update time, nullable
 +-- is_deleted                 // freshness: source tombstone signal
 +-- deleted_at                 // freshness: source deletion time, nullable
 +-- permission_policy_key      // permission: source ACL policy namespace, nullable
 +-- visibility_hash            // permission: stable ACL/visibility fingerprint
 +-- source_url                 // citation: deep link/source locator, nullable
 +-- content_hash               // citation/freshness: normalized item hash

EvidenceAnchor
 |
 +-- source_observation_id      // citation: observed item containing this anchor
 +-- anchor_kind                // citation: doc_span/slack_message/jira_comment/pr_review_comment/etc.
 +-- anchor_locator             // citation: source-local paragraph/comment/message locator
 +-- source_span                // citation: source-native span/range/path, nullable
 +-- ordinal                    // citation: stable order inside observed item, nullable
 +-- text_hash                  // citation: exact cited text hash
 +-- anchor_text                // citation/search: bounded normalized text for Day 0 lexical lookup
```

Existing schema hook:

```text
Evidence
 |
 +-- evidence_anchor_id         // citation: optional edge to EvidenceAnchor
```

`anchor_text` is allowed in PR #68 because it makes the anchor minimally queryable without creating a separate retrieval truth store. It must stay bounded and source-derived. No embeddings, no ranking model, no generated summary.

### Required Indexes

```text
SourceRun(source_key, source_instance, scope_kind, scope_key, started_at)
ExternalIdentity(source_key, source_instance, external_id)
ExternalIdentity(target_kind, target_id, identity_status)
SourceObservation(external_identity_id, observed_at)
SourceObservation(source_run_id, observed_kind)
EvidenceAnchor(source_observation_id, anchor_locator)
EvidenceAnchor(target via Evidence edge later)
```

If PR #68 implements SQLite FTS over `EvidenceAnchor.anchor_text`, it is acceptable only as an implementation detail for `contextSearch` over anchors. It must not introduce `SearchDocument`, `SearchChunk`, embeddings, or a second citation identity.

### Four Failure Proof

```text
partial Slack crawl
 |
 +-- SourceRun(slack, channel C123, status=partial, checkpoint_token=...)
 +-- SourceObservation rows only for messages actually seen
 `-- missing messages are unknown, not deleted or absent

renamed/merged/deleted Jira issue
 |
 +-- ExternalIdentity(jira:CUB-123, target=ticket:42, status=retired)
 +-- ExternalIdentity(jira:PLAT-77, target=ticket:42, status=active)
 +-- SourceObservation marks source deletion/redirect if seen
 `-- old links still resolve to canonical Ticket 42

unlinked Google Doc paragraph
 |
 +-- SourceObservation(google_doc doc_abc, content_hash=...)
 +-- EvidenceAnchor(anchor_kind=doc_span, anchor_locator=paragraph/heading, anchor_text=...)
 `-- later search/block projections index the same anchor instead of replacing it

fast whyBlocked(workstream)
 |
 +-- existing typed Ent graph gives bounded object path
 +-- Evidence -> EvidenceAnchor gives exact proof
 +-- SourceObservation gives item freshness/visibility
 `-- SourceRun gives partial-crawl warning
```

### PR #69 Immediately After

PR #69 should wire the smallest read path over the spine:

```text
GraphQL reads
 |
 +-- resolveExternalIdentity(source_key, source_instance, external_id)
 +-- sourceCoverage(source_key, source_instance, scope_kind, scope_key)
 +-- evidenceAnchors(object_kind, object_id, first)
 +-- contextSearch(query, first, source_kinds, freshness_policy)
       -> lexical over EvidenceAnchor.anchor_text only
       -> returns anchor + source observation + source run warning + optional object ref
```

Seed fixtures should prove exactly:

```text
1. partial Slack crawl
2. Jira rename/merge/delete
3. unlinked Google Doc paragraph
4. launch-blocker answer with evidence and source health warning
```

### Block In Review

Block any PR #68 that adds:

```text
SearchDocument / SearchChunk
DocumentSection / DocumentBlock
generic nodes / edges
Cypher-like query language
vector DB / embeddings / RAG answers
Foundry-style scheduler/health/action platform
raw source payload blobs as the foundation
source_key/source_instance/external_id directly on ontology objects as identity
tombstone inference from partial source runs
```

This is the current moderator ruling unless a later round produces a concrete failure that the Source Evidence Spine cannot handle.

## Moderator Challenge - Round E

Attack the ruling as if you are reviewing PR #68 before merge.

Do not relitigate search-first versus block-first unless you can show a concrete bug in the Source Evidence Spine.

Required focus:

```text
1. Ent implementation risk
   Is `target_kind` + `target_id` acceptable for PR #68, or must Ent use typed optional edges?
   Show the exact migration/query/codegen pain if your answer is strict.

2. Identity uniqueness
   What unique indexes are required so Jira rename, GitHub repo move, Slack mirrored message,
   and Google Doc copy do not duplicate or corrupt canonical objects?

3. Anchor text risk
   Does `EvidenceAnchor.anchor_text` leak too much, bloat SQLite, or recreate SearchChunk?
   If yes, propose a smaller field set that still supports Day 0 lexical lookup.

4. Permission/freshness correctness
   What exact query is illegal unless it checks `visibility_hash`, `permission_policy_key`,
   `SourceRun.status`, and `SourceObservation.is_deleted`?

5. PR boundary
   What must be cut from PR #68 to keep it reviewable?
```

Round E must end with:

```text
Approve PR #68 only if:
...

Block PR #68 if:
...

One migration/index I insist on:
...
```

## Moderator Ruling - After Round E

Round E caught two real problems in the prior ruling:

```text
Problem 1
 |
 +-- `anchor_text` is too broad
 `-- it can become SearchChunk under a different name

Problem 2
 |
 +-- ExternalIdentity uniqueness was underspecified
 `-- source_key/source_instance/external_id is not enough without external_kind
```

The Source Evidence Spine still stands, but PR #68 is now narrower.

### Final PR #68 Scope

PR #68 is schema, migration, validation, and tests only.

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

### Revised Minimal Schema

```text
SourceRun
 |
 +-- run_key                    // identity: connector idempotency key
 +-- source_key                 // identity: slack/jira/github/google_docs
 +-- source_instance            // identity: workspace/repo/site/account namespace
 +-- scope_kind                 // identity: channel/project/repo/document/folder/etc.
 +-- scope_key                  // identity: source-local scope id
 +-- status                     // freshness: running/complete/partial/failed/rate_limited
 +-- started_at                 // freshness: run start
 +-- completed_at               // freshness: terminal completion, nullable
 +-- coverage_start_at          // freshness: lower covered time bound, nullable
 +-- coverage_end_at            // freshness: upper covered time bound, nullable
 +-- checkpoint_token           // freshness: opaque source cursor/checkpoint, nullable
 +-- error_code                 // freshness: bounded failure class, nullable
 +-- error_message              // freshness: bounded debug message, nullable

ExternalIdentity
 |
 +-- target_kind                // identity: Cubicle object kind
 +-- target_id                  // identity: Cubicle object id
 +-- source_key                 // identity: source family
 +-- source_instance            // identity: source namespace
 +-- external_kind              // identity: source item kind, e.g. jira_issue/slack_message/google_doc/github_pr
 +-- external_id                // identity: source-owned id/key/url tuple
 +-- identity_status            // identity: active/alias/retired/merged/deleted
 +-- first_seen_at              // freshness: first observed mapping
 +-- last_seen_at               // freshness: last observed mapping
 +-- replaced_by_identity_id    // identity: optional self-reference for rename/merge/redirect

SourceObservation
 |
 +-- source_run_id              // freshness: run that produced this observation
 +-- external_identity_id       // identity: required source identity observed
 +-- observed_kind              // identity: source item kind
 +-- observed_at                // freshness: Cubicle observation time
 +-- source_updated_at          // freshness: source-reported update time, nullable
 +-- is_deleted                 // freshness: source tombstone signal
 +-- deleted_at                 // freshness: source deletion time, nullable
 +-- permission_policy_key      // permission: source ACL policy namespace
 +-- visibility_hash            // permission: stable ACL/visibility fingerprint
 +-- source_url                 // citation: source deep link, nullable
 +-- content_hash               // citation/freshness: normalized source item hash

EvidenceAnchor
 |
 +-- source_observation_id      // citation: observed source item containing the anchor
 +-- anchor_kind                // citation: doc_span/slack_message/jira_comment/pr_review_comment/etc.
 +-- anchor_locator             // citation: source-local paragraph/comment/message locator
 +-- source_span_key            // citation: normalized source span identity for dedupe
 +-- ordinal                    // citation: stable order inside observed item, nullable
 +-- text_hash                  // citation: exact cited text hash
 +-- text_preview               // citation: bounded display snippet, max 512 chars
 +-- text_preview_truncated     // citation: whether preview is incomplete
 +-- lexical_fingerprint        // citation/search: optional bounded normalized token fingerprint
```

Important correction:

```text
No `anchor_text`.
 |
 +-- `text_preview` is for inspection.
 +-- `lexical_fingerprint` is for primitive Day 0 debug lookup.
 `-- real search remains PR #69+ projection over anchors.
```

### Required Indexes And Constraints

```text
SourceRun
 |
 +-- UNIQUE(source_key, source_instance, run_key)
 +-- INDEX(source_key, source_instance, scope_kind, scope_key, started_at)

ExternalIdentity
 |
 +-- UNIQUE(source_key, source_instance, external_kind, external_id)
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
 +-- UNIQUE(source_observation_id, anchor_kind, source_span_key, text_hash)
 +-- INDEX(source_observation_id, ordinal)
```

`target_kind + target_id` is accepted only as a constrained provenance pointer.

```text
Allowed
 |
 +-- ExternalIdentity resolves source identities to Cubicle objects
 +-- repository/helper validates target_kind against known types
 +-- tests reject unknown target_kind

Forbidden
 |
 +-- generic GraphQL traversal over target_kind/target_id
 +-- treating target_kind/target_id as a replacement for typed Ent edges
 +-- adding nullable typed edges to every ontology object in PR #68
```

### Legal Query Rule

Any query that returns evidence text or preview must join through freshness and permission rows.

Illegal:

```sql
SELECT ea.text_preview
FROM evidence_anchors ea
WHERE ea.lexical_fingerprint LIKE '%launch%';
```

Legal shape:

```sql
SELECT ea.text_preview, so.source_url, sr.status
FROM evidence_anchors ea
JOIN source_observations so ON so.id = ea.source_observation_id
JOIN source_runs sr ON sr.id = so.source_run_id
WHERE ea.lexical_fingerprint LIKE :query_token
  AND so.is_deleted = false
  AND so.permission_policy_key IN (:viewer_policy_keys)
  AND so.visibility_hash IN (:viewer_visibility_hashes)
  AND sr.status IN ('complete', 'partial')
LIMIT :first;
```

If `sr.status = partial`, returned evidence must carry a coverage warning. Missing rows from that scope cannot support absence claims.

### Approve PR #68 Only If

```text
1. It adds exactly the Source Evidence Spine schemas.
2. It uses ordinary Ent tables/indexes only.
3. It includes external_kind in ExternalIdentity uniqueness.
4. It uses bounded text_preview plus lexical_fingerprint, not unbounded anchor_text.
5. It validates target_kind/target_id as provenance pointers.
6. It proves duplicate identity insertion fails.
7. It proves partial SourceRun cannot tombstone missing data.
8. It proves deleted observations are excluded from current-evidence helpers.
9. It proves evidence helpers require permission/freshness checks.
```

### Block PR #68 If

```text
SearchChunk appears.
DocumentBlock appears.
FTS5 appears.
GraphQL search or answer APIs appear.
raw source payload storage appears.
embeddings/RAG appears.
source identity fields are pasted onto canonical ontology objects.
target_kind/target_id becomes a generic traversal API.
queries return preview text without visibility/deletion/source-run checks.
```

This is the implementation-ready moderator ruling.
