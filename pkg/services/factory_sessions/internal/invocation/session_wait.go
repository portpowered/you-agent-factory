package invocation

import (
	"context"
	"errors"
	"time"

	interfaces "github.com/portpowered/infinite-you/pkg/services/factory_definitions"
	factorysessions "github.com/portpowered/infinite-you/pkg/services/factory_sessions"
	"github.com/portpowered/infinite-you/pkg/services/work"
)

// A timed-out invocation still owes its Factory Session a cancel-on-timeout
// control request, and the expired wait context cannot bound that call. The
// control context stays detached from cancellation so the request is still
// issued, but it carries a deadline so a stalled lifecycle gateway cannot hold
// an already-resolved invocation open indefinitely. Tests shorten this bound;
// production code never reassigns it.
var sessionTimeoutCancelControlTimeout = 15 * time.Second

// Context cancellation cannot interrupt a non-cooperative lifecycle gateway.
// Limit outstanding detached cancel calls so repeated timeouts stay bounded.
var sessionTimeoutCancelSlots = make(chan struct{}, 16)

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
	waitNext, releaseWaiter := o.authority.WaitSession(waitCtx, sessionID)
	defer releaseWaiter()

	packaged := o.specialCase != nil && o.specialCase.Active(input.FactoryConfig)
	loggedActive := false
	for {
		observation, err := o.authority.Observe(waitCtx, sessionID, input)
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
			// A cancellation can arrive after resolveObservation's result check
			// but before it returns from failure classification. Re-check at the
			// wait boundary so that classification cannot publish a stale result.
			if ctx != nil {
				if contextErr := ctx.Err(); contextErr != nil {
					alreadyCanceled := result.Status == interfaces.InvocationTerminalStatusCanceled && err == nil
					if !alreadyCanceled {
						result, waitErr := o.waitErrorResult(sessionID, input, contextErr)
						return result, waitErr
					}
				}
			}
			return result, err
		}
		if err := waitNext(waitCtx); err != nil {
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
	// Work checks the context before selecting a result. Re-check it after the
	// call so a cancellation racing with that check cannot be classified as a
	// successful or runtime-failed invocation.
	if result, canceled, waitErr := o.canceledObservationResult(ctx, sessionID, input); canceled {
		return result, true, waitErr
	}
	if err == nil {
		if packaged {
			if telemetry, ok := o.telemetry.(SessionInvocationPackagedTelemetry); ok {
				telemetry.PackagedInvocationCompleted(sessionID, input, selection)
			}
		}
		return o.completedResult(sessionID, input, selection), true, nil
	}
	if isInvocationWaitError(err) {
		result, waitErr := o.waitErrorResult(sessionID, input, err)
		return result, true, waitErr
	}
	primaryErr, ok := err.(*work.PrimaryResultError)
	if !ok {
		return FactoryInvocationResult{}, true, err
	}
	if observation.MissingPrimaryResult != nil {
		return o.failedResult(sessionID, input, observation.MissingPrimaryResult, observation.WorldState), true, nil
	}
	if classified, ok := work.ClassifyInvocationControlState(sessionID, observation.FactoryState, selectionInput); ok {
		return o.failedResult(sessionID, input, classified, observation.WorldState), true, nil
	}
	if classified, ok := work.ClassifyMissingPrimaryResult(selectionInput); ok {
		return o.failedResult(sessionID, input, classified, observation.WorldState), true, nil
	}
	if packaged {
		worldState, _ := selectionInput.WorldState.(interfaces.FactoryWorldState)
		if result, ok := o.packagedTerminalFailureResult(sessionID, input, worldState); ok {
			return result, true, nil
		}
	}
	if classified, ok := work.ClassifyFailedInvocation(sessionID, selectionInput); ok {
		return o.failedResult(sessionID, input, classified, observation.WorldState), true, nil
	}
	if _, exists := observation.WorldState.WorkRequestsByID[input.RequestID]; !exists || observation.ActiveWork {
		return FactoryInvocationResult{}, false, nil
	}
	return o.resolveStoppedInvocation(sessionID, input, selectionInput, primaryErr, packaged), true, nil
}

func (o *SessionOwner) canceledObservationResult(
	ctx context.Context,
	sessionID string,
	input SessionInvocationWaitInput,
) (FactoryInvocationResult, bool, error) {
	if ctx == nil {
		return FactoryInvocationResult{}, false, nil
	}
	contextErr := ctx.Err()
	if contextErr == nil {
		return FactoryInvocationResult{}, false, nil
	}
	result, waitErr := o.waitErrorResult(sessionID, input, contextErr)
	return result, true, waitErr
}

func isInvocationWaitError(err error) bool {
	return errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded)
}

func (o *SessionOwner) resolveStoppedInvocation(
	sessionID string,
	input SessionInvocationWaitInput,
	selectionInput work.PrimaryResultSelectionInput,
	primaryErr *work.PrimaryResultError,
	packaged bool,
) FactoryInvocationResult {
	worldState, _ := selectionInput.WorldState.(interfaces.FactoryWorldState)
	if packaged {
		if result, ok := o.packagedTerminalFailureResult(sessionID, input, worldState); ok {
			return result
		}
	}
	if classified, ok := work.ClassifyInvocationControlState(sessionID, "", selectionInput); ok {
		return o.failedResult(sessionID, input, classified, worldState)
	}
	if classified, ok := work.ClassifyFailedInvocation(sessionID, selectionInput); ok {
		return o.failedResult(sessionID, input, classified, worldState)
	}
	if classified, ok := work.ClassifyMissingPrimaryResult(selectionInput); ok {
		return o.failedResult(sessionID, input, classified, worldState)
	}
	return o.failedResult(sessionID, input, primaryErr, worldState)
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
	worldState interfaces.FactoryWorldState,
) FactoryInvocationResult {
	result := FactoryInvocationResult{
		RequestID: input.RequestID, TraceID: input.TraceID,
		Status:    interfaces.InvocationTerminalStatusFailed,
		ErrorCode: string(primaryErr.Code), Message: primaryErr.Message,
		SessionID: primaryErr.Context.SessionID, WorkID: primaryErr.Context.WorkID,
		WorkName: primaryErr.Context.WorkName, WorkState: primaryErr.Context.WorkState,
		ApprovalID: primaryErr.Context.ApprovalID, DispatchID: primaryErr.Context.DispatchID,
		WorkstationID: primaryErr.Context.WorkstationID, WorkstationName: primaryErr.Context.WorkstationName,
		Decisions: append([]string(nil), primaryErr.Context.Decisions...),
	}
	if detail, ok := worldState.FailureDetailsByWorkID[result.WorkID]; ok && detail.FailureDetail != nil {
		result.FailureReason = string(detail.FailureDetail.Reason)
	} else if result.DispatchID != "" {
		matches := 0
		for _, detail := range worldState.FailureDetailsByWorkID {
			if detail.DispatchID != result.DispatchID || detail.FailureDetail == nil {
				continue
			}
			matches++
			result.FailureReason = string(detail.FailureDetail.Reason)
		}
		if matches != 1 {
			result.FailureReason = ""
		}
	}
	if result.FailureReason == "" && len(worldState.FailureDetailsByWorkID) == 1 {
		// A dispatch can fail before Work reaches a failed place. The primary
		// result then remains unresolved, but its sole recorded failure still
		// identifies the provider outcome for this one-shot invocation.
		for _, detail := range worldState.FailureDetailsByWorkID {
			if detail.FailureDetail != nil {
				result.FailureReason = string(detail.FailureDetail.Reason)
			}
		}
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
	result := FactoryInvocationResult{RequestID: input.RequestID, TraceID: input.TraceID, WorkID: input.WorkID}
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
	if result.Status == interfaces.InvocationTerminalStatusTimedOut && input.CancelOnTimeout {
		// The cancel-on-timeout control must run even though the invocation wait
		// already expired, so it detaches from the canceled wait context. Give
		// cooperative control paths a deadline rather than waiting indefinitely.
		cancelCtx, stopCancel := context.WithTimeout(context.WithoutCancel(context.Background()), sessionTimeoutCancelControlTimeout)
		defer stopCancel()
		var cancelErr error
		select {
		case sessionTimeoutCancelSlots <- struct{}{}:
			completed := make(chan error, 1)
			go func() {
				defer func() { <-sessionTimeoutCancelSlots }()
				_, err := o.controls.CancelLiveFactorySession(cancelCtx, sessionID, factorysessions.ControlRequest{
					RequestID: input.RequestID,
					Reason:    "invocation wait timed out",
				})
				completed <- err
			}()
			select {
			case cancelErr = <-completed:
			case <-cancelCtx.Done():
				cancelErr = cancelCtx.Err()
			}
		case <-cancelCtx.Done():
			cancelErr = cancelCtx.Err()
		}
		if cancelErr != nil {
			result.Message = "invocation timed out while waiting for primary result; cancel-on-timeout control failed"
		}
	}
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
