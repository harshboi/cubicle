# Ontology Service

`ontology-service` is the Go backend shell for Cubicle's future ontology graph.
The Day-0 service proves the local server and GraphQL codegen shape without
choosing typed ontology schemas yet.

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
       -> local GraphQL playground
```

## Current Scope

This slice intentionally does **not** add durable ontology tables.

```text
included now
 |
 +-- Gin routing, recovery, logging
 +-- gqlgen schema/codegen wiring
 +-- minimal health query
 +-- SQLite storage foundation from the previous PR

deferred
 |
 +-- typed Ent schemas
 +-- Ent edge schemas
 +-- tickets/docs/people/workstream GraphQL model
 +-- crawler ingestion API
```

The important direction is that GraphQL is the product query contract. REST is
kept only for health and server mechanics.

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
go test ./...
```

Framework dependencies in this slice:

```text
Gin     -> server shell: routing, recovery, request logging
gqlgen  -> GraphQL schema, generated execution code, typed resolvers
SQLite  -> local persistence foundation for later ontology storage
```

## Run Server

```bash
cd services/ontology-service
go run ./cmd/ontology-service serve --listen 127.0.0.1:48080
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

## Current Packages

```text
graph
 |
 +-- schema.graphqls
 |     -> minimal GraphQL schema
 |
 +-- generate.go
       -> gqlgen codegen entrypoint

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
 |     -> Gin router, middleware, endpoint registration
 |
 +-- health.go
 |     -> GET /healthz
 |
 +-- graphql.go
       -> POST /graphql and GET /playground

internal/storage
 |
 +-- storage.go
       -> SQLite open, PRAGMAs, transaction helper

cmd/ontology-service
 |
 +-- main.go
       -> serve command and localhost bind guard
```

## Design Rules

- GraphQL is the product API contract from Day 0.
- Keep REST limited to health and local server mechanics.
- Do not introduce generic durable `Object` / `Association` Ent tables.
- Do not introduce typed ontology Ent schemas until the ontology model is
  designed explicitly.
- Keep generated code committed because gqlgen generation is part of the build.

## Next PRs

```text
PR 5: GraphQL service foundation
 |
 v
PR 6: GraphQL config/runtime settings
 |
 v
PR 7: typed ontology schema design with Ent
 |
 v
PR 8: source ingestion once schema targets are explicit
```
