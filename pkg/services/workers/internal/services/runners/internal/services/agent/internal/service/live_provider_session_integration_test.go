package service

import (
	"context"
	"errors"
	"strings"
	"sync"
	"testing"

	"github.com/portpowered/infinite-you/pkg/platform/logging"
	eventswire "github.com/portpowered/infinite-you/pkg/services/events/wire"
	"github.com/portpowered/infinite-you/pkg/services/providers"
	providerswire "github.com/portpowered/infinite-you/pkg/services/providers/wire"
	"github.com/portpowered/infinite-you/pkg/services/work"
	workersessions "github.com/portpowered/infinite-you/pkg/services/worker_sessions"
	workersessionswire "github.com/portpowered/infinite-you/pkg/services/worker_sessions/wire"
	"github.com/portpowered/infinite-you/pkg/services/workers"
)

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
	boundary := newLiveSessionBoundary(runner)
	sessions, err := workersessionswire.NewService(boundary, eventsService, logging.NoopLogger{}, nil)
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
	resume                 workers.CommandRequest
}

func newLiveSessionCommandRunner() *liveSessionCommandRunner {
	return &liveSessionCommandRunner{initialSessionObserved: make(chan struct{})}
}

func (r *liveSessionCommandRunner) Run(ctx context.Context, request workers.CommandRequest) (workers.CommandResult, error) {
	return r.RunStreaming(ctx, request, nil)
}

func (r *liveSessionCommandRunner) RunStreaming(
	ctx context.Context,
	request workers.CommandRequest,
	observe workers.OutputChunkObserver,
) (workers.CommandResult, error) {
	if containsLiveSessionSequence(request.Args, "resume", "codex-live-thread-1") {
		r.mu.Lock()
		r.resume = request
		r.resume.Args = append([]string(nil), request.Args...)
		r.mu.Unlock()
		emitLiveSessionChunk(observe, `{"type":"thread.started","thread_id":"codex-live-thread-1"}`+"\n")
		emitLiveSessionChunk(observe, `{"type":"item.completed","item":{"id":"message-resumed","type":"agent_message","text":"resumed exact output"}}`+"\n")
		return workers.CommandResult{}, nil
	}
	emitLiveSessionChunk(observe, `{"type":"thread.started","thread_id":"codex-live-thread-1"}`+"\n")
	close(r.initialSessionObserved)
	<-ctx.Done()
	return workers.CommandResult{}, ctx.Err()
}

func (r *liveSessionCommandRunner) resumeArgs() []string {
	r.mu.Lock()
	defer r.mu.Unlock()
	return append([]string(nil), r.resume.Args...)
}

func emitLiveSessionChunk(observe workers.OutputChunkObserver, payload string) {
	if observe != nil {
		observe(workers.OutputStreamStdout, []byte(payload))
	}
}

type liveSessionBoundary struct {
	runner interface {
		Execute(context.Context, workers.RunnerExecutionRequest) (workers.RunnerExecutionResult, error)
	}
	mu      sync.Mutex
	cancels map[string]context.CancelFunc
}

func newLiveSessionBoundary(runner interface {
	Execute(context.Context, workers.RunnerExecutionRequest) (workers.RunnerExecutionResult, error)
}) *liveSessionBoundary {
	return &liveSessionBoundary{runner: runner, cancels: make(map[string]context.CancelFunc)}
}

func (*liveSessionBoundary) Start(context.Context) error { return nil }

func (b *liveSessionBoundary) Publish(ctx context.Context, request workers.WorkstationDispatchRequest, accept workers.WorkstationDispatchAcceptFunc) error {
	return b.PublishWithAdmission(ctx, request, nil, accept)
}

func (b *liveSessionBoundary) PublishWithAdmission(
	ctx context.Context,
	request workers.WorkstationDispatchRequest,
	admitted workers.WorkstationDispatchAdmissionFunc,
	accept workers.WorkstationDispatchAcceptFunc,
) error {
	dispatchID := request.Execution.Dispatch.DispatchID
	attemptCtx, cancel := context.WithCancel(context.WithoutCancel(ctx))
	b.mu.Lock()
	b.cancels[dispatchID] = cancel
	b.mu.Unlock()
	if admitted != nil {
		admitted()
	}
	go func() {
		response, attemptErr := b.runner.Execute(attemptCtx, liveSessionRunnerRequest(request))
		accept(context.Background(), request, liveSessionDispatchResult(request, response, attemptErr), attemptErr)
		b.mu.Lock()
		delete(b.cancels, dispatchID)
		b.mu.Unlock()
	}()
	return nil
}

func (b *liveSessionBoundary) Cancel(_ context.Context, request workers.WorkstationDispatchCancelRequest) (workers.WorkstationDispatchCancelResult, error) {
	b.mu.Lock()
	cancel := b.cancels[request.DispatchID]
	b.mu.Unlock()
	if cancel == nil {
		return workers.WorkstationDispatchCancelResult{DispatchID: request.DispatchID, Outcome: workers.WorkstationDispatchCancelOutcomeAlreadyTerminal}, workers.ErrWorkstationDispatchAlreadyTerminal
	}
	cancel()
	return workers.WorkstationDispatchCancelResult{DispatchID: request.DispatchID, Outcome: workers.WorkstationDispatchCancelOutcomeCanceled}, nil
}

func (*liveSessionBoundary) Stop(context.Context) error { return nil }

func liveSessionRunnerRequest(request workers.WorkstationDispatchRequest) workers.RunnerExecutionRequest {
	execution := request.Execution
	return workers.RunnerExecutionRequest{
		Dispatch:           work.CloneWorkDispatch(execution.Dispatch),
		WorkerType:         execution.WorkerType,
		WorkstationType:    execution.WorkstationType,
		RunnerID:           execution.RunnerID,
		Model:              execution.Model,
		ReasoningEffort:    execution.ReasoningEffort,
		SystemPrompt:       execution.SystemPrompt,
		UserMessage:        execution.UserMessage,
		InputTokens:        append([]any(nil), execution.InputTokens...),
		OutputSchema:       execution.OutputSchema,
		WorkingDirectory:   execution.WorkingDirectory,
		Worktree:           execution.Worktree,
		ProcessEnvironment: append([]string(nil), execution.ProcessEnvironment...),
		ResumeSession:      workers.CloneProviderSessionReference(execution.ResumeSession),
	}
}

func liveSessionDispatchResult(request workers.WorkstationDispatchRequest, response workers.RunnerExecutionResult, attemptErr error) workers.WorkstationDispatchResult {
	result := workers.WorkResult{
		DispatchID: request.Execution.Dispatch.DispatchID, TransitionID: request.Execution.Dispatch.TransitionID,
		Output: response.Content, ProviderSession: workers.CloneProviderSessionMetadata(response.ProviderSession), Diagnostics: workers.CloneWorkDiagnostics(response.Diagnostics),
	}
	terminal := workers.WorkstationDispatchTerminalOutcomeCompleted
	if attemptErr != nil {
		result.Outcome, result.Error, terminal = workers.OutcomeFailed, attemptErr.Error(), workers.WorkstationDispatchTerminalOutcomeFailed
		if errors.Is(attemptErr, context.Canceled) {
			terminal = workers.WorkstationDispatchTerminalOutcomeCanceled
		}
	} else {
		result.Outcome = workers.OutcomeAccepted
	}
	return workers.WorkstationDispatchResult{DispatchID: request.Execution.Dispatch.DispatchID, WorkstationName: request.WorkstationName, TerminalOutcome: terminal, Result: result}
}

func containsLiveSessionSequence(values []string, first, second string) bool {
	for index := 0; index+1 < len(values); index++ {
		if strings.TrimSpace(values[index]) == first && strings.TrimSpace(values[index+1]) == second {
			return true
		}
	}
	return false
}
