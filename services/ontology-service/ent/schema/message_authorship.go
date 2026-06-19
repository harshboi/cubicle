package schema

import (
	"entgo.io/ent"
	entschema "entgo.io/ent/schema"
	"entgo.io/ent/schema/edge"
	"entgo.io/ent/schema/field"
	"entgo.io/ent/schema/index"
)

// MessageAuthorship is the typed relationship from a person to a message.
//
// Association:
//
//	Person -> MessageAuthorship -> Message
//	MessageAuthorship -> Evidence
//
// The relationship row keeps sender/author role and proof attached to the message link.
type MessageAuthorship struct {
	ent.Schema
}

// Annotations declares the composite endpoint identity for the edge schema.
func (MessageAuthorship) Annotations() []entschema.Annotation {
	return edgeSchemaAnnotations("person_id", "message_id", "authorship_kind")
}

// Fields stores endpoint foreign keys plus authorship metadata.
func (MessageAuthorship) Fields() []ent.Field {
	return appendFields(
		[]ent.Field{
			field.Int("person_id").
				Immutable().
				Comment("Person endpoint for this message authorship."),
			field.Int("message_id").
				Immutable().
				Comment("Message endpoint for this authorship."),
		},
		linkFields("authorship_kind", messageAuthorshipKindValues()),
	)
}

// Edges connects the authorship row to endpoints and proof.
func (MessageAuthorship) Edges() []ent.Edge {
	return []ent.Edge{
		edge.To("person", Person.Type).
			Unique().
			Required().
			Immutable().
			Field("person_id").
			Comment("Person endpoint for this authorship."),
		edge.To("message", Message.Type).
			Unique().
			Required().
			Immutable().
			Field("message_id").
			Comment("Message endpoint for this authorship."),
		edge.To("latest_evidence", Evidence.Type).
			Unique().
			Field("latest_evidence_id").
			Comment("Most recent evidence supporting this authorship."),
	}
}

// Indexes supports person message queries and message reverse lookups.
func (MessageAuthorship) Indexes() []ent.Index {
	return []ent.Index{
		semanticEdgeIndex("person_id", "message_id", "authorship_kind"),
		index.Fields("person_id", "freshness_state", "rank_score", "last_activity_at"),
		index.Fields("message_id", "authorship_kind"),
		index.Fields("message_id", "freshness_state", "rank_score", "last_activity_at"),
	}
}
