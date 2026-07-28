package runtime

import (
	"context"
	"testing"

	"github.com/portpowered/infinite-you/pkg/services/factory_runtime/internal/orchestrators/petri"
	"github.com/portpowered/infinite-you/pkg/services/factory_runtime/internal/services/orchestration/state"
	"github.com/portpowered/infinite-you/pkg/services/work"
	"github.com/portpowered/infinite-you/pkg/services/workers"
	workerexecution "github.com/portpowered/infinite-you/pkg/services/workers"
)

type routeNamesTestExecutor struct{}

func (routeNamesTestExecutor) Execute(context.Context, work.WorkDispatch) (workerexecution.WorkResult, error) {
	return workerexecution.WorkResult{}, nil
}

func TestRuntimeWorkstationRouteNamesIncludeWorkerAndWorkstationKeys(t *testing.T) {
	net := &state.Net{
		Transitions: map[string]*petri.Transition{
			"tr-1": {ID: "tr-1", Name: "review", WorkerType: "swe"},
		},
	}
	names := runtimeWorkstationRouteNames(net, map[string]workers.WorkerExecutor{
		"swe": routeNamesTestExecutor{},
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
