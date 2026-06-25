package schema

import (
	"entgo.io/ent"
	entschema "entgo.io/ent/schema"
	"entgo.io/ent/schema/edge"
	"entgo.io/ent/schema/field"
	"entgo.io/ent/schema/index"
)

// OpenGraphAssociation is a generic connector-backed relationship row.
//
// It uses open association names while preserving the same evidence, source,
// visibility, freshness, confidence, and ranking metadata required by typed
// product relationship rows.
type OpenGraphAssociation struct {
	ent.Schema
}

// Annotations declares the composite endpoint identity for the open edge row.
func (OpenGraphAssociation) Annotations() []entschema.Annotation {
	return edgeSchemaAnnotations("from_object_id", "to_object_id", "association_type")
}

// Fields stores endpoint foreign keys plus open relationship metadata.
func (OpenGraphAssociation) Fields() []ent.Field {
	return appendFields(
		[]ent.Field{
			field.Int("from_object_id").
				Immutable().
				Comment("Open graph object that emits this relationship."),
			field.Int("to_object_id").
				Immutable().
				Comment("Open graph object targeted by this relationship."),
			field.String("association_type").
				NotEmpty().
				Immutable().
				Comment("Open semantic relationship name represented by this row."),
			evidenceRefField(),
			field.Int("evidence_count").
				Default(0).
				Comment("Number of evidence records known to support this relationship."),
			field.Text("properties_json").
				Optional().
				Comment("Connector-specific relationship attributes that have not earned first-class typed schema."),
		},
		activityFields(),
		sourceBackedFields(),
		qualityFields(),
		timestampFields(),
	)
}

// Edges connects the open relationship row to its endpoints and latest evidence.
func (OpenGraphAssociation) Edges() []ent.Edge {
	return []ent.Edge{
		edge.To("from_object", OpenGraphObject.Type).
			Unique().
			Required().
			Immutable().
			Field("from_object_id").
			Comment("Open graph object that emits this relationship."),
		edge.To("to_object", OpenGraphObject.Type).
			Unique().
			Required().
			Immutable().
			Field("to_object_id").
			Comment("Open graph object targeted by this relationship."),
		edge.To("latest_evidence", Evidence.Type).
			Unique().
			Field("latest_evidence_id").
			Comment("Most recent evidence supporting this open graph relationship."),
	}
}

// Indexes support forward/reverse traversal and deterministic ranked expansion.
func (OpenGraphAssociation) Indexes() []ent.Index {
	return []ent.Index{
		index.Fields("from_object_id", "to_object_id", "association_type").Unique(),
		index.Fields("from_object_id", "freshness_state", "rank_score", "last_activity_at"),
		index.Fields("to_object_id", "freshness_state", "rank_score", "last_activity_at"),
		index.Fields("association_type", "freshness_state", "rank_score", "last_activity_at"),
		index.Fields("source_system", "source_instance", "external_kind", "external_id"),
	}
}
