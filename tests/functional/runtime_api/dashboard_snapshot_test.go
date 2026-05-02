package runtime_api

import (
	"context"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/portpowered/infinite-you/pkg/factory/state"
	"github.com/portpowered/infinite-you/pkg/interfaces"
	"github.com/portpowered/infinite-you/pkg/petri"
	"github.com/portpowered/infinite-you/pkg/testutil"
	"github.com/portpowered/infinite-you/tests/functional/internal/support"
)

func singleStagePipelineConfig() *interfaces.FactoryConfig {
	return &interfaces.FactoryConfig{
		WorkTypes: []interfaces.WorkTypeConfig{{Name: "task", States: []interfaces.StateConfig{
			{Name: "init", Type: interfaces.StateTypeInitial},
			{Name: "complete", Type: interfaces.StateTypeTerminal},
			{Name: "failed", Type: interfaces.StateTypeFailed},
		}}},
		Workers: []interfaces.WorkerConfig{{Name: "step-worker"}},
		Workstations: []interfaces.FactoryWorkstationConfig{{
			Name: "process", WorkerTypeName: "step-worker",
			Inputs:  []interfaces.IOConfig{{WorkTypeName: "task", StateName: "init"}},
			Outputs: []interfaces.IOConfig{{WorkTypeName: "task", StateName: "complete"}},
		}},
	}
}

// portos:func-length-exception owner=agent-factory reason=dashboard-single-snapshot-smoke review=2026-07-22 removal=split-runtime-setup-selected-work-and-world-view-assertions-before-next-dashboard-snapshot-change
func TestDashboard_SingleWorkItemSnapshot(t *testing.T) {
	support.SkipLongFunctional(t, "slow dashboard single-work snapshot sweep")
	dir := testutil.ScaffoldFactoryDir(t, persistTestPipelineConfig())

	var snapshotMu sync.Mutex
	var capturedSnapshot *interfaces.EngineStateSnapshot[petri.MarkingSnapshot, *state.Net]

	snapshotExec := &snapshotCapturingExecutor{mu: &snapshotMu, snapshot: &capturedSnapshot}

	h := testutil.NewServiceTestHarness(t, dir, testutil.WithRunAsync())
	h.SetCustomExecutor("step-worker", snapshotExec)
	snapshotExec.harness = h

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	errCh := h.RunInBackground(ctx)

	h.SubmitWork("task", []byte(`{"item": "snapshot-test"}`))

	deadline := time.After(5 * time.Second)
	for {
		snapshotMu.Lock()
		got := capturedSnapshot
		snapshotMu.Unlock()
		if got != nil {
			break
		}
		select {
		case <-deadline:
			t.Fatal("timed out waiting for executor to capture snapshot")
		default:
			time.Sleep(20 * time.Millisecond)
		}
	}

	snapshotMu.Lock()
	snap := capturedSnapshot
	snapshotMu.Unlock()

	if len(snap.Dispatches) == 0 {
		t.Fatal("expected at least 1 in-flight dispatch in snapshot, got 0")
	}
	for _, d := range snap.Dispatches {
		tokenIdentities := support.DeriveTokenIdentities(d.ConsumedTokens, nil)
		if len(tokenIdentities.WorkIDs) == 0 {
			t.Error("expected at least one work ID for in-flight dispatch")
		}
		if len(tokenIdentities.WorkTypes) == 0 {
			t.Error("expected at least one work type for in-flight dispatch")
		}
		if d.TransitionID == "" {
			t.Error("expected non-empty TransitionID for in-flight dispatch")
		}
	}

	heldTokens := 0
	for _, d := range snap.Dispatches {
		for _, m := range d.HeldMutations {
			if m.Type == interfaces.MutationConsume {
				heldTokens++
			}
		}
	}
	if heldTokens == 0 {
		t.Error("expected at least 1 held token (CONSUME mutation) in active dispatches")
	}

	select {
	case <-h.WaitToComplete():
		cancel()
	case <-ctx.Done():
		t.Fatal("timed out waiting for factory to complete")
	}
	if err := <-errCh; err != nil && err != context.Canceled {
		t.Fatalf("factory run error: %v", err)
	}

	rtAfter, err := h.GetEngineStateSnapshot()
	if err != nil {
		t.Fatalf("GetEngineStateSnapshot after completion failed: %v", err)
	}
	if len(rtAfter.DispatchHistory) == 0 {
		t.Error("expected at least 1 entry in completed workstations (DispatchHistory)")
	}
}

type snapshotCapturingExecutor struct {
	harness  *testutil.ServiceTestHarness
	mu       *sync.Mutex
	snapshot **interfaces.EngineStateSnapshot[petri.MarkingSnapshot, *state.Net]
	captured atomic.Bool
}

func (e *snapshotCapturingExecutor) Execute(_ context.Context, d interfaces.WorkDispatch) (interfaces.WorkResult, error) {
	if !e.captured.Load() {
		if rt, err := e.harness.GetEngineStateSnapshot(); err == nil {
			e.mu.Lock()
			*e.snapshot = rt
			e.mu.Unlock()
			e.captured.Store(true)
		}
	}
	return interfaces.WorkResult{DispatchID: d.DispatchID, TransitionID: d.TransitionID, Outcome: interfaces.OutcomeAccepted}, nil
}

// portos:func-length-exception owner=agent-factory reason=dashboard-parallel-snapshot-smoke review=2026-07-22 removal=split-parallel-submission-occupancy-and-dashboard-assertions-before-next-dashboard-snapshot-change
func TestDashboard_ParallelWorkItemsSnapshot(t *testing.T) {
	support.SkipLongFunctional(t, "slow dashboard parallel snapshot sweep")
	cfg := singleStagePipelineConfig()
	dir := testutil.ScaffoldFactoryDir(t, cfg)

	const numItems = 3

	var snapshotMu sync.Mutex
	var capturedSnapshot *interfaces.EngineStateSnapshot[petri.MarkingSnapshot, *state.Net]

	barrier := &barrierSnapshotExecutor{expected: numItems, mu: &snapshotMu, snapshot: &capturedSnapshot}

	h := testutil.NewServiceTestHarness(t, dir, testutil.WithRunAsync())
	h.SetCustomExecutor("step-worker", barrier)
	barrier.harness = h

	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	errCh := h.RunInBackground(ctx)

	for i := 0; i < numItems; i++ {
		h.SubmitWork("task", []byte(`{"item": "parallel-test"}`))
	}

	deadline := time.After(10 * time.Second)
	for {
		snapshotMu.Lock()
		got := capturedSnapshot
		snapshotMu.Unlock()
		if got != nil {
			break
		}
		select {
		case <-deadline:
			t.Fatal("timed out waiting for barrier executor to capture snapshot")
		default:
			time.Sleep(20 * time.Millisecond)
		}
	}

	snapshotMu.Lock()
	snap := capturedSnapshot
	snapshotMu.Unlock()

	if snap.InFlightCount < numItems {
		t.Errorf("expected InFlightCount >= %d, got %d", numItems, snap.InFlightCount)
	}

	heldTokens := 0
	for _, d := range snap.Dispatches {
		for _, m := range d.HeldMutations {
			if m.Type == interfaces.MutationConsume {
				heldTokens++
			}
		}
	}
	if heldTokens == 0 {
		t.Error("expected held tokens (CONSUME mutations) in active dispatches")
	}

	select {
	case <-h.WaitToComplete():
		cancel()
	case <-ctx.Done():
		t.Fatal("timed out waiting for factory to complete")
	}
	if err := <-errCh; err != nil && err != context.Canceled {
		t.Fatalf("factory run error: %v", err)
	}

	rtAfter, err := h.GetEngineStateSnapshot()
	if err != nil {
		t.Fatalf("GetEngineStateSnapshot after completion failed: %v", err)
	}
	terminalCount := 0
	for _, tok := range rtAfter.Marking.Tokens {
		if tok.PlaceID == "task:complete" {
			terminalCount++
		}
	}
	if terminalCount < numItems {
		t.Errorf("expected %d tokens in terminal state, got %d", numItems, terminalCount)
	}
	if len(rtAfter.DispatchHistory) == 0 {
		t.Error("expected at least 1 entry in completed workstations (DispatchHistory)")
	}
}

type barrierSnapshotExecutor struct {
	harness  *testutil.ServiceTestHarness
	expected int
	mu       *sync.Mutex
	snapshot **interfaces.EngineStateSnapshot[petri.MarkingSnapshot, *state.Net]
	arrived  atomic.Int32
	release  chan struct{}
	initOnce sync.Once
}

func (e *barrierSnapshotExecutor) Execute(_ context.Context, d interfaces.WorkDispatch) (interfaces.WorkResult, error) {
	e.initOnce.Do(func() { e.release = make(chan struct{}) })

	count := int(e.arrived.Add(1))
	if count >= e.expected {
		if rt, err := e.harness.GetEngineStateSnapshot(); err == nil {
			e.mu.Lock()
			*e.snapshot = rt
			e.mu.Unlock()
		}
		close(e.release)
	} else {
		<-e.release
	}

	return interfaces.WorkResult{DispatchID: d.DispatchID, TransitionID: d.TransitionID, Outcome: interfaces.OutcomeAccepted}, nil
}
