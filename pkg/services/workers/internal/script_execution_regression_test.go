package internal

import (
	"context"
	"fmt"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/portpowered/infinite-you/pkg/platform/logging"
	interfaces "github.com/portpowered/infinite-you/pkg/services/factory_definitions"
	"github.com/portpowered/infinite-you/pkg/services/workers"
	executeservice "github.com/portpowered/infinite-you/pkg/services/workers/internal/service"
	"github.com/portpowered/infinite-you/pkg/services/workers/internal/services/runners"
	runnerswire "github.com/portpowered/infinite-you/pkg/services/workers/internal/services/runners/wire"
)

var testFactoryDocsLoader workers.FactoryDocsLoader = func(string) (map[string]string, error) {
	return nil, nil
}

// recordingScriptCommandRunner is a thread-safe fake implementing both
// workers.CommandRunner and the streaming edge the Script Runner requires.
// The tests below exercise the request-scoped Workers service, so every
// assertion proves the injected command effect is reached without a
// per-worker Workstation adapter.
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

func scriptInvocationWorkersService(
	t *testing.T,
	scriptRunner workers.CommandRunner,
	publisher workers.ProgressPublisher,
) *executeservice.Service {
	t.Helper()
	registry, err := runnerswire.NewScriptRegistry(
		runners.ScriptConfig{RequestSelected: true},
		runners.ScriptDependencies{
			CommandRunner: scriptRunner,
			FactoryDocs:   testFactoryDocsLoader,
			Now:           time.Now,
			Publish:       publisher,
			Record:        func(workers.ScriptEvent) {},
		},
	)
	if err != nil {
		t.Fatalf("NewScriptRegistry() error = %v", err)
	}
	service, err := executeservice.New(
		registry,
		nil,
		nil,
		logging.NoopLogger{},
		time.Now,
		nil,
		nil,
		nil,
		testFactoryDocsLoader,
	)
	if err != nil {
		t.Fatalf("construct request-scoped Workers service: %v", err)
	}
	return service
}

func scriptExecutionRequest(dispatchID string) workers.ExecuteRequest {
	return workers.ExecuteRequest{
		Correlation: workers.ExecutionCorrelation{
			FactorySessionID: "session-1",
			RuntimeID:        "runtime-1",
			GenerationID:     "generation-1",
			DispatchID:       dispatchID,
			AttemptID:        dispatchID + "-attempt",
			RequestID:        dispatchID + "-request",
			TraceID:          dispatchID + "-trace",
		},
		Target: workers.ExecutionTarget{
			WorkerName:      "script-worker",
			WorkerType:      interfaces.WorkerTypeScript,
			WorkstationName: "script-workstation",
			RunnerID:        runners.ScriptIdentity,
			Command:         "test-script",
		},
	}
}

func scriptResultOutput(result workers.ExecuteResult) string {
	if len(result.Output.Primary) == 0 {
		return ""
	}
	return result.Output.Primary[0].Text
}

// TestScriptExecutionThroughRequestScopedWorkersServiceDeliversSuccess proves
// script dispatch reaches the injected command runner through Service.Execute,
// preserves final stdout, and emits streamed progress with the dispatch's
// correlation.
func TestScriptExecutionThroughRequestScopedWorkersServiceDeliversSuccess(t *testing.T) {
	t.Parallel()

	runner := &recordingScriptCommandRunner{stdout: []byte("script output")}
	var published []workers.ProgressFragment
	publisher := workers.ProgressPublisher(func(fragment workers.ProgressFragment) {
		published = append(published, cloneServiceProgressFragment(fragment))
	})

	service := scriptInvocationWorkersService(t, runner, publisher)
	request := scriptExecutionRequest("script-dispatch-success-1")
	result, err := service.Execute(context.Background(), request)
	if err != nil {
		t.Fatalf("Execute() error = %v", err)
	}
	if result.Outcome != workers.ExecutionOutcomeAccepted || scriptResultOutput(result) != "script output" {
		t.Fatalf("Execute() result = %#v, want accepted script output", result)
	}
	if runner.calls() != 1 {
		t.Fatalf("script CommandRunner calls = %d, want exactly 1", runner.calls())
	}
	if len(published) != 1 || published[0].Correlation.DispatchID != "script-dispatch-success-1" {
		t.Fatalf("published fragments = %#v, want exactly one progress fragment for script-dispatch-success-1", published)
	}
}

// TestScriptExecutionThroughRequestScopedWorkersServiceDeliversFailure proves
// a non-zero script exit remains a terminal failed ExecuteResult while still
// reaching the injected command runner exactly once.
func TestScriptExecutionThroughRequestScopedWorkersServiceDeliversFailure(t *testing.T) {
	t.Parallel()

	runner := &recordingScriptCommandRunner{stdout: []byte("partial output"), exitCode: 1}
	var published []workers.ProgressFragment
	publisher := workers.ProgressPublisher(func(fragment workers.ProgressFragment) {
		published = append(published, cloneServiceProgressFragment(fragment))
	})

	service := scriptInvocationWorkersService(t, runner, publisher)
	request := scriptExecutionRequest("script-dispatch-failure-1")
	result, err := service.Execute(context.Background(), request)
	if err != nil {
		t.Fatalf("Execute() error = %v, want a normalized failed result", err)
	}
	if result.Outcome != workers.ExecutionOutcomeFailed || result.Failure == nil {
		t.Fatalf("Execute() result = %#v, want a failed terminal outcome", result)
	}
	if runner.calls() != 1 {
		t.Fatalf("script CommandRunner calls = %d, want exactly 1", runner.calls())
	}
	if len(published) != 1 || published[0].Correlation.DispatchID != "script-dispatch-failure-1" {
		t.Fatalf("published fragments = %#v, want exactly one progress fragment for script-dispatch-failure-1", published)
	}
}

// TestConcurrentRepeatedScriptExecutionsThroughRequestScopedWorkersService
// proves repeated detached requests retain independent correlation and
// progress state while all reach the same injected command runner exactly once.
func TestConcurrentRepeatedScriptExecutionsThroughRequestScopedWorkersService(t *testing.T) {
	t.Parallel()

	runner := &recordingScriptCommandRunner{stdout: []byte("concurrent script output")}
	recorder := newRecordingDispatchPublisher()
	service := scriptInvocationWorkersService(t, runner, workers.ProgressPublisher(recorder.publish))

	for round := 0; round < concurrencyRepeatRounds; round++ {
		var wg sync.WaitGroup
		var executeErrors atomic.Int32
		for worker := 0; worker < concurrencyFanout; worker++ {
			wg.Add(1)
			go func(worker int) {
				defer wg.Done()
				dispatchID := fmt.Sprintf("script-dispatch-r%d-w%d", round, worker)
				result, err := service.Execute(context.Background(), scriptExecutionRequest(dispatchID))
				if err != nil || result.Outcome != workers.ExecutionOutcomeAccepted || scriptResultOutput(result) != "concurrent script output" {
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
			if len(fragments) != 1 || fragments[0].Correlation.DispatchID != dispatchID {
				t.Fatalf("round %d worker %d: published fragments = %#v, want exactly one for %s", round, worker, fragments, dispatchID)
			}
		}
	}

	wantCalls := concurrencyRepeatRounds * concurrencyFanout
	if got := runner.calls(); got != wantCalls {
		t.Fatalf("script CommandRunner calls = %d, want exactly %d", got, wantCalls)
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
