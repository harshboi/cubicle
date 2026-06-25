package schema

import (
	"entgo.io/ent"
	"entgo.io/ent/dialect/entsql"
	entschema "entgo.io/ent/schema"
	"entgo.io/ent/schema/edge"
	"entgo.io/ent/schema/field"
	"entgo.io/ent/schema/index"
)

// WorkProgramBriefCaveat records a human-visible limitation on an AI-TPM brief.
//
// Caveats are the product-facing guardrails for the brief: they explain which
// parts of the operating readout must remain warning-only, review-only, or
// blocked from autonomous action.
type WorkProgramBriefCaveat struct {
	ent.Schema
}

// Annotations declares WorkProgramBriefCaveat as a public operating view.
func (WorkProgramBriefCaveat) Annotations() []entschema.Annotation {
	return graphqlAnnotations()
}

// Fields defines one persisted brief caveat.
func (WorkProgramBriefCaveat) Fields() []ent.Field {
	return appendFields(
		stableKeyFields(),
		[]ent.Field{
			field.Int("workstream_id").
				Optional().
				Comment("Optional Workstream row this caveat belongs to."),
			field.String("workstream_key").
				NotEmpty().
				Comment("Stable workstream key summarized by this caveat."),
			field.Time("generated_at").
				Optional().
				Comment("Analytics generation time for this caveat."),
			field.String("caveat_key").
				NotEmpty().
				Comment("Stable brief caveat key."),
			field.String("severity").
				NotEmpty().
				Comment("Human-visible caveat severity, such as warning or danger."),
			field.Text("title").
				NotEmpty().
				Comment("Human-readable caveat title."),
			field.Text("detail").
				NotEmpty().
				Comment("Why the caveat applies to this brief."),
			field.Text("recommended_action").
				Optional().
				Comment("What a TPM or automation should do next."),
			field.Text("evidence_ref").
				Optional().
				Comment("Display evidence reference supporting this caveat."),
		},
		sourceIdentityFields(),
		objectEvidenceFields(),
		qualityFields(),
		activityFields(),
		timestampFields(),
	)
}

// Edges connects the caveat to its workstream and evidence.
func (WorkProgramBriefCaveat) Edges() []ent.Edge {
	return []ent.Edge{
		edge.To("workstream", Workstream.Type).
			Unique().
			Field("workstream_id").
			Annotations(entsql.OnDelete(entsql.SetNull)).
			Comment("Workstream this caveat belongs to."),
		edge.To("latest_evidence", Evidence.Type).
			Unique().
			Field("latest_evidence_id").
			Comment("Most recent evidence supporting this caveat row."),
	}
}

// Indexes supports latest caveat reads and source identity upserts.
func (WorkProgramBriefCaveat) Indexes() []ent.Index {
	return []ent.Index{
		index.Fields("workstream_key", "generated_at", "caveat_key"),
		index.Fields("severity", "generated_at"),
		index.Fields("workstream_id", "generated_at"),
		index.Fields("source_system", "source_instance", "external_kind", "external_id").Unique(),
	}
}
