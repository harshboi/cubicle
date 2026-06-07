package schema

import (
	"entgo.io/ent"
	"entgo.io/ent/schema/field"
	"entgo.io/ent/schema/index"
)

// Evidence stores explainable text backing graph nodes and edges.
type Evidence struct {
	ent.Schema
}

func (Evidence) Fields() []ent.Field {
	return []ent.Field{
		field.String("evidence_key").NotEmpty().Unique(),
		field.String("run_key").NotEmpty(),
		field.String("source").NotEmpty(),
		field.String("source_instance").NotEmpty(),
		field.String("snapshot_key").Default(""),
		field.String("source_url").Default(""),
		field.String("text_hash").NotEmpty(),
		field.String("summary").Default(""),
		field.String("quoted_text").Default(""),
		field.Float("confidence").Default(0),
		field.Time("observed_at"),
	}
}

func (Evidence) Indexes() []ent.Index {
	return []ent.Index{
		index.Fields("run_key"),
		index.Fields("snapshot_key"),
		index.Fields("source", "source_instance"),
	}
}
