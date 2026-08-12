package service_test

import (
	"context"
	"errors"
	"strings"
	"sync"
	"sync/atomic"
	"testing"

	"github.com/portpowered/infinite-you/pkg/platform/logging"
	"github.com/portpowered/infinite-you/pkg/services/providers"
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
		metadata func(providers.SessionRef) *workers.ProviderSessionMetadata
	}{
		{name: "missing reference"},
		{
			name: "malformed reference",
			metadata: func(reference providers.SessionRef) *workers.ProviderSessionMetadata {
				return &workers.ProviderSessionMetadata{Provider: reference.Provider.String(), Kind: reference.Kind}
			},
		},
		{
			name: "foreign reference",
			metadata: func(reference providers.SessionRef) *workers.ProviderSessionMetadata {
				return &workers.ProviderSessionMetadata{Provider: reference.Provider.String(), Kind: reference.Kind, ID: "foreign-provider-session"}
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
				invalid.Result.ProviderSession = test.metadata(reference)
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

// TestInvokeSession_RetriesARetryableFailureUnderOneWorkerIdentity is the
// attempt loop's core contract.
//
// A retried Worker is still one Worker: its attempts run under the same session
// identity and the same already-open publication window, so a client sees one
// tool call whose content continues rather than a second tool call appearing.
// Only the Workers dispatch identity changes, and it changes to ".../attempt/N"
// so Workers can tell the attempts apart.
func TestInvokeSession_RetriesARetryableFailureUnderOneWorkerIdentity(t *testing.T) {
	var attempts atomic.Int32
	execution := &fakeExecution{
		dispatch: func(_ context.Context, req workers.WorkstationDispatchRequest) (workers.WorkstationDispatchResult, error) {
			if attempts.Add(1) == 1 {
				return retryableFailureResult(req), nil
			}
			return acceptedResult(req), nil
		},
	}
	registry := newRegistryWithExecution(execution)

	req := validStartRequest("worker-1", "dispatch-1")
	req.Retry = workersessions.RetryPolicy{MaxAttempts: 2}
	result, err := registry.InvokeSession(context.Background(), req)
	if err != nil {
		t.Fatalf("InvokeSession: %v", err)
	}
	if result.Session.State != workersessions.StateCompleted {
		t.Fatalf("session state = %q, want COMPLETED after the retry succeeded", result.Session.State)
	}
	if result.Attempts != 2 {
		t.Fatalf("attempts = %d, want 2", result.Attempts)
	}

	requests := execution.requests()
	if len(requests) != 2 {
		t.Fatalf("Workers dispatches = %d, want 2", len(requests))
	}
	if got := requests[0].Execution.Dispatch.DispatchID; got != "dispatch-1" {
		t.Fatalf("first attempt dispatch ID = %q, want the caller's own", got)
	}
	if got := requests[1].Execution.Dispatch.DispatchID; got != "dispatch-1/attempt/2" {
		t.Fatalf("second attempt dispatch ID = %q, want dispatch-1/attempt/2", got)
	}
	if strings.TrimSpace(result.Session.ID) != "worker-1" {
		t.Fatalf("session ID = %q, want the one Worker identity to survive the retry", result.Session.ID)
	}
}

// TestInvokeSession_StopsAtTheAttemptBudget proves the budget is a ceiling on
// attempts, not on retries after the first: a Worker allowed two attempts that
// fails both is terminal, and Workers is not asked a third time.
func TestInvokeSession_StopsAtTheAttemptBudget(t *testing.T) {
	execution := &fakeExecution{
		dispatch: func(_ context.Context, req workers.WorkstationDispatchRequest) (workers.WorkstationDispatchResult, error) {
			return retryableFailureResult(req), nil
		},
	}
	registry := newRegistryWithExecution(execution)

	req := validStartRequest("worker-1", "dispatch-1")
	req.Retry = workersessions.RetryPolicy{MaxAttempts: 2}
	result, err := registry.InvokeSession(context.Background(), req)
	if err != nil {
		t.Fatalf("InvokeSession: %v", err)
	}
	if !result.Session.Terminal() {
		t.Fatalf("session state = %q, want a terminal state once the budget is spent", result.Session.State)
	}
	if execution.callCount() != 2 {
		t.Fatalf("Workers dispatches = %d, want exactly the 2-attempt budget", execution.callCount())
	}
	if result.Attempts != 2 {
		t.Fatalf("attempts = %d, want 2", result.Attempts)
	}
}

// TestInvokeSession_DefaultPolicyIsOneAttempt pins the property that lets one
// operation serve both orchestrators. A Petri dispatch has always been one
// attempt with retryability classified and handed outward for the graph to act
// on; converging JavaScript children onto this call must not quietly give
// every Petri Worker attempt-level retry it never had.
func TestInvokeSession_DefaultPolicyIsOneAttempt(t *testing.T) {
	execution := &fakeExecution{
		dispatch: func(_ context.Context, req workers.WorkstationDispatchRequest) (workers.WorkstationDispatchResult, error) {
			return retryableFailureResult(req), nil
		},
	}
	registry := newRegistryWithExecution(execution)

	result, err := registry.InvokeSession(context.Background(), validStartRequest("worker-1", "dispatch-1"))
	if err != nil {
		t.Fatalf("InvokeSession: %v", err)
	}
	if execution.callCount() != 1 {
		t.Fatalf("Workers dispatches = %d, want 1 for the zero-value retry policy", execution.callCount())
	}
	if result.Attempts != 1 {
		t.Fatalf("attempts = %d, want 1", result.Attempts)
	}
}

// TestInvokeSession_DoesNotRetryATerminalFailure keeps the retry decision
// Workers' own: a failure Workers classifies as terminal is terminal here too,
// because a second opinion would let two orchestrators disagree about the
// identical provider failure.
func TestInvokeSession_DoesNotRetryATerminalFailure(t *testing.T) {
	execution := &fakeExecution{
		dispatch: func(_ context.Context, req workers.WorkstationDispatchRequest) (workers.WorkstationDispatchResult, error) {
			result := retryableFailureResult(req)
			result.Result.FailureMetadata = &workers.WorkFailureMetadata{
				Family: workers.WorkFailureFamilyTerminal,
				Type:   workers.WorkFailureTypePermanentBadRequest,
			}
			return result, nil
		},
	}
	registry := newRegistryWithExecution(execution)

	req := validStartRequest("worker-1", "dispatch-1")
	req.Retry = workersessions.RetryPolicy{MaxAttempts: 5}
	if _, err := registry.InvokeSession(context.Background(), req); err != nil {
		t.Fatalf("InvokeSession: %v", err)
	}
	if execution.callCount() != 1 {
		t.Fatalf("Workers dispatches = %d, want 1; a terminal classification is not retried", execution.callCount())
	}
}

func retryableFailureResult(req workers.WorkstationDispatchRequest) workers.WorkstationDispatchResult {
	dispatchID := req.Execution.Dispatch.DispatchID
	return workers.WorkstationDispatchResult{
		DispatchID:      dispatchID,
		WorkstationName: req.WorkstationName,
		TerminalOutcome: workers.WorkstationDispatchTerminalOutcomeFailed,
		Result: workers.WorkResult{
			DispatchID: dispatchID,
			Outcome:    workers.OutcomeFailed,
			FailureMetadata: &workers.WorkFailureMetadata{
				Family: workers.WorkFailureFamilyRetryable,
				Type:   workers.WorkFailureTypeInternalServerError,
			},
		},
	}
}

func acceptedResult(req workers.WorkstationDispatchRequest) workers.WorkstationDispatchResult {
	dispatchID := req.Execution.Dispatch.DispatchID
	return workers.WorkstationDispatchResult{
		DispatchID:      dispatchID,
		WorkstationName: req.WorkstationName,
		TerminalOutcome: workers.WorkstationDispatchTerminalOutcomeCompleted,
		Result: workers.WorkResult{
			DispatchID: dispatchID,
			Outcome:    workers.OutcomeAccepted,
		},
	}
}

// TestInvokeSession_ControlDuringAnAttemptSpendsNoRetryBudget closes the race
// between the attempt loop and a control.
//
// Every disqualifying condition is checked before the budget, so a Worker a
// control already owns can never consume an attempt: publishing another would
// run provider work for a session that has moved on.
func TestInvokeSession_ControlDuringAnAttemptSpendsNoRetryBudget(t *testing.T) {
	execution := newGatedRetryExecution()
	registry := newRegistryWithExecution(execution)

	req := validStartRequest("worker-1", "dispatch-1")
	req.Retry = workersessions.RetryPolicy{MaxAttempts: 3}
	done := make(chan workersessions.InvokeSessionResult, 1)
	go func() {
		result, err := registry.InvokeSession(context.Background(), req)
		if err != nil {
			t.Errorf("InvokeSession: %v", err)
		}
		done <- result
	}()

	<-execution.entered
	if _, err := registry.Cancel(context.Background(), workersessions.ControlRequest{ID: "worker-1"}); err != nil {
		t.Fatalf("Cancel: %v", err)
	}

	result := <-done
	if !result.Session.Terminal() {
		t.Fatalf("session state = %q, want terminal after the control won", result.Session.State)
	}
	if execution.callCount() != 1 {
		t.Fatalf("Workers dispatches = %d, want 1; a canceled Worker must not consume its retry budget",
			execution.callCount())
	}
}

// gatedRetryExecution holds its first attempt open until a control cancels it,
// so the retry decision is made for a Worker a control already owns.
type gatedRetryExecution struct {
	*fakeExecution

	entered     chan struct{}
	enteredOnce sync.Once
	release     chan struct{}
	releaseOnce sync.Once
}

func newGatedRetryExecution() *gatedRetryExecution {
	gated := &gatedRetryExecution{
		entered: make(chan struct{}),
		release: make(chan struct{}),
	}
	gated.fakeExecution = &fakeExecution{
		dispatch: func(ctx context.Context, req workers.WorkstationDispatchRequest) (workers.WorkstationDispatchResult, error) {
			gated.enteredOnce.Do(func() { close(gated.entered) })
			select {
			case <-gated.release:
			case <-ctx.Done():
			}
			return retryableFailureResult(req), nil
		},
		cancel: func(_ context.Context, req workers.WorkstationDispatchCancelRequest) (workers.WorkstationDispatchCancelResult, error) {
			gated.releaseOnce.Do(func() { close(gated.release) })
			return workers.WorkstationDispatchCancelResult{
				DispatchID: req.DispatchID,
				Outcome:    workers.WorkstationDispatchCancelOutcomeCanceled,
			}, nil
		},
	}
	return gated
}

func TestInterrupt_CancelsExactSourceBeforeAdmittingExactSessionSuccessor(t *testing.T) {
	boundary := newControlledBoundary()
	registry := newControlledRegistry(t, boundary)
	sourceResult := startControlledSession(t, registry, boundary, "source-session", "dispatch-source")
	reference := providers.SessionRef{
		Provider: providers.IDCodex,
		Kind:     providers.SessionIDKind,
		ID:       "provider-session-interrupt",
	}
	if _, err := registry.AssociateProviderSession(context.Background(), workersessions.ProviderSessionAssociationRequest{
		WorkerSessionID: "source-session",
		DispatchID:      "dispatch-source",
		Reference:       reference,
	}); err != nil {
		t.Fatalf("AssociateProviderSession() error = %v", err)
	}
	boundary.setCancel(func(_ context.Context, request workers.WorkstationDispatchCancelRequest) (workers.WorkstationDispatchCancelResult, error) {
		if request.DispatchID != "dispatch-source" {
			t.Errorf("cancel dispatch = %q, want dispatch-source", request.DispatchID)
		}
		if got := boundary.publishCount(); got != 1 {
			t.Errorf("publish count during source cancellation = %d, want 1", got)
		}
		boundary.complete(canceledDispatchResult("dispatch-source"), workers.ErrWorkstationDispatchCanceled)
		return workers.WorkstationDispatchCancelResult{
			DispatchID: "dispatch-source",
			Outcome:    workers.WorkstationDispatchCancelOutcomeCanceled,
		}, nil
	})

	request := workersessions.InterruptRequest{
		RequestID:                "interrupt-request-1",
		SourceWorkerSessionID:    "source-session",
		SuccessorWorkerSessionID: "successor-session",
		ReplacementMessage:       " replacement message ",
	}
	result, err := registry.Interrupt(context.Background(), request)
	if err != nil {
		t.Fatalf("Interrupt() error = %v", err)
	}
	if !result.Accepted || result.Phase != workersessions.InterruptPhaseSuccessorAdmission {
		t.Fatalf("Interrupt() result = %#v, want accepted successor admission", result)
	}
	if result.Source.State != workersessions.StateCanceled || result.Source.SuccessorWorkerSessionID != "successor-session" {
		t.Fatalf("interrupt source = %#v, want CANCELED with successor lineage", result.Source)
	}
	if result.Successor.State != workersessions.StateRunning || result.Successor.PredecessorWorkerSessionID != "source-session" {
		t.Fatalf("interrupt successor = %#v, want RUNNING with predecessor lineage", result.Successor)
	}
	if result.Source.ProviderSessionAssociation == nil || result.Source.ProviderSessionAssociation.Reference != reference {
		t.Fatalf("interrupt source reference = %#v, want exact %v", result.Source.ProviderSessionAssociation, reference)
	}
	if result.Successor.ProviderSessionAssociation == nil || result.Successor.ProviderSessionAssociation.Reference != reference {
		t.Fatalf("interrupt successor reference = %#v, want exact %v", result.Successor.ProviderSessionAssociation, reference)
	}
	cancellations := boundary.cancellations()
	if len(cancellations) != 1 || cancellations[0].DispatchID != "dispatch-source" {
		t.Fatalf("cancellations = %#v, want one exact source dispatch", cancellations)
	}

	handoff := boundary.currentRequest()
	if handoff.Execution.UserMessage != request.ReplacementMessage {
		t.Fatalf("replacement message = %q, want byte-equivalent %q", handoff.Execution.UserMessage, request.ReplacementMessage)
	}
	if handoff.Execution.ResumeSession == nil || *handoff.Execution.ResumeSession != reference {
		t.Fatalf("successor ResumeSession = %#v, want exact %v", handoff.Execution.ResumeSession, reference)
	}
	if handoff.Execution.Dispatch.DispatchID == "dispatch-source" || handoff.Execution.Dispatch.DispatchID == "" {
		t.Fatalf("successor dispatch ID = %q, want distinct non-empty ID", handoff.Execution.Dispatch.DispatchID)
	}
	if sourceResult := <-sourceResult; sourceResult.Session.State != workersessions.StateCanceled {
		t.Fatalf("source InvokeSession() = %#v, want CANCELED", sourceResult.Session)
	}

	boundary.complete(completedDispatchWithProviderSession(handoff.Execution.Dispatch.DispatchID, reference), nil)
	finalSuccessor, err := registry.Get(context.Background(), workersessions.GetRequest{ID: "successor-session"})
	if err != nil {
		t.Fatalf("Get(successor) error = %v", err)
	}
	if finalSuccessor.State != workersessions.StateCompleted {
		t.Fatalf("final successor = %#v, want COMPLETED", finalSuccessor)
	}
}

func TestInterrupt_ValidationRejectsBeforeDispatchEffects(t *testing.T) {
	boundary := newControlledBoundary()
	registry := newControlledRegistry(t, boundary)
	for name, request := range map[string]workersessions.InterruptRequest{
		"missing request ID": {
			SourceWorkerSessionID: "source-session", SuccessorWorkerSessionID: "successor-session", ReplacementMessage: "replacement",
		},
		"same source and successor": {
			RequestID: "interrupt-invalid", SourceWorkerSessionID: "source-session", SuccessorWorkerSessionID: "source-session", ReplacementMessage: "replacement",
		},
		"blank replacement": {
			RequestID: "interrupt-invalid", SourceWorkerSessionID: "source-session", SuccessorWorkerSessionID: "successor-session", ReplacementMessage: "  \t",
		},
	} {
		t.Run(name, func(t *testing.T) {
			result, err := registry.Interrupt(context.Background(), request)
			var interruptErr *workersessions.InterruptError
			if !errors.As(err, &interruptErr) || interruptErr.Phase != workersessions.InterruptPhaseValidation ||
				!errors.Is(err, workersessions.ErrInterruptValidation) || result.Phase != workersessions.InterruptPhaseValidation {
				t.Fatalf("validation Interrupt() = %#v, %v, want VALIDATION", result, err)
			}
		})
	}
	if boundary.publishCount() != 0 || len(boundary.cancellations()) != 0 {
		t.Fatalf("validation effects = publishes %d, cancels %d, want 0/0", boundary.publishCount(), len(boundary.cancellations()))
	}
}

func TestInterrupt_IdempotencyAndRequestConflictAvoidDuplicateEffects(t *testing.T) {
	boundary := newControlledBoundary()
	registry := newControlledRegistry(t, boundary)
	sourceResult := startControlledSession(t, registry, boundary, "source-session", "dispatch-source")
	reference := providers.SessionRef{Provider: providers.IDCodex, Kind: providers.SessionIDKind, ID: "provider-session-idempotent"}
	if _, err := registry.AssociateProviderSession(context.Background(), workersessions.ProviderSessionAssociationRequest{
		WorkerSessionID: "source-session", DispatchID: "dispatch-source", Reference: reference,
	}); err != nil {
		t.Fatalf("AssociateProviderSession() error = %v", err)
	}
	boundary.setCancel(func(_ context.Context, request workers.WorkstationDispatchCancelRequest) (workers.WorkstationDispatchCancelResult, error) {
		boundary.complete(canceledDispatchResult(request.DispatchID), workers.ErrWorkstationDispatchCanceled)
		return workers.WorkstationDispatchCancelResult{DispatchID: request.DispatchID, Outcome: workers.WorkstationDispatchCancelOutcomeCanceled}, nil
	})
	request := workersessions.InterruptRequest{
		RequestID: "interrupt-request-idempotent", SourceWorkerSessionID: "source-session",
		SuccessorWorkerSessionID: "successor-session", ReplacementMessage: "first replacement",
	}
	first, err := registry.Interrupt(context.Background(), request)
	if err != nil || !first.Accepted {
		t.Fatalf("first Interrupt() = %#v, %v, want accepted", first, err)
	}
	firstPublishCount := boundary.publishCount()
	firstCancellationCount := len(boundary.cancellations())
	second, err := registry.Interrupt(context.Background(), request)
	if err != nil || !second.Accepted {
		t.Fatalf("replayed Interrupt() = %#v, %v, want accepted replay", second, err)
	}
	if boundary.publishCount() != firstPublishCount || len(boundary.cancellations()) != firstCancellationCount {
		t.Fatalf("replay effects changed: publishes=%d cancels=%d, want %d/%d", boundary.publishCount(), len(boundary.cancellations()), firstPublishCount, firstCancellationCount)
	}
	conflict := request
	conflict.ReplacementMessage = "different replacement"
	conflictResult, err := registry.Interrupt(context.Background(), conflict)
	var interruptErr *workersessions.InterruptError
	if !errors.As(err, &interruptErr) || !errors.Is(err, workersessions.ErrInterruptValidation) ||
		!errors.Is(err, workersessions.ErrInterruptRequestIDConflict) || conflictResult.Phase != workersessions.InterruptPhaseValidation {
		t.Fatalf("conflicting Interrupt() = %#v, %v, want validation/idempotency conflict", conflictResult, err)
	}
	if boundary.publishCount() != firstPublishCount || len(boundary.cancellations()) != firstCancellationCount {
		t.Fatalf("conflict effects changed: publishes=%d cancels=%d", boundary.publishCount(), len(boundary.cancellations()))
	}
	handoff := boundary.currentRequest()
	boundary.complete(completedDispatchWithProviderSession(handoff.Execution.Dispatch.DispatchID, reference), nil)
	if got := <-sourceResult; got.Session.State != workersessions.StateCanceled {
		t.Fatalf("source InvokeSession() = %#v, want CANCELED", got.Session)
	}
}

func TestInterrupt_ReportsSourceCancellationPhaseWithoutSuccessor(t *testing.T) {
	boundary := newControlledBoundary()
	registry := newControlledRegistry(t, boundary)
	sourceResult := startControlledSession(t, registry, boundary, "source-session", "dispatch-source")
	reference := providers.SessionRef{Provider: providers.IDCodex, Kind: providers.SessionIDKind, ID: "provider-session-cancel-failure"}
	if _, err := registry.AssociateProviderSession(context.Background(), workersessions.ProviderSessionAssociationRequest{
		WorkerSessionID: "source-session", DispatchID: "dispatch-source", Reference: reference,
	}); err != nil {
		t.Fatalf("AssociateProviderSession() error = %v", err)
	}
	boundary.setCancel(func(context.Context, workers.WorkstationDispatchCancelRequest) (workers.WorkstationDispatchCancelResult, error) {
		return workers.WorkstationDispatchCancelResult{}, errors.New("cancel boundary unavailable")
	})
	result, err := registry.Interrupt(context.Background(), workersessions.InterruptRequest{
		RequestID: "interrupt-cancel-failure", SourceWorkerSessionID: "source-session",
		SuccessorWorkerSessionID: "successor-session", ReplacementMessage: "replacement",
	})
	var interruptErr *workersessions.InterruptError
	if !errors.As(err, &interruptErr) || interruptErr.Phase != workersessions.InterruptPhaseSourceCancellation ||
		!errors.Is(err, workersessions.ErrInterruptSourceCancellation) || result.Phase != workersessions.InterruptPhaseSourceCancellation ||
		result.Source.State != workersessions.StateRunning || result.Successor.ID != "" {
		t.Fatalf("source cancellation failure = %#v, %v, want SOURCE_CANCELLATION with running source", result, err)
	}
	if boundary.publishCount() != 1 || len(boundary.cancellations()) != 1 {
		t.Fatalf("source cancellation failure effects = publishes %d, cancels %d, want 1/1", boundary.publishCount(), len(boundary.cancellations()))
	}
	boundary.setCancel(nil)
	boundary.complete(completedDispatchResult("dispatch-source"), nil)
	if got := <-sourceResult; got.Session.State != workersessions.StateCompleted {
		t.Fatalf("source cleanup InvokeSession() = %#v, want COMPLETED", got.Session)
	}
}

func TestInterrupt_ReportsSuccessorAdmissionPhaseAfterSourceCancellation(t *testing.T) {
	boundary := newControlledBoundary()
	registry := newControlledRegistry(t, boundary)
	sourceResult := startControlledSession(t, registry, boundary, "source-session", "dispatch-source")
	reference := providers.SessionRef{Provider: providers.IDCodex, Kind: providers.SessionIDKind, ID: "provider-session-admission-failure"}
	if _, err := registry.AssociateProviderSession(context.Background(), workersessions.ProviderSessionAssociationRequest{
		WorkerSessionID: "source-session", DispatchID: "dispatch-source", Reference: reference,
	}); err != nil {
		t.Fatalf("AssociateProviderSession() error = %v", err)
	}
	boundary.setCancel(func(_ context.Context, request workers.WorkstationDispatchCancelRequest) (workers.WorkstationDispatchCancelResult, error) {
		boundary.complete(canceledDispatchResult(request.DispatchID), workers.ErrWorkstationDispatchCanceled)
		return workers.WorkstationDispatchCancelResult{DispatchID: request.DispatchID, Outcome: workers.WorkstationDispatchCancelOutcomeCanceled}, nil
	})
	boundary.setPublishError(func(call int, _ workers.WorkstationDispatchRequest) error {
		if call == 2 {
			return errors.New("successor admission unavailable")
		}
		return nil
	})
	result, err := registry.Interrupt(context.Background(), workersessions.InterruptRequest{
		RequestID: "interrupt-admission-failure", SourceWorkerSessionID: "source-session",
		SuccessorWorkerSessionID: "successor-session", ReplacementMessage: "replacement",
	})
	var interruptErr *workersessions.InterruptError
	if !errors.As(err, &interruptErr) || interruptErr.Phase != workersessions.InterruptPhaseSuccessorAdmission ||
		!errors.Is(err, workersessions.ErrInterruptSuccessorAdmission) || result.Phase != workersessions.InterruptPhaseSuccessorAdmission ||
		result.Source.State != workersessions.StateCanceled || result.Successor.State != workersessions.StateFailed {
		t.Fatalf("successor admission failure = %#v, %v, want SUCCESSOR_ADMISSION with canceled source and failed successor", result, err)
	}
	if boundary.publishCount() != 2 || len(boundary.cancellations()) != 1 {
		t.Fatalf("successor admission failure effects = publishes %d, cancels %d, want 2/1", boundary.publishCount(), len(boundary.cancellations()))
	}
	if got := <-sourceResult; got.Session.State != workersessions.StateCanceled {
		t.Fatalf("source cleanup InvokeSession() = %#v, want CANCELED", got.Session)
	}
}

func TestInterrupt_ConcurrentIdenticalRequestsReplayOneOperation(t *testing.T) {
	boundary := newControlledBoundary()
	registry := newControlledRegistry(t, boundary)
	sourceResult := startControlledSession(t, registry, boundary, "source-session", "dispatch-source")
	reference := providers.SessionRef{Provider: providers.IDCodex, Kind: providers.SessionIDKind, ID: "provider-session-concurrent"}
	if _, err := registry.AssociateProviderSession(context.Background(), workersessions.ProviderSessionAssociationRequest{
		WorkerSessionID: "source-session", DispatchID: "dispatch-source", Reference: reference,
	}); err != nil {
		t.Fatalf("AssociateProviderSession() error = %v", err)
	}
	boundary.setCancel(func(_ context.Context, request workers.WorkstationDispatchCancelRequest) (workers.WorkstationDispatchCancelResult, error) {
		boundary.complete(canceledDispatchResult(request.DispatchID), workers.ErrWorkstationDispatchCanceled)
		return workers.WorkstationDispatchCancelResult{DispatchID: request.DispatchID, Outcome: workers.WorkstationDispatchCancelOutcomeCanceled}, nil
	})
	request := workersessions.InterruptRequest{
		RequestID: "interrupt-concurrent", SourceWorkerSessionID: "source-session",
		SuccessorWorkerSessionID: "successor-session", ReplacementMessage: "replacement",
	}
	type outcome struct {
		result workersessions.InterruptResult
		err    error
	}
	outcomes := make(chan outcome, 2)
	var group sync.WaitGroup
	group.Add(2)
	for range 2 {
		go func() {
			defer group.Done()
			result, err := registry.Interrupt(context.Background(), request)
			outcomes <- outcome{result: result, err: err}
		}()
	}
	group.Wait()
	close(outcomes)
	for result := range outcomes {
		if result.err != nil || !result.result.Accepted {
			t.Fatalf("concurrent Interrupt() = %#v, %v, want accepted replay", result.result, result.err)
		}
	}
	if got := len(boundary.cancellations()); got != 1 {
		t.Fatalf("concurrent interrupt cancellations = %d, want 1", got)
	}
	if got := boundary.publishCount(); got != 2 {
		t.Fatalf("concurrent interrupt publishes = %d, want source plus one successor", got)
	}
	handoff := boundary.currentRequest()
	boundary.complete(completedDispatchWithProviderSession(handoff.Execution.Dispatch.DispatchID, reference), nil)
	if got := <-sourceResult; got.Session.State != workersessions.StateCanceled {
		t.Fatalf("source cleanup InvokeSession() = %#v, want CANCELED", got.Session)
	}
}
