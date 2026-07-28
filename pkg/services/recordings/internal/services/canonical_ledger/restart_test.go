package canonicalledger_test

import (
	"context"
	"errors"
	"strconv"
	"testing"
	"time"

	factorydefinitions "github.com/portpowered/infinite-you/pkg/services/factory_definitions"
	recordings "github.com/portpowered/infinite-you/pkg/services/recordings"
	recordingevents "github.com/portpowered/infinite-you/pkg/services/recordings/internal/services/canonical_ledger/events"
	"github.com/portpowered/infinite-you/pkg/services/recordings/internal/services/canonical_ledger/wire"
)

func TestReconstructedCanonicalLedgerPreservesOrderAndReconnect(t *testing.T) {
	t.Parallel()

	const generationID = "restart-generation"
	now := time.Unix(1_700_000_000, 0).UTC()
	scope := recordings.CanonicalEventScope{FactorySessionID: "session-restart"}

	originalLedger := recordingevents.NewRuntimeLedger(nil, func() time.Time { return now }, generationID, nil)
	original := wire.NewService(originalLedger)

	var reconnectCursor recordings.CanonicalEventCursor
	for index := 0; index < 3; index++ {
		accepted, err := original.Append(recordings.AppendRecordedEventRequest{
			Event: restartAppendEvent(index, scope, now),
		})
		if err != nil {
			t.Fatalf("Append restart event %d: %v", index, err)
		}
		if accepted.Event.Sequence != recordings.CanonicalEventSequence(index) {
			t.Fatalf("accepted sequence %d = %d, want %d", index, accepted.Event.Sequence, index)
		}
		if index == 0 {
			reconnectCursor = accepted.Event.Cursor
		}
	}

	retained := originalLedger.CanonicalEvents()
	restartedLedger := reconstructRuntimeLedger(t, retained, generationID, now)
	restarted := wire.NewService(restartedLedger)

	if got := restartedLedger.CanonicalEvents(); len(got) != len(retained) {
		t.Fatalf("restarted retained events = %d, want %d", len(got), len(retained))
	}
	for index, event := range retained {
		if got := restartedLedger.CanonicalEvents()[index]; got.Id != event.Id ||
			got.Context.Sequence != event.Context.Sequence {
			t.Fatalf(
				"restarted event[%d] = (%q, %d), want preserved (%q, %d)",
				index,
				got.Id,
				got.Context.Sequence,
				event.Id,
				event.Context.Sequence,
			)
		}
	}

	reconnected, err := restarted.SubscribeFrom(context.Background(), recordings.SubscribeRequest{
		Cursor: &reconnectCursor,
		Scope:  scope,
	})
	if err != nil {
		t.Fatalf("SubscribeFrom after restart: %v", err)
	}
	for index := 1; index < 3; index++ {
		outcome := reconnected.Subscription.Next(context.Background())
		if outcome.Kind != recordings.SubscriptionEvent ||
			outcome.Event.Sequence != recordings.CanonicalEventSequence(index) {
			t.Fatalf("reconnect outcome %d = %#v, want retained event %d", index, outcome, index)
		}
	}
}

func TestReconstructedCanonicalLedgerRejectsReplacedStreamGeneration(t *testing.T) {
	t.Parallel()

	const originalGeneration = "restart-generation"
	now := time.Unix(1_700_000_000, 0).UTC()
	scope := recordings.CanonicalEventScope{FactorySessionID: "session-restart"}

	originalLedger := recordingevents.NewRuntimeLedger(nil, func() time.Time { return now }, originalGeneration, nil)
	original := wire.NewService(originalLedger)
	accepted, err := original.Append(recordings.AppendRecordedEventRequest{
		Event: restartAppendEvent(0, scope, now),
	})
	if err != nil {
		t.Fatalf("Append restart seed event: %v", err)
	}

	replacedLedger := reconstructRuntimeLedger(
		t,
		originalLedger.CanonicalEvents(),
		"replaced-generation",
		now,
	)
	replaced := wire.NewService(replacedLedger)

	_, err = replaced.SubscribeFrom(context.Background(), recordings.SubscribeRequest{
		Cursor: &accepted.Event.Cursor,
		Scope:  scope,
	})
	if !errors.Is(err, recordings.ErrReconnectCursorUnavailable) {
		t.Fatalf("replaced generation error = %v, want ErrReconnectCursorUnavailable", err)
	}
}

func reconstructRuntimeLedger(
	t *testing.T,
	retained []factorydefinitions.FactoryEvent,
	generationID string,
	now time.Time,
) recordings.RuntimeEventLedger {
	t.Helper()

	ledger := recordingevents.NewRuntimeLedger(nil, func() time.Time { return now }, generationID, nil)
	for _, event := range retained {
		ledger.AppendRecordedEvent(event)
	}
	return ledger
}

func restartAppendEvent(
	index int,
	scope recordings.CanonicalEventScope,
	recordedAt time.Time,
) recordings.CanonicalEvent {
	return recordings.CanonicalEvent{
		ID:         recordings.CanonicalEventID("restart-event-" + strconv.Itoa(index)),
		Scope:      scope,
		RecordedAt: recordedAt.Add(time.Duration(index) * time.Second),
		Kind:       recordings.CanonicalEventKind(factorydefinitions.FactoryEventTypeWorkRequest),
		Payload:    `{"restart":true}`,
	}
}
