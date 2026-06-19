package schema

import (
	"entgo.io/ent"
	entschema "entgo.io/ent/schema"
	"entgo.io/ent/schema/edge"
	"entgo.io/ent/schema/field"
	"entgo.io/ent/schema/index"
)

// Document is a durable source document such as a Google Doc, spec, or README.
//
// Association:
//
//	Person -> DocumentAuthorship -> Document
//	Ticket -> TicketDocument -> Document
//	Document -> DocumentLink -> Document
//
// Document relationships are typed so proof and relation kind stay reviewable.
type Document struct {
	ent.Schema
}

// Annotations declares that Document is part of the future public entgql API.
func (Document) Annotations() []entschema.Annotation {
	return graphqlAnnotations()
}

// Fields defines document metadata and text used by V0 Ent-filter search.
func (Document) Fields() []ent.Field {
	return appendFields(
		stableKeyFields(),
		[]ent.Field{
			field.String("title").
				NotEmpty().
				Comment("Human-readable document title."),
			field.Enum("document_kind").
				Values(documentKindUnknown, documentKindGoogle, documentKindMarkdown, documentKindSpec).
				Default(documentKindUnknown).
				Comment("Normalized source document kind."),
			field.String("revision").
				Optional().
				Comment("Source revision, version, or etag when available."),
		},
		textFields(),
		sourceBackedFields(),
		objectEvidenceFields(),
		qualityFields(),
		activityFields(),
		timestampFields(),
	)
}

// Edges connects a document to typed product link rows.
func (Document) Edges() []ent.Edge {
	return []ent.Edge{
		edge.From("tickets", Ticket.Type).
			Ref("documents").
			Comment("Tickets documented or explained by this document."),
		edge.From("outgoing_document_links", DocumentLink.Type).
			Ref("from_document").
			Comment("Typed document-link rows where this document contains the reference."),
		edge.From("incoming_document_links", DocumentLink.Type).
			Ref("to_document").
			Comment("Typed document-link rows where another document references this one."),
		edge.To("latest_evidence", Evidence.Type).
			Unique().
			Field("latest_evidence_id").
			Comment("Most recent evidence supporting this document state."),
	}
}

// Indexes supports document-kind filtering and recency slicing.
func (Document) Indexes() []ent.Index {
	return []ent.Index{
		index.Fields("document_kind", "last_activity_at"),
		index.Fields("source_system", "source_instance", "external_kind", "external_id").Unique(),
	}
}
