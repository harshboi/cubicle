package ingestpipeline

import (
	"context"
	"fmt"
	"sync/atomic"
	"time"

	"cubicle/services/ontology-service/internal/domain"
	"cubicle/services/ontology-service/internal/graphstore"
	snapshotstore "cubicle/services/ontology-service/internal/snapshot"
)

// Mapper converts source snapshots into one or more ontology ingest batches.
type Mapper interface {
	Map([]SnapshotRecord, MapOptions) ([]domain.IngestBatch, error)
}

// MapOptions carries run-level metadata that mapper implementations should stamp onto emitted facts.
type MapOptions struct {
	RunKeyPrefix string    // RunKeyPrefix is prepended to per-source run keys for idempotent grouped imports.
	ObservedAt   time.Time // ObservedAt is the logical timestamp used on mapped ontology facts.
	Slice        string    // Slice is the product/workstream partition the mapped facts belong to.
}

// FixtureImportConfig configures an offline fixture import from local snapshot files.
type FixtureImportConfig struct {
	FixtureDir   string    // FixtureDir is the directory containing manifest.json and payload files.
	SnapshotRoot string    // SnapshotRoot is the root directory for content-addressed snapshot body storage.
	Now          time.Time // Now overrides the import clock for deterministic tests and fixture replays.
	RunKeyPrefix string    // RunKeyPrefix overrides the default run-key prefix used by mapped batches.
	Slice        string    // Slice overrides the default workstream/product slice used by mapped batches.
}

// ImportConfig configures an import from already-fetched snapshot records.
type ImportConfig struct {
	Now          time.Time // Now overrides the import clock for deterministic tests and source replays.
	RunKeyPrefix string    // RunKeyPrefix overrides the generated run-key prefix.
	Slice        string    // Slice assigns mapped facts to a product/workstream partition.
}

// FixtureImportResult summarizes how many graph facts and source artifacts an import wrote.
type FixtureImportResult struct {
	RunsCompleted        int // RunsCompleted counts source-grouped ingest runs completed successfully.
	SnapshotsWritten     int // SnapshotsWritten counts raw source snapshots recorded for provenance.
	ObjectsUpserted      int // ObjectsUpserted counts ontology objects inserted or updated by mapped batches.
	AssociationsUpserted int // AssociationsUpserted counts ontology associations inserted or updated by mapped batches.
	EvidenceUpserted     int // EvidenceUpserted counts evidence records inserted or updated by mapped batches.
	EventsUpserted       int // EventsUpserted counts source event records inserted or updated by mapped batches.
}

// Importer coordinates source snapshots, mapper output, and graph writes.
type Importer struct {
	writer   graphstore.IngestWriter // writer persists source snapshots and mapped ontology facts.
	mapper   Mapper                  // mapper transforms source snapshots into ontology ingest batches.
	sequence *atomic.Uint64          // sequence keeps generated run keys unique within this process.
}

// NewImporter creates an importer for already-fetched source snapshot records.
func NewImporter(writer graphstore.IngestWriter, mapper Mapper) Importer {
	return Importer{
		writer:   writer,
		mapper:   mapper,
		sequence: &atomic.Uint64{},
	}
}

// NewFixtureImporter creates a source-neutral importer for offline fixture directories.
func NewFixtureImporter(writer graphstore.IngestWriter, mapper Mapper) Importer {
	return NewImporter(writer, mapper)
}

// Import loads fixture snapshots from disk and writes the mapped ontology facts.
func (i Importer) Import(ctx context.Context, cfg FixtureImportConfig) (FixtureImportResult, error) {
	records, err := LoadFixtureSnapshots(ctx, cfg.FixtureDir, snapshotstore.NewStore(cfg.SnapshotRoot))
	if err != nil {
		return FixtureImportResult{}, err
	}
	return i.ImportRecords(ctx, records, ImportConfig{
		Now:          cfg.Now,
		RunKeyPrefix: cfg.RunKeyPrefix,
		Slice:        cfg.Slice,
	})
}

// ImportRecords writes already-fetched snapshots and their mapped ontology facts.
func (i Importer) ImportRecords(ctx context.Context, records []SnapshotRecord, cfg ImportConfig) (FixtureImportResult, error) {
	now := cfg.Now
	if now.IsZero() {
		now = time.Now().UTC()
	}
	runKeyPrefix := cfg.RunKeyPrefix
	if runKeyPrefix == "" {
		runKeyPrefix = fmt.Sprintf("run:fixture:%s:%d", now.Format("20060102T150405Z"), i.sequence.Add(1))
	}
	batches, err := i.mapper.Map(records, MapOptions{
		RunKeyPrefix: runKeyPrefix,
		ObservedAt:   now,
		Slice:        cfg.Slice,
	})
	if err != nil {
		return FixtureImportResult{}, err
	}

	recordsByIdentity := groupRecords(records)
	var result FixtureImportResult
	for _, batch := range batches {
		run, err := i.writer.BeginIngestRun(ctx, domain.IngestRunStart{
			RunKey:         batch.RunKey,
			Source:         batch.Source,
			SourceInstance: batch.SourceInstance,
			Slice:          batch.Slice,
			MapperVersion:  batch.MapperVersion,
			StartedAt:      now,
		})
		if err != nil {
			return FixtureImportResult{}, err
		}
		for _, record := range recordsByIdentity[sourceIdentity{source: batch.Source, sourceInstance: batch.SourceInstance}] {
			fetchedAt := record.FetchedAt
			if fetchedAt.IsZero() {
				fetchedAt = now
			}
			if _, err := i.writer.WriteSnapshot(ctx, domain.SourceSnapshotWrite{
				RunKey:           run.RunKey,
				Source:           run.Source,
				SourceInstance:   run.SourceInstance,
				SnapshotKey:      record.SnapshotKey,
				SourceObjectType: record.SourceObjectType,
				SourceObjectID:   record.SourceObjectID,
				BodySHA256:       record.BodySHA256,
				BodyRef:          record.BodyRef,
				SourceURL:        record.SourceURL,
				FetchedAt:        fetchedAt,
			}); err != nil {
				return FixtureImportResult{}, err
			}
			result.SnapshotsWritten++
		}
		batchResult, err := i.writer.WriteMappedBatch(ctx, batch)
		if err != nil {
			return FixtureImportResult{}, err
		}
		result.ObjectsUpserted += batchResult.ObjectsUpserted
		result.AssociationsUpserted += batchResult.AssociationsUpserted
		result.EvidenceUpserted += batchResult.EvidenceUpserted
		result.EventsUpserted += batchResult.EventsUpserted
		if _, err := i.writer.CompleteIngestRun(ctx, domain.IngestRunComplete{
			RunKey:      run.RunKey,
			Status:      domain.IngestRunCompleted,
			CompletedAt: now,
		}); err != nil {
			return FixtureImportResult{}, err
		}
		result.RunsCompleted++
	}
	return result, nil
}

// sourceIdentity groups snapshot records by the source run that owns them.
type sourceIdentity struct {
	source         string // source is the source system name, such as jira or github.
	sourceInstance string // sourceInstance is the project, repo, or workspace inside the source.
}

// groupRecords indexes snapshots by source identity so each ingest run writes its own provenance.
func groupRecords(records []SnapshotRecord) map[sourceIdentity][]SnapshotRecord {
	groups := make(map[sourceIdentity][]SnapshotRecord)
	for _, record := range records {
		identity := sourceIdentity{source: record.Source, sourceInstance: record.SourceInstance}
		groups[identity] = append(groups[identity], record)
	}
	return groups
}
