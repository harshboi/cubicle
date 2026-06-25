package schema

import (
	"entgo.io/ent"
	"entgo.io/ent/dialect/entsql"
	entschema "entgo.io/ent/schema"
	"entgo.io/ent/schema/edge"
	"entgo.io/ent/schema/field"
	"entgo.io/ent/schema/index"
)

// WorkProgramSummarySnapshot records the aggregate TPM operating state.
//
// Program items, blockers, dependencies, forecasts, and owner-load rows remain
// the typed source of detail. This snapshot persists the aggregate decision
// surface used by workProgramSummary and by the AI-TPM brief.
type WorkProgramSummarySnapshot struct {
	ent.Schema
}

// Annotations declares WorkProgramSummarySnapshot as a public operating view.
func (WorkProgramSummarySnapshot) Annotations() []entschema.Annotation {
	return graphqlAnnotations()
}

// Fields defines one aggregate work-program summary snapshot.
func (WorkProgramSummarySnapshot) Fields() []ent.Field {
	return appendFields(
		stableKeyFields(),
		[]ent.Field{
			field.Int("workstream_id").
				Optional().
				Comment("Optional Workstream row this summary snapshot belongs to."),
			field.String("workstream_key").
				NotEmpty().
				Comment("Stable workstream key summarized by this snapshot."),
			field.Time("generated_at").
				Optional().
				Comment("Analytics generation time for this work-program summary."),
			field.Int("total_count").
				Default(0).
				Comment("Typed program item count behind the summary."),
			field.Int("needs_decision_count").Default(0),
			field.Int("validate_signal_count").Default(0),
			field.Int("ci_failing_count").Default(0),
			field.Int("waiting_review_count").Default(0),
			field.Int("source_repair_count").Default(0),
			field.Int("closed_pending_review_count").Default(0),
			field.Int("model_quality_count").Default(0),
			field.Int("closure_candidate_count").Default(0),
			field.Int("dismissed_count").Default(0),
			field.Int("now_count").Default(0),
			field.Int("high_risk_count").Default(0),
			field.Int("unassigned_count").Default(0),
			field.Int("product_action_count").Default(0),
			field.Int("validation_lead_count").Default(0),
			field.Int("source_coverage_limited_count").Default(0),
			field.String("owner_load_status").
				NotEmpty().
				Comment("Aggregate owner-load state for the current run."),
			field.Int("owner_load_action_count").Default(0),
			field.Int("overloaded_owner_count").Default(0),
			field.Int("attention_owner_count").Default(0),
			field.Int("unassigned_action_count").Default(0),
			field.Int("blocker_count").Default(0),
			field.Int("active_blocker_count").Default(0),
			field.Int("validating_blocker_count").Default(0),
			field.Int("blocker_impact_count").Default(0),
			field.Int("active_blocker_impact_count").Default(0),
			field.Int("dependency_edge_count").Default(0),
			field.Int("blocking_dependency_count").Default(0),
			field.Int("needs_action_dependency_count").Default(0),
			field.String("operating_status").
				NotEmpty().
				Comment("Top-level work-program operating state."),
			field.String("decision_pressure").
				NotEmpty().
				Comment("Top-level product/owner decision pressure."),
			field.String("forecast_state").
				NotEmpty().
				Comment("Forecast readiness state used by the work-program summary."),
			field.Text("primary_risk").
				Optional().
				Comment("Primary aggregate risk driving this summary."),
			field.Text("recommended_focus").
				NotEmpty().
				Comment("Recommended TPM focus for the current cycle."),
			field.Text("capability_gaps").
				Optional().
				Comment("Line-delimited capability gaps limiting autonomous TPM replacement."),
			field.Text("breakdown_dimensions").
				Optional().
				Comment("Line-delimited breakdown dimensions aligned with breakdown_keys/counts."),
			field.Text("breakdown_keys").
				Optional().
				Comment("Line-delimited breakdown keys aligned with breakdown_dimensions/counts."),
			field.Text("breakdown_counts").
				Optional().
				Comment("Line-delimited breakdown counts aligned with breakdown_dimensions/keys."),
		},
		sourceIdentityFields(),
		objectEvidenceFields(),
		qualityFields(),
		activityFields(),
		timestampFields(),
	)
}

// Edges connects the summary snapshot to its workstream and evidence.
func (WorkProgramSummarySnapshot) Edges() []ent.Edge {
	return []ent.Edge{
		edge.To("workstream", Workstream.Type).
			Unique().
			Field("workstream_id").
			Annotations(entsql.OnDelete(entsql.SetNull)).
			Comment("Workstream this summary snapshot belongs to."),
		edge.To("latest_evidence", Evidence.Type).
			Unique().
			Field("latest_evidence_id").
			Comment("Most recent generated evidence supporting this summary snapshot."),
	}
}

// Indexes supports latest summary reads and source identity upserts.
func (WorkProgramSummarySnapshot) Indexes() []ent.Index {
	return []ent.Index{
		index.Fields("workstream_key", "generated_at"),
		index.Fields("operating_status", "decision_pressure", "generated_at"),
		index.Fields("workstream_id", "generated_at"),
		index.Fields("source_system", "source_instance", "external_kind", "external_id").Unique(),
	}
}
