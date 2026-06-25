package schema

import (
	"entgo.io/ent"
	"entgo.io/ent/dialect/entsql"
	entschema "entgo.io/ent/schema"
	"entgo.io/ent/schema/edge"
	"entgo.io/ent/schema/field"
	"entgo.io/ent/schema/index"
)

// WorkProgramRunMember records one row included in an AI-TPM operating run.
//
// Members keep the run boundary inspectable even while individual packet
// tables remain separate typed projections.
type WorkProgramRunMember struct {
	ent.Schema
}

// Annotations declares WorkProgramRunMember as a public operating view.
func (WorkProgramRunMember) Annotations() []entschema.Annotation {
	return graphqlAnnotations()
}

// Fields defines one run membership pointer.
func (WorkProgramRunMember) Fields() []ent.Field {
	return []ent.Field{
		field.Int("work_program_run_id").
			Comment("Ent run row this membership belongs to."),
		field.String("run_key").
			NotEmpty().
			Comment("Stable run key, retained for replay compatibility."),
		field.String("member_table").
			NotEmpty().
			Comment("Physical table containing the member row."),
		field.Int("member_id").
			Comment("Numeric row ID in member_table at generation time."),
		field.String("member_key").
			Optional().
			Comment("Stable key of the member row when available."),
		field.String("member_external_kind").
			Optional().
			Comment("External kind of the member row when available."),
		field.String("member_external_id").
			Optional().
			Comment("External ID of the member row when available."),
		field.Float("member_rank_score").
			Optional().
			Comment("Rank score copied from the member row for deterministic run inspection."),
		field.Time("created_at").
			Comment("Time this run membership was generated."),
	}
}

// Edges connects the member to its owning run.
func (WorkProgramRunMember) Edges() []ent.Edge {
	return []ent.Edge{
		edge.To("work_program_run", WorkProgramRun.Type).
			Unique().
			Required().
			Field("work_program_run_id").
			Annotations(entsql.OnDelete(entsql.Cascade)).
			Comment("Run boundary that owns this member."),
	}
}

// Indexes supports run membership reads and idempotent refresh.
func (WorkProgramRunMember) Indexes() []ent.Index {
	return []ent.Index{
		index.Fields("run_key", "member_table", "member_id").Unique(),
		index.Fields("run_key", "member_table"),
		index.Fields("member_table", "member_key"),
		index.Fields("work_program_run_id", "member_table"),
	}
}
