package schema

import (
	"entgo.io/ent"
	entschema "entgo.io/ent/schema"
	"entgo.io/ent/schema/edge"
	"entgo.io/ent/schema/field"
	"entgo.io/ent/schema/index"
)

// ExternalIdentity maps one source-native identity to one Cubicle ontology object.
//
// Canonical object identity remains the typed Ent object ID and stable Cubicle
// key. ExternalIdentity owns source aliases, renames, merges, moved repositories,
// mirrored messages, and copied documents without mutating canonical objects.
type ExternalIdentity struct {
	ent.Schema
}

// Annotations declares ExternalIdentity as an internal provenance schema.
func (ExternalIdentity) Annotations() []entschema.Annotation {
	return nil
}

// Fields defines the source identity tuple and target ontology pointer.
func (ExternalIdentity) Fields() []ent.Field {
	return appendFields(
		[]ent.Field{
			field.String("target_kind").
				NotEmpty().
				Immutable().
				Comment("Cubicle ontology kind this source identity resolves to, such as ticket or document."),
			field.Int("target_id").
				Immutable().
				Comment("Typed Ent object ID this source identity resolves to."),
			field.String("source_key").
				NotEmpty().
				Immutable().
				Comment("Source family such as slack, jira, github, or google_docs."),
			field.String("source_instance").
				NotEmpty().
				Immutable().
				Comment("Source namespace such as workspace, tenant, account, or repository owner."),
			field.String("external_kind").
				NotEmpty().
				Immutable().
				Comment("Source item kind such as jira_issue, slack_message, google_doc, or github_pr."),
			field.String("external_id").
				NotEmpty().
				Immutable().
				Comment("Source-owned identifier, key, URL tuple, or provider ID for this item."),
			field.Enum("identity_status").
				Values(identityStatusValues()...).
				Default(identityStatusActive).
				Comment("Lifecycle status of this source identity mapping."),
			field.Time("first_seen_at").
				Optional().
				Comment("First time Cubicle observed this source identity mapping."),
			field.Time("last_seen_at").
				Optional().
				Comment("Most recent time Cubicle observed this source identity mapping."),
			field.Int("replaced_by_identity_id").
				Optional().
				Comment("Replacement ExternalIdentity row for source renames, moves, merges, or redirects."),
		},
		timestampFields(),
	)
}

// Edges links identity aliases to observations and optional replacements.
func (ExternalIdentity) Edges() []ent.Edge {
	return []ent.Edge{
		edge.To("observations", SourceObservation.Type).
			Comment("Source observations recorded for this external identity."),
		edge.To("replaced_by_identity", ExternalIdentity.Type).
			Unique().
			Field("replaced_by_identity_id").
			Comment("Replacement identity when this source identity is retired, merged, or redirected."),
		edge.From("replaced_identities", ExternalIdentity.Type).
			Ref("replaced_by_identity").
			Comment("Historical identities that now redirect to this identity."),
	}
}

// Indexes keeps source identity resolution deterministic and source aliases listable.
func (ExternalIdentity) Indexes() []ent.Index {
	return []ent.Index{
		index.Fields("source_key", "source_instance", "external_kind", "external_id").Unique(),
		index.Fields("target_kind", "target_id", "identity_status"),
		index.Fields("replaced_by_identity_id"),
	}
}
