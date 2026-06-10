package schema

import (
	"entgo.io/ent"
	entschema "entgo.io/ent/schema"
	"entgo.io/ent/schema/edge"
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
			field.Int("evidence_anchor_id").
				Optional().
				Comment("Optional EvidenceAnchor row that contains the exact source span for this graph claim."),
		},
		sourceFields(),
		qualityFields(),
		timestampFields(),
	)
}

// Edges connects Evidence to the exact source anchor it cites.
func (Evidence) Edges() []ent.Edge {
	return []ent.Edge{
		edge.To("evidence_anchor", EvidenceAnchor.Type).
			Unique().
			Field("evidence_anchor_id").
			Comment("Exact source span cited by this evidence row."),
	}
}

// Indexes supports citation lookup by source and content hash.
func (Evidence) Indexes() []ent.Index {
	return []ent.Index{
		index.Fields("source", "source_instance"),
		index.Fields("text_hash"),
		index.Fields("evidence_anchor_id"),
	}
}
