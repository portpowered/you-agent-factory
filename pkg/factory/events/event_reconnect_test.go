package events

import (
	"testing"
	"time"

	factoryapi "github.com/portpowered/infinite-you/pkg/api/generated"
	"github.com/portpowered/infinite-you/pkg/factory/projections"
	"github.com/portpowered/infinite-you/pkg/interfaces"
)

func TestBuildReconnectReplay_AfterEventIDReturnsOnlyNewerEvents(t *testing.T) {
	events := reconnectFixtureEvents(t)

	replay, err := BuildReconnectReplay(events, interfaces.FactoryEventReconnectCursor{
		AfterEventID: "dispatch-queued/dispatch-js-1",
	}, interfaces.FactoryEventReconnectScope{SessionID: "session-js"})
	if err != nil {
		t.Fatalf("BuildReconnectReplay: %v", err)
	}
	if len(replay) != 2 {
		t.Fatalf("replay = %d events, want interrupted and reconciled only", len(replay))
	}
	if replay[0].Type != factoryapi.FactoryEventTypeDispatchInterrupted {
		t.Fatalf("first replay event = %q, want DISPATCH_INTERRUPTED", replay[0].Type)
	}
	if replay[1].Type != factoryapi.FactoryEventTypeDispatchReconciled {
		t.Fatalf("second replay event = %q, want DISPATCH_RECONCILED", replay[1].Type)
	}
}

func TestBuildReconnectReplay_AfterSessionSequenceReturnsOnlyNewerSessionEvents(t *testing.T) {
	events := reconnectFixtureEvents(t)
	sequence := 0

	replay, err := BuildReconnectReplay(events, interfaces.FactoryEventReconnectCursor{
		AfterSequence: &sequence,
	}, interfaces.FactoryEventReconnectScope{SessionID: "session-js"})
	if err != nil {
		t.Fatalf("BuildReconnectReplay: %v", err)
	}
	if len(replay) != 2 {
		t.Fatalf("replay = %d events, want two newer session events", len(replay))
	}
}

func TestBuildReconnectReplay_ReconstructsDispatchStateWithoutSessionCompleted(t *testing.T) {
	events := reconnectFixtureEvents(t)
	replay, err := BuildReconnectReplay(events, interfaces.FactoryEventReconnectCursor{
		AfterEventID: "dispatch-queued/dispatch-js-1",
	}, interfaces.FactoryEventReconnectScope{SessionID: "session-js"})
	if err != nil {
		t.Fatalf("BuildReconnectReplay: %v", err)
	}

	worldState, err := projections.ReconstructFactoryWorldState(replay, 3)
	if err != nil {
		t.Fatalf("ReconstructFactoryWorldState: %v", err)
	}
	if worldState.SessionBracket != nil && worldState.SessionBracket.Terminal {
		t.Fatalf("session bracket = %#v, want reconnect replay without SESSION_COMPLETED", worldState.SessionBracket)
	}
	if worldState.JavaScriptRuntime == nil || worldState.JavaScriptRuntime.CompletedDispatches != 1 {
		t.Fatalf("javascript runtime = %#v, want one completed dispatch from reconnect replay", worldState.JavaScriptRuntime)
	}
}

func TestBuildReconnectReplay_IdempotentWhenNoNewEvents(t *testing.T) {
	events := reconnectFixtureEvents(t)
	cursor := interfaces.FactoryEventReconnectCursor{AfterEventID: "dispatch-reconciled/dispatch-js-1"}

	first, err := BuildReconnectReplay(events, cursor, interfaces.FactoryEventReconnectScope{SessionID: "session-js"})
	if err != nil {
		t.Fatalf("first BuildReconnectReplay: %v", err)
	}
	second, err := BuildReconnectReplay(events, cursor, interfaces.FactoryEventReconnectScope{SessionID: "session-js"})
	if err != nil {
		t.Fatalf("second BuildReconnectReplay: %v", err)
	}
	if len(first) != 0 || len(second) != 0 {
		t.Fatalf("idempotent replay = %#v and %#v, want empty slices", first, second)
	}
}

func reconnectFixtureEvents(t *testing.T) []factoryapi.FactoryEvent {
	t.Helper()
	t0 := time.Date(2026, 6, 9, 15, 0, 0, 0, time.UTC)
	sessionID := "session-js"
	kind := factoryapi.JAVASCRIPT
	dispatchID := "dispatch-js-1"
	events := []factoryapi.FactoryEvent{
		reconnectFixtureEvent(factoryapi.FactoryEventTypeDispatchQueued, "dispatch-queued/"+dispatchID, 1, t0, 0, factoryapi.FactoryEventContext{
			Sequence:         0,
			SessionId:        &sessionID,
			SessionSequence:  intPtr(0),
			OrchestratorKind: &kind,
			DispatchId:       stringPtr(dispatchID),
		}, factoryapi.DispatchQueuedEventPayload{
			DispatchKind: factoryapi.FactoryDispatchKindJAVASCRIPTAGENT,
		}),
		reconnectFixtureEvent(factoryapi.FactoryEventTypeDispatchInterrupted, "dispatch-interrupted/"+dispatchID, 2, t0.Add(time.Second), 1, factoryapi.FactoryEventContext{
			Sequence:         1,
			SessionId:        &sessionID,
			SessionSequence:  intPtr(1),
			OrchestratorKind: &kind,
			DispatchId:       stringPtr(dispatchID),
		}, factoryapi.DispatchInterruptedEventPayload{
			Reason:         "provider disconnected",
			ObservedStatus: factoryapi.FactoryDispatchStatusFAILED,
			InterruptedAt:  t0.Add(time.Second),
		}),
		reconnectFixtureEvent(factoryapi.FactoryEventTypeDispatchReconciled, "dispatch-reconciled/"+dispatchID, 3, t0.Add(2*time.Second), 2, factoryapi.FactoryEventContext{
			Sequence:         2,
			SessionId:        &sessionID,
			SessionSequence:  intPtr(2),
			OrchestratorKind: &kind,
			DispatchId:       stringPtr(dispatchID),
		}, factoryapi.DispatchReconciledEventPayload{
			ReconciledStatus:     factoryapi.FactoryDispatchStatusCOMPLETED,
			ReconciliationSource: factoryapi.PROVIDERSESSION,
			Replayed:             false,
		}),
	}
	return events
}

func reconnectFixtureEvent(
	eventType factoryapi.FactoryEventType,
	id string,
	tick int,
	eventTime time.Time,
	sequence int,
	context factoryapi.FactoryEventContext,
	payload any,
) factoryapi.FactoryEvent {
	context.Tick = tick
	context.EventTime = eventTime
	context.Sequence = sequence
	event := factoryEvent(eventType, id, context, payload)
	event.SchemaVersion = factoryapi.AgentFactoryEventV1
	return event
}

func intPtr(value int) *int {
	return &value
}
