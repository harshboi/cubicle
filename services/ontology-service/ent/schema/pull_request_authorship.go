package schema

import (
	"entgo.io/ent"
	entschema "entgo.io/ent/schema"
	"entgo.io/ent/schema/edge"
	"entgo.io/ent/schema/field"
	"entgo.io/ent/schema/index"
)

// PullRequestAuthorship is the typed relationship from a person to a pull request.
type PullRequestAuthorship struct {
	ent.Schema
}

// Annotations declares the composite endpoint identity for the edge schema.
func (PullRequestAuthorship) Annotations() []entschema.Annotation {
	return edgeSchemaAnnotations("person_id", "pull_request_id", "authorship_kind")
}

// Fields stores endpoint foreign keys plus authorship metadata.
func (PullRequestAuthorship) Fields() []ent.Field {
	return appendFields(
		[]ent.Field{
			field.Int("person_id").
				Immutable().
				Comment("Person endpoint for this pull-request authorship."),
			field.Int("pull_request_id").
				Immutable().
				Comment("PullRequest endpoint for this authorship."),
		},
		linkFields("authorship_kind", pullRequestAuthorshipKindValues()),
	)
}

// Edges connects the authorship row to endpoints and proof.
func (PullRequestAuthorship) Edges() []ent.Edge {
	return []ent.Edge{
		edge.To("person", Person.Type).
			Unique().
			Required().
			Immutable().
			Field("person_id").
			Comment("Person endpoint for this authorship."),
		edge.To("pull_request", PullRequest.Type).
			Unique().
			Required().
			Immutable().
			Field("pull_request_id").
			Comment("PullRequest endpoint for this authorship."),
		edge.To("latest_evidence", Evidence.Type).
			Unique().
			Field("latest_evidence_id").
			Comment("Most recent evidence supporting this authorship."),
	}
}

// Indexes supports person PR queries and PR reverse lookups.
func (PullRequestAuthorship) Indexes() []ent.Index {
	return []ent.Index{
		semanticEdgeIndex("person_id", "pull_request_id", "authorship_kind"),
		index.Fields("person_id", "freshness_state", "rank_score", "last_activity_at"),
		index.Fields("pull_request_id", "authorship_kind"),
		index.Fields("pull_request_id", "freshness_state", "rank_score", "last_activity_at"),
	}
}
