package service

import (
	"strings"

	"github.com/portpowered/infinite-you/pkg/services/workers"
)

// Successful opt-in scripts use the injected contract owner once. Process
// errors bypass parsing; a declared FAILED keeps its output and exact reason.
func (s *Service) normalizeScriptDecisionEnvelope(
	result workers.RunnerExecutionResult,
	request workers.RunnerExecutionRequest,
) (workers.RunnerExecutionResult, error) {
	if s.decisionEnvelopes == nil {
		return result, errMisconfigured("script decision-envelope service is required for decision-envelope output")
	}
	parsed := decisionEnvelopeWorkResult(s.decisionEnvelopes, request, result.Content)
	result.Outcome = parsed.Outcome
	result.Feedback = strings.TrimSpace(parsed.Feedback)
	result.Classification = parsed.SelectedClassificationLabel
	result.RecordedOutputWork = parsed.RecordedOutputWork
	result.Diagnostics = mergeRunnerDecisionEnvelopeDiagnostics(result.Diagnostics, parsed.Diagnostics)
	result.Content = strings.TrimSpace(parsed.Output)
	if parsed.Outcome == workers.OutcomeFailed && strings.TrimSpace(parsed.Error) == "" {
		parsed.Error = result.Feedback
		if parsed.Error == "" {
			parsed.Error = "script declared FAILED"
		}
	}
	if strings.TrimSpace(parsed.Error) == "" {
		return result, nil
	}
	failureType := workers.WorkFailureTypeUnknown
	if parsed.FailureMetadata != nil && strings.TrimSpace(string(parsed.FailureMetadata.Type)) != "" {
		failureType = parsed.FailureMetadata.Type
	}
	failure := workers.NewProviderError(failureType, boundedDetachedDecisionEnvelopeMessage(parsed.Error), nil)
	failure.Diagnostics = workers.CloneWorkDiagnostics(result.Diagnostics)
	return result, failure
}
