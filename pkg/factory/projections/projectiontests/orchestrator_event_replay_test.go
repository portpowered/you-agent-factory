package projections_test

import (
	"encoding/json"
	"strings"
	"testing"
	"time"

	interfaces "github.com/portpowered/infinite-you/pkg/factory/contracts"
	. "github.com/portpowered/infinite-you/pkg/factory/projections"
	factorysessions "github.com/portpowered/infinite-you/pkg/factory/sessions"
	workflowresult "github.com/portpowered/infinite-you/pkg/orchestrators/javascript/result"
	factoryapi "github.com/portpowered/infinite-you/pkg/transports/http/generated"
	apisurface "github.com/portpowered/infinite-you/pkg/transports/mapping"
	"github.com/portpowered/infinite-you/pkg/work"
)

func TestReconstructFactoryWorldState_PetriFixtureReconstructsMarkingWithoutJavaScriptProjection(t *testing.T) {
	t0 := time.Date(2026, 6, 8, 17, 0, 0, 0, time.UTC)
	workItem := work.FactoryWorkItem{ID: "work-1", WorkTypeID: "task", DisplayName: "draft", TraceID: "trace-1"}
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
	resultArtifact := interfaces.FactoryArtifactRef{
		ID:         "artifact-result-1",
		Kind:       "FINAL_RESULT",
		Visibility: "PUBLIC",
	}
	input := workflowresult.SessionResultInput{
		SessionID:      sessionID,
		Status:         interfaces.RuntimeStatusFinished,
		PrimaryValue:   workflowresult.TypedValue{JSON: primaryJSON},
		ResultArtifact: &resultArtifact,
		Artifacts: []interfaces.FactorySessionArtifactState{{
			ID:         "artifact-result-1",
			Kind:       "FINAL_RESULT",
			Visibility: "PUBLIC",
		}},
	}
	eventPayload := apisurface.BuildWorkflowSessionResultUpdatedPayload(input)
	events := []factoryapi.FactoryEvent{
		javascriptRunRequestEvent(t0),
		javascriptArtifactCreatedEvent(1, t0.Add(1500*time.Millisecond)),
		javascriptSessionResultUpdatedEvent(2, t0.Add(2*time.Second), eventPayload),
	}

	worldState, err := ReconstructFactoryWorldState(events, 2)
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
	if durableResult.PrimaryResult == nil || eventPayload.ResultSummary == nil {
		t.Fatalf("primary results = session %#v event %#v", durableResult.PrimaryResult, eventPayload.ResultSummary)
	}
	if durableResult.ArtifactIds == nil || len(*durableResult.ArtifactIds) == 0 || eventPayload.ArtifactIds == nil || len(*eventPayload.ArtifactIds) == 0 {
		t.Fatal("expected matching result artifact ids")
	}
	if (*durableResult.ArtifactIds)[0] != (*eventPayload.ArtifactIds)[0] {
		t.Fatalf("artifact ids differ: %q vs %q", (*durableResult.ArtifactIds)[0], (*eventPayload.ArtifactIds)[0])
	}
	liveResult := factorysessions.ProjectSessionResult(sessionID, ctx, factorysessions.NewJavaScriptCheckpointStore())
	if liveResult.ResultArtifactRef == nil || liveResult.ResultArtifactRef.ID != (*eventPayload.ArtifactIds)[0] {
		t.Fatalf("live result artifact = %#v, want id %q", liveResult.ResultArtifactRef, (*eventPayload.ArtifactIds)[0])
	}
}

func TestReconstructFactoryWorldState_JavaScriptFixtureReconstructsPhaseCheckpointArtifactsWithoutPetriMarking(t *testing.T) {
	t0 := time.Date(2026, 6, 8, 17, 5, 0, 0, time.UTC)
	checkpointTime := t0.Add(3 * time.Second)
	artifactTime := t0.Add(4 * time.Second)
	events := []factoryapi.FactoryEvent{
		javascriptRunRequestEvent(t0),
		orchestratorPhaseChangedEvent(1, t0.Add(2*time.Second), "plan", "", factoryapi.ACTIVE, "Planning workflow"),
		orchestratorPhaseChangedEvent(2, t0.Add(2500*time.Millisecond), "execute", "plan", factoryapi.ACTIVE, "Entered execute phase"),
		orchestratorCheckpointWrittenEvent(2, checkpointTime),
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
		t.Fatalf("javascript phase = %#v, want execute with phase history [plan execute]", worldState.JavaScriptRuntime)
	}
	if len(worldState.JavaScriptCheckpoints) != 1 || len(worldState.Artifacts) != 1 {
		t.Fatalf("checkpoints=%d artifacts=%d, want one each", len(worldState.JavaScriptCheckpoints), len(worldState.Artifacts))
	}
	checkpoint := worldState.JavaScriptCheckpoints[0]
	if checkpoint.ID != "ckpt-1" || checkpoint.ResumabilityStatus != string(factoryapi.RESUMABLE) {
		t.Fatalf("checkpoint ref = %#v, want ckpt-1 RESUMABLE", checkpoint)
	}
	if len(checkpoint.Warnings) != 1 || checkpoint.Warnings[0].Code != "checkpoint_stale_inputs" {
		t.Fatalf("checkpoint warnings = %#v, want stale input warning", checkpoint.Warnings)
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
	if len(view.Runtime.JavaScript.Phases) != 2 || view.Runtime.JavaScript.Phases[0] != "plan" {
		t.Fatalf("javascript phase history = %#v, want [plan execute]", view.Runtime.JavaScript.Phases)
	}
	if len(view.Runtime.JavaScript.Checkpoints) != 1 || len(view.Runtime.JavaScript.Artifacts) != 1 {
		t.Fatalf("javascript projection refs = %#v, want checkpoint and artifact", view.Runtime.JavaScript)
	}
	latestCheckpoint := view.Runtime.JavaScript.Checkpoints[0]
	if latestCheckpoint.ArtifactRef == nil || latestCheckpoint.ArtifactRef.Visibility != interfaces.JavaScriptCheckpointArtifactVisibility {
		t.Fatalf("latest checkpoint ref = %#v, want INTERNAL_CHECKPOINT artifact ref", latestCheckpoint)
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

func orchestratorPhaseChangedEvent(
	tick int,
	eventTime time.Time,
	phaseName string,
	previousPhaseName string,
	phaseStatus factoryapi.OrchestratorPhaseStatus,
	progressSummary string,
) factoryapi.FactoryEvent {
	sessionID := "session-js"
	kind := factoryapi.JAVASCRIPT
	phaseID := "phase-" + phaseName
	context := factoryapi.FactoryEventContext{
		SessionId:        &sessionID,
		OrchestratorKind: &kind,
		PhaseId:          stringPointer(phaseID),
		PhaseName:        stringPointer(phaseName),
	}
	payload := factoryapi.OrchestratorPhaseChangedEventPayload{
		PhaseStatus:     phaseStatus,
		StartedAt:       &eventTime,
		ProgressSummary: stringPointer(progressSummary),
	}
	if previousPhaseName != "" {
		previousPhaseID := "phase-" + previousPhaseName
		payload.PreviousPhaseId = stringPointer(previousPhaseID)
		payload.PreviousPhaseName = stringPointer(previousPhaseName)
	}
	return generatedProjectionEvent(factoryapi.FactoryEventTypeOrchestratorPhaseChanged, "orchestrator-phase-changed/"+phaseName, tick, eventTime, context, payload)
}

func orchestratorCheckpointWrittenEvent(tick int, eventTime time.Time) factoryapi.FactoryEvent {
	sessionID := "session-js"
	kind := factoryapi.JAVASCRIPT
	phaseID := "phase-execute"
	phaseName := "execute"
	checkpointID := "ckpt-1"
	hash := "sha256:checkpoint-body"
	size := int64(128)
	context := factoryapi.FactoryEventContext{
		SessionId:        &sessionID,
		OrchestratorKind: &kind,
		PhaseId:          stringPointer(phaseID),
		PhaseName:        stringPointer(phaseName),
		CheckpointId:     stringPointer(checkpointID),
	}
	payload := factoryapi.OrchestratorCheckpointWrittenEventPayload{
		Label:                 "after-plan",
		Timestamp:             &eventTime,
		SourceHash:            stringPointer("sha256:source"),
		RuntimeSnapshotDigest: stringPointer("sha256:snapshot"),
		ResumabilityStatus:    factoryapi.RESUMABLE,
		ArtifactRef: &factoryapi.FactoryArtifactRef{
			Id:          "artifact-ckpt-1",
			Kind:        factoryapi.FactoryArtifactKindCHECKPOINT,
			Visibility:  factoryapi.FactoryArtifactVisibilityINTERNALCHECKPOINT,
			ContentHash: &hash,
			SizeBytes:   &size,
		},
		Warnings: &[]factoryapi.FactoryDispatchWarning{{
			Code:    "checkpoint_stale_inputs",
			Message: "Some inputs were captured before the latest dispatch completed",
		}},
	}
	return generatedProjectionEvent(
		factoryapi.FactoryEventTypeOrchestratorCheckpointWritten,
		"orchestrator-checkpoint-written/"+checkpointID,
		tick,
		eventTime,
		context,
		payload,
	)
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
