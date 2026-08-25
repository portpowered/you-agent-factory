package service

import (
	"context"
	"errors"
	"strings"
	"testing"

	workersessions "github.com/portpowered/infinite-you/pkg/services/worker_sessions"
	"github.com/portpowered/infinite-you/pkg/services/workers"
)

func TestCancel_QueuedPauseBeforeAdmissionTerminalizesWithoutWorkersHandoff(t *testing.T) {
	registry, supervision := newRunningPauseRegistry(t)
	supervision.mu.Lock()
	supervision.accepted = false
	supervision.publishing = false
	supervision.preAdmissionAction = workersessions.ControlActionPause
	supervision.mu.Unlock()

	result, err := registry.Cancel(context.Background(), workersessions.ControlRequest{ID: "worker-1"})
	if err != nil || result.Outcome != workersessions.ControlOutcomeApplied || result.Session.State != workersessions.StateCanceled {
		t.Fatalf("Cancel() = %#v, %v, want applied CANCELED result", result, err)
	}
	final, err := registry.Get(context.Background(), workersessions.GetRequest{ID: "worker-1"})
	if err != nil || final.State != workersessions.StateCanceled {
		t.Fatalf("Get() = %#v, %v, want absorbing CANCELED session", final, err)
	}
}

func TestStop_CollectsTerminationAndDriverWaitFailures(t *testing.T) {
	registry, supervision := newRunningPauseRegistry(t)
	supervision.serverOwned = true
	supervision.driverDone = make(chan struct{})
	boundaryErr := errors.New("shutdown cancellation failed")
	supervision.installCancelFailure(func() error { return boundaryErr })
	registry.startsDone = make(chan struct{})

	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if err := registry.Stop(ctx); !errors.Is(err, boundaryErr) || !errors.Is(err, context.Canceled) {
		t.Fatalf("Stop() error = %v, want both cancellation and driver-wait failures", err)
	}
}

func TestBeginRuntimeAttempt_ContradictoryAcceptedResultWithDispatchErrorIsAdapterFailure(t *testing.T) {
	registry := newTestRegistry(t)
	attempt, err := registry.BeginRuntimeAttempt(context.Background(), workersessions.RuntimeAttemptRequest{
		ID:        "worker-adapter-error",
		AttemptID: "attempt-adapter-error",
		Execution: dispatchHandoff("dispatch-adapter-error"),
	})
	if err != nil {
		t.Fatalf("BeginRuntimeAttempt() error = %v, want nil", err)
	}

	result := runtimeAttemptCompletedDispatch("dispatch-adapter-error")
	result.TerminalOutcome = workers.WorkstationDispatchTerminalOutcomeFailed
	if err := attempt.Complete(context.Background(), result, errors.New("adapter failed after successful result")); err != nil {
		t.Fatalf("Complete() error = %v, want nil", err)
	}

	session, err := registry.Get(context.Background(), workersessions.GetRequest{ID: "worker-adapter-error"})
	if err != nil {
		t.Fatalf("Get() error = %v, want nil", err)
	}
	if session.State != workersessions.StateFailed || session.Result == nil || session.Result.Cause == nil {
		t.Fatalf("session = %#v, want FAILED with a cause", session)
	}
	if session.Result.Cause.Kind != workersessions.FailureCauseAdapterFailure {
		t.Fatalf("failure cause kind = %q, want ADAPTER_FAILURE", session.Result.Cause.Kind)
	}
	if !strings.HasPrefix(session.Result.Cause.Detail, "the Workers adapter reported failure after a successful result") {
		t.Fatalf("failure cause detail = %q, want adapter contradiction detail", session.Result.Cause.Detail)
	}
}
