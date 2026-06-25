package schema

import (
	"entgo.io/ent"
	entschema "entgo.io/ent/schema"
	"entgo.io/ent/schema/edge"
	"entgo.io/ent/schema/field"
	"entgo.io/ent/schema/index"
)

// WorkDecisionTargetEvaluation records classifier/backtest metrics for TPM decision targets.
//
// Association:
//
//	analytics decision-target rows -> WorkDecisionTargetEvaluation -> optional Evidence
//
// These rows evaluate whether a model can rank or classify a TPM decision target
// such as abandonment risk. They are not ETA forecasts and must not be interpreted
// as product action approval unless ready_for_product_action is true.
type WorkDecisionTargetEvaluation struct {
	ent.Schema
}

// Annotations declares WorkDecisionTargetEvaluation as a future public operating view.
func (WorkDecisionTargetEvaluation) Annotations() []entschema.Annotation {
	return graphqlAnnotations()
}

// Fields defines one classifier-evaluation row with explicit classifier metrics.
func (WorkDecisionTargetEvaluation) Fields() []ent.Field {
	return appendFields(
		stableKeyFields(),
		[]ent.Field{
			field.String("target_kind").
				NotEmpty().
				Comment("Decision target being evaluated, such as abandonment_risk."),
			field.String("evaluation_kind").
				NotEmpty().
				Comment("Evaluation design, such as source_event_as_of_grouped_kfold or coverage_stratum."),
			field.String("model_name").
				NotEmpty().
				Comment("Classifier, heuristic, or guardrail model represented by this row."),
			field.Int("fold").
				Default(0).
				Comment("Fold number for fold-level evaluations; zero for summaries or aggregate rows."),
			field.Int("train_count").
				Default(0).
				Comment("Training sample count for this classifier evaluation row."),
			field.Int("test_count").
				Default(0).
				Comment("Test or scored sample count for this classifier evaluation row."),
			field.Int("positive_count").
				Default(0).
				Comment("Positive target count in the scored population."),
			field.Float("baseline_positive_rate").
				Optional().
				Nillable().
				Comment("Positive class base rate for the scored population."),
			field.Float("precision_at_10pct").
				Optional().
				Nillable().
				Comment("Precision among the top ten percent by classifier score."),
			field.Float("lift_at_10pct").
				Optional().
				Nillable().
				Comment("Precision-at-ten-percent minus the scored population base rate."),
			field.Float("roc_auc").
				Optional().
				Nillable().
				Comment("ROC AUC for this classifier evaluation row when both classes are present."),
			field.Float("average_precision").
				Optional().
				Nillable().
				Comment("Average precision for this classifier evaluation row when labels permit it."),
			field.Text("coverage_stratum").
				Optional().
				Comment("Source coverage/provenance stratum for coverage-confounding checks."),
			field.Bool("ready_for_product_action").
				Default(false).
				Comment("Whether this classifier row is validated enough to support product action."),
			field.String("product_action_gate_state").
				NotEmpty().
				Comment("Gate state explaining product-action readiness, such as gated or ready."),
			field.Text("product_action_gate_reason").
				NotEmpty().
				Comment("Human-readable reason this classifier row is or is not product-action ready."),
			field.Text("note").
				Optional().
				Comment("Producer note explaining the evaluation row and intended use."),
			field.Time("evaluated_at").
				Optional().
				Comment("Time this decision-target evaluation was generated."),
		},
		sourceIdentityFields(),
		objectEvidenceFields(),
		qualityFields(),
		activityFields(),
		timestampFields(),
	)
}

// Edges connects the classifier evaluation to optional supporting evidence.
func (WorkDecisionTargetEvaluation) Edges() []ent.Edge {
	return []ent.Edge{
		edge.To("latest_evidence", Evidence.Type).
			Unique().
			Field("latest_evidence_id").
			Comment("Most recent evidence supporting this decision-target evaluation row."),
	}
}

// Indexes supports latest evaluation reads and stable source upserts.
func (WorkDecisionTargetEvaluation) Indexes() []ent.Index {
	return []ent.Index{
		index.Fields("target_kind", "evaluation_kind", "model_name", "fold"),
		index.Fields("ready_for_product_action", "product_action_gate_state", "evaluated_at"),
		index.Fields("source_system", "source_instance", "external_kind", "external_id").Unique(),
	}
}
