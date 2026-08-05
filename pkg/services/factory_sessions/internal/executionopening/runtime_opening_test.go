package executionopening

import (
	"context"
	"testing"

	factoryruntime "github.com/portpowered/infinite-you/pkg/services/factory_runtime"
	factorysessions "github.com/portpowered/infinite-you/pkg/services/factory_sessions"
	"github.com/portpowered/infinite-you/pkg/services/factory_sessions/internal/roles"
	"github.com/portpowered/infinite-you/pkg/services/factory_sessions/internal/runtimeopening"
	durableexecution "github.com/portpowered/infinite-you/pkg/services/factory_sessions/internal/services/durable_execution"
	"go.uber.org/zap"
)

// TestExecutionOpeningUsesItsNarrowRuntimeView proves runtime-backed direct
// execution opens exactly one execution view through the grouped Factory
// Sessions capability and preserves its selected runtime inputs.
func TestExecutionOpeningUsesItsNarrowRuntimeView(t *testing.T) {
	t.Parallel()

	wantExecution := &runtimeOpeningExecutionStub{}
	opening := &executionRuntimeOpeningStub{
		opened: roles.OpenedExecutionRuntime{Execution: wantExecution},
	}
	factory := &Factory{
		runtimes: opening,
		artifactRoots: func(home string) factoryruntime.RuntimeArtifactRoots {
			if home != "home" {
				t.Fatalf("artifact roots home = %q, want home", home)
			}
			return factoryruntime.RuntimeArtifactRoots{Logs: "logs", Metrics: "metrics"}
		},
		logger: zap.NewNop(),
	}

	opened, err := factory.OpenExecutionRuntime(t.Context(), factorysessions.ExecutionRuntimeOpeningRequest{
		ProjectRoot: "project", SystemConfigHome: "home",
	})
	if err != nil {
		t.Fatalf("OpenExecutionRuntime: %v", err)
	}
	if opened.Execution != wantExecution {
		t.Fatalf("opened execution = %#v, want injected execution", opened.Execution)
	}
	if opening.calls != 1 {
		t.Fatalf("execution runtime openings = %d, want 1", opening.calls)
	}
	if opening.request == nil || opening.request.FactoryDefinition.Directory != "project" ||
		opening.request.FactoryDefinition.ExecutionBaseDir != "project" ||
		opening.request.FactorySession.SystemConfigHome != "home" ||
		opening.request.FactoryRuntime.LogDirectory != "logs" ||
		opening.request.FactoryRuntime.MetricsDirectory != "metrics" {
		t.Fatalf("execution runtime request = %#v", opening.request)
	}
}

type executionRuntimeOpeningStub struct {
	calls   int
	request *factorysessions.RuntimeOpeningRequest
	opened  roles.OpenedExecutionRuntime
}

func (stub *executionRuntimeOpeningStub) OpenExecutionRuntime(
	_ context.Context,
	request *factorysessions.RuntimeOpeningRequest,
	_ runtimeopening.ExternalEffects,
	_ *zap.Logger,
) (roles.OpenedExecutionRuntime, error) {
	stub.calls++
	stub.request = request
	return stub.opened, nil
}

type runtimeOpeningExecutionStub struct{ durableexecution.Service }
