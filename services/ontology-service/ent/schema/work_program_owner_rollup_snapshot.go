package schema

import (
	"entgo.io/ent"
	"entgo.io/ent/dialect/entsql"
	entschema "entgo.io/ent/schema"
	"entgo.io/ent/schema/edge"
	"entgo.io/ent/schema/field"
	"entgo.io/ent/schema/index"
)

// WorkProgramOwnerRollupSnapshot records durable owner routing for a TPM register run.
//
// WorkProgramItem remains the typed source of product detail. This snapshot
// persists the aggregate "who owns what next" surface so AI-TPM reads do not
// recompute owner load from raw analytics tables on every request.
type WorkProgramOwnerRollupSnapshot struct {
	ent.Schema
}

// Annotations declares WorkProgramOwnerRollupSnapshot as a public operating view.
func (WorkProgramOwnerRollupSnapshot) Annotations() []entschema.Annotation {
	return graphqlAnnotations()
}

// Fields defines one owner rollup row within a generated work-program run.
func (WorkProgramOwnerRollupSnapshot) Fields() []ent.Field {
	return appendFields(
		stableKeyFields(),
		[]ent.Field{
			field.Int("workstream_id").
				Optional().
				Comment("Optional Workstream row this owner rollup belongs to."),
			field.String("workstream_key").
				NotEmpty().
				Comment("Stable workstream key summarized by this owner rollup."),
			field.Time("generated_at").
				Optional().
				Comment("Analytics generation time for this owner rollup snapshot."),
			field.String("owner_key").
				NotEmpty().
				Comment("Owner or DRI key for this program-register rollup."),
			field.String("owner_source").
				Optional().
				Comment("How owner_key was chosen for this rollup."),
			field.Int("item_count").
				Default(0).
				Comment("Typed WorkProgramItem count routed to this owner."),
			field.Int("needs_decision_count").Default(0),
			field.Int("validate_signal_count").Default(0),
			field.Int("ci_failing_count").Default(0),
			field.Int("waiting_review_count").Default(0),
			field.Int("source_repair_count").Default(0),
			field.Int("closure_candidate_count").Default(0),
			field.Int("now_count").Default(0),
			field.Int("high_risk_count").Default(0),
			field.Float("max_risk_score").
				Default(0).
				Comment("Highest risk score among this owner's typed program items."),
			field.Text("top_item_keys").
				Optional().
				Comment("Line-delimited WorkProgramItem stable keys for the owner's top typed drilldowns."),
		},
		sourceIdentityFields(),
		objectEvidenceFields(),
		qualityFields(),
		activityFields(),
		timestampFields(),
	)
}

// Edges connects the owner rollup to its workstream and supporting evidence.
func (WorkProgramOwnerRollupSnapshot) Edges() []ent.Edge {
	return []ent.Edge{
		edge.To("workstream", Workstream.Type).
			Unique().
			Field("workstream_id").
			Annotations(entsql.OnDelete(entsql.SetNull)).
			Comment("Workstream this owner rollup belongs to."),
		edge.To("latest_evidence", Evidence.Type).
			Unique().
			Field("latest_evidence_id").
			Comment("Most recent evidence supporting this generated owner rollup."),
	}
}

// Indexes supports latest owner-rollup reads and source identity upserts.
func (WorkProgramOwnerRollupSnapshot) Indexes() []ent.Index {
	return []ent.Index{
		index.Fields("workstream_key", "generated_at"),
		index.Fields("owner_key", "generated_at"),
		index.Fields("workstream_id", "generated_at"),
		index.Fields("source_system", "source_instance", "external_kind", "external_id").Unique(),
	}
}
