package schema

import (
	"entgo.io/ent"
	entschema "entgo.io/ent/schema"
	"entgo.io/ent/schema/edge"
	"entgo.io/ent/schema/field"
	"entgo.io/ent/schema/index"
)

// TicketDocument is the metadata-bearing association from a ticket to a document.
//
// Association:
//
//	Ticket -> TicketDocument -> Document
//	TicketDocument -> Evidence
//
// Documentation links remain typed product relationships with proof.
type TicketDocument struct {
	ent.Schema
}

// Annotations declares the composite endpoint identity for the edge schema.
func (TicketDocument) Annotations() []entschema.Annotation {
	return edgeSchemaAnnotations("ticket_id", "document_id", "ticket_document_kind")
}

// Fields stores endpoint foreign keys plus documentation-link metadata.
func (TicketDocument) Fields() []ent.Field {
	return appendFields(
		[]ent.Field{
			field.Int("ticket_id").
				Immutable().
				Comment("Ticket endpoint for this association."),
			field.Int("document_id").
				Immutable().
				Comment("Document endpoint for this association."),
		},
		linkFields("ticket_document_kind", ticketDocumentRelationValues()),
	)
}

// Edges connects the relationship row to endpoints and latest evidence.
func (TicketDocument) Edges() []ent.Edge {
	return []ent.Edge{
		edge.To("ticket", Ticket.Type).
			Unique().
			Required().
			Immutable().
			Field("ticket_id").
			Comment("Ticket endpoint for this association."),
		edge.To("document", Document.Type).
			Unique().
			Required().
			Immutable().
			Field("document_id").
			Comment("Document endpoint for this association."),
		edge.To("latest_evidence", Evidence.Type).
			Unique().
			Field("latest_evidence_id").
			Comment("Most recent evidence supporting this association."),
	}
}

// Indexes supports bounded document-evidence reads from a ticket.
func (TicketDocument) Indexes() []ent.Index {
	return []ent.Index{
		semanticEdgeIndex("ticket_id", "document_id", "ticket_document_kind"),
		index.Fields("ticket_id", "freshness_state", "rank_score", "last_activity_at"),
		index.Fields("document_id", "freshness_state", "rank_score", "last_activity_at"),
	}
}
