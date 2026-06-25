package schema

import (
	"entgo.io/ent"
	"entgo.io/ent/dialect/entsql"
	entschema "entgo.io/ent/schema"
	"entgo.io/ent/schema/edge"
	"entgo.io/ent/schema/field"
	"entgo.io/ent/schema/index"
)

// WorkDependencyEdge is a bounded operating-topology edge between work nodes.
//
// It complements typed relationship rows such as TicketPullRequest by storing
// the derived TPM operating graph: workstream membership, dependency clusters,
// blocked-by relationships, and blocker-to-action links.
type WorkDependencyEdge struct {
	ent.Schema
}

// Annotations declares WorkDependencyEdge as a product topology edge.
func (WorkDependencyEdge) Annotations() []entschema.Annotation {
	annotations := graphqlAnnotations()
	return append(annotations, entsql.Checks(map[string]string{
		"work_dependency_edges_no_self_edge":                         "from_kind != to_kind OR from_key != to_key",
		"work_dependency_edges_canonical_mirror_requires_typed_kind": "relationship_authority != 'canonical_mirror' OR canonical_relationship_kind IS NOT NULL",
		"work_dependency_edges_projection_has_no_canonical_kind":     "relationship_authority = 'canonical_mirror' OR canonical_relationship_kind IS NULL",
		"work_dependency_edges_ticket_pr_mirror_shape":               "relationship_authority != 'canonical_mirror' OR (edge_kind = 'ticket_pr' AND canonical_relationship_kind = 'ticket_pull_request' AND from_kind = 'ticket' AND to_kind = 'pull_request' AND ticket_id IS NOT NULL AND pull_request_id IS NOT NULL)",
	}))
}

// Fields defines the graph endpoints and operating edge metadata.
func (WorkDependencyEdge) Fields() []ent.Field {
	return appendFields(
		stableKeyFields(),
		[]ent.Field{
			field.Enum("edge_kind").
				Values(workDependencyEdgeKindValues()...).
				Comment("Machine-readable operating relationship kind."),
			field.Enum("relationship_authority").
				Values(workDependencyRelationshipAuthorityValues()...).
				Default(workDependencyRelationshipOperatingProjection).
				Comment("Whether this edge is a canonical typed relationship mirror or a generated operating projection."),
			field.Enum("canonical_relationship_kind").
				Values(workDependencyCanonicalRelationshipKindValues()...).
				Optional().
				Comment("Typed relationship row this edge mirrors when relationship_authority is canonical_mirror."),
			field.Enum("from_kind").
				Values(workDependencyNodeKindValues()...).
				Comment("Source endpoint kind."),
			field.String("from_key").
				NotEmpty().
				Comment("Source endpoint stable key."),
			field.Enum("to_kind").
				Values(workDependencyNodeKindValues()...).
				Comment("Target endpoint kind."),
			field.String("to_key").
				NotEmpty().
				Comment("Target endpoint stable key."),
			field.String("risk_signal").
				Optional().
				Comment("Source or analytics risk signal explaining why this edge matters operationally."),
			field.String("source_coverage_state").
				Optional().
				Comment("Latest coverage/freshness state for the source evidence behind this edge."),
			field.Int("workstream_id").
				Optional().
				Comment("Optional Workstream context for this dependency edge."),
			field.Int("work_blocker_id").
				Optional().
				Comment("Optional WorkBlocker context for blocked-by or needs-action edges."),
			field.Int("work_action_id").
				Optional().
				Comment("Optional WorkAction context for needs-action edges."),
			field.Int("ticket_id").
				Optional().
				Comment("Optional Ticket endpoint or context for this dependency edge."),
			field.Int("pull_request_id").
				Optional().
				Comment("Optional PullRequest endpoint or context for this dependency edge."),
		},
		sourceBackedFields(),
		objectEvidenceFields(),
		qualityFields(),
		activityFields(),
		timestampFields(),
	)
}

// Edges connects derived topology edges to their most recent supporting evidence.
func (WorkDependencyEdge) Edges() []ent.Edge {
	return []ent.Edge{
		edge.To("work_blocker", WorkBlocker.Type).
			Unique().
			Field("work_blocker_id").
			Annotations(entsql.OnDelete(entsql.SetNull)).
			Comment("Blocker claim context for blocked-by or needs-action edges."),
		edge.To("work_action", WorkAction.Type).
			Unique().
			Field("work_action_id").
			Annotations(entsql.OnDelete(entsql.SetNull)).
			Comment("Action ledger context for needs-action edges."),
		edge.To("latest_evidence", Evidence.Type).
			Unique().
			Field("latest_evidence_id").
			Comment("Most recent evidence supporting this dependency edge."),
		edge.From("endpoints", WorkDependencyEndpoint.Type).
			Ref("work_dependency_edge").
			Comment("Explicit from/to endpoint rows for this dependency edge."),
	}
}

// Indexes supports bounded graph reads by workstream, endpoint, and edge kind.
func (WorkDependencyEdge) Indexes() []ent.Index {
	return []ent.Index{
		index.Fields("edge_kind", "from_kind", "from_key", "to_kind", "to_key").Unique(),
		index.Fields("relationship_authority", "edge_kind", "rank_score", "last_activity_at"),
		index.Fields("canonical_relationship_kind", "edge_kind"),
		index.Fields("workstream_id", "edge_kind", "rank_score", "last_activity_at"),
		index.Fields("work_blocker_id", "edge_kind", "rank_score", "last_activity_at"),
		index.Fields("work_action_id", "edge_kind", "rank_score", "last_activity_at"),
		index.Fields("ticket_id", "edge_kind", "rank_score", "last_activity_at"),
		index.Fields("pull_request_id", "edge_kind", "rank_score", "last_activity_at"),
		index.Fields("from_kind", "from_key", "edge_kind", "rank_score"),
		index.Fields("to_kind", "to_key", "edge_kind", "rank_score"),
		index.Fields("source_system", "source_instance", "external_kind", "external_id").Unique(),
	}
}
