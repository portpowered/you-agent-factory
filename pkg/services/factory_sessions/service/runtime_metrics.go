// Session metrics are domain contracts consumed by composition adapters.
package service

import (
	factorymetrics "github.com/portpowered/infinite-you/pkg/services/factory_runtime"
	sessioninvocation "github.com/portpowered/infinite-you/pkg/services/factory_sessions/invocation"
)

// Runtime metric names emitted by the transport runtime host.
const (
	RuntimeMetricLifecycleStarted               = factorymetrics.RuntimeLifecycleStarted
	RuntimeMetricLifecycleStopped               = factorymetrics.RuntimeLifecycleStopped
	RuntimeMetricStateActive                    = factorymetrics.RuntimeStateActive
	RuntimeMetricStateIdle                      = factorymetrics.RuntimeStateIdle
	RuntimeMetricStatePaused                    = factorymetrics.RuntimeStatePaused
	RuntimeMetricStateFailed                    = factorymetrics.RuntimeStateFailed
	RuntimeMetricQueueInFlight                  = factorymetrics.RuntimeQueueInFlight
	RuntimeMetricQueueSubmissionCount           = factorymetrics.RuntimeQueueSubmissionCount
	RuntimeMetricDispatchStarted                = factorymetrics.RuntimeDispatchStarted
	RuntimeMetricDispatchComplete               = factorymetrics.RuntimeDispatchComplete
	RuntimeMetricDispatchDuration               = factorymetrics.RuntimeDispatchDuration
	RuntimeMetricDispatchRetries                = factorymetrics.RuntimeDispatchRetries
	RuntimeMetricDispatchCost                   = factorymetrics.RuntimeDispatchCost
	RuntimeMetricProviderRequest                = factorymetrics.RuntimeProviderRequest
	RuntimeMetricProviderComplete               = factorymetrics.RuntimeProviderComplete
	RuntimeMetricProviderFailed                 = factorymetrics.RuntimeProviderFailed
	RuntimeMetricProviderDuration               = factorymetrics.RuntimeProviderDuration
	RuntimeMetricProviderInputTok               = factorymetrics.RuntimeProviderInputTokens
	RuntimeMetricProviderOutputTok              = factorymetrics.RuntimeProviderOutputTokens
	RuntimeMetricProviderCost                   = factorymetrics.RuntimeProviderCost
	RuntimeMetricScriptStarted                  = factorymetrics.RuntimeScriptStarted
	RuntimeMetricScriptComplete                 = factorymetrics.RuntimeScriptComplete
	RuntimeMetricScriptDuration                 = factorymetrics.RuntimeScriptDuration
	RuntimeMetricScriptTimedOut                 = factorymetrics.RuntimeScriptTimedOut
	RuntimeMetricScriptFailed                   = factorymetrics.RuntimeScriptFailed
	RuntimeMetricSessionResponseStreamPublished = factorymetrics.RuntimeSessionResponseStreamPublished
	RuntimeMetricSessionResponseStreamCompacted = factorymetrics.RuntimeSessionResponseStreamCompacted
	RuntimeMetricSessionResponseStreamDegraded  = factorymetrics.RuntimeSessionResponseStreamDegraded
	RuntimeMetricLifecycleControl               = factorymetrics.RuntimeLifecycleControl
)

const (
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
	InvocationMetricNormalizationAttempts = sessioninvocation.InvocationMetricNormalizationAttempts
	InvocationMetricNormalizationSuccess  = sessioninvocation.InvocationMetricNormalizationSuccess
	InvocationMetricNormalizationFailure  = sessioninvocation.InvocationMetricNormalizationFailure
	InvocationMetricInterpolationFailure  = sessioninvocation.InvocationMetricInterpolationFailure
	InvocationMetricAttempts              = sessioninvocation.InvocationMetricAttempts
	InvocationMetricSuccess               = sessioninvocation.InvocationMetricSuccess
	InvocationMetricFailure               = sessioninvocation.InvocationMetricFailure
	InvocationMetricUnresolvedPrimary     = sessioninvocation.InvocationMetricUnresolvedPrimary
	InvocationMetricFallbackPolicyUsed    = sessioninvocation.InvocationMetricFallbackPolicyUsed
	InvocationMetricResultType            = sessioninvocation.InvocationMetricResultType
)

const (
	modelPullMetricAttempts      = ModelPullMetricAttempts
	modelPullMetricSuccess       = ModelPullMetricSuccess
	modelPullMetricFailure       = ModelPullMetricFailure
	modelPullMetricSourceFailure = ModelPullMetricSourceFailure
)
