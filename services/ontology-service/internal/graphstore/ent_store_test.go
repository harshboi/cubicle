package graphstore

import (
	"context"
	"path/filepath"
	"testing"
	"time"

	"cubicle/services/ontology-service/ent"
	"cubicle/services/ontology-service/internal/domain"
	"cubicle/services/ontology-service/internal/storage"
	"entgo.io/ent/dialect"
	entsql "entgo.io/ent/dialect/sql"
)

func TestEntStoreExpandReturnsPersistedGraphWithEvidence(t *testing.T) {
	ctx := context.Background()
	client := openEntClient(t, ctx)
	store := NewEntStore(client)
	observedAt := time.Date(2026, 6, 7, 10, 0, 0, 0, time.UTC)

	nodes := []domain.Node{
		{Kind: domain.KindWorkstream, Key: "workstream:flink-autoscaler", Title: "Flink Autoscaler", Source: "fixture", Visibility: "public", FreshnessState: "fresh", ObservedAt: observedAt},
		{Kind: domain.KindTicket, Key: "ticket:FLINK-39743", Title: "Incorrect Expected Processing Rate Computation", Source: "jira", ExternalID: "FLINK-39743", Visibility: "public", FreshnessState: "fresh", ObservedAt: observedAt},
		{Kind: domain.KindPullRequest, Key: "pr:apache/flink-kubernetes-operator#1127", Title: "[FLINK-39743] Fix expected processing rate", Source: "github", ExternalID: "apache/flink-kubernetes-operator#1127", Visibility: "public", FreshnessState: "fresh", ObservedAt: observedAt},
		{Kind: domain.KindCodeFile, Key: "file:JobVertexScaler.java", Title: "JobVertexScaler.java", Source: "github", Visibility: "public", FreshnessState: "fresh", ObservedAt: observedAt},
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
	}
	for _, edge := range edges {
		if err := store.UpsertEdge(ctx, edge); err != nil {
			t.Fatalf("upsert edge %s: %v", edge.Key, err)
		}
	}

	graph, err := store.Expand(ctx, domain.ExpandRequest{
		Start:        domain.NodeRef{Kind: domain.KindWorkstream, Key: "workstream:flink-autoscaler"},
		Depth:        3,
		LimitPerNode: 10,
	})
	if err != nil {
		t.Fatalf("expand persisted graph: %v", err)
	}

	assertNode(t, graph, "ticket:FLINK-39743")
	assertNode(t, graph, "pr:apache/flink-kubernetes-operator#1127")
	assertNode(t, graph, "file:JobVertexScaler.java")
	assertEdge(t, graph, "workstream:flink-autoscaler", domain.PredicateContains, "ticket:FLINK-39743")
	assertEdge(t, graph, "ticket:FLINK-39743", domain.PredicateImplementedBy, "pr:apache/flink-kubernetes-operator#1127")
	assertEdge(t, graph, "pr:apache/flink-kubernetes-operator#1127", domain.PredicateChangesFile, "file:JobVertexScaler.java")
}

func openEntClient(t *testing.T, ctx context.Context) *ent.Client {
	t.Helper()
	store, err := storage.Open(ctx, storage.Config{
		DatabasePath: filepath.Join(t.TempDir(), "graph.db"),
	})
	if err != nil {
		t.Fatalf("open sqlite store: %v", err)
	}
	t.Cleanup(func() { _ = store.Close() })

	client := ent.NewClient(ent.Driver(entsql.OpenDB(dialect.SQLite, store.DB())))
	t.Cleanup(func() { _ = client.Close() })
	if err := client.Schema.Create(ctx); err != nil {
		t.Fatalf("create ent schema: %v", err)
	}
	return client
}
