package schema

import (
	"entgo.io/ent"
	"entgo.io/ent/dialect/entsql"
	entschema "entgo.io/ent/schema"
	"entgo.io/ent/schema/edge"
	"entgo.io/ent/schema/field"
	"entgo.io/ent/schema/index"
)

// WorkAction is a durable TPM decision, validation lead, or closeout item.
//
// Association:
//
//	WorkInsight -> WorkAction -> WorkActionObservation
//	WorkAction -> optional typed product subject
//
// WorkInsight rows are generated leads. WorkAction rows are the gated operating
// ledger that tells product reads whether a lead is a measurement-backed action,
// a validation lead, source repair, closeout review, model/rule QA, or a
// suppressed signal.
type WorkAction struct {
	ent.Schema
}

// Annotations declares that WorkAction is intended for future public GraphQL reads.
func (WorkAction) Annotations() []entschema.Annotation {
	annotations := graphqlAnnotations()
	return append(annotations, entsql.Checks(map[string]string{
		"work_actions_subject_pointer_matches_kind": "((subject_kind = 'pull_request' AND pull_request_id IS NOT NULL AND ticket_id IS NULL) OR (subject_kind = 'ticket' AND ticket_id IS NOT NULL AND pull_request_id IS NULL) OR (subject_kind = 'unknown' AND pull_request_id IS NULL AND ticket_id IS NULL))",
		"work_actions_closed_has_closed_at":         "action_state != 'closed' OR closed_at IS NOT NULL",
	}))
}

// Fields defines the action ledger row and its gating decision.
func (WorkAction) Fields() []ent.Field {
	return appendFields(
		stableKeyFields(),
		[]ent.Field{
			field.Enum("action_type").
				Values(workActionTypeValues()...).
				Comment("Machine-readable TPM operating action type."),
			field.Enum("action_state").
				Values(workActionStateValues()...).
				Default(workActionStateOpen).
				Comment("Lifecycle state of this action row."),
			field.Enum("decision_state").
				Values(workActionDecisionStateValues()...).
				Default(workActionDecisionPendingReview).
				Comment("Gate outcome: product action, validation lead, source repair, closeout, QA, or suppressed signal."),
			field.String("decision").
				Optional().
				Comment("Short decision or gate outcome string used by projections."),
			field.Text("decision_reason").
				Optional().
				Comment("Human-readable reason why the action is gated or product-action ready."),
			field.Enum("subject_kind").
				Values(workInsightSubjectKindValues()...).
				Default(workInsightSubjectUnknown).
				Comment("Resolved typed product kind this action is about."),
			genericSubjectObjectTypeField(),
			field.String("subject_key").
				NotEmpty().
				Comment("Stable product key or source-neutral subject key this action is about."),
			field.Int("pull_request_id").
				Optional().
				Comment("Optional PullRequest subject when the action has a typed PR target."),
			field.Int("ticket_id").
				Optional().
				Comment("Optional Ticket subject when the action has a typed ticket target."),
			field.String("owner_key").
				Optional().
				Comment("Current accountable person/team key, when one is known."),
			field.String("owner_source").
				Optional().
				Comment("How owner_key was chosen, such as pr_author or requested_reviewer."),
			field.Enum("due_bucket").
				Values(workActionDueBucketValues()...).
				Default(workActionDueUnscheduled).
				Comment("Operating due bucket used by TPM briefs."),
			field.String("created_from_run_key").
				Optional().
				Comment("Analytics or sync run key that created or last regenerated this action."),
			field.Time("opened_at").
				Optional().
				Comment("Time this action became active."),
			field.Time("decided_at").
				Optional().
				Comment("Time this action received a product/validation/suppression decision."),
			field.Time("closed_at").
				Optional().
				Comment("Time this action was closed."),
		},
		sourceIdentityFields(),
		objectEvidenceFields(),
		qualityFields(),
		activityFields(),
		timestampFields(),
	)
}

// Edges connects the action to its typed subject, source insights, evidence, and observations.
func (WorkAction) Edges() []ent.Edge {
	return []ent.Edge{
		edge.To("pull_request", PullRequest.Type).
			Unique().
			Field("pull_request_id").
			Annotations(entsql.OnDelete(entsql.Restrict)).
			Comment("Pull request this action is about, when resolved."),
		edge.To("ticket", Ticket.Type).
			Unique().
			Field("ticket_id").
			Annotations(entsql.OnDelete(entsql.Restrict)).
			Comment("Ticket this action is about, when resolved."),
		edge.To("source_insights", WorkInsight.Type).
			Comment("Generated insights that produced or support this action."),
		edge.To("latest_evidence", Evidence.Type).
			Unique().
			Field("latest_evidence_id").
			Comment("Most recent evidence supporting this action state."),
		edge.To("observations", WorkActionObservation.Type).
			Comment("Current source observations or QA evidence attached to this action."),
	}
}

// Indexes supports product reads by gate state, subject, owner, and due bucket.
func (WorkAction) Indexes() []ent.Index {
	return []ent.Index{
		index.Fields("decision_state", "action_state", "due_bucket", "rank_score", "last_activity_at"),
		index.Fields("subject_kind", "subject_key", "decision_state", "action_state"),
		index.Fields("owner_key", "action_state", "due_bucket", "rank_score"),
		index.Fields("pull_request_id", "decision_state", "action_state", "rank_score"),
		index.Fields("ticket_id", "decision_state", "action_state", "rank_score"),
		index.Fields("source_system", "source_instance", "external_kind", "external_id").Unique(),
	}
}
