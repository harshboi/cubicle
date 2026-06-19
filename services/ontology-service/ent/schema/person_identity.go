package schema

import (
	"entgo.io/ent"
	entschema "entgo.io/ent/schema"
	"entgo.io/ent/schema/edge"
	"entgo.io/ent/schema/field"
	"entgo.io/ent/schema/index"
)

// PersonIdentity maps one source-native identity or handle to a canonical
// Person. It keeps source-native people identity out of a generic association table.
//
// Association:
//
//	source identity -> PersonIdentity -> Person
//	PersonIdentity -> Evidence
//
// Identity evidence stays separate from product work relationships.
type PersonIdentity struct {
	ent.Schema
}

// Annotations declares that PersonIdentity is part of identity resolution APIs.
func (PersonIdentity) Annotations() []entschema.Annotation {
	return graphqlAnnotations()
}

// Fields defines source-native identity attributes.
func (PersonIdentity) Fields() []ent.Field {
	return appendFields(
		[]ent.Field{
			field.Int("person_id").
				Immutable().
				Comment("Canonical Person row this identity resolves to."),
			field.String("handle").
				Optional().
				Comment("Human-readable source handle, username, or display identifier."),
			field.String("email").
				Optional().
				Comment("Source email address when available."),
			field.Enum("identity_status").
				Values(identityStatusValues()...).
				Default(identityStatusActive).
				Comment("Lifecycle state for this source identity mapping."),
			field.Int("replaced_by_identity_id").
				Optional().
				Comment("Newer PersonIdentity row that replaces this identity."),
			field.Int("latest_evidence_id").
				Optional().
				Comment("Latest Evidence row that justified this identity mapping."),
			field.Time("first_seen_at").
				Optional().
				Comment("Time Cubicle first observed this identity."),
			field.Time("last_seen_at").
				Optional().
				Comment("Time Cubicle last observed this identity."),
		},
		sourceIdentityFields(),
		timestampFields(),
	)
}

// Edges connects identity rows to Person and optional replacement/evidence.
func (PersonIdentity) Edges() []ent.Edge {
	return []ent.Edge{
		edge.From("person", Person.Type).
			Ref("identities").
			Unique().
			Required().
			Immutable().
			Field("person_id").
			Comment("Canonical person this source identity resolves to."),
		edge.To("replaced_by_identity", PersonIdentity.Type).
			Unique().
			Field("replaced_by_identity_id").
			Comment("Newer identity row that replaces this one."),
		edge.From("replaced_identities", PersonIdentity.Type).
			Ref("replaced_by_identity").
			Comment("Older identity rows replaced by this identity."),
		edge.To("latest_evidence", Evidence.Type).
			Unique().
			Field("latest_evidence_id").
			Comment("Latest proof supporting this identity mapping."),
	}
}

// Indexes supports identity resolution by source identity and person.
func (PersonIdentity) Indexes() []ent.Index {
	return []ent.Index{
		index.Fields("source_system", "source_instance", "external_kind", "external_id").Unique(),
		index.Fields("person_id", "identity_status"),
		index.Fields("email"),
		index.Fields("handle"),
	}
}
