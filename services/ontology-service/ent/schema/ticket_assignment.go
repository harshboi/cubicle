package schema

import (
	"entgo.io/ent"
	entschema "entgo.io/ent/schema"
	"entgo.io/ent/schema/edge"
	"entgo.io/ent/schema/field"
	"entgo.io/ent/schema/index"
)

// TicketAssignment is the typed relationship from a person to a ticket for
// assignee/reporter/owner facts.
type TicketAssignment struct {
	ent.Schema
}

// Annotations declares the composite endpoint identity for the edge schema.
func (TicketAssignment) Annotations() []entschema.Annotation {
	return edgeSchemaAnnotations("person_id", "ticket_id", "assignment_kind")
}

// Fields stores endpoint foreign keys plus assignment metadata.
func (TicketAssignment) Fields() []ent.Field {
	return appendFields(
		[]ent.Field{
			field.Int("person_id").
				Immutable().
				Comment("Person endpoint for this assignment."),
			field.Int("ticket_id").
				Immutable().
				Comment("Ticket endpoint for this assignment."),
		},
		linkFields("assignment_kind", assignmentKindValues()),
	)
}

// Edges connects the assignment row to endpoints and proof.
func (TicketAssignment) Edges() []ent.Edge {
	return []ent.Edge{
		edge.To("person", Person.Type).
			Unique().
			Required().
			Immutable().
			Field("person_id").
			Comment("Person endpoint for this assignment."),
		edge.To("ticket", Ticket.Type).
			Unique().
			Required().
			Immutable().
			Field("ticket_id").
			Comment("Ticket endpoint for this assignment."),
		edge.To("latest_evidence", Evidence.Type).
			Unique().
			Field("latest_evidence_id").
			Comment("Most recent evidence supporting this assignment."),
	}
}

// Indexes supports person work queries and ticket reverse lookups.
func (TicketAssignment) Indexes() []ent.Index {
	return []ent.Index{
		semanticEdgeIndex("person_id", "ticket_id", "assignment_kind"),
		index.Fields("person_id", "freshness_state", "rank_score", "last_activity_at"),
		index.Fields("ticket_id", "assignment_kind"),
		index.Fields("ticket_id", "freshness_state", "rank_score", "last_activity_at"),
	}
}
