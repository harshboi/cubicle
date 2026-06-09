package schema

import (
	"entgo.io/ent"
	entschema "entgo.io/ent/schema"
	"entgo.io/ent/schema/field"
)

// linkFields returns relationship metadata shared by every Through edge schema.
func linkFields(relationValues []string) []ent.Field {
	return appendFields(
		[]ent.Field{
			field.Enum("relation_kind").
				Values(relationValues...).
				Comment("Semantic relationship represented by this link row."),
			evidenceRefField(),
			field.Int("evidence_count").
				Default(0).
				Comment("Number of evidence records known to support this relationship."),
		},
		activityFields(),
		sourceFields(),
		qualityFields(),
		timestampFields(),
	)
}

// edgeSchemaAnnotations returns entgql annotations plus a composite endpoint ID.
func edgeSchemaAnnotations(firstID string, secondID string) []entschema.Annotation {
	annotations := graphqlAnnotations()
	return append(annotations, field.ID(firstID, secondID))
}
