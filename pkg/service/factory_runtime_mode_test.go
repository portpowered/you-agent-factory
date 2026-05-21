package service

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	factoryapi "github.com/portpowered/infinite-you/pkg/api/generated"
	"github.com/portpowered/infinite-you/pkg/config"
	"github.com/portpowered/infinite-you/pkg/interfaces"
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
