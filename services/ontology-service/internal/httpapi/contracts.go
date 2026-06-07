package httpapi

import "cubicle/services/ontology-service/internal/domain"

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

type ErrorResponse struct {
	Code    string `json:"code"`
	Message string `json:"message"`
}
