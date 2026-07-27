package wire_test

import (
	"context"
	"errors"
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

func TestNewServiceRoutesStartIdempotencyThroughOwner(t *testing.T) {
	t.Parallel()

	stub := &idempotencySpy{}
	service, err := durableexecutionwire.NewService(stub)
	if err != nil {
		t.Fatalf("NewService: %v", err)
	}

	request := factorysessions.DurableStartRequest{RequestID: "req-idempotent"}
	first, err := service.StartAsync(context.Background(), request)
	if err != nil || first.SessionID != "sess-idempotent" {
		t.Fatalf("first StartAsync = (%#v, %v), want stable durable async start", first, err)
	}
	second, err := service.StartAsync(context.Background(), request)
	if err != nil || second.SessionID != first.SessionID {
		t.Fatalf("replay StartAsync = (%#v, %v), want same session identity", second, err)
	}

	conflict := request
	conflict.Args = map[string]any{"changed": true}
	if _, err := service.StartAsync(context.Background(), conflict); !errors.Is(err, factorysessions.ErrExecutionRequestIDConflict) {
		t.Fatalf("conflicting StartAsync error = %v, want ErrExecutionRequestIDConflict", err)
	}
}

func TestNewServiceRoutesControlThroughOwner(t *testing.T) {
	t.Parallel()

	stub := &controlSpy{}
	service, err := durableexecutionwire.NewService(stub)
	if err != nil {
		t.Fatalf("NewService: %v", err)
	}

	paused, err := service.Pause(context.Background(), "sess-control", factorysessions.ControlRequest{})
	if err != nil || paused.SessionID != "sess-control" ||
		paused.Outcome != factorysessions.LifecycleControlOutcomeAccepted ||
		paused.Status != factorysessions.LifecycleStatusPaused {
		t.Fatalf("Pause = (%#v, %v), want published durable control result", paused, err)
	}
	if stub.pauseCalls != 1 {
		t.Fatalf("pause calls = %d, want 1", stub.pauseCalls)
	}
}

func TestNewServiceRoutesInspectThroughOwner(t *testing.T) {
	t.Parallel()

	stub := &inspectSpy{}
	service, err := durableexecutionwire.NewService(stub)
	if err != nil {
		t.Fatalf("NewService: %v", err)
	}

	inspect, err := service.GetSession(context.Background(), "sess-inspect")
	if err != nil || inspect.SessionID != "sess-inspect" || inspect.Status != factorysessions.LifecycleStatusRunning {
		t.Fatalf("GetSession = (%#v, %v), want published durable inspect result", inspect, err)
	}
	if stub.inspectCalls != 1 {
		t.Fatalf("inspect calls = %d, want 1", stub.inspectCalls)
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

type controlSpy struct {
	factorysessions.ExecutionService
	pauseCalls int
}

func (s *controlSpy) Pause(_ context.Context, sessionID string, _ factorysessions.ControlRequest) (factorysessions.LifecycleControlResult, error) {
	s.pauseCalls++
	return factorysessions.LifecycleControlResult{
		SessionID: sessionID,
		Operation: factorysessions.LifecycleControlPause,
		Outcome:   factorysessions.LifecycleControlOutcomeAccepted,
		Status:    factorysessions.LifecycleStatusPaused,
	}, nil
}

type inspectSpy struct {
	factorysessions.ExecutionService
	inspectCalls int
}

func (s *inspectSpy) GetSession(_ context.Context, sessionID string) (factorysessions.SessionReadResult, error) {
	s.inspectCalls++
	return factorysessions.SessionReadResult{
		SessionID: sessionID,
		Status:    factorysessions.LifecycleStatusRunning,
	}, nil
}

type idempotencySpy struct {
	factorysessions.ExecutionService
	recorded map[string]factorysessions.StartRequest
}

func (s *idempotencySpy) StartAsync(_ context.Context, req factorysessions.StartRequest) (factorysessions.AsyncStartResult, error) {
	if s.recorded == nil {
		s.recorded = make(map[string]factorysessions.StartRequest)
	}
	if prior, ok := s.recorded[req.RequestID]; ok {
		if len(prior.Args) != len(req.Args) {
			return factorysessions.AsyncStartResult{}, factorysessions.ErrExecutionRequestIDConflict
		}
		for key, value := range req.Args {
			if prior.Args[key] != value {
				return factorysessions.AsyncStartResult{}, factorysessions.ErrExecutionRequestIDConflict
			}
		}
		return factorysessions.AsyncStartResult{
			SessionID: "sess-idempotent",
			Status:    string(factorysessions.LifecycleStatusRunning),
		}, nil
	}
	s.recorded[req.RequestID] = req
	return factorysessions.AsyncStartResult{
		SessionID: "sess-idempotent",
		Status:    string(factorysessions.LifecycleStatusRunning),
	}, nil
}

func (s *idempotencySpy) StartSync(context.Context, factorysessions.StartRequest) (factorysessions.SyncStartResult, error) {
	return factorysessions.SyncStartResult{}, errors.New("unexpected StartSync")
}
