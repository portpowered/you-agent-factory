package service

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/portpowered/infinite-you/pkg/services/events"
)

const subscribeTestTopic events.Topic = "chat-session/subscribe/events"

func mustSubscribe(t *testing.T, st *Store, ctx context.Context, req events.SubscribeRequest) events.Subscription {
	t.Helper()
	sub, err := st.Subscribe(ctx, req)
	if err != nil {
		t.Fatalf("Subscribe() error = %v", err)
	}
	return sub
}

func TestSubscribe_LiveOnlyFromCurrentHeadDoesNotReplayRetainedRecords(t *testing.T) {
	st := New()
	ctx := context.Background()
	appendN(t, st, ctx, subscribeTestTopic, 3)

	sub := mustSubscribe(t, st, ctx, events.SubscribeRequest{
		Topic: subscribeTestTopic,
		From:  events.Cursor{Topic: subscribeTestTopic, Position: 3},
		Limit: 10,
	})

	appendOne(t, st, ctx, subscribeTestTopic, 4)

	delivery := sub.Next(ctx)
	if delivery.Kind != events.DeliveryRecord {
		t.Fatalf("Next().Kind = %v, want DeliveryRecord", delivery.Kind)
	}
	if err := delivery.Validate(); err != nil {
		t.Fatalf("Delivery.Validate() error = %v", err)
	}
	if delivery.Record.ID.Position != 4 {
		t.Fatalf("Record.ID.Position = %d, want 4 (no replay of positions 1-3)", delivery.Record.ID.Position)
	}
}

func TestSubscribe_RetainedThenLiveDeliversContiguousPositionsAcrossHandoff(t *testing.T) {
	st := New()
	ctx := context.Background()
	appendN(t, st, ctx, subscribeTestTopic, 3)

	sub := mustSubscribe(t, st, ctx, events.SubscribeRequest{
		Topic: subscribeTestTopic,
		From:  events.Cursor{Topic: subscribeTestTopic},
		Limit: 10,
	})

	// Commit two more records during the handoff, before any retained record
	// has actually been drained by the subscriber.
	appendOne(t, st, ctx, subscribeTestTopic, 4)
	appendOne(t, st, ctx, subscribeTestTopic, 5)

	var positions []events.AggregateSequence
	for i := range 5 {
		delivery := sub.Next(ctx)
		if delivery.Kind != events.DeliveryRecord {
			t.Fatalf("Next() [%d].Kind = %v, want DeliveryRecord", i, delivery.Kind)
		}
		if err := delivery.Validate(); err != nil {
			t.Fatalf("Next() [%d] Delivery.Validate() error = %v", i, err)
		}
		positions = append(positions, delivery.Record.ID.Position)
	}

	for i, pos := range positions {
		if pos != events.AggregateSequence(i+1) {
			t.Fatalf("positions = %v, want contiguous 1..5 with no missing or duplicate position", positions)
		}
	}
}

func TestSubscribe_MultipleSubscribersAreIndependent(t *testing.T) {
	st := New()
	ctx := context.Background()
	appendN(t, st, ctx, subscribeTestTopic, 2)

	subA := mustSubscribe(t, st, ctx, events.SubscribeRequest{Topic: subscribeTestTopic, From: events.Cursor{Topic: subscribeTestTopic}, Limit: 10})
	subB := mustSubscribe(t, st, ctx, events.SubscribeRequest{Topic: subscribeTestTopic, From: events.Cursor{Topic: subscribeTestTopic, Position: 1}, Limit: 10})

	// A has not consumed anything yet; B starts one position ahead. Draining
	// one from A must not affect B's next delivery, and vice versa.
	deliveryA1 := subA.Next(ctx)
	if deliveryA1.Record.ID.Position != 1 {
		t.Fatalf("A first Position = %d, want 1", deliveryA1.Record.ID.Position)
	}
	deliveryB1 := subB.Next(ctx)
	if deliveryB1.Record.ID.Position != 2 {
		t.Fatalf("B first Position = %d, want 2", deliveryB1.Record.ID.Position)
	}
	deliveryA2 := subA.Next(ctx)
	if deliveryA2.Record.ID.Position != 2 {
		t.Fatalf("A second Position = %d, want 2 (B's read must not have advanced A)", deliveryA2.Record.ID.Position)
	}
}

func TestSubscribe_GapReportsEvictedStartingPositionThenResumes(t *testing.T) {
	st := NewWithRetention(2)
	ctx := context.Background()
	appendN(t, st, ctx, subscribeTestTopic, 5) // retains only positions 4,5; evicts 1-3

	sub := mustSubscribe(t, st, ctx, events.SubscribeRequest{
		Topic: subscribeTestTopic,
		From:  events.Cursor{Topic: subscribeTestTopic, Position: 1},
		Limit: 10,
	})

	gapDelivery := sub.Next(ctx)
	if gapDelivery.Kind != events.DeliveryGap {
		t.Fatalf("Next().Kind = %v, want DeliveryGap", gapDelivery.Kind)
	}
	if err := gapDelivery.Validate(); err != nil {
		t.Fatalf("Delivery.Validate() error = %v", err)
	}
	if gapDelivery.Gap.Requested != 1 || gapDelivery.Gap.EarliestRetained != 4 || gapDelivery.Gap.Head != 5 {
		t.Fatalf("Gap = %+v, want Requested=1 EarliestRetained=4 Head=5", gapDelivery.Gap)
	}

	// The subscription must recover deterministically: catch-up resumes from
	// the earliest retained position with no fabricated or skipped record.
	next := sub.Next(ctx)
	if next.Kind != events.DeliveryRecord || next.Record.ID.Position != 4 {
		t.Fatalf("Next() after gap = %+v, want DeliveryRecord at position 4", next)
	}
}

func TestSubscribe_UnresolvableCursorAheadOfHeadReturnsError(t *testing.T) {
	st := New()
	ctx := context.Background()
	appendN(t, st, ctx, subscribeTestTopic, 2)

	_, err := st.Subscribe(ctx, events.SubscribeRequest{
		Topic: subscribeTestTopic,
		From:  events.Cursor{Topic: subscribeTestTopic, Position: 99},
		Limit: 10,
	})
	if !errors.Is(err, events.ErrUnresolvableCursor) {
		t.Fatalf("Subscribe() error = %v, want ErrUnresolvableCursor", err)
	}
}

func TestSubscribe_ContextCancellationReturnsCanceled(t *testing.T) {
	st := New()
	ctx := context.Background()
	sub := mustSubscribe(t, st, ctx, events.SubscribeRequest{Topic: subscribeTestTopic, From: events.Cursor{Topic: subscribeTestTopic}, Limit: 10})

	subCtx, cancel := context.WithCancel(context.Background())
	cancel()

	delivery := sub.Next(subCtx)
	if delivery.Kind != events.DeliveryCanceled {
		t.Fatalf("Next().Kind = %v, want DeliveryCanceled", delivery.Kind)
	}
	if err := delivery.Validate(); err != nil {
		t.Fatalf("Delivery.Validate() error = %v", err)
	}
}

func TestSubscribe_ContextCancellationWhileBlockedReleasesNext(t *testing.T) {
	st := New()
	ctx := context.Background()
	sub := mustSubscribe(t, st, ctx, events.SubscribeRequest{Topic: subscribeTestTopic, From: events.Cursor{Topic: subscribeTestTopic}, Limit: 10})

	subCtx, cancel := context.WithCancel(context.Background())
	result := make(chan events.Delivery, 1)
	go func() {
		result <- sub.Next(subCtx)
	}()

	cancel()

	select {
	case delivery := <-result:
		if delivery.Kind != events.DeliveryCanceled {
			t.Fatalf("Next().Kind = %v, want DeliveryCanceled", delivery.Kind)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("Next() did not return after context cancellation")
	}
}

func TestSubscribe_BackpressureTerminatesConsumerWithoutSilentLoss(t *testing.T) {
	st := New()
	ctx := context.Background()

	sub := mustSubscribe(t, st, ctx, events.SubscribeRequest{Topic: subscribeTestTopic, From: events.Cursor{Topic: subscribeTestTopic}, Limit: 2})

	// Never drain: commit more live records than the bounded buffer (2) can
	// hold so Append must detect and terminate the subscriber.
	appendN(t, st, ctx, subscribeTestTopic, 5)

	var sawBackpressure bool
	var delivered int
	for i := range 10 {
		delivery := sub.Next(ctx)
		if delivery.Kind == events.DeliveryBackpressure {
			sawBackpressure = true
			if err := delivery.Validate(); err != nil {
				t.Fatalf("Delivery.Validate() error = %v", err)
			}
			break
		}
		if delivery.Kind != events.DeliveryRecord {
			t.Fatalf("Next() [%d].Kind = %v, want DeliveryRecord or terminal DeliveryBackpressure", i, delivery.Kind)
		}
		delivered++
	}
	if !sawBackpressure {
		t.Fatalf("subscriber never observed DeliveryBackpressure; delivered=%d records", delivered)
	}

	// Repeated observation after backpressure is deterministic.
	again := sub.Next(ctx)
	if again.Kind != events.DeliveryBackpressure {
		t.Fatalf("repeated Next().Kind = %v, want DeliveryBackpressure (deterministic terminal outcome)", again.Kind)
	}
}

func TestSubscribe_StoreCloseTerminatesBlockedSubscriberWithClosed(t *testing.T) {
	st := New()
	ctx := context.Background()
	sub := mustSubscribe(t, st, ctx, events.SubscribeRequest{Topic: subscribeTestTopic, From: events.Cursor{Topic: subscribeTestTopic}, Limit: 10})

	result := make(chan events.Delivery, 1)
	go func() {
		result <- sub.Next(ctx)
	}()

	if err := st.Close(ctx); err != nil {
		t.Fatalf("Close() error = %v", err)
	}

	select {
	case delivery := <-result:
		if delivery.Kind != events.DeliveryClosed {
			t.Fatalf("Next().Kind = %v, want DeliveryClosed", delivery.Kind)
		}
		if err := delivery.Validate(); err != nil {
			t.Fatalf("Delivery.Validate() error = %v", err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("Next() did not return after Store.Close()")
	}

	again := sub.Next(ctx)
	if again.Kind != events.DeliveryClosed {
		t.Fatalf("repeated Next().Kind after Close() = %v, want DeliveryClosed (deterministic terminal outcome)", again.Kind)
	}
}

func TestSubscribe_AfterStoreCloseObservesClosedWithoutRegistering(t *testing.T) {
	st := New()
	ctx := context.Background()
	if err := st.Close(ctx); err != nil {
		t.Fatalf("Close() error = %v", err)
	}

	sub := mustSubscribe(t, st, ctx, events.SubscribeRequest{Topic: subscribeTestTopic, From: events.Cursor{Topic: subscribeTestTopic}, Limit: 10})
	delivery := sub.Next(ctx)
	if delivery.Kind != events.DeliveryClosed {
		t.Fatalf("Next().Kind = %v, want DeliveryClosed for a topic created after Store.Close()", delivery.Kind)
	}
}

func TestSubscribe_CloseIsIdempotent(t *testing.T) {
	st := New()
	ctx := context.Background()
	if err := st.Close(ctx); err != nil {
		t.Fatalf("Close() error = %v", err)
	}
	if err := st.Close(ctx); err != nil { // must not panic (double-close of already-closed channels/state)
		t.Fatalf("second Close() error = %v", err)
	}
}

func TestSubscribe_DeliveredRecordsAreDetached(t *testing.T) {
	st := New()
	ctx := context.Background()
	sub := mustSubscribe(t, st, ctx, events.SubscribeRequest{Topic: subscribeTestTopic, From: events.Cursor{Topic: subscribeTestTopic}, Limit: 10})

	appendOne(t, st, ctx, subscribeTestTopic, 1)
	delivery := sub.Next(ctx)
	if delivery.Kind != events.DeliveryRecord {
		t.Fatalf("Next().Kind = %v, want DeliveryRecord", delivery.Kind)
	}
	delivery.Record.Payload[2] = 'X'

	readResult, err := st.Read(ctx, events.ReadRequest{Topic: subscribeTestTopic, From: events.Cursor{Topic: subscribeTestTopic}, Limit: 10})
	if err != nil {
		t.Fatalf("Read() error = %v", err)
	}
	want := `{"tool":"grep","status":"ok"}`
	if string(readResult.Records[0].Payload) != want {
		t.Fatalf("Read() Payload = %s, want unaffected by mutation of a delivered Delivery.Record", readResult.Records[0].Payload)
	}
}

func TestSubscribe_RejectsMalformedRequestBeforeAnyStateChange(t *testing.T) {
	st := New()
	ctx := context.Background()

	_, err := st.Subscribe(ctx, events.SubscribeRequest{Topic: subscribeTestTopic, From: events.Cursor{Topic: subscribeTestTopic}, Limit: 0})
	if !errors.Is(err, events.ErrInvalidReadLimit) {
		t.Fatalf("Subscribe() error = %v, want ErrInvalidReadLimit", err)
	}

	result, err := st.Append(ctx, validRequestForTopic(subscribeTestTopic))
	if err != nil {
		t.Fatalf("Append() error = %v", err)
	}
	if result.Record.ID.Position != 1 {
		t.Fatalf("Position = %d, want 1 (rejected Subscribe must not change aggregate state)", result.Record.ID.Position)
	}
}

func TestSubscribe_RejectsCanceledContextBeforeAnyStateChange(t *testing.T) {
	st := New()
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	_, err := st.Subscribe(ctx, events.SubscribeRequest{Topic: subscribeTestTopic, From: events.Cursor{Topic: subscribeTestTopic}, Limit: 10})
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("Subscribe() error = %v, want context.Canceled", err)
	}
}

func appendOne(t *testing.T, st *Store, ctx context.Context, topic events.Topic, sourceSequence int) {
	t.Helper()
	req := validAppendRequest()
	req.Topic = topic
	req.SourceSequence = events.SourceSequence(sourceSequence)
	req.SourceEventID = events.SourceEventID("evt-" + itoa(sourceSequence))
	if _, err := st.Append(ctx, req); err != nil {
		t.Fatalf("Append() error = %v", err)
	}
}

func validRequestForTopic(topic events.Topic) events.AppendRequest {
	req := validAppendRequest()
	req.Topic = topic
	return req
}
