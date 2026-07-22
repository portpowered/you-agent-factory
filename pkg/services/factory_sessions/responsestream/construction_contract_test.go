package responsestream_test

import (
	"context"
	"errors"
	"testing"

	"github.com/portpowered/infinite-you/pkg/services/factory_sessions/responsestream"
)

// pkgmaintcheck:ignore-cyclomatic-complexity service-ownership migration preserves this decision flow; simplify branches and remove this exemption.
func TestPublishesAndCompletesOneOrderedDispatch(t *testing.T) {
	t.Parallel()

	stream := newResponseStream()
	subscription, err := stream.Subscribe(0)
	if err != nil {
		t.Fatalf("Subscribe() error = %v", err)
	}
	t.Cleanup(subscription.Detach)

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

func TestResponseStreamConstructionRequiresExplicitClockAndFactories(t *testing.T) {
	t.Parallel()

	if stream := responsestream.NewSessionResponseStream(nil); stream != nil {
		t.Fatalf("NewSessionResponseStream without clock = %#v, want nil", stream)
	}
	if set := responsestream.NewStreamSet(nil); set != nil {
		t.Fatalf("NewStreamSet without clock = %#v, want nil", set)
	}
	if set := responsestream.NewStreamSetWithFactory(nil, &fixedClock{}); set != nil {
		t.Fatalf("NewStreamSetWithFactory without stream factory = %#v, want nil", set)
	}
	if registry := responsestream.NewRegistry(nil, &fixedClock{}); registry != nil {
		t.Fatalf("NewRegistry without stream factory = %#v, want nil", registry)
	}
	if registry := responsestream.NewRegistry(newResponseStream, nil); registry != nil {
		t.Fatalf("NewRegistry without clock = %#v, want nil", registry)
	}
}
