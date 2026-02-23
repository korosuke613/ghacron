package github

import (
	"context"
	"encoding/base64"
	"fmt"
	"log/slog"
	"net/http"
	"path/filepath"
	"strings"

	gh "github.com/google/go-github/v68/github"
)

// Client GitHub APIクライアント
type Client struct {
	gh *gh.Client
}

// NewClient 新しいGitHubクライアントを作成
func NewClient(appID int64, privateKeyPEM []byte) (*Client, error) {
	transport, err := NewTransport(appID, privateKeyPEM)
	if err != nil {
		return nil, fmt.Errorf("Transport作成に失敗: %w", err)
	}

	httpClient := &http.Client{Transport: transport}
	ghClient := gh.NewClient(httpClient)

	return &Client{gh: ghClient}, nil
}

// GetInstallationRepos インストール先の全リポジトリを取得
func (c *Client) GetInstallationRepos(ctx context.Context) ([]Repository, error) {
	var repos []Repository
	opts := &gh.ListOptions{PerPage: 100}

	for {
		result, resp, err := c.gh.Apps.ListRepos(ctx, opts)
		if err != nil {
			return nil, fmt.Errorf("リポジトリ一覧の取得に失敗: %w", err)
		}

		for _, r := range result.Repositories {
			repos = append(repos, Repository{
				Owner:         r.GetOwner().GetLogin(),
				Name:          r.GetName(),
				DefaultBranch: r.GetDefaultBranch(),
			})
		}

		if resp.NextPage == 0 {
			break
		}
		opts.Page = resp.NextPage
	}

	return repos, nil
}

// GetWorkflowFiles リポジトリの .github/workflows/ 配下のファイル一覧を取得
func (c *Client) GetWorkflowFiles(ctx context.Context, owner, repo string) ([]WorkflowFile, error) {
	_, dirContent, _, err := c.gh.Repositories.GetContents(
		ctx, owner, repo,
		".github/workflows",
		&gh.RepositoryContentGetOptions{},
	)
	if err != nil {
		// 404 = ワークフローディレクトリが存在しない
		if ghErr, ok := err.(*gh.ErrorResponse); ok && ghErr.Response.StatusCode == 404 {
			return nil, nil
		}
		return nil, fmt.Errorf("ワークフロー一覧の取得に失敗 (%s/%s): %w", owner, repo, err)
	}

	var files []WorkflowFile
	for _, entry := range dirContent {
		name := entry.GetName()
		ext := strings.ToLower(filepath.Ext(name))
		if ext == ".yml" || ext == ".yaml" {
			files = append(files, WorkflowFile{
				Name: name,
				Path: entry.GetPath(),
			})
		}
	}

	return files, nil
}

// GetFileContent ファイルの内容を取得
func (c *Client) GetFileContent(ctx context.Context, owner, repo, path, ref string) (string, error) {
	opts := &gh.RepositoryContentGetOptions{}
	if ref != "" {
		opts.Ref = ref
	}

	fileContent, _, _, err := c.gh.Repositories.GetContents(ctx, owner, repo, path, opts)
	if err != nil {
		return "", fmt.Errorf("ファイル内容の取得に失敗 (%s/%s/%s): %w", owner, repo, path, err)
	}
	if fileContent == nil {
		return "", fmt.Errorf("ファイルが見つかりません: %s/%s/%s", owner, repo, path)
	}

	// Base64デコード
	if fileContent.Content != nil {
		decoded, err := base64.StdEncoding.DecodeString(strings.ReplaceAll(*fileContent.Content, "\n", ""))
		if err != nil {
			// GetContent() を試行
			content, err := fileContent.GetContent()
			if err != nil {
				return "", fmt.Errorf("ファイル内容のデコードに失敗: %w", err)
			}
			return content, nil
		}
		return string(decoded), nil
	}

	content, err := fileContent.GetContent()
	if err != nil {
		return "", fmt.Errorf("ファイル内容の取得に失敗: %w", err)
	}
	return content, nil
}

// DispatchWorkflow workflow_dispatch イベントを発火
func (c *Client) DispatchWorkflow(ctx context.Context, owner, repo, workflowFile, ref string) error {
	resp, err := c.gh.Actions.CreateWorkflowDispatchEventByFileName(
		ctx, owner, repo, workflowFile,
		gh.CreateWorkflowDispatchEventRequest{
			Ref: ref,
		},
	)
	if err != nil {
		if resp != nil {
			return fmt.Errorf("workflow_dispatch発火に失敗 (%s/%s/%s, status=%d): %w",
				owner, repo, workflowFile, resp.StatusCode, err)
		}
		return fmt.Errorf("workflow_dispatch発火に失敗 (%s/%s/%s): %w",
			owner, repo, workflowFile, err)
	}

	slog.Info("workflow_dispatch を発火",
		"owner", owner,
		"repo", repo,
		"workflow_file", workflowFile,
		"ref", ref,
	)
	return nil
}

// GetVariable リポジトリのActions Variableを取得
func (c *Client) GetVariable(ctx context.Context, owner, repo, name string) (string, error) {
	variable, resp, err := c.gh.Actions.GetRepoVariable(ctx, owner, repo, name)
	if err != nil {
		if resp != nil && resp.StatusCode == 404 {
			return "", nil // 変数が存在しない
		}
		return "", fmt.Errorf("Variable取得に失敗 (%s/%s/%s): %w", owner, repo, name, err)
	}
	return variable.Value, nil
}

// SetVariable リポジトリのActions Variableを作成または更新
func (c *Client) SetVariable(ctx context.Context, owner, repo, name, value string) error {
	// まず更新を試みる
	_, err := c.gh.Actions.UpdateRepoVariable(ctx, owner, repo, &gh.ActionsVariable{
		Name:  name,
		Value: value,
	})
	if err != nil {
		// 更新に失敗した場合は作成を試みる
		_, createErr := c.gh.Actions.CreateRepoVariable(ctx, owner, repo, &gh.ActionsVariable{
			Name:  name,
			Value: value,
		})
		if createErr != nil {
			return fmt.Errorf("Variable設定に失敗 (%s/%s/%s): update=%v, create=%v", owner, repo, name, err, createErr)
		}
	}
	return nil
}
