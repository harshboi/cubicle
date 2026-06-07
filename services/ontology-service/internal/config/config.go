package config

import (
	"fmt"
	"path/filepath"
	"strconv"
	"time"

	hocon "github.com/o3co/go.hocon"
)

const (
	envListenAddr   = "CUBICLE_ONTOLOGY_LISTEN_ADDR"
	envDataRoot     = "CUBICLE_ONTOLOGY_DATA_ROOT"
	envDatabasePath = "CUBICLE_ONTOLOGY_DATABASE_PATH"
	envConfigPath   = "CUBICLE_ONTOLOGY_CONFIG_PATH"
	envSeedFixtures = "CUBICLE_ONTOLOGY_SEED_FIXTURES"
	envOpenAPIURL   = "CUBICLE_ONTOLOGY_OPENAPI_SERVER_URL"
	envBusyTimeout  = "CUBICLE_ONTOLOGY_SQLITE_BUSY_TIMEOUT"
)

// Config is process-level service configuration.
//
// Keep this type small and explicit. Go services are easier to reason about
// when configuration is loaded once near process startup and passed down as
// values, rather than read ad hoc from environment variables across packages.
type Config struct {
	ConfigPath               string
	ListenAddr               string
	OpenAPIServerURL         string
	OpenAPIServerURLExplicit bool
	DataRoot                 string
	DatabasePath             string
	SeedFixtures             bool
	SQLiteBusyTimeout        time.Duration
}

type LoadOptions struct {
	ConfigPath string
	Getenv     func(string) string
}

// Load reads environment-only configuration through the supplied lookup
// function.
//
// Accepting a function instead of calling os.Getenv directly keeps tests simple
// and avoids mutating global process environment. main can pass os.Getenv.
func Load(getenv func(string) string) Config {
	cfg, _ := LoadWithOptions(LoadOptions{Getenv: getenv})
	return cfg
}

// LoadWithOptions reads defaults, then an optional HOCON file, then
// environment variable overrides.
//
// This keeps precedence explicit:
//
//	defaults < config file < environment < command-line flags
//
// The command-line flag layer lives in cmd/ontology-service because flags are a
// process concern, while this package owns reusable configuration loading.
func LoadWithOptions(opts LoadOptions) (Config, error) {
	getenv := opts.Getenv
	if getenv == nil {
		getenv = func(string) string { return "" }
	}

	cfg := Config{
		ListenAddr:        "127.0.0.1:48080",
		DataRoot:          ".data",
		SeedFixtures:      true,
		SQLiteBusyTimeout: 5 * time.Second,
	}
	cfg.DatabasePath = filepath.Join(cfg.DataRoot, "graph.db")
	cfg.OpenAPIServerURL = openAPIURLFromListen(cfg.ListenAddr)

	configPath := opts.ConfigPath
	if configPath == "" {
		configPath = getenv(envConfigPath)
	}
	if configPath != "" {
		if err := applyHOCONFile(&cfg, configPath); err != nil {
			return Config{}, err
		}
		cfg.ConfigPath = configPath
	}
	if err := applyEnv(&cfg, getenv); err != nil {
		return Config{}, err
	}
	return cfg, nil
}

func applyHOCONFile(cfg *Config, path string) error {
	file, err := hocon.ParseFile(path)
	if err != nil {
		return fmt.Errorf("parse HOCON config %s: %w", path, err)
	}

	if value, ok := file.GetStringOption("server.listen_addr").Get(); ok {
		cfg.ListenAddr = value
		if !cfg.OpenAPIServerURLExplicit {
			cfg.OpenAPIServerURL = openAPIURLFromListen(value)
		}
	}
	if value, ok := file.GetStringOption("server.openapi_server_url").Get(); ok {
		cfg.OpenAPIServerURL = value
		cfg.OpenAPIServerURLExplicit = true
	}
	if value, ok := file.GetStringOption("storage.data_root").Get(); ok {
		cfg.DataRoot = value
		cfg.DatabasePath = filepath.Join(value, "graph.db")
	}
	if value, ok := file.GetStringOption("storage.database_path").Get(); ok {
		cfg.DatabasePath = value
	}
	if value, ok := file.GetBoolOption("fixtures.seed").Get(); ok {
		cfg.SeedFixtures = value
	}
	if value, ok := file.GetDurationOption("storage.sqlite_busy_timeout").Get(); ok {
		cfg.SQLiteBusyTimeout = value
	}
	return nil
}

func applyEnv(cfg *Config, getenv func(string) string) error {
	if v := getenv(envListenAddr); v != "" {
		cfg.ListenAddr = v
		if !cfg.OpenAPIServerURLExplicit {
			cfg.OpenAPIServerURL = openAPIURLFromListen(v)
		}
	}
	if v := getenv(envOpenAPIURL); v != "" {
		cfg.OpenAPIServerURL = v
		cfg.OpenAPIServerURLExplicit = true
	}
	if v := getenv(envDataRoot); v != "" {
		cfg.DataRoot = v
		cfg.DatabasePath = filepath.Join(v, "graph.db")
	}
	if v := getenv(envDatabasePath); v != "" {
		cfg.DatabasePath = v
	}
	if v := getenv(envSeedFixtures); v != "" {
		value, err := strconv.ParseBool(v)
		if err != nil {
			return fmt.Errorf("parse %s: %w", envSeedFixtures, err)
		}
		cfg.SeedFixtures = value
	}
	if v := getenv(envBusyTimeout); v != "" {
		value, err := time.ParseDuration(v)
		if err != nil {
			return fmt.Errorf("parse %s: %w", envBusyTimeout, err)
		}
		cfg.SQLiteBusyTimeout = value
	}
	return nil
}

func openAPIURLFromListen(listen string) string {
	return "http://" + listen
}
