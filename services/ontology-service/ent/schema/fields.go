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

// sourceFields returns optional source identity fields shared by objects and
// links. These fields keep every query-facing row explainable without requiring
// callers to load raw crawler snapshots.
func sourceFields() []ent.Field {
	return []ent.Field{
		field.String("source").
			Optional().
			Comment("Source system that produced or most recently confirmed this row."),
		field.String("source_instance").
			Optional().
			Comment("Concrete source instance, such as a Jira tenant or GitHub repository."),
		field.String("external_id").
			Optional().
			Comment("Source-native identifier before Cubicle normalization."),
		field.String("source_url").
			Optional().
			Comment("Human-readable source URL when the source provides one."),
	}
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
