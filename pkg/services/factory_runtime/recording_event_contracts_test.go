package factory

import (
	"encoding/json"
	"testing"
	"time"
)

func TestRunFinishedFactoryEventPublishesRuntimeRootVocabulary(t *testing.T) {
	t.Parallel()

	startedAt := time.Date(2026, 7, 27, 15, 0, 0, 0, time.UTC)
	finishedAt := startedAt.Add(time.Minute)
	event := RunFinishedFactoryEvent(startedAt, finishedAt)

	if event.Id != RunFinishedFactoryEventID {
		t.Fatalf("event id = %q, want %q", event.Id, RunFinishedFactoryEventID)
	}
	if event.Type != FactoryEventTypeRunResponse {
		t.Fatalf("event type = %q, want %q", event.Type, FactoryEventTypeRunResponse)
	}
	if event.SchemaVersion != FactoryEventSchemaVersionV1 {
		t.Fatalf("schema version = %q, want %q", event.SchemaVersion, FactoryEventSchemaVersionV1)
	}
	if !event.Context.EventTime.Equal(finishedAt) {
		t.Fatalf("event time = %v, want %v", event.Context.EventTime, finishedAt)
	}

	var payload RunResponseEventPayload
	if err := json.Unmarshal(event.Payload, &payload); err != nil {
		t.Fatalf("decode payload: %v", err)
	}
	if payload.State == nil || *payload.State != FactoryStateCompleted {
		t.Fatalf("payload state = %#v, want completed", payload.State)
	}
	if payload.WallClock == nil ||
		payload.WallClock.StartedAt == nil ||
		!payload.WallClock.StartedAt.Equal(startedAt) ||
		payload.WallClock.FinishedAt == nil ||
		!payload.WallClock.FinishedAt.Equal(finishedAt) {
		t.Fatalf("payload wall clock = %#v", payload.WallClock)
	}
}
