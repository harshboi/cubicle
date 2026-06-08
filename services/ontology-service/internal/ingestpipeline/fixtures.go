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

// SnapshotRecord is the raw source payload plus identity metadata that an importer can map into ontology facts.
type SnapshotRecord struct {
	SnapshotKey      string    `json:"snapshot_key"`         // SnapshotKey is the stable source snapshot identity for replay and idempotency.
	Source           string    `json:"source"`               // Source is the system name, such as jira, github, docs, or ponymail.
	SourceInstance   string    `json:"source_instance"`      // SourceInstance distinguishes one project, repo, or workspace within a source.
	SourceObjectType string    `json:"source_object_type"`   // SourceObjectType is the source-native payload type recorded with the snapshot.
	SourceObjectID   string    `json:"source_object_id"`     // SourceObjectID is the source-native object identifier for debugging and replay.
	SourceURL        string    `json:"source_url"`           // SourceURL is the human-readable source URL used as evidence provenance.
	Path             string    `json:"path"`                 // Path points to the fixture body file relative to the manifest directory.
	BodySHA256       string    `json:"body_sha256"`          // BodySHA256 is the content hash used to verify replayed snapshot bodies.
	BodyRef          string    `json:"body_ref"`             // BodyRef is the content-addressed location under the local snapshot store.
	FetchedAt        time.Time `json:"fetched_at,omitempty"` // FetchedAt records when the snapshot was observed from the source.
	Body             []byte    `json:"-"`                    // Body carries the raw payload in memory before it is materialized to BodyRef.
}

// fixtureManifest is the on-disk manifest shape for replayable offline fixture snapshots.
type fixtureManifest struct {
	Snapshots []SnapshotRecord `json:"snapshots"` // Snapshots lists every source payload in deterministic import order.
}

// LoadFixtureSnapshots reads a fixture manifest, loads each body, and stores bodies content-addressably.
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
