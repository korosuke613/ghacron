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
	showVersion := flag.Bool("version", false, "show version")
	flag.Parse()

	if *showVersion {
		fmt.Printf("ghacron v%s\n", version)
		return
	}

	// Bootstrap logger with JSON/stdout defaults (before config is available)
	slog.SetDefault(slog.New(slog.NewJSONHandler(os.Stdout, nil)))

	cfg, err := config.Load()
	if err != nil {
		slog.Error("failed to load config", "error", err)
		os.Exit(1)
	}

	// Re-initialize logger with configured level and format
	initLogger(&cfg.Log)

	slog.Info("starting ghacron", "version", version)

	// Initialize GitHub App client
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

	// Load timezone
	loc, err := time.LoadLocation(cfg.Reconcile.Timezone)
	if err != nil {
		slog.Error("failed to load timezone", "error", err)
		os.Exit(1)
	}

	// Initialize scheduler
	sched := scheduler.New(ghClient, &cfg.Reconcile, loc)

	// Initialize and start API server
	apiServer := api.NewServer(&cfg.WebAPI, cfg)
	apiServer.SetStatusProvider(sched)
	if err := apiServer.Start(); err != nil {
		slog.Error("failed to start API server", "error", err)
		os.Exit(1)
	}

	// Start reconciliation loop
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	go sched.RunReconcileLoop(ctx, time.Duration(cfg.Reconcile.IntervalMinutes)*time.Minute)

	slog.Info("ghacron started",
		"interval_minutes", cfg.Reconcile.IntervalMinutes,
		"duplicate_guard_seconds", cfg.Reconcile.DuplicateGuardSeconds,
		"dry_run", cfg.Reconcile.DryRun,
	)

	// Wait for shutdown signal
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
