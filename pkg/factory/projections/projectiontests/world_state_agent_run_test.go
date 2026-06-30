package projections_test

import (
	"testing"
	"time"

	factoryapi "github.com/portpowered/infinite-you/pkg/api/generated"
	. "github.com/portpowered/infinite-you/pkg/factory/projections"
	"github.com/portpowered/infinite-you/pkg/interfaces"
)

func TestReconstructFactoryWorldState_PreservesAgentRunInspectionDiagnostics(t *testing.T) {
	t0 := time.Date(2026, 6, 30, 12, 0, 0, 0, time.UTC)
	agentRunDiagnostics := generatedWorkDiagnosticsForProjectionTest(&interfaces.SafeWorkDiagnostics{
		AgentRun: &interfaces.SafeAgentRunDiagnostic{
			ExecutionBehavior: interfaces.AgentRunExecutionBehavior,
			ToolPolicy:        interfaces.AgentWorkerToolPolicyEnabled,
			ToolCallCount:     1,
			ToolDiagnostics: []interfaces.AgentRunToolDiagnostic{
				{ToolName: "read_file", Phase: "success", Detail: "bytes=12"},
			},
			Transcript: []interfaces.AgentRunTranscriptEntry{
				{Role: "assistant", Summary: "done"},
			},
		},
	})
	events := []factoryapi.FactoryEvent{
		initialStructureEvent(t0),
		workInputEventWithToken(1, t0.Add(time.Second), "tok-story-1", interfaces.FactoryWorkItem{
			ID:         "work-1",
			WorkTypeID: "story",
			TraceID:    "trace-1",
			PlaceID:    "story:init",
		}),
		workstationRequestEvent(2, t0.Add(2*time.Second), interfaces.WorkstationRequestPayload{
			DispatchID:   "dispatch-agent-1",
			TransitionID: "execute-story",
			Workstation:  interfaces.FactoryWorkstationRef{ID: "execute-story", Name: "Execute story"},
			Inputs: []interfaces.WorkstationInput{{
				TokenID:  "tok-story-1",
				PlaceID:  "story:init",
				WorkItem: &interfaces.FactoryWorkItem{ID: "work-1", WorkTypeID: "story", TraceID: "trace-1", PlaceID: "story:init"},
			}},
		}),
		agentRunResponseEvent(3, t0.Add(3*time.Second), factoryapi.AgentRunResponseEventPayload{
			AgentRunId:     "dispatch-agent-1/agent-run/1",
			Outcome:        factoryapi.WorkOutcomeAccepted,
			DurationMillis: 1200,
			Diagnostics:    agentRunDiagnostics,
		}),
		workstationResponseEvent(4, t0.Add(4*time.Second), interfaces.WorkstationResponsePayload{
			DispatchID:   "dispatch-agent-1",
			TransitionID: "execute-story",
			Result: interfaces.WorkstationResult{
				Outcome: string(interfaces.OutcomeAccepted),
				Output:  "done",
			},
			DurationMillis: 1200,
		}),
	}

	state, err := ReconstructFactoryWorldState(events, 4)
	if err != nil {
		t.Fatalf("ReconstructFactoryWorldState() error = %v", err)
	}
	if len(state.CompletedDispatches) != 1 {
		t.Fatalf("completed dispatches = %d, want 1", len(state.CompletedDispatches))
	}
	diagnostics := state.CompletedDispatches[0].Diagnostics
	if diagnostics == nil || diagnostics.AgentRun == nil {
		t.Fatalf("dispatch diagnostics = %#v, want agentRun inspection", diagnostics)
	}
	if diagnostics.AgentRun.ToolPolicy != interfaces.AgentWorkerToolPolicyEnabled {
		t.Fatalf("tool policy = %q, want ENABLED", diagnostics.AgentRun.ToolPolicy)
	}
	if len(diagnostics.AgentRun.Transcript) != 1 || diagnostics.AgentRun.Transcript[0].Summary != "done" {
		t.Fatalf("transcript = %#v, want assistant summary", diagnostics.AgentRun.Transcript)
	}
}

func agentRunResponseEvent(tick int, eventTime time.Time, payload factoryapi.AgentRunResponseEventPayload) factoryapi.FactoryEvent {
	context := factoryapi.FactoryEventContext{
		DispatchId: stringPtrForProjectionTest("dispatch-agent-1"),
	}
	return generatedProjectionEvent(
		factoryapi.FactoryEventTypeAgentRunResponse,
		"agent-run-response/"+payload.AgentRunId,
		tick,
		eventTime,
		context,
		payload,
	)
}
