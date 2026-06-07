package httpapi

import (
	"context"
	"net/http"

	"github.com/danielgtaylor/huma/v2"
)

func registerHealth(api huma.API) {
	huma.Register(api, huma.Operation{
		OperationID: "get-health",
		Method:      http.MethodGet,
		Path:        "/healthz",
		Summary:     "Check ontology service health",
		Tags:        []string{"system"},
	}, func(context.Context, *struct{}) (*HealthOutput, error) {
		return &HealthOutput{Body: HealthResponse{OK: true}}, nil
	})
}
