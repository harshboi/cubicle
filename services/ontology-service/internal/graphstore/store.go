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
	Expand(context.Context, domain.ExpandRequest) (domain.Neighborhood, error)
}

// Writer is the ingestion-side object/association boundary.
//
// Keeping writes separate from reads lets fixtures and future crawlers seed the
// graph without depending on HTTP-only behavior. Both MemoryStore and EntStore
// satisfy this interface.
type Writer interface {
	UpsertObject(context.Context, domain.Object) error
	UpsertAssociation(context.Context, domain.Association) error
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

// IngestWriter is the durable boundary crawlers use after mapping source data
// into generic ontology facts.
//
// The important rule is that callers provide source-neutral DTOs from
// internal/domain. Jira, GitHub, docs, or Flink-specific parsing stays in the
// crawler/mapper layer; graphstore only validates and persists the resulting
// facts.
type IngestWriter interface {
	BeginIngestRun(context.Context, domain.IngestRunStart) (domain.IngestRun, error)
	WriteSnapshot(context.Context, domain.SourceSnapshotWrite) (domain.SourceSnapshot, error)
	WriteMappedBatch(context.Context, domain.IngestBatch) (domain.IngestBatchResult, error)
	CompleteIngestRun(context.Context, domain.IngestRunComplete) (domain.IngestRun, error)
	GetIngestRun(context.Context, string) (domain.IngestRun, error)
	ListSourceStatus(context.Context) ([]domain.SourceStatus, error)
}
