package schema

import (
	"entgo.io/ent"
	"entgo.io/ent/schema/field"
	"entgo.io/ent/schema/index"
)

// SourceCheckpoint records the last durable cursor for a source slice.
type SourceCheckpoint struct {
	ent.Schema
}

func (SourceCheckpoint) Fields() []ent.Field {
	return []ent.Field{
		field.String("source").NotEmpty(),
		field.String("source_instance").NotEmpty(),
		field.String("slice").Default(""),
		field.String("status").Default("healthy"),
		field.String("checkpoint_key").Default(""),
		field.String("checkpoint_value").Default(""),
		field.String("last_successful_run_key").Default(""),
		field.String("last_attempted_run_key").Default(""),
		field.String("last_error_key").Default(""),
		field.Time("next_allowed_at").Optional(),
		field.String("object_counts_json").Default("{}"),
		field.Time("updated_at"),
	}
}

func (SourceCheckpoint) Indexes() []ent.Index {
	return []ent.Index{
		index.Fields("source", "source_instance", "slice").Unique(),
		index.Fields("last_successful_run_key"),
		index.Fields("last_error_key"),
	}
}
