package factory

// Canonical Factory Runtime metric names.
const (
	RuntimeLifecycleStarted               = "runtime.lifecycle.started"
	RuntimeLifecycleStopped               = "runtime.lifecycle.stopped"
	RuntimeStateActive                    = "runtime.state.active"
	RuntimeStateIdle                      = "runtime.state.idle"
	RuntimeStatePaused                    = "runtime.state.paused"
	RuntimeStateFailed                    = "runtime.state.failed"
	RuntimeQueueInFlight                  = "runtime.queue.in_flight"
	RuntimeQueueSubmissionCount           = "queue.submission_count"
	RuntimeDispatchStarted                = "dispatch.started"
	RuntimeDispatchComplete               = "dispatch.completed"
	RuntimeDispatchDuration               = "dispatch.duration"
	RuntimeDispatchRetries                = "dispatch.retry_count"
	// RuntimeDispatchCost and RuntimeProviderCost remain recognized names for
	// historical records. The runtime host does not emit them because the
	// Costs service values canonical token rows and no read model consumes
	// independent cost samples.
	RuntimeDispatchCost                   = "dispatch.cost"
	RuntimeProviderRequest                = "provider.requested"
	RuntimeProviderComplete               = "provider.completed"
	RuntimeProviderFailed                 = "provider.failed"
	RuntimeProviderDuration               = "provider.duration"
	RuntimeProviderInputTokens            = "provider.input_tokens"
	RuntimeProviderOutputTokens           = "provider.output_tokens"
	RuntimeProviderCachedInputTokens      = "provider.cached_input_tokens"
	RuntimeProviderReasoningOutputTokens  = "provider.reasoning_output_tokens"
	RuntimeProviderCost                   = "provider.cost"
	RuntimeScriptStarted                  = "script.started"
	RuntimeScriptComplete                 = "script.completed"
	RuntimeScriptDuration                 = "script.duration"
	RuntimeScriptTimedOut                 = "script.timed_out"
	RuntimeScriptFailed                   = "script.failed"
	RuntimeSessionResponseStreamPublished = "session_response_stream.published"
	RuntimeSessionResponseStreamCompacted = "session_response_stream.compacted"
	RuntimeSessionResponseStreamDegraded  = "session_response_stream.degraded"
	RuntimeLifecycleControl               = "runtime.lifecycle_control"
)
