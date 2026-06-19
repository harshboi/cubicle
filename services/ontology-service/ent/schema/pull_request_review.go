package schema

import (
	"entgo.io/ent"
	entschema "entgo.io/ent/schema"
	"entgo.io/ent/schema/edge"
	"entgo.io/ent/schema/field"
	"entgo.io/ent/schema/index"
)

// PullRequestReview is the typed relationship from a person to a pull request
// for review, approval, reviewer request, and review-comment facts.
//
// Association:
//
//	Person -> PullRequestReview -> PullRequest
//	PullRequestReview -> Evidence
//
// Review rows keep review kind separate from authorship and implementation.
type PullRequestReview struct {
	ent.Schema
}

// Annotations declares the composite endpoint identity for the edge schema.
func (PullRequestReview) Annotations() []entschema.Annotation {
	return edgeSchemaAnnotations("person_id", "pull_request_id", "review_kind")
}

// Fields stores endpoint foreign keys plus review metadata.
func (PullRequestReview) Fields() []ent.Field {
	return appendFields(
		[]ent.Field{
			field.Int("person_id").
				Immutable().
				Comment("Person endpoint for this review relationship."),
			field.Int("pull_request_id").
				Immutable().
				Comment("PullRequest endpoint for this review relationship."),
		},
		linkFields("review_kind", reviewKindValues()),
	)
}

// Edges connects the review row to endpoints and proof.
func (PullRequestReview) Edges() []ent.Edge {
	return []ent.Edge{
		edge.To("person", Person.Type).
			Unique().
			Required().
			Immutable().
			Field("person_id").
			Comment("Person endpoint for this review relationship."),
		edge.To("pull_request", PullRequest.Type).
			Unique().
			Required().
			Immutable().
			Field("pull_request_id").
			Comment("PullRequest endpoint for this review relationship."),
		edge.To("latest_evidence", Evidence.Type).
			Unique().
			Field("latest_evidence_id").
			Comment("Most recent evidence supporting this review relationship."),
	}
}

// Indexes supports person review queries and PR reverse lookups.
func (PullRequestReview) Indexes() []ent.Index {
	return []ent.Index{
		semanticEdgeIndex("person_id", "pull_request_id", "review_kind"),
		index.Fields("person_id", "freshness_state", "rank_score", "last_activity_at"),
		index.Fields("pull_request_id", "review_kind"),
		index.Fields("pull_request_id", "freshness_state", "rank_score", "last_activity_at"),
	}
}
