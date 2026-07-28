package tts

import (
	factorydefinitions "github.com/portpowered/infinite-you/pkg/services/factory_definitions"
	"github.com/portpowered/infinite-you/pkg/services/factory_definitions/internal/services/invocation_policy/ttsobservability"
)

const (
	PackagedFactoryName                    = factorydefinitions.PackagedTTSFactoryName
	InvocationErrorCodeModelNotReady       = factorydefinitions.TTSInvocationErrorCodeModelNotReady
	InvocationErrorCodeGenerationFailed    = factorydefinitions.TTSInvocationErrorCodeGenerationFailed
	FailureClassModelNotReady              = factorydefinitions.TTSFailureClassModelNotReady
	FailureClassGenerationFailed           = factorydefinitions.TTSFailureClassGenerationFailed
	FailureClassLoading                    = factorydefinitions.TTSFailureClassLoading
	FailureClassSuccess                    = factorydefinitions.TTSFailureClassSuccess
	MetricPackagedFactoryAttempts          = factorydefinitions.TTSMetricPackagedFactoryAttempts
	MetricPackagedFactorySuccess           = factorydefinitions.TTSMetricPackagedFactorySuccess
	MetricPackagedFactoryFailure           = factorydefinitions.TTSMetricPackagedFactoryFailure
	MetricPackagedFactoryNotReady          = factorydefinitions.TTSMetricPackagedFactoryNotReady
	InvocationWaitOutcomeLoading           = factorydefinitions.TTSInvocationWaitOutcomeLoading
	InvocationWaitOutcomeModelNotReady     = factorydefinitions.TTSInvocationWaitOutcomeModelNotReady
	InvocationWaitOutcomeGenerationFailed  = factorydefinitions.TTSInvocationWaitOutcomeGenerationFailed
	InvocationWaitOutcomeUnresolvedFailure = factorydefinitions.TTSInvocationWaitOutcomeUnresolvedFailure
)

type InvocationWaitOutcome = factorydefinitions.TTSInvocationWaitOutcome
type InvocationFailure = factorydefinitions.TTSInvocationFailure

var (
	IsPackagedFactory      = ttsobservability.IsPackagedTTSFactory
	BackendRuntimeLabel    = ttsobservability.TTSBackendRuntimeLabel
	ClassifyInvocationWait = ttsobservability.ClassifyTTSInvocationWait
)

func isModelNotReadyFailure(message string) bool {
	return ttsobservability.IsTTSModelNotReadyFailure(message)
}
