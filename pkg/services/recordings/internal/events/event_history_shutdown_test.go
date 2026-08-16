package events

import (
	"context"
	"testing"
	"time"

	interfaces "github.com/portpowered/infinite-you/pkg/services/factory_definitions"
)

func TestFactoryEventHistory_NilSubscribeReturnsClosedStream(t *testing.T) {
	stream, err := (*FactoryEventHistory)(nil).Subscribe(context.Background(), nil, interfaces.FactoryEventReconnectScope{})
	if err != nil || stream.Events == nil {
		t.Fatalf("Subscribe() = %#v, %v; want a closed event stream", stream, err)
	}
	if _, ok := <-stream.Events; ok {
		t.Fatal("nil history returned an open event stream")
	}
}

func TestFactoryEventHistory_CloseLiveSubscriptionsDeliversQueuedTerminalEvent(t *testing.T) {
	history := newTestFactoryEventHistory(nil, func() time.Time { return time.Unix(0, 0).UTC() })
	stream, err := history.Subscribe(context.Background(), nil, interfaces.FactoryEventReconnectScope{})
	if err != nil {
		t.Fatalf("Subscribe: %v", err)
	}

	history.RecordRunResponse(1, interfaces.FactoryStateCompleted, "", time.Unix(1, 0).UTC())
	canonical := history.CanonicalEvents()
	if len(canonical) != 1 || canonical[0].Type != interfaces.FactoryEventTypeRunResponse {
		t.Fatalf("canonical events = %#v, want one RUN_RESPONSE", canonical)
	}

	history.mu.RLock()
	subscription := history.streams[0]
	history.mu.RUnlock()
	if subscription == nil {
		t.Fatal("live subscription was not registered")
	}

	closeDone := make(chan struct{})
	go func() {
		history.CloseLiveSubscriptions()
		close(closeDone)
	}()
	<-subscription.terminal

	event, ok := <-stream.Events
	if !ok {
		t.Fatal("live stream closed before queued RUN_RESPONSE")
	}
	if event.Type != interfaces.FactoryEventTypeRunResponse {
		t.Fatalf("live event type = %s, want %s", event.Type, interfaces.FactoryEventTypeRunResponse)
	}
	if _, ok := <-stream.Events; ok {
		t.Fatal("live stream delivered an event after RUN_RESPONSE")
	}
	<-closeDone

	late, err := history.Subscribe(context.Background(), nil, interfaces.FactoryEventReconnectScope{})
	if err != nil {
		t.Fatalf("late Subscribe: %v", err)
	}
	if _, ok := <-late.Events; ok {
		t.Fatal("late live subscription remained open after terminal close")
	}
}

func TestFactoryEventHistory_CloseLiveSubscriptionsBoundsUnreadSubscriber(t *testing.T) {
	history := newTestFactoryEventHistory(nil, func() time.Time { return time.Unix(0, 0).UTC() })
	_, err := history.Subscribe(context.Background(), nil, interfaces.FactoryEventReconnectScope{})
	if err != nil {
		t.Fatalf("Subscribe: %v", err)
	}
	history.mu.RLock()
	subscription := history.streams[0]
	history.mu.RUnlock()
	if subscription == nil {
		t.Fatal("live subscription was not registered")
	}

	history.RecordRunResponse(1, interfaces.FactoryStateCompleted, "", time.Unix(1, 0).UTC())
	closeDone := make(chan struct{})
	go func() {
		history.CloseLiveSubscriptions()
		close(closeDone)
	}()

	deadline := time.NewTimer(eventHistoryCloseDrainTimeout + 250*time.Millisecond)
	defer deadline.Stop()
	select {
	case <-closeDone:
	case <-subscription.overflow:
		select {
		case <-closeDone:
		case <-deadline.C:
			t.Fatal("CloseLiveSubscriptions did not return after releasing the unread subscriber")
		}
	case <-deadline.C:
		t.Fatal("CloseLiveSubscriptions blocked on a subscriber that stopped reading")
	}
}
