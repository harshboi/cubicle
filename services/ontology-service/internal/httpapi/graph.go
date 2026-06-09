package httpapi

import (
	"context"
	"errors"
	"net/http"

	"github.com/danielgtaylor/huma/v2"

	"cubicle/services/ontology-service/internal/graphstore"
)

func registerGraph(api huma.API, store graphstore.Expander) {
	huma.Register(api, huma.Operation{
		OperationID: "expand-graph",
		Method:      http.MethodPost,
		Path:        "/v1/graph/expand",
		Summary:     "Expand a bounded ontology graph neighborhood",
		Tags:        []string{"graph"},
	}, func(ctx context.Context, input *ExpandInput) (*ExpandOutput, error) {
		graph, err := store.Expand(ctx, input.Body)
		if err != nil {
			if errors.Is(err, graphstore.ErrInvalidExpansion) || errors.Is(err, graphstore.ErrMissingObject) {
				return nil, huma.Error400BadRequest(err.Error())
			}
			return nil, huma.Error500InternalServerError("graph expansion failed")
		}
		return &ExpandOutput{Body: graph}, nil
	})
}
