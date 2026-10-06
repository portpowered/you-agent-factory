package service

import (
	"context"
	"encoding/json"
	"errors"
	"sync"
	"testing"

	"github.com/portpowered/infinite-you/pkg/services/recordings"

	platformclock "github.com/portpowered/infinite-you/pkg/platform/clock"
	"github.com/portpowered/infinite-you/pkg/platform/logging"
	eventswire "github.com/portpowered/infinite-you/pkg/services/events/wire"
	modelinference "github.com/portpowered/infinite-you/pkg/services/models"
	providersessions "github.com/portpowered/infinite-you/pkg/services/provider_sessions"
	"github.com/portpowered/infinite-you/pkg/services/providers"
	"github.com/portpowered/infinite-you/pkg/services/work"
	workersessions "github.com/portpowered/infinite-you/pkg/services/worker_sessions"
	workersessionswire "github.com/portpowered/infinite-you/pkg/services/worker_sessions/wire"
	"github.com/portpowered/infinite-you/pkg/services/workers"
)

type unavailableProviderSessions struct {
	providersessions.Service
}

func (unavailableProviderSessions) Project(providersessions.ProjectRequest) (providersessions.ProjectResult, error) {
	return providersessions.ProjectResult{}, providersessions.ErrSessionStorageUnavailable
}

// TestLiveProviderSessionObservationEnablesExactWorkerSessionContinuation
// supplies a completed Providers fake to the Agent runner and Worker Sessions
// bridge. It reports a provider-authored thread while live and verifies Resume
// reaches the exact thread through Providers.ContinueReference.
// pkgmaintcheck:ignore-cyclomatic-complexity pre-existing baseline debt recorded 2026-08-08; refactor this code below the maintainability threshold and remove this exemption
func TestLiveProviderSessionObservationEnablesExactWorkerSessionContinuation(t *testing.T) {
	command := &liveSessionProvidersFake{initialSessionObserved: make(chan struct{})}
	providerService := command
	bridge := &workersessions.ProviderSessionObservationPublisher{}
	var sessions workersessions.Service
	runner, err := New(providerService, func(fragment workers.ProgressFragment) {
		if fragment.Correlation.DispatchID == "" {
			fragment.Correlation.DispatchID = fragment.DispatchID
		}
		if fragment.Correlation.AttemptID == "" {
			fragment.Correlation.AttemptID = fragment.DispatchID
		}
		if err := bridge.PublishWorkerSessionProgress(context.Background(), sessions, "worker-live-provider-session", fragment); err != nil {
			t.Errorf("publish live progress: %v", err)
		}
	})
	if err != nil {
		t.Fatalf("agent New() error = %v", err)
	}
	eventsService, err := eventswire.NewService(logging.NoopLogger{})
	if err != nil {
		t.Fatalf("events wire NewService() error = %v", err)
	}
	service := newLiveSessionService(runner)
	sessions, err = workersessionswire.NewService(service, eventsService, logging.NoopLogger{}, platformclock.Real{}, platformclock.Real{}, unavailableProviderSessions{}, nil, nil, unavailableWorkerControlStore{}, unavailableWorkerControlStore{}, new(workersessionswire.HistorySnapshotBudget))
	if err != nil {
		t.Fatalf("Worker Sessions wire NewService() error = %v", err)
	}

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
	if reference := command.resumeReference(); reference != want {
		t.Fatalf("continued provider reference = %#v, want exact identity %#v", reference, want)
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

// The supplied provider reports a live session before cancellation and captures
// the exact opaque reference on continuation; native decoding is provider-owned.
type liveSessionProvidersFake struct {
	providers.Service
	initialSessionObserved chan struct{}
	mu                     sync.Mutex
	resume                 providers.SessionRef
}

func (r *liveSessionProvidersFake) Execute(ctx context.Context, request providers.ExecuteRequest) (providers.ExecuteResult, error) {
	reference := providers.SessionRef{Provider: providers.IDCodex, Kind: providers.SessionIDKind, ID: "codex-live-thread-1"}
	request.ObserveSession(reference)
	close(r.initialSessionObserved)
	<-ctx.Done()
	return providers.ExecuteResult{SessionRef: &reference}, providers.ExecuteFailure{Kind: providers.ExecuteFailureKindCanceled, Message: "provider invocation was canceled"}
}
func (r *liveSessionProvidersFake) ContinueReference(_ context.Context, request providers.ContinueReferenceRequest) (providers.ContinueReferenceResult, error) {
	reference, err := request.Reference.ToSessionRef()
	if err != nil {
		return providers.ContinueReferenceResult{}, err
	}
	r.mu.Lock()
	r.resume = reference
	r.mu.Unlock()
	request.Attempt.ObserveSession(reference)
	return providers.ContinueReferenceResult{Reference: request.Reference, Outcome: providers.ContinuationOutcomeResumed, Result: providers.ExecuteResult{Content: "resumed exact output", SessionRef: &reference}}, nil
}
func (r *liveSessionProvidersFake) resumeReference() providers.SessionRef {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.resume
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

// unavailableWorkerControlStore is a controlled persistence outage for tests
// that do not own durable control behavior. Every operation fails explicitly;
// it must never be used as evidence that an intent was committed or replayed.
type unavailableWorkerControlStore struct{}

func (unavailableWorkerControlStore) BeginWorkerControlOperation(context.Context, recordings.WorkerControlOperationRecord) (recordings.WorkerControlOperationRecord, bool, error) {
	return recordings.WorkerControlOperationRecord{}, false, recordings.ErrWorkerRecordingPersistence
}

func (unavailableWorkerControlStore) AdvanceWorkerControlOperation(context.Context, recordings.WorkerControlOperationRecord, uint64) (recordings.WorkerControlOperationRecord, error) {
	return recordings.WorkerControlOperationRecord{}, recordings.ErrWorkerRecordingPersistence
}

func (unavailableWorkerControlStore) LoadWorkerControlOperation(context.Context, recordings.WorkerControlOperationKey) (recordings.WorkerControlOperationRecord, error) {
	return recordings.WorkerControlOperationRecord{}, recordings.ErrWorkerRecordingPersistence
}

func (unavailableWorkerControlStore) ListWorkerControlOperations(context.Context, recordings.WorkerControlTarget) ([]recordings.WorkerControlOperationRecord, error) {
	return nil, recordings.ErrWorkerRecordingPersistence
}

func (unavailableWorkerControlStore) PersistWorkerControlInput(context.Context, recordings.WorkerControlOperationKey, json.RawMessage) (string, error) {
	return "", recordings.ErrWorkerRecordingPersistence
}

func (unavailableWorkerControlStore) ReadWorkerControlInput(context.Context, recordings.WorkerControlOperationKey, string) (json.RawMessage, error) {
	return nil, recordings.ErrWorkerRecordingPersistence
}

func (unavailableWorkerControlStore) SaveWorkerRestartRecipe(context.Context, recordings.WorkerControlTarget, workers.WorkstationDispatchRequest) error {
	return recordings.ErrWorkerRecordingPersistence
}

func (unavailableWorkerControlStore) ReadWorkerRestartRecipe(context.Context, recordings.WorkerControlTarget) (workers.WorkstationDispatchRequest, error) {
	return workers.WorkstationDispatchRequest{}, recordings.ErrWorkerRecordingPersistence
}

func (unavailableWorkerControlStore) ReadWorkerContinuationSource(context.Context, recordings.WorkerControlTarget) (recordings.WorkerContinuationSource, error) {
	return recordings.WorkerContinuationSource{}, recordings.ErrWorkerRecordingPersistence
}
