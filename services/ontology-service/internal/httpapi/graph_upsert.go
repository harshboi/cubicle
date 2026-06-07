package httpapi

import (
	"context"
	"errors"
	"net/http"

	"github.com/danielgtaylor/huma/v2"

	"cubicle/services/ontology-service/internal/domain"
	"cubicle/services/ontology-service/internal/graphstore"
)

func registerGraphUpsert(api huma.API, store graphstore.Writer) {
	huma.Register(api, huma.Operation{
		OperationID: "upsert-graph",
		Method:      http.MethodPost,
		Path:        "/v1/graph/upsert",
		Summary:     "Insert or update ontology graph facts",
		Tags:        []string{"graph"},
	}, func(ctx context.Context, input *GraphUpsertInput) (*GraphUpsertOutput, error) {
		for _, node := range input.Body.Nodes {
			if err := store.UpsertNode(ctx, node); err != nil {
				return nil, graphWriteError(err)
			}
		}
		for _, edge := range input.Body.Edges {
			if err := store.UpsertEdge(ctx, graphUpsertEdgeToDomain(edge)); err != nil {
				return nil, graphWriteError(err)
			}
		}
		return &GraphUpsertOutput{Body: GraphUpsertResponse{
			NodeCount: len(input.Body.Nodes),
			EdgeCount: len(input.Body.Edges),
		}}, nil
	})
}

func graphUpsertEdgeToDomain(edge GraphUpsertEdge) domain.Edge {
	return domain.Edge{
		Key:  edge.Key,
		From: edge.From,
		To:   edge.To,
		Metadata: domain.EdgeMetadata{
			Predicate:      edge.Metadata.Predicate,
			EvidenceKey:    edge.Metadata.EvidenceKey,
			Source:         edge.Metadata.Source,
			Confidence:     edge.Metadata.Confidence,
			Visibility:     edge.Metadata.Visibility,
			FreshnessState: edge.Metadata.FreshnessState,
			ObservedAt:     edge.Metadata.ObservedAt,
		},
	}
}

func graphWriteError(err error) error {
	if errors.Is(err, graphstore.ErrInvalidExpansion) || errors.Is(err, graphstore.ErrMissingNode) {
		return huma.Error400BadRequest(err.Error())
	}
	return huma.Error500InternalServerError("graph upsert failed")
}
