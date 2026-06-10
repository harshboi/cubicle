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
activity.

## Table Model

The schema has 19 tables.

| Group | Tables | Purpose |
|---|---|---|
| Core objects | `persons`, `workstreams`, `tickets`, `pull_requests`, `documents`, `document_fragments`, `messages`, `evidences` | Real workplace things and citations. |
| Cardinality control | `work_areas`, `work_lenses`, `work_lens_windows` | Bounded person graph nodes that organize high-cardinality activity and crawler checkpoints. |
| Execution links | `workstream_tickets`, `ticket_pull_requests`, `ticket_document_fragments`, `ticket_messages` | Typed Through edges for operational context. |
| Lens result links | `document_lens_results`, `pull_request_lens_results`, `ticket_lens_results`, `message_lens_results` | Typed Through edges from lenses to target objects. |

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

`WorkLens` is generic at the ontology-pattern level, but Ent edges are concrete
because Ent is statically typed.

```text
WorkLens
 |
 +-- windows        -> WorkLensWindow
 +-- documents      -> Document       through DocumentLensResult
 +-- pull_requests  -> PullRequest    through PullRequestLensResult
 +-- tickets        -> Ticket         through TicketLensResult
 +-- messages       -> Message        through MessageLensResult
```

Invalid and valid shapes:

```text
invalid                           valid
 |                                |
 +-- one polymorphic edge         +-- one concrete edge per target schema
     WorkLens -> Target          +-- one concrete Through schema per target
```

## Lens Result Metadata

Every result Through schema stores:

```text
work_lens_window_id
relation_kind
latest_evidence_id
evidence_count
event_count
first_seen_at
last_activity_at
rank_score
source / source_instance / external_id / source_url
freshness_state
visibility
confidence
created_at / updated_at
```

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
