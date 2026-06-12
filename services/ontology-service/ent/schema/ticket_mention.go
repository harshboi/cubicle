package schema

import (
	"entgo.io/ent"
	entschema "entgo.io/ent/schema"
	"entgo.io/ent/schema/edge"
	"entgo.io/ent/schema/field"
	"entgo.io/ent/schema/index"
)

// TicketMention is the typed relationship from a person to a ticket that
// mentions or references them.
type TicketMention struct {
	ent.Schema
}

// Annotations declares the composite endpoint identity for the edge schema.
func (TicketMention) Annotations() []entschema.Annotation {
	return edgeSchemaAnnotations("person_id", "ticket_id", "mention_kind")
}

// Fields stores endpoint foreign keys plus mention metadata.
func (TicketMention) Fields() []ent.Field {
	return appendFields(
		[]ent.Field{
			field.Int("person_id").
				Immutable().
				Comment("Person endpoint mentioned by this ticket."),
			field.Int("ticket_id").
				Immutable().
				Comment("Ticket endpoint containing the mention."),
		},
		linkFields("mention_kind", mentionKindValues()),
	)
}

// Edges connects the mention row to endpoints and proof.
func (TicketMention) Edges() []ent.Edge {
	return []ent.Edge{
		edge.To("person", Person.Type).
			Unique().
			Required().
			Immutable().
			Field("person_id").
			Comment("Person mentioned by this ticket."),
		edge.To("ticket", Ticket.Type).
			Unique().
			Required().
			Immutable().
			Field("ticket_id").
			Comment("Ticket containing this mention."),
		edge.To("latest_evidence", Evidence.Type).
			Unique().
			Field("latest_evidence_id").
			Comment("Most recent evidence supporting this mention."),
	}
}

// Indexes supports person mention queries and ticket reverse lookups.
func (TicketMention) Indexes() []ent.Index {
	return []ent.Index{
		semanticEdgeIndex("person_id", "ticket_id", "mention_kind"),
		index.Fields("person_id", "freshness_state", "rank_score", "last_activity_at"),
		index.Fields("ticket_id", "mention_kind"),
		index.Fields("ticket_id", "freshness_state", "rank_score", "last_activity_at"),
	}
}
