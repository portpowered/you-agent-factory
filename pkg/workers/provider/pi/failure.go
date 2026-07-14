package pi

import (
	"context"
	"errors"

	provider "github.com/portpowered/infinite-you/pkg/workers/provider"
	"github.com/portpowered/infinite-you/pkg/interfaces"
	"github.com/portpowered/infinite-you/pkg/workers/provider/adapter"
)

func classifyPiFailure(input adapter.FailureContext) adapter.FailureResult {
	if !piFailureNeedsClassification(input) {
		return adapter.FailureResult{}
	}
	if result := classifyPiRetryFailure(input); result.Failure != nil {
		return result
	}
	if failure := parseTerminalFailure(input.CommandResult.Stdout); failure != nil {
		return terminalFailureResult(failure)
	}
	return classifyPiExecutionFailure(input)
}

func piFailureNeedsClassification(input adapter.FailureContext) bool {
	return input.CommandError != nil || input.CommandResult.ExitCode != 0 ||
		input.DecodeError != nil || input.FlushError != nil || input.ParseError != nil ||
		input.FlushReason == adapter.FlushReasonCanceled
}

func classifyPiRetryFailure(input adapter.FailureContext) adapter.FailureResult {
	if failure := piRetryFailureFromStdout(input.CommandResult.Stdout); failure != nil {
		return adapter.FailureResult{Failure: failure}
	}
	var retryErr *piRetryError
	if errors.As(input.ParseError, &retryErr) {
		failure := retryErr.failure
		return adapter.FailureResult{Failure: &failure}
	}
	return adapter.FailureResult{}
}

func classifyPiExecutionFailure(input adapter.FailureContext) adapter.FailureResult {
	if errors.Is(input.CommandError, context.Canceled) || input.FlushReason == adapter.FlushReasonCanceled {
		return normalizedFailureResult(interfaces.WorkFailureTypeUnknown, "Pi execution was canceled.", nil)
	}
	if errors.Is(input.CommandError, context.DeadlineExceeded) || input.CommandResult.ExitCode == 124 {
		return normalizedFailureResult(interfaces.WorkFailureTypeTimeout, "Pi execution timed out.", nil)
	}
	if input.CommandError != nil || input.CommandResult.ExitCode != 0 {
		return normalizedFailureResult(interfaces.WorkFailureTypeUnknown, "Pi invocation failed.", nil)
	}
	if input.DecodeError != nil || input.FlushError != nil || input.ParseError != nil {
		return normalizedFailureResult(interfaces.WorkFailureTypeUnknown, "Pi did not produce a valid completed response.", nil)
	}
	return adapter.FailureResult{}
}

func terminalFailureResult(err error) adapter.FailureResult {
	message := "Pi returned a terminal failure"
	if err != nil {
		message = err.Error()
	}
	return normalizedFailureResult(interfaces.WorkFailureTypeUnknown, message, nil)
}

func normalizedFailureResult(failureType interfaces.WorkFailureType, message string, session *interfaces.ProviderSessionMetadata) adapter.FailureResult {
	providerError := provider.NewProviderErrorFromResult(provider.ProviderFailureResult{Reason: failureType, Message: message}, nil)
	decision := provider.WorkFailureDecisionFromProviderError(providerError)
	return adapter.FailureResult{Failure: &adapter.FailureFacts{
		Family: providerError.Family, Type: providerError.Type, Message: providerError.Message,
		Retry: adapter.RetryGuidance{Retryable: decision.Retryable}, ProviderSession: session,
	}}
}
