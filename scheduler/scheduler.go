package scheduler

import (
	"context"
	"fmt"
	"log"
	"sync"
	"time"

	"github.com/korosuke613/ghacron/config"
	"github.com/korosuke613/ghacron/github"

	"github.com/robfig/cron/v3"
)

// GitHubClient schedulerが使用するGitHub APIインターフェース
type GitHubClient interface {
	DispatchWorkflow(ctx context.Context, owner, repo, workflowFile, ref string) error
	GetVariable(ctx context.Context, owner, repo, name string) (string, error)
	SetVariable(ctx context.Context, owner, repo, name, value string) error
	GetInstallationRepos(ctx context.Context) ([]github.Repository, error)
	GetWorkflowFiles(ctx context.Context, owner, repo string) ([]github.WorkflowFile, error)
	GetFileContent(ctx context.Context, owner, repo, path, ref string) (string, error)
}

// Scheduler cronジョブマネージャー
type Scheduler struct {
	client     GitHubClient
	reconciler *Reconciler
	cron       *cron.Cron
	config     *config.ReconcileConfig

	mu               sync.RWMutex
	registeredJobs   map[github.CronJobKey]cron.EntryID
	lastReconcile    time.Time
}

// New 新しいスケジューラーを作成
func New(client GitHubClient, cfg *config.ReconcileConfig, loc *time.Location) *Scheduler {
	// 5フィールド標準cron（WithSecondsなし）
	c := cron.New(cron.WithLocation(loc))

	s := &Scheduler{
		client:         client,
		cron:           c,
		config:         cfg,
		registeredJobs: make(map[github.CronJobKey]cron.EntryID),
	}

	s.reconciler = NewReconciler(client, s, cfg)

	// cronスケジューラーを開始
	c.Start()
	log.Println("cronスケジューラーを開始")

	return s
}

// AddJob cronジョブを登録
func (s *Scheduler) AddJob(annotation github.CronAnnotation) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	key := annotation.Key()

	// 既に登録済みならスキップ
	if _, exists := s.registeredJobs[key]; exists {
		return nil
	}

	// ジョブハンドラー作成
	handler := s.createJobHandler(annotation)

	entryID, err := s.cron.AddFunc(annotation.CronExpr, handler)
	if err != nil {
		return fmt.Errorf("cronジョブの追加に失敗 (%s/%s/%s %q): %w",
			annotation.Owner, annotation.Repo, annotation.WorkflowFile, annotation.CronExpr, err)
	}

	s.registeredJobs[key] = entryID
	log.Printf("cronジョブを登録: %s/%s %s %q",
		annotation.Owner, annotation.Repo, annotation.WorkflowFile, annotation.CronExpr)

	return nil
}

// RemoveJob cronジョブを削除
func (s *Scheduler) RemoveJob(key github.CronJobKey) {
	s.mu.Lock()
	defer s.mu.Unlock()

	if entryID, exists := s.registeredJobs[key]; exists {
		s.cron.Remove(entryID)
		delete(s.registeredJobs, key)
		log.Printf("cronジョブを削除: %s/%s %s %q",
			key.Owner, key.Repo, key.WorkflowFile, key.CronExpr)
	}
}

// GetRegisteredJobCount 登録済みジョブ数を返す（StatusProvider インターフェース）
func (s *Scheduler) GetRegisteredJobCount() int {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return len(s.registeredJobs)
}

// GetLastReconcileTime 最終Reconcile時刻を返す（StatusProvider インターフェース）
func (s *Scheduler) GetLastReconcileTime() time.Time {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.lastReconcile
}

// JobDetail ジョブの詳細情報
type JobDetail struct {
	Owner        string    `json:"owner"`
	Repo         string    `json:"repo"`
	WorkflowFile string    `json:"workflow_file"`
	CronExpr     string    `json:"cron_expr"`
	NextRun      time.Time `json:"next_run"`
}

// GetJobDetails 登録済みジョブの詳細一覧を返す（StatusProvider インターフェース）
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

// GetRegisteredKeys 登録済みジョブのキー一覧を返す
func (s *Scheduler) GetRegisteredKeys() []github.CronJobKey {
	s.mu.RLock()
	defer s.mu.RUnlock()

	keys := make([]github.CronJobKey, 0, len(s.registeredJobs))
	for k := range s.registeredJobs {
		keys = append(keys, k)
	}
	return keys
}

// RunReconcileLoop Reconciliationループを実行
func (s *Scheduler) RunReconcileLoop(ctx context.Context, interval time.Duration) {
	// 初回は即座に実行
	s.runReconcile(ctx)

	ticker := time.NewTicker(interval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			log.Println("Reconciliationループを終了")
			return
		case <-ticker.C:
			s.runReconcile(ctx)
		}
	}
}

func (s *Scheduler) runReconcile(ctx context.Context) {
	log.Println("Reconciliation開始")
	start := time.Now()

	if err := s.reconciler.Reconcile(ctx); err != nil {
		log.Printf("Reconciliationに失敗: %v", err)
	}

	s.mu.Lock()
	s.lastReconcile = time.Now()
	s.mu.Unlock()

	log.Printf("Reconciliation完了 (所要時間: %v, 登録ジョブ数: %d)",
		time.Since(start), s.GetRegisteredJobCount())
}

// Stop スケジューラーを停止
func (s *Scheduler) Stop() {
	ctx := s.cron.Stop()
	<-ctx.Done()
	log.Println("cronスケジューラーを停止")
}

// createJobHandler dispatch実行用のジョブハンドラーを作成
func (s *Scheduler) createJobHandler(annotation github.CronAnnotation) func() {
	return func() {
		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()

		stateManager := NewStateManager(s.client)

		// 重複ガード
		lastDispatch, err := stateManager.GetLastDispatchTime(ctx, annotation)
		canRollback := err == nil // 取得成功時のみロールバック可能
		if err != nil {
			log.Printf("前回dispatch時刻の取得に失敗 (%s/%s/%s): %v",
				annotation.Owner, annotation.Repo, annotation.WorkflowFile, err)
			// 取得失敗時は安全側に倒してdispatchを実行
		} else if !lastDispatch.IsZero() {
			elapsed := time.Since(lastDispatch)
			guard := time.Duration(s.config.DuplicateGuardSeconds) * time.Second
			if elapsed < guard {
				log.Printf("重複ガード: %s/%s %s は %v前にdispatch済み（ガード=%v）",
					annotation.Owner, annotation.Repo, annotation.WorkflowFile,
					elapsed.Round(time.Second), guard)
				return
			}
		}

		// dry-run モード
		if s.config.DryRun {
			log.Printf("[DRY-RUN] dispatch対象: %s/%s %s (ref=%s, cron=%q)",
				annotation.Owner, annotation.Repo, annotation.WorkflowFile,
				annotation.Ref, annotation.CronExpr)
			return
		}

		// dispatch時刻を事前に永続化（競合防止のため）
		now := time.Now()
		if err := stateManager.SetLastDispatchTime(ctx, annotation, now); err != nil {
			log.Printf("dispatch時刻の事前保存に失敗 (%s/%s/%s): %v",
				annotation.Owner, annotation.Repo, annotation.WorkflowFile, err)
			// 保存失敗時は重複の可能性があるためdispatchをスキップ
			return
		}

		// workflow_dispatch を発火
		if err := s.client.DispatchWorkflow(ctx, annotation.Owner, annotation.Repo,
			annotation.WorkflowFile, annotation.Ref); err != nil {
			log.Printf("dispatch失敗: %v", err)
			// ファントム・ガード防止: 前回時刻の取得に成功していた場合のみロールバック
			if canRollback {
				if rbErr := stateManager.SetLastDispatchTime(ctx, annotation, lastDispatch); rbErr != nil {
					log.Printf("dispatch時刻のロールバックに失敗 (%s/%s/%s): %v",
						annotation.Owner, annotation.Repo, annotation.WorkflowFile, rbErr)
				}
			}
			return
		}
	}
}
