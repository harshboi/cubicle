<!--
Association:
Person -> WorkArea -> WorkLens -> WorkLensWindow -> typed LensResult -> product row.
Evidence proves product rows and typed relationships; SourceSync* tables report
coverage and connector health without becoming product adjacency.
-->

# Cardinality-Safe Ent Ontology

## Goal

Build Cubicle's first durable ontology as typed Ent schemas without creating a
generic object/edge table. The graph must stay readable for humans, useful to
LLM crawlers, and bounded enough that high-cardinality person activity does not
turn into slow unbounded database traversals.

## Service Shape

```text
Cubicle client
 |
 v
Gin localhost shell
 |
 +-- /healthz
 +-- /graphql
 +-- /playground
 |
 v
gqlgen now, reviewed entgql object queries later
 |
 v
Ent typed schemas + Through edge schemas
 |
v
SQLite local POC database
```

GraphQL is the product query contract. REST remains limited to process health
and local server mechanics.

## Why This Is Better

Current Architecture              Better Architecture
 |                                |
 +-- Person -> all documents      +-- Person -> WorkArea(documents)
 |   -> high-cardinality edge     |   -> small stable domain node
 |                                |
 +-- Person -> all PRs/tickets    +-- WorkArea -> WorkLens
 |   -> mixed semantics           |   -> one bounded saved view
 |                                |
 +-- load targets first           +-- WorkLens -> WorkLensWindow -> *LensResult
     -> ranking happens late          -> bound, page, rank, then load targets

This mirrors the useful part of Meta TAO-style association lists: keep a stable
object graph, then page and rank bounded association lists before expanding the
target objects. `WorkLensWindow` adds one more cardinality boundary so a single
long-lived lens does not become a hot parent for years of messages or document
activity. Source crawl monitoring lives in connector/scope/run tables, not in
serving windows or per-object run snapshots.

## Table Model

The schema has 34 product, relationship, proof, lens, and sync-support tables.

| Group | Tables | Purpose |
|---|---|---|
| Source-backed product objects | `persons`, `workstreams`, `tickets`, `pull_requests`, `documents`, `messages` | Real workplace things. Source-native identity, URL, ACL state, deletion state, content hash, and freshness live on these rows when the row is source-backed. |
| Cardinality control | `work_areas`, `work_lenses`, `work_lens_windows` | Bounded person graph nodes that organize high-cardinality activity and serving materialization. |
| Typed relationship links | `ticket_assignments`, `document_authorships`, `message_authorships`, `pull_request_authorships`, `pull_request_reviews`, `message_mentions`, `ticket_mentions`, `workstream_tickets`, `ticket_pull_requests`, `ticket_documents`, `ticket_messages`, `document_links` | Concrete Ent edge schemas with relationship-specific kind columns, evidence counts, confidence, freshness, source state, and activity metadata. |
| Lens result links | `document_lens_results`, `pull_request_lens_results`, `ticket_lens_results`, `message_lens_results` | Typed Through edges from lenses to target objects. |
| Proof and resolution support | `evidences`, `person_identities`, `source_aliases`, `unresolved_references` | Locator-grade proof, focused identity resolution, source aliases, and references that are not yet materialized as typed product relationships. |
| Sync monitoring support | `source_connections`, `source_scopes`, `source_scope_states`, `source_sync_runs`, `source_sync_issues` | Connector configuration, bounded scope state, run coverage, cursors, failures, and rate-limit diagnostics. These rows are operational support, not normal product graph traversal. |

## Person-Centered Topology

```text
                              Person
                                |
             +------------------+-------------------+
             |                  |                   |
             v                  v                   v
          WorkArea           WorkArea            WorkArea
       kind: documents       kind: code        kind: tickets
             |                  |                   |
             v                  v                   v
          WorkLens           WorkLens            WorkLens
 documents_commented_on  pull_requests_reviewed tickets_owned
             |                  |                   |
             v                  v                   v
      WorkLensWindow     WorkLensWindow       WorkLensWindow
       recent/source       time_bucket          recent/source
             |                  |                   |
             v                  v                   v
   DocumentLensResult  PullRequestLensResult  TicketLensResult
             |                  |                   |
             v                  v                   v
          Document          PullRequest            Ticket
```

The high-cardinality path always crosses a window before the final result
table. Query code chooses windows first, then pages and ranks result rows before
loading target objects.

## Ent Pattern

`WorkLens` is generic at the ontology-pattern level, but serving traversal
stays window-first. Ent result schemas are concrete because Ent is statically
typed, but `WorkLens` itself does not expose direct target edges.

```text
WorkLens
 |
 +-- windows -> WorkLensWindow
       |
       +-- DocumentLensResult      -> Document
       +-- PullRequestLensResult   -> PullRequest
       +-- TicketLensResult        -> Ticket
       +-- MessageLensResult       -> Message
```

Invalid and valid shapes:

```text
invalid                            valid
 |                                 |
 +-- one polymorphic edge          +-- window-first result rows
 |   WorkLens -> Target           |   WorkLensWindow -> *LensResult -> Target
 |                                |
 +-- direct WorkLens -> Target     +-- one concrete result schema per target
     bypasses the window
```

## Lens Result Metadata

Every lens result Through schema stores:

```text
work_lens_window_id
relation_kind
latest_evidence_id
evidence_count
event_count
first_seen_at
last_activity_at
rank_score
freshness_state
visibility
confidence
created_at / updated_at
```

The legacy `source`, `source_instance`, `external_id`, and `source_url` fields
on object and relationship rows are no longer legacy display/cache hints.
Together with `source_version`, deletion state, ACL state, content hash, and
freshness fields, they are the source-backed address and state for that product
row. There is no second canonical source-object table for the same ticket,
document, message, PR, or workstream.

Efficient read shape:

```text
Person
 |
 +-- WorkArea
 |
 +-- WorkLens
 |
 +-- WorkLensWindow
 |     where lens_window_kind in (recent, time_bucket, source)
 |     limit a bounded set of partitions
 |
 +-- *LensResult
 |     where work_lens_window_id = selected window
 |     where freshness_state = fresh
 |     order by rank_score, last_activity_at
 |     limit N
 |
 +-- target objects
```

## Northstar Source-Backed Graph

Product facts and source state share the same durable row. Proof and sync
monitoring remain separate because they have different read patterns:

```text
SourceConnection
 |
 +-- SourceScope
 |     -> SourceScopeState
 |     -> SourceSyncRun
 |          -> SourceSyncIssue
 |
 +-- source-backed product graph
       |
       +-- Person -> PersonIdentity
       |
       +-- Ticket
       |     <- TicketAssignment -> Person
       |     <- TicketMention -> Person
       |     -> TicketDocument -> Document
       |     -> TicketMessage -> Message
       |     -> TicketPullRequest -> PullRequest
       |
       +-- Document
       |     <- DocumentAuthorship -> Person
       |     -> DocumentLink -> Document
       |
       +-- Message
       |     <- MessageAuthorship -> Person
       |     <- MessageMention -> Person
       |
       +-- PullRequest
             <- PullRequestAuthorship -> Person
             <- PullRequestReview -> Person
```

The intended evidence read path is direct by claim or by locator, not by
crawling a separate source graph:

```text
Product object or typed relationship
 |
 +-- latest_evidence_id
 +-- evidence_count
 |
Evidence
 |
 +-- claim_target_kind / claim_target_id
 +-- relationship_kind / relationship_id
 +-- locator_kind / locator / source_span_key
 +-- proof_state
```

This keeps the typed ontology graph efficient for product queries while giving
future search, RAG, and ML flows a citeable source trail. It also prevents
connector run metadata from becoming a high-cardinality neighbor of every
product object.

## Invariants

Lens vocabulary is centralized in `internal/ontology`, not duplicated across
GraphQL, Ent schema files, and future writer code.

```text
WorkLensKind
 |
 +-- implies exactly one WorkAreaKind
 +-- implies exactly one LensTargetKind
 +-- implies exactly one WorkRelationKind
```

Example:

```text
documents_created
 |
 +-- work_area_kind    = documents
 +-- lens_target_kind  = document
 +-- relation_kind     = created
```

| Rule | Enforcement |
|---|---|
| `WorkLens.work_lens_kind` and `WorkLens.lens_target_kind` agree | Ent schema hook. |
| `WorkLens` belongs under the implied `WorkArea.work_area_kind` | Generated Ent client hook from `internal/ontologyhooks.Register`. |
| `*LensResult.work_lens_window_id` belongs to the same `WorkLens` | Generated Ent client hook from `internal/ontologyhooks.Register`. |
| `*LensResult` uses the target table implied by the parent lens | Generated Ent client hook from `internal/ontologyhooks.Register`. |
| `*LensResult.relation_kind` matches the parent lens kind | Generated Ent client hook from `internal/ontologyhooks.Register`. |
| Result endpoints and relation identity do not mutate after create | Ent immutable fields and regenerated update builders. |

Runtime ontology writer code should use `internal/entstore.Open`, which creates
the generated Ent client, runs migrations, and installs
`ontologyhooks.Register(client)` before writes.

## GraphQL Direction

Current service state:

```text
GraphQL endpoint
 |
 +-- gqlgen health query
 +-- playground for local development
```

Next service slice:

```text
reviewed ontology GraphQL API
 |
 +-- object queries
 +-- WorkArea and WorkLens reads
 +-- connection pagination for *LensResult rows
 +-- target expansion after result filtering
```

REST remains limited to local process mechanics such as `/healthz`.

## Proof Basis

- Meta TAO organizes graph reads as object association lists with counts and
  ranged reads, which maps to `WorkLens -> WorkLensWindow -> *LensResult`
  association lists.
- Ent supports metadata-bearing edge schemas with `Through`, but `Through`
  must be applied to concrete M2M edges, not polymorphic target unions.
- Neo4j modeling guidance is query-driven: model labels, relationships, and
  properties around the questions the graph must answer, which is why Cubicle
  keeps product query shapes explicit instead of generic object/edge tables.
- Neptune and TigerGraph guidance both push teams to understand access patterns
  and avoid unbounded high-degree traversals; `WorkLensWindow` makes those
  access patterns explicit in storage.
- Glean-style workplace search needs content, people, activity, permissions,
  freshness, and evidence to stay queryable.
- Palantir-style ontology design favors typed objects and typed links over
  generic graph tables.

Primary references:

```text
Ent schema edges / Through: https://entgo.io/docs/schema-edges/
Ent GraphQL / entgql:       https://entgo.io/docs/graphql/
Meta TAO paper:             https://www.usenix.org/system/files/conference/atc13/atc13-bronson.pdf
Neo4j graph modeling:       https://neo4j.com/docs/getting-started/data-modeling/modeling-designs/
AWS Neptune modeling:       https://docs.aws.amazon.com/neptune/latest/userguide/best-practices.html
TigerGraph schema design:   https://docs.tigergraph.com/tigergraph-server/current/schema-design/overview
Glean Knowledge Graph:      https://docs.glean.com/security/knowledge-graph
Palantir link types:        https://palantir.com/docs/foundry/object-link-types/link-types-overview/
```
