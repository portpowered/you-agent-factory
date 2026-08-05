package service_test

import (
	"context"
	"errors"
	"sync"
	"testing"

	"github.com/portpowered/infinite-you/pkg/platform/logging"
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
	b.mu.Lock()
	b.request = request
	b.accept = accept
	b.mu.Unlock()
	b.startedOnce.Do(func() { close(b.started) })
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

func TestControl_UnknownAndInvalidIdentityRemainDistinguishable(t *testing.T) {
	registry := newControlledRegistry(t, newControlledBoundary())
	_, invalidErr := registry.Cancel(context.Background(), workersessions.ControlRequest{ID: " "})
	_, unknownErr := registry.Cancel(context.Background(), workersessions.ControlRequest{ID: "missing"})
	if !errors.Is(invalidErr, workersessions.ErrInvalidSessionID) || !errors.Is(unknownErr, workersessions.ErrSessionNotFound) {
		t.Fatalf("invalid=%v unknown=%v, want distinct typed errors", invalidErr, unknownErr)
	}
}
