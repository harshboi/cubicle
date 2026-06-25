#!/bin/sh
set -eu

SCRIPT_DIR=$(CDPATH= cd -- "$(dirname -- "$0")" && pwd)
ROOT_DIR=$(CDPATH= cd -- "$SCRIPT_DIR/../../.." && pwd)
OUT_DIR=${OUT_DIR:-/tmp/real_pypi_source_scope_stale}
BASE_FIXTURE_JSON="$ROOT_DIR/tools/eval_packs/real_pypi_project_release_minimum/open_graph_fixture.json"
SOURCE_AUTHORITY_JSON="$ROOT_DIR/tools/eval_packs/real_pypi_project_release_minimum/source_authority.json"

mkdir -p "$OUT_DIR"
cd "$ROOT_DIR"

fixture_json="$OUT_DIR/stale_open_graph_fixture.json"
db="$OUT_DIR/real_pypi_source_scope_stale.db"
context_json="$OUT_DIR/context.json"
contract_json="$OUT_DIR/contract.json"
promotion_json="$OUT_DIR/promotion_audit.json"
inventory_json="$OUT_DIR/inventory.json"
inventory_md="$OUT_DIR/inventory.md"
eval_json="$OUT_DIR/eval.json"
load_json="$OUT_DIR/load_summary.json"

rm -f "$db" "$db-wal" "$db-shm" "$db-journal"

.venv/bin/python - "$BASE_FIXTURE_JSON" "$fixture_json" <<'PY'
import json
import sys

src, dst = sys.argv[1:3]
payload = json.load(open(src, encoding="utf-8"))
payload["observedAt"] = "2026-06-20T16:53:04Z"
scope = payload["sourceScope"]
scope["freshnessState"] = "stale"
scope["coverageMode"] = "partial_scope"
scope["status"] = "complete"
scope["runKey"] = "source-sync-run:pypi:project:requests:20260620T165304Z"
scope["startedAt"] = "2026-06-20T16:53:04Z"
scope["completedAt"] = "2026-06-20T16:53:04Z"
scope["coverageStartAt"] = "2026-06-20T16:53:04Z"
scope["coverageEndAt"] = "2026-06-20T16:53:04Z"
open(dst, "w", encoding="utf-8").write(json.dumps(payload, indent=2, sort_keys=True))
PY

go run ./cmd/ontology-service open-graph-fixture-load \
  --fixture-json "$fixture_json" \
  --database "$db" \
  --reset-database \
  > "$load_json"

go run ./cmd/ontology-service bounded-graph-context-export \
  --fixture open-graph \
  --database "$db" \
  --source-authority-json "$SOURCE_AUTHORITY_JSON" \
  --start-object-type pypi_project \
  --start-key pypi:project:requests \
  --association-types has_release,has_distribution,has_contact \
  --depth 2 \
  --limit-per-object 4 \
  --out "$context_json"

.venv/bin/python tools/bounded_graph_contract.py \
  --bounded-graph-context-json "$context_json" \
  --report-json "$contract_json"

.venv/bin/python tools/bounded_graph_promotion_audit.py \
  --bounded-graph-context-json "$context_json" \
  --source-authority-json "$SOURCE_AUTHORITY_JSON" \
  --report-json "$promotion_json"

.venv/bin/python tools/bounded_graph_real_connector_inventory.py \
  --database "$db" \
  --limit-per-db 5 \
  --require-real-source-scope-negative \
  --report-json "$inventory_json" \
  --report-md "$inventory_md"

.venv/bin/python - \
  "$db" \
  "$context_json" \
  "$contract_json" \
  "$promotion_json" \
  "$inventory_json" \
  "$load_json" \
  "$eval_json" <<'PY'
import json
import sqlite3
import sys
from pathlib import Path

db, context_path, contract_path, promotion_path, inventory_path, load_path, eval_path = sys.argv[1:8]


def load_json(path: str) -> dict:
    return json.load(open(path, encoding="utf-8"))


def context(path: str) -> dict:
    payload = load_json(path)
    return payload.get("boundedGraphContext") or payload.get("data", {}).get("boundedGraphContext") or payload


def scalar(conn: sqlite3.Connection, query: str) -> int:
    return int(conn.execute(query).fetchone()[0] or 0)


def pass_check(checks: list[dict], key: str, detail: str) -> None:
    checks.append({"key": key, "passes": True, "detail": detail})


def fail(checks: list[dict], key: str, detail: str) -> None:
    checks.append({"key": key, "passes": False, "detail": detail})


checks: list[dict] = []
ctx = context(context_path)
contract = load_json(contract_path)
promotion = load_json(promotion_path)
inventory = load_json(inventory_path)
load_summary = load_json(load_path)

with sqlite3.connect(db) as conn:
    state_row = conn.execute(
        """
        select freshness_state, coverage_mode, last_attempted_at is not null, last_successful_sync_run_id is not null
        from source_scope_states
        """
    ).fetchone()
    connector_kind = conn.execute("select connector_kind from source_connections").fetchone()[0]
    run_row = conn.execute(
        """
        select status, coverage_mode, objects_seen_count, objects_created_count,
               relationships_created_count, evidence_created_count
        from source_sync_runs
        """
    ).fetchone()
    scoped_counts = {
        "objects": scalar(conn, "select count(*) from open_graph_objects where source_scope_state_id is not null"),
        "associations": scalar(conn, "select count(*) from open_graph_associations where source_scope_state_id is not null"),
        "evidence": scalar(conn, "select count(*) from evidences where source_scope_state_id is not null"),
    }

if load_summary.get("openGraphFixtureLoad") == {"objectCount": 5, "associationCount": 4, "evidenceCount": 4}:
    pass_check(checks, "fixture_loaded_expected_rows", "PyPI stale replay loaded 5 objects, 4 associations, and 4 evidence rows")
else:
    fail(checks, "fixture_loaded_expected_rows", f"unexpected load summary {load_summary}")

if connector_kind == "pypi_json_api":
    pass_check(checks, "production_like_connector_kind", "source connection uses pypi_json_api")
else:
    fail(checks, "production_like_connector_kind", f"unexpected connector kind {connector_kind!r}")

if state_row == ("stale", "partial_scope", 1, 1):
    pass_check(checks, "stale_source_scope_state", "source scope state is stale partial_scope with attempted and successful run references")
else:
    fail(checks, "stale_source_scope_state", f"unexpected state row {state_row}")

if run_row == ("complete", "partial_scope", 5, 5, 4, 4):
    pass_check(checks, "source_sync_run_counters", "sync run records complete partial-scope snapshot counters")
else:
    fail(checks, "source_sync_run_counters", f"unexpected run row {run_row}")

if scoped_counts == {"objects": 5, "associations": 4, "evidence": 4}:
    pass_check(checks, "graph_rows_reference_scope_state", "all OpenGraph product and evidence rows reference the stale source scope state")
else:
    fail(checks, "graph_rows_reference_scope_state", f"unexpected scoped counts {scoped_counts}")

db_report = (inventory.get("databases") or [{}])[0]
if inventory.get("passes_real_source_scope_negative_requirement") and db_report.get("production_source_scope_negative_candidate_count") == 4:
    pass_check(checks, "inventory_hard_gate_passes", "inventory sees four production-like source-scope negative candidate relationships")
else:
    fail(checks, "inventory_hard_gate_passes", f"unexpected inventory {inventory}")

coverage = ctx.get("coverage") or {}
if coverage.get("absenceClaimsAllowed") is False and coverage.get("coverageState") == "sparse":
    pass_check(checks, "opengraph_absence_still_sparse", "OpenGraph export does not infer absence coverage from the stale source scope")
else:
    fail(checks, "opengraph_absence_still_sparse", f"unexpected coverage {coverage}")

text = json.dumps(ctx, sort_keys=True)
forbidden = ["SourceSyncRun", "SourceSyncIssue", "source_scope_states", "error_message", "pypi_json_api"]
hits = [value for value in forbidden if value in text]
if not hits:
    pass_check(checks, "source_diagnostics_not_prompted", "source diagnostics and connector internals do not leak into bounded context")
else:
    fail(checks, "source_diagnostics_not_prompted", "context leaked " + ", ".join(hits))

if contract.get("passes_contract"):
    pass_check(checks, "bounded_context_contract_passes", "bounded graph contract still passes")
else:
    fail(checks, "bounded_context_contract_passes", f"contract failed {contract}")

if promotion.get("promotable_association_count") == 4 and promotion.get("blocked_association_count") == 0:
    pass_check(checks, "positive_edges_remain_promotable", "stale source scope does not erase positive PyPI relationship facts")
else:
    fail(checks, "positive_edges_remain_promotable", f"unexpected promotion audit {promotion}")

passed = sum(1 for row in checks if row["passes"])
failed = len(checks) - passed
report = {
    "passes_eval": failed == 0,
    "passes_smoke_eval": failed == 0,
    "scope": "real_pypi_source_scope_stale",
    "real_stale_or_not_attempted_source_scope_proven": failed == 0,
    "database": db,
    "context": context_path,
    "inventory": inventory_path,
    "golden_eval": {
        "pass_count": passed,
        "failure_count": failed,
        "question_count": len(checks),
        "passes_golden_eval": failed == 0,
        "questions": checks,
    },
}
Path(eval_path).write_text(json.dumps(report, indent=2, sort_keys=True), encoding="utf-8")
print("real_pypi_source_scope_stale passes_eval=" + str(report["passes_eval"]), "golden=" + str(passed) + "/" + str(len(checks)))
for row in checks:
    print("real_pypi_source_scope_stale", row["key"], "passes=" + str(row["passes"]))
if failed:
    raise SystemExit("real PyPI stale source-scope eval failed")
PY
