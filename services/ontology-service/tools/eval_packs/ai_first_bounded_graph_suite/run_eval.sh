#!/bin/sh
set -eu

SCRIPT_DIR=$(CDPATH= cd -- "$(dirname -- "$0")" && pwd)
ROOT_DIR=$(CDPATH= cd -- "$SCRIPT_DIR/../../.." && pwd)
OUT_DIR=${OUT_DIR:-/tmp/ai_first_bounded_graph_suite}
RUN_CORE_GO_TESTS=${RUN_CORE_GO_TESTS:-1}
RUN_REAL_GITHUB_LLM=${RUN_REAL_GITHUB_LLM:-0}
RUN_REAL_CONNECTOR_LLM=${RUN_REAL_CONNECTOR_LLM:-0}
REAL_CONNECTOR_DATABASE=${REAL_CONNECTOR_DATABASE:-/Users/harsh/workspace/data/flink-pr-jira-1000-plus-500-jira-2026-06-22/ontology.source-scope-claimable-20260624.db}
REAL_CONNECTOR_START_OBJECT_TYPE=${REAL_CONNECTOR_START_OBJECT_TYPE:-ticket}
REAL_CONNECTOR_START_KEY=${REAL_CONNECTOR_START_KEY:-ticket:jira:FLINK-36332}
REAL_CONNECTOR_ASSOCIATION_TYPES=${REAL_CONNECTOR_ASSOCIATION_TYPES:-implemented_by}
REAL_CONNECTOR_DEPTH=${REAL_CONNECTOR_DEPTH:-1}
REAL_CONNECTOR_LIMIT_PER_OBJECT=${REAL_CONNECTOR_LIMIT_PER_OBJECT:-6}
LLM_MAX_TOKENS=${LLM_MAX_TOKENS:-32768}
LLM_TIMEOUT_SECONDS=${LLM_TIMEOUT_SECONDS:-1800}

mkdir -p "$OUT_DIR"
cd "$ROOT_DIR"

run_step() {
  name=$1
  shift
  step_dir="$OUT_DIR/$name"
  mkdir -p "$step_dir"
  log="$step_dir/stdout.log"
  printf 'ai_first_suite %s start\n' "$name"
  if "$@" >"$log" 2>&1; then
    printf 'ai_first_suite %s pass log=%s\n' "$name" "$log"
  else
    status=$?
    printf 'ai_first_suite %s fail status=%s log=%s\n' "$name" "$status" "$log" >&2
    tail -n 80 "$log" >&2 || true
    exit "$status"
  fi
}

if [ "$RUN_CORE_GO_TESTS" = "1" ]; then
  run_step core_go_tests go test \
    ./internal/graphcontext \
    ./internal/entgraph \
    ./internal/graphql \
    ./cmd/ontology-service \
    ./internal/flinkcubiclepoc/sourcegraph
fi

run_step bounded_graph_auth_limited env \
  OUT_DIR="$OUT_DIR/bounded_graph_auth_limited" \
  sh tools/eval_packs/bounded_graph_auth_limited/run_eval.sh

run_step open_graph_incident_minimum env \
  OUT_DIR="$OUT_DIR/open_graph_incident_minimum" \
  sh tools/eval_packs/open_graph_incident_minimum/run_eval.sh

run_step open_graph_revenue_minimum env \
  OUT_DIR="$OUT_DIR/open_graph_revenue_minimum" \
  sh tools/eval_packs/open_graph_revenue_minimum/run_eval.sh

run_step real_github_issue_pr_minimum env \
  OUT_DIR="$OUT_DIR/real_github_issue_pr_minimum" \
  sh tools/eval_packs/real_github_issue_pr_minimum/run_eval.sh

run_step real_pypi_project_release_minimum env \
  OUT_DIR="$OUT_DIR/real_pypi_project_release_minimum" \
  sh tools/eval_packs/real_pypi_project_release_minimum/run_eval.sh

run_step real_connector_negative_partial env \
  OUT_DIR="$OUT_DIR/real_connector_negative_partial" \
  sh tools/eval_packs/real_connector_negative_partial/run_eval.sh

if [ "$RUN_REAL_GITHUB_LLM" = "1" ]; then
  run_step real_github_issue_pr_llm env \
    OUT_DIR="$OUT_DIR/real_github_issue_pr_llm" \
    LLM_MAX_TOKENS="$LLM_MAX_TOKENS" \
    LLM_TIMEOUT_SECONDS="$LLM_TIMEOUT_SECONDS" \
    tools/eval_packs/real_github_issue_pr_minimum/run_llm.sh
else
  printf 'ai_first_suite real_github_issue_pr_llm skip RUN_REAL_GITHUB_LLM=0\n'
fi

run_step company_ai_first_minimum env \
  OUT_DIR="$OUT_DIR/company_ai_first_minimum" \
  sh tools/eval_packs/company_ai_first_minimum/run_eval.sh

if [ "$RUN_REAL_CONNECTOR_LLM" = "1" ]; then
  if [ ! -f "$REAL_CONNECTOR_DATABASE" ]; then
    echo "ai_first_suite real_connector_llm missing database: $REAL_CONNECTOR_DATABASE" >&2
    exit 2
  fi
  run_step real_connector_llm env \
    OUT_DIR="$OUT_DIR/real_connector_llm" \
    DATABASE="$REAL_CONNECTOR_DATABASE" \
    START_OBJECT_TYPE="$REAL_CONNECTOR_START_OBJECT_TYPE" \
    START_KEY="$REAL_CONNECTOR_START_KEY" \
    ASSOCIATION_TYPES="$REAL_CONNECTOR_ASSOCIATION_TYPES" \
    DEPTH="$REAL_CONNECTOR_DEPTH" \
    LIMIT_PER_OBJECT="$REAL_CONNECTOR_LIMIT_PER_OBJECT" \
    LLM_MAX_TOKENS="$LLM_MAX_TOKENS" \
    LLM_TIMEOUT_SECONDS="$LLM_TIMEOUT_SECONDS" \
    tools/eval_packs/real_connector_bounded_probe/run_llm.sh
else
  printf 'ai_first_suite real_connector_llm skip RUN_REAL_CONNECTOR_LLM=0\n'
fi

python3 - "$OUT_DIR" <<'PY'
import json
import sys
from pathlib import Path

out = Path(sys.argv[1])

def read_json(path: Path) -> dict:
    if not path.exists():
        return {}
    return json.loads(path.read_text(encoding="utf-8"))

reports = {
    "bounded_graph_auth_limited": out / "bounded_graph_auth_limited" / "eval.json",
    "open_graph_incident_minimum": out / "open_graph_incident_minimum" / "eval.json",
    "open_graph_revenue_minimum": out / "open_graph_revenue_minimum" / "eval.json",
    "real_github_issue_pr_minimum": out / "real_github_issue_pr_minimum" / "eval.json",
    "real_pypi_project_release_minimum": out / "real_pypi_project_release_minimum" / "eval.json",
    "real_connector_negative_partial": out / "real_connector_negative_partial" / "eval.json",
    "real_github_issue_pr_raw": out / "real_github_issue_pr_llm" / "raw_eval.json",
    "real_github_issue_pr_repaired": out / "real_github_issue_pr_llm" / "repaired_eval.json",
    "real_github_issue_pr_generic": out / "real_github_issue_pr_llm" / "generic_baseline_eval.json",
    "real_connector_raw": out / "real_connector_llm" / "raw_eval.json",
    "real_connector_repaired": out / "real_connector_llm" / "repaired_eval.json",
    "real_connector_generic": out / "real_connector_llm" / "generic_baseline_eval.json",
}
company_seeds = ["document", "person", "pull_request", "ticket", "message"]
for seed in company_seeds:
    reports[f"company_ai_first_{seed}"] = out / "company_ai_first_minimum" / f"{seed}_eval.json"

summary = {}
for name, path in reports.items():
    data = read_json(path)
    if not data:
        continue
    golden = data.get("golden_eval") or {}
    summary[name] = {
        "passes_eval": data.get("passes_eval"),
        "passes_smoke_eval": data.get("passes_smoke_eval"),
        "golden_pass_count": golden.get("pass_count"),
        "golden_question_count": golden.get("question_count"),
        "repair_applied": data.get("repair_applied", False),
    }

summary_path = out / "summary.json"
summary_path.write_text(json.dumps(summary, indent=2, sort_keys=True), encoding="utf-8")
for name, row in sorted(summary.items()):
    print(
        "ai_first_suite",
        name,
        "passes_eval=" + str(row.get("passes_eval")),
        "passes_smoke_eval=" + str(row.get("passes_smoke_eval")),
        "golden=" + str(row.get("golden_pass_count")) + "/" + str(row.get("golden_question_count")),
        "repair_applied=" + str(row.get("repair_applied")),
    )
print("ai_first_suite summary=" + str(summary_path))
PY
