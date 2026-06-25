package schema

import (
	"entgo.io/ent"
	"entgo.io/ent/dialect/entsql"
	entschema "entgo.io/ent/schema"
	"entgo.io/ent/schema/edge"
	"entgo.io/ent/schema/field"
	"entgo.io/ent/schema/index"
)

// WorkOwnerLoadSnapshot records generated TPM owner load for a workstream run.
//
// This is not a generic person profile. It is a bounded operating snapshot:
// how many generated actions are currently routed to an owner, whether those
// actions are product-ready or validation-gated, and what a TPM should do next.
type WorkOwnerLoadSnapshot struct {
	ent.Schema
}

// Annotations declares WorkOwnerLoadSnapshot as a public operating view.
func (WorkOwnerLoadSnapshot) Annotations() []entschema.Annotation {
	return graphqlAnnotations()
}

// Fields defines owner load counts and routing guidance.
func (WorkOwnerLoadSnapshot) Fields() []ent.Field {
	return appendFields(
		stableKeyFields(),
		[]ent.Field{
			field.Int("workstream_id").
				Optional().
				Comment("Optional Workstream row this owner load belongs to."),
			field.Int("person_id").
				Optional().
				Comment("Optional canonical Person row for this owner, when resolved without guessing."),
			field.String("workstream_key").
				NotEmpty().
				Comment("Stable workstream key summarized by this owner load row."),
			field.String("owner_key").
				NotEmpty().
				Comment("Owner or DRI key from the generated TPM action rollup."),
			field.String("owner_display_name").
				Optional().
				Comment("Display name for the owner key, if known."),
			field.Time("generated_at").
				Optional().
				Comment("Analytics generation time for this owner load snapshot."),
			field.Enum("load_status").
				Values(workOwnerLoadStatusValues()...).
				Default(workOwnerLoadUnknown).
				Comment("Overall owner-load state for TPM routing."),
			field.Int("action_count").
				Default(0).
				Comment("Open generated actions currently routed to this owner."),
			field.Int("product_action_count").
				Default(0).
				Comment("Measurement-backed product actions routed to this owner."),
			field.Int("validation_lead_count").
				Default(0).
				Comment("Validation-gated leads routed to this owner."),
			field.Int("model_or_rule_qa_count").
				Default(0).
				Comment("Model or rule quality actions routed to this owner."),
			field.Int("critical_or_high_count").
				Default(0).
				Comment("Critical or high-priority product actions routed to this owner."),
			field.Float("max_priority_score").
				Default(0).
				Comment("Highest generated priority score among the owner's actions."),
			field.Float("avg_priority_score").
				Default(0).
				Comment("Average generated priority score among the owner's actions."),
			field.Int("decision_followup_count").
				Default(0).
				Comment("Decision or owner follow-up actions routed to this owner."),
			field.Int("validate_signal_count").
				Default(0).
				Comment("Signal-validation actions routed to this owner."),
			field.Int("ci_check_followup_count").
				Default(0).
				Comment("CI check follow-up actions routed to this owner."),
			field.Int("review_wait_followup_count").
				Default(0).
				Comment("Review-wait follow-up actions routed to this owner."),
			field.Int("coverage_limited_count").
				Default(0).
				Comment("Actions for this owner affected by source coverage limitations."),
			field.Int("anonymous_observation_count").
				Default(0).
				Comment("Actions for this owner observed through anonymous/public APIs."),
			field.Int("needs_human_review_count").
				Default(0).
				Comment("Actions that still need human review before product escalation."),
			field.String("top_action_type").
				Optional().
				Comment("Most common or most urgent action type for this owner."),
			field.Text("top_subjects").
				Optional().
				Comment("Small display list of top subjects currently routed to this owner."),
			field.Text("recommended_focus").
				Optional().
				Comment("Recommended TPM routing focus for this owner."),
		},
		sourceIdentityFields(),
		objectEvidenceFields(),
		qualityFields(),
		activityFields(),
		timestampFields(),
	)
}

// Edges connects owner load to its workstream, optional person, and evidence.
func (WorkOwnerLoadSnapshot) Edges() []ent.Edge {
	return []ent.Edge{
		edge.To("workstream", Workstream.Type).
			Unique().
			Field("workstream_id").
			Annotations(entsql.OnDelete(entsql.SetNull)).
			Comment("Workstream this owner load belongs to."),
		edge.To("person", Person.Type).
			Unique().
			Field("person_id").
			Annotations(entsql.OnDelete(entsql.SetNull)).
			Comment("Canonical person resolved for this owner key, if known."),
		edge.To("latest_evidence", Evidence.Type).
			Unique().
			Field("latest_evidence_id").
			Comment("Most recent evidence supporting this generated owner load row."),
	}
}

// Indexes supports latest owner-load reads and source identity upserts.
func (WorkOwnerLoadSnapshot) Indexes() []ent.Index {
	return []ent.Index{
		index.Fields("workstream_key", "generated_at", "load_status", "action_count"),
		index.Fields("owner_key", "generated_at"),
		index.Fields("person_id", "generated_at"),
		index.Fields("workstream_id", "generated_at"),
		index.Fields("source_system", "source_instance", "external_kind", "external_id").Unique(),
	}
}
