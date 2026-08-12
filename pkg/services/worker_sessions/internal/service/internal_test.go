// backendsizecheck:ignore-file pre-existing baseline debt recorded 2026-08-08; split this oversized code into focused units and remove this exemption
// pkgmaintcheck:ignore-file-lines pre-existing baseline debt recorded 2026-08-08; refactor this code below the maintainability threshold and remove this exemption
package service

import (
	"context"
	"encoding/json"
	"errors"
	"reflect"
	"strings"
	"sync"
	"testing"

	platformclock "github.com/portpowered/infinite-you/pkg/platform/clock"

	"github.com/portpowered/infinite-you/pkg/platform/logging"
	"github.com/portpowered/infinite-you/pkg/services/events"
	eventswire "github.com/portpowered/infinite-you/pkg/services/events/wire"
	providersessions "github.com/portpowered/infinite-you/pkg/services/provider_sessions"
	"github.com/portpowered/infinite-you/pkg/services/providers"
	"github.com/portpowered/infinite-you/pkg/services/work"
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

type unavailableProviderSessions struct {
	providersessions.Service
}

func (unavailableProviderSessions) Project(providersessions.ProjectRequest) (providersessions.ProjectResult, error) {
	return providersessions.ProjectResult{}, providersessions.ErrSessionStorageUnavailable
}

type failingPublishBoundary struct {
	unusedExecution
	err error
}

func (b failingPublishBoundary) Publish(context.Context, workers.WorkstationDispatchRequest, workers.WorkstationDispatchAcceptFunc) error {
	return b.err
}

func (b failingPublishBoundary) PublishWithAdmission(context.Context, workers.WorkstationDispatchRequest, workers.WorkstationDispatchAdmissionFunc, workers.WorkstationDispatchAcceptFunc) error {
	return b.err
}

// cancellationResultBoundary supplies one deterministic boundary cancellation
// result without starting or publishing any Workers attempt. It lets the
// control tests exercise only the already-admitted control effect.
type cancellationResultBoundary struct {
	unusedExecution
	result workers.WorkstationDispatchCancelResult
	err    error
}

func (b cancellationResultBoundary) Cancel(context.Context, workers.WorkstationDispatchCancelRequest) (workers.WorkstationDispatchCancelResult, error) {
	return b.result, b.err
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

func (u unusedExecution) DispatchWorkstationWithAdmission(context.Context, workers.WorkstationDispatchRequest, workers.WorkstationDispatchAdmissionFunc) (workers.WorkstationDispatchResult, error) {
	u.t.Fatal("unexpected DispatchWorkstationWithAdmission call")
	return workers.WorkstationDispatchResult{}, nil
}

func (u unusedExecution) CancelWorkstationDispatch(context.Context, workers.WorkstationDispatchCancelRequest) (workers.WorkstationDispatchCancelResult, error) {
	u.t.Fatal("unexpected CancelWorkstationDispatch call")
	return workers.WorkstationDispatchCancelResult{}, nil
}

func (u unusedExecution) Start(context.Context) error {
	u.t.Fatal("unexpected boundary Start call")
	return nil
}

func (u unusedExecution) Publish(context.Context, workers.WorkstationDispatchRequest, workers.WorkstationDispatchAcceptFunc) error {
	u.t.Fatal("unexpected boundary Publish call")
	return nil
}

func (u unusedExecution) PublishWithAdmission(context.Context, workers.WorkstationDispatchRequest, workers.WorkstationDispatchAdmissionFunc, workers.WorkstationDispatchAcceptFunc) error {
	u.t.Fatal("unexpected boundary PublishWithAdmission call")
	return nil
}

func (u unusedExecution) Cancel(context.Context, workers.WorkstationDispatchCancelRequest) (workers.WorkstationDispatchCancelResult, error) {
	u.t.Fatal("unexpected boundary Cancel call")
	return workers.WorkstationDispatchCancelResult{}, nil
}

func (u unusedExecution) Stop(context.Context) error {
	u.t.Fatal("unexpected boundary Stop call")
	return nil
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
	svc, err := New(unusedExecution{t: t}, events, nil, platformclock.Real{}, unavailableProviderSessions{}, nil)
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

func TestCommitTerminal_NormalizesEmptyFailureCauseBeforeCommit(t *testing.T) {
	r := newTestRegistry(t)
	r.reserveIfAbsent("worker-1")
	if _, err := r.transitionToStarting("worker-1"); err != nil {
		t.Fatalf("transitionToStarting() error = %v, want nil", err)
	}

	committed, won := r.commitTerminal("worker-1", workersessions.StateFailed, workersessions.TerminalResult{
		Outcome: workersessions.TerminalOutcomeFailed,
		Cause:   &workersessions.FailureCause{Kind: workersessions.FailureCauseAdapterFailure},
	})
	if !won {
		t.Fatal("commitTerminal() committed = false, want true")
	}
	if committed.Result == nil || committed.Result.Cause == nil {
		t.Fatalf("committed result = %#v, want failure cause", committed.Result)
	}
	if strings.TrimSpace(committed.Result.Cause.Detail) == "" ||
		len([]rune(committed.Result.Cause.Detail)) > workersessions.MaxFailureCauseDetailRunes {
		t.Fatalf("committed failure detail = %q, want non-empty bounded detail", committed.Result.Cause.Detail)
	}
	if err := committed.Validate(); err != nil {
		t.Fatalf("committed.Validate() = %v, want valid normalized terminal", err)
	}
}

// TestTransitionToRunning_TerminalSessionRemainsAbsorbing proves a late
// admission callback cannot resurrect a Worker Session after its terminal
// result has been committed. This is the guard that preserves the existing
// terminal authority when completion wins an admission/control race.
func TestTransitionToRunning_TerminalSessionRemainsAbsorbing(t *testing.T) {
	r := newTestRegistry(t)
	r.reserveIfAbsent("worker-1")
	if _, err := r.transitionToStarting("worker-1"); err != nil {
		t.Fatalf("transitionToStarting() error = %v, want nil", err)
	}
	if _, committed := r.commitTerminal("worker-1", workersessions.StateCompleted, workersessions.TerminalResult{
		Outcome: workersessions.TerminalOutcomeCompleted,
	}); !committed {
		t.Fatal("commitTerminal() committed = false, want true")
	}

	r.transitionToRunning("worker-1")

	got, err := r.Get(context.Background(), workersessions.GetRequest{ID: "worker-1"})
	if err != nil {
		t.Fatalf("Get() error = %v, want nil", err)
	}
	if got.State != workersessions.StateCompleted || got.Result == nil || got.Result.Outcome != workersessions.TerminalOutcomeCompleted {
		t.Fatalf("Get() after late running transition = %#v, want unchanged COMPLETED terminal result", got)
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

// pkgmaintcheck:ignore-cyclomatic-complexity pre-existing baseline debt recorded 2026-08-08; refactor this code below the maintainability threshold and remove this exemption
func TestControlGuards_RejectInvalidTransitionsAndPreserveObservableSessionState(t *testing.T) {
	r := newTestRegistry(t)
	ctx := context.Background()

	if got, committed := r.commitControlTerminal("missing", workersessions.StateCanceled); committed || got.ID != "" {
		t.Fatalf("commit missing control terminal = %#v, %t; want no fabricated session", got, committed)
	}
	r.reserveIfAbsent("worker-1")
	if got, committed := r.commitControlTerminal("worker-1", workersessions.StateCompleted); committed || got.State != workersessions.StateReserved {
		t.Fatalf("commit invalid control terminal = %#v, %t; want unchanged RESERVED", got, committed)
	}
	if got, committed := r.commitControlTerminal("worker-1", workersessions.StateCanceled); !committed || got.State != workersessions.StateCanceled {
		t.Fatalf("commit canceled control terminal = %#v, %t; want CANCELED", got, committed)
	}
	if got, committed := r.commitControlTerminal("worker-1", workersessions.StateTerminated); committed || got.State != workersessions.StateCanceled {
		t.Fatalf("second control terminal = %#v, %t; want absorbing CANCELED", got, committed)
	}
	if session, ok := r.preAdmissionControlTerminal("missing", "dispatch-missing"); ok || session.ID != "" {
		t.Fatalf("preAdmissionControlTerminal(missing) = %#v, %t; want no terminal", session, ok)
	}

	r.reserveIfAbsent("worker-2")
	if _, err := r.transitionToStarting("worker-2"); err != nil {
		t.Fatalf("transitionToStarting(worker-2): %v", err)
	}
	supervision, ok := r.registerSupervision("worker-2", "dispatch-2", "")
	if !ok || supervision == nil {
		t.Fatal("registerSupervision(worker-2) = unavailable, want exact supervised attempt")
	}
	if _, ok := r.registerSupervision("missing", "dispatch-missing", ""); ok {
		t.Fatal("registerSupervision(missing) unexpectedly succeeded")
	}
	supervision.requestedAction = workersessions.ControlActionCancel
	if r.beginBoundaryPublish("worker-2", supervision) {
		t.Fatal("beginBoundaryPublish() succeeded after a control request")
	}
	r.reserveIfAbsent("worker-3")
	if workerSession, err := r.Get(ctx, workersessions.GetRequest{ID: "worker-3"}); err != nil || workerSession.State != workersessions.StateReserved {
		t.Fatalf("Get(worker-3) = %#v, %v, want RESERVED", workerSession, err)
	}
	if r.beginBoundaryPublish("worker-3", newSupervision("dispatch-3", "")) {
		t.Fatal("beginBoundaryPublish() succeeded for a session that never started")
	}
	if session, err := r.Get(ctx, workersessions.GetRequest{ID: "worker-2"}); err != nil || session.State != workersessions.StateStarting {
		t.Fatalf("Get(worker-2) = %#v, %v, want unchanged STARTING", session, err)
	}
}

func TestDriveInvocation_ControlAndPublishFailureHaveTerminalObservableOutcomes(t *testing.T) {
	t.Run("control before boundary publication", func(t *testing.T) {
		r := newTestRegistry(t)
		r.reserveIfAbsent("worker-1")
		if _, err := r.transitionToStarting("worker-1"); err != nil {
			t.Fatalf("transitionToStarting: %v", err)
		}
		supervision, ok := r.registerSupervision("worker-1", "dispatch-1", "")
		if !ok {
			t.Fatal("registerSupervision before control: want exact starting attempt")
		}
		if _, committed := r.commitControlTerminal("worker-1", workersessions.StateCanceled); !committed {
			t.Fatal("commit control terminal did not win before boundary publication")
		}
		result, retry := r.publishRegisteredAttempt(
			context.Background(), "worker-1", dispatchHandoff("dispatch-1"), supervision, false,
		)
		if retry || result.Session.State != workersessions.StateCanceled {
			t.Fatalf("publishRegisteredAttempt() = %#v, retry %v, want retained CANCELED session and no retry", result, retry)
		}

		r.reserveIfAbsent("worker-2")
		if _, err := r.transitionToStarting("worker-2"); err != nil {
			t.Fatalf("transitionToStarting(worker-2): %v", err)
		}
		if _, committed := r.commitControlTerminal("worker-2", workersessions.StateCanceled); !committed {
			t.Fatal("commit control terminal for worker-2 did not win before supervision registration")
		}
		result, err := r.driveInvocation(
			context.Background(),
			workersessions.InvokeSessionRequest{ID: "worker-2", Execution: dispatchHandoff("dispatch-2")},
			"dispatch-2",
		)
		if err != nil || result.Session.State != workersessions.StateCanceled {
			t.Fatalf("driveInvocation() after control = %#v, %v, want retained CANCELED session", result, err)
		}
	})

	t.Run("boundary publication failure", func(t *testing.T) {
		r := newTestRegistry(t)
		r.boundary = failingPublishBoundary{unusedExecution: unusedExecution{t: t}, err: errors.New("boundary publish failed")}
		r.reserveIfAbsent("worker-1")
		if _, err := r.transitionToStarting("worker-1"); err != nil {
			t.Fatalf("transitionToStarting: %v", err)
		}
		result, err := r.driveInvocation(
			context.Background(),
			workersessions.InvokeSessionRequest{ID: "worker-1", Execution: dispatchHandoff("dispatch-1")},
			"dispatch-1",
		)
		if err != nil || result.Session.State != workersessions.StateFailed || result.Session.Result == nil {
			t.Fatalf("driveInvocation() = %#v, %v, want failed terminal session", result, err)
		}
	})
}

// dispatchHandoff builds the minimal well-formed dispatch request the
// invocation driver needs to name one attempt.
func dispatchHandoff(dispatchID string) workers.WorkstationDispatchRequest {
	return workers.WorkstationDispatchRequest{
		WorkstationName: "review",
		Execution: workers.WorkstationExecutionRequest{
			Dispatch: work.WorkDispatch{DispatchID: dispatchID, WorkstationName: "review"},
		},
	}
}

func TestCancel_BeforeBoundaryAdmissionEitherWaitsOrTerminatesTheExactSupervision(t *testing.T) {
	r := newTestRegistry(t)
	r.reserveIfAbsent("worker-1")
	if _, err := r.transitionToStarting("worker-1"); err != nil {
		t.Fatalf("transitionToStarting: %v", err)
	}
	supervision, ok := r.registerSupervision("worker-1", "dispatch-1", "")
	if !ok {
		t.Fatal("registerSupervision: want supervised STARTING attempt")
	}
	supervision.mu.Lock()
	supervision.publishing = true
	supervision.mu.Unlock()
	resultCh := make(chan workersessions.ControlResult, 1)
	errCh := make(chan error, 1)
	go func() {
		result, err := r.Cancel(context.Background(), workersessions.ControlRequest{ID: "worker-1"})
		resultCh <- result
		errCh <- err
	}()
	supervision.mu.Lock()
	supervision.publishing = false
	supervision.mu.Unlock()
	supervision.signalPublished()
	result := <-resultCh
	if err := <-errCh; err != nil || result.Outcome != workersessions.ControlOutcomeApplied || result.Session.State != workersessions.StateCanceled {
		t.Fatalf("Cancel() before admission = %#v, %v, want applied CANCELED", result, err)
	}

	r.reserveIfAbsent("worker-2")
	if _, err := r.transitionToStarting("worker-2"); err != nil {
		t.Fatalf("transitionToStarting(worker-2): %v", err)
	}
	noOpSupervision, ok := r.registerSupervision("worker-2", "dispatch-2", "")
	if !ok {
		t.Fatal("registerSupervision(worker-2): want exact attempt")
	}
	noOpSupervision.controlAction = workersessions.ControlActionCancel
	noOp, err := r.Cancel(context.Background(), workersessions.ControlRequest{ID: "worker-2"})
	if err != nil || noOp.Outcome != workersessions.ControlOutcomeNoop {
		t.Fatalf("repeated pre-admission Cancel() = %#v, %v, want NOOP", noOp, err)
	}
	requestedSupervision := newSupervision("dispatch-requested", "")
	requestedSupervision.requestedAction = workersessions.ControlActionPause
	if attempt := requestedSupervision.beginCancellation(workersessions.ControlActionCancel); attempt.kind != cancellationAttemptNoop {
		t.Fatalf("beginCancellation() after a requested action = %#v, want noop", attempt)
	}

	r.reserveIfAbsent("worker-3")
	if _, err := r.transitionToStarting("worker-3"); err != nil {
		t.Fatalf("transitionToStarting(worker-3): %v", err)
	}
	activeSupervision, ok := r.registerSupervision("worker-3", "dispatch-3", "")
	if !ok {
		t.Fatal("registerSupervision(worker-3): want exact attempt")
	}
	activeSupervision.mu.Lock()
	activeSupervision.controlActive = true
	activeSupervision.controlDone = make(chan struct{})
	closedWait := activeSupervision.controlDone
	close(closedWait)
	activeSupervision.mu.Unlock()
	attempt := activeSupervision.beginCancellation(workersessions.ControlActionCancel)
	if attempt.kind != cancellationAttemptWait || attempt.wait != closedWait {
		t.Fatalf("beginCancellation() with another active control = %#v, want wait for that control", attempt)
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

// TestAppendDraft_InvalidDraft_ReturnsErrorAndAppendsNothing proves
// appendDraft applies the existing workers.ValidateDraft rules itself before
// ever marshaling or calling Events, so every caller that funnels through it
// -- publishOpeningRecord, publishTerminalRecord, and PublishRecord alike --
// shares this one rejection path even if a future caller skips its own
// pre-validation.
func TestAppendDraft_InvalidDraft_ReturnsErrorAndAppendsNothing(t *testing.T) {
	r := newTestRegistry(t)
	identity := events.AppendIdentity{
		SourceType:     "worker_provider",
		SourceID:       "worker-1",
		SourceSequence: 1,
		SourceEventID:  "evt-1",
	}

	if _, err := r.appendDraft(context.Background(), workersessions.Topic("worker-1"), identity, "workers.draft.v1", workers.Draft{}); err == nil {
		t.Fatal("appendDraft() error = nil, want a non-nil error for a zero-value Draft")
	}
}

func TestPublishOpeningRecordWithoutWorkerRecordingArgumentStillOpensAndCloses(t *testing.T) {
	r := newTestRegistry(t)
	r.reserveIfAbsent("worker-1")

	if err := r.publishOpeningRecord(
		context.Background(),
		"worker-1",
		"dispatch-1",
		workers.SessionPayload{Status: string(workersessions.StateStarting)},
		"codex",
	); err != nil {
		t.Fatalf("publishOpeningRecord() error = %v, want nil", err)
	}
	if err := r.publishTerminalRecord(
		context.Background(),
		"worker-1",
		"dispatch-1",
		workersessions.StateCompleted,
		workersessions.TerminalResult{Outcome: workersessions.TerminalOutcomeCompleted},
	); err != nil {
		t.Fatalf("publishTerminalRecord() error = %v, want nil", err)
	}
}

func TestPublishOpeningRecordAwaitOpeningFailureClosesCapture(t *testing.T) {
	r := newTestRegistry(t)
	r.reserveIfAbsent("worker-1")
	recording := &awaitOpeningFailureRecording{err: errors.New("opening barrier failed")}

	err := r.publishOpeningRecord(
		context.Background(),
		"worker-1",
		"dispatch-1",
		workers.SessionPayload{Status: string(workersessions.StateStarting)},
		"codex",
		recording,
	)
	if !errors.Is(err, recording.err) {
		t.Fatalf("publishOpeningRecord() error = %v, want %v", err, recording.err)
	}
	if !recording.closed {
		t.Fatal("publishOpeningRecord() did not close the failed opening capture")
	}
}

func TestPublishTerminalRecordMissingSessionReturnsNotFound(t *testing.T) {
	r := newTestRegistry(t)
	err := r.publishTerminalRecord(
		context.Background(),
		"missing",
		"dispatch-1",
		workersessions.StateCompleted,
		workersessions.TerminalResult{Outcome: workersessions.TerminalOutcomeCompleted},
	)
	if !errors.Is(err, workersessions.ErrSessionNotFound) {
		t.Fatalf("publishTerminalRecord() error = %v, want ErrSessionNotFound", err)
	}
}

type awaitOpeningFailureRecording struct {
	err    error
	closed bool
}

func (recording *awaitOpeningFailureRecording) AwaitOpening(context.Context) error {
	return recording.err
}

func (recording *awaitOpeningFailureRecording) Close(context.Context) error {
	recording.closed = true
	return nil
}

func (recording *awaitOpeningFailureRecording) Abort(ctx context.Context, _ error) error {
	return recording.Close(ctx)
}

// TestPublishOutcomeLabel_CoversEveryOutcomeIncludingUnspecified proves the
// pure label mapping PublishRecord's logging depends on names every
// PublishOutcome, including the zero value no production PublishRecord call
// ever returns but the type still permits.
func TestPublishOutcomeLabel_CoversEveryOutcomeIncludingUnspecified(t *testing.T) {
	cases := map[workersessions.PublishOutcome]string{
		workersessions.PublishOutcomeAccepted:    "accepted",
		workersessions.PublishOutcomeDuplicate:   "duplicate",
		workersessions.PublishOutcomeUnspecified: "unspecified",
	}
	for outcome, want := range cases {
		if got := publishOutcomeLabel(outcome); got != want {
			t.Errorf("publishOutcomeLabel(%v) = %q, want %q", outcome, got, want)
		}
	}
}

func TestSafeDetail_NoFailureMetadata_ReturnsFixedGenericPlaceholderForKind(t *testing.T) {
	for kind, want := range genericFailureDetail {
		if got := safeDetail(kind, nil); got != want {
			t.Fatalf("safeDetail(%q, nil) = %q, want %q", kind, got, want)
		}
	}
}

func TestSafeDetail_WithFailureMetadata_ReturnsClosedVocabularyFamilyAndType(t *testing.T) {
	metadata := &workers.WorkFailureMetadata{
		Family: workers.WorkFailureFamilyRetryable,
		Type:   workers.WorkFailureTypeTimeout,
	}
	got := safeDetail(workersessions.FailureCauseWorkersExecutionFailure, metadata)
	want := "family=retryable type=timeout"
	if got != want {
		t.Fatalf("safeDetail() = %q, want %q", got, want)
	}
}

func TestSafeDetail_WithPartialFailureMetadata_FillsMissingHalfWithUnknown(t *testing.T) {
	metadata := &workers.WorkFailureMetadata{Type: workers.WorkFailureTypeAuthFailure}
	got := safeDetail(workersessions.FailureCauseWorkersExecutionFailure, metadata)
	want := "family=unknown type=auth_failure"
	if got != want {
		t.Fatalf("safeDetail() = %q, want %q", got, want)
	}
}

func TestSafeDetail_WithStructuredSchemaViolationPreservesClosedVocabularyType(t *testing.T) {
	metadata := &workers.WorkFailureMetadata{
		Family: workers.WorkFailureFamilyTerminal,
		Type:   workers.WorkFailureTypeStructuredOutputSchemaViolation,
	}
	got := safeDetail(workersessions.FailureCauseWorkersExecutionFailure, metadata)
	want := "family=terminal type=structured_output_schema_violation"
	if got != want {
		t.Fatalf("safeDetail() = %q, want %q", got, want)
	}
}

func TestSafeDetail_WithEmptyFailureMetadata_FallsBackToGenericPlaceholder(t *testing.T) {
	got := safeDetail(workersessions.FailureCauseWorkersExecutionFailure, &workers.WorkFailureMetadata{})
	want := genericFailureDetail[workersessions.FailureCauseWorkersExecutionFailure]
	if got != want {
		t.Fatalf("safeDetail() = %q, want fixed generic placeholder %q", got, want)
	}
}

func TestClassifyTerminal_ProviderSessionInspectionFailureRetainsSafeCauseContext(t *testing.T) {
	dispatchResult := workers.WorkstationDispatchResult{
		DispatchID: "dispatch-inspection-failure",
		Result: workers.WorkResult{
			DispatchID: "dispatch-inspection-failure",
			Outcome:    workers.OutcomeFailed,
			FailureMetadata: &workers.WorkFailureMetadata{
				Family: workers.WorkFailureFamilyTerminal,
				Type:   workers.WorkFailureTypeUnknown,
			},
			Diagnostics: &workers.WorkDiagnostics{
				Provider: &workers.ProviderDiagnostic{
					ResponseMetadata: map[string]string{
						workers.ProviderResponseMetadataFailureOperation:      "provider_session_ingestion",
						workers.ProviderResponseMetadataFailureClassification: "resource_limit",
						workers.ProviderResponseMetadataFailureStage:          "final_parse",
						"raw_rollout": "must not be copied",
					},
				},
			},
		},
	}

	terminal := classifyTerminal(nil, dispatchResult)
	if terminal.Outcome != workersessions.TerminalOutcomeFailed || terminal.Cause == nil {
		t.Fatalf("terminal = %#v, want failed result with cause", terminal)
	}
	if got := terminal.Cause.Detail; got != "family=terminal type=unknown operation=provider_session_ingestion classification=resource_limit stage=final_parse" {
		t.Fatalf("terminal cause detail = %q, want safe inspection context", got)
	}
	if strings.Contains(terminal.Cause.Detail, "raw_rollout") || strings.Contains(terminal.Cause.Detail, "must not") {
		t.Fatalf("terminal cause detail leaked untrusted diagnostics: %q", terminal.Cause.Detail)
	}
	if err := terminal.Validate(); err != nil {
		t.Fatalf("terminal.Validate() = %v, want valid bounded terminal result", err)
	}
}

func TestClassifyTerminal_IncompleteOutputUsesStructuredCompletionFacts(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name           string
		classification string
		putInMetadata  bool
	}{
		{name: "missing completion evidence", classification: "missing_completion_evidence", putInMetadata: true},
		{name: "missing required output", classification: "missing_required_output"},
		{name: "contradictory completion", classification: "contradictory_completion"},
	}
	for _, test := range cases {
		test := test
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()

			diagnostics := &workers.WorkDiagnostics{}
			if test.putInMetadata {
				diagnostics.Metadata = map[string]string{
					"failure_operation":      "completion_validation",
					"failure_classification": test.classification,
				}
			} else {
				diagnostics.Provider = &workers.ProviderDiagnostic{
					ResponseMetadata: map[string]string{
						"failure_operation":      "completion_validation",
						"failure_classification": test.classification,
					},
				}
			}
			terminal := classifyTerminal(nil, workers.WorkstationDispatchResult{Result: workers.WorkResult{
				Outcome:     workers.OutcomeFailed,
				Error:       "raw prompt=secret provider transcript must not be exposed",
				Diagnostics: diagnostics,
			}})

			if terminal.Outcome != workersessions.TerminalOutcomeFailed || terminal.Cause == nil {
				t.Fatalf("terminal = %#v, want failed result with cause", terminal)
			}
			if terminal.Cause.Kind != workersessions.FailureCauseIncompleteOutput {
				t.Fatalf("terminal cause kind = %q, want INCOMPLETE_OUTPUT", terminal.Cause.Kind)
			}
			if strings.Contains(terminal.Cause.Detail, "secret") || !strings.Contains(terminal.Cause.Detail, "completion_validation") {
				t.Fatalf("terminal cause detail = %q, want safe completion-validation context", terminal.Cause.Detail)
			}
			if err := terminal.Validate(); err != nil {
				t.Fatalf("terminal.Validate() = %v, want nil", err)
			}
		})
	}
}

func TestTerminalDraft_IncompleteOutputPreservesBoundedFailureKind(t *testing.T) {
	draft, err := terminalDraft(
		workersessions.StateFailed,
		workersessions.TerminalResult{
			Outcome: workersessions.TerminalOutcomeFailed,
			Cause: &workersessions.FailureCause{
				Kind:   workersessions.FailureCauseIncompleteOutput,
				Detail: genericFailureDetail[workersessions.FailureCauseIncompleteOutput],
			},
		},
		"dispatch-incomplete-output",
	)
	if err != nil {
		t.Fatalf("terminalDraft() error = %v, want nil", err)
	}
	var payload terminalSessionPayload
	if err := json.Unmarshal(draft.Payload, &payload); err != nil {
		t.Fatalf("terminal payload decode error = %v", err)
	}
	if payload.FailureCause != string(workersessions.FailureCauseIncompleteOutput) {
		t.Fatalf("terminal payload failureCause = %q, want INCOMPLETE_OUTPUT", payload.FailureCause)
	}
}

func TestClassifyTerminal_CleanRejectedWorkResultUsesBoundedRejectionCause(t *testing.T) {
	terminal := classifyTerminal(nil, workers.WorkstationDispatchResult{
		TerminalOutcome: workers.WorkstationDispatchTerminalOutcomeCompleted,
		Result: workers.WorkResult{
			Outcome:  workers.OutcomeRejected,
			Feedback: "raw reviewer feedback remains on the Work result",
		},
	})
	if terminal.Outcome != workersessions.TerminalOutcomeFailed || terminal.Cause == nil {
		t.Fatalf("terminal = %#v, want failed Worker Session projection with cause", terminal)
	}
	if terminal.Cause.Kind != workersessions.FailureCauseRejected {
		t.Fatalf("terminal cause kind = %q, want REJECTED", terminal.Cause.Kind)
	}
	if terminal.Cause.Detail != genericFailureDetail[workersessions.FailureCauseRejected] {
		t.Fatalf("terminal cause detail = %q, want bounded generic rejection detail", terminal.Cause.Detail)
	}
	if strings.Contains(terminal.Cause.Detail, "raw reviewer feedback") {
		t.Fatalf("terminal cause detail leaked reviewer feedback: %q", terminal.Cause.Detail)
	}
	if err := terminal.Validate(); err != nil {
		t.Fatalf("terminal.Validate() = %v, want nil", err)
	}
}

func TestClassifyTerminal_RejectedWithFailedDispatchFallsThroughToExecutionFailure(t *testing.T) {
	terminal := classifyTerminal(nil, workers.WorkstationDispatchResult{
		TerminalOutcome: workers.WorkstationDispatchTerminalOutcomeFailed,
		Result:          workers.WorkResult{Outcome: workers.OutcomeRejected},
	})
	if terminal.Cause == nil || terminal.Cause.Kind != workersessions.FailureCauseWorkersExecutionFailure {
		t.Fatalf("terminal cause = %#v, want WORKERS_EXECUTION_FAILURE", terminal.Cause)
	}
}

func TestClassifyTerminal_UnknownCompletionClassificationRemainsExecutionFailure(t *testing.T) {
	terminal := classifyTerminal(nil, workers.WorkstationDispatchResult{Result: workers.WorkResult{
		Outcome: workers.OutcomeFailed,
		Diagnostics: &workers.WorkDiagnostics{Provider: &workers.ProviderDiagnostic{ResponseMetadata: map[string]string{
			workers.ProviderResponseMetadataFailureOperation:      "completion_validation",
			workers.ProviderResponseMetadataFailureClassification: "unrecognized_completion_fact",
		}}},
	}})
	if terminal.Cause == nil || terminal.Cause.Kind != workersessions.FailureCauseWorkersExecutionFailure {
		t.Fatalf("terminal cause = %#v, want WORKERS_EXECUTION_FAILURE", terminal.Cause)
	}
}

func TestClassifyTerminal_SuccessWithFailedDispatchUsesExplicitFailureCause(t *testing.T) {
	terminal := classifyTerminal(nil, workers.WorkstationDispatchResult{
		TerminalOutcome: workers.WorkstationDispatchTerminalOutcomeFailed,
		Result: workers.WorkResult{
			Outcome: workers.OutcomeAccepted,
		},
	})
	if terminal.Outcome != workersessions.TerminalOutcomeFailed || terminal.Cause == nil {
		t.Fatalf("terminal = %#v, want failed result with cause", terminal)
	}
	if terminal.Cause.Kind != workersessions.FailureCauseAdapterFailure {
		t.Fatalf("terminal cause kind = %q, want ADAPTER_FAILURE", terminal.Cause.Kind)
	}
	if terminal.Cause.Detail != "the dispatch reported failure after a successful Workers result" {
		t.Fatalf("terminal cause detail = %q, want explicit contradictory-evidence cause", terminal.Cause.Detail)
	}
}

// TestSafeDetail_WithUnrecognizedTypeValue_FallsBackToGenericPlaceholder
// proves the exact review concern: WorkFailureType is an exported Go string
// type, not a runtime-validated enum, so any string (including
// attacker-controlled prompt/command/credential text) can be constructed and
// attached as WorkFailureMetadata.Type. A value outside the whitelisted
// constants must never be echoed into Detail.
func TestSafeDetail_WithUnrecognizedTypeValue_FallsBackToGenericPlaceholder(t *testing.T) {
	metadata := &workers.WorkFailureMetadata{
		Family: workers.WorkFailureFamilyTerminal,
		Type:   workers.WorkFailureType("codex exec summarize confidential acquisition memo"),
	}
	got := safeDetail(workersessions.FailureCauseWorkersExecutionFailure, metadata)
	want := genericFailureDetail[workersessions.FailureCauseWorkersExecutionFailure]
	if got != want {
		t.Fatalf("safeDetail() = %q, want fixed generic placeholder %q (unrecognized Type must never be echoed)", got, want)
	}
	if strings.Contains(got, "confidential") {
		t.Fatalf("safeDetail() leaked unrecognized Type text: %q", got)
	}
}

// TestSafeDetail_WithUnrecognizedFamilyValue_FallsBackToGenericPlaceholder
// mirrors TestSafeDetail_WithUnrecognizedTypeValue_FallsBackToGenericPlaceholder
// for WorkFailureMetadata.Family.
func TestSafeDetail_WithUnrecognizedFamilyValue_FallsBackToGenericPlaceholder(t *testing.T) {
	metadata := &workers.WorkFailureMetadata{
		Family: workers.WorkFailureFamily("password=hunter2"),
		Type:   workers.WorkFailureTypeTimeout,
	}
	got := safeDetail(workersessions.FailureCauseWorkersExecutionFailure, metadata)
	want := genericFailureDetail[workersessions.FailureCauseWorkersExecutionFailure]
	if got != want {
		t.Fatalf("safeDetail() = %q, want fixed generic placeholder %q (unrecognized Family must never be echoed)", got, want)
	}
	if strings.Contains(got, "hunter2") {
		t.Fatalf("safeDetail() leaked unrecognized Family text: %q", got)
	}
}

// TestClassifyTerminal_UnrecognizedFailureMetadataWithSensitiveText_NeverExposesItInDetail
// proves the review's exact end-to-end scenario through classifyTerminal:
// a WorkResult carrying an unrecognized (attacker-controlled-shaped)
// WorkFailureMetadata.Type never surfaces that text in the committed
// FailureCause.Detail.
func TestClassifyTerminal_UnrecognizedFailureMetadataWithSensitiveText_NeverExposesItInDetail(t *testing.T) {
	dispatchResult := workers.WorkstationDispatchResult{
		Result: workers.WorkResult{
			Outcome: workers.OutcomeFailed,
			FailureMetadata: &workers.WorkFailureMetadata{
				Type: workers.WorkFailureType("codex exec summarize confidential acquisition memo"),
			},
		},
	}

	terminal := classifyTerminal(nil, dispatchResult)

	if terminal.Cause == nil {
		t.Fatal("terminal cause = nil, want non-nil")
	}
	if strings.Contains(terminal.Cause.Detail, "confidential") {
		t.Fatalf("terminal cause detail leaked unrecognized metadata text: %q", terminal.Cause.Detail)
	}
	want := genericFailureDetail[workersessions.FailureCauseWorkersExecutionFailure]
	if terminal.Cause.Detail != want {
		t.Fatalf("terminal cause detail = %q, want fixed generic placeholder %q", terminal.Cause.Detail, want)
	}
}

// TestClassifyTerminal_FailureMetadataPresentAlongsideSensitiveRawText_UsesOnlyMetadata
// proves classifyTerminal's Detail comes exclusively from the closed-
// vocabulary FailureMetadata even when a sensitive/free-form WorkResult.Error
// is also present: the raw text is never consulted for Detail, so it cannot
// leak regardless of its content.
func TestClassifyTerminal_FailureMetadataPresentAlongsideSensitiveRawText_UsesOnlyMetadata(t *testing.T) {
	dispatchResult := workers.WorkstationDispatchResult{
		Result: workers.WorkResult{
			Outcome: workers.OutcomeFailed,
			Error:   "password=hunter2 while running the confidential board memo prompt",
			FailureMetadata: &workers.WorkFailureMetadata{
				Family: workers.WorkFailureFamilyTerminal,
				Type:   workers.WorkFailureTypeAuthFailure,
			},
		},
	}

	terminal := classifyTerminal(nil, dispatchResult)

	if terminal.Cause == nil {
		t.Fatal("terminal cause = nil, want non-nil")
	}
	want := "family=terminal type=auth_failure"
	if terminal.Cause.Detail != want {
		t.Fatalf("terminal cause detail = %q, want %q", terminal.Cause.Detail, want)
	}
	if strings.Contains(terminal.Cause.Detail, "hunter2") || strings.Contains(terminal.Cause.Detail, "confidential") {
		t.Fatalf("terminal cause detail leaked raw text: %q", terminal.Cause.Detail)
	}
}

func TestClassifyTerminal_PreservesOnlyBoundedAgentRunProviderAndHarnessClasses(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name  string
		class string
		want  string
	}{
		{name: "provider", class: workers.AgentRunFailureClassProvider, want: workers.AgentRunFailureClassProvider},
		{name: "harness", class: workers.AgentRunFailureClassHarness, want: workers.AgentRunFailureClassHarness},
		{name: "unknown class", class: "agent_run_failure_with_secret", want: ""},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()

			terminal := classifyTerminal(nil, workers.WorkstationDispatchResult{Result: workers.WorkResult{
				Outcome: workers.OutcomeFailed,
				Diagnostics: &workers.WorkDiagnostics{Metadata: map[string]string{
					workers.AgentRunMetadataExecutionBehavior: workers.AgentRunExecutionBehavior,
					workers.AgentRunMetadataFailureClass:      test.class,
				}},
			}})
			if terminal.Cause == nil {
				t.Fatal("terminal cause = nil, want failed cause")
			}
			if terminal.Cause.Kind != workersessions.FailureCauseWorkersExecutionFailure {
				t.Fatalf("terminal cause kind = %q, want generic Workers execution failure", terminal.Cause.Kind)
			}
			if terminal.Cause.AgentRunFailureClass != test.want {
				t.Fatalf("agent-run failure class = %q, want %q", terminal.Cause.AgentRunFailureClass, test.want)
			}
			if err := terminal.Cause.Validate(); err != nil {
				t.Fatalf("terminal cause validation = %v, want nil", err)
			}
		})
	}
}

// TestClassifyTerminal_ExecutorPanicWithSensitiveRawEvidence_ClassifiesCorrectlyWithoutLeakingDetail
// proves executor-panic classification still works from raw evidence text
// (isExecutorPanicEvidence legitimately inspects it), while Detail itself
// never reproduces any part of that raw text.
func TestClassifyTerminal_ExecutorPanicWithSensitiveRawEvidence_ClassifiesCorrectlyWithoutLeakingDetail(t *testing.T) {
	dispatchResult := workers.WorkstationDispatchResult{
		Result: workers.WorkResult{
			Outcome: workers.OutcomeFailed,
			Error:   "executor panic: authorization Bearer sk-live-abc123 rejected",
		},
	}

	terminal := classifyTerminal(nil, dispatchResult)

	if terminal.Outcome != workersessions.TerminalOutcomeFailed {
		t.Fatalf("terminal outcome = %q, want FAILED", terminal.Outcome)
	}
	if terminal.Cause == nil || terminal.Cause.Kind != workersessions.FailureCauseExecutorPanic {
		t.Fatalf("terminal cause = %+v, want Kind EXECUTOR_PANIC", terminal.Cause)
	}
	if strings.Contains(terminal.Cause.Detail, "sk-live-abc123") {
		t.Fatalf("terminal cause detail leaked secret: %q", terminal.Cause.Detail)
	}
	if terminal.Cause.Detail != genericFailureDetail[workersessions.FailureCauseExecutorPanic] {
		t.Fatalf("terminal cause detail = %q, want fixed generic placeholder %q", terminal.Cause.Detail, genericFailureDetail[workersessions.FailureCauseExecutorPanic])
	}
}

func TestClassifyTerminal_StartFailureWithSensitiveAdapterError_NeverAttachesRawText(t *testing.T) {
	dispatchErr := errors.New("dial tcp failed: password=hunter2")

	terminal := classifyTerminal(dispatchErr, workers.WorkstationDispatchResult{})

	if terminal.Cause == nil {
		t.Fatal("terminal cause = nil, want non-nil")
	}
	if strings.Contains(terminal.Cause.Detail, "hunter2") {
		t.Fatalf("terminal cause detail leaked secret: %q", terminal.Cause.Detail)
	}
	if terminal.Cause.Detail != genericFailureDetail[workersessions.FailureCauseStartFailure] {
		t.Fatalf("terminal cause detail = %q, want fixed generic placeholder %q", terminal.Cause.Detail, genericFailureDetail[workersessions.FailureCauseStartFailure])
	}
}

// TestTerminalDraft_Completed_MapsToPhaseCompletedWithStatusOnly proves the
// pure COMPLETED mapping: no FailureCause exists, so the payload carries only
// the status.
func TestTerminalDraft_Completed_MapsToPhaseCompletedWithStatusOnly(t *testing.T) {
	draft, err := terminalDraft(workersessions.StateCompleted, workersessions.TerminalResult{Outcome: workersessions.TerminalOutcomeCompleted}, "dispatch-1")
	if err != nil {
		t.Fatalf("terminalDraft() error = %v, want nil", err)
	}
	if draft.Kind != workers.KindSession || draft.Phase != workers.PhaseCompleted {
		t.Fatalf("terminalDraft() = %+v, want Kind=SESSION Phase=COMPLETED", draft)
	}
	if err := workers.ValidateDraft(draft); err != nil {
		t.Fatalf("workers.ValidateDraft(draft) error = %v, want nil", err)
	}

	var payload terminalSessionPayload
	if err := json.Unmarshal(draft.Payload, &payload); err != nil {
		t.Fatalf("unmarshal payload error = %v", err)
	}
	if payload.Status != string(workersessions.StateCompleted) {
		t.Fatalf("payload.Status = %q, want %q", payload.Status, workersessions.StateCompleted)
	}
	if payload.FailureCause != "" || payload.FailureDetail != "" {
		t.Fatalf("payload = %+v, want no failure fields on a COMPLETED terminal record", payload)
	}

	var sessionPayload workers.SessionPayload
	if err := json.Unmarshal(draft.Payload, &sessionPayload); err != nil {
		t.Fatalf("unmarshal into workers.SessionPayload error = %v, want the terminal payload to remain a valid SessionPayload", err)
	}
	if sessionPayload.Status != string(workersessions.StateCompleted) {
		t.Fatalf("workers.SessionPayload.Status = %q, want %q", sessionPayload.Status, workersessions.StateCompleted)
	}
}

// TestTerminalDraft_Failed_MapsToPhaseFailedPreservingCauseAndSafeDetail
// proves the FAILED mapping preserves the already-computed typed
// FailureCause classification and its bounded safe Detail in the payload.
func TestTerminalDraft_Failed_MapsToPhaseFailedPreservingCauseAndSafeDetail(t *testing.T) {
	result := workersessions.TerminalResult{
		Outcome: workersessions.TerminalOutcomeFailed,
		Cause: &workersessions.FailureCause{
			Kind:   workersessions.FailureCauseExecutorPanic,
			Detail: safeDetail(workersessions.FailureCauseExecutorPanic, nil),
		},
	}
	draft, err := terminalDraft(workersessions.StateFailed, result, "dispatch-1")
	if err != nil {
		t.Fatalf("terminalDraft() error = %v, want nil", err)
	}
	if draft.Kind != workers.KindSession || draft.Phase != workers.PhaseFailed {
		t.Fatalf("terminalDraft() = %+v, want Kind=SESSION Phase=FAILED", draft)
	}
	if err := workers.ValidateDraft(draft); err != nil {
		t.Fatalf("workers.ValidateDraft(draft) error = %v, want nil", err)
	}

	var payload terminalSessionPayload
	if err := json.Unmarshal(draft.Payload, &payload); err != nil {
		t.Fatalf("unmarshal payload error = %v", err)
	}
	if payload.Status != string(workersessions.StateFailed) {
		t.Fatalf("payload.Status = %q, want %q", payload.Status, workersessions.StateFailed)
	}
	if payload.FailureCause != string(workersessions.FailureCauseExecutorPanic) {
		t.Fatalf("payload.FailureCause = %q, want %q", payload.FailureCause, workersessions.FailureCauseExecutorPanic)
	}
	if payload.FailureDetail != result.Cause.Detail {
		t.Fatalf("payload.FailureDetail = %q, want %q", payload.FailureDetail, result.Cause.Detail)
	}
}

// TestTerminalDraft_CanceledAndTerminated_ShareExistingPhaseCanceledButPreserveDistinctStatus
// proves the pure mapping the W3 scope note requires for CANCELED/TERMINATED
// (neither reachable through Start until W6 adds controls): both project
// through the same existing PhaseCanceled, with the distinct originating
// state preserved as the payload Status so a consumer can still tell them
// apart.
func TestTerminalDraft_CanceledAndTerminated_ShareExistingPhaseCanceledButPreserveDistinctStatus(t *testing.T) {
	for _, state := range []workersessions.State{workersessions.StateCanceled, workersessions.StateTerminated} {
		t.Run(string(state), func(t *testing.T) {
			draft, err := terminalDraft(state, workersessions.TerminalResult{Outcome: workersessions.TerminalOutcomeCompleted}, "dispatch-1")
			if err != nil {
				t.Fatalf("terminalDraft() error = %v, want nil", err)
			}
			if draft.Kind != workers.KindSession || draft.Phase != workers.PhaseCanceled {
				t.Fatalf("terminalDraft() = %+v, want Kind=SESSION Phase=CANCELED", draft)
			}
			if err := workers.ValidateDraft(draft); err != nil {
				t.Fatalf("workers.ValidateDraft(draft) error = %v, want nil", err)
			}

			var payload terminalSessionPayload
			if err := json.Unmarshal(draft.Payload, &payload); err != nil {
				t.Fatalf("unmarshal payload error = %v", err)
			}
			if payload.Status != string(state) {
				t.Fatalf("payload.Status = %q, want %q", payload.Status, state)
			}
		})
	}
}

// TestTerminalDraft_NonTerminalState_ReturnsErrorAndNoDraft proves
// terminalPhase/terminalDraft refuse to fabricate a terminal projection for a
// state that is not one of the four absorbing terminal states.
func TestTerminalDraft_NonTerminalState_ReturnsErrorAndNoDraft(t *testing.T) {
	for _, state := range []workersessions.State{
		workersessions.StateReserved,
		workersessions.StateStarting,
		workersessions.StateRunning,
		workersessions.StatePaused,
	} {
		t.Run(string(state), func(t *testing.T) {
			if _, err := terminalDraft(state, workersessions.TerminalResult{Outcome: workersessions.TerminalOutcomeCompleted}, "dispatch-1"); err == nil {
				t.Fatalf("terminalDraft(%q) error = nil, want a non-nil error", state)
			}
		})
	}
}

// TestPublishTerminalRecord_NonTerminalState_PropagatesTerminalDraftErrorAndAppendsNothing
// proves publishTerminalRecord propagates terminalDraft's error unchanged and
// never reaches appendDraft/Events for a state with no terminal projection.
// Start itself can never pass such a state (see terminalPhase), so this
// drives the registry method directly, the same way
// TestTerminalDraft_NonTerminalState_ReturnsErrorAndNoDraft exercises the
// pure mapping directly.
func TestPublishTerminalRecord_NonTerminalState_PropagatesTerminalDraftErrorAndAppendsNothing(t *testing.T) {
	r := newTestRegistry(t)

	err := r.publishTerminalRecord(context.Background(), "worker-1", "dispatch-1", workersessions.StateReserved, workersessions.TerminalResult{Outcome: workersessions.TerminalOutcomeCompleted})
	if err == nil {
		t.Fatal("publishTerminalRecord() error = nil, want a non-nil error for a non-terminal state")
	}
}

// TestPublishRecord_RejectsPublicationForMerelyReservedSession proves the
// review-flagged defect is fixed at the lowest level: a session that has
// only ever been reserved (never Started, so its opening record was never
// committed) must reject PublishRecord rather than accepting output for a
// publication window that was never opened.
func TestPublishRecord_RejectsPublicationForMerelyReservedSession(t *testing.T) {
	r := newTestRegistry(t)
	r.reserveIfAbsent("worker-1")

	_, err := r.PublishRecord(context.Background(), workersessions.PublishRecordRequest{
		SessionID:      "worker-1",
		Draft:          workers.Draft{Kind: workers.KindProgress, Phase: workers.PhaseUpdated, Payload: []byte(`{"label":"x"}`)},
		SourceType:     "worker_provider",
		SourceID:       "worker-1",
		SourceSequence: 1,
		SourceEventID:  "evt-1",
		SchemaID:       "workers.draft.v1",
	})
	if !errors.Is(err, workersessions.ErrPublicationNotOpen) {
		t.Fatalf("PublishRecord() for a merely reserved session error = %v, want ErrPublicationNotOpen", err)
	}
}

func TestResume_RejectsMissingOrMalformedAssociationBeforeContinuationHandoff(t *testing.T) {
	tests := []struct {
		name        string
		association *workersessions.ProviderSessionAssociation
		wantErr     error
	}{
		{
			name:    "missing association",
			wantErr: workersessions.ErrProviderSessionAssociationMissing,
		},
		{
			name: "malformed association",
			association: &workersessions.ProviderSessionAssociation{
				WorkerSessionID: "worker-1",
				DispatchID:      "dispatch-1",
				AttemptID:       "dispatch-1",
				Reference: providers.SessionRef{
					Provider: providers.IDCodex,
					Kind:     providers.SessionIDKind,
				},
			},
			wantErr: workersessions.ErrInvalidProviderSessionAssociation,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			session := workersessions.Session{
				ID:                         "worker-1",
				State:                      workersessions.StatePaused,
				ProviderSessionAssociation: test.association,
			}
			registry := &registry{
				sessions: map[string]workersessions.Session{"worker-1": session},
				supervisions: map[string]*supervision{
					"worker-1": newSupervision("dispatch-1", "turn-1"),
				},
				logger: logging.NoopLogger{},
			}

			result, err := registry.Resume(context.Background(), workersessions.ControlRequest{ID: "worker-1"})
			if !errors.Is(err, test.wantErr) || result.Outcome != workersessions.ControlOutcomeFailed ||
				result.Session.State != workersessions.StatePaused {
				t.Fatalf("Resume() = %#v, %v, want failed PAUSED result with %v", result, err, test.wantErr)
			}
			if test.name == "malformed association" && !errors.Is(err, providers.ErrInvalidSessionRef) {
				t.Fatalf("Resume() malformed association error = %v, want Providers ErrInvalidSessionRef", err)
			}
			if current := registry.sessions["worker-1"]; !reflect.DeepEqual(current, session) {
				t.Fatalf("Resume() mutated rejected association: got %#v, want %#v", current, session)
			}
		})
	}
}

func newPausedContinuationRegistry(t *testing.T) (*registry, *supervision, providers.SessionRef) {
	t.Helper()
	r := newTestRegistry(t)
	reference := providers.SessionRef{
		Provider: providers.IDCodex,
		Kind:     providers.SessionIDKind,
		ID:       "provider-session-1",
	}
	supervision := newSupervision("dispatch-1", "turn-1")
	supervision.accepted = true
	r.sessions["worker-1"] = workersessions.Session{
		ID:    "worker-1",
		State: workersessions.StatePaused,
		ProviderSessionAssociation: &workersessions.ProviderSessionAssociation{
			WorkerSessionID: "worker-1",
			TurnID:          "turn-1",
			DispatchID:      "dispatch-1",
			AttemptID:       "dispatch-1",
			Reference:       reference,
		},
	}
	r.supervisions["worker-1"] = supervision
	return r, supervision, reference
}

func newRunningPauseRegistry(t *testing.T) (*registry, *supervision) {
	t.Helper()
	r, supervision, _ := newPausedContinuationRegistry(t)
	session := r.sessions["worker-1"]
	session.State = workersessions.StateRunning
	r.sessions["worker-1"] = session
	return r, supervision
}

// pkgmaintcheck:ignore-function-lines pre-existing baseline debt recorded 2026-08-08; refactor this code below the maintainability threshold and remove this exemption
// pkgmaintcheck:ignore-cyclomatic-complexity pre-existing baseline debt recorded 2026-08-08; refactor this code below the maintainability threshold and remove this exemption
func TestContinuationControl_GuardsPreserveThePausedSession(t *testing.T) {
	t.Run("transition to paused rejects a non-running session", func(t *testing.T) {
		r, _, _ := newPausedContinuationRegistry(t)
		if r.transitionToPaused("worker-1") {
			t.Fatal("transitionToPaused() = true for an already PAUSED session, want false")
		}
	})

	t.Run("association validation keeps provider identity errors typed", func(t *testing.T) {
		if err := validateResumeAssociation(workersessions.Session{}); !errors.Is(err, workersessions.ErrProviderSessionAssociationMissing) {
			t.Fatalf("validateResumeAssociation(nil) = %v, want ErrProviderSessionAssociationMissing", err)
		}
		malformed := workersessions.Session{
			ID:    "worker-1",
			State: workersessions.StatePaused,
			ProviderSessionAssociation: &workersessions.ProviderSessionAssociation{
				WorkerSessionID: "worker-1",
				DispatchID:      "dispatch-1",
				AttemptID:       "dispatch-1",
				Reference:       providers.SessionRef{Provider: providers.IDCodex, Kind: providers.SessionIDKind},
			},
		}
		if err := validateResumeAssociation(malformed); !errors.Is(err, workersessions.ErrInvalidProviderSessionAssociation) || !errors.Is(err, providers.ErrInvalidSessionRef) {
			t.Fatalf("validateResumeAssociation(malformed) = %v, want typed association and provider errors", err)
		}
		mismatched := malformed
		associationCopy := malformed.ProviderSessionAssociation.Clone()
		mismatched.ProviderSessionAssociation = &associationCopy
		mismatched.ProviderSessionAssociation.Reference.ID = "provider-session-1"
		mismatched.ProviderSessionAssociation.WorkerSessionID = "worker-2"
		if err := validateResumeAssociation(mismatched); !errors.Is(err, workersessions.ErrInvalidProviderSessionAssociation) {
			t.Fatalf("validateResumeAssociation(mismatched) = %v, want ErrInvalidProviderSessionAssociation", err)
		}
	})

	t.Run("preparation rejects stale state and competing continuation", func(t *testing.T) {
		r, supervision, reference := newPausedContinuationRegistry(t)
		if _, _, prepared := r.prepareContinuation("missing", supervision, reference); prepared {
			t.Fatal("prepareContinuation() prepared a missing session")
		}
		other := reference
		other.ID = "provider-session-other"
		if _, _, prepared := r.prepareContinuation("worker-1", supervision, other); prepared {
			t.Fatal("prepareContinuation() prepared a mismatched provider session")
		}
		supervision.continuing = true
		if _, _, prepared := r.prepareContinuation("worker-1", supervision, reference); prepared {
			t.Fatal("prepareContinuation() prepared while a continuation was already in flight")
		}
		supervision.continuing = false

		continuation, previousDispatchID, prepared := r.prepareContinuation("worker-1", supervision, reference)
		if !prepared || previousDispatchID != "dispatch-1" || continuation.Execution.ResumeSession == nil || *continuation.Execution.ResumeSession != reference {
			t.Fatalf("prepareContinuation() = %#v, %q, %t, want a detached exact continuation", continuation, previousDispatchID, prepared)
		}
		r.revertContinuation("worker-1", supervision, previousDispatchID)
		current, err := r.Get(context.Background(), workersessions.GetRequest{ID: "worker-1"})
		if err != nil || current.State != workersessions.StatePaused {
			t.Fatalf("Get() after revertContinuation = %#v, %v, want PAUSED", current, err)
		}
		supervision.mu.Lock()
		defer supervision.mu.Unlock()
		if supervision.dispatchID != "dispatch-1" || supervision.publishing || supervision.continuing || !supervision.accepted {
			t.Fatalf("revertContinuation supervision = %#v, want restored admitted dispatch", supervision)
		}
	})

	t.Run("resume rejects invalid and missing identities", func(t *testing.T) {
		r := newTestRegistry(t)
		if _, err := r.Resume(context.Background(), workersessions.ControlRequest{ID: " "}); !errors.Is(err, workersessions.ErrInvalidSessionID) {
			t.Fatalf("Resume(blank) = %v, want ErrInvalidSessionID", err)
		}
		if _, err := r.Resume(context.Background(), workersessions.ControlRequest{ID: "missing"}); !errors.Is(err, workersessions.ErrSessionNotFound) {
			t.Fatalf("Resume(missing) = %v, want ErrSessionNotFound", err)
		}
	})

	t.Run("resume is a no-op while continuation is publishing", func(t *testing.T) {
		r, supervision, _ := newPausedContinuationRegistry(t)
		supervision.continuing = true
		supervision.publishing = true
		result, err := r.Resume(context.Background(), workersessions.ControlRequest{ID: "worker-1"})
		if err != nil || result.Outcome != workersessions.ControlOutcomeNoop || result.Session.State != workersessions.StatePaused {
			t.Fatalf("Resume() = %#v, %v, want PAUSED NOOP", result, err)
		}
	})

	t.Run("resume is a no-op when preparation loses its race", func(t *testing.T) {
		r, supervision, _ := newPausedContinuationRegistry(t)
		supervision.continuing = true
		supervision.accepted = false
		result, err := r.Resume(context.Background(), workersessions.ControlRequest{ID: "worker-1"})
		if err != nil || result.Outcome != workersessions.ControlOutcomeNoop || result.Session.State != workersessions.StatePaused {
			t.Fatalf("Resume() = %#v, %v, want PAUSED NOOP", result, err)
		}
	})

	t.Run("publication failure restores the exact paused continuation", func(t *testing.T) {
		r, supervision, _ := newPausedContinuationRegistry(t)
		publishErr := errors.New("continuation publish failed")
		r.boundary = failingPublishBoundary{unusedExecution: unusedExecution{t: t}, err: publishErr}
		result, err := r.Resume(context.Background(), workersessions.ControlRequest{ID: "worker-1"})
		if !errors.Is(err, publishErr) || result.Outcome != workersessions.ControlOutcomeFailed || result.DispatchID != "dispatch-1/resume/1" {
			t.Fatalf("Resume() = %#v, %v, want failed fresh continuation dispatch", result, err)
		}
		if result.Session.State != workersessions.StatePaused || supervision.dispatchID != "dispatch-1" {
			t.Fatalf("Resume() after publish failure = %#v with dispatch %q, want restored PAUSED dispatch-1", result, supervision.dispatchID)
		}
	})

	t.Run("unsupported control distinguishes invalid missing and terminal sessions", func(t *testing.T) {
		r := newTestRegistry(t)
		if _, err := r.unsupportedControl(context.Background(), workersessions.ControlRequest{ID: " "}, workersessions.ControlActionPause); !errors.Is(err, workersessions.ErrInvalidSessionID) {
			t.Fatalf("unsupportedControl(blank) = %v, want ErrInvalidSessionID", err)
		}
		if _, err := r.unsupportedControl(context.Background(), workersessions.ControlRequest{ID: "missing"}, workersessions.ControlActionPause); !errors.Is(err, workersessions.ErrSessionNotFound) {
			t.Fatalf("unsupportedControl(missing) = %v, want ErrSessionNotFound", err)
		}
		r.sessions["terminal"] = workersessions.Session{ID: "terminal", State: workersessions.StateCanceled}
		result, err := r.unsupportedControl(context.Background(), workersessions.ControlRequest{ID: "terminal"}, workersessions.ControlActionPause)
		if err != nil || result.Outcome != workersessions.ControlOutcomeNoop {
			t.Fatalf("unsupportedControl(terminal) = %#v, %v, want NOOP", result, err)
		}
	})
}

// pkgmaintcheck:ignore-function-lines pre-existing baseline debt recorded 2026-08-08; refactor this code below the maintainability threshold and remove this exemption
// pkgmaintcheck:ignore-cyclomatic-complexity pre-existing baseline debt recorded 2026-08-08; refactor this code below the maintainability threshold and remove this exemption
func TestPause_ControlOutcomesKeepTheLifecycleTruthful(t *testing.T) {
	t.Run("invalid and missing identities fail without a boundary effect", func(t *testing.T) {
		r := newTestRegistry(t)
		if _, err := r.Pause(context.Background(), workersessions.ControlRequest{ID: " "}); !errors.Is(err, workersessions.ErrInvalidSessionID) {
			t.Fatalf("Pause(blank) = %v, want ErrInvalidSessionID", err)
		}
		if _, err := r.Pause(context.Background(), workersessions.ControlRequest{ID: "missing"}); !errors.Is(err, workersessions.ErrSessionNotFound) {
			t.Fatalf("Pause(missing) = %v, want ErrSessionNotFound", err)
		}
	})

	t.Run("duplicate and pre-admission pause requests do not fabricate paused state", func(t *testing.T) {
		r, supervision := newRunningPauseRegistry(t)
		supervision.controlAction = workersessions.ControlActionCancel
		result, err := r.Pause(context.Background(), workersessions.ControlRequest{ID: "worker-1"})
		if err != nil || result.Outcome != workersessions.ControlOutcomeNoop || result.Session.State != workersessions.StateRunning {
			t.Fatalf("duplicate Pause() = %#v, %v, want RUNNING NOOP", result, err)
		}

		r, supervision = newRunningPauseRegistry(t)
		supervision.accepted = false
		result, err = r.Pause(context.Background(), workersessions.ControlRequest{ID: "worker-1"})
		if err != nil || result.Outcome != workersessions.ControlOutcomeUnsupported || result.Session.State != workersessions.StateRunning {
			t.Fatalf("pre-admission Pause() = %#v, %v, want RUNNING UNSUPPORTED", result, err)
		}
	})

	t.Run("boundary error returns a failed control without changing the session", func(t *testing.T) {
		r, _ := newRunningPauseRegistry(t)
		boundaryErr := errors.New("cancel boundary failed")
		r.boundary = cancellationResultBoundary{unusedExecution: unusedExecution{t: t}, err: boundaryErr}
		result, err := r.Pause(context.Background(), workersessions.ControlRequest{ID: "worker-1"})
		if !errors.Is(err, boundaryErr) || result.Outcome != workersessions.ControlOutcomeFailed || result.Session.State != workersessions.StateRunning {
			t.Fatalf("Pause() = %#v, %v, want failed RUNNING result", result, err)
		}
	})

	t.Run("already-terminal cancellation is a no-op", func(t *testing.T) {
		r, supervision := newRunningPauseRegistry(t)
		supervision.signalDone()
		r.boundary = cancellationResultBoundary{
			unusedExecution: unusedExecution{t: t},
			result:          workers.WorkstationDispatchCancelResult{DispatchID: "dispatch-1", Outcome: workers.WorkstationDispatchCancelOutcomeAlreadyCanceled},
		}
		result, err := r.Pause(context.Background(), workersessions.ControlRequest{ID: "worker-1"})
		if err != nil || result.Outcome != workersessions.ControlOutcomeNoop || result.Session.State != workersessions.StateRunning {
			t.Fatalf("Pause() = %#v, %v, want RUNNING NOOP", result, err)
		}
	})

	t.Run("cancellation without a paused callback remains a no-op", func(t *testing.T) {
		r, supervision := newRunningPauseRegistry(t)
		supervision.signalDone()
		r.boundary = cancellationResultBoundary{
			unusedExecution: unusedExecution{t: t},
			result:          workers.WorkstationDispatchCancelResult{DispatchID: "dispatch-1", Outcome: workers.WorkstationDispatchCancelOutcomeCanceled},
		}
		result, err := r.Pause(context.Background(), workersessions.ControlRequest{ID: "worker-1"})
		if err != nil || result.Outcome != workersessions.ControlOutcomeNoop || result.Session.State != workersessions.StateRunning {
			t.Fatalf("Pause() = %#v, %v, want RUNNING NOOP", result, err)
		}
	})

	t.Run("pause waits for an already active control before reading the terminal snapshot", func(t *testing.T) {
		r, supervision := newRunningPauseRegistry(t)
		supervision.mu.Lock()
		supervision.controlActive = true
		supervision.controlDone = make(chan struct{})
		wait := supervision.controlDone
		supervision.mu.Unlock()

		observedWait := make(chan struct{})
		go func() {
			wait <- struct{}{}
			close(observedWait)
		}()

		resultCh := make(chan workersessions.ControlResult, 1)
		errCh := make(chan error, 1)
		go func() {
			result, err := r.Pause(context.Background(), workersessions.ControlRequest{ID: "worker-1"})
			resultCh <- result
			errCh <- err
		}()
		<-observedWait

		// observedWait proves Pause selected cancellationAttemptWait. Hold the
		// registry writer lock while recording the terminal winner so the next
		// control-loop read sees an authoritative terminal snapshot.
		r.mu.Lock()
		session := r.sessions["worker-1"]
		session.State = workersessions.StateCanceled
		r.sessions["worker-1"] = session
		r.mu.Unlock()
		close(wait)

		result := <-resultCh
		if err := <-errCh; err != nil || result.Outcome != workersessions.ControlOutcomeNoop || result.Session.State != workersessions.StateCanceled {
			t.Fatalf("Pause() after active control = %#v, %v, want CANCELED NOOP", result, err)
		}
	})
}

func TestFailureDiagnosticsUseClosedVocabularyAndProviderFallbacks(t *testing.T) {
	primary := &workers.WorkDiagnostics{
		Metadata: map[string]string{
			workers.ProviderResponseMetadataFailureOperation:      "provider_session_ingestion",
			workers.ProviderResponseMetadataFailureClassification: "resource_limit",
			workers.ProviderResponseMetadataFailureStage:          "final_parse",
		},
	}
	if got := safeDiagnosticFailureContext(primary); got != "operation=provider_session_ingestion classification=resource_limit stage=final_parse" {
		t.Fatalf("safeDiagnosticFailureContext(primary) = %q, want all safe fields", got)
	}

	fallback := &workers.WorkDiagnostics{
		Provider: &workers.ProviderDiagnostic{ResponseMetadata: map[string]string{
			workers.ProviderResponseMetadataFailureOperation:      "provider_session_ingestion",
			workers.ProviderResponseMetadataFailureClassification: "resource_limit",
			workers.ProviderResponseMetadataFailureStage:          "final_parse",
		}},
	}
	if got := mergeDiagnosticContexts(&workers.WorkDiagnostics{}, fallback); got != "operation=provider_session_ingestion classification=resource_limit stage=final_parse" {
		t.Fatalf("mergeDiagnosticContexts(empty, fallback) = %q, want provider fields", got)
	}
	if got := safeDetailWithDiagnostics(
		workersessions.FailureCauseAdapterFailure,
		&workers.WorkFailureMetadata{Family: workers.WorkFailureFamilyTerminal, Type: workers.WorkFailureTypeUnknown},
		primary,
	); !strings.Contains(got, "operation=provider_session_ingestion") {
		t.Fatalf("safeDetailWithDiagnostics() = %q, want safe diagnostic context", got)
	}

	if got := diagnosticValue(nil, workers.ProviderResponseMetadataFailureOperation); got != "" {
		t.Fatalf("diagnosticValue(nil) = %q, want empty", got)
	}
	if got := diagnosticValue(&workers.WorkDiagnostics{}, workers.ProviderResponseMetadataFailureOperation); got != "" {
		t.Fatalf("diagnosticValue(empty) = %q, want empty", got)
	}
	if got := safeDiagnosticValue(strings.Repeat("x", 65), knownFailureOperations); got != "" {
		t.Fatalf("safeDiagnosticValue(overlong) = %q, want empty", got)
	}
	if got := safeDiagnosticValue("untrusted-value", knownFailureOperations); got != "" {
		t.Fatalf("safeDiagnosticValue(unrecognized) = %q, want empty", got)
	}
}

func TestBoundedFailureDetailUsesFallbackAndTruncates(t *testing.T) {
	unknownKind := workersessions.FailureCauseKind("unrecognized")
	if got := boundedFailureDetail(unknownKind, ""); got == "" {
		t.Fatal("boundedFailureDetail(unknown, empty) = empty, want fallback")
	}

	long := strings.Repeat("x", workersessions.MaxFailureCauseDetailRunes+10)
	if got := boundedFailureDetail(workersessions.FailureCauseAdapterFailure, long); len([]rune(got)) != workersessions.MaxFailureCauseDetailRunes {
		t.Fatalf("boundedFailureDetail() length = %d, want %d", len([]rune(got)), workersessions.MaxFailureCauseDetailRunes)
	}
}

func TestClassifyTerminal_ContradictorySuccessWithExecutorPanicUsesPanicCause(t *testing.T) {
	terminal := classifyTerminal(
		&workers.WorkerExecutorPanicError{Cause: "panic evidence"},
		workers.WorkstationDispatchResult{
			TerminalOutcome: workers.WorkstationDispatchTerminalOutcomeFailed,
			Result:          workers.WorkResult{Outcome: workers.OutcomeAccepted},
		},
	)
	if terminal.Cause == nil || terminal.Cause.Kind != workersessions.FailureCauseExecutorPanic {
		t.Fatalf("terminal cause = %#v, want EXECUTOR_PANIC", terminal.Cause)
	}
}

func TestTerminalDraft_FailedRequiresValidTerminalResult(t *testing.T) {
	if _, err := terminalDraft(workersessions.StateFailed, workersessions.TerminalResult{}, "dispatch-1"); err == nil {
		t.Fatal("terminalDraft(FAILED, zero result) error = nil, want validation error")
	}
}

func TestNormalizeCommittedTerminal_RepairsInvalidFailure(t *testing.T) {
	result := normalizeCommittedTerminal(workersessions.StateFailed, workersessions.TerminalResult{
		Outcome: workersessions.TerminalOutcomeCompleted,
	})
	if result.Outcome != workersessions.TerminalOutcomeFailed || result.Cause == nil {
		t.Fatalf("normalizeCommittedTerminal() = %#v, want FAILED with fallback cause", result)
	}
	if strings.TrimSpace(result.Cause.Detail) == "" {
		t.Fatal("normalized failure cause detail is empty")
	}
}

type staticProviderBindingAppender struct {
	result events.AppendResult
	err    error
}

func (a staticProviderBindingAppender) Append(context.Context, events.AppendRequest) (events.AppendResult, error) {
	return a.result, a.err
}

func providerBindingRegistry(t *testing.T, appender EventsAppender) *registry {
	t.Helper()
	r := newTestRegistry(t)
	if appender != nil {
		r.events = appender
	}
	r.dispatchOwners["dispatch-1"] = "worker-1"
	r.publications["worker-1"] = &publication{open: true, turnID: "turn-1"}
	return r
}

func TestProviderBindingAndDispatchLookupEdgesAreObservable(t *testing.T) {
	ctx := context.Background()

	r := newTestRegistry(t)
	for _, test := range []struct {
		name string
		id   string
		want error
	}{
		{name: "blank dispatch", id: " ", want: workersessions.ErrInvalidProviderBinding},
		{name: "unknown dispatch", id: "missing", want: workersessions.ErrProviderBindingAttemptMismatch},
	} {
		t.Run(test.name, func(t *testing.T) {
			if _, err := r.WorkerSessionIDForDispatch(ctx, test.id); !errors.Is(err, test.want) {
				t.Fatalf("WorkerSessionIDForDispatch(%q) error = %v, want %v", test.id, err, test.want)
			}
		})
	}
	r.dispatchOwners["dispatch-1"] = "worker-1"
	if got, err := r.WorkerSessionIDForDispatch(ctx, " dispatch-1 "); err != nil || got != "worker-1" {
		t.Fatalf("WorkerSessionIDForDispatch(known) = %q, %v, want worker-1", got, err)
	}

	if _, err := New(unusedExecution{t: t}, newEventsAppenderForInternalTest(), nil, nil, unavailableProviderSessions{}, nil); !errors.Is(err, ErrMissingClock) {
		t.Fatalf("New(missing clock) error = %v, want ErrMissingClock", err)
	}
	if _, err := New(unusedExecution{t: t}, newEventsAppenderForInternalTest(), nil, platformclock.Real{}, nil, nil); !errors.Is(err, ErrMissingProviderSessions) {
		t.Fatalf("New(missing provider sessions) error = %v, want ErrMissingProviderSessions", err)
	}

	if got := providerIdentityForExecution(workers.WorkstationExecutionRequest{
		ExecutorProvider: workers.ExecutorProviderACP,
		ModelProvider:    "cursor-acp",
	}); got != "cursor-acp" {
		t.Fatalf("providerIdentityForExecution(ACP) = %q, want cursor-acp", got)
	}
	wantResult := workers.WorkstationDispatchResult{DispatchID: "dispatch-1"}
	if got := (&supervision{result: wantResult}).lastResult(); !reflect.DeepEqual(got, wantResult) {
		t.Fatalf("lastResult() = %#v, want %#v", got, wantResult)
	}
}

type internalTestEventsAppender struct{}

func (internalTestEventsAppender) Append(context.Context, events.AppendRequest) (events.AppendResult, error) {
	return events.AppendResult{}, nil
}

func newEventsAppenderForInternalTest() EventsAppender {
	return internalTestEventsAppender{}
}

func TestProviderBindingPublicationEdgesPreserveAttributionAndOrdering(t *testing.T) {
	t.Run("canonical provider draft", testCanonicalProviderDraftEdges)
	t.Run("binding lifecycle", testProviderBindingLifecycleEdges)
	t.Run("lookup errors", testProviderBindingLookupEdges)
	t.Run("append outcomes", testProviderBindingAppendEdges)
}

func testCanonicalProviderDraftEdges(t *testing.T) {
	ctx := context.Background()
	providerDraft := func(provider, dispatchID string) workers.Draft {
		return workers.Draft{DispatchID: dispatchID, Provenance: workers.Provenance{Provider: provider}}
	}
	r := providerBindingRegistry(t, nil)
	pub := r.publications["worker-1"]
	request := workersessions.PublishRecordRequest{SessionID: "worker-1", Draft: providerDraft("claude", "")}
	if err := r.ensurePublishRecordProvider(ctx, request, pub); !errors.Is(err, workersessions.ErrInvalidProviderBinding) {
		t.Fatalf("ensurePublishRecordProvider(blank dispatch) error = %v, want ErrInvalidProviderBinding", err)
	}
	request.Draft.DispatchID = "foreign-dispatch"
	r.dispatchOwners["foreign-dispatch"] = "worker-2"
	if err := r.ensurePublishRecordProvider(ctx, request, pub); !errors.Is(err, workersessions.ErrProviderBindingAttemptMismatch) {
		t.Fatalf("ensurePublishRecordProvider(foreign dispatch) error = %v, want ErrProviderBindingAttemptMismatch", err)
	}
	request.Draft.DispatchID = "dispatch-1"
	if err := r.ensurePublishRecordProvider(ctx, request, pub); err != nil {
		t.Fatalf("ensurePublishRecordProvider(first provider) error = %v, want nil", err)
	}
	request.Draft.Provenance.Provider = "CLAUDE"
	if err := r.ensurePublishRecordProvider(ctx, request, pub); err != nil {
		t.Fatalf("ensurePublishRecordProvider(same provider) error = %v, want nil", err)
	}
	request.Draft.Provenance.Provider = "codex"
	if err := r.ensurePublishRecordProvider(ctx, request, pub); !errors.Is(err, workersessions.ErrProviderBindingConflict) {
		t.Fatalf("ensurePublishRecordProvider(conflicting provider) error = %v, want ErrProviderBindingConflict", err)
	}
}

func testProviderBindingLifecycleEdges(t *testing.T) {
	ctx := context.Background()
	closed := providerBindingRegistry(t, nil)
	closed.publications["worker-1"].open = false
	if _, err := closed.EnsureProviderBinding(ctx, workersessions.ProviderBindingRequest{DispatchID: "dispatch-1", Provider: "claude"}); !errors.Is(err, workersessions.ErrPublicationNotOpen) {
		t.Fatalf("EnsureProviderBinding(closed) error = %v, want ErrPublicationNotOpen", err)
	}

	accepted := providerBindingRegistry(t, nil)
	first, err := accepted.EnsureProviderBinding(ctx, workersessions.ProviderBindingRequest{DispatchID: "dispatch-1", Provider: "claude"})
	if err != nil || first.Outcome != workersessions.ProviderBindingOutcomeAccepted {
		t.Fatalf("EnsureProviderBinding(first) = %#v, %v, want ACCEPTED", first, err)
	}
	duplicate, err := accepted.EnsureProviderBinding(ctx, workersessions.ProviderBindingRequest{DispatchID: "dispatch-1", Provider: "CLAUDE"})
	if err != nil || duplicate.Outcome != workersessions.ProviderBindingOutcomeDuplicate {
		t.Fatalf("EnsureProviderBinding(duplicate) = %#v, %v, want DUPLICATE", duplicate, err)
	}
	if _, err := accepted.EnsureProviderBinding(ctx, workersessions.ProviderBindingRequest{DispatchID: "dispatch-1", Provider: "codex"}); !errors.Is(err, workersessions.ErrProviderBindingConflict) {
		t.Fatalf("EnsureProviderBinding(conflict) error = %v, want ErrProviderBindingConflict", err)
	}
}

func testProviderBindingLookupEdges(t *testing.T) {
	ctx := context.Background()
	unknown := providerBindingRegistry(t, nil)
	if _, err := unknown.EnsureProviderBinding(ctx, workersessions.ProviderBindingRequest{DispatchID: "missing", Provider: "claude"}); !errors.Is(err, workersessions.ErrProviderBindingAttemptMismatch) {
		t.Fatalf("EnsureProviderBinding(unknown) error = %v, want ErrProviderBindingAttemptMismatch", err)
	}
	unknown.dispatchOwners["orphan-dispatch"] = "orphan-session"
	if _, err := unknown.EnsureProviderBinding(ctx, workersessions.ProviderBindingRequest{DispatchID: "orphan-dispatch", Provider: "claude"}); !errors.Is(err, workersessions.ErrSessionNotFound) {
		t.Fatalf("EnsureProviderBinding(orphan publication) error = %v, want ErrSessionNotFound", err)
	}
	if _, err := unknown.EnsureProviderBinding(ctx, workersessions.ProviderBindingRequest{}); !errors.Is(err, workersessions.ErrInvalidProviderBinding) {
		t.Fatalf("EnsureProviderBinding(invalid) error = %v, want ErrInvalidProviderBinding", err)
	}
}

func testProviderBindingAppendEdges(t *testing.T) {
	ctx := context.Background()
	appendFailure := providerBindingRegistry(t, staticProviderBindingAppender{err: errors.New("binding append failed")})
	if _, err := appendFailure.EnsureProviderBinding(ctx, workersessions.ProviderBindingRequest{DispatchID: "dispatch-1", Provider: "claude"}); err == nil {
		t.Fatal("EnsureProviderBinding(append failure) error = nil, want append error")
	}
	appendDuplicate := providerBindingRegistry(t, staticProviderBindingAppender{result: events.AppendResult{Outcome: events.AppendOutcomeDuplicate}})
	result, err := appendDuplicate.EnsureProviderBinding(ctx, workersessions.ProviderBindingRequest{DispatchID: "dispatch-1", Provider: "claude"})
	if err != nil || result.Outcome != workersessions.ProviderBindingOutcomeDuplicate {
		t.Fatalf("EnsureProviderBinding(append duplicate) = %#v, %v, want DUPLICATE", result, err)
	}
}

func assertWorkerObservationLookups(t *testing.T, registry *registry, got workersessions.Observation, canceled context.Context) {
	t.Helper()
	gotByWorker, err := registry.GetObservationByWorkerSessionID(context.Background(), workersessions.GetObservationByWorkerSessionIDRequest{WorkerSessionID: "worker-1"})
	if err != nil || gotByWorker.WorkerSessionID != "worker-1" || gotByWorker.ProviderSessionAvailable != got.ProviderSessionAvailable {
		t.Fatalf("GetObservationByWorkerSessionID() = %#v, %v", gotByWorker, err)
	}
	if _, err := registry.GetObservationByWorkerSessionID(context.Background(), workersessions.GetObservationByWorkerSessionIDRequest{WorkerSessionID: "missing"}); !errors.Is(err, workersessions.ErrObservationSessionNotFound) {
		t.Fatalf("GetObservationByWorkerSessionID(missing) error = %v", err)
	}
	if _, err := registry.GetObservationByWorkerSessionID(canceled, workersessions.GetObservationByWorkerSessionIDRequest{WorkerSessionID: "worker-1"}); !errors.Is(err, workersessions.ErrObservationCanceled) {
		t.Fatalf("GetObservationByWorkerSessionID(canceled) error = %v", err)
	}
}

func TestStreamObservationsByWorkerSessionIDRejectsInvalidContextAndMissing(t *testing.T) {
	registry := newObservationRegistry(nil, nil)
	if _, err := registry.StreamObservationsByWorkerSessionID(context.Background(), workersessions.StreamObservationsByWorkerSessionIDRequest{}); !errors.Is(err, workersessions.ErrInvalidSessionID) {
		t.Fatalf("invalid Worker Session stream error = %v, want %v", err, workersessions.ErrInvalidSessionID)
	}

	canceled, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := registry.StreamObservationsByWorkerSessionID(canceled, workersessions.StreamObservationsByWorkerSessionIDRequest{WorkerSessionID: "worker-1"}); !errors.Is(err, workersessions.ErrObservationCanceled) {
		t.Fatalf("canceled Worker Session stream error = %v, want %v", err, workersessions.ErrObservationCanceled)
	}
	if _, err := registry.StreamObservationsByWorkerSessionID(context.Background(), workersessions.StreamObservationsByWorkerSessionIDRequest{WorkerSessionID: "missing"}); !errors.Is(err, workersessions.ErrObservationSessionNotFound) {
		t.Fatalf("missing Worker Session stream error = %v, want %v", err, workersessions.ErrObservationSessionNotFound)
	}
}

func assertObservationProjectionEdges(t *testing.T, registry *registry, canceled context.Context) {
	t.Helper()
	if _, err := registry.projectObservation(canceled, "worker-1"); !errors.Is(err, workersessions.ErrObservationCanceled) {
		t.Fatalf("projectObservation(canceled) error = %v", err)
	}
	if _, err := registry.projectObservation(context.Background(), "missing"); !errors.Is(err, workersessions.ErrObservationSessionNotFound) {
		t.Fatalf("projectObservation(missing) error = %v", err)
	}
}

func TestInvokeObservationProjectionUnavailableOutcomes(t *testing.T) {
	ref := observationProviderRef()
	registry := newObservationRegistry(observationProjectorFake{}, nil)
	registry.sessions["worker-1"] = observationSession("worker-1", workersessions.StateRunning)
	registry.observations["worker-1"] = observationMetadata()
	noProvider := newObservationRegistry(nil, nil)
	noProvider.sessions["worker-1"] = observationSession("worker-1", workersessions.StateRunning)
	noProvider.observations["worker-1"] = observationMetadata()
	if _, err := noProvider.GetObservation(context.Background(), workersessions.GetObservationRequest{ProviderSession: ref}); !errors.Is(err, workersessions.ErrObservationProjectionUnavailable) {
		t.Fatalf("GetObservation(without provider service) error = %v", err)
	}
	noReference := newObservationRegistry(nil, nil)
	noReference.sessions["worker-no-reference"] = workersessions.Session{ID: "worker-no-reference", State: workersessions.StateCompleted}
	noReference.observations["worker-no-reference"] = observationMetadata()
	gotNoReference, err := noReference.GetObservationByWorkerSessionID(context.Background(), workersessions.GetObservationByWorkerSessionIDRequest{WorkerSessionID: "worker-no-reference"})
	if err != nil || gotNoReference.ProviderSessionAvailable || gotNoReference.WorkerSessionID != "worker-no-reference" {
		t.Fatalf("GetObservationByWorkerSessionID(no reference) = %#v, %v", gotNoReference, err)
	}
	gotIdentity, err := noProvider.GetObservationByWorkerSessionID(context.Background(), workersessions.GetObservationByWorkerSessionIDRequest{WorkerSessionID: "worker-1"})
	if err != nil || !gotIdentity.ProviderSessionAvailable || gotIdentity.ProviderSession.ID != ref.ID {
		t.Fatalf("GetObservationByWorkerSessionID(provider reference without projector) = %#v, %v", gotIdentity, err)
	}
	canceledProvider := newObservationRegistry(observationProjectorFake{err: context.Canceled}, nil)
	canceledProvider.sessions["worker-1"] = observationSession("worker-1", workersessions.StateRunning)
	canceledProvider.observations["worker-1"] = observationMetadata()
	if _, err := canceledProvider.GetObservation(context.Background(), workersessions.GetObservationRequest{ProviderSession: ref}); !errors.Is(err, workersessions.ErrObservationCanceled) {
		t.Fatalf("GetObservation(provider canceled) error = %v", err)
	}
	projectionFailure := newObservationRegistry(observationProjectorFake{err: errors.New("projection failed")}, nil)
	projectionFailure.sessions["worker-1"] = observationSession("worker-1", workersessions.StateRunning)
	projectionFailure.observations["worker-1"] = observationMetadata()
	if _, err := projectionFailure.ListObservations(context.Background(), workersessions.ListObservationsRequest{WorkID: "work-1"}); !errors.Is(err, workersessions.ErrObservationProjectionUnavailable) {
		t.Fatalf("ListObservations(projection failure) error = %v", err)
	}
	if _, _, ok := registry.loadObservationState("missing"); ok {
		t.Fatal("loadObservationState(missing) = ok, want false")
	}
	registry.sessions["no-metadata"] = observationSession("no-metadata", workersessions.StateRunning)
	if _, _, ok := registry.loadObservationState("no-metadata"); ok {
		t.Fatal("loadObservationState(missing metadata) = ok, want false")
	}
}

func TestStartLifecycleClassificationHelpers(t *testing.T) {
	noCause := startNotAccepted(nil)
	if noCause.Error() != workersessions.ErrStartNotAccepted.Error() {
		t.Fatalf("startNotAccepted(nil).Error() = %q, want stable public classification", noCause.Error())
	}
	if !errors.Is(noCause, workersessions.ErrStartNotAccepted) {
		t.Fatal("startNotAccepted(nil) does not unwrap to ErrStartNotAccepted")
	}

	cause := errors.New("admission detail")
	withCause := startNotAccepted(cause)
	if !errors.Is(withCause, cause) {
		t.Fatal("startNotAccepted(cause) does not preserve the underlying cause")
	}

	replay := &startReplay{
		done:   make(chan struct{}),
		result: workersessions.StartResult{Session: workersessions.Session{ID: "worker-replay", State: workersessions.StateRunning}},
	}
	close(replay.done)
	result, err := awaitStartReplay(nil, replay)
	if err != nil || result.Session.ID != "worker-replay" {
		t.Fatalf("awaitStartReplay(nil) = %+v, %v, want completed replay", result, err)
	}

	canceled, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := awaitStartReplay(canceled, &startReplay{done: make(chan struct{})}); !errors.Is(err, context.Canceled) {
		t.Fatalf("awaitStartReplay(canceled) error = %v, want context.Canceled", err)
	}

	if got := startReservationOutcome(workersessions.ErrStartRequestIDConflict); got != "idempotency_conflict" {
		t.Fatalf("startReservationOutcome(conflict) = %q, want idempotency_conflict", got)
	}
	if got := startReservationOutcome(workersessions.ErrStartServerStopping); got != "server_stopping" {
		t.Fatalf("startReservationOutcome(stopping) = %q, want server_stopping", got)
	}
	if got := startReservationOutcome(errors.New("not startable")); got != "not_startable" {
		t.Fatalf("startReservationOutcome(other) = %q, want not_startable", got)
	}

	registry := &registry{}
	if registry.serverOwnedContext().Done() != nil {
		t.Fatal("nil lifecycle context should fall back to a non-cancelable context")
	}
	if registry.supervisionContext(nil).Done() != nil {
		t.Fatal("nil supervision should fall back to a non-cancelable context")
	}
}

func TestStartSupervisionLifecycleFallbacks(t *testing.T) {
	registry := newTestRegistry(t)
	t.Cleanup(func() { _ = registry.Stop(context.Background()) })

	registry.finishStart()
	registry.activeStarts = 1
	registry.startsDone = make(chan struct{})
	registry.finishStart()
	select {
	case <-registry.startsDone:
	default:
		t.Fatal("finishStart did not close startsDone at zero active starts")
	}

	if err := registry.waitForSupervisionDriver(context.Background(), "missing"); err != nil {
		t.Fatalf("waitForSupervisionDriver(missing) = %v, want nil", err)
	}
	driver := newSupervision("dispatch", "turn")
	registry.supervisions["worker"] = driver
	canceled, cancel := context.WithCancel(context.Background())
	cancel()
	if err := registry.waitForSupervisionDriver(canceled, "worker"); !errors.Is(err, context.Canceled) {
		t.Fatalf("waitForSupervisionDriver(canceled) = %v, want context.Canceled", err)
	}

	owned := newSupervision("owned-dispatch", "owned-turn")
	owned.serverOwned = true
	if got := registry.supervisionContext(owned); got != registry.lifecycleCtx {
		t.Fatal("server-owned supervision did not use the registry lifecycle context")
	}
	owned.serverOwned = false
	if registry.supervisionContext(owned).Done() != nil {
		t.Fatal("non-server-owned supervision should use a non-cancelable context")
	}

	registry.reserveIfAbsent("owned-worker")
	if _, err := registry.transitionToStarting("owned-worker"); err != nil {
		t.Fatalf("transitionToStarting() = %v, want nil", err)
	}
	owned, ok := registry.registerServerOwnedSupervision("owned-worker", "owned-dispatch", "owned-turn")
	if !ok || owned == nil || !owned.serverOwned {
		t.Fatalf("registerServerOwnedSupervision() = %#v, %t, want owned supervision", owned, ok)
	}
	registry.stopping = true
	if _, ok := registry.registerServerOwnedSupervision("owned-worker", "late-dispatch", "late-turn"); ok {
		t.Fatal("registerServerOwnedSupervision() accepted after shutdown began")
	}
	delete(registry.supervisions, "owned-worker")
}

func TestStartPreparationFailureBranches(t *testing.T) {
	ctx := context.Background()
	stopping := newTestRegistry(t)
	stopping.reserveIfAbsent("stopping")
	if _, err := stopping.transitionToStarting("stopping"); err != nil {
		t.Fatalf("transitionToStarting(stopping) = %v, want nil", err)
	}
	stopping.stopping = true
	prepared, err := stopping.registerInvocationSupervision(ctx, coverageInvokeRequest("stopping"), invocationPreparationOptions{serverOwned: true})
	if err != nil || !prepared.terminal || !errors.Is(prepared.failure, workersessions.ErrStartServerStopping) {
		t.Fatalf("stopping registration = %+v, %v, want terminal server-stopping result", prepared, err)
	}

	terminal := newTestRegistry(t)
	terminal.reserveIfAbsent("terminal")
	if _, err := terminal.transitionToStarting("terminal"); err != nil {
		t.Fatalf("transitionToStarting(terminal) = %v, want nil", err)
	}
	terminal.commitTerminal("terminal", workersessions.StateCompleted, workersessions.TerminalResult{Outcome: workersessions.TerminalOutcomeCompleted})
	prepared, err = terminal.registerInvocationSupervision(ctx, coverageInvokeRequest("terminal"), invocationPreparationOptions{})
	if err != nil || !prepared.terminal || !errors.Is(prepared.failure, workersessions.ErrStartAdmissionFailed) {
		t.Fatalf("terminal registration = %+v, %v, want terminal admission-failed result", prepared, err)
	}

	reserved := newTestRegistry(t)
	reserved.reserveIfAbsent("reserved")
	_, err = reserved.registerInvocationSupervision(ctx, coverageInvokeRequest("reserved"), invocationPreparationOptions{})
	if !errors.Is(err, workersessions.ErrStartNotAccepted) {
		t.Fatalf("reserved registration error = %v, want ErrStartNotAccepted", err)
	}

	running := newTestRegistry(t)
	running.sessions["running"] = workersessions.Session{ID: "running", State: workersessions.StateRunning}
	if _, err := running.startReserved(ctx, workersessions.StartRequest{RequestID: "running-request", ID: "running"}); !errors.Is(err, workersessions.ErrSessionNotStartable) {
		t.Fatalf("startReserved(running) error = %v, want ErrSessionNotStartable", err)
	}

	if got := running.startAdmissionCause(&supervision{}); !errors.Is(got, workersessions.ErrStartAdmissionFailed) {
		t.Fatalf("startAdmissionCause(no error) = %v, want ErrStartAdmissionFailed", got)
	}
	running.publishTerminalSnapshot(ctx, "running", "", workersessions.Session{ID: "running", State: workersessions.StateRunning})
	running.publishTerminalSnapshot(ctx, "missing", "", workersessions.Session{ID: "missing", State: workersessions.StateFailed})

	secondTerminal := newTestRegistry(t)
	secondTerminal.reserveIfAbsent("second-terminal")
	if _, err := secondTerminal.transitionToStarting("second-terminal"); err != nil {
		t.Fatalf("transitionToStarting(second-terminal) = %v, want nil", err)
	}
	secondTerminal.terminalizeInvocationBeforeAdmission(ctx, "second-terminal", "dispatch")
	secondTerminal.terminalizeInvocationBeforeAdmission(ctx, "second-terminal", "dispatch")

	if err := running.publishTerminalRecord(ctx, "missing-publication", "dispatch", workersessions.StateCompleted, workersessions.TerminalResult{Outcome: workersessions.TerminalOutcomeCompleted}); !errors.Is(err, workersessions.ErrSessionNotFound) {
		t.Fatalf("publishTerminalRecord(missing publication) = %v, want ErrSessionNotFound", err)
	}
}

func TestStartOpeningReadinessRejectsInvalidReaderOutcomes(t *testing.T) {
	ctx := context.Background()
	readFailure := newTestRegistry(t)
	readFailure.retainedReader = coverageRetainedReader{err: errors.New("read failed")}
	if err := readFailure.ensureOpeningTopicReady(ctx, "read-failure"); !errors.Is(err, workersessions.ErrEventTopicUnavailable) {
		t.Fatalf("ensureOpeningTopicReady(read failure) = %v, want ErrEventTopicUnavailable", err)
	}

	for name, reader := range map[string]EventsReader{
		"subscribe-error":  coverageEventsReader{err: errors.New("subscribe failed")},
		"nil-subscription": coverageEventsReader{},
		"invalid-delivery": coverageEventsReader{subscription: events.Subscription(func(context.Context) events.Delivery { return events.Delivery{} })},
	} {
		t.Run(name, func(t *testing.T) {
			registry := coverageOpeningRegistry(t, name)
			registry.eventReader = reader
			if err := registry.ensureOpeningTopicReady(ctx, name); !errors.Is(err, workersessions.ErrEventTopicUnavailable) {
				t.Fatalf("ensureOpeningTopicReady() = %v, want ErrEventTopicUnavailable", err)
			}
		})
	}
}

func TestStartAndObservationEdgeBranches(t *testing.T) {
	registry := newTestRegistry(t)
	if _, err := registry.Start(nil, workersessions.StartRequest{}); !errors.Is(err, workersessions.ErrInvalidStartRequestID) {
		t.Fatalf("Start(nil, invalid request) = %v, want ErrInvalidStartRequestID", err)
	}

	registry.startReplays = nil
	registry.startsDone = nil
	first := workersessions.StartRequest{RequestID: "first-request", ID: "reserved"}
	if _, owner, err := registry.reserveStart(first); err != nil || !owner {
		t.Fatalf("reserveStart(first) = %t, %v, want new owner", owner, err)
	}
	conflict := first
	conflict.RequestID = "second-request"
	if _, _, err := registry.reserveStart(conflict); !errors.Is(err, workersessions.ErrSessionNotStartable) {
		t.Fatalf("reserveStart(existing session) = %v, want ErrSessionNotStartable", err)
	}

	topic := events.Topic("coverage-observation")
	if _, err := newReplayObservationSubscription(context.Background(), nil, topic, workersessions.StateRunning, 0); !errors.Is(err, workersessions.ErrObservationSourceUnavailable) {
		t.Fatalf("newReplayObservationSubscription(nil reader) = %v, want source unavailable", err)
	}
	replay := &replayObservationSubscription{topic: topic, next: events.Cursor{Topic: topic}}
	if err := replay.appendPage(events.ReadResult{Outcome: events.ReadOutcomeInvalidCursor}); !errors.Is(err, workersessions.ErrObservationSourceUnavailable) {
		t.Fatalf("appendPage(invalid cursor) = %v, want source unavailable", err)
	}
	if err := replay.appendPage(events.ReadResult{
		Outcome:  events.ReadOutcomeAtHead,
		Next:     events.Cursor{Topic: topic},
		Retained: events.RetainedRange{Topic: topic},
	}); err != nil {
		t.Fatalf("appendPage(at head) = %v, want nil", err)
	}
	replay.terminalRecordSeen = true
	if got := replay.replaySummaryReason(); got != "session-terminal-record" {
		t.Fatalf("replaySummaryReason() = %q, want session-terminal-record", got)
	}
}

func coverageInvokeRequest(id string) workersessions.InvokeSessionRequest {
	return workersessions.InvokeSessionRequest{ID: id}
}

func coverageOpeningRegistry(t *testing.T, id string) *registry {
	t.Helper()
	registry := newTestRegistry(t)
	registry.reserveIfAbsent(id)
	if _, err := registry.transitionToStarting(id); err != nil {
		t.Fatalf("transitionToStarting(%q) = %v, want nil", id, err)
	}
	startedAt := registry.ensureObservation(id, "dispatch", "", nil)
	if err := registry.publishOpeningRecord(context.Background(), id, "dispatch", openingSessionPayload(id, "dispatch", startedAt, workers.WorkstationExecutionRequest{}), ""); err != nil {
		t.Fatalf("publishOpeningRecord(%q) = %v, want nil", id, err)
	}
	return registry
}

type coverageRetainedReader struct{ err error }

func (r coverageRetainedReader) Read(context.Context, events.ReadRequest) (events.ReadResult, error) {
	return events.ReadResult{}, r.err
}

type coverageEventsReader struct {
	subscription events.Subscription
	err          error
}

func (r coverageEventsReader) Subscribe(context.Context, events.SubscribeRequest) (events.Subscription, error) {
	return r.subscription, r.err
}
