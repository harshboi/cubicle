package config

import "path/filepath"

const (
	envListenAddr   = "CUBICLE_ONTOLOGY_LISTEN_ADDR"
	envDataRoot     = "CUBICLE_ONTOLOGY_DATA_ROOT"
	envDatabasePath = "CUBICLE_ONTOLOGY_DATABASE_PATH"
)

// Config is process-level service configuration.
//
// Keep this type small and explicit. Go services are easier to reason about
// when configuration is loaded once near process startup and passed down as
// values, rather than read ad hoc from environment variables across packages.
type Config struct {
	ListenAddr   string
	DataRoot     string
	DatabasePath string
}

// Load reads configuration through the supplied lookup function.
//
// Accepting a function instead of calling os.Getenv directly keeps tests simple
// and avoids mutating global process environment. main can pass os.Getenv.
func Load(getenv func(string) string) Config {
	cfg := Config{
		ListenAddr: "127.0.0.1:48080",
		DataRoot:   ".data",
	}
	cfg.DatabasePath = filepath.Join(cfg.DataRoot, "graph.db")

	if v := getenv(envListenAddr); v != "" {
		cfg.ListenAddr = v
	}
	if v := getenv(envDataRoot); v != "" {
		cfg.DataRoot = v
		cfg.DatabasePath = filepath.Join(v, "graph.db")
	}
	if v := getenv(envDatabasePath); v != "" {
		cfg.DatabasePath = v
	}
	return cfg
}
