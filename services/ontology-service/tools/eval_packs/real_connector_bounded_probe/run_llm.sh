#!/usr/bin/env bash
set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
ROOT_DIR="$(cd "$SCRIPT_DIR/../../.." && pwd)"
OUT_DIR="${OUT_DIR:-/tmp/real_connector_bounded_probe}"

DATABASE="${DATABASE:-}"
START_OBJECT_TYPE="${START_OBJECT_TYPE:-ticket}"
START_KEY="${START_KEY:-ticket:jira:FLINK-32695}"
ASSOCIATION_TYPES="${ASSOCIATION_TYPES:-implemented_by}"
DEPTH="${DEPTH:-1}"
LIMIT_PER_OBJECT="${LIMIT_PER_OBJECT:-6}"
SOURCE_AUTHORITY_JSON="${SOURCE_AUTHORITY_JSON:-$ROOT_DIR/internal/graphcontext/source_authority.json}"
CONTRACT_PROFILE="${CONTRACT_PROFILE:-connector}"
COVERAGE_SOURCE_SYSTEM="${COVERAGE_SOURCE_SYSTEM:-}"
COVERAGE_SOURCE_INSTANCE="${COVERAGE_SOURCE_INSTANCE:-}"
COVERAGE_WINDOW_START="${COVERAGE_WINDOW_START:-}"
COVERAGE_WINDOW_END="${COVERAGE_WINDOW_END:-}"
ABSENCE_CLAIM_ASSOCIATION_TYPES="${ABSENCE_CLAIM_ASSOCIATION_TYPES:-$ASSOCIATION_TYPES}"

MODEL="${MODEL:-mlx-community/Qwen3-Coder-30B-A3B-Instruct-bf16}"
MLX_PYTHON="${MLX_PYTHON:-/Users/harsh/.venv-vllm-metal/bin/python}"
LLM_MAX_TOKENS="${LLM_MAX_TOKENS:-24576}"
LLM_TIMEOUT_SECONDS="${LLM_TIMEOUT_SECONDS:-1200}"
REQUIRE_RAW_PASS="${REQUIRE_RAW_PASS:-0}"
REQUIRE_REPAIRED_PASS="${REQUIRE_REPAIRED_PASS:-1}"

if [[ -z "$DATABASE" ]]; then
  echo "DATABASE is required" >&2
  exit 2
fi

mkdir -p "$OUT_DIR"
cd "$ROOT_DIR"

context_json="$OUT_DIR/context.json"
normalized_json="$OUT_DIR/normalized.json"
scaffold_md="$OUT_DIR/scaffold.md"
prompt_md="$OUT_DIR/prompt.md"
generic_baseline_md="$OUT_DIR/generic_baseline.md"
generic_baseline_eval_json="$OUT_DIR/generic_baseline_eval.json"
typed_row_baseline_md="$OUT_DIR/typed_row_baseline.md"
typed_row_baseline_eval_json="$OUT_DIR/typed_row_baseline_eval.json"
seed_only_context_json="$OUT_DIR/seed_only_context.json"
seed_only_baseline_md="$OUT_DIR/seed_only_baseline.md"
seed_only_eval_json="$OUT_DIR/seed_only_eval.json"
raw_md="$OUT_DIR/raw.md"
raw_eval_json="$OUT_DIR/raw_eval.json"
repaired_md="$OUT_DIR/repaired.md"
repaired_eval_json="$OUT_DIR/repaired_eval.json"
golden_json="$OUT_DIR/golden_dynamic.json"
answers_json="$OUT_DIR/answers.json"
comparison_json="$OUT_DIR/comparison.json"
promotion_audit_json="$OUT_DIR/promotion_audit.json"

assert_no_source_diagnostic_leakage() {
  local path=$1
  .venv/bin/python - "$path" <<'PY'
import json
import sys

path = sys.argv[1]
payload = json.load(open(path, encoding="utf-8"))
context = payload.get("boundedGraphContext") or payload.get("data", {}).get("boundedGraphContext") or {}
text = json.dumps(context, sort_keys=True)
for forbidden in [
    "SourceSyncIssue",
    "SourceSyncRun",
    "SourceScope",
    "UnresolvedReference",
    "source_sync_issue",
    "source_sync_run",
    "unresolved_reference",
    "token=",
    "Authorization",
]:
    if forbidden in text:
        raise SystemExit("bounded graph context leaked source diagnostic row/text: " + forbidden)
PY
}

export_args=(
  go run ./cmd/ontology-service bounded-graph-context-export
  --fixture real-connector
  --database "$DATABASE"
  --source-authority-json "$SOURCE_AUTHORITY_JSON"
  --start-object-type "$START_OBJECT_TYPE"
  --start-key "$START_KEY"
  --association-types "$ASSOCIATION_TYPES"
  --depth "$DEPTH"
  --limit-per-object "$LIMIT_PER_OBJECT"
  --absence-claim-association-types "$ABSENCE_CLAIM_ASSOCIATION_TYPES"
  --out "$context_json"
)
if [[ -n "$COVERAGE_SOURCE_SYSTEM" ]]; then
  export_args+=(--coverage-source-system "$COVERAGE_SOURCE_SYSTEM")
fi
if [[ -n "$COVERAGE_SOURCE_INSTANCE" ]]; then
  export_args+=(--coverage-source-instance "$COVERAGE_SOURCE_INSTANCE")
fi
if [[ -n "$COVERAGE_WINDOW_START" ]]; then
  export_args+=(--coverage-window-start "$COVERAGE_WINDOW_START")
fi
if [[ -n "$COVERAGE_WINDOW_END" ]]; then
  export_args+=(--coverage-window-end "$COVERAGE_WINDOW_END")
fi
"${export_args[@]}"

assert_no_source_diagnostic_leakage "$context_json"

.venv/bin/python tools/bounded_graph_contract.py \
  --bounded-graph-context-json "$context_json" \
  --profile "$CONTRACT_PROFILE" \
  --report-json "$OUT_DIR/contract.json"

.venv/bin/python tools/bounded_graph_promotion_audit.py \
  --bounded-graph-context-json "$context_json" \
  --source-authority-json "$SOURCE_AUTHORITY_JSON" \
  --report-json "$promotion_audit_json"

.venv/bin/python tools/bounded_graph_dynamic_golden.py \
  --bounded-graph-context-json "$context_json" \
  --golden-json "$golden_json" \
  --name "real-connector:${START_OBJECT_TYPE}:${START_KEY}" \
  --max-associations 4

.venv/bin/python tools/bounded_graph_brief.py \
  --bounded-graph-context-json "$context_json" \
  --context-json "$normalized_json" \
  --brief-md "$scaffold_md" \
  --generic-baseline-md "$generic_baseline_md" \
  --typed-row-baseline-md "$typed_row_baseline_md" \
  --prompt-mode generic \
  --prompt-md "$prompt_md"

raw_args=(
  .venv/bin/python tools/bounded_graph_brief.py
  --bounded-graph-context-json "$context_json"
  --context-json "$OUT_DIR/raw_context.json"
  --brief-md "$OUT_DIR/raw_scaffold.md"
  --prompt-mode generic
  --prompt-md "$OUT_DIR/raw_prompt.md"
  --llm-timeout-seconds "$LLM_TIMEOUT_SECONDS"
  --llm-brief-md "$raw_md"
  --evaluation-json "$raw_eval_json"
  --golden-json "$golden_json"
)
if [[ -n "${LLM_COMMAND:-}" ]]; then
  raw_args+=(--llm-command "$LLM_COMMAND")
else
  raw_args+=(--mlx-python "$MLX_PYTHON" --mlx-model "$MODEL" --llm-max-tokens "$LLM_MAX_TOKENS")
fi
"${raw_args[@]}"

.venv/bin/python tools/bounded_graph_brief.py \
  --bounded-graph-context-json "$context_json" \
  --context-json "$OUT_DIR/repaired_context.json" \
  --brief-md "$OUT_DIR/repaired_scaffold.md" \
  --prompt-mode generic \
  --prompt-md "$OUT_DIR/repaired_prompt.md" \
  --llm-brief-md "$raw_md" \
  --repaired-brief-md "$repaired_md" \
  --evaluation-json "$repaired_eval_json" \
  --golden-json "$golden_json"

.venv/bin/python tools/bounded_graph_brief.py \
  --bounded-graph-context-json "$context_json" \
  --context-json "$OUT_DIR/generic_baseline_eval_context.json" \
  --brief-md "$OUT_DIR/generic_baseline_eval_scaffold.md" \
  --prompt-mode generic \
  --prompt-md "$OUT_DIR/generic_baseline_eval_prompt.md" \
  --llm-brief-md "$generic_baseline_md" \
  --evaluation-json "$generic_baseline_eval_json" \
  --golden-json "$golden_json"

.venv/bin/python tools/bounded_graph_brief.py \
  --bounded-graph-context-json "$context_json" \
  --context-json "$OUT_DIR/typed_row_baseline_eval_context.json" \
  --brief-md "$OUT_DIR/typed_row_baseline_eval_scaffold.md" \
  --prompt-mode generic \
  --prompt-md "$OUT_DIR/typed_row_baseline_eval_prompt.md" \
  --llm-brief-md "$typed_row_baseline_md" \
  --evaluation-json "$typed_row_baseline_eval_json" \
  --golden-json "$golden_json"

go run ./cmd/ontology-service bounded-graph-context-export \
  --fixture real-connector \
  --database "$DATABASE" \
  --source-authority-json "$SOURCE_AUTHORITY_JSON" \
  --start-object-type "$START_OBJECT_TYPE" \
  --start-key "$START_KEY" \
  --association-types "$ASSOCIATION_TYPES" \
  --depth 0 \
  --limit-per-object "$LIMIT_PER_OBJECT" \
  --out "$seed_only_context_json"

assert_no_source_diagnostic_leakage "$seed_only_context_json"

.venv/bin/python tools/bounded_graph_brief.py \
  --bounded-graph-context-json "$seed_only_context_json" \
  --context-json "$OUT_DIR/seed_only_context_normalized.json" \
  --brief-md "$OUT_DIR/seed_only_scaffold.md" \
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
  --golden-json "$golden_json"

.venv/bin/python - "$answers_json" "$raw_md" "$repaired_md" "$generic_baseline_md" "$seed_only_baseline_md" "$typed_row_baseline_md" <<'PY'
import json
import sys

answers_path, raw_path, repaired_path, generic_path, seed_only_path, typed_row_path = sys.argv[1:7]
payload = {
    "answers": [
        {
            "key": "raw_model",
            "label": "Raw local model answer",
            "path": raw_path,
            "strategy": "real_connector_bounded_graph_raw_model",
            "answer_kind": "raw",
        },
        {
            "key": "repaired_model",
            "label": "Repaired local model answer",
            "path": repaired_path,
            "strategy": "real_connector_bounded_graph_repaired_model",
            "answer_kind": "repaired",
        },
        {
            "key": "generic_baseline",
            "label": "Deterministic generic graph traversal",
            "path": generic_path,
            "strategy": "real_connector_bounded_graph_generic_baseline",
            "answer_kind": "candidate",
        },
        {
            "key": "depth_0_seed",
            "label": "Seed-only bounded graph context",
            "path": seed_only_path,
            "strategy": "real_connector_bounded_graph_depth_0",
            "answer_kind": "baseline",
        },
        {
            "key": "typed_rows",
            "label": "Typed object rows without associations",
            "path": typed_row_path,
            "strategy": "typed_row_object_summary",
            "answer_kind": "baseline",
        },
    ],
    "promotion_gates": [
        {
            "key": "repaired_over_seed_only",
            "candidate_key": "repaired_model",
            "baseline_key": "depth_0_seed",
        },
        {
            "key": "repaired_over_typed_rows",
            "candidate_key": "repaired_model",
            "baseline_key": "typed_rows",
        },
        {
            "key": "generic_over_seed_only",
            "candidate_key": "generic_baseline",
            "baseline_key": "depth_0_seed",
        },
        {
            "key": "generic_over_typed_rows",
            "candidate_key": "generic_baseline",
            "baseline_key": "typed_rows",
        },
    ],
}
open(answers_path, "w", encoding="utf-8").write(json.dumps(payload, indent=2, sort_keys=True))
PY

.venv/bin/python tools/cubicle_graph_brief.py \
  --golden-json "$golden_json" \
  --compare-answers-json "$answers_json" \
  --comparison-json "$comparison_json"

.venv/bin/python - "$OUT_DIR" "$REQUIRE_RAW_PASS" "$REQUIRE_REPAIRED_PASS" <<'PY'
import json
import sys
from pathlib import Path

out = Path(sys.argv[1])
require_raw = sys.argv[2] == "1"
require_repaired = sys.argv[3] == "1"
failures = []

promotion = json.loads((out / "promotion_audit.json").read_text(encoding="utf-8"))
print(
    "real_connector_probe",
    "objects=" + str(promotion.get("object_count")),
    "associations=" + str(promotion.get("association_count")),
    "promotable_objects=" + str(promotion.get("promotable_object_count")),
    "blocked_objects=" + str(promotion.get("blocked_object_count")),
    "promotable_associations=" + str(promotion.get("promotable_association_count")),
    "blocked_associations=" + str(promotion.get("blocked_association_count")),
)

for label in ["raw", "repaired", "generic_baseline", "seed_only", "typed_row_baseline"]:
    path = out / f"{label}_eval.json"
    data = json.loads(path.read_text(encoding="utf-8"))
    golden = data.get("golden_eval", {})
    print(
        "real_connector_probe",
        label,
        "passes_eval=" + str(data.get("passes_eval")),
        "passes_smoke_eval=" + str(data.get("passes_smoke_eval")),
        "golden=" + str(golden.get("pass_count")) + "/" + str(golden.get("question_count")),
        "repair_applied=" + str(data.get("repair_applied", False)),
    )
    if label == "raw" and require_raw and not data.get("passes_eval"):
        failures.append("raw model answer failed required gate")
    if label == "repaired" and require_repaired and not data.get("passes_eval"):
        failures.append("repaired model answer failed required gate")

repaired_eval = json.loads((out / "repaired_eval.json").read_text(encoding="utf-8"))
raw = repaired_eval.get("raw_answer_eval") or {}
if raw:
    print("real_connector_probe raw_unknown_citations=" + json.dumps(raw.get("unknown_citations", [])))
    print("real_connector_probe raw_policy_violations=" + json.dumps(raw.get("citation_policy_violations", [])))
    print("real_connector_probe raw_unsupported_statement_count=" + str(raw.get("unsupported_statement_count")))

comparison = json.loads((out / "comparison.json").read_text(encoding="utf-8"))
for gate in comparison.get("promotion_gates", []):
    total = gate.get("candidate_pass_count", 0) + gate.get("candidate_failure_count", 0)
    baseline_total = gate.get("baseline_pass_count", 0) + gate.get("baseline_failure_count", 0)
    print(
        "real_connector_probe",
        gate.get("key"),
        "passes=" + str(gate.get("passes")),
        "candidate=" + str(gate.get("candidate_pass_count")) + "/" + str(total),
        "baseline=" + str(gate.get("baseline_pass_count")) + "/" + str(baseline_total),
    )

if failures:
    raise SystemExit("; ".join(failures))
PY
