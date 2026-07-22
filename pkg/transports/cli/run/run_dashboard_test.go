package run

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"net"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	serviceedges "github.com/portpowered/infinite-you/pkg/services/edges"
	interfaces "github.com/portpowered/infinite-you/pkg/services/factory_definitions"
	factoryruntime "github.com/portpowered/infinite-you/pkg/services/factory_runtime"
	"github.com/portpowered/infinite-you/pkg/services/work"
	workerexecution "github.com/portpowered/infinite-you/pkg/services/workers"
	"go.uber.org/zap"
	"go.uber.org/zap/zaptest/observer"
)

func TestRun_DefaultDashboardRendering_PrintsSimpleDashboardOutput(t *testing.T) {
	dir, workFile := writeDashboardRunFixture(t)
	executionBaseDir := t.TempDir()

	output, err := runWithCapturedStdout(t, RunConfig{
		Dir:                     dir,
		ExecutionBaseDir:        executionBaseDir,
		Port:                    0,
		WorkFile:                workFile,
		MockWorkersEnabled:      true,
		DisableDefaultRecording: true,
		Logger:                  zap.NewNop(),
	})
	if err != nil {
		t.Fatalf("Run: %v", err)
	}

	if !strings.Contains(output, "Factory:") {
		t.Fatalf("expected simple dashboard output, got %q", output)
	}
}

func TestRun_SuppressDashboardRendering_SkipsSimpleDashboardOutput(t *testing.T) {
	dir, workFile := writeDashboardRunFixture(t)

	output, err := runWithCapturedStdout(t, RunConfig{
		Dir:                        dir,
		ExecutionBaseDir:           t.TempDir(),
		Port:                       0,
		WorkFile:                   workFile,
		MockWorkersEnabled:         true,
		DisableDefaultRecording:    true,
		SuppressDashboardRendering: true,
		Logger:                     zap.NewNop(),
	})
	if err != nil {
		t.Fatalf("Run: %v", err)
	}

	if output != "" {
		t.Fatalf("expected no simple dashboard output, got %q", output)
	}
}

func TestRun_CleanInvocationKeepsOperatorChatterOffStdout(t *testing.T) {
	dir, workFile := writeDashboardRunFixture(t)

	var startupOut bytes.Buffer
	output, err := runWithCapturedStdout(t, RunConfig{
		Dir:                     dir,
		ExecutionBaseDir:        t.TempDir(),
		Port:                    0,
		WorkFile:                workFile,
		MockWorkersEnabled:      true,
		CleanInvocation:         true,
		DisableDefaultRecording: true,
		StartupOutput:           &startupOut,
		Logger:                  zap.NewNop(),
	})
	if err != nil {
		t.Fatalf("Run: %v", err)
	}

	if output != "mock worker accepted" {
		t.Fatalf("stdout = %q, want primary clean invocation output", output)
	}
	assertNoOperatorChatter(t, output)
	if startupOut.Len() != 0 {
		t.Fatalf("startup output = %q, want clean invocation to suppress operator chatter", startupOut.String())
	}
}

func TestRun_CleanInvocationEmitsPrimaryTextOutputRepeatedly(t *testing.T) {
	for i := 0; i < 2; i++ {
		dir, workFile := writeDashboardRunFixture(t)

		output, err := runWithCapturedStdout(t, RunConfig{
			Dir:                     dir,
			ExecutionBaseDir:        t.TempDir(),
			Port:                    0,
			WorkFile:                workFile,
			MockWorkersEnabled:      true,
			CleanInvocation:         true,
			DisableDefaultRecording: true,
			Logger:                  zap.NewNop(),
		})
		if err != nil {
			t.Fatalf("Run iteration %d: %v", i, err)
		}
		if output != "mock worker accepted" {
			t.Fatalf("iteration %d stdout = %q, want primary clean invocation output", i, output)
		}
		assertNoOperatorChatter(t, output)
	}
}

func TestRun_CleanInvocationJSONEmitsSinglePrimaryResultObject(t *testing.T) {
	dir, workFile := writeDashboardRunFixture(t)

	output, err := runWithCapturedStdout(t, RunConfig{
		Dir:                     dir,
		ExecutionBaseDir:        t.TempDir(),
		Port:                    0,
		WorkFile:                workFile,
		MockWorkersEnabled:      true,
		CleanInvocation:         true,
		JSON:                    true,
		DisableDefaultRecording: true,
		Logger:                  zap.NewNop(),
	})
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	assertNoOperatorChatter(t, output)

	var got cleanInvocationSuccess
	if err := json.Unmarshal([]byte(output), &got); err != nil {
		t.Fatalf("stdout is not one JSON object: %v\n%s", err, output)
	}
	if got.Output != "mock worker accepted" {
		t.Fatalf("output = %q, want primary clean invocation output", got.Output)
	}
	if got.WorkID != "dashboard-render-test-work" ||
		got.WorkTypeName != "task" ||
		got.TraceID != "dashboard-render-test-trace" ||
		got.SessionID != defaultFactorySessionID {
		t.Fatalf("json result = %#v", got)
	}
}

func TestRun_CleanInvocationSuccessRecordsStructuredLogAndMetrics(t *testing.T) {
	resetCleanInvocationMetricsForTest()

	dir, workFile := writeDashboardRunFixture(t)
	core, observed := observer.New(zap.InfoLevel)

	output, err := runWithCapturedStdout(t, RunConfig{
		Dir:                        dir,
		ExecutionBaseDir:           t.TempDir(),
		Port:                       0,
		WorkFile:                   workFile,
		MockWorkersEnabled:         true,
		CleanInvocation:            true,
		CleanInvocationInputSource: InvocationInputSourcePositional,
		DisableDefaultRecording:    true,
		Logger:                     zap.New(core),
	})
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if output != "mock worker accepted" {
		t.Fatalf("stdout = %q, want primary clean invocation output", output)
	}

	entry := observed.FilterMessage(cleanInvocationLogMessageCompleted).AllUntimed()
	if len(entry) != 1 {
		t.Fatalf("completed logs = %d, want 1", len(entry))
	}
	fields := entry[0].ContextMap()
	if fields["outcome"] != cleanInvocationOutcomeSuccess {
		t.Fatalf("outcome = %#v, want success", fields["outcome"])
	}
	if fields["mode"] != cleanInvocationModeLabel {
		t.Fatalf("mode = %#v, want clean", fields["mode"])
	}
	if fields["inputSource"] != "positional_prompt" {
		t.Fatalf("inputSource = %#v, want positional_prompt", fields["inputSource"])
	}
	if fields["workId"] != "dashboard-render-test-work" {
		t.Fatalf("workId = %#v", fields["workId"])
	}
	if fields["workTypeName"] != "task" {
		t.Fatalf("workTypeName = %#v", fields["workTypeName"])
	}
	if fields["traceId"] != "dashboard-render-test-trace" {
		t.Fatalf("traceId = %#v", fields["traceId"])
	}
	if fields["sessionId"] != defaultFactorySessionID {
		t.Fatalf("sessionId = %#v", fields["sessionId"])
	}
	if duration, ok := fields["durationMs"].(int64); !ok || duration < 0 {
		t.Fatalf("durationMs = %#v, want non-negative int64", fields["durationMs"])
	}

	if got := snapshotCleanInvocationMetrics(); got != (CleanInvocationMetricsSnapshot{
		Attempts:  1,
		Successes: 1,
	}) {
		t.Fatalf("metrics = %#v", got)
	}
}

func TestRun_CleanInvocationFailureReturnsStableErrorAndNoStdout(t *testing.T) {
	originalBuilder := openTestRuntimeRunner
	defer func() {
		openTestRuntimeRunner = originalBuilder
	}()

	_, workFile := writeDashboardRunFixture(t)
	openTestRuntimeRunner = func(_ context.Context, _ *testRuntimeSelections, _ serviceedges.Edges) (factoryServiceRunner, error) {
		return stubFactoryService{
			run: func(context.Context) error { return nil },
			snapshot: func(context.Context) (*interfaces.EngineStateSnapshot[factoryruntime.PetriMarkingSnapshot, *factoryruntime.Net], error) {
				return failedCleanInvocationSnapshot("mock worker rejected"), nil
			},
		}, nil
	}

	output, err := runWithCapturedStdout(t, RunConfig{
		WorkFile:                workFile,
		CleanInvocation:         true,
		DisableDefaultRecording: true,
		Logger:                  zap.NewNop(),
	}, openTestRuntimeRunner)
	if output != "" {
		t.Fatalf("stdout = %q, want empty on failure", output)
	}

	var invocationErr *InvocationError
	if !errors.As(err, &invocationErr) {
		t.Fatalf("error = %#v, want InvocationError", err)
	}
	if invocationErr.Code != InvocationErrorCodeFailed {
		t.Fatalf("code = %q, want %q", invocationErr.Code, InvocationErrorCodeFailed)
	}
	if invocationErr.Message != "clean invocation failed: mock worker rejected" {
		t.Fatalf("message = %q", invocationErr.Message)
	}
}

func TestRun_CleanInvocationTimeoutReturnsStableErrorAndNoStdout(t *testing.T) {
	originalBuilder := openTestRuntimeRunner
	defer func() {
		openTestRuntimeRunner = originalBuilder
	}()

	_, workFile := writeDashboardRunFixture(t)
	openTestRuntimeRunner = func(_ context.Context, _ *testRuntimeSelections, _ serviceedges.Edges) (factoryServiceRunner, error) {
		return stubFactoryService{
			run: func(context.Context) error { return nil },
			snapshot: func(context.Context) (*interfaces.EngineStateSnapshot[factoryruntime.PetriMarkingSnapshot, *factoryruntime.Net], error) {
				return timedOutCleanInvocationSnapshot(), nil
			},
		}, nil
	}

	output, err := runWithCapturedStdout(t, RunConfig{
		WorkFile:                workFile,
		CleanInvocation:         true,
		DisableDefaultRecording: true,
		Logger:                  zap.NewNop(),
	}, openTestRuntimeRunner)
	if output != "" {
		t.Fatalf("stdout = %q, want empty on timeout", output)
	}

	var invocationErr *InvocationError
	if !errors.As(err, &invocationErr) {
		t.Fatalf("error = %#v, want InvocationError", err)
	}
	if invocationErr.Code != InvocationErrorCodeTimeout {
		t.Fatalf("code = %q, want %q", invocationErr.Code, InvocationErrorCodeTimeout)
	}
	if invocationErr.Message != "clean invocation timed out" {
		t.Fatalf("message = %q", invocationErr.Message)
	}
}

func TestRun_CleanInvocationCancellationReturnsStableErrorAndNoStdout(t *testing.T) {
	originalBuilder := openTestRuntimeRunner
	defer func() {
		openTestRuntimeRunner = originalBuilder
	}()

	_, workFile := writeDashboardRunFixture(t)
	openTestRuntimeRunner = func(_ context.Context, _ *testRuntimeSelections, _ serviceedges.Edges) (factoryServiceRunner, error) {
		return stubFactoryService{
			run: func(context.Context) error { return context.Canceled },
			snapshot: func(context.Context) (*interfaces.EngineStateSnapshot[factoryruntime.PetriMarkingSnapshot, *factoryruntime.Net], error) {
				return nil, errors.New("snapshot not needed")
			},
		}, nil
	}

	output, err := runWithCapturedStdout(t, RunConfig{
		WorkFile:                workFile,
		CleanInvocation:         true,
		DisableDefaultRecording: true,
		Logger:                  zap.NewNop(),
	}, openTestRuntimeRunner)
	if output != "" {
		t.Fatalf("stdout = %q, want empty on cancellation", output)
	}

	var invocationErr *InvocationError
	if !errors.As(err, &invocationErr) {
		t.Fatalf("error = %#v, want InvocationError", err)
	}
	if invocationErr.Code != InvocationErrorCodeCancelled {
		t.Fatalf("code = %q, want %q", invocationErr.Code, InvocationErrorCodeCancelled)
	}
	if invocationErr.Message != "clean invocation cancelled" {
		t.Fatalf("message = %q", invocationErr.Message)
	}
}

func TestRun_CleanInvocationCancellationRecordsStructuredLogAndMetrics(t *testing.T) {
	resetCleanInvocationMetricsForTest()

	originalBuilder := openTestRuntimeRunner
	defer func() {
		openTestRuntimeRunner = originalBuilder
	}()

	_, workFile := writeDashboardRunFixture(t)
	core, observed := observer.New(zap.InfoLevel)
	openTestRuntimeRunner = func(_ context.Context, _ *testRuntimeSelections, _ serviceedges.Edges) (factoryServiceRunner, error) {
		return stubFactoryService{
			run: func(context.Context) error { return context.Canceled },
			snapshot: func(context.Context) (*interfaces.EngineStateSnapshot[factoryruntime.PetriMarkingSnapshot, *factoryruntime.Net], error) {
				return nil, errors.New("snapshot not needed")
			},
		}, nil
	}

	err := Run(context.Background(), RunConfig{
		WorkFile:                   workFile,
		CleanInvocation:            true,
		CleanInvocationInputSource: InvocationInputSourceWorkFile,
		DisableDefaultRecording:    true,
		Logger:                     zap.New(core),
	})
	if err == nil {
		t.Fatal("expected cancellation error")
	}

	entry := observed.FilterMessage(cleanInvocationLogMessageCompleted).AllUntimed()
	if len(entry) != 1 {
		t.Fatalf("completed logs = %d, want 1", len(entry))
	}
	fields := entry[0].ContextMap()
	if fields["outcome"] != cleanInvocationOutcomeCancelled {
		t.Fatalf("outcome = %#v, want cancelled", fields["outcome"])
	}
	if fields["errorCode"] != InvocationErrorCodeCancelled {
		t.Fatalf("errorCode = %#v, want %q", fields["errorCode"], InvocationErrorCodeCancelled)
	}
	if fields["inputSource"] != "work_file" {
		t.Fatalf("inputSource = %#v, want work_file", fields["inputSource"])
	}

	if got := snapshotCleanInvocationMetrics(); got != (CleanInvocationMetricsSnapshot{
		Attempts:      1,
		Cancellations: 1,
	}) {
		t.Fatalf("metrics = %#v", got)
	}
}

func TestRun_CleanInvocationKeepsStdoutEmptyUntilTerminalOutcome(t *testing.T) {
	originalBuilder := openTestRuntimeRunner
	defer func() {
		openTestRuntimeRunner = originalBuilder
	}()

	_, workFile := writeDashboardRunFixture(t)
	started := make(chan struct{})
	release := make(chan struct{})
	openTestRuntimeRunner = func(_ context.Context, _ *testRuntimeSelections, _ serviceedges.Edges) (factoryServiceRunner, error) {
		return stubFactoryService{
			run: func(context.Context) error {
				close(started)
				<-release
				return context.Canceled
			},
			snapshot: func(context.Context) (*interfaces.EngineStateSnapshot[factoryruntime.PetriMarkingSnapshot, *factoryruntime.Net], error) {
				return nil, errors.New("snapshot not needed")
			},
		}, nil
	}

	var stdout bytes.Buffer
	errCh := make(chan error, 1)
	go func() {
		errCh <- Run(context.Background(), RunConfig{
			WorkFile:                workFile,
			CleanInvocation:         true,
			Output:                  &stdout,
			DisableDefaultRecording: true,
			Logger:                  zap.NewNop(),
		})
	}()

	select {
	case <-started:
	case err := <-errCh:
		t.Fatalf("Run returned before blocking phase: %v", err)
	case <-time.After(time.Second):
		t.Fatal("timed out waiting for clean invocation startup")
	}
	if got := stdout.String(); got != "" {
		t.Fatalf("stdout before terminal outcome = %q, want empty", got)
	}

	close(release)

	err := <-errCh
	if got := stdout.String(); got != "" {
		t.Fatalf("stdout after terminal outcome = %q, want empty", got)
	}
	var invocationErr *InvocationError
	if !errors.As(err, &invocationErr) {
		t.Fatalf("error = %#v, want InvocationError", err)
	}
	if invocationErr.Code != InvocationErrorCodeCancelled {
		t.Fatalf("code = %q, want %q", invocationErr.Code, InvocationErrorCodeCancelled)
	}
}

func TestRun_ContinuouslyUsesServiceModeUntilCanceled(t *testing.T) {
	originalBuilder := openTestRuntimeRunner
	defer func() {
		openTestRuntimeRunner = originalBuilder
	}()

	started := make(chan struct{})
	var capturedMode interfaces.RuntimeMode
	openTestRuntimeRunner = func(_ context.Context, cfg *testRuntimeSelections, _ serviceedges.Edges) (factoryServiceRunner, error) {
		capturedMode = cfg.RuntimeMode
		return stubFactoryService{
			run: func(ctx context.Context) error {
				close(started)
				<-ctx.Done()
				return nil
			},
		}, nil
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	errCh := make(chan error, 1)
	go func() {
		errCh <- Run(ctx, RunConfig{Continuously: true})
	}()

	select {
	case <-started:
	case err := <-errCh:
		t.Fatalf("Run returned before cancellation: %v", err)
	case <-time.After(time.Second):
		t.Fatal("timed out waiting for continuous run to start")
	}

	if capturedMode != interfaces.RuntimeModeService {
		t.Fatalf("runtime mode = %q, want %q", capturedMode, interfaces.RuntimeModeService)
	}

	select {
	case err := <-errCh:
		t.Fatalf("Run returned before cancellation: %v", err)
	case <-time.After(100 * time.Millisecond):
	}

	cancel()

	select {
	case err := <-errCh:
		if err != nil {
			t.Fatalf("Run after cancellation: %v", err)
		}
	case <-time.After(time.Second):
		t.Fatal("timed out waiting for continuous run to stop after cancellation")
	}
}

func assertNoOperatorChatter(t *testing.T, output string) {
	t.Helper()

	for _, forbidden := range []string{
		"Factory initiated",
		"Dashboard URL",
		"Runtime log",
		"Opening dashboard",
		"Factory:",
		"Recording saved",
	} {
		if strings.Contains(output, forbidden) {
			t.Fatalf("output = %q, want no %q chatter", output, forbidden)
		}
	}
}

func writeDashboardRunFixture(t *testing.T) (string, string) {
	t.Helper()

	dir := t.TempDir()
	writeFile(t, filepath.Join(dir, "factory.json"), `{
  "name": "dashboard-run-fixture",
  "workTypes": [
    {
      "name": "task",
      "states": [
        { "name": "init", "type": "INITIAL" },
        { "name": "done", "type": "TERMINAL" },
        { "name": "failed", "type": "FAILED" }
      ]
    }
  ],
  "workers": [
    { "name": "script-worker" }
  ],
  "workstations": [
    {
      "name": "run-script",
      "worker": "script-worker",
      "inputs": [{ "workType": "task", "state": "init" }],
      "outputs": [{ "workType": "task", "state": "done" }],
      "onFailure": [{"workType": "task", "state": "failed"}]
    }
  ]
}
`)
	writeFile(t, filepath.Join(dir, "workers", "script-worker", "AGENTS.md"), `---
type: SCRIPT_WORKER
command: echo
args:
  - "dashboard-test"
---
`)
	writeFile(t, filepath.Join(dir, "workstations", "run-script", "AGENTS.md"), `---
type: MODEL_WORKSTATION
---
Run the script.
`)

	workFile := filepath.Join(t.TempDir(), "work.json")
	req := work.WorkRequest{
		Type: work.WorkRequestTypeFactoryRequestBatch,
		Works: []work.Work{{
			Name:       "dashboard-render-test-work",
			WorkID:     "dashboard-render-test-work",
			WorkTypeID: "task",
			TraceID:    "dashboard-render-test-trace",
			Payload:    "exercise dashboard rendering",
		}},
	}
	data, err := json.Marshal(req)
	if err != nil {
		t.Fatalf("marshal work file: %v", err)
	}
	writeFile(t, workFile, string(data))

	return dir, workFile
}

func unusedTCPPort(t *testing.T) int {
	t.Helper()

	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen on unused TCP port: %v", err)
	}
	defer listener.Close()

	addr, ok := listener.Addr().(*net.TCPAddr)
	if !ok {
		t.Fatalf("listener address has type %T, want *net.TCPAddr", listener.Addr())
	}
	return addr.Port
}

func listenOnBusyTCPPort(t *testing.T) (net.Listener, int) {
	t.Helper()

	listener, err := net.Listen("tcp", ":0")
	if err != nil {
		t.Fatalf("listen on busy TCP port fixture: %v", err)
	}

	addr, ok := listener.Addr().(*net.TCPAddr)
	if !ok {
		listener.Close()
		t.Fatalf("listener address has type %T, want *net.TCPAddr", listener.Addr())
	}
	return listener, addr.Port
}

func writeFile(t *testing.T, path, content string) {
	t.Helper()

	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatalf("create parent dir for %s: %v", path, err)
	}
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatalf("write %s: %v", path, err)
	}
}

func runWithCapturedStdout(
	t *testing.T,
	cfg RunConfig,
	builders ...testRuntimeRunnerOpener,
) (string, error) {
	t.Helper()
	var output bytes.Buffer
	if cfg.ExecutionBaseDir == "" && cfg.Dir != "" {
		cfg.ExecutionBaseDir = cfg.Dir
	}
	if cfg.WorkRequestFileLoader == nil {
		cfg.WorkRequestFileLoader = func(path string) (work.WorkRequest, error) {
			data, err := os.ReadFile(path)
			if err != nil {
				return work.WorkRequest{}, err
			}
			var request work.WorkRequest
			if err := json.Unmarshal(data, &request); err != nil {
				return work.WorkRequest{}, err
			}
			return request, nil
		}
	}

	cfg.Output = &output
	builder := adaptTestRuntimeRunnerOpener(buildTransportTestRuntime)
	if len(builders) > 0 {
		builder = builders[0]
	}
	runErr := runWithTestRuntimeRunner(context.Background(), cfg, builder)

	return output.String(), runErr
}

func failedCleanInvocationSnapshot(reason string) *interfaces.EngineStateSnapshot[factoryruntime.PetriMarkingSnapshot, *factoryruntime.Net] {
	return &interfaces.EngineStateSnapshot[factoryruntime.PetriMarkingSnapshot, *factoryruntime.Net]{
		DispatchHistory: []interfaces.CompletedDispatch{{
			Outcome: workerexecution.OutcomeFailed,
			Reason:  reason,
			ConsumedTokens: []factoryruntime.RuntimeToken{{
				ID:      "failed-token",
				PlaceID: "task:init",
				Color: factoryruntime.RuntimeTokenColor{
					WorkID:     "dashboard-render-test-work",
					WorkTypeID: "task",
					TraceID:    "dashboard-render-test-trace",
				},
			}},
		}},
	}
}

func timedOutCleanInvocationSnapshot() *interfaces.EngineStateSnapshot[factoryruntime.PetriMarkingSnapshot, *factoryruntime.Net] {
	return &interfaces.EngineStateSnapshot[factoryruntime.PetriMarkingSnapshot, *factoryruntime.Net]{
		DispatchHistory: []interfaces.CompletedDispatch{{
			Outcome: workerexecution.OutcomeFailed,
			ConsumedTokens: []factoryruntime.RuntimeToken{{
				ID:      "timeout-token",
				PlaceID: "task:init",
				Color: factoryruntime.RuntimeTokenColor{
					WorkID:     "dashboard-render-test-work",
					WorkTypeID: "task",
					TraceID:    "dashboard-render-test-trace",
				},
			}},
			FailureMetadata: &workerexecution.WorkFailureMetadata{
				Family: workerexecution.WorkFailureFamilyRetryable,
				Type:   workerexecution.WorkFailureTypeTimeout,
			},
		}},
	}
}
