# Ontology Service

`ontology-service` is the Go backend for Cubicle's work graph. The service will own the ontology, graph traversal, source evidence, and eventually the Ent-backed SQLite database that powers Cubicle's local execution context.

The first implementation slice is intentionally small:

```text
Cubicle Swift app
 |
 v
localhost REST API          current PR: Gin + Huma + OpenAPI
 |
 v
query services              future PR: readiness, trace, action candidates
 |
 v
AssociationStore            current PR: graph-facing store boundary
 |
 v
in-memory graphstore        current PR: deterministic POC graph traversal
 |
 v
Ent + SQLite                future PR: durable graph database
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
- macOS or Linux shell
- No database dependency for the current server slice

Check your Go version:

```bash
go version
```

Expected shape:

```text
go version go1.25.1 ...
```

## Setup

From the Cubicle repo root:

```bash
cd /Users/prabhat/workspace/cubicle/services/ontology-service
go mod download
go test ./...
```

This downloads the HTTP framework dependencies used by the server slice:

```text
Gin   -> server framework: routing, middleware, recovery
Huma  -> typed REST operations, validation, OpenAPI, docs
```

## Run Tests

```bash
cd /Users/prabhat/workspace/cubicle/services/ontology-service
go test ./...
```

The current tests validate that the in-memory graphstore can expand a bounded workstream graph and that invalid expansion requests fail.

## Run Server

```bash
cd /Users/prabhat/workspace/cubicle/services/ontology-service
go run ./cmd/ontology-service serve --listen 127.0.0.1:48080
```

In another terminal:

```bash
curl -s http://127.0.0.1:48080/healthz
curl -s http://127.0.0.1:48080/openapi.json
curl -s -X POST http://127.0.0.1:48080/v1/graph/expand \
  -H 'Content-Type: application/json' \
  -d '{"start":{"kind":"workstream","key":"workstream:flink-autoscaler"},"depth":2,"limit_per_node":10}'
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
       -> API-safe graph DTOs: nodes, edges, predicates, expansion requests

internal/graphstore
 |
 +-- memory_store.go
 |     -> deterministic in-memory graph implementation
 |
 +-- store.go
       -> small interface boundary for future HTTP/query layers

internal/fixtures
 |
 +-- workstream.go
       -> deterministic Flink Autoscaler graph used by tests and local server

internal/httpapi
 |
 +-- router.go
 |     -> Gin engine plus Huma operation registration
 |
 +-- health.go
 |     -> GET /healthz
 |
 +-- graph.go
       -> POST /v1/graph/expand

cmd/ontology-service
 |
 +-- main.go
       -> serve command and localhost bind guard
```

## Go Design Rules Used Here

- Keep packages small and purpose-specific.
- Put service internals under `internal/` so other modules cannot accidentally depend on unstable implementation details.
- Depend on small interfaces at the boundary, not concrete storage types.
- Keep domain DTOs separate from storage entities. Ent structs should not leak into HTTP responses.
- Make graph expansion bounded by depth and per-node limit to avoid high-degree graph explosions.
- Keep evidence metadata on edges, because Cubicle answers must be traceable back to source facts.

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
PR 4: Ent + SQLite AssociationStore
 |
 v
PR 5: product query endpoints
```
