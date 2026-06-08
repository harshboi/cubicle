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

type Mapper interface {
	Map([]SnapshotRecord, MapOptions) ([]domain.IngestBatch, error)
}

type MapOptions struct {
	RunKeyPrefix string
	ObservedAt   time.Time
	Slice        string
}

type FixtureImportConfig struct {
	FixtureDir   string
	SnapshotRoot string
	Now          time.Time
	RunKeyPrefix string
	Slice        string
}

type ImportConfig struct {
	Now          time.Time
	RunKeyPrefix string
	Slice        string
}

type FixtureImportResult struct {
	RunsCompleted    int
	SnapshotsWritten int
	NodesUpserted    int
	EdgesUpserted    int
	EvidenceUpserted int
	EventsUpserted   int
}

type Importer struct {
	writer   graphstore.IngestWriter
	mapper   Mapper
	sequence *atomic.Uint64
}

func NewImporter(writer graphstore.IngestWriter, mapper Mapper) Importer {
	return Importer{
		writer:   writer,
		mapper:   mapper,
		sequence: &atomic.Uint64{},
	}
}

func NewFixtureImporter(writer graphstore.IngestWriter, mapper Mapper) Importer {
	return NewImporter(writer, mapper)
}

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
				SourceObjectKind: record.SourceObjectKind,
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
		result.NodesUpserted += batchResult.NodesUpserted
		result.EdgesUpserted += batchResult.EdgesUpserted
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

type sourceIdentity struct {
	source         string
	sourceInstance string
}

func groupRecords(records []SnapshotRecord) map[sourceIdentity][]SnapshotRecord {
	groups := make(map[sourceIdentity][]SnapshotRecord)
	for _, record := range records {
		identity := sourceIdentity{source: record.Source, sourceInstance: record.SourceInstance}
		groups[identity] = append(groups[identity], record)
	}
	return groups
}
