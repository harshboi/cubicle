package schema

import (
	"entgo.io/ent"
	"entgo.io/ent/dialect/entsql"
	entschema "entgo.io/ent/schema"
	"entgo.io/ent/schema/edge"
	"entgo.io/ent/schema/field"
	"entgo.io/ent/schema/index"
)

// WorkProgramRun records one generated AI-TPM operating run boundary.
//
// The run is the durable cut used by product packets and validation reports to
// avoid mixing quality gates, evidence needs, actions, and snapshots from
// different analytics generations.
type WorkProgramRun struct {
	ent.Schema
}

// Annotations declares WorkProgramRun as a public operating view.
func (WorkProgramRun) Annotations() []entschema.Annotation {
	return graphqlAnnotations()
}

// Fields defines the persisted operating run summary.
func (WorkProgramRun) Fields() []ent.Field {
	return appendFields(
		[]ent.Field{
			field.String("key").
				StorageKey("run_key").
				NotEmpty().
				Unique().
				Comment("Stable source-neutral run key; stored as run_key for replay compatibility."),
			field.Int("workstream_id").
				Optional().
				Comment("Optional Workstream row summarized by this run."),
			field.String("workstream_key").
				NotEmpty().
				Comment("Stable workstream key summarized by this run."),
			field.Time("generated_at").
				Optional().
				Comment("Analytics generation time that defines the run boundary."),
			field.String("readiness_state").
				NotEmpty().
				Comment("Top-level AI-TPM readiness state for this run."),
			field.Float("readiness_score").
				Default(0).
				Comment("Numeric readiness score persisted for run comparisons."),
			field.Bool("autonomous_action_ready").
				Default(false).
				Comment("Whether this run claims autonomous action is ready."),
			field.Bool("human_review_required").
				Default(true).
				Comment("Whether this run still requires human TPM review."),
			field.Int("blocking_gate_count").
				Default(0).
				Comment("Number of blocking quality gates in this run."),
			field.Int("evidence_need_count").
				Default(0).
				Comment("Number of evidence needs in this run."),
			field.Int("tpm_function_count").
				Default(0).
				Comment("Number of TPM function readiness rows in this run."),
			field.Int("quality_gate_count").
				Default(0).
				Comment("Number of quality gates included in this run."),
			field.Int("adversarial_check_count").
				Default(0).
				Comment("Number of adversarial checks included in this run."),
			field.Int("owner_load_snapshot_count").
				Default(0).
				Comment("Number of owner-load snapshots included in this run."),
			field.Int("summary_snapshot_count").
				Default(0).
				Comment("Number of summary snapshots included in this run."),
			field.Int("brief_snapshot_count").
				Default(0).
				Comment("Number of brief snapshots included in this run."),
			field.Int("member_count").
				Default(0).
				Comment("Total number of member rows attached to this run boundary."),
		},
		sourceIdentityFields(),
		objectEvidenceFields(),
		qualityFields(),
		activityFields(),
		timestampFields(),
	)
}

// Edges connects the run to its workstream, members, and evidence.
func (WorkProgramRun) Edges() []ent.Edge {
	return []ent.Edge{
		edge.To("workstream", Workstream.Type).
			Unique().
			Field("workstream_id").
			Annotations(entsql.OnDelete(entsql.SetNull)).
			Comment("Workstream this run summarizes."),
		edge.To("members", WorkProgramRunMember.Type).
			Comment("Rows that belong to this run boundary."),
		edge.To("latest_evidence", Evidence.Type).
			Unique().
			Field("latest_evidence_id").
			Comment("Most recent evidence supporting this run summary."),
	}
}

// Indexes supports latest-run lookup and source identity upserts.
func (WorkProgramRun) Indexes() []ent.Index {
	return []ent.Index{
		index.Fields("source_instance", "workstream_key", "generated_at"),
		index.Fields("workstream_key", "generated_at", "readiness_state"),
		index.Fields("readiness_state", "human_review_required", "generated_at"),
		index.Fields("workstream_id", "generated_at"),
		index.Fields("source_system", "source_instance", "external_kind", "external_id").Unique(),
	}
}
