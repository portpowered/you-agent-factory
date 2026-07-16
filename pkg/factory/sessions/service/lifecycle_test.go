package service_test

import (
	"context"
	"testing"

	"github.com/portpowered/infinite-you/pkg/work"

	"github.com/portpowered/infinite-you/pkg/factory"
	interfaces "github.com/portpowered/infinite-you/pkg/factory/contracts"
	factorysessionexecution "github.com/portpowered/infinite-you/pkg/factory/sessions/execution"
	factorysessionservice "github.com/portpowered/infinite-you/pkg/factory/sessions/service"
	"github.com/portpowered/infinite-you/pkg/factory/state"
	"github.com/portpowered/infinite-you/pkg/orchestrators/petri"
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
	factory   factory.Factory
	stopCalls []string
}

func (h *lifecycleGatewayHost) SessionFactory(_ string) (factory.Factory, error) {
	return h.factory, nil
}

func (h *lifecycleGatewayHost) StopLiveSession(sessionID string) error {
	h.stopCalls = append(h.stopCalls, sessionID)
	return nil
}

type gatewayLifecycleFactory struct {
	factoryState string
}

func (f *gatewayLifecycleFactory) Run(context.Context) error { return nil }

func (f *gatewayLifecycleFactory) Pause(context.Context) error { return nil }

func (f *gatewayLifecycleFactory) Resume(context.Context) error { return nil }

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

func (f *gatewayLifecycleFactory) SubscribeFactoryEvents(context.Context, *interfaces.FactoryEventReconnectCursor, interfaces.FactoryEventReconnectScope) (*interfaces.FactoryEventStream, error) {
	return nil, nil
}

func (f *gatewayLifecycleFactory) GetEngineStateSnapshot(context.Context) (*interfaces.EngineStateSnapshot[petri.MarkingSnapshot, *state.Net], error) {
	return &interfaces.EngineStateSnapshot[petri.MarkingSnapshot, *state.Net]{
		FactoryState: f.factoryState,
	}, nil
}

func (f *gatewayLifecycleFactory) MoveWork(context.Context, string, string, work.WorkStateChangeSource, string) (work.OperatorMoveResult, error) {
	return work.OperatorMoveResult{}, nil
}

func TestService_PauseLiveFactorySession_DelegatesToDataplane(t *testing.T) {
	t.Parallel()

	host := &lifecycleGatewayHost{
		factory: &gatewayLifecycleFactory{factoryState: string(interfaces.FactoryStateRunning)},
	}
	gateway := factorysessionservice.New(host)

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
}

func TestService_ResumeLiveFactorySession_DelegatesToDataplane(t *testing.T) {
	t.Parallel()

	host := &lifecycleGatewayHost{
		factory: &gatewayLifecycleFactory{factoryState: string(interfaces.FactoryStatePaused)},
	}
	gateway := factorysessionservice.New(host)

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
	gateway := factorysessionservice.New(host)

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
	gateway := factorysessionservice.New(host)

	if err := gateway.CloseFactorySession(context.Background(), "sess-close"); err != nil {
		t.Fatalf("CloseFactorySession: %v", err)
	}
	if len(host.stopCalls) != 1 || host.stopCalls[0] != "sess-close" {
		t.Fatalf("stop calls = %#v, want [sess-close]", host.stopCalls)
	}
}

func TestService_CloseFactorySession_RejectsEmptySessionID(t *testing.T) {
	t.Parallel()

	gateway := factorysessionservice.New(&lifecycleGatewayHost{})
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
