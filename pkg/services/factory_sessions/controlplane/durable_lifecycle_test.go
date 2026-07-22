package controlplane_test

import (
	"context"
	"errors"
	"testing"

	"github.com/portpowered/infinite-you/pkg/services/factory_sessions/controlplane"
	factorysessionexecution "github.com/portpowered/infinite-you/pkg/services/factory_sessions/execution"
	factoryapi "github.com/portpowered/infinite-you/pkg/transports/http/generated"
)

type durableLifecycleTestHost struct {
	service   factorysessionexecution.Service
	operation string
}

func (h *durableLifecycleTestHost) DurableExecution() factorysessionexecution.Service {
	return h.service
}

type recordingDurableExecution struct {
	lastOperation string
	lastSessionID string
	result        factorysessionexecution.LifecycleControlResult
	err           error
}

func (r *recordingDurableExecution) StartAsync(context.Context, factorysessionexecution.StartRequest) (factorysessionexecution.AsyncStartResult, error) {
	return factorysessionexecution.AsyncStartResult{}, errors.New("not implemented")
}

func (r *recordingDurableExecution) StartSync(context.Context, factorysessionexecution.StartRequest) (factorysessionexecution.SyncStartResult, error) {
	return factorysessionexecution.SyncStartResult{}, errors.New("not implemented")
}

func (r *recordingDurableExecution) ResumeInterruptedSession(context.Context, string, factorysessionexecution.ResumeSessionRequest) (factorysessionexecution.AsyncStartResult, error) {
	return factorysessionexecution.AsyncStartResult{}, errors.New("not implemented")
}

func (r *recordingDurableExecution) GetSession(context.Context, string) (factorysessionexecution.SessionReadResult, error) {
	return factorysessionexecution.SessionReadResult{}, errors.New("not implemented")
}

func (r *recordingDurableExecution) Pause(ctx context.Context, sessionID string, _ factorysessionexecution.ControlRequest) (factorysessionexecution.LifecycleControlResult, error) {
	r.lastOperation = "pause"
	r.lastSessionID = sessionID
	return r.result, r.err
}

func (r *recordingDurableExecution) Resume(ctx context.Context, sessionID string, _ factorysessionexecution.ControlRequest) (factorysessionexecution.LifecycleControlResult, error) {
	r.lastOperation = "resume"
	r.lastSessionID = sessionID
	return r.result, r.err
}

func (r *recordingDurableExecution) Cancel(ctx context.Context, sessionID string, _ factorysessionexecution.ControlRequest) (factorysessionexecution.LifecycleControlResult, error) {
	r.lastOperation = "cancel"
	r.lastSessionID = sessionID
	return r.result, r.err
}

func (r *recordingDurableExecution) Terminate(ctx context.Context, sessionID string, _ factorysessionexecution.ControlRequest) (factorysessionexecution.LifecycleControlResult, error) {
	r.lastOperation = "terminate"
	r.lastSessionID = sessionID
	return r.result, r.err
}

func (r *recordingDurableExecution) Approve(ctx context.Context, sessionID string, _ factorysessionexecution.ApproveRequest) (factorysessionexecution.LifecycleControlResult, error) {
	r.lastOperation = "approve"
	r.lastSessionID = sessionID
	return r.result, r.err
}

func (r *recordingDurableExecution) RetryDispatch(ctx context.Context, sessionID string, _ factorysessionexecution.RetryDispatchRequest) (factorysessionexecution.LifecycleControlResult, error) {
	r.lastOperation = "retry-dispatch"
	r.lastSessionID = sessionID
	return r.result, r.err
}

func (r *recordingDurableExecution) InterruptDispatch(ctx context.Context, sessionID string, _ factorysessionexecution.InterruptDispatchRequest) (factorysessionexecution.LifecycleControlResult, error) {
	r.lastOperation = "interrupt-dispatch"
	r.lastSessionID = sessionID
	return r.result, r.err
}

func (r *recordingDurableExecution) GetResult(context.Context, string, factorysessionexecution.ResultRequest) (factorysessionexecution.ResultReadResult, error) {
	return factorysessionexecution.ResultReadResult{}, errors.New("not implemented")
}

func (r *recordingDurableExecution) ListDispatches(context.Context, string) (factorysessionexecution.ListDispatchesResult, error) {
	return factorysessionexecution.ListDispatchesResult{}, errors.New("not implemented")
}

func (r *recordingDurableExecution) QueryDispatches(ctx context.Context, request factorysessionexecution.DispatchQueryRequest) (factorysessionexecution.ListDispatchesResult, error) {
	result, err := r.ListDispatches(ctx, request.SessionID)
	if err != nil {
		return factorysessionexecution.ListDispatchesResult{}, err
	}
	return factorysessionexecution.FilterDispatches(result, request.Filters)
}

func (r *recordingDurableExecution) GetDispatch(context.Context, string, string) (factorysessionexecution.DispatchDetail, error) {
	return factorysessionexecution.DispatchDetail{}, errors.New("not implemented")
}

func (r *recordingDurableExecution) ListArtifacts(context.Context, string) (factorysessionexecution.ListArtifactsResult, error) {
	return factorysessionexecution.ListArtifactsResult{}, errors.New("not implemented")
}

func (r *recordingDurableExecution) GetArtifact(context.Context, string, string) (factorysessionexecution.ArtifactDetail, error) {
	return factorysessionexecution.ArtifactDetail{}, errors.New("not implemented")
}

func (r *recordingDurableExecution) ReadEvents(context.Context, string, factorysessionexecution.EventReconnectRequest) (factorysessionexecution.EventReadResult, error) {
	return factorysessionexecution.EventReadResult{}, errors.New("not implemented")
}

func (r *recordingDurableExecution) ListSessions(context.Context, factorysessionexecution.ListSessionsRequest) (factorysessionexecution.ListSessionsResult, error) {
	return factorysessionexecution.ListSessionsResult{}, errors.New("not implemented")
}

func TestPauseDurableFactorySession_RoutesDurableSessionToExecution(t *testing.T) {
	t.Parallel()

	recorder := &recordingDurableExecution{
		result: factorysessionexecution.LifecycleControlResult{
			SessionID: "dur-sess-js-run-n-001",
			Operation: factorysessionexecution.LifecycleControlPause,
			Outcome:   factorysessionexecution.LifecycleControlOutcomeAccepted,
			Status:    factorysessionexecution.LifecycleStatusPaused,
		},
	}
	host := &durableLifecycleTestHost{service: recorder}

	result, err := controlplane.PauseDurableFactorySession(
		context.Background(),
		host,
		"dur-sess-js-run-n-001",
		factorysessionexecution.ControlRequest{},
	)
	if err != nil {
		t.Fatalf("PauseDurableFactorySession: %v", err)
	}
	if recorder.lastOperation != "pause" || recorder.lastSessionID != "dur-sess-js-run-n-001" {
		t.Fatalf("execution calls = %q %q, want pause dur-sess-js-run-n-001", recorder.lastOperation, recorder.lastSessionID)
	}
	if result.Status != factorysessionexecution.LifecycleStatusPaused {
		t.Fatalf("status = %q, want PAUSED", result.Status)
	}
}

func TestPauseDurableFactorySession_RejectsLiveSessionID(t *testing.T) {
	t.Parallel()

	host := &durableLifecycleTestHost{service: &recordingDurableExecution{}}

	_, err := controlplane.PauseDurableFactorySession(
		context.Background(),
		host,
		"live-session-001",
		factorysessionexecution.ControlRequest{},
	)
	if err == nil || !errors.Is(err, controlplane.ErrDurableSessionLifecycleRouting) {
		t.Fatalf("PauseDurableFactorySession error = %v, want durable routing error", err)
	}
}

func TestCancelDurableFactorySession_RoutesDurableSessionToExecution(t *testing.T) {
	t.Parallel()

	recorder := &recordingDurableExecution{
		result: factorysessionexecution.LifecycleControlResult{
			SessionID: "dur-sess-js-run-n-001",
			Operation: factorysessionexecution.LifecycleControlCancel,
			Outcome:   factorysessionexecution.LifecycleControlOutcomeAccepted,
			Status:    factorysessionexecution.LifecycleStatusCanceling,
		},
	}
	host := &durableLifecycleTestHost{service: recorder}

	result, err := controlplane.CancelDurableFactorySession(
		context.Background(),
		host,
		"dur-sess-js-run-n-001",
		factorysessionexecution.ControlRequest{},
	)
	if err != nil {
		t.Fatalf("CancelDurableFactorySession: %v", err)
	}
	if recorder.lastOperation != "cancel" {
		t.Fatalf("execution operation = %q, want cancel", recorder.lastOperation)
	}
	if result.Status != factorysessionexecution.LifecycleStatusCanceling {
		t.Fatalf("status = %q, want CANCELING", result.Status)
	}
}

func TestApproveDurableFactorySession_RejectsLiveSessionID(t *testing.T) {
	t.Parallel()

	host := &durableLifecycleTestHost{service: &recordingDurableExecution{}}

	_, err := controlplane.ApproveDurableFactorySession(
		context.Background(),
		host,
		"~default",
		factorysessionexecution.ApproveRequest{},
	)
	if err == nil || !errors.Is(err, controlplane.ErrDurableSessionLifecycleRouting) {
		t.Fatalf("ApproveDurableFactorySession error = %v, want durable routing error", err)
	}
}

func TestRetryDurableFactorySessionDispatch_PreservesDistinctFromLiveLifecycle(t *testing.T) {
	t.Parallel()

	recorder := &recordingDurableExecution{
		result: factorysessionexecution.LifecycleControlResult{
			SessionID: "dur-sess-js-run-n-001",
			Operation: factorysessionexecution.LifecycleControlRetryDispatch,
			Outcome:   factorysessionexecution.LifecycleControlOutcomeAccepted,
			Status:    factorysessionexecution.LifecycleStatusRunning,
		},
	}
	host := &durableLifecycleTestHost{service: recorder}

	response, err := controlplane.RetryDurableFactorySessionDispatch(
		context.Background(),
		host,
		"dur-sess-js-run-n-001",
		factorysessionexecution.RetryDispatchRequest{DispatchID: "dispatch-1"},
	)
	if err != nil {
		t.Fatalf("RetryDurableFactorySessionDispatch: %v", err)
	}
	if recorder.lastOperation != "retry-dispatch" {
		t.Fatalf("execution operation = %q, want retry-dispatch", recorder.lastOperation)
	}
	if response.Operation != factorysessionexecution.LifecycleControlRetryDispatch {
		t.Fatalf("operation = %q, want RETRY_DISPATCH", response.Operation)
	}
	if factoryapi.FactorySessionDurableLifecycleStatus(response.Status) != factoryapi.FactorySessionDurableLifecycleStatusRunning {
		t.Fatalf("status = %q, want RUNNING", response.Status)
	}
}

func TestResumeDurableFactorySession_RoutesDurableSessionToExecution(t *testing.T) {
	t.Parallel()

	recorder := &recordingDurableExecution{
		result: factorysessionexecution.LifecycleControlResult{
			SessionID: "dur-sess-js-run-n-001",
			Operation: factorysessionexecution.LifecycleControlResume,
			Outcome:   factorysessionexecution.LifecycleControlOutcomeAccepted,
			Status:    factorysessionexecution.LifecycleStatusRunning,
		},
	}
	host := &durableLifecycleTestHost{service: recorder}

	result, err := controlplane.ResumeDurableFactorySession(
		context.Background(),
		host,
		"dur-sess-js-run-n-001",
		factorysessionexecution.ControlRequest{},
	)
	if err != nil {
		t.Fatalf("ResumeDurableFactorySession: %v", err)
	}
	if recorder.lastOperation != "resume" {
		t.Fatalf("execution operation = %q, want resume", recorder.lastOperation)
	}
	if result.Status != factorysessionexecution.LifecycleStatusRunning {
		t.Fatalf("status = %q, want RUNNING", result.Status)
	}
}

func TestTerminateDurableFactorySession_RoutesDurableSessionToExecution(t *testing.T) {
	t.Parallel()

	recorder := &recordingDurableExecution{
		result: factorysessionexecution.LifecycleControlResult{
			SessionID: "dur-sess-js-run-n-001",
			Operation: factorysessionexecution.LifecycleControlTerminate,
			Outcome:   factorysessionexecution.LifecycleControlOutcomeAccepted,
			Status:    factorysessionexecution.LifecycleStatusTerminated,
		},
	}
	host := &durableLifecycleTestHost{service: recorder}

	result, err := controlplane.TerminateDurableFactorySession(
		context.Background(),
		host,
		"dur-sess-js-run-n-001",
		factorysessionexecution.ControlRequest{},
	)
	if err != nil {
		t.Fatalf("TerminateDurableFactorySession: %v", err)
	}
	if recorder.lastOperation != "terminate" {
		t.Fatalf("execution operation = %q, want terminate", recorder.lastOperation)
	}
	if result.Status != factorysessionexecution.LifecycleStatusTerminated {
		t.Fatalf("status = %q, want TERMINATED", result.Status)
	}
}

func TestApproveDurableFactorySession_RoutesDurableSessionToExecution(t *testing.T) {
	t.Parallel()

	recorder := &recordingDurableExecution{
		result: factorysessionexecution.LifecycleControlResult{
			SessionID: "dur-sess-js-run-n-001",
			Operation: factorysessionexecution.LifecycleControlApprove,
			Outcome:   factorysessionexecution.LifecycleControlOutcomeAccepted,
			Status:    factorysessionexecution.LifecycleStatusRunning,
		},
	}
	host := &durableLifecycleTestHost{service: recorder}

	result, err := controlplane.ApproveDurableFactorySession(
		context.Background(),
		host,
		"dur-sess-js-run-n-001",
		factorysessionexecution.ApproveRequest{},
	)
	if err != nil {
		t.Fatalf("ApproveDurableFactorySession: %v", err)
	}
	if recorder.lastOperation != "approve" {
		t.Fatalf("execution operation = %q, want approve", recorder.lastOperation)
	}
	if result.Operation != factorysessionexecution.LifecycleControlApprove {
		t.Fatalf("operation = %q, want APPROVE", result.Operation)
	}
}

func TestInterruptDurableFactorySessionDispatch_RoutesDurableSessionToExecution(t *testing.T) {
	t.Parallel()

	recorder := &recordingDurableExecution{
		result: factorysessionexecution.LifecycleControlResult{
			SessionID: "dur-sess-js-run-n-001",
			Operation: factorysessionexecution.LifecycleControlInterruptDispatch,
			Outcome:   factorysessionexecution.LifecycleControlOutcomeAccepted,
			Status:    factorysessionexecution.LifecycleStatusRunning,
		},
	}
	host := &durableLifecycleTestHost{service: recorder}

	result, err := controlplane.InterruptDurableFactorySessionDispatch(
		context.Background(),
		host,
		"dur-sess-js-run-n-001",
		factorysessionexecution.InterruptDispatchRequest{DispatchID: "dispatch-1"},
	)
	if err != nil {
		t.Fatalf("InterruptDurableFactorySessionDispatch: %v", err)
	}
	if recorder.lastOperation != "interrupt-dispatch" {
		t.Fatalf("execution operation = %q, want interrupt-dispatch", recorder.lastOperation)
	}
	if result.Operation != factorysessionexecution.LifecycleControlInterruptDispatch {
		t.Fatalf("operation = %q, want INTERRUPT_DISPATCH", result.Operation)
	}
}
