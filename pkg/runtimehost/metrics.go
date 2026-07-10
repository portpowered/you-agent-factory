package runtimehost

import "github.com/portpowered/infinite-you/pkg/invocations"

// InvocationMetric records one emitted runtime counter together with its
// low-cardinality dimensions.
type InvocationMetric struct {
	Name   string
	Labels map[string]string
}

// InvocationMetricsRecorder receives invocation counter emissions from CLI and
// session-runtime boundaries. Implementations should treat each call as a
// single counter increment.
type InvocationMetricsRecorder interface {
	RecordInvocationMetric(InvocationMetric)
}

// ModelPullMetricsRecorder receives managed-runtime pull counter emissions.
// Implementations should treat each call as a single counter increment.
type ModelPullMetricsRecorder interface {
	RecordModelPullMetric(InvocationMetric)
}

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
	InvocationMetricNormalizationAttempts = invocations.InvocationMetricNormalizationAttempts
	InvocationMetricNormalizationSuccess  = invocations.InvocationMetricNormalizationSuccess
	InvocationMetricNormalizationFailure  = invocations.InvocationMetricNormalizationFailure
	InvocationMetricInterpolationFailure  = invocations.InvocationMetricInterpolationFailure
	InvocationMetricAttempts              = invocations.InvocationMetricAttempts
	InvocationMetricSuccess               = invocations.InvocationMetricSuccess
	InvocationMetricFailure               = invocations.InvocationMetricFailure
	InvocationMetricUnresolvedPrimary     = invocations.InvocationMetricUnresolvedPrimary
	InvocationMetricFallbackPolicyUsed    = invocations.InvocationMetricFallbackPolicyUsed
	InvocationMetricResultType            = invocations.InvocationMetricResultType
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
