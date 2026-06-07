package schema

import (
	"entgo.io/ent"
	"entgo.io/ent/schema/field"
	"entgo.io/ent/schema/index"
)

// OntologyNode stores one source-neutral graph object.
type OntologyNode struct {
	ent.Schema
}

func (OntologyNode) Fields() []ent.Field {
	return []ent.Field{
		field.String("key").NotEmpty().Unique(),
		field.String("kind").NotEmpty(),
		field.String("title").Default(""),
		field.String("source").Default(""),
		field.String("external_id").Default(""),
		field.String("visibility").Default(""),
		field.String("freshness_state").Default(""),
		field.Time("observed_at"),
	}
}

func (OntologyNode) Indexes() []ent.Index {
	return []ent.Index{
		index.Fields("kind", "key").Unique(),
		index.Fields("source", "external_id"),
	}
}
