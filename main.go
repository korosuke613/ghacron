package main

import (
	"context"
	"flag"
	"fmt"
	"log"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/korosuke613/ghacron/api"
	"github.com/korosuke613/ghacron/config"
	"github.com/korosuke613/ghacron/github"
	"github.com/korosuke613/ghacron/scheduler"
)

const Version = "0.1.0"

func main() {
	var (
		configPath  = flag.String("config", config.GetDefaultConfigPath(), "設定ファイルのパス")
		showVersion = flag.Bool("version", false, "バージョンを表示")
	)
	flag.Parse()

	if *showVersion {
		fmt.Printf("ghacron v%s\n", Version)
		return
	}

	// 設定ファイルを読み込み
	cfg, err := config.Load(*configPath)
	if err != nil {
		log.Fatalf("設定ファイルの読み込みに失敗: %v", err)
	}

	log.Printf("ghacron v%s を開始します...", Version)

	// GitHub App クライアントを初期化
	privateKey, err := cfg.GetPrivateKey()
	if err != nil {
		log.Fatalf("秘密鍵の取得に失敗: %v", err)
	}

	ghClient, err := github.NewClient(cfg.GitHub.AppID, privateKey)
	if err != nil {
		log.Fatalf("GitHubクライアントの初期化に失敗: %v", err)
	}

	// タイムゾーンを読み込み
	loc, err := time.LoadLocation(cfg.Reconcile.Timezone)
	if err != nil {
		log.Fatalf("タイムゾーンの読み込みに失敗: %v", err)
	}

	// スケジューラーを初期化
	sched := scheduler.New(ghClient, &cfg.Reconcile, loc)

	// APIサーバーを初期化・開始
	apiServer := api.NewServer(&cfg.WebAPI, cfg)
	apiServer.SetStatusProvider(sched)
	if err := apiServer.Start(); err != nil {
		log.Fatalf("APIサーバーの開始に失敗: %v", err)
	}

	// Reconciliationループを開始
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	go sched.RunReconcileLoop(ctx, time.Duration(cfg.Reconcile.IntervalMinutes)*time.Minute)

	log.Printf("設定: reconcile間隔=%d分, 重複ガード=%d秒, dry_run=%v",
		cfg.Reconcile.IntervalMinutes, cfg.Reconcile.DuplicateGuardSeconds, cfg.Reconcile.DryRun)
	log.Println("ghacron が開始されました。Ctrl+C で停止できます。")

	// シグナルハンドリング
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM)

	sig := <-sigChan
	log.Printf("シグナル %v を受信。サービスを停止します...", sig)

	cancel()
	sched.Stop()
	apiServer.Stop()

	log.Println("ghacron を停止しました")
}

func init() {
	log.SetFlags(log.LstdFlags | log.Lshortfile)
}
