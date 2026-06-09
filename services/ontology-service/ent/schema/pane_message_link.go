package schema

import (
	"entgo.io/ent"
	entschema "entgo.io/ent/schema"
	"entgo.io/ent/schema/edge"
	"entgo.io/ent/schema/field"
	"entgo.io/ent/schema/index"
)

// PaneMessageLink is the metadata-bearing association from a work pane to a message.
type PaneMessageLink struct {
	ent.Schema
}

// Annotations declares the composite endpoint identity for the edge schema.
func (PaneMessageLink) Annotations() []entschema.Annotation {
	return edgeSchemaAnnotations("pane_id", "message_id")
}

// Fields stores endpoint foreign keys plus ranked message relationship metadata.
func (PaneMessageLink) Fields() []ent.Field {
	return appendFields(
		[]ent.Field{
			field.Int("pane_id").
				Comment("Source WorkPane endpoint for this association."),
			field.Int("message_id").
				Comment("Target Message endpoint for this association."),
		},
		linkFields(messageRelationValues()),
	)
}

// Edges connects the link row to its pane, message, and latest evidence.
func (PaneMessageLink) Edges() []ent.Edge {
	return []ent.Edge{
		edge.To("pane", WorkPane.Type).
			Unique().
			Required().
			Field("pane_id").
			Comment("Work pane that owns this message association."),
		edge.To("message", Message.Type).
			Unique().
			Required().
			Field("message_id").
			Comment("Message target for this association."),
		edge.To("latest_evidence", Evidence.Type).
			Unique().
			Field("latest_evidence_id").
			Comment("Most recent evidence supporting this association."),
	}
}

// Indexes supports fast paged message reads from a pane.
func (PaneMessageLink) Indexes() []ent.Index {
	return []ent.Index{
		index.Fields("pane_id", "freshness_state", "rank_score", "last_activity_at"),
		index.Fields("pane_id", "relation_kind", "last_activity_at"),
		index.Fields("message_id"),
	}
}
