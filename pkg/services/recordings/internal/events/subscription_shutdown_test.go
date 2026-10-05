package events

import (
	"context"
	"testing"

	interfaces "github.com/portpowered/infinite-you/pkg/services/factory_definitions"
)

func TestSubscriptionOverflowRejectsNewOffersWithoutReservingCapacity(t *testing.T) {
	subscription := &eventHistorySubscription{overflow: make(chan struct{}), limit: 1}
	subscription.signalOverflow()
	if !subscription.offer(interfaces.FactoryEvent{}) || subscription.pending != 0 {
		t.Fatal("overflowed subscription must discard offers without reserving capacity")
	}
}

func TestSubscriptionShutdownReleasesUndeliveredEvent(t *testing.T) {
	for _, mode := range []string{"drain-cancel", "drain-overflow", "relay-overflow"} {
		t.Run(mode, func(t *testing.T) {
			ctx, cancel := context.WithCancel(t.Context())
			t.Cleanup(cancel)
			subscription := &eventHistorySubscription{
				events: make(chan interfaces.FactoryEvent), inbox: make(chan interfaces.FactoryEvent),
				done: ctx.Done(), overflow: make(chan struct{}), terminal: make(chan struct{}),
				drained: make(chan struct{}), pending: 1, limit: 1,
			}
			finished := make(chan bool, 1)
			if mode == "relay-overflow" {
				history := &FactoryEventHistory{}
				go func() { history.relayLiveSubscription(1, subscription); finished <- false }()
			} else {
				go func() { finished <- subscription.drainTerminalEvents() }()
			}
			// An unbuffered handoff proves the receiver selected the inbox before
			// shutdown. The output has no receiver, so only shutdown can release it.
			select {
			case subscription.inbox <- interfaces.FactoryEvent{}:
			case <-t.Context().Done():
				t.Fatal("subscription did not receive the event")
			}
			if mode == "drain-cancel" {
				cancel()
			} else {
				subscription.signalOverflow()
			}
			select {
			case drained := <-finished:
				if drained || subscription.pending != 0 {
					t.Fatalf("shutdown = %v, pending = %d", drained, subscription.pending)
				}
			case <-t.Context().Done():
				t.Fatal("subscription did not shut down")
			}
		})
	}
}
