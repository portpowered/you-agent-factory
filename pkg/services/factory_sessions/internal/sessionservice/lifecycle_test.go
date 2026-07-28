package service_test

import (
	"context"
	"testing"

	"github.com/portpowered/infinite-you/pkg/services/work"

	interfaces "github.com/portpowered/infinite-you/pkg/services/factory_definitions"
	"github.com/portpowered/infinite-you/pkg/services/factory_runtime"
	factorysessionexecution "github.com/portpowered/infinite-you/pkg/services/factory_sessions/internal/execution"
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
	factory   factory.Service
	stopCalls []string
}

func (h *lifecycleGatewayHost) SessionFactory(_ string) (factory.Service, error) {
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

func (f *gatewayLifecycleFactory) CaptureCheckpoint(_ context.Context, req factory.CaptureCheckpointRequest) (factory.CaptureCheckpointResult, error) {
	id := req.CheckpointID
	if id == "" {
		id = "checkpoint-stub"
	}
	return factory.CaptureCheckpointResult{
		Outcome: factory.CheckpointOutcomeCaptured,
		Checkpoint: factory.Checkpoint{
			CheckpointID:  id,
			SchemaVersion: 1,
			StrategyKind:  "runtime",
			Payload:       []byte(`{}`),
		},
	}, nil
}

func (f *gatewayLifecycleFactory) LoadCheckpoint(_ context.Context, req factory.LoadCheckpointRequest) (factory.LoadCheckpointResult, error) {
	if req.CheckpointID == "" {
		return factory.LoadCheckpointResult{}, factory.ErrCheckpointNotFound
	}
	return factory.LoadCheckpointResult{}, factory.ErrCheckpointNotFound
}

func (f *gatewayLifecycleFactory) RestoreCheckpoint(_ context.Context, req factory.RestoreCheckpointRequest) (factory.RestoreCheckpointResult, error) {
	return factory.RestoreCheckpointResult{
		Outcome:      factory.CheckpointOutcomeRestored,
		CheckpointID: req.Checkpoint.CheckpointID,
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

func TestService_ObserveForSession_ForwardsRootObserveRequest(t *testing.T) {
	t.Parallel()

	runtimeFactory := &gatewayLifecycleFactory{factoryState: string(interfaces.FactoryStateRunning)}
	host := &lifecycleGatewayHost{factory: runtimeFactory}
	gateway := newServiceTestGateway(host)

	result, err := gateway.ObserveForSession(
		context.Background(),
		"sess-observe",
		factory.ObserveRequest{Scope: factory.ObservationScopeStatus},
	)
	if err != nil {
		t.Fatalf("ObserveForSession: %v", err)
	}
	if runtimeFactory.observeCalls != 1 {
		t.Fatalf("Observe calls = %d, want 1 through Runtime root Service", runtimeFactory.observeCalls)
	}
	if runtimeFactory.lastObserveRequest.Scope != factory.ObservationScopeStatus {
		t.Fatalf("observe scope = %q, want STATUS", runtimeFactory.lastObserveRequest.Scope)
	}
	if result.Observation.Status != factory.ObservationStatusActive {
		t.Fatalf("observation status = %q, want ACTIVE", result.Observation.Status)
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
	durable, err := gateway.PauseDurableFactorySession(
		context.Background(),
		"dur-sess-js-run-n-001",
		factorysessionexecution.ControlRequest{},
	)
	if err != nil {
		t.Fatalf("PauseDurableFactorySession: %v", err)
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
