# Ontology Service

`ontology-service` is Cubicle's local Go backend for the typed workplace graph.
The service currently has a Gin localhost shell, gqlgen endpoint, SQLite
storage, HOCON configuration, and the first Ent ontology schemas.

```text
Cubicle client
 |
 v
Gin localhost server
 |
 +-- GET /healthz
 |     -> process health for local validation
 |
 +-- POST /graphql
 |     -> gqlgen product API contract
 |
 +-- GET /playground
 |     -> local GraphQL playground
 |
 v
Ent typed ontology
 |
 +-- Person -> WorkArea -> WorkLens -> WorkLensWindow -> *LensResult -> target
 |
 +-- SourceRun -> SourceObservation -> EvidenceAnchor -> Evidence
 |
v
SQLite local POC database
```

## Current Scope

```text
included now
 |
 +-- Gin routing, recovery, logging
 +-- gqlgen schema/codegen wiring
 +-- minimal GraphQL health query
 +-- SQLite storage foundation
 +-- Ent runtime startup, migration, and ontology hook registration
 +-- HOCON/env/flag runtime configuration
 +-- typed Ent schemas for the cardinality-safe ontology
 +-- Source Evidence Spine schemas for source coverage, identity, observations, and citations

deferred
 |
 +-- crawler ingestion API
 +-- source-specific writers
 +-- public entgql object queries
 +-- Swift graph explorer screens
```

GraphQL is the product query contract. REST stays limited to health and local
server mechanics until there is a concrete process endpoint that does not fit
GraphQL.

## Requirements

- Go `1.25.1`
- CGO enabled for `github.com/mattn/go-sqlite3`
- macOS or Linux shell

Check your local toolchain:

```bash
go version
go env CGO_ENABLED
```

Expected shape:

```text
go version go1.25.1 ...
1
```

## Setup

From the Cubicle repo root:

```bash
cd services/ontology-service
go mod download
go generate ./graph
go generate ./ent
go test ./...
```

Framework dependencies:

```text
Gin     -> server shell: routing, recovery, request logging
gqlgen  -> GraphQL schema, generated execution code, typed resolvers
Ent     -> typed ontology schemas and generated query builders
SQLite  -> local persistence foundation for POC ontology storage
HOCON   -> local runtime configuration files
```

## Configure

The service loads configuration in this order:

```text
defaults < HOCON config file < environment variables < command-line flags
```

Example config:

```bash
mkdir -p .data
cp config/ontology-service.conf.example .data/ontology-service.conf
```

Important keys:

```hocon
server {
  listen_addr = "127.0.0.1:48080"
  allow_public_bind = false
}

storage {
  data_root = ".data"
  database_path = ".data/graph.db"
  sqlite_busy_timeout = 5s
}

graphql {
  playground_enabled = true
}
```

Equivalent environment variables:

```text
CUBICLE_ONTOLOGY_CONFIG_PATH
CUBICLE_ONTOLOGY_LISTEN_ADDR
CUBICLE_ONTOLOGY_ALLOW_PUBLIC_BIND
CUBICLE_ONTOLOGY_DATA_ROOT
CUBICLE_ONTOLOGY_DATABASE_PATH
CUBICLE_ONTOLOGY_SQLITE_BUSY_TIMEOUT
CUBICLE_ONTOLOGY_GRAPHQL_PLAYGROUND_ENABLED
```

## Run Server

```bash
cd services/ontology-service
go run ./cmd/ontology-service serve \
  --config .data/ontology-service.conf \
  --listen 127.0.0.1:48080
```

In another terminal:

```bash
curl -s http://127.0.0.1:48080/healthz

curl -s -X POST http://127.0.0.1:48080/graphql \
  -H 'Content-Type: application/json' \
  -d '{"query":"query { health { ok service } }"}'
```

Expected responses:

```json
{"ok":true,"service":"ontology-service"}
```

```json
{"data":{"health":{"ok":true,"service":"ontology-service"}}}
```

The local playground is available at:

```text
http://127.0.0.1:48080/playground
```

Set `graphql.playground_enabled = false` or pass
`--graphql-playground=false` when you want only `/healthz` and `/graphql`.

## Ontology Graph

Current Architecture              Better Architecture
 |                                |
 +-- Person -> every doc/PR       +-- Person -> WorkArea
 |   -> unbounded fanout          |   -> few stable domains
 |                                |
 +-- one edge per raw activity    +-- WorkArea -> WorkLens
 |   -> hard to page/reason       |   -> saved bounded view
 |                                |
 +-- target loaded directly       +-- WorkLens -> WorkLensWindow -> *LensResult
     -> timeout risk                  -> bound, page, rank, then load targets

The implemented topology is:

```text
Person
 |
 +-- WorkArea(kind: documents)
 |    |
 |    +-- WorkLens(kind: documents_commented_on, target: document)
 |         |
 |         +-- WorkLensWindow(kind: recent/time_bucket/source)
 |              |
 |              +-- DocumentLensResult
 |                   |
 |                   +-- Document
 |
 +-- WorkArea(kind: code)
 |    |
 |    +-- WorkLens(kind: pull_requests_reviewed, target: pull_request)
 |         |
 |         +-- WorkLensWindow(kind: recent/time_bucket/source)
 |              |
 |              +-- PullRequestLensResult
 |                   |
 |                   +-- PullRequest
 |
 +-- WorkArea(kind: tickets)
 |    |
 |    +-- WorkLens(kind: tickets_owned, target: ticket)
 |         |
 |         +-- WorkLensWindow(kind: recent/time_bucket/source)
 |              |
 |              +-- TicketLensResult
 |                   |
 |                   +-- Ticket
 |
 +-- WorkArea(kind: communications)
      |
      +-- WorkLens(kind: messages_authored, target: message)
           |
           +-- WorkLensWindow(kind: recent/time_bucket/source)
                |
                +-- MessageLensResult
                     |
                     +-- Message
```

The schema has 23 ontology tables.

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

bounded person graph
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

source evidence spine
 |
 +-- source_runs
 +-- external_identities
 +-- source_observations
 +-- evidence_anchors
```

Each `*LensResult` row stores relation metadata, freshness, rank score,
activity timestamps, visibility hints, confidence, and evidence counts. Query
code should select bounded `WorkLensWindow` rows first, then page and rank
result rows before loading targets.

Source identity and source-observation authority live in the Source Evidence
Spine. Existing `source`, `source_instance`, `external_id`, and `source_url`
fields on object/link rows are local display/cache fields until writers migrate
to `ExternalIdentity` and `SourceObservation`.

## Source Evidence Spine

```text
current overloaded fields          source evidence spine
 |                                  |
 +-- WorkLensWindow                 +-- SourceRun
 |   -> serving partition           |   -> crawl coverage and failure authority
 |                                  |
 +-- LensResult.source/external_id  +-- ExternalIdentity
 |   -> display/cache hint          |   -> aliases, moves, merges, deleted IDs
 |                                  |
 +-- Evidence.excerpt               +-- SourceObservation
 |   -> citation text only          |   -> observed state, permissions, content hash
 |                                  |
 +-- source_url on target rows      +-- EvidenceAnchor
     -> open-link hint                  -> exact citeable source span
```

The read path for evidence-backed answers is explicit:

```text
Evidence
 |
 +-- EvidenceAnchor
 |     -> exact paragraph, message, comment, or review-comment span
 |
 +-- SourceObservation
 |     where is_deleted = false
 |     where permission_policy_key and visibility_hash match the viewer
 |
 +-- SourceRun
       where status in complete, optionally partial with a visible warning
```

This keeps search and LLM answers source-grounded without making arbitrary text
chunks the canonical graph. `WorkLensWindow` remains a bounded serving/read
partition; `SourceRun` owns crawl checkpoints and coverage explanation.

## Current Packages

```text
graph
 |
 +-- schema.graphqls
 |     -> minimal GraphQL schema
 |
 +-- generate.go
       -> gqlgen codegen entrypoint

ent/schema
 |
 +-- source_run.go
 |     -> source crawl coverage and checkpoint authority
 |
 +-- external_identity.go
 |     -> source-native IDs mapped to typed Cubicle objects
 |
 +-- source_observation.go
 |     -> observed item state, visibility, deletion, and content hash
 |
 +-- evidence_anchor.go
 |     -> exact citeable spans inside observed source items
 |
 +-- work_area.go
 |     -> bounded person-owned work domain
 |
 +-- work_lens.go
 |     -> bounded saved view under a work area
 |
 +-- work_lens_window.go
 |     -> bounded partition for paging and serving materialization
 |
 +-- *_lens_result.go
       -> metadata-bearing Through edges to targets

internal/ontology
 |
 +-- lens_model.go
       -> canonical WorkArea/WorkLens/WorkRelation vocabulary

internal/ontologyhooks
 |
 +-- lens_hooks.go
       -> cross-row invariant checks for WorkLens and results

internal/sourcespine
 |
 +-- source_spine.go
       -> source target validation and permission-gated evidence-anchor queries

internal/entstore
 |
 +-- entstore.go
      -> SQLite-backed Ent startup, migration, and hook registration

internal/graphql
 |
 +-- resolver.go
 |     -> gqlgen dependency root
 |
 +-- schema.resolvers.go
 |     -> health query resolver
 |
 +-- generated/
 |     -> gqlgen generated execution package
 |
 +-- model/
       -> gqlgen generated DTO package

internal/httpapi
 |
 +-- router.go
 |     -> Gin router, middleware, endpoint registration, runtime options
 |
 +-- health.go
 |     -> GET /healthz
 |
 +-- graphql.go
       -> POST /graphql and GET /playground

internal/storage
 |
 +-- storage.go
      -> SQLite open, PRAGMAs, transaction helper used under Ent

internal/config
 |
 +-- config.go
       -> defaults, HOCON file loading, environment overrides

cmd/ontology-service
 |
 +-- main.go
      -> serve command, config parsing, Ent startup, localhost bind guard
```

## Design Rules

- Use typed Ent schemas, not durable generic `Object` / `Association` tables.
- Keep high-cardinality activity behind `WorkLens -> WorkLensWindow -> *LensResult`.
- Use `WorkLensWindow` for source/time/rank bounded reads and serving materialization.
- Use `SourceRun` for source crawl coverage, checkpoints, partial failure, and
  rate-limit explanation.
- Use `ExternalIdentity -> SourceObservation -> EvidenceAnchor` for source
  identity, item state, permissions, deletion, content hashes, and citations.
- Keep result table endpoint and relation identity immutable.
- Build runtime writers through `internal/entstore.Open`, which migrates schema
  and installs `ontologyhooks.Register(client)`.
- Expose product queries through GraphQL; keep REST for health and mechanics.
- Keep generated gqlgen and Ent code committed because generation is part of
  review and build verification.

## Review Stack

```text
Person ontology foundation
 |
 +-- Evidence
 |
 +-- Workstream
 |
 +-- Ticket
 |
 +-- PullRequest
 |
 +-- Document
 |
 +-- Message
 |
 +-- WorkArea
 |
 +-- WorkLens
 |
 +-- DocumentLensResult
 |
 +-- PullRequestLensResult
 |
 +-- TicketLensResult
 |
 +-- MessageLensResult
 |
 +-- Cardinality docs
 |
 +-- Ent runtime + WorkLensWindow cardinality hardening
 |
 +-- Graph foundation handoff docs and skill
 |
 +-- Source Evidence Spine
```
