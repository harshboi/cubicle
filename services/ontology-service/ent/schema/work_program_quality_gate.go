package schema

import (
	"entgo.io/ent"
	"entgo.io/ent/dialect/entsql"
	entschema "entgo.io/ent/schema"
	"entgo.io/ent/schema/edge"
	"entgo.io/ent/schema/field"
	"entgo.io/ent/schema/index"
)

// WorkProgramQualityGate records an AI-TPM automation gate.
//
// These rows make the replacement boundary durable: every brief can say which
// automation gates passed, which are blocking, and what evidence must change
// before Cubicle should act autonomously.
type WorkProgramQualityGate struct {
	ent.Schema
}

// Annotations declares WorkProgramQualityGate as a public operating view.
func (WorkProgramQualityGate) Annotations() []entschema.Annotation {
	return graphqlAnnotations()
}

// Fields defines one persisted automation quality gate.
func (WorkProgramQualityGate) Fields() []ent.Field {
	return appendFields(
		stableKeyFields(),
		[]ent.Field{
			field.Int("workstream_id").
				Optional().
				Comment("Optional Workstream row this quality gate belongs to."),
			field.String("workstream_key").
				NotEmpty().
				Comment("Stable workstream key summarized by this quality gate."),
			field.Time("generated_at").
				Optional().
				Comment("Analytics generation time for this quality gate."),
			field.String("gate_key").
				NotEmpty().
				Comment("Stable automation quality gate key."),
			field.String("gate_state").
				NotEmpty().
				Comment("Whether the quality gate passed, is gated, or is otherwise not ready."),
			field.Bool("blocking").
				Default(false).
				Comment("Whether this gate blocks autonomous TPM action."),
			field.Text("detail").
				NotEmpty().
				Comment("Why the gate has its current state."),
			field.Text("recommended_action").
				Optional().
				Comment("What a TPM or automation should do next."),
		},
		sourceIdentityFields(),
		objectEvidenceFields(),
		qualityFields(),
		activityFields(),
		timestampFields(),
	)
}

// Edges connects the gate to its workstream and evidence.
func (WorkProgramQualityGate) Edges() []ent.Edge {
	return []ent.Edge{
		edge.To("workstream", Workstream.Type).
			Unique().
			Field("workstream_id").
			Annotations(entsql.OnDelete(entsql.SetNull)).
			Comment("Workstream this quality gate belongs to."),
		edge.To("latest_evidence", Evidence.Type).
			Unique().
			Field("latest_evidence_id").
			Comment("Most recent evidence supporting this quality gate row."),
		edge.From("blocking_tpm_function_readinesses", WorkProgramTPMFunctionReadiness.Type).
			Ref("blocking_quality_gates").
			Comment("TPM function readiness rows currently blocked by this quality gate."),
	}
}

// Indexes supports latest gate reads and source identity upserts.
func (WorkProgramQualityGate) Indexes() []ent.Index {
	return []ent.Index{
		index.Fields("workstream_key", "generated_at", "gate_key"),
		index.Fields("gate_state", "blocking", "generated_at"),
		index.Fields("workstream_id", "generated_at"),
		index.Fields("source_system", "source_instance", "external_kind", "external_id").Unique(),
	}
}
