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
	"cubicle/services/ontology-service/internal/fixtures"
	"cubicle/services/ontology-service/internal/graphstore"
	"cubicle/services/ontology-service/internal/httpapi"
	"cubicle/services/ontology-service/internal/storage"
	"entgo.io/ent/dialect"
	entsql "entgo.io/ent/dialect/sql"
)

type serveConfig struct {
	ConfigPath               string
	Listen                   string
	OpenAPIServerURL         string
	OpenAPIServerURLExplicit bool
	DatabasePath             string
	SQLiteBusyTimeout        time.Duration
	SeedFixtures             bool
	AllowPublicBind          bool
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
		SeedFixtures:             appCfg.SeedFixtures,
	}
	flags.StringVar(&cfg.ConfigPath, "config", cfg.ConfigPath, "HOCON config file path")
	flags.StringVar(&cfg.Listen, "listen", cfg.Listen, "host:port for the local HTTP server")
	flags.StringVar(&cfg.OpenAPIServerURL, "openapi-server-url", cfg.OpenAPIServerURL, "server URL advertised in OpenAPI")
	flags.StringVar(&cfg.DatabasePath, "database", cfg.DatabasePath, "SQLite database path for the ontology graph")
	flags.DurationVar(&cfg.SQLiteBusyTimeout, "sqlite-busy-timeout", cfg.SQLiteBusyTimeout, "SQLite busy timeout")
	flags.BoolVar(&cfg.SeedFixtures, "seed-fixtures", cfg.SeedFixtures, "seed the local Flink demo graph before serving")
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
		"seed_fixtures", cfg.SeedFixtures,
	)

	graph, cleanup, err := openGraphStore(context.Background(), cfg)
	if err != nil {
		return err
	}
	defer cleanup()

	router := httpapi.NewRouterWithOptions(graph, logger, httpapi.RouterOptions{
		OpenAPIServerURL: cfg.OpenAPIServerURL,
	})
	server := &http.Server{
		Addr:    cfg.Listen,
		Handler: router,
	}

	logger.Info("ontology_service_listening", "url", "http://"+cfg.Listen)
	return server.ListenAndServe()
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
	if cfg.SeedFixtures {
		if err := fixtures.SeedFlinkAutoscaler(ctx, graph); err != nil {
			cleanup()
			return nil, nil, fmt.Errorf("seed fixture graph: %w", err)
		}
	}
	return graph, cleanup, nil
}
