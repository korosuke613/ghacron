package scheduler

import (
	"context"
	"crypto/sha256"
	"fmt"
	"time"

	"github.com/korosuke613/ghacron/github"
)

// StateClient StateManagerが使用するインターフェース
type StateClient interface {
	GetVariable(ctx context.Context, owner, repo, name string) (string, error)
	SetVariable(ctx context.Context, owner, repo, name, value string) error
}

// StateManager GitHub Actions Variablesによる状態管理
type StateManager struct {
	client StateClient
}

// NewStateManager 新しいStateManagerを作成
func NewStateManager(client StateClient) *StateManager {
	return &StateManager{client: client}
}

// GetLastDispatchTime 前回のdispatch時刻を取得
func (sm *StateManager) GetLastDispatchTime(ctx context.Context, annotation github.CronAnnotation) (time.Time, error) {
	varName := sm.variableName(annotation)

	value, err := sm.client.GetVariable(ctx, annotation.Owner, annotation.Repo, varName)
	if err != nil {
		return time.Time{}, err
	}
	if value == "" {
		return time.Time{}, nil // 変数が存在しない = 未dispatch
	}

	t, err := time.Parse(time.RFC3339, value)
	if err != nil {
		return time.Time{}, fmt.Errorf("dispatch時刻のパースに失敗 (%q): %w", value, err)
	}

	return t, nil
}

// SetLastDispatchTime dispatch時刻を保存
func (sm *StateManager) SetLastDispatchTime(ctx context.Context, annotation github.CronAnnotation, t time.Time) error {
	varName := sm.variableName(annotation)
	value := t.Format(time.RFC3339)

	return sm.client.SetVariable(ctx, annotation.Owner, annotation.Repo, varName, value)
}

// variableName アノテーションからVariable名を生成
// 形式: GHACRON_LAST_<SHA256先頭8文字>
func (sm *StateManager) variableName(annotation github.CronAnnotation) string {
	input := annotation.WorkflowFile + ":" + annotation.CronExpr
	hash := sha256.Sum256([]byte(input))
	return fmt.Sprintf("GHACRON_LAST_%X", hash[:4])
}
