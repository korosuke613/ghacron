package scanner

import (
	"context"
	"log/slog"

	"github.com/korosuke613/ghacron/github"

	"github.com/robfig/cron/v3"
)

// SkippedAnnotation holds info about an annotation that failed validation.
type SkippedAnnotation struct {
	Owner        string `json:"owner"`
	Repo         string `json:"repo"`
	WorkflowFile string `json:"workflow_file"`
	CronExpr     string `json:"cron_expr"`
	Reason       string `json:"reason"`
}

// ScanResult holds the scan results.
type ScanResult struct {
	Annotations []github.CronAnnotation
	Skipped     []SkippedAnnotation
}

// ScannerClient is the GitHub API interface used by the scanner.
type ScannerClient interface {
	GetInstallationRepos(ctx context.Context) ([]github.Repository, error)
	GetWorkflowFiles(ctx context.Context, owner, repo string) ([]github.WorkflowFile, error)
	GetFileContent(ctx context.Context, owner, repo, path, ref string) (string, error)
}

// Scanner scans repositories for cron annotations.
type Scanner struct {
	client     ScannerClient
	cronParser cron.Parser
}

// New creates a new Scanner.
func New(client ScannerClient) *Scanner {
	return &Scanner{
		client:     client,
		cronParser: cron.NewParser(cron.Minute | cron.Hour | cron.Dom | cron.Month | cron.Dow),
	}
}

// ScanAll scans all installation repositories and collects annotations.
func (s *Scanner) ScanAll(ctx context.Context) (*ScanResult, error) {
	repos, err := s.client.GetInstallationRepos(ctx)
	if err != nil {
		return nil, err
	}

	slog.Info("scanning repositories", "repo_count", len(repos))

	result := &ScanResult{}

	for _, repo := range repos {
		annotations, skipped, err := s.scanRepo(ctx, repo)
		if err != nil {
			slog.Error("failed to scan repository",
				"owner", repo.Owner,
				"repo", repo.Name,
				"error", err,
			)
			continue
		}
		result.Annotations = append(result.Annotations, annotations...)
		result.Skipped = append(result.Skipped, skipped...)
	}

	slog.Info("scan completed",
		"annotation_count", len(result.Annotations),
		"skipped_count", len(result.Skipped),
	)
	return result, nil
}

// scanRepo scans workflow files in a single repository.
func (s *Scanner) scanRepo(ctx context.Context, repo github.Repository) ([]github.CronAnnotation, []SkippedAnnotation, error) {
	files, err := s.client.GetWorkflowFiles(ctx, repo.Owner, repo.Name)
	if err != nil {
		return nil, nil, err
	}

	if len(files) == 0 {
		return nil, nil, nil
	}

	var annotations []github.CronAnnotation
	var skipped []SkippedAnnotation

	for _, file := range files {
		content, err := s.client.GetFileContent(ctx, repo.Owner, repo.Name, file.Path, repo.DefaultBranch)
		if err != nil {
			slog.Error("failed to read file",
				"owner", repo.Owner,
				"repo", repo.Name,
				"path", file.Path,
				"error", err,
			)
			continue
		}

		fileAnnotations, fileSkipped := s.parseFile(repo, file, content)
		annotations = append(annotations, fileAnnotations...)
		skipped = append(skipped, fileSkipped...)
	}

	return annotations, skipped, nil
}

// parseFile parses a workflow file and extracts cron annotations.
func (s *Scanner) parseFile(repo github.Repository, file github.WorkflowFile, content string) ([]github.CronAnnotation, []SkippedAnnotation) {
	// Check if workflow_dispatch is in the on: trigger
	if !HasWorkflowDispatch(content) {
		return nil, nil
	}

	// Extract annotations
	cronExprs := ParseAnnotations(content)
	if len(cronExprs) == 0 {
		return nil, nil
	}

	var annotations []github.CronAnnotation
	var skipped []SkippedAnnotation

	for _, expr := range cronExprs {
		// Validate cron expression
		if _, err := s.cronParser.Parse(expr); err != nil {
			slog.Warn("skipping invalid cron expression",
				"owner", repo.Owner,
				"repo", repo.Name,
				"workflow_file", file.Name,
				"cron_expr", expr,
				"error", err,
			)
			skipped = append(skipped, SkippedAnnotation{
				Owner:        repo.Owner,
				Repo:         repo.Name,
				WorkflowFile: file.Name,
				CronExpr:     expr,
				Reason:       err.Error(),
			})
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

	return annotations, skipped
}
