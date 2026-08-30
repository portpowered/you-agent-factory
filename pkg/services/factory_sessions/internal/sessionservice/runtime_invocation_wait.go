// Event-driven invocation waiting for the active Factory Session.
package service

import (
	"context"
	"time"

	interfaces "github.com/portpowered/infinite-you/pkg/services/factory_definitions"
	sessioninvocation "github.com/portpowered/infinite-you/pkg/services/factory_sessions/internal/invocation"
	"github.com/portpowered/infinite-you/pkg/services/factory_sessions/internal/runtimebinding"
)

// invocationWaiterFallbackInterval bounds one event-driven wait iteration. The
// canonical event subscription wakes the wait loop as soon as an
// outcome-relevant event lands; this heartbeat only covers subscription gaps
// (a dropped live stream, an event type the relevance filter missed) so the
// wait can never regress past the historical poll cadence.
const invocationWaiterFallbackInterval = 250 * time.Millisecond

// newSessionInvocationWaitOpener binds the invocation owner's wait loop to the
// session's canonical Factory event stream. Waking is a hint, never a
// decision: the owner still resolves outcomes exclusively through its
// canonical observation, so a spurious wake costs one extra observation and a
// missed wake falls back to the heartbeat interval.
func newSessionInvocationWaitOpener(
	fs *SessionRuntime,
) func(context.Context, string) (sessioninvocation.SessionInvocationWaiter, sessioninvocation.ReleaseSessionInvocationWaiter) {
	return func(ctx context.Context, sessionID string) (sessioninvocation.SessionInvocationWaiter, sessioninvocation.ReleaseSessionInvocationWaiter) {
		if fs == nil {
			return nil, nil
		}
		activeFactory, err := runtimebinding.FactoryForSession(fs.sessionState, sessionID)
		if err != nil {
			return nil, nil
		}
		ingress, ok := runtimebinding.WorkAndEventIngressForService(activeFactory)
		if !ok {
			return nil, nil
		}
		subscribeCtx, cancel := context.WithCancel(ctx)
		stream, err := ingress.SubscribeFactoryEvents(subscribeCtx, nil, interfaces.FactoryEventReconnectScope{
			SessionID:    sessionID,
			HistoryLimit: 1,
		})
		if err != nil || stream == nil {
			cancel()
			return nil, nil
		}
		wake := make(chan struct{}, 1)
		go relayInvocationWakeEvents(stream.Events, wake)
		return newEventDrivenInvocationWaiter(wake), cancel
	}
}

// relayInvocationWakeEvents coalesces outcome-relevant canonical events into a
// level-triggered wake signal. It ends when the live stream closes, which the
// ledger ties to subscription-context cancellation and live-stream shutdown.
func relayInvocationWakeEvents(events <-chan interfaces.FactoryEvent, wake chan<- struct{}) {
	for event := range events {
		if !invocationOutcomeRelevantEvent(event.Type) {
			continue
		}
		select {
		case wake <- struct{}{}:
		default:
		}
	}
}

func newEventDrivenInvocationWaiter(wake <-chan struct{}) sessioninvocation.SessionInvocationWaiter {
	return func(ctx context.Context) error {
		timer := time.NewTimer(invocationWaiterFallbackInterval)
		defer timer.Stop()
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-wake:
			return nil
		case <-timer.C:
			return nil
		}
	}
}

// invocationOutcomeRelevantEvent reports whether one canonical event type can
// change an invocation wait outcome: Work state movement, session lifecycle
// and result changes, and run completion. High-frequency execution telemetry
// (model, inference, script, and dispatch chatter) is deliberately excluded so
// waking does not rebuild the event-derived world state per telemetry event;
// the heartbeat interval covers any outcome path this filter misses.
func invocationOutcomeRelevantEvent(eventType interfaces.FactoryEventType) bool {
	switch eventType {
	case interfaces.FactoryEventTypeWorkStateChange,
		interfaces.FactoryEventTypeWorkRequest,
		interfaces.FactoryEventTypeRunResponse,
		interfaces.FactoryEventTypeFactoryStateResponse,
		interfaces.FactoryEventTypeSessionCompleted,
		interfaces.FactoryEventTypeSessionPaused,
		interfaces.FactoryEventTypeSessionResumed,
		interfaces.FactoryEventTypeSessionResultUpdated,
		interfaces.FactoryEventTypeSessionLifecycleControl:
		return true
	default:
		return false
	}
}
