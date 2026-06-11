package projections_test

import (
	"testing"
	"time"

	factoryapi "github.com/portpowered/infinite-you/pkg/api/generated"
	. "github.com/portpowered/infinite-you/pkg/factory/projections"
	"github.com/portpowered/infinite-you/pkg/interfaces"
	workstationprojection "github.com/portpowered/infinite-you/pkg/api/workstationprojection"
)

func TestReconstructFactoryWorldState_ProjectsDispatchRequestModelProviderMetadata(t *testing.T) {
	t0 := time.Date(2026, 6, 11, 12, 0, 0, 0, time.UTC)
	modelProvider := factoryapi.WorkerModelProviderGemini
	source := factoryapi.ModelProviderSelectionSourceWorkstation
	workItem := interfaces.FactoryWorkItem{
		ID:          "work-1",
		WorkTypeID:  "task",
		DisplayName: "Draft",
		TraceID:     "trace-1",
		PlaceID:     "task:init",
	}
	events := []factoryapi.FactoryEvent{
		initialStructureEvent(t0),
		workInputEvent(1, t0.Add(time.Second), workItem),
		dispatchRequestEventWithProviderMetadata(
			2,
			t0.Add(2*time.Second),
			"dispatch-provider-1",
			"t-review",
			workItem,
			&factoryapi.DispatchRequestEventMetadata{
				ModelProvider:                &modelProvider,
				ModelProviderSelectionSource: &source,
			},
		),
	}

	worldState, err := ReconstructFactoryWorldState(events, 2)
	if err != nil {
		t.Fatalf("ReconstructFactoryWorldState: %v", err)
	}
	dispatch := worldState.ActiveDispatches["dispatch-provider-1"]
	if dispatch.RunnerID != interfaces.RunnerIDGemini {
		t.Fatalf("dispatch runnerID = %q, want %q", dispatch.RunnerID, interfaces.RunnerIDGemini)
	}
	if dispatch.RunnerSelectionSource != interfaces.RunnerSelectionSourceWorkstation {
		t.Fatalf("dispatch selection source = %q, want %q", dispatch.RunnerSelectionSource, interfaces.RunnerSelectionSourceWorkstation)
	}

	slice := workstationprojection.BuildFactoryWorldWorkstationRequestProjectionSlice(worldState)
	if slice.WorkstationRequestsByDispatchId == nil {
		t.Fatal("workstation request projection missing")
	}
	request := (*slice.WorkstationRequestsByDispatchId)["dispatch-provider-1"]
	if request.Request.ModelProvider == nil {
		t.Fatal("request modelProvider view missing")
	}
	if got := stringValue(request.Request.ModelProvider.ModelProvider); got != string(factoryapi.WorkerModelProviderGemini) {
		t.Fatalf("request modelProvider = %q, want %q", got, factoryapi.WorkerModelProviderGemini)
	}
	if got := stringValue(request.Request.ModelProvider.ModelProviderSelectionSource); got != string(factoryapi.ModelProviderSelectionSourceWorkstation) {
		t.Fatalf("request modelProviderSelectionSource = %q, want %q", got, factoryapi.ModelProviderSelectionSourceWorkstation)
	}
}

func TestReconstructFactoryWorldState_ProjectsDispatchRequestClaudeModelProviderWithoutLegacyRunnerID(t *testing.T) {
	t0 := time.Date(2026, 6, 11, 13, 0, 0, 0, time.UTC)
	modelProvider := factoryapi.WorkerModelProviderClaude
	source := factoryapi.ModelProviderSelectionSourceWorker
	workItem := interfaces.FactoryWorkItem{
		ID:          "work-claude",
		WorkTypeID:  "task",
		DisplayName: "Draft",
		TraceID:     "trace-claude",
		PlaceID:     "task:init",
	}
	events := []factoryapi.FactoryEvent{
		initialStructureEvent(t0),
		workInputEvent(1, t0.Add(time.Second), workItem),
		dispatchRequestEventWithProviderMetadata(
			2,
			t0.Add(2*time.Second),
			"dispatch-claude-1",
			"t-review",
			workItem,
			&factoryapi.DispatchRequestEventMetadata{
				ModelProvider:                &modelProvider,
				ModelProviderSelectionSource: &source,
			},
		),
	}

	worldState, err := ReconstructFactoryWorldState(events, 2)
	if err != nil {
		t.Fatalf("ReconstructFactoryWorldState: %v", err)
	}
	dispatch := worldState.ActiveDispatches["dispatch-claude-1"]
	if dispatch.ModelProvider != string(factoryapi.WorkerModelProviderClaude) {
		t.Fatalf("dispatch modelProvider = %q, want %q", dispatch.ModelProvider, factoryapi.WorkerModelProviderClaude)
	}
	if dispatch.RunnerID != "" {
		t.Fatalf("dispatch runnerID = %q, want empty for claude provider", dispatch.RunnerID)
	}

	slice := workstationprojection.BuildFactoryWorldWorkstationRequestProjectionSlice(worldState)
	if slice.WorkstationRequestsByDispatchId == nil {
		t.Fatal("workstation request projection missing")
	}
	request := (*slice.WorkstationRequestsByDispatchId)["dispatch-claude-1"]
	if request.Request.ModelProvider == nil {
		t.Fatal("request modelProvider view missing")
	}
	if got := stringValue(request.Request.ModelProvider.ModelProvider); got != string(factoryapi.WorkerModelProviderClaude) {
		t.Fatalf("request modelProvider = %q, want %q", got, factoryapi.WorkerModelProviderClaude)
	}
	if request.Request.ModelProvider.DisplayName == nil || *request.Request.ModelProvider.DisplayName != "Claude" {
		t.Fatalf("request displayName = %#v, want Claude", request.Request.ModelProvider.DisplayName)
	}
}

func TestReconstructFactoryWorldState_ProjectsLegacyDispatchQueuedRunnerIDAsModelProvider(t *testing.T) {
	t0 := time.Date(2026, 6, 11, 12, 5, 0, 0, time.UTC)
	modelProvider := factoryapi.WorkerModelProviderCursor
	events := []factoryapi.FactoryEvent{
		javascriptRunRequestEvent(t0),
		dispatchQueuedEventWithModelProvider(1, t0.Add(time.Second), &modelProvider),
	}

	worldState, err := ReconstructFactoryWorldState(events, 1)
	if err != nil {
		t.Fatalf("ReconstructFactoryWorldState: %v", err)
	}
	if worldState.JavaScriptRuntime == nil || len(worldState.JavaScriptRuntime.Dispatches) != 1 {
		t.Fatalf("javascript dispatches = %#v, want one queued dispatch", worldState.JavaScriptRuntime)
	}
	dispatch := worldState.JavaScriptRuntime.Dispatches[0]
	if dispatch.RunnerID != interfaces.RunnerIDCursorCLI {
		t.Fatalf("dispatch runnerID = %q, want %q", dispatch.RunnerID, interfaces.RunnerIDCursorCLI)
	}
}

func dispatchRequestEventWithProviderMetadata(
	tick int,
	eventTime time.Time,
	dispatchID string,
	transitionID string,
	workItem interfaces.FactoryWorkItem,
	metadata *factoryapi.DispatchRequestEventMetadata,
) factoryapi.FactoryEvent {
	context := factoryapi.FactoryEventContext{
		DispatchId: stringPointer(dispatchID),
		TraceIds:   stringSlicePtrForProjectionTest([]string{workItem.TraceID}),
		WorkIds:    stringSlicePtrForProjectionTest([]string{workItem.ID}),
	}
	payload := factoryapi.DispatchRequestEventPayload{
		TransitionId: transitionID,
		Inputs: []factoryapi.DispatchConsumedWorkRef{{
			WorkId: workItem.ID,
		}},
		Metadata: metadata,
	}
	return generatedProjectionEvent(factoryapi.FactoryEventTypeDispatchRequest, "request/"+dispatchID, tick, eventTime, context, payload)
}

func dispatchQueuedEventWithModelProvider(tick int, eventTime time.Time, modelProvider *factoryapi.WorkerModelProvider) factoryapi.FactoryEvent {
	sessionID := "session-js"
	kind := factoryapi.JAVASCRIPT
	dispatchID := "dispatch-js-provider"
	context := factoryapi.FactoryEventContext{
		SessionId:        &sessionID,
		OrchestratorKind: &kind,
		DispatchId:       stringPointer(dispatchID),
	}
	payload := factoryapi.DispatchQueuedEventPayload{
		DispatchKind:  factoryapi.FactoryDispatchKindJAVASCRIPTAGENT,
		ModelProvider: modelProvider,
	}
	return generatedProjectionEvent(factoryapi.FactoryEventTypeDispatchQueued, "dispatch-queued/"+dispatchID, tick, eventTime, context, payload)
}

func stringValue[T ~string](value *T) string {
	if value == nil {
		return ""
	}
	return string(*value)
}
