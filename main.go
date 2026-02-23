package main

import (
	"context"
	"flag"
	"fmt"
	"log/slog"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"github.com/korosuke613/ghacron/api"
	"github.com/korosuke613/ghacron/config"
	"github.com/korosuke613/ghacron/github"
	"github.com/korosuke613/ghacron/scheduler"
)

var version = "dev"

func main() {
	var (
		configPath  = flag.String("config", config.GetDefaultConfigPath(), "設定ファイルのパス")
		showVersion = flag.Bool("version", false, "バージョンを表示")
	)
	flag.Parse()

	if *showVersion {
		fmt.Printf("ghacron v%s\n", version)
		return
	}

	// 設定ファイルを読み込み
	cfg, err := config.Load(*configPath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "failed to load config: %v\n", err)
		os.Exit(1)
	}

	// 構造化ロガーを初期化
	initLogger(&cfg.Log)

	slog.Info("starting ghacron", "version", version)

	// GitHub App クライアントを初期化
	privateKey, err := cfg.GetPrivateKey()
	if err != nil {
		slog.Error("failed to get private key", "error", err)
		os.Exit(1)
	}

	ghClient, err := github.NewClient(cfg.GitHub.AppID, privateKey)
	if err != nil {
		slog.Error("failed to initialize GitHub client", "error", err)
		os.Exit(1)
	}

	// タイムゾーンを読み込み
	loc, err := time.LoadLocation(cfg.Reconcile.Timezone)
	if err != nil {
		slog.Error("failed to load timezone", "error", err)
		os.Exit(1)
	}

	// スケジューラーを初期化
	sched := scheduler.New(ghClient, &cfg.Reconcile, loc)

	// APIサーバーを初期化・開始
	apiServer := api.NewServer(&cfg.WebAPI, cfg)
	apiServer.SetStatusProvider(sched)
	if err := apiServer.Start(); err != nil {
		slog.Error("failed to start API server", "error", err)
		os.Exit(1)
	}

	// Reconciliationループを開始
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	go sched.RunReconcileLoop(ctx, time.Duration(cfg.Reconcile.IntervalMinutes)*time.Minute)

	slog.Info("ghacron started",
		"interval_minutes", cfg.Reconcile.IntervalMinutes,
		"duplicate_guard_seconds", cfg.Reconcile.DuplicateGuardSeconds,
		"dry_run", cfg.Reconcile.DryRun,
	)

	// シグナルハンドリング
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM)

	sig := <-sigChan
	slog.Info("received signal, shutting down", "signal", sig.String())

	cancel()
	sched.Stop()
	apiServer.Stop()

	slog.Info("ghacron stopped")
}

func initLogger(logCfg *config.LogConfig) {
	level := logCfg.SlogLevel()
	opts := &slog.HandlerOptions{Level: level}

	var handler slog.Handler
	switch strings.ToLower(logCfg.Format) {
	case "text":
		handler = slog.NewTextHandler(os.Stdout, opts)
	default:
		handler = slog.NewJSONHandler(os.Stdout, opts)
	}

	slog.SetDefault(slog.New(handler))
}
