# Cubicle Graph Product Hardening Research

Date: 2026-06-07

## Purpose

This is a product-wide hardening pass for the Cubicle Go/Ent graph backend. It extends the Flink crawler and Go service design by studying adjacent systems: enterprise search, teamwork graphs, data catalogs, authorization engines, CDC/connector platforms, graph databases, search engines, RAG evaluation tools, and observability standards.

The product thesis stays the same:

```text
Cubicle should reduce TPM / program-manager coordination work
by answering engineering execution questions with evidence,
freshness, provenance, and safe next actions.
```

The design implication is sharper now:

```text
do not build "a graph database"
build an evidence-backed work context layer
 |
 +-- source snapshots
 +-- connector state
 +-- permission facts
 +-- typed execution objects
 +-- metadata-rich associations
 +-- search/evidence lanes
 +-- no-answer behavior
 +-- read-only action candidates
 +-- product evals
```

## External Patterns That Matter

- Glean-style systems index content, metadata, permissions, and activity together. The lesson is that permissions and source freshness are not secondary metadata.
- Atlassian Teamwork Graph and Notion Enterprise Search expose source scope, connector status, and permission behavior to admins/users. The lesson is that graph quality is an operational product surface.
- Palantir Ontology links objects, properties, actions, functions, dynamic security, and lineage. The lesson is that actions must be modeled with governance from day one.
- DataHub, OpenMetadata, and Apache Atlas treat lineage, classifications, ingestion state, and stale metadata as first-class. The lesson is that historical provenance and stale-state handling are core graph features.
- Airbyte and Debezium treat state/checkpoints/idempotency as the difference between a demo connector and a real connector.
- OpenFGA/Zanzibar-style authorization models show why access facts should be typed relationship tuples, not ad hoc strings inside search rows.
- Elasticsearch/OpenSearch and Lucene show that search is near-real-time, index-backed, analyzer-sensitive, and permission-sensitive.
- RAGAS, TruLens, and OpenAI evaluation guidance show that retrieval, grounding, and answer relevance need separate evals.

## Product Hardening Ledger

| Iteration | Deep Question | External Pattern | Design Correction |
|---|---|---|---|
| 141 | What if Cubicle mirrors content but not permissions as first-class data? | Glean connectors fetch content plus permissions. | Split content indexing and access indexing; every query-facing object carries permission facts and permission freshness. |
| 142 | What if a document is public by link but only discoverable after it appears in Slack or a pin? | Glean documents nuanced Drive public/domain visibility. | Model `discoverability_source`: direct ACL, source searchability, link mention, pin, or manual allow. |
| 143 | What if attachments are too large to download or parse? | Glean applies crawler/indexing size limits. | Add `AttachmentExtractionState`; metadata/permissions can index even when body extraction is skipped. |
| 144 | What if indexing is queued and content is crawled before search catches up? | Glean indexing APIs queue work. | Separate `crawled_at`, `mapped_at`, `indexed_at`, and `query_visible_at`. |
| 145 | What if admins cannot tell whether a connector handles permissions/deletes correctly? | Atlassian asks admins to evaluate Teamwork Graph connectors. | Add `ConnectorCapability` manifest: permissions, deletions, webhooks, cursor type, stale policy, rate policy. |
| 146 | What if users need to scope a query to Slack, Jira, docs, or web? | Notion Enterprise Search exposes source scope controls. | Query DTO includes explicit source scope; UI can default to all, but API must make scope visible. |
| 147 | What if admins cannot see graph/data volume by source? | Atlassian exposes indexed object counts. | Add `/v1/sources` counters: objects, edges, evidence, hidden, stale, partial, last sync. |
| 148 | What if permission fields tokenize incorrectly because they contain hyphens or special characters? | OpenSearch warns DLS text analyzers can break special-character IDs. | Store ACL principals as exact keyword-like fields; never permission-filter on FTS text. |
| 149 | What if permissions become too complex for ad hoc filters? | OpenFGA uses typed relationship tuples. | Model access as `principal -> relation -> object` tuples; keep source ACL metadata separate from search text. |
| 150 | What if query-time contextual permissions go stale? | OpenFGA contextual tuples can depend on token freshness. | Permission checks carry `identity_snapshot_at` and source group freshness; stale identity blocks generated answers. |
| 151 | What if the ontology is only a schema and not an operational layer? | Palantir Ontology is semantic plus kinetic: objects, links, actions, functions, security. | Keep object/link/action registries together; action candidates are part of ontology, not an afterthought. |
| 152 | What if link types lack cardinality and inverse semantics? | Palantir link types track cardinality and metadata. | `OntologyLinkType` includes cardinality, inverse predicate, evidence policy, sort policy, and status. |
| 153 | What if actions mutate source systems without enough governance? | Palantir action rules define ontology edits and effects. | V0 actions stay read-only; future writeback needs effect preview, authorization, idempotency key, and audit trail. |
| 154 | What if sensitive labels disappear when summaries or embeddings are derived? | Palantir markings propagate through dependencies. | Sensitivity/visibility propagates from source evidence to summaries, embeddings, action candidates, and derived edges. |
| 155 | What if inferred lineage references objects not in the graph? | OpenMetadata lineage requires entities to exist before lineage is useful. | Do not create query-facing edges unless both endpoint objects are anchored and evidence exists. |
| 156 | What if stale source metadata remains in graph forever? | DataHub stateful ingestion can soft-delete stale metadata. | Add soft-delete/tombstone reconciliation after every source run. |
| 157 | What if classification or sensitivity should propagate along graph paths? | Apache Atlas propagates classifications via lineage. | Add a rule for propagating `classification` and `sensitivity` along `derived_from`, `summarizes`, and `embedded_as`. |
| 158 | What if important lineage exists only in human knowledge? | OpenMetadata supports manual lineage. | Allow `manual_assertion` edges with actor, timestamp, confidence, and review status. |
| 159 | What if ontology types change while apps depend on them? | Palantir link/object metadata uses lifecycle status. | Object/link/action types have `experimental`, `active`, `deprecated`, and `removed` states. |
| 160 | What if every noun becomes a graph object? | Graph modeling guidance is query-driven. | Type admission requires a product query, source evidence, and lifecycle owner. |
| 161 | What if delta APIs replay records? | Microsoft Graph delta query can produce replays. | Every connector mapper must be idempotent by source object key plus version/hash. |
| 162 | What if webhooks stop and changes are missed? | Microsoft Graph lifecycle notifications warn about missed notifications. | Webhooks are wakeups only; scheduled reconciliation remains mandatory. |
| 163 | What if API throttling keeps counting immediate retries? | Microsoft Graph says to honor `Retry-After` and avoid immediate retries. | Required-source crawler stops, records `SourceError`, and resumes later instead of retry-spinning. |
| 164 | What if checkpoint state advances before rows are committed? | Airbyte checkpointing depends on destination commit acknowledgement. | Advance connector cursor only after snapshot write and mapper transaction commit. |
| 165 | What if future actions need reliable outbound events? | Debezium outbox uses event IDs and aggregate keys. | Add future `ActionOutbox` design with idempotency key, aggregate key, payload hash, and dispatch status. |
| 166 | What if a durable crawl workflow retries non-deterministic code? | Temporal-style durable workflows require deterministic workflow logic. | Keep live HTTP calls in activities; mapper/replay logic is deterministic over snapshots. |
| 167 | What if push subscriptions expire quietly? | Microsoft Graph and Drive push channels have subscription lifetimes. | ConnectorState tracks subscription expiry, renewal deadline, and missed-notification recovery. |
| 168 | What if shared-drive or team-space discovery is incomplete? | Drive/Microsoft Graph broad corpus listing can miss content or need explicit corpora. | Source state includes corpus inventory: drives, sites, workspaces, channels, and discovery gaps. |
| 169 | What if deletion events carry only IDs? | Drive/Microsoft delta/remove events can be sparse. | Tombstone by stable source ID and preserve last known metadata for traceability. |
| 170 | What if GitHub/Jira webhooks are offline during laptop sleep? | Webhooks are incomplete if receiver is unavailable. | Localhost POC does not rely on webhooks; reconcile through REST snapshots. |
| 171 | What if users expect search to reflect writes immediately? | Elasticsearch search is near-real-time, not real-time. | Search responses disclose index freshness; tests avoid assuming immediate FTS visibility after ingest. |
| 172 | What if forced refresh kills ingest throughput? | Elasticsearch refresh has index/search/merge costs. | Do not force refresh during bulk ingest; use explicit index state transitions and test-only waits. |
| 173 | What if paginated search results shift while paging? | Elasticsearch uses point-in-time plus `search_after`. | Query APIs use stable cursors based on sort key plus object key and include result snapshot metadata. |
| 174 | What if engineering identifiers are split by analyzers? | OpenSearch warns special characters affect tokenization. | Exact keys and paths use keyword/exact lanes; FTS is a secondary lane. |
| 175 | What if hybrid search hides why a result matched? | Enterprise search products expose source/scope/facets. | Return exact, lexical, graph, and future vector hits in separate lanes with match reason. |
| 176 | What if RAG retrieves irrelevant context and the answer looks plausible? | TruLens RAG triad separates context relevance, groundedness, and answer relevance. | Add eval metrics for retrieval relevance, grounding, and answer utility separately. |
| 177 | What if the right evidence is missing from retrieval? | RAGAS tracks context recall and precision. | Add retrieval evals for context precision, context recall, faithfulness, and no-answer correctness. |
| 178 | What if evaluation is just vibe testing? | OpenAI eval guidance warns against vibe-based evals. | Product gates are concrete tasks with golden evidence paths and negative cases. |
| 179 | What if one fragmenting strategy fails across docs, PRs, tickets, and chat? | LangChain retrieval docs separate loaders/splitters/retrievers. | Fragmenting is source-type-specific but maps into one evidence DTO. |
| 180 | What if generated answers cite stale or irrelevant sources? | RAG evaluation tools check source grounding. | V0 has no generated prose; future generated answers require per-claim citations and freshness checks. |
| 181 | What if permission decisions need consistency metadata? | Zanzibar-style systems emphasize consistent authorization checks. | Permission results include `acl_snapshot_key` and `acl_observed_at`. |
| 182 | What if group access explodes into too many per-user rows? | OpenFGA usersets represent groups and nested sets. | Store group/userset relationships separately; expand lazily or through cached permission materialization. |
| 183 | What if document-level security filters hurt search latency? | OpenSearch recommends keeping DLS queries simple. | Overfetch search candidates, then apply fast exact permission filters; record filtered count. |
| 184 | What if a broad connector indexes content available to all crawler users? | Atlassian website connector warns individual permissions may not be respected. | Connector capability manifest marks `permission_model=global`, `source_acl`, or `unknown`; queries disclose it. |
| 185 | What if connector credentials outlive the user who installed them? | Notion connector docs discuss owner/admin install behavior. | Store connector owner/admin principal and rotation state. |
| 186 | What if one tenant/site connects to multiple workspaces? | Atlassian connectors are site/workspace scoped. | Every external ID is scoped by source instance, tenant/site, and workspace. |
| 187 | What if a third-party connector claims permission support but deletes poorly? | Atlassian connector evaluation asks about sync/deletes/permissions. | Add connector certification tests: delete, permission removal, rename, stale cursor, and rate-limit fixture. |
| 188 | What if DLP wants content hidden regardless of source ACL? | Glean supports hiding/removing content from search. | Add `hidden_by_policy` and make it stronger than source permission. |
| 189 | What if support/customer data enters the graph later? | Enterprise search guidance stresses minimization and retrieval guardrails. | Default to metadata-only ingestion for sensitive sources until minimization policy exists. |
| 190 | What if an agent uses graph context beyond the invoking user's scope? | Enterprise agent products surface agent governance concerns. | Every action/query run records invoking user, agent identity, effective scopes, and policy decisions. |
| 191 | What if high-degree nodes dominate association lists? | Meta TAO and JanusGraph emphasize association/vertex-centric indexes. | Association lists are indexed by `from_kind/from_key/predicate/sort_key`; high-degree nodes get per-predicate limits. |
| 192 | What if a hub node like `project:flink` makes every traversal expensive? | Graph systems call these supernodes. | Add hub detection and require predicate filters for hub expansion. |
| 193 | What if graph modeling optimizes elegance instead of queries? | Neo4j modeling guidance starts from query needs. | Every edge type must name the product query it accelerates. |
| 194 | What if local SQLite cannot handle graph volume? | Neptune/JanusGraph target billions of edges with managed/distributed stores. | Add promotion trigger: edge count, DB size, write contention, query p95, or multi-user serving. |
| 195 | What if reverse edges double storage and diverge? | Graph stores often rely on adjacency indexes. | Default to reverse indexes; materialize inverse edges only with `derived_from_edge_key` and tests. |
| 196 | What if recursive traversal causes path explosion? | Graph databases require bounded traversal patterns. | `Expand` requires depth, predicate allowlist, per-node limit, total edge budget, and cursor. |
| 197 | What if path order matters for "why did this happen?" | Lineage tools preserve upstream/downstream provenance. | Query DTO includes ordered path segments with evidence per segment. |
| 198 | What if materialized graph facts go stale? | Search/catalog systems track index and ingestion status. | Derived edges include `derived_from`, `mapper_version`, `freshness_state`, and remap trigger. |
| 199 | What if users ask historical questions? | Data lineage and graph delta research preserve state over time. | Keep valid-time and observed-time on events, evidence, and edges. |
| 200 | What if SQLite works in tests but not with realistic edge counts? | Graph DB benchmarks show scale changes behavior. | Add synthetic scale fixture: high-degree project, 10k fragments, 100k edges, bounded-query p95 target. |
| 201 | What if the product becomes another dashboard? | Cubicle thesis is to reduce TPM coordination work. | Product gates remain question/action oriented: can launch, what blocks, who owns, what changed, what action next. |
| 202 | What if documented context is not the real decision trail? | Teamwork graph discussions highlight gaps across PR comments, Slack, docs, and tickets. | Decision trace query prioritizes evidence chain, not only final docs. |
| 203 | What if sources contradict each other? | Operational graphs need confidence and provenance. | Add `Conflict` object and contradiction detection: same target, incompatible status/owner/date, competing evidence. |
| 204 | What if ownership is ambiguous? | Enterprise search connects people/activity, but aliases can be weak. | Owner answers must distinguish assigned, mentioned, reviewer, approver, and inferred owner. |
| 205 | What if every risk is labeled a blocker? | Operational workflows need clear semantics. | `Blocker` requires evidence of dependency or launch impact; otherwise classify as `Risk` or `Question`. |
| 206 | What if action candidates are noisy? | Palantir-style actions are useful when tied to workflow effects. | Action candidates need trigger rule, expected outcome, target owner, evidence, and suppression state. |
| 207 | What if users ignore source health? | Glean/Atlassian/Notion surface connectors and scopes. | Every answer includes source status summary when a source is partial, stale, or absent. |
| 208 | What if a source gap is the most important answer? | Missing Slack/docs/Jira data changes confidence. | Add "evidence gap" as a first-class answer state. |
| 209 | What if freshness means different things per source? | Connectors update via webhooks, deltas, or crawls. | Source freshness SLA is per-source and included in `ConnectorCapability`. |
| 210 | What if users correct a bad graph fact? | Data catalogs allow manual lineage/metadata curation. | User correction creates manual assertion or suppression edge, never overwrites source evidence. |
| 211 | What if logs cannot explain a bad answer? | OpenTelemetry correlates logs, traces, and metrics. | Query/crawl logs include request ID, crawl run ID, source, snapshot key, and evidence keys. |
| 212 | What if source sync looks healthy but user answers are bad? | SRE golden signals focus on latency, traffic, errors, saturation. | Add graph product signals: answer latency, no-answer rate, source partial rate, stale-hit drop rate, eval pass rate. |
| 213 | What if a crawl failure cannot be reproduced? | Data platforms preserve run/checkpoint details. | Every crawl run records request pages, cursors, budgets, retries, errors, and mapper versions. |
| 214 | What if one connector consumes all API budget? | API platforms throttle by app/user/resource. | Global and per-source budgets; optional sources cannot starve required sources. |
| 215 | What if audit trails only cover actions, not reads? | Enterprise AI governance tracks tool calls and access decisions. | Audit query reads that include sensitive/private sources; log effective visibility policy. |
| 216 | What if backup loses WAL data? | SQLite WAL requires backup API or checkpointed copies. | DB backup path uses backup API/checkpoint; source snapshots stay separate and replayable. |
| 217 | What if raw snapshots contain secrets or private messages? | Enterprise search products apply governance and content hiding. | Raw snapshot export requires redaction; local snapshots are backend-private. |
| 218 | What if migrations break replayed snapshots? | Ent/Atlas versioned migrations avoid silent schema drift. | Add schema version plus mapper version to replay tests. |
| 219 | What if a fixture masks a source API change? | Connector platforms test against fixtures and live probes. | Keep fixture mode deterministic and add explicit live probe command with source status output. |
| 220 | What if incident recovery needs rebuilding the graph? | Event sourcing/outbox patterns use durable logs. | Rebuild graph from snapshots and source events; normalized tables are derived state. |
| 221 | What if synthetic data passes but real data fails? | RAG evals need domain-specific goldens. | Maintain both synthetic ground truth and Flink public-slice evals. |
| 222 | What if real data has no answer? | No-answer evals catch hallucination. | Negative evals include missing decision, private doc, stale source, and unsupported action. |
| 223 | What if permission-safe retrieval lowers recall too much? | RAG systems often overfetch then filter. | Measure pre-filter recall, post-filter recall, and filtered-out count. |
| 224 | What if stale docs rank above fresh tickets? | Search freshness and source type matter. | Ranking includes freshness, source type, exact key match, graph proximity, and evidence confidence. |
| 225 | What if answer faithfulness passes but answer is useless? | TruLens separates answer relevance from groundedness. | Product eval checks utility: identifies owner/blocker/decision/action when asked. |
| 226 | What if context precision is high but recall is low? | RAGAS tracks both precision and recall. | Eval requires expected evidence coverage, not just top-hit quality. |
| 227 | What if reranking drifts silently? | IR evals use MRR/nDCG/hit rate. | Track exact hit rate, MRR, nDCG, and source diversity for search queries. |
| 228 | What if actions are evidence-backed but not useful? | Workflow products measure completion/acceptance. | Later action eval records user accepted/dismissed/ignored, but V0 remains read-only. |
| 229 | What if citations are present but weak? | RAG faithfulness checks claims against context. | Citation eval requires every claim map to evidence and every evidence object map to source snapshot. |
| 230 | What if source API fixtures drift from live APIs? | Connector frameworks need fixture refresh workflows. | Add `refresh-fixtures` command that writes new snapshots and flags schema diffs. |
| 231 | What if Swift binds to backend internals? | Enterprise products version public APIs. | Swift consumes versioned HTTP DTOs only; no Ent, SQLite, FTS, or snapshot paths in DTOs. |
| 232 | What if the local backend is down? | Local apps need health/status surfaces. | Swift integration starts with `/healthz`, `/v1/sources`, and graceful "graph unavailable" state. |
| 233 | What if users cannot trust an answer at a glance? | Notion/Glean cite sources and scope. | Answer UI must show source badges, freshness, confidence, and evidence count. |
| 234 | What if users need to inspect the reasoning path? | Teamwork Graph exposes objects and relationships. | Add graph path explanation DTO: nodes, edges, predicates, confidence, evidence. |
| 235 | What if connector consent is unclear? | Notion/Atlassian connectors require admin/user setup. | Settings must show source, credentials owner, scopes, last sync, and permission model. |
| 236 | What if localhost POC skips auth and then schema cannot support auth? | OpenFGA/enterprise search show auth must be structural. | Keep authN off for POC, but keep principal/visibility/source permission schema from day one. |
| 237 | What if the local port is reachable by other machines? | Localhost services should bind narrowly. | Bind to `127.0.0.1` by default and reject public bind unless explicit dev flag. |
| 238 | What if private workplace data enters before policy exists? | Enterprise search security guidance favors minimization. | Product validation starts synthetic/public; private connectors require redaction and permission eval gates. |
| 239 | What if developers cannot reproduce a bad graph answer? | Observability and replay systems preserve inputs. | Every answer includes `answer_run_id`; CLI can replay answer from DB/snapshot state. |
| 240 | What implementation order reduces risk most? | Connector/search/eval systems fail when built live-first. | Build order stays: schema -> synthetic -> eval -> query -> HTTP -> offline snapshots -> bounded live crawl -> Swift. |

## Design Additions

### Connector Capability Manifest

Every source connector should declare:

```text
source_name
source_instance_scope
permission_model = source_acl | global | unknown
supports_incremental
cursor_type = timestamp | delta_token | page_token | webhook_hint | none
supports_deletes
supports_permission_changes
supports_threads
supports_comments
supports_attachments
max_backfill_policy
rate_limit_policy
fixture_coverage
freshness_sla
private_data_policy
```

This lets Cubicle show honest source health and prevents a connector from silently pretending it has complete data.

### Permission Tuple Model

Represent access facts as graph-compatible relationship tuples:

```text
principal_kind
principal_key
relation = viewer | commenter | editor | owner | member | discoverer
object_kind
object_key
source
source_instance
acl_snapshot_key
observed_at
freshness_state
```

Search filtering must use exact permission tuples or derived materialized permission sets, not FTS tokens.

### Product Trust Contract

Every product answer should carry:

```text
answer_run_id
query_scope
source_status_summary
freshness_summary
exact_hits
lexical_hits
graph_hits
evidence_refs
permission_policy_summary
confidence
no_answer_reason
action_candidates
```

The product should be comfortable saying:

```text
I cannot answer that from current evidence.
The GitHub source is fresh, Jira is partial, Slack is unavailable,
and no source contains a decision record.
```

### Graph Promotion Trigger

SQLite remains right for the local POC, but the design must define when it is no longer enough:

```text
graph.db > 2 GB
edge count > 1 million
search index > 500k rows
write contention appears in p95 query latency
more than one concurrent writer is required
remote clients need access
permission checks require shared organizational identity state
```

First promotion target should be Postgres plus Ent and a dedicated search index. A native graph database should be considered only after product queries prove multi-hop traversal is the main bottleneck.

## Sources

- Glean connectors: https://docs.glean.com/connectors/about
- Glean crawler/indexing limits: https://docs.glean.com/connectors/crawler-and-indexing-limits
- Glean Google Drive permissions: https://docs.glean.com/connectors/native/gdrive/security/permissions
- Atlassian Teamwork Graph connectors: https://support.atlassian.com/organization-administration/docs/manage-rovo-connectors/
- Atlassian connector evaluation: https://support.atlassian.com/organization-administration/docs/evaluate-third-party-connectors-in-the-teamwork-graph/
- Atlassian indexed objects: https://support.atlassian.com/organization-administration/docs/what-are-indexed-objects/
- Notion Enterprise Search: https://www.notion.com/help/enterprise-search
- Notion AI Connectors: https://www.notion.com/help/notion-ai-connectors
- Palantir Ontology overview: https://www.palantir.com/docs/foundry/ontology/overview/
- Palantir action rules: https://www.palantir.com/docs/foundry/action-types/rules/
- Palantir markings: https://www.palantir.com/docs/foundry/security/markings/index.html
- Palantir link type metadata: https://www.palantir.com/docs/foundry/object-link-types/link-type-metadata/
- DataHub stateful ingestion: https://docs.datahub.com/docs/metadata-ingestion/docs/dev_guides/stateful
- DataHub lineage: https://docs.datahub.com/docs/features/feature-guides/lineage
- OpenMetadata lineage ingestion: https://docs.open-metadata.org/latest/connectors/ingestion/lineage
- Apache Atlas type system: https://atlas.apache.org/0.8.3/TypeSystem.html
- Apache Atlas overview: https://atlas.apache.org/
- Airbyte checkpointing: https://airbyte.com/blog/checkpointing
- Debezium outbox event router: https://debezium.io/documentation/reference/stable/transformations/outbox-event-router.html
- Microsoft Graph throttling: https://learn.microsoft.com/en-us/graph/throttling
- Microsoft Graph change notifications: https://learn.microsoft.com/en-us/graph/change-notifications-overview
- Microsoft Graph lifecycle notifications: https://learn.microsoft.com/en-us/graph/change-notifications-lifecycle-events
- Microsoft Graph delta query: https://learn.microsoft.com/en-us/graph/delta-query-overview
- OpenFGA concepts: https://openfga.dev/docs/concepts
- OpenFGA usersets: https://openfga.dev/docs/modeling/building-blocks/usersets
- Zanzibar paper: https://pdos.csail.mit.edu/6.824/papers/zanzibar.pdf
- Elasticsearch near-real-time search: https://www.elastic.co/guide/en/elasticsearch/reference/current/near-real-time.html
- Elasticsearch refresh parameter: https://www.elastic.co/docs/reference/elasticsearch/rest-apis/refresh-parameter
- OpenSearch document-level security: https://docs.opensearch.org/2.19/security/access-control/document-level-security/
- JanusGraph indexing: https://docs.janusgraph.org/v0.6/schema/index-management/index-performance/
- Neo4j graph modeling core principles: https://neo4j.com/graphacademy/training-gdm-40/03-graph-data-modeling-core-principles/
- Amazon Neptune docs: https://docs.aws.amazon.com/neptune/
- TruLens RAG triad: https://www.trulens.org/getting_started/core_concepts/rag_triad/
- RAGAS metrics: https://docs.ragas.io/en/latest/concepts/metrics/available_metrics/
- LangChain retrieval: https://docs.langchain.com/oss/python/langchain/retrieval
- OpenAI evaluation best practices: https://platform.openai.com/docs/guides/evaluation-best-practices
- OpenTelemetry semantic conventions: https://opentelemetry.io/docs/concepts/semantic-conventions/
- OpenTelemetry HTTP semantic conventions: https://opentelemetry.io/docs/specs/semconv/http/
- OpenTelemetry logging: https://opentelemetry.io/docs/specs/otel/logs/
- Google SRE monitoring distributed systems: https://sre.google/sre-book/monitoring-distributed-systems/
