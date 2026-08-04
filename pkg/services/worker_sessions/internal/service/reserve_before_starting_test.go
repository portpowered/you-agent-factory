package service

import (
	"context"
	"errors"
	"sync"
	"testing"

	eventswire "github.com/portpowered/infinite-you/pkg/services/events/wire"
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
	events, err := eventswire.NewService()
	if err != nil {
		t.Fatalf("eventswire.NewService() error = %v, want nil", err)
	}
	svc, err := New(unusedExecution{t: t}, events, nil)
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

// TestCommitTerminal_AlreadyTerminalIdentity_IsAbsorbingAndDoesNotOverwrite
// proves the review-flagged defect is fixed: a second commitTerminal call for
// an identity that is already terminal never overwrites the first committed
// outcome or its FailureCause, and reports it did not win the commit.
func TestCommitTerminal_AlreadyTerminalIdentity_IsAbsorbingAndDoesNotOverwrite(t *testing.T) {
	r := newTestRegistry(t)
	r.reserveIfAbsent("worker-1")
	if _, err := r.transitionToStarting("worker-1"); err != nil {
		t.Fatalf("transitionToStarting() error = %v, want nil", err)
	}

	first := workersessions.TerminalResult{Outcome: workersessions.TerminalOutcomeCompleted}
	committedFirst, wonFirst := r.commitTerminal("worker-1", workersessions.StateCompleted, first)
	if !wonFirst {
		t.Fatal("first commitTerminal() committed = false, want true")
	}
	if committedFirst.State != workersessions.StateCompleted {
		t.Fatalf("first commitTerminal() state = %q, want COMPLETED", committedFirst.State)
	}

	second := workersessions.TerminalResult{
		Outcome: workersessions.TerminalOutcomeFailed,
		Cause:   &workersessions.FailureCause{Kind: workersessions.FailureCauseWorkersExecutionFailure, Detail: "late duplicate callback"},
	}
	committedSecond, wonSecond := r.commitTerminal("worker-1", workersessions.StateFailed, second)
	if wonSecond {
		t.Fatal("second commitTerminal() on an already-terminal identity committed = true, want false (absorbing)")
	}
	if committedSecond.State != workersessions.StateCompleted {
		t.Fatalf("second commitTerminal() returned state = %q, want unchanged COMPLETED", committedSecond.State)
	}
	if committedSecond.Result == nil || committedSecond.Result.Outcome != workersessions.TerminalOutcomeCompleted || committedSecond.Result.Cause != nil {
		t.Fatalf("second commitTerminal() returned result = %+v, want unchanged COMPLETED with nil Cause", committedSecond.Result)
	}

	got, err := r.Get(context.Background(), workersessions.GetRequest{ID: "worker-1"})
	if err != nil {
		t.Fatalf("Get() error = %v, want nil", err)
	}
	if got.State != workersessions.StateCompleted {
		t.Fatalf("Get() after duplicate commitTerminal state = %q, want COMPLETED (unchanged)", got.State)
	}
}

// TestCommitTerminal_MissingIdentity_DoesNotFabricateASession proves
// commitTerminal never creates or terminalizes an identity that was never
// reserved or transitioned to StateStarting: it must be a no-op that reports
// committed=false and leaves the registry without that identity.
func TestCommitTerminal_MissingIdentity_DoesNotFabricateASession(t *testing.T) {
	r := newTestRegistry(t)

	result := workersessions.TerminalResult{Outcome: workersessions.TerminalOutcomeCompleted}
	_, committed := r.commitTerminal("worker-1", workersessions.StateCompleted, result)
	if committed {
		t.Fatal("commitTerminal() on a missing identity committed = true, want false")
	}

	if _, err := r.Get(context.Background(), workersessions.GetRequest{ID: "worker-1"}); !errors.Is(err, workersessions.ErrSessionNotFound) {
		t.Fatalf("Get() after commitTerminal() on a missing identity = %v, want ErrSessionNotFound (no session fabricated)", err)
	}
}

// TestCommitTerminal_ReservedPredecessor_IsRejectedAndLeavesSessionUnchanged
// proves commitTerminal requires the one allowed W2 predecessor,
// StateStarting: an identity still in StateReserved (which never reached
// Workers handoff) must not be terminalized.
func TestCommitTerminal_ReservedPredecessor_IsRejectedAndLeavesSessionUnchanged(t *testing.T) {
	r := newTestRegistry(t)
	r.reserveIfAbsent("worker-1")

	result := workersessions.TerminalResult{Outcome: workersessions.TerminalOutcomeCompleted}
	got, committed := r.commitTerminal("worker-1", workersessions.StateCompleted, result)
	if committed {
		t.Fatal("commitTerminal() from RESERVED committed = true, want false")
	}
	if got.State != workersessions.StateReserved {
		t.Fatalf("commitTerminal() from RESERVED returned state = %q, want unchanged RESERVED", got.State)
	}

	session, err := r.Get(context.Background(), workersessions.GetRequest{ID: "worker-1"})
	if err != nil {
		t.Fatalf("Get() error = %v, want nil", err)
	}
	if session.State != workersessions.StateReserved {
		t.Fatalf("Get() after rejected commitTerminal() state = %q, want unchanged RESERVED", session.State)
	}
}

// TestCommitTerminal_ConcurrentCompetingOutcomes_OnlyOneWinsAndStateStaysAbsorbing
// deterministically synchronizes several goroutines to reach commitTerminal
// for the same identity at once with different, disagreeing outcomes and
// causes. Exactly one may win the commit, and the resulting session must
// remain a single valid, absorbing terminal snapshot no matter which
// goroutine's outcome happened to win the race.
func TestCommitTerminal_ConcurrentCompetingOutcomes_OnlyOneWinsAndStateStaysAbsorbing(t *testing.T) {
	r := newTestRegistry(t)
	r.reserveIfAbsent("worker-1")
	if _, err := r.transitionToStarting("worker-1"); err != nil {
		t.Fatalf("transitionToStarting() error = %v, want nil", err)
	}

	outcomes := []workersessions.TerminalResult{
		{Outcome: workersessions.TerminalOutcomeCompleted},
		{Outcome: workersessions.TerminalOutcomeFailed, Cause: &workersessions.FailureCause{Kind: workersessions.FailureCauseWorkersExecutionFailure, Detail: "a"}},
		{Outcome: workersessions.TerminalOutcomeFailed, Cause: &workersessions.FailureCause{Kind: workersessions.FailureCauseAdapterFailure, Detail: "b"}},
	}
	states := []workersessions.State{workersessions.StateCompleted, workersessions.StateFailed, workersessions.StateFailed}

	var wg sync.WaitGroup
	var mu sync.Mutex
	wins := 0
	ready := make(chan struct{})
	wg.Add(len(outcomes))
	for i := range outcomes {
		go func(i int) {
			defer wg.Done()
			<-ready
			_, committed := r.commitTerminal("worker-1", states[i], outcomes[i])
			if committed {
				mu.Lock()
				wins++
				mu.Unlock()
			}
		}(i)
	}
	close(ready)
	wg.Wait()

	if wins != 1 {
		t.Fatalf("commitTerminal concurrent winners = %d, want exactly 1", wins)
	}

	got, err := r.Get(context.Background(), workersessions.GetRequest{ID: "worker-1"})
	if err != nil {
		t.Fatalf("Get() error = %v, want nil", err)
	}
	if err := got.Validate(); err != nil {
		t.Fatalf("Get() after concurrent commitTerminal returned an invalid session: %v", err)
	}
	if !got.State.Terminal() {
		t.Fatalf("Get() after concurrent commitTerminal state = %q, want a terminal state", got.State)
	}
}
