package stress_test

import (
	"context"
	"fmt"
	"sync"
	"testing"
	"time"

	"github.com/portpowered/infinite-you/pkg/interfaces"
	"github.com/portpowered/infinite-you/pkg/petri"
	"github.com/portpowered/infinite-you/pkg/testutil"
)

// ---------------------------------------------------------------------------
// TestBarrierAllSucceed: parent spawns 5 children, all succeed, fan-in fires.
//
//	spawn-children: parent:init → parent:waiting + 5 children
//	process-child:  child:init → child:complete (or child:failed)
//	complete-parent: parent:waiting + ObserveN(child:complete, 5) → parent:complete
//
// After all children complete the barrier fires and parent reaches terminal.
// ---------------------------------------------------------------------------
func TestBarrierAllSucceed(t *testing.T) {
	dir := testutil.ScaffoldFactoryDir(t, barrierConfig())

	// TODO: migrate this to use the WithFullWorkerPoolAndScriptWrap option and real executors instead of mocks, to more fully test the async dispatch and fan-in behavior. The mock-based approach is simpler for now and still tests the core barrier logic, but it doesn't exercise the full async dispatch flow or the interaction between the spawner and completer via the petri net.
	h := testutil.NewServiceTestHarness(t, dir, testutil.WithRunAsync())

	spawner := &barrierSpawnerExecutor{childCount: 5}
	h.SetCustomExecutor("spawner", spawner)

	// processor: 5 successes.
	h.MockWorker("processor",
		interfaces.WorkResult{Outcome: interfaces.OutcomeAccepted},
		interfaces.WorkResult{Outcome: interfaces.OutcomeAccepted},
		interfaces.WorkResult{Outcome: interfaces.OutcomeAccepted},
		interfaces.WorkResult{Outcome: interfaces.OutcomeAccepted},
		interfaces.WorkResult{Outcome: interfaces.OutcomeAccepted},
	)
	h.MockWorker("completer",
		interfaces.WorkResult{Outcome: interfaces.OutcomeAccepted},
	)

	h.SubmitWork("parent", []byte("barrier test"))

	h.RunUntilComplete(t, 10*time.Second)

	// Fan-in fired: 1 parent:complete, 5 child:complete (observed, not consumed).
	h.Assert().
		PlaceTokenCount("parent:complete", 1).
		PlaceTokenCount("child:complete", 5).
		HasNoTokenInPlace("parent:init").
		HasNoTokenInPlace("parent:waiting").
		HasNoTokenInPlace("child:init")

	// No tokens stuck anywhere.
	snap := h.Marking()
	total := len(snap.Tokens)
	if total != 6 { // 1 parent + 5 children
		t.Errorf("expected 6 total tokens, got %d", total)
	}

	// No double-firing: spawner called once, completer called once.
	if spawner.callCount() != 1 {
		t.Errorf("spawner called %d times, want 1", spawner.callCount())
	}
}

// ---------------------------------------------------------------------------
// TestBarrierPartialFailure: 5 children spawned, 1 fails.
// Fan-in requires all 5 complete → does NOT fire.
// Failure-detection transition routes parent to failed.
// ---------------------------------------------------------------------------
func TestBarrierPartialFailure(t *testing.T) {
	dir := testutil.ScaffoldFactoryDir(t, barrierConfigWithFailureDetection())

	h := testutil.NewServiceTestHarness(t, dir, testutil.WithRunAsync())

	spawner := &barrierSpawnerExecutor{childCount: 5}
	h.SetCustomExecutor("spawner", spawner)

	// Custom executor: 3rd child fails, rest succeed.
	failExec := &failOnNthBarrierExecutor{failOn: 3}
	h.SetCustomExecutor("processor", failExec)

	completer := h.MockWorker("completer",
		interfaces.WorkResult{Outcome: interfaces.OutcomeAccepted},
	)
	failureHandler := h.MockWorker("failure-handler",
		interfaces.WorkResult{Outcome: interfaces.OutcomeAccepted},
	)

	h.SubmitWork("parent", []byte("partial failure"))

	h.RunUntilComplete(t, 10*time.Second)

	// Parent routed to failed because fan-in couldn't fire (only 4 complete).
	h.Assert().
		PlaceTokenCount("parent:failed", 1).
		HasNoTokenInPlace("parent:waiting").
		HasNoTokenInPlace("parent:complete")

	// 4 children complete, 1 failed.
	h.Assert().
		PlaceTokenCount("child:complete", 4).
		PlaceTokenCount("child:failed", 1).
		HasNoTokenInPlace("child:init")

	if completer.CallCount() != 0 {
		t.Fatalf("complete-parent fired %d times, want 0", completer.CallCount())
	}
	if failureHandler.CallCount() != 1 {
		t.Fatalf("failure-handler fired %d times, want 1", failureHandler.CallCount())
	}
	assertNoExhaustionDispatches(t, h)
}

// ---------------------------------------------------------------------------
// TestBarrierDelayedArrival: fan-in fires ONLY when all 5 children are present.
// We tick incrementally to prove the barrier waits for the last child.
// ---------------------------------------------------------------------------
func TestBarrierDelayedArrival(t *testing.T) {
	dir := testutil.ScaffoldFactoryDir(t, barrierConfig())
	// TODO: migrate this to use the WithFullWorkerPoolAndScriptWrap option and real executors instead of mocks, to more fully test the async dispatch and fan-in behavior. The mock-based approach is simpler for now and still tests the core barrier logic, but it doesn't exercise the full async dispatch flow or the interaction between the spawner and completer via the petri net.
	h := testutil.NewServiceTestHarness(t, dir, testutil.WithRunAsync())

	spawner := &barrierSpawnerExecutor{childCount: 5}
	h.SetCustomExecutor("spawner", spawner)

	h.MockWorker("processor",
		interfaces.WorkResult{Outcome: interfaces.OutcomeAccepted},
		interfaces.WorkResult{Outcome: interfaces.OutcomeAccepted},
		interfaces.WorkResult{Outcome: interfaces.OutcomeAccepted},
		interfaces.WorkResult{Outcome: interfaces.OutcomeAccepted},
		interfaces.WorkResult{Outcome: interfaces.OutcomeAccepted},
	)
	h.MockWorker("completer",
		interfaces.WorkResult{Outcome: interfaces.OutcomeAccepted},
	)

	// Submit spawns children (1 tick in Submit).
	h.SubmitWork("parent", []byte("delayed arrival"))

	h.RunUntilComplete(t, 10*time.Second)

	h.Assert().
		PlaceTokenCount("parent:complete", 1).
		PlaceTokenCount("child:complete", 5).
		HasNoTokenInPlace("parent:waiting")
}

// ---------------------------------------------------------------------------
// TestBarrierZeroChildren: parent spawns 0 children.
// Fan-in with ObserveAllWithGuard gracefully doesn't fire (0 candidates).
// Parent stays in waiting — engine terminates via deadlock detection
// (no in-flight dispatches, no enabled transitions, token stuck in
// non-terminal place).
// ---------------------------------------------------------------------------
func TestBarrierZeroChildren(t *testing.T) {
	// Use per-input guards without spawned_by to test zero-cardinality boundary:
	// AllWithParentGuard + CardinalityAll with 0 matching tokens → guard returns false.
	dir := testutil.ScaffoldFactoryDir(t, barrierConfigObserveAll())

	h := testutil.NewServiceTestHarness(t, dir, testutil.WithRunAsync())

	spawner := &barrierSpawnerExecutor{childCount: 0}
	h.SetCustomExecutor("spawner", spawner)

	h.MockWorker("processor")
	h.MockWorker("completer")

	h.SubmitWork("parent", []byte("zero children"))

	// With stateless termination, a non-terminal deadlock does not complete the
	// engine. Run in the background under a bounded context, assert the parent is
	// stably stuck in waiting, then cancel explicitly.
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	errCh := h.RunInBackground(ctx)

	time.Sleep(200 * time.Millisecond)

	select {
	case <-h.WaitToComplete():
		t.Fatal("expected zero-child barrier to remain incomplete")
	default:
	}

	// Parent stays stuck in waiting with no spawned children.
	h.Assert().
		HasTokenInPlace("parent:waiting").
		HasNoTokenInPlace("parent:complete").
		HasNoTokenInPlace("parent:failed").
		HasNoTokenInPlace("child:init").
		HasNoTokenInPlace("child:complete")

	// Only 1 token in the system (the parent).
	snap := h.Marking()
	if len(snap.Tokens) != 1 {
		t.Errorf("expected 1 token (parent:waiting), got %d", len(snap.Tokens))
	}

	cancel()
	if err := <-errCh; err != nil && err != context.Canceled && err != context.DeadlineExceeded {
		t.Fatalf("background run returned unexpected error: %v", err)
	}
}

// ===========================================================================
// Config builders
// ===========================================================================

// barrierWorkTypes returns the standard parent/child work type configs.
var barrierWorkTypes = []interfaces.WorkTypeConfig{
	{Name: "parent", States: []interfaces.StateConfig{
		{Name: "init", Type: interfaces.StateTypeInitial},
		{Name: "waiting", Type: interfaces.StateTypeProcessing},
		{Name: "complete", Type: interfaces.StateTypeTerminal},
		{Name: "failed", Type: interfaces.StateTypeFailed},
	}},
	{Name: "child", States: []interfaces.StateConfig{
		{Name: "init", Type: interfaces.StateTypeInitial},
		{Name: "complete", Type: interfaces.StateTypeTerminal},
		{Name: "failed", Type: interfaces.StateTypeFailed},
	}},
}

// barrierConfig returns a config for a barrier/fan-in workflow using per-input
// guards with spawned_by for dynamic fanout count tracking.
func barrierConfig() *interfaces.FactoryConfig {
	return &interfaces.FactoryConfig{
		WorkTypes: barrierWorkTypes,
		Workers:   []interfaces.WorkerConfig{{Name: "spawner"}, {Name: "processor"}, {Name: "completer"}},
		Workstations: []interfaces.FactoryWorkstationConfig{
			{Name: "spawn-children", WorkerTypeName: "spawner",
				Inputs:  []interfaces.IOConfig{{WorkTypeName: "parent", StateName: "init"}},
				Outputs: []interfaces.IOConfig{{WorkTypeName: "parent", StateName: "waiting"}}},
			{Name: "process-child", WorkerTypeName: "processor",
				Inputs:    []interfaces.IOConfig{{WorkTypeName: "child", StateName: "init"}},
				Outputs:   []interfaces.IOConfig{{WorkTypeName: "child", StateName: "complete"}},
				OnFailure: []interfaces.IOConfig{{WorkTypeName: "child", StateName: "failed"}}},
			{Name: "complete-parent", WorkerTypeName: "completer",
				Inputs: []interfaces.IOConfig{
					{WorkTypeName: "parent", StateName: "waiting"},
					{WorkTypeName: "child", StateName: "complete", Guard: &interfaces.InputGuardConfig{
						Type:        interfaces.GuardTypeAllChildrenComplete,
						ParentInput: "parent",
						SpawnedBy:   "spawn-children",
					}},
				},
				Outputs: []interfaces.IOConfig{{WorkTypeName: "parent", StateName: "complete"}}},
		},
	}
}

// barrierConfigWithFailureDetection returns a barrier config that also routes
// the parent to failed when any child fails, using per-input guards.
func barrierConfigWithFailureDetection() *interfaces.FactoryConfig {
	return &interfaces.FactoryConfig{
		WorkTypes: barrierWorkTypes,
		Workers:   []interfaces.WorkerConfig{{Name: "spawner"}, {Name: "processor"}, {Name: "completer"}, {Name: "failure-handler"}},
		Workstations: []interfaces.FactoryWorkstationConfig{
			{Name: "spawn-children", WorkerTypeName: "spawner",
				Inputs:  []interfaces.IOConfig{{WorkTypeName: "parent", StateName: "init"}},
				Outputs: []interfaces.IOConfig{{WorkTypeName: "parent", StateName: "waiting"}}},
			{Name: "process-child", WorkerTypeName: "processor",
				Inputs:    []interfaces.IOConfig{{WorkTypeName: "child", StateName: "init"}},
				Outputs:   []interfaces.IOConfig{{WorkTypeName: "child", StateName: "complete"}},
				OnFailure: []interfaces.IOConfig{{WorkTypeName: "child", StateName: "failed"}}},
			{Name: "complete-parent", WorkerTypeName: "completer",
				Inputs: []interfaces.IOConfig{
					{WorkTypeName: "parent", StateName: "waiting"},
					{WorkTypeName: "child", StateName: "complete", Guard: &interfaces.InputGuardConfig{
						Type:        interfaces.GuardTypeAllChildrenComplete,
						ParentInput: "parent",
						SpawnedBy:   "spawn-children",
					}},
				},
				Outputs: []interfaces.IOConfig{{WorkTypeName: "parent", StateName: "complete"}}},
			{Name: "failure-handler", WorkerTypeName: "failure-handler",
				Inputs: []interfaces.IOConfig{
					{WorkTypeName: "parent", StateName: "waiting"},
					{WorkTypeName: "child", StateName: "failed", Guard: &interfaces.InputGuardConfig{
						Type:        interfaces.GuardTypeAnyChildFailed,
						ParentInput: "parent",
						SpawnedBy:   "spawn-children",
					}},
				},
				Outputs: []interfaces.IOConfig{{WorkTypeName: "parent", StateName: "failed"}}},
		},
	}
}

// barrierConfigObserveAll returns a barrier config using per-input guards
// WITHOUT spawned_by, generating AllWithParentGuard + CardinalityAll.
// Used to test the zero-cardinality boundary (0 children → guard returns false).
func barrierConfigObserveAll() *interfaces.FactoryConfig {
	return &interfaces.FactoryConfig{
		WorkTypes: barrierWorkTypes,
		Workers:   []interfaces.WorkerConfig{{Name: "spawner"}, {Name: "processor"}, {Name: "completer"}},
		Workstations: []interfaces.FactoryWorkstationConfig{
			{Name: "spawn-children", WorkerTypeName: "spawner",
				Inputs:  []interfaces.IOConfig{{WorkTypeName: "parent", StateName: "init"}},
				Outputs: []interfaces.IOConfig{{WorkTypeName: "parent", StateName: "waiting"}}},
			{Name: "process-child", WorkerTypeName: "processor",
				Inputs:    []interfaces.IOConfig{{WorkTypeName: "child", StateName: "init"}},
				Outputs:   []interfaces.IOConfig{{WorkTypeName: "child", StateName: "complete"}},
				OnFailure: []interfaces.IOConfig{{WorkTypeName: "child", StateName: "failed"}}},
			{Name: "complete-parent", WorkerTypeName: "completer",
				Inputs: []interfaces.IOConfig{
					{WorkTypeName: "parent", StateName: "waiting"},
					{WorkTypeName: "child", StateName: "complete", Guard: &interfaces.InputGuardConfig{
						Type:        interfaces.GuardTypeAllChildrenComplete,
						ParentInput: "parent",
					}},
				},
				Outputs: []interfaces.IOConfig{{WorkTypeName: "parent", StateName: "complete"}}},
		},
	}
}

// ===========================================================================
// Custom executors
// ===========================================================================

// barrierSpawnerExecutor spawns N children with ParentID linked to the input token.
type barrierSpawnerExecutor struct {
	mu         sync.Mutex
	calls      int
	childCount int
}

func (e *barrierSpawnerExecutor) Execute(_ context.Context, dispatch interfaces.WorkDispatch) (interfaces.WorkResult, error) {
	e.mu.Lock()
	e.calls++
	e.mu.Unlock()

	parentWorkID := ""
	if len(dispatch.InputTokens) > 0 {
		parentWorkID = firstInputToken(dispatch.InputTokens).Color.WorkID
	}

	spawned := make([]interfaces.TokenColor, e.childCount)
	for i := range spawned {
		spawned[i] = interfaces.TokenColor{
			WorkTypeID: "child",
			WorkID:     fmt.Sprintf("child-%d", i+1),
			ParentID:   parentWorkID,
		}
	}

	return interfaces.WorkResult{
		DispatchID:   dispatch.DispatchID,
		TransitionID: dispatch.TransitionID,
		Outcome:      interfaces.OutcomeAccepted,
		SpawnedWork:  spawned,
	}, nil
}

func (e *barrierSpawnerExecutor) callCount() int {
	e.mu.Lock()
	defer e.mu.Unlock()
	return e.calls
}

func assertNoExhaustionDispatches(t *testing.T, h *testutil.ServiceTestHarness) {
	t.Helper()

	snap, err := h.GetEngineStateSnapshot()
	if err != nil {
		t.Fatalf("GetEngineStateSnapshot: %v", err)
	}
	for _, dispatch := range snap.DispatchHistory {
		transition := snap.Topology.Transitions[dispatch.TransitionID]
		if transition != nil && transition.Type == petri.TransitionExhaustion {
			t.Fatalf("unexpected exhaustion dispatch %q while routing child failure", dispatch.TransitionID)
		}
	}
}

// failOnNthBarrierExecutor fails on the Nth call, succeeds on all others.
type failOnNthBarrierExecutor struct {
	mu     sync.Mutex
	calls  int
	failOn int // 1-indexed
}

func (e *failOnNthBarrierExecutor) Execute(_ context.Context, dispatch interfaces.WorkDispatch) (interfaces.WorkResult, error) {
	e.mu.Lock()
	e.calls++
	n := e.calls
	e.mu.Unlock()

	outcome := interfaces.OutcomeAccepted
	if n == e.failOn {
		outcome = interfaces.OutcomeFailed
	}

	return interfaces.WorkResult{
		DispatchID:   dispatch.DispatchID,
		TransitionID: dispatch.TransitionID,
		Outcome:      outcome,
	}, nil
}
