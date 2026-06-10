package schema

import (
	"entgo.io/ent"
	entschema "entgo.io/ent/schema"
	"entgo.io/ent/schema/edge"
	"entgo.io/ent/schema/field"
	"entgo.io/ent/schema/index"
)

// TicketDocumentFragment is the metadata-bearing association from a ticket to a document fragment.
type TicketDocumentFragment struct {
	ent.Schema
}

// Annotations declares the composite endpoint identity for the edge schema.
func (TicketDocumentFragment) Annotations() []entschema.Annotation {
	return edgeSchemaAnnotations("ticket_id", "document_fragment_id")
}

// Fields stores endpoint foreign keys plus documentation-link metadata.
func (TicketDocumentFragment) Fields() []ent.Field {
	return appendFields(
		[]ent.Field{
			field.Int("ticket_id").
				Immutable().
				Comment("Source Ticket endpoint for this association."),
			field.Int("document_fragment_id").
				Immutable().
				Comment("Target DocumentFragment endpoint for this association."),
		},
		linkFields([]string{relationDocumentedBy}),
	)
}

// Edges connects the relationship row to its endpoints and latest evidence.
func (TicketDocumentFragment) Edges() []ent.Edge {
	return []ent.Edge{
		edge.To("ticket", Ticket.Type).
			Unique().
			Required().
			Immutable().
			Field("ticket_id").
			Comment("Ticket endpoint for this association."),
		edge.To("document_fragment", DocumentFragment.Type).
			Unique().
			Required().
			Immutable().
			Field("document_fragment_id").
			Comment("Document fragment endpoint for this association."),
		edge.To("latest_evidence", Evidence.Type).
			Unique().
			Field("latest_evidence_id").
			Comment("Most recent evidence supporting this association."),
	}
}

// Indexes supports bounded document-evidence reads from a ticket.
func (TicketDocumentFragment) Indexes() []ent.Index {
	return []ent.Index{
		index.Fields("ticket_id", "freshness_state", "rank_score", "last_activity_at"),
		index.Fields("document_fragment_id"),
	}
}
