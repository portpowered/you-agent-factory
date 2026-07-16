package invocation

import (
	"context"
	"errors"
	"time"

	interfaces "github.com/portpowered/infinite-you/pkg/factory/contracts"
	workinvocation "github.com/portpowered/infinite-you/pkg/work/invocation"
)

const sessionInvocationPollInterval = 10 * time.Millisecond

// SessionInvocationObservation is one event-derived view of invocation state.
// Runtime adapters populate it without exposing engine or Petri-net types.
type SessionInvocationObservation struct {
	WorldState           interfaces.FactoryWorldState
	FactoryState         string
	ActiveWork           bool
	MissingPrimaryResult *workinvocation.PrimaryResultError
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

	packaged := o.deps.SpecialCase != nil && o.deps.SpecialCase.Active(input.FactoryConfig)
	loggedActive := false
	for {
		observation, err := o.deps.Observe(waitCtx, sessionID, input)
		if err != nil {
			return o.waitErrorResult(sessionID, input, err)
		}
		if packaged && observation.ActiveWork && !loggedActive {
			if telemetry, ok := o.deps.Telemetry.(SessionInvocationPackagedTelemetry); ok {
				telemetry.PackagedInvocationActive(sessionID, input)
			}
			loggedActive = true
		}
		if result, done, err := o.resolveObservation(sessionID, input, observation, packaged); done {
			return result, err
		}
		if err := o.waitNext(waitCtx); err != nil {
			return o.waitErrorResult(sessionID, input, err)
		}
	}
}

func (o *SessionOwner) resolveObservation(
	sessionID string,
	input SessionInvocationWaitInput,
	observation SessionInvocationObservation,
	packaged bool,
) (FactoryInvocationResult, bool, error) {
	selectionInput := workinvocation.PrimaryResultSelectionInput{
		RequestID: input.RequestID, InvocationReturn: input.InvocationReturn, WorldState: observation.WorldState,
	}
	selection, err := workinvocation.ResolvePrimaryResult(selectionInput)
	if err == nil {
		if packaged {
			if telemetry, ok := o.deps.Telemetry.(SessionInvocationPackagedTelemetry); ok {
				telemetry.PackagedInvocationCompleted(sessionID, input, selection)
			}
		}
		return o.completedResult(sessionID, input, selection), true, nil
	}
	primaryErr, ok := err.(*workinvocation.PrimaryResultError)
	if !ok {
		return FactoryInvocationResult{}, true, err
	}
	if observation.MissingPrimaryResult != nil {
		return o.failedResult(sessionID, input, observation.MissingPrimaryResult), true, nil
	}
	if classified, ok := workinvocation.ClassifyInvocationControlState(sessionID, observation.FactoryState, selectionInput); ok {
		return o.failedResult(sessionID, input, classified), true, nil
	}
	if classified, ok := workinvocation.ClassifyMissingPrimaryResult(selectionInput); ok {
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
	selectionInput workinvocation.PrimaryResultSelectionInput,
	primaryErr *workinvocation.PrimaryResultError,
	packaged bool,
) FactoryInvocationResult {
	if packaged {
		if failure := o.deps.SpecialCase.TerminalFailure(selectionInput.WorldState, input.RequestID); failure != nil {
			result := FactoryInvocationResult{
				RequestID: input.RequestID, TraceID: input.TraceID,
				Status:    interfaces.InvocationTerminalStatusFailed,
				ErrorCode: failure.ErrorCode, Message: failure.Message,
			}
			// Packaged terminal failures retain the general failure metric, but
			// their diagnostic record is the packaged failure log emitted below.
			if o.deps.Telemetry != nil {
				o.deps.Telemetry.InvocationFailed(input.FactoryConfig, input.InputSource, result.ErrorCode)
			}
			if telemetry, ok := o.deps.Telemetry.(SessionInvocationPackagedTelemetry); ok {
				telemetry.PackagedInvocationFailed(sessionID, input, *failure)
			}
			return result
		}
	}
	if classified, ok := workinvocation.ClassifyInvocationControlState(sessionID, "", selectionInput); ok {
		return o.failedResult(sessionID, input, classified)
	}
	if classified, ok := workinvocation.ClassifyFailedInvocation(sessionID, selectionInput); ok {
		return o.failedResult(sessionID, input, classified)
	}
	if classified, ok := workinvocation.ClassifyMissingPrimaryResult(selectionInput); ok {
		return o.failedResult(sessionID, input, classified)
	}
	return o.failedResult(sessionID, input, primaryErr)
}

func (o *SessionOwner) completedResult(
	sessionID string,
	input SessionInvocationWaitInput,
	selection workinvocation.PrimaryResultSelection,
) FactoryInvocationResult {
	result := FactoryInvocationResult{
		RequestID: input.RequestID, TraceID: input.TraceID,
		Status: interfaces.InvocationTerminalStatusCompleted, PrimaryResult: selection.PrimaryResult,
	}
	if o.deps.Telemetry != nil {
		o.deps.Telemetry.InvocationCompleted(input.FactoryConfig, input.InputSource, selection.PrimaryResult)
		o.deps.Telemetry.LogInvocationCompleted(sessionID, input, selection)
	}
	return result
}

func (o *SessionOwner) failedResult(
	sessionID string,
	input SessionInvocationWaitInput,
	primaryErr *workinvocation.PrimaryResultError,
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
	if o.deps.Telemetry != nil {
		o.deps.Telemetry.InvocationFailed(input.FactoryConfig, input.InputSource, result.ErrorCode)
		o.deps.Telemetry.LogInvocationFailed(sessionID, input, result, failureClass)
	}
}

func (o *SessionOwner) waitNext(ctx context.Context) error {
	if o.deps.WaitNext != nil {
		return o.deps.WaitNext(ctx)
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

func failureClassForPrimaryResultError(code workinvocation.PrimaryResultErrorCode) string {
	switch code {
	case workinvocation.PrimaryResultErrorCodeFailed:
		return "failed"
	case workinvocation.PrimaryResultErrorCodePaused:
		return "paused"
	case workinvocation.PrimaryResultErrorCodeInterrupted:
		return "interrupted"
	case workinvocation.PrimaryResultErrorCodeBlocked:
		return "blocked"
	case workinvocation.PrimaryResultErrorCodeNeedsHuman:
		return "needs_human"
	default:
		return "unresolved_primary"
	}
}
