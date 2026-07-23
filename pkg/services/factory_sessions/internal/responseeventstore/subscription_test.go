package responseeventstore_test

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/portpowered/infinite-you/pkg/services/factory_sessions/internal/responseevents"
	"github.com/portpowered/infinite-you/pkg/services/factory_sessions/internal/responseeventstore"
)

func TestSessionResponseEventStoreSubscription_CatchUpThenLiveInOrder(t *testing.T) {
	t.Parallel()

	store := newResponseEventStore("session-abc")
	first, err := store.Publish(samplePublishInput())
	if err != nil {
		t.Fatalf("first publish: %v", err)
	}
	second, err := store.Publish(samplePublishInput())
	if err != nil {
		t.Fatalf("second publish: %v", err)
	}

	subscription, err := store.Subscribe(0)
	if err != nil {
		t.Fatalf("Subscribe: %v", err)
	}
	defer subscription.Detach()

	initial, err := subscription.Next(context.Background())
	if err != nil {
		t.Fatalf("Next(initial): %v", err)
	}
	if len(initial) != 2 {
		t.Fatalf("initial event count = %d, want 2", len(initial))
	}
	if initial[0].Sequence != first.Sequence || initial[1].Sequence != second.Sequence {
		t.Fatalf("initial sequences = [%d %d], want [%d %d]",
			initial[0].Sequence, initial[1].Sequence, first.Sequence, second.Sequence)
	}

	third, err := store.Publish(samplePublishInput())
	if err != nil {
		t.Fatalf("third publish: %v", err)
	}
	live, err := subscription.Next(context.Background())
	if err != nil {
		t.Fatalf("Next(live): %v", err)
	}
	if len(live) != 1 || live[0].Sequence != third.Sequence {
		t.Fatalf("live events = %#v, want sequence %d", live, third.Sequence)
	}
}

func TestSessionResponseEventStoreSubscription_AfterSequenceSkipsEarlierEvents(t *testing.T) {
	t.Parallel()

	store := newResponseEventStore("session-abc")
	for range 3 {
		if _, err := store.Publish(samplePublishInput()); err != nil {
			t.Fatalf("publish: %v", err)
		}
	}

	subscription, err := store.Subscribe(1)
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
	if events[0].Sequence != 2 || events[1].Sequence != 3 {
		t.Fatalf("sequences = [%d %d], want [2 3]", events[0].Sequence, events[1].Sequence)
	}
}

func TestSessionResponseEventStoreSubscription_DrainReturnsRetainedEventsWithoutWaiting(t *testing.T) {
	t.Parallel()

	store := newResponseEventStore("session-abc")
	subscription, err := store.Subscribe(0)
	if err != nil {
		t.Fatalf("Subscribe: %v", err)
	}
	defer subscription.Detach()

	published, err := store.Publish(samplePublishInput())
	if err != nil {
		t.Fatalf("Publish: %v", err)
	}
	events, err := subscription.Drain()
	if err != nil {
		t.Fatalf("Drain: %v", err)
	}
	if len(events) != 1 || events[0].Sequence != published.Sequence {
		t.Fatalf("events = %#v, want sequence %d", events, published.Sequence)
	}
	empty, err := subscription.Drain()
	if err != nil {
		t.Fatalf("Drain(empty): %v", err)
	}
	if len(empty) != 0 {
		t.Fatalf("second drain = %#v, want no duplicate events", empty)
	}
}

func TestSessionResponseEventStoreSubscription_NoDuplicateSequencesOrEventIDs(t *testing.T) {
	t.Parallel()

	store := newResponseEventStore("session-abc")
	for range 2 {
		if _, err := store.Publish(samplePublishInput()); err != nil {
			t.Fatalf("publish: %v", err)
		}
	}

	subscription, err := store.Subscribe(0)
	if err != nil {
		t.Fatalf("Subscribe: %v", err)
	}
	defer subscription.Detach()

	seenSequences := make(map[int64]struct{})
	seenEventIDs := make(map[string]struct{})
	record := func(events []responseevents.FactoryResponseEvent) {
		for _, event := range events {
			if _, exists := seenSequences[event.Sequence]; exists {
				t.Fatalf("duplicate sequence %d", event.Sequence)
			}
			seenSequences[event.Sequence] = struct{}{}
			if _, exists := seenEventIDs[event.EventID]; exists {
				t.Fatalf("duplicate event ID %q", event.EventID)
			}
			seenEventIDs[event.EventID] = struct{}{}
		}
	}

	initial, err := subscription.Next(context.Background())
	if err != nil {
		t.Fatalf("Next(initial): %v", err)
	}
	record(initial)

	if _, err := store.Publish(samplePublishInput()); err != nil {
		t.Fatalf("publish live: %v", err)
	}
	live, err := subscription.Next(context.Background())
	if err != nil {
		t.Fatalf("Next(live): %v", err)
	}
	record(live)
}

func TestSessionResponseEventStoreSubscription_AfterLatestSequenceWaitsForLiveOnly(t *testing.T) {
	t.Parallel()

	store := newResponseEventStore("session-abc")
	for range 2 {
		if _, err := store.Publish(samplePublishInput()); err != nil {
			t.Fatalf("publish: %v", err)
		}
	}

	subscription, err := store.Subscribe(2)
	if err != nil {
		t.Fatalf("Subscribe: %v", err)
	}
	defer subscription.Detach()

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

	third, err := store.Publish(samplePublishInput())
	if err != nil {
		t.Fatalf("third publish: %v", err)
	}

	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("timed out waiting for live delivery")
	}
	if nextErr != nil {
		t.Fatalf("Next(live): %v", nextErr)
	}
	if len(live) != 1 || live[0].Sequence != third.Sequence {
		t.Fatalf("live events = %#v, want sequence %d", live, third.Sequence)
	}
}

func TestSessionResponseEventStoreSubscription_SubscribeOnClosedStoreFails(t *testing.T) {
	t.Parallel()

	store := newResponseEventStore("session-abc")
	store.Close()

	if _, err := store.Subscribe(0); !errors.Is(err, responseeventstore.ErrStoreClosed) {
		t.Fatalf("Subscribe on closed store error = %v, want ErrStoreClosed", err)
	}
	if got := store.SubscriberCount(); got != 0 {
		t.Fatalf("subscriber count = %d, want 0", got)
	}
}

func TestSessionResponseEventStoreSubscription_DetachStopsFurtherDelivery(t *testing.T) {
	t.Parallel()

	store := newResponseEventStore("session-abc")
	subscription, err := store.Subscribe(0)
	if err != nil {
		t.Fatalf("Subscribe: %v", err)
	}

	if got := store.SubscriberCount(); got != 1 {
		t.Fatalf("subscriber count = %d, want 1", got)
	}
	subscription.Detach()
	if got := store.SubscriberCount(); got != 0 {
		t.Fatalf("subscriber count after detach = %d, want 0", got)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancel()
	if _, err := subscription.Next(ctx); !errors.Is(err, responseeventstore.ErrSubscriptionClosed) {
		t.Fatalf("Next after detach error = %v, want ErrSubscriptionClosed", err)
	}
}

func TestSessionResponseEventStoreSubscription_CloseReleasesSubscribers(t *testing.T) {
	t.Parallel()

	store := newResponseEventStore("session-abc")
	first, err := store.Subscribe(0)
	if err != nil {
		t.Fatalf("Subscribe(first): %v", err)
	}
	second, err := store.Subscribe(0)
	if err != nil {
		t.Fatalf("Subscribe(second): %v", err)
	}

	if got := store.SubscriberCount(); got != 2 {
		t.Fatalf("subscriber count = %d, want 2", got)
	}
	store.Close()
	if got := store.SubscriberCount(); got != 0 {
		t.Fatalf("subscriber count after close = %d, want 0", got)
	}

	for _, subscription := range []*responseeventstore.Subscription{first, second} {
		if _, err := subscription.Next(context.Background()); !errors.Is(err, responseeventstore.ErrSubscriptionClosed) {
			t.Fatalf("Next after close error = %v, want ErrSubscriptionClosed", err)
		}
	}
}

func TestSessionResponseEventStoreSubscription_ConcurrentPublishAndSubscribe(t *testing.T) {
	t.Parallel()

	store := newResponseEventStore("session-abc")
	subscription, err := store.Subscribe(0)
	if err != nil {
		t.Fatalf("Subscribe: %v", err)
	}
	defer subscription.Detach()

	const workers = 16
	var wg sync.WaitGroup
	wg.Add(workers)
	for range workers {
		go func() {
			defer wg.Done()
			if _, err := store.Publish(samplePublishInput()); err != nil {
				t.Errorf("publish: %v", err)
			}
		}()
	}
	wg.Wait()

	events, err := subscription.Next(context.Background())
	if err != nil {
		t.Fatalf("Next: %v", err)
	}
	if len(events) != workers {
		t.Fatalf("event count = %d, want %d", len(events), workers)
	}
	for index := 1; index < len(events); index++ {
		if events[index].Sequence <= events[index-1].Sequence {
			t.Fatalf("events not ascending at index %d: %#v", index, events)
		}
	}
}
