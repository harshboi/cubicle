package schema

import (
	"time"

	"entgo.io/ent"
	"entgo.io/ent/schema/field"
)

// stableKeyFields returns the natural-key fields used by Cubicle-facing graph
// objects. Ent numeric IDs stay storage details; key is the source-neutral
// address used by importers, GraphQL clients, and future ML crawlers.
func stableKeyFields() []ent.Field {
	return []ent.Field{
		field.String("key").
			NotEmpty().
			Unique().
			Comment("Stable source-neutral key used outside SQLite and Ent internals."),
	}
}

// sourceIdentityFields returns source-native identity fields shared by product
// rows, typed relationship rows, and proof rows. These are not ingestion run
// statistics; they are the address needed to refetch or open the source object.
func sourceIdentityFields() []ent.Field {
	return []ent.Field{
		field.String("source_system").
			Optional().
			Comment("Source system that produced or most recently confirmed this row."),
		field.String("source_instance").
			Optional().
			Comment("Concrete source instance, such as a Jira tenant or GitHub repository."),
		field.String("external_kind").
			Optional().
			Comment("Source-native object kind, such as jira_issue, slack_message, github_pr, or google_doc."),
		field.String("external_id").
			Optional().
			Comment("Source-native identifier before Cubicle normalization."),
		field.String("source_url").
			Optional().
			Comment("Human-readable source URL when the source provides one."),
	}
}

// sourceStateFields returns per-row source state needed for permission,
// deletion, freshness, and refetch gates. It deliberately stores only the
// current source state for this product/proof row; run history belongs to
// SourceSyncRun and SourceScopeState.
func sourceStateFields() []ent.Field {
	return []ent.Field{
		field.String("source_version").
			Optional().
			Comment("Source version, etag, cursor, or revision for the current row state."),
		field.Time("source_updated_at").
			Optional().
			Comment("Source-reported update time for freshness checks."),
		field.String("content_hash").
			Optional().
			Comment("Hash of normalized current source content or relationship evidence."),
		field.Enum("deletion_state").
			Values(deletionStateValues()...).
			Default(deletionStatePresent).
			Comment("Whether the backing source object is currently present, deleted, or unknown."),
		field.Time("deleted_at").
			Optional().
			Comment("Source or Cubicle time when the backing source object was observed deleted."),
		field.String("acl_policy_key").
			Optional().
			Comment("Stable key for the source permission policy namespace used to authorize reads."),
		field.String("visibility_hash").
			Optional().
			Comment("Fingerprint of the effective source ACL/visibility set."),
		field.Enum("acl_state").
			Values(aclStateValues()...).
			Default(aclStateUnknown).
			Comment("Whether permission metadata for this row is current, stale, blocked, or unavailable."),
		field.Time("acl_checked_at").
			Optional().
			Comment("Time Cubicle last checked source permissions for this row."),
		field.Time("freshness_checked_at").
			Optional().
			Comment("Time Cubicle last evaluated freshness for this row."),
		field.Int("source_scope_state_id").
			Optional().
			Comment("Optional SourceScopeState row that explains the latest scope-level coverage without graph traversal."),
		field.Time("last_confirmed_at").
			Optional().
			Comment("Time Cubicle last confirmed this row still exists or remains valid in source."),
		field.Time("last_changed_at").
			Optional().
			Comment("Time Cubicle last observed source content or relationship state change."),
	}
}

// sourceBackedFields returns the full source address and current-state fields
// for rows that are themselves source-backed product facts.
func sourceBackedFields() []ent.Field {
	return appendFields(sourceIdentityFields(), sourceStateFields())
}

// qualityFields returns visibility, freshness, and confidence fields used for
// Glean-style permission/freshness awareness and Palantir-style operational
// explainability.
func qualityFields() []ent.Field {
	return []ent.Field{
		field.Enum("freshness_state").
			Values(freshnessValues()...).
			Default(freshnessUnknown).
			Comment("Freshness state for filtering stale or partial graph facts."),
		field.Enum("visibility").
			Values(visibilityValues()...).
			Default(visibilityUnknown).
			Comment("Visibility class used by future permission-aware query filtering."),
		field.Float("confidence").
			Default(1.0).
			Comment("Confidence score for source-backed or inferred facts."),
	}
}

// objectEvidenceFields returns the bounded proof summary stored on source-backed
// product rows. Full proof history remains queryable from Evidence by claim.
func objectEvidenceFields() []ent.Field {
	return []ent.Field{
		field.Int("latest_evidence_id").
			Optional().
			Comment("Optional Evidence row that most recently justified this product object's current source state."),
		field.Int("evidence_count").
			Default(0).
			Comment("Number of evidence records known to support this product object's current source state."),
	}
}

// textFields returns the summary/search columns that make objects easier for
// LLM crawlers to inspect before vector or FTS search exists.
func textFields() []ent.Field {
	return []ent.Field{
		field.Text("summary").
			Optional().
			Comment("Short object summary for UI, search snippets, and future LLM context."),
		field.Text("search_text").
			Optional().
			Comment("Normalized text used by V0 Ent-filter search before FTS/vector indexes exist."),
	}
}

// activityFields returns ranking and activity metadata for bounded association
// lists. Query code sorts on these fields before loading final target objects.
func activityFields() []ent.Field {
	return []ent.Field{
		field.Int("event_count").
			Default(0).
			Comment("Number of source events collapsed into this graph object or link."),
		field.Time("first_seen_at").
			Optional().
			Comment("Earliest time Cubicle observed this object or relationship."),
		field.Time("last_activity_at").
			Optional().
			Comment("Latest source activity time used for ranking and recency slicing."),
		field.Float("rank_score").
			Default(0).
			Comment("Deterministic ranking score used before ML ranking exists."),
	}
}

// timestampFields returns standard Ent lifecycle fields.
func timestampFields() []ent.Field {
	return []ent.Field{
		field.Time("created_at").
			Default(time.Now).
			Immutable().
			Comment("Time this row was first created in Cubicle storage."),
		field.Time("updated_at").
			Default(time.Now).
			UpdateDefault(time.Now).
			Comment("Time this row was last updated in Cubicle storage."),
	}
}

// evidenceRefField returns the optional latest evidence foreign-key field used
// by relationship rows. It intentionally stores only the latest citation in V0;
// a full many-evidence table can be added when conflicts need first-class UI.
func evidenceRefField() ent.Field {
	return field.Int("latest_evidence_id").
		Optional().
		Comment("Optional Evidence row that most recently justified this relationship.")
}

// appendFields joins field groups while keeping schema definitions readable.
func appendFields(groups ...[]ent.Field) []ent.Field {
	total := 0
	for _, group := range groups {
		total += len(group)
	}
	fields := make([]ent.Field, 0, total)
	for _, group := range groups {
		fields = append(fields, group...)
	}
	return fields
}
