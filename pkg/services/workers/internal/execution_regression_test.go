package internal

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/portpowered/infinite-you/internal/testutil/runtimefixtures"
	platformfilesystem "github.com/portpowered/infinite-you/pkg/platform/filesystem"
	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
	"go.uber.org/zap/zaptest/observer"

	interfaces "github.com/portpowered/infinite-you/pkg/services/factory_definitions"
	"github.com/portpowered/infinite-you/pkg/services/models"
	"github.com/portpowered/infinite-you/pkg/services/providers"
	"github.com/portpowered/infinite-you/pkg/services/work"
	"github.com/portpowered/infinite-you/pkg/services/workers"
	workeragentrun "github.com/portpowered/infinite-you/pkg/services/workers/internal/services/workstations/executor/agentrun"

	workerconstruction "github.com/portpowered/infinite-you/pkg/services/workers/internal/services/runtime_assembly/construction"
)

// agentInvocationRuntimeService constructs a minimal, directly-composed
// *Service exercising the same construction inputs New() validates, without
// requiring the full New()/NewRuntime() dependency graph. It is used to prove
// BuildModelInvocationExecutor forwards the exact construction-injected
// progress publisher into execution, rather than the previously hardcoded nil.
func agentInvocationRuntimeService(providersService providers.Service, publisher workers.ProgressPublisher) *Service {
	executorBuilder := workerconstruction.New(
		providersService,
		nil,
		nil,
		nil,
		testFactoryDocsLoader,
		testFactoryWorktreePreparer{},
		workeragentrun.NewLibraryHarnessAdapter(platformfilesystem.Local{}),
		testRetryRandom,
		platformfilesystem.Local{},
	)
	return &Service{
		sessions:                inertCurrentRuntimeResolver{},
		executorBuilder:         executorBuilder,
		progressPublisher:       publisher,
		clock:                   time.Now,
		processEnvironment:      func() []string { return nil },
		currentWorkingDirectory: func() (string, error) { return "", nil },
	}
}

func agentInvocationRuntimeConfig() runtimefixtures.RuntimeConfigLookupFixture {
	worker := &interfaces.FactoryWorkerConfig{
		Name: "agent-worker",
		Type: interfaces.WorkerTypeAgent,
	}
	return runtimefixtures.RuntimeConfigLookupFixture{
		Workers: map[string]*interfaces.FactoryWorkerConfig{worker.Name: worker},
		Factory: &interfaces.FactoryConfig{},
	}
}

func agentInvocationExecutionRequest(dispatchID string) workers.WorkstationExecutionRequest {
	return workers.WorkstationExecutionRequest{
		Dispatch:     work.WorkDispatch{DispatchID: dispatchID, WorkerType: "agent-worker"},
		WorkerType:   "agent-worker",
		RunnerID:     string(providers.IDCodex),
		SystemPrompt: "system fixture",
		UserMessage:  "user fixture",
	}
}

// TestBuildModelInvocationExecutorDeliversSuccessProgressThroughConstructionInjectedPublisher
// proves the direct model-invocation executor path (used by InvokeModel) now
// delivers progress through the construction-injected publisher retained on
// Service, with the same ordering and cardinality the registered Agent Runner
// already produces, instead of the previously hardcoded no-op.
func TestBuildModelInvocationExecutorDeliversSuccessProgressThroughConstructionInjectedPublisher(t *testing.T) {
	t.Parallel()

	fake := newServiceAgentProvidersFake()
	fake.result.Diagnostics.Progress = []providers.ExecuteProgress{
		{Phase: "planning", Detail: "first"},
		{Phase: "responding", Detail: "second"},
	}

	var published []workers.ProgressFragment
	publisher := workers.ProgressPublisher(func(fragment workers.ProgressFragment) {
		published = append(published, cloneServiceProgressFragment(fragment))
	})

	service := agentInvocationRuntimeService(fake, publisher)
	runtimeCfg := agentInvocationRuntimeConfig()

	executor, err := service.BuildModelInvocationExecutor(runtimeCfg, runtimeCfg.FactoryConfig(), "agent-worker")
	if err != nil {
		t.Fatalf("BuildModelInvocationExecutor() error = %v", err)
	}

	result, err := executor.Execute(context.Background(), agentInvocationExecutionRequest("dispatch-success-1"))
	if err != nil {
		t.Fatalf("Execute() error = %v", err)
	}
	if result.Outcome != workers.OutcomeAccepted || result.Output != "fixture output" {
		t.Fatalf("Execute() result = %#v, want accepted fixture output", result)
	}
	if fake.calls.Load() != 1 {
		t.Fatalf("Providers.Execute calls = %d, want 1", fake.calls.Load())
	}

	// Two raw provider progress facts, then the default terminal message and
	// stream-completion the runner always appends when content is present and
	// no explicit run/turn-completion phase was observed.
	wantKinds := []string{
		workers.ProgressFragmentKind,
		workers.ProgressFragmentKind,
		workers.ProgressFragmentKind,
		workers.CompletedFragmentKind,
	}
	if len(published) != len(wantKinds) {
		t.Fatalf("published fragments = %#v, want %d entries", published, len(wantKinds))
	}
	for i, kind := range wantKinds {
		if published[i].Kind != kind || published[i].DispatchID != "dispatch-success-1" {
			t.Fatalf("published[%d] = %#v, want kind %q for dispatch-success-1", i, published[i], kind)
		}
	}
}

// TestBuildModelInvocationExecutorDeliversFailureProgressThroughConstructionInjectedPublisher
// proves a runner failure still surfaces the expected error outcome and the
// same failure-progress ordering/cardinality through the construction-injected
// publisher.
func TestBuildModelInvocationExecutorDeliversFailureProgressThroughConstructionInjectedPublisher(t *testing.T) {
	t.Parallel()

	// A permanent bad-request failure is not retryable, so the underlying
	// provider fake observes exactly one attempt.
	fake := &failingServiceAgentProvidersFake{
		serviceAgentProvidersFake: newServiceAgentProvidersFake(),
		failure:                   providerServiceFailureFixture(providers.ExecuteFailureKindInvalidRequest),
	}

	var published []workers.ProgressFragment
	publisher := workers.ProgressPublisher(func(fragment workers.ProgressFragment) {
		published = append(published, cloneServiceProgressFragment(fragment))
	})

	service := agentInvocationRuntimeService(fake, publisher)
	runtimeCfg := agentInvocationRuntimeConfig()

	executor, err := service.BuildModelInvocationExecutor(runtimeCfg, runtimeCfg.FactoryConfig(), "agent-worker")
	if err != nil {
		t.Fatalf("BuildModelInvocationExecutor() error = %v", err)
	}

	result, err := executor.Execute(context.Background(), agentInvocationExecutionRequest("dispatch-failure-1"))
	if err != nil {
		t.Fatalf("Execute() error = %v, want a failed WorkResult rather than a transport error", err)
	}
	if result.Outcome != workers.OutcomeFailed {
		t.Fatalf("Execute() result = %#v, want a failed outcome", result)
	}
	if fake.calls.Load() != 1 {
		t.Fatalf("Providers.Execute calls = %d, want exactly one (no retry for a throttled failure without decision)", fake.calls.Load())
	}

	if len(published) != 2 ||
		published[0].Kind != workers.ProgressFragmentKind ||
		published[0].DispatchID != "dispatch-failure-1" ||
		published[1].Kind != workers.FailedFragmentKind ||
		published[1].DispatchID != "dispatch-failure-1" {
		t.Fatalf("published fragments = %#v, want one progress fact followed by one terminal failure", published)
	}
}

// TestNewRuntimeConstructsIndependentWorkstationLifecycle proves two runtimes
// constructed through the canonical Workers construction provider (NewRuntime)
// retain independent workstation pool lifecycle state: starting and stopping
// one runtime's pool has no observable effect on an independently constructed
// runtime's pool.
func TestNewRuntimeConstructsIndependentWorkstationLifecycle(t *testing.T) {
	t.Parallel()

	first := newTestFullRuntimeService(t, zap.NewNop())
	second := newTestFullRuntimeService(t, zap.NewNop())

	binding := []workers.AssembledRuntimeBinding{{
		RoleName: "review",
		RoleKind: workers.RuntimeBuildRoleKindWorkstation,
	}}
	if _, err := first.StartWorkstationPool(context.Background(), workers.WorkstationPoolStartRequest{Bindings: binding}); err != nil {
		t.Fatalf("first.StartWorkstationPool() error = %v", err)
	}

	firstRoute, err := first.WorkstationRoute(context.Background(), workers.WorkstationRouteRequest{WorkstationName: "review"})
	if err != nil || !firstRoute.Available {
		t.Fatalf("first.WorkstationRoute() = %#v, err = %v, want available", firstRoute, err)
	}

	if _, err := second.WorkstationRoute(context.Background(), workers.WorkstationRouteRequest{WorkstationName: "review"}); !errors.Is(err, workers.ErrWorkstationPoolUnavailable) {
		t.Fatalf("second.WorkstationRoute() error = %v, want ErrWorkstationPoolUnavailable (independent pool never started)", err)
	}

	if _, err := first.StopWorkstationPool(context.Background()); err != nil {
		t.Fatalf("first.StopWorkstationPool() error = %v", err)
	}

	if _, err := second.WorkstationRoute(context.Background(), workers.WorkstationRouteRequest{WorkstationName: "review"}); !errors.Is(err, workers.ErrWorkstationPoolUnavailable) {
		t.Fatalf("second.WorkstationRoute() error = %v after first's pool stopped, want it to remain independently unstarted", err)
	}

	if _, err := second.StartWorkstationPool(context.Background(), workers.WorkstationPoolStartRequest{Bindings: binding}); err != nil {
		t.Fatalf("second.StartWorkstationPool() error = %v", err)
	}
	secondRoute, err := second.WorkstationRoute(context.Background(), workers.WorkstationRouteRequest{WorkstationName: "review"})
	if err != nil || !secondRoute.Available {
		t.Fatalf("second.WorkstationRoute() = %#v, err = %v, want available after its own independent start", secondRoute, err)
	}
}

// TestNewRuntimeWorkstationDispatchLogsExcludePromptContent proves a fully
// constructed runtime (through NewRuntime, the canonical Workers construction
// provider) keeps forwarding the construction-injected logger to workstation
// dispatch lifecycle logs, and that those logs carry only safe structured
// fields, never dispatch prompt/output payload content.
func TestNewRuntimeWorkstationDispatchLogsExcludePromptContent(t *testing.T) {
	t.Parallel()

	core, logs := observer.New(zapcore.InfoLevel)
	service := newTestFullRuntimeService(t, zap.New(core))

	const sensitivePrompt = "TOP-SECRET-SYSTEM-PROMPT"
	const sensitiveOutput = "TOP-SECRET-MODEL-OUTPUT"
	executor := recordingSensitiveExecutor{output: sensitiveOutput}
	binding := []workers.AssembledRuntimeBinding{{
		RoleName: "review",
		RoleKind: workers.RuntimeBuildRoleKindWorkstation,
		Executor: executor,
	}}
	if _, err := service.StartWorkstationPool(context.Background(), workers.WorkstationPoolStartRequest{Bindings: binding}); err != nil {
		t.Fatalf("StartWorkstationPool() error = %v", err)
	}

	result, err := service.DispatchWorkstation(context.Background(), workers.WorkstationDispatchRequest{
		WorkstationName: "review",
		Execution: workers.WorkstationExecutionRequest{
			Dispatch:     work.WorkDispatch{DispatchID: "dispatch-1", TransitionID: "transition-1", WorkstationName: "review"},
			SystemPrompt: sensitivePrompt,
			UserMessage:  sensitivePrompt,
		},
	})
	if err != nil {
		t.Fatalf("DispatchWorkstation() error = %v", err)
	}
	if result.Result.Output != sensitiveOutput {
		t.Fatalf("DispatchWorkstation() output = %q, want executor output preserved", result.Result.Output)
	}

	accepted := logs.FilterMessage("workers workstation dispatch accepted").All()
	if len(accepted) != 1 {
		t.Fatalf("accepted logs = %#v, want exactly one entry surviving normal construction", accepted)
	}
	terminal := logs.FilterMessage("workers workstation dispatch terminal").All()
	if len(terminal) != 1 {
		t.Fatalf("terminal logs = %#v, want exactly one entry surviving normal construction", terminal)
	}

	for _, entry := range logs.All() {
		if entry.Message == sensitivePrompt || entry.Message == sensitiveOutput {
			t.Fatalf("log message leaked payload content: %q", entry.Message)
		}
		for _, field := range entry.Context {
			if field.String == sensitivePrompt || field.String == sensitiveOutput {
				t.Fatalf("log field %q leaked payload content in entry %q", field.Key, entry.Message)
			}
		}
	}
}

type recordingSensitiveExecutor struct {
	output string
}

func (e recordingSensitiveExecutor) Execute(
	context.Context,
	workers.WorkstationExecutionRequest,
) (workers.WorkResult, error) {
	return workers.WorkResult{Outcome: workers.OutcomeAccepted, Output: e.output}, nil
}

func newTestFullRuntimeService(t *testing.T, logger *zap.Logger) *Service {
	t.Helper()
	runtime, err := NewRuntime(
		inertCurrentRuntimeResolver{},
		testModelsService{},
		testProvidersService{},
		models.RuntimeScopeRef{},
		injectedProviderRunner{},
		injectedProviderRunner{},
		workers.ProgressPublisher(testProgressPublisher),
		&workers.MockPTYAllocator{},
		logger,
		false,
		"",
		"",
		"",
		nil,
		nil,
		time.Now,
		func() []string { return nil },
		func() (string, error) { return "", nil },
		nil,
		nil,
		nil,
		testFactoryDocsLoader,
		testResolveSymlinks,
		nil,
		platformfilesystem.Local{},
		platformfilesystem.Local{},
		"linux",
		testFactoryWorktreePreparer{},
		workeragentrun.NewLibraryHarnessAdapter(platformfilesystem.Local{}),
		testRetryRandom,
		platformfilesystem.Local{},
		platformfilesystem.Local{},
		nil,
	)
	if err != nil {
		t.Fatalf("NewRuntime() error = %v", err)
	}
	service, ok := runtime.(*Service)
	if !ok {
		t.Fatalf("NewRuntime() returned %T, want *Service", runtime)
	}
	return service
}
