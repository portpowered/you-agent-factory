package runtime

import (
	"context"
	"strings"

	factory "github.com/portpowered/infinite-you/pkg/services/factory_runtime"
	workersessions "github.com/portpowered/infinite-you/pkg/services/worker_sessions"
)

// workerSessionControlKey identifies the one immutable target set owned by a
// committed Factory turn control. ControlID is the upstream control-intent
// identity; a retry must return the original evidence, never reselect the
// current ledger or reach a replacement turn.
type workerSessionControlKey struct {
	turnID    string
	controlID string
	action    factory.WorkerSessionControlAction
}

// controlAssociatedWorkerSessions applies one committed Factory turn control
// to the captured turn's canonical Worker Session target set. It always
// attempts every target in the selector's stable order. Individual Worker
// Sessions failures remain per-child evidence instead of becoming a fail-fast
// Factory error, leaving Factory Runtime's existing dispatch-result authority
// intact.
func (f *factoryImpl) controlAssociatedWorkerSessions(
	ctx context.Context,
	turnID string,
	controlID string,
	action factory.WorkerSessionControlAction,
	lifecycleOutcome factory.ControlOutcome,
) factory.WorkerSessionControlResult {
	turnID = strings.TrimSpace(turnID)
	controlID = strings.TrimSpace(controlID)
	key := workerSessionControlKey{turnID: turnID, controlID: controlID, action: action}
	if controlID != "" {
		if prior, ok := f.workerSessionControlResults[key]; ok {
			return cloneWorkerSessionControlResult(prior)
		}
	}
	if lifecycleOutcome == factory.ControlOutcomeNoOp {
		if prior, ok := f.workerSessionControlResults[key]; ok {
			return cloneWorkerSessionControlResult(prior)
		}
		return newWorkerSessionControlNoOp(turnID, action)
	}

	captured := captureAssociatedWorkerSessionTargets(f.eventHistory, turnID)
	result := fanOutWorkerSessionControl(ctx, f.cfg.workerSessions, captured, action)
	f.workerSessionControlResults[key] = cloneWorkerSessionControlResult(result)
	f.logWorkerSessionControlFanout(result)
	return result
}

func newWorkerSessionControlNoOp(
	turnID string,
	action factory.WorkerSessionControlAction,
) factory.WorkerSessionControlResult {
	return factory.WorkerSessionControlResult{
		TurnID:   turnID,
		Action:   action,
		Outcome:  factory.WorkerSessionControlAggregateOutcomeNoOp,
		Children: []factory.WorkerSessionControlChildResult{},
	}
}

func fanOutWorkerSessionControl(
	ctx context.Context,
	service workersessions.Service,
	captured capturedWorkerSessionControlTargets,
	action factory.WorkerSessionControlAction,
) factory.WorkerSessionControlResult {
	result := newWorkerSessionControlNoOp(captured.turnID, action)
	targets := captured.workerSessionIDsSnapshot()
	if len(targets) == 0 {
		return result
	}

	// The parent control has already committed before fan-out. A client
	// disconnect must not abandon later siblings; Worker Sessions remains the
	// owner of any accepted child lifecycle and its own explicit effects.
	if ctx == nil {
		ctx = context.Background()
	}
	controlCtx := context.WithoutCancel(ctx)
	result.Children = make([]factory.WorkerSessionControlChildResult, 0, len(targets))
	for _, workerSessionID := range targets {
		child := factory.WorkerSessionControlChildResult{WorkerSessionID: workerSessionID}
		if service == nil {
			child.Outcome = factory.WorkerSessionControlChildOutcomeFailed
			result.Children = append(result.Children, child)
			continue
		}
		controlResult, err := callWorkerSessionControl(
			controlCtx, service, action, workersessions.ControlRequest{ID: workerSessionID},
		)
		child.DispatchID = controlResult.DispatchID
		child.Outcome = workerSessionControlChildOutcome(controlResult.Outcome, err)
		result.Children = append(result.Children, child)
	}
	result.Outcome = aggregateWorkerSessionControlOutcome(result.Children)
	return result
}

func callWorkerSessionControl(
	ctx context.Context,
	service workersessions.Service,
	action factory.WorkerSessionControlAction,
	req workersessions.ControlRequest,
) (workersessions.ControlResult, error) {
	switch action {
	case factory.WorkerSessionControlActionPause:
		return service.Pause(ctx, req)
	case factory.WorkerSessionControlActionResume:
		return service.Resume(ctx, req)
	case factory.WorkerSessionControlActionCancel:
		return service.Cancel(ctx, req)
	case factory.WorkerSessionControlActionTerminate:
		return service.Terminate(ctx, req)
	default:
		return workersessions.ControlResult{}, workersessions.ErrInvalidSessionID
	}
}

func workerSessionControlChildOutcome(
	outcome workersessions.ControlOutcome,
	err error,
) factory.WorkerSessionControlChildOutcome {
	if err != nil {
		return factory.WorkerSessionControlChildOutcomeFailed
	}
	switch outcome {
	case workersessions.ControlOutcomeApplied:
		return factory.WorkerSessionControlChildOutcomeApplied
	case workersessions.ControlOutcomeNoop:
		return factory.WorkerSessionControlChildOutcomeNoOp
	case workersessions.ControlOutcomeUnsupported:
		return factory.WorkerSessionControlChildOutcomeUnsupported
	default:
		return factory.WorkerSessionControlChildOutcomeFailed
	}
}

func aggregateWorkerSessionControlOutcome(
	children []factory.WorkerSessionControlChildResult,
) factory.WorkerSessionControlAggregateOutcome {
	if len(children) == 0 {
		return factory.WorkerSessionControlAggregateOutcomeNoOp
	}
	first := children[0].Outcome
	mixed := false
	for _, child := range children {
		if child.Outcome == factory.WorkerSessionControlChildOutcomeFailed {
			return factory.WorkerSessionControlAggregateOutcomeFailed
		}
		if child.Outcome != first {
			mixed = true
		}
	}
	if mixed {
		return factory.WorkerSessionControlAggregateOutcomePartial
	}
	switch first {
	case factory.WorkerSessionControlChildOutcomeApplied:
		return factory.WorkerSessionControlAggregateOutcomeApplied
	case factory.WorkerSessionControlChildOutcomeNoOp:
		return factory.WorkerSessionControlAggregateOutcomeNoOp
	case factory.WorkerSessionControlChildOutcomeUnsupported:
		return factory.WorkerSessionControlAggregateOutcomeUnsupported
	default:
		return factory.WorkerSessionControlAggregateOutcomeFailed
	}
}

func cloneWorkerSessionControlResult(
	value factory.WorkerSessionControlResult,
) factory.WorkerSessionControlResult {
	value.Children = append([]factory.WorkerSessionControlChildResult(nil), value.Children...)
	return value
}

func (f *factoryImpl) logWorkerSessionControlFanout(result factory.WorkerSessionControlResult) {
	if f == nil || f.logger == nil {
		return
	}
	f.logger.Info(
		"factory runtime worker session control fanout",
		"session_id", sessionIDFromFactoryConfig(f.cfg),
		"turn_id", result.TurnID,
		"action", string(result.Action),
		"outcome", string(result.Outcome),
		"target_count", len(result.Children),
	)
}
