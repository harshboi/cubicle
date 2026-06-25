# Real Connector Bounded Probe

This pack probes an existing ontology SQLite database instead of a synthetic
fixture.

It exports `boundedGraphContext` from typed Ent product rows, generates a
golden-question file from the exported bounded context, and scores:

- raw local model output
- repaired model output
- deterministic generic traversal baseline
- seed-only baseline
- typed-row/object-only baseline

The generated golden pack is intentionally narrow. It checks whether the answer
can restate the exported traversal shape, seed object, selected relationship
edges, hydration gates, and sparse coverage guardrail without using
WorkProgram, TPM, analytics citations, or source diagnostic rows.

Example:

```sh
DATABASE=/Users/harsh/workspace/data/flink-pr-jira-1000-plus-500-jira-2026-06-22/ontology.ai-tpm-1000-open-auth-hydrated-retry998-20260622.db \
START_OBJECT_TYPE=ticket \
START_KEY=ticket:jira:FLINK-32695 \
ASSOCIATION_TYPES=implemented_by \
DEPTH=1 \
LIMIT_PER_OBJECT=6 \
OUT_DIR=/tmp/real_connector_bounded_probe_flink_32695 \
tools/eval_packs/real_connector_bounded_probe/run_llm.sh
```
