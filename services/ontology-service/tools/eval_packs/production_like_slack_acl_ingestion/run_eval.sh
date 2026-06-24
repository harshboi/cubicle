#!/bin/sh
set -eu

SCRIPT_DIR=$(CDPATH= cd -- "$(dirname -- "$0")" && pwd)
ROOT_DIR=$(CDPATH= cd -- "$SCRIPT_DIR/../../.." && pwd)
OUT_DIR=${OUT_DIR:-/tmp/production_like_slack_acl_ingestion}
DB="$OUT_DIR/production_like_slack_acl.db"
OPEN_GRAPH_FIXTURE_JSON="$SCRIPT_DIR/open_graph_fixture.json"
SOURCE_AUTHORITY_JSON="$SCRIPT_DIR/source_authority.json"

mkdir -p "$OUT_DIR"
rm -f "$DB" "$DB-wal" "$DB-shm" "$DB-journal"
cd "$ROOT_DIR"

load_json="$OUT_DIR/load_summary.json"
public_context_json="$OUT_DIR/public_context.json"
private_context_json="$OUT_DIR/private_allowed_context.json"
public_contract_json="$OUT_DIR/public_contract.json"
private_contract_json="$OUT_DIR/private_allowed_contract.json"
public_promotion_json="$OUT_DIR/public_promotion_audit.json"
private_promotion_json="$OUT_DIR/private_allowed_promotion_audit.json"
inventory_json="$OUT_DIR/inventory.json"
inventory_md="$OUT_DIR/inventory.md"
eval_json="$OUT_DIR/eval.json"

go run ./cmd/ontology-service open-graph-fixture-load \
  --fixture-json "$OPEN_GRAPH_FIXTURE_JSON" \
  --database "$DB" \
  --reset-database \
  > "$load_json"

go run ./cmd/ontology-service bounded-graph-context-export \
  --fixture open-graph \
  --database "$DB" \
  --source-authority-json "$SOURCE_AUTHORITY_JSON" \
  --principal-key principal:public-only \
  --start-object-type slack_channel \
  --start-key slack:channel:C-incident \
  --association-types contains_message,links_to \
  --depth 1 \
  --limit-per-object 1 \
  --out "$public_context_json"

go run ./cmd/ontology-service bounded-graph-context-export \
  --fixture open-graph \
  --database "$DB" \
  --source-authority-json "$SOURCE_AUTHORITY_JSON" \
  --principal-key principal:private-reader \
  --allowed-visibility-classes private \
  --start-object-type slack_channel \
  --start-key slack:channel:C-incident \
  --association-types contains_message,links_to \
  --depth 1 \
  --limit-per-object 1 \
  --out "$private_context_json"

.venv/bin/python tools/bounded_graph_contract.py \
  --bounded-graph-context-json "$public_context_json" \
  --report-json "$public_contract_json"

.venv/bin/python tools/bounded_graph_contract.py \
  --bounded-graph-context-json "$private_context_json" \
  --report-json "$private_contract_json"

.venv/bin/python tools/bounded_graph_promotion_audit.py \
  --bounded-graph-context-json "$public_context_json" \
  --source-authority-json "$SOURCE_AUTHORITY_JSON" \
  --report-json "$public_promotion_json"

.venv/bin/python tools/bounded_graph_promotion_audit.py \
  --bounded-graph-context-json "$private_context_json" \
  --source-authority-json "$SOURCE_AUTHORITY_JSON" \
  --report-json "$private_promotion_json" || true

.venv/bin/python tools/bounded_graph_real_connector_inventory.py \
  --database "$DB" \
  --limit-per-db 5 \
  --require-real-acl-ingestion \
  --report-json "$inventory_json" \
  --report-md "$inventory_md"

.venv/bin/python - \
  "$DB" \
  "$load_json" \
  "$public_context_json" \
  "$private_context_json" \
  "$public_contract_json" \
  "$private_contract_json" \
  "$public_promotion_json" \
  "$private_promotion_json" \
  "$inventory_json" \
  "$eval_json" <<'PY'
import json
import sqlite3
import sys
from pathlib import Path

(
    db,
    load_path,
    public_context_path,
    private_context_path,
    public_contract_path,
    private_contract_path,
    public_promotion_path,
    private_promotion_path,
    inventory_path,
    eval_path,
) = sys.argv[1:11]


def load_json(path: str) -> dict:
    return json.load(open(path, encoding="utf-8"))


def context(path: str) -> dict:
    payload = load_json(path)
    return payload.get("boundedGraphContext") or payload.get("data", {}).get("boundedGraphContext") or payload


def scalar(conn: sqlite3.Connection, query: str) -> int:
    return int(conn.execute(query).fetchone()[0] or 0)


def has_object(ctx: dict, key: str) -> bool:
    return any(row.get("key") == key for row in ctx.get("objects") or [])


def has_association_to(ctx: dict, key: str) -> bool:
    return any((row.get("to") or {}).get("key") == key for row in ctx.get("associations") or [])


def pass_check(checks: list[dict], key: str, detail: str) -> None:
    checks.append({"key": key, "passes": True, "detail": detail})


def fail(checks: list[dict], key: str, detail: str) -> None:
    checks.append({"key": key, "passes": False, "detail": detail})


checks: list[dict] = []
load_summary = load_json(load_path)
public = context(public_context_path)
private = context(private_context_path)
public_contract = load_json(public_contract_path)
private_contract = load_json(private_contract_path)
public_promotion = load_json(public_promotion_path)
private_promotion = load_json(private_promotion_path)
inventory = load_json(inventory_path)

with sqlite3.connect(db) as conn:
    connector_kind = conn.execute("select connector_kind from source_connections").fetchone()[0]
    source_identity_count = scalar(
        conn,
        """
        select count(*)
        from source_connections
        where source_system='slack'
          and source_instance='slack.example.test/workspace-a'
          and connector_kind='slack_api'
        """,
    )
    private_counts = {
        "objects": scalar(conn, "select count(*) from open_graph_objects where visibility='private' and acl_state='current' and source_system='slack' and source_instance='slack.example.test/workspace-a'"),
        "associations": scalar(conn, "select count(*) from open_graph_associations where visibility='private' and acl_state='current' and source_system='slack' and source_instance='slack.example.test/workspace-a'"),
        "evidence": scalar(conn, "select count(*) from evidences where visibility='private' and acl_state='current' and source_system='slack' and source_instance='slack.example.test/workspace-a'"),
    }
    scoped_counts = {
        "objects": scalar(conn, "select count(*) from open_graph_objects where source_scope_state_id is not null"),
        "associations": scalar(conn, "select count(*) from open_graph_associations where source_scope_state_id is not null"),
        "evidence": scalar(conn, "select count(*) from evidences where source_scope_state_id is not null"),
    }

if load_summary.get("openGraphFixtureLoad") == {"objectCount": 4, "associationCount": 3, "evidenceCount": 3}:
    pass_check(checks, "fixture_loaded_expected_rows", "Slack ACL replay loaded 4 objects, 3 associations, and 3 evidence rows")
else:
    fail(checks, "fixture_loaded_expected_rows", f"unexpected load summary {load_summary}")

if connector_kind == "slack_api" and source_identity_count == 1:
    pass_check(checks, "production_like_connector_identity", "source connection identity is slack/slack.example.test/workspace-a with slack_api")
else:
    fail(checks, "production_like_connector_identity", f"unexpected connector identity kind={connector_kind!r} count={source_identity_count}")

if private_counts == {"objects": 1, "associations": 1, "evidence": 1}:
    pass_check(checks, "private_acl_rows_ingested", "private object, relationship, and evidence rows have current Slack ACL metadata")
else:
    fail(checks, "private_acl_rows_ingested", f"unexpected private ACL counts {private_counts}")

if scoped_counts == {"objects": 4, "associations": 3, "evidence": 3}:
    pass_check(checks, "graph_rows_reference_scope_state", "all OpenGraph rows and evidence reference the Slack source scope state")
else:
    fail(checks, "graph_rows_reference_scope_state", f"unexpected scoped counts {scoped_counts}")

db_report = (inventory.get("databases") or [{}])[0]
if inventory.get("passes_real_acl_ingestion_requirement") and db_report.get("production_acl_candidate_count") == 1:
    pass_check(checks, "inventory_hard_acl_gate_passes", "inventory sees one production-like connector-backed private ACL candidate")
else:
    fail(checks, "inventory_hard_acl_gate_passes", f"unexpected inventory {inventory}")

private_message = "slack:message:C-incident:T-private"
public_message = "slack:message:C-incident:T-public"
public_text = json.dumps(public, sort_keys=True)
if not has_object(public, private_message) and not has_association_to(public, private_message) and private_message not in public_text:
    pass_check(checks, "public_context_hides_private_message", "public-only traversal hides the high-rank private message and edge")
else:
    fail(checks, "public_context_hides_private_message", "public context leaked private message")

if has_object(public, public_message) and has_association_to(public, public_message):
    pass_check(checks, "public_context_uses_public_fallback", "public-only traversal returns the lower-ranked public fallback")
else:
    fail(checks, "public_context_uses_public_fallback", "public context did not return public fallback")

if has_object(private, private_message) and has_association_to(private, private_message):
    pass_check(checks, "private_context_includes_private_message", "private-allowed traversal returns the high-rank private message")
else:
    fail(checks, "private_context_includes_private_message", "private context did not include private message")

if not has_object(private, public_message):
    pass_check(checks, "private_context_spends_fanout_on_private_edge", "private-allowed traversal spends limit=1 fanout on private edge")
else:
    fail(checks, "private_context_spends_fanout_on_private_edge", "private context unexpectedly included public fallback")

private_associations = private.get("associations") or []
if len(private_associations) == 1 and private_associations[0].get("claimAllowed") is False and private_associations[0].get("claimGateReason") == "relationship_visibility_restricted":
    pass_check(checks, "private_edge_not_public_claim", "private edge is readable for principal but not promotable as public claim")
else:
    fail(checks, "private_edge_not_public_claim", f"unexpected private association policy {private_associations}")

if public_contract.get("passes_contract") and private_contract.get("passes_contract"):
    pass_check(checks, "bounded_context_contracts_pass", "public and private bounded contexts pass contract")
else:
    fail(checks, "bounded_context_contracts_pass", f"contract failure public={public_contract} private={private_contract}")

if public_promotion.get("promotable_association_count") == 1 and public_promotion.get("blocked_association_count") == 0:
    pass_check(checks, "public_promotion_audit_passes", "public fallback relationship remains promotable")
else:
    fail(checks, "public_promotion_audit_passes", f"unexpected public promotion {public_promotion}")

if private_promotion.get("blocked_association_count") == 1 and private_promotion.get("promotable_association_count") == 0:
    pass_check(checks, "private_promotion_blocks_restricted_claim", "private readable relationship remains blocked from public promotion")
else:
    fail(checks, "private_promotion_blocks_restricted_claim", f"unexpected private promotion {private_promotion}")

context_text = json.dumps({"public": public, "private": private}, sort_keys=True)
forbidden = ["SourceSyncRun", "SourceSyncIssue", "source_scope_states", "error_message", "slack_api"]
hits = [value for value in forbidden if value in context_text]
if not hits:
    pass_check(checks, "source_diagnostics_not_prompted", "connector internals do not leak into bounded context")
else:
    fail(checks, "source_diagnostics_not_prompted", "context leaked " + ", ".join(hits))

passed = sum(1 for row in checks if row["passes"])
failed = len(checks) - passed
report = {
    "passes_eval": failed == 0,
    "passes_smoke_eval": failed == 0,
    "scope": "production_like_slack_acl_ingestion",
    "production_like_acl_ingestion_proven": failed == 0,
    "live_private_saas_capture_proven": False,
    "database": db,
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
print("production_like_slack_acl_ingestion passes_eval=" + str(report["passes_eval"]), "golden=" + str(passed) + "/" + str(len(checks)))
for row in checks:
    print("production_like_slack_acl_ingestion", row["key"], "passes=" + str(row["passes"]))
if failed:
    raise SystemExit("production-like Slack ACL ingestion eval failed")
PY
