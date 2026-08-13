package runtime

import (
	"context"
	"errors"
	"sync"
	"testing"

	"github.com/portpowered/infinite-you/pkg/services/workers"
)

type attemptExecuteFunc func(context.Context, workers.ExecuteRequest) (workers.ExecuteResult, error)

func (fn attemptExecuteFunc) Execute(ctx context.Context, request workers.ExecuteRequest) (workers.ExecuteResult, error) {
	return fn(ctx, request)
}

func TestAttemptLifecycleBoundsCapacityAndCleansActiveState(t *testing.T) {
	started := make(chan struct{})
	release := make(chan struct{})
	results := make(chan workers.ExecuteResult, 1)
	service := attemptExecuteFunc(func(_ context.Context, request workers.ExecuteRequest) (workers.ExecuteResult, error) {
		started <- struct{}{}
		<-release
		return workers.ExecuteResult{Correlation: request.Correlation, Outcome: workers.ExecutionOutcomeAccepted}, nil
	})
	ids := 0
	lifecycle := newAttemptLifecycle(service, func() string {
		ids++
		return "attempt-" + string(rune('0'+ids))
	}, 1)
	terminal := func(_ context.Context, request workers.ExecuteRequest, result workers.ExecuteResult, err error) {
		if err != nil {
			t.Errorf("terminal error = %v", err)
		}
		if request.Correlation.DispatchID != result.Correlation.DispatchID {
			t.Errorf("terminal correlation = %#v, result = %#v", request.Correlation, result.Correlation)
		}
		results <- result
	}

	if err := lifecycle.start(context.Background(), attemptTestRequest("dispatch-1", ""), true, terminal); err != nil {
		t.Fatalf("start(first) error = %v", err)
	}
	<-started
	if err := lifecycle.start(context.Background(), attemptTestRequest("dispatch-2", ""), true, terminal); !errors.Is(err, ErrAttemptCapacityExceeded) {
		t.Fatalf("start(second) error = %v, want capacity error", err)
	}
	if got := lifecycle.activeCount(); got != 1 {
		t.Fatalf("active count while first is blocked = %d, want 1", got)
	}
	close(release)
	result := <-results
	if result.Outcome != workers.ExecutionOutcomeAccepted {
		t.Fatalf("result outcome = %q", result.Outcome)
	}
	if got := lifecycle.activeCount(); got != 0 {
		t.Fatalf("active count after completion = %d, want 0", got)
	}
	if attemptID, ok := lifecycle.terminalAttemptID("dispatch-1"); !ok || attemptID != "attempt-1" {
		t.Fatalf("terminal attempt = %q, %v; want attempt-1, true", attemptID, ok)
	}
}

func TestAttemptLifecycleCancellationIsPerDispatch(t *testing.T) {
	started := make(chan string, 2)
	releaseSecond := make(chan struct{})
	results := make(chan workers.ExecuteResult, 2)
	service := attemptExecuteFunc(func(ctx context.Context, request workers.ExecuteRequest) (workers.ExecuteResult, error) {
		started <- request.Correlation.DispatchID
		if request.Correlation.DispatchID == "dispatch-1" {
			<-ctx.Done()
			return workers.ExecuteResult{}, ctx.Err()
		}
		<-releaseSecond
		return workers.ExecuteResult{Correlation: request.Correlation, Outcome: workers.ExecutionOutcomeAccepted}, nil
	})
	sequence := 0
	lifecycle := newAttemptLifecycle(service, func() string {
		sequence++
		return "attempt-" + string(rune('0'+sequence))
	}, 2)
	terminal := func(_ context.Context, _ workers.ExecuteRequest, result workers.ExecuteResult, err error) {
		if err != nil && !errors.Is(err, context.Canceled) {
			t.Errorf("terminal error = %v", err)
		}
		results <- result
	}
	if err := lifecycle.start(context.Background(), attemptTestRequest("dispatch-1", ""), true, terminal); err != nil {
		t.Fatalf("start(first) error = %v", err)
	}
	if err := lifecycle.start(context.Background(), attemptTestRequest("dispatch-2", ""), true, terminal); err != nil {
		t.Fatalf("start(second) error = %v", err)
	}
	startedDispatches := map[string]bool{<-started: true, <-started: true}
	if !startedDispatches["dispatch-1"] || !startedDispatches["dispatch-2"] {
		t.Fatalf("started dispatches = %#v", startedDispatches)
	}
	if outcome, err := lifecycle.cancel(context.Background(), "dispatch-1"); err != nil || outcome != workers.WorkstationDispatchCancelOutcomeCanceled {
		t.Fatalf("cancel(first) = %q, %v", outcome, err)
	}
	if outcome, err := lifecycle.cancel(context.Background(), "dispatch-1"); err != nil || outcome != workers.WorkstationDispatchCancelOutcomeAlreadyCanceled {
		t.Fatalf("repeat cancel(first) = %q, %v", outcome, err)
	}
	result := <-results
	if result.Outcome != workers.ExecutionOutcomeCanceled {
		t.Fatalf("canceled result outcome = %q", result.Outcome)
	}
	if got := lifecycle.activeCount(); got != 1 {
		t.Fatalf("active count after first cancellation = %d, want 1", got)
	}
	close(releaseSecond)
	result = <-results
	if result.Outcome != workers.ExecutionOutcomeAccepted {
		t.Fatalf("second result outcome = %q", result.Outcome)
	}
	if got := lifecycle.activeCount(); got != 0 {
		t.Fatalf("active count after second completion = %d, want 0", got)
	}
	if outcome, err := lifecycle.cancel(context.Background(), "dispatch-1"); err != nil || outcome != workers.WorkstationDispatchCancelOutcomeAlreadyTerminal {
		t.Fatalf("late cancel(first) = %q, %v", outcome, err)
	}
}

func TestAttemptLifecycleCancellationWinsACompletionRaceAfterCancel(t *testing.T) {
	started := make(chan struct{})
	release := make(chan struct{})
	results := make(chan workers.ExecuteResult, 1)
	service := attemptExecuteFunc(func(_ context.Context, request workers.ExecuteRequest) (workers.ExecuteResult, error) {
		close(started)
		<-release
		return workers.ExecuteResult{
			Correlation: request.Correlation,
			Outcome:     workers.ExecutionOutcomeAccepted,
		}, nil
	})
	lifecycle := newAttemptLifecycle(service, func() string { return "attempt-race" }, 1)
	if err := lifecycle.start(
		context.Background(), attemptTestRequest("dispatch-race", ""), true,
		func(_ context.Context, _ workers.ExecuteRequest, result workers.ExecuteResult, _ error) {
			results <- result
		},
	); err != nil {
		t.Fatalf("start() error = %v", err)
	}
	<-started
	if outcome, err := lifecycle.cancel(context.Background(), "dispatch-race"); err != nil || outcome != workers.WorkstationDispatchCancelOutcomeCanceled {
		t.Fatalf("cancel() = %q, %v", outcome, err)
	}
	close(release)
	if result := <-results; result.Outcome != workers.ExecutionOutcomeCanceled {
		t.Fatalf("racing completion outcome = %q, want CANCELED", result.Outcome)
	}
}

func TestAttemptLifecyclePanicAndDuplicateTerminalAreExactlyOnce(t *testing.T) {
	var mu sync.Mutex
	callbackCount := 0
	service := attemptExecuteFunc(func(context.Context, workers.ExecuteRequest) (workers.ExecuteResult, error) {
		panic("boom")
	})
	lifecycle := newAttemptLifecycle(service, func() string { return "attempt-1" }, 1)
	terminal := func(_ context.Context, _ workers.ExecuteRequest, result workers.ExecuteResult, _ error) {
		mu.Lock()
		defer mu.Unlock()
		callbackCount++
		if result.Outcome != workers.ExecutionOutcomeFailed {
			t.Errorf("panic result outcome = %q", result.Outcome)
		}
	}
	if err := lifecycle.start(context.Background(), attemptTestRequest("dispatch-1", ""), false, terminal); err != nil {
		t.Fatalf("start(panic) error = %v", err)
	}
	if err := lifecycle.start(context.Background(), attemptTestRequest("dispatch-1", "attempt-late"), false, terminal); err != nil {
		t.Fatalf("duplicate start error = %v", err)
	}
	mu.Lock()
	defer mu.Unlock()
	if callbackCount != 1 {
		t.Fatalf("terminal callback count = %d, want 1", callbackCount)
	}
	if got := lifecycle.activeCount(); got != 0 {
		t.Fatalf("active count after panic = %d, want 0", got)
	}
}

func attemptTestRequest(dispatchID, attemptID string) workers.ExecuteRequest {
	return workers.ExecuteRequest{
		Correlation: workers.ExecutionCorrelation{DispatchID: dispatchID, AttemptID: attemptID},
		Target:      workers.ExecutionTarget{RunnerID: workers.RunnerIDCodex},
	}
}
