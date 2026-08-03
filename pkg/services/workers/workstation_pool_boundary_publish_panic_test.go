package workers

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

// poolBoundaryFakeService is a minimal WorkstationExecutionService that routes
// a dispatch straight to its bound executor, mirroring the pass-through error
// behavior of the production internal pool service closely enough to prove
// Publish propagates a recovered executor panic's typed error unchanged.
type poolBoundaryFakeService struct {
	mu     sync.Mutex
	routes map[string]WorkstationRequestExecutor
}

func (s *poolBoundaryFakeService) StartWorkstationPool(
	_ context.Context,
	request WorkstationPoolStartRequest,
) (WorkstationPoolStartResult, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.routes = make(map[string]WorkstationRequestExecutor, len(request.Bindings))
	for _, binding := range request.Bindings {
		s.routes[binding.RoleName] = binding.Executor
	}
	return WorkstationPoolStartResult{Outcome: WorkstationPoolLifecycleOutcomeStarted}, nil
}

func (*poolBoundaryFakeService) StopWorkstationPool(context.Context) (WorkstationPoolStopResult, error) {
	return WorkstationPoolStopResult{Outcome: WorkstationPoolLifecycleOutcomeStopped}, nil
}

func (s *poolBoundaryFakeService) DispatchWorkstation(
	ctx context.Context,
	request WorkstationDispatchRequest,
) (WorkstationDispatchResult, error) {
	s.mu.Lock()
	executor := s.routes[request.WorkstationName]
	s.mu.Unlock()
	result, err := executor.Execute(ctx, request.Execution)
	terminal := WorkstationDispatchTerminalOutcomeCompleted
	if err != nil || result.Outcome == OutcomeFailed {
		terminal = WorkstationDispatchTerminalOutcomeFailed
	}
	return WorkstationDispatchResult{
		DispatchID:      request.Execution.Dispatch.DispatchID,
		WorkstationName: request.WorkstationName,
		TerminalOutcome: terminal,
		Result:          result,
	}, err
}

func (*poolBoundaryFakeService) CancelWorkstationDispatch(
	_ context.Context,
	request WorkstationDispatchCancelRequest,
) (WorkstationDispatchCancelResult, error) {
	return WorkstationDispatchCancelResult{
		DispatchID: request.DispatchID,
		Outcome:    WorkstationDispatchCancelOutcomeCanceled,
	}, nil
}

type publishOutcome struct {
	result WorkstationDispatchResult
	err    error
}

func publishAndAwait(
	t *testing.T,
	boundary WorkstationPoolBoundary,
	request WorkstationDispatchRequest,
) publishOutcome {
	t.Helper()
	done := make(chan publishOutcome, 1)
	var callbacks int32
	err := boundary.Publish(context.Background(), request, func(
		_ context.Context,
		_ WorkstationDispatchRequest,
		result WorkstationDispatchResult,
		publishErr error,
	) {
		if atomic.AddInt32(&callbacks, 1) != 1 {
			t.Errorf("dispatch %s received more than one terminal callback", request.Execution.Dispatch.DispatchID)
		}
		done <- publishOutcome{result: result, err: publishErr}
	})
	if err != nil {
		t.Fatalf("Publish() err = %v, want nil", err)
	}
	select {
	case outcome := <-done:
		return outcome
	case <-time.After(5 * time.Second):
		t.Fatalf("dispatch %s: timed out waiting for terminal callback", request.Execution.Dispatch.DispatchID)
		return publishOutcome{}
	}
}

func assertPublishPanicFailure(
	t *testing.T,
	outcome publishOutcome,
	dispatchID, transitionID, cause string,
) {
	t.Helper()
	if outcome.err == nil {
		t.Fatalf("dispatch %s: publish err = nil, want non-nil typed panic error", dispatchID)
	}
	var panicErr *WorkerExecutorPanicError
	if !errors.As(outcome.err, &panicErr) || panicErr == nil {
		t.Fatalf("dispatch %s: errors.As(err, *WorkerExecutorPanicError) = false; err = %v", dispatchID, outcome.err)
	}
	if panicErr.Cause != any(cause) {
		t.Fatalf("dispatch %s: panicErr.Cause = %v, want %q", dispatchID, panicErr.Cause, cause)
	}
	wantText := fmt.Sprintf("executor panic: %s", cause)
	if outcome.result.Result.Error != wantText {
		t.Fatalf("dispatch %s: result.Error = %q, want %q", dispatchID, outcome.result.Result.Error, wantText)
	}
	if outcome.result.Result.Outcome != OutcomeFailed {
		t.Fatalf("dispatch %s: result.Outcome = %q, want %q", dispatchID, outcome.result.Result.Outcome, OutcomeFailed)
	}
	if outcome.result.TerminalOutcome != WorkstationDispatchTerminalOutcomeFailed {
		t.Fatalf(
			"dispatch %s: TerminalOutcome = %q, want %q",
			dispatchID, outcome.result.TerminalOutcome, WorkstationDispatchTerminalOutcomeFailed,
		)
	}
	if outcome.result.Result.DispatchID != dispatchID || outcome.result.Result.TransitionID != transitionID {
		t.Fatalf(
			"dispatch %s: result identity = (%q, %q), want (%q, %q)",
			dispatchID, outcome.result.Result.DispatchID, outcome.result.Result.TransitionID, dispatchID, transitionID,
		)
	}
}

func TestWorkstationPoolBoundaryPublishSynchronousPanicDeliversTypedFailure(t *testing.T) {
	service := &poolBoundaryFakeService{}
	boundary := NewWorkstationPoolBoundary(WorkstationPoolBoundaryConfig{
		Service:    service,
		Executors:  map[string]WorkerExecutor{"swe": poolBoundaryPanicExecutor{panicValue: "sync boom"}},
		RouteNames: []string{"swe"},
		Async:      false,
	})

	request := WorkstationDispatchRequest{
		WorkstationName: "swe",
		Execution:       poolBoundaryDispatchRequest("dispatch-sync-panic", "transition-sync-panic", "swe"),
	}
	outcome := publishAndAwait(t, boundary, request)
	assertPublishPanicFailure(t, outcome, "dispatch-sync-panic", "transition-sync-panic", "sync boom")

	// A later dispatch on the same boundary still executes after the
	// recovered panic; the panic must not have escaped its goroutine or
	// left the boundary unusable.
	nextRequest := WorkstationDispatchRequest{
		WorkstationName: "swe",
		Execution:       poolBoundaryDispatchRequest("dispatch-sync-after-panic", "transition-sync-panic", "swe"),
	}
	next := publishAndAwait(t, boundary, nextRequest)
	if next.err == nil {
		t.Fatalf("second dispatch err = nil, want typed panic error (same panicking executor)")
	}
	var panicErr *WorkerExecutorPanicError
	if !errors.As(next.err, &panicErr) {
		t.Fatalf("second dispatch errors.As(err, *WorkerExecutorPanicError) = false; err = %v", next.err)
	}
}

func TestWorkstationPoolBoundaryPublishAsynchronousPanicDeliversTypedFailure(t *testing.T) {
	service := &poolBoundaryFakeService{}
	boundary := NewWorkstationPoolBoundary(WorkstationPoolBoundaryConfig{
		Service:    service,
		Executors:  map[string]WorkerExecutor{"swe": poolBoundaryPanicExecutor{panicValue: "async boom"}},
		RouteNames: []string{"swe"},
		Async:      true,
	})

	request := WorkstationDispatchRequest{
		WorkstationName: "swe",
		Execution:       poolBoundaryDispatchRequest("dispatch-async-panic", "transition-async-panic", "swe"),
	}
	outcome := publishAndAwait(t, boundary, request)
	assertPublishPanicFailure(t, outcome, "dispatch-async-panic", "transition-async-panic", "async boom")
}

func TestWorkstationPoolBoundaryPublishAsynchronousPanicLaterDispatchStillExecutes(t *testing.T) {
	panicService := &poolBoundaryFakeService{}
	panicBoundary := NewWorkstationPoolBoundary(WorkstationPoolBoundaryConfig{
		Service:    panicService,
		Executors:  map[string]WorkerExecutor{"swe": poolBoundaryPanicExecutor{panicValue: "boom"}},
		RouteNames: []string{"swe"},
		Async:      true,
	})
	panicked := publishAndAwait(t, panicBoundary, WorkstationDispatchRequest{
		WorkstationName: "swe",
		Execution:       poolBoundaryDispatchRequest("dispatch-async-panic-first", "transition-1", "swe"),
	})
	assertPublishPanicFailure(t, panicked, "dispatch-async-panic-first", "transition-1", "boom")

	successService := &poolBoundaryFakeService{}
	successBoundary := NewWorkstationPoolBoundary(WorkstationPoolBoundaryConfig{
		Service:    successService,
		Executors:  map[string]WorkerExecutor{"swe": poolBoundaryErrorExecutorSuccess{result: WorkResult{Outcome: OutcomeAccepted}}},
		RouteNames: []string{"swe"},
		Async:      true,
	})
	succeeded := publishAndAwait(t, successBoundary, WorkstationDispatchRequest{
		WorkstationName: "swe",
		Execution:       poolBoundaryDispatchRequest("dispatch-async-after-panic", "transition-2", "swe"),
	})
	if succeeded.err != nil {
		t.Fatalf("later dispatch err = %v, want nil", succeeded.err)
	}
	if succeeded.result.Result.Outcome != OutcomeAccepted {
		t.Fatalf("later dispatch outcome = %q, want %q", succeeded.result.Result.Outcome, OutcomeAccepted)
	}
}

func TestWorkstationPoolBoundaryPublishAsynchronousPanicRepeatWithoutDuplicateAcceptance(t *testing.T) {
	const iterations = 25
	service := &poolBoundaryFakeService{}
	boundary := NewWorkstationPoolBoundary(WorkstationPoolBoundaryConfig{
		Service:    service,
		Executors:  map[string]WorkerExecutor{"swe": poolBoundaryPanicExecutor{panicValue: "repeat boom"}},
		RouteNames: []string{"swe"},
		Async:      true,
	})
	for i := range iterations {
		dispatchID := fmt.Sprintf("dispatch-async-repeat-%d", i)
		outcome := publishAndAwait(t, boundary, WorkstationDispatchRequest{
			WorkstationName: "swe",
			Execution:       poolBoundaryDispatchRequest(dispatchID, "transition-repeat", "swe"),
		})
		assertPublishPanicFailure(t, outcome, dispatchID, "transition-repeat", "repeat boom")
	}
}
