package schema

import (
	"entgo.io/ent"
	"entgo.io/ent/schema/field"
	"entgo.io/ent/schema/index"
)

// ConnectorCapability describes what a source instance can provide.
type ConnectorCapability struct {
	ent.Schema
}

func (ConnectorCapability) Fields() []ent.Field {
	return []ent.Field{
		field.String("source").NotEmpty(),
		field.String("source_instance").NotEmpty(),
		field.String("slice").Default(""),
		field.String("status").Default("healthy"),
		field.String("display_name").Default(""),
		field.String("object_kinds_json").Default("[]"),
		field.Time("next_allowed_at").Optional(),
		field.String("notes").Default(""),
	}
}

func (ConnectorCapability) Indexes() []ent.Index {
	return []ent.Index{
		index.Fields("source", "source_instance", "slice").Unique(),
	}
}
