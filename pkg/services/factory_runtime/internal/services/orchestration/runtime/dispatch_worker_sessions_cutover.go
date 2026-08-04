package runtime

import (
	"context"

	"github.com/portpowered/infinite-you/pkg/services/recordings"
	workersessions "github.com/portpowered/infinite-you/pkg/services/worker_sessions"
	"github.com/portpowered/infinite-you/pkg/services/workers"
	workerexecution "github.com/portpowered/infinite-you/pkg/services/workers"
)

// startThroughWorkerSessions is the W4 Runtime dispatch cutover seam. For
// every resolved dispatch it: boots the underlying Workers pool if needed,
// binds the stable dispatch ID to one Worker Session identity and commits
// that association to canonical Factory Events before any Worker Sessions
// call, then hands the resolved request to worker_sessions.Service.Start
// (which drives the existing Workers workstation-pool boundary underneath).
// The Worker Sessions terminal outcome is translated back into the exact
// workers.WorkstationDispatchResult shape the pre-cutover accept callback
// expects, so existing Work materialization and Factory result behavior is
// unchanged.
func startThroughWorkerSessions(
	ctx context.Context,
	cfg *runtimeConfig,
	eventHistory recordings.RuntimeLedger,
	workersBoundary workers.WorkstationPoolBoundary,
	request workers.WorkstationDispatchRequest,
	accept workers.WorkstationDispatchAcceptFunc,
) error {
	if err := workersBoundary.Start(ctx); err != nil {
		return err
	}
	dispatchID := request.Execution.Dispatch.DispatchID
	sessionID := dispatchID
	eventHistory.RecordDispatchWorkerSessionAssociation(
		request.Execution.Dispatch.Execution.DispatchCreatedTick,
		dispatchID,
		sessionID,
		cfg.clock.Now(),
	)
	execute := func() {
		startResult, startErr := cfg.workerSessions.Start(
			context.WithoutCancel(ctx),
			workersessions.StartRequest{ID: sessionID, Execution: request},
		)
		result, dispatchErr := workerSessionDispatchOutcome(request, startResult, startErr)
		accept(context.Background(), request, result, dispatchErr)
	}
	async := !cfg.inlineDispatch && cfg.completionDeliveryPlanner == nil
	if async {
		go execute()
		return nil
	}
	execute()
	return nil
}

// workerSessionDispatchOutcome translates one Worker Sessions Start result
// into the exact workers.WorkstationDispatchResult/error shape the Runtime
// accept callback expects. When Start handed the attempt off to Workers, the
// raw StartResult.Dispatch/DispatchErr already carry that exact shape and are
// returned unchanged. When Start rejected the request before any Workers
// call (invalid request, conflicting start, or a before-handoff Events
// publication failure), a synthesized FAILED result is returned instead of
// fabricating a Workers payload that never existed.
func workerSessionDispatchOutcome(
	request workers.WorkstationDispatchRequest,
	startResult workersessions.StartResult,
	startErr error,
) (workers.WorkstationDispatchResult, error) {
	dispatchID := request.Execution.Dispatch.DispatchID
	transitionID := request.Execution.Dispatch.TransitionID
	if startErr != nil {
		return workers.WorkstationDispatchResult{
			DispatchID:      dispatchID,
			WorkstationName: request.WorkstationName,
			TerminalOutcome: workers.WorkstationDispatchTerminalOutcomeFailed,
			Result: workerexecution.WorkResult{
				DispatchID:   dispatchID,
				TransitionID: transitionID,
				Outcome:      workerexecution.OutcomeFailed,
				Error:        startErr.Error(),
			},
		}, startErr
	}
	if handedOffToWorkers(startResult) {
		return startResult.Dispatch, startResult.DispatchErr
	}
	errText := "worker session start failed before Workers handoff"
	if startResult.Session.Result != nil && startResult.Session.Result.Cause != nil {
		errText = string(startResult.Session.Result.Cause.Kind)
	}
	return workers.WorkstationDispatchResult{
		DispatchID:      dispatchID,
		WorkstationName: request.WorkstationName,
		TerminalOutcome: workers.WorkstationDispatchTerminalOutcomeFailed,
		Result: workerexecution.WorkResult{
			DispatchID:   dispatchID,
			TransitionID: transitionID,
			Outcome:      workerexecution.OutcomeFailed,
			Error:        errText,
		},
	}, nil
}

// handedOffToWorkers reports whether Start actually reached the Workers
// DispatchWorkstation call. The only FAILED terminal Start commits without a
// Workers handoff is FailureCauseEventPublicationFailure; every other
// terminal cause (start failure, adapter failure, executor panic, or Workers
// execution failure) is only produced from within the handoff itself.
func handedOffToWorkers(startResult workersessions.StartResult) bool {
	result := startResult.Session.Result
	if result == nil || result.Cause == nil {
		return true
	}
	return result.Cause.Kind != workersessions.FailureCauseEventPublicationFailure
}
