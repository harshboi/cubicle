package schema

import (
	"entgo.io/ent"
	entschema "entgo.io/ent/schema"
	"entgo.io/ent/schema/edge"
	"entgo.io/ent/schema/field"
	"entgo.io/ent/schema/index"
)

// OpenGraphObject is the generic connector-backed object escape hatch.
//
// Product-critical object families should still graduate into strongly typed
// schemas once their fields and write authority are understood. This table
// exists to prove the AI-first bounded graph contract can run on non-work
// connector shapes without adding one Ent schema per experiment.
type OpenGraphObject struct {
	ent.Schema
}

// Annotations marks the object as queryable when the Ent GraphQL surface is
// generated in a later slice.
func (OpenGraphObject) Annotations() []entschema.Annotation {
	return graphqlAnnotations()
}

// Fields stores the open object address and shared source/quality metadata.
func (OpenGraphObject) Fields() []ent.Field {
	return appendFields(
		[]ent.Field{
			field.String("object_type").
				NotEmpty().
				Comment("Open ontology object type, such as customer_account, incident, runbook_document, or slack_message."),
			field.String("key").
				NotEmpty().
				Comment("Stable source-neutral key unique within object_type."),
			field.String("title").
				NotEmpty().
				Comment("Human-readable label for graph context and UI display."),
			field.Text("properties_json").
				Optional().
				Comment("Connector-specific attributes that have not earned first-class typed schema."),
		},
		textFields(),
		sourceBackedFields(),
		objectEvidenceFields(),
		qualityFields(),
		activityFields(),
		timestampFields(),
	)
}

// Edges exposes generic association rows and latest object evidence.
func (OpenGraphObject) Edges() []ent.Edge {
	return []ent.Edge{
		edge.From("outgoing_associations", OpenGraphAssociation.Type).
			Ref("from_object").
			Comment("Open graph associations emitted from this object."),
		edge.From("incoming_associations", OpenGraphAssociation.Type).
			Ref("to_object").
			Comment("Open graph associations targeting this object."),
		edge.To("latest_evidence", Evidence.Type).
			Unique().
			Field("latest_evidence_id").
			Comment("Most recent evidence supporting this open graph object."),
	}
}

// Indexes keep object identity source-neutral and scoped by object type.
func (OpenGraphObject) Indexes() []ent.Index {
	return []ent.Index{
		index.Fields("object_type", "key").Unique(),
		index.Fields("object_type", "freshness_state", "rank_score", "last_activity_at"),
		index.Fields("source_system", "source_instance", "external_kind", "external_id"),
	}
}
