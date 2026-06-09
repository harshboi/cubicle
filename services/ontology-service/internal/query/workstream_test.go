package query

import (
	"context"
	"testing"

	"cubicle/services/ontology-service/internal/domain"
	"cubicle/services/ontology-service/internal/fixtures"
)

func TestWorkstreamServiceOverviewClassifiesGraphNodes(t *testing.T) {
	service := NewWorkstreamService(fixtures.NewFlinkAutoscalerStore())

	overview, err := service.Overview(context.Background(), "flink-autoscaler")
	if err != nil {
		t.Fatalf("load overview: %v", err)
	}

	if overview.Workstream.Key != "workstream:flink-autoscaler" {
		t.Fatalf("unexpected workstream: %#v", overview.Workstream)
	}
	assertNodeKey(t, overview.Tickets, "ticket:FLINK-39743")
	assertNodeKey(t, overview.PullRequests, "pr:apache/flink-kubernetes-operator#1127")
	assertNodeKey(t, overview.CodeFiles, "file:JobVertexScaler.java")
	assertNodeKey(t, overview.Blockers, "blocker:missing-review")
	assertNodeKey(t, overview.ActionCandidates, "action:request-review")
	if len(overview.Associations) == 0 {
		t.Fatal("expected evidence edges in overview")
	}
}

func assertNodeKey(t *testing.T, nodes []domain.Object, key string) {
	t.Helper()
	for _, node := range nodes {
		if node.Key == key {
			return
		}
	}
	t.Fatalf("missing node %s in %#v", key, nodes)
}
