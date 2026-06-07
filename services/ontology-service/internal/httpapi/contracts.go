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
