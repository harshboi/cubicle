package sampledata

import (
	"context"
	"testing"

	"cubicle/services/ontology-service/internal/domain"
	"cubicle/services/ontology-service/internal/ontology"
)

func TestFakeFlinkAutoscalerMemoryStoreExpandsKnownWorkstream(t *testing.T) {
	store := NewFakeFlinkAutoscalerMemoryStore()

	graph, err := store.Expand(context.Background(), domain.ExpandRequest{
		Start:          domain.ObjectRef{ObjectType: ontology.ObjectWorkstream, Key: "workstream:flink-autoscaler"},
		Depth:          2,
		LimitPerObject: 10,
	})
	if err != nil {
		t.Fatalf("expand fake sample graph: %v", err)
	}
	if len(graph.Objects) < 4 {
		t.Fatalf("expected connected fake sample graph, got %d objects: %#v", len(graph.Objects), graph.Objects)
	}
	if len(graph.Associations) < 3 {
		t.Fatalf("expected evidence-backed fake sample associations, got %d associations: %#v", len(graph.Associations), graph.Associations)
	}
	for _, association := range graph.Associations {
		if association.Metadata.EvidenceKey == "" {
			t.Fatalf("association %s has empty evidence key", association.Key)
		}
	}
}
