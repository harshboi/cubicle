# Ontology Service

`ontology-service` is the Go backend for Cubicle's work graph. The service owns the ontology, graph traversal, source evidence, and Ent-backed SQLite database that powers Cubicle's local execution context.

The first implementation slice is intentionally small:

```text
Cubicle Swift app
 |
 v
localhost REST API          Gin + Huma + OpenAPI
 |
 v
query services              workstream overview now; readiness/actions later
 |
 v
Object/association store    read/write interfaces for graph consumers and seeders
 |
 v
Ent-backed graphstore       generated schema plus domain mapping
 |
 v
SQLite                      local durable graph database
```

## Why This Service Exists

Cubicle needs a source-neutral graph for engineering work:

```text
workstream
 |
 +-- ticket
 |    |
 |    +-- pull request
 |    |    |
 |    |    +-- changed file
 |    |
 |    +-- blocker
 |         |
 |         +-- action candidate
 |
 +-- docs, messages, decisions, people, teams
```

The Swift app should not know about Ent schemas, SQLite tables, crawler snapshots, or raw source payloads. It should call stable HTTP DTOs. This service keeps that boundary explicit.

## Requirements

- Go `1.25.1`
- CGO enabled for `github.com/mattn/go-sqlite3`
- macOS or Linux shell
- Local SQLite database created under `.data/graph.db` by default

Check your Go version:

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
cd /Users/prabhat/workspace/cubicle/services/ontology-service
go mod download
go test ./...
```

This downloads the service dependencies used by the server slice:

```text
Gin   -> server framework: routing, middleware, recovery
Huma  -> typed REST operations, validation, OpenAPI, docs
Ent   -> generated graph persistence layer
SQLite -> local graph database
```

## Run Tests

```bash
cd /Users/prabhat/workspace/cubicle/services/ontology-service
go test ./...
```

The current tests validate bounded graph expansion for both memory and Ent stores, SQLite setup, HTTP/OpenAPI behavior, and command startup composition.

## Storage

The service now has a SQLite storage foundation under `internal/storage`.
It is intentionally lower-level than Ent:

```text
internal/storage
 |
 +-- opens database/sql SQLite handle
 +-- applies local-first PRAGMAs
 +-- owns transaction commit/rollback
 |
 v
Ent client
 |
 v
EntStore
 |
 v
graphstore.Expander
```

The Ent graphstore slice adds generated code under `ent/`:

```text
ent/schema
 |
 +-- object.go
 |     -> source-neutral graph object
 |
 +-- association.go
       -> metadata-rich association between graph objects

internal/graphstore
 |
 +-- ent_store.go
       -> Ent-backed implementation of bounded graph expansion
```

Regenerate Ent code after schema edits:

```bash
cd /Users/prabhat/workspace/cubicle/services/ontology-service
go generate ./ent
go test ./...
```

Default local paths come from `internal/config`:

```text
CUBICLE_ONTOLOGY_LISTEN_ADDR   default: 127.0.0.1:48080
CUBICLE_ONTOLOGY_DATA_ROOT     default: .data
CUBICLE_ONTOLOGY_DATABASE_PATH default: .data/graph.db
```

SQLite settings enforced by tests:

```text
foreign_keys=ON
journal_mode=WAL
busy_timeout>=5000ms
synchronous=NORMAL
```

## Run Server

```bash
cd /Users/prabhat/workspace/cubicle/services/ontology-service
go run ./cmd/ontology-service serve \
  --listen 127.0.0.1:48080 \
  --database .data/graph.db
```

The server creates the Ent schema on startup. It starts with an empty graph
unless a later config layer explicitly enables dev-only fake sample data.

In another terminal:

```bash
curl -s http://127.0.0.1:48080/healthz
curl -s http://127.0.0.1:48080/openapi.json
curl -s http://127.0.0.1:48080/v1/workstreams/flink-autoscaler/overview
curl -s -X POST http://127.0.0.1:48080/v1/graph/expand \
  -H 'Content-Type: application/json' \
  -d '{"start":{"object_type":"workstream","key":"workstream:flink-autoscaler"},"depth":2,"limit_per_object":10}'
```

Expected health response:

```json
{"ok":true}
```

## Current Packages

```text
internal/domain
 |
 +-- graph.go
       -> API-safe graph DTOs: objects, associations, expansion requests

internal/graphstore
 |
 +-- memory_store.go
 |     -> deterministic in-memory graph implementation
 |
 +-- ent_store.go
 |     -> Ent-backed graph implementation
 |
 +-- store.go
       -> small read/write interface boundaries for HTTP, sample data, and future crawlers

internal/ontology
 |
 +-- registry.go
      -> built-in object and association terms used by product queries

internal/sampledata
 |
 +-- workstream.go
       -> deterministic fake Flink Autoscaler graph used by tests

internal/query
 |
 +-- workstream.go
       -> Swift-friendly workstream overview built on bounded graph expansion

internal/httpapi
 |
 +-- router.go
 |     -> Gin engine plus Huma operation registration
 |
 +-- health.go
 |     -> GET /healthz
 |
 +-- graph.go
 |     -> POST /v1/graph/expand
 |
 +-- workstream.go
       -> GET /v1/workstreams/{slug}/overview

cmd/ontology-service
 |
 +-- main.go
       -> serve command, localhost bind guard, and Ent-backed startup wiring
```

## Go Design Rules Used Here

- Keep packages small and purpose-specific.
- Put service internals under `internal/` so other modules cannot accidentally depend on unstable implementation details.
- Depend on small interfaces at the boundary, not concrete storage types.
- Keep domain DTOs separate from storage entities. Ent structs should not leak into HTTP responses.
- Make graph expansion bounded by depth and per-object limit to avoid high-degree graph explosions.
- Keep evidence metadata on associations, because Cubicle answers must be traceable back to source facts.

## Next PRs

```text
PR 1: design docs
 |
 v
PR 2: ontology-service scaffold and README
 |
 v
PR 3: Gin + Huma HTTP server
 |
 v
PR 4: SQLite storage foundation
 |
 v
PR 5: Ent-backed graphstore
 |
 v
PR 6: Ent-backed server startup
 |
 v
PR 7: workstream overview query endpoint
 |
 v
PR 8: crawler/import endpoints
```
