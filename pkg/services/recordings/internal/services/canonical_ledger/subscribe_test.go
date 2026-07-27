package canonicalledger_test

import (
	"context"
	"errors"
	"testing"

	factorydefinitions "github.com/portpowered/infinite-you/pkg/services/factory_definitions"
	recordings "github.com/portpowered/infinite-you/pkg/services/recordings"
	"github.com/portpowered/infinite-you/pkg/services/recordings/internal/services/canonical_ledger/wire"
)

func TestSubscribeFromDeliversScopedRetainedHistoryInOrder(t *testing.T) {
	t.Parallel()

	ledger := &stubLedger{}
	svc := wire.NewService(ledger)
	ledger.subscribeStream = factorydefinitions.FactoryEventStream{
		StreamGenerationID: "gen-1",
		History: []factorydefinitions.FactoryEvent{
			scopedLegacyEvent("session-1/0", 4, 0, "session-1"),
			scopedLegacyEvent("session-2/0", 5, 0, "session-2"),
			scopedLegacyEvent("session-1/1", 6, 1, "session-1"),
		},
		Events: make(chan factorydefinitions.FactoryEvent),
	}

	subscribed, err := svc.SubscribeFrom(context.Background(), recordings.SubscribeRequest{
		Scope: recordings.CanonicalEventScope{FactorySessionID: "session-1"},
	})
	if err != nil {
		t.Fatalf("SubscribeFrom retained history: %v", err)
	}
	first := subscribed.Subscription.Next(context.Background())
	if first.Kind != recordings.SubscriptionEvent || first.Event.ID != "session-1/0" ||
		first.Event.Sequence != 4 {
		t.Fatalf("first outcome = %#v, want scoped session-1/0 at global sequence 4", first)
	}
	second := subscribed.Subscription.Next(context.Background())
	if second.Kind != recordings.SubscriptionEvent || second.Event.ID != "session-1/1" ||
		second.Event.Sequence != 6 {
		t.Fatalf("second outcome = %#v, want scoped session-1/1 at global sequence 6", second)
	}
}

func TestSubscribeFromReconnectsAfterValidCursor(t *testing.T) {
	t.Parallel()

	ledger := &stubLedger{}
	svc := wire.NewService(ledger)
	ledger.subscribeStream = factorydefinitions.FactoryEventStream{
		StreamGenerationID: "gen-1",
		History: []factorydefinitions.FactoryEvent{
			scopedLegacyEvent("session-1/0", 4, 0, "session-1"),
			scopedLegacyEvent("session-1/1", 6, 1, "session-1"),
		},
		Events: make(chan factorydefinitions.FactoryEvent),
	}

	initial, err := svc.SubscribeFrom(context.Background(), recordings.SubscribeRequest{
		Scope: recordings.CanonicalEventScope{FactorySessionID: "session-1"},
	})
	if err != nil {
		t.Fatalf("SubscribeFrom initial: %v", err)
	}
	first := initial.Subscription.Next(context.Background())
	cursor := first.Event.Cursor

	reconnected, err := svc.SubscribeFrom(context.Background(), recordings.SubscribeRequest{
		Cursor: &cursor,
		Scope:  recordings.CanonicalEventScope{FactorySessionID: "session-1"},
	})
	if err != nil {
		t.Fatalf("SubscribeFrom reconnect: %v", err)
	}
	outcome := reconnected.Subscription.Next(context.Background())
	if outcome.Kind != recordings.SubscriptionEvent || outcome.Event.ID != "session-1/1" {
		t.Fatalf("reconnect outcome = %#v, want only later scoped session-1/1", outcome)
	}
}

func TestSubscribeFromLatestCursorReturnsClosedWithoutFailure(t *testing.T) {
	t.Parallel()

	ledger := &stubLedger{}
	svc := wire.NewService(ledger)
	ledger.subscribeStream = factorydefinitions.FactoryEventStream{
		StreamGenerationID: "gen-1",
		History: []factorydefinitions.FactoryEvent{
			scopedLegacyEvent("session-1/0", 4, 0, "session-1"),
		},
		Events: make(chan factorydefinitions.FactoryEvent),
	}
	cursor := recordings.CanonicalEventCursor{
		StreamGenerationID: "gen-1",
		Sequence:           4,
	}

	result, err := svc.SubscribeFrom(context.Background(), recordings.SubscribeRequest{
		Cursor: &cursor,
		Scope:  recordings.CanonicalEventScope{FactorySessionID: "session-1"},
	})
	if err != nil {
		t.Fatalf("SubscribeFrom at latest retained cursor: %v", err)
	}
	outcome := result.Subscription.Next(context.Background())
	if outcome.Kind != recordings.SubscriptionClosed {
		t.Fatalf("empty continuation outcome = %#v, want CLOSED without failure", outcome)
	}
}

func TestSubscribeFromTypedFailures(t *testing.T) {
	t.Parallel()

	ledger := &stubLedger{}
	svc := wire.NewService(ledger)
	ledger.subscribeStream = factorydefinitions.FactoryEventStream{
		StreamGenerationID: "gen-1",
		History: []factorydefinitions.FactoryEvent{
			scopedLegacyEvent("session-1/0", 0, 0, "session-1"),
		},
		Events: make(chan factorydefinitions.FactoryEvent),
	}
	scope := recordings.CanonicalEventScope{FactorySessionID: "session-1"}

	if _, err := svc.SubscribeFrom(context.Background(), recordings.SubscribeRequest{
		Scope: recordings.CanonicalEventScope{FactorySessionID: "   "},
	}); !errors.Is(err, recordings.ErrInvalidSubscribeScope) {
		t.Fatalf("whitespace scope error = %v, want ErrInvalidSubscribeScope", err)
	}

	invalid := recordings.CanonicalEventCursor{Sequence: 0}
	if _, err := svc.SubscribeFrom(context.Background(), recordings.SubscribeRequest{
		Cursor: &invalid,
	}); !errors.Is(err, recordings.ErrInvalidReconnectCursor) {
		t.Fatalf("malformed cursor error = %v, want ErrInvalidReconnectCursor", err)
	}

	unavailable := recordings.CanonicalEventCursor{
		StreamGenerationID: "replaced-generation",
		Sequence:           0,
	}
	if _, err := svc.SubscribeFrom(context.Background(), recordings.SubscribeRequest{
		Cursor: &unavailable,
		Scope:  scope,
	}); !errors.Is(err, recordings.ErrReconnectCursorUnavailable) {
		t.Fatalf("unavailable generation error = %v, want ErrReconnectCursorUnavailable", err)
	}

	expired := recordings.CanonicalEventCursor{
		StreamGenerationID: "gen-1",
		Sequence:           99,
	}
	if _, err := svc.SubscribeFrom(context.Background(), recordings.SubscribeRequest{
		Cursor: &expired,
		Scope:  scope,
	}); !errors.Is(err, recordings.ErrReconnectCursorExpired) {
		t.Fatalf("missing retained position error = %v, want ErrReconnectCursorExpired", err)
	}
}

func TestSubscribeFromFailureCancelsLedgerSubscription(t *testing.T) {
	t.Parallel()

	ledger := &stubLedger{}
	svc := wire.NewService(ledger)
	ledger.subscribeStream = factorydefinitions.FactoryEventStream{
		StreamGenerationID: "gen-1",
		History: []factorydefinitions.FactoryEvent{
			scopedLegacyEvent("session-1/0", 0, 0, "session-1"),
		},
		Events: make(chan factorydefinitions.FactoryEvent),
	}

	expired := recordings.CanonicalEventCursor{
		StreamGenerationID: "gen-1",
		Sequence:           99,
	}
	if _, err := svc.SubscribeFrom(context.Background(), recordings.SubscribeRequest{
		Cursor: &expired,
		Scope:  recordings.CanonicalEventScope{FactorySessionID: "session-1"},
	}); !errors.Is(err, recordings.ErrReconnectCursorExpired) {
		t.Fatalf("failed subscribe error = %v, want ErrReconnectCursorExpired", err)
	}
	if ledger.subscribeCount != 1 {
		t.Fatalf("subscribe count = %d, want one ledger subscribe attempt", ledger.subscribeCount)
	}
	if ledger.lastSubscribeCtx == nil || ledger.lastSubscribeCtx.Err() == nil {
		t.Fatal("failed subscribe must cancel the ledger stream context")
	}
}
