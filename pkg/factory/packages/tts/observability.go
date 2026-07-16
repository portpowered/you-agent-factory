package tts

import (
	"strings"

	interfaces "github.com/portpowered/infinite-you/pkg/factory/contracts"
	workerexecution "github.com/portpowered/infinite-you/pkg/workers/execution"
)

const (
	// PackagedFactoryName is the canonical named factory identifier for @you/tts.
	PackagedFactoryName = "@you/tts"

	InvocationErrorCodeModelNotReady    = "INVOCATION_TTS_MODEL_NOT_READY"
	InvocationErrorCodeGenerationFailed = "INVOCATION_TTS_GENERATION_FAILED"
	FailureClassModelNotReady           = "model_not_ready"
	FailureClassGenerationFailed        = "generation_failed"
	FailureClassLoading                 = "loading"
	FailureClassSuccess                 = "success"

	MetricPackagedFactoryAttempts = "packaged_factory.invocation.attempts"
	MetricPackagedFactorySuccess  = "packaged_factory.invocation.success"
	MetricPackagedFactoryFailure  = "packaged_factory.invocation.failure"
	MetricPackagedFactoryNotReady = "packaged_factory.invocation.not_ready"
)

// InvocationWaitOutcome classifies one packaged TTS invocation wait observation.
type InvocationWaitOutcome string

const (
	InvocationWaitOutcomeLoading           InvocationWaitOutcome = "loading"
	InvocationWaitOutcomeModelNotReady     InvocationWaitOutcome = "model_not_ready"
	InvocationWaitOutcomeGenerationFailed  InvocationWaitOutcome = "generation_failed"
	InvocationWaitOutcomeUnresolvedFailure InvocationWaitOutcome = "unresolved_failure"
)

// InvocationFailure carries a stable packaged TTS invocation failure surface.
type InvocationFailure struct {
	Outcome      InvocationWaitOutcome
	ErrorCode    string
	FailureClass string
	Message      string
}

// IsPackagedFactory reports whether cfg is the built-in @you/tts packaged factory.
func IsPackagedFactory(cfg *interfaces.FactoryConfig) bool {
	if cfg == nil {
		return false
	}
	if strings.TrimSpace(cfg.Name) == PackagedFactoryName {
		return true
	}
	return strings.TrimSpace(cfg.Project) == PackagedFactoryProject
}

// BackendRuntimeLabel returns the default packaged TTS backend/runtime identifier.
func BackendRuntimeLabel() string {
	return defaultBackendLabel()
}

// ClassifyInvocationWait inspects world state during an invocation wait loop.
// When hasActiveWork is true the invocation is still in progress and must not
// emit a success payload.
func ClassifyInvocationWait(
	worldState interfaces.FactoryWorldState,
	requestID string,
	hasActiveWork bool,
) (InvocationWaitOutcome, *InvocationFailure) {
	if hasActiveWork {
		return InvocationWaitOutcomeLoading, nil
	}
	if failure, ok := classifyPackagedTTSFailure(worldState, requestID); ok {
		return failure.Outcome, failure
	}
	return InvocationWaitOutcomeUnresolvedFailure, nil
}

func classifyPackagedTTSFailure(
	worldState interfaces.FactoryWorldState,
	requestID string,
) (*InvocationFailure, bool) {
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
		if isModelNotReadyFailure(message) {
			return &InvocationFailure{
				Outcome:      InvocationWaitOutcomeModelNotReady,
				ErrorCode:    InvocationErrorCodeModelNotReady,
				FailureClass: FailureClassModelNotReady,
				Message:      boundedFailureSummary(message, "packaged tts model is not ready"),
			}, true
		}
		return &InvocationFailure{
			Outcome:      InvocationWaitOutcomeGenerationFailed,
			ErrorCode:    InvocationErrorCodeGenerationFailed,
			FailureClass: FailureClassGenerationFailed,
			Message:      boundedFailureSummary(message, "packaged tts generation failed"),
		}, true
	}
	return nil, false
}

func isPackagedInvokeFailure(detail interfaces.FactoryWorldFailureDetail) bool {
	workstation := strings.TrimSpace(detail.WorkstationName)
	if workstation == "" {
		return true
	}
	return workstation == PackagedInvokeWorkstationName
}

func failureEvidence(detail interfaces.FactoryWorldFailureDetail, hasDetail bool) string {
	if !hasDetail {
		return ""
	}
	return strings.TrimSpace(strings.Join([]string{
		failureDetailReason(detail.FailureDetail),
		failureDetailMessage(detail.FailureDetail),
	}, " "))
}

func failureDetailReason(detail *workerexecution.FailureDetail) string {
	if detail == nil {
		return ""
	}
	return strings.TrimSpace(string(detail.Reason))
}

func failureDetailMessage(detail *workerexecution.FailureDetail) string {
	if detail == nil {
		return ""
	}
	return strings.TrimSpace(detail.Message)
}

func isModelNotReadyFailure(message string) bool {
	trimmed := strings.ToLower(strings.TrimSpace(message))
	if trimmed == "" {
		return false
	}
	return strings.Contains(trimmed, "model not available") ||
		strings.Contains(trimmed, "required assets missing")
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
