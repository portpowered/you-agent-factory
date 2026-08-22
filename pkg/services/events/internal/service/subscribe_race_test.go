package service

import (
	"context"
	"encoding/json"
	"fmt"
	"runtime"
	"sync"
	"testing"
	"time"

	"github.com/portpowered/infinite-you/pkg/services/events"
)

// TestSubscribe_ConcurrentSubscribersObserveContiguousLiveHistory proves that
// many independent live subscriptions, started concurrently with ongoing
// appends, each observe every committed position exactly once and in
// increasing order, with no subscriber's progress affecting another's. Run
// with -race to additionally prove liveSubscriber/topicState.subscribers is
// free of data races under concurrent Append+Subscribe+Next contention (see
// the repository-wide -race sandbox limitation noted in append_race_test.go
// and read_race_test.go).
func TestSubscribe_ConcurrentSubscribersObserveContiguousLiveHistory(t *testing.T) {
	const totalAppends = 200
	const subscribers = 20

	st := New()
	ctx := context.Background()
	topic := events.Topic("chat-session/concurrent-subscribe/events")

	var subs []events.Subscription
	for range subscribers {
		sub, err := st.Subscribe(ctx, events.SubscribeRequest{Topic: topic, From: events.Cursor{Topic: topic}, Limit: totalAppends})
		if err != nil {
			t.Fatalf("Subscribe() error = %v", err)
		}
		subs = append(subs, sub)
	}

	var appendWG sync.WaitGroup
	appendWG.Go(func() {
		for i := 1; i <= totalAppends; i++ {
			req := events.AppendRequest{
				Topic:          topic,
				SourceType:     "worker.tool",
				SourceID:       "worker-1",
				SourceSequence: events.SourceSequence(i),
				SourceEventID:  events.SourceEventID(fmt.Sprintf("evt-%d", i)),
				SchemaID:       "worker.output.v1",
				Payload:        json.RawMessage(`{"ok":true}`),
			}
			if _, err := st.Append(ctx, req); err != nil {
				t.Errorf("Append() error = %v", err)
				return
			}
		}
	})

	var subWG sync.WaitGroup
	errs := make(chan error, subscribers)
	for _, sub := range subs {
		subWG.Go(func() {
			var last events.AggregateSequence
			for last < totalAppends {
				delivery := sub.Next(ctx)
				if delivery.Kind != events.DeliveryRecord {
					errs <- fmt.Errorf("unexpected Delivery.Kind = %v", delivery.Kind)
					return
				}
				if err := delivery.Validate(); err != nil {
					errs <- fmt.Errorf("Delivery.Validate() = %w", err)
					return
				}
				want := last + 1
				if delivery.Record.ID.Position != want {
					errs <- fmt.Errorf("expected contiguous position %d, got %d", want, delivery.Record.ID.Position)
					return
				}
				last = delivery.Record.ID.Position
			}
		})
	}

	appendWG.Wait()
	subWG.Wait()
	close(errs)

	for err := range errs {
		t.Fatalf("subscriber error: %v", err)
	}
}

// TestSubscribe_ConcurrentAppendDuringCancellationNeverPanics proves that a
// context cancellation racing a concurrent Append targeting the same
// subscriber never panics with a send on a closed channel. Before the fix,
// subscriptionState.cancel closed the live subscriber's buffered channel
// (liveSubscriber.terminate) before unregistering it from the topic's
// subscriber map (Store.unregisterSubscriber); Append's
// notifySubscribersLocked holds only the topic's own mutex while sending,
// which does not serialize against that close, so a send arriving during the
// close window would panic. Cancellation now unregisters (acquiring the same
// topic mutex Append's send is performed under) strictly before closing the
// channel, which serializes the two and makes the race unreachable. Many
// rounds with unsynchronized goroutines are used because the panic is a
// genuine timing race, not deterministically reproducible in one iteration;
// this test crashing the process (a panic is never recovered by `go test`)
// is itself the regression signal, no explicit assertion is needed to detect
// it.
func TestSubscribe_ConcurrentAppendDuringCancellationNeverPanics(t *testing.T) {
	const rounds = 500
	st := New()
	topic := events.Topic("chat-session/cancel-append-race/events")

	for i := range rounds {
		subCtx, cancel := context.WithCancel(context.Background())
		sub, err := st.Subscribe(subCtx, events.SubscribeRequest{Topic: topic, From: events.Cursor{Topic: topic}, Limit: 1})
		if err != nil {
			t.Fatalf("round %d: Subscribe() error = %v", i, err)
		}

		var wg sync.WaitGroup
		wg.Add(2)
		go func(seq int) {
			defer wg.Done()
			appendOne(t, st, context.Background(), topic, seq)
		}(i + 1)
		go func() {
			defer wg.Done()
			cancel()
		}()

		delivery := sub.Next(subCtx)
		if delivery.Kind != events.DeliveryRecord && delivery.Kind != events.DeliveryCanceled {
			t.Fatalf("round %d: Next().Kind = %v, want DeliveryRecord or DeliveryCanceled", i, delivery.Kind)
		}
		wg.Wait()
		cancel()
	}
}

// TestSubscribe_NoGoroutineLeakAcrossSubscribeNextAndClose proves that
// Subscribe/Next/Close never leave a background goroutine running: this
// Store's Subscribe spawns no goroutine of its own (Next performs its
// blocking wait on the caller's own goroutine), so the only source of a
// leak would be an internal implementation mistake such as an unclosed
// channel forcing a stray goroutine to block forever.
func TestSubscribe_NoGoroutineLeakAcrossSubscribeNextAndClose(t *testing.T) {
	runtime.GC()
	baseline := runtime.NumGoroutine()

	st := New()
	ctx := context.Background()
	topic := events.Topic("chat-session/goroutine-leak/events")

	var subs []events.Subscription
	for range 10 {
		sub, err := st.Subscribe(ctx, events.SubscribeRequest{Topic: topic, From: events.Cursor{Topic: topic}, Limit: 4})
		if err != nil {
			t.Fatalf("Subscribe() error = %v", err)
		}
		subs = append(subs, sub)
	}

	appendOne(t, st, ctx, topic, 1)
	for _, sub := range subs {
		if delivery := sub.Next(ctx); delivery.Kind != events.DeliveryRecord {
			t.Fatalf("Next().Kind = %v, want DeliveryRecord", delivery.Kind)
		}
	}

	if err := st.Close(ctx); err != nil {
		t.Fatalf("Close() error = %v", err)
	}
	for _, sub := range subs {
		if delivery := sub.Next(ctx); delivery.Kind != events.DeliveryClosed {
			t.Fatalf("Next().Kind after Close() = %v, want DeliveryClosed", delivery.Kind)
		}
	}

	deadline := time.Now().Add(2 * time.Second)
	for {
		runtime.GC()
		if leaked := runtime.NumGoroutine() - baseline; leaked <= 0 {
			return
		} else if time.Now().After(deadline) {
			t.Fatalf("goroutine leak: baseline=%d current=%d delta=%d", baseline, runtime.NumGoroutine(), leaked)
		}
		time.Sleep(10 * time.Millisecond)
	}
}
