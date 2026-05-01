package functional_test

import (
	"context"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/portpowered/infinite-you/pkg/interfaces"
	"github.com/portpowered/infinite-you/pkg/petri"

	"github.com/portpowered/infinite-you/pkg/factory/state"
	"github.com/portpowered/infinite-you/pkg/testutil"
)

// singleStagePipelineConfig returns a 1-stage pipeline (init → complete)
// so multiple work items all hit the same transition concurrently.
func singleStagePipelineConfig() *interfaces.FactoryConfig {
	return &interfaces.FactoryConfig{
		WorkTypes: []interfaces.WorkTypeConfig{{
			Name: "task",
			States: []interfaces.StateConfig{
				{Name: "init", Type: interfaces.StateTypeInitial},
				{Name: "complete", Type: interfaces.StateTypeTerminal},
				{Name: "failed", Type: interfaces.StateTypeFailed},
			},
		}},
		Workers: []interfaces.WorkerConfig{{Name: "step-worker"}},
		Workstations: []interfaces.FactoryWorkstationConfig{
			{Name: "process", WorkerTypeName: "step-worker",
				Inputs:  []interfaces.IOConfig{{WorkTypeName: "task", StateName: "init"}},
				Outputs: []interfaces.IOConfig{{WorkTypeName: "task", StateName: "complete"}}},
		},
	}
}

// --- US-004: single work item dashboard snapshot ---

// TestDashboard_SingleWorkItemSnapshot dispatches a single work item and
// confirms the dashboard runtime-state snapshot reflects the correct state
// at each lifecycle point (in-flight and completed).
// portos:func-length-exception owner=agent-factory reason=legacy-functional-dashboard-lifecycle-smoke review=2026-07-18 removal=split-dashboard-snapshot-smoke-setup-and-assertions
func TestDashboard_SingleWorkItemSnapshot(t *testing.T) {
	dir := testutil.ScaffoldFactoryDir(t, persistTestPipelineConfig())

	// snapshotExecutor captures a runtime-state snapshot synchronously
	// during dispatch, then returns ACCEPTED.
	var snapshotMu sync.Mutex
	var capturedSnapshot *interfaces.EngineStateSnapshot[petri.MarkingSnapshot, *state.Net]

	snapshotExec := &snapshotCapturingExecutor{
		mu:       &snapshotMu,
		snapshot: &capturedSnapshot,
	}

	h := testutil.NewServiceTestHarness(t, dir,
		testutil.WithRunAsync(),
	)
	h.SetCustomExecutor("step-worker", snapshotExec)

	// Provide the harness reference so the executor can call GetEngineStateSnapshot.
	snapshotExec.harness = h

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	errCh := h.RunInBackground(ctx)

	// Submit one work item.
	h.SubmitWork("task", []byte(`{"item": "snapshot-test"}`))

	// Wait for at least one dispatch to capture a snapshot.
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

	// Verify the snapshot captured during dispatch.
	snapshotMu.Lock()
	snap := capturedSnapshot
	snapshotMu.Unlock()

	if len(snap.Dispatches) == 0 {
		t.Fatal("expected at least 1 in-flight dispatch in snapshot, got 0")
	}

	// Verify the dispatch entry has correct fields.
	for _, d := range snap.Dispatches {
		tokenIdentities := deriveTokenIdentities(d.ConsumedTokens, nil)
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

	// Verify work summary includes the dispatched token: HeldMutations
	// should contain at least one CONSUME mutation.
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

	// Wait for completion.
	select {
	case <-h.WaitToComplete():
		cancel()
	case <-ctx.Done():
		t.Fatal("timed out waiting for factory to complete")
	}
	if err := <-errCh; err != nil && err != context.Canceled {
		t.Fatalf("factory run error: %v", err)
	}

	// After completion: verify dispatch history contains the work item.
	rtAfter, err := h.GetEngineStateSnapshot()
	if err != nil {
		t.Fatalf("GetEngineStateSnapshot after completion failed: %v", err)
	}
	if len(rtAfter.DispatchHistory) == 0 {
		t.Error("expected at least 1 entry in completed workstations (DispatchHistory)")
	}
}

// snapshotCapturingExecutor captures a runtime-state snapshot on the first
// dispatch, then immediately returns ACCEPTED.
type snapshotCapturingExecutor struct {
	harness  *testutil.ServiceTestHarness
	mu       *sync.Mutex
	snapshot **interfaces.EngineStateSnapshot[petri.MarkingSnapshot, *state.Net]
	captured atomic.Bool
}

func (e *snapshotCapturingExecutor) Execute(_ context.Context, d interfaces.WorkDispatch) (interfaces.WorkResult, error) {
	// Capture snapshot only on the first call to avoid races on later dispatches.
	if !e.captured.Load() {
		if rt, err := e.harness.GetEngineStateSnapshot(); err == nil {
			e.mu.Lock()
			*e.snapshot = rt
			e.mu.Unlock()
			e.captured.Store(true)
		}
	}
	return interfaces.WorkResult{
		DispatchID:   d.DispatchID,
		TransitionID: d.TransitionID,
		Outcome:      interfaces.OutcomeAccepted,
	}, nil
}

// --- US-005: parallel work items dashboard snapshot ---

// TestDashboard_ParallelWorkItemsSnapshot dispatches multiple work items
// concurrently and confirms the dashboard reports all of them as actively
// dispatching.
// portos:func-length-exception owner=agent-factory reason=legacy-functional-dashboard-parallel-smoke review=2026-07-18 removal=split-parallel-dashboard-snapshot-setup-and-assertions
func TestDashboard_ParallelWorkItemsSnapshot(t *testing.T) {
	// Use a single-stage pipeline so all items hit the same transition concurrently.
	cfg := singleStagePipelineConfig()
	dir := testutil.ScaffoldFactoryDir(t, cfg)

	const numItems = 3

	// barrierExecutor blocks all dispatches until the expected number arrive,
	// then captures a snapshot and releases them all.
	var snapshotMu sync.Mutex
	var capturedSnapshot *interfaces.EngineStateSnapshot[petri.MarkingSnapshot, *state.Net]

	barrier := &barrierSnapshotExecutor{
		expected: numItems,
		mu:       &snapshotMu,
		snapshot: &capturedSnapshot,
	}

	h := testutil.NewServiceTestHarness(t, dir,
		// TODO: migrate to WithFullWorkerPoolAndScriptWrap
		testutil.WithRunAsync(),
	)
	h.SetCustomExecutor("step-worker", barrier)
	barrier.harness = h

	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	errCh := h.RunInBackground(ctx)

	// Submit multiple work items.
	for i := 0; i < numItems; i++ {
		h.SubmitWork("task", []byte(`{"item": "parallel-test"}`))
	}

	// Wait for the barrier to capture a snapshot.
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

	// Verify the snapshot shows all dispatched items as active.
	snapshotMu.Lock()
	snap := capturedSnapshot
	snapshotMu.Unlock()

	// Use InFlightCount for accuracy (map may have key collisions for same transition).
	if snap.InFlightCount < numItems {
		t.Errorf("expected InFlightCount >= %d, got %d", numItems, snap.InFlightCount)
	}

	// Verify held tokens account for all dispatched items.
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

	// Wait for completion.
	select {
	case <-h.WaitToComplete():
		cancel()
	case <-ctx.Done():
		t.Fatal("timed out waiting for factory to complete")
	}
	if err := <-errCh; err != nil && err != context.Canceled {
		t.Fatalf("factory run error: %v", err)
	}

	// After completion: verify all tokens reached terminal state.
	// Note: DispatchHistory may have fewer entries than numItems due to
	// Dispatches map key collisions (same TransitionID for each firing),
	// so we verify completion via the marking instead.
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

// barrierSnapshotExecutor blocks until the expected number of concurrent
// dispatches arrive, captures a runtime-state snapshot, then releases all.
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
	e.initOnce.Do(func() {
		e.release = make(chan struct{})
	})

	count := int(e.arrived.Add(1))

	if count >= e.expected {
		// All expected dispatches have arrived — capture snapshot.
		if rt, err := e.harness.GetEngineStateSnapshot(); err == nil {
			e.mu.Lock()
			*e.snapshot = rt
			e.mu.Unlock()
		}
		close(e.release)
	} else {
		// Wait for all dispatches to arrive.
		<-e.release
	}

	return interfaces.WorkResult{
		DispatchID:   d.DispatchID,
		TransitionID: d.TransitionID,
		Outcome:      interfaces.OutcomeAccepted,
	}, nil
}
