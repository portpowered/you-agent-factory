// Invocation dependencies are projected from the active Factory Session.
package service

import (
	"context"
	"fmt"
	"time"

	platformclock "github.com/portpowered/infinite-you/pkg/platform/clock"
	interfaces "github.com/portpowered/infinite-you/pkg/services/factory_definitions"
	factoryruntime "github.com/portpowered/infinite-you/pkg/services/factory_runtime"
	factorysessions "github.com/portpowered/infinite-you/pkg/services/factory_sessions"
	"github.com/portpowered/infinite-you/pkg/services/factory_sessions/internal/fileeffects"
	sessioninvocation "github.com/portpowered/infinite-you/pkg/services/factory_sessions/internal/invocation"
	"github.com/portpowered/infinite-you/pkg/services/factory_sessions/internal/invocation/packagedtts"
	invocationruntime "github.com/portpowered/infinite-you/pkg/services/factory_sessions/internal/invocation/runtimeadapter"
	sessionruntime "github.com/portpowered/infinite-you/pkg/services/factory_sessions/internal/runtime"
	"github.com/portpowered/infinite-you/pkg/services/factory_sessions/internal/runtimebinding"
	invocationservice "github.com/portpowered/infinite-you/pkg/services/factory_sessions/internal/services/invocation"
	invocationwire "github.com/portpowered/infinite-you/pkg/services/factory_sessions/internal/services/invocation/wire"
	"github.com/portpowered/infinite-you/pkg/services/work"
)

// NewInvocationOwner constructs the canonical invocation owner from the
// injected query authority and remaining session-owned effects.
func NewInvocationOwner(
	fs *SessionRuntime,
	authority InvocationAuthority,
	interpolation interfaces.InvocationInterpolationService,
	invocationWorkTypes interfaces.InvocationWorkTypeService,
	ttsObservability interfaces.TTSObservabilityService,
	inputFiles fileeffects.InvocationInputReader,
) (invocationservice.Service, error) {
	if fs == nil {
		return nil, fmt.Errorf("session runtime is required")
	}
	return invocationwire.New(invocationservice.Dependencies{
		FactoryConfig: authority.FactoryConfig,
		SubmitWork:    authority.SubmitWork,
		Observe:       authority.Observe,
		WaitSession:   authority.WaitSession,
		Telemetry: packagedtts.NewTelemetry(
			ttsObservability,
			func(metric sessioninvocation.SessionInvocationMetric) {
				fs.recordInvocationMetric(metric.Name, metric.Labels)
			},
			func(record sessioninvocation.SessionInvocationLogRecord) {
				invocationruntime.WriteLogRecord(fs.logger, record)
			},
		),
		SpecialCase:   packagedtts.NewSpecialCase(ttsObservability),
		Interpolation: interpolation,
		WorkTypes:     invocationWorkTypes,
		InputFiles:    inputFiles,
		Work:          work.NewInvocationPolicyService(),
		CancelOnTimeout: func(ctx context.Context, sessionID string, request factorysessions.ControlRequest) (factorysessions.LifecycleControlResult, error) {
			if fs == nil {
				return factorysessions.LifecycleControlResult{}, fmt.Errorf("Factory Session runtime is required")
			}
			return fs.requireSessionGateway().CancelLiveFactorySession(ctx, sessionID, request)
		},
	})
}

func (fs *SessionRuntime) recordInvocationMetric(name string, labels map[string]string) {
	if fs == nil || fs.invocationMetricsRecorder == nil {
		return
	}
	fs.invocationMetricsRecorder.RecordInvocationMetric(factorysessions.InvocationMetric{Name: name, Labels: labels})
}

// invocationWaiterFallbackInterval bounds one event-driven wait iteration. The
// canonical event subscription wakes the wait loop as soon as an
// outcome-relevant event lands; this heartbeat only covers subscription gaps
// (a dropped live stream, an event type the relevance filter missed) so the
// wait can never regress past the historical poll cadence.
const invocationWaiterFallbackInterval = 250 * time.Millisecond

// InvocationAuthority reads and admits Work against the addressed live generation.
// It has no dependency on the opening owner, gateway, or invocation engine.
type InvocationAuthority interface {
	FactoryConfig(string) (*interfaces.FactoryConfig, error)
	SubmitWork(context.Context, string, work.SubmitRequest) (work.WorkRequestSubmitResult, error)
	Observe(context.Context, string, sessioninvocation.SessionInvocationWaitInput) (sessioninvocation.SessionInvocationObservation, error)
	WaitSession(context.Context, string) (sessioninvocation.SessionInvocationWaiter, sessioninvocation.ReleaseSessionInvocationWaiter)
}

type invocationAuthority struct {
	state     *sessionruntime.Service
	scheduler platformclock.TimerSource
	projector factoryruntime.WorldStateProjector
}

// NewInvocationAuthority constructs query behavior over the canonical session state.
func NewInvocationAuthority(state *sessionruntime.Service, scheduler platformclock.TimerSource, projector factoryruntime.WorldStateProjector) InvocationAuthority {
	return &invocationAuthority{state: state, scheduler: scheduler, projector: projector}
}

func (a *invocationAuthority) FactoryConfig(sessionID string) (*interfaces.FactoryConfig, error) {
	config, err := runtimebinding.RuntimeConfigForSession(a.state, sessionID)
	if err != nil {
		return nil, err
	}
	return config.FactoryConfig(), nil
}

func (a *invocationAuthority) SubmitWork(ctx context.Context, sessionID string, request work.SubmitRequest) (work.WorkRequestSubmitResult, error) {
	session, err := runtimebinding.RequireLiveSession(a.state, sessionID)
	if err != nil {
		return work.WorkRequestSubmitResult{}, err
	}
	ingress, ok := runtimebinding.WorkAndEventIngressForLiveRuntime(session.Runtime)
	if !ok {
		return work.WorkRequestSubmitResult{}, fmt.Errorf("Factory Runtime work submission is required")
	}
	return ingress.SubmitWorkRequest(ctx, work.WorkRequestFromSubmitRequests([]work.SubmitRequest{request}))
}

func (a *invocationAuthority) Observe(ctx context.Context, sessionID string, input sessioninvocation.SessionInvocationWaitInput) (sessioninvocation.SessionInvocationObservation, error) {
	return invocationruntime.Observe(ctx, a.state, sessionID, input, a.projector)
}

// WaitSession subscribes to canonical events from the generation addressed when
// the wait opens. Wakes are hints; Observe selects the current generation again.
func (a *invocationAuthority) WaitSession(ctx context.Context, sessionID string) (sessioninvocation.SessionInvocationWaiter, sessioninvocation.ReleaseSessionInvocationWaiter) {
	activeFactory, err := runtimebinding.FactoryForSession(a.state, sessionID)
	if err != nil {
		return nil, nil
	}
	ingress, ok := runtimebinding.WorkAndEventIngressForService(activeFactory)
	if !ok {
		return nil, nil
	}
	subscribeCtx, cancel := context.WithCancel(ctx)
	stream, err := ingress.SubscribeFactoryEvents(subscribeCtx, nil, interfaces.FactoryEventReconnectScope{SessionID: sessionID, HistoryLimit: 1})
	if err != nil || stream == nil {
		cancel()
		return nil, nil
	}
	wake := make(chan struct{}, 1)
	go relayInvocationWakeEvents(stream.Events, wake)
	return newEventDrivenInvocationWaiter(wake, a.scheduler), cancel
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

func newEventDrivenInvocationWaiter(wake <-chan struct{}, scheduler platformclock.TimerSource) sessioninvocation.SessionInvocationWaiter {
	return func(ctx context.Context) error {
		timer := scheduler.NewTimer(invocationWaiterFallbackInterval)
		defer timer.Stop()
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-wake:
			return nil
		case <-timer.C():
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
