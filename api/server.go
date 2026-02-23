package api

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"net"
	"net/http"
	"sync"
	"time"

	"github.com/korosuke613/ghacron/config"
	"github.com/korosuke613/ghacron/scheduler"
)

// StatusProvider ステータス情報を提供するインターフェース
type StatusProvider interface {
	GetRegisteredJobCount() int
	GetLastReconcileTime() time.Time
	GetJobDetails() []scheduler.JobDetail
}

// Server ヘルス/ステータスAPIサーバー
type Server struct {
	config         *config.WebAPIConfig
	appConfig      *config.Config
	httpServer     *http.Server
	statusProvider StatusProvider
	startTime      time.Time
	mu             sync.RWMutex
}

// NewServer 新しいAPIサーバーを作成
func NewServer(cfg *config.WebAPIConfig, appCfg *config.Config) *Server {
	return &Server{
		config:    cfg,
		appConfig: appCfg,
		startTime: time.Now(),
	}
}

// SetStatusProvider ステータスプロバイダーを設定
func (s *Server) SetStatusProvider(provider StatusProvider) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.statusProvider = provider
}

// Start APIサーバーを開始
func (s *Server) Start() error {
	if !s.config.Enabled {
		slog.Info("web API server is disabled")
		return nil
	}

	mux := http.NewServeMux()
	mux.HandleFunc("/", s.handleIndex)
	mux.HandleFunc("/healthz", s.handleHealthz)
	mux.HandleFunc("/status", s.handleStatus)
	mux.HandleFunc("/jobs", s.handleJobs)
	mux.HandleFunc("/config", s.handleConfig)

	addr := net.JoinHostPort(s.config.Host, fmt.Sprintf("%d", s.config.Port))
	s.httpServer = &http.Server{
		Addr:         addr,
		Handler:      mux,
		ReadTimeout:  5 * time.Second,
		WriteTimeout: 10 * time.Second,
	}

	go func() {
		slog.Info("API server started", "addr", addr)
		if err := s.httpServer.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			slog.Error("API server error", "error", err)
		}
	}()

	return nil
}

// Stop APIサーバーを停止
func (s *Server) Stop() {
	if s.httpServer == nil {
		return
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if err := s.httpServer.Shutdown(ctx); err != nil {
		slog.Error("failed to stop API server", "error", err)
	}
}

func (s *Server) handleIndex(w http.ResponseWriter, r *http.Request) {
	if r.URL.Path != "/" {
		http.NotFound(w, r)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{
		"service": "ghacron",
		"endpoints": []map[string]string{
			{"path": "/healthz", "description": "Health check"},
			{"path": "/status", "description": "Service status (uptime, job count, last reconcile)"},
			{"path": "/jobs", "description": "Registered cron job list"},
			{"path": "/config", "description": "Public configuration"},
		},
	})
}

func (s *Server) handleHealthz(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]string{
		"status": "ok",
	})
}

func (s *Server) handleStatus(w http.ResponseWriter, r *http.Request) {
	s.mu.RLock()
	provider := s.statusProvider
	s.mu.RUnlock()

	status := map[string]interface{}{
		"uptime_seconds": time.Since(s.startTime).Seconds(),
	}

	if provider != nil {
		status["registered_jobs"] = provider.GetRegisteredJobCount()
		lastReconcile := provider.GetLastReconcileTime()
		if !lastReconcile.IsZero() {
			status["last_reconcile"] = lastReconcile.Format(time.RFC3339)
		}
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(status)
}

func (s *Server) handleJobs(w http.ResponseWriter, r *http.Request) {
	s.mu.RLock()
	provider := s.statusProvider
	s.mu.RUnlock()

	var jobs []scheduler.JobDetail
	if provider != nil {
		jobs = provider.GetJobDetails()
	}
	if jobs == nil {
		jobs = []scheduler.JobDetail{}
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(jobs)
}

// configResponse /config エンドポイントの公開可能な設定
type configResponse struct {
	GitHub    configGitHub    `json:"github"`
	Reconcile configReconcile `json:"reconcile"`
	Log       configLog       `json:"log"`
	WebAPI    configWebAPI    `json:"webapi"`
}

type configGitHub struct {
	AppID int64 `json:"app_id"`
}

type configReconcile struct {
	IntervalMinutes       int  `json:"interval_minutes"`
	DuplicateGuardSeconds int  `json:"duplicate_guard_seconds"`
	DryRun                bool `json:"dry_run"`
}

type configLog struct {
	Level  string `json:"level"`
	Format string `json:"format"`
}

type configWebAPI struct {
	Enabled bool   `json:"enabled"`
	Host    string `json:"host"`
	Port    int    `json:"port"`
}

func (s *Server) handleConfig(w http.ResponseWriter, r *http.Request) {
	s.mu.RLock()
	appCfg := s.appConfig
	s.mu.RUnlock()

	resp := configResponse{
		GitHub: configGitHub{
			AppID: appCfg.GitHub.AppID,
		},
		Reconcile: configReconcile{
			IntervalMinutes:       appCfg.Reconcile.IntervalMinutes,
			DuplicateGuardSeconds: appCfg.Reconcile.DuplicateGuardSeconds,
			DryRun:                appCfg.Reconcile.DryRun,
		},
		Log: configLog{
			Level:  appCfg.Log.Level,
			Format: appCfg.Log.Format,
		},
		WebAPI: configWebAPI{
			Enabled: appCfg.WebAPI.Enabled,
			Host:    appCfg.WebAPI.Host,
			Port:    appCfg.WebAPI.Port,
		},
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(resp)
}
