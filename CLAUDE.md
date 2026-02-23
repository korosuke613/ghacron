# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

コミットメッセージ、プルリクエストは英語で書くこと。

## Language Rules

- `.go`ファイル内の文字列（ログメッセージ、エラーメッセージ、コメント）はすべて英語で記述すること
- CLAUDE.md、`plans/`、`.claude/` 配下のファイルは日本語OK（ランタイムに影響しないため）

## Commit Convention

コミットメッセージは [Conventional Commits](https://www.conventionalcommits.org/) に従うこと。

```
<type>(<scope>): <description>

[optional body]

[optional footer(s)]
```

- **type**: `feat`, `fix`, `docs`, `refactor`, `test`, `ci`, `chore`, `perf`, `build`
- **scope**: 使用しない
- **description**: 変更の要約（英語、小文字始まり、末尾ピリオドなし）
- **breaking change**: 破壊的変更がある場合は `!` を付与（例: `feat!: rename annotation syntax`）
  - デフォルト値の変更、設定キーのリネーム、APIレスポンス構造の変更など、既存ユーザーが同じ設定のままアップグレードしたときに挙動が変わるものはすべて破壊的変更

## Common Commands

```bash
# Build
go build -o ghacron main.go

# Production build (version injection)
go build -ldflags="-s -w -X main.version=$(git describe --tags --always)" -o ghacron main.go

# Run (requires GitHub App credentials)
GHACRON_APP_ID=123456 GHACRON_APP_PRIVATE_KEY="$(cat key.pem)" go run main.go

# Test all
go test ./...

# Test single package
go test ./scheduler/

# Test single function
go test ./scheduler/ -run TestHandler_NormalDispatch

# Vet
go vet ./...

# Dependencies
go mod tidy

# Docker build
docker build --build-arg VERSION=dev -t ghacron .
```

### API Testing (requires running instance)

```bash
curl http://localhost:8080/healthz
curl http://localhost:8080/status
curl http://localhost:8080/jobs
curl http://localhost:8080/config
```

## Architecture Overview

GitHub Actions の `schedule` イベントの遅延問題を解決するGoスタンドアロンサービス。ワークフローファイル内の `# ghacron: "cron_expr"` アノテーションを読み取り、`workflow_dispatch` を正確な時刻に発火する。

### Startup Flow

```
main.go: -version flag → bootstrap slog (JSON) → config.Load (env vars)
  → re-init slog → github.NewClient (App JWT auth) → scheduler.New
  → api.NewServer → reconcile loop (5min ticker, immediate first run)
  → signal wait → graceful shutdown
```

### Reconciliation Loop (core mechanism)

`scheduler/reconciler.go` が5分間隔でリポジトリをスキャンし、desired state（アノテーション）と actual state（登録済みcronジョブ）の差分を取って追加/削除する。Kubernetesのコントローラーパターンに類似。

```
Reconcile() → scanner.ScanAll() → diff(desired, actual) → AddJob / RemoveJob
```

### Core Packages

| Package | 責務 |
|---------|------|
| `config/` | `GHACRON_*` 環境変数による設定管理 |
| `github/` | GitHub App認証（自作JWT RS256 + Installation Tokenキャッシュ）、go-github/v68ラッパー |
| `scanner/` | ワークフローファイルスキャン。正規表現でアノテーション抽出、`workflow_dispatch`存在チェック |
| `scheduler/` | robfig/cron/v3によるcronジョブ管理、Reconciler、状態管理（GitHub Actions Variables） |
| `api/` | HTTP監視エンドポイント（`/healthz`, `/status`, `/jobs`, `/config`）。k8s probes用 |

### Key Design Decisions

- **CronJobKey** = `{Owner, Repo, WorkflowFile, CronExpr}` の4つ組で一意識別
- **5フィールド標準cron**（`robfig/cron/v3`、WithSecondsなし）
- **重複dispatch防止**: GitHub Actions Variables に前回dispatch時刻をRFC3339で永続化。変数名は `GHACRON_LAST_<SHA256先頭8hex>`
- **Fail-open**: 状態取得失敗時はdispatchを続行（可用性優先）
- **Dispatch rollback**: dispatch失敗時はpre-saveした時刻を前回値にロールバック
- **外部DB不要**: 永続化はすべてGitHub Actions Variables経由

### Annotation Format

```yaml
on:
  # ghacron: "0 8 * * *"
  workflow_dispatch:
```

`workflow_dispatch` が `on:` セクション内に存在し、`# ghacron: "cron_expr"` コメントがファイル内にあることが必要条件。1ファイル複数アノテーション可。

ジョブ単位のタイムゾーン指定（`CRON_TZ=`/`TZ=` プレフィックス）:
```yaml
  # ghacron: "CRON_TZ=Asia/Tokyo 0 8 * * *"
```
グローバル `reconcile.timezone` よりジョブ単位の指定が優先。`robfig/cron/v3` の組み込みTZプレフィックス機能を利用。

### Configuration

全パラメータを `GHACRON_*` プレフィックスの環境変数で設定。configファイル不要。

必須環境変数:
- `GHACRON_APP_ID`: GitHub App ID
- `GHACRON_APP_PRIVATE_KEY` または `GHACRON_APP_PRIVATE_KEY_PATH`: GitHub App Private Key

### Test Structure

テストは `scheduler/scheduler_test.go` に集中。mockクライアントパターンで外部依存なし。
主要テストケース: 正常dispatch、dispatch失敗ロールバック、重複ガード、dry-run、fail-open動作。
