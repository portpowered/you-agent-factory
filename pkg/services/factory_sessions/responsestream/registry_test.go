package responsestream_test

import (
	"testing"

	"github.com/portpowered/infinite-you/pkg/services/factory_sessions/responsestream"
)

func TestRegistryOwnsAndReleasesSessionStreamSets(t *testing.T) {
	created := 0
	registry := responsestream.NewRegistry(func() *responsestream.SessionResponseStream {
		created++
		return newResponseStream()
	}, &fixedClock{})

	first := registry.Streams("session-a")
	if first != registry.Streams("session-a") {
		t.Fatal("Streams returned different sets for one session")
	}
	first.Stream("dispatch-a")
	if created != 1 {
		t.Fatalf("created streams = %d, want 1", created)
	}
	registry.Close("session-a")
	if registry.Existing("session-a") != first {
		t.Fatal("closed session did not retain its closed stream set")
	}
	if _, err := registry.Streams("session-a").Subscribe("dispatch-a", 0); err != responsestream.ErrSubscriptionClosed {
		t.Fatalf("Subscribe after close = %v, want %v", err, responsestream.ErrSubscriptionClosed)
	}
}
