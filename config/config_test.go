package config

import (
	"os"
	"path/filepath"
	"testing"
)

func setRequiredEnv(t *testing.T) {
	t.Helper()
	t.Setenv("GHACRON_APP_ID", "123456")
	t.Setenv("GHACRON_APP_PRIVATE_KEY", "dummy-key")
}

func TestLoad_Defaults(t *testing.T) {
	setRequiredEnv(t)

	cfg, err := Load()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if cfg.GitHub.AppID != 123456 {
		t.Errorf("AppID = %d, want 123456", cfg.GitHub.AppID)
	}
	if cfg.Reconcile.IntervalMinutes != 5 {
		t.Errorf("IntervalMinutes = %d, want 5", cfg.Reconcile.IntervalMinutes)
	}
	if cfg.Reconcile.DuplicateGuardSeconds != 60 {
		t.Errorf("DuplicateGuardSeconds = %d, want 60", cfg.Reconcile.DuplicateGuardSeconds)
	}
	if cfg.Reconcile.DryRun {
		t.Errorf("DryRun = true, want false")
	}
	if cfg.Reconcile.Timezone != "UTC" {
		t.Errorf("Timezone = %q, want %q", cfg.Reconcile.Timezone, "UTC")
	}
	if cfg.Log.Level != "info" {
		t.Errorf("LogLevel = %q, want %q", cfg.Log.Level, "info")
	}
	if cfg.Log.Format != "json" {
		t.Errorf("LogFormat = %q, want %q", cfg.Log.Format, "json")
	}
	if !cfg.WebAPI.Enabled {
		t.Errorf("WebAPI.Enabled = false, want true")
	}
	if cfg.WebAPI.Host != "0.0.0.0" {
		t.Errorf("WebAPI.Host = %q, want %q", cfg.WebAPI.Host, "0.0.0.0")
	}
	if cfg.WebAPI.Port != 8080 {
		t.Errorf("WebAPI.Port = %d, want 8080", cfg.WebAPI.Port)
	}
}

func TestLoad_AllEnvVars(t *testing.T) {
	t.Setenv("GHACRON_APP_ID", "999")
	t.Setenv("GHACRON_APP_PRIVATE_KEY", "test-pem")
	t.Setenv("GHACRON_RECONCILE_INTERVAL_MINUTES", "10")
	t.Setenv("GHACRON_RECONCILE_DUPLICATE_GUARD_SECONDS", "120")
	t.Setenv("GHACRON_DRY_RUN", "true")
	t.Setenv("GHACRON_TIMEZONE", "Asia/Tokyo")
	t.Setenv("GHACRON_LOG_LEVEL", "debug")
	t.Setenv("GHACRON_LOG_FORMAT", "text")
	t.Setenv("GHACRON_WEBAPI_ENABLED", "false")
	t.Setenv("GHACRON_WEBAPI_HOST", "127.0.0.1")
	t.Setenv("GHACRON_WEBAPI_PORT", "9090")

	cfg, err := Load()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if cfg.GitHub.AppID != 999 {
		t.Errorf("AppID = %d, want 999", cfg.GitHub.AppID)
	}
	if cfg.GitHub.PrivateKey != "test-pem" {
		t.Errorf("PrivateKey = %q, want %q", cfg.GitHub.PrivateKey, "test-pem")
	}
	if cfg.Reconcile.IntervalMinutes != 10 {
		t.Errorf("IntervalMinutes = %d, want 10", cfg.Reconcile.IntervalMinutes)
	}
	if cfg.Reconcile.DuplicateGuardSeconds != 120 {
		t.Errorf("DuplicateGuardSeconds = %d, want 120", cfg.Reconcile.DuplicateGuardSeconds)
	}
	if !cfg.Reconcile.DryRun {
		t.Errorf("DryRun = false, want true")
	}
	if cfg.Reconcile.Timezone != "Asia/Tokyo" {
		t.Errorf("Timezone = %q, want %q", cfg.Reconcile.Timezone, "Asia/Tokyo")
	}
	if cfg.Log.Level != "debug" {
		t.Errorf("LogLevel = %q, want %q", cfg.Log.Level, "debug")
	}
	if cfg.Log.Format != "text" {
		t.Errorf("LogFormat = %q, want %q", cfg.Log.Format, "text")
	}
	if cfg.WebAPI.Enabled {
		t.Errorf("WebAPI.Enabled = true, want false")
	}
	if cfg.WebAPI.Host != "127.0.0.1" {
		t.Errorf("WebAPI.Host = %q, want %q", cfg.WebAPI.Host, "127.0.0.1")
	}
	if cfg.WebAPI.Port != 9090 {
		t.Errorf("WebAPI.Port = %d, want 9090", cfg.WebAPI.Port)
	}
}

func TestLoad_MissingAppID(t *testing.T) {
	t.Setenv("GHACRON_APP_PRIVATE_KEY", "dummy")

	_, err := Load()
	if err == nil {
		t.Fatal("expected error for missing GHACRON_APP_ID")
	}
}

func TestLoad_MissingPrivateKey(t *testing.T) {
	t.Setenv("GHACRON_APP_ID", "123")

	_, err := Load()
	if err == nil {
		t.Fatal("expected error for missing private key")
	}
}

func TestLoad_InvalidAppID(t *testing.T) {
	t.Setenv("GHACRON_APP_ID", "not-a-number")
	t.Setenv("GHACRON_APP_PRIVATE_KEY", "dummy")

	_, err := Load()
	if err == nil {
		t.Fatal("expected error for non-numeric GHACRON_APP_ID")
	}
}

func TestLoad_InvalidTimezone(t *testing.T) {
	setRequiredEnv(t)
	t.Setenv("GHACRON_TIMEZONE", "Invalid/Zone")

	_, err := Load()
	if err == nil {
		t.Fatal("expected error for invalid timezone")
	}
}

func TestLoad_InvalidLogLevel(t *testing.T) {
	setRequiredEnv(t)
	t.Setenv("GHACRON_LOG_LEVEL", "verbose")

	_, err := Load()
	if err == nil {
		t.Fatal("expected error for invalid log level")
	}
}

func TestLoad_InvalidLogFormat(t *testing.T) {
	setRequiredEnv(t)
	t.Setenv("GHACRON_LOG_FORMAT", "yaml")

	_, err := Load()
	if err == nil {
		t.Fatal("expected error for invalid log format")
	}
}

func TestLoad_InvalidBool(t *testing.T) {
	setRequiredEnv(t)
	t.Setenv("GHACRON_DRY_RUN", "yes-please")

	_, err := Load()
	if err == nil {
		t.Fatal("expected error for invalid boolean")
	}
}

func TestLoad_InvalidInt(t *testing.T) {
	setRequiredEnv(t)
	t.Setenv("GHACRON_RECONCILE_INTERVAL_MINUTES", "five")

	_, err := Load()
	if err == nil {
		t.Fatal("expected error for non-numeric interval")
	}
}

func TestLoad_BoolVariants(t *testing.T) {
	tests := []struct {
		input string
		want  bool
	}{
		{"true", true},
		{"false", false},
		{"1", true},
		{"0", false},
		{"TRUE", true},
		{"FALSE", false},
	}
	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			setRequiredEnv(t)
			t.Setenv("GHACRON_DRY_RUN", tt.input)

			cfg, err := Load()
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if cfg.Reconcile.DryRun != tt.want {
				t.Errorf("DryRun = %v, want %v", cfg.Reconcile.DryRun, tt.want)
			}
		})
	}
}

func TestGetPrivateKey_EnvPriority(t *testing.T) {
	tmpDir := t.TempDir()
	keyFile := filepath.Join(tmpDir, "key.pem")
	if err := os.WriteFile(keyFile, []byte("file-key"), 0600); err != nil {
		t.Fatal(err)
	}

	cfg := &Config{
		GitHub: GitHubConfig{
			PrivateKey:     "env-key",
			PrivateKeyPath: keyFile,
		},
	}

	key, err := cfg.GetPrivateKey()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if string(key) != "env-key" {
		t.Errorf("key = %q, want %q", string(key), "env-key")
	}
}

func TestGetPrivateKey_FileFallback(t *testing.T) {
	tmpDir := t.TempDir()
	keyFile := filepath.Join(tmpDir, "key.pem")
	if err := os.WriteFile(keyFile, []byte("file-key"), 0600); err != nil {
		t.Fatal(err)
	}

	cfg := &Config{
		GitHub: GitHubConfig{
			PrivateKeyPath: keyFile,
		},
	}

	key, err := cfg.GetPrivateKey()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if string(key) != "file-key" {
		t.Errorf("key = %q, want %q", string(key), "file-key")
	}
}

func TestGetPrivateKey_NeitherSet(t *testing.T) {
	cfg := &Config{}

	_, err := cfg.GetPrivateKey()
	if err == nil {
		t.Fatal("expected error when neither key nor path is set")
	}
}

func TestGetPrivateKey_FileNotFound(t *testing.T) {
	cfg := &Config{
		GitHub: GitHubConfig{
			PrivateKeyPath: "/nonexistent/path/key.pem",
		},
	}

	_, err := cfg.GetPrivateKey()
	if err == nil {
		t.Fatal("expected error for nonexistent key file")
	}
}
