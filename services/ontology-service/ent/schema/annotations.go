package schema

import (
	"entgo.io/contrib/entgql"
	entschema "entgo.io/ent/schema"
)

// graphqlAnnotations marks a schema as intended for the public entgql API.
// The actual GraphQL exposure is generated in a later slice after this storage
// model is reviewed.
func graphqlAnnotations() []entschema.Annotation {
	return []entschema.Annotation{
		entgql.QueryField(),
		entgql.RelayConnection(),
		entgql.Mutations(),
	}
}
