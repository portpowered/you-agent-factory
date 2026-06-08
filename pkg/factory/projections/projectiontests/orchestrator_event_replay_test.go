package projections_test

import (
	"encoding/json"
	"strings"
	"testing"
	"time"

	factoryapi "github.com/portpowered/infinite-you/pkg/api/generated"
	"github.com/portpowered/infinite-you/pkg/apisurface"
	. "github.com/portpowered/infinite-you/pkg/factory/projections"
	"github.com/portpowered/infinite-you/pkg/factorysessions"
	"github.com/portpowered/infinite-you/pkg/interfaces"
	"github.com/portpowered/infinite-you/pkg/workflowresult"
)

func TestReconstructFactoryWorldState_PetriFixtureReconstructsMarkingWithoutJavaScriptProjection(t *testing.T) {
	t0 := time.Date(2026, 6, 8, 17, 0, 0, 0, time.UTC)
	workItem := interfaces.FactoryWorkItem{ID: "work-1", WorkTypeID: "task", DisplayName: "draft", TraceID: "trace-1"}
	events := []factoryapi.FactoryEvent{
		initialStructureEvent(t0),
		workInputEvent(1, t0.Add(time.Second), workItem),
		workStateChangeEvent(1, t0.Add(2*time.Second), "work-1", "init", "review", "task:init", "task:review", factoryapi.WorkStateChangeSourceAPI),
	}

	worldState, err := ReconstructFactoryWorldState(events, 1)
	if err != nil {
		t.Fatalf("ReconstructFactoryWorldState: %v", err)
	}
	if len(worldState.WorkStateChangesByWorkID["work-1"]) != 1 {
		t.Fatalf("work state changes = %#v, want one Petri marking change", worldState.WorkStateChangesByWorkID["work-1"])
	}
	if worldState.JavaScriptRuntime != nil {
		t.Fatalf("javascript runtime = %#v, want nil for Petri replay", worldState.JavaScriptRuntime)
	}

	view := BuildFactoryWorldView(worldState)
	if view.Runtime.JavaScript != nil {
		t.Fatalf("javascript projection = %#v, want nil for Petri replay", view.Runtime.JavaScript)
	}
	if len(view.Runtime.PlaceTokenCounts) == 0 && len(view.Runtime.CurrentWorkItemsByPlaceID) == 0 {
		t.Fatalf("petri runtime projection = %#v, want marking occupancy", view.Runtime)
	}
}

func TestReconstructFactoryWorldState_SessionResultUpdatedMatchesSessionResultProjection(t *testing.T) {
	t0 := time.Date(2026, 6, 8, 17, 10, 0, 0, time.UTC)
	sessionID := "session-js"
	primaryJSON, err := json.Marshal(map[string]any{"ok": true, "count": 2})
	if err != nil {
		t.Fatalf("marshal primary result: %v", err)
	}
	var primaryResult factoryapi.WorkContent
	if err := json.Unmarshal([]byte(`[{"type":"JSON","json":{"ok":true,"count":2}}]`), &primaryResult); err != nil {
		t.Fatalf("unmarshal primary result content: %v", err)
	}
	resultArtifact := factoryapi.FactoryArtifactRef{
		Id:         "artifact-final-1",
		Kind:       factoryapi.FactoryArtifactKindFINALRESULT,
		Visibility: factoryapi.FactoryArtifactVisibilityPUBLIC,
	}
	input := workflowresult.SessionResultInput{
		SessionID:    sessionID,
		Status:       factoryapi.FactorySessionStatusFINISHED,
		PrimaryValue: workflowresult.TypedValue{JSON: primaryJSON},
		ResultArtifact: &resultArtifact,
		Artifacts: []interfaces.FactorySessionArtifactState{{
			ID:         "artifact-final-1",
			Kind:       "FINAL_RESULT",
			Visibility: "PUBLIC",
		}},
	}
	eventPayload := apisurface.BuildWorkflowSessionResultUpdatedPayload(input)
	events := []factoryapi.FactoryEvent{
		javascriptRunRequestEvent(t0),
		javascriptSessionResultUpdatedEvent(1, t0.Add(2*time.Second), eventPayload),
	}

	worldState, err := ReconstructFactoryWorldState(events, 1)
	if err != nil {
		t.Fatalf("ReconstructFactoryWorldState: %v", err)
	}
	ctx := factorysessions.ProjectionContext{
		Session: &factorysessions.LiveSession{ID: sessionID},
		FactoryCfg: &interfaces.FactoryConfig{
			Orchestrator: &interfaces.FactoryOrchestratorConfig{
				Kind: interfaces.OrchestratorKindJavaScript,
			},
		},
		JavaScript: &interfaces.FactorySessionJavaScriptRuntimeState{
			ScriptStatus:  "FINISHED",
			PrimaryResult: worldState.JavaScriptRuntime.PrimaryResult,
			Artifacts:     worldState.JavaScriptRuntime.Artifacts,
		},
		Now: t0.Add(2 * time.Second),
	}
	durableResult := apisurface.BuildWorkflowSessionResult(input)
	if durableResult.PrimaryResult == nil || eventPayload.PrimaryResult == nil {
		t.Fatalf("primary results = session %#v event %#v", durableResult.PrimaryResult, eventPayload.PrimaryResult)
	}
	if durableResult.ArtifactIds == nil || len(*durableResult.ArtifactIds) == 0 || eventPayload.ResultArtifactRef == nil {
		t.Fatal("expected matching result artifact ids")
	}
	if (*durableResult.ArtifactIds)[0] != eventPayload.ResultArtifactRef.Id {
		t.Fatalf("artifact ids differ: %q vs %q", (*durableResult.ArtifactIds)[0], eventPayload.ResultArtifactRef.Id)
	}
	liveResult := factorysessions.ProjectSessionResult(sessionID, ctx, factorysessions.NewJavaScriptCheckpointStore())
	if liveResult.ResultArtifactRef == nil || liveResult.ResultArtifactRef.Id != eventPayload.ResultArtifactRef.Id {
		t.Fatalf("live result artifact = %#v, want id %q", liveResult.ResultArtifactRef, eventPayload.ResultArtifactRef.Id)
	}
}

func TestReconstructFactoryWorldState_JavaScriptFixtureReconstructsPhaseCheckpointArtifactsWithoutPetriMarking(t *testing.T) {
	t0 := time.Date(2026, 6, 8, 17, 5, 0, 0, time.UTC)
	checkpointTime := t0.Add(3 * time.Second)
	artifactTime := t0.Add(4 * time.Second)
	events := []factoryapi.FactoryEvent{
		javascriptRunRequestEvent(t0),
		javascriptPhaseChangeEvent(1, t0.Add(2*time.Second)),
		javascriptCheckpointRefEvent(2, checkpointTime),
		javascriptArtifactCreatedEvent(2, artifactTime),
	}

	worldState, err := ReconstructFactoryWorldState(events, 2)
	if err != nil {
		t.Fatalf("ReconstructFactoryWorldState: %v", err)
	}
	assertJavaScriptWorldReplay(t, worldState)
}

func assertJavaScriptWorldReplay(t *testing.T, worldState interfaces.FactoryWorldState) {
	t.Helper()
	assertJavaScriptWorldStateReplay(t, worldState)
	view := BuildFactoryWorldView(worldState)
	assertJavaScriptWorldViewReplay(t, view)
}

func assertJavaScriptWorldStateReplay(t *testing.T, worldState interfaces.FactoryWorldState) {
	t.Helper()
	if len(worldState.WorkStateChangesByWorkID) != 0 {
		t.Fatalf("work state changes = %#v, want none for JavaScript replay", worldState.WorkStateChangesByWorkID)
	}
	if worldState.JavaScriptRuntime == nil {
		t.Fatal("javascript runtime = nil, want phase and dispatch counts")
	}
	if worldState.JavaScriptRuntime.Phase != "execute" || len(worldState.JavaScriptRuntime.Phases) != 2 {
		t.Fatalf("javascript phase = %#v, want execute with two phases", worldState.JavaScriptRuntime)
	}
	if worldState.JavaScriptRuntime.QueuedDispatches != 1 || worldState.JavaScriptRuntime.RunningDispatches != 2 || worldState.JavaScriptRuntime.CompletedDispatches != 3 {
		t.Fatalf("dispatch counts = queued=%d running=%d completed=%d, want 1/2/3",
			worldState.JavaScriptRuntime.QueuedDispatches,
			worldState.JavaScriptRuntime.RunningDispatches,
			worldState.JavaScriptRuntime.CompletedDispatches,
		)
	}
	if len(worldState.JavaScriptCheckpoints) != 1 || len(worldState.Artifacts) != 1 {
		t.Fatalf("checkpoints=%d artifacts=%d, want one each", len(worldState.JavaScriptCheckpoints), len(worldState.Artifacts))
	}
}

func assertJavaScriptWorldViewReplay(t *testing.T, view interfaces.FactoryWorldView) {
	t.Helper()
	if view.Runtime.JavaScript == nil {
		t.Fatal("javascript projection = nil, want orchestrator runtime projection")
	}
	if view.Runtime.JavaScript.Phase != "execute" || view.Runtime.JavaScript.ScriptStatus != "RUNNING" {
		t.Fatalf("javascript projection = %#v, want execute/RUNNING", view.Runtime.JavaScript)
	}
	if len(view.Runtime.JavaScript.Checkpoints) != 1 || len(view.Runtime.JavaScript.Artifacts) != 1 {
		t.Fatalf("javascript projection refs = %#v, want checkpoint and artifact", view.Runtime.JavaScript)
	}
	if view.Runtime.JavaScript.ChildDispatchCounts.Completed != 3 {
		t.Fatalf("child dispatch counts = %#v, want completed=3", view.Runtime.JavaScript.ChildDispatchCounts)
	}

	encoded, err := json.Marshal(view)
	if err != nil {
		t.Fatalf("marshal world view: %v", err)
	}
	body := string(encoded)
	for _, forbidden := range []string{"rawBody", "storagePath", "vmState", "fromPlaceId", "toPlaceId"} {
		if strings.Contains(body, forbidden) {
			t.Fatalf("javascript world view leaked %q: %s", forbidden, body)
		}
	}
}

func javascriptRunRequestEvent(eventTime time.Time) factoryapi.FactoryEvent {
	payload := factoryapi.RunRequestEventPayload{
		RecordedAt: eventTime,
		Factory: factoryapi.Factory{
			Name: "dynamic-workflow",
			Orchestrator: &factoryapi.FactoryOrchestrator{
				Kind: factoryapi.JAVASCRIPT,
				Javascript: &factoryapi.FactoryOrchestratorJavaScriptConfig{
					Dialect:    stringPointer("workflow-v1"),
					SourceRef:  stringPointer("factory/workflows/review.js"),
					SourceHash: stringPointer("sha256:abc123"),
				},
			},
		},
	}
	return generatedProjectionEvent(factoryapi.FactoryEventTypeRunRequest, "run-request-js", 0, eventTime, factoryapi.FactoryEventContext{}, payload)
}

func javascriptPhaseChangeEvent(tick int, eventTime time.Time) factoryapi.FactoryEvent {
	payload := factoryapi.JavaScriptPhaseChangeEventPayload{
		Phase:      "execute",
		Phases:     []string{"plan", "execute"},
		ArgsDigest: stringPointer("sha256:args"),
		ScriptStatus: factoryapi.FactorySessionJavaScriptScriptStatusRUNNING,
		ChildDispatchCounts: factoryapi.FactorySessionJavaScriptChildDispatchCounts{
			Queued:    1,
			Running:   2,
			Completed: 3,
		},
	}
	return generatedProjectionEvent(factoryapi.FactoryEventTypeJavaScriptPhaseChange, "javascript-phase-change", tick, eventTime, factoryapi.FactoryEventContext{}, payload)
}

func javascriptCheckpointRefEvent(tick int, eventTime time.Time) factoryapi.FactoryEvent {
	hash := "sha256:checkpoint-body"
	size := int64(128)
	payload := factoryapi.JavaScriptCheckpointRefEventPayload{
		CheckpointId: "ckpt-1",
		Label:        stringPointer("after-plan"),
		Summary:      stringPointer("Completed planning phase"),
		Timestamp:    &eventTime,
		ArtifactRef: factoryapi.FactoryArtifactRef{
			Id:          "artifact-ckpt-1",
			Kind:        factoryapi.FactoryArtifactKindCHECKPOINT,
			Visibility:  factoryapi.FactoryArtifactVisibilityINTERNALCHECKPOINT,
			ContentHash: &hash,
			SizeBytes:   &size,
		},
	}
	return generatedProjectionEvent(factoryapi.FactoryEventTypeJavaScriptCheckpointRef, "javascript-checkpoint-ref", tick, eventTime, factoryapi.FactoryEventContext{}, payload)
}

func javascriptSessionResultUpdatedEvent(tick int, eventTime time.Time, payload factoryapi.SessionResultUpdatedEventPayload) factoryapi.FactoryEvent {
	return generatedProjectionEvent(factoryapi.FactoryEventTypeSessionResultUpdated, "session-result-updated", tick, eventTime, factoryapi.FactoryEventContext{}, payload)
}

func javascriptArtifactCreatedEvent(tick int, eventTime time.Time) factoryapi.FactoryEvent {
	hash := "sha256:result-body"
	size := int64(512)
	label := "Review summary"
	summary := "Completed review findings"
	payload := factoryapi.ArtifactCreatedEventPayload{
		Artifact: factoryapi.FactoryArtifact{
			Id:          "artifact-result-1",
			Kind:        factoryapi.FactoryArtifactKindFINALRESULT,
			Visibility:  factoryapi.FactoryArtifactVisibilityPUBLIC,
			Label:       &label,
			Summary:     &summary,
			ContentHash: &hash,
			SizeBytes:   &size,
		},
		CapturedAt: &eventTime,
	}
	return generatedProjectionEvent(factoryapi.FactoryEventTypeArtifactCreated, "artifact-created", tick, eventTime, factoryapi.FactoryEventContext{}, payload)
}
