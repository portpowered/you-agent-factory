package wire_test

import (
	"context"
	"testing"

	factorysessions "github.com/portpowered/infinite-you/pkg/services/factory_sessions"
	durableexecutionwire "github.com/portpowered/infinite-you/pkg/services/factory_sessions/internal/services/durable_execution/wire"
)

func TestNewServiceConstructsInertCapability(t *testing.T) {
	t.Parallel()

	stub := &executionSpy{}
	service, err := durableexecutionwire.NewService(stub)
	if err != nil || service == nil {
		t.Fatalf("NewService = (%v, %v)", service, err)
	}
	if stub.calls != 0 {
		t.Fatalf("construction invoked execution %d times, want 0", stub.calls)
	}
}

func TestNewServiceRejectsMissingExecutionCollaborator(t *testing.T) {
	t.Parallel()

	service, err := durableexecutionwire.NewService(nil)
	if err == nil || service != nil {
		t.Fatalf("NewService(nil) = (%v, %v), want deterministic dependency error", service, err)
	}
}

func TestNewServiceDefersRuntimeEffectsUntilInvocation(t *testing.T) {
	t.Parallel()

	stub := &executionSpy{}
	service, err := durableexecutionwire.NewService(stub)
	if err != nil {
		t.Fatalf("NewService: %v", err)
	}

	if _, err := service.StartAsync(context.Background(), factorysessions.StartRequest{RequestID: "req-1"}); err != nil {
		t.Fatalf("StartAsync: %v", err)
	}
	if _, err := service.GetSession(context.Background(), "sess-1"); err != nil {
		t.Fatalf("GetSession: %v", err)
	}
	if stub.calls != 2 {
		t.Fatalf("runtime calls = %d, want 2 after start and inspect", stub.calls)
	}
}

func TestNewServiceRoutesDurableStartThroughOwner(t *testing.T) {
	t.Parallel()

	stub := &executionSpy{}
	service, err := durableexecutionwire.NewService(stub)
	if err != nil {
		t.Fatalf("NewService: %v", err)
	}

	asyncStarted, err := service.StartAsync(context.Background(), factorysessions.DurableStartRequest{
		RequestID: "req-async",
	})
	if err != nil || asyncStarted.SessionID != "sess-1" || asyncStarted.Status != string(factorysessions.LifecycleStatusRunning) {
		t.Fatalf("StartAsync = (%#v, %v), want published durable async start result", asyncStarted, err)
	}

	syncStarted, err := service.StartSync(context.Background(), factorysessions.DurableStartRequest{
		RequestID: "req-sync",
	})
	if err != nil || syncStarted.SessionID != "sess-sync" || syncStarted.SyncOutcome != factorysessions.SyncOutcome("COMPLETED") {
		t.Fatalf("StartSync = (%#v, %v), want published durable sync start result", syncStarted, err)
	}
	if stub.startAsyncCalls != 1 || stub.startSyncCalls != 1 {
		t.Fatalf("start calls = async %d sync %d, want 1 each", stub.startAsyncCalls, stub.startSyncCalls)
	}
}

type executionSpy struct {
	factorysessions.ExecutionService
	calls           int
	startAsyncCalls int
	startSyncCalls  int
}

func (s *executionSpy) StartAsync(context.Context, factorysessions.StartRequest) (factorysessions.AsyncStartResult, error) {
	s.calls++
	s.startAsyncCalls++
	return factorysessions.AsyncStartResult{
		SessionID: "sess-1",
		Status:    string(factorysessions.LifecycleStatusRunning),
	}, nil
}

func (s *executionSpy) StartSync(context.Context, factorysessions.StartRequest) (factorysessions.SyncStartResult, error) {
	s.calls++
	s.startSyncCalls++
	return factorysessions.SyncStartResult{
		AsyncStartResult: factorysessions.AsyncStartResult{
			SessionID: "sess-sync",
			Status:    string(factorysessions.LifecycleStatusSucceeded),
		},
		SyncOutcome: factorysessions.SyncOutcome("COMPLETED"),
	}, nil
}

func (s *executionSpy) GetSession(context.Context, string) (factorysessions.SessionReadResult, error) {
	s.calls++
	return factorysessions.SessionReadResult{SessionID: "sess-1"}, nil
}
