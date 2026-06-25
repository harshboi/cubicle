#!/usr/bin/env bash
set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
SERVICE_ROOT="$(cd "$SCRIPT_DIR/../../.." && pwd)"
OUT_DIR="${OUT_DIR:-/tmp/bounded_graph_cli_demo}"
MLX_MODEL="${MLX_MODEL:-mlx-community/Qwen3-Coder-30B-A3B-Instruct-bf16}"
MLX_PYTHON="${MLX_PYTHON:-/Users/harsh/.venv-vllm-metal/bin/python}"
LLM_MAX_TOKENS="${LLM_MAX_TOKENS:-8192}"
LLM_TIMEOUT_SECONDS="${LLM_TIMEOUT_SECONDS:-600}"

mkdir -p "$OUT_DIR"
rm -f \
  "$OUT_DIR/generic_baseline_eval.json" \
  "$OUT_DIR/mlx_raw_eval.json" \
  "$OUT_DIR/mlx_repaired_eval.json"
cd "$SERVICE_ROOT"

go run ./cmd/ontology-service bounded-graph-context-export \
  --out "$OUT_DIR/context.json"

.venv/bin/python tools/bounded_graph_contract.py \
  --bounded-graph-context-json "$OUT_DIR/context.json" \
  --report-json "$OUT_DIR/contract.json"

.venv/bin/python tools/bounded_graph_brief.py \
  --bounded-graph-context-json "$OUT_DIR/context.json" \
  --context-json "$OUT_DIR/normalized.json" \
  --brief-md "$OUT_DIR/scaffold.md" \
  --generic-baseline-md "$OUT_DIR/generic_baseline.md" \
  --prompt-mode generic \
  --prompt-md "$OUT_DIR/prompt.md"

.venv/bin/python tools/bounded_graph_brief.py \
  --bounded-graph-context-json "$OUT_DIR/context.json" \
  --context-json "$OUT_DIR/baseline_eval_context.json" \
  --brief-md "$OUT_DIR/baseline_eval_scaffold.md" \
  --prompt-mode generic \
  --prompt-md "$OUT_DIR/baseline_eval_prompt.md" \
  --llm-brief-md "$OUT_DIR/generic_baseline.md" \
  --evaluation-json "$OUT_DIR/generic_baseline_eval.json" \
  --golden-json "$SCRIPT_DIR/golden_questions.json"

if [[ "${RUN_MLX:-0}" == "1" ]]; then
  .venv/bin/python tools/bounded_graph_brief.py \
    --bounded-graph-context-json "$OUT_DIR/context.json" \
    --context-json "$OUT_DIR/mlx_context.json" \
    --brief-md "$OUT_DIR/mlx_scaffold.md" \
    --prompt-mode generic \
    --prompt-md "$OUT_DIR/mlx_prompt.md" \
    --mlx-model "$MLX_MODEL" \
    --mlx-python "$MLX_PYTHON" \
    --llm-max-tokens "$LLM_MAX_TOKENS" \
    --llm-timeout-seconds "$LLM_TIMEOUT_SECONDS" \
    --llm-brief-md "$OUT_DIR/mlx_raw.md" \
    --evaluation-json "$OUT_DIR/mlx_raw_eval.json" \
    --golden-json "$SCRIPT_DIR/golden_questions.json"

  .venv/bin/python tools/bounded_graph_brief.py \
    --bounded-graph-context-json "$OUT_DIR/context.json" \
    --context-json "$OUT_DIR/mlx_repaired_context.json" \
    --brief-md "$OUT_DIR/mlx_repaired_scaffold.md" \
    --prompt-mode generic \
    --prompt-md "$OUT_DIR/mlx_repaired_prompt.md" \
    --llm-brief-md "$OUT_DIR/mlx_raw.md" \
    --repaired-brief-md "$OUT_DIR/mlx_repaired.md" \
    --evaluation-json "$OUT_DIR/mlx_repaired_eval.json" \
    --golden-json "$SCRIPT_DIR/golden_questions.json"
fi

python3 - "$OUT_DIR" <<'PY'
import json
import sys
from pathlib import Path

out = Path(sys.argv[1])
for name in ["generic_baseline_eval", "mlx_raw_eval", "mlx_repaired_eval"]:
    path = out / f"{name}.json"
    if not path.exists():
        continue
    data = json.loads(path.read_text())
    golden = data.get("golden_eval", {})
    print(
        name,
        "passes_eval=" + str(data.get("passes_eval")),
        "passes_smoke_eval=" + str(data.get("passes_smoke_eval")),
        "golden=" + str(golden.get("pass_count")) + "/" + str(golden.get("question_count")),
        "repair_applied=" + str(data.get("repair_applied", False)),
    )
    raw = data.get("raw_answer_eval")
    if raw:
        print(" ", "raw_unknown_citations=", raw.get("unknown_citations", []))
        print(" ", "raw_policy_violations=", raw.get("citation_policy_violations", []))
PY
