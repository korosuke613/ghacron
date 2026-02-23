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

// Config 全体の設定構造
type Config struct {
	GitHub    GitHubConfig    `yaml:"github"`
	Reconcile ReconcileConfig `yaml:"reconcile"`
	Log       LogConfig       `yaml:"log"`
	WebAPI    WebAPIConfig    `yaml:"webapi"`
}

// GitHubConfig GitHub App設定
type GitHubConfig struct {
	AppID          int64  `yaml:"app_id"`
	PrivateKey     string `yaml:"private_key"`
	PrivateKeyPath string `yaml:"private_key_path"`
}

// ReconcileConfig Reconciliation設定
type ReconcileConfig struct {
	IntervalMinutes       int    `yaml:"interval_minutes"`
	DuplicateGuardSeconds int    `yaml:"duplicate_guard_seconds"`
	DryRun                bool   `yaml:"dry_run"`
	Timezone              string `yaml:"timezone"`
}

// LogConfig ログ設定
type LogConfig struct {
	Level  string `yaml:"level"`
	Format string `yaml:"format"`
}

// SlogLevel Level文字列をslog.Levelに変換
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

// WebAPIConfig WebAPI設定
type WebAPIConfig struct {
	Enabled bool   `yaml:"enabled"`
	Host    string `yaml:"host"`
	Port    int    `yaml:"port"`
}

// rawConfig YAML読み込み用の中間構造体（app_idを文字列として受け取る）
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

// Load 設定ファイルを読み込む
func Load(configPath string) (*Config, error) {
	data, err := os.ReadFile(configPath)
	if err != nil {
		return nil, fmt.Errorf("設定ファイルの読み込みに失敗: %w", err)
	}

	// 環境変数を展開
	content := os.ExpandEnv(string(data))

	// YAMLをパース（中間構造体へ）
	var raw rawConfig
	if err := yaml.Unmarshal([]byte(content), &raw); err != nil {
		return nil, fmt.Errorf("設定ファイルのパースに失敗: %w", err)
	}

	// app_idを数値に変換
	appID, err := strconv.ParseInt(raw.GitHub.AppID, 10, 64)
	if err != nil {
		return nil, fmt.Errorf("github.app_id の数値変換に失敗 (%q): %w", raw.GitHub.AppID, err)
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
		return nil, fmt.Errorf("設定の検証に失敗: %w", err)
	}

	return config, nil
}

// GetPrivateKey Private Keyを取得（環境変数 > 直接指定 > ファイル）
// PEM秘密鍵は複数行テキストのため、YAML展開（os.ExpandEnv）では改行が壊れる。
// 環境変数 GH_APP_PRIVATE_KEY からの直接読み取りを最優先とする。
func (c *Config) GetPrivateKey() ([]byte, error) {
	// 環境変数から直接取得（YAML展開を経由しない）
	if envKey := os.Getenv("GH_APP_PRIVATE_KEY"); envKey != "" {
		return []byte(envKey), nil
	}
	if c.GitHub.PrivateKey != "" {
		return []byte(c.GitHub.PrivateKey), nil
	}
	if c.GitHub.PrivateKeyPath != "" {
		data, err := os.ReadFile(c.GitHub.PrivateKeyPath)
		if err != nil {
			return nil, fmt.Errorf("秘密鍵ファイルの読み込みに失敗: %w", err)
		}
		return data, nil
	}
	return nil, fmt.Errorf("秘密鍵が設定されていません（GH_APP_PRIVATE_KEY 環境変数、private_key、または private_key_path を指定してください）")
}

func (c *Config) validate() error {
	if c.GitHub.AppID <= 0 {
		return fmt.Errorf("github.app_id が未設定または不正です")
	}
	if c.GitHub.PrivateKey == "" && c.GitHub.PrivateKeyPath == "" && os.Getenv("GH_APP_PRIVATE_KEY") == "" {
		return fmt.Errorf("GH_APP_PRIVATE_KEY 環境変数、github.private_key、または github.private_key_path のいずれかを設定してください")
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
		return fmt.Errorf("reconcile.timezone が不正です (%q): %w", c.Reconcile.Timezone, err)
	}
	if c.WebAPI.Port <= 0 {
		c.WebAPI.Port = 8080
	}
	switch strings.ToLower(c.Log.Level) {
	case "debug", "info", "warn", "error", "":
		// OK
	default:
		return fmt.Errorf("log.level が不正です (%q): debug, info, warn, error のいずれかを指定してください", c.Log.Level)
	}
	switch strings.ToLower(c.Log.Format) {
	case "json", "text", "":
		// OK
	default:
		return fmt.Errorf("log.format が不正です (%q): json, text のいずれかを指定してください", c.Log.Format)
	}
	return nil
}

// GetDefaultConfigPath デフォルトの設定ファイルパスを取得
func GetDefaultConfigPath() string {
	if path := os.Getenv("GHACRON_CONFIG"); path != "" {
		return path
	}
	return "config/config.yaml"
}
