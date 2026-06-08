package ingestpipeline

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"time"

	snapshotstore "cubicle/services/ontology-service/internal/snapshot"
)

type SnapshotRecord struct {
	SnapshotKey      string    `json:"snapshot_key"`
	Source           string    `json:"source"`
	SourceInstance   string    `json:"source_instance"`
	SourceObjectKind string    `json:"source_object_kind"`
	SourceObjectID   string    `json:"source_object_id"`
	SourceURL        string    `json:"source_url"`
	Path             string    `json:"path"`
	BodySHA256       string    `json:"body_sha256"`
	BodyRef          string    `json:"body_ref"`
	FetchedAt        time.Time `json:"fetched_at,omitempty"`
	Body             []byte    `json:"-"`
}

type fixtureManifest struct {
	Snapshots []SnapshotRecord `json:"snapshots"`
}

func LoadFixtureSnapshots(ctx context.Context, fixtureDir string, store snapshotstore.Store) ([]SnapshotRecord, error) {
	body, err := os.ReadFile(filepath.Join(fixtureDir, "manifest.json"))
	if err != nil {
		return nil, fmt.Errorf("read fixture manifest: %w", err)
	}
	var manifest fixtureManifest
	if err := json.Unmarshal(body, &manifest); err != nil {
		return nil, fmt.Errorf("decode fixture manifest: %w", err)
	}
	records := make([]SnapshotRecord, 0, len(manifest.Snapshots))
	for _, record := range manifest.Snapshots {
		if record.SnapshotKey == "" || record.Source == "" || record.SourceInstance == "" || record.Path == "" {
			return nil, fmt.Errorf("invalid fixture manifest entry: %#v", record)
		}
		raw, err := os.ReadFile(filepath.Join(fixtureDir, record.Path))
		if err != nil {
			return nil, fmt.Errorf("read fixture body %s: %w", record.Path, err)
		}
		record.Body = raw
		records = append(records, record)
	}
	return MaterializeBodies(ctx, records, store)
}
