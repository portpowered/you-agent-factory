package responsestream_test

import (
	"context"
	"errors"
	"testing"

	"github.com/portpowered/infinite-you/pkg/factory/sessions/responsestream"
)

func TestSessionResponseStreamPublishesAndCompletesOneOrderedDispatch(t *testing.T) {
	t.Parallel()

	stream := responsestream.NewSessionResponseStream()
	subscription, err := stream.Subscribe(0)
	if err != nil {
		t.Fatalf("Subscribe() error = %v", err)
	}
	t.Cleanup(subscription.Detach)

	assertPublishedOrderedResponseFragment(t, stream, subscription)
	assertCompletedOrderedDispatch(t, stream, subscription)
}

func assertPublishedOrderedResponseFragment(t *testing.T, stream *responsestream.SessionResponseStream, subscription *responsestream.Subscription) {
	t.Helper()

	stored, compaction := stream.Append(responsestream.Event{
		Kind:       responsestream.EventKindResponseFragment,
		Type:       responsestream.EventTypeTextDelta,
		DispatchID: "dispatch-1",
		Payload:    "hello",
	})
	if compaction != nil || stored.Sequence != 1 {
		t.Fatalf("Append() = sequence %d, compaction %+v; want sequence 1 without compaction", stored.Sequence, compaction)
	}

	result, err := subscription.Next(context.Background())
	if err != nil {
		t.Fatalf("Next() error = %v", err)
	}
	if len(result.Events) != 1 || result.Events[0].Payload != "hello" {
		t.Fatalf("Next() events = %+v, want one ordered response fragment", result.Events)
	}
	if got := stream.RetentionAccounting(); got.EventCount != 1 || got.TotalPayloadBytes != len("hello") {
		t.Fatalf("RetentionAccounting() = %+v, want one five-byte event", got)
	}
	if stream.LatestSequence() != 1 || len(stream.EventsAfter(0).Events) != 1 || stream.SubscriberCount() != 1 {
		t.Fatalf("live stream state = latest %d, retained %d, subscribers %d; want 1, 1, 1", stream.LatestSequence(), len(stream.EventsAfter(0).Events), stream.SubscriberCount())
	}
}

func assertCompletedOrderedDispatch(t *testing.T, stream *responsestream.SessionResponseStream, subscription *responsestream.Subscription) {
	t.Helper()

	stream.CompleteDispatch()
	if !stream.DispatchCompleted() || stream.DispatchCompletedAt().IsZero() || stream.SubscriberCount() != 0 {
		t.Fatalf("completed stream state = completed %t at %v with %d subscribers", stream.DispatchCompleted(), stream.DispatchCompletedAt(), stream.SubscriberCount())
	}
	if _, err := subscription.Next(context.Background()); !errors.Is(err, responsestream.ErrSubscriptionClosed) {
		t.Fatalf("Next() after completion error = %v, want %v", err, responsestream.ErrSubscriptionClosed)
	}
	stream.Append(responsestream.Event{Payload: "ignored after completion"})
	if got := stream.Events(); len(got) != 1 {
		t.Fatalf("len(Events()) after completion append = %d, want retained event only", len(got))
	}
}
