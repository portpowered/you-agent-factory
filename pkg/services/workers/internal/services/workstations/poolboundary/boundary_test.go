package poolboundary

import (
	"context"
	"testing"

	"github.com/portpowered/infinite-you/pkg/services/work"
	"github.com/portpowered/infinite-you/pkg/services/workers"
)

type poolBoundaryTestExecutor struct{}

func (poolBoundaryTestExecutor) Execute(context.Context, work.WorkDispatch) (workers.WorkResult, error) {
	return workers.WorkResult{}, nil
}

func TestWorkstationPoolBoundaryBindingsPreserveLegacyConcurrency(t *testing.T) {
	boundary := NewWorkstationPoolBoundary(WorkstationPoolBoundaryConfig{
		Executors:  map[string]workers.WorkerExecutor{"swe": poolBoundaryTestExecutor{}},
		RouteNames: []string{"swe"},
		Async:      true,
	})
	pool, ok := boundary.(*workstationPoolBoundary)
	if !ok {
		t.Fatalf("boundary type = %T, want *workstationPoolBoundary", boundary)
	}
	if len(pool.bindings) != 1 {
		t.Fatalf("bindings = %d, want 1", len(pool.bindings))
	}
	binding := pool.bindings[0]
	if binding.Capacity != DefaultRuntimePoolBindingCapacity ||
		binding.QueueCapacity != DefaultRuntimePoolBindingCapacity {
		t.Fatalf(
			"binding capacity = (%d, %d), want (%d, %d)",
			binding.Capacity,
			binding.QueueCapacity,
			DefaultRuntimePoolBindingCapacity,
			DefaultRuntimePoolBindingCapacity,
		)
	}
}
