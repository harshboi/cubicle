#!/bin/sh
set -eu

SCRIPT_DIR=$(CDPATH= cd -- "$(dirname -- "$0")" && pwd)
ROOT_DIR=$(CDPATH= cd -- "$SCRIPT_DIR/../../.." && pwd)
OUT_DIR=${OUT_DIR:-/tmp/open_graph_revenue_minimum}
DB="$OUT_DIR/open_graph_revenue.db"
SOURCE_AUTHORITY_JSON="$SCRIPT_DIR/source_authority.json"
OPEN_GRAPH_FIXTURE_JSON="$SCRIPT_DIR/open_graph_fixture.json"

mkdir -p "$OUT_DIR"
rm -f "$DB" "$DB-wal" "$DB-shm" "$DB-journal"
cd "$ROOT_DIR"

context_json="$OUT_DIR/context.json"
normalized_json="$OUT_DIR/normalized.json"
scaffold_md="$OUT_DIR/scaffold.md"
prompt_md="$OUT_DIR/prompt.md"
baseline_md="$OUT_DIR/generic_baseline.md"
eval_json="$OUT_DIR/eval.json"
typed_row_baseline_md="$OUT_DIR/typed_row_baseline.md"
typed_row_eval_json="$OUT_DIR/typed_row_eval.json"
seed_only_context_json="$OUT_DIR/seed_only_context.json"
seed_only_normalized_json="$OUT_DIR/seed_only_normalized.json"
seed_only_scaffold_md="$OUT_DIR/seed_only_scaffold.md"
seed_only_baseline_md="$OUT_DIR/seed_only_baseline.md"
seed_only_eval_json="$OUT_DIR/seed_only_eval.json"
answers_json="$OUT_DIR/answers.json"
comparison_json="$OUT_DIR/promotion_comparison.json"
promotion_audit_json="$OUT_DIR/promotion_audit.json"

assert_no_leakage() {
  context_json=$1
  .venv/bin/python - "$context_json" <<'PY'
import json
import sys

path = sys.argv[1]
payload = json.load(open(path, encoding="utf-8"))
context = payload.get("boundedGraphContext") or payload.get("data", {}).get("boundedGraphContext") or {}
text = json.dumps(context, sort_keys=True)
for forbidden in [
    "opportunity:hidden-private-renewal",
    "ticket:",
    "pull-request:",
    "incident:",
    "runbook:",
    "WorkProgram",
    "TicketLensResult",
    "PullRequestLensResult",
    "TPM",
    "Flink",
]:
    if forbidden in text:
        raise SystemExit("open graph revenue context leaked " + forbidden)
PY
}

assert_promotion_audit_full_coverage() {
  report_json=$1
  .venv/bin/python - "$report_json" <<'PY'
import json
import sys

path = sys.argv[1]
data = json.load(open(path, encoding="utf-8"))
if not data.get("passes_promotion_audit"):
    raise SystemExit("promotion audit failed in " + path)
if data.get("blocked_association_count") != 0:
    raise SystemExit("blocked associations in " + path + ": " + str(data.get("blocked_association_count")))
if data.get("promotable_association_count") != data.get("association_count"):
    raise SystemExit(
        "not all associations promotable in "
        + path
        + ": "
        + str(data.get("promotable_association_count"))
        + "/"
        + str(data.get("association_count"))
    )
PY
}

go run ./cmd/ontology-service open-graph-fixture-load \
  --fixture-json "$OPEN_GRAPH_FIXTURE_JSON" \
  --database "$DB" \
  --reset-database \
  > "$OUT_DIR/load_summary.json"

go run ./cmd/ontology-service bounded-graph-context-export \
  --fixture open-graph \
  --database "$DB" \
  --source-authority-json "$SOURCE_AUTHORITY_JSON" \
  --start-object-type customer_account \
  --start-key customer:globex \
  --association-types has_opportunity,blocked_by,guided_by,updated_in \
  --depth 3 \
  --limit-per-object 4 \
  --out "$context_json"

assert_no_leakage "$context_json"

.venv/bin/python tools/bounded_graph_contract.py \
  --bounded-graph-context-json "$context_json" \
  --report-json "$OUT_DIR/contract.json"

.venv/bin/python tools/bounded_graph_promotion_audit.py \
  --bounded-graph-context-json "$context_json" \
  --source-authority-json "$SOURCE_AUTHORITY_JSON" \
  --report-json "$promotion_audit_json"
assert_promotion_audit_full_coverage "$promotion_audit_json"

.venv/bin/python tools/bounded_graph_brief.py \
  --bounded-graph-context-json "$context_json" \
  --context-json "$normalized_json" \
  --brief-md "$scaffold_md" \
  --generic-baseline-md "$baseline_md" \
  --typed-row-baseline-md "$typed_row_baseline_md" \
  --prompt-mode generic \
  --prompt-md "$prompt_md"

.venv/bin/python tools/bounded_graph_brief.py \
  --bounded-graph-context-json "$context_json" \
  --context-json "$OUT_DIR/eval_context.json" \
  --brief-md "$OUT_DIR/eval_scaffold.md" \
  --prompt-mode generic \
  --prompt-md "$OUT_DIR/eval_prompt.md" \
  --llm-brief-md "$baseline_md" \
  --evaluation-json "$eval_json" \
  --golden-json "$SCRIPT_DIR/golden_questions.json"

.venv/bin/python tools/bounded_graph_brief.py \
  --bounded-graph-context-json "$context_json" \
  --context-json "$OUT_DIR/typed_row_eval_context.json" \
  --brief-md "$OUT_DIR/typed_row_eval_scaffold.md" \
  --prompt-mode generic \
  --prompt-md "$OUT_DIR/typed_row_eval_prompt.md" \
  --llm-brief-md "$typed_row_baseline_md" \
  --evaluation-json "$typed_row_eval_json" \
  --golden-json "$SCRIPT_DIR/golden_questions.json"

go run ./cmd/ontology-service bounded-graph-context-export \
  --fixture open-graph \
  --database "$DB" \
  --source-authority-json "$SOURCE_AUTHORITY_JSON" \
  --start-object-type customer_account \
  --start-key customer:globex \
  --association-types has_opportunity,blocked_by,guided_by,updated_in \
  --depth 0 \
  --limit-per-object 4 \
  --out "$seed_only_context_json"

assert_no_leakage "$seed_only_context_json"

.venv/bin/python tools/bounded_graph_contract.py \
  --bounded-graph-context-json "$seed_only_context_json" \
  --report-json "$OUT_DIR/seed_only_contract.json"

.venv/bin/python tools/bounded_graph_brief.py \
  --bounded-graph-context-json "$seed_only_context_json" \
  --context-json "$seed_only_normalized_json" \
  --brief-md "$seed_only_scaffold_md" \
  --generic-baseline-md "$seed_only_baseline_md" \
  --prompt-mode generic \
  --prompt-md "$OUT_DIR/seed_only_prompt.md"

.venv/bin/python tools/bounded_graph_brief.py \
  --bounded-graph-context-json "$seed_only_context_json" \
  --context-json "$OUT_DIR/seed_only_eval_context.json" \
  --brief-md "$OUT_DIR/seed_only_eval_scaffold.md" \
  --prompt-mode generic \
  --prompt-md "$OUT_DIR/seed_only_eval_prompt.md" \
  --llm-brief-md "$seed_only_baseline_md" \
  --evaluation-json "$seed_only_eval_json" \
  --golden-json "$SCRIPT_DIR/golden_questions.json"

.venv/bin/python - "$answers_json" "$baseline_md" "$seed_only_baseline_md" "$typed_row_baseline_md" <<'PY'
import json
import sys

answers_path, depth_path, seed_only_path, typed_row_path = sys.argv[1:5]
payload = {
    "answers": [
        {
            "key": "depth_3_open_graph",
            "label": "Depth-3 open Ent revenue graph traversal",
            "path": depth_path,
            "strategy": "open_ent_bounded_graph_context_depth_3",
            "answer_kind": "candidate",
        },
        {
            "key": "depth_0_seed",
            "label": "Seed-only customer account",
            "path": seed_only_path,
            "strategy": "open_ent_bounded_graph_context_depth_0",
            "answer_kind": "baseline",
        },
        {
            "key": "typed_rows",
            "label": "Open object rows without associations",
            "path": typed_row_path,
            "strategy": "typed_row_object_summary",
            "answer_kind": "baseline",
        },
    ],
    "promotion_gates": [
        {
            "key": "depth_3_over_seed_only",
            "candidate_key": "depth_3_open_graph",
            "baseline_key": "depth_0_seed",
        },
        {
            "key": "depth_3_over_typed_rows",
            "candidate_key": "depth_3_open_graph",
            "baseline_key": "typed_rows",
        },
    ],
}
open(answers_path, "w", encoding="utf-8").write(json.dumps(payload, indent=2, sort_keys=True))
PY

.venv/bin/python tools/cubicle_graph_brief.py \
  --golden-json "$SCRIPT_DIR/golden_questions.json" \
  --compare-answers-json "$answers_json" \
  --comparison-json "$comparison_json" \
  --require-promotion-gates

.venv/bin/python - "$eval_json" "$seed_only_eval_json" "$typed_row_eval_json" "$comparison_json" <<'PY'
import json
import sys

eval_path, seed_eval_path, typed_eval_path, comparison_path = sys.argv[1:5]
data = json.load(open(eval_path, encoding="utf-8"))
seed_eval = json.load(open(seed_eval_path, encoding="utf-8"))
typed_eval = json.load(open(typed_eval_path, encoding="utf-8"))
comparison = json.load(open(comparison_path, encoding="utf-8"))
golden = data.get("golden_eval", {})
seed_golden = seed_eval.get("golden_eval", {})
typed_golden = typed_eval.get("golden_eval", {})
print(
    "open_graph_revenue",
    "passes_eval=" + str(data.get("passes_eval")),
    "passes_smoke_eval=" + str(data.get("passes_smoke_eval")),
    "golden=" + str(golden.get("pass_count")) + "/" + str(golden.get("question_count")),
)
for gate in comparison.get("promotion_gates", []):
    total = gate.get("candidate_pass_count", 0) + gate.get("candidate_failure_count", 0)
    baseline_total = gate.get("baseline_pass_count", 0) + gate.get("baseline_failure_count", 0)
    print(
        "open_graph_revenue",
        gate.get("key"),
        "passes=" + str(gate.get("passes")),
        "candidate=" + str(gate.get("candidate_pass_count")) + "/" + str(total),
        "baseline=" + str(gate.get("baseline_pass_count")) + "/" + str(baseline_total),
    )
print(
    "open_graph_revenue",
    "seed_only_golden=" + str(seed_golden.get("pass_count")) + "/" + str(seed_golden.get("question_count")),
    "typed_row_golden=" + str(typed_golden.get("pass_count")) + "/" + str(typed_golden.get("question_count")),
)
if not data.get("passes_eval") or not comparison.get("passes_promotion_gates"):
    sys.exit(1)
PY
