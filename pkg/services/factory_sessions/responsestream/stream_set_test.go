package responsestream_test

import (
	"context"
	"errors"
	"sync/atomic"
	"testing"
	"time"

	"github.com/portpowered/infinite-you/pkg/services/factory_sessions/responsestream"
)

func TestStreamSet_ReusesDispatchStreamAndSeparatesOtherDispatches(t *testing.T) {
	set := newStreamSet()

	alphaFirst := set.Stream("dispatch-alpha")
	alphaSecond := set.Stream(" dispatch-alpha ")
	beta := set.Stream("dispatch-beta")

	if alphaFirst == nil || beta == nil {
		t.Fatal("stream = nil, want allocated dispatch stream")
	}
	if alphaFirst != alphaSecond {
		t.Fatal("same dispatch returned different streams")
	}
	if alphaFirst == beta {
		t.Fatal("different dispatches shared a stream")
	}
	if got := set.Count(); got != 2 {
		t.Fatalf("stream count = %d, want 2", got)
	}
	if got := set.DispatchIDs(); len(got) != 2 || got[0] != "dispatch-alpha" || got[1] != "dispatch-beta" {
		t.Fatalf("dispatch ids = %#v, want sorted identities", got)
	}
}

func TestStreamSet_StreamFactoryRunsOncePerDispatch(t *testing.T) {
	var calls atomic.Int32
	set := newStreamSetWithFactory(func() *responsestream.SessionResponseStream {
		calls.Add(1)
		return newResponseStream()
	})

	first := set.Stream("dispatch-1")
	second := set.Stream("dispatch-1")
	third := set.Stream("dispatch-2")

	if first == nil || second == nil || third == nil {
		t.Fatal("stream = nil, want allocated streams")
	}
	if first != second {
		t.Fatal("same dispatch returned different streams")
	}
	if got := calls.Load(); got != 2 {
		t.Fatalf("factory calls = %d, want 2", got)
	}
}

func TestStreamSet_CloseDispatchDetachesSubscribersAndRetainsStream(t *testing.T) {
	set := newStreamSet()
	assertLiveSubscriberClosedOnDispatchComplete(t, set)
	assertLateSubscriberDrainsRetainedDispatch(t, set)
}

func assertLiveSubscriberClosedOnDispatchComplete(t *testing.T, set *responsestream.StreamSet) {
	t.Helper()

	liveSubscription, err := set.Subscribe("dispatch-1", 0)
	if err != nil {
		t.Fatalf("Subscribe: %v", err)
	}
	if got := set.SubscriberCount("dispatch-1"); got != 1 {
		t.Fatalf("subscriber count = %d, want 1", got)
	}
	if closed := set.CloseDispatch("dispatch-1"); !closed {
		t.Fatal("CloseDispatch returned false, want dispatch completed")
	}
	if got := set.SubscriberCount("dispatch-1"); got != 0 {
		t.Fatalf("subscriber count after close = %d, want 0", got)
	}
	if _, err := liveSubscription.Next(context.Background()); !errors.Is(err, responsestream.ErrSubscriptionClosed) {
		t.Fatalf("Next after dispatch close error = %v, want ErrSubscriptionClosed", err)
	}
}

func assertLateSubscriberDrainsRetainedDispatch(t *testing.T, set *responsestream.StreamSet) {
	t.Helper()

	stream := set.Stream("dispatch-2")
	if stream == nil {
		t.Fatal("Stream = nil, want allocated dispatch stream")
	}
	stream.Append(responsestream.Event{
		Kind:    responsestream.EventKindProgressFragment,
		Type:    responsestream.EventTypeProgress,
		Payload: "planning",
	})
	if closed := set.CloseDispatch("dispatch-2"); !closed {
		t.Fatal("CloseDispatch for retained dispatch returned false")
	}
	if got := set.Count(); got != 2 {
		t.Fatalf("stream count after retained close = %d, want both dispatch streams", got)
	}
	if retained := set.Stream("dispatch-2"); retained == nil {
		t.Fatal("Stream after dispatch close = nil, want retained dispatch stream")
	}

	lateSubscription, err := set.Subscribe("dispatch-2", 0)
	if err != nil {
		t.Fatalf("Subscribe after dispatch close: %v", err)
	}
	result, err := lateSubscription.Next(context.Background())
	if err != nil {
		t.Fatalf("Next after late subscribe: %v", err)
	}
	if len(result.Events) != 1 || result.Events[0].Payload != "planning" {
		t.Fatalf("late subscribe events = %#v, want retained planning progress", result.Events)
	}
	if _, err := lateSubscription.Next(context.Background()); !errors.Is(err, responsestream.ErrSubscriptionClosed) {
		t.Fatalf("Next after drained retained window error = %v, want ErrSubscriptionClosed", err)
	}
	if newStream := set.Stream("dispatch-3"); newStream == nil {
		t.Fatal("Stream for unrelated dispatch = nil, want new dispatch stream")
	}
}

func TestStreamSet_EvictsCompletedDispatchAfterRetentionWindow(t *testing.T) {
	start := time.Date(2026, 6, 30, 0, 0, 0, 0, time.UTC)
	clock := &fixedClock{now: start}
	set := responsestream.NewStreamSetWithFactoryAndRetention(
		func() *responsestream.SessionResponseStream {
			return responsestream.NewSessionResponseStreamWithClock(
				clock,
				responsestream.RetentionLimits{MaxAge: time.Minute},
			)
		},
		time.Minute,
		clock,
	)
	stream := set.Stream("dispatch-1")
	if stream == nil {
		t.Fatal("Stream = nil, want allocated dispatch stream")
	}
	stream.Append(responsestream.Event{
		Kind:    responsestream.EventKindProgressFragment,
		Type:    responsestream.EventTypeProgress,
		Payload: "planning",
	})
	if closed := set.CloseDispatch("dispatch-1"); !closed {
		t.Fatal("CloseDispatch returned false, want dispatch completed")
	}
	if got := set.Count(); got != 1 {
		t.Fatalf("stream count before eviction = %d, want 1 retained dispatch", got)
	}

	clock.now = start.Add(2 * time.Minute)
	if ids := set.DispatchIDs(); len(ids) != 0 {
		t.Fatalf("dispatch ids after retention window = %#v, want eviction", ids)
	}
	if got := set.Count(); got != 0 {
		t.Fatalf("stream count after eviction = %d, want 0", got)
	}
	if retained := set.Stream("dispatch-1"); retained != nil {
		t.Fatal("Stream after eviction = non-nil, want completed dispatch removed")
	}
}

func TestStreamSetWithFactoryAndRetention_RequiresSelectedClock(t *testing.T) {
	t.Parallel()

	set := responsestream.NewStreamSetWithFactoryAndRetention(
		newResponseStream,
		time.Minute,
		nil,
	)
	if set != nil {
		t.Fatalf("set = %#v, want nil when no clock was selected", set)
	}
}

func TestStreamSet_StreamEvictsExpiredCompletedDispatchOnDirectAccess(t *testing.T) {
	start := time.Date(2026, 6, 30, 0, 0, 0, 0, time.UTC)
	clock := &fixedClock{now: start}
	set := responsestream.NewStreamSetWithFactoryAndRetention(
		func() *responsestream.SessionResponseStream {
			return responsestream.NewSessionResponseStreamWithClock(
				clock,
				responsestream.RetentionLimits{MaxAge: time.Minute},
			)
		},
		time.Minute,
		clock,
	)
	stream := set.Stream("dispatch-1")
	if stream == nil {
		t.Fatal("Stream = nil, want allocated dispatch stream")
	}
	stream.Append(responsestream.Event{
		Kind:    responsestream.EventKindProgressFragment,
		Type:    responsestream.EventTypeProgress,
		Payload: "planning",
	})
	if closed := set.CloseDispatch("dispatch-1"); !closed {
		t.Fatal("CloseDispatch returned false, want dispatch completed")
	}

	clock.now = start.Add(2 * time.Minute)
	if retained := set.Stream("dispatch-1"); retained != nil {
		t.Fatal("Stream after retention window = non-nil, want completed dispatch evicted on direct access")
	}
}

func TestStreamSet_SubscribeRejectsExpiredCompletedDispatchOnDirectAccess(t *testing.T) {
	start := time.Date(2026, 6, 30, 0, 0, 0, 0, time.UTC)
	clock := &fixedClock{now: start}
	set := responsestream.NewStreamSetWithFactoryAndRetention(
		func() *responsestream.SessionResponseStream {
			return responsestream.NewSessionResponseStreamWithClock(
				clock,
				responsestream.RetentionLimits{MaxAge: time.Minute},
			)
		},
		time.Minute,
		clock,
	)
	stream := set.Stream("dispatch-1")
	if stream == nil {
		t.Fatal("Stream = nil, want allocated dispatch stream")
	}
	stream.Append(responsestream.Event{
		Kind:    responsestream.EventKindProgressFragment,
		Type:    responsestream.EventTypeProgress,
		Payload: "planning",
	})
	if closed := set.CloseDispatch("dispatch-1"); !closed {
		t.Fatal("CloseDispatch returned false, want dispatch completed")
	}

	clock.now = start.Add(2 * time.Minute)
	if _, err := set.Subscribe("dispatch-1", 0); !errors.Is(err, responsestream.ErrSubscriptionClosed) {
		t.Fatalf("Subscribe after retention window error = %v, want ErrSubscriptionClosed without prior DispatchIDs/Count", err)
	}
}

func TestStreamSet_ClosePreventsStreamRecreation(t *testing.T) {
	set := newStreamSet()
	stream := set.Stream("dispatch-1")
	if stream == nil {
		t.Fatal("Stream before close = nil, want allocated stream")
	}

	set.Close()

	if stream := set.Stream("dispatch-1"); stream != nil {
		t.Fatal("Stream after close = non-nil, want terminal closed set")
	}
	if _, err := set.Subscribe("dispatch-1", 0); !errors.Is(err, responsestream.ErrSubscriptionClosed) {
		t.Fatalf("Subscribe after close error = %v, want ErrSubscriptionClosed", err)
	}
	if got := set.Count(); got != 0 {
		t.Fatalf("stream count after closed subscribe = %d, want 0", got)
	}
}
