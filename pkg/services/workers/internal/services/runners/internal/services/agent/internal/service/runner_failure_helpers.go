package service

import (
	"context"
	"errors"
	"strings"

	"github.com/portpowered/infinite-you/pkg/services/providers"
	"github.com/portpowered/infinite-you/pkg/services/workers"
)

func canceledProviderError(cause error, result workers.RunnerExecutionResult) *workers.ProviderError {
	normalized := workers.NewProviderError(
		workers.WorkFailureTypeUnknown,
		agentCanceledFailureMessage,
		cause,
	)
	normalized.Continuation = cloneContinuation(result.Continuation)
	normalized.Diagnostics = workers.CloneWorkDiagnostics(result.Diagnostics)
	return normalized
}

func normalizeExecutionError(ctx context.Context, err error) error {
	if contextErr := ctx.Err(); contextErr != nil {
		return errors.Join(contextErr, err)
	}
	return workers.NewProviderError(
		workers.WorkFailureTypeInternalServerError,
		boundedFailureMessage(err.Error()),
		err,
	)
}

func failureTypeForProviderKind(kind providers.ExecuteFailureKind) workers.WorkFailureType {
	switch kind {
	case providers.ExecuteFailureKindAuthentication:
		return workers.WorkFailureTypeAuthFailure
	case providers.ExecuteFailureKindInvalidRequest, providers.ExecuteFailureKindCapabilityMismatch:
		return workers.WorkFailureTypePermanentBadRequest
	case providers.ExecuteFailureKindMisconfigured:
		return workers.WorkFailureTypeMisconfigured
	case providers.ExecuteFailureKindThrottled:
		return workers.WorkFailureTypeThrottled
	case providers.ExecuteFailureKindDependency:
		return workers.WorkFailureTypeInternalServerError
	case providers.ExecuteFailureKindTimeout:
		return workers.WorkFailureTypeTimeout
	case providers.ExecuteFailureKindUnknown, providers.ExecuteFailureKindSessionNotFound:
		// A provider refusal that has no specific classifier is deterministic
		// for the same request. Map it onto the existing terminal route so it
		// cannot follow an authored retry arc or expose provider output.
		return workers.WorkFailureTypePermanentBadRequest
	default:
		return workers.WorkFailureTypePermanentBadRequest
	}
}

const failureMessageRuneLimit = 512

const (
	agentTimeoutFailureMessage           = "provider invocation timed out"
	agentCanceledFailureMessage          = "provider invocation was canceled"
	unrecognizedProviderFailureMessage   = "provider rejected the execution request"
	missingProviderSessionFailureMessage = "provider session was not found"
)

func canonicalAgentFailureMessage(
	providerKind providers.ExecuteFailureKind,
	failureType workers.WorkFailureType,
	providerMessage string,
) string {
	switch providerKind {
	case providers.ExecuteFailureKindUnknown:
		return unrecognizedProviderFailureMessage
	case providers.ExecuteFailureKindSessionNotFound:
		return missingProviderSessionFailureMessage
	}
	switch failureType {
	case workers.WorkFailureTypeTimeout:
		return agentTimeoutFailureMessage
	case workers.WorkFailureTypeUnknown:
		if strings.TrimSpace(providerMessage) == "" {
			return "provider invocation failed"
		}
	}
	return providerMessage
}

func boundedFailureMessage(message string) string {
	message = strings.TrimSpace(message)
	runes := []rune(message)
	if len(runes) <= failureMessageRuneLimit {
		return message
	}
	return string(runes[:failureMessageRuneLimit])
}

func cloneMetadata(values map[string]string) map[string]string {
	if values == nil {
		return nil
	}
	cloned := make(map[string]string, len(values))
	for key, value := range values {
		cloned[key] = value
	}
	return cloned
}

func cloneInputTokens(values []any) []any {
	if values == nil {
		return nil
	}
	return append([]any(nil), values...)
}

func badRequest(message string, cause error) error {
	return workers.NewProviderError(workers.WorkFailureTypePermanentBadRequest, message, cause)
}

func misconfigured(message string, cause error) error {
	return workers.NewProviderError(workers.WorkFailureTypeMisconfigured, message, cause)
}
