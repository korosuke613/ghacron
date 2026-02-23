# 構造化ログ移行（`log` → `log/slog`）

## Context

ghacron は Go 標準の `log` パッケージでプレーンテキストログを stderr に出力している。
Grafana/Loki でのクエリ・フィルタリングに対応するため、`log/slog`（Go 1.21+ 標準）による JSON 構造化ログに移行する。
外部依存の追加は不要。`config.yaml` の `log.level` が定義済みだが未使用のため、これも機能させる。

## アプローチ

- **`slog.SetDefault()` + パッケージ直接使用（ラッパーなし）**
- `main.go` で `slog.SetDefault()` を一度呼び、全パッケージから `slog.Info()` / `slog.Error()` 等を直接使用
- JSON ハンドラで stdout に出力（k8s ログ収集の標準パターン）
- `log.Fatalf` → `slog.Error` + `os.Exit(1)`（設定読み込み前のみ `fmt.Fprintf(os.Stderr, ...)` + `os.Exit(1)`）

## 変更対象ファイル

### 1. `config/config.go` — LogConfig 拡張

- `LogConfig` に `Format string` フィールド追加（`"json"` or `"text"`、デフォルト `"json"`）
- `SlogLevel() slog.Level` メソッド追加（Level 文字列 → slog.Level 変換）
- `validate()` にログ設定バリデーション追加（level: debug/info/warn/error、format: json/text）

### 2. `config/config.yaml` — format 追加

```yaml
log:
  level: "info"
  format: "json"
```

### 3. `main.go` — slog 初期化 + 全ログ移行

- `init()` 削除（`log.SetFlags` 不要に）
- `initLogger(logCfg *config.LogConfig)` 関数追加
  - `config.Load` 後に呼び出し
  - Format に応じて `slog.NewJSONHandler` or `slog.NewTextHandler` を選択
  - Level は `LogConfig.SlogLevel()` で取得
- import `"log"` → `"log/slog"` + `"fmt"` + `"os"`
- `log.Fatalf` x5 → `slog.Error` + `os.Exit(1)`（config.Load 失敗のみ `fmt.Fprintf`）
- `log.Printf` / `log.Println` → `slog.Info` + 構造化フィールド

### 4. `scheduler/scheduler.go` — 全15箇所移行

- import `"log"` → `"log/slog"`
- ジョブハンドラ内: `owner`, `repo`, `workflow_file`, `cron_expr` を構造化フィールドに
- Reconciliation: `duration`, `registered_jobs` を構造化フィールドに
- 重複ガード: `elapsed`, `guard` を構造化フィールドに

### 5. `scheduler/reconciler.go` — 2箇所移行

- import `"log"` → `"log/slog"`
- Reconcile 結果: `added`, `removed`, `desired_total` を構造化フィールドに

### 6. `scanner/scanner.go` — 5箇所移行

- import `"log"` → `"log/slog"`
- スキャン: `repo_count`, `annotation_count`, `owner`, `repo`, `path` を構造化フィールドに
- 不正 cron 式: `slog.Warn` に昇格

### 7. `api/server.go` — 4箇所移行

- import `"log"` → `"log/slog"`
- サーバー開始: `addr` を構造化フィールドに

### 8. `github/client.go` — 1箇所移行

- import `"log"` → `"log/slog"`
- dispatch: `owner`, `repo`, `workflow_file`, `ref` を構造化フィールドに

## 構造化フィールド設計（主要キー）

| キー | 型 | 用途 |
|---|---|---|
| `"error"` | error | 全エラーログ |
| `"owner"` | string | GitHub リポジトリオーナー |
| `"repo"` | string | GitHub リポジトリ名 |
| `"workflow_file"` | string | ワークフローファイル名 |
| `"cron_expr"` | string | cron 式 |
| `"version"` | string | アプリバージョン |
| `"duration"` | string | 処理時間 |
| `"dry_run"` | bool | DryRun フラグ |

## JSON 出力例

```json
{"time":"2026-02-24T09:00:00.123Z","level":"INFO","msg":"cronジョブを登録","owner":"korosuke613","repo":"ghacron","workflow_file":"ci.yml","cron_expr":"0 9 * * *"}
{"time":"2026-02-24T09:00:05.456Z","level":"ERROR","msg":"dispatch に失敗","owner":"korosuke613","repo":"test-repo","workflow_file":"build.yml","error":"API error: 503"}
```

## 検証

```bash
# 1. コンパイル確認
go build -o ghacron main.go

# 2. "log" パッケージが完全除去されたことを確認
grep -r '"log"' --include='*.go' .
# → 結果が空（"log/slog" のみ残る）

# 3. テスト
go test ./...

# 4. JSON 出力確認（手動）
# config.yaml の dry_run: true で起動し、stdout が JSON 行であることを確認

# 5. ログレベルフィルタリング確認
# log.level: "error" に変更 → Info ログが抑制されること
```
