package domain

import "time"

const (
	// VisibilityPublic is the default for open-source POC data. Later private
	// workplace connectors can use this same field without changing the API.
	VisibilityPublic = "public"

	// FreshnessFresh marks facts produced by the current crawler pass. Ingest
	// runs can later mark untouched facts stale without deleting historical
	// evidence.
	FreshnessFresh = "fresh"
)

type Kind string

const (
	KindWorkstream       Kind = "workstream"
	KindTicket           Kind = "ticket"
	KindPullRequest      Kind = "pull_request"
	KindCodeFile         Kind = "code_file"
	KindDocument         Kind = "document"
	KindDocumentFragment Kind = "document_fragment"
	KindMessage          Kind = "message"
	KindDecision         Kind = "decision"
	KindBlocker          Kind = "blocker"
	KindRisk             Kind = "risk"
	KindPerson           Kind = "person"
	KindTeam             Kind = "team"
	KindActionCandidate  Kind = "action_candidate"
)

type Predicate string

const (
	PredicateContains      Predicate = "contains"
	PredicateHasComponent  Predicate = "has_component"
	PredicateImplementedBy Predicate = "implemented_by"
	PredicateChangesFile   Predicate = "changes_file"
	PredicateDiscussedIn   Predicate = "discussed_in"
	PredicateDocuments     Predicate = "documents"
	PredicateSupports      Predicate = "supports"
	PredicateBlockedBy     Predicate = "blocked_by"
	PredicateOwnedBy       Predicate = "owned_by"
	PredicateNeedsAction   Predicate = "needs_action"
	PredicateEvidencedBy   Predicate = "evidenced_by"
)

type NodeRef struct {
	Kind Kind   `json:"kind"`
	Key  string `json:"key"`
}

// Node is an ontology object that can appear in graph query responses.
//
// The key is a stable domain key such as "ticket:FLINK-39743", not a database
// primary key. Keeping API keys domain-shaped prevents Swift from coupling to
// Ent IDs or SQLite implementation details.
type Node struct {
	Kind            Kind      `json:"kind"`
	Key             string    `json:"key"`
	Title           string    `json:"title"`
	Source          string    `json:"source,omitempty"`
	SourceInstance  string    `json:"source_instance,omitempty"`
	ExternalID      string    `json:"external_id,omitempty"`
	SourceURL       string    `json:"source_url,omitempty"`
	SnapshotKey     string    `json:"snapshot_key,omitempty"`
	MapperVersion   string    `json:"mapper_version,omitempty"`
	Visibility      string    `json:"visibility,omitempty"`
	FreshnessState  string    `json:"freshness_state,omitempty"`
	ObservedAt      time.Time `json:"observed_at,omitempty"`
	SourceUpdatedAt time.Time `json:"source_updated_at,omitempty"`
	PropertiesJSON  string    `json:"properties_json,omitempty"`
}

func (n Node) Ref() NodeRef {
	return NodeRef{Kind: n.Kind, Key: n.Key}
}

// EdgeMetadata carries the evidence and freshness context for a relationship.
//
// Cubicle should not answer "what is blocked?" or "who owns this?" unless the
// edge can explain where that fact came from. This is why metadata lives on the
// edge itself instead of being an afterthought in a separate log.
type EdgeMetadata struct {
	Predicate       Predicate `json:"predicate"`
	EvidenceKey     string    `json:"evidence_key"`
	Source          string    `json:"source,omitempty"`
	SourceInstance  string    `json:"source_instance,omitempty"`
	SourceURL       string    `json:"source_url,omitempty"`
	SnapshotKey     string    `json:"snapshot_key,omitempty"`
	MapperVersion   string    `json:"mapper_version,omitempty"`
	Confidence      float64   `json:"confidence,omitempty"`
	Visibility      string    `json:"visibility,omitempty"`
	FreshnessState  string    `json:"freshness_state,omitempty"`
	ObservedAt      time.Time `json:"observed_at,omitempty"`
	SourceUpdatedAt time.Time `json:"source_updated_at,omitempty"`
	PropertiesJSON  string    `json:"properties_json,omitempty"`
}

type Edge struct {
	Key      string       `json:"key,omitempty"`
	From     NodeRef      `json:"from"`
	To       NodeRef      `json:"to"`
	Metadata EdgeMetadata `json:"metadata"`
}

// ExpandRequest asks the graphstore for a bounded neighborhood.
//
// Depth and LimitPerNode are required because workplace graphs quickly develop
// high-degree nodes such as projects, teams, and popular documents. Every graph
// query must make that fan-out explicit.
type ExpandRequest struct {
	Start        NodeRef     `json:"start"`
	Predicates   []Predicate `json:"predicates,omitempty"`
	Depth        int         `json:"depth"`
	LimitPerNode int         `json:"limit_per_node"`
}

type Neighborhood struct {
	Nodes []Node `json:"nodes"`
	Edges []Edge `json:"edges"`
}
