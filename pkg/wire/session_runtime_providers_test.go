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

func TestProvideFactorySessionExecutionFactory_RespectsMockWorkersGate(t *testing.T) {
	t.Parallel()

	edges := serviceedges.Edges{
		ProviderCommandRunner: testutil.NewProviderCommandRunner(),
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
	invocation := provideWorkerInvocationFactory(edges)
	invocationWithProgress := provideWorkerInvocationWithProgressFactory(edges)
	allocator, err := provideAgyPTYAllocator(edges)
	if err != nil {
		t.Fatalf("provideAgyPTYAllocator() error = %v", err)
	}
	adaptRunner := provideWorkerCommandRunnerAdapter()
	factory := provideFactorySessionExecutionFactory(
		workflows,
		writer,
		stores,
		syncWaits,
		sessionIDs,
		responseEventIDs,
		invocation,
		invocationWithProgress,
		allocator,
		adaptRunner,
		edges,
	)

	provider := wireTestProvider{}
	clock := platformclock.Real{}
	for _, mockWorkersEnabled := range []bool{true, false} {
		execution, err := factory(
			t.TempDir(),
			factorysessions.PersistencePolicy(""),
			provider,
			clock,
			nil,
			factoryruntime.JavaScriptWorkerSettings{},
			mockWorkersEnabled,
		)
		if err != nil {
			t.Fatalf("factory(mockWorkersEnabled=%t) error = %v", mockWorkersEnabled, err)
		}
		if execution == nil {
			t.Fatalf("factory(mockWorkersEnabled=%t) returned nil execution service", mockWorkersEnabled)
		}
	}
}
