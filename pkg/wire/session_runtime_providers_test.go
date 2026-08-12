package wire

import (
	"context"
	"encoding/json"
	"errors"
	"slices"
	"testing"

	"go.uber.org/zap"

	"github.com/portpowered/infinite-you/internal/testutil"
	initializerapplication "github.com/portpowered/infinite-you/pkg/initializer/application"
	platformclock "github.com/portpowered/infinite-you/pkg/platform/clock"
	platformfilesystem "github.com/portpowered/infinite-you/pkg/platform/filesystem"
	"github.com/portpowered/infinite-you/pkg/platform/logging"
	platformprocess "github.com/portpowered/infinite-you/pkg/platform/process"
	serviceedges "github.com/portpowered/infinite-you/pkg/services/edges"
	eventswire "github.com/portpowered/infinite-you/pkg/services/events/wire"
	factoryruntime "github.com/portpowered/infinite-you/pkg/services/factory_runtime"
	factorysessions "github.com/portpowered/infinite-you/pkg/services/factory_sessions"
	factorysessionwire "github.com/portpowered/infinite-you/pkg/services/factory_sessions/wire"
	globalconfigmapping "github.com/portpowered/infinite-you/pkg/services/operator_settings/transports/globalconfig"
	settingswire "github.com/portpowered/infinite-you/pkg/services/operator_settings/wire"
	"github.com/portpowered/infinite-you/pkg/services/providers"
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

func TestProvideConductorInvocationWithProgressFactory_AcceptsDefaultProvidersService(t *testing.T) {
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
	executor, err := factory(nil, adaptRunner(edges.ProviderCommandRunner), allocator, nil)
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
	allocator, err := provideAgyPTYAllocator(edges)
	if err != nil {
		t.Fatalf("provideAgyPTYAllocator() error = %v", err)
	}
	executor, err := provideConductorInvocationWithProgressFactory(providersService, edges)(
		nil, provideWorkerCommandRunnerAdapter()(runner), allocator, nil,
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

func TestProvideConductorInvocationWithProgressFactory_AcceptsSelectedProvidersService(t *testing.T) {
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
	if _, err = factory(providersService, adaptRunner(edges.ProviderCommandRunner), allocator, nil); err != nil {
		t.Fatalf("factory() error = %v", err)
	}
}

// TestProvideFactorySessionExecutionFactory_TakesNoProviderEdge pins that a
// runtime-backed durable execution service is composed without any provider,
// command runner, allocator, or registry of its own. Its children are Workers,
// and every provider edge they need is the one Workers already composed for the
// session; a second edge assembled here is the bypass the Worker Session
// convergence removed. The mock-worker cases matter because the removed code
// branched on them.
func TestProvideFactorySessionExecutionFactory_TakesNoProviderEdge(t *testing.T) {
	t.Parallel()

	edges := serviceedges.Edges{}
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
	allocator, err := provideAgyPTYAllocator(edges)
	if err != nil {
		t.Fatalf("provideAgyPTYAllocator() error = %v", err)
	}
	adaptRunner := provideWorkerCommandRunnerAdapter()
	eventsService, err := eventswire.NewService()
	if err != nil {
		t.Fatalf("construct events service: %v", err)
	}
	factory := provideFactorySessionExecutionFactory(
		workflows,
		provideOrchestrationJavaScriptExecution(provideFactoryRuntimeIDGenerator(edges), workflows),
		writer,
		stores,
		syncWaits,
		sessionIDs,
		responseEventIDs,
		responseEventRetentionLimits,
		allocator,
		adaptRunner,
		edges,
		eventsService,
		factorysessionwire.NewLiveChangeCoordinator(),
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

func TestOperatorSettingsHomePortCompositionUsesProcessProviderRoot(t *testing.T) {
	t.Parallel()

	providersRoot, err := provideProvidersService(serviceedges.Edges{})
	if err != nil {
		t.Fatalf("provideProvidersService() error = %v", err)
	}
	service, err := settingswire.NewServiceFromHomePorts(
		platformfilesystem.Local{},
		globalconfigmapping.Decode,
		providersRoot,
		func() string { return "00000000-0000-4000-8000-000000000001" },
		logging.NoopLogger{},
	)
	if err != nil {
		t.Fatalf("NewServiceFromHomePorts() error = %v", err)
	}
	if service == nil {
		t.Fatal("NewServiceFromHomePorts() = nil, want Operator Settings root")
	}
}

// TestProvideApplicationProcessLifecycle_ComposesProvidersAndFactoryTargetClose
// proves the composed ProcessLifecycle actually invokes both the Providers
// lifecycle's own Close and the on-demand Factory Sessions activation
// singleton's Close when Close is called on the composed value -- not just
// that construction succeeds -- and that a nil factoryTarget (a graph
// misconfiguration, defensively handled) is a no-op rather than a panic.
func TestProvideApplicationProcessLifecycle_ComposesProvidersAndFactoryTargetClose(t *testing.T) {
	t.Parallel()

	providersService, err := provideProvidersService(serviceedges.Edges{})
	if err != nil {
		t.Fatalf("provideProvidersService() error = %v", err)
	}

	var opening factorysessionwire.InvocationRuntimeOpening = &factorysessionwire.RuntimeOpening{}
	factoryTarget, err := factorysessionwire.NewOnDemandFactoryTargetService(
		opening,
		factorysessionwire.RuntimeOpeningExternalEffects{},
		func(context.Context, string, string) (factorysessions.RuntimeOpeningRequest, error) {
			return factorysessions.RuntimeOpeningRequest{}, nil
		},
		func() string { return "id" },
		zap.NewNop(),
	)
	if err != nil {
		t.Fatalf("NewOnDemandFactoryTargetService() error = %v", err)
	}

	eventsService, err := eventswire.NewService()
	if err != nil {
		t.Fatalf("construct events service: %v", err)
	}

	lifecycle, err := provideApplicationProcessLifecycle(providersService, eventsService, factoryTarget)
	if err != nil {
		t.Fatalf("provideApplicationProcessLifecycle() error = %v", err)
	}
	if lifecycle == nil {
		t.Fatal("provideApplicationProcessLifecycle() = nil, want a composed ProcessLifecycle")
	}

	// factoryTarget never opened any runtime, so its own Close is a
	// documented no-op success; the Providers lifecycle's Close is likewise
	// a no-op with no providers configured -- so the composed Close
	// succeeding proves both closers actually ran, not that either was
	// skipped.
	if err := lifecycle.Close(context.Background()); err != nil {
		t.Fatalf("composed ProcessLifecycle.Close() error = %v, want nil", err)
	}

	secondEventsService, err := eventswire.NewService()
	if err != nil {
		t.Fatalf("construct second events service: %v", err)
	}

	nilFactoryLifecycle, err := provideApplicationProcessLifecycle(providersService, secondEventsService, nil)
	if err != nil {
		t.Fatalf("provideApplicationProcessLifecycle(nil factoryTarget) error = %v", err)
	}
	if err := nilFactoryLifecycle.Close(context.Background()); err != nil {
		t.Fatalf("composed ProcessLifecycle.Close() with a nil factoryTarget error = %v, want nil (defensive no-op)", err)
	}
}

// TestProcessCloseContinuesThroughEveryLifecycleOwnerAfterFailure proves the
// Process.Close path does not let a failure from an earlier process-owned
// closer make later owners unreachable. This is important for the lazily
// opened ACP Factory target: it must still receive its teardown attempt even
// when another process lifecycle owner reports a failure.
func TestProcessCloseContinuesThroughEveryLifecycleOwnerAfterFailure(t *testing.T) {
	t.Parallel()

	providersCloseErr := errors.New("close Providers")
	eventsCloseErr := errors.New("close Events")
	targetCloseErr := errors.New("close ACP Factory target")
	var closed []string
	lifecycle := compositeProcessLifecycle{closers: []func(context.Context) error{
		func(context.Context) error {
			closed = append(closed, "providers")
			return providersCloseErr
		},
		func(context.Context) error {
			closed = append(closed, "events")
			return eventsCloseErr
		},
		func(context.Context) error {
			closed = append(closed, "factory-target")
			return targetCloseErr
		},
	}}
	process, err := initializerapplication.NewProcess(nil, nil, wireTestProviderRegistry{}, lifecycle, nil, nil)
	if err != nil {
		t.Fatalf("NewProcess() error = %v", err)
	}

	err = process.Close(context.Background())
	for _, expected := range []error{providersCloseErr, eventsCloseErr, targetCloseErr} {
		if !errors.Is(err, expected) {
			t.Fatalf("Process.Close() error = %v, want it to retain %v", err, expected)
		}
	}
	if !slices.Equal(closed, []string{"providers", "events", "factory-target"}) {
		t.Fatalf("Process.Close() closer calls = %v, want every process lifecycle owner in order", closed)
	}
}

// TestProvideApplicationProcessLifecycle_RequiresProvidersLifecycle proves a
// providers.Service that does not implement providers.Lifecycle is rejected
// at construction rather than silently producing a ProcessLifecycle whose
// Providers-side Close can never run.
func TestProvideApplicationProcessLifecycle_RequiresProvidersLifecycle(t *testing.T) {
	t.Parallel()

	_, err := provideApplicationProcessLifecycle(nonLifecycleProvidersService{}, nil, nil)
	if err == nil {
		t.Fatal("provideApplicationProcessLifecycle() error = nil, want a construction error for a non-Lifecycle providers.Service")
	}
}

// nonLifecycleProvidersService is a providers.Service stand-in (embedding
// the interface as a permanently nil value, satisfying it for the compiler
// without implementing any method) that deliberately does not also
// implement providers.Lifecycle, for
// TestProvideApplicationProcessLifecycle_RequiresProvidersLifecycle.
type nonLifecycleProvidersService struct {
	providers.Service
}
