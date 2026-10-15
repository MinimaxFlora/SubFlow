package config

import (
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
)

// Config holds runtime configuration.
//
// Precedence, highest first: command-line flag > environment variable >
// configuration file > built-in default.
type Config struct {
	// Host is the address the HTTP server listens on.
	Host string `json:"host"`
	// Port is the HTTP listen port.
	Port int `json:"port"`
	// DataDir is the root directory for persistent data.
	DataDir string `json:"dataDir"`
	// DBPath is the SQLite database file path. Defaults to <DataDir>/subflow.db.
	DBPath string `json:"dbPath"`
	// LogLevel is one of debug, info, warn, error.
	LogLevel string `json:"logLevel"`
	// Dev enables development behaviour such as permissive CORS.
	Dev bool `json:"dev"`
	// FetchUserAgent is the User-Agent sent when retrieving remote subscriptions.
	FetchUserAgent string `json:"fetchUserAgent"`
	// FetchAllowPrivateHosts permits fetching subscriptions from private
	// network addresses. Disabled by default to prevent SSRF.
	FetchAllowPrivateHosts bool `json:"fetchAllowPrivateHosts"`
	// ConfigFile is the path of the loaded configuration file, if any.
	ConfigFile string `json:"-"`
}

func defaults() Config {
	return Config{
		Host:     "0.0.0.0",
		Port:     8080,
		DataDir:  "./data",
		LogLevel: "info",
		Dev:      false,
	}
}

// Load resolves configuration from file, environment and flags.
func Load(args []string) (Config, error) {
	cfg := defaults()

	// The configuration file path itself can only come from env or flag.
	configPath := os.Getenv("SUBFLOW_CONFIG")
	if configPath != "" {
		if err := applyFile(&cfg, configPath); err != nil {
			return Config{}, err
		}
		cfg.ConfigFile = configPath
	}

	applyEnv(&cfg)

	fs := flag.NewFlagSet("subflow", flag.ContinueOnError)
	fs.SetOutput(os.Stderr)
	host := fs.String("host", cfg.Host, "HTTP listen address")
	port := fs.Int("port", cfg.Port, "HTTP listen port")
	dataDir := fs.String("data-dir", cfg.DataDir, "persistent data directory")
	dbPath := fs.String("db", cfg.DBPath, "SQLite database file path")
	logLevel := fs.String("log-level", cfg.LogLevel, "log level: debug, info, warn, error")
	dev := fs.Bool("dev", cfg.Dev, "enable development mode")
	fetchUserAgent := fs.String("fetch-user-agent", cfg.FetchUserAgent, "User-Agent used for remote subscription fetches")
	fetchAllowPrivate := fs.Bool("fetch-allow-private-hosts", cfg.FetchAllowPrivateHosts, "allow fetching remote subscriptions from private network addresses")
	configFlag := fs.String("config", cfg.ConfigFile, "path to JSON configuration file")
	if err := fs.Parse(args); err != nil {
		return Config{}, err
	}

	cfg.Host = *host
	cfg.Port = *port
	cfg.DataDir = *dataDir
	cfg.DBPath = *dbPath
	cfg.LogLevel = *logLevel
	cfg.Dev = *dev
	cfg.FetchUserAgent = *fetchUserAgent
	cfg.FetchAllowPrivateHosts = *fetchAllowPrivate
	cfg.ConfigFile = *configFlag

	if cfg.DBPath == "" {
		cfg.DBPath = filepath.Join(cfg.DataDir, "subflow.db")
	}

	if err := cfg.validate(); err != nil {
		return Config{}, err
	}
	return cfg, nil
}

func (c Config) validate() error {
	if c.Port < 1 || c.Port > 65535 {
		return fmt.Errorf("invalid port %d", c.Port)
	}
	if c.DataDir == "" {
		return fmt.Errorf("data-dir must not be empty")
	}
	switch c.LogLevel {
	case "debug", "info", "warn", "error":
	default:
		return fmt.Errorf("invalid log-level %q", c.LogLevel)
	}
	return nil
}

func applyFile(cfg *Config, path string) error {
	raw, err := os.ReadFile(path)
	if err != nil {
		return fmt.Errorf("read config file: %w", err)
	}
	// Use a pointer struct so omitted fields keep their defaults.
	var fromFile struct {
		Host                   *string `json:"host"`
		Port                   *int    `json:"port"`
		DataDir                *string `json:"dataDir"`
		DBPath                 *string `json:"dbPath"`
		LogLevel               *string `json:"logLevel"`
		Dev                    *bool   `json:"dev"`
		FetchUserAgent         *string `json:"fetchUserAgent"`
		FetchAllowPrivateHosts *bool   `json:"fetchAllowPrivateHosts"`
	}
	if err := json.Unmarshal(raw, &fromFile); err != nil {
		return fmt.Errorf("parse config file %s: %w", path, err)
	}
	if fromFile.Host != nil {
		cfg.Host = *fromFile.Host
	}
	if fromFile.Port != nil {
		cfg.Port = *fromFile.Port
	}
	if fromFile.DataDir != nil {
		cfg.DataDir = *fromFile.DataDir
	}
	if fromFile.DBPath != nil {
		cfg.DBPath = *fromFile.DBPath
	}
	if fromFile.LogLevel != nil {
		cfg.LogLevel = *fromFile.LogLevel
	}
	if fromFile.Dev != nil {
		cfg.Dev = *fromFile.Dev
	}
	if fromFile.FetchUserAgent != nil {
		cfg.FetchUserAgent = *fromFile.FetchUserAgent
	}
	if fromFile.FetchAllowPrivateHosts != nil {
		cfg.FetchAllowPrivateHosts = *fromFile.FetchAllowPrivateHosts
	}
	return nil
}

func applyEnv(cfg *Config) {
	if v, ok := os.LookupEnv("SUBFLOW_HOST"); ok {
		cfg.Host = v
	}
	if v, ok := os.LookupEnv("SUBFLOW_PORT"); ok {
		if n, err := strconv.Atoi(strings.TrimSpace(v)); err == nil {
			cfg.Port = n
		}
	}
	if v, ok := os.LookupEnv("SUBFLOW_DATA_DIR"); ok {
		cfg.DataDir = v
	}
	if v, ok := os.LookupEnv("SUBFLOW_DB_PATH"); ok {
		cfg.DBPath = v
	}
	if v, ok := os.LookupEnv("SUBFLOW_LOG_LEVEL"); ok {
		cfg.LogLevel = v
	}
	if v, ok := os.LookupEnv("SUBFLOW_DEV"); ok {
		if b, err := strconv.ParseBool(strings.TrimSpace(v)); err == nil {
			cfg.Dev = b
		}
	}
	if v, ok := os.LookupEnv("SUBFLOW_FETCH_USER_AGENT"); ok {
		cfg.FetchUserAgent = v
	}
	if v, ok := os.LookupEnv("SUBFLOW_FETCH_ALLOW_PRIVATE_HOSTS"); ok {
		if b, err := strconv.ParseBool(strings.TrimSpace(v)); err == nil {
			cfg.FetchAllowPrivateHosts = b
		}
	}
}
