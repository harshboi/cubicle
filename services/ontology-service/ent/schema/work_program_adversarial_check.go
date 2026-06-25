package schema

import (
	"entgo.io/ent"
	"entgo.io/ent/dialect/entsql"
	entschema "entgo.io/ent/schema"
	"entgo.io/ent/schema/edge"
	"entgo.io/ent/schema/field"
	"entgo.io/ent/schema/index"
)

// WorkProgramAdversarialCheck records durable AI-TPM claim-safety checks.
//
// The brief can be rendered from typed program rows, but the adversarial checks
// are first-class audit rows: they say which product claims are safe, gated, or
// overclaimed, and they point to the evidence/gates that made that decision.
type WorkProgramAdversarialCheck struct {
	ent.Schema
}

// Annotations declares WorkProgramAdversarialCheck as a public operating view.
func (WorkProgramAdversarialCheck) Annotations() []entschema.Annotation {
	return graphqlAnnotations()
}

// Fields defines the persisted adversarial check result and its evidence links.
func (WorkProgramAdversarialCheck) Fields() []ent.Field {
	return appendFields(
		stableKeyFields(),
		[]ent.Field{
			field.Int("workstream_id").
				Optional().
				Comment("Optional Workstream row this check belongs to."),
			field.String("workstream_key").
				NotEmpty().
				Comment("Stable workstream key summarized by this check."),
			field.Time("generated_at").
				Optional().
				Comment("Analytics generation time for this check."),
			field.String("check_kind").
				NotEmpty().
				Comment("Machine-readable adversarial check category."),
			field.Enum("check_state").
				Values(workProgramAdversarialCheckStateValues()...).
				Default(workProgramAdversarialCheckStateWarning).
				Comment("Result of the adversarial check."),
			field.Enum("severity").
				Values(workInsightSeverityValues()...).
				Default(workInsightSeverityMedium).
				Comment("Severity if the check blocks or qualifies product use."),
			field.Text("title").
				NotEmpty().
				Comment("Human-readable check title."),
			field.Text("detail").
				NotEmpty().
				Comment("Why the check has its current state."),
			field.Text("recommended_action").
				NotEmpty().
				Comment("What a TPM or automation should do next."),
			field.Text("blocking_gate_keys").
				Optional().
				Comment("Line-delimited WorkProgramBriefQualityGate keys that block this check."),
			field.Text("evidence_refs").
				Optional().
				Comment("Line-delimited source or generated evidence refs supporting this check."),
		},
		sourceIdentityFields(),
		objectEvidenceFields(),
		qualityFields(),
		activityFields(),
		timestampFields(),
	)
}

// Edges connects the check to its workstream and generated evidence.
func (WorkProgramAdversarialCheck) Edges() []ent.Edge {
	return []ent.Edge{
		edge.To("workstream", Workstream.Type).
			Unique().
			Field("workstream_id").
			Annotations(entsql.OnDelete(entsql.SetNull)).
			Comment("Workstream this check belongs to."),
		edge.To("latest_evidence", Evidence.Type).
			Unique().
			Field("latest_evidence_id").
			Comment("Most recent evidence supporting this adversarial check result."),
	}
}

// Indexes supports latest workstream brief reads and source identity upserts.
func (WorkProgramAdversarialCheck) Indexes() []ent.Index {
	return []ent.Index{
		index.Fields("workstream_key", "generated_at", "check_kind"),
		index.Fields("check_state", "severity", "generated_at"),
		index.Fields("workstream_id", "generated_at"),
		index.Fields("source_system", "source_instance", "external_kind", "external_id").Unique(),
	}
}
