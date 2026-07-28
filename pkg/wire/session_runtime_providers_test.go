package wire

import (
	"context"
	"testing"

	"github.com/portpowered/infinite-you/internal/testutil"
	platformclock "github.com/portpowered/infinite-you/pkg/platform/clock"
	serviceedges "github.com/portpowered/infinite-you/pkg/services/edges"
	factoryruntime "github.com/portpowered/infinite-you/pkg/services/factory_runtime"
	factorysessions "github.com/portpowered/infinite-you/pkg/services/factory_sessions"
	"github.com/portpowered/infinite-you/pkg/services/workers"
)

type wireTestProvider struct{}

func (wireTestProvider) Infer(context.Context, workers.ProviderInferenceRequest) (workers.InferenceResponse, error) {
	return workers.InferenceResponse{}, nil
}

type wireTestProviderRegistry struct{}

func (wireTestProviderRegistry) UsesNativeRunner(string) bool { return false }

func (wireTestProviderRegistry) RunnerMetadata(string) (workers.RunnerMetadata, error) {
	return workers.RunnerMetadata{}, nil
}

func (wireTestProviderRegistry) ResolveRunnerSelection(string, string, string) (workers.ResolvedRunnerSelection, error) {
	return workers.ResolvedRunnerSelection{}, nil
}

func TestProvideConductorInvocationWithProgressFactory_AcceptsWorkersProviderRegistry(t *testing.T) {
	t.Parallel()

	edges := serviceedges.Edges{
		ProviderCommandRunner: testutil.NewProviderCommandRunner(),
	}
	providersService, err := provideProvidersService(edges)
	if err != nil {
		t.Fatalf("provideProvidersService() error = %v", err)
	}
	registry, err := provideProviderRegistry(edges, providersService)
	if err != nil {
		t.Fatalf("provideProviderRegistry() error = %v", err)
	}
	allocator, err := provideAgyPTYAllocator(edges)
	if err != nil {
		t.Fatalf("provideAgyPTYAllocator() error = %v", err)
	}
	adaptRunner := provideWorkerCommandRunnerAdapter()
	factory := provideConductorInvocationWithProgressFactory(edges)
	executor, err := factory(registry, adaptRunner(edges.ProviderCommandRunner), allocator, nil)
	if err != nil {
		t.Fatalf("factory() error = %v", err)
	}
	if executor == nil {
		t.Fatal("factory() returned nil executor")
	}
}

func TestProvideConductorInvocationWithProgressFactory_RejectsNonConcreteRegistry(t *testing.T) {
	t.Parallel()

	edges := serviceedges.Edges{
		ProviderCommandRunner: testutil.NewProviderCommandRunner(),
	}
	allocator, err := provideAgyPTYAllocator(edges)
	if err != nil {
		t.Fatalf("provideAgyPTYAllocator() error = %v", err)
	}
	adaptRunner := provideWorkerCommandRunnerAdapter()
	factory := provideConductorInvocationWithProgressFactory(edges)
	_, err = factory(wireTestProviderRegistry{}, adaptRunner(edges.ProviderCommandRunner), allocator, nil)
	if err == nil {
		t.Fatal("factory() error = nil, want non-concrete registry rejection")
	}
}

func TestProvideFactorySessionExecutionFactory_BuildsLiveChildInvocation(t *testing.T) {
	t.Parallel()

	edges := serviceedges.Edges{
		ProviderCommandRunner: testutil.NewProviderCommandRunner(),
	}
	providersService, err := provideProvidersService(edges)
	if err != nil {
		t.Fatalf("provideProvidersService() error = %v", err)
	}
	registry, err := provideProviderRegistry(edges, providersService)
	if err != nil {
		t.Fatalf("provideProviderRegistry() error = %v", err)
	}
	registryRebinder, err := provideProviderRegistryRebinder(edges)
	if err != nil {
		t.Fatalf("provideProviderRegistryRebinder() error = %v", err)
	}
	workflowFiles := provideFactoryRuntimeWorkflowSources(edges)
	workflowHome := provideFactoryRuntimeWorkflowHome(edges)
	workflowSymlinks := provideFactoryRuntimeWorkflowSourceResolveSymlinks(edges)
	workflows := provideJavaScriptWorkflows(workflowFiles, workflowHome, workflowSymlinks)
	writer, err := providePortableRecordingWriter(edges)
	if err != nil {
		t.Fatalf("providePortableRecordingWriter() error = %v", err)
	}
	fs := provideFactorySessionRuntimePersistenceFileSystem(edges)
	stores := provideFactorySessionRuntimePersistenceStoreFactory(fs)
	syncWaits := provideFactorySessionSyncWaitScheduler()
	sessionIDs := provideFactorySessionIDGenerator(edges)
	responseEventIDs := provideFactorySessionResponseEventIDGenerator(edges)
	responseEventRetentionLimits := provideFactorySessionResponseEventRetentionLimits(edges)
	invocationWithProgress := provideWorkerInvocationWithProgressFactory(edges)
	allocator, err := provideAgyPTYAllocator(edges)
	if err != nil {
		t.Fatalf("provideAgyPTYAllocator() error = %v", err)
	}
	adaptRunner := provideWorkerCommandRunnerAdapter()
	mockRunnerFactory := provideWorkersMockCommandRunnerFactory()
	conductorInvocation := provideConductorInvocationWithProgressFactory(edges)
	factory := provideFactorySessionExecutionFactory(
		workflows,
		provideOrchestrationJavaScriptExecution(provideFactoryRuntimeIDGenerator(edges), workflows),
		writer,
		stores,
		syncWaits,
		sessionIDs,
		responseEventIDs,
		responseEventRetentionLimits,
		invocationWithProgress,
		allocator,
		adaptRunner,
		registry,
		registryRebinder,
		mockRunnerFactory,
		conductorInvocation,
		edges,
	)

	provider := wireTestProvider{}
	clock := platformclock.Real{}
	for _, mockWorkers := range []*workers.MockWorkersConfig{
		nil,
		workers.NewEmptyMockWorkersConfig(),
	} {
		execution, err := factory(
			t.TempDir(),
			factorysessions.PersistencePolicy(""),
			provider,
			clock,
			nil,
			factoryruntime.JavaScriptWorkerSettings{},
			mockWorkers,
			nil,
		)
		if err != nil {
			t.Fatalf("factory(mockWorkers=%#v) error = %v", mockWorkers, err)
		}
		if execution == nil {
			t.Fatalf("factory(mockWorkers=%#v) returned nil execution service", mockWorkers)
		}
	}
}
