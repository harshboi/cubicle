package schema

import (
	"entgo.io/ent"
	entschema "entgo.io/ent/schema"
	"entgo.io/ent/schema/edge"
	"entgo.io/ent/schema/field"
	"entgo.io/ent/schema/index"
)

// DocumentAuthorship is the typed relationship from a person to a document.
//
// Association:
//
//	Person -> DocumentAuthorship -> Document
//	DocumentAuthorship -> Evidence
//
// The relationship row carries authorship kind, freshness, and proof.
type DocumentAuthorship struct {
	ent.Schema
}

// Annotations declares the composite endpoint identity for the edge schema.
func (DocumentAuthorship) Annotations() []entschema.Annotation {
	return edgeSchemaAnnotations("person_id", "document_id", "authorship_kind")
}

// Fields stores endpoint foreign keys plus authorship metadata.
func (DocumentAuthorship) Fields() []ent.Field {
	return appendFields(
		[]ent.Field{
			field.Int("person_id").
				Immutable().
				Comment("Person endpoint for this document authorship."),
			field.Int("document_id").
				Immutable().
				Comment("Document endpoint for this authorship."),
		},
		linkFields("authorship_kind", documentAuthorshipKindValues()),
	)
}

// Edges connects the authorship row to endpoints and proof.
func (DocumentAuthorship) Edges() []ent.Edge {
	return []ent.Edge{
		edge.To("person", Person.Type).
			Unique().
			Required().
			Immutable().
			Field("person_id").
			Comment("Person endpoint for this authorship."),
		edge.To("document", Document.Type).
			Unique().
			Required().
			Immutable().
			Field("document_id").
			Comment("Document endpoint for this authorship."),
		edge.To("latest_evidence", Evidence.Type).
			Unique().
			Field("latest_evidence_id").
			Comment("Most recent evidence supporting this authorship."),
	}
}

// Indexes supports person document queries and document reverse lookups.
func (DocumentAuthorship) Indexes() []ent.Index {
	return []ent.Index{
		semanticEdgeIndex("person_id", "document_id", "authorship_kind"),
		index.Fields("person_id", "freshness_state", "rank_score", "last_activity_at"),
		index.Fields("document_id", "authorship_kind"),
		index.Fields("document_id", "freshness_state", "rank_score", "last_activity_at"),
	}
}
