package service

import (
	"context"
	"errors"
	"strings"
	"time"

	"github.com/portpowered/infinite-you/pkg/services/work"
	"github.com/portpowered/infinite-you/pkg/services/workers"
	"github.com/portpowered/infinite-you/pkg/services/workers/internal/services/runners"
	workerexecutor "github.com/portpowered/infinite-you/pkg/services/workers/internal/services/workstations/executor"
)

func (s *Service) normalizeResult(
	correlation workers.ExecutionCorrelation,
	request workers.ExecuteRequest,
	runnerResult workers.RunnerExecutionResult,
	runErr error,
	duration time.Duration,
	executionCanceled bool,
) workers.ExecuteResult {
	result := baseExecuteResult(correlation, request, runnerResult, duration)
	if runErr == nil {
		result, runErr = s.normalizeSuccessfulResult(result, request, runnerResult)
	}
	if runErr == nil {
		return result
	}
	return normalizeFailedResult(result, request, runnerResult, runErr, executionCanceled)
}

func baseExecuteResult(
	correlation workers.ExecutionCorrelation,
	request workers.ExecuteRequest,
	runnerResult workers.RunnerExecutionResult,
	duration time.Duration,
) workers.ExecuteResult {
	return workers.ExecuteResult{
		Correlation:  correlation,
		Metrics:      workers.ExecutionMetrics{Duration: duration},
		Diagnostics:  safeDiagnosticsFromWork(runnerResult.Diagnostics),
		Continuation: continuationFromSession(runnerResult.Continuation, request.Input.Resume),
	}
}

func (s *Service) normalizeSuccessfulResult(
	result workers.ExecuteResult,
	request workers.ExecuteRequest,
	runnerResult workers.RunnerExecutionResult,
) (workers.ExecuteResult, error) {
	result.Outcome = normalizeRunnerOutcome(runnerResult.Outcome)
	result.Output = proposedOutputFromRunnerResult(runnerResult)
	switch result.Outcome {
	case workers.ExecutionOutcomeFailed:
		return result, errors.New("runner returned failed outcome")
	case workers.ExecutionOutcomeRejected:
		if s != nil && providerOverrideApplies(&request, s.providerOverride) &&
			resolveRunnerIdentity(request.Target) == runners.AgentIdentity &&
			!usesACPProvider(request.Target.ExecutorProvider) &&
			!hasProviderCompletionEvidence(runnerResult) {
			message, _ := workerexecutor.CompletionValidationFailure(runnerResult)
			return result, workers.NewProviderError(workers.WorkFailureTypeUnknown, message, nil)
		}
	case workers.ExecutionOutcomeAccepted:
		if schema := strings.TrimSpace(request.Target.Prompt.OutputSchema); schema != "" {
			structured, err := workerexecutor.ParseOutputAgainstSchema(runnerResult.Content, schema)
			if err != nil {
				return result, workers.NewProviderError(
					workers.WorkFailureTypeStructuredOutputSchemaViolation,
					"structured output schema violation: "+err.Error(),
					nil,
				)
			}
			result.StructuredResult = structured
			result.StructuredResultPresent = true
		}
		if err := workerexecutor.ValidateOutputContract(runnerResult.Content, request.Target.Output.Contract); err != nil {
			return result, workers.NewProviderError(
				workers.WorkFailureTypePermanentBadRequest,
				"output contract failed",
				err,
			)
		}
	}
	return result, nil
}

func usesACPProvider(executorProvider string) bool {
	return strings.EqualFold(strings.TrimSpace(executorProvider), workers.ExecutorProviderACP)
}

func hasProviderCompletionEvidence(result workers.RunnerExecutionResult) bool {
	if strings.TrimSpace(result.Content) == "" {
		return false
	}
	values := make([]string, 0, 2)
	if result.Diagnostics != nil {
		values = append(values, result.Diagnostics.Metadata[workers.ProviderResponseMetadataCompletionEvidence])
		if result.Diagnostics.Provider != nil {
			values = append(values, result.Diagnostics.Provider.ResponseMetadata[workers.ProviderResponseMetadataCompletionEvidence])
		}
	}
	for _, value := range values {
		switch strings.ToLower(strings.TrimSpace(value)) {
		case "agent_message", "provider_response":
			return true
		}
	}
	return false
}

func normalizeFailedResult(
	result workers.ExecuteResult,
	request workers.ExecuteRequest,
	runnerResult workers.RunnerExecutionResult,
	runErr error,
	executionCanceled bool,
) workers.ExecuteResult {
	switch {
	case executionCanceled:
		return canceledResult(result, request, runErr)
	case errors.Is(runErr, context.DeadlineExceeded):
		return timeoutResult(result, runErr)
	default:
		result = genericFailureResult(result, request, runnerResult, runErr)
		if request.Input.MockWorkers != nil {
			result = prefixMockProviderFailure(result, runErr)
		}
		return normalizeScriptFailure(result, request)
	}
}

func prefixMockProviderFailure(
	result workers.ExecuteResult,
	runErr error,
) workers.ExecuteResult {
	if result.Failure == nil || strings.TrimSpace(result.Failure.Message) == "" {
		return result
	}
	var providerErr *workers.ProviderError
	if !errors.As(runErr, &providerErr) || providerErr == nil {
		return result
	}
	prefix := strings.TrimSpace(providerErr.Error()) + ": "
	if strings.HasPrefix(result.Failure.Message, prefix) {
		return result
	}
	result.Failure.Message = prefix + result.Failure.Message
	if result.Failure.Detail != nil {
		result.Failure.Detail.Message = result.Failure.Message
	}
	return result
}

// normalizeScriptFailure preserves the established workstation boundary for
// ordinary script process failures. A non-zero exit or command-start failure
// is terminal Work, while timeout and missing-executable failures retain their
// explicit metadata for the existing retry/diagnostic policy.
func normalizeScriptFailure(
	result workers.ExecuteResult,
	request workers.ExecuteRequest,
) workers.ExecuteResult {
	if request.Target.RunnerID != runners.ScriptIdentity || result.Failure == nil {
		return result
	}
	switch result.Failure.Type {
	case workers.WorkFailureTypeTimeout, workers.WorkFailureTypeMissingExecutable:
		return result
	default:
		result.Failure.Family = workers.WorkFailureFamilyTerminal
		result.Failure.RetryHint = false
		return result
	}
}

func canceledResult(
	result workers.ExecuteResult,
	request workers.ExecuteRequest,
	runErr error,
) workers.ExecuteResult {
	result.Outcome = workers.ExecutionOutcomeCanceled
	var providerErr *workers.ProviderError
	if errors.As(runErr, &providerErr) && providerErr != nil {
		result.Failure = failureFromError(providerErr)
		return result
	}
	message := "execution canceled"
	if request.Target.RunnerID == "script" {
		message = "execution cancelled: context canceled"
	}
	result.Failure = &workers.ExecutionFailure{
		Type: workers.WorkFailureTypeUnknown, Family: workers.WorkFailureFamilyTerminal, Message: message,
	}
	return result
}

func timeoutResult(result workers.ExecuteResult, runErr error) workers.ExecuteResult {
	result.Outcome = workers.ExecutionOutcomeFailed
	var providerErr *workers.ProviderError
	if errors.As(runErr, &providerErr) && providerErr != nil {
		result.Failure = failureFromError(providerErr)
		return result
	}
	result.Failure = &workers.ExecutionFailure{
		Type: workers.WorkFailureTypeTimeout, Family: workers.WorkFailureFamilyRetryable,
		Message: "execution timed out", RetryHint: true,
		Detail: &workers.FailureDetail{Reason: workers.WorkFailureTypeTimeout, Message: "execution timed out"},
	}
	return result
}

func genericFailureResult(
	result workers.ExecuteResult,
	request workers.ExecuteRequest,
	runnerResult workers.RunnerExecutionResult,
	runErr error,
) workers.ExecuteResult {
	result.Outcome = workers.ExecutionOutcomeFailed
	if request.Target.RunnerID == runners.ScriptIdentity && errors.Is(runErr, context.Canceled) {
		result.Failure = &workers.ExecutionFailure{
			Type:    workers.WorkFailureTypeUnknown,
			Family:  workers.WorkFailureFamilyTerminal,
			Message: "execution cancelled: context canceled",
		}
		return result
	}
	result.Failure = failureFromError(runErr)
	var providerErr *workers.ProviderError
	if errors.As(runErr, &providerErr) && providerErr != nil {
		if result.Diagnostics == nil {
			result.Diagnostics = safeDiagnosticsFromWork(providerErr.Diagnostics)
		}
		if result.Continuation == nil {
			result.Continuation = continuationFromSession(providerErr.Continuation, request.Input.Resume)
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
	output := workers.ProposedOutput{}
	if result.ProposedOutput != nil {
		output = result.ProposedOutput.Clone()
	} else {
		output = proposedOutputFromContent(result.Content)
	}
	output.Feedback = result.Feedback
	output.Classification = result.Classification
	// Decision-envelope reviewers record work on the envelope. The detached
	// path must propose it just as the workstation executor did, so Runtime
	// keeps validating and materializing those items.
	if len(result.RecordedOutputWork) > 0 {
		output.ProposedWork = workers.ProposedWorkFromFactoryWorkItems(result.RecordedOutputWork)
	}
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
			Type:                            failureType,
			Family:                          family,
			Message:                         message,
			RetryHint:                       decision.Retryable,
			ProviderFailureKind:             providerErr.ProviderFailureKind,
			ProviderContinuationFailureKind: providerErr.ProviderContinuationFailureKind,
			ProviderContinuationOutcome:     providerErr.ProviderContinuationOutcome,
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
	var panicErr *workers.WorkerExecutorPanicError
	if errors.As(providerErr.Cause, &panicErr) && panicErr != nil {
		return panicErr.Error()
	}
	return message
}

func continuationFromSession(
	continuation *workers.ProviderContinuationRef,
	resume *workers.ProviderContinuationRef,
) *workers.ProviderContinuationRef {
	if continuation != nil {
		return (continuation).ClonePtr()
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
