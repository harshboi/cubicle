package schema

import (
	"entgo.io/ent"
	entschema "entgo.io/ent/schema"
	"entgo.io/ent/schema/edge"
	"entgo.io/ent/schema/field"
	"entgo.io/ent/schema/index"
)

// MessageLensResult is the ranked association from a work lens window to a message.
//
// Association:
//
//	WorkArea -> WorkLens -> WorkLensWindow -> MessageLensResult -> Message
//
// The window parent keeps message results pageable without a direct WorkLens
// to Message fanout.
type MessageLensResult struct {
	ent.Schema
}

// Annotations declares the composite endpoint identity for the edge schema.
func (MessageLensResult) Annotations() []entschema.Annotation {
	return edgeSchemaAnnotations("work_lens_id", "message_id", "relation_kind")
}

// Fields stores endpoint foreign keys plus ranked message relationship metadata.
func (MessageLensResult) Fields() []ent.Field {
	return appendFields(
		[]ent.Field{
			field.Int("work_lens_id").
				Immutable().
				Comment("Source WorkLens endpoint for this result."),
			field.Int("work_lens_window_id").
				Immutable().
				Comment("Bounded WorkLensWindow this result is assigned to for paging and materialization."),
			field.Int("message_id").
				Immutable().
				Comment("Target Message endpoint for this result."),
		},
		linkFields("relation_kind", messageRelationValues()),
	)
}

// Edges connects the result row to its lens, message, and latest evidence.
func (MessageLensResult) Edges() []ent.Edge {
	return []ent.Edge{
		edge.To("lens", WorkLens.Type).
			Unique().
			Required().
			Immutable().
			Field("work_lens_id").
			Comment("Work lens that owns this message result."),
		edge.From("window", WorkLensWindow.Type).
			Ref("message_results").
			Unique().
			Required().
			Immutable().
			Field("work_lens_window_id").
			Comment("Bounded lens window used to page this message result."),
		edge.To("message", Message.Type).
			Unique().
			Required().
			Immutable().
			Field("message_id").
			Comment("Message target for this result."),
		edge.To("latest_evidence", Evidence.Type).
			Unique().
			Field("latest_evidence_id").
			Comment("Most recent evidence supporting this result."),
	}
}

// Indexes supports fast paged message reads from a lens.
func (MessageLensResult) Indexes() []ent.Index {
	return []ent.Index{
		semanticEdgeIndex("work_lens_id", "message_id", "relation_kind"),
		index.Fields("work_lens_window_id", "freshness_state", "rank_score", "last_activity_at"),
		index.Fields("work_lens_window_id", "relation_kind", "last_activity_at"),
		index.Fields("work_lens_id", "freshness_state", "rank_score", "last_activity_at"),
		index.Fields("work_lens_id", "relation_kind", "last_activity_at"),
		index.Fields("message_id", "freshness_state", "rank_score", "last_activity_at"),
	}
}
