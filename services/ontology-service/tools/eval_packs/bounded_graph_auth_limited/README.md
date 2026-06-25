# Bounded Graph Auth-Limited Eval Pack

This pack exercises the generic `BoundedGraphContext` path under auth-limited
source coverage. It is intentionally non-Flink and has no `WorkProgram*`, TPM,
forecast, or analytics rows.

It checks that:

- a known typed ticket-document relationship remains visible as validation
  context when the document endpoint is still partial
- `403` / `429` source failures remain coverage guardrails only
- missing neighbors are unknown, not absent
- raw sync issue bodies and private locators are not prompt facts

Run:

```sh
sh tools/eval_packs/bounded_graph_auth_limited/run_eval.sh
```

The script writes artifacts to `/tmp/bounded_graph_auth_limited` by default and
evaluates the deterministic generic graph baseline against the golden questions.
