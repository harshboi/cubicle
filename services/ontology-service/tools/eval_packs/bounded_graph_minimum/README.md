# Bounded Graph Minimum Eval Pack

This pack exercises the generic graph-context path without Flink, Jira, GitHub
PRs, or `WorkProgram*` rows.

It is intentionally small:

- one document seed
- one message object
- one ticket object
- one claimable source-observed association
- one non-claimable candidate association
- sparse source coverage that blocks absence claims

Run:

```sh
python tools/cubicle_graph_brief.py \
  --bounded-graph-context-json tools/eval_packs/bounded_graph_minimum/context.json \
  --context-json /tmp/bounded_graph_context.normalized.json \
  --brief-md /tmp/bounded_graph_scaffold.md \
  --generic-baseline-md /tmp/bounded_graph_generic_baseline.md \
  --prompt-mode generic \
  --prompt-md /tmp/bounded_graph_prompt.md
```

Then evaluate the deterministic generic baseline with golden questions:

```sh
python tools/cubicle_graph_brief.py \
  --bounded-graph-context-json tools/eval_packs/bounded_graph_minimum/context.json \
  --context-json /tmp/bounded_graph_eval_context.json \
  --brief-md /tmp/bounded_graph_eval_scaffold.md \
  --llm-brief-md /tmp/bounded_graph_generic_baseline.md \
  --evaluation-json /tmp/bounded_graph_generic_baseline_eval.json \
  --golden-json tools/eval_packs/bounded_graph_minimum/golden_questions.json
```

The golden questions cover:

- bounded traversal shape
- claimable document-message association
- non-claimable message-ticket candidate association
- sparse coverage / no absence claims
- no analytics citation shortcuts in the generic graph context

The executable CLI demo starts from the Go `graphstore` path instead of the
static JSON fixture:

```sh
sh tools/eval_packs/bounded_graph_minimum/run_cli_demo.sh
```

To include the local MLX model run:

```sh
RUN_MLX=1 sh tools/eval_packs/bounded_graph_minimum/run_cli_demo.sh
```

The current local MLX/Qwen result is intentionally tracked as raw versus
repaired behavior:

- deterministic generic baseline: smoke pass, golden `5/5`
- raw MLX answer: smoke fail, golden `3/5`; it invents an analytics forecast
  citation and makes one confirmed relationship claim from an object citation
- repaired MLX answer: smoke pass, golden `5/5`, but only after removing those
  raw failures

This is not a production graph API. It is the smallest runnable contract that
keeps `WorkProgramGraphContext` from becoming the only mental model for Cubicle
AI context.
