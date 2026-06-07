package schema

import (
	"entgo.io/ent"
	"entgo.io/ent/schema/field"
	"entgo.io/ent/schema/index"
)

// Object stores one source-neutral ontology object.
//
// This follows the Meta/TAO vocabulary Cubicle is adopting: typed objects are
// the durable graph vertices, and higher-level ontology code owns the allowed
// object type vocabulary.
type Object struct {
	ent.Schema
}

func (Object) Fields() []ent.Field {
	return []ent.Field{
		field.String("key").NotEmpty().Unique(),
		field.String("object_type").NotEmpty(),
		field.String("title").Default(""),
		field.String("source").Default(""),
		field.String("source_instance").Default(""),
		field.String("external_id").Default(""),
		field.String("source_url").Default(""),
		field.String("snapshot_key").Default(""),
		field.String("mapper_version").Default(""),
		field.String("visibility").Default(""),
		field.String("freshness_state").Default(""),
		field.Time("observed_at"),
		field.Time("source_updated_at").Optional(),
		field.String("properties_json").Default(""),
	}
}

func (Object) Indexes() []ent.Index {
	return []ent.Index{
		index.Fields("object_type", "key").Unique(),
		index.Fields("source", "external_id"),
		index.Fields("source", "source_instance", "snapshot_key"),
	}
}
