# ghacron

GitHub Actions の `schedule` イベント（cron トリガー）は最大1時間以上の遅延が発生する既知の問題がある。ghacron は、ワークフローファイル内のアノテーションを読み取り、時間通りに `workflow_dispatch` を発火する Go 製サービス。

## 仕組み

- ワークフローファイルに `# gh-cron-trigger: "0 8 * * *"` のようなアノテーションを記述
- サービスが5分間隔でリポジトリをスキャンし、アノテーションを検出
- cron式に従って `workflow_dispatch` を発火
- 状態管理は GitHub Actions Variables で永続化（PVC不要）

## アノテーション形式

```yaml
on:
  # gh-cron-trigger: "0 8 * * *"
  workflow_dispatch:
```

- `workflow_dispatch:` が `on:` に含まれていることが必須
- 1ファイルに複数アノテーション可
- cron式は標準5フィールド形式（分 時 日 月 曜日）

## 必要な環境

- Go 1.25以上
- GitHub App（App ID + Private Key）
  - 必要な権限: `contents: read`, `actions: write`, `variables: write`, `metadata: read`

## 設定

`config/config.yaml` で設定:

```yaml
github:
  app_id: ${GH_APP_ID}
  private_key: "${GH_APP_PRIVATE_KEY}"

reconcile:
  interval_minutes: 5
  duplicate_guard_seconds: 60
  dry_run: false

log:
  level: "info"

webapi:
  enabled: true
  host: "0.0.0.0"
  port: 8080
```

## 開発コマンド

```bash
# ビルド
go build -o gh-cron-trigger main.go

# 実行（dry-run）
GH_APP_ID=123456 GH_APP_PRIVATE_KEY="$(cat key.pem)" ./gh-cron-trigger

# テスト
go test ./...
```

## Docker

```bash
# ビルド
docker build -t ghacron .

# 実行
docker run -e GH_APP_ID=123456 -e GH_APP_PRIVATE_KEY="$(cat key.pem)" ghacron
```

## Kubernetes デプロイ

```yaml
containers:
- name: ghacron
  image: ghcr.io/korosuke613/ghacron:latest
  env:
  - name: GH_APP_ID
    value: "123456"
  - name: GH_APP_PRIVATE_KEY
    valueFrom:
      secretKeyRef:
        name: ghacron-secrets
        key: private-key
```

## API エンドポイント

| エンドポイント | メソッド | 説明 |
|---------------|---------|------|
| `/healthz` | GET | ヘルスチェック |
| `/status` | GET | サービス状態（登録cronジョブ数等） |
| `/jobs` | GET | 登録済みcronジョブ一覧 |
| `/config` | GET | 公開設定情報 |

## アーキテクチャ

```
ghacron/
├── main.go              # エントリポイント
├── config/              # 設定管理
├── github/              # GitHub App認証・APIクライアント
├── scanner/             # ワークフロースキャン・アノテーション解析
├── scheduler/           # cronジョブ管理・Reconciliation
├── api/                 # ヘルス/ステータスAPI
├── Dockerfile
└── README.md
```

## 参考

- [cronium](https://zenn.dev/cybozu_ept/articles/run-github-actions-scheduled-workflows-on-time) — 同様のアプローチの先行事例

## ライセンス

MIT License
