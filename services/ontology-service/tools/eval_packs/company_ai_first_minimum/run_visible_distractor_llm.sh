#!/bin/sh
set -eu

SCRIPT_DIR=$(CDPATH= cd -- "$(dirname -- "$0")" && pwd)
ROOT_DIR=$(CDPATH= cd -- "$SCRIPT_DIR/../../.." && pwd)
OUT_DIR=${OUT_DIR:-/tmp/company_ai_first_visible_distractor_llm}
DB="$OUT_DIR/company_ai_first.db"

MODEL=${MODEL:-mlx-community/Qwen3-Coder-30B-A3B-Instruct-bf16}
MLX_PYTHON=${MLX_PYTHON:-/Users/harsh/.venv-vllm-metal/bin/python}
LLM_MAX_TOKENS=${LLM_MAX_TOKENS:-16384}
LLM_TIMEOUT_SECONDS=${LLM_TIMEOUT_SECONDS:-900}
SEEDS=${SEEDS:-message}
REQUIRE_RAW_PASS=${REQUIRE_RAW_PASS:-0}
REQUIRE_REPAIRED_PASS=${REQUIRE_REPAIRED_PASS:-1}
REQUIRE_RAW_NO_DISTRACTOR=${REQUIRE_RAW_NO_DISTRACTOR:-0}
REQUIRE_REPAIRED_NO_DISTRACTOR=${REQUIRE_REPAIRED_NO_DISTRACTOR:-1}

if [ "$SEEDS" = "all" ]; then
  SEEDS="document person pull_request ticket message"
fi

mkdir -p "$OUT_DIR"
rm -f "$DB" "$DB-wal" "$DB-shm" "$DB-journal"
cd "$ROOT_DIR"

SEEDED=0

seed_args() {
  seed_name=$1
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
  seed_name=$1
  set -- $(seed_args "$seed_name")
  object_type=$1
  object_key=$2

  context_json="$OUT_DIR/${seed_name}_context.json"
  visible_context_json="$OUT_DIR/${seed_name}_visible_distractor_context.json"
  visible_golden_json="$OUT_DIR/${seed_name}_golden_visible_distractor.json"
  normalized_json="$OUT_DIR/${seed_name}_visible_distractor_normalized.json"
  scaffold_md="$OUT_DIR/${seed_name}_visible_distractor_scaffold.md"
  prompt_md="$OUT_DIR/${seed_name}_visible_distractor_prompt.md"
  raw_md="$OUT_DIR/${seed_name}_visible_distractor_raw.md"
  raw_eval_json="$OUT_DIR/${seed_name}_visible_distractor_raw_eval.json"
  repaired_md="$OUT_DIR/${seed_name}_visible_distractor_repaired.md"
  repaired_eval_json="$OUT_DIR/${seed_name}_visible_distractor_repaired_eval.json"

  if [ "$SEEDED" = "0" ]; then
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
    SEEDED=1
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

  .venv/bin/python - "$context_json" "$visible_context_json" <<'PY'
import copy
import json
import sys

context_path, visible_context_path = sys.argv[1:3]
payload = json.load(open(context_path, encoding="utf-8"))
visible_payload = copy.deepcopy(payload)
context = visible_payload.get("boundedGraphContext") or visible_payload.get("data", {}).get("boundedGraphContext")
if not isinstance(context, dict):
    raise SystemExit("missing boundedGraphContext in " + context_path)
context.setdefault("objects", []).extend(
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
context.setdefault("associations", []).append(
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
json.dump(visible_payload, open(visible_context_path, "w", encoding="utf-8"), indent=2, sort_keys=True)
PY

  .venv/bin/python tools/cubicle_graph_brief.py \
    --bounded-graph-context-json "$visible_context_json" \
    --context-json "$normalized_json" \
    --brief-md "$scaffold_md" \
    --prompt-mode generic \
    --prompt-md "$prompt_md"

  .venv/bin/python - "$seed_name" "$normalized_json" "$visible_golden_json" <<'PY'
import json
import sys

seed_name, normalized_path, golden_path = sys.argv[1:4]
context = json.load(open(normalized_path, encoding="utf-8"))
rows = context.get("rows", {})
associations = rows.get("graph_associations", [])
context_hash = str(context.get("context_hash") or "")
seed_key = str(context.get("seed", {}).get("key") or "object:unknown")

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
    "visible-distractor",
]

expectations = {
    "document": [
        ("auth-limited-coverage", "limited", "[source_coverage:document:company-plan]", "limited source coverage"),
        ("document-link", "links_to", None, "document link"),
        ("ticket-document", "documented_by", None, "ticket document link"),
        ("ticket-pr", "implemented_by", None, "ticket PR implementation"),
    ],
    "person": [
        ("person-ticket", "assignee", None, "person ticket assignment"),
        ("person-pr", "author", None, "person PR authorship"),
        ("ticket-pr", "implemented_by", None, "ticket PR implementation"),
        ("sparse-coverage", "sparse", "[source_coverage:person:alice]", "sparse source coverage"),
    ],
    "pull_request": [
        ("ticket-pr", "implemented_by", None, "ticket PR implementation"),
        ("pr-author", "author", None, "PR author"),
        ("pr-approver", "approver", None, "PR approver"),
        ("sparse-coverage", "sparse", "[source_coverage:pull-request:company/app#42]", "sparse source coverage"),
    ],
    "ticket": [
        ("ticket-pr", "implemented_by", None, "ticket PR implementation"),
        ("ticket-document", "documented_by", None, "ticket document link"),
        ("ticket-message", "discussed_in", None, "ticket message discussion"),
        ("sparse-coverage", "sparse", "[source_coverage:ticket:COMP-101]", "sparse source coverage"),
    ],
    "message": [
        ("message-ticket", "discussed_in", None, "message ticket discussion"),
        ("ticket-document", "documented_by", None, "ticket document link"),
        ("ticket-pr", "implemented_by", None, "ticket PR implementation"),
        ("sparse-coverage", "sparse", "[source_coverage:message:launch-standup]", "sparse source coverage"),
    ],
}

def association_citation(association_type):
    for row in associations:
        if row.get("seed_reachable") is False:
            continue
        if str(row.get("association_type") or "") != association_type:
            continue
        from_key = str(row.get("from_key") or "")
        to_key = str(row.get("to_key") or "")
        if "COMP-999" in from_key or "COMP-999" in to_key or "company/app#99" in from_key or "company/app#99" in to_key:
            continue
        return "[graph_associations:" + str(row.get("key")) + "]"
    raise SystemExit(f"missing association citation for {association_type}")

questions = []
for category, expectation, explicit_citation, description in expectations[seed_name]:
    if expectation in {"limited", "sparse"}:
        citation = explicit_citation or f"[source_coverage:{seed_key}]"
        expected_facts = [
            {"citation": citation},
            {"text": "absence claims", "citation": f"[guardrail:{context_hash}]"},
        ]
        source_state = expectation
    else:
        citation = explicit_citation or association_citation(expectation)
        expected_facts = [{"citation": citation}]
        source_state = "limited" if seed_name == "document" else "sparse"
    questions.append(
        {
            "key": f"{seed_name}:{category}",
            "category": category,
            "source_coverage_state": source_state,
            "question": f"Does the answer cite the {description}?",
            "expected_facts": expected_facts,
            "forbidden_phrases": forbidden,
        }
    )

golden = {
    "name": f"company-ai-first-minimum-{seed_name}-visible-distractor",
    "required_categories": sorted({row["category"] for row in questions}),
    "required_source_coverage_states": sorted({row["source_coverage_state"] for row in questions}),
    "questions": questions,
}
json.dump(golden, open(golden_path, "w", encoding="utf-8"), indent=2, sort_keys=True)
PY

  .venv/bin/python tools/cubicle_graph_brief.py \
    --bounded-graph-context-json "$visible_context_json" \
    --context-json "$OUT_DIR/${seed_name}_visible_distractor_raw_context.json" \
    --brief-md "$OUT_DIR/${seed_name}_visible_distractor_raw_scaffold.md" \
    --prompt-mode generic \
    --prompt-md "$OUT_DIR/${seed_name}_visible_distractor_raw_prompt.md" \
    --mlx-python "$MLX_PYTHON" \
    --mlx-model "$MODEL" \
    --llm-max-tokens "$LLM_MAX_TOKENS" \
    --llm-timeout-seconds "$LLM_TIMEOUT_SECONDS" \
    --llm-brief-md "$raw_md" \
    --evaluation-json "$raw_eval_json" \
    --golden-json "$visible_golden_json"

  .venv/bin/python tools/cubicle_graph_brief.py \
    --bounded-graph-context-json "$visible_context_json" \
    --context-json "$OUT_DIR/${seed_name}_visible_distractor_repair_context.json" \
    --brief-md "$OUT_DIR/${seed_name}_visible_distractor_repair_scaffold.md" \
    --prompt-mode generic \
    --prompt-md "$OUT_DIR/${seed_name}_visible_distractor_repair_prompt.md" \
    --llm-brief-md "$raw_md" \
    --repaired-brief-md "$repaired_md" \
    --evaluation-json "$repaired_eval_json" \
    --golden-json "$visible_golden_json"

  .venv/bin/python - "$seed_name" "$raw_eval_json" "$repaired_eval_json" "$raw_md" "$repaired_md" "$REQUIRE_RAW_PASS" "$REQUIRE_REPAIRED_PASS" "$REQUIRE_RAW_NO_DISTRACTOR" "$REQUIRE_REPAIRED_NO_DISTRACTOR" <<'PY'
import json
import sys

seed_name, raw_path, repaired_path, raw_answer_path, repaired_answer_path, require_raw, require_repaired, require_raw_no_distractor, require_repaired_no_distractor = sys.argv[1:10]
raw = json.load(open(raw_path, encoding="utf-8"))
repaired = json.load(open(repaired_path, encoding="utf-8"))

DISTRACTOR_TERMS = [
    "ticket:comp-999",
    "pull-request:company/app#99",
    "person:mallory",
    "document:unrelated-roadmap",
    "message:finance-thread",
    "comp-999",
    "finance",
    "mallory",
    "#99",
    "visible-distractor",
]

def strict_distractor_hits(answer_path):
    text = open(answer_path, encoding="utf-8").read().lower()
    return sorted({term for term in DISTRACTOR_TERMS if term in text})

def summary(label, data, answer_path):
    golden = data.get("golden_eval", {})
    golden_distractor_hits = sorted(
        {
            hit
            for question in golden.get("questions", [])
            for hit in question.get("forbidden_phrase_hits", [])
        }
    )
    strict_hits = strict_distractor_hits(answer_path)
    print(
        seed_name,
        label,
        "passes_eval=" + str(data.get("passes_eval")),
        "passes_smoke_eval=" + str(data.get("passes_smoke_eval")),
        "golden=" + str(golden.get("pass_count")) + "/" + str(golden.get("question_count")),
        "golden_distractor_hits=" + ",".join(golden_distractor_hits or []),
        "strict_distractor_hits=" + ",".join(strict_hits or []),
    )
    return strict_hits

raw_strict_hits = summary("raw", raw, raw_answer_path)
repaired_strict_hits = summary("repaired", repaired, repaired_answer_path)

failed = False
if require_raw == "1" and not raw.get("passes_eval"):
    print(f"{seed_name}: raw visible-distractor eval failed required gate", file=sys.stderr)
    failed = True
if require_repaired == "1" and not repaired.get("passes_eval"):
    print(f"{seed_name}: repaired visible-distractor eval failed required gate", file=sys.stderr)
    failed = True
if require_raw_no_distractor == "1" and raw_strict_hits:
    print(f"{seed_name}: raw visible-distractor eval mentioned distractors", file=sys.stderr)
    failed = True
if require_repaired_no_distractor == "1" and repaired_strict_hits:
    print(f"{seed_name}: repaired visible-distractor eval mentioned distractors", file=sys.stderr)
    failed = True
if failed:
    sys.exit(1)
PY
}

for seed in $SEEDS; do
  run_seed "$seed"
done
