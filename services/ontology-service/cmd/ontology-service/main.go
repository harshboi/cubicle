package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"log/slog"
	"net"
	"net/http"
	"os"
	"strings"
	"time"

	"cubicle/services/ontology-service/ent"
	"cubicle/services/ontology-service/internal/config"
	"cubicle/services/ontology-service/internal/flink"
	"cubicle/services/ontology-service/internal/graphstore"
	"cubicle/services/ontology-service/internal/httpapi"
	"cubicle/services/ontology-service/internal/ingestclient"
	"cubicle/services/ontology-service/internal/sampledata"
	"cubicle/services/ontology-service/internal/storage"
	"entgo.io/ent/dialect"
	entsql "entgo.io/ent/dialect/sql"
)

type serveConfig struct {
	ConfigPath               string        // ConfigPath is the optional HOCON file path used to load runtime defaults.
	Listen                   string        // Listen is the host:port address the local HTTP server binds to.
	OpenAPIServerURL         string        // OpenAPIServerURL is the base URL advertised in generated OpenAPI metadata.
	OpenAPIServerURLExplicit bool          // OpenAPIServerURLExplicit tracks whether config or flags set the OpenAPI URL directly.
	DatabasePath             string        // DatabasePath is the SQLite file path used by the Ent-backed ontology store.
	SQLiteBusyTimeout        time.Duration // SQLiteBusyTimeout is the SQLite lock wait timeout configured for local persistence.
	SeedFakeFlinkWorkstream  bool          // SeedFakeFlinkWorkstream controls explicit dev-only fake workstream seeding.
	AllowPublicBind          bool          // AllowPublicBind permits non-localhost binds for explicit development use.
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

// ingestFlinkFixtureConfig is the command config for importing the offline Flink fixture dataset.
type ingestFlinkFixtureConfig struct {
	ConfigPath        string        // ConfigPath is the optional HOCON file path used to load importer defaults.
	DatabasePath      string        // DatabasePath is the SQLite file path used when importing directly into Ent.
	SQLiteBusyTimeout time.Duration // SQLiteBusyTimeout is the SQLite lock wait timeout for direct imports.
	IngestURL         string        // IngestURL is the optional ontology-service base URL for HTTP ingestion.
	FixtureDir        string        // FixtureDir is the local directory containing offline Flink source snapshots.
	SnapshotRoot      string        // SnapshotRoot is where content-addressed snapshot bodies are materialized.
}

func main() {
	logger := slog.New(slog.NewTextHandler(os.Stderr, nil))
	if err := run(os.Args[1:], logger); err != nil {
		logger.Error("ontology_service_exit", "error", err)
		os.Exit(1)
	}
}

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
	case "ingest-flink-fixture":
		cfg, err := parseIngestFlinkFixtureConfig(args)
		if err != nil {
			return err
		}
		return ingestFlinkFixture(context.Background(), cfg, logger)
	default:
		return fmt.Errorf("unknown command: %s", args[0])
	}
}

func parseServeConfig(args []string) (serveConfig, error) {
	return parseServeConfigWithEnv(args, os.Getenv)
}

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
		OpenAPIServerURL:         appCfg.OpenAPIServerURL,
		OpenAPIServerURLExplicit: appCfg.OpenAPIServerURLExplicit,
		DatabasePath:             appCfg.DatabasePath,
		SQLiteBusyTimeout:        appCfg.SQLiteBusyTimeout,
		SeedFakeFlinkWorkstream:  appCfg.SeedFakeFlinkWorkstream,
	}
	flags.StringVar(&cfg.ConfigPath, "config", cfg.ConfigPath, "HOCON config file path")
	flags.StringVar(&cfg.Listen, "listen", cfg.Listen, "host:port for the local HTTP server")
	flags.StringVar(&cfg.OpenAPIServerURL, "openapi-server-url", cfg.OpenAPIServerURL, "server URL advertised in OpenAPI")
	flags.StringVar(&cfg.DatabasePath, "database", cfg.DatabasePath, "SQLite database path for the ontology graph")
	flags.DurationVar(&cfg.SQLiteBusyTimeout, "sqlite-busy-timeout", cfg.SQLiteBusyTimeout, "SQLite busy timeout")
	flags.BoolVar(&cfg.SeedFakeFlinkWorkstream, "dev-seed-fake-flink-workstream", cfg.SeedFakeFlinkWorkstream, "seed the dev-only fake Flink workstream sample")
	flags.BoolVar(&cfg.AllowPublicBind, "allow-public-bind", false, "allow binding outside localhost for development")
	if err := flags.Parse(args[1:]); err != nil {
		return serveConfig{}, err
	}
	if flagWasSet(flags, "openapi-server-url") {
		cfg.OpenAPIServerURLExplicit = true
	}
	if flagWasSet(flags, "listen") && !cfg.OpenAPIServerURLExplicit {
		cfg.OpenAPIServerURL = "http://" + cfg.Listen
	}
	if err := validateListenAddress(cfg.Listen, cfg.AllowPublicBind); err != nil {
		return serveConfig{}, err
	}
	return cfg, nil
}

// parseIngestFlinkFixtureConfig parses CLI and config defaults for the offline Flink fixture importer.
func parseIngestFlinkFixtureConfig(args []string) (ingestFlinkFixtureConfig, error) {
	return parseIngestFlinkFixtureConfigWithEnv(args, os.Getenv)
}

// parseIngestFlinkFixtureConfigWithEnv parses fixture importer config with an injectable env reader for tests.
func parseIngestFlinkFixtureConfigWithEnv(args []string, getenv func(string) string) (ingestFlinkFixtureConfig, error) {
	configPath, err := configPathFromArgs(args)
	if err != nil {
		return ingestFlinkFixtureConfig{}, err
	}
	appCfg, err := config.LoadWithOptions(config.LoadOptions{
		ConfigPath: configPath,
		Getenv:     getenv,
	})
	if err != nil {
		return ingestFlinkFixtureConfig{}, err
	}

	flags := flag.NewFlagSet("ingest-flink-fixture", flag.ContinueOnError)
	flags.SetOutput(os.Stderr)
	cfg := ingestFlinkFixtureConfig{
		ConfigPath:        appCfg.ConfigPath,
		DatabasePath:      appCfg.DatabasePath,
		SQLiteBusyTimeout: appCfg.SQLiteBusyTimeout,
		FixtureDir:        "internal/flink/testdata/flink-fixture",
		SnapshotRoot:      ".data/snapshots",
	}
	flags.StringVar(&cfg.ConfigPath, "config", cfg.ConfigPath, "HOCON config file path")
	flags.StringVar(&cfg.DatabasePath, "database", cfg.DatabasePath, "SQLite database path for the ontology graph")
	flags.DurationVar(&cfg.SQLiteBusyTimeout, "sqlite-busy-timeout", cfg.SQLiteBusyTimeout, "SQLite busy timeout")
	flags.StringVar(&cfg.IngestURL, "ingest-url", "", "ontology service base URL for HTTP ingestion")
	flags.StringVar(&cfg.FixtureDir, "fixture-dir", cfg.FixtureDir, "offline Flink fixture snapshot directory")
	flags.StringVar(&cfg.SnapshotRoot, "snapshot-root", cfg.SnapshotRoot, "content-addressed snapshot body root")
	if err := flags.Parse(args[1:]); err != nil {
		return ingestFlinkFixtureConfig{}, err
	}
	return cfg, nil
}

func flagWasSet(flags *flag.FlagSet, name string) bool {
	found := false
	flags.Visit(func(f *flag.Flag) {
		if f.Name == name {
			found = true
		}
	})
	return found
}

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

func serve(cfg serveConfig, logger *slog.Logger) error {
	if logger == nil {
		logger = slog.Default()
	}
	logger.Info(
		"ontology_service_config",
		"config_path", cfg.ConfigPath,
		"listen_addr", cfg.Listen,
		"openapi_server_url", cfg.OpenAPIServerURL,
		"database_path", cfg.DatabasePath,
		"sqlite_busy_timeout_ms", cfg.SQLiteBusyTimeout.Milliseconds(),
		"seed_fake_flink_workstream", cfg.SeedFakeFlinkWorkstream,
	)

	graph, cleanup, err := openGraphStore(context.Background(), cfg)
	if err != nil {
		return err
	}
	defer cleanup()

	router := httpapi.NewRouterWithOptions(graph, logger, httpapi.RouterOptions{
		OpenAPIServerURL: cfg.OpenAPIServerURL,
	})
	server := newHTTPServer(cfg, router)

	logger.Info("ontology_service_listening", "url", "http://"+cfg.Listen)
	return server.ListenAndServe()
}

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

// ingestFlinkFixture imports the offline Flink fixture through HTTP or directly into the configured graph store.
func ingestFlinkFixture(ctx context.Context, cfg ingestFlinkFixtureConfig, logger *slog.Logger) error {
	if logger == nil {
		logger = slog.Default()
	}
	if cfg.IngestURL != "" {
		return ingestFlinkFixtureWithWriter(ctx, cfg, ingestclient.New(cfg.IngestURL, http.DefaultClient), logger)
	}
	graph, cleanup, err := openGraphStore(ctx, serveConfig{
		DatabasePath:      cfg.DatabasePath,
		SQLiteBusyTimeout: cfg.SQLiteBusyTimeout,
	})
	if err != nil {
		return err
	}
	defer cleanup()
	writer, ok := graph.(graphstore.IngestWriter)
	if !ok {
		return errors.New("graph store does not implement ingest writer")
	}
	return ingestFlinkFixtureWithWriter(ctx, cfg, writer, logger)
}

// ingestFlinkFixtureWithWriter maps the offline Flink fixture and writes the resulting ontology ingest batches.
func ingestFlinkFixtureWithWriter(ctx context.Context, cfg ingestFlinkFixtureConfig, writer graphstore.IngestWriter, logger *slog.Logger) error {
	result, err := flink.NewFlinkFixtureImporter(writer).Import(ctx, flink.FlinkFixtureImportConfig{
		FixtureDir:   cfg.FixtureDir,
		SnapshotRoot: cfg.SnapshotRoot,
	})
	if err != nil {
		return err
	}
	logger.Info(
		"flink_fixture_ingested",
		"runs_completed", result.RunsCompleted,
		"snapshots_written", result.SnapshotsWritten,
		"objects_upserted", result.ObjectsUpserted,
		"associations_upserted", result.AssociationsUpserted,
		"evidence_upserted", result.EvidenceUpserted,
		"events_upserted", result.EventsUpserted,
	)
	return nil
}

func openGraphStore(ctx context.Context, cfg serveConfig) (graphstore.Store, func(), error) {
	store, err := storage.Open(ctx, storage.Config{
		DatabasePath: cfg.DatabasePath,
		BusyTimeout:  cfg.SQLiteBusyTimeout,
	})
	if err != nil {
		return nil, nil, err
	}

	client := ent.NewClient(ent.Driver(entsql.OpenDB(dialect.SQLite, store.DB())))
	cleanup := func() {
		_ = client.Close()
		_ = store.Close()
	}

	if err := client.Schema.Create(ctx); err != nil {
		cleanup()
		return nil, nil, fmt.Errorf("create ontology schema: %w", err)
	}

	graph := graphstore.NewEntStore(client)
	if cfg.SeedFakeFlinkWorkstream {
		if err := sampledata.SeedFakeFlinkAutoscalerWorkstream(ctx, graph); err != nil {
			cleanup()
			return nil, nil, fmt.Errorf("seed fake Flink workstream: %w", err)
		}
	}
	return graph, cleanup, nil
}
