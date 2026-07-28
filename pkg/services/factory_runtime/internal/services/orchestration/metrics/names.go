package metrics

import factory "github.com/portpowered/infinite-you/pkg/services/factory_runtime"

const (
	RuntimeLifecycleStarted               = factory.RuntimeLifecycleStarted
	RuntimeLifecycleStopped               = factory.RuntimeLifecycleStopped
	RuntimeStateActive                    = factory.RuntimeStateActive
	RuntimeStateIdle                      = factory.RuntimeStateIdle
	RuntimeStatePaused                    = factory.RuntimeStatePaused
	RuntimeStateFailed                    = factory.RuntimeStateFailed
	RuntimeQueueInFlight                  = factory.RuntimeQueueInFlight
	RuntimeQueueSubmissionCount           = factory.RuntimeQueueSubmissionCount
	RuntimeDispatchStarted                = factory.RuntimeDispatchStarted
	RuntimeDispatchComplete               = factory.RuntimeDispatchComplete
	RuntimeDispatchDuration               = factory.RuntimeDispatchDuration
	RuntimeDispatchRetries                = factory.RuntimeDispatchRetries
	RuntimeDispatchCost                   = factory.RuntimeDispatchCost
	RuntimeProviderRequest                = factory.RuntimeProviderRequest
	RuntimeProviderComplete               = factory.RuntimeProviderComplete
	RuntimeProviderFailed                 = factory.RuntimeProviderFailed
	RuntimeProviderDuration               = factory.RuntimeProviderDuration
	RuntimeProviderInputTokens            = factory.RuntimeProviderInputTokens
	RuntimeProviderOutputTokens           = factory.RuntimeProviderOutputTokens
	RuntimeProviderCost                   = factory.RuntimeProviderCost
	RuntimeScriptStarted                  = factory.RuntimeScriptStarted
	RuntimeScriptComplete                 = factory.RuntimeScriptComplete
	RuntimeScriptDuration                 = factory.RuntimeScriptDuration
	RuntimeScriptTimedOut                 = factory.RuntimeScriptTimedOut
	RuntimeScriptFailed                   = factory.RuntimeScriptFailed
	RuntimeSessionResponseStreamPublished = factory.RuntimeSessionResponseStreamPublished
	RuntimeSessionResponseStreamCompacted = factory.RuntimeSessionResponseStreamCompacted
	RuntimeSessionResponseStreamDegraded  = factory.RuntimeSessionResponseStreamDegraded
	RuntimeLifecycleControl               = factory.RuntimeLifecycleControl
)
