package service_test

import (
	"context"
	"sync"

	"github.com/portpowered/infinite-you/pkg/services/work"
	workersessions "github.com/portpowered/infinite-you/pkg/services/worker_sessions"
	"github.com/portpowered/infinite-you/pkg/services/workers"
)

// fakeExecution is a controlled test double for
// workers.WorkstationExecutionService. dispatch is called for every
// DispatchWorkstation invocation; a nil dispatch reports an unconfigured
// double as an error rather than silently succeeding.
type fakeExecution struct {
	dispatch func(context.Context, workers.WorkstationDispatchRequest) (workers.WorkstationDispatchResult, error)

	mu    sync.Mutex
	calls []workers.WorkstationDispatchRequest
}

var _ workers.WorkstationExecutionService = (*fakeExecution)(nil)

func (f *fakeExecution) StartWorkstationPool(
	context.Context,
	workers.WorkstationPoolStartRequest,
) (workers.WorkstationPoolStartResult, error) {
	return workers.WorkstationPoolStartResult{}, nil
}

func (f *fakeExecution) StopWorkstationPool(context.Context) (workers.WorkstationPoolStopResult, error) {
	return workers.WorkstationPoolStopResult{}, nil
}

func (f *fakeExecution) DispatchWorkstation(
	ctx context.Context,
	req workers.WorkstationDispatchRequest,
) (workers.WorkstationDispatchResult, error) {
	f.mu.Lock()
	f.calls = append(f.calls, req)
	dispatch := f.dispatch
	f.mu.Unlock()
	if dispatch == nil {
		return workers.WorkstationDispatchResult{}, workers.ErrWorkstationPoolUnavailable
	}
	return dispatch(ctx, req)
}

func (f *fakeExecution) CancelWorkstationDispatch(
	context.Context,
	workers.WorkstationDispatchCancelRequest,
) (workers.WorkstationDispatchCancelResult, error) {
	return workers.WorkstationDispatchCancelResult{}, nil
}

func (f *fakeExecution) callCount() int {
	f.mu.Lock()
	defer f.mu.Unlock()
	return len(f.calls)
}

func (f *fakeExecution) requests() []workers.WorkstationDispatchRequest {
	f.mu.Lock()
	defer f.mu.Unlock()
	return append([]workers.WorkstationDispatchRequest(nil), f.calls...)
}

// validStartRequest returns a minimally well-formed StartRequest for id,
// naming attemptID as its resolved attempt (dispatch) identity.
func validStartRequest(id, attemptID string) workersessions.StartRequest {
	return workersessions.StartRequest{
		ID: id,
		Execution: workers.WorkstationDispatchRequest{
			WorkstationName: "review",
			Execution: workers.WorkstationExecutionRequest{
				Dispatch: work.WorkDispatch{
					DispatchID:      attemptID,
					WorkstationName: "review",
				},
			},
		},
	}
}

// succeedingExecution returns a fakeExecution whose DispatchWorkstation
// reports an ordinary successful WorkResult.
func succeedingExecution() *fakeExecution {
	return &fakeExecution{
		dispatch: func(_ context.Context, req workers.WorkstationDispatchRequest) (workers.WorkstationDispatchResult, error) {
			return workers.WorkstationDispatchResult{
				DispatchID:      req.Execution.Dispatch.DispatchID,
				WorkstationName: req.WorkstationName,
				TerminalOutcome: workers.WorkstationDispatchTerminalOutcomeCompleted,
				Result: workers.WorkResult{
					DispatchID: req.Execution.Dispatch.DispatchID,
					Outcome:    workers.OutcomeAccepted,
				},
			}, nil
		},
	}
}
