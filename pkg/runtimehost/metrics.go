package runtimehost

// Runtime metric names emitted by the transport runtime host.
const (
	RuntimeMetricLifecycleStarted               = "runtime.lifecycle.started"
	RuntimeMetricLifecycleStopped               = "runtime.lifecycle.stopped"
	RuntimeMetricStateActive                    = "runtime.state.active"
	RuntimeMetricStateIdle                      = "runtime.state.idle"
	RuntimeMetricStatePaused                    = "runtime.state.paused"
	RuntimeMetricStateFailed                    = "runtime.state.failed"
	RuntimeMetricQueueInFlight                  = "runtime.queue.in_flight"
	RuntimeMetricQueueSubmissionCount           = "queue.submission_count"
	RuntimeMetricDispatchStarted                = "dispatch.started"
	RuntimeMetricDispatchComplete               = "dispatch.completed"
	RuntimeMetricDispatchDuration               = "dispatch.duration"
	RuntimeMetricDispatchRetries                = "dispatch.retry_count"
	RuntimeMetricDispatchCost                   = "dispatch.cost"
	RuntimeMetricProviderRequest                = "provider.requested"
	RuntimeMetricProviderComplete               = "provider.completed"
	RuntimeMetricProviderFailed                 = "provider.failed"
	RuntimeMetricProviderDuration               = "provider.duration"
	RuntimeMetricProviderInputTok               = "provider.input_tokens"
	RuntimeMetricProviderOutputTok              = "provider.output_tokens"
	RuntimeMetricProviderCost                   = "provider.cost"
	RuntimeMetricScriptStarted                  = "script.started"
	RuntimeMetricScriptComplete                 = "script.completed"
	RuntimeMetricScriptDuration                 = "script.duration"
	RuntimeMetricScriptTimedOut                 = "script.timed_out"
	RuntimeMetricScriptFailed                   = "script.failed"
	RuntimeMetricSessionResponseStreamPublished = "session_response_stream.published"
	RuntimeMetricSessionResponseStreamCompacted = "session_response_stream.compacted"
	RuntimeMetricSessionResponseStreamDegraded  = "session_response_stream.degraded"
	RuntimeMetricLifecycleControl               = "runtime.lifecycle_control"
)

const (
	runtimeMetricLifecycleStarted               = RuntimeMetricLifecycleStarted
	runtimeMetricLifecycleStopped               = RuntimeMetricLifecycleStopped
	runtimeMetricStateActive                    = RuntimeMetricStateActive
	runtimeMetricStateIdle                      = RuntimeMetricStateIdle
	runtimeMetricStatePaused                    = RuntimeMetricStatePaused
	runtimeMetricStateFailed                    = RuntimeMetricStateFailed
	runtimeMetricQueueInFlight                  = RuntimeMetricQueueInFlight
	runtimeMetricQueueSubmissionCount           = RuntimeMetricQueueSubmissionCount
	runtimeMetricDispatchStarted                = RuntimeMetricDispatchStarted
	runtimeMetricDispatchComplete               = RuntimeMetricDispatchComplete
	runtimeMetricDispatchDuration               = RuntimeMetricDispatchDuration
	runtimeMetricDispatchRetries                = RuntimeMetricDispatchRetries
	runtimeMetricDispatchCost                   = RuntimeMetricDispatchCost
	runtimeMetricProviderRequest                = RuntimeMetricProviderRequest
	runtimeMetricProviderComplete               = RuntimeMetricProviderComplete
	runtimeMetricProviderFailed                 = RuntimeMetricProviderFailed
	runtimeMetricProviderDuration               = RuntimeMetricProviderDuration
	runtimeMetricProviderInputTok               = RuntimeMetricProviderInputTok
	runtimeMetricProviderOutputTok              = RuntimeMetricProviderOutputTok
	runtimeMetricProviderCost                   = RuntimeMetricProviderCost
	runtimeMetricScriptStarted                  = RuntimeMetricScriptStarted
	runtimeMetricScriptComplete                 = RuntimeMetricScriptComplete
	runtimeMetricScriptDuration                 = RuntimeMetricScriptDuration
	runtimeMetricScriptTimedOut                 = RuntimeMetricScriptTimedOut
	runtimeMetricScriptFailed                   = RuntimeMetricScriptFailed
	runtimeMetricSessionResponseStreamPublished = RuntimeMetricSessionResponseStreamPublished
	runtimeMetricSessionResponseStreamCompacted = RuntimeMetricSessionResponseStreamCompacted
	runtimeMetricSessionResponseStreamDegraded  = RuntimeMetricSessionResponseStreamDegraded
	runtimeMetricLifecycleControl               = RuntimeMetricLifecycleControl
)

const (
	ModelPullMetricAttempts      = "managed_runtime.pull.attempts"
	ModelPullMetricSuccess       = "managed_runtime.pull.success"
	ModelPullMetricFailure       = "managed_runtime.pull.failure"
	ModelPullMetricSourceFailure = "managed_runtime.pull.source_failure"
)

const (
	InvocationMetricNormalizationAttempts = "invocation.normalization_attempts"
	InvocationMetricNormalizationSuccess  = "invocation.normalization_success"
	InvocationMetricNormalizationFailure  = "invocation.normalization_failure"
	InvocationMetricInterpolationFailure  = "invocation.interpolation_failure"
	InvocationMetricAttempts              = "invocation.attempts"
	InvocationMetricSuccess               = "invocation.success"
	InvocationMetricFailure               = "invocation.failure"
	InvocationMetricUnresolvedPrimary     = "invocation.unresolved_primary"
	InvocationMetricFallbackPolicyUsed    = "invocation.fallback_policy_used"
	InvocationMetricResultType            = "invocation.result_type"
)

const (
	modelPullMetricAttempts      = ModelPullMetricAttempts
	modelPullMetricSuccess       = ModelPullMetricSuccess
	modelPullMetricFailure       = ModelPullMetricFailure
	modelPullMetricSourceFailure = ModelPullMetricSourceFailure
)

const (
	invocationMetricNormalizationAttempts = InvocationMetricNormalizationAttempts
	invocationMetricNormalizationSuccess  = InvocationMetricNormalizationSuccess
	invocationMetricNormalizationFailure  = InvocationMetricNormalizationFailure
	invocationMetricInterpolationFailure  = InvocationMetricInterpolationFailure
	invocationMetricAttempts              = InvocationMetricAttempts
	invocationMetricSuccess               = InvocationMetricSuccess
	invocationMetricFailure               = InvocationMetricFailure
	invocationMetricUnresolvedPrimary     = InvocationMetricUnresolvedPrimary
	invocationMetricFallbackPolicyUsed    = InvocationMetricFallbackPolicyUsed
	invocationMetricResultType            = InvocationMetricResultType
)
