package flink

import (
	"context"
	"testing"
	"time"

	"cubicle/services/ontology-service/ent"
	"cubicle/services/ontology-service/internal/domain"
	"cubicle/services/ontology-service/internal/graphstore"
	"cubicle/services/ontology-service/internal/ontology"
	"cubicle/services/ontology-service/internal/storage"
	"entgo.io/ent/dialect"
	entsql "entgo.io/ent/dialect/sql"
)

func TestFixtureImporterWritesSnapshotsBeforeMappedBatches(t *testing.T) {
	ctx := context.Background()
	writer := &recordingIngestWriter{}
	importer := NewFixtureImporter(writer)

	result, err := importer.Import(ctx, FixtureImportConfig{
		FixtureDir:   "testdata/flink-fixture",
		SnapshotRoot: t.TempDir(),
		Now:          time.Date(2026, 6, 7, 12, 0, 0, 0, time.UTC),
	})
	if err != nil {
		t.Fatalf("import fixture: %v", err)
	}
	if result.RunsCompleted != 4 {
		t.Fatalf("runs completed = %d", result.RunsCompleted)
	}
	if len(writer.calls) == 0 {
		t.Fatal("writer was not called")
	}
	for i, call := range writer.calls {
		if call == "batch" && !writer.sawSnapshotBeforeBatch {
			t.Fatalf("batch call %d happened before any snapshot: %v", i, writer.calls)
		}
	}
	if writer.batchCount != 4 {
		t.Fatalf("batch count = %d, calls=%v", writer.batchCount, writer.calls)
	}
}

func TestFixtureImporterReplaysThroughEntStoreIdempotently(t *testing.T) {
	ctx := context.Background()
	client := openEntClient(t, ctx)
	store := graphstore.NewEntStore(client)
	importer := NewFixtureImporter(store)
	cfg := FixtureImportConfig{
		FixtureDir:   "testdata/flink-fixture",
		SnapshotRoot: t.TempDir(),
		Now:          time.Date(2026, 6, 7, 12, 0, 0, 0, time.UTC),
	}

	first, err := importer.Import(ctx, cfg)
	if err != nil {
		t.Fatalf("first import: %v", err)
	}
	second, err := importer.Import(ctx, cfg)
	if err != nil {
		t.Fatalf("second import: %v", err)
	}
	if first.ObjectsUpserted != second.ObjectsUpserted || first.AssociationsUpserted != second.AssociationsUpserted {
		t.Fatalf("replay changed counts: first=%#v second=%#v", first, second)
	}

	graph, err := store.Expand(ctx, domain.ExpandRequest{
		Start:          domain.ObjectRef{ObjectType: ontology.ObjectWorkstream, Key: "workstream:flink-autoscaler"},
		Depth:          3,
		LimitPerObject: 20,
	})
	if err != nil {
		t.Fatalf("expand imported graph: %v", err)
	}
	assertNode(t, graph, "ticket:FLINK-39743")
	assertNode(t, graph, "pr:apache/flink-kubernetes-operator#1234")
	assertNode(t, graph, "document:apache/flink-kubernetes-operator:docs/content/docs/custom-resource/autoscaler.md")
	assertEdge(t, graph, ontology.AssocImplementedBy)
	assertEdge(t, graph, ontology.AssocChangesFile)
	assertEdge(t, graph, ontology.AssocDiscussedIn)
}

func openEntClient(t *testing.T, ctx context.Context) *ent.Client {
	t.Helper()
	sqlite, err := storage.Open(ctx, storage.Config{DatabasePath: t.TempDir() + "/graph.db"})
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	t.Cleanup(func() { _ = sqlite.Close() })
	client := ent.NewClient(ent.Driver(entsql.OpenDB(dialect.SQLite, sqlite.DB())))
	t.Cleanup(func() { _ = client.Close() })
	if err := client.Schema.Create(ctx); err != nil {
		t.Fatalf("create schema: %v", err)
	}
	return client
}

func assertNode(t *testing.T, graph domain.Neighborhood, key string) {
	t.Helper()
	for _, node := range graph.Objects {
		if node.Key == key {
			return
		}
	}
	t.Fatalf("missing node %q in %#v", key, graph.Objects)
}

func assertEdge(t *testing.T, graph domain.Neighborhood, predicate domain.AssociationType) {
	t.Helper()
	for _, edge := range graph.Associations {
		if edge.AssociationType == predicate {
			return
		}
	}
	t.Fatalf("missing edge predicate %q in %#v", predicate, graph.Associations)
}

type recordingIngestWriter struct {
	runs                   map[string]domain.IngestRun
	calls                  []string
	batchCount             int
	sawSnapshot            bool
	sawSnapshotBeforeBatch bool
}

func (w *recordingIngestWriter) BeginIngestRun(_ context.Context, start domain.IngestRunStart) (domain.IngestRun, error) {
	if w.runs == nil {
		w.runs = make(map[string]domain.IngestRun)
	}
	run := domain.IngestRun{
		RunKey:         start.RunKey,
		Source:         start.Source,
		SourceInstance: start.SourceInstance,
		Slice:          start.Slice,
		MapperVersion:  start.MapperVersion,
		Status:         domain.IngestRunOpen,
		StartedAt:      start.StartedAt,
	}
	w.runs[run.RunKey] = run
	w.calls = append(w.calls, "run")
	return run, nil
}

func (w *recordingIngestWriter) WriteSnapshot(_ context.Context, snapshot domain.SourceSnapshotWrite) (domain.SourceSnapshot, error) {
	w.calls = append(w.calls, "snapshot")
	w.sawSnapshot = true
	return domain.SourceSnapshot{SourceSnapshotWrite: snapshot, CreatedAt: snapshot.FetchedAt}, nil
}

func (w *recordingIngestWriter) WriteMappedBatch(_ context.Context, batch domain.IngestBatch) (domain.IngestBatchResult, error) {
	w.calls = append(w.calls, "batch")
	w.batchCount++
	if w.sawSnapshot {
		w.sawSnapshotBeforeBatch = true
	}
	return domain.IngestBatchResult{
		RunKey:               batch.RunKey,
		ObjectsUpserted:      len(batch.Objects),
		AssociationsUpserted: len(batch.Associations),
		EvidenceUpserted:     len(batch.Evidence),
		EventsUpserted:       len(batch.Events),
	}, nil
}

func (w *recordingIngestWriter) CompleteIngestRun(_ context.Context, complete domain.IngestRunComplete) (domain.IngestRun, error) {
	w.calls = append(w.calls, "complete")
	run := w.runs[complete.RunKey]
	run.Status = complete.Status
	run.CompletedAt = complete.CompletedAt
	return run, nil
}

func (w *recordingIngestWriter) GetIngestRun(_ context.Context, runKey string) (domain.IngestRun, error) {
	return w.runs[runKey], nil
}

func (w *recordingIngestWriter) ListSourceStatus(context.Context) ([]domain.SourceStatus, error) {
	return nil, nil
}
