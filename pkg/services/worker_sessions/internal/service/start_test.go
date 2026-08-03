package service_test

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"testing"

	workersessions "github.com/portpowered/infinite-you/pkg/services/worker_sessions"
	"github.com/portpowered/infinite-you/pkg/services/worker_sessions/internal/service"
	"github.com/portpowered/infinite-you/pkg/services/workers"
)

func TestNew_RejectsNilExecution(t *testing.T) {
	if _, err := service.New(nil, nil); !errors.Is(err, service.ErrMissingExecution) {
		t.Fatalf("New(nil, nil) error = %v, want ErrMissingExecution", err)
	}
}

func TestStart_InvalidRequest_ReturnsTypedErrorAndMakesNoWorkersCall(t *testing.T) {
	execution := succeedingExecution()
	registry := newRegistryWithExecution(execution)
	ctx := context.Background()

	req := validStartRequest("worker-1", "dispatch-1")
	req.ID = "   "
	if _, err := registry.Start(ctx, req); !errors.Is(err, workersessions.ErrInvalidSessionID) {
		t.Fatalf("Start() error = %v, want ErrInvalidSessionID", err)
	}
	if execution.callCount() != 0 {
		t.Fatalf("Start() with invalid request called Workers %d times, want 0", execution.callCount())
	}

	if _, err := registry.Get(ctx, workersessions.GetRequest{ID: "worker-1"}); !errors.Is(err, workersessions.ErrSessionNotFound) {
		t.Fatalf("Get() after invalid Start() = %v, want ErrSessionNotFound (no registry mutation)", err)
	}
}

func TestStart_ValidNewIdentity_IsReservedBeforeWorkersIsInvoked(t *testing.T) {
	started := make(chan struct{})
	release := make(chan struct{})
	execution := &fakeExecution{
		dispatch: func(context.Context, workers.WorkstationDispatchRequest) (workers.WorkstationDispatchResult, error) {
			close(started)
			<-release
			return workers.WorkstationDispatchResult{
				Result: workers.WorkResult{Outcome: workers.OutcomeAccepted},
			}, nil
		},
	}
	registry := newRegistryWithExecution(execution)
	ctx := context.Background()

	var wg sync.WaitGroup
	wg.Add(1)
	go func() {
		defer wg.Done()
		if _, err := registry.Start(ctx, validStartRequest("worker-1", "dispatch-1")); err != nil {
			t.Errorf("Start() error = %v, want nil", err)
		}
	}()

	<-started
	session, err := registry.Get(ctx, workersessions.GetRequest{ID: "worker-1"})
	if err != nil {
		t.Fatalf("Get() during in-flight Start() error = %v, want nil", err)
	}
	if session.State != workersessions.StateStarting {
		t.Fatalf("Get() during in-flight Start() state = %q, want STARTING", session.State)
	}

	close(release)
	wg.Wait()
}

func TestStart_ReuseAlreadyReservedIdentity_DoesNotCreateAReplacementSession(t *testing.T) {
	execution := succeedingExecution()
	registry := newRegistryWithExecution(execution)
	ctx := context.Background()

	if _, err := registry.Reserve(ctx, workersessions.ReserveRequest{ID: "worker-1"}); err != nil {
		t.Fatalf("Reserve() error = %v, want nil", err)
	}

	result, err := registry.Start(ctx, validStartRequest("worker-1", "dispatch-1"))
	if err != nil {
		t.Fatalf("Start() error = %v, want nil", err)
	}
	if result.Session.ID != "worker-1" || result.Session.State != workersessions.StateCompleted {
		t.Fatalf("Start() session = %+v, want ID=worker-1 State=COMPLETED", result.Session)
	}
}

func TestStart_ExactlyOneDetachedRequestReachesWorkersWithExpectedAttemptIdentity(t *testing.T) {
	execution := succeedingExecution()
	registry := newRegistryWithExecution(execution)
	ctx := context.Background()

	if _, err := registry.Start(ctx, validStartRequest("worker-1", "dispatch-1")); err != nil {
		t.Fatalf("Start() error = %v, want nil", err)
	}

	requests := execution.requests()
	if len(requests) != 1 {
		t.Fatalf("Workers received %d dispatch calls, want 1", len(requests))
	}
	if got := requests[0].Execution.Dispatch.DispatchID; got != "dispatch-1" {
		t.Fatalf("dispatch request attempt id = %q, want dispatch-1", got)
	}
}

func TestStart_SuccessfulWorkersResult_TerminalizesCompleted(t *testing.T) {
	registry := newRegistryWithExecution(succeedingExecution())
	ctx := context.Background()

	result, err := registry.Start(ctx, validStartRequest("worker-1", "dispatch-1"))
	if err != nil {
		t.Fatalf("Start() error = %v, want nil", err)
	}
	if result.Session.State != workersessions.StateCompleted {
		t.Fatalf("Start() state = %q, want COMPLETED", result.Session.State)
	}
	if result.Session.Result == nil || result.Session.Result.Outcome != workersessions.TerminalOutcomeCompleted || result.Session.Result.Cause != nil {
		t.Fatalf("Start() result = %+v, want COMPLETED with nil Cause", result.Session.Result)
	}

	got, err := registry.Get(ctx, workersessions.GetRequest{ID: "worker-1"})
	if err != nil {
		t.Fatalf("Get() error = %v, want nil", err)
	}
	if got.State != workersessions.StateCompleted {
		t.Fatalf("Get() after Start() state = %q, want COMPLETED", got.State)
	}
}

func TestStart_DispatchAdmissionFailure_TerminalizesFailedWithStartFailureCauseAndNoRunningObservation(t *testing.T) {
	execution := &fakeExecution{
		dispatch: func(context.Context, workers.WorkstationDispatchRequest) (workers.WorkstationDispatchResult, error) {
			return workers.WorkstationDispatchResult{}, workers.ErrWorkstationPoolUnavailable
		},
	}
	registry := newRegistryWithExecution(execution)
	ctx := context.Background()

	result, err := registry.Start(ctx, validStartRequest("worker-1", "dispatch-1"))
	if err != nil {
		t.Fatalf("Start() error = %v, want nil", err)
	}
	if result.Session.State != workersessions.StateFailed {
		t.Fatalf("Start() state = %q, want FAILED", result.Session.State)
	}
	if result.Session.Result == nil || result.Session.Result.Cause == nil {
		t.Fatalf("Start() result = %+v, want a non-nil Cause", result.Session.Result)
	}
	if got := result.Session.Result.Cause.Kind; got != workersessions.FailureCauseStartFailure {
		t.Fatalf("Start() cause kind = %q, want START_FAILURE", got)
	}
}

func TestStart_OrdinaryFailedWorkResult_TerminalizesFailedWithWorkersExecutionFailureCause(t *testing.T) {
	execution := &fakeExecution{
		dispatch: func(_ context.Context, req workers.WorkstationDispatchRequest) (workers.WorkstationDispatchResult, error) {
			return workers.WorkstationDispatchResult{
				DispatchID:      req.Execution.Dispatch.DispatchID,
				TerminalOutcome: workers.WorkstationDispatchTerminalOutcomeCompleted,
				Result: workers.WorkResult{
					DispatchID: req.Execution.Dispatch.DispatchID,
					Outcome:    workers.OutcomeFailed,
					Error:      "the business rule rejected this attempt",
				},
			}, nil
		},
	}
	registry := newRegistryWithExecution(execution)
	ctx := context.Background()

	result, err := registry.Start(ctx, validStartRequest("worker-1", "dispatch-1"))
	if err != nil {
		t.Fatalf("Start() error = %v, want nil", err)
	}
	if result.Session.State != workersessions.StateFailed {
		t.Fatalf("Start() state = %q, want FAILED", result.Session.State)
	}
	if got := result.Session.Result.Cause.Kind; got != workersessions.FailureCauseWorkersExecutionFailure {
		t.Fatalf("Start() cause kind = %q, want WORKERS_EXECUTION_FAILURE", got)
	}
}

func TestStart_ResultAndErrorDisagreement_TrustsWorkResultOutcomeOverAdapterError(t *testing.T) {
	// TerminalOutcome (adapter-error-derived) says COMPLETED, but the nested
	// WorkResult says FAILED. Worker Sessions must classify FAILED: the
	// Workers result is authoritative over the adapter's own summary field.
	execution := &fakeExecution{
		dispatch: func(_ context.Context, req workers.WorkstationDispatchRequest) (workers.WorkstationDispatchResult, error) {
			return workers.WorkstationDispatchResult{
				DispatchID:      req.Execution.Dispatch.DispatchID,
				TerminalOutcome: workers.WorkstationDispatchTerminalOutcomeCompleted,
				Result: workers.WorkResult{
					DispatchID: req.Execution.Dispatch.DispatchID,
					Outcome:    workers.OutcomeFailed,
					Error:      "silently disagreeing failure",
				},
			}, nil
		},
	}
	registry := newRegistryWithExecution(execution)
	ctx := context.Background()

	result, err := registry.Start(ctx, validStartRequest("worker-1", "dispatch-1"))
	if err != nil {
		t.Fatalf("Start() error = %v, want nil", err)
	}
	if result.Session.State != workersessions.StateFailed {
		t.Fatalf("Start() state = %q, want FAILED despite a COMPLETED-looking dispatch TerminalOutcome", result.Session.State)
	}
}

func TestStart_ResultSuccessDisagreesWithNonNilAdapterError_TrustsWorkResultOutcome(t *testing.T) {
	// The WorkResult reports success even though a non-nil adapter error
	// also came back. Result outcome is authoritative before adapter error:
	// this must still terminalize COMPLETED.
	execution := &fakeExecution{
		dispatch: func(_ context.Context, req workers.WorkstationDispatchRequest) (workers.WorkstationDispatchResult, error) {
			return workers.WorkstationDispatchResult{
				DispatchID: req.Execution.Dispatch.DispatchID,
				Result: workers.WorkResult{
					DispatchID: req.Execution.Dispatch.DispatchID,
					Outcome:    workers.OutcomeAccepted,
				},
			}, errors.New("cosmetic non-nil error alongside an accepted result")
		},
	}
	registry := newRegistryWithExecution(execution)
	ctx := context.Background()

	result, err := registry.Start(ctx, validStartRequest("worker-1", "dispatch-1"))
	if err != nil {
		t.Fatalf("Start() error = %v, want nil", err)
	}
	if result.Session.State != workersessions.StateCompleted {
		t.Fatalf("Start() state = %q, want COMPLETED because WorkResult.Outcome is authoritative", result.Session.State)
	}
}

func TestStart_ExecutorPanicEvidenceWithNilAdapterError_MapsToExecutorPanicCause(t *testing.T) {
	execution := &fakeExecution{
		dispatch: func(_ context.Context, req workers.WorkstationDispatchRequest) (workers.WorkstationDispatchResult, error) {
			return workers.WorkstationDispatchResult{
				DispatchID:      req.Execution.Dispatch.DispatchID,
				TerminalOutcome: workers.WorkstationDispatchTerminalOutcomeCompleted,
				Result: workers.WorkResult{
					DispatchID: req.Execution.Dispatch.DispatchID,
					Outcome:    workers.OutcomeFailed,
					Error:      "executor panic: boom",
				},
			}, nil // adapter error is nil despite the panic evidence in the result
		},
	}
	registry := newRegistryWithExecution(execution)
	ctx := context.Background()

	result, err := registry.Start(ctx, validStartRequest("worker-1", "dispatch-1"))
	if err != nil {
		t.Fatalf("Start() error = %v, want nil", err)
	}
	if result.Session.State != workersessions.StateFailed {
		t.Fatalf("Start() state = %q, want FAILED", result.Session.State)
	}
	if got := result.Session.Result.Cause.Kind; got != workersessions.FailureCauseExecutorPanic {
		t.Fatalf("Start() cause kind = %q, want EXECUTOR_PANIC despite a nil adapter error", got)
	}
}

func TestStart_ExecutorPanicTypedAdapterError_MapsToExecutorPanicCause(t *testing.T) {
	panicErr := &workers.WorkerExecutorPanicError{Cause: "boom"}
	execution := &fakeExecution{
		dispatch: func(_ context.Context, req workers.WorkstationDispatchRequest) (workers.WorkstationDispatchResult, error) {
			return workers.WorkstationDispatchResult{
				DispatchID: req.Execution.Dispatch.DispatchID,
				Result: workers.WorkResult{
					DispatchID: req.Execution.Dispatch.DispatchID,
					Outcome:    workers.OutcomeFailed,
					Error:      panicErr.Error(),
				},
			}, panicErr
		},
	}
	registry := newRegistryWithExecution(execution)
	ctx := context.Background()

	result, err := registry.Start(ctx, validStartRequest("worker-1", "dispatch-1"))
	if err != nil {
		t.Fatalf("Start() error = %v, want nil", err)
	}
	if got := result.Session.Result.Cause.Kind; got != workersessions.FailureCauseExecutorPanic {
		t.Fatalf("Start() cause kind = %q, want EXECUTOR_PANIC", got)
	}
}

func TestStart_FailedResultWithNonPanicAdapterError_MapsToAdapterFailureCause(t *testing.T) {
	execution := &fakeExecution{
		dispatch: func(_ context.Context, req workers.WorkstationDispatchRequest) (workers.WorkstationDispatchResult, error) {
			return workers.WorkstationDispatchResult{
				DispatchID: req.Execution.Dispatch.DispatchID,
				Result: workers.WorkResult{
					DispatchID: req.Execution.Dispatch.DispatchID,
					Outcome:    workers.OutcomeFailed,
					Error:      "transport reset",
				},
			}, errors.New("transport reset")
		},
	}
	registry := newRegistryWithExecution(execution)
	ctx := context.Background()

	result, err := registry.Start(ctx, validStartRequest("worker-1", "dispatch-1"))
	if err != nil {
		t.Fatalf("Start() error = %v, want nil", err)
	}
	if got := result.Session.Result.Cause.Kind; got != workersessions.FailureCauseAdapterFailure {
		t.Fatalf("Start() cause kind = %q, want ADAPTER_FAILURE", got)
	}
}

func TestStart_FailedResultWithBlankWorkResultError_FallsBackToAdapterErrorDetail(t *testing.T) {
	adapterErr := errors.New("transport reset")
	execution := &fakeExecution{
		dispatch: func(_ context.Context, req workers.WorkstationDispatchRequest) (workers.WorkstationDispatchResult, error) {
			return workers.WorkstationDispatchResult{
				DispatchID: req.Execution.Dispatch.DispatchID,
				Result: workers.WorkResult{
					DispatchID: req.Execution.Dispatch.DispatchID,
					Outcome:    workers.OutcomeFailed,
				},
			}, adapterErr
		},
	}
	registry := newRegistryWithExecution(execution)
	ctx := context.Background()

	result, err := registry.Start(ctx, validStartRequest("worker-1", "dispatch-1"))
	if err != nil {
		t.Fatalf("Start() error = %v, want nil", err)
	}
	if got := result.Session.Result.Cause.Kind; got != workersessions.FailureCauseAdapterFailure {
		t.Fatalf("Start() cause kind = %q, want ADAPTER_FAILURE", got)
	}
	if got := result.Session.Result.Cause.Detail; got != adapterErr.Error() {
		t.Fatalf("Start() cause detail = %q, want fallback to adapter error %q", got, adapterErr.Error())
	}
}

func TestStart_DuplicateConflictingStart_ReturnsTypedErrorAndMakesNoWorkersCall(t *testing.T) {
	execution := succeedingExecution()
	registry := newRegistryWithExecution(execution)
	ctx := context.Background()

	first, err := registry.Start(ctx, validStartRequest("worker-1", "dispatch-1"))
	if err != nil {
		t.Fatalf("first Start() error = %v, want nil", err)
	}
	if first.Session.State != workersessions.StateCompleted {
		t.Fatalf("first Start() state = %q, want COMPLETED", first.Session.State)
	}

	callsBefore := execution.callCount()
	if _, err := registry.Start(ctx, validStartRequest("worker-1", "dispatch-2")); !errors.Is(err, workersessions.ErrSessionNotStartable) {
		t.Fatalf("second Start() on a terminal session error = %v, want ErrSessionNotStartable", err)
	}
	if execution.callCount() != callsBefore {
		t.Fatalf("conflicting Start() called Workers, want no additional calls")
	}

	got, err := registry.Get(ctx, workersessions.GetRequest{ID: "worker-1"})
	if err != nil {
		t.Fatalf("Get() error = %v, want nil", err)
	}
	if !sessionsEqual(got, first.Session) {
		t.Fatalf("Get() after conflicting Start() = %+v, want unchanged %+v", got, first.Session)
	}
}

// sessionsEqual compares two detached Session snapshots by value, following
// the Result pointer, since Get/Start return freshly cloned pointers even
// when the underlying committed outcome is unchanged.
func sessionsEqual(a, b workersessions.Session) bool {
	if a.ID != b.ID || a.State != b.State {
		return false
	}
	if (a.Result == nil) != (b.Result == nil) {
		return false
	}
	if a.Result == nil {
		return true
	}
	if a.Result.Outcome != b.Result.Outcome {
		return false
	}
	if (a.Result.Cause == nil) != (b.Result.Cause == nil) {
		return false
	}
	if a.Result.Cause == nil {
		return true
	}
	return *a.Result.Cause == *b.Result.Cause
}

func TestStart_MissingReservedIdentityCollidingWithInFlightStart_ReturnsTypedErrorAndMakesNoWorkersCall(t *testing.T) {
	started := make(chan struct{})
	release := make(chan struct{})
	execution := &fakeExecution{
		dispatch: func(context.Context, workers.WorkstationDispatchRequest) (workers.WorkstationDispatchResult, error) {
			close(started)
			<-release
			return workers.WorkstationDispatchResult{
				Result: workers.WorkResult{Outcome: workers.OutcomeAccepted},
			}, nil
		},
	}
	registry := newRegistryWithExecution(execution)
	ctx := context.Background()

	var wg sync.WaitGroup
	wg.Add(1)
	go func() {
		defer wg.Done()
		if _, err := registry.Start(ctx, validStartRequest("worker-1", "dispatch-1")); err != nil {
			t.Errorf("first Start() error = %v, want nil", err)
		}
	}()
	<-started

	if _, err := registry.Start(ctx, validStartRequest("worker-1", "dispatch-2")); !errors.Is(err, workersessions.ErrSessionNotStartable) {
		t.Fatalf("concurrent Start() on an in-flight session error = %v, want ErrSessionNotStartable", err)
	}
	if execution.callCount() != 1 {
		t.Fatalf("Workers received %d dispatch calls while in-flight, want 1", execution.callCount())
	}

	close(release)
	wg.Wait()
}

func TestStart_TerminalStateIsAbsorbingUnderConcurrentGetAndList(t *testing.T) {
	registry := newRegistryWithExecution(succeedingExecution())
	ctx := context.Background()

	if _, err := registry.Start(ctx, validStartRequest("worker-1", "dispatch-1")); err != nil {
		t.Fatalf("Start() error = %v, want nil", err)
	}

	var wg sync.WaitGroup
	const readers = 50
	wg.Add(readers)
	for range readers {
		go func() {
			defer wg.Done()
			session, err := registry.Get(ctx, workersessions.GetRequest{ID: "worker-1"})
			if err != nil {
				t.Errorf("Get() error = %v, want nil", err)
				return
			}
			if err := session.Validate(); err != nil {
				t.Errorf("Get() returned an invalid terminal snapshot: %v", err)
			}
			if session.State != workersessions.StateCompleted {
				t.Errorf("Get() state = %q, want COMPLETED (absorbing)", session.State)
			}
			result, err := registry.List(ctx, workersessions.ListRequest{})
			if err != nil {
				t.Errorf("List() error = %v, want nil", err)
				return
			}
			for _, s := range result.Sessions {
				if err := s.Validate(); err != nil {
					t.Errorf("List() returned an invalid snapshot: %v", err)
				}
			}
		}()
	}
	wg.Wait()
}

func TestConcurrentStart_DistinctSessions_TerminalizeIndependentlyWithoutCrossAssignment(t *testing.T) {
	ctx := context.Background()
	const count = 100

	dispatches := make(chan workers.WorkstationDispatchRequest, count)
	execution := &fakeExecution{
		dispatch: func(_ context.Context, req workers.WorkstationDispatchRequest) (workers.WorkstationDispatchResult, error) {
			dispatches <- req
			return workers.WorkstationDispatchResult{
				DispatchID: req.Execution.Dispatch.DispatchID,
				Result: workers.WorkResult{
					DispatchID: req.Execution.Dispatch.DispatchID,
					Outcome:    workers.OutcomeAccepted,
				},
			}, nil
		},
	}
	registry := newRegistryWithExecution(execution)

	var wg sync.WaitGroup
	wg.Add(count)
	for i := range count {
		go func(i int) {
			defer wg.Done()
			id := fmt.Sprintf("worker-%d", i)
			attemptID := fmt.Sprintf("dispatch-%d", i)
			result, err := registry.Start(ctx, validStartRequest(id, attemptID))
			if err != nil {
				t.Errorf("Start(%q) error = %v, want nil", id, err)
				return
			}
			if result.Session.ID != id {
				t.Errorf("Start(%q) returned session ID %q, cross-assigned identity", id, result.Session.ID)
			}
			if result.Session.State != workersessions.StateCompleted {
				t.Errorf("Start(%q) state = %q, want COMPLETED", id, result.Session.State)
			}
		}(i)
	}
	wg.Wait()
	close(dispatches)

	seen := make(map[string]bool, count)
	for req := range dispatches {
		if seen[req.Execution.Dispatch.DispatchID] {
			t.Fatalf("attempt id %q dispatched more than once", req.Execution.Dispatch.DispatchID)
		}
		seen[req.Execution.Dispatch.DispatchID] = true
	}
	if len(seen) != count {
		t.Fatalf("saw %d distinct dispatched attempt ids, want %d", len(seen), count)
	}

	result, err := registry.List(ctx, workersessions.ListRequest{})
	if err != nil {
		t.Fatalf("List() error = %v, want nil", err)
	}
	if len(result.Sessions) != count {
		t.Fatalf("List() returned %d sessions, want %d", len(result.Sessions), count)
	}
	for _, session := range result.Sessions {
		if err := session.Validate(); err != nil {
			t.Errorf("List() returned an invalid session %q: %v", session.ID, err)
		}
		if session.State != workersessions.StateCompleted {
			t.Errorf("session %q state = %q, want COMPLETED", session.ID, session.State)
		}
	}
}
