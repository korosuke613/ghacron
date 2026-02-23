package scheduler

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/korosuke613/ghacron/config"
	"github.com/korosuke613/ghacron/github"

	"github.com/robfig/cron/v3"
)

// mockClient is a mock GitHub client for testing.
type mockClient struct {
	getVarValue string
	getVarErr   error
	getVarCalls int

	setVarErr   error
	setVarCalls int
	setVarArgs  []setVarCall

	dispatchErr   error
	dispatchCalls int

	mu sync.Mutex
}

type setVarCall struct {
	owner, repo, name, value string
}

func (m *mockClient) GetVariable(_ context.Context, _, _, _ string) (string, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.getVarCalls++
	return m.getVarValue, m.getVarErr
}

func (m *mockClient) SetVariable(_ context.Context, owner, repo, name, value string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.setVarCalls++
	m.setVarArgs = append(m.setVarArgs, setVarCall{owner, repo, name, value})
	return m.setVarErr
}

func (m *mockClient) DispatchWorkflow(_ context.Context, _, _, _, _ string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.dispatchCalls++
	return m.dispatchErr
}

func (m *mockClient) GetInstallationRepos(_ context.Context) ([]github.Repository, error) {
	return nil, nil
}

func (m *mockClient) GetWorkflowFiles(_ context.Context, _, _ string) ([]github.WorkflowFile, error) {
	return nil, nil
}

func (m *mockClient) GetFileContent(_ context.Context, _, _, _, _ string) (string, error) {
	return "", nil
}

func newTestScheduler(client GitHubClient, cfg *config.ReconcileConfig) *Scheduler {
	return &Scheduler{
		client:         client,
		cron:           cron.New(cron.WithLocation(time.UTC)),
		config:         cfg,
		registeredJobs: make(map[github.CronJobKey]cron.EntryID),
	}
}

func testAnnotation() github.CronAnnotation {
	return github.CronAnnotation{
		Owner:        "test-owner",
		Repo:         "test-repo",
		WorkflowFile: "ci.yml",
		CronExpr:     "0 9 * * *",
		Ref:          "main",
	}
}

func defaultConfig() *config.ReconcileConfig {
	return &config.ReconcileConfig{
		DuplicateGuardSeconds: 60,
		DryRun:                false,
	}
}

func TestHandler_NormalDispatch(t *testing.T) {
	mock := &mockClient{}
	s := newTestScheduler(mock, defaultConfig())
	handler := s.createJobHandler(testAnnotation())

	handler()

	if mock.dispatchCalls != 1 {
		t.Errorf("DispatchWorkflow call count: got %d, want 1", mock.dispatchCalls)
	}
	if mock.setVarCalls != 1 {
		t.Errorf("SetVariable call count: got %d, want 1", mock.setVarCalls)
	}
}

func TestHandler_DispatchFailure_Rollback(t *testing.T) {
	mock := &mockClient{
		dispatchErr: errors.New("API error"),
	}
	s := newTestScheduler(mock, defaultConfig())
	handler := s.createJobHandler(testAnnotation())

	handler()

	if mock.dispatchCalls != 1 {
		t.Errorf("DispatchWorkflow call count: got %d, want 1", mock.dispatchCalls)
	}
	// SetVariable: pre-save + rollback = 2 calls
	if mock.setVarCalls != 2 {
		t.Errorf("SetVariable call count: got %d, want 2", mock.setVarCalls)
	}
	// Rollback value should be zero time (GetVariable returns empty string)
	if len(mock.setVarArgs) >= 2 {
		rollbackValue := mock.setVarArgs[1].value
		zeroTime := time.Time{}.Format(time.RFC3339)
		if rollbackValue != zeroTime {
			t.Errorf("rollback value: got %q, want %q (zero time)", rollbackValue, zeroTime)
		}
	}
}

func TestHandler_DispatchFailure_RollbackToPrevious(t *testing.T) {
	prevTime := time.Date(2000, 1, 1, 12, 0, 0, 0, time.UTC)
	mock := &mockClient{
		getVarValue: prevTime.Format(time.RFC3339),
		dispatchErr: errors.New("API error"),
	}
	cfg := &config.ReconcileConfig{
		DuplicateGuardSeconds: 60,
	}
	s := newTestScheduler(mock, cfg)
	handler := s.createJobHandler(testAnnotation())

	handler()

	// SetVariable: pre-save + rollback = 2 calls
	if mock.setVarCalls != 2 {
		t.Fatalf("SetVariable call count: got %d, want 2", mock.setVarCalls)
	}
	// Rollback value should be previous dispatch time
	rollbackValue := mock.setVarArgs[1].value
	expectedValue := prevTime.Format(time.RFC3339)
	if rollbackValue != expectedValue {
		t.Errorf("rollback value: got %q, want %q", rollbackValue, expectedValue)
	}
}

func TestHandler_DuplicateGuard_Blocks(t *testing.T) {
	recentTime := time.Now().Add(-10 * time.Second)
	mock := &mockClient{
		getVarValue: recentTime.Format(time.RFC3339),
	}
	cfg := &config.ReconcileConfig{
		DuplicateGuardSeconds: 60, // 60s guard > 10s ago => blocked
	}
	s := newTestScheduler(mock, cfg)
	handler := s.createJobHandler(testAnnotation())

	handler()

	if mock.setVarCalls != 0 {
		t.Errorf("SetVariable call count: got %d, want 0 (should be blocked by guard)", mock.setVarCalls)
	}
	if mock.dispatchCalls != 0 {
		t.Errorf("DispatchWorkflow call count: got %d, want 0 (should be blocked by guard)", mock.dispatchCalls)
	}
}

func TestHandler_DryRun(t *testing.T) {
	mock := &mockClient{}
	cfg := &config.ReconcileConfig{
		DuplicateGuardSeconds: 60,
		DryRun:                true,
	}
	s := newTestScheduler(mock, cfg)
	handler := s.createJobHandler(testAnnotation())

	handler()

	if mock.setVarCalls != 0 {
		t.Errorf("SetVariable call count: got %d, want 0 (should be skipped in dry-run)", mock.setVarCalls)
	}
	if mock.dispatchCalls != 0 {
		t.Errorf("DispatchWorkflow call count: got %d, want 0 (should be skipped in dry-run)", mock.dispatchCalls)
	}
}

func TestHandler_GetTimeFails_ProceedsDispatch(t *testing.T) {
	mock := &mockClient{
		getVarErr: errors.New("variable fetch error"),
	}
	s := newTestScheduler(mock, defaultConfig())
	handler := s.createJobHandler(testAnnotation())

	handler()

	if mock.dispatchCalls != 1 {
		t.Errorf("DispatchWorkflow call count: got %d, want 1 (should proceed as fail-open)", mock.dispatchCalls)
	}
	if mock.setVarCalls != 1 {
		t.Errorf("SetVariable call count: got %d, want 1 (pre-save)", mock.setVarCalls)
	}
}

func TestHandler_GetTimeFails_DispatchFails_NoRollback(t *testing.T) {
	mock := &mockClient{
		getVarErr:   errors.New("variable fetch error"),
		dispatchErr: errors.New("dispatch error"),
	}
	s := newTestScheduler(mock, defaultConfig())
	handler := s.createJobHandler(testAnnotation())

	handler()

	// SetVariable: only pre-save (rollback should be skipped)
	if mock.setVarCalls != 1 {
		t.Errorf("SetVariable call count: got %d, want 1 (rollback should be skipped)", mock.setVarCalls)
	}
}

func TestHandler_SetTimeFails_SkipsDispatch(t *testing.T) {
	mock := &mockClient{
		setVarErr: errors.New("variable set error"),
	}
	s := newTestScheduler(mock, defaultConfig())
	handler := s.createJobHandler(testAnnotation())

	handler()

	if mock.dispatchCalls != 0 {
		t.Errorf("DispatchWorkflow call count: got %d, want 0 (should be skipped on SetVar failure)", mock.dispatchCalls)
	}
	if mock.setVarCalls != 1 {
		t.Errorf("SetVariable call count: got %d, want 1 (pre-save attempt)", mock.setVarCalls)
	}
}

func TestHandler_RollbackFailure(t *testing.T) {
	dynamicMock := &dynamicMockClient{
		mockClient: mockClient{
			dispatchErr: errors.New("dispatch error"),
		},
	}
	dynamicMock.setVarFunc = func() error {
		if dynamicMock.setVarCalls == 1 {
			return nil // pre-save succeeds
		}
		return errors.New("rollback error") // rollback fails
	}

	s := newTestScheduler(dynamicMock, defaultConfig())
	handler := s.createJobHandler(testAnnotation())

	// should not panic
	handler()

	if dynamicMock.dispatchCalls != 1 {
		t.Errorf("DispatchWorkflow call count: got %d, want 1", dynamicMock.dispatchCalls)
	}
	if dynamicMock.setVarCalls != 2 {
		t.Errorf("SetVariable call count: got %d, want 2 (pre-save + rollback attempt)", dynamicMock.setVarCalls)
	}
}

// dynamicMockClient allows dynamic control of SetVariable behavior.
type dynamicMockClient struct {
	mockClient
	setVarFunc func() error
}

func (m *dynamicMockClient) SetVariable(_ context.Context, owner, repo, name, value string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.setVarCalls++
	m.setVarArgs = append(m.setVarArgs, setVarCall{owner, repo, name, value})
	if m.setVarFunc != nil {
		return m.setVarFunc()
	}
	return m.setVarErr
}
