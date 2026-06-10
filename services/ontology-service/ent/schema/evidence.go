package schema

import (
	"entgo.io/ent"
	entschema "entgo.io/ent/schema"
	"entgo.io/ent/schema/field"
	"entgo.io/ent/schema/index"
)

// Evidence is a citation/provenance row that explains why Cubicle believes a fact.
type Evidence struct {
	ent.Schema
}

// Annotations declares that Evidence is part of the future public entgql API.
func (Evidence) Annotations() []entschema.Annotation {
	return graphqlAnnotations()
}

// Fields defines citation metadata shared by object and relationship claims.
func (Evidence) Fields() []ent.Field {
	return appendFields(
		stableKeyFields(),
		[]ent.Field{
			field.Text("excerpt").
				Optional().
				Comment("Short source excerpt or citation text."),
			field.String("text_hash").
				Optional().
				Comment("Hash of normalized evidence text for idempotency checks."),
			field.Time("observed_at").
				Optional().
				Comment("Time Cubicle observed this evidence."),
			field.Time("source_updated_at").
				Optional().
				Comment("Source-reported update time for freshness checks."),
		},
		sourceFields(),
		qualityFields(),
		timestampFields(),
	)
}

// Indexes supports citation lookup by source and content hash.
func (Evidence) Indexes() []ent.Index {
	return []ent.Index{
		index.Fields("source", "source_instance"),
		index.Fields("text_hash"),
	}
}
