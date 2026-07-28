package goal

import (
	"errors"
	"testing"

	"github.com/portpowered/infinite-you/pkg/services/work"
	factorydefinitions "github.com/portpowered/infinite-you/pkg/services/factory_definitions"
)

func TestPackagedGoalInvocationPrimaryResult_ReturnsSummaryNotSubmittedInput(t *testing.T) {
	summaryContent, err := SummaryContentFromWorkerOutput("Updated README.md with the completed repository edit.\n<COMPLETE>", "<COMPLETE>")
	if err != nil {
		t.Fatalf("SummaryContentFromWorkerOutput: %v", err)
	}

	requestID := "req-goal"
	workID := "work-goal"
	submitted := work.FactoryWorkItem{
		ID:         workID,
		WorkTypeID: PackagedInvocationReturnWorkTypeName,
		State:      "init",
		TraceID:    requestID,
		Content: []work.WorkContentPart{{
			Type: work.WorkContentPartTypeText,
			Text: "customer goal request text",
		}},
	}
	terminal := work.FactoryWorkItem{
		ID:         workID,
		WorkTypeID: PackagedInvocationReturnWorkTypeName,
		State:      PackagedInvocationReturnTerminalState,
		TraceID:    requestID,
		PlaceID:    PackagedInvocationReturnWorkTypeName + ":" + PackagedInvocationReturnTerminalState,
		Content:    summaryContent,
	}

	state := factorydefinitions.FactoryWorldState{
		PayloadLineage:   work.WorkPayloadLineageProjection{},
		WorkRequestsByID: make(map[string]factorydefinitions.WorkRequestPayload),
		TerminalWorkByID: make(map[string]factorydefinitions.FactoryTerminalWork),
	}
	state.WorkRequestsByID[requestID] = factorydefinitions.WorkRequestPayload{
		RequestID: requestID,
		Type:      work.WorkRequestTypeFactoryRequestBatch,
		WorkItems: []work.FactoryWorkItem{submitted},
	}
	state.PayloadLineage.RecordWorkRequestSnapshot(1, requestID, submitted)
	state.PayloadLineage.RecordConsumedInputSnapshot("dispatch-goal", submitted)
	state.PayloadLineage.RecordDispatchOutputSnapshot(2, "dispatch-goal", []work.FactoryWorkItem{submitted}, terminal, 0)
	state.TerminalWorkByID[workID] = factorydefinitions.FactoryTerminalWork{WorkItem: terminal, Status: "TERMINAL"}

	selection, err := work.ResolvePrimaryResult(work.PrimaryResultSelectionInput{
		RequestID: requestID,
		InvocationReturn: &factorydefinitions.InvocationReturnConfig{
			Policy:        "EXPLICIT",
			WorkTypeName:  PackagedInvocationReturnWorkTypeName,
			TerminalState: PackagedInvocationReturnTerminalState,
		},
		WorldState: state,
	})
	if err != nil {
		t.Fatalf("ResolvePrimaryResult: %v", err)
	}
	if len(selection.PrimaryResult) != 1 || selection.PrimaryResult[0].Type != work.WorkContentPartTypeText {
		t.Fatalf("primary result = %#v, want one text summary part", selection.PrimaryResult)
	}
	if selection.PrimaryResult[0].Text != "Updated README.md with the completed repository edit." {
		t.Fatalf("primary result text = %q, want worker summary", selection.PrimaryResult[0].Text)
	}
	if selection.PrimaryResult[0].Text == submitted.Content[0].Text {
		t.Fatalf("primary result echoed submitted goal text")
	}
}

func TestPackagedGoalInvocationPrimaryResult_UnresolvedWhenConfiguredTerminalMissing(t *testing.T) {
	requestID := "req-goal-unresolved"
	workID := "work-goal-unresolved"
	submitted := work.FactoryWorkItem{
		ID:         workID,
		WorkTypeID: PackagedInvocationReturnWorkTypeName,
		State:      "init",
		TraceID:    requestID,
		Content: []work.WorkContentPart{{
			Type: work.WorkContentPartTypeText,
			Text: "customer goal request text",
		}},
	}
	failedTerminal := work.FactoryWorkItem{
		ID:         workID,
		WorkTypeID: PackagedInvocationReturnWorkTypeName,
		State:      "failed",
		TraceID:    requestID,
		PlaceID:    PackagedInvocationReturnWorkTypeName + ":failed",
		Content:    submitted.Content,
	}

	state := factorydefinitions.FactoryWorldState{
		PayloadLineage:   work.WorkPayloadLineageProjection{},
		WorkRequestsByID: make(map[string]factorydefinitions.WorkRequestPayload),
		TerminalWorkByID: make(map[string]factorydefinitions.FactoryTerminalWork),
	}
	state.WorkRequestsByID[requestID] = factorydefinitions.WorkRequestPayload{
		RequestID: requestID,
		Type:      work.WorkRequestTypeFactoryRequestBatch,
		WorkItems: []work.FactoryWorkItem{submitted},
	}
	state.PayloadLineage.RecordWorkRequestSnapshot(1, requestID, submitted)
	state.PayloadLineage.RecordConsumedInputSnapshot("dispatch-goal", submitted)
	state.PayloadLineage.RecordDispatchOutputSnapshot(2, "dispatch-goal", []work.FactoryWorkItem{submitted}, failedTerminal, 0)
	state.TerminalWorkByID[workID] = factorydefinitions.FactoryTerminalWork{WorkItem: failedTerminal, Status: "FAILED"}

	_, err := work.ResolvePrimaryResult(work.PrimaryResultSelectionInput{
		RequestID: requestID,
		InvocationReturn: &factorydefinitions.InvocationReturnConfig{
			Policy:        "EXPLICIT",
			WorkTypeName:  PackagedInvocationReturnWorkTypeName,
			TerminalState: PackagedInvocationReturnTerminalState,
		},
		WorldState: state,
	})
	var selectionErr *work.PrimaryResultError
	if !errors.As(err, &selectionErr) {
		t.Fatalf("error = %v, want PrimaryResultError", err)
	}
	if selectionErr.Code != work.PrimaryResultErrorCodeUnresolved {
		t.Fatalf("code = %q, want %q", selectionErr.Code, work.PrimaryResultErrorCodeUnresolved)
	}
}
