# AI-First Mixed Minimum Eval Pack

This pack is a harness/reference fixture for the Cubicle graph-brief evaluator.
It is not proof from a live model run.

It checks that promotion decisions require a candidate answer to beat a simpler
typed-row baseline across mixed categories instead of winning only on one
packet or workstream family.

Run:

```sh
python tools/cubicle_graph_brief.py \
  --golden-json tools/eval_packs/ai_first_mixed_minimum/golden_questions.json \
  --compare-answers-json tools/eval_packs/ai_first_mixed_minimum/answers.json \
  --comparison-json /tmp/ai_first_mixed_minimum_comparison.json \
  --require-promotion-gates
```

When running against a real `WorkProgramGraphContext`, first emit deterministic
baselines from the same context:

```sh
python tools/cubicle_graph_brief.py \
  --workstream-key workstream:example \
  --graph-context-json context.json \
  --context-json normalized_context.json \
  --brief-md scaffold.md \
  --typed-row-baseline-md typed_row_baseline.md \
  --generic-baseline-md generic_graph_baseline.md \
  --prompt-mode generic \
  --prompt-md prompt.md
```
