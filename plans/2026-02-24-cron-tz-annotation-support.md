# CRON_TZ アノテーション対応 & テストワークフロー整備

## Context

ユーザーがテスト用noop workflowを作成中、アノテーション行にインラインコメント `# Note: TZ=Asia/Tokyo` を付けたことで正規表現マッチが破壊される問題が発覚。ワークフロー単位でのTZ指定ニーズがあるため、`CRON_TZ=` プレフィックスによる対応を検討した。

**調査の結果**: `robfig/cron/v3` の `Parser.Parse()` は、カスタム5フィールドパーサーであっても `CRON_TZ=`/`TZ=` プレフィックスを先にstripしてからフィールド解析する（`parser.go:93-103`）。**したがってghacronのGoコードに変更は不要。** 既にサポート済み。

必要なのは: テスト追加、ドキュメント更新、テストワークフロー修正。

## 変更ファイル一覧

| ファイル | 変更種別 | 内容 |
|---------|---------|------|
| `scanner/parser_test.go` | **新規** | ParseAnnotations, HasWorkflowDispatch のユニットテスト |
| `scanner/scanner_test.go` | **新規** | parseFile の統合テスト（CRON_TZ バリデーション通過確認） |
| `github/types.go` | 修正（コメントのみ） | CronExpr フィールドコメントにTZプレフィックス言及追加 |
| `.github/workflows/test-noop.yml` | 修正 | インラインコメント除去 → `CRON_TZ=` 形式に変更 + `run-name` 追加 |
| `README.md` | 修正 | Annotation Format セクションに CRON_TZ ドキュメント追加 |
| `CLAUDE.md` | 修正 | Annotation Format セクションに CRON_TZ 記述追加 |

**変更しないファイル（理由）**:
- `scanner/parser.go` — 正規表現はクォート内全体をキャプチャ済み。変更不要
- `scanner/scanner.go` — robfig/cron/v3 が TZ プレフィックスをハンドル済み。変更不要
- `scheduler/scheduler.go` — `cron.New()` のフルパーサーは元から対応。変更不要
- `scheduler/state.go` — SHA256ハッシュで長さ無関係。変更不要

## 実装手順

### Step 1: `scanner/parser_test.go` 新規作成

`ParseAnnotations()` と `HasWorkflowDispatch()` のユニットテスト。
テストケース:
- 標準5フィールドcron
- `CRON_TZ=Asia/Tokyo` プレフィックス付き
- `TZ=UTC` プレフィックス付き
- シングルクォート
- 複数アノテーション
- アノテーションなし
- インデント付き
- インラインコメント付き（マッチしないことを確認）

### Step 2: `scanner/scanner_test.go` 新規作成

`parseFile()` の統合テスト。`cronParser.Parse()` を通過することを実証。
テストケース:
- CRON_TZ付き式がバリデーション通過
- 不正TZ（`CRON_TZ=Invalid/Zone`）が拒否される
- 通常式が従来通り動作

### Step 3: `github/types.go` コメント修正

```go
// L8
CronExpr string // cron expression (5-field standard format)
↓
CronExpr string // cron expression (5-field format, optional CRON_TZ=/TZ= prefix)
```

### Step 4: `.github/workflows/test-noop.yml` 修正

```yaml
name: Test Noop
run-name: "ghacron noop test (${{ github.event_name }})"

on:
  # ghacron: "CRON_TZ=Asia/Tokyo 45 1 * * 1"
  workflow_dispatch:

jobs:
  noop:
    runs-on: ubuntu-24.04
    steps:
      - run: echo "ghacron dispatched at $(date -u)"
```

### Step 5: `README.md` Annotation Format セクション更新

既存の説明の後に CRON_TZ 指定例とその挙動説明を追加:
- `CRON_TZ=`/`TZ=` プレフィックスでジョブ単位のTZ上書きが可能
- グローバル `reconcile.timezone` よりジョブ単位の指定が優先
- 有効なIANAタイムゾーン名が必要

### Step 6: `CLAUDE.md` Annotation Format セクション更新

CRON_TZ の記述を追加（開発者向けドキュメント）。

## 検証手順

```bash
# テスト実行
go test ./scanner/ -v
go test ./...

# vet
go vet ./...
```

## 設計上の注意点

- `CRON_TZ=` 付きと無しの式は異なる `CronJobKey` → 異なるジョブとして識別される（意図通り）
- Dockerイメージにtzdataが含まれている必要あり（既存の `reconcile.timezone` 機能と同じ前提条件）
- `CRON_TZ=` が推奨、`TZ=` も利用可能（robfig/cron/v3 の仕様）
