package schema

import (
	"entgo.io/ent"
	entschema "entgo.io/ent/schema"
	"entgo.io/ent/schema/edge"
	"entgo.io/ent/schema/field"
	"entgo.io/ent/schema/index"
)

// UnresolvedReference stores source references that did not become typed
// product graph relationships. Resolved references should materialize a typed
// edge such as TicketPullRequest or TicketMessage instead of living here.
type UnresolvedReference struct {
	ent.Schema
}

// Annotations declares that UnresolvedReference is storage-visible for resolver tools.
func (UnresolvedReference) Annotations() []entschema.Annotation {
	return graphqlAnnotations()
}

// Fields defines a resolver/candidate queue entry.
func (UnresolvedReference) Fields() []ent.Field {
	return appendFields(
		[]ent.Field{
			field.String("from_product_kind").
				NotEmpty().
				Comment("Product row kind where the reference was observed."),
			field.Int("from_product_id").
				Optional().
				Comment("Optional Ent row ID where the reference was observed."),
			field.String("from_product_key").
				Optional().
				Comment("Stable product key where the reference was observed."),
			field.Enum("reference_kind").
				Values(referenceKindValues()...).
				Default(referenceURL).
				Comment("Kind of source reference observed."),
			field.String("raw_ref").
				NotEmpty().
				Comment("Raw reference text found in source content."),
			field.String("normalized_ref").
				Optional().
				Comment("Normalized reference value used by resolvers."),
			field.Enum("resolution_state").
				Values(referenceResolutionValues()...).
				Default(referenceResolutionUnresolved).
				Comment("Current resolver state for this reference."),
			field.String("resolver").
				Optional().
				Comment("Resolver or parser that produced this candidate."),
			field.Int("latest_evidence_id").
				Optional().
				Comment("Evidence row that shows where this reference was observed."),
		},
		sourceIdentityFields(),
		timestampFields(),
	)
}

// Edges connects an unresolved reference to its proof only.
func (UnresolvedReference) Edges() []ent.Edge {
	return []ent.Edge{
		edge.To("latest_evidence", Evidence.Type).
			Unique().
			Field("latest_evidence_id").
			Comment("Proof showing where this unresolved reference was observed."),
	}
}

// Indexes supports resolver queue scans and source-local dedupe.
func (UnresolvedReference) Indexes() []ent.Index {
	return []ent.Index{
		index.Fields("from_product_kind", "from_product_id", "reference_kind", "raw_ref").Unique(),
		index.Fields("resolution_state", "reference_kind", "updated_at"),
		index.Fields("source_system", "source_instance", "external_kind", "external_id"),
	}
}
