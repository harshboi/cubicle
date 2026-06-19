// Association:
//
//	ontology-service serve -> entstore.Open -> Ent client -> ontology hooks
//	ontology hooks -> WorkLensWindow -> typed lens result rows
//
// The command wires the runtime path that makes Ent schema and hook invariants
// active before HTTP traffic reaches the ontology graph.
package main

import (
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"log/slog"
	"net"
	"net/http"
	"os"
	"sort"
	"strconv"
	"strings"
	"time"

	"cubicle/services/ontology-service/internal/config"
	"cubicle/services/ontology-service/internal/entstore"
	"cubicle/services/ontology-service/internal/flinkcubiclepoc/sourcecapture"
	"cubicle/services/ontology-service/internal/flinkcubiclepoc/sourcegraph"
	"cubicle/services/ontology-service/internal/httpapi"
)

// serveConfig is the runtime path: config/env/flags -> Ent store -> HTTP API.
type serveConfig struct {
	ConfigPath               string        // ConfigPath is the optional HOCON file path used to load runtime defaults.
	Listen                   string        // Listen is the host:port address the local HTTP server binds to.
	AllowPublicBind          bool          // AllowPublicBind permits non-localhost binds for explicit development use.
	DatabasePath             string        // DatabasePath is the SQLite file path used for local ontology storage.
	SQLiteBusyTimeout        time.Duration // SQLiteBusyTimeout is SQLite's local lock wait timeout.
	GraphQLPlaygroundEnabled bool          // GraphQLPlaygroundEnabled controls whether GET /playground is mounted.
}

// flinkFixtureSummaryConfig points at a source-capture fixture for coverage counts.
type flinkFixtureSummaryConfig struct {
	Dir string // Dir is a Flink source-capture fixture directory.
}

// flinkFixtureLoadConfig points at a fixture and database for graph materialization.
type flinkFixtureLoadConfig struct {
	Dir               string        // Dir is a Flink source-capture fixture directory.
	DatabasePath      string        // DatabasePath is the SQLite graph database to materialize into.
	SQLiteBusyTimeout time.Duration // SQLiteBusyTimeout is SQLite's local lock wait timeout.
	StreamKey         string        // StreamKey overrides the default fixture stream key.
	RunKey            string        // RunKey overrides the generated source sync run key.
}

// fixtureSummary reports replay coverage before product rows are written.
type fixtureSummary struct {
	Total    int             `json:"total"`
	Sources  []summaryBucket `json:"sources"`
	Statuses []summaryBucket `json:"statuses"`
}

// summaryBucket is one source or status count in a fixture summary.
type summaryBucket struct {
	Key   string `json:"key"`
	Count int    `json:"count"`
}

const (
	// serverReadHeaderTimeout bounds how long the server waits for request headers.
	serverReadHeaderTimeout = 5 * time.Second

	// serverReadTimeout bounds how long the server spends reading the full request.
	serverReadTimeout = 15 * time.Second

	// serverWriteTimeout bounds how long the server spends writing a response.
	serverWriteTimeout = 30 * time.Second

	// serverIdleTimeout bounds how long idle keep-alive connections stay open.
	serverIdleTimeout = 60 * time.Second
)

// main routes CLI failures through structured logs and process exit.
func main() {
	logger := slog.New(slog.NewTextHandler(os.Stderr, nil))
	if err := run(os.Args[1:], logger); err != nil {
		logger.Error("ontology_service_exit", "error", err)
		os.Exit(1)
	}
}

// run chooses the command path: serve, summarize fixture, or load fixture.
func run(args []string, logger *slog.Logger) error {
	if len(args) == 0 {
		return errors.New("expected command: serve")
	}

	switch args[0] {
	case "serve":
		cfg, err := parseServeConfig(args)
		if err != nil {
			return err
		}
		return serve(cfg, logger)
	case "flink-fixture-summary":
		cfg, err := parseFlinkFixtureSummaryConfig(args)
		if err != nil {
			return err
		}
		return summarizeFlinkFixture(cfg, os.Stdout)
	case "flink-fixture-load":
		cfg, err := parseFlinkFixtureLoadConfig(args)
		if err != nil {
			return err
		}
		return loadFlinkFixture(context.Background(), cfg, os.Stdout)
	default:
		return fmt.Errorf("unknown command: %s", args[0])
	}
}

// parseServeConfig loads serve config using the real process environment.
func parseServeConfig(args []string) (serveConfig, error) {
	return parseServeConfigWithEnv(args, os.Getenv)
}

// parseServeConfigWithEnv merges config file, env, and flags for the HTTP runtime.
func parseServeConfigWithEnv(args []string, getenv func(string) string) (serveConfig, error) {
	configPath, err := configPathFromArgs(args)
	if err != nil {
		return serveConfig{}, err
	}
	appCfg, err := config.LoadWithOptions(config.LoadOptions{
		ConfigPath: configPath,
		Getenv:     getenv,
	})
	if err != nil {
		return serveConfig{}, err
	}

	flags := flag.NewFlagSet("serve", flag.ContinueOnError)
	flags.SetOutput(os.Stderr)

	cfg := serveConfig{
		ConfigPath:               appCfg.ConfigPath,
		Listen:                   appCfg.ListenAddr,
		AllowPublicBind:          appCfg.AllowPublicBind,
		DatabasePath:             appCfg.DatabasePath,
		SQLiteBusyTimeout:        appCfg.SQLiteBusyTimeout,
		GraphQLPlaygroundEnabled: appCfg.GraphQLPlaygroundEnabled,
	}
	flags.StringVar(&cfg.ConfigPath, "config", cfg.ConfigPath, "HOCON config file path")
	flags.StringVar(&cfg.Listen, "listen", cfg.Listen, "host:port for the local HTTP server")
	flags.StringVar(&cfg.DatabasePath, "database", cfg.DatabasePath, "SQLite database path for ontology-service storage")
	flags.DurationVar(&cfg.SQLiteBusyTimeout, "sqlite-busy-timeout", cfg.SQLiteBusyTimeout, "SQLite busy timeout")
	flags.BoolVar(&cfg.GraphQLPlaygroundEnabled, "graphql-playground", cfg.GraphQLPlaygroundEnabled, "mount the local GraphQL playground")
	flags.BoolVar(&cfg.AllowPublicBind, "allow-public-bind", cfg.AllowPublicBind, "allow binding outside localhost for development")
	if err := flags.Parse(args[1:]); err != nil {
		return serveConfig{}, err
	}
	if err := validateListenAddress(cfg.Listen, cfg.AllowPublicBind); err != nil {
		return serveConfig{}, err
	}
	return cfg, nil
}

// configPathFromArgs finds --config before loading env-backed defaults.
func configPathFromArgs(args []string) (string, error) {
	for i := 1; i < len(args); i++ {
		arg := args[i]
		if arg == "--config" || arg == "-config" {
			if i+1 >= len(args) {
				return "", errors.New("missing value for --config")
			}
			return args[i+1], nil
		}
		if strings.HasPrefix(arg, "--config=") {
			return strings.TrimPrefix(arg, "--config="), nil
		}
		if strings.HasPrefix(arg, "-config=") {
			return strings.TrimPrefix(arg, "-config="), nil
		}
	}
	return "", nil
}

// parseFlinkFixtureSummaryConfig validates the fixture summary command inputs.
func parseFlinkFixtureSummaryConfig(args []string) (flinkFixtureSummaryConfig, error) {
	flags := flag.NewFlagSet("flink-fixture-summary", flag.ContinueOnError)
	flags.SetOutput(os.Stderr)
	var cfg flinkFixtureSummaryConfig
	flags.StringVar(&cfg.Dir, "dir", "", "Flink source-capture fixture directory")
	if err := flags.Parse(args[1:]); err != nil {
		return flinkFixtureSummaryConfig{}, err
	}
	if cfg.Dir == "" {
		return flinkFixtureSummaryConfig{}, errors.New("missing required --dir")
	}
	return cfg, nil
}

// parseFlinkFixtureLoadConfig validates the fixture load command inputs.
func parseFlinkFixtureLoadConfig(args []string) (flinkFixtureLoadConfig, error) {
	flags := flag.NewFlagSet("flink-fixture-load", flag.ContinueOnError)
	flags.SetOutput(os.Stderr)
	cfg := flinkFixtureLoadConfig{
		SQLiteBusyTimeout: 5 * time.Second,
	}
	flags.StringVar(&cfg.Dir, "dir", "", "Flink source-capture fixture directory")
	flags.StringVar(&cfg.DatabasePath, "database", "", "SQLite database path for ontology-service storage")
	flags.DurationVar(&cfg.SQLiteBusyTimeout, "sqlite-busy-timeout", cfg.SQLiteBusyTimeout, "SQLite busy timeout")
	flags.StringVar(&cfg.StreamKey, "stream-key", "", "fixture stream key")
	flags.StringVar(&cfg.RunKey, "run-key", "", "source sync run key")
	if err := flags.Parse(args[1:]); err != nil {
		return flinkFixtureLoadConfig{}, err
	}
	if cfg.Dir == "" {
		return flinkFixtureLoadConfig{}, errors.New("missing required --dir")
	}
	if cfg.DatabasePath == "" {
		return flinkFixtureLoadConfig{}, errors.New("missing required --database")
	}
	return cfg, nil
}

// summarizeFlinkFixture counts replay coverage without touching the product graph.
func summarizeFlinkFixture(cfg flinkFixtureSummaryConfig, writer io.Writer) error {
	records, err := sourcegraph.ReadFixtureManifest(cfg.Dir)
	if err != nil {
		return err
	}
	summary := fixtureSummary{
		Total: len(records),
		Sources: countBuckets(records, func(key recordStatusKey) string {
			return key.source
		}),
		Statuses: countBuckets(records, func(key recordStatusKey) string {
			return strconv.Itoa(key.status)
		}),
	}
	encoder := json.NewEncoder(writer)
	encoder.SetIndent("", "  ")
	return encoder.Encode(summary)
}

// loadFlinkFixture replays captured source bytes into typed ontology rows.
func loadFlinkFixture(ctx context.Context, cfg flinkFixtureLoadConfig, writer io.Writer) error {
	records, err := sourcegraph.ReadFixtureManifest(cfg.Dir)
	if err != nil {
		return err
	}
	graphStore, err := entstore.Open(ctx, entstore.Config{
		DatabasePath: cfg.DatabasePath,
		BusyTimeout:  cfg.SQLiteBusyTimeout,
	})
	if err != nil {
		return err
	}
	defer graphStore.Close()

	result, err := sourcegraph.LoadFixture(ctx, graphStore.Client(), records, sourcegraph.LoadOptions{
		StreamKey: cfg.StreamKey,
		RunKey:    cfg.RunKey,
	})
	if err != nil {
		return err
	}
	encoder := json.NewEncoder(writer)
	encoder.SetIndent("", "  ")
	return encoder.Encode(result)
}

// recordStatusKey is the source/status tuple used by fixture coverage buckets.
type recordStatusKey struct {
	source string
	status int
}

// countBuckets turns replay records into stable summary buckets.
func countBuckets(records []sourcecapture.Record, keyFunc func(recordStatusKey) string) []summaryBucket {
	counts := make(map[string]int)
	for _, record := range records {
		key := keyFunc(recordStatusKey{source: record.SourceKey, status: record.Response.StatusCode})
		if key == "" {
			continue
		}
		counts[key]++
	}
	buckets := make([]summaryBucket, 0, len(counts))
	for key, count := range counts {
		buckets = append(buckets, summaryBucket{Key: key, Count: count})
	}
	sort.Slice(buckets, func(i, j int) bool {
		return buckets[i].Key < buckets[j].Key
	})
	return buckets
}

// validateListenAddress keeps the local service private unless the caller opts out.
func validateListenAddress(listen string, allowPublicBind bool) error {
	host, _, err := net.SplitHostPort(listen)
	if err != nil {
		return fmt.Errorf("invalid listen address %q: %w", listen, err)
	}
	if allowPublicBind {
		return nil
	}
	if host != "127.0.0.1" && host != "localhost" && host != "::1" {
		return fmt.Errorf("refusing public bind %q without --allow-public-bind", listen)
	}
	return nil
}

// serve opens Ent, registers runtime invariants, and starts the HTTP API.
func serve(cfg serveConfig, logger *slog.Logger) error {
	if logger == nil {
		logger = slog.Default()
	}
	logger.Info(
		"ontology_service_config",
		"config_path", cfg.ConfigPath,
		"listen_addr", cfg.Listen,
		"database_path", cfg.DatabasePath,
		"sqlite_busy_timeout_ms", cfg.SQLiteBusyTimeout.Milliseconds(),
		"graphql_playground_enabled", cfg.GraphQLPlaygroundEnabled,
	)

	graphStore, err := entstore.Open(context.Background(), entstore.Config{
		DatabasePath: cfg.DatabasePath,
		BusyTimeout:  cfg.SQLiteBusyTimeout,
	})
	if err != nil {
		return err
	}
	defer graphStore.Close()
	logger.Info("ontology_ent_ready")

	router := httpapi.NewRouterWithOptions(logger, httpapi.RouterOptions{
		GraphQLPlaygroundEnabled: cfg.GraphQLPlaygroundEnabled,
	})
	server := newHTTPServer(cfg, router)

	logger.Info("ontology_service_listening", "url", "http://"+cfg.Listen)
	return server.ListenAndServe()
}

// newHTTPServer applies fixed HTTP timeouts around the ontology API.
func newHTTPServer(cfg serveConfig, router http.Handler) *http.Server {
	return &http.Server{
		Addr:              cfg.Listen,
		Handler:           router,
		ReadHeaderTimeout: serverReadHeaderTimeout,
		ReadTimeout:       serverReadTimeout,
		WriteTimeout:      serverWriteTimeout,
		IdleTimeout:       serverIdleTimeout,
	}
}
