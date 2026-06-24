#!/bin/sh
set -eu

SCRIPT_DIR=$(CDPATH= cd -- "$(dirname -- "$0")" && pwd)
ROOT_DIR=$(CDPATH= cd -- "$SCRIPT_DIR/../../.." && pwd)
DATA_DIR=${DATA_DIR:-/Users/harsh/workspace/data/flink-pr-jira-1000-plus-500-jira-2026-06-22}
OUT_DIR=${OUT_DIR:-/tmp/real_connector_source_scope_runtime}
SOURCE_SCOPE_DB=${SOURCE_SCOPE_DB:-$DATA_DIR/ontology.source-scope-claimable-20260624.db}
SOURCE_AUTHORITY_JSON=${SOURCE_AUTHORITY_JSON:-$ROOT_DIR/internal/graphcontext/source_authority.json}

mkdir -p "$OUT_DIR"
cd "$ROOT_DIR"

stale_db="$OUT_DIR/source_scope_stale_probe.db"
not_attempted_db="$OUT_DIR/source_scope_not_attempted_probe.db"
stale_context_json="$OUT_DIR/stale_context.json"
not_attempted_context_json="$OUT_DIR/not_attempted_context.json"
stale_contract_json="$OUT_DIR/stale_contract.json"
not_attempted_contract_json="$OUT_DIR/not_attempted_contract.json"
stale_promotion_json="$OUT_DIR/stale_promotion_audit.json"
report_json="$OUT_DIR/eval.json"

if [ ! -f "$SOURCE_SCOPE_DB" ]; then
  echo "missing required database: $SOURCE_SCOPE_DB" >&2
  exit 2
fi

cp "$SOURCE_SCOPE_DB" "$stale_db"
cp "$SOURCE_SCOPE_DB" "$not_attempted_db"

sqlite3 "$stale_db" <<'SQL'
update source_scopes
set crawl_policy = 'fixture_replay bounded_graph_absence=implemented_by'
where id = 3;

update source_scope_states
set freshness_state = 'stale',
    coverage_mode = 'exact_scope',
    last_attempted_at = coalesce(last_attempted_at, '2026-06-24T15:37:50Z'),
    error_code = 'derived_real_stale_scope',
    error_message = 'Derived-real stale source-scope runtime probe.'
where id = 2;
SQL

sqlite3 "$not_attempted_db" <<'SQL'
update source_scopes
set crawl_policy = 'fixture_replay bounded_graph_absence=implemented_by'
where id = 3;

update source_scope_states
set freshness_state = 'unknown',
    coverage_mode = 'unknown',
    last_attempted_at = null,
    last_successful_at = null,
    last_successful_sync_run_id = null,
    error_code = 'derived_real_source_not_attempted',
    error_message = 'Derived-real source-not-attempted runtime probe.'
where id = 2;
SQL

go run ./cmd/ontology-service bounded-graph-context-export \
  --fixture real-connector \
  --database "$stale_db" \
  --source-authority-json "$SOURCE_AUTHORITY_JSON" \
  --principal-key principal:coverage-owner \
  --principal-coverage-complete \
  --start-object-type ticket \
  --start-key ticket:jira:FLINK-36332 \
  --association-types implemented_by \
  --depth 1 \
  --limit-per-object 6 \
  --out "$stale_context_json"

go run ./cmd/ontology-service bounded-graph-context-export \
  --fixture real-connector \
  --database "$not_attempted_db" \
  --source-authority-json "$SOURCE_AUTHORITY_JSON" \
  --principal-key principal:coverage-owner \
  --principal-coverage-complete \
  --start-object-type ticket \
  --start-key ticket:jira:FLINK-36332 \
  --association-types implemented_by \
  --depth 1 \
  --limit-per-object 6 \
  --out "$not_attempted_context_json"

.venv/bin/python tools/bounded_graph_contract.py \
  --bounded-graph-context-json "$stale_context_json" \
  --profile connector \
  --report-json "$stale_contract_json"

.venv/bin/python tools/bounded_graph_contract.py \
  --bounded-graph-context-json "$not_attempted_context_json" \
  --profile connector \
  --report-json "$not_attempted_contract_json" > "$OUT_DIR/not_attempted_contract.log" 2>&1 || true

.venv/bin/python tools/bounded_graph_promotion_audit.py \
  --bounded-graph-context-json "$stale_context_json" \
  --source-authority-json "$SOURCE_AUTHORITY_JSON" \
  --report-json "$stale_promotion_json"

.venv/bin/python - \
  "$DATA_DIR" \
  "$stale_db" \
  "$not_attempted_db" \
  "$stale_context_json" \
  "$not_attempted_context_json" \
  "$stale_contract_json" \
  "$not_attempted_contract_json" \
  "$stale_promotion_json" \
  "$report_json" <<'PY'
import json
import sqlite3
import sys
from pathlib import Path

(
    data_dir,
    stale_db,
    not_attempted_db,
    stale_context_path,
    not_attempted_context_path,
    stale_contract_path,
    not_attempted_contract_path,
    stale_promotion_path,
    report_path,
) = sys.argv[1:10]


def load_context(path: str) -> dict:
    payload = json.load(open(path, encoding="utf-8"))
    return payload.get("boundedGraphContext") or payload.get("data", {}).get("boundedGraphContext") or payload


def load_json(path: str) -> dict:
    return json.load(open(path, encoding="utf-8"))


def table_exists(conn: sqlite3.Connection, name: str) -> bool:
    row = conn.execute("select 1 from sqlite_master where type='table' and name=?", (name,)).fetchone()
    return row is not None


def scalar(db_path: str, query: str, args: tuple = ()) -> int:
    with sqlite3.connect(db_path) as conn:
        row = conn.execute(query, args).fetchone()
    return int(row[0] or 0)


def current_real_scope_inventory(data_root: str) -> dict:
    total = 0
    stale_or_not_attempted = 0
    fresh_partial = 0
    dbs_with_states = 0
    for db_path in sorted(Path(data_root).glob("ontology*.db")):
        with sqlite3.connect(db_path) as conn:
            if not table_exists(conn, "source_scope_states"):
                continue
            rows = conn.execute(
                """
                select freshness_state, coverage_mode, last_attempted_at, last_successful_at
                from source_scope_states
                """
            ).fetchall()
        if rows:
            dbs_with_states += 1
        for freshness, coverage, attempted, successful in rows:
            total += 1
            if freshness == "fresh" and coverage == "partial_scope":
                fresh_partial += 1
            if freshness != "fresh" or attempted is None:
                stale_or_not_attempted += 1
    return {
        "database_count_with_source_scope_states": dbs_with_states,
        "source_scope_state_count": total,
        "fresh_partial_count": fresh_partial,
        "stale_or_not_attempted_count": stale_or_not_attempted,
    }


def pass_check(checks: list[dict], key: str, detail: str) -> None:
    checks.append({"key": key, "passes": True, "detail": detail})


def fail(checks: list[dict], key: str, detail: str) -> None:
    checks.append({"key": key, "passes": False, "detail": detail})


checks: list[dict] = []
stale = load_context(stale_context_path)
not_attempted = load_context(not_attempted_context_path)
stale_contract = load_json(stale_contract_path)
not_attempted_contract = load_json(not_attempted_contract_path)
stale_promotion = load_json(stale_promotion_path)

inventory = current_real_scope_inventory(data_dir)
if inventory["source_scope_state_count"] >= 2 and inventory["fresh_partial_count"] >= 2 and inventory["stale_or_not_attempted_count"] == 0:
    pass_check(checks, "real_source_scope_gap_inventory", "current real source-scope rows are fresh partial_scope only; stale/not-attempted remains unavailable")
else:
    fail(checks, "real_source_scope_gap_inventory", f"unexpected current real source-scope inventory {inventory}")

mutation_counts = {
    "stale_state": scalar(stale_db, "select count(*) from source_scope_states where id=2 and freshness_state='stale' and coverage_mode='exact_scope' and last_attempted_at is not null"),
    "not_attempted_state": scalar(not_attempted_db, "select count(*) from source_scope_states where id=2 and freshness_state='unknown' and coverage_mode='unknown' and last_attempted_at is null and last_successful_at is null and last_successful_sync_run_id is null"),
}
if all(value == 1 for value in mutation_counts.values()):
    pass_check(checks, "derived_real_source_scope_mutations_applied", "derived DBs contain one stale exact-scope state and one not-attempted unknown state")
else:
    fail(checks, "derived_real_source_scope_mutations_applied", f"unexpected mutation counts {mutation_counts}")

coverage = stale.get("coverage") or {}
if coverage.get("coverageState") == "limited" and coverage.get("absenceClaimsAllowed") is False and coverage.get("absenceClaimGateReason") == "source_scope_not_fresh":
    pass_check(checks, "stale_source_scope_absence_gated", "stale exact source scope gates absence claims with source_scope_not_fresh")
else:
    fail(checks, "stale_source_scope_absence_gated", f"unexpected stale coverage {coverage}")

summary = coverage.get("summary") or ""
if "freshness=stale" in summary and "coverage_mode=exact_scope" in summary:
    pass_check(checks, "stale_source_scope_summary_precise", "stale coverage summary exposes state and mode without raw source diagnostics")
else:
    fail(checks, "stale_source_scope_summary_precise", f"stale summary missing state/mode: {summary!r}")

if coverage.get("coverageWindowStart") and coverage.get("coverageWindowEnd"):
    pass_check(checks, "stale_source_scope_window_present", "stale attempted timestamp is retained as a bounded observation window")
else:
    fail(checks, "stale_source_scope_window_present", "stale context should retain attempted observation window")

stale_associations = stale.get("associations") or []
if len(stale_associations) == 4 and all(row.get("claimAllowed") is True and row.get("claimGateReason") == "source_evidence_full_confidence" for row in stale_associations):
    pass_check(checks, "stale_positive_edges_preserved", "stale source coverage blocks absence but does not erase positive claimable remote-link edges")
else:
    fail(checks, "stale_positive_edges_preserved", "expected four positive claimable relationships in stale context")

if stale_contract.get("passes_contract"):
    pass_check(checks, "stale_contract_passes", "stale source-scope context remains prompt-contract clean")
else:
    fail(checks, "stale_contract_passes", "stale connector contract failed")

if stale_promotion.get("promotable_association_count") == 4 and stale_promotion.get("blocked_association_count") == 0:
    pass_check(checks, "stale_promotion_audit_preserves_positive_edges", "promotion audit keeps four positive relationship facts promotable")
else:
    fail(checks, "stale_promotion_audit_preserves_positive_edges", "stale promotion audit should preserve four positive relationships")

coverage = not_attempted.get("coverage") or {}
if coverage.get("coverageState") == "limited" and coverage.get("absenceClaimsAllowed") is False and coverage.get("absenceClaimGateReason") == "source_scope_not_fresh":
    pass_check(checks, "not_attempted_absence_gated", "not-attempted source scope gates absence claims")
else:
    fail(checks, "not_attempted_absence_gated", f"unexpected not-attempted coverage {coverage}")

if not coverage.get("coverageWindowStart") and not coverage.get("coverageWindowEnd"):
    pass_check(checks, "not_attempted_has_no_fake_window", "not-attempted source scope does not fabricate a coverage window")
else:
    fail(checks, "not_attempted_has_no_fake_window", "not-attempted context should not invent coverage windows")

not_attempted_warnings = not_attempted_contract.get("warnings") or []
if not not_attempted_contract.get("passes_contract") and any(row.get("kind") == "connector_source_scope_missing" and "coverageWindow" in row.get("path", "") for row in not_attempted_warnings):
    pass_check(checks, "not_attempted_contract_blocks_prompt_ready_claims", "not-attempted context is not connector prompt-contract ready without coverage windows")
else:
    fail(checks, "not_attempted_contract_blocks_prompt_ready_claims", "not-attempted contract should fail on missing coverage window")

text = json.dumps({"stale": stale, "not_attempted": not_attempted}, sort_keys=True)
forbidden = ["Derived-real stale source-scope runtime probe.", "Derived-real source-not-attempted runtime probe.", "SourceSyncRun", "SourceSyncIssue", "error_message"]
hits = [value for value in forbidden if value in text]
if not hits:
    pass_check(checks, "source_scope_diagnostics_not_prompted", "source-scope error details stay out of bounded context")
else:
    fail(checks, "source_scope_diagnostics_not_prompted", "context leaked diagnostic details: " + ", ".join(hits))

passed = sum(1 for row in checks if row["passes"])
failed = len(checks) - passed
report = {
    "passes_eval": failed == 0,
    "passes_smoke_eval": failed == 0,
    "scope": "derived_real_connector_source_scope_runtime_probe",
    "real_stale_or_not_attempted_source_scope_proven": False,
    "current_real_source_scope_inventory": inventory,
    "mutation_counts": mutation_counts,
    "golden_eval": {
        "pass_count": passed,
        "failure_count": failed,
        "question_count": len(checks),
        "passes_golden_eval": failed == 0,
        "questions": checks,
    },
    "cases": {
        "stale": {
            "database": stale_db,
            "context": stale_context_path,
            "contract": stale_contract_path,
            "promotion_audit": stale_promotion_path,
            "seed": "ticket:jira:FLINK-36332",
        },
        "not_attempted": {
            "database": not_attempted_db,
            "context": not_attempted_context_path,
            "contract": not_attempted_contract_path,
            "seed": "ticket:jira:FLINK-36332",
            "prompt_contract_ready": False,
        },
    },
}
Path(report_path).write_text(json.dumps(report, indent=2, sort_keys=True), encoding="utf-8")
print(
    "real_connector_source_scope_runtime",
    "passes_eval=" + str(report["passes_eval"]),
    "golden=" + str(passed) + "/" + str(len(checks)),
)
for row in checks:
    print("real_connector_source_scope_runtime", row["key"], "passes=" + str(row["passes"]))
if failed:
    raise SystemExit("real connector source-scope runtime eval failed")
PY
