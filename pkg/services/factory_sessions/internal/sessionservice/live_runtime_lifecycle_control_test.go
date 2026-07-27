package service_test

import (
	"context"
	"errors"
	"testing"

	interfaces "github.com/portpowered/infinite-you/pkg/services/factory_definitions"
	factorysessions "github.com/portpowered/infinite-you/pkg/services/factory_sessions"
	"github.com/portpowered/infinite-you/pkg/services/factory_sessions/internal/livesession"
)

func TestService_LivePauseResumeThroughLiveRuntimeOwnerReturnsAcceptedOutcomeOnce(t *testing.T) {
	t.Parallel()

	factory := &gatewayLifecycleFactory{factoryState: string(interfaces.FactoryStateRunning)}
	session := &livesession.LiveSession{ID: "sess-pause-resume"}
	host := &liveRuntimeEffectHost{
		openTestHost: openTestHost{
			sessions: map[string]*livesession.LiveSession{session.ID: session},
		},
		factory: factory,
	}
	gateway := newLiveRuntimeCompositionGateway(t, host)
	ctx := context.Background()

	paused, err := gateway.PauseLiveFactorySession(ctx, session.ID, factorysessions.ControlRequest{})
	if err != nil || paused.Outcome != factorysessions.LifecycleControlOutcomeAccepted || paused.Status != factorysessions.LifecycleStatusPaused {
		t.Fatalf("pause = (%#v, %v), want accepted paused", paused, err)
	}
	if factory.pauseCalls != 1 {
		t.Fatalf("pause calls = %d, want 1", factory.pauseCalls)
	}

	factory.factoryState = string(interfaces.FactoryStatePaused)
	resumed, err := gateway.ResumeLiveFactorySession(ctx, session.ID, factorysessions.ControlRequest{})
	if err != nil || resumed.Outcome != factorysessions.LifecycleControlOutcomeAccepted || resumed.Status != factorysessions.LifecycleStatusRunning {
		t.Fatalf("resume = (%#v, %v), want accepted running", resumed, err)
	}
	if factory.resumeCalls != 1 {
		t.Fatalf("resume calls = %d, want 1", factory.resumeCalls)
	}
}

func TestService_LivePauseRejectsInvalidStateWithoutRegistryMutation(t *testing.T) {
	t.Parallel()

	factory := &gatewayLifecycleFactory{factoryState: "QUEUED"}
	session := &livesession.LiveSession{ID: "sess-invalid"}
	host := &liveRuntimeEffectHost{
		openTestHost: openTestHost{
			sessions: map[string]*livesession.LiveSession{session.ID: session},
		},
		factory: factory,
	}
	gateway := newLiveRuntimeCompositionGateway(t, host)
	ctx := context.Background()

	_, err := gateway.PauseLiveFactorySession(ctx, session.ID, factorysessions.ControlRequest{})
	var controlErr *factorysessions.ControlError
	if !errors.As(err, &controlErr) || controlErr.Outcome != factorysessions.LifecycleControlOutcomeInvalidState {
		t.Fatalf("pause error = %v, want invalid-state control error", err)
	}
	if factory.pauseCalls != 0 || host.stopCalls != 0 {
		t.Fatalf("invalid pause mutated registry: pause=%d stop=%d", factory.pauseCalls, host.stopCalls)
	}
	if gateway.ResolveFactorySession(session.ID) == nil {
		t.Fatal("registry entry removed after invalid pause")
	}
}

func TestService_CloseFactorySessionThroughLiveRuntimeRetiresRegistryEntry(t *testing.T) {
	t.Parallel()

	factory := &gatewayLifecycleFactory{factoryState: string(interfaces.FactoryStateRunning)}
	session := &livesession.LiveSession{
		ID: "sess-close-owner",
		Runtime: &factorysessions.LiveRuntime{
			Factory: factory,
		},
	}
	sessions := map[string]*livesession.LiveSession{session.ID: session}
	host := &retiringLiveRuntimeHost{
		liveRuntimeEffectHost: liveRuntimeEffectHost{
			openTestHost: openTestHost{
				sessions: sessions,
			},
			factory: factory,
		},
	}
	gateway := newLiveRuntimeCompositionGateway(t, host)
	ctx := context.Background()

	if err := gateway.CloseFactorySession(ctx, session.ID); err != nil {
		t.Fatalf("CloseFactorySession: %v", err)
	}
	if factory.terminateCalls != 1 {
		t.Fatalf("terminate calls = %d, want 1", factory.terminateCalls)
	}
	if host.stopCalls != 1 {
		t.Fatalf("stop calls = %d, want 1", host.stopCalls)
	}
	if gateway.ResolveFactorySession(session.ID) != nil {
		t.Fatal("closed session still resolves as active")
	}
}

type retiringLiveRuntimeHost struct {
	liveRuntimeEffectHost
}

func (h *retiringLiveRuntimeHost) RequireSession(sessionID string) (*livesession.LiveSession, error) {
	if session := h.sessions[sessionID]; session != nil {
		return session, nil
	}
	return nil, factorysessions.ErrNotFound
}

func (h *retiringLiveRuntimeHost) StopLiveSession(sessionID string) error {
	h.stopCalls++
	delete(h.sessions, sessionID)
	return nil
}

func TestService_CloseFactorySessionUnknownReturnsNotFoundWithoutStoppingOthers(t *testing.T) {
	t.Parallel()

	remaining := &livesession.LiveSession{ID: "sess-remaining"}
	host := &liveRuntimeEffectHost{
		openTestHost: openTestHost{
			sessions: map[string]*livesession.LiveSession{remaining.ID: remaining},
		},
	}
	gateway := newLiveRuntimeCompositionGateway(t, host)
	ctx := context.Background()

	err := gateway.CloseFactorySession(ctx, "missing")
	if err == nil || !errors.Is(err, factorysessions.ErrNotFound) {
		t.Fatalf("CloseFactorySession error = %v, want ErrNotFound", err)
	}
	if host.stopCalls != 0 {
		t.Fatalf("stop calls = %d, want none", host.stopCalls)
	}
	if gateway.ResolveFactorySession(remaining.ID) == nil {
		t.Fatal("unrelated session removed after close of unknown id")
	}
}

func TestService_LivePauseHonorsContextCancellation(t *testing.T) {
	t.Parallel()

	factory := &gatewayLifecycleFactory{factoryState: string(interfaces.FactoryStateRunning)}
	session := &livesession.LiveSession{ID: "sess-cancel-control"}
	host := &liveRuntimeEffectHost{
		openTestHost: openTestHost{
			sessions: map[string]*livesession.LiveSession{session.ID: session},
		},
		factory: factory,
	}
	gateway := newLiveRuntimeCompositionGateway(t, host)

	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	_, err := gateway.PauseLiveFactorySession(ctx, session.ID, factorysessions.ControlRequest{})
	if err == nil || !errors.Is(err, context.Canceled) {
		t.Fatalf("PauseLiveFactorySession error = %v, want context canceled", err)
	}
	if factory.pauseCalls != 0 {
		t.Fatalf("pause calls = %d after cancellation, want none", factory.pauseCalls)
	}
	if gateway.ResolveFactorySession(session.ID) == nil {
		t.Fatal("registry entry removed after cancelled pause")
	}
}

func TestService_CloseFactorySessionHonorsContextCancellation(t *testing.T) {
	t.Parallel()

	session := &livesession.LiveSession{ID: "sess-cancel-close"}
	host := &liveRuntimeEffectHost{
		openTestHost: openTestHost{
			sessions: map[string]*livesession.LiveSession{session.ID: session},
		},
	}
	gateway := newLiveRuntimeCompositionGateway(t, host)

	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	err := gateway.CloseFactorySession(ctx, session.ID)
	if err == nil || !errors.Is(err, context.Canceled) {
		t.Fatalf("CloseFactorySession error = %v, want context canceled", err)
	}
	if host.stopCalls != 0 {
		t.Fatalf("stop calls = %d after cancellation, want none", host.stopCalls)
	}
	if gateway.ResolveFactorySession(session.ID) == nil {
		t.Fatal("registry entry removed after cancelled close")
	}
}
