Read before replying:
- graph-foundations-live-chat.md
- graph-foundations-meta.md
- graph-foundations-palantir.md
- graph-foundations-glean.md
- graph-foundations-obsidian.md

## Round A - Obsidian Reply

Meta claim quoted directly: "Cubicle's foundation should be **typed work objects + typed associations + bounded association-list reads**, with a narrow source metadata contract on source-backed rows."

Attack: this confuses serving discipline with knowledge foundation. Typed association lists are necessary for performance, but they do not define the smallest addressable unit of work knowledge. A bounded list of `Document`, `Message`, and `Evidence` objects still cannot answer "which paragraph blocked launch?" without a block-level address. Meta is proposing fast navigation between opaque containers.

Real-world evidence: Obsidian's implementation does not stop at note-level objects. Its official links docs support links to files, headings, and blocks. Obsidian defines a block as a paragraph, quote, or list item and lets users link to that exact unit with block identifiers. That is the foundation lesson for Cubicle: the graph must address the exact knowledge unit before search, backlinks, evidence, and local graph can be trustworthy. Source: https://obsidian.md/help/links

Meta claim quoted directly: "But Obsidian is wrong as a first foundation if it starts with `DocumentSection` and `DocumentBlock`. That makes Google Docs the center of a product that must also model Jira, GitHub, Slack, people, workstreams, blockers, and evidence."

Attack: false framing. Blocks do not make Google Docs the center. Blocks make exact evidence addressable. Cubicle can start with `DocumentSection` and `DocumentBlock` because `Document` already exists in the typed ontology, but the foundational pattern is broader:

```text
source container
 |
 +-- addressable content unit
      |
      +-- typed graph links
      +-- backlinks
      +-- evidence citations
      +-- search index rows later
      +-- source freshness later
```

That pattern applies to Google Docs paragraphs, Slack messages, PR comments, Jira descriptions, and decision notes. Starting with documents is not a docs island. It is the first concrete implementation of addressable content.

Real-world evidence: Obsidian stores notes as local Markdown files in a vault, then maintains a metadata cache that powers graph view and outline. That means the durable foundation is not just "node and edge"; it is local content plus parsed metadata. Source: https://obsidian.md/help/data-storage

Meta claim quoted directly: "Blocks are a content granularity, not the backend foundation."

Attack: wrong. Content granularity is exactly the backend foundation when the product promise is evidence-backed engineering context. A ticket edge can say `Workstream -> blocked_by -> Ticket`, but the user's trust comes from the cited content unit:

```text
Ticket says blocked
 |
 +-- why?
      |
      +-- DocumentBlock:
          "rollout blocked until checkpoint migration is verified"
```

Without the block, source metadata only says a document is fresh. It cannot cite the paragraph. Search only returns snippets. It cannot produce a stable graph anchor. Backlinks only show note-level adjacency. They cannot explain the sentence-level evidence path.

Real-world evidence: Obsidian graph view represents notes as nodes and internal links as edges, but its product also supports heading and block links because note-level graph is not precise enough for real knowledge work. Source: https://obsidian.md/help/plugins/graph and https://obsidian.md/help/links

Meta claim quoted directly: "A block-first PR would give precise paragraphs while tickets, PRs, messages, and association rows still lack a common source contract. That is a docs island, not an engineering execution graph."

Attack: the common source contract is not useful if it attaches only to coarse objects. A `source_updated_at` field on a 30-page launch document is a weak foundation. It proves the haystack is fresh. It does not identify the needle. Add minimal source fields to the block rows now:

```text
DocumentBlock
 |
 +-- source
 +-- source_instance
 +-- external_id
 +-- text_hash
 +-- source_updated_at
 +-- ingested_at
 +-- last_seen_at
```

That is not rejecting Meta. It is applying Meta's bounded-read discipline to the correct unit:

```text
Document
 |
 +-- sections(first:N)
      |
      +-- blocks(first:N)
```

Real-world evidence: Obsidian properties show that structured metadata is useful when attached to knowledge objects, and Bases builds table/card/list views over files and properties. The structure comes after the content unit exists, not before. Sources: https://obsidian.md/help/properties and https://obsidian.md/help/bases

Meta claim quoted directly: "The foundation API should answer bounded questions first."

Attack: agreed on bounded. Rejected on object granularity. The bounded API should not only page through tickets and messages. It must page through addressable content inside a document:

```graphql
documentBlocks(documentID: ID!, first: Int, after: String) {
  nodes {
    id
    headingPath
    blockOrdinal
    textPreview
    textHash
    sourceUpdatedAt
    lastSeenAt
  }
}
```

That is a bounded association list. It is just not pretending that `Document` is precise enough.

Foundation decision demanded:

```text
PR #68 = DocumentSection + DocumentBlock
 |
 +-- keep Ent typed schemas
 +-- keep bounded GraphQL reads
 +-- add minimal source freshness fields on blocks
 +-- do not add generic nodes/edges
 +-- do not add SearchChunk yet
 +-- do not add full ingestion/projection yet
```

How this maps to Cubicle:

```text
Document
 |
 +-- DocumentSection
      |
      +-- DocumentBlock
           |
           +-- future KnowledgeLink
           +-- future MentionCandidate
           +-- future Evidence citation
           +-- future SearchChunk/SearchIndex row
```

What breaks if Cubicle copies Obsidian too literally: free-form links alone are not enough. Cubicle must not become a loose note graph. It still needs Ent typed objects, typed associations, source metadata, and bounded reads. But copying Meta too literally creates a storage graph that knows tickets and documents exist while missing the exact evidence that makes the graph useful.

Final foundation attack: source metadata first is a safety label on coarse rows. Search chunks first are a duplicate content substrate. Blocks first create the canonical address that both metadata and search can attach to.

Read before replying:
- graph-foundations-live-chat.md
- graph-foundations-meta.md
- graph-foundations-palantir.md
- graph-foundations-glean.md
- graph-foundations-obsidian.md

## Round B - Obsidian Counterattack

Meta claim quoted: "Cubicle's foundation should be **typed work objects + typed associations + bounded association-list reads**, with a narrow source metadata contract on source-backed rows."

Accept the bounded-read discipline. Reject it as the foundation. Meta's own evidence says TAO is an object/association serving system: data items are typed objects, relationships are typed edges grouped in association lists, and TAO solves cache/read-scale pain for Facebook product surfaces. That is a serving substrate, not a knowledge substrate. Source: https://engineering.fb.com/2013/06/25/core-infra/tao-the-power-of-the-graph/

Cubicle's foundation problem is smaller and more brutal: the product must cite the exact unit of evidence. A bounded association list from `Workstream -> Document` is fast and still useless if the blocker is one paragraph in a 30-page launch doc. Obsidian's official product implementation supports links to files, headings, and blocks, and defines blocks as paragraph/list/quote-level units with stable block identifiers. That is real-world proof that note-level nodes are not precise enough for knowledge work. Source: https://obsidian.md/help/links

Meta claim quoted: "A block-first PR would give precise paragraphs while tickets, PRs, messages, and association rows still lack a common source contract. That is a docs island, not an engineering execution graph."

Attack: this is a fake dichotomy. Blocks are not a docs island. Blocks are the first canonical addressable content unit. The correct foundation is not:

```text
Document only
 |
 +-- source metadata
```

The correct foundation is:

```text
Document
 |
 +-- DocumentSection
      |
      +-- DocumentBlock
           |
           +-- source metadata
           +-- future evidence link
           +-- future retrieval index row
```

That still respects Meta: `Document -> sections(first:N) -> blocks(first:N)` is a bounded association-list read. The difference is that Obsidian refuses to pretend the container is the evidence.

Palantir claim quoted: "Correct foundation compromise: `PR #68 = Ontology Lifecycle Metadata`."

Accept the trust concern. Reject the first move. Palantir is right that lifecycle, transactions, health, and provenance matter. Foundry datasets explicitly model transaction types like `SNAPSHOT`, `APPEND`, `UPDATE`, and `DELETE`, and Foundry health checks validate successful jobs, build duration, and freshness over time. Sources: https://www.palantir.com/docs/foundry/data-integration/datasets and https://www.palantir.com/docs/foundry/data-integration/health-checks

But that evidence cuts against lifecycle metadata as PR #68. Palantir's own model tracks changes over meaningful data resources and pipeline outputs. A lifecycle field on a coarse `Document` row is not meaningful enough. It can prove the launch doc was last seen; it cannot prove which paragraph changed, disappeared, or became stale. If the evidence unit is wrong, the lifecycle label is a clean label on a bad abstraction.

Palantir claim quoted: "Do not put `source_key` / `source_instance` / `external_id` everywhere in PR #68 as if one source row equals one ontology object."

Accepted. This is the strongest Palantir point. Source identity and ontology identity are not the same. But the conclusion should be block foundation with minimal observation metadata, not graph-wide lifecycle metadata first:

```text
DocumentBlock
 |
 +-- text_hash
 +-- source_updated_at
 +-- ingested_at
 +-- last_seen_at
```

Do not pretend this solves full provenance. It gives Palantir the first observable unit that future `SourceRecord`, `ProjectionRun`, and `DataCheck` can validate.

Glean claim quoted: "Cubicle PR #68 should be Option D: **search/retrieval sidecar first**, with a tiny trust contract on searchable source items."

Accept discovery as a foundation requirement. Reject a parallel `SearchDocument` / `SearchChunk` truth store as the first move. Glean's public docs prove that enterprise search needs content and permissions together: connectors fetch source data and permission maps, parse titles, bodies, comments, media, and permissions, keep data updated, and use indexed access for low-latency permission-aware search. Sources: https://docs.glean.com/connectors/about and https://docs.glean.com/security/knowledge-graph

But Glean's lesson is not "create a second content substrate before the ontology has content units." The lesson is "retrievable units need permissions, metadata, and freshness." Cubicle should make `DocumentBlock` the retrievable unit first, then let search index it. Otherwise every future relationship has to pick between two competing addresses:

```text
Bad foundation:
 |
 +-- DocumentBlock later
 +-- SearchChunk now
      |
      +-- optional object_kind
      +-- optional object_id

Better foundation:
 |
 +-- DocumentBlock now
      |
      +-- SearchIndex/SearchChunk later
      +-- KnowledgeLink later
      +-- Evidence citation later
```

Glean claim quoted: "A typed association list cannot retrieve evidence it has never indexed."

Accepted, but incomplete. A search sidecar cannot safely explain evidence it cannot stably address. Indexing text before defining the canonical block address creates a substrate that is easy to query and hard to connect. The first foundation should make evidence addressable; the next foundation should make it discoverable.

Final Obsidian foundation position:

```text
PR #68 = DocumentSection + DocumentBlock
 |
 +-- Ent typed schemas
 +-- bounded GraphQL reads
 +-- minimal source/lifecycle fields on blocks
 +-- no generic node/edge table
 +-- no full ingestion platform
 +-- no standalone search chunk truth store
```

Required foundation API:

```graphql
documentBlocks(documentID: ID!, first: Int, after: String) {
  nodes {
    id
    headingPath
    blockOrdinal
    textPreview
    textHash
    sourceUpdatedAt
    ingestedAt
    lastSeenAt
  }
}
```

What breaks if Cubicle copies Obsidian too literally: free-form backlinks cannot replace typed objects, source health, or permission-aware retrieval. But what breaks if Cubicle follows Meta, Palantir, or Glean first is worse: it builds fast containers, trustworthy labels, or searchable snippets before it has the canonical evidence address the whole system must point at.

Read before replying:
- graph-foundations-live-chat.md
- graph-foundations-meta.md
- graph-foundations-palantir.md
- graph-foundations-glean.md
- graph-foundations-obsidian.md

## Round C - Obsidian Foundation Invariants

### 1. Canonical identity

Canonical identity must be a Cubicle-owned object identity, not a source key.

```text
CubicleObjectID
 |
 +-- stable internal ID
 +-- typed Ent object kind
 +-- survives source rename/copy/move/merge
 |
 +-- ExternalIdentity later
      |
      +-- source
      +-- source_instance
      +-- external_id
      +-- valid_from / valid_to
      +-- merge / alias reason
```

Jira keys can rename or merge. GitHub repositories can move. Slack messages can be mirrored. Google Docs can be copied. Therefore `CUB-123`, `repo/name#88`, Slack timestamp, and Google Doc ID are source identities, not Cubicle identities.

This accepts Palantir's strongest point: raw source shape and operational object shape are not the same. Foundry models object types as real-world entities/events and link types as relationships between object types, while data integration/datasets are separate source/data resources. Sources: https://palantir.com/docs/foundry/object-link-types/object-types-overview/, https://palantir.com/docs/foundry/object-link-types/link-types-overview/, https://www.palantir.com/docs/foundry/data-integration/datasets

This also aligns with Obsidian's rename behavior: links can be updated when a file is renamed, and aliases provide alternate names for the same note. That is the product pattern Cubicle needs for Jira key rename and old references. Source: https://obsidian.md/help/links

Foundation answer:

```text
Ticket(CubicleObjectID=internal)
 |
 +-- ExternalIdentity(jira, CUB-123, valid_to=...)
 +-- ExternalIdentity(jira, CUB-456, valid_from=...)
 +-- ObjectAlias("CUB-123", reason=renamed)
```

### 2. Canonical evidence unit

Canonical evidence must be an addressable content anchor, not a whole object, not a search chunk, and not a source record.

Obsidian's real product proves why: it supports links to headings and blocks, and defines a block as a paragraph, block quote, or list item with a block identifier. That is the smallest user-comprehensible unit people cite in knowledge work. Source: https://obsidian.md/help/links

For Cubicle, the generic invariant is:

```text
EvidenceAnchor
 |
 +-- source-backed content unit
 +-- stable enough to cite
 +-- can carry text_hash/source span
 +-- can attach provenance/freshness
 +-- can be indexed later
```

Concrete source-specific shapes:

```text
Google Doc
 |
 +-- DocumentSection
      |
      +-- DocumentBlock

Slack
 |
 +-- Message / ThreadMessage

Jira
 |
 +-- IssueDescriptionBlock
 +-- IssueComment

GitHub
 |
 +-- PullRequestComment
 +-- ReviewComment
```

Source records are provenance inputs. Association rows are graph claims. Search chunks are retrieval projections. None of those should be the canonical cited evidence if the user asks "show me why."

Glean is right that content has to be parsed and searchable: its connector docs describe parsing titles, body, comments, media, permissions, and common file types; its knowledge graph docs describe full content analysis, metadata extraction, permissions, and facets. Sources: https://docs.glean.com/connectors/about, https://docs.glean.com/security/knowledge-graph

But that evidence supports content anchors before search chunks. If a chunk is the first durable thing, search becomes the address. That is wrong. Search should index the address.

### 3. Freshness authority

Freshness has two levels, and the debate has been mixing them:

```text
Coverage freshness
 |
 +-- source run / crawl window / checkpoint
 +-- allowed to say partial or failed

Item freshness
 |
 +-- source-backed EvidenceAnchor or ontology row
 +-- allowed to say observed / stale / deleted for that item
```

Authority rules:

```text
SourceRun / SyncCheckpoint
 |
 +-- only layer allowed to say a source or time window is partial

SourceRecord / Observation later
 |
 +-- best authority for what the source said at crawl time

EvidenceAnchor / source-backed object
 |
 +-- may expose derived last_seen_at, source_updated_at, deleted_at, visibility_hash

SearchChunk / SearchIndex
 |
 +-- never freshness authority
 +-- copies freshness from the anchor/source observation
```

Palantir is right that operational systems need health/freshness checks. Foundry health checks validate successful jobs, build duration, and freshness for scheduled data pipelines. Source: https://www.palantir.com/docs/foundry/data-integration/health-checks

Glean is right that connector freshness/permissions matter at retrieval time. Its connector docs say connectors keep data updated through webhooks or incremental crawling and fetch permissions so search follows source access rules. Source: https://docs.glean.com/connectors/about

Obsidian adds a useful warning: its metadata cache powers graph/outline but can get out of sync with underlying files and may need rebuild. That is exactly why Cubicle search chunks must not become freshness authority. A cache/index is useful, but not the truth layer. Source: https://obsidian.md/help/data-storage

Foundation answer:

```text
partial Slack crawl
 |
 +-- SourceRun says coverage is partial
 +-- Message anchors inside covered window can be fresh
 +-- local graph must show source warning

deleted Jira issue
 |
 +-- ExternalIdentity preserves old key
 +-- source observation marks deleted/merged
 +-- Ticket canonical object can redirect or tombstone
```

### 4. Retrieval/addressability ordering

Ordering must be:

```text
1. canonical addressable evidence unit
2. minimal observation metadata on that unit
3. retrieval index over that unit
4. source-run/provenance expansion
5. local graph / answer API over typed objects + anchors
```

Not:

```text
metadata on coarse objects first
 |
 `-- labels the haystack

search chunks first
 |
 `-- makes the index the address

full ingestion first
 |
 `-- builds platform machinery before cited evidence exists
```

Real-world evidence:

- Obsidian supports file, heading, and block links because exact addressability is required for knowledge navigation: https://obsidian.md/help/links
- Obsidian graph view visualizes linked notes, but block/heading links exist because note-level graph alone is too coarse: https://obsidian.md/help/plugins/graph
- Glean proves retrieval needs parsed content and permissions, but that should index canonical anchors rather than create the first truth substrate: https://docs.glean.com/connectors/about
- Palantir proves freshness/provenance matters over changing datasets and pipeline outputs, but that metadata is useful only when attached to the right data resource: https://www.palantir.com/docs/foundry/data-integration/datasets
- Meta proves bounded typed graph serving matters, but TAO is an object/association serving model, not a citation model: https://engineering.fb.com/2013/06/25/core-infra/tao-the-power-of-the-graph/

Foundation answer for the four failures:

```text
partial Slack crawl
 |
 +-- SourceRun later says partial
 +-- Message anchors are citeable only inside covered window

renamed/merged/deleted Jira issue
 |
 +-- CubicleObjectID stays stable
 +-- ExternalIdentity/ObjectAlias later preserves old keys

unlinked Google Doc paragraph
 |
 +-- DocumentBlock is the canonical evidence anchor
 +-- MentionCandidate/SearchIndex later discovers it

"why is launch blocked?"
 |
 +-- fast path uses typed Ent associations
 +-- answer cites EvidenceAnchor rows
 +-- source health warnings come from SourceRun/observations
```

Non-negotiable foundation invariant:
Every user-visible graph claim must be able to point to a canonical Cubicle object identity and, when evidence-backed, to an addressable evidence anchor that is finer than the container object. Search indexes and source records can support that anchor; they cannot replace it.

Concession to strongest opposing camp:
Palantir has the strongest objection: source identity and ontology identity must be separated. Obsidian accepts that `source`, `source_instance`, and `external_id` cannot be treated as the canonical object identity. Cubicle needs `ExternalIdentity` and source-run provenance immediately after the first addressable evidence anchor lands.

Concrete PR #68 after this invariant:
Add `DocumentSection` + `DocumentBlock` as the first `EvidenceAnchor` implementation, with bounded GraphQL reads and minimal item observation fields: `text_hash`, `source_updated_at`, `ingested_at`, `last_seen_at`, and `visibility_hash`. Do not add `SearchChunk`, generic node/edge tables, full ingestion runs, or graph-wide lifecycle mixins in this PR.

Read before replying:
- graph-foundations-live-chat.md
- graph-foundations-meta.md
- graph-foundations-palantir.md
- graph-foundations-glean.md
- graph-foundations-obsidian.md

## Round D - Obsidian Design Review

### 1. Accept/reject the moderator candidate

| Moderator Candidate | Obsidian Verdict | Exact Row/Table Failure If Changed |
|---|---|---|
| `ExternalIdentity` | Accept. Source identity must not be canonical object identity. | If PR #68 instead puts `source_key/source_instance/external_id` directly on `Ticket`, `resolveWorkObject("CUB-123")` fails after Jira renames or merges the issue because the old key is overwritten or stranded on the wrong row. Palantir's Foundry separation between data resources and ontology object types supports this split: https://www.palantir.com/docs/foundry/data-integration/datasets and https://palantir.com/docs/foundry/object-link-types/object-types-overview/ |
| `SourceObservation` | Accept. Observed source state needs a durable row. | If PR #68 only adds lifecycle fields to `Ticket`, `Document`, and `Message`, `whyBlocked(workstream)` cannot distinguish "Slack did not mention the blocker" from "Slack crawl failed before that thread." Palantir health checks exist because synced data reliability and freshness must be validated over pipeline runs: https://www.palantir.com/docs/foundry/data-integration/health-checks |
| `EvidenceAnchor` | Accept, but make it mandatory in PR #68, not later. This is the Obsidian line. | If PR #68 has `ExternalIdentity` and `SourceObservation` but no `EvidenceAnchor`, `whyBlocked(workstream)` returns a fresh document or fresh ticket but cannot cite the exact paragraph. Obsidian supports links to headings and blocks because note-level/document-level anchors are too coarse: https://obsidian.md/help/links |
| `SearchChunk later` | Accept. Search is a projection over anchors. | If `SearchChunk` arrives before `EvidenceAnchor`, `searchChunks("why is launch blocked")` returns a row whose address is the index row itself. That makes retrieval the citation system. Glean proves content and permissions must be indexed, but connectors parse source content into a search system; that does not make chunks canonical identity: https://docs.glean.com/connectors/about |
| `DocumentBlock later` | Accept only if `EvidenceAnchor` has document-capable locator/span fields now. | If PR #68 defers both `DocumentBlock` and anchor spans, an unlinked Google Doc paragraph cannot be represented at all. Obsidian block links prove paragraph/list/quote-level addressability is a product primitive, not decorative UX: https://obsidian.md/help/links |
| Missing `SourceRun` in the candidate diagram | Reject. Add `SourceRun` in PR #68. | Without `SourceRun`, the partial Slack crawl failure is not representable. `SourceObservation` says one message was seen; it cannot say channel `C123` was only crawled through cursor `abc` and therefore missing messages are unknown, not deleted. |

Concrete rejection of weak alternatives:

```text
Graph mixin only
 |
 +-- Ticket.source_key = jira
 +-- Ticket.external_id = CUB-123
 |
 `-- fails resolveWorkObject("CUB-123") after rename/merge

Search first
 |
 +-- SearchChunk.object_id optional
 +-- SearchChunk.freshness_state self-owned
 |
 `-- fails because index row becomes citation + freshness authority

DocumentBlock first
 |
 +-- DocumentBlock exists
 +-- no SourceRun
 +-- no ExternalIdentity
 |
 `-- fails Slack partial crawl and Jira rename/delete
```

Meta is right that serving must stay typed and bounded. TAO models typed objects and associations and serves point/range/count association reads over those shapes; Cubicle should not build generic `nodes`/`edges` or Cypher in PR #68. Source: https://engineering.fb.com/2013/06/25/core-infra/tao-the-power-of-the-graph/

### 2. Minimal PR #68 schema

Only four Ent schemas plus one existing-schema edge are allowed. This is the smallest spine that does not lie about identity, freshness, or citations.

| Ent Schema / Field | Category | Purpose |
|---|---|---|
| `SourceRun.id` | identity | Cubicle-owned run identity. |
| `SourceRun.source_key` | identity | Source family, e.g. `slack`, `jira`, `github`, `google_docs`. |
| `SourceRun.source_instance` | identity | Workspace/repo/site/account namespace. |
| `SourceRun.scope_key` | identity | Crawl scope, e.g. Slack channel, Jira project, GitHub repo, Drive folder. |
| `SourceRun.status` | freshness | `complete`, `partial`, `failed`, `running`. This is the only PR #68 row allowed to speak about crawl completeness. |
| `SourceRun.started_at` | freshness | Run start timestamp. |
| `SourceRun.completed_at` | freshness | Run completion timestamp when terminal. |
| `SourceRun.coverage_start_at` | freshness | Lower bound of source time window covered. |
| `SourceRun.coverage_end_at` | freshness | Upper bound of source time window covered. |
| `SourceRun.checkpoint_token` | freshness | Source cursor/checkpoint for resuming or explaining partial coverage. |
| `ExternalIdentity.id` | identity | Cubicle-owned identity row. |
| `ExternalIdentity.object_kind` | identity | Target Ent kind: `ticket`, `document`, `message`, `pull_request`, etc. |
| `ExternalIdentity.object_id` | identity | Target Ent row ID. |
| `ExternalIdentity.source_key` | identity | Source family. |
| `ExternalIdentity.source_instance` | identity | Source namespace. |
| `ExternalIdentity.external_id` | identity | Source-owned ID/key/URL tuple. |
| `ExternalIdentity.identity_status` | identity | `active`, `retired`, `merged`, `deleted`, `alias`. |
| `ExternalIdentity.valid_from` | identity | When this source identity became valid. |
| `ExternalIdentity.valid_to` | identity | When this source identity stopped being valid. |
| `SourceObservation.id` | identity | Cubicle-owned observation row. |
| `SourceObservation.source_run_id` | freshness | Run that produced the observation. |
| `SourceObservation.external_identity_id` | identity | Source identity observed, nullable until resolved. |
| `SourceObservation.observed_kind` | identity | Source item kind, e.g. issue, doc, message, PR comment. |
| `SourceObservation.source_updated_at` | freshness | Source-reported update time. |
| `SourceObservation.observed_at` | freshness | Time Cubicle observed the source item. |
| `SourceObservation.is_deleted` | freshness | Source item deletion/tombstone signal. |
| `SourceObservation.deleted_at` | freshness | Source deletion time if known. |
| `SourceObservation.visibility_hash` | permission | Stable hash of visible ACL/policy state. |
| `SourceObservation.content_hash` | freshness | Hash of normalized source item content/metadata. |
| `EvidenceAnchor.id` | identity | Cubicle-owned citation anchor row. |
| `EvidenceAnchor.source_observation_id` | freshness | Observation that this citation came from. |
| `EvidenceAnchor.anchor_kind` | citation | `document_span`, `message_body`, `thread_reply`, `issue_comment`, `pr_review_comment`, etc. |
| `EvidenceAnchor.anchor_locator` | citation | Source-local locator: heading path, Slack ts, comment ID, byte/char range key. |
| `EvidenceAnchor.source_span` | citation | Offset/range/path inside the observed item. |
| `EvidenceAnchor.text_hash` | citation | Hash of cited text to detect drift. |
| `EvidenceAnchor.text_preview` | citation | Short display snippet for inspectable answers. |
| `Evidence.evidence_anchor_id` | citation | Existing `Evidence` row points to exact anchor. |

Explicitly not in the schema:

```text
SearchDocument
SearchChunk
DocumentSection
DocumentBlock
generic Object / generic Edge
ProjectionRun
HealthCheck
permission policy engine
vector embedding
```

### 3. Four failure-mode proof

| Failure Mode | Rows Written | Query That Now Works | Why The Competing First Moves Fail |
|---|---|---|---|
| Partial Slack crawl | `SourceRun(source_key=slack, source_instance=T1, scope_key=C123, status=partial, coverage_end_at=10:15, checkpoint_token=abc)` plus `SourceObservation` rows for messages actually seen. | `whyBlocked(workstreamID)` can include evidence from observed Slack messages and a source warning: `slack:C123 partial through 10:15`. Missing Slack messages are not treated as deleted or absent truth. | `DocumentBlock` first has no row that can say Slack channel coverage was partial. `SearchChunk` first can return stale snippets but still cannot represent crawler scope. Graph mixin only cannot distinguish item freshness from crawl completeness. |
| Renamed/merged/deleted Jira issue | `ExternalIdentity(object=ticket:42, source_key=jira, external_id=CUB-123, identity_status=retired, valid_to=...)`; `ExternalIdentity(object=ticket:42, external_id=CUB-456, identity_status=active)`; `SourceObservation(is_deleted=true)` if source tombstones. | `resolveWorkObject("CUB-123")` returns canonical `Ticket:42`, old alias status, and current key. Backlinks mentioning old keys still resolve. | `Ticket.external_id` as a direct field loses identity history or requires overwriting. `SearchChunk` can find text `CUB-123` but cannot decide canonical object identity. `DocumentBlock` cannot model Jira identity at all. |
| Unlinked but relevant Google Doc paragraph | `ExternalIdentity(object=document:7, source_key=google_docs, external_id=doc_abc)`; `SourceObservation(document:7, content_hash=...)`; `EvidenceAnchor(anchor_kind=document_span, anchor_locator="Launch Readiness > Checkpoint migration", source_span="paragraph:18", text_hash=..., text_preview="rollout blocked until checkpoint migration is verified")`. | `evidenceAnchors(object=document:7)` can cite the exact paragraph. Later `searchChunks` or `MentionCandidate` can discover it, but PR #68 already has the canonical citation target. | Graph metadata first labels `Document:7` fresh but still cites a whole haystack. Search first makes `SearchChunk` the citation identity. Block first handles docs but not Slack/Jira/GitHub with the same source-neutral anchor contract. |
| Fast "why is launch blocked?" with inspectable evidence | Existing typed graph serves `Workstream -> Ticket -> Evidence`; `Evidence.evidence_anchor_id` hydrates exact `EvidenceAnchor`; `SourceObservation` gives item freshness/visibility; `SourceRun` gives source coverage warning. | `whyBlocked(workstreamID)` can return bounded object path plus snippet: `Ticket:42 blocked_by evidence_anchor:99`, with `visibility_hash`, `observed_at`, and `slack partial` warning if relevant. | Pure Meta association reads return adjacent objects but not exact citation. Pure Palantir lifecycle fields return trust labels but not evidence text. Pure Glean chunks return snippets but weak graph path. Pure Obsidian document blocks return paragraph precision but not source-run or identity coverage. |

This design is source-neutral enough for Slack/Jira/GitHub/Docs and still keeps Obsidian's non-negotiable product requirement: every answer has an exact inspectable anchor. Glean's docs show connectors parse bodies, comments, media, permissions, and source-specific structures, which is why `EvidenceAnchor.anchor_kind` must not be document-only. Source: https://docs.glean.com/connectors/about

### 4. What must NOT be in PR #68

Brutal cuts:

- No `SearchDocument` / `SearchChunk`. Retrieval is a projection over `EvidenceAnchor`; do not let the index become truth.
- No `DocumentSection` / `DocumentBlock`. They are document-specific projections over `EvidenceAnchor`; do not make Google Docs the graph foundation.
- No generic `nodes` / `edges`. Ent typed schemas stay. Meta/TAO's object-association discipline is enough; Cypher is out. Source: https://engineering.fb.com/2013/06/25/core-infra/tao-the-power-of-the-graph/
- No graph-wide `source_key/source_instance/external_id` pasted onto `Ticket`, `Document`, `Message`, or `PullRequest`. Source identity belongs in `ExternalIdentity`.
- No `ProjectionRun`, `HealthCheck`, scheduler, retry engine, or full Foundry clone. Palantir proves health matters, but PR #68 only needs `SourceRun.status` and observation rows.
- No vector embeddings, LLM summaries, RAG answer generation, semantic ranking, or mention inference.
- No ACL engine. `visibility_hash` is a permission fingerprint, not enforcement.
- No local graph API changes beyond the existing `Evidence.evidence_anchor_id` hook. Product APIs come after the spine.

Final PR #68 recommendation:
Accept the moderator candidate with one mandatory amendment: include `SourceRun`. Implement `SourceRun`, `ExternalIdentity`, `SourceObservation`, `EvidenceAnchor`, and `Evidence.evidence_anchor_id`. This is the source-neutral evidence/provenance spine. It preserves Meta typed serving, Palantir identity/freshness separation, Glean future retrieval, and Obsidian exact citations.

PR #69 immediately after:
Build the first projection on top of the spine. Obsidian preference: `DocumentSection` + `DocumentBlock` backed by `EvidenceAnchor` for Google Docs/design docs. Glean can then index those anchors without becoming the citation authority.

What I would block in review:
Any PR #68 that makes `SearchChunk`, `DocumentBlock`, or direct `source_key/external_id` fields on ontology objects canonical. I would also block any design that cannot answer this exact query path: `whyBlocked(workstreamID) -> Ticket -> Evidence -> EvidenceAnchor -> SourceObservation -> SourceRun`.

## Round E - Obsidian Implementation Review

Read before replying:
- graph-foundations-live-chat.md
- graph-foundations-meta.md
- graph-foundations-palantir.md
- graph-foundations-glean.md
- graph-foundations-obsidian.md

### 1. Ent implementation risk

`target_kind` + `target_id` is acceptable for PR #68, but only inside `ExternalIdentity` and only as a source-identity resolver. It must not become a graph traversal model.

| Implementation choice | What Ent gives us | Exact migration/query/codegen pain | Obsidian verdict |
|---|---|---|---|
| `target_kind` + `target_id` on `ExternalIdentity` | One Ent schema, stable migration, simple composite indexes. Ent IDs are table-local by default unless universal IDs are enabled, so `target_kind` is required context for `target_id`. Source: https://entgo.io/docs/migrate/ | No FK to `Ticket`/`Document`/`Message`; integrity must be enforced by enum validation, a resolver, tests, and mutation hooks. Ent hooks are the correct place for mutation-time guardrails when DB constraints cannot express the invariant. Source: https://entgo.io/docs/hooks/ | Accept for PR #68. This is an identity map, not the work graph. |
| Typed optional edges on `ExternalIdentity` | Ent edges give real typed traversal and DB-side foreign keys for concrete relationships. Source: https://entgo.io/docs/schema-edges/ | Bad first move. Every new target type adds `ticket_id`, `pull_request_id`, `document_id`, `message_id`, later `jira_comment_id`, `pr_review_comment_id`, etc. Codegen churns new optional edge APIs, migrations add nullable columns, and the app still needs "exactly one target edge is set" validation that Ent does not magically infer from a pile of optional edges. | Block for PR #68. It is type-pure but migration-hostile. |
| Generic `object_id` without kind | Smaller table. | Incorrect. In SQL Ent, different tables can have the same integer ID unless universal IDs are explicitly enabled. Source: https://entgo.io/docs/migrate/ | Block. It corrupts identity. |
| Generic `nodes` / `edges` table | Avoids polymorphic resolver. | Rejected already. It abandons the typed Ent object/association model that the backend is being built around. | Block. |

Strict rule:

```text
ExternalIdentity.target_kind + target_id
 |
 +-- allowed for source identity resolution
 +-- must use a closed enum for target_kind
 +-- must resolve through one small identity resolver
 +-- must not be exposed as "query arbitrary graph by kind/id"
```

Typed Ent edges are still mandatory where the relationship is not polymorphic:

```text
SourceRun -> SourceObservation
SourceObservation -> EvidenceAnchor
Evidence -> EvidenceAnchor
```

Those are real one-to-many / many-to-one relationships. If PR #68 models those as string IDs, it is throwing away Ent for no reason.

### 2. Identity uniqueness

The ruling is still under-specified. `ExternalIdentity(source_key, source_instance, external_id)` is not enough because different source item classes can share identifier shapes. PR #68 needs `external_kind`.

Required identity fields:

```text
ExternalIdentity
 |
 +-- source_key          // slack, jira, github, google_docs
 +-- source_instance     // workspace/site/org/account namespace
 +-- external_kind       // jira_issue, jira_comment, slack_message, github_pr, google_doc
 +-- external_id         // source-owned stable id tuple
 +-- target_kind         // Cubicle object kind
 +-- target_id           // Cubicle object id within target_kind
 +-- identity_status     // active, alias, retired, merged, deleted
```

Indexes PR #68 must include:

| Table | Index | Why this is not optional |
|---|---|---|
| `SourceRun` | unique `(source_key, source_instance, run_key)` | Connector retries must be idempotent. Without a run key, the same partial Slack crawl can create two competing coverage facts. |
| `SourceRun` | `(source_key, source_instance, scope_kind, scope_key, started_at)` | `whyBlocked` needs latest coverage for a Slack channel/Jira project/GitHub repo without scanning all runs. |
| `ExternalIdentity` | unique `(source_key, source_instance, external_kind, external_id)` | One source item identity must not resolve to two canonical Cubicle objects. Ent supports composite unique indexes directly. Source: https://entgo.io/docs/schema-indexes/ |
| `ExternalIdentity` | `(target_kind, target_id, identity_status)` | Needed to list aliases after Jira rename, GitHub repo move, and Google Doc copy resolution. |
| `SourceObservation` | unique `(source_run_id, external_identity_id)` | A run should observe one source identity once. Retried writes update, not duplicate. |
| `SourceObservation` | `(external_identity_id, observed_at)` | Needed for source history and stale/deleted debugging. |
| `EvidenceAnchor` | unique `(source_observation_id, anchor_kind, anchor_locator)` | Prevents duplicate paragraph/comment/message anchors inside the same observed source item. |
| `EvidenceAnchor` | `(source_observation_id, ordinal)` | Ordered local navigation through anchors. |

Concrete failure cases:

```text
Jira project key rename
 |
 +-- Atlassian documents project key edits and re-indexing.
 |   Source: https://confluence.atlassian.com/spaces/ADMINJIRASERVER112/pages/1688896748/Editing%2Ba%2Bproject%2Bkey
 |
 `-- If "CUB-123" is the only identity, old backlinks either overwrite or duplicate Ticket rows.
     ExternalIdentity must preserve old and new source identities against one Ticket.

GitHub repo move
 |
 +-- GitHub repository transfers can leave redirects, and old locations can later be reused.
 |   Source: https://docs.github.com/en/repositories/creating-and-managing-repositories/transferring-a-repository
 |
 `-- owner/repo#PR is a display locator, not sufficient canonical identity.
     Store source identity history; do not mutate PullRequest identity.

Slack mirrored message
 |
 +-- Slack permalinks are built from channel ID and message timestamp.
 |   Source: https://docs.slack.dev/reference/methods/chat.getPermalink
 |
 `-- source_instance + external_kind must be part of uniqueness, or mirrored/exported workspaces collide.

Google Doc copy
 |
 +-- Google Drive API has an explicit file copy operation.
 |   Source: https://developers.google.com/workspace/drive/api/guides/create-file
 |
 `-- identical content_hash must not merge copied documents automatically.
     Copy detection is a later resolver, not a PR #68 uniqueness rule.
```

### 3. Anchor text risk

`EvidenceAnchor.anchor_text` is too broad. It leaks source text, bloats SQLite, and quietly rebuilds `SearchChunk` under a less honest name.

Obsidian's line is exact addressability, not hoarding text. Block links prove the product value of stable anchors, not storing every paragraph in the citation table. Source: https://obsidian.md/help/links

Use this smaller field set:

| Field | Keep? | Rule |
|---|---|---|
| `text_hash` | Yes | Hash of exact cited text/span. Required for drift detection. |
| `text_preview` | Yes, but bounded | 240 chars max for inspectable local UI. It is display-only, not a corpus. |
| `lexical_fingerprint` | Yes | Space-separated normalized token hashes or short canonical terms for Day 0 lookup. This gives primitive lexical search without storing the full paragraph/thread. |
| `anchor_text` | No | Too easy to turn into a hidden `SearchChunk`. |
| SQLite FTS table | No in PR #68 | FTS is a projection/index. SQLite FTS5 external-content tables require consistency management with triggers/rebuilds; that is PR #69+ complexity, not foundation. Source: https://sqlite.org/fts5.html |

Day 0 lookup can be deliberately primitive:

```text
contextFind("checkpoint migration")
 |
 +-- normalize query terms
 +-- compare against EvidenceAnchor.lexical_fingerprint
 +-- return anchor locator + source observation + source run warning
```

This is weaker than Glean-style search, and that is the point. PR #68 should prove anchors are addressable and minimally findable. It should not smuggle in a retrieval system.

### 4. Permission/freshness correctness

This query is illegal:

```sql
SELECT ea.id, ea.text_preview, so.source_url
FROM evidence_anchors ea
JOIN source_observations so ON so.id = ea.source_observation_id
WHERE ea.lexical_fingerprint LIKE '%launch%';
```

It is illegal because it can leak private source text, cite deleted source state, and produce a false "not found" conclusion from partial data.

Minimum legal shape:

```sql
SELECT ea.id, ea.text_preview, so.source_url, sr.status
FROM evidence_anchors ea
JOIN source_observations so ON so.id = ea.source_observation_id
JOIN source_runs sr ON sr.id = so.source_run_id
WHERE ea.lexical_fingerprint LIKE :query_token
  AND so.is_deleted = false
  AND so.visibility_hash IN (:viewer_visibility_hashes)
  AND so.permission_policy_key IN (:viewer_permission_policy_keys)
  AND sr.status IN ('complete', 'partial')
ORDER BY so.observed_at DESC
LIMIT :first;
```

Rules reviewers should enforce:

```text
Current answer
 |
 +-- must exclude SourceObservation.is_deleted = true
 +-- must filter by visibility_hash and permission_policy_key before returning preview text
 +-- may include SourceRun.status = partial only with a warning
 +-- must not make absence claims when SourceRun.status is partial/failed/rate_limited

Historical answer
 |
 +-- may include deleted observations only if the API explicitly asks for history
 +-- must label them as historical/deleted evidence
```

The dangerous product bug is:

```text
User asks: "why is launch blocked?"
 |
 +-- query finds no Slack anchors
 +-- latest Slack SourceRun for #launch is partial
 |
 `-- illegal answer: "No Slack discussion mentions a blocker."
     legal answer: "No blocker discussion found in observed Slack data; Slack crawl is partial."
```

### 5. PR boundary

Cut PR #68 to the spine plus correctness tests. Anything else is bait.

Keep:

```text
SourceRun
ExternalIdentity
SourceObservation
EvidenceAnchor
Evidence.evidence_anchor_id
required composite indexes
closed enums
small identity resolver
idempotent upsert helpers
tests for uniqueness, partial run semantics, deletion filtering, and permission filtering
```

Cut:

```text
GraphQL contextSearch
GraphQL whyBlocked
SearchDocument / SearchChunk
DocumentSection / DocumentBlock
SQLite FTS5
embeddings / vector DB / RAG
raw payload blobs
ProjectionRun / HealthCheck / scheduler
full permission engine
typed optional target edges for every ontology object
generic graph traversal over target_kind + target_id
```

Approve PR #68 only if:
It implements `SourceRun`, `ExternalIdentity`, `SourceObservation`, `EvidenceAnchor`, and `Evidence.evidence_anchor_id`; keeps `target_kind + target_id` confined to `ExternalIdentity`; adds `external_kind`; adds the composite unique indexes above; replaces broad `anchor_text` with bounded `text_preview` plus `lexical_fingerprint`; and proves by tests that deleted observations, permission-mismatched observations, and partial-run absence claims cannot be served as current truth.

Block PR #68 if:
It adds `SearchChunk`, `DocumentBlock`, FTS, embeddings, GraphQL search/answer APIs, raw payload blobs, direct source identity fields on ontology objects, nullable typed target edges for every object kind, or any query that returns anchor preview text without checking `visibility_hash`, `permission_policy_key`, `SourceRun.status`, and `SourceObservation.is_deleted`.

One migration/index I insist on:
`ExternalIdentity` must add `external_kind` and a unique composite index on `(source_key, source_instance, external_kind, external_id)`. Without that, the Source Evidence Spine cannot survive Jira renames, GitHub repo moves, Slack mirrors, or Google Doc copies without duplicate or corrupted canonical mappings.
