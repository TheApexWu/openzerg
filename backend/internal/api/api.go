// Package api hosts the HTTP+SSE control surface for `openzerg serve`.
//
// The same Go binary that runs the headless `openzerg run` CLI can also be
// invoked as `openzerg serve --addr :8080` to expose a small REST + SSE API
// and serve the embedded single-page frontend. There is one process, one
// port, one artifact — no separate npm dev server, no nginx, no docker
// compose. The frontend tree is baked in at compile time via go:embed.
package api

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"os"
	"path/filepath"
	"sync"
	"time"

	"github.com/TheApexWu/openzerg/backend/internal/config"
	"github.com/TheApexWu/openzerg/backend/internal/events"
	"github.com/TheApexWu/openzerg/backend/internal/runner"
	"github.com/TheApexWu/openzerg/backend/internal/secrets"
	"github.com/TheApexWu/openzerg/backend/internal/store"
)

// ServerConfig parameterizes NewServer.
type ServerConfig struct {
	Addr           string
	FrontendDir    string // empty -> use embedded frontend
	KubeconfigPath string
	OutDir         string
	EnvFilePath    string
	CORSOrigin     string
	Stdout         io.Writer
	Stderr         io.Writer
}

// Server is the HTTP entrypoint. It holds the events broker, the run
// controller, and the frontend handler. Single-instance per process.
type Server struct {
	cfg        ServerConfig
	broker     *events.Broker
	controller *RunController
	httpServer *http.Server
	logger     *slog.Logger
	mux        *http.ServeMux
}

// NewServer constructs a server. Validates the frontend serving choice
// (embedded vs --frontend dir) and fails loud if the embedded tree is empty.
func NewServer(cfg ServerConfig) (*Server, error) {
	if cfg.Stdout == nil {
		cfg.Stdout = os.Stdout
	}
	if cfg.Stderr == nil {
		cfg.Stderr = os.Stderr
	}
	if cfg.OutDir == "" {
		cfg.OutDir = "./out"
	}

	broker := events.NewBroker()
	controller := &RunController{
		broker:         broker,
		envFilePath:    cfg.EnvFilePath,
		kubeconfigPath: cfg.KubeconfigPath,
		outDir:         cfg.OutDir,
		stdout:         cfg.Stdout,
		stderr:         cfg.Stderr,
	}
	logger := slog.New(slog.NewTextHandler(cfg.Stderr, &slog.HandlerOptions{Level: slog.LevelInfo}))

	frontendHandler, err := buildFrontendHandler(cfg.FrontendDir)
	if err != nil {
		return nil, err
	}

	mux := http.NewServeMux()
	server := &Server{
		cfg:        cfg,
		broker:     broker,
		controller: controller,
		logger:     logger,
		mux:        mux,
	}

	mux.HandleFunc("GET /healthz", server.handleHealthz)
	mux.HandleFunc("GET /api/events", server.handleSSE)
	mux.HandleFunc("GET /api/runs", server.handleRunsList)
	mux.HandleFunc("GET /api/runs/current", server.handleCurrentRun)
	mux.HandleFunc("GET /api/runs/{id}", server.handleRunByID)
	mux.HandleFunc("POST /api/runs", server.handleStartRun)
	mux.HandleFunc("POST /api/runs/current/cancel", server.handleCancelRun)
	mux.HandleFunc("GET /api/integrations/openrouter", server.handleOpenRouterIntegration)
	mux.HandleFunc("GET /api/integrations/nimble", server.handleNimbleIntegration)
	mux.Handle("/", frontendHandler)

	server.httpServer = &http.Server{
		Addr:              cfg.Addr,
		Handler:           server.withMiddleware(mux),
		ReadHeaderTimeout: 5 * time.Second,
	}
	return server, nil
}

// ListenAndServe starts the HTTP server. Blocks until the server exits.
func (s *Server) ListenAndServe(ctx context.Context) error {
	serverErr := make(chan error, 1)
	go func() {
		err := s.httpServer.ListenAndServe()
		if err != nil && !errors.Is(err, http.ErrServerClosed) {
			serverErr <- err
		}
		close(serverErr)
	}()
	select {
	case <-ctx.Done():
		shutdownContext, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		return s.httpServer.Shutdown(shutdownContext)
	case err := <-serverErr:
		return err
	}
}

// withMiddleware wraps the mux with structured logging and an optional
// CORS opener (dev mode only — same-origin is the default).
func (s *Server) withMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()
		if s.cfg.CORSOrigin != "" && len(r.Header.Get("Origin")) > 0 {
			w.Header().Set("Access-Control-Allow-Origin", s.cfg.CORSOrigin)
			w.Header().Set("Access-Control-Allow-Methods", "GET, POST, OPTIONS")
			w.Header().Set("Access-Control-Allow-Headers", "content-type")
			if r.Method == http.MethodOptions {
				w.WriteHeader(http.StatusNoContent)
				return
			}
		}
		next.ServeHTTP(w, r)
		s.logger.Info("http",
			"method", r.Method,
			"path", r.URL.Path,
			"remote", r.RemoteAddr,
			"dur_ms", time.Since(start).Milliseconds(),
		)
	})
}

func (s *Server) handleHealthz(w http.ResponseWriter, _ *http.Request) {
	writeJSON(w, http.StatusOK, map[string]any{"ok": true, "ts": time.Now().UTC().Format(time.RFC3339)})
}

func (s *Server) handleRunsList(w http.ResponseWriter, _ *http.Request) {
	runs := s.controller.History()
	writeJSON(w, http.StatusOK, runs)
}

func (s *Server) handleCurrentRun(w http.ResponseWriter, _ *http.Request) {
	current := s.controller.Current()
	if current == nil {
		writeJSON(w, http.StatusNotFound, map[string]any{"error": "no run in flight"})
		return
	}
	writeJSON(w, http.StatusOK, current)
}

func (s *Server) handleRunByID(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	stored := s.controller.RunByID(id)
	if stored == nil {
		writeJSON(w, http.StatusNotFound, map[string]any{"error": "run not found", "id": id})
		return
	}
	writeJSON(w, http.StatusOK, stored)
}

func (s *Server) handleStartRun(w http.ResponseWriter, r *http.Request) {
	var body startRunRequest
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]any{"error": "invalid json: " + err.Error()})
		return
	}
	cfg := config.DefaultRuntimeConfig()
	cfg.TargetURL = body.TargetURL
	if body.Population > 0 {
		cfg.Population = body.Population
	}
	if body.Generations > 0 {
		cfg.Generations = body.Generations
	}
	cfg.DisableNimble = body.DisableNimble
	cfg.EnableCVESeed = body.EnableCVESeed
	cfg.KubeconfigPath = s.cfg.KubeconfigPath
	cfg.OutDir = s.cfg.OutDir
	if cfg.TargetURL == "" {
		writeJSON(w, http.StatusBadRequest, map[string]any{"error": "target_url is required"})
		return
	}
	startedRunID, err := s.controller.Start(cfg)
	if err != nil {
		if errors.Is(err, errRunInFlight) {
			writeJSON(w, http.StatusConflict, map[string]any{"error": err.Error()})
			return
		}
		writeJSON(w, http.StatusInternalServerError, map[string]any{"error": err.Error()})
		return
	}
	writeJSON(w, http.StatusAccepted, map[string]any{"run_id": startedRunID, "status": "started"})
}

func (s *Server) handleCancelRun(w http.ResponseWriter, _ *http.Request) {
	cancelledID, err := s.controller.Cancel()
	if err != nil {
		writeJSON(w, http.StatusNotFound, map[string]any{"error": err.Error()})
		return
	}
	writeJSON(w, http.StatusAccepted, map[string]any{"run_id": cancelledID, "status": "cancelling"})
}

func (s *Server) handleOpenRouterIntegration(w http.ResponseWriter, _ *http.Request) {
	cfg, err := secrets.Load(s.cfg.EnvFilePath)
	if err != nil {
		writeJSON(w, http.StatusOK, map[string]any{"ok": false, "error": err.Error()})
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"ok":    cfg.HasOpenRouter(),
		"model": runner.PreferredOpenRouterModel,
	})
}

func (s *Server) handleNimbleIntegration(w http.ResponseWriter, _ *http.Request) {
	cfg, err := secrets.Load(s.cfg.EnvFilePath)
	if err != nil {
		writeJSON(w, http.StatusOK, map[string]any{"ok": false, "error": err.Error()})
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"ok": cfg.HasNimble()})
}

type startRunRequest struct {
	TargetURL     string `json:"target_url"`
	Population    int    `json:"population"`
	Generations   int    `json:"generations"`
	DisableNimble bool   `json:"disable_nimble"`
	EnableCVESeed bool   `json:"enable_cve_seed"`
}

func writeJSON(w http.ResponseWriter, status int, payload any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	enc := json.NewEncoder(w)
	enc.SetIndent("", "  ")
	_ = enc.Encode(payload)
}

// runHistoryEntry is what GET /api/runs returns per row.
type runHistoryEntry struct {
	RunID       string    `json:"run_id"`
	TargetURL   string    `json:"target_url"`
	Outcome     string    `json:"outcome"`
	BestFitness float64   `json:"best_fitness"`
	StartedAt   time.Time `json:"started_at"`
	FinishedAt  time.Time `json:"finished_at"`
	Cancelled   bool      `json:"cancelled,omitempty"`
}

func runStoreToHistoryEntry(s *store.RunStore) runHistoryEntry {
	return runHistoryEntry{
		RunID:       s.RunID,
		TargetURL:   s.TargetURL,
		Outcome:     s.Outcome,
		BestFitness: s.BestFitness,
		StartedAt:   s.StartedAt,
		FinishedAt:  s.FinishedAt,
		Cancelled:   s.Cancelled,
	}
}

// loadEnvFile is exported so tests can stub the env-file resolver without
// touching the secrets package. It is otherwise unused outside this file.
func loadEnvFile(envFilePath string) (secrets.Config, error) {
	return secrets.Load(envFilePath)
}

// makeAbsoluteOutDir is a defensive util used during config validation.
// Returns outDir unchanged if it is already absolute or if filepath.Abs
// fails (which it shouldn't in normal use). Not currently invoked by any
// route handler but kept here in case the controller wants to surface the
// resolved path in /api/integrations/health.
func makeAbsoluteOutDir(outDir string) string {
	if filepath.IsAbs(outDir) {
		return outDir
	}
	abs, err := filepath.Abs(outDir)
	if err != nil {
		return outDir
	}
	return abs
}

// ensure compile-time use of the import we keep for test scaffolding
var _ = sync.Mutex{}
var _ = fmt.Sprintf
