package service

import (
	"context"
	"errors"
	"testing"

	workersessions "github.com/portpowered/infinite-you/pkg/services/worker_sessions"
	"github.com/portpowered/infinite-you/pkg/services/workers"
)

// unusedExecution is a workers.WorkstationExecutionService double that fails
// the test if it is ever called. reserveIfAbsent and transitionToStarting
// never reach Workers, so this proves the reservation and starting
// transition are genuinely separate, Workers-free steps.
type unusedExecution struct {
	t *testing.T
}

func (u unusedExecution) StartWorkstationPool(context.Context, workers.WorkstationPoolStartRequest) (workers.WorkstationPoolStartResult, error) {
	u.t.Fatal("unexpected StartWorkstationPool call")
	return workers.WorkstationPoolStartResult{}, nil
}

func (u unusedExecution) StopWorkstationPool(context.Context) (workers.WorkstationPoolStopResult, error) {
	u.t.Fatal("unexpected StopWorkstationPool call")
	return workers.WorkstationPoolStopResult{}, nil
}

func (u unusedExecution) DispatchWorkstation(context.Context, workers.WorkstationDispatchRequest) (workers.WorkstationDispatchResult, error) {
	u.t.Fatal("unexpected DispatchWorkstation call")
	return workers.WorkstationDispatchResult{}, nil
}

func (u unusedExecution) CancelWorkstationDispatch(context.Context, workers.WorkstationDispatchCancelRequest) (workers.WorkstationDispatchCancelResult, error) {
	u.t.Fatal("unexpected CancelWorkstationDispatch call")
	return workers.WorkstationDispatchCancelResult{}, nil
}

// newTestRegistry returns the concrete *registry (not just the Service
// interface) so white-box tests in this package can drive reserveIfAbsent
// and transitionToStarting directly.
func newTestRegistry(t *testing.T) *registry {
	t.Helper()
	svc, err := New(unusedExecution{t: t}, nil)
	if err != nil {
		t.Fatalf("New() error = %v, want nil", err)
	}
	r, ok := svc.(*registry)
	if !ok {
		t.Fatalf("New() returned %T, want *registry", svc)
	}
	return r
}

// TestReserveIfAbsent_NewIdentity_IsObservableAsReservedBeforeStartingTransition
// proves the defect the review flagged is fixed: a brand-new identity is a
// genuine, Get-observable RESERVED map write, distinct from and before the
// STARTING transition, with no Workers call involved in either step.
func TestReserveIfAbsent_NewIdentity_IsObservableAsReservedBeforeStartingTransition(t *testing.T) {
	r := newTestRegistry(t)
	ctx := context.Background()

	r.reserveIfAbsent("worker-1")

	reserved, err := r.Get(ctx, workersessions.GetRequest{ID: "worker-1"})
	if err != nil {
		t.Fatalf("Get() after reserveIfAbsent error = %v, want nil", err)
	}
	if reserved.State != workersessions.StateReserved {
		t.Fatalf("Get() after reserveIfAbsent state = %q, want RESERVED", reserved.State)
	}

	if _, err := r.transitionToStarting("worker-1"); err != nil {
		t.Fatalf("transitionToStarting() error = %v, want nil", err)
	}

	starting, err := r.Get(ctx, workersessions.GetRequest{ID: "worker-1"})
	if err != nil {
		t.Fatalf("Get() after transitionToStarting error = %v, want nil", err)
	}
	if starting.State != workersessions.StateStarting {
		t.Fatalf("Get() after transitionToStarting state = %q, want STARTING", starting.State)
	}
}

// TestReserveIfAbsent_ExistingReservedIdentity_IsLeftUntouched proves
// reserveIfAbsent never replaces an already-registered session, including
// one that was reserved by a prior call.
func TestReserveIfAbsent_ExistingReservedIdentity_IsLeftUntouched(t *testing.T) {
	r := newTestRegistry(t)
	r.reserveIfAbsent("worker-1")
	r.reserveIfAbsent("worker-1")

	r.mu.RLock()
	session, exists := r.sessions["worker-1"]
	r.mu.RUnlock()
	if !exists {
		t.Fatal("session missing after second reserveIfAbsent call")
	}
	if session.State != workersessions.StateReserved {
		t.Fatalf("session state = %q, want RESERVED", session.State)
	}
}

// TestTransitionToStarting_UnreservedIdentity_ReturnsErrSessionNotStartable
// proves the STARTING transition can never fabricate a session that skipped
// the RESERVED write.
func TestTransitionToStarting_UnreservedIdentity_ReturnsErrSessionNotStartable(t *testing.T) {
	r := newTestRegistry(t)

	if _, err := r.transitionToStarting("worker-1"); !errors.Is(err, workersessions.ErrSessionNotStartable) {
		t.Fatalf("transitionToStarting() error = %v, want ErrSessionNotStartable", err)
	}

	r.mu.RLock()
	_, exists := r.sessions["worker-1"]
	r.mu.RUnlock()
	if exists {
		t.Fatal("transitionToStarting() on an unreserved identity must not create a session")
	}
}
