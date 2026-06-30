package interfaces_test

import (
	"testing"

	"github.com/portpowered/infinite-you/pkg/interfaces"
)

func TestSafeAgentRunDiagnosticFromWorkDiagnostics_ProjectsMetadata(t *testing.T) {
	diagnostics := &interfaces.WorkDiagnostics{
		Metadata: map[string]string{
			interfaces.AgentRunMetadataExecutionBehavior: interfaces.AgentRunExecutionBehavior,
			interfaces.AgentRunMetadataFailureClass:      "agent_run_timeout",
			interfaces.AgentRunMetadataRecoveryAction:    "retry later",
			interfaces.AgentRunMetadataToolPolicy:        interfaces.AgentWorkerToolPolicyDisabled,
			interfaces.AgentRunMetadataToolCallCount:     "2",
			interfaces.AgentRunMetadataToolDiagnostics:   "read_file:denied:policy=disabled,write_file:start",
		},
	}

	got := interfaces.SafeAgentRunDiagnosticFromWorkDiagnostics(diagnostics)
	if got == nil || got.ExecutionBehavior != interfaces.AgentRunExecutionBehavior {
		t.Fatalf("SafeAgentRunDiagnosticFromWorkDiagnostics() = %#v, want agent_run behavior", got)
	}
	if got.FailureClass != "agent_run_timeout" || got.RecoveryAction != "retry later" {
		t.Fatalf("failure metadata = %#v, want timeout + retry later", got)
	}
	if got.ToolPolicy != interfaces.AgentWorkerToolPolicyDisabled || got.ToolCallCount != 2 {
		t.Fatalf("tool metadata = %#v, want disabled policy and count 2", got)
	}
	if len(got.ToolDiagnostics) != 2 || got.ToolDiagnostics[0].ToolName != "read_file" {
		t.Fatalf("tool diagnostics = %#v, want parsed entries", got.ToolDiagnostics)
	}
}

func TestGeneratedSafeWorkDiagnostics_IncludesAgentRun(t *testing.T) {
	safe := &interfaces.SafeWorkDiagnostics{
		AgentRun: &interfaces.SafeAgentRunDiagnostic{
			ExecutionBehavior: interfaces.AgentRunExecutionBehavior,
			ToolPolicy:        interfaces.AgentWorkerToolPolicyReadOnly,
			Transcript: []interfaces.AgentRunTranscriptEntry{
				{Role: "assistant", Summary: "final answer"},
			},
		},
	}
	generated := interfaces.GeneratedSafeWorkDiagnostics(safe)
	if generated == nil || generated.AgentRun == nil {
		t.Fatalf("GeneratedSafeWorkDiagnostics() = %#v, want agentRun populated", generated)
	}
	if generated.AgentRun.ToolPolicy == nil || *generated.AgentRun.ToolPolicy != interfaces.AgentWorkerToolPolicyReadOnly {
		t.Fatalf("generated tool policy = %#v, want read_only", generated.AgentRun.ToolPolicy)
	}
}
