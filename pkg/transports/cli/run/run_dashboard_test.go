package run

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/portpowered/infinite-you/pkg/factory/state"
	"github.com/portpowered/infinite-you/pkg/interfaces"
	"github.com/portpowered/infinite-you/pkg/orchestrators/petri"
	"github.com/portpowered/infinite-you/pkg/service"
	initcmd "github.com/portpowered/infinite-you/pkg/transports/cli/init"
	"go.uber.org/zap"
	"go.uber.org/zap/zaptest/observer"
)

func TestRun_DefaultDashboardRendering_PrintsSimpleDashboardOutput(t *testing.T) {
	dir, workFile := writeDashboardRunFixture(t)
	executionBaseDir := t.TempDir()
	packageLocalSnapshot := durableSessionSnapshotPath(".")

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
	if _, err := os.Stat(durableSessionSnapshotPath(executionBaseDir)); err != nil {
		t.Fatalf("temporary durable session snapshot was not written: %v", err)
	}
	if _, err := os.Stat(packageLocalSnapshot); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("package-local durable session snapshot must not be written, got err=%v", err)
	}
}

func durableSessionSnapshotPath(root string) string {
	return filepath.Join(root, ".you-agent-factory", "durable-sessions", defaultFactorySessionID+".json")
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
	originalDefaultRecordPath := defaultLiveRunRecordPath
	defer func() {
		defaultLiveRunRecordPath = originalDefaultRecordPath
	}()

	dir, workFile := writeDashboardRunFixture(t)
	recordPath := filepath.Join(t.TempDir(), "factory-session-__factory_session_id__-clean.json")
	defaultLiveRunRecordPath = func() (string, error) {
		return recordPath, nil
	}

	var startupOut bytes.Buffer
	output, err := runWithCapturedStdout(t, RunConfig{
		Dir:                dir,
		ExecutionBaseDir:   t.TempDir(),
		Port:               0,
		WorkFile:           workFile,
		MockWorkersEnabled: true,
		CleanInvocation:    true,
		StartupOutput:      &startupOut,
		Logger:             zap.NewNop(),
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
	if _, err := os.Stat(resolveDefaultSessionRecordPath(recordPath)); err != nil {
		t.Fatalf("default recording was not written: %v", err)
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
	originalBuilder := buildFactoryService
	defer func() {
		buildFactoryService = originalBuilder
	}()

	_, workFile := writeDashboardRunFixture(t)
	buildFactoryService = func(_ context.Context, _ *service.FactoryServiceConfig) (factoryServiceRunner, error) {
		return stubFactoryService{
			run: func(context.Context) error { return nil },
			snapshot: func(context.Context) (*interfaces.EngineStateSnapshot[petri.MarkingSnapshot, *state.Net], error) {
				return failedCleanInvocationSnapshot("mock worker rejected"), nil
			},
		}, nil
	}

	output, err := runWithCapturedStdout(t, RunConfig{
		WorkFile:                workFile,
		CleanInvocation:         true,
		DisableDefaultRecording: true,
		Logger:                  zap.NewNop(),
	}, buildFactoryService)
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
	originalBuilder := buildFactoryService
	defer func() {
		buildFactoryService = originalBuilder
	}()

	_, workFile := writeDashboardRunFixture(t)
	buildFactoryService = func(_ context.Context, _ *service.FactoryServiceConfig) (factoryServiceRunner, error) {
		return stubFactoryService{
			run: func(context.Context) error { return nil },
			snapshot: func(context.Context) (*interfaces.EngineStateSnapshot[petri.MarkingSnapshot, *state.Net], error) {
				return timedOutCleanInvocationSnapshot(), nil
			},
		}, nil
	}

	output, err := runWithCapturedStdout(t, RunConfig{
		WorkFile:                workFile,
		CleanInvocation:         true,
		DisableDefaultRecording: true,
		Logger:                  zap.NewNop(),
	}, buildFactoryService)
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
	originalBuilder := buildFactoryService
	defer func() {
		buildFactoryService = originalBuilder
	}()

	_, workFile := writeDashboardRunFixture(t)
	buildFactoryService = func(_ context.Context, _ *service.FactoryServiceConfig) (factoryServiceRunner, error) {
		return stubFactoryService{
			run: func(context.Context) error { return context.Canceled },
			snapshot: func(context.Context) (*interfaces.EngineStateSnapshot[petri.MarkingSnapshot, *state.Net], error) {
				return nil, errors.New("snapshot not needed")
			},
		}, nil
	}

	output, err := runWithCapturedStdout(t, RunConfig{
		WorkFile:                workFile,
		CleanInvocation:         true,
		DisableDefaultRecording: true,
		Logger:                  zap.NewNop(),
	}, buildFactoryService)
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

	originalBuilder := buildFactoryService
	defer func() {
		buildFactoryService = originalBuilder
	}()

	_, workFile := writeDashboardRunFixture(t)
	core, observed := observer.New(zap.InfoLevel)
	buildFactoryService = func(_ context.Context, _ *service.FactoryServiceConfig) (factoryServiceRunner, error) {
		return stubFactoryService{
			run: func(context.Context) error { return context.Canceled },
			snapshot: func(context.Context) (*interfaces.EngineStateSnapshot[petri.MarkingSnapshot, *state.Net], error) {
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
	originalBuilder := buildFactoryService
	defer func() {
		buildFactoryService = originalBuilder
	}()

	_, workFile := writeDashboardRunFixture(t)
	started := make(chan struct{})
	release := make(chan struct{})
	buildFactoryService = func(_ context.Context, _ *service.FactoryServiceConfig) (factoryServiceRunner, error) {
		return stubFactoryService{
			run: func(context.Context) error {
				close(started)
				<-release
				return context.Canceled
			},
			snapshot: func(context.Context) (*interfaces.EngineStateSnapshot[petri.MarkingSnapshot, *state.Net], error) {
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
	originalBuilder := buildFactoryService
	defer func() {
		buildFactoryService = originalBuilder
	}()

	started := make(chan struct{})
	var capturedMode interfaces.RuntimeMode
	buildFactoryService = func(_ context.Context, cfg *service.FactoryServiceConfig) (factoryServiceRunner, error) {
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

func TestRun_OOTBIntegrationSmokeBootstrapsProcessesDefaultTaskAndReportsDashboard(t *testing.T) {
	preserveRunGlobals(t)

	dir := filepath.Join(t.TempDir(), "factory")
	taskPath := filepath.Join(dir, "inputs", initcmd.DefaultFactoryInputType, "default", "ootb-smoke.md")
	installOOTBSmokeBootstrap(taskPath)
	disableInteractiveDashboardForSmoke(t)

	capturedCh := make(chan capturedOOTBSmokeRun, 1)
	captureOOTBSmokeServiceBuilds(capturedCh)

	port := unusedTCPPort(t)
	var out bytes.Buffer
	cancel, errCh := startOOTBSmokeRun(t, dir, port, &out)
	captured := waitForOOTBSmokeServiceStartup(t, capturedCh, errCh)

	assertOOTBSmokeStartupConfig(t, captured.cfg, dir, taskPath)
	snapshot := waitForOOTBSmokeTaskCompletion(t, captured.svc, errCh)
	assertOOTBSmokeTaskResult(t, snapshot)
	assertContinuousRunStillActive(t, errCh)
	assertOOTBSmokeStartupOutput(t, out.String(), dir, port)
	stopOOTBSmokeRun(t, cancel, errCh)
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

func installOOTBSmokeBootstrap(taskPath string) {
	originalBootstrap := bootstrapFactory
	bootstrapFactory = func(inDir string) error {
		if err := originalBootstrap(inDir); err != nil {
			return err
		}
		return os.WriteFile(taskPath, []byte("# OOTB smoke\n\nConfirm the default task path is processed."), 0o644)
	}
}

func disableInteractiveDashboardForSmoke(t *testing.T) {
	t.Helper()

	dashboardOpener = func(_ context.Context, _ string) error {
		t.Fatal("dashboard opener should not run for non-interactive smoke output")
		return nil
	}
	interactiveOutput = func(io.Writer) bool {
		return false
	}
}

func captureOOTBSmokeServiceBuilds(capturedCh chan<- capturedOOTBSmokeRun) {
	buildFactoryService = func(ctx context.Context, cfg *service.FactoryServiceConfig) (factoryServiceRunner, error) {
		cfgCopy := *cfg
		svc, err := service.BuildFactoryService(ctx, cfg)
		if err != nil {
			return nil, err
		}
		capturedCh <- capturedOOTBSmokeRun{cfg: &cfgCopy, svc: svc}
		return svc, nil
	}
}

func startOOTBSmokeRun(t *testing.T, dir string, port int, out *bytes.Buffer) (context.CancelFunc, chan error) {
	t.Helper()

	ctx, cancel := context.WithCancel(context.Background())
	errCh := make(chan error, 1)
	go func() {
		errCh <- Run(ctx, RunConfig{
			Dir:                dir,
			ExecutionBaseDir:   dir,
			Bootstrap:          true,
			Continuously:       true,
			MockWorkersEnabled: true,
			Port:               port,
			OpenDashboard:      true,
			StartupOutput:      out,
			Logger:             zap.NewNop(),
		})
	}()
	return cancel, errCh
}

func waitForOOTBSmokeServiceStartup(
	t *testing.T,
	capturedCh <-chan capturedOOTBSmokeRun,
	errCh <-chan error,
) capturedOOTBSmokeRun {
	t.Helper()

	select {
	case captured := <-capturedCh:
		return captured
	case err := <-errCh:
		t.Fatalf("Run returned before service startup: %v", err)
	case <-time.After(15 * time.Second):
		t.Fatal("timed out waiting for factory service startup")
	}
	return capturedOOTBSmokeRun{}
}

func assertOOTBSmokeStartupConfig(t *testing.T, cfg *service.FactoryServiceConfig, dir, taskPath string) {
	t.Helper()

	if cfg.RuntimeMode != interfaces.RuntimeModeService {
		t.Fatalf("runtime mode = %q, want %q", cfg.RuntimeMode, interfaces.RuntimeModeService)
	}
	if cfg.MockWorkersConfig == nil {
		t.Fatalf("mock-worker config was not passed through: %#v", cfg)
	}
	if _, err := os.Stat(filepath.Join(dir, "factory.json")); err != nil {
		t.Fatalf("expected bootstrap to create factory.json: %v", err)
	}
	if _, err := os.Stat(filepath.Dir(taskPath)); err != nil {
		t.Fatalf("expected bootstrap to create inputs/%s/default: %v", initcmd.DefaultFactoryInputType, err)
	}
	if _, err := os.Stat(taskPath); err != nil {
		t.Fatalf("expected bootstrap to seed canonical starter task path %q: %v", taskPath, err)
	}
}

func waitForOOTBSmokeTaskCompletion(
	t *testing.T,
	svc *service.FactoryService,
	errCh <-chan error,
) *interfaces.EngineStateSnapshot[petri.MarkingSnapshot, *state.Net] {
	t.Helper()

	deadline := time.Now().Add(15 * time.Second)
	for {
		snapshot, err := svc.GetEngineStateSnapshot(context.Background())
		if err != nil {
			t.Fatalf("GetEngineStateSnapshot: %v", err)
		}
		if len(snapshot.Marking.TokensInPlace(initcmd.DefaultFactoryInputType+":complete")) == 1 {
			return snapshot
		}
		select {
		case err := <-errCh:
			t.Fatalf("Run returned before completing default task: %v", err)
		default:
		}
		if time.Now().After(deadline) {
			t.Fatalf("timed out waiting for default task; places: %#v", snapshot.Marking.PlaceTokens)
		}
		time.Sleep(100 * time.Millisecond)
	}
}

func assertOOTBSmokeTaskResult(
	t *testing.T,
	snapshot *interfaces.EngineStateSnapshot[petri.MarkingSnapshot, *state.Net],
) {
	t.Helper()

	if got := len(snapshot.Marking.TokensInPlace(initcmd.DefaultFactoryInputType + ":complete")); got != 1 {
		t.Fatalf("%s:complete token count = %d, want 1; places: %#v", initcmd.DefaultFactoryInputType, got, snapshot.Marking.PlaceTokens)
	}
	if got := len(snapshot.Marking.TokensInPlace(initcmd.DefaultFactoryInputType + ":failed")); got != 0 {
		t.Fatalf("%s:failed token count = %d, want 0; places: %#v", initcmd.DefaultFactoryInputType, got, snapshot.Marking.PlaceTokens)
	}
}

func assertContinuousRunStillActive(t *testing.T, errCh <-chan error) {
	t.Helper()

	select {
	case err := <-errCh:
		t.Fatalf("continuous Run returned before cancellation: %v", err)
	default:
	}
}

func assertOOTBSmokeStartupOutput(t *testing.T, output, dir string, port int) {
	t.Helper()

	wantURL := fmt.Sprintf("http://localhost:%d/dashboard/ui", port)
	for _, want := range []string{
		"Factory initiated: " + dir,
		"Factory directory ready: " + dir,
		"Runtime mode: continuous",
		"Dashboard URL: " + wantURL,
		"Dashboard auto-open disabled; open " + wantURL,
	} {
		if !strings.Contains(output, want) {
			t.Fatalf("startup output = %q, want %q", output, want)
		}
	}
}

func stopOOTBSmokeRun(t *testing.T, cancel context.CancelFunc, errCh <-chan error) {
	t.Helper()

	cancel()
	select {
	case err := <-errCh:
		if err != nil {
			t.Fatalf("Run after cancellation: %v", err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("timed out waiting for continuous run to stop after cancellation")
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
	req := interfaces.WorkRequest{
		Type: interfaces.WorkRequestTypeFactoryRequestBatch,
		Works: []interfaces.Work{{
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
	builders ...FactoryServiceBuilder,
) (string, error) {
	t.Helper()
	if cfg.ExecutionBaseDir == "" && cfg.Dir != "" {
		cfg.ExecutionBaseDir = cfg.Dir
	}

	oldStdout := os.Stdout
	readPipe, writePipe, err := os.Pipe()
	if err != nil {
		t.Fatalf("pipe stdout: %v", err)
	}

	readCh := make(chan []byte, 1)
	readErrCh := make(chan error, 1)
	go func() {
		data, readErr := io.ReadAll(readPipe)
		readCh <- data
		readErrCh <- readErr
	}()

	os.Stdout = writePipe
	builder := FactoryServiceBuilderFromService(service.BuildFactoryService)
	if len(builders) > 0 {
		builder = builders[0]
	}
	runErr := runWithFactoryServiceBuilder(context.Background(), cfg, builder)
	os.Stdout = oldStdout

	if err := writePipe.Close(); err != nil {
		t.Fatalf("close captured stdout writer: %v", err)
	}
	output := <-readCh
	if err := <-readErrCh; err != nil {
		t.Fatalf("read captured stdout: %v", err)
	}
	if err := readPipe.Close(); err != nil {
		t.Fatalf("close captured stdout reader: %v", err)
	}

	return string(output), runErr
}

func failedCleanInvocationSnapshot(reason string) *interfaces.EngineStateSnapshot[petri.MarkingSnapshot, *state.Net] {
	return &interfaces.EngineStateSnapshot[petri.MarkingSnapshot, *state.Net]{
		DispatchHistory: []interfaces.CompletedDispatch{{
			Outcome: interfaces.OutcomeFailed,
			Reason:  reason,
			ConsumedTokens: []interfaces.Token{{
				ID:      "failed-token",
				PlaceID: "task:init",
				Color: interfaces.TokenColor{
					WorkID:     "dashboard-render-test-work",
					WorkTypeID: "task",
					TraceID:    "dashboard-render-test-trace",
				},
			}},
		}},
	}
}

func timedOutCleanInvocationSnapshot() *interfaces.EngineStateSnapshot[petri.MarkingSnapshot, *state.Net] {
	return &interfaces.EngineStateSnapshot[petri.MarkingSnapshot, *state.Net]{
		DispatchHistory: []interfaces.CompletedDispatch{{
			Outcome: interfaces.OutcomeFailed,
			ConsumedTokens: []interfaces.Token{{
				ID:      "timeout-token",
				PlaceID: "task:init",
				Color: interfaces.TokenColor{
					WorkID:     "dashboard-render-test-work",
					WorkTypeID: "task",
					TraceID:    "dashboard-render-test-trace",
				},
			}},
			FailureMetadata: &interfaces.WorkFailureMetadata{
				Family: interfaces.WorkFailureFamilyRetryable,
				Type:   interfaces.WorkFailureTypeTimeout,
			},
		}},
	}
}
