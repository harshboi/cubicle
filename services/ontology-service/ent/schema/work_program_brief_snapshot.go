package schema

import (
	"entgo.io/ent"
	"entgo.io/ent/dialect/entsql"
	entschema "entgo.io/ent/schema"
	"entgo.io/ent/schema/edge"
	"entgo.io/ent/schema/field"
	"entgo.io/ent/schema/index"
)

// WorkProgramBriefSnapshot records the product-facing AI-TPM brief header.
//
// The rest of the brief is made from typed rows such as program items, blockers,
// gates, risk drivers, and caveats. This snapshot makes the top-level narrative
// durable for one analytics run: status, focus, cadence, and capability gaps.
type WorkProgramBriefSnapshot struct {
	ent.Schema
}

// Annotations declares WorkProgramBriefSnapshot as a public operating view.
func (WorkProgramBriefSnapshot) Annotations() []entschema.Annotation {
	return graphqlAnnotations()
}

// Fields defines one persisted brief snapshot.
func (WorkProgramBriefSnapshot) Fields() []ent.Field {
	return appendFields(
		stableKeyFields(),
		[]ent.Field{
			field.Int("workstream_id").
				Optional().
				Comment("Optional Workstream row this brief snapshot belongs to."),
			field.String("workstream_key").
				NotEmpty().
				Comment("Stable workstream key summarized by this brief snapshot."),
			field.Time("generated_at").
				Optional().
				Comment("Analytics generation time for this brief snapshot."),
			field.String("operating_status").
				NotEmpty().
				Comment("Top-level workstream operating status."),
			field.String("decision_pressure").
				NotEmpty().
				Comment("Top-level decision pressure classification."),
			field.String("forecast_state").
				NotEmpty().
				Comment("Top-level forecast readiness state."),
			field.Text("primary_risk").
				Optional().
				Comment("Primary risk summary for the brief."),
			field.Text("executive_summary").
				NotEmpty().
				Comment("Short operating summary shown at the top of the brief."),
			field.Text("recommended_focus").
				NotEmpty().
				Comment("Primary TPM focus for the current cycle."),
			field.Text("next_cadence_focus").
				NotEmpty().
				Comment("Recommended focus for the next TPM cadence."),
			field.Text("capability_gaps").
				Optional().
				Comment("Line-delimited capability gaps that limit autonomous TPM replacement."),
			field.Int("total_count").
				Default(0).
				Comment("Typed program item count behind the snapshot."),
			field.Int("product_action_count").
				Default(0).
				Comment("Program items classified as product actions."),
			field.Int("validation_lead_count").
				Default(0).
				Comment("Program items classified as validation leads."),
			field.Int("source_coverage_limited_count").
				Default(0).
				Comment("Program items with limited source coverage."),
			field.Int("active_blocker_count").
				Default(0).
				Comment("Active blocker count in scope."),
			field.Int("active_blocker_impact_count").
				Default(0).
				Comment("Active blocker impact count in scope."),
			field.Int("needs_action_dependency_count").
				Default(0).
				Comment("Dependency edges that need action."),
			field.Int("overloaded_owner_count").
				Default(0).
				Comment("Overloaded owner count in latest owner-load snapshot."),
			field.Int("unassigned_action_count").
				Default(0).
				Comment("Unassigned action count in latest owner-load snapshot."),
			field.Int("quality_gate_count").
				Default(0).
				Comment("Quality gates evaluated for this snapshot."),
			field.Int("blocking_gate_count").
				Default(0).
				Comment("Blocking quality gates for this snapshot."),
			field.Int("caveat_count").
				Default(0).
				Comment("Visible caveats for this snapshot."),
			field.Int("risk_driver_count").
				Default(0).
				Comment("Persisted risk drivers for this snapshot."),
		},
		sourceIdentityFields(),
		objectEvidenceFields(),
		qualityFields(),
		activityFields(),
		timestampFields(),
	)
}

// Edges connects the snapshot to its workstream and evidence.
func (WorkProgramBriefSnapshot) Edges() []ent.Edge {
	return []ent.Edge{
		edge.To("workstream", Workstream.Type).
			Unique().
			Field("workstream_id").
			Annotations(entsql.OnDelete(entsql.SetNull)).
			Comment("Workstream this brief snapshot belongs to."),
		edge.To("latest_evidence", Evidence.Type).
			Unique().
			Field("latest_evidence_id").
			Comment("Most recent evidence supporting this brief snapshot row."),
	}
}

// Indexes supports latest brief reads and source identity upserts.
func (WorkProgramBriefSnapshot) Indexes() []ent.Index {
	return []ent.Index{
		index.Fields("workstream_key", "generated_at"),
		index.Fields("operating_status", "decision_pressure", "generated_at"),
		index.Fields("workstream_id", "generated_at"),
		index.Fields("source_system", "source_instance", "external_kind", "external_id").Unique(),
	}
}
