// Association:
//
//	CLI args/env/config -> command config
//	fixture manifest -> summary/load command -> JSON output
//	serve config -> HTTP server timeouts
//
// These tests keep the command layer honest before it reaches source replay or
// the local ontology service.
package main

import (
	"bytes"
	"context"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"cubicle/services/ontology-service/internal/flinkcubiclepoc/sourcecapture"
)

// TestParseServeConfigDefaultsToLocalhost keeps the default service bind local.
func TestParseServeConfigDefaultsToLocalhost(t *testing.T) {
	cfg, err := parseServeConfigWithEnv([]string{"serve"}, func(string) string { return "" })
	if err != nil {
		t.Fatalf("parse serve config: %v", err)
	}
	if cfg.Listen != "127.0.0.1:48080" {
		t.Fatalf("unexpected listen address: %s", cfg.Listen)
	}
	if cfg.DatabasePath != filepath.Join(".data", "graph.db") {
		t.Fatalf("unexpected database path: %s", cfg.DatabasePath)
	}
	if cfg.SQLiteBusyTimeout != 5*time.Second {
		t.Fatalf("SQLiteBusyTimeout = %s", cfg.SQLiteBusyTimeout)
	}
	if !cfg.GraphQLPlaygroundEnabled {
		t.Fatal("GraphQLPlaygroundEnabled = false, want true")
	}
}

// TestSummarizeFlinkFixtureReportsSourceAndStatusCounts checks fixture coverage before loading.
func TestSummarizeFlinkFixtureReportsSourceAndStatusCounts(t *testing.T) {
	dir := t.TempDir()
	if err := os.MkdirAll(filepath.Join(dir, "github"), 0o755); err != nil {
		t.Fatalf("create fixture dir: %v", err)
	}
	okBody := []byte(`{"ok":true}`)
	limitedBody := []byte(`{"message":"limited"}`)
	if err := os.WriteFile(filepath.Join(dir, "github", "ok.json"), okBody, 0o644); err != nil {
		t.Fatalf("write ok body: %v", err)
	}
	if err := os.WriteFile(filepath.Join(dir, "github", "limited.json"), limitedBody, 0o644); err != nil {
		t.Fatalf("write limited body: %v", err)
	}
	manifest := strings.Join([]string{
		`{"path":"github/ok.json","source":"github","source_object_type":"github_pull_request","source_object_id":"apache/flink-kubernetes-operator#1078","url":"https://api.github.test/pulls/1078","status_code":200,"body_sha256":"` + sourcecapture.HashBody(okBody) + `","bytes":11}`,
		`{"path":"github/limited.json","source":"github","source_object_type":"github_pull_request","source_object_id":"apache/flink-kubernetes-operator#1127","url":"https://api.github.test/pulls/1127","status_code":429,"body_sha256":"` + sourcecapture.HashBody(limitedBody) + `","bytes":21}`,
		"",
	}, "\n")
	if err := os.WriteFile(filepath.Join(dir, "manifest.ndjson"), []byte(manifest), 0o644); err != nil {
		t.Fatalf("write manifest: %v", err)
	}

	var out bytes.Buffer
	if err := summarizeFlinkFixture(flinkFixtureSummaryConfig{Dir: dir}, &out); err != nil {
		t.Fatalf("summarizeFlinkFixture returned error: %v", err)
	}
	got := out.String()
	for _, want := range []string{`"total": 2`, `"key": "github"`, `"count": 2`, `"key": "200"`, `"key": "429"`} {
		if !strings.Contains(got, want) {
			t.Fatalf("summary missing %s: %s", want, got)
		}
	}
}

// TestLoadFlinkFixtureReportsMaterializedCounts checks the load command's graph counters.
func TestLoadFlinkFixtureReportsMaterializedCounts(t *testing.T) {
	dir := t.TempDir()
	records := []testFixtureRecord{
		{
			Path:       "jira/issues/FLINK-2.json",
			Source:     "jira",
			ObjectType: "jira_issue",
			ObjectID:   "FLINK-2",
			Status:     200,
			Body: []byte(`{
			  "key":"FLINK-2",
			  "fields":{
			    "summary":"Tune autoscaler stabilization",
			    "description":"Stabilize scaling decisions.",
			    "status":{"name":"Closed"},
			    "priority":{"name":"Major"},
			    "updated":"2026-06-10T15:04:05.000+0000"
			  }
			}`),
		},
		{
			Path:       "jira/remote-links/FLINK-2.json",
			Source:     "jira",
			ObjectType: "jira_remote_links",
			ObjectID:   "FLINK-2",
			Status:     200,
			Body:       []byte(`[{"object":{"url":"https://github.com/apache/flink-kubernetes-operator/pull/20"}}]`),
		},
		{
			Path:       "github/pr-details/apache__flink-kubernetes-operator__20/pull.json",
			Source:     "github",
			ObjectType: "github_pull_request",
			ObjectID:   "apache/flink-kubernetes-operator#20",
			Status:     200,
			Body: []byte(`{
			  "html_url":"https://github.com/apache/flink-kubernetes-operator/pull/20",
			  "title":"[FLINK-2] Tune autoscaler stabilization",
			  "state":"closed",
			  "merged_at":"2026-06-10T16:00:00Z",
			  "updated_at":"2026-06-10T16:00:00Z",
			  "number":20,
			  "base":{"repo":{"full_name":"apache/flink-kubernetes-operator"}}
			}`),
		},
	}
	for _, objectType := range []string{
		"github_pull_request_files",
		"github_issue_comments",
		"github_pull_request_review_comments",
		"github_pull_request_reviews",
		"github_pull_request_commits",
	} {
		records = append(records, testFixtureRecord{
			Path:       "github/pr-details/apache__flink-kubernetes-operator__20/" + objectType + ".json",
			Source:     "github",
			ObjectType: objectType,
			ObjectID:   "apache/flink-kubernetes-operator#20",
			Status:     200,
			Body:       []byte(`[]`),
		})
	}
	records = append(records, testFixtureRecord{
		Path:       "github/pr-details/apache__flink-kubernetes-operator__21/pull.json",
		Source:     "github",
		ObjectType: "github_pull_request",
		ObjectID:   "apache/flink-kubernetes-operator#21",
		Status:     429,
		Body:       []byte(`{"message":"rate limited"}`),
	})
	writeTestFixture(t, dir, records)

	var out bytes.Buffer
	dbPath := filepath.Join(t.TempDir(), "graph.db")
	err := loadFlinkFixture(context.Background(), flinkFixtureLoadConfig{
		Dir:          dir,
		DatabasePath: dbPath,
		StreamKey:    "test-flink-load-command",
		RunKey:       "source-sync-run:test-flink-load-command",
	}, &out)
	if err != nil {
		t.Fatalf("loadFlinkFixture returned error: %v", err)
	}

	got := out.String()
	for _, want := range []string{
		`"records_seen": 9`,
		`"records_failed": 1`,
		`"complete_pull_request_bundles": 1`,
		`"tickets": 1`,
		`"pull_requests": 1`,
		`"ticket_pull_requests": 1`,
		`"evidence": 3`,
		`"sync_issues": 1`,
	} {
		if !strings.Contains(got, want) {
			t.Fatalf("load output missing %s: %s", want, got)
		}
	}
}

// TestParseServeConfigUsesEnvironmentDefaults checks env vars feed the serve runtime.
func TestParseServeConfigUsesEnvironmentDefaults(t *testing.T) {
	env := map[string]string{
		"CUBICLE_ONTOLOGY_LISTEN_ADDR":                "127.0.0.1:49090",
		"CUBICLE_ONTOLOGY_DATABASE_PATH":              "/tmp/cubicle-ontology/env.db",
		"CUBICLE_ONTOLOGY_SQLITE_BUSY_TIMEOUT":        "1200ms",
		"CUBICLE_ONTOLOGY_GRAPHQL_PLAYGROUND_ENABLED": "false",
	}

	cfg, err := parseServeConfigWithEnv([]string{"serve"}, func(key string) string { return env[key] })
	if err != nil {
		t.Fatalf("parse env serve config: %v", err)
	}

	if cfg.Listen != "127.0.0.1:49090" {
		t.Fatalf("unexpected listen address: %s", cfg.Listen)
	}
	if cfg.DatabasePath != "/tmp/cubicle-ontology/env.db" {
		t.Fatalf("unexpected database path: %s", cfg.DatabasePath)
	}
	if cfg.SQLiteBusyTimeout != 1200*time.Millisecond {
		t.Fatalf("SQLiteBusyTimeout = %s", cfg.SQLiteBusyTimeout)
	}
	if cfg.GraphQLPlaygroundEnabled {
		t.Fatal("GraphQLPlaygroundEnabled = true, want false")
	}
}

// TestParseServeConfigLoadsHOCONFile checks file-backed runtime defaults.
func TestParseServeConfigLoadsHOCONFile(t *testing.T) {
	path := filepath.Join(t.TempDir(), "ontology-service.conf")
	if err := os.WriteFile(path, []byte(`
server.listen_addr = "127.0.0.1:49300"
storage.database_path = "/tmp/cubicle-config-file.db"
graphql.playground_enabled = false
`), 0o644); err != nil {
		t.Fatalf("write config file: %v", err)
	}

	cfg, err := parseServeConfigWithEnv([]string{"serve", "--config", path}, func(string) string { return "" })
	if err != nil {
		t.Fatalf("parse config file: %v", err)
	}

	if cfg.ConfigPath != path {
		t.Fatalf("ConfigPath = %q", cfg.ConfigPath)
	}
	if cfg.Listen != "127.0.0.1:49300" {
		t.Fatalf("Listen = %q", cfg.Listen)
	}
	if cfg.DatabasePath != "/tmp/cubicle-config-file.db" {
		t.Fatalf("DatabasePath = %q", cfg.DatabasePath)
	}
	if cfg.GraphQLPlaygroundEnabled {
		t.Fatal("GraphQLPlaygroundEnabled = true, want false")
	}
}

// TestParseServeConfigLoadsHOCONPathFromEnv checks env can select the config file.
func TestParseServeConfigLoadsHOCONPathFromEnv(t *testing.T) {
	path := filepath.Join(t.TempDir(), "ontology-service.conf")
	if err := os.WriteFile(path, []byte(`
server.listen_addr = "127.0.0.1:49350"
storage.database_path = "/tmp/cubicle-config-env-file.db"
`), 0o644); err != nil {
		t.Fatalf("write config file: %v", err)
	}
	env := map[string]string{
		"CUBICLE_ONTOLOGY_CONFIG_PATH": path,
	}

	cfg, err := parseServeConfigWithEnv([]string{"serve"}, func(key string) string { return env[key] })
	if err != nil {
		t.Fatalf("parse config file from env: %v", err)
	}

	if cfg.ConfigPath != path {
		t.Fatalf("ConfigPath = %q", cfg.ConfigPath)
	}
	if cfg.Listen != "127.0.0.1:49350" {
		t.Fatalf("Listen = %q", cfg.Listen)
	}
	if cfg.DatabasePath != "/tmp/cubicle-config-env-file.db" {
		t.Fatalf("DatabasePath = %q", cfg.DatabasePath)
	}
}

// TestParseServeConfigFlagsOverrideHOCONFile keeps explicit CLI flags highest priority.
func TestParseServeConfigFlagsOverrideHOCONFile(t *testing.T) {
	path := filepath.Join(t.TempDir(), "ontology-service.conf")
	if err := os.WriteFile(path, []byte(`
server.listen_addr = "127.0.0.1:49300"
storage.database_path = "/tmp/from-file.db"
graphql.playground_enabled = true
`), 0o644); err != nil {
		t.Fatalf("write config file: %v", err)
	}

	cfg, err := parseServeConfigWithEnv([]string{
		"serve",
		"--config", path,
		"--listen", "127.0.0.1:49400",
		"--database", "/tmp/from-flag.db",
		"--sqlite-busy-timeout", "1500ms",
		"--graphql-playground=false",
	}, func(string) string { return "" })
	if err != nil {
		t.Fatalf("parse config file: %v", err)
	}

	if cfg.Listen != "127.0.0.1:49400" {
		t.Fatalf("Listen = %q", cfg.Listen)
	}
	if cfg.DatabasePath != "/tmp/from-flag.db" {
		t.Fatalf("DatabasePath = %q", cfg.DatabasePath)
	}
	if cfg.SQLiteBusyTimeout != 1500*time.Millisecond {
		t.Fatalf("SQLiteBusyTimeout = %s", cfg.SQLiteBusyTimeout)
	}
	if cfg.GraphQLPlaygroundEnabled {
		t.Fatal("GraphQLPlaygroundEnabled = true, want false")
	}
}

// TestParseServeConfigRejectsPublicBindWithoutFlag protects local-only default serving.
func TestParseServeConfigRejectsPublicBindWithoutFlag(t *testing.T) {
	_, err := parseServeConfigWithEnv([]string{"serve", "--listen", "0.0.0.0:48080"}, func(string) string { return "" })
	if err == nil {
		t.Fatal("expected public bind without --allow-public-bind to fail")
	}
}

// TestParseServeConfigAllowsPublicBindWithFlag keeps public bind opt-in and explicit.
func TestParseServeConfigAllowsPublicBindWithFlag(t *testing.T) {
	cfg, err := parseServeConfigWithEnv([]string{"serve", "--listen", "0.0.0.0:48080", "--allow-public-bind"}, func(string) string { return "" })
	if err != nil {
		t.Fatalf("parse public serve config: %v", err)
	}
	if cfg.Listen != "0.0.0.0:48080" {
		t.Fatalf("unexpected listen address: %s", cfg.Listen)
	}
}

// TestHTTPServerUsesTimeouts checks the service has fixed HTTP timeout bounds.
func TestHTTPServerUsesTimeouts(t *testing.T) {
	handler := http.HandlerFunc(func(http.ResponseWriter, *http.Request) {})
	server := newHTTPServer(serveConfig{Listen: "127.0.0.1:48080"}, handler)

	if server.ReadHeaderTimeout != serverReadHeaderTimeout {
		t.Fatalf("ReadHeaderTimeout = %s", server.ReadHeaderTimeout)
	}
	if server.ReadTimeout != serverReadTimeout {
		t.Fatalf("ReadTimeout = %s", server.ReadTimeout)
	}
	if server.WriteTimeout != serverWriteTimeout {
		t.Fatalf("WriteTimeout = %s", server.WriteTimeout)
	}
	if server.IdleTimeout != serverIdleTimeout {
		t.Fatalf("IdleTimeout = %s", server.IdleTimeout)
	}
}

// testFixtureRecord is one raw capture row plus body bytes for CLI fixture tests.
type testFixtureRecord struct {
	Path       string
	Source     string
	ObjectType string
	ObjectID   string
	Status     int
	Body       []byte
}

// writeTestFixture writes body files and manifest rows for command-level replay tests.
func writeTestFixture(t *testing.T, dir string, records []testFixtureRecord) {
	t.Helper()
	lines := make([]string, 0, len(records)+1)
	for _, record := range records {
		path := filepath.Join(dir, record.Path)
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			t.Fatalf("create fixture path: %v", err)
		}
		if err := os.WriteFile(path, record.Body, 0o644); err != nil {
			t.Fatalf("write fixture body: %v", err)
		}
		lines = append(lines, fmt.Sprintf(
			`{"path":%q,"source":%q,"source_object_type":%q,"source_object_id":%q,"url":%q,"status_code":%d,"body_sha256":%q,"bytes":%d}`,
			record.Path,
			record.Source,
			record.ObjectType,
			record.ObjectID,
			"https://example.test/"+record.ObjectType+"/"+record.ObjectID,
			record.Status,
			sourcecapture.HashBody(record.Body),
			len(record.Body),
		))
	}
	lines = append(lines, "")
	if err := os.WriteFile(filepath.Join(dir, "manifest.ndjson"), []byte(strings.Join(lines, "\n")), 0o644); err != nil {
		t.Fatalf("write fixture manifest: %v", err)
	}
}
