package schema

import (
	"entgo.io/ent"
	entschema "entgo.io/ent/schema"
	"entgo.io/ent/schema/edge"
	"entgo.io/ent/schema/field"
	"entgo.io/ent/schema/index"
)

// TicketMessage is the metadata-bearing association from a ticket to a message.
type TicketMessage struct {
	ent.Schema
}

// Annotations declares the composite endpoint identity for the edge schema.
func (TicketMessage) Annotations() []entschema.Annotation {
	return edgeSchemaAnnotations("ticket_id", "message_id", "ticket_message_kind")
}

// Fields stores endpoint foreign keys plus discussion-link metadata.
func (TicketMessage) Fields() []ent.Field {
	return appendFields(
		[]ent.Field{
			field.Int("ticket_id").
				Immutable().
				Comment("Source Ticket endpoint for this association."),
			field.Int("message_id").
				Immutable().
				Comment("Target Message endpoint for this association."),
		},
		linkFields("ticket_message_kind", []string{relationDiscussedIn}),
	)
}

// Edges connects the relationship row to its endpoints and latest evidence.
func (TicketMessage) Edges() []ent.Edge {
	return []ent.Edge{
		edge.To("ticket", Ticket.Type).
			Unique().
			Required().
			Immutable().
			Field("ticket_id").
			Comment("Ticket endpoint for this association."),
		edge.To("message", Message.Type).
			Unique().
			Required().
			Immutable().
			Field("message_id").
			Comment("Message endpoint for this association."),
		edge.To("latest_evidence", Evidence.Type).
			Unique().
			Field("latest_evidence_id").
			Comment("Most recent evidence supporting this association."),
	}
}

// Indexes supports bounded discussion reads from a ticket.
func (TicketMessage) Indexes() []ent.Index {
	return []ent.Index{
		semanticEdgeIndex("ticket_id", "message_id", "ticket_message_kind"),
		index.Fields("ticket_id", "freshness_state", "rank_score", "last_activity_at"),
		index.Fields("message_id", "freshness_state", "rank_score", "last_activity_at"),
	}
}
