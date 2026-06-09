# Cardinality-Safe Ent Ontology

## Goal

Build Cubicle's first durable ontology as typed Ent schemas without creating a
generic object/edge table. The graph must stay readable for humans, safe for LLM
crawlers, and bounded enough that high-cardinality person activity does not
turn into slow unbounded database traversals.

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
gqlgen now, entgql ontology API next
 |
 v
Ent typed schemas + Through edge schemas
 |
 v
SQLite local POC database
```

## Table Model

The schema has 18 tables.

| Group | Tables | Purpose |
|---|---|---|
| Core objects | `persons`, `workstreams`, `tickets`, `pull_requests`, `documents`, `document_fragments`, `messages`, `evidences` | Real workplace things and citations. |
| Cardinality-control objects | `work_surfaces`, `work_panes` | Bounded graph nodes that organize high-cardinality person activity. |
| Work graph links | `workstream_tickets`, `ticket_pull_requests`, `ticket_document_fragments`, `ticket_messages` | Typed Through edges for execution-context relationships. |
| Pane target links | `pane_document_links`, `pane_pull_request_links`, `pane_ticket_links`, `pane_message_links` | Typed Through edges from panes to target objects. |

This replaces the rejected 40-table design:

| 40-table idea | 18-table implementation |
|---|---|
| One table per document/code/ticket/message portfolio | `work_surfaces(kind = documents/code/tickets/communications)` |
| One table per facet like `DocumentsCommentedOn` | `work_panes(pane_kind = documents_commented_on)` |
| One link table per action like `DocumentsCommentedOnLink` | Target-specific link tables with `relation_kind` |

## Person-Centered Topology

```text
                              Person
                                |
             +------------------+-------------------+
             |                  |                   |
             v                  v                   v
     WorkSurface            WorkSurface        WorkSurface
    kind: documents         kind: code         kind: tickets
             |                  |                   |
             v                  v                   v
        WorkPane            WorkPane            WorkPane
 documents_commented_on  pull_requests_reviewed tickets_owned
             |                  |                   |
             v                  v                   v
    PaneDocumentLink   PanePullRequestLink   PaneTicketLink
             |                  |                   |
             v                  v                   v
         Document          PullRequest            Ticket
```

The high-cardinality layer is always the final link table. Query code pages and
ranks link rows before loading target objects.

## Ent Pattern

`WorkPane` is generic at the ontology-pattern level, but Ent edges are concrete
because Ent is statically typed.

```text
WorkPane
 |
 +-- documents      -> Document       through PaneDocumentLink
 +-- pull_requests  -> PullRequest    through PanePullRequestLink
 +-- tickets        -> Ticket         through PaneTicketLink
 +-- messages       -> Message        through PaneMessageLink
```

This is the important Ent-specific correction:

```text
invalid
 |
 +-- one polymorphic WorkPane -> Target edge

valid
 |
 +-- one concrete edge per target schema
 +-- one concrete Through schema per target schema
```

## Link Metadata

Every Through link has metadata needed for slicing, ML ranking, and evidence:

```text
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

This makes the query shape efficient:

```text
Person
 |
 +-- WorkSurface
 |
 +-- WorkPane
 |
 +-- Pane*Link
       where freshness_state = fresh
       order by rank_score, last_activity_at
       limit N
 |
 +-- target objects
```

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
entgql public ontology API
 |
 +-- generated object queries
 +-- generated filters
 +-- generated CRUD mutations for POC speed
 +-- connection pagination for high-cardinality link reads
```

REST remains limited to local process mechanics such as `/healthz`.

## Proof Basis

- Meta TAO organizes graph reads as object association lists with counts and
  ranged reads, which maps to `WorkPane -> Pane*Link` association lists.
- Ent supports metadata-bearing edge schemas with `Through`, but `Through` must
  be applied to concrete M2M edges, not polymorphic target unions.
- Glean-style workplace search needs content, people, activity, permissions,
  freshness, and evidence to stay queryable.
- Palantir-style ontology design favors typed objects and typed links over
  generic graph tables.

Primary references:

```text
Ent schema edges / Through: https://entgo.io/docs/schema-edges/
Ent GraphQL / entgql:       https://entgo.io/docs/graphql/
Meta TAO paper:             https://www.usenix.org/system/files/conference/atc13/atc13-bronson.pdf
Glean Knowledge Graph:      https://docs.glean.com/security/knowledge-graph
Palantir link types:        https://palantir.com/docs/foundry/object-link-types/link-types-overview/
```
