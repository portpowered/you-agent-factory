package events

import (
	"context"
	"testing"
	"time"

	interfaces "github.com/portpowered/infinite-you/pkg/services/factory_definitions"
)

func TestFactoryEventHistory_RecordDispatchWorkerSessionAssociation_RecordsCanonicalAssociation(t *testing.T) {
	eventTime := time.Date(2026, 4, 22, 16, 0, 0, 0, time.UTC)
	history := newTestFactoryEventHistory(eventHistoryProjectionNet(), func() time.Time { return time.Unix(0, 0).UTC() })

	history.RecordDispatchWorkerSessionAssociation(4, "dispatch-1", "worker-session-1", eventTime)

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

func TestFactoryEventHistory_RecordDispatchWorkerSessionAssociation_IgnoresIncompleteIdentities(t *testing.T) {
	eventTime := time.Date(2026, 4, 22, 16, 0, 0, 0, time.UTC)
	history := newTestFactoryEventHistory(eventHistoryProjectionNet(), func() time.Time { return time.Unix(0, 0).UTC() })

	history.RecordDispatchWorkerSessionAssociation(4, "", "worker-session-1", eventTime)
	history.RecordDispatchWorkerSessionAssociation(4, "dispatch-1", "", eventTime)

	if got := len(history.CanonicalEvents()); got != 0 {
		t.Fatalf("canonical event count = %d, want 0 for incomplete association identities", got)
	}
}

func TestFactoryEventHistory_RecordDispatchWorkerSessionAssociation_NilHistoryDoesNotPanic(t *testing.T) {
	var history *FactoryEventHistory
	history.RecordDispatchWorkerSessionAssociation(4, "dispatch-1", "worker-session-1", time.Now())
}
