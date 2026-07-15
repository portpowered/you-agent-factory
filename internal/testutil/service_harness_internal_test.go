package testutil

import (
	"context"
	"errors"
	"io/fs"
	"path/filepath"
	"testing"
	"time"

	"github.com/portpowered/infinite-you/internal/testutil/testdeps"
	interfaces "github.com/portpowered/infinite-you/pkg/factory/contracts"
	factorysessionexecution "github.com/portpowered/infinite-you/pkg/factory/sessions/execution"
	"github.com/portpowered/infinite-you/pkg/service"
	"github.com/portpowered/infinite-you/pkg/transports/cli/sessionpath"
	factoryapi "github.com/portpowered/infinite-you/pkg/transports/http/generated"
	apisurface "github.com/portpowered/infinite-you/pkg/transports/mapping"
	"github.com/portpowered/infinite-you/pkg/work"
	workerexecution "github.com/portpowered/infinite-you/pkg/workers/execution"
	"go.uber.org/zap/zapcore"
)

type waitUntilCancelExecutor struct{}

func (waitUntilCancelExecutor) Execute(ctx context.Context, _ work.WorkDispatch) (workerexecution.WorkResult, error) {
	<-ctx.Done()
	return workerexecution.WorkResult{Outcome: workerexecution.OutcomeFailed}, ctx.Err()
}

func TestServiceTestHarnessMarkingFallsBackToCachedSnapshot(t *testing.T) {
	cfg := PipelineConfig(1, "processor")
	dir := ScaffoldFactoryDir(t, cfg)

	h := NewServiceTestHarness(t, dir)
	h.MockWorker("processor",
		workerexecution.WorkResult{Outcome: workerexecution.OutcomeAccepted},
		workerexecution.WorkResult{Outcome: workerexecution.OutcomeAccepted},
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

func TestServiceTestHarnessRunUntilCompleteAcceptsRunThatFinishesBeforeAvailabilityPoll(t *testing.T) {
	cfg := PipelineConfig(1, "processor")
	dir := ScaffoldFactoryDir(t, cfg)

	h := NewServiceTestHarness(t, dir)
	h.MockWorker("processor", workerexecution.WorkResult{Outcome: workerexecution.OutcomeAccepted})
	if err := h.SubmitWork("task", []byte(`{"title":"finish immediately"}`)); err != nil {
		t.Fatalf("submit work: %v", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if err := h.run(ctx); err != nil {
		t.Fatalf("run: %v", err)
	}
	runErrCh := make(chan error, 1)
	runErrCh <- nil

	if err := h.waitForRuntimeAvailability(ctx, runErrCh); err != nil {
		t.Fatalf("waitForRuntimeAvailability after clean run: %v", err)
	}
	if err := <-runErrCh; err != nil {
		t.Fatalf("preserved run result: %v", err)
	}
	if got := len(h.Marking().TokensInPlace("task:complete")); got != 1 {
		t.Fatalf("TokensInPlace(task:complete) = %d, want 1", got)
	}
}

func TestNewServiceTestHarness_WithZapLogger_PreservesCapturingLoggerThroughRun(t *testing.T) {
	cfg := PipelineConfig(1, "processor")
	dir := ScaffoldFactoryDir(t, cfg)
	logDir := t.TempDir()

	capturingLogger, observed := testdeps.CapturingZapLogger(zapcore.InfoLevel)
	h := NewServiceTestHarness(t, dir,
		WithZapLogger(capturingLogger),
		WithRuntimeFileLoggingEnabled(true),
		WithRuntimeLogDir(logDir),
		WithRuntimeInstanceID("harness-capture"),
	)
	h.MockWorker("processor", workerexecution.WorkResult{Outcome: workerexecution.OutcomeAccepted})

	if err := h.SubmitWork("task", []byte(`{"title":"capture harness logger"}`)); err != nil {
		t.Fatalf("submit work: %v", err)
	}
	h.RunUntilComplete(t, 5*time.Second)

	logs := observed.FilterMessage("factory started").All()
	if len(logs) != 1 {
		t.Fatalf("factory started logs = %d, want 1", len(logs))
	}
}

func TestNewServiceTestHarness_WithInvocationMetricsRecorder_RecordsSessionInvocationMetrics(t *testing.T) {
	cfg := PipelineConfig(1, "processor")
	dir := ScaffoldFactoryDir(t, cfg)

	recorder := testdeps.NewRecordingInvocationMetrics()
	h := NewServiceTestHarness(t, dir,
		WithInvocationMetricsRecorder(recorder),
		WithRunAsync(),
	)
	h.SetCustomExecutor("processor", waitUntilCancelExecutor{})

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	if err := h.SubmitWork("task", []byte(`{"title":"keep runtime available for invocation metrics"}`)); err != nil {
		t.Fatalf("submit work: %v", err)
	}

	errCh := h.RunInBackground(ctx)
	if err := h.WaitForRuntimeAvailability(ctx, errCh); err != nil {
		t.Fatalf("WaitForRuntimeAvailability: %v", err)
	}

	_, err := h.svc.InvokeFactorySession(ctx, sessionpath.DefaultFactorySessionID, factoryapi.InvocationRequest{})
	if err == nil {
		t.Fatal("InvokeFactorySession() error = nil, want invocation error")
	}
	if !recorder.Contains(service.InvocationMetricNormalizationAttempts, nil) {
		t.Fatalf("expected %q invocation metric via harness recorder", service.InvocationMetricNormalizationAttempts)
	}
}

func TestNewServiceTestHarness_DisablesRuntimeFileLoggingByDefault(t *testing.T) {
	cfg := PipelineConfig(1, "processor")
	dir := ScaffoldFactoryDir(t, cfg)
	logDir := t.TempDir()

	h := NewServiceTestHarness(t, dir, WithRuntimeLogDir(logDir))
	h.MockWorker("processor", workerexecution.WorkResult{Outcome: workerexecution.OutcomeAccepted})

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
