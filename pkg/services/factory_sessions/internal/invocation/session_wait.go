package invocation

import (
	"context"
	"errors"
	"time"

	interfaces "github.com/portpowered/infinite-you/pkg/services/factory_definitions"
	"github.com/portpowered/infinite-you/pkg/services/work"
)

const sessionInvocationPollInterval = 10 * time.Millisecond

// SessionInvocationObservation is one event-derived view of invocation state.
// Runtime adapters populate it without exposing engine or Petri-net types.
type SessionInvocationObservation struct {
	WorldState           interfaces.FactoryWorldState
	FactoryState         string
	ActiveWork           bool
	MissingPrimaryResult *work.PrimaryResultError
}

// SessionInvocationSpecialFailure describes a packaged-factory terminal
// classification without moving package-specific knowledge into the owner.
type SessionInvocationSpecialFailure struct {
	ErrorCode    string
	Message      string
	FailureClass string
}

// SessionInvocationSpecialCase supplies optional packaged-factory
// classification and telemetry at explicit points in the canonical wait loop.
type SessionInvocationSpecialCase interface {
	Active(*interfaces.FactoryConfig) bool
	TerminalFailure(interfaces.FactoryWorldState, string) *SessionInvocationSpecialFailure
}

func (o *SessionOwner) waitForResult(
	ctx context.Context,
	sessionID string,
	input SessionInvocationWaitInput,
) (FactoryInvocationResult, error) {
	waitCtx, cancel := invocationWaitContext(ctx, input.TimeoutMillis)
	defer cancel()

	packaged := o.specialCase != nil && o.specialCase.Active(input.FactoryConfig)
	loggedActive := false
	for {
		observation, err := o.observe(waitCtx, sessionID, input)
		if err != nil {
			return o.waitErrorResult(sessionID, input, err)
		}
		if packaged && observation.ActiveWork && !loggedActive {
			if telemetry, ok := o.telemetry.(SessionInvocationPackagedTelemetry); ok {
				telemetry.PackagedInvocationActive(sessionID, input)
			}
			loggedActive = true
		}
		if result, done, err := o.resolveObservation(ctx, sessionID, input, observation, packaged); done {
			return result, err
		}
		if err := o.waitNext(waitCtx); err != nil {
			return o.waitErrorResult(sessionID, input, err)
		}
	}
}

func (o *SessionOwner) resolveObservation(
	ctx context.Context,
	sessionID string,
	input SessionInvocationWaitInput,
	observation SessionInvocationObservation,
	packaged bool,
) (FactoryInvocationResult, bool, error) {
	selectionInput := work.PrimaryResultSelectionInput{
		RequestID: input.RequestID, InvocationReturn: input.InvocationReturn, WorldState: observation.WorldState,
	}
	selection, err := o.workService.ResolvePrimaryResult(ctx, selectionInput)
	if err == nil {
		if packaged {
			if telemetry, ok := o.telemetry.(SessionInvocationPackagedTelemetry); ok {
				telemetry.PackagedInvocationCompleted(sessionID, input, selection)
			}
		}
		return o.completedResult(sessionID, input, selection), true, nil
	}
	if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
		result, waitErr := o.waitErrorResult(sessionID, input, err)
		return result, true, waitErr
	}
	primaryErr, ok := err.(*work.PrimaryResultError)
	if !ok {
		return FactoryInvocationResult{}, true, err
	}
	if observation.MissingPrimaryResult != nil {
		return o.failedResult(sessionID, input, observation.MissingPrimaryResult), true, nil
	}
	if classified, ok := work.ClassifyInvocationControlState(sessionID, observation.FactoryState, selectionInput); ok {
		return o.failedResult(sessionID, input, classified), true, nil
	}
	if classified, ok := work.ClassifyMissingPrimaryResult(selectionInput); ok {
		return o.failedResult(sessionID, input, classified), true, nil
	}
	if packaged {
		worldState, _ := selectionInput.WorldState.(interfaces.FactoryWorldState)
		if result, ok := o.packagedTerminalFailureResult(sessionID, input, worldState); ok {
			return result, true, nil
		}
	}
	if classified, ok := work.ClassifyFailedInvocation(sessionID, selectionInput); ok {
		return o.failedResult(sessionID, input, classified), true, nil
	}
	if _, exists := observation.WorldState.WorkRequestsByID[input.RequestID]; !exists || observation.ActiveWork {
		return FactoryInvocationResult{}, false, nil
	}
	return o.resolveStoppedInvocation(sessionID, input, selectionInput, primaryErr, packaged), true, nil
}

func (o *SessionOwner) resolveStoppedInvocation(
	sessionID string,
	input SessionInvocationWaitInput,
	selectionInput work.PrimaryResultSelectionInput,
	primaryErr *work.PrimaryResultError,
	packaged bool,
) FactoryInvocationResult {
	if packaged {
		worldState, _ := selectionInput.WorldState.(interfaces.FactoryWorldState)
		if result, ok := o.packagedTerminalFailureResult(sessionID, input, worldState); ok {
			return result
		}
	}
	if classified, ok := work.ClassifyInvocationControlState(sessionID, "", selectionInput); ok {
		return o.failedResult(sessionID, input, classified)
	}
	if classified, ok := work.ClassifyFailedInvocation(sessionID, selectionInput); ok {
		return o.failedResult(sessionID, input, classified)
	}
	if classified, ok := work.ClassifyMissingPrimaryResult(selectionInput); ok {
		return o.failedResult(sessionID, input, classified)
	}
	return o.failedResult(sessionID, input, primaryErr)
}

func (o *SessionOwner) packagedTerminalFailureResult(
	sessionID string,
	input SessionInvocationWaitInput,
	worldState interfaces.FactoryWorldState,
) (FactoryInvocationResult, bool) {
	if o.specialCase == nil {
		return FactoryInvocationResult{}, false
	}
	failure := o.specialCase.TerminalFailure(worldState, input.RequestID)
	if failure == nil {
		return FactoryInvocationResult{}, false
	}
	result := FactoryInvocationResult{
		RequestID: input.RequestID, TraceID: input.TraceID,
		Status:    interfaces.InvocationTerminalStatusFailed,
		ErrorCode: failure.ErrorCode, Message: failure.Message,
	}
	// Packaged terminal failures retain the general failure metric, but
	// their diagnostic record is the packaged failure log emitted below.
	if o.telemetry != nil {
		o.telemetry.InvocationFailed(input.FactoryConfig, input.InputSource, result.ErrorCode)
	}
	if telemetry, ok := o.telemetry.(SessionInvocationPackagedTelemetry); ok {
		telemetry.PackagedInvocationFailed(sessionID, input, *failure)
	}
	return result, true
}

func (o *SessionOwner) completedResult(
	sessionID string,
	input SessionInvocationWaitInput,
	selection work.PrimaryResultSelection,
) FactoryInvocationResult {
	result := FactoryInvocationResult{
		RequestID: input.RequestID, TraceID: input.TraceID,
		Status: interfaces.InvocationTerminalStatusCompleted, PrimaryResult: selection.PrimaryResult,
	}
	if o.telemetry != nil {
		o.telemetry.InvocationCompleted(input.FactoryConfig, input.InputSource, selection.PrimaryResult)
		o.telemetry.LogInvocationCompleted(sessionID, input, selection)
	}
	return result
}

func (o *SessionOwner) failedResult(
	sessionID string,
	input SessionInvocationWaitInput,
	primaryErr *work.PrimaryResultError,
) FactoryInvocationResult {
	result := FactoryInvocationResult{
		RequestID: input.RequestID, TraceID: input.TraceID,
		Status:    interfaces.InvocationTerminalStatusFailed,
		ErrorCode: string(primaryErr.Code), Message: primaryErr.Message,
		SessionID: primaryErr.Context.SessionID, WorkID: primaryErr.Context.WorkID,
		WorkName: primaryErr.Context.WorkName, WorkState: primaryErr.Context.WorkState,
	}
	o.recordFailure(sessionID, input, result, failureClassForPrimaryResultError(primaryErr.Code))
	return result
}

func (o *SessionOwner) waitErrorResult(
	sessionID string,
	input SessionInvocationWaitInput,
	err error,
) (FactoryInvocationResult, error) {
	if !errors.Is(err, context.Canceled) && !errors.Is(err, context.DeadlineExceeded) {
		return FactoryInvocationResult{}, err
	}
	result := FactoryInvocationResult{RequestID: input.RequestID, TraceID: input.TraceID}
	switch {
	case errors.Is(err, context.DeadlineExceeded):
		result.Status = interfaces.InvocationTerminalStatusTimedOut
		result.ErrorCode = string(interfaces.InvocationErrorCodeTimedOut)
		result.Message = "invocation timed out while waiting for primary result"
	case errors.Is(err, context.Canceled):
		result.Status = interfaces.InvocationTerminalStatusCanceled
		result.ErrorCode = string(interfaces.InvocationErrorCodeCanceled)
		result.Message = "invocation was canceled while waiting for primary result"
	}
	failureClass := "timeout"
	if result.Status == interfaces.InvocationTerminalStatusCanceled {
		failureClass = "cancellation"
	}
	o.recordFailure(sessionID, input, result, failureClass)
	return result, nil
}

func (o *SessionOwner) recordFailure(
	sessionID string,
	input SessionInvocationWaitInput,
	result FactoryInvocationResult,
	failureClass string,
) {
	if o.telemetry != nil {
		o.telemetry.InvocationFailed(input.FactoryConfig, input.InputSource, result.ErrorCode)
		o.telemetry.LogInvocationFailed(sessionID, input, result, failureClass)
	}
}

func (o *SessionOwner) waitNext(ctx context.Context) error {
	if o.waitNextFn != nil {
		return o.waitNextFn(ctx)
	}
	timer := time.NewTimer(sessionInvocationPollInterval)
	defer timer.Stop()
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-timer.C:
		return nil
	}
}

func invocationWaitContext(ctx context.Context, timeoutMillis *int64) (context.Context, context.CancelFunc) {
	if timeoutMillis != nil && *timeoutMillis > 0 {
		return context.WithTimeout(ctx, time.Duration(*timeoutMillis)*time.Millisecond)
	}
	return ctx, func() {}
}

func failureClassForPrimaryResultError(code work.PrimaryResultErrorCode) string {
	switch code {
	case work.PrimaryResultErrorCodeFailed:
		return "failed"
	case work.PrimaryResultErrorCodePaused:
		return "paused"
	case work.PrimaryResultErrorCodeInterrupted:
		return "interrupted"
	case work.PrimaryResultErrorCodeBlocked:
		return "blocked"
	case work.PrimaryResultErrorCodeNeedsHuman:
		return "needs_human"
	default:
		return "unresolved_primary"
	}
}
