package responseeventstore_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/portpowered/infinite-you/pkg/services/factory_sessions/responseevents"
	"github.com/portpowered/infinite-you/pkg/services/factory_sessions/internal/responseeventstore"
)

func publishForDispatch(t *testing.T, store *responseeventstore.SessionResponseEventStore, dispatchID string) responseevents.FactoryResponseEvent {
	t.Helper()
	input := samplePublishInput()
	input.DispatchID = dispatchID
	published, err := store.Publish(input)
	if err != nil {
		t.Fatalf("publish dispatch %q: %v", dispatchID, err)
	}
	return published
}

func TestSessionResponseEventStoreSubscription_DispatchFilterOmitsNonMatchingCatchUp(t *testing.T) {
	t.Parallel()

	store := newResponseEventStore("session-abc")
	alphaFirst := publishForDispatch(t, store, "dispatch-alpha")
	publishForDispatch(t, store, "dispatch-beta")
	alphaSecond := publishForDispatch(t, store, "dispatch-alpha")

	subscription, err := store.Subscribe(0, responseeventstore.WithDispatchFilter("dispatch-alpha"))
	if err != nil {
		t.Fatalf("Subscribe: %v", err)
	}
	defer subscription.Detach()
	if got := subscription.DispatchFilter(); got != "dispatch-alpha" {
		t.Fatalf("DispatchFilter() = %q, want dispatch-alpha", got)
	}

	events, err := subscription.Next(context.Background())
	if err != nil {
		t.Fatalf("Next: %v", err)
	}
	if len(events) != 2 {
		t.Fatalf("event count = %d, want 2 matching alpha events", len(events))
	}
	if events[0].Sequence != alphaFirst.Sequence || events[1].Sequence != alphaSecond.Sequence {
		t.Fatalf("sequences = [%d %d], want global [%d %d]",
			events[0].Sequence, events[1].Sequence, alphaFirst.Sequence, alphaSecond.Sequence)
	}
	if events[0].EventID != alphaFirst.EventID || events[1].EventID != alphaSecond.EventID {
		t.Fatalf("event IDs changed under filter: got [%q %q], want [%q %q]",
			events[0].EventID, events[1].EventID, alphaFirst.EventID, alphaSecond.EventID)
	}
}

func TestSessionResponseEventStoreSubscription_DispatchFilterContinuesLiveWithoutRenumbering(t *testing.T) {
	t.Parallel()

	store := newResponseEventStore("session-abc")
	publishForDispatch(t, store, "dispatch-alpha")

	subscription, err := store.Subscribe(0, responseeventstore.WithDispatchFilter("dispatch-alpha"))
	if err != nil {
		t.Fatalf("Subscribe: %v", err)
	}
	defer subscription.Detach()

	if _, err := subscription.Next(context.Background()); err != nil {
		t.Fatalf("Next(catch-up): %v", err)
	}

	done := make(chan struct{})
	var live []responseevents.FactoryResponseEvent
	var nextErr error
	go func() {
		live, nextErr = subscription.Next(context.Background())
		close(done)
	}()

	select {
	case <-done:
		t.Fatal("Next returned before live publish")
	case <-time.After(100 * time.Millisecond):
	}

	publishForDispatch(t, store, "dispatch-beta")
	alphaLive := publishForDispatch(t, store, "dispatch-alpha")

	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("timed out waiting for filtered live delivery")
	}
	if nextErr != nil {
		t.Fatalf("Next(live): %v", nextErr)
	}
	if len(live) != 1 {
		t.Fatalf("live event count = %d, want 1", len(live))
	}
	if live[0].Sequence != alphaLive.Sequence || live[0].EventID != alphaLive.EventID {
		t.Fatalf("live event = (%d, %q), want global (%d, %q)",
			live[0].Sequence, live[0].EventID, alphaLive.Sequence, alphaLive.EventID)
	}
}

func TestSessionResponseEventStoreSubscription_DispatchFilterAfterSequenceSkipsNonMatchingWithoutSkippingMatches(t *testing.T) {
	t.Parallel()

	store := newResponseEventStore("session-abc")
	alphaFirst := publishForDispatch(t, store, "dispatch-alpha")
	publishForDispatch(t, store, "dispatch-beta")
	alphaSecond := publishForDispatch(t, store, "dispatch-alpha")

	subscription, err := store.Subscribe(alphaFirst.Sequence, responseeventstore.WithDispatchFilter("dispatch-alpha"))
	if err != nil {
		t.Fatalf("Subscribe: %v", err)
	}
	defer subscription.Detach()

	events, err := subscription.Next(context.Background())
	if err != nil {
		t.Fatalf("Next: %v", err)
	}
	if len(events) != 1 {
		t.Fatalf("event count = %d, want 1 later alpha event", len(events))
	}
	if events[0].Sequence != alphaSecond.Sequence {
		t.Fatalf("sequence = %d, want %d", events[0].Sequence, alphaSecond.Sequence)
	}
}

func TestSessionResponseEventStoreSubscription_UnfilteredSubscriptionSeesAllDispatches(t *testing.T) {
	t.Parallel()

	store := newResponseEventStore("session-abc")
	first := publishForDispatch(t, store, "dispatch-alpha")
	second := publishForDispatch(t, store, "dispatch-beta")

	subscription, err := store.Subscribe(0)
	if err != nil {
		t.Fatalf("Subscribe: %v", err)
	}
	defer subscription.Detach()

	events, err := subscription.Next(context.Background())
	if err != nil {
		t.Fatalf("Next: %v", err)
	}
	if len(events) != 2 {
		t.Fatalf("event count = %d, want 2", len(events))
	}
	if events[0].Sequence != first.Sequence || events[1].Sequence != second.Sequence {
		t.Fatalf("sequences = [%d %d], want [%d %d]", events[0].Sequence, events[1].Sequence, first.Sequence, second.Sequence)
	}
}

func TestSessionResponseEventStoreSubscription_MultipleDispatchesShareGlobalSequenceSpace(t *testing.T) {
	t.Parallel()

	store := newResponseEventStore("session-abc")
	alpha := publishForDispatch(t, store, "dispatch-alpha")
	beta := publishForDispatch(t, store, "dispatch-beta")
	alphaAgain := publishForDispatch(t, store, "dispatch-alpha")

	if beta.Sequence != alpha.Sequence+1 || alphaAgain.Sequence != beta.Sequence+1 {
		t.Fatalf("sequences = [%d %d %d], want strictly increasing global space",
			alpha.Sequence, beta.Sequence, alphaAgain.Sequence)
	}
	if store.LatestSequence() != 3 {
		t.Fatalf("latest sequence = %d, want 3", store.LatestSequence())
	}
}

func TestSessionResponseEventStoreSubscription_DispatchFilterTrimsWhitespace(t *testing.T) {
	t.Parallel()

	store := newResponseEventStore("session-abc")
	published := publishForDispatch(t, store, "dispatch-alpha")

	subscription, err := store.Subscribe(0, responseeventstore.WithDispatchFilter(" dispatch-alpha "))
	if err != nil {
		t.Fatalf("Subscribe: %v", err)
	}
	defer subscription.Detach()

	events, err := subscription.Next(context.Background())
	if err != nil {
		t.Fatalf("Next: %v", err)
	}
	if len(events) != 1 || events[0].Sequence != published.Sequence {
		t.Fatalf("events = %#v, want one alpha event at sequence %d", events, published.Sequence)
	}
}

func TestSessionResponseEventStoreSubscription_DispatchFilterRejectsEmptyIdentity(t *testing.T) {
	t.Parallel()

	store := newResponseEventStore("session-abc")
	if _, err := store.Subscribe(0, responseeventstore.WithDispatchFilter("   ")); !errors.Is(err, responseeventstore.ErrInvalidDispatchFilter) {
		t.Fatalf("Subscribe error = %v, want ErrInvalidDispatchFilter", err)
	}
}
