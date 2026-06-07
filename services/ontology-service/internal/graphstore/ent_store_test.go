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

	objects := []domain.Object{
		{ObjectType: ontology.ObjectWorkstream, Key: "workstream:flink-autoscaler", Title: "Flink Autoscaler", Source: "fake_sampledata", Visibility: "public", FreshnessState: "fresh", ObservedAt: observedAt},
		{ObjectType: ontology.ObjectTicket, Key: "ticket:FLINK-39743", Title: "Incorrect Expected Processing Rate Computation", Source: "jira", ExternalID: "FLINK-39743", Visibility: "public", FreshnessState: "fresh", ObservedAt: observedAt},
		{ObjectType: ontology.ObjectPullRequest, Key: "pr:apache/flink-kubernetes-operator#1127", Title: "[FLINK-39743] Fix expected processing rate", Source: "github", ExternalID: "apache/flink-kubernetes-operator#1127", Visibility: "public", FreshnessState: "fresh", ObservedAt: observedAt},
		{ObjectType: ontology.ObjectCodeFile, Key: "file:JobVertexScaler.java", Title: "JobVertexScaler.java", Source: "github", Visibility: "public", FreshnessState: "fresh", ObservedAt: observedAt},
	}
	for _, object := range objects {
		if err := store.UpsertObject(ctx, object); err != nil {
			t.Fatalf("upsert object %s: %v", object.Key, err)
		}
	}

	associations := []domain.Association{
		association("workstream:flink-autoscaler", ontology.ObjectWorkstream, ontology.AssocContains, "ticket:FLINK-39743", ontology.ObjectTicket, "evidence:jira-component", observedAt),
		association("ticket:FLINK-39743", ontology.ObjectTicket, ontology.AssocImplementedBy, "pr:apache/flink-kubernetes-operator#1127", ontology.ObjectPullRequest, "evidence:jira-remote-link", observedAt),
		association("pr:apache/flink-kubernetes-operator#1127", ontology.ObjectPullRequest, ontology.AssocChangesFile, "file:JobVertexScaler.java", ontology.ObjectCodeFile, "evidence:github-files", observedAt),
	}
	for _, association := range associations {
		if err := store.UpsertAssociation(ctx, association); err != nil {
			t.Fatalf("upsert association %s: %v", association.Key, err)
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

	assertObject(t, graph, "ticket:FLINK-39743")
	assertObject(t, graph, "pr:apache/flink-kubernetes-operator#1127")
	assertObject(t, graph, "file:JobVertexScaler.java")
	assertAssociation(t, graph, "workstream:flink-autoscaler", ontology.AssocContains, "ticket:FLINK-39743")
	assertAssociation(t, graph, "ticket:FLINK-39743", ontology.AssocImplementedBy, "pr:apache/flink-kubernetes-operator#1127")
	assertAssociation(t, graph, "pr:apache/flink-kubernetes-operator#1127", ontology.AssocChangesFile, "file:JobVertexScaler.java")
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
		t.Fatalf("upsert association: %v", err)
	}

	graph, err := store.Expand(ctx, domain.ExpandRequest{
		Start:          domain.ObjectRef{ObjectType: ontology.ObjectWorkstream, Key: "workstream:flink-autoscaler"},
		Depth:          1,
		LimitPerObject: 10,
	})
	if err != nil {
		t.Fatalf("expand persisted graph: %v", err)
	}

	object := objectByKey(t, graph, "ticket:FLINK-39743")
	if object.SourceInstance != "apache-jira" || object.SourceURL == "" || object.SnapshotKey == "" || object.MapperVersion == "" {
		t.Fatalf("object ingest metadata was not preserved: %#v", object)
	}
	if !object.SourceUpdatedAt.Equal(sourceUpdatedAt) || object.PropertiesJSON != `{"priority":"major"}` {
		t.Fatalf("object source details were not preserved: %#v", object)
	}

	graphAssociation := associationByType(t, graph, ontology.AssocContains)
	metadata := graphAssociation.Metadata
	if metadata.SourceInstance != "apache-jira" || metadata.SourceURL == "" || metadata.SnapshotKey == "" || metadata.MapperVersion == "" {
		t.Fatalf("association ingest metadata was not preserved: %#v", metadata)
	}
	if !metadata.SourceUpdatedAt.Equal(sourceUpdatedAt) || metadata.PropertiesJSON != `{"relationship":"component"}` {
		t.Fatalf("association source details were not preserved: %#v", metadata)
	}
}

func objectByKey(t *testing.T, graph domain.Neighborhood, key string) domain.Object {
	t.Helper()
	for _, object := range graph.Objects {
		if object.Key == key {
			return object
		}
	}
	t.Fatalf("expected object %q in graph; got %#v", key, graph.Objects)
	return domain.Object{}
}

func associationByType(t *testing.T, graph domain.Neighborhood, associationType domain.AssociationType) domain.Association {
	t.Helper()
	for _, association := range graph.Associations {
		if association.AssociationType == associationType {
			return association
		}
	}
	t.Fatalf("expected association with type %q in graph; got %#v", associationType, graph.Associations)
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
