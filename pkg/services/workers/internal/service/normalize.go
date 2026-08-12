package service

import (
	"context"
	"errors"
	"strings"
	"time"

	"github.com/portpowered/infinite-you/pkg/services/work"
	"github.com/portpowered/infinite-you/pkg/services/workers"
)

func (s *Service) normalizeResult(
	correlation workers.ExecutionCorrelation,
	request workers.ExecuteRequest,
	runnerResult workers.RunnerExecutionResult,
	runErr error,
	duration time.Duration,
) workers.ExecuteResult {
	result := workers.ExecuteResult{
		Correlation: correlation,
		Metrics: workers.ExecutionMetrics{
			Duration: duration,
		},
		Diagnostics: safeDiagnosticsFromWork(runnerResult.Diagnostics),
		Continuation: continuationFromSession(
			runnerResult.ProviderSession,
			request.Input.Resume,
		),
	}
	if runErr == nil {
		result.Outcome = workers.ExecutionOutcomeAccepted
		result.Output = proposedOutputFromContent(runnerResult.Content)
		return result
	}

	if errors.Is(runErr, context.Canceled) {
		result.Outcome = workers.ExecutionOutcomeCanceled
		result.Failure = &workers.ExecutionFailure{
			Type:    workers.WorkFailureTypeUnknown,
			Family:  workers.WorkFailureFamilyTerminal,
			Message: "execution canceled",
		}
		return result
	}
	if errors.Is(runErr, context.DeadlineExceeded) {
		result.Outcome = workers.ExecutionOutcomeFailed
		result.Failure = &workers.ExecutionFailure{
			Type:      workers.WorkFailureTypeTimeout,
			Family:    workers.WorkFailureFamilyRetryable,
			Message:   "execution timed out",
			RetryHint: true,
			Detail: &workers.FailureDetail{
				Reason:  workers.WorkFailureTypeTimeout,
				Message: "execution timed out",
			},
		}
		return result
	}

	result.Outcome = workers.ExecutionOutcomeFailed
	result.Failure = failureFromError(runErr)
	if result.Diagnostics == nil {
		var providerErr *workers.ProviderError
		if errors.As(runErr, &providerErr) && providerErr != nil {
			result.Diagnostics = safeDiagnosticsFromWork(providerErr.Diagnostics)
		}
	}
	if result.Continuation == nil {
		var providerErr *workers.ProviderError
		if errors.As(runErr, &providerErr) && providerErr != nil {
			result.Continuation = continuationFromSession(providerErr.ProviderSession, request.Input.Resume)
		}
	}
	if content := strings.TrimSpace(runnerResult.Content); content != "" {
		result.Output = proposedOutputFromContent(content)
	}
	return result
}

func proposedOutputFromContent(content string) workers.ProposedOutput {
	content = strings.TrimSpace(content)
	if content == "" {
		return workers.ProposedOutput{}
	}
	return workers.ProposedOutput{
		Primary: []work.WorkContentPart{{
			Type: work.WorkContentPartTypeText,
			Text: content,
		}},
	}
}

func failureFromError(err error) *workers.ExecutionFailure {
	if err == nil {
		return nil
	}
	var providerErr *workers.ProviderError
	if errors.As(err, &providerErr) && providerErr != nil {
		metadata := workers.WorkFailureMetadataFromProviderError(providerErr)
		decision := workers.FailureDecisionFromMetadata(metadata)
		failureType := workers.WorkFailureTypeUnknown
		family := workers.WorkFailureFamilyTerminal
		if metadata != nil {
			failureType = metadata.Type
			family = metadata.Family
		}
		message := strings.TrimSpace(providerErr.Message)
		if message == "" {
			message = strings.TrimSpace(providerErr.Error())
		}
		if message == "" {
			message = "worker execution failed"
		}
		return &workers.ExecutionFailure{
			Type:      failureType,
			Family:    family,
			Message:   message,
			RetryHint: decision.Retryable,
			Detail: &workers.FailureDetail{
				Reason:  failureType,
				Message: message,
			},
		}
	}
	message := strings.TrimSpace(err.Error())
	if message == "" {
		message = "worker execution failed"
	}
	return &workers.ExecutionFailure{
		Type:    workers.WorkFailureTypeUnknown,
		Family:  workers.WorkFailureFamilyTerminal,
		Message: message,
		Detail: &workers.FailureDetail{
			Reason:  workers.WorkFailureTypeUnknown,
			Message: message,
		},
	}
}

func continuationFromSession(
	session *workers.ProviderSessionMetadata,
	resume *workers.ProviderContinuationRef,
) *workers.ProviderContinuationRef {
	// Legacy runner results still carry Provider Session metadata. Execute
	// projects only the opaque continuation reference onto the public result.
	if session != nil && strings.TrimSpace(session.ID) != "" {
		return &workers.ProviderContinuationRef{
			Provider:          workers.CanonicalProviderSessionProvider(session.Provider),
			ProviderSessionID: session.ID,
			ExternalRef:       session.ID,
		}
	}
	if resume == nil {
		return nil
	}
	clone := *resume
	return &clone
}

func safeDiagnosticsFromWork(diagnostics *workers.WorkDiagnostics) *workers.SafeDiagnostics {
	if diagnostics == nil {
		return nil
	}
	safe := &workers.SafeDiagnostics{
		Metadata: cloneStringMap(diagnostics.Metadata),
	}
	if diagnostics.RenderedPrompt != nil {
		safe.RenderedPrompt = &workers.SafeRenderedPromptDiagnostic{
			SystemPromptHash: diagnostics.RenderedPrompt.SystemPromptHash,
			UserMessageHash:  diagnostics.RenderedPrompt.UserMessageHash,
			Variables:        cloneStringMap(diagnostics.RenderedPrompt.Variables),
		}
	}
	if diagnostics.Provider != nil {
		safe.Provider = &workers.SafeProviderDiagnostic{
			Provider:         diagnostics.Provider.Provider,
			Model:            diagnostics.Provider.Model,
			RequestMetadata:  cloneStringMap(diagnostics.Provider.RequestMetadata),
			ResponseMetadata: cloneStringMap(diagnostics.Provider.ResponseMetadata),
		}
	}
	if diagnostics.Invocation != nil {
		safe.Invocation = workers.CloneInvocationDiagnostic(diagnostics.Invocation)
	}
	if diagnostics.Command != nil {
		// Intentionally omit Stdin and Env so unsafe execution material is never
		// persisted through Execute diagnostics.
		safe.Command = &workers.SafeCommandDiagnostic{
			Command:    diagnostics.Command.Command,
			Args:       append([]string(nil), diagnostics.Command.Args...),
			Stdout:     diagnostics.Command.Stdout,
			Stderr:     diagnostics.Command.Stderr,
			ExitCode:   diagnostics.Command.ExitCode,
			TimedOut:   diagnostics.Command.TimedOut,
			Duration:   diagnostics.Command.Duration,
			WorkingDir: diagnostics.Command.WorkingDir,
		}
	}
	if diagnostics.Panic != nil {
		safe.Panic = &workers.PanicDiagnostic{
			Message: diagnostics.Panic.Message,
			Stack:   diagnostics.Panic.Stack,
		}
	}
	return safe
}
