package service

import (
	"context"
	"strings"

	factorydefinitions "github.com/portpowered/infinite-you/pkg/services/factory_definitions"
	"github.com/portpowered/infinite-you/pkg/services/workers"
)

// agentLoopProviderOverrideRunner keeps the process-scoped provider override
// on the same output-policy path as the registered Agent runner. The override
// is an effect seam only; Factory Definitions remains the decision-envelope
// owner for the structured result.
type agentLoopProviderOverrideRunner struct {
	runner            workers.Runner
	decisionEnvelopes factorydefinitions.DecisionEnvelopeService
}

func (runner agentLoopProviderOverrideRunner) Execute(
	ctx context.Context,
	request workers.RunnerExecutionRequest,
) (workers.RunnerExecutionResult, error) {
	result, err := runner.runner.Execute(ctx, request)
	if err != nil {
		return result, err
	}
	if !usesDecisionEnvelopeRequest(request) {
		return normalizeProviderOverrideResult(result, request), nil
	}
	if runner.decisionEnvelopes == nil {
		return result, errMisconfigured(
			"agent decision-envelope service is required for decision-envelope output",
		)
	}
	return normalizeDetachedDecisionEnvelope(result, request, runner.decisionEnvelopes)
}

func usesDecisionEnvelopeRequest(request workers.RunnerExecutionRequest) bool {
	return request.DecisionEnvelope || strings.EqualFold(
		strings.TrimSpace(request.OutputFormat),
		factorydefinitions.DecisionEnvelopeOutcomeFormat,
	)
}

func normalizeDetachedDecisionEnvelope(
	result workers.RunnerExecutionResult,
	request workers.RunnerExecutionRequest,
	decisionEnvelopes factorydefinitions.DecisionEnvelopeService,
) (workers.RunnerExecutionResult, error) {
	parsed := decisionEnvelopeWorkResult(decisionEnvelopes, request, result.Content)
	result.Outcome = parsed.Outcome
	result.Feedback = strings.TrimSpace(parsed.Feedback)
	result.Classification = parsed.SelectedClassificationLabel
	result.RecordedOutputWork = parsed.RecordedOutputWork
	result.Diagnostics = mergeRunnerDecisionEnvelopeDiagnostics(result.Diagnostics, parsed.Diagnostics)
	if strings.TrimSpace(parsed.Error) != "" {
		failureType := workers.WorkFailureTypeUnknown
		if parsed.FailureMetadata != nil && strings.TrimSpace(string(parsed.FailureMetadata.Type)) != "" {
			failureType = parsed.FailureMetadata.Type
		}
		failure := workers.NewProviderError(
			failureType,
			boundedDetachedDecisionEnvelopeMessage(parsed.Error),
			nil,
		)
		failure.Continuation = workers.CloneContinuationReference(result.Continuation)
		failure.Diagnostics = workers.CloneWorkDiagnostics(result.Diagnostics)
		return result, failure
	}
	result.Content = strings.TrimSpace(parsed.Output)
	return result, nil
}

func decisionEnvelopeWorkResult(
	decisionEnvelopes factorydefinitions.DecisionEnvelopeService,
	request workers.RunnerExecutionRequest,
	raw string,
) workers.WorkResult {
	if request.GoalRoutingDecisionEnvelope {
		return decisionEnvelopes.WorkResultFromGoalRoutingDecisionEnvelopeJSONOrFailed(
			request.Dispatch.DispatchID,
			request.Dispatch.TransitionID,
			raw,
		)
	}
	return decisionEnvelopes.WorkResultFromDecisionEnvelopeJSONOrFailed(
		request.Dispatch.DispatchID,
		request.Dispatch.TransitionID,
		raw,
	)
}

func mergeRunnerDecisionEnvelopeDiagnostics(
	base *workers.WorkDiagnostics,
	envelope *workers.WorkDiagnostics,
) *workers.WorkDiagnostics {
	if envelope == nil {
		return base
	}
	merged := workers.CloneWorkDiagnostics(base)
	if merged == nil {
		return workers.CloneWorkDiagnostics(envelope)
	}
	overlay := workers.CloneWorkDiagnostics(envelope)
	if overlay.Provider != nil {
		if merged.Provider == nil {
			merged.Provider = overlay.Provider
		} else {
			if merged.Provider.ResponseMetadata == nil {
				merged.Provider.ResponseMetadata = make(map[string]string, len(overlay.Provider.ResponseMetadata))
			}
			for key, value := range overlay.Provider.ResponseMetadata {
				merged.Provider.ResponseMetadata[key] = value
			}
		}
	}
	if overlay.Metadata != nil {
		if merged.Metadata == nil {
			merged.Metadata = make(map[string]string, len(overlay.Metadata))
		}
		for key, value := range overlay.Metadata {
			merged.Metadata[key] = value
		}
	}
	return merged
}

func boundedDetachedDecisionEnvelopeMessage(message string) string {
	message = strings.TrimSpace(message)
	const maxRunes = 512
	runes := []rune(message)
	if len(runes) <= maxRunes {
		return message
	}
	return string(runes[:maxRunes])
}
