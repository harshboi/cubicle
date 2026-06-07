package httpapi

import (
	"time"

	"cubicle/services/ontology-service/internal/domain"
	"cubicle/services/ontology-service/internal/query"
)

// HealthOutput is the Huma response wrapper for GET /healthz.
//
// Huma separates transport metadata from the JSON body. Fields on this output
// struct can become headers later, while Body remains the response DTO Swift
// will see.
type HealthOutput struct {
	Body HealthResponse // Body is the JSON payload returned to health-check callers.
}

// HealthResponse is the JSON payload returned by GET /healthz.
type HealthResponse struct {
	OK bool `json:"ok"` // OK reports whether the process can answer HTTP requests.
}

// ExpandInput is the typed request body for bounded graph expansion.
type ExpandInput struct {
	Body domain.ExpandRequest // Body is the graph expansion request supplied by the client.
}

// ExpandOutput is the typed response body for bounded graph expansion.
type ExpandOutput struct {
	Body domain.Neighborhood // Body is the bounded graph neighborhood returned to the client.
}

// GraphUpsertRequest is the crawler-facing write DTO.
//
// Objects are written before associations so a simple crawler can send one small batch
// without manually ordering endpoint creation across multiple HTTP calls.
type GraphUpsertRequest struct {
	Objects      []domain.Object          `json:"objects"`      // Objects are ontology objects to upsert before associations.
	Associations []GraphUpsertAssociation `json:"associations"` // Associations are directed ontology links to upsert after objects.
}

// GraphUpsertAssociation is intentionally looser than domain.Association.
//
// The persisted graph still gets a full domain association, but simple crawlers should
// not have to calculate Cubicle's association key or supply optional freshness fields
// for a first local import.
type GraphUpsertAssociation struct {
	Key             string                         `json:"key,omitempty"`    // Key is an optional stable association key supplied by callers.
	From            domain.ObjectRef               `json:"from"`             // From is the source object reference for the directed association.
	To              domain.ObjectRef               `json:"to"`               // To is the target object reference for the directed association.
	AssociationType domain.AssociationType         `json:"association_type"` // AssociationType names the semantic relationship between From and To.
	Metadata        GraphUpsertAssociationMetadata `json:"metadata"`         // Metadata carries evidence and freshness details for persistence.
}

// GraphUpsertAssociationMetadata carries optional crawler-provided association metadata.
type GraphUpsertAssociationMetadata struct {
	EvidenceKey    string    `json:"evidence_key"`              // EvidenceKey links the association back to source evidence.
	Source         string    `json:"source,omitempty"`          // Source identifies the connector or fixture that observed the association.
	Confidence     float64   `json:"confidence,omitempty"`      // Confidence is the caller's trust score for the association.
	Visibility     string    `json:"visibility,omitempty"`      // Visibility controls whether the association is considered public/local/etc.
	FreshnessState string    `json:"freshness_state,omitempty"` // FreshnessState describes whether the association is fresh or stale.
	ObservedAt     time.Time `json:"observed_at,omitempty"`     // ObservedAt records when the association was seen in the source system.
}

// GraphUpsertInput is the typed request body for POST /v1/graph/upsert.
type GraphUpsertInput struct {
	Body GraphUpsertRequest // Body is the write batch sent by the crawler or fixture importer.
}

// GraphUpsertResponse summarizes how many graph records were written.
type GraphUpsertResponse struct {
	ObjectCount      int `json:"object_count"`      // ObjectCount is the number of objects accepted in the batch.
	AssociationCount int `json:"association_count"` // AssociationCount is the number of associations accepted in the batch.
}

// GraphUpsertOutput is the typed response body for POST /v1/graph/upsert.
type GraphUpsertOutput struct {
	Body GraphUpsertResponse // Body is the write-count response returned to the caller.
}

// WorkstreamOverviewInput captures the slug segment in
// /v1/workstreams/{slug}/overview. The service turns that slug into the stable
// ontology key "workstream:<slug>" so Swift does not need to repeat that rule.
type WorkstreamOverviewInput struct {
	Slug string `path:"slug"` // Slug is the URL-safe workstream identifier from the route path.
}

// WorkstreamOverviewOutput is the typed response body for workstream overview queries.
type WorkstreamOverviewOutput struct {
	Body query.WorkstreamOverview // Body is the product-shaped workstream summary returned to Swift.
}
