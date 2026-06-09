package schema

import (
	"entgo.io/ent"
	entschema "entgo.io/ent/schema"
	"entgo.io/ent/schema/edge"
	"entgo.io/ent/schema/field"
	"entgo.io/ent/schema/index"
)

// PaneDocumentLink is the metadata-bearing association from a work pane to a document.
type PaneDocumentLink struct {
	ent.Schema
}

// Annotations declares the composite endpoint identity for the edge schema.
func (PaneDocumentLink) Annotations() []entschema.Annotation {
	return edgeSchemaAnnotations("pane_id", "document_id")
}

// Fields stores endpoint foreign keys plus ranked document relationship metadata.
func (PaneDocumentLink) Fields() []ent.Field {
	return appendFields(
		[]ent.Field{
			field.Int("pane_id").
				Comment("Source WorkPane endpoint for this association."),
			field.Int("document_id").
				Comment("Target Document endpoint for this association."),
		},
		linkFields(documentRelationValues()),
	)
}

// Edges connects the link row to its pane, document, and latest evidence.
func (PaneDocumentLink) Edges() []ent.Edge {
	return []ent.Edge{
		edge.To("pane", WorkPane.Type).
			Unique().
			Required().
			Field("pane_id").
			Comment("Work pane that owns this document association."),
		edge.To("document", Document.Type).
			Unique().
			Required().
			Field("document_id").
			Comment("Document target for this association."),
		edge.To("latest_evidence", Evidence.Type).
			Unique().
			Field("latest_evidence_id").
			Comment("Most recent evidence supporting this association."),
	}
}

// Indexes supports fast paged document reads from a pane.
func (PaneDocumentLink) Indexes() []ent.Index {
	return []ent.Index{
		index.Fields("pane_id", "freshness_state", "rank_score", "last_activity_at"),
		index.Fields("pane_id", "relation_kind", "last_activity_at"),
		index.Fields("document_id"),
	}
}
