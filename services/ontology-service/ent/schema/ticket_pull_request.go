package schema

import (
	"entgo.io/ent"
	entschema "entgo.io/ent/schema"
	"entgo.io/ent/schema/edge"
	"entgo.io/ent/schema/field"
	"entgo.io/ent/schema/index"
)

// TicketPullRequest is the metadata-bearing association from a ticket to a pull request.
type TicketPullRequest struct {
	ent.Schema
}

// Annotations declares the composite endpoint identity for the edge schema.
func (TicketPullRequest) Annotations() []entschema.Annotation {
	return edgeSchemaAnnotations("ticket_id", "pull_request_id")
}

// Fields stores endpoint foreign keys plus implementation-link metadata.
func (TicketPullRequest) Fields() []ent.Field {
	return appendFields(
		[]ent.Field{
			field.Int("ticket_id").
				Immutable().
				Comment("Source Ticket endpoint for this association."),
			field.Int("pull_request_id").
				Immutable().
				Comment("Target PullRequest endpoint for this association."),
		},
		linkFields([]string{relationImplementedBy}),
	)
}

// Edges connects the relationship row to its endpoints and latest evidence.
func (TicketPullRequest) Edges() []ent.Edge {
	return []ent.Edge{
		edge.To("ticket", Ticket.Type).
			Unique().
			Required().
			Immutable().
			Field("ticket_id").
			Comment("Ticket endpoint for this association."),
		edge.To("pull_request", PullRequest.Type).
			Unique().
			Required().
			Immutable().
			Field("pull_request_id").
			Comment("Pull request endpoint for this association."),
		edge.To("latest_evidence", Evidence.Type).
			Unique().
			Field("latest_evidence_id").
			Comment("Most recent evidence supporting this association."),
	}
}

// Indexes supports bounded implementation-list reads from a ticket.
func (TicketPullRequest) Indexes() []ent.Index {
	return []ent.Index{
		index.Fields("ticket_id", "freshness_state", "rank_score", "last_activity_at"),
		index.Fields("pull_request_id"),
	}
}
