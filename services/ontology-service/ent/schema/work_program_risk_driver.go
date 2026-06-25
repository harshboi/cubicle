package schema

import (
	"entgo.io/ent"
	"entgo.io/ent/dialect/entsql"
	entschema "entgo.io/ent/schema"
	"entgo.io/ent/schema/edge"
	"entgo.io/ent/schema/field"
	"entgo.io/ent/schema/index"
)

// WorkProgramRiskDriver records a ranked cross-source TPM risk driver.
//
// These rows make the "what matters now" part of the AI-TPM brief durable:
// blockers, blocker impacts, dependencies, forecast risks, and owner-load
// pressure are materialized into one ranked operating queue.
type WorkProgramRiskDriver struct {
	ent.Schema
}

// Annotations declares WorkProgramRiskDriver as a public operating view.
func (WorkProgramRiskDriver) Annotations() []entschema.Annotation {
	return graphqlAnnotations()
}

// Fields defines one persisted risk driver row.
func (WorkProgramRiskDriver) Fields() []ent.Field {
	return appendFields(
		stableKeyFields(),
		[]ent.Field{
			field.Int("workstream_id").
				Optional().
				Comment("Optional Workstream row this risk driver belongs to."),
			field.String("workstream_key").
				NotEmpty().
				Comment("Stable workstream key summarized by this risk driver."),
			field.Time("generated_at").
				Optional().
				Comment("Analytics generation time for this risk driver."),
			field.String("driver_key").
				NotEmpty().
				Comment("Stable risk-driver key exposed to product reads."),
			field.String("driver_kind").
				NotEmpty().
				Comment("Risk-driver kind, such as blocker, blocker_impact, dependency, owner_load, or forecast_risk."),
			field.String("subject_kind").
				Optional().
				Comment("Kind of subject this risk driver points at."),
			field.String("subject_key").
				Optional().
				Comment("Subject key this risk driver points at."),
			field.Text("title").
				NotEmpty().
				Comment("Human-readable risk title."),
			field.String("status").
				NotEmpty().
				Comment("Driver-specific status used for filtering and display."),
			field.Text("recommended_action").
				Optional().
				Comment("What a TPM or automation should do next."),
			field.Text("evidence_ref").
				Optional().
				Comment("Display evidence reference supporting this driver."),
			field.Text("badge_keys").
				Optional().
				Comment("Line-delimited badge keys."),
			field.Text("badge_labels").
				Optional().
				Comment("Line-delimited badge labels aligned with badge_keys."),
			field.Text("badge_tones").
				Optional().
				Comment("Line-delimited badge tones aligned with badge_keys."),
			field.Text("badge_details").
				Optional().
				Comment("Line-delimited badge details aligned with badge_keys."),
		},
		sourceIdentityFields(),
		objectEvidenceFields(),
		qualityFields(),
		activityFields(),
		timestampFields(),
	)
}

// Edges connects the risk driver to its workstream and evidence.
func (WorkProgramRiskDriver) Edges() []ent.Edge {
	return []ent.Edge{
		edge.To("workstream", Workstream.Type).
			Unique().
			Field("workstream_id").
			Annotations(entsql.OnDelete(entsql.SetNull)).
			Comment("Workstream this risk driver belongs to."),
		edge.To("latest_evidence", Evidence.Type).
			Unique().
			Field("latest_evidence_id").
			Comment("Most recent evidence supporting this risk driver row."),
	}
}

// Indexes supports latest ranked driver reads and source identity upserts.
func (WorkProgramRiskDriver) Indexes() []ent.Index {
	return []ent.Index{
		index.Fields("workstream_key", "generated_at", "driver_kind"),
		index.Fields("driver_kind", "rank_score", "generated_at"),
		index.Fields("workstream_id", "generated_at"),
		index.Fields("source_system", "source_instance", "external_kind", "external_id").Unique(),
	}
}
