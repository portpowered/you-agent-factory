package wire

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/portpowered/infinite-you/internal/testutil"
	platformclock "github.com/portpowered/infinite-you/pkg/platform/clock"
	platformprocess "github.com/portpowered/infinite-you/pkg/platform/process"
	serviceedges "github.com/portpowered/infinite-you/pkg/services/edges"
	factoryruntime "github.com/portpowered/infinite-you/pkg/services/factory_runtime"
	factorysessions "github.com/portpowered/infinite-you/pkg/services/factory_sessions"
	"github.com/portpowered/infinite-you/pkg/services/work"
	"github.com/portpowered/infinite-you/pkg/services/workers"
)

func codexWireTestOutput(content string) []byte {
	item, _ := json.Marshal(map[string]any{"type": "item.completed", "item": map[string]any{"id": "message", "type": "agent_message", "text": content}})
	return []byte("{\"type\":\"turn.started\"}\n" + string(item) + "\n{\"type\":\"turn.completed\",\"usage\":{\"input_tokens\":1,\"output_tokens\":1}}\n")
}

type wireTestProvider struct{}

func (wireTestProvider) Infer(context.Context, workers.ProviderInferenceRequest) (workers.InferenceResponse, error) {
	return workers.InferenceResponse{}, nil
}

type wireTestProviderRegistry struct{}

func (wireTestProviderRegistry) UsesNativeRunner(string) bool { return false }
func (wireTestProviderRegistry) CanonicalIdentity(identity string) (string, error) {
	return identity, nil
}
func (wireTestProviderRegistry) RunnerIdentities() []string { return nil }

func (wireTestProviderRegistry) RunnerMetadata(string) (workers.RunnerMetadata, error) {
	return workers.RunnerMetadata{}, nil
}

func (wireTestProviderRegistry) ResolveRunnerSelection(string, string, string) (workers.ResolvedRunnerSelection, error) {
	return workers.ResolvedRunnerSelection{}, nil
}
func (wireTestProviderRegistry) ValidateRunnerPrerequisites(platformprocess.ExecutableLocator, string) error {
	return nil
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
	factory := provideConductorInvocationWithProgressFactory(providersService, edges)
	executor, err := factory(registry, adaptRunner(edges.ProviderCommandRunner), allocator, nil)
	if err != nil {
		t.Fatalf("factory() error = %v", err)
	}
	if executor == nil {
		t.Fatal("factory() returned nil executor")
	}
}

func TestProvideConductorInvocationWithProgressFactory_ExecutesCodexThroughInjectedRunner(t *testing.T) {
	runner := testutil.NewProviderCommandRunner(platformprocess.CommandResult{Stdout: codexWireTestOutput("child result")})
	edges := serviceedges.Edges{ProviderCommandRunner: runner}
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
	executor, err := provideConductorInvocationWithProgressFactory(providersService, edges)(
		registry, provideWorkerCommandRunnerAdapter()(runner), allocator, nil,
	)
	if err != nil {
		t.Fatalf("construct invocation executor: %v", err)
	}
	result, err := executor.Execute(t.Context(), workers.InvocationInput{Request: workers.ProviderInferenceRequest{
		Dispatch: work.WorkDispatch{DispatchID: "wire-codex-child"},
		RunnerID: "codex", ModelProvider: "codex", Model: "gpt-5", UserMessage: "do work",
	}})
	if err != nil {
		t.Fatalf("execute Codex child: %v; result=%#v", err, result)
	}
	if runner.CallCount() != 1 || result.Response.Content != "child result" {
		t.Fatalf("runner calls=%d result=%#v", runner.CallCount(), result)
	}
}

func TestProvideConductorInvocationWithProgressFactory_AcceptsRootRegistry(t *testing.T) {
	t.Parallel()

	edges := serviceedges.Edges{
		ProviderCommandRunner: testutil.NewProviderCommandRunner(),
	}
	providersService, err := provideProvidersService(edges)
	if err != nil {
		t.Fatalf("provideProvidersService() error = %v", err)
	}
	allocator, err := provideAgyPTYAllocator(edges)
	if err != nil {
		t.Fatalf("provideAgyPTYAllocator() error = %v", err)
	}
	adaptRunner := provideWorkerCommandRunnerAdapter()
	factory := provideConductorInvocationWithProgressFactory(providersService, edges)
	if _, err = factory(wireTestProviderRegistry{}, adaptRunner(edges.ProviderCommandRunner), allocator, nil); err != nil {
		t.Fatalf("factory() error = %v", err)
	}
}

func TestProvideFactorySessionExecutionFactory_BuildsLiveChildInvocation(t *testing.T) {
	t.Parallel()

	edges := serviceedges.Edges{}
	providersService, err := provideProvidersService(edges)
	if err != nil {
		t.Fatalf("provideProvidersService() error = %v", err)
	}
	registry, err := provideProviderRegistry(edges, providersService)
	if err != nil {
		t.Fatalf("provideProviderRegistry() error = %v", err)
	}
	registryRebinder, err := provideProviderRegistryRebinder(providersService)
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
	invocationWithProgress := provideWorkerInvocationWithProgressFactory(providersService, edges)
	allocator, err := provideAgyPTYAllocator(edges)
	if err != nil {
		t.Fatalf("provideAgyPTYAllocator() error = %v", err)
	}
	adaptRunner := provideWorkerCommandRunnerAdapter()
	mockRunnerFactory := provideWorkersMockCommandRunnerFactory()
	conductorInvocation := provideConductorInvocationWithProgressFactory(providersService, edges)
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
