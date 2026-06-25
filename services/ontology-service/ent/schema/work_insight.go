package schema

import (
	"entgo.io/ent"
	"entgo.io/ent/dialect/entsql"
	entschema "entgo.io/ent/schema"
	"entgo.io/ent/schema/edge"
	"entgo.io/ent/schema/field"
	"entgo.io/ent/schema/index"
)

// WorkInsight is a generated, evidence-backed TPM/product signal.
//
// Association:
//
//	source-backed product rows -> analytics/rules -> WorkInsight -> Evidence
//	WorkInsight -> optional typed product subject
//
// Insights are product-facing derived facts. They stay separate from source
// sync issues because they make a recommendation about work, not connector
// health, and separate from lens results because they carry narrative action
// text and model metadata rather than only ranked membership.
type WorkInsight struct {
	ent.Schema
}

// Annotations declares that WorkInsight is intended for future public GraphQL reads.
func (WorkInsight) Annotations() []entschema.Annotation {
	annotations := graphqlAnnotations()
	return append(annotations, entsql.Checks(map[string]string{
		"work_insights_score_range":                  "score >= 0 AND score <= 100",
		"work_insights_subject_pointer_matches_kind": "((subject_kind = 'pull_request' AND pull_request_id IS NOT NULL AND ticket_id IS NULL) OR (subject_kind = 'ticket' AND ticket_id IS NOT NULL AND pull_request_id IS NULL) OR (subject_kind = 'unknown' AND pull_request_id IS NULL AND ticket_id IS NULL))",
	}))
}

// Fields defines the generated signal, the product subject, and model context.
func (WorkInsight) Fields() []ent.Field {
	return appendFields(
		stableKeyFields(),
		[]ent.Field{
			field.Enum("insight_kind").
				Values(workInsightKindValues()...).
				Comment("Machine-readable kind of generated TPM/product insight."),
			field.Enum("severity").
				Values(workInsightSeverityValues()...).
				Default(workInsightSeverityInfo).
				Comment("User-facing urgency of this insight."),
			field.Enum("producer_state").
				Values(workInsightProducerStateValues()...).
				Default(workInsightProducerCurrent).
				Comment("Whether the latest producer run reproduced this generated insight."),
			field.Enum("subject_kind").
				Values(workInsightSubjectKindValues()...).
				Default(workInsightSubjectUnknown).
				Comment("Resolved typed product kind this insight is about; unsupported subjects stay unknown until modeled."),
			genericSubjectObjectTypeField(),
			field.String("subject_key").
				NotEmpty().
				Comment("Stable product key or source-neutral subject key this insight is about."),
			field.Int("pull_request_id").
				Optional().
				Comment("Optional PullRequest subject when the insight has a typed PR target."),
			field.Int("ticket_id").
				Optional().
				Comment("Optional Ticket subject when the insight has a typed ticket target."),
			field.String("title").
				NotEmpty().
				Comment("Short insight title suitable for a TPM brief."),
			field.Text("details").
				Optional().
				Comment("Generated explanation of why the insight was emitted; source proof remains in Evidence/input rows."),
			field.Text("recommended_action").
				Optional().
				Comment("Suggested TPM follow-up action for the insight."),
			field.String("model_name").
				Optional().
				Comment("Rule, model, or analytics job that produced this insight."),
			field.String("model_version").
				Optional().
				Comment("Version of the rule/model/analytics job that produced this insight."),
			field.String("model_method").
				Optional().
				Comment("Forecasting or rule method used, including fallback/rejection state."),
			field.Float("score").
				Default(0).
				Comment("Model or rule score on a 0-100 scale used for ranking within severity."),
			field.Text("score_explanation").
				Optional().
				Comment("Short explanation of how the score should be interpreted."),
		},
		objectEvidenceFields(),
		sourceBackedFields(),
		qualityFields(),
		activityFields(),
		timestampFields(),
	)
}

// Edges connects an insight to its optional typed subject and latest evidence.
func (WorkInsight) Edges() []ent.Edge {
	return []ent.Edge{
		edge.To("pull_request", PullRequest.Type).
			Unique().
			Field("pull_request_id").
			Annotations(entsql.OnDelete(entsql.Restrict)).
			Comment("Pull request this insight is about, when resolved."),
		edge.To("ticket", Ticket.Type).
			Unique().
			Field("ticket_id").
			Annotations(entsql.OnDelete(entsql.Restrict)).
			Comment("Ticket this insight is about, when resolved."),
		edge.To("latest_evidence", Evidence.Type).
			Unique().
			Field("latest_evidence_id").
			Comment("Most recent evidence supporting this generated insight."),
		edge.To("reviews", WorkInsightReview.Type).
			Comment("Review requests, human assessments, and evaluation labels for this insight."),
		edge.From("actions", WorkAction.Type).
			Ref("source_insights").
			Comment("Durable TPM actions produced from or supported by this generated insight."),
	}
}

// Indexes supports TPM briefs by subject, severity, producer state, and recency.
func (WorkInsight) Indexes() []ent.Index {
	return []ent.Index{
		index.Fields("insight_kind", "severity", "rank_score", "last_activity_at"),
		index.Fields("subject_kind", "subject_key", "producer_state"),
		index.Fields("producer_state", "severity", "rank_score", "last_activity_at"),
		index.Fields("pull_request_id", "producer_state", "severity", "rank_score"),
		index.Fields("ticket_id", "producer_state", "severity", "rank_score"),
		index.Fields("source_system", "source_instance", "external_kind", "external_id").Unique(),
	}
}
