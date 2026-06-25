package entgraph

import (
	"context"
	"fmt"
	"sort"
	"strings"

	genent "cubicle/services/ontology-service/ent"
	"cubicle/services/ontology-service/ent/opengraphassociation"
	"cubicle/services/ontology-service/ent/opengraphobject"
	"cubicle/services/ontology-service/internal/domain"
	"cubicle/services/ontology-service/internal/graphstore"

	entsql "entgo.io/ent/dialect/sql"
)

// OpenGraphExpander reads bounded neighborhoods from generic Ent graph rows.
//
// It is deliberately separate from ProductExpander. ProductExpander remains the
// strongly typed company-work path; OpenGraphExpander is the falsifier for
// whether boundedGraphContext can run on connector-shaped non-work graphs
// without hard-coding every object family.
type OpenGraphExpander struct {
	client *genent.Client
}

// NewOpenGraphExpander returns an Ent-backed expander for open connector graph rows.
func NewOpenGraphExpander(client *genent.Client) *OpenGraphExpander {
	return &OpenGraphExpander{client: client}
}

// Expand returns a deterministic bounded neighborhood around an open graph object.
func (e *OpenGraphExpander) Expand(ctx context.Context, req domain.ExpandRequest) (domain.Neighborhood, error) {
	if e == nil || e.client == nil {
		return domain.Neighborhood{}, fmt.Errorf("ent open graph expander: client is required")
	}
	if req.Start.ObjectType == "" || req.Start.Key == "" || req.Depth < 0 || req.LimitPerObject <= 0 {
		return domain.Neighborhood{}, fmt.Errorf("%w: start, non-negative depth, and positive limit are required", graphstore.ErrInvalidExpansion)
	}
	start, err := e.object(ctx, req.Start)
	if err != nil {
		return domain.Neighborhood{}, err
	}
	if !objectAllowed(req.ReadFilter, start) {
		return domain.Neighborhood{}, fmt.Errorf("%w: %s", graphstore.ErrMissingObject, req.Start.Key)
	}

	allowed := associationTypeSet(req.AssociationTypes)
	candidateLimit := expansionCandidateLimit(req.LimitPerObject)
	startRefKey := objectRefKey(start.Ref())
	seenObjects := map[string]domain.Object{startRefKey: start}
	objectOrder := []string{startRefKey}
	seenAssociations := make(map[string]domain.Association)
	associationOrder := make([]string, 0)
	frontier := []domain.ObjectRef{start.Ref()}

	for depth := 0; depth < req.Depth && len(frontier) > 0; depth++ {
		next := make([]domain.ObjectRef, 0)
		for _, ref := range frontier {
			associations, err := e.associations(ctx, ref, candidateLimit, allowed)
			if err != nil {
				return domain.Neighborhood{}, err
			}
			used := 0
			for _, association := range associations {
				if used >= req.LimitPerObject {
					break
				}
				if !associationAllowed(req.ReadFilter, association) {
					continue
				}
				endpoints, ok, err := e.readableAssociationEndpointObjects(ctx, req.ReadFilter, association)
				if err != nil {
					return domain.Neighborhood{}, err
				}
				if !ok {
					continue
				}
				used++
				key := associationKey(association)
				if _, ok := seenAssociations[key]; !ok {
					seenAssociations[key] = association
					associationOrder = append(associationOrder, key)
				}
				for _, object := range endpoints {
					if objectRefKey(object.Ref()) == objectRefKey(ref) {
						continue
					}
					endpointRefKey := objectRefKey(object.Ref())
					if _, ok := seenObjects[endpointRefKey]; ok {
						continue
					}
					seenObjects[endpointRefKey] = object
					objectOrder = append(objectOrder, endpointRefKey)
					next = append(next, object.Ref())
				}
			}
		}
		frontier = next
	}

	out := domain.Neighborhood{
		Objects:      make([]domain.Object, 0, len(objectOrder)),
		Associations: make([]domain.Association, 0, len(associationOrder)),
	}
	for _, key := range objectOrder {
		out.Objects = append(out.Objects, seenObjects[key])
	}
	for _, key := range associationOrder {
		out.Associations = append(out.Associations, seenAssociations[key])
	}
	return out, nil
}

func (e *OpenGraphExpander) readableAssociationEndpointObjects(ctx context.Context, filter domain.ExpandReadFilter, association domain.Association) ([]domain.Object, bool, error) {
	endpoints := []domain.ObjectRef{association.From, association.To}
	out := make([]domain.Object, 0, len(endpoints))
	for _, endpoint := range endpoints {
		if endpoint.Key == "" {
			return nil, false, nil
		}
		object, err := e.object(ctx, endpoint)
		if err != nil {
			return nil, false, err
		}
		if !objectAllowed(filter, object) {
			return nil, false, nil
		}
		out = append(out, object)
	}
	return out, true, nil
}

func (e *OpenGraphExpander) object(ctx context.Context, ref domain.ObjectRef) (domain.Object, error) {
	row, err := e.client.OpenGraphObject.Query().
		Where(
			opengraphobject.ObjectTypeEQ(string(ref.ObjectType)),
			opengraphobject.KeyEQ(ref.Key),
		).
		Only(ctx)
	if genent.IsNotFound(err) {
		return domain.Object{}, fmt.Errorf("%w: %s", graphstore.ErrMissingObject, ref.Key)
	}
	if err != nil {
		return domain.Object{}, err
	}
	return openGraphObject(row), nil
}

func (e *OpenGraphExpander) associations(ctx context.Context, ref domain.ObjectRef, limit int, allowed map[domain.AssociationType]bool) ([]domain.Association, error) {
	row, err := e.client.OpenGraphObject.Query().
		Where(
			opengraphobject.ObjectTypeEQ(string(ref.ObjectType)),
			opengraphobject.KeyEQ(ref.Key),
		).
		Only(ctx)
	if genent.IsNotFound(err) {
		return nil, fmt.Errorf("%w: %s", graphstore.ErrMissingObject, ref.Key)
	}
	if err != nil {
		return nil, err
	}

	query := e.client.OpenGraphAssociation.Query().
		Where(opengraphassociation.Or(
			opengraphassociation.FromObjectIDEQ(row.ID),
			opengraphassociation.ToObjectIDEQ(row.ID),
		)).
		WithFromObject().
		WithToObject().
		WithLatestEvidence().
		Limit(limit).
		Order(
			opengraphassociation.ByRankScore(entsql.OrderDesc()),
			opengraphassociation.ByLastActivityAt(entsql.OrderDesc()),
			opengraphassociation.ByUpdatedAt(entsql.OrderDesc()),
		)
	if len(allowed) > 0 {
		values := make([]string, 0, len(allowed))
		for associationType := range allowed {
			values = append(values, string(associationType))
		}
		sort.Strings(values)
		query = query.Where(opengraphassociation.AssociationTypeIn(values...))
	}

	rows, err := query.All(ctx)
	if err != nil {
		return nil, err
	}
	ranked := make([]rankedAssociation, 0, len(rows))
	for _, row := range rows {
		if row.Edges.FromObject == nil || row.Edges.ToObject == nil {
			continue
		}
		ranked = append(ranked, rankedAssociation{
			association:    openGraphAssociation(row),
			rankScore:      row.RankScore,
			lastActivityAt: row.LastActivityAt,
			updatedAt:      row.UpdatedAt,
		})
	}
	sort.SliceStable(ranked, func(i, j int) bool {
		return rankedAssociationLess(ranked[i], ranked[j])
	})
	if len(ranked) > limit {
		ranked = ranked[:limit]
	}
	out := make([]domain.Association, 0, len(ranked))
	for _, association := range ranked {
		out = append(out, association.association)
	}
	return out, nil
}

func openGraphObject(row *genent.OpenGraphObject) domain.Object {
	title := strings.TrimSpace(row.Title)
	if title == "" {
		title = row.Key
	}
	return domain.Object{
		ObjectType:      domain.ObjectType(row.ObjectType),
		Key:             row.Key,
		Title:           title,
		Source:          row.SourceSystem,
		SourceInstance:  row.SourceInstance,
		ExternalID:      row.ExternalID,
		SourceURL:       row.SourceURL,
		MapperVersion:   row.SourceVersion,
		Visibility:      row.Visibility.String(),
		FreshnessState:  row.FreshnessState.String(),
		ObservedAt:      row.LastConfirmedAt,
		SourceUpdatedAt: row.SourceUpdatedAt,
		PropertiesJSON:  row.PropertiesJSON,
	}
}

func openGraphAssociation(row *genent.OpenGraphAssociation) domain.Association {
	return domain.Association{
		From:            openGraphObject(row.Edges.FromObject).Ref(),
		To:              openGraphObject(row.Edges.ToObject).Ref(),
		AssociationType: domain.AssociationType(row.AssociationType),
		Metadata: associationMetadata(
			row.SourceSystem,
			row.SourceInstance,
			row.SourceURL,
			row.SourceVersion,
			row.Confidence,
			row.Visibility.String(),
			row.FreshnessState.String(),
			row.LastConfirmedAt,
			row.SourceUpdatedAt,
			row.EvidenceCount,
			row.Edges.LatestEvidence,
		),
	}
}
