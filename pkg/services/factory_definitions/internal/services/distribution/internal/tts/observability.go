package tts

import (
	factorydefinitions "github.com/portpowered/infinite-you/pkg/services/factory_definitions"
	invocationpolicywire "github.com/portpowered/infinite-you/pkg/services/factory_definitions/internal/services/invocation_policy/wire"
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
	IsPackagedFactory      = invocationpolicywire.IsPackagedTTSFactory
	BackendRuntimeLabel    = invocationpolicywire.TTSBackendRuntimeLabel
	ClassifyInvocationWait = invocationpolicywire.ClassifyTTSInvocationWait
)

func isModelNotReadyFailure(message string) bool {
	return invocationpolicywire.IsTTSModelNotReadyFailure(message)
}
