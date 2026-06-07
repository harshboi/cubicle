package fixtures

import (
	"context"
	"testing"

	"cubicle/services/ontology-service/internal/domain"
)

func TestFlinkAutoscalerStoreExpandsKnownWorkstream(t *testing.T) {
	store := NewFlinkAutoscalerStore()

	graph, err := store.Expand(context.Background(), domain.ExpandRequest{
		Start:        domain.NodeRef{Kind: domain.KindWorkstream, Key: "workstream:flink-autoscaler"},
		Depth:        2,
		LimitPerNode: 10,
	})
	if err != nil {
		t.Fatalf("expand fixture graph: %v", err)
	}
	if len(graph.Nodes) < 4 {
		t.Fatalf("expected connected graph fixture, got %d nodes: %#v", len(graph.Nodes), graph.Nodes)
	}
	if len(graph.Edges) < 3 {
		t.Fatalf("expected evidence-backed fixture edges, got %d edges: %#v", len(graph.Edges), graph.Edges)
	}
	for _, edge := range graph.Edges {
		if edge.Metadata.EvidenceKey == "" {
			t.Fatalf("edge %s has empty evidence key", edge.Key)
		}
	}
}
