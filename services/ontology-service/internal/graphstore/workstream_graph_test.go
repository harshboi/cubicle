package graphstore

import (
	"context"
	"testing"
	"time"

	"cubicle/services/ontology-service/internal/domain"
)

func TestWorkstreamNeighborhoodReturnsConnectedGraphWithEvidence(t *testing.T) {
	ctx := context.Background()
	store := NewMemoryStore()
	observedAt := time.Date(2026, 6, 7, 10, 0, 0, 0, time.UTC)

	nodes := []domain.Node{
		{Kind: domain.KindWorkstream, Key: "workstream:flink-autoscaler", Title: "Flink Autoscaler"},
		{Kind: domain.KindTicket, Key: "ticket:FLINK-39743", Title: "Incorrect Expected Processing Rate Computation"},
		{Kind: domain.KindPullRequest, Key: "pr:apache/flink-kubernetes-operator#1127", Title: "[FLINK-39743] Fix expected processing rate"},
		{Kind: domain.KindCodeFile, Key: "file:JobVertexScaler.java", Title: "JobVertexScaler.java"},
		{Kind: domain.KindBlocker, Key: "blocker:missing-review", Title: "Missing review"},
		{Kind: domain.KindActionCandidate, Key: "action:request-review", Title: "Request review"},
	}
	for _, node := range nodes {
		if err := store.UpsertNode(ctx, node); err != nil {
			t.Fatalf("upsert node %s: %v", node.Key, err)
		}
	}

	edges := []domain.Edge{
		edge("workstream:flink-autoscaler", domain.KindWorkstream, domain.PredicateContains, "ticket:FLINK-39743", domain.KindTicket, "evidence:jira-component", observedAt),
		edge("ticket:FLINK-39743", domain.KindTicket, domain.PredicateImplementedBy, "pr:apache/flink-kubernetes-operator#1127", domain.KindPullRequest, "evidence:jira-remote-link", observedAt),
		edge("pr:apache/flink-kubernetes-operator#1127", domain.KindPullRequest, domain.PredicateChangesFile, "file:JobVertexScaler.java", domain.KindCodeFile, "evidence:github-files", observedAt),
		edge("ticket:FLINK-39743", domain.KindTicket, domain.PredicateBlockedBy, "blocker:missing-review", domain.KindBlocker, "evidence:review-gap", observedAt),
		edge("blocker:missing-review", domain.KindBlocker, domain.PredicateNeedsAction, "action:request-review", domain.KindActionCandidate, "evidence:action-rule", observedAt),
	}
	for _, edge := range edges {
		if err := store.UpsertEdge(ctx, edge); err != nil {
			t.Fatalf("upsert edge %s: %v", edge.Key, err)
		}
	}

	graph, err := store.Expand(ctx, domain.ExpandRequest{
		Start: domain.NodeRef{
			Kind: domain.KindWorkstream,
			Key:  "workstream:flink-autoscaler",
		},
		Predicates: []domain.Predicate{
			domain.PredicateContains,
			domain.PredicateImplementedBy,
			domain.PredicateChangesFile,
			domain.PredicateBlockedBy,
			domain.PredicateNeedsAction,
		},
		Depth:        4,
		LimitPerNode: 10,
	})
	if err != nil {
		t.Fatalf("expand workstream: %v", err)
	}

	assertNode(t, graph, "ticket:FLINK-39743")
	assertNode(t, graph, "pr:apache/flink-kubernetes-operator#1127")
	assertNode(t, graph, "file:JobVertexScaler.java")
	assertNode(t, graph, "blocker:missing-review")
	assertNode(t, graph, "action:request-review")
	assertEdge(t, graph, "workstream:flink-autoscaler", domain.PredicateContains, "ticket:FLINK-39743")
	assertEdge(t, graph, "ticket:FLINK-39743", domain.PredicateImplementedBy, "pr:apache/flink-kubernetes-operator#1127")
	assertEdge(t, graph, "pr:apache/flink-kubernetes-operator#1127", domain.PredicateChangesFile, "file:JobVertexScaler.java")
	assertEdge(t, graph, "ticket:FLINK-39743", domain.PredicateBlockedBy, "blocker:missing-review")
	assertEdge(t, graph, "blocker:missing-review", domain.PredicateNeedsAction, "action:request-review")
}

func TestWorkstreamNeighborhoodRequiresBounds(t *testing.T) {
	store := NewMemoryStore()
	_, err := store.Expand(context.Background(), domain.ExpandRequest{
		Start: domain.NodeRef{Kind: domain.KindWorkstream, Key: "workstream:flink-autoscaler"},
		Depth: 2,
	})
	if err == nil {
		t.Fatal("expected missing limit to fail")
	}
}

func edge(fromKey string, fromKind domain.Kind, predicate domain.Predicate, toKey string, toKind domain.Kind, evidenceKey string, observedAt time.Time) domain.Edge {
	return domain.Edge{
		From: domain.NodeRef{Kind: fromKind, Key: fromKey},
		To:   domain.NodeRef{Kind: toKind, Key: toKey},
		Metadata: domain.EdgeMetadata{
			Predicate:      predicate,
			EvidenceKey:    evidenceKey,
			Source:         "fixture",
			Confidence:     1,
			Visibility:     "public",
			FreshnessState: "fresh",
			ObservedAt:     observedAt,
		},
	}
}

func assertNode(t *testing.T, graph domain.Neighborhood, key string) {
	t.Helper()
	for _, node := range graph.Nodes {
		if node.Key == key {
			return
		}
	}
	t.Fatalf("expected node %q in graph; got %#v", key, graph.Nodes)
}

func assertEdge(t *testing.T, graph domain.Neighborhood, fromKey string, predicate domain.Predicate, toKey string) {
	t.Helper()
	for _, edge := range graph.Edges {
		if edge.From.Key == fromKey && edge.Metadata.Predicate == predicate && edge.To.Key == toKey {
			if edge.Metadata.EvidenceKey == "" {
				t.Fatalf("edge %s -> %s has no evidence", fromKey, toKey)
			}
			return
		}
	}
	t.Fatalf("expected edge %q -[%s]-> %q in graph; got %#v", fromKey, predicate, toKey, graph.Edges)
}
