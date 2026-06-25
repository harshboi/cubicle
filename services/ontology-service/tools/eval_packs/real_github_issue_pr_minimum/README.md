# real-github-issue-pr-minimum

Real non-Flink OpenGraph eval pack captured from public GitHub data.

Current fixture:

- repository: `cli/cli`
- issue: `13262`
- pull request: `13523`
- relationship: `github_issue -> github_pull_request` via `closed_by`

The fixture was generated with:

```sh
.venv/bin/python tools/github_issue_pr_open_graph_fixture.py \
  --repo cli/cli \
  --issue 13262 \
  --pr 13523 \
  --fixture-json tools/eval_packs/real_github_issue_pr_minimum/open_graph_fixture.json \
  --source-authority-json tools/eval_packs/real_github_issue_pr_minimum/source_authority.json \
  --golden-json tools/eval_packs/real_github_issue_pr_minimum/golden_questions.json
```

Run:

```sh
sh tools/eval_packs/real_github_issue_pr_minimum/run_eval.sh
```

Run the optional local-model gate:

```sh
OUT_DIR=/tmp/real_github_issue_pr_minimum_mlx \
LLM_MAX_TOKENS=32768 \
LLM_TIMEOUT_SECONDS=1800 \
tools/eval_packs/real_github_issue_pr_minimum/run_llm.sh
```

The local-model gate records raw, repaired, deterministic, seed-only, and
typed-row scores separately. Raw output is diagnostic; repaired or
deterministic output is the product-safe path.

This pack is meant to satisfy the `real-non-flink-connector` promotion-matrix
coverage tag without adding product-specific typed code.
