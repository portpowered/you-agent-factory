package service_test

import (
	"context"
	"errors"
	"reflect"
	"testing"

	"github.com/portpowered/infinite-you/pkg/services/work"

	interfaces "github.com/portpowered/infinite-you/pkg/services/factory_definitions"
	"github.com/portpowered/infinite-you/pkg/services/factory_runtime"
	factorysessions "github.com/portpowered/infinite-you/pkg/services/factory_sessions"
	factorysessionexecution "github.com/portpowered/infinite-you/pkg/services/factory_sessions/internal/execution"
	"github.com/portpowered/infinite-you/pkg/services/factory_sessions/internal/livesession"
	factorysessionservice "github.com/portpowered/infinite-you/pkg/services/factory_sessions/internal/sessionservice"
)

type unifiedLifecycleGatewayHost struct {
	lifecycleGatewayHost
	execution factorysessionexecution.Service
}

func (h *unifiedLifecycleGatewayHost) DurableExecution() factorysessionexecution.Service {
	return h.execution
}

type lifecycleGatewayHost struct {
	openTestHost
	factory             factory.Service
	sessionFactoryErr   error
	sessionFactoryCalls []string
	stopCalls           []string
}

func (h *lifecycleGatewayHost) SessionFactory(sessionID string) (factory.Service, error) {
	h.sessionFactoryCalls = append(h.sessionFactoryCalls, sessionID)
	if h.sessionFactoryErr != nil {
		return nil, h.sessionFactoryErr
	}
	return h.factory, nil
}

func (h *lifecycleGatewayHost) StopLiveSession(sessionID string) error {
	h.stopCalls = append(h.stopCalls, sessionID)
	return nil
}

type gatewayLifecycleFactory struct {
	factoryState       string
	observeResult      factory.Observation
	useObserveResult   bool
	subscribeFactoryFn func(context.Context, *interfaces.FactoryEventReconnectCursor, interfaces.FactoryEventReconnectScope) (*interfaces.FactoryEventStream, error)
	pauseCalls         int
	resumeCalls        int
	terminateCalls     int
	observeCalls       int
	lastObserveRequest factory.ObserveRequest
}

func (f *gatewayLifecycleFactory) Run(context.Context) error { return nil }

func (f *gatewayLifecycleFactory) Pause(context.Context) error { f.pauseCalls++; return nil }

func (f *gatewayLifecycleFactory) Resume(context.Context) error { f.resumeCalls++; return nil }

func (f *gatewayLifecycleFactory) ControlPause(ctx context.Context, _ factory.PauseRequest) (factory.PauseResult, error) {
	err := f.Pause(ctx)
	return factory.PauseResult{Outcome: factory.ControlOutcomeAccepted}, err
}

func (f *gatewayLifecycleFactory) ControlResume(ctx context.Context, _ factory.ResumeRequest) (factory.ResumeResult, error) {
	err := f.Resume(ctx)
	return factory.ResumeResult{Outcome: factory.ControlOutcomeAccepted}, err
}

func (f *gatewayLifecycleFactory) Terminate(context.Context, factory.TerminateRequest) (factory.TerminateResult, error) {
	f.terminateCalls++
	return factory.TerminateResult{Outcome: factory.ControlOutcomeAccepted}, nil
}

func (f *gatewayLifecycleFactory) ControlTerminate(ctx context.Context, req factory.TerminateRequest) (factory.TerminateResult, error) {
	return f.Terminate(ctx, req)
}

func (f *gatewayLifecycleFactory) ControlWaitToComplete(factory.WaitToCompleteRequest) factory.WaitToCompleteResult {
	return factory.WaitToCompleteResult{Done: f.WaitToComplete()}
}

func (f *gatewayLifecycleFactory) ControlMoveWork(
	ctx context.Context,
	req factory.MoveWorkRequest,
) (factory.MoveWorkResult, error) {
	result, err := f.MoveWork(ctx, req.WorkID, req.StateName, work.WorkStateChangeSource(req.Source), req.RequestID)
	return factory.MoveWorkResult{
		WorkID: result.WorkID, WorkTypeID: result.WorkTypeID,
		FromState: result.FromState, ToState: result.ToState,
	}, err
}

func (f *gatewayLifecycleFactory) Observe(_ context.Context, req factory.ObserveRequest) (factory.ObserveResult, error) {
	f.observeCalls++
	f.lastObserveRequest = req
	if f.useObserveResult {
		return factory.ObserveResult{Observation: f.observeResult}, nil
	}
	return factory.ObserveResult{
		Observation: factory.Observation{
			Status: factory.ObservationStatusActive,
			Health: factory.ObservationHealth{FactoryState: f.factoryState},
		},
	}, nil
}

func (*gatewayLifecycleFactory) CleanInvocationSnapshot(context.Context) (factory.CleanInvocationSnapshot, error) {
	return factory.CleanInvocationSnapshot{}, nil
}

func (f *gatewayLifecycleFactory) PlanDispatch(_ context.Context, req factory.PlanDispatchRequest) (factory.PlanDispatchResult, error) {
	return factory.PlanDispatchResult{
		Outcome:       factory.DispatchPlanOutcomeAccepted,
		DispatchID:    req.DispatchID,
		CorrelationID: req.CorrelationID,
	}, nil
}

func (f *gatewayLifecycleFactory) AcceptDispatchResult(_ context.Context, req factory.AcceptDispatchResultRequest) (factory.AcceptDispatchResultResult, error) {
	return factory.AcceptDispatchResultResult{
		Outcome:       factory.DispatchPlanOutcomeRetired,
		DispatchID:    req.DispatchID,
		CorrelationID: req.CorrelationID,
	}, nil
}

func (f *gatewayLifecycleFactory) GetFactoryEvents(context.Context) ([]interfaces.FactoryEvent, error) {
	return nil, nil
}

func (f *gatewayLifecycleFactory) WaitToComplete() <-chan struct{} {
	ch := make(chan struct{})
	close(ch)
	return ch
}

func (f *gatewayLifecycleFactory) SubmitWorkRequest(context.Context, work.WorkRequest) (work.WorkRequestSubmitResult, error) {
	return work.WorkRequestSubmitResult{}, nil
}

func (f *gatewayLifecycleFactory) SubscribeFactoryEvents(ctx context.Context, reconnect *interfaces.FactoryEventReconnectCursor, scope interfaces.FactoryEventReconnectScope) (*interfaces.FactoryEventStream, error) {
	if f.subscribeFactoryFn != nil {
		return f.subscribeFactoryFn(ctx, reconnect, scope)
	}
	return nil, nil
}

func TestService_ProbeFactoryEventsForSession_CancelsOwnedSubscription(t *testing.T) {
	t.Parallel()

	var subscriptionContext context.Context
	factory := &gatewayLifecycleFactory{
		subscribeFactoryFn: func(
			ctx context.Context,
			_ *interfaces.FactoryEventReconnectCursor,
			scope interfaces.FactoryEventReconnectScope,
		) (*interfaces.FactoryEventStream, error) {
			if scope.SessionID != "sess-probe" {
				t.Fatalf("scope session = %q, want sess-probe", scope.SessionID)
			}
			subscriptionContext = ctx
			return &interfaces.FactoryEventStream{}, nil
		},
	}
	gateway := newServiceTestGateway(&lifecycleGatewayHost{factory: factory})
	if err := gateway.ProbeFactoryEventsForSession(context.Background(), "sess-probe", nil); err != nil {
		t.Fatalf("ProbeFactoryEventsForSession: %v", err)
	}
	if subscriptionContext == nil {
		t.Fatal("subscription context was not observed")
	}
	select {
	case <-subscriptionContext.Done():
	default:
		t.Fatal("probe-owned subscription context remains active after probe")
	}
}

func TestService_SubscribeFactoryEventsForSession_StampsResolvedCanonicalID(t *testing.T) {
	t.Parallel()

	for _, test := range []struct {
		name          string
		requestedID   string
		canonicalID   string
		eventScopeID  string
		wantFactoryID string
	}{
		{
			name:          "exact selector",
			requestedID:   "session-legacy-exact-001",
			canonicalID:   "session-legacy-exact-001",
			eventScopeID:  "session-legacy-exact-001",
			wantFactoryID: "session-legacy-exact-001",
		},
		{
			name:          "default selector",
			requestedID:   factorysessions.DefaultSessionID,
			canonicalID:   "3c1d4c6b-0d6a-4e8f-b0c0-9e5a2bb1d8aa",
			eventScopeID:  factorysessions.DefaultSessionID,
			wantFactoryID: "3c1d4c6b-0d6a-4e8f-b0c0-9e5a2bb1d8aa",
		},
	} {
		test := test
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()

			events := make(chan interfaces.FactoryEvent)
			history := []interfaces.FactoryEvent{{Id: "legacy-event-1", Type: interfaces.FactoryEventTypeWorkRequest}}
			session := &livesession.LiveSession{
				ID:                      test.requestedID,
				RuntimeFactorySessionID: test.canonicalID,
				RuntimeEventSessionID:   test.eventScopeID,
			}
			factory := &gatewayLifecycleFactory{
				subscribeFactoryFn: func(
					_ context.Context,
					_ *interfaces.FactoryEventReconnectCursor,
					scope interfaces.FactoryEventReconnectScope,
				) (*interfaces.FactoryEventStream, error) {
					if scope.SessionID != test.eventScopeID {
						t.Fatalf("scope session = %q, want %q", scope.SessionID, test.eventScopeID)
					}
					return &interfaces.FactoryEventStream{History: history, Events: events}, nil
				},
			}
			host := &lifecycleGatewayHost{
				openTestHost: openTestHost{requireSession: session},
				factory:      factory,
			}
			gateway := newServiceTestGateway(host)

			stream, err := gateway.SubscribeFactoryEventsForSession(context.Background(), test.requestedID, nil)
			if err != nil {
				t.Fatalf("SubscribeFactoryEventsForSession: %v", err)
			}
			if stream == nil {
				t.Fatal("SubscribeFactoryEventsForSession returned nil stream")
			}
			if stream.FactorySessionID != test.wantFactoryID {
				t.Fatalf("FactorySessionID = %q, want canonical %q", stream.FactorySessionID, test.wantFactoryID)
			}
			if stream.Events != events {
				t.Fatal("SubscribeFactoryEventsForSession replaced the request-local event channel")
			}
			if !reflect.DeepEqual(stream.History, history) {
				t.Fatalf("history = %#v, want unchanged %#v", stream.History, history)
			}
		})
	}
}

type legacyEventSubscriptionFailureCase struct {
	name          string
	host          *lifecycleGatewayHost
	ctx           context.Context
	wantStream    bool
	wantError     error
	wantFactoryID string
}

func TestService_SubscribeFactoryEventsForSession_DoesNotFabricateIdentityOnFailure(t *testing.T) {
	t.Parallel()
	resolvedSession := &livesession.LiveSession{ID: "session-legacy-failure-001"}
	cases := legacyEventSubscriptionFailureCases(resolvedSession, errors.New("subscription failed"))
	for _, test := range cases {
		test := test
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			assertLegacyEventSubscriptionFailure(t, test)
		})
	}
}

func legacyEventSubscriptionFailureCases(
	resolvedSession *livesession.LiveSession,
	subscriptionErr error,
) []legacyEventSubscriptionFailureCase {
	canceledContext := func() context.Context {
		ctx, cancel := context.WithCancel(context.Background())
		cancel()
		return ctx
	}()
	return []legacyEventSubscriptionFailureCase{
		{
			name: "unknown session",
			host: &lifecycleGatewayHost{
				openTestHost:      openTestHost{requireSessionE: factorysessions.ErrSessionNotFound},
				sessionFactoryErr: factorysessions.ErrSessionNotFound,
			},
			wantError: factorysessions.ErrSessionNotFound,
		},
		{
			name: "canceled subscription",
			host: &lifecycleGatewayHost{
				openTestHost: openTestHost{requireSession: resolvedSession},
				factory: &gatewayLifecycleFactory{
					subscribeFactoryFn: func(ctx context.Context, _ *interfaces.FactoryEventReconnectCursor, _ interfaces.FactoryEventReconnectScope) (*interfaces.FactoryEventStream, error) {
						return nil, ctx.Err()
					},
				},
			},
			ctx:       canceledContext,
			wantError: context.Canceled,
		},
		{
			name: "nil stream",
			host: &lifecycleGatewayHost{
				openTestHost: openTestHost{requireSession: resolvedSession},
				factory:      &gatewayLifecycleFactory{},
			},
		},
		{
			name: "partial stream failure",
			host: &lifecycleGatewayHost{
				openTestHost: openTestHost{requireSession: resolvedSession},
				factory: &gatewayLifecycleFactory{
					subscribeFactoryFn: func(context.Context, *interfaces.FactoryEventReconnectCursor, interfaces.FactoryEventReconnectScope) (*interfaces.FactoryEventStream, error) {
						return &interfaces.FactoryEventStream{FactorySessionID: "untrusted-id"}, subscriptionErr
					},
				},
			},
			wantError: subscriptionErr,
		},
		{
			name: "session resolution unavailable",
			host: &lifecycleGatewayHost{
				openTestHost: openTestHost{requireSessionE: factorysessions.ErrSessionNotFound},
				factory: &gatewayLifecycleFactory{
					subscribeFactoryFn: func(context.Context, *interfaces.FactoryEventReconnectCursor, interfaces.FactoryEventReconnectScope) (*interfaces.FactoryEventStream, error) {
						return &interfaces.FactoryEventStream{}, nil
					},
				},
			},
			wantStream: true,
		},
	}
}

func assertLegacyEventSubscriptionFailure(t *testing.T, test legacyEventSubscriptionFailureCase) {
	t.Helper()
	gateway := newServiceTestGateway(test.host)
	ctx := test.ctx
	if ctx == nil {
		ctx = context.Background()
	}
	sessionID := "session-legacy-unknown-001"
	if test.host.openTestHost.requireSession != nil {
		sessionID = test.host.openTestHost.requireSession.ID
	}
	stream, err := gateway.SubscribeFactoryEventsForSession(ctx, sessionID, nil)
	if test.wantError != nil {
		if !errors.Is(err, test.wantError) {
			t.Fatalf("error = %v, want %v", err, test.wantError)
		}
		if stream != nil {
			t.Fatalf("stream = %#v on failed subscription, want nil", stream)
		}
		return
	}
	if err != nil {
		t.Fatalf("SubscribeFactoryEventsForSession: %v", err)
	}
	if (stream != nil) != test.wantStream {
		t.Fatalf("stream present = %t, want %t", stream != nil, test.wantStream)
	}
	if stream != nil && stream.FactorySessionID != test.wantFactoryID {
		t.Fatalf("FactorySessionID = %q, want %q", stream.FactorySessionID, test.wantFactoryID)
	}
}

func (f *gatewayLifecycleFactory) MoveWork(context.Context, string, string, work.WorkStateChangeSource, string) (work.OperatorMoveResult, error) {
	return work.OperatorMoveResult{}, nil
}

func TestService_PauseLiveFactorySession_DelegatesToDataplane(t *testing.T) {
	t.Parallel()

	host := &lifecycleGatewayHost{
		factory: &gatewayLifecycleFactory{factoryState: string(interfaces.FactoryStateRunning)},
	}
	gateway := newServiceTestGateway(host)
	runtimeFactory := host.factory.(*gatewayLifecycleFactory)

	response, err := gateway.PauseLiveFactorySession(
		context.Background(),
		"sess-1",
		factorysessionexecution.ControlRequest{},
	)
	if err != nil {
		t.Fatalf("PauseLiveFactorySession: %v", err)
	}
	if response.Outcome != factorysessionexecution.LifecycleControlOutcomeAccepted {
		t.Fatalf("outcome = %q, want ACCEPTED", response.Outcome)
	}
	if runtimeFactory.pauseCalls != 1 {
		t.Fatalf("ControlPause calls = %d, want 1 through Runtime root Service", runtimeFactory.pauseCalls)
	}
}

func TestService_ResumeLiveFactorySession_DelegatesToDataplane(t *testing.T) {
	t.Parallel()

	host := &lifecycleGatewayHost{
		factory: &gatewayLifecycleFactory{factoryState: string(interfaces.FactoryStatePaused)},
	}
	gateway := newServiceTestGateway(host)

	response, err := gateway.ResumeLiveFactorySession(
		context.Background(),
		"sess-1",
		factorysessionexecution.ControlRequest{},
	)
	if err != nil {
		t.Fatalf("ResumeLiveFactorySession: %v", err)
	}
	if response.Outcome != factorysessionexecution.LifecycleControlOutcomeAccepted {
		t.Fatalf("outcome = %q, want ACCEPTED", response.Outcome)
	}
	if response.Status != factorysessionexecution.LifecycleStatusRunning {
		t.Fatalf("status = %q, want RUNNING", response.Status)
	}
}

func TestService_LifecycleGatewayRoutesLiveAndDurableSessions(t *testing.T) {
	t.Parallel()

	execution := &stubDurableExecution{}
	host := &unifiedLifecycleGatewayHost{
		lifecycleGatewayHost: lifecycleGatewayHost{
			factory: &gatewayLifecycleFactory{factoryState: string(interfaces.FactoryStateRunning)},
		},
		execution: execution,
	}
	gateway := newServiceTestGateway(host)

	live, err := gateway.PauseLiveFactorySession(
		context.Background(),
		"live-session-001",
		factorysessionexecution.ControlRequest{},
	)
	if err != nil {
		t.Fatalf("PauseLiveFactorySession: %v", err)
	}
	durable, err := gateway.Pause(
		context.Background(),
		"dur-sess-js-run-n-001",
		factorysessionexecution.ControlRequest{},
	)
	if err != nil {
		t.Fatalf("Pause: %v", err)
	}

	if live.Status != factorysessionexecution.LifecycleStatusPaused {
		t.Fatalf("live status = %q, want PAUSED", live.Status)
	}
	if durable.Status != factorysessionexecution.LifecycleStatusPaused {
		t.Fatalf("durable status = %q, want PAUSED", durable.Status)
	}
}

func TestService_CloseFactorySession_DelegatesToDataplane(t *testing.T) {
	t.Parallel()

	host := &lifecycleGatewayHost{}
	gateway := newServiceTestGateway(host)

	if err := gateway.CloseFactorySession(context.Background(), "sess-close"); err != nil {
		t.Fatalf("CloseFactorySession: %v", err)
	}
	if len(host.stopCalls) != 1 || host.stopCalls[0] != "sess-close" {
		t.Fatalf("stop calls = %#v, want [sess-close]", host.stopCalls)
	}
}

func TestService_CloseFactorySession_RejectsEmptySessionID(t *testing.T) {
	t.Parallel()

	gateway := newServiceTestGateway(&lifecycleGatewayHost{})
	if err := gateway.CloseFactorySession(context.Background(), "  "); err == nil {
		t.Fatal("CloseFactorySession = nil, want required session id")
	}
}

func TestService_PauseLiveFactorySession_RejectsNilGateway(t *testing.T) {
	t.Parallel()

	var gateway *factorysessionservice.Service
	_, err := gateway.PauseLiveFactorySession(context.Background(), "sess-1", factorysessionexecution.ControlRequest{})
	if err == nil {
		t.Fatal("PauseLiveFactorySession = nil, want gateway required")
	}
}

func (f *gatewayLifecycleFactory) InvokeWorker(_ context.Context, _ factory.InvokeWorkerRequest) (factory.InvokeWorkerResult, error) {
	return factory.InvokeWorkerResult{}, nil
}
