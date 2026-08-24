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
		return workers.WorkFailureTypeUnknown
	default:
		return workers.WorkFailureTypeUnknown
	}
}

func failureTypeForProviderFailure(failure providers.ExecuteFailure) workers.WorkFailureType {
	if isUnrecognizedProviderRefusal(failure) {
		return workers.WorkFailureTypePermanentBadRequest
	}
	return failureTypeForProviderKind(failure.Kind)
}

func isUnrecognizedProviderRefusal(failure providers.ExecuteFailure) bool {
	return failure.Kind == providers.ExecuteFailureKindUnknown &&
		failure.Diagnostics != nil &&
		failure.Diagnostics.Metadata[providers.ExecuteDiagnosticMetadataUnrecognizedProviderRefusal] == "true"
}

func hasUnrecognizedProviderRefusalMarker(failure *workers.ProviderError) bool {
	if failure == nil || failure.Diagnostics == nil {
		return false
	}
	if failure.Diagnostics.Metadata[providers.ExecuteDiagnosticMetadataUnrecognizedProviderRefusal] == "true" {
		return true
	}
	return failure.Diagnostics.Provider != nil &&
		failure.Diagnostics.Provider.ResponseMetadata[providers.ExecuteDiagnosticMetadataUnrecognizedProviderRefusal] == "true"
}

const failureMessageRuneLimit = 512

const (
	agentTimeoutFailureMessage         = "provider invocation timed out"
	agentCanceledFailureMessage        = "provider invocation was canceled"
	unrecognizedProviderFailureMessage = "provider rejected the execution request"
)

func canonicalAgentFailureMessage(
	failure providers.ExecuteFailure,
	failureType workers.WorkFailureType,
	providerMessage string,
) string {
	if isUnrecognizedProviderRefusal(failure) {
		return unrecognizedProviderFailureMessage
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

func providerRequest(request workers.RunnerExecutionRequest) providers.ExecuteRequest {
	providerID := providerIDForRequest(request)
	dispatchID := strings.TrimSpace(request.Correlation.DispatchID)
	if dispatchID == "" {
		dispatchID = strings.TrimSpace(request.Dispatch.DispatchID)
	}
	attemptID := strings.TrimSpace(request.Correlation.AttemptID)
	if attemptID == "" {
		attemptID = dispatchID
	}
	requestID := strings.TrimSpace(request.Correlation.RequestID)
	if requestID == "" {
		requestID = strings.TrimSpace(request.Dispatch.Execution.RequestID)
	}
	traceID := strings.TrimSpace(request.Correlation.TraceID)
	if traceID == "" {
		traceID = strings.TrimSpace(request.Dispatch.Execution.TraceID)
	}
	return providers.ExecuteRequest{
		Provider: providerID,
		// Providers' native session identity remains dispatch-based. The
		// detached attempt identity is carried independently in Correlation.
		AttemptID: dispatchID,
		Correlation: providers.ExecuteCorrelation{
			FactorySessionID: request.Correlation.FactorySessionID,
			RuntimeID:        request.Correlation.RuntimeID,
			GenerationID:     request.Correlation.GenerationID,
			DispatchID:       dispatchID,
			AttemptID:        attemptID,
			RequestID:        requestID,
			TraceID:          traceID,
			ReplayKey:        request.Dispatch.Execution.ReplayKey,
			WorkIDs:          append([]string(nil), request.Dispatch.Execution.WorkIDs...),
		},
		WorkerType:               request.WorkerType,
		WorkstationName:          request.WorkstationType,
		ProjectID:                request.ProjectID,
		TransitionID:             request.Dispatch.TransitionID,
		InputBindings:            cloneProviderInputBindings(request.Dispatch.InputBindings),
		Model:                    request.Model,
		ReasoningEffort:          request.ReasoningEffort,
		SkipPermissions:          request.SkipPermissions,
		PrintTimeout:             request.PrintTimeout,
		SystemPrompt:             request.SystemPrompt,
		UserMessage:              request.UserMessage,
		InputTokens:              cloneInputTokens(request.InputTokens),
		OutputSchema:             request.OutputSchema,
		WorkingDirectory:         request.WorkingDirectory,
		Worktree:                 request.Worktree,
		EnvVars:                  cloneMetadata(request.EnvVars),
		ProcessEnvironment:       append([]string(nil), request.ProcessEnvironment...),
		ExecutionLogger:          request.ExecutionLogger,
		ProcessLifecycleObserver: request.ProcessLifecycleObserver,
	}
}

func cloneProviderInputBindings(values map[string][]string) map[string][]string {
	if len(values) == 0 {
		return nil
	}
	cloned := make(map[string][]string, len(values))
	for key, items := range values {
		cloned[key] = append([]string(nil), items...)
	}
	return cloned
}
