#!/bin/sh
set -eu

SCRIPT_DIR=$(CDPATH= cd -- "$(dirname -- "$0")" && pwd)
ROOT_DIR=$(CDPATH= cd -- "$SCRIPT_DIR/../../.." && pwd)
OUT_DIR=${OUT_DIR:-/tmp/incident_runbook_minimum}

mkdir -p "$OUT_DIR"
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
visible_distractor_context_json="$OUT_DIR/visible_distractor_context.json"
visible_distractor_normalized_json="$OUT_DIR/visible_distractor_normalized.json"
visible_distractor_scaffold_md="$OUT_DIR/visible_distractor_scaffold.md"
visible_distractor_baseline_md="$OUT_DIR/visible_distractor_baseline.md"
answers_json="$OUT_DIR/answers.json"
comparison_json="$OUT_DIR/promotion_comparison.json"

go run ./cmd/ontology-service bounded-graph-context-export \
  --fixture customer-incident-runbook \
  --start-object-type customer_account \
  --start-key customer-account:acme \
  --association-types reported_incident,has_update,has_runbook \
  --depth 2 \
  --limit-per-object 4 \
  --coverage-state sparse \
  --absence-claim-gate-reason partial_incident_sources \
  --coverage-summary "Only selected customer incident, update, and runbook rows were loaded." \
  --guardrails "Shared channels are high-degree context; do not cross them unless the query requests channel traversal." \
  --out "$context_json"

python3 - "$context_json" <<'PY'
import json
import sys

payload = json.load(open(sys.argv[1], encoding="utf-8"))
context = payload["boundedGraphContext"]
text = json.dumps(context, sort_keys=True)
for forbidden in [
    "incident:finance-export",
    "runbook:finance-export",
    "slack-channel:customer-incidents",
    "ticket:",
    "pull-request:",
    "WorkProgram",
    "TPM",
    "Flink",
]:
    if forbidden in text:
        raise SystemExit("filtered incident context leaked " + forbidden)
PY

.venv/bin/python tools/bounded_graph_contract.py \
  --bounded-graph-context-json "$context_json" \
  --report-json "$OUT_DIR/contract.json"

.venv/bin/python tools/bounded_graph_brief.py \
  --bounded-graph-context-json "$context_json" \
  --context-json "$normalized_json" \
  --brief-md "$scaffold_md" \
  --generic-baseline-md "$baseline_md" \
  --typed-row-baseline-md "$typed_row_baseline_md" \
  --prompt-mode generic \
  --prompt-md "$prompt_md"

python3 - "$context_json" "$visible_distractor_context_json" <<'PY'
import copy
import json
import sys

input_path, output_path = sys.argv[1:3]
payload = json.load(open(input_path, encoding="utf-8"))
out = copy.deepcopy(payload)
context = out["boundedGraphContext"]
context.setdefault("objects", []).extend(
    [
        {
            "objectType": "incident",
            "key": "incident:finance-export",
            "title": "Finance export incident",
            "claimAllowed": True,
            "proofState": "source_observed",
            "visibility": "public",
            "freshnessState": "fresh",
            "rankScore": 999,
            "sourceInstance": "customer-incident-runbook",
        },
        {
            "objectType": "runbook_document",
            "key": "runbook:finance-export",
            "title": "Finance export runbook",
            "claimAllowed": True,
            "proofState": "source_observed",
            "visibility": "public",
            "freshnessState": "fresh",
            "rankScore": 999,
            "sourceInstance": "customer-incident-runbook",
        },
    ]
)
context.setdefault("associations", []).append(
    {
        "key": "visible-distractor:finance-runbook",
        "associationType": "has_runbook",
        "from": {"objectType": "incident", "key": "incident:finance-export"},
        "to": {"objectType": "runbook_document", "key": "runbook:finance-export"},
        "evidenceKey": "evidence:incident:finance-runbook",
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
        "key": "evidence:incident:finance-runbook",
        "source": "generic_sampledata",
        "sourceInstance": "customer-incident-runbook",
        "visibility": "public",
        "freshnessState": "fresh",
        "confidence": 1,
    }
)
open(output_path, "w", encoding="utf-8").write(json.dumps(out, indent=2, sort_keys=True))
PY

.venv/bin/python tools/bounded_graph_brief.py \
  --bounded-graph-context-json "$visible_distractor_context_json" \
  --context-json "$visible_distractor_normalized_json" \
  --brief-md "$visible_distractor_scaffold_md" \
  --generic-baseline-md "$visible_distractor_baseline_md" \
  --prompt-mode generic \
  --prompt-md "$OUT_DIR/visible_distractor_prompt.md"

python3 - "$visible_distractor_baseline_md" <<'PY'
import pathlib
import sys

text = pathlib.Path(sys.argv[1]).read_text(encoding="utf-8")
for forbidden in [
    "incident:finance-export",
    "runbook:finance-export",
    "finance export",
    "Finance export",
]:
    if forbidden in text:
        raise SystemExit("visible distractor selected: " + forbidden)
PY

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
  --fixture customer-incident-runbook \
  --start-object-type customer_account \
  --start-key customer-account:acme \
  --association-types reported_incident,has_update,has_runbook \
  --depth 0 \
  --limit-per-object 4 \
  --coverage-state sparse \
  --absence-claim-gate-reason partial_incident_sources \
  --coverage-summary "Only selected customer incident, update, and runbook rows were loaded." \
  --guardrails "Shared channels are high-degree context; do not cross them unless the query requests channel traversal." \
  --out "$seed_only_context_json"

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

python3 - "$answers_json" "$baseline_md" "$seed_only_baseline_md" "$typed_row_baseline_md" <<'PY'
import json
import sys

answers_path, depth2_path, seed_only_path, typed_row_path = sys.argv[1:5]
payload = {
    "answers": [
        {
            "key": "depth_2_graph",
            "label": "Depth-2 incident graph traversal",
            "path": depth2_path,
            "strategy": "bounded_graph_context_depth_2",
            "answer_kind": "candidate",
        },
        {
            "key": "depth_0_seed",
            "label": "Seed-only customer account",
            "path": seed_only_path,
            "strategy": "bounded_graph_context_depth_0",
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
  --golden-json "$SCRIPT_DIR/golden_questions.json" \
  --compare-answers-json "$answers_json" \
  --comparison-json "$comparison_json" \
  --require-promotion-gates

python3 - "$eval_json" "$seed_only_eval_json" "$typed_row_eval_json" "$comparison_json" <<'PY'
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
    "incident_runbook",
    "passes_eval=" + str(data.get("passes_eval")),
    "passes_smoke_eval=" + str(data.get("passes_smoke_eval")),
    "golden=" + str(golden.get("pass_count")) + "/" + str(golden.get("question_count")),
)
for gate in comparison.get("promotion_gates", []):
    print(
        "incident_runbook",
        gate.get("key"),
        "passes=" + str(gate.get("passes")),
        "candidate=" + str(gate.get("candidate_pass_count")) + "/" + str(golden.get("question_count")),
        "baseline=" + str(gate.get("baseline_pass_count")) + "/" + str(golden.get("question_count")),
    )
print(
    "incident_runbook",
    "seed_only_golden=" + str(seed_golden.get("pass_count")) + "/" + str(seed_golden.get("question_count")),
    "typed_row_golden=" + str(typed_golden.get("pass_count")) + "/" + str(typed_golden.get("question_count")),
)
if not data.get("passes_eval") or not comparison.get("passes_promotion_gates"):
    sys.exit(1)
PY
