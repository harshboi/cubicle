package schema

import (
	"entgo.io/ent"
	"entgo.io/ent/schema/field"
	"entgo.io/ent/schema/index"
)

// SourceSnapshot stores a content-addressed raw source object reference.
type SourceSnapshot struct {
	ent.Schema
}

func (SourceSnapshot) Fields() []ent.Field {
	return []ent.Field{
		field.String("snapshot_key").NotEmpty().Unique(),
		field.String("run_key").NotEmpty(),
		field.String("source").NotEmpty(),
		field.String("source_instance").NotEmpty(),
		field.String("source_object_type").Default(""),
		field.String("source_object_id").Default(""),
		field.String("body_sha256").NotEmpty(),
		field.String("body_ref").NotEmpty(),
		field.String("source_url").Default(""),
		field.Time("fetched_at"),
		field.String("headers_json").Default(""),
		field.Time("created_at"),
	}
}

func (SourceSnapshot) Indexes() []ent.Index {
	return []ent.Index{
		index.Fields("run_key"),
		index.Fields("source", "source_instance", "source_object_type", "source_object_id"),
	}
}
