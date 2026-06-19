package schema

import (
	"entgo.io/ent"
	entschema "entgo.io/ent/schema"
	"entgo.io/ent/schema/field"
	"entgo.io/ent/schema/index"
)

// linkFields returns relationship metadata shared by every Through edge schema.
func linkFields(kindFieldName string, relationValues []string) []ent.Field {
	return appendFields(
		[]ent.Field{
			field.Enum(kindFieldName).
				Values(relationValues...).
				Immutable().
				Comment("Semantic relationship represented by this link row."),
			evidenceRefField(),
			field.Int("evidence_count").
				Default(0).
				Comment("Number of evidence records known to support this relationship."),
		},
		activityFields(),
		sourceBackedFields(),
		qualityFields(),
		timestampFields(),
	)
}

// edgeSchemaAnnotations returns the shared GraphQL ID tag for edge schemas.
// Semantic identity is enforced with explicit unique indexes over endpoints and kind.
func edgeSchemaAnnotations(idFields ...string) []entschema.Annotation {
	return []entschema.Annotation{
		field.Annotation{StructTag: map[string]string{"id": `json:"id,omitempty"`}},
	}
}

// semanticEdgeIndex prevents distinct semantic relationships between the same
// endpoints from collapsing into one row while still using Ent's default edge ID.
func semanticEdgeIndex(firstID string, secondID string, kindField string) ent.Index {
	return index.Fields(firstID, secondID, kindField).Unique()
}
