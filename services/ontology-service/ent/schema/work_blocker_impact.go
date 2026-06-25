package schema

import (
	"entgo.io/ent"
	"entgo.io/ent/dialect/entsql"
	entschema "entgo.io/ent/schema"
	"entgo.io/ent/schema/edge"
	"entgo.io/ent/schema/field"
	"entgo.io/ent/schema/index"
)

// WorkBlockerImpact is a derived operating projection for ranking blockers.
//
// It does not create new source truth. It materializes how an existing
// WorkBlocker affects a typed subject or workstream so product reads can ask
// "what is threatening the path?" without re-walking raw source records.
type WorkBlockerImpact struct {
	ent.Schema
}

// Annotations declares WorkBlockerImpact as a public product topology view.
func (WorkBlockerImpact) Annotations() []entschema.Annotation {
	annotations := graphqlAnnotations()
	return append(annotations, entsql.Checks(map[string]string{
		"work_blocker_impacts_score_range": "impact_score >= 0",
	}))
}

// Fields defines the projected affected node, source blocker, and impact score.
func (WorkBlockerImpact) Fields() []ent.Field {
	return appendFields(
		stableKeyFields(),
		[]ent.Field{
			field.Enum("impact_kind").
				Values(workBlockerImpactKindValues()...).
				Comment("How the blocker impacts the affected node."),
			field.Enum("impact_state").
				Values(workBlockerStateValues()...).
				Default(workBlockerStateUnknown).
				Comment("Current impact state, copied from the source blocker for bounded reads."),
			field.Float("impact_score").
				Default(0).
				Comment("Derived score used to rank blockers by operating impact."),
			field.Enum("severity").
				Values(workInsightSeverityValues()...).
				Default(workInsightSeverityInfo).
				Comment("Severity copied from the source blocker."),
			field.Enum("blocker_kind").
				Values(workBlockerKindValues()...).
				Default(workBlockerKindSourceSignal).
				Comment("Blocker category copied from the source blocker."),
			field.Int("work_blocker_id").
				Optional().
				Comment("Source WorkBlocker row this impact projects."),
			field.Int("work_action_id").
				Optional().
				Comment("Operating action attached to the source blocker, when known."),
			field.Int("workstream_id").
				Optional().
				Comment("Workstream affected by this blocker projection, when known."),
			field.Int("pull_request_id").
				Optional().
				Comment("PullRequest affected by this blocker projection, when resolved."),
			field.Int("ticket_id").
				Optional().
				Comment("Ticket affected by this blocker projection, when resolved."),
			field.Enum("affected_kind").
				Values(workDependencyNodeKindValues()...).
				Comment("Kind of node affected by the blocker."),
			field.String("affected_key").
				NotEmpty().
				Comment("Stable key for the affected node."),
			field.Enum("subject_kind").
				Values(workInsightSubjectKindValues()...).
				Default(workInsightSubjectUnknown).
				Comment("Original typed product subject kind on the blocker."),
			field.String("subject_key").
				NotEmpty().
				Comment("Original typed product subject key on the blocker."),
			field.Int("path_length").
				Default(0).
				Comment("Number of derived topology hops between the blocker and affected node."),
			field.String("source_coverage_state").
				Optional().
				Comment("Coverage/freshness state inherited from the blocker or dependency edge."),
			field.String("title").
				NotEmpty().
				Comment("Short impact title for product reads."),
			field.Text("recommended_action").
				Optional().
				Comment("Suggested action to reduce this impact."),
		},
		textFields(),
		sourceIdentityFields(),
		objectEvidenceFields(),
		qualityFields(),
		activityFields(),
		timestampFields(),
	)
}

// Edges connects the projection to its source blocker and affected objects.
func (WorkBlockerImpact) Edges() []ent.Edge {
	return []ent.Edge{
		edge.To("work_blocker", WorkBlocker.Type).
			Unique().
			Field("work_blocker_id").
			Annotations(entsql.OnDelete(entsql.Cascade)).
			Comment("Source blocker for this impact projection."),
		edge.To("work_action", WorkAction.Type).
			Unique().
			Field("work_action_id").
			Annotations(entsql.OnDelete(entsql.SetNull)).
			Comment("Action attached to the source blocker, when known."),
		edge.To("workstream", Workstream.Type).
			Unique().
			Field("workstream_id").
			Annotations(entsql.OnDelete(entsql.SetNull)).
			Comment("Affected workstream, when this is a workstream impact."),
		edge.To("pull_request", PullRequest.Type).
			Unique().
			Field("pull_request_id").
			Annotations(entsql.OnDelete(entsql.SetNull)).
			Comment("Affected pull request, when resolved."),
		edge.To("ticket", Ticket.Type).
			Unique().
			Field("ticket_id").
			Annotations(entsql.OnDelete(entsql.SetNull)).
			Comment("Affected ticket, when resolved."),
		edge.To("latest_evidence", Evidence.Type).
			Unique().
			Field("latest_evidence_id").
			Comment("Most recent evidence supporting this impact projection."),
	}
}

// Indexes supports ranked product reads by impact state, affected node, and source blocker.
func (WorkBlockerImpact) Indexes() []ent.Index {
	return []ent.Index{
		index.Fields("impact_state", "impact_score", "rank_score", "last_activity_at"),
		index.Fields("affected_kind", "affected_key", "impact_state", "impact_score"),
		index.Fields("subject_kind", "subject_key", "impact_state", "impact_score"),
		index.Fields("work_blocker_id", "impact_kind", "affected_kind", "affected_key").Unique(),
		index.Fields("workstream_id", "impact_state", "impact_score"),
		index.Fields("pull_request_id", "impact_state", "impact_score"),
		index.Fields("ticket_id", "impact_state", "impact_score"),
		index.Fields("source_system", "source_instance", "external_kind", "external_id").Unique(),
	}
}
