package service_test

import (
	"context"
	"errors"
	"testing"

	"github.com/portpowered/infinite-you/pkg/factory/sessions/controlplane"
	factorysessionexecution "github.com/portpowered/infinite-you/pkg/factory/sessions/execution"
	factorysessionservice "github.com/portpowered/infinite-you/pkg/factory/sessions/service"
)

type durableLifecycleGatewayHost struct {
	openTestHost
	execution factorysessionexecution.Service
}

func (h *durableLifecycleGatewayHost) DurableExecution() factorysessionexecution.Service {
	return h.execution
}

type stubDurableExecution struct {
	pauseResult factorysessionexecution.LifecycleControlResult
	cancelCalls int
}

func (s *stubDurableExecution) StartAsync(context.Context, factorysessionexecution.StartRequest) (factorysessionexecution.AsyncStartResult, error) {
	return factorysessionexecution.AsyncStartResult{}, errors.New("not implemented")
}

func (s *stubDurableExecution) StartSync(context.Context, factorysessionexecution.StartRequest) (factorysessionexecution.SyncStartResult, error) {
	return factorysessionexecution.SyncStartResult{}, errors.New("not implemented")
}

func (s *stubDurableExecution) ResumeInterruptedSession(context.Context, string, factorysessionexecution.ResumeSessionRequest) (factorysessionexecution.AsyncStartResult, error) {
	return factorysessionexecution.AsyncStartResult{}, errors.New("not implemented")
}

func (s *stubDurableExecution) GetSession(context.Context, string) (factorysessionexecution.SessionReadResult, error) {
	return factorysessionexecution.SessionReadResult{}, errors.New("not implemented")
}

func (s *stubDurableExecution) Pause(_ context.Context, sessionID string, _ factorysessionexecution.ControlRequest) (factorysessionexecution.LifecycleControlResult, error) {
	if s.pauseResult.SessionID == "" {
		s.pauseResult = factorysessionexecution.LifecycleControlResult{
			SessionID: sessionID,
			Operation: factorysessionexecution.LifecycleControlPause,
			Outcome:   factorysessionexecution.LifecycleControlOutcomeAccepted,
			Status:    factorysessionexecution.LifecycleStatusPaused,
		}
	}
	return s.pauseResult, nil
}

func (s *stubDurableExecution) Resume(_ context.Context, sessionID string, _ factorysessionexecution.ControlRequest) (factorysessionexecution.LifecycleControlResult, error) {
	return factorysessionexecution.LifecycleControlResult{
		SessionID: sessionID,
		Operation: factorysessionexecution.LifecycleControlResume,
		Outcome:   factorysessionexecution.LifecycleControlOutcomeAccepted,
		Status:    factorysessionexecution.LifecycleStatusRunning,
	}, nil
}

func (s *stubDurableExecution) Cancel(context.Context, string, factorysessionexecution.ControlRequest) (factorysessionexecution.LifecycleControlResult, error) {
	s.cancelCalls++
	return factorysessionexecution.LifecycleControlResult{
		Operation: factorysessionexecution.LifecycleControlCancel,
		Outcome:   factorysessionexecution.LifecycleControlOutcomeAccepted,
		Status:    factorysessionexecution.LifecycleStatusCanceling,
	}, nil
}

func (s *stubDurableExecution) Terminate(_ context.Context, sessionID string, _ factorysessionexecution.ControlRequest) (factorysessionexecution.LifecycleControlResult, error) {
	return factorysessionexecution.LifecycleControlResult{
		SessionID: sessionID,
		Operation: factorysessionexecution.LifecycleControlTerminate,
		Outcome:   factorysessionexecution.LifecycleControlOutcomeAccepted,
		Status:    factorysessionexecution.LifecycleStatusTerminated,
	}, nil
}

func (s *stubDurableExecution) Approve(_ context.Context, sessionID string, _ factorysessionexecution.ApproveRequest) (factorysessionexecution.LifecycleControlResult, error) {
	return factorysessionexecution.LifecycleControlResult{
		SessionID: sessionID,
		Operation: factorysessionexecution.LifecycleControlApprove,
		Outcome:   factorysessionexecution.LifecycleControlOutcomeAccepted,
		Status:    factorysessionexecution.LifecycleStatusRunning,
	}, nil
}

func (s *stubDurableExecution) RetryDispatch(_ context.Context, sessionID string, _ factorysessionexecution.RetryDispatchRequest) (factorysessionexecution.LifecycleControlResult, error) {
	return factorysessionexecution.LifecycleControlResult{
		SessionID: sessionID,
		Operation: factorysessionexecution.LifecycleControlRetryDispatch,
		Outcome:   factorysessionexecution.LifecycleControlOutcomeAccepted,
		Status:    factorysessionexecution.LifecycleStatusRunning,
	}, nil
}

func (s *stubDurableExecution) InterruptDispatch(_ context.Context, sessionID string, _ factorysessionexecution.InterruptDispatchRequest) (factorysessionexecution.LifecycleControlResult, error) {
	return factorysessionexecution.LifecycleControlResult{
		SessionID: sessionID,
		Operation: factorysessionexecution.LifecycleControlInterruptDispatch,
		Outcome:   factorysessionexecution.LifecycleControlOutcomeAccepted,
		Status:    factorysessionexecution.LifecycleStatusRunning,
	}, nil
}

func (s *stubDurableExecution) GetResult(context.Context, string, factorysessionexecution.ResultRequest) (factorysessionexecution.ResultReadResult, error) {
	return factorysessionexecution.ResultReadResult{}, errors.New("not implemented")
}

func (s *stubDurableExecution) ListDispatches(context.Context, string) (factorysessionexecution.ListDispatchesResult, error) {
	return factorysessionexecution.ListDispatchesResult{}, errors.New("not implemented")
}

func (s *stubDurableExecution) GetDispatch(context.Context, string, string) (factorysessionexecution.DispatchDetail, error) {
	return factorysessionexecution.DispatchDetail{}, errors.New("not implemented")
}

func (s *stubDurableExecution) ListArtifacts(context.Context, string) (factorysessionexecution.ListArtifactsResult, error) {
	return factorysessionexecution.ListArtifactsResult{}, errors.New("not implemented")
}

func (s *stubDurableExecution) GetArtifact(context.Context, string, string) (factorysessionexecution.ArtifactDetail, error) {
	return factorysessionexecution.ArtifactDetail{}, errors.New("not implemented")
}

func (s *stubDurableExecution) ReadEvents(context.Context, string, factorysessionexecution.EventReconnectRequest) (factorysessionexecution.EventReadResult, error) {
	return factorysessionexecution.EventReadResult{}, errors.New("not implemented")
}

func (s *stubDurableExecution) ListSessions(context.Context, factorysessionexecution.ListSessionsRequest) (factorysessionexecution.ListSessionsResult, error) {
	return factorysessionexecution.ListSessionsResult{}, errors.New("not implemented")
}

func TestService_PauseDurableFactorySession_DelegatesToControlPlane(t *testing.T) {
	t.Parallel()

	execution := &stubDurableExecution{}
	gateway := factorysessionservice.New(&durableLifecycleGatewayHost{execution: execution})

	response, err := gateway.PauseDurableFactorySession(
		context.Background(),
		"dur-sess-js-run-n-001",
		factorysessionexecution.ControlRequest{},
	)
	if err != nil {
		t.Fatalf("PauseDurableFactorySession: %v", err)
	}
	if response.Outcome != factorysessionexecution.LifecycleControlOutcomeAccepted {
		t.Fatalf("outcome = %q, want ACCEPTED", response.Outcome)
	}
	if response.Status != factorysessionexecution.LifecycleStatusPaused {
		t.Fatalf("status = %q, want PAUSED", response.Status)
	}
}

func TestService_PauseDurableFactorySession_RejectsLiveSessionID(t *testing.T) {
	t.Parallel()

	gateway := factorysessionservice.New(&durableLifecycleGatewayHost{execution: &stubDurableExecution{}})

	_, err := gateway.PauseDurableFactorySession(
		context.Background(),
		"live-session-001",
		factorysessionexecution.ControlRequest{},
	)
	if err == nil || !errors.Is(err, controlplane.ErrDurableSessionLifecycleRouting) {
		t.Fatalf("PauseDurableFactorySession error = %v, want durable routing error", err)
	}
}

func TestService_CancelDurableFactorySession_DelegatesToControlPlane(t *testing.T) {
	t.Parallel()

	execution := &stubDurableExecution{}
	gateway := factorysessionservice.New(&durableLifecycleGatewayHost{execution: execution})

	response, err := gateway.CancelDurableFactorySession(
		context.Background(),
		"dur-sess-js-run-n-001",
		factorysessionexecution.ControlRequest{},
	)
	if err != nil {
		t.Fatalf("CancelDurableFactorySession: %v", err)
	}
	if execution.cancelCalls != 1 {
		t.Fatalf("cancel calls = %d, want 1", execution.cancelCalls)
	}
	if response.Status != factorysessionexecution.LifecycleStatusCanceling {
		t.Fatalf("status = %q, want CANCELING", response.Status)
	}
}

func TestService_ResumeDurableFactorySession_DelegatesToControlPlane(t *testing.T) {
	t.Parallel()

	execution := &stubDurableExecution{}
	execution.pauseResult = factorysessionexecution.LifecycleControlResult{
		Operation: factorysessionexecution.LifecycleControlResume,
		Outcome:   factorysessionexecution.LifecycleControlOutcomeAccepted,
		Status:    factorysessionexecution.LifecycleStatusRunning,
	}
	gateway := factorysessionservice.New(&durableLifecycleGatewayHost{execution: execution})

	response, err := gateway.ResumeDurableFactorySession(
		context.Background(),
		"dur-sess-js-run-n-001",
		factorysessionexecution.ControlRequest{},
	)
	if err != nil {
		t.Fatalf("ResumeDurableFactorySession: %v", err)
	}
	if response.Status != factorysessionexecution.LifecycleStatusRunning {
		t.Fatalf("status = %q, want RUNNING", response.Status)
	}
}

func TestService_TerminateDurableFactorySession_DelegatesToControlPlane(t *testing.T) {
	t.Parallel()

	execution := &stubDurableExecution{}
	gateway := factorysessionservice.New(&durableLifecycleGatewayHost{execution: execution})

	response, err := gateway.TerminateDurableFactorySession(
		context.Background(),
		"dur-sess-js-run-n-001",
		factorysessionexecution.ControlRequest{},
	)
	if err != nil {
		t.Fatalf("TerminateDurableFactorySession: %v", err)
	}
	if response.Outcome != factorysessionexecution.LifecycleControlOutcomeAccepted {
		t.Fatalf("outcome = %q, want ACCEPTED", response.Outcome)
	}
}

func TestService_ApproveDurableFactorySession_DelegatesToControlPlane(t *testing.T) {
	t.Parallel()

	execution := &stubDurableExecution{}
	gateway := factorysessionservice.New(&durableLifecycleGatewayHost{execution: execution})

	response, err := gateway.ApproveDurableFactorySession(
		context.Background(),
		"dur-sess-js-run-n-001",
		factorysessionexecution.ApproveRequest{},
	)
	if err != nil {
		t.Fatalf("ApproveDurableFactorySession: %v", err)
	}
	if response.Outcome != factorysessionexecution.LifecycleControlOutcomeAccepted {
		t.Fatalf("outcome = %q, want ACCEPTED", response.Outcome)
	}
}

func TestService_RetryDurableFactorySessionDispatch_DelegatesToControlPlane(t *testing.T) {
	t.Parallel()

	execution := &stubDurableExecution{}
	gateway := factorysessionservice.New(&durableLifecycleGatewayHost{execution: execution})

	response, err := gateway.RetryDurableFactorySessionDispatch(
		context.Background(),
		"dur-sess-js-run-n-001",
		factorysessionexecution.RetryDispatchRequest{DispatchID: "dispatch-1"},
	)
	if err != nil {
		t.Fatalf("RetryDurableFactorySessionDispatch: %v", err)
	}
	if response.Outcome != factorysessionexecution.LifecycleControlOutcomeAccepted {
		t.Fatalf("outcome = %q, want ACCEPTED", response.Outcome)
	}
}

func TestService_InterruptDurableFactorySessionDispatch_DelegatesToControlPlane(t *testing.T) {
	t.Parallel()

	execution := &stubDurableExecution{}
	gateway := factorysessionservice.New(&durableLifecycleGatewayHost{execution: execution})

	response, err := gateway.InterruptDurableFactorySessionDispatch(
		context.Background(),
		"dur-sess-js-run-n-001",
		factorysessionexecution.InterruptDispatchRequest{DispatchID: "dispatch-1"},
	)
	if err != nil {
		t.Fatalf("InterruptDurableFactorySessionDispatch: %v", err)
	}
	if response.Outcome != factorysessionexecution.LifecycleControlOutcomeAccepted {
		t.Fatalf("outcome = %q, want ACCEPTED", response.Outcome)
	}
}
