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

// Client is a GitHub API client.
type Client struct {
	gh *gh.Client
}

// NewClient creates a new GitHub client with App authentication.
func NewClient(appID int64, privateKeyPEM []byte) (*Client, error) {
	transport, err := NewTransport(appID, privateKeyPEM)
	if err != nil {
		return nil, fmt.Errorf("failed to create transport: %w", err)
	}

	httpClient := &http.Client{Transport: transport}
	ghClient := gh.NewClient(httpClient)

	return &Client{gh: ghClient}, nil
}

// GetInstallationRepos returns all repositories accessible to the installation.
func (c *Client) GetInstallationRepos(ctx context.Context) ([]Repository, error) {
	var repos []Repository
	opts := &gh.ListOptions{PerPage: 100}

	for {
		result, resp, err := c.gh.Apps.ListRepos(ctx, opts)
		if err != nil {
			return nil, fmt.Errorf("failed to list repositories: %w", err)
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

// GetWorkflowFiles returns workflow files under .github/workflows/.
func (c *Client) GetWorkflowFiles(ctx context.Context, owner, repo string) ([]WorkflowFile, error) {
	_, dirContent, _, err := c.gh.Repositories.GetContents(
		ctx, owner, repo,
		".github/workflows",
		&gh.RepositoryContentGetOptions{},
	)
	if err != nil {
		// 404 = workflows directory does not exist
		if ghErr, ok := err.(*gh.ErrorResponse); ok && ghErr.Response.StatusCode == 404 {
			return nil, nil
		}
		return nil, fmt.Errorf("failed to list workflows (%s/%s): %w", owner, repo, err)
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

// GetFileContent returns the content of a file in a repository.
func (c *Client) GetFileContent(ctx context.Context, owner, repo, path, ref string) (string, error) {
	opts := &gh.RepositoryContentGetOptions{}
	if ref != "" {
		opts.Ref = ref
	}

	fileContent, _, _, err := c.gh.Repositories.GetContents(ctx, owner, repo, path, opts)
	if err != nil {
		return "", fmt.Errorf("failed to get file content (%s/%s/%s): %w", owner, repo, path, err)
	}
	if fileContent == nil {
		return "", fmt.Errorf("file not found: %s/%s/%s", owner, repo, path)
	}

	if fileContent.Content != nil {
		decoded, err := base64.StdEncoding.DecodeString(strings.ReplaceAll(*fileContent.Content, "\n", ""))
		if err != nil {
			content, err := fileContent.GetContent()
			if err != nil {
				return "", fmt.Errorf("failed to decode file content: %w", err)
			}
			return content, nil
		}
		return string(decoded), nil
	}

	content, err := fileContent.GetContent()
	if err != nil {
		return "", fmt.Errorf("failed to get file content: %w", err)
	}
	return content, nil
}

// DispatchWorkflow triggers a workflow_dispatch event.
func (c *Client) DispatchWorkflow(ctx context.Context, owner, repo, workflowFile, ref string) error {
	resp, err := c.gh.Actions.CreateWorkflowDispatchEventByFileName(
		ctx, owner, repo, workflowFile,
		gh.CreateWorkflowDispatchEventRequest{
			Ref: ref,
		},
	)
	if err != nil {
		if resp != nil {
			return fmt.Errorf("failed to dispatch workflow (%s/%s/%s, status=%d): %w",
				owner, repo, workflowFile, resp.StatusCode, err)
		}
		return fmt.Errorf("failed to dispatch workflow (%s/%s/%s): %w",
			owner, repo, workflowFile, err)
	}

	slog.Info("dispatched workflow_dispatch",
		"owner", owner,
		"repo", repo,
		"workflow_file", workflowFile,
		"ref", ref,
	)
	return nil
}

// GetVariable returns the value of a repository Actions variable.
func (c *Client) GetVariable(ctx context.Context, owner, repo, name string) (string, error) {
	variable, resp, err := c.gh.Actions.GetRepoVariable(ctx, owner, repo, name)
	if err != nil {
		if resp != nil && resp.StatusCode == 404 {
			return "", nil // variable does not exist
		}
		return "", fmt.Errorf("failed to get variable (%s/%s/%s): %w", owner, repo, name, err)
	}
	return variable.Value, nil
}

// SetVariable creates or updates a repository Actions variable.
func (c *Client) SetVariable(ctx context.Context, owner, repo, name, value string) error {
	_, err := c.gh.Actions.UpdateRepoVariable(ctx, owner, repo, &gh.ActionsVariable{
		Name:  name,
		Value: value,
	})
	if err != nil {
		_, createErr := c.gh.Actions.CreateRepoVariable(ctx, owner, repo, &gh.ActionsVariable{
			Name:  name,
			Value: value,
		})
		if createErr != nil {
			return fmt.Errorf("failed to set variable (%s/%s/%s): update=%v, create=%v", owner, repo, name, err, createErr)
		}
	}
	return nil
}
