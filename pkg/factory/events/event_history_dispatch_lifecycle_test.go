package events

import (
	"testing"
	"time"

	factoryapi "github.com/portpowered/infinite-you/pkg/api/generated"
	"github.com/portpowered/infinite-you/pkg/factory/projections"
)

func TestFactoryEventHistory_RecordDispatchLifecycle_EmitsReconstructableQueueInterruptReconcileAndArtifactSequence(t *testing.T) {
	t0 := time.Date(2026, 6, 9, 14, 0, 0, 0, time.UTC)
	queuedAt := t0.Add(2 * time.Second)
	interruptedAt := t0.Add(3 * time.Second)
	reconciledAt := t0.Add(4 * time.Second)
	artifactAt := t0.Add(5 * time.Second)
	history := NewFactoryEventHistory(nil, func() time.Time { return t0 })
	kind := factoryapi.JAVASCRIPT
	queuePosition := 0
	hash := "sha256:result-body"
	size := int64(512)
	label := "Review summary"
	summary := "Completed review findings"

	history.RecordDispatchQueued(DispatchQueuedInput{
		SessionID:        "session-js",
		OrchestratorKind: kind,
		PhaseID:          "phase-execute",
		PhaseName:        "execute",
		DispatchID:       "dispatch-js-1",
		Source:           "runtime",
		Tick:             2,
		DispatchKind:     factoryapi.FactoryDispatchKindJAVASCRIPTAGENT,
		Label:            "summarize findings",
		RunnerID:         "cursor",
		Model:            "gpt-4.1",
		Provider:         "openai",
		QueuePosition:    &queuePosition,
		PromptDigest:     "sha256:prompt",
		SchemaDigest:     "sha256:schema",
		InputWorkIDs:     []string{"work-1"},
	}, queuedAt)
	history.RecordDispatchInterrupted(DispatchInterruptedInput{
		SessionID:        "session-js",
		OrchestratorKind: kind,
		PhaseID:          "phase-execute",
		PhaseName:        "execute",
		DispatchID:       "dispatch-js-1",
		Source:           "runtime",
		Tick:             2,
		Reason:           "provider disconnected",
		ObservedStatus:   factoryapi.FactoryDispatchStatusFAILED,
		RetryPlanned:     true,
	}, interruptedAt)
	history.RecordDispatchReconciled(DispatchReconciledInput{
		SessionID:            "session-js",
		OrchestratorKind:     kind,
		PhaseID:              "phase-execute",
		PhaseName:            "execute",
		DispatchID:           "dispatch-js-1",
		Source:               "replay",
		Tick:                 3,
		ReconciledStatus:     factoryapi.FactoryDispatchStatusCOMPLETED,
		ReconciliationSource: factoryapi.PROVIDERSESSION,
		Replayed:             true,
		ArtifactIDs:          []string{"artifact-result-1"},
	}, reconciledAt)
	history.RecordArtifactCreated(ArtifactCreatedInput{
		SessionID:        "session-js",
		OrchestratorKind: kind,
		PhaseID:          "phase-execute",
		PhaseName:        "execute",
		DispatchID:       "dispatch-js-1",
		Source:           "runtime",
		Tick:             3,
		Artifact: factoryapi.FactoryArtifact{
			Id:          "artifact-result-1",
			Kind:        factoryapi.FactoryArtifactKindFINALRESULT,
			Visibility:  factoryapi.FactoryArtifactVisibilityPUBLIC,
			Label:       &label,
			Summary:     &summary,
			ContentHash: &hash,
			SizeBytes:   &size,
		},
		CapturedAt: &artifactAt,
	}, artifactAt)

	events := history.Events()
	if len(events) != 4 {
		t.Fatalf("events = %d, want queued, interrupted, reconciled, artifact", len(events))
	}
	assertDispatchLifecycleEventType(t, events[0], factoryapi.FactoryEventTypeDispatchQueued)
	assertDispatchLifecycleEventType(t, events[1], factoryapi.FactoryEventTypeDispatchInterrupted)
	assertDispatchLifecycleEventType(t, events[2], factoryapi.FactoryEventTypeDispatchReconciled)
	assertDispatchLifecycleEventType(t, events[3], factoryapi.FactoryEventTypeArtifactCreated)

	worldState, err := projections.ReconstructFactoryWorldState(events, 3)
	if err != nil {
		t.Fatalf("ReconstructFactoryWorldState: %v", err)
	}
	if worldState.JavaScriptRuntime == nil {
		t.Fatal("javascript runtime = nil, want dispatch lifecycle projection")
	}
	if worldState.JavaScriptRuntime.QueuedDispatches != 0 || worldState.JavaScriptRuntime.CompletedDispatches != 1 {
		t.Fatalf("dispatch counts = queued:%d completed:%d, want queued=0 completed=1",
			worldState.JavaScriptRuntime.QueuedDispatches, worldState.JavaScriptRuntime.CompletedDispatches)
	}
	if len(worldState.JavaScriptRuntime.Dispatches) != 1 {
		t.Fatalf("dispatches = %#v, want one reconciled dispatch", worldState.JavaScriptRuntime.Dispatches)
	}
	dispatch := worldState.JavaScriptRuntime.Dispatches[0]
	if dispatch.ID != "dispatch-js-1" || dispatch.Status != string(factoryapi.FactoryDispatchStatusCOMPLETED) {
		t.Fatalf("dispatch = %#v, want dispatch-js-1 COMPLETED", dispatch)
	}
	if dispatch.PromptDigest != "sha256:prompt" || dispatch.Label != "summarize findings" {
		t.Fatalf("dispatch metadata = %#v, want prompt digest and label preserved", dispatch)
	}
	if len(worldState.Artifacts) != 1 || worldState.Artifacts[0].ID != "artifact-result-1" {
		t.Fatalf("artifacts = %#v, want artifact-result-1", worldState.Artifacts)
	}
}

func assertDispatchLifecycleEventType(
	t *testing.T,
	event factoryapi.FactoryEvent,
	wantType factoryapi.FactoryEventType,
) {
	t.Helper()
	if event.Type != wantType {
		t.Fatalf("event type = %q, want %q", event.Type, wantType)
	}
	if event.Context.SessionId == nil || *event.Context.SessionId != "session-js" {
		t.Fatalf("session id = %#v, want session-js", event.Context.SessionId)
	}
	if event.Context.DispatchId == nil || *event.Context.DispatchId != "dispatch-js-1" {
		t.Fatalf("dispatch id = %#v, want dispatch-js-1 for %s", event.Context.DispatchId, wantType)
	}
}
