package service

import (
	"context"
	"testing"
	"time"

	interfaces "github.com/portpowered/infinite-you/pkg/services/factory_definitions"
)

func TestInvocationOutcomeRelevantEventSelectsStateAndLifecycleTypes(t *testing.T) {
	relevant := []interfaces.FactoryEventType{
		interfaces.FactoryEventTypeWorkStateChange,
		interfaces.FactoryEventTypeWorkRequest,
		interfaces.FactoryEventTypeRunResponse,
		interfaces.FactoryEventTypeFactoryStateResponse,
		interfaces.FactoryEventTypeSessionCompleted,
		interfaces.FactoryEventTypeSessionPaused,
		interfaces.FactoryEventTypeSessionResumed,
		interfaces.FactoryEventTypeSessionResultUpdated,
		interfaces.FactoryEventTypeSessionLifecycleControl,
	}
	for _, eventType := range relevant {
		if !invocationOutcomeRelevantEvent(eventType) {
			t.Errorf("invocationOutcomeRelevantEvent(%q) = false, want true", eventType)
		}
	}
	telemetry := []interfaces.FactoryEventType{
		interfaces.FactoryEventTypeModelRequest,
		interfaces.FactoryEventTypeModelResponse,
		interfaces.FactoryEventTypeInferenceRequest,
		interfaces.FactoryEventTypeInferenceResponse,
		interfaces.FactoryEventTypeScriptRequest,
		interfaces.FactoryEventTypeScriptResponse,
		interfaces.FactoryEventTypeDispatchRequest,
		interfaces.FactoryEventTypeDispatchQueued,
		interfaces.FactoryEventTypeAgentRunResponse,
	}
	for _, eventType := range telemetry {
		if invocationOutcomeRelevantEvent(eventType) {
			t.Errorf("invocationOutcomeRelevantEvent(%q) = true, want false", eventType)
		}
	}
}

func TestEventDrivenInvocationWaiterWakesOnRelevantEventBeforeFallback(t *testing.T) {
	events := make(chan interfaces.FactoryEvent, 2)
	wake := make(chan struct{}, 1)
	go relayInvocationWakeEvents(events, wake)

	events <- interfaces.FactoryEvent{Type: interfaces.FactoryEventTypeModelResponse}
	events <- interfaces.FactoryEvent{Type: interfaces.FactoryEventTypeWorkStateChange}
	close(events)

	waiter := newEventDrivenInvocationWaiter(wake)
	start := time.Now()
	if err := waiter(context.Background()); err != nil {
		t.Fatalf("waiter: %v", err)
	}
	if elapsed := time.Since(start); elapsed >= invocationWaiterFallbackInterval {
		t.Fatalf("waiter took %v, want an event-driven wake before the %v fallback", elapsed, invocationWaiterFallbackInterval)
	}
}

func TestEventDrivenInvocationWaiterHonorsContextCancellation(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	waiter := newEventDrivenInvocationWaiter(make(chan struct{}))
	if err := waiter(ctx); err != context.Canceled {
		t.Fatalf("waiter err = %v, want context.Canceled", err)
	}
}

func TestRelayInvocationWakeEventsCoalescesBurstsWithoutBlocking(t *testing.T) {
	events := make(chan interfaces.FactoryEvent)
	wake := make(chan struct{}, 1)
	relayDone := make(chan struct{})
	go func() {
		relayInvocationWakeEvents(events, wake)
		close(relayDone)
	}()

	for range 8 {
		events <- interfaces.FactoryEvent{Type: interfaces.FactoryEventTypeWorkStateChange}
	}
	close(events)
	<-relayDone

	select {
	case <-wake:
	default:
		t.Fatal("coalesced wake signal is missing after an event burst")
	}
	select {
	case <-wake:
		t.Fatal("wake signal is not coalesced: second token present")
	default:
	}
}
