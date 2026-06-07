package httpapi

import (
	"context"
	"net/http"

	"github.com/danielgtaylor/huma/v2"

	"cubicle/services/ontology-service/internal/domain"
	"cubicle/services/ontology-service/internal/graphstore"
)

type SourceStatusOutput struct {
	Body []domain.SourceStatus
}

func registerSources(api huma.API, store graphstore.IngestWriter) {
	huma.Register(api, huma.Operation{
		OperationID: "list-sources",
		Method:      http.MethodGet,
		Path:        "/v1/sources",
		Summary:     "List source ingestion status",
		Tags:        []string{"sources"},
	}, func(ctx context.Context, input *struct{}) (*SourceStatusOutput, error) {
		statuses, err := store.ListSourceStatus(ctx)
		if err != nil {
			return nil, ingestHTTPError(err)
		}
		return &SourceStatusOutput{Body: statuses}, nil
	})
}
