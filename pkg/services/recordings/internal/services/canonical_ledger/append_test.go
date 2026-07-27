package canonicalledger_test

import (
	"errors"
	"testing"
	"time"

	factorydefinitions "github.com/portpowered/infinite-you/pkg/services/factory_definitions"
	recordings "github.com/portpowered/infinite-you/pkg/services/recordings"
	"github.com/portpowered/infinite-you/pkg/services/recordings/internal/services/canonical_ledger/wire"
)

func TestAppendAssignsRecordingsOrderAndRejectsCallerSequence(t *testing.T) {
	t.Parallel()

	ledger := &stubLedger{}
	svc := wire.NewService(ledger)
	event := scopedAppendEvent("evt-1", 99)
	accepted, err := svc.Append(recordings.AppendRecordedEventRequest{Event: event})
	if err != nil {
		t.Fatalf("Append valid event: %v", err)
	}
	if accepted.Event.Sequence != 0 ||
		accepted.Event.Cursor != (recordings.CanonicalEventCursor{
			StreamGenerationID: "gen-1",
			Sequence:           0,
		}) {
		t.Fatalf("accepted event = %#v, want Recordings-assigned order 0", accepted.Event)
	}
	if ledger.events[0].Context.Sequence != 0 {
		t.Fatalf("ledger sequence = %d, want caller sequence ignored", ledger.events[0].Context.Sequence)
	}
}

func TestAppendDistinctEventsRetainIncreasingOrder(t *testing.T) {
	t.Parallel()

	ledger := &stubLedger{}
	svc := wire.NewService(ledger)
	scope := recordings.CanonicalEventScope{FactorySessionID: "session-1"}

	first, err := svc.Append(recordings.AppendRecordedEventRequest{
		Event: scopedAppendEvent("evt-1", 0, scope),
	})
	if err != nil {
		t.Fatalf("Append first event: %v", err)
	}
	second, err := svc.Append(recordings.AppendRecordedEventRequest{
		Event: scopedAppendEvent("evt-2", 0, scope),
	})
	if err != nil {
		t.Fatalf("Append second event: %v", err)
	}
	if len(ledger.events) != 2 {
		t.Fatalf("ledger events = %d, want 2 distinct appends", len(ledger.events))
	}
	if first.Event.Sequence != 0 || second.Event.Sequence != 1 {
		t.Fatalf(
			"accepted sequences = (%d, %d), want increasing canonical order 0 then 1",
			first.Event.Sequence,
			second.Event.Sequence,
		)
	}
	if ledger.events[0].Context.Sequence != 0 || ledger.events[1].Context.Sequence != 1 {
		t.Fatalf("ledger global order = (%d, %d), want 0 then 1", ledger.events[0].Context.Sequence, ledger.events[1].Context.Sequence)
	}
}

func TestAppendIdempotentByAcceptedIdentity(t *testing.T) {
	t.Parallel()

	ledger := &stubLedger{}
	svc := wire.NewService(ledger)
	scope := recordings.CanonicalEventScope{FactorySessionID: "session-1"}
	event := scopedAppendEvent("evt-idempotent", 0, scope)

	first, err := svc.Append(recordings.AppendRecordedEventRequest{Event: event})
	if err != nil {
		t.Fatalf("Append first accepted identity: %v", err)
	}
	replay := event
	replay.Sequence = 99
	replay.Payload = `{"changed":true}`
	second, err := svc.Append(recordings.AppendRecordedEventRequest{Event: replay})
	if err != nil {
		t.Fatalf("Append idempotent replay: %v", err)
	}
	if len(ledger.events) != 1 {
		t.Fatalf("ledger events = %d, want one retained ordered fact", len(ledger.events))
	}
	if second.Event != first.Event {
		t.Fatalf("idempotent accepted facts = %#v, want %#v", second.Event, first.Event)
	}
}

func TestAppendInvalidEventsDoNotMutateRetainedHistory(t *testing.T) {
	t.Parallel()

	ledger := &stubLedger{}
	svc := wire.NewService(ledger)
	valid := scopedAppendEvent("evt-valid", 0, recordings.CanonicalEventScope{FactorySessionID: "session-1"})
	tests := map[string]func(*recordings.CanonicalEvent){
		"missing identity": func(event *recordings.CanonicalEvent) { event.ID = "" },
		"missing kind":     func(event *recordings.CanonicalEvent) { event.Kind = "" },
		"missing timestamp": func(event *recordings.CanonicalEvent) {
			event.RecordedAt = time.Time{}
		},
		"whitespace scope": func(event *recordings.CanonicalEvent) {
			event.Scope.FactorySessionID = "   "
		},
		"invalid payload": func(event *recordings.CanonicalEvent) {
			event.Payload = `{"incomplete":`
		},
	}
	for name, mutate := range tests {
		t.Run(name, func(t *testing.T) {
			event := valid
			mutate(&event)
			if _, err := svc.Append(recordings.AppendRecordedEventRequest{Event: event}); !errors.Is(
				err,
				recordings.ErrInvalidAppendEvent,
			) {
				t.Fatalf("Append invalid event error = %v, want ErrInvalidAppendEvent", err)
			}
			if len(ledger.events) != 0 {
				t.Fatalf("Append invalid event mutated ledger: %#v", ledger.events)
			}
		})
	}
}

func scopedAppendEvent(
	id string,
	callerSequence recordings.CanonicalEventSequence,
	scope ...recordings.CanonicalEventScope,
) recordings.CanonicalEvent {
	eventScope := recordings.CanonicalEventScope{FactorySessionID: "session-1"}
	if len(scope) > 0 {
		eventScope = scope[0]
	}
	return recordings.CanonicalEvent{
		ID:         recordings.CanonicalEventID(id),
		Sequence:   callerSequence,
		Scope:      eventScope,
		RecordedAt: time.Unix(1_700_000_000, 0).UTC(),
		Kind:       recordings.CanonicalEventKind(factorydefinitions.FactoryEventTypeWorkRequest),
		Payload:    `{"work":"one"}`,
	}
}
