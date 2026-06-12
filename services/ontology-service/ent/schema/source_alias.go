package schema

import (
	"entgo.io/ent"
	entschema "entgo.io/ent/schema"
	"entgo.io/ent/schema/edge"
	"entgo.io/ent/schema/field"
	"entgo.io/ent/schema/index"
)

// SourceAlias maps moved/renamed source-native object identifiers to a stable
// product key. It is a lookup table, not a product graph relationship.
type SourceAlias struct {
	ent.Schema
}

// Annotations declares that SourceAlias is storage-visible for resolver tools.
func (SourceAlias) Annotations() []entschema.Annotation {
	return graphqlAnnotations()
}

// Fields defines an alias from source address to product key.
func (SourceAlias) Fields() []ent.Field {
	return appendFields(
		[]ent.Field{
			field.String("product_kind").
				NotEmpty().
				Comment("Product row kind, such as ticket, document, message, or pull_request."),
			field.String("product_key").
				NotEmpty().
				Comment("Stable Cubicle product key this source alias resolves to."),
			field.Enum("alias_status").
				Values(identityStatusValues()...).
				Default(identityStatusActive).
				Comment("Lifecycle state for this source alias."),
			field.Int("replaced_by_alias_id").
				Optional().
				Comment("Newer SourceAlias row that replaces this alias."),
			field.Time("first_seen_at").
				Optional().
				Comment("Time Cubicle first observed this alias."),
			field.Time("last_seen_at").
				Optional().
				Comment("Time Cubicle last observed this alias."),
		},
		sourceIdentityFields(),
		timestampFields(),
	)
}

// Edges connects alias replacements without linking aliases into the product graph.
func (SourceAlias) Edges() []ent.Edge {
	return []ent.Edge{
		edge.To("replaced_by_alias", SourceAlias.Type).
			Unique().
			Field("replaced_by_alias_id").
			Comment("Newer alias row that replaces this one."),
		edge.From("replaced_aliases", SourceAlias.Type).
			Ref("replaced_by_alias").
			Comment("Older aliases replaced by this alias."),
	}
}

// Indexes supports source-address lookup and product-key reverse lookup.
func (SourceAlias) Indexes() []ent.Index {
	return []ent.Index{
		index.Fields("source_system", "source_instance", "external_kind", "external_id").Unique(),
		index.Fields("product_kind", "product_key", "alias_status"),
	}
}
