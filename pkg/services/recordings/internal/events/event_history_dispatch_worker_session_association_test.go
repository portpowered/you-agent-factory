package events

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	interfaces "github.com/portpowered/infinite-you/pkg/services/factory_definitions"
	"github.com/portpowered/infinite-you/pkg/services/recordings"
)

func TestFactoryEventHistory_RecordDispatchWorkerSessionAssociation_RecordsCanonicalAssociation(t *testing.T) {
	eventTime := time.Date(2026, 4, 22, 16, 0, 0, 0, time.UTC)
	history := newTestFactoryEventHistory(eventHistoryProjectionNet(), func() time.Time { return time.Unix(0, 0).UTC() })

	history.RecordDispatchWorkerSessionAssociation(4, "dispatch-1", "worker-session-1", "turn-1", eventTime)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	stream, err := history.Subscribe(ctx, nil, interfaces.FactoryEventReconnectScope{})
	if err != nil {
		t.Fatalf("subscribe canonical events: %v", err)
	}
	if len(stream.History) != 1 {
		t.Fatalf("canonical history count = %d, want 1", len(stream.History))
	}

	event := stream.History[0]
	if event.Type != interfaces.FactoryEventTypeDispatchWorkerSessionAssoc {
		t.Fatalf("event type = %s, want %s", event.Type, interfaces.FactoryEventTypeDispatchWorkerSessionAssoc)
	}
	if event.Context.DispatchID == nil || *event.Context.DispatchID != "dispatch-1" {
		t.Fatalf("context dispatchId = %v, want dispatch-1", event.Context.DispatchID)
	}
	if event.Context.RequestID == nil || *event.Context.RequestID != "turn-1" {
		t.Fatalf("context requestId = %v, want turn-1", event.Context.RequestID)
	}
	if !event.Context.EventTime.Equal(eventTime) {
		t.Fatalf("context eventTime = %s, want %s", event.Context.EventTime, eventTime)
	}

	var payload interfaces.DispatchWorkerSessionAssociationEventPayload
	if err := event.DecodePayload(&payload); err != nil {
		t.Fatalf("decode dispatch worker session association payload: %v", err)
	}
	if payload.WorkerSessionID != "worker-session-1" {
		t.Fatalf("payload workerSessionId = %q, want worker-session-1", payload.WorkerSessionID)
	}
}

func TestFactoryEventHistory_RecordDispatchWorkerSessionAssociationWithExecution_RetainsReplayFactsWithoutWideningPublicPayload(t *testing.T) {
	eventTime := time.Date(2026, 4, 22, 16, 0, 0, 0, time.UTC)
	history := newTestFactoryEventHistory(eventHistoryProjectionNet(), func() time.Time { return time.Unix(0, 0).UTC() })

	history.RecordDispatchWorkerSessionAssociationWithExecution(
		4,
		"dispatch-model",
		"worker-session-model",
		"turn-model",
		recordings.DispatchWorkerSessionExecutionFacts{Model: "gpt-5.6-luna", ReasoningEffort: "high"},
		eventTime,
	)

	event := history.CanonicalEvents()[0]
	var replayPayload struct {
		WorkerSessionID string `json:"workerSessionId"`
		Model           string `json:"model"`
		ReasoningEffort string `json:"reasoningEffort"`
	}
	if err := json.Unmarshal(event.Payload, &replayPayload); err != nil {
		t.Fatalf("decode replay association payload: %v", err)
	}
	if replayPayload.WorkerSessionID != "worker-session-model" || replayPayload.Model != "gpt-5.6-luna" || replayPayload.ReasoningEffort != "high" {
		t.Fatalf("replay association payload = %#v, want worker session and execution facts", replayPayload)
	}

	var publicPayload interfaces.DispatchWorkerSessionAssociationEventPayload
	if err := event.DecodePayload(&publicPayload); err != nil {
		t.Fatalf("decode public association payload: %v", err)
	}
	if publicPayload.WorkerSessionID != "worker-session-model" {
		t.Fatalf("public association payload workerSessionId = %q, want worker-session-model", publicPayload.WorkerSessionID)
	}
}

func TestFactoryEventHistory_RecordDispatchWorkerSessionAssociation_IgnoresIncompleteIdentities(t *testing.T) {
	eventTime := time.Date(2026, 4, 22, 16, 0, 0, 0, time.UTC)
	history := newTestFactoryEventHistory(eventHistoryProjectionNet(), func() time.Time { return time.Unix(0, 0).UTC() })

	history.RecordDispatchWorkerSessionAssociation(4, "", "worker-session-1", "turn-1", eventTime)
	history.RecordDispatchWorkerSessionAssociation(4, "dispatch-1", "", "turn-1", eventTime)

	if got := len(history.CanonicalEvents()); got != 0 {
		t.Fatalf("canonical event count = %d, want 0 for incomplete association identities", got)
	}
}

func TestFactoryEventHistory_RecordDispatchWorkerSessionAssociation_NilHistoryDoesNotPanic(t *testing.T) {
	var history *FactoryEventHistory
	history.RecordDispatchWorkerSessionAssociation(4, "dispatch-1", "worker-session-1", "turn-1", time.Now())
}
