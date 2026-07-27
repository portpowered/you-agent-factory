package service

import (
	"context"
	"errors"
	"fmt"

	automations "github.com/portpowered/infinite-you/pkg/services/automations"
)

func pendingObservation(identity automations.SourceIdentity) automations.SourceObservation {
	return automations.SourceObservation{
		Identity:   identity,
		InstanceID: stableInstanceID(identity.AutomationID, identity.SourceID),
		State:      automations.ObservedLifecyclePending,
	}
}

func terminalConvergence(
	state automations.ObservedLifecycleState,
) automations.ConvergenceStatus {
	switch state {
	case automations.ObservedLifecycleFailed:
		return automations.ConvergenceStatusFailed
	case automations.ObservedLifecycleCancelled:
		return automations.ConvergenceStatusCancelled
	default:
		return ""
	}
}

func terminalLifecycleOutcome(
	desired automations.DesiredLifecycleState,
	observation automations.SourceObservation,
	idempotent bool,
) automations.LifecycleOutcome {
	return lifecycleOutcome(
		desired, observation, terminalConvergence(observation.State), idempotent,
	)
}

func cancelledLifecycleOutcome(
	desired automations.DesiredLifecycleState,
	observation automations.SourceObservation,
	idempotent bool,
) automations.LifecycleOutcome {
	observation.State = automations.ObservedLifecycleCancelled
	return lifecycleOutcome(
		desired, observation, automations.ConvergenceStatusCancelled, idempotent,
	)
}

func recordEffectFailure(
	op string,
	desired automations.DesiredLifecycleState,
	record *sourceRecord,
	err error,
) (automations.LifecycleOutcome, error) {
	if isCancellation(err) {
		return cancelledLifecycleOutcome(desired, record.observation, false),
			cancelledOperationError(op, err)
	}
	record.desired = desired
	record.observation.State = automations.ObservedLifecycleFailed
	record.terminalErr = failedOperationError(op, err)
	return terminalLifecycleOutcome(desired, record.observation, false), record.terminalErr
}

func isCancellation(err error) bool {
	return errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded)
}

func cancelledOperationError(op string, err error) *automations.Error {
	return &automations.Error{Op: op, Code: automations.ErrorCodeCancelled, Err: err}
}

func failedOperationError(op string, err error) *automations.Error {
	return &automations.Error{
		Op:   op,
		Code: automations.ErrorCodeFailed,
		Err:  fmt.Errorf("%w: %w", automations.ErrSupervisionFailed, err),
	}
}

func observationTerminalError(
	op string,
	state automations.ObservedLifecycleState,
) error {
	switch state {
	case automations.ObservedLifecycleCancelled:
		return cancelledOperationError(op, context.Canceled)
	case automations.ObservedLifecycleFailed:
		return failedOperationError(op, errors.New("supervision observed failed state"))
	default:
		return nil
	}
}

func lifecycleOutcome(
	desired automations.DesiredLifecycleState,
	observation automations.SourceObservation,
	convergence automations.ConvergenceStatus,
	idempotent bool,
) automations.LifecycleOutcome {
	return automations.LifecycleOutcome{
		Desired:     desired,
		Observation: observation,
		Convergence: convergence,
		Idempotent:  idempotent,
	}
}

func invalidOperationError(op, reason string) *automations.Error {
	return operationError(
		op, automations.ErrorCodeInvalid, automations.ErrInvalidRequest, reason,
	)
}

func unavailableEffectsError(op string) *automations.Error {
	return operationError(
		op, automations.ErrorCodeNotReady, automations.ErrNotReady,
		"source supervision effects are not configured",
	)
}

func operationError(
	op string,
	code automations.ErrorCode,
	sentinel error,
	reason string,
) *automations.Error {
	return &automations.Error{
		Op:   op,
		Code: code,
		Err:  fmt.Errorf("%w: %s", sentinel, reason),
	}
}
