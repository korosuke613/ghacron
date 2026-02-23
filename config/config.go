package config

import (
	"fmt"
	"log/slog"
	"os"
	"strconv"
	"strings"
	"time"

	"gopkg.in/yaml.v3"
)

// Config represents the entire application configuration.
type Config struct {
	GitHub    GitHubConfig    `yaml:"github"`
	Reconcile ReconcileConfig `yaml:"reconcile"`
	Log       LogConfig       `yaml:"log"`
	WebAPI    WebAPIConfig    `yaml:"webapi"`
}

// GitHubConfig holds GitHub App credentials.
type GitHubConfig struct {
	AppID          int64  `yaml:"app_id"`
	PrivateKey     string `yaml:"private_key"`
	PrivateKeyPath string `yaml:"private_key_path"`
}

// ReconcileConfig holds reconciliation loop settings.
type ReconcileConfig struct {
	IntervalMinutes       int    `yaml:"interval_minutes"`
	DuplicateGuardSeconds int    `yaml:"duplicate_guard_seconds"`
	DryRun                bool   `yaml:"dry_run"`
	Timezone              string `yaml:"timezone"`
}

// LogConfig holds logging settings.
type LogConfig struct {
	Level  string `yaml:"level"`
	Format string `yaml:"format"`
}

// SlogLevel converts the Level string to slog.Level.
func (lc *LogConfig) SlogLevel() slog.Level {
	switch strings.ToLower(lc.Level) {
	case "debug":
		return slog.LevelDebug
	case "warn":
		return slog.LevelWarn
	case "error":
		return slog.LevelError
	default:
		return slog.LevelInfo
	}
}

// WebAPIConfig holds web API server settings.
type WebAPIConfig struct {
	Enabled bool   `yaml:"enabled"`
	Host    string `yaml:"host"`
	Port    int    `yaml:"port"`
}

// rawConfig is an intermediate struct for YAML parsing (app_id as string).
type rawConfig struct {
	GitHub struct {
		AppID          string `yaml:"app_id"`
		PrivateKey     string `yaml:"private_key"`
		PrivateKeyPath string `yaml:"private_key_path"`
	} `yaml:"github"`
	Reconcile ReconcileConfig `yaml:"reconcile"`
	Log       LogConfig       `yaml:"log"`
	WebAPI    WebAPIConfig    `yaml:"webapi"`
}

// Load reads and parses the config file.
func Load(configPath string) (*Config, error) {
	data, err := os.ReadFile(configPath)
	if err != nil {
		return nil, fmt.Errorf("failed to read config file: %w", err)
	}

	expanded := os.ExpandEnv(string(data))

	var raw rawConfig
	if err := yaml.Unmarshal([]byte(expanded), &raw); err != nil {
		return nil, fmt.Errorf("failed to parse config file: %w", err)
	}

	appID, err := strconv.ParseInt(raw.GitHub.AppID, 10, 64)
	if err != nil {
		return nil, fmt.Errorf("failed to parse github.app_id (%q): %w", raw.GitHub.AppID, err)
	}

	config := &Config{
		GitHub: GitHubConfig{
			AppID:          appID,
			PrivateKey:     raw.GitHub.PrivateKey,
			PrivateKeyPath: raw.GitHub.PrivateKeyPath,
		},
		Reconcile: raw.Reconcile,
		Log:       raw.Log,
		WebAPI:    raw.WebAPI,
	}

	if err := config.validate(); err != nil {
		return nil, fmt.Errorf("config validation failed: %w", err)
	}

	return config, nil
}

// GetPrivateKey returns the private key bytes.
// Priority: GH_APP_PRIVATE_KEY env var > private_key field > private_key_path file.
func (c *Config) GetPrivateKey() ([]byte, error) {
	if envKey := os.Getenv("GH_APP_PRIVATE_KEY"); envKey != "" {
		return []byte(envKey), nil
	}
	if c.GitHub.PrivateKey != "" {
		return []byte(c.GitHub.PrivateKey), nil
	}
	if c.GitHub.PrivateKeyPath != "" {
		data, err := os.ReadFile(c.GitHub.PrivateKeyPath)
		if err != nil {
			return nil, fmt.Errorf("failed to read private key file: %w", err)
		}
		return data, nil
	}
	return nil, fmt.Errorf("private key not configured (set GH_APP_PRIVATE_KEY env var, private_key, or private_key_path)")
}

func (c *Config) validate() error {
	if c.GitHub.AppID <= 0 {
		return fmt.Errorf("github.app_id is not set or invalid")
	}
	if c.GitHub.PrivateKey == "" && c.GitHub.PrivateKeyPath == "" && os.Getenv("GH_APP_PRIVATE_KEY") == "" {
		return fmt.Errorf("set GH_APP_PRIVATE_KEY env var, github.private_key, or github.private_key_path")
	}
	if c.Reconcile.IntervalMinutes <= 0 {
		c.Reconcile.IntervalMinutes = 5
	}
	if c.Reconcile.DuplicateGuardSeconds <= 0 {
		c.Reconcile.DuplicateGuardSeconds = 60
	}
	if c.Reconcile.Timezone == "" {
		c.Reconcile.Timezone = "UTC"
	}
	if _, err := time.LoadLocation(c.Reconcile.Timezone); err != nil {
		return fmt.Errorf("invalid reconcile.timezone (%q): %w", c.Reconcile.Timezone, err)
	}
	if c.WebAPI.Port <= 0 {
		c.WebAPI.Port = 8080
	}
	switch strings.ToLower(c.Log.Level) {
	case "debug", "info", "warn", "error", "":
		// OK
	default:
		return fmt.Errorf("invalid log.level (%q): must be one of debug, info, warn, error", c.Log.Level)
	}
	switch strings.ToLower(c.Log.Format) {
	case "json", "text", "":
		// OK
	default:
		return fmt.Errorf("invalid log.format (%q): must be one of json, text", c.Log.Format)
	}
	return nil
}

// GetDefaultConfigPath returns the default config file path.
func GetDefaultConfigPath() string {
	if path := os.Getenv("GHACRON_CONFIG"); path != "" {
		return path
	}
	return "config/config.yaml"
}
