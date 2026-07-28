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
	checkpointrecovery "github.com/portpowered/infinite-you/pkg/services/factory_runtime/internal/services/checkpoint_recovery"
	dispatchplanning "github.com/portpowered/infinite-you/pkg/services/factory_runtime/internal/services/dispatch_planning"
	"github.com/portpowered/infinite-you/pkg/services/work"
)

func (f *factoryImpl) ControlPause(ctx context.Context, _ factory.PauseRequest) (factory.PauseResult, error) {
	outcome, _, err := f.applyPauseControl()
	if err != nil {
		return factory.PauseResult{}, err
	}
	if f.dispatchPlan != nil {
		if err := f.dispatchPlan.Pause(ctx); err != nil {
			return factory.PauseResult{}, err
		}
	}
	return factory.PauseResult{Outcome: outcome}, nil
}

func (f *factoryImpl) ControlResume(ctx context.Context, _ factory.ResumeRequest) (factory.ResumeResult, error) {
	outcome, _, err := f.applyResumeControl()
	if err != nil {
		return factory.ResumeResult{}, err
	}
	if f.dispatchPlan != nil {
		if err := f.dispatchPlan.Resume(ctx); err != nil {
			return factory.ResumeResult{}, err
		}
	}
	return factory.ResumeResult{Outcome: outcome}, nil
}

func (f *factoryImpl) ControlTerminate(ctx context.Context, req factory.TerminateRequest) (factory.TerminateResult, error) {
	f.mu.Lock()
	state := f.state
	switch state {
	case interfaces.FactoryStateCompleted, interfaces.FactoryStateFailed:
		f.mu.Unlock()
		return factory.TerminateResult{}, factory.ErrAlreadyStopped
	case interfaces.FactoryStateRunning, interfaces.FactoryStatePaused, interfaces.FactoryStateIdle:
		f.state = interfaces.FactoryStateCompleted
		cancel := f.runCancel
		f.mu.Unlock()
		f.recordStateChange(state, interfaces.FactoryStateCompleted, req.Reason)
		if cancel != nil {
			cancel()
		}
		stopErr := f.stopDispatchRuntime(ctx, dispatchplanning.RuntimeStopReasonTerminated)
		if cancel == nil {
			f.completeOnce.Do(func() { close(f.completeCh) })
		}
		return factory.TerminateResult{Outcome: factory.ControlOutcomeAccepted}, stopErr
	default:
		f.mu.Unlock()
		return factory.TerminateResult{}, factory.ErrNotRunning
	}
}

func (f *factoryImpl) stopDispatchRuntime(
	ctx context.Context,
	reason dispatchplanning.RuntimeStopReason,
) error {
	if ctx == nil {
		ctx = context.Background()
	}
	stopCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), 5*time.Second)
	defer cancel()
	var stopErr error
	if f.dispatchPlan != nil {
		stopErr = f.dispatchPlan.Stop(stopCtx, reason)
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
	if f.dispatchPlan == nil {
		return factory.PlanDispatchResult{}, factory.ErrCapabilityUnavailable
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
	if f.dispatchPlan == nil {
		return factory.AcceptDispatchResultResult{}, factory.ErrCapabilityUnavailable
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
	if f.dispatchFlow == nil {
		return factory.AcceptDispatchResultResult{}, factory.ErrCapabilityUnavailable
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

func (f *factoryImpl) CaptureCheckpoint(
	_ context.Context,
	req factory.CaptureCheckpointRequest,
) (factory.CaptureCheckpointResult, error) {
	checkpointID := strings.TrimSpace(req.CheckpointID)
	if checkpointID == "" {
		return factory.CaptureCheckpointResult{}, factory.ErrCheckpointNotFound
	}
	f.mu.RLock()
	state := f.state
	recovery := f.checkpointRecovery
	f.mu.RUnlock()
	if err := checkpointCaptureLifecycleError(state); err != nil {
		return factory.CaptureCheckpointResult{}, err
	}
	if recovery == nil {
		return factory.CaptureCheckpointResult{}, factory.ErrCapabilityUnavailable
	}
	payload, err := checkpointrecovery.EncodeRuntimeOpaquePayload(checkpointrecovery.ExecutionCaptureFacts{
		FactoryState: string(state),
	})
	if err != nil {
		return factory.CaptureCheckpointResult{}, err
	}
	captured, err := recovery.Capture(checkpointrecovery.CaptureRequest{
		CheckpointID: checkpointID,
		Payload:      payload,
	})
	if err != nil {
		return factory.CaptureCheckpointResult{}, err
	}
	return factory.CaptureCheckpointResult{
		Outcome:    factory.CheckpointOutcomeCaptured,
		Checkpoint: checkpointrecovery.RootCheckpointFromEnvelope(captured.Envelope),
	}, nil
}

func checkpointCaptureLifecycleError(state interfaces.FactoryState) error {
	switch state {
	case interfaces.FactoryStateRunning, interfaces.FactoryStatePaused, interfaces.FactoryStateIdle:
		return nil
	case interfaces.FactoryStateCompleted, interfaces.FactoryStateFailed:
		return factory.ErrNotRunning
	default:
		return factory.ErrNotRunning
	}
}

func (f *factoryImpl) LoadCheckpoint(_ context.Context, req factory.LoadCheckpointRequest) (factory.LoadCheckpointResult, error) {
	return factory.LoadCheckpointResult{}, f.unavailableRootCapability(
		req.CheckpointID == "",
		factory.ErrCheckpointNotFound,
		true,
	)
}

func (f *factoryImpl) RestoreCheckpoint(
	_ context.Context,
	req factory.RestoreCheckpointRequest,
) (factory.RestoreCheckpointResult, error) {
	var validationErr error
	switch {
	case req.Checkpoint.CheckpointID == "":
		validationErr = factory.ErrCheckpointNotFound
	case req.Checkpoint.SchemaVersion <= 0 || len(req.Checkpoint.Payload) == 0:
		validationErr = factory.ErrCorruptCheckpoint
	case req.Checkpoint.SchemaVersion != 1:
		validationErr = factory.ErrIncompatibleCheckpoint
	}
	return factory.RestoreCheckpointResult{}, f.unavailableRootCapability(validationErr != nil, validationErr, false)
}

func (f *factoryImpl) unavailableRootCapability(invalid bool, invalidErr error, allowStopped bool) error {
	f.mu.RLock()
	state := f.state
	f.mu.RUnlock()
	switch state {
	case interfaces.FactoryStateRunning, interfaces.FactoryStatePaused, interfaces.FactoryStateIdle:
	case interfaces.FactoryStateCompleted, interfaces.FactoryStateFailed:
		if !allowStopped {
			return factory.ErrNotRunning
		}
	default:
		return factory.ErrNotRunning
	}
	if invalid {
		return invalidErr
	}
	return factory.ErrCapabilityUnavailable
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
