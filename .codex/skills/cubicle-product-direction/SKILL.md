---
name: cubicle-product-direction
description: Preserve and apply Cubicle product direction. Use when Codex is asked what Cubicle should become, what to build next, how to prioritize, how to reduce TPM/program-manager work, how to extend with Slack/Google Docs/Drive/Jira/GitHub, how to align architecture to the product thesis, or how Glean/Palantir-style lessons should influence Cubicle.
---

# Cubicle Product Direction

Use this skill for product, roadmap, and architecture-priority decisions.

## Thesis

Cubicle should reduce TPM/program-manager coordination work by giving engineers evidence-backed execution context.

```text
engineering signals
 |
 v
source-neutral knowledge layer
 |
 v
insight surfaces
 |
 +-- what changed
 +-- what matters
 +-- what is blocked
 +-- who owns it
 +-- what decision is pending
 +-- what action should happen next
```

## Current Build Arc

```text
foundation
 |
 +-- monorepo                  -> code has a durable home
 +-- DAO / DB setup            -> storage can become connector-neutral
 +-- signal connectors         -> sources emit normalized batches
 +-- production wiring         -> refresh uses connector path
 +-- runtime reliability       -> app can find local data root
 +-- iMessage correctness      -> unavailable sources are visible
```

## What To Build Toward

```text
next product architecture
 |
 +-- connectors
 |     -> Webex, iMessage, Slack, Docs, Drive, Jira, GitHub, Calendar
 |
 +-- knowledge graph
 |     -> people, teams, projects, docs, decisions, issues, owners, blockers
 |
 +-- permission + provenance layer
 |     -> no insight without source, timestamp, visibility, and confidence
 |
 +-- insight engine
 |     -> focus, questions, beliefs, contradictions, stale decisions
 |
 +-- action layer
       -> suggested follow-ups, owner pings, decision records, safe writeback
```

## Decision Rules

- Prefer evidence-backed next actions over dashboards.
- Prefer source-neutral signal models over source-specific UI state.
- Prefer typed objects/relations over text-only summaries.
- Keep permission, provenance, and freshness visible.
- Make partial connector failure visible instead of silently stale.
- Do not prioritize transcription unless the user explicitly asks.

## External Benchmarks

Use these as product shapes, not copy targets:

```text
Glean lesson
 |
 +-- many connectors
 +-- permission-aware indexing
 +-- content + people + activity graph
 +-- enterprise search and answers
```

```text
Palantir lesson
 |
 +-- ontology objects
 +-- properties + links
 +-- actions/functions
 +-- dynamic security
 +-- operational workflows
```

```text
Cubicle path
 |
 +-- engineering execution graph
 +-- evidence-backed operational insights
 +-- safe actions for engineers
```

## References

Read selectively:

- `references/product-roadmap.md` for product stages and anti-goals.
- `references/pr-history.md` for what has already been built.
- Repo docs: `docs/knowledge-graph-deep-dive.md`, `docs/superpowers/specs/2026-06-01-signal-connectors-design.md`.
