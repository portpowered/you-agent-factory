package service

import (
	"context"
	"errors"
	"strings"
	"sync"
	"testing"

	platformclock "github.com/portpowered/infinite-you/pkg/platform/clock"
	"github.com/portpowered/infinite-you/pkg/platform/logging"
	eventswire "github.com/portpowered/infinite-you/pkg/services/events/wire"
	modelinference "github.com/portpowered/infinite-you/pkg/services/models"
	providersessions "github.com/portpowered/infinite-you/pkg/services/provider_sessions"
	"github.com/portpowered/infinite-you/pkg/services/providers"
	providerswire "github.com/portpowered/infinite-you/pkg/services/providers/wire"
	"github.com/portpowered/infinite-you/pkg/services/work"
	workersessions "github.com/portpowered/infinite-you/pkg/services/worker_sessions"
	workersessionswire "github.com/portpowered/infinite-you/pkg/services/worker_sessions/wire"
	"github.com/portpowered/infinite-you/pkg/services/workers"
	workerprocess "github.com/portpowered/infinite-you/pkg/services/workers/internal/services/runners/process"
)

type unavailableProviderSessions struct {
	providersessions.Service
}

func (unavailableProviderSessions) Project(providersessions.ProjectRequest) (providersessions.ProjectResult, error) {
	return providersessions.ProjectResult{}, providersessions.ErrSessionStorageUnavailable
}

// TestLiveProviderSessionObservationEnablesExactWorkerSessionContinuation
// composes the real Providers root, native Codex streaming adapter, Agent
// runner, and Worker Sessions bridge. The command runner is the controlled
// external edge: it reports a provider-authored thread while live, then
// verifies Resume reaches the exact thread through Providers.Continue.
// pkgmaintcheck:ignore-cyclomatic-complexity pre-existing baseline debt recorded 2026-08-08; refactor this code below the maintainability threshold and remove this exemption
func TestLiveProviderSessionObservationEnablesExactWorkerSessionContinuation(t *testing.T) {
	command := newLiveSessionCommandRunner()
	providerService, err := providerswire.NewService(
		providerswire.WithWorkersCommandRunner(command),
		providerswire.WithLogger(logging.NoopLogger{}),
	)
	if err != nil {
		t.Fatalf("providers wire NewService() error = %v", err)
	}
	bridge := workersessions.NewProviderSessionObservationPublisher(nil)
	runner, err := New(providerService, bridge.Publish)
	if err != nil {
		t.Fatalf("agent New() error = %v", err)
	}
	eventsService, err := eventswire.NewService(logging.NoopLogger{})
	if err != nil {
		t.Fatalf("events wire NewService() error = %v", err)
	}
	service := newLiveSessionService(runner)
	sessions, err := workersessionswire.NewService(service, eventsService, logging.NoopLogger{}, platformclock.Real{}, unavailableProviderSessions{}, nil)
	if err != nil {
		t.Fatalf("Worker Sessions wire NewService() error = %v", err)
	}
	bridge.Bind(sessions)

	type startOutcome struct {
		result workersessions.InvokeSessionResult
		err    error
	}
	started := make(chan startOutcome, 1)
	go func() {
		result, err := sessions.InvokeSession(context.Background(), liveSessionStartRequest())
		started <- startOutcome{result: result, err: err}
	}()

	// The controlled edge signals only after the real streaming decoder has
	// invoked ExecuteRequest.SessionObserver and the bridge returned.
	<-command.initialSessionObserved
	beforePause, err := sessions.Get(context.Background(), workersessions.GetRequest{ID: "worker-live-provider-session"})
	if err != nil {
		t.Fatalf("Get() before Pause error = %v", err)
	}
	want := providers.SessionRef{Provider: providers.IDCodex, Kind: providers.SessionIDKind, ID: "codex-live-thread-1"}
	if beforePause.State != workersessions.StateRunning || beforePause.ProviderSessionAssociation == nil || beforePause.ProviderSessionAssociation.Reference != want {
		t.Fatalf("live Worker Session = %#v, want RUNNING with exact reference %#v", beforePause, want)
	}

	paused, err := sessions.Pause(context.Background(), workersessions.ControlRequest{ID: beforePause.ID})
	if err != nil || paused.Outcome != workersessions.ControlOutcomeApplied || paused.Session.State != workersessions.StatePaused {
		t.Fatalf("Pause() = (%#v, %v), want applied PAUSED", paused, err)
	}
	resumed, err := sessions.Resume(context.Background(), workersessions.ControlRequest{ID: beforePause.ID})
	if err != nil || resumed.Outcome != workersessions.ControlOutcomeApplied {
		t.Fatalf("Resume() = (%#v, %v), want applied", resumed, err)
	}
	completed := <-started
	if completed.err != nil {
		t.Fatalf("Start() error = %v", completed.err)
	}
	if completed.result.Session.State != workersessions.StateCompleted || completed.result.Session.ProviderSessionAssociation == nil || completed.result.Session.ProviderSessionAssociation.Reference != want {
		t.Fatalf("Start() final session = %#v, want COMPLETED with retained exact reference %#v", completed.result.Session, want)
	}
	if args := command.resumeArgs(); !containsLiveSessionSequence(args, "resume", want.ID) {
		t.Fatalf("continued Codex args = %#v, want exact resume identity %q", args, want.ID)
	}
}

func liveSessionStartRequest() workersessions.InvokeSessionRequest {
	return workersessions.InvokeSessionRequest{
		ID: "worker-live-provider-session",
		Execution: workers.WorkstationDispatchRequest{
			WorkstationName: "review",
			Execution: workers.WorkstationExecutionRequest{
				Dispatch: work.WorkDispatch{
					DispatchID: "dispatch-live-provider-session", WorkerType: "reviewer", WorkstationName: "review",
					Execution: work.ExecutionMetadata{RequestID: "factory-turn-live-provider-session"},
				},
				RunnerID:        workers.RunnerIDCodex,
				WorkerType:      "reviewer",
				WorkstationType: "review",
				SystemPrompt:    "review the request",
				UserMessage:     "continue the existing provider session",
			},
		},
	}
}

type liveSessionCommandRunner struct {
	initialSessionObserved chan struct{}
	mu                     sync.Mutex
	resume                 workerprocess.CommandRequest
}

func newLiveSessionCommandRunner() *liveSessionCommandRunner {
	return &liveSessionCommandRunner{initialSessionObserved: make(chan struct{})}
}

func (r *liveSessionCommandRunner) Run(ctx context.Context, request workerprocess.CommandRequest) (workerprocess.CommandResult, error) {
	return r.RunStreaming(ctx, request, nil)
}

func (r *liveSessionCommandRunner) RunStreaming(
	ctx context.Context,
	request workerprocess.CommandRequest,
	observe workerprocess.OutputChunkObserver,
) (workerprocess.CommandResult, error) {
	if containsLiveSessionSequence(request.Args, "resume", "codex-live-thread-1") {
		r.mu.Lock()
		r.resume = request
		r.resume.Args = append([]string(nil), request.Args...)
		r.mu.Unlock()
		emitLiveSessionChunk(observe, `{"type":"thread.started","thread_id":"codex-live-thread-1"}`+"\n")
		emitLiveSessionChunk(observe, `{"type":"item.completed","item":{"id":"message-resumed","type":"agent_message","text":"resumed exact output"}}`+"\n")
		return workerprocess.CommandResult{}, nil
	}
	emitLiveSessionChunk(observe, `{"type":"thread.started","thread_id":"codex-live-thread-1"}`+"\n")
	close(r.initialSessionObserved)
	<-ctx.Done()
	return workerprocess.CommandResult{}, ctx.Err()
}

func (r *liveSessionCommandRunner) resumeArgs() []string {
	r.mu.Lock()
	defer r.mu.Unlock()
	return append([]string(nil), r.resume.Args...)
}

func emitLiveSessionChunk(observe workerprocess.OutputChunkObserver, payload string) {
	if observe != nil {
		observe(workerprocess.OutputStreamStdout, []byte(payload))
	}
}

type liveSessionService struct {
	runner interface {
		Execute(context.Context, workers.RunnerExecutionRequest) (workers.RunnerExecutionResult, error)
	}
}

func newLiveSessionService(runner interface {
	Execute(context.Context, workers.RunnerExecutionRequest) (workers.RunnerExecutionResult, error)
}) workers.Service {
	return &liveSessionService{runner: runner}
}

func (s *liveSessionService) Execute(ctx context.Context, request workers.ExecuteRequest) (workers.ExecuteResult, error) {
	response, attemptErr := s.runner.Execute(ctx, liveSessionRunnerRequest(request))
	result := workers.ExecuteResult{
		Correlation: request.Correlation,
		Outcome:     workers.ExecutionOutcomeAccepted,
		Output: workers.ProposedOutput{Primary: []work.WorkContentPart{{
			Type: work.WorkContentPartTypeText,
			Text: response.Content,
		}}},
		Continuation: cloneContinuation(response.Continuation),
	}
	if attemptErr != nil {
		result.Outcome = workers.ExecutionOutcomeFailed
		if errors.Is(attemptErr, context.Canceled) {
			result.Outcome = workers.ExecutionOutcomeCanceled
		}
	}
	return result, attemptErr
}

func (s *liveSessionService) InvokeModel(context.Context, string, modelinference.Request) (modelinference.Result, error) {
	return modelinference.Result{}, workers.ErrExecuteUnavailable
}

func liveSessionRunnerRequest(request workers.ExecuteRequest) workers.RunnerExecutionRequest {
	return workers.RunnerExecutionRequest{
		Dispatch:           work.CloneWorkDispatch(request.Input.Dispatch),
		WorkerType:         request.Target.WorkerType,
		WorkstationType:    request.Target.WorkstationName,
		RunnerID:           request.Target.RunnerID,
		Model:              request.Target.Model.Name,
		ReasoningEffort:    request.Target.Model.ReasoningEffort,
		SystemPrompt:       request.Target.Prompt.SystemPrompt,
		UserMessage:        request.Target.Prompt.UserMessage,
		OutputSchema:       request.Target.Prompt.OutputSchema,
		WorkingDirectory:   request.Target.Environment.WorkingDirectory,
		Worktree:           request.Target.Workspace.Worktree,
		ProcessEnvironment: append([]string(nil), request.Target.Environment.ProcessEnvironment...),
		EnvVars:            cloneLiveSessionStringMap(request.Target.Environment.Vars),
		Continuation:       (request.Input.Resume).ClonePtr(),
	}
}

func cloneLiveSessionStringMap(value map[string]string) map[string]string {
	if len(value) == 0 {
		return nil
	}
	clone := make(map[string]string, len(value))
	for key, item := range value {
		clone[key] = item
	}
	return clone
}

func containsLiveSessionSequence(values []string, first, second string) bool {
	for index := 0; index+1 < len(values); index++ {
		if strings.TrimSpace(values[index]) == first && strings.TrimSpace(values[index+1]) == second {
			return true
		}
	}
	return false
}
