// Package packagedtts adapts the built-in TTS Factory's invocation facts to
// the canonical Factory Session invocation service. The classifier is
// consumer-owned; it does not inject Definitions policy into Sessions.
package packagedtts

import (
	"strings"

	interfaces "github.com/portpowered/infinite-you/pkg/services/factory_definitions"
	sessioninvocation "github.com/portpowered/infinite-you/pkg/services/factory_sessions/internal/invocation"
)

// NewTelemetry supplies the packaged TTS telemetry descriptor.
func NewTelemetry(
	recordMetric func(sessioninvocation.SessionInvocationMetric),
	recordLog func(sessioninvocation.SessionInvocationLogRecord),
) sessioninvocation.SessionInvocationTelemetry {
	return sessioninvocation.NewSessionInvocationTelemetry(
		recordMetric,
		recordLog,
		&sessioninvocation.PackagedInvocationTelemetry{
			Active:         isPackagedTTSFactory,
			FactoryName:    interfaces.PackagedTTSFactoryName,
			Backend:        interfaces.DefaultTTSModelName + "/" + interfaces.DefaultTTSBackendName,
			AttemptsMetric: interfaces.TTSMetricPackagedFactoryAttempts,
			SuccessMetric:  interfaces.TTSMetricPackagedFactorySuccess,
			FailureMetric:  interfaces.TTSMetricPackagedFactoryFailure,
			NotReadyMetric: interfaces.TTSMetricPackagedFactoryNotReady,
			LoadingClass:   interfaces.TTSFailureClassLoading,
			SuccessClass:   interfaces.TTSFailureClassSuccess,
			NotReadyClass:  interfaces.TTSFailureClassModelNotReady,
		},
	)
}

// SpecialCase supplies packaged TTS terminal classification.
type SpecialCase struct{}

func NewSpecialCase() SpecialCase { return SpecialCase{} }

func (SpecialCase) Active(cfg *interfaces.FactoryConfig) bool {
	return isPackagedTTSFactory(cfg)
}

func (SpecialCase) TerminalFailure(
	worldState interfaces.FactoryWorldState,
	requestID string,
) *sessioninvocation.SessionInvocationSpecialFailure {
	_, failure := classifyInvocationWait(worldState, requestID, false)
	if failure == nil {
		return nil
	}
	return &sessioninvocation.SessionInvocationSpecialFailure{
		ErrorCode: failure.ErrorCode, Message: failure.Message, FailureClass: failure.FailureClass,
	}
}

type invocationFailure struct {
	ErrorCode    string
	FailureClass string
	Message      string
}

func isPackagedTTSFactory(cfg *interfaces.FactoryConfig) bool {
	if cfg == nil {
		return false
	}
	return strings.TrimSpace(cfg.Name) == interfaces.PackagedTTSFactoryName ||
		strings.TrimSpace(cfg.Project) == interfaces.PackagedTTSFactoryProject
}

func classifyInvocationWait(
	worldState interfaces.FactoryWorldState,
	requestID string,
	hasActiveWork bool,
) (string, *invocationFailure) {
	if hasActiveWork {
		return interfaces.TTSFailureClassLoading, nil
	}
	request, ok := worldState.WorkRequestsByID[requestID]
	if !ok || len(request.WorkItems) == 0 {
		return "", nil
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
		if isTTSModelNotReadyFailure(message) {
			return interfaces.TTSFailureClassModelNotReady, &invocationFailure{
				ErrorCode:    interfaces.TTSInvocationErrorCodeModelNotReady,
				FailureClass: interfaces.TTSFailureClassModelNotReady,
				Message:      boundedFailureSummary(message, "packaged tts model is not ready"),
			}
		}
		return interfaces.TTSFailureClassGenerationFailed, &invocationFailure{
			ErrorCode:    interfaces.TTSInvocationErrorCodeGenerationFailed,
			FailureClass: interfaces.TTSFailureClassGenerationFailed,
			Message:      boundedFailureSummary(message, "packaged tts generation failed"),
		}
	}
	return "", nil
}

func isPackagedInvokeFailure(detail interfaces.FactoryWorldFailureDetail) bool {
	workstation := strings.TrimSpace(detail.WorkstationName)
	return workstation == "" || workstation == interfaces.PackagedTTSInvokeWorkstationName
}

func failureEvidence(detail interfaces.FactoryWorldFailureDetail, hasDetail bool) string {
	if !hasDetail || detail.FailureDetail == nil {
		return ""
	}
	return strings.TrimSpace(strings.Join([]string{
		string(detail.FailureDetail.Reason), detail.FailureDetail.Message,
	}, " "))
}

func isTTSModelNotReadyFailure(message string) bool {
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
