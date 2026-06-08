package ingestpipeline

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"

	snapshotstore "cubicle/services/ontology-service/internal/snapshot"
)

func MaterializeBodies(ctx context.Context, records []SnapshotRecord, store snapshotstore.Store) ([]SnapshotRecord, error) {
	materialized := make([]SnapshotRecord, len(records))
	copy(materialized, records)
	for idx := range materialized {
		record := &materialized[idx]
		if record.Body != nil {
			bodySHA256 := hashBody(record.Body)
			if record.BodySHA256 != "" && record.BodySHA256 != bodySHA256 {
				return nil, fmt.Errorf("materialize snapshot %s: body hash mismatch", record.SnapshotKey)
			}
			record.BodySHA256 = bodySHA256
		}
		if record.BodySHA256 != "" && record.BodyRef != "" {
			continue
		}
		if record.Body == nil {
			return nil, fmt.Errorf("materialize snapshot %s: body is required when body metadata is incomplete", record.SnapshotKey)
		}
		written, err := store.Write(ctx, record.SnapshotKey, record.Body)
		if err != nil {
			return nil, fmt.Errorf("materialize snapshot %s: %w", record.SnapshotKey, err)
		}
		record.BodySHA256 = written.BodySHA256
		record.BodyRef = written.BodyRef
	}
	return materialized, nil
}

func hashBody(body []byte) string {
	sum := sha256.Sum256(body)
	return "sha256:" + hex.EncodeToString(sum[:])
}
