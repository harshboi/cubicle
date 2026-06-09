package graphstore

import (
	"context"
	"path/filepath"
	"testing"
	"time"

	"cubicle/services/ontology-service/ent"
	"cubicle/services/ontology-service/internal/domain"
	"cubicle/services/ontology-service/internal/ontology"
	"cubicle/services/ontology-service/internal/storage"
	"entgo.io/ent/dialect"
	entsql "entgo.io/ent/dialect/sql"
)

func TestEntStoreExpandReturnsPersistedGraphWithEvidence(t *testing.T) {
	ctx := context.Background()
	client := openEntClient(t, ctx)
	store := NewEntStore(client)
	observedAt := time.Date(2026, 6, 7, 10, 0, 0, 0, time.UTC)

	nodes := []domain.Object{
		{ObjectType: ontology.ObjectWorkstream, Key: "workstream:flink-autoscaler", Title: "Flink Autoscaler", Source: "fixture", Visibility: "public", FreshnessState: "fresh", ObservedAt: observedAt},
		{ObjectType: ontology.ObjectTicket, Key: "ticket:FLINK-39743", Title: "Incorrect Expected Processing Rate Computation", Source: "jira", ExternalID: "FLINK-39743", Visibility: "public", FreshnessState: "fresh", ObservedAt: observedAt},
		{ObjectType: ontology.ObjectPullRequest, Key: "pr:apache/flink-kubernetes-operator#1127", Title: "[FLINK-39743] Fix expected processing rate", Source: "github", ExternalID: "apache/flink-kubernetes-operator#1127", Visibility: "public", FreshnessState: "fresh", ObservedAt: observedAt},
		{ObjectType: ontology.ObjectCodeFile, Key: "file:JobVertexScaler.java", Title: "JobVertexScaler.java", Source: "github", Visibility: "public", FreshnessState: "fresh", ObservedAt: observedAt},
	}
	for _, node := range nodes {
		if err := store.UpsertObject(ctx, node); err != nil {
			t.Fatalf("upsert node %s: %v", node.Key, err)
		}
	}

	edges := []domain.Association{
		edge("workstream:flink-autoscaler", ontology.ObjectWorkstream, ontology.AssocContains, "ticket:FLINK-39743", ontology.ObjectTicket, "evidence:jira-component", observedAt),
		edge("ticket:FLINK-39743", ontology.ObjectTicket, ontology.AssocImplementedBy, "pr:apache/flink-kubernetes-operator#1127", ontology.ObjectPullRequest, "evidence:jira-remote-link", observedAt),
		edge("pr:apache/flink-kubernetes-operator#1127", ontology.ObjectPullRequest, ontology.AssocChangesFile, "file:JobVertexScaler.java", ontology.ObjectCodeFile, "evidence:github-files", observedAt),
	}
	for _, edge := range edges {
		if err := store.UpsertAssociation(ctx, edge); err != nil {
			t.Fatalf("upsert edge %s: %v", edge.Key, err)
		}
	}

	graph, err := store.Expand(ctx, domain.ExpandRequest{
		Start:          domain.ObjectRef{ObjectType: ontology.ObjectWorkstream, Key: "workstream:flink-autoscaler"},
		Depth:          3,
		LimitPerObject: 10,
	})
	if err != nil {
		t.Fatalf("expand persisted graph: %v", err)
	}

	assertNode(t, graph, "ticket:FLINK-39743")
	assertNode(t, graph, "pr:apache/flink-kubernetes-operator#1127")
	assertNode(t, graph, "file:JobVertexScaler.java")
	assertEdge(t, graph, "workstream:flink-autoscaler", ontology.AssocContains, "ticket:FLINK-39743")
	assertEdge(t, graph, "ticket:FLINK-39743", ontology.AssocImplementedBy, "pr:apache/flink-kubernetes-operator#1127")
	assertEdge(t, graph, "pr:apache/flink-kubernetes-operator#1127", ontology.AssocChangesFile, "file:JobVertexScaler.java")
}

func TestEntStoreRoundTripsIngestFactMetadata(t *testing.T) {
	ctx := context.Background()
	client := openEntClient(t, ctx)
	store := NewEntStore(client)
	observedAt := time.Date(2026, 6, 7, 10, 0, 0, 0, time.UTC)
	sourceUpdatedAt := time.Date(2026, 6, 6, 18, 30, 0, 0, time.UTC)

	if err := store.UpsertObject(ctx, domain.Object{
		ObjectType:     ontology.ObjectWorkstream,
		Key:            "workstream:flink-autoscaler",
		Title:          "Flink Autoscaler",
		Source:         "fixture",
		Visibility:     domain.VisibilityPublic,
		FreshnessState: domain.FreshnessFresh,
		ObservedAt:     observedAt,
	}); err != nil {
		t.Fatalf("upsert workstream: %v", err)
	}
	if err := store.UpsertObject(ctx, domain.Object{
		ObjectType:      ontology.ObjectTicket,
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
	if err := store.UpsertAssociation(ctx, domain.Association{
		From:            domain.ObjectRef{ObjectType: ontology.ObjectWorkstream, Key: "workstream:flink-autoscaler"},
		To:              domain.ObjectRef{ObjectType: ontology.ObjectTicket, Key: "ticket:FLINK-39743"},
		AssociationType: ontology.AssocContains,
		Metadata: domain.AssociationMetadata{
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
		Start:          domain.ObjectRef{ObjectType: ontology.ObjectWorkstream, Key: "workstream:flink-autoscaler"},
		Depth:          1,
		LimitPerObject: 10,
	})
	if err != nil {
		t.Fatalf("expand persisted graph: %v", err)
	}

	node := objectByKey(t, graph, "ticket:FLINK-39743")
	if node.SourceInstance != "apache-jira" || node.SourceURL == "" || node.SnapshotKey == "" || node.MapperVersion == "" {
		t.Fatalf("node ingest metadata was not preserved: %#v", node)
	}
	if !node.SourceUpdatedAt.Equal(sourceUpdatedAt) || node.PropertiesJSON != `{"priority":"major"}` {
		t.Fatalf("node source details were not preserved: %#v", node)
	}

	graphEdge := edgeByPredicate(t, graph, ontology.AssocContains)
	metadata := graphEdge.Metadata
	if metadata.SourceInstance != "apache-jira" || metadata.SourceURL == "" || metadata.SnapshotKey == "" || metadata.MapperVersion == "" {
		t.Fatalf("edge ingest metadata was not preserved: %#v", metadata)
	}
	if !metadata.SourceUpdatedAt.Equal(sourceUpdatedAt) || metadata.PropertiesJSON != `{"relationship":"component"}` {
		t.Fatalf("edge source details were not preserved: %#v", metadata)
	}
}

func TestEntStoreAssociationIdentityIgnoresEvidenceKey(t *testing.T) {
	ctx := context.Background()
	client := openEntClient(t, ctx)
	store := NewEntStore(client)

	objects := []domain.Object{
		{ObjectType: "custom_system", Key: "custom:system:atlas", Title: "Atlas"},
		{ObjectType: "custom_artifact", Key: "custom:artifact:readiness", Title: "Readiness note"},
	}
	for _, object := range objects {
		if err := store.UpsertObject(ctx, object); err != nil {
			t.Fatalf("upsert object %s: %v", object.Key, err)
		}
	}

	association := domain.Association{
		From:            objects[0].Ref(),
		To:              objects[1].Ref(),
		AssociationType: "custom_depends_on",
		Metadata: domain.AssociationMetadata{
			EvidenceKey: "evidence:first",
			Confidence:  0.7,
		},
	}
	if err := store.UpsertAssociation(ctx, association); err != nil {
		t.Fatalf("upsert first association: %v", err)
	}
	association.Metadata.EvidenceKey = "evidence:latest"
	association.Metadata.Confidence = 0.9
	if err := store.UpsertAssociation(ctx, association); err != nil {
		t.Fatalf("upsert replacement association: %v", err)
	}

	graph, err := store.Expand(ctx, domain.ExpandRequest{
		Start:            objects[0].Ref(),
		AssociationTypes: []domain.AssociationType{"custom_depends_on"},
		Depth:            1,
		LimitPerObject:   10,
	})
	if err != nil {
		t.Fatalf("expand custom graph: %v", err)
	}
	if len(graph.Associations) != 1 {
		t.Fatalf("expected one logical association, got %#v", graph.Associations)
	}
	if graph.Associations[0].Metadata.EvidenceKey != "evidence:latest" {
		t.Fatalf("association evidence was not replaced: %#v", graph.Associations[0])
	}
}

func objectByKey(t *testing.T, graph domain.Neighborhood, key string) domain.Object {
	t.Helper()
	for _, node := range graph.Objects {
		if node.Key == key {
			return node
		}
	}
	t.Fatalf("expected node %q in graph; got %#v", key, graph.Objects)
	return domain.Object{}
}

func edgeByPredicate(t *testing.T, graph domain.Neighborhood, predicate domain.AssociationType) domain.Association {
	t.Helper()
	for _, edge := range graph.Associations {
		if edge.AssociationType == predicate {
			return edge
		}
	}
	t.Fatalf("expected edge with predicate %q in graph; got %#v", predicate, graph.Associations)
	return domain.Association{}
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
