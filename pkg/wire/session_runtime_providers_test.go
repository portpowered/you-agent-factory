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
		writer,
		stores,
		syncWaits,
		sessionIDs,
		responseEventIDs,
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
		)
		if err != nil {
			t.Fatalf("factory(mockWorkers=%#v) error = %v", mockWorkers, err)
		}
		if execution == nil {
			t.Fatalf("factory(mockWorkers=%#v) returned nil execution service", mockWorkers)
		}
	}
}
