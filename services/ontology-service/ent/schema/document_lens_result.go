package schema

import (
	"entgo.io/ent"
	entschema "entgo.io/ent/schema"
	"entgo.io/ent/schema/edge"
	"entgo.io/ent/schema/field"
	"entgo.io/ent/schema/index"
)

// DocumentLensResult is the ranked association from a work lens to a document.
type DocumentLensResult struct {
	ent.Schema
}

// Annotations declares the composite endpoint identity for the edge schema.
func (DocumentLensResult) Annotations() []entschema.Annotation {
	return edgeSchemaAnnotations("work_lens_id", "document_id")
}

// Fields stores endpoint foreign keys plus ranked document relationship metadata.
func (DocumentLensResult) Fields() []ent.Field {
	return appendFields(
		[]ent.Field{
			field.Int("work_lens_id").
				Immutable().
				Comment("Source WorkLens endpoint for this result."),
			field.Int("work_lens_window_id").
				Immutable().
				Comment("Bounded WorkLensWindow this result is assigned to for paging and recrawl."),
			field.Int("document_id").
				Immutable().
				Comment("Target Document endpoint for this result."),
		},
		linkFields(documentRelationValues()),
	)
}

// Edges connects the result row to its lens, document, and latest evidence.
func (DocumentLensResult) Edges() []ent.Edge {
	return []ent.Edge{
		edge.To("lens", WorkLens.Type).
			Unique().
			Required().
			Immutable().
			Field("work_lens_id").
			Comment("Work lens that owns this document result."),
		edge.From("window", WorkLensWindow.Type).
			Ref("document_results").
			Unique().
			Required().
			Immutable().
			Field("work_lens_window_id").
			Comment("Bounded lens window used to page this document result."),
		edge.To("document", Document.Type).
			Unique().
			Required().
			Immutable().
			Field("document_id").
			Comment("Document target for this result."),
		edge.To("latest_evidence", Evidence.Type).
			Unique().
			Field("latest_evidence_id").
			Comment("Most recent evidence supporting this result."),
	}
}

// Indexes supports fast paged document reads from a lens.
func (DocumentLensResult) Indexes() []ent.Index {
	return []ent.Index{
		index.Fields("work_lens_window_id", "freshness_state", "rank_score", "last_activity_at"),
		index.Fields("work_lens_window_id", "relation_kind", "last_activity_at"),
		index.Fields("work_lens_id", "freshness_state", "rank_score", "last_activity_at"),
		index.Fields("work_lens_id", "relation_kind", "last_activity_at"),
		index.Fields("document_id"),
	}
}
