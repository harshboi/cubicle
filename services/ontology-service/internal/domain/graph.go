package domain

import "time"

const (
	// VisibilityPublic marks a graph fact that can be shown without a user or
	// group permission check. The Flink POC uses only public data, while future
	// workplace connectors can store restricted facts with a different value.
	VisibilityPublic = "public"

	// FreshnessFresh marks a fact observed by the current crawler pass. Ingest
	// runs can later mark untouched facts stale while keeping historical evidence
	// available for audits and time-aware answers.
	FreshnessFresh = "fresh"
)

// ObjectType is an open ontology object type, not a closed enum.
//
// Meta's TAO model stores typed objects, but the object type vocabulary is owned
// by schema/ontology code instead of the transport DTO itself. Cubicle follows
// that shape: domain accepts any non-empty object type, while internal/ontology
// provides the built-in terms product queries use.
type ObjectType string

// ObjectRef is the stable address of an object in API and store calls.
//
// Stores should use this pair to resolve objects instead of exposing Ent IDs or
// SQLite primary keys to Swift, crawlers, or query services.
type ObjectRef struct {
	// ObjectType tells the store which ontology bucket the key belongs to. It is
	// open-ended so custom connector object types can participate in the graph.
	ObjectType ObjectType `json:"object_type"`

	// Key is the caller-owned natural key, such as "ticket:FLINK-39743". Query
	// code uses it for URLs and stable joins across crawls.
	Key string `json:"key"`
}

// Object is a typed ontology entity that can appear in query responses.
//
// Key is a stable domain key such as "ticket:FLINK-39743", not a database
// primary key. Keeping API keys domain-shaped prevents Swift from coupling to
// Ent IDs or SQLite implementation details.
type Object struct {
	// ObjectType is the ontology type used for routing, filtering, and UI
	// bucketing. Built-ins live in internal/ontology, but custom values are valid.
	ObjectType ObjectType `json:"object_type"`

	// Key is the stable graph key chosen by mapper code. It is unique within the
	// service and should not depend on database IDs.
	Key string `json:"key"`

	// Title is the human-readable label Swift can render in lists, cards, and
	// graph previews without fetching source-specific payloads.
	Title string `json:"title"`

	// Source names the connector or fixture that produced the object, such as
	// "jira", "github", or "fixture".
	Source string `json:"source,omitempty"`

	// SourceInstance identifies the tenant, project, repo, workspace, or list
	// inside the source. It lets two Jira/GitHub instances coexist safely.
	SourceInstance string `json:"source_instance,omitempty"`

	// ExternalID is the source-system identifier, for example a Jira issue key or
	// GitHub repository/PR number. Mappers use it for dedupe and refetches.
	ExternalID string `json:"external_id,omitempty"`

	// SourceURL is the user-openable canonical URL for the object in its source
	// system. Swift can use it for "open in source" actions.
	SourceURL string `json:"source_url,omitempty"`

	// SnapshotKey links this object back to the raw snapshot that produced it.
	// Evidence tracing and replay rely on this value.
	SnapshotKey string `json:"snapshot_key,omitempty"`

	// MapperVersion records which mapper emitted the object. It helps detect
	// whether graph differences came from source data or mapper logic changes.
	MapperVersion string `json:"mapper_version,omitempty"`

	// Visibility is the coarse display policy for the object. The POC defaults
	// to public, while private connectors can store restricted visibility here.
	Visibility string `json:"visibility,omitempty"`

	// FreshnessState records whether the object is current, stale, tombstoned, or
	// otherwise lifecycle-tagged by ingestion.
	FreshnessState string `json:"freshness_state,omitempty"`

	// ObservedAt is when Cubicle observed or derived this object, not necessarily
	// when the source object was last modified.
	ObservedAt time.Time `json:"observed_at,omitempty"`

	// SourceUpdatedAt is the source-system update timestamp when the connector
	// can provide it. Query code can use it to explain staleness.
	SourceUpdatedAt time.Time `json:"source_updated_at,omitempty"`

	// PropertiesJSON stores source- or mapper-specific attributes that are not
	// worth first-class fields yet. Query code should treat it as optional detail.
	PropertiesJSON string `json:"properties_json,omitempty"`
}

// Ref returns the object address used by associations and expansion requests.
//
// Keeping this helper on Object prevents call sites from accidentally dropping
// the object type when they create association endpoints.
func (o Object) Ref() ObjectRef {
	return ObjectRef{ObjectType: o.ObjectType, Key: o.Key}
}

// AssociationType is an open typed relationship name.
//
// A value such as "contains" or "implemented_by" is the association identity,
// equivalent to TAO's association type. Evidence is metadata attached to that
// association, not part of the logical association type.
type AssociationType string

// AssociationMetadata carries evidence and freshness context for a relationship.
//
// Cubicle should not answer "what is blocked?" or "who owns this?" unless the
// association can explain where that fact came from. This metadata is intentionally
// attached to the association DTO even though the logical association identity is
// only from-object, association-type, and to-object.
type AssociationMetadata struct {
	// EvidenceKey points at the source evidence supporting the relationship.
	// Associations should be queryable only when they can explain their origin.
	EvidenceKey string `json:"evidence_key"`

	// Source names the connector or fixture that observed the relationship.
	Source string `json:"source,omitempty"`

	// SourceInstance scopes Source to a repo, workspace, Jira instance, or other
	// source tenant so association provenance remains unambiguous.
	SourceInstance string `json:"source_instance,omitempty"`

	// SourceURL is the source page that best explains the relationship.
	SourceURL string `json:"source_url,omitempty"`

	// SnapshotKey links the association to the raw snapshot used by the mapper.
	SnapshotKey string `json:"snapshot_key,omitempty"`

	// MapperVersion records which mapper version derived the association.
	MapperVersion string `json:"mapper_version,omitempty"`

	// Confidence is the mapper's belief strength from 0 to 1. Rule-based links
	// can use 1, while inferred or LLM-derived links should use lower values.
	Confidence float64 `json:"confidence,omitempty"`

	// Visibility is the coarse display policy for this relationship. It may be
	// stricter than either endpoint object when evidence is permissioned.
	Visibility string `json:"visibility,omitempty"`

	// FreshnessState records whether this relationship is current, stale, or
	// otherwise lifecycle-tagged by ingestion.
	FreshnessState string `json:"freshness_state,omitempty"`

	// ObservedAt is when Cubicle observed or derived the relationship.
	ObservedAt time.Time `json:"observed_at,omitempty"`

	// SourceUpdatedAt is the source-system update time if available.
	SourceUpdatedAt time.Time `json:"source_updated_at,omitempty"`

	// PropertiesJSON stores optional relationship-specific details, such as a
	// matched issue key or source object type.
	PropertiesJSON string `json:"properties_json,omitempty"`
}

// Association is a typed relationship between two ontology objects.
//
// In TAO terms, From + AssociationType + To is the logical association. Metadata
// explains why Cubicle believes the association exists.
type Association struct {
	// Key is the store-level dedupe key. Callers may omit it and let the store
	// derive one from the object endpoints and association type.
	Key string `json:"key,omitempty"`

	// From is the source object of the directed association.
	From ObjectRef `json:"from"`

	// To is the target object of the directed association.
	To ObjectRef `json:"to"`

	// AssociationType names the relationship, such as "contains" or
	// "implemented_by". It is open-ended for future connector-specific links.
	AssociationType AssociationType `json:"association_type"`

	// Metadata carries evidence, visibility, freshness, and source provenance
	// for the association.
	Metadata AssociationMetadata `json:"metadata"`
}

// ExpandRequest asks the store for a bounded association neighborhood.
//
// Depth and LimitPerObject are required because workplace graphs quickly develop
// high-degree objects such as projects, teams, and popular documents. Every graph
// query must make fan-out explicit.
type ExpandRequest struct {
	// Start is the object whose neighborhood the caller wants to inspect.
	Start ObjectRef `json:"start"`

	// AssociationTypes optionally restricts traversal to specific relationship
	// types. Empty means "allow every association type".
	AssociationTypes []AssociationType `json:"association_types,omitempty"`

	// Depth is the maximum number of association hops from Start. Zero returns
	// only the start object.
	Depth int `json:"depth"`

	// LimitPerObject caps fan-out from every object visited during expansion.
	LimitPerObject int `json:"limit_per_object"`
}

// Neighborhood is the bounded graph slice returned by expansion APIs.
//
// Objects are returned once, and Associations explain how those objects connect.
type Neighborhood struct {
	// Objects contains the unique objects reached during traversal, including
	// the start object.
	Objects []Object `json:"objects"`

	// Associations contains the unique directed associations traversed while
	// building the neighborhood.
	Associations []Association `json:"associations"`
}
