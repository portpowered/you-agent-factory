package service_test

import (
	"context"
	"encoding/json"
	"testing"

	interfaces "github.com/portpowered/infinite-you/pkg/services/factory_definitions"
	factoryruntime "github.com/portpowered/infinite-you/pkg/services/factory_runtime"
	factorysessions "github.com/portpowered/infinite-you/pkg/services/factory_sessions"
	"github.com/portpowered/infinite-you/pkg/services/factory_sessions/internal/livechange"
	"github.com/portpowered/infinite-you/pkg/services/factory_sessions/internal/livesession"
	responsestreamwire "github.com/portpowered/infinite-you/pkg/services/factory_sessions/internal/services/response_stream/wire"
	factorysessionservice "github.com/portpowered/infinite-you/pkg/services/factory_sessions/internal/sessionservice"
	"github.com/portpowered/infinite-you/pkg/services/factory_sessions/internal/stream"
	factorysessioncontracts "github.com/portpowered/infinite-you/pkg/services/factory_sessions/wire/contracts"
)

type liveRuntimeEffectHost struct {
	openTestHost
	openCalls int
	listCalls int
	getCalls  int
	stopCalls int
	factory   *gatewayLifecycleFactory
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
	return newLiveRuntimeCompositionGatewayWithCoordinator(t, host, livechange.NewCoordinator())
}

func newLiveRuntimeCompositionGatewayWithCoordinator(
	t *testing.T,
	host liveRuntimeGatewayHost,
	coordinator factorysessioncontracts.LiveChangeCoordinator,
) *factorysessionservice.Service {
	t.Helper()
	responseService, err := responsestreamwire.NewService(func() string { return "response-event-live-runtime" }, nil, newTestEventsServiceForSessionService(t))
	if err != nil {
		t.Fatalf("construct response-stream service: %v", err)
	}
	registry, err := responseService.NewStreamRegistry(serviceTestClock)
	if err != nil {
		t.Fatalf("construct response-stream registry: %v", err)
	}
	return factorysessionservice.NewWithLiveChangeCoordinator(host, host, host, registry, nil, nil, responseService, coordinator)
}

type recordingLiveChangeCoordinator struct {
	applyCalls   int
	recoverCalls int
}

func (c *recordingLiveChangeCoordinator) ApplyLiveChange(
	context.Context,
	string,
	factorysessions.LiveChangeRequest,
	factorysessions.LiveChangeOperation,
) (factorysessions.LiveChangeResult, error) {
	c.applyCalls++
	return factorysessions.LiveChangeResult{Outcome: factorysessions.LiveChangeOutcomeNoOp}, nil
}

func (c *recordingLiveChangeCoordinator) RecoverLiveChange(
	context.Context,
	string,
	string,
	factorysessions.LiveChangeOperation,
) (factorysessions.LiveChangeResult, error) {
	c.recoverCalls++
	return factorysessions.LiveChangeResult{Outcome: factorysessions.LiveChangeOutcomeReplayed}, nil
}

func TestService_UsesInjectedLiveChangeCoordinator(t *testing.T) {
	coordinator := &recordingLiveChangeCoordinator{}
	const sessionID = "session-injected-coordinator"
	factory := &gatewayLifecycleFactory{factoryState: "RUNNING"}
	session := &livesession.LiveSession{
		ID: sessionID,
		Runtime: &factorysessions.LiveRuntime{
			Factory: factory,
			Clock:   serviceTestClock,
		},
	}
	host := &liveRuntimeEffectHost{
		openTestHost: openTestHost{sessions: map[string]*livesession.LiveSession{sessionID: session}},
		factory:      factory,
	}
	gateway := newLiveRuntimeCompositionGatewayWithCoordinator(t, host, coordinator)
	result, err := gateway.ApplyLiveChange(
		context.Background(),
		sessionID,
		factorysessions.LiveChangeRequest{RequestID: "request-injected-coordinator"},
	)
	if err != nil {
		t.Fatalf("ApplyLiveChange: %v", err)
	}
	if result.Outcome != factorysessions.LiveChangeOutcomeNoOp || coordinator.applyCalls != 1 {
		t.Fatalf("result = %#v, apply calls = %d, want injected coordinator", result, coordinator.applyCalls)
	}
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

// pkgmaintcheck:ignore-cyclomatic-complexity pre-existing baseline debt recorded 2026-08-08; refactor this code below the maintainability threshold and remove this exemption
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
	if _, ok := any(factory).(interface {
		GetEngineStateSnapshot(context.Context) (*interfaces.EngineStateSnapshot[factoryruntime.PetriMarkingSnapshot, *factoryruntime.RuntimeNet], error)
	}); ok {
		t.Fatal("peer-shaped session runtime must not implement the Petri snapshot capability")
	}
}

type liveChangeCompositionEventLog struct {
	events []interfaces.FactoryEvent
}

func (log *liveChangeCompositionEventLog) AppendLiveChangeEvent(event interfaces.FactoryEvent) (interfaces.FactoryEvent, error) {
	event.SchemaVersion = interfaces.FactoryEventSchemaVersionV1
	event.Context.Sequence = len(log.events)
	if event.Type == interfaces.FactoryEventTypeFactoryChange {
		var payload interfaces.FactoryChangeEventPayload
		if err := json.Unmarshal(event.Payload, &payload); err == nil && payload.ChangeID != "" {
			sequence := event.Context.Sequence
			payload.EffectiveSequence = &sequence
			event.Payload, _ = json.Marshal(payload)
		}
	}
	log.events = append(log.events, event.Clone())
	return event.Clone(), nil
}

func (log *liveChangeCompositionEventLog) LiveChangeEvents() []interfaces.FactoryEvent {
	cloned := make([]interfaces.FactoryEvent, len(log.events))
	for index, event := range log.events {
		cloned[index] = event.Clone()
	}
	return cloned
}

type liveChangeCompositionApplication struct {
	snapshot  *interfaces.FactorySnapshot
	applyCall int
}

func (app *liveChangeCompositionApplication) PreflightLiveChange(context.Context, factorysessions.LiveChangeApplicationRequest) (factorysessions.LiveChangePreflightResult, error) {
	return factorysessions.LiveChangePreflightResult{Admissible: true}, nil
}

func (app *liveChangeCompositionApplication) ApplyLiveChange(context.Context, factorysessions.LiveChangeApplicationRequest) (factorysessions.LiveChangeApplicationResult, error) {
	app.applyCall++
	return factorysessions.LiveChangeApplicationResult{Factory: app.snapshot}, nil
}

func TestService_LiveChangeUsesRootCapabilityAndSessionScopedCanonicalEvents(t *testing.T) {
	const sessionID = "session-live-change"
	updated := liveChangeUpdatedSnapshot(t)
	log := &liveChangeCompositionEventLog{}
	application := &liveChangeCompositionApplication{snapshot: updated}
	factory := &gatewayLifecycleFactory{factoryState: "RUNNING"}
	session := &livesession.LiveSession{
		ID: sessionID,
		Runtime: &factorysessions.LiveRuntime{
			Factory:               factory,
			Clock:                 serviceTestClock,
			LiveChangeEvents:      log,
			LiveChangeApplication: application,
		},
	}
	host := &liveRuntimeEffectHost{
		openTestHost: openTestHost{sessions: map[string]*livesession.LiveSession{sessionID: session}},
		factory:      factory,
	}
	gateway := newLiveRuntimeCompositionGateway(t, host)
	var capability factorysessions.LiveChangeService = gateway
	request := factorysessions.LiveChangeRequest{
		RequestID:        "request-root-live-change",
		ExpectedRevision: 0,
		Operation:        "resource.capacity.set",
		TargetID:         "reviewers",
		RequestedValue:   json.RawMessage("8"),
		Source:           "test",
	}
	result, err := capability.ApplyLiveChange(context.Background(), sessionID, request)
	if err != nil {
		t.Fatalf("ApplyLiveChange: %v", err)
	}
	assertLiveChangeCompositionResult(t, result, sessionID, application)
	assertLiveChangeCompositionEvents(t, log.events, sessionID, request.RequestID)

	replayed, err := capability.ApplyLiveChange(context.Background(), sessionID, request)
	if err != nil {
		t.Fatalf("replay ApplyLiveChange: %v", err)
	}
	if replayed.Outcome != factorysessions.LiveChangeOutcomeReplayed || len(log.events) != 2 || application.applyCall != 1 {
		t.Fatalf("replayed root live change = %#v events=%d applicationCalls=%d, want idempotent outcome", replayed, len(log.events), application.applyCall)
	}
}

func liveChangeUpdatedSnapshot(t *testing.T) *interfaces.FactorySnapshot {
	t.Helper()
	updated, err := interfaces.NewFactorySnapshot(map[string]any{"name": "updated", "revision": 1})
	if err != nil {
		t.Fatalf("create updated snapshot: %v", err)
	}
	return updated
}

func assertLiveChangeCompositionResult(
	t *testing.T,
	result factorysessions.LiveChangeResult,
	sessionID string,
	application *liveChangeCompositionApplication,
) {
	t.Helper()
	if result.SessionID != sessionID || result.Outcome != factorysessions.LiveChangeOutcomeApplied ||
		result.PreviousRevision != 0 || result.NewRevision != 1 || result.EffectiveSequence != 1 || application.applyCall != 1 {
		t.Fatalf("live change result = %#v, application calls=%d, want root-scoped applied revision 0->1", result, application.applyCall)
	}
}

func assertLiveChangeCompositionEvents(t *testing.T, events []interfaces.FactoryEvent, sessionID, requestID string) {
	t.Helper()
	if len(events) != 2 {
		t.Fatalf("canonical event count = %d, want request and one terminal event", len(events))
	}
	for index, event := range events {
		if event.Context.SessionID == nil || *event.Context.SessionID != sessionID || event.Context.RequestID == nil || *event.Context.RequestID != requestID {
			t.Fatalf("event[%d] context = %#v, want session/request correlation", index, event.Context)
		}
	}
}
