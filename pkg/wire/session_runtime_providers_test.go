package wire

import (
	"context"
	"encoding/json"
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"sync"
	"testing"

	"go.uber.org/zap"

	"github.com/portpowered/infinite-you/internal/testutil"
	initializerapplication "github.com/portpowered/infinite-you/pkg/initializer/application"
	platformclock "github.com/portpowered/infinite-you/pkg/platform/clock"
	platformfilesystem "github.com/portpowered/infinite-you/pkg/platform/filesystem"
	"github.com/portpowered/infinite-you/pkg/platform/logging"
	platformprocess "github.com/portpowered/infinite-you/pkg/platform/process"
	serviceedges "github.com/portpowered/infinite-you/pkg/services/edges"
	events "github.com/portpowered/infinite-you/pkg/services/events"
	eventswire "github.com/portpowered/infinite-you/pkg/services/events/wire"
	factorydefinitions "github.com/portpowered/infinite-you/pkg/services/factory_definitions"
	factoryruntime "github.com/portpowered/infinite-you/pkg/services/factory_runtime"
	factorysessions "github.com/portpowered/infinite-you/pkg/services/factory_sessions"
	factorysessionwire "github.com/portpowered/infinite-you/pkg/services/factory_sessions/wire"
	factoryvisualization "github.com/portpowered/infinite-you/pkg/services/factory_visualization"
	"github.com/portpowered/infinite-you/pkg/services/models"
	globalconfigmapping "github.com/portpowered/infinite-you/pkg/services/operator_settings/transports/globalconfig"
	settingswire "github.com/portpowered/infinite-you/pkg/services/operator_settings/wire"
	"github.com/portpowered/infinite-you/pkg/services/providers"
	"github.com/portpowered/infinite-you/pkg/services/recordings"
	"github.com/portpowered/infinite-you/pkg/services/work"
	"github.com/portpowered/infinite-you/pkg/services/workers"
)

func codexWireTestOutput(content string) []byte {
	item, _ := json.Marshal(map[string]any{"type": "item.completed", "item": map[string]any{"id": "message", "type": "agent_message", "text": content}})
	return []byte("{\"type\":\"turn.started\"}\n" + string(item) + "\n{\"type\":\"turn.completed\",\"usage\":{\"input_tokens\":1,\"output_tokens\":1}}\n")
}

func TestProvideFactoryVisualizationMetricsQueryConstructsInertCapability(t *testing.T) {
	t.Parallel()

	query, err := provideFactoryVisualizationMetricsQuery(logging.NoopLogger{})
	if err != nil {
		t.Fatalf("provideFactoryVisualizationMetricsQuery() error = %v", err)
	}
	if query == nil {
		t.Fatal("provideFactoryVisualizationMetricsQuery() returned nil query")
	}
	capability, err := provideRuntimeMetricsQueryCapability(query)
	if err != nil {
		t.Fatalf("provideRuntimeMetricsQueryCapability() error = %v", err)
	}
	got, ok := capability.RuntimeMetricsQuery().(factoryvisualization.RuntimeMetricsQuery)
	if !ok || got == nil {
		t.Fatalf("RuntimeMetricsQuery() = %#v, want a non-nil query operation", capability.RuntimeMetricsQuery())
	}

	result, err := query.QueryRuntimeMetrics(t.Context(), factoryvisualization.RuntimeMetricsQueryRequest{
		MetricsRoot: t.TempDir(),
	})
	if err != nil {
		t.Fatalf("constructed query operation error = %v", err)
	}
	if result.Cost.Availability != factoryvisualization.RuntimeMetricsCostUnavailable {
		t.Fatalf("constructed query cost = %#v, want unavailable", result.Cost)
	}
}

func TestProvideRuntimeMetricsQueryCapabilityRejectsMissingQuery(t *testing.T) {
	t.Parallel()

	if capability, err := provideRuntimeMetricsQueryCapability(nil); capability != nil || err == nil {
		t.Fatalf("provideRuntimeMetricsQueryCapability(nil) = (%#v, %v), want construction failure", capability, err)
	}
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
	factory := provideConductorInvocationWithProgressFactory(providersService, edges, allocator)
	executor, err := factory(nil, adaptRunner(edges.ProviderCommandRunner), nil)
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
	executor, err := provideConductorInvocationWithProgressFactory(providersService, edges, allocator)(
		nil, provideWorkerCommandRunnerAdapter()(runner), nil,
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
	factory := provideConductorInvocationWithProgressFactory(providersService, edges, allocator)
	if _, err = factory(providersService, adaptRunner(edges.ProviderCommandRunner), nil); err != nil {
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
	provider, err := provideProvidersService(edges)
	if err != nil {
		t.Fatalf("provideProvidersService() error = %v", err)
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
		provideFactoryRuntimeProviderOverride(edges),
		eventsService,
		factorysessionwire.NewLiveChangeCoordinator(),
	)

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
// lifecycle's own Close, the metrics retention lifecycle, and the on-demand Factory Sessions activation
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
	metricsClosed := false
	metricsOwner := &closingRuntimeMetricsOwner{
		onClose: func() error {
			metricsClosed = true
			return nil
		},
	}
	modelsClosed := false
	modelsService := &closingModelsService{
		onClose: func() error {
			modelsClosed = true
			return nil
		},
	}

	sessionsClosed := false
	sessionsService := &closingFactorySessionsService{
		onClose: func() error {
			sessionsClosed = true
			return nil
		},
	}
	lifecycle, err := provideApplicationProcessLifecycle(providersService, modelsService, eventsService, sessionsService, factoryTarget, &localWorkerSessionsBoundary{}, metricsOwner)
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
	if !metricsClosed {
		t.Fatal("composed ProcessLifecycle.Close() did not close metrics retention lifecycle")
	}
	if !modelsClosed {
		t.Fatal("composed ProcessLifecycle.Close() did not close the Models lifecycle")
	}
	if !sessionsClosed {
		t.Fatal("composed ProcessLifecycle.Close() did not close the Factory Sessions lifecycle")
	}

	secondEventsService, err := eventswire.NewService()
	if err != nil {
		t.Fatalf("construct second events service: %v", err)
	}

	nilFactoryLifecycle, err := provideApplicationProcessLifecycle(providersService, &closingModelsService{}, secondEventsService, &closingFactorySessionsService{}, nil, &localWorkerSessionsBoundary{}, nil)
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
	lifecycle := &compositeProcessLifecycle{closers: []func(context.Context) error{
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
	process, err := initializerapplication.NewProcess(nil, nil, wireTestProviderRegistry{}, lifecycle, nil, nil, nil, nil, nil)
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
	if err := process.Close(context.Background()); !errors.Is(err, providersCloseErr) || !errors.Is(err, eventsCloseErr) || !errors.Is(err, targetCloseErr) {
		t.Fatalf("repeated Process.Close() error = %v, want the cached joined lifecycle errors", err)
	}
	if !slices.Equal(closed, []string{"providers", "events", "factory-target"}) {
		t.Fatalf("repeated Process.Close() closer calls = %v, want exact-once lifecycle attempts", closed)
	}
}

// TestProvideApplicationProcessLifecycle_ClosesProvidersAndEventsExactlyOnceAfterAFailure
// proves the failure-continuation contract above holds for the lifecycle Wire
// actually composes, not only for a hand-assembled composite. The two owners
// that retain real process resources -- Providers (executable/session
// teardown) and the singular Events root -- must each be reached exactly once
// per Close, in composition order, and an earlier owner's teardown failure must
// neither skip nor mask the later one.
func TestProvideApplicationProcessLifecycle_ClosesProvidersAndEventsExactlyOnceAfterAFailure(t *testing.T) {
	t.Parallel()

	providersCloseErr := errors.New("close Providers")
	eventsCloseErr := errors.New("close Events")
	var closed []string
	providersStub := &closingProvidersService{
		onClose: func() error { closed = append(closed, "providers"); return providersCloseErr },
	}
	eventsStub := &closingEventsService{
		onClose: func() error { closed = append(closed, "events"); return eventsCloseErr },
	}
	sessionsStub := &closingFactorySessionsService{
		onClose: func() error { closed = append(closed, "factory-sessions"); return nil },
	}

	lifecycle, err := provideApplicationProcessLifecycle(providersStub, &closingModelsService{}, eventsStub, sessionsStub, nil, &localWorkerSessionsBoundary{}, nil)
	if err != nil {
		t.Fatalf("provideApplicationProcessLifecycle() error = %v", err)
	}

	closeErr := lifecycle.Close(context.Background())
	for _, expected := range []error{providersCloseErr, eventsCloseErr} {
		if !errors.Is(closeErr, expected) {
			t.Fatalf("composed ProcessLifecycle.Close() error = %v, want it to retain %v", closeErr, expected)
		}
	}
	if !slices.Equal(closed, []string{"factory-sessions", "providers", "events"}) {
		t.Fatalf(
			"composed ProcessLifecycle.Close() closer calls = %v, want Factory Sessions then Providers then Events exactly once each",
			closed,
		)
	}
	if repeated := lifecycle.Close(context.Background()); !errors.Is(repeated, providersCloseErr) || !errors.Is(repeated, eventsCloseErr) {
		t.Fatalf("repeated composed ProcessLifecycle.Close() error = %v, want cached provider/events failures", repeated)
	}
	if !slices.Equal(closed, []string{"factory-sessions", "providers", "events"}) {
		t.Fatalf("repeated composed lifecycle calls = %v, want exact-once provider/session/event attempts", closed)
	}
}

// closingProvidersService is a providers.Service stand-in (embedding the
// interface as a permanently nil value) that implements only the process
// teardown method provideApplicationProcessLifecycle requires, so its Close is
// observable in the composed lifecycle.
type closingProvidersService struct {
	providers.Service

	onClose func() error
}

func (service *closingProvidersService) Close(context.Context) error { return service.onClose() }

// closingEventsService is the equivalent events.Service stand-in for the
// singular Events root's process teardown.
type closingEventsService struct {
	events.Service

	onClose func() error
}

func (service *closingEventsService) Close(context.Context) error { return service.onClose() }

// closingModelsService is the equivalent Models root stand-in. Its embedded
// public interface keeps this test focused on the internal process lifecycle
// hook rather than duplicating the full Models contract.
type closingModelsService struct {
	models.Service

	onClose func() error
}

func (service *closingModelsService) Close(context.Context) error {
	if service.onClose == nil {
		return nil
	}
	return service.onClose()
}

type closingRuntimeMetricsOwner struct {
	factoryruntime.RuntimeMetricsOwner
	onClose func() error
}

func (owner *closingRuntimeMetricsOwner) Close(context.Context) error { return owner.onClose() }

type closingFactorySessionsService struct {
	factorysessions.Service
	onClose func() error
}

type factorySessionsServiceWithoutLifecycle struct {
	factorysessions.Service
}

func (service *closingFactorySessionsService) Close(context.Context) error {
	if service.onClose == nil {
		return nil
	}
	return service.onClose()
}

// TestProvideApplicationProcessLifecycle_RequiresProvidersLifecycle proves a
// providers.Service that does not implement providers.Lifecycle is rejected
// at construction rather than silently producing a ProcessLifecycle whose
// Providers-side Close can never run.
func TestProvideApplicationProcessLifecycle_RequiresProvidersLifecycle(t *testing.T) {
	t.Parallel()

	_, err := provideApplicationProcessLifecycle(nonLifecycleProvidersService{}, &closingModelsService{}, nil, &closingFactorySessionsService{}, nil, &localWorkerSessionsBoundary{}, nil)
	if err == nil {
		t.Fatal("provideApplicationProcessLifecycle() error = nil, want a construction error for a non-Lifecycle providers.Service")
	}
}

func TestProvideApplicationProcessLifecycle_RequiresFactorySessionsLifecycle(t *testing.T) {
	t.Parallel()

	providersService := &closingProvidersService{}
	_, err := provideApplicationProcessLifecycle(
		providersService,
		&closingModelsService{},
		&closingEventsService{},
		factorySessionsServiceWithoutLifecycle{},
		nil,
		&localWorkerSessionsBoundary{},
		nil,
	)
	if err == nil || !strings.Contains(err.Error(), "Factory Sessions lifecycle is required") {
		t.Fatalf("provideApplicationProcessLifecycle() error = %v, want Factory Sessions lifecycle diagnostic", err)
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

func TestCanonicalStatelessWorkersExecuteBeforeRuntimeOpening(t *testing.T) {
	providersService, err := provideProvidersService(serviceedges.Edges{})
	if err != nil {
		t.Fatalf("provideProvidersService() error = %v", err)
	}
	modelsService, err := provideModelsService(serviceedges.Edges{})
	if err != nil {
		t.Fatalf("provideModelsService() error = %v", err)
	}
	worktreePreparer, err := provideWorkersWorktree(serviceedges.Edges{})
	if err != nil {
		t.Fatalf("provideWorkersWorktree() error = %v", err)
	}
	worktreeRelease := provideWorkersWorktreeRelease(worktreePreparer)
	service, err := provideStatelessWorkersService(
		providersService,
		modelsService,
		statelessCompositionCommandRunner{},
		platformfilesystem.Local{},
		platformclock.Real{},
		zap.NewNop(),
		worktreePreparer,
		worktreeRelease,
		platformfilesystem.Local{},
		nil,
		platformfilesystem.Local{},
		statelessDecisionEnvelopeService(t),
	)
	if err != nil {
		t.Fatalf("provideStatelessWorkersService() error = %v", err)
	}

	result, err := service.Execute(context.Background(), workers.ExecuteRequest{
		Correlation: workers.ExecutionCorrelation{
			FactorySessionID: "session-canonical",
			RuntimeID:        "runtime-canonical",
			GenerationID:     "generation-canonical",
			DispatchID:       "dispatch-canonical",
			AttemptID:        "attempt-canonical",
		},
		Target: workers.ExecutionTarget{
			WorkerName: "script-worker",
			RunnerID:   "script",
			Command:    "canonical-script",
		},
	})
	if err != nil {
		t.Fatalf("Execute() error = %v", err)
	}
	if result.Outcome != workers.ExecutionOutcomeAccepted {
		t.Fatalf("outcome = %q, want ACCEPTED", result.Outcome)
	}
	if len(result.Output.Primary) != 1 || result.Output.Primary[0].Text != "canonical-output" {
		t.Fatalf("output = %#v, want canonical-output", result.Output)
	}
}

func TestBuildStatelessWorkersExecutesBeforeRuntimeOpening(t *testing.T) {
	service, err := BuildStatelessWorkers(t.Context(), serviceedges.Edges{
		ScriptCommandRunner: statelessProcessCommandRunner{},
	})
	if err != nil {
		t.Fatalf("BuildStatelessWorkers() error = %v", err)
	}

	result, err := service.Execute(context.Background(), workers.ExecuteRequest{
		Correlation: workers.ExecutionCorrelation{
			FactorySessionID: "session-built-canonical",
			RuntimeID:        "runtime-built-canonical",
			GenerationID:     "generation-built-canonical",
			DispatchID:       "dispatch-built-canonical",
			AttemptID:        "attempt-built-canonical",
		},
		Target: workers.ExecutionTarget{
			WorkerName: "script-worker",
			RunnerID:   "script",
			Command:    "built-canonical-script",
		},
	})
	if err != nil {
		t.Fatalf("Execute() error = %v", err)
	}
	if result.Outcome != workers.ExecutionOutcomeAccepted ||
		len(result.Output.Primary) != 1 || result.Output.Primary[0].Text != "canonical-output" {
		t.Fatalf("result = %#v, want accepted canonical output", result)
	}
}

func TestProvideWorkersWorktreeReleaseReturnsNilForPreparerWithoutRelease(t *testing.T) {
	if release := provideWorkersWorktreeRelease(statelessPreparerOnly{}); release != nil {
		t.Fatal("provideWorkersWorktreeRelease() returned a callback for a preparer without release support")
	}
}

func TestProvideStatelessWorkersServiceRejectsMissingClock(t *testing.T) {
	_, err := provideStatelessWorkersService(nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil)
	if err == nil {
		t.Fatal("provideStatelessWorkersService() error = nil, want missing clock error")
	}
}

func TestCanonicalStatelessWorkersReleasesProductionWorktreeAfterSuccess(t *testing.T) {
	git := &statelessWorktreeGit{}
	service := newProductionCleanupStatelessService(t, git, statelessCompositionCommandRunner{})

	result, err := service.Execute(context.Background(), statelessWorktreeRequest())
	if err != nil {
		t.Fatalf("Execute() error = %v", err)
	}
	if result.Outcome != workers.ExecutionOutcomeAccepted {
		t.Fatalf("outcome = %q, want ACCEPTED", result.Outcome)
	}
	git.assertRemoved(t)
}

func TestCanonicalStatelessWorkersReleasesProductionWorktreeAfterCancellation(t *testing.T) {
	started := make(chan struct{})
	git := &statelessWorktreeGit{}
	service := newProductionCleanupStatelessService(t, git, &statelessBlockingCommandRunner{started: started})

	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct {
		result workers.ExecuteResult
		err    error
	}, 1)
	go func() {
		result, err := service.Execute(ctx, statelessWorktreeRequest())
		done <- struct {
			result workers.ExecuteResult
			err    error
		}{result: result, err: err}
	}()
	<-started
	cancel()

	completed := <-done
	if completed.err != nil {
		t.Fatalf("Execute() error = %v", completed.err)
	}
	if completed.result.Outcome != workers.ExecutionOutcomeCanceled {
		t.Fatalf("outcome = %q, want CANCELED", completed.result.Outcome)
	}
	git.assertRemoved(t)
}

func TestCanonicalStatelessWorkersReleasesProductionWorktreeAfterPreStartFailure(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	git := &statelessWorktreeGit{onAdd: cancel}
	service := newProductionCleanupStatelessService(t, git, statelessCompositionCommandRunner{})

	_, err := service.Execute(ctx, statelessWorktreeRequest())
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("Execute() error = %v, want context.Canceled", err)
	}
	git.assertRemoved(t)
}

func newProductionCleanupStatelessService(
	t *testing.T,
	git *statelessWorktreeGit,
	commandRunner factorysessionwire.ScriptCommandRunner,
) workers.Service {
	t.Helper()
	edges := serviceedges.Edges{
		WorkersWorktreeFileSystem: statelessWorktreeFileSystem{},
		WorkersWorktreeGit:        git,
	}
	providersService, err := provideProvidersService(edges)
	if err != nil {
		t.Fatalf("provideProvidersService() error = %v", err)
	}
	modelsService, err := provideModelsService(edges)
	if err != nil {
		t.Fatalf("provideModelsService() error = %v", err)
	}
	worktreePreparer, err := provideWorkersWorktree(edges)
	if err != nil {
		t.Fatalf("provideWorkersWorktree() error = %v", err)
	}
	worktreeRelease := provideWorkersWorktreeRelease(worktreePreparer)
	service, err := provideStatelessWorkersService(
		providersService,
		modelsService,
		commandRunner,
		platformfilesystem.Local{},
		platformclock.Real{},
		zap.NewNop(),
		worktreePreparer,
		worktreeRelease,
		platformfilesystem.Local{},
		nil,
		platformfilesystem.Local{},
		statelessDecisionEnvelopeService(t),
	)
	if err != nil {
		t.Fatalf("provideStatelessWorkersService() error = %v", err)
	}
	return service
}

func statelessWorktreeRequest() workers.ExecuteRequest {
	return workers.ExecuteRequest{
		Correlation: workers.ExecutionCorrelation{
			FactorySessionID: "session-worktree",
			RuntimeID:        "runtime-worktree",
			GenerationID:     "generation-worktree",
			DispatchID:       "dispatch-worktree",
			AttemptID:        "attempt-worktree",
		},
		Target: workers.ExecutionTarget{
			WorkerName: "script-worker",
			RunnerID:   "script",
			Command:    "cleanup-script",
			Workspace: workers.WorkspacePolicy{
				PrepareWorktree:    true,
				FactoryDirectory:   "factory-root",
				CheckoutIdentifier: "attempt-worktree",
			},
		},
	}
}

type statelessWorktreeFileSystem struct{}

type statelessPreparerOnly struct{}

func (statelessPreparerOnly) Prepare(
	context.Context,
	string,
	string,
) (workers.FactoryWorktreePreparation, error) {
	return workers.FactoryWorktreePreparation{}, nil
}

func (statelessWorktreeFileSystem) Stat(string) (fs.FileInfo, error) {
	return nil, fs.ErrNotExist
}

func (statelessWorktreeFileSystem) Lstat(string) (fs.FileInfo, error) {
	return nil, fs.ErrNotExist
}

func (statelessWorktreeFileSystem) MkdirAll(string, fs.FileMode) error {
	return nil
}

type statelessWorktreeGit struct {
	mu    sync.Mutex
	calls []string
	onAdd func()
}

func (git *statelessWorktreeGit) Run(_ context.Context, _ string, args ...string) (string, string, int, error) {
	git.mu.Lock()
	git.calls = append(git.calls, strings.Join(args, " "))
	git.mu.Unlock()
	if len(args) >= 2 && args[0] == "worktree" && args[1] == "add" {
		if git.onAdd != nil {
			git.onAdd()
		}
		return "", "", 0, nil
	}
	if len(args) >= 2 && args[0] == "worktree" && args[1] == "remove" {
		return "", "", 0, nil
	}
	return "repo-root", "", 0, nil
}

func (git *statelessWorktreeGit) assertRemoved(t *testing.T) {
	t.Helper()
	git.mu.Lock()
	defer git.mu.Unlock()
	if len(git.calls) == 0 || !strings.HasPrefix(git.calls[len(git.calls)-1], "worktree remove --force ") {
		t.Fatalf("Git calls = %#v, want final worktree remove", git.calls)
	}
}

type statelessBlockingCommandRunner struct {
	started chan struct{}
	once    sync.Once
}

func (runner *statelessBlockingCommandRunner) Run(ctx context.Context, _ platformprocess.CommandRequest) (platformprocess.CommandResult, error) {
	runner.once.Do(func() { close(runner.started) })
	<-ctx.Done()
	return platformprocess.CommandResult{}, ctx.Err()
}

type statelessCompositionCommandRunner struct{}

func (statelessCompositionCommandRunner) Run(
	context.Context,
	platformprocess.CommandRequest,
) (platformprocess.CommandResult, error) {
	return platformprocess.CommandResult{Stdout: []byte("canonical-output")}, nil
}

type statelessProcessCommandRunner struct{}

func (statelessProcessCommandRunner) Run(
	context.Context,
	platformprocess.CommandRequest,
) (platformprocess.CommandResult, error) {
	return platformprocess.CommandResult{Stdout: []byte("canonical-output")}, nil
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

func TestWorkerRecordingRootUsesCanonicalHomeAndScenarioHome(t *testing.T) {
	t.Parallel()

	defaultRoot, err := workerRecordingRoot(serviceedges.Edges{})
	if err != nil {
		t.Fatalf("workerRecordingRoot(default) error = %v", err)
	}
	defaultHome, err := os.UserHomeDir()
	if err != nil {
		t.Fatalf("os.UserHomeDir() error = %v", err)
	}
	if want := filepath.Join(defaultHome, workerRecordingHomeDirectory, workerRecordingStoreDirectory); defaultRoot != want {
		t.Fatalf("workerRecordingRoot(default) = %q, want %q", defaultRoot, want)
	}

	home := t.TempDir()
	resolvedRoot, err := workerRecordingRoot(serviceedges.Edges{
		WorkerSessionResolveHomeDirectory: func() (string, error) { return home, nil },
	})
	if err != nil {
		t.Fatalf("workerRecordingRoot(scenario) error = %v", err)
	}
	want := filepath.Join(home, workerRecordingHomeDirectory, workerRecordingStoreDirectory)
	if resolvedRoot != want {
		t.Fatalf("workerRecordingRoot(scenario) = %q, want %q", resolvedRoot, want)
	}
	if filepath.Dir(resolvedRoot) == filepath.Dir(defaultRoot) {
		t.Fatalf("scenario recording root %q unexpectedly shares the temporary parent %q", resolvedRoot, defaultRoot)
	}
}

func TestProvideWorkerRecordingWriterPersistsUnderResolvedHome(t *testing.T) {
	t.Parallel()

	for _, home := range []string{t.TempDir(), t.TempDir()} {
		writer, err := provideWorkerRecordingWriter(serviceedges.Edges{
			WorkerSessionResolveHomeDirectory: func() (string, error) { return home, nil },
		})
		if err != nil {
			t.Fatalf("provideWorkerRecordingWriter() error = %v", err)
		}
		failureWriter, ok := writer.(recordings.WorkerRecordingFailureWriter)
		if !ok || failureWriter == nil {
			t.Fatalf("worker recording writer type %T does not expose failure persistence", writer)
		}
		if err := failureWriter.PersistWorkerRecordingFailure(context.Background(), recordings.WorkerRecordingFailure{
			RecordingID:     "recording-home-seam",
			WorkerSessionID: "worker-home-seam",
			Topic:           "worker-session/worker-home-seam/events",
			Code:            "PERSISTENCE_FAILED",
		}); err != nil {
			t.Fatalf("PersistWorkerRecordingFailure() error = %v", err)
		}

		root := filepath.Join(home, workerRecordingHomeDirectory, workerRecordingStoreDirectory)
		entries, err := os.ReadDir(root)
		if err != nil {
			t.Fatalf("read resolved Worker recording root %q: %v", root, err)
		}
		if len(entries) != 1 || entries[0].IsDir() {
			t.Fatalf("resolved Worker recording entries = %#v, want one artifact", entries)
		}
		reader, ok := writer.(recordings.WorkerRecordingReader)
		if !ok || reader == nil {
			t.Fatalf("worker recording writer type %T does not expose reading", writer)
		}
		snapshot, err := reader.LoadWorkerRecording(context.Background(), "recording-home-seam")
		if err != nil || snapshot.RecordingID != "recording-home-seam" {
			t.Fatalf("LoadWorkerRecording() = (%#v, %v), want persisted scenario identity", snapshot, err)
		}
	}
}

func TestWorkerRecordingRootReportsResolverFailures(t *testing.T) {
	t.Parallel()

	resolverErr := errors.New("home unavailable")
	if root, err := workerRecordingRoot(serviceedges.Edges{
		WorkerSessionResolveHomeDirectory: func() (string, error) { return "", resolverErr },
	}); root != "" || !errors.Is(err, resolverErr) {
		t.Fatalf("workerRecordingRoot(failing resolver) = (%q, %v), want wrapped resolver error", root, err)
	}
	if root, err := workerRecordingRoot(serviceedges.Edges{
		WorkerSessionResolveHomeDirectory: func() (string, error) { return "  ", nil },
	}); root != "" || err == nil {
		t.Fatalf("workerRecordingRoot(empty resolver) = (%q, %v), want empty-path error", root, err)
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
	execution := wireTestWorkersService{}
	factory := provideWorkerSessionsFactory(eventsService, providerSessions, logging.NoopLogger{}, nil)
	service, err := factory(execution, platformclock.Real{})
	if err != nil {
		t.Fatalf("worker sessions factory() error = %v", err)
	}
	if service == nil {
		t.Fatal("worker sessions factory() returned nil service")
	}
}

type wireTestWorkersService struct {
	workers.ModelInvoker
}

func (wireTestWorkersService) Execute(
	_ context.Context,
	request workers.ExecuteRequest,
) (workers.ExecuteResult, error) {
	return workers.ExecuteResult{Correlation: request.Correlation, Outcome: workers.ExecutionOutcomeAccepted}, nil
}

// statelessDecisionEnvelopeService resolves the same Factory Definitions
// decision-envelope owner the composed process injects, so the stateless
// Workers root under test parses envelopes through the canonical contract.
func statelessDecisionEnvelopeService(t *testing.T) factorydefinitions.DecisionEnvelopeService {
	t.Helper()

	ports, err := provideFactoryInvocationPolicyPorts()
	if err != nil {
		t.Fatalf("provideFactoryInvocationPolicyPorts() error = %v", err)
	}
	return provideDecisionEnvelopeService(ports)
}
