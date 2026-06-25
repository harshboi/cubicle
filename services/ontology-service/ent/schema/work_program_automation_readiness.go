package schema

import (
	"entgo.io/ent"
	"entgo.io/ent/dialect/entsql"
	entschema "entgo.io/ent/schema"
	"entgo.io/ent/schema/edge"
	"entgo.io/ent/schema/field"
	"entgo.io/ent/schema/index"
)

// WorkProgramAutomationReadiness records the aggregate AI-TPM readiness verdict.
//
// Quality gates, evidence needs, and function-readiness rows explain the
// details; this row records the operating answer a product surface needs: what
// can be automated, what still requires humans, and why autonomous action is or
// is not allowed for a workstream run.
type WorkProgramAutomationReadiness struct {
	ent.Schema
}

// Annotations declares WorkProgramAutomationReadiness as a public operating view.
func (WorkProgramAutomationReadiness) Annotations() []entschema.Annotation {
	return graphqlAnnotations()
}

// Fields defines one persisted automation-readiness snapshot.
func (WorkProgramAutomationReadiness) Fields() []ent.Field {
	return appendFields(
		stableKeyFields(),
		[]ent.Field{
			field.Int("workstream_id").
				Optional().
				Comment("Optional Workstream row this readiness snapshot belongs to."),
			field.String("workstream_key").
				NotEmpty().
				Comment("Stable workstream key summarized by this readiness snapshot."),
			field.Time("generated_at").
				Optional().
				Comment("Analytics generation time for this readiness snapshot."),
			field.String("readiness_state").
				NotEmpty().
				Comment("Aggregate automation readiness state."),
			field.Float("readiness_score").
				Default(0).
				Comment("Deterministic readiness score from zero to one hundred."),
			field.Bool("autonomous_action_ready").
				Default(false).
				Comment("Whether autonomous TPM action is allowed without human review."),
			field.Bool("human_review_required").
				Default(true).
				Comment("Whether human review remains required before action."),
			field.Text("safe_automation_areas").
				Optional().
				Comment("Line-delimited automation areas considered safe."),
			field.Text("human_required_areas").
				Optional().
				Comment("Line-delimited areas requiring human judgment."),
			field.Text("rationale").
				NotEmpty().
				Comment("Human-readable readiness rationale."),
			field.Text("required_evidence").
				Optional().
				Comment("Line-delimited evidence required before readiness can improve."),
			field.Text("blocking_gate_keys").
				Optional().
				Comment("Line-delimited quality gate keys currently blocking autonomy."),
			field.Int("quality_gate_count").
				Default(0).
				Comment("Number of quality gates evaluated for this snapshot."),
			field.Int("blocking_gate_count").
				Default(0).
				Comment("Number of blocking quality gates in this snapshot."),
			field.Int("evidence_need_count").
				Default(0).
				Comment("Number of evidence needs associated with this snapshot."),
			field.Int("tpm_function_count").
				Default(0).
				Comment("Number of TPM function-readiness rows associated with this snapshot."),
		},
		sourceIdentityFields(),
		objectEvidenceFields(),
		qualityFields(),
		activityFields(),
		timestampFields(),
	)
}

// Edges connects the snapshot to its workstream and evidence.
func (WorkProgramAutomationReadiness) Edges() []ent.Edge {
	return []ent.Edge{
		edge.To("workstream", Workstream.Type).
			Unique().
			Field("workstream_id").
			Annotations(entsql.OnDelete(entsql.SetNull)).
			Comment("Workstream this readiness snapshot belongs to."),
		edge.To("latest_evidence", Evidence.Type).
			Unique().
			Field("latest_evidence_id").
			Comment("Most recent evidence supporting this readiness snapshot."),
	}
}

// Indexes supports latest readiness reads and source identity upserts.
func (WorkProgramAutomationReadiness) Indexes() []ent.Index {
	return []ent.Index{
		index.Fields("workstream_key", "generated_at", "readiness_state"),
		index.Fields("readiness_state", "human_review_required", "generated_at"),
		index.Fields("workstream_id", "generated_at"),
		index.Fields("source_system", "source_instance", "external_kind", "external_id").Unique(),
	}
}
