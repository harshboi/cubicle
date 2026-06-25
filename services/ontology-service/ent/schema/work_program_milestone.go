package schema

import (
	"entgo.io/ent"
	"entgo.io/ent/dialect/entsql"
	entschema "entgo.io/ent/schema"
	"entgo.io/ent/schema/edge"
	"entgo.io/ent/schema/field"
	"entgo.io/ent/schema/index"
)

// WorkProgramMilestone records source-authored milestone and date signals.
//
// These rows deliberately do not use forecast output or generated due buckets as
// commitments. Jira fixVersion release dates are release-target metadata, Jira
// due dates are the stronger commitment-like signal, and resolution dates are
// outcome evidence.
type WorkProgramMilestone struct {
	ent.Schema
}

// Annotations declares WorkProgramMilestone as a public operating view.
func (WorkProgramMilestone) Annotations() []entschema.Annotation {
	annotations := graphqlAnnotations()
	return append(annotations, entsql.Checks(map[string]string{
		"work_program_milestones_subject_pointer_matches_kind":          "((subject_kind = 'pull_request' AND pull_request_id IS NOT NULL AND ticket_id IS NULL) OR (subject_kind = 'ticket' AND ticket_id IS NOT NULL AND pull_request_id IS NULL) OR (subject_kind = 'unknown' AND pull_request_id IS NULL AND ticket_id IS NULL))",
		"work_program_milestones_date_claim_requires_source_date":       "date_claim_allowed = false OR target_date IS NOT NULL OR outcome_date IS NOT NULL",
		"work_program_milestones_delivery_requires_explicit_due_target": "delivery_commitment_allowed = false OR (milestone_kind = 'explicit_due_date' AND commitment_strength = 'explicit_commitment' AND target_date IS NOT NULL)",
		"work_program_milestones_explicit_commitment_requires_due_kind": "commitment_strength != 'explicit_commitment' OR milestone_kind = 'explicit_due_date'",
		"work_program_milestones_resolution_outcome_is_not_commitment":  "milestone_kind != 'resolution_outcome' OR (commitment_strength = 'outcome_evidence' AND delivery_commitment_allowed = false AND target_date IS NULL AND outcome_date IS NOT NULL)",
		"work_program_milestones_release_target_is_not_delivery_commit": "milestone_kind != 'release_target' OR delivery_commitment_allowed = false",
	}))
}

// Fields defines a source-backed milestone/date-signal row.
func (WorkProgramMilestone) Fields() []ent.Field {
	return appendFields(
		stableKeyFields(),
		[]ent.Field{
			field.Int("workstream_id").
				Optional().
				Comment("Optional Workstream row this milestone belongs to."),
			field.Int("pull_request_id").
				Optional().
				Comment("Optional PullRequest subject when this milestone is about a PR."),
			field.Int("ticket_id").
				Optional().
				Comment("Optional Ticket subject when this milestone is about a ticket."),
			field.String("workstream_key").
				NotEmpty().
				Comment("Stable workstream key summarized by this milestone."),
			field.Enum("subject_kind").
				Values(workInsightSubjectKindValues()...).
				Default(workInsightSubjectUnknown).
				Comment("Typed product kind this milestone is about."),
			field.String("subject_key").
				NotEmpty().
				Comment("Stable product key this milestone is about."),
			field.Enum("milestone_kind").
				Values(workProgramMilestoneKindValues()...).
				Default(workProgramMilestoneReleaseTarget).
				Comment("Kind of source milestone/date signal."),
			field.Text("milestone_name").
				NotEmpty().
				Comment("Source milestone, release, or outcome label."),
			field.Time("target_date").
				Optional().
				Comment("Source-authored target date, when present."),
			field.Time("outcome_date").
				Optional().
				Comment("Observed source outcome date, when present."),
			field.Enum("milestone_state").
				Values(workProgramMilestoneStateValues()...).
				Default(workProgramMilestoneStateUnknown).
				Comment("Comparison of target and outcome dates without implying ETA truth."),
			field.Enum("commitment_strength").
				Values(workProgramMilestoneCommitmentStrengthValues()...).
				Default(workProgramMilestoneCommitmentUnknown).
				Comment("How strong this source signal is as a commitment."),
			field.Bool("date_claim_allowed").
				Default(false).
				Comment("Whether product reads may make the narrow date claim described by claim_gate_reason."),
			field.Bool("delivery_commitment_allowed").
				Default(false).
				Comment("Whether product reads may present this as an owner delivery commitment."),
			field.Text("claim_gate_reason").
				NotEmpty().
				Comment("Why the date claim or commitment claim is allowed or gated."),
			field.String("source_field").
				NotEmpty().
				Comment("Source payload field that produced the milestone signal."),
			field.String("source_payload_key").
				Optional().
				Comment("Source object key or field path for replay/debugging."),
			field.Time("captured_at").
				Optional().
				Comment("Time the source signal was captured by analytics."),
			field.Time("generated_at").
				Optional().
				Comment("Analytics generation time for this milestone row."),
		},
		sourceIdentityFields(),
		objectEvidenceFields(),
		qualityFields(),
		activityFields(),
		timestampFields(),
	)
}

// Edges connects the milestone to its typed subject and supporting evidence.
func (WorkProgramMilestone) Edges() []ent.Edge {
	return []ent.Edge{
		edge.To("workstream", Workstream.Type).
			Unique().
			Field("workstream_id").
			Annotations(entsql.OnDelete(entsql.SetNull)).
			Comment("Workstream this milestone belongs to."),
		edge.To("pull_request", PullRequest.Type).
			Unique().
			Field("pull_request_id").
			Annotations(entsql.OnDelete(entsql.Restrict)).
			Comment("Pull request subject, when resolved."),
		edge.To("ticket", Ticket.Type).
			Unique().
			Field("ticket_id").
			Annotations(entsql.OnDelete(entsql.Restrict)).
			Comment("Ticket subject, when resolved."),
		edge.To("latest_evidence", Evidence.Type).
			Unique().
			Field("latest_evidence_id").
			Comment("Most recent evidence supporting this milestone row."),
	}
}

// Indexes supports latest milestone reads and source identity upserts.
func (WorkProgramMilestone) Indexes() []ent.Index {
	return []ent.Index{
		index.Fields("workstream_key", "generated_at", "milestone_kind", "target_date"),
		index.Fields("subject_kind", "subject_key", "milestone_kind"),
		index.Fields("commitment_strength", "date_claim_allowed", "target_date"),
		index.Fields("ticket_id", "milestone_kind", "target_date"),
		index.Fields("pull_request_id", "milestone_kind", "target_date"),
		index.Fields("source_system", "source_instance", "external_kind", "external_id").Unique(),
	}
}
