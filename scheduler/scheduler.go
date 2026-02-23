package scheduler

import (
	"context"
	"fmt"
	"log/slog"
	"sync"
	"time"

	"github.com/korosuke613/ghacron/config"
	"github.com/korosuke613/ghacron/github"

	"github.com/robfig/cron/v3"
)

// GitHubClient is the GitHub API interface used by the scheduler.
type GitHubClient interface {
	DispatchWorkflow(ctx context.Context, owner, repo, workflowFile, ref string) error
	GetVariable(ctx context.Context, owner, repo, name string) (string, error)
	SetVariable(ctx context.Context, owner, repo, name, value string) error
	GetInstallationRepos(ctx context.Context) ([]github.Repository, error)
	GetWorkflowFiles(ctx context.Context, owner, repo string) ([]github.WorkflowFile, error)
	GetFileContent(ctx context.Context, owner, repo, path, ref string) (string, error)
}

// Scheduler manages cron jobs.
type Scheduler struct {
	client     GitHubClient
	reconciler *Reconciler
	cron       *cron.Cron
	config     *config.ReconcileConfig

	mu             sync.RWMutex
	registeredJobs map[github.CronJobKey]cron.EntryID
	lastReconcile  time.Time
}

// New creates a new Scheduler.
func New(client GitHubClient, cfg *config.ReconcileConfig, loc *time.Location) *Scheduler {
	// 5-field standard cron (no WithSeconds)
	c := cron.New(cron.WithLocation(loc))

	s := &Scheduler{
		client:         client,
		cron:           c,
		config:         cfg,
		registeredJobs: make(map[github.CronJobKey]cron.EntryID),
	}

	s.reconciler = NewReconciler(client, s, cfg)

	c.Start()
	slog.Info("cron scheduler started")

	return s
}

// AddJob registers a cron job.
func (s *Scheduler) AddJob(annotation github.CronAnnotation) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	key := annotation.Key()

	// Skip if already registered
	if _, exists := s.registeredJobs[key]; exists {
		return nil
	}

	handler := s.createJobHandler(annotation)

	entryID, err := s.cron.AddFunc(annotation.CronExpr, handler)
	if err != nil {
		return fmt.Errorf("failed to add cron job (%s/%s/%s %q): %w",
			annotation.Owner, annotation.Repo, annotation.WorkflowFile, annotation.CronExpr, err)
	}

	s.registeredJobs[key] = entryID
	slog.Info("registered cron job",
		"owner", annotation.Owner,
		"repo", annotation.Repo,
		"workflow_file", annotation.WorkflowFile,
		"cron_expr", annotation.CronExpr,
	)

	return nil
}

// RemoveJob removes a cron job.
func (s *Scheduler) RemoveJob(key github.CronJobKey) {
	s.mu.Lock()
	defer s.mu.Unlock()

	if entryID, exists := s.registeredJobs[key]; exists {
		s.cron.Remove(entryID)
		delete(s.registeredJobs, key)
		slog.Info("removed cron job",
			"owner", key.Owner,
			"repo", key.Repo,
			"workflow_file", key.WorkflowFile,
			"cron_expr", key.CronExpr,
		)
	}
}

// GetRegisteredJobCount returns the number of registered jobs (StatusProvider).
func (s *Scheduler) GetRegisteredJobCount() int {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return len(s.registeredJobs)
}

// GetLastReconcileTime returns the last reconcile timestamp (StatusProvider).
func (s *Scheduler) GetLastReconcileTime() time.Time {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.lastReconcile
}

// JobDetail holds detailed information about a registered job.
type JobDetail struct {
	Owner        string    `json:"owner"`
	Repo         string    `json:"repo"`
	WorkflowFile string    `json:"workflow_file"`
	CronExpr     string    `json:"cron_expr"`
	NextRun      time.Time `json:"next_run"`
}

// GetJobDetails returns details of all registered jobs (StatusProvider).
func (s *Scheduler) GetJobDetails() []JobDetail {
	s.mu.RLock()
	defer s.mu.RUnlock()

	details := make([]JobDetail, 0, len(s.registeredJobs))
	for key, entryID := range s.registeredJobs {
		entry := s.cron.Entry(entryID)
		details = append(details, JobDetail{
			Owner:        key.Owner,
			Repo:         key.Repo,
			WorkflowFile: key.WorkflowFile,
			CronExpr:     key.CronExpr,
			NextRun:      entry.Next,
		})
	}
	return details
}

// GetRegisteredKeys returns all registered job keys.
func (s *Scheduler) GetRegisteredKeys() []github.CronJobKey {
	s.mu.RLock()
	defer s.mu.RUnlock()

	keys := make([]github.CronJobKey, 0, len(s.registeredJobs))
	for k := range s.registeredJobs {
		keys = append(keys, k)
	}
	return keys
}

// RunReconcileLoop runs the reconciliation loop.
func (s *Scheduler) RunReconcileLoop(ctx context.Context, interval time.Duration) {
	// Run immediately on startup
	s.runReconcile(ctx)

	ticker := time.NewTicker(interval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			slog.Info("reconciliation loop stopped")
			return
		case <-ticker.C:
			s.runReconcile(ctx)
		}
	}
}

func (s *Scheduler) runReconcile(ctx context.Context) {
	slog.Info("reconciliation started")
	start := time.Now()

	if err := s.reconciler.Reconcile(ctx); err != nil {
		slog.Error("reconciliation failed", "error", err)
	}

	s.mu.Lock()
	s.lastReconcile = time.Now()
	s.mu.Unlock()

	slog.Info("reconciliation completed",
		"duration", time.Since(start).String(),
		"registered_jobs", s.GetRegisteredJobCount(),
	)
}

// Stop stops the scheduler.
func (s *Scheduler) Stop() {
	ctx := s.cron.Stop()
	<-ctx.Done()
	slog.Info("cron scheduler stopped")
}

// createJobHandler creates a job handler for dispatching workflows.
func (s *Scheduler) createJobHandler(annotation github.CronAnnotation) func() {
	return func() {
		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()

		stateManager := NewStateManager(s.client)

		// Duplicate guard
		lastDispatch, err := stateManager.GetLastDispatchTime(ctx, annotation)
		canRollback := err == nil // rollback only if previous time was retrieved
		if err != nil {
			slog.Error("failed to get last dispatch time",
				"owner", annotation.Owner,
				"repo", annotation.Repo,
				"workflow_file", annotation.WorkflowFile,
				"error", err,
			)
			// Fail-open: proceed with dispatch on retrieval failure
		} else if !lastDispatch.IsZero() {
			elapsed := time.Since(lastDispatch)
			guard := time.Duration(s.config.DuplicateGuardSeconds) * time.Second
			if elapsed < guard {
				slog.Info("duplicate guard: already dispatched",
					"owner", annotation.Owner,
					"repo", annotation.Repo,
					"workflow_file", annotation.WorkflowFile,
					"elapsed", elapsed.Round(time.Second).String(),
					"guard", guard.String(),
				)
				return
			}
		}

		// Dry-run mode
		if s.config.DryRun {
			slog.Info("[DRY-RUN] dispatch target",
				"owner", annotation.Owner,
				"repo", annotation.Repo,
				"workflow_file", annotation.WorkflowFile,
				"ref", annotation.Ref,
				"cron_expr", annotation.CronExpr,
			)
			return
		}

		// Persist dispatch time before dispatching (to prevent races)
		now := time.Now()
		if err := stateManager.SetLastDispatchTime(ctx, annotation, now); err != nil {
			slog.Error("failed to save dispatch time",
				"owner", annotation.Owner,
				"repo", annotation.Repo,
				"workflow_file", annotation.WorkflowFile,
				"error", err,
			)
			// Skip dispatch to avoid potential duplicates
			return
		}

		// Fire workflow_dispatch
		if err := s.client.DispatchWorkflow(ctx, annotation.Owner, annotation.Repo,
			annotation.WorkflowFile, annotation.Ref); err != nil {
			slog.Error("dispatch failed",
				"owner", annotation.Owner,
				"repo", annotation.Repo,
				"workflow_file", annotation.WorkflowFile,
				"error", err,
			)
			// Phantom guard prevention: rollback only if previous time was retrieved
			if canRollback {
				if rbErr := stateManager.SetLastDispatchTime(ctx, annotation, lastDispatch); rbErr != nil {
					slog.Error("failed to rollback dispatch time",
						"owner", annotation.Owner,
						"repo", annotation.Repo,
						"workflow_file", annotation.WorkflowFile,
						"error", rbErr,
					)
				}
			}
			return
		}
	}
}
