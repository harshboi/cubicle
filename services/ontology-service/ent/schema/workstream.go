package schema

import (
	"entgo.io/ent"
	entschema "entgo.io/ent/schema"
	"entgo.io/ent/schema/edge"
	"entgo.io/ent/schema/field"
	"entgo.io/ent/schema/index"
)

// Workstream is a project, initiative, launch, or body of related engineering work.
type Workstream struct {
	ent.Schema
}

// Annotations declares that Workstream is part of the future public entgql API.
func (Workstream) Annotations() []entschema.Annotation {
	return graphqlAnnotations()
}

// Fields defines the stable workstream fields used for grouping tickets and
// product execution context.
func (Workstream) Fields() []ent.Field {
	return appendFields(
		stableKeyFields(),
		[]ent.Field{
			field.String("title").
				NotEmpty().
				Comment("Human-readable workstream title."),
			field.Enum("status").
				Values(workstreamStatusUnknown, workstreamStatusActive, workstreamStatusPaused, workstreamStatusDone).
				Default(workstreamStatusUnknown).
				Comment("Operational state of the workstream."),
		},
		textFields(),
		sourceBackedFields(),
		objectEvidenceFields(),
		qualityFields(),
		activityFields(),
		timestampFields(),
	)
}

// Edges connects this source-backed product row to its latest proof.
func (Workstream) Edges() []ent.Edge {
	return []ent.Edge{
		edge.To("latest_evidence", Evidence.Type).
			Unique().
			Field("latest_evidence_id").
			Comment("Most recent evidence supporting this workstream state."),
	}
}

// Indexes supports filtering by status and recency.
func (Workstream) Indexes() []ent.Index {
	return []ent.Index{
		index.Fields("status", "last_activity_at"),
		index.Fields("source_system", "source_instance", "external_kind", "external_id").Unique(),
	}
}
