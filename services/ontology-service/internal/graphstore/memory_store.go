package graphstore

import (
	"context"
	"errors"
	"fmt"
	"sort"

	"cubicle/services/ontology-service/internal/domain"
)

var (
	ErrMissingNode      = errors.New("missing node")
	ErrInvalidExpansion = errors.New("invalid expansion request")
	ErrInvalidIngest    = errors.New("invalid ingest request")
	ErrIngestConflict   = errors.New("ingest conflict")
	ErrRunNotOpen       = errors.New("ingest run is not open")
	ErrSnapshotNotFound = errors.New("source snapshot not found")
)

type MemoryStore struct {
	nodes map[string]domain.Node
	edges map[string]domain.Edge
	out   map[string][]string
}

// NewMemoryStore creates an empty graphstore for tests, fixtures, and the first
// localhost server slice.
func NewMemoryStore() *MemoryStore {
	return &MemoryStore{
		nodes: make(map[string]domain.Node),
		edges: make(map[string]domain.Edge),
		out:   make(map[string][]string),
	}
}

// UpsertNode inserts or replaces a node by its stable domain key.
func (s *MemoryStore) UpsertNode(_ context.Context, node domain.Node) error {
	if node.Kind == "" || node.Key == "" {
		return fmt.Errorf("%w: kind and key are required", ErrMissingNode)
	}
	s.nodes[node.Key] = node
	return nil
}

// UpsertEdge inserts or replaces an evidence-backed relationship.
//
// The store requires both endpoint nodes to exist first. That mirrors the
// invariant the Ent store should enforce later: an edge is not authoritative if
// it points at objects the graph cannot return.
func (s *MemoryStore) UpsertEdge(_ context.Context, edge domain.Edge) error {
	if edge.From.Key == "" || edge.To.Key == "" || edge.Metadata.Predicate == "" {
		return fmt.Errorf("%w: from, to, and predicate are required", ErrInvalidExpansion)
	}
	if _, ok := s.nodes[edge.From.Key]; !ok {
		return fmt.Errorf("%w: %s", ErrMissingNode, edge.From.Key)
	}
	if _, ok := s.nodes[edge.To.Key]; !ok {
		return fmt.Errorf("%w: %s", ErrMissingNode, edge.To.Key)
	}
	if edge.Key == "" {
		edge.Key = edgeKey(edge)
	}
	if _, exists := s.edges[edge.Key]; !exists {
		s.out[edge.From.Key] = append(s.out[edge.From.Key], edge.Key)
	}
	s.edges[edge.Key] = edge
	return nil
}

// Expand performs deterministic breadth-first traversal from a start node.
//
// Deterministic ordering is important for tests, OpenAPI examples, and Swift UI
// snapshots. The implementation sorts edge keys at each hop so the same graph
// always yields the same response order.
func (s *MemoryStore) Expand(_ context.Context, req domain.ExpandRequest) (domain.Neighborhood, error) {
	if req.Start.Kind == "" || req.Start.Key == "" || req.Depth < 0 || req.LimitPerNode <= 0 {
		return domain.Neighborhood{}, fmt.Errorf("%w: start, non-negative depth, and positive limit are required", ErrInvalidExpansion)
	}
	if _, ok := s.nodes[req.Start.Key]; !ok {
		return domain.Neighborhood{}, fmt.Errorf("%w: %s", ErrMissingNode, req.Start.Key)
	}

	allowedPredicates := predicateSet(req.Predicates)
	seenNodes := map[string]bool{req.Start.Key: true}
	seenEdges := make(map[string]bool)
	nodeOrder := []string{req.Start.Key}
	edgeOrder := make([]string, 0)
	frontier := []domain.NodeRef{req.Start}

	for depth := 0; depth < req.Depth && len(frontier) > 0; depth++ {
		next := make([]domain.NodeRef, 0)
		for _, ref := range frontier {
			edgeKeys := append([]string(nil), s.out[ref.Key]...)
			sort.Strings(edgeKeys)

			used := 0
			for _, key := range edgeKeys {
				if used >= req.LimitPerNode {
					break
				}
				edge := s.edges[key]
				if len(allowedPredicates) > 0 && !allowedPredicates[edge.Metadata.Predicate] {
					continue
				}
				used++
				if !seenEdges[key] {
					seenEdges[key] = true
					edgeOrder = append(edgeOrder, key)
				}
				if !seenNodes[edge.To.Key] {
					seenNodes[edge.To.Key] = true
					nodeOrder = append(nodeOrder, edge.To.Key)
					next = append(next, edge.To)
				}
			}
		}
		frontier = next
	}

	graph := domain.Neighborhood{
		Nodes: make([]domain.Node, 0, len(nodeOrder)),
		Edges: make([]domain.Edge, 0, len(edgeOrder)),
	}
	for _, key := range nodeOrder {
		graph.Nodes = append(graph.Nodes, s.nodes[key])
	}
	for _, key := range edgeOrder {
		graph.Edges = append(graph.Edges, s.edges[key])
	}
	return graph, nil
}

func predicateSet(predicates []domain.Predicate) map[domain.Predicate]bool {
	if len(predicates) == 0 {
		return nil
	}
	set := make(map[domain.Predicate]bool, len(predicates))
	for _, predicate := range predicates {
		set[predicate] = true
	}
	return set
}

func edgeKey(edge domain.Edge) string {
	return string(edge.From.Kind) + ":" + edge.From.Key +
		"|" + string(edge.Metadata.Predicate) +
		"|" + string(edge.To.Kind) + ":" + edge.To.Key +
		"|" + edge.Metadata.EvidenceKey
}
