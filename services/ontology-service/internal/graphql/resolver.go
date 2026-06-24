package graphql

import (
	"context"

	genent "cubicle/services/ontology-service/ent"
	"cubicle/services/ontology-service/internal/domain"
	"cubicle/services/ontology-service/internal/graphcontext"
	"cubicle/services/ontology-service/internal/graphstore"
)

// Resolver is the gqlgen dependency root for Cubicle's GraphQL API.
//
// HTTP wiring owns transport concerns. Resolver owns the product read boundary
// and starts from typed Ent rows such as WorkAction instead of raw replay data.
type Resolver struct {
	EntClient                      *genent.Client
	GraphExpander                  graphstore.Expander
	BoundedGraphReadFilterProvider BoundedGraphReadFilterProvider
	BoundedGraphSourceAuthority    graphcontext.SourceAuthorityPolicy
}

// BoundedGraphReadFilterProvider adapts request/principal context into the
// graphstore read filter used before bounded graph traversal.
type BoundedGraphReadFilterProvider func(ctx context.Context) domain.ExpandReadFilter
