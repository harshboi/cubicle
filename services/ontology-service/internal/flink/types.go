package flink

import (
	"context"

	"cubicle/services/ontology-service/internal/graphstore"
	"cubicle/services/ontology-service/internal/ingestpipeline"
	snapshotstore "cubicle/services/ontology-service/internal/snapshot"
)

type SnapshotRecord = ingestpipeline.SnapshotRecord
type MapOptions = ingestpipeline.MapOptions
type FixtureImportConfig = ingestpipeline.FixtureImportConfig
type FixtureImportResult = ingestpipeline.FixtureImportResult

func NewFixtureImporter(writer graphstore.IngestWriter) ingestpipeline.Importer {
	return ingestpipeline.NewFixtureImporter(writer, Mapper{MapperVersion: FixtureMapperVersion})
}

func LoadFixtureSnapshots(ctx context.Context, fixtureDir string, store snapshotstore.Store) ([]SnapshotRecord, error) {
	return ingestpipeline.LoadFixtureSnapshots(ctx, fixtureDir, store)
}
