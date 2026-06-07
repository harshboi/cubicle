package config

import (
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
