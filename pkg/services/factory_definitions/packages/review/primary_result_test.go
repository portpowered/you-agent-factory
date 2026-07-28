package review

import (
	"errors"
	"testing"

	"github.com/portpowered/infinite-you/pkg/services/work"
	factorydefinitions "github.com/portpowered/infinite-you/pkg/services/factory_definitions"
)

func TestInvocationPrimaryResult_ReturnsApprovedWorkOnly(t *testing.T) {
	requestID, workID := "req-review", "work-review"
	submitted := work.FactoryWorkItem{ID: workID, WorkTypeID: PackagedWorkTypeName, State: "init", TraceID: requestID, Content: []work.WorkContentPart{{Type: work.WorkContentPartTypeText, Text: "original request"}}}
	approved := submitted
	approved.State, approved.PlaceID = PackagedInvocationReturnTerminalState, PackagedWorkTypeName+":"+PackagedInvocationReturnTerminalState
	approved.Content = []work.WorkContentPart{{Type: work.WorkContentPartTypeText, Text: "approved candidate work"}}
	state := factorydefinitions.FactoryWorldState{PayloadLineage: work.WorkPayloadLineageProjection{}, WorkRequestsByID: map[string]factorydefinitions.WorkRequestPayload{requestID: {RequestID: requestID, WorkItems: []work.FactoryWorkItem{submitted}}}, TerminalWorkByID: map[string]factorydefinitions.FactoryTerminalWork{workID: {WorkItem: approved, Status: "TERMINAL"}}}
	state.PayloadLineage.RecordWorkRequestSnapshot(1, requestID, submitted)
	state.PayloadLineage.RecordConsumedInputSnapshot("execute", submitted)
	state.PayloadLineage.RecordDispatchOutputSnapshot(2, "review", []work.FactoryWorkItem{submitted}, approved, 0)
	selection, err := work.ResolvePrimaryResult(work.PrimaryResultSelectionInput{RequestID: requestID, InvocationReturn: &factorydefinitions.InvocationReturnConfig{Policy: "EXPLICIT", WorkTypeName: PackagedWorkTypeName, TerminalState: PackagedInvocationReturnTerminalState}, WorldState: state})
	if err != nil {
		t.Fatalf("ResolvePrimaryResult: %v", err)
	}
	if len(selection.PrimaryResult) != 1 || selection.PrimaryResult[0].Text != "approved candidate work" {
		t.Fatalf("primary result = %#v", selection.PrimaryResult)
	}
}

func TestInvocationPrimaryResult_DoesNotSelectFailedWork(t *testing.T) {
	state := factorydefinitions.FactoryWorldState{TerminalWorkByID: map[string]factorydefinitions.FactoryTerminalWork{"failed": {WorkItem: work.FactoryWorkItem{ID: "failed", WorkTypeID: PackagedWorkTypeName, State: "failed", TraceID: "req"}, Status: "FAILED"}}}
	_, err := work.ResolvePrimaryResult(work.PrimaryResultSelectionInput{RequestID: "req", InvocationReturn: &factorydefinitions.InvocationReturnConfig{Policy: "EXPLICIT", WorkTypeName: PackagedWorkTypeName, TerminalState: PackagedInvocationReturnTerminalState}, WorldState: state})
	var selectionErr *work.PrimaryResultError
	if !errors.As(err, &selectionErr) || selectionErr.Code != work.PrimaryResultErrorCodeUnresolved {
		t.Fatalf("error = %v", err)
	}
}
