package service

import (
	"context"
	"sync"

	workersessions "github.com/portpowered/infinite-you/pkg/services/worker_sessions"
	"github.com/portpowered/infinite-you/pkg/services/workers"
)

// publishExecution hands one resolved request to the directly injected
// Workers execution service and returns at the exact admission barrier. The
// execution call itself remains detached from the caller context after it has
// reached Workers, while the registry receives the terminal result from the
// same goroutine that performed the call.
func (r *registry) publishExecution(
	ctx context.Context,
	sessionID string,
	request workers.WorkstationDispatchRequest,
	supervision *supervision,
) error {
	if ctx == nil {
		ctx = context.Background()
	}
	admitted := make(chan struct{})
	finished := make(chan struct{})
	dispatchDone := make(chan error, 1)
	var admittedOnce sync.Once
	execution := r.execution
	go func() {
		defer close(finished)
		result, dispatchErr := dispatchExecutionWithService(execution, ctx, request, func() {
			admittedOnce.Do(func() {
				r.acceptSupervision(sessionID, supervision)
				close(admitted)
			})
		})
		select {
		case <-admitted:
			r.completeSupervision(sessionID, supervision, result, dispatchErr)
		default:
		}
		dispatchDone <- dispatchErr
	}()
	select {
	case <-admitted:
	case <-finished:
		if dispatchErr := <-dispatchDone; dispatchErr != nil {
			select {
			case <-admitted:
			default:
				return dispatchErr
			}
		}
		select {
		case <-admitted:
		default:
			return workersessions.ErrStartAdmissionFailed
		}
	}
	return nil
}
func dispatchExecutionWithService(
	execution workers.WorkstationExecutionService,
	ctx context.Context,
	request workers.WorkstationDispatchRequest,
	admitted workers.WorkstationDispatchAdmissionFunc,
) (result workers.WorkstationDispatchResult, err error) {
	defer func() {
		if recovered := recover(); recovered != nil {
			panicErr := &workers.WorkerExecutorPanicError{Cause: recovered}
			result = workers.WorkstationDispatchResult{
				DispatchID:      request.Execution.Dispatch.DispatchID,
				WorkstationName: request.WorkstationName,
				TerminalOutcome: workers.WorkstationDispatchTerminalOutcomeFailed,
				Result: workers.WorkResult{
					DispatchID:   request.Execution.Dispatch.DispatchID,
					TransitionID: request.Execution.Dispatch.TransitionID,
					Outcome:      workers.OutcomeFailed,
					Error:        panicErr.Error(),
				},
			}
			err = panicErr
		}
	}()
	if execution == nil {
		return workers.WorkstationDispatchResult{
			DispatchID:      request.Execution.Dispatch.DispatchID,
			WorkstationName: request.WorkstationName,
			TerminalOutcome: workers.WorkstationDispatchTerminalOutcomeFailed,
			Result: workers.WorkResult{
				DispatchID:   request.Execution.Dispatch.DispatchID,
				TransitionID: request.Execution.Dispatch.TransitionID,
				Outcome:      workers.OutcomeFailed,
			},
		}, workers.ErrWorkstationPoolUnavailable
	}
	return execution.DispatchWorkstationWithAdmission(ctx, request, admitted)
}
