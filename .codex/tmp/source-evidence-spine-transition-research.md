# Cubicle Source Evidence Spine Transition Research

Date: 2026-06-10
Workspace: `/Users/prabhat/workspace/cubicle`
Remote repo: `harshboi/cubicle`
Local branch inspected: `feat/ontology-service-scaffold`
Remote stack head inspected: `origin/feat/ontology-ent-runtime-cardinality`

This is a scratch research note for follow-up review questions. It is intentionally more exhaustive than a clean design doc.

## Executive Summary

The intended foundation after the agent debate is the **Source Evidence Spine**:

```text
typed Ent ontology graph
 |
 +-- existing durable work objects
 |     -> Person, Workstream, Ticket, PullRequest, Document, Message, Evidence, etc.
 |
 +-- existing bounded activity topology
 |     -> Person -> WorkArea -> WorkLens -> WorkLensWindow -> *LensResult -> target
 |
 +-- new PR #68 Source Evidence Spine
       |
       +-- SourceRun
       |     -> source crawl coverage and partial/failure authority
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

The current open stack (#53-#67) is mostly compatible with this direction because it already uses typed Ent schemas and metadata-bearing Through edges. The big changes are:

1. Stop treating `source/source_instance/external_id/source_url` fields on ontology objects and link rows as canonical identity. Move that responsibility to `ExternalIdentity` and `SourceObservation`.
2. Do not let `DocumentFragment` become the canonical cross-source evidence/search unit. Either move it after the Source Evidence Spine or shrink it into a document-specific projection that depends on `EvidenceAnchor`.
3. Stop using `WorkLensWindow.is_complete` / `checkpoint` as source crawl authority. Source crawl coverage must be represented by `SourceRun`.
4. Add `Evidence.evidence_anchor_id` so existing graph claims can point at exact proof without making `Evidence` itself the source observation or text chunk.
5. Keep GraphQL product APIs out of PR #68. PR #68 should be Ent schema/migration/generated code/tests only. GraphQL reads should come in PR #69.

## Sources Inspected

Local and GitHub state:

- `git status --short --branch`
- `git log --oneline --decorate --graph --max-count=18`
- `gh pr list --state open --json number,title,headRefName,baseRefName,url,updatedAt --limit 50`
- `gh pr list --state merged --json number,title,headRefName,baseRefName,url,mergedAt --limit 40`
- `gh pr view 53`, `gh pr view 54` ... `gh pr view 67`
- Remote branch: `origin/feat/ontology-ent-runtime-cardinality`

Files inspected on `origin/feat/ontology-ent-runtime-cardinality`:

- `services/ontology-service/README.md`
- `docs/superpowers/specs/2026-06-09-cardinality-safe-ent-ontology.md`
- `services/ontology-service/ent/schema/*.go`
- `services/ontology-service/internal/ontology/lens_model.go`
- `services/ontology-service/internal/ontologyhooks/lens_hooks.go`
- `services/ontology-service/internal/entstore/entstore.go`
- `services/ontology-service/internal/httpapi/*.go`
- `services/ontology-service/internal/graphql/schema.resolvers.go`
- `services/ontology-service/internal/domain/graph.go`
- `services/ontology-service/internal/graphstore/*.go`
- `services/ontology-service/internal/sampledata/workstream.go`
- `services/ontology-service/internal/config/config.go`
- `services/ontology-service/go.mod`

Debate/moderator files:

- `.codex/agent-instructions/graph-foundations-live-chat.md`
- `.codex/agent-instructions/graph-foundations-meta.md`
- `.codex/agent-instructions/graph-foundations-palantir.md`
- `.codex/agent-instructions/graph-foundations-glean.md`
- `.codex/agent-instructions/graph-foundations-obsidian.md`

External evidence cited in debate:

- Meta TAO paper: https://www.usenix.org/system/files/conference/atc13/atc13-bronson.pdf
- Meta TAO engineering writeup: https://engineering.fb.com/2013/06/25/core-infra/tao-the-power-of-the-graph/
- Meta Unicorn paper: https://www.vldb.org/pvldb/vol6/p1150-curtiss.pdf
- Ent schema edges / Through: https://entgo.io/docs/schema-edges/
- Ent indexes: https://entgo.io/docs/schema-indexes/
- Ent migration: https://entgo.io/docs/migrate/
- Ent GraphQL / entgql: https://entgo.io/docs/graphql/
- Palantir object types: https://palantir.com/docs/foundry/object-link-types/object-types-overview/
- Palantir link types: https://palantir.com/docs/foundry/object-link-types/link-types-overview/
- Palantir data integration datasets: https://palantir.com/docs/foundry/data-integration/datasets/
- Palantir health checks: https://palantir.com/docs/foundry/data-integration/health-checks/
- Glean connectors: https://docs.glean.com/connectors/about
- Glean knowledge graph: https://docs.glean.com/security/knowledge-graph
- Glean indexing/permissions: https://developers.glean.com/api-info/indexing/documents/permissions
- Obsidian links/blocks: https://obsidian.md/help/links
- Obsidian graph view: https://obsidian.md/help/plugins/graph
- SQLite FTS5: https://sqlite.org/fts5.html

## Current Local State

The local checkout is not on the remote stack head:

```text
local branch
 |
 +-- feat/ontology-service-scaffold
 |     -> behind origin/feat/ontology-service-scaffold by 2
 |
 +-- local modified/untracked files
       -> .codex/skills/cubicle-explain-visually/SKILL.md
       -> memory.md
       -> .codex/agent-instructions/
       -> docs/superpowers/plans/2026-06-07-ontology-ingestion-foundation.md
```

Important implication:

```text
Do not blindly git pull or checkout branches before preserving local/untracked files.
For review/planning, inspect remote refs and PR metadata directly.
```

The service exists in the remote stack head, not in the current local working tree. The stack head is:

```text
origin/feat/ontology-ent-runtime-cardinality
 |
 +-- services/ontology-service
 +-- Ent generated code
 +-- Ent schemas
 +-- Gin/gqlgen/SQLite/HOCON service
```

## Existing PR Stack

Merged service scaffold PRs:

| PR | Title | Role |
|---:|---|---|
| #35 | Add ontology service scaffold | Creates `services/ontology-service` module. |
| #36 | Add ontology service HTTP API | Adds Gin server shell and health mechanics. |
| #37 | Add ontology service SQLite storage | Adds SQLite local persistence foundation. |
| #38 | Add ontology service GraphQL scaffold | Adds gqlgen shell and health GraphQL. |
| #51 | Add ontology service runtime config | Adds HOCON/env/flag configuration. |

Open Ent stack:

| PR | Branch | Base | Title | Current role |
|---:|---|---|---|---|
| #53 | `feat/ontology-ent-person` | `feat/ontology-service-graphql-config` | Add Ent person ontology foundation | Ent dependencies, generated code, helper fields, vocabulary, `Person`. |
| #54 | `feat/ontology-ent-evidence` | #53 | Add Ent evidence node | Adds `Evidence`. |
| #55 | `feat/ontology-ent-workstream` | #54 | Add Ent workstream node | Adds `Workstream`. |
| #56 | `feat/ontology-ent-ticket` | #55 | Add Ent ticket node | Adds `Ticket` and `WorkstreamTicket`. |
| #57 | `feat/ontology-ent-pull-request` | #56 | Add Ent pull request node | Adds `PullRequest` and `TicketPullRequest`. |
| #58 | `feat/ontology-ent-document` | #57 | Add Ent document nodes | Adds `Document`, `DocumentFragment`, `TicketDocumentFragment`. |
| #59 | `feat/ontology-ent-message` | #58 | Add Ent message node | Adds `Message`, `TicketMessage`. |
| #60 | `feat/ontology-ent-work-surface` | #59 | Add Ent work area node | Adds `WorkArea`; connects `Person -> WorkArea`. |
| #61 | `feat/ontology-ent-work-pane` | #60 | Add Ent work lens ontology | Adds `WorkLens`, lens vocabulary/hooks. |
| #62 | `feat/ontology-ent-document-lens-result` | #61 | Add Ent document lens result | Adds `DocumentLensResult`. |
| #63 | `feat/ontology-ent-pull-request-lens-result` | #62 | Add Ent pull request lens result | Adds `PullRequestLensResult`. |
| #64 | `feat/ontology-ent-ticket-lens-result` | #63 | Add Ent ticket lens result | Adds `TicketLensResult`. |
| #65 | `feat/ontology-ent-message-lens-result` | #64 | Add Ent message lens result | Adds `MessageLensResult`. |
| #66 | `feat/ontology-ent-cardinality-docs` | #65 | Document cardinality-safe Ent ontology | Updates README/spec around current architecture. |
| #67 | `feat/ontology-ent-runtime-cardinality` | #66 | Harden ontology Ent runtime and lens windows | Adds `WorkLensWindow`, runtime Ent migration/hooks, docs. |

## Current Backend Architecture At PR #67

Service stack:

```text
Swift / local client later
 |
 v
Gin localhost server
 |
 +-- GET /healthz
 |     -> process health only
 |
 +-- POST /graphql
 |     -> gqlgen schema currently has only health query
 |
 +-- GET /playground
       -> local GraphQL playground when enabled
 |
 v
Ent runtime
 |
 +-- internal/entstore.Open
 |     -> opens SQLite
 |     -> applies SQLite pragmas
 |     -> creates generated Ent client
 |     -> installs ontology hooks
 |     -> runs Ent migrations
 |
 v
SQLite local DB
```

Current GraphQL schema:

```graphql
schema {
  query: Query
}

type Query {
  health: Health!
}

type Health {
  ok: Boolean!
  service: String!
}
```

Current topology:

```text
Person
 |
 +-- WorkArea(kind: documents/code/tickets/communications)
      |
      +-- WorkLens(kind: documents_created / tickets_owned / etc.)
           |
           +-- WorkLensWindow(kind: recent/time_bucket/source)
                |
                +-- DocumentLensResult     -> Document
                +-- PullRequestLensResult  -> PullRequest
                +-- TicketLensResult       -> Ticket
                +-- MessageLensResult      -> Message
```

Current table groups:

```text
core objects
 |
 +-- persons
 +-- workstreams
 +-- tickets
 +-- pull_requests
 +-- documents
 +-- document_fragments
 +-- messages
 +-- evidences

cardinality control
 |
 +-- work_areas
 +-- work_lenses
 +-- work_lens_windows

execution links
 |
 +-- workstream_tickets
 +-- ticket_pull_requests
 +-- ticket_document_fragments
 +-- ticket_messages

lens result links
 |
 +-- document_lens_results
 +-- pull_request_lens_results
 +-- ticket_lens_results
 +-- message_lens_results
```

Current table count: 19.

## Current Things That Are Good And Should Stay

### Typed Ent object model

Current design already follows the right high-level philosophy:

```text
typed object schemas
 |
 +-- Person
 +-- Workstream
 +-- Ticket
 +-- PullRequest
 +-- Document
 +-- Message
 +-- Evidence
```

This aligns with the Meta/Palantir-informed direction: typed objects and typed relationships, not generic durable `nodes` / `edges` tables.

### Metadata-bearing Through edges

Current link tables use Ent edge schemas and Through edges:

```text
Workstream -> Ticket                  through WorkstreamTicket
Ticket -> PullRequest                 through TicketPullRequest
Ticket -> DocumentFragment            through TicketDocumentFragment
Ticket -> Message                     through TicketMessage
WorkLens -> Document                  through DocumentLensResult
WorkLens -> PullRequest               through PullRequestLensResult
WorkLens -> Ticket                    through TicketLensResult
WorkLens -> Message                   through MessageLensResult
```

This is still the right shape for metadata-bearing relationships like:

```text
Ticket implemented_by PullRequest
Ticket discussed_in Message
Ticket documented_by Evidence/Doc fragment
Workstream contains Ticket
```

### WorkArea / WorkLens / WorkLensWindow cardinality boundary

This also still fits the intended design:

```text
bad
 |
 +-- Person -> every document/ticket/message/PR
      -> high-degree direct fanout

good
 |
 +-- Person -> WorkArea -> WorkLens -> WorkLensWindow -> LensResult -> target
      -> bounded windows, ranked reads, target expansion after filtering
```

Important: `WorkLensWindow` should remain a serving/read partition. It should not become source crawl truth. Source crawl truth belongs to `SourceRun`.

### Ent runtime is real

PR #67 added `internal/entstore.Open`, and service startup now:

```text
serve command
 |
 +-- entstore.Open
 |     -> storage.Open
 |     -> genent.NewClient
 |     -> ontologyhooks.Register
 |     -> client.Schema.Create
 |
 +-- httpapi.NewRouterWithOptions
```

This means PR #68 can be ordinary Ent schema + generated code + tests.

## Current Things That Conflict With Intended State

### Conflict 1: `sourceFields()` are pasted onto canonical ontology rows

Current helper:

```go
func sourceFields() []ent.Field {
  source
  source_instance
  external_id
  source_url
}
```

Current usages:

```text
Person
Workstream
Ticket
PullRequest
Document
DocumentFragment
Message
Evidence
WorkLensWindow
linkFields(...)
```

Why this conflicts:

```text
current
 |
 +-- Ticket.source = jira
 +-- Ticket.external_id = CUB-123
      -> implies source identity lives on canonical object row

intended
 |
 +-- Ticket.ID / Ticket.key is Cubicle-owned canonical identity
 +-- ExternalIdentity(jira, jira_issue, CUB-123) -> Ticket
 +-- SourceObservation(...) records source state and URL
```

Concrete failure modes:

| Failure | Current direct source field problem | Intended solution |
|---|---|---|
| Jira key rename | `Ticket.external_id` must mutate or duplicate. | Old and new `ExternalIdentity` rows point to same `Ticket`. |
| GitHub repo move | `PullRequest.repository` / `external_id` may become stale or conflict. | Old and new source identities preserve move history. |
| Slack mirror/export | Slack timestamp can collide across workspaces/exports. | `ExternalIdentity` uniqueness includes `source_instance` and `external_kind`. |
| Google Doc copy | Same `content_hash` or title does not mean same doc. | Copy gets separate external identity unless resolver links it. |

Recommended change:

```text
PR stack before #68
 |
 +-- remove sourceFields() from canonical ontology objects and link rows
 +-- do not bless source/source_instance/external_id/source_url as identity
 +-- if any source fields remain temporarily, rename/comment them as denormalized display cache only

PR #68
 |
 +-- add ExternalIdentity
 +-- add SourceObservation
```

I think the cleanest route is to revise the open Ent stack before merging #53-#67:

```text
revise #53 fields.go
 |
 +-- delete or stop using sourceFields()
 +-- add comments that canonical object keys are Cubicle keys, not source keys
 |
 +-- leave source-specific user convenience fields only if explicitly non-canonical
       -> e.g. Person.primary_email, github_login can be profile/search hints
       -> but source identity still belongs in ExternalIdentity
```

If the stack has already merged by the time we implement this, then PR #68 should:

```text
compatibility mode
 |
 +-- leave existing columns in place to avoid destructive migration
 +-- update docs/comments: deprecated/non-authoritative source cache fields
 +-- make new ingestion/write helpers write ExternalIdentity/SourceObservation instead
 +-- stop future query code from using object.source/external_id for identity resolution
```

### Conflict 2: `DocumentFragment` is currently a source-specific evidence/search unit

Current PR #58 adds:

```text
Document
 |
 +-- DocumentFragment
      |
      +-- heading
      +-- path
      +-- text
      +-- ordinal
      +-- text_hash
      +-- source fields
      +-- quality fields
```

Current comments say:

```text
DocumentFragment is a searchable section or chunk extracted from a document.
Fragment body text is used as the smallest evidence/search unit.
```

Why this conflicts:

```text
current
 |
 +-- DocumentFragment becomes the first exact evidence/search unit
 +-- exact evidence is document-shaped
 +-- Slack/Jira/GitHub evidence need separate concepts

intended
 |
 +-- EvidenceAnchor is the source-neutral citation unit
 +-- DocumentFragment/DocumentBlock becomes a later projection over anchors
 +-- SearchChunk becomes a later retrieval projection over anchors
```

Concrete failure modes:

| Failure | DocumentFragment-first problem | Source Evidence Spine solution |
|---|---|---|
| Slack thread reply evidence | Does not fit document fragment anatomy. | `EvidenceAnchor(anchor_kind=slack_message/thread_reply)`. |
| Jira issue comment | Needs source comment locator, not document path. | `EvidenceAnchor(anchor_kind=jira_comment)`. |
| GitHub PR review comment | Needs review/comment/line locator. | `EvidenceAnchor(anchor_kind=pr_review_comment)`. |
| Google Doc paragraph | Fits document fragment, but should still use source-neutral anchor first. | `EvidenceAnchor(anchor_kind=doc_span, source_span_key=...)`. |

Recommended change options:

Option A, cleaner because PR #58 is open:

```text
Revise PR #58
 |
 +-- keep Document
 +-- remove DocumentFragment
 +-- remove TicketDocumentFragment
 +-- remove Document -> fragments edge
 +-- update PR body/docs
 |
PR #68
 |
 +-- add EvidenceAnchor
 |
PR #70+
 |
 +-- add DocumentFragment / DocumentBlock as a projection over EvidenceAnchor
```

Option B, lower churn but semantically riskier:

```text
Keep DocumentFragment
 |
 +-- remove/limit full text field
 +-- remove sourceFields()
 +-- rewrite comments:
 |     "document-specific projection over EvidenceAnchor, not canonical evidence"
 +-- add nullable evidence_anchor_id later
 +-- stop calling it the smallest evidence/search unit
```

I prefer Option A if the open stack can still be rebased. It keeps PR #68 clean and avoids document-specific evidence sneaking into the foundation before the source-neutral spine.

### Conflict 3: `Evidence` currently duplicates source observation/citation fields

Current `Evidence` fields:

```text
key
excerpt
text_hash
observed_at
source_updated_at
source/source_instance/external_id/source_url
freshness_state
visibility
confidence
created_at/updated_at
```

Why this conflicts:

```text
current
 |
 +-- Evidence has excerpt/text/source/freshness itself
      -> Evidence becomes source observation + anchor + graph claim all at once

intended
 |
 +-- SourceObservation owns observed source item state
 +-- EvidenceAnchor owns exact citation/span and preview/hash
 +-- Evidence owns graph-claim support and points to EvidenceAnchor
```

Recommended change:

```text
PR #54
 |
 +-- keep Evidence as a graph-claim/citation envelope
 +-- remove sourceFields() if possible
 +-- avoid making excerpt/full text canonical
 +-- keep confidence only if it means graph-claim confidence, not source permission

PR #68
 |
 +-- add Evidence.evidence_anchor_id optional edge
 +-- add edge from Evidence -> EvidenceAnchor
 +-- migrate docs so latest_evidence_id on link rows ultimately hydrates to EvidenceAnchor
```

Long-term query path:

```text
Workstream
 |
 +-- WorkstreamTicket
 |     |
 |     +-- latest_evidence -> Evidence
 |             |
 |             +-- evidence_anchor -> EvidenceAnchor
 |                     |
 |                     +-- source_observation -> SourceObservation
 |                             |
 |                             +-- source_run -> SourceRun
 |
 +-- Ticket
```

### Conflict 4: `WorkLensWindow` currently carries crawler checkpoint/completeness semantics

Current `WorkLensWindow` fields include:

```text
checkpoint
is_complete
last_indexed_at
source/source_instance/external_id/source_url via sourceFields()
freshness_state/visibility/confidence via qualityFields()
```

Docs currently say `WorkLensWindow` is for paging and crawler checkpoints.

Why this conflicts:

```text
current
 |
 +-- WorkLensWindow can be read as source crawl authority
 +-- partial source crawls may be confused with lens completeness

intended
 |
 +-- SourceRun is the only crawl coverage authority
 +-- WorkLensWindow is a serving/read partition
 +-- WorkLensWindow may be rebuilt from source runs, but cannot say source scope complete
```

Recommended change:

```text
PR #67
 |
 +-- update comments/docs:
 |     WorkLensWindow is a serving/ranking/paging partition, not source-run authority
 |
 +-- consider removing:
 |     checkpoint
 |     is_complete
 |     sourceFields()
 |
 +-- or rename if kept:
       window_checkpoint_cache
       window_result_complete
       last_rebuilt_at
```

If kept temporarily:

```text
Rule
 |
 +-- WorkLensWindow.is_complete can only mean "this materialized result window is complete"
 +-- it must not mean "the source system scope has been fully crawled"
 +-- absence claims must inspect SourceRun.status, not WorkLensWindow.is_complete
```

### Conflict 5: `qualityFields()` are ambiguous authority

Current helper:

```text
freshness_state
visibility
confidence
```

Current usage:

```text
objects
links
lens windows
lens result rows
evidence
```

This is not automatically wrong. The intended model allows derived serving freshness on ontology rows. But the current comments imply these fields are the permission/freshness truth.

Required semantic change:

```text
Ontology object/link quality fields
 |
 +-- allowed as serving caches / denormalized labels
 +-- not allowed as source crawl coverage truth
 +-- not allowed as source ACL authority
 +-- not enough for returning evidence text

SourceObservation / SourceRun
 |
 +-- required authority for evidence reads and absence claims
```

Recommended rename or comment change:

```text
freshness_state
 |
 +-- "Derived serving freshness state; source-run coverage remains in SourceRun."

visibility
 |
 +-- "Derived coarse display hint; evidence text reads must check SourceObservation.permission_policy_key and visibility_hash."
```

### Conflict 6: old `internal/domain` and `internal/graphstore` are generic graph leftovers

Current packages:

```text
internal/domain
 |
 +-- ObjectType
 +-- ObjectRef
 +-- Object
 +-- AssociationType
 +-- Association
 +-- ExpandRequest
 +-- Neighborhood

internal/graphstore
 |
 +-- Expander
 +-- Writer
 +-- MemoryStore
 +-- BFS Expand over generic object/association maps
```

These were useful for PR #36/#38 scaffolding and fake sample data. They conflict with the intended storage/query foundation if they remain product API truth.

Current runtime after #67 does not appear to use `graphstore` in `httpapi` or GraphQL. That is good.

Recommended change:

```text
PR #68 or small cleanup before PR #68
 |
 +-- keep graphstore only as test/sample scaffolding if still needed
 +-- do not expose generic graph expansion as product GraphQL
 +-- do not implement Source Evidence Spine by writing into graphstore.MemoryStore
 +-- route future product query code through Ent/repository helpers
```

Potential cleanup PR:

```text
chore/ontology-remove-generic-graphstore-product-contract
 |
 +-- move fake Flink sample data into tests/testdata or internal/sampledata only
 +-- remove graphstore references from README/product docs
 +-- document that typed Ent schemas are the canonical backend
```

## Intended Source Evidence Spine Details

### PR #68 scope

PR #68 should be schema, generated Ent code, validation, and tests only.

Allowed:

```text
SourceRun
ExternalIdentity
SourceObservation
EvidenceAnchor
Evidence.evidence_anchor_id
generated Ent code
composite indexes
enum constants
idempotent upsert helpers
schema/repository tests
README/docs explaining the spine
```

Cut:

```text
GraphQL contextSearch
GraphQL whyBlocked
SearchDocument / SearchChunk
DocumentSection / DocumentBlock
SQLite FTS5
embeddings/vector DB/RAG
raw source payload blobs
scheduler/retry/health platform
generic traversal over target_kind/target_id
```

### New tables

```text
SourceRun
 |
 +-- run_key                    // connector idempotency key
 +-- source_key                 // slack/jira/github/google_docs
 +-- source_instance            // workspace/repo/site/account namespace
 +-- scope_kind                 // channel/project/repo/document/folder/etc.
 +-- scope_key                  // source-local scope id
 +-- status                     // running/complete/partial/failed/rate_limited
 +-- started_at
 +-- completed_at
 +-- coverage_start_at
 +-- coverage_end_at
 +-- checkpoint_token
 +-- error_code
 +-- error_message

ExternalIdentity
 |
 +-- target_kind                // Cubicle object kind
 +-- target_id                  // Cubicle object id
 +-- source_key
 +-- source_instance
 +-- external_kind              // jira_issue/slack_message/google_doc/github_pr/etc.
 +-- external_id
 +-- identity_status            // active/alias/retired/merged/deleted
 +-- first_seen_at
 +-- last_seen_at
 +-- replaced_by_identity_id

SourceObservation
 |
 +-- source_run_id
 +-- external_identity_id
 +-- observed_kind
 +-- observed_at
 +-- source_updated_at
 +-- is_deleted
 +-- deleted_at
 +-- permission_policy_key
 +-- visibility_hash
 +-- source_url
 +-- content_hash

EvidenceAnchor
 |
 +-- source_observation_id
 +-- anchor_kind                // doc_span/slack_message/jira_comment/pr_review_comment/etc.
 +-- anchor_locator             // source-local paragraph/comment/message locator
 +-- source_span_key            // normalized source span identity for dedupe
 +-- ordinal
 +-- text_hash
 +-- text_preview               // bounded display snippet, max 512 chars
 +-- text_preview_truncated
 +-- lexical_fingerprint        // optional bounded primitive lookup fingerprint
```

Existing schema hook:

```text
Evidence
 |
 +-- evidence_anchor_id
```

### Required indexes

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

### `target_kind + target_id` rule

Accepted:

```text
ExternalIdentity.target_kind + target_id
 |
 +-- provenance pointer
 +-- validated by repository/helper
 +-- not a graph traversal surface
```

Rejected in PR #68:

```text
nullable typed edges to every target table
 |
 +-- ticket_id
 +-- pull_request_id
 +-- document_id
 +-- message_id
 +-- person_id
 +-- workstream_id
```

Reason:

```text
Strict typed edges are cleaner long-term but too much codegen/schema churn for every source identity row in PR #68.
For PR #68, target_kind/target_id is acceptable because it is not the ontology graph itself.
```

Validation required:

```text
known target kinds
 |
 +-- person
 +-- workstream
 +-- ticket
 +-- pull_request
 +-- document
 +-- message
 +-- evidence? maybe not unless needed

repository helper
 |
 +-- rejects unknown target_kind
 +-- optionally verifies target_id exists for known kinds
 +-- tests cover unknown kind and missing target
```

### Anchor text rule

Do not use:

```text
anchor_text
full fragment body
raw source payload blob
SQLite FTS5
SearchChunk
```

Use:

```text
text_hash
text_preview <= 512 chars
text_preview_truncated bool
lexical_fingerprint optional bounded normalized token/string
```

Reason:

```text
unbounded anchor_text
 |
 +-- leaks source content
 +-- bloats SQLite
 +-- recreates SearchChunk under another name
 +-- pushes FTS/index consistency into PR #68
```

Day 0 lookup can be deliberately primitive:

```text
contextFindDebug("checkpoint migration")
 |
 +-- normalize query terms
 +-- compare against lexical_fingerprint / text_preview
 +-- return anchor with SourceObservation and SourceRun status
```

But this belongs in PR #69, not PR #68.

## PR-By-PR Change Plan

### PR #53: Ent person ontology foundation

Current role:

```text
adds Ent dependencies/generated code
adds schema helpers fields.go
adds vocabulary.go
adds Person
adds internal/ontology/lens_model.go
```

Needed changes:

```text
fields.go
 |
 +-- remove or stop using sourceFields()
 +-- reword qualityFields() as derived serving metadata, not source authority
 +-- consider moving freshness/visibility constants to names that clarify derived/cache semantics

Person
 |
 +-- keep display_name and primary_email as profile/search fields
 +-- think hard before keeping github_login/jira_account_id/slack_user_id/google_account_id
 |     -> these are source identities in disguise
 |     -> acceptable only as denormalized profile hints, not canonical source identity
 +-- remove sourceFields() usage

vocabulary
 |
 +-- add target/external kind constants if PR #68 will reuse ontology vocabulary
 +-- avoid putting SourceRun/ExternalIdentity constants in random packages
```

Risk:

```text
Because fields.go is used by every later PR, changing it here causes generated Ent churn through #67.
Still better here than adding compatibility debt later.
```

### PR #54: Evidence

Current role:

```text
adds Evidence with excerpt/text_hash/observed_at/source_updated_at/sourceFields/qualityFields
```

Needed changes:

```text
Evidence
 |
 +-- stop being source observation + evidence span all at once
 +-- remove sourceFields() if stack is revised
 +-- consider removing source_updated_at; SourceObservation owns that
 +-- keep text_hash only if it is claim/evidence hash, not source item hash
 +-- add evidence_anchor_id in PR #68, not in #54 unless restacked
```

Better semantic comment:

```text
Evidence is a graph-claim support row. It can point to exact source proof through EvidenceAnchor.
It is not the raw source item, not the source crawl run, and not the source identity mapping.
```

### PR #55: Workstream

Current role:

```text
adds Workstream object with status/title and source/quality fields
```

Needed changes:

```text
Workstream
 |
 +-- remove sourceFields()
 +-- source-backed workstream identity should be ExternalIdentity if source system has one
 +-- workstream may often be Cubicle-native aggregate, so do not force source identity
 +-- keep status/title/summary as ontology fields
```

### PR #56: Ticket and WorkstreamTicket

Current role:

```text
adds Ticket
adds WorkstreamTicket link
linkFields adds source/freshness/evidence metadata
```

Needed changes:

```text
Ticket
 |
 +-- remove sourceFields()
 +-- Jira issue key/id moves to ExternalIdentity(external_kind=jira_issue)
 +-- source_updated/deleted/visibility moves to SourceObservation

WorkstreamTicket
 |
 +-- keep typed Through edge
 +-- keep latest_evidence_id, evidence_count
 +-- remove sourceFields from linkFields()
 +-- freshness_state/visibility only derived if retained
```

Question to resolve:

```text
Should link rows also get ExternalIdentity?
 |
 +-- usually no; source identity maps source items to canonical objects
 +-- relationship source/proof should be Evidence -> EvidenceAnchor -> SourceObservation
 +-- if a source has relationship IDs, add later, not PR #68
```

### PR #57: PullRequest and TicketPullRequest

Current role:

```text
adds PullRequest(repository, number, title, state, merged_at)
adds TicketPullRequest
```

Needed changes:

```text
PullRequest
 |
 +-- repository and number can remain as display/query fields but are not source identity authority
 +-- GitHub stable node IDs / repo move identities belong in ExternalIdentity
 +-- remove sourceFields()

TicketPullRequest
 |
 +-- keep as typed implementation link
 +-- evidence path should eventually resolve through EvidenceAnchor
 +-- remove sourceFields from linkFields()
```

Repo move case:

```text
old-org/repo#123 -> ExternalIdentity(status=retired/replaced)
new-org/repo#123 -> ExternalIdentity(status=active)
both point to same PullRequest if resolver confirms same provider object
```

### PR #58: Document, DocumentFragment, TicketDocumentFragment

Current role:

```text
adds Document
adds DocumentFragment as chunk/search/evidence unit
adds TicketDocumentFragment
```

Needed changes:

Preferred:

```text
Revise PR #58 to add only Document.
 |
 +-- remove DocumentFragment
 +-- remove TicketDocumentFragment
 +-- remove Ticket -> document_fragments edge
 +-- update docs/counts
```

Then:

```text
PR #68
 |
 +-- EvidenceAnchor handles doc spans source-neutrally

later PR
 |
 +-- DocumentFragment or DocumentBlock as doc-specific projection over EvidenceAnchor
```

If not removing:

```text
DocumentFragment
 |
 +-- remove full text
 +-- remove sourceFields()
 +-- add future evidence_anchor_id or source_observation_id
 +-- comment as document projection, not canonical evidence
```

I strongly prefer removing/postponing DocumentFragment because it is the current largest drift from the Source Evidence Spine ruling.

### PR #59: Message and TicketMessage

Current role:

```text
adds Message(body/channel_key/thread_key/author/sent_at)
adds TicketMessage
```

Needed changes:

```text
Message
 |
 +-- channel/thread/sent_at are source-local metadata; okay as display/filter fields
 +-- source identity must be ExternalIdentity(external_kind=slack_message/email_message/etc.)
 +-- body can be useful, but exact citeable snippet should be EvidenceAnchor
 +-- consider limiting body or treating it as source-derived object text, not citation authority
 +-- remove sourceFields()

TicketMessage
 |
 +-- keep typed discussion link
 +-- remove sourceFields from linkFields()
 +-- latest_evidence -> Evidence -> EvidenceAnchor
```

Slack partial crawl:

```text
Do not infer "no discussion exists" from missing Message rows.
Must inspect SourceRun(status=partial/failed/rate_limited).
```

### PR #60: WorkArea

Current role:

```text
adds WorkArea under Person
```

Needed changes:

```text
WorkArea
 |
 +-- largely fine
 +-- qualityFields() are derived UI/ranking hints if retained
 +-- no source identity should be present
```

### PR #61: WorkLens

Current role:

```text
adds WorkLens with kind/target/rollups/is_complete/last_indexed_at and hooks
```

Needed changes:

```text
WorkLens
 |
 +-- keep as cardinality-control node
 +-- is_complete must not mean source completeness
 +-- if kept, define as materialized lens completeness only
 +-- last_indexed_at is local materialization time, not source freshness
 +-- no source identity
```

Possible rename:

```text
is_complete -> materialization_complete
last_indexed_at -> last_rebuilt_at
```

This can wait if comments are clear, but names may confuse reviewers.

### PR #62-#65: Lens result rows

Current role:

```text
DocumentLensResult
PullRequestLensResult
TicketLensResult
MessageLensResult
```

Needed changes:

```text
all *LensResult
 |
 +-- keep work_lens_id / work_lens_window_id / target_id
 +-- keep relation_kind
 +-- keep latest_evidence_id / evidence_count
 +-- keep rank_score/event_count/last_activity_at
 +-- remove sourceFields from linkFields()
 +-- freshness/visibility only derived if retained
```

Query rule:

```text
LensResult.freshness_state can help sorting/filtering.
It cannot authorize serving evidence text.
Evidence text/preview reads must go through EvidenceAnchor -> SourceObservation -> SourceRun.
```

### PR #66: Cardinality docs

Current role:

```text
documents current 19-table topology
documents source fields on lens results
documents WorkLensWindow crawler checkpoints
```

Needed changes:

```text
docs
 |
 +-- update table count if DocumentFragment/TicketDocumentFragment move later
 +-- update architecture with Source Evidence Spine as next foundation
 +-- remove language that sourceFields on result rows are authoritative
 +-- remove or qualify WorkLensWindow crawler checkpoint language
 +-- add diagram showing Evidence -> EvidenceAnchor -> SourceObservation -> SourceRun
```

### PR #67: Ent runtime and WorkLensWindow

Current role:

```text
adds Ent runtime startup
adds WorkLensWindow
adds hooks that result window belongs to same lens
updates docs
```

Needed changes:

```text
entstore
 |
 +-- keep

WorkLensWindow
 |
 +-- keep as read partition
 +-- remove sourceFields()
 +-- re-evaluate checkpoint/is_complete
 +-- update comments to avoid source-run authority

README/docs
 |
 +-- update current topology to distinguish serving windows vs SourceRun authority
```

## Proposed New PR Stack After #67

Assuming #53-#67 are revised or merged with compatibility comments:

```text
#68 Source Evidence Spine
 |
 +-- SourceRun
 +-- ExternalIdentity
 +-- SourceObservation
 +-- EvidenceAnchor
 +-- Evidence.evidence_anchor_id
 +-- generated Ent code
 +-- indexes/tests/upsert helpers
 |
 +-- no GraphQL product APIs
 +-- no SearchChunk
 +-- no DocumentBlock
 +-- no FTS

#69 Spine read helpers / minimal GraphQL reads
 |
 +-- resolveExternalIdentity(...)
 +-- sourceCoverage(...)
 +-- evidenceAnchors(...)
 +-- contextFindDebug(...) maybe, only primitive lexical_fingerprint
 +-- inject Ent client into GraphQL Resolver
 +-- legal query helper enforces visibility/deleted/source-run filters

#70 Source-backed demo seed / fixtures
 |
 +-- partial Slack crawl fixture
 +-- Jira rename/merge/delete fixture
 +-- unlinked Google Doc paragraph fixture
 +-- launch blocker fixture
 +-- no fake Flink naming unless explicitly test fixture

#71 Document projection
 |
 +-- DocumentFragment / DocumentBlock over EvidenceAnchor
 +-- document-specific navigation
 +-- not canonical cross-source evidence

#72 Retrieval projection
 |
 +-- SearchDocument/SearchChunk or FTS projection over EvidenceAnchor
 +-- permission/freshness-aware
 +-- no vector/LLM yet
```

If we want smaller PRs:

```text
#68a SourceRun + ExternalIdentity
#68b SourceObservation
#68c EvidenceAnchor + Evidence.evidence_anchor_id
```

But the debate ruling prefers one PR #68 with all four because each table alone is insufficient:

```text
SourceRun without SourceObservation
 |
 `-- knows crawl partiality but not item state

ExternalIdentity without SourceObservation
 |
 `-- knows source aliases but not observed/deleted/fresh content

SourceObservation without SourceRun
 |
 `-- knows items seen but not coverage/partiality

EvidenceAnchor without SourceObservation
 |
 `-- has locators but no source state/permission/freshness authority
```

## Current vs Intended Architecture Diagram

```text
Current PR #67                         Intended PR #68+
 |                                      |
 +-- typed Ent graph                    +-- typed Ent graph
 |   -> good foundation                 |   -> remains canonical ontology
 |
 +-- sourceFields on objects/links      +-- ExternalIdentity
 |   -> identity confusion              |   -> source alias/rename/copy history
 |
 +-- qualityFields on objects/links     +-- SourceObservation
 |   -> ambiguous authority             |   -> observed item state/visibility/deletion
 |
 +-- WorkLensWindow checkpoint          +-- SourceRun
 |   -> serving window vs crawl mixed    |   -> crawl coverage authority
 |
 +-- Evidence excerpt/text/source       +-- Evidence -> EvidenceAnchor
 |   -> citation/source mixed            |   -> graph claim points to exact proof
 |
 +-- DocumentFragment text              +-- Document projection later
     -> doc-specific chunk first            -> source-neutral anchor first
```

## Query Correctness Rules

Any query returning evidence preview/text must join through the spine:

```text
EvidenceAnchor
 |
 +-- SourceObservation
 |     -> is_deleted = false
 |     -> permission_policy_key is allowed
 |     -> visibility_hash is allowed
 |
 +-- SourceRun
       -> status in complete/partial
       -> partial returns warning
```

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

Absence claim rule:

```text
if latest relevant SourceRun.status in partial/failed/rate_limited
 |
 `-- query may say "not found in observed data"
     query may not say "does not exist"
```

Current answer rule:

```text
SourceObservation.is_deleted = true
 |
 `-- exclude from current evidence answers
     allow only in explicit history/provenance queries
```

Permission rule:

```text
localhost/no-auth POC
 |
 +-- still use explicit local_dev_open permission policy
 +-- do not skip the shape of permission filtering
 +-- future Swift clients should already see the API shape that includes visibility warnings
```

## Required Tests For PR #68

Schema/index tests:

```text
SourceRun table exists with:
 |
 +-- run_key
 +-- source_key
 +-- source_instance
 +-- scope_kind
 +-- scope_key
 +-- status
 +-- checkpoint_token
 +-- coverage_start_at / coverage_end_at

ExternalIdentity table exists with:
 |
 +-- target_kind
 +-- target_id
 +-- external_kind
 +-- external_id
 +-- identity_status
 +-- replaced_by_identity_id

SourceObservation table exists with:
 |
 +-- source_run_id
 +-- external_identity_id
 +-- observed_kind
 +-- is_deleted
 +-- permission_policy_key
 +-- visibility_hash
 +-- content_hash

EvidenceAnchor table exists with:
 |
 +-- source_observation_id
 +-- anchor_kind
 +-- anchor_locator
 +-- source_span_key
 +-- text_hash
 +-- text_preview
 +-- lexical_fingerprint
```

Uniqueness tests:

```text
duplicate SourceRun(source_key, source_instance, run_key) fails
duplicate ExternalIdentity(source_key, source_instance, external_kind, external_id) fails
duplicate SourceObservation(source_run_id, external_identity_id) fails
duplicate EvidenceAnchor(source_observation_id, anchor_kind, source_span_key, text_hash) fails
```

Semantics tests:

```text
partial SourceRun does not tombstone missing source items
deleted SourceObservation is excluded from current evidence helper
unknown target_kind is rejected by helper/hook
Evidence can optionally point to EvidenceAnchor
EvidenceAnchor helper refuses to return preview without SourceObservation + SourceRun checks
```

## Required Docs Updates

Update `services/ontology-service/README.md`:

```text
Current Architecture
 |
 +-- Gin/gqlgen/Ent/SQLite remains
 +-- typed ontology remains
 +-- Source Evidence Spine added below/alongside typed graph
```

Add diagram:

```text
whyBlocked(workstream)
 |
 +-- Workstream -> WorkstreamTicket -> Ticket
 |     -> typed bounded graph
 |
 +-- WorkstreamTicket.latest_evidence -> Evidence
 |     -> graph claim support
 |
 +-- Evidence.evidence_anchor -> EvidenceAnchor
 |     -> exact citation
 |
 +-- EvidenceAnchor.source_observation -> SourceObservation
 |     -> source item state/permission/deletion
 |
 +-- SourceObservation.source_run -> SourceRun
       -> crawl coverage and partial warning
```

Update `docs/superpowers/specs/2026-06-09-cardinality-safe-ent-ontology.md`:

```text
Add Source Evidence Spine section.
Clarify WorkLensWindow is not source-run authority.
Clarify source identity is ExternalIdentity.
Clarify DocumentFragment is deferred/projection if kept.
Update table count.
Update PR stack.
```

Update PR descriptions after rebasing:

```text
#53
 |
 +-- Ent typed object foundation; source identity deferred to ExternalIdentity

#54
 |
 +-- Evidence graph-claim support; exact source anchors deferred to PR #68

#58
 |
 +-- if DocumentFragment removed, title/body must not claim fragment evidence

#66/#67
 |
 +-- docs must stop saying source fields/checkpoints are authoritative
```

## Implementation Shape For PR #68

Files likely created/changed:

```text
services/ontology-service/ent/schema/source_run.go
services/ontology-service/ent/schema/external_identity.go
services/ontology-service/ent/schema/source_observation.go
services/ontology-service/ent/schema/evidence_anchor.go
services/ontology-service/ent/schema/evidence.go
services/ontology-service/ent/schema/vocabulary.go
services/ontology-service/internal/ontology/source_model.go
services/ontology-service/internal/ontology/source_model_test.go
services/ontology-service/internal/ontologyhooks/source_hooks.go
services/ontology-service/internal/ontologyhooks/source_hooks_test.go
services/ontology-service/internal/entstore/source_spine_test.go
services/ontology-service/ent/ontology_schema_test.go
services/ontology-service/README.md
docs/superpowers/specs/2026-06-09-cardinality-safe-ent-ontology.md
```

Generated files:

```text
services/ontology-service/ent/*
services/ontology-service/ent/sourcerun/*
services/ontology-service/ent/externalidentity/*
services/ontology-service/ent/sourceobservation/*
services/ontology-service/ent/evidenceanchor/*
services/ontology-service/ent/migrate/schema.go
services/ontology-service/ent/mutation.go
...
```

Possible enum constants:

```text
SourceRunStatusRunning = "running"
SourceRunStatusComplete = "complete"
SourceRunStatusPartial = "partial"
SourceRunStatusFailed = "failed"
SourceRunStatusRateLimited = "rate_limited"

IdentityStatusActive = "active"
IdentityStatusAlias = "alias"
IdentityStatusRetired = "retired"
IdentityStatusMerged = "merged"
IdentityStatusDeleted = "deleted"

AnchorKindDocSpan = "doc_span"
AnchorKindSlackMessage = "slack_message"
AnchorKindSlackThreadReply = "slack_thread_reply"
AnchorKindJiraComment = "jira_comment"
AnchorKindTicketDescription = "ticket_description"
AnchorKindPRReviewComment = "pr_review_comment"
AnchorKindPRComment = "pr_comment"
```

Potential source/external kind values:

```text
source_key
 |
 +-- jira
 +-- github
 +-- slack
 +-- google_docs
 +-- google_drive
 +-- markdown
 +-- fixture

external_kind
 |
 +-- jira_issue
 +-- jira_comment
 +-- github_pull_request
 +-- github_issue
 +-- github_pr_review_comment
 +-- github_pr_comment
 +-- slack_message
 +-- slack_thread
 +-- google_doc
 +-- google_doc_comment
 +-- google_doc_span
```

Need to decide if `source_key` and `external_kind` should be closed enums or strings:

```text
closed enum
 |
 +-- safer reviews
 +-- more codegen churn when connectors added

string with validation helper
 |
 +-- easier POC iteration
 +-- must test non-empty and document known values
```

For Day 0, I would use strings for `source_key`, `source_instance`, `scope_kind`, `scope_key`, `external_kind`, and `external_id`, with closed enum only for status fields. Reason: connector vocabulary will evolve quickly, but statuses need stable semantics.

## Open Questions To Ask Before Coding

1. Should PR #58 be rewritten to remove `DocumentFragment`, or should it be kept but redefined as a future projection?
2. Should `sourceFields()` be removed from the open stack before merge, or kept as deprecated cache columns for compatibility?
3. Should `qualityFields()` stay on ontology rows as derived serving metadata, or should it move entirely to SourceObservation/SourceRun first?
4. Should `Person.github_login`, `Person.jira_account_id`, etc. remain profile hints, or move entirely to ExternalIdentity?
5. Should `ExternalIdentity.target_kind/target_id` support `Person` in PR #68, or only work objects first?
6. Should `EvidenceAnchor` point only to `SourceObservation`, or also optionally to `target_kind/target_id`? The final ruling did not require direct target fields on anchors if `Evidence` links graph claims to anchors.
7. Should `Evidence.evidence_anchor_id` be one-to-one latest anchor only, or should `EvidenceAnchor` have many evidence links later?
8. Should `SourceObservation.external_identity_id` be required? Final ruling says required. This means unresolved source items need placeholder ExternalIdentity rows before observation insert.
9. Should raw payload refs be fully excluded from PR #68? Final ruling says no raw source payload storage. A future `payload_ref` can come with snapshot/replay design.
10. Should PR #69 include `contextFindDebug`, or should it only include identity/source/evidence reads and no lexical lookup?

## My Recommended Execution Path

Best clean path if open PRs can still be rebased:

```text
Step 1
 |
 +-- revise #53 helper fields
 |     -> remove sourceFields from canonical object schemas
 |     -> clarify qualityFields as derived if kept
 |
 +-- regenerate Ent through #67

Step 2
 |
 +-- revise #58
 |     -> remove/postpone DocumentFragment and TicketDocumentFragment
 |     -> keep Document only
 |
 +-- update downstream docs/table counts

Step 3
 |
 +-- revise #67 WorkLensWindow
 |     -> remove/rename checkpoint/is_complete if possible
 |     -> at least update comments so SourceRun is future authority

Step 4
 |
 +-- create #68 on top of #67
 |     -> SourceRun
 |     -> ExternalIdentity
 |     -> SourceObservation
 |     -> EvidenceAnchor
 |     -> Evidence.evidence_anchor_id
 |     -> tests

Step 5
 |
 +-- create #69
 |     -> GraphQL/resolver injection of Ent client
 |     -> resolveExternalIdentity/sourceCoverage/evidenceAnchors
 |     -> optional contextFindDebug with no FTS

Step 6
 |
 +-- create fixture/demo PR
 |     -> partial Slack crawl
 |     -> Jira rename/delete
 |     -> Google Doc paragraph
 |     -> launch blocker answer
```

Less clean but lower stack-churn path if #53-#67 must remain close to current:

```text
Step 1
 |
 +-- merge or leave #53-#67 mostly as-is
 +-- update docs/comments to mark direct source fields as non-authoritative

Step 2
 |
 +-- PR #68 adds Source Evidence Spine
 +-- all new writers prefer ExternalIdentity/SourceObservation/EvidenceAnchor

Step 3
 |
 +-- cleanup PR removes/ignores sourceFields and DocumentFragment-as-evidence semantics over time
```

I do not prefer this because the first implementation questions will keep getting muddied by old source fields and document fragments.

## Reviewer Notes For Future Questions

When reviewing any proposed change, ask:

```text
Identity
 |
 +-- Is this Cubicle canonical identity or source identity?
 +-- If source identity, why is it not ExternalIdentity?

Freshness
 |
 +-- Is this item freshness or source crawl coverage?
 +-- If source crawl coverage, why is it not SourceRun?

Evidence
 |
 +-- Can this answer cite exact proof?
 +-- If exact proof, why is it not EvidenceAnchor?

Search
 |
 +-- Is this retrieval/indexing?
 +-- If yes, why is it in PR #68?

Cardinality
 |
 +-- Can this create a hot parent or unbounded traversal?
 +-- If person activity, why is it not behind WorkLensWindow?

Permissions
 |
 +-- Could this return source text without checking permission/freshness?
 +-- If yes, block it.
```

## Short Answer: What Needs To Change

```text
Existing architecture
 |
 +-- keep typed Ent graph
 +-- keep WorkArea/WorkLens/WorkLensWindow cardinality model
 +-- keep Gin/gqlgen/SQLite/HOCON/Ent runtime
 |
 +-- change source identity handling
 |     -> remove/deprecate sourceFields on canonical rows
 |     -> add ExternalIdentity
 |
 +-- change freshness/provenance handling
 |     -> SourceRun for crawl coverage
 |     -> SourceObservation for observed item state
 |
 +-- change evidence handling
 |     -> Evidence points to EvidenceAnchor
 |     -> EvidenceAnchor points to SourceObservation
 |
 +-- change document fragment handling
 |     -> do not make DocumentFragment the first canonical evidence/search unit
 |     -> move it after Source Evidence Spine or redefine as projection
 |
 +-- change PR boundary
       -> PR #68 is schema/tests only; APIs/search/docs blocks later
```
