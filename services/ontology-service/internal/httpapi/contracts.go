package httpapi

import (
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
