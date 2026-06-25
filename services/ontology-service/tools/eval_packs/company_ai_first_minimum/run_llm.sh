#!/usr/bin/env bash
set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
ROOT_DIR="$(cd "$SCRIPT_DIR/../../.." && pwd)"
OUT_DIR="${OUT_DIR:-/tmp/company_ai_first_llm}"
DB="$OUT_DIR/company_ai_first.db"

MODEL="${MODEL:-mlx-community/Qwen3-Coder-30B-A3B-Instruct-bf16}"
MLX_PYTHON="${MLX_PYTHON:-/Users/harsh/.venv-vllm-metal/bin/python}"
LLM_MAX_TOKENS="${LLM_MAX_TOKENS:-24576}"
LLM_TIMEOUT_SECONDS="${LLM_TIMEOUT_SECONDS:-1200}"
SEEDS="${SEEDS:-message}"
REQUIRE_RAW_PASS="${REQUIRE_RAW_PASS:-0}"
REQUIRE_REPAIRED_PASS="${REQUIRE_REPAIRED_PASS:-1}"

if [[ "$SEEDS" == "all" ]]; then
  SEEDS="document person pull_request ticket message"
fi

mkdir -p "$OUT_DIR"
rm -f "$DB" "$DB-wal" "$DB-shm" "$DB-journal"
cd "$ROOT_DIR"

seeded=0

seed_args() {
  local seed_name=$1
  case "$seed_name" in
    document) echo "document document:company-plan" ;;
    person) echo "person person:alice" ;;
    pull_request) echo "pull_request pull-request:company/app#42" ;;
    ticket) echo "ticket ticket:COMP-101" ;;
    message) echo "message message:launch-standup" ;;
    *) echo "unknown seed: $seed_name" >&2; exit 2 ;;
  esac
}

run_seed() {
  local seed_name=$1
  local object_type object_key golden_json
  read -r object_type object_key <<<"$(seed_args "$seed_name")"
  golden_json="$SCRIPT_DIR/golden_${seed_name}.json"

  local context_json="$OUT_DIR/${seed_name}_context.json"
  local normalized_json="$OUT_DIR/${seed_name}_normalized.json"
  local scaffold_md="$OUT_DIR/${seed_name}_scaffold.md"
  local prompt_md="$OUT_DIR/${seed_name}_prompt.md"
  local generic_baseline_md="$OUT_DIR/${seed_name}_generic_baseline.md"
  local generic_baseline_eval_json="$OUT_DIR/${seed_name}_generic_baseline_eval.json"
  local typed_row_baseline_md="$OUT_DIR/${seed_name}_typed_row_baseline.md"
  local typed_row_baseline_eval_json="$OUT_DIR/${seed_name}_typed_row_baseline_eval.json"
  local raw_md="$OUT_DIR/${seed_name}_raw.md"
  local raw_eval_json="$OUT_DIR/${seed_name}_raw_eval.json"
  local repaired_md="$OUT_DIR/${seed_name}_repaired.md"
  local repaired_eval_json="$OUT_DIR/${seed_name}_repaired_eval.json"

  local export_args=(
    go run ./cmd/ontology-service bounded-graph-context-export
    --fixture company-ai-first-minimum
    --database "$DB"
    --start-object-type "$object_type"
    --start-key "$object_key"
    --depth 2
    --limit-per-object 8
    --out "$context_json"
  )
  if [[ "$seeded" == "0" ]]; then
    export_args+=(--seed-fixture --reset-database)
    seeded=1
  fi
  "${export_args[@]}"

  .venv/bin/python tools/bounded_graph_contract.py \
    --bounded-graph-context-json "$context_json" \
    --report-json "$OUT_DIR/${seed_name}_contract.json"
  if [[ "$seed_name" != "person" ]]; then
    .venv/bin/python tools/bounded_graph_contract.py \
      --bounded-graph-context-json "$context_json" \
      --profile connector \
      --report-json "$OUT_DIR/${seed_name}_connector_contract.json"
  fi

  local raw_args=(
    .venv/bin/python tools/bounded_graph_brief.py
    --bounded-graph-context-json "$context_json" \
    --context-json "$normalized_json" \
    --brief-md "$scaffold_md" \
    --generic-baseline-md "$generic_baseline_md" \
    --typed-row-baseline-md "$typed_row_baseline_md" \
    --prompt-mode generic \
    --prompt-md "$prompt_md" \
    --llm-timeout-seconds "$LLM_TIMEOUT_SECONDS" \
    --llm-brief-md "$raw_md" \
    --evaluation-json "$raw_eval_json" \
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
    --context-json "$OUT_DIR/${seed_name}_repaired_context.json" \
    --brief-md "$OUT_DIR/${seed_name}_repaired_scaffold.md" \
    --prompt-mode generic \
    --prompt-md "$OUT_DIR/${seed_name}_repaired_prompt.md" \
    --llm-brief-md "$raw_md" \
    --repaired-brief-md "$repaired_md" \
    --evaluation-json "$repaired_eval_json" \
    --golden-json "$golden_json"

  .venv/bin/python tools/bounded_graph_brief.py \
    --bounded-graph-context-json "$context_json" \
    --context-json "$OUT_DIR/${seed_name}_generic_baseline_eval_context.json" \
    --brief-md "$OUT_DIR/${seed_name}_generic_baseline_eval_scaffold.md" \
    --prompt-mode generic \
    --prompt-md "$OUT_DIR/${seed_name}_generic_baseline_eval_prompt.md" \
    --llm-brief-md "$generic_baseline_md" \
    --evaluation-json "$generic_baseline_eval_json" \
    --golden-json "$golden_json"

  .venv/bin/python tools/bounded_graph_brief.py \
    --bounded-graph-context-json "$context_json" \
    --context-json "$OUT_DIR/${seed_name}_typed_row_baseline_eval_context.json" \
    --brief-md "$OUT_DIR/${seed_name}_typed_row_baseline_eval_scaffold.md" \
    --prompt-mode generic \
    --prompt-md "$OUT_DIR/${seed_name}_typed_row_baseline_eval_prompt.md" \
    --llm-brief-md "$typed_row_baseline_md" \
    --evaluation-json "$typed_row_baseline_eval_json" \
    --golden-json "$golden_json"
}

for seed in $SEEDS; do
  run_seed "$seed"
done

python3 - "$OUT_DIR" $SEEDS <<'PY'
import json
import sys
from pathlib import Path

out = Path(sys.argv[1])
seeds = sys.argv[2:]
required_raw = bool(int(__import__("os").environ.get("REQUIRE_RAW_PASS", "0")))
required_repaired = bool(int(__import__("os").environ.get("REQUIRE_REPAIRED_PASS", "1")))
failures = []

for seed in seeds:
    for label in ["raw", "repaired", "generic_baseline", "typed_row_baseline"]:
        path = out / f"{seed}_{label}_eval.json"
        data = json.loads(path.read_text(encoding="utf-8"))
        golden = data.get("golden_eval", {})
        print(
            seed,
            label,
            "passes_eval=" + str(data.get("passes_eval")),
            "passes_smoke_eval=" + str(data.get("passes_smoke_eval")),
            "golden=" + str(golden.get("pass_count")) + "/" + str(golden.get("question_count")),
            "repair_applied=" + str(data.get("repair_applied", False)),
        )
        if label == "raw" and required_raw and not data.get("passes_eval"):
            failures.append(f"{seed} raw failed")
        if label == "repaired" and required_repaired and not data.get("passes_eval"):
            failures.append(f"{seed} repaired failed")
    raw = json.loads((out / f"{seed}_repaired_eval.json").read_text(encoding="utf-8")).get("raw_answer_eval", {})
    if raw:
        print(" ", seed, "raw_unknown_citations=", raw.get("unknown_citations", []))
        print(" ", seed, "raw_policy_violations=", raw.get("citation_policy_violations", []))

if failures:
    raise SystemExit("; ".join(failures))
PY
