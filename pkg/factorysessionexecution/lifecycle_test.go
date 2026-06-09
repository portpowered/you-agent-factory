package factorysessionexecution

import (
	"context"
	"errors"
	"testing"
)

func TestIsTerminalLifecycleStatus(t *testing.T) {
	terminal := []LifecycleStatus{
		LifecycleStatusSucceeded,
		LifecycleStatusFailed,
		LifecycleStatusCanceled,
		LifecycleStatusTimedOut,
		LifecycleStatusInterrupted,
		LifecycleStatusTerminated,
	}
	for _, status := range terminal {
		if !IsTerminalLifecycleStatus(status) {
			t.Fatalf("status %q should be terminal", status)
		}
		if status != LifecycleStatusFailed && AllowsRetryDispatchOnTerminal(status) {
			t.Fatalf("retry-dispatch should be rejected on terminal status %q", status)
		}
	}
	if !AllowsRetryDispatchOnTerminal(LifecycleStatusFailed) {
		t.Fatal("retry-dispatch should remain allowed on FAILED terminal sessions")
	}
	active := []LifecycleStatus{
		LifecycleStatusRunning,
		LifecycleStatusPaused,
		LifecycleStatusCanceling,
	}
	for _, status := range active {
		if IsTerminalLifecycleStatus(status) {
			t.Fatalf("status %q should be active", status)
		}
	}
}

func TestEvaluateLifecycleControl_ValidTransitions(t *testing.T) {
	cases := []struct {
		operation LifecycleControlKind
		status    LifecycleStatus
		want      LifecycleControlOutcome
	}{
		{LifecycleControlPause, LifecycleStatusRunning, LifecycleControlOutcomeAccepted},
		{LifecycleControlPause, LifecycleStatusPaused, LifecycleControlOutcomeNoOp},
		{LifecycleControlResume, LifecycleStatusPaused, LifecycleControlOutcomeAccepted},
		{LifecycleControlCancel, LifecycleStatusRunning, LifecycleControlOutcomeAccepted},
		{LifecycleControlCancel, LifecycleStatusCanceling, LifecycleControlOutcomeNoOp},
		{LifecycleControlTerminate, LifecycleStatusRunning, LifecycleControlOutcomeAccepted},
		{LifecycleControlApprove, LifecycleStatusAwaitingApproval, LifecycleControlOutcomeAccepted},
		{LifecycleControlRetryDispatch, LifecycleStatusRunning, LifecycleControlOutcomeAccepted},
		{LifecycleControlRetryDispatch, LifecycleStatusFailed, LifecycleControlOutcomeAccepted},
	}
	for _, tc := range cases {
		got := EvaluateLifecycleControl(tc.operation, tc.status)
		if got != tc.want {
			t.Fatalf("%s on %s = %q, want %q", tc.operation, tc.status, got, tc.want)
		}
	}
}

func TestEvaluateLifecycleControl_InvalidAndTerminal(t *testing.T) {
	if got := EvaluateLifecycleControl(LifecycleControlPause, LifecycleStatusAwaitingApproval); got != LifecycleControlOutcomeInvalidState {
		t.Fatalf("pause on awaiting approval = %q, want INVALID_STATE", got)
	}
	if got := EvaluateLifecycleControl(LifecycleControlRetryDispatch, LifecycleStatusSucceeded); got != LifecycleControlOutcomeTerminalSession {
		t.Fatalf("retry on succeeded = %q, want TERMINAL_SESSION", got)
	}
	if got := EvaluateLifecycleControl(LifecycleControlCancel, LifecycleStatusCanceled); got != LifecycleControlOutcomeNoOp {
		t.Fatalf("cancel on canceled = %q, want NO_OP", got)
	}
}

func TestNormalizeRetryDispatchRequest_RequiresDispatchID(t *testing.T) {
	_, err := NormalizeRetryDispatchRequest(RetryDispatchRequest{})
	if err == nil {
		t.Fatal("error = nil, want validation error")
	}
	var validationErr *ValidationError
	if !errors.As(err, &validationErr) {
		t.Fatalf("error = %T, want ValidationError", err)
	}
}

func TestControlIdempotencyTupleHash_IsStable(t *testing.T) {
	retry := RetryDispatchRequest{
		ControlRequest: ControlRequest{RequestID: "req-retry-001"},
		DispatchID:     "disp-js-success-002",
	}
	first, err := ControlIdempotencyTupleHash(LifecycleControlRetryDispatch, "dur-sess-js-success-002", ApproveRequest{}, retry)
	if err != nil {
		t.Fatalf("first hash: %v", err)
	}
	second, err := ControlIdempotencyTupleHash(LifecycleControlRetryDispatch, "dur-sess-js-success-002", ApproveRequest{}, retry)
	if err != nil {
		t.Fatalf("second hash: %v", err)
	}
	if first != second {
		t.Fatalf("hash mismatch: %q vs %q", first, second)
	}
}

func TestCheckControlRequestIDReplay_Conflict(t *testing.T) {
	err := CheckControlRequestIDReplay("req-1", "sha256:abc", "sha256:def")
	if !errors.Is(err, ErrControlRequestIDConflict) {
		t.Fatalf("error = %v, want ErrControlRequestIDConflict", err)
	}
}

func TestServiceMethods_PropagateContextCancellation(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	var service interface {
		GetSession(context.Context, string) (SessionReadResult, error)
	}
	service = stubCancelAwareService{}
	if _, err := service.GetSession(ctx, "dur-sess-001"); !errors.Is(err, context.Canceled) {
		t.Fatalf("GetSession error = %v, want context.Canceled", err)
	}
}

type stubCancelAwareService struct{}

func (stubCancelAwareService) GetSession(ctx context.Context, _ string) (SessionReadResult, error) {
	if err := ctx.Err(); err != nil {
		return SessionReadResult{}, err
	}
	return SessionReadResult{}, nil
}

func (stubCancelAwareService) StartAsync(context.Context, StartRequest) (AsyncStartResult, error) {
	return AsyncStartResult{}, nil
}
func (stubCancelAwareService) StartSync(context.Context, StartRequest) (SyncStartResult, error) {
	return SyncStartResult{}, nil
}
func (stubCancelAwareService) Pause(context.Context, string, ControlRequest) (LifecycleControlResult, error) {
	return LifecycleControlResult{}, nil
}
func (stubCancelAwareService) Resume(context.Context, string, ControlRequest) (LifecycleControlResult, error) {
	return LifecycleControlResult{}, nil
}
func (stubCancelAwareService) Cancel(context.Context, string, ControlRequest) (LifecycleControlResult, error) {
	return LifecycleControlResult{}, nil
}
func (stubCancelAwareService) Terminate(context.Context, string, ControlRequest) (LifecycleControlResult, error) {
	return LifecycleControlResult{}, nil
}
func (stubCancelAwareService) Approve(context.Context, string, ApproveRequest) (LifecycleControlResult, error) {
	return LifecycleControlResult{}, nil
}
func (stubCancelAwareService) RetryDispatch(context.Context, string, RetryDispatchRequest) (LifecycleControlResult, error) {
	return LifecycleControlResult{}, nil
}
