package scheduler

import (
	"context"
	"log/slog"

	"github.com/korosuke613/ghacron/config"
	"github.com/korosuke613/ghacron/github"
	"github.com/korosuke613/ghacron/scanner"
)

// Reconciler desired state と actual state の差分を適用
type Reconciler struct {
	client    GitHubClient
	scheduler *Scheduler
	scanner   *scanner.Scanner
	config    *config.ReconcileConfig
}

// NewReconciler 新しいReconcilerを作成
func NewReconciler(client GitHubClient, sched *Scheduler, cfg *config.ReconcileConfig) *Reconciler {
	return &Reconciler{
		client:    client,
		scheduler: sched,
		scanner:   scanner.New(client),
		config:    cfg,
	}
}

// Reconcile desired state（アノテーション）と actual state（登録済みcron）の差分を適用
func (r *Reconciler) Reconcile(ctx context.Context) error {
	// 1. Discovery + Scan: 全リポジトリからアノテーションを収集
	result, err := r.scanner.ScanAll(ctx)
	if err != nil {
		return err
	}

	// 2. Desired state をマップに変換
	desiredMap := make(map[github.CronJobKey]github.CronAnnotation)
	for _, a := range result.Annotations {
		desiredMap[a.Key()] = a
	}

	// 3. Actual state（登録済みジョブ）を取得
	actualKeys := r.scheduler.GetRegisteredKeys()

	// 4. Diff: toAdd = desired - actual, toRemove = actual - desired
	actualSet := make(map[github.CronJobKey]struct{})
	for _, key := range actualKeys {
		actualSet[key] = struct{}{}
	}

	var toAdd []github.CronAnnotation
	for key, annotation := range desiredMap {
		if _, exists := actualSet[key]; !exists {
			toAdd = append(toAdd, annotation)
		}
	}

	var toRemove []github.CronJobKey
	for _, key := range actualKeys {
		if _, exists := desiredMap[key]; !exists {
			toRemove = append(toRemove, key)
		}
	}

	// 5. Apply
	for _, annotation := range toAdd {
		if err := r.scheduler.AddJob(annotation); err != nil {
			slog.Error("ジョブ追加に失敗", "error", err)
		}
	}

	for _, key := range toRemove {
		r.scheduler.RemoveJob(key)
	}

	// 6. Log summary
	if len(toAdd) > 0 || len(toRemove) > 0 {
		slog.Info("Reconcile結果",
			"added", len(toAdd),
			"removed", len(toRemove),
			"desired_total", len(desiredMap),
		)
	}

	return nil
}
