package service

import (
	"context"
	"fmt"
	"reflect"
	"strings"

	dispatchplanning "github.com/portpowered/infinite-you/pkg/services/factory_runtime/internal/services/dispatch_planning"
)

// Retire correlates one terminal result and returns RETIRED exactly once. The
// retained result is an idempotency tombstone: it is no longer publishable but
// remains available to classify equivalent and conflicting redelivery.
func (p *Planner) Retire(
	ctx context.Context,
	result dispatchplanning.TerminalResult,
) (dispatchplanning.RetirementResult, error) {
	if err := validateTerminalResult(ctx, result); err != nil {
		return dispatchplanning.RetirementResult{}, err
	}

	p.mu.Lock()
	defer p.mu.Unlock()
	record := p.byCorrelation[result.CorrelationID]
	if record == nil {
		return dispatchplanning.RetirementResult{}, fmt.Errorf(
			"%w: correlation ID %q is not present",
			dispatchplanning.ErrUnknownDispatchCorrelation,
			result.CorrelationID,
		)
	}
	if err := validateResultAgainstIntent(record, result); err != nil {
		return dispatchplanning.RetirementResult{}, err
	}
	if record.result != nil {
		if reflect.DeepEqual(*record.result, result) {
			return retirementResult(dispatchplanning.RetirementOutcomeDuplicateIdempotent, result), nil
		}
		return dispatchplanning.RetirementResult{}, fmt.Errorf(
			"%w: terminal result conflicts with the first accepted outcome",
			dispatchplanning.ErrInvalidDispatchResultBoundary,
		)
	}
	if record.status != dispatchplanning.OutboxIntentStatusPublished &&
		record.status != dispatchplanning.OutboxIntentStatusPublishing {
		return dispatchplanning.RetirementResult{}, fmt.Errorf(
			"%w: dispatch %q has not been published",
			dispatchplanning.ErrInvalidDispatchResultBoundary,
			result.DispatchID,
		)
	}

	stable := result
	record.result = &stable
	record.status = dispatchplanning.OutboxIntentStatusRetired
	return retirementResult(dispatchplanning.RetirementOutcomeRetired, result), nil
}

func validateTerminalResult(ctx context.Context, result dispatchplanning.TerminalResult) error {
	if ctx == nil {
		return fmt.Errorf("%w: context is required", dispatchplanning.ErrInvalidDispatchResultBoundary)
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	if strings.TrimSpace(result.CorrelationID) == "" {
		return fmt.Errorf(
			"%w: correlation ID is required",
			dispatchplanning.ErrUnknownDispatchCorrelation,
		)
	}
	if strings.TrimSpace(result.DispatchID) == "" || strings.TrimSpace(result.WorkID) == "" {
		return fmt.Errorf(
			"%w: dispatch ID and Work ID are required",
			dispatchplanning.ErrInvalidDispatchResultBoundary,
		)
	}
	switch result.Outcome {
	case dispatchplanning.TerminalResultOutcomeSuccess,
		dispatchplanning.TerminalResultOutcomeFailure,
		dispatchplanning.TerminalResultOutcomeCancelled:
		return nil
	default:
		return fmt.Errorf(
			"%w: result outcome %q is not terminal",
			dispatchplanning.ErrInvalidDispatchResultBoundary,
			result.Outcome,
		)
	}
}

func validateResultAgainstIntent(record *intentRecord, result dispatchplanning.TerminalResult) error {
	dispatch := record.action.Request.Execution.Dispatch
	if record.action.CorrelationID != result.CorrelationID ||
		dispatch.DispatchID != result.DispatchID ||
		!containsValue(dispatch.Execution.WorkIDs, result.WorkID) {
		return fmt.Errorf(
			"%w: result identities or Work scope conflict with the accepted intent",
			dispatchplanning.ErrInvalidDispatchResultBoundary,
		)
	}
	return nil
}

func retirementResult(
	outcome dispatchplanning.RetirementOutcome,
	result dispatchplanning.TerminalResult,
) dispatchplanning.RetirementResult {
	return dispatchplanning.RetirementResult{
		Outcome:       outcome,
		DispatchID:    result.DispatchID,
		CorrelationID: result.CorrelationID,
	}
}

func cloneTerminalResult(result *dispatchplanning.TerminalResult) *dispatchplanning.TerminalResult {
	if result == nil {
		return nil
	}
	cloned := *result
	return &cloned
}

func containsValue(values []string, want string) bool {
	for _, value := range values {
		if value == want {
			return true
		}
	}
	return false
}
