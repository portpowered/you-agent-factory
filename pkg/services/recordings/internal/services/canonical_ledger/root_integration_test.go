package canonicalledger_test

import (
	"context"
	"errors"
	"testing"
	"time"

	factorydefinitions "github.com/portpowered/infinite-you/pkg/services/factory_definitions"
	recordings "github.com/portpowered/infinite-you/pkg/services/recordings"
	recordingsservice "github.com/portpowered/infinite-you/pkg/services/recordings/service"
)

func TestAcceptedRecordingsRootUsesPrivateCanonicalLedger(t *testing.T) {
	t.Parallel()

	ledger := &stubLedger{}
	root := recordingsservice.NewService(ledger, recordingsservice.NewProjectionService())
	if root == nil {
		t.Fatal("NewService returned nil")
	}

	event := recordings.CanonicalEvent{
		ID:         "evt-private-ledger",
		Scope:      recordings.CanonicalEventScope{FactorySessionID: "session-1"},
		RecordedAt: time.Unix(1_700_000_000, 0).UTC(),
		Kind:       recordings.CanonicalEventKind(factorydefinitions.FactoryEventTypeWorkRequest),
		Payload:    `{"work":"one"}`,
	}
	accepted, err := root.Append(recordings.AppendRecordedEventRequest{Event: event})
	if err != nil {
		t.Fatalf("Append valid event: %v", err)
	}
	if len(ledger.events) != 1 || ledger.events[0].Id != "evt-private-ledger" {
		t.Fatalf("Append did not delegate to ledger: %#v", ledger.events)
	}
	if accepted.Event.Cursor.StreamGenerationID != "gen-1" {
		t.Fatalf("Append accepted cursor = %#v, want gen-1 generation", accepted.Event.Cursor)
	}

	ledger.subscribeStream = factorydefinitions.FactoryEventStream{
		StreamGenerationID: "gen-1",
		History: []factorydefinitions.FactoryEvent{
			scopedLegacyEvent("session-1/0", 0, 0, "session-1"),
		},
	}
	subscribed, err := root.SubscribeFrom(context.Background(), recordings.SubscribeRequest{
		Scope: recordings.CanonicalEventScope{FactorySessionID: "session-1"},
	})
	if err != nil {
		t.Fatalf("SubscribeFrom: %v", err)
	}
	outcome := subscribed.Subscription.Next(context.Background())
	if outcome.Kind != recordings.SubscriptionEvent || outcome.Event.ID != "session-1/0" {
		t.Fatalf("SubscribeFrom outcome = %#v, want session-1/0", outcome)
	}

	if _, err := root.SubscribeFrom(context.Background(), recordings.SubscribeRequest{
		Scope: recordings.CanonicalEventScope{FactorySessionID: "   "},
	}); !errors.Is(err, recordings.ErrInvalidSubscribeScope) {
		t.Fatalf("SubscribeFrom whitespace scope = %v, want ErrInvalidSubscribeScope", err)
	}
}

type stubLedger struct {
	events          []factorydefinitions.FactoryEvent
	subscribeStream factorydefinitions.FactoryEventStream
}

func (ledger *stubLedger) CanonicalEvents() []factorydefinitions.FactoryEvent {
	out := make([]factorydefinitions.FactoryEvent, len(ledger.events))
	copy(out, ledger.events)
	return out
}

func (ledger *stubLedger) Subscribe(
	_ context.Context,
	_ *factorydefinitions.FactoryEventReconnectCursor,
	_ factorydefinitions.FactoryEventReconnectScope,
) (factorydefinitions.FactoryEventStream, error) {
	return ledger.subscribeStream, nil
}

func (ledger *stubLedger) StreamGenerationID() string { return "gen-1" }

func (ledger *stubLedger) AddEventRecorder(func(factorydefinitions.FactoryEvent)) {}

func (ledger *stubLedger) AddEventTypeRecorder(func(factorydefinitions.FactoryEventType)) {}

func (ledger *stubLedger) AppendRecordedEvent(event factorydefinitions.FactoryEvent) {
	event.Context.Sequence = len(ledger.events)
	ledger.events = append(ledger.events, event)
}

func scopedLegacyEvent(
	id string,
	globalSequence int,
	sessionSequence int,
	sessionID string,
) factorydefinitions.FactoryEvent {
	return factorydefinitions.FactoryEvent{
		Id: id,
		Context: factorydefinitions.FactoryEventContext{
			Sequence:        globalSequence,
			SessionID:       &sessionID,
			SessionSequence: &sessionSequence,
		},
	}
}
