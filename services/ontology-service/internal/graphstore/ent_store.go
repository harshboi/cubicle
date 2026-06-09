package graphstore

import (
	"context"
	"fmt"

	"cubicle/services/ontology-service/ent"
	"cubicle/services/ontology-service/ent/graphedge"
	"cubicle/services/ontology-service/ent/ontologynode"
	"cubicle/services/ontology-service/internal/domain"
)

// EntStore persists ontology objects and associations through Ent.
//
// It deliberately implements the same graph-facing behavior as MemoryStore.
// That lets higher layers depend on the AssociationStore-style contract instead
// of knowing whether the graph is backed by maps, generated Ent code, or a
// future remote database.
type EntStore struct {
	client *ent.Client
}

func NewEntStore(client *ent.Client) *EntStore {
	return &EntStore{client: client}
}

func (s *EntStore) UpsertObject(ctx context.Context, node domain.Object) error {
	return upsertObject(ctx, s.client, node)
}

func upsertObject(ctx context.Context, client *ent.Client, object domain.Object) error {
	if object.ObjectType == "" || object.Key == "" {
		return fmt.Errorf("%w: object_type and key are required", ErrMissingObject)
	}
	existing, err := client.OntologyNode.Query().
		Where(ontologynode.KeyEQ(object.Key)).
		Only(ctx)
	if err == nil {
		return existing.Update().
			SetKind(string(object.ObjectType)).
			SetTitle(object.Title).
			SetSource(object.Source).
			SetSourceInstance(object.SourceInstance).
			SetExternalID(object.ExternalID).
			SetSourceURL(object.SourceURL).
			SetSnapshotKey(object.SnapshotKey).
			SetMapperVersion(object.MapperVersion).
			SetVisibility(object.Visibility).
			SetFreshnessState(object.FreshnessState).
			SetObservedAt(object.ObservedAt).
			SetSourceUpdatedAt(object.SourceUpdatedAt).
			SetPropertiesJSON(object.PropertiesJSON).
			Exec(ctx)
	}
	if !ent.IsNotFound(err) {
		return err
	}
	return client.OntologyNode.Create().
		SetKey(object.Key).
		SetKind(string(object.ObjectType)).
		SetTitle(object.Title).
		SetSource(object.Source).
		SetSourceInstance(object.SourceInstance).
		SetExternalID(object.ExternalID).
		SetSourceURL(object.SourceURL).
		SetSnapshotKey(object.SnapshotKey).
		SetMapperVersion(object.MapperVersion).
		SetVisibility(object.Visibility).
		SetFreshnessState(object.FreshnessState).
		SetObservedAt(object.ObservedAt).
		SetSourceUpdatedAt(object.SourceUpdatedAt).
		SetPropertiesJSON(object.PropertiesJSON).
		Exec(ctx)
}

func (s *EntStore) UpsertAssociation(ctx context.Context, association domain.Association) error {
	return upsertAssociation(ctx, s.client, association)
}

func upsertAssociation(ctx context.Context, client *ent.Client, association domain.Association) error {
	if association.From.Key == "" || association.To.Key == "" || association.AssociationType == "" {
		return fmt.Errorf("%w: from, to, and association_type are required", ErrInvalidExpansion)
	}
	if _, err := objectByKeyWithClient(ctx, client, association.From.Key); err != nil {
		return err
	}
	if _, err := objectByKeyWithClient(ctx, client, association.To.Key); err != nil {
		return err
	}
	if association.Key == "" {
		association.Key = associationKey(association)
	}
	existing, err := client.GraphEdge.Query().
		Where(graphedge.KeyEQ(association.Key)).
		Only(ctx)
	if err == nil {
		return existing.Update().
			SetFromKind(string(association.From.ObjectType)).
			SetFromKey(association.From.Key).
			SetToKind(string(association.To.ObjectType)).
			SetToKey(association.To.Key).
			SetPredicate(string(association.AssociationType)).
			SetEvidenceKey(association.Metadata.EvidenceKey).
			SetSource(association.Metadata.Source).
			SetSourceInstance(association.Metadata.SourceInstance).
			SetSourceURL(association.Metadata.SourceURL).
			SetSnapshotKey(association.Metadata.SnapshotKey).
			SetMapperVersion(association.Metadata.MapperVersion).
			SetConfidence(association.Metadata.Confidence).
			SetVisibility(association.Metadata.Visibility).
			SetFreshnessState(association.Metadata.FreshnessState).
			SetObservedAt(association.Metadata.ObservedAt).
			SetSourceUpdatedAt(association.Metadata.SourceUpdatedAt).
			SetPropertiesJSON(association.Metadata.PropertiesJSON).
			Exec(ctx)
	}
	if !ent.IsNotFound(err) {
		return err
	}
	return client.GraphEdge.Create().
		SetKey(association.Key).
		SetFromKind(string(association.From.ObjectType)).
		SetFromKey(association.From.Key).
		SetToKind(string(association.To.ObjectType)).
		SetToKey(association.To.Key).
		SetPredicate(string(association.AssociationType)).
		SetEvidenceKey(association.Metadata.EvidenceKey).
		SetSource(association.Metadata.Source).
		SetSourceInstance(association.Metadata.SourceInstance).
		SetSourceURL(association.Metadata.SourceURL).
		SetSnapshotKey(association.Metadata.SnapshotKey).
		SetMapperVersion(association.Metadata.MapperVersion).
		SetConfidence(association.Metadata.Confidence).
		SetVisibility(association.Metadata.Visibility).
		SetFreshnessState(association.Metadata.FreshnessState).
		SetObservedAt(association.Metadata.ObservedAt).
		SetSourceUpdatedAt(association.Metadata.SourceUpdatedAt).
		SetPropertiesJSON(association.Metadata.PropertiesJSON).
		Exec(ctx)
}

func (s *EntStore) Expand(ctx context.Context, req domain.ExpandRequest) (domain.Neighborhood, error) {
	if req.Start.ObjectType == "" || req.Start.Key == "" || req.Depth < 0 || req.LimitPerObject <= 0 {
		return domain.Neighborhood{}, fmt.Errorf("%w: start, non-negative depth, and positive limit are required", ErrInvalidExpansion)
	}
	start, err := s.objectByKey(ctx, req.Start.Key)
	if err != nil {
		return domain.Neighborhood{}, err
	}

	allowedTypes := associationTypeSet(req.AssociationTypes)
	seenObjects := map[string]bool{req.Start.Key: true}
	seenAssociations := make(map[string]bool)
	objects := []domain.Object{objectToDomain(start)}
	associations := make([]domain.Association, 0)
	frontier := []domain.ObjectRef{req.Start}

	for depth := 0; depth < req.Depth && len(frontier) > 0; depth++ {
		next := make([]domain.ObjectRef, 0)
		for _, ref := range frontier {
			graphEdges, err := s.client.GraphEdge.Query().
				Where(graphedge.FromKeyEQ(ref.Key)).
				Order(graphedge.ByKey()).
				All(ctx)
			if err != nil {
				return domain.Neighborhood{}, err
			}

			used := 0
			for _, graphEdge := range graphEdges {
				association := associationToDomain(graphEdge)
				if len(allowedTypes) > 0 && !allowedTypes[association.AssociationType] {
					continue
				}
				used++
				if used > req.LimitPerObject {
					break
				}
				if !seenAssociations[association.Key] {
					seenAssociations[association.Key] = true
					associations = append(associations, association)
				}
				if !seenObjects[association.To.Key] {
					object, err := s.objectByKey(ctx, association.To.Key)
					if err != nil {
						return domain.Neighborhood{}, err
					}
					seenObjects[association.To.Key] = true
					objects = append(objects, objectToDomain(object))
					next = append(next, association.To)
				}
			}
		}
		frontier = next
	}

	return domain.Neighborhood{Objects: objects, Associations: associations}, nil
}

func (s *EntStore) objectByKey(ctx context.Context, key string) (*ent.OntologyNode, error) {
	return objectByKeyWithClient(ctx, s.client, key)
}

func objectByKeyWithClient(ctx context.Context, client *ent.Client, key string) (*ent.OntologyNode, error) {
	node, err := client.OntologyNode.Query().
		Where(ontologynode.KeyEQ(key)).
		Only(ctx)
	if ent.IsNotFound(err) {
		return nil, fmt.Errorf("%w: %s", ErrMissingObject, key)
	}
	return node, err
}

func objectToDomain(node *ent.OntologyNode) domain.Object {
	return domain.Object{
		ObjectType:      domain.ObjectType(node.Kind),
		Key:             node.Key,
		Title:           node.Title,
		Source:          node.Source,
		SourceInstance:  node.SourceInstance,
		ExternalID:      node.ExternalID,
		SourceURL:       node.SourceURL,
		SnapshotKey:     node.SnapshotKey,
		MapperVersion:   node.MapperVersion,
		Visibility:      node.Visibility,
		FreshnessState:  node.FreshnessState,
		ObservedAt:      node.ObservedAt,
		SourceUpdatedAt: node.SourceUpdatedAt,
		PropertiesJSON:  node.PropertiesJSON,
	}
}

func associationToDomain(edge *ent.GraphEdge) domain.Association {
	return domain.Association{
		Key:             edge.Key,
		From:            domain.ObjectRef{ObjectType: domain.ObjectType(edge.FromKind), Key: edge.FromKey},
		To:              domain.ObjectRef{ObjectType: domain.ObjectType(edge.ToKind), Key: edge.ToKey},
		AssociationType: domain.AssociationType(edge.Predicate),
		Metadata: domain.AssociationMetadata{
			EvidenceKey:     edge.EvidenceKey,
			Source:          edge.Source,
			SourceInstance:  edge.SourceInstance,
			SourceURL:       edge.SourceURL,
			SnapshotKey:     edge.SnapshotKey,
			MapperVersion:   edge.MapperVersion,
			Confidence:      edge.Confidence,
			Visibility:      edge.Visibility,
			FreshnessState:  edge.FreshnessState,
			ObservedAt:      edge.ObservedAt,
			SourceUpdatedAt: edge.SourceUpdatedAt,
			PropertiesJSON:  edge.PropertiesJSON,
		},
	}
}
