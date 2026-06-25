package schema

import (
	"entgo.io/ent"
	"entgo.io/ent/dialect/entsql"
	entschema "entgo.io/ent/schema"
	"entgo.io/ent/schema/edge"
	"entgo.io/ent/schema/field"
	"entgo.io/ent/schema/index"
)

// WorkstreamStandupSection records one generated agenda row in a TPM standup.
//
// WorkstreamHealthSnapshot stores the operating rollup. These section rows keep
// the ordered agenda durable: what should be discussed, who likely owns it,
// whether it is product-ready or validation-gated, and which action/evidence
// supports the row.
type WorkstreamStandupSection struct {
	ent.Schema
}

// Annotations declares WorkstreamStandupSection as a public operating view.
func (WorkstreamStandupSection) Annotations() []entschema.Annotation {
	return graphqlAnnotations()
}

// Fields defines the ordered generated standup agenda row.
func (WorkstreamStandupSection) Fields() []ent.Field {
	return appendFields(
		stableKeyFields(),
		[]ent.Field{
			field.Int("workstream_health_snapshot_id").
				Optional().
				Comment("Optional WorkstreamHealthSnapshot this agenda row belongs to."),
			field.Int("workstream_id").
				Optional().
				Comment("Optional Workstream row this agenda item summarizes."),
			field.Int("work_action_id").
				Optional().
				Comment("Optional WorkAction ledger row that this agenda item is derived from."),
			field.String("workstream_key").
				NotEmpty().
				Comment("Stable workstream key summarized by this agenda item."),
			field.Time("generated_at").
				Optional().
				Comment("Analytics generation time for this standup agenda item."),
			field.Int("section_rank").
				Comment("1-based display rank within the generated standup agenda."),
			field.Enum("section_kind").
				Values(workstreamStandupSectionKindValues()...).
				Default(workstreamStandupSectionTopAction).
				Comment("Machine-readable standup section type."),
			field.Enum("urgency").
				Values(workstreamStandupUrgencyValues()...).
				Default(workstreamStandupUrgencyUnknown).
				Comment("User-facing urgency bucket for agenda ordering."),
			field.String("owner_key").
				Optional().
				Comment("Likely owner or DRI key for this standup item."),
			field.Enum("subject_kind").
				Values(workInsightSubjectKindValues()...).
				Default(workInsightSubjectUnknown).
				Comment("Typed product kind this agenda item is about, when known."),
			field.String("subject_key").
				Optional().
				Comment("Stable product subject key this agenda item is about."),
			field.String("action_type").
				Optional().
				Comment("Action or generated agenda type, when this row has one."),
			field.String("status_signal").
				Optional().
				Comment("Current status signal that explains why this row appears in standup."),
			field.Text("summary").
				NotEmpty().
				Comment("Short agenda summary suitable for standup display."),
			field.Text("recommended_action").
				Optional().
				Comment("Concrete TPM follow-up recommended by the generated standup."),
			field.String("evidence_ref").
				Optional().
				Comment("Human-readable source or generated evidence reference for display."),
		},
		sourceIdentityFields(),
		objectEvidenceFields(),
		qualityFields(),
		activityFields(),
		timestampFields(),
	)
}

// Edges connects the agenda row to its rollup, workstream, action, and proof.
func (WorkstreamStandupSection) Edges() []ent.Edge {
	return []ent.Edge{
		edge.To("workstream_health_snapshot", WorkstreamHealthSnapshot.Type).
			Unique().
			Field("workstream_health_snapshot_id").
			Annotations(entsql.OnDelete(entsql.SetNull)).
			Comment("Operating rollup this standup row belongs to."),
		edge.To("workstream", Workstream.Type).
			Unique().
			Field("workstream_id").
			Annotations(entsql.OnDelete(entsql.SetNull)).
			Comment("Workstream summarized by this agenda item."),
		edge.To("work_action", WorkAction.Type).
			Unique().
			Field("work_action_id").
			Annotations(entsql.OnDelete(entsql.SetNull)).
			Comment("Action ledger row this agenda item asks someone to handle."),
		edge.To("latest_evidence", Evidence.Type).
			Unique().
			Field("latest_evidence_id").
			Comment("Most recent evidence supporting this agenda item."),
	}
}

// Indexes supports newest-first standup reads and source identity upserts.
func (WorkstreamStandupSection) Indexes() []ent.Index {
	return []ent.Index{
		index.Fields("workstream_key", "generated_at", "section_rank"),
		index.Fields("workstream_health_snapshot_id", "section_rank"),
		index.Fields("workstream_id", "generated_at", "section_rank"),
		index.Fields("section_kind", "urgency", "generated_at"),
		index.Fields("work_action_id"),
		index.Fields("source_system", "source_instance", "external_kind", "external_id").Unique(),
	}
}
