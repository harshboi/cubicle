# AI-First Graph Brief Contract

Date: 2026-06-24

## Purpose

This contract keeps the current Cubicle AI-first PoC from turning into a broad
TPM product platform before the evidence supports it.

The working shape is:

```text
typed product rows + typed graph relationships + source coverage + evidence
  |
  v
bounded WorkProgramGraphContext or BoundedGraphContext
  |
  v
local LLM brief
  |
  v
verifier-gated generated evidence
```

Normal product truth still starts from typed Cubicle rows such as `Ticket`,
`PullRequest`, `Document`, `Message`, their typed relationship rows
(`TicketPullRequest`, `TicketDocument`, `TicketMessage`), and `Evidence`.
Source failures and sparse coverage stay in `SourceSyncRun` /
`SourceSyncIssue` diagnostics. Generated TPM rows and LLM briefs are not
source truth.

Boundary clarification from the memory/debate review:

- `WorkProgramGraphContext` is the canonical LLM input for WorkProgram operating
  briefs, not the canonical Cubicle AI context for every graph question.
- A broader `BoundedGraphContext` contract now has a small Go adapter over
  `graphstore.Expander` in `internal/graphcontext`. It supports arbitrary start
  objects, relation filters, depth/fanout bounds from `domain.ExpandRequest`,
  sparse coverage policy, guardrails, and conservative claim policy. Do not
  force person, document, message, or ticket-only questions through fake
  `WorkProgramItem` rows just to use a WorkProgram packet.
- The first product-facing `BoundedGraphContext` shape now exists as the
  GraphQL query `boundedGraphContext(...)`. It is dependency-injected over
  `graphstore.Expander` and fails closed with a configuration error when no
  expander is configured. The HTTP server now auto-wires a narrow Ent-backed
  product expander when an Ent client is present, starting with
  `Ticket -> TicketPullRequest -> PullRequest`. This is the first real storage
  bridge, not a full mixed-domain traversal store. It now includes typed
  ticket/document/message relationships, but still excludes source fragments,
  raw message payload crawling, WorkProgram packets, analytics projections, and
  generated summaries.
- For the current PoC, the LLM summarizes, ranks, and asks validation questions
  over bounded typed rows. It does not traverse raw source, prove missing facts,
  or promote generated summaries into source truth.

## 2026-06-24 Debate Updates

The memory/debate review forced these corrections before the next build slice:

- `WorkDependencyEdge` is topology context, not a confirmed relationship claim.
  A linked `WorkAction` or `WorkBlocker` may be claimable on its own, but the
  derived edge connecting it to another node must keep `claimAllowed=false`
  unless a future schema adds explicit source-native or human-labeled dependency
  evidence.
- The HTTP topology regression now enforces this same rule:
  `relationshipClaimAllowed=false`, `claimUse=blocked_by_validation`, and
  `claimGateReason=derived_dependency_edge_not_product_claim` for derived
  blocked-by edges.
- The current six-question Flink golden set is a smoke gate, not proof of
  general architecture fit. It is overfit to Flink, TPM, forecast-risk, and the
  current packet vocabulary.
- Pre-repair and post-repair LLM scores must be reported separately. A repair
  loop is a display-safety mechanism, not proof that the model reasoned well.
- Source-sync issue content must be sanitized before LLM prompting. Counters,
  issue classes, and coverage states are allowed; raw auth/rate-limit bodies
  and private locators are not prompt or product facts.
- Durable `Work*` schema remains frozen for new concepts until a fair eval beats
  simpler typed-row and generic graph-context baselines.
- Every future graph prompt entrypoint must declare which graph it traverses:
  typed product graph, WorkProgram operating graph, generic bounded graph,
  evidence graph, analytics projection, or raw replay/debug graph. Raw replay
  and analytics projection graphs are never product-truth prompt sources.
- Document and Message traversal is allowed only through typed product rows and
  typed relationship rows with evidence. This is not permission to reintroduce
  the rejected document-fragment/source-spine serving model.
- Object identity inside graph traversal is `(objectType, key)`, not key alone.
  Generic fixtures may use globally prefixed keys, but stores and expanders must
  not silently collapse two object types that happen to share a natural key.
- Mixed relationship fanout must be capped by relationship relevance
  (`rankScore`, last activity, update time, then deterministic key), not by
  lexicographic association type order.
- Confirmed relationship claims require relationship-shaped evidence when that
  metadata is available: current proof state, relationship claim kind, matching
  relationship kind, public visibility, and sufficient confidence.

Current eval status:

- `packet_context_mlx_raw` and `packet_context_mlx_repaired` both pass the
  six-question Flink golden gate.
- The raw MLX answer still fails smoke eval on answer shape because it emits too
  many confirmed-fact bullets; repair is currently enforcing display contract,
  not rescuing missing facts or forbidden claims.
- The generated generic claimable-graph baseline passes smoke eval and `1/6`
  golden questions. It proves a simpler graph traversal can produce a safe
  answer shape, but the current golden set still rewards packet/analytics facts
  it does not attempt to summarize.
- The deterministic scaffold and thin typed-row baseline still fail all six
  questions, so broad architecture claims still need non-Flink and non-TPM
  golden questions.

Generic safety-suite status:

- `internal/graphcontext` now builds a source-neutral `BoundedGraphContext` from
  the generic domain graph. Its first regression expands a document seed to a
  message and ticket with no Flink, Jira, GitHub PR, TPM, or `WorkProgram*`
  dependency.
- `ontology-service bounded-graph-context-export` now provides the first
  executable generic graph traversal export. Its default fixture is
  `generic-doc-message-ticket`, not Flink. It emits a harness-ready
  `{"boundedGraphContext": ...}` envelope from `graphstore.MemoryStore`.
- The Go adapter marks typed/source-observed objects claimable, but only marks
  associations as confirmed relationship claims when they have explicit
  evidence, explicit public visibility, and full-confidence support.
  Lower-confidence links are exported as validation leads with
  `claimAllowed=false`.
- Multi-evidence relationship handling now distinguishes consolidated evidence
  from duplicate relationship rows. A single row with `evidence_count > 1` can
  support confirmed relationship prose when the latest evidence passes
  source-authority policy. Duplicate logical `(from, associationType, to)` rows
  still fail closed with `relationship_multi_evidence_requires_review` and stay
  visible only for audit until a merge/review policy selects canonical truth.
- The Ent product expander now has a regression proving populated
  `SourceScopeState`, `SourceSyncRun`, `SourceSyncIssue`, and
  `UnresolvedReference` rows do not become bounded product-graph adjacency.
  A ticket traversal with matching source diagnostics still returns only typed
  product objects and typed relationship rows, and starting from a
  `source_sync_issue` object type fails closed as missing/unsupported.
- The Flink replay loader now has a repeated-load regression proving duplicate
  replay records do not create duplicate durable product rows, relationships,
  or evidence rows. Persisted `SourceSyncRun` created counters are derived from
  before/after DB row deltas, so a replay refresh does not claim new product
  objects or evidence were created.
- `ExpandRequest` now carries a server-side `ReadFilter`. Stores apply it
  before traversal and fanout counting. With no principal-specific filter,
  `BoundedGraphContext` installs a public/prompt-safe filter by default; private
  starts fail before prompt construction. Memory and Ent product expanders both
  have regressions proving unreadable private edges do not consume fanout or
  bridge to public descendants. This is a traversal-level auth hook, not a full
  production ACL provider. The GraphQL resolver now passes this hook through
  `BoundedGraphReadFilterProvider`, with context helpers for a coarse
  principal/visibility read filter. HTTP can attach that principal access from
  a configured request provider before GraphQL execution; this proves framework
  propagation, not tenant/source ACL correctness.
- The adapter adds a sparse-coverage guardrail automatically when
  `absenceClaimsAllowed=false`; missing neighbors stay unknown, not absent.
- Absence claims are additionally clamped by relation/path/source/time coverage:
  `coverageState=complete` and `absenceClaimsAllowed=true` are insufficient
  unless the coverage policy names every requested association type, or uses
  `*`/`all` for unfiltered traversal, and also names the source system, source
  instance, and coverage freshness window.
- `BoundedGraphCoverage` now exposes that same scope in GraphQL:
  `absenceClaimAssociationTypes`, `sourceSystem`, `sourceInstance`,
  `coverageWindowStart`, and `coverageWindowEnd`. Clients and prompt builders
  can inspect the relation/source/time basis for any absence claim instead of
  relying on an opaque `absenceClaimsAllowed` boolean.
- Ent-backed `boundedGraphContext` now has a first source-scope coverage
  provider. It may surface `coverageState=complete` when the start product row
  points at a fresh `SourceScopeState` with `coverageMode=exact_scope`, a
  matching source connection, a complete latest `SourceSyncRun`, and explicit
  `coverage_start_at` / `coverage_end_at`. Absence claims still require a
  relationship allowlist in the scope policy and request context with
  `CoverageCompleteForPrincipal=true`; otherwise coverage may be complete while
  absence claims remain gated.
- Declared relationship coverage is also filtered through source/object
  compatibility before it can support an absence claim. For example, GitHub PR
  source scope can support PR-native participation gaps such as `author` or
  `reviewer`, but it cannot prove absence of Jira remote-link relationships
  such as `implemented_by`.
- Current source-authority matrix:

  | Start object | Source system | Absence-claim relationship families this source can prove | Must stay unknown from this source alone |
  | --- | --- | --- | --- |
  | `Ticket` | `jira` | `implemented_by`, `documented_by`, `discussed_in` | PR files, PR reviews, GitHub-only participation detail |
  | `PullRequest` | `github` | `author`, `creator`, `reviewer`, `approver`, `commenter`, `requested_reviewer` | Jira remote-link absence such as `implemented_by`; docs/chat links unless separately covered |
  | `Document` | `docs` | `links_to` | Jira ticket links, GitHub PR participation, chat discussion absence |
  | `Message` | `chat` | `discussed_in` | Jira remote links, GitHub PR participation, document backlink absence |

  This table is deliberately small. Adding GitLab, Linear, Slack, docs, mail,
  CI, or a new relationship family requires extending the source-authority
  matrix and adding one positive plus one negative absence-claim regression.
- Source authority now has one executable presence-authority matrix at
  `internal/graphcontext/source_authority.json`. The fact-level promotion audit
  and Ent-backed `boundedGraphContext` runtime both consume that matrix; a
  claimable relationship fails promotion if its evidence source or
  source-specific locator kind is not allowed to prove that relationship family,
  or if the relationship family is missing from the matrix. The audit reads
  authority from the resolved evidence row, not from the association row, so a
  relationship cannot self-attest its source authority when evidence provenance
  is incomplete. Ent-backed `boundedGraphContext` now applies the same
  source-authority gate before setting `claimAllowed=true` on relationships.
- The source-scope provider has regressions for ticket, pull request, document,
  and message starts. The provider must not leak `SourceScope*`,
  `SourceSyncRun`, or `SourceSyncIssue` rows into bounded graph context, and an
  unfiltered traversal must keep absence claims gated even when source and
  principal coverage are complete for one relationship path.
- The Python bounded-context harness mirrors this clamp for saved JSON and eval
  fixtures, so a raw `absenceClaimsAllowed=true` flag cannot bypass
  relation/path/source/time coverage checks there either.
- The Python bounded-context harness also mirrors the multi-evidence
  relationship clamp for saved JSON. A fixture cannot mark one fresh row
  claimable while hiding a stale/private sibling for the same logical
  relationship from the claim policy.
- The anti-pigeonhole suite now includes a replayable real PyPI package
  metadata OpenGraph pack (`real_pypi_project_release_minimum`). It proves the
  same bounded traversal, source-authority, sparse-coverage guardrail, and
  seed-only/typed-row promotion gates on a non-GitHub, non-Jira, non-Flink
  source domain without adding product-specific schema.
- The suite also includes a real connector negative/partial pack
  (`real_connector_negative_partial`). It proves partial PR endpoints are
  visible but non-claimable, real PR-file `401`/missing-snapshot source issues
  force limited coverage without raw diagnostic leakage, positive claimable
  edges survive limited coverage, non-authoritative author evidence remains
  gated, and real `partial_scope` source states keep absence claims disabled.
- Low-trust objects are now validation context, not product truth. Partial,
  stale, restricted, and generated graph objects are projected with
  `claimAllowed=false`; a remote-link-only PR stub can still appear in the
  bounded graph for audit, but it cannot support a confirmed object fact until
  hydration upgrades its freshness/writer state.
- Relationship endpoint verification now fails closed. If a saved or live
  bounded graph association is missing either endpoint object, or either
  endpoint is still `freshnessState=partial`, the relationship remains visible
  but is projected as `claimAllowed=false` with a hydration gate.
- Ent-backed relationships now carry known `evidence_count` into domain
  metadata. Counts above one require either source-authority policy for the
  latest evidence or an explicit merge/review policy. The Flink replay loader
  persists actual `TicketPullRequest` relationship evidence counts instead of
  resetting them to one.
- A second generic graph-context suite checks answer shape, bounded traversal
  counts, claimable row handling, derived-edge caveats, and guardrails.
- `generic_graph_baseline` passes `5/5` generic questions.
- `generic_graph_mlx_raw` currently fails smoke eval and scores `3/5` generic
  questions because it invented an analytics citation and promoted a candidate
  relationship using only an object citation.
- `generic_graph_mlx_repaired` passes smoke eval and `5/5` generic questions
  after the verifier removes the unsafe raw bullets. This proves the display
  repair loop can enforce the contract; it does not prove raw model reliability.
- `packet_context_mlx_raw` and `packet_context_mlx_repaired` pass only `2/5`
  generic questions because they focus on TPM analytics facts.
- This split is the current architecture warning: packet-rich context is useful,
  but prompt/answer mode must be explicit before we claim Cubicle is broadly
  AI-first rather than a Flink/TPM assistant.
- A first sparse, non-Flink ticket-only regression now builds a
  `WorkProgramGraphContext` from GraphQL-style JSON and verifies that
  `generic_graph_baseline` can pass smoke plus golden gates without Flink or TPM
  claims. This is only a bias check, not proof of generality; the next fair eval
  still needs real GitHub-only, ticket-only, and sparse mixed-source workstreams.
- A persisted Ent-backed `company-ai-first-minimum` eval pack now seeds a
  source-neutral company graph into SQLite, exports `boundedGraphContext` through
  the GraphQL resolver, and verifies deterministic generic baselines for
  `Document`, `Person`, `PullRequest`, `Ticket`, and `Message` seeds. It passes
  `5/5`, `4/4`, `4/4`, `4/4`, and `4/4` respectively, including one
  auth/rate-limited document coverage case that keeps 403-style source failures
  as coverage evidence rather than absence evidence.
- The same pack now exports depth-0 seed-only baselines and requires the depth-2
  bounded traversal answer to beat them on the same golden questions. Current
  seed-only scores are `2/5`, `1/4`, `1/4`, `1/4`, and `1/4`, while depth-2
  scores remain `5/5`, `4/4`, `4/4`, `4/4`, and `4/4`. This is the first
  executable proof that traversal adds value over a direct object-only summary
  in the generic PoC path.
- The company pack also renders a typed-row baseline from the same depth-2
  context that may cite typed graph objects and coverage guardrails, but not
  relationship association rows. Current typed-row baseline scores are `2/5`,
  `1/4`, `1/4`, `1/4`, and `1/4`, so depth-2 traversal now beats both seed-only
  and typed-row/object-only baselines on this fixture.
- The company fixture now includes a disconnected high-rank finance cluster in
  the same source tenant/repository (`ticket:COMP-999`,
  `pull-request:company/app#99`, `person:mallory`,
  `document:unrelated-roadmap`, and `message:finance-thread`). The eval script
  inspects every exported context and fails before prose evaluation if any of
  those keys leak into a launch-context traversal. This is the first
  anti-overfit check against accidental source-instance/global-rank fanout.
- The same eval now injects a visible unrelated high-rank finance ticket/PR into
  each context and fails if the deterministic generic baseline selects or
  mentions it. This caught a real ordering bug: non-seed `implemented_by`
  associations were sorted by relationship priority before seed-component
  distance. Association ordering now demotes disconnected components before
  applying relationship priority.
- The optional local-model visible-distractor stress test now runs all five
  company seeds through MLX/Qwen with `LLM_MAX_TOKENS=16384`. Raw output failed
  smoke/golden on every seed and mentioned disconnected finance distractors.
  Repaired output passed smoke, golden, and strict no-distractor checks on all
  seeds: document `4/4`, person `4/4`, pull request `4/4`, ticket `4/4`, and
  message `4/4`. This is evidence for an AI-first graph PoC with deterministic
  seed-relevance metadata plus verifier/repair filtering, not evidence that raw
  model output is safe for product display.

Implemented product-surface correction:

- `workProgramGraphBrief` now accepts `promptMode` and returns `briefMode`.
  `operating` remains the default for legacy/current TPM artifacts; `generic`
  must be requested explicitly.
- Persisted AI graph briefs include prompt mode in `model_method`,
  `external_id`, `source_url`, evidence identity, and snapshot identity. A new
  generic run must not supersede the current operating run, and vice versa.
- Prompt rows no longer include raw evidence `locator`, `excerpt`, or
  `source_url` fields. Source URLs/excerpts remain gated by citation policy
  rather than handed to the LLM by default.
- Structured citation policy wins over analytics shortcuts. If
  `[analytics:tpm_forecast_summary]` is marked `claimAllowed=false`, it cannot
  support a `Confirmed Facts` bullet.
- Generated graph briefs are quarantined from graph-context inputs. GraphQL
  context reads only analytics `tpm_insight` rows for contextual insights, and
  the replay/debug SQL builder excludes `ai_graph_brief` / `cubicle_ai`
  generated summary rows before constructing prompt rows.
- The quarantine is now metadata-based as well as label-based. GraphQL
  `WorkProgramGraphContext` and the Python replay/debug builder reject rows
  with graph-brief `model_method` prefixes or `cubicle://graph-brief/` source
  URLs, so a misclassified generated summary cannot re-enter prompt rows merely
  by using `source_system=cubicle_analytics` and `external_kind=tpm_insight`.
- Ent-backed generic graph traversal treats message titles conservatively:
  `Message.Summary` is not used as a `BoundedGraphContext` object title. Current
  source adapters may derive summaries from raw comment bodies; those spans
  belong in Evidence, not prompt-visible object identity.
- Generated evidence can still be cited as provenance, but
  `WorkProgramGraphContext` citation policy does not allow generated evidence
  excerpts or generated source URLs to be quoted from graph-context citations.
- `--persist-ai-insight` now requires the raw LLM answer to pass configured
  smoke/golden gates. A repaired answer may be written for display diagnostics,
  but repair alone cannot create the persisted current AI artifact.
- `--llm-command`, `--mlx-model`, and `--persist-ai-insight` now require
  `--graph-context-json`. The direct SQL builder may still emit replay/debug
  scaffolds and deterministic baselines, but it cannot drive live model
  generation or persist generated AI artifacts.
- Local MLX generation defaults to `8192` max tokens and remains explicitly
  bounded. Larger local runs should still use `--llm-max-tokens` rather than
  changing the prompt or eval contract.
- Saved GraphQL `--graph-context-json` inputs can now drive local MLX
  generation directly; they no longer require a synthetic `--workstream-key`.
  A Qwen3-Coder MLX run with `--llm-max-tokens 12288` completed against the
  saved WorkProgram graph context. Raw output failed smoke gates due to
  unsupported confirmed-fact citations. Repaired output passed smoke gates only
  after the repair loop removed a contradictory source-coverage bullet, but it
  still failed the six-question Flink golden suite. Treat this as proof that the
  local model path runs with a larger budget, not proof that larger budget
  produces product-usable summaries.
- Generic bounded graph repair now canonicalizes source-coverage and guardrail
  bullets, rejects any material line that mentions disconnected seed-component
  rows even when it cites an allowed guardrail, and requires confirmed
  relationship claims to cite a claim-allowed association whose relationship
  kind matches the claim.
- `WorkProgramGraphContext` now surfaces a latest-graph-row caveat in its
  `llmTask` and badges. `runKey` / `generatedAt` can anchor packet rows, but the
  graph rows are still latest scoped rows until a true graph-context snapshot or
  graph-row run membership exists.
- A run-key regression now seeds an old run and a newer graph row, queries the
  old `runKey`, and verifies that the newer graph row is returned only with the
  `packet_boundary_latest_graph_rows` scope mode, LLM caveat, and
  `graph_context:latest_graph_rows` badge. This documents the current boundary;
  it does not prove immutable replay.

Still-open skeptic findings:

- `runKey` currently anchors packet selection, but parts of
  `WorkProgramGraphContext` still read latest graph rows. The API now says this
  explicitly and has a regression for the old-run/new-row case; a future
  hardening slice should add graph-row run membership or a persisted
  graph-context snapshot and then replace the caveat regression with a
  run-member-only immutability test.
- Generic `BoundedGraphContext` has an in-memory Go adapter, Python harness
  path, CLI demo exporter, GraphQL query wired through `graphstore.Expander`,
  and a first Ent-backed product expander for `Ticket`, `PullRequest`,
  `Document`, `Message`, `TicketPullRequest`, `TicketDocument`, and
  `TicketMessage`, plus participation/document-link traversal and a persisted
  non-Flink eval pack with depth-2-over-depth-0 promotion gates and same-tenant
  distractor leakage checks. The GraphQL boundary now has an explicit
  high-degree hub regression proving an `associationTypes` allowlist can block
  a shared-person bridge into unrelated work, and the graph-context projection
  has traversal-level `ReadFilter` regressions proving explicit private rows
  are filtered before graph context construction and do not consume fanout. The
  GraphQL resolver also has a two-principal regression proving a private hub
  cannot bridge a public seed to a public descendant unless the request context
  supplies matching visibility access. HTTP now has a configured provider path
  that attaches that access before `/graphql` execution, with an end-to-end
  regression for public-only versus private-allowed requests. It also has an
  internal relation/path/source/time absence-claim clamp: seed-level or sparse
  completeness cannot enable absence claims unless the requested association
  types, source system, source instance, and coverage freshness window are
  present. It still has no persisted graph-context snapshot, production
  source/workspace ACL translator, production coverage provider that can prove
  those fields from source scope state, broad mixed-domain production loader, or
  promotion gate that beats typed-row baselines across realistic company
  datasets.
- `WorkProgramGraphContext` still has TPM/Flink-shaped analytics and source
  scope assumptions. The first non-Flink golden sets must include a GitHub-only
  repo, a ticket-only workstream, and a sparse source-coverage workstream.
- Generated summaries still need a broader product-level quarantine audit across
  future loaders/read paths: `ai_graph_brief` rows may be retrieved by
  `workProgramGraphBrief`, but must not become source/product facts in any later
  materializer.

## Repeatable Cross-Exam Gate

Before promoting any new graph-LLM slice, answer these with executable evidence:

- The entrypoint declares the graph it traverses: typed product graph,
  WorkProgram operating graph, evidence graph, analytics projection, or
  replay/debug graph.
- The context builder reads typed product objects and typed relationship rows
  for product claims. `SourceSync*`, `UnresolvedReference`, analytics rows,
  search projections, raw replay rows, and generated briefs are not adjacency.
- Principal-aware authorization happens before expansion and fanout counting,
  not after prompt assembly.
- Relationship claims require relationship-shaped evidence, not only object
  citations.
- Sparse coverage is relation/path/source/time scoped before any absence claim;
  otherwise the answer must say unknown.
- Identity confidence is visible before assignee, owner, reviewer, approver,
  or person-responsibility claims.
- Depth-2 traversal beats depth-0 seed-only and typed-row baselines on the same
  questions. Raw model, repaired model, deterministic scaffold, and graph
  baseline scores are reported separately.
- Same-tenant high-rank distractors exist for each promoted seed type. The eval
  checks context JSON for leakage before prose scoring and checks visible
  distractor answers for accidental selection.
- `runKey` is either an immutable graph snapshot or the response carries the
  latest-graph-row caveat and badge.
- New durable `Work*` schema is justified by a human lifecycle, repeated product
  read need, or measured eval improvement over generated/query DTOs.

## Current Verdict

Keep two LLM input contracts explicit:

```text
WorkProgram operating brief -> GraphQL WorkProgramGraphContext JSON
generic AI-first graph PoC -> GraphQL boundedGraphContext JSON
```

`WorkProgramGraphContext` remains canonical for WorkProgram/TPM operating
briefs. It is not the universal Cubicle AI context. The generic PoC spine is
`boundedGraphContext`: typed product rows, typed relationships, evidence,
coverage policy, guardrails, and a verifier-gated LLM summary.

Treat the older direct SQL graph-brief builder as a replay/debug helper. A
production LLM path must not bypass GraphQL citation policy, source coverage
packets, forecast gates, guardrail packets, or the generic bounded graph claim
policy.

Do not generalize this verdict to all Cubicle AI use cases. The falsifying
checks for a broader AI-first architecture are non-Flink, non-TPM, person-
centered, document/message-only, sparse-coverage, and auth-limited contexts
that work without adding more `tpm_*` schema or source-specific prompt rules.

Freeze new durable TPM schema until a golden-question eval proves a concept has
one of these properties:

- A human-edited lifecycle, such as an action that can be accepted, closed,
  suppressed, or corrected.
- A repeated product read need that cannot be handled by typed product rows plus
  bounded graph context.
- A measurable improvement in citation correctness, no-answer behavior,
  forbidden-claim avoidance, or actionability.

## Product-Safe Claim Matrix

| Claim type | Product-safe now? | Required support | Forbidden promotion |
|---|---:|---|---|
| Object exists | Yes | Typed product row or graph-context citation with `claimAllowed=true` | Discovered-only references without source proof |
| Typed relationship exists | Yes, narrow | Typed relationship row with evidence or `claimAllowed=true` relationship citation | Generic derived dependency edge as product truth |
| Source coverage state | Yes | Source coverage packet / `SourceSyncIssue` counters | Treating missing rows as complete coverage |
| Absence claim | Only when gated true | `absenceClaimsAllowed=true` and source scope proof | "No blockers", "no reviews", "no linked ticket" from missing rows |
| Owner/status follow-up | Yes, as action lead | Typed row plus action citation; human review gate respected | Autonomous product action without human label |
| Blocker | No, unless directly sourced | Direct dependency / launch-impact evidence or human label | Blocker candidates, topology clusters, stale PR age |
| ETA/date commitment | No | Forecast gate must say ETA-ready and beat baselines | Risk triage, age, stale status, model score |
| Risk ranking | Yes, as attention ordering | Forecast or analytics citation plus readiness caveat | Date commitment or measured precision claim |
| Source excerpt / URL | Only when authorized | Citation has `excerptAllowed` / `sourceUrlAllowed` | Raw source text, URLs, error bodies, private locators |
| Generated summary | Yes, as generated evidence | Verifier-passing AI graph brief evidence | Feeding summary back as product/source fact |

## Forbidden Read Paths

These paths are not allowed for product-facing graph briefs:

```text
raw manifest -> LLM
SourceSyncIssue message/error body -> product absence claim
SourceSyncRun counters -> product relationship
Search projection -> product truth
WorkDependencyEdge -> canonical blocker or dependency truth
AI graph brief -> future graph context source fact
direct SQL table crawl -> production LLM prompt
```

Allowed diagnostic use:

```text
SourceSyncIssue / SourceSyncRun
  -> source coverage packet
  -> guardrail / caveat / repair lead
```

Allowed generated use:

```text
LLM brief
  -> WorkInsight / Evidence with proof_state=generated
  -> verifier-gated display
  -> human feedback or label queue
```

## Durable TPM Schema Admission

Default stance:

```text
generated/query DTO first
durable table later
```

Likely durable:

- `WorkAction`, only if it becomes a real human action ledger.
- `WorkProgramItem`, only if stable sorting/filtering and user closeout need a
  materialized register.

Constrained / suspect:

- `WorkDependencyEdge` must remain derived context unless backed by typed source
  relationships or human-labeled dependency evidence.

Prefer generated/query artifacts for now:

- forecasts
- risk drivers
- quality gates
- readiness packets
- adversarial checks
- evidence needs
- summary snapshots
- TPM function readiness

TPM-specific answer:

```text
model source truth and durable human workflow in Ent
model TPM/readiness/forecast packets as query-time or generated projections
promote only the pieces that gain a stable human lifecycle or beat the generic
bounded graph baseline in evals
```

The current architecture is acceptable only if `WorkProgram*` remains a
specialized operating layer over shared product rows. It becomes the wrong
architecture if generic questions like "what is this document connected to?",
"who owns this PR?", or "what evidence is missing?" must be forced through TPM
rows instead of typed product relationships and `boundedGraphContext`.

## Minimum PoC Eval

Before hardening more TPM surface, run 10-20 golden workstream questions:

- What changed?
- What should a TPM do next?
- What is blocked?
- Who owns it?
- What evidence is missing?
- What must not be claimed?

Each question needs:

- expected facts
- expected citations
- forbidden claims
- allowed no-answer behavior through `expected_no_answer` when source coverage
  cannot support a product fact
- source coverage state
- a `category` or `categories` tag such as `github-only`, `ticket-only`,
  `sparse-coverage`, `auth-limited`, `mixed-source`, or `forecast-gated`

Compare at least four paths:

```text
direct typed-row summary
LLM over typed rows + evidence
LLM over bounded WorkProgramGraphContext
current packet-rich context
```

Score separately:

- citation correctness
- faithfulness
- forbidden-claim violations
- no-answer behavior
- actionability
- source coverage honesty
- per-category pass/fail so one strategy cannot win only by overfitting to one
  workstream or packet family

Only promote schema or product-facing claims when this eval shows measurable
lift over the simpler typed-row baseline and does not regress any required
category.

## Memory And Debate Review Loop

Before promoting a table, packet field, or product-facing claim, run a short
four-role review:

- Memory-questioner: read `/Users/harsh/.codex/memories` and ask what prior
  decisions this slice might violate.
- Architecture skeptic: inspect the live repo and identify overfit, product-code
  creep, source-truth leaks, and unfair evals.
- Current-architecture reviewer: inspect the live implementation and separate
  what the current code proves from what it only makes possible.
- AI-first graph advocate: argue for the smallest graph traversal plus LLM path
  that preserves product safety.

Every ingestion or graph-brief slice must leave three artifacts:

- Source-authority matrix: for each source scope, which object and relationship
  families can be proven present, proven absent, or only marked unknown.
- Fixture expectation table: raw hydration/status counts, materialized DB row
  counts from a fresh DB, and positive/negative relationship expectations.
- Product-read invariant checklist: typed rows serve reads, source diagnostics
  are coverage/proof only, non-200s do not become product absence, evidence is
  locator-grade, `Person` remains bounded, `WorkProgram*` remains workstream
  centered, and AI/analytics rows remain derived until separately validated.

The moderator must answer these gate questions before implementation proceeds:

- Which graph is being traversed: typed product graph, WorkProgram operating
  graph, evidence graph, analytics projection, or raw source replay graph?
- Is any old source-spine idea being reintroduced under a new name?
- Are raw source/replay rows completely barred from production LLM prompts?
- Does generated evidence prove the source fact, the traversal result, or only
  the model interpretation?
- Which `Work*` objects have a real human lifecycle, and which are packet DTOs
  that should stay generated?
- Does the packet-rich path beat a fair typed-row or generic graph traversal
  baseline without relying on repair-only wins?
- Are source coverage failures represented only as coverage/caveat evidence?
- Is `runKey` a true immutable graph snapshot, or only a packet boundary with
  latest graph rows?
- Does HTTP wire a production principal/group/source ACL provider into the
  bounded graph principal access path, or only prove configured request-context
  propagation?
- Do association rows support stale, conflicting, private, duplicate, or
  multi-evidence cases without collapsing product truth? The first source-
  authority-aware prompt safety clamp exists; a final product merge and
  relationship-upsert policy is still required for duplicate/conflicting rows.
- Is the generic proof based on captured non-Flink data, or only on synthetic
  fixtures and TPM-adjacent harnesses?

Latest memory-questioner challenge, 2026-06-24:

- Does the expander start only from typed product or graph rows, or did a
  source-diagnostic table become adjacency by accident?
- Which adapter is allowed to write typed product rows, relationship rows,
  evidence rows, sync diagnostics, and lens results?
- Are complete PR bundle rules a GitHub policy, or a core architecture rule
  that has equivalents for Jira-only, GitLab, docs, mail, and CI?
- What distinguishes a partial low-trust stub from a full product object in the
  graph and in LLM prose?
- Is `SourceScopeState` a diagnostic snapshot, a freshness gate, or part of read
  eligibility?
- What is the durable relationship identity/upsert key when remote links,
  source IDs, and partial stubs disagree?
- Can repeated loads into fresh databases prove idempotence without overstating
  evidence attempts?
- What is the v1 node and edge allowlist, and can the same path summarize a
  document/message/ticket graph without Flink, Jira, GitHub PR, or TPM terms?
- Are typed relationships preserved as relationships with identity, evidence,
  confidence, visibility, freshness, and claim policy, or flattened into loose
  node text?
- Does the GraphQL response expose sparse coverage and absence-claim gates so
  an empty or partial traversal cannot mean "nothing exists"?
- Does every confirmed relationship claim require a claim-allowed association
  citation rather than only an object citation?
- Does a missing production expander return a configuration error, not an empty
  graph?
- Is `WorkProgramGraphContext` still canonical for WorkProgram operating
  briefs, with generic `BoundedGraphContext` kept as a separate AI-first PoC
  path?
- Is the GraphQL hardening explicitly scoped as API boundary and test harness,
  not product proof?
- How does identity confidence travel through graph context before any
  person-level claim or owner attribution?
- Are AI outputs still treated as verifier-gated triage/status synthesis, not
  reliable ETA or autonomous TPM replacement?
- If the plan adds or broadens `Document` and `Message` traversal, what exact
  Ent schema proves it is typed product traversal rather than old
  `document_fragments` / `ticket_document_fragments` source-spine revival?
- Does the bounded traversal distinguish "zero documents/messages exist" from
  "documents/messages were not hydrated due to source coverage limits"? If not,
  the answer must stay unknown.
- For message bodies, does the prompt receive only typed metadata/summaries, or
  has raw source content leaked into production graph context?

Latest architecture-skeptic hardening, 2026-06-24:

- Fixed: `entgraph.ProductExpander` now dedupes traversal objects by
  `(objectType, key)` and has a regression proving a `Ticket` and `Document`
  can share the same key without collapsing.
- Fixed: `graphstore.MemoryStore` now uses `(objectType, key)` for object and
  adjacency identity, preserving the same contract in generic fixtures and CLI
  demos.
- Fixed: mixed Ent relationship fanout now carries rank/recency/update metadata
  through the merge and applies `limitPerObject` to the highest-ranked
  relationships across `implemented_by`, `documented_by`, and `discussed_in`.
- Fixed: relationship claim policy now requires complete relationship proof
  metadata before a relationship is claimable: evidence key, claim kind
  `relationship`, matching relationship kind, current proof state, visibility,
  and confidence. Missing metadata now gates the association as incomplete
  instead of treating it as confirmed.
- Fixed: `entgraph.ProductExpander` no longer uses `Message.Summary` or raw
  `Message.Body` for graph object titles. Messages use their stable message key
  as the prompt-visible label, so typed message traversal does not reintroduce
  raw source-payload crawling or summary laundering.
- Fixed: the minimum bounded graph eval fixture now makes its claimable
  association production-shaped: evidence key, full confidence, public
  visibility, fresh proof, and an evidence stub. The Python regression asserts
  claimable associations in that pack carry relationship evidence.
- Fixed: confirmed facts can no longer use sparse source-coverage citations to
  claim "no reviews", "no blockers", "no owners", or similar absence facts
  unless source coverage explicitly allows absence claims. Confirmed facts also
  cannot use only context/source-coverage/analytics boundary citations to claim
  reviewer approval or linked-PR product facts.
- Fixed, seed-scope only: Ent-backed `boundedGraphContext` now derives
  prompt-safe seed-object coverage from matching `SourceSyncIssue` rows. Matching
  `403`/`429` or forbidden/rate-limit issues set `coverageState=limited`,
  `absenceClaimsAllowed=false`, and
  `absenceClaimGateReason=source_auth_or_rate_limit` without exposing raw sync
  issue bodies or source URLs.
- Fixed: `entgraph.ProductExpander` now traverses typed person participation and
  document-link rows in the generic graph path:

  ```text
  Ticket -> assignee/reporter/owner -> Person
  PullRequest -> author/creator/reviewer/approver/commenter/requested_reviewer -> Person
  Document -> links_to -> Document
  ```

  The emitted association labels preserve the row kind so association citations
  continue to match `Evidence.relationship_kind`.
- Fixed: mixed row-kind association filters are exact after fanout merge. A
  request for `approver` cannot leak an `author` edge just because both live in
  the PR/person participation family.
- Fixed: `TicketDocument` now uses `documented_by` as the canonical association
  value in ontology, Ent-backed graph responses, HTTP tests, and auth-limited
  eval fixtures. Product UI copy may render this as "supporting documentation",
  but proof semantics stay aligned with the durable row/evidence kind.
- Still open: coverage is not yet provider-proven across every traversed
  neighbor. A missing document/message/PR edge is still unknown unless a future
  source-scope coverage provider proves completeness for that relationship
  family, source scope, and freshness window.
- Still open: the generic product expander is still intentionally narrow. It
  does not yet traverse person identity aliases, mentions, document/message
  authorship, or product-language relationship aliases. "Who owns this?" and
  "who reviewed this?" can now be answered only from claimable typed assignment
  or review rows; unresolved identities and source-coverage gaps must stay
  unknown.
- Still open: product-facing source coverage packets may expose
  `SourceSyncIssue` messages or source URLs outside graph-brief prompts. The
  graph-brief prompt path is sanitized; the broader GraphQL product surface
  still needs an ops-vs-product split or explicit redaction gate.
- Still open: citation support is still heuristic. Context, source-coverage, and
  analytics citations may describe boundaries or metrics, but future negative
  evals must broaden claim-type coverage for owner, review, linked-PR, blocker,
  and absence claims across non-Flink contexts.
- Still open: relationship naming still needs product-language hardening. Public
  copy should verbalize `Ticket -> documented_by -> Document` as supporting
  documentation, not reverse authorship or proof that a document itself owns the
  ticket.

Standing memory-questioner checklist:

- What is the hard serving boundary? Source capture/replay may support
  ingestion and provenance, but product APIs and graph briefs must not crawl raw
  manifests or `SourceSync*` rows except for coverage/proof badges.
- Who can write or update typed facts and relationships? Specify source
  authority, unique keys, idempotent upserts, stale-state handling, and read
  gates before adding more loaders.
- What is the evidence contract? A product fact, relationship, generated
  insight, or brief claim needs an auditable source row, locator or record
  identity, confidence/trust state, and source coverage state.
- Is the AI brief deterministic retrieval plus LLM narration, or LLM graph
  traversal? The allowed PoC path is bounded typed context first; the LLM
  summarizes and ranks under citation policy.
- Which TPM concepts belong in Ent versus derived analytics? Durable
  user-authored state and workflow registers may be Ent; forecasts, clustering,
  evidence needs, adversarial checks, and experiments stay derived until a
  product contract and eval justify promotion.
- What gates stop forecasting from becoming fake ETA? No ETA label ships unless
  held-out evaluation beats baselines; otherwise forecasts are risk signals with
  calibration and caveats.
- How does sparse/non-Flink coverage generalize? Missing auth, missing
  hydration, or sparse source scope means unknown/unhydrated, never absent.
- How is owner identity modeled? `owner_key`, source, confidence, and unresolved
  state must remain distinct from canonical `person_id` until identity
  resolution is safe.
- What exactly validates the PoC? Fresh fixture replay, expected edges,
  unresolved refs, evidence rows, graph-brief citations, coverage counters, and
  golden questions must all match before calling the architecture validated.
- What is the traversal contract? The PoC needs explicit max hops, node budget,
  edge allowlist, ordering, time window, trust threshold, and a deterministic
  packet before the LLM writes prose.

Current generic Go adapter validation:

```text
go test ./internal/entgraph -count=1
go test ./internal/graphcontext ./internal/graphstore -count=1
go test ./internal/graphql -run TestBoundedGraphContext -count=1
go test ./internal/httpapi -run 'TestGraphQLBoundedGraphContext' -count=1
```

The document/message/ticket regression is the current anti-pigeonhole test. It
must keep passing before any broader Cubicle AI context claim is made. The
high-degree hub regression is the current relation-filter anti-bleed test; it
proves caller-supplied association allowlists can prevent a shared person from
bridging into unrelated work, but it is not a substitute for authorization or
path-scoped coverage. The read-filter regressions are the current traversal
auth tests; they prove unreadable rows are filtered before fanout counting in
memory, Ent, GraphQL resolver, and HTTP request paths. They do not prove a
production principal/group ACL provider or source-specific ACL translator yet.

The auth-limited bounded graph regression is the current coverage-safety test.
It must keep passing before any source-limited graph brief can claim product
absence, source completeness, or sanitized prompt readiness:

```text
sh tools/eval_packs/bounded_graph_auth_limited/run_eval.sh
```

The relation/path/source/time coverage regression proves seed-level completeness
cannot enable absence claims. A request filtered to `implemented_by` needs
coverage for `implemented_by`, source system, source instance, and a freshness
window; an unfiltered traversal needs wildcard coverage before any absence claim
is allowed.

The multi-evidence regressions split consolidated evidence from true duplicate
logical rows. A consolidated relationship can be claimable when its latest
evidence is source-authoritative; stale or otherwise unclaimable sibling rows
for the same logical relationship stay visible as audit context while disabling
confirmed-fact citations for that logical edge. This is a prompt-safety
invariant, not a complete source-of-truth conflict resolution policy.

The Ent-backed bridge regression is the current "not sample-only" test. It must
prove that `boundedGraphContext` can traverse persisted typed product rows from
a `Ticket`, `PullRequest`, `Document`, or `Message` start while returning
canonical directed relationships and evidence, including:

```text
Ticket -> implemented_by -> PullRequest
Ticket -> documented_by -> Document
Ticket -> discussed_in -> Message
Ticket -> assignee / reporter / owner -> Person
PullRequest -> author / creator -> Person
PullRequest -> reviewer / approver / commenter / requested_reviewer -> Person
Document -> links_to -> Document
```

GraphQL product boundary:

```graphql
query {
  boundedGraphContext(
    startObjectType: "document"
    startKey: "doc:architecture-note"
    depth: 2
    limitPerObject: 4
  ) {
    contextHash
    coverage { coverageState absenceClaimsAllowed absenceClaimGateReason }
    objects { objectType key claimAllowed claimGateReason }
    associations { associationType claimAllowed claimGateReason confidence }
  }
}
```

This query is intentionally generic, but production must inject a real typed
graph expander before it returns data. Without that dependency, the resolver
returns a configuration error rather than an empty graph. Callers cannot supply
`coverageState` or `absenceClaimsAllowed`; those values are server-owned and
must remain sparse/false until a source-scope coverage provider proves a
stronger policy.

The current HTTP server uses `entgraph.ProductExpander` automatically when
`RouterOptions.EntClient` is present and no explicit `GraphExpander` is passed.
That makes the PoC usable over persisted `Ticket`/`PullRequest`/`Document`/
`Message`/`Person` objects and supported typed relationships without serving
fake sample data. The bridge is intentionally narrow: it does not traverse
`SourceSync*`, WorkProgram packets, analytics tables, generated briefs, document
fragments, person identities, mentions, unsupported authorship families, or raw
replay artifacts.

Current executable generic PoC:

```text
sh tools/eval_packs/bounded_graph_minimum/run_cli_demo.sh
sh tools/eval_packs/bounded_graph_auth_limited/run_eval.sh
sh tools/eval_packs/incident_runbook_minimum/run_eval.sh
SEEDS=message bash tools/eval_packs/company_ai_first_minimum/run_llm.sh

RUN_MLX=1 sh tools/eval_packs/bounded_graph_minimum/run_cli_demo.sh
SEEDS=all bash tools/eval_packs/company_ai_first_minimum/run_llm.sh
```

The minimum script writes replay artifacts to `/tmp/bounded_graph_cli_demo` by
default. It runs the generic CLI exporter, deterministic baseline, golden eval,
and optionally the local MLX raw/repaired passes. The auth-limited script writes
to `/tmp/bounded_graph_auth_limited` by default and checks that 403/429 coverage
cannot become absence claims or raw prompt facts.

The incident/runbook script is the current non-ticket/PR falsifier. It exports a
memory-store graph shaped as `customer_account -> incident -> slack_message` and
`incident -> runbook_document`, filters out a shared Slack-channel branch, then
injects a visible disconnected finance incident distractor. The deterministic
bounded graph answer must pass golden `6/6` and beat both seed-only and
typed-row baselines; the latest run scored candidate `6/6`, seed-only `2/6`,
and typed-row `2/6`.

The company AI-first LLM runner is the normal Ent-backed local model path. It
seeds the tiny persisted product graph, exports `boundedGraphContext`, runs the
generic prompt through either `--mlx-model` or `LLM_COMMAND`, evaluates raw
output, repairs display output, evaluates the repaired brief, and prints the
generic and typed-row baseline scores beside both model scores. Default seed is
`message` for a quick local PoC; `SEEDS=all` runs document, person, pull
request, ticket, and message. Default local MLX max tokens is `24576`.

The bounded graph render/eval steps now use
`services/ontology-service/tools/bounded_graph_brief.py`. That wrapper accepts
only `boundedGraphContext` input and generic prompt mode; it intentionally does
not expose Ent DB reads, analytics DB reads, `WorkProgramGraphContext`, or
generated-insight persistence. It also rejects missing
`scopeMode=bounded_graph_context`, `WorkProgramGraphContext` payloads,
WorkProgram row-family keys/object types, analytics rows, and analytics
citations before prompt construction. `cubicle_graph_brief.py` remains the
mixed legacy harness for WorkProgram operating contexts and answer-comparison
utilities.

Equivalent manual commands:

```text
go run ./cmd/ontology-service bounded-graph-context-export \
  --out /tmp/bounded_graph_context_cli.json

.venv/bin/python tools/bounded_graph_brief.py \
  --bounded-graph-context-json /tmp/bounded_graph_context_cli.json \
  --context-json /tmp/bounded_graph_context_cli.normalized.json \
  --brief-md /tmp/bounded_graph_cli_scaffold.md \
  --generic-baseline-md /tmp/bounded_graph_cli_generic_baseline.md \
  --prompt-mode generic \
  --prompt-md /tmp/bounded_graph_cli_prompt.md

.venv/bin/python tools/bounded_graph_brief.py \
  --bounded-graph-context-json /tmp/bounded_graph_context_cli.json \
  --context-json /tmp/bounded_graph_context_cli.normalized.eval.json \
  --brief-md /tmp/bounded_graph_cli_scaffold_eval.md \
  --prompt-mode generic \
  --prompt-md /tmp/bounded_graph_cli_prompt_eval.md \
  --llm-brief-md /tmp/bounded_graph_cli_generic_baseline.md \
  --evaluation-json /tmp/bounded_graph_cli_eval.json \
  --golden-json tools/eval_packs/bounded_graph_minimum/golden_questions.json
```

The latest deterministic run passed smoke eval and golden `5/5` with zero
unknown citations, zero uncited material claims, zero forbidden claims, and zero
semantic guardrail violations. This proves the executable traversal-to-brief
harness works for one tiny generic graph. It does not prove production
readiness.

Local model result on the CLI-exported generic graph:

- Raw MLX/Qwen output still invented an
  `[analytics:forecast_summary:eta_forecast_ready]` citation even though generic
  contexts expose no analytics citations.
- Raw output also made a confirmed relationship claim from a graph object
  citation: it said the message was associated with a ticket as a possible
  follow-up while citing only `[graph_objects:message:standup-1]`.
- Raw MLX scored golden `3/5` and failed smoke eval.
- The verifier now rejects confirmed relationship claims unless they cite a
  claim-allowed graph association. This closes the object-citation overclaim
  found in the local model run.
- Repair can remove the invented analytics claim and the bad confirmed
  relationship line, yielding smoke-passing, golden `5/5` display-safe output.
  This remains repair success, not raw model reliability.

## Harness Entry Point

Use `services/ontology-service/tools/bounded_graph_brief.py` as the generic
bounded-graph PoC harness:

```text
--bounded-graph-context-json bounded_graph_context.json
--context-json normalized_context.json
--brief-md scaffold.md
--typed-row-baseline-md typed_row_baseline.md
--generic-baseline-md generic_graph_baseline.md
--prompt-mode generic
--llm-brief-md answer.md
--repaired-brief-md repaired.md
--evaluation-json eval.json
--golden-json golden_questions.json
```

Use `services/ontology-service/tools/cubicle_graph_brief.py` only for
WorkProgram operating graph contexts and answer-comparison utilities:

```text
--graph-context-json context.json
--typed-row-baseline-md typed_row_baseline.md
--generic-baseline-md generic_graph_baseline.md
--prompt-mode generic
--llm-brief-md answer.md
--repaired-brief-md repaired.md
--evaluation-json eval.json
--golden-json golden_questions.json
```

Export real GraphQL-shaped input from an ontology database with the CLI first:

```text
go run ./cmd/ontology-service work-program-graph-context-export \
  --database ontology.db \
  --workstream-key workstream:flink-kubernetes-operator \
  --source-instance flink-pr-jira-1000-plus-500-jira-2026-06-22 \
  --item-limit 12 \
  --action-limit 12 \
  --edge-limit 30 \
  --insight-limit 12 \
  --forecast-limit 12 \
  --evidence-limit 30 \
  --traversal-depth 2 \
  --out work_program_graph_context.json
```

The exporter calls the GraphQL resolver over typed Ent rows and writes:

```json
{"data":{"workProgramGraphContext":{}}}
```

This is the production-side boundary for live model runs. Old normalized SQL
context bundles may still be used for replay/debug baselines, but they must not
drive `--mlx-model`, `--llm-command`, or persistence.

`--golden-json` adds the product-question gate on top of the smoke verifier. A
brief can pass formatting and citation smoke tests but still fail the golden
gate if it misses expected facts, omits required citations, uses a forbidden
phrase, skips required sections, or the suite declares `required_categories`
or `required_source_coverage_states` that are not represented by any question.
For sparse or auth-limited contexts, a question may define `expected_no_answer`
as an alternative to `expected_facts`; this lets the correct answer be
"unknown / not claimable" when coverage cannot support a product fact.

For strategy comparisons, use:

```text
--golden-json golden_questions.json
--compare-answers-json answers.json
--comparison-json comparison.json
--require-promotion-gates
```

Answer comparison shape:

```json
{
  "promotion_gates": [
    {
      "key": "packet-context-over-typed-row",
      "baseline_key": "typed-row-baseline",
      "candidate_key": "packet-context"
    }
  ],
  "answers": [
    {
      "key": "typed-row-baseline",
      "label": "Typed-row baseline",
      "strategy": "typed_row_summary",
      "answer_kind": "raw",
      "path": "typed_row_answer.md"
    },
    {
      "key": "packet-context",
      "label": "Packet-rich graph context",
      "strategy": "packet_context_mlx",
      "answer_kind": "raw",
      "path": "packet_context_answer.md"
    }
  ]
}
```

## Latest Contract Hardening

The live memory-folder questioner and current bounded-graph adversary are now
saved as debate artifacts:

```text
/Users/harsh/workspace/debate/ai-first-bounded-graph-validation-20260624/agents/memory-folder-questioner-planck-20260624.md
/Users/harsh/workspace/debate/ai-first-bounded-graph-validation-20260624/agents/current-bounded-graph-adversary-laplace-20260624.md
```

Their shared conclusion is deliberately narrow: `boundedGraphContext` plus
deterministic citation/repair gates is a promising generic prompt contract. It
is not yet proof that raw model output over connector-backed company data is
production-safe.

New contract gates:

- `tools/bounded_graph_contract.py` validates saved `boundedGraphContext` JSON
  before prompt construction. The generic eval scripts call it directly.
- Connector-backed exports must run with `--profile connector`. In connector
  profile, warnings are blocking until policy says otherwise.
- The company eval pack now exercises that connector profile for source-backed
  Ent exports. Its docs, Jira, GitHub, and chat seeds carry
  `SourceScopeState -> SourceSyncRun` coverage windows, source
  system/instance, and relationship coverage policy. The deterministic and LLM
  company scripts validate `document`, `ticket`, `pull_request`, and `message`
  contexts with `--profile connector`; `person` stays source-neutral.
- Claimable objects must explicitly declare `visibility=public`; missing
  visibility is not enough for product facts.
- Claimable associations must carry an `evidenceKey` and that key must resolve
  to a matching evidence row in saved JSON.
- Partial, stale, restricted, or generated objects remain visible only as
  validation context.
- Associations with missing, restricted, or partial endpoints remain visible
  only as validation context. The standalone JSON contract now checks that every
  claimable association points at endpoint object rows that are present, public,
  and hydrated/current.
- Duplicate logical associations with the same from/type/to remain visible only
  as validation context. The Go projector and standalone JSON contract both
  fail them closed until product relationship merge and writer-authority policy
  picks canonical truth.
- Generated writer output is not product truth. Claimable generated objects,
  generated relationship evidence, and Go relationship metadata sourced from
  `cubicle_ai`, `generated`, or `llm` fail closed until source-backed evidence
  promotes the claim.
- Fact-level promotion audit is executable through
  `tools/bounded_graph_promotion_audit.py`. It reports which bounded graph
  objects and associations are promotion-ready and which remain blocked by
  candidate status, missing evidence, generated evidence, duplicate logical
  relationships, partial endpoints, visibility, freshness, confidence, or
  unauthoritative relationship sources. With `--source-authority-json`, the
  audit also requires every claimable relationship family to appear in the
  source-authority matrix and requires the authority source to come from the
  relationship evidence row. This audit is now part of the company eval output,
  alongside answer-level promotion gates.
- The bounded-only CLI now writes `brief-md` with the generic bounded renderer,
  not the mixed WorkProgram/analytics scaffold.
- `evaluate_llm_brief` now emits a `statement_support` audit. Every material
  bullet is reported with its section, citations, citation claim uses, support
  status, and support failures. Smoke eval requires zero unsupported
  statements, so uncited lines, unknown citations, disconnected rows,
  unsupported confirmed relationships, unsupported absence claims, unsupported
  product claims, and source URL leakage fail before golden scoring.

The auth-limited fixture now reflects the rule: its partial document endpoint
and ticket-document association are visible as validation context, not claimable
product facts. This prevents `403`/`429` or partial hydration from becoming
absence, completion, or confirmed relationship claims.

The statement-support audit is not a full semantic entailment model. It is the
first executable traceability layer over generated prose. Stronger entailment is
still required before model output can become anything stronger than
verifier-gated display text.

Source-authority enforcement is now proven in the fact-level promotion audit,
company eval pack, and Ent-backed `boundedGraphContext` runtime projection.
It is not yet a universal product policy: other product display paths must not
treat `claimAllowed=true` alone as sufficient for relationship truth until they
apply the same relationship-family authority boundary. It is also not yet a
durable merge/source-precedence policy because source instance and extractor
identity are not part of the authority decision.

Current focused verification:

```text
.venv/bin/python -m unittest tools.test_bounded_graph_brief tools.test_cubicle_graph_brief
go test ./internal/graphcontext ./internal/entgraph ./internal/sampledata
sh tools/eval_packs/bounded_graph_minimum/run_cli_demo.sh
sh tools/eval_packs/bounded_graph_auth_limited/run_eval.sh
sh tools/eval_packs/incident_runbook_minimum/run_eval.sh
sh tools/eval_packs/company_ai_first_minimum/run_eval.sh
.venv/bin/python tools/bounded_graph_contract.py \
  --bounded-graph-context-json /tmp/company_ai_first_minimum/ticket_context.json \
  --profile connector \
  --report-json /tmp/company_ai_first_minimum/ticket_connector_contract.json
```

The comparison output ranks answers by passed golden questions, then by fewer
failures. It also reports `category_summary` per answer and
`best_answer_keys_by_category`. When `promotion_gates` are supplied, it also
reports whether a candidate beats its baseline overall and avoids regressions
across every required category. A packet-heavy path must pass those promotion
gates before a new packet, table, or graph-context field is promoted. Use
`--require-promotion-gates` in CI or release checks so the command exits
nonzero when a candidate fails its baseline gate.

A runnable reference pack lives at:

```text
services/ontology-service/tools/eval_packs/ai_first_mixed_minimum
```

It covers ten categories and demonstrates the promotion-gate mechanics with
reference answers. It is a harness check, not live-model proof. Replace or
extend it with real captured GitHub-only, ticket-only, sparse mixed-source, and
auth-limited contexts before using the result to promote product schema.

A runnable generic graph minimum pack lives at:

```text
services/ontology-service/tools/eval_packs/bounded_graph_minimum
```

It uses a document seed, message object, ticket object, one claimable
association, one non-claimable candidate association, and sparse source
coverage. This is the anti-pigeonhole smoke test: it must continue to work
without Flink, GitHub PRs, Jira issue assumptions, or `WorkProgram*` rows.

Local model result:

- The deterministic generic baseline passes the smoke verifier with no
  analytics citations.
- A local MLX/Qwen run correctly summarized the document/message/ticket graph,
  but still invented an `[analytics:forecast_summary]` citation after analytics
  shortcuts were removed from the prompt. The verifier rejected the raw answer;
  the repair pass removed the bad bullet and produced display-safe output.
- Interpretation: generic graph + LLM is viable as a PoC path, but model output
  must remain verifier-gated and generated. Raw model output is not a product
  fact and should not persist unless it passes citation, structure, semantic,
  and golden gates without repair.

`--typed-row-baseline-md` emits a deterministic answer from only typed
work-item/action rows and guardrails. It is the minimum baseline for checking
whether bounded graph traversal adds value over direct typed product rows.

`--generic-baseline-md` emits a deterministic, smoke-evaluable answer from the
same normalized graph rows, including bounded traversal shape and validation
context. It is the minimum baseline for checking whether the LLM is adding
useful synthesis beyond safe graph traversal.

Golden question shape:

```json
{
  "required_categories": [
    "github-only",
    "ticket-only",
    "sparse-coverage"
  ],
  "required_source_coverage_states": [
    "complete",
    "sparse",
    "auth_limited"
  ],
  "questions": [
    {
      "key": "flink:what-next",
      "category": "github-only",
      "source_coverage_state": "complete",
      "question": "What should a TPM do next?",
      "expected_facts": [
        {
          "text": "risk triage",
          "citation": "[analytics:tpm_forecast_summary]"
        }
      ],
      "expected_no_answer": [
        {
          "text": "coverage is too sparse to answer",
          "citation": "[source_coverage:workstream:example]"
        }
      ],
      "expected_citations": [
        "[analytics:tpm_evaluation_readiness]"
      ],
      "forbidden_phrases": [
        "will merge by Friday",
        "confirmed blocker"
      ],
      "required_sections": [
        "## What Not To Claim"
      ]
    }
  ]
}
```

## Latest Debate Refresh

The whole-memory questioner and two-agent architecture debate are captured in:

- `/Users/harsh/workspace/debate/ai-first-bounded-graph-validation-20260624/agents/memory-folder-questioner-hegel-20260624.md`
- `/Users/harsh/workspace/debate/ai-first-bounded-graph-validation-20260624/agents/current-bounded-graph-adversary-singer-20260624.md`
- `/Users/harsh/workspace/debate/ai-first-bounded-graph-validation-20260624/agents/typed-ontology-defender-turing-20260624.md`

Current safe claim:

- `boundedGraphContext` is a promising generic prompt contract.
- The Ent-backed runtime is still mostly company-work/product shaped.
- The next genericity proof must be Ent-backed, non-work, connector-shaped, and
  pass traversal, authority, coverage, ACL, distractor, and baseline gates
  without adding bespoke Go expansion switches.

Do not promote repair-dependent prose as product truth. Raw, repaired, and
persisted generated outputs must be scored separately. If repair is required
for user-facing answers, say that explicitly in the product contract.

Do not treat source plus locator kind as durable merge authority. It is enough
for current `boundedGraphContext` claim gating only when it comes from resolved
evidence. Durable source precedence still needs source instance, extractor
identity and version, source scope/run, ACL snapshot, freshness window,
relationship direction/type, and writer authority by row family.

## Ent-Backed Open Graph Proof

The first non-work Ent-backed proof is implemented in:

- `services/ontology-service/ent/schema/open_graph_object.go`
- `services/ontology-service/ent/schema/open_graph_association.go`
- `services/ontology-service/internal/entgraph/open_expander.go`
- `services/ontology-service/internal/graphql/bounded_graph_open_ent_test.go`

This proves the generic contract can run on Ent-backed open connector rows:

```text
customer_account
  -> affected_by -> incident
  -> mitigated_by -> runbook_document
  -> updated_in -> slack_message
```

The regression intentionally checks both sides of the authority boundary:

- the central company-work matrix blocks open relationship families by default
- a connector-owned matrix makes those same rows claimable when resolved
  evidence carries the expected source and locator kind

This changes the current safe claim from "generic only in memory-store JSON" to
"generic has an Ent-backed open graph path." It is still a PoC path until a real
connector loader, connector-authority lifecycle, and production ACL/merge policy
validate the same path outside fixtures.

## Open Graph CLI/Eval Pack

The open proof is now also executable through the CLI/eval harness:

- `services/ontology-service/internal/sampledata/open_graph_incident.go`
- `services/ontology-service/cmd/ontology-service/main.go`
- `services/ontology-service/tools/eval_packs/open_graph_incident_minimum/run_eval.sh`

The eval seeds persisted `open_graph_objects` and `open_graph_associations`,
exports `boundedGraphContext` through `OpenGraphExpander`, applies a
fixture-specific source-authority matrix, and compares the depth-2 answer
against seed-only and typed-row baselines.

Current result:

- depth-2 open graph traversal: `7/7`
- seed-only baseline: `2/7`
- typed-row baseline: `2/7`
- promotion audit: all exported associations promotable under the connector
  authority matrix
- local MLX/Qwen raw answer with `--llm-max-tokens 16384`: `0/7`
- deterministic repair of that raw answer: `7/7`

Updated safe claim:

`boundedGraphContext` has an Ent-backed, non-work, connector-shaped PoC path
with repeatable contract, promotion-audit, and baseline gates. It is not yet a
production connector architecture. Remaining production proof needs a real
connector writer, connector-authority lifecycle, source/workspace ACL
translation, and durable merge/source-precedence rules.

Model-output implication:

More output tokens did not make raw generation safe. The raw answer used
incorrect citation granularity and introduced an unsupported analytics citation.
Repair passed by reducing the answer back to deterministic cited graph facts.
Generated prose remains display-only until raw, repaired, and persisted outputs
are separately scored and promotion-gated.

## HTTP Routed Expander

The server-side GraphQL composition now has the same product/open split:

- `services/ontology-service/internal/entgraph/routed_expander.go`
- `services/ontology-service/internal/httpapi/graphql.go`
- `services/ontology-service/internal/httpapi/router.go`
- `services/ontology-service/internal/httpapi/router_test.go`

When `RouterOptions` has an Ent client and no explicit expander, GraphQL uses a
routed expander:

- product object types use `ProductExpander`
- open connector object types use `OpenGraphExpander`
- explicit connector relationship authority can be passed through
  `RouterOptions.BoundedGraphSourceAuthority`

Current result:

- `/graphql boundedGraphContext` can start at `customer_account/customer:acme`
  and traverse persisted open graph rows
- the private high-rank incident remains filtered before fanout accounting
- open relationships become claimable only when the router is given connector
  source authority

Remaining gap:

Routing is now good enough for the PoC. It does not solve production connector
governance: source-authority versioning, writer authority, ACL translation, and
cross-family merge/source-precedence policy are still required.

## Data-Backed Open Graph Loader

The non-work proof is now data-backed instead of only sampledata-backed:

- `services/ontology-service/internal/opengraphfixture/loader.go`
- `services/ontology-service/cmd/ontology-service/main.go`
- `services/ontology-service/tools/eval_packs/open_graph_incident_minimum/open_graph_fixture.json`

`open-graph-fixture-load` writes connector-shaped JSON into:

- `OpenGraphObject`
- `OpenGraphAssociation`
- relationship `Evidence`

The eval path then exports `boundedGraphContext` from the same persisted DB.
This proves the prompt contract can start from arbitrary open connector objects
without adding a typed Ent schema for each object family.

Atomic-load rule:

- A malformed fixture must not leave partial open graph rows behind.
- A repeated fixture load currently fails on uniqueness and must not inflate
  objects, associations, or evidence.
- This is not production upsert semantics. Real connectors still need writer
  authority, source-authority versioning, merge/source-precedence, and ACL
  translation.

Current validation:

- `go test ./...`
- `.venv/bin/python -m unittest tools.test_bounded_graph_brief tools.test_cubicle_graph_brief`
- `OUT_DIR=/tmp/data_backed_open_graph_incident_eval sh tools/eval_packs/open_graph_incident_minimum/run_eval.sh`

## Open Graph ACL/Fanout Gate

The open graph expander now has a production-shaped ACL regression:

- `services/ontology-service/internal/entgraph/open_expander_test.go`

The fixture intentionally ranks inaccessible rows above accessible rows:

- public seed
- private high-ranked hub
- public descendant behind the private hub
- team-only relationship to a public runbook
- public descendant behind the team-only relationship
- lower-ranked public direct neighbor

Required behavior:

- public-only principals must skip private and team-only rows before those rows
  consume fanout
- public-only principals must not see public descendants reachable only through
  private or team-only paths
- team principals may traverse team-only edges but still cannot see private
  hubs
- unrestricted principals see the private high-ranked hub first

This proves the generic Ent open graph traversal applies read filters before
expansion and fanout accounting. It does not yet prove production ACL
translation from real sources. That still requires connector ACL mapping,
principal/group semantics, policy versions, and coverage audits.

## Source Coverage Matrix

Bounded graph absence claims now have an explicit matrix regression:

- `services/ontology-service/internal/graphql/bounded_graph_context_query_test.go`
- `TestBoundedGraphContextSourceCoverageMatrixGatesAbsenceClaims`

Absence claims are allowed only when source coverage is complete across source,
relationship path, principal, and time:

- exact source scope
- fresh source scope state
- matching product-row source system and source instance
- explicit requested relationship path
- relationship family supported by that source/object type
- principal-aware coverage
- successful source run with coverage window
- no matching auth/rate-limit source sync issue

The matrix keeps absence claims disabled for:

- wrong source instance
- stale source scope
- partial source scope
- missing principal coverage
- unfiltered traversal
- missing source time window
- auth/rate-limited source issue

The auth/rate-limit case verifies that raw `429`/secret-like source issue body
text stays out of the prompt summary. Those bodies remain replay/coverage
evidence only.

Current safe claim:

`boundedGraphContext` can support LLM summarization over graph traversal without
turning missing neighbors into absence claims unless source coverage is proven
for the requested relation path, principal, source instance, and time window.

## Fresh Architecture Review

Two fresh read-only agents were run after the data-backed open graph slice:

- `debate/ai-first-bounded-graph-validation-20260624/agents/memory-folder-questioner-volta-20260624.md`
- `debate/ai-first-bounded-graph-validation-20260624/agents/current-bounded-graph-review-euler-20260624.md`

The current answer:

- Keep durable product truth strongly typed in Ent once identity, lifecycle,
  writer authority, and product reads are stable.
- Use `OpenGraphObject` / `OpenGraphAssociation` as a connector escape hatch
  while new object families prove value.
- Keep `boundedGraphContext`, absence gates, repair, generated summaries, and
  LLM prose as runtime/display projections, not canonical graph truth.

Do not model in Ent yet:

- every connector object family
- arbitrary connector relationship families
- `BoundedGraphContext` as durable truth
- raw manifests, source-sync bodies/counters, source fragments, search
  projections, or raw message bodies as traversal adjacency
- generated summaries, repaired prose, ETA commitments, blocker candidates, or
  forecast-risk scores as product truth without lifecycle and eval improvement

Next gates:

- second non-Flink production-shaped domain
- real connector ACL translation into principal/group-aware read filters
- source-authority lifecycle and connector writer governance
- raw, repaired, deterministic, and persisted-generated scores kept separate

## Second Open Graph Eval Pack

`tools/eval_packs/open_graph_revenue_minimum` is the second non-Flink generic
open-graph proof.

The pack loads persisted `OpenGraphObject` and `OpenGraphAssociation` rows from
JSON, then exports `boundedGraphContext` from the same Ent-backed open graph
path used by the incident pack. It does not add `CustomerAccount`,
`RenewalOpportunity`, `SupportCase`, or `PlaybookDocument` product schemas.

Fixture shape:

- seed: `customer_account/customer:globex`
- visible relationships: `has_opportunity`, `blocked_by`, `guided_by`,
  `updated_in`
- private distractor: `opportunity:hidden-private-renewal`
- coverage state: sparse, so missing opportunities/cases remain unknown

The eval requires the bounded traversal answer to beat:

- depth-0 seed-only summary
- typed-row/object-only summary with no relationship association citations

Current deterministic scores:

- candidate depth-3 open graph traversal: `8/8`
- seed-only baseline: `2/8`
- typed-row baseline: `2/8`

Local model runner:

- `tools/eval_packs/open_graph_revenue_minimum/run_llm.sh`

Default local settings:

- model: `mlx-community/Qwen3-Coder-30B-A3B-Instruct-bf16`
- max tokens: `24576`
- timeout: `1200` seconds
- raw pass required: no
- repaired pass required: yes

Current local-model scores:

- raw MLX/Qwen answer: smoke fail, golden `2/8`
- raw failure mode: invented an unknown `[analytics:43740ce6e0bf1899]`
  citation and did not cite the exact graph facts required by the golden pack
- repaired answer: smoke pass, golden `8/8`
- deterministic traversal baseline: smoke pass, golden `8/8`
- seed-only baseline: smoke pass, golden `2/8`
- typed-row baseline: smoke pass, golden `2/8`

The original incident/runbook pack still passes after this change:

- candidate depth-2 open graph traversal: `7/7`
- seed-only baseline: `2/7`
- typed-row baseline: `2/7`

Scaffold rule:

Bounded open-graph briefs may emit four bullets per section so a four-edge
traversal can be cited. The existing WorkProgram/TPM brief and repair contract
keeps the prior three-bullet section cap. This is an explicit generic-graph
exception, not a loosening of product operating briefs.

Safe claim:

The PoC can now summarize at least two non-product open graph domains through
the same persisted row, source-authority, ACL, coverage, and verifier path.
That supports continuing the `OpenGraphObject` / `OpenGraphAssociation`
escape-hatch design. Local model execution is feasible on this hardware with a
larger token budget, but raw model output still fails the safety/eval contract.
It does not prove production readiness until the same gates pass on
connector-backed data with real ACL translation, writer authority,
source-authority versioning, and raw/repaired/persisted LLM score separation.

## Real Connector Bounded Probe

`tools/eval_packs/real_connector_bounded_probe/run_llm.sh` probes an existing
ontology SQLite DB rather than a synthetic fixture.

It uses `tools/bounded_graph_dynamic_golden.py` to generate a golden-question
pack from the exported `boundedGraphContext`, then scores:

- raw local model answer
- repaired local model answer
- deterministic generic traversal baseline
- seed-only baseline
- typed-row/object-only baseline

First connector-backed run:

- DB:
  `/Users/harsh/workspace/data/flink-pr-jira-1000-plus-500-jira-2026-06-22/ontology.ai-tpm-1000-open-auth-hydrated-retry998-20260622.db`
- seed: `ticket/ticket:jira:FLINK-32695`
- association filter: `implemented_by`
- traversal depth: `1`
- fanout: `6`

Context/promotion result:

- `7` bounded objects
- `6` bounded associations
- `1` promotable object
- `6` blocked objects
- `0` promotable associations
- `6` blocked associations
- relationship blocker: `relationship_endpoint_partial_requires_hydration`

Local-model scores with `mlx-community/Qwen3-Coder-30B-A3B-Instruct-bf16` and
`LLM_MAX_TOKENS=24576`:

- raw answer: smoke fail, golden `1/9`
- raw failure mode: invented `[analytics:forecast_summary:eta_forecast_ready]`
  and did not preserve exact cited graph facts
- repaired answer: smoke pass, golden `9/9`
- deterministic traversal baseline: smoke pass, golden `9/9`
- seed-only baseline: smoke pass, golden `3/9`
- typed-row baseline: smoke pass, golden `3/9`

Updated interpretation:

The generic bounded graph path can now run over real connector-backed rows.
The result is not a product-readiness proof. It shows that partial source
coverage is a dominant real-data issue: the graph can expose ticket-PR links as
validation context, but cannot confirm them as product truth until PR endpoints
are hydrated and relationship evidence is current. It also shows that raw local
model output still drifts into analytics/forecast vocabulary, so verifier,
repair, and raw/repaired score separation are core architecture, not polish.

## Prompt Boundary Retest

The generic bounded graph prompt boundary is:

```text
boundedGraphContext
  -> graph objects
  -> graph associations
  -> evidence
  -> source_coverage
  -> guardrails
  -> citation_policy
```

It is not:

```text
WorkProgramGraphContext
  -> forecast_summary
  -> measurement_readiness
  -> blocker_candidate_count
  -> TPM analytics shortcuts
```

Decision:

Synthetic analytics removal is not the full architecture boundary. The boundary
is the bounded-only generic entrypoint plus contract and promotion gates.
`WorkProgramGraphContext` remains the TPM/operating-brief path; generic bounded
graph prompts must stay graph/evidence/source-coverage/citation-policy shaped.

Prompt contract additions:

- Generic bounded prompt JSON exposes source coverage as `source_coverage`, not
  under an `analytics` wrapper.
- Generic bounded prompts must not contain `[analytics:]`, `"analytics"`,
  `forecast_summary`, `measurement_readiness`, `blocker_candidate_count`, `TPM`,
  `WorkProgram`, or `ETA` unless those strings are actual graph input.
- Generic bounded prompt JSON may include a deterministic `graph_summary` with
  `object_count`, `association_count`, `traversal_count_phrase`, and
  `association_endpoint_format`.
- `Confirmed Facts` require claimable row citations. Gated objects and gated
  associations belong in `Validation Leads` or `What Not To Claim`.
- Relationship claims need claim-allowed association citations; context,
  guardrail, and source-coverage citations are not product relationship proof.

Observed validation after the retest:

- Python tests: `91` pass
- Revenue deterministic eval: `8/8`, seed-only `2/8`, typed-row `2/8`
- Incident deterministic eval: `7/7`, seed-only `2/7`, typed-row `2/7`
- Real connector dry run: repaired and deterministic `9/9`, seed-only and
  typed-row `3/9`
- Sequential local MLX/Qwen at `LLM_MAX_TOKENS=24576`:
  - revenue raw `3/8`, smoke pass, no unknown citations, no policy violations,
    no unsupported statements
  - revenue repaired `8/8`
  - real connector raw `2/9`, smoke fail, invented `[traversal:seed_subject_keys]`,
    and promoted a non-claimable partial relationship in `Confirmed Facts`
  - real connector repaired `9/9`

Interpretation:

The prompt cleanup removes the analytics/forecast prompt contamination and makes
generic prompts domain-neutral. It does not make raw local model output a safe
product answer path. The PoC must keep deterministic scaffolding,
statement-support audit, repair, and separate raw/repaired/deterministic scores.

Follow-up claimability metadata:

- Generic `graph_summary` includes claimable/gated object and association
  counts.
- Generic `graph_summary.confirmed_fact_instruction` states whether association
  rows may appear in `Confirmed Facts`.
- Generic prompts explicitly forbid bracket citations derived from JSON field
  names such as `seed`, `traversal`, `rows`, `graph_summary`, and
  `source_coverage`.

Retest:

- Real connector raw: smoke pass, golden `2/9`, unknown citations `0`, policy
  violations `0`, unsupported statements `0`
- Real connector repaired: `9/9`
- Revenue raw: smoke pass, golden `1/8`, unknown citations `0`, policy
  violations `0`, unsupported statements `0`
- Revenue repaired: `8/8`

Interpretation:

The claimability metadata improved raw safety on partial connector data, but it
did not make raw output complete. Generic raw output can be used as a diagnostic
signal; repaired/deterministic output remains the product-facing PoC path.

Endpoint phrase refinement:

- `graph_associations` prompt rows include `endpoint_phrase`, formatted as
  ``from_key` -> `to_key``.
- Exact relationship wording should live on the cited association row, not in a
  separate summary list.
- Summary fields remain copy-only hints and must not become a citation
  namespace.

Retest:

- Real connector raw: smoke pass, golden `3/9`, unknown citations `0`, policy
  violations `0`, unsupported statements `0`
- Real connector repaired: `9/9`
- Revenue raw: smoke pass, golden `1/8`, unknown citations `0`, policy
  violations `0`, unsupported statements `0`
- Revenue repaired: `8/8`

Interpretation:

This is a safer generic prompt shape, but not enough for raw product answers.
The bounded graph PoC should treat raw model output as diagnostic evidence and
continue using deterministic/repair gates for user-facing summaries.

## Claimable Connector Profile

The next validation layer is not prompt polish. It is connector claimability.

Added audit and gate updates:

- `tools/bounded_graph_probe_candidates.py` scans persisted ontology DBs for
  source-instance mix, relationship family coverage, promotable-looking rows,
  and runnable bounded-graph probe hints.
- `tools/eval_packs/real_connector_bounded_probe/run_llm.sh` now defaults to
  `CONTRACT_PROFILE=connector`.
- `ontology-service bounded-graph-context-export` accepts explicit connector
  coverage metadata:
  - `--coverage-source-system`
  - `--coverage-source-instance`
  - `--coverage-window-start`
  - `--coverage-window-end`
  - `--absence-claim-association-types`
- `tools/bounded_graph_contract.py` distinguishes missing relation scope from
  an explicitly declared empty proof scope.

Flink DB audit:

- The 1k PR + 500 Jira ontology DB has `10283` promotable-looking typed
  relationships, including `975/1511` promotable `implemented_by` ticket-PR
  rows.
- The same DB has no non-Flink source instances and no OpenGraph rows. It is a
  stronger Flink/Jira/GitHub real connector proof, not a generic second-domain
  proof.

Claimable probe:

```text
seed: ticket:jira:FLINK-36332
association path: implemented_by
depth: 1
fanout: 6
```

Observed state:

- `6` objects
- `5` associations
- `4` claimable `implemented_by` associations
- `1` blocked candidate association with
  `relationship_locator_not_authoritative_for_presence` and
  `relationship_proof_not_source_observed`

MLX/Qwen with `LLM_MAX_TOKENS=32768`:

- raw: smoke fail, golden `1/8`, unknown citations `0`, unsupported statements
  `1`
- repaired: `8/8`
- deterministic: `8/8`
- seed-only: `3/8`
- typed-row: `2/8`

Strict connector contract:

- `boundedGraphCoveragePolicy` falls back to seed-row source identity plus
  `last_confirmed_at` as a point observation window when no `SourceScopeState`
  exists.
- For `ticket:jira:FLINK-36332`, strict connector mode now passes without
  manual coverage overrides:
  - source: `jira/apache-jira`
  - observation window: `2026-06-22T18:05:35Z` to `2026-06-22T18:05:35Z`
  - `coverageState`: `sparse`
  - `absenceClaimsAllowed`: `false`
  - contract warnings: `0`
- This is sparse observation provenance, not complete source-scope coverage.
  Missing neighbors remain unknown, not absent. Production absence claims still
  require exact relation/path/source/time/principal coverage from
  source-scope/run evidence.

Source-specific replay state:

- Flink replay loaders now persist source-specific `SourceConnection`,
  `SourceScope`, and `SourceScopeState` rows for replayed source instances and
  attach typed `Ticket`, `PullRequest`, and directly observed
  `TicketPullRequest` rows to those states.
- Replay source states are `partial_scope` by default. They are provenance and
  freshness gates, not absence authority.
- Partial source states may expose an observation point window from
  `last_attempted_at`, but absence remains disabled.
- `SourceSyncIssue` lookup must include both the row's source scope and matching
  source-object issue families. This prevents stream-scoped `403`/`429` or
  missing-endpoint failures from disappearing after a product row gains a
  source-specific state pointer.
- Fresh validation DB:
  `/Users/harsh/workspace/data/flink-pr-jira-1000-plus-500-jira-2026-06-22/ontology.source-scope-claimable-20260624.db`
  has `2` source states, `1/1` scoped ticket rows, `4/4` scoped PR rows, and
  `4/4` scoped ticket-PR rows for `FLINK-36332` plus PRs `881`, `900`, `906`,
  and `909`.
- The bounded export from that DB reports `coverageState=limited`,
  `absenceClaimsAllowed=false`, `source_scope_not_exact`, and
  `jira/apache-jira` coverage identity.
- The four ticket-PR associations now export as `claimAllowed=true` with
  `claimGateReason=source_evidence_full_confidence`. Jira remote-link evidence
  is source-authoritative for `implemented_by` on these consolidated rows.
  Coverage is still `limited`, so this proves positive relationship claims, not
  absence claims.

Standing anti-pigeonhole suite:

- `tools/eval_packs/ai_first_bounded_graph_suite/run_eval.sh` is the repeatable
  regression gate for the AI-first bounded graph path.
- The default deterministic suite runs core Go bounded graph/sourcegraph/GraphQL
  tests plus `bounded_graph_auth_limited`, `open_graph_incident_minimum`,
  `open_graph_revenue_minimum`, and `company_ai_first_minimum`.
- Observed result with
  `OUT_DIR=/tmp/ai_first_bounded_graph_suite_20260624`: auth-limited `6/6`,
  company document `5/5`, company person/pull-request/ticket/message `4/4`
  each, incident `7/7`, revenue `8/8`, and core Go tests passing.
- The optional real connector pass is enabled with `RUN_REAL_CONNECTOR_LLM=1`
  and keeps raw local-model output separate from repaired and deterministic
  output.

Promotion matrix:

- `tools/bounded_graph_promotion_matrix.py` is the stricter machine-readable
  promotion gate. It runs declared cases from
  `tools/eval_packs/bounded_graph_promotion_matrix/cases.json`, then validates
  contract reports, promotion audits, golden eval reports, promotion comparison
  reports, required coverage tags, and forbidden-term scans.
- Observed result with
  `--out-dir /tmp/bounded_graph_promotion_matrix_20260624`: matrix pass, `5`
  cases, auth-limited `6/6`, incident `7/7`, revenue `8/8`, company multi-seed
  `21/21`, and real connector FLINK-36332 repaired/deterministic `7/7`.
- The real connector raw local-model row remains diagnostic: smoke pass,
  golden `2/7`, no unknown citations, no policy violations, no unsupported
  statements.
- The matrix carries an advisory `real-non-flink-connector` tag. It is now
  covered by `real_github_issue_pr_minimum`, which captures public `cli/cli`
  GitHub issue/PR data into generic OpenGraph rows. Strict advisory mode with
  `--require-advisory-tags` passes.
- The same real GitHub pack now has a local MLX/Qwen gate with
  `LLM_MAX_TOKENS=32768`. Raw model output passes smoke but scores only `2/6`;
  repaired output and deterministic traversal both score `6/6` and beat
  seed-only `2/6` plus typed-row `3/6` baselines. This strengthens the
  non-Flink PoC path, but it does not promote raw output into product truth.
- The full promotion matrix now passes with `7` cases after adding the local
  GitHub LLM case. The GitHub LLM case reports aggregate `14/18` because raw is
  intentionally diagnostic while repaired and deterministic outputs are both
  perfect on the six-question pack.
- `tools/bounded_graph_real_connector_inventory.py` scans the local ontology DB
  pool for candidates that satisfy that advisory tag. After loading
  `/Users/harsh/workspace/data/real-github-issue-pr-cli-2026-06-24/real_github_issue_pr.db`,
  the `/Users/harsh/workspace/data` scan found `46` DBs, `1` real non-Flink
  connector candidate, `27` Flink-shaped candidates, and `1` DB with open graph
  rows.

Standing memory-questioner checks:

- Is OpenGraph/generic serving truth, or a candidate/audit substrate?
- What exactly makes a connector probe claimable?
- Are repaired/deterministic summaries only summarizing claimable graph facts,
  or creating new graph truth?
- How do partial fetches and `403`/`429` stay visible as coverage rather than
  absence?
- How do WorkProgram/TPM consumers avoid becoming claim authorities for
  blockers, ownership, or forecasts?

Standing debate cadence:

- Before schema, prompt, or connector promotion, run four adversarial roles:
  memory-questioner, architecture skeptic, current-code reviewer, and AI-first
  graph advocate.
- Each promotion must leave three concrete artifacts: source-authority matrix,
  fixture expectation table, and product-read invariant checklist.
- Architecture confidence remains tiered: PoC green, production-genericity
  advisory green, and product-safe architecture green are separate claims.
- New connector/domain work needs one positive proof case and one
  negative/partial case covering missing auth, stale source window, duplicate
  or conflicting relationship evidence, hidden hub, wrong source instance, or
  source-not-attempted state.

Updated decision:

Generic bounded graph summaries are viable for the PoC only when they stay
downstream of typed/OpenGraph rows, source authority, claimability, coverage,
ACL/readability filters, statement-support audit, and repair/deterministic
gates. Raw local model output is not the product path.
