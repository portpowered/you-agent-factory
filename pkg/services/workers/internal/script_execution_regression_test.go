package internal

import (
	"context"
	"fmt"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/portpowered/infinite-you/internal/testutil/runtimefixtures"
	platformfilesystem "github.com/portpowered/infinite-you/pkg/platform/filesystem"
	"github.com/portpowered/infinite-you/pkg/platform/logging"
	interfaces "github.com/portpowered/infinite-you/pkg/services/factory_definitions"
	"github.com/portpowered/infinite-you/pkg/services/work"
	"github.com/portpowered/infinite-you/pkg/services/workers"
	workerconstruction "github.com/portpowered/infinite-you/pkg/services/workers/internal/services/runtime_assembly/construction"
	workerexecutor "github.com/portpowered/infinite-you/pkg/services/workers/internal/services/workstations/executor"
	workeragentrun "github.com/portpowered/infinite-you/pkg/services/workers/internal/services/workstations/executor/agentrun"
	"go.uber.org/zap"
)

// recordingScriptCommandRunner is a thread-safe fake implementing both
// workers.CommandRunner and the streaming edge the Script Runner requires, so
// script-backed dispatch actually exercises the construction-injected script
// command runner (rather than going through providers.Service, as the
// AGENT/MODEL/INFERENCE path does).
type recordingScriptCommandRunner struct {
	mu       sync.Mutex
	callList []workers.CommandRequest
	stdout   []byte
	exitCode int
}

func (r *recordingScriptCommandRunner) Run(
	ctx context.Context,
	request workers.CommandRequest,
) (workers.CommandResult, error) {
	return r.RunStreaming(ctx, request, nil)
}

func (r *recordingScriptCommandRunner) RunStreaming(
	_ context.Context,
	request workers.CommandRequest,
	observer workers.OutputChunkObserver,
) (workers.CommandResult, error) {
	r.mu.Lock()
	r.callList = append(r.callList, request)
	r.mu.Unlock()
	if observer != nil && len(r.stdout) > 0 {
		observer("stdout", append([]byte(nil), r.stdout...))
	}
	return workers.CommandResult{Stdout: append([]byte(nil), r.stdout...), ExitCode: r.exitCode}, nil
}

func (r *recordingScriptCommandRunner) calls() int {
	r.mu.Lock()
	defer r.mu.Unlock()
	return len(r.callList)
}

func (r *recordingScriptCommandRunner) requests() []workers.CommandRequest {
	r.mu.Lock()
	defer r.mu.Unlock()
	return append([]workers.CommandRequest(nil), r.callList...)
}

// scriptInvocationRuntimeService constructs a minimal *Service whose
// executorBuilder is wired to a real *workerexecutor.ScriptFactory built from
// the supplied script command runner, so Script-type worker construction and
// execution reach the exact construction-injected script command runner and
// progress publisher, not a stand-in.
func scriptInvocationRuntimeService(t *testing.T, scriptRunner workers.CommandRunner, publisher workers.ProgressPublisher) *Service {
	t.Helper()
	scriptFactory, err := workerexecutor.NewScriptFactory(scriptRunner, workers.ClockFunc(time.Now), testFactoryDocsLoader)
	if err != nil {
		t.Fatalf("NewScriptFactory() error = %v", err)
	}
	executorBuilder := workerconstruction.New(
		nil,
		scriptFactory,
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
		scriptFactory:           scriptFactory,
		scriptCommandRunner:     scriptRunner,
		progressPublisher:       publisher,
		logger:                  zap.NewNop(),
		clock:                   time.Now,
		processEnvironment:      func() []string { return nil },
		currentWorkingDirectory: func() (string, error) { return "", nil },
	}
}

func scriptInvocationRuntimeConfig() runtimefixtures.RuntimeConfigLookupFixture {
	worker := &interfaces.FactoryWorkerConfig{
		Name:    "script-worker",
		Type:    interfaces.WorkerTypeScript,
		Command: "test-script",
	}
	return runtimefixtures.RuntimeConfigLookupFixture{
		Workers: map[string]*interfaces.FactoryWorkerConfig{worker.Name: worker},
		Factory: &interfaces.FactoryConfig{},
	}
}

func buildScriptExecutor(t *testing.T, service *Service, runtimeCfg interfaces.RuntimeConfigLookup) workers.WorkstationRequestExecutor {
	t.Helper()
	result, err := service.executorBuilder.Build(
		runtimeCfg, "script-worker", "", nil,
		logging.NewZapLogger(service.logger, false),
		nil, nil,
		service.progressPublisher, nil, nil, nil,
		service.clock, service.processEnvironment, service.currentWorkingDirectory,
		nil,
	)
	if err != nil {
		t.Fatalf("executorBuilder.Build() error = %v", err)
	}
	if result.Direct == nil {
		t.Fatalf("executorBuilder.Build() Direct = nil, want script executor")
	}
	return result.Direct
}

func scriptExecutionRequest(dispatchID string) workers.WorkstationExecutionRequest {
	return workers.WorkstationExecutionRequest{
		Dispatch:   work.WorkDispatch{DispatchID: dispatchID, WorkerType: "script-worker"},
		WorkerType: "script-worker",
	}
}

// TestBuildScriptExecutorDeliversSuccessThroughConstructionInjectedScriptRunner
// proves script-backed dispatch actually invokes the construction-injected
// script command runner (not just providers.Service, as the AGENT/MODEL path
// does) and delivers streamed output as progress through the
// construction-injected publisher.
func TestBuildScriptExecutorDeliversSuccessThroughConstructionInjectedScriptRunner(t *testing.T) {
	t.Parallel()

	runner := &recordingScriptCommandRunner{stdout: []byte("script output")}
	var published []workers.ProgressFragment
	publisher := workers.ProgressPublisher(func(fragment workers.ProgressFragment) {
		published = append(published, cloneServiceProgressFragment(fragment))
	})

	service := scriptInvocationRuntimeService(t, runner, publisher)
	runtimeCfg := scriptInvocationRuntimeConfig()
	executor := buildScriptExecutor(t, service, runtimeCfg)

	result, err := executor.Execute(context.Background(), scriptExecutionRequest("script-dispatch-success-1"))
	if err != nil {
		t.Fatalf("Execute() error = %v", err)
	}
	if result.Outcome != workers.OutcomeAccepted || result.Output != "script output" {
		t.Fatalf("Execute() result = %#v, want accepted script output", result)
	}
	if runner.calls() != 1 {
		t.Fatalf("script CommandRunner calls = %d, want exactly 1 (construction-injected runner reached)", runner.calls())
	}
	if len(published) != 1 || published[0].DispatchID != "script-dispatch-success-1" {
		t.Fatalf("published fragments = %#v, want exactly one progress fragment for script-dispatch-success-1", published)
	}
}

// TestBuildScriptExecutorDeliversFailureThroughConstructionInjectedScriptRunner
// proves a non-zero script exit surfaces as a failed WorkResult while still
// reaching the exact construction-injected script command runner exactly once
// (no retry loop for a plain non-zero exit).
func TestBuildScriptExecutorDeliversFailureThroughConstructionInjectedScriptRunner(t *testing.T) {
	t.Parallel()

	runner := &recordingScriptCommandRunner{stdout: []byte("partial output"), exitCode: 1}
	var published []workers.ProgressFragment
	publisher := workers.ProgressPublisher(func(fragment workers.ProgressFragment) {
		published = append(published, cloneServiceProgressFragment(fragment))
	})

	service := scriptInvocationRuntimeService(t, runner, publisher)
	runtimeCfg := scriptInvocationRuntimeConfig()
	executor := buildScriptExecutor(t, service, runtimeCfg)

	result, err := executor.Execute(context.Background(), scriptExecutionRequest("script-dispatch-failure-1"))
	if err != nil {
		t.Fatalf("Execute() error = %v, want a failed WorkResult rather than a transport error", err)
	}
	if result.Outcome != workers.OutcomeFailed {
		t.Fatalf("Execute() result = %#v, want a failed outcome for non-zero exit", result)
	}
	if runner.calls() != 1 {
		t.Fatalf("script CommandRunner calls = %d, want exactly 1", runner.calls())
	}
	if len(published) != 1 || published[0].DispatchID != "script-dispatch-failure-1" {
		t.Fatalf("published fragments = %#v, want exactly one progress fragment for script-dispatch-failure-1", published)
	}
}

// TestConcurrentRepeatedScriptExecutionsRetainConstructionInjectedScriptRunner
// extends the story-003 race/repeat proof to the script command runner itself:
// concurrent, repeated script-backed dispatches on one constructed runtime
// must all reach the exact construction-injected script command runner
// (proved by exact call-count and per-dispatch request identity), with
// per-dispatch progress isolation and no duplicate delivery. Synchronization
// is limited to sync.WaitGroup/atomic/the runner's own mutex (no sleeps).
func TestConcurrentRepeatedScriptExecutionsRetainConstructionInjectedScriptRunner(t *testing.T) {
	t.Parallel()

	runner := &recordingScriptCommandRunner{stdout: []byte("concurrent script output")}
	recorder := newRecordingDispatchPublisher()
	service := scriptInvocationRuntimeService(t, runner, workers.ProgressPublisher(recorder.publish))
	runtimeCfg := scriptInvocationRuntimeConfig()

	for round := 0; round < concurrencyRepeatRounds; round++ {
		var wg sync.WaitGroup
		var executeErrors atomic.Int32
		for worker := 0; worker < concurrencyFanout; worker++ {
			wg.Add(1)
			go func(worker int) {
				defer wg.Done()
				executor := buildScriptExecutor(t, service, runtimeCfg)
				dispatchID := fmt.Sprintf("script-dispatch-r%d-w%d", round, worker)
				result, err := executor.Execute(context.Background(), scriptExecutionRequest(dispatchID))
				if err != nil || result.Outcome != workers.OutcomeAccepted || result.Output != "concurrent script output" {
					executeErrors.Add(1)
					t.Errorf("round %d worker %d: Execute() = %#v, err = %v, want accepted concurrent script output", round, worker, result, err)
				}
			}(worker)
		}
		wg.Wait()
		if executeErrors.Load() != 0 {
			t.Fatalf("round %d: %d concurrent script executions failed", round, executeErrors.Load())
		}

		snapshot := recorder.snapshot()
		for worker := 0; worker < concurrencyFanout; worker++ {
			dispatchID := fmt.Sprintf("script-dispatch-r%d-w%d", round, worker)
			fragments := snapshot[dispatchID]
			if len(fragments) != 1 || fragments[0].DispatchID != dispatchID {
				t.Fatalf("round %d worker %d: published fragments = %#v, want exactly one for %s (no cross-request leakage or duplicate delivery)", round, worker, fragments, dispatchID)
			}
		}
	}

	wantCalls := concurrencyRepeatRounds * concurrencyFanout
	if got := runner.calls(); got != wantCalls {
		t.Fatalf("script CommandRunner calls = %d, want exactly %d (every concurrent/repeated dispatch reached the construction-injected script runner exactly once)", got, wantCalls)
	}
	seenDispatchIDs := make(map[string]struct{}, wantCalls)
	for _, request := range runner.requests() {
		if _, exists := seenDispatchIDs[request.DispatchID]; exists {
			t.Fatalf("script CommandRunner observed duplicate dispatch ID %q", request.DispatchID)
		}
		seenDispatchIDs[request.DispatchID] = struct{}{}
	}
	if len(seenDispatchIDs) != wantCalls {
		t.Fatalf("distinct script dispatch IDs observed = %d, want %d", len(seenDispatchIDs), wantCalls)
	}
}
