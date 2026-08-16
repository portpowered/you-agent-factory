package http

import (
	"encoding/json"
	"testing"
	"time"

	interfaces "github.com/portpowered/infinite-you/pkg/services/factory_definitions"
	workerexecution "github.com/portpowered/infinite-you/pkg/services/workers"
	factoryapi "github.com/portpowered/infinite-you/pkg/transports/http/generated"
)

func TestReconstructFactoryWorldStateMapsGeneratedEventsToOwnerReducer(t *testing.T) {
	eventTime := time.Date(2026, 7, 16, 3, 0, 0, 0, time.UTC)
	var event factoryapi.FactoryEvent
	if err := json.Unmarshal([]byte(`{
		"context":{"eventTime":"2026-07-16T03:00:00Z","sequence":1,"tick":1},
		"id":"evt-run-response","payload":{"status":"COMPLETED"},
		"schemaVersion":"agent-factory.event.v1","type":"RUN_RESPONSE"
	}`), &event); err != nil {
		t.Fatalf("decode generated event fixture: %v", err)
	}

	var reduced []interfaces.FactoryEvent
	state, err := ReconstructFactoryWorldState(func(events []interfaces.FactoryEvent, selectedTick int) (interfaces.FactoryWorldState, error) {
		reduced = append(reduced, events...)
		return interfaces.FactoryWorldState{Tick: selectedTick, EventTime: events[0].Context.EventTime}, nil
	}, []factoryapi.FactoryEvent{event}, 1)
	if err != nil {
		t.Fatalf("reconstruct mapped world state: %v", err)
	}
	if len(reduced) != 1 || reduced[0].Type != interfaces.FactoryEventTypeRunResponse || !state.EventTime.Equal(eventTime) || state.Tick != 1 {
		t.Fatalf("reduced=%#v state=%#v, want one RUN_RESPONSE at tick 1", reduced, state)
	}
}

func TestReconstructFactoryWorldStatePreservesEmptyInput(t *testing.T) {
	state, err := ReconstructFactoryWorldState(func(events []interfaces.FactoryEvent, selectedTick int) (interfaces.FactoryWorldState, error) {
		if len(events) != 0 {
			t.Fatalf("canonical reducer events = %#v, want empty", events)
		}
		return interfaces.FactoryWorldState{Tick: selectedTick}, nil
	}, nil, 4)
	if err != nil || state.Tick != 4 || !state.EventTime.IsZero() {
		t.Fatalf("state=%#v err=%v, want selected tick without event time", state, err)
	}
}

func TestCanonicalFactoryEventProjectsProviderSessionToContinuation(t *testing.T) {
	var event factoryapi.FactoryEvent
	if err := json.Unmarshal([]byte(`{
		"context":{"eventTime":"2026-07-16T03:00:00Z","sequence":1,"tick":1},
		"id":"evt-model-response","payload":{"outcome":"FAILED","providerSession":{"provider":"antigravity","kind":"session_id","id":"session-1"}},
		"schemaVersion":"agent-factory.event.v1","type":"MODEL_RESPONSE"
	}`), &event); err != nil {
		t.Fatalf("decode provider-session event: %v", err)
	}

	canonical, err := CanonicalFactoryEvent(event)
	if err != nil {
		t.Fatalf("canonical event: %v", err)
	}
	var payload workerexecution.ModelResponseEventPayload
	if err := canonical.DecodePayload(&payload); err != nil {
		t.Fatalf("decode canonical model response: %v", err)
	}
	if payload.ProviderSession != nil || payload.Continuation == nil || payload.Continuation.Provider != "antigravity" || payload.Continuation.ProviderSessionID != "session-1" {
		t.Fatalf("canonical payload = %#v, want provider continuation", payload)
	}
}

func TestCanonicalFactoryEventRejectsMalformedExecutionPayload(t *testing.T) {
	event := factoryapi.FactoryEvent{Type: factoryapi.FactoryEventTypeModelResponse}
	encoded, err := json.Marshal(map[string]any{
		"context": event.Context, "id": "malformed", "payload": "not an object",
		"schemaVersion": event.SchemaVersion, "type": event.Type,
	})
	if err != nil {
		t.Fatalf("marshal malformed event: %v", err)
	}
	if err := json.Unmarshal(encoded, &event); err != nil {
		t.Fatalf("decode malformed event: %v", err)
	}
	if _, err := CanonicalFactoryEvent(event); err == nil {
		t.Fatal("CanonicalFactoryEvent() error = nil, want malformed payload error")
	}
}
