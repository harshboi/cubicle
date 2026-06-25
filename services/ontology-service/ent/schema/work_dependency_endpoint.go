package schema

import (
	"entgo.io/ent"
	"entgo.io/ent/dialect/entsql"
	entschema "entgo.io/ent/schema"
	"entgo.io/ent/schema/edge"
	"entgo.io/ent/schema/field"
	"entgo.io/ent/schema/index"
)

// WorkDependencyEndpoint records one explicit endpoint of a WorkDependencyEdge.
//
// WorkDependencyEdge keeps the compact from_kind/from_key -> to_kind/to_key
// topology. Endpoint rows make each side separately inspectable and allow the
// materializer to report whether the endpoint resolved to a typed Cubicle node.
type WorkDependencyEndpoint struct {
	ent.Schema
}

// Annotations declares WorkDependencyEndpoint as an operating-topology node.
func (WorkDependencyEndpoint) Annotations() []entschema.Annotation {
	annotations := graphqlAnnotations()
	return append(annotations, entsql.Checks(map[string]string{
		"work_dependency_endpoint_resolved_pointer_matches_kind": "(resolution_state != 'resolved' OR ((node_kind = 'ticket' AND ticket_id IS NOT NULL AND pull_request_id IS NULL AND workstream_id IS NULL AND work_blocker_id IS NULL AND work_action_id IS NULL) OR (node_kind = 'pull_request' AND pull_request_id IS NOT NULL AND ticket_id IS NULL AND workstream_id IS NULL AND work_blocker_id IS NULL AND work_action_id IS NULL) OR (node_kind = 'workstream' AND workstream_id IS NOT NULL AND ticket_id IS NULL AND pull_request_id IS NULL AND work_blocker_id IS NULL AND work_action_id IS NULL) OR (node_kind = 'blocker' AND work_blocker_id IS NOT NULL AND ticket_id IS NULL AND pull_request_id IS NULL AND workstream_id IS NULL AND work_action_id IS NULL) OR (node_kind = 'action' AND work_action_id IS NOT NULL AND ticket_id IS NULL AND pull_request_id IS NULL AND workstream_id IS NULL AND work_blocker_id IS NULL)))",
	}))
}

// Fields defines the endpoint role, source key, and optional typed resolution.
func (WorkDependencyEndpoint) Fields() []ent.Field {
	return appendFields(
		stableKeyFields(),
		[]ent.Field{
			field.Int("work_dependency_edge_id").
				Immutable().
				Comment("Dependency edge this endpoint belongs to."),
			field.Enum("endpoint_role").
				Values(workDependencyEndpointRoleValues()...).
				Comment("Whether this row describes the from or to endpoint."),
			field.Enum("node_kind").
				Values(workDependencyNodeKindValues()...).
				Comment("Endpoint node kind from the dependency edge."),
			field.String("node_key").
				NotEmpty().
				Comment("Endpoint stable key from the dependency edge."),
			field.Enum("resolution_state").
				Values(workDependencyEndpointResolutionValues()...).
				Default(workDependencyEndpointMissing).
				Comment("Whether the endpoint resolved to a typed Cubicle target."),
			field.String("resolution_reason").
				Optional().
				Comment("Short explanation for the endpoint resolution state."),
			field.Int("workstream_id").
				Optional().
				Comment("Resolved Workstream endpoint, when node_kind is workstream."),
			field.Int("work_blocker_id").
				Optional().
				Comment("Resolved WorkBlocker endpoint, when node_kind is blocker."),
			field.Int("work_action_id").
				Optional().
				Comment("Resolved WorkAction endpoint, when node_kind is action."),
			field.Int("ticket_id").
				Optional().
				Comment("Resolved Ticket endpoint, when node_kind is ticket."),
			field.Int("pull_request_id").
				Optional().
				Comment("Resolved PullRequest endpoint, when node_kind is pull_request."),
		},
		sourceBackedFields(),
		objectEvidenceFields(),
		qualityFields(),
		activityFields(),
		timestampFields(),
	)
}

// Edges connects the endpoint back to its edge and optional typed target.
func (WorkDependencyEndpoint) Edges() []ent.Edge {
	return []ent.Edge{
		edge.To("work_dependency_edge", WorkDependencyEdge.Type).
			Unique().
			Required().
			Immutable().
			Field("work_dependency_edge_id").
			Annotations(entsql.OnDelete(entsql.Cascade)).
			Comment("Dependency edge this endpoint belongs to."),
		edge.To("workstream", Workstream.Type).
			Unique().
			Field("workstream_id").
			Annotations(entsql.OnDelete(entsql.SetNull)).
			Comment("Resolved Workstream endpoint."),
		edge.To("work_blocker", WorkBlocker.Type).
			Unique().
			Field("work_blocker_id").
			Annotations(entsql.OnDelete(entsql.SetNull)).
			Comment("Resolved WorkBlocker endpoint."),
		edge.To("work_action", WorkAction.Type).
			Unique().
			Field("work_action_id").
			Annotations(entsql.OnDelete(entsql.SetNull)).
			Comment("Resolved WorkAction endpoint."),
		edge.To("ticket", Ticket.Type).
			Unique().
			Field("ticket_id").
			Annotations(entsql.OnDelete(entsql.SetNull)).
			Comment("Resolved Ticket endpoint."),
		edge.To("pull_request", PullRequest.Type).
			Unique().
			Field("pull_request_id").
			Annotations(entsql.OnDelete(entsql.SetNull)).
			Comment("Resolved PullRequest endpoint."),
		edge.To("latest_evidence", Evidence.Type).
			Unique().
			Field("latest_evidence_id").
			Comment("Most recent evidence supporting the endpoint resolution."),
	}
}

// Indexes supports endpoint traversal and resolution-quality audits.
func (WorkDependencyEndpoint) Indexes() []ent.Index {
	return []ent.Index{
		index.Fields("work_dependency_edge_id", "endpoint_role").Unique(),
		index.Fields("endpoint_role", "node_kind", "node_key"),
		index.Fields("node_kind", "node_key", "resolution_state"),
		index.Fields("workstream_id", "endpoint_role"),
		index.Fields("work_blocker_id", "endpoint_role"),
		index.Fields("work_action_id", "endpoint_role"),
		index.Fields("ticket_id", "endpoint_role"),
		index.Fields("pull_request_id", "endpoint_role"),
		index.Fields("source_system", "source_instance", "external_kind", "external_id").Unique(),
	}
}
