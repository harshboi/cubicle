package main

import (
	"context"
	"errors"
	"net/http"
	"os"
	"path/filepath"
	"testing"

	"cubicle/services/ontology-service/internal/domain"
	"cubicle/services/ontology-service/internal/graphstore"
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
