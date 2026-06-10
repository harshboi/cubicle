# Cubicle Codex Handoff

Use this file when setting up a new laptop or a fresh Codex session for Cubicle.
It is the short entrypoint; deeper context lives in the files and PRs below.

## Start Here

```text
new machine / fresh session
 |
 +-- clone repo
 |
 +-- read memory.md
 |     -> user preferences, product direction, graph defaults
 |
 +-- load these skills when relevant
 |     -> cubicle-repo-workflow
 |     -> cubicle-product-direction
 |     -> cubicle-explain-visually
 |     -> cubicle-architecture-navigation
 |
 +-- if working on ontology-service graph
       -> load cubicle-graph-foundations from PR #68
       -> read .codex/tmp/source-evidence-spine-transition-research.md from PR #68
```

## Context PRs

```text
main
 |
 +-- #71 Capture Cubicle Codex memory
 |     -> memory.md
 |     -> visual explanation skill updates
 |     -> this handoff entrypoint
 |
 +-- ontology-service stack
       |
       +-- #68 Document graph foundation handoff
       |     -> cubicle-graph-foundations skill
       |     -> raw debate references
       |     -> Source Evidence Spine transition research note
       |
       +-- #69 Add Source Evidence Spine
       |     -> SourceRun / ExternalIdentity / SourceObservation / EvidenceAnchor
       |     -> Ent generated code and source-spine tests
       |
       +-- #70 Add Source Evidence Spine GraphQL reads
             -> sourceRun / externalIdentity / evidenceAnchors queries
             -> Ent client injection into gqlgen resolvers
```

## Checkout Commands

```bash
gh repo clone harshboi/cubicle
cd cubicle
gh pr checkout 71
```

For ontology-service graph work:

```bash
gh pr checkout 70
```

PR #70 includes #69 and #68 in its stacked history.

## What To Preserve

```text
working style
 |
 +-- diagrams first
 +-- comparisons side by side
 +-- show current architecture vs better architecture
 +-- explain PRs chronologically when reviewing
 +-- keep PRs small and stacked
 +-- write detailed PR summaries
```

```text
Go service style
 |
 +-- use framework-backed service patterns
 +-- keep Gin for HTTP mechanics
 +-- keep gqlgen GraphQL for product reads
 +-- keep Ent as typed ontology ORM/query layer
 +-- comment exported types/functions and important vars/consts
 +-- use truthful names for fixture/demo data
```

```text
graph foundation
 |
 +-- typed Ent schemas, not durable generic node/edge tables
 +-- high-cardinality reads behind WorkArea -> WorkLens -> WorkLensWindow
 +-- source truth behind SourceRun -> SourceObservation -> EvidenceAnchor
 +-- retrieval/vector/search projections come after exact evidence anchors
```

## Do Not Re-Research First

These decisions are already captured unless the user explicitly asks to reopen
them:

```text
settled for current POC
 |
 +-- use typed Ent schemas
 +-- keep Source Evidence Spine source-neutral
 +-- do not make DocumentFragment the universal evidence unit
 +-- do not make embeddings or summaries canonical truth
 +-- use GraphQL for product graph reads
 +-- keep REST for health/local mechanics
```

## Local Verification

For ontology-service branches:

```bash
cd services/ontology-service
go generate ./ent
go generate ./graph
go test ./...
go vet ./...
```

From repo root:

```bash
git diff --check
gh pr list --state open --json number,title,headRefName,baseRefName,url
```
