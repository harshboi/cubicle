package schema

import (
	"entgo.io/ent"
	entschema "entgo.io/ent/schema"
	"entgo.io/ent/schema/edge"
	"entgo.io/ent/schema/field"
	"entgo.io/ent/schema/index"
)

// Document is a durable source document such as a Google Doc, spec, or README.
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
		sourceFields(),
		qualityFields(),
		activityFields(),
		timestampFields(),
	)
}

// Edges connects a document to searchable fragments.
func (Document) Edges() []ent.Edge {
	return []ent.Edge{
		edge.To("fragments", DocumentFragment.Type).
			Comment("Searchable fragments extracted from this document."),
	}
}

// Indexes supports document-kind filtering and recency slicing.
func (Document) Indexes() []ent.Index {
	return []ent.Index{
		index.Fields("document_kind", "last_activity_at"),
	}
}
