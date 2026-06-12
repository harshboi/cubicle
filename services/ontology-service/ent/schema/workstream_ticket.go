package schema

import (
	"entgo.io/ent"
	entschema "entgo.io/ent/schema"
	"entgo.io/ent/schema/edge"
	"entgo.io/ent/schema/field"
	"entgo.io/ent/schema/index"
)

// WorkstreamTicket is the metadata-bearing association from a workstream to a ticket.
type WorkstreamTicket struct {
	ent.Schema
}

// Annotations declares the composite endpoint identity for the edge schema.
func (WorkstreamTicket) Annotations() []entschema.Annotation {
	return edgeSchemaAnnotations("workstream_id", "ticket_id", "workstream_ticket_kind")
}

// Fields stores endpoint foreign keys plus relationship metadata.
func (WorkstreamTicket) Fields() []ent.Field {
	return appendFields(
		[]ent.Field{
			field.Int("workstream_id").
				Immutable().
				Comment("Source Workstream endpoint for this association."),
			field.Int("ticket_id").
				Immutable().
				Comment("Target Ticket endpoint for this association."),
		},
		linkFields("workstream_ticket_kind", []string{relationContains}),
	)
}

// Edges connects the relationship row to its endpoints and latest evidence.
func (WorkstreamTicket) Edges() []ent.Edge {
	return []ent.Edge{
		edge.To("workstream", Workstream.Type).
			Unique().
			Required().
			Immutable().
			Field("workstream_id").
			Comment("Workstream endpoint for this association."),
		edge.To("ticket", Ticket.Type).
			Unique().
			Required().
			Immutable().
			Field("ticket_id").
			Comment("Ticket endpoint for this association."),
		edge.To("latest_evidence", Evidence.Type).
			Unique().
			Field("latest_evidence_id").
			Comment("Most recent evidence supporting this association."),
	}
}

// Indexes supports bounded association-list reads from a workstream.
func (WorkstreamTicket) Indexes() []ent.Index {
	return []ent.Index{
		semanticEdgeIndex("workstream_id", "ticket_id", "workstream_ticket_kind"),
		index.Fields("workstream_id", "freshness_state", "rank_score", "last_activity_at"),
		index.Fields("ticket_id", "freshness_state", "rank_score", "last_activity_at"),
	}
}
