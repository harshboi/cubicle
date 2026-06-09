package schema

import (
	"entgo.io/ent"
	entschema "entgo.io/ent/schema"
	"entgo.io/ent/schema/edge"
	"entgo.io/ent/schema/field"
	"entgo.io/ent/schema/index"
)

// PanePullRequestLink is the metadata-bearing association from a work pane to a pull request.
type PanePullRequestLink struct {
	ent.Schema
}

// Annotations declares the composite endpoint identity for the edge schema.
func (PanePullRequestLink) Annotations() []entschema.Annotation {
	return edgeSchemaAnnotations("pane_id", "pull_request_id")
}

// Fields stores endpoint foreign keys plus ranked pull-request relationship metadata.
func (PanePullRequestLink) Fields() []ent.Field {
	return appendFields(
		[]ent.Field{
			field.Int("pane_id").
				Comment("Source WorkPane endpoint for this association."),
			field.Int("pull_request_id").
				Comment("Target PullRequest endpoint for this association."),
		},
		linkFields(pullRequestRelationValues()),
	)
}

// Edges connects the link row to its pane, pull request, and latest evidence.
func (PanePullRequestLink) Edges() []ent.Edge {
	return []ent.Edge{
		edge.To("pane", WorkPane.Type).
			Unique().
			Required().
			Field("pane_id").
			Comment("Work pane that owns this pull-request association."),
		edge.To("pull_request", PullRequest.Type).
			Unique().
			Required().
			Field("pull_request_id").
			Comment("Pull request target for this association."),
		edge.To("latest_evidence", Evidence.Type).
			Unique().
			Field("latest_evidence_id").
			Comment("Most recent evidence supporting this association."),
	}
}

// Indexes supports fast paged pull-request reads from a pane.
func (PanePullRequestLink) Indexes() []ent.Index {
	return []ent.Index{
		index.Fields("pane_id", "freshness_state", "rank_score", "last_activity_at"),
		index.Fields("pane_id", "relation_kind", "last_activity_at"),
		index.Fields("pull_request_id"),
	}
}
