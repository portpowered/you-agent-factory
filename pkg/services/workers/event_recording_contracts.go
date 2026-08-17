package workers

// ScriptEventRecorder receives one Worker-owned script execution event.
type ScriptEventRecorder func(ScriptEvent)

// InferenceEventRecorder receives one Worker-owned provider inference event.
type InferenceEventRecorder func(InferenceEvent)

// ModelEventRecorder receives one Worker-owned managed-model event.
type ModelEventRecorder func(ModelEvent)

// AgentRunEventRecorder receives one Worker-owned agent-run event.
type AgentRunEventRecorder func(AgentRunResponseEvent)
