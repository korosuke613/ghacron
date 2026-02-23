package scanner

import (
	"context"
	"log"

	"github.com/korosuke613/ghacron/github"

	"github.com/robfig/cron/v3"
)

// ScanResult スキャン結果
type ScanResult struct {
	Annotations []github.CronAnnotation
}

// ScannerClient スキャナーが使用するGitHub APIインターフェース
type ScannerClient interface {
	GetInstallationRepos(ctx context.Context) ([]github.Repository, error)
	GetWorkflowFiles(ctx context.Context, owner, repo string) ([]github.WorkflowFile, error)
	GetFileContent(ctx context.Context, owner, repo, path, ref string) (string, error)
}

// Scanner リポジトリ横断スキャナー
type Scanner struct {
	client     ScannerClient
	cronParser cron.Parser
}

// New 新しいスキャナーを作成
func New(client ScannerClient) *Scanner {
	return &Scanner{
		client:     client,
		cronParser: cron.NewParser(cron.Minute | cron.Hour | cron.Dom | cron.Month | cron.Dow),
	}
}

// ScanAll 全インストール先リポジトリをスキャンしてアノテーションを収集
func (s *Scanner) ScanAll(ctx context.Context) (*ScanResult, error) {
	repos, err := s.client.GetInstallationRepos(ctx)
	if err != nil {
		return nil, err
	}

	log.Printf("スキャン対象: %d リポジトリ", len(repos))

	result := &ScanResult{}

	for _, repo := range repos {
		annotations, err := s.scanRepo(ctx, repo)
		if err != nil {
			log.Printf("リポジトリスキャンに失敗 (%s/%s): %v", repo.Owner, repo.Name, err)
			continue
		}
		result.Annotations = append(result.Annotations, annotations...)
	}

	log.Printf("スキャン完了: %d アノテーション検出", len(result.Annotations))
	return result, nil
}

// scanRepo 単一リポジトリのワークフローファイルをスキャン
func (s *Scanner) scanRepo(ctx context.Context, repo github.Repository) ([]github.CronAnnotation, error) {
	files, err := s.client.GetWorkflowFiles(ctx, repo.Owner, repo.Name)
	if err != nil {
		return nil, err
	}

	if len(files) == 0 {
		return nil, nil
	}

	var annotations []github.CronAnnotation

	for _, file := range files {
		content, err := s.client.GetFileContent(ctx, repo.Owner, repo.Name, file.Path, repo.DefaultBranch)
		if err != nil {
			log.Printf("ファイル読取に失敗 (%s/%s/%s): %v", repo.Owner, repo.Name, file.Path, err)
			continue
		}

		fileAnnotations := s.parseFile(repo, file, content)
		annotations = append(annotations, fileAnnotations...)
	}

	return annotations, nil
}

// parseFile ワークフローファイルを解析してアノテーションを抽出
func (s *Scanner) parseFile(repo github.Repository, file github.WorkflowFile, content string) []github.CronAnnotation {
	// workflow_dispatch が on: に含まれているかチェック
	if !HasWorkflowDispatch(content) {
		return nil
	}

	// アノテーションを抽出
	cronExprs := ParseAnnotations(content)
	if len(cronExprs) == 0 {
		return nil
	}

	var annotations []github.CronAnnotation

	for _, expr := range cronExprs {
		// cron式の妥当性チェック
		if _, err := s.cronParser.Parse(expr); err != nil {
			log.Printf("不正なcron式をスキップ (%s/%s/%s): %q: %v",
				repo.Owner, repo.Name, file.Name, expr, err)
			continue
		}

		annotations = append(annotations, github.CronAnnotation{
			Owner:        repo.Owner,
			Repo:         repo.Name,
			WorkflowFile: file.Name,
			CronExpr:     expr,
			Ref:          repo.DefaultBranch,
		})
	}

	return annotations
}
