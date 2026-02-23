package github

// CronAnnotation ワークフローファイルから抽出されたcronアノテーション
type CronAnnotation struct {
	Owner        string // リポジトリオーナー
	Repo         string // リポジトリ名
	WorkflowFile string // ワークフローファイル名（例: "build.yml"）
	CronExpr     string // cron式（5フィールド標準形式）
	Ref          string // デフォルトブランチ
}

// CronJobKey cronジョブの一意識別キー
type CronJobKey struct {
	Owner        string
	Repo         string
	WorkflowFile string
	CronExpr     string
}

// Key CronAnnotationからCronJobKeyを生成
func (a *CronAnnotation) Key() CronJobKey {
	return CronJobKey{
		Owner:        a.Owner,
		Repo:         a.Repo,
		WorkflowFile: a.WorkflowFile,
		CronExpr:     a.CronExpr,
	}
}

// Repository GitHub Appインストール先リポジトリ情報
type Repository struct {
	Owner         string
	Name          string
	DefaultBranch string
}

// WorkflowFile ワークフローファイル情報
type WorkflowFile struct {
	Name string // ファイル名（例: "build.yml"）
	Path string // フルパス（例: ".github/workflows/build.yml"）
}
