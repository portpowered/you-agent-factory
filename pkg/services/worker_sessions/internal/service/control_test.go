package service_test

import (
	"context"
	"errors"
	"sync"
	"testing"

	"github.com/portpowered/infinite-you/pkg/platform/logging"
	"github.com/portpowered/infinite-you/pkg/services/providers"
	workersessions "github.com/portpowered/infinite-you/pkg/services/worker_sessions"
	"github.com/portpowered/infinite-you/pkg/services/worker_sessions/internal/service"
	"github.com/portpowered/infinite-you/pkg/services/workers"
)

// controlledBoundary records the exact pool-boundary controls Worker Sessions
// performs and exposes callback completion as deterministic test input. It
// models an accepted asynchronous dispatch without sleeps or polling.
type controlledBoundary struct {
	mu sync.Mutex

	started      chan struct{}
	startedOnce  sync.Once
	accept       workers.WorkstationDispatchAcceptFunc
	request      workers.WorkstationDispatchRequest
	cancelCalls  []workers.WorkstationDispatchCancelRequest
	cancelCalled chan struct{}
	cancel       func(context.Context, workers.WorkstationDispatchCancelRequest) (workers.WorkstationDispatchCancelResult, error)
}

var _ workers.WorkstationPoolBoundary = (*controlledBoundary)(nil)

func newControlledBoundary() *controlledBoundary {
	return &controlledBoundary{started: make(chan struct{}), cancelCalled: make(chan struct{}, 1)}
}

func (*controlledBoundary) Start(context.Context) error { return nil }

func (b *controlledBoundary) Publish(_ context.Context, request workers.WorkstationDispatchRequest, accept workers.WorkstationDispatchAcceptFunc) error {
	return b.PublishWithAdmission(context.Background(), request, nil, accept)
}

func (b *controlledBoundary) PublishWithAdmission(_ context.Context, request workers.WorkstationDispatchRequest, admitted workers.WorkstationDispatchAdmissionFunc, accept workers.WorkstationDispatchAcceptFunc) error {
	b.mu.Lock()
	b.request = request
	b.accept = accept
	b.mu.Unlock()
	b.startedOnce.Do(func() { close(b.started) })
	if admitted != nil {
		admitted()
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

func newControlledRegistry(t *testing.T, boundary workers.WorkstationPoolBoundary) workersessions.Service {
	t.Helper()
	registry, err := service.New(boundary, newEventsAppender(), logging.NoopLogger{})
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

func startControlledSession(t *testing.T, registry workersessions.Service, boundary *controlledBoundary, id, dispatchID string) <-chan workersessions.StartResult {
	t.Helper()
	result := make(chan workersessions.StartResult, 1)
	go func() {
		started, err := registry.Start(context.Background(), validStartRequest(id, dispatchID))
		if err != nil {
			t.Errorf("Start() error = %v", err)
		}
		result <- started
	}()
	<-boundary.started
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

func TestTerminate_AfterCancelWaitsForTheEstablishedTerminalCallback(t *testing.T) {
	boundary := newControlledBoundary()
	registry := newControlledRegistry(t, boundary)
	started := startControlledSession(t, registry, boundary, "worker-1", "dispatch-1")
	boundary.setCancel(func(context.Context, workers.WorkstationDispatchCancelRequest) (workers.WorkstationDispatchCancelResult, error) {
		return workers.WorkstationDispatchCancelResult{
			DispatchID: "dispatch-1", Outcome: workers.WorkstationDispatchCancelOutcomeCanceled,
		}, nil
	})

	canceled, err := registry.Cancel(context.Background(), workersessions.ControlRequest{ID: "worker-1"})
	if err != nil || canceled.Outcome != workersessions.ControlOutcomeApplied || canceled.Session.State != workersessions.StateRunning {
		t.Fatalf("Cancel() = %#v, %v, want applied control while callback remains pending", canceled, err)
	}

	terminated := make(chan workersessions.ControlResult, 1)
	terminateErr := make(chan error, 1)
	go func() {
		result, callErr := registry.Terminate(context.Background(), workersessions.ControlRequest{ID: "worker-1"})
		terminated <- result
		terminateErr <- callErr
	}()
	select {
	case result := <-terminated:
		t.Fatalf("Terminate() returned before the prior Cancel callback: %#v", result)
	default:
	}

	boundary.complete(canceledDispatchResult("dispatch-1"), workers.ErrWorkstationDispatchCanceled)
	result := <-terminated
	if callErr := <-terminateErr; callErr != nil || result.Outcome != workersessions.ControlOutcomeNoop || result.Session.State != workersessions.StateCanceled {
		t.Fatalf("Terminate() = %#v, %v, want joined CANCELED NOOP", result, callErr)
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
	if continuation.Execution.ResumeSession == nil || *continuation.Execution.ResumeSession != reference {
		t.Fatalf("continuation ResumeSession = %#v, want exact %#v", continuation.Execution.ResumeSession, reference)
	}
	if continuation.WorkstationName != initial.WorkstationName || continuation.Execution.Dispatch.Execution.RequestID != initial.Execution.Dispatch.Execution.RequestID {
		t.Fatalf("continuation correlation = %#v, want preserved workstation and turn request", continuation)
	}
	if continuation.Execution.Dispatch.DispatchID == "dispatch-1" || continuation.Execution.Dispatch.DispatchID != resumed.DispatchID {
		t.Fatalf("continuation dispatch = %q, want fresh resumed dispatch %q", continuation.Execution.Dispatch.DispatchID, resumed.DispatchID)
	}

	resumedResult := completedDispatchResult(resumed.DispatchID)
	resumedResult.Result.ProviderSession = &workers.ProviderSessionMetadata{
		Provider: reference.Provider.String(),
		Kind:     reference.Kind,
		ID:       reference.ID,
	}
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
			started, err := registry.Start(context.Background(), validStartRequest("worker-1", "dispatch-1"))
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

func TestCancel_BoundaryAlreadyCanceledReturnsNoopWithoutChangingRunningSession(t *testing.T) {
	boundary := newControlledBoundary()
	registry := newControlledRegistry(t, boundary)
	started := startControlledSession(t, registry, boundary, "worker-1", "dispatch-1")
	boundary.setCancel(func(context.Context, workers.WorkstationDispatchCancelRequest) (workers.WorkstationDispatchCancelResult, error) {
		return workers.WorkstationDispatchCancelResult{
			DispatchID: "dispatch-1", Outcome: workers.WorkstationDispatchCancelOutcomeAlreadyCanceled,
		}, nil
	})

	result, err := registry.Cancel(context.Background(), workersessions.ControlRequest{ID: "worker-1"})
	if err != nil || result.Outcome != workersessions.ControlOutcomeNoop || result.Session.State != workersessions.StateRunning {
		t.Fatalf("Cancel() = %#v, %v, want NOOP with unchanged RUNNING session", result, err)
	}
	boundary.complete(completedDispatchResult("dispatch-1"), nil)
	if final := <-started; final.Session.State != workersessions.StateCompleted {
		t.Fatalf("Start() after ordinary completion = %#v, want COMPLETED", final)
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
