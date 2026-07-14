package pi

import (
	provider "github.com/portpowered/infinite-you/pkg/workers/provider"
	"github.com/portpowered/infinite-you/pkg/interfaces"
	"github.com/portpowered/infinite-you/pkg/workers/provider/adapter"
)

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
