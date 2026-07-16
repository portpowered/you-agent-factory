package factoryevent_test

import (
	"encoding/json"
	"testing"
	"time"

	interfaces "github.com/portpowered/infinite-you/pkg/factory/contracts"
	"github.com/portpowered/infinite-you/pkg/transports/mapping/factoryevent"
)

func TestSliceToAPIPreservesCanonicalEnvelopeAndPayload(t *testing.T) {
	event := interfaces.FactoryEvent{
		Context: interfaces.FactoryEventContext{EventTime: time.Date(2026, 7, 16, 6, 30, 0, 0, time.UTC), Sequence: 7, Tick: 3},
		Id:      "factory-event/run-finished", Payload: json.RawMessage(`{"durationMs":42}`),
		SchemaVersion: interfaces.FactoryEventSchemaVersionV1, Type: interfaces.FactoryEventTypeRunResponse,
	}
	mapped, err := factoryevent.SliceToAPI([]interfaces.FactoryEvent{event})
	if err != nil {
		t.Fatalf("SliceToAPI: %v", err)
	}
	if len(mapped) != 1 || mapped[0].Id != event.Id || string(mapped[0].Type) != string(event.Type) || mapped[0].Context.Sequence != 7 {
		t.Fatalf("mapped event = %#v, want canonical envelope", mapped)
	}
	wire, err := json.Marshal(mapped[0])
	if err != nil {
		t.Fatalf("encode mapped event: %v", err)
	}
	var envelope struct {
		Payload map[string]any `json:"payload"`
	}
	if err := json.Unmarshal(wire, &envelope); err != nil || envelope.Payload["durationMs"] != float64(42) {
		t.Fatalf("mapped payload = %#v, err %v", envelope.Payload, err)
	}
}

func TestSliceToAPIRejectsMalformedCanonicalPayload(t *testing.T) {
	_, err := factoryevent.SliceToAPI([]interfaces.FactoryEvent{{
		Id: "factory-event/run-finished", Payload: json.RawMessage(`{"durationMs":`),
		SchemaVersion: interfaces.FactoryEventSchemaVersionV1, Type: interfaces.FactoryEventTypeRunResponse,
	}})
	if err == nil {
		t.Fatal("SliceToAPI malformed payload error = nil")
	}
}
