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
	Body HealthResponse
}

type HealthResponse struct {
	OK bool `json:"ok"`
}

// ExpandInput is the typed request body for bounded graph expansion.
type ExpandInput struct {
	Body domain.ExpandRequest
}

// ExpandOutput is the typed response body for bounded graph expansion.
type ExpandOutput struct {
	Body domain.Neighborhood
}

// GraphUpsertRequest is the crawler-facing write DTO.
//
// Objects are written before associations so a simple crawler can send one small batch
// without manually ordering endpoint creation across multiple HTTP calls.
type GraphUpsertRequest struct {
	Objects      []domain.Object          `json:"objects"`
	Associations []GraphUpsertAssociation `json:"associations"`
}

// GraphUpsertAssociation is intentionally looser than domain.Association.
//
// The persisted graph still gets a full domain association, but simple crawlers
// should not have to calculate Cubicle's association key or supply freshness fields
// for a first local import.
type GraphUpsertAssociation struct {
	Key             string                         `json:"key,omitempty"`
	From            domain.ObjectRef               `json:"from"`
	To              domain.ObjectRef               `json:"to"`
	AssociationType domain.AssociationType         `json:"association_type"`
	Metadata        GraphUpsertAssociationMetadata `json:"metadata"`
}

type GraphUpsertAssociationMetadata struct {
	EvidenceKey    string    `json:"evidence_key"`
	Source         string    `json:"source,omitempty"`
	Confidence     float64   `json:"confidence,omitempty"`
	Visibility     string    `json:"visibility,omitempty"`
	FreshnessState string    `json:"freshness_state,omitempty"`
	ObservedAt     time.Time `json:"observed_at,omitempty"`
}

type GraphUpsertInput struct {
	Body GraphUpsertRequest
}

type GraphUpsertResponse struct {
	ObjectCount      int `json:"object_count"`
	AssociationCount int `json:"association_count"`
}

type GraphUpsertOutput struct {
	Body GraphUpsertResponse
}

// WorkstreamOverviewInput captures the slug segment in
// /v1/workstreams/{slug}/overview. The service turns that slug into the stable
// ontology key "workstream:<slug>" so Swift does not need to repeat that rule.
type WorkstreamOverviewInput struct {
	Slug string `path:"slug"`
}

type WorkstreamOverviewOutput struct {
	Body query.WorkstreamOverview
}

type ErrorResponse struct {
	Code    string `json:"code"`
	Message string `json:"message"`
}
