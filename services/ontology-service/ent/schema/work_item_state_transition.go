package schema

import (
	"entgo.io/ent"
	"entgo.io/ent/dialect/entsql"
	entschema "entgo.io/ent/schema"
	"entgo.io/ent/schema/edge"
	"entgo.io/ent/schema/field"
	"entgo.io/ent/schema/index"
)

// WorkItemStateTransition records an observed change between two snapshots.
//
// Association:
//
//	PullRequest/Ticket -> WorkItemStateTransition -> WorkItemStateSnapshot
//
// Transitions are outcome evidence for the TPM loop: they explain whether
// previously open work became terminal, changed state, or simply changed source
// coverage. Transition rows remain candidates until verified by source evidence
// or human closeout review.
type WorkItemStateTransition struct {
	ent.Schema
}

// Annotations declares that WorkItemStateTransition is intended for public product reads.
func (WorkItemStateTransition) Annotations() []entschema.Annotation {
	annotations := graphqlAnnotations()
	return append(annotations, entsql.Checks(map[string]string{
		"work_item_state_transitions_subject_pointer_matches_kind": "((subject_kind = 'pull_request' AND pull_request_id IS NOT NULL AND ticket_id IS NULL) OR (subject_kind = 'ticket' AND ticket_id IS NOT NULL AND pull_request_id IS NULL) OR (subject_kind = 'unknown' AND pull_request_id IS NULL AND ticket_id IS NULL))",
	}))
}

// Fields defines the changed subject, observed states, confidence, and timestamps.
func (WorkItemStateTransition) Fields() []ent.Field {
	return appendFields(
		stableKeyFields(),
		[]ent.Field{
			field.Enum("subject_kind").
				Values(workInsightSubjectKindValues()...).
				Default(workInsightSubjectUnknown).
				Comment("Resolved typed product kind this transition is about."),
			genericSubjectObjectTypeField(),
			field.String("subject_key").
				NotEmpty().
				Comment("Stable product key this transition is about."),
			field.Int("pull_request_id").
				Optional().
				Comment("Optional PullRequest subject when the transition targets a PR."),
			field.Int("ticket_id").
				Optional().
				Comment("Optional Ticket subject when the transition targets a ticket."),
			field.Int("from_snapshot_id").
				Optional().
				Comment("Optional snapshot row representing the previous observed state."),
			field.Int("to_snapshot_id").
				Optional().
				Comment("Optional snapshot row representing the latest observed state."),
			field.Time("from_observed_at").
				Optional().
				Comment("Previous observation time."),
			field.Time("to_observed_at").
				Optional().
				Comment("Latest observation time."),
			field.String("from_state").
				Optional().
				Comment("Previous observed state."),
			field.String("to_state").
				Optional().
				Comment("Latest observed state."),
			field.Enum("transition_kind").
				Values(workItemTransitionKindValues()...).
				Default(workItemTransitionStateChange).
				Comment("Kind of observed transition."),
			field.Float("transition_confidence").
				Default(0).
				Comment("Detector confidence for this transition candidate; not source-truth confidence."),
			field.Enum("confidence_basis").
				Values(workItemTransitionConfidenceBasisValues()...).
				Default(workItemTransitionConfidenceBasisUnknown).
				Comment("What transition_confidence measures, such as adjacent snapshot detection rather than product truth."),
			field.Enum("verification_state").
				Values(workItemTransitionVerificationStateValues()...).
				Default(workItemTransitionVerificationCandidate).
				Comment("Verification state for the transition candidate before source or human closeout confirmation."),
			field.Bool("terminal").
				Default(false).
				Comment("Whether this transition moves the subject into a terminal state."),
			field.Bool("requires_closeout").
				Default(false).
				Comment("Whether a TPM should confirm closeout after this transition."),
			field.Text("note").
				Optional().
				Comment("Explanation of how the transition was derived and what still needs validation."),
		},
		sourceIdentityFields(),
		objectEvidenceFields(),
		qualityFields(),
		activityFields(),
		timestampFields(),
	)
}

// Edges connects the transition to the typed subject, snapshots, and optional evidence.
func (WorkItemStateTransition) Edges() []ent.Edge {
	return []ent.Edge{
		edge.To("pull_request", PullRequest.Type).
			Unique().
			Field("pull_request_id").
			Annotations(entsql.OnDelete(entsql.Restrict)).
			Comment("Pull request this transition is about, when resolved."),
		edge.To("ticket", Ticket.Type).
			Unique().
			Field("ticket_id").
			Annotations(entsql.OnDelete(entsql.Restrict)).
			Comment("Ticket this transition is about, when resolved."),
		edge.To("from_snapshot", WorkItemStateSnapshot.Type).
			Unique().
			Field("from_snapshot_id").
			Annotations(entsql.OnDelete(entsql.SetNull)).
			Comment("Previous snapshot for this transition."),
		edge.To("to_snapshot", WorkItemStateSnapshot.Type).
			Unique().
			Field("to_snapshot_id").
			Annotations(entsql.OnDelete(entsql.SetNull)).
			Comment("Latest snapshot for this transition."),
		edge.To("latest_evidence", Evidence.Type).
			Unique().
			Field("latest_evidence_id").
			Comment("Most recent evidence supporting this transition row."),
	}
}

// Indexes supports closeout queues, subject history, and source identity upserts.
func (WorkItemStateTransition) Indexes() []ent.Index {
	return []ent.Index{
		index.Fields("subject_kind", "subject_key", "to_observed_at"),
		index.Fields("transition_kind", "terminal", "requires_closeout", "to_observed_at"),
		index.Fields("verification_state", "requires_closeout", "to_observed_at"),
		index.Fields("pull_request_id", "to_observed_at"),
		index.Fields("ticket_id", "to_observed_at"),
		index.Fields("source_system", "source_instance", "external_kind", "external_id").Unique(),
	}
}
