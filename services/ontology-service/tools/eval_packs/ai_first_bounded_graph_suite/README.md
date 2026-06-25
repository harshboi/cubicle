# AI-First Bounded Graph Suite

This is the standing anti-pigeonhole gate for the Cubicle AI-first graph PoC.

It runs the checks that prove the current path is not only a Flink, Jira, TPM,
or `WorkProgram*` implementation:

- core Go bounded graph tests
- auth-limited sparse coverage pack
- persisted open-graph incident pack
- persisted open-graph revenue pack
- real public GitHub issue/PR OpenGraph pack
- real public PyPI project/release OpenGraph pack
- real connector negative/partial pack
- persisted company multi-seed pack

The default suite is deterministic and does not call a local model. Product
answers still have to be backed by claimable bounded graph rows, contract
checks, promotion audits, and golden-question scoring.

OpenGraph fixtures must declare quality metadata intentionally. The fixture
loader treats omitted or unknown visibility/freshness as `unknown`, and bounded
graph claim policy prevents unknown-freshness objects or relationships from
supporting product prose. Existing OpenGraph packs now state `public` / `fresh`
explicitly where that is the intended source state.

Custom/OpenGraph object rows are context-only until promoted into built-in
product schema. OpenGraph relationships can still support user-facing claims
when relationship evidence, source authority, visibility, freshness, and
endpoint gates pass; the object rows themselves export as validation context.

Run:

```sh
sh tools/eval_packs/ai_first_bounded_graph_suite/run_eval.sh
```

Artifacts go to `/tmp/ai_first_bounded_graph_suite` by default. The wrapper
writes `summary.json` with one row per pack.

The stricter promotion gate is:

```sh
.venv/bin/python tools/bounded_graph_promotion_matrix.py \
  --out-dir /tmp/bounded_graph_promotion_matrix \
  --report-json /tmp/bounded_graph_promotion_matrix/report.json
```

Use the suite for a fast deterministic regression pass. Use the matrix when
deciding whether the AI-first bounded graph path is generic enough to promote.

After a matrix run, generate the tiered architecture-readiness report:

```sh
.venv/bin/python tools/bounded_graph_architecture_readiness.py \
  --matrix-report-json /tmp/bounded_graph_promotion_matrix/report.json \
  --product-safe-evidence-json tools/eval_packs/bounded_graph_architecture_readiness/product_safe_evidence.json \
  --out-json /tmp/bounded_graph_promotion_matrix/architecture_readiness.json \
  --out-md /tmp/bounded_graph_promotion_matrix/architecture_readiness.md \
  --require-production-genericity
```

That report keeps the bars separate:

- PoC green
- production-genericity advisory green
- raw model product readiness
- repaired/deterministic product path readiness
- product-safe architecture green

To render the strategic answer to "are we pigeonholed and can an LLM summarize
by traversing the graph?", combine the readiness report with the real connector
inventory:

```sh
.venv/bin/python tools/ai_first_poc_architecture_review.py \
  --architecture-readiness-json /tmp/bounded_graph_promotion_matrix/architecture_readiness.json \
  --real-connector-inventory-json /tmp/real_connector_inventory_with_nonflink_20260624/report.rerun.json \
  --out-json /tmp/ai_first_poc_architecture_review/review.json \
  --out-md /tmp/ai_first_poc_architecture_review/review.md \
  --require-poc-viable
```

That report intentionally distinguishes graph-boundary genericity from the
current persisted connector data root. It can say the bounded graph PoC is not
pigeonholed while still reporting whether the scanned connector DBs are
Flink-shaped.

The current persisted non-Flink connector inventory uses:

```sh
.venv/bin/python tools/bounded_graph_real_connector_inventory.py \
  --database /Users/harsh/workspace/data/flink-pr-jira-1000-plus-500-jira-2026-06-22/ontology.source-scope-claimable-20260624.db \
  --database /Users/harsh/workspace/data/real-nonflink-open-graph-2026-06-24/real_github_issue_pr_minimum/real_github_issue_pr.db \
  --database /Users/harsh/workspace/data/real-nonflink-open-graph-2026-06-24/real_pypi_project_release_minimum/real_pypi_project_release.db \
  --database /Users/harsh/workspace/data/real-source-scope-registration-2026-06-24/not-attempted-source-scope.db \
  --database /Users/harsh/workspace/data/open-graph-acl-ingestion-2026-06-24/open_graph_incident_acl.db \
  --limit-per-db 5 \
  --report-json /tmp/real_connector_inventory_with_acl_and_registered_scope_20260624/report.json \
  --report-md /tmp/real_connector_inventory_with_acl_and_registered_scope_20260624/report.md \
  --require-real-non-flink \
  --require-product-acl-row \
  --require-source-scope-negative-row
```

Use `--require-product-safe` only when intentionally asking whether the broader
architecture bar is green. It is expected to fail until real connector
source-ACL ingestion, a real external connector graph candidate with
stale/source-not-attempted source-scope state, and any remaining product-safe
evidence are all present. The current evidence manifest already proves
generated-summary quarantine, relationship conflict/upsert handling, complete
bounded-graph absence-claim proof, a second real non-GitHub domain through PyPI
metadata, real connector-backed partial endpoint/source issue/partial
source-scope gates, and source-scope registration for a planned but
not-attempted lifecycle state. It also proves generic OpenGraph product-row ACL
ingestion: explicit source visibility now writes current `acl_state`,
`acl_policy_key`, and `visibility_hash` onto product rows and evidence. A
derived-real ACL runtime probe covers private high-rank connector row filtering
before traversal and fanout. A derived-real source-scope runtime probe covers
stale and not-attempted bounded-context behavior, but it deliberately does not
satisfy the requirement for a genuinely real stale-window/source-not-attempted
external connector capture.

The real product-safe inventory gates are provenance-aware.
`--require-real-acl-ingestion` requires source-backed ACL metadata on a
candidate relationship or endpoint through a production-like `connector_kind`,
not just any product ACL row plus any `SourceConnection` in the same DB.
Production-like connector kinds are explicit, currently including
`github_app`, `github_rest_api`, `jira_cloud`, `linear_api`, `slack_api`,
`google_drive_api`, `pypi_json_api`, `confluence_cloud`, and `notion_api`.
Fixture replay, source-scope registration, vague connector kinds such as
`github`, and derived runtime probes are intentionally excluded from the hard
real gates.
`--require-real-source-scope-negative` requires the candidate relationship or
endpoint to reference the stale/not-attempted `SourceScopeState` through the
same production-like connector provenance, not merely co-exist with one.

To measure those remaining real-connector blockers directly:

```sh
.venv/bin/python tools/bounded_graph_real_connector_inventory.py \
  --data-root /Users/harsh/workspace/data/flink-pr-jira-1000-plus-500-jira-2026-06-22 \
  --limit-per-db 2 \
  --report-json /tmp/real_connector_inventory_blockers_20260624/report.json \
  --report-md /tmp/real_connector_inventory_blockers_20260624/report.md
```

The hard gates are expected to fail until a real connector capture contains the
missing evidence:

```sh
.venv/bin/python tools/bounded_graph_real_connector_inventory.py \
  --data-root /Users/harsh/workspace/data/flink-pr-jira-1000-plus-500-jira-2026-06-22 \
  --limit-per-db 2 \
  --require-real-acl-ingestion

.venv/bin/python tools/bounded_graph_real_connector_inventory.py \
  --database /Users/harsh/workspace/data/open-graph-acl-ingestion-2026-06-24/open_graph_incident_acl.db \
  --limit-per-db 2 \
  --require-product-acl-row

.venv/bin/python tools/bounded_graph_real_connector_inventory.py \
  --database /Users/harsh/workspace/data/real-source-scope-registration-2026-06-24/not-attempted-source-scope.db \
  --limit-per-db 2 \
  --require-source-scope-negative-row

.venv/bin/python tools/bounded_graph_real_connector_inventory.py \
  --database /Users/harsh/workspace/data/flink-pr-jira-1000-plus-500-jira-2026-06-22/ontology.source-scope-claimable-20260624.db \
  --database /Users/harsh/workspace/data/real-nonflink-open-graph-2026-06-24/real_github_issue_pr_minimum/real_github_issue_pr.db \
  --database /Users/harsh/workspace/data/real-nonflink-open-graph-2026-06-24/real_pypi_project_release_minimum/real_pypi_project_release.db \
  --database /Users/harsh/workspace/data/real-source-scope-registration-2026-06-24/not-attempted-source-scope.db \
  --limit-per-db 5 \
  --require-real-source-scope-negative
```

Raw model readiness is reported as a diagnostic tier, not as a required product
path. Product-facing output is expected to use verifier/repair or deterministic
traversal gates unless raw output independently passes the same eval bar.

To include the real connector MLX/Qwen path:

```sh
RUN_REAL_GITHUB_LLM=1 \
RUN_REAL_CONNECTOR_LLM=1 \
REAL_CONNECTOR_DATABASE=/Users/harsh/workspace/data/flink-pr-jira-1000-plus-500-jira-2026-06-22/ontology.source-scope-claimable-20260624.db \
LLM_MAX_TOKENS=32768 \
LLM_TIMEOUT_SECONDS=1800 \
sh tools/eval_packs/ai_first_bounded_graph_suite/run_eval.sh
```

`RUN_REAL_GITHUB_LLM=1` scores the real non-Flink GitHub OpenGraph pack with
raw, repaired, deterministic traversal, seed-only, and typed-row baselines.
`RUN_REAL_CONNECTOR_LLM=1` does the same for the real Flink/Jira/GitHub typed
connector pack. Raw output is diagnostic. Product-facing output remains
verifier/repair or deterministic-traversal gated.
