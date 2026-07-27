package engine

import (
	"context"
	"testing"
	"time"

	interfaces "github.com/portpowered/infinite-you/pkg/services/factory_definitions"
	"github.com/portpowered/infinite-you/pkg/services/factory_runtime/internal/orchestrators/petri"
	"github.com/portpowered/infinite-you/pkg/services/factory_runtime/state"
	"github.com/portpowered/infinite-you/pkg/services/factory_runtime/subsystems"
	"github.com/portpowered/infinite-you/pkg/services/work"
	workerexecution "github.com/portpowered/infinite-you/pkg/services/workers"
)

// mockSubsystem records calls and returns configured results.
type mockSubsystem struct {
	group     subsystems.TickGroup
	execFn    func(ctx context.Context, snap *interfaces.EngineStateSnapshot[petri.MarkingSnapshot, *state.Net]) (*interfaces.TickResult, error)
	callCount int
	lastSnap  *interfaces.EngineStateSnapshot[petri.MarkingSnapshot, *state.Net]
}

func (m *mockSubsystem) TickGroup() subsystems.TickGroup { return m.group }

func (m *mockSubsystem) Execute(ctx context.Context, snap *interfaces.EngineStateSnapshot[petri.MarkingSnapshot, *state.Net]) (*interfaces.TickResult, error) {
	m.callCount++
	m.lastSnap = snap
	if m.execFn != nil {
		return m.execFn(ctx, snap)
	}
	return &interfaces.TickResult{}, nil
}

func submitWorkRequests(ctx context.Context, engine *FactoryEngine, reqs []work.SubmitRequest) (work.WorkRequestSubmitResult, error) {
	return engine.SubmitWorkRequest(ctx, work.WorkRequestFromSubmitRequests(reqs))
}

func waitForEngineRunLoopActive(t *testing.T, engine *FactoryEngine, timeout time.Duration) {
	t.Helper()
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		engine.mu.Lock()
		active := engine.runLoopActive
		engine.mu.Unlock()
		if active {
			return
		}
		time.Sleep(5 * time.Millisecond)
	}
	t.Fatal("timed out waiting for engine run loop to become active")
}

type testSubmissionHook struct {
	name     string
	priority int
	onTick   func(ctx context.Context, input interfaces.SubmissionHookContext[interfaces.EngineStateSnapshot[petri.MarkingSnapshot, *state.Net]]) (interfaces.SubmissionHookResult, error)
}

func (h *testSubmissionHook) Name() string {
	return h.name
}

func (h *testSubmissionHook) Priority() int {
	return h.priority
}

func (h *testSubmissionHook) OnTick(ctx context.Context, input interfaces.SubmissionHookContext[interfaces.EngineStateSnapshot[petri.MarkingSnapshot, *state.Net]]) (interfaces.SubmissionHookResult, error) {
	if h.onTick != nil {
		return h.onTick(ctx, input)
	}
	return interfaces.SubmissionHookResult{}, nil
}

type testDispatchResultHook struct {
	waitCh   chan struct{}
	submit   func(context.Context, work.WorkDispatch) error
	onTick   func(context.Context, interfaces.EngineStateSnapshot[petri.MarkingSnapshot, *state.Net]) ([]workerexecution.WorkResult, error)
	submits  []work.WorkDispatch
	results  []workerexecution.WorkResult
	waitOnce bool
}

func newTestDispatchResultHook() *testDispatchResultHook {
	return &testDispatchResultHook{waitCh: make(chan struct{}, 1)}
}

func (h *testDispatchResultHook) SubmitDispatch(ctx context.Context, dispatch work.WorkDispatch) error {
	h.submits = append(h.submits, dispatch)
	if h.submit != nil {
		return h.submit(ctx, dispatch)
	}
	return nil
}

func (h *testDispatchResultHook) OnTick(ctx context.Context, input interfaces.EngineStateSnapshot[petri.MarkingSnapshot, *state.Net]) ([]workerexecution.WorkResult, error) {
	if h.onTick != nil {
		return h.onTick(ctx, input)
	}
	if len(h.results) == 0 {
		return nil, nil
	}
	results := make([]workerexecution.WorkResult, len(h.results))
	copy(results, h.results)
	h.results = nil
	return results, nil
}

func (h *testDispatchResultHook) WaitCh() <-chan struct{} {
	return h.waitCh
}

func (h *testDispatchResultHook) HasPendingResults() bool {
	return len(h.results) > 0
}

func (h *testDispatchResultHook) HasBufferedResults() bool {
	return h.HasPendingResults()
}

func (h *testDispatchResultHook) SignalBufferedResults() {
	select {
	case h.waitCh <- struct{}{}:
	default:
	}
}

// buildTestNet creates a minimal net with one work type (init -> complete -> failed).
func buildTestNet() *state.Net {
	wt := &state.WorkType{
		ID:   "task",
		Name: "Task",
		States: []state.StateDefinition{
			{Value: "init", Category: state.StateCategoryInitial},
			{Value: "complete", Category: state.StateCategoryTerminal},
			{Value: "failed", Category: state.StateCategoryFailed},
		},
	}
	places := make(map[string]*petri.Place)
	for _, p := range wt.GeneratePlaces() {
		places[p.ID] = p
	}
	return &state.Net{
		ID:          "test-net",
		Places:      places,
		Transitions: make(map[string]*petri.Transition),
		WorkTypes:   map[string]*state.WorkType{"task": wt},
		Resources:   make(map[string]*state.ResourceDef),
	}
}
