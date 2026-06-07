package query

import (
	"context"
	"testing"

	"cubicle/services/ontology-service/internal/domain"
	"cubicle/services/ontology-service/internal/fixtures"
)

func TestWorkstreamServiceOverviewClassifiesGraphObjects(t *testing.T) {
	service := NewWorkstreamService(fixtures.NewFlinkAutoscalerStore())

	overview, err := service.Overview(context.Background(), "flink-autoscaler")
	if err != nil {
		t.Fatalf("load overview: %v", err)
	}

	if overview.Workstream.Key != "workstream:flink-autoscaler" {
		t.Fatalf("unexpected workstream: %#v", overview.Workstream)
	}
	assertObjectKey(t, overview.Tickets, "ticket:FLINK-39743")
	assertObjectKey(t, overview.PullRequests, "pr:apache/flink-kubernetes-operator#1127")
	assertObjectKey(t, overview.CodeFiles, "file:JobVertexScaler.java")
	assertObjectKey(t, overview.Blockers, "blocker:missing-review")
	assertObjectKey(t, overview.ActionCandidates, "action:request-review")
	if len(overview.Associations) == 0 {
		t.Fatal("expected evidence associations in overview")
	}
}

func assertObjectKey(t *testing.T, objects []domain.Object, key string) {
	t.Helper()
	for _, object := range objects {
		if object.Key == key {
			return
		}
	}
	t.Fatalf("missing object %s in %#v", key, objects)
}
