package main

import (
	"context"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
	"time"

	"cubicle/services/ontology-service/internal/domain"
	"cubicle/services/ontology-service/internal/flink"
	"cubicle/services/ontology-service/internal/httpapi"
)

func TestParseServeConfigDefaultsToLocalhost(t *testing.T) {
	cfg, err := parseServeConfigWithEnv([]string{"serve"}, func(string) string { return "" })
	if err != nil {
		t.Fatalf("parse serve config: %v", err)
	}
	if cfg.Listen != "127.0.0.1:48080" {
		t.Fatalf("unexpected listen address: %s", cfg.Listen)
	}
}

func TestParseServeConfigUsesEnvironmentDefaults(t *testing.T) {
	env := map[string]string{
		"CUBICLE_ONTOLOGY_LISTEN_ADDR":   "127.0.0.1:49090",
		"CUBICLE_ONTOLOGY_DATABASE_PATH": "/tmp/cubicle-ontology/env.db",
	}
	cfg, err := parseServeConfigWithEnv([]string{"serve"}, func(key string) string {
		return env[key]
	})
	if err != nil {
		t.Fatalf("parse env serve config: %v", err)
	}
	if cfg.Listen != "127.0.0.1:49090" {
		t.Fatalf("unexpected listen address: %s", cfg.Listen)
	}
	if cfg.DatabasePath != "/tmp/cubicle-ontology/env.db" {
		t.Fatalf("unexpected database path: %s", cfg.DatabasePath)
	}
}

func TestParseServeConfigLoadsHOCONFile(t *testing.T) {
	path := filepath.Join(t.TempDir(), "ontology-service.conf")
	if err := os.WriteFile(path, []byte(`
server.listen_addr = "127.0.0.1:49300"
storage.database_path = "/tmp/cubicle-config-file.db"
fixtures.seed = false
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
	if cfg.SeedFixtures {
		t.Fatal("SeedFixtures = true, want false")
	}
}

func TestParseServeConfigLoadsRuntimeTuningFromHOCONFile(t *testing.T) {
	path := filepath.Join(t.TempDir(), "ontology-service.conf")
	if err := os.WriteFile(path, []byte(`
server {
  listen_addr = "127.0.0.1:49300"
  openapi_server_url = "http://ontology.local:49300"
}
storage {
  database_path = "/tmp/cubicle-runtime-command.db"
  sqlite_busy_timeout = 900ms
}
`), 0o644); err != nil {
		t.Fatalf("write config file: %v", err)
	}

	cfg, err := parseServeConfigWithEnv([]string{"serve", "--config", path}, func(string) string { return "" })
	if err != nil {
		t.Fatalf("parse config file: %v", err)
	}

	if cfg.OpenAPIServerURL != "http://ontology.local:49300" {
		t.Fatalf("OpenAPIServerURL = %q", cfg.OpenAPIServerURL)
	}
	if cfg.SQLiteBusyTimeout != 900*time.Millisecond {
		t.Fatalf("SQLiteBusyTimeout = %s", cfg.SQLiteBusyTimeout)
	}
}

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

func TestParseServeConfigFlagsOverrideHOCONFile(t *testing.T) {
	path := filepath.Join(t.TempDir(), "ontology-service.conf")
	if err := os.WriteFile(path, []byte(`
server.listen_addr = "127.0.0.1:49300"
storage.database_path = "/tmp/from-file.db"
fixtures.seed = false
`), 0o644); err != nil {
		t.Fatalf("write config file: %v", err)
	}

	cfg, err := parseServeConfigWithEnv([]string{
		"serve",
		"--config", path,
		"--listen", "127.0.0.1:49400",
		"--database", "/tmp/from-flag.db",
		"--openapi-server-url", "http://flag.local:49400",
		"--sqlite-busy-timeout", "1500ms",
		"--seed-fixtures=true",
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
	if !cfg.SeedFixtures {
		t.Fatal("SeedFixtures = false, want true")
	}
	if cfg.OpenAPIServerURL != "http://flag.local:49400" {
		t.Fatalf("OpenAPIServerURL = %q", cfg.OpenAPIServerURL)
	}
	if cfg.SQLiteBusyTimeout != 1500*time.Millisecond {
		t.Fatalf("SQLiteBusyTimeout = %s", cfg.SQLiteBusyTimeout)
	}
}

func TestParseServeConfigRejectsPublicBindWithoutFlag(t *testing.T) {
	_, err := parseServeConfig([]string{"serve", "--listen", "0.0.0.0:48080"})
	if err == nil {
		t.Fatal("expected public bind without --allow-public-bind to fail")
	}
}

func TestParseServeConfigAllowsPublicBindWithFlag(t *testing.T) {
	cfg, err := parseServeConfig([]string{"serve", "--listen", "0.0.0.0:48080", "--allow-public-bind"})
	if err != nil {
		t.Fatalf("parse public serve config: %v", err)
	}
	if cfg.Listen != "0.0.0.0:48080" {
		t.Fatalf("unexpected listen address: %s", cfg.Listen)
	}
}

func TestParseServeConfigDerivesOpenAPIURLFromListenFlag(t *testing.T) {
	cfg, err := parseServeConfigWithEnv([]string{
		"serve",
		"--listen", "127.0.0.1:49500",
	}, func(string) string { return "" })
	if err != nil {
		t.Fatalf("parse listen flag: %v", err)
	}
	if cfg.OpenAPIServerURL != "http://127.0.0.1:49500" {
		t.Fatalf("OpenAPIServerURL = %q", cfg.OpenAPIServerURL)
	}
}

func TestParseServeConfigAcceptsDatabasePath(t *testing.T) {
	cfg, err := parseServeConfig([]string{"serve", "--database", "/tmp/cubicle-ontology/graph.db"})
	if err != nil {
		t.Fatalf("parse database path: %v", err)
	}
	if cfg.DatabasePath != "/tmp/cubicle-ontology/graph.db" {
		t.Fatalf("unexpected database path: %s", cfg.DatabasePath)
	}
}

func TestParseServeConfigAllowsDisablingFixtureSeed(t *testing.T) {
	cfg, err := parseServeConfig([]string{"serve", "--seed-fixtures=false"})
	if err != nil {
		t.Fatalf("parse seed fixture flag: %v", err)
	}
	if cfg.SeedFixtures {
		t.Fatal("expected fixture seeding to be disabled")
	}
}

func TestOpenGraphStoreSeedsFixtureIntoSQLite(t *testing.T) {
	ctx := context.Background()
	expander, cleanup, err := openGraphStore(ctx, serveConfig{
		DatabasePath: filepath.Join(t.TempDir(), "graph.db"),
		SeedFixtures: true,
	})
	if err != nil {
		t.Fatalf("open graph store: %v", err)
	}
	t.Cleanup(cleanup)

	graph, err := expander.Expand(ctx, domain.ExpandRequest{
		Start:        domain.NodeRef{Kind: domain.KindWorkstream, Key: "workstream:flink-autoscaler"},
		Depth:        2,
		LimitPerNode: 10,
	})
	if err != nil {
		t.Fatalf("expand seeded graph: %v", err)
	}
	if len(graph.Nodes) == 0 || len(graph.Edges) == 0 {
		t.Fatalf("expected seeded graph, got %d nodes and %d edges", len(graph.Nodes), len(graph.Edges))
	}
}

func TestRunIngestFlinkImportsFixtureIntoSQLite(t *testing.T) {
	ctx := context.Background()
	dbPath := filepath.Join(t.TempDir(), "graph.db")
	fixtureDir := filepath.Join("..", "..", "internal", "flink", "testdata", "flink-fixture")
	snapshotRoot := filepath.Join(t.TempDir(), "snapshots")

	if err := run([]string{
		"ingest-flink",
		"--database", dbPath,
		"--fixture-dir", fixtureDir,
		"--snapshot-root", snapshotRoot,
	}, slog.New(slog.NewTextHandler(io.Discard, nil))); err != nil {
		t.Fatalf("run ingest-flink: %v", err)
	}

	expander, cleanup, err := openGraphStore(ctx, serveConfig{DatabasePath: dbPath})
	if err != nil {
		t.Fatalf("open imported graph: %v", err)
	}
	t.Cleanup(cleanup)
	graph, err := expander.Expand(ctx, domain.ExpandRequest{
		Start:        domain.NodeRef{Kind: domain.KindWorkstream, Key: "workstream:flink-autoscaler"},
		Depth:        3,
		LimitPerNode: 20,
	})
	if err != nil {
		t.Fatalf("expand imported graph: %v", err)
	}
	if len(graph.Nodes) < 5 || len(graph.Edges) < 5 {
		t.Fatalf("expected Flink fixture graph, got %d nodes and %d edges", len(graph.Nodes), len(graph.Edges))
	}
}

func TestRunIngestFlinkCanWriteThroughHTTPIngestURL(t *testing.T) {
	ctx := context.Background()
	store, cleanup, err := openGraphStore(ctx, serveConfig{DatabasePath: filepath.Join(t.TempDir(), "graph.db")})
	if err != nil {
		t.Fatalf("open graph store: %v", err)
	}
	t.Cleanup(cleanup)
	server := httptest.NewServer(httpapi.NewRouter(store, slog.New(slog.NewTextHandler(io.Discard, nil))))
	t.Cleanup(server.Close)

	if err := run([]string{
		"ingest-flink",
		"--ingest-url", server.URL,
		"--fixture-dir", filepath.Join("..", "..", "internal", "flink", "testdata", "flink-fixture"),
		"--snapshot-root", filepath.Join(t.TempDir(), "snapshots"),
	}, slog.New(slog.NewTextHandler(io.Discard, nil))); err != nil {
		t.Fatalf("run ingest-flink over HTTP: %v", err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, server.URL+"/v1/sources", nil)
	if err != nil {
		t.Fatalf("new sources request: %v", err)
	}
	resp, err := server.Client().Do(req)
	if err != nil {
		t.Fatalf("get sources: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("sources status = %d", resp.StatusCode)
	}

	graph, err := store.Expand(ctx, domain.ExpandRequest{
		Start:        domain.NodeRef{Kind: domain.KindWorkstream, Key: "workstream:flink-autoscaler"},
		Depth:        3,
		LimitPerNode: 20,
		Predicates:   []domain.Predicate{domain.PredicateImplementedBy, domain.PredicateChangesFile, domain.PredicateDiscussedIn, domain.PredicateContains},
	})
	if err != nil {
		t.Fatalf("expand imported graph: %v", err)
	}
	if len(graph.Nodes) < 4 {
		t.Fatalf("expected HTTP-imported Flink fixture graph, got %#v", graph)
	}
	if flink.FixtureMapperVersion == "" {
		t.Fatal("fixture mapper version is empty")
	}
}
