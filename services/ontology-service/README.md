# Ontology Service

`ontology-service` is the Go backend for Cubicle's work graph. The service will own the ontology, graph traversal, source evidence, and eventually the Ent-backed SQLite database that powers Cubicle's local execution context.

The first implementation slice is intentionally small:

```text
Cubicle Swift app
 |
 v
localhost REST API          future PR: Gin + Huma + OpenAPI
 |
 v
query services              future PR: readiness, trace, action candidates
 |
 v
Object/association store    current PR: graph-facing store boundary
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
- No database dependency for the current scaffold PR

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

The scaffold currently has no third-party runtime dependencies, so `go mod download` should be fast.

## Run Tests

```bash
cd /Users/prabhat/workspace/cubicle/services/ontology-service
go test ./...
```

The current tests validate that the in-memory graphstore can expand a bounded workstream graph and that invalid expansion requests fail.

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
 +-- store.go
       -> small interface boundary for future HTTP/query layers

internal/ontology
 |
 +-- registry.go
      -> built-in object and association terms used by product queries
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
PR 4: Ent + SQLite AssociationStore
 |
 v
PR 5: product query endpoints
```

After the HTTP PR lands, setup will include:

```bash
cd /Users/prabhat/workspace/cubicle/services/ontology-service
go run ./cmd/ontology-service serve --listen 127.0.0.1:48080
curl -s http://127.0.0.1:48080/healthz
curl -s http://127.0.0.1:48080/openapi.json
```
