package main

import (
	"net/http"
	"os"
	"path/filepath"
	"testing"
	"time"
)

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

func TestParseServeConfigRejectsPublicBindWithoutFlag(t *testing.T) {
	_, err := parseServeConfigWithEnv([]string{"serve", "--listen", "0.0.0.0:48080"}, func(string) string { return "" })
	if err == nil {
		t.Fatal("expected public bind without --allow-public-bind to fail")
	}
}

func TestParseServeConfigAllowsPublicBindWithFlag(t *testing.T) {
	cfg, err := parseServeConfigWithEnv([]string{"serve", "--listen", "0.0.0.0:48080", "--allow-public-bind"}, func(string) string { return "" })
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
