package schema

import (
	"entgo.io/ent"
	entschema "entgo.io/ent/schema"
	"entgo.io/ent/schema/edge"
	"entgo.io/ent/schema/field"
	"entgo.io/ent/schema/index"
)

// PaneTicketLink is the metadata-bearing association from a work pane to a ticket.
type PaneTicketLink struct {
	ent.Schema
}

// Annotations declares the composite endpoint identity for the edge schema.
func (PaneTicketLink) Annotations() []entschema.Annotation {
	return edgeSchemaAnnotations("pane_id", "ticket_id")
}

// Fields stores endpoint foreign keys plus ranked ticket relationship metadata.
func (PaneTicketLink) Fields() []ent.Field {
	return appendFields(
		[]ent.Field{
			field.Int("pane_id").
				Comment("Source WorkPane endpoint for this association."),
			field.Int("ticket_id").
				Comment("Target Ticket endpoint for this association."),
		},
		linkFields(ticketRelationValues()),
	)
}

// Edges connects the link row to its pane, ticket, and latest evidence.
func (PaneTicketLink) Edges() []ent.Edge {
	return []ent.Edge{
		edge.To("pane", WorkPane.Type).
			Unique().
			Required().
			Field("pane_id").
			Comment("Work pane that owns this ticket association."),
		edge.To("ticket", Ticket.Type).
			Unique().
			Required().
			Field("ticket_id").
			Comment("Ticket target for this association."),
		edge.To("latest_evidence", Evidence.Type).
			Unique().
			Field("latest_evidence_id").
			Comment("Most recent evidence supporting this association."),
	}
}

// Indexes supports fast paged ticket reads from a pane.
func (PaneTicketLink) Indexes() []ent.Index {
	return []ent.Index{
		index.Fields("pane_id", "freshness_state", "rank_score", "last_activity_at"),
		index.Fields("pane_id", "relation_kind", "last_activity_at"),
		index.Fields("ticket_id"),
	}
}
