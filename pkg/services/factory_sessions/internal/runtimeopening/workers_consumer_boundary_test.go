package runtimeopening

import (
	"context"
	"testing"
	"time"

	factoryruntime "github.com/portpowered/infinite-you/pkg/services/factory_runtime"
	"github.com/portpowered/infinite-you/pkg/services/factory_sessions/internal/roles"
	"github.com/portpowered/infinite-you/pkg/services/models"
	operatorconfig "github.com/portpowered/infinite-you/pkg/services/operator_settings"
	"github.com/portpowered/infinite-you/pkg/services/work"
	"github.com/portpowered/infinite-you/pkg/services/workers"
	"go.uber.org/zap"
)

const workersImportRoot = "github.com/portpowered/infinite-you/pkg/services/workers"

// TestRuntimeOpeningPackagesImportWorkersOnlyThroughRoot seals runtime-opening
// and construction call sites to the Workers service root contract.

// TestRuntimeOpeningFactoryRolesNameWorkersRootContracts proves runtime-opening
// construction helpers type Workers-facing bindings only through the Workers
// service root.
func TestRuntimeOpeningFactoryRolesNameWorkersRootContracts(t *testing.T) {
	t.Parallel()

	var (
		_ FactorySessionExecutionFactory
		_ ConductorInvocationWithProgressFactory
		_ WorkersRuntimeFactory
		_ WorkerExecutionFactory
		_ DurableExecutionFactory
		_ ProviderFromCommandRunnerFactory
	)

	var _ ExternalEffects = ExternalEffects{
		ProviderOverride:     (workers.Provider)(nil),
		HostedClock:          (workers.HostedPollerClock)(nil),
		HostedHTTPClient:     (workers.HostedPollerHTTPDoer)(nil),
		HostedSecretResolver: (workers.HostedPollerSecretResolver)(nil),
	}
}

type workersRootBindingProbeRunner struct{}

func (workersRootBindingProbeRunner) Run(
	context.Context,
	workers.CommandRequest,
) (workers.CommandResult, error) {
	return workers.CommandResult{}, nil
}

func TestNewWorkerExecutionForwardsWorkersRootBindings(t *testing.T) {
	t.Parallel()

	const runnerID = "session-worker-runner"
	ptyAllocator := &workers.MockPTYAllocator{}
	var gotRunnerID string
	var gotPTY workers.PTYAllocator
	var gotProvider workers.Provider

	workersRuntimeFactory := func(
		_ roles.CurrentRuntimeResolver,
		_ models.Service,
		_ models.RuntimeScopeRef,
		_ workers.CommandRunner,
		_ workers.CommandRunner,
		pty workers.PTYAllocator,
		_ *zap.Logger,
		_ bool,
		runner string,
		_ *bool,
		provider workers.Provider,
		_ func() time.Time,
		_ work.ContentMaterializer,
		_ []operatorconfig.ACPIntegration,
	) (workers.RuntimeService, error) {
		gotRunnerID = runner
		gotPTY = pty
		gotProvider = provider
		return nil, nil
	}

	service, err := NewWorkerExecution(
		factoryruntime.RuntimeOpeningRequest{},
		workers.RuntimeOpeningRequest{RunnerID: runnerID},
		openingCoordinatorClock{},
		zap.NewNop(),
		workersRootBindingProbeRunner{},
		workersRootBindingProbeRunner{},
		ptyAllocator,
		nil,
		nil,
		nil,
		models.RuntimeScopeRef{},
		work.MaterializationService(materializerStub{}),
		workersRuntimeFactory,
		nil,
	)
	if err != nil {
		t.Fatalf("NewWorkerExecution() error = %v, want nil", err)
	}
	if service != nil {
		t.Fatalf("service = %#v, want nil placeholder", service)
	}
	if gotRunnerID != runnerID {
		t.Fatalf("runner ID = %q, want %q", gotRunnerID, runnerID)
	}
	if gotPTY != ptyAllocator {
		t.Fatalf("PTY allocator = %#v, want %#v", gotPTY, ptyAllocator)
	}
	if gotProvider != nil {
		t.Fatalf("provider = %#v, want nil override", gotProvider)
	}
}
