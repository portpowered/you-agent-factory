package service

import (
	"context"
	"strconv"
	"testing"
	"time"

	factorydefinitions "github.com/portpowered/infinite-you/pkg/services/factory_definitions"
	recordings "github.com/portpowered/infinite-you/pkg/services/recordings"
)

func TestCombinedServiceRealLedgerOverflowReportsGapAndReconnects(t *testing.T) {
	t.Parallel()

	const eventCount = 512
	now := time.Unix(1_700_000_000, 0).UTC()
	ledger := NewRuntimeLedger(nil, func() time.Time { return now }, "overflow-generation", nil)
	svc := NewService(ledger, NewProjectionService())
	if svc == nil {
		t.Fatal("NewService returned nil")
	}

	appendOverflowEvent(t, svc, 0, now)
	result, err := svc.SubscribeFrom(context.Background(), recordings.SubscribeRequest{})
	if err != nil {
		t.Fatalf("SubscribeFrom real ledger: %v", err)
	}
	first := result.Subscription.Next(context.Background())
	if first.Kind != recordings.SubscriptionEvent || first.Event.Sequence != 0 {
		t.Fatalf("first subscription outcome = %#v, want event 0", first)
	}
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
			t.Fatal("overflow closed root subscription without an explicit gap")
		}
		if outcome.Kind == recordings.SubscriptionGap {
			return assertOverflowGap(t, outcome, lastDelivered)
		}
		if outcome.Event.Sequence != lastDelivered.Sequence+1 {
			t.Fatalf("pre-gap sequence = %d, want %d", outcome.Event.Sequence, lastDelivered.Sequence+1)
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
	svc recordings.Service,
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
			t.Fatalf("reconnect outcome at %d = %#v, want event %d", sequence, outcome, sequence)
		}
	}
}

func appendOverflowEvent(t *testing.T, svc recordings.Service, sequence int, recordedAt time.Time) {
	t.Helper()
	result, err := svc.Append(recordings.AppendRecordedEventRequest{
		Event: recordings.CanonicalEvent{
			ID:         recordings.CanonicalEventID("overflow-" + strconv.Itoa(sequence)),
			RecordedAt: recordedAt,
			Kind:       recordings.CanonicalEventKind(factorydefinitions.FactoryEventTypeWorkRequest),
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
