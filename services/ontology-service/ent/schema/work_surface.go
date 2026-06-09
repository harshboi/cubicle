package schema

import (
	"entgo.io/ent"
	entschema "entgo.io/ent/schema"
	"entgo.io/ent/schema/edge"
	"entgo.io/ent/schema/field"
	"entgo.io/ent/schema/index"
)

// WorkSurface is a bounded Cubicle graph surface owned by one person.
//
// A person should have only a few surfaces, such as documents, code, tickets,
// and communications. High-cardinality target edges live below panes, not here.
type WorkSurface struct {
	ent.Schema
}

// Annotations declares that WorkSurface is part of the future public entgql API.
func (WorkSurface) Annotations() []entschema.Annotation {
	return graphqlAnnotations()
}

// Fields defines surface identity and rollup metadata.
func (WorkSurface) Fields() []ent.Field {
	return appendFields(
		stableKeyFields(),
		[]ent.Field{
			field.Int("person_id").
				Immutable().
				Comment("Person row that owns this work surface."),
			field.Enum("surface_kind").
				Values(surfaceKindValues()...).
				Immutable().
				Comment("Major work domain represented by this surface."),
			field.String("display_name").
				NotEmpty().
				Comment("Human-readable surface label, such as Documents or Code."),
			field.Text("description").
				Optional().
				Comment("Short explanation of what this surface contains."),
			field.Int("pane_count").
				Default(0).
				Comment("Cached number of panes below this surface."),
			field.Int("target_count").
				Default(0).
				Comment("Cached total target count across panes below this surface."),
		},
		activityFields(),
		qualityFields(),
		timestampFields(),
	)
}

// Edges connects a surface to its owning person and bounded panes.
func (WorkSurface) Edges() []ent.Edge {
	return []ent.Edge{
		edge.From("person", Person.Type).
			Ref("work_surfaces").
			Unique().
			Required().
			Immutable().
			Field("person_id").
			Comment("Person who owns this bounded work surface."),
		edge.To("panes", WorkPane.Type).
			Comment("Focused work panes available under this surface."),
	}
}

// Indexes prevents duplicate surface kinds per person and supports listing a
// person's surfaces in stable order.
func (WorkSurface) Indexes() []ent.Index {
	return []ent.Index{
		index.Fields("person_id", "surface_kind").Unique(),
		index.Fields("person_id", "rank_score", "last_activity_at"),
	}
}
