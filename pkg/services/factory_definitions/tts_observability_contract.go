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
