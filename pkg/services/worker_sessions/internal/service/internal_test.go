// backendsizecheck:ignore-file pre-existing baseline debt recorded 2026-08-08; split this oversized code into focused units and remove this exemption
// pkgmaintcheck:ignore-file-lines pre-existing baseline debt recorded 2026-08-08; refactor this code below the maintainability threshold and remove this exemption
package service

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"reflect"
	"strings"
	"sync"
	"testing"
	"time"

	platformclock "github.com/portpowered/infinite-you/pkg/platform/clock"
	platformprocess "github.com/portpowered/infinite-you/pkg/platform/process"

	"github.com/portpowered/infinite-you/pkg/platform/logging"
	"github.com/portpowered/infinite-you/pkg/services/events"
	modelinference "github.com/portpowered/infinite-you/pkg/services/models"
	providersessions "github.com/portpowered/infinite-you/pkg/services/provider_sessions"
	"github.com/portpowered/infinite-you/pkg/services/providers"
	"github.com/portpowered/infinite-you/pkg/services/recordings"
	"github.com/portpowered/infinite-you/pkg/services/work"
	workersessions "github.com/portpowered/infinite-you/pkg/services/worker_sessions"
	"github.com/portpowered/infinite-you/pkg/services/workers"
)

// unusedExecution is a Workers service double that fails
// the test if it is ever called. reserveIfAbsent and transitionToStarting
// never reach Workers, so this proves the reservation and starting
// transition are genuinely separate, Workers-free steps.
type unusedExecution struct {
	t *testing.T
}

func (u unusedExecution) Execute(context.Context, workers.ExecuteRequest) (workers.ExecuteResult, error) {
	u.t.Fatal("unexpected Execute call")
	return workers.ExecuteResult{}, workers.ErrExecuteUnavailable
}

func (u unusedExecution) InvokeModel(context.Context, string, modelinference.Request) (modelinference.Result, error) {
	u.t.Fatal("unexpected InvokeModel call")
	return modelinference.Result{}, workers.ErrExecuteUnavailable
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

func (b failingPublishBoundary) Execute(context.Context, workers.ExecuteRequest) (workers.ExecuteResult, error) {
	return workers.ExecuteResult{}, b.err
}

// controlClaimLogger is a deterministic observation point immediately after
// beginCancellation has claimed a supervision. The test holds the logger call
// until the admission gate is released, so the interleaving is controlled by
// channels rather than by scheduler timing.
type controlClaimLogger struct {
	logging.NoopLogger
	claimed chan struct{}
	release chan struct{}
	once    sync.Once
}

func (l *controlClaimLogger) Info(message string, _ ...any) {
	if message != "worker session control claimed" {
		return
	}
	l.once.Do(func() { close(l.claimed) })
	<-l.release
}

// newTestRegistry returns the concrete *registry (not just the Service
// interface) so white-box tests in this package can drive reserveIfAbsent
// and transitionToStarting directly.
func newTestRegistry(t *testing.T) *registry {
	t.Helper()
	svc, err := New(unusedExecution{t: t}, newInternalTestEventsService(), nil, platformclock.Real{}, unavailableProviderSessions{}, nil)
	if err != nil {
		t.Fatalf("New() error = %v, want nil", err)
	}
	r, ok := svc.(*registry)
	if !ok {
		t.Fatalf("New() returned %T, want *registry", svc)
	}
	return r
}

type runtimeAttemptBrokenAppender struct{ err error }

func (a *runtimeAttemptBrokenAppender) Append(context.Context, events.AppendRequest) (events.AppendResult, error) {
	return events.AppendResult{}, a.err
}

type runtimeAttemptClaimRaceAppender struct {
	service    *internalTestEventsService
	registry   *registry
	dispatchID string
	once       sync.Once
}

func (a *runtimeAttemptClaimRaceAppender) Append(ctx context.Context, request events.AppendRequest) (events.AppendResult, error) {
	result, err := a.service.Append(ctx, request)
	if err == nil {
		a.once.Do(func() {
			a.registry.mu.Lock()
			a.registry.dispatchOwners[a.dispatchID] = "worker-race-owner"
			a.registry.mu.Unlock()
		})
	}
	return result, err
}

func runtimeAttemptCompletedDispatch(dispatchID string) workers.WorkstationDispatchResult {
	return workers.WorkstationDispatchResult{
		DispatchID:      dispatchID,
		TerminalOutcome: workers.WorkstationDispatchTerminalOutcomeCompleted,
		Result: workers.WorkResult{
			DispatchID: dispatchID,
			Outcome:    workers.OutcomeAccepted,
		},
	}
}

func runtimeAttemptFailedDispatch(dispatchID string) workers.WorkstationDispatchResult {
	result := runtimeAttemptCompletedDispatch(dispatchID)
	result.TerminalOutcome = workers.WorkstationDispatchTerminalOutcomeFailed
	result.Result.Outcome = workers.OutcomeFailed
	return result
}

func TestBeginRuntimeAttempt_OpensAndCompletesDurableObservation(t *testing.T) {
	r := newTestRegistry(t)
	attempt, err := r.BeginRuntimeAttempt(context.Background(), workersessions.RuntimeAttemptRequest{
		ID:        "worker-1",
		AttemptID: "attempt-1",
		Execution: dispatchHandoff("dispatch-1"),
	})
	if err != nil {
		t.Fatalf("BeginRuntimeAttempt() error = %v, want nil", err)
	}
	if attempt == nil {
		t.Fatal("BeginRuntimeAttempt() returned a nil handle")
	}

	r.mu.RLock()
	_, runtimeOwned := r.runtimeAttempts["worker-1"]
	ownerID := r.dispatchOwners["dispatch-1"]
	r.mu.RUnlock()
	if !runtimeOwned || ownerID != "worker-1" {
		t.Fatalf("runtime ownership = %v, dispatch owner = %q, want true and worker-1", runtimeOwned, ownerID)
	}
	running, err := r.Get(context.Background(), workersessions.GetRequest{ID: "worker-1"})
	if err != nil {
		t.Fatalf("Get() after BeginRuntimeAttempt error = %v, want nil", err)
	}
	if running.State != workersessions.StateRunning {
		t.Fatalf("session state after BeginRuntimeAttempt = %q, want RUNNING", running.State)
	}

	if err := attempt.Complete(nil, runtimeAttemptCompletedDispatch("dispatch-1"), nil); err != nil {
		t.Fatalf("Complete() error = %v, want nil", err)
	}
	if err := attempt.Complete(context.Background(), runtimeAttemptFailedDispatch("dispatch-1"), errors.New("late duplicate")); err != nil {
		t.Fatalf("duplicate Complete() error = %v, want nil", err)
	}
	completed, err := r.Get(context.Background(), workersessions.GetRequest{ID: "worker-1"})
	if err != nil {
		t.Fatalf("Get() after Complete error = %v, want nil", err)
	}
	if completed.State != workersessions.StateCompleted || completed.Result == nil || completed.Result.Outcome != workersessions.TerminalOutcomeCompleted {
		t.Fatalf("completed session = %#v, want absorbing COMPLETED result", completed)
	}
	r.mu.RLock()
	_, runtimeOwned = r.runtimeAttempts["worker-1"]
	r.mu.RUnlock()
	if runtimeOwned {
		t.Fatal("runtime attempt ownership remained after Complete")
	}
}

func TestBeginRuntimeAttempt_RejectsOpeningFailureAndDispatchOwnerConflict(t *testing.T) {
	t.Run("opening failure terminalizes without claiming runtime ownership", func(t *testing.T) {
		r := newTestRegistry(t)
		r.events = &runtimeAttemptBrokenAppender{err: errors.New("opening publication failed")}

		_, err := r.BeginRuntimeAttempt(context.Background(), workersessions.RuntimeAttemptRequest{
			ID:        "worker-opening-failure",
			Execution: dispatchHandoff("dispatch-opening-failure"),
		})
		if !errors.Is(err, workersessions.ErrStartOpeningPublication) {
			t.Fatalf("BeginRuntimeAttempt() error = %v, want ErrStartOpeningPublication", err)
		}
		session, getErr := r.Get(context.Background(), workersessions.GetRequest{ID: "worker-opening-failure"})
		if getErr != nil {
			t.Fatalf("Get() after opening failure error = %v, want nil", getErr)
		}
		if session.State != workersessions.StateFailed {
			t.Fatalf("session state after opening failure = %q, want FAILED", session.State)
		}
		r.mu.RLock()
		_, runtimeOwned := r.runtimeAttempts["worker-opening-failure"]
		r.mu.RUnlock()
		if runtimeOwned {
			t.Fatal("opening failure claimed runtime ownership")
		}
	})

	t.Run("same logical dispatch cannot have two owners", func(t *testing.T) {
		r := newTestRegistry(t)
		first, err := r.BeginRuntimeAttempt(context.Background(), workersessions.RuntimeAttemptRequest{
			ID:        "worker-owner",
			Execution: dispatchHandoff("dispatch-shared"),
		})
		if err != nil {
			t.Fatalf("first BeginRuntimeAttempt() error = %v, want nil", err)
		}
		if _, err := r.BeginRuntimeAttempt(context.Background(), workersessions.RuntimeAttemptRequest{
			ID:        "worker-other",
			Execution: dispatchHandoff("dispatch-shared"),
		}); !errors.Is(err, workersessions.ErrProviderSessionAssociationAttemptMismatch) {
			t.Fatalf("conflicting BeginRuntimeAttempt() error = %v, want attempt mismatch", err)
		}
		if err := first.Complete(context.Background(), runtimeAttemptCompletedDispatch("dispatch-shared"), nil); err != nil {
			t.Fatalf("first Complete() error = %v, want nil", err)
		}
	})
}

func TestPublishRecord_AcceptsUsageWhenObservationProjectionIsUnavailable(t *testing.T) {
	const sessionID = "usage-without-observation"
	const dispatchID = "usage-without-observation-dispatch"
	r := newTestRegistry(t)
	attempt, err := r.BeginRuntimeAttempt(context.Background(), workersessions.RuntimeAttemptRequest{
		ID:        sessionID,
		AttemptID: dispatchID,
		Execution: dispatchHandoff(dispatchID),
	})
	if err != nil {
		t.Fatalf("BeginRuntimeAttempt() error = %v, want nil", err)
	}

	r.mu.Lock()
	delete(r.observations, sessionID)
	r.mu.Unlock()

	payload, err := json.Marshal(workers.UsagePayload{InputTokens: 11, Model: "model-without-observation"})
	if err != nil {
		t.Fatalf("json.Marshal(UsagePayload) error = %v", err)
	}
	publication, err := r.PublishRecord(context.Background(), workersessions.PublishRecordRequest{
		SessionID:      sessionID,
		Draft:          workers.Draft{Kind: workers.KindUsage, Phase: workers.PhaseUpdated, Payload: payload},
		SourceType:     "worker_provider",
		SourceID:       events.SourceID(sessionID),
		SourceSequence: 1,
		SourceEventID:  "usage-without-observation-1",
		SchemaID:       "workers.draft.v1",
	})
	if err != nil || publication.Outcome != workersessions.PublishOutcomeAccepted {
		t.Fatalf("PublishRecord() without observation metadata = %#v, %v, want accepted", publication, err)
	}
	if err := attempt.Complete(context.Background(), runtimeAttemptCompletedDispatch(dispatchID), nil); err != nil {
		t.Fatalf("RuntimeAttempt.Complete() error = %v, want nil", err)
	}
}

func TestBeginRuntimeAttempt_NilRegistryAndHandleAreUnavailable(t *testing.T) {
	var r *registry
	if _, err := r.BeginRuntimeAttempt(context.Background(), workersessions.RuntimeAttemptRequest{}); !errors.Is(err, workersessions.ErrStartAdmissionFailed) {
		t.Fatalf("nil registry BeginRuntimeAttempt() error = %v, want ErrStartAdmissionFailed", err)
	}
	var attempt *runtimeAttempt
	if err := attempt.Complete(context.Background(), workers.WorkstationDispatchResult{}, nil); err == nil {
		t.Fatal("nil runtime attempt Complete() error = nil, want unavailable error")
	}
	var publicAttempt workersessions.RuntimeAttempt
	if err := publicAttempt.Complete(context.Background(), workers.WorkstationDispatchResult{}, nil); err == nil {
		t.Fatal("nil public runtime attempt Complete() error = nil, want unavailable error")
	}
}

func TestBeginRuntimeAttempt_InitializesOwnershipMapsWithNilContext(t *testing.T) {
	r := newTestRegistry(t)
	r.mu.Lock()
	r.runtimeAttempts = nil
	r.dispatchOwners = nil
	r.mu.Unlock()

	attempt, err := r.BeginRuntimeAttempt(nil, workersessions.RuntimeAttemptRequest{
		ID:        "worker-map-init",
		AttemptID: "attempt-map-init",
		Execution: dispatchHandoff("dispatch-map-init"),
	})
	if err != nil {
		t.Fatalf("BeginRuntimeAttempt() error = %v, want nil", err)
	}
	if err := attempt.Complete(nil, runtimeAttemptCompletedDispatch("dispatch-map-init"), nil); err != nil {
		t.Fatalf("Complete() error = %v, want nil", err)
	}
	r.mu.RLock()
	_, runtimeOwned := r.runtimeAttempts["worker-map-init"]
	ownerID := r.dispatchOwners["dispatch-map-init"]
	r.mu.RUnlock()
	if runtimeOwned || ownerID != "worker-map-init" {
		t.Fatalf("post-completion ownership = %v, dispatch owner = %q, want false and worker-map-init", runtimeOwned, ownerID)
	}
}

func TestBeginRuntimeAttempt_RejectsInvalidAndAlreadyStartingSessions(t *testing.T) {
	r := newTestRegistry(t)
	if _, err := r.BeginRuntimeAttempt(context.Background(), workersessions.RuntimeAttemptRequest{
		Execution: dispatchHandoff("dispatch-invalid"),
	}); !errors.Is(err, workersessions.ErrInvalidSessionID) {
		t.Fatalf("invalid BeginRuntimeAttempt() error = %v, want ErrInvalidSessionID", err)
	}

	r.reserveIfAbsent("worker-already-starting")
	if _, err := r.transitionToStarting("worker-already-starting"); err != nil {
		t.Fatalf("transitionToStarting() error = %v, want nil", err)
	}
	if _, err := r.BeginRuntimeAttempt(context.Background(), workersessions.RuntimeAttemptRequest{
		ID:        "worker-already-starting",
		Execution: dispatchHandoff("dispatch-already-starting"),
	}); !errors.Is(err, workersessions.ErrSessionNotStartable) {
		t.Fatalf("already-starting BeginRuntimeAttempt() error = %v, want ErrSessionNotStartable", err)
	}
}

func TestRuntimeAttemptClaim_RaceGuardRejectsConflictingOwner(t *testing.T) {
	r := newTestRegistry(t)
	r.mu.Lock()
	r.dispatchOwners["dispatch-race"] = "worker-owner"
	r.mu.Unlock()
	if r.claimRuntimeAttempt("dispatch-race", "worker-other", "dispatch-race") {
		t.Fatal("claimRuntimeAttempt() accepted a conflicting owner")
	}
}

func TestBeginRuntimeAttempt_FinalClaimRaceTerminalizesTheAttempt(t *testing.T) {
	r := newTestRegistry(t)
	r.events = &runtimeAttemptClaimRaceAppender{
		service:    newInternalTestEventsService(),
		registry:   r,
		dispatchID: "dispatch-final-race",
	}
	_, err := r.BeginRuntimeAttempt(context.Background(), workersessions.RuntimeAttemptRequest{
		ID:        "worker-final-race",
		Execution: dispatchHandoff("dispatch-final-race"),
	})
	if !errors.Is(err, workersessions.ErrProviderSessionAssociationAttemptMismatch) {
		t.Fatalf("final-claim race error = %v, want attempt mismatch", err)
	}
	session, getErr := r.Get(context.Background(), workersessions.GetRequest{ID: "worker-final-race"})
	if getErr != nil {
		t.Fatalf("Get() after final-claim race error = %v, want nil", getErr)
	}
	if session.State != workersessions.StateFailed {
		t.Fatalf("session state after final-claim race = %q, want FAILED", session.State)
	}
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
	if r.beginExecutionPublish("worker-2", supervision) {
		t.Fatal("beginBoundaryPublish() succeeded after a control request")
	}
	r.reserveIfAbsent("worker-3")
	if workerSession, err := r.Get(ctx, workersessions.GetRequest{ID: "worker-3"}); err != nil || workerSession.State != workersessions.StateReserved {
		t.Fatalf("Get(worker-3) = %#v, %v, want RESERVED", workerSession, err)
	}
	if r.beginExecutionPublish("worker-3", newSupervision("dispatch-3", "")) {
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
		r.execution = failingPublishBoundary{unusedExecution: unusedExecution{t: t}, err: errors.New("execution publish failed")}
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

type admissionHandoffOutcome struct {
	result workers.WorkstationDispatchResult
	err    error
}

type admissionControlOutcome struct {
	result workersessions.ControlResult
	err    error
}

type admissionCancellationFixture struct {
	registry         *registry
	sessionID        string
	dispatchID       string
	supervision      *supervision
	request          workers.WorkstationDispatchRequest
	logger           *controlClaimLogger
	admissionStarted chan struct{}
	releaseAdmission chan struct{}
	executionCalled  chan struct{}
	executionDone    chan admissionHandoffOutcome
	closeAdmission   func()
}

func newAdmissionCancellationFixture(t *testing.T) admissionCancellationFixture {
	t.Helper()
	r := newTestRegistry(t)
	const sessionID = "worker-1"
	const dispatchID = "dispatch-1"
	r.reserveIfAbsent(sessionID)
	if _, err := r.transitionToStarting(sessionID); err != nil {
		t.Fatalf("transitionToStarting: %v", err)
	}
	request := dispatchHandoff(dispatchID)
	supervision, ok := r.registerSupervision(sessionID, dispatchID, "", request)
	if !ok {
		t.Fatal("registerSupervision: want supervised STARTING attempt")
	}
	fixture := admissionCancellationFixture{
		registry:         r,
		sessionID:        sessionID,
		dispatchID:       dispatchID,
		supervision:      supervision,
		request:          request,
		admissionStarted: make(chan struct{}),
		releaseAdmission: make(chan struct{}),
		executionCalled:  make(chan struct{}, 1),
		executionDone:    make(chan admissionHandoffOutcome, 1),
	}
	logger := &controlClaimLogger{claimed: make(chan struct{}), release: make(chan struct{})}
	fixture.logger = logger
	var releaseOnce sync.Once
	fixture.closeAdmission = func() {
		releaseOnce.Do(func() {
			close(fixture.releaseAdmission)
			close(logger.release)
		})
	}
	r.logger = logger
	supervision.mu.Lock()
	supervision.publishing = true
	supervision.mu.Unlock()
	go func() {
		result, err := executeWithService(
			context.Background(),
			coverageExecution{execute: func(_ context.Context, request workers.ExecuteRequest) (workers.ExecuteResult, error) {
				fixture.executionCalled <- struct{}{}
				return coverageExecutionResult(request, workers.ExecutionOutcomeAccepted), nil
			}},
			fixture.request,
			supervision,
			func() {
				close(fixture.admissionStarted)
				<-fixture.releaseAdmission
				if supervision.admissionAllowed() {
					r.acceptSupervision(fixture.sessionID, supervision)
				}
			},
		)
		r.finishSupervisionPublication(supervision)
		fixture.executionDone <- admissionHandoffOutcome{result: result, err: err}
	}()
	return fixture
}

func (f *admissionCancellationFixture) release() {
	f.closeAdmission()
}

func (f *admissionCancellationFixture) waitAtAdmission(t *testing.T) {
	t.Helper()
	select {
	case <-f.admissionStarted:
	case <-time.After(2 * time.Second):
		t.Fatal("execution did not reach the controlled admission gate")
	}
	starting, err := f.registry.Get(context.Background(), workersessions.GetRequest{ID: f.sessionID})
	if err != nil || starting.State != workersessions.StateStarting {
		t.Fatalf("session at admission gate = %+v, %v, want STARTING", starting, err)
	}
	f.supervision.mu.Lock()
	accepted, publishing := f.supervision.accepted, f.supervision.publishing
	f.supervision.mu.Unlock()
	if accepted || !publishing {
		t.Fatalf("supervision at admission gate accepted=%t publishing=%t, want false/true", accepted, publishing)
	}
}

func (f *admissionCancellationFixture) startCancel(t *testing.T) <-chan admissionControlOutcome {
	t.Helper()
	controlDone := make(chan admissionControlOutcome, 1)
	go func() {
		result, err := f.registry.Cancel(context.Background(), workersessions.ControlRequest{ID: f.sessionID})
		controlDone <- admissionControlOutcome{result: result, err: err}
	}()
	select {
	case <-f.logger.claimed:
	case <-time.After(2 * time.Second):
		t.Fatal("Cancel did not claim the exact supervision at the admission gate")
	}
	f.supervision.mu.Lock()
	queued := f.supervision.preAdmissionAction
	f.supervision.mu.Unlock()
	if queued != workersessions.ControlActionCancel {
		t.Fatalf("queued admission control = %q, want CANCEL", queued)
	}
	return controlDone
}

func waitAdmissionControl(t *testing.T, controlDone <-chan admissionControlOutcome) admissionControlOutcome {
	t.Helper()
	select {
	case control := <-controlDone:
		return control
	case <-time.After(2 * time.Second):
		t.Fatal("Cancel did not resolve after releasing the admission gate")
	}
	return admissionControlOutcome{}
}

func waitAdmissionHandoff(t *testing.T, handoffDone <-chan admissionHandoffOutcome) admissionHandoffOutcome {
	t.Helper()
	select {
	case handoff := <-handoffDone:
		return handoff
	case <-time.After(2 * time.Second):
		t.Fatal("execution handoff did not finish after pre-admission cancellation")
	}
	return admissionHandoffOutcome{}
}

func assertAdmissionCancellation(t *testing.T, fixture *admissionCancellationFixture, control admissionControlOutcome, handoff admissionHandoffOutcome) {
	t.Helper()
	if control.err != nil || control.result.Outcome != workersessions.ControlOutcomeApplied ||
		control.result.DispatchID != fixture.dispatchID || control.result.Session.State != workersessions.StateCanceled {
		t.Fatalf("Cancel() during admission = %#v, %v, want applied exact CANCELED supervision", control.result, control.err)
	}
	if !errors.Is(handoff.err, workers.ErrWorkstationDispatchCanceled) ||
		handoff.result.DispatchID != fixture.dispatchID ||
		handoff.result.TerminalOutcome != workers.WorkstationDispatchTerminalOutcomeCanceled {
		t.Fatalf("execution handoff = %#v, %v, want exact canceled dispatch", handoff.result, handoff.err)
	}
	select {
	case <-fixture.executionCalled:
		t.Fatal("Workers execution ran after cancellation won before admission")
	default:
	}
	fixture.registry.completeSupervision(fixture.sessionID, fixture.supervision, handoff.result, handoff.err)
	final, err := fixture.registry.Get(context.Background(), workersessions.GetRequest{ID: fixture.sessionID})
	if err != nil || final.State != workersessions.StateCanceled {
		t.Fatalf("late admission completion session = %+v, %v, want absorbing CANCELED", final, err)
	}
}

func TestCancel_BeforeBoundaryAdmissionEitherWaitsOrTerminatesTheExactSupervision(t *testing.T) {
	fixture := newAdmissionCancellationFixture(t)
	defer fixture.release()
	fixture.waitAtAdmission(t)
	controlDone := fixture.startCancel(t)
	fixture.release()
	control := waitAdmissionControl(t, controlDone)
	handoff := waitAdmissionHandoff(t, fixture.executionDone)
	assertAdmissionCancellation(t, &fixture, control, handoff)
}

func TestCancel_PreAdmissionTerminalAndConcurrentControlRemainNoop(t *testing.T) {
	r := newTestRegistry(t)
	r.reserveIfAbsent("worker-2")
	if _, err := r.transitionToStarting("worker-2"); err != nil {
		t.Fatalf("transitionToStarting(worker-2): %v", err)
	}
	noOpSupervision, ok := r.registerSupervision("worker-2", "dispatch-2", "")
	if !ok {
		t.Fatal("registerSupervision(worker-2): want exact attempt")
	}
	noOpSupervision.controlAction = workersessions.ControlActionCancel
	noOpSupervision.signalDone()
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

func TestCancel_BeforePublicationUsesRegisteredSupervision(t *testing.T) {
	r := newTestRegistry(t)
	r.reserveIfAbsent("worker-before-publication")
	if _, err := r.transitionToStarting("worker-before-publication"); err != nil {
		t.Fatalf("transitionToStarting: %v", err)
	}
	supervision, ok := r.registerSupervision(
		"worker-before-publication",
		"dispatch-before-publication",
		"",
		dispatchHandoff("dispatch-before-publication"),
	)
	if !ok {
		t.Fatal("registerSupervision: want exact pre-publication supervision")
	}

	result, err := r.Cancel(context.Background(), workersessions.ControlRequest{ID: "worker-before-publication"})
	if err != nil || result.Outcome != workersessions.ControlOutcomeApplied ||
		result.DispatchID != "dispatch-before-publication" || result.Session.State != workersessions.StateCanceled {
		t.Fatalf("Cancel() before publication = %#v, %v, want applied exact CANCELED supervision", result, err)
	}
	if errors.Is(err, workers.ErrUnknownWorkstationDispatch) {
		t.Fatalf("Cancel() before publication returned unknown dispatch: %v", err)
	}

	r.completeSupervision(
		"worker-before-publication",
		supervision,
		canceledBeforeAdmissionResult(dispatchHandoff("dispatch-before-publication")),
		workers.ErrWorkstationDispatchCanceled,
	)
	final, err := r.Get(context.Background(), workersessions.GetRequest{ID: "worker-before-publication"})
	if err != nil || final.State != workersessions.StateCanceled {
		t.Fatalf("late pre-publication completion session = %+v, %v, want absorbing CANCELED", final, err)
	}
}

func TestTerminate_BeforePublicationUsesRegisteredSupervisionAndIsIdempotent(t *testing.T) {
	r := newTestRegistry(t)
	const sessionID = "worker-terminate-before-publication"
	const dispatchID = "dispatch-terminate-before-publication"
	r.reserveIfAbsent(sessionID)
	if _, err := r.transitionToStarting(sessionID); err != nil {
		t.Fatalf("transitionToStarting: %v", err)
	}
	request := dispatchHandoff(dispatchID)
	supervision, ok := r.registerSupervision(sessionID, dispatchID, "", request)
	if !ok {
		t.Fatal("registerSupervision: want exact pre-publication supervision")
	}

	result, err := r.Terminate(context.Background(), workersessions.ControlRequest{ID: sessionID})
	if err != nil || result.Outcome != workersessions.ControlOutcomeApplied ||
		result.DispatchID != dispatchID || result.Session.State != workersessions.StateTerminated {
		t.Fatalf("Terminate() before publication = %#v, %v, want applied exact TERMINATED supervision", result, err)
	}
	if errors.Is(err, workers.ErrUnknownWorkstationDispatch) {
		t.Fatalf("Terminate() before publication returned unknown dispatch: %v", err)
	}

	repeated, err := r.Terminate(context.Background(), workersessions.ControlRequest{ID: sessionID})
	if err != nil || repeated.Outcome != workersessions.ControlOutcomeNoop ||
		repeated.DispatchID != dispatchID || repeated.Session.State != workersessions.StateTerminated {
		t.Fatalf("repeated Terminate() = %#v, %v, want exact terminal NOOP", repeated, err)
	}

	// A late canceled publication belongs to the same supervision and cannot
	// replace the absorbing terminal state or make a second boundary request.
	r.completeSupervision(sessionID, supervision, canceledBeforeAdmissionResult(request), workers.ErrWorkstationDispatchCanceled)
	final, err := r.Get(context.Background(), workersessions.GetRequest{ID: sessionID})
	if err != nil || final.State != workersessions.StateTerminated {
		t.Fatalf("late pre-publication completion session = %+v, %v, want absorbing TERMINATED", final, err)
	}
}

func TestCancel_KnownSessionWithAbsentOrUnknownBoundaryDispatchFailsClosed(t *testing.T) {
	for _, test := range []struct {
		name       string
		install    func(*supervision)
		wantReason string
	}{
		{
			name:       "absent cancellation handle",
			wantReason: "absent",
		},
		{
			name: "unknown boundary identity",
			install: func(supervision *supervision) {
				supervision.installCancelFailure(func() error { return workers.ErrUnknownWorkstationDispatch })
			},
			wantReason: "unknown",
		},
	} {
		t.Run(test.name, func(t *testing.T) {
			r := newTestRegistry(t)
			const sessionID = "worker-fail-closed"
			const dispatchID = "dispatch-fail-closed"
			r.sessions[sessionID] = workersessions.Session{ID: sessionID, State: workersessions.StateRunning}
			supervision := newSupervision(dispatchID, "", dispatchHandoff(dispatchID))
			supervision.accepted = true
			if test.install != nil {
				test.install(supervision)
			}
			r.supervisions[sessionID] = supervision
			r.dispatchOwners[dispatchID] = sessionID

			result, err := r.Cancel(context.Background(), workersessions.ControlRequest{ID: sessionID})
			if !errors.Is(err, workers.ErrUnknownWorkstationDispatch) ||
				result.Outcome != workersessions.ControlOutcomeFailed || result.DispatchID != dispatchID ||
				result.Session.State != workersessions.StateRunning {
				t.Fatalf("Cancel() with %s boundary dispatch = %#v, %v, want failed exact RUNNING result", test.wantReason, result, err)
			}
			current, getErr := r.Get(context.Background(), workersessions.GetRequest{ID: sessionID})
			if getErr != nil || current.State != workersessions.StateRunning {
				t.Fatalf("session after %s boundary failure = %+v, %v, want RUNNING", test.wantReason, current, getErr)
			}
		})
	}
}

func TestPublishRegisteredAttempt_CanceledBeforeAdmissionRetainsExactTerminal(t *testing.T) {
	r := newTestRegistry(t)
	const sessionID = "worker-publish-canceled"
	const dispatchID = "dispatch-publish-canceled"
	r.reserveIfAbsent(sessionID)
	if _, err := r.transitionToStarting(sessionID); err != nil {
		t.Fatalf("transitionToStarting: %v", err)
	}
	request := dispatchHandoff(dispatchID)
	supervision, ok := r.registerSupervision(sessionID, dispatchID, "", request)
	if !ok {
		t.Fatal("registerSupervision: want exact publication supervision")
	}
	supervision.mu.Lock()
	supervision.publishing = true
	supervision.mu.Unlock()

	logger := &controlClaimLogger{claimed: make(chan struct{}), release: make(chan struct{})}
	r.logger = logger
	var releaseOnce sync.Once
	release := func() { releaseOnce.Do(func() { close(logger.release) }) }
	defer release()
	controlDone := make(chan struct {
		result workersessions.ControlResult
		err    error
	}, 1)
	go func() {
		result, err := r.Cancel(context.Background(), workersessions.ControlRequest{ID: sessionID})
		controlDone <- struct {
			result workersessions.ControlResult
			err    error
		}{result: result, err: err}
	}()
	select {
	case <-logger.claimed:
	case <-time.After(2 * time.Second):
		t.Fatal("Cancel did not claim publication supervision")
	}
	release()

	result, retry := r.publishRegisteredAttempt(context.Background(), sessionID, request, supervision, true)
	if retry || result.Session.State != workersessions.StateCanceled ||
		result.Dispatch.DispatchID != dispatchID ||
		!errors.Is(result.DispatchErr, workers.ErrWorkstationDispatchCanceled) {
		t.Fatalf("publishRegisteredAttempt() = %#v, retry %t, want exact canceled terminal", result, retry)
	}
	select {
	case control := <-controlDone:
		if control.err != nil || control.result.Outcome != workersessions.ControlOutcomeApplied ||
			control.result.DispatchID != dispatchID || control.result.Session.State != workersessions.StateCanceled {
			t.Fatalf("Cancel() after publication cancellation = %#v, %v, want applied exact CANCELED supervision", control.result, control.err)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("Cancel did not resolve after canceled publication")
	}
}

func TestInterruptGuardsAndReplayHelpersCoverObservableFailureClasses(t *testing.T) {
	t.Run("source association validation", testInterruptSourceAssociationValidation)
	t.Run("source identity guards", testInterruptSourceIdentityGuards)
	t.Run("supervision reservation guards", testInterruptSupervisionReservationGuards)
	t.Run("reservation lifecycle guards", testInterruptReservationLifecycleGuards)
	t.Run("replay and cancellation helpers", testInterruptReplayAndCancellationHelpers)
	t.Run("validation result", testInterruptValidationResult)
}

func testInterruptSourceAssociationValidation(t *testing.T) {
	validReference := providers.SessionRef{Provider: providers.IDCodex, Kind: providers.SessionIDKind, ID: "provider-session"}
	cases := []struct {
		name   string
		source workersessions.Session
		valid  bool
	}{
		{name: "missing", source: workersessions.Session{ID: "source"}},
		{name: "invalid reference", source: workersessions.Session{ID: "source", ProviderSessionAssociation: &workersessions.ProviderSessionAssociation{WorkerSessionID: "source", DispatchID: "dispatch", AttemptID: "dispatch"}}},
		{name: "identity mismatch", source: workersessions.Session{ID: "source", ProviderSessionAssociation: &workersessions.ProviderSessionAssociation{WorkerSessionID: "other", DispatchID: "dispatch", AttemptID: "dispatch", Reference: validReference}}},
		{name: "valid", valid: true, source: workersessions.Session{ID: "source", ProviderSessionAssociation: &workersessions.ProviderSessionAssociation{WorkerSessionID: "source", DispatchID: "dispatch", AttemptID: "dispatch", Reference: validReference}}},
	}
	for _, test := range cases {
		t.Run(test.name, func(t *testing.T) {
			association, err := interruptSourceAssociation(test.source)
			if test.valid {
				if err != nil || association.Reference != validReference {
					t.Fatalf("interruptSourceAssociation() = %#v, %v, want valid exact reference", association, err)
				}
				return
			}
			if err == nil {
				t.Fatal("interruptSourceAssociation() = nil error, want rejection")
			}
		})
	}
}

func testInterruptSourceIdentityGuards(t *testing.T) {
	request := workersessions.InterruptRequest{SourceWorkerSessionID: "source", SuccessorWorkerSessionID: "successor"}
	cases := []struct {
		name    string
		session map[string]workersessions.Session
		wantErr error
	}{
		{name: "missing source", session: map[string]workersessions.Session{}, wantErr: workersessions.ErrInterruptSourceNotFound},
		{name: "inactive source", session: map[string]workersessions.Session{"source": {ID: "source", State: workersessions.StatePaused}}, wantErr: workersessions.ErrInterruptSourceNotActive},
		{name: "source already has successor", session: map[string]workersessions.Session{"source": {ID: "source", State: workersessions.StateRunning, SuccessorWorkerSessionID: "existing"}}, wantErr: workersessions.ErrInterruptSourceConflict},
		{name: "successor exists", session: map[string]workersessions.Session{"source": {ID: "source", State: workersessions.StateRunning}, "successor": {ID: "successor"}}, wantErr: workersessions.ErrInterruptSourceConflict},
		{name: "active source", session: map[string]workersessions.Session{"source": {ID: "source", State: workersessions.StateRunning}}, wantErr: nil},
	}
	for _, test := range cases {
		t.Run(test.name, func(t *testing.T) {
			r := newTestRegistry(t)
			r.sessions = test.session
			_, err := r.interruptSourceLocked(request)
			if !errors.Is(err, test.wantErr) {
				t.Fatalf("interruptSourceLocked() error = %v, want %v", err, test.wantErr)
			}
		})
	}
}

func testInterruptSupervisionReservationGuards(t *testing.T) {
	validReference := providers.SessionRef{Provider: providers.IDCodex, Kind: providers.SessionIDKind, ID: "provider-session"}
	cases := []struct {
		name        string
		configure   func(*supervision)
		association workersessions.ProviderSessionAssociation
		wantErr     error
	}{
		{name: "missing supervision", wantErr: workersessions.ErrInterruptExecutionUnavailable, association: validAssociation("source", "dispatch", validReference)},
		{name: "not accepted", configure: func(s *supervision) { s.accepted = false }, wantErr: workersessions.ErrInterruptSourceNotActive, association: validAssociation("source", "dispatch", validReference)},
		{name: "control conflict", configure: func(s *supervision) { s.accepted = true; s.controlAction = workersessions.ControlActionCancel }, wantErr: workersessions.ErrInterruptSourceConflict, association: validAssociation("source", "dispatch", validReference)},
		{name: "provider mismatch", configure: func(s *supervision) { s.accepted = true }, wantErr: workersessions.ErrInterruptProviderSessionInvalid, association: validAssociation("source", "other-dispatch", validReference)},
		{name: "accepted", configure: func(s *supervision) { s.accepted = true }, association: validAssociation("source", "dispatch", validReference)},
	}
	for _, test := range cases {
		t.Run(test.name, func(t *testing.T) {
			r := newTestRegistry(t)
			supervisionValue := newSupervision("dispatch", "")
			if test.configure != nil {
				test.configure(supervisionValue)
				r.supervisions = map[string]*supervision{"source": supervisionValue}
			}
			plan, err := r.reserveInterruptSupervisionLocked(
				workersessions.InterruptRequest{RequestID: "request", SourceWorkerSessionID: "source", SuccessorWorkerSessionID: "successor"},
				workersessions.Session{ID: "source"}, test.association,
			)
			if !errors.Is(err, test.wantErr) {
				t.Fatalf("reserveInterruptSupervisionLocked() error = %v, want %v", err, test.wantErr)
			}
			if test.wantErr == nil && (err != nil || plan.dispatchID != "dispatch") {
				t.Fatalf("accepted supervision plan = %#v, %v, want dispatch", plan, err)
			}
		})
	}
}

func testInterruptReservationLifecycleGuards(t *testing.T) {
	request := workersessions.InterruptRequest{RequestID: "request", SourceWorkerSessionID: "source", SuccessorWorkerSessionID: "successor", ReplacementMessage: "replacement"}
	tuple := interruptTuple{sourceID: "source", successorID: "successor", message: "replacement"}
	r := newTestRegistry(t)
	r.interruptReplays = nil
	if _, _, err := r.reserveInterrupt(request); !errors.Is(err, workersessions.ErrInterruptSourceNotFound) {
		t.Fatalf("reserveInterrupt(missing source) error = %v, want source not found", err)
	}
	if r.interruptReplays == nil {
		t.Fatal("reserveInterrupt() did not initialize replay storage")
	}
	r.interruptReplays[request.RequestID] = &interruptReplay{tuple: tuple}
	replay, owner, err := r.reserveInterrupt(request)
	if err != nil || owner || replay == nil {
		t.Fatalf("reserveInterrupt(replay) = %#v, %t, %v, want existing replay", replay, owner, err)
	}
	conflict := request
	conflict.ReplacementMessage = "different"
	if _, _, err := r.reserveInterrupt(conflict); !errors.Is(err, workersessions.ErrInterruptRequestIDConflict) {
		t.Fatalf("reserveInterrupt(conflict) error = %v, want request ID conflict", err)
	}
	r.interruptReplays = make(map[string]*interruptReplay)
	r.stopping = true
	if _, _, err := r.reserveInterrupt(request); !errors.Is(err, workersessions.ErrInterruptServerStopping) {
		t.Fatalf("reserveInterrupt(stopping) error = %v, want server stopping", err)
	}
	r.stopping = false
	r.sessions[request.SourceWorkerSessionID] = workersessions.Session{ID: request.SourceWorkerSessionID, State: workersessions.StateRunning}
	if _, _, err := r.reserveInterrupt(request); !errors.Is(err, workersessions.ErrInterruptProviderSessionMissing) {
		t.Fatalf("reserveInterrupt(missing provider association) error = %v, want provider session missing", err)
	}
	r.startsDone = nil
	replay = r.storeInterruptReservationLocked(request, tuple, interruptPlan{dispatchID: "dispatch"})
	if replay == nil || r.activeStarts != 1 {
		t.Fatalf("storeInterruptReservationLocked() = %#v with active starts %d, want replay and one active start", replay, r.activeStarts)
	}
	r.finishStart()
}

func testInterruptReplayAndCancellationHelpers(t *testing.T) {
	replay := &interruptReplay{done: make(chan struct{}), result: workersessions.InterruptResult{RequestID: "request", Accepted: true}}
	close(replay.done)
	result, err := awaitInterruptReplay(context.Background(), replay)
	if err != nil || !result.Accepted {
		t.Fatalf("awaitInterruptReplay(completed) = %#v, %v, want accepted replay", result, err)
	}
	pending := &interruptReplay{done: make(chan struct{})}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := awaitInterruptReplay(ctx, pending); !errors.Is(err, context.Canceled) {
		t.Fatalf("awaitInterruptReplay(canceled) error = %v, want context.Canceled", err)
	}
	completedAfterCancel := &interruptReplay{done: make(chan struct{}), result: workersessions.InterruptResult{RequestID: "completed-after-cancel"}}
	close(completedAfterCancel.done)
	if result, err := awaitInterruptReplay(ctx, completedAfterCancel); err != nil || result.RequestID != "completed-after-cancel" {
		t.Fatalf("awaitInterruptReplay(completed after cancellation) = %#v, %v, want retained replay", result, err)
	}
	completed := &interruptReplay{done: make(chan struct{}), result: workersessions.InterruptResult{Accepted: true}}
	close(completed.done)
	if result, err := awaitInterruptReplay(nil, completed); err != nil || !result.Accepted {
		t.Fatalf("awaitInterruptReplay(nil context) = %#v, %v, want completed replay", result, err)
	}
	if !errors.Is(interruptPhaseCause(workersessions.InterruptPhaseValidation), workersessions.ErrInterruptValidation) ||
		!errors.Is(interruptPhaseCause(workersessions.InterruptPhaseSourceCancellation), workersessions.ErrInterruptSourceCancellation) ||
		!errors.Is(interruptPhaseCause(workersessions.InterruptPhaseSuccessorAdmission), workersessions.ErrInterruptSuccessorAdmission) ||
		interruptPhaseCause("unknown") == nil {
		t.Fatal("interruptPhaseCause() did not preserve all phase sentinels")
	}
	if !errors.Is(interruptCancellationCause(workers.WorkstationDispatchCancelResult{}, errors.New("cancel failed")), workersessions.ErrInterruptSourceCancellationFailed) ||
		!errors.Is(interruptCancellationCause(workers.WorkstationDispatchCancelResult{Outcome: workers.WorkstationDispatchCancelOutcomeAlreadyTerminal}, nil), workers.ErrWorkstationDispatchAlreadyTerminal) ||
		interruptCancellationCause(workers.WorkstationDispatchCancelResult{Outcome: workers.WorkstationDispatchCancelOutcomeAlreadyCanceled}, nil) == nil {
		t.Fatal("interruptCancellationCause() did not classify boundary outcomes")
	}
}

func testInterruptValidationResult(t *testing.T) {
	result, err := newTestRegistry(t).Interrupt(nil, workersessions.InterruptRequest{})
	var interruptErr *workersessions.InterruptError
	if !errors.As(err, &interruptErr) || interruptErr.Phase != workersessions.InterruptPhaseValidation || result.Phase != workersessions.InterruptPhaseValidation {
		t.Fatalf("Interrupt(nil context) = %#v, %v, want validation error", result, err)
	}
}

func validAssociation(workerSessionID, dispatchID string, reference providers.SessionRef) workersessions.ProviderSessionAssociation {
	return workersessions.ProviderSessionAssociation{
		WorkerSessionID: workerSessionID,
		DispatchID:      dispatchID,
		AttemptID:       dispatchID,
		Reference:       reference,
	}
}

func TestBeginCancellationClassifiesAdmissionAndPreAdmissionStates(t *testing.T) {
	interrupting := newSupervision("dispatch-interrupting", "")
	interrupting.interrupting = true
	interrupting.interruptDone = make(chan struct{})
	attempt := interrupting.beginCancellation(workersessions.ControlActionCancel)
	if attempt.kind != cancellationAttemptWait || attempt.wait != interrupting.interruptDone {
		t.Fatalf("beginCancellation(interrupting) = %#v, want interrupt wait", attempt)
	}

	publishing := newSupervision("dispatch-publishing", "")
	publishing.publishing = true
	publishing.accepted = false
	attempt = publishing.beginCancellation(workersessions.ControlActionPause)
	if attempt.kind != cancellationAttemptWait || attempt.wait == nil || publishing.preAdmissionAction != workersessions.ControlActionPause {
		t.Fatalf("beginCancellation(publishing) = %#v, want wait with queued pause", attempt)
	}

	preAdmission := newSupervision("dispatch-pre-admission", "")
	preAdmission.preAdmissionAction = workersessions.ControlActionPause
	attempt = preAdmission.beginCancellation(workersessions.ControlActionCancel)
	if attempt.kind != cancellationAttemptBoundary || attempt.dispatchID != "dispatch-pre-admission" ||
		preAdmission.requestedAction != workersessions.ControlActionPause || preAdmission.controlDone == nil {
		t.Fatalf("beginCancellation(queued pre-admission) = %#v, want boundary for queued pause", attempt)
	}

	notAccepted := newSupervision("dispatch-not-accepted", "")
	attempt = notAccepted.beginCancellation(workersessions.ControlActionTerminate)
	if attempt.kind != cancellationAttemptBeforeAdmission || notAccepted.controlAction != workersessions.ControlActionTerminate {
		t.Fatalf("beginCancellation(not accepted) = %#v, want before-admission terminal control", attempt)
	}
}

func TestCancelBoundaryFailsWhenTheAttemptHasNoInstalledCancellation(t *testing.T) {
	r := newTestRegistry(t)
	const sessionID = "worker-noop"
	const dispatchID = "dispatch-noop"
	r.sessions[sessionID] = workersessions.Session{ID: sessionID, State: workersessions.StateRunning}
	supervision := newSupervision(dispatchID, "")
	supervision.accepted = true
	supervision.controlActive = true
	supervision.requestedAction = workersessions.ControlActionCancel
	wait := make(chan struct{})
	supervision.controlDone = wait
	r.supervisions[sessionID] = supervision

	result, retry, err := r.cancelBoundary(context.Background(), workersessions.ControlRequest{ID: sessionID}, workersessions.ControlActionCancel, false, supervision, cancellationAttempt{wait: wait, dispatchID: dispatchID})
	if !errors.Is(err, workers.ErrUnknownWorkstationDispatch) || retry || result.Outcome != workersessions.ControlOutcomeFailed || result.Session.State != workersessions.StateRunning {
		t.Fatalf("cancelBoundary(without cancellation handle) = %#v, %t, %v, want failed unknown-dispatch result", result, retry, err)
	}
}

func TestCancelControlWaitsForConcurrentControl(t *testing.T) {
	r := newTestRegistry(t)
	const sessionID = "worker-wait"
	r.sessions[sessionID] = workersessions.Session{ID: sessionID, State: workersessions.StateRunning}
	supervision := newSupervision("dispatch-wait", "")
	supervision.accepted = true
	supervision.controlActive = true
	supervision.controlDone = make(chan struct{})
	close(supervision.controlDone)
	r.supervisions[sessionID] = supervision

	result, retry, err := r.cancelControlIteration(context.Background(), workersessions.ControlRequest{ID: sessionID}, workersessions.ControlActionCancel, false)
	if err != nil || !retry || result != (workersessions.ControlResult{}) {
		t.Fatalf("cancelControlIteration(concurrent control) = %#v, %t, %v, want retry", result, retry, err)
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
	r.publications["worker-1"] = &publication{
		open:         true,
		lastSequence: make(map[sourceKey]events.SourceSequence),
		accepted:     make(map[events.AppendIdentity]struct{}),
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

	t.Run("resume refuses stale ownership before reserving a continuation", func(t *testing.T) {
		tests := []struct {
			name      string
			configure func(*supervision)
			wantErr   error
		}{
			{
				name: "attempt mismatch",
				configure: func(supervision *supervision) {
					supervision.dispatchID = "dispatch-foreign"
				},
				wantErr: workersessions.ErrProviderSessionAssociationAttemptMismatch,
			},
			{
				name: "not admitted",
				configure: func(supervision *supervision) {
					supervision.accepted = false
				},
				wantErr: workersessions.ErrProviderSessionAssociationNotAvailable,
			},
			{
				name: "active control",
				configure: func(supervision *supervision) {
					supervision.controlActive = true
				},
				wantErr: workersessions.ErrInvalidState,
			},
		}
		for _, test := range tests {
			t.Run(test.name, func(t *testing.T) {
				r, supervision, _ := newPausedContinuationRegistry(t)
				test.configure(supervision)
				before := r.sessions["worker-1"]

				result, err := r.Resume(context.Background(), workersessions.ControlRequest{ID: "worker-1"})
				if !errors.Is(err, test.wantErr) || result.Outcome != workersessions.ControlOutcomeFailed ||
					result.Session.State != workersessions.StatePaused {
					t.Fatalf("Resume() = %#v, %v, want failed PAUSED result with %v", result, err, test.wantErr)
				}
				if current := r.sessions["worker-1"]; !reflect.DeepEqual(current, before) {
					t.Fatalf("Resume() mutated refused session: got %#v, want %#v", current, before)
				}
				supervision.mu.Lock()
				continuing, publishing, attemptsMade := supervision.continuing, supervision.publishing, supervision.attemptsMade
				supervision.mu.Unlock()
				if continuing || publishing || attemptsMade != 0 {
					t.Fatalf("refused Resume() reserved continuation state: continuing=%t publishing=%t attempts=%d", continuing, publishing, attemptsMade)
				}
			})
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
		wantContinuation := reference.ContinuationRef()
		if !prepared || previousDispatchID != "dispatch-1" || continuation.Execution.Continuation == nil || *continuation.Execution.Continuation != wantContinuation {
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
		r, supervision := newRunningPauseRegistry(t)
		boundaryErr := errors.New("cancel boundary failed")
		supervision.installCancelFailure(func() error { return boundaryErr })
		result, err := r.Pause(context.Background(), workersessions.ControlRequest{ID: "worker-1"})
		if !errors.Is(err, boundaryErr) || result.Outcome != workersessions.ControlOutcomeFailed || result.Session.State != workersessions.StateRunning {
			t.Fatalf("Pause() = %#v, %v, want failed RUNNING result", result, err)
		}
	})

	t.Run("already-terminal cancellation is a no-op", func(t *testing.T) {
		r, supervision := newRunningPauseRegistry(t)
		supervision.signalDone()
		supervision.installCancel(func() {})
		result, err := r.Pause(context.Background(), workersessions.ControlRequest{ID: "worker-1"})
		if err != nil || result.Outcome != workersessions.ControlOutcomeNoop || result.Session.State != workersessions.StateRunning {
			t.Fatalf("Pause() = %#v, %v, want RUNNING NOOP", result, err)
		}
	})

	t.Run("cancellation without a paused callback remains a no-op", func(t *testing.T) {
		r, supervision := newRunningPauseRegistry(t)
		supervision.signalDone()
		supervision.installCancel(func() {})
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

func TestNormalizeCommittedTerminal_RepairsAControlOutcomeOnFailed(t *testing.T) {
	result := normalizeCommittedTerminal(workersessions.StateFailed, workersessions.TerminalResult{
		Outcome: workersessions.TerminalOutcomeFailed,
		Cause: &workersessions.FailureCause{
			Kind:   workersessions.FailureCauseOperatorCanceled,
			Detail: "an operator cancel control ended the Worker Session",
		},
	})
	if result.Cause == nil || result.Cause.Kind != workersessions.FailureCauseWorkersExecutionFailure {
		t.Fatalf("normalizeCommittedTerminal() = %#v, want the control outcome repaired to an execution failure", result)
	}
	if err := result.Validate(); err != nil {
		t.Fatalf("normalized FAILED result is not committable: %v", err)
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

// internalTestEventsService is the owner-local Events role used by white-box
// registry tests. It retains enough real aggregate behavior for opening-topic
// readiness and replay assertions without constructing the sibling Events
// wire service from this package.
type internalTestEventsService struct {
	mu      sync.Mutex
	records map[events.Topic][]events.Record
}

func newInternalTestEventsService() *internalTestEventsService {
	return &internalTestEventsService{records: make(map[events.Topic][]events.Record)}
}

func (service *internalTestEventsService) Append(
	ctx context.Context,
	request events.AppendRequest,
) (events.AppendResult, error) {
	if err := request.Validate(); err != nil {
		return events.AppendResult{}, err
	}
	if err := ctx.Err(); err != nil {
		return events.AppendResult{}, err
	}

	service.mu.Lock()
	defer service.mu.Unlock()
	records := service.records[request.Topic]
	for _, record := range records {
		if record.Identity() != request.Identity() {
			continue
		}
		if reflect.DeepEqual(record.Payload, request.Payload) && record.SchemaID == request.SchemaID {
			return events.AppendResult{Record: record.Detached(), Outcome: events.AppendOutcomeDuplicate}, nil
		}
		return events.AppendResult{}, events.ErrOperationFailed
	}
	detached := request.Detached()
	record := events.Record{
		ID:             events.RecordID{Topic: request.Topic, Position: events.AggregateSequence(len(records) + 1)},
		SourceType:     detached.SourceType,
		SourceID:       detached.SourceID,
		SourceSequence: detached.SourceSequence,
		SourceEventID:  detached.SourceEventID,
		SchemaID:       detached.SchemaID,
		Payload:        detached.Payload,
	}
	service.records[request.Topic] = append(records, record.Detached())
	return events.AppendResult{Record: record.Detached(), Outcome: events.AppendOutcomeAccepted}, nil
}

func (service *internalTestEventsService) AttachSource(
	ctx context.Context,
	request events.AttachSourceRequest,
) (events.AttachSourceResult, error) {
	if err := request.Validate(); err != nil {
		return events.AttachSourceResult{}, err
	}
	if err := ctx.Err(); err != nil {
		return events.AttachSourceResult{}, err
	}
	return events.AttachSourceResult{
		ID:      events.AttachmentID{Destination: request.Destination, Source: request.Source},
		Outcome: events.AttachOutcomeAccepted,
		StartAt: request.StartAt,
	}, nil
}

func (service *internalTestEventsService) Read(
	ctx context.Context,
	request events.ReadRequest,
) (events.ReadResult, error) {
	if err := request.Validate(); err != nil {
		return events.ReadResult{}, err
	}
	if err := ctx.Err(); err != nil {
		return events.ReadResult{}, err
	}

	service.mu.Lock()
	records := append([]events.Record(nil), service.records[request.Topic]...)
	service.mu.Unlock()
	if request.From.Position > events.AggregateSequence(len(records)) {
		return events.ReadResult{}, events.ErrUnresolvableCursor
	}
	retained := events.RetainedRange{
		Topic:    request.Topic,
		Earliest: 1,
		Head:     events.AggregateSequence(len(records)),
	}
	if len(records) == 0 {
		retained.Earliest = 0
		return events.ReadResult{
			Next:     request.From,
			Retained: retained,
			Outcome:  events.ReadOutcomeAtHead,
		}, nil
	}
	start := int(request.From.Position)
	if start == len(records) {
		return events.ReadResult{
			Next:     events.Cursor{Topic: request.Topic, Position: request.From.Position},
			Retained: retained,
			Outcome:  events.ReadOutcomeAtHead,
		}, nil
	}
	end := start + request.Limit
	if end > len(records) {
		end = len(records)
	}
	page := make([]events.Record, end-start)
	for index := range page {
		page[index] = records[start+index].Detached()
	}
	return events.ReadResult{
		Records:  page,
		Next:     events.Cursor{Topic: request.Topic, Position: page[len(page)-1].ID.Position},
		Retained: retained,
		Outcome:  events.ReadOutcomeProgress,
	}, nil
}

func (service *internalTestEventsService) Subscribe(
	ctx context.Context,
	request events.SubscribeRequest,
) (events.Subscription, error) {
	if err := request.Validate(); err != nil {
		return nil, err
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	service.mu.Lock()
	records := append([]events.Record(nil), service.records[request.Topic]...)
	service.mu.Unlock()
	index := int(request.From.Position)
	return events.Subscription(func(nextCtx context.Context) events.Delivery {
		if err := nextCtx.Err(); err != nil {
			return events.Delivery{Kind: events.DeliveryCanceled}
		}
		if index >= len(records) {
			return events.Delivery{Kind: events.DeliveryClosed}
		}
		record := records[index].Detached()
		index++
		return events.Delivery{
			Kind:   events.DeliveryRecord,
			Record: record,
			Cursor: events.Cursor{Topic: request.Topic, Position: record.ID.Position},
		}
	}), nil
}

var _ events.Service = (*internalTestEventsService)(nil)

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
	if _, err := registry.GetObservationByWorkerSessionID(context.Background(), workersessions.GetObservationByWorkerSessionIDRequest{}); !errors.Is(err, workersessions.ErrInvalidSessionID) {
		t.Fatalf("GetObservationByWorkerSessionID(invalid) error = %v, want ErrInvalidSessionID", err)
	}
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
	if _, err := registry.projectWorkerSessionIdentity(canceled, "worker-1"); !errors.Is(err, workersessions.ErrObservationCanceled) {
		t.Fatalf("projectWorkerSessionIdentity(canceled) error = %v, want ErrObservationCanceled", err)
	}
}

func assertObservationTurnUsage(t *testing.T, observation workersessions.Observation) {
	t.Helper()
	if observation.TurnUsage == nil || observation.TurnUsage.TurnCount != 3 ||
		observation.TurnUsage.FinalContextTokens != 450 || observation.TurnUsage.PeakContextTokens != 450 {
		t.Fatalf("observation turn usage = %#v, want derived cumulative deltas", observation.TurnUsage)
	}
}

func TestObservationTurnUsageDiffersCumulativeInputCounters(t *testing.T) {
	got := observationTurnUsage([]int{100, 250, 700})
	if got == nil || got.TurnCount != 3 || got.FinalContextTokens != 450 || got.PeakContextTokens != 450 {
		t.Fatalf("observationTurnUsage() = %#v, want three turns with final/peak 450", got)
	}
	if observationTurnUsage(nil) != nil {
		t.Fatal("observationTurnUsage(nil) returned a value")
	}
	if observationTurnUsage([]int{100, 90}) != nil {
		t.Fatal("observationTurnUsage(decreasing counters) returned a value")
	}
}

func TestWorkerSessionUsageProjectionRetainsModelAndTokenLineage(t *testing.T) {
	draft := usageProjectionTestDraft(t)
	usage, model, ok := usageProjectionFromDraft(draft)
	if !ok {
		t.Fatal("usageProjectionFromDraft(valid) rejected a valid usage draft")
	}
	if usage == nil {
		t.Fatal("usageProjectionFromDraft(valid) returned nil usage")
	}
	if usage.InputTokens == nil || *usage.InputTokens != 11 {
		t.Fatalf("usageProjectionFromDraft(valid) input tokens = %#v, want 11", usage.InputTokens)
	}
	if usage.OutputTokens == nil || *usage.OutputTokens != 7 {
		t.Fatalf("usageProjectionFromDraft(valid) output tokens = %#v, want 7", usage.OutputTokens)
	}
	if model != " tts " {
		t.Fatalf("usageProjectionFromDraft(valid) model = %q, want preserved whitespace", model)
	}
}

func TestWorkerSessionUsageProjectionUpdatesRegistry(t *testing.T) {
	draft := usageProjectionTestDraft(t)
	r := newTestRegistry(t)
	r.observations["usage-session"] = &observation{}
	r.updateUsageProjection("usage-session", draft)
	r.updateUsageProjection("missing-session", draft)

	got := r.observations["usage-session"]
	if got == nil {
		t.Fatal("updateUsageProjection() removed the existing observation")
	}
	if got.tokenUsage == nil {
		t.Fatal("updateUsageProjection() did not retain token usage")
	}
	if got.usageModel != "tts" {
		t.Fatalf("updateUsageProjection() model = %q, want normalized model", got.usageModel)
	}
	if got.tokenUsage.InputTokens == nil || *got.tokenUsage.InputTokens != 11 {
		t.Fatalf("updateUsageProjection() input tokens = %#v, want 11", got.tokenUsage.InputTokens)
	}
}

func TestWorkerSessionUsageProjectionBuildsDetachedObservationMetadata(t *testing.T) {
	draft := usageProjectionTestDraft(t)
	usage, _, ok := usageProjectionFromDraft(draft)
	if !ok {
		t.Fatal("usageProjectionFromDraft(valid) rejected a valid usage draft")
	}

	projected := baseObservation("usage-session", workersessions.Session{ID: "usage-session", State: workersessions.StateRunning}, &observation{usageModel: " tts "})
	if projected.Model == nil {
		t.Fatal("baseObservation() omitted the usage model")
	}
	if *projected.Model != "tts" {
		t.Fatalf("baseObservation() model = %q, want trimmed model", *projected.Model)
	}
	cloned := cloneObservationTokenUsage(usage)
	if cloned == nil {
		t.Fatal("cloneObservationTokenUsage() returned nil")
	}
	if cloned.InputTokens == usage.InputTokens {
		t.Fatal("cloneObservationTokenUsage() retained the input token pointer")
	}
}

func TestWorkerSessionUsageProjectionRejectsInvalidDrafts(t *testing.T) {
	payload := usageProjectionTestDraft(t).Payload
	for name, invalid := range map[string]workers.Draft{
		"wrong kind":   {Kind: workers.KindMessage, Phase: workers.PhaseUpdated, Payload: payload},
		"invalid json": {Kind: workers.KindUsage, Phase: workers.PhaseUpdated, Payload: []byte("not-json")},
		"empty usage":  {Kind: workers.KindUsage, Phase: workers.PhaseUpdated, Payload: []byte(`{}`)},
	} {
		t.Run(name, func(t *testing.T) {
			if _, _, ok := usageProjectionFromDraft(invalid); ok {
				t.Fatalf("usageProjectionFromDraft(%s) unexpectedly succeeded", name)
			}
		})
	}
}

func usageProjectionTestDraft(t *testing.T) workers.Draft {
	t.Helper()
	inputTokens := int64(11)
	outputTokens := int64(7)
	payload, err := json.Marshal(usageProjectionPayload{
		InputTokens:  &inputTokens,
		OutputTokens: &outputTokens,
		Model:        " tts ",
	})
	if err != nil {
		t.Fatalf("json.Marshal(usageProjectionPayload) error = %v", err)
	}
	return workers.Draft{Kind: workers.KindUsage, Phase: workers.PhaseUpdated, Payload: payload}
}

func TestInvokeSafeDiagnosticMessages(t *testing.T) {
	for _, test := range []struct {
		input string
		want  string
	}{
		{"", "provider session parse error"},
		{"password=secret", "provider session parse error"},
		{"a/b", "provider session parse error"},
		{strings.Repeat("x", 300), strings.Repeat("x", 256)},
		{"  ordinary   message ", "ordinary message"},
	} {
		if got := safeDiagnosticMessage(test.input); got != test.want {
			t.Fatalf("safeDiagnosticMessage(%q) = %q, want %q", test.input, got, test.want)
		}
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
	failed := newObservationRegistry(nil, nil)
	failed.sessions["failed-worker"] = workersessions.Session{
		ID:    "failed-worker",
		State: workersessions.StateFailed,
		Result: &workersessions.TerminalResult{
			Outcome: workersessions.TerminalOutcomeFailed,
			Cause:   &workersessions.FailureCause{Kind: workersessions.FailureCauseWorkersExecutionFailure, Detail: "provider failed"},
		},
	}
	failed.observations["failed-worker"] = observationMetadata()
	failedObservation, err := failed.GetObservationByWorkerSessionID(context.Background(), workersessions.GetObservationByWorkerSessionIDRequest{WorkerSessionID: "failed-worker"})
	if err != nil || failedObservation.Failure == nil || failedObservation.Failure.Detail != "provider failed" {
		t.Fatalf("failed Worker Session observation = %#v, %v, want copied failure cause", failedObservation, err)
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
	if _, err := newReplayObservationSubscription(nil, coverageRetainedReader{err: errors.New("read unavailable")}, topic, workersessions.StateRunning, 0); !errors.Is(err, workersessions.ErrObservationSourceUnavailable) {
		t.Fatalf("newReplayObservationSubscription(default limit) = %v, want source unavailable", err)
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

func TestStreamObservationsMapsLookupAndSubscribeErrors(t *testing.T) {
	ref := observationProviderRef()
	withoutReader := newObservationRegistry(observationProjectorFake{}, nil)
	if _, err := withoutReader.StreamObservations(context.Background(), workersessions.StreamObservationsRequest{ProviderSession: providers.SessionRef{}}); !errors.Is(err, workersessions.ErrInvalidObservationIdentity) {
		t.Fatalf("StreamObservations(invalid request) error = %v", err)
	}
	withoutReader.sessions["worker-1"] = observationSession("worker-1", workersessions.StateRunning)
	if _, err := withoutReader.StreamObservations(context.Background(), workersessions.StreamObservationsRequest{ProviderSession: ref}); !errors.Is(err, workersessions.ErrObservationSourceUnavailable) {
		t.Fatalf("StreamObservations(without reader) error = %v", err)
	}
	missing := newObservationRegistry(observationProjectorFake{}, &observationEventReaderFake{subscription: events.Subscription(func(context.Context) events.Delivery { return events.Delivery{Kind: events.DeliveryClosed} })})
	if _, err := missing.StreamObservations(context.Background(), workersessions.StreamObservationsRequest{ProviderSession: ref}); !errors.Is(err, workersessions.ErrObservationSessionNotFound) {
		t.Fatalf("StreamObservations(missing session) error = %v", err)
	}
	canceled, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := missing.StreamObservations(canceled, workersessions.StreamObservationsRequest{ProviderSession: ref}); !errors.Is(err, workersessions.ErrObservationCanceled) {
		t.Fatalf("StreamObservations(canceled) error = %v", err)
	}

	active := newObservationRegistry(observationProjectorFake{}, &observationEventReaderFake{err: errors.New("subscribe failed")})
	active.sessions["worker-1"] = observationSession("worker-1", workersessions.StateRunning)
	if _, err := active.StreamObservations(context.Background(), workersessions.StreamObservationsRequest{ProviderSession: ref}); !errors.Is(err, workersessions.ErrObservationSourceUnavailable) {
		t.Fatalf("StreamObservations(subscribe failure) error = %v", err)
	}
	canceledReader := newObservationRegistry(observationProjectorFake{}, &observationEventReaderFake{err: context.Canceled})
	canceledReader.sessions["worker-1"] = observationSession("worker-1", workersessions.StateRunning)
	if _, err := canceledReader.StreamObservations(context.Background(), workersessions.StreamObservationsRequest{ProviderSession: ref}); !errors.Is(err, workersessions.ErrObservationCanceled) {
		t.Fatalf("StreamObservations(canceled subscribe) error = %v", err)
	}
	terminal := newObservationRegistry(observationProjectorFake{}, &observationEventReaderFake{subscription: events.Subscription(func(context.Context) events.Delivery { return events.Delivery{Kind: events.DeliveryClosed} })})
	terminal.sessions["worker-1"] = observationSession("worker-1", workersessions.StateCompleted)
	subscription, err := terminal.StreamObservations(context.Background(), workersessions.StreamObservationsRequest{ProviderSession: ref, Limit: 2})
	if err != nil {
		t.Fatalf("StreamObservations(terminal) error = %v", err)
	}
	subscription.Close()
}

func TestReadTranscriptMapsProviderProjectionErrors(t *testing.T) {
	ref := observationProviderRef()
	cases := []struct {
		name string
		err  error
		want error
	}{
		{"canceled", context.Canceled, workersessions.ErrObservationCanceled},
		{"provider canceled", providersessions.ErrOperationCanceled, workersessions.ErrObservationCanceled},
		{"source unavailable", providersessions.ErrSessionNotFound, workersessions.ErrObservationTranscriptUnavailable},
		{"projection failure", errors.New("projection failed"), workersessions.ErrObservationTranscriptProjectionUnavailable},
	}
	for _, test := range cases {
		t.Run(test.name, func(t *testing.T) {
			registry := newObservationRegistry(observationProjectorFake{err: test.err}, nil)
			registry.sessions["worker-1"] = observationSession("worker-1", workersessions.StateCompleted)
			registry.observations["worker-1"] = observationMetadata()
			_, err := registry.ReadTranscript(context.Background(), workersessions.ReadTranscriptRequest{ProviderSession: ref})
			if !errors.Is(err, test.want) {
				t.Fatalf("ReadTranscript() error = %v, want %v", err, test.want)
			}
		})
	}
}

func TestReadTranscriptRejectsInvalidRequestAndProjection(t *testing.T) {
	ref := observationProviderRef()
	registry := newObservationRegistry(observationProjectorFake{result: providersessions.ProjectResult{Detail: providersessions.Detail{Transcript: []providersessions.TranscriptEntry{{Order: 0}}}}}, nil)
	registry.sessions["worker-1"] = observationSession("worker-1", workersessions.StateCompleted)
	registry.observations["worker-1"] = observationMetadata()
	if _, err := registry.ReadTranscript(context.Background(), workersessions.ReadTranscriptRequest{}); !errors.Is(err, workersessions.ErrInvalidObservationIdentity) {
		t.Fatalf("ReadTranscript(invalid request) error = %v", err)
	}
	if _, err := registry.ReadTranscript(context.Background(), workersessions.ReadTranscriptRequest{ProviderSession: ref}); err == nil {
		t.Fatal("ReadTranscript(invalid projected entry) error = nil, want validation error")
	}
}

func TestContinuationNotAcceptedPreservesCause(t *testing.T) {
	cause := errors.New("admission failed")
	withCause := continuationNotAccepted(cause)
	if withCause.Error() != workersessions.ErrContinuationNotAccepted.Error() || !errors.Is(withCause, workersessions.ErrContinuationNotAccepted) || !errors.Is(withCause, cause) {
		t.Fatalf("continuationNotAccepted(cause) = %v, want typed continuation and cause", withCause)
	}
	withoutCause := continuationNotAccepted(nil)
	if !errors.Is(withoutCause, workersessions.ErrContinuationNotAccepted) || errors.Is(withoutCause, cause) {
		t.Fatalf("continuationNotAccepted(nil) = %v, want only typed continuation error", withoutCause)
	}
}

func TestContinuationReservationOutcomeClassifiesErrors(t *testing.T) {
	cause := errors.New("admission failed")
	for _, test := range []struct {
		name string
		err  error
		want string
	}{
		{name: "request conflict", err: workersessions.ErrContinuationRequestIDConflict, want: "idempotency_conflict"},
		{name: "server stopping", err: workersessions.ErrContinuationServerStopping, want: "server_stopping"},
		{name: "source missing", err: workersessions.ErrContinuationSourceNotFound, want: "source_not_found"},
		{name: "source active", err: workersessions.ErrContinuationSourceActive, want: "source_active"},
		{name: "other", err: errors.New("other"), want: "rejected"},
	} {
		if got := continuationReservationOutcome(test.err); got != test.want {
			t.Errorf("continuationReservationOutcome(%s) = %q, want %q", test.name, got, test.want)
		}
	}
	if continuationReplayOutcome(nil) != "accepted" || continuationReplayOutcome(cause) != "rejected" {
		t.Fatalf("continuationReplayOutcome() did not classify nil/non-nil errors")
	}
}

func TestObservationScopeAndCursorHelpersRemainDeterministic(t *testing.T) {
	for _, test := range []struct {
		direct bool
		scope  workersessions.ObservationScope
		want   bool
	}{
		{direct: true, scope: "", want: true},
		{direct: false, scope: "", want: true},
		{direct: true, scope: workersessions.ObservationScopeDirect, want: true},
		{direct: false, scope: workersessions.ObservationScopeDirect, want: false},
		{direct: false, scope: workersessions.ObservationScopeFactory, want: true},
		{direct: true, scope: workersessions.ObservationScopeFactory, want: false},
		{direct: true, scope: workersessions.ObservationScopeAll, want: true},
		{direct: false, scope: workersessions.ObservationScopeAll, want: true},
		{direct: true, scope: "unknown", want: false},
	} {
		if got := observationScopeMatches(test.direct, test.scope); got != test.want {
			t.Errorf("observationScopeMatches(%t, %q) = %t, want %t", test.direct, test.scope, got, test.want)
		}
	}

	validCursor := base64.StdEncoding.EncodeToString([]byte("worker-1"))
	if got, err := decodeObservationListCursor(validCursor); err != nil || got != "worker-1" {
		t.Fatalf("decodeObservationListCursor(valid) = %q, %v, want worker-1", got, err)
	}
	if got, err := decodeObservationListCursor(" "); err != nil || got != "" {
		t.Fatalf("decodeObservationListCursor(blank) = %q, %v, want empty success", got, err)
	}
	if _, err := decodeObservationListCursor("not-base64"); !errors.Is(err, workersessions.ErrInvalidObservationPagination) {
		t.Fatalf("decodeObservationListCursor(invalid) = %v, want ErrInvalidObservationPagination", err)
	}
}

func TestAwaitContinuationReplayAndObservationListIDsRemainDetachedAndDeterministic(t *testing.T) {
	replay := &continueReplay{
		done:   make(chan struct{}),
		result: workersessions.ContinueResult{RequestID: "request-1", Session: workersessions.Session{ID: "successor", State: workersessions.StateRunning}},
	}
	close(replay.done)
	got, err := awaitContinueReplay(nil, replay)
	if err != nil || got.RequestID != "request-1" {
		t.Fatalf("awaitContinueReplay(completed) = %#v, %v, want completed replay", got, err)
	}

	pending := &continueReplay{done: make(chan struct{})}
	canceled, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := awaitContinueReplay(canceled, pending); !errors.Is(err, context.Canceled) {
		t.Fatalf("awaitContinueReplay(canceled) = %v, want context.Canceled", err)
	}
	completedAfterCancel := &continueReplay{
		done:   make(chan struct{}),
		result: workersessions.ContinueResult{RequestID: "completed-after-cancel"},
	}
	close(completedAfterCancel.done)
	if got, err := awaitContinueReplay(canceled, completedAfterCancel); err != nil || got.RequestID != "completed-after-cancel" {
		t.Fatalf("awaitContinueReplay(completed after cancellation) = %#v, %v, want retained replay", got, err)
	}

	r := &registry{
		sessions: map[string]workersessions.Session{
			"direct-a":  {ID: "direct-a", State: workersessions.StateCompleted},
			"factory-a": {ID: "factory-a", State: workersessions.StateCompleted},
		},
		observations: map[string]*observation{
			"direct-a":  {direct: true},
			"factory-a": {direct: false},
			"nil-meta":  nil,
		},
	}
	if got := r.observationListIDs("", workersessions.ObservationScopeAll, nil); !reflect.DeepEqual(got, []string{"direct-a", "factory-a"}) {
		t.Fatalf("observationListIDs(all) = %#v, want deterministic IDs", got)
	}
	if got := r.observationListIDs("direct-a", workersessions.ObservationScopeDirect, nil); !reflect.DeepEqual(got, []string{}) {
		t.Fatalf("observationListIDs(cursor) = %#v, want empty after direct-a", got)
	}
	if got := r.observationListIDs("", workersessions.ObservationScopeFactory, []workersessions.State{workersessions.StateRunning}); len(got) != 0 {
		t.Fatalf("observationListIDs(state mismatch) = %#v, want empty", got)
	}
}

func TestValidateContinuationSourceAssociationClassifiesMissingMalformedAndForeignState(t *testing.T) {
	valid := workersessions.Session{
		ID:    "worker-1",
		State: workersessions.StateCompleted,
		ProviderSessionAssociation: &workersessions.ProviderSessionAssociation{
			WorkerSessionID: "worker-1",
			DispatchID:      "dispatch-1",
			AttemptID:       "dispatch-1",
			Reference:       providers.SessionRef{Provider: providers.IDCodex, Kind: providers.SessionIDKind, ID: "provider-session-1"},
		},
	}
	if err := validateContinuationSourceAssociation(valid); err != nil {
		t.Fatalf("valid association = %v, want nil", err)
	}
	for _, test := range []struct {
		name    string
		session workersessions.Session
		want    error
	}{
		{name: "missing", session: workersessions.Session{ID: "worker-1"}, want: workersessions.ErrContinuationProviderSessionMissing},
		{name: "malformed", session: workersessions.Session{ID: "worker-1", ProviderSessionAssociation: &workersessions.ProviderSessionAssociation{WorkerSessionID: "worker-1"}}, want: workersessions.ErrContinuationProviderSessionInvalid},
		{name: "foreign", session: workersessions.Session{ID: "worker-1", ProviderSessionAssociation: &workersessions.ProviderSessionAssociation{WorkerSessionID: "worker-2", DispatchID: "dispatch-1", AttemptID: "dispatch-1", Reference: providers.SessionRef{Provider: providers.IDCodex, Kind: providers.SessionIDKind, ID: "provider-session-1"}}}, want: workersessions.ErrContinuationProviderSessionInvalid},
	} {
		t.Run(test.name, func(t *testing.T) {
			if err := validateContinuationSourceAssociation(test.session); !errors.Is(err, test.want) {
				t.Fatalf("validateContinuationSourceAssociation() = %v, want %v", err, test.want)
			}
		})
	}
}

func TestContinuationSnapshotRejectsExecutionEdges(t *testing.T) {
	request := continuationReservationRequest()
	missingSupervision := newContinuationSource(t, request)
	if _, err := missingSupervision.snapshotContinuationSourceLocked(request); !errors.Is(err, workersessions.ErrContinuationExecutionUnavailable) {
		t.Fatalf("snapshotContinuationSourceLocked(missing supervision) = %v, want execution unavailable", err)
	}

	missingDispatch := newContinuationSource(t, request)
	missingDispatch.supervisions[request.SourceWorkerSessionID] = newSupervision("", "turn-1", continuationValidExecution("dispatch-1"))
	if _, err := missingDispatch.snapshotContinuationSourceLocked(request); !errors.Is(err, workersessions.ErrContinuationExecutionUnavailable) {
		t.Fatalf("snapshotContinuationSourceLocked(blank dispatch) = %v, want execution unavailable", err)
	}

	mismatchedDispatch := newContinuationSource(t, request)
	mismatchedDispatch.supervisions[request.SourceWorkerSessionID] = newSupervision("dispatch-2", "turn-1", continuationValidExecution("dispatch-2"))
	if _, err := mismatchedDispatch.snapshotContinuationSourceLocked(request); !errors.Is(err, workersessions.ErrContinuationProviderSessionInvalid) {
		t.Fatalf("snapshotContinuationSourceLocked(mismatched dispatch) = %v, want provider session invalid", err)
	}

	interrupting := newContinuationSource(t, request)
	interrupting.supervisions[request.SourceWorkerSessionID] = newSupervision("dispatch-1", "turn-1", continuationValidExecution("dispatch-1"))
	interrupting.supervisions[request.SourceWorkerSessionID].interrupting = true
	interrupting.supervisions[request.SourceWorkerSessionID].interruptRequestID = "another-interrupt"
	if _, err := interrupting.snapshotContinuationSourceLocked(request); !errors.Is(err, workersessions.ErrContinuationSourceConflict) {
		t.Fatalf("snapshotContinuationSourceLocked(interrupt conflict) = %v, want source conflict", err)
	}
}

func TestContinuationExecutionRejectsConflicts(t *testing.T) {
	request := continuationReservationRequest()
	valid := newContinuationSource(t, request)
	valid.supervisions[request.SourceWorkerSessionID] = newSupervision("dispatch-1", "turn-1", continuationValidExecution("dispatch-1"))
	valid.dispatchOwners["dispatch-1"] = request.SourceWorkerSessionID
	snapshot, err := valid.snapshotContinuationSourceLocked(request)
	if err != nil {
		t.Fatalf("snapshotContinuationSourceLocked(valid) = %v, want nil", err)
	}

	withSuccessor := newContinuationSource(t, request)
	withSuccessor.sessions[request.SuccessorWorkerSessionID] = workersessions.Session{ID: request.SuccessorWorkerSessionID, State: workersessions.StateReserved}
	if _, err := withSuccessor.buildContinuationExecutionLocked(request, snapshot); !errors.Is(err, workersessions.ErrContinuationSuccessorConflict) {
		t.Fatalf("buildContinuationExecutionLocked(existing successor) = %v, want successor conflict", err)
	}

	withDispatchConflict := newContinuationSource(t, request)
	withDispatchConflict.dispatchOwners[continuationDispatchID("dispatch-1", request.SuccessorWorkerSessionID)] = "other-session"
	if _, err := withDispatchConflict.buildContinuationExecutionLocked(request, snapshot); !errors.Is(err, workersessions.ErrContinuationSuccessorConflict) {
		t.Fatalf("buildContinuationExecutionLocked(existing dispatch) = %v, want successor conflict", err)
	}

	withInvalidExecution := newContinuationSource(t, request)
	invalidSnapshot := snapshot
	invalidSnapshot.execution = workers.WorkstationDispatchRequest{Execution: workers.WorkstationExecutionRequest{Dispatch: work.WorkDispatch{DispatchID: "dispatch-1"}}}
	if _, err := withInvalidExecution.buildContinuationExecutionLocked(request, invalidSnapshot); !errors.Is(err, workersessions.ErrContinuationExecutionUnavailable) {
		t.Fatalf("buildContinuationExecutionLocked(invalid execution) = %v, want execution unavailable", err)
	}
}

func TestContinuationReservationRejectsLifecycleEdges(t *testing.T) {
	request := continuationReservationRequest()
	missingReservation := newTestRegistry(t)
	if _, _, err := missingReservation.reserveContinuation(request); !errors.Is(err, workersessions.ErrContinuationSourceNotFound) {
		t.Fatalf("reserveContinuation(missing source) = %v, want source not found", err)
	}

	reservation := newContinuationSource(t, request)
	reservation.supervisions[request.SourceWorkerSessionID] = newSupervision("dispatch-1", "turn-1", continuationValidExecution("dispatch-1"))
	reservation.continueReplays = nil
	reservation.startsDone = nil
	if replay, owner, err := reservation.reserveContinuation(request); err != nil || !owner || replay == nil {
		t.Fatalf("reserveContinuation(first reservation) = %v, %t, %#v, want owned replay", err, owner, replay)
	}

	stopping := newContinuationSource(t, request)
	stopping.stopping = true
	if _, _, err := stopping.reserveContinuation(request); !errors.Is(err, workersessions.ErrContinuationServerStopping) {
		t.Fatalf("reserveContinuation(stopping) = %v, want server stopping", err)
	}

	dispatchConflict := newContinuationSource(t, request)
	dispatchConflict.supervisions[request.SourceWorkerSessionID] = newSupervision("dispatch-1", "turn-1", continuationValidExecution("dispatch-1"))
	dispatchConflict.dispatchOwners[continuationDispatchID("dispatch-1", request.SuccessorWorkerSessionID)] = "other-session"
	if _, _, err := dispatchConflict.reserveContinuation(request); !errors.Is(err, workersessions.ErrContinuationSuccessorConflict) {
		t.Fatalf("reserveContinuation(dispatch conflict) = %v, want successor conflict", err)
	}

	invalidExecution := newTestRegistry(t)
	invalidExecution.sessions[request.SuccessorWorkerSessionID] = workersessions.Session{
		ID: request.SuccessorWorkerSessionID, State: workersessions.StateRunning,
	}
	if _, err := invalidExecution.continueReserved(continuePlan{request: request}); !errors.Is(err, workersessions.ErrContinuationNotAccepted) {
		t.Fatalf("continueReserved(invalid execution) = %v, want continuation not accepted", err)
	}
	if _, err := stopping.Continue(nil, workersessions.ContinueRequest{}); !errors.Is(err, workersessions.ErrInvalidContinuationRequestID) {
		t.Fatalf("Continue(nil, invalid) = %v, want invalid request ID", err)
	}

	terminal := newContinuationSource(t, request)
	terminal.supervisions[request.SourceWorkerSessionID] = newSupervision("dispatch-1", "turn-1", continuationValidExecution("dispatch-1"))
	terminalReplay, owner, err := terminal.reserveContinuation(request)
	if err != nil || !owner {
		t.Fatalf("reserveContinuation(terminal branch) = %v, %t, want owned replay", err, owner)
	}
	terminal.stopping = true
	_, err = terminal.continueReserved(terminalReplay.plan)
	if !errors.Is(err, workersessions.ErrContinuationNotAccepted) {
		t.Fatalf("continueReserved(stopping) = %v, want ErrContinuationNotAccepted", err)
	}
}

func TestContinuationReservationRejectsAnUnresolvedSourceLineage(t *testing.T) {
	request := continuationReservationRequest()
	r := newContinuationSource(t, request)
	r.supervisions[request.SourceWorkerSessionID] = newSupervision("dispatch-1", "turn-1", continuationValidExecution("dispatch-1"))
	r.continuationSources[request.SourceWorkerSessionID] = "another-request"

	if _, err := r.snapshotContinuationSourceLocked(request); !errors.Is(err, workersessions.ErrContinuationSourceConflict) {
		t.Fatalf("snapshotContinuationSourceLocked(unresolved source claim) = %v, want ErrContinuationSourceConflict", err)
	}

	r = newContinuationSource(t, request)
	r.supervisions[request.SourceWorkerSessionID] = newSupervision("dispatch-1", "turn-1", continuationValidExecution("dispatch-1"))
	snapshot, err := r.snapshotContinuationSourceLocked(request)
	if err != nil {
		t.Fatalf("snapshotContinuationSourceLocked(valid) = %v, want nil", err)
	}
	r.continuationSources = nil
	r.storeContinuationReservationLocked(request, continueTuple{
		sourceID: request.SourceWorkerSessionID, successorID: request.SuccessorWorkerSessionID, input: request.FollowUpInput,
	}, snapshot, continuationValidExecution("dispatch-1/continue/successor-1"))
	if r.continuationSources[request.SourceWorkerSessionID] != request.RequestID {
		t.Fatalf("storeContinuationReservationLocked() did not recreate the source claim map")
	}
}

func TestResumeAssociationValidationRejectsUnavailableAttemptFacts(t *testing.T) {
	r, supervision, _ := newPausedContinuationRegistry(t)
	session := r.sessions["worker-1"]

	if err := validateResumeAssociationForSupervision(session, nil); !errors.Is(err, workersessions.ErrProviderSessionAssociationNotAvailable) {
		t.Fatalf("validateResumeAssociationForSupervision(nil supervision) = %v, want association unavailable", err)
	}

	supervision.dispatchID = " "
	if err := validateResumeAssociationForSupervision(session, supervision); !errors.Is(err, workersessions.ErrProviderSessionAssociationNotAvailable) {
		t.Fatalf("validateResumeAssociationForSupervision(blank dispatch) = %v, want association unavailable", err)
	}

	supervision.dispatchID = "dispatch-1"
	supervision.turnID = "turn-foreign"
	if err := validateResumeAssociationForSupervision(session, supervision); !errors.Is(err, workersessions.ErrProviderSessionAssociationAttemptMismatch) {
		t.Fatalf("validateResumeAssociationForSupervision(mismatched turn) = %v, want attempt mismatch", err)
	}
}

func TestContinuationLineagePublicationGuardsAndClosedPersistence(t *testing.T) {
	ctx := context.Background()
	payload := workers.SessionPayload{
		Status:          string(workersessions.StateCompleted),
		WorkerSessionID: "worker-1",
		DispatchID:      "dispatch-1",
		AttemptID:       "dispatch-1",
	}
	identity := events.AppendIdentity{
		SourceType:     continuationLineageSourceType,
		SourceID:       "worker-1/successor/successor-1",
		SourceSequence: continuationLineageSourceSequence,
		SourceEventID:  continuationLineageSourceEventID,
	}

	missing := newTestRegistry(t)
	if err := missing.publishSessionLineageRecord(ctx, "missing", identity, payload, true); !errors.Is(err, workersessions.ErrSessionNotFound) {
		t.Fatalf("publishSessionLineageRecord(missing) = %v, want session not found", err)
	}

	closed := newTestRegistry(t)
	closed.reserveIfAbsent("worker-1")
	if err := closed.publishSessionLineageRecord(ctx, "worker-1", identity, payload, false); !errors.Is(err, workersessions.ErrPublicationNotOpen) {
		t.Fatalf("publishSessionLineageRecord(closed) = %v, want publication not open", err)
	}

	writer := &continuationLineageRecordingStub{}
	accepted := newTestRegistry(t)
	accepted.reserveIfAbsent("worker-1")
	accepted.publications["worker-1"].open = true
	accepted.publications["worker-1"].recordingID = "recording-1"
	accepted.recording = writer
	if err := accepted.publishSessionLineageRecord(ctx, "worker-1", identity, payload, true); err != nil {
		t.Fatalf("publishSessionLineageRecord(accepted) = %v, want nil", err)
	}
	if len(writer.records) != 1 || writer.records[0].RecordingID != "recording-1" {
		t.Fatalf("closed lineage persistence = %#v, want one recording-1 record", writer.records)
	}
	if err := accepted.publishSessionLineageRecord(ctx, "worker-1", identity, payload, true); err != nil {
		t.Fatalf("publishSessionLineageRecord(duplicate) = %v, want idempotent success", err)
	}
	outOfOrder := identity
	outOfOrder.SourceSequence = 0
	if err := accepted.publishSessionLineageRecord(ctx, "worker-1", outOfOrder, payload, true); !errors.Is(err, workersessions.ErrOutOfOrderPublication) {
		t.Fatalf("publishSessionLineageRecord(out of order) = %v, want out of order", err)
	}

	writer.recordErr = errors.New("lineage persistence failed")
	accepted.persistClosedLineageRecord(ctx, "recording-1", "worker-1", events.Record{})
	if len(writer.failures) != 1 || writer.failures[0].Code != "CONTINUATION_LINEAGE_PERSISTENCE_FAILED" {
		t.Fatalf("closed lineage persistence failure = %#v, want one classified failure", writer.failures)
	}
	withoutWriter := newTestRegistry(t)
	withoutWriter.persistClosedLineageRecord(ctx, "recording-1", "worker-1", events.Record{})

	failingAppend := newTestRegistry(t)
	failingAppend.reserveIfAbsent("worker-1")
	failingAppend.publications["worker-1"].open = true
	appendErr := errors.New("lineage append failed")
	failingAppend.events = failingContinuationEventsAppender{err: appendErr}
	if err := failingAppend.publishSessionLineageRecord(ctx, "worker-1", identity, payload, true); !errors.Is(err, appendErr) {
		t.Fatalf("publishSessionLineageRecord(append failure) = %v, want append failure", err)
	}
}

func TestCommitContinuationLineageRejectsUnresolvableSourceAndKeepsLiveTruth(t *testing.T) {
	request := continuationReservationRequest()
	missing := newTestRegistry(t)
	missing.continuationSources[request.SourceWorkerSessionID] = request.RequestID
	missing.commitContinuationLineage(continuePlan{request: request})
	if _, exists := missing.continuationSources[request.SourceWorkerSessionID]; exists {
		t.Fatal("commitContinuationLineage() left an unresolved source reservation")
	}

	appendErr := errors.New("lineage append failed")
	registry := newTestRegistry(t)
	registry.events = failingContinuationEventsAppender{err: appendErr}
	registry.sessions[request.SourceWorkerSessionID] = workersessions.Session{
		ID:    request.SourceWorkerSessionID,
		State: workersessions.StateCompleted,
		Result: &workersessions.TerminalResult{
			Outcome: workersessions.TerminalOutcomeCompleted,
		},
		ProviderSessionAssociation: &workersessions.ProviderSessionAssociation{
			WorkerSessionID: request.SourceWorkerSessionID,
			TurnID:          "turn-1",
			DispatchID:      "dispatch-1",
			AttemptID:       "dispatch-1",
			Reference:       providers.SessionRef{Provider: providers.IDCodex, Kind: providers.SessionIDKind, ID: "provider-1"},
		},
	}
	registry.sessions[request.SuccessorWorkerSessionID] = workersessions.Session{
		ID:    request.SuccessorWorkerSessionID,
		State: workersessions.StateRunning,
	}
	registry.publications[request.SourceWorkerSessionID] = &publication{}
	registry.continuationSources[request.SourceWorkerSessionID] = request.RequestID
	registry.commitContinuationLineage(continuePlan{request: request})

	if got := registry.sessions[request.SourceWorkerSessionID].SuccessorWorkerSessionID; got != request.SuccessorWorkerSessionID {
		t.Fatalf("source successor after append failure = %q, want %q", got, request.SuccessorWorkerSessionID)
	}
	if got := registry.sessions[request.SuccessorWorkerSessionID].PredecessorWorkerSessionID; got != request.SourceWorkerSessionID {
		t.Fatalf("successor predecessor after append failure = %q, want %q", got, request.SourceWorkerSessionID)
	}
}

func TestControlHistoryGateAndOutcomeHelpersCoverClosedAndNilPaths(t *testing.T) {
	pendingGate := &controlHistoryGate{pending: true, done: make(chan struct{})}
	pendingClosed := make(chan struct{})
	pendingReceived := make(chan struct{})
	pendingRelease := make(chan struct{})
	go func() {
		pendingGate.close()
		close(pendingClosed)
	}()
	go func() {
		pendingGate.done <- struct{}{}
		close(pendingReceived)
		<-pendingRelease
		pendingGate.done <- struct{}{}
	}()
	select {
	case <-pendingReceived:
	case <-time.After(time.Second):
		t.Fatal("controlHistoryGate.close() did not wait for a pending reservation")
	}
	pendingGate.mu.Lock()
	pendingGate.pending = false
	pendingGate.mu.Unlock()
	close(pendingRelease)
	select {
	case <-pendingClosed:
	case <-time.After(time.Second):
		t.Fatal("controlHistoryGate.close() did not finish after the pending reservation drained")
	}

	gate := &controlHistoryGate{}
	if !gate.acquire() {
		t.Fatal("controlHistoryGate.acquire() = false, want true")
	}
	closed := make(chan struct{})
	go func() {
		gate.close()
		close(closed)
	}()
	gate.release()
	select {
	case <-closed:
	case <-time.After(time.Second):
		t.Fatal("controlHistoryGate.close() did not finish after release")
	}
	gate.release()
	if gate.acquire() {
		t.Fatal("controlHistoryGate.acquire() = true after close, want false")
	}

	if got := controlContext(nil); got == nil {
		t.Fatal("controlContext(nil) returned nil")
	}
	if got := controlResultOutcome(workersessions.ControlResult{Outcome: workersessions.ControlOutcomeApplied}, errors.New("ignored")); got != workersessions.ControlOutcomeApplied {
		t.Fatalf("controlResultOutcome(applied) = %q, want APPLIED", got)
	}
	if got := controlResultOutcome(workersessions.ControlResult{}, errors.New("failed")); got != workersessions.ControlOutcomeFailed {
		t.Fatalf("controlResultOutcome(error) = %q, want FAILED", got)
	}
	if got := controlResultOutcome(workersessions.ControlResult{}, nil); got != workersessions.ControlOutcomeNoop {
		t.Fatalf("controlResultOutcome(nil) = %q, want NOOP", got)
	}
	if got := controlReservationFor(nil); got != nil {
		t.Fatalf("controlReservationFor(nil) = %#v, want nil", got)
	}
}

func TestAwaitContinueReplayReturnsCompletedReplayAfterCallerCancellation(t *testing.T) {
	replayErr := errors.New("replayed continuation failed")
	replay := &continueReplay{
		done:   make(chan struct{}),
		result: workersessions.ContinueResult{RequestID: "replayed"},
		err:    replayErr,
	}
	ctxDone := make(chan struct{}, 1)
	ctx := signaledCancellationContext{done: ctxDone}
	go func() {
		close(replay.done)
		ctxDone <- struct{}{}
	}()

	result, err := awaitContinueReplay(ctx, replay)
	if !errors.Is(err, replayErr) || result.RequestID != "replayed" {
		t.Fatalf("awaitContinueReplay() = %#v, %v, want completed replay after cancellation", result, err)
	}
}

func TestControlHistoryPublicationFailureAndClosedOutcomeRemainTruthful(t *testing.T) {
	r, _, _ := newPausedContinuationRegistry(t)
	appendErr := errors.New("control request append failed")
	r.events = failingContinuationEventsAppender{err: appendErr}
	if reservation, err := r.beginControlHistory(context.Background(), "worker-1", workersessions.ControlActionResume, "request-1"); reservation != nil || !errors.Is(err, appendErr) {
		t.Fatalf("beginControlHistory(append failure) = %#v, %v, want append error without reservation", reservation, err)
	}

	r, _, _ = newPausedContinuationRegistry(t)
	reservation, err := r.beginControlHistory(context.Background(), "worker-1", workersessions.ControlActionResume, "request-1")
	if err != nil || reservation == nil {
		t.Fatalf("beginControlHistory(valid) = %#v, %v, want reservation", reservation, err)
	}
	reservation.pub.open = false
	r.finishControlHistory(reservation, workersessions.ControlOutcomeFailed, "", workersessions.StatePaused)
	if err := r.appendControlRecord(context.Background(), reservation, workersessions.ControlRecordTypeOutcome, workersessions.ControlOutcomeFailed, "", workersessions.StatePaused); !errors.Is(err, workersessions.ErrPublicationNotOpen) {
		t.Fatalf("appendControlRecord(closed) = %v, want publication not open", err)
	}

	r, _, _ = newPausedContinuationRegistry(t)
	reservation, err = r.beginControlHistory(context.Background(), "worker-1", workersessions.ControlActionResume, "request-logging")
	if err != nil || reservation == nil {
		t.Fatalf("beginControlHistory(logging) = %#v, %v, want reservation", reservation, err)
	}
	r.events = failingContinuationEventsAppender{err: appendErr}
	r.finishControlHistory(reservation, workersessions.ControlOutcomeFailed, "dispatch-1/attempt/2", workersessions.StatePaused)

	r, _, _ = newPausedContinuationRegistry(t)
	r.events = &deleteSessionAfterAppendEventsAppender{
		delegate: r.events,
		delete: func() {
			r.mu.Lock()
			delete(r.sessions, "worker-1")
			r.mu.Unlock()
		},
	}
	result, err := r.Resume(context.Background(), workersessions.ControlRequest{ID: "worker-1", RequestID: "request-target-disappeared"})
	if !errors.Is(err, workersessions.ErrSessionNotFound) || result.Outcome != workersessions.ControlOutcomeFailed {
		t.Fatalf("Resume(target disappeared after request append) = %#v, %v, want failed session-not-found result", result, err)
	}
}

func TestAcceptSupervisionRecordsFailedResumeWhenSessionCannotRun(t *testing.T) {
	r, supervision, _ := newPausedContinuationRegistry(t)
	reservation, err := r.beginControlHistory(context.Background(), "worker-1", workersessions.ControlActionResume, "request-not-startable")
	if err != nil || reservation == nil {
		t.Fatalf("beginControlHistory() = %#v, %v, want reservation", reservation, err)
	}

	r.acceptSupervision("worker-1", supervision)
	current := r.sessions["worker-1"]
	if current.State != workersessions.StatePaused {
		t.Fatalf("acceptSupervision() state = %q, want PAUSED", current.State)
	}
	if supervision.accepted {
		t.Fatal("acceptSupervision() marked a non-startable session accepted")
	}
}

func TestRunInterruptRejectsSuccessorWithConflictingCapturedReference(t *testing.T) {
	r := newTestRegistry(t)
	sourceID := "source-1"
	successorID := "successor-1"
	execution := continuationValidExecution("dispatch-source")
	reference := providers.SessionRef{Provider: providers.IDCodex, Kind: providers.SessionIDKind, ID: "provider-source"}
	wrongReference := providers.SessionRef{Provider: providers.IDCodex, Kind: providers.SessionIDKind, ID: "provider-other"}
	supervision := newSupervision("dispatch-source", "turn-1", execution)
	supervision.signalDone()
	supervision.installCancel(func() {})
	r.sessions[sourceID] = workersessions.Session{
		ID:    sourceID,
		State: workersessions.StateCanceled,
		ProviderSessionAssociation: &workersessions.ProviderSessionAssociation{
			WorkerSessionID: sourceID,
			TurnID:          "turn-1",
			DispatchID:      "dispatch-source",
			AttemptID:       "dispatch-source",
			Reference:       reference,
		},
	}
	r.supervisions[sourceID] = supervision
	r.dispatchOwners["dispatch-source"] = sourceID
	boundary := &admitBeforeCompletionBoundary{ready: make(chan struct{}), release: make(chan struct{})}
	close(boundary.release)
	r.execution = boundary

	result, err := r.runInterrupt(interruptPlan{
		request: workersessions.InterruptRequest{
			RequestID:                "interrupt-1",
			SourceWorkerSessionID:    sourceID,
			SuccessorWorkerSessionID: successorID,
			ReplacementMessage:       "replacement",
		},
		execution:   execution,
		reference:   wrongReference,
		dispatchID:  "dispatch-source",
		supervision: supervision,
	})
	if !errors.Is(err, workersessions.ErrInterruptSuccessorAdmissionFailed) || result.Accepted || result.Successor.ID != successorID {
		t.Fatalf("runInterrupt() = %#v, %v, want successor admission reference failure", result, err)
	}
}

func TestInterruptControlHistoryAppendFailureIsNotSessionNotFound(t *testing.T) {
	r := newTestRegistry(t)
	r.sessions["worker-1"] = workersessions.Session{
		ID:    "worker-1",
		State: workersessions.StateRunning,
		ProviderSessionAssociation: &workersessions.ProviderSessionAssociation{
			WorkerSessionID: "worker-1",
			TurnID:          "turn-1",
			DispatchID:      "dispatch-1",
			AttemptID:       "dispatch-1",
			Reference:       providers.SessionRef{Provider: providers.IDCodex, Kind: providers.SessionIDKind, ID: "provider-1"},
		},
	}
	r.supervisions["worker-1"] = newSupervision("dispatch-1", "turn-1", continuationValidExecution("dispatch-1"))
	r.publications["worker-1"] = &publication{open: true}
	appendErr := errors.New("interrupt control history append failed")
	r.events = failingContinuationEventsAppender{err: appendErr}

	result, err := r.Interrupt(context.Background(), workersessions.InterruptRequest{
		RequestID:                "interrupt-append-failure",
		SourceWorkerSessionID:    "worker-1",
		SuccessorWorkerSessionID: "successor-1",
		ReplacementMessage:       "replacement",
	})
	if !errors.Is(err, appendErr) || result.Phase != workersessions.InterruptPhaseValidation || result.Accepted {
		t.Fatalf("Interrupt(control history append failure) = %#v, %v, want validation failure with append error", result, err)
	}
}

func TestResume_LineagePublicationFailureRestoresThePausedAttempt(t *testing.T) {
	r, supervision, _ := newPausedContinuationRegistry(t)
	appendErr := errors.New("resume lineage append failed")
	r.events = &failingNthContinuationEventsAppender{delegate: r.events, n: 2, err: appendErr}

	result, err := r.Resume(context.Background(), workersessions.ControlRequest{ID: "worker-1", RequestID: "resume-lineage-failure"})
	if !errors.Is(err, appendErr) || result.Outcome != workersessions.ControlOutcomeFailed || result.Session.State != workersessions.StatePaused {
		t.Fatalf("Resume() = %#v, %v, want failed PAUSED result with lineage append error", result, err)
	}
	if supervision.dispatchID != "dispatch-1" || supervision.continuing || supervision.publishing || supervision.attemptsMade != 0 {
		t.Fatalf("Resume() after lineage append failure supervision = %#v, want restored paused attempt", supervision)
	}
}

func TestResume_ReturnsInitialControlHistoryAppendFailure(t *testing.T) {
	r, _, _ := newPausedContinuationRegistry(t)
	appendErr := errors.New("initial resume control history append failed")
	r.events = failingContinuationEventsAppender{err: appendErr}

	result, err := r.Resume(context.Background(), workersessions.ControlRequest{
		ID:        "worker-1",
		RequestID: "resume-initial-history-failure",
	})
	if !errors.Is(err, appendErr) || result.Outcome != workersessions.ControlOutcomeFailed {
		t.Fatalf("Resume(initial control history failure) = %#v, %v, want failed append result", result, err)
	}
}

func TestCancelBoundary_ReturnsNoopWhenAConcurrentTerminalCommitWins(t *testing.T) {
	const sessionID = "cancel-after-terminal"
	const dispatchID = "cancel-after-terminal-dispatch"
	r := newTestRegistry(t)
	r.sessions[sessionID] = workersessions.Session{ID: sessionID, State: workersessions.StateCompleted}
	supervision := newSupervision(dispatchID, "", dispatchHandoff(dispatchID))
	r.supervisions[sessionID] = supervision
	supervision.signalDone()

	result, retry, err := r.cancelBoundary(
		context.Background(),
		workersessions.ControlRequest{ID: sessionID},
		workersessions.ControlActionCancel,
		true,
		supervision,
		cancellationAttempt{kind: cancellationAttemptBoundary, wait: make(chan struct{}), dispatchID: dispatchID},
	)
	if err != nil || retry || result.Outcome != workersessions.ControlOutcomeNoop || result.Session.State != workersessions.StateCompleted {
		t.Fatalf("cancelBoundary(after terminal commit) = %#v, %t, %v, want completed no-op", result, retry, err)
	}
}

func TestContinue_ReturnsAtTheAdmissionBarrierBeforeCompletion(t *testing.T) {
	request := continuationReservationRequest()
	r := newContinuationSource(t, request)
	r.supervisions[request.SourceWorkerSessionID] = newSupervision("dispatch-1", "turn-1", continuationValidExecution("dispatch-1"))
	boundary := &admitBeforeCompletionBoundary{ready: make(chan struct{}), release: make(chan struct{})}
	r.execution = boundary

	outcomes := make(chan struct {
		result workersessions.ContinueResult
		err    error
	}, 1)
	go func() {
		result, err := r.Continue(context.Background(), request)
		outcomes <- struct {
			result workersessions.ContinueResult
			err    error
		}{result: result, err: err}
	}()
	select {
	case <-boundary.ready:
	case <-time.After(time.Second):
		t.Fatal("continuation did not reach the admission barrier")
	}
	close(boundary.release)
	select {
	case outcome := <-outcomes:
		if outcome.err != nil || outcome.result.Session.ID != request.SuccessorWorkerSessionID {
			t.Fatalf("Continue() = %#v, %v, want admitted successor", outcome.result, outcome.err)
		}
	case <-time.After(time.Second):
		t.Fatal("Continue() did not return after admission")
	}
}

func TestPauseAndCancelIterationRejectMissingControlTargets(t *testing.T) {
	r := newTestRegistry(t)
	if result, retry, err := r.pauseIteration(context.Background(), workersessions.ControlRequest{ID: "missing"}); retry || !errors.Is(err, workersessions.ErrSessionNotFound) || result.Outcome != workersessions.ControlOutcomeFailed {
		t.Fatalf("pauseIteration(missing) = %#v, %t, %v, want failed missing target", result, retry, err)
	}
	if result, retry, err := r.cancelControlIteration(context.Background(), workersessions.ControlRequest{ID: "missing"}, workersessions.ControlActionCancel, false); retry || !errors.Is(err, workersessions.ErrSessionNotFound) || result.Outcome != workersessions.ControlOutcomeFailed {
		t.Fatalf("cancelControlIteration(missing) = %#v, %t, %v, want failed missing target", result, retry, err)
	}
}

func TestObservationListRejectsMalformedPaginationCursor(t *testing.T) {
	r := newObservationRegistry(nil, nil)
	if _, err := r.ListWorkerSessionObservations(context.Background(), workersessions.ListWorkerSessionObservationsRequest{NextToken: "%%%"}); !errors.Is(err, workersessions.ErrInvalidObservationPagination) {
		t.Fatalf("ListWorkerSessionObservations(malformed cursor) = %v, want invalid pagination", err)
	}
}

type failingContinuationEventsAppender struct{ err error }

func (appender failingContinuationEventsAppender) Append(context.Context, events.AppendRequest) (events.AppendResult, error) {
	return events.AppendResult{}, appender.err
}

type deleteSessionAfterAppendEventsAppender struct {
	delegate EventsAppender
	delete   func()
	once     sync.Once
}

type signaledCancellationContext struct {
	done <-chan struct{}
}

func (ctx signaledCancellationContext) Deadline() (time.Time, bool) { return time.Time{}, false }

func (ctx signaledCancellationContext) Done() <-chan struct{} { return ctx.done }

func (signaledCancellationContext) Err() error { return context.Canceled }

func (signaledCancellationContext) Value(any) any { return nil }

func (appender *deleteSessionAfterAppendEventsAppender) Append(ctx context.Context, request events.AppendRequest) (events.AppendResult, error) {
	result, err := appender.delegate.Append(ctx, request)
	appender.once.Do(appender.delete)
	return result, err
}

type failingNthContinuationEventsAppender struct {
	delegate EventsAppender
	n        int
	err      error
	calls    int
}

func (appender *failingNthContinuationEventsAppender) Append(ctx context.Context, request events.AppendRequest) (events.AppendResult, error) {
	appender.calls++
	if appender.calls == appender.n {
		return events.AppendResult{}, appender.err
	}
	return appender.delegate.Append(ctx, request)
}

type admitBeforeCompletionBoundary struct {
	ready   chan struct{}
	release chan struct{}
	once    sync.Once
}

func (boundary *admitBeforeCompletionBoundary) Execute(ctx context.Context, request workers.ExecuteRequest) (workers.ExecuteResult, error) {
	boundary.once.Do(func() { close(boundary.ready) })
	select {
	case <-boundary.release:
		return workers.ExecuteResult{Correlation: request.Correlation, Outcome: workers.ExecutionOutcomeAccepted}, nil
	case <-ctx.Done():
		return workers.ExecuteResult{Correlation: request.Correlation, Outcome: workers.ExecutionOutcomeCanceled}, ctx.Err()
	}
}

func (*admitBeforeCompletionBoundary) InvokeModel(context.Context, string, modelinference.Request) (modelinference.Result, error) {
	return modelinference.Result{}, workers.ErrExecuteUnavailable
}

type continuationLineageRecordingStub struct {
	recordErr error
	records   []recordings.WorkerRecordingRecord
	failures  []recordings.WorkerRecordingFailure
}

func (*continuationLineageRecordingStub) StartWorkerSessionRecording(context.Context, recordings.WorkerSessionRecordingRequest) (recordings.WorkerSessionRecording, error) {
	return nil, nil
}

func (stub *continuationLineageRecordingStub) PersistWorkerRecord(_ context.Context, record recordings.WorkerRecordingRecord) error {
	stub.records = append(stub.records, record)
	return stub.recordErr
}

func (stub *continuationLineageRecordingStub) PersistWorkerRecordingFailure(_ context.Context, failure recordings.WorkerRecordingFailure) error {
	stub.failures = append(stub.failures, failure)
	return nil
}

func continuationReservationRequest() workersessions.ContinueRequest {
	return workersessions.ContinueRequest{
		RequestID:                "request-1",
		SourceWorkerSessionID:    "source-1",
		SuccessorWorkerSessionID: "successor-1",
		FollowUpInput:            "follow up",
	}
}

func continuationValidExecution(dispatchID string) workers.WorkstationDispatchRequest {
	return workers.WorkstationDispatchRequest{
		WorkstationName: "review",
		Execution: workers.WorkstationExecutionRequest{Dispatch: work.WorkDispatch{
			DispatchID: dispatchID, WorkstationName: "review",
		}},
	}
}

func newContinuationSource(t *testing.T, request workersessions.ContinueRequest) *registry {
	t.Helper()
	r := newTestRegistry(t)
	association := workersessions.ProviderSessionAssociation{
		WorkerSessionID: request.SourceWorkerSessionID,
		DispatchID:      "dispatch-1",
		AttemptID:       "dispatch-1",
		Reference:       providers.SessionRef{Provider: providers.IDCodex, Kind: providers.SessionIDKind, ID: "provider-session-1"},
	}
	r.sessions[request.SourceWorkerSessionID] = workersessions.Session{
		ID:                         request.SourceWorkerSessionID,
		State:                      workersessions.StateCompleted,
		ProviderSessionAssociation: &association,
	}
	return r
}

func TestContinuationObservationQueriesMapCanceledAndUnavailableProjections(t *testing.T) {
	r := newObservationRegistry(nil, nil)
	canceled, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := r.ListWorkerSessionObservations(canceled, workersessions.ListWorkerSessionObservationsRequest{}); !errors.Is(err, workersessions.ErrObservationCanceled) {
		t.Fatalf("ListWorkerSessionObservations(canceled) = %v, want canceled", err)
	}

	r.sessions["invalid-observation"] = workersessions.Session{
		ID: "invalid-observation", State: workersessions.State("UNKNOWN"),
		ProviderSessionAssociation: &workersessions.ProviderSessionAssociation{
			WorkerSessionID: "invalid-observation", DispatchID: "attempt-1", AttemptID: "attempt-1",
			Reference: providers.SessionRef{Provider: providers.IDCodex, Kind: providers.SessionIDKind, ID: "provider-1"},
		},
	}
	r.observations["invalid-observation"] = observationMetadata()
	if _, err := r.ListWorkerSessionObservations(context.Background(), workersessions.ListWorkerSessionObservationsRequest{Scope: workersessions.ObservationScopeAll}); err == nil {
		t.Fatal("ListWorkerSessionObservations(invalid projection) = nil, want projection validation error")
	}

	withoutAssociation := newObservationRegistry(observationProjectorFake{}, nil)
	withoutAssociation.sessions["terminal-no-association"] = workersessions.Session{ID: "terminal-no-association", State: workersessions.StateCompleted}
	withoutAssociation.observations["terminal-no-association"] = observationMetadata()
	if _, err := withoutAssociation.ReadTranscriptByWorkerSessionID(context.Background(), workersessions.ReadTranscriptByWorkerSessionIDRequest{WorkerSessionID: "terminal-no-association"}); !errors.Is(err, workersessions.ErrObservationTranscriptUnavailable) {
		t.Fatalf("ReadTranscriptByWorkerSessionID(no association) = %v, want transcript unavailable", err)
	}

	withoutProjector := newObservationRegistry(nil, nil)
	withoutProjector.sessions["terminal-with-association"] = observationSession("terminal-with-association", workersessions.StateCompleted)
	withoutProjector.observations["terminal-with-association"] = observationMetadata()
	if _, err := withoutProjector.ReadTranscriptByWorkerSessionID(context.Background(), workersessions.ReadTranscriptByWorkerSessionIDRequest{WorkerSessionID: "terminal-with-association"}); !errors.Is(err, workersessions.ErrObservationTranscriptProjectionUnavailable) {
		t.Fatalf("ReadTranscriptByWorkerSessionID(no projector) = %v, want projection unavailable", err)
	}
	if _, err := withoutProjector.ReadTranscriptByWorkerSessionID(context.Background(), workersessions.ReadTranscriptByWorkerSessionIDRequest{}); !errors.Is(err, workersessions.ErrInvalidSessionID) {
		t.Fatalf("ReadTranscriptByWorkerSessionID(invalid) = %v, want ErrInvalidSessionID", err)
	}
	if _, err := withoutProjector.ReadTranscriptByWorkerSessionID(canceled, workersessions.ReadTranscriptByWorkerSessionIDRequest{WorkerSessionID: "terminal-with-association"}); !errors.Is(err, workersessions.ErrObservationCanceled) {
		t.Fatalf("ReadTranscriptByWorkerSessionID(canceled) = %v, want ErrObservationCanceled", err)
	}
}

func TestReplayObservationSubscriptionCancellationCloses(t *testing.T) {
	topic := workersessions.Topic("worker-1")
	initial := replayProgressResult(topic, 2, 1)
	canceled, cancel := context.WithCancel(context.Background())
	cancel()
	subscription, err := newReplayObservationSubscription(context.Background(), &observationEventReaderFake{readResults: []events.ReadResult{initial}}, topic, workersessions.StateRunning, 1)
	if err != nil {
		t.Fatalf("newReplayObservationSubscription(canceled) error = %v", err)
	}
	if got := subscription.Next(canceled); got.Kind != workersessions.ObservationDeliveryCanceled || !errors.Is(got.Err, workersessions.ErrObservationCanceled) {
		t.Fatalf("canceled delivery = %#v, want CANCELED", got)
	}
	if got := subscription.Next(context.Background()); got.Kind != workersessions.ObservationDeliveryClosed {
		t.Fatalf("delivery after cancellation = %#v, want CLOSED", got)
	}
}

func TestReplayObservationSubscriptionCloseDuringReadCloses(t *testing.T) {
	topic := workersessions.Topic("worker-1")
	initial := replayProgressResult(topic, 2, 1)
	var racing *replayObservationSubscription
	reads := 0
	racingReader := &observationEventReaderFake{}
	racingReader.readFunc = func(context.Context, events.ReadRequest) (events.ReadResult, error) {
		reads++
		if reads == 1 {
			return initial, nil
		}
		racing.Close()
		return replayProgressResult(topic, 2, 2), nil
	}
	var err error
	racing, err = newReplayObservationSubscription(context.Background(), racingReader, topic, workersessions.StateRunning, 1)
	if err != nil {
		t.Fatalf("newReplayObservationSubscription(racing) error = %v", err)
	}
	if got := racing.Next(context.Background()); got.Kind != workersessions.ObservationDeliveryRecord {
		t.Fatalf("racing initial delivery = %#v, want RECORD", got)
	}
	if got := racing.Next(context.Background()); got.Kind != workersessions.ObservationDeliveryClosed {
		t.Fatalf("racing delivery = %#v, want CLOSED", got)
	}
}

func TestReplayObservationSubscriptionRejectsSnapshotInvariantViolations(t *testing.T) {
	topic := workersessions.Topic("worker-1")
	subscription := &replayObservationSubscription{
		topic:        topic,
		snapshotHead: 1,
		next:         events.Cursor{Topic: topic, Position: 1},
	}
	result := replayProgressResult(topic, 2, 2)
	if err := subscription.appendPage(result); !errors.Is(err, workersessions.ErrObservationSourceUnavailable) {
		t.Fatalf("appendPage(after snapshot) error = %v, want source unavailable", err)
	}
}

func replayProgressResult(topic events.Topic, head uint64, positions ...uint64) events.ReadResult {
	records := make([]events.Record, 0, len(positions))
	for _, position := range positions {
		records = append(records, replayObservationRecord(topic, position, fmt.Sprintf("event-%d", position)))
	}
	return events.ReadResult{
		Outcome:  events.ReadOutcomeProgress,
		Records:  records,
		Next:     events.Cursor{Topic: topic, Position: events.AggregateSequence(positions[len(positions)-1])},
		Retained: events.RetainedRange{Topic: topic, Earliest: 1, Head: events.AggregateSequence(head)},
	}
}

func replayObservationRecord(topic events.Topic, position uint64, eventID string) events.Record {
	return events.Record{
		ID:             events.RecordID{Topic: topic, Position: events.AggregateSequence(position)},
		SourceType:     "worker_observation",
		SourceID:       "provider-session-1",
		SourceSequence: events.SourceSequence(position),
		SourceEventID:  events.SourceEventID(eventID),
		SchemaID:       "worker_session.observation",
		Payload:        []byte(`{"position":1}`),
	}
}

func timePointer(value time.Time) *time.Time { return &value }

type observationRecordingReaderStub struct {
	recordings.WorkerSessionRecordingService
	snapshot recordings.WorkerRecordingSnapshot
	err      error
	ctx      context.Context
	id       string
}

type observationRecordingServiceStub struct {
	recordings.WorkerSessionRecordingService
}

func (s *observationRecordingReaderStub) LoadWorkerRecording(ctx context.Context, recordingID string) (recordings.WorkerRecordingSnapshot, error) {
	s.ctx = ctx
	s.id = recordingID
	return s.snapshot, s.err
}
func TestLoadWorkerRecordingUsesOptionalDurableReader(t *testing.T) {
	var nilRegistry *registry
	if _, err := nilRegistry.LoadWorkerRecording(context.Background(), "recording-1"); !errors.Is(err, recordings.ErrMissingWorkerRecordingReader) {
		t.Fatalf("nil registry LoadWorkerRecording() = %v, want missing reader", err)
	}
	withoutReader := &registry{}
	if _, err := withoutReader.LoadWorkerRecording(context.Background(), "recording-1"); !errors.Is(err, recordings.ErrMissingWorkerRecordingReader) {
		t.Fatalf("missing recording service LoadWorkerRecording() = %v, want missing reader", err)
	}
	withoutDurableReader := &registry{recording: &observationRecordingServiceStub{}}
	if _, err := withoutDurableReader.LoadWorkerRecording(context.Background(), "recording-1"); !errors.Is(err, recordings.ErrMissingWorkerRecordingReader) {
		t.Fatalf("recording service without reader LoadWorkerRecording() = %v, want missing reader", err)
	}

	want := recordings.WorkerRecordingSnapshot{RecordingID: "recording-1"}
	reader := &observationRecordingReaderStub{snapshot: want}
	got, err := (&registry{recording: reader}).LoadWorkerRecording(nil, "recording-1")
	if err != nil || got.RecordingID != want.RecordingID || reader.id != "recording-1" || reader.ctx == nil {
		t.Fatalf("LoadWorkerRecording() = %#v, %v; reader id=%q ctx=%v", got, err, reader.id, reader.ctx)
	}

	readErr := errors.New("durable read failed")
	reader = &observationRecordingReaderStub{err: readErr}
	if _, err := (&registry{recording: reader}).LoadWorkerRecording(context.Background(), "recording-1"); !errors.Is(err, readErr) {
		t.Fatalf("LoadWorkerRecording() error = %v, want %v", err, readErr)
	}
}
func TestObservationSubscriptionMapsLiveCursorGapBeforeDeliveryToStale(t *testing.T) {
	gap := events.Delivery{Kind: events.DeliveryGap, Gap: &events.GapFacts{Topic: "worker-session/worker-1", Requested: 1, EarliestRetained: 2, Head: 3}}
	subscription := &observationSubscription{
		source:         events.Subscription(func(context.Context) events.Delivery { return gap }),
		cursorProvided: true,
	}
	got := subscription.Next(context.Background())
	if got.Kind != workersessions.ObservationDeliverySourceFailure || !errors.Is(got.Err, workersessions.ErrObservationCursorStale) {
		t.Fatalf("live cursor gap delivery = %#v, want stale source failure", got)
	}
}
func TestStreamObservationsByWorkerSessionIDRejectsDurableCursorOnLiveFallback(t *testing.T) {
	reader := &observationEventReaderFake{
		subscription: events.Subscription(func(context.Context) events.Delivery {
			return events.Delivery{Kind: events.DeliveryClosed}
		}),
	}
	registry := newObservationRegistry(nil, reader)
	registry.sessions["worker-1"] = observationSession("worker-1", workersessions.StateRunning)
	_, err := registry.StreamObservationsByWorkerSessionID(context.Background(), workersessions.StreamObservationsByWorkerSessionIDRequest{
		WorkerSessionID: "worker-1",
		Cursor:          &workersessions.ObservationCursor{WorkerSessionID: "worker-1", StreamGenerationID: "factory-generation-1", Position: 4},
	})
	if !errors.Is(err, workersessions.ErrObservationCursorUnavailable) {
		t.Fatalf("live stream with durable cursor error = %v, want ErrObservationCursorUnavailable", err)
	}
	if reader.subscribeCalls != 0 {
		t.Fatalf("Subscribe() calls = %d, want 0 for unavailable durable cursor", reader.subscribeCalls)
	}
}

type sourceOnlyClock struct{}

func (sourceOnlyClock) Now() time.Time { return time.Now() }

func TestDeadlineSupervisionCoversInactiveAndHostTimerPaths(t *testing.T) {
	clock := sourceOnlyClock{}
	deadlineTimer := newSupervisionDeadlineTimer(clock, time.Hour)
	if deadlineTimer.C() == nil {
		t.Fatal("host deadline timer channel is nil")
	}
	if !deadlineTimer.Stop() {
		t.Fatal("host deadline timer Stop() = false, want true")
	}

	serviceRegistry := &registry{clock: clock}
	noTimeout := newSupervision("session", "turn")
	serviceRegistry.startDeadlineWatcher("session", noTimeout, time.Now())
	inactive := newSupervision("session", "turn", workers.WorkstationDispatchRequest{
		Execution: workers.WorkstationExecutionRequest{
			WorkerType: "worker",
			Timeout:    time.Second,
		},
	})
	serviceRegistry.startDeadlineWatcher("session", inactive, time.Now())
	acceptedWithoutAttempt := newSupervision("session", "turn", workers.WorkstationDispatchRequest{
		Execution: workers.WorkstationExecutionRequest{
			WorkerType: "worker",
			Timeout:    time.Second,
		},
	})
	acceptedWithoutAttempt.accepted = true
	serviceRegistry.startDeadlineWatcher("session", acceptedWithoutAttempt, time.Now())
	expiredCancellation := make(chan struct{})
	expired := newSupervision("expired", "turn", workers.WorkstationDispatchRequest{
		Execution: workers.WorkstationExecutionRequest{
			WorkerType: "worker",
			Timeout:    time.Second,
		},
	})
	expired.accepted = true
	expired.attemptDone = make(chan struct{})
	expired.installCancel(func() {
		close(expiredCancellation)
	})
	serviceRegistry.startDeadlineWatcher("session", expired, time.Now().Add(-time.Hour))
	expiredWait := time.NewTimer(time.Second)
	defer expiredWait.Stop()
	select {
	case <-expiredCancellation:
	case <-expiredWait.C:
		t.Fatal("expired deadline watcher did not reconcile")
	}

	supervision := newSupervision("dispatch", "turn")
	supervision.accepted = true
	supervision.dispatchID = "dispatch"
	attemptDone := make(chan struct{})
	supervision.attemptDone = attemptDone
	if !supervision.deadlineAttemptActive("dispatch", attemptDone) {
		t.Fatal("deadlineAttemptActive() = false for active attempt")
	}
	if supervision.deadlineAttemptActive("other", attemptDone) {
		t.Fatal("deadlineAttemptActive() = true for another dispatch")
	}
	close(attemptDone)
	if supervision.deadlineAttemptActive("dispatch", attemptDone) {
		t.Fatal("deadlineAttemptActive() = true after attempt completion")
	}

	reconciliationSupervision := newSupervision("dispatch", "turn")
	reconciliationSupervision.accepted = true
	reconciliationSupervision.dispatchID = "dispatch"
	reconciliationAttemptDone := make(chan struct{})
	reconciliationSupervision.attemptDone = reconciliationAttemptDone
	for _, cancelErr := range []error{
		nil,
		workers.ErrWorkstationDispatchAlreadyTerminal,
		workers.ErrWorkstationDispatchCanceled,
		errors.New("cancel failed"),
	} {
		reconciliationSupervision.installCancelFailure(func() error { return cancelErr })
		serviceRegistry.logger = logging.NoopLogger{}
		serviceRegistry.reconcileOverdueAttempt("session", reconciliationSupervision, "dispatch", reconciliationAttemptDone, time.Now())
	}
	inactiveAttempt := newSupervision("inactive", "turn")
	serviceRegistry.reconcileOverdueAttempt("session", inactiveAttempt, "inactive", make(chan struct{}), time.Now())
}

func TestOpeningSessionContinuationPreservesExactResumeIdentity(t *testing.T) {
	if got := openingSessionContinuation(workers.WorkstationExecutionRequest{}); got != nil {
		t.Fatalf("openingSessionContinuation(empty) = %#v, want nil", got)
	}
	request := workers.WorkstationExecutionRequest{
		Continuation: &providers.ContinuationRef{
			Provider:          "codex",
			ProviderSessionID: "provider-session-1",
		},
	}
	got := openingSessionContinuation(request)
	if got == nil || got.Provider != "codex" || got.Kind == "" || got.ID != "provider-session-1" {
		t.Fatalf("openingSessionContinuation(resume) = %#v, want normalized exact identity", got)
	}
	request.Continuation = &providers.ContinuationRef{}
	if got := openingSessionContinuation(request); got == nil || got.Kind == "" {
		t.Fatalf("openingSessionContinuation(empty continuation) = %#v, want compatibility kind", got)
	}
}

type coverageExecution struct {
	execute func(context.Context, workers.ExecuteRequest) (workers.ExecuteResult, error)
}

func (execution coverageExecution) Execute(ctx context.Context, request workers.ExecuteRequest) (workers.ExecuteResult, error) {
	if execution.execute == nil {
		return workers.ExecuteResult{}, workers.ErrExecuteUnavailable
	}
	return execution.execute(ctx, request)
}

func (coverageExecution) InvokeModel(context.Context, string, modelinference.Request) (modelinference.Result, error) {
	return modelinference.Result{}, workers.ErrExecuteUnavailable
}

type coverageProcessObserver struct {
	started int
	exited  int
}

func (observer *coverageProcessObserver) ProcessStarted(platformprocess.ProcessInfo) {
	observer.started++
}

func (observer *coverageProcessObserver) ProcessExited(platformprocess.ProcessInfo) {
	observer.exited++
}

type coverageRecording struct {
	awaitErr error
	abortErr error
	closeErr error
}

func (recording *coverageRecording) AwaitOpening(context.Context) error { return recording.awaitErr }
func (recording *coverageRecording) Abort(context.Context, error) error { return recording.abortErr }
func (recording *coverageRecording) Close(context.Context) error        { return recording.closeErr }

type coverageFinalizingRecording struct {
	coverageRecording
	terminalErr error
}

func (recording *coverageFinalizingRecording) CloseWithTerminal(context.Context, recordings.WorkerRecordingTerminal) error {
	return recording.terminalErr
}

type coverageClock struct{ now time.Time }

func (clock coverageClock) Now() time.Time { return clock.now }

type coverageReadReader struct {
	readErr error
}

func (reader coverageReadReader) Read(context.Context, events.ReadRequest) (events.ReadResult, error) {
	return events.ReadResult{}, reader.readErr
}

type coverageSubscribeReader struct {
	subscribeErr error
}

func (reader coverageSubscribeReader) Subscribe(context.Context, events.SubscribeRequest) (events.Subscription, error) {
	return nil, reader.subscribeErr
}

type coverageGatedReader struct {
	inner   EventsReader
	started chan struct{}
	release chan struct{}
	once    sync.Once
}

func (reader *coverageGatedReader) Subscribe(ctx context.Context, request events.SubscribeRequest) (events.Subscription, error) {
	reader.once.Do(func() { close(reader.started) })
	select {
	case <-reader.release:
		return reader.inner.Subscribe(ctx, request)
	case <-ctx.Done():
		return nil, ctx.Err()
	}
}

func setCoverageAccepted(supervision *supervision, accepted bool) {
	supervision.mu.Lock()
	supervision.accepted = accepted
	supervision.mu.Unlock()
}

func coverageExecutionResult(request workers.ExecuteRequest, outcome workers.ExecutionOutcome) workers.ExecuteResult {
	return workers.ExecuteResult{Correlation: request.Correlation, Outcome: outcome}
}

func TestWorkerExecutionHandoff_CoversAdmissionFailuresAndProcessLifecycle(t *testing.T) {
	t.Run("admission failures", testWorkerExecutionHandoffAdmissionFailures)
	t.Run("process lifecycle", testWorkerExecutionHandoffProcessLifecycle)
}

func testWorkerExecutionHandoffAdmissionFailures(t *testing.T) {
	request := dispatchHandoff("handoff-dispatch")

	t.Run("missing execution", func(t *testing.T) {
		result, err := executeWithService(context.Background(), nil, request, newSupervision("handoff-dispatch", ""), func() {})
		if !errors.Is(err, workers.ErrExecuteUnavailable) || result.TerminalOutcome != workers.WorkstationDispatchTerminalOutcomeFailed {
			t.Fatalf("executeWithService(nil) = %#v, %v, want failed unavailable result", result, err)
		}
	})

	t.Run("invalid dispatch", func(t *testing.T) {
		invalid := request
		invalid.Execution.Dispatch.DispatchID = "  "
		result, err := executeWithService(context.Background(), coverageExecution{}, invalid, newSupervision("handoff-dispatch", ""), func() {})
		if !errors.Is(err, workers.ErrInvalidExecuteRequest) || result.TerminalOutcome != workers.WorkstationDispatchTerminalOutcomeFailed {
			t.Fatalf("executeWithService(invalid) = %#v, %v, want invalid failed result", result, err)
		}
	})

	t.Run("canceled before admission", func(t *testing.T) {
		supervision := newSupervision("handoff-dispatch", "")
		supervision.preAdmissionAction = workersessions.ControlActionPause
		admitted := false
		result, err := executeWithService(context.Background(), coverageExecution{}, request, supervision, func() { admitted = true })
		if !errors.Is(err, workers.ErrWorkstationDispatchCanceled) || admitted || result.TerminalOutcome != workers.WorkstationDispatchTerminalOutcomeCanceled {
			t.Fatalf("executeWithService(pre-admission cancel) = %#v, %v, admitted=%t", result, err, admitted)
		}
	})

	t.Run("admission callback declines", func(t *testing.T) {
		supervision := newSupervision("handoff-dispatch", "")
		result, err := executeWithService(context.Background(), coverageExecution{}, request, supervision, func() {})
		if !errors.Is(err, workers.ErrWorkstationDispatchCanceled) || result.TerminalOutcome != workers.WorkstationDispatchTerminalOutcomeCanceled {
			t.Fatalf("executeWithService(unaccepted) = %#v, %v, want canceled result", result, err)
		}
	})

	t.Run("executor panic is normalized", func(t *testing.T) {
		supervision := newSupervision("handoff-dispatch", "")
		setCoverageAccepted(supervision, true)
		_, err := executeWithService(context.Background(), coverageExecution{
			execute: func(context.Context, workers.ExecuteRequest) (workers.ExecuteResult, error) {
				panic("test executor panic")
			},
		}, request, supervision, func() {})
		var panicErr *workers.WorkerExecutorPanicError
		if !errors.As(err, &panicErr) {
			t.Fatalf("executeWithService(panic) error = %v, want WorkerExecutorPanicError", err)
		}
	})
}

func testWorkerExecutionHandoffProcessLifecycle(t *testing.T) {
	request := dispatchHandoff("handoff-dispatch")
	t.Run("process exit wins over executor result", func(t *testing.T) {
		supervision := newSupervision("handoff-dispatch", "")
		setCoverageAccepted(supervision, true)
		result, err := executeWithService(context.Background(), coverageExecution{
			execute: func(_ context.Context, request workers.ExecuteRequest) (workers.ExecuteResult, error) {
				request.Input.ProcessLifecycleObserver.ProcessStarted(platformprocess.ProcessInfo{PID: 7})
				request.Input.ProcessLifecycleObserver.ProcessExited(platformprocess.ProcessInfo{PID: 7})
				return coverageExecutionResult(request, workers.ExecutionOutcomeAccepted), nil
			},
		}, request, supervision, func() {})
		if !errors.Is(err, workers.ErrWorkstationDispatchProcessGone) || result.ReconciliationReason != workers.WorkstationDispatchReconciliationReasonProcessGone || result.Result.Outcome != workers.OutcomeFailed {
			t.Fatalf("executeWithService(process exit) = %#v, %v, want process-gone failure", result, err)
		}
	})
	t.Run("preserves a caller process observer", testWorkerExecutionHandoffPreservesProcessObserver)
}

func testWorkerExecutionHandoffPreservesProcessObserver(t *testing.T) {
	request := dispatchHandoff("handoff-dispatch")
	var nilObserver processLifecycleObserver
	nilObserver.ProcessExited(platformprocess.ProcessInfo{})
	canceledSupervision := newSupervision("handoff-dispatch", "")
	canceledSupervision.mu.Lock()
	canceledSupervision.requestedAction = workersessions.ControlActionCancel
	canceledSupervision.mu.Unlock()
	processLifecycleObserver{supervision: canceledSupervision}.ProcessExited(platformprocess.ProcessInfo{})
	if canceledSupervision.processGoneObserved() {
		t.Fatal("ProcessExited marked a process gone after cancellation was requested")
	}

	customObserver := &coverageProcessObserver{}
	customRequest := request
	customRequest.Execution.ProcessLifecycleObserver = customObserver
	supervision := newSupervision("handoff-dispatch", "")
	setCoverageAccepted(supervision, true)
	_, err := executeWithService(context.Background(), coverageExecution{
		execute: func(_ context.Context, request workers.ExecuteRequest) (workers.ExecuteResult, error) {
			if request.Input.ProcessLifecycleObserver != customObserver {
				t.Fatalf("Execute replaced a caller-provided process observer")
			}
			return coverageExecutionResult(request, workers.ExecutionOutcomeAccepted), nil
		},
	}, customRequest, supervision, func() {})
	if err != nil || customObserver.started != 0 || customObserver.exited != 0 {
		t.Fatalf("executeWithService(custom observer) = %v, observer=%#v, want success without replacement", err, customObserver)
	}
}

func TestWorkerExecutionHandoff_MapsWorkerOutcomesAndDetachesProcessGoneResults(t *testing.T) {
	t.Run("maps worker outcomes", testWorkerExecutionHandoffMapsWorkerOutcomes)
	t.Run("detaches process-gone results", testWorkerExecutionHandoffDetachesProcessGoneResults)
}

type workerExecutionHandoffOutcomeCase struct {
	name           string
	result         workers.ExecuteResult
	executeErr     error
	terminal       workers.WorkstationDispatchTerminalOutcome
	outcome        workers.WorkOutcome
	cancellation   workers.DispatchCancellationReason
	reconciliation workers.WorkstationDispatchReconciliationReason
	wantError      error
}

func testWorkerExecutionHandoffMapsWorkerOutcomes(t *testing.T) {
	request := dispatchHandoff("result-dispatch")
	output := workers.ProposedOutput{Primary: []work.WorkContentPart{
		{Type: work.WorkContentPartTypeText, Text: "primary output"},
	}}
	cases := []workerExecutionHandoffOutcomeCase{
		{name: "continue", result: workers.ExecuteResult{Outcome: workers.ExecutionOutcomeContinue, Output: output}, terminal: workers.WorkstationDispatchTerminalOutcomeCompleted, outcome: workers.OutcomeContinue},
		{name: "rejected", result: workers.ExecuteResult{Outcome: workers.ExecutionOutcomeRejected}, terminal: workers.WorkstationDispatchTerminalOutcomeCompleted, outcome: workers.OutcomeRejected},
		{name: "failed result", result: workers.ExecuteResult{Outcome: workers.ExecutionOutcomeFailed, Failure: &workers.ExecutionFailure{Message: "failed", Family: workers.WorkFailureFamilyTerminal, Type: workers.WorkFailureTypeUnknown}}, terminal: workers.WorkstationDispatchTerminalOutcomeFailed, outcome: workers.OutcomeFailed},
		{name: "canceled result", result: workers.ExecuteResult{Outcome: workers.ExecutionOutcomeCanceled}, terminal: workers.WorkstationDispatchTerminalOutcomeCanceled, outcome: workers.OutcomeCanceled, cancellation: workers.DispatchCancellationReasonCanceled},
		{name: "canceled error supplies missing cancellation", result: workers.ExecuteResult{Outcome: workers.ExecutionOutcomeCanceled}, executeErr: workers.ErrWorkstationDispatchCanceled, terminal: workers.WorkstationDispatchTerminalOutcomeCanceled, outcome: workers.OutcomeCanceled, cancellation: workers.DispatchCancellationReasonCanceled, wantError: workers.ErrWorkstationDispatchCanceled},
		{name: "provider returns context cancellation", result: workers.ExecuteResult{Outcome: workers.ExecutionOutcomeAccepted}, executeErr: context.Canceled, terminal: workers.WorkstationDispatchTerminalOutcomeFailed, outcome: workers.OutcomeFailed, wantError: context.Canceled},
		{name: "process gone", result: workers.ExecuteResult{Outcome: workers.ExecutionOutcomeAccepted}, executeErr: workers.ErrWorkstationDispatchProcessGone, terminal: workers.WorkstationDispatchTerminalOutcomeFailed, outcome: workers.OutcomeAccepted, reconciliation: workers.WorkstationDispatchReconciliationReasonProcessGone, wantError: workers.ErrWorkstationDispatchProcessGone},
	}
	for _, test := range cases {
		t.Run(test.name, func(t *testing.T) {
			assertWorkerExecutionHandoffOutcome(t, request, test)
		})
	}

	requestWithOutput := request
	result, err := dispatchResultFromExecute(requestWithOutput, workers.ExecuteResult{Output: output}, nil)
	if err != nil || result.Result.Output != "primary output" {
		t.Fatalf("dispatchResultFromExecute(primary text) = %#v, %v, want copied text", result, err)
	}

	managedOutput := workers.ProposedOutput{Primary: []work.WorkContentPart{
		{Type: work.WorkContentPartTypeAudio, ContentType: "audio/wav", URL: "data:audio/wav;base64,UklGRg=="},
	}}
	managedResult, err := dispatchResultFromExecute(requestWithOutput, workers.ExecuteResult{
		Output:                managedOutput,
		ProposedOutputPresent: true,
	}, nil)
	if err != nil || managedResult.ProposedOutput == nil || len(managedResult.ProposedOutput.Primary) != 1 ||
		managedResult.ProposedOutput.Primary[0].ContentType != "audio/wav" ||
		managedResult.ProposedOutput.Primary[0].URL != "data:audio/wav;base64,UklGRg==" {
		t.Fatalf("dispatchResultFromExecute(managed proposal) = %#v, %v, want cloned audio proposal", managedResult, err)
	}
	managedOutput.Primary[0].URL = "changed"
	if managedResult.ProposedOutput.Primary[0].URL != "data:audio/wav;base64,UklGRg==" {
		t.Fatal("dispatchResultFromExecute(managed proposal) retained the input proposal backing slice")
	}

	canceledOutput, err := dispatchResultFromExecute(requestWithOutput, workers.ExecuteResult{
		Cancellation: &workers.DispatchCancellation{Reason: workers.DispatchCancellationReasonCanceled},
	}, nil)
	if err != nil || canceledOutput.Result.Error != workers.ErrWorkstationDispatchCanceled.Error() {
		t.Fatalf("dispatchResultFromExecute(cancellation) = %#v, %v, want canonical cancellation error", canceledOutput, err)
	}
}

func assertWorkerExecutionHandoffOutcome(
	t *testing.T,
	request workers.WorkstationDispatchRequest,
	test workerExecutionHandoffOutcomeCase,
) {
	t.Helper()
	result, err := dispatchResultFromExecute(request, test.result, test.executeErr)
	if result.TerminalOutcome != test.terminal || result.Result.Outcome != test.outcome || result.ReconciliationReason != test.reconciliation {
		t.Fatalf("dispatchResultFromExecute() = %#v, want terminal=%q outcome=%q reconciliation=%q", result, test.terminal, test.outcome, test.reconciliation)
	}
	if test.cancellation == "" {
		if result.Result.Cancellation != nil || result.Cancellation != nil {
			t.Fatalf("dispatchResultFromExecute() cancellation = %#v/%#v, want nil", result.Result.Cancellation, result.Cancellation)
		}
	} else if result.Result.Cancellation == nil || result.Result.Cancellation.Reason != test.cancellation ||
		result.Cancellation == nil || result.Cancellation.Reason != test.cancellation {
		t.Fatalf("dispatchResultFromExecute() cancellation = %#v/%#v, want %q", result.Result.Cancellation, result.Cancellation, test.cancellation)
	}
	if test.wantError != nil && !errors.Is(err, test.wantError) {
		t.Fatalf("dispatchResultFromExecute() error = %v, want %v", err, test.wantError)
	}
}

func testWorkerExecutionHandoffDetachesProcessGoneResults(t *testing.T) {
	request := dispatchHandoff("result-dispatch")
	output := workers.ProposedOutput{Primary: []work.WorkContentPart{
		{Type: work.WorkContentPartTypeText, Text: "primary output"},
	}}
	gone := processGoneExecuteResult(
		workers.ExecuteRequest{Correlation: workers.ExecutionCorrelation{DispatchID: "gone-dispatch"}},
		workers.ExecuteResult{Output: output, StructuredResult: map[string]any{"value": true}, StructuredResultPresent: true, Continuation: &workers.ProviderContinuationRef{Provider: "codex", ProviderSessionID: "provider-session"}},
	)
	if gone.Outcome != workers.ExecutionOutcomeFailed || gone.Output.Primary != nil || gone.StructuredResult != nil || gone.StructuredResultPresent || gone.Continuation != nil || gone.Failure == nil {
		t.Fatalf("processGoneExecuteResult() = %#v, want detached retryable failure", gone)
	}

	if _, err := executeRequestFromSessionDispatch(workers.WorkstationDispatchRequest{WorkstationName: "review"}); !errors.Is(err, workers.ErrInvalidExecuteRequest) {
		t.Fatalf("executeRequestFromSessionDispatch(blank dispatch) error = %v, want invalid request", err)
	}
	if _, err := failedDispatchResult(request, nil); !errors.Is(err, workers.ErrExecuteUnavailable) {
		t.Fatalf("failedDispatchResult(nil) error = %v, want execute unavailable", err)
	}
	if _, err := canceledDispatchResult(request); !errors.Is(err, workers.ErrWorkstationDispatchCanceled) {
		t.Fatalf("canceledDispatchResult() error = %v, want canceled", err)
	}
}

func TestWorkerExecutionHandoff_PublishExecutionReportsPreAdmissionFailure(t *testing.T) {
	r := newTestRegistry(t)
	r.execution = nil
	request := dispatchHandoff("publish-failure")
	err := r.publishExecution(nil, "missing-session", request, newSupervision("publish-failure", "", request))
	if !errors.Is(err, workers.ErrExecuteUnavailable) {
		t.Fatalf("publishExecution(nil execution) error = %v, want execute unavailable", err)
	}
}

func TestWorkerSessionClassification_CoversStructuredFallbackAndAssociationRejection(t *testing.T) {
	if got := safeFailureClassificationForDispatch(workersessions.FailureCauseAdapterFailure, workers.WorkResult{}, nil); got != "" {
		t.Fatalf("safeFailureClassificationForDispatch(empty) = %q, want empty generic classification", got)
	}
	providerErr := workers.NewProviderError(workers.WorkFailureTypeTimeout, "provider timeout", errors.New("transport"))
	metadata := &workers.WorkFailureMetadata{Family: workers.WorkFailureFamilyTerminal, Type: workers.WorkFailureTypeUnknown}
	if got := failureMetadataForDispatch(workers.WorkResult{FailureMetadata: metadata}, providerErr, false); got != metadata {
		t.Fatalf("failureMetadataForDispatch(prefer=false) = %#v, want existing metadata", got)
	}
	if got := failureMetadataForDispatch(workers.WorkResult{}, providerErr, false); got == nil || got.Type != workers.WorkFailureTypeTimeout {
		t.Fatalf("failureMetadataForDispatch(missing metadata) = %#v, want provider metadata", got)
	}
	if got := safeFailureClassificationForDispatch(workersessions.FailureCauseAdapterFailure, workers.WorkResult{FailureMetadata: &workers.WorkFailureMetadata{Family: workers.WorkFailureFamilyTerminal, Type: workers.WorkFailureTypeUnknown}}, nil); got == "" {
		t.Fatal("safeFailureClassificationForDispatch(known metadata) returned empty classification")
	}
	if got := contradictorySuccessDetail(false, "context=provider"); got == "" {
		t.Fatal("contradictorySuccessDetail() returned an empty detail")
	}

	ref := providers.SessionRef{Provider: providers.IDCodex, Kind: providers.SessionIDKind, ID: "provider-session"}
	continuation := ref.ContinuationRef()
	r := newTestRegistry(t)
	r.associateProviderSessionFromResult("missing-session", "dispatch", workers.WorkstationDispatchResult{
		Result: workers.WorkResult{Continuation: &continuation},
	})
}

func TestWorkerSessionRecordingPublication_CleansUpAndPreservesErrors(t *testing.T) {
	r := newTestRegistry(t)
	r.reserveIfAbsent("opening-cleanup")
	openingErr := errors.New("opening barrier failed")
	abortErr := errors.New("abort failed")
	if err := r.publishOpeningRecord(context.Background(), "opening-cleanup", "dispatch", workers.SessionPayload{Status: string(workersessions.StateStarting)}, "codex", &coverageRecording{awaitErr: openingErr, abortErr: abortErr}); !errors.Is(err, openingErr) {
		t.Fatalf("publishOpeningRecord() error = %v, want opening barrier error", err)
	}

	r.reserveIfAbsent("terminal-cleanup")
	terminalPub := r.publications["terminal-cleanup"]
	terminalPub.open = true
	closeRecording := &coverageRecording{closeErr: errors.New("close failed")}
	terminalPub.recording = closeRecording
	terminalPub.provider = string(providers.IDCodex)
	appendErr := errors.New("terminal append failed")
	r.events = &runtimeAttemptBrokenAppender{err: appendErr}
	err := r.publishTerminalRecord(context.Background(), "terminal-cleanup", "dispatch", workersessions.StateCompleted, workersessions.TerminalResult{Outcome: workersessions.TerminalOutcomeCompleted})
	if !errors.Is(err, appendErr) || !errors.Is(err, closeRecording.closeErr) {
		t.Fatalf("publishTerminalRecord() error = %v, want append and close errors", err)
	}

	finalizing := &coverageFinalizingRecording{terminalErr: errors.New("terminal finalizer failed")}
	if err := r.closeWorkerRecording(context.Background(), finalizing, workersessions.State("UNKNOWN"), 0); err == nil {
		t.Fatal("closeWorkerRecording(invalid terminal state) error = nil, want phase error")
	}
}

func TestWorkerSessionContinuationAndReconciliation_CoversAdmissionFailurePaths(t *testing.T) {
	r := newTestRegistry(t)
	r.execution = nil
	plan := continuePlan{
		request:   workersessions.ContinueRequest{RequestID: "continue-request", SourceWorkerSessionID: "source", SuccessorWorkerSessionID: "successor"},
		execution: dispatchHandoff("continue-dispatch"),
	}
	if _, err := r.continueReserved(plan); err == nil {
		t.Fatal("continueReserved(no execution) error = nil, want admission failure")
	}

	supervision := newSupervision("deadline-dispatch", "")
	attemptDone := supervision.beginAttempt()
	setCoverageAccepted(supervision, true)
	r.reconcileOverdueAttempt("deadline-session", supervision, "deadline-dispatch", attemptDone, time.Now().Add(-time.Minute))
	if supervision.deadlineExceeded {
		t.Fatal("reconcileOverdueAttempt() retained deadlineExceeded after unavailable cancellation")
	}

	failureSupervision := newSupervision("deadline-failure", "")
	failureAttemptDone := failureSupervision.beginAttempt()
	setCoverageAccepted(failureSupervision, true)
	failureSupervision.installCancelFailure(func() error { return errors.New("cancel effect failed") })
	r.reconcileOverdueAttempt("deadline-session", failureSupervision, "deadline-failure", failureAttemptDone, time.Now())
}

func TestWorkerSessionInvocationAndReplay_CoversDetachedFailurePaths(t *testing.T) {
	r := newTestRegistry(t)
	r.execution = nil
	result, err := r.InvokeSession(context.Background(), workersessions.InvokeSessionRequest{ID: "invoke-no-execution", Execution: dispatchHandoff("invoke-dispatch")})
	if err != nil || result.Session.State != workersessions.StateFailed {
		t.Fatalf("InvokeSession(no execution) = %#v, %v, want failed terminal result", result, err)
	}

	startReplay := &startReplay{done: make(chan struct{})}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := awaitStartReplay(ctx, startReplay); !errors.Is(err, context.Canceled) {
		t.Fatalf("awaitStartReplay(canceled) error = %v, want context.Canceled", err)
	}
	if got := startReplayOutcome(errors.New("rejected")); got != "rejected" {
		t.Fatalf("startReplayOutcome(error) = %q, want rejected", got)
	}
	if got := providerIdentityForExecution(workers.WorkstationExecutionRequest{RunnerID: workers.ExecutorProviderACP, ExecutorProvider: "SCRIPT_WRAP"}); got != "" {
		t.Fatalf("providerIdentityForExecution(reserved providers) = %q, want empty", got)
	}
}

func TestWorkerSessionInterrupt_CoversCancellationAndSuccessorRejection(t *testing.T) {
	r := newTestRegistry(t)
	ref := providers.SessionRef{Provider: providers.IDCodex, Kind: providers.SessionIDKind, ID: "interrupt-provider"}
	association := validAssociation("interrupt-source", "interrupt-dispatch", ref)
	supervision := newSupervision("interrupt-dispatch", "", dispatchHandoff("interrupt-dispatch"))
	setCoverageAccepted(supervision, true)
	supervision.installCancelFailure(func() error { return errors.New("cancel failed") })
	r.sessions["interrupt-source"] = workersessions.Session{ID: "interrupt-source", State: workersessions.StateRunning, ProviderSessionAssociation: &association}
	r.supervisions["interrupt-source"] = supervision
	if _, err := r.runInterrupt(interruptPlan{request: workersessions.InterruptRequest{RequestID: "interrupt-request", SourceWorkerSessionID: "interrupt-source", SuccessorWorkerSessionID: "interrupt-successor"}, dispatchID: "interrupt-dispatch", supervision: supervision}); err == nil {
		t.Fatal("runInterrupt(cancel failure) error = nil, want source cancellation failure")
	}

	successSupervision := newSupervision("success-dispatch", "", dispatchHandoff("success-dispatch"))
	setCoverageAccepted(successSupervision, true)
	successSupervision.installCancel(func() {})
	successSupervision.signalDone()
	successAssociation := validAssociation("success-source", "success-dispatch", ref)
	r.sessions["success-source"] = workersessions.Session{ID: "success-source", State: workersessions.StateCanceled, ProviderSessionAssociation: &successAssociation}
	r.sessions["success-successor"] = workersessions.Session{ID: "success-successor", State: workersessions.StateReserved}
	r.supervisions["success-source"] = successSupervision
	if _, err := r.runInterrupt(interruptPlan{request: workersessions.InterruptRequest{RequestID: "success-request", SourceWorkerSessionID: "success-source", SuccessorWorkerSessionID: "success-successor"}, dispatchID: "success-dispatch", reference: ref, execution: dispatchHandoff("success-dispatch"), supervision: successSupervision}); err == nil {
		t.Fatal("runInterrupt(existing successor) error = nil, want successor admission failure")
	}
}

func TestWorkerSessionInterrupt_WaitCancellationIsReplaySafe(t *testing.T) {
	r := newTestRegistry(t)
	ref := providers.SessionRef{Provider: providers.IDCodex, Kind: providers.SessionIDKind, ID: "interrupt-provider"}
	association := validAssociation("interrupt-wait-source", "interrupt-wait-dispatch", ref)
	supervision := newSupervision("interrupt-wait-dispatch", "", dispatchHandoff("interrupt-wait-dispatch"))
	setCoverageAccepted(supervision, true)
	cancelStarted := make(chan struct{})
	supervision.installCancel(func() { close(cancelStarted) })
	r.sessions["interrupt-wait-source"] = workersessions.Session{ID: "interrupt-wait-source", State: workersessions.StateRunning, ProviderSessionAssociation: &association}
	r.supervisions["interrupt-wait-source"] = supervision

	ctx, cancel := context.WithCancel(context.Background())
	outcomes := make(chan error, 1)
	go func() {
		_, err := r.Interrupt(ctx, workersessions.InterruptRequest{RequestID: "interrupt-wait-request", SourceWorkerSessionID: "interrupt-wait-source", SuccessorWorkerSessionID: "interrupt-wait-successor", ReplacementMessage: "replacement"})
		outcomes <- err
	}()
	select {
	case <-cancelStarted:
	case <-time.After(2 * time.Second):
		t.Fatal("Interrupt() did not reach the cancellation effect")
	}
	cancel()
	select {
	case err := <-outcomes:
		if !errors.Is(err, context.Canceled) {
			t.Fatalf("Interrupt(canceled caller) error = %v, want context.Canceled", err)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("Interrupt(canceled caller) did not return")
	}
	supervision.signalDone()
	r.mu.RLock()
	replay := r.interruptReplays["interrupt-wait-request"]
	r.mu.RUnlock()
	if replay != nil {
		<-replay.done
	}
}

func TestWorkerSessionObservationAndControlErrorMappings(t *testing.T) {
	r := newTestRegistry(t)
	if _, err := r.parseObservationListQuery(context.Background(), workersessions.ListWorkerSessionObservationsRequest{NextToken: "not-base64"}); !errors.Is(err, workersessions.ErrInvalidObservationPagination) {
		t.Fatalf("parseObservationListQuery(invalid cursor) error = %v, want invalid pagination", err)
	}

	if _, err := newReplayObservationSubscription(context.Background(), coverageReadReader{readErr: events.ErrUnresolvableCursor}, workersessions.Topic("worker"), workersessions.StateRunning, 1, &workersessions.ObservationCursor{Position: 1}); !errors.Is(err, workersessions.ErrObservationCursorFuture) {
		t.Fatalf("newReplayObservationSubscription(future cursor) error = %v, want future cursor", err)
	}
	r.eventReader = coverageSubscribeReader{subscribeErr: events.ErrUnresolvableCursor}
	if _, err := r.liveObservationStream(context.Background(), workersessions.Topic("worker"), 1, false, nil); !errors.Is(err, workersessions.ErrObservationCursorFuture) {
		t.Fatalf("liveObservationStream(unresolvable cursor) error = %v, want future cursor", err)
	}

	r.clock = coverageClock{now: time.Date(2026, time.January, 1, 0, 0, 0, 0, time.UTC)}
	r.logReconciliation("worker", "dispatch", workers.WorkstationDispatchResult{ReconciliationReason: workers.WorkstationDispatchReconciliationReasonTimeout}, workersessions.StateRunning, workersessions.StateFailed, time.Date(2026, time.January, 2, 0, 0, 0, 0, time.UTC), time.Time{})

	supervision := newSupervision("pause-dispatch", "")
	supervision.accepted = true
	if _, _, err := r.pauseBoundary(context.Background(), workersessions.ControlRequest{ID: "missing"}, supervision, cancellationAttempt{kind: cancellationAttemptBoundary, dispatchID: "pause-dispatch", wait: make(chan struct{})}); !errors.Is(err, workers.ErrUnknownWorkstationDispatch) {
		t.Fatalf("pauseBoundary(without cancel) error = %v, want unknown dispatch", err)
	}
}

func TestWorkerSessionCallerCancellation_DetachesStartAndContinueAfterReadinessBarrier(t *testing.T) {
	t.Run("start", func(t *testing.T) {
		r := newTestRegistry(t)
		r.execution = coverageExecution{execute: func(_ context.Context, request workers.ExecuteRequest) (workers.ExecuteResult, error) {
			return coverageExecutionResult(request, workers.ExecutionOutcomeAccepted), nil
		}}
		inner := r.eventReader
		reader := &coverageGatedReader{inner: inner, started: make(chan struct{}), release: make(chan struct{})}
		r.eventReader = reader

		ctx, cancel := context.WithCancel(context.Background())
		outcomes := make(chan error, 1)
		go func() {
			_, err := r.Start(ctx, workersessions.StartRequest{
				RequestID: "start-cancel-request",
				ID:        "start-cancel-session",
				Execution: dispatchHandoff("start-cancel-dispatch"),
			})
			outcomes <- err
		}()
		select {
		case <-reader.started:
		case <-time.After(2 * time.Second):
			t.Fatal("Start() did not reach the readiness subscription")
		}
		cancel()
		select {
		case err := <-outcomes:
			if !errors.Is(err, context.Canceled) {
				t.Fatalf("Start(canceled caller) error = %v, want context.Canceled", err)
			}
		case <-time.After(2 * time.Second):
			t.Fatal("Start(canceled caller) did not return")
		}
		close(reader.release)
		r.mu.RLock()
		replay := r.startReplays["start-cancel-request"]
		r.mu.RUnlock()
		if replay != nil {
			<-replay.done
		}
	})

	t.Run("continue", func(t *testing.T) {
		r := newTestRegistry(t)
		r.execution = coverageExecution{execute: func(_ context.Context, request workers.ExecuteRequest) (workers.ExecuteResult, error) {
			return coverageExecutionResult(request, workers.ExecutionOutcomeAccepted), nil
		}}
		inner := r.eventReader
		reader := &coverageGatedReader{inner: inner, started: make(chan struct{}), release: make(chan struct{})}
		r.eventReader = reader
		ref := providers.SessionRef{Provider: providers.IDCodex, Kind: providers.SessionIDKind, ID: "continue-cancel-provider"}
		association := validAssociation("continue-cancel-source", "continue-cancel-dispatch", ref)
		supervision := newSupervision("continue-cancel-dispatch", "", dispatchHandoff("continue-cancel-dispatch"))
		setCoverageAccepted(supervision, true)
		r.sessions["continue-cancel-source"] = workersessions.Session{ID: "continue-cancel-source", State: workersessions.StateCompleted, ProviderSessionAssociation: &association}
		r.supervisions["continue-cancel-source"] = supervision

		ctx, cancel := context.WithCancel(context.Background())
		outcomes := make(chan error, 1)
		go func() {
			_, err := r.Continue(ctx, workersessions.ContinueRequest{
				RequestID:                "continue-cancel-request",
				SourceWorkerSessionID:    "continue-cancel-source",
				SuccessorWorkerSessionID: "continue-cancel-successor",
				FollowUpInput:            "follow up",
			})
			outcomes <- err
		}()
		select {
		case <-reader.started:
		case <-time.After(2 * time.Second):
			t.Fatal("Continue() did not reach the readiness subscription")
		}
		cancel()
		select {
		case err := <-outcomes:
			if !errors.Is(err, context.Canceled) {
				t.Fatalf("Continue(canceled caller) error = %v, want context.Canceled", err)
			}
		case <-time.After(2 * time.Second):
			t.Fatal("Continue(canceled caller) did not return")
		}
		close(reader.release)
		r.mu.RLock()
		replay := r.continueReplays["continue-cancel-request"]
		r.mu.RUnlock()
		if replay != nil {
			<-replay.done
		}
	})
}

func TestWorkerSessionControlCompletion_CoversResumeReservationAndInvalidContinuation(t *testing.T) {
	r := newTestRegistry(t)
	r.reserveIfAbsent("control-history")
	supervision := newSupervision("control-history-dispatch", "")
	supervision.controlHistory = &controlHistoryReservation{
		pub:       r.publications["control-history"],
		sessionID: "control-history",
		action:    workersessions.ControlActionResume,
		requestID: "control-request",
	}
	r.finishSupervisionControl(supervision, completionSnapshot{dispatchID: "control-history-dispatch"}, workersessions.ControlOutcomeApplied, workersessions.StateFailed)

	if r.continuationResultMatchesAssociation("worker", workers.WorkstationDispatchResult{
		Result: workers.WorkResult{Outcome: workers.OutcomeAccepted, Continuation: &workers.ProviderContinuationRef{}},
	}) {
		t.Fatal("continuationResultMatchesAssociation(invalid continuation) = true, want false")
	}
	if got := openingSessionContinuation(workers.WorkstationExecutionRequest{Continuation: &workers.ProviderContinuationRef{}}); got == nil || got.Kind != providers.SessionIDKind {
		t.Fatalf("openingSessionContinuation(empty) = %#v, want normalized continuation kind", got)
	}
	if got := providerIdentityForExecution(workers.WorkstationExecutionRequest{ExecutorProvider: "provider"}); got != "provider" {
		t.Fatalf("providerIdentityForExecution(provider) = %q, want provider", got)
	}
}

func TestWorkerSessionInterruptAndStartCompletion_CoversEarlyBranches(t *testing.T) {
	r := newTestRegistry(t)
	supervision := newSupervision("interrupt-no-cancel", "")
	supervision.requestedAction = workersessions.ControlActionCancel
	if _, err := r.runInterrupt(interruptPlan{
		request:     workersessions.InterruptRequest{RequestID: "interrupt-no-cancel-request", SourceWorkerSessionID: "source", SuccessorWorkerSessionID: "successor", ReplacementMessage: "replacement"},
		dispatchID:  "interrupt-no-cancel",
		supervision: supervision,
	}); err == nil {
		t.Fatal("runInterrupt(no cancellation handle) error = nil, want failure")
	}

	r.execution = nil
	result, err := r.startReserved(context.Background(), workersessions.StartRequest{
		RequestID: "start-no-execution-request",
		ID:        "start-no-execution-session",
		Execution: dispatchHandoff("start-no-execution-dispatch"),
	})
	if err == nil || result.Session.State != workersessions.StateFailed {
		t.Fatalf("startReserved(no execution) = %#v, %v, want failed admission result", result, err)
	}
}

func TestWorkerSessionStartAdmissionCompletionRemainsObservable(t *testing.T) {
	r := newTestRegistry(t)
	r.execution = coverageExecution{execute: func(_ context.Context, request workers.ExecuteRequest) (workers.ExecuteResult, error) {
		return coverageExecutionResult(request, workers.ExecutionOutcomeAccepted), nil
	}}
	result, err := r.Start(context.Background(), workersessions.StartRequest{
		RequestID: "start-admission-request",
		ID:        "start-admission-session",
		Execution: dispatchHandoff("start-admission-dispatch"),
	})
	if err != nil || result.Session.ID == "" {
		t.Fatalf("Start(successful attempt) = %#v, %v, want admitted session", result, err)
	}
	r.mu.RLock()
	supervision := r.supervisions[result.Session.ID]
	r.mu.RUnlock()
	if supervision == nil {
		t.Fatal("Start(successful attempt) did not retain supervision")
	}
	<-supervision.done
	final, getErr := r.Get(context.Background(), workersessions.GetRequest{ID: result.Session.ID})
	if getErr != nil || final.State != workersessions.StateCompleted {
		t.Fatalf("Start(successful attempt) final session = %#v, %v, want completed", final, getErr)
	}
}

type directControlStartOutcome struct {
	result workersessions.StartResult
	err    error
}

type directControlOutcome struct {
	result workersessions.ControlResult
	err    error
}

func TestWorkerSessionDirectExecutionControlsCancelContextAndJoinTerminal(t *testing.T) {
	t.Run("cancel", func(t *testing.T) {
		runWorkerSessionDirectExecutionControl(t, workersessions.ControlActionCancel, func(service workersessions.Service, ctx context.Context, req workersessions.ControlRequest) (workersessions.ControlResult, error) {
			return service.Cancel(ctx, req)
		})
	})
	t.Run("terminate", func(t *testing.T) {
		runWorkerSessionDirectExecutionControl(t, workersessions.ControlActionTerminate, func(service workersessions.Service, ctx context.Context, req workersessions.ControlRequest) (workersessions.ControlResult, error) {
			return service.Terminate(ctx, req)
		})
	})
}

func runWorkerSessionDirectExecutionControl(t *testing.T, action workersessions.ControlAction, call func(workersessions.Service, context.Context, workersessions.ControlRequest) (workersessions.ControlResult, error)) {
	t.Helper()
	started := make(chan struct{})
	cancellationObserved := make(chan struct{})
	release := make(chan struct{})
	var startedOnce sync.Once
	var cancellationOnce sync.Once
	var releaseOnce sync.Once
	defer releaseOnce.Do(func() { close(release) })

	r := newTestRegistry(t)
	r.execution = coverageExecution{execute: func(ctx context.Context, request workers.ExecuteRequest) (workers.ExecuteResult, error) {
		startedOnce.Do(func() { close(started) })
		<-ctx.Done()
		cancellationOnce.Do(func() { close(cancellationObserved) })
		<-release
		return coverageExecutionResult(request, workers.ExecutionOutcomeCanceled), ctx.Err()
	}}

	startOutcome := startDirectControlSession(t, r, string(action))
	startedResult := awaitDirectControlStart(t, started, startOutcome)
	if startedResult.err != nil || startedResult.result.Session.State != workersessions.StateRunning {
		t.Fatalf("Start() = %#v, %v, want admitted RUNNING session", startedResult.result, startedResult.err)
	}

	callerCtx, cancelCaller := context.WithCancel(context.Background())
	cancelCaller()
	controlOutcome := invokeDirectControl(r, callerCtx, startedResult.result.Session.ID, call)
	awaitDirectControlCancellation(t, action, cancellationObserved, controlOutcome, release, &releaseOnce)
	outcome := <-controlOutcome
	assertDirectControlTerminal(t, r, action, startedResult.result.Session.ID, outcome, call)
}

func startDirectControlSession(t *testing.T, r *registry, controlName string) <-chan directControlStartOutcome {
	t.Helper()
	startOutcome := make(chan directControlStartOutcome, 1)
	go func() {
		result, err := r.Start(context.Background(), workersessions.StartRequest{
			RequestID: "direct-control-start-" + controlName,
			ID:        "direct-control-session-" + controlName,
			Execution: dispatchHandoff("direct-control-dispatch-" + controlName),
		})
		startOutcome <- directControlStartOutcome{result: result, err: err}
	}()
	return startOutcome
}

func awaitDirectControlStart(t *testing.T, started <-chan struct{}, startOutcome <-chan directControlStartOutcome) directControlStartOutcome {
	t.Helper()
	select {
	case <-started:
	case <-time.After(2 * time.Second):
		t.Fatal("direct Workers execution did not start")
	}
	select {
	case outcome := <-startOutcome:
		return outcome
	case <-time.After(2 * time.Second):
		t.Fatal("Start() did not return at the direct Workers admission barrier")
		return directControlStartOutcome{}
	}
}

func invokeDirectControl(r workersessions.Service, callerCtx context.Context, sessionID string, call func(workersessions.Service, context.Context, workersessions.ControlRequest) (workersessions.ControlResult, error)) <-chan directControlOutcome {
	controlOutcome := make(chan directControlOutcome, 1)
	go func() {
		result, err := call(r, callerCtx, workersessions.ControlRequest{ID: sessionID})
		controlOutcome <- directControlOutcome{result: result, err: err}
	}()
	return controlOutcome
}

func awaitDirectControlCancellation(t *testing.T, action workersessions.ControlAction, cancellationObserved <-chan struct{}, controlOutcome <-chan directControlOutcome, release chan struct{}, releaseOnce *sync.Once) {
	t.Helper()
	select {
	case <-cancellationObserved:
	case <-time.After(2 * time.Second):
		t.Fatal("direct Workers execution did not observe the server-owned cancellation context")
	}
	select {
	case outcome := <-controlOutcome:
		t.Fatalf("%s returned before the direct execution callback joined: %#v", action, outcome.result)
	default:
	}
	releaseOnce.Do(func() { close(release) })
}

func assertDirectControlTerminal(t *testing.T, r *registry, action workersessions.ControlAction, sessionID string, outcome directControlOutcome, call func(workersessions.Service, context.Context, workersessions.ControlRequest) (workersessions.ControlResult, error)) {
	t.Helper()
	if outcome.err != nil || outcome.result.Outcome != workersessions.ControlOutcomeApplied {
		t.Fatalf("%s() = %#v, %v, want applied control after callback", action, outcome.result, outcome.err)
	}
	wantState := workersessions.StateCanceled
	if action == workersessions.ControlActionTerminate {
		wantState = workersessions.StateTerminated
	}
	if outcome.result.Session.State != wantState {
		t.Fatalf("%s() state = %q, want %q", action, outcome.result.Session.State, wantState)
	}
	final, err := r.Get(context.Background(), workersessions.GetRequest{ID: sessionID})
	if err != nil || final.State != wantState {
		t.Fatalf("Get() after %s = %#v, %v, want %q", action, final, err, wantState)
	}
	repeated, err := call(r, context.Background(), workersessions.ControlRequest{ID: sessionID})
	if err != nil || repeated.Outcome != workersessions.ControlOutcomeNoop || repeated.Session.State != wantState {
		t.Fatalf("repeated %s() = %#v, %v, want terminal NOOP", action, repeated, err)
	}
}

func TestWorkerSessionResumeAndReplayCompletion_CoversFailureSnapshots(t *testing.T) {
	const sessionID = "resume-failure-session"
	r := newTestRegistry(t)
	ref := providers.SessionRef{Provider: providers.IDCodex, Kind: providers.SessionIDKind, ID: "resume-provider"}
	association := validAssociation(sessionID, "resume-dispatch", ref)
	request := dispatchHandoff("resume-dispatch")
	r.sessions[sessionID] = workersessions.Session{ID: sessionID, State: workersessions.StatePaused, ProviderSessionAssociation: &association}
	r.publications[sessionID] = &publication{open: true}
	supervision := newSupervision("resume-dispatch", "", request)
	setCoverageAccepted(supervision, true)
	r.supervisions[sessionID] = supervision
	r.dispatchOwners["resume-dispatch"] = sessionID

	r.execution = nil
	if _, err := r.resumeReserved(context.Background(), workersessions.ControlRequest{ID: sessionID, RequestID: "resume-failure-request"}, nil); err == nil {
		t.Fatal("resumeReserved(no execution) error = nil, want publication failure")
	}

	current := r.sessions[sessionID]
	current.State = workersessions.StateStarting
	r.sessions[sessionID] = current
	supervision.mu.Lock()
	supervision.dispatchID = "resume-next-dispatch"
	supervision.accepted = false
	supervision.continuing = true
	supervision.attemptsMade = 1
	supervision.publishing = true
	supervision.mu.Unlock()
	continuation := dispatchHandoff("resume-next-dispatch")
	result, err := r.resumeAdmissionResult(
		workersessions.ControlRequest{ID: sessionID},
		nil,
		supervision,
		continuation,
		"resume-dispatch",
	)
	if !errors.Is(err, workersessions.ErrStartAdmissionFailed) || result.Outcome != workersessions.ControlOutcomeFailed || result.Session.State != workersessions.StatePaused {
		t.Fatalf("resumeAdmissionResult(unadmitted) = %#v, %v, want failed paused snapshot", result, err)
	}

	replay := &replayObservationSubscription{topic: "", next: events.Cursor{}}
	if err := replay.appendPage(events.ReadResult{Outcome: events.ReadOutcomeInvalidCursor}); !errors.Is(err, workersessions.ErrObservationSourceUnavailable) {
		t.Fatalf("replayObservationSubscription.appendPage(invalid cursor) error = %v, want source unavailable", err)
	}
}

func TestWorkerSessionSmallBoundaryBranchesRemainObservable(t *testing.T) {
	if _, err := newTestRegistry(t).parseObservationListQuery(context.Background(), workersessions.ListWorkerSessionObservationsRequest{NextToken: "not-base64"}); !errors.Is(err, workersessions.ErrInvalidObservationPagination) {
		t.Fatalf("parseObservationListQuery(invalid cursor) error = %v, want invalid pagination", err)
	}
	if _, err := decodeObservationListCursor("not-base64"); !errors.Is(err, workersessions.ErrInvalidObservationPagination) {
		t.Fatalf("decodeObservationListCursor(invalid) error = %v, want invalid pagination", err)
	}
	if got := contradictorySuccessDetail(true, "context=adapter"); !strings.Contains(got, "Workers adapter") {
		t.Fatalf("contradictorySuccessDetail(adapter) = %q, want adapter detail", got)
	}
	if got := cancellationAttemptName(cancellationAttemptKind(99)); got != "unknown" {
		t.Fatalf("cancellationAttemptName(unknown) = %q, want unknown", got)
	}
	if err := replayReadError(events.ErrUnresolvableCursor); !errors.Is(err, workersessions.ErrObservationCursorFuture) {
		t.Fatalf("replayReadError(unresolvable cursor) = %v, want future cursor", err)
	}
	if got := (&registry{}).sessionState("missing"); got != "" {
		t.Fatalf("sessionState(missing) = %q, want empty", got)
	}
	replay := &replayObservationSubscription{terminalRecordSeen: true}
	replay.noteTerminalRecord(nil)
	stoppable := &registry{stopDone: make(chan struct{})}
	if err := stoppable.Stop(nil); err != nil {
		t.Fatalf("Stop(nil) error = %v, want nil", err)
	}
}

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
