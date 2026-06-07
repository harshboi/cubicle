package graphstore

import (
	"context"

	"cubicle/services/ontology-service/internal/domain"
)

// Expander is the first small graphstore boundary consumed by higher layers.
//
// In Go, small consumer-facing interfaces are preferred over broad DAO-style
// interfaces. The HTTP layer only needs bounded expansion for the first server
// slice, so it should depend on this one behavior instead of the full concrete
// MemoryStore. The Ent implementation can satisfy this interface later.
type Expander interface {
	Expand(context.Context, domain.ExpandRequest) (domain.Neighborhood, error)
}
