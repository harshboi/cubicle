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
		for _, object := range input.Body.Objects {
			if err := store.UpsertObject(ctx, object); err != nil {
				return nil, graphWriteError(err)
			}
		}
		for _, association := range input.Body.Associations {
			if err := store.UpsertAssociation(ctx, graphUpsertAssociationToDomain(association)); err != nil {
				return nil, graphWriteError(err)
			}
		}
		return &GraphUpsertOutput{Body: GraphUpsertResponse{
			ObjectCount:      len(input.Body.Objects),
			AssociationCount: len(input.Body.Associations),
		}}, nil
	})
}

func graphUpsertAssociationToDomain(association GraphUpsertAssociation) domain.Association {
	return domain.Association{
		Key:             association.Key,
		From:            association.From,
		To:              association.To,
		AssociationType: association.AssociationType,
		Metadata: domain.AssociationMetadata{
			EvidenceKey:    association.Metadata.EvidenceKey,
			Source:         association.Metadata.Source,
			Confidence:     association.Metadata.Confidence,
			Visibility:     association.Metadata.Visibility,
			FreshnessState: association.Metadata.FreshnessState,
			ObservedAt:     association.Metadata.ObservedAt,
		},
	}
}

func graphWriteError(err error) error {
	if errors.Is(err, graphstore.ErrInvalidExpansion) || errors.Is(err, graphstore.ErrMissingObject) {
		return huma.Error400BadRequest(err.Error())
	}
	return huma.Error500InternalServerError("graph upsert failed")
}
