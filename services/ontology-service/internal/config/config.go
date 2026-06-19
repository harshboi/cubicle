package config

import (
	"fmt"
	"path/filepath"
	"strconv"
	"time"

	hocon "github.com/o3co/go.hocon"
)

const (
	envConfigPath               = "CUBICLE_ONTOLOGY_CONFIG_PATH"                // envConfigPath points to an optional HOCON config file.
	envListenAddr               = "CUBICLE_ONTOLOGY_LISTEN_ADDR"                // envListenAddr overrides the local HTTP listen address.
	envAllowPublicBind          = "CUBICLE_ONTOLOGY_ALLOW_PUBLIC_BIND"          // envAllowPublicBind allows non-localhost binds for development.
	envDataRoot                 = "CUBICLE_ONTOLOGY_DATA_ROOT"                  // envDataRoot overrides the base directory for local ontology data.
	envDatabasePath             = "CUBICLE_ONTOLOGY_DATABASE_PATH"              // envDatabasePath overrides the SQLite database file path.
	envSQLiteBusyTimeout        = "CUBICLE_ONTOLOGY_SQLITE_BUSY_TIMEOUT"        // envSQLiteBusyTimeout overrides SQLite's lock wait timeout.
	envGraphQLPlaygroundEnabled = "CUBICLE_ONTOLOGY_GRAPHQL_PLAYGROUND_ENABLED" // envGraphQLPlaygroundEnabled toggles the local GraphQL playground.
)

// defaultConfigHOCON is the serialized built-in baseline parsed before file,
// environment, and flag overrides.
const defaultConfigHOCON = `
server {
  listen_addr = "127.0.0.1:48080"
  allow_public_bind = false
}

storage {
  data_root = ".data"
  database_path = ".data/graph.db"
  sqlite_busy_timeout = 5s
}

graphql {
  playground_enabled = true
}
`

// Config is process-level service configuration.
//
// Keep this type small and explicit. Go services are easier to reason about
// when configuration is loaded once near process startup and passed down as
// values, rather than read ad hoc from environment variables across packages.
type Config struct {
	ConfigPath               string        // ConfigPath is the optional HOCON file path that produced this config.
	ListenAddr               string        // ListenAddr is the host:port address used by the HTTP server.
	AllowPublicBind          bool          // AllowPublicBind permits non-localhost binds for explicit development use.
	DataRoot                 string        // DataRoot is the base directory for local ontology-service data.
	DatabasePath             string        // DatabasePath is the SQLite database file path.
	SQLiteBusyTimeout        time.Duration // SQLiteBusyTimeout is SQLite's local lock wait timeout.
	GraphQLPlaygroundEnabled bool          // GraphQLPlaygroundEnabled controls whether GET /playground is mounted.
}

// LoadOptions controls how reusable configuration loading talks to the process.
type LoadOptions struct {
	ConfigPath string              // ConfigPath is the explicit HOCON file path to load before env overrides.
	Getenv     func(string) string // Getenv is the environment lookup function used for testable overrides.
}

// serializedConfig mirrors the HOCON document shape.
type serializedConfig struct {
	Server  serializedServerConfig  `hocon:"server,omitempty"`  // Server contains HTTP binding settings.
	Storage serializedStorageConfig `hocon:"storage,omitempty"` // Storage contains local SQLite settings.
	GraphQL serializedGraphQLConfig `hocon:"graphql,omitempty"` // GraphQL contains query endpoint settings.
}

// serializedServerConfig is the server section of a HOCON config document.
type serializedServerConfig struct {
	ListenAddr      string `hocon:"listen_addr,omitempty"`       // ListenAddr is the host:port address used by the HTTP server.
	AllowPublicBind bool   `hocon:"allow_public_bind,omitempty"` // AllowPublicBind permits non-localhost binds for explicit development use.
}

// serializedStorageConfig is the storage section of a HOCON config document.
type serializedStorageConfig struct {
	DataRoot          string        `hocon:"data_root,omitempty"`           // DataRoot is the base directory for local ontology-service data.
	DatabasePath      string        `hocon:"database_path,omitempty"`       // DatabasePath is the SQLite database file path.
	SQLiteBusyTimeout time.Duration `hocon:"sqlite_busy_timeout,omitempty"` // SQLiteBusyTimeout is SQLite's local lock wait timeout.
}

// serializedGraphQLConfig is the graphql section of a HOCON config document.
type serializedGraphQLConfig struct {
	PlaygroundEnabled bool `hocon:"playground_enabled,omitempty"` // PlaygroundEnabled controls whether GET /playground is mounted.
}

// LoadWithOptions reads defaults, then an optional HOCON file, then environment
// variable overrides.
//
// The final command-line flag override layer lives in cmd/ontology-service
// because flags are a process concern, while this package owns reusable config
// file and environment parsing.
func LoadWithOptions(opts LoadOptions) (Config, error) {
	getenv := opts.Getenv
	if getenv == nil {
		getenv = func(string) string { return "" }
	}

	serialized, err := loadDefaultSerializedConfig()
	if err != nil {
		return Config{}, err
	}

	configPath := opts.ConfigPath
	if configPath == "" {
		configPath = getenv(envConfigPath)
	}
	if configPath != "" {
		if err := applyHOCONFile(&serialized, configPath); err != nil {
			return Config{}, err
		}
	}
	cfg := serialized.toConfig(configPath)
	if err := applyEnv(&cfg, getenv); err != nil {
		return Config{}, err
	}
	return cfg, nil
}

func loadDefaultSerializedConfig() (serializedConfig, error) {
	document, err := hocon.ParseString(defaultConfigHOCON)
	if err != nil {
		return serializedConfig{}, fmt.Errorf("parse built-in HOCON defaults: %w", err)
	}

	var cfg serializedConfig
	if err := cfg.applyHOCON(document); err != nil {
		return serializedConfig{}, fmt.Errorf("deserialize built-in HOCON defaults: %w", err)
	}
	return cfg, nil
}

func applyHOCONFile(cfg *serializedConfig, path string) error {
	file, err := hocon.ParseFile(path)
	if err != nil {
		return fmt.Errorf("parse HOCON config %s: %w", path, err)
	}
	if err := cfg.applyHOCON(file); err != nil {
		return fmt.Errorf("deserialize HOCON config %s: %w", path, err)
	}
	return nil
}

func (cfg *serializedConfig) applyHOCON(document *hocon.Config) error {
	dataRootWasSet := document.Has("storage.data_root")
	databasePathWasSet := document.Has("storage.database_path")
	if err := document.Unmarshal(cfg); err != nil {
		return err
	}
	if dataRootWasSet && !databasePathWasSet {
		cfg.Storage.DatabasePath = filepath.Join(cfg.Storage.DataRoot, "graph.db")
	}
	return nil
}

func (cfg serializedConfig) toConfig(configPath string) Config {
	return Config{
		ConfigPath:               configPath,
		ListenAddr:               cfg.Server.ListenAddr,
		AllowPublicBind:          cfg.Server.AllowPublicBind,
		DataRoot:                 cfg.Storage.DataRoot,
		DatabasePath:             cfg.Storage.DatabasePath,
		SQLiteBusyTimeout:        cfg.Storage.SQLiteBusyTimeout,
		GraphQLPlaygroundEnabled: cfg.GraphQL.PlaygroundEnabled,
	}
}

func applyEnv(cfg *Config, getenv func(string) string) error {
	if v := getenv(envListenAddr); v != "" {
		cfg.ListenAddr = v
	}
	if v := getenv(envAllowPublicBind); v != "" {
		value, err := strconv.ParseBool(v)
		if err != nil {
			return fmt.Errorf("parse %s: %w", envAllowPublicBind, err)
		}
		cfg.AllowPublicBind = value
	}
	if v := getenv(envDataRoot); v != "" {
		cfg.DataRoot = v
		cfg.DatabasePath = filepath.Join(v, "graph.db")
	}
	if v := getenv(envDatabasePath); v != "" {
		cfg.DatabasePath = v
	}
	if v := getenv(envSQLiteBusyTimeout); v != "" {
		value, err := time.ParseDuration(v)
		if err != nil {
			return fmt.Errorf("parse %s: %w", envSQLiteBusyTimeout, err)
		}
		cfg.SQLiteBusyTimeout = value
	}
	if v := getenv(envGraphQLPlaygroundEnabled); v != "" {
		value, err := strconv.ParseBool(v)
		if err != nil {
			return fmt.Errorf("parse %s: %w", envGraphQLPlaygroundEnabled, err)
		}
		cfg.GraphQLPlaygroundEnabled = value
	}
	return nil
}
