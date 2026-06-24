#!/bin/sh
set -eu

SCRIPT_DIR=$(CDPATH= cd -- "$(dirname -- "$0")" && pwd)
ROOT_DIR=$(CDPATH= cd -- "$SCRIPT_DIR/../../.." && pwd)
DATA_DIR=${DATA_DIR:-/Users/harsh/workspace/data/flink-pr-jira-1000-plus-500-jira-2026-06-22}
OUT_DIR=${OUT_DIR:-/tmp/real_connector_negative_partial}
PARTIAL_DB=${PARTIAL_DB:-$DATA_DIR/ontology.ai-tpm-1000-20260622.db}
SOURCE_ISSUE_DB=${SOURCE_ISSUE_DB:-$DATA_DIR/ontology.ai-tpm-1000-open-auth-hydrated-20260622.db}
SOURCE_SCOPE_DB=${SOURCE_SCOPE_DB:-$DATA_DIR/ontology.source-scope-claimable-20260624.db}
SOURCE_AUTHORITY_JSON=${SOURCE_AUTHORITY_JSON:-$ROOT_DIR/internal/graphcontext/source_authority.json}

mkdir -p "$OUT_DIR"
cd "$ROOT_DIR"

partial_context_json="$OUT_DIR/partial_endpoint_context.json"
partial_contract_json="$OUT_DIR/partial_endpoint_contract.json"
partial_promotion_json="$OUT_DIR/partial_endpoint_promotion_audit.json"
source_issue_context_json="$OUT_DIR/source_sync_issue_context.json"
source_issue_contract_json="$OUT_DIR/source_sync_issue_contract.json"
source_issue_promotion_json="$OUT_DIR/source_sync_issue_promotion_audit.json"
source_scope_context_json="$OUT_DIR/source_scope_partial_context.json"
source_scope_contract_json="$OUT_DIR/source_scope_partial_contract.json"
source_scope_promotion_json="$OUT_DIR/source_scope_partial_promotion_audit.json"
report_json="$OUT_DIR/eval.json"

require_db() {
  db=$1
  if [ ! -f "$db" ]; then
    echo "missing required database: $db" >&2
    exit 2
  fi
}

require_db "$PARTIAL_DB"
require_db "$SOURCE_ISSUE_DB"
require_db "$SOURCE_SCOPE_DB"

assert_no_source_issue_body_leakage() {
  context_json=$1
  .venv/bin/python - "$context_json" <<'PY'
import json
import sys

payload = json.load(open(sys.argv[1], encoding="utf-8"))
context = payload.get("boundedGraphContext") or payload.get("data", {}).get("boundedGraphContext") or {}
text = json.dumps(context, sort_keys=True)
for forbidden in [
    "SourceSyncIssue",
    "SourceSyncRun",
    "source_non_200",
    "source_missing_snapshot",
    "source snapshot returned status 401",
    "source snapshot for github_pull_request_files was not captured",
    "https://github.com/apache/flink-kubernetes-operator/pull/998",
    "Authorization",
    "token=",
]:
    if forbidden in text:
        raise SystemExit("negative connector context leaked source diagnostic body/text: " + forbidden)
PY
}

go run ./cmd/ontology-service bounded-graph-context-export \
  --fixture real-connector \
  --database "$PARTIAL_DB" \
  --source-authority-json "$SOURCE_AUTHORITY_JSON" \
  --start-object-type ticket \
  --start-key ticket:jira:FLINK-32695 \
  --association-types implemented_by \
  --depth 1 \
  --limit-per-object 6 \
  --out "$partial_context_json"

assert_no_source_issue_body_leakage "$partial_context_json"

.venv/bin/python tools/bounded_graph_contract.py \
  --bounded-graph-context-json "$partial_context_json" \
  --profile connector \
  --report-json "$partial_contract_json"

.venv/bin/python tools/bounded_graph_promotion_audit.py \
  --bounded-graph-context-json "$partial_context_json" \
  --source-authority-json "$SOURCE_AUTHORITY_JSON" \
  --report-json "$partial_promotion_json"

go run ./cmd/ontology-service bounded-graph-context-export \
  --fixture real-connector \
  --database "$SOURCE_ISSUE_DB" \
  --source-authority-json "$SOURCE_AUTHORITY_JSON" \
  --start-object-type pull_request \
  --start-key pull-request:github:apache/flink-kubernetes-operator#998 \
  --association-types author,approver,implemented_by \
  --depth 1 \
  --limit-per-object 6 \
  --out "$source_issue_context_json"

assert_no_source_issue_body_leakage "$source_issue_context_json"

.venv/bin/python tools/bounded_graph_contract.py \
  --bounded-graph-context-json "$source_issue_context_json" \
  --profile connector \
  --report-json "$source_issue_contract_json"

.venv/bin/python tools/bounded_graph_promotion_audit.py \
  --bounded-graph-context-json "$source_issue_context_json" \
  --source-authority-json "$SOURCE_AUTHORITY_JSON" \
  --report-json "$source_issue_promotion_json"

go run ./cmd/ontology-service bounded-graph-context-export \
  --fixture real-connector \
  --database "$SOURCE_SCOPE_DB" \
  --source-authority-json "$SOURCE_AUTHORITY_JSON" \
  --start-object-type ticket \
  --start-key ticket:jira:FLINK-36332 \
  --association-types implemented_by \
  --depth 1 \
  --limit-per-object 6 \
  --out "$source_scope_context_json"

assert_no_source_issue_body_leakage "$source_scope_context_json"

.venv/bin/python tools/bounded_graph_contract.py \
  --bounded-graph-context-json "$source_scope_context_json" \
  --profile connector \
  --report-json "$source_scope_contract_json"

.venv/bin/python tools/bounded_graph_promotion_audit.py \
  --bounded-graph-context-json "$source_scope_context_json" \
  --source-authority-json "$SOURCE_AUTHORITY_JSON" \
  --report-json "$source_scope_promotion_json"

.venv/bin/python - \
  "$PARTIAL_DB" \
  "$SOURCE_ISSUE_DB" \
  "$SOURCE_SCOPE_DB" \
  "$partial_context_json" \
  "$partial_contract_json" \
  "$partial_promotion_json" \
  "$source_issue_context_json" \
  "$source_issue_contract_json" \
  "$source_issue_promotion_json" \
  "$source_scope_context_json" \
  "$source_scope_contract_json" \
  "$source_scope_promotion_json" \
  "$report_json" <<'PY'
import json
import sqlite3
import sys
from pathlib import Path

(
    partial_db,
    source_issue_db,
    source_scope_db,
    partial_context_path,
    partial_contract_path,
    partial_promotion_path,
    source_issue_context_path,
    source_issue_contract_path,
    source_issue_promotion_path,
    source_scope_context_path,
    source_scope_contract_path,
    source_scope_promotion_path,
    report_path,
) = sys.argv[1:14]


def load_context(path: str) -> dict:
    payload = json.load(open(path, encoding="utf-8"))
    return payload.get("boundedGraphContext") or payload.get("data", {}).get("boundedGraphContext") or payload


def load_json(path: str) -> dict:
    return json.load(open(path, encoding="utf-8"))


def db_scalar(path: str, query: str, args: tuple = ()) -> int:
    with sqlite3.connect(path) as conn:
        row = conn.execute(query, args).fetchone()
    return int(row[0] or 0)


def fail(checks: list[dict], key: str, detail: str) -> None:
    checks.append({"key": key, "passes": False, "detail": detail})


def pass_check(checks: list[dict], key: str, detail: str) -> None:
    checks.append({"key": key, "passes": True, "detail": detail})


checks: list[dict] = []

partial = load_context(partial_context_path)
partial_contract = load_json(partial_contract_path)
partial_promotion = load_json(partial_promotion_path)
partial_associations = partial.get("associations") or []
partial_prs = [row for row in partial.get("objects", []) if row.get("objectType") == "pull_request"]
if partial_contract.get("passes_contract"):
    pass_check(checks, "partial_endpoint_contract", "connector contract passes for real partial endpoint context")
else:
    fail(checks, "partial_endpoint_contract", "connector contract failed")
if len(partial_associations) == 6 and all(not row.get("claimAllowed") and row.get("claimGateReason") == "relationship_endpoint_partial_requires_hydration" for row in partial_associations):
    pass_check(checks, "partial_endpoint_relationships_gated", "six real ticket-PR relationships are visible but gated by partial PR endpoints")
else:
    fail(checks, "partial_endpoint_relationships_gated", "expected six non-claimable partial-endpoint relationships")
if len(partial_prs) == 6 and all(not row.get("claimAllowed") and row.get("claimGateReason") == "object_partial_requires_hydration" and row.get("freshnessState") == "partial" for row in partial_prs):
    pass_check(checks, "partial_endpoint_objects_gated", "six real partial PR endpoint objects are non-claimable")
else:
    fail(checks, "partial_endpoint_objects_gated", "expected six partial PR objects with hydration gate")
if partial_promotion.get("blocked_association_count") == 6 and partial_promotion.get("promotable_association_count") == 0 and partial_promotion.get("blocked_object_count") == 6:
    pass_check(checks, "partial_endpoint_promotion_audit", "promotion audit blocks all partial endpoint relationships and PR objects")
else:
    fail(checks, "partial_endpoint_promotion_audit", "promotion audit did not block the expected partial rows")
coverage = partial.get("coverage") or {}
if coverage.get("absenceClaimsAllowed") is False and coverage.get("absenceClaimGateReason") == "source_coverage_gate":
    pass_check(checks, "partial_endpoint_absence_gated", "partial endpoint context does not support absence claims")
else:
    fail(checks, "partial_endpoint_absence_gated", "partial endpoint coverage should keep absence claims gated")

source_issue = load_context(source_issue_context_path)
source_issue_contract = load_json(source_issue_contract_path)
source_issue_promotion = load_json(source_issue_promotion_path)
source_issue_counts = {
    "source_non_200": db_scalar(
        source_issue_db,
        """
        select count(*) from source_sync_issues
        where source_system='github'
          and source_instance='github.com/apache/flink-kubernetes-operator'
          and external_kind='github_pull_request_files'
          and external_id='apache/flink-kubernetes-operator#998'
          and issue_code='source_non_200'
          and message like '%status 401%'
        """,
    ),
    "source_missing_snapshot": db_scalar(
        source_issue_db,
        """
        select count(*) from source_sync_issues
        where source_system='github'
          and source_instance='github.com/apache/flink-kubernetes-operator'
          and external_kind='github_pull_request_files'
          and external_id='apache/flink-kubernetes-operator#998'
          and issue_code='source_missing_snapshot'
        """,
    ),
}
if source_issue_contract.get("passes_contract"):
    pass_check(checks, "source_issue_contract", "connector contract passes for real source-sync issue context")
else:
    fail(checks, "source_issue_contract", "source-sync issue connector contract failed")
coverage = source_issue.get("coverage") or {}
if coverage.get("coverageState") == "limited" and coverage.get("absenceClaimsAllowed") is False and coverage.get("absenceClaimGateReason") == "source_sync_issue":
    pass_check(checks, "source_issue_absence_gated", "real source-sync issues force limited coverage and block absence claims")
else:
    fail(checks, "source_issue_absence_gated", "source-sync issue coverage should be limited and absence-gated")
summary = coverage.get("summary") or ""
if "2 source sync issue(s)" in summary and "Raw sync issue bodies and source URLs are coverage evidence only, not prompt facts." in summary:
    pass_check(checks, "source_issue_summary_sanitized", "context exposes sanitized source issue counts and guardrail text")
else:
    fail(checks, "source_issue_summary_sanitized", "source issue summary did not expose the sanitized count/guardrail")
if source_issue_counts["source_non_200"] == 1 and source_issue_counts["source_missing_snapshot"] == 1:
    pass_check(checks, "source_issue_db_evidence", "real DB contains one 401 non-200 issue and one missing-snapshot issue for PR #998 files")
else:
    fail(checks, "source_issue_db_evidence", f"unexpected source issue DB counts {source_issue_counts}")
context_text = json.dumps(source_issue, sort_keys=True)
if "source snapshot returned status 401" not in context_text and "https://github.com/apache/flink-kubernetes-operator/pull/998" not in context_text:
    pass_check(checks, "source_issue_raw_body_not_prompted", "raw source issue body and source URL do not appear in bounded context")
else:
    fail(checks, "source_issue_raw_body_not_prompted", "raw source issue body or URL leaked into bounded context")
source_issue_associations = source_issue.get("associations") or []
author_edges = [row for row in source_issue_associations if row.get("associationType") == "author"]
approver_edges = [row for row in source_issue_associations if row.get("associationType") == "approver"]
implemented_edges = [row for row in source_issue_associations if row.get("associationType") == "implemented_by"]
if len(author_edges) == 1 and author_edges[0].get("claimAllowed") is False and author_edges[0].get("claimGateReason") == "relationship_locator_not_authoritative_for_presence":
    pass_check(checks, "source_issue_author_authority_gated", "real author edge stays non-claimable because locator kind is not authoritative for author presence")
else:
    fail(checks, "source_issue_author_authority_gated", "expected one non-claimable author edge with source-authority locator gate")
if len(approver_edges) == 2 and all(row.get("claimAllowed") is True and row.get("claimGateReason") == "source_evidence_full_confidence" for row in approver_edges):
    pass_check(checks, "source_issue_approver_edges_preserved", "two real GitHub approver edges remain claimable under limited source coverage")
else:
    fail(checks, "source_issue_approver_edges_preserved", "expected two claimable approver edges")
if len(implemented_edges) == 1 and implemented_edges[0].get("claimAllowed") is True and implemented_edges[0].get("claimGateReason") == "source_evidence_full_confidence":
    pass_check(checks, "source_issue_implemented_by_preserved", "positive Jira-backed ticket-PR relationship remains claimable while coverage remains limited")
else:
    fail(checks, "source_issue_implemented_by_preserved", "expected one claimable implemented_by edge")
if source_issue_promotion.get("promotable_association_count") == 3 and source_issue_promotion.get("blocked_association_count") == 1:
    pass_check(checks, "source_issue_promotion_audit", "promotion audit preserves three positive edges and blocks one non-authoritative author edge")
else:
    fail(checks, "source_issue_promotion_audit", "source issue promotion audit did not return the expected 3 promotable / 1 blocked associations")

source_scope = load_context(source_scope_context_path)
source_scope_contract = load_json(source_scope_contract_path)
source_scope_promotion = load_json(source_scope_promotion_path)
if source_scope_contract.get("passes_contract"):
    pass_check(checks, "source_scope_contract", "connector contract passes for real partial source-scope context")
else:
    fail(checks, "source_scope_contract", "partial source-scope connector contract failed")
coverage = source_scope.get("coverage") or {}
if coverage.get("coverageState") == "limited" and coverage.get("absenceClaimsAllowed") is False and coverage.get("absenceClaimGateReason") == "source_scope_not_exact" and "coverage_mode=partial_scope" in (coverage.get("summary") or ""):
    pass_check(checks, "source_scope_partial_absence_gated", "real partial source scope gates absence claims with source_scope_not_exact")
else:
    fail(checks, "source_scope_partial_absence_gated", "partial source scope should be limited and absence-gated")
if source_scope_promotion.get("promotable_association_count") == 4 and source_scope_promotion.get("blocked_association_count") == 0:
    pass_check(checks, "source_scope_positive_edges_preserved", "four positive remote-link relationships remain claimable under partial source scope")
else:
    fail(checks, "source_scope_positive_edges_preserved", "partial source-scope case should preserve four positive claimable edges")
scope_state_count = db_scalar(
    source_scope_db,
    """
    select count(*)
    from source_scope_states ss
    join source_scopes sc on sc.id=ss.source_scope_id
    join source_connections co on co.id=sc.source_connection_id
    where co.source_system in ('jira', 'github')
      and ss.freshness_state='fresh'
      and ss.coverage_mode='partial_scope'
      and ss.last_attempted_at is not null
    """,
)
if scope_state_count >= 2:
    pass_check(checks, "source_scope_db_evidence", "real DB contains fresh partial_scope source states with attempted timestamps")
else:
    fail(checks, "source_scope_db_evidence", "expected real partial_scope source states in source-scope DB")

passed = sum(1 for row in checks if row["passes"])
failed = len(checks) - passed
report = {
    "passes_eval": failed == 0,
    "passes_smoke_eval": failed == 0,
    "golden_eval": {
        "pass_count": passed,
        "failure_count": failed,
        "question_count": len(checks),
        "passes_golden_eval": failed == 0,
        "questions": checks,
    },
    "cases": {
        "partial_endpoint": {
            "database": partial_db,
            "context": partial_context_path,
            "contract": partial_contract_path,
            "promotion_audit": partial_promotion_path,
            "seed": "ticket:jira:FLINK-32695",
        },
        "source_sync_issue": {
            "database": source_issue_db,
            "context": source_issue_context_path,
            "contract": source_issue_contract_path,
            "promotion_audit": source_issue_promotion_path,
            "seed": "pull-request:github:apache/flink-kubernetes-operator#998",
            "source_issue_counts": source_issue_counts,
        },
        "source_scope_partial": {
            "database": source_scope_db,
            "context": source_scope_context_path,
            "contract": source_scope_contract_path,
            "promotion_audit": source_scope_promotion_path,
            "seed": "ticket:jira:FLINK-36332",
            "partial_scope_state_count": scope_state_count,
        },
    },
}
Path(report_path).write_text(json.dumps(report, indent=2, sort_keys=True), encoding="utf-8")
print(
    "real_connector_negative_partial",
    "passes_eval=" + str(report["passes_eval"]),
    "golden=" + str(passed) + "/" + str(len(checks)),
)
for row in checks:
    print("real_connector_negative_partial", row["key"], "passes=" + str(row["passes"]))
if failed:
    raise SystemExit("real connector negative/partial eval failed")
PY
