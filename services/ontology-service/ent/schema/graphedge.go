package schema

import (
	"entgo.io/ent"
	"entgo.io/ent/schema/field"
	"entgo.io/ent/schema/index"
)

// GraphEdge stores a metadata-rich association between ontology nodes.
type GraphEdge struct {
	ent.Schema
}

func (GraphEdge) Fields() []ent.Field {
	return []ent.Field{
		field.String("key").NotEmpty().Unique(),
		field.String("from_kind").NotEmpty(),
		field.String("from_key").NotEmpty(),
		field.String("to_kind").NotEmpty(),
		field.String("to_key").NotEmpty(),
		field.String("predicate").NotEmpty(),
		field.String("evidence_key").NotEmpty(),
		field.String("source").Default(""),
		field.Float("confidence").Default(0),
		field.String("visibility").Default(""),
		field.String("freshness_state").Default(""),
		field.Time("observed_at"),
	}
}

func (GraphEdge) Indexes() []ent.Index {
	return []ent.Index{
		index.Fields("from_key", "predicate"),
		index.Fields("to_key", "predicate"),
		index.Fields("evidence_key"),
	}
}
