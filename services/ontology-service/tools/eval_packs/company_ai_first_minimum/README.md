# company-ai-first-minimum

Persisted Ent-backed bounded-graph eval pack for the generic AI-first context path.

This pack seeds a tiny source-neutral company graph into SQLite, exports five bounded graph contexts through the GraphQL resolver path, renders the deterministic generic baseline, and evaluates the answer against golden checks.

The source-backed seeds (`document`, `ticket`, `pull_request`, and `message`)
also run `tools/bounded_graph_contract.py --profile connector` against the
exported context. That gate requires connector-grade coverage metadata:
source system, source instance, coverage window, relationship coverage scope,
and evidence-backed claim policy. `person` is intentionally source-neutral and
uses the normal fixture contract.

The script now also writes fact-level promotion audit reports through
`tools/bounded_graph_promotion_audit.py`. These reports separate promotable
objects and associations from candidates blocked by hydration, duplicate
relationship identity, generated evidence, visibility, freshness, or missing
proof. The audit also consumes the canonical source-authority matrix at
`internal/graphcontext/source_authority.json`, so claimable relationships must
be backed by a source system and evidence locator kind that are allowed to prove
that relationship family's presence. That authority is read from the resolved
evidence row, not copied from the association row; relationship rows cannot
self-attest their source authority. Source-backed seeds get both fixture-profile
and connector-profile audit JSON files.

For each seed, the script also exports a depth-0 seed-only baseline and requires
the depth-2 bounded traversal answer to beat it on the same golden questions.
That gate is the important architecture check: graph traversal must add
observable value over a direct object-only summary.

The script also renders a typed-row baseline from the depth-2 context. That
baseline may cite typed graph objects and coverage guardrails, but deliberately
excludes relationship association citations. The depth-2 traversal must beat
that baseline too, proving the graph relationships add value beyond object rows
alone. Current scores are:

- document: depth-2 `5/5`, seed-only `2/5`, typed-row baseline `2/5`
- person: depth-2 `4/4`, seed-only `1/4`, typed-row baseline `1/4`
- pull request: depth-2 `4/4`, seed-only `1/4`, typed-row baseline `1/4`
- ticket: depth-2 `4/4`, seed-only `1/4`, typed-row baseline `1/4`
- message: depth-2 `4/4`, seed-only `1/4`, typed-row baseline `1/4`

The fixture includes a disconnected high-rank finance cluster in the same source
tenant/repository (`COMP-999`, PR `#99`, Mallory, an unrelated roadmap, and a
finance thread). Every exported context is checked directly for those keys
before the prose evaluator runs. This catches accidental global source-instance
fanout and keeps the eval from passing merely because the answer omitted leaked
rows.

The script also injects a visible unrelated high-rank finance ticket/PR into
each exported context and renders the deterministic generic baseline. The
baseline must not mention those distractor keys. This caught and fixed an
ordering bug where disconnected `implemented_by` rows could outrank closer
seed-component relationships.

`run_visible_distractor_llm.sh` is the optional local-model version of that
stress test. It runs one or all seeds with a prompt-visible disconnected
finance ticket/PR through MLX/Qwen using a larger token budget. Set
`SEEDS=all` to cover `document`, `person`, `pull_request`, `ticket`, and
`message`. The current observed all-seed result with
`mlx-community/Qwen3-Coder-30B-A3B-Instruct-bf16` and
`LLM_MAX_TOKENS=16384` is:

- document: raw golden `0/4`, smoke fail, strict distractor mentions present;
  repaired golden `4/4`, smoke pass, no strict distractor mentions
- person: raw golden `0/4`, smoke fail, strict distractor mentions present;
  repaired golden `4/4`, smoke pass, no strict distractor mentions
- pull request: raw golden `0/4`, smoke fail, strict distractor mentions
  present; repaired golden `4/4`, smoke pass, no strict distractor mentions
- ticket: raw golden `0/4`, smoke fail, strict distractor mentions present;
  repaired golden `4/4`, smoke pass, no strict distractor mentions
- message: raw golden `0/4`, smoke fail, strict distractor mentions present;
  repaired golden `4/4`, smoke pass, no strict distractor mentions

That means prompt-visible irrelevant rows still require deterministic
seed-relevance metadata plus verifier/repair filtering before product display.
The repair path now also rejects lines that mention disconnected row keys even
when they cite an allowed guardrail, and confirmed relationship claims must cite
a claim-allowed association of the same relationship kind.

Seeds:

- `document:company-plan`
- `person:alice`
- `pull-request:company/app#42`
- `ticket:COMP-101`
- `message:launch-standup`

The document seed also carries a matching `SourceSyncIssue` with a 403-style code, so coverage should be `limited` and absence claims must stay blocked.
