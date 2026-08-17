package executionopening

import (
	"context"
	"testing"

	factory "github.com/portpowered/infinite-you/pkg/services/factory_runtime"
	factorysessions "github.com/portpowered/infinite-you/pkg/services/factory_sessions"
	durableexecution "github.com/portpowered/infinite-you/pkg/services/factory_sessions/internal/services/durable_execution"
	"github.com/portpowered/infinite-you/pkg/services/workers"
)

const workersImportRoot = "github.com/portpowered/infinite-you/pkg/services/workers"

// TestExecutionOpeningPackagesImportWorkersOnlyThroughRoot seals execution-opening
// and durable-provider construction call sites to the Workers service root contract.

// TestExecutionOpeningFactoryRolesNameWorkersRootContracts proves execution-opening
// construction helpers type Workers-facing bindings only through the Workers
// service root.
func TestExecutionOpeningFactoryRolesNameWorkersRootContracts(t *testing.T) {
	t.Parallel()

	var (
		_ StandaloneSessionExecutionFactory
		_ WorkerInvocationWithProgressFactory
	)

	var _ *Factory = &Factory{workerExecution: workersRootExecutionProbe{}}
}

type workersRootExecutionProbe struct{}

func (workersRootExecutionProbe) Execute(
	context.Context,
	workers.ExecuteRequest,
) (workers.ExecuteResult, error) {
	return workers.ExecuteResult{}, nil
}

func TestBuildWithWorkerEffectsForwardsWorkersRootExecutionBinding(t *testing.T) {
	t.Parallel()

	workerExecution := workersRootExecutionProbe{}

	factory := &Factory{
		workerExecution: workerExecution,
		standalone: func(
			_ factorysessions.ExecutionProvider,
			_ string,
			_ string,
			_ string,
			execution WorkerExecution,
			_ factory.Clock,
		) (durableexecution.Service, error) {
			if execution != workerExecution {
				t.Fatalf("Workers execution = %#v, want %#v", execution, workerExecution)
			}
			return nil, nil
		},
		resolveClock: func(factory.Clock) factory.Clock { return nil },
	}

	_, err := factory.buildWithWorkerEffects(
		context.Background(),
		string(factorysessions.ExecutionProviderJavaScriptRuntime),
		t.TempDir(),
		"",
		factorysessions.ChildExecutorModeLive,
	)
	if err != nil {
		t.Fatalf("buildWithWorkerEffects() error = %v, want nil", err)
	}
}
