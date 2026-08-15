package factoryeventprojection_test

import (
	"encoding/json"
	"testing"
	"time"

	interfaces "github.com/portpowered/infinite-you/pkg/services/factory_definitions"
	workerexecution "github.com/portpowered/infinite-you/pkg/services/workers"
	factoryapi "github.com/portpowered/infinite-you/pkg/transports/http/generated"
	"github.com/portpowered/infinite-you/pkg/transports/mapping/factoryeventprojection"
)

func TestReconstructFactoryWorldState_MapsGeneratedEventsToCanonicalReducer(t *testing.T) {
	eventTime := time.Date(2026, 7, 16, 3, 0, 0, 0, time.UTC)
	raw := []byte(`{
		"context":{"eventTime":"2026-07-16T03:00:00Z","sequence":1,"tick":1},
		"id":"evt-run-response",
		"payload":{"status":"COMPLETED"},
		"schemaVersion":"agent-factory.event.v1",
		"type":"RUN_RESPONSE"
	}`)
	var event factoryapi.FactoryEvent
	if err := json.Unmarshal(raw, &event); err != nil {
		t.Fatalf("decode generated event fixture: %v", err)
	}

	var reduced []interfaces.FactoryEvent
	state, err := factoryeventprojection.ReconstructFactoryWorldState(func(events []interfaces.FactoryEvent, selectedTick int) (interfaces.FactoryWorldState, error) {
		reduced = append(reduced, events...)
		return interfaces.FactoryWorldState{Tick: selectedTick, EventTime: events[0].Context.EventTime}, nil
	}, []factoryapi.FactoryEvent{event}, 1)
	if err != nil {
		t.Fatalf("reconstruct mapped world state: %v", err)
	}
	if len(reduced) != 1 || reduced[0].Type != interfaces.FactoryEventTypeRunResponse {
		t.Fatalf("canonical reducer input = %#v, want one RUN_RESPONSE", reduced)
	}
	if !state.EventTime.Equal(eventTime) {
		t.Fatalf("event time = %s, want %s", state.EventTime, eventTime)
	}
	if state.Tick != 1 {
		t.Fatalf("tick = %d, want 1", state.Tick)
	}
}

func TestReconstructFactoryWorldState_PreservesEmptyInput(t *testing.T) {
	state, err := factoryeventprojection.ReconstructFactoryWorldState(func(events []interfaces.FactoryEvent, selectedTick int) (interfaces.FactoryWorldState, error) {
		if len(events) != 0 {
			t.Fatalf("canonical reducer events = %#v, want empty", events)
		}
		return interfaces.FactoryWorldState{Tick: selectedTick}, nil
	}, nil, 4)
	if err != nil {
		t.Fatalf("reconstruct empty world state: %v", err)
	}
	if state.Tick != 4 || state.EventTime != (time.Time{}) {
		t.Fatalf("empty state = %#v, want selected tick without event time", state)
	}
}

func TestCanonicalFactoryEventProjectsProviderSessionToContinuation(t *testing.T) {
	var event factoryapi.FactoryEvent
	if err := json.Unmarshal([]byte(`{
		"context":{"eventTime":"2026-07-16T03:00:00Z","sequence":1,"tick":1},
		"id":"evt-model-response",
		"payload":{"outcome":"FAILED","providerSession":{"provider":"antigravity","kind":"session_id","id":"session-1"}},
		"schemaVersion":"agent-factory.event.v1",
		"type":"MODEL_RESPONSE"
	}`), &event); err != nil {
		t.Fatalf("decode provider-session event: %v", err)
	}

	canonical, err := factoryeventprojection.CanonicalFactoryEvent(event)
	if err != nil {
		t.Fatalf("canonical event: %v", err)
	}
	var payload workerexecution.ModelResponseEventPayload
	if err := canonical.DecodePayload(&payload); err != nil {
		t.Fatalf("decode canonical model response: %v", err)
	}
	if payload.ProviderSession != nil {
		t.Fatalf("canonical payload retained provider session = %#v", payload.ProviderSession)
	}
	if payload.Continuation == nil || payload.Continuation.Provider != "antigravity" || payload.Continuation.ProviderSessionID != "session-1" {
		t.Fatalf("canonical continuation = %#v, want provider/session identity", payload.Continuation)
	}
}
