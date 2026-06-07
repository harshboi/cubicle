package httpapi

import (
	"context"
	"errors"
	"net/http"

	"github.com/danielgtaylor/huma/v2"

	"cubicle/services/ontology-service/internal/graphstore"
	"cubicle/services/ontology-service/internal/query"
)

func registerWorkstreams(api huma.API, store graphstore.Expander) {
	service := query.NewWorkstreamService(store)

	huma.Register(api, huma.Operation{
		OperationID: "get-workstream-overview",
		Method:      http.MethodGet,
		Path:        "/v1/workstreams/{slug}/overview",
		Summary:     "Get a classified workstream overview",
		Tags:        []string{"workstreams"},
	}, func(ctx context.Context, input *WorkstreamOverviewInput) (*WorkstreamOverviewOutput, error) {
		overview, err := service.Overview(ctx, input.Slug)
		if err != nil {
			if errors.Is(err, query.ErrInvalidWorkstream) ||
				errors.Is(err, graphstore.ErrInvalidExpansion) ||
				errors.Is(err, graphstore.ErrMissingObject) {
				return nil, huma.Error400BadRequest(err.Error())
			}
			return nil, huma.Error500InternalServerError("workstream overview failed")
		}
		return &WorkstreamOverviewOutput{Body: overview}, nil
	})
}
