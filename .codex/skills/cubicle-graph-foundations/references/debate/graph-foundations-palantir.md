## Round A - Palantir Reply

Read before replying:
- graph-foundations-live-chat.md
- graph-foundations-meta.md
- graph-foundations-palantir.md
- graph-foundations-glean.md
- graph-foundations-obsidian.md

Meta claim quoted: "Cubicle's foundation should be **typed work objects + typed associations + bounded association-list reads**, with a narrow source metadata contract on source-backed rows."

Attack: this is a serving foundation, not a trust foundation. It is necessary and insufficient. TAO's public design exists to serve a changing social graph through objects and associations backed by MySQL/cache; it is not a data integration, provenance, health, or operational decision framework. Copying TAO as Cubicle's center would make Cubicle fast at returning graph neighborhoods and weak at proving whether those neighborhoods are current, complete, permission-safe, or source-backed. Evidence: Meta TAO paper and engineering writeup describe object/association graph serving, while Foundry docs explicitly put connected source data, datasets/transactions, health checks, ontology objects/links, actions, and functions into one operational stack.

Evidence:
- Palantir says connecting to data is the first step to value from an organization's data, and supports enterprise sources such as stores, file systems, databases, and warehouses: https://palantir.com/docs/foundry/data-integration/connecting-to-data/
- Foundry datasets have transaction types such as `SNAPSHOT`, `APPEND`, `UPDATE`, and `DELETE`, which is exactly the difference between "current row" and "how did this state arrive": https://palantir.com/docs/foundry/data-integration/datasets/
- Foundry health checks exist because data synced and transformed on schedules must stay reliable over time: https://palantir.com/docs/foundry/data-integration/health-checks/
- Palantir's recommended health check guidance frames the operational goal as ensuring data gets in, data gets built, and data gets out: https://palantir.com/docs/foundry/maintaining-pipelines/recommended-health-checks/
- Foundry object types are schema definitions for real-world entities/events, and link types define relationships between object types: https://palantir.com/docs/foundry/object-link-types/object-types-overview/ and https://palantir.com/docs/foundry/object-link-types/link-types-overview/
- Foundry action types define edits to objects, properties, and links; functions execute operational logic over ontology data: https://palantir.com/docs/foundry/action-types/overview/ and https://palantir.com/docs/foundry/functions/overview/
- TAO evidence for Meta's own claim is limited to graph serving: https://www.usenix.org/system/files/conference/atc13/atc13-bronson.pdf and https://engineering.fb.com/2013/06/25/core-infra/tao-the-power-of-the-graph/

Meta claim quoted: "If PR #68 starts with `SourceConnection`, `IngestionRun`, `ProjectionRun`, and `HealthCheck`, we get pipeline observability before a canonical serving graph can answer 'what is blocking launch?' That is infrastructure cosplay."

Attack: this is a false binary. The Palantir foundation does not require a full Foundry clone or a workflow engine. It requires a minimal operational substrate that prevents graph rows from pretending to be truth when the crawl was partial, stale, deleted, or permission-shifted. Calling this "infrastructure cosplay" is backwards. The cosplay is a typed graph that looks authoritative while hiding whether Slack failed halfway or Jira deleted the issue.

Concrete failure:

```text
Question: "why is launch blocked?"
 |
 +-- Meta graph returns Workstream -> Ticket -> SlackThread
 |
 +-- but Slack crawl was partial
 +-- Jira key was renamed
 +-- Google Doc ACL changed
 |
 `-- bounded association list is fast and still operationally unsafe
```

Palantir foundation:

```text
Source system
 |
 +-- SourceConnection
 +-- SyncRun / SyncCheckpoint
 +-- SourceRecord or lifecycle observation
 |
 +-- Projection
 |
 +-- Ontology object + link
 |
 +-- Health / freshness / visibility status
```

This is not "build a platform first." This is the minimum distinction between:

```text
what the source said
what Cubicle believes
whether that belief is safe to serve
```

Meta claim quoted: "The first trust step should be narrower: source metadata on the typed graph rows that will later be populated by ingestion."

Attack: narrower is correct only if it does not confuse source identity with ontology identity. Meta's proposed `source_key`, `source_instance`, and `external_id` on graph rows is too eager as a universal row property. Jira rename/merge, GitHub transfer, Slack export/mirror, and Google Doc copy all prove source identity is a relationship between a source record and a canonical object, not always an intrinsic property of the object. Foundry's distinction between data integration resources, datasets/transactions, ontology object types, and links exists because raw source shape and operational object shape are not the same thing.

Correct foundation compromise:

```text
PR #68 = Ontology Lifecycle Metadata
 |
 +-- source_updated_at
 +-- ingested_at
 +-- last_seen_at
 +-- is_deleted
 +-- deleted_at
 +-- permission_policy_key
 +-- visibility_hash
```

Do not put `source_key` / `source_instance` / `external_id` everywhere in PR #68 as if one source row equals one ontology object. That belongs in the next identity foundation:

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

Cubicle schema/API mapping:

```text
Foundation layer
 |
 +-- Ent typed ontology remains
 |     -> Person, Workstream, Ticket, PullRequest, Document, Message, Evidence
 |
 +-- lifecycle metadata lands on trusted rows
 |     -> freshness, deletion, ingestion, permission visibility
 |
 +-- next identity/provenance PRs attach source runs and external IDs
 |     -> SourceConnection, SyncRun, ExternalIdentity, SourceRecord, ProjectionRun
 |
 +-- GraphQL can expose trust without building search first
       -> objectProvenance later
       -> sourceHealth later
       -> whyBlocked returns freshness warnings now
```

What breaks if Cubicle copies Palantir too literally: a local POC does not need Foundry-scale dataset branching, marketplace resources, full health-check scheduling, dynamic security policy engines, or action workflow writeback. Copy the operating principles, not the enterprise surface area.

What breaks if Cubicle copies Meta too literally: Cubicle becomes a fast adjacency-list service that cannot prove source completeness, source lineage, permission drift, deletion, or identity history. That is unacceptable for an engineering-work graph because the dangerous failures are not slow queries; they are believable stale answers.

Palantir foundation demand:

```text
1. Keep Meta's typed Ent graph.
2. Make PR #68 ontology lifecycle metadata, not source identity pasted onto rows.
3. Follow with ExternalIdentity + minimal SyncRun/SourceRecord/ProjectionRun.
4. Build Glean retrieval and Obsidian blocks only after lifecycle/visibility semantics exist.
```

Do not build now:

- No generic `nodes` / `edges`.
- No Cypher.
- No vector-first RAG.
- No document-block foundation before graph-wide lifecycle semantics.
- No full Foundry clone.

## Round B - Palantir Counterattack

Read before replying:
- graph-foundations-live-chat.md
- graph-foundations-meta.md
- graph-foundations-palantir.md
- graph-foundations-glean.md
- graph-foundations-obsidian.md

Meta claim quoted: "Cubicle's foundation should be **typed work objects + typed associations + bounded association-list reads**, with a narrow source metadata contract on source-backed rows."

Accepted: typed work objects, typed associations, and bounded reads are the correct serving substrate. Meta is right that Cubicle should not build generic `nodes` / `edges`, Cypher, or vector-first graph truth. TAO's actual public evidence supports exactly that limited conclusion: TAO objects are typed nodes, associations are typed directed edges, and Meta's engineering writeup says TAO persists objects/associations in MySQL while serving reads through cache/shards. Sources: https://www.usenix.org/system/files/conference/atc13/atc13-bronson.pdf and https://engineering.fb.com/2013/06/25/core-infra/tao-the-power-of-the-graph/

Attack: Meta is still trying to make the serving substrate the foundation of truth. That is too shallow for Cubicle. TAO answers "give me this object's association list fast." It does not answer "was the Slack channel fully crawled?", "did Jira rename this issue?", "did this Google Doc permission change?", or "is this blocker answer based on a stale projection?" Foundry's real-world architecture separates data integration, dataset transactions, health checks, ontology, and actions because operational products need more than fast adjacency reads. Foundry datasets explicitly model transactions as atomic changes and support transaction types like `SNAPSHOT`, `APPEND`, `UPDATE`, and `DELETE`; health checks exist to validate data quality over scheduled pipelines. Sources: https://palantir.com/docs/foundry/data-integration/datasets/ and https://palantir.com/docs/foundry/data-integration/health-checks/

Meta claim quoted: "PR #68 = Graph Source Metadata Contract" with `source_key`, `source_instance`, and `external_id` on source-backed typed objects.

Attack: this is the weak part of Meta's foundation. It takes a good trust instinct and contaminates canonical ontology rows with source identity. Jira issue keys change. GitHub repos move. Slack messages can be exported, mirrored, or rethreaded. Google Docs can be copied. A Cubicle object must survive those changes. Source identity is not the object; it is a relationship between a source system and a canonical object. Palantir's distinction between connected data, datasets/transactions, object types, and link types exists because raw source shape is not the operational ontology shape. Foundry object types model real-world entities/events and link types model relationships between object types, not arbitrary source rows pretending to be object identity. Sources: https://palantir.com/docs/foundry/object-link-types/object-types-overview/ and https://palantir.com/docs/foundry/object-link-types/link-types-overview/

Palantir correction:

```text
PR #68
 |
 +-- ontology lifecycle/trust metadata only
 |     -> source_updated_at
 |     -> ingested_at
 |     -> last_seen_at
 |     -> is_deleted
 |     -> deleted_at
 |     -> permission_policy_key
 |     -> visibility_hash
 |
 `-- no row-level claim that one source key equals one object

PR #69 / #70
 |
 +-- SourceConnection / SyncRun
 +-- ExternalIdentity(source_key, source_instance, external_id, object_kind, object_id)
```

Glean claim quoted: "No. That is a serving foundation, not a knowledge foundation."

Accepted: correct. Meta's typed graph alone cannot discover unlinked evidence. Glean's real-world evidence is strong: its connectors are built around source APIs/data models, index content, mirror source permissions, and keep data current in an isolated tenant; its knowledge graph docs say the crawler indexes titles, body copy, comments, media, metadata, permissions, and facets. Sources: https://docs.glean.com/connectors/about and https://docs.glean.com/security/knowledge-graph

Attack: Glean then overreaches by making retrieval the first foundation. It claims "source-aware searchable evidence with visibility/freshness attached" should come first, but that produces a second truth substrate before Cubicle has defined operational trust semantics. A `SearchChunk` row with `freshness_state` and `visibility_hash` is not inherently safer than an ontology row with those fields; it is just more text-shaped. Glean's documented product is an enterprise search and assistant stack. That is a retrieval system over connected content and permissions, not a canonical engineering-work ontology. Its connector docs prove search needs permissions and freshness, not that chunks should precede the ontology trust contract.

Glean claim quoted: "The canonical state can still be Ent... But canonical state without searchable evidence is a thin ontology."

Accepted and constrained: yes, canonical state without retrieval is thin. But retrieval before lifecycle metadata is blind confidence. The first Cubicle foundation should make current ontology rows state whether they are fresh, deleted, visible, and last seen. Then Glean-style `SearchDocument` / `SearchChunk` can attach to those same semantics instead of inventing parallel freshness. Glean's own product handles permissions as part of indexing; Cubicle should copy that requirement later, not make search rows the first source of truth. Source: https://developers.glean.com/api-info/indexing/documents/permissions

Failure if Glean wins PR #68:

```text
searchChunks("why is launch blocked")
 |
 +-- finds exact stale paragraph
 +-- finds Slack snippet from partial crawl
 +-- finds old Jira key after rename
 |
 `-- answer is discoverable but not operationally trustworthy
```

Obsidian claim quoted: "Cubicle's foundation problem is not storage, ingestion, or search first. It is addressability: the graph needs stable, exact anchors for the smallest meaningful unit of work knowledge."

Accepted: exact anchors matter. Obsidian is right that whole-document evidence is too coarse. Official Obsidian docs support links to files, headings, and blocks; a block can be a paragraph, quote, or list item with a unique block identifier. Graph view represents notes as nodes and internal links as edges. Sources: https://obsidian.md/help/links and https://obsidian.md/help/plugins/graph

Attack: Obsidian turns a crucial content granularity into the whole foundation. That is wrong for Cubicle. Obsidian's own graph view is note/link oriented; it is not a source-governed operational ontology with crawl status, permission drift, deletion semantics, identity resolution, or projection history. Blocks are evidence anchors, not truth boundaries. A precise block from a stale document is precisely wrong. A precise block from a source the user cannot access is a security defect. A precise block from a partial crawl is an incomplete story.

Obsidian claim quoted: "The common source contract is not useful if it attaches only to coarse objects."

Accepted and redirected: source/trust metadata should eventually exist at the smallest source-backed unit, including `DocumentBlock`, Slack message/reply, Jira comment, and PR review comment. But that is not a reason to build document blocks first. It is a reason to define lifecycle semantics first so every later fine-grained unit inherits the same trust contract. Obsidian's own local-file model is useful for navigation; Cubicle's backend foundation must be broader than documents because the product is engineering execution across Jira, GitHub, Slack, Docs, people, and workstreams. Obsidian data storage docs describe local files and plugin storage, not enterprise source provenance. Source: https://obsidian.md/help/data-storage

Failure if Obsidian wins PR #68:

```text
DocumentSection -> DocumentBlock
 |
 +-- exact paragraph exists
 +-- but ticket identity is still source-key fragile
 +-- Slack crawl completeness is invisible
 +-- GitHub/Jira links have no shared lifecycle contract
 |
 `-- Cubicle becomes precise inside docs and weak across work
```

Palantir foundation decision:

```text
PR #68 = Ontology Lifecycle Metadata
 |
 +-- source_updated_at
 +-- ingested_at
 +-- last_seen_at
 +-- is_deleted
 +-- deleted_at
 +-- permission_policy_key
 +-- visibility_hash
```

Apply it to canonical source-backed ontology objects and metadata-bearing association/result rows. Do not include `source_key`, `source_instance`, or `external_id` in this PR as universal row identity. Follow with `ExternalIdentity`, `SourceConnection`, `SyncRun`, `SourceRecord`, and `ProjectionRun`.

Foundation order:

```text
1. Ent typed graph remains the serving/storage substrate.
2. Lifecycle/trust metadata becomes the first cross-graph contract.
3. External identity and source-run provenance explain where truth came from.
4. Glean-style retrieval indexes source items using the same trust contract.
5. Obsidian-style blocks become exact evidence anchors using the same trust contract.
```

What not to build now:

- No source identity pasted onto every ontology row.
- No `SearchChunk` foundation before lifecycle semantics.
- No `DocumentBlock` foundation before graph-wide trust semantics.
- No generic graph table.
- No Cypher.
- No vector-first RAG.
- No full Foundry clone or action engine.

## Round C - Palantir Foundation Invariants

Read before replying:
- graph-foundations-live-chat.md
- graph-foundations-meta.md
- graph-foundations-palantir.md
- graph-foundations-glean.md
- graph-foundations-obsidian.md

### 1. Canonical identity

The stable identity of a Cubicle work object is a Cubicle-owned ontology object ID, not a Jira key, GitHub repo path, Slack timestamp/channel tuple, Google Doc ID, search chunk ID, or block ID.

Canonical identity:

```text
CubicleObjectID
 |
 +-- object_kind
 +-- object_id
 |
 +-- ExternalIdentity*
      |
      +-- source_key
      +-- source_instance
      +-- external_id
      +-- alias / active / retired / merged
```

This is the only model that survives all four identity failures:

```text
Jira key rename        -> new ExternalIdentity, same Ticket
Jira merge/delete      -> Ticket relation/tombstone, not ID replacement
GitHub repo move       -> new repo namespace identity, same PullRequest if resolved
Google Doc copy        -> new ExternalIdentity, maybe new Document unless resolved
Slack mirror/export    -> multiple source identities, same Message only if resolved
```

Meta is right that typed objects/associations are the serving substrate. TAO's public evidence supports typed objects and associations backed by MySQL/cache, not mutable source keys as identity. Sources: https://www.usenix.org/system/files/conference/atc13/atc13-bronson.pdf and https://engineering.fb.com/2013/06/25/core-infra/tao-the-power-of-the-graph/

Palantir evidence is stronger for the identity invariant: Foundry object types model real-world entities/events, while data integration/datasets/transactions model landed data and change history. Those are separate layers. Sources: https://palantir.com/docs/foundry/object-link-types/object-types-overview/ and https://palantir.com/docs/foundry/data-integration/datasets/

### 2. Canonical evidence unit

The smallest durable evidence unit is not the ontology object, not the whole document, not a search chunk, and not a free-standing document block. It is an **EvidenceAnchor**: a stable citation to a source-backed observation plus an address inside that observation.

Canonical evidence:

```text
EvidenceAnchor
 |
 +-- source_record_id or source_observation_id
 +-- source_span / source_url / source_locator
 +-- content_hash
 +-- observed_at
 +-- permission_policy_key
 +-- visibility_hash
 |
 +-- may resolve to:
      +-- DocumentFragment / future DocumentBlock
      +-- Slack message or reply
      +-- Jira comment or description span
      +-- PR review comment
      +-- association row evidence
```

Glean is right that unlinked evidence must be retrievable. Its connector docs show content, metadata, permissions, and activity are ingested/indexed from source systems. Sources: https://docs.glean.com/connectors/about and https://docs.glean.com/security/knowledge-graph/

Obsidian is right that exact addressability matters. Official Obsidian docs support links to headings and blocks, where a block can be a paragraph/list/quote with a stable block identifier. Source: https://obsidian.md/help/links

But both are wrong if they make their rendering unit canonical. A `SearchChunk` is optimized for retrieval. A `DocumentBlock` is optimized for document navigation. Cubicle's canonical evidence must be source-neutral enough to cite a Slack reply, Jira comment, PR review, or document paragraph without inventing a new truth model per source.

### 3. Freshness authority

Freshness authority is layered. No single row can honestly answer every freshness question.

```text
Source scope freshness
 |
 +-- SyncRun / SyncCheckpoint
 |     -> crawl complete, partial, failed, rate-limited
 |
 +-- SourceRecord / SourceObservation
 |     -> what source returned, source_updated_at, observed_at
 |
 +-- Ontology object / association row
 |     -> derived belief freshness, last_seen_at, deleted_at
 |
 +-- SearchChunk / DocumentBlock
       -> local index/content freshness, never source authority by itself
```

Rules:

```text
partial Slack crawl
 |
 +-- SyncRun is authority for scope completeness
 +-- Message rows must not tombstone missing messages from an incomplete scope

renamed/deleted Jira issue
 |
 +-- SourceRecord/ExternalIdentity records source-level change
 +-- Ticket row records Cubicle belief state

unlinked Google Doc paragraph
 |
 +-- Document/fragment/block can say content freshness
 +-- EvidenceAnchor records what exact source span was cited

"why is launch blocked?"
 |
 +-- GraphQL response must include object freshness + source health warning
```

Palantir's health check docs validate this layering: Foundry health checks cover job/build/freshness/sync status, while dataset transactions model atomic changes to dataset contents. Sources: https://palantir.com/docs/foundry/data-integration/health-checks/ and https://palantir.com/docs/foundry/health-checks/checks-reference/

Glean's connector docs also support the layering: connectors keep indexed data current and mirror permissions from the source, but that proves retrieval rows need freshness; it does not make retrieval rows the authority for source crawl completeness. Source: https://docs.glean.com/connectors/about

### 4. Retrieval/addressability ordering

Cubicle should define shared source/freshness metadata first, then source-neutral evidence anchors, then addressable blocks/search chunks as concrete projections.

Ordering:

```text
1. shared lifecycle/trust metadata
   |
   +-- every source-backed graph row speaks the same freshness/deletion/visibility language

2. external identity + source-run provenance
   |
   +-- source keys stop pretending to be ontology identity
   +-- partial crawls become visible

3. source-neutral EvidenceAnchor
   |
   +-- exact citation can point to doc block, Slack reply, Jira comment, PR review

4. retrieval and addressability projections
   |
   +-- SearchDocument / SearchChunk for Glean-style discovery
   +-- DocumentSection / DocumentBlock for Obsidian-style navigation
```

Why not blocks first: Obsidian proves exact block links are useful, but its graph view is note/link navigation, not source health, identity resolution, or permission governance. Sources: https://obsidian.md/help/plugins/graph and https://obsidian.md/help/links

Why not search chunks first: Glean proves source-aware indexing is essential, but its docs describe an enterprise search/retrieval layer fed by connectors; this is evidence discovery, not canonical ontology identity or source-run authority. Sources: https://docs.glean.com/connectors/connectors-power-glean and https://docs.glean.com/security/knowledge-graph/

Why not source identity on every row first: Meta's SourceObservationMixin gets the trust problem but still risks confusing source identity with canonical identity. Palantir separates landed/source data and ontology object semantics; Cubicle should too. Sources: https://palantir.com/docs/foundry/data-integration/datasets/ and https://palantir.com/docs/foundry/object-link-types/object-types-overview/

Non-negotiable foundation invariant:

```text
Cubicle must never let source identity, retrieval chunks, or document blocks become canonical object identity.

canonical object identity      -> Cubicle-owned typed ontology object
canonical source identity      -> ExternalIdentity
canonical evidence citation    -> EvidenceAnchor over source observation + address/span
canonical freshness authority  -> layered: SyncRun -> SourceRecord -> ontology row -> projection row
```

Concession to strongest opposing camp:

```text
Glean has the strongest product-value objection:
 |
 `-- without retrieval, Cubicle cannot find unlinked evidence.

Accepted:
 |
 `-- SearchDocument/SearchChunk must come soon and must carry permission/freshness fields.

Rejected:
 |
 `-- SearchChunk cannot be the first canonical foundation because retrieval is not identity,
     not source-run completeness, and not ontology truth.
```

Concrete PR #68 after this invariant:

```text
PR #68 = Shared lifecycle/trust metadata on source-backed Ent ontology rows
 |
 +-- source_updated_at
 +-- ingested_at
 +-- last_seen_at
 +-- is_deleted
 +-- deleted_at
 +-- permission_policy_key
 +-- visibility_hash
 |
 +-- apply to:
      +-- Ticket
      +-- PullRequest
      +-- Document
      +-- DocumentFragment
      +-- Message
      +-- Evidence
      +-- metadata-bearing LensResult rows
 |
 +-- do not include:
      +-- source_key / source_instance / external_id as universal object fields
      +-- SearchDocument / SearchChunk
      +-- DocumentSection / DocumentBlock
      +-- SourceConnection / SyncRun / SourceRecord
```

PR #69 should add `ExternalIdentity` plus minimal source-run provenance. PR #70 should choose the first concrete evidence projection: Glean-style search chunks or Obsidian-style document blocks, both attached to the same lifecycle/trust contract.

## Round D - Palantir Design Review

Read before replying:
- graph-foundations-live-chat.md
- graph-foundations-meta.md
- graph-foundations-palantir.md
- graph-foundations-glean.md
- graph-foundations-obsidian.md

### 1. Accept/reject the moderator candidate

| Moderator candidate | Palantir verdict | Concrete failure if shipped literally |
|---|---|---|
| `ExternalIdentity` | Accept | This fixes Meta's bad `source_key`-on-object shortcut. Jira key rename and GitHub repo move need identity history, not mutation of the canonical `Ticket` / `PullRequest` row. Foundry separates landed data from ontology object types, which is the same distinction Cubicle needs: https://palantir.com/docs/foundry/data-integration/datasets/ and https://palantir.com/docs/foundry/object-link-types/object-types-overview/ |
| `SourceObservation` | Accept | This is the right place for observed source state. Foundry dataset transactions exist because source state changes over time; a final ontology row is not enough provenance: https://palantir.com/docs/foundry/data-integration/datasets/ |
| `EvidenceAnchor` | Accept | This beats Glean's `SearchChunk` and Obsidian's `DocumentBlock` as canonical evidence because it is source-neutral. It can point to Slack reply, Jira comment, PR review, or Google Doc paragraph. Glean proves content/metadata/permissions must be indexed, but indexing should project from anchors, not become truth: https://docs.glean.com/connectors/about and https://docs.glean.com/security/knowledge-graph |
| Missing `SourceRun` | Reject exact candidate | The exact row/table that fails is absent: no `SourceRun`. Concrete query failure: `sourceHealth(sourceKey: "slack", scope: "channel:C-launch")` cannot answer `partial` vs `complete`, so `whyBlocked(workstream)` cannot warn that Slack evidence is incomplete. Palantir health checks model sync/build/freshness status because pipeline completeness is not derivable from item rows alone: https://palantir.com/docs/foundry/data-integration/health-checks/ and https://palantir.com/docs/foundry/health-checks/checks-reference/ |

Verdict: accept the moderator candidate only with `SourceRun` added to PR #68. Without it, the design fails the partial Slack crawl requirement immediately. Meta's object/association model is still the serving substrate; TAO validates bounded reads, not crawl completeness or source provenance: https://engineering.fb.com/2013/06/25/core-infra/tao-the-power-of-the-graph/

### 2. Minimal PR #68 schema

List only these Ent schemas and fields. No GraphQL API expansion beyond generated Ent. No search. No document blocks.

| Ent schema | Field | Category | Why it exists |
|---|---|---|---|
| `SourceRun` | `source_key` | identity | Source system namespace such as `slack`, `jira`, `github`, `google_docs`. |
| `SourceRun` | `source_instance` | identity | Workspace/repo/site/account namespace. |
| `SourceRun` | `scope_kind` | identity | Crawl scope type: `channel`, `project`, `repo`, `drive`, `doc`. |
| `SourceRun` | `scope_key` | identity | Source-local crawl scope, such as Slack channel ID or Jira project key. |
| `SourceRun` | `status` | freshness | `running`, `complete`, `partial`, `failed`. This is the only PR #68 row allowed to speak for crawl completeness. |
| `SourceRun` | `started_at` | freshness | Run start time. |
| `SourceRun` | `completed_at` | freshness | Nullable run completion time. |
| `SourceRun` | `high_watermark` | freshness | Source cursor/time watermark reached by this run. |
| `SourceRun` | `error_code` | freshness | Source/API failure classification. |
| `SourceRun` | `error_message` | freshness | Human-readable failure detail for logs/debugging. |
| `ExternalIdentity` | `object_kind` | identity | Cubicle ontology type: `ticket`, `pull_request`, `document`, `message`, etc. |
| `ExternalIdentity` | `object_id` | identity | Cubicle-owned Ent object ID. |
| `ExternalIdentity` | `source_key` | identity | Source system namespace. |
| `ExternalIdentity` | `source_instance` | identity | Source account/workspace/repo namespace. |
| `ExternalIdentity` | `external_id` | identity | Source-owned ID/key/URL fragment. |
| `ExternalIdentity` | `status` | identity | `active`, `alias`, `retired`, `merged`, `deleted`. |
| `ExternalIdentity` | `first_seen_at` | freshness | When Cubicle first saw this mapping. |
| `ExternalIdentity` | `last_seen_at` | freshness | When Cubicle last saw this mapping. |
| `ExternalIdentity` | `replaced_by_identity_id` | identity | Points old Jira key or moved repo identity to the replacement identity. |
| `SourceObservation` | `source_run_id` | freshness | Links the observed item to crawl completeness. |
| `SourceObservation` | `source_key` | identity | Source namespace for lookup and debugging. |
| `SourceObservation` | `source_instance` | identity | Source account/workspace/repo namespace. |
| `SourceObservation` | `external_id` | identity | Source-owned item ID/key. |
| `SourceObservation` | `observed_kind` | identity | `jira_issue`, `slack_message`, `google_doc`, `github_pr`, `github_review_comment`, etc. |
| `SourceObservation` | `content_hash` | freshness | Hash of normalized observed content or metadata envelope. |
| `SourceObservation` | `source_updated_at` | freshness | Source-reported update time when available. |
| `SourceObservation` | `observed_at` | freshness | When Cubicle observed this item. |
| `SourceObservation` | `deleted_at` | freshness | Source deletion/tombstone time when known. |
| `SourceObservation` | `permission_policy_key` | permission | Stable policy namespace for later ACL evaluation. |
| `SourceObservation` | `visibility_hash` | permission | Compact representation of source visibility/ACL state. |
| `SourceObservation` | `payload_ref` | citation | Pointer to local raw/normalized payload storage, not a blob in this table. |
| `EvidenceAnchor` | `source_observation_id` | citation | The observed source item this exact evidence comes from. |
| `EvidenceAnchor` | `evidence_id` | citation | Optional link to existing `Evidence` once promoted into graph belief. |
| `EvidenceAnchor` | `object_kind` | identity | Optional Cubicle object this anchor supports. |
| `EvidenceAnchor` | `object_id` | identity | Optional Cubicle object ID this anchor supports. |
| `EvidenceAnchor` | `anchor_kind` | citation | `paragraph`, `heading`, `slack_reply`, `jira_comment`, `pr_review_comment`, `association_evidence`. |
| `EvidenceAnchor` | `source_locator` | citation | Source URL/path/deep link. |
| `EvidenceAnchor` | `source_span` | citation | Paragraph ID, line range, comment ID, message timestamp, or source-native span. |
| `EvidenceAnchor` | `content_hash` | citation | Hash of the cited span, independent from the whole observation hash. |
| `EvidenceAnchor` | `observed_at` | freshness | When the anchor was observed. |
| `EvidenceAnchor` | `permission_policy_key` | permission | Inherited or narrowed permission policy for this anchor. |
| `EvidenceAnchor` | `visibility_hash` | permission | Inherited or narrowed visibility hash for this anchor. |

Indexes:

| Ent schema | Index | Category | Why it exists |
|---|---|---|---|
| `SourceRun` | `(source_key, source_instance, scope_kind, scope_key, started_at)` | serving optimization | Fast latest-run lookup for source health. |
| `ExternalIdentity` | unique `(source_key, source_instance, external_id, object_kind)` | serving optimization | Resolve Jira/GitHub/Slack/Docs source references without scanning ontology tables. |
| `ExternalIdentity` | `(object_kind, object_id, status)` | serving optimization | List all source aliases for a canonical object. |
| `SourceObservation` | `(source_key, source_instance, external_id, observed_at)` | serving optimization | Lookup observation history for one source item. |
| `SourceObservation` | `(source_run_id, observed_kind)` | serving optimization | Explain what a run saw. |
| `EvidenceAnchor` | `(object_kind, object_id, observed_at)` | serving optimization | Fetch inspectable evidence for `whyBlocked`. |
| `EvidenceAnchor` | `(source_observation_id, source_span)` | serving optimization | Dedupe source-local anchors. |

### 3. Four failure-mode proof

| Failure mode | Rows written | Query that works | Why competing first moves fail |
|---|---|---|---|
| Partial Slack crawl | `SourceRun(source_key=slack, scope_kind=channel, scope_key=C-launch, status=partial, high_watermark=...)`; `SourceObservation` rows only for messages actually observed; `EvidenceAnchor` rows only for citeable spans. | `sourceHealth(slack, C-launch)` returns `partial`; `whyBlocked(workstream)` can include "Slack channel crawl partial; evidence may be incomplete." Missing Slack messages are not tombstoned because no complete `SourceRun` exists for that scope. | Metadata-only ontology rows cannot distinguish "message deleted" from "channel page skipped." `SearchChunk` cannot prove crawl coverage. `DocumentBlock` is irrelevant to Slack. |
| Renamed/merged/deleted Jira issue | `ExternalIdentity(jira, CUB-123, status=retired, replaced_by_identity_id=...)`; `ExternalIdentity(jira, PLAT-44, status=active, object_kind=ticket, object_id=same Ticket)`; `SourceObservation(observed_kind=jira_issue, deleted_at or content_hash changed)`. | `resolveExternalIdentity(jira, CUB-123)` resolves to the canonical `Ticket` or replacement. `ticketHistory(ticketID)` can show old/new source identities and deletion/merge observation. | Meta's earlier direct `external_id` on `Ticket` mutates or duplicates the ticket. Glean search may find old key text but cannot decide canonical identity. Obsidian block links do not model Jira identity lifecycle. |
| Unlinked but relevant Google Doc paragraph | `SourceRun(google_docs, scope_kind=doc, status=complete)`; `SourceObservation(observed_kind=google_doc, external_id=docID, content_hash=docVersionHash, visibility_hash=...)`; `EvidenceAnchor(anchor_kind=paragraph, source_locator=doc URL, source_span=heading/paragraph id, content_hash=paragraphHash, object_kind/object_id nullable until promoted)`. | Later `searchChunks` can index the `EvidenceAnchor`, but the anchor is already canonical and source-neutral. Once promoted, `whyBlocked` cites the same anchor. | `SearchChunk` first makes the index the citation. `DocumentBlock` first only handles docs and forces Slack/Jira/GitHub into different evidence systems. Metadata-only object rows cannot cite a paragraph. |
| Fast "why is launch blocked?" with inspectable evidence | Existing typed Ent graph supplies `Workstream -> Ticket -> PullRequest/Message/Document` bounded reads; PR #68 rows supply `EvidenceAnchor` citations and `SourceRun` health. | `whyBlocked(workstreamID)` performs bounded graph reads, fetches anchors by `(object_kind, object_id)`, and fetches latest `SourceRun` by scope for warnings. It does not run global search or arbitrary traversal in the hot path. | Glean-first makes answer quality depend on live retrieval. Obsidian-first makes docs inspectable but leaves Slack/Jira/GitHub incomplete. Meta-only is fast but cannot show source-run partiality or exact evidence span. |

This is consistent with the real systems, not a vibes compromise:

- Meta TAO proves fast serving should use typed object/association reads, not arbitrary traversal: https://www.usenix.org/system/files/conference/atc13/atc13-bronson.pdf
- Palantir Foundry proves operational trust needs transactions/health and ontology separation: https://palantir.com/docs/foundry/data-integration/datasets/ and https://palantir.com/docs/foundry/data-integration/health-checks/
- Glean proves source content, metadata, permissions, and retrieval are necessary, but they should project from source-backed evidence: https://docs.glean.com/connectors/about
- Obsidian proves exact block/heading addressability matters, but that is one projection of the anchor model: https://obsidian.md/help/links

### 4. What must NOT be in PR #68

| Cut from PR #68 | Why I would block it |
|---|---|
| `SearchDocument` / `SearchChunk` | Retrieval is a projection over `EvidenceAnchor`. If PR #68 starts here, the index becomes truth and drifts from object identity/source-run completeness. |
| `DocumentSection` / `DocumentBlock` | Useful later, but document-shaped. PR #68 must also support Slack replies, Jira comments, and PR review comments. |
| Generic `nodes` / `edges` | Rejected. We chose Ent typed objects/associations for reviewability and Meta-style bounded reads. |
| Cypher or arbitrary traversal | Rejected. Hot product queries should be bounded GraphQL over typed Ent paths. |
| Vector DB / embeddings / LLM answers | Rejected. Retrieval and answer generation come after source identity, observation, and anchor semantics. |
| Full Foundry clone | No dataset branching, Marketplace resources, dynamic security engine, or action workflow engine in PR #68. |
| Source payload blobs inline in Ent tables | `payload_ref` only. Do not turn SQLite ontology tables into a blob lake. |
| `source_key` / `external_id` pasted onto every ontology object | Source identity belongs in `ExternalIdentity`; direct source fields on `Ticket`/`Document` recreate the rename/copy bug. |
| Tombstoning missing objects from partial runs | Illegal. Only a complete `SourceRun` for that scope can justify deletion inference. |

Final PR #68 recommendation:
Add the source-neutral foundation spine: `SourceRun`, `ExternalIdentity`, `SourceObservation`, and `EvidenceAnchor`, with only the fields listed above and indexes needed for source health, identity resolution, observation lookup, and evidence lookup. This accepts the moderator candidate but fixes the missing `SourceRun` row that would otherwise fail partial Slack crawls.

PR #69 immediately after:
Add the first projection over the spine. Palantir preference: `SearchDocument` / `SearchChunk` as a lexical-only Glean-style projection over `EvidenceAnchor`, because it unlocks unlinked evidence without making retrieval canonical. Obsidian `DocumentSection` / `DocumentBlock` can follow as a document-native projection over the same anchors.

What I would block in review:
I would block any PR #68 that puts `source_key`/`external_id` directly on canonical ontology objects as identity, any PR that creates `SearchChunk` or `DocumentBlock` as the canonical evidence unit, any PR that omits `SourceRun` but claims to solve partial crawls, any PR that tombstones missing source items from partial runs, and any PR that adds generic graph tables or query-language scope creep.

## Round E - Palantir Implementation Review

Read before replying:
- graph-foundations-live-chat.md
- graph-foundations-meta.md
- graph-foundations-palantir.md
- graph-foundations-glean.md
- graph-foundations-obsidian.md

### 1. Ent implementation risk

| Choice | Verdict | Exact Ent migration/query/codegen risk |
|---|---|---|
| `target_kind` + `target_id` on `ExternalIdentity` / `EvidenceAnchor` | Accept for PR #68 only as a provenance pointer, not as a graph traversal edge. | Ent will not generate typed FK traversal or compile-time referential safety for polymorphic `(kind,id)`. The query layer must not expose this as an Ent edge; it must hydrate targets through a resolver that switches on `target_kind`. This is the cost of keeping PR #68 small. Ent's strength is static schema/codegen over typed schemas and edges, so this is intentionally outside the primary graph path: https://entgo.io/docs/getting-started/ and https://entgo.io/docs/schema-edges/ |
| Typed optional edges from `ExternalIdentity` to every target table | Reject for PR #68. | This forces nullable `ticket_id`, `pull_request_id`, `document_id`, `message_id`, `person_id`, etc. Every new ontology object adds a migration, generated edge methods, more nullability checks, and reviewer surface. It also makes one identity row structurally tied to today's object list. |
| Edge schema / `Through` for the source spine | Reject. | Ent edge schemas are for one relationship; docs state edge schemas cannot be reused by more than one relationship. The Source Evidence Spine is intentionally polymorphic across Ticket/PR/Doc/Message/Person and cannot be modeled cleanly as one reusable Through edge without exploding into one table per target type. Source: https://entgo.io/docs/schema-edges/ |
| Typed edge `SourceObservation -> ExternalIdentity` and `EvidenceAnchor -> SourceObservation` | Require. | These are not polymorphic. They should be real Ent edges with FK fields. Otherwise the spine itself becomes stringly typed and every query needs manual joins. |

Concrete implementation rule:

```text
Allowed polymorphism:
 |
 +-- ExternalIdentity.target_kind + target_id
 +-- EvidenceAnchor.object_kind + object_id
 |
 `-- only for optional hydration back to ontology objects

Required typed Ent edges:
 |
 +-- SourceObservation.source_run_id -> SourceRun
 +-- SourceObservation.external_identity_id -> ExternalIdentity
 +-- EvidenceAnchor.source_observation_id -> SourceObservation
 +-- Evidence.evidence_anchor_id -> EvidenceAnchor
```

Migration risk:

```text
Typed optional edges everywhere
 |
 +-- PR #68 migration touches all future ontology target types
 +-- adding DocumentBlock later changes EvidenceAnchor schema again
 +-- Ent codegen emits target-specific edge methods before the model is stable

target_kind + target_id
 |
 +-- no FK safety
 +-- no Ent traversal
 +-- one compact migration
 +-- must add resolver validation + tests
```

Palantir verdict: accept the compact migration and explicitly document the loss of FK safety. We can add typed projection edges later when the product knows which target hydrations are hot.

### 2. Identity uniqueness

The ruling is not mergeable unless the uniqueness story is explicit. Without it, the first duplicate Jira key or mirrored Slack message corrupts identity resolution.

| Table | Required uniqueness / index | Why |
|---|---|---|
| `SourceRun` | unique `(source_key, source_instance, run_key)` | Prevents replaying the same connector run into duplicate coverage rows. If `run_key` is omitted, the same run can be inserted twice with different row IDs. |
| `SourceRun` | index `(source_key, source_instance, scope_kind, scope_key, started_at)` | Supports latest coverage lookup for `sourceHealth` and `whyBlocked` warnings. Ent supports multi-field indexes for retrieval/uniqueness: https://entgo.io/docs/schema-indexes/ |
| `ExternalIdentity` | unique `(source_key, source_instance, external_kind, external_id)` | One source item identity resolves to one canonical Cubicle target at a time. Add `external_kind`; otherwise `123` can collide between Jira issue/comment or GitHub PR/comment namespaces. |
| `ExternalIdentity` | index `(target_kind, target_id, identity_status)` | Lists all active/retired aliases for a canonical object after Jira rename or GitHub repo move. |
| `ExternalIdentity` | index `(source_key, source_instance, external_kind, identity_status)` | Lets importers find active identity mappings by source kind without scanning. |
| `SourceObservation` | unique `(source_run_id, external_identity_id, content_hash)` | Makes ingestion idempotent for a run while preserving observation history across content changes. |
| `SourceObservation` | index `(external_identity_id, observed_at)` | Lets queries find latest source state for a source item. |
| `SourceObservation` | index `(source_run_id, observed_kind)` | Explains what a run saw, by source item kind. |
| `EvidenceAnchor` | unique `(source_observation_id, anchor_kind, anchor_locator, source_span_key)` | Prevents duplicate paragraph/comment/message anchors inside one observation. Use a required `source_span_key` string with empty sentinel; SQLite unique indexes allow multiple `NULL` values, so nullable `source_span` is not safe for dedupe. |
| `EvidenceAnchor` | index `(object_kind, object_id, observed_at)` | Fast evidence hydration for `whyBlocked`. |
| `EvidenceAnchor` | index `(visibility_hash, observed_at)` | Cheap permission/freshness prefilter before hydrating objects. |

Required field correction:

```text
ExternalIdentity
 |
 +-- add external_kind
      -> jira_issue / jira_comment / github_pr / github_review_comment / slack_message / google_doc

EvidenceAnchor
 |
 +-- source_span_key required
      -> normalized string used for unique index
 +-- source_span optional
      -> display/debug value
```

Concrete failure without these indexes:

```text
Jira rename
 |
 +-- jira:CUB-123 and jira:PLAT-77 both point to ticket:42
 |
 `-- fine only if ExternalIdentity uniqueness prevents a second active jira:CUB-123 row

Slack mirrored message
 |
 +-- slack:workspaceA/channel1/ts1
 +-- slack:workspaceA/channel2/ts2
 |
 `-- two ExternalIdentity rows may point to one Message only after explicit resolution;
     uniqueness must be per source_instance/external_kind/external_id, not content_hash

Google Doc copy
 |
 +-- old doc id and copied doc id have same content_hash
 |
 `-- content_hash must not dedupe identity; it only dedupes observation content
```

### 3. Anchor text risk

`EvidenceAnchor.anchor_text` as proposed is too dangerous for PR #68 if it means full chunk text.

Risks:

```text
privacy
 |
 +-- storing full paragraph/comment text in the spine makes provenance rows sensitive

SQLite bloat
 |
 +-- repeated observed anchors can duplicate text across observations

SearchChunk creep
 |
 +-- full anchor_text plus indexes recreates SearchChunk under another name

Ent migration mismatch
 |
 +-- Ent automatic migration can manage normal tables, but SQLite FTS virtual tables/triggers are a separate schema concern
```

SQLite's FTS5 docs are explicit that external-content FTS tables require keeping the FTS table consistent with the content table, often via triggers, and inconsistencies can produce confusing results. That is too much hidden migration behavior for PR #68: https://sqlite.org/fts5.html

Smaller PR #68 field set:

| Field | Category | Rule |
|---|---|---|
| `text_hash` | citation | Required hash of exact cited span. |
| `text_preview` | citation | Bounded preview only, e.g. max 280 or 512 chars. Not full chunk text. |
| `text_preview_truncated` | citation | Boolean so UI knows the preview is incomplete. |
| `source_locator` | citation | Deep link/path to source. |
| `source_span_key` | citation | Required normalized source span identity for dedupe. |
| `payload_ref` on `SourceObservation` | citation | Optional pointer to local payload storage; not inline blob text. |

Day 0 lexical lookup:

```text
Allowed in PR #68
 |
 +-- LIKE/ContainsFold over bounded EvidenceAnchor.text_preview
 +-- debug-only, first N, no ranking claims

Blocked in PR #68
 |
 +-- SQLite FTS5 virtual table
 +-- external-content triggers
 +-- embeddings
 +-- SearchChunk
 +-- full source payload text in EvidenceAnchor
```

This still supports a small `contextSearchDebug(query)` for fixtures without pretending we built Glean.

### 4. Permission/freshness correctness

This query is illegal:

```sql
SELECT ea.*
FROM evidence_anchors ea
WHERE ea.object_kind = 'ticket'
  AND ea.object_id = ?
ORDER BY ea.observed_at DESC
LIMIT 20;
```

It is illegal because it can return deleted, invisible, stale, or partial-context evidence.

Minimum legal query shape:

```sql
SELECT ea.*, so.*, sr.*
FROM evidence_anchors ea
JOIN source_observations so ON so.id = ea.source_observation_id
JOIN source_runs sr ON sr.id = so.source_run_id
WHERE ea.object_kind = ?
  AND ea.object_id = ?
  AND so.is_deleted = false
  AND so.visibility_hash IN (:allowed_visibility_hashes)
  AND so.permission_policy_key IN (:allowed_permission_policy_keys)
  AND sr.status IN ('complete', 'partial')
ORDER BY ea.observed_at DESC
LIMIT :first;
```

Additional rule:

```text
sr.status = partial
 |
 +-- result may be returned only with coverage_warning
 +-- caller may not infer absence from missing anchors in that source scope

sr.status = failed/running
 |
 +-- result must be hidden by default or returned only in debug mode with explicit warning

so.is_deleted = true
 |
 +-- anchor must not support an active claim
 +-- it may appear only in history/provenance views
```

The exact product query that must enforce this:

```graphql
whyBlocked(workstreamID: ID!, firstEvidence: Int)
```

Illegal implementation:

```text
Workstream -> Ticket -> Evidence -> EvidenceAnchor
 |
 `-- returns anchor preview without joining SourceObservation and SourceRun
```

Legal implementation:

```text
Workstream -> Ticket -> Evidence -> EvidenceAnchor
 |
 +-- SourceObservation
 |     -> is_deleted=false
 |     -> permission policy/visibility allowed
 |
 +-- SourceRun
       -> status complete or partial
       -> partial becomes warning, not absence proof
```

This follows the same operational split Palantir's health checks imply: data freshness/sync status and item state are separate facts, not a single label on the object row. Sources: https://palantir.com/docs/foundry/data-integration/health-checks/ and https://palantir.com/docs/foundry/health-checks/checks-reference/

### 5. PR boundary

Cut PR #68 harder than Round D did.

| Cut | Reason |
|---|---|
| Full `anchor_text` | Replace with bounded `text_preview`, `text_preview_truncated`, `text_hash`, and `source_span_key`. |
| SQLite FTS5 | Requires virtual table/triggers/manual migration. Not Ent-codegen friendly for this PR. |
| Typed optional target edges | Causes migration/codegen explosion for every ontology target. Use `target_kind` + `target_id` with resolver validation in PR #68. |
| Cross-target resolver API | The schema can land first. Public GraphQL hydration comes PR #69. |
| `SearchDocument` / `SearchChunk` | Projection over anchors, not the spine. |
| `DocumentSection` / `DocumentBlock` | Source-specific projection over anchors, not the spine. |
| Raw payload blobs | Use `payload_ref`; raw payload storage policy comes later. |
| ACL enforcement engine | Keep permission fingerprints; enforcement/resolution comes later. |
| ProjectionRun / HealthCheck tables | `SourceRun.status` is enough for PR #68. Full pipeline health waits. |
| Any direct source identity fields on canonical ontology objects | Existing fields may remain for compatibility, but PR #68 must not expand or bless them as identity. |

Concrete Ent review requirements:

```text
1. All new schema types have explicit table names if pluralization is ambiguous.
2. All enum fields use shared constants.
3. All nullable fields are kept out of unique indexes unless normalized to a required key field.
4. Ent generated code is regenerated in the PR.
5. Schema test verifies expected fields and indexes.
6. Hook or service validation rejects unknown target_kind values.
7. No generated GraphQL public API exposes target_kind/target_id as a traversable edge.
```

Ent migration note:

Ent automatic migration keeps the database schema aligned with Ent schema objects, but this is precisely why PR #68 should stay in ordinary tables/indexes and avoid manual SQLite FTS virtual tables. Source: https://entgo.io/docs/migrate/

Approve PR #68 only if:
It adds only ordinary Ent schemas for `SourceRun`, `ExternalIdentity`, `SourceObservation`, and `EvidenceAnchor`; uses `target_kind` + `target_id` only as a validated provenance pointer; adds required uniqueness/indexes for run idempotency, source identity, observation history, and anchor dedupe; replaces full `anchor_text` with bounded `text_preview` plus hashes/locators; and makes `whyBlocked`-style evidence queries illegal unless they join `SourceObservation` and `SourceRun` for permission, deletion, and partial-coverage checks.

Block PR #68 if:
It introduces `SearchChunk`, `DocumentBlock`, FTS5, embeddings, typed optional edges to every ontology target, raw source payload blobs, generic graph tables, Cypher/query-language scope, direct source identity on canonical ontology rows, nullable fields inside unique dedupe indexes, or evidence reads that can return anchors without checking `visibility_hash`, `permission_policy_key`, `SourceObservation.is_deleted`, and `SourceRun.status`.

One migration/index I insist on:
`ExternalIdentity` must have a unique index on `(source_key, source_instance, external_kind, external_id)`. Without that, Jira rename, GitHub repo move, Slack mirror, and Google Doc copy all degrade into duplicate or conflicting source identity rows, and every downstream query becomes guesswork instead of ontology.
