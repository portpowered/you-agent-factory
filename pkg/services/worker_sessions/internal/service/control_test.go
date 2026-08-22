package service_test

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/portpowered/infinite-you/pkg/platform/logging"
	"github.com/portpowered/infinite-you/pkg/services/events"
	"github.com/portpowered/infinite-you/pkg/services/providers"
	"github.com/portpowered/infinite-you/pkg/services/recordings"
	workersessions "github.com/portpowered/infinite-you/pkg/services/worker_sessions"
	"github.com/portpowered/infinite-you/pkg/services/workers"
)

const controlledBoundaryWaitTimeout = 2 * time.Second

var errControlledBoundaryCompletionTimeout = errors.New("controlled boundary completion signal not received")

type controlledDispatch struct {
	prepared      chan struct{}
	preparedOnce  sync.Once
	completed     chan struct{}
	completedOnce sync.Once
	returned      chan struct{}
	returnedOnce  sync.Once
	request       workers.WorkstationDispatchRequest
	result        workers.WorkstationDispatchResult
	err           error
}

func newControlledDispatch() *controlledDispatch {
	return &controlledDispatch{
		prepared:  make(chan struct{}),
		completed: make(chan struct{}),
		returned:  make(chan struct{}),
	}
}

// controlledBoundary records the exact Workers execution controls Worker
// Sessions performs and exposes completion as deterministic test input. It
// models an accepted asynchronous dispatch without sleeps or polling.
type controlledBoundary struct {
	mu sync.Mutex

	started            chan struct{}
	startedOnce        sync.Once
	admitted           chan struct{}
	admittedOnce       sync.Once
	request            workers.WorkstationDispatchRequest
	publishCalls       int
	publishError       func(int, workers.WorkstationDispatchRequest) error
	dispatches         map[string]*controlledDispatch
	dispatchesChanged  chan struct{}
	cancelled          []workers.WorkstationDispatchCancelRequest
	cancelObserved     chan struct{}
	cancelObservedOnce sync.Once
	ignoreCancellation bool
	waitTimeout        time.Duration
}

func newControlledBoundary() *controlledBoundary {
	return newControlledBoundaryWithTimeout(controlledBoundaryWaitTimeout)
}

func newControlledBoundaryWithTimeout(timeout time.Duration) *controlledBoundary {
	if timeout <= 0 {
		timeout = controlledBoundaryWaitTimeout
	}
	return &controlledBoundary{
		started:           make(chan struct{}),
		admitted:          make(chan struct{}),
		cancelObserved:    make(chan struct{}),
		dispatches:        make(map[string]*controlledDispatch),
		dispatchesChanged: make(chan struct{}),
		waitTimeout:       timeout,
	}
}

func (b *controlledBoundary) DispatchWorkstation(ctx context.Context, request workers.WorkstationDispatchRequest) (workers.WorkstationDispatchResult, error) {
	return b.DispatchWorkstationWithAdmission(ctx, request, nil)
}

func (b *controlledBoundary) DispatchWorkstationWithAdmission(ctx context.Context, request workers.WorkstationDispatchRequest, admitted workers.WorkstationDispatchAdmissionFunc) (workers.WorkstationDispatchResult, error) {
	dispatch, err := b.prepare(request)
	if err != nil {
		return workers.WorkstationDispatchResult{}, err
	}
	if admitted != nil {
		admitted()
		b.admittedOnce.Do(func() { close(b.admitted) })
	}
	return b.await(ctx, dispatch, request.Execution.Dispatch.DispatchID)
}

func (b *controlledBoundary) await(ctx context.Context, dispatch *controlledDispatch, dispatchID string) (workers.WorkstationDispatchResult, error) {
	defer dispatch.returnedOnce.Do(func() { close(dispatch.returned) })
	if ctx == nil {
		ctx = context.Background()
	}
	timer := time.NewTimer(b.waitTimeout)
	defer timer.Stop()
	select {
	case <-dispatch.completed:
	case <-ctx.Done():
		b.recordCancellation(dispatchID)
		b.mu.Lock()
		ignoreCancellation := b.ignoreCancellation
		b.mu.Unlock()
		if ignoreCancellation {
			select {
			case <-dispatch.completed:
			case <-timer.C:
				return controlledBoundaryTimeoutResult(dispatchID), controlledBoundaryTimeoutError(dispatchID)
			}
			return b.dispatchResult(dispatch)
		}
		return canceledDispatchResult(dispatchID), ctx.Err()
	case <-timer.C:
		return controlledBoundaryTimeoutResult(dispatchID), controlledBoundaryTimeoutError(dispatchID)
	}
	return b.dispatchResult(dispatch)
}

func (b *controlledBoundary) recordCancellation(dispatchID string) {
	b.mu.Lock()
	b.cancelled = append(b.cancelled, workers.WorkstationDispatchCancelRequest{DispatchID: dispatchID})
	b.mu.Unlock()
	b.cancelObservedOnce.Do(func() { close(b.cancelObserved) })
}

func (b *controlledBoundary) cancellations() []workers.WorkstationDispatchCancelRequest {
	b.mu.Lock()
	defer b.mu.Unlock()
	return append([]workers.WorkstationDispatchCancelRequest(nil), b.cancelled...)
}

func (b *controlledBoundary) cancellationObserved() <-chan struct{} {
	return b.cancelObserved
}

func (b *controlledBoundary) setIgnoreCancellation(ignore bool) {
	b.mu.Lock()
	b.ignoreCancellation = ignore
	b.mu.Unlock()
}

func (b *controlledBoundary) prepare(request workers.WorkstationDispatchRequest) (*controlledDispatch, error) {
	dispatchID := request.Execution.Dispatch.DispatchID
	b.mu.Lock()
	dispatch := b.dispatches[dispatchID]
	if dispatch == nil {
		dispatch = newControlledDispatch()
		b.dispatches[dispatchID] = dispatch
		close(b.dispatchesChanged)
		b.dispatchesChanged = make(chan struct{})
	}
	select {
	case <-dispatch.prepared:
		b.mu.Unlock()
		return nil, fmt.Errorf("controlled boundary duplicate preparation for dispatch %q", dispatchID)
	default:
	}
	b.request = request
	b.publishCalls++
	publishCall := b.publishCalls
	publishError := b.publishError
	dispatch.request = request
	b.startedOnce.Do(func() { close(b.started) })
	dispatch.preparedOnce.Do(func() { close(dispatch.prepared) })
	b.mu.Unlock()
	if publishError != nil {
		if err := publishError(publishCall, request); err != nil {
			return dispatch, err
		}
	}
	return dispatch, nil
}

func (b *controlledBoundary) complete(result workers.WorkstationDispatchResult, err error) {
	dispatchID := result.DispatchID
	if dispatchID == "" {
		dispatchID = result.Result.DispatchID
	}
	if dispatchID == "" {
		panic("controlled boundary complete: dispatch ID is required")
	}
	b.mu.Lock()
	dispatch := b.dispatches[dispatchID]
	if dispatch == nil {
		dispatch = newControlledDispatch()
		b.dispatches[dispatchID] = dispatch
		close(b.dispatchesChanged)
		b.dispatchesChanged = make(chan struct{})
	}
	dispatch.result, dispatch.err = result, err
	b.mu.Unlock()
	dispatch.completedOnce.Do(func() { close(dispatch.completed) })
	if err := waitControlledSignal(dispatch.returned, b.waitTimeout); err != nil {
		panic(fmt.Sprintf("controlled boundary complete(%q): %v", dispatchID, err))
	}
}

func (b *controlledBoundary) dispatchResult(dispatch *controlledDispatch) (workers.WorkstationDispatchResult, error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	return dispatch.result, dispatch.err
}

func controlledBoundaryTimeoutError(dispatchID string) error {
	return fmt.Errorf("%w: dispatch %q", errControlledBoundaryCompletionTimeout, dispatchID)
}

func controlledBoundaryTimeoutResult(dispatchID string) workers.WorkstationDispatchResult {
	err := controlledBoundaryTimeoutError(dispatchID)
	return workers.WorkstationDispatchResult{
		DispatchID:      dispatchID,
		TerminalOutcome: workers.WorkstationDispatchTerminalOutcomeFailed,
		Result: workers.WorkResult{
			DispatchID: dispatchID,
			Outcome:    workers.OutcomeFailed,
			Error:      err.Error(),
		},
	}
}

func waitControlledSignal(signal <-chan struct{}, timeout time.Duration) error {
	timer := time.NewTimer(timeout)
	defer timer.Stop()
	select {
	case <-signal:
		return nil
	case <-timer.C:
		return fmt.Errorf("%w after %s", errControlledBoundaryCompletionTimeout, timeout)
	}
}

func (b *controlledBoundary) requestFor(t *testing.T, dispatchID string) workers.WorkstationDispatchRequest {
	t.Helper()
	dispatch, err := b.dispatchFor(dispatchID)
	if err != nil {
		t.Fatalf("controlled boundary dispatch %q was not prepared: %v", dispatchID, err)
	}
	if err := waitControlledSignal(dispatch.prepared, b.waitTimeout); err != nil {
		t.Fatalf("controlled boundary dispatch %q readiness: %v", dispatchID, err)
	}
	b.mu.Lock()
	defer b.mu.Unlock()
	return workers.WorkstationDispatchRequest{
		WorkstationName: dispatch.request.WorkstationName,
		Execution:       workers.CloneWorkstationExecutionRequest(dispatch.request.Execution),
	}
}

func (b *controlledBoundary) dispatchFor(dispatchID string) (*controlledDispatch, error) {
	timer := time.NewTimer(b.waitTimeout)
	defer timer.Stop()
	for {
		b.mu.Lock()
		dispatch := b.dispatches[dispatchID]
		changed := b.dispatchesChanged
		b.mu.Unlock()
		if dispatch != nil {
			return dispatch, nil
		}
		select {
		case <-changed:
		case <-timer.C:
			return nil, controlledBoundaryTimeoutError(dispatchID)
		}
	}
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

func (b *controlledBoundary) setPublishError(fn func(int, workers.WorkstationDispatchRequest) error) {
	b.mu.Lock()
	b.publishError = fn
	b.mu.Unlock()
}

func newControlledRegistry(t *testing.T, execution any) workersessions.Service {
	t.Helper()
	registry, err := newService(execution, newEventsAppender(), logging.NoopLogger{})
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
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	go func() {
		defer close(done)
		started, err := registry.InvokeSession(ctx, validStartRequest(id, dispatchID))
		if err != nil {
			t.Errorf("Start() error = %v", err)
		}
		result <- started
	}()
	t.Cleanup(func() {
		cancel()
		if err := waitControlledSignal(done, boundary.waitTimeout); err != nil {
			t.Errorf("controlled Start() goroutine did not join: %v", err)
		}
	})
	if err := waitControlledSignal(boundary.admitted, boundary.waitTimeout); err != nil {
		t.Fatalf("controlled Start() admission: %v", err)
	}
	return result
}

func TestControlledBoundary_MissingCompletionFailsWithDispatchDiagnostic(t *testing.T) {
	boundary := newControlledBoundaryWithTimeout(10 * time.Millisecond)
	dispatch, err := boundary.prepare(validStartRequest("worker-1", "dispatch-missing").Execution)
	if err != nil {
		t.Fatalf("prepare() error = %v", err)
	}
	_, err = boundary.await(context.Background(), dispatch, "dispatch-missing")
	if !errors.Is(err, errControlledBoundaryCompletionTimeout) ||
		!strings.Contains(err.Error(), `dispatch "dispatch-missing"`) {
		t.Fatalf("await() error = %v, want named dispatch completion timeout", err)
	}
}

func TestCancel_UsesServerOwnedAttemptContextDespiteCanceledCallerContextAndIsIdempotent(t *testing.T) {
	boundary := newControlledBoundary()
	registry := newControlledRegistry(t, boundary)
	started := startControlledSession(t, registry, boundary, "worker-1", "dispatch-1")

	ctx, cancel := context.WithCancel(context.Background())
	cancel()

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
}

// Cancel resolves a session by its stable Worker Session ID and never by the
// Provider Session it may not have published yet. This asserts the
// pre-association window explicitly -- providerSessionAvailable is false on the
// observation route while the session is RUNNING -- because that is the exact
// window an operator reported as uncancellable.
func TestCancel_RunningSessionWithoutAProviderSessionIsCancelledByItsWorkerSessionID(t *testing.T) {
	boundary := newControlledBoundary()
	registry := newControlledRegistry(t, boundary)
	started := startControlledSession(t, registry, boundary, "worker-1", "dispatch-1")

	observed, err := registry.GetObservationByWorkerSessionID(
		context.Background(),
		workersessions.GetObservationByWorkerSessionIDRequest{WorkerSessionID: "worker-1"},
	)
	if err != nil {
		t.Fatalf("GetObservationByWorkerSessionID() error = %v", err)
	}
	if observed.State != workersessions.StateRunning || observed.ProviderSessionAvailable {
		t.Fatalf("observation = state %q providerSessionAvailable %t, want RUNNING without a provider session",
			observed.State, observed.ProviderSessionAvailable)
	}

	result, err := registry.Cancel(context.Background(), workersessions.ControlRequest{ID: observed.WorkerSessionID})
	if err != nil {
		t.Fatalf("Cancel() by observed Worker Session ID error = %v, want no error", err)
	}
	if errors.Is(err, workersessions.ErrSessionNotFound) {
		t.Fatalf("Cancel() = %v, want the session resolved by its stable identity", err)
	}
	if result.Outcome != workersessions.ControlOutcomeApplied || result.Session.State != workersessions.StateCanceled {
		t.Fatalf("Cancel() = %#v, want APPLIED with a CANCELED session", result)
	}
	if got := <-started; got.Session.State != workersessions.StateCanceled {
		t.Fatalf("Start() result after cancel = %#v, want CANCELED", got.Session)
	}

	repeated, err := registry.Cancel(context.Background(), workersessions.ControlRequest{ID: "worker-1"})
	if err != nil || repeated.Outcome != workersessions.ControlOutcomeNoop {
		t.Fatalf("repeated Cancel() = %#v, %v, want NOOP without an error", repeated, err)
	}
}

func TestControl_UnsupportedPauseResumeAndCanonicalCancellationLeaveLifecycleTruthful(t *testing.T) {
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

	canceled, err := registry.Cancel(context.Background(), workersessions.ControlRequest{ID: "worker-1"})
	if err != nil || canceled.Outcome != workersessions.ControlOutcomeApplied || canceled.Session.State != workersessions.StateCanceled {
		t.Fatalf("Cancel() canonical context cancellation = %#v, %v, want applied CANCELED", canceled, err)
	}
	if got := <-started; got.Session.State != workersessions.StateCanceled {
		t.Fatalf("Start() after cancellation = %#v, want CANCELED", got.Session)
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
	continuation := boundary.requestFor(t, resumed.DispatchID)
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
			if paused, err := registry.Pause(context.Background(), workersessions.ControlRequest{ID: "worker-1"}); err != nil || paused.Session.State != workersessions.StatePaused {
				t.Fatalf("Pause() = %#v, %v, want PAUSED", paused, err)
			}

			result, err := test.call(registry, context.Background(), workersessions.ControlRequest{ID: "worker-1"})
			if err != nil || result.Outcome != workersessions.ControlOutcomeApplied || result.Session.State != test.state {
				t.Fatalf("%s() = %#v, %v, want applied %s", test.name, result, err, test.state)
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
