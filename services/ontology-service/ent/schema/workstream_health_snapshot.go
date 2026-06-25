package schema

import (
	"entgo.io/ent"
	entschema "entgo.io/ent/schema"
	"entgo.io/ent/schema/edge"
	"entgo.io/ent/schema/field"
	"entgo.io/ent/schema/index"
)

// WorkstreamHealthSnapshot records a generated TPM operating snapshot.
//
// Workstream stays the durable product area. This row is the replayable,
// evidence-backed operating summary for a specific analytics run: action load,
// validation debt, forecast readiness, source coverage risk, and cadence focus.
type WorkstreamHealthSnapshot struct {
	ent.Schema
}

// Annotations declares WorkstreamHealthSnapshot as a public operating view.
func (WorkstreamHealthSnapshot) Annotations() []entschema.Annotation {
	return graphqlAnnotations()
}

// Fields defines a bounded standup/operating health summary for a workstream.
func (WorkstreamHealthSnapshot) Fields() []ent.Field {
	return appendFields(
		stableKeyFields(),
		[]ent.Field{
			field.Int("workstream_id").
				Optional().
				Comment("Optional Workstream row this operating snapshot summarizes."),
			field.String("workstream_key").
				NotEmpty().
				Comment("Stable workstream key summarized by this row."),
			field.Time("generated_at").
				Optional().
				Comment("Analytics generation time for this operating snapshot."),
			field.Enum("operating_status").
				Values(workstreamOperatingStatusValues()...).
				Default(workstreamOperatingUnknown).
				Comment("Overall operating state for TPM cadence."),
			field.Int("action_item_count").
				Default(0).
				Comment("Total open action items represented in the snapshot."),
			field.Int("product_action_count").
				Default(0).
				Comment("Measurement-backed product actions ready for operating follow-up."),
			field.Int("validation_lead_count").
				Default(0).
				Comment("Generated leads that need source or label validation before product escalation."),
			field.Int("critical_or_high_validation_lead_count").
				Default(0).
				Comment("Urgent validation leads that should be reviewed first."),
			field.Int("model_or_rule_qa_count").
				Default(0).
				Comment("Actions about model or rule quality rather than product follow-up."),
			field.Int("closeout_review_count").
				Default(0).
				Comment("Terminal transition closeout reviews awaiting TPM confirmation."),
			field.Int("owner_count").
				Default(0).
				Comment("Distinct assigned owners represented in this snapshot."),
			field.Int("top_owner_action_count").
				Default(0).
				Comment("Largest number of actions assigned to one owner."),
			field.Int("failing_check_pr_count").
				Default(0).
				Comment("PRs with failing or pending checks observed in the analytics run."),
			field.Int("open_failing_check_pr_count").
				Default(0).
				Comment("Open PRs with failing or pending checks that require follow-up."),
			field.Int("source_repair_count").
				Default(0).
				Comment("Items blocked by source coverage repair before product action."),
			field.Int("coverage_limited_count").
				Default(0).
				Comment("Items with partial, failed, or otherwise limited source coverage."),
			field.Int("anonymous_observation_count").
				Default(0).
				Comment("Items observed only through anonymous/public APIs."),
			field.Int("terminal_transition_count").
				Default(0).
				Comment("Terminal state transitions detected in the current time-series window."),
			field.Text("terminal_transition_subjects").
				Optional().
				Comment("Small display list of terminal transition subjects represented by this snapshot."),
			field.Bool("eta_forecast_ready").
				Default(false).
				Comment("Whether forecast evaluation currently supports ETA-style use."),
			field.String("truth_label_coverage").
				Optional().
				Comment("Truth-label coverage numerator/denominator for current generated insights."),
			field.String("actionability_label_coverage").
				Optional().
				Comment("Actionability-label coverage numerator/denominator for current generated insights."),
			field.Text("recommended_cadence_focus").
				Optional().
				Comment("Recommended TPM cadence focus generated from the operating snapshot."),
		},
		sourceIdentityFields(),
		objectEvidenceFields(),
		qualityFields(),
		activityFields(),
		timestampFields(),
	)
}

// Edges connects the snapshot to its workstream and generated evidence.
func (WorkstreamHealthSnapshot) Edges() []ent.Edge {
	return []ent.Edge{
		edge.To("workstream", Workstream.Type).
			Unique().
			Field("workstream_id").
			Comment("Workstream summarized by this operating snapshot."),
		edge.To("latest_evidence", Evidence.Type).
			Unique().
			Field("latest_evidence_id").
			Comment("Most recent generated evidence supporting this operating snapshot."),
	}
}

// Indexes supports newest-first workstream health reads and source identity upserts.
func (WorkstreamHealthSnapshot) Indexes() []ent.Index {
	return []ent.Index{
		index.Fields("workstream_key", "generated_at"),
		index.Fields("operating_status", "generated_at"),
		index.Fields("workstream_id", "generated_at"),
		index.Fields("source_system", "source_instance", "external_kind", "external_id").Unique(),
	}
}
