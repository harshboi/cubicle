---
name: cubicle-explain-visually
description: Explain Cubicle architecture and code visually. Use when the user asks to understand Cubicle components, AppModel, local services, call flow, data flow, PR structure, or "what/why/how" explanations and prefers DAGs, side-by-side diagrams, starred important files, and fewer words.
---

# Cubicle Visual Explainer

Use diagram-first explanations. Keep text short.

## Style

Prefer this shape:

```text
Component Name
 |
 +-- * ImportantFile.swift
 |     -> one-line summary
 |
 +-- SupportingFile.swift
       -> one-line summary
```

Use `*` next to important nodes. Put summaries inline on the right with `->`.

## Answer Shape

For architecture questions:

```text
Top Layer
 |
 +-- next layer
 |     -> short purpose
 |
 +-- next layer
       -> short purpose
```

Then add a short `What / Why / How` block only if useful.

For comparisons:

```text
Current                         Proposed
 |                               |
 +-- file A -> role              +-- package A -> role
 +-- file B -> role              +-- package B -> role
```

For call flow:

```text
User action
 |
 v
FileA.swift
 |
 v
FileB.swift
 |
 v
DB/cache/network
```

## Rules

- Lead with the diagram.
- Use fewer words than normal.
- Avoid long paragraphs.
- Do not explain common programming concepts.
- For code walkthroughs, cite files after the diagram.
- When unsure, inspect files first with `rg`, `sed`, and `git`.

## References

Read `references/cubicle-maps.md` when explaining AppModel, local services, runtime flow, or top-level code structure.
