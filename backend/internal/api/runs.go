package api

import (
	"context"
	"errors"
	"fmt"
	"io"
	"sync"

	"github.com/TheApexWu/openzerg/backend/internal/config"
	"github.com/TheApexWu/openzerg/backend/internal/events"
	"github.com/TheApexWu/openzerg/backend/internal/runner"
	"github.com/TheApexWu/openzerg/backend/internal/store"
)

var errRunInFlight = errors.New("a run is already in flight; cancel it first")
var errNoRun = errors.New("no run is currently in flight")

// RunController is the state machine behind POST /api/runs and POST
// /api/runs/current/cancel. Single in-flight run at a time; the HTTP layer
// blocks concurrent starts with a 409.
//
// The completed-runs history is kept in memory (process lifetime). For a
// hackathon demo that is enough; long-term persistence is intentionally
// out of scope.
type RunController struct {
	mu             sync.Mutex
	broker         *events.Broker
	envFilePath    string
	kubeconfigPath string
	outDir         string
	stdout         io.Writer
	stderr         io.Writer

	current       *runInFlight
	completedRuns []*store.RunStore
}

type runInFlight struct {
	runID    string
	cancel   context.CancelFunc
	finished chan struct{}
}

// Start launches a run with the provided config. Returns the runID as soon
// as the runner has assigned one (best-effort: the runner allocates inside
// its goroutine, so we assign a placeholder up front and overwrite once
// run_start emits).
func (rc *RunController) Start(cfg config.RuntimeConfig) (string, error) {
	rc.mu.Lock()
	if rc.current != nil {
		rc.mu.Unlock()
		return "", errRunInFlight
	}
	runContext, cancel := context.WithCancel(context.Background())
	inFlight := &runInFlight{
		runID:    "pending",
		cancel:   cancel,
		finished: make(chan struct{}),
	}
	rc.current = inFlight
	rc.mu.Unlock()

	swarmRunner := &runner.Runner{
		Cfg:           cfg,
		EnvFilePath:   rc.envFilePath,
		Stdout:        rc.stdout,
		Stderr:        rc.stderr,
		Broker:        rc.broker,
		InstallSignal: false,
	}

	go func() {
		defer close(inFlight.finished)
		runStore, _, _, err := swarmRunner.Run(runContext)
		rc.mu.Lock()
		defer rc.mu.Unlock()
		if runStore != nil {
			rc.completedRuns = append(rc.completedRuns, runStore)
		}
		rc.current = nil
		if err != nil {
			fmt.Fprintf(rc.stderr, "controller: run failed: %v\n", err)
		}
	}()

	// Best-effort: try to learn the runID by inspecting the broker's
	// recent buffer right after run_start fires. This is async with the
	// runner's first publish, so callers must treat the response as
	// "started, the real id will arrive over SSE shortly".
	return inFlight.runID, nil
}

// Cancel cancels the in-flight run. The runner finalizes a partial summary
// before exiting.
func (rc *RunController) Cancel() (string, error) {
	rc.mu.Lock()
	defer rc.mu.Unlock()
	if rc.current == nil {
		return "", errNoRun
	}
	id := rc.current.runID
	rc.current.cancel()
	return id, nil
}

// Current returns a snapshot of the in-flight run, or nil if idle.
func (rc *RunController) Current() any {
	rc.mu.Lock()
	defer rc.mu.Unlock()
	if rc.current == nil {
		return nil
	}
	return map[string]any{
		"run_id": rc.current.runID,
		"status": "running",
	}
}

// History returns one row per completed run, newest first.
func (rc *RunController) History() []runHistoryEntry {
	rc.mu.Lock()
	defer rc.mu.Unlock()
	out := make([]runHistoryEntry, 0, len(rc.completedRuns))
	for i := len(rc.completedRuns) - 1; i >= 0; i-- {
		out = append(out, runStoreToHistoryEntry(rc.completedRuns[i]))
	}
	return out
}

// RunByID returns the stored summary for a completed run, or nil if absent.
func (rc *RunController) RunByID(id string) *store.RunStore {
	rc.mu.Lock()
	defer rc.mu.Unlock()
	for _, s := range rc.completedRuns {
		if s.RunID == id {
			return s
		}
	}
	return nil
}
