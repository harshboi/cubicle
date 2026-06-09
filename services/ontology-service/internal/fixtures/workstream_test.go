package fixtures

import (
	"context"
	"testing"

	"cubicle/services/ontology-service/internal/domain"
	"cubicle/services/ontology-service/internal/ontology"
)

func TestFlinkAutoscalerStoreExpandsKnownWorkstream(t *testing.T) {
	store := NewFlinkAutoscalerStore()

	graph, err := store.Expand(context.Background(), domain.ExpandRequest{
		Start:          domain.ObjectRef{ObjectType: ontology.ObjectWorkstream, Key: "workstream:flink-autoscaler"},
		Depth:          2,
		LimitPerObject: 10,
	})
	if err != nil {
		t.Fatalf("expand fixture graph: %v", err)
	}
	if len(graph.Objects) < 4 {
		t.Fatalf("expected connected graph fixture, got %d nodes: %#v", len(graph.Objects), graph.Objects)
	}
	if len(graph.Associations) < 3 {
		t.Fatalf("expected evidence-backed fixture edges, got %d edges: %#v", len(graph.Associations), graph.Associations)
	}
	for _, edge := range graph.Associations {
		if edge.Metadata.EvidenceKey == "" {
			t.Fatalf("edge %s has empty evidence key", edge.Key)
		}
	}
}
