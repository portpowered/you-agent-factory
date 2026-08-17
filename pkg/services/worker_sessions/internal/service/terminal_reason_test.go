package service_test

import (
	"context"
	"testing"
	"time"

	platformclock "github.com/portpowered/infinite-you/pkg/platform/clock"
	"github.com/portpowered/infinite-you/pkg/platform/logging"
	workersessions "github.com/portpowered/infinite-you/pkg/services/worker_sessions"
	"github.com/portpowered/infinite-you/pkg/services/workers"
)

// observeTerminalReason reads the named reason the Worker Session inspection
// surface reports for id. It uses the Worker-Session-identity observation
// route because that is the projection the CLI show/list commands and the
// HTTP observation handlers all render.
func observeTerminalReason(t *testing.T, registry workersessions.Service, id string) (workersessions.State, *workersessions.FailureCause) {
	t.Helper()
	observed, err := registry.GetObservationByWorkerSessionID(
		context.Background(),
		workersessions.GetObservationByWorkerSessionIDRequest{WorkerSessionID: id},
	)
	if err != nil {
		t.Fatalf("GetObservationByWorkerSessionID(%q) error = %v", id, err)
	}
	if err := observed.Validate(); err != nil {
		t.Fatalf("observation for %q is not a valid projection: %v", id, err)
	}
	return observed.State, observed.Failure
}

func assertNamedTerminalReason(
	t *testing.T,
	registry workersessions.Service,
	id string,
	wantState workersessions.State,
	wantKind workersessions.FailureCauseKind,
	wantDetail string,
) {
	t.Helper()
	state, failure := observeTerminalReason(t, registry, id)
	if state != wantState {
		t.Fatalf("observed state = %q, want %q", state, wantState)
	}
	if failure == nil {
		t.Fatalf("observed reason for a %s session = nil, want the named reason %q", wantState, wantKind)
	}
	if failure.Kind != wantKind {
		t.Fatalf("observed reason kind = %q, want %q", failure.Kind, wantKind)
	}
	if failure.Detail != wantDetail {
		t.Fatalf("observed reason detail = %q, want the fixed safe detail %q", failure.Detail, wantDetail)
	}
}

// TestObservation_OperatorCancelReportsANamedTerminalReason covers the reason
// an operator had no way to read before: a canceled Worker Session carries no
// TerminalResult, so the inspection surface reported "unavailable" — the same
// thing it reports for a failure whose cause was never recorded.
func TestObservation_OperatorCancelReportsANamedTerminalReason(t *testing.T) {
	boundary := newControlledBoundary()
	registry := newControlledRegistry(t, boundary)
	started := startControlledSession(t, registry, boundary, "worker-canceled", "dispatch-canceled")

	result, err := registry.Cancel(context.Background(), workersessions.ControlRequest{ID: "worker-canceled"})
	if err != nil || result.Outcome != workersessions.ControlOutcomeApplied {
		t.Fatalf("Cancel() = %#v, %v, want APPLIED", result, err)
	}
	if got := <-started; got.Session.State != workersessions.StateCanceled {
		t.Fatalf("InvokeSession() after cancel = %#v, want CANCELED", got.Session)
	}

	assertNamedTerminalReason(
		t, registry, "worker-canceled",
		workersessions.StateCanceled,
		workersessions.FailureCauseOperatorCanceled,
		"an operator cancel control ended the Worker Session",
	)
}

func TestObservation_OperatorTerminateReportsANamedTerminalReason(t *testing.T) {
	boundary := newControlledBoundary()
	registry := newControlledRegistry(t, boundary)
	started := startControlledSession(t, registry, boundary, "worker-terminated", "dispatch-terminated")

	result, err := registry.Terminate(context.Background(), workersessions.ControlRequest{ID: "worker-terminated"})
	if err != nil || result.Outcome != workersessions.ControlOutcomeApplied {
		t.Fatalf("Terminate() = %#v, %v, want APPLIED", result, err)
	}
	if got := <-started; got.Session.State != workersessions.StateTerminated {
		t.Fatalf("InvokeSession() after terminate = %#v, want TERMINATED", got.Session)
	}

	assertNamedTerminalReason(
		t, registry, "worker-terminated",
		workersessions.StateTerminated,
		workersessions.FailureCauseOperatorTerminated,
		"an operator terminate control ended the Worker Session",
	)
}

func TestObservation_ProcessGoneReportsANamedTerminalReason(t *testing.T) {
	execution := &fakeExecution{
		dispatch: func(_ context.Context, request workers.WorkstationDispatchRequest) (workers.WorkstationDispatchResult, error) {
			return processGoneResult(request), workers.ErrWorkstationDispatchProcessGone
		},
	}
	registry := newRegistryWithExecution(execution)

	if _, err := registry.InvokeSession(context.Background(), validStartRequest("worker-gone", "dispatch-gone")); err != nil {
		t.Fatalf("InvokeSession() error = %v", err)
	}

	assertNamedTerminalReason(
		t, registry, "worker-gone",
		workersessions.StateFailed,
		workersessions.FailureCauseProcessGone,
		"the worker process exited before dispatch completion",
	)
}

func TestObservation_ExecutionTimeoutReportsANamedTerminalReason(t *testing.T) {
	base := time.Date(2035, time.March, 4, 5, 6, 7, 0, time.UTC)
	clock := platformclock.NewDeterministic(base, time.Second)
	boundary := newControlledBoundary()
	registry, err := newServiceWithClock(boundary, newEventsAppender(), logging.NoopLogger{}, clock)
	if err != nil {
		t.Fatalf("service.New() error = %v", err)
	}
	request := validStartRequest("worker-timeout", "dispatch-timeout")
	request.Execution.Execution.Timeout = 5 * time.Second

	invoked := make(chan error, 1)
	go func() {
		_, invokeErr := registry.InvokeSession(context.Background(), request)
		invoked <- invokeErr
	}()
	select {
	case <-boundary.admitted:
	case <-time.After(controlledBoundaryWaitTimeout):
		t.Fatal("Worker Session did not reach the admitted deadline-watch state")
	}
	clock.SetTick(5)
	select {
	case invokeErr := <-invoked:
		if invokeErr != nil {
			t.Fatalf("InvokeSession() error = %v", invokeErr)
		}
	case <-time.After(controlledBoundaryWaitTimeout):
		t.Fatal("deadline reconciliation did not terminalize the Worker Session")
	}

	assertNamedTerminalReason(
		t, registry, "worker-timeout",
		workersessions.StateFailed,
		workersessions.FailureCauseTimeout,
		"the worker execution exceeded its hard deadline",
	)
}

// TestObservation_TerminalReasonsAreMutuallyDistinct is the guard the story
// exists for: an operator must be able to tell the three endings apart from
// the inspection surface alone, without re-deriving the cause from process
// forensics.
func TestObservation_TerminalReasonsAreMutuallyDistinct(t *testing.T) {
	kinds := []workersessions.FailureCauseKind{
		workersessions.FailureCauseOperatorCanceled,
		workersessions.FailureCauseOperatorTerminated,
		workersessions.FailureCauseProcessGone,
		workersessions.FailureCauseTimeout,
	}
	seen := make(map[workersessions.FailureCauseKind]bool, len(kinds))
	for _, kind := range kinds {
		if seen[kind] {
			t.Fatalf("terminal reason %q is not distinct from an earlier reason", kind)
		}
		if !kind.Valid() {
			t.Fatalf("terminal reason %q is not an accepted FailureCauseKind", kind)
		}
		seen[kind] = true
	}
}

// TestObservation_LiveSessionReportsNoTerminalReason keeps the projection from
// naming a reason for a session that has not ended.
func TestObservation_LiveSessionReportsNoTerminalReason(t *testing.T) {
	boundary := newControlledBoundary()
	registry := newControlledRegistry(t, boundary)
	started := startControlledSession(t, registry, boundary, "worker-live", "dispatch-live")
	t.Cleanup(func() {
		_, _ = registry.Cancel(context.Background(), workersessions.ControlRequest{ID: "worker-live"})
		<-started
	})

	state, failure := observeTerminalReason(t, registry, "worker-live")
	if state != workersessions.StateRunning {
		t.Fatalf("observed state = %q, want RUNNING", state)
	}
	if failure != nil {
		t.Fatalf("observed reason for a RUNNING session = %#v, want none", failure)
	}
}
