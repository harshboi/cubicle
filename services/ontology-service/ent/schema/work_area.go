package schema

import (
	"entgo.io/ent"
	entschema "entgo.io/ent/schema"
	"entgo.io/ent/schema/edge"
	"entgo.io/ent/schema/field"
	"entgo.io/ent/schema/index"
)

// WorkArea is a bounded Cubicle graph area owned by one person.
//
// A person should have only a few work areas, such as documents, code, tickets,
// and communications. High-cardinality target edges live below lenses, not here.
//
// Association:
//
//	Person -> WorkArea -> WorkLens
//
// WorkArea is the first cardinality boundary below Person.
type WorkArea struct {
	ent.Schema
}

// Annotations declares that WorkArea is part of the future public entgql API.
func (WorkArea) Annotations() []entschema.Annotation {
	return graphqlAnnotations()
}

// Fields defines work area identity and rollup metadata.
func (WorkArea) Fields() []ent.Field {
	return appendFields(
		stableKeyFields(),
		[]ent.Field{
			field.Int("person_id").
				Immutable().
				Comment("Person row that owns this work area."),
			field.Enum("work_area_kind").
				Values(workAreaKindValues()...).
				Immutable().
				Comment("Major work domain represented by this work area."),
			field.String("display_name").
				NotEmpty().
				Comment("Human-readable work area label, such as Documents or Code."),
			field.Text("description").
				Optional().
				Comment("Short explanation of what this work area contains."),
			field.Int("lens_count").
				Default(0).
				Comment("Cached number of lenses below this work area."),
			field.Int("result_count").
				Default(0).
				Comment("Cached total target count across lenses below this work area."),
		},
		activityFields(),
		qualityFields(),
		timestampFields(),
	)
}

// Edges connects a work area to its owning person and bounded lenses.
func (WorkArea) Edges() []ent.Edge {
	return []ent.Edge{
		edge.From("person", Person.Type).
			Ref("work_areas").
			Unique().
			Required().
			Immutable().
			Field("person_id").
			Comment("Person who owns this bounded work area."),
		edge.To("lenses", WorkLens.Type).
			Comment("Bounded work lenses available under this work area."),
	}
}

// Indexes prevents duplicate work area kinds per person and supports listing a
// person's work areas in stable order.
func (WorkArea) Indexes() []ent.Index {
	return []ent.Index{
		index.Fields("person_id", "work_area_kind").Unique(),
		index.Fields("person_id", "rank_score", "last_activity_at"),
	}
}
