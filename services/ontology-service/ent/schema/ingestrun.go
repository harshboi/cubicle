package schema

import (
	"entgo.io/ent"
	"entgo.io/ent/schema/field"
	"entgo.io/ent/schema/index"
)

// IngestRun stores one logical source crawl attempt.
type IngestRun struct {
	ent.Schema
}

func (IngestRun) Fields() []ent.Field {
	return []ent.Field{
		field.String("run_key").NotEmpty().Unique(),
		field.String("source").NotEmpty(),
		field.String("source_instance").NotEmpty(),
		field.String("slice").Default(""),
		field.String("mapper_version").Default(""),
		field.String("status").NotEmpty(),
		field.Time("started_at"),
		field.Time("completed_at").Optional(),
		field.String("error_code").Default(""),
		field.String("error_message").Default(""),
	}
}

func (IngestRun) Indexes() []ent.Index {
	return []ent.Index{
		index.Fields("source", "source_instance", "slice", "started_at"),
		index.Fields("status"),
	}
}
