package subagent

import (
	"testing"

	interfaces "github.com/portpowered/infinite-you/pkg/factory/contracts"
	"github.com/portpowered/infinite-you/pkg/work"
	invocations "github.com/portpowered/infinite-you/pkg/work/invocation"
)

func TestPackagedSubagentInvocationPrimaryResult_ReturnsAgentResponseNotSubmittedInput(t *testing.T) {
	const agentResponse = "mock worker accepted"

	requestID := "req-subagent"
	workID := "work-subagent"
	submitted := work.FactoryWorkItem{
		ID:         workID,
		WorkTypeID: PackagedWorkTypeName,
		State:      "init",
		TraceID:    requestID,
		Content: []work.WorkContentPart{{
			Type: work.WorkContentPartTypeText,
			Text: "customer subagent request text",
		}},
	}
	terminal := work.FactoryWorkItem{
		ID:         workID,
		WorkTypeID: PackagedWorkTypeName,
		State:      "complete",
		TraceID:    requestID,
		PlaceID:    PackagedWorkTypeName + ":complete",
		Content: []work.WorkContentPart{{
			Type: work.WorkContentPartTypeText,
			Text: agentResponse,
		}},
	}

	state := interfaces.FactoryWorldState{
		PayloadLineage:   work.WorkPayloadLineageProjection{},
		WorkRequestsByID: make(map[string]interfaces.WorkRequestPayload),
		TerminalWorkByID: make(map[string]interfaces.FactoryTerminalWork),
	}
	state.WorkRequestsByID[requestID] = interfaces.WorkRequestPayload{
		RequestID: requestID,
		Type:      work.WorkRequestTypeFactoryRequestBatch,
		WorkItems: []work.FactoryWorkItem{submitted},
	}
	state.PayloadLineage.RecordWorkRequestSnapshot(1, requestID, submitted)
	state.PayloadLineage.RecordConsumedInputSnapshot("dispatch-subagent", submitted)
	state.PayloadLineage.RecordDispatchOutputSnapshot(2, "dispatch-subagent", []work.FactoryWorkItem{submitted}, terminal, 0)
	state.TerminalWorkByID[workID] = interfaces.FactoryTerminalWork{WorkItem: terminal, Status: "TERMINAL"}

	selection, err := invocations.ResolvePrimaryResult(invocations.PrimaryResultSelectionInput{
		RequestID:  requestID,
		WorldState: state,
	})
	if err != nil {
		t.Fatalf("ResolvePrimaryResult: %v", err)
	}
	if len(selection.PrimaryResult) != 1 || selection.PrimaryResult[0].Type != work.WorkContentPartTypeText {
		t.Fatalf("primary result = %#v, want one text agent response part", selection.PrimaryResult)
	}
	if selection.PrimaryResult[0].Text != agentResponse {
		t.Fatalf("primary result text = %q, want agent response %q", selection.PrimaryResult[0].Text, agentResponse)
	}
	if selection.PrimaryResult[0].Text == submitted.Content[0].Text {
		t.Fatalf("primary result echoed submitted request text")
	}
}
