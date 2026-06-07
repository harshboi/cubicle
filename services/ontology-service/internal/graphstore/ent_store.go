package graphstore

import (
	"context"
	"fmt"

	"cubicle/services/ontology-service/ent"
	entassociation "cubicle/services/ontology-service/ent/association"
	entobject "cubicle/services/ontology-service/ent/object"
	"cubicle/services/ontology-service/internal/domain"
)

// EntStore persists ontology objects and associations through Ent.
//
// It deliberately implements the same graph-facing behavior as MemoryStore.
// That lets higher layers depend on the object/association contract instead of
// knowing whether the graph is backed by maps, generated Ent code, or a future
// remote database.
type EntStore struct {
	client *ent.Client
}

func NewEntStore(client *ent.Client) *EntStore {
	return &EntStore{client: client}
}

func (s *EntStore) UpsertObject(ctx context.Context, object domain.Object) error {
	if object.ObjectType == "" || object.Key == "" {
		return fmt.Errorf("%w: object_type and key are required", ErrMissingObject)
	}
	existing, err := s.client.Object.Query().
		Where(entobject.KeyEQ(object.Key)).
		Only(ctx)
	if err == nil {
		return existing.Update().
			SetObjectType(string(object.ObjectType)).
			SetTitle(object.Title).
			SetSource(object.Source).
			SetExternalID(object.ExternalID).
			SetVisibility(object.Visibility).
			SetFreshnessState(object.FreshnessState).
			SetObservedAt(object.ObservedAt).
			Exec(ctx)
	}
	if !ent.IsNotFound(err) {
		return err
	}
	return s.client.Object.Create().
		SetKey(object.Key).
		SetObjectType(string(object.ObjectType)).
		SetTitle(object.Title).
		SetSource(object.Source).
		SetExternalID(object.ExternalID).
		SetVisibility(object.Visibility).
		SetFreshnessState(object.FreshnessState).
		SetObservedAt(object.ObservedAt).
		Exec(ctx)
}

func (s *EntStore) UpsertAssociation(ctx context.Context, association domain.Association) error {
	if association.From.Key == "" || association.To.Key == "" || association.AssociationType == "" {
		return fmt.Errorf("%w: from, to, and association_type are required", ErrInvalidExpansion)
	}
	if _, err := s.objectByKey(ctx, association.From.Key); err != nil {
		return err
	}
	if _, err := s.objectByKey(ctx, association.To.Key); err != nil {
		return err
	}
	if association.Key == "" {
		association.Key = associationKey(association)
	}
	existing, err := s.client.Association.Query().
		Where(entassociation.KeyEQ(association.Key)).
		Only(ctx)
	if err == nil {
		return existing.Update().
			SetFromObjectType(string(association.From.ObjectType)).
			SetFromObjectKey(association.From.Key).
			SetToObjectType(string(association.To.ObjectType)).
			SetToObjectKey(association.To.Key).
			SetAssociationType(string(association.AssociationType)).
			SetEvidenceKey(association.Metadata.EvidenceKey).
			SetSource(association.Metadata.Source).
			SetConfidence(association.Metadata.Confidence).
			SetVisibility(association.Metadata.Visibility).
			SetFreshnessState(association.Metadata.FreshnessState).
			SetObservedAt(association.Metadata.ObservedAt).
			Exec(ctx)
	}
	if !ent.IsNotFound(err) {
		return err
	}
	return s.client.Association.Create().
		SetKey(association.Key).
		SetFromObjectType(string(association.From.ObjectType)).
		SetFromObjectKey(association.From.Key).
		SetToObjectType(string(association.To.ObjectType)).
		SetToObjectKey(association.To.Key).
		SetAssociationType(string(association.AssociationType)).
		SetEvidenceKey(association.Metadata.EvidenceKey).
		SetSource(association.Metadata.Source).
		SetConfidence(association.Metadata.Confidence).
		SetVisibility(association.Metadata.Visibility).
		SetFreshnessState(association.Metadata.FreshnessState).
		SetObservedAt(association.Metadata.ObservedAt).
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
			storedAssociations, err := s.client.Association.Query().
				Where(entassociation.FromObjectKeyEQ(ref.Key)).
				Order(entassociation.ByKey()).
				All(ctx)
			if err != nil {
				return domain.Neighborhood{}, err
			}

			used := 0
			for _, storedAssociation := range storedAssociations {
				domainAssociation := associationToDomain(storedAssociation)
				if len(allowedTypes) > 0 && !allowedTypes[domainAssociation.AssociationType] {
					continue
				}
				used++
				if used > req.LimitPerObject {
					break
				}
				if !seenAssociations[domainAssociation.Key] {
					seenAssociations[domainAssociation.Key] = true
					associations = append(associations, domainAssociation)
				}
				if !seenObjects[domainAssociation.To.Key] {
					object, err := s.objectByKey(ctx, domainAssociation.To.Key)
					if err != nil {
						return domain.Neighborhood{}, err
					}
					seenObjects[domainAssociation.To.Key] = true
					objects = append(objects, objectToDomain(object))
					next = append(next, domainAssociation.To)
				}
			}
		}
		frontier = next
	}

	return domain.Neighborhood{Objects: objects, Associations: associations}, nil
}

func (s *EntStore) objectByKey(ctx context.Context, key string) (*ent.Object, error) {
	storedObject, err := s.client.Object.Query().
		Where(entobject.KeyEQ(key)).
		Only(ctx)
	if ent.IsNotFound(err) {
		return nil, fmt.Errorf("%w: %s", ErrMissingObject, key)
	}
	return storedObject, err
}

func objectToDomain(storedObject *ent.Object) domain.Object {
	return domain.Object{
		ObjectType:     domain.ObjectType(storedObject.ObjectType),
		Key:            storedObject.Key,
		Title:          storedObject.Title,
		Source:         storedObject.Source,
		ExternalID:     storedObject.ExternalID,
		Visibility:     storedObject.Visibility,
		FreshnessState: storedObject.FreshnessState,
		ObservedAt:     storedObject.ObservedAt,
	}
}

func associationToDomain(storedAssociation *ent.Association) domain.Association {
	return domain.Association{
		Key:             storedAssociation.Key,
		From:            domain.ObjectRef{ObjectType: domain.ObjectType(storedAssociation.FromObjectType), Key: storedAssociation.FromObjectKey},
		To:              domain.ObjectRef{ObjectType: domain.ObjectType(storedAssociation.ToObjectType), Key: storedAssociation.ToObjectKey},
		AssociationType: domain.AssociationType(storedAssociation.AssociationType),
		Metadata: domain.AssociationMetadata{
			EvidenceKey:    storedAssociation.EvidenceKey,
			Source:         storedAssociation.Source,
			Confidence:     storedAssociation.Confidence,
			Visibility:     storedAssociation.Visibility,
			FreshnessState: storedAssociation.FreshnessState,
			ObservedAt:     storedAssociation.ObservedAt,
		},
	}
}
