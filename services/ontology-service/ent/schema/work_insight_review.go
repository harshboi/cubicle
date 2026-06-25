package schema

import (
	"entgo.io/ent"
	"entgo.io/ent/dialect/entsql"
	entschema "entgo.io/ent/schema"
	"entgo.io/ent/schema/edge"
	"entgo.io/ent/schema/field"
	"entgo.io/ent/schema/index"
)

// WorkInsightReview records review, validation, and evaluation labels for a generated insight.
//
// Association:
//
//	WorkInsight -> WorkInsightReview
//
// Reviews are intentionally separated from WorkInsight producer fields so
// analytics reruns can update generated state without overwriting human or
// imported truth/actionability labels.
type WorkInsightReview struct {
	ent.Schema
}

// Annotations declares that WorkInsightReview is intended for future public GraphQL reads.
func (WorkInsightReview) Annotations() []entschema.Annotation {
	annotations := graphqlAnnotations()
	return append(annotations, entsql.Checks(map[string]string{
		"work_insight_reviews_triage_unlabeled": "review_kind != 'triage_request' OR (truth_label = 'unknown' AND actionability_label = 'unknown')",
		"work_insight_reviews_measurement_kind": "measurement_eligible = false OR review_kind IN ('human_assessment', 'evaluation_label')",
	}))
}

// Fields defines the insight under review and the labels needed for evaluation.
func (WorkInsightReview) Fields() []ent.Field {
	return appendFields(
		stableKeyFields(),
		[]ent.Field{
			field.Int("work_insight_id").
				Immutable().
				Comment("Generated WorkInsight being reviewed or labeled."),
			field.Enum("review_kind").
				Values(workInsightReviewKindValues()...).
				Default(workInsightReviewKindTriageRequest).
				Comment("Whether this row queues review, records a human assessment, or imports an evaluation label."),
			field.Enum("review_state").
				Values(workInsightReviewStateValues()...).
				Default(workInsightReviewStateRequested).
				Comment("Current state of this review request or assessment."),
			field.Enum("truth_label").
				Values(workInsightTruthValues()...).
				Default(workInsightTruthUnknown).
				Comment("Evaluation truth label for precision/recall measurement."),
			field.Enum("actionability_label").
				Values(workInsightActionabilityValues()...).
				Default(workInsightActionabilityUnknown).
				Comment("Whether the insight was useful enough for TPM follow-up."),
			field.String("label_set").
				Optional().
				Comment("Imported, human, or evaluation campaign label-set key for this assessment."),
			field.Enum("label_quality").
				Values(workInsightLabelQualityValues()...).
				Default(workInsightLabelQualityUnknown).
				Comment("Measurement quality tier for this review label; gold labels can support aggregate gates."),
			field.Bool("measurement_eligible").
				Default(false).
				Comment("Whether this review row is allowed to count toward measurement gates."),
			field.Enum("reviewer_kind").
				Values(workInsightReviewerKindValues()...).
				Default(workInsightReviewerSystem).
				Comment("Source of this review row: seeded by system, supplied by human, or imported."),
			field.String("reviewer_key").
				Optional().
				Comment("Stable reviewer identifier, such as a user key or imported label-set key."),
			field.String("owner_key").
				Optional().
				Comment("Optional person/team key responsible for follow-up."),
			field.Text("next_action").
				Optional().
				Comment("Reviewer-supplied next action or handoff note."),
			field.Text("rationale").
				Optional().
				Comment("Reason for the labels on this review row."),
			field.Time("reviewed_at").
				Optional().
				Comment("Time the review or imported label was supplied."),
		},
		sourceIdentityFields(),
		timestampFields(),
	)
}

// Edges connects a review row to the generated insight being assessed.
func (WorkInsightReview) Edges() []ent.Edge {
	return []ent.Edge{
		edge.From("insight", WorkInsight.Type).
			Ref("reviews").
			Unique().
			Required().
			Immutable().
			Field("work_insight_id").
			Comment("Generated insight being reviewed."),
	}
}

// Indexes supports review queues and offline evaluation slices.
func (WorkInsightReview) Indexes() []ent.Index {
	return []ent.Index{
		index.Fields("work_insight_id", "review_kind", "reviewer_kind"),
		index.Fields("review_kind", "label_quality", "review_state"),
		index.Fields("review_state", "review_kind", "created_at"),
		index.Fields("truth_label", "actionability_label", "review_kind"),
		index.Fields("reviewer_kind", "reviewer_key", "review_kind"),
		index.Fields("source_system", "source_instance", "external_kind", "external_id").Unique(),
	}
}
