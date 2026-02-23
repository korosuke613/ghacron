# /jobs APIにスキップされたアノテーション情報を追加

## Context

ghacronのアノテーションパースが失敗した場合（不正なcron式、無効なタイムゾーン名など）、現状は `slog.Warn` でログ出力してサイレントにスキップするのみ。APIからスキップ情報を取得する手段がなく、ユーザーがtypoに気づきにくい。

`/jobs` エンドポイントのレスポンスを `{"registered": [...], "skipped": [...]}` に変更し、スキップされたアノテーションの詳細（リポジトリ、ファイル、式、理由）を返す。

**破壊的変更**: `/jobs` のレスポンスが配列からオブジェクトに変わる。

## 変更ファイル一覧

| ファイル | 変更内容 |
|---------|---------|
| `scanner/scanner.go` | `ScanResult` に `Skipped` フィールド追加。`parseFile` でスキップ情報を収集 |
| `scanner/scanner_test.go` | スキップ情報収集のテスト追加 |
| `scheduler/scheduler.go` | `Scheduler` に skipped 状態保持、`StatusProvider` に `GetSkippedAnnotations()` 追加 |
| `scheduler/reconciler.go` | `ScanResult.Skipped` を `Scheduler` に渡す |
| `api/server.go` | `/jobs` レスポンス構造を `{registered, skipped}` に変更 |
| `README.md` | `/jobs` レスポンス例を更新 |

## 実装手順

### Step 1: `scanner/scanner.go` — スキップ情報の型と収集

`SkippedAnnotation` 型を追加し、`ScanResult` に `Skipped` フィールドを追加:

```go
// SkippedAnnotation holds info about an annotation that failed validation.
type SkippedAnnotation struct {
    Owner        string `json:"owner"`
    Repo         string `json:"repo"`
    WorkflowFile string `json:"workflow_file"`
    CronExpr     string `json:"cron_expr"`
    Reason       string `json:"reason"`
}

type ScanResult struct {
    Annotations []github.CronAnnotation
    Skipped     []SkippedAnnotation
}
```

`parseFile` の戻り値を `([]github.CronAnnotation, []SkippedAnnotation)` に変更。
`scanner.go:115` のパース失敗時に `SkippedAnnotation` を生成して返す:

```go
if _, err := s.cronParser.Parse(expr); err != nil {
    slog.Warn("skipping invalid cron expression", ...)
    skipped = append(skipped, SkippedAnnotation{
        Owner: repo.Owner, Repo: repo.Name,
        WorkflowFile: file.Name, CronExpr: expr,
        Reason: err.Error(),
    })
    continue
}
```

`scanRepo` と `ScanAll` も `Skipped` を集約するように修正。

### Step 2: `scanner/scanner_test.go` — スキップ収集テスト

`TestParseFile_InvalidTZ` を拡張し、`parseFile` がスキップ情報を正しく返すことを検証。
→ `parseFile` の戻り値変更に伴い、既存テストも更新。

### Step 3: `scheduler/scheduler.go` — スキップ状態の保持と公開

```go
type Scheduler struct {
    // ...existing fields...
    skippedAnnotations []scanner.SkippedAnnotation  // 追加
}
```

`SetSkippedAnnotations(skipped []scanner.SkippedAnnotation)` メソッド追加。
`StatusProvider` インターフェースに `GetSkippedAnnotations()` 追加:

```go
type StatusProvider interface {
    GetRegisteredJobCount() int
    GetLastReconcileTime() time.Time
    GetJobDetails() []JobDetail
    GetSkippedAnnotations() []scanner.SkippedAnnotation  // 追加
}
```

### Step 4: `scheduler/reconciler.go` — スキップ情報の受け渡し

`Reconcile()` 内で `result.Skipped` を `r.scheduler.SetSkippedAnnotations()` に渡す:

```go
func (r *Reconciler) Reconcile(ctx context.Context) error {
    result, err := r.scanner.ScanAll(ctx)
    if err != nil {
        return err
    }
    r.scheduler.SetSkippedAnnotations(result.Skipped)
    // ...rest unchanged...
}
```

### Step 5: `api/server.go` — `/jobs` レスポンス構造変更

```go
type jobsResponse struct {
    Registered []scheduler.JobDetail          `json:"registered"`
    Skipped    []scanner.SkippedAnnotation    `json:"skipped"`
}
```

`handleJobs` を更新:

```go
func (s *Server) handleJobs(w http.ResponseWriter, r *http.Request) {
    // ...
    resp := jobsResponse{
        Registered: provider.GetJobDetails(),
        Skipped:    provider.GetSkippedAnnotations(),
    }
    if resp.Registered == nil { resp.Registered = []scheduler.JobDetail{} }
    if resp.Skipped == nil { resp.Skipped = []scanner.SkippedAnnotation{} }
    json.NewEncoder(w).Encode(resp)
}
```

### Step 6: `README.md` — `/jobs` レスポンス例更新

```json
{
  "registered": [
    {
      "owner": "myorg",
      "repo": "myrepo",
      "workflow_file": "ci.yml",
      "cron_expr": "0 8 * * *",
      "next_run": "2026-02-25T08:00:00Z"
    }
  ],
  "skipped": []
}
```

## 検証手順

```bash
go test ./scanner/ -v
go test ./...
go vet ./...
```

## 設計上の注意

- `SkippedAnnotation` は scanner パッケージに定義（発生源が scanner のため）
- Reconcile のたびに `skippedAnnotations` は上書き（前回分はクリア）
- スキップが0件の場合も `"skipped": []` を返す（null回避）
- `/jobs` のレスポンス構造変更は破壊的変更。利用者がいる場合は注意
