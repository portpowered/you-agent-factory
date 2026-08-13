package internal

import (
	"context"
	"fmt"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	platformfilesystem "github.com/portpowered/infinite-you/pkg/platform/filesystem"
	"github.com/portpowered/infinite-you/pkg/services/providers"
	"github.com/portpowered/infinite-you/pkg/services/workers"
	workerconstruction "github.com/portpowered/infinite-you/pkg/services/workers/internal/services/runtime_assembly/construction"
	workeragentrun "github.com/portpowered/infinite-you/pkg/services/workers/internal/services/workstations/executor/agentrun"
)

// recordingProviderCommandRunner is a thread-safe workers.CommandRunner fake
// that records every call it receives, so a test can prove provider-backed
// dispatch actually invokes the construction-injected provider command
// runner -- the same runner instance production provider registry rebinding
// (rebindProviderRegistry / ProviderRegistryRebinder) threads into the
// Providers execution path -- rather than only reaching providers.Service
// directly.
type recordingProviderCommandRunner struct {
	mu       sync.Mutex
	callList []workers.CommandRequest
	stdout   []byte
	exitCode int
}

func (r *recordingProviderCommandRunner) Run(
	_ context.Context,
	request workers.CommandRequest,
) (workers.CommandResult, error) {
	r.mu.Lock()
	r.callList = append(r.callList, request)
	r.mu.Unlock()
	return workers.CommandResult{Stdout: append([]byte(nil), r.stdout...), ExitCode: r.exitCode}, nil
}

func (r *recordingProviderCommandRunner) calls() int {
	r.mu.Lock()
	defer r.mu.Unlock()
	return len(r.callList)
}

func (r *recordingProviderCommandRunner) requests() []workers.CommandRequest {
	r.mu.Lock()
	defer r.mu.Unlock()
	return append([]workers.CommandRequest(nil), r.callList...)
}

// commandRunnerBackedProvidersService is a providers.Service fake whose
// Execute method is backed by a workers.CommandRunner, mirroring how
// production provider registry rebinding threads the construction-injected
// provider command runner into the Providers execution path. It lets tests
// prove provider-backed dispatch reaches the exact construction-injected
// provider command runner, matching the coverage the script command runner
// already has (recordingScriptCommandRunner).
type commandRunnerBackedProvidersService struct {
	providers.Service
	runner   workers.CommandRunner
	progress []providers.ExecuteProgress
}

func (s commandRunnerBackedProvidersService) Execute(
	ctx context.Context,
	request providers.ExecuteRequest,
) (providers.ExecuteResult, error) {
	result, err := s.runner.Run(ctx, workers.CommandRequest{
		Command:    string(request.Provider),
		DispatchID: request.AttemptID,
	})
	if err != nil {
		return providers.ExecuteResult{}, providers.ExecuteFailure{
			Kind:    providers.ExecuteFailureKindInvalidRequest,
			Message: "provider command runner failed",
		}
	}
	if result.ExitCode != 0 {
		return providers.ExecuteResult{}, providers.ExecuteFailure{
			Kind:        providers.ExecuteFailureKindInvalidRequest,
			Message:     "provider command exited non-zero",
			Diagnostics: &providers.ExecuteDiagnostics{Progress: s.progress},
		}
	}
	return providers.ExecuteResult{
		Content: string(result.Stdout),
		Diagnostics: &providers.ExecuteDiagnostics{
			Progress: s.progress,
			Metadata: map[string]string{
				workers.ProviderResponseMetadataCompletionEvidence: "provider_response",
			},
		},
	}, nil
}

// providerInvocationRuntimeService constructs a minimal *Service whose
// executorBuilder is wired to a providers.Service that itself is backed by
// the supplied provider command runner, and whose providerCommandRunner
// field is that exact same runner instance -- mirroring how production
// construction (workerswire.NewRuntimeWithSelection, via wire's
// provideRuntimeProviderBindings/ProviderRegistryRebinder) threads one
// construction-injected provider command runner through to Providers
// execution. Provider-backed dispatch and execution therefore reach the
// exact construction-injected provider command runner, not a stand-in.
func providerInvocationRuntimeService(
	providerRunner workers.CommandRunner,
	providersService providers.Service,
	publisher workers.ProgressPublisher,
) *Service {
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
		executorBuilder:         executorBuilder,
		providerCommandRunner:   providerRunner,
		progressPublisher:       publisher,
		clock:                   time.Now,
		processEnvironment:      func() []string { return nil },
		currentWorkingDirectory: func() (string, error) { return "", nil },
	}
}

// TestBuildModelInvocationExecutorReachesConstructionInjectedProviderCommandRunner
// proves provider-backed dispatch actually invokes the construction-injected
// provider command runner (not just providers.Service), and delivers the
// runner's output as progress and WorkResult content through the
// construction-injected publisher.
func TestBuildModelInvocationExecutorReachesConstructionInjectedProviderCommandRunner(t *testing.T) {
	t.Parallel()

	runner := &recordingProviderCommandRunner{stdout: []byte("provider output")}
	providersService := commandRunnerBackedProvidersService{
		runner: runner,
		progress: []providers.ExecuteProgress{
			{Phase: "planning", Detail: "first"},
		},
	}

	var published []workers.ProgressFragment
	publisher := workers.ProgressPublisher(func(fragment workers.ProgressFragment) {
		published = append(published, cloneServiceProgressFragment(fragment))
	})

	service := providerInvocationRuntimeService(runner, providersService, publisher)
	if service.ProviderCommandRunner() != workers.CommandRunner(runner) {
		t.Fatalf("service.ProviderCommandRunner() = %#v, want the exact supplied instance", service.ProviderCommandRunner())
	}
	runtimeCfg := agentInvocationRuntimeConfig()

	executor, err := service.BuildModelInvocationExecutor(runtimeCfg, runtimeCfg.FactoryConfig(), "agent-worker")
	if err != nil {
		t.Fatalf("BuildModelInvocationExecutor() error = %v", err)
	}

	result, err := executor.Execute(context.Background(), agentInvocationExecutionRequest("provider-dispatch-success-1"))
	if err != nil {
		t.Fatalf("Execute() error = %v", err)
	}
	if result.Outcome != workers.OutcomeAccepted || result.Output != "provider output" {
		t.Fatalf("Execute() result = %#v, want accepted provider output", result)
	}
	if runner.calls() != 1 {
		t.Fatalf("provider CommandRunner calls = %d, want exactly 1 (construction-injected runner reached)", runner.calls())
	}
	if len(published) != 3 ||
		published[0].Kind != workers.ProgressFragmentKind ||
		published[1].Kind != workers.ProgressFragmentKind ||
		published[1].Payload != "provider output" ||
		published[2].Kind != workers.CompletedFragmentKind {
		t.Fatalf("published fragments = %#v, want two progress facts (runner progress, then content) followed by completion", published)
	}
}

// TestBuildModelInvocationExecutorDeliversProviderCommandRunnerFailure proves
// a provider command runner's non-zero exit surfaces as a failed WorkResult
// while still reaching the exact construction-injected provider command
// runner exactly once.
func TestBuildModelInvocationExecutorDeliversProviderCommandRunnerFailure(t *testing.T) {
	t.Parallel()

	runner := &recordingProviderCommandRunner{stdout: []byte("partial output"), exitCode: 1}
	providersService := commandRunnerBackedProvidersService{runner: runner}

	var published []workers.ProgressFragment
	publisher := workers.ProgressPublisher(func(fragment workers.ProgressFragment) {
		published = append(published, cloneServiceProgressFragment(fragment))
	})

	service := providerInvocationRuntimeService(runner, providersService, publisher)
	runtimeCfg := agentInvocationRuntimeConfig()

	executor, err := service.BuildModelInvocationExecutor(runtimeCfg, runtimeCfg.FactoryConfig(), "agent-worker")
	if err != nil {
		t.Fatalf("BuildModelInvocationExecutor() error = %v", err)
	}

	result, err := executor.Execute(context.Background(), agentInvocationExecutionRequest("provider-dispatch-failure-1"))
	if err != nil {
		t.Fatalf("Execute() error = %v, want a failed WorkResult rather than a transport error", err)
	}
	if result.Outcome != workers.OutcomeFailed {
		t.Fatalf("Execute() result = %#v, want a failed outcome for non-zero exit", result)
	}
	if runner.calls() != 1 {
		t.Fatalf("provider CommandRunner calls = %d, want exactly 1 (no retry for a plain invalid-request failure)", runner.calls())
	}
	if len(published) != 1 || published[0].Kind != workers.FailedFragmentKind {
		t.Fatalf("published fragments = %#v, want exactly one terminal failure fragment", published)
	}
}

// TestConcurrentRepeatedProviderExecutionsRetainConstructionInjectedProviderCommandRunner
// extends the story-003 race/repeat proof to the provider command runner
// itself: concurrent, repeated provider-backed dispatches on one constructed
// runtime must all reach the exact construction-injected provider command
// runner (proved by exact call-count and per-dispatch request identity),
// with per-dispatch progress isolation. Synchronization is limited to
// sync.WaitGroup/atomic/the runner's own mutex (no sleeps).
// pkgmaintcheck:ignore-cyclomatic-complexity pre-existing baseline debt recorded 2026-08-08; refactor this code below the maintainability threshold and remove this exemption
func TestConcurrentRepeatedProviderExecutionsRetainConstructionInjectedProviderCommandRunner(t *testing.T) {
	t.Parallel()

	runner := &recordingProviderCommandRunner{stdout: []byte("concurrent provider output")}
	providersService := commandRunnerBackedProvidersService{runner: runner}
	recorder := newRecordingDispatchPublisher()
	service := providerInvocationRuntimeService(runner, providersService, workers.ProgressPublisher(recorder.publish))
	runtimeCfg := agentInvocationRuntimeConfig()

	for round := 0; round < concurrencyRepeatRounds; round++ {
		var wg sync.WaitGroup
		var executeErrors atomic.Int32
		for worker := 0; worker < concurrencyFanout; worker++ {
			wg.Add(1)
			go func(worker int) {
				defer wg.Done()
				if service.ProviderCommandRunner() != workers.CommandRunner(runner) {
					executeErrors.Add(1)
					t.Errorf("round %d worker %d: ProviderCommandRunner() observed a substituted or missing runner", round, worker)
					return
				}
				executor, err := service.BuildModelInvocationExecutor(runtimeCfg, runtimeCfg.FactoryConfig(), "agent-worker")
				if err != nil {
					executeErrors.Add(1)
					t.Errorf("round %d worker %d: BuildModelInvocationExecutor() error = %v", round, worker, err)
					return
				}
				dispatchID := fmt.Sprintf("provider-dispatch-r%d-w%d", round, worker)
				result, err := executor.Execute(context.Background(), agentInvocationExecutionRequest(dispatchID))
				if err != nil || result.Outcome != workers.OutcomeAccepted || result.Output != "concurrent provider output" {
					executeErrors.Add(1)
					t.Errorf("round %d worker %d: Execute() = %#v, err = %v, want accepted concurrent provider output", round, worker, result, err)
				}
			}(worker)
		}
		wg.Wait()
		if executeErrors.Load() != 0 {
			t.Fatalf("round %d: %d concurrent provider executions failed", round, executeErrors.Load())
		}

		snapshot := recorder.snapshot()
		for worker := 0; worker < concurrencyFanout; worker++ {
			dispatchID := fmt.Sprintf("provider-dispatch-r%d-w%d", round, worker)
			fragments := snapshot[dispatchID]
			if len(fragments) != 2 ||
				fragments[0].DispatchID != dispatchID || fragments[0].Kind != workers.ProgressFragmentKind ||
				fragments[1].DispatchID != dispatchID || fragments[1].Kind != workers.CompletedFragmentKind {
				t.Fatalf("round %d worker %d: published fragments = %#v, want exactly one progress fact and one completion for %s (no cross-request leakage or duplicate delivery)", round, worker, fragments, dispatchID)
			}
		}
	}

	wantCalls := concurrencyRepeatRounds * concurrencyFanout
	if got := runner.calls(); got != wantCalls {
		t.Fatalf("provider CommandRunner calls = %d, want exactly %d (every concurrent/repeated dispatch reached the construction-injected provider runner exactly once)", got, wantCalls)
	}
	seenAttemptIDs := make(map[string]struct{}, wantCalls)
	for _, request := range runner.requests() {
		if _, exists := seenAttemptIDs[request.DispatchID]; exists {
			t.Fatalf("provider CommandRunner observed duplicate attempt ID %q", request.DispatchID)
		}
		seenAttemptIDs[request.DispatchID] = struct{}{}
	}
	if len(seenAttemptIDs) != wantCalls {
		t.Fatalf("distinct provider attempt IDs observed = %d, want %d", len(seenAttemptIDs), wantCalls)
	}
}
