package workers

// SafeWorkDiagnostics carries the canonical dashboard-safe execution
// diagnostics surface used by history, replay, and projections.
type SafeWorkDiagnostics struct {
	RenderedPrompt *SafeRenderedPromptDiagnostic `json:"rendered_prompt,omitempty"`
	Provider       *SafeProviderDiagnostic       `json:"provider,omitempty"`
	AgentRun       *SafeAgentRunDiagnostic       `json:"agent_run,omitempty"`
	Invocation     *InvocationDiagnostic         `json:"invocation,omitempty"`
}

type SafeRenderedPromptDiagnostic struct {
	SystemPromptHash string            `json:"system_prompt_hash,omitempty"`
	UserMessageHash  string            `json:"user_message_hash,omitempty"`
	Variables        map[string]string `json:"variables,omitempty"`
}

type SafeProviderDiagnostic struct {
	Provider         string            `json:"provider,omitempty"`
	Model            string            `json:"model,omitempty"`
	RequestMetadata  map[string]string `json:"request_metadata,omitempty"`
	ResponseMetadata map[string]string `json:"response_metadata,omitempty"`
}

const (
	AgentRunExecutionBehavior = "agent_run"

	AgentRunMetadataExecutionBehavior = "execution_behavior"
	AgentRunMetadataFailureClass      = "failure_class"
	AgentRunMetadataRecoveryAction    = "recovery_action"
	AgentRunMetadataToolPolicy        = "tool_policy"
	AgentRunMetadataToolCallCount     = "tool_call_count"
	AgentRunMetadataToolDiagnostics   = "tool_diagnostics"
)

type SafeAgentRunDiagnostic struct {
	ExecutionBehavior string                    `json:"execution_behavior,omitempty"`
	FailureClass      string                    `json:"failure_class,omitempty"`
	RecoveryAction    string                    `json:"recovery_action,omitempty"`
	ToolPolicy        string                    `json:"tool_policy,omitempty"`
	ToolCallCount     int                       `json:"tool_call_count,omitempty"`
	ToolDiagnostics   []AgentRunToolDiagnostic  `json:"tool_diagnostics,omitempty"`
	Transcript        []AgentRunTranscriptEntry `json:"transcript,omitempty"`
}

type AgentRunToolDiagnostic struct {
	ToolName string `json:"tool_name,omitempty"`
	Phase    string `json:"phase,omitempty"`
	Detail   string `json:"detail,omitempty"`
}

type AgentRunTranscriptEntry struct {
	Role    string `json:"role,omitempty"`
	Summary string `json:"summary,omitempty"`
}
