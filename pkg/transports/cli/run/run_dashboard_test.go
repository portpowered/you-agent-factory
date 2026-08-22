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
	"github.com/portpowered/infinite-you/pkg/services/work"
	factoryapi "github.com/portpowered/infinite-you/pkg/transports/http/generated"
	apisurface "github.com/portpowered/infinite-you/pkg/transports/mapping"
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

	var got factoryapi.InvocationResponse
	if err := json.Unmarshal([]byte(output), &got); err != nil {
		t.Fatalf("stdout is not one JSON object: %v\n%s", err, output)
	}
	if got.Status != factoryapi.InvocationTerminalStatusCompleted {
		t.Fatalf("status = %q, want completed", got.Status)
	}
	if got.PrimaryResult == nil || len(*got.PrimaryResult) != 1 {
		t.Fatalf("primary result = %#v, want one content part", got.PrimaryResult)
	}
	part, partErr := (*got.PrimaryResult)[0].AsWorkTextContentPart()
	if partErr != nil || part.Text != "mock worker accepted" {
		t.Fatalf("primary result = %#v, want accepted text", got.PrimaryResult)
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
	originalInvocation := openTestInvocationRunner
	defer func() {
		openTestInvocationRunner = originalInvocation
	}()

	_, workFile := writeDashboardRunFixture(t)
	openTestInvocationRunner = func(_ context.Context, _ *testRuntimeSelections, _ serviceedges.Edges) (InvocationRunner, error) {
		return stubInvocationService{
			run: func(context.Context) error { return nil },
			invoke: func(context.Context, string, factoryapi.InvocationRequest) (apisurface.FactoryInvocationResult, error) {
				return dashboardCleanInvocationResultWithStatus(
					interfaces.InvocationTerminalStatusFailed,
					InvocationErrorCodeFailed,
					"mock worker rejected",
				), nil
			},
		}, nil
	}

	output, err := runWithCapturedStdout(t, RunConfig{
		WorkFile:                workFile,
		CleanInvocation:         true,
		DisableDefaultRecording: true,
		Logger:                  zap.NewNop(),
	})
	if output != "" {
		t.Fatalf("stdout = %q, want empty on failure", output)
	}

	assertCleanInvocationCLIError(t, err, InvocationErrorCodeFailed, "mock worker rejected")
}

func TestRun_CleanInvocationTimeoutReturnsStableErrorAndNoStdout(t *testing.T) {
	originalInvocation := openTestInvocationRunner
	defer func() {
		openTestInvocationRunner = originalInvocation
	}()

	_, workFile := writeDashboardRunFixture(t)
	openTestInvocationRunner = func(_ context.Context, _ *testRuntimeSelections, _ serviceedges.Edges) (InvocationRunner, error) {
		return stubInvocationService{
			run: func(context.Context) error { return nil },
			invoke: func(context.Context, string, factoryapi.InvocationRequest) (apisurface.FactoryInvocationResult, error) {
				return dashboardCleanInvocationResultWithStatus(
					interfaces.InvocationTerminalStatusTimedOut,
					InvocationErrorCodeTimeout,
					"clean invocation timed out",
				), nil
			},
		}, nil
	}

	output, err := runWithCapturedStdout(t, RunConfig{
		WorkFile:                workFile,
		CleanInvocation:         true,
		DisableDefaultRecording: true,
		Logger:                  zap.NewNop(),
	})
	if output != "" {
		t.Fatalf("stdout = %q, want empty on timeout", output)
	}

	assertCleanInvocationCLIError(t, err, InvocationErrorCodeTimeout, "clean invocation timed out")
}

func TestRun_CleanInvocationCancellationReturnsStableErrorAndNoStdout(t *testing.T) {
	originalInvocation := openTestInvocationRunner
	defer func() {
		openTestInvocationRunner = originalInvocation
	}()

	_, workFile := writeDashboardRunFixture(t)
	openTestInvocationRunner = func(_ context.Context, _ *testRuntimeSelections, _ serviceedges.Edges) (InvocationRunner, error) {
		return stubInvocationService{
			run: func(context.Context) error { return nil },
			invoke: func(context.Context, string, factoryapi.InvocationRequest) (apisurface.FactoryInvocationResult, error) {
				return dashboardCleanInvocationResultWithStatus(
					interfaces.InvocationTerminalStatusCanceled,
					InvocationErrorCodeCancelled,
					"clean invocation cancelled",
				), nil
			},
		}, nil
	}

	output, err := runWithCapturedStdout(t, RunConfig{
		WorkFile:                workFile,
		CleanInvocation:         true,
		DisableDefaultRecording: true,
		Logger:                  zap.NewNop(),
	})
	if output != "" {
		t.Fatalf("stdout = %q, want empty on cancellation", output)
	}

	assertCleanInvocationCLIError(t, err, InvocationErrorCodeCancelled, "clean invocation cancelled")
}

func TestRun_CleanInvocationCancellationRecordsStructuredLogAndMetrics(t *testing.T) {
	resetCleanInvocationMetricsForTest()

	originalInvocation := openTestInvocationRunner
	defer func() {
		openTestInvocationRunner = originalInvocation
	}()

	_, workFile := writeDashboardRunFixture(t)
	core, observed := observer.New(zap.InfoLevel)
	openTestInvocationRunner = func(_ context.Context, _ *testRuntimeSelections, _ serviceedges.Edges) (InvocationRunner, error) {
		return stubInvocationService{
			run: func(context.Context) error { return nil },
			invoke: func(context.Context, string, factoryapi.InvocationRequest) (apisurface.FactoryInvocationResult, error) {
				return dashboardCleanInvocationResultWithStatus(
					interfaces.InvocationTerminalStatusCanceled,
					InvocationErrorCodeCancelled,
					"clean invocation cancelled",
				), nil
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
	originalInvocation := openTestInvocationRunner
	defer func() {
		openTestInvocationRunner = originalInvocation
	}()

	_, workFile := writeDashboardRunFixture(t)
	started := make(chan struct{})
	release := make(chan struct{})
	openTestInvocationRunner = func(_ context.Context, _ *testRuntimeSelections, _ serviceedges.Edges) (InvocationRunner, error) {
		return stubInvocationService{
			run: func(context.Context) error { return nil },
			invoke: func(_ context.Context, _ string, _ factoryapi.InvocationRequest) (apisurface.FactoryInvocationResult, error) {
				close(started)
				<-release
				return dashboardCleanInvocationResultWithStatus(
					interfaces.InvocationTerminalStatusCanceled,
					InvocationErrorCodeCancelled,
					"clean invocation cancelled",
				), nil
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
	assertCleanInvocationCLIError(t, err, InvocationErrorCodeCancelled, "clean invocation cancelled")
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

func dashboardCleanInvocationResult() apisurface.FactoryInvocationResult {
	return dashboardCleanInvocationResultWithStatus(
		interfaces.InvocationTerminalStatusCompleted,
		"",
		"",
	)
}

func dashboardCleanInvocationResultWithStatus(
	status interfaces.InvocationTerminalStatus,
	code string,
	message string,
) apisurface.FactoryInvocationResult {
	return apisurface.FactoryInvocationResult{
		Status: status, ErrorCode: code, Message: message,
		PrimaryResult: []work.WorkContentPart{{
			Type: work.WorkContentPartTypeText, Text: "mock worker accepted",
		}},
		SessionID: defaultFactorySessionID,
		WorkID:    "dashboard-render-test-work",
		WorkName:  "task",
		TraceID:   "dashboard-render-test-trace",
	}
}

func assertCleanInvocationCLIError(t *testing.T, err error, wantCode, wantMessage string) {
	t.Helper()
	if err == nil {
		t.Fatalf("error = nil, want code %q", wantCode)
	}
	var cliErr interface {
		error
		InvocationErrorCode() string
		InvocationErrorMessage() string
	}
	if !errors.As(err, &cliErr) {
		t.Fatalf("error = %#v, want Invocation CLI error", err)
	}
	if cliErr.InvocationErrorCode() != wantCode {
		t.Fatalf("code = %q, want %q", cliErr.InvocationErrorCode(), wantCode)
	}
	if !strings.Contains(cliErr.InvocationErrorMessage(), wantMessage) {
		t.Fatalf("message = %q, want substring %q", cliErr.InvocationErrorMessage(), wantMessage)
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
	originalInvocation := openTestInvocationRunner
	if cfg.CleanInvocation && openTestInvocationRunner == nil {
		openTestInvocationRunner = func(
			context.Context,
			*testRuntimeSelections,
			serviceedges.Edges,
		) (InvocationRunner, error) {
			return stubInvocationService{
				run: func(context.Context) error { return nil },
				invoke: func(context.Context, string, factoryapi.InvocationRequest) (apisurface.FactoryInvocationResult, error) {
					return dashboardCleanInvocationResult(), nil
				},
			}, nil
		}
	}
	defer func() { openTestInvocationRunner = originalInvocation }()

	cfg.Output = &output
	builder := adaptTestRuntimeRunnerOpener(buildTransportTestRuntime)
	if len(builders) > 0 {
		builder = builders[0]
	}
	runErr := runWithTestRuntimeRunner(context.Background(), cfg, builder)

	return output.String(), runErr
}
