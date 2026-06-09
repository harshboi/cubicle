package schema

import (
	"entgo.io/ent"
	entschema "entgo.io/ent/schema"
	"entgo.io/ent/schema/edge"
	"entgo.io/ent/schema/field"
	"entgo.io/ent/schema/index"
)

// DocumentFragment is a searchable section or chunk extracted from a document.
type DocumentFragment struct {
	ent.Schema
}

// Annotations declares that DocumentFragment is part of the future public entgql API.
func (DocumentFragment) Annotations() []entschema.Annotation {
	return graphqlAnnotations()
}

// Fields defines fragment text and position metadata.
func (DocumentFragment) Fields() []ent.Field {
	return appendFields(
		stableKeyFields(),
		[]ent.Field{
			field.Int("document_id").
				Immutable().
				Comment("Parent Document row that owns this fragment."),
			field.String("heading").
				Optional().
				Comment("Nearest heading or section label for this fragment."),
			field.String("path").
				Optional().
				Comment("Source-specific document path or section path."),
			field.Text("text").
				Optional().
				Comment("Fragment body text used as the smallest evidence/search unit."),
			field.Int("ordinal").
				Default(0).
				Comment("Stable fragment order within the parent document."),
			field.String("text_hash").
				Optional().
				Comment("Hash of normalized fragment text for idempotent re-ingestion."),
		},
		textFields(),
		sourceFields(),
		qualityFields(),
		timestampFields(),
	)
}

// Edges links each fragment to its parent document and any tickets it supports.
func (DocumentFragment) Edges() []ent.Edge {
	return []ent.Edge{
		edge.From("document", Document.Type).
			Ref("fragments").
			Unique().
			Required().
			Immutable().
			Field("document_id").
			Comment("Parent document for this fragment."),
		edge.From("tickets", Ticket.Type).
			Ref("document_fragments").
			Comment("Tickets supported by this document fragment."),
	}
}

// Indexes supports ordered fragment reads within a document.
func (DocumentFragment) Indexes() []ent.Index {
	return []ent.Index{
		index.Fields("document_id", "ordinal").Unique(),
		index.Fields("text_hash"),
	}
}
