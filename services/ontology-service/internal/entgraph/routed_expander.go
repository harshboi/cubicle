package entgraph

import (
	"context"

	genent "cubicle/services/ontology-service/ent"
	"cubicle/services/ontology-service/internal/domain"
	"cubicle/services/ontology-service/internal/graphstore"
	"cubicle/services/ontology-service/internal/ontology"
)

// RoutedExpander chooses the persisted Ent expansion strategy for a start row.
//
// Product objects keep their strongly typed Ent path. Connector-specific object
// families use open graph rows until they earn a first-class product schema.
type RoutedExpander struct {
	product graphstore.Expander
	open    graphstore.Expander
}

// NewRoutedExpander returns the default Ent-backed bounded graph expander.
func NewRoutedExpander(client *genent.Client) *RoutedExpander {
	return &RoutedExpander{
		product: NewProductExpander(client),
		open:    NewOpenGraphExpander(client),
	}
}

// Expand delegates the whole bounded traversal to the row family that owns the
// seed object. Cross-family traversal should be added only with explicit merge
// and source-authority rules.
func (e *RoutedExpander) Expand(ctx context.Context, req domain.ExpandRequest) (domain.Neighborhood, error) {
	if isProductStartObjectType(req.Start.ObjectType) {
		return e.product.Expand(ctx, req)
	}
	return e.open.Expand(ctx, req)
}

func isProductStartObjectType(value domain.ObjectType) bool {
	switch value {
	case ontology.ObjectTicket,
		ontology.ObjectPullRequest,
		ontology.ObjectDocument,
		ontology.ObjectMessage,
		ontology.ObjectPerson:
		return true
	default:
		return false
	}
}
