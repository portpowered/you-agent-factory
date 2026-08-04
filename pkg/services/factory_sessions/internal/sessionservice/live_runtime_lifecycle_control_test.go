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

func TestLiveControlCapability_OpenPauseResumePreservesLifecycleResults(t *testing.T) {
	t.Parallel()

	const sessionID = "sess-live-control-pause-resume"
	target := factorysessions.Target{
		Ref:        factorysessions.TargetRef{Kind: factorysessions.TargetKindDefault},
		FactoryDir: "/tmp/factory",
		FolderPath: "/tmp",
		Project:    "demo",
	}
	factory := &gatewayLifecycleFactory{factoryState: string(interfaces.FactoryStateRunning)}
	session := &livesession.LiveSession{
		ID: sessionID,
		SessionState: livesession.SessionState{
			FactoryDir: target.FactoryDir,
			FolderPath: target.FolderPath,
		},
		Project: target.Project,
		Target:  target.Ref,
	}
	host := &liveRuntimeEffectHost{
		openTestHost: openTestHost{
			targets:        []factorysessions.Target{target},
			openSessionID:  sessionID,
			requireSession: session,
			sessionIDs:     []string{sessionID},
			sessions:       map[string]*livesession.LiveSession{sessionID: session},
		},
		factory: factory,
	}

	// The client holds only the owner-published live-control capability.
	var client factorysessions.LiveControlService = newLiveRuntimeCompositionGateway(t, host)
	opened, err := client.OpenFactorySession(context.Background(), factorysessions.LiveControlOpenRequest{
		FolderPath: target.FolderPath,
	})
	if err != nil {
		t.Fatalf("OpenFactorySession: %v", err)
	}
	if opened == nil || opened.SessionID != sessionID {
		t.Fatalf("open result = %#v, want session id %q", opened, sessionID)
	}

	paused, err := client.PauseLiveFactorySession(
		context.Background(),
		opened.SessionID,
		factorysessions.LiveControlRequest{Reason: "operator-pause"},
	)
	if err != nil || paused.SessionID != opened.SessionID ||
		paused.Operation != factorysessions.LifecycleControlPause ||
		paused.Outcome != factorysessions.LifecycleControlOutcomeAccepted ||
		paused.Status != factorysessions.LifecycleStatusPaused {
		t.Fatalf("PauseLiveFactorySession = (%#v, %v), want accepted paused result", paused, err)
	}
	if factory.pauseCalls != 1 {
		t.Fatalf("pause calls = %d, want 1", factory.pauseCalls)
	}

	// The gateway test double models the runtime's state observation boundary.
	// Production pause transitions this state inside the Factory Runtime.
	factory.factoryState = string(interfaces.FactoryStatePaused)
	resumed, err := client.ResumeLiveFactorySession(
		context.Background(),
		opened.SessionID,
		factorysessions.LiveControlRequest{Reason: "operator-resume"},
	)
	if err != nil || resumed.SessionID != opened.SessionID ||
		resumed.Operation != factorysessions.LifecycleControlResume ||
		resumed.Outcome != factorysessions.LifecycleControlOutcomeAccepted ||
		resumed.Status != factorysessions.LifecycleStatusRunning {
		t.Fatalf("ResumeLiveFactorySession = (%#v, %v), want accepted running result", resumed, err)
	}
	if factory.resumeCalls != 1 {
		t.Fatalf("resume calls = %d, want 1", factory.resumeCalls)
	}
}

func TestLiveControlCapability_PreservesTypedRejectionAndCancellation(t *testing.T) {
	t.Parallel()

	const sessionID = "sess-live-control-rejection"
	factory := &gatewayLifecycleFactory{factoryState: string(interfaces.FactoryStateCompleted)}
	session := &livesession.LiveSession{ID: sessionID}
	host := &liveRuntimeEffectHost{
		openTestHost: openTestHost{
			requireSession: session,
			sessionIDs:     []string{sessionID},
			sessions:       map[string]*livesession.LiveSession{sessionID: session},
		},
		factory: factory,
	}

	var client factorysessions.LiveControlService = newLiveRuntimeCompositionGateway(t, host)
	_, err := client.ResumeLiveFactorySession(context.Background(), sessionID, factorysessions.LiveControlRequest{})
	var controlErr *factorysessions.LiveControlError
	if !errors.As(err, &controlErr) {
		t.Fatalf("ResumeLiveFactorySession terminal error = %v, want *LiveControlError", err)
	}
	if controlErr.Outcome != factorysessions.LifecycleControlOutcomeTerminalSession {
		t.Fatalf("terminal resume outcome = %q, want TERMINAL_SESSION", controlErr.Outcome)
	}
	if errors.Is(err, factorysessions.ErrSessionNotFound) {
		t.Fatal("terminal lifecycle rejection must remain distinct from ErrSessionNotFound")
	}
	if factory.resumeCalls != 0 {
		t.Fatalf("resume calls = %d after terminal rejection, want none", factory.resumeCalls)
	}

	factory.factoryState = string(interfaces.FactoryStateRunning)
	canceledCtx, cancel := context.WithCancel(context.Background())
	cancel()
	_, err = client.PauseLiveFactorySession(canceledCtx, sessionID, factorysessions.LiveControlRequest{})
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("PauseLiveFactorySession canceled error = %v, want context canceled", err)
	}
	if factory.pauseCalls != 0 {
		t.Fatalf("pause calls = %d after cancellation, want none", factory.pauseCalls)
	}
	if factory.factoryState != string(interfaces.FactoryStateRunning) {
		t.Fatalf("factory state = %q after canceled pause, want RUNNING", factory.factoryState)
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
