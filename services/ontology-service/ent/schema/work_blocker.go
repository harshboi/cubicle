package schema

import (
	"entgo.io/ent"
	"entgo.io/ent/dialect/entsql"
	entschema "entgo.io/ent/schema"
	"entgo.io/ent/schema/edge"
	"entgo.io/ent/schema/field"
	"entgo.io/ent/schema/index"
)

// WorkBlocker is a first-class operating blocker claim.
//
// Association:
//
//	WorkAction / WorkInsight -> WorkBlocker -> typed product subject
//	WorkBlocker -> Evidence
//
// WorkAction remains the TPM ledger entry. WorkBlocker is the durable topology
// object that can be linked from tickets, pull requests, and workstream graph
// reads without reducing blockers to action text.
type WorkBlocker struct {
	ent.Schema
}

// Annotations declares WorkBlocker as a public product graph object.
func (WorkBlocker) Annotations() []entschema.Annotation {
	annotations := graphqlAnnotations()
	return append(annotations, entsql.Checks(map[string]string{
		"work_blockers_subject_pointer_matches_kind": "((subject_kind = 'pull_request' AND pull_request_id IS NOT NULL AND ticket_id IS NULL) OR (subject_kind = 'ticket' AND ticket_id IS NOT NULL AND pull_request_id IS NULL) OR (subject_kind = 'unknown' AND pull_request_id IS NULL AND ticket_id IS NULL))",
	}))
}

// Fields defines blocker identity, state, gating provenance, and typed subject.
func (WorkBlocker) Fields() []ent.Field {
	return appendFields(
		stableKeyFields(),
		[]ent.Field{
			field.Enum("blocker_kind").
				Values(workBlockerKindValues()...).
				Default(workBlockerKindSourceSignal).
				Comment("Machine-readable blocker category."),
			field.Enum("blocker_state").
				Values(workBlockerStateValues()...).
				Default(workBlockerStateUnknown).
				Comment("Current operating state of the blocker claim."),
			field.Enum("severity").
				Values(workInsightSeverityValues()...).
				Default(workInsightSeverityInfo).
				Comment("User-facing urgency of the blocker."),
			field.Enum("subject_kind").
				Values(workInsightSubjectKindValues()...).
				Default(workInsightSubjectUnknown).
				Comment("Typed product kind this blocker is about."),
			genericSubjectObjectTypeField(),
			field.String("subject_key").
				NotEmpty().
				Comment("Stable product key this blocker is about."),
			field.Int("pull_request_id").
				Optional().
				Comment("Optional PullRequest subject when the blocker has a typed PR target."),
			field.Int("ticket_id").
				Optional().
				Comment("Optional Ticket subject when the blocker has a typed ticket target."),
			field.Int("work_action_id").
				Optional().
				Comment("Optional WorkAction ledger row that asks someone to clear or validate this blocker."),
			field.Int("work_insight_id").
				Optional().
				Comment("Optional WorkInsight row that generated or supports this blocker."),
			field.String("owner_key").
				Optional().
				Comment("Current accountable person/team key, when one is known."),
			field.String("owner_source").
				Optional().
				Comment("How owner_key was chosen."),
			field.Enum("decision_state").
				Values(workActionDecisionStateValues()...).
				Default(workActionDecisionPendingReview).
				Comment("Action gate state attached to this blocker."),
			field.String("source_coverage_state").
				Optional().
				Comment("Latest source observation or coverage state used before making blocker claims."),
			field.Enum("review_state").
				Values(workInsightReviewStateValues()...).
				Default(workInsightReviewStateRequested).
				Comment("Best visible review state supporting this blocker."),
			field.Enum("truth_label").
				Values(workInsightTruthValues()...).
				Default(workInsightTruthUnknown).
				Comment("Truth label from the best visible blocker review."),
			field.Enum("actionability_label").
				Values(workInsightActionabilityValues()...).
				Default(workInsightActionabilityUnknown).
				Comment("Actionability label from the best visible blocker review."),
			field.Enum("label_quality").
				Values(workInsightLabelQualityValues()...).
				Default(workInsightLabelQualityUnknown).
				Comment("Quality tier of the best visible blocker review label."),
			field.Bool("measurement_eligible").
				Default(false).
				Comment("Whether this blocker's displayed review can count toward measurement gates."),
			field.Enum("reviewer_kind").
				Values(workInsightReviewerKindValues()...).
				Default(workInsightReviewerSystem).
				Comment("Source of the displayed review row."),
			field.String("reviewer_key").
				Optional().
				Comment("Stable reviewer key for the displayed review row."),
			field.String("label_set").
				Optional().
				Comment("Review/evaluation campaign label-set key."),
			field.String("title").
				NotEmpty().
				Comment("Short blocker title suitable for workstream operating views."),
			field.Text("recommended_action").
				Optional().
				Comment("Suggested TPM follow-up for clearing or validating the blocker."),
		},
		textFields(),
		sourceBackedFields(),
		objectEvidenceFields(),
		qualityFields(),
		activityFields(),
		timestampFields(),
	)
}

// Edges connects the blocker to typed subjects, the ledger action, source insight, and evidence.
func (WorkBlocker) Edges() []ent.Edge {
	return []ent.Edge{
		edge.To("pull_request", PullRequest.Type).
			Unique().
			Field("pull_request_id").
			Annotations(entsql.OnDelete(entsql.Restrict)).
			Comment("Pull request this blocker is about, when resolved."),
		edge.To("ticket", Ticket.Type).
			Unique().
			Field("ticket_id").
			Annotations(entsql.OnDelete(entsql.Restrict)).
			Comment("Ticket this blocker is about, when resolved."),
		edge.To("work_action", WorkAction.Type).
			Unique().
			Field("work_action_id").
			Annotations(entsql.OnDelete(entsql.SetNull)).
			Comment("Operating ledger row attached to this blocker."),
		edge.To("work_insight", WorkInsight.Type).
			Unique().
			Field("work_insight_id").
			Annotations(entsql.OnDelete(entsql.SetNull)).
			Comment("Generated insight that produced or supports this blocker."),
		edge.To("latest_evidence", Evidence.Type).
			Unique().
			Field("latest_evidence_id").
			Comment("Most recent evidence supporting this blocker state."),
	}
}

// Indexes supports blocker reads by state, subject, owner, and source identity.
func (WorkBlocker) Indexes() []ent.Index {
	return []ent.Index{
		index.Fields("blocker_state", "decision_state", "severity", "rank_score", "last_activity_at"),
		index.Fields("subject_kind", "subject_key", "blocker_state", "decision_state"),
		index.Fields("owner_key", "blocker_state", "severity", "rank_score"),
		index.Fields("pull_request_id", "blocker_state", "severity", "rank_score"),
		index.Fields("ticket_id", "blocker_state", "severity", "rank_score"),
		index.Fields("work_action_id").Unique(),
		index.Fields("source_system", "source_instance", "external_kind", "external_id").Unique(),
	}
}
