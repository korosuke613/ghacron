# CLAUDE.md

This file provides guidance to Claude Code when working with this application.

## Common Commands

### Development
```bash
# Build
go build -o gh-cron-trigger main.go

# Run
GH_APP_ID=123456 GH_APP_PRIVATE_KEY="$(cat key.pem)" go run main.go

# Dependencies
go mod tidy

# Test
go test ./...
```

### Production Build
```bash
go build -ldflags="-s -w" -o gh-cron-trigger main.go
```

### API Testing
```bash
# Health check
curl http://localhost:8080/healthz

# Status
curl http://localhost:8080/status

# Job list
curl http://localhost:8080/jobs
```

## Architecture Overview

Go製のスタンドアロンサービス。GitHub Actions schedule の遅延問題を解決するため、ワークフローアノテーションを読み取り workflow_dispatch を時間通りに発火する。

### Core Components

- `config/`: YAML設定管理（os.ExpandEnvで環境変数展開）
- `github/`: GitHub App認証（自作JWT + Installation Token）、APIクライアント（go-github/v68）
- `scanner/`: ワークフローファイルスキャン、アノテーション抽出（正規表現）
- `scheduler/`: cronジョブ管理（robfig/cron/v3）、Reconciliationループ、状態管理（GitHub Actions Variables）
- `api/`: ヘルス/ステータスAPI（k8s probes用）

### Key Technical Details

- GitHub App認証は自作（crypto/rsa + JWT RS256 + Installation Token キャッシュ）
- 5分間隔のReconciliationでcronジョブを動的に追加/削除
- 重複dispatch防止: GitHub Actions Variables に前回dispatch時刻を永続化
- CronJobKey = {Owner, Repo, WorkflowFile, CronExpr} の4つ組で一意識別
- 5フィールド標準cron（WithSecondsなし）

### Configuration Requirements

- `GH_APP_ID`: GitHub App ID（環境変数）
- `GH_APP_PRIVATE_KEY`: GitHub App Private Key PEM（環境変数）
