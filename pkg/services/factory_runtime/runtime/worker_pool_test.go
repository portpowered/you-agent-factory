package runtime

import (
	"context"
	"testing"

	"github.com/portpowered/infinite-you/pkg/services/factory_runtime/internal/orchestrators/petri"
	"github.com/portpowered/infinite-you/pkg/services/factory_runtime/state"
	"github.com/portpowered/infinite-you/pkg/services/work"
	"github.com/portpowered/infinite-you/pkg/services/workers"
	workerexecution "github.com/portpowered/infinite-you/pkg/services/workers"
)

// mockExecutor implements WorkerExecutor for testing.
type mockExecutor struct {
	fn func(ctx context.Context, dispatch work.WorkDispatch) (workerexecution.WorkResult, error)
}

func (m *mockExecutor) Execute(ctx context.Context, dispatch work.WorkDispatch) (workerexecution.WorkResult, error) {
	return m.fn(ctx, dispatch)
}

func TestRuntimeWorkersBoundaryRouteNamesIncludeWorkerAndWorkstationKeys(t *testing.T) {
	net := &state.Net{
		Transitions: map[string]*petri.Transition{
			"tr-1": {ID: "tr-1", Name: "review", WorkerType: "swe"},
		},
	}
	names := runtimeWorkersBoundaryRouteNames(net, map[string]workers.WorkerExecutor{
		"swe": &mockExecutor{},
	})
	want := map[string]struct{}{"tr-1": {}, "review": {}, "swe": {}}
	if len(names) != len(want) {
		t.Fatalf("route names = %v, want %v", names, want)
	}
	for _, name := range names {
		if _, ok := want[name]; !ok {
			t.Fatalf("unexpected route name %q in %v", name, names)
		}
	}
}

func TestRuntimeWorkersBoundaryBindingsPreserveLegacyConcurrency(t *testing.T) {
	boundary := newRuntimeWorkersBoundary(
		nil,
		nil,
		map[string]workers.WorkerExecutor{"swe": &mockExecutor{}},
		true,
	)
	if len(boundary.bindings) != 1 {
		t.Fatalf("bindings = %d, want 1", len(boundary.bindings))
	}
	binding := boundary.bindings[0]
	if binding.Capacity != defaultRuntimeBufferSize || binding.QueueCapacity != defaultRuntimeBufferSize {
		t.Fatalf(
			"binding capacity = (%d, %d), want (%d, %d)",
			binding.Capacity,
			binding.QueueCapacity,
			defaultRuntimeBufferSize,
			defaultRuntimeBufferSize,
		)
	}
}
