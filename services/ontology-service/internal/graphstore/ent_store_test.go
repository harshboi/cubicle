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

func TestEntStoreRoundTripsIngestFactMetadata(t *testing.T) {
	ctx := context.Background()
	client := openEntClient(t, ctx)
	store := NewEntStore(client)
	observedAt := time.Date(2026, 6, 7, 10, 0, 0, 0, time.UTC)
	sourceUpdatedAt := time.Date(2026, 6, 6, 18, 30, 0, 0, time.UTC)

	if err := store.UpsertNode(ctx, domain.Node{
		Kind:           domain.KindWorkstream,
		Key:            "workstream:flink-autoscaler",
		Title:          "Flink Autoscaler",
		Source:         "fixture",
		Visibility:     domain.VisibilityPublic,
		FreshnessState: domain.FreshnessFresh,
		ObservedAt:     observedAt,
	}); err != nil {
		t.Fatalf("upsert workstream: %v", err)
	}
	if err := store.UpsertNode(ctx, domain.Node{
		Kind:            domain.KindTicket,
		Key:             "ticket:FLINK-39743",
		Title:           "Autoscaler bug",
		Source:          "jira",
		SourceInstance:  "apache-jira",
		ExternalID:      "FLINK-39743",
		SourceURL:       "https://issues.apache.org/jira/browse/FLINK-39743",
		SnapshotKey:     "snapshot:jira:FLINK-39743",
		MapperVersion:   "flink-fixture/v1",
		Visibility:      domain.VisibilityPublic,
		FreshnessState:  domain.FreshnessFresh,
		ObservedAt:      observedAt,
		SourceUpdatedAt: sourceUpdatedAt,
		PropertiesJSON:  `{"priority":"major"}`,
	}); err != nil {
		t.Fatalf("upsert ticket: %v", err)
	}
	if err := store.UpsertEdge(ctx, domain.Edge{
		From: domain.NodeRef{Kind: domain.KindWorkstream, Key: "workstream:flink-autoscaler"},
		To:   domain.NodeRef{Kind: domain.KindTicket, Key: "ticket:FLINK-39743"},
		Metadata: domain.EdgeMetadata{
			Predicate:       domain.PredicateContains,
			EvidenceKey:     "evidence:jira:FLINK-39743",
			Source:          "jira",
			SourceInstance:  "apache-jira",
			SourceURL:       "https://issues.apache.org/jira/browse/FLINK-39743",
			SnapshotKey:     "snapshot:jira:FLINK-39743",
			MapperVersion:   "flink-fixture/v1",
			Confidence:      0.92,
			Visibility:      domain.VisibilityPublic,
			FreshnessState:  domain.FreshnessFresh,
			ObservedAt:      observedAt,
			SourceUpdatedAt: sourceUpdatedAt,
			PropertiesJSON:  `{"relationship":"component"}`,
		},
	}); err != nil {
		t.Fatalf("upsert edge: %v", err)
	}

	graph, err := store.Expand(ctx, domain.ExpandRequest{
		Start:        domain.NodeRef{Kind: domain.KindWorkstream, Key: "workstream:flink-autoscaler"},
		Depth:        1,
		LimitPerNode: 10,
	})
	if err != nil {
		t.Fatalf("expand persisted graph: %v", err)
	}

	node := nodeByKey(t, graph, "ticket:FLINK-39743")
	if node.SourceInstance != "apache-jira" || node.SourceURL == "" || node.SnapshotKey == "" || node.MapperVersion == "" {
		t.Fatalf("node ingest metadata was not preserved: %#v", node)
	}
	if !node.SourceUpdatedAt.Equal(sourceUpdatedAt) || node.PropertiesJSON != `{"priority":"major"}` {
		t.Fatalf("node source details were not preserved: %#v", node)
	}

	graphEdge := edgeByPredicate(t, graph, domain.PredicateContains)
	metadata := graphEdge.Metadata
	if metadata.SourceInstance != "apache-jira" || metadata.SourceURL == "" || metadata.SnapshotKey == "" || metadata.MapperVersion == "" {
		t.Fatalf("edge ingest metadata was not preserved: %#v", metadata)
	}
	if !metadata.SourceUpdatedAt.Equal(sourceUpdatedAt) || metadata.PropertiesJSON != `{"relationship":"component"}` {
		t.Fatalf("edge source details were not preserved: %#v", metadata)
	}
}

func nodeByKey(t *testing.T, graph domain.Neighborhood, key string) domain.Node {
	t.Helper()
	for _, node := range graph.Nodes {
		if node.Key == key {
			return node
		}
	}
	t.Fatalf("expected node %q in graph; got %#v", key, graph.Nodes)
	return domain.Node{}
}

func edgeByPredicate(t *testing.T, graph domain.Neighborhood, predicate domain.Predicate) domain.Edge {
	t.Helper()
	for _, edge := range graph.Edges {
		if edge.Metadata.Predicate == predicate {
			return edge
		}
	}
	t.Fatalf("expected edge with predicate %q in graph; got %#v", predicate, graph.Edges)
	return domain.Edge{}
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
