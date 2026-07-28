package tts

import distributiontts "github.com/portpowered/infinite-you/pkg/services/factory_definitions/internal/services/distribution/tts"

const (
	InvocationErrorCodeModelNotReady       = distributiontts.InvocationErrorCodeModelNotReady
	InvocationErrorCodeGenerationFailed    = distributiontts.InvocationErrorCodeGenerationFailed
	FailureClassModelNotReady              = distributiontts.FailureClassModelNotReady
	FailureClassGenerationFailed           = distributiontts.FailureClassGenerationFailed
	FailureClassLoading                    = distributiontts.FailureClassLoading
	FailureClassSuccess                    = distributiontts.FailureClassSuccess
	MetricPackagedFactoryAttempts          = distributiontts.MetricPackagedFactoryAttempts
	MetricPackagedFactorySuccess           = distributiontts.MetricPackagedFactorySuccess
	MetricPackagedFactoryFailure           = distributiontts.MetricPackagedFactoryFailure
	MetricPackagedFactoryNotReady          = distributiontts.MetricPackagedFactoryNotReady
	InvocationWaitOutcomeLoading           = distributiontts.InvocationWaitOutcomeLoading
	InvocationWaitOutcomeModelNotReady     = distributiontts.InvocationWaitOutcomeModelNotReady
	InvocationWaitOutcomeGenerationFailed  = distributiontts.InvocationWaitOutcomeGenerationFailed
	InvocationWaitOutcomeUnresolvedFailure = distributiontts.InvocationWaitOutcomeUnresolvedFailure
)

type InvocationWaitOutcome = distributiontts.InvocationWaitOutcome
type InvocationFailure = distributiontts.InvocationFailure

var (
	IsPackagedFactory      = distributiontts.IsPackagedFactory
	BackendRuntimeLabel    = distributiontts.BackendRuntimeLabel
	ClassifyInvocationWait = distributiontts.ClassifyInvocationWait
)
