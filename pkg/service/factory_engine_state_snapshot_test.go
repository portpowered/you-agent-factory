package service

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/portpowered/infinite-you/pkg/config"
	"github.com/portpowered/infinite-you/pkg/factory/state"
	"github.com/portpowered/infinite-you/pkg/interfaces"
	"github.com/portpowered/infinite-you/pkg/petri"
	"go.uber.org/zap"
)

func TestGetEngineStateSnapshot_AggregatesAllState(t *testing.T) {
	dir := t.TempDir()
	writeFactoryJSON(t, dir, minimalFactoryConfig())
	writeWorkstationAgentsMD(t, dir, "process")
	if err := os.MkdirAll(filepath.Join(dir, interfaces.InputsDir), 0o755); err != nil {
		t.Fatalf("create inputs dir: %v", err)
	}

	ctx := context.Background()
	svc, err := BuildFactoryService(ctx, &FactoryServiceConfig{
		Dir:               dir,
		MockWorkersConfig: config.NewEmptyMockWorkersConfig(),
		Logger:            zap.NewNop(),
	})
	if err != nil {
		t.Fatalf("BuildFactoryService: %v", err)
	}

	snap, err := svc.GetEngineStateSnapshot(ctx)
	if err != nil {
		t.Fatalf("GetEngineStateSnapshot: %v", err)
	}

	if snap.FactoryState != string(interfaces.FactoryStateIdle) {
		t.Errorf("expected FactoryState=IDLE, got %s", snap.FactoryState)
	}
	if snap.RuntimeStatus != interfaces.RuntimeStatusIdle {
		t.Errorf("expected RuntimeStatus=IDLE, got %s", snap.RuntimeStatus)
	}
	if snap.TickCount != 0 {
		t.Errorf("expected TickCount=0, got %d", snap.TickCount)
	}
	if snap.Topology == nil {
		t.Fatal("expected aggregate snapshot topology")
	}
	if _, ok := snap.Topology.WorkTypes["task"]; !ok {
		t.Fatalf("expected topology to include task work type, got %#v", snap.Topology.WorkTypes)
	}
	if snap.Uptime != 0 {
		t.Errorf("expected zero uptime before runtime start, got %v", snap.Uptime)
	}
}

func TestFactoryService_GetEngineStateSnapshot_DelegatesToFactoryAggregateSnapshot(t *testing.T) {
	topology := &state.Net{ID: "aggregate-net"}
	expected := &interfaces.EngineStateSnapshot[petri.MarkingSnapshot, *state.Net]{
		RuntimeStatus: interfaces.RuntimeStatusActive,
		FactoryState:  string(interfaces.FactoryStateRunning),
		Uptime:        42 * time.Second,
		Topology:      topology,
		InFlightCount: 3,
		TickCount:     7,
	}
	mock := &aggregateSnapshotFactory{engineState: expected}
	svc := &FactoryService{}
	bindServiceStartupRuntime(svc, &factoryRuntimeBundle{factory: mock})

	got, err := svc.GetEngineStateSnapshot(context.Background())
	if err != nil {
		t.Fatalf("GetEngineStateSnapshot: %v", err)
	}
	if got != expected {
		t.Fatalf("service returned %#v, want factory aggregate snapshot %#v", got, expected)
	}
	if mock.engineStateSnapshotCalls != 1 {
		t.Fatalf("factory aggregate snapshot calls = %d, want 1", mock.engineStateSnapshotCalls)
	}
}

func TestFactoryService_GetEngineStateSnapshot_ReportsIdleActiveAndFinishedStates(t *testing.T) {
	svc, releaseCh := buildServiceModeSnapshotFixture(t)
	runCtx, cancelRun := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancelRun()

	errCh := make(chan error, 1)
	go func() {
		errCh <- svc.Run(runCtx)
	}()

	waitForSnapshotMatch(t, svc, time.Second, "idle startup", func(snap *interfaces.EngineStateSnapshot[petri.MarkingSnapshot, *state.Net]) bool {
		return snap.RuntimeStatus == interfaces.RuntimeStatusIdle
	})
	submitSnapshotStatusWork(t, svc)
	waitForSnapshotMatch(t, svc, time.Second, "active work", func(snap *interfaces.EngineStateSnapshot[petri.MarkingSnapshot, *state.Net]) bool {
		return snap.RuntimeStatus == interfaces.RuntimeStatusActive && snap.InFlightCount > 0
	})

	close(releaseCh)
	waitForSnapshotMatch(t, svc, time.Second, "idle after completion", func(snap *interfaces.EngineStateSnapshot[petri.MarkingSnapshot, *state.Net]) bool {
		return snapshotHasCompletedTaskToken(snap)
	})

	cancelRun()
	if err := <-errCh; err != nil {
		t.Fatalf("service-mode run error: %v", err)
	}

	batchSvc := buildBatchSnapshotFixture(t)
	batchCtx, cancelBatch := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancelBatch()
	if err := batchSvc.Run(batchCtx); err != nil {
		t.Fatalf("batch Run: %v", err)
	}

	terminalSnap, err := batchSvc.GetEngineStateSnapshot(context.Background())
	if err != nil {
		t.Fatalf("GetEngineStateSnapshot terminal: %v", err)
	}
	if terminalSnap.RuntimeStatus != interfaces.RuntimeStatusFinished {
		t.Fatalf("terminal runtime status = %q, want %q", terminalSnap.RuntimeStatus, interfaces.RuntimeStatusFinished)
	}
	if terminalSnap.FactoryState != string(interfaces.FactoryStateCompleted) {
		t.Fatalf("terminal factory state = %q, want %q", terminalSnap.FactoryState, interfaces.FactoryStateCompleted)
	}
}

func buildServiceModeSnapshotFixture(t *testing.T) (*FactoryService, chan struct{}) {
	t.Helper()

	dir := t.TempDir()
	writeFactoryJSON(t, dir, minimalFactoryConfig())
	writeWorkstationAgentsMD(t, dir, "process")
	writeWorkerAgentsMD(t, dir, "worker-a")
	if err := os.MkdirAll(filepath.Join(dir, interfaces.InputsDir), 0o755); err != nil {
		t.Fatalf("create inputs dir: %v", err)
	}

	releaseCh := make(chan struct{})
	provider := &blockingInferenceProvider{releaseCh: releaseCh, content: "COMPLETE"}
	svc, err := BuildFactoryService(context.Background(), &FactoryServiceConfig{
		Dir:              dir,
		RuntimeMode:      interfaces.RuntimeModeService,
		Logger:           zap.NewNop(),
		ProviderOverride: provider,
	})
	if err != nil {
		t.Fatalf("BuildFactoryService: %v", err)
	}
	return svc, releaseCh
}

func buildBatchSnapshotFixture(t *testing.T) *FactoryService {
	t.Helper()

	dir := t.TempDir()
	writeFactoryJSON(t, dir, minimalFactoryConfig())
	writeWorkstationAgentsMD(t, dir, "process")
	if err := os.MkdirAll(filepath.Join(dir, interfaces.InputsDir, "task", interfaces.DefaultChannelName), 0o755); err != nil {
		t.Fatalf("create batch inputs dir: %v", err)
	}
	workFile := filepath.Join(dir, interfaces.InputsDir, "task", interfaces.DefaultChannelName, "seed.json")
	if err := os.WriteFile(workFile, []byte(`{"title":"terminal-status"}`), 0o644); err != nil {
		t.Fatalf("write seed work file: %v", err)
	}

	svc, err := BuildFactoryService(context.Background(), &FactoryServiceConfig{
		Dir:               dir,
		MockWorkersConfig: config.NewEmptyMockWorkersConfig(),
		Logger:            zap.NewNop(),
	})
	if err != nil {
		t.Fatalf("BuildFactoryService batch: %v", err)
	}
	return svc
}

func submitSnapshotStatusWork(t *testing.T, svc *FactoryService) {
	t.Helper()

	if err := submitWorkRequestsToService(context.Background(), svc, []interfaces.SubmitRequest{{
		WorkTypeID: "task",
		TraceID:    "trace-engine-state-statuses",
		Payload:    json.RawMessage(`{"title":"runtime-statuses"}`),
	}}); err != nil {
		t.Fatalf("Submit: %v", err)
	}
}

func waitForSnapshotMatch(
	t *testing.T,
	svc *FactoryService,
	timeout time.Duration,
	phase string,
	match func(*interfaces.EngineStateSnapshot[petri.MarkingSnapshot, *state.Net]) bool,
) *interfaces.EngineStateSnapshot[petri.MarkingSnapshot, *state.Net] {
	t.Helper()

	deadline := time.Now().Add(timeout)
	var last *interfaces.EngineStateSnapshot[petri.MarkingSnapshot, *state.Net]
	for time.Now().Before(deadline) {
		snap, err := svc.GetEngineStateSnapshot(context.Background())
		if err != nil {
			t.Fatalf("GetEngineStateSnapshot during %s: %v", phase, err)
		}
		last = snap
		if match(snap) {
			return snap
		}
		time.Sleep(10 * time.Millisecond)
	}
	if last == nil {
		t.Fatalf("timed out waiting for %s snapshot", phase)
	}
	t.Fatalf("timed out waiting for %s, last status=%q inflight=%d tokens=%d", phase, last.RuntimeStatus, last.InFlightCount, len(last.Marking.Tokens))
	return nil
}

func snapshotHasCompletedTaskToken(snap *interfaces.EngineStateSnapshot[petri.MarkingSnapshot, *state.Net]) bool {
	if snap.RuntimeStatus != interfaces.RuntimeStatusIdle || len(snap.Marking.Tokens) != 1 {
		return false
	}
	for _, token := range snap.Marking.Tokens {
		return token.PlaceID == "task:complete"
	}
	return false
}
