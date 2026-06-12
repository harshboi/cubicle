package schema

import (
	"entgo.io/ent"
	entschema "entgo.io/ent/schema"
	"entgo.io/ent/schema/edge"
	"entgo.io/ent/schema/field"
	"entgo.io/ent/schema/index"
)

// PullRequestLensResult is the ranked association from a work lens to a pull request.
type PullRequestLensResult struct {
	ent.Schema
}

// Annotations declares the composite endpoint identity for the edge schema.
func (PullRequestLensResult) Annotations() []entschema.Annotation {
	return edgeSchemaAnnotations("work_lens_id", "pull_request_id", "relation_kind")
}

// Fields stores endpoint foreign keys plus ranked pull-request relationship metadata.
func (PullRequestLensResult) Fields() []ent.Field {
	return appendFields(
		[]ent.Field{
			field.Int("work_lens_id").
				Immutable().
				Comment("Source WorkLens endpoint for this result."),
			field.Int("work_lens_window_id").
				Immutable().
				Comment("Bounded WorkLensWindow this result is assigned to for paging and materialization."),
			field.Int("pull_request_id").
				Immutable().
				Comment("Target PullRequest endpoint for this result."),
		},
		linkFields("relation_kind", pullRequestRelationValues()),
	)
}

// Edges connects the result row to its lens, pull request, and latest evidence.
func (PullRequestLensResult) Edges() []ent.Edge {
	return []ent.Edge{
		edge.To("lens", WorkLens.Type).
			Unique().
			Required().
			Immutable().
			Field("work_lens_id").
			Comment("Work lens that owns this pull-request result."),
		edge.From("window", WorkLensWindow.Type).
			Ref("pull_request_results").
			Unique().
			Required().
			Immutable().
			Field("work_lens_window_id").
			Comment("Bounded lens window used to page this pull-request result."),
		edge.To("pull_request", PullRequest.Type).
			Unique().
			Required().
			Immutable().
			Field("pull_request_id").
			Comment("Pull request target for this result."),
		edge.To("latest_evidence", Evidence.Type).
			Unique().
			Field("latest_evidence_id").
			Comment("Most recent evidence supporting this result."),
	}
}

// Indexes supports fast paged pull-request reads from a lens.
func (PullRequestLensResult) Indexes() []ent.Index {
	return []ent.Index{
		semanticEdgeIndex("work_lens_id", "pull_request_id", "relation_kind"),
		index.Fields("work_lens_window_id", "freshness_state", "rank_score", "last_activity_at"),
		index.Fields("work_lens_window_id", "relation_kind", "last_activity_at"),
		index.Fields("work_lens_id", "freshness_state", "rank_score", "last_activity_at"),
		index.Fields("work_lens_id", "relation_kind", "last_activity_at"),
		index.Fields("pull_request_id", "freshness_state", "rank_score", "last_activity_at"),
	}
}
