package config

import (
	"os"
	"path/filepath"
	"testing"
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
}

func TestLoadEnvOverrides(t *testing.T) {
	env := map[string]string{
		"CUBICLE_ONTOLOGY_LISTEN_ADDR":   "127.0.0.1:49090",
		"CUBICLE_ONTOLOGY_DATA_ROOT":     "/tmp/cubicle-ontology",
		"CUBICLE_ONTOLOGY_DATABASE_PATH": "/tmp/cubicle-ontology/custom.db",
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
}

func TestLoadWithOptionsLetsEnvOverrideHOCONFile(t *testing.T) {
	path := filepath.Join(t.TempDir(), "ontology-service.conf")
	if err := os.WriteFile(path, []byte(`
server.listen_addr = "127.0.0.1:49100"
storage.database_path = "/tmp/from-file.db"
`), 0o644); err != nil {
		t.Fatalf("write config file: %v", err)
	}
	env := map[string]string{
		"CUBICLE_ONTOLOGY_LISTEN_ADDR":   "127.0.0.1:49200",
		"CUBICLE_ONTOLOGY_DATABASE_PATH": "/tmp/from-env.db",
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
}
