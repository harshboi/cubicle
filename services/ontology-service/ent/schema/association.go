package schema

import (
	"entgo.io/ent"
	"entgo.io/ent/schema/field"
	"entgo.io/ent/schema/index"
)

// Association stores a metadata-rich relationship between ontology objects.
//
// In TAO terms, association_type is the relationship identity. Evidence and
// freshness fields describe why Cubicle believes the association exists.
type Association struct {
	ent.Schema
}

func (Association) Fields() []ent.Field {
	return []ent.Field{
		field.String("key").NotEmpty().Unique(),
		field.String("from_object_type").NotEmpty(),
		field.String("from_object_key").NotEmpty(),
		field.String("to_object_type").NotEmpty(),
		field.String("to_object_key").NotEmpty(),
		field.String("association_type").NotEmpty(),
		field.String("evidence_key").NotEmpty(),
		field.String("source").Default(""),
		field.Float("confidence").Default(0),
		field.String("visibility").Default(""),
		field.String("freshness_state").Default(""),
		field.Time("observed_at"),
	}
}

func (Association) Indexes() []ent.Index {
	return []ent.Index{
		index.Fields("from_object_type", "from_object_key", "association_type"),
		index.Fields("to_object_type", "to_object_key", "association_type"),
		index.Fields("evidence_key"),
	}
}
