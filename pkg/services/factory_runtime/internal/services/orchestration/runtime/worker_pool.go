package runtime

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	interfaces "github.com/portpowered/infinite-you/pkg/services/factory_definitions"
	factory "github.com/portpowered/infinite-you/pkg/services/factory_runtime"
	"github.com/portpowered/infinite-you/pkg/services/factory_runtime/internal/rootobservation"
	dispatchplanning "github.com/portpowered/infinite-you/pkg/services/factory_runtime/internal/services/dispatch_planning"
	"github.com/portpowered/infinite-you/pkg/services/work"
)

func (f *factoryImpl) ControlPause(ctx context.Context, req factory.PauseRequest) (factory.PauseResult, error) {
	f.workerSessionControlMu.Lock()
	defer f.workerSessionControlMu.Unlock()
	outcome, _, err := f.applyPauseControl()
	if err != nil {
		return factory.PauseResult{}, err
	}
	workerSessionControl := f.controlAssociatedWorkerSessions(
		ctx, req.TurnID, req.ControlID, factory.WorkerSessionControlActionPause, outcome,
	)
	if f.dispatchPlan != nil {
		if err := f.dispatchPlan.Pause(ctx); err != nil {
			return factory.PauseResult{Outcome: outcome, WorkerSessionControl: workerSessionControl}, err
		}
	}
	return factory.PauseResult{Outcome: outcome, WorkerSessionControl: workerSessionControl}, nil
}

func (f *factoryImpl) ControlResume(ctx context.Context, req factory.ResumeRequest) (factory.ResumeResult, error) {
	f.workerSessionControlMu.Lock()
	defer f.workerSessionControlMu.Unlock()
	outcome, _, err := f.applyResumeControl()
	if err != nil {
		return factory.ResumeResult{}, err
	}
	workerSessionControl := f.controlAssociatedWorkerSessions(
		ctx, req.TurnID, req.ControlID, factory.WorkerSessionControlActionResume, outcome,
	)
	if f.dispatchPlan != nil {
		if err := f.dispatchPlan.Resume(ctx); err != nil {
			return factory.ResumeResult{Outcome: outcome, WorkerSessionControl: workerSessionControl}, err
		}
	}
	return factory.ResumeResult{Outcome: outcome, WorkerSessionControl: workerSessionControl}, nil
}

func (f *factoryImpl) ControlTerminate(ctx context.Context, req factory.TerminateRequest) (factory.TerminateResult, error) {
	f.workerSessionControlMu.Lock()
	defer f.workerSessionControlMu.Unlock()
	action, err := terminateWorkerSessionControlAction(req.WorkerSessionAction)
	if err != nil {
		return factory.TerminateResult{}, err
	}
	f.mu.Lock()
	state := f.state
	switch state {
	case interfaces.FactoryStateCompleted, interfaces.FactoryStateFailed:
		f.mu.Unlock()
		if strings.TrimSpace(req.TurnID) != "" || strings.TrimSpace(req.ControlID) != "" {
			f.logRuntimeLifecycleControl(string(action), state, state, "NO_OP")
			return factory.TerminateResult{
				Outcome: factory.ControlOutcomeNoOp,
				WorkerSessionControl: f.controlAssociatedWorkerSessions(
					ctx, req.TurnID, req.ControlID, action, factory.ControlOutcomeNoOp,
				),
			}, nil
		}
		return factory.TerminateResult{}, factory.ErrAlreadyStopped
	case interfaces.FactoryStateRunning, interfaces.FactoryStatePaused, interfaces.FactoryStateIdle:
		f.state = interfaces.FactoryStateCompleted
		cancel := f.runCancel
		f.mu.Unlock()
		f.recordStateChange(state, interfaces.FactoryStateCompleted, req.Reason)
		// The control has committed before child coordination starts. Canceling
		// the engine prevents new dispatch planning while the shared control
		// lock keeps Run's pool shutdown behind the exact Worker Sessions
		// cancellations below.
		if cancel != nil {
			cancel()
		}
		workerSessionControl := f.controlAssociatedWorkerSessions(
			ctx, req.TurnID, req.ControlID, action, factory.ControlOutcomeAccepted,
		)
		stopErr := f.stopDispatchRuntimeLocked(ctx, dispatchplanning.RuntimeStopReasonTerminated)
		f.logRuntimeLifecycleControl(string(action), state, interfaces.FactoryStateCompleted, "ACCEPTED")
		if cancel == nil {
			f.completeOnce.Do(func() { close(f.completeCh) })
		}
		return factory.TerminateResult{
			Outcome:              factory.ControlOutcomeAccepted,
			WorkerSessionControl: workerSessionControl,
		}, stopErr
	default:
		f.mu.Unlock()
		return factory.TerminateResult{}, factory.ErrNotRunning
	}
}

func terminateWorkerSessionControlAction(action factory.WorkerSessionControlAction) (factory.WorkerSessionControlAction, error) {
	switch action {
	case "", factory.WorkerSessionControlActionTerminate:
		return factory.WorkerSessionControlActionTerminate, nil
	case factory.WorkerSessionControlActionCancel:
		return factory.WorkerSessionControlActionCancel, nil
	default:
		return "", fmt.Errorf("%w: stop Worker Session action %q", factory.ErrInvalidLifecycleTransition, action)
	}
}

func (f *factoryImpl) stopDispatchRuntime(
	ctx context.Context,
	reason dispatchplanning.RuntimeStopReason,
) error {
	f.workerSessionControlMu.Lock()
	defer f.workerSessionControlMu.Unlock()
	return f.stopDispatchRuntimeLocked(ctx, reason)
}

func (f *factoryImpl) stopDispatchRuntimeLocked(
	ctx context.Context,
	reason dispatchplanning.RuntimeStopReason,
) error {
	if ctx == nil {
		ctx = context.Background()
	}
	stopCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), 5*time.Second)
	defer cancel()
	var stopErr error
	// Worker Sessions closes asynchronous admission and joins its server-owned
	// supervisors before Workers closes the dispatch pool. This ordering keeps
	// a shutdown race from admitting new work after the pool has stopped and
	// lets terminal callbacks publish through the still-open Events boundary.
	if f.cfg != nil && f.cfg.workerSessions != nil {
		if lifecycle, ok := f.cfg.workerSessions.(interface{ Stop(context.Context) error }); ok {
			stopErr = lifecycle.Stop(stopCtx)
		}
	}
	if f.dispatchPlan != nil {
		stopErr = errors.Join(stopErr, f.dispatchPlan.Stop(stopCtx, reason))
	}
	if f.workers != nil {
		stopErr = errors.Join(stopErr, f.workers.Stop(stopCtx))
	}
	return stopErr
}

func (f *factoryImpl) ControlWaitToComplete(_ factory.WaitToCompleteRequest) factory.WaitToCompleteResult {
	return factory.WaitToCompleteResult{Done: f.WaitToComplete()}
}

func (f *factoryImpl) ControlMoveWork(ctx context.Context, req factory.MoveWorkRequest) (factory.MoveWorkResult, error) {
	result, err := f.MoveWork(ctx, req.WorkID, req.StateName, work.WorkStateChangeSource(req.Source), req.RequestID)
	if err != nil {
		if errors.Is(err, work.ErrMoveWorkRequestAlreadyApplied) {
			return factory.MoveWorkResult{}, factory.ErrMoveWorkRequestConflict
		}
		return factory.MoveWorkResult{}, err
	}
	return factory.MoveWorkResult{
		WorkID: result.WorkID, WorkTypeID: result.WorkTypeID,
		FromState: result.FromState, ToState: result.ToState,
	}, nil
}

func (f *factoryImpl) Observe(ctx context.Context, req factory.ObserveRequest) (factory.ObserveResult, error) {
	if !validObservationScope(req.Scope) {
		return factory.ObserveResult{}, factory.ErrInvalidObservationScope
	}
	f.mu.RLock()
	state := f.state
	f.mu.RUnlock()
	switch state {
	case interfaces.FactoryStateRunning, interfaces.FactoryStatePaused, interfaces.FactoryStateIdle,
		interfaces.FactoryStateCompleted, interfaces.FactoryStateFailed:
	default:
		return factory.ObserveResult{}, factory.ErrNotRunning
	}
	snapshot, err := f.GetEngineStateSnapshot(ctx)
	if err != nil {
		return factory.ObserveResult{}, err
	}
	return factory.ObserveResult{Observation: rootobservation.Project(snapshot, req.Scope)}, nil
}

func (f *factoryImpl) PlanDispatch(
	ctx context.Context,
	req factory.PlanDispatchRequest,
) (factory.PlanDispatchResult, error) {
	if err := f.requireActiveDispatchRuntime(); err != nil {
		return factory.PlanDispatchResult{}, err
	}
	if err := validateRootDispatchPlan(req); err != nil {
		return factory.PlanDispatchResult{}, err
	}
	dispatch := work.WorkDispatch{
		DispatchID:      req.DispatchID,
		TransitionID:    req.WorkstationName,
		WorkerType:      req.WorkerType,
		WorkstationName: req.WorkstationName,
		Execution: work.ExecutionMetadata{
			WorkIDs:   append([]string(nil), req.WorkIDs...),
			ReplayKey: req.ReplayKey,
		},
		InputTokens: make([]any, 0),
	}
	planned, err := f.dispatchPlan.Plan(ctx, dispatchplanning.PlanRequest{
		Decisions: []dispatchplanning.RunnableDecision{{
			CorrelationID: req.CorrelationID,
			Dispatch:      dispatch,
			Execution: dispatchplanning.ExecutionFacts{
				WorkerType:   req.WorkerType,
				InputPayload: make([]any, 0),
			},
		}},
	})
	if err != nil {
		return factory.PlanDispatchResult{}, mapDispatchPlanningError(err)
	}
	if len(planned.Actions) != 1 {
		return factory.PlanDispatchResult{}, fmt.Errorf(
			"%w: planner returned %d actions",
			factory.ErrInvalidDispatchResultBoundary,
			len(planned.Actions),
		)
	}
	published, err := f.dispatchPlan.Publish(ctx, planned.Actions[0])
	if err != nil {
		return factory.PlanDispatchResult{}, mapDispatchPlanningError(err)
	}
	return factory.PlanDispatchResult{
		Outcome:       factory.DispatchPlanOutcome(published.Outcome),
		DispatchID:    published.DispatchID,
		CorrelationID: published.CorrelationID,
	}, nil
}

func (f *factoryImpl) AcceptDispatchResult(
	ctx context.Context,
	req factory.AcceptDispatchResultRequest,
) (factory.AcceptDispatchResultResult, error) {
	if err := f.requireDispatchResultRuntime(); err != nil {
		return factory.AcceptDispatchResultResult{}, err
	}
	if req.CorrelationID == "" {
		return factory.AcceptDispatchResultResult{}, factory.ErrUnknownDispatchCorrelation
	}
	if req.DispatchID == "" || req.WorkID == "" {
		return factory.AcceptDispatchResultResult{}, factory.ErrInvalidDispatchResultBoundary
	}
	outcome, err := rootTerminalResultOutcome(req.ResultOutcome)
	if err != nil {
		return factory.AcceptDispatchResultResult{}, err
	}
	retired, err := f.dispatchFlow.acceptRootResult(ctx, req, outcome)
	if err != nil {
		return factory.AcceptDispatchResultResult{}, mapDispatchPlanningError(err)
	}
	return factory.AcceptDispatchResultResult{
		Outcome:       factory.DispatchPlanOutcome(retired.Outcome),
		DispatchID:    retired.DispatchID,
		CorrelationID: retired.CorrelationID,
	}, nil
}

func (f *factoryImpl) requireDispatchResultRuntime() error {
	f.mu.RLock()
	state := f.state
	f.mu.RUnlock()
	switch state {
	case interfaces.FactoryStateRunning, interfaces.FactoryStatePaused, interfaces.FactoryStateIdle,
		interfaces.FactoryStateCompleted, interfaces.FactoryStateFailed:
		return nil
	default:
		return factory.ErrNotRunning
	}
}

func (f *factoryImpl) requireActiveDispatchRuntime() error {
	f.mu.RLock()
	state := f.state
	f.mu.RUnlock()
	switch state {
	case interfaces.FactoryStateRunning, interfaces.FactoryStatePaused, interfaces.FactoryStateIdle:
		return nil
	default:
		return factory.ErrNotRunning
	}
}

func validateRootDispatchPlan(req factory.PlanDispatchRequest) error {
	if req.DispatchID == "" || req.CorrelationID == "" || req.WorkstationName == "" ||
		req.WorkerType == "" || req.ReplayKey == "" || len(req.WorkIDs) == 0 {
		return factory.ErrInvalidDispatchResultBoundary
	}
	for _, workID := range req.WorkIDs {
		if workID == "" {
			return factory.ErrInvalidDispatchResultBoundary
		}
	}
	return nil
}

func rootTerminalResultOutcome(
	outcome factory.DispatchResultOutcome,
) (dispatchplanning.TerminalResultOutcome, error) {
	switch outcome {
	case factory.DispatchResultOutcomeSuccess:
		return dispatchplanning.TerminalResultOutcomeSuccess, nil
	case factory.DispatchResultOutcomeFailure:
		return dispatchplanning.TerminalResultOutcomeFailure, nil
	case factory.DispatchResultOutcomeCancelled:
		return dispatchplanning.TerminalResultOutcomeCancelled, nil
	default:
		return "", factory.ErrInvalidDispatchResultBoundary
	}
}

func mapDispatchPlanningError(err error) error {
	switch {
	case errors.Is(err, dispatchplanning.ErrDuplicateDispatchIntent):
		return fmt.Errorf("%w: %v", factory.ErrDuplicateDispatchIntent, err)
	case errors.Is(err, dispatchplanning.ErrUnknownDispatchCorrelation):
		return fmt.Errorf("%w: %v", factory.ErrUnknownDispatchCorrelation, err)
	case errors.Is(err, dispatchplanning.ErrInvalidDispatchResultBoundary),
		errors.Is(err, dispatchplanning.ErrInvalidRunnableDecision):
		return fmt.Errorf("%w: %v", factory.ErrInvalidDispatchResultBoundary, err)
	default:
		return err
	}
}

func validObservationScope(scope factory.ObservationScope) bool {
	switch scope {
	case "", factory.ObservationScopeFull, factory.ObservationScopeStatus, factory.ObservationScopeProgress,
		factory.ObservationScopeDispatches, factory.ObservationScopeResults, factory.ObservationScopeResources,
		factory.ObservationScopeHealth:
		return true
	default:
		return false
	}
}
