package schema

import (
	"entgo.io/ent"
	"entgo.io/ent/dialect/entsql"
	entschema "entgo.io/ent/schema"
	"entgo.io/ent/schema/edge"
	"entgo.io/ent/schema/field"
	"entgo.io/ent/schema/index"
)

// WorkItemStateSnapshot records one observed state for a typed work item.
//
// Association:
//
//	PullRequest/Ticket -> WorkItemStateSnapshot -> optional Evidence
//
// Snapshots preserve the time-series substrate used by transition detection
// and forecasting. They record source coverage alongside observed state so a
// partial or failed read does not become a product absence claim.
type WorkItemStateSnapshot struct {
	ent.Schema
}

// Annotations declares that WorkItemStateSnapshot is intended for public product reads.
func (WorkItemStateSnapshot) Annotations() []entschema.Annotation {
	annotations := graphqlAnnotations()
	return append(annotations, entsql.Checks(map[string]string{
		"work_item_state_snapshots_subject_pointer_matches_kind": "((subject_kind = 'pull_request' AND pull_request_id IS NOT NULL AND ticket_id IS NULL) OR (subject_kind = 'ticket' AND ticket_id IS NOT NULL AND pull_request_id IS NULL) OR (subject_kind = 'unknown' AND pull_request_id IS NULL AND ticket_id IS NULL))",
	}))
}

// Fields defines the typed subject, observed state, trend metrics, and coverage metadata.
func (WorkItemStateSnapshot) Fields() []ent.Field {
	return appendFields(
		stableKeyFields(),
		[]ent.Field{
			field.Enum("subject_kind").
				Values(workInsightSubjectKindValues()...).
				Default(workInsightSubjectUnknown).
				Comment("Resolved typed product kind this snapshot is about."),
			field.String("subject_key").
				NotEmpty().
				Comment("Stable product key this snapshot is about."),
			field.Int("pull_request_id").
				Optional().
				Comment("Optional PullRequest subject when the snapshot targets a PR."),
			field.Int("ticket_id").
				Optional().
				Comment("Optional Ticket subject when the snapshot targets a ticket."),
			field.String("state").
				Optional().
				Comment("Observed lifecycle state/status at observed_at."),
			field.String("title").
				Optional().
				Comment("Observed subject title at observed_at."),
			field.Time("observed_at").
				Optional().
				Comment("Time the source state was observed."),
			field.Time("captured_at").
				Optional().
				Comment("Time the analytics pipeline captured or replayed this snapshot."),
			field.Time("source_created_at").
				Optional().
				Nillable().
				Comment("Source-created time for PR cycle analysis when available."),
			field.Time("source_updated_at").
				Optional().
				Nillable().
				Comment("Source-updated time for freshness analysis when available."),
			field.Time("closed_at").
				Optional().
				Nillable().
				Comment("Observed closed time when the source provides one."),
			field.Time("merged_at").
				Optional().
				Nillable().
				Comment("Observed merged time when the source provides one."),
			field.Float("age_days").
				Optional().
				Nillable().
				Comment("Subject age in days at observed_at."),
			field.Float("stale_days").
				Optional().
				Nillable().
				Comment("Days since source update at observed_at."),
			field.Float("cycle_time_days").
				Optional().
				Nillable().
				Comment("Observed cycle time in days for terminal work."),
			field.Float("risk_score").
				Default(0).
				Comment("Risk score recorded with this snapshot, when available."),
			field.Enum("risk_band").
				Values(workForecastRiskBandValues()...).
				Default(workForecastRiskUnknown).
				Comment("Risk band recorded with this snapshot, when available."),
			field.String("forecast_method").
				Optional().
				Comment("Forecast method that produced risk fields on this snapshot."),
			field.String("source_current_coverage_state").
				Optional().
				Comment("Coverage state for the source read that produced this snapshot."),
			field.String("source_current_detail_state").
				Optional().
				Comment("Detail completeness state for the source read that produced this snapshot."),
			field.Text("source_current_issue_codes").
				Optional().
				Comment("Source issue codes observed for this snapshot."),
			field.Text("source_current_issue_kinds").
				Optional().
				Comment("Source issue kinds observed for this snapshot."),
			field.String("lifecycle_fields_source").
				Optional().
				Comment("How lifecycle fields were populated, such as live follow-up or fixture replay."),
			field.String("churn_fields_source").
				Optional().
				Comment("How churn fields were populated."),
			field.String("mergeability_fields_source").
				Optional().
				Comment("How mergeability fields were populated."),
			field.String("priority").
				Optional().
				Comment("Observed ticket priority when the snapshot targets a ticket."),
			field.Int("linked_pr_count").
				Optional().
				Comment("Count of PR links observed for a ticket snapshot."),
			field.Int("fresh_pr_link_count").
				Optional().
				Comment("Count of fresh PR links observed for a ticket snapshot."),
			field.Int("partial_pr_link_count").
				Optional().
				Comment("Count of partial PR links observed for a ticket snapshot."),
			field.Int("comment_count").
				Optional().
				Comment("Observed ticket comment count."),
			field.Int("participant_count").
				Optional().
				Comment("Observed participant count."),
			field.Int("blocker_keyword_count").
				Optional().
				Comment("Count of blocker-like keywords observed in ticket context."),
		},
		sourceIdentityFields(),
		objectEvidenceFields(),
		qualityFields(),
		activityFields(),
		timestampFields(),
	)
}

// Edges connects the snapshot to its typed subject and optional evidence row.
func (WorkItemStateSnapshot) Edges() []ent.Edge {
	return []ent.Edge{
		edge.To("pull_request", PullRequest.Type).
			Unique().
			Field("pull_request_id").
			Annotations(entsql.OnDelete(entsql.Restrict)).
			Comment("Pull request this snapshot is about, when resolved."),
		edge.To("ticket", Ticket.Type).
			Unique().
			Field("ticket_id").
			Annotations(entsql.OnDelete(entsql.Restrict)).
			Comment("Ticket this snapshot is about, when resolved."),
		edge.To("latest_evidence", Evidence.Type).
			Unique().
			Field("latest_evidence_id").
			Comment("Most recent evidence supporting this snapshot row."),
		edge.From("from_transitions", WorkItemStateTransition.Type).
			Ref("from_snapshot").
			Comment("Transitions that use this snapshot as the previous observation."),
		edge.From("to_transitions", WorkItemStateTransition.Type).
			Ref("to_snapshot").
			Comment("Transitions that use this snapshot as the latest observation."),
	}
}

// Indexes supports subject history, transition detection, and coverage audits.
func (WorkItemStateSnapshot) Indexes() []ent.Index {
	return []ent.Index{
		index.Fields("subject_kind", "subject_key", "observed_at"),
		index.Fields("pull_request_id", "observed_at"),
		index.Fields("ticket_id", "observed_at"),
		index.Fields("state", "observed_at"),
		index.Fields("risk_band", "risk_score", "observed_at"),
		index.Fields("source_current_coverage_state", "source_current_detail_state"),
		index.Fields("source_system", "source_instance", "external_kind", "external_id").Unique(),
	}
}
