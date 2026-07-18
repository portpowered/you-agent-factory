package contractstaging_test

import (
	"bytes"
	"encoding/json"
	"testing"

	"github.com/portpowered/infinite-you/internal/contractstaging"
	"github.com/portpowered/infinite-you/internal/testpath"
)

const (
	factoryEventSchemaPath     = "packages/api/generated/schemas/factory-event.schema.json"
	factoryRecordingSchemaPath = "packages/api/generated/schemas/factory-recording.schema.json"
)

func TestStandaloneFactorySchemasValidateCanonicalEventAndRecordingShapes(t *testing.T) {
	repositoryRoot := testpath.MustRepoPathFromCaller(t, 0)
	artifacts, err := contractstaging.Artifacts(repositoryRoot)
	if err != nil {
		t.Fatalf("Artifacts() error = %v", err)
	}

	validEvent := map[string]any{
		"schemaVersion": "agent-factory.event.v1",
		"id":            "event-1",
		"type":          "INITIAL_STRUCTURE_REQUEST",
		"context": map[string]any{
			"sequence":  json.Number("0"),
			"tick":      json.Number("0"),
			"eventTime": "2026-07-18T05:00:00Z",
			"sessionId": "session-1",
		},
		"payload": map[string]any{
			"factory": map[string]any{"name": "recording-test"},
		},
	}
	validRecording := map[string]any{
		"schemaVersion": "agent-factory.recording.v1",
		"sessionId":     "session-1",
		"events":        []any{validEvent},
	}

	eventSchema := compileArtifactSchema(t, artifacts[factoryEventSchemaPath])
	recordingSchema := compileArtifactSchema(t, artifacts[factoryRecordingSchemaPath])
	assertSchemaValueValid(t, eventSchema, validEvent, true)
	assertSchemaValueValid(t, recordingSchema, validRecording, true)

	missingEventID := cloneJSONValue(t, validEvent).(map[string]any)
	delete(missingEventID, "id")
	assertSchemaValueValid(t, eventSchema, missingEventID, false)

	unsupportedEventVersion := cloneJSONValue(t, validEvent).(map[string]any)
	unsupportedEventVersion["schemaVersion"] = "agent-factory.event.v2"
	assertSchemaValueValid(t, eventSchema, unsupportedEventVersion, false)

	missingSessionID := cloneJSONValue(t, validRecording).(map[string]any)
	delete(missingSessionID, "sessionId")
	assertSchemaValueValid(t, recordingSchema, missingSessionID, false)

	unsupportedRecordingVersion := cloneJSONValue(t, validRecording).(map[string]any)
	unsupportedRecordingVersion["schemaVersion"] = "agent-factory.recording.v2"
	assertSchemaValueValid(t, recordingSchema, unsupportedRecordingVersion, false)

	malformedEventRecording := cloneJSONValue(t, validRecording).(map[string]any)
	events := malformedEventRecording["events"].([]any)
	delete(events[0].(map[string]any), "payload")
	assertSchemaValueValid(t, recordingSchema, malformedEventRecording, false)
}

func TestStandaloneFactorySchemasAreCompleteAndByteStable(t *testing.T) {
	repositoryRoot := testpath.MustRepoPathFromCaller(t, 0)
	first, err := contractstaging.Artifacts(repositoryRoot)
	if err != nil {
		t.Fatalf("first Artifacts() error = %v", err)
	}
	second, err := contractstaging.Artifacts(repositoryRoot)
	if err != nil {
		t.Fatalf("second Artifacts() error = %v", err)
	}

	for _, path := range []string{factoryEventSchemaPath, factoryRecordingSchemaPath} {
		if !bytes.Equal(first[path], second[path]) {
			t.Fatalf("repeated generation changed %s", path)
		}
	}

	event := decodeSchemaObject(t, first[factoryEventSchemaPath])
	discriminator := event["discriminator"].(map[string]any)
	mapping := discriminator["mapping"].(map[string]any)
	if discriminator["propertyName"] != "type" || len(mapping) != 31 {
		t.Fatalf("FactoryEvent discriminator = %#v, want type with 31 mappings", discriminator)
	}
	if variants := event["oneOf"].([]any); len(variants) != 31 {
		t.Fatalf("FactoryEvent variants = %d, want 31", len(variants))
	}

	recording := decodeSchemaObject(t, first[factoryRecordingSchemaPath])
	definitions := recording["$defs"].(map[string]any)
	if _, ok := definitions["FactoryEvent"]; !ok {
		t.Fatal("FactoryRecording schema has no canonical FactoryEvent definition")
	}
	if bytes.Contains(first[factoryRecordingSchemaPath], []byte("#/components/schemas/")) {
		t.Fatal("FactoryRecording schema retains OpenAPI-only component references")
	}
}

func assertSchemaValueValid(t *testing.T, schema interface{ Validate(any) error }, value any, valid bool) {
	t.Helper()
	err := schema.Validate(value)
	if valid && err != nil {
		t.Fatalf("valid value rejected: %v", err)
	}
	if !valid && err == nil {
		t.Fatal("invalid value accepted")
	}
}

func cloneJSONValue(t *testing.T, value any) any {
	t.Helper()
	payload, err := json.Marshal(value)
	if err != nil {
		t.Fatalf("marshal clone: %v", err)
	}
	var clone any
	decoder := json.NewDecoder(bytes.NewReader(payload))
	decoder.UseNumber()
	if err := decoder.Decode(&clone); err != nil {
		t.Fatalf("decode clone: %v", err)
	}
	return clone
}

func decodeSchemaObject(t *testing.T, payload []byte) map[string]any {
	t.Helper()
	var document map[string]any
	if err := json.Unmarshal(payload, &document); err != nil {
		t.Fatalf("decode schema: %v", err)
	}
	return document
}
