package factoryeventprojection_test

import (
	"encoding/json"
	"testing"
	"time"

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

	state, err := factoryeventprojection.ReconstructFactoryWorldState([]factoryapi.FactoryEvent{event}, 1)
	if err != nil {
		t.Fatalf("reconstruct mapped world state: %v", err)
	}
	if !state.EventTime.Equal(eventTime) {
		t.Fatalf("event time = %s, want %s", state.EventTime, eventTime)
	}
	if state.Tick != 1 {
		t.Fatalf("tick = %d, want 1", state.Tick)
	}
}

func TestReconstructFactoryWorldState_PreservesEmptyInput(t *testing.T) {
	state, err := factoryeventprojection.ReconstructFactoryWorldState(nil, 4)
	if err != nil {
		t.Fatalf("reconstruct empty world state: %v", err)
	}
	if state.Tick != 4 || state.EventTime != (time.Time{}) {
		t.Fatalf("empty state = %#v, want selected tick without event time", state)
	}
}
