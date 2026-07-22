package projections_test

import (
	"encoding/json"
	"strings"
	"testing"
	"time"

	interfaces "github.com/portpowered/infinite-you/pkg/services/factory_definitions"
	. "github.com/portpowered/infinite-you/pkg/services/recordings/projections"
	"github.com/portpowered/infinite-you/pkg/services/work"
	workerexecution "github.com/portpowered/infinite-you/pkg/services/workers"
	factoryapi "github.com/portpowered/infinite-you/pkg/transports/http/generated"
)

func TestReconstructFactoryWorldState_JavaScriptDispatchLifecycleReconstructsQueueInterruptReconcileAndArtifact(t *testing.T) {
	t0 := time.Date(2026, 6, 9, 14, 10, 0, 0, time.UTC)
	events := []factoryapi.FactoryEvent{
		javascriptRunRequestEvent(t0),
		dispatchQueuedEvent(1, t0.Add(2*time.Second)),
		dispatchInterruptedEvent(2, t0.Add(3*time.Second)),
		dispatchReconciledEvent(3, t0.Add(4*time.Second)),
		javascriptArtifactCreatedEvent(3, t0.Add(5*time.Second)),
	}

	worldState, err := ReconstructFactoryWorldState(events, 3)
	if err != nil {
		t.Fatalf("ReconstructFactoryWorldState: %v", err)
	}
	assertJavaScriptDispatchLifecycleReplay(t, worldState)

	view := BuildFactoryWorldView(worldState)
	if view.Runtime.JavaScript == nil {
		t.Fatal("javascript projection = nil, want dispatch lifecycle projection")
	}
	if view.Runtime.JavaScript.ChildDispatchCounts.Completed != 1 {
		t.Fatalf("child dispatch counts = %#v, want one completed dispatch", view.Runtime.JavaScript.ChildDispatchCounts)
	}
	if len(view.Runtime.JavaScript.Artifacts) != 1 {
		t.Fatalf("javascript artifacts = %#v, want one artifact", view.Runtime.JavaScript.Artifacts)
	}
}

func TestReconstructFactoryWorldState_JavaScriptDispatchLifecycleSuppressesLateReconcileAfterInterrupt(t *testing.T) {
	t0 := time.Date(2026, 6, 9, 14, 10, 0, 0, time.UTC)
	events := []factoryapi.FactoryEvent{
		javascriptRunRequestEvent(t0),
		dispatchQueuedEvent(1, t0.Add(2*time.Second)),
		dispatchInterruptedEvent(2, t0.Add(3*time.Second)),
		lateDispatchReconciledEvent(3, t0.Add(4*time.Second)),
	}

	worldState, err := ReconstructFactoryWorldState(events, 3)
	if err != nil {
		t.Fatalf("ReconstructFactoryWorldState: %v", err)
	}
	if worldState.JavaScriptRuntime == nil || len(worldState.JavaScriptRuntime.Dispatches) != 1 {
		t.Fatalf("dispatches = %#v, want one dispatch", worldState.JavaScriptRuntime)
	}
	dispatch := worldState.JavaScriptRuntime.Dispatches[0]
	if dispatch.Status != string(factoryapi.FactoryDispatchStatusINTERRUPTED) {
		t.Fatalf("dispatch status = %q, want INTERRUPTED after suppressed reconcile", dispatch.Status)
	}
	if len(dispatch.ArtifactIDs) != 0 {
		t.Fatalf("artifact ids = %#v, want late reconcile artifacts suppressed", dispatch.ArtifactIDs)
	}
}

func lateDispatchReconciledEvent(tick int, eventTime time.Time) factoryapi.FactoryEvent {
	sessionID := "session-js"
	kind := factoryapi.JAVASCRIPT
	dispatchID := "dispatch-js-1"
	source := "provider-session"
	context := factoryapi.FactoryEventContext{
		SessionId:        &sessionID,
		OrchestratorKind: &kind,
		DispatchId:       stringPointer(dispatchID),
		Source:           &source,
	}
	payload := factoryapi.DispatchReconciledEventPayload{
		ReconciledStatus:     factoryapi.FactoryDispatchStatusCOMPLETED,
		ReconciliationSource: factoryapi.PROVIDERSESSION,
		Replayed:             false,
		ArtifactIds:          &[]string{"artifact-result-1"},
	}
	return generatedProjectionEvent(factoryapi.FactoryEventTypeDispatchReconciled, "dispatch-reconciled/"+dispatchID, tick, eventTime, context, payload)
}

func TestReconstructFactoryWorldState_PetriDispatchRequestResponseRemainsRepresentable(t *testing.T) {
	t0 := time.Date(2026, 6, 9, 14, 15, 0, 0, time.UTC)
	workItem := work.FactoryWorkItem{ID: "work-1", WorkTypeID: "task", DisplayName: "draft", TraceID: "trace-1"}
	events := []factoryapi.FactoryEvent{
		initialStructureEvent(t0),
		workInputEvent(1, t0.Add(time.Second), workItem),
		workstationRequestEvent(2, t0.Add(2*time.Second), interfaces.WorkstationRequestPayload{
			DispatchID:   "dispatch-petri-1",
			TransitionID: "t-review",
			Workstation:  interfaces.FactoryWorkstationRef{ID: "t-review", Name: "Review"},
			Inputs: []interfaces.WorkstationInput{{
				TokenID:  "work-1",
				PlaceID:  "task:init",
				WorkItem: &workItem,
			}},
		}),
		workstationResponseEvent(2, t0.Add(3*time.Second), interfaces.WorkstationResponsePayload{
			DispatchID:   "dispatch-petri-1",
			TransitionID: "t-review",
			Result:       interfaces.WorkstationResult{Outcome: string(workerexecution.OutcomeContinue)},
		}),
	}

	worldState, err := ReconstructFactoryWorldState(events, 2)
	if err != nil {
		t.Fatalf("ReconstructFactoryWorldState: %v", err)
	}
	if len(worldState.CompletedDispatches) != 1 {
		t.Fatalf("completed dispatches = %#v, want one Petri dispatch completion", worldState.CompletedDispatches)
	}
	if worldState.JavaScriptRuntime != nil {
		t.Fatalf("javascript runtime = %#v, want nil for Petri replay", worldState.JavaScriptRuntime)
	}
}

func TestReconstructFactoryWorldState_ContinueDispatchReplaysNextTurnContent(t *testing.T) {
	t0 := time.Date(2026, 7, 12, 19, 0, 0, 0, time.UTC)
	input := work.FactoryWorkItem{
		ID:          "work-1",
		WorkTypeID:  "task",
		DisplayName: "draft",
		TraceID:     "trace-1",
		PlaceID:     "task:init",
		Content: []work.WorkContentPart{{
			Type: work.WorkContentPartTypeText,
			Text: "input-content",
		}},
	}
	continued := work.FactoryWorkItem{
		ID:          "work-1",
		WorkTypeID:  "task",
		DisplayName: "draft",
		TraceID:     "trace-1",
		PlaceID:     "task:init",
		Content: []work.WorkContentPart{{
			Type: work.WorkContentPartTypeText,
			Text: "next-turn-output",
		}},
	}
	outputWork := []factoryapi.Work{generatedLineageWorkForProjectionTest(t, continued, "")}
	events := []factoryapi.FactoryEvent{
		initialStructureEvent(t0),
		workInputEvent(1, t0.Add(time.Second), input),
		workstationRequestEvent(2, t0.Add(2*time.Second), interfaces.WorkstationRequestPayload{
			DispatchID:   "dispatch-continue",
			TransitionID: "t-review",
			Workstation:  interfaces.FactoryWorkstationRef{ID: "t-review", Name: "Review"},
			Inputs: []interfaces.WorkstationInput{{
				TokenID:  "work-1",
				PlaceID:  "task:init",
				WorkItem: &input,
			}},
		}),
		generatedProjectionEvent(
			factoryapi.FactoryEventTypeDispatchResponse,
			"response/dispatch-continue",
			2,
			t0.Add(3*time.Second),
			factoryapi.FactoryEventContext{
				DispatchId: stringPtrForProjectionTest("dispatch-continue"),
				TraceIds:   stringSlicePtrForProjectionTest([]string{"trace-1"}),
				WorkIds:    stringSlicePtrForProjectionTest([]string{"work-1"}),
			},
			factoryapi.DispatchResponseEventPayload{
				TransitionId: "t-review",
				Outcome:      factoryapi.WorkOutcomeContinue,
				OutputWork:   &outputWork,
			},
		),
	}

	worldState, err := ReconstructFactoryWorldState(events, 2)
	if err != nil {
		t.Fatalf("ReconstructFactoryWorldState: %v", err)
	}
	if len(worldState.CompletedDispatches) != 1 {
		t.Fatalf("completed dispatches = %#v, want one continue dispatch", worldState.CompletedDispatches)
	}
	dispatch := worldState.CompletedDispatches[0]
	if dispatch.Result.Outcome != string(workerexecution.OutcomeContinue) {
		t.Fatalf("dispatch outcome = %s, want CONTINUE", dispatch.Result.Outcome)
	}
	if len(dispatch.OutputWorkItems) != 1 {
		t.Fatalf("output work items = %#v, want one continued work item", dispatch.OutputWorkItems)
	}
	if len(dispatch.OutputWorkItems[0].Content) != 1 || dispatch.OutputWorkItems[0].Content[0].Text != "next-turn-output" {
		t.Fatalf("output work content = %#v, want next-turn-output", dispatch.OutputWorkItems[0].Content)
	}
	if dispatch.OutputWorkItems[0].Content[0].Text == "input-content" {
		t.Fatalf("projection echoed submitted request content on continue")
	}
}

func TestReconstructFactoryWorldState_FailedDispatchReplaysRequestContent(t *testing.T) {
	t0 := time.Date(2026, 7, 12, 20, 0, 0, 0, time.UTC)
	input := work.FactoryWorkItem{
		ID:          "work-1",
		WorkTypeID:  "task",
		DisplayName: "draft",
		TraceID:     "trace-1",
		PlaceID:     "task:init",
		Content: []work.WorkContentPart{{
			Type: work.WorkContentPartTypeText,
			Text: "input-content",
		}},
	}
	failed := work.FactoryWorkItem{
		ID:          "work-1",
		WorkTypeID:  "task",
		DisplayName: "draft",
		TraceID:     "trace-1",
		PlaceID:     "task:failed",
		Content: []work.WorkContentPart{{
			Type: work.WorkContentPartTypeText,
			Text: "input-content",
		}},
	}
	outputWork := []factoryapi.Work{generatedLineageWorkForProjectionTest(t, failed, "")}
	events := []factoryapi.FactoryEvent{
		initialStructureEvent(t0),
		workInputEvent(1, t0.Add(time.Second), input),
		workstationRequestEvent(2, t0.Add(2*time.Second), interfaces.WorkstationRequestPayload{
			DispatchID:   "dispatch-failed",
			TransitionID: "t-execute",
			Workstation:  interfaces.FactoryWorkstationRef{ID: "t-execute", Name: "Execute"},
			Inputs: []interfaces.WorkstationInput{{
				TokenID:  "work-1",
				PlaceID:  "task:init",
				WorkItem: &input,
			}},
		}),
		generatedProjectionEvent(
			factoryapi.FactoryEventTypeDispatchResponse,
			"response/dispatch-failed",
			2,
			t0.Add(3*time.Second),
			factoryapi.FactoryEventContext{
				DispatchId: stringPtrForProjectionTest("dispatch-failed"),
				TraceIds:   stringSlicePtrForProjectionTest([]string{"trace-1"}),
				WorkIds:    stringSlicePtrForProjectionTest([]string{"work-1"}),
			},
			factoryapi.DispatchResponseEventPayload{
				TransitionId: "t-execute",
				Outcome:      factoryapi.WorkOutcomeFailed,
				OutputWork:   &outputWork,
			},
		),
	}

	worldState, err := ReconstructFactoryWorldState(events, 2)
	if err != nil {
		t.Fatalf("ReconstructFactoryWorldState: %v", err)
	}
	if len(worldState.FailedDispatches) != 1 {
		t.Fatalf("failed dispatches = %#v, want one failed dispatch", worldState.FailedDispatches)
	}
	dispatch := worldState.FailedDispatches[0]
	if dispatch.Result.Outcome != string(workerexecution.OutcomeFailed) {
		t.Fatalf("dispatch outcome = %s, want FAILED", dispatch.Result.Outcome)
	}
	if len(dispatch.OutputWorkItems) != 1 {
		t.Fatalf("output work items = %#v, want one failed work item", dispatch.OutputWorkItems)
	}
	if len(dispatch.OutputWorkItems[0].Content) != 1 || dispatch.OutputWorkItems[0].Content[0].Text != "input-content" {
		t.Fatalf("output work content = %#v, want preserved request content", dispatch.OutputWorkItems[0].Content)
	}
	if _, ok := worldState.FailedWorkItemsByID["work-1"]; !ok {
		t.Fatalf("FailedWorkItemsByID = %#v, want work-1", worldState.FailedWorkItemsByID)
	}
}

// pkgmaintcheck:ignore-cyclomatic-complexity this replay assertion keeps JavaScript dispatch lifecycle counts, metadata, and artifacts visible in one scenario.
func assertJavaScriptDispatchLifecycleReplay(t *testing.T, worldState interfaces.FactoryWorldState) {
	t.Helper()
	if worldState.JavaScriptRuntime == nil {
		t.Fatal("javascript runtime = nil, want dispatch lifecycle projection")
	}
	if len(worldState.JavaScriptRuntime.Dispatches) != 1 {
		t.Fatalf("dispatches = %#v, want one dispatch", worldState.JavaScriptRuntime.Dispatches)
	}
	dispatch := worldState.JavaScriptRuntime.Dispatches[0]
	if dispatch.ID != "dispatch-js-1" || dispatch.Status != string(factoryapi.FactoryDispatchStatusCOMPLETED) {
		t.Fatalf("dispatch = %#v, want dispatch-js-1 COMPLETED", dispatch)
	}
	if dispatch.Phase != "execute" || dispatch.PromptDigest != "sha256:prompt" {
		t.Fatalf("dispatch metadata = %#v, want execute phase and prompt digest", dispatch)
	}
	if dispatch.JavaScript == nil || dispatch.JavaScript.TaskKind != "AGENT" {
		t.Fatalf("javascript dispatch = %#v, want AGENT task kind", dispatch.JavaScript)
	}
	if len(dispatch.ArtifactIDs) != 1 || dispatch.ArtifactIDs[0] != "artifact-result-1" {
		t.Fatalf("dispatch artifact ids = %#v, want artifact-result-1", dispatch.ArtifactIDs)
	}
	if len(worldState.Artifacts) != 1 || worldState.Artifacts[0].ID != "artifact-result-1" {
		t.Fatalf("artifacts = %#v, want artifact-result-1", worldState.Artifacts)
	}

	encoded, err := json.Marshal(worldState)
	if err != nil {
		t.Fatalf("marshal world state: %v", err)
	}
	body := string(encoded)
	for _, forbidden := range []string{"rawBody", "storagePath", "vmState", "systemPrompt", "userMessage"} {
		if strings.Contains(body, forbidden) {
			t.Fatalf("dispatch lifecycle world state leaked %q: %s", forbidden, body)
		}
	}
}

func dispatchQueuedEvent(tick int, eventTime time.Time) factoryapi.FactoryEvent {
	sessionID := "session-js"
	kind := factoryapi.JAVASCRIPT
	dispatchID := "dispatch-js-1"
	phaseID := "phase-execute"
	phaseName := "execute"
	queuePosition := 0
	context := factoryapi.FactoryEventContext{
		SessionId:        &sessionID,
		OrchestratorKind: &kind,
		PhaseId:          stringPointer(phaseID),
		PhaseName:        stringPointer(phaseName),
		DispatchId:       stringPointer(dispatchID),
	}
	payload := factoryapi.DispatchQueuedEventPayload{
		DispatchKind:  factoryapi.FactoryDispatchKindJAVASCRIPTAGENT,
		Label:         stringPointer("summarize findings"),
		QueuePosition: &queuePosition,
		PromptDigest:  stringPointer("sha256:prompt"),
		SchemaDigest:  stringPointer("sha256:schema"),
		InputWorkIds:  &[]string{"work-1"},
	}
	return generatedProjectionEvent(factoryapi.FactoryEventTypeDispatchQueued, "dispatch-queued/"+dispatchID, tick, eventTime, context, payload)
}

func dispatchInterruptedEvent(tick int, eventTime time.Time) factoryapi.FactoryEvent {
	sessionID := "session-js"
	kind := factoryapi.JAVASCRIPT
	dispatchID := "dispatch-js-1"
	context := factoryapi.FactoryEventContext{
		SessionId:        &sessionID,
		OrchestratorKind: &kind,
		DispatchId:       stringPointer(dispatchID),
	}
	payload := factoryapi.DispatchInterruptedEventPayload{
		Reason:         "provider disconnected",
		ObservedStatus: factoryapi.FactoryDispatchStatusFAILED,
		InterruptedAt:  eventTime,
		RetryPlanned:   true,
	}
	return generatedProjectionEvent(factoryapi.FactoryEventTypeDispatchInterrupted, "dispatch-interrupted/"+dispatchID, tick, eventTime, context, payload)
}

func dispatchReconciledEvent(tick int, eventTime time.Time) factoryapi.FactoryEvent {
	sessionID := "session-js"
	kind := factoryapi.JAVASCRIPT
	dispatchID := "dispatch-js-1"
	source := "replay"
	context := factoryapi.FactoryEventContext{
		SessionId:        &sessionID,
		OrchestratorKind: &kind,
		DispatchId:       stringPointer(dispatchID),
		Source:           &source,
	}
	payload := factoryapi.DispatchReconciledEventPayload{
		ReconciledStatus:     factoryapi.FactoryDispatchStatusCOMPLETED,
		ReconciliationSource: factoryapi.PROVIDERSESSION,
		Replayed:             true,
		ArtifactIds:          &[]string{"artifact-result-1"},
	}
	return generatedProjectionEvent(factoryapi.FactoryEventTypeDispatchReconciled, "dispatch-reconciled/"+dispatchID, tick, eventTime, context, payload)
}
