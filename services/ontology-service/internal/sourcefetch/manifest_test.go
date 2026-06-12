package sourcefetch

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestWriteReadManifestSortsAndValidatesBodies(t *testing.T) {
	dir := t.TempDir()
	generatedAt := time.Date(2026, 6, 11, 12, 0, 0, 0, time.UTC)
	records := []SnapshotRecord{
		manifestRecord("snapshot:z", "github", "github_pull_request", "apache/flink#2", []byte(`{"number":2}`)),
		manifestRecord("snapshot:a", "jira", "jira_issue", "FLINK-1", []byte(`{"key":"FLINK-1"}`)),
	}

	if err := WriteManifest(dir, records, DumpOptions{GeneratedAt: generatedAt}); err != nil {
		t.Fatalf("WriteManifest returned error: %v", err)
	}

	manifestBody, err := os.ReadFile(filepath.Join(dir, "manifest.json"))
	if err != nil {
		t.Fatalf("read manifest: %v", err)
	}
	manifestText := string(manifestBody)
	if !strings.Contains(manifestText, `"version": 1`) {
		t.Fatalf("manifest missing version: %s", manifestText)
	}
	if strings.Index(manifestText, "snapshot:z") > strings.Index(manifestText, "snapshot:a") {
		t.Fatalf("manifest was not sorted by source identity: %s", manifestText)
	}

	loaded, err := ReadManifest(dir)
	if err != nil {
		t.Fatalf("ReadManifest returned error: %v", err)
	}
	if len(loaded) != 2 {
		t.Fatalf("loaded len = %d, want 2", len(loaded))
	}
	if string(loaded[0].Body) != `{"number":2}` || string(loaded[1].Body) != `{"key":"FLINK-1"}` {
		t.Fatalf("unexpected loaded order/bodies: %#v %#v", loaded[0], loaded[1])
	}
}

func TestReadManifestRejectsHashMismatch(t *testing.T) {
	dir := t.TempDir()
	record := manifestRecord("snapshot:a", "jira", "jira_issue", "FLINK-1", []byte("good"))
	if err := WriteManifest(dir, []SnapshotRecord{record}, DumpOptions{}); err != nil {
		t.Fatalf("WriteManifest returned error: %v", err)
	}
	loadedManifest, err := ReadManifest(dir)
	if err != nil {
		t.Fatalf("ReadManifest returned error: %v", err)
	}
	if err := os.WriteFile(filepath.Join(dir, loadedManifest[0].Path), []byte("bad"), 0o644); err != nil {
		t.Fatalf("corrupt body: %v", err)
	}

	if _, err := ReadManifest(dir); err == nil {
		t.Fatal("expected hash mismatch error")
	}
}

func TestReadCaptureManifestConvertsNDJSONRows(t *testing.T) {
	dir := t.TempDir()
	if err := os.MkdirAll(filepath.Join(dir, "github"), 0o755); err != nil {
		t.Fatalf("create fixture dir: %v", err)
	}
	body := []byte(`{"message":"rate limited"}`)
	bodyHash := HashBody(body)
	if err := os.WriteFile(filepath.Join(dir, "github", "pr-1127.json"), body, 0o644); err != nil {
		t.Fatalf("write fixture body: %v", err)
	}
	manifest := `{"path":"github/pr-1127.json","source":"github","source_object_type":"github_pull_request","source_object_id":"apache/flink-kubernetes-operator#1127","url":"https://api.github.test/repos/apache/flink-kubernetes-operator/pulls/1127","status_code":429,"body_sha256":"` + bodyHash + `","bytes":26}` + "\n"
	if err := os.WriteFile(filepath.Join(dir, "manifest.ndjson"), []byte(manifest), 0o644); err != nil {
		t.Fatalf("write fixture manifest: %v", err)
	}

	records, err := ReadCaptureManifest(dir, CaptureManifestOptions{
		SourceInstances: map[string]string{"github": "github.com/apache/flink-kubernetes-operator"},
	})
	if err != nil {
		t.Fatalf("ReadCaptureManifest returned error: %v", err)
	}
	if len(records) != 1 {
		t.Fatalf("records len = %d, want 1", len(records))
	}
	record := records[0]
	if record.SourceInstance != "github.com/apache/flink-kubernetes-operator" {
		t.Fatalf("source instance = %q", record.SourceInstance)
	}
	if record.Response.StatusCode != 429 {
		t.Fatalf("status = %d, want 429", record.Response.StatusCode)
	}
	if string(record.Body) != string(body) {
		t.Fatalf("body = %q", record.Body)
	}
	if record.SnapshotKey != "capture:github-pr-1127.json" {
		t.Fatalf("snapshot key = %q", record.SnapshotKey)
	}
}

func TestReadCaptureManifestRejectsHashMismatch(t *testing.T) {
	dir := t.TempDir()
	if err := os.MkdirAll(filepath.Join(dir, "jira"), 0o755); err != nil {
		t.Fatalf("create fixture dir: %v", err)
	}
	if err := os.WriteFile(filepath.Join(dir, "jira", "issue.json"), []byte("actual"), 0o644); err != nil {
		t.Fatalf("write fixture body: %v", err)
	}
	manifest := `[{"path":"jira/issue.json","source":"jira","source_object_type":"jira_issue","source_object_id":"FLINK-39743","url":"https://issues.test/FLINK-39743","status_code":200,"body_sha256":"sha256:bad","bytes":6}]`
	if err := os.WriteFile(filepath.Join(dir, "manifest.json"), []byte(manifest), 0o644); err != nil {
		t.Fatalf("write fixture manifest: %v", err)
	}

	if _, err := ReadCaptureManifest(dir, CaptureManifestOptions{}); err == nil {
		t.Fatal("expected hash mismatch error")
	}
}

func manifestRecord(snapshotKey string, sourceKey string, objectType string, objectID string, body []byte) SnapshotRecord {
	return SnapshotRecord{
		SnapshotKey:      snapshotKey,
		SourceKey:        sourceKey,
		SourceInstance:   "test-instance",
		SourceObjectType: objectType,
		SourceObjectID:   objectID,
		BodySHA256:       HashBody(body),
		Request: RequestMetadata{
			Method: "GET",
			URL:    "https://source.test/" + objectID,
		},
		Response: ResponseMetadata{
			StatusCode: 200,
			FetchedAt:  time.Date(2026, 6, 11, 12, 0, 0, 0, time.UTC),
		},
		Body: body,
	}
}
