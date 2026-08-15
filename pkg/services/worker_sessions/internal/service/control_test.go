package service_test

import (
	"context"
	"encoding/json"
	"errors"
	"sync"
	"testing"

	"github.com/portpowered/infinite-you/pkg/platform/logging"
	"github.com/portpowered/infinite-you/pkg/services/events"
	"github.com/portpowered/infinite-you/pkg/services/providers"
	"github.com/portpowered/infinite-you/pkg/services/recordings"
	workersessions "github.com/portpowered/infinite-you/pkg/services/worker_sessions"
	"github.com/portpowered/infinite-you/pkg/services/workers"
)

// controlledBoundary records the exact pool-boundary controls Worker Sessions
// performs and exposes callback completion as deterministic test input. It
// models an accepted asynchronous dispatch without sleeps or polling.
type controlledBoundary struct {
	mu sync.Mutex

	started      chan struct{}
	startedOnce  sync.Once
	admitted     chan struct{}
	admittedOnce sync.Once
	accept       workers.WorkstationDispatchAcceptFunc
	request      workers.WorkstationDispatchRequest
	publishCalls int
	cancelCalls  []workers.WorkstationDispatchCancelRequest
	cancelCalled chan struct{}
	cancel       func(context.Context, workers.WorkstationDispatchCancelRequest) (workers.WorkstationDispatchCancelResult, error)
	publishError func(int, workers.WorkstationDispatchRequest) error
}

var _ workers.WorkstationPoolBoundary = (*controlledBoundary)(nil)

func newControlledBoundary() *controlledBoundary {
	return &controlledBoundary{
		started:      make(chan struct{}),
		admitted:     make(chan struct{}),
		cancelCalled: make(chan struct{}, 1),
	}
}

func (*controlledBoundary) Start(context.Context) error { return nil }

func (b *controlledBoundary) Publish(_ context.Context, request workers.WorkstationDispatchRequest, accept workers.WorkstationDispatchAcceptFunc) error {
	return b.PublishWithAdmission(context.Background(), request, nil, accept)
}

func (b *controlledBoundary) PublishWithAdmission(_ context.Context, request workers.WorkstationDispatchRequest, admitted workers.WorkstationDispatchAdmissionFunc, accept workers.WorkstationDispatchAcceptFunc) error {
	b.mu.Lock()
	b.request = request
	b.accept = accept
	b.publishCalls++
	publishCall := b.publishCalls
	publishError := b.publishError
	b.mu.Unlock()
	b.startedOnce.Do(func() { close(b.started) })
	if publishError != nil {
		if err := publishError(publishCall, request); err != nil {
			return err
		}
	}
	if admitted != nil {
		admitted()
		b.admittedOnce.Do(func() { close(b.admitted) })
	}
	return nil
}

func (b *controlledBoundary) Cancel(ctx context.Context, request workers.WorkstationDispatchCancelRequest) (workers.WorkstationDispatchCancelResult, error) {
	b.mu.Lock()
	b.cancelCalls = append(b.cancelCalls, request)
	cancel := b.cancel
	b.mu.Unlock()
	b.cancelCalled <- struct{}{}
	if cancel != nil {
		return cancel(ctx, request)
	}
	return workers.WorkstationDispatchCancelResult{DispatchID: request.DispatchID, Outcome: workers.WorkstationDispatchCancelOutcomeCanceled}, nil
}

func (*controlledBoundary) Stop(context.Context) error { return nil }

func (b *controlledBoundary) complete(result workers.WorkstationDispatchResult, err error) {
	b.mu.Lock()
	accept, request := b.accept, b.request
	b.mu.Unlock()
	if accept == nil {
		panic("complete before Publish")
	}
	accept(context.Background(), request, result, err)
}

func (b *controlledBoundary) cancellations() []workers.WorkstationDispatchCancelRequest {
	b.mu.Lock()
	defer b.mu.Unlock()
	return append([]workers.WorkstationDispatchCancelRequest(nil), b.cancelCalls...)
}

func (b *controlledBoundary) publishCount() int {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.publishCalls
}

func (b *controlledBoundary) currentRequest() workers.WorkstationDispatchRequest {
	b.mu.Lock()
	defer b.mu.Unlock()
	return workers.WorkstationDispatchRequest{
		WorkstationName: b.request.WorkstationName,
		Execution:       workers.CloneWorkstationExecutionRequest(b.request.Execution),
	}
}

func (b *controlledBoundary) setCancel(fn func(context.Context, workers.WorkstationDispatchCancelRequest) (workers.WorkstationDispatchCancelResult, error)) {
	b.mu.Lock()
	b.cancel = fn
	b.mu.Unlock()
}

func (b *controlledBoundary) setPublishError(fn func(int, workers.WorkstationDispatchRequest) error) {
	b.mu.Lock()
	b.publishError = fn
	b.mu.Unlock()
}

func newControlledRegistry(t *testing.T, boundary workers.WorkstationPoolBoundary) workersessions.Service {
	t.Helper()
	registry, err := newService(boundary, newEventsAppender(), logging.NoopLogger{})
	if err != nil {
		t.Fatalf("service.New() error = %v", err)
	}
	return registry
}

func canceledDispatchResult(dispatchID string) workers.WorkstationDispatchResult {
	return workers.WorkstationDispatchResult{
		DispatchID:      dispatchID,
		TerminalOutcome: workers.WorkstationDispatchTerminalOutcomeCanceled,
		Result: workers.WorkResult{
			DispatchID: dispatchID,
			Outcome:    workers.OutcomeFailed,
			Error:      workers.ErrWorkstationDispatchCanceled.Error(),
		},
	}
}

func completedDispatchResult(dispatchID string) workers.WorkstationDispatchResult {
	return workers.WorkstationDispatchResult{
		DispatchID:      dispatchID,
		TerminalOutcome: workers.WorkstationDispatchTerminalOutcomeCompleted,
		Result:          workers.WorkResult{DispatchID: dispatchID, Outcome: workers.OutcomeAccepted},
	}
}

func startControlledSession(t *testing.T, registry workersessions.Service, boundary *controlledBoundary, id, dispatchID string) <-chan workersessions.InvokeSessionResult {
	t.Helper()
	result := make(chan workersessions.InvokeSessionResult, 1)
	go func() {
		started, err := registry.InvokeSession(context.Background(), validStartRequest(id, dispatchID))
		if err != nil {
			t.Errorf("Start() error = %v", err)
		}
		result <- started
	}()
	<-boundary.admitted
	return result
}

func TestCancel_UsesExactBoundaryDispatchDespiteCanceledCallerContextAndIsIdempotent(t *testing.T) {
	boundary := newControlledBoundary()
	registry := newControlledRegistry(t, boundary)
	started := startControlledSession(t, registry, boundary, "worker-1", "dispatch-1")

	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	boundary.setCancel(func(received context.Context, request workers.WorkstationDispatchCancelRequest) (workers.WorkstationDispatchCancelResult, error) {
		if received.Err() != nil {
			t.Fatalf("boundary Cancel context = %v, want detached context", received.Err())
		}
		if request.DispatchID != "dispatch-1" {
			t.Fatalf("boundary Cancel dispatch = %q, want dispatch-1", request.DispatchID)
		}
		boundary.complete(canceledDispatchResult(request.DispatchID), workers.ErrWorkstationDispatchCanceled)
		return workers.WorkstationDispatchCancelResult{DispatchID: request.DispatchID, Outcome: workers.WorkstationDispatchCancelOutcomeCanceled}, nil
	})

	result, err := registry.Cancel(ctx, workersessions.ControlRequest{ID: "worker-1"})
	if err != nil || result.Outcome != workersessions.ControlOutcomeApplied || result.DispatchID != "dispatch-1" {
		t.Fatalf("Cancel() = %#v, %v, want applied exact dispatch", result, err)
	}
	if result.Session.State != workersessions.StateCanceled {
		t.Fatalf("Cancel() state = %q, want CANCELED", result.Session.State)
	}
	if got := <-started; got.Session.State != workersessions.StateCanceled || got.Dispatch.TerminalOutcome != workers.WorkstationDispatchTerminalOutcomeCanceled {
		t.Fatalf("Start() result after cancel = %#v, want canceled raw dispatch", got)
	}

	repeated, err := registry.Cancel(context.Background(), workersessions.ControlRequest{ID: "worker-1"})
	if err != nil || repeated.Outcome != workersessions.ControlOutcomeNoop {
		t.Fatalf("repeated Cancel() = %#v, %v, want NOOP", repeated, err)
	}
	if calls := boundary.cancellations(); len(calls) != 1 || calls[0].DispatchID != "dispatch-1" {
		t.Fatalf("boundary cancellation calls = %#v, want one exact call", calls)
	}
}

func TestTerminate_WaitsForAcceptedDispatchCallbackBeforeReportingTerminal(t *testing.T) {
	boundary := newControlledBoundary()
	registry := newControlledRegistry(t, boundary)
	started := startControlledSession(t, registry, boundary, "worker-1", "dispatch-1")

	terminated := make(chan workersessions.ControlResult, 1)
	go func() {
		result, err := registry.Terminate(context.Background(), workersessions.ControlRequest{ID: "worker-1"})
		if err != nil {
			t.Errorf("Terminate() error = %v", err)
		}
		terminated <- result
	}()
	<-boundary.cancelCalled
	select {
	case result := <-terminated:
		t.Fatalf("Terminate() returned before callback joined: %#v", result)
	default:
	}

	boundary.complete(canceledDispatchResult("dispatch-1"), workers.ErrWorkstationDispatchCanceled)
	result := <-terminated
	if result.Outcome != workersessions.ControlOutcomeApplied || result.Session.State != workersessions.StateTerminated {
		t.Fatalf("Terminate() = %#v, want applied TERMINATED after callback", result)
	}
	if got := <-started; got.Session.State != workersessions.StateTerminated {
		t.Fatalf("Start() session after terminate = %#v, want TERMINATED", got.Session)
	}
}

func TestCancel_WaitsForTheAuthoritativeTerminalCallback(t *testing.T) {
	boundary := newControlledBoundary()
	registry := newControlledRegistry(t, boundary)
	started := startControlledSession(t, registry, boundary, "worker-1", "dispatch-1")
	boundary.setCancel(func(context.Context, workers.WorkstationDispatchCancelRequest) (workers.WorkstationDispatchCancelResult, error) {
		return workers.WorkstationDispatchCancelResult{
			DispatchID: "dispatch-1", Outcome: workers.WorkstationDispatchCancelOutcomeCanceled,
		}, nil
	})

	canceled := make(chan workersessions.ControlResult, 1)
	cancelErr := make(chan error, 1)
	go func() {
		result, callErr := registry.Cancel(context.Background(), workersessions.ControlRequest{ID: "worker-1"})
		canceled <- result
		cancelErr <- callErr
	}()
	<-boundary.cancelCalled
	select {
	case result := <-canceled:
		t.Fatalf("Cancel() returned before its callback: %#v", result)
	default:
	}

	boundary.complete(canceledDispatchResult("dispatch-1"), workers.ErrWorkstationDispatchCanceled)
	result := <-canceled
	if callErr := <-cancelErr; callErr != nil || result.Outcome != workersessions.ControlOutcomeApplied || result.Session.State != workersessions.StateCanceled {
		t.Fatalf("Cancel() = %#v, %v, want joined CANCELED APPLIED", result, callErr)
	}

	terminated, err := registry.Terminate(context.Background(), workersessions.ControlRequest{ID: "worker-1"})
	if err != nil || terminated.Outcome != workersessions.ControlOutcomeNoop || terminated.Session.State != workersessions.StateCanceled {
		t.Fatalf("Terminate() = %#v, %v, want joined CANCELED NOOP", terminated, err)
	}
	if calls := boundary.cancellations(); len(calls) != 1 || calls[0].DispatchID != "dispatch-1" {
		t.Fatalf("boundary cancellation calls = %#v, want one exact call", calls)
	}
	if final := <-started; final.Session.State != workersessions.StateCanceled {
		t.Fatalf("Start() after Cancel then Terminate = %#v, want CANCELED", final)
	}
}

func TestTerminate_ConcurrentCallsShareOneCancellationAndJoinTheCallback(t *testing.T) {
	boundary := newControlledBoundary()
	registry := newControlledRegistry(t, boundary)
	started := startControlledSession(t, registry, boundary, "worker-1", "dispatch-1")
	cancelReturned := make(chan struct{})
	boundary.setCancel(func(context.Context, workers.WorkstationDispatchCancelRequest) (workers.WorkstationDispatchCancelResult, error) {
		close(cancelReturned)
		return workers.WorkstationDispatchCancelResult{
			DispatchID: "dispatch-1", Outcome: workers.WorkstationDispatchCancelOutcomeCanceled,
		}, nil
	})

	first := make(chan workersessions.ControlResult, 1)
	firstErr := make(chan error, 1)
	go func() {
		result, callErr := registry.Terminate(context.Background(), workersessions.ControlRequest{ID: "worker-1"})
		first <- result
		firstErr <- callErr
	}()
	<-boundary.cancelCalled
	<-cancelReturned

	second := make(chan workersessions.ControlResult, 1)
	secondErr := make(chan error, 1)
	go func() {
		result, callErr := registry.Terminate(context.Background(), workersessions.ControlRequest{ID: "worker-1"})
		second <- result
		secondErr <- callErr
	}()
	select {
	case result := <-first:
		t.Fatalf("first Terminate() returned before callback: %#v", result)
	default:
	}
	select {
	case result := <-second:
		t.Fatalf("second Terminate() returned before callback: %#v", result)
	default:
	}

	boundary.complete(canceledDispatchResult("dispatch-1"), workers.ErrWorkstationDispatchCanceled)
	firstResult := <-first
	if callErr := <-firstErr; callErr != nil || firstResult.Outcome != workersessions.ControlOutcomeApplied || firstResult.Session.State != workersessions.StateTerminated {
		t.Fatalf("first Terminate() = %#v, %v, want joined TERMINATED applied result", firstResult, callErr)
	}
	secondResult := <-second
	if callErr := <-secondErr; callErr != nil || secondResult.Outcome != workersessions.ControlOutcomeNoop || secondResult.Session.State != workersessions.StateTerminated {
		t.Fatalf("second Terminate() = %#v, %v, want joined TERMINATED NOOP", secondResult, callErr)
	}
	if calls := boundary.cancellations(); len(calls) != 1 || calls[0].DispatchID != "dispatch-1" {
		t.Fatalf("boundary cancellation calls = %#v, want one exact call", calls)
	}
	if final := <-started; final.Session.State != workersessions.StateTerminated {
		t.Fatalf("Start() after concurrent Terminate() = %#v, want TERMINATED", final)
	}
}

func TestControl_UnsupportedPauseResumeAndBoundaryFailureLeaveLifecycleTruthful(t *testing.T) {
	boundary := newControlledBoundary()
	registry := newControlledRegistry(t, boundary)
	started := startControlledSession(t, registry, boundary, "worker-1", "dispatch-1")

	for _, action := range []struct {
		name string
		call func(context.Context, workersessions.ControlRequest) (workersessions.ControlResult, error)
	}{
		{name: "pause", call: registry.Pause},
		{name: "resume", call: registry.Resume},
	} {
		t.Run(action.name, func(t *testing.T) {
			result, err := action.call(context.Background(), workersessions.ControlRequest{ID: "worker-1"})
			if err != nil || result.Outcome != workersessions.ControlOutcomeUnsupported || result.Session.State == workersessions.StatePaused {
				t.Fatalf("%s = %#v, %v, want unsupported without a fabricated PAUSED state", action.name, result, err)
			}
		})
	}

	boundary.setCancel(func(context.Context, workers.WorkstationDispatchCancelRequest) (workers.WorkstationDispatchCancelResult, error) {
		return workers.WorkstationDispatchCancelResult{}, errors.New("boundary unavailable")
	})
	failed, err := registry.Cancel(context.Background(), workersessions.ControlRequest{ID: "worker-1"})
	if err == nil || failed.Outcome != workersessions.ControlOutcomeFailed || failed.Session.State != workersessions.StateRunning {
		t.Fatalf("Cancel() boundary failure = %#v, %v, want FAILED with unchanged RUNNING", failed, err)
	}

	boundary.complete(completedDispatchResult("dispatch-1"), nil)
	if got := <-started; got.Session.State != workersessions.StateCompleted {
		t.Fatalf("Start() after ordinary completion = %#v, want COMPLETED", got.Session)
	}
	terminal, err := registry.Pause(context.Background(), workersessions.ControlRequest{ID: "worker-1"})
	if err != nil || terminal.Outcome != workersessions.ControlOutcomeNoop {
		t.Fatalf("Pause() on terminal session = %#v, %v, want NOOP", terminal, err)
	}
}

// pkgmaintcheck:ignore-cyclomatic-complexity pre-existing baseline debt recorded 2026-08-08; refactor this code below the maintainability threshold and remove this exemption
func TestPauseResume_ContinuesExactProviderReferenceWithSameWorkerSessionCorrelation(t *testing.T) {
	boundary := newControlledBoundary()
	registry := newControlledRegistry(t, boundary)
	started := startControlledSession(t, registry, boundary, "worker-1", "dispatch-1")
	initial := boundary.currentRequest()

	reference := providers.SessionRef{
		Provider: providers.IDCodex,
		Kind:     providers.SessionIDKind,
		ID:       "provider-session-1",
	}
	if _, err := registry.AssociateProviderSession(context.Background(), workersessions.ProviderSessionAssociationRequest{
		WorkerSessionID: "worker-1",
		DispatchID:      "dispatch-1",
		Reference:       reference,
	}); err != nil {
		t.Fatalf("AssociateProviderSession() error = %v", err)
	}

	boundary.setCancel(func(context.Context, workers.WorkstationDispatchCancelRequest) (workers.WorkstationDispatchCancelResult, error) {
		boundary.complete(canceledDispatchResult("dispatch-1"), workers.ErrWorkstationDispatchCanceled)
		return workers.WorkstationDispatchCancelResult{
			DispatchID: "dispatch-1", Outcome: workers.WorkstationDispatchCancelOutcomeCanceled,
		}, nil
	})
	paused, err := registry.Pause(context.Background(), workersessions.ControlRequest{ID: "worker-1"})
	if err != nil || paused.Outcome != workersessions.ControlOutcomeApplied || paused.Session.State != workersessions.StatePaused {
		t.Fatalf("Pause() = %#v, %v, want applied PAUSED", paused, err)
	}
	select {
	case result := <-started:
		t.Fatalf("Start() returned from pause as %#v, want it to await continuation", result)
	default:
	}

	resumed, err := registry.Resume(context.Background(), workersessions.ControlRequest{ID: "worker-1"})
	if err != nil || resumed.Outcome != workersessions.ControlOutcomeApplied || resumed.Session.State != workersessions.StateRunning {
		t.Fatalf("Resume() = %#v, %v, want applied RUNNING", resumed, err)
	}
	continuation := boundary.currentRequest()
	wantContinuation := reference.ContinuationRef()
	if continuation.Execution.Continuation == nil || *continuation.Execution.Continuation != wantContinuation {
		t.Fatalf("continuation Continuation = %#v, want exact %#v", continuation.Execution.Continuation, wantContinuation)
	}
	if continuation.WorkstationName != initial.WorkstationName || continuation.Execution.Dispatch.Execution.RequestID != initial.Execution.Dispatch.Execution.RequestID {
		t.Fatalf("continuation correlation = %#v, want preserved workstation and turn request", continuation)
	}
	if continuation.Execution.Dispatch.DispatchID == "dispatch-1" || continuation.Execution.Dispatch.DispatchID != resumed.DispatchID {
		t.Fatalf("continuation dispatch = %q, want fresh resumed dispatch %q", continuation.Execution.Dispatch.DispatchID, resumed.DispatchID)
	}

	resumedResult := completedDispatchResult(resumed.DispatchID)
	resumedResult.Result.Continuation = continuationFromProviderMetadata(&providers.SessionMetadata{
		Provider: reference.Provider.String(),
		Kind:     reference.Kind,
		ID:       reference.ID,
	})
	boundary.complete(resumedResult, nil)
	final := <-started
	if final.Session.State != workersessions.StateCompleted || final.Dispatch.DispatchID != resumed.DispatchID {
		t.Fatalf("Start() final = %#v, want resumed terminal result", final)
	}
	if final.Session.ProviderSessionAssociation == nil || final.Session.ProviderSessionAssociation.Reference != reference {
		t.Fatalf("final association = %#v, want retained exact reference", final.Session.ProviderSessionAssociation)
	}

	repeated, err := registry.Resume(context.Background(), workersessions.ControlRequest{ID: "worker-1"})
	if err != nil || repeated.Outcome != workersessions.ControlOutcomeNoop || repeated.Session.State != workersessions.StateCompleted {
		t.Fatalf("repeated Resume() = %#v, %v, want terminal NOOP", repeated, err)
	}
}

func TestPausedControl_TerminalizesWithoutRecancelingCompletedPauseDispatch(t *testing.T) {
	tests := []struct {
		name  string
		call  func(workersessions.Service, context.Context, workersessions.ControlRequest) (workersessions.ControlResult, error)
		state workersessions.State
	}{
		{name: "cancel", call: func(service workersessions.Service, ctx context.Context, request workersessions.ControlRequest) (workersessions.ControlResult, error) {
			return service.Cancel(ctx, request)
		}, state: workersessions.StateCanceled},
		{name: "terminate", call: func(service workersessions.Service, ctx context.Context, request workersessions.ControlRequest) (workersessions.ControlResult, error) {
			return service.Terminate(ctx, request)
		}, state: workersessions.StateTerminated},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			boundary := newControlledBoundary()
			registry := newControlledRegistry(t, boundary)
			started := startControlledSession(t, registry, boundary, "worker-1", "dispatch-1")
			reference := providers.SessionRef{Provider: providers.IDCodex, Kind: providers.SessionIDKind, ID: "provider-session-1"}
			if _, err := registry.AssociateProviderSession(context.Background(), workersessions.ProviderSessionAssociationRequest{
				WorkerSessionID: "worker-1", DispatchID: "dispatch-1", Reference: reference,
			}); err != nil {
				t.Fatalf("AssociateProviderSession() error = %v", err)
			}
			boundary.setCancel(func(_ context.Context, request workers.WorkstationDispatchCancelRequest) (workers.WorkstationDispatchCancelResult, error) {
				boundary.complete(canceledDispatchResult(request.DispatchID), workers.ErrWorkstationDispatchCanceled)
				return workers.WorkstationDispatchCancelResult{DispatchID: request.DispatchID, Outcome: workers.WorkstationDispatchCancelOutcomeCanceled}, nil
			})
			if paused, err := registry.Pause(context.Background(), workersessions.ControlRequest{ID: "worker-1"}); err != nil || paused.Session.State != workersessions.StatePaused {
				t.Fatalf("Pause() = %#v, %v, want PAUSED", paused, err)
			}

			result, err := test.call(registry, context.Background(), workersessions.ControlRequest{ID: "worker-1"})
			if err != nil || result.Outcome != workersessions.ControlOutcomeApplied || result.Session.State != test.state {
				t.Fatalf("%s() = %#v, %v, want applied %s", test.name, result, err, test.state)
			}
			if calls := boundary.cancellations(); len(calls) != 1 || calls[0].DispatchID != "dispatch-1" {
				t.Fatalf("boundary cancellations = %#v, want only the original pause cancellation", calls)
			}
			if final := <-started; final.Session.State != test.state {
				t.Fatalf("Start() after paused %s = %#v, want %s", test.name, final, test.state)
			}
		})
	}
}

func TestPauseResume_InvalidContinuationResultFailsAndRetainsAssociation(t *testing.T) {
	tests := []struct {
		name     string
		metadata func(providers.SessionRef) *providers.SessionMetadata
	}{
		{name: "missing reference"},
		{
			name: "malformed reference",
			metadata: func(reference providers.SessionRef) *providers.SessionMetadata {
				return &providers.SessionMetadata{Provider: reference.Provider.String(), Kind: reference.Kind}
			},
		},
		{
			name: "foreign reference",
			metadata: func(reference providers.SessionRef) *providers.SessionMetadata {
				return &providers.SessionMetadata{Provider: reference.Provider.String(), Kind: reference.Kind, ID: "foreign-provider-session"}
			},
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			boundary := newControlledBoundary()
			registry := newControlledRegistry(t, boundary)
			started := startControlledSession(t, registry, boundary, "worker-1", "dispatch-1")
			reference := providers.SessionRef{Provider: providers.IDCodex, Kind: providers.SessionIDKind, ID: "provider-session-1"}
			if _, err := registry.AssociateProviderSession(context.Background(), workersessions.ProviderSessionAssociationRequest{
				WorkerSessionID: "worker-1", DispatchID: "dispatch-1", Reference: reference,
			}); err != nil {
				t.Fatalf("AssociateProviderSession() error = %v", err)
			}
			boundary.setCancel(func(_ context.Context, request workers.WorkstationDispatchCancelRequest) (workers.WorkstationDispatchCancelResult, error) {
				boundary.complete(canceledDispatchResult(request.DispatchID), workers.ErrWorkstationDispatchCanceled)
				return workers.WorkstationDispatchCancelResult{DispatchID: request.DispatchID, Outcome: workers.WorkstationDispatchCancelOutcomeCanceled}, nil
			})
			if paused, err := registry.Pause(context.Background(), workersessions.ControlRequest{ID: "worker-1"}); err != nil || paused.Session.State != workersessions.StatePaused {
				t.Fatalf("Pause() = %#v, %v, want PAUSED", paused, err)
			}
			resumed, err := registry.Resume(context.Background(), workersessions.ControlRequest{ID: "worker-1"})
			if err != nil || resumed.Outcome != workersessions.ControlOutcomeApplied {
				t.Fatalf("Resume() = %#v, %v, want applied continuation", resumed, err)
			}
			invalid := completedDispatchResult(resumed.DispatchID)
			if test.metadata != nil {
				invalid.Result.Continuation = continuationFromProviderMetadata(test.metadata(reference))
			}
			boundary.complete(invalid, nil)

			final := <-started
			if final.Session.State != workersessions.StateFailed || final.Session.Result == nil || final.Session.Result.Cause == nil {
				t.Fatalf("Start() after invalid continuation = %#v, want failed terminal result", final)
			}
			cause := final.Session.Result.Cause
			if cause.Kind != workersessions.FailureCauseWorkersExecutionFailure || cause.ProviderContinuationFailureKind != providers.ContinuationFailureKindInvalid {
				t.Fatalf("terminal failure cause = %#v, want invalid continuation classification", cause)
			}
			if final.Session.ProviderSessionAssociation == nil || final.Session.ProviderSessionAssociation.Reference != reference {
				t.Fatalf("terminal association = %#v, want retained %#v", final.Session.ProviderSessionAssociation, reference)
			}
		})
	}
}

// pkgmaintcheck:ignore-cyclomatic-complexity pre-existing baseline debt recorded 2026-08-08; refactor this code below the maintainability threshold and remove this exemption
func TestPauseResume_ContinuationFailureKeepsAssociationAndProviderClassification(t *testing.T) {
	tests := []struct {
		name                    string
		providerFailureKind     providers.ExecuteFailureKind
		continuationFailureKind providers.ContinuationFailureKind
		continuationOutcome     providers.ContinuationOutcome
	}{
		{
			name:                    "invalid reference",
			continuationFailureKind: providers.ContinuationFailureKindInvalid,
		},
		{
			name:                    "foreign reference",
			continuationFailureKind: providers.ContinuationFailureKindForeign,
		},
		{
			name:                    "stale reference",
			continuationFailureKind: providers.ContinuationFailureKindStale,
		},
		{
			name:                "unsupported continuation",
			continuationOutcome: providers.ContinuationOutcomeUnsupported,
		},
		{
			name:                "provider operational failure",
			providerFailureKind: providers.ExecuteFailureKindDependency,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			boundary := newControlledBoundary()
			registry := newControlledRegistry(t, boundary)
			started := startControlledSession(t, registry, boundary, "worker-1", "dispatch-1")
			reference := providers.SessionRef{
				Provider: providers.IDCodex,
				Kind:     providers.SessionIDKind,
				ID:       "provider-session-1",
			}
			if _, err := registry.AssociateProviderSession(context.Background(), workersessions.ProviderSessionAssociationRequest{
				WorkerSessionID: "worker-1",
				DispatchID:      "dispatch-1",
				Reference:       reference,
			}); err != nil {
				t.Fatalf("AssociateProviderSession() error = %v", err)
			}
			boundary.setCancel(func(context.Context, workers.WorkstationDispatchCancelRequest) (workers.WorkstationDispatchCancelResult, error) {
				boundary.complete(canceledDispatchResult("dispatch-1"), workers.ErrWorkstationDispatchCanceled)
				return workers.WorkstationDispatchCancelResult{DispatchID: "dispatch-1", Outcome: workers.WorkstationDispatchCancelOutcomeCanceled}, nil
			})
			if paused, err := registry.Pause(context.Background(), workersessions.ControlRequest{ID: "worker-1"}); err != nil || paused.Session.State != workersessions.StatePaused {
				t.Fatalf("Pause() = %#v, %v, want PAUSED", paused, err)
			}

			resumed, err := registry.Resume(context.Background(), workersessions.ControlRequest{ID: "worker-1"})
			if err != nil || resumed.Outcome != workersessions.ControlOutcomeApplied {
				t.Fatalf("Resume() = %#v, %v, want applied continuation", resumed, err)
			}
			failed := completedDispatchResult(resumed.DispatchID)
			failed.Result.Outcome = workers.OutcomeFailed
			failed.Result.FailureMetadata = &workers.WorkFailureMetadata{
				Family: workers.WorkFailureFamilyTerminal,
				Type:   workers.WorkFailureTypePermanentBadRequest,
			}
			failed.Result.ProviderFailureKind = test.providerFailureKind
			failed.Result.ProviderContinuationFailureKind = test.continuationFailureKind
			failed.Result.ProviderContinuationOutcome = test.continuationOutcome
			boundary.complete(failed, nil)

			final := <-started
			if final.Session.State != workersessions.StateFailed || final.Session.Result == nil || final.Session.Result.Cause == nil {
				t.Fatalf("Start() final = %#v, want failed terminal result", final)
			}
			cause := final.Session.Result.Cause
			if cause.ProviderFailureKind != test.providerFailureKind ||
				cause.ProviderContinuationFailureKind != test.continuationFailureKind ||
				cause.ProviderContinuationOutcome != test.continuationOutcome {
				t.Fatalf("terminal continuation classification = %#v", cause)
			}
			if final.Session.ProviderSessionAssociation == nil || final.Session.ProviderSessionAssociation.Reference != reference {
				t.Fatalf("terminal association = %#v, want retained exact reference", final.Session.ProviderSessionAssociation)
			}
			repeated, err := registry.Resume(context.Background(), workersessions.ControlRequest{ID: "worker-1"})
			if err != nil || repeated.Outcome != workersessions.ControlOutcomeNoop || repeated.Session.Result == nil ||
				repeated.Session.Result.Cause.ProviderContinuationFailureKind != test.continuationFailureKind {
				t.Fatalf("repeated Resume() = %#v, %v, want unchanged terminal no-op", repeated, err)
			}
		})
	}
}

func TestControl_UnknownAndInvalidIdentityRemainDistinguishable(t *testing.T) {
	registry := newControlledRegistry(t, newControlledBoundary())
	_, invalidErr := registry.Cancel(context.Background(), workersessions.ControlRequest{ID: " "})
	_, unknownErr := registry.Cancel(context.Background(), workersessions.ControlRequest{ID: "missing"})
	if !errors.Is(invalidErr, workersessions.ErrInvalidSessionID) || !errors.Is(unknownErr, workersessions.ErrSessionNotFound) {
		t.Fatalf("invalid=%v unknown=%v, want distinct typed errors", invalidErr, unknownErr)
	}
	_, pauseInvalidErr := registry.Pause(context.Background(), workersessions.ControlRequest{ID: " "})
	_, pauseUnknownErr := registry.Pause(context.Background(), workersessions.ControlRequest{ID: "missing"})
	if !errors.Is(pauseInvalidErr, workersessions.ErrInvalidSessionID) || !errors.Is(pauseUnknownErr, workersessions.ErrSessionNotFound) {
		t.Fatalf("pause invalid=%v unknown=%v, want distinct typed errors", pauseInvalidErr, pauseUnknownErr)
	}
}

func TestControl_BeforeStartCancellationAndTerminationPreventWorkersHandoff(t *testing.T) {
	for _, tc := range []struct {
		name  string
		state workersessions.State
	}{
		{name: "cancel", state: workersessions.StateCanceled},
		{name: "terminate", state: workersessions.StateTerminated},
	} {
		t.Run(tc.name, func(t *testing.T) {
			boundary := newControlledBoundary()
			registry := newControlledRegistry(t, boundary)
			if _, err := registry.Reserve(context.Background(), workersessions.ReserveRequest{ID: "worker-1"}); err != nil {
				t.Fatalf("Reserve() error = %v", err)
			}
			var result workersessions.ControlResult
			var err error
			if tc.name == "cancel" {
				result, err = registry.Cancel(context.Background(), workersessions.ControlRequest{ID: "worker-1"})
			} else {
				result, err = registry.Terminate(context.Background(), workersessions.ControlRequest{ID: "worker-1"})
			}
			if err != nil || result.Outcome != workersessions.ControlOutcomeApplied || result.Session.State != tc.state {
				t.Fatalf("%s before Start = %#v, %v, want applied %s", tc.name, result, err, tc.state)
			}
			started, err := registry.InvokeSession(context.Background(), validStartRequest("worker-1", "dispatch-1"))
			if err != nil || !errors.Is(started.DispatchErr, workers.ErrWorkstationDispatchCanceled) ||
				started.Dispatch.TerminalOutcome != workers.WorkstationDispatchTerminalOutcomeCanceled {
				t.Fatalf("Start() after %s = %#v, %v, want canceled dispatch without handoff", tc.name, started, err)
			}
			select {
			case request := <-boundary.started:
				t.Fatalf("Workers Publish called after %s before Start: %#v", tc.name, request)
			default:
			}
		})
	}
}

func TestCancel_BoundaryAlreadyCanceledWaitsForTheCanonicalTerminalSnapshot(t *testing.T) {
	boundary := newControlledBoundary()
	registry := newControlledRegistry(t, boundary)
	started := startControlledSession(t, registry, boundary, "worker-1", "dispatch-1")
	boundary.setCancel(func(context.Context, workers.WorkstationDispatchCancelRequest) (workers.WorkstationDispatchCancelResult, error) {
		return workers.WorkstationDispatchCancelResult{
			DispatchID: "dispatch-1", Outcome: workers.WorkstationDispatchCancelOutcomeAlreadyCanceled,
		}, nil
	})

	resultCh := make(chan workersessions.ControlResult, 1)
	errCh := make(chan error, 1)
	go func() {
		result, err := registry.Cancel(context.Background(), workersessions.ControlRequest{ID: "worker-1"})
		resultCh <- result
		errCh <- err
	}()
	<-boundary.cancelCalled
	select {
	case result := <-resultCh:
		t.Fatalf("Cancel() returned before the canonical callback: %#v", result)
	default:
	}
	boundary.complete(canceledDispatchResult("dispatch-1"), workers.ErrWorkstationDispatchCanceled)
	result := <-resultCh
	if err := <-errCh; err != nil || result.Outcome != workersessions.ControlOutcomeNoop || result.Session.State != workersessions.StateCanceled {
		t.Fatalf("Cancel() = %#v, %v, want joined CANCELED NOOP", result, err)
	}
	if final := <-started; final.Session.State != workersessions.StateCanceled {
		t.Fatalf("Start() after canonical cancellation = %#v, want CANCELED", final)
	}
}

func TestCancel_ConcurrentControlsShareOneBoundaryEffect(t *testing.T) {
	boundary := newControlledBoundary()
	registry := newControlledRegistry(t, boundary)
	started := startControlledSession(t, registry, boundary, "worker-1", "dispatch-1")
	releaseCancel := make(chan struct{})
	boundary.setCancel(func(context.Context, workers.WorkstationDispatchCancelRequest) (workers.WorkstationDispatchCancelResult, error) {
		<-releaseCancel
		boundary.complete(canceledDispatchResult("dispatch-1"), workers.ErrWorkstationDispatchCanceled)
		return workers.WorkstationDispatchCancelResult{
			DispatchID: "dispatch-1", Outcome: workers.WorkstationDispatchCancelOutcomeCanceled,
		}, nil
	})

	first := make(chan workersessions.ControlResult, 1)
	firstErr := make(chan error, 1)
	go func() {
		result, err := registry.Cancel(context.Background(), workersessions.ControlRequest{ID: "worker-1"})
		first <- result
		firstErr <- err
	}()
	<-boundary.cancelCalled
	second := make(chan workersessions.ControlResult, 1)
	secondErr := make(chan error, 1)
	go func() {
		result, err := registry.Cancel(context.Background(), workersessions.ControlRequest{ID: "worker-1"})
		second <- result
		secondErr <- err
	}()
	close(releaseCancel)
	if result := <-first; result.Outcome != workersessions.ControlOutcomeApplied || <-firstErr != nil {
		t.Fatalf("first Cancel() = %#v, want applied", result)
	}
	if result := <-second; result.Outcome != workersessions.ControlOutcomeNoop || <-secondErr != nil {
		t.Fatalf("second Cancel() = %#v, want no-op after the first control", result)
	}
	if calls := boundary.cancellations(); len(calls) != 1 || calls[0].DispatchID != "dispatch-1" {
		t.Fatalf("boundary cancellation calls = %#v, want one exact call", calls)
	}
	if final := <-started; final.Session.State != workersessions.StateCanceled {
		t.Fatalf("Start() after shared cancellation = %#v, want CANCELED", final)
	}
}

// TestInvokeSession_RetriesARetryableFailureUnderOneWorkerIdentity is the
// attempt loop's core contract.
//
// A retried Worker is still one Worker: its attempts run under the same session
// identity and the same already-open publication window, so a client sees one
// tool call whose content continues rather than a second tool call appearing.
// Only the Workers dispatch identity changes, and it changes to ".../attempt/N"
// so Workers can tell the attempts apart.

func readControlHistory(t *testing.T, eventsSvc events.Service, topic events.Topic, limit int) events.ReadResult {
	t.Helper()
	read, err := eventsSvc.Read(context.Background(), events.ReadRequest{Topic: topic, From: events.Cursor{Topic: topic}, Limit: limit})
	if err != nil {
		t.Fatalf("Read(%q) error = %v", topic, err)
	}
	return read
}

func decodeControlPayload(t *testing.T, record events.Record) workersessions.ControlRecordPayload {
	t.Helper()
	var draft workers.Draft
	if err := json.Unmarshal(record.Payload, &draft); err != nil {
		t.Fatalf("control draft decode error = %v", err)
	}
	if draft.Kind != workers.KindSession || draft.Phase != workers.PhaseUpdated {
		t.Fatalf("control draft = %#v, want SESSION/UPDATED", draft)
	}
	var payload workersessions.ControlRecordPayload
	if err := json.Unmarshal(draft.Payload, &payload); err != nil {
		t.Fatalf("control payload decode error = %v", err)
	}
	if err := payload.Validate(); err != nil {
		t.Fatalf("control payload validation error = %v", err)
	}
	return payload
}

func decodeOrderedControlPayloads(t *testing.T, read events.ReadResult, indexes []int) []workersessions.ControlRecordPayload {
	t.Helper()
	payloads := make([]workersessions.ControlRecordPayload, 0, len(indexes))
	for _, index := range indexes {
		payloads = append(payloads, decodeControlPayload(t, read.Records[index]))
	}
	return payloads
}

func assertPauseResumeControlHistory(t *testing.T, read events.ReadResult, resumedDispatchID string) {
	t.Helper()
	records := decodeOrderedControlPayloads(t, read, []int{1, 2, 3, 5})
	assertControlRequest(t, records[0], workersessions.ControlActionPause, "pause-1")
	assertControlOutcome(t, records[0], records[1], workersessions.ControlOutcomeApplied, "pause")
	assertControlRequest(t, records[2], workersessions.ControlActionResume, "resume-1")
	assertControlOutcome(t, records[2], records[3], workersessions.ControlOutcomeApplied, "resume")
	if records[1].DispatchID != "dispatch-1" || records[3].DispatchID != resumedDispatchID {
		t.Fatalf("control dispatch identities = pause %q, resume %q; want exact attempts", records[1].DispatchID, records[3].DispatchID)
	}
	assertResumeAttemptLineage(t, read.Records[4])
}

func assertControlRequest(t *testing.T, payload workersessions.ControlRecordPayload, action workersessions.ControlAction, requestID string) {
	t.Helper()
	if payload.RecordType != workersessions.ControlRecordTypeRequest {
		t.Fatalf("control request payload = %#v, want request", payload)
	}
	if payload.Action != action {
		t.Fatalf("control request action = %q, want %q", payload.Action, action)
	}
	if payload.RequestID != requestID {
		t.Fatalf("control request ID = %q, want %q", payload.RequestID, requestID)
	}
}

func assertControlOutcome(t *testing.T, request, outcome workersessions.ControlRecordPayload, want workersessions.ControlOutcome, label string) {
	t.Helper()
	if outcome.RecordType != workersessions.ControlRecordTypeOutcome {
		t.Fatalf("%s outcome payload = %#v, want outcome", label, outcome)
	}
	if outcome.Outcome != want {
		t.Fatalf("%s outcome = %q, want %q", label, outcome.Outcome, want)
	}
	if outcome.CorrelationID != request.CorrelationID {
		t.Fatalf("%s correlation = %q, want request correlation %q", label, outcome.CorrelationID, request.CorrelationID)
	}
}

func assertResumeAttemptLineage(t *testing.T, record events.Record) {
	t.Helper()
	resumeAttempt := decodeLineageSessionPayload(t, record)
	if resumeAttempt.AttemptReason != workers.AttemptReasonResume {
		t.Fatalf("resume attempt reason = %q, want RESUME", resumeAttempt.AttemptReason)
	}
	if resumeAttempt.Lineage == nil || resumeAttempt.Lineage.PreviousDispatchID != "dispatch-1" {
		t.Fatalf("resume attempt lineage = %#v, want previous dispatch", resumeAttempt.Lineage)
	}
	if resumeAttempt.Continuation == nil {
		t.Fatalf("resume attempt continuation = nil, want exact continuation")
	}
}

func decodeLineageSessionPayload(t *testing.T, record events.Record) workers.SessionPayload {
	t.Helper()
	var draft workers.Draft
	if err := json.Unmarshal(record.Payload, &draft); err != nil {
		t.Fatalf("session draft decode error = %v", err)
	}
	var payload workers.SessionPayload
	if err := json.Unmarshal(draft.Payload, &payload); err != nil {
		t.Fatalf("session payload decode error = %v", err)
	}
	return payload
}

func assertPortableControlHistory(t *testing.T, sessionID string, topic events.Topic, read events.ReadResult) {
	t.Helper()
	portable, err := (recordings.WorkerRecordingCodec{}).BuildWorkerPortableRecording(recordings.WorkerRecordingSnapshot{
		RecordingID: "recording-control-history",
		Sessions: []recordings.WorkerSessionRecordingSnapshot{{
			WorkerSessionID: sessionID,
			Topic:           topic,
			Status:          recordings.WorkerRecordingStatusCompleted,
			LastPosition:    read.Records[len(read.Records)-1].ID.Position,
			Records:         read.Records,
		}},
	})
	if err != nil {
		t.Fatalf("BuildWorkerPortableRecording() error = %v", err)
	}
	codec := recordings.WorkerRecordingCodec{}
	encoded, err := codec.EncodeWorkerPortableRecording(portable)
	if err != nil {
		t.Fatalf("EncodeWorkerPortableRecording() error = %v", err)
	}
	decoded, err := codec.DecodeWorkerPortableRecording(encoded)
	if err != nil {
		t.Fatalf("DecodeWorkerPortableRecording() error = %v", err)
	}
	if len(decoded.Records) != len(portable.Records) || decoded.Records[1].Payload == nil ||
		decoded.Records[1].SourceType != events.SourceType("worker_session_control") ||
		decoded.Records[5].SourceType != events.SourceType("worker_session_control") {
		t.Fatalf("portable control records = %#v, want exact ordered control records", decoded.Records)
	}
	replayed, err := codec.ReplayWorkerPortableRecording(decoded)
	if err != nil || replayed.Projection.Status != recordings.WorkerRecordingStatusComplete {
		t.Fatalf("ReplayWorkerPortableRecording() = %#v, %v, want complete preserved history", replayed, err)
	}
}

func assertNaturalControlHistory(t *testing.T, eventsSvc events.Service, topic events.Topic) {
	t.Helper()
	read := readControlHistory(t, eventsSvc, topic, 10)
	if len(read.Records) != 4 {
		t.Fatalf("natural control history = %+v, want opening/request/outcome/terminal", read)
	}
	request := decodeControlPayload(t, read.Records[1])
	outcome := decodeControlPayload(t, read.Records[2])
	if request.RecordType != workersessions.ControlRecordTypeRequest || outcome.RecordType != workersessions.ControlRecordTypeOutcome ||
		outcome.Outcome != workersessions.ControlOutcomeNoop || request.CorrelationID != outcome.CorrelationID {
		t.Fatalf("natural control bracket = request %#v outcome %#v, want one matching NOOP bracket", request, outcome)
	}
}

func assertInterruptControlHistory(t *testing.T, eventsSvc events.Service, topic events.Topic) {
	t.Helper()
	read := readControlHistory(t, eventsSvc, topic, 10)
	if len(read.Records) != 5 {
		t.Fatalf("interrupt source history = %+v, want opening/request/outcome/terminal/lineage", read)
	}
	request := decodeControlPayload(t, read.Records[1])
	outcome := decodeControlPayload(t, read.Records[2])
	if request.RecordType != workersessions.ControlRecordTypeRequest || request.Action != workersessions.ControlActionInterrupt ||
		request.RequestID != "interrupt-history-1" || outcome.RecordType != workersessions.ControlRecordTypeOutcome ||
		outcome.Action != workersessions.ControlActionInterrupt || outcome.Outcome != workersessions.ControlOutcomeApplied ||
		request.CorrelationID != outcome.CorrelationID {
		t.Fatalf("interrupt control bracket = request %#v outcome %#v, want applied matching bracket", request, outcome)
	}
	if outcome.DispatchID != "dispatch-source-interrupt" || read.Records[3].ID.Position <= read.Records[2].ID.Position ||
		read.Records[4].SourceType != events.SourceType("worker_session_lineage") {
		t.Fatalf("interrupt ordering/dispatch = outcome %#v terminal position %d, want exact dispatch before terminal", outcome, read.Records[3].ID.Position)
	}
}
