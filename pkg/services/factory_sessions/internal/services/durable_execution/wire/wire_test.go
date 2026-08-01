package wire_test

import (
	"context"
	"encoding/json"
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

func TestNewServiceRoutesResumeThroughOwner(t *testing.T) {
	t.Parallel()

	stub := &resumeSpy{}
	service, err := durableexecutionwire.NewService(stub)
	if err != nil {
		t.Fatalf("NewService: %v", err)
	}

	resumed, err := service.ResumeInterruptedSession(
		context.Background(),
		"sess-resume",
		factorysessions.DurableResumeRequest{RequestID: "req-resume"},
	)
	if err != nil || resumed.SessionID != "sess-resume" ||
		resumed.Status != string(factorysessions.LifecycleStatusResuming) {
		t.Fatalf("ResumeInterruptedSession = (%#v, %v), want published durable resume result", resumed, err)
	}
	if stub.resumeCalls != 1 {
		t.Fatalf("resume calls = %d, want 1", stub.resumeCalls)
	}
}

func TestNewServiceRoutesRestartReadsThroughOwner(t *testing.T) {
	t.Parallel()

	stub := &restartReadSpy{}
	service, err := durableexecutionwire.NewService(stub)
	if err != nil {
		t.Fatalf("NewService: %v", err)
	}
	ctx := context.Background()
	sessionID := "sess-restart"

	inspect, err := service.GetSession(ctx, sessionID)
	if err != nil || inspect.SessionID != sessionID || inspect.Status != factorysessions.LifecycleStatusSucceeded {
		t.Fatalf("GetSession = (%#v, %v), want reconstructed durable inspect projection", inspect, err)
	}
	result, err := service.GetResult(ctx, sessionID, factorysessions.ResultRequest{Mode: factorysessions.ResultModeFinal})
	if err != nil || result.SessionID != sessionID || result.ResultStatus != factorysessions.ResultStatusFinal {
		t.Fatalf("GetResult = (%#v, %v), want reconstructed durable result projection", result, err)
	}
	dispatches, err := service.ListDispatches(ctx, sessionID)
	if err != nil || dispatches.SessionID != sessionID || len(dispatches.Dispatches) != 1 {
		t.Fatalf("ListDispatches = (%#v, %v), want reconstructed durable dispatch projection", dispatches, err)
	}
	events, err := service.ReadEvents(ctx, sessionID, factorysessions.EventReconnectRequest{})
	if err != nil || events.SessionID != sessionID || len(events.Events) != 1 {
		t.Fatalf("ReadEvents = (%#v, %v), want reconstructed durable event projection", events, err)
	}
	if stub.inspectCalls != 1 || stub.resultCalls != 1 || stub.dispatchCalls != 1 || stub.eventCalls != 1 {
		t.Fatalf("restart read calls = inspect %d result %d dispatches %d events %d, want 1 each",
			stub.inspectCalls, stub.resultCalls, stub.dispatchCalls, stub.eventCalls)
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

type resumeSpy struct {
	factorysessions.ExecutionService
	resumeCalls int
}

func (s *resumeSpy) ResumeInterruptedSession(
	_ context.Context,
	sessionID string,
	_ factorysessions.DurableResumeRequest,
) (factorysessions.AsyncStartResult, error) {
	s.resumeCalls++
	return factorysessions.AsyncStartResult{
		SessionID: sessionID,
		Status:    string(factorysessions.LifecycleStatusResuming),
	}, nil
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

type restartReadSpy struct {
	factorysessions.ExecutionService
	inspectCalls  int
	resultCalls   int
	dispatchCalls int
	eventCalls    int
}

func (s *restartReadSpy) GetSession(_ context.Context, sessionID string) (factorysessions.SessionReadResult, error) {
	s.inspectCalls++
	return factorysessions.SessionReadResult{
		SessionID: sessionID,
		Status:    factorysessions.LifecycleStatusSucceeded,
	}, nil
}

func (s *restartReadSpy) GetResult(_ context.Context, sessionID string, _ factorysessions.ResultRequest) (factorysessions.ResultReadResult, error) {
	s.resultCalls++
	return factorysessions.ResultReadResult{
		SessionID:    sessionID,
		ResultStatus: factorysessions.ResultStatusFinal,
	}, nil
}

func (s *restartReadSpy) ListDispatches(_ context.Context, sessionID string) (factorysessions.ListDispatchesResult, error) {
	s.dispatchCalls++
	return factorysessions.ListDispatchesResult{
		SessionID: sessionID,
		Dispatches: []factorysessions.DispatchSummary{{
			ID:     "dispatch-restart-1",
			Status: "COMPLETED",
		}},
	}, nil
}

func (s *restartReadSpy) ReadEvents(_ context.Context, sessionID string, _ factorysessions.EventReconnectRequest) (factorysessions.EventReadResult, error) {
	s.eventCalls++
	return factorysessions.EventReadResult{
		SessionID: sessionID,
		Events:    []json.RawMessage{json.RawMessage(`{"type":"SESSION_STARTED"}`)},
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
