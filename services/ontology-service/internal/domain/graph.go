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

// ObjectType is an open ontology object type, not a closed enum.
//
// Meta's TAO model stores typed objects, but the object type vocabulary is owned
// by schema/ontology code instead of the transport DTO itself. Cubicle follows
// that shape: domain accepts any non-empty object type, while internal/ontology
// provides the built-in terms product queries use.
type ObjectType string

type ObjectRef struct {
	ObjectType ObjectType `json:"object_type"`
	Key        string     `json:"key"`
}

// Object is a typed ontology entity that can appear in query responses.
//
// Key is a stable domain key such as "ticket:FLINK-39743", not a database
// primary key. Keeping API keys domain-shaped prevents Swift from coupling to
// Ent IDs or SQLite implementation details.
type Object struct {
	ObjectType      ObjectType `json:"object_type"`
	Key             string     `json:"key"`
	Title           string     `json:"title"`
	Source          string     `json:"source,omitempty"`
	SourceInstance  string     `json:"source_instance,omitempty"`
	ExternalID      string     `json:"external_id,omitempty"`
	SourceURL       string     `json:"source_url,omitempty"`
	SnapshotKey     string     `json:"snapshot_key,omitempty"`
	MapperVersion   string     `json:"mapper_version,omitempty"`
	Visibility      string     `json:"visibility,omitempty"`
	FreshnessState  string     `json:"freshness_state,omitempty"`
	ObservedAt      time.Time  `json:"observed_at,omitempty"`
	SourceUpdatedAt time.Time  `json:"source_updated_at,omitempty"`
	PropertiesJSON  string     `json:"properties_json,omitempty"`
}

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

type Association struct {
	Key             string              `json:"key,omitempty"`
	From            ObjectRef           `json:"from"`
	To              ObjectRef           `json:"to"`
	AssociationType AssociationType     `json:"association_type"`
	Metadata        AssociationMetadata `json:"metadata"`
}

// ExpandRequest asks the store for a bounded association neighborhood.
//
// Depth and LimitPerObject are required because workplace graphs quickly develop
// high-degree objects such as projects, teams, and popular documents. Every graph
// query must make fan-out explicit.
type ExpandRequest struct {
	Start            ObjectRef         `json:"start"`
	AssociationTypes []AssociationType `json:"association_types,omitempty"`
	Depth            int               `json:"depth"`
	LimitPerObject   int               `json:"limit_per_object"`
}

type Neighborhood struct {
	Objects      []Object      `json:"objects"`
	Associations []Association `json:"associations"`
}
