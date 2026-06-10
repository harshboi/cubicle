package schema

import (
	"entgo.io/ent"
	entschema "entgo.io/ent/schema"
	"entgo.io/ent/schema/edge"
	"entgo.io/ent/schema/field"
	"entgo.io/ent/schema/index"
)

// SourceObservation records how one source item looked during one SourceRun.
//
// This row is the authority for item-level source state: observed time,
// source-updated time, deletion/tombstone state, visibility hash, permission
// policy, source URL, and normalized content hash.
type SourceObservation struct {
	ent.Schema
}

// Annotations declares SourceObservation as an internal provenance schema.
func (SourceObservation) Annotations() []entschema.Annotation {
	return nil
}

// Fields defines the observed source item state and permission fingerprint.
func (SourceObservation) Fields() []ent.Field {
	return appendFields(
		[]ent.Field{
			field.Int("source_run_id").
				Immutable().
				Comment("SourceRun row that produced this source observation."),
			field.Int("external_identity_id").
				Immutable().
				Comment("ExternalIdentity row whose source item was observed."),
			field.String("observed_kind").
				NotEmpty().
				Immutable().
				Comment("Source item kind observed, such as jira_issue, slack_message, google_doc, or github_pr."),
			field.Time("observed_at").
				Optional().
				Comment("Time Cubicle observed this source item."),
			field.Time("source_updated_at").
				Optional().
				Comment("Source-reported update time for this item when available."),
			field.Bool("is_deleted").
				Default(false).
				Comment("Whether the source reported this item as deleted or tombstoned."),
			field.Time("deleted_at").
				Optional().
				Comment("Source deletion or tombstone time when known."),
			field.String("permission_policy_key").
				Default(permissionPolicyUnknown).
				NotEmpty().
				Comment("Named source permission policy used to interpret the visibility hash."),
			field.String("visibility_hash").
				Default(visibilityUnknown).
				NotEmpty().
				Comment("Stable fingerprint of source ACL or visibility state for permission filtering."),
			field.String("source_url").
				Optional().
				Comment("Deep link or source URL for opening this observed item."),
			field.String("content_hash").
				NotEmpty().
				Comment("Hash of normalized observed source content or metadata envelope."),
		},
		timestampFields(),
	)
}

// Edges connects an observation to its run, source identity, and citeable anchors.
func (SourceObservation) Edges() []ent.Edge {
	return []ent.Edge{
		edge.From("source_run", SourceRun.Type).
			Ref("observations").
			Unique().
			Required().
			Immutable().
			Field("source_run_id").
			Comment("Source run that produced this observation."),
		edge.From("external_identity", ExternalIdentity.Type).
			Ref("observations").
			Unique().
			Required().
			Immutable().
			Field("external_identity_id").
			Comment("External identity observed by this source item state."),
		edge.To("evidence_anchors", EvidenceAnchor.Type).
			Comment("Exact evidence anchors inside this observed source item."),
	}
}

// Indexes supports observation history, run inspection, and permission filters.
func (SourceObservation) Indexes() []ent.Index {
	return []ent.Index{
		index.Fields("source_run_id", "external_identity_id").Unique(),
		index.Fields("external_identity_id", "observed_at"),
		index.Fields("source_run_id", "observed_kind"),
		index.Fields("permission_policy_key", "visibility_hash"),
	}
}
