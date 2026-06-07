# Go Ent Graph POC Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Build a barebones Go service and query layer that ingests a realistic engineering-work dataset into an Ent-backed SQLite graph and answers product-shaped execution questions.

**Architecture:** Create a standalone Go module under `services/cubicle-graph/`. The service owns `graph.db`, Ent schemas, raw snapshots, ingestion, graph traversal, deterministic read-only action candidates, and localhost HTTP/CLI queries. The durable graph is a Meta/TAO-inspired object-association store over Ent and SQLite. The graph follows a Glean-style context contract for source, freshness, visibility, and provenance, while using a Palantir-style operational ontology for typed execution objects, typed links, and read-only actions.

**Tech Stack:** Go 1.25.1 local toolchain, Ent, SQLite, standard `net/http` `ServeMux`, `log/slog`, `go test`, synthetic JSON fixtures, optional Apache Flink public-data import.

---

## Apache Flink Slice

An **Apache Flink slice** means a bounded subset of public Flink project data, not the entire Flink ecosystem.

Use one narrow project area, one time window, and a small set of sources:

```text
Apache Flink slice
 |
 +-- project area: Flink Kubernetes Operator / Autoscaler
 |
 +-- Jira time window: updated since 2023-01-01
 |
 +-- GitHub PR time window: updated since 2025-06-01
 |
 +-- Jira: FLINK issues for that area
 |
 +-- GitHub: PRs mentioning FLINK-#### IDs
 |
 +-- Docs: flink-kubernetes-operator docs and release notes
 |
 +-- Mailing lists / Slack archive: messages mentioning those issue IDs
```

This gives Cubicle enough cross-source evidence to test graph traversal without building a huge crawler. Flink is attractive because the Flink community page documents Jira, mailing lists, public Slack archives, and GitHub mirrors for Flink repositories.

Slack is not part of the live public crawl. Use synthetic Slack-shaped data, user-provided exports, or archive links only. The public Flink crawl uses Jira, GitHub, docs, and optional Pony Mail.

For POC order:

```text
1. Synthetic data with known ground truth
2. Flink slice import
3. Compare graph answers against ground truth and real public evidence
```

## Scope

In scope:

- A new Go module: `services/cubicle-graph/`.
- Ent schemas for project execution graph objects, document fragments, source permissions, source snapshots, connector state, evidence, metadata-rich associations, search index state, embedding artifacts, and read-only action candidates.
- A synthetic workplace dataset generator.
- A raw snapshot and replay model for source-backed imports.
- A fixture ingester that writes Ent rows into SQLite.
- TAO-style `AssociationStore` primitives for object lookup, association lists, counts, expansion, and evidence tracing.
- Query layer for decision traces, launch readiness, blockers, owners, search, document traces, and graph neighborhoods.
- Deterministic read-only action candidates such as ask owner, request review, update docs, and record decision.
- A minimal HTTP service for query endpoints.
- CLI commands for ingesting and querying without Swift.
- Optional Flink importer skeleton that can ingest public Jira/GitHub/docs data later.

Out of scope for this POC:

- Swift app changes.
- OAuth or enterprise auth.
- Production deployment.
- Full Slack connector.
- Full Jira connector.
- Runtime vector search. The schema may store embedding metadata and references, but V0 search is lexical/object/graph search.
- LLM extraction.
- Production action writeback.

## Cross-Cutting Operational Gates

The wider research pass changed the backend bar. The POC is still barebones, but these constraints must shape schema and tests from the first implementation:

```text
connector state
 |
 +-- each source stores cursor/watermark, crawl_started_at, partial state, and resume data
 +-- page-level snapshots are persisted before normalization
 +-- deletions, expired cursors, missing remote links, and retention gaps create tombstone/stale events

permissions
 |
 +-- localhost skips auth, but every query-facing object carries source visibility metadata
 +-- search and graph answers must be permission-filterable before Swift integration
 +-- aliases keep source-specific identity confidence instead of rewriting people blindly

search
 |
 +-- exact object lookup, lexical evidence search, and graph traversal are separate lanes
 +-- FTS rows are indexed through SearchIndexState and cannot outlive their owner object
 +-- vector artifacts are metadata only; fragments remain the evidence unit

storage
 |
 +-- SQLite is local-disk only for V0
 +-- enable WAL, busy timeout, short write transactions, and bounded read queries
 +-- ingestion uses a single writer path; query handlers use read-only transactions
 +-- no SQLite-specific semantics leak outside storage/search modules

ontology
 |
 +-- every object/link/action type must support readiness, trace, blocker, owner, decision, search, or action-candidate queries
 +-- action candidates stay read-only, deterministic, and evidence-backed
```

These gates come from practical pain points in Slack ingestion, Google Docs/Drive ingestion, Jira/GitHub pagination and rate limits, Ent schema evolution, SQLite concurrency, and enterprise graph/search permission models.

## Implementation Detail Decisions

Use these choices unless the codebase gives a stronger reason during implementation:

```text
HTTP
 |
 +-- use net/http ServeMux for V0 routes
 +-- use small local middleware for request ID, logging, timeout, and JSON errors
 +-- do not add chi until routing/middleware complexity proves it is needed

SQLite / Ent
 |
 +-- open storage through internal/storage only
 +-- configure PRAGMA foreign_keys=ON, journal_mode=WAL, busy_timeout, and synchronous=NORMAL
 +-- route all writes through one transaction helper / ingestion path
 +-- map Ent rows to domain DTOs inside transactions; do not leak transaction-bound Ent entities
 +-- use SQLite backup API or controlled checkpoint for DB copies; do not copy graph.db alone while WAL is active
 +-- use Ent auto migration in early POC only
 +-- add Atlas versioned migrations before any shared or long-lived user database

snapshots and idempotency
 |
 +-- live fetch writes source snapshots before normalization
 +-- snapshots include source URL, request params, response headers, body hash, fetched_at, and crawl_run_id
 +-- snapshots stay local and backend-private; shared/exported snapshots require a redaction step
 +-- normalized objects/edges carry source external ID, version/revision, snapshot hash, and mapper_version
 +-- provider events dedupe by provider_event_key when present, otherwise by stable composite source key + payload hash

FTS / search
 |
 +-- FTS table is not authoritative
 +-- every search hit revalidates owner object, freshness_state, visibility, and evidence pointer
 +-- SearchIndexState tracks indexed_hash, mapper_version, indexed_at, and needs_reindex
 +-- exact object search, lexical FTS, and graph-neighborhood search are returned as separate result lanes

graph edges
 |
 +-- GraphEdge is an explicit entity, not a thin Ent M2M edge
 +-- edge metadata includes confidence, visibility, source refs, source event time, observed time, freshness_state, and association_sort_key
 +-- reverse edges are materialized only with derived_from_edge_key and in the same transaction as the forward edge
 +-- Expand requires depth, per-node limit, predicate filter, cursor, and visited-edge dedupe
```

### Validation Matrix

Each implementation slice must include a fixture or test for the invariant it introduces:

| Invariant | Required Validation |
|---|---|
| Live source never normalizes before snapshot write | fake source server test asserts snapshot exists before mapper runs |
| Replay is idempotent | run same snapshot twice and assert object/edge counts do not change |
| Mapper changes are traceable | mapper_version change creates updated SearchIndexState/remap result |
| Duplicate provider events are ignored | duplicate event fixture creates one source event and one graph fact |
| 429/secondary limits stop source cleanly | fake source returns 429 with Retry-After; connector marks partial with resume cursor |
| Invalid cursor does not corrupt state | Drive/Jira-style invalid cursor fixture sets full_rescan_required |
| Hidden/permission-lost object is not searchable | search fixture includes hidden fragment and expects no result |
| Stale FTS row cannot become evidence | delete/update fixture leaves stale FTS row; query drops it and marks needs_reindex |
| Thread replies are not lost | Slack-shaped fixture maps root and replies with thread_root_key |
| Jira changelog is lazy but available | selected ticket fixture maps IssueEvent transitions |
| GitHub PR comments and review comments are distinct | PR fixture maps IssueComment, Review, and ReviewComment separately |
| Reverse edges stay consistent | forward edge insert creates derived reverse edge in same transaction |
| High-degree graph expansion is bounded | synthetic high-degree project returns limited page with cursor |
| SQLite settings are applied | storage test reads PRAGMA foreign_keys, journal_mode, busy_timeout |
| Swift contract is backend-private | HTTP tests assert DTO shape and no Ent/SQLite fields leak |

## Product Hardening Addendum

The next research pass lives in [2026-06-07-graph-product-hardening-research.md](../specs/2026-06-07-graph-product-hardening-research.md). Its implementation impact is:

```text
source health
 |
 +-- add ConnectorCapability manifest per source
 +-- /v1/sources exposes capability, freshness, counts, partial state, stale count, hidden count, and last error
 +-- source gaps are answer states, not internal logs

permissions
 |
 +-- represent access facts as principal -> relation -> object tuples
 +-- keep ACL fields exact-match, never FTS-tokenized
 +-- permission results include acl_snapshot_key and observed_at
 +-- hidden_by_policy beats source ACL

product answers
 |
 +-- every answer returns answer_run_id, source_status_summary, freshness_summary, evidence refs, confidence, no_answer_reason, and action candidates
 +-- generated prose remains out of scope until per-claim citation and retrieval evals exist

graph model
 |
 +-- add Conflict as a future object for contradictory source facts
 +-- add manual_assertion edges for user-corrected facts without overwriting source evidence
 +-- edge types require a named product query and lifecycle owner

search/eval
 |
 +-- track exact hit rate, MRR, nDCG, context precision, context recall, citation coverage, no-answer correctness, and permission-safe recall
 +-- exact keys/paths/PR refs use exact lanes; FTS and future vector lanes explain match reason separately

scale/ops
 |
 +-- add synthetic scale fixture: high-degree project, 10k fragments, 100k edges
 +-- define promotion trigger for Postgres/search service before SQLite becomes a hidden bottleneck
 +-- snapshots are source of truth; normalized DB can be rebuilt from snapshots plus mapper version
```

Additional hardening tests:

| Invariant | Required Validation |
|---|---|
| Connector capability is visible | `/v1/sources` returns permission model, cursor type, delete support, freshness SLA |
| Source gap changes answer state | query with missing Slack/docs source returns evidence gap, not confident answer |
| Permission tuples filter search | tuple fixture allows one principal and denies another |
| Policy-hidden object is stronger than ACL | `hidden_by_policy` fixture is never returned |
| Search ranking is measurable | golden queries produce exact hit rate, MRR, nDCG, and source diversity metrics |
| Contradictory facts are not flattened | conflicting owner/status fixture creates `Conflict` or unresolved state |
| User correction preserves provenance | manual assertion adds evidence path and does not overwrite source edge |
| High-degree graph stays bounded | 100k-edge fixture respects depth, edge budget, cursor, and latency target |
| DB rebuild works | delete normalized DB, replay snapshots, and compare eval output |
| Product DTO is stable | HTTP golden responses include answer_run_id and no backend internals |

## Target Shape

```text
services/cubicle-graph/
 |
 +-- cmd/cubicle-graph/
 |     -> CLI entry point: serve, ingest-synthetic, query
 |
 +-- ent/schema/
 |     -> Ent graph schema
 |
 +-- internal/domain/
 |     -> product-level graph structs and query outputs
 |
 +-- internal/synthetic/
 |     -> deterministic fake workplace dataset
 |
 +-- internal/ingest/
 |     -> maps synthetic/public source records and snapshots into Ent
 |
 +-- internal/snapshot/
 |     -> writes and replays raw source snapshots
 |
 +-- internal/graphstore/
 |     -> Meta/TAO-style AssociationStore primitives over Ent
 |
 +-- internal/ontology/
 |     -> object/link/action type registry for product queries
 |
 +-- internal/query/
 |     -> graph traversal and product query logic
 |
 +-- internal/search/
 |     -> object search and SQLite FTS evidence search
 |
 +-- internal/actions/
 |     -> deterministic read-only action candidate rules
 |
 +-- internal/httpapi/
 |     -> localhost JSON API
 |
 +-- internal/store/
 |     -> Ent client opening, migrations, transaction helper
 |
 +-- internal/sources/
 |     +-- jira/      -> Jira fetcher and snapshot mapper
 |     +-- github/    -> GitHub fetcher and snapshot mapper
 |     +-- docs/      -> markdown docs fetcher and fragmenter
 |     +-- ponymail/  -> optional mailing-list fetcher
 |     +-- gdocs/     -> future Google Drive/Docs snapshot mapper
 |
 +-- testdata/synthetic/atlas/
       -> generated fixture and ground truth
```

## Initial Graph Objects

Use typed Ent schemas for stable concepts:

```text
Person
PersonAlias
ActorAlias
Team
Project
Component
Ticket
PullRequest
CodeFile
Document
DocumentRevision
DocumentTab
DocumentFragment
DocumentSummary
Message
Decision
Risk
Blocker
ActionCandidate
Conflict
ConnectorState
ConnectorCapability
SourceSnapshot
SourceEvent
ProviderEvent
SourceError
SourcePermission
Evidence
IssueEvent
Attachment
EmbeddingArtifact
SearchIndexState
OntologyObjectType
OntologyLinkType
OntologyActionType
GraphEdge
ImportCheckpoint
```

Every queryable object must expose these identity fields unless the type has a documented reason not to:

```text
kind
key
source
external_id
source_url
observed_at
source_updated_at
visibility
freshness_state
```

Key examples:

```text
ticket:ATLAS-42
ticket:FLINK-39743
pr:apache/flink-kubernetes-operator#1127
doc:gdrive:<file_id>
doc:github:apache/flink-kubernetes-operator:docs/content/docs/autoscaler.md
fragment:RFC-7#rev-1/default/0003
person:canonical:<hash-or-slug>
```

Alias rows, not canonical-key rewrites, handle source-specific identities. For example, Jira assignee, GitHub login, Slack user ID, and email hash can all point to one `Person`.

Actor aliases must be source scoped:

```text
source
tenant/site/workspace/repo scope
external actor ID
display name/login/email hash
actor_kind = person | bot | app | group | unknown
confidence
observed_at
```

Do not merge a bot/app/group into a person unless a later rule has explicit evidence.

Use `GraphEdge` for inferred or still-evolving relations. Every edge must carry source, evidence, confidence, visibility, observed time, and freshness metadata:

```text
ticket depends_on ticket          evidence_key=...
ticket blocked_by blocker         evidence_key=...
blocker owned_by person           evidence_key=...
decision resolves blocker         evidence_key=...
risk threatens project            evidence_key=...
pull_request implements ticket     evidence_key=...
document supports decision         evidence_key=...
message evidences blocker          evidence_key=...
ticket needs_action action         evidence_key=...
document has_fragment fragment     evidence_key=...
fragment supports decision         evidence_key=...
document visible_to principal      evidence_key=...
fragment embedded_as embedding     evidence_key=...
```

This is the main Glean/Palantir correction from the adversarial review: do not build a thin graph of `from_ref -> relation -> to_ref`. Build an execution graph where every query-facing fact is explainable, fresh-or-stale, and actionable.

## Graph Serving Contract

The Go service is the graph-serving layer. Ent and SQLite own persistence, but product code should call a narrow object-association API:

```text
GetObject(kind, key)
ListAssociations(fromKind, fromKey, predicate, cursor, limit)
CountAssociations(fromKind, fromKey, predicate)
Expand(startNode, predicates, depth, limits)
Intersect(leftNodes, rightNodes, predicate)
TraceEvidence(edgeKey or evidenceKey, cursor, limit)
```

This is the Cubicle version of the Meta TAO lesson: keep graph access shaped around objects, association lists, association counts, and evidence. Do not let readiness/search/action code grow one-off Ent traversals that cannot be cached, paginated, tested, or served to Swift consistently.

This layer is not a replacement for Ent codegen. Ent owns typed CRUD, generated predicates, migrations, and transactions. `AssociationStore` owns only Cubicle graph semantics that Ent cannot infer: polymorphic `kind + key` references, edge-list pagination, metadata enforcement, evidence tracing, and future cache compatibility.

Evidence tracing must be paginated. Query DTOs include the first small evidence page plus a cursor when more evidence exists.

## Document And Search Contract

Google Docs and GitHub markdown docs use the same normalized shape:

```text
Document
  -> DocumentRevision
  -> DocumentTab
  -> DocumentFragment
  -> Evidence
```

A whole-document vector is not enough. V0 stores searchable fragments and optional `EmbeddingArtifact` metadata for fragments or generated summaries. SQLite FTS5 backs lexical evidence search first. Vector retrieval and RAG can be added after graph correctness is proven.

## Query Contract

The POC succeeds if these commands return evidence-backed answers:

```bash
go run ./cmd/cubicle-graph query readiness --project atlas
go run ./cmd/cubicle-graph query ticket-trace --ticket ATLAS-42
go run ./cmd/cubicle-graph query blockers --project atlas
go run ./cmd/cubicle-graph query decision-gaps --project atlas
go run ./cmd/cubicle-graph query action-candidates --project atlas
go run ./cmd/cubicle-graph query neighborhood --node ticket:ATLAS-42
go run ./cmd/cubicle-graph search --q "ATLAS-42 rollout decision" --project atlas
go run ./cmd/cubicle-graph query document-trace --document RFC-7
```

The HTTP layer should expose the same concepts:

```text
GET /healthz
GET /v1/projects/{projectKey}/readiness
GET /v1/tickets/{ticketKey}/trace
GET /v1/projects/{projectKey}/blockers
GET /v1/projects/{projectKey}/decision-gaps
GET /v1/projects/{projectKey}/action-candidates
GET /v1/search?q=ATLAS-42+rollout+decision&project=atlas
GET /v1/documents/{documentKey}/trace
GET /v1/graph/neighborhood?node=ticket:ATLAS-42
```

Before Swift integration, the backend must answer three product gates:

```text
1. Can project Atlas launch, and what blocks it?
2. What is the full trace for ticket ATLAS-42?
3. Where is the rollout decision documented, and what evidence supports it?
```

Each gate must return:

```text
answer status
source status by connector
freshness state
evidence refs with source URLs and snapshot refs
confidence
related action candidates when applicable
explicit no-answer when evidence is missing
```

## Task 1: Scaffold Go Module

**Files:**

- Create: `services/cubicle-graph/go.mod`
- Create: `services/cubicle-graph/cmd/cubicle-graph/main.go`
- Create: `services/cubicle-graph/internal/config/config.go`
- Create: `services/cubicle-graph/internal/config/config_test.go`
- Create: `services/cubicle-graph/internal/observability/logging.go`

- [ ] **Step 1: Initialize module**

Run:

```bash
cd /Users/prabhat/workspace/cubicle
mkdir -p services/cubicle-graph
cd services/cubicle-graph
go mod init cubicle/services/cubicle-graph
go test ./...
```

Expected:

```text
go: creating new go.mod
go: warning: "./..." matched no packages
```

- [ ] **Step 2: Add config test**

Create `services/cubicle-graph/internal/config/config_test.go`:

```go
package config

import "testing"

func TestLoadDefaults(t *testing.T) {
	cfg := Load(map[string]string{})
	if cfg.ListenAddr != "127.0.0.1:0" {
		t.Fatalf("ListenAddr = %q", cfg.ListenAddr)
	}
	if cfg.DatabaseURL != "file:graph.db?_fk=1" {
		t.Fatalf("DatabaseURL = %q", cfg.DatabaseURL)
	}
}

func TestLoadEnvOverrides(t *testing.T) {
	cfg := Load(map[string]string{
		"CUBICLE_GRAPH_LISTEN_ADDR": "127.0.0.1:48080",
		"CUBICLE_GRAPH_DATABASE_URL": "file:/tmp/cubicle-graph.db?_fk=1",
	})
	if cfg.ListenAddr != "127.0.0.1:48080" {
		t.Fatalf("ListenAddr = %q", cfg.ListenAddr)
	}
	if cfg.DatabaseURL != "file:/tmp/cubicle-graph.db?_fk=1" {
		t.Fatalf("DatabaseURL = %q", cfg.DatabaseURL)
	}
}
```

- [ ] **Step 3: Add config implementation**

Create `services/cubicle-graph/internal/config/config.go`:

```go
package config

type Config struct {
	ListenAddr  string
	DatabaseURL string
}

func Load(env map[string]string) Config {
	cfg := Config{
		ListenAddr:  "127.0.0.1:0",
		DatabaseURL: "file:graph.db?_fk=1",
	}
	if v := env["CUBICLE_GRAPH_LISTEN_ADDR"]; v != "" {
		cfg.ListenAddr = v
	}
	if v := env["CUBICLE_GRAPH_DATABASE_URL"]; v != "" {
		cfg.DatabaseURL = v
	}
	return cfg
}
```

- [ ] **Step 4: Add logger helper**

Create `services/cubicle-graph/internal/observability/logging.go`:

```go
package observability

import (
	"log/slog"
	"os"
)

func NewLogger() *slog.Logger {
	return slog.New(slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{}))
}
```

- [ ] **Step 5: Add CLI version command with real JSON output**

Create `services/cubicle-graph/cmd/cubicle-graph/main.go`:

```go
package main

import (
	"encoding/json"
	"fmt"
	"os"
)

func main() {
	cmd := "help"
	if len(os.Args) > 1 {
		cmd = os.Args[1]
	}

	switch cmd {
	case "version":
		_ = json.NewEncoder(os.Stdout).Encode(map[string]string{
			"service": "cubicle-graph",
			"version": "poc",
		})
	default:
		fmt.Fprintln(os.Stderr, "usage: cubicle-graph [version|serve|ingest-synthetic|query]")
		os.Exit(2)
	}
}
```

- [ ] **Step 6: Verify**

Run:

```bash
cd /Users/prabhat/workspace/cubicle/services/cubicle-graph
go test ./...
go run ./cmd/cubicle-graph version
```

Expected:

```text
ok  	cubicle/services/cubicle-graph/internal/config
{"service":"cubicle-graph","version":"poc"}
```

## Task 2: Add Ent And SQLite Store

**Files:**

- Create: `services/cubicle-graph/ent/generate.go`
- Create: `services/cubicle-graph/ent/schema/person.go`
- Create: `services/cubicle-graph/ent/schema/personalias.go`
- Create: `services/cubicle-graph/ent/schema/actoralias.go`
- Create: `services/cubicle-graph/ent/schema/team.go`
- Create: `services/cubicle-graph/ent/schema/project.go`
- Create: `services/cubicle-graph/ent/schema/component.go`
- Create: `services/cubicle-graph/ent/schema/ticket.go`
- Create: `services/cubicle-graph/ent/schema/pullrequest.go`
- Create: `services/cubicle-graph/ent/schema/codefile.go`
- Create: `services/cubicle-graph/ent/schema/document.go`
- Create: `services/cubicle-graph/ent/schema/documentrevision.go`
- Create: `services/cubicle-graph/ent/schema/documenttab.go`
- Create: `services/cubicle-graph/ent/schema/documentfragment.go`
- Create: `services/cubicle-graph/ent/schema/documentsummary.go`
- Create: `services/cubicle-graph/ent/schema/message.go`
- Create: `services/cubicle-graph/ent/schema/decision.go`
- Create: `services/cubicle-graph/ent/schema/risk.go`
- Create: `services/cubicle-graph/ent/schema/blocker.go`
- Create: `services/cubicle-graph/ent/schema/actioncandidate.go`
- Create: `services/cubicle-graph/ent/schema/conflict.go`
- Create: `services/cubicle-graph/ent/schema/connectorstate.go`
- Create: `services/cubicle-graph/ent/schema/connectorcapability.go`
- Create: `services/cubicle-graph/ent/schema/sourcesnapshot.go`
- Create: `services/cubicle-graph/ent/schema/sourceevent.go`
- Create: `services/cubicle-graph/ent/schema/providerevent.go`
- Create: `services/cubicle-graph/ent/schema/sourceerror.go`
- Create: `services/cubicle-graph/ent/schema/sourcepermission.go`
- Create: `services/cubicle-graph/ent/schema/evidence.go`
- Create: `services/cubicle-graph/ent/schema/issueevent.go`
- Create: `services/cubicle-graph/ent/schema/attachment.go`
- Create: `services/cubicle-graph/ent/schema/embeddingartifact.go`
- Create: `services/cubicle-graph/ent/schema/searchindexstate.go`
- Create: `services/cubicle-graph/ent/schema/ontologyobjecttype.go`
- Create: `services/cubicle-graph/ent/schema/ontologylinktype.go`
- Create: `services/cubicle-graph/ent/schema/ontologyactiontype.go`
- Create: `services/cubicle-graph/ent/schema/graphedge.go`
- Create: `services/cubicle-graph/internal/store/store.go`
- Create: `services/cubicle-graph/internal/store/store_test.go`
- Create: `services/cubicle-graph/internal/store/tx.go`
- Create: `services/cubicle-graph/internal/store/tx_test.go`

- [ ] **Step 1: Install Ent dependencies**

Run:

```bash
cd /Users/prabhat/workspace/cubicle/services/cubicle-graph
go get entgo.io/ent
go get github.com/mattn/go-sqlite3
go get entgo.io/ent/cmd/ent
```

Expected:

```text
go: added entgo.io/ent
```

- [ ] **Step 2: Add Ent generator marker**

Create `services/cubicle-graph/ent/generate.go`:

```go
package ent

//go:generate go run entgo.io/ent/cmd/ent generate ./schema
```

- [ ] **Step 3: Add metadata-rich schema set**

Each schema must include stable natural keys and enough metadata for Glean-style freshness/provenance and Palantir-style operational links. The first implementation can use explicit foreign-key fields plus `GraphEdge` rather than complex typed Ent edges, but the edge itself must not be thin.

Example `services/cubicle-graph/ent/schema/ticket.go`:

```go
package schema

import (
	"entgo.io/ent"
	"entgo.io/ent/schema/field"
	"entgo.io/ent/schema/index"
)

type Ticket struct {
	ent.Schema
}

func (Ticket) Fields() []ent.Field {
	return []ent.Field{
		field.String("source"),
		field.String("external_id"),
		field.String("key"),
		field.String("project_key"),
		field.String("summary"),
		field.String("status"),
		field.String("issue_type").Default(""),
		field.String("priority").Default(""),
		field.String("reporter_key").Default(""),
		field.String("assignee_key").Default(""),
		field.String("source_url").Default(""),
		field.String("visibility").Default("public"),
		field.String("freshness_state").Default("fresh"),
		field.Time("created_at"),
		field.Time("updated_at"),
	}
}

func (Ticket) Indexes() []ent.Index {
	return []ent.Index{
		index.Fields("source", "external_id").Unique(),
		index.Fields("key").Unique(),
		index.Fields("project_key", "status"),
		index.Fields("assignee_key"),
	}
}
```

Example `services/cubicle-graph/ent/schema/evidence.go`:

```go
package schema

import (
	"entgo.io/ent"
	"entgo.io/ent/schema/field"
	"entgo.io/ent/schema/index"
)

type Evidence struct {
	ent.Schema
}

func (Evidence) Fields() []ent.Field {
	return []ent.Field{
		field.String("key").Unique(),
		field.String("source"),
		field.String("source_url").Default(""),
		field.String("source_snapshot_key").Default(""),
		field.String("source_event_key").Default(""),
		field.String("excerpt").Default(""),
		field.Time("observed_at"),
		field.Time("source_updated_at").Optional().Nillable(),
		field.Float("confidence").Default(1.0),
		field.String("visibility").Default("public"),
		field.String("freshness_state").Default("fresh"),
	}
}

func (Evidence) Indexes() []ent.Index {
	return []ent.Index{
		index.Fields("source", "source_snapshot_key"),
		index.Fields("source_event_key"),
		index.Fields("freshness_state"),
	}
}
```

Example `services/cubicle-graph/ent/schema/graphedge.go`:

```go
package schema

import (
	"entgo.io/ent"
	"entgo.io/ent/schema/field"
	"entgo.io/ent/schema/index"
)

type GraphEdge struct {
	ent.Schema
}

func (GraphEdge) Fields() []ent.Field {
	return []ent.Field{
		field.String("from_kind"),
		field.String("from_key"),
		field.String("predicate"),
		field.String("to_kind"),
		field.String("to_key"),
		field.String("source"),
		field.String("source_snapshot_key").Default(""),
		field.String("source_event_key").Default(""),
		field.String("evidence_key").Default(""),
		field.String("derived_from_edge_key").Default(""),
		field.String("mapper_version").Default(""),
		field.Float("confidence").Default(1.0),
		field.String("visibility").Default("public"),
		field.String("freshness_state").Default("fresh"),
		field.String("rule_name").Default(""),
		field.String("status").Default("active"),
		field.String("association_sort_key").Default(""),
		field.Float("rank_score").Default(0),
		field.Time("observed_at"),
		field.Time("source_event_time").Optional().Nillable(),
		field.Time("valid_from").Optional().Nillable(),
		field.Time("valid_to").Optional().Nillable(),
	}
}

func (GraphEdge) Indexes() []ent.Index {
	return []ent.Index{
		index.Fields("from_kind", "from_key", "predicate", "to_kind", "to_key", "evidence_key", "rule_name").Unique(),
		index.Fields("from_kind", "from_key", "predicate"),
		index.Fields("from_kind", "from_key", "predicate", "association_sort_key"),
		index.Fields("to_kind", "to_key", "predicate"),
		index.Fields("derived_from_edge_key"),
		index.Fields("source_snapshot_key"),
		index.Fields("source_event_key"),
		index.Fields("evidence_key"),
		index.Fields("freshness_state"),
	}
}
```

Example `services/cubicle-graph/ent/schema/actioncandidate.go`:

```go
package schema

import (
	"entgo.io/ent"
	"entgo.io/ent/schema/field"
	"entgo.io/ent/schema/index"
)

type ActionCandidate struct {
	ent.Schema
}

func (ActionCandidate) Fields() []ent.Field {
	return []ent.Field{
		field.String("key").Unique(),
		field.String("action_type"),
		field.String("target_kind"),
		field.String("target_key"),
		field.String("owner_key").Default(""),
		field.String("summary"),
		field.String("rationale").Default(""),
		field.String("status").Default("open"),
		field.String("evidence_key").Default(""),
		field.Float("confidence").Default(1.0),
		field.Time("created_at"),
		field.Time("expires_at").Optional().Nillable(),
	}
}

func (ActionCandidate) Indexes() []ent.Index {
	return []ent.Index{
		index.Fields("target_kind", "target_key"),
		index.Fields("action_type", "status"),
		index.Fields("owner_key"),
	}
}
```

Create `ConnectorCapability`, `SourceSnapshot`, `SourceEvent`, `ProviderEvent`, `SourceError`, `ConnectorState`, `SourcePermission`, `IssueEvent`, `Attachment`, `Conflict`, `SearchIndexState`, and `EmbeddingArtifact` with these fields:

```text
ConnectorCapability: source, source_instance, permission_model, cursor_type, supports_incremental, supports_deletes, supports_permission_changes, supports_threads, supports_comments, supports_attachments, freshness_sla_seconds, rate_limit_policy, private_data_policy
SourceSnapshot: key, source, run_id, kind, external_id, source_url, request_url, request_params_json, field_mask, response_headers_json, path, content_hash, fetched_at, source_updated_at, http_status, visibility
SourceEvent: key, source, external_id, provider_event_key, event_type, event_time, observed_at, actor_key, source_snapshot_key, payload_hash, visibility
ProviderEvent: key, source, provider_event_key, event_type, event_time, observed_at, payload_hash, source_snapshot_key, status
SourceError: key, source, run_id, error_kind, status_code, retry_after, cursor_value, message, observed_at
ConnectorState: source, slice, status, last_success_at, last_attempt_at, last_error, freshness_state, cursor_value, gap_started_at, gap_ended_at, full_rescan_required
SourcePermission: source, source_object_kind, source_object_key, principal_kind, principal_key, role, visibility, observed_at, source_updated_at, snapshot_key
IssueEvent: source, issue_key, event_type, actor_key, from_value, to_value, source_event_key, event_time, observed_at
Attachment: key, source, external_id, owner_kind, owner_key, filename, mime_type, byte_size, content_hash, source_url, extraction_state, visibility
Conflict: key, object_kind, object_key, conflict_type, left_evidence_key, right_evidence_key, status, detected_at, resolved_at
SearchIndexState: owner_kind, owner_key, index_name, indexed_hash, mapper_version, indexed_at, status, needs_reindex, error
EmbeddingArtifact: key, owner_kind, owner_key, content_hash, model, dimensions, embedding_ref, generated_at, status
```

These schemas are part of the graph contract, not importer bookkeeping. Query responses use them to disclose source freshness and provenance.

- [ ] **Step 4: Generate Ent code**

Run:

```bash
cd /Users/prabhat/workspace/cubicle/services/cubicle-graph
go generate ./ent
go test ./...
```

Expected:

```text
ok  	cubicle/services/cubicle-graph/internal/config
```

- [ ] **Step 5: Add store opener**

Create `services/cubicle-graph/internal/store/store.go`:

```go
package store

import (
	"context"
	stdsql "database/sql"

	"cubicle/services/cubicle-graph/ent"
	"entgo.io/ent/dialect"
	entsql "entgo.io/ent/dialect/sql"
	_ "github.com/mattn/go-sqlite3"
)

type Store struct {
	Client *ent.Client
}

func Open(ctx context.Context, databaseURL string) (*Store, error) {
	db, err := stdsql.Open("sqlite3", databaseURL)
	if err != nil {
		return nil, err
	}
	db.SetMaxOpenConns(1)
	db.SetMaxIdleConns(1)
	if _, err := db.ExecContext(ctx, "PRAGMA foreign_keys=ON"); err != nil {
		_ = db.Close()
		return nil, err
	}
	if _, err := db.ExecContext(ctx, "PRAGMA journal_mode=WAL"); err != nil {
		_ = db.Close()
		return nil, err
	}
	if _, err := db.ExecContext(ctx, "PRAGMA busy_timeout=5000"); err != nil {
		_ = db.Close()
		return nil, err
	}
	if _, err := db.ExecContext(ctx, "PRAGMA synchronous=NORMAL"); err != nil {
		_ = db.Close()
		return nil, err
	}
	drv := entsql.OpenDB(dialect.SQLite, db)
	client := ent.NewClient(ent.Driver(drv))
	if err := client.Schema.Create(ctx); err != nil {
		_ = client.Close()
		return nil, err
	}
	return &Store{Client: client}, nil
}

func (s *Store) Close() error {
	if s == nil || s.Client == nil {
		return nil
	}
	return s.Client.Close()
}
```

`MaxOpenConns(1)` is acceptable for the local POC because ingestion is single-writer and the HTTP API is localhost. Before multi-user serving, split reader/writer handles or move to Postgres.

Create `services/cubicle-graph/internal/store/tx.go`:

```go
package store

import (
	"context"

	"cubicle/services/cubicle-graph/ent"
)

func (s *Store) WithTx(ctx context.Context, fn func(*ent.Tx) error) error {
	tx, err := s.Client.Tx(ctx)
	if err != nil {
		return err
	}
	if err := fn(tx); err != nil {
		_ = tx.Rollback()
		return err
	}
	return tx.Commit()
}
```

Do not return Ent entities loaded inside `WithTx` unless they are mapped to domain DTOs first.

- [ ] **Step 6: Add store integration test**

Create `services/cubicle-graph/internal/store/store_test.go`:

```go
package store

import (
	"context"
	"testing"
)

func TestOpenCreatesSQLiteSchema(t *testing.T) {
	ctx := context.Background()
	st, err := Open(ctx, "file:ent-store-test?mode=memory&cache=shared&_fk=1")
	if err != nil {
		t.Fatalf("Open() error = %v", err)
	}
	defer st.Close()

	if _, err := st.Client.Project.Create().
		SetKey("atlas").
		SetName("Atlas Launch").
		Save(ctx); err != nil {
		t.Fatalf("create project: %v", err)
	}
}
```

The store tests must also assert:

```text
Open is idempotent
foreign key support is enabled
journal_mode is wal for file-backed databases
busy_timeout is non-zero
context cancellation returns an error without panicking
WithTx rolls back on mapper error
```

- [ ] **Step 7: Verify**

Run:

```bash
cd /Users/prabhat/workspace/cubicle/services/cubicle-graph
CGO_ENABLED=1 go test ./...
```

Expected: all tests pass.

## Task 2A: Add AssociationStore Primitives

**Files:**

- Create: `services/cubicle-graph/internal/graphstore/association_store.go`
- Create: `services/cubicle-graph/internal/graphstore/association_store_test.go`
- Create: `services/cubicle-graph/internal/ontology/registry.go`
- Create: `services/cubicle-graph/internal/ontology/registry_test.go`

- [ ] **Step 1: Define graph primitives**

Create `services/cubicle-graph/internal/graphstore/association_store.go`:

```go
package graphstore

import (
	"context"
	"time"

	"cubicle/services/cubicle-graph/ent"
)

type NodeRef struct {
	Kind string `json:"kind"`
	Key  string `json:"key"`
}

type Association struct {
	From            NodeRef    `json:"from"`
	Predicate       string     `json:"predicate"`
	To              NodeRef    `json:"to"`
	EvidenceKey     string     `json:"evidence_key"`
	Confidence      float64    `json:"confidence"`
	Visibility      string     `json:"visibility"`
	FreshnessState  string     `json:"freshness_state"`
	SortKey         string     `json:"sort_key"`
	RankScore       float64    `json:"rank_score"`
	ObservedAt      time.Time  `json:"observed_at"`
}

type ListOptions struct {
	Cursor string
	Limit  int
}

type EntAssociationStore struct {
	client *ent.Client
}

type AssociationStore interface {
	ListAssociations(ctx context.Context, from NodeRef, predicate string, opts ListOptions) ([]Association, string, error)
	CountAssociations(ctx context.Context, from NodeRef, predicate string) (int, error)
	Expand(ctx context.Context, start NodeRef, predicates []string, depth int, limitPerNode int) ([]Association, error)
	TraceEvidence(ctx context.Context, evidenceKey string) (*ent.Evidence, error)
}

func New(client *ent.Client) *EntAssociationStore {
	return &EntAssociationStore{client: client}
}
```

Implement `EntAssociationStore` with Ent `GraphEdge` queries ordered by `association_sort_key` and `observed_at`. Cursor encoding can be a base64 JSON object containing the last sort key and graph edge ID. `Expand` should breadth-first traverse up to `depth`, cap every node by `limitPerNode`, dedupe repeated edges, and return metadata-rich `Association` values.

Do not add generic object CRUD methods here. Use Ent-generated code for typed object create/update/query. `AssociationStore` is only for polymorphic association lists, counts, expansion, and evidence tracing.

Reverse-edge rule:

```text
materialize inverse edges only when the inverse predicate has product meaning or high query volume
materialized inverse edges must set derived_from_edge_key
create forward and derived reverse edges in the same transaction
otherwise use the GraphEdge reverse index: to_kind, to_key, predicate
```

Edge dedupe key:

```text
from_kind, from_key, predicate, to_kind, to_key, evidence_key, rule_name
```

- [ ] **Step 2: Add ontology registry**

The registry defines allowed product object, link, and action types:

```text
objects: person, team, project, component, ticket, pull_request, code_file, document, document_fragment, message, decision, blocker, risk, action_candidate
links: contains, has_component, implemented_by, changes_file, mentions, blocked_by, owned_by, evidenced_by, discussed_in, supports, visible_to, needs_action, needs_decision
actions: ask_owner, request_review, update_docs, record_decision, mark_stale
```

Tests must fail if query code tries to emit a predicate not in the registry.

- [ ] **Step 3: Test association behavior**

The AssociationStore test must:

```text
open in-memory SQLite
insert project:atlas, ticket:ATLAS-42, blocker:B-17, evidence:E-1
insert graph edges:
  project:atlas contains ticket:ATLAS-42
  ticket:ATLAS-42 blocked_by blocker:B-17
call ListAssociations(project:atlas, contains)
call Expand(project:atlas, contains|blocked_by, depth=2)
call CountAssociations(ticket:ATLAS-42, blocked_by)
call TraceEvidence(E-1)
assert reverse lookup works through the to_kind/to_key index without requiring a duplicate inverse edge
assert association metadata survives the round trip
```

- [ ] **Step 4: Verify**

Run:

```bash
cd /Users/prabhat/workspace/cubicle/services/cubicle-graph
go test ./internal/graphstore ./internal/ontology -v
```

Expected: AssociationStore and ontology tests pass.

## Task 2B: Add Document Fragment And Search Infra

**Files:**

- Create: `services/cubicle-graph/internal/search/search.go`
- Create: `services/cubicle-graph/internal/search/search_test.go`
- Create: `services/cubicle-graph/internal/docmodel/docmodel.go`
- Create: `services/cubicle-graph/internal/docmodel/docmodel_test.go`

- [ ] **Step 1: Define document normalization rules**

Document normalization maps source docs into:

```text
Document: stable file/doc identity
DocumentRevision: content snapshot or exported revision
DocumentTab: Google Docs tab, or "default" for markdown
DocumentFragment: heading/paragraph/list/table fragments with structural path and text hash
SourcePermission: source ACL or public visibility metadata
SearchIndexState: FTS indexing status by owner object
EmbeddingArtifact: optional future vector reference by fragment or summary
```

Rules:

```text
never use one vector as the only representation of a long document
store raw source snapshots before mapping fragments
use fragments as retrieval and evidence units
Google Docs fragment keys include document ID, revision/hash, tab ID, structural path, ordinal, and content hash
Drive/Docs comments and replies are separate future source objects, not body fragments
store summaries as derived artifacts with source_content_hash and prompt_version
store permissions as metadata, not searchable text
mark summaries and embedding artifacts stale when source_content_hash no longer matches
return no-answer when no evidence hit satisfies source, freshness, and visibility filters
```

- [ ] **Step 2: Add lexical search API**

`internal/search` exposes:

```go
type Hit struct {
	Kind           string  `json:"kind"`
	Key            string  `json:"key"`
	Title          string  `json:"title"`
	Snippet        string  `json:"snippet"`
	SourceURL      string  `json:"source_url"`
	Score          float64 `json:"score"`
	Visibility     string  `json:"visibility"`
	FreshnessState string  `json:"freshness_state"`
	EvidenceKey    string  `json:"evidence_key"`
}

type Searcher interface {
	Search(ctx context.Context, req Request) (Response, error)
}

type Request struct {
	Query        string   `json:"query"`
	ProjectKey   string   `json:"project_key"`
	VisibleTo    []string `json:"visible_to"`
	Limit        int      `json:"limit"`
	IncludeStale bool     `json:"include_stale"`
}

type Response struct {
	ExactHits   []Hit `json:"exact_hits"`
	LexicalHits []Hit `json:"lexical_hits"`
	GraphHits   []Hit `json:"graph_hits"`
	NoAnswer    bool  `json:"no_answer"`
}
```

Back V0 with SQLite FTS5 over:

```text
ticket key + summary
PR title + number
document title + path
document fragment text + heading path
message excerpt
evidence excerpt
code file path
```

Implementation rule:

```text
the FTS table stores owner_kind, owner_key, indexed_hash, mapper_version, title, search_text, and evidence_key
configure FTS5 tokenizer so issue keys, PR refs, file paths, and dotted package names remain searchable
FTS rows are never returned directly as evidence
every candidate hit reloads the owner object and verifies visibility, freshness_state, indexed_hash, and evidence_key
permission or content-hash changes mark SearchIndexState.needs_reindex=true
```

- [ ] **Step 3: Test search evidence trace**

The search test must:

```text
insert document RFC-7
insert revision rev-1
insert tab default
insert fragment containing "rollout gate depends on ATLAS-42"
insert evidence E-doc-1 pointing to that fragment
index the fragment in FTS
search "rollout gate ATLAS-42"
assert a document_fragment hit is returned
assert hit evidence_key = E-doc-1
assert hit source_url and freshness_state are non-empty
search exact issue keys ATLAS-42 and FLINK-39743 and assert tokenization returns expected objects
search file paths and PR refs and assert lexical hits are not lost to punctuation tokenization
update the source fragment hash
assert the old summary and embedding artifact become stale
leave a stale FTS row behind and assert search drops it after owner revalidation
insert a hidden fragment and assert it is not returned for a caller without matching visibility
search for an unsupported claim
assert the result reports no evidence instead of fabricating an answer
```

- [ ] **Step 4: Verify**

Run:

```bash
cd /Users/prabhat/workspace/cubicle/services/cubicle-graph
go test ./internal/docmodel ./internal/search -v
```

Expected: document model and search tests pass.

## Task 3: Build Synthetic Dataset Generator

**Files:**

- Create: `services/cubicle-graph/internal/domain/models.go`
- Create: `services/cubicle-graph/internal/synthetic/generator.go`
- Create: `services/cubicle-graph/internal/synthetic/generator_test.go`
- Create: `services/cubicle-graph/testdata/synthetic/README.md`

- [ ] **Step 1: Define source DTOs**

Create domain structs that look like real workplace source data, not Ent rows:

```go
package domain

type SyntheticDataset struct {
	People             []PersonSource            `json:"people"`
	Teams              []TeamSource              `json:"teams"`
	Projects           []ProjectSource           `json:"projects"`
	Tickets            []TicketSource            `json:"tickets"`
	PullRequests       []PullRequestSource       `json:"pull_requests"`
	Documents          []DocumentSource          `json:"documents"`
	DocumentRevisions  []DocumentRevisionSource  `json:"document_revisions"`
	DocumentTabs       []DocumentTabSource       `json:"document_tabs"`
	DocumentFragments  []DocumentFragmentSource  `json:"document_fragments"`
	DocumentSummaries  []DocumentSummarySource   `json:"document_summaries"`
	Messages           []MessageSource           `json:"messages"`
	SourceSnapshots    []SourceSnapshotSource    `json:"source_snapshots"`
	ConnectorStates    []ConnectorStateSource    `json:"connector_states"`
	SourcePermissions  []SourcePermissionSource  `json:"source_permissions"`
	EmbeddingArtifacts []EmbeddingArtifactSource `json:"embedding_artifacts"`
	ActionCandidates   []ActionCandidateSource   `json:"action_candidates"`
	Edges              []GroundTruthEdge         `json:"ground_truth_edges"`
}

type GroundTruthEdge struct {
	FromKind       string  `json:"from_kind"`
	FromKey        string  `json:"from_key"`
	Predicate      string  `json:"predicate"`
	ToKind         string  `json:"to_kind"`
	ToKey          string  `json:"to_key"`
	EvidenceKey    string  `json:"evidence_key"`
	Confidence     float64 `json:"confidence"`
	Visibility     string  `json:"visibility"`
	FreshnessState string  `json:"freshness_state"`
}
```

- [ ] **Step 2: Generate deterministic Atlas project**

The fixture must include:

```text
12 people
3 teams
1 project: atlas
30 tickets
25 PRs
8 documents
8 document revisions
8 document tabs
24 document fragments
4 document summaries
12 source permissions
24 embedding artifact metadata rows
80 Slack-shaped messages
12 source snapshots
4 connector states
5 decisions
5 blockers
5 dependencies
5 risks
8 action candidates
95 ground-truth edges
```

Use stable IDs like:

```text
person:priya
team:platform
project:atlas
ticket:ATLAS-42
pr:184
doc:RFC-7
fragment:RFC-7#default/rollout-gates/0003
message:C-platform-1717000000.000100
decision:D-42
blocker:B-17
```

- [ ] **Step 3: Test graph realism**

The generator test must assert:

```text
ATLAS-42 has at least one blocker edge
ATLAS-42 has at least one PR edge
project:atlas has at least one risk edge
at least one decision is stale
at least one blocker has ambiguous or missing owner evidence
at least one document fragment supports a decision
at least one document permission row has public visibility
at least one document summary records model, prompt_version, and source_content_hash
every embedding artifact points to a fragment or summary and stores model/dimensions/ref
every ground-truth edge has evidence, confidence, visibility, and freshness_state
at least one action candidate asks for a missing owner
at least one connector state is stale or partial
```

- [ ] **Step 4: Verify**

Run:

```bash
cd /Users/prabhat/workspace/cubicle/services/cubicle-graph
go test ./internal/synthetic -v
```

Expected: generator tests pass.

## Task 4: Implement Ingest Service

**Files:**

- Create: `services/cubicle-graph/internal/ingest/ingest.go`
- Create: `services/cubicle-graph/internal/ingest/ingest_test.go`
- Modify: `services/cubicle-graph/cmd/cubicle-graph/main.go`

- [ ] **Step 1: Add ingest API**

Create `IngestSynthetic(ctx, client, dataset)` that:

```text
upserts people
upserts teams
upserts projects
upserts tickets
upserts PRs
upserts documents
upserts document revisions, tabs, fragments, summaries
upserts messages
upserts decisions, blockers, dependencies, risks
upserts source snapshots and connector states
upserts source permissions, search index state, and embedding artifact metadata
upserts evidence rows
upserts graph edges
upserts read-only action candidates
```

Use source DTO keys as stable natural keys. Re-running ingest must not duplicate rows.

- [ ] **Step 2: Add idempotency test**

The test must:

```text
open in-memory SQLite
generate synthetic dataset
ingest once
record counts
ingest again
assert counts do not change
assert ATLAS-42 trace edges exist
assert all query-facing graph edges have non-empty evidence_key
assert all query-facing graph edges have visibility and freshness_state
assert at least one action candidate exists for project:atlas
```

- [ ] **Step 3: Add CLI command**

Add:

```bash
go run ./cmd/cubicle-graph ingest-synthetic --db file:graph.db?_fk=1
```

Expected output:

```json
{"projects":1,"tickets":30,"pull_requests":25,"documents":8,"document_fragments":24,"messages":80,"source_snapshots":12,"action_candidates":8,"edges":95}
```

- [ ] **Step 4: Verify**

Run:

```bash
cd /Users/prabhat/workspace/cubicle/services/cubicle-graph
rm -f graph.db
go run ./cmd/cubicle-graph ingest-synthetic --db 'file:graph.db?_fk=1'
go test ./...
```

Expected: command prints JSON counts and tests pass.

## Task 5: Implement Query Layer

**Files:**

- Create: `services/cubicle-graph/internal/query/models.go`
- Create: `services/cubicle-graph/internal/query/readiness.go`
- Create: `services/cubicle-graph/internal/query/ticket_trace.go`
- Create: `services/cubicle-graph/internal/query/neighborhood.go`
- Create: `services/cubicle-graph/internal/query/decision_gaps.go`
- Create: `services/cubicle-graph/internal/query/document_trace.go`
- Create: `services/cubicle-graph/internal/query/search.go`
- Create: `services/cubicle-graph/internal/actions/actions.go`
- Create: `services/cubicle-graph/internal/actions/actions_test.go`
- Create: `services/cubicle-graph/internal/query/query_test.go`
- Modify: `services/cubicle-graph/cmd/cubicle-graph/main.go`

- [ ] **Step 1: Define query outputs**

The query layer returns product objects, not raw Ent rows:

```go
type EvidenceRef struct {
	Key              string  `json:"key"`
	Source           string  `json:"source"`
	SourceURL        string  `json:"source_url"`
	ObservedAt       string  `json:"observed_at"`
	SourceUpdatedAt  string  `json:"source_updated_at"`
	Confidence       float64 `json:"confidence"`
	Visibility       string  `json:"visibility"`
	FreshnessState   string  `json:"freshness_state"`
	SnapshotRef      string  `json:"snapshot_ref"`
	Preview          string  `json:"preview"`
}

type SourceStatus struct {
	Source         string `json:"source"`
	Status         string `json:"status"`
	FreshnessState string `json:"freshness_state"`
	LastSuccessAt  string `json:"last_success_at"`
	LastError      string `json:"last_error"`
}

type ActionCandidateSummary struct {
	Key        string        `json:"key"`
	ActionType string       `json:"action_type"`
	TargetKind string       `json:"target_kind"`
	TargetKey  string       `json:"target_key"`
	OwnerKey   string       `json:"owner_key"`
	Summary    string       `json:"summary"`
	Rationale  string       `json:"rationale"`
	Confidence float64      `json:"confidence"`
	Evidence   []EvidenceRef `json:"evidence"`
}

type BlockerSummary struct {
	Key      string        `json:"key"`
	Title    string        `json:"title"`
	OwnerKey string        `json:"owner_key"`
	Evidence []EvidenceRef `json:"evidence"`
}

type ReadinessReport struct {
	AnswerGeneratedAt string                   `json:"answer_generated_at"`
	ProjectKey        string                   `json:"project_key"`
	CanLaunch         bool                     `json:"can_launch"`
	SourceStatus      []SourceStatus           `json:"source_status"`
	BlockingReasons   []string                 `json:"blocking_reasons"`
	Blockers           []BlockerSummary         `json:"blockers"`
	OpenDecisions     []string                 `json:"open_decisions"`
	Risks              []string                 `json:"risks"`
	ActionCandidates  []ActionCandidateSummary `json:"action_candidates"`
}

type SearchResult struct {
	Query             string         `json:"query"`
	ProjectKey        string         `json:"project_key"`
	ObjectHits         []SearchHit    `json:"object_hits"`
	EvidenceHits       []SearchHit    `json:"evidence_hits"`
	HasEvidence        bool           `json:"has_evidence"`
	NoAnswerReason     string         `json:"no_answer_reason"`
	SourceStatus       []SourceStatus `json:"source_status"`
	AnswerGeneratedAt  string         `json:"answer_generated_at"`
}

type SearchHit struct {
	Kind           string        `json:"kind"`
	Key            string        `json:"key"`
	Title          string        `json:"title"`
	Snippet        string        `json:"snippet"`
	SourceURL      string        `json:"source_url"`
	Score          float64       `json:"score"`
	Visibility     string        `json:"visibility"`
	FreshnessState string        `json:"freshness_state"`
	Evidence       []EvidenceRef `json:"evidence"`
	Related        []GraphNodeRef `json:"related"`
}

type GraphNodeRef struct {
	Kind string `json:"kind"`
	Key  string `json:"key"`
}

type DocumentTrace struct {
	DocumentKey        string         `json:"document_key"`
	LatestRevisionKey  string         `json:"latest_revision_key"`
	Fragments          []SearchHit    `json:"fragments"`
	MentionedTickets   []GraphNodeRef `json:"mentioned_tickets"`
	SupportedDecisions []GraphNodeRef `json:"supported_decisions"`
	HasEvidence        bool           `json:"has_evidence"`
	NoAnswerReason     string         `json:"no_answer_reason"`
	Evidence           []EvidenceRef  `json:"evidence"`
	SourceStatus       []SourceStatus `json:"source_status"`
}
```

- [ ] **Step 2: Implement readiness traversal**

Use `internal/graphstore.AssociationStore` for polymorphic graph access. The query package should not call `ent.GraphEdge.Query()` directly except in tests. Typed object loading may still use Ent-generated queries through small store/query helpers.

Traversal:

```text
project:atlas
 |
 +-- threatens <- risk
 |
 +-- contains -> ticket
       |
       +-- blocked_by -> blocker
       +-- depends_on -> ticket
       +-- implemented_by -> pull_request
       +-- needs_decision -> decision
```

Rules:

```text
can_launch = false when active blockers exist
can_launch = false when open critical risks exist
can_launch = false when stale required decisions exist
source_status reports stale or partial connector state
action_candidates are generated from deterministic rules and include evidence
```

- [ ] **Step 3: Implement ticket trace**

`ticket-trace` must return:

```text
ticket
linked PRs
blocking blockers
dependencies
decisions
documents
messages
evidence previews
source status for every source used in the trace
action candidates attached to the ticket
```

- [ ] **Step 4: Implement neighborhood query**

`neighborhood --node ticket:ATLAS-42` must return one-hop edges by default and `--depth 2` for two-hop expansion.

- [ ] **Step 5: Implement action candidate query**

`action-candidates --project atlas` must return read-only actions generated from graph facts:

```text
ask_owner when an active ticket has no assignee
request_review when an open PR has no review edge
update_docs when a PR changes a code file linked to a document
record_decision when a blocker or ticket has unresolved decision evidence
```

- [ ] **Step 6: Add query CLI**

Search and document trace are evidence retrieval commands. They do not generate prose answers in V0.

Commands:

```bash
go run ./cmd/cubicle-graph query readiness --project atlas --db 'file:graph.db?_fk=1'
go run ./cmd/cubicle-graph query ticket-trace --ticket ATLAS-42 --db 'file:graph.db?_fk=1'
go run ./cmd/cubicle-graph query neighborhood --node ticket:ATLAS-42 --db 'file:graph.db?_fk=1'
go run ./cmd/cubicle-graph query decision-gaps --project atlas --db 'file:graph.db?_fk=1'
go run ./cmd/cubicle-graph query action-candidates --project atlas --db 'file:graph.db?_fk=1'
go run ./cmd/cubicle-graph search --q 'rollout gate ATLAS-42' --project atlas --db 'file:graph.db?_fk=1'
go run ./cmd/cubicle-graph query document-trace --document RFC-7 --db 'file:graph.db?_fk=1'
```

- [ ] **Step 7: Verify**

Run:

```bash
cd /Users/prabhat/workspace/cubicle/services/cubicle-graph
rm -f graph.db
go run ./cmd/cubicle-graph ingest-synthetic --db 'file:graph.db?_fk=1'
go run ./cmd/cubicle-graph query readiness --project atlas --db 'file:graph.db?_fk=1'
go test ./internal/query ./internal/actions -v
```

Expected:

```text
readiness JSON includes "can_launch": false
readiness JSON includes ATLAS-42 blocker evidence
readiness JSON includes at least one action candidate
search JSON returns a document_fragment hit with evidence
document trace returns RFC-7 fragments and supported decisions
query tests pass
```

## Task 6: Add Localhost HTTP Service

**Files:**

- Create: `services/cubicle-graph/internal/httpapi/router.go`
- Create: `services/cubicle-graph/internal/httpapi/router_test.go`
- Modify: `services/cubicle-graph/cmd/cubicle-graph/main.go`

- [ ] **Step 1: Add router**

Routes:

```text
GET /healthz
GET /v1/projects/{projectKey}/readiness
GET /v1/tickets/{ticketKey}/trace
GET /v1/projects/{projectKey}/blockers
GET /v1/projects/{projectKey}/decision-gaps
GET /v1/projects/{projectKey}/action-candidates
GET /v1/search?q=rollout+gate+ATLAS-42&project=atlas
GET /v1/documents/{documentKey}/trace
GET /v1/graph/neighborhood?node=ticket:ATLAS-42&depth=1
GET /v1/sources
```

Build routes with standard `net/http` `ServeMux`. Add small local helpers for JSON responses, request timeout, request ID, and `slog` fields. Do not expose Ent structs directly; every handler returns versioned DTOs from `internal/domain` or `internal/query`.

- [ ] **Step 2: Add HTTP tests**

Use `httptest` to assert:

```text
/healthz returns 200 and {"ok":true}
/v1/projects/atlas/readiness returns can_launch=false after synthetic ingest
/v1/tickets/ATLAS-42/trace returns evidence refs
/v1/projects/atlas/action-candidates returns evidence-backed actions
/v1/search returns object and evidence hits without generated prose
/v1/documents/RFC-7/trace returns fragments with evidence refs
/v1/sources returns connector freshness, partial state, and last error
responses include source status summary when a query depends on a partial source
responses do not include Ent IDs, SQLite paths, FTS table names, or raw snapshot file paths
unknown ticket returns 404
```

- [ ] **Step 3: Add serve command**

Command:

```bash
go run ./cmd/cubicle-graph serve --db 'file:graph.db?_fk=1' --listen 127.0.0.1:48080
```

Startup output:

```json
{"service":"cubicle-graph","url":"http://127.0.0.1:48080","db":"file:graph.db?_fk=1"}
```

- [ ] **Step 4: Verify**

Run in one terminal:

```bash
cd /Users/prabhat/workspace/cubicle/services/cubicle-graph
go run ./cmd/cubicle-graph serve --db 'file:graph.db?_fk=1' --listen 127.0.0.1:48080
```

Run in another:

```bash
curl -s http://127.0.0.1:48080/healthz
curl -s http://127.0.0.1:48080/v1/projects/atlas/readiness
curl -s http://127.0.0.1:48080/v1/projects/atlas/action-candidates
curl -s 'http://127.0.0.1:48080/v1/search?q=rollout+gate+ATLAS-42&project=atlas'
```

Expected:

```json
{"ok":true}
```

## Task 7: Add Flink Autoscaler Snapshot Importer

**Files:**

- Create: `services/cubicle-graph/internal/snapshot/writer.go`
- Create: `services/cubicle-graph/internal/snapshot/writer_test.go`
- Create: `services/cubicle-graph/internal/snapshot/replay.go`
- Create: `services/cubicle-graph/internal/snapshot/replay_test.go`
- Create: `services/cubicle-graph/internal/flink/crawl_budget.go`
- Create: `services/cubicle-graph/internal/flink/crawl_budget_test.go`
- Create: `services/cubicle-graph/internal/sources/testserver/server.go`
- Create: `services/cubicle-graph/internal/sources/jira/client.go`
- Create: `services/cubicle-graph/internal/sources/jira/remotelinks.go`
- Create: `services/cubicle-graph/internal/sources/github/client.go`
- Create: `services/cubicle-graph/internal/sources/docs/client.go`
- Create: `services/cubicle-graph/internal/sources/docs/fragmenter.go`
- Create: `services/cubicle-graph/internal/sources/ponymail/client.go`
- Create: `services/cubicle-graph/internal/flink/importer.go`
- Create: `services/cubicle-graph/internal/flink/importer_test.go`
- Modify: `services/cubicle-graph/cmd/cubicle-graph/main.go`

- [ ] **Step 1: Define bounded import request**

Use:

```text
project = FLINK
component = Autoscaler
github_repo = apache/flink-kubernetes-operator
jira_since = 2023-01-01T00:00:00Z
github_seed_since = 2025-06-01T00:00:00Z
issue_limit = explicit issue count
pr_detail_limit = explicit PR count
ponymail_issue_limit = explicit issue-key count
crawl_started_at = run watermark used to cap source updated ranges
max_pr_detail_pages = explicit per-PR page cap
max_changed_files = explicit per-PR changed-file cap
```

Default live source bounds:

```text
Jira:
  JQL = project = FLINK AND component = "Autoscaler" AND updated >= "2023-01-01" AND updated <= crawl_started_at
  expected count from 2026-06-07 probe = 156
  page size = 50
  first live run fetches remote links for all imported Autoscaler issues
  incremental runs fetch remote links only for changed issues

GitHub:
  seed query = repo:apache/flink-kubernetes-operator is:pr FLINK- updated:>=2025-06-01 updated:<=crawl_started_at_date
  expected count from 2026-06-07 probe = 158
  only 14 distinct PR-linked FLINK keys intersected the Autoscaler Jira key set in the 2025-06-01 seed probe
  discovery must continue with exact targeted per-Jira-key PR searches without date filters
  Jira remote links are imported before GitHub Search and can directly select PRs
  full detail crawl requires GITHUB_TOKEN

Docs:
  tree = /repos/apache/flink-kubernetes-operator/git/trees/main?recursive=1
  markdown files under docs/content/docs = 28

Pony Mail:
  optional targeted query by issue key
  cap first live run to 25 issue keys

Slack:
  excluded from live crawl
```

The importer writes a crawl budget snapshot before live fetch:

```json
{
  "sources": {
    "jira": {"required": true, "max_requests": 250},
    "github": {"required": true, "max_requests": 1000, "requires_token_for_full_crawl": true},
    "docs": {"required": true, "max_requests": 50},
    "ponymail": {"required": false, "max_requests": 25}
  }
}
```

Budget tests must assert:

```text
full GitHub PR detail crawl without GITHUB_TOKEN is rejected or marked partial
Pony Mail over budget marks only Pony Mail partial
required source over budget marks crawl run partial
fixture mode never touches live budgets
Slack is not a live source
oversized PR details are marked partial instead of exhausting source budget
429 with Retry-After writes SourceError and resume cursor
invalid cursor sets ConnectorState.full_rescan_required
duplicate snapshot replay is idempotent
```

- [ ] **Step 2: Implement offline-first importer**

The first importer must read local JSON snapshots, not crawl live websites during tests:

```text
testdata/flink/snapshots/jira/search-page-000.json
testdata/flink/snapshots/github/pr-1127.json
testdata/flink/snapshots/github/pr-1127-files.json
testdata/flink/snapshots/docs/docs-tree.json
testdata/flink/snapshots/docs/autoscaler.md
testdata/flink/snapshots/ponymail/FLINK-39743-issues.json
testdata/flink/snapshots/crawl-budget.json
testdata/flink/snapshots/errors/github-429.json
testdata/flink/snapshots/errors/jira-invalid-cursor.json
testdata/flink/snapshots/mutations/deleted-remote-link.json
testdata/flink/snapshots/mutations/updated-ticket-status.json
```

This keeps tests stable and proves the replay model before live crawling.

Snapshot writer requirements:

```text
write page snapshot before mapper runs
snapshot key = source/run_id/kind/external_id-or-page/content_hash
include request URL, request params JSON, field mask, response headers JSON, status code, fetched_at, and body hash
write SourceError snapshots for 429/403/404/410/invalid-cursor cases
mapper reads snapshots, not HTTP response objects
mapper upserts by source external ID, version/revision, snapshot hash, and mapper_version
```

- [ ] **Step 3: Add live-fetch command behind explicit flag**

Command:

```bash
go run ./cmd/cubicle-graph ingest-flink \
  --db 'file:graph.db?_fk=1' \
  --component 'Autoscaler' \
  --jira-since '2023-01-01T00:00:00Z' \
  --github-seed-since '2025-06-01T00:00:00Z' \
  --issue-limit 156 \
  --pr-detail-limit 50 \
  --ponymail-issue-limit 25 \
  --max-pr-detail-pages 10 \
  --max-changed-files 300 \
  --snapshot-root '.data/snapshots' \
  --live
```

The command must refuse live crawling unless `--live` is present.

The command must require `GITHUB_TOKEN` when `--pr-detail-limit` is high enough to exceed the unauthenticated REST core budget. Without a token, it may run a capped probe and mark GitHub source status as `partial`.

GitHub importer discovery order:

```text
1. Fetch Jira Autoscaler keys.
2. Fetch Jira remote links and select GitHub PR URLs for Autoscaler keys.
3. Run recent repo-wide GitHub seed search.
4. Keep only PRs whose extracted FLINK keys intersect the Jira Autoscaler key set.
5. Run exact targeted GitHub PR searches for remaining Jira keys, without date filters, until pr_detail_limit is reached.
6. Fetch PR detail/files/issue comments/review comments/reviews/commits only for selected on-slice PRs.
7. Store off-slice seed search pages as snapshots but do not normalize off-slice PR detail into the graph.
```

GitHub detail endpoints:

```text
GET /repos/apache/flink-kubernetes-operator/pulls/{number}
GET /repos/apache/flink-kubernetes-operator/pulls/{number}/files?per_page=100&page=<n>
GET /repos/apache/flink-kubernetes-operator/issues/{number}/comments
GET /repos/apache/flink-kubernetes-operator/pulls/{number}/comments
GET /repos/apache/flink-kubernetes-operator/pulls/{number}/reviews
GET /repos/apache/flink-kubernetes-operator/pulls/{number}/commits
```

Do not use GitHub code search in V0. Use the repo tree and raw markdown/code content for docs and file references.

PR detail caps:

```text
if PR exceeds max_pr_detail_pages or max_changed_files:
  keep PR summary
  mark PR detail freshness_state = partial
  skip lower-value pages first: commits, reviews, review comments
```

Pony Mail behavior:

```text
query dev@flink.apache.org by issue key for human development discussion
query issues@flink.apache.org by issue key for Jira mirror evidence and dedupe
mark issues@ messages as jira_mirror_email
do not bulk-crawl lists
do not use monthly mbox export in V0
cap first live run to 25 issue keys
```

Graph-density gates after live import:

```text
at least 10 on-slice ticket-to-PR traces
at least 5 ticket-to-PR-to-file traces
at least 3 ticket-to-PR-to-doc candidate traces with confidence >= 0.75
at least 3 ticket-to-discussion traces from Jira, PR comments/reviews, or dev@ Pony Mail
FLINK-39743 trace reproduces ticket -> PR -> changed file -> discussion/gap
```

If these fail, do not keep widening blindly. First inspect missing links and remote links. If the slice is genuinely too sparse, widen only to the Flink Kubernetes Operator component.

- [ ] **Step 4: Verify**

Run:

```bash
cd /Users/prabhat/workspace/cubicle/services/cubicle-graph
go test ./internal/flink ./internal/snapshot ./internal/sources/... -v
```

Expected: importer replays fixture snapshots into source objects, graph edges, evidence, connector state, and action candidates, and never touches the network in tests.

## Task 8: Add Evaluation Against Ground Truth

**Files:**

- Create: `services/cubicle-graph/internal/eval/eval.go`
- Create: `services/cubicle-graph/internal/eval/eval_test.go`
- Modify: `services/cubicle-graph/cmd/cubicle-graph/main.go`

- [ ] **Step 1: Define evaluation output**

```go
type EvaluationReport struct {
	ExpectedEdges                  int     `json:"expected_edges"`
	FoundEdges                     int     `json:"found_edges"`
	MissingEdges                   int     `json:"missing_edges"`
	Precision                      float64 `json:"precision"`
	Recall                         float64 `json:"recall"`
	EdgeMetadataCompleteness       float64 `json:"edge_metadata_completeness"`
	AssociationMetadataCompleteness float64 `json:"association_metadata_completeness"`
	DocumentFragmentTraceCoverage   float64 `json:"document_fragment_trace_coverage"`
	ExactObjectSearchRecall         float64 `json:"exact_object_search_recall"`
	LexicalEvidenceSearchRecall     float64 `json:"lexical_evidence_search_recall"`
	MeanReciprocalRank              float64 `json:"mean_reciprocal_rank"`
	NDCG                            float64 `json:"ndcg"`
	CitationCoverage                float64 `json:"citation_coverage"`
	PermissionSafeRecall            float64 `json:"permission_safe_recall"`
	SourceHealthCoverage            float64 `json:"source_health_coverage"`
	ProductGatePassCount            int     `json:"product_gate_pass_count"`
	NoAnswerBehaviorPass            bool    `json:"no_answer_behavior_pass"`
	ActionCandidateEvidenceCoverage float64 `json:"action_candidate_evidence_coverage"`
	SnapshotReplayConsistent        bool    `json:"snapshot_replay_consistent"`
	RebuildFromSnapshotsConsistent  bool    `json:"rebuild_from_snapshots_consistent"`
}
```

- [ ] **Step 2: Compare graph edges to ground truth**

The evaluator loads synthetic `ground_truth_edges` and compares them to Ent `GraphEdge` rows by:

```text
from_kind
from_key
predicate
to_kind
to_key
evidence_key
```

The evaluator must also assert every query-facing edge has:

```text
confidence > 0
visibility is non-empty
freshness_state is fresh, stale, partial, or unknown
observed_at is non-zero
association_sort_key is set when the predicate has ordered lists
document_fragment hits trace to document revision, source URL, and evidence
exact object searches find ATLAS-42, RFC-7, and pr:184
lexical evidence search finds the known rollout-gate fragment
the three product gates pass with evidence
unsupported searches return no-answer with no fabricated evidence
permission filters prevent hidden objects from appearing in search results
tombstone or stale events remove or mark invalid query-facing edges
SearchIndexState catches and repairs stale FTS rows
graph expansion is bounded and uses read-only transactions in query tests
ranking eval reports exact hit rate, MRR, nDCG, citation coverage, and permission-safe recall
source health eval confirms partial/missing sources appear in answer DTOs
rebuild eval deletes normalized rows, replays snapshots, and compares report output
conflicting source facts create unresolved conflict state instead of overwriting each other
```

- [ ] **Step 3: Add CLI**

```bash
go run ./cmd/cubicle-graph eval --db 'file:graph.db?_fk=1'
```

Expected for synthetic ingest:

```json
{"expected_edges":95,"found_edges":95,"missing_edges":0,"precision":1,"recall":1,"edge_metadata_completeness":1,"association_metadata_completeness":1,"document_fragment_trace_coverage":1,"exact_object_search_recall":1,"lexical_evidence_search_recall":1,"mean_reciprocal_rank":1,"ndcg":1,"citation_coverage":1,"permission_safe_recall":1,"source_health_coverage":1,"product_gate_pass_count":3,"no_answer_behavior_pass":true,"action_candidate_evidence_coverage":1,"snapshot_replay_consistent":true,"rebuild_from_snapshots_consistent":true}
```

## Task 9: Documentation And Handoff

**Files:**

- Create: `services/cubicle-graph/README.md`
- Create: `docs/go-ent-graph-poc.md`

- [ ] **Step 1: Add service README**

The README must include:

```text
what the POC does
why Swift is not integrated yet
how to ingest synthetic data
how to run queries
how to run HTTP service
how Apache Flink slice import works
how raw snapshots and replay work
how AssociationStore object/association primitives work
how document revisions, tabs, fragments, permissions, and search are represented
how action candidates are generated
how to reset graph.db
```

- [ ] **Step 2: Add Cubicle docs page**

The docs page must include:

```text
graph POC architecture
query examples
Flink slice definition
synthetic data rationale
Meta/TAO-style object-association graph serving
Glean-style context metadata: source, visibility, freshness, provenance
Palantir-style operational ontology: typed objects, typed links, read-only actions
Google Docs/document fragment model
search model: object search, evidence search, graph traversal, future RAG
future Swift integration path
```

- [ ] **Step 3: Verify docs and tests**

Run:

```bash
cd /Users/prabhat/workspace/cubicle/services/cubicle-graph
go test ./...
```

Expected: all tests pass.

## Acceptance Criteria

The POC is complete when:

```text
services/cubicle-graph exists as a working Go module
synthetic dataset can be ingested into graph.db
Ent schemas represent typed execution graph concepts
AssociationStore exposes object and association primitives over Ent
source snapshots, source events, connector state, evidence, and edge metadata are stored
connector cursor, partial, tombstone, and stale states are represented
connector capabilities and source health are visible through /v1/sources
query-facing objects carry visibility/source-permission metadata
permission tuple filtering is covered by tests
document revisions, tabs, fragments, permissions, summaries, search state, and embedding metadata are stored
readiness query answers "can project Atlas launch?"
ticket trace query links ticket -> PRs -> docs -> decisions -> messages -> blockers
search query returns exact object hits and lexical evidence hits
search excludes hidden or stale evidence and detects stale FTS rows through SearchIndexState
search evaluation tracks exact hit rate, MRR, nDCG, citation coverage, and no-answer correctness
document trace query links a doc to fragments, tickets, decisions, and evidence
action candidate query returns evidence-backed read-only next actions
conflicting source facts create unresolved conflict state instead of being flattened
SQLite runs as a local WAL-backed store with bounded reads and a single writer ingestion path
normalized DB can be rebuilt from source snapshots and mapper versions
HTTP service exposes the same query layer
evaluation compares graph edges to ground truth and verifies metadata, search, fragment, and snapshot completeness
Flink Autoscaler snapshot importer exists and is offline-testable
Swift app remains untouched
```

## Recommended Execution Order

```text
1. Scaffold Go module
2. Ent + SQLite store
3. AssociationStore association primitives
4. Document fragment/search infra
5. Synthetic dataset
6. Ingest service
7. Query layer and action candidate rules
8. HTTP service
9. Source health and hardening fixtures
10. Flink Autoscaler snapshot importer
11. Evaluation
12. Docs
```

Do not start with Flink live crawling. Start with synthetic data because it gives a known answer key. Add Flink after the query layer works.
