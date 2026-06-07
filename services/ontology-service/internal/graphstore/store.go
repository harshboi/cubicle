package graphstore

import (
	"context"

	"cubicle/services/ontology-service/internal/domain"
)

// Expander is the first small object/association boundary consumed by higher layers.
//
// In Go, small consumer-facing interfaces are preferred over broad DAO-style
// interfaces. The HTTP layer only needs bounded expansion for the first server
// slice, so it should depend on this one behavior instead of the full concrete
// MemoryStore. The Ent implementation can satisfy this interface later.
type Expander interface {
	// Expand returns a bounded object/association neighborhood for a start
	// object. Implementations must enforce the request limits.
	Expand(context.Context, domain.ExpandRequest) (domain.Neighborhood, error)
}

// Writer is the ingestion-side object/association boundary.
//
// Keeping writes separate from reads lets explicit sample-data setup and future
// crawlers seed the graph without depending on HTTP-only behavior. Both
// MemoryStore and EntStore implementations should satisfy this interface.
type Writer interface {
	// UpsertObject inserts or replaces one object by its stable domain key.
	UpsertObject(context.Context, domain.Object) error

	// UpsertAssociation inserts or replaces one directed relationship between
	// existing objects.
	UpsertAssociation(context.Context, domain.Association) error
}

// Store is the full local graph boundary used by HTTP composition.
//
// Query-only code should still prefer Expander, and ingestion-only code should
// prefer Writer. The server needs both so crawlers can insert graph facts and
// clients can query them through the same process.
type Store interface {
	// Expander gives server/query code the read side of the graph contract.
	Expander

	// Writer gives sample-data setup and crawler code the mutation side of the
	// graph contract.
	Writer
}
