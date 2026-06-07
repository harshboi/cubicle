package schema

import (
	"entgo.io/ent"
	"entgo.io/ent/schema/field"
	"entgo.io/ent/schema/index"
)

// SourceError records source failures without conflating them with graph facts.
type SourceError struct {
	ent.Schema
}

func (SourceError) Fields() []ent.Field {
	return []ent.Field{
		field.String("error_key").NotEmpty().Unique(),
		field.String("run_key").Default(""),
		field.String("source").NotEmpty(),
		field.String("source_instance").NotEmpty(),
		field.String("slice").Default(""),
		field.String("category").NotEmpty(),
		field.String("message").Default(""),
		field.String("source_url").Default(""),
		field.Bool("retriable").Default(false),
		field.Time("occurred_at"),
		field.String("payload_json").Default(""),
	}
}

func (SourceError) Indexes() []ent.Index {
	return []ent.Index{
		index.Fields("source", "source_instance", "slice", "occurred_at"),
		index.Fields("run_key"),
		index.Fields("category"),
	}
}
