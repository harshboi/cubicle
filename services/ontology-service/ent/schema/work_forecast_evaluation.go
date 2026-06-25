package schema

import (
	"entgo.io/ent"
	entschema "entgo.io/ent/schema"
	"entgo.io/ent/schema/edge"
	"entgo.io/ent/schema/field"
	"entgo.io/ent/schema/index"
)

// WorkForecastEvaluation records typed forecast quality and backtest metrics.
//
// Association:
//
//	analytics/backtest rows -> WorkForecastEvaluation -> optional Evidence
//
// Forecast evaluation rows are model facts, not product absence claims. They
// decide whether cycle predictions may be used as ETA commitments or only as TPM
// risk-triage signals.
type WorkForecastEvaluation struct {
	ent.Schema
}

// Annotations declares that WorkForecastEvaluation is intended for future public GraphQL reads.
func (WorkForecastEvaluation) Annotations() []entschema.Annotation {
	return graphqlAnnotations()
}

// Fields defines summary and fold-level forecast evaluation metrics.
func (WorkForecastEvaluation) Fields() []ent.Field {
	return appendFields(
		stableKeyFields(),
		[]ent.Field{
			field.Enum("evaluation_kind").
				Values(workForecastEvaluationKindValues()...).
				Default(workForecastEvaluationSummary).
				Comment("Forecast evaluation row kind, such as summary, k-fold, or chronological holdout."),
			field.String("model_name").
				Optional().
				Comment("Forecast model or baseline name evaluated by this row."),
			field.String("forecast_method").
				Optional().
				Comment("Forecast method selected for current product reads."),
			field.String("best_model_name").
				Optional().
				Comment("Best model selected by the backtest metric for this evaluation set."),
			field.Int("fold").
				Optional().
				Comment("Fold number for fold-level evaluations; zero or unset for summary rows."),
			field.Int("train_count").
				Default(0).
				Comment("Training sample count for this evaluation row."),
			field.Int("test_count").
				Default(0).
				Comment("Test sample count for this evaluation row."),
			field.Int("baseline_sample_count").
				Default(0).
				Comment("Merged PR count available for delivery-cycle baseline evaluation."),
			field.Int("open_candidate_count").
				Default(0).
				Comment("Current open PR count scored by the forecast/risk method."),
			field.Int("closed_unmerged_count").
				Default(0).
				Comment("Closed-unmerged PR count retained as abandonment/closure context."),
			field.Int("observed_snapshot_time_count").
				Default(0).
				Comment("Distinct observed snapshot times available for transition-history evaluation."),
			field.Int("transition_candidate_count").
				Default(0).
				Comment("Adjacent state or coverage transition candidates detected from persisted snapshots."),
			field.Int("terminal_transition_candidate_count").
				Default(0).
				Comment("Transition candidates into merged/closed/resolved terminal states."),
			field.Bool("transition_history_ready").
				Default(false).
				Comment("Whether persisted snapshot history is sufficient to evaluate transition or replanning forecasts."),
			field.Float("median_cycle_days").
				Optional().
				Nillable().
				Comment("Median merged PR cycle time in days for the evaluation population."),
			field.Float("p75_cycle_days").
				Optional().
				Nillable().
				Comment("75th percentile merged PR cycle time in days for the evaluation population."),
			field.Float("avg_closed_unmerged_cycle_days").
				Optional().
				Nillable().
				Comment("Average closed-unmerged cycle time in days, used as closure context rather than ETA training."),
			field.Float("mae_days").
				Optional().
				Nillable().
				Comment("Mean absolute error in days for this evaluation row."),
			field.Float("median_error_days").
				Optional().
				Nillable().
				Comment("Median absolute error in days for this evaluation row."),
			field.Float("p75_error_days").
				Optional().
				Nillable().
				Comment("75th percentile absolute error in days for this evaluation row."),
			field.Float("max_error_days").
				Optional().
				Nillable().
				Comment("Worst absolute error in days for this evaluation row."),
			field.Float("improvement_vs_median_pct").
				Optional().
				Nillable().
				Comment("Percent MAE improvement versus the median-cycle baseline for this row."),
			field.Bool("ready_for_eta").
				Default(false).
				Comment("Whether this row supports ETA-style forecast use."),
			field.Enum("readiness_state").
				Values(workForecastReadinessStateValues()...).
				Default(workForecastReadinessUnknown).
				Comment("Forecast readiness state derived from typed evaluation evidence."),
			field.Text("readiness_reason").
				Optional().
				Comment("Human-readable gate reason for forecast readiness."),
			field.Text("note").
				Optional().
				Comment("Producer note explaining this metric row."),
			field.Time("evaluated_at").
				Optional().
				Comment("Time this forecast evaluation was generated."),
		},
		sourceIdentityFields(),
		objectEvidenceFields(),
		qualityFields(),
		activityFields(),
		timestampFields(),
	)
}

// Edges connects the forecast evaluation to optional supporting evidence.
func (WorkForecastEvaluation) Edges() []ent.Edge {
	return []ent.Edge{
		edge.To("latest_evidence", Evidence.Type).
			Unique().
			Field("latest_evidence_id").
			Comment("Most recent evidence supporting this forecast evaluation row."),
	}
}

// Indexes supports forecast readiness reads and stable source upserts.
func (WorkForecastEvaluation) Indexes() []ent.Index {
	return []ent.Index{
		index.Fields("evaluation_kind", "model_name", "fold"),
		index.Fields("readiness_state", "ready_for_eta", "updated_at"),
		index.Fields("source_system", "source_instance", "external_kind", "external_id").Unique(),
	}
}
