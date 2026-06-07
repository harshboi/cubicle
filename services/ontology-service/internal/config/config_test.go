package config

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestLoadDefaults(t *testing.T) {
	cfg := Load(func(string) string { return "" })

	if cfg.ListenAddr != "127.0.0.1:48080" {
		t.Fatalf("ListenAddr = %q", cfg.ListenAddr)
	}
	if cfg.DataRoot != ".data" {
		t.Fatalf("DataRoot = %q", cfg.DataRoot)
	}
	if cfg.DatabasePath != filepath.Join(".data", "graph.db") {
		t.Fatalf("DatabasePath = %q", cfg.DatabasePath)
	}
	if cfg.OpenAPIServerURL != "http://127.0.0.1:48080" {
		t.Fatalf("OpenAPIServerURL = %q", cfg.OpenAPIServerURL)
	}
	if cfg.SQLiteBusyTimeout != 5*time.Second {
		t.Fatalf("SQLiteBusyTimeout = %s", cfg.SQLiteBusyTimeout)
	}
	if !cfg.SeedFixtures {
		t.Fatal("SeedFixtures = false, want true")
	}
}

func TestLoadEnvOverrides(t *testing.T) {
	env := map[string]string{
		"CUBICLE_ONTOLOGY_LISTEN_ADDR":         "127.0.0.1:49090",
		"CUBICLE_ONTOLOGY_OPENAPI_SERVER_URL":  "http://ontology.env:49090",
		"CUBICLE_ONTOLOGY_DATA_ROOT":           "/tmp/cubicle-ontology",
		"CUBICLE_ONTOLOGY_DATABASE_PATH":       "/tmp/cubicle-ontology/custom.db",
		"CUBICLE_ONTOLOGY_SQLITE_BUSY_TIMEOUT": "1200ms",
	}

	cfg := Load(func(key string) string { return env[key] })

	if cfg.ListenAddr != "127.0.0.1:49090" {
		t.Fatalf("ListenAddr = %q", cfg.ListenAddr)
	}
	if cfg.DataRoot != "/tmp/cubicle-ontology" {
		t.Fatalf("DataRoot = %q", cfg.DataRoot)
	}
	if cfg.DatabasePath != "/tmp/cubicle-ontology/custom.db" {
		t.Fatalf("DatabasePath = %q", cfg.DatabasePath)
	}
	if cfg.OpenAPIServerURL != "http://ontology.env:49090" {
		t.Fatalf("OpenAPIServerURL = %q", cfg.OpenAPIServerURL)
	}
	if cfg.SQLiteBusyTimeout != 1200*time.Millisecond {
		t.Fatalf("SQLiteBusyTimeout = %s", cfg.SQLiteBusyTimeout)
	}
}

func TestLoadWithOptionsReadsHOCONFile(t *testing.T) {
	path := filepath.Join(t.TempDir(), "ontology-service.conf")
	if err := os.WriteFile(path, []byte(`
server {
  listen_addr = "127.0.0.1:49100"
}
storage {
  data_root = "/tmp/cubicle-ontology-hocon"
  database_path = "/tmp/cubicle-ontology-hocon/custom.db"
}
fixtures {
  seed = false
}
`), 0o644); err != nil {
		t.Fatalf("write config file: %v", err)
	}

	cfg, err := LoadWithOptions(LoadOptions{
		ConfigPath: path,
		Getenv:     func(string) string { return "" },
	})
	if err != nil {
		t.Fatalf("load config file: %v", err)
	}

	if cfg.ListenAddr != "127.0.0.1:49100" {
		t.Fatalf("ListenAddr = %q", cfg.ListenAddr)
	}
	if cfg.DataRoot != "/tmp/cubicle-ontology-hocon" {
		t.Fatalf("DataRoot = %q", cfg.DataRoot)
	}
	if cfg.DatabasePath != "/tmp/cubicle-ontology-hocon/custom.db" {
		t.Fatalf("DatabasePath = %q", cfg.DatabasePath)
	}
	if cfg.SeedFixtures {
		t.Fatal("SeedFixtures = true, want false")
	}
}

func TestLoadWithOptionsLetsEnvOverrideHOCONFile(t *testing.T) {
	path := filepath.Join(t.TempDir(), "ontology-service.conf")
	if err := os.WriteFile(path, []byte(`
server.listen_addr = "127.0.0.1:49100"
storage.database_path = "/tmp/from-file.db"
fixtures.seed = false
`), 0o644); err != nil {
		t.Fatalf("write config file: %v", err)
	}
	env := map[string]string{
		"CUBICLE_ONTOLOGY_LISTEN_ADDR":   "127.0.0.1:49200",
		"CUBICLE_ONTOLOGY_DATABASE_PATH": "/tmp/from-env.db",
		"CUBICLE_ONTOLOGY_SEED_FIXTURES": "true",
	}

	cfg, err := LoadWithOptions(LoadOptions{
		ConfigPath: path,
		Getenv:     func(key string) string { return env[key] },
	})
	if err != nil {
		t.Fatalf("load config file: %v", err)
	}

	if cfg.ListenAddr != "127.0.0.1:49200" {
		t.Fatalf("ListenAddr = %q", cfg.ListenAddr)
	}
	if cfg.DatabasePath != "/tmp/from-env.db" {
		t.Fatalf("DatabasePath = %q", cfg.DatabasePath)
	}
	if !cfg.SeedFixtures {
		t.Fatal("SeedFixtures = false, want true")
	}
}

func TestLoadWithOptionsReadsRuntimeTuningFromHOCONFile(t *testing.T) {
	path := filepath.Join(t.TempDir(), "ontology-service.conf")
	if err := os.WriteFile(path, []byte(`
server {
  listen_addr = "127.0.0.1:49100"
  openapi_server_url = "http://ontology.local:49100"
}
storage {
  database_path = "/tmp/cubicle-runtime-config.db"
  sqlite_busy_timeout = 750ms
}
`), 0o644); err != nil {
		t.Fatalf("write config file: %v", err)
	}

	cfg, err := LoadWithOptions(LoadOptions{
		ConfigPath: path,
		Getenv:     func(string) string { return "" },
	})
	if err != nil {
		t.Fatalf("load config file: %v", err)
	}

	if cfg.OpenAPIServerURL != "http://ontology.local:49100" {
		t.Fatalf("OpenAPIServerURL = %q", cfg.OpenAPIServerURL)
	}
	if cfg.SQLiteBusyTimeout != 750*time.Millisecond {
		t.Fatalf("SQLiteBusyTimeout = %s", cfg.SQLiteBusyTimeout)
	}
}
