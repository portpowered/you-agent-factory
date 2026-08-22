package canonicalledger_test

import (
	"context"
	"errors"
	"testing"

	factorydefinitions "github.com/portpowered/infinite-you/pkg/services/factory_definitions"
	recordings "github.com/portpowered/infinite-you/pkg/services/recordings"
	ledgerservice "github.com/portpowered/infinite-you/pkg/services/recordings/internal/services/canonical_ledger/internal/service"
	"github.com/portpowered/infinite-you/pkg/services/recordings/internal/services/canonical_ledger/wire"
)

func TestCanonicalLedgerNewAndAppendFallbackRemainExplicit(t *testing.T) {
	t.Parallel()

	if got := ledgerservice.New(nil); got != nil {
		t.Fatalf("New(nil) = %#v, want nil", got)
	}

	ledger := &discardingLedger{stubLedger: &stubLedger{}}
	service := wire.NewService(ledger)
	accepted, err := service.Append(recordings.AppendRecordedEventRequest{
		Event: scopedAppendEvent("evt-discarded", 0, recordings.CanonicalEventScope{}),
	})
	if err != nil {
		t.Fatalf("Append to non-retaining ledger: %v", err)
	}
	if accepted.Event.ID != "" {
		t.Fatalf("Append to non-retaining ledger = %#v, want empty result", accepted.Event)
	}
}

func TestCanonicalLedgerAppendAdvancesFromScopedHistory(t *testing.T) {
	t.Parallel()

	sessionID := "session-1"
	otherSessionID := "session-2"
	olderSequence := 2
	lowerSequence := 1
	ledger := &stubLedger{events: []factorydefinitions.FactoryEvent{
		{
			Id: "other-session",
			Context: factorydefinitions.FactoryEventContext{
				SessionID: &otherSessionID,
			},
		},
		{
			Id: "legacy-session-event",
			Context: factorydefinitions.FactoryEventContext{
				SessionID: &sessionID,
			},
		},
		{
			Id: "high-session-event",
			Context: factorydefinitions.FactoryEventContext{
				SessionID:       &sessionID,
				SessionSequence: &olderSequence,
			},
		},
		{
			Id: "lower-session-event",
			Context: factorydefinitions.FactoryEventContext{
				SessionID:       &sessionID,
				SessionSequence: &lowerSequence,
			},
		},
	}}
	service := wire.NewService(ledger)
	accepted, err := service.Append(recordings.AppendRecordedEventRequest{
		Event: scopedAppendEvent(
			"evt-after-history",
			0,
			recordings.CanonicalEventScope{FactorySessionID: sessionID},
		),
	})
	if err != nil {
		t.Fatalf("Append after scoped history: %v", err)
	}
	if accepted.Event.Sequence != 4 {
		t.Fatalf("Append global sequence = %d, want retained position 4", accepted.Event.Sequence)
	}
	if got := ledger.events[len(ledger.events)-1].Context.SessionSequence; got == nil || *got != 3 {
		t.Fatalf("Append scoped sequence = %v, want 3 after retained history", got)
	}
}

func TestCanonicalLedgerSubscribeMapsLedgerFailures(t *testing.T) {
	t.Parallel()

	ledger := &stubLedger{subscribeErr: errors.New("ledger unavailable")}
	service := wire.NewService(ledger)
	if _, err := service.SubscribeFrom(context.Background(), recordings.SubscribeRequest{}); err == nil || err.Error() != "ledger unavailable" {
		t.Fatalf("generic SubscribeFrom error = %v, want ledger unavailable", err)
	}

	ledger.subscribeErr = recordings.ErrReconnectCursorNotFound
	if _, err := service.SubscribeFrom(context.Background(), recordings.SubscribeRequest{}); !errors.Is(err, recordings.ErrReconnectCursorExpired) {
		t.Fatalf("missing cursor SubscribeFrom error = %v, want ErrReconnectCursorExpired", err)
	}
}

func TestCanonicalLedgerSubscriptionSkipsStaleAndOutOfScopeLiveEvents(t *testing.T) {
	t.Parallel()

	ledger := &stubLedger{}
	live := make(chan factorydefinitions.FactoryEvent, 3)
	live <- scopedLegacyEvent("session-2/0", 1, 0, "session-2")
	live <- scopedLegacyEvent("session-1/stale", 2, 0, "session-1")
	live <- scopedLegacyEvent("session-1/1", 3, 1, "session-1")
	close(live)
	ledger.subscribeStream = factorydefinitions.FactoryEventStream{
		StreamGenerationID: "gen-1",
		History: []factorydefinitions.FactoryEvent{
			scopedLegacyEvent("session-1/0", 0, 0, "session-1"),
		},
		Events: live,
	}
	service := wire.NewService(ledger)
	subscribed, err := service.SubscribeFrom(context.Background(), recordings.SubscribeRequest{
		Scope: recordings.CanonicalEventScope{FactorySessionID: "session-1"},
	})
	if err != nil {
		t.Fatalf("SubscribeFrom live filtering: %v", err)
	}
	first := subscribed.Subscription.Next(context.Background())
	if first.Kind != recordings.SubscriptionEvent || first.Event.ID != "session-1/0" {
		t.Fatalf("first subscription outcome = %#v, want retained session-1/0", first)
	}
	second := subscribed.Subscription.Next(context.Background())
	if second.Kind != recordings.SubscriptionEvent || second.Event.ID != "session-1/1" {
		t.Fatalf("second subscription outcome = %#v, want live session-1/1 after filtering", second)
	}
	gap := subscribed.Subscription.Next(context.Background())
	if gap.Kind != recordings.SubscriptionGap || gap.Gap == nil || gap.Gap.Cause != recordings.SubscriptionBackpressure {
		t.Fatalf("closed live subscription outcome = %#v, want backpressure gap", gap)
	}
	closed := subscribed.Subscription.Next(context.Background())
	if closed.Kind != recordings.SubscriptionClosed {
		t.Fatalf("terminal subscription outcome = %#v, want CLOSED", closed)
	}
}

type discardingLedger struct {
	*stubLedger
}

func (ledger *discardingLedger) AppendRecordedEvent(factorydefinitions.FactoryEvent) {}

var _ recordings.Ledger = (*discardingLedger)(nil)
