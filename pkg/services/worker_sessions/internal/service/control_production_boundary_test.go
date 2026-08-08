package service_test

import (
	"context"
	"errors"
	"sync"
	"testing"

	workersessions "github.com/portpowered/infinite-you/pkg/services/worker_sessions"
	"github.com/portpowered/infinite-you/pkg/services/workers"
)

// admissionControlledExecution is a controlled Workers execution service used
// only behind the real asynchronous WorkstationPoolBoundary. Its channels make
// the production boundary's admission barrier and terminal/control race
// observable without sleeps or polling.
type admissionControlledExecution struct {
	mu sync.Mutex

	dispatchEntered     chan struct{}
	dispatchEnteredOnce sync.Once
	allowAdmission      chan struct{}
	admitted            chan struct{}
	admittedOnce        sync.Once
	cancelCalls         chan workers.WorkstationDispatchCancelRequest
	complete            chan struct{}
	completionCommitted chan struct{}
	completionOnce      sync.Once
	completionWins      bool
	alreadyCanceled     bool
	cancellations       int
	completeOnce        sync.Once
}

var _ workers.WorkstationExecutionService = (*admissionControlledExecution)(nil)

func newAdmissionControlledExecution(completionWins bool) *admissionControlledExecution {
	return &admissionControlledExecution{
		dispatchEntered:     make(chan struct{}),
		allowAdmission:      make(chan struct{}),
		admitted:            make(chan struct{}),
		cancelCalls:         make(chan workers.WorkstationDispatchCancelRequest, 1),
		complete:            make(chan struct{}),
		completionCommitted: make(chan struct{}),
		completionWins:      completionWins,
	}
}

func (*admissionControlledExecution) StartWorkstationPool(
	context.Context,
	workers.WorkstationPoolStartRequest,
) (workers.WorkstationPoolStartResult, error) {
	return workers.WorkstationPoolStartResult{Outcome: workers.WorkstationPoolLifecycleOutcomeStarted}, nil
}

func (*admissionControlledExecution) StopWorkstationPool(
	context.Context,
) (workers.WorkstationPoolStopResult, error) {
	return workers.WorkstationPoolStopResult{Outcome: workers.WorkstationPoolLifecycleOutcomeStopped}, nil
}

func (e *admissionControlledExecution) DispatchWorkstation(
	ctx context.Context,
	request workers.WorkstationDispatchRequest,
) (workers.WorkstationDispatchResult, error) {
	return e.DispatchWorkstationWithAdmission(ctx, request, nil)
}

func (e *admissionControlledExecution) DispatchWorkstationWithAdmission(
	ctx context.Context,
	request workers.WorkstationDispatchRequest,
	admitted workers.WorkstationDispatchAdmissionFunc,
) (workers.WorkstationDispatchResult, error) {
	e.dispatchEnteredOnce.Do(func() { close(e.dispatchEntered) })
	select {
	case <-e.allowAdmission:
	case <-ctx.Done():
		return workers.WorkstationDispatchResult{}, ctx.Err()
	}
	if admitted != nil {
		admitted()
	}
	e.admittedOnce.Do(func() { close(e.admitted) })
	select {
	case <-e.complete:
	case <-ctx.Done():
		return workers.WorkstationDispatchResult{}, ctx.Err()
	}
	e.completionOnce.Do(func() { close(e.completionCommitted) })
	result := completedDispatchResult(request.Execution.Dispatch.DispatchID)
	if e.completionWins {
		return result, nil
	}
	return canceledDispatchResult(request.Execution.Dispatch.DispatchID), workers.ErrWorkstationDispatchCanceled
}

func (e *admissionControlledExecution) CancelWorkstationDispatch(
	_ context.Context,
	request workers.WorkstationDispatchCancelRequest,
) (workers.WorkstationDispatchCancelResult, error) {
	e.mu.Lock()
	e.cancellations++
	alreadyCanceled := e.alreadyCanceled && e.cancellations > 1
	firstCancellation := e.alreadyCanceled && e.cancellations == 1
	e.mu.Unlock()
	e.cancelCalls <- request
	if alreadyCanceled {
		return workers.WorkstationDispatchCancelResult{
			DispatchID: request.DispatchID,
			Outcome:    workers.WorkstationDispatchCancelOutcomeAlreadyCanceled,
		}, nil
	}
	if firstCancellation {
		return workers.WorkstationDispatchCancelResult{
			DispatchID: request.DispatchID,
			Outcome:    workers.WorkstationDispatchCancelOutcomeCanceled,
		}, nil
	}
	e.completeOnce.Do(func() { close(e.complete) })
	if e.completionWins {
		<-e.completionCommitted
		return workers.WorkstationDispatchCancelResult{
			DispatchID: request.DispatchID,
			Outcome:    workers.WorkstationDispatchCancelOutcomeAlreadyTerminal,
		}, workers.ErrWorkstationDispatchAlreadyTerminal
	}
	return workers.WorkstationDispatchCancelResult{
		DispatchID: request.DispatchID,
		Outcome:    workers.WorkstationDispatchCancelOutcomeCanceled,
	}, nil
}

func newProductionBoundaryRegistry(t *testing.T, execution workers.WorkstationExecutionService) workersessions.Service {
	t.Helper()
	boundary := workers.NewWorkstationPoolBoundary(workers.WorkstationPoolBoundaryConfig{
		Service:    execution,
		RouteNames: []string{"review"},
		Async:      true,
	})
	return newControlledRegistry(t, boundary)
}

func newSynchronousProductionBoundaryRegistry(t *testing.T, execution workers.WorkstationExecutionService) workersessions.Service {
	t.Helper()
	boundary := workers.NewWorkstationPoolBoundary(workers.WorkstationPoolBoundaryConfig{
		Service:    execution,
		RouteNames: []string{"review"},
		Async:      false,
	})
	return newControlledRegistry(t, boundary)
}

func TestCancel_WaitsForProductionAsyncBoundaryAdmissionBeforeExactCancellation(t *testing.T) {
	execution := newAdmissionControlledExecution(false)
	registry := newProductionBoundaryRegistry(t, execution)
	started := make(chan workersessions.InvokeSessionResult, 1)
	go func() {
		result, err := registry.InvokeSession(context.Background(), validStartRequest("worker-1", "dispatch-1"))
		if err != nil {
			t.Errorf("Start() error = %v", err)
		}
		started <- result
	}()
	<-execution.dispatchEntered

	cancelled := make(chan workersessions.ControlResult, 1)
	cancelErr := make(chan error, 1)
	go func() {
		result, err := registry.Cancel(context.Background(), workersessions.ControlRequest{ID: "worker-1"})
		cancelled <- result
		cancelErr <- err
	}()
	select {
	case call := <-execution.cancelCalls:
		t.Fatalf("Cancel reached dispatch %q before Workers admission", call.DispatchID)
	default:
	}

	close(execution.allowAdmission)
	result := <-cancelled
	if err := <-cancelErr; err != nil || result.Outcome != workersessions.ControlOutcomeApplied {
		t.Fatalf("Cancel() = %#v, %v, want applied after admission", result, err)
	}
	call := <-execution.cancelCalls
	if call.DispatchID != "dispatch-1" {
		t.Fatalf("Cancel dispatch ID = %q, want dispatch-1", call.DispatchID)
	}
	if final := <-started; final.Session.State != workersessions.StateCanceled ||
		final.Dispatch.TerminalOutcome != workers.WorkstationDispatchTerminalOutcomeCanceled {
		t.Fatalf("Start() after admitted cancellation = %#v, want canceled terminal result", final)
	}
}

func TestCancel_ProductionBoundaryCompletionWinReturnsNoopAndPreservesCompletedSession(t *testing.T) {
	execution := newAdmissionControlledExecution(true)
	registry := newProductionBoundaryRegistry(t, execution)
	started := make(chan workersessions.InvokeSessionResult, 1)
	go func() {
		result, err := registry.InvokeSession(context.Background(), validStartRequest("worker-1", "dispatch-1"))
		if err != nil {
			t.Errorf("Start() error = %v", err)
		}
		started <- result
	}()
	<-execution.dispatchEntered
	close(execution.allowAdmission)

	result, err := registry.Cancel(context.Background(), workersessions.ControlRequest{ID: "worker-1"})
	if err != nil || result.Outcome != workersessions.ControlOutcomeNoop {
		t.Fatalf("Cancel() = %#v, %v, want completion-wins NOOP", result, err)
	}
	if result.Session.State != workersessions.StateCompleted {
		t.Fatalf("Cancel() session state = %q, want COMPLETED", result.Session.State)
	}
	if call := <-execution.cancelCalls; call.DispatchID != "dispatch-1" {
		t.Fatalf("Cancel dispatch ID = %q, want dispatch-1", call.DispatchID)
	}
	final := <-started
	if final.Session.State != workersessions.StateCompleted ||
		final.Dispatch.TerminalOutcome != workers.WorkstationDispatchTerminalOutcomeCompleted {
		t.Fatalf("Start() after completion win = %#v, want completed terminal result", final)
	}
	if final.DispatchErr != nil {
		t.Fatalf("Start() dispatch error = %v, want nil", final.DispatchErr)
	}
}

func TestTerminate_ProductionSynchronousBoundaryCancelsAdmittedDispatchBeforePublishReturns(t *testing.T) {
	execution := newAdmissionControlledExecution(false)
	registry := newSynchronousProductionBoundaryRegistry(t, execution)
	started := make(chan workersessions.InvokeSessionResult, 1)
	startErr := make(chan error, 1)
	go func() {
		result, err := registry.InvokeSession(context.Background(), validStartRequest("worker-1", "dispatch-1"))
		started <- result
		startErr <- err
	}()
	<-execution.dispatchEntered
	close(execution.allowAdmission)
	<-execution.admitted

	select {
	case <-execution.complete:
		t.Fatal("synchronous dispatch completed before explicit control")
	default:
	}

	terminated := make(chan workersessions.ControlResult, 1)
	terminateErr := make(chan error, 1)
	go func() {
		result, err := registry.Terminate(context.Background(), workersessions.ControlRequest{ID: "worker-1"})
		terminated <- result
		terminateErr <- err
	}()
	if call := <-execution.cancelCalls; call.DispatchID != "dispatch-1" {
		t.Fatalf("Terminate dispatch ID = %q, want dispatch-1", call.DispatchID)
	}
	result := <-terminated
	if err := <-terminateErr; err != nil || result.Outcome != workersessions.ControlOutcomeApplied || result.Session.State != workersessions.StateTerminated {
		t.Fatalf("Terminate() = %#v, %v, want applied TERMINATED result", result, err)
	}
	final := <-started
	if err := <-startErr; err != nil {
		t.Fatalf("Start() error = %v, want nil", err)
	}
	if final.Session.State != workersessions.StateTerminated || final.Dispatch.TerminalOutcome != workers.WorkstationDispatchTerminalOutcomeCanceled || !errors.Is(final.DispatchErr, workers.ErrWorkstationDispatchCanceled) {
		t.Fatalf("Start() after synchronous cancellation = %#v, want one terminated canceled result", final)
	}
}

func TestTerminate_ProductionBoundaryAlreadyCanceledJoinsHeldCallback(t *testing.T) {
	execution := newAdmissionControlledExecution(false)
	execution.alreadyCanceled = true
	boundary := workers.NewWorkstationPoolBoundary(workers.WorkstationPoolBoundaryConfig{
		Service:    execution,
		RouteNames: []string{"review"},
		Async:      true,
	})
	registry := newControlledRegistry(t, boundary)
	started := make(chan workersessions.InvokeSessionResult, 1)
	startErr := make(chan error, 1)
	go func() {
		result, err := registry.InvokeSession(context.Background(), validStartRequest("worker-1", "dispatch-1"))
		started <- result
		startErr <- err
	}()
	<-execution.dispatchEntered
	close(execution.allowAdmission)
	<-execution.admitted

	first, err := boundary.Cancel(context.Background(), workers.WorkstationDispatchCancelRequest{DispatchID: "dispatch-1"})
	if err != nil || first.Outcome != workers.WorkstationDispatchCancelOutcomeCanceled {
		t.Fatalf("first boundary Cancel() = %#v, %v, want committed cancellation", first, err)
	}
	if call := <-execution.cancelCalls; call.DispatchID != "dispatch-1" {
		t.Fatalf("first boundary cancel dispatch ID = %q, want dispatch-1", call.DispatchID)
	}

	terminated := make(chan workersessions.ControlResult, 1)
	terminateErr := make(chan error, 1)
	go func() {
		result, err := registry.Terminate(context.Background(), workersessions.ControlRequest{ID: "worker-1"})
		terminated <- result
		terminateErr <- err
	}()
	if call := <-execution.cancelCalls; call.DispatchID != "dispatch-1" {
		t.Fatalf("Terminate boundary cancel dispatch ID = %q, want dispatch-1", call.DispatchID)
	}
	select {
	case result := <-terminated:
		t.Fatalf("Terminate() returned before canceled callback joined: %#v", result)
	default:
	}

	close(execution.complete)
	result := <-terminated
	if err := <-terminateErr; err != nil || result.Outcome != workersessions.ControlOutcomeNoop || result.Session.State != workersessions.StateCanceled {
		t.Fatalf("Terminate() = %#v, %v, want joined canceled NOOP", result, err)
	}
	final := <-started
	if err := <-startErr; err != nil {
		t.Fatalf("Start() error = %v, want nil", err)
	}
	if final.Session.State != workersessions.StateCanceled || final.Dispatch.TerminalOutcome != workers.WorkstationDispatchTerminalOutcomeCanceled || !errors.Is(final.DispatchErr, workers.ErrWorkstationDispatchCanceled) {
		t.Fatalf("Start() after held cancellation = %#v, want established canceled result", final)
	}
}
