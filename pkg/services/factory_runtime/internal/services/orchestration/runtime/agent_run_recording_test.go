package runtime

import (
	"testing"
	"time"

	"github.com/portpowered/infinite-you/internal/testutil/recordingfixtures"
	"github.com/portpowered/infinite-you/internal/testutil/runtimefixtures"
	interfaces "github.com/portpowered/infinite-you/pkg/services/factory_definitions"
	"github.com/portpowered/infinite-you/pkg/services/work"
	"github.com/portpowered/infinite-you/pkg/services/workers"
)

func TestRecordDetachedAgentRunResponsePreservesSafeDiagnosticsAndTranscript(t *testing.T) {
	t.Parallel()

	ledger := &agentRunRecordingLedger{
		ScriptedRuntimeLedger: &recordingfixtures.ScriptedRuntimeLedger{},
	}
	cfg := &runtimeConfig{
		eventHistory: ledger,
		runtimeConfig: runtimefixtures.RuntimeDefinitionLookupFixture{
			Workstations: map[string]*interfaces.FactoryWorkstationConfig{
				"agent": {Name: "agent", Type: interfaces.WorkstationTypeAgent},
			},
			Workers: map[string]*interfaces.FactoryWorkerConfig{},
		},
	}
	request := workers.ExecuteRequest{
		Correlation: workers.ExecutionCorrelation{DispatchID: "dispatch-1"},
		Target: workers.ExecutionTarget{
			WorkstationName: "agent",
			Prompt: workers.PromptPolicy{
				SystemPrompt: "Reasoning effort: low",
				UserMessage:  "run the task",
			},
		},
	}
	result := workers.ExecuteResult{
		Outcome: workers.ExecutionOutcomeAccepted,
		Output: workers.ProposedOutput{Primary: []work.WorkContentPart{{
			Type: work.WorkContentPartTypeText,
			Text: "completed",
		}}},
		Diagnostics: &workers.SafeDiagnostics{Provider: &workers.SafeProviderDiagnostic{
			Provider: "codex",
		}},
		Metrics: workers.ExecutionMetrics{Duration: 1250 * time.Millisecond},
	}

	recordDetachedAgentRunResponse(cfg, request, result, nil)

	event := ledger.event
	if event.ID != "factory-event/agent-run-response/dispatch-1" ||
		event.DispatchID != "dispatch-1" ||
		event.Payload.AgentRunID != "dispatch-1/agent-run/1" ||
		event.Payload.Outcome != string(workers.ExecutionOutcomeAccepted) ||
		event.Payload.DurationMillis != 1250 {
		t.Fatalf("recorded event = %#v, want detached agent-run identity and duration", event)
	}
	diagnostics, err := workers.SafeWorkDiagnosticsFromEventPayload(event.Payload.Diagnostics)
	if err != nil {
		t.Fatalf("decode diagnostics: %v", err)
	}
	if diagnostics.Provider == nil || diagnostics.Provider.Provider != "codex" {
		t.Fatalf("provider diagnostics = %#v, want codex", diagnostics.Provider)
	}
	if diagnostics.AgentRun == nil || len(diagnostics.AgentRun.Transcript) != 3 {
		t.Fatalf("agent-run diagnostics = %#v, want system/user/assistant transcript", diagnostics.AgentRun)
	}
	if diagnostics.AgentRun.Transcript[0].Summary != "Reasoning effort: low" ||
		diagnostics.AgentRun.Transcript[2].Summary != "completed" {
		t.Fatalf("transcript = %#v, want interpolated prompt and output", diagnostics.AgentRun.Transcript)
	}
}

type agentRunRecordingLedger struct {
	*recordingfixtures.ScriptedRuntimeLedger
	event workers.AgentRunResponseEvent
}

func (ledger *agentRunRecordingLedger) RecordAgentRunEvent(event workers.AgentRunResponseEvent) {
	ledger.event = event
}
