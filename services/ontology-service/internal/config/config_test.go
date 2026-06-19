package config

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestLoadDefaults(t *testing.T) {
	cfg, err := LoadWithOptions(LoadOptions{
		Getenv: func(string) string { return "" },
	})
	if err != nil {
		t.Fatalf("load defaults: %v", err)
	}

	if cfg.ListenAddr != "127.0.0.1:48080" {
		t.Fatalf("ListenAddr = %q", cfg.ListenAddr)
	}
	if cfg.DataRoot != ".data" {
		t.Fatalf("DataRoot = %q", cfg.DataRoot)
	}
	if cfg.DatabasePath != filepath.Join(".data", "graph.db") {
		t.Fatalf("DatabasePath = %q", cfg.DatabasePath)
	}
	if cfg.SQLiteBusyTimeout != 5*time.Second {
		t.Fatalf("SQLiteBusyTimeout = %s", cfg.SQLiteBusyTimeout)
	}
	if !cfg.GraphQLPlaygroundEnabled {
		t.Fatal("GraphQLPlaygroundEnabled = false, want true")
	}
	if cfg.AllowPublicBind {
		t.Fatal("AllowPublicBind = true, want false")
	}
}

func TestLoadEnvOverrides(t *testing.T) {
	env := map[string]string{
		"CUBICLE_ONTOLOGY_LISTEN_ADDR":                "127.0.0.1:49090",
		"CUBICLE_ONTOLOGY_ALLOW_PUBLIC_BIND":          "true",
		"CUBICLE_ONTOLOGY_DATA_ROOT":                  "/tmp/cubicle-ontology",
		"CUBICLE_ONTOLOGY_DATABASE_PATH":              "/tmp/cubicle-ontology/custom.db",
		"CUBICLE_ONTOLOGY_SQLITE_BUSY_TIMEOUT":        "1500ms",
		"CUBICLE_ONTOLOGY_GRAPHQL_PLAYGROUND_ENABLED": "false",
	}

	cfg, err := LoadWithOptions(LoadOptions{
		Getenv: func(key string) string { return env[key] },
	})
	if err != nil {
		t.Fatalf("load env overrides: %v", err)
	}

	if cfg.ListenAddr != "127.0.0.1:49090" {
		t.Fatalf("ListenAddr = %q", cfg.ListenAddr)
	}
	if cfg.DataRoot != "/tmp/cubicle-ontology" {
		t.Fatalf("DataRoot = %q", cfg.DataRoot)
	}
	if cfg.DatabasePath != "/tmp/cubicle-ontology/custom.db" {
		t.Fatalf("DatabasePath = %q", cfg.DatabasePath)
	}
	if cfg.SQLiteBusyTimeout != 1500*time.Millisecond {
		t.Fatalf("SQLiteBusyTimeout = %s", cfg.SQLiteBusyTimeout)
	}
	if cfg.GraphQLPlaygroundEnabled {
		t.Fatal("GraphQLPlaygroundEnabled = true, want false")
	}
	if !cfg.AllowPublicBind {
		t.Fatal("AllowPublicBind = false, want true")
	}
}

func TestLoadWithOptionsReadsHOCONFile(t *testing.T) {
	path := filepath.Join(t.TempDir(), "ontology-service.conf")
	if err := os.WriteFile(path, []byte(`
server {
  listen_addr = "127.0.0.1:49100"
  allow_public_bind = true
}
storage {
  data_root = "/tmp/cubicle-ontology-hocon"
  database_path = "/tmp/cubicle-ontology-hocon/custom.db"
  sqlite_busy_timeout = 900ms
}
graphql {
  playground_enabled = false
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

	if cfg.ConfigPath != path {
		t.Fatalf("ConfigPath = %q", cfg.ConfigPath)
	}
	if cfg.ListenAddr != "127.0.0.1:49100" {
		t.Fatalf("ListenAddr = %q", cfg.ListenAddr)
	}
	if !cfg.AllowPublicBind {
		t.Fatal("AllowPublicBind = false, want true")
	}
	if cfg.DataRoot != "/tmp/cubicle-ontology-hocon" {
		t.Fatalf("DataRoot = %q", cfg.DataRoot)
	}
	if cfg.DatabasePath != "/tmp/cubicle-ontology-hocon/custom.db" {
		t.Fatalf("DatabasePath = %q", cfg.DatabasePath)
	}
	if cfg.SQLiteBusyTimeout != 900*time.Millisecond {
		t.Fatalf("SQLiteBusyTimeout = %s", cfg.SQLiteBusyTimeout)
	}
	if cfg.GraphQLPlaygroundEnabled {
		t.Fatal("GraphQLPlaygroundEnabled = true, want false")
	}
}

func TestLoadWithOptionsDerivesDatabasePathFromOverriddenDataRoot(t *testing.T) {
	path := filepath.Join(t.TempDir(), "ontology-service.conf")
	if err := os.WriteFile(path, []byte(`
storage.data_root = "/tmp/cubicle-root-only"
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

	if cfg.DataRoot != "/tmp/cubicle-root-only" {
		t.Fatalf("DataRoot = %q", cfg.DataRoot)
	}
	if cfg.DatabasePath != filepath.Join("/tmp/cubicle-root-only", "graph.db") {
		t.Fatalf("DatabasePath = %q", cfg.DatabasePath)
	}
}

func TestLoadWithOptionsUsesConfigPathFromEnv(t *testing.T) {
	path := filepath.Join(t.TempDir(), "ontology-service.conf")
	if err := os.WriteFile(path, []byte(`
server.listen_addr = "127.0.0.1:49150"
storage.database_path = "/tmp/from-env-config-path.db"
`), 0o644); err != nil {
		t.Fatalf("write config file: %v", err)
	}
	env := map[string]string{
		"CUBICLE_ONTOLOGY_CONFIG_PATH": path,
	}

	cfg, err := LoadWithOptions(LoadOptions{
		Getenv: func(key string) string { return env[key] },
	})
	if err != nil {
		t.Fatalf("load config file from env: %v", err)
	}

	if cfg.ConfigPath != path {
		t.Fatalf("ConfigPath = %q", cfg.ConfigPath)
	}
	if cfg.ListenAddr != "127.0.0.1:49150" {
		t.Fatalf("ListenAddr = %q", cfg.ListenAddr)
	}
	if cfg.DatabasePath != "/tmp/from-env-config-path.db" {
		t.Fatalf("DatabasePath = %q", cfg.DatabasePath)
	}
}

func TestLoadWithOptionsLetsEnvOverrideHOCONFile(t *testing.T) {
	path := filepath.Join(t.TempDir(), "ontology-service.conf")
	if err := os.WriteFile(path, []byte(`
server.listen_addr = "127.0.0.1:49100"
storage.database_path = "/tmp/from-file.db"
graphql.playground_enabled = true
`), 0o644); err != nil {
		t.Fatalf("write config file: %v", err)
	}
	env := map[string]string{
		"CUBICLE_ONTOLOGY_LISTEN_ADDR":                "127.0.0.1:49200",
		"CUBICLE_ONTOLOGY_DATABASE_PATH":              "/tmp/from-env.db",
		"CUBICLE_ONTOLOGY_GRAPHQL_PLAYGROUND_ENABLED": "false",
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
	if cfg.GraphQLPlaygroundEnabled {
		t.Fatal("GraphQLPlaygroundEnabled = true, want false")
	}
}

func TestLoadWithOptionsRejectsInvalidHOCONValues(t *testing.T) {
	tests := []struct {
		name string
		body string
	}{
		{
			name: "allow public bind",
			body: "server.allow_public_bind = not-bool",
		},
		{
			name: "sqlite busy timeout",
			body: "storage.sqlite_busy_timeout = not-duration",
		},
		{
			name: "graphql playground",
			body: "graphql.playground_enabled = not-bool",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			path := filepath.Join(t.TempDir(), "ontology-service.conf")
			if err := os.WriteFile(path, []byte(tt.body), 0o644); err != nil {
				t.Fatalf("write config file: %v", err)
			}

			_, err := LoadWithOptions(LoadOptions{
				ConfigPath: path,
				Getenv:     func(string) string { return "" },
			})
			if err == nil {
				t.Fatal("expected invalid HOCON value to fail")
			}
		})
	}
}

func TestLoadWithOptionsRejectsInvalidEnvValues(t *testing.T) {
	tests := []struct {
		name string
		env  map[string]string
	}{
		{
			name: "allow public bind",
			env:  map[string]string{"CUBICLE_ONTOLOGY_ALLOW_PUBLIC_BIND": "not-bool"},
		},
		{
			name: "sqlite busy timeout",
			env:  map[string]string{"CUBICLE_ONTOLOGY_SQLITE_BUSY_TIMEOUT": "not-duration"},
		},
		{
			name: "graphql playground",
			env:  map[string]string{"CUBICLE_ONTOLOGY_GRAPHQL_PLAYGROUND_ENABLED": "not-bool"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := LoadWithOptions(LoadOptions{
				Getenv: func(key string) string { return tt.env[key] },
			})
			if err == nil {
				t.Fatal("expected invalid env value to fail")
			}
		})
	}
}
