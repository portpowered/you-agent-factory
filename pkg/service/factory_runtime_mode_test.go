package service

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	factoryapi "github.com/portpowered/infinite-you/pkg/api/generated"
	"github.com/portpowered/infinite-you/pkg/apisurface"
	"github.com/portpowered/infinite-you/pkg/config"
	"github.com/portpowered/infinite-you/pkg/factory/state"
	"github.com/portpowered/infinite-you/pkg/interfaces"
	"github.com/portpowered/infinite-you/pkg/petri"
	"go.uber.org/zap"
)

func TestBuildFactoryService_RecordAndReplayTogetherRejected(t *testing.T) {
	_, err := BuildFactoryService(context.Background(), &FactoryServiceConfig{
		Dir:        t.TempDir(),
		RecordPath: "recording.json",
		ReplayPath: "recording.json",
		Logger:     zap.NewNop(),
	})
	if err == nil {
		t.Fatal("expected record and replay combination to fail")
	}
	if !strings.Contains(err.Error(), "--record and --replay cannot be used together") {
		t.Fatalf("unexpected error: %v", err)
	}
}

// portos:func-length-exception owner=agent-factory reason=legacy-service-mode-fixture review=2026-07-18 removal=split-late-submit-fixture-before-next-service-mode-change
// pkgmaintcheck:ignore-cyclomatic-complexity this service-mode runtime test keeps idle-startup and late-submission assertions together on the public seam.
func TestBuildFactoryService_ServiceModeAcceptsLateSubmissionAfterIdleStartup(t *testing.T) {
	dir := t.TempDir()
	writeFactoryJSON(t, dir, minimalFactoryConfig())
	writeWorkstationAgentsMD(t, dir, "process")
	if err := os.MkdirAll(filepath.Join(dir, interfaces.InputsDir), 0o755); err != nil {
		t.Fatalf("create inputs dir: %v", err)
	}

	svc, err := BuildFactoryService(context.Background(), &FactoryServiceConfig{
		Dir:               dir,
		RuntimeMode:       interfaces.RuntimeModeService,
		MockWorkersConfig: config.NewEmptyMockWorkersConfig(),
		Logger:            zap.NewNop(),
	})
	if err != nil {
		t.Fatalf("BuildFactoryService: %v", err)
	}

	runCtx, cancel := context.WithCancel(context.Background())
	errCh := make(chan error, 1)
	go func() {
		errCh <- svc.Run(runCtx)
	}()

	select {
	case err := <-errCh:
		t.Fatalf("Run returned before late submission: %v", err)
	case <-time.After(150 * time.Millisecond):
	}

	snapBeforeSubmit, err := svc.GetEngineStateSnapshot(context.Background())
	if err != nil {
		t.Fatalf("GetEngineStateSnapshot before submit: %v", err)
	}

	time.Sleep(100 * time.Millisecond)

	snapAfterIdleWait, err := svc.GetEngineStateSnapshot(context.Background())
	if err != nil {
		t.Fatalf("GetEngineStateSnapshot after idle wait: %v", err)
	}
	if snapAfterIdleWait.RuntimeStatus != interfaces.RuntimeStatusIdle {
		t.Fatalf("service-mode idle status = %q, want %q", snapAfterIdleWait.RuntimeStatus, interfaces.RuntimeStatusIdle)
	}

	if snapAfterIdleWait.TickCount != snapBeforeSubmit.TickCount {
		t.Fatalf("service-mode idle wait should not busy-spin: tick count advanced from %d to %d",
			snapBeforeSubmit.TickCount,
			snapAfterIdleWait.TickCount,
		)
	}

	err = submitWorkRequestsToService(context.Background(), svc, []interfaces.SubmitRequest{{
		WorkTypeID: "task",
		TraceID:    "trace-late-submit",
		Payload:    json.RawMessage(`{"title":"late submit"}`),
	}})
	if err != nil {
		t.Fatalf("Submit late work: %v", err)
	}

	deadline := time.Now().Add(time.Second)
	for time.Now().Before(deadline) {
		state, err := svc.GetEngineStateSnapshot(context.Background())
		if err != nil {
			t.Fatalf("GetEngineStateSnapshot: %v", err)
		}
		for _, token := range state.Marking.Tokens {
			if token.PlaceID == "task:complete" {
				cancel()
				select {
				case err := <-errCh:
					if err != nil {
						t.Fatalf("Run after cancellation: %v", err)
					}
				case <-time.After(time.Second):
					t.Fatal("timed out waiting for service-mode factory service to stop")
				}
				return
			}
		}
		time.Sleep(10 * time.Millisecond)
	}

	cancel()
	<-errCh
	t.Fatal("late-submitted service work did not reach task:complete before timeout")
}

func TestBuildFactoryService_BatchModeRejectsLateSubmissionAfterTermination(t *testing.T) {
	dir := t.TempDir()
	writeFactoryJSON(t, dir, minimalFactoryConfig())
	writeWorkstationAgentsMD(t, dir, "process")
	if err := os.MkdirAll(filepath.Join(dir, interfaces.InputsDir), 0o755); err != nil {
		t.Fatalf("create inputs dir: %v", err)
	}

	svc, err := BuildFactoryService(context.Background(), &FactoryServiceConfig{
		Dir:               dir,
		MockWorkersConfig: config.NewEmptyMockWorkersConfig(),
		Logger:            zap.NewNop(),
	})
	if err != nil {
		t.Fatalf("BuildFactoryService: %v", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()

	if err := svc.Run(ctx); err != nil {
		t.Fatalf("Run: %v", err)
	}

	snap, err := svc.GetEngineStateSnapshot(context.Background())
	if err != nil {
		t.Fatalf("GetEngineStateSnapshot after batch completion: %v", err)
	}
	if snap.RuntimeStatus != interfaces.RuntimeStatusFinished {
		t.Fatalf("batch completion status = %q, want %q", snap.RuntimeStatus, interfaces.RuntimeStatusFinished)
	}

	err = submitWorkRequestsToService(context.Background(), svc, []interfaces.SubmitRequest{{
		WorkTypeID: "task",
		TraceID:    "trace-after-stop",
	}})
	if err == nil {
		t.Fatal("expected late batch submission to fail after runtime termination")
	}
	if !strings.Contains(err.Error(), "terminated") {
		t.Fatalf("expected terminated error, got %v", err)
	}
}

func TestFactoryService_ServiceModeAPISurfaceStartsBeforeStartupWorkFileSubmission(t *testing.T) {
	dir := t.TempDir()
	writeFactoryJSON(t, dir, minimalFactoryConfig())
	writeWorkstationAgentsMD(t, dir, "process")
	if err := os.MkdirAll(filepath.Join(dir, interfaces.InputsDir), 0o755); err != nil {
		t.Fatalf("create inputs dir: %v", err)
	}

	workFile := filepath.Join(dir, "startup-work.json")
	if err := os.WriteFile(workFile, []byte(`{
  "requestId": "request-startup-before-workfile",
  "type": "FACTORY_REQUEST_BATCH",
  "works": [
    {
      "name": "startup-item",
      "workId": "work-startup-before-workfile",
      "workTypeName": "task",
      "traceId": "trace-startup-before-workfile",
      "payload": {"title": "startup work"}
    }
  ]
}`), 0o644); err != nil {
		t.Fatalf("write work file: %v", err)
	}

	type starterObservation struct {
		snapshot *interfaces.EngineStateSnapshot[petri.MarkingSnapshot, *state.Net]
		err      error
	}

	observedCh := make(chan starterObservation, 1)
	apiReady := make(chan struct{})
	svc, err := BuildFactoryService(context.Background(), &FactoryServiceConfig{
		Dir:               dir,
		RuntimeMode:       interfaces.RuntimeModeService,
		MockWorkersConfig: config.NewEmptyMockWorkersConfig(),
		WorkFile:          workFile,
		Port:              1,
		Logger:            zap.NewNop(),
		APIServerReady:    apiReady,
		APIServerStarter: func(ctx context.Context, runtime apisurface.APISurface, _ int, _ *zap.Logger) error {
			snapshot, err := runtime.GetEngineStateSnapshot(ctx)
			if err != nil {
				observedCh <- starterObservation{err: err}
			} else {
				observedCh <- starterObservation{snapshot: snapshot}
			}
			close(apiReady)
			<-ctx.Done()
			return nil
		},
	})
	if err != nil {
		t.Fatalf("BuildFactoryService: %v", err)
	}

	runCtx, cancel := context.WithCancel(context.Background())
	errCh := make(chan error, 1)
	go func() {
		errCh <- svc.Run(runCtx)
	}()

	var observation starterObservation
	select {
	case observation = <-observedCh:
	case err := <-errCh:
		t.Fatalf("Run returned before API starter observation: %v", err)
	case <-time.After(time.Second):
		t.Fatal("timed out waiting for API starter observation")
	}
	if observation.err != nil {
		t.Fatalf("APIServerStarter GetEngineStateSnapshot: %v", observation.err)
	}
	if observation.snapshot == nil {
		t.Fatal("APIServerStarter snapshot = nil, want idle runtime snapshot")
	}
	if observation.snapshot.RuntimeStatus != interfaces.RuntimeStatusIdle {
		t.Fatalf("startup runtime status = %q, want %q", observation.snapshot.RuntimeStatus, interfaces.RuntimeStatusIdle)
	}
	if len(observation.snapshot.Marking.Tokens) != 0 {
		t.Fatalf("startup tokens = %#v, want no startup-work tokens before work-file submission", observation.snapshot.Marking.Tokens)
	}

	waitForTokenInPlaceByWorkID(t, svc, "task:complete", "work-startup-before-workfile", time.Second)
	cancel()
	select {
	case err := <-errCh:
		if err != nil {
			t.Fatalf("Run after cancellation: %v", err)
		}
	case <-time.After(time.Second):
		t.Fatal("timed out waiting for service-mode factory service to stop")
	}
}

func TestFactoryService_ServiceModeStartupWorkReadabilityFailsWhenAPIServerStartFails(t *testing.T) {
	dir := t.TempDir()
	writeFactoryJSON(t, dir, minimalFactoryConfig())
	writeWorkstationAgentsMD(t, dir, "process")
	if err := os.MkdirAll(filepath.Join(dir, interfaces.InputsDir), 0o755); err != nil {
		t.Fatalf("create inputs dir: %v", err)
	}

	workFile := filepath.Join(dir, "startup-work.json")
	if err := os.WriteFile(workFile, []byte(`{
  "requestId": "request-startup-api-failure",
  "type": "FACTORY_REQUEST_BATCH",
  "works": [
    {
      "name": "startup-item",
      "workId": "work-startup-api-failure",
      "workTypeName": "task",
      "traceId": "trace-startup-api-failure",
      "payload": {"title": "startup work"}
    }
  ]
}`), 0o644); err != nil {
		t.Fatalf("write work file: %v", err)
	}

	apiStartErr := errors.New("listen tcp 127.0.0.1:7777: bind: address already in use")
	svc, err := BuildFactoryService(context.Background(), &FactoryServiceConfig{
		Dir:               dir,
		RuntimeMode:       interfaces.RuntimeModeService,
		MockWorkersConfig: config.NewEmptyMockWorkersConfig(),
		WorkFile:          workFile,
		Port:              7777,
		Logger:            zap.NewNop(),
		APIServerReady:    make(chan struct{}),
		APIServerStarter: func(context.Context, apisurface.APISurface, int, *zap.Logger) error {
			return apiStartErr
		},
	})
	if err != nil {
		t.Fatalf("BuildFactoryService: %v", err)
	}

	runErrCh := make(chan error, 1)
	go func() {
		runErrCh <- svc.Run(context.Background())
	}()

	select {
	case runErr := <-runErrCh:
		if runErr == nil {
			t.Fatal("Run error = nil, want API startup failure")
		}
		if !strings.Contains(runErr.Error(), "wait for service-mode startup work readiness") {
			t.Fatalf("Run error = %q, want startup readability context", runErr.Error())
		}
		if !strings.Contains(runErr.Error(), apiStartErr.Error()) {
			t.Fatalf("Run error = %q, want API startup failure detail %q", runErr.Error(), apiStartErr.Error())
		}
	case <-time.After(time.Second):
		t.Fatal("timed out waiting for service-mode API startup failure to return")
	}
}

func TestFactoryService_ServiceModeStartupWorkSkipsAPIReadinessWaitWhenPortDisabled(t *testing.T) {
	dir := t.TempDir()
	writeFactoryJSON(t, dir, minimalFactoryConfig())
	writeWorkstationAgentsMD(t, dir, "process")
	if err := os.MkdirAll(filepath.Join(dir, interfaces.InputsDir), 0o755); err != nil {
		t.Fatalf("create inputs dir: %v", err)
	}

	workFile := filepath.Join(dir, "startup-work.json")
	if err := os.WriteFile(workFile, []byte(`{
  "requestId": "request-service-port-disabled",
  "type": "FACTORY_REQUEST_BATCH",
  "works": [
    {
      "name": "startup-item",
      "workId": "work-service-port-disabled",
      "workTypeName": "task",
      "traceId": "trace-service-port-disabled",
      "payload": {"title": "startup work"}
    }
  ]
}`), 0o644); err != nil {
		t.Fatalf("write work file: %v", err)
	}

	svc, err := BuildFactoryService(context.Background(), &FactoryServiceConfig{
		Dir:               dir,
		RuntimeMode:       interfaces.RuntimeModeService,
		MockWorkersConfig: config.NewEmptyMockWorkersConfig(),
		WorkFile:          workFile,
		Logger:            zap.NewNop(),
		Port:              0,
		APIServerReady:    make(chan struct{}),
		APIServerStarter: func(context.Context, apisurface.APISurface, int, *zap.Logger) error {
			return errors.New("API server should not start when port is disabled")
		},
	})
	if err != nil {
		t.Fatalf("BuildFactoryService: %v", err)
	}

	runCtx, cancel := context.WithCancel(context.Background())
	runErrCh := make(chan error, 1)
	go func() {
		runErrCh <- svc.Run(runCtx)
	}()

	waitForTokenInPlaceByWorkID(t, svc, "task:complete", "work-service-port-disabled", time.Second)
	cancel()
	select {
	case runErr := <-runErrCh:
		if runErr != nil {
			t.Fatalf("Run after cancellation: %v", runErr)
		}
	case <-time.After(time.Second):
		t.Fatal("timed out waiting for service-mode run with disabled API port to stop")
	}
}

// pkgmaintcheck:ignore-cyclomatic-complexity this runtime observability test keeps snapshot and event-stream assertions together in one service flow.
func TestFactoryService_RunPreservesSnapshotAndFactoryEventObservability(t *testing.T) {
	dir := t.TempDir()
	writeFactoryJSON(t, dir, minimalFactoryConfig())
	writeWorkstationAgentsMD(t, dir, "process")
	inputDir := filepath.Join(dir, interfaces.InputsDir, "task", interfaces.DefaultChannelName)
	if err := os.MkdirAll(inputDir, 0o755); err != nil {
		t.Fatalf("create inputs dir: %v", err)
	}
	if err := os.WriteFile(filepath.Join(inputDir, "seed.json"), []byte(`{"title":"observe runtime"}`), 0o644); err != nil {
		t.Fatalf("write seed work file: %v", err)
	}

	svc, err := BuildFactoryService(context.Background(), &FactoryServiceConfig{
		Dir:               dir,
		MockWorkersConfig: config.NewEmptyMockWorkersConfig(),
		Logger:            zap.NewNop(),
	})
	if err != nil {
		t.Fatalf("BuildFactoryService: %v", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if err := svc.Run(ctx); err != nil {
		t.Fatalf("Run: %v", err)
	}

	snap, err := svc.GetEngineStateSnapshot(context.Background())
	if err != nil {
		t.Fatalf("GetEngineStateSnapshot: %v", err)
	}
	if snap.FactoryState != string(interfaces.FactoryStateCompleted) {
		t.Fatalf("factory state = %q, want %q", snap.FactoryState, interfaces.FactoryStateCompleted)
	}
	if snap.RuntimeStatus != interfaces.RuntimeStatusFinished {
		t.Fatalf("runtime status = %q, want %q", snap.RuntimeStatus, interfaces.RuntimeStatusFinished)
	}
	if snap.Topology == nil || snap.Topology.WorkTypes["task"] == nil {
		t.Fatalf("snapshot topology work types = %#v, want task work type", snap.Topology)
	}
	if snap.Marking.Tokens == nil || len(snap.Marking.Tokens) != 1 {
		t.Fatalf("snapshot marking tokens = %#v, want one completed token", snap.Marking.Tokens)
	}
	for _, token := range snap.Marking.Tokens {
		if token.PlaceID != "task:complete" {
			t.Fatalf("snapshot token place = %q, want task:complete", token.PlaceID)
		}
	}
	if snap.TickCount == 0 {
		t.Fatal("snapshot tick count = 0, want runtime activity")
	}
	if len(snap.DispatchHistory) == 0 {
		t.Fatal("snapshot dispatch history is empty, want completed runtime activity")
	}
	if len(snap.DispatchHistory[0].ConsumedTokens) == 0 {
		t.Fatalf("completed dispatch = %#v, want consumed token evidence", snap.DispatchHistory[0])
	}

	events, err := svc.GetFactoryEvents(context.Background())
	if err != nil {
		t.Fatalf("GetFactoryEvents: %v", err)
	}
	assertServiceFactoryEventsContainTypes(t, events, []factoryapi.FactoryEventType{
		factoryapi.FactoryEventTypeWorkRequest,
		factoryapi.FactoryEventTypeDispatchRequest,
		factoryapi.FactoryEventTypeDispatchResponse,
	})
}

func TestBuildFactoryService_InvalidWorkFile(t *testing.T) {
	dir := t.TempDir()
	writeFactoryJSON(t, dir, minimalFactoryConfig())
	writeWorkstationAgentsMD(t, dir, "process")
	if err := os.MkdirAll(filepath.Join(dir, interfaces.InputsDir), 0o755); err != nil {
		t.Fatalf("create inputs dir: %v", err)
	}

	ctx := context.Background()

	// Build service with a nonexistent work file.
	svc, err := BuildFactoryService(ctx, &FactoryServiceConfig{
		Dir:               dir,
		MockWorkersConfig: config.NewEmptyMockWorkersConfig(),
		Logger:            zap.NewNop(),
		WorkFile:          filepath.Join(dir, "nonexistent.json"),
	})
	if err != nil {
		t.Fatalf("BuildFactoryService: %v", err)
	}

	// submitWorkFile should fail for nonexistent file.
	err = svc.submitWorkFile(ctx)
	if err == nil {
		t.Fatal("expected error for nonexistent work file")
	}
}

func TestBuildFactoryService_WorkFileRejectsRetiredTargetStateAlias(t *testing.T) {
	dir := t.TempDir()
	writeFactoryJSON(t, dir, minimalFactoryConfig())
	writeWorkstationAgentsMD(t, dir, "process")
	if err := os.MkdirAll(filepath.Join(dir, interfaces.InputsDir), 0o755); err != nil {
		t.Fatalf("create inputs dir: %v", err)
	}

	workFile := filepath.Join(dir, "initial-work.json")
	if err := os.WriteFile(workFile, []byte(`{
  "request_id": "request-service-target-state",
  "type": "FACTORY_REQUEST_BATCH",
  "works": [
    {"name": "draft", "work_type_name": "task", "target_state": "waiting"}
  ]
}`), 0o644); err != nil {
		t.Fatalf("write work file: %v", err)
	}

	svc, err := BuildFactoryService(context.Background(), &FactoryServiceConfig{
		Dir:               dir,
		MockWorkersConfig: config.NewEmptyMockWorkersConfig(),
		Logger:            zap.NewNop(),
		WorkFile:          workFile,
	})
	if err != nil {
		t.Fatalf("BuildFactoryService: %v", err)
	}

	err = svc.submitWorkFile(context.Background())
	if err == nil {
		t.Fatal("expected retired target_state alias to fail")
	}
	if !strings.Contains(err.Error(), "target_state") || !strings.Contains(err.Error(), "state") {
		t.Fatalf("error = %q, want target_state rejection with state guidance", err.Error())
	}
}

func TestBuildFactoryService_WorkFileRejectsConflictingTraceAliases(t *testing.T) {
	dir := t.TempDir()
	writeFactoryJSON(t, dir, minimalFactoryConfig())
	writeWorkstationAgentsMD(t, dir, "process")
	if err := os.MkdirAll(filepath.Join(dir, interfaces.InputsDir), 0o755); err != nil {
		t.Fatalf("create inputs dir: %v", err)
	}

	workFile := filepath.Join(dir, "initial-work.json")
	if err := os.WriteFile(workFile, []byte(`{
  "requestId": "request-service-trace-conflict",
  "type": "FACTORY_REQUEST_BATCH",
  "works": [
    {"name": "draft", "workTypeName": "task", "currentChainingTraceId": "chain-a", "traceId": "trace-b"}
  ]
}`), 0o644); err != nil {
		t.Fatalf("write work file: %v", err)
	}

	svc, err := BuildFactoryService(context.Background(), &FactoryServiceConfig{
		Dir:               dir,
		MockWorkersConfig: config.NewEmptyMockWorkersConfig(),
		Logger:            zap.NewNop(),
		WorkFile:          workFile,
	})
	if err != nil {
		t.Fatalf("BuildFactoryService: %v", err)
	}

	err = svc.submitWorkFile(context.Background())
	if err == nil {
		t.Fatal("expected conflicting trace aliases to fail")
	}
	if !strings.Contains(err.Error(), "currentChainingTraceId and traceId must match") {
		t.Fatalf("error = %q, want conflicting trace alias rejection", err.Error())
	}
}

func TestBuildReplacementFactoryRuntime_ServiceModeStaysRunningUntilCanceled(t *testing.T) {
	rootDir := t.TempDir()
	runtimeLogDir := filepath.Join(t.TempDir(), "runtime-logs")
	alphaDir := writeNamedFactoryFixture(t, rootDir, "alpha")
	betaDir := writeNamedFactoryFixture(t, rootDir, "beta")
	if err := config.WriteCurrentFactoryPointer(rootDir, "alpha"); err != nil {
		t.Fatalf("WriteCurrentFactoryPointer(alpha): %v", err)
	}

	svc, err := BuildFactoryService(context.Background(), &FactoryServiceConfig{
		Dir:               rootDir,
		RuntimeMode:       interfaces.RuntimeModeService,
		MockWorkersConfig: config.NewEmptyMockWorkersConfig(),
		Logger:            zap.NewNop(),
		RuntimeLogDir:     runtimeLogDir,
	})
	if err != nil {
		t.Fatalf("BuildFactoryService: %v", err)
	}
	if svc.cfg.RuntimeMode != interfaces.RuntimeModeService {
		t.Fatalf("service runtime mode = %q, want %q", svc.cfg.RuntimeMode, interfaces.RuntimeModeService)
	}
	if svc.cfg.Dir != alphaDir {
		t.Fatalf("service dir = %q, want %q", svc.cfg.Dir, alphaDir)
	}

	createReplacementWatchChannel(t, betaDir, "task", "activated")
	replacement, err := svc.buildReplacementFactoryRuntime(context.Background(), rootDir, betaDir, defaultFactorySessionID)
	if err != nil {
		t.Fatalf("buildReplacementFactoryRuntime: %v", err)
	}
	if replacement.dir != betaDir {
		t.Fatalf("replacement dir = %q, want %q", replacement.dir, betaDir)
	}

	runCtx, cancel := context.WithCancel(context.Background())
	errCh := make(chan error, 1)
	go func() {
		errCh <- replacement.factory.Run(runCtx)
	}()

	select {
	case err := <-errCh:
		t.Fatalf("replacement runtime returned before cancellation: %v", err)
	case <-time.After(150 * time.Millisecond):
	}

	cancel()
	select {
	case err := <-errCh:
		if err != nil {
			t.Fatalf("replacement runtime after cancellation: %v", err)
		}
	case <-time.After(time.Second):
		t.Fatal("timed out waiting for replacement runtime to stop")
	}
}

func TestBuildReplacementFactoryRuntime_WiresLocalModelDelegationSeam(t *testing.T) {
	rootDir := t.TempDir()
	writeNamedFactoryFixture(t, rootDir, "alpha")
	betaDir := writeNamedFactoryFixture(t, rootDir, "beta")
	if err := config.WriteCurrentFactoryPointer(rootDir, "alpha"); err != nil {
		t.Fatalf("WriteCurrentFactoryPointer(alpha): %v", err)
	}

	svc, err := BuildFactoryService(context.Background(), &FactoryServiceConfig{
		Dir:               rootDir,
		RuntimeMode:       interfaces.RuntimeModeService,
		MockWorkersConfig: config.NewEmptyMockWorkersConfig(),
		Logger:            zap.NewNop(),
		LocalModelRuntimeOverride: &fakeLocalModelRuntime{
			response: interfaces.InferenceResponse{Content: "ok"},
		},
	})
	if err != nil {
		t.Fatalf("BuildFactoryService: %v", err)
	}

	replacement, err := svc.buildReplacementFactoryRuntime(context.Background(), rootDir, betaDir, defaultFactorySessionID)
	if err != nil {
		t.Fatalf("buildReplacementFactoryRuntime: %v", err)
	}
	if replacement.localModels == nil {
		t.Fatal("runtime bundle localModels = nil, want managed localmodels.Manager from buildRuntimeBundle seam")
	}
	if replacement.modelAssets == nil {
		t.Fatal("runtime bundle modelAssets = nil, want localmodels.AssetPuller from buildRuntimeBundle seam")
	}
	if replacement.modelResources == nil {
		t.Fatal("runtime bundle modelResources = nil, want localmodels.ResourceLimiter from buildRuntimeBundle seam")
	}
	if replacement.logSink == nil {
		t.Fatal("runtime bundle logSink = nil, want runtime log sink from buildRuntimeBundle seam")
	}
	if replacement.logger == nil {
		t.Fatal("runtime bundle logger = nil, want session logger from buildRuntimeBundle seam")
	}
}

func TestBuildFactoryService_StartupRuntimeBundleMatchesLiveHandleShape(t *testing.T) {
	rootDir := t.TempDir()
	writeNamedFactoryFixture(t, rootDir, "alpha")
	if err := config.WriteCurrentFactoryPointer(rootDir, "alpha"); err != nil {
		t.Fatalf("WriteCurrentFactoryPointer(alpha): %v", err)
	}

	svc, err := BuildFactoryService(context.Background(), &FactoryServiceConfig{
		Dir:               rootDir,
		RuntimeMode:       interfaces.RuntimeModeService,
		MockWorkersConfig: config.NewEmptyMockWorkersConfig(),
		Logger:            zap.NewNop(),
	})
	if err != nil {
		t.Fatalf("BuildFactoryService: %v", err)
	}
	bundle := svc.currentRuntimeBundle()
	if bundle == nil {
		t.Fatal("currentRuntimeBundle = nil, want startup bundle before Run")
	}
	if bundle.logSink == nil {
		t.Fatal("startup bundle logSink = nil, want runtime log sink")
	}
	if bundle.factory == nil {
		t.Fatal("startup bundle factory = nil")
	}
	if svc.startupRuntimeBundle() != bundle {
		t.Fatal("currentRuntimeBundle should resolve to startup bundle before Run registers ~default")
	}
}

func TestFactoryService_Run_ClearsStartupBundleAfterDefaultRegisters(t *testing.T) {
	rootDir := t.TempDir()
	writeNamedFactoryFixture(t, rootDir, "alpha")
	if err := config.WriteCurrentFactoryPointer(rootDir, "alpha"); err != nil {
		t.Fatalf("WriteCurrentFactoryPointer: %v", err)
	}

	svc, err := BuildFactoryService(context.Background(), &FactoryServiceConfig{
		Dir:               rootDir,
		RuntimeMode:       interfaces.RuntimeModeService,
		MockWorkersConfig: config.NewEmptyMockWorkersConfig(),
		Logger:            zap.NewNop(),
	})
	if err != nil {
		t.Fatalf("BuildFactoryService: %v", err)
	}
	if svc.startupRuntimeBundle() == nil {
		t.Fatal("expected startup bundle before Run")
	}

	runCtx, cancelRun := context.WithCancel(context.Background())
	runErrCh := make(chan error, 1)
	go func() { runErrCh <- svc.Run(runCtx) }()
	t.Cleanup(func() {
		cancelRun()
		<-runErrCh
	})

	waitForSessionRuntimeStatus(t, svc, defaultFactorySessionID, interfaces.RuntimeStatusIdle, time.Second, "default runtime")
	if svc.startupRuntimeBundle() != nil {
		t.Fatal("startup bundle should be cleared after ~default registers at Run")
	}
	defaultHandle := liveSessionHandle(svc.defaultSession())
	if bundle := svc.currentRuntimeBundle(); bundle == nil || defaultHandle == nil || bundle != defaultHandle.runtime {
		t.Fatal("currentRuntimeBundle should resolve only through the default session handle after Run")
	}
}

func TestBuildFactoryService_PreservesSessionsRegistryAcrossRuntimeReplacement(t *testing.T) {
	rootDir := t.TempDir()
	alphaDir := writeNamedFactoryFixture(t, rootDir, "alpha")
	betaDir := writeNamedFactoryFixture(t, rootDir, "beta")
	if err := config.WriteCurrentFactoryPointer(rootDir, "alpha"); err != nil {
		t.Fatalf("WriteCurrentFactoryPointer(alpha): %v", err)
	}

	svc, err := BuildFactoryService(context.Background(), &FactoryServiceConfig{
		Dir:               rootDir,
		RuntimeMode:       interfaces.RuntimeModeService,
		MockWorkersConfig: config.NewEmptyMockWorkersConfig(),
		Logger:            zap.NewNop(),
	})
	if err != nil {
		t.Fatalf("BuildFactoryService: %v", err)
	}
	if svc.sessions == nil {
		t.Fatal("expected factorysessions.Registry on FactoryService")
	}
	registryBefore := svc.sessions

	if _, err := svc.buildReplacementFactoryRuntime(context.Background(), rootDir, betaDir, defaultFactorySessionID); err != nil {
		t.Fatalf("buildReplacementFactoryRuntime: %v", err)
	}
	if svc.sessions != registryBefore {
		t.Fatal("buildReplacementFactoryRuntime replaced sessions registry; session ownership should stay on FactoryService")
	}
	if svc.cfg.Dir != alphaDir {
		t.Fatalf("service cfg.Dir = %q, want unchanged %q until activation", svc.cfg.Dir, alphaDir)
	}
}

func TestBuildFactoryService_InitializesFactorySessionsRegistry(t *testing.T) {
	rootDir := t.TempDir()
	alphaDir := writeNamedFactoryFixture(t, rootDir, "alpha")
	if err := config.WriteCurrentFactoryPointer(rootDir, "alpha"); err != nil {
		t.Fatalf("WriteCurrentFactoryPointer(alpha): %v", err)
	}

	svc, err := BuildFactoryService(context.Background(), &FactoryServiceConfig{
		Dir:               rootDir,
		RuntimeMode:       interfaces.RuntimeModeService,
		MockWorkersConfig: config.NewEmptyMockWorkersConfig(),
		Logger:            zap.NewNop(),
	})
	if err != nil {
		t.Fatalf("BuildFactoryService: %v", err)
	}
	if svc.sessions == nil {
		t.Fatal("expected factorysessions.Registry on FactoryService")
	}
	if svc.cfg.Dir != alphaDir {
		t.Fatalf("service cfg.Dir = %q, want %q", svc.cfg.Dir, alphaDir)
	}
	if svc.sessions.Count() != 0 {
		t.Fatalf("sessions.Count() = %d before Run, want 0 until live sessions register", svc.sessions.Count())
	}
}

func TestFactoryService_Run_RegistersDefaultSessionInRegistry(t *testing.T) {
	rootDir := t.TempDir()
	alphaDir := writeNamedFactoryFixture(t, rootDir, "alpha")
	if err := config.WriteCurrentFactoryPointer(rootDir, "alpha"); err != nil {
		t.Fatalf("WriteCurrentFactoryPointer(alpha): %v", err)
	}

	svc, err := BuildFactoryService(context.Background(), &FactoryServiceConfig{
		Dir:               rootDir,
		RuntimeMode:       interfaces.RuntimeModeService,
		MockWorkersConfig: config.NewEmptyMockWorkersConfig(),
		Logger:            zap.NewNop(),
	})
	if err != nil {
		t.Fatalf("BuildFactoryService: %v", err)
	}
	if svc.sessions.Count() != 0 {
		t.Fatalf("sessions.Count() = %d before Run, want 0 until default registers", svc.sessions.Count())
	}

	runCtx, cancelRun := context.WithCancel(context.Background())
	runErrCh := make(chan error, 1)
	go func() {
		runErrCh <- svc.Run(runCtx)
	}()
	t.Cleanup(func() {
		cancelRun()
		select {
		case err := <-runErrCh:
			if err != nil {
				t.Fatalf("Run: %v", err)
			}
		case <-time.After(time.Second):
			t.Fatal("timed out waiting for service shutdown")
		}
	})

	waitForSessionRuntimeStatus(t, svc, defaultFactorySessionID, interfaces.RuntimeStatusIdle, time.Second, "default runtime")

	defaultSession := svc.defaultSession()
	if defaultSession == nil {
		t.Fatal("defaultSession = nil after Run, want ~default registry entry")
	}
	if defaultSession.ID != defaultFactorySessionID {
		t.Fatalf("default session id = %q, want %q", defaultSession.ID, defaultFactorySessionID)
	}
	if !defaultSession.IsDefault {
		t.Fatal("default session IsDefault = false, want true")
	}
	if got := cleanResolvedPath(defaultSession.FactoryDir); got != cleanResolvedPath(alphaDir) {
		t.Fatalf("default session factoryDir = %q, want %q", defaultSession.FactoryDir, alphaDir)
	}
	if got := cleanResolvedPath(defaultSession.FolderPath); got != cleanResolvedPath(rootDir) {
		t.Fatalf("default session folderPath = %q, want %q", defaultSession.FolderPath, rootDir)
	}

	defaultHandle := liveSessionHandle(defaultSession)
	if defaultHandle == nil || defaultHandle.runtime == nil {
		t.Fatal("default session live handle is required after Run")
	}
	if got := cleanResolvedPath(defaultHandle.runtime.dir); got != cleanResolvedPath(alphaDir) {
		t.Fatalf("default live handle runtime dir = %q, want %q", defaultHandle.runtime.dir, alphaDir)
	}

	runState := svc.currentRunState()
	if runState == nil {
		t.Fatal("runState = nil after Run, want default session run state")
	}
	if runState.sessionID != defaultFactorySessionID {
		t.Fatalf("runState.sessionID = %q, want %q", runState.sessionID, defaultFactorySessionID)
	}
	if runState.runtime != defaultHandle {
		t.Fatal("runState.runtime != default session live handle")
	}
	if current := svc.currentSession(); current == nil || current.ID != defaultFactorySessionID {
		t.Fatalf("currentSession = %#v, want selected %q", current, defaultFactorySessionID)
	}
	if bundle := svc.currentRuntimeBundle(); bundle != defaultHandle.runtime {
		t.Fatal("currentRuntimeBundle should resolve through the default session registry handle after Run")
	}
}

func createReplacementWatchChannel(t *testing.T, factoryDir, workType, channel string) {
	t.Helper()

	inputDir := filepath.Join(factoryDir, interfaces.InputsDir, workType, channel)
	if err := os.MkdirAll(inputDir, 0o755); err != nil {
		t.Fatalf("create watched input dir %q: %v", inputDir, err)
	}
}

func writeNamedFactoryFixture(t *testing.T, rootDir, name string) string {
	t.Helper()

	payload, err := json.Marshal(map[string]any{
		"name": name,
		"id":   name,
		"workTypes": []map[string]any{
			{
				"name": "task",
				"states": []map[string]string{
					{"name": "init", "type": "INITIAL"},
					{"name": "complete", "type": "TERMINAL"},
					{"name": "failed", "type": "FAILED"},
				},
			},
		},
		"workers": []map[string]any{
			{
				"name": "executor",
				"type": "MODEL_WORKER",
				"body": "You are the executor.",
			},
		},
		"workstations": []map[string]any{
			{
				"name":      "execute-" + name,
				"worker":    "executor",
				"inputs":    []map[string]string{{"workType": "task", "state": "init"}},
				"outputs":   []map[string]string{{"workType": "task", "state": "complete"}},
				"onFailure": []map[string]string{{"workType": "task", "state": "failed"}},
				"type":      "MODEL_WORKSTATION",
				"body":      "Implement {{ .WorkID }}.",
			},
		},
	})
	if err != nil {
		t.Fatalf("Marshal(named factory fixture): %v", err)
	}

	factoryDir, err := config.PersistNamedFactory(rootDir, name, payload)
	if err != nil {
		t.Fatalf("PersistNamedFactory(%s): %v", name, err)
	}
	return factoryDir
}

