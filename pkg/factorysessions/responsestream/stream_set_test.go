package responsestream_test

import (
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
