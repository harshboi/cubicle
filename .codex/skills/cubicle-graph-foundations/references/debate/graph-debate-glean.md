# Glean Debate Log

## Round 5 - Glean Call-Out

### Opposing Claims Attacked

- Meta claim: "The backend should primarily be typed objects, typed associations, and bounded association lists."
- Palantir claim: "Cubicle is first an operational belief/ontology system, so ingestion, projection, actions, and health are the core."
- Obsidian claim: "Engineer value comes from local graph, backlinks, blocks, and canvas-like exploration."

### What They Are Dangerously Wrong About

Meta is dangerously wrong if it thinks explicit associations are enough. Bounded association lists are a serving discipline, not a discovery system. Cubicle's highest-value evidence will often be in unlinked paragraphs, Slack replies, PR comments, and doc sections that no ontology edge knows about yet. A graph that only follows known edges will look clean while missing the answer.

Palantir is dangerously wrong if it makes Cubicle build a miniature enterprise data platform before users can retrieve useful evidence. Ingestion runs, projection history, health checks, and actions matter, but they are support systems. Without retrieval over content chunks, Palantir-style trust machinery can prove that Cubicle faithfully ingested data the user still cannot find.

Obsidian is dangerously wrong if it treats navigability as correctness. Backlinks and local graph views are excellent surfaces, but they do not solve source permissions, stale crawls, duplicate identities, renamed Jira issues, or ranking. A pretty local graph can be confidently incomplete.

### Failure Scenario

User asks: "Why is launch blocked?"

The real answer is in a Google Doc paragraph that says the launch is blocked by an OAuth config migration. The doc does not link to the Jira ticket. The Jira ticket only says "blocked by platform dependency." Slack has a thread with engineers discussing the same config issue, but the Slack crawl partially failed yesterday.

Meta-only fails because there is no explicit edge from the ticket to the paragraph, so bounded traversal never reaches the evidence.

Palantir-only fails because an ingestion/projection system can say which crawls are fresh or failed, but it still needs retrieval to find the relevant paragraph and Slack snippet.

Obsidian-only fails because backlinks/local graph only surface human-created links. The missing edge is precisely the problem.

Glean-style retrieval handles the scenario by searching chunks first, permission/freshness filtering the candidates, ranking by recency/activity, then asking Ent to explain the top candidates through bounded graph expansion.

```text
"why is launch blocked?"
 |
 +-- SearchChunk topK
 |    -> finds unlinked Google Doc paragraph
 |    -> finds Jira ticket language
 |    -> finds Slack snippet if crawl coverage allows it
 |
 +-- Freshness / permission filter
 |    -> marks Slack partial
 |    -> keeps source visibility attached
 |
 +-- Ent expansion
 |    -> Ticket -> PullRequest -> Workstream
 |
 +-- Context response
      -> answerable evidence, graph path, freshness warning
```

### Exact Next Cubicle PR I Demand

PR #68 should be: **Add SearchDocument and SearchChunk retrieval sidecar**.

Purpose: make unlinked workplace evidence discoverable without corrupting the typed Ent ontology or pretending every useful relationship already exists as an edge.

Schemas:

- `SearchDocument`
  - `id` // local search document id
  - `source` // source system such as github, jira, slack, google_docs
  - `source_instance` // workspace/repo/site/account namespace
  - `external_id` // source-owned document/message/thread/file id
  - `object_kind` // optional Ent target kind when known
  - `object_id` // optional Ent target id when known
  - `title` // display title for result cards
  - `url` // source URL when available
  - `visibility_hash` // stable representation of source visibility/ACL state
  - `freshness_state` // fresh, stale, partial, unknown
  - `last_indexed_at` // when Cubicle indexed the document

- `SearchChunk`
  - `id` // local chunk id
  - `search_document_id` // owning search document
  - `chunk_ordinal` // stable order within the document
  - `source_span` // source-local paragraph/line/thread span
  - `text_hash` // dedupe/change detection for chunk text
  - `lexical_text` // searchable chunk text for the POC
  - `object_kind` // optional Ent target kind when the chunk maps directly
  - `object_id` // optional Ent target id when the chunk maps directly
  - `created_at` // local row creation time
  - `updated_at` // local row update time

API:

- `searchChunks(query, first, sourceKinds, freshnessPolicy)`
  - returns chunk id, snippet, document metadata, optional Ent object reference, freshness state, and visibility hash.

Acceptance:

- A seeded Google Doc paragraph with no graph edge can be found by text query.
- Search results can point back to an Ent object when known, but do not require one.
- Results expose freshness and visibility metadata.
- No vector DB, embeddings, or LLM answer generation in this PR.

### What Not To Build

- Do not build a generic object/edge table. Ent typed objects and typed associations stay as the ontology.
- Do not build Cypher, arbitrary path traversal, or a graph query engine. Bounded Ent expansion is enough.
- Do not build Palantir-style actions, workflow writeback, or a full ingestion warehouse yet.
- Do not build an Obsidian canvas/backend block editor yet. Local graph UX can consume chunks later.
- Do not build vector-first RAG. Lexical chunk retrieval plus citations must work before embeddings hide mistakes.

### Backend Decision

Cubicle should use Meta for storage discipline, Palantir for freshness/provenance, Obsidian for inspection UX, and Glean for the next backend move. The next move is retrieval. Without chunk retrieval, the graph can only explain what it already knows. Cubicle needs to find what it does not know yet.

## Round 6 - Final PR #68 Rebuttal

### Opposing PR #68 Choices Attacked

- Meta PR #68 claim: "Build graph source metadata first."
- Palantir PR #68 claim: "Build ontology lifecycle metadata first."
- Obsidian PR #68 claim: "Build DocumentSection and DocumentBlock first."
- Weak Glean variant being rejected: "Build search chunks as a generic/vector-first RAG substrate."

### Final PR #68 Pick

PR #68 must be: **Add permission/freshness-aware lexical SearchDocument and SearchChunk**.

Not generic RAG. Not vectors. Not LLM answers. Not a full indexing platform.

It is the smallest backend move that creates new product capability: Cubicle can find exact unlinked evidence and return it with enough trust metadata to avoid lying.

```text
PR #68
 |
 +-- SearchDocument
 |    -> source-owned searchable item
 |    -> optional link to typed Ent object
 |    -> visibility_hash + freshness_state
 |
 +-- SearchChunk
 |    -> exact searchable paragraph/thread/comment/span
 |    -> lexical_text + text_hash + source_span
 |
 +-- searchChunks(...)
      -> fast evidence retrieval before bounded Ent expansion
```

### Why Metadata-First Is The Wrong First Move

Meta and Palantir are both making the same mistake with different branding: they want to make existing rows safer before Cubicle can find missing evidence.

That is backwards for a POC.

Lifecycle/source metadata helps answer:

```text
"Can I trust this row?"
```

It does not answer:

```text
"Where is the paragraph that explains the blocker?"
```

If the launch blocker is buried in an unlinked Google Doc paragraph, adding `last_seen_at`, `source_updated_at`, `visibility_hash`, and `deleted_at` to `Ticket` still does not create a route to that paragraph. It only makes the already-known graph row better labeled.

Metadata-first is necessary soon, but as PR #68 it is too passive. It improves trust on known objects while leaving the most important day-0 product failure untouched: Cubicle cannot discover relevant text unless an edge already exists.

### Why Document Blocks First Is The Wrong First Move

Obsidian is right that exact blocks are the user-facing unit of meaning. It is wrong that `DocumentSection` and `DocumentBlock` should be the first backend move.

Document blocks are source-shape modeling. Search chunks are retrieval-shape modeling.

Starting with `DocumentBlock` overfits Google Docs and document-like sources. Cubicle also needs to search:

```text
Slack thread replies
PR review comments
Jira comments
GitHub issue bodies
meeting notes
code review summaries
```

Those are not all naturally `Document -> Section -> Block`. Forcing everything into document blocks creates fake document structure before we know what retrieval needs.

Search chunks can later point at Obsidian-style blocks when blocks exist. Blocks should become one source of chunks, not the root abstraction for all workplace evidence.

### Why Naive Search Chunks Are Also Wrong

I am not defending a sloppy Glean PR.

Search chunks without freshness and visibility are dangerous. Vector-first chunks are worse. They create plausible answers over stale or unauthorized text and hide the reason a result was retrieved.

So PR #68 must include the minimum trust contract inside the retrieval sidecar:

```text
SearchDocument
 |
 +-- source
 +-- source_instance
 +-- external_id
 +-- object_kind / object_id
 +-- visibility_hash
 +-- freshness_state
 +-- last_indexed_at

SearchChunk
 |
 +-- search_document_id
 +-- chunk_ordinal
 +-- source_span
 +-- text_hash
 +-- lexical_text
```

That gives Palantir enough freshness vocabulary, Meta enough typed-object attachment, and Obsidian enough exact spans to render later.

### Failure Scenario

User asks: "Why is launch blocked?"

Real source state:

```text
Jira ticket
 |
 +-- says "blocked by platform dependency"
 |
 +-- no explicit link to the real explanation

Google Doc
 |
 +-- paragraph: "OAuth launch is blocked until config migration verification passes"
 |
 +-- no Jira backlink

Slack
 |
 +-- partial crawl
 |
 +-- one stale thread mentions the same migration
```

Metadata-first result:

```text
Ticket row is fresh.
Document row is fresh.
Slack source is partial.

No answer, because the paragraph is still not retrievable.
```

Document-block-first result:

```text
Google Doc has blocks.

No answer, unless there is already a query/index path across blocks, Slack snippets, Jira comments, and PR comments.
```

Correct PR #68 result:

```text
searchChunks("why is launch blocked?", first: 20)
 |
 +-- returns Google Doc paragraph
 +-- returns Jira ticket snippet
 +-- marks Slack candidate as partial/stale
 +-- includes optional Ent object refs
 |
 +-- next step can expand Ent graph around top objects
```

### Concrete Cubicle Backend Decisions

1. Build PR #68 as `SearchDocument` + `SearchChunk` + `searchChunks`.
2. Use SQLite FTS or simple lexical querying only. No embeddings yet.
3. Include `visibility_hash`, `freshness_state`, and `last_indexed_at` on `SearchDocument` in PR #68.
4. Allow chunks to attach to typed Ent objects through optional `object_kind` and `object_id`.
5. Do not add `DocumentSection` / `DocumentBlock` until after search proves which text units matter.
6. Do not add full lifecycle/source metadata across every ontology object until after retrieval has a consumer that benefits from those fields.
7. Keep Ent as the typed graph. Search chunks are evidence candidates, not canonical ontology objects.

### What Not To Build In PR #68

- No generic node/edge tables.
- No Cypher or arbitrary graph traversal.
- No full ingestion/source-record platform.
- No document-block ontology.
- No vector DB.
- No LLM answer generation.
- No activity ranking yet.

Final Glean position: build retrieval first, but make it permission/freshness-aware from day one. Metadata without retrieval labels an incomplete graph. Blocks without retrieval create inspectable structure nobody can find. Retrieval with minimal trust metadata is the only PR #68 that can produce immediate Cubicle product value without corrupting the typed Ent ontology.
