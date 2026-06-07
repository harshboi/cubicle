package config

import (
	"fmt"
	"path/filepath"
	"strconv"
	"time"

	hocon "github.com/o3co/go.hocon"
)

const (
	envListenAddr   = "CUBICLE_ONTOLOGY_LISTEN_ADDR"                    // envListenAddr overrides the local HTTP listen address.
	envDataRoot     = "CUBICLE_ONTOLOGY_DATA_ROOT"                      // envDataRoot overrides the base directory for local ontology data.
	envDatabasePath = "CUBICLE_ONTOLOGY_DATABASE_PATH"                  // envDatabasePath overrides the SQLite database file path.
	envConfigPath   = "CUBICLE_ONTOLOGY_CONFIG_PATH"                    // envConfigPath points to the optional HOCON config file.
	envOpenAPIURL   = "CUBICLE_ONTOLOGY_OPENAPI_SERVER_URL"             // envOpenAPIURL overrides the server URL advertised in OpenAPI.
	envBusyTimeout  = "CUBICLE_ONTOLOGY_SQLITE_BUSY_TIMEOUT"            // envBusyTimeout overrides the SQLite busy timeout duration.
	envFakeSeed     = "CUBICLE_ONTOLOGY_DEV_SEED_FAKE_FLINK_WORKSTREAM" // envFakeSeed enables dev-only fake Flink workstream data.
)

// Config is process-level service configuration.
//
// Keep this type small and explicit. Go services are easier to reason about
// when configuration is loaded once near process startup and passed down as
// values, rather than read ad hoc from environment variables across packages.
type Config struct {
	ConfigPath               string        // ConfigPath is the optional HOCON file path that produced this config.
	ListenAddr               string        // ListenAddr is the host:port address used by the HTTP server.
	OpenAPIServerURL         string        // OpenAPIServerURL is the base URL advertised in generated OpenAPI metadata.
	OpenAPIServerURLExplicit bool          // OpenAPIServerURLExplicit tracks whether OpenAPIServerURL was explicitly configured.
	DataRoot                 string        // DataRoot is the base directory for local ontology-service data.
	DatabasePath             string        // DatabasePath is the SQLite database file path.
	SeedFakeFlinkWorkstream  bool          // SeedFakeFlinkWorkstream enables explicit dev-only fake graph seeding.
	SQLiteBusyTimeout        time.Duration // SQLiteBusyTimeout is the lock wait timeout used by SQLite.
}

type LoadOptions struct {
	ConfigPath string              // ConfigPath is the explicit HOCON file path to load before env overrides.
	Getenv     func(string) string // Getenv is the environment lookup function used for testable overrides.
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
	if value, ok := file.GetDurationOption("storage.sqlite_busy_timeout").Get(); ok {
		cfg.SQLiteBusyTimeout = value
	}
	if value, ok := file.GetBoolOption("dev_seed.fake_flink_workstream").Get(); ok {
		cfg.SeedFakeFlinkWorkstream = value
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
	if v := getenv(envBusyTimeout); v != "" {
		value, err := time.ParseDuration(v)
		if err != nil {
			return fmt.Errorf("parse %s: %w", envBusyTimeout, err)
		}
		cfg.SQLiteBusyTimeout = value
	}
	if v := getenv(envFakeSeed); v != "" {
		value, err := strconv.ParseBool(v)
		if err != nil {
			return fmt.Errorf("parse %s: %w", envFakeSeed, err)
		}
		cfg.SeedFakeFlinkWorkstream = value
	}
	return nil
}

func openAPIURLFromListen(listen string) string {
	return "http://" + listen
}
