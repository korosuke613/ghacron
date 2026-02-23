package config

import (
	"errors"
	"fmt"
	"log/slog"
	"os"
	"strconv"
	"strings"
	"time"
)

// Config represents the entire application configuration.
type Config struct {
	GitHub    GitHubConfig
	Reconcile ReconcileConfig
	Log       LogConfig
	WebAPI    WebAPIConfig
}

// GitHubConfig holds GitHub App credentials.
type GitHubConfig struct {
	AppID          int64
	PrivateKey     string
	PrivateKeyPath string
}

// ReconcileConfig holds reconciliation loop settings.
type ReconcileConfig struct {
	IntervalMinutes       int
	DuplicateGuardSeconds int
	DryRun                bool
	Timezone              string
}

// LogConfig holds logging settings.
type LogConfig struct {
	Level  string
	Format string
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
	Enabled bool
	Host    string
	Port    int
}

// Load reads configuration from GHACRON_* environment variables.
func Load() (*Config, error) {
	appID, err := envInt64("GHACRON_APP_ID", 0)
	if err != nil {
		return nil, fmt.Errorf("invalid GHACRON_APP_ID: %w", err)
	}

	intervalMinutes, err := envInt("GHACRON_RECONCILE_INTERVAL_MINUTES", 5)
	if err != nil {
		return nil, fmt.Errorf("invalid GHACRON_RECONCILE_INTERVAL_MINUTES: %w", err)
	}

	duplicateGuardSeconds, err := envInt("GHACRON_RECONCILE_DUPLICATE_GUARD_SECONDS", 60)
	if err != nil {
		return nil, fmt.Errorf("invalid GHACRON_RECONCILE_DUPLICATE_GUARD_SECONDS: %w", err)
	}

	dryRun, err := envBool("GHACRON_DRY_RUN", false)
	if err != nil {
		return nil, fmt.Errorf("invalid GHACRON_DRY_RUN: %w", err)
	}

	timezone := envStr("GHACRON_TIMEZONE", "UTC")

	logLevel := envStr("GHACRON_LOG_LEVEL", "info")
	logFormat := envStr("GHACRON_LOG_FORMAT", "json")

	webapiEnabled, err := envBool("GHACRON_WEBAPI_ENABLED", true)
	if err != nil {
		return nil, fmt.Errorf("invalid GHACRON_WEBAPI_ENABLED: %w", err)
	}

	webapiHost := envStr("GHACRON_WEBAPI_HOST", "0.0.0.0")

	webapiPort, err := envInt("GHACRON_WEBAPI_PORT", 8080)
	if err != nil {
		return nil, fmt.Errorf("invalid GHACRON_WEBAPI_PORT: %w", err)
	}

	config := &Config{
		GitHub: GitHubConfig{
			AppID:          appID,
			PrivateKey:     os.Getenv("GHACRON_APP_PRIVATE_KEY"),
			PrivateKeyPath: os.Getenv("GHACRON_APP_PRIVATE_KEY_PATH"),
		},
		Reconcile: ReconcileConfig{
			IntervalMinutes:       intervalMinutes,
			DuplicateGuardSeconds: duplicateGuardSeconds,
			DryRun:                dryRun,
			Timezone:              timezone,
		},
		Log: LogConfig{
			Level:  logLevel,
			Format: logFormat,
		},
		WebAPI: WebAPIConfig{
			Enabled: webapiEnabled,
			Host:    webapiHost,
			Port:    webapiPort,
		},
	}

	if err := config.validate(); err != nil {
		return nil, err
	}

	return config, nil
}

// GetPrivateKey returns the private key bytes.
// Priority: GHACRON_APP_PRIVATE_KEY env > GHACRON_APP_PRIVATE_KEY_PATH file.
func (c *Config) GetPrivateKey() ([]byte, error) {
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
	return nil, errors.New("private key not configured (set GHACRON_APP_PRIVATE_KEY or GHACRON_APP_PRIVATE_KEY_PATH)")
}

func (c *Config) validate() error {
	if c.GitHub.AppID <= 0 {
		return errors.New("GHACRON_APP_ID is required")
	}
	if c.GitHub.PrivateKey == "" && c.GitHub.PrivateKeyPath == "" {
		return errors.New("GHACRON_APP_PRIVATE_KEY or GHACRON_APP_PRIVATE_KEY_PATH is required")
	}
	if _, err := time.LoadLocation(c.Reconcile.Timezone); err != nil {
		return fmt.Errorf("invalid GHACRON_TIMEZONE (%q): %w", c.Reconcile.Timezone, err)
	}
	switch strings.ToLower(c.Log.Level) {
	case "debug", "info", "warn", "error":
		// OK
	default:
		return fmt.Errorf("invalid GHACRON_LOG_LEVEL (%q): must be one of debug, info, warn, error", c.Log.Level)
	}
	switch strings.ToLower(c.Log.Format) {
	case "json", "text":
		// OK
	default:
		return fmt.Errorf("invalid GHACRON_LOG_FORMAT (%q): must be one of json, text", c.Log.Format)
	}
	return nil
}

func envStr(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}

func envInt(key string, fallback int) (int, error) {
	v := os.Getenv(key)
	if v == "" {
		return fallback, nil
	}
	n, err := strconv.Atoi(v)
	if err != nil {
		return 0, fmt.Errorf("expected integer for %s: %w", key, err)
	}
	return n, nil
}

func envInt64(key string, fallback int64) (int64, error) {
	v := os.Getenv(key)
	if v == "" {
		return fallback, nil
	}
	n, err := strconv.ParseInt(v, 10, 64)
	if err != nil {
		return 0, fmt.Errorf("expected integer for %s: %w", key, err)
	}
	return n, nil
}

func envBool(key string, fallback bool) (bool, error) {
	v := os.Getenv(key)
	if v == "" {
		return fallback, nil
	}
	b, err := strconv.ParseBool(v)
	if err != nil {
		return false, fmt.Errorf("expected boolean for %s: %w", key, err)
	}
	return b, nil
}
