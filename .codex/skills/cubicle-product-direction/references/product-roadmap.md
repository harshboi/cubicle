# Cubicle Product Roadmap

## Mission

```text
replace coordination overhead
 |
 +-- fewer status meetings
 +-- fewer TPM/program-manager follow-ups
 +-- more engineer-owned clarity
 |
 v
evidence-backed next actions
```

## Product Layers

```text
Layer 1: source acquisition
 |
 +-- Webex / iMessage now
 +-- Slack / Docs / Drive / Jira / GitHub / Calendar later

Layer 2: durable knowledge
 |
 +-- objects
 +-- events
 +-- relations
 +-- permissions
 +-- provenance
 +-- freshness

Layer 3: insight
 |
 +-- focus views
 +-- questions
 +-- beliefs
 +-- contradictions
 +-- blockers / owners / decisions

Layer 4: action
 |
 +-- suggested follow-ups
 +-- owner confirmation
 +-- stale-decision nudges
 +-- safe writeback to source systems
```

## Near-Term Priorities

```text
current codebase
 |
 +-- finish DAO + connector stack
 +-- make connector failures visible
 +-- harden cache/DB boundaries
 +-- define typed knowledge objects and relations
 +-- add future connector contract tests before adding sources
```

## Anti-Goals

```text
avoid
 |
 +-- chat mirror as product
 +-- dashboards without recommended action
 +-- AppModel as source-specific integration layer
 +-- silent stale data
 +-- permission-blind indexing
 +-- transcription work unless explicitly requested
```

## Good Feature Test

A feature is valuable if it answers at least one:

```text
does this tell an engineer...
 |
 +-- what changed?
 +-- what matters?
 +-- what is blocked?
 +-- who owns it?
 +-- what decision is pending?
 +-- what should happen next?
```
