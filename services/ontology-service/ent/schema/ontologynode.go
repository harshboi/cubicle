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

func (OntologyNode) Indexes() []ent.Index {
	return []ent.Index{
		index.Fields("kind", "key").Unique(),
		index.Fields("source", "external_id"),
		index.Fields("source", "source_instance", "snapshot_key"),
	}
}
