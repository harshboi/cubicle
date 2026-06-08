package main

import (
	"context"
	"errors"
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
	"cubicle/services/ontology-service/internal/graphstore"
	"cubicle/services/ontology-service/internal/httpapi"
	"cubicle/services/ontology-service/internal/ontology"
)

func TestParseServeConfigDefaultsToLocalhost(t *testing.T) {
	cfg, err := parseServeConfigWithEnv([]string{"serve"}, func(string) string { return "" })
	if err != nil {
		t.Fatalf("parse serve config: %v", err)
	}
	if cfg.Listen != "127.0.0.1:48080" {
		t.Fatalf("unexpected listen address: %s", cfg.Listen)
	}
	if cfg.SeedFakeFlinkWorkstream {
		t.Fatal("SeedFakeFlinkWorkstream = true, want false")
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
dev_seed.fake_flink_workstream = true
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
	if !cfg.SeedFakeFlinkWorkstream {
		t.Fatal("SeedFakeFlinkWorkstream = false, want true")
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
		"--dev-seed-fake-flink-workstream",
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
	if cfg.OpenAPIServerURL != "http://flag.local:49400" {
		t.Fatalf("OpenAPIServerURL = %q", cfg.OpenAPIServerURL)
	}
	if cfg.SQLiteBusyTimeout != 1500*time.Millisecond {
		t.Fatalf("SQLiteBusyTimeout = %s", cfg.SQLiteBusyTimeout)
	}
	if !cfg.SeedFakeFlinkWorkstream {
		t.Fatal("SeedFakeFlinkWorkstream = false, want true")
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

func TestOpenGraphStoreStartsEmpty(t *testing.T) {
	ctx := context.Background()
	expander, cleanup, err := openGraphStore(ctx, serveConfig{
		DatabasePath: filepath.Join(t.TempDir(), "graph.db"),
	})
	if err != nil {
		t.Fatalf("open graph store: %v", err)
	}
	t.Cleanup(cleanup)

	graph, err := expander.Expand(ctx, domain.ExpandRequest{
		Start:          domain.ObjectRef{ObjectType: ontology.ObjectWorkstream, Key: "workstream:flink-autoscaler"},
		Depth:          2,
		LimitPerObject: 10,
	})
	if !errors.Is(err, graphstore.ErrMissingObject) {
		t.Fatalf("expected missing object from empty graph, got graph=%#v err=%v", graph, err)
	}
}

func TestOpenGraphStoreSeedsFakeWorkstreamOnlyWhenEnabled(t *testing.T) {
	ctx := context.Background()
	expander, cleanup, err := openGraphStore(ctx, serveConfig{
		DatabasePath:            filepath.Join(t.TempDir(), "graph.db"),
		SeedFakeFlinkWorkstream: true,
	})
	if err != nil {
		t.Fatalf("open graph store: %v", err)
	}
	t.Cleanup(cleanup)

	graph, err := expander.Expand(ctx, domain.ExpandRequest{
		Start:          domain.ObjectRef{ObjectType: ontology.ObjectWorkstream, Key: "workstream:flink-autoscaler"},
		Depth:          2,
		LimitPerObject: 10,
	})
	if err != nil {
		t.Fatalf("expand fake seeded graph: %v", err)
	}
	if len(graph.Objects) == 0 || len(graph.Associations) == 0 {
		t.Fatalf("expected fake seeded graph, got %d objects and %d associations", len(graph.Objects), len(graph.Associations))
	}
}

func TestRunIngestFlinkFixtureImportsFixtureIntoSQLite(t *testing.T) {
	ctx := context.Background()
	dbPath := filepath.Join(t.TempDir(), "graph.db")
	fixtureDir := filepath.Join("..", "..", "internal", "flink", "testdata", "flink-fixture")
	snapshotRoot := filepath.Join(t.TempDir(), "snapshots")

	if err := run([]string{
		"ingest-flink-fixture",
		"--database", dbPath,
		"--fixture-dir", fixtureDir,
		"--snapshot-root", snapshotRoot,
	}, slog.New(slog.NewTextHandler(io.Discard, nil))); err != nil {
		t.Fatalf("run ingest-flink-fixture: %v", err)
	}

	expander, cleanup, err := openGraphStore(ctx, serveConfig{DatabasePath: dbPath})
	if err != nil {
		t.Fatalf("open imported graph: %v", err)
	}
	t.Cleanup(cleanup)
	graph, err := expander.Expand(ctx, domain.ExpandRequest{
		Start:          domain.ObjectRef{ObjectType: ontology.ObjectWorkstream, Key: "workstream:flink-autoscaler"},
		Depth:          3,
		LimitPerObject: 20,
	})
	if err != nil {
		t.Fatalf("expand imported graph: %v", err)
	}
	if len(graph.Objects) < 5 || len(graph.Associations) < 5 {
		t.Fatalf("expected Flink fixture graph, got %d objects and %d associations", len(graph.Objects), len(graph.Associations))
	}
}

func TestRunIngestFlinkFixtureCanWriteThroughHTTPIngestURL(t *testing.T) {
	ctx := context.Background()
	store, cleanup, err := openGraphStore(ctx, serveConfig{DatabasePath: filepath.Join(t.TempDir(), "graph.db")})
	if err != nil {
		t.Fatalf("open graph store: %v", err)
	}
	t.Cleanup(cleanup)
	server := httptest.NewServer(httpapi.NewRouter(store, slog.New(slog.NewTextHandler(io.Discard, nil))))
	t.Cleanup(server.Close)

	if err := run([]string{
		"ingest-flink-fixture",
		"--ingest-url", server.URL,
		"--fixture-dir", filepath.Join("..", "..", "internal", "flink", "testdata", "flink-fixture"),
		"--snapshot-root", filepath.Join(t.TempDir(), "snapshots"),
	}, slog.New(slog.NewTextHandler(io.Discard, nil))); err != nil {
		t.Fatalf("run ingest-flink-fixture over HTTP: %v", err)
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
		Start:            domain.ObjectRef{ObjectType: ontology.ObjectWorkstream, Key: "workstream:flink-autoscaler"},
		Depth:            3,
		LimitPerObject:   20,
		AssociationTypes: []domain.AssociationType{ontology.AssocImplementedBy, ontology.AssocChangesFile, ontology.AssocDiscussedIn, ontology.AssocContains},
	})
	if err != nil {
		t.Fatalf("expand imported graph: %v", err)
	}
	if len(graph.Objects) < 4 {
		t.Fatalf("expected HTTP-imported Flink fixture graph, got %#v", graph)
	}
	if flink.FlinkFixtureMapperVersion == "" {
		t.Fatal("fixture mapper version is empty")
	}
}
