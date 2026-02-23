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

// mockClient テスト用モッククライアント
type mockClient struct {
	// GetVariable の戻り値制御
	getVarValue string
	getVarErr   error
	getVarCalls int

	// SetVariable の戻り値制御
	setVarErr   error
	setVarCalls int
	setVarArgs  []setVarCall // 呼び出し引数記録

	// DispatchWorkflow の戻り値制御
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

// newTestScheduler テスト用Schedulerを構築（cronやreconcilerは不要）
func newTestScheduler(client GitHubClient, cfg *config.ReconcileConfig) *Scheduler {
	return &Scheduler{
		client:         client,
		cron:           cron.New(),
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

// TestHandler_NormalDispatch 正常系: dispatch成功
func TestHandler_NormalDispatch(t *testing.T) {
	mock := &mockClient{}
	s := newTestScheduler(mock, defaultConfig())
	handler := s.createJobHandler(testAnnotation())

	handler()

	if mock.dispatchCalls != 1 {
		t.Errorf("DispatchWorkflow呼び出し回数: got %d, want 1", mock.dispatchCalls)
	}
	// SetVariable: 事前保存の1回のみ（ロールバックなし）
	if mock.setVarCalls != 1 {
		t.Errorf("SetVariable呼び出し回数: got %d, want 1", mock.setVarCalls)
	}
}

// TestHandler_DispatchFailure_Rollback dispatch失敗でゼロ値にロールバック
func TestHandler_DispatchFailure_Rollback(t *testing.T) {
	mock := &mockClient{
		dispatchErr: errors.New("API error"),
	}
	s := newTestScheduler(mock, defaultConfig())
	handler := s.createJobHandler(testAnnotation())

	handler()

	if mock.dispatchCalls != 1 {
		t.Errorf("DispatchWorkflow呼び出し回数: got %d, want 1", mock.dispatchCalls)
	}
	// SetVariable: 事前保存 + ロールバック = 2回
	if mock.setVarCalls != 2 {
		t.Errorf("SetVariable呼び出し回数: got %d, want 2", mock.setVarCalls)
	}
	// ロールバック時の値はゼロ値time（GetVariableが空文字を返すため）
	if len(mock.setVarArgs) >= 2 {
		rollbackValue := mock.setVarArgs[1].value
		zeroTime := time.Time{}.Format(time.RFC3339)
		if rollbackValue != zeroTime {
			t.Errorf("ロールバック値: got %q, want %q (ゼロ値)", rollbackValue, zeroTime)
		}
	}
}

// TestHandler_DispatchFailure_RollbackToPrevious 前回dispatch時刻が存在する場合のロールバック
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

	// SetVariable: 事前保存 + ロールバック = 2回
	if mock.setVarCalls != 2 {
		t.Fatalf("SetVariable呼び出し回数: got %d, want 2", mock.setVarCalls)
	}
	// ロールバック時の値は前回dispatch時刻
	rollbackValue := mock.setVarArgs[1].value
	expectedValue := prevTime.Format(time.RFC3339)
	if rollbackValue != expectedValue {
		t.Errorf("ロールバック値: got %q, want %q", rollbackValue, expectedValue)
	}
}

// TestHandler_DuplicateGuard_Blocks ガード期間内はdispatchされない
func TestHandler_DuplicateGuard_Blocks(t *testing.T) {
	recentTime := time.Now().Add(-10 * time.Second)
	mock := &mockClient{
		getVarValue: recentTime.Format(time.RFC3339),
	}
	cfg := &config.ReconcileConfig{
		DuplicateGuardSeconds: 60, // 60秒ガード → 10秒前のdispatchはブロック
	}
	s := newTestScheduler(mock, cfg)
	handler := s.createJobHandler(testAnnotation())

	handler()

	if mock.setVarCalls != 0 {
		t.Errorf("SetVariable呼び出し回数: got %d, want 0（ガードでブロックされるべき）", mock.setVarCalls)
	}
	if mock.dispatchCalls != 0 {
		t.Errorf("DispatchWorkflow呼び出し回数: got %d, want 0（ガードでブロックされるべき）", mock.dispatchCalls)
	}
}

// TestHandler_DryRun DryRunモードではdispatchされない
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
		t.Errorf("SetVariable呼び出し回数: got %d, want 0（DryRunでスキップされるべき）", mock.setVarCalls)
	}
	if mock.dispatchCalls != 0 {
		t.Errorf("DispatchWorkflow呼び出し回数: got %d, want 0（DryRunでスキップされるべき）", mock.dispatchCalls)
	}
}

// TestHandler_GetTimeFails_ProceedsDispatch GetVariable失敗でもdispatchは実行（fail-open）
func TestHandler_GetTimeFails_ProceedsDispatch(t *testing.T) {
	mock := &mockClient{
		getVarErr: errors.New("variable fetch error"),
	}
	s := newTestScheduler(mock, defaultConfig())
	handler := s.createJobHandler(testAnnotation())

	handler()

	if mock.dispatchCalls != 1 {
		t.Errorf("DispatchWorkflow呼び出し回数: got %d, want 1（fail-openで実行されるべき）", mock.dispatchCalls)
	}
	if mock.setVarCalls != 1 {
		t.Errorf("SetVariable呼び出し回数: got %d, want 1（事前保存）", mock.setVarCalls)
	}
}

// TestHandler_GetTimeFails_DispatchFails_NoRollback GetVariable失敗+dispatch失敗時はロールバックをスキップ
func TestHandler_GetTimeFails_DispatchFails_NoRollback(t *testing.T) {
	mock := &mockClient{
		getVarErr:   errors.New("variable fetch error"),
		dispatchErr: errors.New("dispatch error"),
	}
	s := newTestScheduler(mock, defaultConfig())
	handler := s.createJobHandler(testAnnotation())

	handler()

	// SetVariable: 事前保存の1回のみ（ロールバックはスキップされるべき）
	if mock.setVarCalls != 1 {
		t.Errorf("SetVariable呼び出し回数: got %d, want 1（ロールバックはスキップされるべき）", mock.setVarCalls)
	}
}

// TestHandler_SetTimeFails_SkipsDispatch SetVariable失敗時はdispatchをスキップ
func TestHandler_SetTimeFails_SkipsDispatch(t *testing.T) {
	mock := &mockClient{
		setVarErr: errors.New("variable set error"),
	}
	s := newTestScheduler(mock, defaultConfig())
	handler := s.createJobHandler(testAnnotation())

	handler()

	if mock.dispatchCalls != 0 {
		t.Errorf("DispatchWorkflow呼び出し回数: got %d, want 0（SetVar失敗でスキップされるべき）", mock.dispatchCalls)
	}
	if mock.setVarCalls != 1 {
		t.Errorf("SetVariable呼び出し回数: got %d, want 1（事前保存試行）", mock.setVarCalls)
	}
}

// TestHandler_RollbackFailure dispatch失敗+ロールバック失敗でもpanicしない
func TestHandler_RollbackFailure(t *testing.T) {
	// setVarErrを動的に制御するためカスタムモックを使う
	dynamicMock := &dynamicMockClient{
		mockClient: mockClient{
			dispatchErr: errors.New("dispatch error"),
		},
	}
	dynamicMock.setVarFunc = func() error {
		// setVarCallsはSetVariable内でインクリメント済み
		if dynamicMock.setVarCalls == 1 {
			return nil // 事前保存は成功
		}
		return errors.New("rollback error") // ロールバックは失敗
	}

	s := newTestScheduler(dynamicMock, defaultConfig())
	handler := s.createJobHandler(testAnnotation())

	// panicしないことを確認
	handler()

	if dynamicMock.dispatchCalls != 1 {
		t.Errorf("DispatchWorkflow呼び出し回数: got %d, want 1", dynamicMock.dispatchCalls)
	}
	if dynamicMock.setVarCalls != 2 {
		t.Errorf("SetVariable呼び出し回数: got %d, want 2（事前保存+ロールバック試行）", dynamicMock.setVarCalls)
	}
}

// dynamicMockClient SetVariableの挙動を動的に制御するモック
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
