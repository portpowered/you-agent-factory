package factoryeventkinds

import (
	"encoding/json"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"

	recordings "github.com/portpowered/infinite-you/pkg/services/recordings"
	factoryapi "github.com/portpowered/infinite-you/pkg/transports/http/generated"
)

func TestPublicEmittableFactoryEventKinds_HaveRepresentativeSerializationCoverage(t *testing.T) {
	eventsByType := loadCanonicalFactoryEventFixtureByType(t)
	publicKinds := PublicEmittableFactoryEventKinds()

	if len(eventsByType) == 0 {
		t.Fatal("canonical factory event fixture is empty")
	}

	for _, entry := range publicKinds {
		event, ok := eventsByType[factoryapi.FactoryEventType(entry.Kind)]
		if !ok {
			t.Fatalf("representative serialization fixture missing public emittable kind %s", entry.Kind)
		}
		if err := RoundTripFactoryEventEnvelope(event); err != nil {
			t.Fatal(err)
		}
	}
}

func TestRoundTripFactoryEventEnvelope_NamesKindOnFailure(t *testing.T) {
	event := factoryapi.FactoryEvent{
		SchemaVersion: factoryapi.AgentFactoryEventV1,
		Id:            "event-bad-payload",
		Type:          factoryapi.FactoryEventTypeWorkRequest,
		Context: factoryapi.FactoryEventContext{
			Sequence:  1,
			Tick:      1,
			EventTime: time.Date(2026, 4, 21, 12, 0, 2, 0, time.UTC),
		},
		Payload: factoryapi.FactoryEvent_Payload{},
	}

	err := RoundTripFactoryEventEnvelope(event)
	if err == nil {
		t.Fatal("expected serialization failure for empty WORK_REQUEST payload")
	}

	serializationErr, ok := err.(FactoryEventSerializationError)
	if !ok {
		t.Fatalf("error = %T %v, want FactoryEventSerializationError", err, err)
	}
	if serializationErr.Kind != recordings.FactoryEventTypeWorkRequest {
		t.Fatalf("serialization error kind = %q, want WORK_REQUEST", serializationErr.Kind)
	}
	if !strings.Contains(err.Error(), "WORK_REQUEST") {
		t.Fatalf("serialization error = %q, want kind name WORK_REQUEST", err.Error())
	}
}

func TestPublicFactoryEventPayloadDecoders_CoverPublicEmittableInventory(t *testing.T) {
	publicKinds := PublicEmittableFactoryEventKinds()
	for _, entry := range publicKinds {
		if _, ok := publicFactoryEventPayloadDecoders[entry.Kind]; !ok {
			t.Fatalf("public emittable kind %s missing payload decoder registration", entry.Kind)
		}
	}
	if len(publicFactoryEventPayloadDecoders) != len(publicKinds) {
		t.Fatalf(
			"payload decoder count = %d, public emittable inventory = %d",
			len(publicFactoryEventPayloadDecoders),
			len(publicKinds),
		)
	}
}

func loadCanonicalFactoryEventFixtureByType(t *testing.T) map[factoryapi.FactoryEventType]factoryapi.FactoryEvent {
	t.Helper()

	data, err := os.ReadFile(canonicalFactoryEventFixturePath(t))
	if err != nil {
		t.Fatalf("read canonical factory event fixture: %v", err)
	}

	var events []factoryapi.FactoryEvent
	if err := json.Unmarshal(data, &events); err != nil {
		t.Fatalf("unmarshal canonical factory event fixture: %v", err)
	}

	eventsByType := make(map[factoryapi.FactoryEventType]factoryapi.FactoryEvent, len(events))
	for _, event := range events {
		if existing, ok := eventsByType[event.Type]; ok {
			t.Fatalf(
				"canonical fixture has duplicate representative for kind %s: %q and %q",
				event.Type,
				existing.Id,
				event.Id,
			)
		}
		eventsByType[event.Type] = event
	}
	return eventsByType
}

func canonicalFactoryEventFixturePath(t *testing.T) string {
	t.Helper()
	_, file, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("resolve test file path")
	}
	return filepath.Clean(filepath.Join(
		filepath.Dir(file),
		"../../../../../transports/http/testdata/canonical-event-vocabulary-stream.json",
	))
}
