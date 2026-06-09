package graphstore

import (
	"context"
	"errors"
	"fmt"
	"sort"

	"cubicle/services/ontology-service/internal/domain"
)

var (
	ErrMissingObject    = errors.New("missing object")
	ErrInvalidExpansion = errors.New("invalid expansion request")
	ErrInvalidIngest    = errors.New("invalid ingest request")
	ErrIngestConflict   = errors.New("ingest conflict")
	ErrRunNotOpen       = errors.New("ingest run is not open")
	ErrSnapshotNotFound = errors.New("source snapshot not found")
)

type MemoryStore struct {
	objects      map[string]domain.Object
	associations map[string]domain.Association
	out          map[string][]string
}

// NewMemoryStore creates an empty graphstore for tests, fixtures, and the first
// localhost server slice.
func NewMemoryStore() *MemoryStore {
	return &MemoryStore{
		objects:      make(map[string]domain.Object),
		associations: make(map[string]domain.Association),
		out:          make(map[string][]string),
	}
}

// UpsertObject inserts or replaces an object by its stable domain key.
func (s *MemoryStore) UpsertObject(_ context.Context, object domain.Object) error {
	if object.ObjectType == "" || object.Key == "" {
		return fmt.Errorf("%w: object_type and key are required", ErrMissingObject)
	}
	s.objects[object.Key] = object
	return nil
}

// UpsertAssociation inserts or replaces an evidence-backed relationship.
//
// The store requires both endpoint objects to exist first. That mirrors the
// invariant the Ent store should enforce later: an association is not authoritative if
// it points at objects the graph cannot return.
func (s *MemoryStore) UpsertAssociation(_ context.Context, association domain.Association) error {
	if association.From.Key == "" || association.To.Key == "" || association.AssociationType == "" {
		return fmt.Errorf("%w: from, to, and association_type are required", ErrInvalidExpansion)
	}
	if _, ok := s.objects[association.From.Key]; !ok {
		return fmt.Errorf("%w: %s", ErrMissingObject, association.From.Key)
	}
	if _, ok := s.objects[association.To.Key]; !ok {
		return fmt.Errorf("%w: %s", ErrMissingObject, association.To.Key)
	}
	if association.Key == "" {
		association.Key = associationKey(association)
	}
	if _, exists := s.associations[association.Key]; !exists {
		s.out[association.From.Key] = append(s.out[association.From.Key], association.Key)
	}
	s.associations[association.Key] = association
	return nil
}

// Expand performs deterministic breadth-first traversal from a start object.
//
// Deterministic ordering is important for tests, OpenAPI examples, and Swift UI
// snapshots. The implementation sorts association keys at each hop so the same graph
// always yields the same response order.
func (s *MemoryStore) Expand(_ context.Context, req domain.ExpandRequest) (domain.Neighborhood, error) {
	if req.Start.ObjectType == "" || req.Start.Key == "" || req.Depth < 0 || req.LimitPerObject <= 0 {
		return domain.Neighborhood{}, fmt.Errorf("%w: start, non-negative depth, and positive limit are required", ErrInvalidExpansion)
	}
	if _, ok := s.objects[req.Start.Key]; !ok {
		return domain.Neighborhood{}, fmt.Errorf("%w: %s", ErrMissingObject, req.Start.Key)
	}

	allowedTypes := associationTypeSet(req.AssociationTypes)
	seenObjects := map[string]bool{req.Start.Key: true}
	seenAssociations := make(map[string]bool)
	objectOrder := []string{req.Start.Key}
	associationOrder := make([]string, 0)
	frontier := []domain.ObjectRef{req.Start}

	for depth := 0; depth < req.Depth && len(frontier) > 0; depth++ {
		next := make([]domain.ObjectRef, 0)
		for _, ref := range frontier {
			associationKeys := append([]string(nil), s.out[ref.Key]...)
			sort.Strings(associationKeys)

			used := 0
			for _, key := range associationKeys {
				if used >= req.LimitPerObject {
					break
				}
				association := s.associations[key]
				if len(allowedTypes) > 0 && !allowedTypes[association.AssociationType] {
					continue
				}
				used++
				if !seenAssociations[key] {
					seenAssociations[key] = true
					associationOrder = append(associationOrder, key)
				}
				if !seenObjects[association.To.Key] {
					seenObjects[association.To.Key] = true
					objectOrder = append(objectOrder, association.To.Key)
					next = append(next, association.To)
				}
			}
		}
		frontier = next
	}

	graph := domain.Neighborhood{
		Objects:      make([]domain.Object, 0, len(objectOrder)),
		Associations: make([]domain.Association, 0, len(associationOrder)),
	}
	for _, key := range objectOrder {
		graph.Objects = append(graph.Objects, s.objects[key])
	}
	for _, key := range associationOrder {
		graph.Associations = append(graph.Associations, s.associations[key])
	}
	return graph, nil
}

func associationTypeSet(types []domain.AssociationType) map[domain.AssociationType]bool {
	if len(types) == 0 {
		return nil
	}
	set := make(map[domain.AssociationType]bool, len(types))
	for _, typ := range types {
		set[typ] = true
	}
	return set
}

func associationKey(association domain.Association) string {
	return string(association.From.ObjectType) + ":" + association.From.Key +
		"|" + string(association.AssociationType) +
		"|" + string(association.To.ObjectType) + ":" + association.To.Key
}
