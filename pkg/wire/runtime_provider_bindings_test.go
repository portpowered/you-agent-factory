package wire

import (
	"errors"
	"testing"

	"github.com/portpowered/infinite-you/internal/testutil"
	platformclock "github.com/portpowered/infinite-you/pkg/platform/clock"
	"github.com/portpowered/infinite-you/pkg/platform/logging"
	serviceedges "github.com/portpowered/infinite-you/pkg/services/edges"
	eventswire "github.com/portpowered/infinite-you/pkg/services/events/wire"
	"github.com/portpowered/infinite-you/pkg/services/providers"
	"github.com/portpowered/infinite-you/pkg/services/workers"
)

func TestProvideRuntimeProviderBindingsRebindsInjectedRunner(t *testing.T) {
	runner := testutil.NewProviderCommandRunner()
	adaptedRunner := workers.AdaptCommandRunner(runner)

	registry, service, rebinder, err := provideRuntimeProviderBindings(
		serviceedges.Edges{}, nil, adaptedRunner,
	)
	if err != nil {
		t.Fatalf("provideRuntimeProviderBindings() error = %v", err)
	}
	if registry == nil || service == nil || rebinder == nil {
		t.Fatalf("provideRuntimeProviderBindings() = registry %T, service %T, rebinder %T; want all non-nil", registry, service, rebinder)
	}

	reboundRegistry, reboundService, err := rebinder(adaptedRunner)
	if err != nil {
		t.Fatalf("rebinder() error = %v", err)
	}
	if reboundRegistry == nil || reboundService == nil {
		t.Fatalf("rebinder() = registry %T, service %T; want both non-nil", reboundRegistry, reboundService)
	}
}

func TestWorkerProviderCommandRunnerHonorsInjectedAndDefaultOwnership(t *testing.T) {
	runner := testutil.NewProviderCommandRunner()

	injected, err := provideWorkersProviderCommandRunner(serviceedges.Edges{ProviderCommandRunner: runner})
	if err != nil {
		t.Fatalf("provideWorkersProviderCommandRunner(injected) error = %v", err)
	}
	if injected == nil {
		t.Fatal("provideWorkersProviderCommandRunner(injected) = nil")
	}
	defaultRunner, err := provideWorkersProviderCommandRunner(serviceedges.Edges{})
	if err != nil {
		t.Fatalf("provideWorkersProviderCommandRunner(default) error = %v", err)
	}
	if defaultRunner == nil {
		t.Fatal("provideWorkersProviderCommandRunner(default) = nil")
	}
}

func TestProviderRegistryRebinderUsesTheSessionRunner(t *testing.T) {
	runner := testutil.NewProviderCommandRunner()
	adaptedRunner := workers.AdaptCommandRunner(runner)

	providersService, err := provideProvidersService(serviceedges.Edges{})
	if err != nil {
		t.Fatalf("provideProvidersService() error = %v", err)
	}
	rebinder, err := provideProviderRegistryRebinder(providersService, serviceedges.Edges{})
	if err != nil {
		t.Fatalf("provideProviderRegistryRebinder() error = %v", err)
	}
	if _, _, err := rebinder(nil); err == nil {
		t.Fatal("provider registry rebinder(nil) error = nil")
	}
	if registry, rebound, err := rebinder(adaptedRunner); err != nil || registry == nil || rebound == nil {
		t.Fatalf("provider registry rebinder(valid) = registry %T, service %T, error %v", registry, rebound, err)
	}
}

func TestProviderInvocationEdgeSelectsTheSessionRunner(t *testing.T) {
	runner := testutil.NewProviderCommandRunner()
	adaptedRunner := workers.AdaptCommandRunner(runner)
	providersService, err := provideProvidersService(serviceedges.Edges{})
	if err != nil {
		t.Fatalf("provideProvidersService() error = %v", err)
	}

	if _, gotRunner, err := providerInvocationProviderEdge(nil, nil, adaptedRunner); err != nil || gotRunner != adaptedRunner {
		t.Fatalf("providerInvocationProviderEdge(no session runner) = runner %v, error %v", gotRunner, err)
	}
	if _, _, err := providerInvocationProviderEdge(nil, adaptedRunner, adaptedRunner); err == nil {
		t.Fatal("providerInvocationProviderEdge(missing rebinder) error = nil")
	}
	rebound := func(runner workers.CommandRunner) (workers.ProviderRegistry, providers.Service, error) {
		if runner != adaptedRunner {
			return nil, nil, errors.New("unexpected runner")
		}
		return nil, providersService, nil
	}
	if reboundService, gotRunner, err := providerInvocationProviderEdge(rebound, adaptedRunner, nil); err != nil || reboundService != providersService || gotRunner != adaptedRunner {
		t.Fatalf("providerInvocationProviderEdge(rebound) = service %T, runner %v, error %v", reboundService, gotRunner, err)
	}
}

func TestWorkerInvocationWithProgressFactoryConstructsExecutor(t *testing.T) {
	runner := testutil.NewProviderCommandRunner()
	edges := serviceedges.Edges{}
	providersService, err := provideProvidersService(edges)
	if err != nil {
		t.Fatalf("provideProvidersService() error = %v", err)
	}
	allocator, err := provideAgyPTYAllocator(edges)
	if err != nil {
		t.Fatalf("provideAgyPTYAllocator() error = %v", err)
	}
	factory := provideWorkerInvocationWithProgressFactory(providersService, edges)
	executor, err := factory(workers.AdaptCommandRunner(runner), allocator, nil)
	if err != nil {
		t.Fatalf("worker invocation factory() error = %v", err)
	}
	if executor == nil {
		t.Fatal("worker invocation factory() = nil executor")
	}
}

func TestWorkerProcessCompatibilityCallbacksRemainCallable(t *testing.T) {
	if environment := provideWorkerProcessEnvironment()(); len(environment) == 0 {
		t.Fatal("provideWorkerProcessEnvironment() returned an empty environment")
	}
	if directory, err := provideWorkerCurrentWorkingDirectory()(); err != nil || directory == "" {
		t.Fatalf("provideWorkerCurrentWorkingDirectory() = %q, error %v", directory, err)
	}
}

func TestCanonicalWorkerCompositionBindingsRemainConstructible(t *testing.T) {
	providersService, err := provideProvidersService(serviceedges.Edges{})
	if err != nil {
		t.Fatalf("provideProvidersService() error = %v", err)
	}
	if registry, err := buildProviderRegistry(serviceedges.Edges{}, providersService); err != nil || registry == nil {
		t.Fatalf("buildProviderRegistry() = registry %T, error %v", registry, err)
	}

	overrideFactory := provideProviderInvocationExecutorFactory(
		nil, nil, nil, wireTestProvider{}, workers.AdaptCommandRunner(testutil.NewProviderCommandRunner()),
	)
	executor, err := overrideFactory(nil, nil)
	if err != nil || executor == nil {
		t.Fatalf("provider invocation override factory() = executor %T, error %v", executor, err)
	}

	noConductorFactory := provideProviderInvocationExecutorFactory(nil, nil, nil, nil, nil)
	executor, err = noConductorFactory(nil, nil)
	if err != nil || executor != nil {
		t.Fatalf("provider invocation no-conductor factory() = executor %T, error %v", executor, err)
	}

	boundary := provideWorkstationPoolBoundaryFactory()(workers.WorkstationPoolBoundaryConfig{})
	if boundary == nil {
		t.Fatal("provideWorkstationPoolBoundaryFactory() returned nil boundary")
	}
}

func TestRuntimeRunnerAndWorkerSessionFactoriesUseInjectedPorts(t *testing.T) {
	runner := testutil.NewProviderCommandRunner()
	edges := serviceedges.Edges{
		ProviderCommandRunner: runner,
		ScriptCommandRunner:   runner,
	}
	providerRunner, err := provideFactoryRuntimeProviderCommandRunner(edges)
	if err != nil || providerRunner == nil {
		t.Fatalf("provider runtime command runner = %T, error %v", providerRunner, err)
	}
	scriptRunner, err := provideFactoryRuntimeScriptCommandRunner(edges)
	if err != nil || scriptRunner == nil {
		t.Fatalf("script runtime command runner = %T, error %v", scriptRunner, err)
	}

	eventsService, err := eventswire.NewService()
	if err != nil {
		t.Fatalf("events service = %v", err)
	}
	providerSessions, err := provideProviderSessions(serviceedges.Edges{})
	if err != nil {
		t.Fatalf("provider sessions service = %v", err)
	}
	boundary := provideWorkstationPoolBoundaryFactory()(workers.WorkstationPoolBoundaryConfig{})
	factory := provideWorkerSessionsFactory(eventsService, providerSessions, logging.NoopLogger{}, nil)
	service, err := factory(boundary, platformclock.Real{})
	if err != nil {
		t.Fatalf("worker sessions factory() error = %v", err)
	}
	if service == nil {
		t.Fatal("worker sessions factory() returned nil service")
	}
}
