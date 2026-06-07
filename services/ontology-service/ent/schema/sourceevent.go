package schema

import (
	"entgo.io/ent"
	"entgo.io/ent/schema/field"
	"entgo.io/ent/schema/index"
)

// SourceEvent stores durable source-level events before they become graph facts.
type SourceEvent struct {
	ent.Schema
}

func (SourceEvent) Fields() []ent.Field {
	return []ent.Field{
		field.String("event_key").NotEmpty().Unique(),
		field.String("run_key").NotEmpty(),
		field.String("source").NotEmpty(),
		field.String("source_instance").NotEmpty(),
		field.String("snapshot_key").Default(""),
		field.String("source_object_type").Default(""),
		field.String("source_object_id").Default(""),
		field.String("event_type").NotEmpty(),
		field.Time("observed_at"),
		field.String("payload_json").Default(""),
	}
}

func (SourceEvent) Indexes() []ent.Index {
	return []ent.Index{
		index.Fields("run_key"),
		index.Fields("snapshot_key"),
		index.Fields("source", "source_instance", "source_object_type", "source_object_id"),
	}
}
