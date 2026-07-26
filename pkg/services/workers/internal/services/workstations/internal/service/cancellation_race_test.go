package service

import (
	"context"
	"errors"
	"fmt"
	"testing"

	"github.com/portpowered/infinite-you/pkg/services/workers"
	workstations "github.com/portpowered/infinite-you/pkg/services/workers/internal/services/workstations"
)

func TestPoolCancellationAndCompletionRaceCommitsOneTerminalOutcome(t *testing.T) {
	t.Parallel()

	const schedules = 50
	for index := range schedules {
		dispatchID := fmt.Sprintf("dispatch-race-%d", index)
		executor := newControlledExecutor(dispatchID)
		pool := New()
		startPool(t, pool, workstations.Route{
			WorkstationName: "review",
			Executor:        executor,
			Capacity:        1,
			QueueCapacity:   1,
		})
		completed := dispatchResultAsync(
			pool,
			context.Background(),
			dispatchID,
			"review",
		)
		assertStarted(t, executor, dispatchID)

		start := make(chan struct{})
		cancelled := make(chan struct {
			result workers.WorkstationDispatchCancelResult
			err    error
		}, 1)
		go func() {
			<-start
			result, err := pool.Cancel(
				context.Background(),
				workers.WorkstationDispatchCancelRequest{DispatchID: dispatchID},
			)
			cancelled <- struct {
				result workers.WorkstationDispatchCancelResult
				err    error
			}{result: result, err: err}
		}()
		go func() {
			<-start
			executor.release(dispatchID)
		}()
		close(start)

		dispatch := <-completed
		cancellation := <-cancelled
		switch cancellation.result.Outcome {
		case workers.WorkstationDispatchCancelOutcomeCanceled:
			if cancellation.err != nil {
				t.Fatalf("winning Cancel() error = %v", cancellation.err)
			}
			assertCanceledDispatch(t, dispatch)
		case workers.WorkstationDispatchCancelOutcomeAlreadyTerminal:
			if !errors.Is(cancellation.err, workers.ErrWorkstationDispatchAlreadyTerminal) {
				t.Fatalf("late Cancel() error = %v", cancellation.err)
			}
			if dispatch.err != nil ||
				dispatch.result.TerminalOutcome != workers.WorkstationDispatchTerminalOutcomeCompleted {
				t.Fatalf("winning completion = %#v, %v", dispatch.result, dispatch.err)
			}
		default:
			t.Fatalf("Cancel() outcome = %q", cancellation.result.Outcome)
		}
	}
}

type dispatchCompletion struct {
	result workers.WorkstationDispatchResult
	err    error
}

func dispatchResultAsync(
	pool *Pool,
	ctx context.Context,
	dispatchID string,
	route string,
) <-chan dispatchCompletion {
	completed := make(chan dispatchCompletion, 1)
	go func() {
		result, err := pool.Dispatch(
			ctx,
			dispatchRequest(dispatchID, "transition-"+dispatchID, route),
		)
		completed <- dispatchCompletion{result: result, err: err}
	}()
	return completed
}

func assertCanceledDispatch(t *testing.T, completed dispatchCompletion) {
	t.Helper()
	if !errors.Is(completed.err, workers.ErrWorkstationDispatchCanceled) ||
		!errors.Is(completed.err, context.Canceled) {
		t.Fatalf("cancelled Dispatch() error = %v", completed.err)
	}
	if completed.result.TerminalOutcome != workers.WorkstationDispatchTerminalOutcomeCanceled ||
		completed.result.Result.Outcome != workers.OutcomeFailed ||
		completed.result.Result.Error != workers.ErrWorkstationDispatchCanceled.Error() {
		t.Fatalf("cancelled Dispatch() result = %#v", completed.result)
	}
}
