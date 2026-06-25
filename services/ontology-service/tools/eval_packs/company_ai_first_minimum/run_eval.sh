#!/bin/sh
set -eu

SCRIPT_DIR=$(CDPATH= cd -- "$(dirname -- "$0")" && pwd)
ROOT_DIR=$(CDPATH= cd -- "$SCRIPT_DIR/../../.." && pwd)
OUT_DIR=${OUT_DIR:-/tmp/company_ai_first_minimum}
DB="$OUT_DIR/company_ai_first.db"
SOURCE_AUTHORITY_JSON="$ROOT_DIR/internal/graphcontext/source_authority.json"

mkdir -p "$OUT_DIR"
rm -f "$DB" "$DB-wal" "$DB-shm" "$DB-journal"
cd "$ROOT_DIR"

assert_no_distractors() {
  context_json=$1
  .venv/bin/python - "$context_json" <<'PY'
import json
import sys

path = sys.argv[1]
payload = json.load(open(path, encoding="utf-8"))
context = payload.get("boundedGraphContext") or payload.get("data", {}).get("boundedGraphContext") or {}
objects = context.get("objects") or []
associations = context.get("associations") or []
forbidden = {
    "ticket:COMP-999",
    "pull-request:company/app#99",
    "person:mallory",
    "document:unrelated-roadmap",
    "message:finance-thread",
}
object_hits = sorted({str(row.get("key") or "") for row in objects} & forbidden)
association_hits = sorted(
    key
    for row in associations
    for key in [
        str((row.get("from") or {}).get("key") or ""),
        str((row.get("to") or {}).get("key") or ""),
    ]
    if key in forbidden
)
if object_hits or association_hits:
    raise SystemExit(
        "distractor leakage in "
        + path
        + ": objects="
        + ",".join(object_hits)
        + " associations="
        + ",".join(association_hits)
    )
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

write_visible_distractor_context() {
  input_json=$1
  output_json=$2
  .venv/bin/python - "$input_json" "$output_json" <<'PY'
import copy
import json
import sys

input_path, output_path = sys.argv[1:3]
payload = json.load(open(input_path, encoding="utf-8"))
out = copy.deepcopy(payload)
context = out.get("boundedGraphContext") or out.get("data", {}).get("boundedGraphContext")
if not isinstance(context, dict):
    raise SystemExit("missing boundedGraphContext in " + input_path)
objects = context.setdefault("objects", [])
associations = context.setdefault("associations", [])
objects.extend(
    [
        {
            "objectType": "ticket",
            "key": "ticket:COMP-999",
            "title": "Unrelated finance export",
            "claimAllowed": True,
            "proofState": "source_observed",
            "visibility": "public",
            "freshnessState": "fresh",
            "rankScore": 999,
            "sourceInstance": "company-ai-first-minimum",
        },
        {
            "objectType": "pull_request",
            "key": "pull-request:company/app#99",
            "title": "Export finance report",
            "claimAllowed": True,
            "proofState": "source_observed",
            "visibility": "public",
            "freshnessState": "fresh",
            "rankScore": 999,
            "sourceInstance": "company/app",
        },
    ]
)
associations.append(
    {
        "key": "visible-distractor:comp-999-pr-99",
        "associationType": "implemented_by",
        "from": {"objectType": "ticket", "key": "ticket:COMP-999"},
        "to": {"objectType": "pull_request", "key": "pull-request:company/app#99"},
        "evidenceKey": "evidence:company:comp-999:pr-99",
        "confidence": 1,
        "visibility": "public",
        "freshnessState": "fresh",
        "proofState": "source_observed",
        "claimAllowed": True,
        "claimGateReason": "visible_distractor_should_not_be_selected",
    }
)
context.setdefault("evidence", []).append(
    {
        "key": "evidence:company:comp-999:pr-99",
        "source": "company_fixture",
        "sourceInstance": "company-ai-first-minimum",
        "visibility": "public",
        "freshnessState": "fresh",
        "confidence": 1,
    }
)
open(output_path, "w", encoding="utf-8").write(json.dumps(out, indent=2, sort_keys=True))
PY
}

assert_no_distractor_answer_mentions() {
  answer_md=$1
  .venv/bin/python - "$answer_md" <<'PY'
import pathlib
import sys

path = pathlib.Path(sys.argv[1])
text = path.read_text(encoding="utf-8")
forbidden = [
    "ticket:COMP-999",
    "pull-request:company/app#99",
    "person:mallory",
    "document:unrelated-roadmap",
    "message:finance-thread",
    "COMP-999",
    "finance",
    "Mallory",
    "#99",
]
hits = [value for value in forbidden if value in text]
if hits:
    raise SystemExit("visible distractor selected in " + str(path) + ": " + ", ".join(hits))
PY
}

run_seed() {
  name=$1
  object_type=$2
  object_key=$3
  golden=$4
  seed_flags=$5

  context_json="$OUT_DIR/${name}_context.json"
  normalized_json="$OUT_DIR/${name}_normalized.json"
  scaffold_md="$OUT_DIR/${name}_scaffold.md"
  prompt_md="$OUT_DIR/${name}_prompt.md"
  baseline_md="$OUT_DIR/${name}_generic_baseline.md"
  eval_json="$OUT_DIR/${name}_eval.json"
  typed_row_baseline_md="$OUT_DIR/${name}_typed_row_baseline.md"
  typed_row_eval_json="$OUT_DIR/${name}_typed_row_eval.json"
  visible_distractor_context_json="$OUT_DIR/${name}_visible_distractor_context.json"
  visible_distractor_normalized_json="$OUT_DIR/${name}_visible_distractor_normalized.json"
  visible_distractor_scaffold_md="$OUT_DIR/${name}_visible_distractor_scaffold.md"
  visible_distractor_baseline_md="$OUT_DIR/${name}_visible_distractor_baseline.md"
  seed_only_context_json="$OUT_DIR/${name}_seed_only_context.json"
  seed_only_normalized_json="$OUT_DIR/${name}_seed_only_normalized.json"
  seed_only_scaffold_md="$OUT_DIR/${name}_seed_only_scaffold.md"
  seed_only_prompt_md="$OUT_DIR/${name}_seed_only_prompt.md"
  seed_only_baseline_md="$OUT_DIR/${name}_seed_only_baseline.md"
  seed_only_eval_json="$OUT_DIR/${name}_seed_only_eval.json"
  answers_json="$OUT_DIR/${name}_answers.json"
  comparison_json="$OUT_DIR/${name}_promotion_comparison.json"

  if [ "$seed_flags" = "seed" ]; then
    go run ./cmd/ontology-service bounded-graph-context-export \
      --fixture company-ai-first-minimum \
      --database "$DB" \
      --seed-fixture \
      --reset-database \
      --start-object-type "$object_type" \
      --start-key "$object_key" \
      --depth 2 \
      --limit-per-object 8 \
      --out "$context_json"
  else
    go run ./cmd/ontology-service bounded-graph-context-export \
      --fixture company-ai-first-minimum \
      --database "$DB" \
      --start-object-type "$object_type" \
      --start-key "$object_key" \
      --depth 2 \
      --limit-per-object 8 \
      --out "$context_json"
  fi

  assert_no_distractors "$context_json"
  .venv/bin/python tools/bounded_graph_contract.py \
    --bounded-graph-context-json "$context_json" \
    --report-json "$OUT_DIR/${name}_contract.json"
  .venv/bin/python tools/bounded_graph_promotion_audit.py \
    --bounded-graph-context-json "$context_json" \
    --source-authority-json "$SOURCE_AUTHORITY_JSON" \
    --report-json "$OUT_DIR/${name}_promotion_audit.json"
  assert_promotion_audit_full_coverage "$OUT_DIR/${name}_promotion_audit.json"
  if [ "$name" != "person" ]; then
    .venv/bin/python tools/bounded_graph_contract.py \
      --bounded-graph-context-json "$context_json" \
      --profile connector \
      --report-json "$OUT_DIR/${name}_connector_contract.json"
    .venv/bin/python tools/bounded_graph_promotion_audit.py \
      --bounded-graph-context-json "$context_json" \
      --profile connector \
      --source-authority-json "$SOURCE_AUTHORITY_JSON" \
      --report-json "$OUT_DIR/${name}_connector_promotion_audit.json"
    assert_promotion_audit_full_coverage "$OUT_DIR/${name}_connector_promotion_audit.json"
  fi

  .venv/bin/python tools/bounded_graph_brief.py \
    --bounded-graph-context-json "$context_json" \
    --context-json "$normalized_json" \
    --brief-md "$scaffold_md" \
    --generic-baseline-md "$baseline_md" \
    --typed-row-baseline-md "$typed_row_baseline_md" \
    --prompt-mode generic \
    --prompt-md "$prompt_md"

  write_visible_distractor_context "$context_json" "$visible_distractor_context_json"
  .venv/bin/python tools/bounded_graph_brief.py \
    --bounded-graph-context-json "$visible_distractor_context_json" \
    --context-json "$visible_distractor_normalized_json" \
    --brief-md "$visible_distractor_scaffold_md" \
    --generic-baseline-md "$visible_distractor_baseline_md" \
    --prompt-mode generic \
    --prompt-md "$OUT_DIR/${name}_visible_distractor_prompt.md"
  assert_no_distractor_answer_mentions "$visible_distractor_baseline_md"

  .venv/bin/python tools/bounded_graph_brief.py \
    --bounded-graph-context-json "$context_json" \
    --context-json "$OUT_DIR/${name}_eval_context.json" \
    --brief-md "$OUT_DIR/${name}_eval_scaffold.md" \
    --prompt-mode generic \
    --prompt-md "$OUT_DIR/${name}_eval_prompt.md" \
    --llm-brief-md "$baseline_md" \
    --evaluation-json "$eval_json" \
    --golden-json "$SCRIPT_DIR/$golden"

  .venv/bin/python tools/bounded_graph_brief.py \
    --bounded-graph-context-json "$context_json" \
    --context-json "$OUT_DIR/${name}_typed_row_eval_context.json" \
    --brief-md "$OUT_DIR/${name}_typed_row_eval_scaffold.md" \
    --prompt-mode generic \
    --prompt-md "$OUT_DIR/${name}_typed_row_eval_prompt.md" \
    --llm-brief-md "$typed_row_baseline_md" \
    --evaluation-json "$typed_row_eval_json" \
    --golden-json "$SCRIPT_DIR/$golden"

  .venv/bin/python - "$name" "$eval_json" <<'PY'
import json
import sys

name = sys.argv[1]
data = json.load(open(sys.argv[2], encoding="utf-8"))
golden = data.get("golden_eval", {})
print(
    name,
    "passes_eval=" + str(data.get("passes_eval")),
    "passes_smoke_eval=" + str(data.get("passes_smoke_eval")),
    "golden=" + str(golden.get("pass_count")) + "/" + str(golden.get("question_count")),
)
if not data.get("passes_eval"):
    sys.exit(1)
PY

  go run ./cmd/ontology-service bounded-graph-context-export \
    --fixture company-ai-first-minimum \
    --database "$DB" \
    --start-object-type "$object_type" \
    --start-key "$object_key" \
    --depth 0 \
    --limit-per-object 8 \
    --out "$seed_only_context_json"

  assert_no_distractors "$seed_only_context_json"
  .venv/bin/python tools/bounded_graph_contract.py \
    --bounded-graph-context-json "$seed_only_context_json" \
    --report-json "$OUT_DIR/${name}_seed_only_contract.json"

  .venv/bin/python tools/bounded_graph_brief.py \
    --bounded-graph-context-json "$seed_only_context_json" \
    --context-json "$seed_only_normalized_json" \
    --brief-md "$seed_only_scaffold_md" \
    --generic-baseline-md "$seed_only_baseline_md" \
    --prompt-mode generic \
    --prompt-md "$seed_only_prompt_md"

  .venv/bin/python tools/bounded_graph_brief.py \
    --bounded-graph-context-json "$seed_only_context_json" \
    --context-json "$OUT_DIR/${name}_seed_only_eval_context.json" \
    --brief-md "$OUT_DIR/${name}_seed_only_eval_scaffold.md" \
    --prompt-mode generic \
    --prompt-md "$OUT_DIR/${name}_seed_only_eval_prompt.md" \
    --llm-brief-md "$seed_only_baseline_md" \
    --evaluation-json "$seed_only_eval_json" \
    --golden-json "$SCRIPT_DIR/$golden"

  .venv/bin/python - "$answers_json" "$baseline_md" "$seed_only_baseline_md" "$typed_row_baseline_md" <<'PY'
import json
import sys

answers_path, depth2_path, seed_only_path, typed_row_path = sys.argv[1:5]
payload = {
    "answers": [
        {
            "key": "depth_2_graph",
            "label": "Depth-2 bounded graph traversal",
            "path": depth2_path,
            "strategy": "bounded_graph_context_depth_2",
            "answer_kind": "candidate",
        },
        {
            "key": "depth_0_seed",
            "label": "Seed-only bounded graph context",
            "path": seed_only_path,
            "strategy": "bounded_graph_context_depth_0",
            "answer_kind": "baseline",
        },
        {
            "key": "typed_rows",
            "label": "Typed object rows without graph associations",
            "path": typed_row_path,
            "strategy": "typed_row_object_summary",
            "answer_kind": "baseline",
        },
    ],
    "promotion_gates": [
        {
            "key": "depth_2_over_seed_only",
            "candidate_key": "depth_2_graph",
            "baseline_key": "depth_0_seed",
        },
        {
            "key": "depth_2_over_typed_rows",
            "candidate_key": "depth_2_graph",
            "baseline_key": "typed_rows",
        },
    ],
}
open(answers_path, "w", encoding="utf-8").write(json.dumps(payload, indent=2, sort_keys=True))
PY

  .venv/bin/python tools/cubicle_graph_brief.py \
    --golden-json "$SCRIPT_DIR/$golden" \
    --compare-answers-json "$answers_json" \
    --comparison-json "$comparison_json" \
    --require-promotion-gates

  .venv/bin/python - "$name" "$seed_only_eval_json" "$typed_row_eval_json" "$comparison_json" <<'PY'
import json
import sys

name, seed_eval_path, typed_eval_path, comparison_path = sys.argv[1:5]
seed_eval = json.load(open(seed_eval_path, encoding="utf-8"))
typed_eval = json.load(open(typed_eval_path, encoding="utf-8"))
seed_golden = seed_eval.get("golden_eval", {})
typed_golden = typed_eval.get("golden_eval", {})
comparison = json.load(open(comparison_path, encoding="utf-8"))
gates = {row.get("key"): row for row in comparison.get("promotion_gates", [])}
seed_gate = gates.get("depth_2_over_seed_only", {})
typed_gate = gates.get("depth_2_over_typed_rows", {})
print(
    name,
    "seed_only_golden=" + str(seed_golden.get("pass_count")) + "/" + str(seed_golden.get("question_count")),
    "seed_promotion_gate=" + str(seed_gate.get("passes")),
    "seed_candidate=" + str(seed_gate.get("candidate_pass_count")) + "/" + str(seed_gate.get("candidate_pass_count", 0) + seed_gate.get("candidate_failure_count", 0)),
    "seed_baseline=" + str(seed_gate.get("baseline_pass_count")) + "/" + str(seed_gate.get("baseline_pass_count", 0) + seed_gate.get("baseline_failure_count", 0)),
    "typed_row_golden=" + str(typed_golden.get("pass_count")) + "/" + str(typed_golden.get("question_count")),
    "typed_row_promotion_gate=" + str(typed_gate.get("passes")),
    "typed_candidate=" + str(typed_gate.get("candidate_pass_count")) + "/" + str(typed_gate.get("candidate_pass_count", 0) + typed_gate.get("candidate_failure_count", 0)),
    "typed_baseline=" + str(typed_gate.get("baseline_pass_count")) + "/" + str(typed_gate.get("baseline_pass_count", 0) + typed_gate.get("baseline_failure_count", 0)),
)
if not seed_gate.get("passes") or not typed_gate.get("passes"):
    sys.exit(1)
PY
}

run_seed document document document:company-plan golden_document.json seed
run_seed person person person:alice golden_person.json reuse
run_seed pull_request pull_request pull-request:company/app#42 golden_pull_request.json reuse
run_seed ticket ticket ticket:COMP-101 golden_ticket.json reuse
run_seed message message message:launch-standup golden_message.json reuse
