# Apache Flink Ent Crawler Design

Date: 2026-06-06

Status: design for review

## Thesis

Build Cubicle's first backend graph as a small Go service that ingests a bounded Apache Flink public-data slice, stores replayable source snapshots, maps them into a Meta/TAO-inspired object-association graph backed by Ent and SQLite, and answers evidence-backed engineering questions.

This is not a generic enterprise search index. It is a proof that Cubicle can connect Jira issues, GitHub pull requests, docs, and discussion into typed execution objects: who owns the work, what changed, what is blocked, where the decision lives, and what evidence supports the answer.

## Refined Design Contract

The revised graph design combines the useful parts of Glean and Palantir without copying either product shape.

```text
Meta/TAO-style graph serving
  typed objects + typed associations + SQL backing store + narrow graph API
        |
        v
Glean-style context graph
  content + people + activity + freshness + visibility
        |
        v
Cubicle engineering execution graph
  tickets + PRs + docs + messages + decisions + blockers + risks
        |
        v
Palantir-style operational ontology
  typed objects + typed links + evidence + read-only action candidates
```

Every query-facing fact must carry:

- source
- source URL or snapshot reference
- observed timestamp
- source-updated timestamp when the source provides one
- confidence
- visibility scope, even when the scope is just `public`
- freshness state: `fresh`, `stale`, `partial`, or `unknown`

This keeps the POC local and small while preventing a fake graph that cannot later support connectors, permissions, or operational actions.

## Meta-Style Graph Serving Contract

The reason to use Ent is not that Ent is a graph database. It is that Ent gives a strong Go schema, migrations, typed object access, and SQL durability while letting Cubicle build a TAO-like graph-serving layer above it.

The durable graph is:

```text
objects
  Person, Project, Ticket, PullRequest, Document, Message, Decision, Blocker, Risk

associations
  from object + predicate + to object + time/sort key + evidence/provenance/security/freshness

SQL store
  Ent schemas over SQLite for V0; same object-association contract can move to Postgres later
```

The Go service exposes graph primitives, not arbitrary SQL. Name this boundary `AssociationStore`, not `GraphDAO`, because Ent codegen already owns the DAO/ORM role.

```text
GetObject(kind, key)
ListAssociations(from, predicate, cursor, limit)
CountAssociations(from, predicate)
Expand(start, predicates, depth, limits)
Intersect(left, right, predicate)
TraceEvidence(edge_or_object)
```

Product queries compile to these primitives. For example, readiness is not a bespoke SQL report; it is `project -> contains tickets -> blocked_by blockers -> evidenced_by fragments/messages/comments -> needs_action action candidates`.

`AssociationStore` is deliberately narrow:

- It wraps Ent-generated queries; it does not duplicate Ent CRUD.
- It handles polymorphic `kind + key` node references that Ent cannot infer.
- It enforces association metadata: evidence, visibility, freshness, confidence, source, observed time.
- It owns cursor/pagination semantics for association lists.
- It is the future cache seam for object rows, association lists, and association counts.

Ban these from `AssociationStore`:

- source-specific ingestion logic
- object creation outside association upsert helpers
- LLM extraction
- product-specific reports such as readiness or decision gaps
- direct Swift-facing response shaping

For the POC there is no cache layer. The API shape still matters because it leaves room for a future TAO-like serving tier that caches object rows, association lists, and association counts without changing Swift or product query code.

## Validation Questions And Corrections

These questions should be re-run before implementation and after each design change.

### Iteration 1: Ent Codegen Boundary

Questions:

- If Ent already generates typed CRUD and traversals, what custom layer remains justified?
- Can this layer become a second ORM?
- Which code is allowed to call `ent.GraphEdge.Query()` directly?
- What would we need to change if a TAO-style cache is inserted later?

Corrections:

- Ent codegen is the persistence API.
- `AssociationStore` is the graph-serving API.
- Product query packages call `AssociationStore` for polymorphic associations and evidence tracing.
- Ingestion may use Ent directly inside transactions.
- Stable typed object reads may use Ent directly inside store/query helpers, but product code should not assemble graph traversals by hand.

### Iteration 2: Identity And Association Semantics

Questions:

- What makes `ticket:FLINK-39743` stable?
- How do aliases from Jira, GitHub, Slack, Drive, and email collapse to one person?
- Which reverse edges are materialized, and which are served by reverse indexes?
- What is the canonical dedupe key for an edge observed from multiple sources?

Corrections:

- Every object has `kind`, `key`, `source`, `external_id`, `source_url`, `observed_at`, `source_updated_at`, `visibility`, and `freshness_state` where applicable.
- Natural keys are source-neutral when possible: `ticket:FLINK-39743`, `pr:apache/flink-kubernetes-operator#1127`, `doc:gdrive:<file_id>`, `fragment:<doc_key>#<revision_or_hash>/<tab>/<ordinal>`.
- Source aliases map into canonical objects through explicit alias rows, not by mutating the canonical key.
- Reverse edges are not blindly duplicated. Materialize them only when the inverse predicate has product meaning or high query volume; otherwise query by `(to_kind, to_key, predicate)`.
- Edge dedupe uses `from_kind`, `from_key`, `predicate`, `to_kind`, `to_key`, `evidence_key`, and `rule_name`. Multiple evidence rows for the same logical relation are preserved unless a mapper explicitly rolls them up.

### Iteration 3: Document Search And RAG

Questions:

- What is the durable identity of a Google Docs fragment after edits?
- When does a summary become stale?
- What happens when vector search finds plausible text but the graph scope says the source is stale or invisible?
- Can search return "no evidence" rather than a generated answer?

Corrections:

- Fragments are retrieval/evidence units and carry revision, tab, heading path, ordinal, source URL, text hash, and visibility.
- Summary and embedding artifacts are invalid when their `source_content_hash` no longer matches the current fragment or revision.
- V0 search returns object and evidence hits only. No generated prose answer is part of V0.
- Future RAG must filter by graph scope and visibility before answer synthesis.
- A no-answer result is valid and should be tested.

### Iteration 4: Product Query Gates

Questions:

- Which three user questions prove the graph is useful?
- What evidence must each answer cite?
- What does the system do when one connector is stale or partial?
- How does Swift consume the answer without learning Ent or SQLite?

Corrections:

- The POC must pass three product queries before Swift integration:
  1. "Can project Atlas launch, and what blocks it?"
  2. "What is the full trace for ticket ATLAS-42?"
  3. "Where is the rollout decision documented, and what evidence supports it?"
- Every answer must include source status, evidence refs, freshness, confidence, and action candidates where applicable.
- Partial source failure is part of the answer, not a hidden backend warning.
- Swift consumes localhost JSON only and never writes `graph.db`.

## Goals

- Create a source-backed graph dataset using public Apache Flink data.
- Prove Go + Ent can model and traverse Cubicle's engineering execution graph.
- Keep ingestion replayable by storing raw source snapshots before normalization.
- Produce deterministic query outputs with source URLs, timestamps, and confidence.
- Keep Swift out of the first loop; the macOS app talks to the Go service only after graph answers are useful.
- Use the Flink import as validation after the synthetic workplace dataset in the Go graph POC plan.

## Non-Goals

- Full Slack connector.
- Private workspace ingestion.
- Auth, authorization, or enterprise permissions for the localhost POC.
- Vector search as the first milestone.
- LLM-only extraction as the source of truth.
- Production writeback to Jira, GitHub, Slack, or docs.
- A graph visualization product.

Read-only action candidates are in scope. Action writeback is out of scope.

## Selected Slice

Use the Apache Flink Kubernetes Operator **Autoscaler** area:

```text
Jira source
  JQL: project = FLINK AND component = "Autoscaler" AND updated >= "2023-01-01"

GitHub source
  primary repo: apache/flink-kubernetes-operator
  fallback repo: apache/flink only when Jira remote links point there
  PR seed search: repo:apache/flink-kubernetes-operator is:pr FLINK- updated:>=2025-06-01

Docs source
  repo tree paths under docs/content/docs/

Discussion source
  Apache Pony Mail for messages mentioning FLINK issue keys

Slack source
  Apache Flink Slack exists, but Slack API crawling is out of scope.
  Use the official public archive only as link/evidence metadata when already discovered.
```

The Autoscaler component is the right first real-world slice because it is narrow enough to crawl but dense enough to validate cross-source graph edges. Live probes on 2026-06-07 showed:

- `project = FLINK AND component = "Autoscaler" AND updated >= "2023-01-01"`: 156 Jira issues.
- `project = FLINK AND component = "Kubernetes Operator"`: 1009 Jira issues.
- `repo:apache/flink-kubernetes-operator is:pr FLINK- updated:>=2025-06-01`: 158 GitHub PRs.
- The 158 recent repo-wide PRs contain 110 distinct FLINK keys, but only 14 intersect the 156 Autoscaler Jira keys. GitHub discovery must therefore be Jira-key-driven after the cheap seed search.
- Wider GitHub probes increased Autoscaler overlap but also request cost: `updated >= 2025-01-01` found 21 intersecting keys, and `updated >= 2024-06-01` found 29 before unauthenticated search rate limiting stopped further probing.
- Jira remote-link probe over the first 60 Autoscaler issues found 40 GitHub PR remote links, all to `apache/flink-kubernetes-operator`.
- `docs/content/docs/**/*.md` in `apache/flink-kubernetes-operator`: 28 markdown docs.
- `project = FLINK AND component = "Autoscaler" AND updated >= "2024-01-01"`: 99 Jira issues.
- `project = FLINK AND component = "Autoscaler" AND updated >= "2025-06-01"`: 30 Jira issues.
- `project = FLINK AND component = "Autoscaler" AND updated >= "2023-01-01" AND status NOT IN (Closed, Resolved)`: 51 open issues.
- `project = FLINK AND component = "Autoscaler" AND updated >= "2023-01-01" AND labels = pull-request-available`: 101 issues.
- `project = FLINK AND component = "Autoscaler" AND updated >= "2023-01-01" AND assignee is EMPTY`: 48 issues.

Do not start with the whole Kubernetes Operator component. It is broad enough to hide mistakes in the mapper and query layer.

Switch slices only if the live import fails graph-density gates:

```text
abandon Autoscaler if:
  fewer than 10 on-slice ticket-to-PR traces after Jira remote links + targeted GitHub search
  fewer than 3 doc-supported traces at confidence >= 0.75
  fewer than 3 discussion-supported traces from Jira/PR/dev-mailing-list evidence
  FLINK-39743 cannot be reproduced as ticket -> PR -> changed file -> discussion/gap

next candidate:
  widen to Flink Kubernetes Operator component, still not all Flink
```

## Crawl Bounds And Acquisition Policy

Use a free-first, API-first crawl:

```text
required:
  Jira public REST API
  GitHub REST API or gh CLI backed by GitHub REST
  GitHub tree/raw content for docs

optional:
  Apache Pony Mail API by issue key

excluded from live crawl:
  Slack API
  paid scraping services
  browser scraping of protected/challenged archives
```

The POC has two crawl modes:

```text
fixture mode
  offline snapshots only
  no network
  deterministic tests

live mode
  explicit --live flag
  source budgets and limits required
  writes raw snapshots before mapping
```

Recommended live time bounds:

```text
Jira:
  2023-01-01T00:00:00Z through crawl time
  reason: component-bound window gives 156 Autoscaler issues; shorter windows are too sparse

GitHub PRs:
  seed search: updated >= 2025-06-01T00:00:00Z through crawl time
  targeted discovery: exact Autoscaler Jira issue-key searches with no date filter
  reason: repo-wide FLINK PR search has weak Autoscaler overlap; Jira-key-driven discovery stays on-slice

Docs:
  latest main branch tree, plus selected raw markdown snapshots
  reason: docs are small; current tree has 28 markdown files under docs/content/docs/

Pony Mail:
  issue-key targeted searches over the same Jira window
  reason: do not bulk-crawl full mailing lists; query only keys/components that already exist in Jira/GitHub
```

Rate and cost policy:

```text
Jira:
  public anonymous reads worked on 2026-06-07
  no charge
  no ASF-specific public rate header observed in successful responses
  Jira Data Center can return 429 when instance-level REST rate limiting is configured
  use maxResults=50 even though a live probe accepted larger pages, one request at a time, 1-2 requests/second max, honor 429 Retry-After

GitHub:
  public data is free
  unauthenticated core limit is 60/hour; live probe showed search bucket 10/minute from this IP
  authenticated token is free and raises core REST budget to 5,000/hour
  full PR detail crawl should require GITHUB_TOKEN; unauthenticated mode should cap PR detail fetches

Pony Mail:
  public API, no charge
  no explicit rate limit found in docs
  use issue-key targeted queries, 1 request/second max, cache every response

Slack:
  Apache Flink Slack exists and public-channel messages are published to linen.dev
  do not use Slack API for this public POC
  do not bulk copy Slack API data into the graph
  Slack API and export paths require explicit authorization, scopes, and sometimes workspace owner/admin capabilities
  Slack API policy risk is too high for a public third-party crawler; treat Slack as synthetic/user-provided only
```

Tooling:

- Prefer direct Go HTTP clients for production crawler behavior.
- Use `gh api --paginate` for manual GitHub probes and snapshot bootstrapping.
- Use `curl` only for one-off verification.
- Use `go-github` only if it reduces pagination/retry code; do not add it before the hand-written client becomes noisy.
- Use `slack-go/slack` only for a future user-provided workspace token/export path, not for Apache Flink Slack.

Twenty-five practical crawler validation questions:

1. Does the source expose an official API?
2. Is the API public, authenticated, or admin-only?
3. Is access free?
4. Is the crawl window dense enough to validate graph queries?
5. Is the crawl window small enough to replay quickly?
6. Does the source publish rate-limit headers?
7. What is the fallback when no rate-limit headers exist?
8. Can every request be replayed from a raw snapshot?
9. Can import resume from a cursor or page offset?
10. Can import rerun without duplicate objects?
11. Does source identity survive across re-fetches?
12. Does the source expose updated timestamps?
13. Can deleted or hidden content be detected?
14. Does the source include comments/reviews/messages or only objects?
15. Can the source link to other systems with exact keys?
16. Which fields are required for evidence?
17. Which source failures make the crawl partial instead of failed?
18. Is the source official enough to cite in product answers?
19. Does the source contain private or policy-sensitive content?
20. Are there terms that prohibit bulk export, indexing, or LLM usage?
21. Is the source better represented as facts, messages, docs, or evidence only?
22. Does the source need per-item backoff?
23. Can the crawler run unauthenticated in fixture mode?
24. What is the minimum live fetch that proves value?
25. What must be excluded until the user provides credentials or exports?

Acquisition review ledger:

| Iteration | Deep Question | Current Answer | Design Correction |
|---|---|---|---|
| 1 | Is Autoscaler still the right Flink slice? | Yes; 156 Jira issues since 2023-01-01 and 51 open issues. | Keep Autoscaler. Do not widen to all Kubernetes Operator. |
| 2 | Is a 6-12 month Jira window dense enough? | No; updated since 2025-06-01 gives only 30 Jira issues. | Use Jira since 2023-01-01. |
| 3 | Is repo-wide GitHub PR search on-slice? | Weakly; 158 recent PRs but only 14 Autoscaler key intersections. | Make GitHub discovery Jira-key-driven. |
| 4 | Can unauthenticated GitHub support full discovery? | No; search probing hit unauthenticated rate limits. | Require `GITHUB_TOKEN` for full live PR discovery/detail. |
| 5 | Can unauthenticated GitHub support fixture mode? | Yes for a few PR/detail fetches. | Allow capped unauthenticated probes. |
| 6 | Should GitHub PR details be fetched for every repo-wide FLINK PR? | No; too many off-slice PRs. | Fetch details only after Autoscaler key intersection. |
| 7 | Should Jira issue detail be fetched for all 156 issues? | Maybe, but search pages already include most fields. | Fetch full detail only for fixtures/changed items first. |
| 8 | Are Jira reads free? | Public anonymous reads worked. | Use public API with conservative serial backoff. |
| 9 | Does Jira expose rate headers? | No useful limit header observed in successful probe. | Use fixed conservative 1-2 rps and 429 backoff. |
| 10 | Are docs small enough for full snapshot? | Yes; 28 markdown files under docs/content/docs. | Snapshot full docs tree and selected/raw markdown. |
| 11 | Should docs use latest main only? | For V0 yes. | Store commit SHA/content hash; revision model handles later history. |
| 12 | Does Pony Mail have useful targeted results? | Yes; `issues@` returned 11 hits for FLINK-39743, `dev@` returned 1. | Query `issues` and `dev` by Jira key. |
| 13 | Should Pony Mail be required? | No; API is useful but secondary. | Optional source, source status can be partial/unavailable. |
| 14 | Should mailing lists be bulk-crawled? | No. | Query issue-key targets only. |
| 15 | Does Apache Flink have Slack? | Yes, official community page references Slack. | Acknowledge it but do not live crawl it. |
| 16 | Are Slack messages canonical for decisions? | No; Flink says important decisions must be reflected to mailing lists. | Treat mailing lists/Jira/GitHub as canonical evidence. |
| 17 | Can Slack API be used without admin/app/token constraints? | No. | Exclude Slack API from public POC. |
| 18 | Can linen.dev archive be scraped reliably? | No; curl received a Vercel challenge. | Do not build crawler around linen.dev scraping. |
| 19 | Can archive URLs be used as evidence links? | Yes when manually discovered or available through public pages. | Store links only; do not copy bodies from challenged pages. |
| 20 | Are paid tools needed? | No. | Free-first: Jira REST, GitHub REST/gh, GitHub raw/tree, Pony Mail. |
| 21 | Is `gh api` better than a Go client? | Good for manual probes, not production crawler. | Production uses Go HTTP client; `gh` for fixtures/probes. |
| 22 | Should `go-github` be added immediately? | Not necessary yet. | Add only if pagination/retry code gets noisy. |
| 23 | Should the crawler run live in tests? | No. | Tests use offline snapshots only. |
| 24 | What happens when GitHub token is missing? | Full crawl would exceed budgets. | Cap and mark GitHub partial. |
| 25 | What happens when a source is challenged or blocked? | Browser scraping would be brittle and ethically messy. | Mark source unavailable; continue with required sources. |
| 26 | Is the graph still useful without Slack? | Yes; Jira/GitHub/docs/Pony Mail cover tickets, PRs, docs, discussions. | Synthetic Slack validates workplace-message shape separately. |
| 27 | What source gives best decision evidence? | Docs, Jira comments, PR reviews, and dev mailing-list threads. | Search these before optional Slack-like data. |
| 28 | What minimum live fetch proves value? | Jira 156 issues, docs tree, FLINK-39743 PR detail, 25 Pony Mail key queries. | First live run can cap PR detail to 50. |
| 29 | What should query output disclose? | Source freshness, partial imports, evidence URLs, confidence. | Keep source status in every product answer. |
| 30 | What would invalidate this slice? | Too few cross-source traces after targeted PR search. | If fewer than 10 ticket->PR->doc/message traces, switch slice or widen to Kubernetes Operator. |
| 31 | Is `issues@flink.apache.org` human discussion or Jira mirror noise? | Mostly Jira mirror activity. | Use it for evidence/dedupe cautiously; prefer `dev@` for human discussion. |
| 32 | Do PR top-level comments come from the pull request comments endpoint? | No; they are GitHub issue comments. | Fetch both `issues/{number}/comments` and `pulls/{number}/comments`. |
| 33 | Should GitHub code search be used to find docs/code references? | No; code search has separate/tighter limits. | Use repo tree and raw content, not code search. |
| 34 | Can a normal Slack member export workspace history? | No; exports are owner/admin features by plan and scope. | User-provided Slack export only, never assumed for Flink. |
| 35 | Can Slack API data be indexed into a persistent public-data corpus? | Policy-sensitive and authorization-bound. | Exclude Slack API data from public POC storage. |
| 36 | Should `linen.dev` bodies be copied if visible in a browser? | No; access is challenged and not an official API. | Store archive URLs only when manually found. |
| 37 | What is the minimum Pony Mail API surface? | `stats.lua` for targeted search; `email.lua` only for selected message details. | Do not use `mbox.lua` in V0. |
| 38 | Should Jira search pages include comments? | Search can include comment fields, but full fidelity may need issue detail. | Search pages first; fetch issue detail for fixtures/changed issues. |
| 39 | Can GitHub Search discover all historical PR links cheaply? | No; key-targeted search consumes budget. | Full discovery requires token and budget cap. |
| 40 | What should happen if GitHub secondary limits appear? | Stop source, mark partial, retain snapshots already fetched. | Never spin retries aggressively. |
| 41 | What if GitHub Search misses PR links that Jira already knows? | Jira remote links are anonymously accessible and directly point FLINK-39743 to PR #1127. | Fetch Jira remote links and treat GitHub PR URLs there as high-confidence links. |
| 42 | Should remote links be fetched for all Autoscaler issues? | Yes for the first live run; 156 requests is acceptable with conservative Jira throttling. | First run fetches all remote links, incremental runs fetch changed issues only. |
| 43 | How do we avoid double-counting Jira and `issues@` emails? | `issues@` mostly mirrors Jira activity. | Mark as `jira_mirror_email` and dedupe by issue key/event/time/body hash. |
| 44 | What if Autoscaler PRs live outside `apache/flink-kubernetes-operator`? | Possible for some linked work. | Primary repo stays operator; add fallback repos only when Jira remote links prove them. |
| 45 | What if a PR has huge file/comment/review pages? | GitHub detail can exceed practical budgets. | Add per-PR page/file caps and mark detail partial for oversized PRs. |
| 46 | What links docs to tickets if there is no exact FLINK ID? | File references and component/path mapping can help, but weaker. | Add doc-link confidence rules; only confidence >= 0.75 supports V0 answers. |
| 47 | What proves the slice is dense enough? | Raw counts are insufficient. | Add cross-source graph-density gates: ticket->PR, ticket->PR->file, ticket->PR->doc, ticket->discussion. |
| 48 | What if source data changes during pagination? | Live totals can drift. | Use `crawl_started_at` watermark and replay snapshot counts, not later live totals. |
| 49 | What if a public item disappears or a remote link is removed? | Deleting immediately would erase historical evidence. | Create tombstone events and mark facts stale unless another source still supports them. |
| 50 | When do we abandon Autoscaler? | If graph-density gates fail. | Widen only to Kubernetes Operator component, not all Flink. |
| 51 | What if Slack ingestion is later added and the app only has channel history access, not workspace export access? | Slack channel APIs require app scopes, membership/visibility, and are rate limited; normal users cannot assume export rights. | Keep Slack as a user-provided export or authorized connector only; model `Conversation`, `Message`, `Thread`, `Reaction`, and `SourcePermission` without assuming complete workspace history. |
| 52 | What if Slack `conversations.history` becomes the bottleneck for all message sync? | Slack documents restrictive limits for some non-Marketplace apps, including low request rate and small page size. | Do not design Slack around full backfills; use incremental, per-channel cursors and partial-source state, and reject unbounded history sync. |
| 53 | What if Slack threads are missed because only channel history is indexed? | Replies are separate from top-level messages in the API/export shape. | Treat thread replies as first-class messages with `thread_root_key` and query-facing evidence only after the thread is complete enough to cite. |
| 54 | What if Slack edits/deletes or retention policies remove context after we indexed it? | Workspace retention and export/API visibility can change. | Store source event tombstones, `edited_at`, `deleted_at`, and `freshness_state`; answers must show stale or missing-message evidence. |
| 55 | What if Google Docs export fails for large docs? | Drive `files.export` has a documented 10 MB exported-content limit. | Prefer Docs API structural reads for Google Docs; use export only for small formats or fixtures and mark oversized docs partial. |
| 56 | What if Google Docs tabs are ignored and fragments collide? | Docs now have tab-aware structure. | Make `DocumentTab` mandatory for Google Docs; fragment keys include document ID, revision/hash, tab ID, structural path, and ordinal. |
| 57 | What if Drive permissions change but old fragments stay visible in search? | Search correctness depends on visibility, not just content. | Search APIs must filter by `SourcePermission` and document revision freshness; permission changes enqueue reindex/tombstone events. |
| 58 | What if Drive change tokens expire or become invalid? | Change feeds are stateful and can require a new start token after invalidation. | `ConnectorState` stores change tokens, invalid-token recovery mode, and a full-rescan-required flag. |
| 59 | What if Google Docs comments/suggestions are the real decision evidence? | Document body alone can miss review/decision context. | Reserve source object types for `DocComment`, `DocSuggestion`, and `DocResolvedThread`; V0 may omit them, but schema should not block them. |
| 60 | What if Jira search omits changelog detail needed to explain ownership/status transitions? | Search pages are not a substitute for issue history. | Add `IssueEvent`/`SourceEvent` rows for status, assignee, labels, and link changes; fetch changelog only for selected tickets or fixture keys. |
| 61 | What if Jira or GitHub returns 429/secondary-limit responses while a required source is mid-run? | Retrying aggressively risks lockout and duplicate snapshots. | Persist page-level snapshots before normalization, honor `Retry-After`, stop the source, and mark the crawl partial with resume cursor. |
| 62 | What if GitHub Search silently hides older matches behind the 1,000-result search cap? | Search APIs are discovery aids, not complete history stores. | Use Jira remote links and exact key searches as primary; store search totals as diagnostic evidence, never completeness proof. |
| 63 | What if GitHub PR review comments and top-level comments disagree? | They live on different endpoints and express different evidence. | Keep `Review`, `ReviewComment`, and `IssueComment` distinct; query layer can merge them only by PR and timestamp. |
| 64 | What if Ent generated traversals tempt us into source-specific query code everywhere? | Ent codegen is powerful, but product queries need stable graph-serving semantics. | Enforce that query packages use `AssociationStore` for association lists/expansion; direct Ent calls are allowed for typed object load only. |
| 65 | What if Ent auto migration hides schema drift until data is already written? | Automatic migration is not enough for production-like graph evolution. | POC can use auto migration; plan for versioned Atlas migrations before any shared database or long-lived user data. |
| 66 | What if association edges need metadata but Ent M2M edges are too thin? | Native relation edges do not encode all Cubicle evidence/provenance semantics. | Keep `GraphEdge` as an explicit entity with association type, confidence, source refs, valid time, observed time, stale state, and sort key. |
| 67 | What if Ent privacy is ignored because localhost has no auth? | Localhost POC still needs future permission shape. | Add permission metadata now; later Ent/privacy or service-layer guards can enforce it without schema replacement. |
| 68 | What if SQLite write contention appears during ingestion plus serving? | SQLite WAL improves read/write overlap, but SQLite still has serialized writes. | Use a single writer queue for ingestion, short transactions, WAL mode, busy timeout, and read-only query transactions. |
| 69 | What if WAL files grow because readers pin checkpoints? | Long reads can delay checkpointing. | Keep graph queries bounded, expose query timeouts, and add checkpoint/DB-size metrics to the service. |
| 70 | What if the SQLite DB lives on a network filesystem or cloud-synced folder? | SQLite WAL and locking semantics are sensitive to filesystem behavior. | Keep `.data` on local disk for V0 and document that network/cloud-synced paths are unsupported. |
| 71 | What if FTS rows drift from document fragments after updates/deletes? | FTS5 external-content setups need explicit consistency. | Store `SearchIndexState` per owner and test insert/update/delete reindex paths; no answer may use stale FTS rows. |
| 72 | What if whole-document embeddings look good in demos but fail evidence retrieval? | Whole-doc vectors blur decisions, tickets, and caveats. | Continue storing fragments as retrieval units; embeddings attach to fragments or summaries, never replace fragment evidence. |
| 73 | What if lexical search misses synonyms while vector search hallucinates relevance? | Each retrieval mode fails differently. | V0 search returns exact object, lexical, and graph-neighborhood hits separately; future hybrid ranking must expose why each hit matched. |
| 74 | What if Palantir-style ontology modeling turns every noun into an object type? | Over-modeling creates brittle schemas and low-value graph noise. | Object types must pass a query-gate: they support readiness, trace, blocker, owner, decision, or action-candidate queries. |
| 75 | What if action candidates sprawl into unsafe workflow automation? | Palantir-style actions are valuable only when grounded and constrained. | V0 action candidates stay read-only, deterministic, evidence-backed, and carry required confirmation/writeback metadata for later. |
| 76 | What if Glean-style permission-aware search is copied only superficially? | Enterprise search depends on identity, groups, permissions, freshness, and activity context. | Every query-facing object stores visibility and source freshness; ranking can use activity later but permissions are mandatory now. |
| 77 | What if identity resolution merges the wrong Jira reporter, GitHub login, Slack user, or email? | False merges corrupt ownership and accountability. | Use alias rows with source-specific confidence; never rewrite canonical person keys without evidence and manual/testable merge rules. |
| 78 | What if attachments and generated files hold the real evidence? | Tickets, PRs, and docs may link PDFs, screenshots, logs, or build artifacts. | Add `Attachment` as a future source object with hash, MIME type, size, source URL, and extraction state; do not ingest arbitrary binaries in V0. |
| 79 | What if graph scale outgrows local SQLite but query code assumes SQLite-specific behavior? | SQLite is right for local POC, not guaranteed for multi-user production. | Keep storage behind Ent plus service APIs; avoid SQLite-only query semantics outside search/index modules and define a Postgres promotion trigger. |
| 80 | What if Swift starts depending on graph internals before the backend stabilizes? | Direct DB/Ent coupling would freeze the wrong contract. | Swift consumes localhost HTTP query DTOs only after product gates pass; DB schema, Ent structs, and FTS internals remain backend-private. |

New-direction corrections from iterations 51-80:

- Treat connectors as state machines, not one-shot crawlers. Every connector stores cursor, freshness, partial state, tombstones, and source-specific limits.
- Treat permissions as query inputs from the first schema pass. Localhost skips auth, but search and graph answers still carry visibility and source permission metadata.
- Treat search as three lanes: exact object lookup, lexical evidence search, and graph-neighborhood traversal. Vector search is future hybrid ranking, not the graph substrate.
- Treat SQLite as a local graph appliance. Use WAL, short write transactions, bounded reads, and local disk only; promote to Postgres when concurrent writers, remote clients, or database-size comfort limits appear.
- Treat ontology growth as query-driven. New object/link types must answer a product question or preserve required evidence/provenance.

Implementation refinement ledger:

| Iteration | Deep Question | Current Answer | Design Correction |
|---|---|---|---|
| 81 | What if Slack Events API retries or duplicate deliveries create duplicate graph facts? | Webhook systems retry and redeliver. | Add `ProviderEvent` idempotency keyed by source, provider event ID, event timestamp, and normalized payload hash. |
| 82 | What if Slack sends `app_rate_limited` or an API sync exceeds limits? | Message sync can have source-specific rate ceilings and gaps. | Connector state records `gap_started_at`, `gap_ended_at`, source status `partial`, and a required reconciliation cursor. |
| 83 | What if Slack replies are not fetched with channel history? | Thread replies require separate handling. | Store `thread_root_key`, `reply_count`, and `thread_freshness_state`; do not cite incomplete threads as complete discussion evidence. |
| 84 | What if Slack messages contain blocks, files, mentions, user groups, or bot IDs? | Plain text alone loses evidence and identity. | Store raw blocks/files as snapshots, normalize mentions into `Mention` edges, and allow `ActorAlias` to point to people, bots, or groups. |
| 85 | What if Slack channels are renamed, archived, or shared across workspaces? | Conversation identity is not just channel name. | Key conversations by provider channel ID; names become alias/version events. |
| 86 | What if Google Drive push notification channels expire? | Push channels are leases, not durable sync state. | Treat Drive Changes tokens as the durable cursor; push notifications only wake a changes pull. |
| 87 | What if Drive `files.list` over all drives reports incomplete search? | Broad corpus search can be incomplete. | Crawl explicit corpora: user drive plus known shared drives; persist `incomplete_search=true` as source partial state. |
| 88 | What if Google Docs comments and replies live in Drive APIs, not Docs body reads? | Decision context may be outside the document body. | Add future `DocComment`/`DocReply` source object support through Drive comments/replies, separate from body fragments. |
| 89 | What if Docs structural indexes shift after edits? | Start/end indexes are not stable durable IDs. | Fragment identity uses document ID, revision/hash, tab ID, structural path, ordinal, and content hash. |
| 90 | What if a Google Doc export is too large or omits structural metadata? | Export is limited and less structured than Docs API reads. | Use Docs API structural reads for Google Docs; exports are fixture/preview fallback only. |
| 91 | What if Jira webhooks or incremental events arrive out of order? | Source events can be delayed or duplicated. | Normalize by source event time and observed time; graph facts carry valid-time and observed-time separately. |
| 92 | What if Jira issue security makes a ticket invisible rather than missing? | 404/permission failures can mean hidden, not deleted. | Store source error kind `not_found`, `forbidden`, or `hidden_or_unavailable`; do not tombstone on ambiguous permission failures. |
| 93 | What if Jira changelog is too expensive to fetch for every issue? | Changelog pagination can dominate crawl cost. | Fetch changelog lazily for fixture issues, changed issues, and trace-query candidates; store `history_freshness_state`. |
| 94 | What if Jira status/assignee transitions are needed for blocker analysis? | Current fields only show latest state. | Add `IssueEvent` rows for status, assignee, label, link, and priority transitions. |
| 95 | What if GitHub webhooks are missed while the service is offline? | Webhooks are not a complete historical source by themselves. | Use webhooks only as hints; scheduled reconciliation still pulls REST snapshots by issue key/PR number. |
| 96 | What if GitHub Search caps or incomplete results hide matches? | Search is not completeness proof. | Partition by known keys/date windows and prefer Jira remote links; store search result counts as diagnostics only. |
| 97 | What if a PR force-push removes commits or files that were once evidence? | PR state is mutable. | Store commit/file snapshots by crawl run and mark previous commit/file evidence stale when current PR state no longer contains it. |
| 98 | What if GitHub review-thread resolution needs GraphQL, not REST? | Some review metadata is easier through GraphQL. | Keep a future `github_graphql` source client seam, but V0 uses REST comments/reviews and records unresolved resolution state as unknown. |
| 99 | Should the Go HTTP service use a bare `net/http` router, `chi`, Gin, or Huma? | Bare `net/http` keeps dependencies low but gives weak API-contract ergonomics for Swift. Gin is a popular Go web framework; Huma adds typed REST/OpenAPI on top of routers. | Use Gin as the V0 server framework and Huma's Gin adapter for typed DTOs, validation, OpenAPI 3.1 generation, generated docs, and Swift client generation. |
| 100 | What if SQLite connection pooling causes write-lock churn? | SQLite concurrency depends on connection and transaction behavior. | Use a single writer path, explicit transaction helper, busy timeout, WAL mode, and bounded read-only queries. |
| 101 | What if a request is cancelled during an Ent transaction? | Context cancellation must roll back cleanly. | All transaction helpers accept context, map Ent rows to DTOs before returning, and never leak open transactional entities. |
| 102 | What if Ent auto migration succeeds locally but production migration needs review? | Auto migration is convenient but weak as a long-lived DB contract. | Use auto migration only in early POC; add Atlas versioned migrations before any user-shared persistent DB. |
| 103 | What if Ent privacy hooks are added later but query code bypasses them? | Direct Ent queries can bypass intended graph semantics. | Keep permission filtering in service/query layer now; future Ent privacy is additive, not the only enforcement point. |
| 104 | What if FTS5 external-content tables drift from owner rows? | External-content FTS requires careful trigger/update discipline. | V0 uses an explicit FTS index table plus `SearchIndexState`; every search result is revalidated against owner freshness and visibility. |
| 105 | What if FTS search returns stale hidden evidence after permissions change? | Search index rows can outlive permission state. | Permission changes mark affected owners `needs_reindex`; result assembly drops stale or invisible owner rows. |
| 106 | What if hybrid/vector search ranks plausible but unsupported content first? | Vector similarity is not evidence. | Query response separates exact, lexical, graph, and future vector lanes; answer generation requires cited fragment/source evidence. |
| 107 | What if RAG generation hides retrieval failure? | Generated text can mask missing evidence. | V0 has no generated prose answers; future RAG must expose retrieved evidence, no-answer behavior, and retrieval metrics. |
| 108 | What if high-degree nodes such as `project:flink` make traversal unusable? | Graph expansion can explode. | `AssociationStore.Expand` requires depth, per-node limit, cursor, predicate filter, and visited-edge dedupe. |
| 109 | What if reverse edges diverge from forward edges? | Materialized inverses can get inconsistent. | Reverse edges are stored with `derived_from_edge_key` and created in the same transaction as the forward edge. |
| 110 | What if idempotent replay maps the same source snapshot twice? | Fixture/live replays must be repeatable. | Add unique constraints on `source_key + external_id + version`, `provider_event_key`, and mapper version/hash. |
| 111 | What if source snapshots are too large to diff or replay? | Raw pages can grow quickly. | Store snapshots content-addressed with hash, source URL, request params, response headers, and optional gzip file pointer. |
| 112 | What if mapper logic changes after snapshots are stored? | Same raw data can map differently. | Store `mapper_version` on normalized objects/edges and support reindex/remap from snapshots. |
| 113 | What if source status is hidden from users? | Partial answers become misleading. | Add `/v1/sources` and include source status summaries in query responses. |
| 114 | What if tests only cover happy-path snapshots? | Edge cases will ship as schema bugs. | Fixture sets must include duplicate events, deleted objects, permission-hidden objects, stale FTS rows, invalid cursors, and rate-limit responses. |
| 115 | What if attachments contain the actual decision evidence? | Text sources miss screenshots, PDFs, and logs. | Add `Attachment` metadata shape now; V0 indexes only whitelisted text attachments or stores link/hash/extraction state. |
| 116 | What if we copy SQLite files while WAL has uncheckpointed data? | File copy can produce inconsistent backups. | Use SQLite backup API or controlled checkpoint before backup; snapshots remain separate source-of-truth artifacts. |
| 117 | What if Swift needs a stable contract before the backend schema settles? | Schema churn should not affect UI. | Define versioned HTTP DTOs and keep Ent structs, SQLite paths, FTS tables, and source snapshots backend-private. |
| 118 | What if local database paths land in iCloud/Dropbox synced folders? | SQLite locking/WAL can behave badly on synced/networked paths. | Default data root stays under local app support/workspace `.data`; warn/refuse known cloud-synced paths for live mode. |
| 119 | What if observability is missing during crawl failures? | Debugging sync issues needs source/run/page context. | Every log line carries `crawl_run_id`, source, snapshot key, cursor, request ID, and partial/failure reason using `slog`. |
| 120 | What if the POC cannot prove its implementation details are correct? | Design without tests will drift. | Add a validation matrix mapping each invariant to a fixture, unit test, integration test, or query golden response. |

Implementation details validated from this pass:

- Use Gin plus Huma for V0 HTTP. Gin provides the familiar server framework shape: route groups, middleware, recovery, and localhost binding. Huma provides the typed API contract: request/response DTOs, validation, OpenAPI 3.1 generation, generated docs, and a path to Swift OpenAPI Generator.
- Source ingestion must be idempotent. The minimum durable keys are provider event ID where available, source external ID, source version/revision, raw snapshot hash, and mapper version.
- Snapshot replay must be separable from live crawling. Live fetch writes snapshots first; mappers read snapshots and can be rerun.
- Search must revalidate every hit against owner freshness and visibility. The FTS table is an acceleration structure, not authority.
- Graph edges must be valid-time aware. Use both source event time and observed crawl time to answer "what changed" and "what do we know now?"
- SQLite must be configured deliberately: local path, WAL, busy timeout, bounded transactions, single writer queue/path, and a backup/checkpoint strategy.
- The Swift contract must be versioned HTTP DTOs generated from the Huma OpenAPI document. The database and Ent schema are implementation details until the graph stabilizes.

Second implementation refinement ledger:

| Iteration | Deep Question | Current Answer | Design Correction |
|---|---|---|---|
| 121 | What if source user IDs are only unique inside a Slack team, GitHub instance, or Jira site? | Provider IDs are scoped. | Alias keys include provider, tenant/workspace/site, and external user ID; canonical person merge is separate and confidence-scored. |
| 122 | What if GitHub logins or Slack channel names are renamed? | Names are mutable display aliases. | Use stable provider IDs when available; names become alias/version events. |
| 123 | What if issue keys, PR refs, and file paths tokenize badly in FTS? | Default tokenization can split punctuation-heavy engineering identifiers. | Configure and test FTS tokenization for `ATLAS-42`, `FLINK-39743`, `apache/repo#1127`, dotted packages, and file paths. |
| 124 | What if snapshots contain private or sensitive data later? | Raw snapshots are replay gold but privacy risk. | Keep snapshots local, backend-private, and content-addressed; add redaction hooks before any shared/exported snapshot workflow. |
| 125 | What if a source page is fetched with incomplete fields because API field masks were too narrow? | Field masks affect replay completeness. | Store requested field masks/params with every snapshot; mapper tests assert required fields exist. |
| 126 | What if Drive shared-drive APIs need explicit `supportsAllDrives`/corpora settings? | Broad file listing can miss shared-drive content. | Future Drive connector stores corpus settings and shared-drive IDs in connector state. |
| 127 | What if Google Drive marks a change as removed without returning a full file object? | Deletion events can be sparse. | Deletion/missing object snapshots create tombstones by file ID and preserve last known metadata. |
| 128 | What if GitHub PR head/base refs move between crawl pages? | PR branches and commits are mutable. | Store PR head/base SHA, merge commit SHA, and crawl run; mark previous commit/file evidence stale when PR detail changes. |
| 129 | What if GitHub users are bots or deleted accounts? | Ownership logic must not assume human users. | `ActorAlias` supports person, bot, app, and unknown actor kinds. |
| 130 | What if Jira issue links point to external systems or deleted issues? | Links are heterogeneous. | Store external links as `ExternalRef`/evidence before turning them into typed graph edges. |
| 131 | What if a document summary conflicts with source fragments? | Summaries are derived and can be wrong. | Query-facing answers cite fragments, not summaries; summaries help ranking/display only. |
| 132 | What if embedding model or dimensions change? | Embeddings are model-versioned artifacts. | `EmbeddingArtifact` includes model, dimensions, content hash, generated_at, and status; mismatches force regeneration. |
| 133 | What if source events have event time earlier than the crawl time? | Engineering questions need both timelines. | Keep `source_event_time` and `observed_at` on events/edges/evidence. |
| 134 | What if source snapshots and DB writes are not atomic together? | A crash can leave snapshot without normalized rows or vice versa. | Treat snapshots as source of truth; mapper writes normalized rows in a DB transaction with snapshot key and mapper version. |
| 135 | What if the mapper crashes midway through a run? | Partial normalization must be resumable. | `ConnectorState` and `ImportCheckpoint` store page cursor, mapper cursor, and partial status. |
| 136 | What if a graph edge is supported by multiple evidence items? | One evidence key can understate confidence. | Allow multiple evidence refs per fact through either duplicate edge rows by evidence or an `EdgeEvidence` join in later versions; V0 duplicate-by-evidence is acceptable. |
| 137 | What if user asks "why did this change?" rather than "what is current?" | Latest-state tables are insufficient. | Preserve `SourceEvent`/`IssueEvent` history and valid-time edges for trace queries. |
| 138 | What if query latency grows from unbounded evidence loading? | Evidence fan-out can be large. | Query APIs cap evidence per edge and provide evidence cursor/detail endpoint. |
| 139 | What if source-specific code leaks into product queries? | It makes Swift/API behavior hard to stabilize. | Product queries use domain DTOs, `AssociationStore`, and source-neutral stores; source packages only fetch/map snapshots. |
| 140 | What if "it works locally" hides missing implementation proof? | Local happy paths are weak. | The plan now includes a validation matrix covering idempotency, 429s, invalid cursors, stale FTS, hidden permissions, thread replies, high-degree expansion, and SQLite PRAGMAs. |

Code-level implications:

- Add source-scoped identity keys: `source + tenant/site/workspace + external_id`.
- Add `ActorAlias` support for people, bots, apps, groups, and unknown actors.
- Store field masks/request params with source snapshots.
- Index engineering identifiers deliberately; test punctuation-heavy query terms.
- Keep summaries and embeddings subordinate to fragment/evidence retrieval.
- Expose evidence pagination so product traces do not overload `/v1/*` responses.

## Concrete Proof Record

Use `FLINK-39743` and GitHub PR `apache/flink-kubernetes-operator#1127` as the first end-to-end fixture:

```text
FLINK-39743
  summary: Incorrect Expected Processing Rate Computation
  status: Open
  type: Bug
  priority: Minor
  reporter: Devika Sudheer
  assignee: null
  components: Autoscaler, Kubernetes Operator
  labels: pull-request-available
  comment_count: 2

PR #1127
  title: [FLINK-39743] [flink-autoscaler] Fix incorrect expected processing rate computation
  state: open
  author: devu1997
  created_at: 2026-06-02T07:09:08Z
  updated_at: 2026-06-06T21:04:47Z
  additions: 62
  deletions: 5
  changed_files: 3
```

The PR changed these files:

```text
flink-autoscaler/src/main/java/org/apache/flink/autoscaler/JobVertexScaler.java
flink-autoscaler/src/test/java/org/apache/flink/autoscaler/JobVertexScalerTest.java
flink-autoscaler/src/test/java/org/apache/flink/autoscaler/ScalingExecutorTest.java
```

That gives the first deterministic chain:

```text
Ticket FLINK-39743
  -> has_component Autoscaler
  -> implemented_by PR #1127
  -> changed_file JobVertexScaler.java
  -> discussed_in PR review comment
  -> has_gap assignee_missing
```

## Benchmark Lessons

### Glean

Glean's public graph model is the closest benchmark for Cubicle's workplace graph. Its knowledge graph is organized around content, people, and activity. Its newer Enterprise Graph and Personal Graph frame higher-order work context: projects, teams, processes, docs, tickets, feature specs, support cases, and user activity.

Cubicle should copy these patterns:

- Connect content, people, and activity instead of building text-only RAG.
- Treat connectors as producers of content, metadata, permissions, activity, and freshness.
- Put timestamps, source, confidence, visibility, and provenance on graph edges.
- Use exact lexical lookup for tickets, PR IDs, paths, errors, and symbols.
- Add semantic retrieval later for fuzzy intent, after typed graph queries work.
- Make stale connectors and partial source failure visible.

Cubicle should avoid these failure modes:

- Indexing everything before proving engineering execution queries.
- Opaque ML-inferred graph facts.
- Personal-graph behavior that feels like surveillance.
- Search answers that cannot cite source artifacts.

### Palantir And Oncology

Palantir's useful lesson is not "make a big graph." It is ontology as an operational layer: real-world objects, properties, links, actions, functions, dynamic security, and decision workflows over raw data. In life sciences and oncology, the public examples emphasize harmonized clinical trial and research data, provenance, governance, and collaboration across specialized domains.

Cubicle should copy these patterns:

- Model reality, not source systems: `Person`, `Project`, `Ticket`, `PullRequest`, `Document`, `Decision`, `Blocker`, `Risk`.
- Keep object types focused.
- Expose workflow actions only after read-only graph proof works: ask owner, record decision, mark stale, open follow-up.
- Preserve provenance so every insight can be traced to raw source data.
- Keep technical ingestion metadata out of user-facing objects.

Cubicle should avoid these failure modes:

- A heavyweight ontology before workflows prove value.
- Source-system silos such as `JiraPerson`, `GitHubPerson`, and `MailPerson`.
- A god object that stores every entity type in one table.
- Action sprawl before the POC has trusted read queries.

## Source Contracts

### Jira

Base endpoint:

```text
https://issues.apache.org/jira/rest/api/2/search
```

Primary search:

```text
jql=project = FLINK AND component = "Autoscaler" AND updated >= "2023-01-01"
startAt=<page offset>
maxResults=50
fields=key,summary,status,issuetype,priority,assignee,reporter,created,updated,components,labels,fixVersions,issuelinks,description,comment
```

Importer behavior:

- Page through `startAt` until all `total` results are captured.
- Store every search page as a raw snapshot.
- Fetch individual issue detail for issues selected as fixtures or changed since the last run.
- Fetch Jira remote links for all imported Autoscaler tickets in the first live run, then only changed tickets in incremental runs:
  `GET /rest/api/2/issue/{issueKey}/remotelink`.
- Normalize comments as messages.
- Normalize `components`, `labels`, and `issuelinks` into graph edges.
- Normalize remote GitHub PR links as high-confidence `ticket implemented_by pull_request` edges before GitHub Search.
- Treat missing assignee as a graph fact, not an import failure.
- Use anonymous public reads only.
- Budget: 4 search-page requests for the 156-issue Autoscaler window at `maxResults=50`, up to 156 remote-link requests in the first live run, plus per-issue detail only for fixture tickets or changed tickets.
- Backoff: serial requests, 1-2 requests/second, honor 429/5xx with exponential backoff and `Retry-After` when present.
- Cost: free public API reads.

### GitHub

Base repository:

```text
https://api.github.com/repos/apache/flink-kubernetes-operator
```

Repository policy:

```text
primary: apache/flink-kubernetes-operator
fallback: apache/flink only when Jira remote links or exact issue-key search prove an Autoscaler-linked PR lives there
```

Do not crawl all Flink repositories. Add a repository only when an Autoscaler Jira key produces direct evidence for that repository.

Primary search:

```text
https://api.github.com/search/issues?q=repo:apache/flink-kubernetes-operator is:pr FLINK- updated:>=2025-06-01
```

Manual probe equivalent:

```bash
gh api --paginate 'search/issues?q=repo:apache/flink-kubernetes-operator+is:pr+FLINK-+updated:>=2025-06-01&per_page=100'
```

Targeted Jira-key search:

```text
https://api.github.com/search/issues?q=repo:apache/flink-kubernetes-operator is:pr FLINK-39743
```

GitHub discovery order:

1. Import Jira remote links for fixture/changed Autoscaler issues.
2. Run the cheap seed search to find recent FLINK-linked PRs.
3. Extract FLINK keys and intersect with Jira Autoscaler keys.
4. For Autoscaler Jira keys not covered by Jira remote links or the seed search, run exact per-key PR searches without an `updated` filter until the `pr_detail_limit` is satisfied.
5. Fetch PR details only for PRs linked to Autoscaler Jira keys.
6. Store off-slice seed search pages as raw snapshots, but do not normalize off-slice PR details into the graph.

For each PR:

```text
GET /repos/apache/flink-kubernetes-operator/pulls/{number}
GET /repos/apache/flink-kubernetes-operator/pulls/{number}/files?per_page=100&page=<n>
GET /repos/apache/flink-kubernetes-operator/issues/{number}/comments
GET /repos/apache/flink-kubernetes-operator/pulls/{number}/comments
GET /repos/apache/flink-kubernetes-operator/pulls/{number}/reviews
GET /repos/apache/flink-kubernetes-operator/pulls/{number}/commits
```

Importer behavior:

- Extract `FLINK-\d+` from PR title and body.
- Link PRs to matching Jira tickets.
- Normalize PR author and reviewers as people.
- Normalize changed files as documents or code artifacts.
- Store top-level PR conversation comments from `issues/{number}/comments`.
- Store PR review comments from `pulls/{number}/comments` with file path and diff hunk metadata when available.
- Store PR reviews from `pulls/{number}/reviews` as review-state events.
- Support `GITHUB_TOKEN` for higher rate limits.
- Require `GITHUB_TOKEN` for full live PR-detail crawl because unauthenticated public REST has a 60/hour core limit and live probe showed a 10/minute search bucket.
- Allow unauthenticated mode only for fixture fetches and capped probes.
- Budget: seed search uses 2-4 Search API requests depending on time window; Jira remote links reduce targeted key-search needs; targeted key discovery can use up to 156 Search API requests for the full Autoscaler Jira set in the worst case; each selected PR can use up to 5 core REST detail requests.
- Full live discovery requires a free `GITHUB_TOKEN` because authenticated Search API limits are materially higher than unauthenticated search, and core REST budget rises from 60/hour to 5,000/hour.
- Backoff: honor `x-ratelimit-*`, `retry-after`, 403/429, and GitHub secondary rate-limit messages. Keep concurrency at 1 for V0.
- Cost: free public API reads; token is free.
- Avoid GitHub code search in V0. Use the repo tree and raw content for docs/code references.
- Per-PR caps: fetch all pages for files/comments/reviews only while the PR stays under `max_pr_detail_pages` and `max_changed_files`. If a PR is too large, store the PR summary, mark detail as `partial`, and skip low-value pages first: commits, then reviews, then review comments.

### Docs

Use the GitHub tree API:

```text
GET /repos/apache/flink-kubernetes-operator/git/trees/main?recursive=1
```

Importer behavior:

- Select markdown files under `docs/content/docs/`.
- Store `path`, `sha`, `size`, raw URL, and content hash.
- Fetch raw markdown for docs likely to explain Autoscaler behavior, starting with:
  - `docs/content/docs/custom-resource/autoscaler.md`
  - `docs/content/docs/operations/plugins.md`
  - `docs/content/docs/development/roadmap.md`
- Link docs to tickets and PRs through exact FLINK IDs, file paths, component names, and explicit mentions.
- Budget: one tree request plus up to 28 raw markdown fetches for a full docs snapshot.
- Cost: free through GitHub public REST/raw content.

Document linking confidence:

```text
1.00 exact FLINK issue key mention in doc
0.95 exact PR URL/number mention in doc
0.90 PR changed file path is explicitly referenced by doc
0.75 doc path/component mapping: docs path contains autoscaler and ticket component is Autoscaler
0.60 keyword-only mention such as "autoscaler" without ticket/PR/file evidence
```

Only links with confidence >= 0.75 are query-facing in V0. Keyword-only links can be stored as candidates but must not support product answers alone.

### Apache Pony Mail

Base docs:

```text
https://ponymail.apache.org/docs/api.html
```

Probe shape:

```text
GET https://lists.apache.org/api/stats.lua?list=dev&domain=flink.apache.org&q=FLINK-39743&emailsOnly=true&d=dfr=2025-06-01,dto=2026-06-07
GET https://lists.apache.org/api/stats.lua?list=issues&domain=flink.apache.org&q=FLINK-39743&emailsOnly=true&d=dfr=2025-06-01,dto=2026-06-07
GET https://lists.apache.org/api/email.lua?id=<message-id>
```

Importer behavior:

- Query Pony Mail by issue key after Jira and GitHub import produce the key set.
- Store subject, list, author metadata when available, timestamp, permalink or message ID, and body snippet when accessible.
- Treat Pony Mail as optional in the first crawler because API behavior is less predictable than Jira and GitHub.
- Prefer `dev@flink.apache.org` for human development discussions.
- Use `issues@flink.apache.org` for Jira mirror evidence and dedupe, not as independent human discussion.
- Dedupe Jira issue activity against `issues@flink.apache.org` by issue key, event type, source timestamp, subject, and body hash.
- Mark `issues@` mirror messages as `source_kind=jira_mirror_email` so query answers do not count them as independent human discussion.
- Do not bulk-crawl entire mailing lists.
- Budget: targeted queries for fixture tickets first; cap to 25 issue keys in the first live run.
- Cost: free public API.
- Backoff: 1 request/second, cache every response, mark source partial on 429/5xx.
- Do not use `mbox.lua` in V0; monthly mbox export is too broad for the first slice.

### Slack-Like Data

Apache Flink has a Slack community, and Flink's official community page says public Slack messages are permanently stored and published in the Apache Flink Slack archive on linen.dev. The same page says important decisions and conclusions must be reflected back to mailing lists.

Do not use Slack API or scrape Slack/public archive pages for the first Flink import. If Slack data is needed for product validation, use either:

- Synthetic Slack-style JSON in the synthetic dataset.
- A user-provided Slack export zip for local parsing.
- Public archive URLs manually attached as evidence links, without copying message bodies.

Slack export JSON can include `channels.json`, `groups.json`, `dms.json`, `mpims.json`, `users.json`, and per-conversation dated JSON files. The POC parser should use that shape later, but it is not part of the Flink import.

Slack API constraints:

- `conversations.history` requires a token with history scopes and access to the conversation.
- New non-Marketplace commercially distributed apps are rate-limited to 1 request/minute with a 15-object page size as of Slack's 2025 policy update.
- Workspace exports require Workspace Owner/Admin or Org Owner/Admin capabilities depending on plan and conversation type.
- A normal public-data crawler should not assume Slack admin, export, or app-install permissions.

## Storage Design

Use two storage layers:

```text
services/cubicle-graph/.data/
  graph.db
  snapshots/
    crawl-budget.json
    jira/<run_id>/search-page-000.json
    jira/<run_id>/issue-FLINK-39743.json
    github/<run_id>/pr-1127.json
    github/<run_id>/pr-1127-files.json
    github/<run_id>/pr-1127-comments.json
    docs/<run_id>/docs-tree.json
    ponymail/<run_id>/FLINK-39743.json
```

`graph.db` is the Ent SQLite database. `snapshots/` is the replay source of truth. `CUBICLE_GRAPH_DATA_ROOT` overrides `.data` for tests and local app integration.

The raw snapshot layer exists so mapper changes do not require re-crawling public APIs, and so every normalized fact can point back to a source artifact.

`crawl-budget.json` records the request budget and observed response metadata for every live run:

```json
{
  "run_id": "flink-autoscaler-2026-06-07T01-00-00Z",
  "sources": {
    "jira": {
      "required": true,
      "max_requests": 250,
      "requests_made": 0,
      "rate_policy": "serial, max 2 rps, honor retry-after"
    },
    "github": {
      "required": true,
      "max_requests": 1000,
      "requests_made": 0,
      "requires_token_for_full_crawl": true
    },
    "docs": {
      "required": true,
      "max_requests": 50,
      "requests_made": 0
    },
    "ponymail": {
      "required": false,
      "max_requests": 25,
      "requests_made": 0
    }
  }
}
```

Each source updates `requests_made`, `last_status`, `last_rate_limit_reset`, `last_retry_after`, and `freshness_state` as it runs.

## Ent Model

Use typed Ent schemas for stable execution objects:

```text
CrawlRun
SourceSnapshot
SourceEvent
Evidence
ConnectorState
Person
PersonAlias
Team
Project
Component
Ticket
PullRequest
CodeFile
Document
DocumentRevision
DocumentTab
DocumentFragment
DocumentSummary
Message
Decision
Blocker
Risk
ActionCandidate
SourcePermission
EmbeddingArtifact
SearchIndexState
OntologyObjectType
OntologyLinkType
OntologyActionType
GraphEdge
ImportCheckpoint
```

### Typed Objects

`Person`

- `display_name`
- `canonical_key`
- `primary_email_hash`
- edges: aliases, authored tickets, authored PRs, comments, owned blockers

`PersonAlias`

- `source`
- `source_user_id`
- `display_name`
- `email_hash`
- edge: person

`Team`

- `key`
- `name`
- `source`
- `visibility`
- edges: members, projects, components

`Project`

- `key`
- `name`
- `source`
- edges: components, tickets, pull requests, docs

`Component`

- `key`
- `name`
- edge: project

`Ticket`

- `source`
- `external_id`
- `key`
- `summary`
- `status`
- `issue_type`
- `priority`
- `created_at`
- `updated_at`
- edges: reporter, assignee, components, evidence

`PullRequest`

- `source`
- `repo`
- `number`
- `title`
- `state`
- `author_login`
- `created_at`
- `updated_at`
- `additions`
- `deletions`
- `changed_files`
- edges: author, tickets, code files, messages, evidence

`Document`

- `source`
- `external_id`
- `key`
- `title`
- `mime_type`
- `owner_key`
- `source_url`
- `parent_key`
- `latest_revision_key`
- `content_hash`
- `visibility`
- `updated_at`
- edges: owner, revisions, tabs, fragments, evidence, mentioned tickets

For GitHub docs, `external_id` can be `repo:path` and `latest_revision_key` can be the commit SHA. For Google Docs, `external_id` is the Drive file ID and `latest_revision_key` is the Docs/Drive revision or exported content hash.

`DocumentRevision`

- `key`
- `document_key`
- `source_revision_id`
- `content_hash`
- `export_mime_type`
- `created_at`
- `observed_at`
- `source_updated_at`
- `snapshot_key`

`DocumentTab`

- `key`
- `document_key`
- `revision_key`
- `tab_id`
- `title`
- `parent_tab_id`
- `path`
- `sort_order`

`DocumentFragment`

- `key`
- `document_key`
- `revision_key`
- `tab_key`
- `fragment_type`
- `heading_path`
- `ordinal`
- `start_index`
- `end_index`
- `content_hash`
- `text`
- `source_url`
- `visibility`
- `observed_at`
- edges: evidence, mentioned people, tickets, projects, decisions, blockers

`DocumentSummary`

- `key`
- `document_key`
- `revision_key`
- `summary_type`
- `summary_text`
- `model`
- `prompt_version`
- `source_content_hash`
- `generated_at`
- edges: source fragments and evidence

`EmbeddingArtifact`

- `key`
- `owner_kind`
- `owner_key`
- `content_hash`
- `model`
- `dimensions`
- `embedding_ref`
- `generated_at`
- `status`

`SearchIndexState`

- `owner_kind`
- `owner_key`
- `index_name`
- `content_hash`
- `indexed_at`
- `status`
- `error`

`SourcePermission`

- `source`
- `source_object_kind`
- `source_object_key`
- `principal_kind`
- `principal_key`
- `role`
- `visibility`
- `observed_at`
- `source_updated_at`
- `snapshot_key`

`Message`

- `source`
- `external_id`
- `channel_or_thread`
- `author_key`
- `body_hash`
- `body_excerpt`
- `created_at`
- edges: author, related ticket, related PR, evidence

`Evidence`

- `source`
- `source_url`
- `source_snapshot_key`
- `excerpt`
- `observed_at`
- `source_updated_at`
- `confidence`
- `visibility`
- `freshness_state`
- edge: source event

`CrawlRun`

- `key`
- `slice`
- `started_at`
- `finished_at`
- `status`
- `source_status_json`
- `error_summary`

`SourceSnapshot`

- `source`
- `run_id`
- `kind`
- `external_id`
- `source_url`
- `request_url`
- `path`
- `content_hash`
- `fetched_at`
- `source_updated_at`
- `http_status`
- `visibility`

`SourceEvent`

- `source`
- `external_id`
- `event_type`
- `event_time`
- `actor_key`
- `source_snapshot_key`
- `payload_hash`
- `visibility`

`ConnectorState`

- `source`
- `slice`
- `status`
- `last_success_at`
- `last_attempt_at`
- `last_error`
- `freshness_state`
- `cursor_value`

`CodeFile`

- `repo`
- `path`
- `language`
- `latest_sha`
- `source_url`
- `last_seen_at`

`Decision`

- `key`
- `summary`
- `status`
- `decided_at`
- `source`
- `confidence`
- edges: evidence, related tickets, related documents

`Blocker`

- `key`
- `summary`
- `status`
- `severity`
- `detected_at`
- `resolved_at`
- edges: owner, evidence, blocked tickets

`Risk`

- `key`
- `summary`
- `severity`
- `status`
- `detected_at`
- `confidence`
- edges: evidence, related project, related tickets

`ActionCandidate`

- `key`
- `action_type`
- `target_kind`
- `target_key`
- `owner_key`
- `summary`
- `rationale`
- `status`
- `confidence`
- `created_at`
- `expires_at`
- edges: evidence, related ticket, related PR, related blocker

`ImportCheckpoint`

- `source`
- `slice`
- `cursor_kind`
- `cursor_value`
- `updated_at`
- `last_successful_run_id`

### GraphEdge

Use `GraphEdge` for relations whose shape may evolve or whose target type varies:

```text
from_kind
from_key
predicate
to_kind
to_key
source
source_event_key
evidence_key
confidence
visibility
freshness_state
rule_name
association_sort_key
rank_score
observed_at
valid_from
valid_to
```

Initial predicates:

```text
has_component
implemented_by
changes_file
mentions
reported_by
assigned_to
reviewed_by
commented_by
depends_on
blocked_by
owned_by
evidenced_by
discussed_in
documents
has_revision
has_tab
has_fragment
summarized_by
embedded_as
visible_to
supports
references
has_gap
needs_action
needs_decision
stale_after
```

Use first-class Ent edges where the relation is stable and useful for common traversals, such as ticket-to-PR and PR-to-code-file. Use `GraphEdge` for inferred, evolving, or cross-type relations where edge metadata matters more than generated traversal methods.

This follows Ent's strengths: typed schemas and edges for stable relationships, edge schemas or explicit relation objects when relationship metadata must be queryable, and transactions around each import batch.

Do not collapse this into `from_ref`, `relation`, and `to_ref` only. That shape is acceptable for a toy graph, but it loses the Glean-style context metadata and the Palantir-style operational link semantics that Cubicle needs.

### Ontology Layer

The operational ontology is the layer above raw associations. It defines the user-facing nouns, allowed link types, and read-only action functions.

```text
OntologyObjectType
  ticket, pull_request, document, document_fragment, message, person, project, decision, blocker, risk

OntologyLinkType
  implemented_by, blocked_by, supports, evidenced_by, owned_by, visible_to, needs_action

OntologyActionType
  ask_owner, request_review, update_docs, record_decision, mark_stale
```

Keep ontology configuration small in V0. The first implementation can be Go constants plus tests. The Ent tables exist to make it easy to expose ontology metadata to Swift later and to prevent relation names from drifting.

### Google Docs Representation

A Google Doc is not one vector and should not be collapsed into one `Document.content` blob.

Represent it as:

```text
Document
  Drive file metadata: file ID, name/title, owner, mime type, source URL, latest revision, visibility
  |
  +-- DocumentRevision
  |     revision/export snapshot, content hash, observed time, source update time
  |
  +-- DocumentTab
  |     tab ID, title, parent tab, tab path, sort order
  |
  +-- DocumentFragment
        paragraph/list/table/heading fragments with heading path, structural location, text hash, text
```

Drive permissions are stored separately as `SourcePermission` rows and linked with `visible_to` graph edges. This mirrors how enterprise search systems keep ACL/freshness metadata close to content without turning permissions into text.

For GitHub markdown docs, the same shape still works:

```text
Document = repo:path
DocumentRevision = commit SHA or content hash
DocumentTab = default tab
DocumentFragment = markdown headings and paragraphs
```

The crawler always stores raw snapshots before fragments. Mapper changes should re-run from snapshots without re-fetching Drive, Docs, GitHub, or Jira.

### Search And RAG Model

Define search as four different operations:

```text
object search
  find tickets, docs, people, projects, PRs by key/title/path/metadata

evidence search
  find document fragments, messages, comments, and reviews by lexical or semantic match

graph traversal search
  expand from a known object through typed associations

answer search
  retrieve evidence, expand graph context, rerank, and synthesize an answer with citations
```

V0 implements object search, graph traversal search, and lexical evidence search. SQLite FTS5 is enough for the first local index over `DocumentFragment.text`, `Message.body_excerpt`, evidence excerpts, ticket summaries, PR titles, and code paths.

Vector search is future infrastructure, not the source of truth. Store embeddings as `EmbeddingArtifact` records tied to fragments or summaries:

```text
fragment embedding
  good for finding local supporting evidence

summary embedding
  good for routing to a likely document or section

whole-document embedding
  too lossy for long docs; avoid as the only representation
```

LLM-generated summaries are derived artifacts. They must record model, prompt version, source content hash, generation time, and source fragments. A summary can help retrieval and UX, but it cannot replace raw fragments or evidence.

Future RAG flow:

```text
user question
  -> parse filters and intent
  -> graph scope: project/person/source/visibility/freshness
  -> lexical retrieval for exact IDs, titles, paths, issue keys, symbols
  -> vector retrieval over document/message/comment fragments
  -> graph expansion around retrieved evidence
  -> rerank
  -> answer only with cited evidence
```

The graph controls trust, scope, freshness, and permissions. Embeddings help find likely text; they do not define truth.

## Ingestion Pipeline

Command:

```bash
go run ./cmd/cubicle-graph crawl-flink --slice autoscaler --since 2025-06-01
```

Flow:

```text
create crawl_run
  |
  v
fetch Jira pages
  |
  v
extract ticket keys and fixture candidates
  |
  v
search GitHub PRs
  |
  v
fetch PR detail/files/comments/reviews/commits
  |
  v
fetch docs tree and selected raw markdown
  |
  v
query Pony Mail by ticket key
  |
  v
write raw snapshots with content hashes
  |
  v
map snapshots into Ent transaction batches
  |
  v
compute graph edges and evaluation metrics
```

Idempotency:

- Every source item has a stable external ID.
- Every snapshot has a content hash.
- Every normalized object upserts on source + external ID.
- Every deterministic edge upserts on `from_kind`, `from_key`, `predicate`, `to_kind`, `to_key`, and `evidence_key`.
- Each source has an `ImportCheckpoint` recording the last successful cursor, page, or updated timestamp.
- Every live run records a `crawl_started_at` watermark.
- Jira queries use `updated >= jira_since AND updated <= crawl_started_at` to avoid a moving result set during pagination.
- GitHub seed search uses `updated:>=github_seed_since updated:<=<crawl_started_date>` when supported by the search query.
- Replay consistency compares normalized counts from snapshots, not live source totals after the fact.

Rate limiting:

- Use small page sizes during development: Jira `maxResults=50`, GitHub `per_page=100`, Pony Mail targeted issue-key queries only.
- Back off on HTTP 429 and 5xx with source-specific jittered exponential backoff.
- Cache snapshots before mapping.
- Make `GITHUB_TOKEN` optional for fixture fetches and capped probes, but required for full live PR-detail crawl.
- Treat missing GitHub token as `partial` when the requested live crawl exceeds the unauthenticated budget.
- Stop a source before exhausting rate limits when remaining budget is too low for the next page/detail batch.
- Show source-level import status in query output when a source is incomplete.

Failure behavior:

- A failed optional source does not fail the whole run.
- A failed required source marks the crawl run as partial and leaves the previous graph queryable.
- A missing GitHub token on a full live crawl marks GitHub partial unless the requested PR detail limit is small enough for unauthenticated budget.
- A challenged or blocked public archive page is not retried through browser scraping; mark that source unavailable and continue.
- A previously seen source item that returns 404/410 is not deleted immediately. Mark the object or edge `freshness_state=stale`, create a tombstone source event, and keep prior evidence visible as historical.
- A missing remote link on a later Jira fetch marks the `implemented_by` edge stale unless another source still supports it.
- Mapper errors include source, snapshot path, external ID, and JSON field path.
- Missing fields become null facts or gap edges when meaningful.

## Query Layer

The query layer returns product-shaped answers, not raw graph dumps.

Query code must use the association store primitives instead of raw Ent queries wherever possible:

```text
ReadinessQuery
  -> AssociationStore.Expand(project:flink-autoscaler, contains, 1)
  -> AssociationStore.Expand(ticket, blocked_by|implemented_by|needs_decision, 1)
  -> AssociationStore.TraceEvidence(edge)
  -> ActionRules.Evaluate(scoped graph)
```

This keeps the service close to the Meta/TAO object-association model while still letting Ent own schema and SQL writes.

Every query response includes:

- `answer_generated_at`
- `source_status`
- `findings`
- `action_candidates`
- `evidence`

Every evidence item includes `source`, `source_url`, `observed_at`, `source_updated_at`, `confidence`, `visibility`, `freshness_state`, and `snapshot_ref`.

### Ticket Trace

```text
GET /v1/tickets/FLINK-39743/trace
```

Returns:

- ticket summary and status
- reporter and assignee
- linked PRs
- changed files
- comments and reviews
- docs mentioning the issue or component
- gaps such as missing assignee or stale open PR
- evidence list

### Readiness

```text
GET /v1/projects/flink-autoscaler/readiness
```

Returns:

- open ticket count
- tickets with pull requests
- tickets without owners
- PRs awaiting review or stale updates
- docs likely affected by open code changes
- blockers and risks inferred from deterministic rules
- read-only action candidates, such as ask owner, review PR, update docs, record decision
- evidence list for each finding

### Owner Gaps

```text
GET /v1/projects/flink-autoscaler/owner-gaps
```

Returns tickets, PRs, blockers, or components with missing or conflicting ownership.

### Review Gaps

```text
GET /v1/projects/flink-autoscaler/review-gaps
```

Returns open PRs with no review, stale reviews, or changed high-signal files without reviewer activity.

### Evidence Neighborhood

```text
GET /v1/graph/neighborhood?node=ticket:FLINK-39743&depth=2
```

Returns a small graph neighborhood with all edges carrying evidence IDs, source URLs, timestamps, and confidence.

### Search

```text
GET /v1/search?q=autoscaler+expected+processing+rate&project=flink-autoscaler
```

Returns typed hits:

- exact object hits: tickets, PRs, docs, people, projects, code files
- lexical evidence hits: document fragments, messages, comments, evidence excerpts
- graph context around each hit: owner, source, visibility, freshness, related tickets or PRs
- explicit `no_answer_reason` when no evidence hit satisfies source, freshness, and visibility filters
- no generated answer in V0

RAG answers are a later layer on top of this result set, not the first search implementation.

### Action Candidates

```text
GET /v1/projects/flink-autoscaler/action-candidates
```

Returns read-only suggestions generated from deterministic graph rules:

- ask missing owner for an unassigned open ticket
- request review on an open PR with no review activity
- update docs when a PR changes files covered by an Autoscaler doc
- record a decision when discussion mentions uncertainty and no decision object exists

The service does not write these actions back to source systems in the POC.

## Evaluation

The first POC must be judged by graph correctness before UI polish.

Deterministic checks:

- PR title/body `FLINK-\d+` extraction links PRs to Jira tickets.
- Every linked Jira key from a sampled PR exists in ASF Jira.
- Jira components produce `has_component` edges.
- Jira labels produce label facts and gap indicators.
- GitHub PR files produce `changes_file` edges.
- PR comments and reviews produce `discussed_in`, `commented_by`, and `reviewed_by` edges.
- Replay import from snapshots produces the same object and edge counts as live import.
- Every query-facing edge has evidence, visibility, confidence, observed time, and freshness state.
- Deterministic rules produce read-only action candidates with evidence.
- Association queries return stable cursor order by `association_sort_key` or observed time.
- Document fragmentation preserves source URL, revision, tab/path, text hash, and heading path.
- Object search finds exact issue keys, PR numbers, doc paths, people, and code paths.
- Lexical evidence search finds the fragment/message/comment that supports a known answer.
- No-answer behavior is explicit when no supporting evidence exists.
- The three product gates pass with evidence: launch readiness, ticket trace, and rollout decision trace.

Metrics:

```text
ticket_pr_link_recall
ticket_pr_link_precision
snapshot_replay_consistency
source_coverage_by_type
evidence_coverage_by_edge_type
edge_metadata_completeness
action_candidate_evidence_coverage
association_metadata_completeness
document_fragment_trace_coverage
exact_object_search_recall
lexical_evidence_search_recall
product_gate_pass_count
no_answer_behavior_pass
query_latency_p95_local
open_gap_count_by_rule
```

Minimum acceptance for the Flink import:

- At least 100 Autoscaler Jira tickets imported.
- At least 10 on-slice ticket-to-PR traces after targeted GitHub discovery.
- At least 5 ticket-to-PR-to-file traces.
- At least 3 ticket-to-PR-to-doc candidate traces with confidence >= 0.75.
- At least 3 ticket-to-discussion traces from Jira comments, PR comments/reviews, or `dev@` Pony Mail.
- Every imported GitHub PR detail must link to at least one Autoscaler Jira key, unless it is explicitly marked as an off-slice seed artifact.
- At least 20 docs imported.
- At least one end-to-end ticket trace for `FLINK-39743`.
- Every returned query finding has at least one evidence object.
- Every returned action candidate has at least one evidence object.
- Every returned document hit can trace to a document revision, fragment, source URL, and snapshot.
- Exact search for `FLINK-39743`, `1127`, `JobVertexScaler.java`, and `Expected Processing Rate` returns the expected object or fragment.
- Snapshot replay creates identical counts for tickets, PRs, docs, messages, and graph edges.

## Go Service Shape

Module:

```text
services/cubicle-graph/
```

Packages:

```text
cmd/cubicle-graph          CLI: serve, crawl-flink, replay, query
internal/config            env and defaults
internal/store             Ent client, migrations, transaction helper
internal/sources/jira      Jira fetcher and snapshot writer
internal/sources/github    GitHub fetcher and snapshot writer
internal/sources/docs      docs tree and raw markdown fetcher
internal/sources/gdocs     future Google Drive/Docs fetcher and snapshot writer
internal/sources/ponymail  optional mailing-list fetcher
internal/ingest            snapshot replay and mapper orchestration
internal/graphmodel        domain structs and graph key helpers
internal/graphstore        AssociationStore object and association primitives
internal/ontology          object/link/action type registry
internal/query             product query layer
internal/search            SQLite FTS/object/evidence search
internal/actions           deterministic read-only action candidate rules
internal/httpapi           Gin + Huma localhost JSON routes and OpenAPI contract
internal/eval              graph correctness metrics
```

HTTP routes:

```text
GET /healthz
GET /v1/crawl-runs
GET /v1/tickets/{ticketKey}/trace
GET /v1/projects/{projectKey}/readiness
GET /v1/projects/{projectKey}/owner-gaps
GET /v1/projects/{projectKey}/review-gaps
GET /v1/projects/{projectKey}/action-candidates
GET /v1/search?q=<query>&project=<projectKey>
GET /v1/documents/{documentKey}/trace
GET /v1/graph/neighborhood?node=<kind:key>&depth=<n>
```

The service owns the graph database. Swift should not write this DB directly. Swift integration later uses localhost HTTP JSON.

The HTTP server uses Gin as the framework and Huma as the typed API layer:

```text
Swift app
 |
 v
Swift OpenAPI Generator client
 |
 v
Huma OpenAPI operations
 |
 v
Gin router / middleware / recovery
 |
 v
internal/query
 |
 v
AssociationStore / Ent / SQLite
```

## Database Choice

Use SQLite for V0:

- free
- local-first
- easy to reset
- supported by Ent
- good enough for a small crawler and query POC
- includes FTS5 for local lexical search over fragments, messages, titles, paths, and evidence excerpts

Use WAL mode when the service runs with concurrent read/query traffic. Move to Postgres only after one of these is true:

- multiple processes need writes
- graph volume exceeds local SQLite comfort
- remote deployment becomes real
- per-user permissions become production requirements
- vector search becomes a first-class runtime dependency, at which point pgvector/Postgres is the cleanest production path

## Privacy And Security

The Flink import uses public data only. Still, the service should avoid creating bad habits:

- Store raw public identities locally only.
- Use email hashes for aliases when email appears.
- Keep source visibility fields even when all sources are public.
- Include source URLs and timestamps with answers.
- Add anonymization for reports generated from synthetic or imported data.
- Avoid private Slack scraping.

The localhost POC does not need auth, but the graph schema should not make permissions impossible later.

## Iteration Order

1. Build and validate synthetic dataset with known ground truth.
2. Build Ent schema and SQLite storage.
3. Add AssociationStore object and association primitives.
4. Add document revision/tab/fragment/search schema.
5. Implement query layer over synthetic data.
6. Add raw snapshot writer.
7. Add Flink Jira crawler and snapshot replay.
8. Add GitHub PR crawler and PR-to-ticket edge mapping.
9. Add docs crawler.
10. Add optional Pony Mail crawler.
11. Add deterministic action candidate rules.
12. Add evaluation metrics and snapshot replay consistency checks.
13. Add localhost HTTP API.
14. Integrate Swift only after queries pass on synthetic and Flink slices.

## Open Decisions Resolved For POC

- Data root defaults to `services/cubicle-graph/.data`; `CUBICLE_GRAPH_DATA_ROOT` overrides it.
- Swift does not access `graph.db` directly.
- Flink import starts with Autoscaler, not all Kubernetes Operator.
- Pony Mail is optional and never blocks Jira/GitHub/docs import.
- LLM extraction is not part of the first mapper.
- Edge metadata is mandatory for all query-facing graph facts.
- Whole-document vectors are not the document representation.
- Document fragments are the retrieval/evidence unit.
- SQLite FTS5 is the V0 search index; vector search remains future infrastructure.
- Read-only action candidates are mandatory for readiness-style answers.
- Writeback actions are deferred until read-only graph answers and action candidates are trusted.

## Sources

- Apache Flink community: https://flink.apache.org/what-is-flink/community/
- Apache Flink Kubernetes Operator repo: https://github.com/apache/flink-kubernetes-operator
- ASF Jira search endpoint used for live probes: https://issues.apache.org/jira/rest/api/2/search
- Atlassian Jira search API docs: https://developer.atlassian.com/server/jira/platform/rest/v10000/api-group-search/
- Atlassian Jira Cloud rate limiting: https://developer.atlassian.com/cloud/jira/platform/rate-limiting/
- Atlassian Jira Cloud webhooks: https://developer.atlassian.com/cloud/jira/software/webhooks/
- Atlassian Jira Cloud issue changelog API: https://developer.atlassian.com/cloud/jira/platform/rest/v3/api-group-issues/
- GitHub REST pull request docs: https://docs.github.com/en/rest/pulls
- GitHub REST search docs: https://docs.github.com/en/rest/search/search
- GitHub REST repository contents docs: https://docs.github.com/en/rest/repos/contents
- GitHub REST pagination docs: https://docs.github.com/en/rest/using-the-rest-api/using-pagination-in-the-rest-api
- GitHub REST rate limits: https://docs.github.com/en/rest/using-the-rest-api/rate-limits-for-the-rest-api
- GitHub CLI manual: https://cli.github.com/manual/gh_api
- Apache Pony Mail API docs: https://ponymail.apache.org/docs/api.html
- Slack export format docs: https://slack.com/help/articles/220556107-How-to-read-Slack-data-exports
- Slack conversations.history docs: https://api.slack.com/methods/conversations.history
- Slack conversations.replies docs: https://api.slack.com/methods/conversations.replies
- Slack Events API docs: https://api.slack.com/apis/events-api
- Slack app_rate_limited event docs: https://api.slack.com/events/app_rate_limited
- Slack message event docs: https://api.slack.com/events/message
- Slack data export docs: https://slack.com/help/articles/201658943-Export-your-workspace-data
- Apache Flink Slack archive: https://www.linen.dev/s/apache-flink
- GitHub webhook delivery docs: https://docs.github.com/en/webhooks/using-webhooks/handling-webhook-deliveries
- GitHub webhook best practices: https://docs.github.com/en/webhooks/using-webhooks/best-practices-for-using-webhooks
- Ent schema edges: https://entgo.io/docs/schema-edges/
- Ent privacy: https://entgo.io/docs/privacy/
- Ent transactions: https://entgo.io/docs/transactions/
- Ent migrations: https://entgo.io/docs/migrate/
- Ent supported dialects: https://entgo.io/docs/dialects/
- Gin package docs: https://pkg.go.dev/github.com/gin-gonic/gin
- Huma package docs: https://pkg.go.dev/github.com/danielgtaylor/huma/v2
- Huma Gin adapter package docs: https://pkg.go.dev/github.com/danielgtaylor/huma/v2/adapters/humagin
- Swift OpenAPI Generator: https://github.com/apple/swift-openapi-generator
- Go database/sql connection pool docs: https://pkg.go.dev/database/sql#DB.SetMaxOpenConns
- Go slog docs: https://pkg.go.dev/log/slog
- Meta TAO engineering article: https://engineering.fb.com/2013/06/25/core-infra/tao-the-power-of-the-graph/
- TAO USENIX paper: https://www.usenix.org/system/files/conference/atc13/atc13-bronson.pdf
- Unicorn social graph search paper: https://www.vldb.org/pvldb/vol6/p1150-curtiss.pdf
- Glean knowledge graph: https://docs.glean.com/security/knowledge-graph
- Glean connectors: https://docs.glean.com/connectors/about
- Glean Enterprise Graph: https://www.glean.com/product/enterprise-graph
- Glean knowledge graph article: https://www.glean.com/blog/knowledge-graph-agentic-engine
- Glean code search architecture: https://docs.glean.com/security/how-code-search-works
- Palantir Ontology overview: https://www.palantir.com/docs/foundry/ontology/overview/
- Palantir Ontology design best practices: https://www.palantir.com/docs/foundry/ontology/ontology-best-practices-and-anti-patterns
- Palantir Health and Life Sciences: https://www.palantir.com/offerings/health/
- Palantir pharmaceutical research whitepaper: https://www.palantir.com/assets/xrfr7uokpv1b/57baRLActYC6ToQaVlRnA1/ff13411f7e7bc8c08022b3b77b250e3c/Palantir_Foundry_Pharmaceutical_Whitepaper.pdf
- Palantir Foundry for Research: https://www.palantir.com/assets/xrfr7uokpv1b/6E5HaZFqWjOa3ssQuU5FON/2005dfd3151ffcb0cd0803f01c26017d/Foundry_for_Research.pdf
- Google Docs document structure: https://developers.google.com/docs/api/concepts/structure
- Google Docs tabs: https://developers.google.com/workspace/docs/api/how-tos/tabs
- Google Drive API files: https://developers.google.com/drive/api/reference/rest/v3/files
- Google Drive API files.export: https://developers.google.com/drive/api/reference/rest/v3/files/export
- Google Drive API files.list: https://developers.google.com/drive/api/reference/rest/v3/files/list
- Google Drive API push notifications: https://developers.google.com/drive/api/guides/push
- Google Drive API comments: https://developers.google.com/drive/api/reference/rest/v3/comments
- Google Drive API replies: https://developers.google.com/drive/api/reference/rest/v3/replies
- Google Drive API permissions: https://developers.google.com/drive/api/reference/rest/v3/permissions
- Google Drive API changes: https://developers.google.com/drive/api/guides/manage-changes
- Google Drive API limits: https://developers.google.com/drive/api/guides/limits
- SQLite WAL: https://www.sqlite.org/wal.html
- SQLite appropriate uses: https://www.sqlite.org/whentouse.html
- SQLite FTS5: https://www.sqlite.org/fts5.html
- SQLite backup API: https://www.sqlite.org/backup.html
- OpenAI embeddings guide: https://platform.openai.com/docs/guides/embeddings
- OpenAI file search guide: https://platform.openai.com/docs/guides/tools-file-search/
- RAG paper: https://arxiv.org/abs/2005.11401
- pgvector: https://github.com/pgvector/pgvector
