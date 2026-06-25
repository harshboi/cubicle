# Bounded Graph Promotion Matrix

This is the machine-readable promotion gate for the AI-first bounded graph PoC.

Run:

```sh
.venv/bin/python tools/bounded_graph_promotion_matrix.py \
  --out-dir /tmp/bounded_graph_promotion_matrix_20260624 \
  --report-json /tmp/bounded_graph_promotion_matrix_20260624/report.json
```

The matrix is stricter than a single eval pack. A passing report must cover:

- a typed real connector case
- a non-Flink open graph case
- auth-limited sparse coverage
- source-authority promotion
- graph traversal beating seed-only and typed-row baselines
- separate raw, repaired, and deterministic scoring where a local model is used

The default matrix is the PoC promotion gate. It reports advisory gaps without
failing the run. Today, the advisory gap is `real-non-flink-connector`: we still
need a second real connector-backed non-Flink case before calling the connector
story production-generic. The `real_github_issue_pr_minimum` pack currently
clears this tag using public `cli/cli` GitHub issue/PR data.

To make advisory gaps blocking:

```sh
.venv/bin/python tools/bounded_graph_promotion_matrix.py \
  --require-advisory-tags \
  --out-dir /tmp/bounded_graph_promotion_matrix_20260624 \
  --report-json /tmp/bounded_graph_promotion_matrix_20260624/report.json
```

To inventory the local data pool for a case that could clear the advisory:

```sh
.venv/bin/python tools/bounded_graph_real_connector_inventory.py \
  --data-root /Users/harsh/workspace/data \
  --report-json /tmp/bounded_graph_real_connector_inventory.json \
  --report-md /tmp/bounded_graph_real_connector_inventory.md \
  --require-real-non-flink
```

Raw model output is diagnostic. Product-facing promotion requires repaired or
deterministic bounded graph output to pass the configured golden and promotion
gates.
