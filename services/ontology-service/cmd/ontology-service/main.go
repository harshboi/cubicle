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
	"cubicle/services/ontology-service/internal/graphstore"
	"cubicle/services/ontology-service/internal/httpapi"
	"cubicle/services/ontology-service/internal/storage"
	"entgo.io/ent/dialect"
	entsql "entgo.io/ent/dialect/sql"
)

type serveConfig struct {
	ConfigPath      string // ConfigPath is the optional HOCON file path used to load runtime defaults.
	Listen          string // Listen is the host:port address the local HTTP server binds to.
	DatabasePath    string // DatabasePath is the SQLite file path used by the Ent-backed ontology store.
	AllowPublicBind bool   // AllowPublicBind permits non-localhost binds for explicit development use.
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
		ConfigPath:   appCfg.ConfigPath,
		Listen:       appCfg.ListenAddr,
		DatabasePath: appCfg.DatabasePath,
	}
	flags.StringVar(&cfg.ConfigPath, "config", cfg.ConfigPath, "HOCON config file path")
	flags.StringVar(&cfg.Listen, "listen", cfg.Listen, "host:port for the local HTTP server")
	flags.StringVar(&cfg.DatabasePath, "database", cfg.DatabasePath, "SQLite database path for the ontology graph")
	flags.BoolVar(&cfg.AllowPublicBind, "allow-public-bind", false, "allow binding outside localhost for development")
	if err := flags.Parse(args[1:]); err != nil {
		return serveConfig{}, err
	}
	if err := validateListenAddress(cfg.Listen, cfg.AllowPublicBind); err != nil {
		return serveConfig{}, err
	}
	return cfg, nil
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

	graph, cleanup, err := openGraphStore(context.Background(), cfg)
	if err != nil {
		return err
	}
	defer cleanup()

	router := httpapi.NewRouter(graph, logger)
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

func openGraphStore(ctx context.Context, cfg serveConfig) (graphstore.Store, func(), error) {
	store, err := storage.Open(ctx, storage.Config{DatabasePath: cfg.DatabasePath})
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
	return graph, cleanup, nil
}
