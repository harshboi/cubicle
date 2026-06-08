package flink

import (
	"context"

	"cubicle/services/ontology-service/internal/graphstore"
	"cubicle/services/ontology-service/internal/ingestpipeline"
	snapshotstore "cubicle/services/ontology-service/internal/snapshot"
)

// SnapshotRecord is the canonical raw source snapshot shape consumed by the Flink fixture mapper.
type SnapshotRecord = ingestpipeline.SnapshotRecord

// MapOptions controls run identity and observed timestamps when mapping Flink fixture snapshots.
type MapOptions = ingestpipeline.MapOptions

// FlinkFixtureImportConfig configures the offline Flink fixture import command.
type FlinkFixtureImportConfig = ingestpipeline.FixtureImportConfig

// FlinkFixtureImportResult summarizes writes produced by the offline Flink fixture import.
type FlinkFixtureImportResult = ingestpipeline.FixtureImportResult

// NewFlinkFixtureImporter builds an importer that maps the offline Flink fixture into ontology ingest batches.
func NewFlinkFixtureImporter(writer graphstore.IngestWriter) ingestpipeline.Importer {
	return ingestpipeline.NewFixtureImporter(writer, FlinkFixtureMapper{MapperVersion: FlinkFixtureMapperVersion})
}

// LoadFlinkFixtureSnapshots loads and materializes the offline Flink fixture snapshot records.
func LoadFlinkFixtureSnapshots(ctx context.Context, fixtureDir string, store snapshotstore.Store) ([]SnapshotRecord, error) {
	return ingestpipeline.LoadFixtureSnapshots(ctx, fixtureDir, store)
}
