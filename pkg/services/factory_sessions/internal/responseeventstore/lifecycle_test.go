package responseeventstore_test

import (
	"context"
	"errors"
	"runtime"
	"sync"
	"testing"
	"time"

	"github.com/portpowered/infinite-you/pkg/services/factory_sessions/internal/responseeventstore"
)

func TestSessionResponseEventStore_CompleteRejectsPublish(t *testing.T) {
	t.Parallel()

	store := newResponseEventStore("session-abc")
	if _, err := store.Publish(samplePublishInput()); err != nil {
		t.Fatalf("publish before complete: %v", err)
	}

	store.Complete()

	if _, err := store.Publish(samplePublishInput()); !errors.Is(err, responseeventstore.ErrStoreCompleted) {
		t.Fatalf("publish after complete error = %v, want ErrStoreCompleted", err)
	}
	if events := store.Events(); len(events) != 1 {
		t.Fatalf("retained event count = %d, want 1", len(events))
	}
	if !store.Completed() {
		t.Fatal("Completed() = false, want true after Complete")
	}
}

func TestSessionResponseEventStore_CompleteSubscriberDrainsRetainedThenObservesCompletion(t *testing.T) {
	t.Parallel()

	start := time.Date(2026, 7, 13, 12, 0, 0, 0, time.UTC)
	store := responseeventstore.NewSessionResponseEventStoreWithClock("session-abc", &fixedClock{now: start}, testResponseEventID)

	for range 3 {
		if _, err := store.Publish(samplePublishInput()); err != nil {
			t.Fatalf("publish: %v", err)
		}
	}

	subscription, err := store.Subscribe(0)
	if err != nil {
		t.Fatalf("Subscribe: %v", err)
	}
	defer subscription.Detach()

	store.Complete()

	events, err := subscription.Next(context.Background())
	if err != nil {
		t.Fatalf("Next(catch-up): %v", err)
	}
	if len(events) != 3 {
		t.Fatalf("catch-up event count = %d, want 3", len(events))
	}

	if _, err := subscription.Next(context.Background()); !errors.Is(err, responseeventstore.ErrSubscriptionClosed) {
		t.Fatalf("Next after drain error = %v, want ErrSubscriptionClosed", err)
	}
	if completedAt := store.CompletedAt(); !completedAt.Equal(start) {
		t.Fatalf("CompletedAt = %s, want %s", completedAt, start)
	}
}

func TestSessionResponseEventStore_CompleteLateSubscribeCatchUp(t *testing.T) {
	t.Parallel()

	store := newResponseEventStore("session-abc")
	for range 2 {
		if _, err := store.Publish(samplePublishInput()); err != nil {
			t.Fatalf("publish: %v", err)
		}
	}
	store.Complete()

	subscription, err := store.Subscribe(0)
	if err != nil {
		t.Fatalf("Subscribe after complete: %v", err)
	}
	if got := store.SubscriberCount(); got != 0 {
		t.Fatalf("subscriber count after late Subscribe = %d, want 0", got)
	}

	events, err := subscription.Next(context.Background())
	if err != nil {
		t.Fatalf("Next: %v", err)
	}
	if len(events) != 2 {
		t.Fatalf("event count = %d, want 2", len(events))
	}
	if _, err := subscription.Next(context.Background()); !errors.Is(err, responseeventstore.ErrSubscriptionClosed) {
		t.Fatalf("Next after drain error = %v, want ErrSubscriptionClosed", err)
	}
	if got := store.SubscriberCount(); got != 0 {
		t.Fatalf("subscriber count after terminal Next = %d, want 0", got)
	}
}

func TestSessionResponseEventStore_CompletedSubscriptionExpiresAfterRetentionWindow(t *testing.T) {
	t.Parallel()

	start := time.Date(2026, 7, 13, 12, 0, 0, 0, time.UTC)
	clock := &fixedClock{now: start}
	store := responseeventstore.NewSessionResponseEventStoreWithClock("session-abc", clock, testResponseEventID)
	if _, err := store.Publish(samplePublishInput()); err != nil {
		t.Fatalf("publish: %v", err)
	}
	store.Complete()

	clock.Set(start.Add(responseeventstore.CompletedStreamRetentionWindow - time.Nanosecond))
	subscription, err := store.Subscribe(0)
	if err != nil {
		t.Fatalf("Subscribe before retention expiry: %v", err)
	}
	subscription.Detach()

	clock.Set(start.Add(responseeventstore.CompletedStreamRetentionWindow))
	if _, err := store.Subscribe(0); !errors.Is(err, responseeventstore.ErrStoreExpired) {
		t.Fatalf("Subscribe at retention expiry error = %v, want ErrStoreExpired", err)
	}
	if events := store.Events(); len(events) != 1 {
		t.Fatalf("retained snapshot after expiry has %d events, want 1", len(events))
	}
}

func TestSessionResponseEventStore_CompletedSubscriptionUsesConfiguredRetentionWindow(t *testing.T) {
	t.Parallel()

	const customWindow = time.Millisecond
	start := time.Date(2026, 7, 13, 12, 0, 0, 0, time.UTC)
	clock := &fixedClock{now: start}
	limits := responseeventstore.DefaultRetentionLimits()
	limits.CompletedRetentionWindow = customWindow
	store, err := responseeventstore.NewSessionResponseEventStoreWithClockAndLimits(
		"session-abc",
		clock,
		limits,
		testResponseEventID,
	)
	if err != nil {
		t.Fatalf("NewSessionResponseEventStoreWithClockAndLimits: %v", err)
	}
	if _, err := store.Publish(samplePublishInput()); err != nil {
		t.Fatalf("publish: %v", err)
	}
	store.Complete()

	clock.Set(start.Add(customWindow - time.Nanosecond))
	subscription, err := store.Subscribe(0)
	if err != nil {
		t.Fatalf("Subscribe before configured retention expiry: %v", err)
	}
	subscription.Detach()

	clock.Set(start.Add(customWindow))
	if _, err := store.Subscribe(0); !errors.Is(err, responseeventstore.ErrStoreExpired) {
		t.Fatalf("Subscribe at configured retention expiry error = %v, want ErrStoreExpired", err)
	}
}

func TestSessionResponseEventStore_CompleteLateSubscribeAtLatestDoesNotRegister(t *testing.T) {
	t.Parallel()

	store := newResponseEventStore("session-abc")
	if _, err := store.Publish(samplePublishInput()); err != nil {
		t.Fatalf("publish: %v", err)
	}
	store.Complete()

	subscription, err := store.Subscribe(store.LatestSequence())
	if err != nil {
		t.Fatalf("Subscribe after complete at latest: %v", err)
	}
	if _, err := subscription.Next(context.Background()); !errors.Is(err, responseeventstore.ErrSubscriptionClosed) {
		t.Fatalf("Next at completed latest cursor error = %v, want ErrSubscriptionClosed", err)
	}
	if got := store.SubscriberCount(); got != 0 {
		t.Fatalf("subscriber count after terminal Next = %d, want 0", got)
	}
}

func TestSessionResponseEventStore_CloseRejectsPublish(t *testing.T) {
	t.Parallel()

	store := newResponseEventStore("session-abc")
	if _, err := store.Publish(samplePublishInput()); err != nil {
		t.Fatalf("publish before close: %v", err)
	}

	store.Close()

	if _, err := store.Publish(samplePublishInput()); !errors.Is(err, responseeventstore.ErrStoreClosed) {
		t.Fatalf("publish after close error = %v, want ErrStoreClosed", err)
	}
}

func TestSessionResponseEventStore_PublishDoesNotBlockOnSlowSubscriber(t *testing.T) {
	t.Parallel()

	store := newResponseEventStore("session-abc")
	subscription, err := store.Subscribe(0)
	if err != nil {
		t.Fatalf("Subscribe: %v", err)
	}
	defer subscription.Detach()

	blocking := make(chan struct{})
	go func() {
		_, _ = subscription.Next(context.Background())
		close(blocking)
	}()

	select {
	case <-blocking:
		t.Fatal("Next returned before any publish")
	case <-time.After(50 * time.Millisecond):
	}

	done := make(chan struct{})
	go func() {
		for range 64 {
			if _, err := store.Publish(samplePublishInput()); err != nil {
				t.Errorf("publish: %v", err)
				return
			}
		}
		close(done)
	}()

	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("publish blocked on slow subscriber")
	}
}

func TestSessionResponseEventStore_DetachStopsWakeups(t *testing.T) {
	t.Parallel()

	store := newResponseEventStore("session-abc")
	subscription, err := store.Subscribe(0)
	if err != nil {
		t.Fatalf("Subscribe: %v", err)
	}

	subscription.Detach()
	if got := store.SubscriberCount(); got != 0 {
		t.Fatalf("subscriber count after detach = %d, want 0", got)
	}

	for range 8 {
		if _, err := store.Publish(samplePublishInput()); err != nil {
			t.Fatalf("publish after detach: %v", err)
		}
	}

	ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancel()
	if _, err := subscription.Next(ctx); !errors.Is(err, responseeventstore.ErrSubscriptionClosed) {
		t.Fatalf("Next after detach error = %v, want ErrSubscriptionClosed", err)
	}
}

func TestSessionResponseEventStore_LifecycleConcurrentRace(t *testing.T) {
	const workers = 24
	store := newResponseEventStore("session-abc")

	var wg sync.WaitGroup
	wg.Add(workers * 3)
	for i := 0; i < workers; i++ {
		go func() {
			defer wg.Done()
			subscription, err := store.Subscribe(0)
			if err != nil {
				return
			}
			ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
			defer cancel()
			for {
				events, err := subscription.Next(ctx)
				if err != nil {
					subscription.Detach()
					return
				}
				if len(events) == 0 {
					continue
				}
			}
		}()
		go func() {
			defer wg.Done()
			for range 8 {
				_, _ = store.Publish(samplePublishInput())
			}
		}()
		go func() {
			defer wg.Done()
			time.Sleep(time.Duration(i%5) * time.Millisecond)
			switch i % 3 {
			case 0:
				store.Complete()
			case 1:
				subscription, err := store.Subscribe(0)
				if err == nil {
					subscription.Detach()
				}
			default:
				store.Close()
			}
		}()
	}
	wg.Wait()
}

func TestSessionResponseEventStore_CloseAndDetachDoNotLeakGoroutines(t *testing.T) {
	runtime.GC()
	time.Sleep(20 * time.Millisecond)
	baseline := runtime.NumGoroutine()

	for range 8 {
		store := newResponseEventStore("session-abc")
		subscriptions := make([]*responseeventstore.Subscription, 0, 4)
		for range 4 {
			subscription, err := store.Subscribe(0)
			if err != nil {
				t.Fatalf("Subscribe: %v", err)
			}
			subscriptions = append(subscriptions, subscription)
			go func(sub *responseeventstore.Subscription) {
				ctx, cancel := context.WithTimeout(context.Background(), 200*time.Millisecond)
				defer cancel()
				_, _ = sub.Next(ctx)
			}(subscription)
		}
		for range 4 {
			if _, err := store.Publish(samplePublishInput()); err != nil {
				t.Fatalf("publish: %v", err)
			}
		}
		for _, subscription := range subscriptions {
			subscription.Detach()
		}
		store.Complete()
		store.Close()
	}

	runtime.GC()
	time.Sleep(100 * time.Millisecond)
	if leaked := runtime.NumGoroutine() - baseline; leaked > 8 {
		t.Fatalf("goroutine leak: baseline=%d current=%d delta=%d", baseline, runtime.NumGoroutine(), leaked)
	}
}

func TestSessionResponseEventStore_FilteredSubscriptionCompleteDrain(t *testing.T) {
	t.Parallel()

	store := newResponseEventStore("session-abc")
	input := samplePublishInput()
	input.DispatchID = "dispatch-a"
	if _, err := store.Publish(input); err != nil {
		t.Fatalf("publish dispatch-a: %v", err)
	}
	other := samplePublishInput()
	other.DispatchID = "dispatch-b"
	if _, err := store.Publish(other); err != nil {
		t.Fatalf("publish dispatch-b: %v", err)
	}

	subscription, err := store.Subscribe(0, responseeventstore.WithDispatchFilter("dispatch-a"))
	if err != nil {
		t.Fatalf("Subscribe: %v", err)
	}
	defer subscription.Detach()

	store.Complete()

	events, err := subscription.Next(context.Background())
	if err != nil {
		t.Fatalf("Next: %v", err)
	}
	if len(events) != 1 {
		t.Fatalf("filtered event count = %d, want 1", len(events))
	}
	if events[0].DispatchID != "dispatch-a" {
		t.Fatalf("dispatch ID = %q, want dispatch-a", events[0].DispatchID)
	}
	if events[0].Sequence != 1 {
		t.Fatalf("sequence = %d, want global sequence 1", events[0].Sequence)
	}
	if _, err := subscription.Next(context.Background()); !errors.Is(err, responseeventstore.ErrSubscriptionClosed) {
		t.Fatalf("Next after drain error = %v, want ErrSubscriptionClosed", err)
	}
}

func TestSessionResponseEventStore_CompleteIsIdempotent(t *testing.T) {
	t.Parallel()

	store := newResponseEventStore("session-abc")
	store.Complete()
	store.Complete()
	if !store.Completed() {
		t.Fatal("Completed() = false after duplicate Complete")
	}
}

func TestSessionResponseEventStore_CompleteWhileWaitingNextDrainsPublishedEvent(t *testing.T) {
	t.Parallel()

	store := newResponseEventStore("session-abc")
	subscription, err := store.Subscribe(0)
	if err != nil {
		t.Fatalf("Subscribe: %v", err)
	}
	defer subscription.Detach()

	waiting := make(chan struct{})
	done := make(chan struct{})
	go func() {
		close(waiting)
		defer close(done)
		events, err := subscription.Next(context.Background())
		if err != nil {
			t.Errorf("Next: %v", err)
			return
		}
		if len(events) != 1 {
			t.Errorf("event count = %d, want 1", len(events))
			return
		}
		if events[0].Sequence != 1 {
			t.Errorf("sequence = %d, want 1", events[0].Sequence)
		}
		if _, err := subscription.Next(context.Background()); !errors.Is(err, responseeventstore.ErrSubscriptionClosed) {
			t.Errorf("Next after drain error = %v, want ErrSubscriptionClosed", err)
		}
	}()

	select {
	case <-waiting:
	case <-time.After(time.Second):
		t.Fatal("subscriber did not start waiting")
	}

	time.Sleep(20 * time.Millisecond)

	if _, err := store.Publish(samplePublishInput()); err != nil {
		t.Fatalf("publish while subscriber waiting: %v", err)
	}
	store.Complete()

	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("timed out waiting for subscriber to drain published event after complete")
	}
}

func TestSessionResponseEventStore_NextCanceledContextDrainsRetainedEvents(t *testing.T) {
	t.Parallel()

	store := newResponseEventStore("session-abc")
	if _, err := store.Publish(samplePublishInput()); err != nil {
		t.Fatalf("publish: %v", err)
	}

	subscription, err := store.Subscribe(0)
	if err != nil {
		t.Fatalf("Subscribe: %v", err)
	}
	defer subscription.Detach()

	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	events, err := subscription.Next(ctx)
	if err != nil {
		t.Fatalf("Next with canceled context and retained events: %v", err)
	}
	if len(events) != 1 {
		t.Fatalf("event count = %d, want 1", len(events))
	}
	if events[0].Sequence != 1 {
		t.Fatalf("sequence = %d, want 1", events[0].Sequence)
	}
}

func TestSessionResponseEventStore_DrainReturnsRetainedEvents(t *testing.T) {
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

	events, err := subscription.Drain()
	if err != nil {
		t.Fatalf("Drain: %v", err)
	}
	if len(events) != 2 {
		t.Fatalf("drained event count = %d, want 2", len(events))
	}
	if events[0].Sequence != 1 || events[1].Sequence != 2 {
		t.Fatalf("sequences = [%d,%d], want [1,2]", events[0].Sequence, events[1].Sequence)
	}

	events, err = subscription.Drain()
	if err != nil {
		t.Fatalf("Drain after cursor advance: %v", err)
	}
	if len(events) != 0 {
		t.Fatalf("second drain event count = %d, want 0", len(events))
	}
}

func TestSessionResponseEventStore_DetachWhileWaitingNextReturnsPromptly(t *testing.T) {
	t.Parallel()

	store := newResponseEventStore("session-abc")
	subscription, err := store.Subscribe(0)
	if err != nil {
		t.Fatalf("Subscribe: %v", err)
	}

	waiting := make(chan struct{})
	closed := make(chan error, 1)
	go func() {
		close(waiting)
		_, err := subscription.Next(context.Background())
		closed <- err
	}()

	select {
	case <-waiting:
	case <-time.After(time.Second):
		t.Fatal("subscriber did not start waiting")
	}

	time.Sleep(20 * time.Millisecond)

	start := time.Now()
	subscription.Detach()

	select {
	case err := <-closed:
		if !errors.Is(err, responseeventstore.ErrSubscriptionClosed) {
			t.Fatalf("Next after detach while waiting error = %v, want ErrSubscriptionClosed", err)
		}
	case <-time.After(500 * time.Millisecond):
		t.Fatal("Next did not return promptly after Detach while waiting")
	}
	if elapsed := time.Since(start); elapsed > 200*time.Millisecond {
		t.Fatalf("Next took %v to return after Detach, want prompt return", elapsed)
	}

	if _, err := store.Publish(samplePublishInput()); err != nil {
		t.Fatalf("publish after detach: %v", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancel()
	if _, err := subscription.Next(ctx); !errors.Is(err, responseeventstore.ErrSubscriptionClosed) {
		t.Fatalf("Next after post-detach publish error = %v, want ErrSubscriptionClosed", err)
	}
}
