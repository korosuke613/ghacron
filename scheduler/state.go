package scheduler

import (
	"context"
	"crypto/sha256"
	"fmt"
	"time"

	"github.com/korosuke613/ghacron/github"
)

// StateClient is the interface used by StateManager.
type StateClient interface {
	GetVariable(ctx context.Context, owner, repo, name string) (string, error)
	SetVariable(ctx context.Context, owner, repo, name, value string) error
}

// StateManager manages state via GitHub Actions Variables.
type StateManager struct {
	client StateClient
}

// NewStateManager creates a new StateManager.
func NewStateManager(client StateClient) *StateManager {
	return &StateManager{client: client}
}

// GetLastDispatchTime retrieves the last dispatch time.
func (sm *StateManager) GetLastDispatchTime(ctx context.Context, annotation github.CronAnnotation) (time.Time, error) {
	varName := sm.variableName(annotation)

	value, err := sm.client.GetVariable(ctx, annotation.Owner, annotation.Repo, varName)
	if err != nil {
		return time.Time{}, err
	}
	if value == "" {
		return time.Time{}, nil // variable does not exist = never dispatched
	}

	t, err := time.Parse(time.RFC3339, value)
	if err != nil {
		return time.Time{}, fmt.Errorf("failed to parse dispatch time (%q): %w", value, err)
	}

	return t, nil
}

// SetLastDispatchTime persists the dispatch time.
func (sm *StateManager) SetLastDispatchTime(ctx context.Context, annotation github.CronAnnotation, t time.Time) error {
	varName := sm.variableName(annotation)
	value := t.Format(time.RFC3339)

	return sm.client.SetVariable(ctx, annotation.Owner, annotation.Repo, varName, value)
}

// variableName generates a variable name from an annotation.
// Format: GHACRON_LAST_<first 8 hex chars of SHA256>
func (sm *StateManager) variableName(annotation github.CronAnnotation) string {
	input := annotation.WorkflowFile + ":" + annotation.CronExpr
	hash := sha256.Sum256([]byte(input))
	return fmt.Sprintf("GHACRON_LAST_%X", hash[:4])
}
