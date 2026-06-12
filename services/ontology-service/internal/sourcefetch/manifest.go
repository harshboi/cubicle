package sourcefetch

import (
	"bufio"
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"time"
)

const manifestVersion = 1

var unsafePathChars = regexp.MustCompile(`[^A-Za-z0-9._-]+`)

// Manifest is the on-disk replay envelope for a source fetch dump.
type Manifest struct {
	Version     int              `json:"version"`
	GeneratedAt time.Time        `json:"generated_at,omitempty"`
	Snapshots   []SnapshotRecord `json:"snapshots"`
}

// DumpOptions controls how snapshots are materialized into a fixture directory.
type DumpOptions struct {
	GeneratedAt time.Time // GeneratedAt is optional metadata for humans; pass a fixed value in tests.
}

// CaptureManifestOptions controls conversion from source-capture manifests.
type CaptureManifestOptions struct {
	SourceInstances map[string]string // SourceInstances maps manifest source keys to Cubicle source instances.
}

// WriteManifest writes a canonical manifest and one body file per snapshot.
func WriteManifest(dir string, records []SnapshotRecord, opts DumpOptions) error {
	if err := os.MkdirAll(filepath.Join(dir, "bodies"), 0o755); err != nil {
		return fmt.Errorf("create snapshot bodies dir: %w", err)
	}
	materialized := make([]SnapshotRecord, len(records))
	copy(materialized, records)
	sortSnapshots(materialized)

	for idx := range materialized {
		record := &materialized[idx]
		if record.SnapshotKey == "" {
			return fmt.Errorf("snapshot at index %d: snapshot key is required", idx)
		}
		if record.Body == nil {
			return fmt.Errorf("snapshot %s: body is required for dump", record.SnapshotKey)
		}
		bodySHA256 := HashBody(record.Body)
		if record.BodySHA256 != "" && record.BodySHA256 != bodySHA256 {
			return fmt.Errorf("snapshot %s: body hash mismatch: got %s want %s", record.SnapshotKey, bodySHA256, record.BodySHA256)
		}
		record.BodySHA256 = bodySHA256
		if record.Path == "" {
			record.Path = bodyPath(record.SnapshotKey, bodySHA256)
		}
		if err := os.WriteFile(filepath.Join(dir, record.Path), record.Body, 0o644); err != nil {
			return fmt.Errorf("write snapshot body %s: %w", record.Path, err)
		}
		record.Body = nil
	}

	manifest := Manifest{
		Version:     manifestVersion,
		GeneratedAt: opts.GeneratedAt,
		Snapshots:   materialized,
	}
	body, err := json.MarshalIndent(manifest, "", "  ")
	if err != nil {
		return fmt.Errorf("encode snapshot manifest: %w", err)
	}
	body = append(body, '\n')
	if err := os.WriteFile(filepath.Join(dir, "manifest.json"), body, 0o644); err != nil {
		return fmt.Errorf("write snapshot manifest: %w", err)
	}
	return nil
}

// ReadManifest reads a replay fixture, loads body files, and validates hashes.
func ReadManifest(dir string) ([]SnapshotRecord, error) {
	body, err := os.ReadFile(filepath.Join(dir, "manifest.json"))
	if err != nil {
		return nil, fmt.Errorf("read snapshot manifest: %w", err)
	}
	var manifest Manifest
	if err := json.Unmarshal(body, &manifest); err != nil {
		return nil, fmt.Errorf("decode snapshot manifest: %w", err)
	}
	if manifest.Version != manifestVersion {
		return nil, fmt.Errorf("unsupported snapshot manifest version %d", manifest.Version)
	}

	records := make([]SnapshotRecord, 0, len(manifest.Snapshots))
	for _, record := range manifest.Snapshots {
		if err := validateManifestRecord(record); err != nil {
			return nil, err
		}
		raw, err := os.ReadFile(filepath.Join(dir, record.Path))
		if err != nil {
			return nil, fmt.Errorf("read snapshot body %s: %w", record.Path, err)
		}
		bodySHA256 := HashBody(raw)
		if record.BodySHA256 != bodySHA256 {
			return nil, fmt.Errorf("snapshot %s: body hash mismatch: got %s want %s", record.SnapshotKey, bodySHA256, record.BodySHA256)
		}
		record.Body = raw
		records = append(records, record)
	}
	sortSnapshots(records)
	return records, nil
}

// ReadCaptureManifest converts a raw source-capture manifest into replay snapshots.
//
// This supports the Flink fixture format, where manifest rows describe body
// files captured from source endpoints. It preserves non-200 bodies so callers
// can turn 403/429/5xx responses into coverage evidence instead of product data.
func ReadCaptureManifest(dir string, opts CaptureManifestOptions) ([]SnapshotRecord, error) {
	entries, err := readCaptureManifestEntries(dir)
	if err != nil {
		return nil, err
	}
	records := make([]SnapshotRecord, 0, len(entries))
	for _, entry := range entries {
		if err := validateCaptureManifestEntry(entry); err != nil {
			return nil, err
		}
		raw, err := os.ReadFile(filepath.Join(dir, entry.Path))
		if err != nil {
			return nil, fmt.Errorf("read capture body %s: %w", entry.Path, err)
		}
		bodySHA256 := HashBody(raw)
		if entry.BodySHA256 != bodySHA256 {
			return nil, fmt.Errorf("capture snapshot %s: body hash mismatch: got %s want %s", entry.Path, bodySHA256, entry.BodySHA256)
		}
		sourceInstance := entry.Source
		if opts.SourceInstances != nil && opts.SourceInstances[entry.Source] != "" {
			sourceInstance = opts.SourceInstances[entry.Source]
		}
		records = append(records, SnapshotRecord{
			SnapshotKey:      captureSnapshotKey(entry.Path),
			SourceKey:        entry.Source,
			SourceInstance:   sourceInstance,
			SourceObjectType: entry.SourceObjectType,
			SourceObjectID:   entry.SourceObjectID,
			SourceURL:        entry.URL,
			Path:             entry.Path,
			BodySHA256:       entry.BodySHA256,
			Request: RequestMetadata{
				Method: http.MethodGet,
				URL:    entry.URL,
			},
			Response: ResponseMetadata{
				StatusCode: entry.StatusCode,
			},
			Body: raw,
		})
	}
	sortSnapshots(records)
	return records, nil
}

func validateManifestRecord(record SnapshotRecord) error {
	if record.SnapshotKey == "" {
		return fmt.Errorf("invalid snapshot manifest entry: snapshot key is required")
	}
	if record.SourceKey == "" || record.SourceInstance == "" || record.SourceObjectType == "" || record.SourceObjectID == "" {
		return fmt.Errorf("snapshot %s: source identity fields are required", record.SnapshotKey)
	}
	if record.Path == "" {
		return fmt.Errorf("snapshot %s: path is required", record.SnapshotKey)
	}
	if record.BodySHA256 == "" {
		return fmt.Errorf("snapshot %s: body_sha256 is required", record.SnapshotKey)
	}
	return nil
}

type captureManifestEntry struct {
	Path             string `json:"path"`
	Source           string `json:"source"`
	SourceObjectType string `json:"source_object_type"`
	SourceObjectID   string `json:"source_object_id"`
	URL              string `json:"url"`
	StatusCode       int    `json:"status_code"`
	BodySHA256       string `json:"body_sha256"`
	Bytes            int64  `json:"bytes"`
}

func readCaptureManifestEntries(dir string) ([]captureManifestEntry, error) {
	ndjsonPath := filepath.Join(dir, "manifest.ndjson")
	if body, err := os.ReadFile(ndjsonPath); err == nil {
		return decodeCaptureManifestNDJSON(ndjsonPath, body)
	} else if !os.IsNotExist(err) {
		return nil, fmt.Errorf("read capture manifest %s: %w", ndjsonPath, err)
	}

	jsonPath := filepath.Join(dir, "manifest.json")
	body, err := os.ReadFile(jsonPath)
	if err != nil {
		return nil, fmt.Errorf("read capture manifest: %w", err)
	}
	body = bytes.TrimSpace(body)
	if len(body) == 0 || body[0] != '[' {
		return nil, fmt.Errorf("capture manifest %s must be a JSON array or manifest.ndjson must exist", jsonPath)
	}
	var entries []captureManifestEntry
	if err := json.Unmarshal(body, &entries); err != nil {
		return nil, fmt.Errorf("decode capture manifest %s: %w", jsonPath, err)
	}
	return entries, nil
}

func decodeCaptureManifestNDJSON(name string, body []byte) ([]captureManifestEntry, error) {
	scanner := bufio.NewScanner(bytes.NewReader(body))
	scanner.Buffer(make([]byte, 0, 64*1024), 8*1024*1024)
	var entries []captureManifestEntry
	lineNumber := 0
	for scanner.Scan() {
		lineNumber++
		line := bytes.TrimSpace(scanner.Bytes())
		if len(line) == 0 {
			continue
		}
		var entry captureManifestEntry
		if err := json.Unmarshal(line, &entry); err != nil {
			return nil, fmt.Errorf("decode capture manifest %s line %d: %w", name, lineNumber, err)
		}
		entries = append(entries, entry)
	}
	if err := scanner.Err(); err != nil {
		return nil, fmt.Errorf("scan capture manifest %s: %w", name, err)
	}
	return entries, nil
}

func validateCaptureManifestEntry(entry captureManifestEntry) error {
	if entry.Path == "" {
		return fmt.Errorf("capture manifest entry: path is required")
	}
	if entry.Source == "" || entry.SourceObjectType == "" || entry.SourceObjectID == "" {
		return fmt.Errorf("capture manifest entry %s: source identity fields are required", entry.Path)
	}
	if entry.URL == "" {
		return fmt.Errorf("capture manifest entry %s: url is required", entry.Path)
	}
	if entry.BodySHA256 == "" {
		return fmt.Errorf("capture manifest entry %s: body_sha256 is required", entry.Path)
	}
	if entry.StatusCode <= 0 {
		return fmt.Errorf("capture manifest entry %s: status_code is required", entry.Path)
	}
	return nil
}

func sortSnapshots(records []SnapshotRecord) {
	sort.SliceStable(records, func(left, right int) bool {
		a := records[left]
		b := records[right]
		return strings.Join([]string{a.SourceKey, a.SourceInstance, a.SourceObjectType, a.SourceObjectID, a.SnapshotKey}, "\x00") <
			strings.Join([]string{b.SourceKey, b.SourceInstance, b.SourceObjectType, b.SourceObjectID, b.SnapshotKey}, "\x00")
	})
}

func bodyPath(snapshotKey string, bodySHA256 string) string {
	safeKey := unsafePathChars.ReplaceAllString(snapshotKey, "-")
	safeKey = strings.Trim(safeKey, "-")
	if safeKey == "" {
		safeKey = "snapshot"
	}
	hashPart := strings.TrimPrefix(bodySHA256, "sha256:")
	if len(hashPart) > 12 {
		hashPart = hashPart[:12]
	}
	return filepath.ToSlash(filepath.Join("bodies", safeKey+"-"+hashPart+".body"))
}

func captureSnapshotKey(path string) string {
	safePath := unsafePathChars.ReplaceAllString(filepath.ToSlash(path), "-")
	safePath = strings.Trim(safePath, "-")
	if safePath == "" {
		safePath = "snapshot"
	}
	return "capture:" + safePath
}
