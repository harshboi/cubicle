package graphstore

import (
	"context"
	"errors"
	"fmt"
	"sort"

	"cubicle/services/ontology-service/internal/domain"
)

var (
	// ErrMissingObject means a requested object key was absent or a write tried
	// to create an association before both endpoint objects existed.
	ErrMissingObject = errors.New("missing object")

	// ErrInvalidExpansion means a graph expansion or association write was
	// structurally invalid, such as missing a start object or fan-out limit.
	ErrInvalidExpansion = errors.New("invalid expansion request")

	// ErrInvalidIngest is reserved for the ingestion PRs stacked above this
	// scaffold. It lets early HTTP error handling share stable error categories.
	ErrInvalidIngest = errors.New("invalid ingest request")

	// ErrIngestConflict is reserved for idempotency/hash conflicts in the later
	// ingestion writer implementation.
	ErrIngestConflict = errors.New("ingest conflict")

	// ErrRunNotOpen is reserved for writes attempted after an ingest run is
	// completed or failed.
	ErrRunNotOpen = errors.New("ingest run is not open")

	// ErrSnapshotNotFound is reserved for mapped batches that reference a raw
	// snapshot the service has not recorded.
	ErrSnapshotNotFound = errors.New("source snapshot not found")
)

// MemoryStore is the first graphstore implementation used for tests, examples,
// and the initial localhost server.
type MemoryStore struct {
	// objects stores every object by stable object ref. This mirrors the domain
	// contract that object identity is (object_type, key), not key alone.
	objects map[string]domain.Object

	// associations stores every association by its derived or caller-supplied
	// key so repeated writes replace the logical relationship.
	associations map[string]domain.Association

	// out is the adjacency list from object ref to association keys. Expand uses
	// it for bounded breadth-first traversal.
	out map[string][]string
}

// NewMemoryStore creates an empty graphstore for tests and the first localhost
// server slice.
func NewMemoryStore() *MemoryStore {
	return &MemoryStore{
		objects:      make(map[string]domain.Object),
		associations: make(map[string]domain.Association),
		out:          make(map[string][]string),
	}
}

// UpsertObject inserts or replaces an object by its stable object ref.
func (s *MemoryStore) UpsertObject(_ context.Context, object domain.Object) error {
	if object.ObjectType == "" || object.Key == "" {
		return fmt.Errorf("%w: object_type and key are required", ErrMissingObject)
	}
	s.objects[objectRefKey(object.Ref())] = object
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
	fromRefKey := objectRefKey(association.From)
	if _, ok := s.objects[fromRefKey]; !ok {
		return fmt.Errorf("%w: %s", ErrMissingObject, association.From.Key)
	}
	toRefKey := objectRefKey(association.To)
	if _, ok := s.objects[toRefKey]; !ok {
		return fmt.Errorf("%w: %s", ErrMissingObject, association.To.Key)
	}
	if association.Key == "" {
		association.Key = associationKey(association)
	}
	if _, exists := s.associations[association.Key]; !exists {
		s.out[fromRefKey] = append(s.out[fromRefKey], association.Key)
	}
	s.associations[association.Key] = association
	return nil
}

// Expand performs deterministic breadth-first traversal from a start object.
//
// Deterministic ordering is important for tests, generated examples, and UI
// snapshots. The implementation sorts association keys at each hop so the same
// graph always yields the same response order.
func (s *MemoryStore) Expand(_ context.Context, req domain.ExpandRequest) (domain.Neighborhood, error) {
	if req.Start.ObjectType == "" || req.Start.Key == "" || req.Depth < 0 || req.LimitPerObject <= 0 {
		return domain.Neighborhood{}, fmt.Errorf("%w: start, non-negative depth, and positive limit are required", ErrInvalidExpansion)
	}
	startRefKey := objectRefKey(req.Start)
	startObject, ok := s.objects[startRefKey]
	if !ok || !objectAllowed(req.ReadFilter, startObject) {
		return domain.Neighborhood{}, fmt.Errorf("%w: %s", ErrMissingObject, req.Start.Key)
	}

	// allowedTypes is nil when traversal should allow every association type;
	// otherwise it is a set used to filter each outgoing association.
	allowedTypes := associationTypeSet(req.AssociationTypes)

	// seenObjects prevents duplicate objects in the response and avoids cycling
	// forever when the graph later contains reverse or cyclic associations.
	seenObjects := map[string]bool{startRefKey: true}

	// seenAssociations prevents repeated associations in the response when two
	// traversal paths reach the same relationship.
	seenAssociations := make(map[string]bool)

	// objectOrder preserves deterministic response order while seenObjects gives
	// constant-time membership checks.
	objectOrder := []string{startRefKey}

	// associationOrder preserves deterministic response order for associations.
	associationOrder := make([]string, 0)

	// frontier contains the objects to expand at the current BFS depth.
	frontier := []domain.ObjectRef{req.Start}

	// depth counts how many association hops have already been expanded.
	for depth := 0; depth < req.Depth && len(frontier) > 0; depth++ {
		// next collects the next BFS frontier while the current frontier is being
		// expanded.
		next := make([]domain.ObjectRef, 0)
		for _, ref := range frontier {
			// associationKeys is copied before sorting so deterministic traversal
			// does not mutate the store's insertion-order adjacency list.
			associationKeys := append([]string(nil), s.out[objectRefKey(ref)]...)
			sort.Strings(associationKeys)

			// used tracks how many associations have been accepted from this one
			// object so LimitPerObject applies per object, not globally.
			used := 0
			for _, key := range associationKeys {
				if used >= req.LimitPerObject {
					break
				}
				// association is the relationship being considered for traversal.
				association := s.associations[key]
				if len(allowedTypes) > 0 && !allowedTypes[association.AssociationType] {
					continue
				}
				toObject, ok := s.objects[objectRefKey(association.To)]
				if !ok || !objectAllowed(req.ReadFilter, toObject) || !associationAllowed(req.ReadFilter, association) {
					continue
				}
				used++
				if !seenAssociations[key] {
					seenAssociations[key] = true
					associationOrder = append(associationOrder, key)
				}
				toRefKey := objectRefKey(association.To)
				if !seenObjects[toRefKey] {
					seenObjects[toRefKey] = true
					objectOrder = append(objectOrder, toRefKey)
					next = append(next, association.To)
				}
			}
		}
		frontier = next
	}

	// graph is the response object assembled after traversal so the public DTO
	// order follows objectOrder and associationOrder.
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

func objectAllowed(filter domain.ExpandReadFilter, object domain.Object) bool {
	if filter.ObjectAllowed == nil {
		return true
	}
	return filter.ObjectAllowed(object)
}

func associationAllowed(filter domain.ExpandReadFilter, association domain.Association) bool {
	if filter.AssociationAllowed == nil {
		return true
	}
	return filter.AssociationAllowed(association)
}

func objectRefKey(ref domain.ObjectRef) string {
	return string(ref.ObjectType) + "\x00" + ref.Key
}

func associationTypeSet(types []domain.AssociationType) map[domain.AssociationType]bool {
	if len(types) == 0 {
		return nil
	}

	// set gives O(1) membership tests while traversing outgoing associations.
	set := make(map[domain.AssociationType]bool, len(types))

	// typ is each association type the caller allowed for traversal.
	for _, typ := range types {
		set[typ] = true
	}
	return set
}

func associationKey(association domain.Association) string {
	// The key is intentionally derived from endpoint object refs and association
	// type. Later ingestion PRs can extend this when multiple evidence rows need
	// separate association records.
	return string(association.From.ObjectType) + ":" + association.From.Key +
		"|" + string(association.AssociationType) +
		"|" + string(association.To.ObjectType) + ":" + association.To.Key
}
