package projections_test

import (
	"testing"
	"time"

	factoryapi "github.com/portpowered/infinite-you/pkg/api/generated"
	factoryevents "github.com/portpowered/infinite-you/pkg/factory/events"
	. "github.com/portpowered/infinite-you/pkg/factory/projections"
	"github.com/portpowered/infinite-you/pkg/interfaces"
)

func TestReconnectReplay_ReconstructsSessionLifecyclePhaseDispatchAndResultWithoutTerminalCompletion(t *testing.T) {
	t0 := time.Date(2026, 6, 9, 15, 30, 0, 0, time.UTC)
	events := reconnectSessionLifecycleEvents(t0)
	replay, err := factoryevents.BuildReconnectReplay(events, interfaces.FactoryEventReconnectCursor{
		AfterEventID: "orchestrator-phase-changed/execute",
	}, interfaces.FactoryEventReconnectScope{SessionID: "session-js"})
	if err != nil {
		t.Fatalf("BuildReconnectReplay: %v", err)
	}
	if len(replay) < 3 {
		t.Fatalf("replay = %d events, want dispatch reconciled, artifact, and partial result", len(replay))
	}

	ackIndex := 1
	merged := append(append([]factoryapi.FactoryEvent{}, events[:ackIndex+1]...), replay...)
	worldState, err := ReconstructFactoryWorldState(merged, 4)
	if err != nil {
		t.Fatalf("ReconstructFactoryWorldState: %v", err)
	}
	if worldState.SessionBracket == nil || worldState.SessionBracket.Terminal {
		t.Fatalf("session bracket = %#v, want non-terminal reconnect view", worldState.SessionBracket)
	}
	if worldState.SessionBracket.ResultStatus != string(factoryapi.PARTIAL) {
		t.Fatalf("result status = %q, want PARTIAL from reconnect replay", worldState.SessionBracket.ResultStatus)
	}
	if worldState.JavaScriptRuntime == nil || worldState.JavaScriptRuntime.CompletedDispatches != 1 {
		t.Fatalf("javascript runtime = %#v, want one completed dispatch", worldState.JavaScriptRuntime)
	}
	if len(worldState.Artifacts) != 1 {
		t.Fatalf("artifacts = %#v, want one artifact ref", worldState.Artifacts)
	}

	view := BuildFactoryWorldView(worldState)
	if view.Runtime.JavaScript == nil || view.Runtime.JavaScript.Phase != "execute" {
		t.Fatalf("javascript projection = %#v, want execute phase after merging ack state with reconnect replay", view.Runtime.JavaScript)
	}
}

func reconnectSessionLifecycleEvents(t0 time.Time) []factoryapi.FactoryEvent {
	sessionID := "session-js"
	kind := factoryapi.JAVASCRIPT
	source := "replay"
	return []factoryapi.FactoryEvent{
		generatedProjectionEvent(factoryapi.FactoryEventTypeSessionStarted, "factory-event/session-started", 0, t0, factoryapi.FactoryEventContext{
			Sequence:         0,
			SessionId:        &sessionID,
			SessionSequence:  intPtrForProjectionTest(0),
			OrchestratorKind: &kind,
			Source:           &source,
		}, factoryapi.SessionStartedEventPayload{
			FactoryId: stringPointer("factory-alpha"),
			StartedAt: t0,
		}),
		orchestratorPhaseChangedEvent(1, t0.Add(time.Second), "execute", "plan", factoryapi.ACTIVE, "running execute phase"),
		dispatchQueuedEvent(2, t0.Add(2*time.Second)),
		dispatchReconciledEvent(3, t0.Add(3*time.Second)),
		javascriptArtifactCreatedEvent(3, t0.Add(4*time.Second)),
		generatedProjectionEvent(factoryapi.FactoryEventTypeSessionResultUpdated, "factory-event/session-result-updated/partial", 4, t0.Add(5*time.Second), factoryapi.FactoryEventContext{
			Sequence:         5,
			SessionId:        &sessionID,
			SessionSequence:  intPtrForProjectionTest(5),
			OrchestratorKind: &kind,
			Source:           &source,
		}, factoryapi.SessionResultUpdatedEventPayload{
			ResultStatus: factoryapi.PARTIAL,
			ArtifactIds:  &[]string{"artifact-result-1"},
		}),
	}
}
