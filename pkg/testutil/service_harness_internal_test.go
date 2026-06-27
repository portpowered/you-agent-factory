package testutil

import (
	"context"
	"errors"
	"io/fs"
	"path/filepath"
	"testing"
	"time"

	factoryapi "github.com/portpowered/infinite-you/pkg/api/generated"
	"github.com/portpowered/infinite-you/pkg/apisurface"
	"github.com/portpowered/infinite-you/pkg/factorysessionexecution"
	"github.com/portpowered/infinite-you/pkg/interfaces"
)

func TestServiceTestHarnessMarkingFallsBackToCachedSnapshot(t *testing.T) {
	cfg := PipelineConfig(1, "processor")
	dir := ScaffoldFactoryDir(t, cfg)

	h := NewServiceTestHarness(t, dir)
	h.MockWorker("processor",
		interfaces.WorkResult{Outcome: interfaces.OutcomeAccepted},
		interfaces.WorkResult{Outcome: interfaces.OutcomeAccepted},
	)

	if err := h.SubmitWork("task", []byte(`{"title":"cache final marking"}`)); err != nil {
		t.Fatalf("submit work: %v", err)
	}
	h.RunUntilComplete(t, 5*time.Second)

	// Simulate the runtime disappearing after the run completes. Marking()
	// should still expose the last successful terminal snapshot.
	h.svc = nil

	snap := h.Marking()
	if snap == nil {
		t.Fatal("Marking() returned nil snapshot")
	}
	if got := len(snap.TokensInPlace("task:complete")); got != 1 {
		t.Fatalf("TokensInPlace(task:complete) = %d, want 1", got)
	}
	if got := len(snap.TokensInPlace("task:init")); got != 0 {
		t.Fatalf("TokensInPlace(task:init) = %d, want 0", got)
	}
}

func TestNewServiceTestHarness_DisablesRuntimeFileLoggingByDefault(t *testing.T) {
	cfg := PipelineConfig(1, "processor")
	dir := ScaffoldFactoryDir(t, cfg)
	logDir := t.TempDir()

	h := NewServiceTestHarness(t, dir, WithRuntimeLogDir(logDir))
	h.MockWorker("processor", interfaces.WorkResult{Outcome: interfaces.OutcomeAccepted})

	if err := h.SubmitWork("task", []byte(`{"title":"no incidental runtime log file"}`)); err != nil {
		t.Fatalf("submit work: %v", err)
	}
	h.RunUntilComplete(t, 5*time.Second)

	diagnostics := h.svc.RuntimeLogDiagnostics()
	if diagnostics.Path != "" {
		t.Fatalf("RuntimeLogDiagnostics().Path = %q, want empty when runtime file logging is disabled", diagnostics.Path)
	}

	var logFiles []string
	err := filepath.WalkDir(logDir, func(path string, d fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if d.IsDir() {
			return nil
		}
		if filepath.Ext(path) == ".log" {
			logFiles = append(logFiles, path)
		}
		return nil
	})
	if err != nil {
		t.Fatalf("WalkDir(%s): %v", logDir, err)
	}
	if len(logFiles) != 0 {
		t.Fatalf("runtime log files = %v, want none", logFiles)
	}
}

func TestMockFactory_LiveSessionPauseResume_ReturnsTypedControlForExistingSession(t *testing.T) {
	ctx := context.Background()
	mock := &MockFactory{
		State: interfaces.FactoryStateRunning,
		SessionFactories: map[string]*MockFactory{
			"live-sess-001": {
				State: interfaces.FactoryStateRunning,
			},
		},
	}

	pauseResp, err := mock.PauseLiveFactorySession(ctx, "live-sess-001", factoryapi.FactorySessionLifecycleControlRequest{})
	if err != nil {
		t.Fatalf("PauseLiveFactorySession() error = %v", err)
	}
	if pauseResp.Outcome != factoryapi.FactorySessionLifecycleControlOutcomeAccepted {
		t.Fatalf("pause outcome = %s, want ACCEPTED", pauseResp.Outcome)
	}
	if pauseResp.Status != factoryapi.FactorySessionDurableLifecycleStatusPaused {
		t.Fatalf("pause status = %s, want PAUSED", pauseResp.Status)
	}

	resumeResp, err := mock.ResumeLiveFactorySession(ctx, "live-sess-001", factoryapi.FactorySessionLifecycleControlRequest{})
	if err != nil {
		t.Fatalf("ResumeLiveFactorySession() error = %v", err)
	}
	if resumeResp.Outcome != factoryapi.FactorySessionLifecycleControlOutcomeAccepted {
		t.Fatalf("resume outcome = %s, want ACCEPTED", resumeResp.Outcome)
	}
	if resumeResp.Status != factoryapi.FactorySessionDurableLifecycleStatusRunning {
		t.Fatalf("resume status = %s, want RUNNING", resumeResp.Status)
	}

	noOpResp, err := mock.ResumeLiveFactorySession(ctx, "live-sess-001", factoryapi.FactorySessionLifecycleControlRequest{})
	if err != nil {
		t.Fatalf("ResumeLiveFactorySession() on running session error = %v", err)
	}
	if noOpResp.Outcome != factoryapi.FactorySessionLifecycleControlOutcomeNoOp {
		t.Fatalf("resume no-op outcome = %s, want NO_OP", noOpResp.Outcome)
	}
	if noOpResp.Status != factoryapi.FactorySessionDurableLifecycleStatusRunning {
		t.Fatalf("resume no-op status = %s, want RUNNING", noOpResp.Status)
	}
}

func TestMockFactory_LiveSessionPauseResume_ReturnsNotFoundForMissingSession(t *testing.T) {
	ctx := context.Background()
	mock := &MockFactory{
		SessionFactories: map[string]*MockFactory{},
	}

	_, err := mock.PauseLiveFactorySession(ctx, "missing-session", factoryapi.FactorySessionLifecycleControlRequest{})
	if err == nil {
		t.Fatal("PauseLiveFactorySession() error = nil, want not found")
	}
	if !errors.Is(err, apisurface.ErrFactorySessionNotFound) {
		t.Fatalf("PauseLiveFactorySession() error = %v, want %v", err, apisurface.ErrFactorySessionNotFound)
	}

	_, err = mock.ResumeLiveFactorySession(ctx, "missing-session", factoryapi.FactorySessionLifecycleControlRequest{})
	if err == nil {
		t.Fatal("ResumeLiveFactorySession() error = nil, want not found")
	}
	if !errors.Is(err, apisurface.ErrFactorySessionNotFound) {
		t.Fatalf("ResumeLiveFactorySession() error = %v, want %v", err, apisurface.ErrFactorySessionNotFound)
	}
}

func TestMockFactory_LiveSessionPauseResume_ReturnsControlErrorForInvalidState(t *testing.T) {
	ctx := context.Background()
	mock := &MockFactory{
		SessionFactories: map[string]*MockFactory{
			"live-sess-001": {
				State: interfaces.FactoryStateCompleted,
			},
		},
	}

	_, err := mock.PauseLiveFactorySession(ctx, "live-sess-001", factoryapi.FactorySessionLifecycleControlRequest{})
	if err == nil {
		t.Fatal("PauseLiveFactorySession() error = nil, want control error")
	}
	var controlErr *factorysessionexecution.ControlError
	if !errors.As(err, &controlErr) {
		t.Fatalf("PauseLiveFactorySession() error = %T(%v), want *ControlError", err, err)
	}
	if controlErr.Outcome != factorysessionexecution.LifecycleControlOutcomeTerminalSession {
		t.Fatalf("control outcome = %s, want TERMINAL_SESSION", controlErr.Outcome)
	}
}
