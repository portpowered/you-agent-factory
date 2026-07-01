package service_test

import (
	"context"
	"errors"
	"testing"

	factoryapi "github.com/portpowered/infinite-you/pkg/api/generated"
	"github.com/portpowered/infinite-you/pkg/factorysessionexecution"
	"github.com/portpowered/infinite-you/pkg/factorysessions/controlplane"
	factorysessionservice "github.com/portpowered/infinite-you/pkg/factorysessions/service"
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

func (s *stubDurableExecution) Resume(context.Context, string, factorysessionexecution.ControlRequest) (factorysessionexecution.LifecycleControlResult, error) {
	return factorysessionexecution.LifecycleControlResult{}, errors.New("not implemented")
}

func (s *stubDurableExecution) Cancel(context.Context, string, factorysessionexecution.ControlRequest) (factorysessionexecution.LifecycleControlResult, error) {
	s.cancelCalls++
	return factorysessionexecution.LifecycleControlResult{
		Operation: factorysessionexecution.LifecycleControlCancel,
		Outcome:   factorysessionexecution.LifecycleControlOutcomeAccepted,
		Status:    factorysessionexecution.LifecycleStatusCanceling,
	}, nil
}

func (s *stubDurableExecution) Terminate(context.Context, string, factorysessionexecution.ControlRequest) (factorysessionexecution.LifecycleControlResult, error) {
	return factorysessionexecution.LifecycleControlResult{}, errors.New("not implemented")
}

func (s *stubDurableExecution) Approve(context.Context, string, factorysessionexecution.ApproveRequest) (factorysessionexecution.LifecycleControlResult, error) {
	return factorysessionexecution.LifecycleControlResult{}, errors.New("not implemented")
}

func (s *stubDurableExecution) RetryDispatch(context.Context, string, factorysessionexecution.RetryDispatchRequest) (factorysessionexecution.LifecycleControlResult, error) {
	return factorysessionexecution.LifecycleControlResult{}, errors.New("not implemented")
}

func (s *stubDurableExecution) InterruptDispatch(context.Context, string, factorysessionexecution.InterruptDispatchRequest) (factorysessionexecution.LifecycleControlResult, error) {
	return factorysessionexecution.LifecycleControlResult{}, errors.New("not implemented")
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
		factoryapi.FactorySessionLifecycleControlRequest{},
	)
	if err != nil {
		t.Fatalf("PauseDurableFactorySession: %v", err)
	}
	if response.Outcome != factoryapi.FactorySessionLifecycleControlOutcomeAccepted {
		t.Fatalf("outcome = %q, want ACCEPTED", response.Outcome)
	}
	if response.Status != factoryapi.FactorySessionDurableLifecycleStatusPaused {
		t.Fatalf("status = %q, want PAUSED", response.Status)
	}
}

func TestService_PauseDurableFactorySession_RejectsLiveSessionID(t *testing.T) {
	t.Parallel()

	gateway := factorysessionservice.New(&durableLifecycleGatewayHost{execution: &stubDurableExecution{}})

	_, err := gateway.PauseDurableFactorySession(
		context.Background(),
		"live-session-001",
		factoryapi.FactorySessionLifecycleControlRequest{},
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
		factoryapi.FactorySessionLifecycleControlRequest{},
	)
	if err != nil {
		t.Fatalf("CancelDurableFactorySession: %v", err)
	}
	if execution.cancelCalls != 1 {
		t.Fatalf("cancel calls = %d, want 1", execution.cancelCalls)
	}
	if response.Status != factoryapi.FactorySessionDurableLifecycleStatusCanceling {
		t.Fatalf("status = %q, want CANCELING", response.Status)
	}
}
