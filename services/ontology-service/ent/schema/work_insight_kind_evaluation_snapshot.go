package schema

import (
	"fmt"

	"entgo.io/ent"
	"entgo.io/ent/dialect/entsql"
	entschema "entgo.io/ent/schema"
	"entgo.io/ent/schema/edge"
	"entgo.io/ent/schema/field"
	"entgo.io/ent/schema/index"
)

// WorkInsightKindEvaluationSnapshot records one insight-kind quality gate.
//
// These rows make the aggregate insight evaluation explainable: each kind keeps
// the counts, rates, gate state, and next action used by product reads.
type WorkInsightKindEvaluationSnapshot struct {
	ent.Schema
}

// Annotations declares WorkInsightKindEvaluationSnapshot as a public operating view.
func (WorkInsightKindEvaluationSnapshot) Annotations() []entschema.Annotation {
	return graphqlAnnotations()
}

// Fields defines one per-kind insight-evaluation snapshot.
func (WorkInsightKindEvaluationSnapshot) Fields() []ent.Field {
	return appendFields(
		stableKeyFields(),
		[]ent.Field{
			field.Int("work_insight_evaluation_snapshot_id").
				Optional().
				Comment("Aggregate insight-evaluation snapshot this kind row belongs to."),
			field.Time("generated_at").
				Optional().
				Comment("Analytics generation time for this kind evaluation snapshot."),
			field.String("insight_kind").
				NotEmpty().
				Comment("Generated insight kind this row evaluates."),
			field.String("measurement_scope").
				Optional().
				Default("").
				Validate(func(scope string) error {
					switch scope {
					case "", "product_candidate", "context_only", "model_quality", "validation_lead":
						return nil
					default:
						return fmt.Errorf("invalid measurement_scope %q", scope)
					}
				}).
				Comment("Evaluation contract for this insight kind: product_candidate, context_only, model_quality, or validation_lead."),
			field.Int("current_insight_count").
				Default(0).
				Comment("Current generated insights for this kind."),
			field.Int("review_row_count").
				Default(0).
				Comment("Review rows considered for this kind."),
			field.Int("measurement_label_count").
				Default(0).
				Comment("Measurement-eligible labels considered for this kind."),
			field.Int("open_review_request_count").
				Default(0).
				Comment("Current insights of this kind still missing resolved measurement labels."),
			field.Int("truth_labeled_count").
				Default(0).
				Comment("Labels with a non-unknown truth label for this kind."),
			field.Int("actionability_labeled_count").
				Default(0).
				Comment("Labels with a non-unknown actionability label for this kind."),
			field.Int("true_positive_count").
				Default(0).
				Comment("True-positive labels for this kind."),
			field.Int("false_positive_count").
				Default(0).
				Comment("False-positive labels for this kind."),
			field.Int("partial_count").
				Default(0).
				Comment("Partial-truth labels for this kind."),
			field.Int("actionable_count").
				Default(0).
				Comment("Actionable labels for this kind."),
			field.Int("needs_owner_count").
				Default(0).
				Comment("Needs-owner labels for this kind."),
			field.Float("precision_rate").
				Default(0).
				Comment("True-positive rate for this kind."),
			field.Float("useful_signal_rate").
				Default(0).
				Comment("True-positive plus partial rate for this kind."),
			field.Float("actionability_rate").
				Default(0).
				Comment("Actionable plus needs-owner rate for this kind."),
			field.Float("false_positive_rate").
				Default(0).
				Comment("False-positive rate for this kind."),
			field.Float("measurement_coverage_rate").
				Default(0).
				Comment("Current insight share covered by measurement labels for this kind."),
			field.Int("required_label_count").
				Default(0).
				Comment("Labels required before this kind can be measured."),
			field.Bool("ready_to_measure").
				Default(false).
				Comment("Whether this kind has enough truth and actionability labels."),
			field.Bool("ready_for_product_action").
				Default(false).
				Comment("Whether this kind passes product-action quality gates."),
			field.String("product_action_gate_state").
				NotEmpty().
				Comment("Product-action gate state for this kind."),
			field.Text("product_action_gate_reason").
				NotEmpty().
				Comment("Reason the product-action gate is in its current state."),
			field.Text("recommended_action").
				NotEmpty().
				Comment("Recommended next action for this insight kind."),
		},
		sourceIdentityFields(),
		objectEvidenceFields(),
		qualityFields(),
		activityFields(),
		timestampFields(),
	)
}

// Edges connects the kind snapshot to its aggregate snapshot and evidence.
func (WorkInsightKindEvaluationSnapshot) Edges() []ent.Edge {
	return []ent.Edge{
		edge.From("evaluation_snapshot", WorkInsightEvaluationSnapshot.Type).
			Ref("kind_evaluations").
			Unique().
			Field("work_insight_evaluation_snapshot_id").
			Annotations(entsql.OnDelete(entsql.Cascade)).
			Comment("Aggregate insight-evaluation snapshot this kind row belongs to."),
		edge.To("latest_evidence", Evidence.Type).
			Unique().
			Field("latest_evidence_id").
			Comment("Most recent generated evidence supporting this kind evaluation row."),
	}
}

// Indexes supports latest per-kind reads and source identity upserts.
func (WorkInsightKindEvaluationSnapshot) Indexes() []ent.Index {
	return []ent.Index{
		index.Fields("insight_kind", "generated_at"),
		index.Fields("ready_to_measure", "ready_for_product_action", "generated_at"),
		index.Fields("work_insight_evaluation_snapshot_id", "insight_kind"),
		index.Fields("source_system", "source_instance", "external_kind", "external_id").Unique(),
	}
}
