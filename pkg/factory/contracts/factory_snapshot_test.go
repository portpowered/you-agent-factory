package factorycontracts

import (
	"encoding/json"
	"testing"
	"time"
)

func TestFactorySnapshotPreservesUnknownFieldsAndCloneIsolation(t *testing.T) {
	t.Parallel()

	source := map[string]any{
		"name": "factory-a",
		"futureField": map[string]any{
			"enabled": true,
		},
	}
	snapshot, err := NewFactorySnapshot(source)
	if err != nil {
		t.Fatalf("NewFactorySnapshot: %v", err)
	}
	clone := snapshot.Clone()
	(*snapshot)[0] = '['

	var decoded map[string]any
	if err := clone.Decode(&decoded); err != nil {
		t.Fatalf("Decode clone: %v", err)
	}
	if decoded["name"] != "factory-a" {
		t.Fatalf("name = %#v, want factory-a", decoded["name"])
	}
	future, ok := decoded["futureField"].(map[string]any)
	if !ok || future["enabled"] != true {
		t.Fatalf("futureField = %#v, want preserved unknown object", decoded["futureField"])
	}
}

func TestFactoryEventEnvelopeDetachesPayloadAndPreservesUnknownFields(t *testing.T) {
	t.Parallel()

	payload := map[string]any{
		"workId": "work-1",
		"future": map[string]any{"enabled": true},
	}
	boundary := struct {
		Context       FactoryEventContext       `json:"context"`
		ID            string                    `json:"id"`
		Payload       map[string]any            `json:"payload"`
		SchemaVersion FactoryEventSchemaVersion `json:"schemaVersion"`
		Type          FactoryEventType          `json:"type"`
	}{
		Context: FactoryEventContext{
			EventTime: time.Date(2026, 7, 15, 18, 0, 0, 0, time.UTC),
			Sequence:  4,
			Tick:      3,
		},
		ID:            "event-4",
		Payload:       payload,
		SchemaVersion: FactoryEventSchemaVersionV1,
		Type:          FactoryEventTypeWorkStateChange,
	}

	event, err := NewFactoryEvent(boundary)
	if err != nil {
		t.Fatalf("NewFactoryEvent() error = %v", err)
	}
	payload["workId"] = "mutated"
	delete(payload, "future")

	var decoded map[string]any
	if err := event.DecodePayload(&decoded); err != nil {
		t.Fatalf("DecodePayload() error = %v", err)
	}
	if decoded["workId"] != "work-1" {
		t.Fatalf("detached workId = %#v, want work-1", decoded["workId"])
	}
	if _, ok := decoded["future"]; !ok {
		t.Fatalf("decoded payload = %#v, want unknown future field preserved", decoded)
	}

	encoded, err := json.Marshal(event)
	if err != nil {
		t.Fatalf("Marshal() error = %v", err)
	}
	var roundTrip map[string]json.RawMessage
	if err := json.Unmarshal(encoded, &roundTrip); err != nil {
		t.Fatalf("Unmarshal() error = %v", err)
	}
	if string(roundTrip["id"]) != `"event-4"` || string(roundTrip["schemaVersion"]) != `"agent-factory.event.v1"` {
		t.Fatalf("round-trip envelope = %s", encoded)
	}
}

func TestFactorySnapshotMarshalsAsFactoryObject(t *testing.T) {
	t.Parallel()

	snapshot, err := NewFactorySnapshot(map[string]any{"name": "factory-a"})
	if err != nil {
		t.Fatalf("NewFactorySnapshot: %v", err)
	}
	encoded, err := json.Marshal(struct {
		Factory *FactorySnapshot `json:"factory"`
	}{Factory: snapshot})
	if err != nil {
		t.Fatalf("Marshal: %v", err)
	}
	if got, want := string(encoded), `{"factory":{"name":"factory-a"}}`; got != want {
		t.Fatalf("Marshal = %s, want %s", got, want)
	}
}

func TestFactorySnapshotRejectsNonObjectJSON(t *testing.T) {
	t.Parallel()

	if _, err := NewFactorySnapshot([]string{"not", "a", "factory"}); err == nil {
		t.Fatal("NewFactorySnapshot(non-object) error = nil, want actionable validation error")
	}
	var snapshot FactorySnapshot
	if err := json.Unmarshal([]byte(`null`), &snapshot); err == nil {
		t.Fatal("UnmarshalJSON(null) error = nil, want Factory object validation error")
	}
}

func TestFactorySnapshotWithNamePreservesUnknownFieldsAndDetaches(t *testing.T) {
	t.Parallel()

	snapshot := FactorySnapshot(`{"name":"old","future":{"enabled":true}}`)
	updated, err := snapshot.WithName("alpha")
	if err != nil {
		t.Fatalf("WithName: %v", err)
	}
	var got struct {
		Name   string          `json:"name"`
		Future json.RawMessage `json:"future"`
	}
	if err := updated.Decode(&got); err != nil {
		t.Fatalf("Decode: %v", err)
	}
	if got.Name != "alpha" {
		t.Fatalf("name = %q, want alpha", got.Name)
	}
	if string(got.Future) != `{"enabled":true}` {
		t.Fatalf("future = %s", got.Future)
	}
	if string(snapshot) != `{"name":"old","future":{"enabled":true}}` {
		t.Fatalf("source snapshot mutated: %s", snapshot)
	}
}

func TestFactorySnapshotWithNameRejectsNilSnapshot(t *testing.T) {
	t.Parallel()

	var snapshot *FactorySnapshot
	if _, err := snapshot.WithName("alpha"); err == nil {
		t.Fatal("WithName: expected error")
	}
}
