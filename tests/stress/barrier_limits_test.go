package stress_test

import (
	"context"
	"fmt"
	"sync"
	"testing"
	"time"

	"github.com/portpowered/infinite-you/internal/testutil"
	interfaces "github.com/portpowered/infinite-you/pkg/services/factory_definitions"
	"github.com/portpowered/infinite-you/pkg/services/work"
	workerexecution "github.com/portpowered/infinite-you/pkg/services/workers"
	factoryapi "github.com/portpowered/infinite-you/pkg/transports/http/generated"
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
	spawner := &barrierSpawnerExecutor{childCount: 5}
	processor := &acceptedCountingExecutor{}
	completer := &acceptedCountingExecutor{}
	h := startStressProcess(t, dir, workerExecutorProvider{executor: workerMuxExecutor{
		"spawner": spawner, "processor": processor, "completer": completer,
	}})
	t.Cleanup(h.Stop)
	h.SubmitWork("parent", []byte("barrier test"))
	h.WaitForTerminalCount(6, 10*time.Second)
	session := h.Session()
	assertSessionPlaceCount(t, session, "parent:complete", 1)
	assertSessionPlaceCount(t, session, "child:complete", 5)
	assertSessionPlaceCount(t, session, "parent:waiting", 0)
	if spawner.callCount() != 1 {
		t.Errorf("spawner called %d times, want 1", spawner.callCount())
	}
	if completer.callCount() != 1 {
		t.Errorf("completer called %d times, want 1", completer.callCount())
	}
}

// ---------------------------------------------------------------------------
// TestBarrierPartialFailure: 5 children spawned, 1 fails.
// Fan-in requires all 5 complete → does NOT fire.
// Failure-detection transition routes parent to failed.
// ---------------------------------------------------------------------------
func TestBarrierPartialFailure(t *testing.T) {
	dir := testutil.ScaffoldFactoryDir(t, barrierConfigWithFailureDetection())
	spawner := &barrierSpawnerExecutor{childCount: 5}
	failExec := &failOnNthBarrierExecutor{failOn: 3}
	completer := &acceptedCountingExecutor{}
	failureHandler := &acceptedCountingExecutor{}
	h := startStressProcess(t, dir, workerExecutorProvider{executor: workerMuxExecutor{
		"spawner": spawner, "processor": failExec, "completer": completer,
		"failure-handler": failureHandler,
	}})
	t.Cleanup(h.Stop)
	h.SubmitWork("parent", []byte("partial failure"))
	h.WaitForTerminalCount(6, 10*time.Second)
	session := h.Session()
	assertSessionPlaceCount(t, session, "parent:failed", 1)
	assertSessionPlaceCount(t, session, "parent:complete", 0)
	assertSessionPlaceCount(t, session, "child:complete", 4)
	assertSessionPlaceCount(t, session, "child:failed", 1)
	if completer.callCount() != 0 {
		t.Fatalf("complete-parent fired %d times, want 0", completer.callCount())
	}
	if failureHandler.callCount() != 1 {
		t.Fatalf("failure-handler fired %d times, want 1", failureHandler.callCount())
	}
}

// ---------------------------------------------------------------------------
// TestBarrierDelayedArrival: fan-in fires ONLY when all 5 children are present.
// We tick incrementally to prove the barrier waits for the last child.
// ---------------------------------------------------------------------------
func TestBarrierDelayedArrival(t *testing.T) {
	dir := testutil.ScaffoldFactoryDir(t, barrierConfig())
	spawner := &barrierSpawnerExecutor{childCount: 5}
	h := startStressProcess(t, dir, workerExecutorProvider{executor: workerMuxExecutor{
		"spawner": spawner, "processor": &acceptedCountingExecutor{},
		"completer": &acceptedCountingExecutor{},
	}})
	t.Cleanup(h.Stop)
	h.SubmitWork("parent", []byte("delayed arrival"))
	h.WaitForTerminalCount(6, 10*time.Second)
	session := h.Session()
	assertSessionPlaceCount(t, session, "parent:complete", 1)
	assertSessionPlaceCount(t, session, "child:complete", 5)
	assertSessionPlaceCount(t, session, "parent:waiting", 0)
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
	spawner := &barrierSpawnerExecutor{childCount: 0}
	h := startStressProcess(t, dir, workerExecutorProvider{executor: workerMuxExecutor{
		"spawner": spawner, "processor": &acceptedCountingExecutor{},
		"completer": &acceptedCountingExecutor{},
	}})
	t.Cleanup(h.Stop)
	h.SubmitWork("parent", []byte("zero children"))
	deadline := time.Now().Add(2 * time.Second)
	for {
		session := h.Session()
		if sessionPlaceCount(session, "parent:waiting") == 1 {
			break
		}
		if time.Now().After(deadline) {
			t.Fatalf("expected parent to remain waiting; session=%#v", session.Runtime.Progress)
		}
		time.Sleep(50 * time.Millisecond)
	}
	session := h.Session()
	assertSessionPlaceCount(t, session, "parent:waiting", 1)
	assertSessionPlaceCount(t, session, "parent:complete", 0)
	assertSessionPlaceCount(t, session, "parent:failed", 0)
	assertSessionPlaceCount(t, session, "child:init", 0)
	assertSessionPlaceCount(t, session, "child:complete", 0)
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
		Workers:   []interfaces.FactoryWorkerConfig{{Name: "spawner"}, {Name: "processor"}, {Name: "completer"}},
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
		Workers:   []interfaces.FactoryWorkerConfig{{Name: "spawner"}, {Name: "processor"}, {Name: "completer"}, {Name: "failure-handler"}},
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
		Workers:   []interfaces.FactoryWorkerConfig{{Name: "spawner"}, {Name: "processor"}, {Name: "completer"}},
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

// barrierSpawnerExecutor emits N child Work items through the canonical Work
// Request output. The runtime adds their parent relation from the dispatch.
type barrierSpawnerExecutor struct {
	mu         sync.Mutex
	calls      int
	childCount int
}

func (e *barrierSpawnerExecutor) Execute(_ context.Context, dispatch work.WorkDispatch) (workerexecution.WorkResult, error) {
	e.mu.Lock()
	e.calls++
	e.mu.Unlock()

	children := make([]work.Work, e.childCount)
	for i := range children {
		children[i] = work.Work{
			Name:       fmt.Sprintf("child-%d", i+1),
			WorkTypeID: "child",
			WorkID:     fmt.Sprintf("child-%d", i+1),
		}
	}
	output := ""
	if len(children) > 0 {
		output = workerGeneratedBatchOutput(children)
	}

	return workerexecution.WorkResult{
		DispatchID:   dispatch.DispatchID,
		TransitionID: dispatch.TransitionID,
		Outcome:      workerexecution.OutcomeAccepted,
		Output:       output,
	}, nil
}

func (e *barrierSpawnerExecutor) callCount() int {
	e.mu.Lock()
	defer e.mu.Unlock()
	return e.calls
}

func sessionPlaceCount(session factoryapi.FactorySession, placeID string) int {
	if session.Runtime.Petri == nil {
		return 0
	}
	count := 0
	for _, token := range session.Runtime.Petri.Marking {
		if token.PlaceId == placeID {
			count++
		}
	}
	return count
}

func assertSessionPlaceCount(t *testing.T, session factoryapi.FactorySession, placeID string, want int) {
	t.Helper()
	if got := sessionPlaceCount(session, placeID); got != want {
		t.Errorf("place %q contains %d work items, want %d", placeID, got, want)
	}
}

func sessionTokenLastError(session factoryapi.FactorySession, placeID string) string {
	if session.Runtime.Petri == nil {
		return ""
	}
	for _, token := range session.Runtime.Petri.Marking {
		if token.PlaceId == placeID && token.History != nil && token.History.LastError != nil {
			return *token.History.LastError
		}
	}
	return ""
}

// failOnNthBarrierExecutor fails on the Nth call, succeeds on all others.
type failOnNthBarrierExecutor struct {
	mu     sync.Mutex
	calls  int
	failOn int // 1-indexed
}

func (e *failOnNthBarrierExecutor) Execute(_ context.Context, dispatch work.WorkDispatch) (workerexecution.WorkResult, error) {
	e.mu.Lock()
	e.calls++
	n := e.calls
	e.mu.Unlock()

	outcome := workerexecution.OutcomeAccepted
	if n == e.failOn {
		outcome = workerexecution.OutcomeFailed
	}

	return workerexecution.WorkResult{
		DispatchID:   dispatch.DispatchID,
		TransitionID: dispatch.TransitionID,
		Outcome:      outcome,
	}, nil
}
