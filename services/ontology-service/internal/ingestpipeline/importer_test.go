package ingestpipeline

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"os"
	"path/filepath"
	"testing"
	"time"

	"cubicle/services/ontology-service/internal/domain"
	"cubicle/services/ontology-service/internal/ontology"
	snapshotstore "cubicle/services/ontology-service/internal/snapshot"
)

func TestFixtureImporterIsSourceNeutral(t *testing.T) {
	ctx := context.Background()
	fixtureDir := t.TempDir()
	if err := os.WriteFile(filepath.Join(fixtureDir, "manifest.json"), []byte(`{
		"snapshots": [{
			"snapshot_key": "snapshot:custom:ticket:1",
			"source": "custom",
			"source_instance": "example/project",
			"source_object_type": "custom_ticket",
			"source_object_id": "TICKET-1",
			"source_url": "https://example.test/tickets/1",
			"path": "ticket.json"
		}]
	}`), 0o644); err != nil {
		t.Fatalf("write manifest: %v", err)
	}
	if err := os.WriteFile(filepath.Join(fixtureDir, "ticket.json"), []byte(`{"id":"TICKET-1"}`), 0o644); err != nil {
		t.Fatalf("write body: %v", err)
	}

	writer := &recordingWriter{}
	importer := NewFixtureImporter(writer, fakeMapper{})
	result, err := importer.Import(ctx, FixtureImportConfig{
		FixtureDir:   fixtureDir,
		SnapshotRoot: t.TempDir(),
		Now:          time.Date(2026, 6, 8, 12, 0, 0, 0, time.UTC),
		RunKeyPrefix: "run:custom-fixture",
	})
	if err != nil {
		t.Fatalf("import custom fixture: %v", err)
	}
	if result.RunsCompleted != 1 || result.SnapshotsWritten != 1 || result.ObjectsUpserted != 1 {
		t.Fatalf("unexpected import result: %#v", result)
	}
	if writer.batch.Source != "custom" || writer.batch.SourceInstance != "example/project" {
		t.Fatalf("mapper/source identity not preserved: %#v", writer.batch)
	}
	if writer.snapshot.BodySHA256 == "" || writer.snapshot.BodyRef == "" {
		t.Fatalf("snapshot body metadata not populated: %#v", writer.snapshot)
	}
}

func TestImporterAcceptsAlreadyFetchedSnapshotRecords(t *testing.T) {
	ctx := context.Background()
	writer := &recordingWriter{}
	importer := NewImporter(writer, fakeMapper{})
	result, err := importer.ImportRecords(ctx, []SnapshotRecord{{
		SnapshotKey:      "snapshot:custom:ticket:1",
		Source:           "custom",
		SourceInstance:   "example/project",
		SourceObjectType: "custom_ticket",
		SourceObjectID:   "TICKET-1",
		SourceURL:        "https://example.test/tickets/1",
		BodySHA256:       "sha256:body",
		BodyRef:          "sha256/body",
		Body:             []byte(`{"id":"TICKET-1"}`),
	}}, ImportConfig{
		Now:          time.Date(2026, 6, 8, 12, 0, 0, 0, time.UTC),
		RunKeyPrefix: "run:live-custom",
		Slice:        "custom-workstream",
	})
	if err != nil {
		t.Fatalf("import fetched records: %v", err)
	}
	if result.RunsCompleted != 1 || result.ObjectsUpserted != 1 {
		t.Fatalf("unexpected import result: %#v", result)
	}
	if writer.snapshot.BodyRef != "sha256/body" {
		t.Fatalf("body ref should come from fetched record: %#v", writer.snapshot)
	}
}

func TestMaterializeBodiesWritesFetchedRecordBodies(t *testing.T) {
	ctx := context.Background()
	store := snapshotstore.NewStore(t.TempDir())
	body := []byte(`{"id":"TICKET-1"}`)
	sum := sha256.Sum256(body)
	bodySHA256 := "sha256:" + hex.EncodeToString(sum[:])

	records, err := MaterializeBodies(ctx, []SnapshotRecord{{
		SnapshotKey:      "snapshot:custom:ticket:1",
		Source:           "custom",
		SourceInstance:   "example/project",
		SourceObjectType: "custom_ticket",
		SourceObjectID:   "TICKET-1",
		SourceURL:        "https://example.test/tickets/1",
		BodySHA256:       bodySHA256,
		Body:             body,
	}}, store)
	if err != nil {
		t.Fatalf("materialize bodies: %v", err)
	}
	if len(records) != 1 {
		t.Fatalf("records len = %d", len(records))
	}
	if records[0].BodySHA256 != bodySHA256 || records[0].BodyRef == "" {
		t.Fatalf("body metadata not materialized: %#v", records[0])
	}
	replayed, err := store.Read(ctx, records[0].BodyRef)
	if err != nil {
		t.Fatalf("read materialized body: %v", err)
	}
	if !bytes.Equal(replayed, body) {
		t.Fatalf("replayed body = %q", string(replayed))
	}
}

func TestMaterializeBodiesRejectsHashMismatch(t *testing.T) {
	_, err := MaterializeBodies(context.Background(), []SnapshotRecord{{
		SnapshotKey:      "snapshot:custom:ticket:1",
		Source:           "custom",
		SourceInstance:   "example/project",
		SourceObjectType: "custom_ticket",
		SourceObjectID:   "TICKET-1",
		BodySHA256:       "sha256:wrong",
		Body:             []byte(`{"id":"TICKET-1"}`),
	}}, snapshotstore.NewStore(t.TempDir()))
	if err == nil {
		t.Fatal("expected hash mismatch error")
	}
}

func TestMaterializeBodiesRejectsMissingBodyWhenMetadataIncomplete(t *testing.T) {
	_, err := MaterializeBodies(context.Background(), []SnapshotRecord{{
		SnapshotKey:      "snapshot:custom:ticket:1",
		Source:           "custom",
		SourceInstance:   "example/project",
		SourceObjectType: "custom_ticket",
		SourceObjectID:   "TICKET-1",
	}}, snapshotstore.NewStore(t.TempDir()))
	if err == nil {
		t.Fatal("expected missing body error")
	}
}

type fakeMapper struct{}

func (fakeMapper) Map(records []SnapshotRecord, opts MapOptions) ([]domain.IngestBatch, error) {
	return []domain.IngestBatch{{
		RunKey:         opts.RunKeyPrefix + ":custom",
		Source:         records[0].Source,
		SourceInstance: records[0].SourceInstance,
		Slice:          opts.Slice,
		MapperVersion:  "custom-fixture/v1",
		SnapshotKeys:   []string{records[0].SnapshotKey},
		ObservedAt:     opts.ObservedAt,
		Objects: []domain.Object{{
			ObjectType:  ontology.ObjectTicket,
			Key:         "ticket:TICKET-1",
			Title:       "TICKET-1",
			SnapshotKey: records[0].SnapshotKey,
		}},
		Evidence: []domain.Evidence{{
			EvidenceKey: "evidence:custom:TICKET-1",
			SnapshotKey: records[0].SnapshotKey,
			TextHash:    "sha256:evidence",
		}},
		Events: []domain.SourceEvent{{
			EventKey:  "event:custom:TICKET-1",
			EventType: "snapshot_observed",
		}},
	}}, nil
}

type recordingWriter struct {
	runs     map[string]domain.IngestRun
	snapshot domain.SourceSnapshotWrite
	batch    domain.IngestBatch
}

func (w *recordingWriter) BeginIngestRun(_ context.Context, start domain.IngestRunStart) (domain.IngestRun, error) {
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
	return run, nil
}

func (w *recordingWriter) WriteSnapshot(_ context.Context, snapshot domain.SourceSnapshotWrite) (domain.SourceSnapshot, error) {
	w.snapshot = snapshot
	return domain.SourceSnapshot{SourceSnapshotWrite: snapshot, CreatedAt: snapshot.FetchedAt}, nil
}

func (w *recordingWriter) WriteMappedBatch(_ context.Context, batch domain.IngestBatch) (domain.IngestBatchResult, error) {
	w.batch = batch
	return domain.IngestBatchResult{
		RunKey:               batch.RunKey,
		ObjectsUpserted:      len(batch.Objects),
		AssociationsUpserted: len(batch.Associations),
		EvidenceUpserted:     len(batch.Evidence),
		EventsUpserted:       len(batch.Events),
	}, nil
}

func (w *recordingWriter) CompleteIngestRun(_ context.Context, complete domain.IngestRunComplete) (domain.IngestRun, error) {
	run := w.runs[complete.RunKey]
	run.Status = complete.Status
	return run, nil
}

func (w *recordingWriter) GetIngestRun(_ context.Context, runKey string) (domain.IngestRun, error) {
	return w.runs[runKey], nil
}

func (w *recordingWriter) ListSourceStatus(context.Context) ([]domain.SourceStatus, error) {
	return nil, nil
}
