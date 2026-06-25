package schema

import (
	"entgo.io/ent"
	"entgo.io/ent/dialect/entsql"
	entschema "entgo.io/ent/schema"
	"entgo.io/ent/schema/edge"
	"entgo.io/ent/schema/field"
	"entgo.io/ent/schema/index"
)

// WorkItemForecast records a typed forecast for a product work item.
//
// Association:
//
//	PullRequest/Ticket -> WorkItemForecast -> optional Evidence
//
// Forecast rows are scoped to typed product subjects and carry the numeric
// estimates/risk scores a TPM would triage from. Forecast readiness remains
// separate in WorkForecastEvaluation so a forecast can be visible while still
// explicitly gated from ETA promises.
type WorkItemForecast struct {
	ent.Schema
}

// Annotations declares that WorkItemForecast is intended for future public GraphQL reads.
func (WorkItemForecast) Annotations() []entschema.Annotation {
	annotations := graphqlAnnotations()
	return append(annotations, entsql.Checks(map[string]string{
		"work_item_forecasts_subject_pointer_matches_kind": "((subject_kind = 'pull_request' AND pull_request_id IS NOT NULL AND ticket_id IS NULL) OR (subject_kind = 'ticket' AND ticket_id IS NOT NULL AND pull_request_id IS NULL) OR (subject_kind = 'unknown' AND pull_request_id IS NULL AND ticket_id IS NULL))",
	}))
}

// Fields defines the forecast subject, prediction, gate state, and rank fields.
func (WorkItemForecast) Fields() []ent.Field {
	return appendFields(
		stableKeyFields(),
		[]ent.Field{
			field.Enum("forecast_kind").
				Values(workForecastKindValues()...).
				Default(workForecastKindCycleTime).
				Comment("Forecast task this row represents, such as cycle_time."),
			field.Enum("subject_kind").
				Values(workInsightSubjectKindValues()...).
				Default(workInsightSubjectUnknown).
				Comment("Resolved typed product kind this forecast is about."),
			genericSubjectObjectTypeField(),
			field.String("subject_key").
				NotEmpty().
				Comment("Stable product key this forecast is about."),
			field.Int("pull_request_id").
				Optional().
				Comment("Optional PullRequest subject when the forecast targets a PR."),
			field.Int("ticket_id").
				Optional().
				Comment("Optional Ticket subject when the forecast targets a ticket."),
			field.Int("work_action_id").
				Optional().
				Comment("Optional TPM action that should execute or validate this forecast risk."),
			field.Int("forecast_evaluation_id").
				Optional().
				Comment("Optional WorkForecastEvaluation row authorizing this forecast's readiness state."),
			field.String("subject_state").
				Optional().
				Comment("Source lifecycle state of the subject when this forecast was generated."),
			field.String("forecast_method").
				Optional().
				Comment("Forecast method selected for this subject."),
			field.String("model_name").
				Optional().
				Comment("Forecast model or baseline name used for this subject."),
			field.Float("age_days").
				Optional().
				Nillable().
				Comment("Subject age in days at forecast time."),
			field.Float("predicted_total_cycle_days").
				Optional().
				Nillable().
				Comment("Estimated total cycle time in days."),
			field.Float("predicted_remaining_days").
				Optional().
				Nillable().
				Comment("Estimated remaining cycle time in days."),
			field.Float("overdue_days").
				Optional().
				Nillable().
				Comment("Days past the forecast/risk threshold."),
			field.Float("risk_score").
				Default(0).
				Comment("Forecast risk score used for TPM triage."),
			field.Enum("risk_band").
				Values(workForecastRiskBandValues()...).
				Default(workForecastRiskUnknown).
				Comment("Risk band derived from the forecast score."),
			field.Enum("readiness_state").
				Values(workForecastReadinessStateValues()...).
				Default(workForecastReadinessUnknown).
				Comment("Whether this forecast may be used for ETA commitments or only risk triage."),
			field.Bool("ready_for_eta").
				Default(false).
				Comment("Whether the forecast can be presented as ETA-ready."),
			field.Text("readiness_reason").
				Optional().
				Comment("Why this forecast is ready or gated."),
			field.Time("forecasted_at").
				Optional().
				Comment("Time this forecast was generated."),
		},
		sourceIdentityFields(),
		objectEvidenceFields(),
		qualityFields(),
		activityFields(),
		timestampFields(),
	)
}

// Edges connects the forecast to its typed subject and optional evidence row.
func (WorkItemForecast) Edges() []ent.Edge {
	return []ent.Edge{
		edge.To("pull_request", PullRequest.Type).
			Unique().
			Field("pull_request_id").
			Annotations(entsql.OnDelete(entsql.Restrict)).
			Comment("Pull request this forecast is about, when resolved."),
		edge.To("ticket", Ticket.Type).
			Unique().
			Field("ticket_id").
			Annotations(entsql.OnDelete(entsql.Restrict)).
			Comment("Ticket this forecast is about, when resolved."),
		edge.To("work_action", WorkAction.Type).
			Unique().
			Field("work_action_id").
			Annotations(entsql.OnDelete(entsql.Restrict)).
			Comment("Executable TPM action for this forecast risk, when materialized."),
		edge.To("forecast_evaluation", WorkForecastEvaluation.Type).
			Unique().
			Field("forecast_evaluation_id").
			Annotations(entsql.OnDelete(entsql.Restrict)).
			Comment("Forecast evaluation row that authorizes ETA readiness for this forecast."),
		edge.To("latest_evidence", Evidence.Type).
			Unique().
			Field("latest_evidence_id").
			Comment("Most recent evidence supporting this forecast row."),
	}
}

// Indexes supports forecast triage by risk, subject, and source identity.
func (WorkItemForecast) Indexes() []ent.Index {
	return []ent.Index{
		index.Fields("forecast_kind", "subject_kind", "subject_key"),
		index.Fields("risk_band", "risk_score", "overdue_days"),
		index.Fields("subject_state", "risk_band", "risk_score"),
		index.Fields("pull_request_id", "forecast_kind"),
		index.Fields("ticket_id", "forecast_kind"),
		index.Fields("work_action_id"),
		index.Fields("forecast_evaluation_id", "readiness_state", "ready_for_eta"),
		index.Fields("source_system", "source_instance", "external_kind", "external_id").Unique(),
	}
}
