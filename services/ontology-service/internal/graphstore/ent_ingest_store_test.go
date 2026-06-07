package graphstore

import (
	"context"
	"errors"
	"testing"
	"time"

	"cubicle/services/ontology-service/internal/domain"
	"cubicle/services/ontology-service/internal/ontology"
)

func TestEntStoreIngestRunSnapshotBatchAndComplete(t *testing.T) {
	ctx := context.Background()
	client := openEntClient(t, ctx)
	store := NewEntStore(client)
	observedAt := time.Date(2026, 6, 7, 14, 0, 0, 0, time.UTC)

	run, err := store.BeginIngestRun(ctx, domain.IngestRunStart{
		RunKey:         "run:flink:fixture:1",
		Source:         "jira",
		SourceInstance: "apache-jira",
		Slice:          "flink-autoscaler",
		MapperVersion:  "flink-fixture/v1",
		StartedAt:      observedAt,
	})
	if err != nil {
		t.Fatalf("begin ingest run: %v", err)
	}
	if run.Status != domain.IngestRunOpen {
		t.Fatalf("run status = %q", run.Status)
	}

	snapshot, err := store.WriteSnapshot(ctx, domain.SourceSnapshotWrite{
		RunKey:           run.RunKey,
		Source:           run.Source,
		SourceInstance:   run.SourceInstance,
		SnapshotKey:      "snapshot:jira:FLINK-39743",
		SourceObjectType: "jira_issue",
		SourceObjectID:   "FLINK-39743",
		BodySHA256:       "sha256:issue-body",
		BodyRef:          "snapshots/sha256/issue-body.json",
		SourceURL:        "https://issues.apache.org/jira/browse/FLINK-39743",
		FetchedAt:        observedAt,
	})
	if err != nil {
		t.Fatalf("write snapshot: %v", err)
	}
	if snapshot.CreatedAt.IsZero() {
		t.Fatal("snapshot created_at was not set")
	}

	result, err := store.WriteMappedBatch(ctx, domain.IngestBatch{
		RunKey:         run.RunKey,
		Source:         run.Source,
		SourceInstance: run.SourceInstance,
		Slice:          run.Slice,
		MapperVersion:  run.MapperVersion,
		SnapshotKeys:   []string{snapshot.SnapshotKey},
		ObservedAt:     observedAt,
		Objects: []domain.Object{{
			ObjectType: ontology.ObjectWorkstream,
			Key:        "workstream:flink-autoscaler",
			Title:      "Flink Autoscaler",
		}, {
			ObjectType:  ontology.ObjectTicket,
			Key:         "ticket:FLINK-39743",
			Title:       "Autoscaler bug",
			ExternalID:  "FLINK-39743",
			SourceURL:   "https://issues.apache.org/jira/browse/FLINK-39743",
			SnapshotKey: snapshot.SnapshotKey,
		}},
		Evidence: []domain.Evidence{{
			EvidenceKey: "evidence:jira:FLINK-39743",
			SnapshotKey: snapshot.SnapshotKey,
			SourceURL:   "https://issues.apache.org/jira/browse/FLINK-39743",
			TextHash:    "sha256:evidence-text",
			Summary:     "Ticket is in the autoscaler component.",
			Confidence:  1,
			ObservedAt:  observedAt,
		}},
		Associations: []domain.Association{{
			From:            domain.ObjectRef{ObjectType: ontology.ObjectWorkstream, Key: "workstream:flink-autoscaler"},
			To:              domain.ObjectRef{ObjectType: ontology.ObjectTicket, Key: "ticket:FLINK-39743"},
			AssociationType: ontology.AssocContains,
			Metadata: domain.AssociationMetadata{
				EvidenceKey: "evidence:jira:FLINK-39743",
				SnapshotKey: snapshot.SnapshotKey,
			},
		}},
		Checkpoint: &domain.SourceCheckpointWrite{
			CheckpointKey:   "jira-search-start-at",
			CheckpointValue: "50",
			UpdatedAt:       observedAt,
		},
	})
	if err != nil {
		t.Fatalf("write mapped batch: %v", err)
	}
	if result.ObjectsUpserted != 2 || result.AssociationsUpserted != 1 || result.EvidenceUpserted != 1 || !result.CheckpointUpdated {
		t.Fatalf("unexpected batch result: %#v", result)
	}

	completed, err := store.CompleteIngestRun(ctx, domain.IngestRunComplete{
		RunKey:      run.RunKey,
		Status:      domain.IngestRunCompleted,
		CompletedAt: observedAt.Add(time.Minute),
	})
	if err != nil {
		t.Fatalf("complete ingest run: %v", err)
	}
	if completed.Status != domain.IngestRunCompleted {
		t.Fatalf("completed status = %q", completed.Status)
	}

	statuses, err := store.ListSourceStatus(ctx)
	if err != nil {
		t.Fatalf("list source status: %v", err)
	}
	if len(statuses) != 1 {
		t.Fatalf("source status count = %d: %#v", len(statuses), statuses)
	}
	status := statuses[0]
	if status.Source != "jira" || status.SourceInstance != "apache-jira" || status.Slice != "flink-autoscaler" {
		t.Fatalf("unexpected source identity: %#v", status)
	}
	if status.Status != domain.SourceStatusHealthy || status.LastSuccessfulRunKey != run.RunKey {
		t.Fatalf("unexpected source status: %#v", status)
	}
	if status.CountsByObjectType[ontology.ObjectTicket] != 1 {
		t.Fatalf("ticket count = %d in %#v", status.CountsByObjectType[ontology.ObjectTicket], status.CountsByObjectType)
	}
}

func TestEntStoreWriteSnapshotIdempotencyUsesBodyHash(t *testing.T) {
	ctx := context.Background()
	client := openEntClient(t, ctx)
	store := NewEntStore(client)
	start := startIngestRun(t, ctx, store)

	first, err := store.WriteSnapshot(ctx, domain.SourceSnapshotWrite{
		RunKey:         start.RunKey,
		Source:         start.Source,
		SourceInstance: start.SourceInstance,
		SnapshotKey:    "snapshot:jira:FLINK-39743",
		BodySHA256:     "sha256:same",
		BodyRef:        "snapshots/sha256/same.json",
	})
	if err != nil {
		t.Fatalf("write first snapshot: %v", err)
	}
	second, err := store.WriteSnapshot(ctx, domain.SourceSnapshotWrite{
		RunKey:         start.RunKey,
		Source:         start.Source,
		SourceInstance: start.SourceInstance,
		SnapshotKey:    first.SnapshotKey,
		BodySHA256:     first.BodySHA256,
		BodyRef:        first.BodyRef,
	})
	if err != nil {
		t.Fatalf("write idempotent snapshot: %v", err)
	}
	if !second.CreatedAt.Equal(first.CreatedAt) {
		t.Fatalf("idempotent snapshot should return existing row: first=%s second=%s", first.CreatedAt, second.CreatedAt)
	}

	_, err = store.WriteSnapshot(ctx, domain.SourceSnapshotWrite{
		RunKey:         start.RunKey,
		Source:         start.Source,
		SourceInstance: start.SourceInstance,
		SnapshotKey:    first.SnapshotKey,
		BodySHA256:     "sha256:different",
		BodyRef:        "snapshots/sha256/different.json",
	})
	if !errors.Is(err, ErrIngestConflict) {
		t.Fatalf("expected conflict for changed snapshot hash, got %v", err)
	}
}

func TestEntStoreWriteMappedBatchRollsBackOnMissingAssociationEndpoint(t *testing.T) {
	ctx := context.Background()
	client := openEntClient(t, ctx)
	store := NewEntStore(client)
	start := startIngestRun(t, ctx, store)
	snapshot := writeSnapshot(t, ctx, store, start)

	_, err := store.WriteMappedBatch(ctx, domain.IngestBatch{
		RunKey:         start.RunKey,
		Source:         start.Source,
		SourceInstance: start.SourceInstance,
		Slice:          start.Slice,
		MapperVersion:  start.MapperVersion,
		SnapshotKeys:   []string{snapshot.SnapshotKey},
		Objects: []domain.Object{{
			ObjectType: ontology.ObjectWorkstream,
			Key:        "workstream:flink-autoscaler",
			Title:      "Flink Autoscaler",
		}},
		Associations: []domain.Association{{
			From:            domain.ObjectRef{ObjectType: ontology.ObjectWorkstream, Key: "workstream:flink-autoscaler"},
			To:              domain.ObjectRef{ObjectType: ontology.ObjectTicket, Key: "ticket:FLINK-39743"},
			AssociationType: ontology.AssocContains,
			Metadata: domain.AssociationMetadata{
				EvidenceKey: "evidence:jira:FLINK-39743",
				SnapshotKey: snapshot.SnapshotKey,
			},
		}},
		Evidence: []domain.Evidence{{
			EvidenceKey: "evidence:jira:FLINK-39743",
			SnapshotKey: snapshot.SnapshotKey,
			TextHash:    "sha256:evidence",
		}},
	})
	if !errors.Is(err, ErrMissingObject) {
		t.Fatalf("expected missing object error, got %v", err)
	}

	_, err = store.Expand(ctx, domain.ExpandRequest{
		Start:          domain.ObjectRef{ObjectType: ontology.ObjectWorkstream, Key: "workstream:flink-autoscaler"},
		Depth:          1,
		LimitPerObject: 10,
	})
	if !errors.Is(err, ErrMissingObject) {
		t.Fatalf("expected rollback to remove partially inserted object, got %v", err)
	}
}

func TestEntStoreWriteMappedBatchRequiresOpenRunAndSnapshot(t *testing.T) {
	ctx := context.Background()
	client := openEntClient(t, ctx)
	store := NewEntStore(client)
	start := startIngestRun(t, ctx, store)

	_, err := store.WriteMappedBatch(ctx, domain.IngestBatch{
		RunKey:         start.RunKey,
		Source:         start.Source,
		SourceInstance: start.SourceInstance,
		SnapshotKeys:   []string{"snapshot:jira:missing"},
	})
	if !errors.Is(err, ErrSnapshotNotFound) {
		t.Fatalf("expected missing snapshot error, got %v", err)
	}

	if _, err := store.CompleteIngestRun(ctx, domain.IngestRunComplete{
		RunKey: start.RunKey,
		Status: domain.IngestRunCompleted,
	}); err != nil {
		t.Fatalf("complete run: %v", err)
	}

	_, err = store.WriteSnapshot(ctx, domain.SourceSnapshotWrite{
		RunKey:         start.RunKey,
		Source:         start.Source,
		SourceInstance: start.SourceInstance,
		SnapshotKey:    "snapshot:jira:late",
		BodySHA256:     "sha256:late",
		BodyRef:        "snapshots/sha256/late.json",
	})
	if !errors.Is(err, ErrRunNotOpen) {
		t.Fatalf("expected closed run error, got %v", err)
	}
}

func startIngestRun(t *testing.T, ctx context.Context, store *EntStore) domain.IngestRun {
	t.Helper()
	run, err := store.BeginIngestRun(ctx, domain.IngestRunStart{
		RunKey:         "run:flink:fixture:1",
		Source:         "jira",
		SourceInstance: "apache-jira",
		Slice:          "flink-autoscaler",
		MapperVersion:  "flink-fixture/v1",
		StartedAt:      time.Date(2026, 6, 7, 14, 0, 0, 0, time.UTC),
	})
	if err != nil {
		t.Fatalf("begin ingest run: %v", err)
	}
	return run
}

func writeSnapshot(t *testing.T, ctx context.Context, store *EntStore, run domain.IngestRun) domain.SourceSnapshot {
	t.Helper()
	snapshot, err := store.WriteSnapshot(ctx, domain.SourceSnapshotWrite{
		RunKey:         run.RunKey,
		Source:         run.Source,
		SourceInstance: run.SourceInstance,
		SnapshotKey:    "snapshot:jira:FLINK-39743",
		BodySHA256:     "sha256:issue-body",
		BodyRef:        "snapshots/sha256/issue-body.json",
	})
	if err != nil {
		t.Fatalf("write snapshot: %v", err)
	}
	return snapshot
}
