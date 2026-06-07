package main

import (
	"context"
	"path/filepath"
	"testing"

	"cubicle/services/ontology-service/internal/domain"
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
