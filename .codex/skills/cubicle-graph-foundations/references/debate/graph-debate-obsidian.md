## Round 5 - Obsidian Call-Out

I am attacking these opposing claims from `graph-debate-live.md`:

- Meta claim: "serving/storage discipline: typed objects, typed associations, bounded reads"
- Palantir claim: "trust/provenance: source records, projection, health, actions later"
- Glean claim: "retrieval: chunks, search, activity, permissions, answer grounding"

### What They Are Dangerously Wrong About

Meta is dangerously wrong if it thinks bounded association lists are a product model. They are a serving model. A fast list of `Ticket -> PullRequest -> Message` edges does not explain the work. Engineers do not debug launches by reading adjacency lists. They debug by finding the exact paragraph, thread, decision, and stale assumption that created the blocker.

Palantir is dangerously wrong if it thinks Cubicle should earn trust by building an ingestion/projection/health platform before it has an inspectable knowledge surface. Provenance without a usable local context map is compliance metadata, not product value. A perfect `IngestionRun` table does not answer "where was this decision made?"

Glean is dangerously wrong if it thinks retrieval should be the center of Cubicle. Search is an entry point, not the graph. A ranked answer that hides topology creates a black box. Cubicle must show the object, the linked blocks, the backlinks, the source health, and the reason a result is adjacent to the user's work.

### Failure Scenario

User asks: "Why is launch blocked?"

Meta-only failure:

```text
Workstream
 |
 +-- blocker tickets
 +-- related pull requests
 +-- related messages
```

This is fast but shallow. It returns objects, not the exact reason. The blocker may live in one paragraph inside a Google Doc saying "rollout blocked until checkpoint migration is verified." Without block-level modeling, Meta gives the user a pile of adjacent objects.

Palantir-only failure:

```text
IngestionRun complete
ProjectionRun complete
Source health green
```

This is trustworthy but still not an answer. The user did not ask whether the crawl succeeded. They asked why the launch is blocked. Trust metadata must attach to an inspectable block, decision, and backlink path.

Glean-only failure:

```text
Top result:
"checkpoint migration is verified..."
```

This is useful but not enough. Why is that paragraph connected to the launch? Which ticket owns it? Who is blocking it? Is the Slack evidence stale? Search without local graph context turns Cubicle into a snippet machine.

Obsidian-style answer:

```text
Launch Workstream
 |
 +-- blocker Ticket
 |    |
 |    +-- linked PR
 |
 +-- Google Doc section: "Migration readiness"
 |    |
 |    +-- exact block: "rollout blocked until checkpoint migration is verified"
 |
 +-- Slack thread
 |    |
 |    +-- owner says verification is still pending
 |
 +-- source health
      |
      +-- docs fresh
      +-- slack partial
```

That is the product. It is not just typed edges, not just provenance, and not just search.

### Exact Next Cubicle PR I Demand

Build `PR #68: Add document section and block ontology`.

Scope:

- Add `DocumentSection`
- Add `DocumentBlock`
- Link `Document -> DocumentSection -> DocumentBlock`
- Add stable block identity fields:
  - `source`
  - `source_instance`
  - `external_id`
  - `document_id`
  - `heading_path`
  - `ordinal`
  - `text_hash`
  - `source_updated_at`
  - `ingested_at`
  - `last_seen_at`
- Add GraphQL query:

```graphql
documentBlocks(documentID: ID!, first: Int, after: String) {
  nodes {
    id
    headingPath
    ordinal
    textPreview
    textHash
    freshnessState
  }
}
```

Why this PR:

- It gives Cubicle the smallest Obsidian-native unit of meaning: the block.
- It lets future backlinks, mention candidates, evidence, local graph, and search attach to exact text instead of whole documents.
- It still respects Meta because reads are bounded and paginated.
- It still respects Palantir because each block carries source/freshness metadata.
- It still respects Glean because blocks become the future retrieval unit.

### What Not To Build

Do not build a generic node/edge table. It will erase the typed Ent shape and make reviews worse.

Do not build Cypher or arbitrary graph traversal. Cubicle is local and day-0; bounded GraphQL queries over typed Ent edges are enough.

Do not build vector-first RAG. Vectors over whole documents will hide the exact paragraph that matters. Blocks first, retrieval later.

Do not build a full Palantir-style ingestion platform before block-level context exists. Source health matters, but provenance without inspectable context is not a product loop.

## Round 6 - Final PR #68 Rebuttal

I am attacking these competing PR #68 choices from `graph-debate-live.md`:

- Meta: `PR #68 = graph source metadata`
- Palantir: `PR #68 = ontology lifecycle metadata`
- Glean: `PR #68 = SearchDocument + SearchChunk`

Final Obsidian position: `PR #68 = DocumentSection + DocumentBlock`.

### Why Metadata-First Is Wrong As The First Move

Meta and Palantir are hiding an empty product loop behind respectable fields.

Lifecycle/source metadata is necessary, but building it first does not create a new inspectable unit of knowledge. It tells us whether a `Document` row is stale, but the user does not care about a stale document row. The user cares about the exact paragraph that says why launch is blocked. A whole document with `last_seen_at`, `source_updated_at`, and `visibility_hash` is still too blunt.

Metadata-first failure:

```text
Document
 |
 +-- source_updated_at
 +-- ingested_at
 +-- last_seen_at
 +-- visibility_hash
 |
 `-- 30 pages of launch plan
```

User asks: "Why is launch blocked?"

Metadata answers: "The document is fresh."

That is not an answer. It is a timestamp on a haystack.

The correct compromise is not to reject metadata. The correct compromise is to attach minimal source lifecycle fields to the first meaningful knowledge unit:

```text
Document
 |
 +-- DocumentSection
      |
      +-- DocumentBlock
           |
           +-- heading_path
           +-- ordinal
           +-- text_hash
           +-- source_updated_at
           +-- ingested_at
           +-- last_seen_at
```

Metadata belongs on blocks now. Blanket graph metadata can follow after there is something precise enough to inspect.

### Why SearchChunk-First Is Wrong As The First Move

Glean is trying to create a parallel content model before Cubicle has a canonical document anatomy.

`SearchDocument` and `SearchChunk` look practical, but as PR #68 they create a retrieval sidecar that will immediately compete with the ontology:

```text
Document ontology path           Search sidecar path
 |                               |
 +-- Document                    +-- SearchDocument
                                  |
                                  +-- SearchChunk
```

Now Cubicle has two answers to "what is this paragraph?":

- the future `DocumentBlock`
- today's `SearchChunk`

That is schema debt on day zero. Search chunks should index canonical blocks, not become the canonical block model by accident. Otherwise every backlink, mention candidate, evidence edge, local graph node, and citation has to decide whether it points to `DocumentBlock` or `SearchChunk`.

Search-first failure:

```text
search("why is launch blocked")
 |
 +-- SearchChunk hit
      |
      +-- optional object_kind
      +-- optional object_id
```

This finds text but cannot cleanly participate in the graph. Optional object references are not graph structure. They are a weak pointer out of a sidecar.

The right order is:

```text
DocumentBlock
 |
 +-- later indexed by SearchChunk/SearchIndex
 |
 +-- linked by KnowledgeLink
 |
 +-- cited by Evidence
 |
 +-- rendered in local graph
```

Glean gets its retrieval unit, but it must be a projection over blocks, not a competing source of truth.

### Why Document Blocks Are The Correct PR #68

Blocks are not "UX." Blocks are addressability.

Without blocks, Cubicle cannot model the smallest work unit engineers actually cite:

- one paragraph in a Google Doc
- one heading in a design doc
- one quoted decision
- one launch-risk sentence
- one unlinked mention of a Jira issue

PR #67 gave Cubicle typed objects, bounded windows, Ent runtime, and GraphQL. The next missing primitive is not more labels and not search. It is the exact unit that labels and search should attach to.

Correct PR #68:

```text
Document
 |
 +-- DocumentSection
      |
      +-- DocumentBlock
           |
           +-- future KnowledgeLink
           +-- future MentionCandidate
           +-- future SearchChunk index row
           +-- future Evidence citation
```

This is the only first move that satisfies all four camps without letting any one camp distort the architecture:

```text
Meta constraint
 |
 +-- bounded reads:
      Document -> sections(first:N) -> blocks(first:N)

Palantir constraint
 |
 +-- provenance on block:
      source_updated_at, ingested_at, last_seen_at, text_hash

Glean constraint
 |
 +-- future retrieval anchor:
      search indexes DocumentBlock, not raw duplicate chunks

Obsidian constraint
 |
 +-- product surface:
      exact paragraphs, backlinks, local graph, context maps
```

### Final PR #68 Demand

Build `PR #68: Add document section and block ontology`.

Schemas:

- `DocumentSection`
- `DocumentBlock`

Edges:

```text
Document
 |
 +-- sections -> DocumentSection
      |
      +-- blocks -> DocumentBlock
```

Fields:

- `document_id` // owning document
- `parent_section_id` // optional parent section for nested headings
- `heading_path` // stable human-readable heading ancestry
- `section_ordinal` // section order inside document
- `block_ordinal` // block order inside section/document
- `text_preview` // short display text, not full indexing strategy
- `text_hash` // stable change detection
- `source`
- `source_instance`
- `external_id`
- `source_updated_at`
- `ingested_at`
- `last_seen_at`

GraphQL:

```graphql
documentBlocks(documentID: ID!, first: Int, after: String) {
  nodes {
    id
    headingPath
    blockOrdinal
    textPreview
    textHash
    sourceUpdatedAt
    lastSeenAt
  }
}
```

Acceptance:

- A seeded document can expose sections and blocks through GraphQL.
- Reads are paginated.
- Blocks carry freshness metadata.
- No search index, no embeddings, no generic chunk table.

### What Not To Build In PR #68

Do not build blanket lifecycle metadata across every object yet. Add only the metadata needed for blocks to be source-aware.

Do not build `SearchDocument` or `SearchChunk` yet. Search should index canonical `DocumentBlock` rows in a later PR.

Do not build backlinks or mention candidates yet. They need `DocumentBlock` as the target/source first.

Do not build generic object/edge tables, Cypher, arbitrary traversal, workflow actions, vector RAG, or a full ingestion platform.

Concrete backend decisions:

1. PR #68: `DocumentSection` + `DocumentBlock` with minimal source lifecycle fields.
2. PR #69: `KnowledgeLink` + `MentionCandidate` pointing at blocks and existing Ent objects.
3. PR #70: source health/lifecycle mixins across canonical source-backed objects.
4. PR #71: retrieval index over `DocumentBlock`, not a parallel chunk truth store.
