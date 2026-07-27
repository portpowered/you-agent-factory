package canonicalledger_test

import (
	"context"
	"strconv"
	"testing"
	"time"

	factorydefinitions "github.com/portpowered/infinite-you/pkg/services/factory_definitions"
	recordings "github.com/portpowered/infinite-you/pkg/services/recordings"
	recordingevents "github.com/portpowered/infinite-you/pkg/services/recordings/events"
	canonicalledger "github.com/portpowered/infinite-you/pkg/services/recordings/internal/services/canonical_ledger"
	"github.com/portpowered/infinite-you/pkg/services/recordings/internal/services/canonical_ledger/wire"
)

func TestSubscribeFromSequenceDiscontinuityReportsExplicitGap(t *testing.T) {
	t.Parallel()

	ledger := &stubLedger{}
	svc := wire.NewService(ledger)
	ledger.subscribeStream = factorydefinitions.FactoryEventStream{
		StreamGenerationID: "gen-1",
		History: []factorydefinitions.FactoryEvent{
			scopedLegacyEvent("session-1/0", 4, 0, "session-1"),
			scopedLegacyEvent("session-2/0", 5, 0, "session-2"),
			scopedLegacyEvent("session-1/2", 8, 2, "session-1"),
		},
		Events: make(chan factorydefinitions.FactoryEvent),
	}

	subscribed, err := svc.SubscribeFrom(context.Background(), recordings.SubscribeRequest{
		Scope: recordings.CanonicalEventScope{FactorySessionID: "session-1"},
	})
	if err != nil {
		t.Fatalf("SubscribeFrom discontinuity setup: %v", err)
	}
	first := subscribed.Subscription.Next(context.Background())
	if first.Kind != recordings.SubscriptionEvent || first.Event.ID != "session-1/0" {
		t.Fatalf("first outcome = %#v, want session-1/0 before gap", first)
	}
	gap := subscribed.Subscription.Next(context.Background())
	if gap.Kind != recordings.SubscriptionGap || gap.Gap == nil ||
		gap.Gap.Cause != recordings.SubscriptionSequenceDiscontinuity ||
		gap.Gap.ExpectedSequence != 1 || gap.Gap.ObservedSequence != 2 ||
		gap.Gap.ReconnectFrom.Sequence != 4 {
		t.Fatalf("discontinuity gap = %#v, want explicit scoped gap 1 -> 2 after global cursor 4", gap)
	}
}

func TestSubscribeFromGapOutcomesAreDeterministic(t *testing.T) {
	t.Parallel()

	history := []factorydefinitions.FactoryEvent{
		scopedLegacyEvent("session-1/0", 4, 0, "session-1"),
		scopedLegacyEvent("session-2/0", 5, 0, "session-2"),
		scopedLegacyEvent("session-1/2", 8, 2, "session-1"),
	}
	scope := recordings.CanonicalEventScope{FactorySessionID: "session-1"}

	firstGap := readScopedDiscontinuityGap(t, history, scope)
	secondGap := readScopedDiscontinuityGap(t, history, scope)
	if firstGap != secondGap {
		t.Fatalf("gap outcomes differ: first %#v, second %#v", firstGap, secondGap)
	}
}

func readScopedDiscontinuityGap(
	t *testing.T,
	history []factorydefinitions.FactoryEvent,
	scope recordings.CanonicalEventScope,
) recordings.SubscriptionGapFacts {
	t.Helper()

	ledger := &stubLedger{}
	svc := wire.NewService(ledger)
	ledger.subscribeStream = factorydefinitions.FactoryEventStream{
		StreamGenerationID: "gen-1",
		History:            history,
		Events:             make(chan factorydefinitions.FactoryEvent),
	}
	subscribed, err := svc.SubscribeFrom(context.Background(), recordings.SubscribeRequest{Scope: scope})
	if err != nil {
		t.Fatalf("SubscribeFrom: %v", err)
	}
	_ = subscribed.Subscription.Next(context.Background())
	gap := subscribed.Subscription.Next(context.Background())
	if gap.Kind != recordings.SubscriptionGap || gap.Gap == nil {
		t.Fatalf("gap outcome = %#v, want GAP", gap)
	}
	return *gap.Gap
}

func TestSubscribeFromBackpressureReportsGapWithoutDeliveringUnavailableEvent(t *testing.T) {
	t.Parallel()

	const eventCount = 512
	now := time.Unix(1_700_000_000, 0).UTC()
	ledger := recordingevents.NewRuntimeLedger(nil, func() time.Time { return now }, "overflow-generation", nil)
	svc := wire.NewService(ledger)

	appendOverflowEvent(t, svc, 0, now)
	result, err := svc.SubscribeFrom(context.Background(), recordings.SubscribeRequest{})
	if err != nil {
		t.Fatalf("SubscribeFrom overflow ledger: %v", err)
	}
	first := result.Subscription.Next(context.Background())
	if first.Kind != recordings.SubscriptionEvent || first.Event.Sequence != 0 {
		t.Fatalf("first outcome = %#v, want event 0", first)
	}
	for sequence := 1; sequence < eventCount; sequence++ {
		appendOverflowEvent(t, svc, sequence, now.Add(time.Duration(sequence)*time.Second))
	}

	gap := readOverflowGap(t, result.Subscription, first.Event.Cursor)
	if gap.Cause != recordings.SubscriptionBackpressure {
		t.Fatalf("overflow gap cause = %q, want BACKPRESSURE", gap.Cause)
	}
}

func TestSubscribeFromBackpressureReconnectResumesRetainedEvents(t *testing.T) {
	t.Parallel()

	const eventCount = 512
	now := time.Unix(1_700_000_000, 0).UTC()
	ledger := recordingevents.NewRuntimeLedger(nil, func() time.Time { return now }, "overflow-generation", nil)
	svc := wire.NewService(ledger)

	appendOverflowEvent(t, svc, 0, now)
	result, err := svc.SubscribeFrom(context.Background(), recordings.SubscribeRequest{})
	if err != nil {
		t.Fatalf("SubscribeFrom overflow ledger: %v", err)
	}
	first := result.Subscription.Next(context.Background())
	for sequence := 1; sequence < eventCount; sequence++ {
		appendOverflowEvent(t, svc, sequence, now.Add(time.Duration(sequence)*time.Second))
	}
	gap := readOverflowGap(t, result.Subscription, first.Event.Cursor)
	assertOverflowReconnect(t, svc, gap, eventCount)
}

func readOverflowGap(
	t *testing.T,
	subscription recordings.EventSubscription,
	lastDelivered recordings.CanonicalEventCursor,
) recordings.SubscriptionGapFacts {
	t.Helper()
	readContext, cancelRead := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancelRead()
	for {
		outcome := subscription.Next(readContext)
		if outcome.Kind == recordings.SubscriptionClosed {
			t.Fatal("overflow closed subscription without an explicit gap")
		}
		if outcome.Kind == recordings.SubscriptionGap {
			return assertOverflowGap(t, outcome, lastDelivered)
		}
		if outcome.Event.Sequence != lastDelivered.Sequence+1 {
			t.Fatalf(
				"pre-gap unavailable event delivered as sequence %d, want only events up to %d",
				outcome.Event.Sequence,
				lastDelivered.Sequence,
			)
		}
		lastDelivered = outcome.Event.Cursor
	}
}

func assertOverflowGap(
	t *testing.T,
	outcome recordings.SubscriptionOutcome,
	lastDelivered recordings.CanonicalEventCursor,
) recordings.SubscriptionGapFacts {
	t.Helper()
	if outcome.Gap == nil || outcome.Gap.Cause != recordings.SubscriptionBackpressure ||
		outcome.Gap.ReconnectFrom != lastDelivered ||
		outcome.Gap.ExpectedSequence != lastDelivered.Sequence+1 ||
		outcome.Gap.ObservedSequence != outcome.Gap.ExpectedSequence {
		t.Fatalf("overflow gap = %#v, want backpressure after %#v", outcome, lastDelivered)
	}
	return *outcome.Gap
}

func assertOverflowReconnect(
	t *testing.T,
	svc canonicalledger.Service,
	gap recordings.SubscriptionGapFacts,
	eventCount recordings.CanonicalEventSequence,
) {
	t.Helper()
	reconnected, err := svc.SubscribeFrom(context.Background(), recordings.SubscribeRequest{
		Cursor: &gap.ReconnectFrom,
	})
	if err != nil {
		t.Fatalf("SubscribeFrom overflow reconnect: %v", err)
	}
	for sequence := gap.ReconnectFrom.Sequence + 1; sequence < eventCount; sequence++ {
		outcome := reconnected.Subscription.Next(context.Background())
		if outcome.Kind != recordings.SubscriptionEvent || outcome.Event.Sequence != sequence {
			t.Fatalf("reconnect outcome at %d = %#v, want retained event %d", sequence, outcome, sequence)
		}
	}
}

func appendOverflowEvent(t *testing.T, svc canonicalledger.Service, sequence int, recordedAt time.Time) {
	t.Helper()
	result, err := svc.Append(recordings.AppendRecordedEventRequest{
		Event: recordings.CanonicalEvent{
			ID:         recordings.CanonicalEventID("overflow-" + strconv.Itoa(sequence)),
			RecordedAt: recordedAt,
			Kind:       "OVERFLOW_TEST",
			Payload:    `{"retained":true}`,
		},
	})
	if err != nil {
		t.Fatalf("Append overflow event %d: %v", sequence, err)
	}
	if result.Event.Sequence != recordings.CanonicalEventSequence(sequence) {
		t.Fatalf("Append overflow event sequence = %d, want %d", result.Event.Sequence, sequence)
	}
}
