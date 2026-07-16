package review

import (
	"errors"
	"testing"

	interfaces "github.com/portpowered/infinite-you/pkg/factory/contracts"
	"github.com/portpowered/infinite-you/pkg/work"
	invocations "github.com/portpowered/infinite-you/pkg/work/invocation"
)

func TestInvocationPrimaryResult_ReturnsApprovedWorkOnly(t *testing.T) {
	requestID, workID := "req-review", "work-review"
	submitted := work.FactoryWorkItem{ID: workID, WorkTypeID: PackagedWorkTypeName, State: "init", TraceID: requestID, Content: []work.WorkContentPart{{Type: work.WorkContentPartTypeText, Text: "original request"}}}
	approved := submitted
	approved.State, approved.PlaceID = PackagedInvocationReturnTerminalState, PackagedWorkTypeName+":"+PackagedInvocationReturnTerminalState
	approved.Content = []work.WorkContentPart{{Type: work.WorkContentPartTypeText, Text: "approved candidate work"}}
	state := interfaces.FactoryWorldState{PayloadLineage: work.WorkPayloadLineageProjection{}, WorkRequestsByID: map[string]interfaces.WorkRequestPayload{requestID: {RequestID: requestID, WorkItems: []work.FactoryWorkItem{submitted}}}, TerminalWorkByID: map[string]interfaces.FactoryTerminalWork{workID: {WorkItem: approved, Status: "TERMINAL"}}}
	state.PayloadLineage.RecordWorkRequestSnapshot(1, requestID, submitted)
	state.PayloadLineage.RecordConsumedInputSnapshot("execute", submitted)
	state.PayloadLineage.RecordDispatchOutputSnapshot(2, "review", []work.FactoryWorkItem{submitted}, approved, 0)
	selection, err := invocations.ResolvePrimaryResult(invocations.PrimaryResultSelectionInput{RequestID: requestID, InvocationReturn: &interfaces.InvocationReturnConfig{Policy: "EXPLICIT", WorkTypeName: PackagedWorkTypeName, TerminalState: PackagedInvocationReturnTerminalState}, WorldState: state})
	if err != nil {
		t.Fatalf("ResolvePrimaryResult: %v", err)
	}
	if len(selection.PrimaryResult) != 1 || selection.PrimaryResult[0].Text != "approved candidate work" {
		t.Fatalf("primary result = %#v", selection.PrimaryResult)
	}
}

func TestInvocationPrimaryResult_DoesNotSelectFailedWork(t *testing.T) {
	state := interfaces.FactoryWorldState{TerminalWorkByID: map[string]interfaces.FactoryTerminalWork{"failed": {WorkItem: work.FactoryWorkItem{ID: "failed", WorkTypeID: PackagedWorkTypeName, State: "failed", TraceID: "req"}, Status: "FAILED"}}}
	_, err := invocations.ResolvePrimaryResult(invocations.PrimaryResultSelectionInput{RequestID: "req", InvocationReturn: &interfaces.InvocationReturnConfig{Policy: "EXPLICIT", WorkTypeName: PackagedWorkTypeName, TerminalState: PackagedInvocationReturnTerminalState}, WorldState: state})
	var selectionErr *invocations.PrimaryResultError
	if !errors.As(err, &selectionErr) || selectionErr.Code != invocations.PrimaryResultErrorCodeUnresolved {
		t.Fatalf("error = %v", err)
	}
}
