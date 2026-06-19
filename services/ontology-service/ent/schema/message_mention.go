package schema

import (
	"entgo.io/ent"
	entschema "entgo.io/ent/schema"
	"entgo.io/ent/schema/edge"
	"entgo.io/ent/schema/field"
	"entgo.io/ent/schema/index"
)

// MessageMention is the typed relationship from a person to a message that
// mentions, references, or replies to them.
//
// Association:
//
//	Person -> MessageMention -> Message
//	MessageMention -> Evidence
//
// Mention rows preserve weak references without turning them into ownership.
type MessageMention struct {
	ent.Schema
}

// Annotations declares the composite endpoint identity for the edge schema.
func (MessageMention) Annotations() []entschema.Annotation {
	return edgeSchemaAnnotations("person_id", "message_id", "mention_kind")
}

// Fields stores endpoint foreign keys plus mention metadata.
func (MessageMention) Fields() []ent.Field {
	return appendFields(
		[]ent.Field{
			field.Int("person_id").
				Immutable().
				Comment("Person endpoint mentioned by this message."),
			field.Int("message_id").
				Immutable().
				Comment("Message endpoint containing the mention."),
		},
		linkFields("mention_kind", mentionKindValues()),
	)
}

// Edges connects the mention row to endpoints and proof.
func (MessageMention) Edges() []ent.Edge {
	return []ent.Edge{
		edge.To("person", Person.Type).
			Unique().
			Required().
			Immutable().
			Field("person_id").
			Comment("Person mentioned by this message."),
		edge.To("message", Message.Type).
			Unique().
			Required().
			Immutable().
			Field("message_id").
			Comment("Message containing this mention."),
		edge.To("latest_evidence", Evidence.Type).
			Unique().
			Field("latest_evidence_id").
			Comment("Most recent evidence supporting this mention."),
	}
}

// Indexes supports person mention queries and message reverse lookups.
func (MessageMention) Indexes() []ent.Index {
	return []ent.Index{
		semanticEdgeIndex("person_id", "message_id", "mention_kind"),
		index.Fields("person_id", "freshness_state", "rank_score", "last_activity_at"),
		index.Fields("message_id", "mention_kind"),
		index.Fields("message_id", "freshness_state", "rank_score", "last_activity_at"),
	}
}
