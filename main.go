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
		fmt.Fprintf(os.Stderr, "設定ファイルの読み込みに失敗: %v\n", err)
		os.Exit(1)
	}

	// 構造化ロガーを初期化
	initLogger(&cfg.Log)

	slog.Info("ghacron を開始します", "version", version)

	// GitHub App クライアントを初期化
	privateKey, err := cfg.GetPrivateKey()
	if err != nil {
		slog.Error("秘密鍵の取得に失敗", "error", err)
		os.Exit(1)
	}

	ghClient, err := github.NewClient(cfg.GitHub.AppID, privateKey)
	if err != nil {
		slog.Error("GitHubクライアントの初期化に失敗", "error", err)
		os.Exit(1)
	}

	// タイムゾーンを読み込み
	loc, err := time.LoadLocation(cfg.Reconcile.Timezone)
	if err != nil {
		slog.Error("タイムゾーンの読み込みに失敗", "error", err)
		os.Exit(1)
	}

	// スケジューラーを初期化
	sched := scheduler.New(ghClient, &cfg.Reconcile, loc)

	// APIサーバーを初期化・開始
	apiServer := api.NewServer(&cfg.WebAPI, cfg)
	apiServer.SetStatusProvider(sched)
	if err := apiServer.Start(); err != nil {
		slog.Error("APIサーバーの開始に失敗", "error", err)
		os.Exit(1)
	}

	// Reconciliationループを開始
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	go sched.RunReconcileLoop(ctx, time.Duration(cfg.Reconcile.IntervalMinutes)*time.Minute)

	slog.Info("ghacron が開始されました",
		"interval_minutes", cfg.Reconcile.IntervalMinutes,
		"duplicate_guard_seconds", cfg.Reconcile.DuplicateGuardSeconds,
		"dry_run", cfg.Reconcile.DryRun,
	)

	// シグナルハンドリング
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM)

	sig := <-sigChan
	slog.Info("シグナルを受信、サービスを停止します", "signal", sig.String())

	cancel()
	sched.Stop()
	apiServer.Stop()

	slog.Info("ghacron を停止しました")
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
