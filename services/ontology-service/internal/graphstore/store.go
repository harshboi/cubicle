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

// Writer is the ingestion-side graph boundary.
//
// Keeping writes separate from reads lets fixtures and future crawlers seed the
// graph without depending on HTTP-only behavior. Both MemoryStore and EntStore
// satisfy this interface.
type Writer interface {
	UpsertNode(context.Context, domain.Node) error
	UpsertEdge(context.Context, domain.Edge) error
}

// Store is the full local graph boundary used by HTTP composition.
//
// Query-only code should still prefer Expander, and ingestion-only code should
// prefer Writer. The server needs both so crawlers can insert graph facts and
// clients can query them through the same process.
type Store interface {
	Expander
	Writer
}
