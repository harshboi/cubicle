package schema

import (
	"entgo.io/ent"
	"entgo.io/ent/dialect/entsql"
	entschema "entgo.io/ent/schema"
	"entgo.io/ent/schema/edge"
	"entgo.io/ent/schema/field"
	"entgo.io/ent/schema/index"
)

// WorkProgramEvidenceNeed records evidence gaps that block AI-TPM automation.
//
// These rows are the durable work queue behind WorkProgramAutomationReadiness:
// what evidence is missing, which gate it blocks, whether an executable action
// already exists, and what a TPM or automation should do next.
type WorkProgramEvidenceNeed struct {
	ent.Schema
}

// Annotations declares WorkProgramEvidenceNeed as a public operating view.
func (WorkProgramEvidenceNeed) Annotations() []entschema.Annotation {
	return graphqlAnnotations()
}

// Fields defines the persisted automation evidence work item.
func (WorkProgramEvidenceNeed) Fields() []ent.Field {
	return appendFields(
		stableKeyFields(),
		[]ent.Field{
			field.Int("workstream_id").
				Optional().
				Comment("Optional Workstream row this evidence need belongs to."),
			field.String("workstream_key").
				NotEmpty().
				Comment("Stable workstream key summarized by this evidence need."),
			field.Time("generated_at").
				Optional().
				Comment("Analytics generation time for this evidence need."),
			field.String("gate_key").
				NotEmpty().
				Comment("Automation quality gate blocked by this evidence need."),
			field.String("evidence_kind").
				NotEmpty().
				Comment("Kind of evidence or operational work required."),
			field.Enum("priority").
				Values(workInsightSeverityValues()...).
				Default(workInsightSeverityMedium).
				Comment("Priority for clearing this evidence need."),
			field.String("target_kind").
				NotEmpty().
				Comment("Kind of target the evidence need applies to."),
			field.String("target_key").
				Optional().
				Comment("Specific target key, when the evidence need is scoped below workstream."),
			field.String("owner_key").
				Optional().
				Comment("Stable owner or owner placeholder responsible for clearing this evidence need."),
			field.String("action_key").
				Optional().
				Comment("Linked WorkAction key that can clear or advance this evidence need."),
			field.Int("work_action_id").
				Optional().
				Comment("Resolved WorkAction row that can clear or advance this evidence need."),
			field.Int("quality_gate_id").
				Optional().
				Comment("Resolved WorkProgramQualityGate row blocked by this evidence need."),
			field.String("action_state").
				Optional().
				Comment("Current state of the linked WorkAction when action_key is set."),
			field.String("metric_key").
				Optional().
				Comment("Metric or source coverage bucket associated with the need."),
			field.String("execution_state").
				NotEmpty().
				Comment("Whether backing actions or rows already make this need executable."),
			field.Int("backing_action_count").
				Default(0).
				Comment("Count of current actions or operating rows backing this evidence need."),
			field.Int("current_count").
				Default(0).
				Comment("Current observed numerator/count for this evidence need."),
			field.Int("required_count").
				Default(0).
				Comment("Required count before the gate can clear."),
			field.Int("missing_count").
				Default(0).
				Comment("Remaining count needed before the gate can clear."),
			field.Float("current_rate").
				Optional().
				Comment("Current measured rate for rate-based evidence needs."),
			field.Float("required_rate").
				Optional().
				Comment("Required rate for rate-based evidence needs."),
			field.Text("recommended_action").
				NotEmpty().
				Comment("What a TPM or automation should do next."),
			field.Text("next_execution_step").
				NotEmpty().
				Comment("Concrete execution step using available backing actions or repair work."),
		},
		sourceIdentityFields(),
		objectEvidenceFields(),
		qualityFields(),
		activityFields(),
		timestampFields(),
	)
}

// Edges connects the evidence need to its workstream and generated evidence.
func (WorkProgramEvidenceNeed) Edges() []ent.Edge {
	return []ent.Edge{
		edge.To("workstream", Workstream.Type).
			Unique().
			Field("workstream_id").
			Annotations(entsql.OnDelete(entsql.SetNull)).
			Comment("Workstream this evidence need belongs to."),
		edge.To("work_action", WorkAction.Type).
			Unique().
			Field("work_action_id").
			Annotations(entsql.OnDelete(entsql.SetNull)).
			Comment("Resolved action that can clear or advance this evidence need."),
		edge.To("quality_gate", WorkProgramQualityGate.Type).
			Unique().
			Field("quality_gate_id").
			Annotations(entsql.OnDelete(entsql.SetNull)).
			Comment("Resolved quality gate this evidence need blocks."),
		edge.To("latest_evidence", Evidence.Type).
			Unique().
			Field("latest_evidence_id").
			Comment("Most recent evidence supporting this evidence need row."),
	}
}

// Indexes supports latest automation-readiness queues and source upserts.
func (WorkProgramEvidenceNeed) Indexes() []ent.Index {
	return []ent.Index{
		index.Fields("workstream_key", "generated_at", "priority"),
		index.Fields("gate_key", "evidence_kind", "generated_at"),
		index.Fields("execution_state", "priority", "generated_at"),
		index.Fields("workstream_key", "action_key", "generated_at"),
		index.Fields("work_action_id", "generated_at"),
		index.Fields("quality_gate_id", "generated_at"),
		index.Fields("workstream_key", "target_key", "generated_at"),
		index.Fields("source_instance", "workstream_key", "action_key", "generated_at"),
		index.Fields("source_instance", "workstream_key", "target_key", "generated_at"),
		index.Fields("workstream_id", "generated_at"),
		index.Fields("source_system", "source_instance", "external_kind", "external_id").Unique(),
	}
}
