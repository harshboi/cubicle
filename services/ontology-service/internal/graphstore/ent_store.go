package graphstore

import (
	"context"
	"fmt"

	"cubicle/services/ontology-service/ent"
	"cubicle/services/ontology-service/ent/graphedge"
	"cubicle/services/ontology-service/ent/ontologynode"
	"cubicle/services/ontology-service/internal/domain"
)

// EntStore persists ontology nodes and graph edges through Ent.
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

func (s *EntStore) UpsertNode(ctx context.Context, node domain.Node) error {
	if node.Kind == "" || node.Key == "" {
		return fmt.Errorf("%w: kind and key are required", ErrMissingNode)
	}
	existing, err := s.client.OntologyNode.Query().
		Where(ontologynode.KeyEQ(node.Key)).
		Only(ctx)
	if err == nil {
		return existing.Update().
			SetKind(string(node.Kind)).
			SetTitle(node.Title).
			SetSource(node.Source).
			SetExternalID(node.ExternalID).
			SetVisibility(node.Visibility).
			SetFreshnessState(node.FreshnessState).
			SetObservedAt(node.ObservedAt).
			Exec(ctx)
	}
	if !ent.IsNotFound(err) {
		return err
	}
	return s.client.OntologyNode.Create().
		SetKey(node.Key).
		SetKind(string(node.Kind)).
		SetTitle(node.Title).
		SetSource(node.Source).
		SetExternalID(node.ExternalID).
		SetVisibility(node.Visibility).
		SetFreshnessState(node.FreshnessState).
		SetObservedAt(node.ObservedAt).
		Exec(ctx)
}

func (s *EntStore) UpsertEdge(ctx context.Context, edge domain.Edge) error {
	if edge.From.Key == "" || edge.To.Key == "" || edge.Metadata.Predicate == "" {
		return fmt.Errorf("%w: from, to, and predicate are required", ErrInvalidExpansion)
	}
	if _, err := s.nodeByKey(ctx, edge.From.Key); err != nil {
		return err
	}
	if _, err := s.nodeByKey(ctx, edge.To.Key); err != nil {
		return err
	}
	if edge.Key == "" {
		edge.Key = edgeKey(edge)
	}
	existing, err := s.client.GraphEdge.Query().
		Where(graphedge.KeyEQ(edge.Key)).
		Only(ctx)
	if err == nil {
		return existing.Update().
			SetFromKind(string(edge.From.Kind)).
			SetFromKey(edge.From.Key).
			SetToKind(string(edge.To.Kind)).
			SetToKey(edge.To.Key).
			SetPredicate(string(edge.Metadata.Predicate)).
			SetEvidenceKey(edge.Metadata.EvidenceKey).
			SetSource(edge.Metadata.Source).
			SetConfidence(edge.Metadata.Confidence).
			SetVisibility(edge.Metadata.Visibility).
			SetFreshnessState(edge.Metadata.FreshnessState).
			SetObservedAt(edge.Metadata.ObservedAt).
			Exec(ctx)
	}
	if !ent.IsNotFound(err) {
		return err
	}
	return s.client.GraphEdge.Create().
		SetKey(edge.Key).
		SetFromKind(string(edge.From.Kind)).
		SetFromKey(edge.From.Key).
		SetToKind(string(edge.To.Kind)).
		SetToKey(edge.To.Key).
		SetPredicate(string(edge.Metadata.Predicate)).
		SetEvidenceKey(edge.Metadata.EvidenceKey).
		SetSource(edge.Metadata.Source).
		SetConfidence(edge.Metadata.Confidence).
		SetVisibility(edge.Metadata.Visibility).
		SetFreshnessState(edge.Metadata.FreshnessState).
		SetObservedAt(edge.Metadata.ObservedAt).
		Exec(ctx)
}

func (s *EntStore) Expand(ctx context.Context, req domain.ExpandRequest) (domain.Neighborhood, error) {
	if req.Start.Kind == "" || req.Start.Key == "" || req.Depth < 0 || req.LimitPerNode <= 0 {
		return domain.Neighborhood{}, fmt.Errorf("%w: start, non-negative depth, and positive limit are required", ErrInvalidExpansion)
	}
	start, err := s.nodeByKey(ctx, req.Start.Key)
	if err != nil {
		return domain.Neighborhood{}, err
	}

	allowedPredicates := predicateSet(req.Predicates)
	seenNodes := map[string]bool{req.Start.Key: true}
	seenEdges := make(map[string]bool)
	nodes := []domain.Node{nodeToDomain(start)}
	edges := make([]domain.Edge, 0)
	frontier := []domain.NodeRef{req.Start}

	for depth := 0; depth < req.Depth && len(frontier) > 0; depth++ {
		next := make([]domain.NodeRef, 0)
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
				edge := edgeToDomain(graphEdge)
				if len(allowedPredicates) > 0 && !allowedPredicates[edge.Metadata.Predicate] {
					continue
				}
				used++
				if used > req.LimitPerNode {
					break
				}
				if !seenEdges[edge.Key] {
					seenEdges[edge.Key] = true
					edges = append(edges, edge)
				}
				if !seenNodes[edge.To.Key] {
					node, err := s.nodeByKey(ctx, edge.To.Key)
					if err != nil {
						return domain.Neighborhood{}, err
					}
					seenNodes[edge.To.Key] = true
					nodes = append(nodes, nodeToDomain(node))
					next = append(next, edge.To)
				}
			}
		}
		frontier = next
	}

	return domain.Neighborhood{Nodes: nodes, Edges: edges}, nil
}

func (s *EntStore) nodeByKey(ctx context.Context, key string) (*ent.OntologyNode, error) {
	node, err := s.client.OntologyNode.Query().
		Where(ontologynode.KeyEQ(key)).
		Only(ctx)
	if ent.IsNotFound(err) {
		return nil, fmt.Errorf("%w: %s", ErrMissingNode, key)
	}
	return node, err
}

func nodeToDomain(node *ent.OntologyNode) domain.Node {
	return domain.Node{
		Kind:           domain.Kind(node.Kind),
		Key:            node.Key,
		Title:          node.Title,
		Source:         node.Source,
		ExternalID:     node.ExternalID,
		Visibility:     node.Visibility,
		FreshnessState: node.FreshnessState,
		ObservedAt:     node.ObservedAt,
	}
}

func edgeToDomain(edge *ent.GraphEdge) domain.Edge {
	return domain.Edge{
		Key:  edge.Key,
		From: domain.NodeRef{Kind: domain.Kind(edge.FromKind), Key: edge.FromKey},
		To:   domain.NodeRef{Kind: domain.Kind(edge.ToKind), Key: edge.ToKey},
		Metadata: domain.EdgeMetadata{
			Predicate:      domain.Predicate(edge.Predicate),
			EvidenceKey:    edge.EvidenceKey,
			Source:         edge.Source,
			Confidence:     edge.Confidence,
			Visibility:     edge.Visibility,
			FreshnessState: edge.FreshnessState,
			ObservedAt:     edge.ObservedAt,
		},
	}
}
