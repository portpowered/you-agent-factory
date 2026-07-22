package factorydefinitions

const (
	// PackagedTTSFactoryName is the canonical named factory identifier for @you/tts.
	PackagedTTSFactoryName = "@you/tts"

	TTSInvocationErrorCodeModelNotReady    = "INVOCATION_TTS_MODEL_NOT_READY"
	TTSInvocationErrorCodeGenerationFailed = "INVOCATION_TTS_GENERATION_FAILED"
	TTSFailureClassModelNotReady           = "model_not_ready"
	TTSFailureClassGenerationFailed        = "generation_failed"
	TTSFailureClassLoading                 = "loading"
	TTSFailureClassSuccess                 = "success"

	TTSMetricPackagedFactoryAttempts = "packaged_factory.invocation.attempts"
	TTSMetricPackagedFactorySuccess  = "packaged_factory.invocation.success"
	TTSMetricPackagedFactoryFailure  = "packaged_factory.invocation.failure"
	TTSMetricPackagedFactoryNotReady = "packaged_factory.invocation.not_ready"
)

// TTSInvocationWaitOutcome classifies one packaged TTS invocation wait observation.
type TTSInvocationWaitOutcome string

const (
	TTSInvocationWaitOutcomeLoading           TTSInvocationWaitOutcome = "loading"
	TTSInvocationWaitOutcomeModelNotReady     TTSInvocationWaitOutcome = "model_not_ready"
	TTSInvocationWaitOutcomeGenerationFailed  TTSInvocationWaitOutcome = "generation_failed"
	TTSInvocationWaitOutcomeUnresolvedFailure TTSInvocationWaitOutcome = "unresolved_failure"
)

// TTSInvocationFailure carries a stable packaged TTS invocation failure surface.
type TTSInvocationFailure struct {
	Outcome      TTSInvocationWaitOutcome
	ErrorCode    string
	FailureClass string
	Message      string
}

// TTSObservabilityService owns packaged TTS identity and failure
// classification policy.
type TTSObservabilityService interface {
	IsPackagedTTSFactory(*FactoryConfig) bool
	TTSBackendRuntimeLabel() string
	ClassifyTTSInvocationWait(FactoryWorldState, string, bool) (TTSInvocationWaitOutcome, *TTSInvocationFailure)
	IsTTSModelNotReadyFailure(string) bool
}
