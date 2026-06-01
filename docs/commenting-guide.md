# Commenting Guide

Use this as the durable commenting rubric for Cubicle.

## Goal

Comments should preserve context that is expensive to recover from code alone.
Assume the reader is a senior engineer.

```text
Good comment target
  -> boundary or ownership
  -> persistence / side effect
  -> compatibility path
  -> failure isolation
  -> cache identity / freshness
  -> non-obvious invariant

Bad comment target
  -> repeats type or method name
  -> explains common Swift syntax
  -> narrates obvious control flow
  -> line numbers from other files
  -> long essays in docblocks
```

## What

Add docstrings to exported and important internal types/methods when they explain:

- API contract at a module boundary.
- Why data is persisted, cached, or normalized.
- What owns a workflow, side effect, or external integration.
- Compatibility with legacy schemas, files, or generated payloads.
- Failure policy, especially when errors are intentionally swallowed or isolated.
- Identity rules for cache keys, question IDs, sync watermarks, or database uniqueness.

Keep inline comments for local invariants only. Prefer one to three lines.

## Why

The codebase mixes product UI, local runtime files, SQLite, Webex polling, iMessage reads,
Codex prompt orchestration, and transcription. New work should preserve the mental model
without forcing readers to reconstruct ownership from scattered call sites.

## How

Before keeping a comment, ask:

```text
Would removing this confuse a senior engineer about behavior, ownership, or risk?
  -> yes: keep it
  -> no: delete it
```

Write comments like this:

```swift
/// Loads an exact analysis cache only when the manifest proves it is current.
func loadExactFocusAnalysisCache(...)

// Sparse fallback detection only matters for tiny live snapshots that can
// represent partial sync output. Larger caches are too expensive to probe.
```

Do not write comments like this:

```swift
/// Initializes the service.
init(...)

/// Loops over candidates and adds them to the array.
```

For PR summaries and diffs, keep prose brief but preserve ASCII DAGs, tables, and
counter trees when they explain structure faster than paragraphs.
