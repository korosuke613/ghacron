# configファイル廃止・環境変数一本化

## Context

現在の ghacron は YAML configファイル + 一部環境変数 のハイブリッド構成だが、以下の問題がある：

- 環境変数オーバーライドが `app_id` と `private_key` にしか存在しない
- `app_id` は YAML優先、`private_key` は env優先と、優先順位が不統一
- デフォルト値が `validate()` 内に散在し、省略可否がコードを読まないと分からない
- 全パラメータがスカラー値12個で、YAML configファイルはオーバーキル

**方針**: configファイルを廃止し、全パラメータを `GHACRON_` プレフィックスの環境変数で統一する。

## 環境変数一覧

| 環境変数 | 型 | デフォルト | 必須 | 説明 |
|---|---|---|---|---|
| `GHACRON_APP_ID` | int64 | — | Yes | GitHub App ID |
| `GHACRON_PRIVATE_KEY` | string | — | Yes* | GitHub App Private Key (PEM) |
| `GHACRON_PRIVATE_KEY_PATH` | string | — | Yes* | Private Key ファイルパス |
| `GHACRON_INTERVAL_MINUTES` | int | `5` | No | Reconcileループ間隔(分) |
| `GHACRON_DUPLICATE_GUARD_SECONDS` | int | `60` | No | 重複dispatch防止の秒数 |
| `GHACRON_DRY_RUN` | bool | `false` | No | ドライランモード |
| `GHACRON_TIMEZONE` | string | `UTC` | No | cronスケジュール評価のIANAタイムゾーン |
| `GHACRON_LOG_LEVEL` | string | `info` | No | ログレベル (debug/info/warn/error) |
| `GHACRON_LOG_FORMAT` | string | `json` | No | ログフォーマット (json/text) |
| `GHACRON_WEBAPI_ENABLED` | bool | `true` | No | Web APIサーバーの有効/無効 |
| `GHACRON_WEBAPI_HOST` | string | `0.0.0.0` | No | Web APIリッスンホスト |
| `GHACRON_WEBAPI_PORT` | int | `8080` | No | Web APIリッスンポート |

*`GHACRON_PRIVATE_KEY` または `GHACRON_PRIVATE_KEY_PATH` のいずれかが必須。両方設定時は `GHACRON_PRIVATE_KEY` 優先。

## 変更ファイル

### 1. `config/config.go` — 環境変数読み取りに全面書き換え

- `rawConfig` 構造体を削除
- `Load(configPath string)` → `Load()` に変更（引数なし）
- YAML パース (`yaml.Unmarshal`, `os.ExpandEnv`) を全て削除
- 全パラメータを `os.Getenv` + ヘルパー関数で読み取る
- デフォルト値を `Load()` 冒頭で宣言的に設定してから env で上書き
- `GetDefaultConfigPath()` を削除
- `GetPrivateKey()` を簡素化: `GHACRON_PRIVATE_KEY` env > `GHACRON_PRIVATE_KEY_PATH` ファイル
- YAML struct tags (`yaml:"..."`) を削除
- `import "gopkg.in/yaml.v3"` を削除

ヘルパー関数（config.go 内にプライベートで追加）：
```go
func envStr(key, fallback string) string
func envInt(key string, fallback int) (int, error)
func envInt64(key string, fallback int64) (int64, error)
func envBool(key string, fallback bool) (bool, error)
```

### 2. `main.go` — CLI フラグ簡素化

- `-config` フラグを削除
- `config.Load(*configPath)` → `config.Load()` に変更
- `config.GetDefaultConfigPath()` の参照を削除

### 3. `config/config_test.go` — 新規作成

テーブルドリブンテストで以下をカバー：
- デフォルト値の確認（環境変数未設定時）
- 各環境変数の読み取り
- `GHACRON_APP_ID` 未設定時のエラー
- `GHACRON_PRIVATE_KEY` / `GHACRON_PRIVATE_KEY_PATH` 両方未設定時のエラー
- 不正値（非数値の APP_ID, 不正な timezone 等）のバリデーションエラー
- `GetPrivateKey()` の優先順位（env > file）
- bool パース（"true", "false", "1", "0"）

`t.Setenv()` を使用してテスト間の環境変数汚染を防止。

### 4. `go.mod` / `go.sum` — yaml.v3 依存削除

`gopkg.in/yaml.v3` は `config/config.go` でのみ使用。`go mod tidy` で除去。

### 5. `README.md` — Configuration セクション書き換え

- YAML configファイルの説明を削除
- 環境変数リファレンス表を追加（上記の表と同等）
- Usage の例を環境変数のみに更新
- `-config` フラグの記述を削除
- `GHACRON_CONFIG` 環境変数の記述を削除
- Docker / Kubernetes の例を `GHACRON_*` に更新
- `/config` エンドポイントのレスポンス例に `timezone` を追加

### 6. `api/server.go` — `/config` レスポンスに timezone 追加

`configReconcile` 構造体に `Timezone string` フィールドを追加。現状欠落している。

### 7. `CLAUDE.md` — 参照更新

- 必須環境変数セクションを `GHACRON_*` に更新
- configファイル関連の記述を削除
- Startup Flow の記述を更新

## 破壊的変更

この変更は破壊的変更（breaking change）：
- configファイル (`ghacron.yaml`) のサポート廃止
- `-config` CLI フラグの廃止
- `GHACRON_CONFIG` 環境変数の廃止
- `GH_APP_ID` → `GHACRON_APP_ID` へのリネーム
- `GH_APP_PRIVATE_KEY` → `GHACRON_PRIVATE_KEY` へのリネーム

コミットメッセージは `feat!:` プレフィックスを使用する。

## 検証手順

```bash
# 1. ビルド確認
go build -o ghacron main.go

# 2. テスト実行
go test ./...

# 3. vet
go vet ./...

# 4. 依存確認（yaml.v3 が消えていること）
grep yaml go.mod  # 結果なしを確認

# 5. 起動確認（dry-run）
GHACRON_APP_ID=123456 GHACRON_PRIVATE_KEY="dummy" GHACRON_DRY_RUN=true ./ghacron

# 6. /config エンドポイント確認
curl http://localhost:8080/config | jq .
# → timezone フィールドが含まれることを確認
```
