// Package ttsobservability implements packaged TTS observability and
// failure-classification policy owned by nested invocation_policy.
package ttsobservability

import (
	"strings"

	factorydefinitions "github.com/portpowered/infinite-you/pkg/services/factory_definitions"
	invocationpolicycontracts "github.com/portpowered/infinite-you/pkg/services/factory_definitions/internal/services/invocation_policy/contracts"
)

func IsPackagedTTSFactory(cfg *factorydefinitions.FactoryConfig) bool {
	if cfg == nil {
		return false
	}
	if strings.TrimSpace(cfg.Name) == factorydefinitions.PackagedTTSFactoryName {
		return true
	}
	return strings.TrimSpace(cfg.Project) == factorydefinitions.PackagedTTSFactoryProject
}

func TTSBackendRuntimeLabel() string {
	return factorydefinitions.DefaultTTSModelName + "/" + factorydefinitions.DefaultTTSBackendName
}

func ClassifyTTSInvocationWait(
	worldState factorydefinitions.FactoryWorldState,
	requestID string,
	hasActiveWork bool,
) (invocationpolicycontracts.TTSInvocationWaitOutcome, *invocationpolicycontracts.TTSInvocationFailure) {
	if hasActiveWork {
		return invocationpolicycontracts.TTSInvocationWaitOutcomeLoading, nil
	}
	if failure, ok := classifyPackagedTTSFailure(worldState, requestID); ok {
		return failure.Outcome, failure
	}
	return invocationpolicycontracts.TTSInvocationWaitOutcomeUnresolvedFailure, nil
}

func classifyPackagedTTSFailure(
	worldState factorydefinitions.FactoryWorldState,
	requestID string,
) (*invocationpolicycontracts.TTSInvocationFailure, bool) {
	request, ok := worldState.WorkRequestsByID[requestID]
	if !ok || len(request.WorkItems) == 0 {
		return nil, false
	}

	for _, item := range request.WorkItems {
		detail, hasDetail := worldState.FailureDetailsByWorkID[item.ID]
		_, hasFailed := worldState.FailedWorkItemsByID[item.ID]
		if !hasDetail && !hasFailed {
			continue
		}
		if hasDetail && !isPackagedInvokeFailure(detail) {
			continue
		}

		message := failureEvidence(detail, hasDetail)
		if IsTTSModelNotReadyFailure(message) {
			return &invocationpolicycontracts.TTSInvocationFailure{
				Outcome:      invocationpolicycontracts.TTSInvocationWaitOutcomeModelNotReady,
				ErrorCode:    factorydefinitions.TTSInvocationErrorCodeModelNotReady,
				FailureClass: factorydefinitions.TTSFailureClassModelNotReady,
				Message:      boundedFailureSummary(message, "packaged tts model is not ready"),
			}, true
		}
		return &invocationpolicycontracts.TTSInvocationFailure{
			Outcome:      invocationpolicycontracts.TTSInvocationWaitOutcomeGenerationFailed,
			ErrorCode:    factorydefinitions.TTSInvocationErrorCodeGenerationFailed,
			FailureClass: factorydefinitions.TTSFailureClassGenerationFailed,
			Message:      boundedFailureSummary(message, "packaged tts generation failed"),
		}, true
	}
	return nil, false
}

func isPackagedInvokeFailure(detail factorydefinitions.FactoryWorldFailureDetail) bool {
	workstation := strings.TrimSpace(detail.WorkstationName)
	return workstation == "" || workstation == factorydefinitions.PackagedTTSInvokeWorkstationName
}

func failureEvidence(detail factorydefinitions.FactoryWorldFailureDetail, hasDetail bool) string {
	if !hasDetail || detail.FailureDetail == nil {
		return ""
	}
	return strings.TrimSpace(strings.Join([]string{
		string(detail.FailureDetail.Reason),
		detail.FailureDetail.Message,
	}, " "))
}

func IsTTSModelNotReadyFailure(message string) bool {
	trimmed := strings.ToLower(strings.TrimSpace(message))
	return trimmed != "" &&
		(strings.Contains(trimmed, "model not available") ||
			strings.Contains(trimmed, "required assets missing"))
}

func boundedFailureSummary(message, fallback string) string {
	message = strings.Join(strings.Fields(strings.TrimSpace(message)), " ")
	if message == "" {
		return fallback
	}
	const limit = 160
	if len(message) <= limit {
		return message
	}
	return message[:limit] + "..."
}
