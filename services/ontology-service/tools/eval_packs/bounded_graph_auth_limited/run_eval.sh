#!/bin/sh
set -eu

SCRIPT_DIR=$(CDPATH= cd -- "$(dirname -- "$0")" && pwd)
ROOT_DIR=$(CDPATH= cd -- "$SCRIPT_DIR/../../.." && pwd)
OUT_DIR=${OUT_DIR:-/tmp/bounded_graph_auth_limited}
mkdir -p "$OUT_DIR"
cd "$ROOT_DIR"

.venv/bin/python tools/bounded_graph_contract.py \
  --bounded-graph-context-json "$SCRIPT_DIR/context.json" \
  --report-json "$OUT_DIR/contract.json"

.venv/bin/python tools/bounded_graph_brief.py \
  --bounded-graph-context-json "$SCRIPT_DIR/context.json" \
  --context-json "$OUT_DIR/context.normalized.json" \
  --brief-md "$OUT_DIR/scaffold.md" \
  --generic-baseline-md "$OUT_DIR/generic_baseline.md" \
  --prompt-mode generic \
  --prompt-md "$OUT_DIR/prompt.md"

.venv/bin/python tools/bounded_graph_brief.py \
  --bounded-graph-context-json "$SCRIPT_DIR/context.json" \
  --context-json "$OUT_DIR/context.eval.json" \
  --brief-md "$OUT_DIR/scaffold_eval.md" \
  --prompt-mode generic \
  --prompt-md "$OUT_DIR/prompt_eval.md" \
  --llm-brief-md "$OUT_DIR/generic_baseline.md" \
  --evaluation-json "$OUT_DIR/eval.json" \
  --golden-json "$SCRIPT_DIR/golden_questions.json"

.venv/bin/python - "$OUT_DIR/eval.json" <<'PY'
import json
import sys
data = json.load(open(sys.argv[1], encoding="utf-8"))
golden = data.get("golden_eval", {})
print(
    "generic_baseline_eval",
    "passes_eval=" + str(data.get("passes_eval")),
    "passes_smoke_eval=" + str(data.get("passes_smoke_eval")),
    "golden=" + str(golden.get("pass_count")) + "/" + str(golden.get("question_count")),
)
if not data.get("passes_eval"):
    sys.exit(1)
PY
