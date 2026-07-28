package recordings_test

import (
	"encoding/json"
	"testing"
	"time"

	"github.com/portpowered/infinite-you/pkg/services/recordings"
)

func TestNewFactoryEvent_RoundTripsEnvelopeIdentityAndSerialization(t *testing.T) {
	t.Parallel()

	eventTime := time.Date(2026, 7, 28, 4, 30, 0, 0, time.UTC)
	payload, err := json.Marshal(recordings.WorkStateChangeEventPayload{
		WorkID:       "work-42",
		FromPlaceID:  "place-a",
		FromState:    "pending",
		ToPlaceID:    "place-b",
		ToState:      "processing",
		WorkTypeName: "alpha",
	})
	if err != nil {
		t.Fatalf("marshal payload: %v", err)
	}

	original := recordings.FactoryEvent{
		Id:            "event-work-state-1",
		SchemaVersion: recordings.FactoryEventSchemaVersionV1,
		Type:          recordings.FactoryEventTypeWorkStateChange,
		Context: recordings.FactoryEventContext{
			EventTime: eventTime,
			Sequence:  12,
			Tick:      3,
		},
		Payload: payload,
	}

	canonical, err := recordings.NewFactoryEvent(original)
	if err != nil {
		t.Fatalf("NewFactoryEvent: %v", err)
	}

	if canonical.Id != original.Id {
		t.Fatalf("event id = %q, want %q", canonical.Id, original.Id)
	}
	if canonical.Type != recordings.FactoryEventTypeWorkStateChange {
		t.Fatalf("event type = %q, want %q", canonical.Type, recordings.FactoryEventTypeWorkStateChange)
	}
	if canonical.SchemaVersion != recordings.FactoryEventSchemaVersionV1 {
		t.Fatalf("schema version = %q, want %q", canonical.SchemaVersion, recordings.FactoryEventSchemaVersionV1)
	}
	if !canonical.Context.EventTime.Equal(eventTime) {
		t.Fatalf("event time = %v, want %v", canonical.Context.EventTime, eventTime)
	}

	var decoded recordings.WorkStateChangeEventPayload
	if err := canonical.DecodePayload(&decoded); err != nil {
		t.Fatalf("DecodePayload: %v", err)
	}
	if decoded.WorkID != "work-42" || decoded.ToState != "processing" {
		t.Fatalf("decoded payload = %#v", decoded)
	}

	encoded, err := json.Marshal(canonical)
	if err != nil {
		t.Fatalf("marshal canonical event: %v", err)
	}
	roundTripped, err := recordings.NewFactoryEvent(json.RawMessage(encoded))
	if err != nil {
		t.Fatalf("NewFactoryEvent round trip: %v", err)
	}
	if roundTripped.Id != canonical.Id || roundTripped.Type != canonical.Type {
		t.Fatalf("round trip = %#v, want %#v", roundTripped, canonical)
	}
}
