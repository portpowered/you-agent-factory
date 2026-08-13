package service

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/portpowered/infinite-you/pkg/services/providers"
	"github.com/portpowered/infinite-you/pkg/services/work"
	"github.com/portpowered/infinite-you/pkg/services/workers"
	workerexecutor "github.com/portpowered/infinite-you/pkg/services/workers/internal/services/workstations/executor"
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
		result.Outcome = normalizeRunnerOutcome(runnerResult.Outcome)
		result.Output = proposedOutputFromRunnerResult(runnerResult)
		if result.Outcome == workers.ExecutionOutcomeFailed {
			runErr = errors.New("runner returned failed outcome")
		} else if result.Outcome == workers.ExecutionOutcomeAccepted {
			contractErr := workerexecutor.ValidateOutputContract(
				runnerResult.Content,
				request.Target.Output.Contract,
			)
			if contractErr != nil {
				runErr = workers.NewProviderError(
					workers.WorkFailureTypePermanentBadRequest,
					"output contract failed",
					contractErr,
				)
			}
		}
	}
	if runErr == nil {
		return result
	}

	if errors.Is(runErr, context.Canceled) {
		result.Outcome = workers.ExecutionOutcomeCanceled
		var providerErr *workers.ProviderError
		if errors.As(runErr, &providerErr) && providerErr != nil {
			result.Failure = failureFromError(providerErr)
		} else if request.Target.RunnerID == "script" {
			result.Failure = &workers.ExecutionFailure{
				Type:    workers.WorkFailureTypeUnknown,
				Family:  workers.WorkFailureFamilyTerminal,
				Message: "execution cancelled: context canceled",
			}
		} else {
			result.Failure = &workers.ExecutionFailure{
				Type:    workers.WorkFailureTypeUnknown,
				Family:  workers.WorkFailureFamilyTerminal,
				Message: "execution canceled",
			}
		}
		return result
	}
	if errors.Is(runErr, context.DeadlineExceeded) {
		result.Outcome = workers.ExecutionOutcomeFailed
		var providerErr *workers.ProviderError
		if errors.As(runErr, &providerErr) && providerErr != nil {
			result.Failure = failureFromError(providerErr)
		} else {
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

func normalizeRunnerOutcome(outcome workers.WorkOutcome) workers.ExecutionOutcome {
	switch outcome {
	case workers.OutcomeContinue:
		return workers.ExecutionOutcomeContinue
	case workers.OutcomeRejected:
		return workers.ExecutionOutcomeRejected
	case workers.OutcomeFailed:
		return workers.ExecutionOutcomeFailed
	default:
		return workers.ExecutionOutcomeAccepted
	}
}

// normalizeProviderOverrideResult preserves the Agent runner's output-policy
// decision when a process-scoped Provider override replaces the native Agent
// runner. The override is an effect seam, not a second outcome policy owner.
func normalizeProviderOverrideResult(
	result workers.RunnerExecutionResult,
	request workers.RunnerExecutionRequest,
) workers.RunnerExecutionResult {
	if result.Outcome != "" || strings.TrimSpace(request.StopToken) == "" {
		return result
	}
	if workers.ContainsStopToken(result.Content, request.StopToken) {
		result.Outcome = workers.OutcomeAccepted
	} else if strings.Contains(result.Content, "<CONTINUE>") {
		result.Outcome = workers.OutcomeContinue
	} else {
		result.Outcome = workers.OutcomeRejected
	}
	return result
}

func proposedOutputFromRunnerResult(result workers.RunnerExecutionResult) workers.ProposedOutput {
	output := proposedOutputFromContent(result.Content)
	output.Feedback = result.Feedback
	output.Classification = result.Classification
	return output
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
		message := normalizedProviderFailureMessage(providerErr)
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

func normalizedProviderFailureMessage(providerErr *workers.ProviderError) string {
	if providerErr == nil {
		return ""
	}
	message := strings.TrimSpace(providerErr.Message)
	if message == "" {
		return strings.TrimSpace(providerErr.Error())
	}
	// Provider-owned normalized failures carry the Providers sentinel through
	// Cause. Preserve the public provider-error prefix for those failures, but
	// leave Workers-owned typed failures (panic, cleanup, and contract errors)
	// at their established customer-facing message.
	if errors.Is(providerErr.Cause, providers.ErrExecuteFailed) {
		return fmt.Sprintf("%s: %s", providerErr.Error(), message)
	}
	return message
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
			// Panic values are outside the trusted request boundary and may
			// contain prompts, credentials, or command input. Keep a stable
			// classification and the bounded code-location stack only.
			Message: "worker runner panicked",
			Stack:   boundedPanicStack([]byte(diagnostics.Panic.Stack)),
		}
	}
	return safe
}
