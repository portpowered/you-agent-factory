package bufferedpausetests

import (
	"context"

	"github.com/portpowered/infinite-you/pkg/factory"
	"github.com/portpowered/infinite-you/pkg/factory/engine"
	"github.com/portpowered/infinite-you/pkg/factory/requests"
	"github.com/portpowered/infinite-you/pkg/factory/state"
	"github.com/portpowered/infinite-you/pkg/factory/subsystems"
	"github.com/portpowered/infinite-you/pkg/interfaces"
	"github.com/portpowered/infinite-you/pkg/petri"
)

type mockSubsystem struct {
	group     subsystems.TickGroup
	execFn    func(ctx context.Context, snap *interfaces.EngineStateSnapshot[petri.MarkingSnapshot, *state.Net]) (*interfaces.TickResult, error)
	callCount int
}

func (m *mockSubsystem) TickGroup() subsystems.TickGroup { return m.group }

func (m *mockSubsystem) Execute(ctx context.Context, snap *interfaces.EngineStateSnapshot[petri.MarkingSnapshot, *state.Net]) (*interfaces.TickResult, error) {
	m.callCount++
	if m.execFn != nil {
		return m.execFn(ctx, snap)
	}
	return &interfaces.TickResult{}, nil
}

func submitWorkRequests(ctx context.Context, eng *engine.FactoryEngine, reqs []interfaces.SubmitRequest) (interfaces.WorkRequestSubmitResult, error) {
	return eng.SubmitWorkRequest(ctx, requests.WorkRequestFromSubmitRequests(reqs))
}

type testDispatchResultHook struct {
	waitCh  chan struct{}
	results []interfaces.WorkResult
}

func newTestDispatchResultHook() *testDispatchResultHook {
	return &testDispatchResultHook{waitCh: make(chan struct{}, 1)}
}

func (h *testDispatchResultHook) SubmitDispatch(context.Context, interfaces.WorkDispatch) error {
	return nil
}

func (h *testDispatchResultHook) OnTick(context.Context, interfaces.EngineStateSnapshot[petri.MarkingSnapshot, *state.Net]) ([]interfaces.WorkResult, error) {
	if len(h.results) == 0 {
		return nil, nil
	}
	results := make([]interfaces.WorkResult, len(h.results))
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

var _ factory.DispatchResultHook = (*testDispatchResultHook)(nil)

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

func dispatchSubsystem(fn func(context.Context, *interfaces.EngineStateSnapshot[petri.MarkingSnapshot, *state.Net]) (*interfaces.TickResult, error)) *mockSubsystem {
	return &mockSubsystem{group: subsystems.Dispatcher, execFn: fn}
}
