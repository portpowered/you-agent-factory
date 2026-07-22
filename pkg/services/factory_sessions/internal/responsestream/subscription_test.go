package responsestream_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/portpowered/infinite-you/pkg/services/factory_sessions/internal/responsestream"
)

func TestSessionResponseStreamSubscription_ReadsRetainedThenLiveEventsInOrder(t *testing.T) {
	stream := newResponseStream()
	publisher := responsestream.NewPublisher(stream, nil)
	first := publisher.Publish(progressEvent("dispatch-1", "retained-1"))
	second := publisher.Publish(progressEvent("dispatch-1", "retained-2"))

	subscription, err := stream.Subscribe(0)
	if err != nil {
		t.Fatalf("Subscribe: %v", err)
	}
	defer subscription.Detach()

	initial, err := subscription.Next(context.Background())
	if err != nil {
		t.Fatalf("Next(initial): %v", err)
	}
	if len(initial.Events) != 2 {
		t.Fatalf("initial event count = %d, want 2", len(initial.Events))
	}
	if initial.Events[0].Sequence != first.Sequence || initial.Events[1].Sequence != second.Sequence {
		t.Fatalf("initial sequences = %#v, want [%d %d]", []int64{initial.Events[0].Sequence, initial.Events[1].Sequence}, first.Sequence, second.Sequence)
	}

	third := publisher.Publish(progressEvent("dispatch-1", "live-3"))
	live, err := subscription.Next(context.Background())
	if err != nil {
		t.Fatalf("Next(live): %v", err)
	}
	if len(live.Events) != 1 || live.Events[0].Sequence != third.Sequence || live.Events[0].Payload != "live-3" {
		t.Fatalf("live events = %#v, want retained live event", live.Events)
	}
}

func TestSessionResponseStreamSubscription_DetachStopsFurtherDelivery(t *testing.T) {
	stream := newResponseStream()
	subscription, err := stream.Subscribe(0)
	if err != nil {
		t.Fatalf("Subscribe: %v", err)
	}

	if got := stream.SubscriberCount(); got != 1 {
		t.Fatalf("subscriber count = %d, want 1", got)
	}
	subscription.Detach()
	if got := stream.SubscriberCount(); got != 0 {
		t.Fatalf("subscriber count after detach = %d, want 0", got)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancel()
	if _, err := subscription.Next(ctx); !errors.Is(err, responsestream.ErrSubscriptionClosed) {
		t.Fatalf("Next after detach error = %v, want ErrSubscriptionClosed", err)
	}
}

func TestSessionResponseStreamSubscription_CloseReleasesAllSubscribers(t *testing.T) {
	stream := newResponseStream()
	first, err := stream.Subscribe(0)
	if err != nil {
		t.Fatalf("Subscribe(first): %v", err)
	}
	second, err := stream.Subscribe(0)
	if err != nil {
		t.Fatalf("Subscribe(second): %v", err)
	}

	if got := stream.SubscriberCount(); got != 2 {
		t.Fatalf("subscriber count = %d, want 2", got)
	}
	stream.Close()
	if got := stream.SubscriberCount(); got != 0 {
		t.Fatalf("subscriber count after close = %d, want 0", got)
	}

	for _, subscription := range []*responsestream.Subscription{first, second} {
		if _, err := subscription.Next(context.Background()); !errors.Is(err, responsestream.ErrSubscriptionClosed) {
			t.Fatalf("Next after close error = %v, want ErrSubscriptionClosed", err)
		}
	}
}
