package schema

import (
	"entgo.io/ent"
	"entgo.io/ent/dialect/entsql"
	entschema "entgo.io/ent/schema"
	"entgo.io/ent/schema/edge"
	"entgo.io/ent/schema/field"
	"entgo.io/ent/schema/index"
)

// WorkProgramTPMFunctionReadiness records readiness by TPM responsibility.
//
// The rows answer the replacement question directly: which TPM functions can
// be automated, which can only be assisted, and which still require human
// judgment because a forecast, measurement, source, owner-load, or blocker gate
// remains open.
type WorkProgramTPMFunctionReadiness struct {
	ent.Schema
}

// Annotations declares WorkProgramTPMFunctionReadiness as a public operating view.
func (WorkProgramTPMFunctionReadiness) Annotations() []entschema.Annotation {
	return graphqlAnnotations()
}

// Fields defines the persisted readiness state for one TPM function.
func (WorkProgramTPMFunctionReadiness) Fields() []ent.Field {
	return appendFields(
		stableKeyFields(),
		[]ent.Field{
			field.Int("workstream_id").
				Optional().
				Comment("Optional Workstream row this function readiness belongs to."),
			field.String("workstream_key").
				NotEmpty().
				Comment("Stable workstream key summarized by this readiness row."),
			field.Time("generated_at").
				Optional().
				Comment("Analytics generation time for this readiness row."),
			field.String("function_key").
				NotEmpty().
				Comment("Stable TPM function key."),
			field.String("function_name").
				NotEmpty().
				Comment("Human-readable TPM function name."),
			field.String("readiness_state").
				NotEmpty().
				Comment("Whether this function is automatable, assisted, supervised, blocked, or watch-only."),
			field.String("automation_state").
				NotEmpty().
				Comment("Machine-readable automation mode for this TPM function."),
			field.Bool("human_required").
				Default(true).
				Comment("Whether human review or ownership is required before this function can act autonomously."),
			field.Int("supporting_signal_count").
				Default(0).
				Comment("Count of current typed signals supporting this readiness decision."),
			field.Text("blocking_gate_keys").
				Optional().
				Comment("Line-delimited quality gate keys blocking this function."),
			field.Text("detail").
				NotEmpty().
				Comment("Human-readable readiness rationale."),
			field.Text("recommended_action").
				NotEmpty().
				Comment("Next step for making this TPM function safer or more autonomous."),
		},
		sourceIdentityFields(),
		objectEvidenceFields(),
		qualityFields(),
		activityFields(),
		timestampFields(),
	)
}

// Edges connects the function readiness row to its workstream and evidence.
func (WorkProgramTPMFunctionReadiness) Edges() []ent.Edge {
	return []ent.Edge{
		edge.To("workstream", Workstream.Type).
			Unique().
			Field("workstream_id").
			Annotations(entsql.OnDelete(entsql.SetNull)).
			Comment("Workstream this TPM function readiness belongs to."),
		edge.To("latest_evidence", Evidence.Type).
			Unique().
			Field("latest_evidence_id").
			Comment("Most recent evidence supporting this function readiness row."),
		edge.To("blocking_quality_gates", WorkProgramQualityGate.Type).
			Comment("Quality gates currently blocking this TPM function from autonomous execution."),
	}
}

// Indexes supports latest readiness matrix reads and source identity upserts.
func (WorkProgramTPMFunctionReadiness) Indexes() []ent.Index {
	return []ent.Index{
		index.Fields("workstream_key", "generated_at", "function_key"),
		index.Fields("readiness_state", "human_required", "generated_at"),
		index.Fields("workstream_id", "generated_at"),
		index.Fields("source_system", "source_instance", "external_kind", "external_id").Unique(),
	}
}
