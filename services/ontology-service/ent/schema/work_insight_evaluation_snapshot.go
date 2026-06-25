package schema

import (
	"entgo.io/ent"
	entschema "entgo.io/ent/schema"
	"entgo.io/ent/schema/edge"
	"entgo.io/ent/schema/field"
	"entgo.io/ent/schema/index"
)

// WorkInsightEvaluationSnapshot records the measured quality of generated insights.
//
// This is the durable aggregate read behind workInsightEvaluation. It lets product
// reads replay the same precision/actionability gates from the DB instead of
// recomputing them from review rows on every request.
type WorkInsightEvaluationSnapshot struct {
	ent.Schema
}

// Annotations declares WorkInsightEvaluationSnapshot as a public operating view.
func (WorkInsightEvaluationSnapshot) Annotations() []entschema.Annotation {
	return graphqlAnnotations()
}

// Fields defines one aggregate insight-evaluation snapshot.
func (WorkInsightEvaluationSnapshot) Fields() []ent.Field {
	return appendFields(
		stableKeyFields(),
		[]ent.Field{
			field.Time("generated_at").
				Optional().
				Comment("Analytics generation time for this insight evaluation snapshot."),
			field.Int("current_insight_count").
				Default(0).
				Comment("Current generated insights covered by this evaluation."),
			field.Int("review_row_count").
				Default(0).
				Comment("Review rows considered for current generated insights."),
			field.Int("measurement_label_count").
				Default(0).
				Comment("Measurement-eligible labels considered for aggregate quality gates."),
			field.Int("open_review_request_count").
				Default(0).
				Comment("Current insights still missing resolved measurement labels."),
			field.Int("min_labeled_total_required").
				Default(0).
				Comment("Minimum aggregate labels required before measurement is considered ready."),
			field.Int("min_labeled_per_kind_required").
				Default(0).
				Comment("Minimum labels required per insight kind before measurement is considered ready."),
			field.Float("min_precision_rate_for_product_action").
				Default(0).
				Comment("Minimum precision rate required before product-action automation."),
			field.Float("min_useful_signal_rate_for_product_action").
				Default(0).
				Comment("Minimum useful-signal rate required before product-action automation."),
			field.Float("min_actionability_rate_for_product_action").
				Default(0).
				Comment("Minimum actionability rate required before product-action automation."),
			field.Float("precision_rate").
				Default(0).
				Comment("True-positive rate across current measurement labels."),
			field.Float("useful_signal_rate").
				Default(0).
				Comment("True-positive plus partial rate across current measurement labels."),
			field.Float("actionability_rate").
				Default(0).
				Comment("Actionable plus needs-owner rate across current measurement labels."),
			field.Float("false_positive_rate").
				Default(0).
				Comment("False-positive rate across current measurement labels."),
			field.Float("measurement_coverage_rate").
				Default(0).
				Comment("Current insight share covered by measurement labels."),
			field.Bool("ready_to_measure_precision").
				Default(false).
				Comment("Whether precision has enough labels overall and per kind."),
			field.Bool("ready_to_measure_actionability").
				Default(false).
				Comment("Whether actionability has enough labels overall and per kind."),
			field.Int("ready_insight_kind_count").
				Default(0).
				Comment("Insight kinds with enough labels for measurement."),
			field.Int("product_action_ready_kind_count").
				Default(0).
				Comment("Insight kinds that pass product-action quality gates."),
			field.Int("quality_gated_insight_kind_count").
				Default(0).
				Comment("Measurement-ready insight kinds that fail product-action quality gates."),
			field.Int("gated_insight_kind_count").
				Default(0).
				Comment("Insight kinds still gated by insufficient measurement coverage."),
			field.Text("recommended_next_step").
				NotEmpty().
				Comment("Recommended next step for improving or promoting insight quality."),
		},
		sourceIdentityFields(),
		objectEvidenceFields(),
		qualityFields(),
		activityFields(),
		timestampFields(),
	)
}

// Edges connects the aggregate snapshot to per-kind snapshots and evidence.
func (WorkInsightEvaluationSnapshot) Edges() []ent.Edge {
	return []ent.Edge{
		edge.To("kind_evaluations", WorkInsightKindEvaluationSnapshot.Type).
			Comment("Per-kind quality gates captured in the same evaluation run."),
		edge.To("latest_evidence", Evidence.Type).
			Unique().
			Field("latest_evidence_id").
			Comment("Most recent generated evidence supporting this evaluation snapshot."),
	}
}

// Indexes supports latest aggregate reads and source identity upserts.
func (WorkInsightEvaluationSnapshot) Indexes() []ent.Index {
	return []ent.Index{
		index.Fields("generated_at"),
		index.Fields("source_system", "source_instance", "external_kind", "generated_at"),
		index.Fields("source_system", "source_instance", "external_kind", "external_id").Unique(),
	}
}
