# タイムゾーン設定可能化

## Context

`main.go:init()` で `time.Local = time.LoadLocation("Asia/Tokyo")` とグローバル状態を汚染している。
これは以下の問題を持つ:
- ハードコードされており設定変更不可
- プロセス全体の `time.Local` を書き換えるため副作用が広範囲
- テストで異なるタイムゾーンを検証不可

`robfig/cron/v3` は `cron.WithLocation(*time.Location)` を提供しており、スケジューラに直接注入可能。

## 変更方針

### 1. `config/config.go` — `ReconcileConfig` に `Timezone` フィールド追加

```go
type ReconcileConfig struct {
    IntervalMinutes       int    `yaml:"interval_minutes"`
    DuplicateGuardSeconds int    `yaml:"duplicate_guard_seconds"`
    DryRun                bool   `yaml:"dry_run"`
    Timezone              string `yaml:"timezone"`
}
```

- `validate()` でデフォルト値 `"UTC"` をセット（空文字の場合）
- `validate()` で `time.LoadLocation(c.Reconcile.Timezone)` を呼び有効性を検証

### 2. `scheduler/scheduler.go` — `New()` に `*time.Location` を引数追加

```go
func New(client GitHubClient, cfg *config.ReconcileConfig, loc *time.Location) *Scheduler {
    c := cron.New(cron.WithLocation(loc))
    ...
}
```

### 3. `main.go` — `init()` からタイムゾーン設定を削除、`main()` で Location を生成して注入

```go
// init() から time.Local 設定を削除
func init() {
    log.SetFlags(log.LstdFlags | log.Lshortfile)
}

// main() 内
loc, err := time.LoadLocation(cfg.Reconcile.Timezone)
if err != nil {
    log.Fatalf("タイムゾーンの読み込みに失敗: %v", err)
}
sched := scheduler.New(ghClient, &cfg.Reconcile, loc)
```

### 4. `scheduler/scheduler_test.go` — `newTestScheduler` に `time.UTC` を渡す

### 5. `config/config.yaml` — `timezone` フィールド追加

```yaml
reconcile:
  interval_minutes: 5
  duplicate_guard_seconds: 60
  dry_run: false
  timezone: "UTC"
```

### 6. `README.md` — Configuration セクションに `timezone` を反映

## 変更対象ファイル

- `config/config.go` (ReconcileConfig, validate)
- `config/config.yaml`
- `main.go` (init削除, main修正)
- `scheduler/scheduler.go` (New関数シグネチャ)
- `scheduler/scheduler_test.go` (newTestScheduler)
- `README.md` (Configuration セクション)

## 検証

```bash
go build -o gh-cron-trigger main.go
go test ./...
```
