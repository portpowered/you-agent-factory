package service_test

import (
	"context"
	"testing"

	interfaces "github.com/portpowered/infinite-you/pkg/services/factory_definitions"
	factoryruntime "github.com/portpowered/infinite-you/pkg/services/factory_runtime"
	factorysessions "github.com/portpowered/infinite-you/pkg/services/factory_sessions"
	"github.com/portpowered/infinite-you/pkg/services/factory_sessions/internal/livesession"
	"github.com/portpowered/infinite-you/pkg/services/factory_sessions/internal/stream"
	responsestreamwire "github.com/portpowered/infinite-you/pkg/services/factory_sessions/internal/services/response_stream/wire"
	factorysessionservice "github.com/portpowered/infinite-you/pkg/services/factory_sessions/internal/sessionservice"
)

type liveRuntimeEffectHost struct {
	openTestHost
	openCalls  int
	listCalls  int
	getCalls   int
	stopCalls  int
	factory    *gatewayLifecycleFactory
}

func (h *liveRuntimeEffectHost) OpenLiveSessionForTarget(ctx context.Context, target factorysessions.Target) (string, error) {
	h.openCalls++
	return h.openTestHost.OpenLiveSessionForTarget(ctx, target)
}

func (h *liveRuntimeEffectHost) ListLiveSessionIDs() []string {
	h.listCalls++
	return h.openTestHost.ListLiveSessionIDs()
}

func (h *liveRuntimeEffectHost) GetLiveSession(sessionID string) *livesession.LiveSession {
	h.getCalls++
	return h.openTestHost.GetLiveSession(sessionID)
}

func (h *liveRuntimeEffectHost) StopLiveSession(sessionID string) error {
	h.stopCalls++
	return h.openTestHost.StopLiveSession(sessionID)
}

func (h *liveRuntimeEffectHost) RequireSession(sessionID string) (*livesession.LiveSession, error) {
	if h.requireSessionE != nil {
		return nil, h.requireSessionE
	}
	if len(h.sessions) > 0 {
		if session := h.sessions[sessionID]; session != nil {
			return session, nil
		}
		return nil, factorysessions.ErrNotFound
	}
	return h.openTestHost.RequireSession(sessionID)
}

func (h *liveRuntimeEffectHost) SessionFactory(_ string) (factoryruntime.Service, error) {
	if h.factory == nil {
		h.factory = &gatewayLifecycleFactory{factoryState: string(interfaces.FactoryStateRunning)}
	}
	return h.factory, nil
}

type liveRuntimeGatewayHost interface {
	factorysessionservice.Host
	stream.SessionResolver
	stream.Observer
}

func newLiveRuntimeCompositionGateway(t *testing.T, host liveRuntimeGatewayHost) *factorysessionservice.Service {
	t.Helper()
	responseService, err := responsestreamwire.NewService(func() string { return "response-event-live-runtime" })
	if err != nil {
		t.Fatalf("construct response-stream service: %v", err)
	}
	registry, err := responseService.NewStreamRegistry(serviceTestClock)
	if err != nil {
		t.Fatalf("construct response-stream registry: %v", err)
	}
	return factorysessionservice.NewWithResponseService(host, host, host, registry, nil, nil, responseService)
}

func TestNewWithResponseService_ComposesLiveRuntimeWithoutRegistryEffects(t *testing.T) {
	t.Parallel()

	host := &liveRuntimeEffectHost{
		openTestHost: openTestHost{
			targets: []factorysessions.Target{{
				Ref:        factorysessions.TargetRef{Kind: factorysessions.TargetKindDefault},
				FactoryDir: "/tmp/factory",
			}},
		},
	}
	gateway := newLiveRuntimeCompositionGateway(t, host)
	if gateway == nil {
		t.Fatal("NewWithResponseService returned nil gateway")
	}
	if host.openCalls != 0 || host.listCalls != 0 || host.getCalls != 0 || host.stopCalls != 0 {
		t.Fatalf(
			"construction invoked live-runtime effects: open=%d list=%d get=%d stop=%d",
			host.openCalls, host.listCalls, host.getCalls, host.stopCalls,
		)
	}
}

func TestService_LiveRegistryPathsDelegateThroughLiveRuntimeOwner(t *testing.T) {
	t.Parallel()

	session := &livesession.LiveSession{
		ID: "sess-live-runtime",
		SessionState: livesession.SessionState{
			FactoryDir: "/tmp/factory",
			FolderPath: "/tmp",
		},
		Target: factorysessions.TargetRef{Kind: factorysessions.TargetKindDefault},
	}
	host := &liveRuntimeEffectHost{
		openTestHost: openTestHost{
			targets: []factorysessions.Target{{
				Ref:        factorysessions.TargetRef{Kind: factorysessions.TargetKindDefault},
				FactoryDir: "/tmp/factory",
				FolderPath: "/tmp",
			}},
			openSessionID:  session.ID,
			requireSession: session,
			sessionIDs:     []string{session.ID},
			sessions:       map[string]*livesession.LiveSession{session.ID: session},
		},
	}
	gateway := newLiveRuntimeCompositionGateway(t, host)
	ctx := context.Background()

	if _, err := gateway.ListFactorySessions(ctx); err != nil {
		t.Fatalf("ListFactorySessions: %v", err)
	}
	if host.listCalls != 1 {
		t.Fatalf("list calls = %d, want 1", host.listCalls)
	}

	if _, err := gateway.GetFactorySession(ctx, session.ID); err != nil {
		t.Fatalf("GetFactorySession: %v", err)
	}
	if host.getCalls < 1 {
		t.Fatalf("get calls = %d, want at least 1", host.getCalls)
	}

	if resolved := gateway.ResolveFactorySession(session.ID); resolved == nil || resolved.ID != session.ID {
		t.Fatalf("ResolveFactorySession = %#v, want sess-live-runtime", resolved)
	}
	if host.getCalls < 2 {
		t.Fatalf("resolve get calls = %d, want at least 2", host.getCalls)
	}

	if _, err := gateway.ObserveForSession(ctx, session.ID, factoryruntime.ObserveRequest{Scope: factoryruntime.ObservationScopeFull}); err != nil {
		t.Fatalf("ObserveForSession: %v", err)
	}

	if _, err := gateway.PauseLiveFactorySession(ctx, session.ID, factorysessions.ControlRequest{}); err != nil {
		t.Fatalf("PauseLiveFactorySession: %v", err)
	}

	if err := gateway.CloseFactorySession(ctx, session.ID); err != nil {
		t.Fatalf("CloseFactorySession: %v", err)
	}
	if host.stopCalls != 1 {
		t.Fatalf("stop calls = %d, want 1", host.stopCalls)
	}

	result, err := gateway.OpenFactorySessionFromFolder(ctx, "/tmp", nil, false, false)
	if err != nil {
		t.Fatalf("OpenFactorySessionFromFolder: %v", err)
	}
	if result == nil || result.SessionID != session.ID {
		t.Fatalf("open result = %#v, want %s", result, session.ID)
	}
	if host.openCalls != 1 {
		t.Fatalf("open calls = %d, want 1", host.openCalls)
	}
}

func TestService_ObserveForSessionReturnsNeutralObservationWithoutLegacySnapshot(t *testing.T) {
	t.Parallel()

	want := factoryruntime.Observation{
		Status: factoryruntime.ObservationStatusActive,
		Progress: factoryruntime.ObservationProgress{
			TickCount:             9,
			InFlightDispatchCount: 1,
			TotalWorkCount:        3,
			WorkCategories: factoryruntime.ObservationWorkCategories{
				Processing: 1,
				Terminal:   2,
			},
		},
		Health: factoryruntime.ObservationHealth{
			FactoryState:           string(interfaces.FactoryStateRunning),
			LifecycleControlStatus: "RUNNING",
		},
	}
	factory := &gatewayLifecycleFactory{
		factoryState:     string(interfaces.FactoryStateRunning),
		useObserveResult: true,
		observeResult:    want,
	}
	session := &livesession.LiveSession{
		ID: "sess-observe",
		Runtime: &factorysessions.LiveRuntime{
			Factory: factory,
		},
	}
	host := &liveRuntimeEffectHost{
		openTestHost: openTestHost{
			sessions:       map[string]*livesession.LiveSession{session.ID: session},
			requireSession: session,
		},
		factory: factory,
	}
	gateway := newLiveRuntimeCompositionGateway(t, host)

	result, err := gateway.ObserveForSession(
		context.Background(),
		session.ID,
		factoryruntime.ObserveRequest{Scope: factoryruntime.ObservationScopeFull},
	)
	if err != nil {
		t.Fatalf("ObserveForSession: %v", err)
	}
	if result.Observation.Status != want.Status ||
		result.Observation.Progress.TickCount != want.Progress.TickCount ||
		result.Observation.Health.FactoryState != want.Health.FactoryState {
		t.Fatalf("observation = %#v, want %#v", result.Observation, want)
	}
	if _, ok := any(factory).(factoryruntime.LegacySnapshotProvider); ok {
		t.Fatal("peer-shaped session runtime must not implement LegacySnapshotProvider")
	}
}
