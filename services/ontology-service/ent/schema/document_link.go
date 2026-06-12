package schema

import (
	"entgo.io/ent"
	entschema "entgo.io/ent/schema"
	"entgo.io/ent/schema/edge"
	"entgo.io/ent/schema/field"
	"entgo.io/ent/schema/index"
)

// DocumentLink is the typed relationship from one document to another document
// it references.
type DocumentLink struct {
	ent.Schema
}

// Annotations declares the composite endpoint identity for the edge schema.
func (DocumentLink) Annotations() []entschema.Annotation {
	return edgeSchemaAnnotations("from_document_id", "to_document_id", "document_link_kind")
}

// Fields stores endpoint foreign keys plus document-link metadata.
func (DocumentLink) Fields() []ent.Field {
	return appendFields(
		[]ent.Field{
			field.Int("from_document_id").
				Immutable().
				Comment("Document that contains the reference."),
			field.Int("to_document_id").
				Immutable().
				Comment("Document referenced by the source document."),
		},
		linkFields("document_link_kind", documentLinkRelationValues()),
	)
}

// Edges connects the document-link row to endpoints and latest evidence.
func (DocumentLink) Edges() []ent.Edge {
	return []ent.Edge{
		edge.To("from_document", Document.Type).
			Unique().
			Required().
			Immutable().
			Field("from_document_id").
			Comment("Document that contains the reference."),
		edge.To("to_document", Document.Type).
			Unique().
			Required().
			Immutable().
			Field("to_document_id").
			Comment("Document referenced by the source document."),
		edge.To("latest_evidence", Evidence.Type).
			Unique().
			Field("latest_evidence_id").
			Comment("Most recent evidence supporting this document link."),
	}
}

// Indexes supports document forward and reverse reference lookups.
func (DocumentLink) Indexes() []ent.Index {
	return []ent.Index{
		semanticEdgeIndex("from_document_id", "to_document_id", "document_link_kind"),
		index.Fields("from_document_id", "freshness_state", "rank_score", "last_activity_at"),
		index.Fields("to_document_id", "freshness_state", "rank_score", "last_activity_at"),
	}
}
