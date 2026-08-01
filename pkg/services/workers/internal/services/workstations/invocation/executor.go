// Package invocation implements the Workers provider-invocation boundary.
package invocation

import (
	"context"

	"github.com/portpowered/infinite-you/pkg/services/workers"
)

// Executor adapts the public Runner contract to one Worker invocation attempt.
type Executor struct {
	runner workers.Runner
}

// NewExecutor constructs the concrete Workers invocation adapter selected by
// Wire and same-owner Workers constituents.
func NewExecutor(runner workers.Runner) workers.InvocationExecutor {
	if runner == nil {
		return nil
	}
	return &Executor{runner: runner}
}

// NewRunnerExecutor constructs the concrete adapter without narrowing its
// return type. It is useful in same-owner tests that exercise adapter details.
func NewRunnerExecutor(runner workers.Runner) *Executor {
	return &Executor{runner: runner}
}

func (e *Executor) Execute(
	ctx context.Context,
	input workers.InvocationInput,
) (workers.InvocationResult, error) {
	attempt := input.Attempt
	if attempt < 1 {
		attempt = 1
	}
	if err := ctx.Err(); err != nil {
		return failedInvocationResult(attempt, err), err
	}
	if e == nil || e.runner == nil {
		err := workers.NewProviderError(
			workers.WorkFailureTypeMisconfigured,
			"provider execution requires a provider",
			nil,
		)
		return failedInvocationResult(attempt, err), err
	}

	response, err := e.runner.Execute(ctx, input.Request)
	if err != nil {
		return failedInvocationResult(attempt, err), err
	}
	response.ProviderSession = canonicalProviderSession(response.ProviderSession)
	return workers.InvocationResult{
		Response:        response,
		Attempt:         attempt,
		ProviderSession: workers.CloneProviderSessionMetadata(response.ProviderSession),
		Diagnostics:     workers.SafeWorkDiagnosticsFromWorkDiagnostics(response.Diagnostics),
	}, nil
}

func failedInvocationResult(attempt int, err error) workers.InvocationResult {
	providerErr := workers.NormalizeProviderExecutionError(err)
	if providerErr == nil {
		providerErr = workers.NewProviderError(
			workers.WorkFailureTypeUnknown,
			"Provider execution failed.",
			err,
		)
	}
	providerErr.ProviderSession = canonicalProviderSession(providerErr.ProviderSession)
	metadata := &workers.WorkFailureMetadata{
		Family: providerErr.Family,
		Type:   providerErr.Type,
	}
	decision := workers.FailureDecisionFromMetadata(metadata)
	return workers.InvocationResult{
		Attempt:         attempt,
		ProviderSession: workers.CloneProviderSessionMetadata(providerErr.ProviderSession),
		FailureMetadata: metadata,
		FailureDecision: &decision,
		FailureDetail: &workers.FailureDetail{
			Reason:  providerErr.Type,
			Message: safeFailureMessage(providerErr.Type),
		},
		Diagnostics: workers.SafeWorkDiagnosticsFromWorkDiagnostics(providerErr.Diagnostics),
	}
}

func canonicalProviderSession(
	session *workers.ProviderSessionMetadata,
) *workers.ProviderSessionMetadata {
	clone := workers.CloneProviderSessionMetadata(session)
	if clone != nil {
		clone.Provider = workers.CanonicalProviderSessionProvider(clone.Provider)
	}
	return clone
}

func safeFailureMessage(reason workers.WorkFailureType) string {
	switch reason {
	case workers.WorkFailureTypeAuthFailure:
		return "Provider authentication failed."
	case workers.WorkFailureTypePermanentBadRequest:
		return "Provider rejected the request as invalid."
	case workers.WorkFailureTypeThrottled:
		return "Provider is temporarily unavailable due to usage or capacity limits."
	case workers.WorkFailureTypeInternalServerError:
		return "Provider encountered a temporary server error."
	case workers.WorkFailureTypeTimeout:
		return "Provider request timed out."
	case workers.WorkFailureTypeMisconfigured:
		return "Provider command could not be started."
	case workers.WorkFailureTypeMissingExecutable:
		return "Provider executable could not be found."
	case workers.WorkFailureTypeCommandLineTooLong:
		return "Provider command exceeded the operating system command-line limit."
	default:
		return "Provider execution failed."
	}
}
