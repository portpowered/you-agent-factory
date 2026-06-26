package responsestream_test

import (
	"context"
	"errors"
	"sync/atomic"
	"testing"

	"github.com/portpowered/infinite-you/pkg/factorysessions/responsestream"
)

func TestStreamSet_ReusesDispatchStreamAndSeparatesOtherDispatches(t *testing.T) {
	set := responsestream.NewStreamSet()

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
	set := responsestream.NewStreamSetWithFactory(func() *responsestream.SessionResponseStream {
		calls.Add(1)
		return responsestream.NewSessionResponseStream()
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

func TestStreamSet_CloseDispatchDetachesSubscribersAndRemovesStream(t *testing.T) {
	set := responsestream.NewStreamSet()
	subscription, err := set.Subscribe("dispatch-1", 0)
	if err != nil {
		t.Fatalf("Subscribe: %v", err)
	}

	if got := set.SubscriberCount("dispatch-1"); got != 1 {
		t.Fatalf("subscriber count = %d, want 1", got)
	}
	if closed := set.CloseDispatch("dispatch-1"); !closed {
		t.Fatal("CloseDispatch returned false, want stream closed")
	}
	if got := set.SubscriberCount("dispatch-1"); got != 0 {
		t.Fatalf("subscriber count after close = %d, want 0", got)
	}
	if got := set.Count(); got != 0 {
		t.Fatalf("stream count after close = %d, want 0", got)
	}
	if _, err := subscription.Next(context.Background()); !errors.Is(err, responsestream.ErrSubscriptionClosed) {
		t.Fatalf("Next after dispatch close error = %v, want ErrSubscriptionClosed", err)
	}
}
