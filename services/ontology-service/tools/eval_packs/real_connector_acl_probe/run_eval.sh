#!/bin/sh
set -eu

SCRIPT_DIR=$(CDPATH= cd -- "$(dirname -- "$0")" && pwd)
ROOT_DIR=$(CDPATH= cd -- "$SCRIPT_DIR/../../.." && pwd)
DATA_DIR=${DATA_DIR:-/Users/harsh/workspace/data/flink-pr-jira-1000-plus-500-jira-2026-06-22}
OUT_DIR=${OUT_DIR:-/tmp/real_connector_acl_probe}
SOURCE_DB=${SOURCE_DB:-$DATA_DIR/ontology.ai-tpm-1000-open-auth-hydrated-20260622.db}
SOURCE_AUTHORITY_JSON=${SOURCE_AUTHORITY_JSON:-$ROOT_DIR/internal/graphcontext/source_authority.json}

mkdir -p "$OUT_DIR"
cd "$ROOT_DIR"

probe_db="$OUT_DIR/real_connector_acl_probe.db"
public_context_json="$OUT_DIR/public_context.json"
private_context_json="$OUT_DIR/private_allowed_context.json"
public_contract_json="$OUT_DIR/public_contract.json"
private_contract_json="$OUT_DIR/private_allowed_contract.json"
public_promotion_json="$OUT_DIR/public_promotion_audit.json"
private_promotion_json="$OUT_DIR/private_allowed_promotion_audit.json"
report_json="$OUT_DIR/eval.json"

if [ ! -f "$SOURCE_DB" ]; then
  echo "missing required database: $SOURCE_DB" >&2
  exit 2
fi

cp "$SOURCE_DB" "$probe_db"

sqlite3 "$probe_db" <<'SQL'
update pull_requests
set visibility = 'private',
    acl_state = 'current',
    acl_policy_key = 'derived-real-acl-probe:private-reader',
    visibility_hash = 'derived-real-acl-probe-private'
where key = 'pull-request:github:apache/flink-kubernetes-operator#909';

update ticket_pull_requests
set visibility = 'private',
    acl_state = 'current',
    acl_policy_key = 'derived-real-acl-probe:private-reader',
    visibility_hash = 'derived-real-acl-probe-private',
    rank_score = 100
where id = 880;

update evidences
set visibility = 'private',
    acl_state = 'current',
    acl_policy_key = 'derived-real-acl-probe:private-reader',
    visibility_hash = 'derived-real-acl-probe-private',
    acl_policy_key_snapshot = 'derived-real-acl-probe:private-reader',
    visibility_hash_snapshot = 'derived-real-acl-probe-private'
where id = (select latest_evidence_id from ticket_pull_requests where id = 880);

update ticket_pull_requests
set rank_score = 10
where id = 877;
SQL

go run ./cmd/ontology-service bounded-graph-context-export \
  --fixture real-connector \
  --database "$probe_db" \
  --source-authority-json "$SOURCE_AUTHORITY_JSON" \
  --principal-key principal:public-only \
  --start-object-type ticket \
  --start-key ticket:jira:FLINK-36332 \
  --association-types implemented_by \
  --depth 1 \
  --limit-per-object 1 \
  --coverage-state sparse \
  --absence-claim-gate-reason derived_real_acl_probe \
  --coverage-summary "Derived-real connector ACL probe; source ACL ingestion remains unproven." \
  --out "$public_context_json"

go run ./cmd/ontology-service bounded-graph-context-export \
  --fixture real-connector \
  --database "$probe_db" \
  --source-authority-json "$SOURCE_AUTHORITY_JSON" \
  --principal-key principal:private-reader \
  --allowed-visibility-classes private \
  --start-object-type ticket \
  --start-key ticket:jira:FLINK-36332 \
  --association-types implemented_by \
  --depth 1 \
  --limit-per-object 1 \
  --coverage-state sparse \
  --absence-claim-gate-reason derived_real_acl_probe \
  --coverage-summary "Derived-real connector ACL probe; source ACL ingestion remains unproven." \
  --out "$private_context_json"

.venv/bin/python tools/bounded_graph_contract.py \
  --bounded-graph-context-json "$public_context_json" \
  --profile connector \
  --report-json "$public_contract_json"

.venv/bin/python tools/bounded_graph_contract.py \
  --bounded-graph-context-json "$private_context_json" \
  --profile connector \
  --report-json "$private_contract_json"

.venv/bin/python tools/bounded_graph_promotion_audit.py \
  --bounded-graph-context-json "$public_context_json" \
  --source-authority-json "$SOURCE_AUTHORITY_JSON" \
  --report-json "$public_promotion_json"

.venv/bin/python tools/bounded_graph_promotion_audit.py \
  --bounded-graph-context-json "$private_context_json" \
  --source-authority-json "$SOURCE_AUTHORITY_JSON" \
  --report-json "$private_promotion_json" || true

.venv/bin/python - \
  "$probe_db" \
  "$public_context_json" \
  "$private_context_json" \
  "$public_contract_json" \
  "$private_contract_json" \
  "$public_promotion_json" \
  "$private_promotion_json" \
  "$report_json" <<'PY'
import json
import sqlite3
import sys
from pathlib import Path

(
    probe_db,
    public_context_path,
    private_context_path,
    public_contract_path,
    private_contract_path,
    public_promotion_path,
    private_promotion_path,
    report_path,
) = sys.argv[1:9]


def load_context(path: str) -> dict:
    payload = json.load(open(path, encoding="utf-8"))
    return payload.get("boundedGraphContext") or payload.get("data", {}).get("boundedGraphContext") or payload


def load_json(path: str) -> dict:
    return json.load(open(path, encoding="utf-8"))


def db_scalar(query: str, args: tuple = ()) -> int:
    with sqlite3.connect(probe_db) as conn:
        row = conn.execute(query, args).fetchone()
    return int(row[0] or 0)


def has_object(context: dict, key: str) -> bool:
    return any(row.get("key") == key for row in context.get("objects") or [])


def has_association_to(context: dict, key: str) -> bool:
    return any((row.get("to") or {}).get("key") == key for row in context.get("associations") or [])


def pass_check(checks: list[dict], key: str, detail: str) -> None:
    checks.append({"key": key, "passes": True, "detail": detail})


def fail(checks: list[dict], key: str, detail: str) -> None:
    checks.append({"key": key, "passes": False, "detail": detail})


checks: list[dict] = []
public = load_context(public_context_path)
private = load_context(private_context_path)
public_contract = load_json(public_contract_path)
private_contract = load_json(private_contract_path)
public_promotion = load_json(public_promotion_path)
private_promotion = load_json(private_promotion_path)

mutation_counts = {
    "private_pr": db_scalar("select count(*) from pull_requests where key='pull-request:github:apache/flink-kubernetes-operator#909' and visibility='private' and acl_state='current'"),
    "private_relationship": db_scalar("select count(*) from ticket_pull_requests where id=880 and visibility='private' and acl_state='current' and rank_score=100"),
    "public_fallback": db_scalar("select count(*) from ticket_pull_requests where id=877 and visibility='public' and rank_score=10"),
}
if all(value == 1 for value in mutation_counts.values()):
    pass_check(checks, "derived_real_acl_mutation_applied", "probe DB contains one private high-rank connector edge and one public fallback")
else:
    fail(checks, "derived_real_acl_mutation_applied", f"unexpected mutation counts {mutation_counts}")

if public_contract.get("passes_contract") and private_contract.get("passes_contract"):
    pass_check(checks, "contracts_pass", "public and private connector contexts pass bounded graph contract")
else:
    fail(checks, "contracts_pass", "one or both connector contracts failed")

private_pr = "pull-request:github:apache/flink-kubernetes-operator#909"
public_fallback_pr = "pull-request:github:apache/flink-kubernetes-operator#906"
public_text = json.dumps(public, sort_keys=True)
if not has_object(public, private_pr) and not has_association_to(public, private_pr) and private_pr not in public_text:
    pass_check(checks, "public_context_hides_private_edge", "public-only traversal hides private high-rank endpoint, association, and key text")
else:
    fail(checks, "public_context_hides_private_edge", "public-only context leaked private PR or association")

if has_object(public, public_fallback_pr) and has_association_to(public, public_fallback_pr):
    pass_check(checks, "public_context_uses_public_fallback", "public-only traversal skips private candidate before fanout and returns lower-ranked public fallback")
else:
    fail(checks, "public_context_uses_public_fallback", "public-only context did not return the expected public fallback")

if has_object(private, private_pr) and has_association_to(private, private_pr):
    pass_check(checks, "private_context_includes_private_edge", "private-allowed traversal returns the higher-ranked private connector edge")
else:
    fail(checks, "private_context_includes_private_edge", "private-allowed context did not include private PR edge")

if not has_object(private, public_fallback_pr):
    pass_check(checks, "private_context_spends_fanout_on_private_edge", "private-allowed traversal spends limit=1 fanout on the high-rank private edge")
else:
    fail(checks, "private_context_spends_fanout_on_private_edge", "private-allowed context unexpectedly included lower-ranked public fallback")

private_associations = private.get("associations") or []
if len(private_associations) == 1 and private_associations[0].get("claimAllowed") is False and private_associations[0].get("claimGateReason") == "relationship_visibility_restricted":
    pass_check(checks, "private_edge_not_product_promotable", "restricted private edge is readable for the principal but not promotable as a public product claim")
else:
    fail(checks, "private_edge_not_product_promotable", "private edge should be readable but claim-gated by restricted visibility")

if public_promotion.get("promotable_association_count") == 1 and public_promotion.get("blocked_association_count") == 0:
    pass_check(checks, "public_promotion_audit_clean", "public fallback edge remains promotable")
else:
    fail(checks, "public_promotion_audit_clean", "public promotion audit should have exactly one promotable association")

private_association_blockers = [
    blocker
    for row in private_promotion.get("associations") or []
    for blocker in row.get("blockers") or []
]
if private_promotion.get("promotable_association_count") == 0 and private_promotion.get("blocked_association_count") == 1 and "relationship_visibility_not_public" in private_association_blockers:
    pass_check(checks, "private_promotion_audit_blocks_restricted_edge", "private edge is visible to authorized principal but blocked from product promotion")
else:
    fail(checks, "private_promotion_audit_blocks_restricted_edge", "private promotion audit did not block the restricted association")

coverage = public.get("coverage") or {}
if coverage.get("absenceClaimsAllowed") is False and coverage.get("absenceClaimGateReason") == "source_coverage_gate":
    pass_check(checks, "absence_still_gated", "ACL probe remains sparse and does not support absence claims")
else:
    fail(checks, "absence_still_gated", "ACL probe should keep absence claims gated")

passed = sum(1 for row in checks if row["passes"])
failed = len(checks) - passed
report = {
    "passes_eval": failed == 0,
    "passes_smoke_eval": failed == 0,
    "scope": "derived_real_connector_runtime_acl_probe",
    "real_connector_acl_ingestion_proven": False,
    "golden_eval": {
        "pass_count": passed,
        "failure_count": failed,
        "question_count": len(checks),
        "passes_golden_eval": failed == 0,
        "questions": checks,
    },
    "cases": {
        "public_only": {
            "context": public_context_path,
            "contract": public_contract_path,
            "promotion_audit": public_promotion_path,
            "principal": "principal:public-only",
        },
        "private_allowed": {
            "context": private_context_path,
            "contract": private_contract_path,
            "promotion_audit": private_promotion_path,
            "principal": "principal:private-reader",
            "allowed_visibility_classes": ["private"],
        },
    },
    "probe_database": probe_db,
    "mutation_counts": mutation_counts,
}
Path(report_path).write_text(json.dumps(report, indent=2, sort_keys=True), encoding="utf-8")
print(
    "real_connector_acl_probe",
    "passes_eval=" + str(report["passes_eval"]),
    "golden=" + str(passed) + "/" + str(len(checks)),
)
for row in checks:
    print("real_connector_acl_probe", row["key"], "passes=" + str(row["passes"]))
if failed:
    raise SystemExit("real connector ACL probe eval failed")
PY
